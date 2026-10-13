package cli

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/phm-aguiar/ai-memory-go/internal/storage"
)

const flagTo = "to"

// NewBackupCommand devolve o subcomando "backup --to <arquivo>". O backup é online
// (INV-09): não passa pela guarda de processo vivo de liveguard.go e copia o banco
// pela API de backup do SQLite enquanto o escritor segue ativo. O comando é
// adicionado à raiz por quem monta a CLI: root.AddCommand(NewBackupCommand()).
func NewBackupCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "backup",
		Short: "Grava um snapshot online do banco em um arquivo novo",
		Args:  cobra.NoArgs,
		RunE:  runBackup,
	}
	cmd.Flags().String(flagTo, "", "caminho do arquivo de destino do snapshot (não pode existir)")
	_ = cmd.MarkFlagRequired(flagTo)
	return cmd
}

// runBackup monta o App com Compose e grava o snapshot do pool de storage.
// Erros voltam com %w; o App é fechado ao final e um erro de fechamento também é
// devolvido quando a cópia deu certo.
func runBackup(cmd *cobra.Command, _ []string) (err error) {
	to, err := cmd.Flags().GetString(flagTo)
	if err != nil {
		return fmt.Errorf("cli: flag --%s: %w", flagTo, err)
	}
	if to == "" {
		return fmt.Errorf("cli: flag --%s vazia", flagTo)
	}

	opts, err := ConfigOptions(cmd)
	if err != nil {
		return err
	}

	ctx := cmd.Context()
	if ctx == nil {
		ctx = context.Background()
	}

	app, err := Compose(ctx, opts)
	if err != nil {
		return err
	}
	defer func() {
		if cerr := app.Close(); cerr != nil && err == nil {
			err = cerr
		}
	}()

	if err := storage.Snapshot(ctx, app.DB, to); err != nil {
		return fmt.Errorf("cli: backup: %w", err)
	}
	if _, err := fmt.Fprintf(cmd.OutOrStdout(), "backup gravado em %s\n", to); err != nil {
		return fmt.Errorf("cli: backup: escrever saída: %w", err)
	}
	return nil
}
