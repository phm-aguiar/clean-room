package acceptance

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/phm-aguiar/ai-memory-go/internal/domain"
	"github.com/phm-aguiar/ai-memory-go/internal/domain/sanitize"
	"github.com/phm-aguiar/ai-memory-go/internal/storage"
	"github.com/phm-aguiar/ai-memory-go/internal/wiki"
)

// Constantes do cenário 2 de specs/features/001-nucleo/acceptance/nucleo.feature: "Busca híbrida
// retorna páginas ranqueadas por RRF com autoridade aplicada".
const (
	rrfQuery         = "writer sqlite"
	rrfLimit         = 10
	rrfFallbackQuery = "pgbouncer"
	rrfEntity        = "writer"
	rrfBothPath      = "controle/writer-sqlite.md"
	rrfFTSOnlyPath   = "controle/so-texto.md"
	rrfFixturesDir   = "../../docs/assets/fixtures"
	rrfGoldenPath    = "testdata/rrf_golden.json"
	rrfTolerance     = 1e-12
)

// rrfFixtureFiles são as fixtures 01 a 05 de docs/assets/fixtures. São observações sem ranking
// esperado (ADR-15 de phase0-findings): o teste só grava as páginas derivadas delas.
var rrfFixtureFiles = []string{
	"01-rust-bug-fix.json",
	"02-architecture-decision.json",
	"03-gotcha-with-rule.json",
	"04-low-signal-session.json",
	"05-multi-topic-session.json",
}

// rrfFixtureTier define o tier da página de cada fixture. As fixtures 02 e 03 registram
// conhecimento durável (decisão e regra), então entram como semantic. As demais são registros de
// sessão e entram como episodic.
var rrfFixtureTier = map[string]domain.Tier{
	"01-rust-bug-fix.json":          domain.TierEpisodic,
	"02-architecture-decision.json": domain.TierSemantic,
	"03-gotcha-with-rule.json":      domain.TierSemantic,
	"04-low-signal-session.json":    domain.TierEpisodic,
	"05-multi-topic-session.json":   domain.TierEpisodic,
}

type rrfObservation struct {
	Kind  string `json:"kind"`
	Title string `json:"title"`
	Body  string `json:"body"`
}

type rrfFixture struct {
	Name         string           `json:"name"`
	Description  string           `json:"description"`
	Observations []rrfObservation `json:"observations"`
}

// rrfGolden é o ranking esperado para a consulta: a ordem dos caminhos, do melhor para o pior.
type rrfGolden struct {
	Query string   `json:"query"`
	Limit int      `json:"limit"`
	Paths []string `json:"paths"`
}

// rrfWritePage grava uma página pelo fluxo de escrita da wiki (WritePage).
func rrfWritePage(ctx context.Context, store *wiki.Store, scope domain.Scope, rel string, tier domain.Tier, fm domain.Frontmatter, body string) error {
	path, err := domain.ParsePagePath(rel)
	if err != nil {
		return fmt.Errorf("caminho %q: %w", rel, err)
	}
	_, err = store.WritePage(ctx, wiki.PageInput{
		Scope:         scope,
		WorkspaceName: "workspace-teste",
		ProjectName:   "ai-memory",
		RepoPath:      "repo-de-teste",
		Path:          path,
		Tier:          tier,
		Body:          body,
		Frontmatter:   fm,
		Evidence:      storage.EvidenceSource{Kind: storage.SourceSession, ID: "sessao-rrf"},
	})
	return err
}

// rrfFixtureBody monta o corpo da página derivada de uma fixture: a descrição e, em seguida,
// título e corpo de cada observação.
func rrfFixtureBody(fx rrfFixture) string {
	var b strings.Builder
	b.WriteString(fx.Description)
	for _, o := range fx.Observations {
		b.WriteString("\n\n## " + o.Title + "\n\n" + o.Body)
	}
	return b.String()
}

// rrfPositions devolve a posição, a partir de 1, de cada caminho num fluxo.
func rrfPositions(flow []domain.PageRef) map[string]int {
	out := make(map[string]int, len(flow))
	for i, ref := range flow {
		out[ref.Path.String()] = i + 1
	}
	return out
}

// rrfIndexOf devolve a posição (base 0) de path nos resultados, ou -1.
func rrfIndexOf(pages []storage.ScoredPage, path string) int {
	for i, p := range pages {
		if p.Ref.Path.String() == path {
			return i
		}
	}
	return -1
}

func rrfPathsOf(pages []storage.ScoredPage) []string {
	out := make([]string, 0, len(pages))
	for _, p := range pages {
		out = append(out, p.Ref.Path.String())
	}
	return out
}

// TestSearchRRFRanqueiaPaginasComAutoridade espelha o cenário 2 de
// specs/features/001-nucleo/acceptance/nucleo.feature. Os passos Dado/Quando/Então viram subtestes
// em sequência. Se um passo falha, os seguintes não rodam.
func TestSearchRRFRanqueiaPaginasComAutoridade(t *testing.T) {
	ctx := context.Background()

	dataDir := t.TempDir()

	db, err := storage.Open(ctx, dataDir)
	if err != nil {
		t.Fatalf("storage.Open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	writer, err := storage.NewWriter(ctx, db)
	if err != nil {
		t.Fatalf("storage.NewWriter: %v", err)
	}
	t.Cleanup(func() { _ = writer.Close() })

	readDB, err := storage.OpenReader(ctx, dataDir)
	if err != nil {
		t.Fatalf("storage.OpenReader: %v", err)
	}
	t.Cleanup(func() { _ = readDB.Close() })
	reader := storage.NewReader(readDB)

	wikiRoot := filepath.Join(dataDir, "wiki")
	repo, err := wiki.Open(wikiRoot)
	if err != nil {
		t.Fatalf("wiki.Open: %v", err)
	}
	store := &wiki.Store{
		Root:   wikiRoot,
		Writer: writer,
		Repo:   repo,
		Locks:  wiki.NewPageLocks(),
	}

	var (
		scope    domain.Scope
		fixtures []rrfFixture
		cutoff   time.Time
		res      storage.SearchResult
	)

	if !t.Run("Dado que o escopo ai-memory contém as páginas das fixtures", func(t *testing.T) {
		wsID, err := domain.NewID()
		if err != nil {
			t.Fatalf("NewID do workspace: %v", err)
		}
		projID, err := domain.NewID()
		if err != nil {
			t.Fatalf("NewID do projeto: %v", err)
		}
		scope = domain.NewScope(domain.WorkspaceID(wsID), domain.ProjectID(projID))
		if err := storage.EnsureScope(ctx, writer, scope, "workspace-teste", "ai-memory", time.Now()); err != nil {
			t.Fatalf("EnsureScope: %v", err)
		}

		for _, file := range rrfFixtureFiles {
			raw, err := os.ReadFile(filepath.Join(rrfFixturesDir, file))
			if err != nil {
				t.Fatalf("ler fixture %s: %v", file, err)
			}
			var fx rrfFixture
			if err := json.Unmarshal(raw, &fx); err != nil {
				t.Fatalf("decodificar fixture %s: %v", file, err)
			}
			if fx.Name == "" {
				t.Fatalf("fixture %s sem name", file)
			}
			fixtures = append(fixtures, fx)

			var fm domain.Frontmatter
			if err := fm.Set("title", fx.Name); err != nil {
				t.Fatalf("frontmatter title de %s: %v", file, err)
			}
			rel := "sessions/" + fx.Name + ".md"
			if err := rrfWritePage(ctx, store, scope, rel, rrfFixtureTier[file], fm, rrfFixtureBody(fx)); err != nil {
				t.Fatalf("WritePage de %s: %v", rel, err)
			}
		}
		if len(fixtures) != len(rrfFixtureFiles) {
			t.Fatalf("fixtures carregadas: %d, esperadas %d", len(fixtures), len(rrfFixtureFiles))
		}
	}) {
		return
	}

	if !t.Run("Dado que uma página casa tanto por FTS quanto por entidade e outra casa só por FTS", func(t *testing.T) {
		var fmBoth domain.Frontmatter
		if err := fmBoth.Set("title", "Writer e SQLite"); err != nil {
			t.Fatalf("frontmatter de %s: %v", rrfBothPath, err)
		}
		// A entidade "writer" vem do frontmatter. O corpo também contém os dois termos da consulta,
		// então esta página casa FTS e entidade.
		if err := fmBoth.Set("entities", []string{rrfEntity}); err != nil {
			t.Fatalf("entities de %s: %v", rrfBothPath, err)
		}
		bodyBoth := "O writer é o único ator de escrita do sqlite em modo WAL. Todo commit passa pelo writer."
		if err := rrfWritePage(ctx, store, scope, rrfBothPath, domain.TierSemantic, fmBoth, bodyBoth); err != nil {
			t.Fatalf("WritePage de %s: %v", rrfBothPath, err)
		}

		var fmText domain.Frontmatter
		if err := fmText.Set("title", "Notas de texto"); err != nil {
			t.Fatalf("frontmatter de %s: %v", rrfFTSOnlyPath, err)
		}
		// Sem entidade: esta página casa só o fluxo FTS.
		bodyText := "Notas sobre writer e sqlite: writer serializa, sqlite guarda. Ver writer e sqlite."
		if err := rrfWritePage(ctx, store, scope, rrfFTSOnlyPath, domain.TierSemantic, fmText, bodyText); err != nil {
			t.Fatalf("WritePage de %s: %v", rrfFTSOnlyPath, err)
		}
	}) {
		return
	}

	if !t.Run("Dado que observações do escopo estão gravadas para o fallback", func(t *testing.T) {
		sessID, err := domain.NewID()
		if err != nil {
			t.Fatalf("NewID da sessão: %v", err)
		}
		// Observações da fixture 03 mais uma com termo que não aparece em nenhuma página. Só ela
		// casa a consulta de fallback.
		obs := append([]rrfObservation{}, fixtures[2].Observations...)
		obs = append(obs, rrfObservation{
			Kind:  "other",
			Title: "pgbouncer",
			Body:  "pool do pgbouncer em modo transação, sem página compilada",
		})

		now := time.Now()
		err = writer.Do(ctx, func(tx *sql.Tx) error {
			if _, err := tx.Exec(
				`INSERT INTO sessions (id, workspace_id, project_id, agent_kind, started_at) VALUES (?, ?, ?, ?, ?)`,
				sessID.Bytes(), scope.WorkspaceID.Bytes(), scope.ProjectID.Bytes(), "claude-code", now.UnixMicro(),
			); err != nil {
				return fmt.Errorf("inserir sessão: %w", err)
			}
			for _, o := range obs {
				oid, err := domain.NewID()
				if err != nil {
					return fmt.Errorf("NewID da observação: %w", err)
				}
				ob, err := domain.NewObservation(
					domain.ObservationID(oid),
					domain.SessionID(sessID),
					scope,
					domain.ObservationKind(o.Kind),
					sanitize.Title(o.Title, sanitize.Limits{TitleMaxRunes: sanitize.ObservationTitleMaxRunes}),
					sanitize.Body(o.Body, sanitize.Limits{BodyMaxBytes: sanitize.ObservationBodyMaxBytes}),
					5,
				)
				if err != nil {
					return fmt.Errorf("montar observação: %w", err)
				}
				ob.OccurredAt = now.UnixMicro()
				if err := storage.InsertObservation(tx, ob); err != nil {
					return fmt.Errorf("inserir observação: %w", err)
				}
			}
			return nil
		})
		if err != nil {
			t.Fatalf("gravar observações: %v", err)
		}
	}) {
		return
	}

	if !t.Run("Quando o chamador faz uma consulta que casa as duas páginas", func(t *testing.T) {
		cutoff = time.Now()
		out, err := storage.Search(ctx, reader, scope, rrfQuery, cutoff, rrfLimit)
		if err != nil {
			t.Fatalf("storage.Search: %v", err)
		}
		if len(out.Pages) == 0 {
			t.Fatalf("busca %q sem páginas", rrfQuery)
		}
		res = out

		// Detalhe por página, para conferir a fusão à mão quando o golden for revisado.
		fts, err := storage.FTSFlow(ctx, reader, scope, rrfQuery, cutoff, storage.CandidateLimit(rrfLimit))
		if err != nil {
			t.Fatalf("FTSFlow: %v", err)
		}
		ent, err := storage.EntityFlow(ctx, reader, scope, rrfQuery, cutoff, storage.CandidateLimit(rrfLimit))
		if err != nil {
			t.Fatalf("EntityFlow: %v", err)
		}
		ftsPos, entPos := rrfPositions(fts), rrfPositions(ent)
		for _, p := range out.Pages {
			path := p.Ref.Path.String()
			t.Logf("resultado %s: rank=%.12f fator=%.2f pos_fts=%d pos_ent=%d",
				path, p.Rank, p.Authority, ftsPos[path], entPos[path])
		}
	}) {
		return
	}

	if !t.Run("Então as páginas saem por rank crescente, com desempate por caminho", func(t *testing.T) {
		for i := 1; i < len(res.Pages); i++ {
			prev, cur := res.Pages[i-1], res.Pages[i]
			if prev.Rank > cur.Rank {
				t.Fatalf("ordem quebrada em %d: %s (rank %v) antes de %s (rank %v)",
					i, prev.Ref.Path, prev.Rank, cur.Ref.Path, cur.Rank)
			}
			if prev.Rank == cur.Rank && prev.Ref.Path.String() > cur.Ref.Path.String() {
				t.Fatalf("desempate fora do caminho em %d: %s antes de %s", i, prev.Ref.Path, cur.Ref.Path)
			}
		}
	}) {
		return
	}

	if !t.Run("Então a página que casa FTS e entidade aparece acima da que casa só FTS", func(t *testing.T) {
		iBoth := rrfIndexOf(res.Pages, rrfBothPath)
		iText := rrfIndexOf(res.Pages, rrfFTSOnlyPath)
		if iBoth < 0 || iText < 0 {
			t.Fatalf("páginas controladas ausentes do resultado: %s no índice %d, %s no índice %d",
				rrfBothPath, iBoth, rrfFTSOnlyPath, iText)
		}
		if iBoth >= iText {
			t.Fatalf("%s (índice %d) devia ficar acima de %s (índice %d)", rrfBothPath, iBoth, rrfFTSOnlyPath, iText)
		}
	}) {
		return
	}

	if !t.Run("E o ranqueamento usa RRF com k igual a 60", func(t *testing.T) {
		if storage.RRFK != 60 {
			t.Fatalf("RRFK = %d, esperado 60", storage.RRFK)
		}
		cand := storage.CandidateLimit(rrfLimit)
		fts, err := storage.FTSFlow(ctx, reader, scope, rrfQuery, cutoff, cand)
		if err != nil {
			t.Fatalf("FTSFlow: %v", err)
		}
		ent, err := storage.EntityFlow(ctx, reader, scope, rrfQuery, cutoff, cand)
		if err != nil {
			t.Fatalf("EntityFlow: %v", err)
		}
		// Para a página que casa FTS e entidade, o score RRF é 1/(60+pos_fts) + 1/(60+pos_ent).
		ftsPos, entPos := rrfPositions(fts), rrfPositions(ent)
		pFTS, okFTS := ftsPos[rrfBothPath]
		pEnt, okEnt := entPos[rrfBothPath]
		if !okFTS || !okEnt {
			t.Fatalf("%s não está nos dois fluxos (FTS=%v, entidade=%v)", rrfBothPath, okFTS, okEnt)
		}
		score := 1.0/float64(storage.RRFK+pFTS) + 1.0/float64(storage.RRFK+pEnt)

		i := rrfIndexOf(res.Pages, rrfBothPath)
		if i < 0 {
			t.Fatalf("%s ausente do resultado", rrfBothPath)
		}
		p := res.Pages[i]
		want := storage.ApplyAuthority(-score, p.Authority)
		if math.Abs(p.Rank-want) > rrfTolerance {
			t.Fatalf("rank de %s = %.15f, esperado %.15f (score RRF %.15f, fator %.2f)",
				rrfBothPath, p.Rank, want, score, p.Authority)
		}
	}) {
		return
	}

	if !t.Run("E o multiplicador de autoridade ajusta a ordem final dentro do intervalo [0,55; 1,50]", func(t *testing.T) {
		for _, p := range res.Pages {
			if p.Authority < storage.AuthorityFloor || p.Authority > storage.AuthorityCeiling {
				t.Fatalf("fator de %s = %.4f fora de [%.2f, %.2f]",
					p.Ref.Path, p.Authority, storage.AuthorityFloor, storage.AuthorityCeiling)
			}
		}
	}) {
		return
	}

	if !t.Run("Então o limite é respeitado", func(t *testing.T) {
		const small = 2
		out, err := storage.Search(ctx, reader, scope, rrfQuery, cutoff, small)
		if err != nil {
			t.Fatalf("storage.Search com limite %d: %v", small, err)
		}
		if len(out.Pages) > small {
			t.Fatalf("limite %d: devolveu %d páginas", small, len(out.Pages))
		}
		full := rrfPathsOf(res.Pages)
		got := rrfPathsOf(out.Pages)
		for i := range got {
			if got[i] != full[i] {
				t.Fatalf("limite %d: posição %d = %s, esperado %s (prefixo do resultado completo)", small, i, got[i], full[i])
			}
		}

		zero, err := storage.Search(ctx, reader, scope, rrfQuery, cutoff, 0)
		if err != nil {
			t.Fatalf("storage.Search com limite 0: %v", err)
		}
		if len(zero.Pages) != 0 {
			t.Fatalf("limite 0 devolveu %d páginas", len(zero.Pages))
		}
	}) {
		return
	}

	if !t.Run("Então o ranking coincide com o golden de test/acceptance/testdata", func(t *testing.T) {
		raw, err := os.ReadFile(rrfGoldenPath)
		if err != nil {
			got, _ := json.MarshalIndent(rrfGolden{Query: rrfQuery, Limit: rrfLimit, Paths: rrfPathsOf(res.Pages)}, "", "  ")
			t.Fatalf("ler golden %s: %v\nranking observado (validar antes de gravar):\n%s", rrfGoldenPath, err, got)
		}
		var golden rrfGolden
		if err := json.Unmarshal(raw, &golden); err != nil {
			t.Fatalf("decodificar golden: %v", err)
		}
		if golden.Query != rrfQuery || golden.Limit != rrfLimit {
			t.Fatalf("golden de outra consulta: query=%q limit=%d", golden.Query, golden.Limit)
		}
		got := rrfPathsOf(res.Pages)
		if len(got) != len(golden.Paths) {
			t.Fatalf("ranking tem %d páginas, golden tem %d\nobservado: %v\ngolden: %v",
				len(got), len(golden.Paths), got, golden.Paths)
		}
		for i := range got {
			if got[i] != golden.Paths[i] {
				t.Fatalf("posição %d: observado %s, golden %s\nobservado: %v\ngolden: %v",
					i, got[i], golden.Paths[i], got, golden.Paths)
			}
		}
	}) {
		return
	}

	t.Run("Então consulta sem match devolve RawHits e consulta com match devolve RawHits vazio", func(t *testing.T) {
		fb, err := storage.Search(ctx, reader, scope, rrfFallbackQuery, cutoff, rrfLimit)
		if err != nil {
			t.Fatalf("storage.Search de fallback: %v", err)
		}
		if len(fb.Pages) != 0 {
			t.Fatalf("consulta %q sem match devolveu %d páginas", rrfFallbackQuery, len(fb.Pages))
		}
		if len(fb.RawHits) == 0 {
			t.Fatalf("consulta %q sem match não devolveu RawHits", rrfFallbackQuery)
		}
		if fb.RawHits[0].Title != "pgbouncer" {
			t.Fatalf("RawHits[0].Title = %q, esperado %q", fb.RawHits[0].Title, "pgbouncer")
		}
		if len(res.RawHits) != 0 {
			t.Fatalf("consulta %q com páginas devolveu %d RawHits, esperado 0", rrfQuery, len(res.RawHits))
		}
	})
}
