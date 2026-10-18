package cli

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/phm-aguiar/ai-memory-go/internal/config"
	"github.com/phm-aguiar/ai-memory-go/internal/storage"
)

// adminBackupApp monta um App real sobre um data_dir temporário, com config.toml
// escrito pelo próprio teste. Devolve o App, o caminho do TOML e o data_dir.
func adminBackupApp(t *testing.T) (*App, string, string) {
	t.Helper()
	for _, kv := range os.Environ() {
		key, _, _ := strings.Cut(kv, "=")
		if strings.HasPrefix(key, "AI_MEMORY_") {
			t.Setenv(key, "")
			_ = os.Unsetenv(key)
		}
	}
	tmp := t.TempDir()
	dataDir := filepath.Join(tmp, "data")
	cfgPath := filepath.Join(tmp, "config.toml")
	// Aspas simples no TOML para que as barras do Windows não sejam escapes.
	if err := os.WriteFile(cfgPath, []byte("data_dir = '"+dataDir+"'\n"), 0o600); err != nil {
		t.Fatalf("gravar config.toml: %v", err)
	}
	envPath := filepath.Join(tmp, "ausente.env")

	app, err := Compose(context.Background(), config.Options{ConfigFile: cfgPath, EnvFile: envPath})
	if err != nil {
		t.Fatalf("Compose: %v", err)
	}
	t.Cleanup(func() { _ = app.Close() })
	return app, cfgPath, dataDir
}

// postAdminBackup chama o handler com POST e devolve a resposta gravada.
func postAdminBackup(t *testing.T, h http.Handler) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/admin/backup", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

// readTarGz devolve as entradas regulares e diretórios do tar.gz (nome -> conteúdo).
func readTarGz(t *testing.T, body []byte) map[string][]byte {
	t.Helper()
	gz, err := gzip.NewReader(bytes.NewReader(body))
	if err != nil {
		t.Fatalf("abrir gzip: %v", err)
	}
	defer func() { _ = gz.Close() }()
	tr := tar.NewReader(gz)
	entries := map[string][]byte{}
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("ler tar: %v", err)
		}
		if hdr.Typeflag == tar.TypeDir {
			entries[hdr.Name] = nil
			continue
		}
		data, err := io.ReadAll(tr)
		if err != nil {
			t.Fatalf("ler entrada %s: %v", hdr.Name, err)
		}
		entries[hdr.Name] = data
	}
	return entries
}

func TestAdminBackupGeraTarGzComWikiDbEConfig(t *testing.T) {
	app, cfgPath, dataDir := adminBackupApp(t)

	wikiDir := filepath.Join(dataDir, "wiki", "notas")
	if err := os.MkdirAll(wikiDir, 0o700); err != nil {
		t.Fatalf("criar wiki: %v", err)
	}
	pagina := "---\ntitle: Nota\n---\ncorpo da nota\n"
	if err := os.WriteFile(filepath.Join(wikiDir, "teste.md"), []byte(pagina), 0o600); err != nil {
		t.Fatalf("gravar página: %v", err)
	}
	if err := os.WriteFile(filepath.Join(wikiDir, "anexo.txt"), []byte("anexo"), 0o600); err != nil {
		t.Fatalf("gravar anexo: %v", err)
	}
	cfgBytes, err := os.ReadFile(cfgPath)
	if err != nil {
		t.Fatalf("ler config: %v", err)
	}

	rec := postAdminBackup(t, NewAdminBackupHandler(app, cfgPath))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, corpo = %s", rec.Code, rec.Body.String())
	}
	if ct := rec.Header().Get("Content-Type"); ct != "application/gzip" {
		t.Fatalf("Content-Type = %q, quer application/gzip", ct)
	}

	entries := readTarGz(t, rec.Body.Bytes())
	for _, name := range []string{"wiki/", "wiki/notas/teste.md", "wiki/notas/anexo.txt", "db/memory.sqlite", "config.toml"} {
		if _, ok := entries[name]; !ok {
			t.Errorf("entrada %q ausente do tar; presentes: %v", name, keys(entries))
		}
	}
	if got := string(entries["wiki/notas/teste.md"]); got != pagina {
		t.Errorf("conteúdo da página = %q, quer %q", got, pagina)
	}
	if !bytes.Equal(entries["config.toml"], cfgBytes) {
		t.Errorf("config.toml do tar difere do original")
	}

	// O snapshot deve ser um banco SQLite utilizável: reabre como data_dir próprio.
	db := entries["db/memory.sqlite"]
	if !bytes.HasPrefix(db, []byte("SQLite format 3\x00")) {
		t.Fatalf("db/memory.sqlite não tem cabeçalho SQLite")
	}
	copyDir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(copyDir, "db"), 0o700); err != nil {
		t.Fatalf("criar dir da cópia: %v", err)
	}
	if err := os.WriteFile(filepath.Join(copyDir, "db", "memory.sqlite"), db, 0o600); err != nil {
		t.Fatalf("gravar cópia do banco: %v", err)
	}
	ctx := context.Background()
	snapDB, err := storage.Open(ctx, copyDir)
	if err != nil {
		t.Fatalf("storage.Open do snapshot: %v", err)
	}
	if err := snapDB.Close(); err != nil {
		t.Fatalf("fechar snapshot: %v", err)
	}
}

func TestAdminBackupSemConfigTomlOmiteEntrada(t *testing.T) {
	app, _, _ := adminBackupApp(t)
	ausente := filepath.Join(t.TempDir(), "nao-existe.toml")

	rec := postAdminBackup(t, NewAdminBackupHandler(app, ausente))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, corpo = %s", rec.Code, rec.Body.String())
	}
	entries := readTarGz(t, rec.Body.Bytes())
	if _, ok := entries["config.toml"]; ok {
		t.Errorf("config.toml presente sem o arquivo existir")
	}
	if _, ok := entries["db/memory.sqlite"]; !ok {
		t.Errorf("db/memory.sqlite ausente do tar")
	}
}

func TestAdminBackupArvoreInvalidaResponde500JSON(t *testing.T) {
	casos := []struct {
		nome     string
		conteudo []byte
		esperado string
	}{
		{"utf8", []byte{0xff, 0xfe, 'x'}, "não é UTF-8"},
		{"frontmatter", []byte("---\na: [\n---\ncorpo\n"), "frontmatter inválido"},
	}
	for _, c := range casos {
		t.Run(c.nome, func(t *testing.T) {
			app, cfgPath, dataDir := adminBackupApp(t)
			dir := filepath.Join(dataDir, "wiki", "notas")
			if err := os.MkdirAll(dir, 0o700); err != nil {
				t.Fatalf("criar wiki: %v", err)
			}
			if err := os.WriteFile(filepath.Join(dir, "ruim.md"), c.conteudo, 0o600); err != nil {
				t.Fatalf("gravar página ruim: %v", err)
			}

			rec := postAdminBackup(t, NewAdminBackupHandler(app, cfgPath))
			if rec.Code != http.StatusInternalServerError {
				t.Fatalf("status = %d, quer 500", rec.Code)
			}
			if ct := rec.Header().Get("Content-Type"); ct != "application/json" {
				t.Fatalf("Content-Type = %q, quer application/json", ct)
			}
			var corpo map[string]string
			if err := json.Unmarshal(rec.Body.Bytes(), &corpo); err != nil {
				t.Fatalf("corpo não é JSON: %v; corpo = %s", err, rec.Body.String())
			}
			msg := corpo["error"]
			if !strings.Contains(msg, "árvore da wiki inválida") || !strings.Contains(msg, "ruim.md") ||
				!strings.Contains(msg, c.esperado) {
				t.Errorf("mensagem de erro = %q", msg)
			}
			if strings.Contains(rec.Body.String(), dataDir) {
				t.Errorf("corpo de erro expõe caminho do data_dir: %s", rec.Body.String())
			}
		})
	}
}

func TestAdminBackupAceitaSomentePOST(t *testing.T) {
	app, cfgPath, _ := adminBackupApp(t)
	req := httptest.NewRequest(http.MethodGet, "/admin/backup", nil)
	rec := httptest.NewRecorder()
	NewAdminBackupHandler(app, cfgPath).ServeHTTP(rec, req)
	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d, quer 405", rec.Code)
	}
	if allow := rec.Header().Get("Allow"); allow != http.MethodPost {
		t.Errorf("Allow = %q, quer POST", allow)
	}
}

// keys lista os nomes do tar para as mensagens de falha.
func keys(m map[string][]byte) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
