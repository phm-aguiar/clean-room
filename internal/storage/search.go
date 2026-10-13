package storage

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sort"
	"time"

	"github.com/phm-aguiar/ai-memory-go/internal/domain"
)

// searchSnippetTokens é o tamanho, em tokens, do trecho de observação devolvido no fallback.
const searchSnippetTokens = 16

// ScoredPage é uma página do resultado. Rank é o rank final, já ajustado pela autoridade
// (menor é melhor). Authority é o fator aplicado (SR §11.5).
type ScoredPage struct {
	Ref       domain.PageRef
	Rank      float64
	Authority float64
}

// RawHit é uma observação casada pelo fallback de observations_fts (ADR-32). Rank é o rank
// BM25 do FTS5 (menor é melhor). Snippet é um trecho do corpo.
type RawHit struct {
	ObservationID domain.ObservationID
	SessionID     domain.SessionID
	Kind          domain.ObservationKind
	Title         string
	Snippet       string
	Rank          float64
}

// SearchResult é o resultado de Search. RawHits só vem preenchido quando nenhuma página
// compilada casa; com páginas, RawHits fica vazio.
type SearchResult struct {
	Pages   []ScoredPage
	RawHits []RawHit
}

// Search executa a busca do escopo (SR §11.3, P-05). Limite menor ou igual a zero, ou consulta
// que prepareFTS5Query reduz a vazio, devolve resultado vazio sem erro.
//
// Passos: janela de candidatos (CandidateLimit); fluxos FTS e de entidades; sementes e fluxo de
// grafo; fusão RRF; fator de autoridade aplicado ao rank; ordenação por rank e caminho; corte em
// limit. Se nenhum dos três fluxos casa, devolve RawHits por observations_fts do escopo.
//
// Cada fluxo abre a própria leitura via r.Read, então a busca não é um único snapshot. Os
// metadados das páginas e o fallback também usam r.Read. Não escreve e não usa goroutine.
func Search(ctx context.Context, r Reader, scope domain.Scope, query string, cutoff time.Time, limit int) (SearchResult, error) {
	empty := SearchResult{Pages: []ScoredPage{}, RawHits: []RawHit{}}
	if limit <= 0 {
		return empty, nil
	}
	match := prepareFTS5Query(query)
	if match == "" {
		return empty, nil
	}

	cand := CandidateLimit(limit)
	fts, err := FTSFlow(ctx, r, scope, query, cutoff, cand)
	if err != nil {
		return SearchResult{}, fmt.Errorf("busca: %w", err)
	}
	ent, err := EntityFlow(ctx, r, scope, query, cutoff, cand)
	if err != nil {
		return SearchResult{}, fmt.Errorf("busca: %w", err)
	}
	graph, err := GraphFlow(ctx, r, scope, SeedsFromFlows(fts, ent), cutoff, cand)
	if err != nil {
		return SearchResult{}, fmt.Errorf("busca: %w", err)
	}

	fused := FuseRRF([][]domain.PageRef{fts, ent, graph})
	if len(fused) == 0 {
		hits, err := rawHits(ctx, r, scope, match, limit)
		if err != nil {
			return SearchResult{}, fmt.Errorf("busca: %w", err)
		}
		return SearchResult{Pages: []ScoredPage{}, RawHits: hits}, nil
	}

	pages, err := scoreWithAuthority(ctx, r, fused)
	if err != nil {
		return SearchResult{}, fmt.Errorf("busca: %w", err)
	}
	sortScored(pages)
	if len(pages) > limit {
		pages = pages[:limit]
	}
	return SearchResult{Pages: pages, RawHits: []RawHit{}}, nil
}

// pinnedScanner acrescenta a coluna pinned ao que scanPageRow lê. O SELECT deve trazer
// pageReadCols seguido de pinned.
type pinnedScanner struct {
	row    rowScanner
	pinned int
}

func (p *pinnedScanner) Scan(dest ...any) error {
	return p.row.Scan(append(dest, &p.pinned)...)
}

// scoreWithAuthority carrega a versão corrente de cada resultado do RRF, calcula o fator de
// autoridade e aplica ao rank. Uma página que sumiu entre a leitura do fluxo e esta (troca de
// versão concorrente) sai do resultado, sem erro.
func scoreWithAuthority(ctx context.Context, r Reader, fused []RRFResult) ([]ScoredPage, error) {
	out := make([]ScoredPage, 0, len(fused))
	err := r.Read(ctx, func(tx *sql.Tx) error {
		for _, res := range fused {
			ref := res.Ref
			row := tx.QueryRowContext(ctx,
				`SELECT `+pageReadCols+`, pinned
				   FROM pages
				  WHERE workspace_id = ? AND project_id = ? AND path = ? AND is_latest = 1`,
				ref.Scope.WorkspaceID.Bytes(), ref.Scope.ProjectID.Bytes(), ref.Path.String(),
			)
			ps := &pinnedScanner{row: row}
			page, err := scanPageRow(ref.Scope, ps)
			if errors.Is(err, sql.ErrNoRows) {
				continue
			}
			if err != nil {
				return fmt.Errorf("metadados de %s: %w", ref.String(), err)
			}

			factor := Authority(AuthorityInput{
				Kind:   page.Kind,
				Tier:   page.Tier,
				Path:   ref.Path,
				Tags:   normalizedTags(page.Frontmatter),
				Pinned: ps.pinned == 1 || frontmatterPinned(page.Frontmatter),
			})
			out = append(out, ScoredPage{
				Ref:       ref,
				Rank:      ApplyAuthority(res.Rank, factor),
				Authority: factor,
			})
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

// normalizedTags lê a chave tags do frontmatter (lista de texto) e normaliza cada tag. Se a chave
// não existe ou não é lista, devolve nil.
func normalizedTags(fm domain.Frontmatter) []string {
	v, ok := fm.Get("tags")
	if !ok {
		return nil
	}
	list, ok := v.([]string)
	if !ok {
		return nil
	}
	out := make([]string, 0, len(list))
	for _, t := range list {
		out = append(out, NormalizeTag(t))
	}
	return out
}

// frontmatterPinned lê a chave pinned do frontmatter. Só o texto "true" conta como fixada.
func frontmatterPinned(fm domain.Frontmatter) bool {
	v, ok := fm.Get("pinned")
	if !ok {
		return false
	}
	s, isStr := v.(string)
	return isStr && s == "true"
}

// sortScored ordena por rank crescente (menor é melhor), depois por caminho (refLess).
func sortScored(pages []ScoredPage) {
	sort.Slice(pages, func(i, j int) bool {
		if pages[i].Rank != pages[j].Rank {
			return pages[i].Rank < pages[j].Rank
		}
		return refLess(pages[i].Ref, pages[j].Ref)
	})
}

// rawHits é o fallback: observações do escopo casadas pela consulta preparada, em
// observations_fts, ordenadas por rank BM25 e depois por criação (mais nova primeiro) e id.
// Limitado a limit.
func rawHits(ctx context.Context, r Reader, scope domain.Scope, match string, limit int) ([]RawHit, error) {
	out := []RawHit{}
	err := r.Read(ctx, func(tx *sql.Tx) error {
		rows, err := tx.QueryContext(ctx,
			`SELECT o.id, o.session_id, o.kind, o.title,
			        snippet(observations_fts, 1, '', '', '...', `+fmt.Sprint(searchSnippetTokens)+`),
			        observations_fts.rank
			   FROM observations_fts
			   JOIN observations o ON o.rowid = observations_fts.rowid
			  WHERE observations_fts MATCH ?
			    AND o.workspace_id = ? AND o.project_id = ?
			  ORDER BY observations_fts.rank, o.created_at DESC, o.id
			  LIMIT ?`,
			match, scope.WorkspaceID.Bytes(), scope.ProjectID.Bytes(), limit,
		)
		if err != nil {
			return fmt.Errorf("consultar observações: %w", err)
		}
		defer func() { _ = rows.Close() }()
		for rows.Next() {
			var (
				idBytes, sessBytes []byte
				kind, title, snip  string
				rank               float64
			)
			if err := rows.Scan(&idBytes, &sessBytes, &kind, &title, &snip, &rank); err != nil {
				return fmt.Errorf("ler observação casada: %w", err)
			}
			obsID, err := domain.ParseIDBytes(idBytes)
			if err != nil {
				return fmt.Errorf("ler id da observação: %w", err)
			}
			sessID, err := domain.ParseIDBytes(sessBytes)
			if err != nil {
				return fmt.Errorf("ler id da sessão: %w", err)
			}
			out = append(out, RawHit{
				ObservationID: domain.ObservationID(obsID),
				SessionID:     domain.SessionID(sessID),
				Kind:          domain.ObservationKind(kind),
				Title:         title,
				Snippet:       snip,
				Rank:          rank,
			})
		}
		if err := rows.Err(); err != nil {
			return fmt.Errorf("iterar observações casadas: %w", err)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}
