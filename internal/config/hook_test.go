package config

import (
	"path/filepath"
	"testing"
	"time"
)

// hookEnvVars são as variáveis de tempo lidas por LoadHook. Limpá-las evita que o
// ambiente da máquina mude o resultado. isolateEnv já limpa configEnvKeys.
var hookEnvVars = []string{
	"AI_MEMORY_HOOK_DRAIN_TIMEOUT_MINUTES",
	"AI_MEMORY_HOOK_HANDOFF_TIMEOUT_MINUTES",
	"AI_MEMORY_HOOK_START_BUDGET_MINUTES",
	"AI_MEMORY_HOOK_BACKGROUND_DRAIN_BUDGET_MINUTES",
}

func isolateHookEnv(t *testing.T) string {
	t.Helper()
	base := isolateEnv(t)
	for _, k := range hookEnvVars {
		t.Setenv(k, "")
	}
	return base
}

func TestLoadHookDefaults(t *testing.T) {
	base := isolateHookEnv(t)

	cfg, err := LoadHook(HookOptions{})
	if err != nil {
		t.Fatal(err)
	}
	wantDir := filepath.Join(canonicalDir(t, base), "ai-memory")
	if cfg.DataDir() != wantDir {
		t.Errorf("DataDir = %q, quer %q (diretório local do usuário + ai-memory)", cfg.DataDir(), wantDir)
	}
	if cfg.ServerURL() != defaultServerURL {
		t.Errorf("ServerURL = %q, quer %q", cfg.ServerURL(), defaultServerURL)
	}
	if want := filepath.Join(wantDir, "hook-token"); cfg.TokenPath() != want {
		t.Errorf("TokenPath = %q, quer %q", cfg.TokenPath(), want)
	}
	if cfg.DrainTimeout() != 3*time.Second {
		t.Errorf("DrainTimeout = %v, quer 3s", cfg.DrainTimeout())
	}
	if cfg.HandoffTimeout() != 3*time.Second {
		t.Errorf("HandoffTimeout = %v, quer 3s", cfg.HandoffTimeout())
	}
	if cfg.StartBudget() != 3*time.Second {
		t.Errorf("StartBudget = %v, quer 3s", cfg.StartBudget())
	}
	if cfg.BackgroundDrainBudget() != 5*time.Minute {
		t.Errorf("BackgroundDrainBudget = %v, quer 5m", cfg.BackgroundDrainBudget())
	}
}

func TestLoadHookFlags(t *testing.T) {
	isolateHookEnv(t)
	tmp := t.TempDir()

	cfg, err := LoadHook(HookOptions{DataDir: tmp, ServerURL: "https://memoria.exemplo:8443"})
	if err != nil {
		t.Fatal(err)
	}
	if want := canonicalDir(t, tmp); cfg.DataDir() != want {
		t.Errorf("DataDir = %q, quer %q (flag --data-dir)", cfg.DataDir(), want)
	}
	if cfg.ServerURL() != "https://memoria.exemplo:8443" {
		t.Errorf("ServerURL = %q, quer a da flag", cfg.ServerURL())
	}
	if want := filepath.Join(canonicalDir(t, tmp), "hook-token"); cfg.TokenPath() != want {
		t.Errorf("TokenPath = %q, quer %q", cfg.TokenPath(), want)
	}
}

func TestLoadHookServerURLInvalidaFalha(t *testing.T) {
	isolateHookEnv(t)
	for _, raw := range []string{"ftp://x", "http://", "sem-esquema"} {
		if _, err := LoadHook(HookOptions{ServerURL: raw}); err == nil {
			t.Errorf("ServerURL %q aceita, quer erro", raw)
		}
	}
}

// Decisão registrada: docs/cli.md §2.1 e §5.3 dizem que as variáveis de tempo são as
// únicas leituras de ambiente no caminho do hook. AI_MEMORY_DATA_DIR não é lida aqui.
func TestLoadHookNaoLeAmbienteGeral(t *testing.T) {
	base := isolateHookEnv(t)
	t.Setenv("AI_MEMORY_DATA_DIR", filepath.Join(base, "outro"))
	t.Setenv("AI_MEMORY_SERVER_URL", "http://10.0.0.9:1")

	cfg, err := LoadHook(HookOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if want := filepath.Join(canonicalDir(t, base), "ai-memory"); cfg.DataDir() != want {
		t.Errorf("DataDir = %q, quer o padrão %q (AI_MEMORY_DATA_DIR não é lida no hook)", cfg.DataDir(), want)
	}
	if cfg.ServerURL() != defaultServerURL {
		t.Errorf("ServerURL = %q, quer o padrão (AI_MEMORY_SERVER_URL não é lida no hook)", cfg.ServerURL())
	}
}

func TestLoadHookTemposEmMinutos(t *testing.T) {
	getters := map[string]func(HookConfig) time.Duration{
		"AI_MEMORY_HOOK_DRAIN_TIMEOUT_MINUTES":           HookConfig.DrainTimeout,
		"AI_MEMORY_HOOK_HANDOFF_TIMEOUT_MINUTES":         HookConfig.HandoffTimeout,
		"AI_MEMORY_HOOK_START_BUDGET_MINUTES":            HookConfig.StartBudget,
		"AI_MEMORY_HOOK_BACKGROUND_DRAIN_BUDGET_MINUTES": HookConfig.BackgroundDrainBudget,
	}
	defaults := map[string]time.Duration{
		"AI_MEMORY_HOOK_DRAIN_TIMEOUT_MINUTES":           3 * time.Second,
		"AI_MEMORY_HOOK_HANDOFF_TIMEOUT_MINUTES":         3 * time.Second,
		"AI_MEMORY_HOOK_START_BUDGET_MINUTES":            3 * time.Second,
		"AI_MEMORY_HOOK_BACKGROUND_DRAIN_BUDGET_MINUTES": 5 * time.Minute,
	}
	cases := []struct {
		raw  string
		want func(def time.Duration) time.Duration
	}{
		{"1", func(time.Duration) time.Duration { return time.Minute }},
		{"5", func(time.Duration) time.Duration { return 5 * time.Minute }},
		{"60", func(time.Duration) time.Duration { return 60 * time.Minute }},
		{"61", func(time.Duration) time.Duration { return 60 * time.Minute }},
		{"100000", func(time.Duration) time.Duration { return 60 * time.Minute }},
		{"99999999999999999999999", func(time.Duration) time.Duration { return 60 * time.Minute }},
		{"0", func(def time.Duration) time.Duration { return def }},
		{"-4", func(def time.Duration) time.Duration { return def }},
		{"abc", func(def time.Duration) time.Duration { return def }},
		{"1.5", func(def time.Duration) time.Duration { return def }},
		{"", func(def time.Duration) time.Duration { return def }},
	}

	for name, get := range getters {
		for _, c := range cases {
			t.Run(name+"="+c.raw, func(t *testing.T) {
				isolateHookEnv(t)
				t.Setenv(name, c.raw)
				cfg, err := LoadHook(HookOptions{})
				if err != nil {
					t.Fatal(err)
				}
				want := c.want(defaults[name])
				if got := get(cfg); got != want {
					t.Errorf("%s=%q: got %v, quer %v", name, c.raw, got, want)
				}
			})
		}
	}
}

func TestLoadHookVariaveisSaoIndependentes(t *testing.T) {
	isolateHookEnv(t)
	t.Setenv("AI_MEMORY_HOOK_DRAIN_TIMEOUT_MINUTES", "2")

	cfg, err := LoadHook(HookOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if cfg.DrainTimeout() != 2*time.Minute {
		t.Errorf("DrainTimeout = %v, quer 2m", cfg.DrainTimeout())
	}
	if cfg.HandoffTimeout() != 3*time.Second || cfg.StartBudget() != 3*time.Second || cfg.BackgroundDrainBudget() != 5*time.Minute {
		t.Errorf("outros tempos mudaram com só DRAIN definido: handoff=%v start=%v bg=%v",
			cfg.HandoffTimeout(), cfg.StartBudget(), cfg.BackgroundDrainBudget())
	}
}
