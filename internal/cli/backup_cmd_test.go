package cli

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/phm-aguiar/ai-memory-go/internal/storage"
)

// isolateBackupEnv remove do processo as variáveis AI_MEMORY_* do host, para que
// o teste dependa só do TOML e do .env que ele mesmo escreve.
func isolateBackupEnv(t *testing.T) {
	t.Helper()
	for _, kv := range os.Environ() {
		key, _, _ := strings.Cut(kv, "=")
		if strings.HasPrefix(key, "AI_MEMORY_") {
			t.Setenv(key, "")
			_ = os.Unsetenv(key)
		}
	}
}

// backupFixture cria um diretório temporário com config.toml apontando data_dir
// para <tmp>/data e um caminho de .env inexistente. Devolve o caminho do TOML, o
// do .env e o data_dir.
func backupFixture(t *testing.T) (tmp, cfgPath, envPath, dataDir string) {
	t.Helper()
	isolateBackupEnv(t)
	tmp = t.TempDir()
	dataDir = filepath.Join(tmp, "data")
	cfgPath = filepath.Join(tmp, "config.toml")
	// Literal TOML (aspas simples) para que barras invertidas do Windows não sejam escapes.
	if err := os.WriteFile(cfgPath, []byte("data_dir = '"+dataDir+"'\n"), 0o600); err != nil {
		t.Fatalf("gravar config.toml: %v", err)
	}
	envPath = filepath.Join(tmp, "ausente.env")
	return tmp, cfgPath, envPath, dataDir
}

// runBackupCmd monta a raiz com o subcomando backup e executa com args.
func runBackupCmd(t *testing.T, args ...string) error {
	t.Helper()
	root := NewRootCommand()
	root.AddCommand(NewBackupCommand())
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetArgs(append([]string{"backup"}, args...))
	return root.Execute()
}

func TestBackupGeraArquivoAbrivelComSchema(t *testing.T) {
	tmp, cfgPath, envPath, _ := backupFixture(t)
	dest := filepath.Join(tmp, "snap.sqlite")

	if err := runBackupCmd(t, "--to", dest, "--config", cfgPath, "--env-file", envPath); err != nil {
		t.Fatalf("backup: %v", err)
	}

	// Reabre a cópia como um data_dir próprio: storage.Open espera <dir>/db/memory.sqlite.
	raw, err := os.ReadFile(dest)
	if err != nil {
		t.Fatalf("ler snapshot: %v", err)
	}
	copyDir := filepath.Join(tmp, "copia")
	if err := os.MkdirAll(filepath.Join(copyDir, "db"), 0o700); err != nil {
		t.Fatalf("criar dir da cópia: %v", err)
	}
	if err := os.WriteFile(filepath.Join(copyDir, "db", "memory.sqlite"), raw, 0o600); err != nil {
		t.Fatalf("gravar cópia: %v", err)
	}

	ctx := context.Background()
	db, err := storage.Open(ctx, copyDir)
	if err != nil {
		t.Fatalf("storage.Open do snapshot: %v", err)
	}
	defer func() { _ = db.Close() }()

	var n int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM schema_migrations`).Scan(&n); err != nil {
		t.Fatalf("consultar schema_migrations: %v", err)
	}
	if n == 0 {
		t.Fatal("schema_migrations vazio no snapshot")
	}
}

func TestBackupDestinoExistenteERecusado(t *testing.T) {
	tmp, cfgPath, envPath, _ := backupFixture(t)
	dest := filepath.Join(tmp, "existente.sqlite")
	const conteudo = "preexistente"
	if err := os.WriteFile(dest, []byte(conteudo), 0o600); err != nil {
		t.Fatalf("gravar destino: %v", err)
	}

	err := runBackupCmd(t, "--to", dest, "--config", cfgPath, "--env-file", envPath)
	if err == nil {
		t.Fatal("esperava erro para destino já existente")
	}
	if !strings.Contains(err.Error(), "já existe") {
		t.Fatalf("erro sem indicação de destino existente: %v", err)
	}

	got, rerr := os.ReadFile(dest)
	if rerr != nil {
		t.Fatalf("ler destino: %v", rerr)
	}
	if string(got) != conteudo {
		t.Fatalf("destino existente foi alterado: %q", got)
	}
}

func TestBackupFlagToAusenteEErro(t *testing.T) {
	_, cfgPath, envPath, _ := backupFixture(t)

	err := runBackupCmd(t, "--config", cfgPath, "--env-file", envPath)
	if err == nil {
		t.Fatal("esperava erro sem --to")
	}
	if !strings.Contains(err.Error(), "to") {
		t.Fatalf("erro não cita a flag --to: %v", err)
	}
}

func TestBackupFlagToVaziaEErro(t *testing.T) {
	_, cfgPath, envPath, _ := backupFixture(t)

	err := runBackupCmd(t, "--to", "", "--config", cfgPath, "--env-file", envPath)
	if err == nil {
		t.Fatal("esperava erro com --to vazio")
	}
}

func TestBackupNaoCriaDiretorioPai(t *testing.T) {
	tmp, cfgPath, envPath, _ := backupFixture(t)
	pai := filepath.Join(tmp, "inexistente")
	dest := filepath.Join(pai, "snap.sqlite")

	err := runBackupCmd(t, "--to", dest, "--config", cfgPath, "--env-file", envPath)
	if err == nil {
		t.Fatal("esperava erro com diretório pai inexistente")
	}
	if _, serr := os.Stat(pai); !os.IsNotExist(serr) {
		t.Fatalf("diretório pai foi criado ou stat falhou: %v", serr)
	}
}
