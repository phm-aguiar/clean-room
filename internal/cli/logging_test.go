package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/phm-aguiar/ai-memory-go/internal/config"
)

// loadTestConfig carrega a configuração real via config.Load, apontando o data_dir
// para dataDir e isolando o ambiente de máquina (.env e variáveis de config).
func loadTestConfig(t *testing.T, dataDir, level string) config.Config {
	t.Helper()
	for _, k := range []string{
		"AI_MEMORY_ENV_FILE",
		"AI_MEMORY_LOG_LEVEL",
		"AI_MEMORY_WIKI__WATCH_DEBOUNCE_MS",
		"AI_MEMORY_WIKI__RECONCILE_INTERVAL_S",
	} {
		t.Setenv(k, "")
		_ = os.Unsetenv(k)
	}
	t.Setenv("AI_MEMORY_DATA_DIR", dataDir)
	t.Setenv("AI_MEMORY_LOG__LEVEL", level)
	cfg, err := config.Load(config.Options{EnvFile: filepath.Join(t.TempDir(), "ausente.env")})
	if err != nil {
		t.Fatalf("config.Load: %v", err)
	}
	return cfg
}

// parseLines decodifica cada linha JSON do log em um mapa.
func parseLines(t *testing.T, out string) []map[string]any {
	t.Helper()
	var recs []map[string]any
	for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
		if line == "" {
			continue
		}
		var m map[string]any
		if err := json.Unmarshal([]byte(line), &m); err != nil {
			t.Fatalf("linha de log não é JSON: %v: %q", err, line)
		}
		recs = append(recs, m)
	}
	return recs
}

func TestNewLoggerNivelInfoSuprimeDebugEmiteInfo(t *testing.T) {
	cfg := loadTestConfig(t, t.TempDir(), "info")
	var buf bytes.Buffer
	logger, err := NewLogger(cfg, &buf)
	if err != nil {
		t.Fatalf("NewLogger: %v", err)
	}

	logger.Debug("oculto")
	logger.Info("visivel")

	recs := parseLines(t, buf.String())
	if len(recs) != 1 {
		t.Fatalf("esperava 1 registro, veio %d: %q", len(recs), buf.String())
	}
	if recs[0]["msg"] != "visivel" || recs[0]["level"] != "INFO" {
		t.Fatalf("registro inesperado: %v", recs[0])
	}
}

func TestNewLoggerNivelInvalidoDevolveErro(t *testing.T) {
	var buf bytes.Buffer
	logger, err := NewLogger(config.Config{}, &buf)
	if err == nil {
		t.Fatal("esperava erro para nível vazio/inválido")
	}
	if !errors.Is(err, ErrLogLevel) {
		t.Fatalf("erro não envolve ErrLogLevel via %%w: %v", err)
	}
	if logger != nil {
		t.Fatal("logger deveria ser nil quando há erro")
	}
}

func TestNewLoggerDestinoNilDevolveErro(t *testing.T) {
	cfg := loadTestConfig(t, t.TempDir(), "info")
	if _, err := NewLogger(cfg, nil); err == nil {
		t.Fatal("esperava erro para destino nil")
	}
}

func TestLogStartupRegistraDataDirCanonico(t *testing.T) {
	dir := t.TempDir()
	cfg := loadTestConfig(t, dir, "info")
	if !filepath.IsAbs(cfg.DataDir()) {
		t.Fatalf("DataDir não é absoluto: %q", cfg.DataDir())
	}

	var buf bytes.Buffer
	logger, err := NewLogger(cfg, &buf)
	if err != nil {
		t.Fatalf("NewLogger: %v", err)
	}
	LogStartup(logger, cfg)

	recs := parseLines(t, buf.String())
	if len(recs) != 1 {
		t.Fatalf("esperava 1 registro de inicialização, veio %d", len(recs))
	}
	if got := recs[0]["data_dir"]; got != cfg.DataDir() {
		t.Fatalf("data_dir = %q, esperava %q", got, cfg.DataDir())
	}
	if got := recs[0]["log_level"]; got != "info" {
		t.Fatalf("log_level = %q, esperava info", got)
	}
}

func TestLogStartupNaoVazaValoresDeAmbiente(t *testing.T) {
	const segredo = "segredo-de-teste-9f3a7c"
	t.Setenv("AI_MEMORY_X_SECRET", segredo)
	cfg := loadTestConfig(t, t.TempDir(), "debug")

	var buf bytes.Buffer
	logger, err := NewLogger(cfg, &buf)
	if err != nil {
		t.Fatalf("NewLogger: %v", err)
	}
	LogStartup(logger, cfg)
	logger.Debug("uso normal")

	if strings.Contains(buf.String(), segredo) {
		t.Fatalf("log contém valor de variável de ambiente: %q", buf.String())
	}
}

func TestFiltroRealimentacaoDescartaRegistroMarcado(t *testing.T) {
	cfg := loadTestConfig(t, t.TempDir(), "debug")
	var buf bytes.Buffer
	logger, err := NewLogger(cfg, &buf)
	if err != nil {
		t.Fatalf("NewLogger: %v", err)
	}

	logger.Info("externo")
	logger.Info("interno-no-registro", originKey, originSelf)
	logger.With(originKey, originSelf).Info("interno-com-with")
	logger.With(originKey, originSelf).With("a", 1).Info("interno-with-encadeado")
	logger.With("b", 2).With(originKey, originSelf).Info("interno-with-tardio")

	recs := parseLines(t, buf.String())
	if len(recs) != 1 {
		t.Fatalf("esperava só o registro externo, veio %d: %q", len(recs), buf.String())
	}
	if recs[0]["msg"] != "externo" {
		t.Fatalf("registro indevido passou pelo filtro: %v", recs[0])
	}
}

func TestFiltroRealimentacaoLoggerMarcadoNaoHabilita(t *testing.T) {
	cfg := loadTestConfig(t, t.TempDir(), "debug")
	logger, err := NewLogger(cfg, &bytes.Buffer{})
	if err != nil {
		t.Fatalf("NewLogger: %v", err)
	}
	marcado := logger.With(originKey, originSelf).Handler()
	if marcado.Enabled(context.Background(), slog.LevelError) {
		t.Fatal("handler marcado com a origem interna não deveria estar habilitado")
	}
	if !logger.Handler().Enabled(context.Background(), slog.LevelError) {
		t.Fatal("handler sem marca deveria estar habilitado para error")
	}
}
