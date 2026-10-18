package storage

import (
	"context"
	"database/sql"
	"errors"
	"io"
	"log/slog"
	"testing"

	"github.com/phm-aguiar/ai-memory-go/internal/domain"
)

// rsTestEnv abre o banco real, o Writer de escrita e o Reader somente leitura
// sobre o mesmo diretório. Devolve o pool de escrita (só para contagem), o
// Writer (para preparar escopos com EnsureScope) e o Reader.
func rsTestEnv(t *testing.T) (*sql.DB, Writer, Reader) {
	t.Helper()
	ctx := context.Background()
	dir := t.TempDir()
	db, err := Open(ctx, dir)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	w, err := NewWriter(ctx, db)
	if err != nil {
		t.Fatalf("NewWriter: %v", err)
	}
	t.Cleanup(func() { _ = w.Close() })
	rdb, err := OpenReader(ctx, dir)
	if err != nil {
		t.Fatalf("OpenReader: %v", err)
	}
	t.Cleanup(func() { _ = rdb.Close() })
	return db, w, NewReader(rdb)
}

// rsCounts devolve (workspaces, projects). Se mudar, algo criou escopo.
func rsCounts(t *testing.T, db *sql.DB) (int, int) {
	t.Helper()
	return esTestCount(t, db, "workspaces"), esTestCount(t, db, "projects")
}

func rsMustEnsure(t *testing.T, w Writer, scope domain.Scope, nameWS, nameProj string) {
	t.Helper()
	if err := EnsureScope(context.Background(), w, scope, nameWS, nameProj, esTestT0); err != nil {
		t.Fatalf("EnsureScope(%s,%s): %v", nameWS, nameProj, err)
	}
}

func mustNewID(t *testing.T) domain.ID {
	t.Helper()
	id, err := domain.NewID()
	if err != nil {
		t.Fatalf("NewID: %v", err)
	}
	return id
}

// rsNewProject cria um projeto novo dentro do workspace de base, com nomes dados.
func rsNewProject(t *testing.T, w Writer, base domain.Scope, nameWS, nameProj string) domain.Scope {
	t.Helper()
	s := domain.NewScope(base.WorkspaceID, domain.ProjectID(mustNewID(t)))
	rsMustEnsure(t, w, s, nameWS, nameProj)
	return s
}

// rsPointer cria um ponteiro per_actor sem log na saída do teste.
func rsPointer(t *testing.T) *ActiveProjectPointer {
	t.Helper()
	// MaxEntries explícito: zero viraria 1 e a entrada identidade-só evictaria a de sessão.
	p, err := NewActiveProjectPointer(ActiveProjectConfig{
		Mode:       string(ActiveProjectModePerActor),
		MaxEntries: DefaultActiveProjectMaxEntries,
	},
		slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatalf("NewActiveProjectPointer: %v", err)
	}
	return p
}

var (
	rsActorA = ActorKey{Identity: "user:a", SessionID: "sess-a"}
	rsActorB = ActorKey{Identity: "user:b", SessionID: "sess-b"}
	rsAnon   = ActorKey{}
)

// --- Leitura: nomes explícitos ---

func TestResolveScopeNomeExplicitoEncontra(t *testing.T) {
	db, w, r := rsTestEnv(t)
	scope := esTestScope(t)
	rsMustEnsure(t, w, scope, "wsA", "pjA")

	got, src, err := ResolveScope(context.Background(), r, ScopeRequest{WorkspaceName: "wsA", ProjectName: "pjA"})
	if err != nil {
		t.Fatalf("ResolveScope: %v", err)
	}
	if got != scope || src != ScopeSourceExplicit {
		t.Fatalf("got (%+v, %q), want (%+v, %q)", got, src, scope, ScopeSourceExplicit)
	}
	if ws, pj := rsCounts(t, db); ws != 1 || pj != 1 {
		t.Fatalf("counts = (%d,%d), want (1,1)", ws, pj)
	}
}

func TestResolveScopeNomeInexistenteFalhaFechadoSemCriar(t *testing.T) {
	db, w, r := rsTestEnv(t)
	rsMustEnsure(t, w, esTestScope(t), "wsA", "pjA")
	wsBefore, pjBefore := rsCounts(t, db)

	casos := map[string]struct {
		req  ScopeRequest
		want error
	}{
		"ws e projeto inexistentes": {ScopeRequest{WorkspaceName: "x", ProjectName: "y"}, ErrScopeNotFound},
		"ws existe, projeto não":    {ScopeRequest{WorkspaceName: "wsA", ProjectName: "y"}, ErrScopeNotFound},
		"projeto existe, ws não":    {ScopeRequest{WorkspaceName: "x", ProjectName: "pjA"}, ErrScopeNotFound},
		"só workspace (parcial)":    {ScopeRequest{WorkspaceName: "wsA"}, ErrScopeInvalid},
		"projeto só, sem alvo":      {ScopeRequest{ProjectName: "pjA"}, ErrScopeNotFound},
	}
	for nome, c := range casos {
		_, _, err := ResolveScope(context.Background(), r, c.req)
		if !errors.Is(err, c.want) {
			t.Fatalf("%s: err = %v, want %v", nome, err, c.want)
		}
	}
	if ws, pj := rsCounts(t, db); ws != wsBefore || pj != pjBefore {
		t.Fatalf("counts = (%d,%d), want (%d,%d): leitura criou escopo", ws, pj, wsBefore, pjBefore)
	}
}

func TestResolveScopeParcialEhInvalidaEAindaNaoEncontrado(t *testing.T) {
	_, _, r := rsTestEnv(t)
	_, _, err := ResolveScope(context.Background(), r, ScopeRequest{WorkspaceName: "wsA"})
	if !errors.Is(err, ErrScopeInvalid) {
		t.Fatalf("err = %v, want ErrScopeInvalid", err)
	}
	if !errors.Is(err, ErrScopeNotFound) {
		t.Fatalf("err = %v, want ErrScopeNotFound também encadeado (contrato de aceitação)", err)
	}
}

// --- Leitura sem escopo: degradação e origem ---

func TestResolveScopeDefaultEncontraDefaultScratch(t *testing.T) {
	db, w, r := rsTestEnv(t)
	scope := esTestScope(t)
	rsMustEnsure(t, w, scope, "default", "scratch")

	got, src, err := ResolveScope(context.Background(), r, ScopeRequest{})
	if err != nil {
		t.Fatalf("ResolveScope: %v", err)
	}
	if got != scope || src != ScopeSourceDefault {
		t.Fatalf("got (%+v, %q), want (%+v, %q)", got, src, scope, ScopeSourceDefault)
	}
	if ws, pj := rsCounts(t, db); ws != 1 || pj != 1 {
		t.Fatalf("counts = (%d,%d), want (1,1)", ws, pj)
	}
}

func TestResolveScopeDefaultAusenteFalhaFechadoSemCriar(t *testing.T) {
	db, _, r := rsTestEnv(t)

	_, _, err := ResolveScope(context.Background(), r, ScopeRequest{})
	if !errors.Is(err, ErrScopeNotFound) {
		t.Fatalf("err = %v, want ErrScopeNotFound", err)
	}
	if ws, pj := rsCounts(t, db); ws != 0 || pj != 0 {
		t.Fatalf("counts = (%d,%d), want (0,0): default criado em leitura", ws, pj)
	}
}

func TestResolveScopeSemPonteiroUsaPadrao(t *testing.T) {
	db, w, r := rsTestEnv(t)
	padrao := esTestScope(t)
	rsMustEnsure(t, w, padrao, "default", "scratch")

	// Sem ponteiro (nil) e com ponteiro Unset: as duas leituras degradam para o padrão.
	for nome, req := range map[string]ScopeRequest{
		"ponteiro nil":   {Now: esTestT0},
		"ponteiro unset": {Pointer: rsPointer(t), Actor: rsActorA, Now: esTestT0},
	} {
		res, err := ResolveReadScope(context.Background(), r, req)
		if err != nil {
			t.Fatalf("%s: %v", nome, err)
		}
		if res.Scope != padrao || res.Source != ScopeSourceDefault || res.DefaultGlobal {
			t.Fatalf("%s: got %+v, want padrão com origem default", nome, res)
		}
	}
	if ws, pj := rsCounts(t, db); ws != 1 || pj != 1 {
		t.Fatalf("counts = (%d,%d), want (1,1)", ws, pj)
	}
}

func TestResolveScopeSemEscopoDegradaPorPonteiroDaSessao(t *testing.T) {
	_, w, r := rsTestEnv(t)
	ptr := rsPointer(t)
	s := esTestScope(t)
	rsMustEnsure(t, w, s, "wsA", "pjA")
	ptr.Publish(rsActorA, s, true, true, esTestT0)

	res, err := ResolveReadScope(context.Background(), r, ScopeRequest{Pointer: ptr, Actor: rsActorA, Now: esTestT0})
	if err != nil {
		t.Fatalf("ResolveReadScope: %v", err)
	}
	if res.Scope != s || res.Source != ScopeSourceSession || !res.DefaultGlobal {
		t.Fatalf("got %+v, want (%+v, session, default_global)", res, s)
	}
}

func TestResolveScopeSemEscopoDegradaPorSlotCompartilhado(t *testing.T) {
	_, w, r := rsTestEnv(t)
	ptr := rsPointer(t)
	s := esTestScope(t)
	rsMustEnsure(t, w, s, "wsA", "pjA")
	// Publicação de A em primeiro plano grava o slot compartilhado. Chamador anônimo não tem entrada própria.
	ptr.Publish(rsActorA, s, false, true, esTestT0)

	res, err := ResolveReadScope(context.Background(), r, ScopeRequest{Pointer: ptr, Actor: rsAnon, Now: esTestT0})
	if err != nil {
		t.Fatalf("ResolveReadScope: %v", err)
	}
	if res.Scope != s || res.Source != ScopeSourceSharedSlot {
		t.Fatalf("got %+v, want (%+v, shared_slot)", res, s)
	}
}

func TestResolveScopeSemEscopoDegradaPorSemente(t *testing.T) {
	_, w, r := rsTestEnv(t)
	ptr := rsPointer(t)
	semente := esTestScope(t)
	rsMustEnsure(t, w, semente, "wsA", "pjA")
	ptr.SeedStartup(semente)

	res, err := ResolveReadScope(context.Background(), r, ScopeRequest{Pointer: ptr, Actor: rsActorA, Now: esTestT0})
	if err != nil {
		t.Fatalf("ResolveReadScope: %v", err)
	}
	if res.Scope != semente || res.Source != ScopeSourceStartupSeed {
		t.Fatalf("got %+v, want (%+v, startup_seed)", res, semente)
	}
}

func TestResolveScopeDivergenciaUsaPadraoEFalhaEmVezDeSemente(t *testing.T) {
	_, w, r := rsTestEnv(t)
	ptr := rsPointer(t)
	outro := esTestScope(t)
	rsMustEnsure(t, w, outro, "wsA", "pjA")
	padrao := esTestScope(t)
	rsMustEnsure(t, w, padrao, "default", "scratch")
	ptr.Publish(rsActorA, outro, false, true, esTestT0)
	ptr.SeedStartup(outro)

	// B tem coordenada e nada casa com ela: divergência. A semente nunca responde nesse caso.
	res, err := ResolveReadScope(context.Background(), r, ScopeRequest{Pointer: ptr, Actor: rsActorB, Now: esTestT0})
	if err != nil {
		t.Fatalf("ResolveReadScope: %v", err)
	}
	if res.Scope != padrao || res.Source != ScopeSourceDefaultAfterMismatch {
		t.Fatalf("got %+v, want (%+v, default_after_mismatch)", res, padrao)
	}
}

func TestResolveScopeSemEscopoPadraoAusenteFalhaAposDivergencia(t *testing.T) {
	_, w, r := rsTestEnv(t)
	ptr := rsPointer(t)
	outro := esTestScope(t)
	rsMustEnsure(t, w, outro, "wsA", "pjA")
	ptr.Publish(rsActorA, outro, false, true, esTestT0)

	_, err := ResolveReadScope(context.Background(), r, ScopeRequest{Pointer: ptr, Actor: rsActorB, Now: esTestT0})
	if !errors.Is(err, ErrScopeNotFound) {
		t.Fatalf("err = %v, want ErrScopeNotFound (padrão ausente, sem degradar para o ponteiro de outro ator)", err)
	}
}

// --- Precedência e isolamento com nome explícito e project sozinho ---

func TestResolveScopeNomeExplicitoGanhaDoPonteiro(t *testing.T) {
	_, w, r := rsTestEnv(t)
	ptr := rsPointer(t)
	ponteiro := esTestScope(t)
	rsMustEnsure(t, w, ponteiro, "wsA", "pjA")
	alvo := esTestScope(t)
	rsMustEnsure(t, w, alvo, "wsB", "pjB")
	ptr.Publish(rsActorA, ponteiro, false, true, esTestT0)

	got, src, err := ResolveScope(context.Background(), r, ScopeRequest{WorkspaceName: "wsB", ProjectName: "pjB", Pointer: ptr, Actor: rsActorA, Now: esTestT0})
	if err != nil {
		t.Fatalf("ResolveScope: %v", err)
	}
	if got != alvo || src != ScopeSourceExplicit {
		t.Fatalf("got (%+v, %q), want (%+v, explicit)", got, src, alvo)
	}
}

func TestResolveScopeNomeExplicitoInexistenteNaoCaiNoPonteiro(t *testing.T) {
	_, w, r := rsTestEnv(t)
	ptr := rsPointer(t)
	ponteiro := esTestScope(t)
	rsMustEnsure(t, w, ponteiro, "wsA", "pjA")
	ptr.Publish(rsActorA, ponteiro, false, true, esTestT0)

	_, _, err := ResolveScope(context.Background(), r, ScopeRequest{WorkspaceName: "wsX", ProjectName: "pjX", Pointer: ptr, Actor: rsActorA, Now: esTestT0})
	if !errors.Is(err, ErrScopeNotFound) {
		t.Fatalf("err = %v, want ErrScopeNotFound (falha fechada, sem cair no ponteiro)", err)
	}
}

func TestResolveScopeProjetoSozinhoPrefereWorkspaceAtivo(t *testing.T) {
	_, w, r := rsTestEnv(t)
	ptr := rsPointer(t)
	ativo := esTestScope(t)
	rsMustEnsure(t, w, ativo, "wsA", "pjA")
	noAtivo := rsNewProject(t, w, ativo, "wsA", "pjX")
	noPadrao := esTestScope(t)
	rsMustEnsure(t, w, noPadrao, "default", "pjX")
	ptr.Publish(rsActorA, ativo, false, true, esTestT0)

	got, src, err := ResolveScope(context.Background(), r, ScopeRequest{ProjectName: "pjX", Pointer: ptr, Actor: rsActorA, Now: esTestT0})
	if err != nil {
		t.Fatalf("ResolveScope: %v", err)
	}
	if got != noAtivo || src != ScopeSourceExplicit {
		t.Fatalf("got (%+v, %q), want projeto do workspace ativo (%+v)", got, src, noAtivo)
	}
}

func TestResolveScopeProjetoSozinhoCaiNoWorkspacePadrao(t *testing.T) {
	_, w, r := rsTestEnv(t)
	ptr := rsPointer(t)
	ativo := esTestScope(t)
	rsMustEnsure(t, w, ativo, "wsA", "pjA")
	noPadrao := esTestScope(t)
	rsMustEnsure(t, w, noPadrao, "default", "pjX")
	ptr.Publish(rsActorA, ativo, false, true, esTestT0)

	got, src, err := ResolveScope(context.Background(), r, ScopeRequest{ProjectName: "pjX", Pointer: ptr, Actor: rsActorA, Now: esTestT0})
	if err != nil {
		t.Fatalf("ResolveScope: %v", err)
	}
	if got != noPadrao || src != ScopeSourceExplicit {
		t.Fatalf("got (%+v, %q), want projeto do workspace padrão (%+v)", got, src, noPadrao)
	}
}

func TestResolveScopeProjetoSozinhoIsolaWorkspaces(t *testing.T) {
	db, w, r := rsTestEnv(t)
	ptr := rsPointer(t)
	ativo := esTestScope(t)
	rsMustEnsure(t, w, ativo, "wsA", "pjA")
	// pjB existe só em outro workspace, que não é o ativo nem o padrão: não pode ser resolvido.
	rsMustEnsure(t, w, esTestScope(t), "wsB", "pjB")
	ptr.Publish(rsActorA, ativo, false, true, esTestT0)
	wsBefore, pjBefore := rsCounts(t, db)

	_, _, err := ResolveScope(context.Background(), r, ScopeRequest{ProjectName: "pjB", Pointer: ptr, Actor: rsActorA, Now: esTestT0})
	if !errors.Is(err, ErrScopeNotFound) {
		t.Fatalf("err = %v, want ErrScopeNotFound", err)
	}
	if ws, pj := rsCounts(t, db); ws != wsBefore || pj != pjBefore {
		t.Fatalf("counts = (%d,%d), want (%d,%d): leitura criou escopo", ws, pj, wsBefore, pjBefore)
	}
}

func TestResolveScopeBarraNomeExatoVenceSobrePar(t *testing.T) {
	_, w, r := rsTestEnv(t)
	rsMustEnsure(t, w, esTestScope(t), "wsX", "pjY")
	literal := esTestScope(t)
	rsMustEnsure(t, w, literal, "default", "wsX/pjY")

	// O nome exato existe no workspace padrão: vence, mesmo com o par wsX/pjY existindo.
	got, src, err := ResolveScope(context.Background(), r, ScopeRequest{ProjectName: "wsX/pjY"})
	if err != nil {
		t.Fatalf("nome exato: %v", err)
	}
	if got != literal || src != ScopeSourceExplicit {
		t.Fatalf("nome exato: got (%+v, %q), want (%+v, explicit)", got, src, literal)
	}
}

func TestResolveScopeBarraViraParSemNomeExato(t *testing.T) {
	_, w, r := rsTestEnv(t)
	par := esTestScope(t)
	rsMustEnsure(t, w, par, "wsX", "pjY")

	// Sem nome exato em nenhum workspace consultado, o par wsX/pjY vale.
	got, src, err := ResolveScope(context.Background(), r, ScopeRequest{ProjectName: "wsX/pjY"})
	if err != nil {
		t.Fatalf("par sem nome exato: %v", err)
	}
	if got != par || src != ScopeSourceExplicit {
		t.Fatalf("par sem nome exato: got (%+v, %q), want (%+v, explicit)", got, src, par)
	}

	// Duas barras não formam par; nada existe com esse nome.
	if _, _, err := ResolveScope(context.Background(), r, ScopeRequest{ProjectName: "a/b/c"}); !errors.Is(err, ErrScopeNotFound) {
		t.Fatalf("a/b/c: err = %v, want ErrScopeNotFound", err)
	}
	// Par com lado vazio não forma rótulo.
	if _, _, err := ResolveScope(context.Background(), r, ScopeRequest{ProjectName: "/pjY"}); !errors.Is(err, ErrScopeNotFound) {
		t.Fatalf("/pjY: err = %v, want ErrScopeNotFound", err)
	}
}

func TestResolveScopeRepoPathSaiDaLeitura(t *testing.T) {
	db, w, r := rsTestEnv(t)
	rsMustEnsure(t, w, esTestScope(t), "wsA", "pjA")
	wsBefore, pjBefore := rsCounts(t, db)

	_, _, err := ResolveScope(context.Background(), r, ScopeRequest{RepoPath: "/repos/alfa"})
	if !errors.Is(err, ErrScopeNotFound) {
		t.Fatalf("repo_path: err = %v, want ErrScopeNotFound (resolução por caminho é do hook, T027)", err)
	}
	_, _, err = ResolveScope(context.Background(), r, ScopeRequest{RepoPath: "/repos/alfa", WorkspaceName: "wsA", ProjectName: "pjA"})
	if !errors.Is(err, ErrScopeInvalid) {
		t.Fatalf("repo_path com nome: err = %v, want ErrScopeInvalid", err)
	}
	if ws, pj := rsCounts(t, db); ws != wsBefore || pj != pjBefore {
		t.Fatalf("counts = (%d,%d), want (%d,%d)", ws, pj, wsBefore, pjBefore)
	}
}

// --- Lista scopes ---

func TestResolveScopeListaLimiteVinteCinco(t *testing.T) {
	_, w, r := rsTestEnv(t)
	rsMustEnsure(t, w, esTestScope(t), "wsA", "pjA")

	muitos := make([]ScopeRef, MaxScopeList+1)
	for i := range muitos {
		muitos[i] = ScopeRef{WorkspaceName: "wsA", ProjectName: "pjA"}
	}
	if _, err := ResolveScopeList(context.Background(), r, muitos); !errors.Is(err, ErrScopeInvalid) {
		t.Fatalf("26 itens: err = %v, want ErrScopeInvalid", err)
	}

	// 25 entradas repetidas estão no limite: passam, e a duplicata é removida.
	no := muitos[:MaxScopeList]
	got, err := ResolveScopeList(context.Background(), r, no)
	if err != nil {
		t.Fatalf("25 itens: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("25 itens iguais: len = %d, want 1", len(got))
	}
}

func TestResolveScopeListaSemDuplicata(t *testing.T) {
	_, w, r := rsTestEnv(t)
	a := esTestScope(t)
	rsMustEnsure(t, w, a, "wsA", "pjA")
	b := esTestScope(t)
	rsMustEnsure(t, w, b, "wsB", "pjB")

	refs := []ScopeRef{
		{WorkspaceName: "wsA", ProjectName: "pjA"},
		{WorkspaceName: "wsB", ProjectName: "pjB"},
		{WorkspaceName: "wsA", ProjectName: "pjA"},
	}
	got, err := ResolveScopeList(context.Background(), r, refs)
	if err != nil {
		t.Fatalf("ResolveScopeList: %v", err)
	}
	if len(got) != 2 || got[0] != a || got[1] != b {
		t.Fatalf("got %+v, want [%+v %+v] na ordem da primeira ocorrência", got, a, b)
	}
}

func TestResolveScopeListaFalhaFechadoNoTodo(t *testing.T) {
	_, w, r := rsTestEnv(t)
	rsMustEnsure(t, w, esTestScope(t), "wsA", "pjA")

	refs := []ScopeRef{
		{WorkspaceName: "wsA", ProjectName: "pjA"},
		{WorkspaceName: "wsA", ProjectName: "inexistente"},
	}
	got, err := ResolveScopeList(context.Background(), r, refs)
	if !errors.Is(err, ErrScopeNotFound) {
		t.Fatalf("err = %v, want ErrScopeNotFound", err)
	}
	if got != nil {
		t.Fatalf("got %+v com erro; want nil", got)
	}
}

func TestResolveScopeListaNomeVazioEhInvalido(t *testing.T) {
	_, _, r := rsTestEnv(t)
	for _, ref := range []ScopeRef{{WorkspaceName: "", ProjectName: "p"}, {WorkspaceName: "w", ProjectName: ""}} {
		if _, err := ResolveScopeList(context.Background(), r, []ScopeRef{ref}); !errors.Is(err, ErrScopeInvalid) {
			t.Fatalf("%+v: err = %v, want ErrScopeInvalid", ref, err)
		}
	}
}

// --- _global ---

func TestResolveGlobalScopeExpoeDefaultGlobal(t *testing.T) {
	_, w, r := rsTestEnv(t)
	global := esTestScope(t)
	rsMustEnsure(t, w, global, "default", "_global")

	got, err := ResolveGlobalScope(context.Background(), r)
	if err != nil {
		t.Fatalf("ResolveGlobalScope: %v", err)
	}
	if got != global {
		t.Fatalf("got %+v, want %+v", got, global)
	}

	// Quando o projeto lido já é o _global, a leitura devolve o mesmo escopo: quem une não deve duplicar.
	lido, _, err := ResolveScope(context.Background(), r, ScopeRequest{WorkspaceName: "default", ProjectName: "_global"})
	if err != nil {
		t.Fatalf("ResolveScope _global: %v", err)
	}
	if lido != got {
		t.Fatalf("projeto _global lido %+v difere do global %+v", lido, got)
	}
}

func TestResolveGlobalScopeAusenteFalhaFechadoSemCriar(t *testing.T) {
	db, _, r := rsTestEnv(t)
	if _, err := ResolveGlobalScope(context.Background(), r); !errors.Is(err, ErrScopeNotFound) {
		t.Fatalf("err = %v, want ErrScopeNotFound", err)
	}
	if ws, pj := rsCounts(t, db); ws != 0 || pj != 0 {
		t.Fatalf("counts = (%d,%d), want (0,0)", ws, pj)
	}
}

// --- Escrita ---

func TestEscritaSemEscopoSemPonteiroRecusada(t *testing.T) {
	db, w, r := rsTestEnv(t)
	rsMustEnsure(t, w, esTestScope(t), "default", "scratch")
	semente := esTestScope(t)
	rsMustEnsure(t, w, semente, "wsA", "pjA")
	seedPtr := rsPointer(t)
	seedPtr.SeedStartup(semente)
	wsBefore, pjBefore := rsCounts(t, db)

	casos := map[string]ScopeRequest{
		"nil":                   {Now: esTestT0},
		"ponteiro unset":        {Pointer: rsPointer(t), Actor: rsActorA, Now: esTestT0},
		"só semente de startup": {Pointer: seedPtr, Actor: rsActorA, Now: esTestT0},
	}
	for nome, req := range casos {
		_, err := ResolveWriteScope(context.Background(), r, req)
		if !errors.Is(err, ErrScopeRequired) {
			t.Fatalf("%s: err = %v, want ErrScopeRequired", nome, err)
		}
	}
	if ws, pj := rsCounts(t, db); ws != wsBefore || pj != pjBefore {
		t.Fatalf("counts = (%d,%d), want (%d,%d): escrita criou escopo", ws, pj, wsBefore, pjBefore)
	}
}

func TestEscritaComPonteiroDivergenteRecusadaSemAmbiguidade(t *testing.T) {
	_, w, r := rsTestEnv(t)
	ptr := rsPointer(t)
	outro := esTestScope(t)
	rsMustEnsure(t, w, outro, "wsA", "pjA")
	ptr.Publish(rsActorA, outro, false, true, esTestT0)

	_, err := ResolveWriteScope(context.Background(), r, ScopeRequest{Pointer: ptr, Actor: rsActorB, Now: esTestT0})
	if !errors.Is(err, ErrScopeMismatch) {
		t.Fatalf("err = %v, want ErrScopeMismatch", err)
	}
	if errors.Is(err, ErrScopeAmbiguous) || errors.Is(err, ErrScopeNotFound) {
		t.Fatalf("err = %v: erro de divergência não pode ser ambíguo nem não encontrado", err)
	}
}

func TestEscritaSemEscopoUsaPonteiroResolvido(t *testing.T) {
	_, w, r := rsTestEnv(t)
	ptr := rsPointer(t)
	s := esTestScope(t)
	rsMustEnsure(t, w, s, "wsA", "pjA")
	ptr.Publish(rsActorA, s, false, true, esTestT0)

	tgt, err := ResolveWriteScope(context.Background(), r, ScopeRequest{Pointer: ptr, Actor: rsActorA, Now: esTestT0})
	if err != nil {
		t.Fatalf("ResolveWriteScope: %v", err)
	}
	if !tgt.Exists || tgt.Scope != s || tgt.Source != ScopeSourceSession {
		t.Fatalf("got %+v, want (%+v existente, session)", tgt, s)
	}

	tgt, err = ResolveWriteScope(context.Background(), r, ScopeRequest{Pointer: ptr, Actor: rsAnon, Now: esTestT0})
	if err != nil {
		t.Fatalf("slot compartilhado: %v", err)
	}
	if !tgt.Exists || tgt.Scope != s || tgt.Source != ScopeSourceSharedSlot {
		t.Fatalf("slot compartilhado: got %+v, want (%+v, shared_slot)", tgt, s)
	}
}

func TestEscritaProjetoSozinhoHerdaWorkspaceDoPonteiro(t *testing.T) {
	db, w, r := rsTestEnv(t)
	ptr := rsPointer(t)
	ativo := esTestScope(t)
	rsMustEnsure(t, w, ativo, "wsA", "pjA")
	existente := rsNewProject(t, w, ativo, "wsA", "pjX")
	rsMustEnsure(t, w, esTestScope(t), "default", "pjZ")
	ptr.Publish(rsActorA, ativo, false, true, esTestT0)
	base := ScopeRequest{Pointer: ptr, Actor: rsActorA, Now: esTestT0}

	req := base
	req.ProjectName = "pjX"
	tgt, err := ResolveWriteScope(context.Background(), r, req)
	if err != nil {
		t.Fatalf("pjX existente: %v", err)
	}
	if !tgt.Exists || tgt.Scope != existente {
		t.Fatalf("pjX existente: got %+v, want %+v", tgt, existente)
	}

	wsBefore, pjBefore := rsCounts(t, db)
	req = base
	req.ProjectName = "novo"
	req.Create = true
	tgt, err = ResolveWriteScope(context.Background(), r, req)
	if err != nil {
		t.Fatalf("novo com Create: %v", err)
	}
	if tgt.Exists || tgt.WorkspaceName != "wsA" || tgt.ProjectName != "novo" {
		t.Fatalf("novo: got %+v, want criação em wsA/novo (herdado do ponteiro, não do padrão)", tgt)
	}
	if ws, pj := rsCounts(t, db); ws != wsBefore || pj != pjBefore {
		t.Fatalf("counts = (%d,%d), want (%d,%d): resolvedor criou escopo", ws, pj, wsBefore, pjBefore)
	}

	req.Create = false
	if _, err := ResolveWriteScope(context.Background(), r, req); !errors.Is(err, ErrScopeNotFound) {
		t.Fatalf("novo sem Create: err = %v, want ErrScopeNotFound", err)
	}
}

func TestEscritaProjetoSozinhoSemPonteiroRecusado(t *testing.T) {
	_, _, r := rsTestEnv(t)
	_, err := ResolveWriteScope(context.Background(), r, ScopeRequest{ProjectName: "pjX", Create: true, Now: esTestT0})
	if !errors.Is(err, ErrScopeRequired) {
		t.Fatalf("err = %v, want ErrScopeRequired", err)
	}
}

func TestEscritaWorkspaceSemProjetoEhInvalida(t *testing.T) {
	_, _, r := rsTestEnv(t)
	_, err := ResolveWriteScope(context.Background(), r, ScopeRequest{WorkspaceName: "wsA", Create: true})
	if !errors.Is(err, ErrScopeInvalid) {
		t.Fatalf("err = %v, want ErrScopeInvalid", err)
	}
}

func TestEscritaParExplicitoInexistenteFalhaFechado(t *testing.T) {
	db, w, r := rsTestEnv(t)
	rsMustEnsure(t, w, esTestScope(t), "wsA", "pjA")
	wsBefore, pjBefore := rsCounts(t, db)

	// Sem Create, par inexistente falha fechado.
	if _, err := ResolveWriteScope(context.Background(), r, ScopeRequest{WorkspaceName: "wsX", ProjectName: "pjX"}); !errors.Is(err, ErrScopeNotFound) {
		t.Fatalf("sem Create: err = %v, want ErrScopeNotFound", err)
	}
	// Com Create (caminho de escrita explícita), o alvo é devolvido sem criar nada aqui.
	tgt, err := ResolveWriteScope(context.Background(), r, ScopeRequest{WorkspaceName: "wsX", ProjectName: "pjX", Create: true})
	if err != nil {
		t.Fatalf("com Create: %v", err)
	}
	if tgt.Exists || tgt.WorkspaceName != "wsX" || tgt.ProjectName != "pjX" {
		t.Fatalf("com Create: got %+v, want alvo wsX/pjX a criar", tgt)
	}
	if ws, pj := rsCounts(t, db); ws != wsBefore || pj != pjBefore {
		t.Fatalf("counts = (%d,%d), want (%d,%d): resolvedor criou escopo", ws, pj, wsBefore, pjBefore)
	}
}

func TestEscritaRotuloWorkspaceProjetoNuncaCria(t *testing.T) {
	_, w, r := rsTestEnv(t)
	ptr := rsPointer(t)
	ativo := esTestScope(t)
	rsMustEnsure(t, w, ativo, "wsA", "pjA")
	ptr.Publish(rsActorA, ativo, false, true, esTestT0)

	_, err := ResolveWriteScope(context.Background(), r, ScopeRequest{ProjectName: "wsB/pjB", Pointer: ptr, Actor: rsActorA, Now: esTestT0, Create: true})
	if !errors.Is(err, ErrScopeNotFound) {
		t.Fatalf("err = %v, want ErrScopeNotFound (rótulo nunca cria)", err)
	}
}

func TestEscritaExplicitoGanhaDoPonteiro(t *testing.T) {
	_, w, r := rsTestEnv(t)
	ptr := rsPointer(t)
	ponteiro := esTestScope(t)
	rsMustEnsure(t, w, ponteiro, "wsA", "pjA")
	alvo := esTestScope(t)
	rsMustEnsure(t, w, alvo, "wsB", "pjB")
	ptr.Publish(rsActorA, ponteiro, false, true, esTestT0)

	tgt, err := ResolveWriteScope(context.Background(), r, ScopeRequest{WorkspaceName: "wsB", ProjectName: "pjB", Pointer: ptr, Actor: rsActorA, Now: esTestT0})
	if err != nil {
		t.Fatalf("ResolveWriteScope: %v", err)
	}
	if !tgt.Exists || tgt.Scope != alvo || tgt.Source != ScopeSourceExplicit {
		t.Fatalf("got %+v, want (%+v, explicit)", tgt, alvo)
	}
}

// --- Origens ---

func TestScopeSourceFallbackEInferida(t *testing.T) {
	casos := []struct {
		src      ScopeSource
		fallback bool
		inferred bool
	}{
		{ScopeSourceExplicit, false, false},
		{ScopeSourceSession, false, false},
		{ScopeSourceSharedSlot, false, true},
		{ScopeSourceStartupSeed, true, true},
		{ScopeSourceDefault, false, true},
		{ScopeSourceDefaultAfterMismatch, true, true},
	}
	for _, c := range casos {
		if c.src.IsFallback() != c.fallback || c.src.IsInferred() != c.inferred {
			t.Fatalf("%q: fallback=%v inferred=%v, want %v %v", c.src, c.src.IsFallback(), c.src.IsInferred(), c.fallback, c.inferred)
		}
	}
}

func TestScopeSourceValoresDoDoc(t *testing.T) {
	casos := map[ScopeSource]string{
		ScopeSourceExplicit:             "explicit",
		ScopeSourceSession:              "session",
		ScopeSourceSharedSlot:           "shared_slot",
		ScopeSourceStartupSeed:          "startup_seed",
		ScopeSourceDefault:              "default",
		ScopeSourceDefaultAfterMismatch: "default_after_mismatch",
	}
	for src, want := range casos {
		if string(src) != want {
			t.Fatalf("ScopeSource = %q, want %q", src, want)
		}
	}
}
