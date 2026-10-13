package storage

import (
	"context"
	"database/sql"
	"math"
	"strings"
	"testing"
	"time"

	"github.com/phm-aguiar/ai-memory-go/internal/domain"
)

// srSession cria uma sessão do escopo e devolve o id.
func srSession(t *testing.T, w Writer, scope domain.Scope) domain.SessionID {
	t.Helper()
	id, err := domain.NewID()
	if err != nil {
		t.Fatalf("NewID session: %v", err)
	}
	err = w.Do(context.Background(), func(tx *sql.Tx) error {
		_, err := tx.Exec(
			`INSERT INTO sessions (id, workspace_id, project_id, agent_kind, started_at) VALUES (?, ?, ?, 'claude-code', ?)`,
			id.Bytes(), scope.WorkspaceID.Bytes(), scope.ProjectID.Bytes(), ssT0.UnixMicro(),
		)
		return err
	})
	if err != nil {
		t.Fatalf("insert session: %v", err)
	}
	return domain.SessionID(id)
}

// srSearch chama Search com o corte padrão do teste e falha em erro.
func srSearch(t *testing.T, r Reader, scope domain.Scope, query string, limit int) SearchResult {
	t.Helper()
	res, err := Search(context.Background(), r, scope, query, ssT0.Add(time.Hour), limit)
	if err != nil {
		t.Fatalf("Search(%q, limit %d): %v", query, limit, err)
	}
	return res
}

// srPaths devolve os caminhos das páginas do resultado, na ordem.
func srPaths(pages []ScoredPage) []string {
	out := make([]string, 0, len(pages))
	for _, p := range pages {
		out = append(out, p.Ref.Path.String())
	}
	return out
}

func TestSearchEncontraPaginaERankeiaPorRRF(t *testing.T) {
	w, r, ws := ssEnv(t)
	scope := upScope(t, w, ws, "ws-search", "pj-rrf")

	ssPut(t, w, ssRef(t, scope, "notas/dupla.md"), "Dupla", "corpo com postgres e replicação", ssFM(t, "Postgres"), nil, ssT0)
	ssPut(t, w, ssRef(t, scope, "notas/so-texto.md"), "Texto", "postgres apenas no corpo", ssFM(t), nil, ssT0)
	ssPut(t, w, ssRef(t, scope, "notas/so-entidade.md"), "Entidade", "corpo sem o termo", ssFM(t, "Postgres"), nil, ssT0)

	res := srSearch(t, r, scope, "postgres", 10)
	if len(res.Pages) != 3 {
		t.Fatalf("páginas = %v, esperadas 3", srPaths(res.Pages))
	}
	if res.Pages[0].Ref.Path.String() != "notas/dupla.md" {
		t.Fatalf("primeira = %s, esperada notas/dupla.md (casa FTS e entidade); ordem %v",
			res.Pages[0].Ref.Path.String(), srPaths(res.Pages))
	}
	if len(res.RawHits) != 0 {
		t.Fatalf("RawHits = %d, esperado vazio com páginas casando", len(res.RawHits))
	}
}

func TestSearchAutoridadeAlteraOrdemComRankIgual(t *testing.T) {
	w, r, ws := ssEnv(t)
	scope := upScope(t, w, ws, "ws-search", "pj-auth")

	// A regra casa só FTS (posição 1 do fluxo FTS). A sessão casa só entidade (posição 1 do
	// fluxo de entidades). Com k = 60, as duas recebem o mesmo score RRF.
	upMustUpsert(t, w, UpsertInput{
		Ref:         ssRef(t, scope, "regras/rota.md"),
		Tier:        domain.TierSemantic,
		Kind:        domain.PageKind("rule"),
		Title:       "Regra",
		Body:        "regra: usar postgres para dados",
		Frontmatter: ssFM(t),
		Evidence:    EvidenceSource{Kind: SourceSession, ID: "auth-regra"},
		At:          ssT0,
	})
	upMustUpsert(t, w, UpsertInput{
		Ref:         ssRef(t, scope, "sessoes/rota.md"),
		Tier:        domain.TierEpisodic,
		Kind:        domain.PageKind("session"),
		Title:       "Sessão",
		Body:        "conversa sobre rotas e filas",
		Frontmatter: ssFM(t, "Postgres"),
		Evidence:    EvidenceSource{Kind: SourceSession, ID: "auth-sessao"},
		At:          ssT0,
	})

	res := srSearch(t, r, scope, "postgres", 10)
	if len(res.Pages) != 2 {
		t.Fatalf("páginas = %v, esperadas 2", srPaths(res.Pages))
	}
	if res.Pages[0].Ref.Path.String() != "regras/rota.md" {
		t.Fatalf("ordem = %v, esperada regras/rota.md primeiro (autoridade)", srPaths(res.Pages))
	}

	rule, session := res.Pages[0], res.Pages[1]
	// Fator: rule + semantic = 1.00 + 0.15 + 0.10 = 1.25; session + episodic = 1.00 - 0.15 - 0.08 = 0.77.
	if math.Abs(rule.Authority-1.25) > 1e-9 || math.Abs(session.Authority-0.77) > 1e-9 {
		t.Fatalf("autoridade = (%v, %v), esperadas (1.25, 0.77)", rule.Authority, session.Authority)
	}
	wantRank := -1.0 / 61.0
	if math.Abs(rule.Rank-wantRank*1.25) > 1e-12 || math.Abs(session.Rank-wantRank*0.77) > 1e-12 {
		t.Fatalf("rank = (%v, %v), esperados (%v, %v)", rule.Rank, session.Rank, wantRank*1.25, wantRank*0.77)
	}
}

func TestSearchRespeitaLimite(t *testing.T) {
	w, r, ws := ssEnv(t)
	scope := upScope(t, w, ws, "ws-search", "pj-limite")

	for i := 1; i <= 5; i++ {
		path := "notas/p" + string(rune('0'+i)) + ".md"
		ssPut(t, w, ssRef(t, scope, path), "Página", "postgres em "+path, ssFM(t), nil, ssT0)
	}

	res := srSearch(t, r, scope, "postgres", 2)
	if len(res.Pages) != 2 {
		t.Fatalf("páginas = %d, esperadas 2 (limite)", len(res.Pages))
	}
}

func TestSearchConsultaVaziaDevolveVazio(t *testing.T) {
	w, r, ws := ssEnv(t)
	scope := upScope(t, w, ws, "ws-search", "pj-vazia")
	ssPut(t, w, ssRef(t, scope, "notas/rota.md"), "Rota", "postgres em produção", ssFM(t), nil, ssT0)

	for _, q := range []string{"", "   ", "?!"} {
		res := srSearch(t, r, scope, q, 10)
		if len(res.Pages) != 0 || len(res.RawHits) != 0 {
			t.Fatalf("consulta %q: páginas %v, raw %d; esperado vazio", q, srPaths(res.Pages), len(res.RawHits))
		}
	}
	res := srSearch(t, r, scope, "postgres", 0)
	if len(res.Pages) != 0 || len(res.RawHits) != 0 {
		t.Fatalf("limite 0: páginas %v, raw %d; esperado vazio", srPaths(res.Pages), len(res.RawHits))
	}
}

func TestSearchFallbackRawHitsQuandoNenhumaPaginaCasa(t *testing.T) {
	w, r, ws := ssEnv(t)
	scope := upScope(t, w, ws, "ws-search", "pj-raw")
	outro := upScope(t, w, ws, "ws-search", "pj-raw-outro")

	ssPut(t, w, ssRef(t, scope, "notas/outra.md"), "Outra", "redis apenas", ssFM(t), nil, ssT0)

	sess := srSession(t, w, scope)
	obs := obsNew(t, scope, sess, domain.ObservationPostToolUse, "Migração", "postgres foi migrado para a versão 16", 5)
	if err := obsDo(w, obs); err != nil {
		t.Fatalf("InsertObservation: %v", err)
	}
	// Observação com o mesmo termo em outro projeto do workspace: não deve aparecer.
	sessOutro := srSession(t, w, outro)
	obsOutro := obsNew(t, outro, sessOutro, domain.ObservationPostToolUse, "Outra", "postgres em outro projeto", 5)
	if err := obsDo(w, obsOutro); err != nil {
		t.Fatalf("InsertObservation outro projeto: %v", err)
	}

	res := srSearch(t, r, scope, "postgres", 10)
	if len(res.Pages) != 0 {
		t.Fatalf("páginas = %v, esperadas vazias", srPaths(res.Pages))
	}
	if len(res.RawHits) != 1 {
		t.Fatalf("RawHits = %d, esperado 1", len(res.RawHits))
	}
	hit := res.RawHits[0]
	if hit.ObservationID != obs.ID {
		t.Fatalf("ObservationID = %s, esperado %s", domain.ID(hit.ObservationID).String(), domain.ID(obs.ID).String())
	}
	if hit.SessionID != sess {
		t.Fatalf("SessionID = %s, esperado %s", domain.ID(hit.SessionID).String(), domain.ID(sess).String())
	}
	if hit.Kind != domain.ObservationPostToolUse || hit.Title != "Migração" {
		t.Fatalf("kind/título = %q/%q, esperados post-tool-use/Migração", hit.Kind, hit.Title)
	}
	if !strings.Contains(hit.Snippet, "postgres") {
		t.Fatalf("snippet = %q, esperado trecho com o termo", hit.Snippet)
	}
}

func TestSearchComPaginaCasandoRawHitsVazio(t *testing.T) {
	w, r, ws := ssEnv(t)
	scope := upScope(t, w, ws, "ws-search", "pj-com-pagina")

	ssPut(t, w, ssRef(t, scope, "notas/rota.md"), "Rota", "postgres em produção", ssFM(t), nil, ssT0)
	sess := srSession(t, w, scope)
	obs := obsNew(t, scope, sess, domain.ObservationPostToolUse, "Migração", "postgres migrado", 5)
	if err := obsDo(w, obs); err != nil {
		t.Fatalf("InsertObservation: %v", err)
	}

	res := srSearch(t, r, scope, "postgres", 10)
	if len(res.Pages) != 1 {
		t.Fatalf("páginas = %v, esperada 1", srPaths(res.Pages))
	}
	if len(res.RawHits) != 0 {
		t.Fatalf("RawHits = %d, esperado vazio com página casando", len(res.RawHits))
	}
}

func TestSearchIgnoraVersaoSuperseded(t *testing.T) {
	w, r, ws := ssEnv(t)
	scope := upScope(t, w, ws, "ws-search", "pj-superseded")
	rota := ssRef(t, scope, "notas/rota.md")

	ssPut(t, w, rota, "Rota", "corpo com postgres antigo", ssFM(t), nil, ssT0)
	ssPut(t, w, rota, "Rota", "corpo com redis novo", ssFM(t), nil, ssT0.Add(time.Minute))

	if res := srSearch(t, r, scope, "postgres", 10); len(res.Pages) != 0 {
		t.Fatalf("termo da versão superseded casou: %v", srPaths(res.Pages))
	}
	res := srSearch(t, r, scope, "redis", 10)
	if len(res.Pages) != 1 || res.Pages[0].Ref != rota {
		t.Fatalf("versão corrente não encontrada: %v", srPaths(res.Pages))
	}
}
