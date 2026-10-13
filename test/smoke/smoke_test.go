// Package smoke é o teste fim a fim da CLI. Compila o binário ai-memory, cria o estado em
// disco (páginas da wiki e banco) pela biblioteca, roda reindex e backup como processos
// separados e confere o resultado. Todo arquivo fica em t.TempDir().
package smoke

import (
	"bytes"
	"context"
	"go/build"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/phm-aguiar/ai-memory-go/internal/domain"
	"github.com/phm-aguiar/ai-memory-go/internal/storage"
	"github.com/phm-aguiar/ai-memory-go/internal/wiki"
)

// cmdTimeout limita cada execução do binário, para que um travamento não prenda o teste.
const cmdTimeout = 3 * time.Minute

// smokePage descreve uma página escrita pela biblioteca antes do binário rodar.
type smokePage struct {
	scope   domain.Scope
	wsName  string
	prjName string
	path    string
	body    string
}

// TestSmokeReindexEBackupFimAFim é o caminho feliz completo: build, semeadura, reindex,
// backup, abertura do backup e leitura das páginas depois do reindex.
func TestSmokeReindexEBackupFimAFim(t *testing.T) {
	ctx := context.Background()
	root := moduleRoot(t)
	bin := buildBinary(t, root)

	tmp := t.TempDir()
	dataDir := filepath.Join(tmp, "data")
	cfgPath := writeConfig(t, tmp, dataDir)
	// O .env nunca existe: a configuração vem só do TOML e do ambiente filtrado.
	envPath := filepath.Join(tmp, "ausente.env")

	pages := seedPages(t, ctx, dataDir)

	// Reindex: o binário lê o mesmo data_dir e reconstrói o índice a partir do markdown.
	out, errOut, err := runBinary(t, bin, "--config", cfgPath, "--env-file", envPath, "reindex")
	if err != nil {
		t.Fatalf("reindex: %v\nsaída:\n%s\nerro:\n%s", err, out, errOut)
	}
	for _, want := range []string{"reindex concluído", "escopos: 2", "páginas: 3", "erros: 0"} {
		if !strings.Contains(out, want) {
			t.Fatalf("saída do reindex sem %q:\n%s", want, out)
		}
	}
	t.Logf("saída do reindex:\n%s", out)

	// Backup online: o binário grava o snapshot em um arquivo novo.
	copyPath := filepath.Join(tmp, "copia.db")
	out, errOut, err = runBinary(t, bin, "--config", cfgPath, "--env-file", envPath, "backup", "--to", copyPath)
	if err != nil {
		t.Fatalf("backup: %v\nsaída:\n%s\nerro:\n%s", err, out, errOut)
	}
	if !strings.Contains(out, "backup gravado em") {
		t.Fatalf("saída do backup sem confirmação:\n%s", out)
	}
	t.Logf("saída do backup:\n%s", out)
	if _, err := os.Stat(copyPath); err != nil {
		t.Fatalf("snapshot não foi gravado: %v", err)
	}

	checkBackupOpens(t, ctx, tmp, copyPath, len(pages))
	checkPagesReadable(t, ctx, dataDir, pages)
}

// moduleRoot sobe a partir do diretório do teste até achar o go.mod.
func moduleRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd: %v", err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatalf("go.mod não encontrado subindo a partir do diretório do teste")
		}
		dir = parent
	}
}

// findGo devolve o caminho do go: primeiro pelo PATH, depois pelo GOROOT do próprio teste.
func findGo(t *testing.T) string {
	t.Helper()
	if p, err := exec.LookPath("go"); err == nil {
		return p
	}
	name := "go"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	p := filepath.Join(build.Default.GOROOT, "bin", name)
	if _, err := os.Stat(p); err != nil {
		t.Fatalf("go não encontrado no PATH nem em GOROOT (%s): %v", p, err)
	}
	return p
}

// buildBinary compila ./cmd/ai-memory com CGO_ENABLED=0 em t.TempDir() e devolve o caminho.
func buildBinary(t *testing.T, root string) string {
	t.Helper()
	name := "ai-memory"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	out := filepath.Join(t.TempDir(), name)
	cmd := exec.Command(findGo(t), "build", "-o", out, "./cmd/ai-memory")
	cmd.Dir = root
	cmd.Env = append(isolatedEnv(), "CGO_ENABLED=0")
	if msg, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("go build ./cmd/ai-memory: %v\n%s", err, msg)
	}
	return out
}

// writeConfig grava um TOML com data_dir apontando para dataDir. Usa string literal (aspas
// simples) para que as barras invertidas do Windows não sejam tratadas como escapes.
func writeConfig(t *testing.T, tmp, dataDir string) string {
	t.Helper()
	if strings.ContainsRune(dataDir, '\'') {
		t.Fatalf("caminho com aspas simples não cabe em TOML literal: %s", dataDir)
	}
	cfg := filepath.Join(tmp, "config.toml")
	if err := os.WriteFile(cfg, []byte("data_dir = '"+dataDir+"'\n"), 0o600); err != nil {
		t.Fatalf("gravar config.toml: %v", err)
	}
	return cfg
}

// isolatedEnv devolve o ambiente do processo sem as variáveis AI_MEMORY_*, para que o
// binário dependa só do TOML que o teste escreve.
func isolatedEnv() []string {
	var env []string
	for _, kv := range os.Environ() {
		key, _, _ := strings.Cut(kv, "=")
		if strings.HasPrefix(key, "AI_MEMORY_") {
			continue
		}
		env = append(env, kv)
	}
	return env
}

// runBinary executa o binário com o ambiente isolado e devolve stdout, stderr e o erro de saída.
func runBinary(t *testing.T, bin string, args ...string) (string, string, error) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), cmdTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, bin, args...)
	cmd.Env = isolatedEnv()
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	return stdout.String(), stderr.String(), err
}

// newID gera um identificador novo ou encerra o teste.
func newID(t *testing.T) domain.ID {
	t.Helper()
	id, err := domain.NewID()
	if err != nil {
		t.Fatalf("NewID: %v", err)
	}
	return id
}

// seedPages cria, pela biblioteca, duas páginas num escopo e uma no outro, sobre dataDir.
// Fecha o escritor e o banco antes de voltar, para que o binário rode sem conexões abertas.
func seedPages(t *testing.T, ctx context.Context, dataDir string) []smokePage {
	t.Helper()
	db, err := storage.Open(ctx, dataDir)
	if err != nil {
		t.Fatalf("storage.Open (semeadura): %v", err)
	}
	defer func() { _ = db.Close() }()

	w, err := storage.NewWriter(ctx, db)
	if err != nil {
		t.Fatalf("storage.NewWriter (semeadura): %v", err)
	}
	defer func() { _ = w.Close() }()

	wikiRoot := filepath.Join(dataDir, "wiki")
	repo, err := wiki.Open(wikiRoot)
	if err != nil {
		t.Fatalf("wiki.Open: %v", err)
	}
	store := &wiki.Store{
		Root:   wikiRoot,
		Writer: w,
		Repo:   repo,
		Locks:  wiki.NewPageLocks(),
	}

	ws := domain.WorkspaceID(newID(t))
	projA := domain.ProjectID(newID(t))
	projB := domain.ProjectID(newID(t))
	scopeA := domain.NewScope(ws, projA)
	scopeB := domain.NewScope(ws, projB)

	pages := []smokePage{
		{scope: scopeA, wsName: "ws-smoke", prjName: "proj-a", path: "notas/inicio.md", body: "# Início\n\nprimeira página do escopo A\n"},
		{scope: scopeA, wsName: "ws-smoke", prjName: "proj-a", path: "conceitos/segunda.md", body: "# Segunda\n\nsegunda página do escopo A\n"},
		{scope: scopeB, wsName: "ws-smoke", prjName: "proj-b", path: "notas/inicio.md", body: "# Início B\n\npágina do escopo B\n"},
	}
	for _, p := range pages {
		path, err := domain.ParsePagePath(p.path)
		if err != nil {
			t.Fatalf("ParsePagePath(%q): %v", p.path, err)
		}
		_, err = store.WritePage(ctx, wiki.PageInput{
			Scope:         p.scope,
			WorkspaceName: p.wsName,
			ProjectName:   p.prjName,
			Path:          path,
			Tier:          domain.TierSemantic,
			Body:          p.body,
			Evidence:      storage.EvidenceSource{Kind: storage.SourceSession, ID: "smoke-sessao"},
		})
		if err != nil {
			t.Fatalf("WritePage(%q): %v", p.path, err)
		}
	}

	if err := w.Close(); err != nil {
		t.Fatalf("fechar escritor da semeadura: %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatalf("fechar banco da semeadura: %v", err)
	}
	return pages
}

// checkBackupOpens copia o snapshot para <tmp>/abre/db/memory.sqlite, abre com storage.Open e
// confere que schema_migrations existe e que as páginas correntes estão lá.
func checkBackupOpens(t *testing.T, ctx context.Context, tmp, copyPath string, wantPages int) {
	t.Helper()
	openDir := filepath.Join(tmp, "abre")
	dbDir := filepath.Join(openDir, "db")
	if err := os.MkdirAll(dbDir, 0o700); err != nil {
		t.Fatalf("criar diretório do backup: %v", err)
	}
	data, err := os.ReadFile(copyPath)
	if err != nil {
		t.Fatalf("ler snapshot: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dbDir, "memory.sqlite"), data, 0o600); err != nil {
		t.Fatalf("copiar snapshot para diretório de abertura: %v", err)
	}

	db, err := storage.Open(ctx, openDir)
	if err != nil {
		t.Fatalf("storage.Open do snapshot: %v", err)
	}
	defer func() { _ = db.Close() }()

	var migrations int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM schema_migrations`).Scan(&migrations); err != nil {
		t.Fatalf("consultar schema_migrations no snapshot: %v", err)
	}
	if migrations == 0 {
		t.Fatalf("snapshot sem migrações aplicadas")
	}

	var pages int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM pages WHERE is_latest = 1`).Scan(&pages); err != nil {
		t.Fatalf("consultar páginas no snapshot: %v", err)
	}
	if pages != wantPages {
		t.Fatalf("snapshot tem %d páginas correntes, esperava %d", pages, wantPages)
	}
}

// checkPagesReadable reabre o data_dir depois do reindex e confere que cada página escrita
// continua legível com o mesmo corpo.
func checkPagesReadable(t *testing.T, ctx context.Context, dataDir string, pages []smokePage) {
	t.Helper()
	db, err := storage.Open(ctx, dataDir)
	if err != nil {
		t.Fatalf("storage.Open (leitura pós-reindex): %v", err)
	}
	defer func() { _ = db.Close() }()

	r := storage.NewReader(db)
	for _, p := range pages {
		path, err := domain.ParsePagePath(p.path)
		if err != nil {
			t.Fatalf("ParsePagePath(%q): %v", p.path, err)
		}
		got, err := storage.ReadPage(ctx, r, domain.NewPageRef(p.scope, path))
		if err != nil {
			t.Fatalf("ReadPage(%q) depois do reindex: %v", p.path, err)
		}
		if strings.TrimSpace(got.Body) != strings.TrimSpace(p.body) {
			t.Fatalf("corpo de %q diverge depois do reindex:\nesperado: %q\nobtido: %q", p.path, p.body, got.Body)
		}
	}
}
