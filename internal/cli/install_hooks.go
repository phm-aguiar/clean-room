package cli

// Comando install-hooks --agent claude-code (docs/cli.md §8, docs/install.md
// "Default project resolution" e "Capture only repositories that opt in",
// docs/users.md "Hooks and agent wiring", docs/marker-file.md "Install-wide
// default" e "Allowlist mode", docs/support-matrix.md).
//
// Sem --apply, o comando só imprime o bloco de hooks dos 9 eventos e não toca
// no disco. Com --apply, mescla o bloco em settings.json (preservando as
// entradas de terceiros) por ApplyAtomic, e grava o estado de captura no
// diretório de dados. Com --auth-token e --apply, o token vai para dois
// arquivos do dono no diretório de dados e nunca entra no comando do hook nem
// na saída.

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"unicode"

	"github.com/spf13/cobra"

	"github.com/phm-aguiar/ai-memory-go/internal/config"
)

const (
	flagAgent            = "agent"
	flagApply            = "apply"
	flagCaptureMode      = "capture-mode"
	flagNoCapturePrompts = "no-capture-prompts"
	flagCapturePrompts   = "capture-prompts"
	flagProjectStrategy  = "project-strategy"
	flagAuthToken        = "auth-token"

	agentClaudeCode = "claude-code"

	captureModeDenylist  = "denylist"
	captureModeAllowlist = "allowlist"

	strategyBasename = "basename"
	strategyRepoRoot = "repo-root"

	// installHooksStateFile guarda, no diretório de dados, o modo de captura e
	// o estado dos prompts, para que um --apply posterior os reaproveite.
	installHooksStateFile = "install-hooks.json"
	// hookTokenFile tem o token puro. hookAuthHeaderFile tem o cabeçalho de
	// autorização completo. Os dois são do dono (0600 no Unix).
	hookTokenFile      = "hook-token"
	hookAuthHeaderFile = "hook-auth-header"
	claudeSettingsFile = "settings.json"
)

// installHooksExecutable devolve o caminho do binário gravado no comando do
// hook. Os testes o substituem.
var installHooksExecutable = os.Executable

// claudeHookEvent liga o evento nativo do Claude Code ao valor de --event do
// binário ai-memory. Matcher "" significa sem matcher.
type claudeHookEvent struct {
	name    string
	hookArg string
	matcher string
}

// claudeHookEvents são os 9 eventos da support matrix, em ordem fixa.
var claudeHookEvents = []claudeHookEvent{
	{name: "SessionStart", hookArg: "session-start"},
	{name: "UserPromptSubmit", hookArg: "user-prompt-submit"},
	{name: "PreToolUse", hookArg: "pre-tool-use", matcher: "*"},
	{name: "PostToolUse", hookArg: "post-tool-use", matcher: "*"},
	{name: "PreCompact", hookArg: "pre-compact"},
	{name: "Stop", hookArg: "stop"},
	{name: "SessionEnd", hookArg: "session-end"},
	{name: "SubagentStart", hookArg: "subagent-start"},
	{name: "SubagentStop", hookArg: "subagent-stop"},
}

// installHooksState é o estado persistido por instalação.
type installHooksState struct {
	CaptureMode    string `json:"capture_mode"`
	CapturePrompts bool   `json:"capture_prompts"`
}

// claudeHookSpec é um handler a ser registrado num evento do Claude Code.
type claudeHookSpec struct {
	event   string
	matcher string
	command string
}

// NewInstallHooksCommand devolve o subcomando "install-hooks". A raiz o
// registra: root.AddCommand(NewInstallHooksCommand()).
func NewInstallHooksCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "install-hooks",
		Short: "Imprime ou aplica os hooks de ciclo de vida do agente",
		Long: "Imprime a configuração dos hooks do agente. Com --apply, mescla no arquivo do agente " +
			"(hoje só claude-code: settings.json) preservando as entradas de terceiros.",
		Args: cobra.NoArgs,
		RunE: runInstallHooks,
	}
	cmd.Flags().String(flagAgent, agentClaudeCode, "agente alvo (hoje só claude-code)")
	cmd.Flags().Bool(flagApply, false, "grava no arquivo do agente; sem ele, só imprime")
	cmd.Flags().String(flagCaptureMode, "", "modo de captura: denylist ou allowlist (padrão: o último gravado, ou denylist)")
	cmd.Flags().Bool(flagNoCapturePrompts, false, "desliga a captura do prompt do usuário (UserPromptSubmit)")
	cmd.Flags().Bool(flagCapturePrompts, false, "religa a captura do prompt do usuário")
	cmd.Flags().String(flagProjectStrategy, "", "basename ou repo-root gravado no comando do hook (padrão: o último gravado, ou basename)")
	cmd.Flags().String(flagAuthToken, "", "token que os hooks enviam; com --apply, vai para arquivos do dono no diretório de dados")
	return cmd
}

// runInstallHooks lê as opções, resolve o estado e imprime ou aplica.
func runInstallHooks(cmd *cobra.Command, _ []string) error {
	agent, err := cmd.Flags().GetString(flagAgent)
	if err != nil {
		return fmt.Errorf("cli: flag --%s: %w", flagAgent, err)
	}
	if agent != agentClaudeCode {
		return fmt.Errorf("cli: install-hooks: agente %q não suportado nesta versão", agent)
	}
	apply, err := cmd.Flags().GetBool(flagApply)
	if err != nil {
		return fmt.Errorf("cli: flag --%s: %w", flagApply, err)
	}

	opts, err := ConfigOptions(cmd)
	if err != nil {
		return err
	}
	cfg, err := loadConfig(opts)
	if err != nil {
		return err
	}

	in, err := readInstallHooksFlags(cmd)
	if err != nil {
		return err
	}

	state, err := readInstallHooksState(cfg.DataDir())
	if err != nil {
		return err
	}
	if in.captureModeSet {
		state.CaptureMode = in.captureMode
	}
	if in.promptsSet {
		state.CapturePrompts = in.capturePrompts
	}

	settingsPath := filepath.Join(cfg.ClaudeConfigDir(), claudeSettingsFile)
	current, err := readOptionalFile(settingsPath)
	if err != nil {
		return err
	}

	strategy := in.projectStrategy
	if !in.strategySet {
		strategy, err = strategyFromSettings(current)
		if err != nil {
			return err
		}
	}

	exe, err := installHooksExecutable()
	if err != nil {
		return fmt.Errorf("cli: install-hooks: localizar o binário: %w", err)
	}
	specs := claudeHookSpecs(exe, cfg.ServerURL(), strategy, state.CapturePrompts)

	out := cmd.OutOrStdout()
	if !apply {
		if _, err := mergeClaudeSettings(current, specs); err != nil {
			return err
		}
		return printInstallHooksPlan(out, settingsPath, specs, state, strategy)
	}
	// Valida o settings antes de gravar qualquer arquivo: JSON inválido não deixa
	// estado nem token parciais.
	if _, err := mergeClaudeSettings(current, specs); err != nil {
		return err
	}

	return applyInstallHooks(out, cfg, settingsPath, specs, state, strategy, in)
}

// installHooksFlags são as opções lidas da linha de comando.
type installHooksFlags struct {
	captureModeSet  bool
	captureMode     string
	promptsSet      bool
	capturePrompts  bool
	strategySet     bool
	projectStrategy string
	tokenSet        bool
	authToken       string
}

// readInstallHooksFlags lê e valida as flags. Erros não repetem o valor do
// token, para que ele não vá para a saída de erro.
func readInstallHooksFlags(cmd *cobra.Command) (installHooksFlags, error) {
	var in installHooksFlags
	var err error

	in.captureModeSet = cmd.Flags().Changed(flagCaptureMode)
	if in.captureModeSet {
		if in.captureMode, err = cmd.Flags().GetString(flagCaptureMode); err != nil {
			return in, fmt.Errorf("cli: flag --%s: %w", flagCaptureMode, err)
		}
		if in.captureMode != captureModeDenylist && in.captureMode != captureModeAllowlist {
			return in, fmt.Errorf("cli: --%s deve ser denylist ou allowlist", flagCaptureMode)
		}
	}

	noPrompts, err := cmd.Flags().GetBool(flagNoCapturePrompts)
	if err != nil {
		return in, fmt.Errorf("cli: flag --%s: %w", flagNoCapturePrompts, err)
	}
	yesPrompts, err := cmd.Flags().GetBool(flagCapturePrompts)
	if err != nil {
		return in, fmt.Errorf("cli: flag --%s: %w", flagCapturePrompts, err)
	}
	if noPrompts && yesPrompts {
		return in, fmt.Errorf("cli: --%s e --%s são mutuamente exclusivas", flagNoCapturePrompts, flagCapturePrompts)
	}
	if noPrompts || yesPrompts {
		in.promptsSet = true
		in.capturePrompts = yesPrompts
	}

	in.strategySet = cmd.Flags().Changed(flagProjectStrategy)
	if in.strategySet {
		if in.projectStrategy, err = cmd.Flags().GetString(flagProjectStrategy); err != nil {
			return in, fmt.Errorf("cli: flag --%s: %w", flagProjectStrategy, err)
		}
		if in.projectStrategy != strategyBasename && in.projectStrategy != strategyRepoRoot {
			return in, fmt.Errorf("cli: --%s deve ser basename ou repo-root", flagProjectStrategy)
		}
	}

	in.tokenSet = cmd.Flags().Changed(flagAuthToken)
	if in.tokenSet {
		if in.authToken, err = cmd.Flags().GetString(flagAuthToken); err != nil {
			return in, fmt.Errorf("cli: flag --%s: %w", flagAuthToken, err)
		}
		if !validHookToken(in.authToken) {
			return in, fmt.Errorf("cli: --%s vazio ou com espaço ou caractere de controle", flagAuthToken)
		}
	}
	return in, nil
}

// validHookToken diz se o token é usável num cabeçalho HTTP: não vazio e sem
// espaço nem caractere de controle.
func validHookToken(token string) bool {
	if token == "" {
		return false
	}
	return strings.IndexFunc(token, func(r rune) bool {
		return unicode.IsSpace(r) || unicode.IsControl(r)
	}) < 0
}

// claudeHookSpecs monta os handlers dos eventos habilitados. Sem captura de
// prompt, UserPromptSubmit fica de fora. O project-strategy vai no comando
// quando é repo-root; basename não grava nada.
func claudeHookSpecs(exe, serverURL, strategy string, capturePrompts bool) []claudeHookSpec {
	specs := make([]claudeHookSpec, 0, len(claudeHookEvents))
	for _, ev := range claudeHookEvents {
		if ev.name == "UserPromptSubmit" && !capturePrompts {
			continue
		}
		parts := []string{
			hookShellQuote(exe), "hook", "--event", ev.hookArg,
			"--agent", agentClaudeCode, "--server-url", hookShellQuote(serverURL),
		}
		if strategy == strategyRepoRoot {
			parts = append(parts, "--"+flagProjectStrategy, strategyRepoRoot)
		}
		specs = append(specs, claudeHookSpec{
			event:   ev.name,
			matcher: ev.matcher,
			command: strings.Join(parts, " "),
		})
	}
	return specs
}

// hookShellQuote põe o valor entre aspas para o shell do agente. No Windows usa
// aspas duplas, pois o caminho tem barras invertidas. No Unix usa aspas simples.
func hookShellQuote(s string) string {
	if runtime.GOOS == "windows" {
		return `"` + s + `"`
	}
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

// isAiMemoryClaudeHook diz se o comando é um handler do ai-memory para o
// Claude Code. Entradas de terceiros não casam.
func isAiMemoryClaudeHook(command string) bool {
	return strings.Contains(command, "ai-memory") &&
		strings.Contains(command, " hook --event ") &&
		strings.Contains(command, " --agent claude-code")
}

// strategyFromSettings lê o --project-strategy do primeiro handler do ai-memory
// que o tiver. Sem handler, ou sem a opção, o padrão é basename.
func strategyFromSettings(current []byte) (string, error) {
	cmds, err := ownedClaudeCommands(current)
	if err != nil {
		return "", err
	}
	for _, c := range cmds {
		fields := strings.Fields(c)
		for i := 0; i+1 < len(fields); i++ {
			if fields[i] == "--"+flagProjectStrategy {
				if fields[i+1] == strategyRepoRoot {
					return strategyRepoRoot, nil
				}
				return strategyBasename, nil
			}
		}
	}
	return strategyBasename, nil
}

// ownedClaudeCommands devolve os comandos do ai-memory já presentes em settings.
func ownedClaudeCommands(current []byte) ([]string, error) {
	root, err := decodeJSONObject(current)
	if err != nil {
		return nil, err
	}
	hooks, err := decodeJSONObject(root["hooks"])
	if err != nil {
		return nil, err
	}
	var cmds []string
	for _, ev := range claudeHookEvents {
		groups, err := decodeJSONArray(hooks[ev.name])
		if err != nil {
			return nil, err
		}
		for _, rawGroup := range groups {
			g, err := decodeJSONObject(rawGroup)
			if err != nil {
				return nil, err
			}
			handlers, err := decodeJSONArray(g["hooks"])
			if err != nil {
				return nil, err
			}
			for _, rawHandler := range handlers {
				var h struct {
					Command string `json:"command"`
				}
				if err := json.Unmarshal(rawHandler, &h); err != nil {
					return nil, fmt.Errorf("cli: install-hooks: handler inválido em settings: %w", err)
				}
				if isAiMemoryClaudeHook(h.Command) {
					cmds = append(cmds, h.Command)
				}
			}
		}
	}
	return cmds, nil
}

// mergeClaudeSettings aplica a transformação de install-hooks sobre o conteúdo
// atual de settings.json. Remove só os handlers do ai-memory dos 9 eventos e
// acrescenta os de specs. Entradas de terceiros ficam como estavam. Conteúdo
// vazio vira objeto novo. JSON inválido gera erro e nada é gravado.
func mergeClaudeSettings(current []byte, specs []claudeHookSpec) ([]byte, error) {
	root, err := decodeJSONObject(current)
	if err != nil {
		return nil, err
	}
	hooks, err := decodeJSONObject(root["hooks"])
	if err != nil {
		return nil, err
	}

	for _, ev := range claudeHookEvents {
		groups, err := decodeJSONArray(hooks[ev.name])
		if err != nil {
			return nil, err
		}
		kept := make([]json.RawMessage, 0, len(groups)+1)
		for _, rawGroup := range groups {
			g, err := decodeJSONObject(rawGroup)
			if err != nil {
				return nil, err
			}
			handlers, err := decodeJSONArray(g["hooks"])
			if err != nil {
				return nil, err
			}
			keep := make([]json.RawMessage, 0, len(handlers))
			removed := 0
			for _, rawHandler := range handlers {
				var h struct {
					Command string `json:"command"`
				}
				if err := json.Unmarshal(rawHandler, &h); err != nil {
					return nil, fmt.Errorf("cli: install-hooks: handler inválido em settings: %w", err)
				}
				if isAiMemoryClaudeHook(h.Command) {
					removed++
					continue
				}
				keep = append(keep, rawHandler)
			}
			switch {
			case removed == 0:
				kept = append(kept, rawGroup)
			case len(keep) == 0:
				// Grupo que só tinha handlers nossos: sai junto.
			default:
				raw, err := rawJSON(keep)
				if err != nil {
					return nil, err
				}
				g["hooks"] = raw
				gr, err := rawJSON(g)
				if err != nil {
					return nil, err
				}
				kept = append(kept, gr)
			}
		}

		for _, s := range specs {
			if s.event != ev.name {
				continue
			}
			group := map[string]any{
				"hooks": []map[string]string{{"type": "command", "command": s.command}},
			}
			if s.matcher != "" {
				group["matcher"] = s.matcher
			}
			gr, err := rawJSON(group)
			if err != nil {
				return nil, err
			}
			kept = append(kept, gr)
		}

		if len(kept) == 0 {
			delete(hooks, ev.name)
			continue
		}
		raw, err := rawJSON(kept)
		if err != nil {
			return nil, err
		}
		hooks[ev.name] = raw
	}

	if len(hooks) == 0 {
		delete(root, "hooks")
	} else {
		raw, err := rawJSON(hooks)
		if err != nil {
			return nil, err
		}
		root["hooks"] = raw
	}
	return encodeIndentedJSON(root)
}

// hooksFragment monta o bloco {"hooks": {...}} que install-hooks imprime, só com
// os handlers do ai-memory.
func hooksFragment(specs []claudeHookSpec) ([]byte, error) {
	hooks := map[string][]any{}
	for _, s := range specs {
		group := map[string]any{
			"hooks": []map[string]string{{"type": "command", "command": s.command}},
		}
		if s.matcher != "" {
			group["matcher"] = s.matcher
		}
		hooks[s.event] = append(hooks[s.event], group)
	}
	return encodeIndentedJSON(map[string]any{"hooks": hooks})
}

// printInstallHooksPlan imprime a configuração sem gravar nada.
func printInstallHooksPlan(out io.Writer, settingsPath string, specs []claudeHookSpec, state installHooksState, strategy string) error {
	frag, err := hooksFragment(specs)
	if err != nil {
		return err
	}
	var b bytes.Buffer
	fmt.Fprintf(&b, "install-hooks %s (simulação: nada foi gravado)\n", agentClaudeCode)
	fmt.Fprintf(&b, "arquivo alvo: %s\n", settingsPath)
	b.WriteString(installHooksSummary(state, strategy))
	b.Write(frag)
	_, err = out.Write(b.Bytes())
	if err != nil {
		return fmt.Errorf("cli: install-hooks: escrever saída: %w", err)
	}
	return nil
}

// applyInstallHooks grava o estado, os arquivos de token (se houver) e o
// settings.json, todos por ApplyAtomic. O token nunca é impresso.
func applyInstallHooks(out io.Writer, cfg config.Config, settingsPath string,
	specs []claudeHookSpec, state installHooksState, strategy string, in installHooksFlags) error {
	dataDir := cfg.DataDir()
	if err := os.MkdirAll(dataDir, 0o700); err != nil {
		return fmt.Errorf("cli: install-hooks: criar diretório de dados: %w", err)
	}
	if runtime.GOOS != "windows" {
		if err := os.Chmod(dataDir, 0o700); err != nil {
			return fmt.Errorf("cli: install-hooks: permissão do diretório de dados: %w", err)
		}
	}

	stateBytes, err := encodeIndentedJSON(state)
	if err != nil {
		return err
	}
	if _, err := ApplyAtomic(filepath.Join(dataDir, installHooksStateFile), constantTransform(stateBytes)); err != nil {
		return fmt.Errorf("cli: install-hooks: gravar estado: %w", err)
	}

	if in.tokenSet {
		if err := writeHookTokenFiles(dataDir, in.authToken); err != nil {
			return err
		}
	}

	if err := os.MkdirAll(filepath.Dir(settingsPath), 0o755); err != nil {
		return fmt.Errorf("cli: install-hooks: criar diretório do agente: %w", err)
	}
	status, err := ApplyAtomic(settingsPath, func(current []byte) ([]byte, error) {
		return mergeClaudeSettings(current, specs)
	})
	if err != nil {
		return fmt.Errorf("cli: install-hooks: gravar %s: %w", settingsPath, err)
	}

	var b bytes.Buffer
	fmt.Fprintf(&b, "install-hooks %s: %s em %s\n", agentClaudeCode, status, settingsPath)
	b.WriteString(installHooksSummary(state, strategy))
	if in.tokenSet {
		fmt.Fprintf(&b, "token: guardado em %s e %s (valor não exibido)\n",
			filepath.Join(dataDir, hookTokenFile), filepath.Join(dataDir, hookAuthHeaderFile))
	}
	if _, err := out.Write(b.Bytes()); err != nil {
		return fmt.Errorf("cli: install-hooks: escrever saída: %w", err)
	}
	return nil
}

// writeHookTokenFiles grava o token puro e o cabeçalho completo no diretório de
// dados, por ApplyAtomic. Os dois ficam com permissão só do dono no Unix.
func writeHookTokenFiles(dataDir, token string) error {
	files := map[string][]byte{
		hookTokenFile:      []byte(token),
		hookAuthHeaderFile: []byte("Authorization: Bearer " + token),
	}
	for name, content := range files {
		path := filepath.Join(dataDir, name)
		if _, err := ApplyAtomic(path, constantTransform(content)); err != nil {
			return fmt.Errorf("cli: install-hooks: gravar %s: %w", name, err)
		}
		if runtime.GOOS != "windows" {
			if err := os.Chmod(path, 0o600); err != nil {
				return fmt.Errorf("cli: install-hooks: permissão de %s: %w", name, err)
			}
		}
	}
	return nil
}

// installHooksSummary descreve o modo, os prompts e o project-strategy em vigor.
func installHooksSummary(state installHooksState, strategy string) string {
	prompts := "ativos"
	if !state.CapturePrompts {
		prompts = "desligados"
	}
	return fmt.Sprintf("modo de captura: %s; prompts: %s; project-strategy: %s\n",
		state.CaptureMode, prompts, strategy)
}

// readInstallHooksState lê o estado de install-hooks.json. Sem arquivo, usa
// denylist e prompts ativos. JSON ou modo inválido gera erro.
func readInstallHooksState(dataDir string) (installHooksState, error) {
	st := installHooksState{CaptureMode: captureModeDenylist, CapturePrompts: true}
	raw, err := os.ReadFile(filepath.Join(dataDir, installHooksStateFile))
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return st, nil
		}
		return st, fmt.Errorf("cli: install-hooks: ler estado: %w", err)
	}
	if err := json.Unmarshal(raw, &st); err != nil {
		return st, fmt.Errorf("cli: install-hooks: estado inválido em %s: %w", installHooksStateFile, err)
	}
	if st.CaptureMode != captureModeDenylist && st.CaptureMode != captureModeAllowlist {
		return st, fmt.Errorf("cli: install-hooks: modo de captura gravado inválido em %s", installHooksStateFile)
	}
	return st, nil
}

// readOptionalFile lê o arquivo; ausente devolve nil.
func readOptionalFile(path string) ([]byte, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, nil
		}
		return nil, fmt.Errorf("cli: install-hooks: ler %s: %w", path, err)
	}
	return raw, nil
}

// constantTransform devolve uma transformação que ignora o conteúdo atual.
func constantTransform(content []byte) func([]byte) ([]byte, error) {
	return func([]byte) ([]byte, error) { return content, nil }
}

// decodeJSONObject lê um objeto JSON. Vazio ou null vira mapa vazio.
func decodeJSONObject(raw []byte) (map[string]json.RawMessage, error) {
	obj := map[string]json.RawMessage{}
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 || string(trimmed) == "null" {
		return obj, nil
	}
	if err := json.Unmarshal(trimmed, &obj); err != nil {
		return nil, fmt.Errorf("cli: install-hooks: JSON de settings não é um objeto: %w", err)
	}
	if obj == nil {
		obj = map[string]json.RawMessage{}
	}
	return obj, nil
}

// decodeJSONArray lê um array JSON. Vazio ou null vira lista vazia.
func decodeJSONArray(raw []byte) ([]json.RawMessage, error) {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 || string(trimmed) == "null" {
		return nil, nil
	}
	var arr []json.RawMessage
	if err := json.Unmarshal(trimmed, &arr); err != nil {
		return nil, fmt.Errorf("cli: install-hooks: JSON de settings não é uma lista: %w", err)
	}
	return arr, nil
}

// rawJSON serializa v sem escapar HTML, para que & e < dos comandos fiquem legíveis.
func rawJSON(v any) (json.RawMessage, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		return nil, fmt.Errorf("cli: install-hooks: serializar: %w", err)
	}
	return json.RawMessage(bytes.TrimRight(buf.Bytes(), "\n")), nil
}

// encodeIndentedJSON serializa v com dois espaços e uma quebra final.
func encodeIndentedJSON(v any) ([]byte, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	if err := enc.Encode(v); err != nil {
		return nil, fmt.Errorf("cli: install-hooks: serializar: %w", err)
	}
	return buf.Bytes(), nil
}
