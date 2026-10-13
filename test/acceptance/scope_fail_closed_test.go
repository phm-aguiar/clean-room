package acceptance

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	"github.com/phm-aguiar/ai-memory-go/internal/domain"
	"github.com/phm-aguiar/ai-memory-go/internal/storage"
)

// Nomes e instante do cenário 5 de specs/features/001-nucleo/acceptance/nucleo.feature
// ("Leitura sem escopo resolvível falha fechada"). Os nomes são fixos e não têm segredo.
const (
	sfWorkspace = "wsExistente"
	sfProjeto   = "pjExistente"
	sfRepoPath  = "/repo/inexistente"
)

var sfInstante = time.Date(2026, time.January, 2, 3, 4, 5, 0, time.UTC)

// sfContar devolve COUNT(*) de uma tabela pelo pool informado.
func sfContar(t *testing.T, db *sql.DB, tabela string) int {
	t.Helper()
	var n int
	if err := db.QueryRow(`SELECT COUNT(*) FROM ` + tabela).Scan(&n); err != nil {
		t.Fatalf("contar %s: %v", tabela, err)
	}
	return n
}

func TestScopeLeituraSemEscopoResolvelFalhaFechada(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()

	db, err := storage.Open(ctx, dir)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	writer, err := storage.NewWriter(ctx, db)
	if err != nil {
		t.Fatalf("NewWriter: %v", err)
	}
	t.Cleanup(func() { _ = writer.Close() })

	readDB, err := storage.OpenReader(ctx, dir)
	if err != nil {
		t.Fatalf("OpenReader: %v", err)
	}
	t.Cleanup(func() { _ = readDB.Close() })
	reader := storage.NewReader(readDB)

	// Dado: um banco com um workspace e um projeto existentes, e a contagem inicial.
	wsID, err := domain.NewID()
	if err != nil {
		t.Fatalf("NewID workspace: %v", err)
	}
	pjID, err := domain.NewID()
	if err != nil {
		t.Fatalf("NewID projeto: %v", err)
	}
	escopo := domain.NewScope(domain.WorkspaceID(wsID), domain.ProjectID(pjID))
	if err := storage.EnsureScope(ctx, writer, escopo, sfWorkspace, sfProjeto, sfInstante); err != nil {
		t.Fatalf("EnsureScope: %v", err)
	}
	wsInicial := sfContar(t, readDB, "workspaces")
	pjInicial := sfContar(t, readDB, "projects")
	if wsInicial != 1 || pjInicial != 1 {
		t.Fatalf("contagem inicial = (%d workspaces, %d projetos), want (1, 1)", wsInicial, pjInicial)
	}

	// Quando e Então: cada requisição que não resolve falha fechada.
	casos := []struct {
		nome string
		req  storage.ScopeRequest
	}{
		{
			nome: "workspace inexistente",
			req:  storage.ScopeRequest{WorkspaceName: "wsInexistente", ProjectName: sfProjeto},
		},
		{
			nome: "só workspace, sem projeto",
			req:  storage.ScopeRequest{WorkspaceName: sfWorkspace},
		},
		{
			nome: "requisição vazia sem default",
			req:  storage.ScopeRequest{},
		},
		{
			nome: "repo_path inexistente",
			req:  storage.ScopeRequest{RepoPath: sfRepoPath},
		},
	}

	for _, c := range casos {
		t.Run(c.nome, func(t *testing.T) {
			// Quando: o chamador pede a leitura com a requisição que não resolve.
			got, src, err := storage.ResolveScope(ctx, reader, c.req)

			// Então (a): erro de escopo encadeando ErrScopeNotFound, sem escopo inventado.
			if err == nil {
				t.Fatalf("ResolveScope(%+v) sem erro; escopo %+v (fonte %q)", c.req, got, src)
			}
			if !errors.Is(err, storage.ErrScopeNotFound) {
				t.Fatalf("ResolveScope(%+v) erro = %v; want encadear ErrScopeNotFound", c.req, err)
			}
			if got != (domain.Scope{}) || src != "" {
				t.Fatalf("ResolveScope(%+v) devolveu escopo %+v (fonte %q) junto com erro", c.req, got, src)
			}

			// Então (c): sem casamento, o erro não é ambiguidade.
			if errors.Is(err, storage.ErrScopeAmbiguous) {
				t.Fatalf("ResolveScope(%+v) erro = %v; não deveria ser ErrScopeAmbiguous sem casamento", c.req, err)
			}

			// Então (b): nada foi criado em workspaces nem em projects.
			if ws := sfContar(t, readDB, "workspaces"); ws != wsInicial {
				t.Fatalf("workspaces = %d após leitura, want %d (nada criado)", ws, wsInicial)
			}
			if pj := sfContar(t, readDB, "projects"); pj != pjInicial {
				t.Fatalf("projects = %d após leitura, want %d (nada criado)", pj, pjInicial)
			}
		})
	}
}
