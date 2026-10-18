// Package config carrega a configuração do processo uma única vez (INV-01).
//
// Precedência, da menor para a maior: padrões embutidos, arquivo TOML, arquivo
// .env local e variáveis do processo AI_MEMORY_*. O separador "__" aninha chaves:
// AI_MEMORY_LOG__LEVEL define log.level.
//
// Este é o único pacote que lê ambiente, e só pelo provider env do koanf.
// Não usa os.LookupEnv nem os.Environ. As exceções são XDG_DATA_HOME, lida por
// os.Getenv em defaultUserDataDir, para o padrão do diretório de dados no Linux, e
// CLAUDE_CONFIG_DIR, lida por os.Getenv em Load. CLAUDE_CONFIG_DIR não tem prefixo
// AI_MEMORY_, então o provider do koanf não a alcança. Ambas são capturadas uma vez.
package config

import (
	"errors"
	"fmt"
	"io/fs"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/knadh/koanf"
	"github.com/knadh/koanf/parsers/dotenv"
	"github.com/knadh/koanf/parsers/toml"
	"github.com/knadh/koanf/providers/confmap"
	"github.com/knadh/koanf/providers/env"
	"github.com/knadh/koanf/providers/file"
)

const (
	envPrefix        = "AI_MEMORY_"
	envFileKey       = "env_file" // AI_MEMORY_ENV_FILE
	keyDataDir       = "data_dir"
	keyLogLevel      = "log.level"
	keyWatchDebounce = "wiki.watch_debounce_ms"
	keyReconcile     = "wiki.reconcile_interval_s"
	keyBind          = "bind"
	keyServerURL     = "server_url"
	keyAllowedHosts  = "allowed_hosts"
	keyAuthToken     = "auth.bearer_token"
	keyClaimOnStart  = "handoff.claim_on_session_start"
	keyCreateOnEnd   = "handoff.create_on_session_end"
	keyMidSession    = "routing.mid_session"
	keyAutoScopeMode = "auto_scope.mode"
	keySanitizeAllow = "sanitize.allowlist"
	keySanitizeExtra = "sanitize.extra_patterns"
	keyHome          = "home" // AI_MEMORY_HOME: não é chave de configuração, só o nome no ambiente
	envClaudeConfig  = "CLAUDE_CONFIG_DIR"
	defaultBind      = "127.0.0.1:49374"
	defaultServerURL = "http://127.0.0.1:49374"
	appDirName       = "ai-memory"
	configFileName   = "config.toml"
	claudeDirName    = ".claude"
)

// userDataDir devolve o diretório de dados local do usuário, sem a subpasta
// ai-memory. Os testes o substituem para não ler o diretório real da máquina.
var userDataDir = defaultUserDataDir

// userHomeDir devolve o diretório pessoal do sistema. Os testes o substituem.
var userHomeDir = os.UserHomeDir

// Config é a configuração resolvida do processo. Os campos são não exportados e
// não há setters: após Load, o valor só é lido pelos getters.
type Config struct {
	dataDir        string
	logLevel       string
	watchDebounce  time.Duration
	reconcileEvery time.Duration
	bind           string
	serverURL      string
	allowedHosts   []string
	authToken      string
	claimOnStart   bool
	createOnEnd    bool
	midSession     string
	autoScopeMode  string
	homeDir        string
	claudeConfig   string
	sanitizeAllow  []string
	sanitizeExtra  []string
}

// DataDir devolve o diretório de dados absoluto e canônico (INV-11).
func (c Config) DataDir() string { return c.dataDir }

// LogLevel devolve o nível de log: debug, info, warn ou error.
func (c Config) LogLevel() string { return c.logLevel }

// WatchDebounce devolve o debounce do watcher da wiki.
func (c Config) WatchDebounce() time.Duration { return c.watchDebounce }

// ReconcileInterval devolve o intervalo da reconciliação da wiki.
func (c Config) ReconcileInterval() time.Duration { return c.reconcileEvery }

// Bind devolve o endereço de escuta HTTP, no formato host:porta.
func (c Config) Bind() string { return c.bind }

// ServerURL devolve a URL do servidor usada pelos clientes.
func (c Config) ServerURL() string { return c.serverURL }

// AllowedHosts devolve a cópia da lista de valores de Host aceitos, em minúsculas.
func (c Config) AllowedHosts() []string { return append([]string(nil), c.allowedHosts...) }

// AuthToken devolve o token bearer estático. Vazio significa sem token.
func (c Config) AuthToken() string { return c.authToken }

// ClaimOnSessionStart diz se o início de sessão reivindica o handoff pendente.
// Falso é o modo oferta.
func (c Config) ClaimOnSessionStart() bool { return c.claimOnStart }

// CreateOnSessionEnd diz se o fim de sessão cria handoff automático.
func (c Config) CreateOnSessionEnd() bool { return c.createOnEnd }

// MidSession devolve a regra de roteamento durante a sessão: follow-cwd ou sticky.
func (c Config) MidSession() string { return c.midSession }

// AutoScopeMode devolve o modo de isolamento do projeto ativo: per_actor,
// per_session ou single.
func (c Config) AutoScopeMode() string { return c.autoScopeMode }

// HomeDir devolve o diretório pessoal canônico: AI_MEMORY_HOME, se não estiver
// vazio, ou o diretório pessoal do sistema.
func (c Config) HomeDir() string { return c.homeDir }

// ClaudeConfigDir devolve a raiz de configuração do Claude Code: CLAUDE_CONFIG_DIR,
// se não estiver vazio, ou <diretório pessoal>/.claude.
func (c Config) ClaudeConfigDir() string { return c.claudeConfig }

// SanitizeAllowlist devolve a cópia de [sanitize].allowlist: trechos que o sanitizador não redige
// quando a ocorrência contém um deles (docs/prompts-schemas-and-sanitizer.md §11.3).
func (c Config) SanitizeAllowlist() []string { return append([]string(nil), c.sanitizeAllow...) }

// SanitizeExtraPatterns devolve a cópia de [sanitize].extra_patterns: expressões RE2 extras do
// operador, com rótulo custom (docs/domain-vocabulary.md §7.3).
func (c Config) SanitizeExtraPatterns() []string { return append([]string(nil), c.sanitizeExtra...) }

// Options define os caminhos das fontes de arquivo e a flag global de diretório de
// dados. Zero value: diretório de dados padrão, TOML em <diretório de dados>/config.toml,
// e o .env vem de AI_MEMORY_ENV_FILE ou, se ausente, de <diretório de trabalho>/.env.
type Options struct {
	// DataDir é a flag --data-dir. Vazio não tem efeito. Tem a maior precedência para data_dir.
	DataDir string
	// ConfigFile é o caminho do TOML. Vazio usa <diretório de dados>/config.toml.
	// Arquivo inexistente significa sem essa camada.
	ConfigFile string
	// EnvFile é o caminho do .env. Vazio usa AI_MEMORY_ENV_FILE ou <diretório de trabalho>/.env.
	EnvFile string
}

// Load resolve a configuração na ordem padrões < TOML < .env < AI_MEMORY_* < --data-dir.
// Para localizar o TOML padrão, o diretório de dados é resolvido antes do TOML, na ordem
// --data-dir > AI_MEMORY_DATA_DIR (processo ou .env) > padrão da plataforma. Assim um
// data_dir dentro do próprio TOML não muda o arquivo que é lido. Arquivos ausentes são
// opcionais e não são erro.
func Load(opts Options) (Config, error) {
	procEnv := koanf.New(".")
	if err := procEnv.Load(env.Provider(envPrefix, ".", envKey), nil); err != nil {
		return Config{}, fmt.Errorf("config: lendo variáveis %s*: %w", envPrefix, err)
	}

	envFile, err := resolveEnvFile(opts.EnvFile, procEnv.String(envFileKey))
	if err != nil {
		return Config{}, err
	}

	// Camada de ambiente: .env e depois variáveis do processo, que vencem o .env.
	envLayer := koanf.New(".")
	if err := loadOptional(envLayer, envFile, dotenv.ParserEnv(envPrefix, ".", envKey)); err != nil {
		return Config{}, fmt.Errorf("config: .env %q: %w", envFile, err)
	}
	if err := envLayer.Merge(procEnv); err != nil {
		return Config{}, fmt.Errorf("config: variáveis %s*: %w", envPrefix, err)
	}

	defDir, err := defaultDataDir()
	if err != nil {
		return Config{}, err
	}
	dataDirSrc := opts.DataDir
	if dataDirSrc == "" {
		dataDirSrc = envLayer.String(keyDataDir)
	}
	if dataDirSrc == "" {
		dataDirSrc = defDir
	}
	configFile := opts.ConfigFile
	if configFile == "" {
		configFile = filepath.Join(dataDirSrc, configFileName)
	}

	k := koanf.New(".")
	if err := k.Load(confmap.Provider(defaults(defDir), "."), nil); err != nil {
		return Config{}, fmt.Errorf("config: padrões: %w", err)
	}
	if err := loadOptional(k, configFile, toml.Parser()); err != nil {
		return Config{}, fmt.Errorf("config: TOML %q: %w", configFile, err)
	}
	if err := k.Merge(envLayer); err != nil {
		return Config{}, fmt.Errorf("config: variáveis %s*: %w", envPrefix, err)
	}
	if opts.DataDir != "" {
		if err := k.Set(keyDataDir, opts.DataDir); err != nil {
			return Config{}, fmt.Errorf("config: --data-dir: %w", err)
		}
	}

	// Ambiente capturado uma única vez, junto com a carga (INV-01).
	amb := ambientEnv{
		home:         envLayer.String(keyHome),
		claudeConfig: os.Getenv(envClaudeConfig),
	}
	return build(k, amb)
}

// ambientEnv guarda as variáveis de ambiente que não são chaves de configuração.
type ambientEnv struct {
	home         string // AI_MEMORY_HOME, já com a precedência de .env e processo
	claudeConfig string // CLAUDE_CONFIG_DIR
}

// defaultDataDir devolve <diretório de dados local do usuário>/ai-memory. Sem diretório
// local informado pela plataforma, o padrão é o diretório atual.
func defaultDataDir() (string, error) {
	base, err := userDataDir()
	if err != nil || base == "" {
		return ".", nil
	}
	return filepath.Join(base, appDirName), nil
}

// defaultUserDataDir segue o padrão de cada plataforma:
// macOS ~/Library/Application Support; Windows %LOCALAPPDATA% (ou ~/AppData/Local);
// demais, $XDG_DATA_HOME ou ~/.local/share.
func defaultUserDataDir() (string, error) {
	home, homeErr := os.UserHomeDir()
	switch runtime.GOOS {
	case "windows":
		if local, err := os.UserCacheDir(); err == nil {
			return local, nil
		}
		if homeErr != nil {
			return "", homeErr
		}
		return filepath.Join(home, "AppData", "Local"), nil
	case "darwin":
		if homeErr != nil {
			return "", homeErr
		}
		return filepath.Join(home, "Library", "Application Support"), nil
	default:
		if xdg := os.Getenv("XDG_DATA_HOME"); xdg != "" && filepath.IsAbs(xdg) {
			return xdg, nil
		}
		if homeErr != nil {
			return "", homeErr
		}
		return filepath.Join(home, ".local", "share"), nil
	}
}

func defaults(dataDir string) map[string]interface{} {
	return map[string]interface{}{
		"data_dir": dataDir,
		"log": map[string]interface{}{
			"level": "info",
		},
		"wiki": map[string]interface{}{
			"watch_debounce_ms":    300,
			"reconcile_interval_s": 30,
		},
		keyBind:         defaultBind,
		keyServerURL:    defaultServerURL,
		keyAllowedHosts: []string{"localhost", "127.0.0.1", "::1"},
		"handoff": map[string]interface{}{
			"claim_on_session_start": true,
			"create_on_session_end":  true,
		},
		"routing": map[string]interface{}{
			"mid_session": "follow-cwd",
		},
		"auto_scope": map[string]interface{}{
			"mode": "per_actor",
		},
		"sanitize": map[string]interface{}{
			"allowlist":      []string{},
			"extra_patterns": []string{},
		},
	}
}

// envKey converte AI_MEMORY_LOG__LEVEL em log.level. AI_MEMORY_AUTH_TOKEN é a
// sobreposição explícita do token bearer (docs/cli.md §2.1) e vai para auth.bearer_token.
func envKey(s string) string {
	key := strings.TrimPrefix(s, envPrefix)
	if key == "AUTH_TOKEN" {
		return keyAuthToken
	}
	return strings.ToLower(strings.ReplaceAll(key, "__", "."))
}

func resolveEnvFile(explicit, fromProcess string) (string, error) {
	if explicit != "" {
		return explicit, nil
	}
	if fromProcess != "" {
		return fromProcess, nil
	}
	wd, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("config: diretório de trabalho: %w", err)
	}
	return filepath.Join(wd, ".env"), nil
}

// loadOptional carrega a fonte só se o caminho foi dado e o arquivo existe.
func loadOptional(k *koanf.Koanf, path string, p koanf.Parser) error {
	if path == "" {
		return nil
	}
	if _, err := os.Stat(path); errors.Is(err, fs.ErrNotExist) {
		return nil
	} else if err != nil {
		return err
	}
	return k.Load(file.Provider(path), p)
}

func build(k *koanf.Koanf, amb ambientEnv) (Config, error) {
	raw := k.String(keyDataDir)
	if raw == "" {
		return Config{}, fmt.Errorf("config: %s vazio", keyDataDir)
	}
	dataDir, err := canonicalPath(raw)
	if err != nil {
		return Config{}, fmt.Errorf("config: %s %q: %w", keyDataDir, raw, err)
	}

	level := k.String(keyLogLevel)
	switch level {
	case "debug", "info", "warn", "error":
	default:
		return Config{}, fmt.Errorf("config: %s inválido %q (use debug|info|warn|error)", keyLogLevel, level)
	}

	debounceMS, err := positiveInt(k, keyWatchDebounce)
	if err != nil {
		return Config{}, err
	}
	reconcileS, err := positiveInt(k, keyReconcile)
	if err != nil {
		return Config{}, err
	}

	bind := k.String(keyBind)
	if err := validBind(bind); err != nil {
		return Config{}, err
	}
	serverURL := k.String(keyServerURL)
	if err := validServerURL(serverURL); err != nil {
		return Config{}, err
	}
	hosts, err := hostList(k, keyAllowedHosts)
	if err != nil {
		return Config{}, err
	}
	claim, err := boolValue(k, keyClaimOnStart)
	if err != nil {
		return Config{}, err
	}
	create, err := boolValue(k, keyCreateOnEnd)
	if err != nil {
		return Config{}, err
	}
	midSession, err := choice(k, keyMidSession, "follow-cwd", "sticky")
	if err != nil {
		return Config{}, err
	}
	autoScope, err := choice(k, keyAutoScopeMode, "per_actor", "per_session", "single")
	if err != nil {
		return Config{}, err
	}
	allow, err := sanitizeAllowlist(k, keySanitizeAllow)
	if err != nil {
		return Config{}, err
	}
	extra, err := sanitizeExtraPatterns(k, keySanitizeExtra)
	if err != nil {
		return Config{}, err
	}

	home, err := resolveHome(amb.home)
	if err != nil {
		return Config{}, err
	}
	claudeDir, err := resolveClaudeConfigDir(amb.claudeConfig, home)
	if err != nil {
		return Config{}, err
	}

	return Config{
		dataDir:        dataDir,
		logLevel:       level,
		watchDebounce:  time.Duration(debounceMS) * time.Millisecond,
		reconcileEvery: time.Duration(reconcileS) * time.Second,
		bind:           bind,
		serverURL:      serverURL,
		allowedHosts:   hosts,
		authToken:      k.String(keyAuthToken),
		claimOnStart:   claim,
		createOnEnd:    create,
		midSession:     midSession,
		autoScopeMode:  autoScope,
		homeDir:        home,
		claudeConfig:   claudeDir,
		sanitizeAllow:  allow,
		sanitizeExtra:  extra,
	}, nil
}

// textList lê uma lista de textos do TOML. Texto solto (por exemplo, de variável de ambiente) é
// erro: a lista não tem separador seguro, porque expressões podem conter vírgula.
func textList(k *koanf.Koanf, path string) ([]string, error) {
	switch v := k.Get(path).(type) {
	case nil:
		return nil, nil
	case []string:
		return append([]string(nil), v...), nil
	case []interface{}:
		out := make([]string, 0, len(v))
		for _, item := range v {
			s, ok := item.(string)
			if !ok {
				return nil, fmt.Errorf("config: %s deve conter só textos, veio %T", path, item)
			}
			out = append(out, s)
		}
		return out, nil
	case string:
		return nil, fmt.Errorf("config: %s deve ser uma lista TOML, não texto (variável de ambiente não define essa lista)", path)
	default:
		return nil, fmt.Errorf("config: %s com tipo inválido %T", path, v)
	}
}

// sanitizeAllowlist lê [sanitize].allowlist. Entrada vazia é erro: ela casaria toda ocorrência e
// desligaria a redação.
func sanitizeAllowlist(k *koanf.Koanf, path string) ([]string, error) {
	list, err := textList(k, path)
	if err != nil {
		return nil, err
	}
	for i, s := range list {
		if s == "" {
			return nil, fmt.Errorf("config: %s[%d] vazio: liberaria todo o texto", path, i)
		}
	}
	return list, nil
}

// sanitizeExtraPatterns lê [sanitize].extra_patterns e exige que cada uma compile em RE2. Um
// padrão inválido impede o início do serviço (docs/domain-vocabulary.md §7.3).
func sanitizeExtraPatterns(k *koanf.Koanf, path string) ([]string, error) {
	list, err := textList(k, path)
	if err != nil {
		return nil, err
	}
	for i, p := range list {
		if p == "" {
			return nil, fmt.Errorf("config: %s[%d] vazio", path, i)
		}
		if _, err := regexp.Compile(p); err != nil {
			return nil, fmt.Errorf("config: %s[%d] inválido %q: %w", path, i, p, err)
		}
	}
	return list, nil
}

// validBind exige host:porta com porta de 1 a 65535. O host pode ser vazio.
func validBind(addr string) error {
	_, port, err := net.SplitHostPort(addr)
	if err != nil {
		return fmt.Errorf("config: %s %q: %w", keyBind, addr, err)
	}
	n, err := strconv.Atoi(port)
	if err != nil || n < 1 || n > 65535 {
		return fmt.Errorf("config: %s %q: porta deve estar entre 1 e 65535", keyBind, addr)
	}
	return nil
}

// validServerURL exige esquema http ou https e um host.
func validServerURL(raw string) error {
	u, err := url.Parse(raw)
	if err != nil {
		return fmt.Errorf("config: %s %q: %w", keyServerURL, raw, err)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return fmt.Errorf("config: %s %q: esquema deve ser http ou https", keyServerURL, raw)
	}
	if u.Host == "" {
		return fmt.Errorf("config: %s %q: falta o host", keyServerURL, raw)
	}
	return nil
}

// hostList aceita lista TOML ou texto separado por vírgulas (AI_MEMORY_ALLOWED_HOSTS).
// Normaliza para minúsculas e descarta entradas vazias. Curinga e caminho são erro,
// porque a lista não tem curinga (docs/https-via-proxy.md).
func hostList(k *koanf.Koanf, path string) ([]string, error) {
	var raw []string
	switch v := k.Get(path).(type) {
	case string:
		raw = strings.Split(v, ",")
	case []string:
		raw = v
	case []interface{}:
		for _, item := range v {
			s, ok := item.(string)
			if !ok {
				return nil, fmt.Errorf("config: %s deve conter só textos, veio %T", path, item)
			}
			raw = append(raw, s)
		}
	default:
		return nil, fmt.Errorf("config: %s com tipo inválido %T", path, v)
	}

	var hosts []string
	for _, h := range raw {
		h = strings.ToLower(strings.TrimSpace(h))
		if h == "" {
			continue
		}
		if h == "*" || strings.ContainsAny(h, "/ ") {
			return nil, fmt.Errorf("config: %s entrada %q inválida (sem curinga, caminho ou espaço)", path, h)
		}
		hosts = append(hosts, h)
	}
	if len(hosts) == 0 {
		return nil, fmt.Errorf("config: %s vazio: nenhum Host seria aceito", path)
	}
	return hosts, nil
}

// boolValue lê um booleano de TOML ou de texto de ambiente (true/false).
func boolValue(k *koanf.Koanf, path string) (bool, error) {
	switch v := k.Get(path).(type) {
	case bool:
		return v, nil
	case string:
		b, err := strconv.ParseBool(strings.TrimSpace(v))
		if err != nil {
			return false, fmt.Errorf("config: %s inválido %q (use true|false)", path, v)
		}
		return b, nil
	default:
		return false, fmt.Errorf("config: %s com tipo inválido %T (use true|false)", path, v)
	}
}

// choice exige um dos valores permitidos, sem normalizar maiúsculas.
func choice(k *koanf.Koanf, path string, allowed ...string) (string, error) {
	v := k.String(path)
	for _, a := range allowed {
		if v == a {
			return v, nil
		}
	}
	return "", fmt.Errorf("config: %s inválido %q (use %s)", path, v, strings.Join(allowed, "|"))
}

// resolveHome usa AI_MEMORY_HOME se não estiver vazio; senão, o diretório pessoal do
// sistema. Devolve o caminho absoluto canônico.
func resolveHome(ambient string) (string, error) {
	raw := strings.TrimSpace(ambient)
	if raw == "" {
		h, err := userHomeDir()
		if err != nil {
			return "", fmt.Errorf("config: diretório pessoal: %w", err)
		}
		raw = h
	}
	home, err := canonicalPath(raw)
	if err != nil {
		return "", fmt.Errorf("config: diretório pessoal %q: %w", raw, err)
	}
	return home, nil
}

// resolveClaudeConfigDir usa CLAUDE_CONFIG_DIR se não estiver vazio (espaços contam
// como vazio); senão, <diretório pessoal>/.claude.
func resolveClaudeConfigDir(ambient, home string) (string, error) {
	raw := strings.TrimSpace(ambient)
	if raw == "" {
		return filepath.Join(home, claudeDirName), nil
	}
	dir, err := canonicalPath(raw)
	if err != nil {
		return "", fmt.Errorf("config: %s %q: %w", envClaudeConfig, raw, err)
	}
	return dir, nil
}

func positiveInt(k *koanf.Koanf, path string) (int, error) {
	var n int
	switch v := k.Get(path).(type) {
	case int:
		n = v
	case int64:
		n = int(v)
	case string:
		parsed, err := strconv.Atoi(v)
		if err != nil {
			return 0, fmt.Errorf("config: %s inválido %q: %w", path, v, err)
		}
		n = parsed
	default:
		return 0, fmt.Errorf("config: %s com tipo inválido %T", path, v)
	}
	if n <= 0 {
		return 0, fmt.Errorf("config: %s deve ser positivo, veio %d", path, n)
	}
	return n, nil
}

// canonicalPath devolve o caminho absoluto com links simbólicos resolvidos.
// Se o caminho ainda não existe, canoniza o ancestral mais próximo e junta o resto.
func canonicalPath(p string) (string, error) {
	abs, err := filepath.Abs(p)
	if err != nil {
		return "", err
	}
	var rest []string
	cur := abs
	for {
		resolved, err := filepath.EvalSymlinks(cur)
		if err == nil {
			return filepath.Join(append([]string{resolved}, rest...)...), nil
		}
		if !errors.Is(err, fs.ErrNotExist) {
			return "", err
		}
		parent := filepath.Dir(cur)
		if parent == cur {
			return "", err
		}
		rest = append([]string{filepath.Base(cur)}, rest...)
		cur = parent
	}
}
