// Raiz da CLI ai-memory. Este arquivo é o ponto de composição do processo. A
// configuração é carregada uma única vez por execução (INV-01): Compose chama
// config.Load e delega a ComposeWithConfig; quem já tem a configuração carregada
// (como reindex) chama ComposeWithConfig diretamente, sem nova carga. Ambos montam
// logger, storage, escritor e a wiki. Nenhum outro pacote lê o ambiente; a
// configuração entra só por config.
package cli

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sync"

	"github.com/spf13/cobra"

	"github.com/phm-aguiar/ai-memory-go/internal/config"
	"github.com/phm-aguiar/ai-memory-go/internal/storage"
	"github.com/phm-aguiar/ai-memory-go/internal/wiki"
)

const (
	flagDataDir = "data-dir"
	flagConfig  = "config"
	flagEnvFile = "env-file"
	wikiDirName = "wiki"
)

// NewRootCommand devolve o comando raiz ai-memory com as flags globais --config
// e --env-file. A raiz não tem ação própria: sem argumentos, mostra a ajuda. Ela
// é marcada como executável só para que o cobra inclua as flags na ajuda (o
// template omite a seção de uso de comandos não executáveis sem subcomandos).
// Subcomandos são adicionados depois e chamam Compose quando precisam dos componentes.
func NewRootCommand() *cobra.Command {
	root := &cobra.Command{
		Use:          "ai-memory",
		Short:        "Memória local para agentes de IA",
		Args:         cobra.NoArgs,
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return cmd.Help()
		},
	}
	root.PersistentFlags().String(flagDataDir, "", "diretório de dados (padrão: diretório de dados local do usuário, pasta ai-memory)")
	root.PersistentFlags().String(flagConfig, "", "caminho do arquivo de configuração TOML (padrão: config.toml no diretório de dados)")
	root.PersistentFlags().String(flagEnvFile, "", "caminho do arquivo .env (padrão: AI_MEMORY_ENV_FILE ou ./.env)")
	root.AddCommand(NewReindexCommand())
	root.AddCommand(NewBackupCommand())
	return root
}

// ConfigOptions lê as flags globais de cmd e devolve as opções de config.Load.
// Chame-a dentro de um subcomando, depois do parse, e passe o resultado a Compose.
func ConfigOptions(cmd *cobra.Command) (config.Options, error) {
	dataDir, err := cmd.Flags().GetString(flagDataDir)
	if err != nil {
		return config.Options{}, fmt.Errorf("cli: flag --%s: %w", flagDataDir, err)
	}
	cfgFile, err := cmd.Flags().GetString(flagConfig)
	if err != nil {
		return config.Options{}, fmt.Errorf("cli: flag --%s: %w", flagConfig, err)
	}
	envFile, err := cmd.Flags().GetString(flagEnvFile)
	if err != nil {
		return config.Options{}, fmt.Errorf("cli: flag --%s: %w", flagEnvFile, err)
	}
	return config.Options{DataDir: dataDir, ConfigFile: cfgFile, EnvFile: envFile}, nil
}

// App agrupa os componentes montados por Compose. O chamador deve chamar Close.
type App struct {
	Cfg    config.Config
	Logger *slog.Logger
	DB     *sql.DB
	Writer storage.Writer
	Store  *wiki.Store

	closeOnce sync.Once
	closeErr  error
}

// loadConfig é o único sítio de produção que chama config.Load (INV-01). Compose e
// runReindex o usam: cada execução de comando carrega a configuração uma única vez.
func loadConfig(opts config.Options) (config.Config, error) {
	cfg, err := config.Load(opts)
	if err != nil {
		return config.Config{}, fmt.Errorf("cli: carregar configuração: %w", err)
	}
	return cfg, nil
}

// Compose carrega a configuração (via loadConfig) e delega a montagem a
// ComposeWithConfig.
func Compose(ctx context.Context, opts config.Options) (*App, error) {
	cfg, err := loadConfig(opts)
	if err != nil {
		return nil, err
	}
	return ComposeWithConfig(ctx, cfg)
}

// ComposeWithConfig monta os componentes a partir de uma configuração já carregada,
// sem chamar config.Load, na ordem: logger (com registro de inicialização), storage,
// escritor, repositório git da wiki em <data_dir>/wiki e wiki.Store. Em falha, libera
// o que já foi aberto e devolve o erro com %w.
func ComposeWithConfig(ctx context.Context, cfg config.Config) (*App, error) {
	logger, err := NewLogger(cfg, os.Stderr)
	if err != nil {
		return nil, fmt.Errorf("cli: criar logger: %w", err)
	}
	LogStartup(logger, cfg)

	db, err := storage.Open(ctx, cfg.DataDir())
	if err != nil {
		return nil, fmt.Errorf("cli: abrir storage: %w", err)
	}

	writer, err := storage.NewWriter(ctx, db)
	if err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("cli: criar escritor: %w", err)
	}

	wikiRoot := filepath.Join(cfg.DataDir(), wikiDirName)
	repo, err := wiki.Open(wikiRoot)
	if err != nil {
		_ = writer.Close()
		_ = db.Close()
		return nil, fmt.Errorf("cli: abrir wiki: %w", err)
	}

	store := &wiki.Store{
		Root:   wikiRoot,
		Writer: writer,
		Repo:   repo,
		Locks:  wiki.NewPageLocks(),
	}

	return &App{
		Cfg:    cfg,
		Logger: logger,
		DB:     db,
		Writer: writer,
		Store:  store,
	}, nil
}

// Close fecha o escritor e depois o banco. É idempotente: chamadas seguintes
// devolvem o mesmo resultado da primeira. Receptor nil não faz nada.
func (a *App) Close() error {
	if a == nil {
		return nil
	}
	a.closeOnce.Do(func() {
		var errs []error
		if a.Writer != nil {
			if err := a.Writer.Close(); err != nil {
				errs = append(errs, fmt.Errorf("cli: fechar escritor: %w", err))
			}
		}
		if a.DB != nil {
			if err := a.DB.Close(); err != nil {
				errs = append(errs, fmt.Errorf("cli: fechar storage: %w", err))
			}
		}
		a.closeErr = errors.Join(errs...)
	})
	return a.closeErr
}
