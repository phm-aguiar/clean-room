package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// installMCPEnv são os caminhos de um teste de install-mcp. Tudo fica em um
// diretório temporário. AI_MEMORY_* do host são removidos, e o diretório pessoal
// é o temporário, para que ~/.claude.json caia dentro do teste.
type installMCPEnv struct {
	data    string
	home    string
	claude  string
	cfg     string
	envFile string
}

// newInstallMCPEnv prepara os diretórios e aponta AI_MEMORY_HOME para o
// temporário. CLAUDE_CONFIG_DIR fica vazio (equivale a não definido).
func newInstallMCPEnv(t *testing.T) installMCPEnv {
	t.Helper()
	isolateReindexEnv(t)
	tmp := t.TempDir()
	e := installMCPEnv{
		data:    filepath.Join(tmp, "data"),
		home:    filepath.Join(tmp, "home"),
		claude:  filepath.Join(tmp, "claude"),
		cfg:     filepath.Join(tmp, "ausente.toml"),
		envFile: filepath.Join(tmp, "ausente.env"),
	}
	for _, d := range []string{e.data, e.home} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatalf("criar %s: %v", d, err)
		}
	}
	t.Setenv("AI_MEMORY_HOME", e.home)
	t.Setenv("CLAUDE_CONFIG_DIR", "")
	return e
}

// runInstallMCPCmd executa "install-mcp" pela raiz da CLI, com as flags globais
// apontando para o ambiente do teste. Devolve a saída combinada.
func runInstallMCPCmd(t *testing.T, e installMCPEnv, args ...string) (string, error) {
	t.Helper()
	root := NewRootCommand()
	root.AddCommand(NewInstallMCPCommand())
	var buf bytes.Buffer
	root.SetOut(&buf)
	root.SetErr(&buf)
	full := append([]string{
		"--data-dir", e.data, "--config", e.cfg, "--env-file", e.envFile, "install-mcp",
	}, args...)
	root.SetArgs(full)
	err := root.Execute()
	return buf.String(), err
}

// writeHookTokenForMCP grava o token do install-hooks (hook-token), como T049.
func writeHookTokenForMCP(t *testing.T, e installMCPEnv, token string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(e.data, hookTokenFile), []byte(token), 0o600); err != nil {
		t.Fatalf("gravar %s: %v", hookTokenFile, err)
	}
}

// readClaudeJSON lê o arquivo como objeto genérico.
func readClaudeJSON(t *testing.T, path string) map[string]any {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ler %s: %v", path, err)
	}
	var doc map[string]any
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatalf("JSON inválido em %s: %v", path, err)
	}
	return doc
}

// aiMemoryEntry devolve mcpServers["ai-memory"] do documento.
func aiMemoryEntry(t *testing.T, doc map[string]any) map[string]any {
	t.Helper()
	servers, ok := doc["mcpServers"].(map[string]any)
	if !ok {
		t.Fatalf("mcpServers ausente ou não é objeto: %v", doc["mcpServers"])
	}
	entry, ok := servers["ai-memory"].(map[string]any)
	if !ok {
		t.Fatalf("mcpServers.ai-memory ausente: %v", servers)
	}
	return entry
}

// TestInstallMCPPrintsWithoutWriting: sem --apply, a saída traz o bloco e nenhum
// arquivo é criado.
func TestInstallMCPPrintsWithoutWriting(t *testing.T) {
	e := newInstallMCPEnv(t)

	out, err := runInstallMCPCmd(t, e, "--client", "claude-code")
	if err != nil {
		t.Fatalf("install-mcp sem --apply: %v\n%s", err, out)
	}
	for _, want := range []string{`"mcpServers"`, `"ai-memory"`, `"type": "http"`, `"url": "http://127.0.0.1:49374/mcp"`, "simulação: nada foi gravado"} {
		if !strings.Contains(out, want) {
			t.Errorf("saída sem %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "headers") {
		t.Errorf("sem token não deve haver cabeçalho:\n%s", out)
	}
	if _, err := os.Stat(filepath.Join(e.home, claudeJSONFile)); !os.IsNotExist(err) {
		t.Errorf(".claude.json criado sem --apply (err=%v)", err)
	}
}

// TestInstallMCPPrintHidesToken: com token no hook-token, a impressão mostra o
// placeholder, não o valor, e continua sem gravar.
func TestInstallMCPPrintHidesToken(t *testing.T) {
	e := newInstallMCPEnv(t)
	const token = "aim_PRINTSECRET789"
	writeHookTokenForMCP(t, e, token)

	out, err := runInstallMCPCmd(t, e)
	if err != nil {
		t.Fatalf("install-mcp: %v\n%s", err, out)
	}
	if strings.Contains(out, token) {
		t.Errorf("o token apareceu na impressão:\n%s", out)
	}
	if !strings.Contains(out, "Bearer "+mcpTokenPlaceholder) {
		t.Errorf("falta o placeholder no cabeçalho:\n%s", out)
	}
	if _, err := os.Stat(filepath.Join(e.home, claudeJSONFile)); !os.IsNotExist(err) {
		t.Errorf(".claude.json criado sem --apply (err=%v)", err)
	}
}

// TestInstallMCPApplyPreservesOtherKeysAndServers: --apply troca só
// mcpServers["ai-memory"]. Outras chaves, outros servidores e a entrada de outro
// nome ficam iguais.
func TestInstallMCPApplyPreservesOtherKeysAndServers(t *testing.T) {
	e := newInstallMCPEnv(t)
	path := filepath.Join(e.home, claudeJSONFile)
	existing := `{
  "numStartups": 7,
  "theme": "dark",
  "mcpServers": {
    "other": {"type": "stdio", "command": "other-bin", "args": ["x"]},
    "ai-memory": {"type": "http", "url": "http://old-host:49374/mcp"}
  }
}
`
	if err := os.WriteFile(path, []byte(existing), 0o600); err != nil {
		t.Fatalf("gravar .claude.json: %v", err)
	}

	out, err := runInstallMCPCmd(t, e, "--apply")
	if err != nil {
		t.Fatalf("--apply: %v\n%s", err, out)
	}
	if !strings.Contains(out, "updated") {
		t.Errorf("resultado esperado updated:\n%s", out)
	}

	doc := readClaudeJSON(t, path)
	if doc["numStartups"] != float64(7) || doc["theme"] != "dark" {
		t.Errorf("chaves de topo não preservadas: %v", doc)
	}
	servers := doc["mcpServers"].(map[string]any)
	other, ok := servers["other"].(map[string]any)
	if !ok || other["command"] != "other-bin" {
		t.Errorf("servidor de terceiros alterado: %v", servers["other"])
	}
	entry := aiMemoryEntry(t, doc)
	if entry["url"] != "http://127.0.0.1:49374/mcp" {
		t.Errorf("url da entrada = %v, quer o endereço padrão com /mcp", entry["url"])
	}
	if _, has := entry["headers"]; has {
		t.Errorf("sem token não deve haver headers: %v", entry)
	}
	if n := countBakFiles(t, e.home); n != 1 {
		t.Errorf("backups = %d, quer 1", n)
	}
}

// TestInstallMCPHonorsClaudeConfigDir: com CLAUDE_CONFIG_DIR, o arquivo vai para
// $CLAUDE_CONFIG_DIR/.claude.json e ~/.claude.json não é tocado.
func TestInstallMCPHonorsClaudeConfigDir(t *testing.T) {
	e := newInstallMCPEnv(t)
	if err := os.MkdirAll(e.claude, 0o755); err != nil {
		t.Fatalf("criar %s: %v", e.claude, err)
	}
	t.Setenv("CLAUDE_CONFIG_DIR", e.claude)

	out, err := runInstallMCPCmd(t, e, "--apply")
	if err != nil {
		t.Fatalf("--apply com CLAUDE_CONFIG_DIR: %v\n%s", err, out)
	}
	if !strings.Contains(out, filepath.Join(e.claude, claudeJSONFile)) {
		t.Errorf("saída não aponta para o diretório de CLAUDE_CONFIG_DIR:\n%s", out)
	}
	doc := readClaudeJSON(t, filepath.Join(e.claude, claudeJSONFile))
	aiMemoryEntry(t, doc)
	if _, err := os.Stat(filepath.Join(e.home, claudeJSONFile)); !os.IsNotExist(err) {
		t.Errorf("~/.claude.json foi tocado com CLAUDE_CONFIG_DIR definido (err=%v)", err)
	}
}

// TestInstallMCPHeaderOnlyToken: o cabeçalho tem só Authorization com o token.
// O token não aparece em argumentos nem na saída. Ele vai só para o arquivo.
func TestInstallMCPHeaderOnlyToken(t *testing.T) {
	e := newInstallMCPEnv(t)
	const token = "aim_HEADERSECRET456"
	writeHookTokenForMCP(t, e, token)
	args := []string{"--apply"}

	out, err := runInstallMCPCmd(t, e, args...)
	if err != nil {
		t.Fatalf("--apply: %v\n%s", err, out)
	}
	for _, a := range args {
		if strings.Contains(a, token) {
			t.Errorf("token na linha de comando: %q", a)
		}
	}
	if strings.Contains(out, token) {
		t.Errorf("o token apareceu na saída:\n%s", out)
	}

	doc := readClaudeJSON(t, filepath.Join(e.home, claudeJSONFile))
	entry := aiMemoryEntry(t, doc)
	headers, ok := entry["headers"].(map[string]any)
	if !ok {
		t.Fatalf("headers ausente: %v", entry)
	}
	if len(headers) != 1 {
		t.Errorf("headers = %v, quer só Authorization", headers)
	}
	if headers["Authorization"] != "Bearer "+token {
		t.Errorf("Authorization = %v, quer Bearer com o token", headers["Authorization"])
	}
}

// TestInstallMCPTokenFromConfigWins: AI_MEMORY_AUTH_TOKEN tem prioridade sobre o
// hook-token. A origem aparece na saída, sem o valor.
func TestInstallMCPTokenFromConfigWins(t *testing.T) {
	e := newInstallMCPEnv(t)
	writeHookTokenForMCP(t, e, "aim_FILETOKEN")
	const envToken = "aim_ENVTOKEN999"
	t.Setenv("AI_MEMORY_AUTH_TOKEN", envToken)

	out, err := runInstallMCPCmd(t, e, "--apply")
	if err != nil {
		t.Fatalf("--apply: %v\n%s", err, out)
	}
	if strings.Contains(out, envToken) {
		t.Errorf("o token apareceu na saída:\n%s", out)
	}
	entry := aiMemoryEntry(t, readClaudeJSON(t, filepath.Join(e.home, claudeJSONFile)))
	headers := entry["headers"].(map[string]any)
	if headers["Authorization"] != "Bearer "+envToken {
		t.Errorf("Authorization = %v, quer o token da configuração", headers["Authorization"])
	}
}

// TestInstallMCPApplyIdempotent: repetir --apply dá no-op, não muda o arquivo e
// não cria mais backup.
func TestInstallMCPApplyIdempotent(t *testing.T) {
	e := newInstallMCPEnv(t)
	path := filepath.Join(e.home, claudeJSONFile)
	if err := os.WriteFile(path, []byte(`{"numStartups": 3}`), 0o600); err != nil {
		t.Fatalf("gravar .claude.json: %v", err)
	}

	if out, err := runInstallMCPCmd(t, e, "--apply"); err != nil {
		t.Fatalf("primeiro --apply: %v\n%s", err, out)
	}
	first, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ler: %v", err)
	}
	backups := countBakFiles(t, e.home)

	out, err := runInstallMCPCmd(t, e, "--apply")
	if err != nil {
		t.Fatalf("segundo --apply: %v\n%s", err, out)
	}
	if !strings.Contains(out, "no-op") {
		t.Errorf("segundo --apply deveria ser no-op:\n%s", out)
	}
	second, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ler: %v", err)
	}
	if !bytes.Equal(first, second) {
		t.Errorf("arquivo mudou no segundo --apply")
	}
	if n := countBakFiles(t, e.home); n != backups {
		t.Errorf("backups = %d, quer %d (sem novo backup)", n, backups)
	}
}

// TestInstallMCPSessionAware: com --session-aware, a entrada é a ponte stdio. O
// token vai para env, e não para headers.
func TestInstallMCPSessionAware(t *testing.T) {
	e := newInstallMCPEnv(t)
	const token = "aim_BRIDGESECRET"
	writeHookTokenForMCP(t, e, token)

	out, err := runInstallMCPCmd(t, e, "--session-aware", "--server-url", "http://192.168.0.90:49374", "--apply")
	if err != nil {
		t.Fatalf("--session-aware --apply: %v\n%s", err, out)
	}
	if strings.Contains(out, token) {
		t.Errorf("o token apareceu na saída:\n%s", out)
	}
	entry := aiMemoryEntry(t, readClaudeJSON(t, filepath.Join(e.home, claudeJSONFile)))
	if entry["type"] != "stdio" || entry["command"] != "ai-memory" {
		t.Errorf("entrada não é a ponte stdio: %v", entry)
	}
	args, _ := entry["args"].([]any)
	if len(args) != 3 || args[0] != "mcp-bridge" || args[1] != "--server-url" || args[2] != "http://192.168.0.90:49374/mcp" {
		t.Errorf("args = %v", args)
	}
	if _, has := entry["headers"]; has {
		t.Errorf("ponte não deve ter headers: %v", entry)
	}
	envMap, _ := entry["env"].(map[string]any)
	if envMap["AI_MEMORY_AUTH_TOKEN"] != token {
		t.Errorf("env = %v, quer AI_MEMORY_AUTH_TOKEN com o token", envMap)
	}
}

// TestInstallMCPRefusesBrokenConfig: .claude.json inválido gera erro e não é
// alterado nem recebe backup.
func TestInstallMCPRefusesBrokenConfig(t *testing.T) {
	e := newInstallMCPEnv(t)
	path := filepath.Join(e.home, claudeJSONFile)
	broken := "{ invalido"
	if err := os.WriteFile(path, []byte(broken), 0o600); err != nil {
		t.Fatalf("gravar: %v", err)
	}

	if _, err := runInstallMCPCmd(t, e, "--apply"); err == nil {
		t.Fatal("--apply com JSON inválido deveria falhar")
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ler: %v", err)
	}
	if string(raw) != broken {
		t.Errorf("arquivo inválido foi alterado: %q", raw)
	}
	if n := countBakFiles(t, e.home); n != 0 {
		t.Errorf("backups = %d, quer 0", n)
	}
}

// TestInstallMCPRejectsOtherClient: só claude-code existe nesta versão.
func TestInstallMCPRejectsOtherClient(t *testing.T) {
	e := newInstallMCPEnv(t)
	if _, err := runInstallMCPCmd(t, e, "--client", "cursor"); err == nil {
		t.Fatal("cliente cursor deveria ser recusado")
	}
}

// TestInstallMCPEndpointNormalization: o /mcp é acrescentado uma vez, e a forma
// normalizada não muda ao ser repetida.
func TestInstallMCPEndpointNormalization(t *testing.T) {
	cases := map[string]string{
		"http://h:49374":        "http://h:49374/mcp",
		"http://h:49374/":       "http://h:49374/mcp",
		"http://h:49374/mcp":    "http://h:49374/mcp",
		"http://h:49374/mcp/":   "http://h:49374/mcp",
		"https://h.example/mcp": "https://h.example/mcp",
	}
	for in, want := range cases {
		got, err := mcpEndpoint(in)
		if err != nil {
			t.Errorf("mcpEndpoint(%q): %v", in, err)
			continue
		}
		if got != want {
			t.Errorf("mcpEndpoint(%q) = %q, quer %q", in, got, want)
		}
		again, err := mcpEndpoint(got)
		if err != nil || again != got {
			t.Errorf("mcpEndpoint não é idempotente para %q: %q (err=%v)", in, again, err)
		}
	}
	if _, err := mcpEndpoint("ftp://h/mcp"); err == nil {
		t.Error("esquema ftp deveria ser recusado")
	}
}
