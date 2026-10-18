package cli

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/phm-aguiar/ai-memory-go/internal/config"
	"github.com/phm-aguiar/ai-memory-go/internal/domain"
	"github.com/phm-aguiar/ai-memory-go/internal/storage"
	"github.com/phm-aguiar/ai-memory-go/internal/wiki"
)

// isolateReindexEnv remove do processo as variáveis AI_MEMORY_* do host, para que o
// teste dependa só do TOML que ele mesmo escreve.
func isolateReindexEnv(t *testing.T) {
	t.Helper()
	for _, kv := range os.Environ() {
		key, _, _ := strings.Cut(kv, "=")
		if strings.HasPrefix(key, "AI_MEMORY_") {
			t.Setenv(key, "")
			_ = os.Unsetenv(key)
		}
	}
}

// reindexFixture cria config.toml com data_dir em <tmp>/data e um .env inexistente.
// Devolve as opções de config e o data_dir.
func reindexFixture(t *testing.T) (config.Options, string) {
	t.Helper()
	isolateReindexEnv(t)
	tmp := t.TempDir()
	dataDir := filepath.Join(tmp, "data")
	cfgPath := filepath.Join(tmp, "config.toml")
	// Literal TOML (aspas simples) para que barras invertidas do Windows não sejam escapes.
	if err := os.WriteFile(cfgPath, []byte("data_dir = '"+dataDir+"'\n"), 0o600); err != nil {
		t.Fatalf("gravar config.toml: %v", err)
	}
	envPath := filepath.Join(tmp, "ausente.env")
	return config.Options{ConfigFile: cfgPath, EnvFile: envPath}, dataDir
}

// seedReindexWiki grava uma página na wiki do data_dir por Compose e WritePage, como
// o servidor faria, fecha o App e remove o banco (db/ com -wal e -shm). Fica só a
// wiki, que é o estado que o reindex reconstrói a partir de um banco limpo
// (docs/lifecycle-ops.md, passo 3 do procedimento de migração).
func seedReindexWiki(t *testing.T, opts config.Options) {
	t.Helper()
	seedReindexWikiRows(t, opts)
	cfg, err := loadConfig(opts)
	if err != nil {
		t.Fatalf("loadConfig: %v", err)
	}
	if err := os.RemoveAll(filepath.Join(cfg.DataDir(), "db")); err != nil {
		t.Fatalf("remover db/ semeado: %v", err)
	}
}

// seedReindexWikiRows grava a página pelo App (banco + wiki) e fecha o App antes de devolver.
// Deixa o banco com linhas: é o caso de banco sujo.
func seedReindexWikiRows(t *testing.T, opts config.Options) {
	t.Helper()
	ctx := context.Background()
	app, err := Compose(ctx, opts)
	if err != nil {
		t.Fatalf("Compose para semear: %v", err)
	}
	defer func() {
		if cerr := app.Close(); cerr != nil {
			t.Fatalf("fechar App semeado: %v", cerr)
		}
	}()

	ws, err := domain.NewID()
	if err != nil {
		t.Fatalf("NewID: %v", err)
	}
	pj, err := domain.NewID()
	if err != nil {
		t.Fatalf("NewID: %v", err)
	}
	path, err := domain.ParsePagePath("notas/inicio.md")
	if err != nil {
		t.Fatalf("ParsePagePath: %v", err)
	}
	_, err = app.Store.WritePage(ctx, wiki.PageInput{
		Scope:         domain.NewScope(domain.WorkspaceID(ws), domain.ProjectID(pj)),
		WorkspaceName: "ws-teste",
		ProjectName:   "proj-teste",
		Path:          path,
		Tier:          domain.TierSemantic,
		Body:          "# Início\n\ncorpo da página",
		Evidence:      storage.EvidenceSource{Kind: storage.SourceSession, ID: "sessao-1"},
	})
	if err != nil {
		t.Fatalf("WritePage: %v", err)
	}
}

// runReindexCmd monta a raiz com o subcomando reindex e a executa com as flags de config.
func runReindexCmd(t *testing.T, opts config.Options, extra ...string) (string, error) {
	t.Helper()
	root := NewRootCommand()
	root.AddCommand(NewReindexCommand())
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	args := append([]string{"reindex", "--" + flagConfig, opts.ConfigFile, "--" + flagEnvFile, opts.EnvFile}, extra...)
	root.SetArgs(args)
	err := root.Execute()
	return out.String(), err
}

func TestReindexExecutaEImprimePaginas(t *testing.T) {
	opts, _ := reindexFixture(t)
	seedReindexWiki(t, opts)

	out, err := runReindexCmd(t, opts)
	if err != nil {
		t.Fatalf("reindex: %v\nsaída:\n%s", err, out)
	}
	if !strings.Contains(out, "reindex concluído") {
		t.Fatalf("saída sem relatório:\n%s", out)
	}
	if !strings.Contains(out, "páginas: 1") {
		t.Fatalf("esperava páginas: 1 (uma página escrita):\n%s", out)
	}
}

func TestReindexRecusaComProcessoVivo(t *testing.T) {
	opts, _ := reindexFixture(t)
	seedReindexWiki(t, opts)
	fakeProcesses(t, procInfo{pid: os.Getpid() + 1, name: "ai-memory"})

	out, err := runReindexCmd(t, opts)
	if !errors.Is(err, ErrProcessLive) {
		t.Fatalf("esperava erro encadeado com ErrProcessLive, veio %v", err)
	}
	if strings.Contains(out, "reindex concluído") {
		t.Fatalf("reindex executou com processo vivo:\n%s", out)
	}
}

func TestReindexSemOutroProcessoExecuta(t *testing.T) {
	opts, _ := reindexFixture(t)
	seedReindexWiki(t, opts)
	fakeProcesses(t, procInfo{pid: os.Getpid(), name: "ai-memory"}, procInfo{pid: os.Getpid() + 1, name: "bash"})

	if _, err := runReindexCmd(t, opts); err != nil {
		t.Fatalf("sem outro ai-memory esperava execução, veio %v", err)
	}
}

func TestReindexFalhaDeEnumeracaoRecusa(t *testing.T) {
	opts, _ := reindexFixture(t)
	seedReindexWiki(t, opts)
	fakeEnumError(t, errors.New("enumeração indisponível"))

	out, err := runReindexCmd(t, opts)
	if err == nil {
		t.Fatalf("falha de enumeração esperava recusa, saída:\n%s", out)
	}
	if errors.Is(err, ErrProcessLive) {
		t.Fatalf("falha de enumeração não deve ser ErrProcessLive: %v", err)
	}
	if strings.Contains(out, "reindex concluído") {
		t.Fatalf("reindex executou com enumeração falhando:\n%s", out)
	}
}

func TestReindexRejeitaArgumentos(t *testing.T) {
	opts, _ := reindexFixture(t)
	if _, err := runReindexCmd(t, opts, "extra"); err == nil {
		t.Fatal("esperava erro com argumento posicional")
	}
}

// openReindexDB abre o banco de dataDir com storage.Open para inspecionar ou semear linhas
// diretamente; quem chama fecha o *sql.DB.
func openReindexDB(t *testing.T, dataDir string) *sql.DB {
	t.Helper()
	db, err := storage.Open(context.Background(), dataDir)
	if err != nil {
		t.Fatalf("storage.Open: %v", err)
	}
	return db
}

func TestReindexBancoLimpoProsegue(t *testing.T) {
	opts, dataDir := reindexFixture(t)
	seedReindexWiki(t, opts)

	db := openReindexDB(t, dataDir)
	defer func() { _ = db.Close() }()
	var n int64
	if err := db.QueryRow("SELECT COUNT(*) FROM pages").Scan(&n); err != nil {
		t.Fatalf("COUNT pages: %v", err)
	}
	if n != 0 {
		t.Fatalf("banco semeado para o teste limpo tem %d páginas, esperado 0", n)
	}
	_ = db.Close()

	out, err := runReindexCmd(t, opts)
	if err != nil {
		t.Fatalf("banco limpo esperava execução, veio %v\nsaída:\n%s", err, out)
	}
	if !strings.Contains(out, "reindex concluído") || !strings.Contains(out, "páginas: 1") {
		t.Fatalf("banco limpo sem relatório esperado:\n%s", out)
	}
}

func TestReindexRecusaBancoComUmaTabela(t *testing.T) {
	opts, dataDir := reindexFixture(t)
	seedReindexWiki(t, opts)

	db := openReindexDB(t, dataDir)
	if _, err := db.Exec(`INSERT INTO workspaces (id, name, created_at) VALUES (?, ?, ?)`,
		make([]byte, 16), "ws-sujo", "2026-01-01T00:00:00Z"); err != nil {
		t.Fatalf("semear workspace: %v", err)
	}
	_ = db.Close()

	out, err := runReindexCmd(t, opts)
	if !errors.Is(err, ErrDatabaseNotClean) {
		t.Fatalf("uma tabela com linhas esperava ErrDatabaseNotClean, veio %v\nsaída:\n%s", err, out)
	}
	if !strings.Contains(err.Error(), "workspaces: 1") {
		t.Fatalf("mensagem não nomeia workspaces: 1: %v", err)
	}
	if strings.Contains(err.Error(), "pages:") {
		t.Fatalf("mensagem nomeia tabela zerada pages: %v", err)
	}
	if strings.Contains(out, "reindex concluído") {
		t.Fatalf("reindex executou com banco sujo:\n%s", out)
	}
}

func TestReindexRecusaBancoComVariasTabelas(t *testing.T) {
	opts, dataDir := reindexFixture(t)
	seedReindexWikiRows(t, opts)

	db := openReindexDB(t, dataDir)
	defer func() { _ = db.Close() }()
	// Contagens reais de cada tabela semeada; a mensagem tem de nomear cada uma delas.
	tables := []string{"workspaces", "projects", "pages"}
	for _, table := range tables {
		var n int64
		if err := db.QueryRow("SELECT COUNT(*) FROM " + table).Scan(&n); err != nil {
			t.Fatalf("COUNT %s: %v", table, err)
		}
		if n == 0 {
			t.Fatalf("semeadura não criou linhas em %s", table)
		}
	}
	_ = db.Close()

	out, err := runReindexCmd(t, opts)
	if !errors.Is(err, ErrDatabaseNotClean) {
		t.Fatalf("várias tabelas com linhas esperava ErrDatabaseNotClean, veio %v\nsaída:\n%s", err, out)
	}
	db2 := openReindexDB(t, dataDir)
	defer func() { _ = db2.Close() }()
	for _, table := range tables {
		var n int64
		if err := db2.QueryRow("SELECT COUNT(*) FROM " + table).Scan(&n); err != nil {
			t.Fatalf("COUNT %s: %v", table, err)
		}
		want := fmt.Sprintf("%s: %d", table, n)
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("mensagem não nomeia %q: %v", want, err)
		}
	}
	if strings.Contains(out, "reindex concluído") {
		t.Fatalf("reindex executou com banco sujo:\n%s", out)
	}
}
