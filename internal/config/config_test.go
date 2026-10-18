package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// configEnvKeys são as variáveis que os testes controlam. Limpá-las evita que
// o ambiente da máquina do desenvolvedor mude o resultado.
var configEnvKeys = []string{
	"AI_MEMORY_DATA_DIR",
	"AI_MEMORY_LOG__LEVEL",
	"AI_MEMORY_LOG_LEVEL",
	"AI_MEMORY_WIKI__WATCH_DEBOUNCE_MS",
	"AI_MEMORY_WIKI__RECONCILE_INTERVAL_S",
	"AI_MEMORY_ENV_FILE",
	"AI_MEMORY_BIND",
	"AI_MEMORY_SERVER_URL",
	"AI_MEMORY_ALLOWED_HOSTS",
	"AI_MEMORY_AUTH_TOKEN",
	"AI_MEMORY_HANDOFF__CLAIM_ON_SESSION_START",
	"AI_MEMORY_HANDOFF__CREATE_ON_SESSION_END",
	"AI_MEMORY_ROUTING__MID_SESSION",
	"AI_MEMORY_AUTO_SCOPE__MODE",
	"AI_MEMORY_SANITIZE__ALLOWLIST",
	"AI_MEMORY_SANITIZE__EXTRA_PATTERNS",
	"AI_MEMORY_HOME",
	"CLAUDE_CONFIG_DIR",
}

// isolateEnv remove as variáveis de config do processo e troca o diretório local
// do usuário por uma pasta temporária. Devolve essa pasta base: o padrão de dados
// é base/ai-memory, e o diretório pessoal do sistema é base/home (ainda não criado).
// t.Setenv restaura o valor original ao fim do teste.
func isolateEnv(t *testing.T) string {
	t.Helper()
	for _, k := range configEnvKeys {
		t.Setenv(k, "")
		_ = os.Unsetenv(k)
	}
	base := t.TempDir()
	prev := userDataDir
	userDataDir = func() (string, error) { return base, nil }
	t.Cleanup(func() { userDataDir = prev })
	prevHome := userHomeDir
	userHomeDir = func() (string, error) { return filepath.Join(base, "home"), nil }
	t.Cleanup(func() { userHomeDir = prevHome })
	return base
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func canonicalDir(t *testing.T, p string) string {
	t.Helper()
	resolved, err := filepath.EvalSymlinks(p)
	if err != nil {
		t.Fatal(err)
	}
	return resolved
}

func TestDefaults(t *testing.T) {
	base := isolateEnv(t)
	tmp := t.TempDir()
	t.Chdir(tmp)

	cfg, err := Load(Options{})
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if want := filepath.Join(canonicalDir(t, base), "ai-memory"); cfg.DataDir() != want {
		t.Errorf("DataDir = %q, quer %q (diretório local do usuário + ai-memory)", cfg.DataDir(), want)
	}
	if cfg.LogLevel() != "info" {
		t.Errorf("LogLevel = %q, quer info", cfg.LogLevel())
	}
	if cfg.WatchDebounce() != 300*time.Millisecond {
		t.Errorf("WatchDebounce = %v, quer 300ms", cfg.WatchDebounce())
	}
	if cfg.ReconcileInterval() != 30*time.Second {
		t.Errorf("ReconcileInterval = %v, quer 30s", cfg.ReconcileInterval())
	}
}

// TestPrecedencia cobre padrões < TOML < .env < AI_MEMORY_*. Cada passo
// acrescenta uma camada e sobrescreve as chaves que ela define.
func TestPrecedencia(t *testing.T) {
	steps := []struct {
		name      string
		toml      string
		dotenv    string
		proc      map[string]string
		wantDir   string
		wantLevel string
		wantDur   time.Duration
	}{
		{
			name:      "só padrões",
			wantDir:   "ai-memory",
			wantLevel: "info",
			wantDur:   300 * time.Millisecond,
		},
		{
			name:      "TOML sobrescreve padrões",
			toml:      "data_dir = \"d-toml\"\n[log]\nlevel = \"warn\"\n[wiki]\nwatch_debounce_ms = 100\n",
			wantDir:   "d-toml",
			wantLevel: "warn",
			wantDur:   100 * time.Millisecond,
		},
		{
			name:      ".env sobrescreve TOML",
			toml:      "data_dir = \"d-toml\"\n[log]\nlevel = \"warn\"\n[wiki]\nwatch_debounce_ms = 100\n",
			dotenv:    "AI_MEMORY_DATA_DIR=d-dotenv\nAI_MEMORY_LOG__LEVEL=error\nAI_MEMORY_WIKI__WATCH_DEBOUNCE_MS=200\n",
			wantDir:   "d-dotenv",
			wantLevel: "error",
			wantDur:   200 * time.Millisecond,
		},
		{
			name:      "processo sobrescreve .env",
			toml:      "data_dir = \"d-toml\"\n[log]\nlevel = \"warn\"\n[wiki]\nwatch_debounce_ms = 100\n",
			dotenv:    "AI_MEMORY_DATA_DIR=d-dotenv\nAI_MEMORY_LOG__LEVEL=error\nAI_MEMORY_WIKI__WATCH_DEBOUNCE_MS=200\n",
			proc:      map[string]string{"AI_MEMORY_DATA_DIR": "d-proc", "AI_MEMORY_LOG__LEVEL": "debug", "AI_MEMORY_WIKI__WATCH_DEBOUNCE_MS": "250"},
			wantDir:   "d-proc",
			wantLevel: "debug",
			wantDur:   250 * time.Millisecond,
		},
	}

	for _, s := range steps {
		t.Run(s.name, func(t *testing.T) {
			isolateEnv(t)
			tmp := t.TempDir()
			t.Chdir(tmp)

			tomlPath := filepath.Join(tmp, "config.toml")
			if s.toml != "" {
				writeFile(t, tomlPath, s.toml)
			}
			envPath := filepath.Join(tmp, "fonte.env")
			if s.dotenv != "" {
				writeFile(t, envPath, s.dotenv)
			}
			for k, v := range s.proc {
				t.Setenv(k, v)
			}

			cfg, err := Load(Options{ConfigFile: tomlPath, EnvFile: envPath})
			if err != nil {
				t.Fatalf("Load: %v", err)
			}
			if got := filepath.Base(cfg.DataDir()); got != s.wantDir {
				t.Errorf("DataDir base = %q, quer %q", got, s.wantDir)
			}
			if cfg.LogLevel() != s.wantLevel {
				t.Errorf("LogLevel = %q, quer %q", cfg.LogLevel(), s.wantLevel)
			}
			if cfg.WatchDebounce() != s.wantDur {
				t.Errorf("WatchDebounce = %v, quer %v", cfg.WatchDebounce(), s.wantDur)
			}
		})
	}
}

func TestSeparadorDuplo(t *testing.T) {
	isolateEnv(t)
	t.Chdir(t.TempDir())
	t.Setenv("AI_MEMORY_WIKI__RECONCILE_INTERVAL_S", "45")
	// Um único "_" não aninha: AI_MEMORY_LOG_LEVEL não vira log.level.
	t.Setenv("AI_MEMORY_LOG_LEVEL", "debug")

	cfg, err := Load(Options{})
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.ReconcileInterval() != 45*time.Second {
		t.Errorf("ReconcileInterval = %v, quer 45s (AI_MEMORY_WIKI__RECONCILE_INTERVAL_S)", cfg.ReconcileInterval())
	}
	if cfg.LogLevel() != "info" {
		t.Errorf("LogLevel = %q, quer info: AI_MEMORY_LOG_LEVEL não deve aninhar", cfg.LogLevel())
	}
}

func TestEnvAusenteNaoQuebra(t *testing.T) {
	t.Run("sem .env na raiz", func(t *testing.T) {
		isolateEnv(t)
		t.Chdir(t.TempDir())
		if _, err := Load(Options{}); err != nil {
			t.Fatalf("Load sem .env: %v", err)
		}
	})

	t.Run("AI_MEMORY_ENV_FILE aponta para inexistente", func(t *testing.T) {
		isolateEnv(t)
		tmp := t.TempDir()
		t.Chdir(tmp)
		t.Setenv("AI_MEMORY_ENV_FILE", filepath.Join(tmp, "nao-existe.env"))
		cfg, err := Load(Options{ConfigFile: filepath.Join(tmp, "nao-existe.toml")})
		if err != nil {
			t.Fatalf("Load com caminhos inexistentes: %v", err)
		}
		if cfg.LogLevel() != "info" {
			t.Errorf("LogLevel = %q, quer padrão info", cfg.LogLevel())
		}
	})
}

func TestEnvPadraoNaRaizDoRepo(t *testing.T) {
	isolateEnv(t)
	tmp := t.TempDir()
	t.Chdir(tmp)
	writeFile(t, filepath.Join(tmp, ".env"), "AI_MEMORY_LOG__LEVEL=error\n")

	cfg, err := Load(Options{})
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.LogLevel() != "error" {
		t.Errorf("LogLevel = %q, quer error vindo do .env da raiz", cfg.LogLevel())
	}
}

func TestEnvFileViaProcesso(t *testing.T) {
	isolateEnv(t)
	tmp := t.TempDir()
	t.Chdir(tmp)
	// Um .env na raiz existe, mas AI_MEMORY_ENV_FILE tem prioridade na escolha do arquivo.
	writeFile(t, filepath.Join(tmp, ".env"), "AI_MEMORY_LOG__LEVEL=error\n")
	alt := filepath.Join(tmp, "alt.env")
	writeFile(t, alt, "AI_MEMORY_LOG__LEVEL=warn\n")
	t.Setenv("AI_MEMORY_ENV_FILE", alt)

	cfg, err := Load(Options{})
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.LogLevel() != "warn" {
		t.Errorf("LogLevel = %q, quer warn do AI_MEMORY_ENV_FILE", cfg.LogLevel())
	}
}

func TestDataDirRelativoViraAbsolutoCanonico(t *testing.T) {
	isolateEnv(t)
	tmp := t.TempDir()
	t.Chdir(tmp)
	tomlPath := filepath.Join(tmp, "config.toml")
	writeFile(t, tomlPath, "data_dir = \"dados/wiki\"\n")

	cfg, err := Load(Options{ConfigFile: tomlPath})
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if !filepath.IsAbs(cfg.DataDir()) {
		t.Fatalf("DataDir %q não é absoluto", cfg.DataDir())
	}
	want := filepath.Join(canonicalDir(t, tmp), "dados", "wiki")
	if cfg.DataDir() != want {
		t.Errorf("DataDir = %q, quer %q", cfg.DataDir(), want)
	}
}

func TestDataDirResolveLinkSimbolico(t *testing.T) {
	isolateEnv(t)
	tmp := t.TempDir()
	t.Chdir(tmp)
	dirReal := filepath.Join(tmp, "real")
	if err := os.MkdirAll(dirReal, 0o755); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(tmp, "link")
	if err := os.Symlink(dirReal, link); err != nil {
		t.Skipf("symlink indisponível neste ambiente: %v", err)
	}
	// "novo" ainda não existe: o ancestral "link" é canonizado e o resto é juntado.
	t.Setenv("AI_MEMORY_DATA_DIR", "link/novo")

	cfg, err := Load(Options{})
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	want := filepath.Join(canonicalDir(t, dirReal), "novo")
	if cfg.DataDir() != want {
		t.Errorf("DataDir = %q, quer %q", cfg.DataDir(), want)
	}
}

// TestPrecedenciaDataDir cobre --data-dir > AI_MEMORY_DATA_DIR > config.toml > padrão.
func TestPrecedenciaDataDir(t *testing.T) {
	cases := []struct {
		name string
		flag string // --data-dir
		proc string // AI_MEMORY_DATA_DIR no processo
		toml string // data_dir no config.toml passado por --config
		// want é caminho relativo ao diretório de trabalho; vazio = padrão da plataforma.
		want string
	}{
		{name: "padrão quando nada é informado", want: ""},
		{name: "config.toml sobrescreve o padrão", toml: "d-toml", want: "d-toml"},
		{name: "AI_MEMORY_DATA_DIR vence o config.toml", proc: "d-proc", toml: "d-toml", want: "d-proc"},
		{name: "--data-dir vence AI_MEMORY_DATA_DIR", flag: "d-flag", proc: "d-proc", toml: "d-toml", want: "d-flag"},
		{name: "--data-dir vence o config.toml", flag: "d-flag", toml: "d-toml", want: "d-flag"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			base := isolateEnv(t)
			tmp := t.TempDir()
			t.Chdir(tmp)
			tomlPath := filepath.Join(tmp, "config.toml")
			if c.toml != "" {
				writeFile(t, tomlPath, "data_dir = \""+c.toml+"\"\n")
			}
			if c.proc != "" {
				t.Setenv("AI_MEMORY_DATA_DIR", c.proc)
			}

			cfg, err := Load(Options{DataDir: c.flag, ConfigFile: tomlPath, EnvFile: filepath.Join(tmp, "ausente.env")})
			if err != nil {
				t.Fatalf("Load: %v", err)
			}
			want := filepath.Join(canonicalDir(t, base), "ai-memory")
			if c.want != "" {
				want = filepath.Join(canonicalDir(t, tmp), c.want)
			}
			if cfg.DataDir() != want {
				t.Errorf("DataDir = %q, quer %q", cfg.DataDir(), want)
			}
		})
	}
}

// TestDataDirSemLocalDoUsuarioUsaCwd cobre o fallback do doc: sem diretório local,
// o padrão é o diretório atual.
func TestDataDirSemLocalDoUsuarioUsaCwd(t *testing.T) {
	isolateEnv(t)
	tmp := t.TempDir()
	t.Chdir(tmp)
	userDataDir = func() (string, error) { return "", os.ErrNotExist }

	cfg, err := Load(Options{})
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if want := canonicalDir(t, tmp); cfg.DataDir() != want {
		t.Errorf("DataDir = %q, quer %q (diretório atual)", cfg.DataDir(), want)
	}
}

// TestConfigTomlNoDataDir cobre o config.toml lido do diretório de dados quando
// --config não é informado, e a prioridade do --config explícito sobre ele.
func TestConfigTomlNoDataDir(t *testing.T) {
	t.Run("--data-dir sem --config lê config.toml dele", func(t *testing.T) {
		isolateEnv(t)
		tmp := t.TempDir()
		t.Chdir(tmp)
		dir := filepath.Join(tmp, "dados")
		writeFile(t, filepath.Join(dir, "config.toml"), "[log]\nlevel = \"warn\"\n")

		cfg, err := Load(Options{DataDir: dir})
		if err != nil {
			t.Fatalf("Load: %v", err)
		}
		if cfg.LogLevel() != "warn" {
			t.Errorf("LogLevel = %q, quer warn do config.toml do diretório de dados", cfg.LogLevel())
		}
	})

	t.Run("AI_MEMORY_DATA_DIR sem --config lê config.toml dele", func(t *testing.T) {
		isolateEnv(t)
		tmp := t.TempDir()
		t.Chdir(tmp)
		dir := filepath.Join(tmp, "dados")
		writeFile(t, filepath.Join(dir, "config.toml"), "[log]\nlevel = \"warn\"\n")
		t.Setenv("AI_MEMORY_DATA_DIR", dir)

		cfg, err := Load(Options{})
		if err != nil {
			t.Fatalf("Load: %v", err)
		}
		if cfg.LogLevel() != "warn" {
			t.Errorf("LogLevel = %q, quer warn do config.toml do diretório de dados", cfg.LogLevel())
		}
	})

	t.Run("padrão sem --config lê config.toml do diretório padrão", func(t *testing.T) {
		base := isolateEnv(t)
		tmp := t.TempDir()
		t.Chdir(tmp)
		writeFile(t, filepath.Join(base, "ai-memory", "config.toml"), "[log]\nlevel = \"error\"\n")

		cfg, err := Load(Options{})
		if err != nil {
			t.Fatalf("Load: %v", err)
		}
		if cfg.LogLevel() != "error" {
			t.Errorf("LogLevel = %q, quer error do config.toml do padrão", cfg.LogLevel())
		}
	})

	t.Run("--config explícito vence o do diretório de dados", func(t *testing.T) {
		isolateEnv(t)
		tmp := t.TempDir()
		t.Chdir(tmp)
		dir := filepath.Join(tmp, "dados")
		writeFile(t, filepath.Join(dir, "config.toml"), "[log]\nlevel = \"warn\"\n")
		explicit := filepath.Join(tmp, "outro.toml")
		writeFile(t, explicit, "[log]\nlevel = \"debug\"\n")

		cfg, err := Load(Options{DataDir: dir, ConfigFile: explicit})
		if err != nil {
			t.Fatalf("Load: %v", err)
		}
		if cfg.LogLevel() != "debug" {
			t.Errorf("LogLevel = %q, quer debug do --config", cfg.LogLevel())
		}
	})
}

func TestLogLevelInvalidoFalha(t *testing.T) {
	isolateEnv(t)
	t.Chdir(t.TempDir())
	t.Setenv("AI_MEMORY_LOG__LEVEL", "trace")

	if _, err := Load(Options{}); err == nil {
		t.Fatal("esperava erro para log.level inválido")
	}
}

// TestDefaultsNovosCampos cobre os padrões dos campos de servidor, cliente, handoff,
// roteamento, auto_scope e diretórios capturados.
func TestDefaultsNovosCampos(t *testing.T) {
	base := isolateEnv(t)
	t.Chdir(t.TempDir())

	cfg, err := Load(Options{})
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Bind() != "127.0.0.1:49374" {
		t.Errorf("Bind = %q, quer 127.0.0.1:49374", cfg.Bind())
	}
	if cfg.ServerURL() != "http://127.0.0.1:49374" {
		t.Errorf("ServerURL = %q, quer http://127.0.0.1:49374", cfg.ServerURL())
	}
	if want := []string{"localhost", "127.0.0.1", "::1"}; !equalStrings(cfg.AllowedHosts(), want) {
		t.Errorf("AllowedHosts = %v, quer %v", cfg.AllowedHosts(), want)
	}
	if cfg.AuthToken() != "" {
		t.Errorf("AuthToken = %q, quer vazio", cfg.AuthToken())
	}
	if !cfg.ClaimOnSessionStart() {
		t.Error("ClaimOnSessionStart = false, quer true por padrão")
	}
	if !cfg.CreateOnSessionEnd() {
		t.Error("CreateOnSessionEnd = false, quer true por padrão")
	}
	if cfg.MidSession() != "follow-cwd" {
		t.Errorf("MidSession = %q, quer follow-cwd", cfg.MidSession())
	}
	if cfg.AutoScopeMode() != "per_actor" {
		t.Errorf("AutoScopeMode = %q, quer per_actor", cfg.AutoScopeMode())
	}
	wantHome := filepath.Join(canonicalDir(t, base), "home")
	if cfg.HomeDir() != wantHome {
		t.Errorf("HomeDir = %q, quer %q", cfg.HomeDir(), wantHome)
	}
	if want := filepath.Join(wantHome, ".claude"); cfg.ClaudeConfigDir() != want {
		t.Errorf("ClaudeConfigDir = %q, quer %q", cfg.ClaudeConfigDir(), want)
	}
}

// TestCamposDoArquivoTOML cobre a leitura de cada campo novo a partir do config.toml.
func TestCamposDoArquivoTOML(t *testing.T) {
	isolateEnv(t)
	tmp := t.TempDir()
	t.Chdir(tmp)
	tomlPath := filepath.Join(tmp, "config.toml")
	writeFile(t, tomlPath, `
bind = "0.0.0.0:8080"
server_url = "https://memoria.exemplo.org"
allowed_hosts = ["Memoria.Exemplo.org", "localhost"]

[auth]
bearer_token = "segredo-do-toml"

[handoff]
claim_on_session_start = false
create_on_session_end = false

[routing]
mid_session = "sticky"

[auto_scope]
mode = "single"
`)

	cfg, err := Load(Options{ConfigFile: tomlPath})
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Bind() != "0.0.0.0:8080" {
		t.Errorf("Bind = %q", cfg.Bind())
	}
	if cfg.ServerURL() != "https://memoria.exemplo.org" {
		t.Errorf("ServerURL = %q", cfg.ServerURL())
	}
	if want := []string{"memoria.exemplo.org", "localhost"}; !equalStrings(cfg.AllowedHosts(), want) {
		t.Errorf("AllowedHosts = %v, quer %v (minúsculas)", cfg.AllowedHosts(), want)
	}
	if cfg.AuthToken() != "segredo-do-toml" {
		t.Errorf("AuthToken = %q, quer o de [auth] bearer_token", cfg.AuthToken())
	}
	if cfg.ClaimOnSessionStart() {
		t.Error("ClaimOnSessionStart = true, quer false do TOML")
	}
	if cfg.CreateOnSessionEnd() {
		t.Error("CreateOnSessionEnd = true, quer false do TOML")
	}
	if cfg.MidSession() != "sticky" {
		t.Errorf("MidSession = %q, quer sticky", cfg.MidSession())
	}
	if cfg.AutoScopeMode() != "single" {
		t.Errorf("AutoScopeMode = %q, quer single", cfg.AutoScopeMode())
	}
}

// TestCamposDeAmbienteSobrepoemArquivo cobre a precedência do ambiente sobre o TOML
// para os campos novos, incluindo a lista de Host separada por vírgulas.
func TestCamposDeAmbienteSobrepoemArquivo(t *testing.T) {
	isolateEnv(t)
	tmp := t.TempDir()
	t.Chdir(tmp)
	tomlPath := filepath.Join(tmp, "config.toml")
	writeFile(t, tomlPath, `
bind = "1.1.1.1:1111"
server_url = "http://1.1.1.1:1111"
allowed_hosts = ["toml.exemplo"]

[auth]
bearer_token = "token-do-toml"

[handoff]
claim_on_session_start = true
create_on_session_end = true

[routing]
mid_session = "follow-cwd"

[auto_scope]
mode = "per_actor"
`)
	t.Setenv("AI_MEMORY_BIND", "10.0.0.5:9000")
	t.Setenv("AI_MEMORY_SERVER_URL", "http://10.0.0.5:9000")
	t.Setenv("AI_MEMORY_ALLOWED_HOSTS", " Host-A.lan ,, 10.0.0.5 ")
	t.Setenv("AI_MEMORY_AUTH_TOKEN", "token-do-ambiente")
	t.Setenv("AI_MEMORY_HANDOFF__CLAIM_ON_SESSION_START", "false")
	t.Setenv("AI_MEMORY_HANDOFF__CREATE_ON_SESSION_END", "false")
	t.Setenv("AI_MEMORY_ROUTING__MID_SESSION", "sticky")
	t.Setenv("AI_MEMORY_AUTO_SCOPE__MODE", "per_session")

	cfg, err := Load(Options{ConfigFile: tomlPath, EnvFile: filepath.Join(tmp, "ausente.env")})
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Bind() != "10.0.0.5:9000" {
		t.Errorf("Bind = %q", cfg.Bind())
	}
	if cfg.ServerURL() != "http://10.0.0.5:9000" {
		t.Errorf("ServerURL = %q", cfg.ServerURL())
	}
	if want := []string{"host-a.lan", "10.0.0.5"}; !equalStrings(cfg.AllowedHosts(), want) {
		t.Errorf("AllowedHosts = %v, quer %v", cfg.AllowedHosts(), want)
	}
	if cfg.AuthToken() != "token-do-ambiente" {
		t.Errorf("AuthToken = %q, quer o de AI_MEMORY_AUTH_TOKEN", cfg.AuthToken())
	}
	if cfg.ClaimOnSessionStart() {
		t.Error("ClaimOnSessionStart = true, quer false de AI_MEMORY_HANDOFF__CLAIM_ON_SESSION_START")
	}
	if cfg.CreateOnSessionEnd() {
		t.Error("CreateOnSessionEnd = true, quer false de AI_MEMORY_HANDOFF__CREATE_ON_SESSION_END")
	}
	if cfg.MidSession() != "sticky" {
		t.Errorf("MidSession = %q, quer sticky do ambiente", cfg.MidSession())
	}
	if cfg.AutoScopeMode() != "per_session" {
		t.Errorf("AutoScopeMode = %q, quer per_session do ambiente", cfg.AutoScopeMode())
	}
}

// TestEnvClaimOnSessionStart cobre a variável de ambiente da chave de handoff pelo nome
// documentado, sem alterar a outra chave de handoff.
func TestEnvClaimOnSessionStart(t *testing.T) {
	isolateEnv(t)
	t.Chdir(t.TempDir())
	t.Setenv("AI_MEMORY_HANDOFF__CLAIM_ON_SESSION_START", "false")

	cfg, err := Load(Options{})
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.ClaimOnSessionStart() {
		t.Error("ClaimOnSessionStart = true, quer false de AI_MEMORY_HANDOFF__CLAIM_ON_SESSION_START")
	}
	if !cfg.CreateOnSessionEnd() {
		t.Error("CreateOnSessionEnd = false, quer o padrão true")
	}
}

// TestHomeEClaudeConfigDir cobre a resolução do diretório pessoal e de CLAUDE_CONFIG_DIR,
// incluindo valores em branco tratados como não definidos.
func TestHomeEClaudeConfigDir(t *testing.T) {
	cases := []struct {
		name       string
		memHome    string            // AI_MEMORY_HOME, relativo ao tmp; vazio = não definido
		claudeDir  string            // CLAUDE_CONFIG_DIR, relativo ao tmp
		dotenv     string            // conteúdo do .env com AI_MEMORY_HOME
		wantHome   string            // relativo ao tmp; vazio = diretório pessoal do sistema
		wantClaude string            // relativo ao tmp; vazio = wantHome/.claude
		raw        map[string]string // variáveis extras do processo
	}{
		{name: "padrão: pessoal do sistema e .claude", wantHome: ""},
		{name: "AI_MEMORY_HOME vence o pessoal do sistema", memHome: "memhome", wantHome: "memhome"},
		{name: "AI_MEMORY_HOME vem do .env", dotenv: "AI_MEMORY_HOME=memenv\n", wantHome: "memenv"},
		{name: "AI_MEMORY_HOME só com espaços é ignorado", raw: map[string]string{"AI_MEMORY_HOME": "   "}, wantHome: ""},
		{name: "CLAUDE_CONFIG_DIR vence o padrão", claudeDir: "cc", wantClaude: "cc"},
		{name: "CLAUDE_CONFIG_DIR segue AI_MEMORY_HOME quando não definido", memHome: "memhome", wantHome: "memhome"},
		{name: "CLAUDE_CONFIG_DIR só com espaços é ignorado", raw: map[string]string{"CLAUDE_CONFIG_DIR": "  \t "}, wantHome: ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			base := isolateEnv(t)
			tmp := t.TempDir()
			t.Chdir(tmp)
			envPath := filepath.Join(tmp, "fonte.env")
			if c.dotenv != "" {
				writeFile(t, envPath, c.dotenv)
			}
			if c.memHome != "" {
				t.Setenv("AI_MEMORY_HOME", filepath.Join(tmp, c.memHome))
			}
			if c.claudeDir != "" {
				t.Setenv("CLAUDE_CONFIG_DIR", filepath.Join(tmp, c.claudeDir))
			}
			for k, v := range c.raw {
				t.Setenv(k, v)
			}

			cfg, err := Load(Options{EnvFile: envPath})
			if err != nil {
				t.Fatalf("Load: %v", err)
			}
			wantHome := filepath.Join(canonicalDir(t, base), "home")
			if c.wantHome != "" {
				wantHome = filepath.Join(canonicalDir(t, tmp), c.wantHome)
			}
			if cfg.HomeDir() != wantHome {
				t.Errorf("HomeDir = %q, quer %q", cfg.HomeDir(), wantHome)
			}
			wantClaude := filepath.Join(wantHome, ".claude")
			if c.wantClaude != "" {
				wantClaude = filepath.Join(canonicalDir(t, tmp), c.wantClaude)
			}
			if cfg.ClaudeConfigDir() != wantClaude {
				t.Errorf("ClaudeConfigDir = %q, quer %q", cfg.ClaudeConfigDir(), wantClaude)
			}
		})
	}
}

// TestCapturadoUmaVez confirma que o diretório pessoal e CLAUDE_CONFIG_DIR são lidos
// na carga: mudar o ambiente depois não altera a configuração já carregada.
func TestCapturadoUmaVez(t *testing.T) {
	isolateEnv(t)
	tmp := t.TempDir()
	t.Chdir(tmp)
	t.Setenv("CLAUDE_CONFIG_DIR", filepath.Join(tmp, "primeiro"))
	t.Setenv("AI_MEMORY_HOME", filepath.Join(tmp, "home-primeiro"))

	cfg, err := Load(Options{})
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	t.Setenv("CLAUDE_CONFIG_DIR", filepath.Join(tmp, "segundo"))
	t.Setenv("AI_MEMORY_HOME", filepath.Join(tmp, "home-segundo"))

	if want := filepath.Join(canonicalDir(t, tmp), "primeiro"); cfg.ClaudeConfigDir() != want {
		t.Errorf("ClaudeConfigDir = %q, quer %q capturado na carga", cfg.ClaudeConfigDir(), want)
	}
	if want := filepath.Join(canonicalDir(t, tmp), "home-primeiro"); cfg.HomeDir() != want {
		t.Errorf("HomeDir = %q, quer %q capturado na carga", cfg.HomeDir(), want)
	}
}

// TestValidacoesNovosCampos confirma que cada valor inválido de ambiente falha na carga.
func TestValidacoesNovosCampos(t *testing.T) {
	cases := []struct {
		key   string
		value string
	}{
		{"AI_MEMORY_BIND", "nao-e-endereco"},
		{"AI_MEMORY_BIND", "127.0.0.1:0"},
		{"AI_MEMORY_BIND", "127.0.0.1:70000"},
		{"AI_MEMORY_BIND", "127.0.0.1:abc"},
		{"AI_MEMORY_SERVER_URL", "ftp://exemplo.org"},
		{"AI_MEMORY_SERVER_URL", "exemplo.org:8080"},
		{"AI_MEMORY_SERVER_URL", "http://"},
		{"AI_MEMORY_SERVER_URL", "://sem-esquema"},
		{"AI_MEMORY_ALLOWED_HOSTS", "*"},
		{"AI_MEMORY_ALLOWED_HOSTS", " , , "},
		{"AI_MEMORY_ALLOWED_HOSTS", "exemplo.org/caminho"},
		{"AI_MEMORY_ALLOWED_HOSTS", "com espaco.org"},
		{"AI_MEMORY_HANDOFF__CLAIM_ON_SESSION_START", "talvez"},
		{"AI_MEMORY_HANDOFF__CREATE_ON_SESSION_END", "2"},
		{"AI_MEMORY_ROUTING__MID_SESSION", "Sticky"},
		{"AI_MEMORY_ROUTING__MID_SESSION", "sempre"},
		{"AI_MEMORY_ROUTING__MID_SESSION", ""},
		{"AI_MEMORY_AUTO_SCOPE__MODE", "PerSession"},
		{"AI_MEMORY_AUTO_SCOPE__MODE", "shared"},
		{"AI_MEMORY_AUTO_SCOPE__MODE", ""},
	}
	for _, c := range cases {
		t.Run(c.key+"="+c.value, func(t *testing.T) {
			isolateEnv(t)
			t.Chdir(t.TempDir())
			t.Setenv(c.key, c.value)
			if _, err := Load(Options{}); err == nil {
				t.Fatalf("esperava erro para %s=%q", c.key, c.value)
			}
		})
	}
}

// TestValidacoesTOMLNovosCampos confirma que tipos errados no arquivo falham na carga.
func TestValidacoesTOMLNovosCampos(t *testing.T) {
	cases := map[string]string{
		"allowed_hosts como número":          "allowed_hosts = 5\n",
		"allowed_hosts com item não texto":   "allowed_hosts = [\"ok.exemplo\", 3]\n",
		"claim_on_session_start como número": "[handoff]\nclaim_on_session_start = 1\n",
		"create_on_session_end texto ruim":   "[handoff]\ncreate_on_session_end = \"nao\"\n",
		"bind inválido":                      "bind = \"sem-porta\"\n",
		"mid_session inválido":               "[routing]\nmid_session = \"tudo\"\n",
		"auto_scope inválido":                "[auto_scope]\nmode = \"global\"\n",
	}
	for name, content := range cases {
		t.Run(name, func(t *testing.T) {
			isolateEnv(t)
			tmp := t.TempDir()
			t.Chdir(tmp)
			tomlPath := filepath.Join(tmp, "config.toml")
			writeFile(t, tomlPath, content)
			if _, err := Load(Options{ConfigFile: tomlPath}); err == nil {
				t.Fatalf("esperava erro para %q", content)
			}
		})
	}
}

// TestSanitizeDefaults confirma que sem [sanitize] as duas listas são vazias.
func TestSanitizeDefaults(t *testing.T) {
	isolateEnv(t)
	t.Chdir(t.TempDir())

	cfg, err := Load(Options{})
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(cfg.SanitizeAllowlist()) != 0 {
		t.Errorf("SanitizeAllowlist = %v, quer vazia", cfg.SanitizeAllowlist())
	}
	if len(cfg.SanitizeExtraPatterns()) != 0 {
		t.Errorf("SanitizeExtraPatterns = %v, quer vazia", cfg.SanitizeExtraPatterns())
	}
}

// TestSanitizeTOML lê [sanitize] do arquivo, inclusive expressão com vírgula (que a lista de
// texto de ambiente não suportaria), e confirma que os getters devolvem cópias.
func TestSanitizeTOML(t *testing.T) {
	isolateEnv(t)
	tmp := t.TempDir()
	t.Chdir(tmp)
	tomlPath := filepath.Join(tmp, "config.toml")
	writeFile(t, tomlPath, `
[sanitize]
allowlist = ["PROJETO_TOKEN_LEGADO", "corp-1234"]
extra_patterns = ["corp-[0-9]{4}", "(?i)acme-[a-z]{2,6}"]
`)

	cfg, err := Load(Options{ConfigFile: tomlPath})
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if want := []string{"PROJETO_TOKEN_LEGADO", "corp-1234"}; !equalStrings(cfg.SanitizeAllowlist(), want) {
		t.Errorf("SanitizeAllowlist = %v, quer %v", cfg.SanitizeAllowlist(), want)
	}
	if want := []string{"corp-[0-9]{4}", "(?i)acme-[a-z]{2,6}"}; !equalStrings(cfg.SanitizeExtraPatterns(), want) {
		t.Errorf("SanitizeExtraPatterns = %v, quer %v", cfg.SanitizeExtraPatterns(), want)
	}

	cfg.SanitizeAllowlist()[0] = "mutado"
	if cfg.SanitizeAllowlist()[0] != "PROJETO_TOKEN_LEGADO" {
		t.Error("SanitizeAllowlist devolveu o slice interno, não uma cópia")
	}
}

// TestSanitizeExtraInvalidaImpedeCarga confirma que expressão RE2 inválida falha a carga, o que
// impede o início do serviço (docs/domain-vocabulary.md §7.3).
func TestSanitizeExtraInvalidaImpedeCarga(t *testing.T) {
	cases := map[string]string{
		"colchete aberto":         "[sanitize]\nextra_patterns = [\"[aberto\"]\n",
		"lookahead não é RE2":     "[sanitize]\nextra_patterns = [\"(?=x)y\"]\n",
		"extra vazio":             "[sanitize]\nextra_patterns = [\"\"]\n",
		"allowlist com vazio":     "[sanitize]\nallowlist = [\"ok\", \"\"]\n",
		"allowlist com não texto": "[sanitize]\nallowlist = [\"ok\", 3]\n",
		"extra com tipo inválido": "[sanitize]\nextra_patterns = 5\n",
	}
	for name, content := range cases {
		t.Run(name, func(t *testing.T) {
			isolateEnv(t)
			tmp := t.TempDir()
			t.Chdir(tmp)
			tomlPath := filepath.Join(tmp, "config.toml")
			writeFile(t, tomlPath, content)
			if _, err := Load(Options{ConfigFile: tomlPath}); err == nil {
				t.Fatalf("esperava erro para %q", content)
			}
		})
	}
}

// TestSanitizeAmbienteTextoFalha confirma que a lista não se define por variável de ambiente
// (texto solto): o erro explica que é preciso lista TOML.
func TestSanitizeAmbienteTextoFalha(t *testing.T) {
	for _, key := range []string{"AI_MEMORY_SANITIZE__ALLOWLIST", "AI_MEMORY_SANITIZE__EXTRA_PATTERNS"} {
		t.Run(key, func(t *testing.T) {
			isolateEnv(t)
			t.Chdir(t.TempDir())
			t.Setenv(key, "algo")
			if _, err := Load(Options{}); err == nil {
				t.Fatalf("esperava erro para %s", key)
			}
		})
	}
}

// equalStrings compara duas listas na mesma ordem.
func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
