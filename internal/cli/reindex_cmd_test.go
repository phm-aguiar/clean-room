package cli

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strconv"
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
// o servidor faria, e fecha o App antes de devolver.
func seedReindexWiki(t *testing.T, opts config.Options) {
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
	opts, dataDir := reindexFixture(t)
	seedReindexWiki(t, opts)
	if err := os.WriteFile(filepath.Join(dataDir, PIDFileName), []byte(strconv.Itoa(os.Getpid())), 0o600); err != nil {
		t.Fatalf("gravar marcador de PID: %v", err)
	}

	out, err := runReindexCmd(t, opts)
	if !errors.Is(err, ErrProcessLive) {
		t.Fatalf("esperava erro encadeado com ErrProcessLive, veio %v", err)
	}
	if strings.Contains(out, "reindex concluído") {
		t.Fatalf("reindex executou com processo vivo:\n%s", out)
	}
}

func TestReindexSemPIDExecuta(t *testing.T) {
	opts, _ := reindexFixture(t)
	seedReindexWiki(t, opts)

	if _, err := runReindexCmd(t, opts); err != nil {
		t.Fatalf("sem marcador de PID esperava execução, veio %v", err)
	}
}

func TestReindexPIDDeProcessoMortoExecuta(t *testing.T) {
	const pid = 999999
	if alive, err := processAlive(pid); err == nil && alive {
		t.Skipf("pid %d existe nesta máquina; teste não se aplica", pid)
	}
	opts, dataDir := reindexFixture(t)
	seedReindexWiki(t, opts)
	if err := os.WriteFile(filepath.Join(dataDir, PIDFileName), []byte(strconv.Itoa(pid)), 0o600); err != nil {
		t.Fatalf("gravar marcador de PID: %v", err)
	}

	if _, err := runReindexCmd(t, opts); err != nil {
		t.Fatalf("PID inexistente esperava execução, veio %v", err)
	}
}

func TestReindexRejeitaArgumentos(t *testing.T) {
	opts, _ := reindexFixture(t)
	if _, err := runReindexCmd(t, opts, "extra"); err == nil {
		t.Fatal("esperava erro com argumento posicional")
	}
}
