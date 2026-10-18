package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// installHooksEnv são os caminhos de um teste de install-hooks. Tudo fica em um
// diretório temporário, e as variáveis AI_MEMORY_* do host são removidas.
type installHooksEnv struct {
	data     string
	claude   string
	settings string
	cfg      string
	envFile  string
}

// newInstallHooksEnv prepara os diretórios, aponta CLAUDE_CONFIG_DIR para o
// temporário, fixa o binário do hook e o instante do backup.
func newInstallHooksEnv(t *testing.T) installHooksEnv {
	t.Helper()
	isolateReindexEnv(t)
	tmp := t.TempDir()
	e := installHooksEnv{
		data:    filepath.Join(tmp, "data"),
		claude:  filepath.Join(tmp, "claude"),
		cfg:     filepath.Join(tmp, "ausente.toml"),
		envFile: filepath.Join(tmp, "ausente.env"),
	}
	e.settings = filepath.Join(e.claude, "settings.json")
	for _, d := range []string{e.data, e.claude} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatalf("criar %s: %v", d, err)
		}
	}
	t.Setenv("CLAUDE_CONFIG_DIR", e.claude)

	prevExe := installHooksExecutable
	installHooksExecutable = func() (string, error) { return "/opt/ai-memory/bin/ai-memory", nil }
	t.Cleanup(func() { installHooksExecutable = prevExe })

	// Cada backup ganha um instante novo: o primeiro é 1700000001.
	prevNow := nowUnix
	clock := int64(1700000000)
	nowUnix = func() int64 { clock++; return clock }
	t.Cleanup(func() { nowUnix = prevNow })
	return e
}

// runInstallHooksCmd executa "install-hooks" pela raiz da CLI, com as flags globais
// apontando para o ambiente do teste. Devolve a saída combinada.
func runInstallHooksCmd(t *testing.T, e installHooksEnv, args ...string) (string, error) {
	t.Helper()
	root := NewRootCommand()
	root.AddCommand(NewInstallHooksCommand())
	var buf bytes.Buffer
	root.SetOut(&buf)
	root.SetErr(&buf)
	full := append([]string{
		"--data-dir", e.data, "--config", e.cfg, "--env-file", e.envFile, "install-hooks",
	}, args...)
	root.SetArgs(full)
	err := root.Execute()
	return buf.String(), err
}

// settingsDoc é a parte do settings.json que os testes leem.
type settingsDoc struct {
	Model string                     `json:"model"`
	Hooks map[string][]settingsGroup `json:"hooks"`
}

type settingsGroup struct {
	Matcher string            `json:"matcher"`
	Hooks   []settingsHandler `json:"hooks"`
}

type settingsHandler struct {
	Type    string `json:"type"`
	Command string `json:"command"`
}

func readSettingsDoc(t *testing.T, path string) settingsDoc {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ler settings: %v", err)
	}
	var doc settingsDoc
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatalf("settings inválido: %v\n%s", err, raw)
	}
	return doc
}

// ownedCount conta os handlers do ai-memory no evento.
func ownedCount(doc settingsDoc, event string) int {
	n := 0
	for _, g := range doc.Hooks[event] {
		for _, h := range g.Hooks {
			if isAiMemoryClaudeHook(h.Command) {
				n++
			}
		}
	}
	return n
}

// thirdPartyCommands lista os comandos que não são do ai-memory.
func thirdPartyCommands(doc settingsDoc) []string {
	var out []string
	for _, groups := range doc.Hooks {
		for _, g := range groups {
			for _, h := range g.Hooks {
				if !isAiMemoryClaudeHook(h.Command) {
					out = append(out, h.Command)
				}
			}
		}
	}
	return out
}

// ownedCommandsOf devolve os comandos do ai-memory no evento.
func ownedCommandsOf(doc settingsDoc, event string) []string {
	var out []string
	for _, g := range doc.Hooks[event] {
		for _, h := range g.Hooks {
			if isAiMemoryClaudeHook(h.Command) {
				out = append(out, h.Command)
			}
		}
	}
	return out
}

// thirdPartySettings é um settings.json com entradas de terceiros nos eventos que
// o ai-memory também usa, e uma chave que não é de hooks.
const thirdPartySettings = `{
  "model": "opus",
  "hooks": {
    "SessionStart": [
      {"matcher": "startup", "hooks": [{"type": "command", "command": "/usr/bin/other-tool start"}]}
    ],
    "PreToolUse": [
      {"matcher": "Bash", "hooks": [{"type": "command", "command": "/usr/bin/guard --check"}]}
    ],
    "UserPromptSubmit": [
      {"hooks": [{"type": "command", "command": "/usr/bin/prompt-thing"}]}
    ]
  }
}
`

// TestInstallHooksPrintsNineEventsWithoutWriting: sem --apply, imprime os 9
// eventos e não cria nada no disco.
func TestInstallHooksPrintsNineEventsWithoutWriting(t *testing.T) {
	e := newInstallHooksEnv(t)

	out, err := runInstallHooksCmd(t, e, "--agent", "claude-code")
	if err != nil {
		t.Fatalf("install-hooks sem --apply: %v\n%s", err, out)
	}
	if !strings.Contains(out, "simulação: nada foi gravado") {
		t.Errorf("a saída não avisa que é simulação:\n%s", out)
	}

	idx := strings.Index(out, "{")
	if idx < 0 {
		t.Fatalf("a saída não traz o bloco JSON:\n%s", out)
	}
	var frag struct {
		Hooks map[string]json.RawMessage `json:"hooks"`
	}
	if err := json.Unmarshal([]byte(out[idx:]), &frag); err != nil {
		t.Fatalf("bloco impresso não é JSON: %v\n%s", err, out)
	}
	want := []string{"SessionStart", "UserPromptSubmit", "PreToolUse", "PostToolUse",
		"PreCompact", "Stop", "SessionEnd", "SubagentStart", "SubagentStop"}
	if len(frag.Hooks) != len(want) {
		t.Errorf("eventos impressos = %d, quer %d", len(frag.Hooks), len(want))
	}
	for _, ev := range want {
		if _, ok := frag.Hooks[ev]; !ok {
			t.Errorf("evento %s ausente da impressão", ev)
		}
	}
	if !strings.Contains(out, "hook --event session-start --agent claude-code") {
		t.Errorf("comando do hook não aponta para o binário certo:\n%s", out)
	}

	if _, err := os.Stat(e.settings); !os.IsNotExist(err) {
		t.Errorf("settings.json foi criado sem --apply (err=%v)", err)
	}
	entries, err := os.ReadDir(e.data)
	if err != nil {
		t.Fatalf("ler diretório de dados: %v", err)
	}
	if len(entries) != 0 {
		t.Errorf("o diretório de dados foi tocado sem --apply: %d entradas", len(entries))
	}
}

// TestInstallHooksApplyMergesThirdParty: --apply acrescenta os 9 eventos, mantém
// as entradas de terceiros e as outras chaves, e guarda backup do arquivo antigo.
func TestInstallHooksApplyMergesThirdParty(t *testing.T) {
	e := newInstallHooksEnv(t)
	if err := os.WriteFile(e.settings, []byte(thirdPartySettings), 0o600); err != nil {
		t.Fatalf("gravar settings de terceiros: %v", err)
	}

	out, err := runInstallHooksCmd(t, e, "--apply")
	if err != nil {
		t.Fatalf("install-hooks --apply: %v\n%s", err, out)
	}
	if !strings.Contains(out, "Written") {
		t.Errorf("primeira gravação deveria dizer Written:\n%s", out)
	}

	doc := readSettingsDoc(t, e.settings)
	if doc.Model != "opus" {
		t.Errorf("chave não relacionada perdida: model=%q", doc.Model)
	}
	for _, ev := range []string{"SessionStart", "UserPromptSubmit", "PreToolUse", "PostToolUse",
		"PreCompact", "Stop", "SessionEnd", "SubagentStart", "SubagentStop"} {
		if n := ownedCount(doc, ev); n != 1 {
			t.Errorf("evento %s: handlers do ai-memory = %d, quer 1", ev, n)
		}
	}
	third := strings.Join(thirdPartyCommands(doc), "\n")
	for _, cmd := range []string{"/usr/bin/other-tool start", "/usr/bin/guard --check", "/usr/bin/prompt-thing"} {
		if !strings.Contains(third, cmd) {
			t.Errorf("entrada de terceiros perdida: %s", cmd)
		}
	}
	if doc.Hooks["PreToolUse"][0].Matcher != "Bash" && doc.Hooks["PreToolUse"][0].Matcher != "*" {
		t.Errorf("matcher de terceiro alterado: %q", doc.Hooks["PreToolUse"][0].Matcher)
	}

	bak := e.settings + ".bak-1700000001"
	raw, err := os.ReadFile(bak)
	if err != nil {
		t.Fatalf("backup não criado: %v", err)
	}
	if string(raw) != thirdPartySettings {
		t.Errorf("backup diferente do arquivo original")
	}
}

// TestInstallHooksReusesCaptureOptions: as escolhas gravadas num --apply são
// reaproveitadas pelo próximo --apply que as omite, e removidas só quando
// explicitamente trocadas.
func TestInstallHooksReusesCaptureOptions(t *testing.T) {
	e := newInstallHooksEnv(t)
	if err := os.WriteFile(e.settings, []byte(thirdPartySettings), 0o600); err != nil {
		t.Fatalf("gravar settings de terceiros: %v", err)
	}

	if _, err := runInstallHooksCmd(t, e, "--apply", "--capture-mode", "allowlist",
		"--no-capture-prompts", "--project-strategy", "repo-root"); err != nil {
		t.Fatalf("primeiro --apply: %v", err)
	}
	doc := readSettingsDoc(t, e.settings)
	if n := ownedCount(doc, "UserPromptSubmit"); n != 0 {
		t.Errorf("prompts desligados mas há %d handler(s) do ai-memory em UserPromptSubmit", n)
	}
	if !strings.Contains(strings.Join(thirdPartyCommands(doc), "\n"), "/usr/bin/prompt-thing") {
		t.Errorf("terceiro em UserPromptSubmit foi removido ao desligar prompts")
	}
	for _, cmd := range ownedCommandsOf(doc, "PostToolUse") {
		if !strings.Contains(cmd, "--project-strategy repo-root") {
			t.Errorf("repo-root não gravado no comando: %s", cmd)
		}
	}

	// Um --apply posterior que omite as opções mantém o estado.
	out, err := runInstallHooksCmd(t, e, "--apply")
	if err != nil {
		t.Fatalf("--apply posterior: %v\n%s", err, out)
	}
	if !strings.Contains(out, "modo de captura: allowlist") || !strings.Contains(out, "prompts: desligados") {
		t.Errorf("modo ou prompts não reaproveitados:\n%s", out)
	}
	doc = readSettingsDoc(t, e.settings)
	if n := ownedCount(doc, "UserPromptSubmit"); n != 0 {
		t.Errorf("prompts religados sem flag: %d handler(s)", n)
	}
	for _, cmd := range ownedCommandsOf(doc, "Stop") {
		if !strings.Contains(cmd, "--project-strategy repo-root") {
			t.Errorf("repo-root perdido no --apply que o omitiu: %s", cmd)
		}
	}

	// Remover o repo-root com basename explícito.
	if _, err := runInstallHooksCmd(t, e, "--apply", "--project-strategy", "basename"); err != nil {
		t.Fatalf("--apply com basename: %v", err)
	}
	doc = readSettingsDoc(t, e.settings)
	for _, cmd := range ownedCommandsOf(doc, "Stop") {
		if strings.Contains(cmd, "--project-strategy") {
			t.Errorf("basename explícito deveria remover o repo-root: %s", cmd)
		}
	}

	// Religar a captura de prompts.
	if _, err := runInstallHooksCmd(t, e, "--apply", "--capture-prompts"); err != nil {
		t.Fatalf("--apply com --capture-prompts: %v", err)
	}
	doc = readSettingsDoc(t, e.settings)
	if n := ownedCount(doc, "UserPromptSubmit"); n != 1 {
		t.Errorf("--capture-prompts: handlers em UserPromptSubmit = %d, quer 1", n)
	}
	if !strings.Contains(strings.Join(thirdPartyCommands(doc), "\n"), "/usr/bin/prompt-thing") {
		t.Errorf("terceiro em UserPromptSubmit perdido ao religar prompts")
	}
}

// TestInstallHooksTokenOutsideCommandLine: com --apply e --auth-token, o token vai
// para os dois arquivos do dono e não aparece no settings.json nem na saída.
func TestInstallHooksTokenOutsideCommandLine(t *testing.T) {
	e := newInstallHooksEnv(t)
	const token = "aim_TESTSECRET123"

	out, err := runInstallHooksCmd(t, e, "--apply", "--auth-token", token)
	if err != nil {
		t.Fatalf("--apply com --auth-token: %v\n%s", err, out)
	}
	if strings.Contains(out, token) {
		t.Errorf("o token apareceu na saída:\n%s", out)
	}

	tokRaw, err := os.ReadFile(filepath.Join(e.data, hookTokenFile))
	if err != nil {
		t.Fatalf("arquivo do token não gravado: %v", err)
	}
	if string(tokRaw) != token {
		t.Errorf("arquivo do token = %q, quer o token puro", tokRaw)
	}
	hdrRaw, err := os.ReadFile(filepath.Join(e.data, hookAuthHeaderFile))
	if err != nil {
		t.Fatalf("arquivo do cabeçalho não gravado: %v", err)
	}
	if string(hdrRaw) != "Authorization: Bearer "+token {
		t.Errorf("cabeçalho = %q", hdrRaw)
	}

	settingsRaw, err := os.ReadFile(e.settings)
	if err != nil {
		t.Fatalf("ler settings: %v", err)
	}
	if strings.Contains(string(settingsRaw), token) || strings.Contains(string(settingsRaw), "--auth-token") {
		t.Errorf("o token ou --auth-token foi para o settings.json")
	}

	if runtime.GOOS != "windows" {
		for _, name := range []string{hookTokenFile, hookAuthHeaderFile} {
			fi, err := os.Stat(filepath.Join(e.data, name))
			if err != nil {
				t.Fatalf("stat %s: %v", name, err)
			}
			if perm := fi.Mode().Perm(); perm != 0o600 {
				t.Errorf("permissão de %s = %o, quer 600", name, perm)
			}
		}
	}

	// Sem --apply, o token não é gravado nem impresso.
	e2 := newInstallHooksEnv(t)
	out, err = runInstallHooksCmd(t, e2, "--auth-token", token)
	if err != nil {
		t.Fatalf("simulação com --auth-token: %v", err)
	}
	if strings.Contains(out, token) {
		t.Errorf("o token apareceu na simulação:\n%s", out)
	}
	if _, err := os.Stat(filepath.Join(e2.data, hookTokenFile)); !os.IsNotExist(err) {
		t.Errorf("token gravado sem --apply (err=%v)", err)
	}
}

// TestInstallHooksApplyIdempotent: repetir o mesmo --apply não grava de novo e não
// cria backup.
func TestInstallHooksApplyIdempotent(t *testing.T) {
	e := newInstallHooksEnv(t)
	if err := os.WriteFile(e.settings, []byte(thirdPartySettings), 0o600); err != nil {
		t.Fatalf("gravar settings de terceiros: %v", err)
	}
	args := []string{"--apply", "--auth-token", "tok-IDEMP-1"}

	if _, err := runInstallHooksCmd(t, e, args...); err != nil {
		t.Fatalf("primeiro --apply: %v", err)
	}
	first, err := os.ReadFile(e.settings)
	if err != nil {
		t.Fatalf("ler settings: %v", err)
	}

	out, err := runInstallHooksCmd(t, e, args...)
	if err != nil {
		t.Fatalf("segundo --apply: %v\n%s", err, out)
	}
	if !strings.Contains(out, "NoOp") {
		t.Errorf("segundo --apply deveria ser NoOp:\n%s", out)
	}
	second, err := os.ReadFile(e.settings)
	if err != nil {
		t.Fatalf("ler settings: %v", err)
	}
	if !bytes.Equal(first, second) {
		t.Errorf("settings mudou no segundo --apply")
	}

	if got := countBakFiles(t, e.claude); got != 1 {
		t.Errorf("backups de settings após dois --apply iguais = %d, quer 1", got)
	}
	if got := countBakFiles(t, e.data); got != 0 {
		t.Errorf("backups no diretório de dados após dois --apply iguais = %d, quer 0", got)
	}
	if _, err := os.Stat(e.settings + ".bak-1700000001"); err != nil {
		t.Errorf("backup do primeiro --apply sumiu: %v", err)
	}
}

// countBakFiles conta as entradas com ".bak-" no diretório.
func countBakFiles(t *testing.T, dir string) int {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("ler %s: %v", dir, err)
	}
	n := 0
	for _, en := range entries {
		if strings.Contains(en.Name(), ".bak-") {
			n++
		}
	}
	return n
}

// TestInstallHooksProjectStrategyIgnoresEnv: AI_MEMORY_PROJECT_STRATEGY não é
// gravado no comando do hook. Ele vale só com marcador, no tempo de execução.
func TestInstallHooksProjectStrategyIgnoresEnv(t *testing.T) {
	e := newInstallHooksEnv(t)
	t.Setenv("AI_MEMORY_PROJECT_STRATEGY", "repo-root")

	if _, err := runInstallHooksCmd(t, e, "--apply"); err != nil {
		t.Fatalf("--apply: %v", err)
	}
	doc := readSettingsDoc(t, e.settings)
	for _, cmd := range ownedCommandsOf(doc, "SessionStart") {
		if strings.Contains(cmd, "--project-strategy") {
			t.Errorf("env var foi gravada no comando do hook: %s", cmd)
		}
	}
}

// TestInstallHooksRejectsBadInput: entradas inválidas são recusadas antes de
// qualquer gravação, e o erro não repete o token.
func TestInstallHooksRejectsBadInput(t *testing.T) {
	e := newInstallHooksEnv(t)

	cases := [][]string{
		{"--agent", "codex"},
		{"--capture-mode", "maybe"},
		{"--project-strategy", "monorepo"},
		{"--no-capture-prompts", "--capture-prompts"},
		{"--apply", "--auth-token", "bad token value"},
	}
	for _, args := range cases {
		out, err := runInstallHooksCmd(t, e, args...)
		if err == nil {
			t.Errorf("args %v: esperava erro, saída:\n%s", args, out)
		}
		if strings.Contains(err.Error(), "bad token value") {
			t.Errorf("args %v: erro repete o token", args)
		}
	}
	if _, err := os.Stat(e.settings); !os.IsNotExist(err) {
		t.Errorf("settings.json gravado apesar de entrada inválida (err=%v)", err)
	}
}

// TestInstallHooksRefusesBrokenSettings: settings.json inválido gera erro e não é
// sobrescrito.
func TestInstallHooksRefusesBrokenSettings(t *testing.T) {
	e := newInstallHooksEnv(t)
	const broken = "{ isto não é json"
	if err := os.WriteFile(e.settings, []byte(broken), 0o600); err != nil {
		t.Fatalf("gravar settings quebrado: %v", err)
	}

	if _, err := runInstallHooksCmd(t, e, "--apply"); err == nil {
		t.Fatalf("esperava erro com settings inválido")
	}
	raw, err := os.ReadFile(e.settings)
	if err != nil {
		t.Fatalf("ler settings: %v", err)
	}
	if string(raw) != broken {
		t.Errorf("settings inválido foi alterado")
	}
}
