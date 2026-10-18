package wiki

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/phm-aguiar/ai-memory-go/internal/domain"
)

// metaNames são os nomes usados em todos os testes de manifesto.
var metaNames = ScopeNames{
	WorkspaceName: "Workspace Demo",
	ProjectName:   "Projeto X",
	RepoPath:      "/repos/projeto-x",
}

// readFile lê um arquivo do teste e falha se não conseguir.
func readFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(%s): %v", path, err)
	}
	return string(data)
}

// scopeDirOf devolve <root>/<ws>/<proj>, o diretório do escopo.
func scopeDirOf(root string, scope domain.Scope) string {
	return filepath.Join(root, scope.WorkspaceID.String(), scope.ProjectID.String())
}

func TestMetaEnsureScopeManifestWritesProjectAndWorkspace(t *testing.T) {
	root, scope, _ := confineFixture(t)

	changed, err := EnsureScopeManifest(root, scope, metaNames)
	if err != nil {
		t.Fatalf("EnsureScopeManifest: %v", err)
	}
	wantChanged := []string{
		scope.WorkspaceID.String() + "/_meta.md",
		scope.WorkspaceID.String() + "/" + scope.ProjectID.String() + "/_meta.md",
	}
	if strings.Join(changed, "|") != strings.Join(wantChanged, "|") {
		t.Fatalf("caminhos alterados = %v, quer %v", changed, wantChanged)
	}

	wsID := scope.WorkspaceID.String()
	projID := scope.ProjectID.String()
	wantWS := "---\n" +
		"type: Scope Manifest\n" +
		"name: Workspace Demo\n" +
		"workspace_id: " + wsID + "\n" +
		"---\n"
	wantProj := "---\n" +
		"type: Scope Manifest\n" +
		"name: Projeto X\n" +
		"workspace_id: " + wsID + "\n" +
		"project_id: " + projID + "\n" +
		"repo_path: /repos/projeto-x\n" +
		"---\n"

	if got := readFile(t, filepath.Join(root, wsID, "_meta.md")); got != wantWS {
		t.Fatalf("_meta.md do workspace:\n%q\nquer:\n%q", got, wantWS)
	}
	if got := readFile(t, filepath.Join(scopeDirOf(root, scope), "_meta.md")); got != wantProj {
		t.Fatalf("_meta.md do projeto:\n%q\nquer:\n%q", got, wantProj)
	}
}

func TestMetaManifestFrontmatterRoundTripsWithoutBody(t *testing.T) {
	root, scope, _ := confineFixture(t)
	if _, err := EnsureScopeManifest(root, scope, metaNames); err != nil {
		t.Fatalf("EnsureScopeManifest: %v", err)
	}

	fm, body, err := ReadMarkdown([]byte(readFile(t, filepath.Join(scopeDirOf(root, scope), "_meta.md"))))
	if err != nil {
		t.Fatalf("ReadMarkdown: %v", err)
	}
	if body != "" {
		t.Fatalf("corpo = %q, quer vazio", body)
	}
	wantKeys := []string{"type", "name", "workspace_id", "project_id", "repo_path"}
	if strings.Join(fm.Keys(), ",") != strings.Join(wantKeys, ",") {
		t.Fatalf("chaves = %v, quer %v", fm.Keys(), wantKeys)
	}
	if v, _ := fm.Get("type"); v != "Scope Manifest" {
		t.Fatalf("type = %v, quer Scope Manifest", v)
	}
}

func TestMetaSecondCallDoesNotRewrite(t *testing.T) {
	root, scope, _ := confineFixture(t)
	if _, err := EnsureScopeManifest(root, scope, metaNames); err != nil {
		t.Fatalf("primeira chamada: %v", err)
	}
	projFile := filepath.Join(scopeDirOf(root, scope), "_meta.md")
	before := readFile(t, projFile)

	changed, err := EnsureScopeManifest(root, scope, metaNames)
	if err != nil {
		t.Fatalf("segunda chamada: %v", err)
	}
	if len(changed) != 0 {
		t.Fatalf("segunda chamada alterou %v, quer nenhum caminho", changed)
	}
	if after := readFile(t, projFile); after != before {
		t.Fatalf("bytes mudaram na segunda chamada:\n%q\n%q", before, after)
	}
}

func TestMetaRenameRewritesManifestWithNewName(t *testing.T) {
	root, scope, _ := confineFixture(t)
	if _, err := EnsureScopeManifest(root, scope, metaNames); err != nil {
		t.Fatalf("primeira chamada: %v", err)
	}
	renamed := metaNames
	renamed.ProjectName = "Projeto Novo"

	changed, err := EnsureScopeManifest(root, scope, renamed)
	if err != nil {
		t.Fatalf("chamada com nome novo: %v", err)
	}
	if len(changed) != 1 || changed[0] != scope.WorkspaceID.String()+"/"+scope.ProjectID.String()+"/_meta.md" {
		t.Fatalf("caminhos alterados = %v, quer só o manifesto do projeto", changed)
	}
	if got := readFile(t, filepath.Join(scopeDirOf(root, scope), "_meta.md")); !strings.Contains(got, "name: Projeto Novo\n") {
		t.Fatalf("manifesto não trouxe o nome novo:\n%s", got)
	}
}

func TestMetaRejectsEmptyNames(t *testing.T) {
	root, scope, _ := confineFixture(t)
	bad := metaNames
	bad.ProjectName = ""
	if _, err := EnsureScopeManifest(root, scope, bad); err == nil {
		t.Fatal("nome de projeto vazio aceito")
	}
	if _, err := os.Stat(filepath.Join(root, scope.WorkspaceID.String())); !os.IsNotExist(err) {
		t.Fatalf("nome inválido criou diretório: %v", err)
	}
}

func TestMetaRefuseRelativeRoot(t *testing.T) {
	_, scope, _ := confineFixture(t)
	const rel = "relativo/raiz"
	if _, err := EnsureScopeManifest(rel, scope, metaNames); !errors.Is(err, ErrConfinement) {
		t.Fatalf("EnsureScopeManifest: erro = %v, quer ErrConfinement", err)
	}
}

func TestMetaRefuseLinkedScopeComponent(t *testing.T) {
	root, scope, _ := confineFixture(t)
	outside := t.TempDir()
	// O diretório do workspace vira link para fora da raiz: nada pode ser gravado por ele.
	makeLink(t, outside, filepath.Join(root, scope.WorkspaceID.String()))

	if _, err := EnsureScopeManifest(root, scope, metaNames); !errors.Is(err, ErrConfinement) {
		t.Fatalf("EnsureScopeManifest: erro = %v, quer ErrConfinement", err)
	}
	entries, err := os.ReadDir(outside)
	if err != nil {
		t.Fatalf("ReadDir(outside): %v", err)
	}
	if len(entries) != 0 {
		t.Fatalf("escrita vazou para fora da raiz: %v", entries)
	}
}
