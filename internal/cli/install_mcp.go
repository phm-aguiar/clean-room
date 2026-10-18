package cli

// Comando install-mcp --client claude-code (docs/cli.md §8, docs/mcp-install.md
// "Claude Code" e "Resolução de endereço e token", docs/install.md "CLAUDE_CONFIG_DIR").
//
// Sem --apply, o comando só imprime o bloco {"mcpServers": {"ai-memory": ...}} e não
// toca no disco. Com --apply, mescla a entrada em .claude.json por ApplyAtomic,
// preservando as demais chaves e os outros servidores MCP. O arquivo é
// $CLAUDE_CONFIG_DIR/.claude.json, ou ~/.claude.json quando a variável não está
// definida.
//
// O token nunca entra na linha de comando nem na saída. Ele vem da configuração
// (AI_MEMORY_AUTH_TOKEN) ou do arquivo hook-token do diretório de dados, gravado
// por install-hooks (T049). Na impressão, o cabeçalho mostra o placeholder <token>.

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/phm-aguiar/ai-memory-go/internal/config"
)

const (
	flagClient       = "client"
	flagServerURL    = "server-url"
	flagSessionAware = "session-aware"

	clientClaudeCode = "claude-code"

	// mcpServerName é a chave da entrada em mcpServers. Só ela é alterada.
	mcpServerName = "ai-memory"
	// mcpEndpointPath é o caminho do endpoint MCP do servidor.
	mcpEndpointPath = "/mcp"
	// mcpBridgeCommand é o binário da ponte stdio com identificação de sessão.
	mcpBridgeCommand = "ai-memory"
	mcpBridgeSubcmd  = "mcp-bridge"
	// mcpTokenPlaceholder substitui o token na impressão. O valor real não é exibido.
	mcpTokenPlaceholder = "<token>"
	claudeJSONFile      = ".claude.json"
)

// NewInstallMCPCommand devolve o subcomando "install-mcp". A raiz o registra:
// root.AddCommand(NewInstallMCPCommand()).
func NewInstallMCPCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "install-mcp",
		Short: "Imprime ou aplica o registro do servidor MCP no cliente",
		Long: "Imprime a entrada mcpServers.ai-memory do cliente. Com --apply, mescla a entrada " +
			"no arquivo do cliente preservando as demais chaves e servidores (hoje só claude-code: " +
			".claude.json, em CLAUDE_CONFIG_DIR quando definido).",
		Args: cobra.NoArgs,
		RunE: runInstallMCP,
	}
	cmd.Flags().String(flagClient, clientClaudeCode, "cliente alvo (hoje só claude-code)")
	cmd.Flags().Bool(flagApply, false, "grava no arquivo do cliente; sem ele, só imprime")
	cmd.Flags().String(flagServerURL, "", "endereço do servidor; sobrepõe AI_MEMORY_SERVER_URL (o /mcp é acrescentado)")
	cmd.Flags().Bool(flagSessionAware, false, "registra a ponte stdio com identificação de sessão (só claude-code)")
	return cmd
}

// runInstallMCP lê as opções, resolve o endereço e o token, e imprime ou aplica.
func runInstallMCP(cmd *cobra.Command, _ []string) error {
	client, err := cmd.Flags().GetString(flagClient)
	if err != nil {
		return fmt.Errorf("cli: flag --%s: %w", flagClient, err)
	}
	if client != clientClaudeCode {
		return fmt.Errorf("cli: install-mcp: cliente %q não suportado nesta versão", client)
	}
	apply, err := cmd.Flags().GetBool(flagApply)
	if err != nil {
		return fmt.Errorf("cli: flag --%s: %w", flagApply, err)
	}
	sessionAware, err := cmd.Flags().GetBool(flagSessionAware)
	if err != nil {
		return fmt.Errorf("cli: flag --%s: %w", flagSessionAware, err)
	}
	serverFlag, err := cmd.Flags().GetString(flagServerURL)
	if err != nil {
		return fmt.Errorf("cli: flag --%s: %w", flagServerURL, err)
	}

	opts, err := ConfigOptions(cmd)
	if err != nil {
		return err
	}
	cfg, err := loadConfig(opts)
	if err != nil {
		return err
	}

	raw := cfg.ServerURL()
	if cmd.Flags().Changed(flagServerURL) {
		raw = serverFlag
	}
	endpoint, err := mcpEndpoint(raw)
	if err != nil {
		return err
	}

	token, source, err := resolveMCPToken(cfg)
	if err != nil {
		return err
	}

	path := claudeJSONPath(cfg)
	out := cmd.OutOrStdout()
	if !apply {
		// Na impressão, o token vira placeholder: o valor real não sai na saída.
		printToken := ""
		if token != "" {
			printToken = mcpTokenPlaceholder
		}
		return printInstallMCPPlan(out, path, mcpEntry(endpoint, printToken, sessionAware), token != "")
	}
	return applyInstallMCP(out, path, endpoint, token, source, sessionAware)
}

// mcpEndpoint normaliza o endereço para terminar em /mcp. Barras finais são
// removidas antes, e a operação é idempotente.
func mcpEndpoint(raw string) (string, error) {
	s := strings.TrimRight(strings.TrimSpace(raw), "/")
	u, err := url.Parse(s)
	if err != nil {
		return "", fmt.Errorf("cli: install-mcp: endereço do servidor inválido: %w", err)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return "", fmt.Errorf("cli: install-mcp: endereço do servidor deve ser http ou https")
	}
	if u.Host == "" {
		return "", fmt.Errorf("cli: install-mcp: endereço do servidor sem host")
	}
	if !strings.HasSuffix(s, mcpEndpointPath) {
		s += mcpEndpointPath
	}
	return s, nil
}

// resolveMCPToken devolve o token e a origem ("configuração" ou "hook-token").
// Ordem: AI_MEMORY_AUTH_TOKEN (config), depois o arquivo hook-token do diretório
// de dados. Sem nenhum, devolve vazio e a entrada não tem cabeçalho. Erros não
// repetem o valor do token.
func resolveMCPToken(cfg config.Config) (string, string, error) {
	if t := cfg.AuthToken(); t != "" {
		if !validHookToken(t) {
			return "", "", fmt.Errorf("cli: install-mcp: token da configuração vazio ou com espaço ou caractere de controle")
		}
		return t, "configuração", nil
	}
	raw, err := os.ReadFile(filepath.Join(cfg.DataDir(), hookTokenFile))
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return "", "", nil
		}
		return "", "", fmt.Errorf("cli: install-mcp: ler %s: %w", hookTokenFile, err)
	}
	t := strings.TrimSpace(string(raw))
	if t == "" {
		return "", "", nil
	}
	if !validHookToken(t) {
		return "", "", fmt.Errorf("cli: install-mcp: %s com espaço ou caractere de controle", hookTokenFile)
	}
	return t, hookTokenFile, nil
}

// claudeJSONPath devolve o arquivo .claude.json. Com CLAUDE_CONFIG_DIR definido,
// o arquivo fica nele. Sem a variável, config.ClaudeConfigDir cai em
// <home>/.claude, e o arquivo é <home>/.claude.json.
//
// Limite conhecido: CLAUDE_CONFIG_DIR apontando explicitamente para <home>/.claude
// é indistinguível do padrão sem ler o ambiente, então o alvo vira <home>/.claude.json.
func claudeJSONPath(cfg config.Config) string {
	dir := cfg.ClaudeConfigDir()
	if dir == filepath.Join(cfg.HomeDir(), ".claude") {
		return filepath.Join(cfg.HomeDir(), claudeJSONFile)
	}
	return filepath.Join(dir, claudeJSONFile)
}

// mcpEntry monta a entrada ai-memory. Sem sessão, é HTTP estático. Com
// sessionAware, é a ponte stdio. O cabeçalho ou o env levam só o token; sem token,
// nenhum dos dois aparece.
func mcpEntry(endpoint, token string, sessionAware bool) map[string]any {
	if sessionAware {
		entry := map[string]any{
			"type":    "stdio",
			"command": mcpBridgeCommand,
			"args":    []string{mcpBridgeSubcmd, "--server-url", endpoint},
		}
		if token != "" {
			entry["env"] = map[string]string{"AI_MEMORY_AUTH_TOKEN": token}
		}
		return entry
	}
	entry := map[string]any{
		"type": "http",
		"url":  endpoint,
	}
	if token != "" {
		entry["headers"] = map[string]string{"Authorization": "Bearer " + token}
	}
	return entry
}

// mcpFragment monta o bloco {"mcpServers": {"ai-memory": ...}} que a impressão mostra.
func mcpFragment(entry map[string]any) ([]byte, error) {
	return encodeIndentedJSON(map[string]any{
		"mcpServers": map[string]any{mcpServerName: entry},
	})
}

// printInstallMCPPlan imprime a configuração sem gravar nada.
func printInstallMCPPlan(out io.Writer, path string, entry map[string]any, hasToken bool) error {
	frag, err := mcpFragment(entry)
	if err != nil {
		return err
	}
	var b bytes.Buffer
	fmt.Fprintf(&b, "install-mcp %s (simulação: nada foi gravado)\n", clientClaudeCode)
	fmt.Fprintf(&b, "arquivo alvo: %s\n", path)
	if hasToken {
		b.WriteString("# o token é gravado por --apply; o valor não é exibido\n")
	}
	b.Write(frag)
	if _, err := out.Write(b.Bytes()); err != nil {
		return fmt.Errorf("cli: install-mcp: escrever saída: %w", err)
	}
	return nil
}

// applyInstallMCP mescla a entrada em .claude.json por ApplyAtomic. Preserva as
// demais chaves e servidores. Repetir o comando produz no-op.
func applyInstallMCP(out io.Writer, path, endpoint, token, source string, sessionAware bool) error {
	_, statErr := os.Stat(path)
	existed := statErr == nil

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("cli: install-mcp: criar diretório do cliente: %w", err)
	}
	entry := mcpEntry(endpoint, token, sessionAware)
	status, err := ApplyAtomic(path, func(current []byte) ([]byte, error) {
		return mergeMCPConfig(current, entry)
	})
	if err != nil {
		return fmt.Errorf("cli: install-mcp: gravar %s: %w", path, err)
	}

	result := "updated"
	switch {
	case status == ApplyNoOp:
		result = "no-op (already up to date)"
	case !existed:
		result = "created"
	}
	var b bytes.Buffer
	fmt.Fprintf(&b, "install-mcp %s: %s em %s\n", clientClaudeCode, result, path)
	if token != "" {
		fmt.Fprintf(&b, "token: lido de %s (valor não exibido)\n", source)
	}
	if _, err := out.Write(b.Bytes()); err != nil {
		return fmt.Errorf("cli: install-mcp: escrever saída: %w", err)
	}
	return nil
}

// mergeMCPConfig aplica a entrada sobre o conteúdo atual de .claude.json. Só
// mcpServers["ai-memory"] muda. As demais chaves e servidores ficam como estavam.
// Conteúdo vazio vira objeto novo. JSON inválido ou não objeto gera erro, e nada
// é gravado.
func mergeMCPConfig(current []byte, entry map[string]any) ([]byte, error) {
	root, err := decodeJSONObject(current)
	if err != nil {
		return nil, fmt.Errorf("cli: install-mcp: %s não é um objeto JSON válido: %w", claudeJSONFile, err)
	}
	servers, err := decodeJSONObject(root["mcpServers"])
	if err != nil {
		return nil, fmt.Errorf("cli: install-mcp: mcpServers em %s não é um objeto: %w", claudeJSONFile, err)
	}
	ours, err := rawJSON(entry)
	if err != nil {
		return nil, err
	}
	servers[mcpServerName] = ours
	srv, err := rawJSON(servers)
	if err != nil {
		return nil, err
	}
	root["mcpServers"] = srv
	return encodeIndentedJSON(root)
}
