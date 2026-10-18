package cli

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
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

const backupTokenTeste = "tok-backup-teste"

// backupFixture cria um diretório temporário com config.toml apontando data_dir para
// <tmp>/data e server_url para serverURL, com um token bearer de teste. Devolve o caminho
// do TOML e o do .env (inexistente).
func backupFixture(t *testing.T, serverURL string) (tmp, cfgPath, envPath string) {
	t.Helper()
	isolateBackupEnv(t)
	tmp = t.TempDir()
	dataDir := filepath.Join(tmp, "data")
	cfgPath = filepath.Join(tmp, "config.toml")
	// Literal TOML (aspas simples) para que barras invertidas do Windows não sejam escapes.
	cfg := "data_dir = '" + dataDir + "'\n" +
		"server_url = '" + serverURL + "'\n" +
		"[auth]\nbearer_token = '" + backupTokenTeste + "'\n"
	if err := os.WriteFile(cfgPath, []byte(cfg), 0o600); err != nil {
		t.Fatalf("gravar config.toml: %v", err)
	}
	envPath = filepath.Join(tmp, "ausente.env")
	return tmp, cfgPath, envPath
}

// runBackupCmd executa a raiz (que já traz o subcomando backup) com os argumentos dados.
func runBackupCmd(t *testing.T, args ...string) error {
	t.Helper()
	root := NewRootCommand()
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetArgs(append([]string{"backup"}, args...))
	return root.Execute()
}

// tarballFake é o corpo que o servidor fake devolve: não precisa ser um tar real, só
// bytes conhecidos para conferir a gravação byte a byte.
var tarballFake = []byte("\x1f\x8b\x08\x00 corpo-do-tarball-de-teste \x00\xff")

// servidorBackup sobe um servidor fake de POST /admin/backup. O handler recebe a
// requisição e conta as chamadas recebidas.
func servidorBackup(t *testing.T, h http.HandlerFunc) (*httptest.Server, *atomic.Int32) {
	t.Helper()
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		h(w, r)
	}))
	t.Cleanup(srv.Close)
	return srv, &hits
}

// respondeTarball é o caminho feliz do servidor: 200, application/gzip e o corpo fake.
func respondeTarball(t *testing.T) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("método = %s, esperava POST", r.Method)
		}
		if r.URL.Path != "/admin/backup" {
			t.Errorf("caminho = %s, esperava /admin/backup", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer "+backupTokenTeste {
			t.Errorf("Authorization = %q, esperava o token bearer do config", got)
		}
		w.Header().Set("Content-Type", "application/gzip")
		w.Header().Set("Content-Disposition", `attachment; filename="backup.tar.gz"`)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(tarballFake)
	}
}

func TestBackupGravaCorpoDaRespostaDoServidor(t *testing.T) {
	srv, hits := servidorBackup(t, respondeTarball(t))
	tmp, cfgPath, envPath := backupFixture(t, srv.URL)
	dest := filepath.Join(tmp, "backup.tar.gz")

	if err := runBackupCmd(t, "--to", dest, "--config", cfgPath, "--env-file", envPath); err != nil {
		t.Fatalf("backup: %v", err)
	}
	if hits.Load() != 1 {
		t.Fatalf("servidor recebeu %d chamadas, esperava 1", hits.Load())
	}
	got, err := os.ReadFile(dest)
	if err != nil {
		t.Fatalf("ler destino: %v", err)
	}
	if !bytes.Equal(got, tarballFake) {
		t.Fatalf("arquivo gravado difere do corpo do servidor:\nobtido: %q\nesperado: %q", got, tarballFake)
	}
}

func TestBackupAtalhoOGravaCorpoDaRespostaDoServidor(t *testing.T) {
	srv, hits := servidorBackup(t, respondeTarball(t))
	tmp, cfgPath, envPath := backupFixture(t, srv.URL)
	dest := filepath.Join(tmp, "backup.tar.gz")

	if err := runBackupCmd(t, "-o", dest, "--config", cfgPath, "--env-file", envPath); err != nil {
		t.Fatalf("backup -o: %v", err)
	}
	if hits.Load() != 1 {
		t.Fatalf("servidor recebeu %d chamadas, esperava 1", hits.Load())
	}
	got, err := os.ReadFile(dest)
	if err != nil {
		t.Fatalf("ler destino: %v", err)
	}
	if !bytes.Equal(got, tarballFake) {
		t.Fatalf("arquivo gravado difere do corpo do servidor")
	}
}

func TestBackupErroDoServidorNaoGravaArquivo(t *testing.T) {
	srv, _ := servidorBackup(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"error":"árvore da wiki inválida: wiki/x.md: não é UTF-8"}`))
	})
	tmp, cfgPath, envPath := backupFixture(t, srv.URL)
	dest := filepath.Join(tmp, "backup.tar.gz")

	err := runBackupCmd(t, "--to", dest, "--config", cfgPath, "--env-file", envPath)
	if err == nil {
		t.Fatal("esperava erro quando o servidor responde 500")
	}
	if !strings.Contains(err.Error(), "não é UTF-8") {
		t.Fatalf("erro não traz a mensagem do servidor: %v", err)
	}
	if _, serr := os.Stat(dest); !os.IsNotExist(serr) {
		t.Fatalf("destino foi criado apesar do erro do servidor (stat: %v)", serr)
	}
}

func TestBackupErroSemJSONNaoGravaArquivoNemCorpoBinario(t *testing.T) {
	srv, _ := servidorBackup(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/gzip")
		w.WriteHeader(http.StatusBadGateway)
		_, _ = w.Write(tarballFake)
	})
	tmp, cfgPath, envPath := backupFixture(t, srv.URL)
	dest := filepath.Join(tmp, "backup.tar.gz")

	err := runBackupCmd(t, "--to", dest, "--config", cfgPath, "--env-file", envPath)
	if err == nil {
		t.Fatal("esperava erro com status 502")
	}
	if strings.Contains(err.Error(), "\x1f\x8b") {
		t.Fatalf("mensagem de erro contém corpo binário: %q", err.Error())
	}
	if _, serr := os.Stat(dest); !os.IsNotExist(serr) {
		t.Fatalf("destino foi criado apesar do erro (stat: %v)", serr)
	}
}

func TestBackupServidorInacessivelNaoGravaArquivo(t *testing.T) {
	srv, _ := servidorBackup(t, respondeTarball(t))
	url := srv.URL
	srv.Close()
	tmp, cfgPath, envPath := backupFixture(t, url)
	dest := filepath.Join(tmp, "backup.tar.gz")

	if err := runBackupCmd(t, "--to", dest, "--config", cfgPath, "--env-file", envPath); err == nil {
		t.Fatal("esperava erro com servidor fora do ar")
	}
	if _, serr := os.Stat(dest); !os.IsNotExist(serr) {
		t.Fatalf("destino foi criado com servidor fora do ar (stat: %v)", serr)
	}
}

func TestBackupDestinoExistenteERecusadoSemPedirAoServidor(t *testing.T) {
	srv, hits := servidorBackup(t, respondeTarball(t))
	tmp, cfgPath, envPath := backupFixture(t, srv.URL)
	dest := filepath.Join(tmp, "existente.tar.gz")
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
	if hits.Load() != 0 {
		t.Fatalf("servidor foi chamado %d vezes com destino existente", hits.Load())
	}
	got, rerr := os.ReadFile(dest)
	if rerr != nil {
		t.Fatalf("ler destino: %v", rerr)
	}
	if string(got) != conteudo {
		t.Fatalf("destino existente foi alterado: %q", got)
	}
}

func TestBackupNaoCriaDiretorioPai(t *testing.T) {
	srv, hits := servidorBackup(t, respondeTarball(t))
	tmp, cfgPath, envPath := backupFixture(t, srv.URL)
	pai := filepath.Join(tmp, "inexistente")
	dest := filepath.Join(pai, "backup.tar.gz")

	err := runBackupCmd(t, "--to", dest, "--config", cfgPath, "--env-file", envPath)
	if err == nil {
		t.Fatal("esperava erro com diretório pai inexistente")
	}
	if _, serr := os.Stat(pai); !os.IsNotExist(serr) {
		t.Fatalf("diretório pai foi criado ou stat falhou: %v", serr)
	}
	if hits.Load() != 0 {
		t.Fatalf("servidor foi chamado %d vezes com diretório pai inexistente", hits.Load())
	}
}

func TestBackupFlagToAusenteEErro(t *testing.T) {
	_, cfgPath, envPath := backupFixture(t, "http://127.0.0.1:1")

	err := runBackupCmd(t, "--config", cfgPath, "--env-file", envPath)
	if err == nil {
		t.Fatal("esperava erro sem --to")
	}
	if !strings.Contains(err.Error(), "to") {
		t.Fatalf("erro não cita a flag --to: %v", err)
	}
}

func TestBackupFlagToVaziaEErro(t *testing.T) {
	_, cfgPath, envPath := backupFixture(t, "http://127.0.0.1:1")

	err := runBackupCmd(t, "--to", "", "--config", cfgPath, "--env-file", envPath)
	if err == nil {
		t.Fatal("esperava erro com --to vazio")
	}
}
