package storage

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"math"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/phm-aguiar/ai-memory-go/internal/domain"
)

// Janela de candidatos (SR §11.3 passo 1): clamp(limite × 4, mínimo 20, máximo limite + 300).
const (
	candidateFactor   = 4
	candidateFloor    = 20
	candidateHeadroom = 300
)

// Tokenização da consulta de entidades (SR §11.3): tokens de 3 a 64 caracteres, no máximo 12.
const (
	entityTokenMinRunes = 3
	entityMaxTokens     = 12
)

// Cada fluxo abaixo filtra o escopo com o mesmo critério comum: só versões correntes
// (is_latest = 1) e não expiradas. Expiração: expires_at > corte, com o corte em microssegundos
// Unix UTC, a mesma unidade de created_at e valid_from. Expiração NULL nunca expira.
// Nenhum fluxo escreve: todos rodam dentro de Reader.Read.

// CandidateLimit devolve o tamanho da janela de candidatos de cada fluxo para um limite de
// resposta. Função pura: clamp(limite × 4, mínimo 20, máximo limite + 300). Limite menor ou
// igual a zero devolve 0 (a janela também é zero).
func CandidateLimit(limit int) int {
	if limit <= 0 {
		return 0
	}
	ceiling := math.MaxInt
	if limit <= math.MaxInt-candidateHeadroom {
		ceiling = limit + candidateHeadroom
	}
	c := math.MaxInt
	if limit <= math.MaxInt/candidateFactor {
		c = limit * candidateFactor
	}
	if c < candidateFloor {
		c = candidateFloor
	}
	if c > ceiling {
		c = ceiling
	}
	return c
}

// FTSFlow é o fluxo de texto. Usa prepareFTS5Query(q) e ordena por BM25 (ORDER BY rank),
// com caminho como desempate. Se a consulta preparada é vazia, devolve lista vazia sem I/O.
// Não há checagem extra de stop words: só stop words ou só pontuação são tratadas por
// prepareFTS5Query. Limite é o tamanho da janela, use CandidateLimit.
func FTSFlow(ctx context.Context, r Reader, scope domain.Scope, q string, cutoff time.Time, limit int) ([]domain.PageRef, error) {
	match := prepareFTS5Query(q)
	if match == "" || limit <= 0 {
		return []domain.PageRef{}, nil
	}
	var paths []string
	err := r.Read(ctx, func(tx *sql.Tx) error {
		var err error
		paths, err = queryPaths(ctx, tx,
			`SELECT pages.path
			   FROM pages_fts
			   JOIN pages ON pages.rowid = pages_fts.rowid
			  WHERE pages_fts MATCH ?
			    AND pages.workspace_id = ? AND pages.project_id = ?
			    AND pages.is_latest = 1
			    AND (pages.expires_at IS NULL OR pages.expires_at > ?)
			  ORDER BY pages_fts.rank, pages.path
			  LIMIT ?`,
			match, scope.WorkspaceID.Bytes(), scope.ProjectID.Bytes(), cutoff.UnixMicro(), limit,
		)
		return err
	})
	if err != nil {
		return nil, fmt.Errorf("fluxo FTS: %w", err)
	}
	return pathRefs(scope, paths)
}

// EntityFlow é o fluxo de entidades. A consulta vira tokens (entityQueryTokens). Um token casa
// uma entidade do projeto se o nome começa com o token, ou se o token vem logo depois de espaço,
// `-` ou `_` no nome. O peso de uma página é a soma de 1/freq por entidade casada, em que freq
// é o número de páginas correntes do projeto com essa entidade (vínculo com superseded_at NULL).
// Ordem: peso desc, número de entidades casadas desc, caminho asc. Limite é o tamanho da janela.
func EntityFlow(ctx context.Context, r Reader, scope domain.Scope, q string, cutoff time.Time, limit int) ([]domain.PageRef, error) {
	tokens := entityQueryTokens(q)
	if len(tokens) == 0 || limit <= 0 {
		return []domain.PageRef{}, nil
	}
	ws := scope.WorkspaceID.Bytes()
	pj := scope.ProjectID.Bytes()
	now := cutoff.UnixMicro()

	var paths []string
	err := r.Read(ctx, func(tx *sql.Tx) error {
		hits, err := matchEntities(ctx, tx, ws, pj, tokens)
		if err != nil {
			return err
		}

		type score struct {
			path    string
			weight  float64
			matches int
		}
		byPath := map[string]*score{}
		// hits vêm ordenados por nome: a soma de pesos segue sempre a mesma ordem.
		for _, h := range hits {
			freq, err := entityFreq(ctx, tx, ws, pj, h.id)
			if err != nil {
				return err
			}
			if freq == 0 {
				continue
			}
			share := 1.0 / float64(freq)
			found, err := queryPaths(ctx, tx, entityPagesSQL, h.id, ws, pj, now)
			if err != nil {
				return fmt.Errorf("páginas da entidade %q: %w", h.name, err)
			}
			for _, p := range found {
				s := byPath[p]
				if s == nil {
					s = &score{path: p}
					byPath[p] = s
				}
				s.weight += share
				s.matches++
			}
		}

		list := make([]*score, 0, len(byPath))
		for _, s := range byPath {
			list = append(list, s)
		}
		sort.Slice(list, func(i, j int) bool {
			if list[i].weight != list[j].weight {
				return list[i].weight > list[j].weight
			}
			if list[i].matches != list[j].matches {
				return list[i].matches > list[j].matches
			}
			return list[i].path < list[j].path
		})
		for i := 0; i < len(list) && i < limit; i++ {
			paths = append(paths, list[i].path)
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("fluxo de entidades: %w", err)
	}
	return pathRefs(scope, paths)
}

// SeedsFromFlows une fluxos (FTS e entidades) em sementes sem repetição, por posição: primeiro a
// posição 1 de cada fluxo, na ordem dos fluxos; depois a posição 2; e assim por diante. Função pura.
func SeedsFromFlows(flows ...[]domain.PageRef) []domain.PageRef {
	out := []domain.PageRef{}
	seen := map[domain.PageRef]struct{}{}
	longest := 0
	for _, f := range flows {
		longest = max(longest, len(f))
	}
	for i := 0; i < longest; i++ {
		for _, f := range flows {
			if i >= len(f) {
				continue
			}
			if _, dup := seen[f[i]]; dup {
				continue
			}
			seen[f[i]] = struct{}{}
			out = append(out, f[i])
		}
	}
	return out
}

// GraphFlow é o fluxo de vizinhos de link. Para cada semente, na ordem dada, devolve primeiro os
// vizinhos de saída (links com from = semente) e depois os de entrada (links com to = semente).
// Cada direção vem ordenada por updated_at desc e caminho asc. Links pendentes (to_page_id NULL)
// não são percorridos. Um vizinho que é semente não aparece, e cada página aparece uma vez, na
// primeira posição. Os vizinhos precisam ser correntes, não expirados e do mesmo escopo.
// O tipo da aresta é informativo e não entra na ordem. Limite corta a saída (use CandidateLimit).
func GraphFlow(ctx context.Context, r Reader, scope domain.Scope, seeds []domain.PageRef, cutoff time.Time, limit int) ([]domain.PageRef, error) {
	if len(seeds) == 0 || limit <= 0 {
		return []domain.PageRef{}, nil
	}
	seedSet := make(map[domain.PageRef]struct{}, len(seeds))
	for _, s := range seeds {
		seedSet[s] = struct{}{}
	}
	ws := scope.WorkspaceID.Bytes()
	pj := scope.ProjectID.Bytes()
	now := cutoff.UnixMicro()

	out := []domain.PageRef{}
	seen := map[domain.PageRef]struct{}{}
	expanded := map[domain.PageRef]struct{}{}

	err := r.Read(ctx, func(tx *sql.Tx) error {
		for _, seed := range seeds {
			if _, done := expanded[seed]; done {
				continue
			}
			expanded[seed] = struct{}{}

			id, found, err := currentPageID(ctx, tx, ws, pj, seed.Path.String(), now)
			if err != nil {
				return err
			}
			if !found {
				continue
			}

			for _, outgoing := range []bool{true, false} {
				query := graphInSQL
				if outgoing {
					query = graphOutSQL
				}
				paths, err := queryPaths(ctx, tx, query, id, ws, pj, now)
				if err != nil {
					return fmt.Errorf("vizinhos de %s: %w", seed.String(), err)
				}
				refs, err := pathRefs(scope, paths)
				if err != nil {
					return err
				}
				for _, ref := range refs {
					if _, isSeed := seedSet[ref]; isSeed {
						continue
					}
					if _, dup := seen[ref]; dup {
						continue
					}
					seen[ref] = struct{}{}
					out = append(out, ref)
					if len(out) >= limit {
						return nil
					}
				}
			}
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("fluxo de grafo: %w", err)
	}
	return out, nil
}

// entityQueryTokens tokeniza a consulta de entidades. Separa por espaço e põe em minúsculas.
// Mantém tokens de 3 a 64 caracteres (em runas), sem repetição, até 12 no total. Um token
// com `-` ou `_` também gera as partes separadas por esses caracteres; o token inteiro conta
// como um só. A ordem é a de aparição.
func entityQueryTokens(q string) []string {
	out := make([]string, 0, entityMaxTokens)
	seen := map[string]struct{}{}
	add := func(t string) {
		if len(out) >= entityMaxTokens {
			return
		}
		n := utf8.RuneCountInString(t)
		if n < entityTokenMinRunes || n > MaxEntityRunes {
			return
		}
		if _, dup := seen[t]; dup {
			return
		}
		seen[t] = struct{}{}
		out = append(out, t)
	}
	isSep := func(r rune) bool { return r == '-' || r == '_' }
	for _, raw := range strings.Fields(strings.ToLower(q)) {
		add(raw)
		if strings.ContainsAny(raw, "-_") {
			for _, part := range strings.FieldsFunc(raw, isSep) {
				add(part)
			}
		}
	}
	return out
}

// entityHit é uma entidade do projeto casada por algum token.
type entityHit struct {
	id   []byte
	name string
}

// matchEntities devolve as entidades casadas por algum token, sem repetição, ordenadas por nome.
// instr é usado em vez de LIKE para que `_` e `%` no token não sejam curingas.
func matchEntities(ctx context.Context, tx *sql.Tx, ws, pj []byte, tokens []string) ([]entityHit, error) {
	byID := map[string]entityHit{}
	for _, t := range tokens {
		hits, err := entitiesMatching(ctx, tx, ws, pj, t)
		if err != nil {
			return nil, err
		}
		for _, h := range hits {
			byID[string(h.id)] = h
		}
	}
	out := make([]entityHit, 0, len(byID))
	for _, h := range byID {
		out = append(out, h)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].name < out[j].name })
	return out, nil
}

func entitiesMatching(ctx context.Context, tx *sql.Tx, ws, pj []byte, tok string) ([]entityHit, error) {
	rows, err := tx.QueryContext(ctx,
		`SELECT id, name FROM entities
		  WHERE workspace_id = ? AND project_id = ?
		    AND (instr(name, ?) = 1 OR instr(name, ?) > 0 OR instr(name, ?) > 0 OR instr(name, ?) > 0)`,
		ws, pj, tok, " "+tok, "-"+tok, "_"+tok,
	)
	if err != nil {
		return nil, fmt.Errorf("casar entidades de %q: %w", tok, err)
	}
	defer func() { _ = rows.Close() }()
	var out []entityHit
	for rows.Next() {
		var h entityHit
		if err := rows.Scan(&h.id, &h.name); err != nil {
			return nil, fmt.Errorf("ler entidade casada: %w", err)
		}
		out = append(out, h)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterar entidades casadas: %w", err)
	}
	return out, nil
}

// entityFreq conta as páginas correntes do projeto com a entidade (vínculo aberto).
// Não aplica o filtro de expiração: a frequência é do projeto, não da consulta.
func entityFreq(ctx context.Context, tx *sql.Tx, ws, pj, entityID []byte) (int, error) {
	var n int
	err := tx.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM entity_page_links l JOIN pages p ON p.id = l.page_id
		  WHERE l.entity_id = ? AND l.superseded_at IS NULL
		    AND p.workspace_id = ? AND p.project_id = ? AND p.is_latest = 1`,
		entityID, ws, pj,
	).Scan(&n)
	if err != nil {
		return 0, fmt.Errorf("frequência de entidade: %w", err)
	}
	return n, nil
}

const entityPagesSQL = `SELECT p.path FROM entity_page_links l JOIN pages p ON p.id = l.page_id
	 WHERE l.entity_id = ? AND l.superseded_at IS NULL
	   AND p.workspace_id = ? AND p.project_id = ? AND p.is_latest = 1
	   AND (p.expires_at IS NULL OR p.expires_at > ?)`

const graphOutSQL = `SELECT p.path FROM links l JOIN pages p ON p.id = l.to_page_id
	 WHERE l.from_page_id = ? AND l.to_page_id IS NOT NULL
	   AND p.workspace_id = ? AND p.project_id = ? AND p.is_latest = 1
	   AND (p.expires_at IS NULL OR p.expires_at > ?)
	 ORDER BY p.updated_at DESC, p.path ASC`

const graphInSQL = `SELECT p.path FROM links l JOIN pages p ON p.id = l.from_page_id
	 WHERE l.to_page_id = ?
	   AND p.workspace_id = ? AND p.project_id = ? AND p.is_latest = 1
	   AND (p.expires_at IS NULL OR p.expires_at > ?)
	 ORDER BY p.updated_at DESC, p.path ASC`

// currentPageID devolve o id da versão corrente, não expirada, do caminho no projeto.
func currentPageID(ctx context.Context, tx *sql.Tx, ws, pj []byte, path string, now int64) ([]byte, bool, error) {
	var id []byte
	err := tx.QueryRowContext(ctx,
		`SELECT id FROM pages
		  WHERE workspace_id = ? AND project_id = ? AND path = ? AND is_latest = 1
		    AND (expires_at IS NULL OR expires_at > ?)`,
		ws, pj, path, now,
	).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, fmt.Errorf("ler semente %q: %w", path, err)
	}
	return id, true, nil
}

// queryPaths executa uma consulta que devolve uma coluna de caminho e fecha as linhas antes de
// voltar, para que a transação possa seguir com outras consultas.
func queryPaths(ctx context.Context, tx *sql.Tx, query string, args ...any) ([]string, error) {
	rows, err := tx.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("consultar caminhos: %w", err)
	}
	defer func() { _ = rows.Close() }()
	out := []string{}
	for rows.Next() {
		var p string
		if err := rows.Scan(&p); err != nil {
			return nil, fmt.Errorf("ler caminho: %w", err)
		}
		out = append(out, p)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterar caminhos: %w", err)
	}
	return out, nil
}

// pathRefs transforma caminhos armazenados em PageRef no escopo, mantendo a ordem.
func pathRefs(scope domain.Scope, paths []string) ([]domain.PageRef, error) {
	out := make([]domain.PageRef, 0, len(paths))
	for _, s := range paths {
		pp, err := domain.ParseStoredPagePath(s)
		if err != nil {
			return nil, fmt.Errorf("caminho armazenado inválido %q: %w", s, err)
		}
		out = append(out, domain.NewPageRef(scope, pp))
	}
	return out, nil
}
