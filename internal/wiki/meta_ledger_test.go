package wiki

import (
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

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

func TestMetaProjectIndexCreatedOnceAndNotRewritten(t *testing.T) {
	root, scope, _ := confineFixture(t)

	changed, err := EnsureProjectIndex(root, scope)
	if err != nil {
		t.Fatalf("primeira criação: %v", err)
	}
	wantPath := scope.WorkspaceID.String() + "/" + scope.ProjectID.String() + "/index.md"
	if len(changed) != 1 || changed[0] != wantPath {
		t.Fatalf("caminhos alterados = %v, quer [%s]", changed, wantPath)
	}
	idxFile := filepath.Join(scopeDirOf(root, scope), "index.md")
	wantContent := "---\nokf_version: \"0.2\"\n---\n# Índice\n"
	if got := readFile(t, idxFile); got != wantContent {
		t.Fatalf("index.md:\n%q\nquer:\n%q", got, wantContent)
	}

	// Simula edição feita depois: a operação normal não pode regravar o arquivo.
	edited := "conteúdo editado\n"
	if err := os.WriteFile(idxFile, []byte(edited), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	changed, err = EnsureProjectIndex(root, scope)
	if err != nil {
		t.Fatalf("segunda chamada: %v", err)
	}
	if len(changed) != 0 {
		t.Fatalf("segunda chamada devolveu %v, quer nenhum", changed)
	}
	if got := readFile(t, idxFile); got != edited {
		t.Fatalf("index.md foi regravado: %q", got)
	}
}

func TestLedgerAppendLineExactFormatAndUTC(t *testing.T) {
	root, scope, _ := confineFixture(t)
	// Horário com fuso +02:00: a linha tem de sair em UTC.
	at := time.Date(2026, 10, 12, 19, 30, 0, 0, time.FixedZone("UTC+2", 2*60*60))

	if err := AppendLogLine(root, scope, "page-write", "Título com acento é ok", at); err != nil {
		t.Fatalf("AppendLogLine: %v", err)
	}
	got := readFile(t, filepath.Join(scopeDirOf(root, scope), "log.md"))
	want := "## [2026-10-12T17:30:00Z] page-write | Título com acento é ok\n"
	if got != want {
		t.Fatalf("log.md = %q, quer %q", got, want)
	}
	re := regexp.MustCompile(`^## \[\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}Z\] (page-write|page-delete) \| [^\r\n]+\n$`)
	if !re.MatchString(got) {
		t.Fatalf("linha não bate com o formato RFC 3339 do ADR-21: %q", got)
	}
}

func TestLedgerAppendAccumulatesInOrder(t *testing.T) {
	root, scope, _ := confineFixture(t)
	base := time.Date(2026, 10, 12, 17, 0, 0, 0, time.UTC)
	steps := []struct {
		event, title string
	}{
		{"page-write", "primeira"},
		{"page-delete", "segunda"},
		{"page-write", "terceira"},
	}
	for i, s := range steps {
		if err := AppendLogLine(root, scope, s.event, s.title, base.Add(time.Duration(i)*time.Second)); err != nil {
			t.Fatalf("AppendLogLine #%d: %v", i, err)
		}
	}
	want := "## [2026-10-12T17:00:00Z] page-write | primeira\n" +
		"## [2026-10-12T17:00:01Z] page-delete | segunda\n" +
		"## [2026-10-12T17:00:02Z] page-write | terceira\n"
	if got := readFile(t, filepath.Join(scopeDirOf(root, scope), "log.md")); got != want {
		t.Fatalf("log.md:\n%q\nquer:\n%q", got, want)
	}
}

func TestLedgerAppendKeepsExistingContent(t *testing.T) {
	root, scope, _ := confineFixture(t)
	logFile := filepath.Join(scopeDirOf(root, scope), "log.md")
	if err := os.MkdirAll(filepath.Dir(logFile), 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	const prior = "## [2026-10-11T08:00:00Z] page-write | anterior\n"
	if err := os.WriteFile(logFile, []byte(prior), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	at := time.Date(2026, 10, 12, 9, 0, 0, 0, time.UTC)
	if err := AppendLogLine(root, scope, "page-write", "nova", at); err != nil {
		t.Fatalf("AppendLogLine: %v", err)
	}
	want := prior + "## [2026-10-12T09:00:00Z] page-write | nova\n"
	if got := readFile(t, logFile); got != want {
		t.Fatalf("log.md = %q, quer %q", got, want)
	}
}

func TestLedgerRejectsInvalidEvent(t *testing.T) {
	root, scope, _ := confineFixture(t)
	for _, ev := range []string{"", "page-create", "Page-Write", "page-write | x"} {
		err := AppendLogLine(root, scope, ev, "título", time.Now())
		if !errors.Is(err, ErrInvalidLogLine) {
			t.Fatalf("evento %q: erro = %v, quer ErrInvalidLogLine", ev, err)
		}
	}
	if _, err := os.Stat(filepath.Join(scopeDirOf(root, scope), "log.md")); !os.IsNotExist(err) {
		t.Fatalf("evento inválido criou log.md: %v", err)
	}
}

func TestLedgerRejectsTitleWithLineBreakOrControl(t *testing.T) {
	root, scope, _ := confineFixture(t)
	titles := []string{
		"",
		"linha\nquebrada",
		"linha\rcr",
		"com\ttab",
		"nul\x00byte",
		"sep linha",
		"par linha",
		"del\x7fchar",
	}
	for _, title := range titles {
		err := AppendLogLine(root, scope, "page-write", title, time.Now())
		if !errors.Is(err, ErrInvalidLogLine) {
			t.Fatalf("título %q: erro = %v, quer ErrInvalidLogLine", title, err)
		}
	}
	if _, err := os.Stat(filepath.Join(scopeDirOf(root, scope), "log.md")); !os.IsNotExist(err) {
		t.Fatalf("título inválido criou log.md: %v", err)
	}
}

func TestMetaAndLedgerRefuseRelativeRoot(t *testing.T) {
	_, scope, _ := confineFixture(t)
	const rel = "relativo/raiz"
	if _, err := EnsureScopeManifest(rel, scope, metaNames); !errors.Is(err, ErrConfinement) {
		t.Fatalf("EnsureScopeManifest: erro = %v, quer ErrConfinement", err)
	}
	if _, err := EnsureProjectIndex(rel, scope); !errors.Is(err, ErrConfinement) {
		t.Fatalf("EnsureProjectIndex: erro = %v, quer ErrConfinement", err)
	}
	if err := AppendLogLine(rel, scope, "page-write", "t", time.Now()); !errors.Is(err, ErrConfinement) {
		t.Fatalf("AppendLogLine: erro = %v, quer ErrConfinement", err)
	}
}

func TestMetaAndLedgerRefuseLinkedScopeComponent(t *testing.T) {
	root, scope, _ := confineFixture(t)
	outside := t.TempDir()
	// O diretório do workspace vira link para fora da raiz: nada pode ser gravado por ele.
	makeLink(t, outside, filepath.Join(root, scope.WorkspaceID.String()))

	if _, err := EnsureScopeManifest(root, scope, metaNames); !errors.Is(err, ErrConfinement) {
		t.Fatalf("EnsureScopeManifest: erro = %v, quer ErrConfinement", err)
	}
	if _, err := EnsureProjectIndex(root, scope); !errors.Is(err, ErrConfinement) {
		t.Fatalf("EnsureProjectIndex: erro = %v, quer ErrConfinement", err)
	}
	if err := AppendLogLine(root, scope, "page-write", "t", time.Now()); !errors.Is(err, ErrConfinement) {
		t.Fatalf("AppendLogLine: erro = %v, quer ErrConfinement", err)
	}
	entries, err := os.ReadDir(outside)
	if err != nil {
		t.Fatalf("ReadDir(outside): %v", err)
	}
	if len(entries) != 0 {
		t.Fatalf("escrita vazou para fora da raiz: %v", entries)
	}
}
