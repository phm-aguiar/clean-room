package storage

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	"github.com/phm-aguiar/ai-memory-go/internal/domain"
)

// esTestEnv abre um banco temporário com a migração real e um Writer real de NewWriter.
func esTestEnv(t *testing.T) (*sql.DB, Writer) {
	t.Helper()
	ctx := context.Background()
	db, err := Open(ctx, t.TempDir())
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	w, err := NewWriter(ctx, db)
	if err != nil {
		t.Fatalf("NewWriter: %v", err)
	}
	t.Cleanup(func() { _ = w.Close() })
	return db, w
}

// esTestScope gera um par de IDs novos e devolve o escopo correspondente.
func esTestScope(t *testing.T) domain.Scope {
	t.Helper()
	wsID, err := domain.NewID()
	if err != nil {
		t.Fatalf("NewID ws: %v", err)
	}
	pjID, err := domain.NewID()
	if err != nil {
		t.Fatalf("NewID pj: %v", err)
	}
	return domain.NewScope(domain.WorkspaceID(wsID), domain.ProjectID(pjID))
}

func esTestCount(t *testing.T, db *sql.DB, table string) int {
	t.Helper()
	var n int
	if err := db.QueryRow(`SELECT COUNT(*) FROM ` + table).Scan(&n); err != nil {
		t.Fatalf("count %s: %v", table, err)
	}
	return n
}

var esTestT0 = time.Date(2026, 10, 12, 15, 30, 0, 123456000, time.UTC)

func TestEnsureScopeCriaWorkspaceEProjeto(t *testing.T) {
	db, w := esTestEnv(t)
	scope := esTestScope(t)

	if err := EnsureScope(context.Background(), w, scope, "default", "scratch", esTestT0); err != nil {
		t.Fatalf("EnsureScope: %v", err)
	}

	var wsName string
	var wsCreated int64
	if err := db.QueryRow(`SELECT name, created_at FROM workspaces WHERE id = ?`, scope.WorkspaceID.Bytes()).
		Scan(&wsName, &wsCreated); err != nil {
		t.Fatalf("workspace row: %v", err)
	}
	if wsName != "default" || wsCreated != esTestT0.UnixMicro() {
		t.Fatalf("workspace = (%q, %d), want (%q, %d)", wsName, wsCreated, "default", esTestT0.UnixMicro())
	}

	var pjName string
	var pjWS, pjRepo []byte
	var pjCreated int64
	if err := db.QueryRow(`SELECT name, workspace_id, created_at, repo_path FROM projects WHERE id = ?`,
		scope.ProjectID.Bytes()).Scan(&pjName, &pjWS, &pjCreated, &pjRepo); err != nil {
		t.Fatalf("project row: %v", err)
	}
	if pjName != "scratch" || pjCreated != esTestT0.UnixMicro() {
		t.Fatalf("project = (%q, %d), want (%q, %d)", pjName, pjCreated, "scratch", esTestT0.UnixMicro())
	}
	if !bytes.Equal(pjWS, scope.WorkspaceID.Bytes()) {
		t.Fatalf("project workspace_id = %x, want %x", pjWS, scope.WorkspaceID.Bytes())
	}
	if pjRepo != nil {
		t.Fatalf("repo_path = %q, want NULL", pjRepo)
	}
}

func TestEnsureScopeIdempotente(t *testing.T) {
	db, w := esTestEnv(t)
	scope := esTestScope(t)
	ctx := context.Background()

	if err := EnsureScope(ctx, w, scope, "ws-a", "proj-a", esTestT0); err != nil {
		t.Fatalf("primeira chamada: %v", err)
	}
	// Segunda chamada com outro horário: não recria nem altera a linha existente.
	later := esTestT0.Add(time.Hour)
	if err := EnsureScope(ctx, w, scope, "ws-a", "proj-a", later); err != nil {
		t.Fatalf("segunda chamada: %v", err)
	}

	if n := esTestCount(t, db, "workspaces"); n != 1 {
		t.Fatalf("workspaces = %d, want 1", n)
	}
	if n := esTestCount(t, db, "projects"); n != 1 {
		t.Fatalf("projects = %d, want 1", n)
	}
	var created int64
	if err := db.QueryRow(`SELECT created_at FROM projects WHERE id = ?`, scope.ProjectID.Bytes()).Scan(&created); err != nil {
		t.Fatalf("read project: %v", err)
	}
	if created != esTestT0.UnixMicro() {
		t.Fatalf("created_at = %d, want primeiro valor %d", created, esTestT0.UnixMicro())
	}
}

func TestEnsureScopeProjetoNovoNoMesmoWorkspace(t *testing.T) {
	db, w := esTestEnv(t)
	ctx := context.Background()

	wsID, err := domain.NewID()
	if err != nil {
		t.Fatalf("NewID: %v", err)
	}
	ws := domain.WorkspaceID(wsID)
	p1, _ := domain.NewID()
	p2, _ := domain.NewID()
	s1 := domain.NewScope(ws, domain.ProjectID(p1))
	s2 := domain.NewScope(ws, domain.ProjectID(p2))

	if err := EnsureScope(ctx, w, s1, "ws-x", "proj-1", esTestT0); err != nil {
		t.Fatalf("escopo 1: %v", err)
	}
	if err := EnsureScope(ctx, w, s2, "ws-x", "proj-2", esTestT0); err != nil {
		t.Fatalf("escopo 2: %v", err)
	}

	if n := esTestCount(t, db, "workspaces"); n != 1 {
		t.Fatalf("workspaces = %d, want 1 (o mesmo workspace não deve ser duplicado)", n)
	}
	if n := esTestCount(t, db, "projects"); n != 2 {
		t.Fatalf("projects = %d, want 2", n)
	}
	var name string
	if err := db.QueryRow(`SELECT name FROM projects WHERE id = ?`, p2.Bytes()).Scan(&name); err != nil {
		t.Fatalf("read proj-2: %v", err)
	}
	if name != "proj-2" {
		t.Fatalf("name = %q, want proj-2", name)
	}
}

func TestEnsureScopePassaPeloWriter(t *testing.T) {
	db, w := esTestEnv(t)
	scope := esTestScope(t)

	if err := w.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	err := EnsureScope(context.Background(), w, scope, "ws", "proj", esTestT0)
	if !errors.Is(err, ErrWriterClosed) {
		t.Fatalf("err = %v, want ErrWriterClosed (a criação deve passar por w.Do)", err)
	}
	if n := esTestCount(t, db, "workspaces"); n != 0 {
		t.Fatalf("workspaces = %d, want 0 após falha", n)
	}
}

func TestEnsureScopeIDsSaoBlob16(t *testing.T) {
	db, w := esTestEnv(t)
	scope := esTestScope(t)

	if err := EnsureScope(context.Background(), w, scope, "ws", "proj", esTestT0); err != nil {
		t.Fatalf("EnsureScope: %v", err)
	}

	var wsType, pjType string
	var wsLen, pjLen, pjWSLen int
	if err := db.QueryRow(`SELECT typeof(id), length(id) FROM workspaces`).Scan(&wsType, &wsLen); err != nil {
		t.Fatalf("workspace id: %v", err)
	}
	if err := db.QueryRow(`SELECT typeof(id), length(id), length(workspace_id) FROM projects`).
		Scan(&pjType, &pjLen, &pjWSLen); err != nil {
		t.Fatalf("project ids: %v", err)
	}
	if wsType != "blob" || wsLen != 16 {
		t.Fatalf("workspace id = (%s, %d), want (blob, 16)", wsType, wsLen)
	}
	if pjType != "blob" || pjLen != 16 || pjWSLen != 16 {
		t.Fatalf("project ids = (%s, %d, %d), want (blob, 16, 16)", pjType, pjLen, pjWSLen)
	}

	var stored []byte
	if err := db.QueryRow(`SELECT id FROM projects`).Scan(&stored); err != nil {
		t.Fatalf("read project id: %v", err)
	}
	if !bytes.Equal(stored, scope.ProjectID.Bytes()) {
		t.Fatalf("project id = %x, want %x", stored, scope.ProjectID.Bytes())
	}
}

func TestEnsureScopeProjetoDeOutroWorkspaceFalha(t *testing.T) {
	db, w := esTestEnv(t)
	ctx := context.Background()

	scopeA := esTestScope(t)
	if err := EnsureScope(ctx, w, scopeA, "ws-a", "proj", esTestT0); err != nil {
		t.Fatalf("escopo A: %v", err)
	}
	// Mesmo ID de projeto, outro workspace: não pode ser aceito em silêncio.
	scopeB := domain.NewScope(
		esTestScope(t).WorkspaceID,
		scopeA.ProjectID,
	)
	if err := EnsureScope(ctx, w, scopeB, "ws-b", "proj", esTestT0); err == nil {
		t.Fatal("esperava erro: projeto já pertence a outro workspace")
	}
	if n := esTestCount(t, db, "workspaces"); n != 1 {
		t.Fatalf("workspaces = %d, want 1 (rollback do ws-b)", n)
	}
}
