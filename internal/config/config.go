// Package config carrega a configuração do processo uma única vez (INV-01).
//
// Precedência, da menor para a maior: padrões embutidos, arquivo TOML, arquivo
// .env local e variáveis do processo AI_MEMORY_*. O separador "__" aninha chaves:
// AI_MEMORY_LOG__LEVEL define log.level.
//
// Este é o único pacote que lê ambiente, e só pelo provider env do koanf.
// Não usa os.Getenv, os.LookupEnv nem os.Environ.
package config

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
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
)

// Config é a configuração resolvida do processo. Os campos são não exportados e
// não há setters: após Load, o valor só é lido pelos getters.
type Config struct {
	dataDir        string
	logLevel       string
	watchDebounce  time.Duration
	reconcileEvery time.Duration
}

// DataDir devolve o diretório de dados absoluto e canônico (INV-11).
func (c Config) DataDir() string { return c.dataDir }

// LogLevel devolve o nível de log: debug, info, warn ou error.
func (c Config) LogLevel() string { return c.logLevel }

// WatchDebounce devolve o debounce do watcher da wiki.
func (c Config) WatchDebounce() time.Duration { return c.watchDebounce }

// ReconcileInterval devolve o intervalo da reconciliação da wiki.
func (c Config) ReconcileInterval() time.Duration { return c.reconcileEvery }

// Options define os caminhos das fontes de arquivo. Zero value: sem TOML, e o
// .env vem de AI_MEMORY_ENV_FILE ou, se ausente, de <diretório de trabalho>/.env.
type Options struct {
	// ConfigFile é o caminho do TOML. Vazio, ou arquivo inexistente, significa sem essa camada.
	ConfigFile string
	// EnvFile é o caminho do .env. Vazio usa AI_MEMORY_ENV_FILE ou <diretório de trabalho>/.env.
	EnvFile string
}

// Load resolve a configuração na ordem padrões < TOML < .env < AI_MEMORY_*.
// Arquivos ausentes são opcionais e não são erro.
func Load(opts Options) (Config, error) {
	procEnv := koanf.New(".")
	if err := procEnv.Load(env.Provider(envPrefix, ".", envKey), nil); err != nil {
		return Config{}, fmt.Errorf("config: lendo variáveis %s*: %w", envPrefix, err)
	}

	envFile, err := resolveEnvFile(opts.EnvFile, procEnv.String(envFileKey))
	if err != nil {
		return Config{}, err
	}

	k := koanf.New(".")
	if err := k.Load(confmap.Provider(defaults(), "."), nil); err != nil {
		return Config{}, fmt.Errorf("config: padrões: %w", err)
	}
	if err := loadOptional(k, opts.ConfigFile, toml.Parser()); err != nil {
		return Config{}, fmt.Errorf("config: TOML %q: %w", opts.ConfigFile, err)
	}
	if err := loadOptional(k, envFile, dotenv.ParserEnv(envPrefix, ".", envKey)); err != nil {
		return Config{}, fmt.Errorf("config: .env %q: %w", envFile, err)
	}
	if err := k.Merge(procEnv); err != nil {
		return Config{}, fmt.Errorf("config: variáveis %s*: %w", envPrefix, err)
	}

	return build(k)
}

func defaults() map[string]interface{} {
	return map[string]interface{}{
		"data_dir": "data",
		"log": map[string]interface{}{
			"level": "info",
		},
		"wiki": map[string]interface{}{
			"watch_debounce_ms":    300,
			"reconcile_interval_s": 30,
		},
	}
}

// envKey converte AI_MEMORY_LOG__LEVEL em log.level.
func envKey(s string) string {
	key := strings.TrimPrefix(s, envPrefix)
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

func build(k *koanf.Koanf) (Config, error) {
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

	return Config{
		dataDir:        dataDir,
		logLevel:       level,
		watchDebounce:  time.Duration(debounceMS) * time.Millisecond,
		reconcileEvery: time.Duration(reconcileS) * time.Second,
	}, nil
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
