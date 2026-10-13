package storage

import (
	"context"
	"database/sql"
	"errors"
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

// rsSetRepoPath grava repo_path de um projeto pelo Writer (EnsureScope deixa NULL).
func rsSetRepoPath(t *testing.T, w Writer, scope domain.Scope, repo string) {
	t.Helper()
	if err := w.Do(context.Background(), func(tx *sql.Tx) error {
		_, err := tx.Exec(`UPDATE projects SET repo_path = ? WHERE id = ?`, repo, scope.ProjectID.Bytes())
		return err
	}); err != nil {
		t.Fatalf("set repo_path: %v", err)
	}
}

func rsMustEnsure(t *testing.T, w Writer, scope domain.Scope, nameWS, nameProj string) {
	t.Helper()
	if err := EnsureScope(context.Background(), w, scope, nameWS, nameProj, esTestT0); err != nil {
		t.Fatalf("EnsureScope(%s,%s): %v", nameWS, nameProj, err)
	}
}

func TestResolveScopeNomeExplicitoEncontra(t *testing.T) {
	db, w, r := rsTestEnv(t)
	scope := esTestScope(t)
	rsMustEnsure(t, w, scope, "wsA", "pjA")

	got, src, err := ResolveScope(context.Background(), r, ScopeRequest{WorkspaceName: "wsA", ProjectName: "pjA"})
	if err != nil {
		t.Fatalf("ResolveScope: %v", err)
	}
	if got != scope {
		t.Fatalf("scope = %+v, want %+v", got, scope)
	}
	if src != ScopeSourceExplicit {
		t.Fatalf("source = %q, want %q", src, ScopeSourceExplicit)
	}
	if ws, pj := rsCounts(t, db); ws != 1 || pj != 1 {
		t.Fatalf("counts = (%d,%d), want (1,1)", ws, pj)
	}
}

func TestResolveScopeNomeInexistenteFalhaFechadoSemCriar(t *testing.T) {
	db, w, r := rsTestEnv(t)
	rsMustEnsure(t, w, esTestScope(t), "wsA", "pjA")
	wsBefore, pjBefore := rsCounts(t, db)

	casos := map[string]ScopeRequest{
		"ws e projeto inexistentes": {WorkspaceName: "x", ProjectName: "y"},
		"ws existe, projeto não":    {WorkspaceName: "wsA", ProjectName: "y"},
		"projeto existe, ws não":    {WorkspaceName: "x", ProjectName: "pjA"},
		"só workspace (parcial)":    {WorkspaceName: "wsA"},
		"só projeto (parcial)":      {ProjectName: "pjA"},
	}
	for nome, req := range casos {
		_, _, err := ResolveScope(context.Background(), r, req)
		if !errors.Is(err, ErrScopeNotFound) {
			t.Fatalf("%s: err = %v, want ErrScopeNotFound", nome, err)
		}
	}

	if ws, pj := rsCounts(t, db); ws != wsBefore || pj != pjBefore {
		t.Fatalf("counts = (%d,%d), want (%d,%d): leitura criou escopo", ws, pj, wsBefore, pjBefore)
	}
}

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

func TestResolveScopeRepoPathUnicoResolve(t *testing.T) {
	db, w, r := rsTestEnv(t)
	scope := esTestScope(t)
	rsMustEnsure(t, w, scope, "wsA", "pjA")
	rsSetRepoPath(t, w, scope, "/repos/alfa")

	got, src, err := ResolveScope(context.Background(), r, ScopeRequest{RepoPath: "/repos/alfa"})
	if err != nil {
		t.Fatalf("ResolveScope: %v", err)
	}
	if got != scope || src != ScopeSourceRepoPath {
		t.Fatalf("got (%+v, %q), want (%+v, %q)", got, src, scope, ScopeSourceRepoPath)
	}

	wsBefore, pjBefore := rsCounts(t, db)
	_, _, err = ResolveScope(context.Background(), r, ScopeRequest{RepoPath: "/repos/outro"})
	if !errors.Is(err, ErrScopeNotFound) {
		t.Fatalf("repo_path sem casamento: err = %v, want ErrScopeNotFound", err)
	}
	if ws, pj := rsCounts(t, db); ws != wsBefore || pj != pjBefore {
		t.Fatalf("counts = (%d,%d), want (%d,%d): leitura criou escopo", ws, pj, wsBefore, pjBefore)
	}
}

func TestResolveScopeRepoPathCompartilhadoEntreWorkspacesAmbiguo(t *testing.T) {
	db, w, r := rsTestEnv(t)
	a := esTestScope(t)
	b := esTestScope(t)
	rsMustEnsure(t, w, a, "wsA", "pjA")
	rsMustEnsure(t, w, b, "wsB", "pjB")
	rsSetRepoPath(t, w, a, "/repos/compartilhado")
	rsSetRepoPath(t, w, b, "/repos/compartilhado")
	wsBefore, pjBefore := rsCounts(t, db)

	_, _, err := ResolveScope(context.Background(), r, ScopeRequest{RepoPath: "/repos/compartilhado"})
	if !errors.Is(err, ErrScopeAmbiguous) {
		t.Fatalf("err = %v, want ErrScopeAmbiguous", err)
	}
	if ws, pj := rsCounts(t, db); ws != wsBefore || pj != pjBefore {
		t.Fatalf("counts = (%d,%d), want (%d,%d)", ws, pj, wsBefore, pjBefore)
	}
}

func TestResolveScopeRepoPathCompartilhadoNoMesmoWorkspaceAmbiguo(t *testing.T) {
	_, w, r := rsTestEnv(t)
	ws, err := domain.NewID()
	if err != nil {
		t.Fatalf("NewID: %v", err)
	}
	wsID := domain.WorkspaceID(ws)
	p1 := domain.NewScope(wsID, domain.ProjectID(mustNewID(t)))
	p2 := domain.NewScope(wsID, domain.ProjectID(mustNewID(t)))
	rsMustEnsure(t, w, p1, "wsA", "pjA")
	rsMustEnsure(t, w, p2, "wsA", "pjB")
	rsSetRepoPath(t, w, p1, "/repos/mesmo")
	rsSetRepoPath(t, w, p2, "/repos/mesmo")

	_, _, err = ResolveScope(context.Background(), r, ScopeRequest{RepoPath: "/repos/mesmo"})
	if !errors.Is(err, ErrScopeAmbiguous) {
		t.Fatalf("err = %v, want ErrScopeAmbiguous", err)
	}
}

func TestResolveScopeMisturaRepoPathComNomeFalhaFechado(t *testing.T) {
	db, w, r := rsTestEnv(t)
	scope := esTestScope(t)
	rsMustEnsure(t, w, scope, "wsA", "pjA")
	rsSetRepoPath(t, w, scope, "/repos/alfa")
	wsBefore, pjBefore := rsCounts(t, db)

	_, _, err := ResolveScope(context.Background(), r, ScopeRequest{WorkspaceName: "wsA", ProjectName: "pjA", RepoPath: "/repos/alfa"})
	if !errors.Is(err, ErrScopeNotFound) {
		t.Fatalf("err = %v, want ErrScopeNotFound", err)
	}
	if ws, pj := rsCounts(t, db); ws != wsBefore || pj != pjBefore {
		t.Fatalf("counts = (%d,%d), want (%d,%d)", ws, pj, wsBefore, pjBefore)
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
