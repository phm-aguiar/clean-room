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
}

// isolateEnv remove as variáveis de config do processo. t.Setenv restaura o
// valor original ao fim do teste.
func isolateEnv(t *testing.T) {
	t.Helper()
	for _, k := range configEnvKeys {
		t.Setenv(k, "")
		_ = os.Unsetenv(k)
	}
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
	isolateEnv(t)
	tmp := t.TempDir()
	t.Chdir(tmp)

	cfg, err := Load(Options{})
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if want := filepath.Join(canonicalDir(t, tmp), "data"); cfg.DataDir() != want {
		t.Errorf("DataDir = %q, quer %q", cfg.DataDir(), want)
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
			wantDir:   "data",
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

func TestLogLevelInvalidoFalha(t *testing.T) {
	isolateEnv(t)
	t.Chdir(t.TempDir())
	t.Setenv("AI_MEMORY_LOG__LEVEL", "trace")

	if _, err := Load(Options{}); err == nil {
		t.Fatal("esperava erro para log.level inválido")
	}
}
