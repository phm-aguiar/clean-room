package config

import (
	"errors"
	"fmt"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/knadh/koanf"
	"github.com/knadh/koanf/providers/env"
)

// Caminho rápido do hook (docs/cli.md §5; plan da 002, Q-09). LoadHook é o carregador
// mínimo que o comando hook e o hook-drain usam: não carrega o TOML, o .env nem o log.
// As únicas leituras de ambiente são as variáveis de tempo AI_MEMORY_HOOK_*, lidas pelo
// provider env do koanf, como em Load (INV-01).

const (
	hookEnvPrefix      = "AI_MEMORY_HOOK_"
	hookMaxMinutes     = 60
	hookTokenFileName  = "hook-token" // mesmo nome gravado por install-hooks (0600, no data_dir)
	hookDefaultRequest = 3 * time.Second
	hookDefaultBudget  = 5 * time.Minute
)

// HookOptions são as flags do comando hook que sobrepõem o padrão. Vazio não tem efeito.
type HookOptions struct {
	// DataDir é a flag --data-dir. Vazio usa o diretório de dados padrão da plataforma.
	DataDir string
	// ServerURL é a flag --server-url. Vazio usa defaultServerURL.
	ServerURL string
}

// HookConfig é a configuração do caminho rápido. Os tempos são durações já convertidas.
type HookConfig struct {
	dataDir          string
	serverURL        string
	drainTimeout     time.Duration
	handoffTimeout   time.Duration
	startBudget      time.Duration
	backgroundBudget time.Duration
}

// DataDir é o diretório de dados absoluto e canônico.
func (h HookConfig) DataDir() string { return h.dataDir }

// ServerURL é a URL do servidor validada (http ou https, com host).
func (h HookConfig) ServerURL() string { return h.serverURL }

// TokenPath é o arquivo 0600 com o token do hook dentro do diretório de dados. O token
// nunca vai para a linha de comando (docs/cli.md §2.4).
func (h HookConfig) TokenPath() string { return filepath.Join(h.dataDir, hookTokenFileName) }

// DrainTimeout é o timeout de cada POST durante a drenagem (AI_MEMORY_HOOK_DRAIN_TIMEOUT_MINUTES).
func (h HookConfig) DrainTimeout() time.Duration { return h.drainTimeout }

// HandoffTimeout é o timeout do GET /handoff do início de sessão (AI_MEMORY_HOOK_HANDOFF_TIMEOUT_MINUTES).
func (h HookConfig) HandoffTimeout() time.Duration { return h.handoffTimeout }

// StartBudget é o orçamento do início de sessão (AI_MEMORY_HOOK_START_BUDGET_MINUTES).
func (h HookConfig) StartBudget() time.Duration { return h.startBudget }

// BackgroundDrainBudget é o orçamento do hook-drain destacado (AI_MEMORY_HOOK_BACKGROUND_DRAIN_BUDGET_MINUTES).
func (h HookConfig) BackgroundDrainBudget() time.Duration { return h.backgroundBudget }

// LoadHook resolve a configuração do caminho rápido. data_dir e server_url vêm das flags
// ou dos padrões; o ambiente só entra pelas variáveis de tempo AI_MEMORY_HOOK_*.
//
// Variáveis de tempo (docs/cli.md §5.3, docs/external-lifecycle.md): inteiros em minutos,
// no máximo 60 (valores acima viram 60). Zero, negativo e não numérico usam o padrão.
// Os padrões são 3 s para drenagem, handoff e início de sessão, e 5 min para a drenagem
// destacada.
func LoadHook(opts HookOptions) (HookConfig, error) {
	k := koanf.New(".")
	if err := k.Load(env.Provider(hookEnvPrefix, ".", hookEnvKey), nil); err != nil {
		return HookConfig{}, fmt.Errorf("config: lendo variáveis %s*: %w", hookEnvPrefix, err)
	}

	dataDirSrc := opts.DataDir
	if dataDirSrc == "" {
		def, err := defaultDataDir()
		if err != nil {
			return HookConfig{}, err
		}
		dataDirSrc = def
	}
	dataDir, err := canonicalPath(dataDirSrc)
	if err != nil {
		return HookConfig{}, fmt.Errorf("config: %s %q: %w", keyDataDir, dataDirSrc, err)
	}

	serverURL := opts.ServerURL
	if serverURL == "" {
		serverURL = defaultServerURL
	}
	if err := validServerURL(serverURL); err != nil {
		return HookConfig{}, err
	}

	return HookConfig{
		dataDir:          dataDir,
		serverURL:        serverURL,
		drainTimeout:     hookMinutes(k, "drain_timeout_minutes", hookDefaultRequest),
		handoffTimeout:   hookMinutes(k, "handoff_timeout_minutes", hookDefaultRequest),
		startBudget:      hookMinutes(k, "start_budget_minutes", hookDefaultRequest),
		backgroundBudget: hookMinutes(k, "background_drain_budget_minutes", hookDefaultBudget),
	}, nil
}

// hookEnvKey converte AI_MEMORY_HOOK_DRAIN_TIMEOUT_MINUTES em drain_timeout_minutes.
func hookEnvKey(s string) string {
	return strings.ToLower(strings.TrimPrefix(s, hookEnvPrefix))
}

// hookMinutes lê um inteiro em minutos do koanf. Ausente, vazio, zero, negativo ou não
// numérico devolvem def. Acima de hookMaxMinutes vira hookMaxMinutes. Um valor que
// estoura int (positivo) também vira o máximo.
func hookMinutes(k *koanf.Koanf, key string, def time.Duration) time.Duration {
	raw := k.String(key)
	if raw == "" {
		return def
	}
	n, err := strconv.Atoi(raw)
	if err != nil {
		if errors.Is(err, strconv.ErrRange) && !strings.HasPrefix(raw, "-") {
			n = hookMaxMinutes
		} else {
			return def
		}
	}
	if n <= 0 {
		return def
	}
	if n > hookMaxMinutes {
		n = hookMaxMinutes
	}
	return time.Duration(n) * time.Minute
}
