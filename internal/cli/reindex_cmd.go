package cli

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/phm-aguiar/ai-memory-go/internal/storage"
	"github.com/phm-aguiar/ai-memory-go/internal/wiki"
)

// ErrDatabaseNotClean indica que o banco já tem linhas nas tabelas que o reindex reconstrói.
var ErrDatabaseNotClean = errors.New("banco não está limpo")

// NewReindexCommand devolve o subcomando "reindex", que reconstrói o índice (páginas,
// links e busca textual) a partir do markdown de <data_dir>/wiki. Não tem argumentos.
// É uma operação de disco direto (INV-09): antes de agir, checa que nenhum outro processo
// ai-memory está vivo sobre o data_dir. O comando é adicionado à raiz por quem monta a
// CLI: root.AddCommand(NewReindexCommand()).
func NewReindexCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "reindex",
		Short: "Reconstrói o índice a partir do markdown da wiki",
		Args:  cobra.NoArgs,
		RunE:  runReindex,
	}
}

// runReindex carrega a configuração uma única vez (INV-01), recusa se há processo vivo
// sobre o data_dir e só então monta o App com ComposeWithConfig (que abre o banco) e
// executa wiki.Reindex. Erros por arquivo ou escopo vão para o relatório e não fazem o
// comando falhar; erro de contexto ou de raiz da wiki faz. Erros voltam com %w; o App
// é fechado ao final.
func runReindex(cmd *cobra.Command, _ []string) (err error) {
	opts, err := ConfigOptions(cmd)
	if err != nil {
		return err
	}

	ctx := cmd.Context()
	if ctx == nil {
		ctx = context.Background()
	}

	// A guarda vem antes de abrir o banco: o banco só é aberto por ComposeWithConfig,
	// que não pode fazê-lo sobre um servidor vivo. A mesma cfg segue para a montagem.
	cfg, err := loadConfig(opts)
	if err != nil {
		return err
	}
	if err := CheckNoLiveProcess(cfg.DataDir()); err != nil {
		return fmt.Errorf("cli: reindex recusado: %w", err)
	}

	app, err := ComposeWithConfig(ctx, cfg)
	if err != nil {
		return err
	}
	defer func() {
		if cerr := app.Close(); cerr != nil && err == nil {
			err = cerr
		}
	}()

	// Clean-target check (docs/lifecycle-ops.md): o reindex reconstrói o banco do zero, então
	// recusa antes de escrever se alguma tabela de reconstrução já tem linhas.
	if err := checkCleanTarget(ctx, app.Writer); err != nil {
		return fmt.Errorf("cli: reindex recusado: %w", err)
	}

	rep, err := wiki.Reindex(ctx, app.Store, filepath.Join(app.Cfg.DataDir(), wikiDirName))
	if err != nil {
		return fmt.Errorf("cli: reindex: %w", err)
	}
	if err := writeReindexReport(cmd.OutOrStdout(), rep); err != nil {
		return fmt.Errorf("cli: reindex: escrever relatório: %w", err)
	}
	return nil
}

// checkCleanTarget conta as linhas das tabelas de reconstrução pelo escritor e recusa se
// alguma tem linhas. A mensagem nomeia cada tabela não zero com a sua contagem.
func checkCleanTarget(ctx context.Context, w storage.Writer) error {
	var counts []storage.RowCount
	err := w.Do(ctx, func(tx *sql.Tx) error {
		var e error
		counts, e = storage.CountRows(tx)
		return e
	})
	if err != nil {
		return fmt.Errorf("contar linhas do banco: %w", err)
	}
	nonZero := storage.NonZeroRowCounts(counts)
	if len(nonZero) == 0 {
		return nil
	}
	parts := make([]string, 0, len(nonZero))
	for _, c := range nonZero {
		parts = append(parts, fmt.Sprintf("%s: %d", c.Table, c.Rows))
	}
	return fmt.Errorf("%w: %s; mova ou remova db/memory.sqlite (e os arquivos -wal e -shm) antes de reconstruir",
		ErrDatabaseNotClean, strings.Join(parts, ", "))
}

// writeReindexReport imprime o resumo do reindex e, em seguida, cada erro e aviso.
// Monta o texto em memória e escreve uma vez, para que a falha de escrita seja devolvida.
func writeReindexReport(w io.Writer, rep wiki.ReindexReport) error {
	var b strings.Builder
	fmt.Fprintf(&b, "reindex concluído\n")
	fmt.Fprintf(&b, "escopos: %d\n", rep.Scopes)
	fmt.Fprintf(&b, "páginas: %d\n", rep.Pages)
	fmt.Fprintf(&b, "inalteradas: %d\n", rep.Unchanged)
	fmt.Fprintf(&b, "erros: %d\n", len(rep.Errors))
	fmt.Fprintf(&b, "avisos: %d\n", len(rep.Avisos))
	for _, e := range rep.Errors {
		fmt.Fprintf(&b, "erro: %s\n", e)
	}
	for _, a := range rep.Avisos {
		fmt.Fprintf(&b, "aviso: %s\n", a)
	}
	_, err := io.WriteString(w, b.String())
	return err
}
