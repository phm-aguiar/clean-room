package cli

import (
	"context"
	"fmt"
	"io"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/phm-aguiar/ai-memory-go/internal/wiki"
)

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

	rep, err := wiki.Reindex(ctx, app.Store, filepath.Join(app.Cfg.DataDir(), wikiDirName))
	if err != nil {
		return fmt.Errorf("cli: reindex: %w", err)
	}
	if err := writeReindexReport(cmd.OutOrStdout(), rep); err != nil {
		return fmt.Errorf("cli: reindex: escrever relatório: %w", err)
	}
	return nil
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
