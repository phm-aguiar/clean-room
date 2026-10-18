package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
)

const (
	flagTo      = "to"
	flagToShort = "o"

	// backupRoute é a rota administrativa que devolve o tarball (docs/lifecycle-ops.md, backup).
	backupRoute = "/admin/backup"
	// backupMaxErrBody limita a leitura do corpo de erro JSON do servidor.
	backupMaxErrBody = 64 << 10
	backupMediaType  = "application/gzip"
)

// NewBackupCommand devolve o subcomando "backup --to <arquivo>" (atalho -o). O backup
// não é direto: o comando é um cliente fino que pede ao servidor, por POST /admin/backup,
// um snapshot online do banco junto com wiki/ e config.toml, e grava o corpo do tarball
// no arquivo de destino. O comando não abre o banco nem toca o disco do data_dir. O
// comando é adicionado à raiz por NewRootCommand.
func NewBackupCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "backup",
		Short: "Pede ao servidor um tarball de backup e grava em um arquivo novo",
		Args:  cobra.NoArgs,
		RunE:  runBackup,
	}
	cmd.Flags().StringP(flagTo, flagToShort, "", "caminho do arquivo de destino do tarball (não pode existir)")
	_ = cmd.MarkFlagRequired(flagTo)
	return cmd
}

// runBackup valida o destino, carrega a configuração (para saber o endereço do servidor
// e o token) e grava a resposta de POST /admin/backup. Nada é gravado em caso de erro.
func runBackup(cmd *cobra.Command, _ []string) error {
	to, err := cmd.Flags().GetString(flagTo)
	if err != nil {
		return fmt.Errorf("cli: flag --%s: %w", flagTo, err)
	}
	if to == "" {
		return fmt.Errorf("cli: flag --%s vazia", flagTo)
	}
	if err := checarDestinoBackup(to); err != nil {
		return err
	}

	opts, err := ConfigOptions(cmd)
	if err != nil {
		return err
	}
	cfg, err := loadConfig(opts)
	if err != nil {
		return err
	}

	ctx := cmd.Context()
	if ctx == nil {
		ctx = context.Background()
	}

	resp, err := pedirBackup(ctx, cfg.ServerURL(), cfg.AuthToken())
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()

	if err := gravarBackup(to, resp.Body); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(cmd.OutOrStdout(), "backup gravado em %s\n", to); err != nil {
		return fmt.Errorf("cli: backup: escrever saída: %w", err)
	}
	return nil
}

// checarDestinoBackup recusa destino que já existe e diretório pai que não existe, antes
// de qualquer pedido ao servidor. Não cria nada.
func checarDestinoBackup(to string) error {
	if _, err := os.Lstat(to); err == nil {
		return fmt.Errorf("cli: backup: destino já existe: %s", to)
	} else if !errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("cli: backup: ler destino: %w", err)
	}
	pai := filepath.Dir(to)
	fi, err := os.Stat(pai)
	if err != nil {
		return fmt.Errorf("cli: backup: diretório de destino: %w", err)
	}
	if !fi.IsDir() {
		return fmt.Errorf("cli: backup: diretório de destino não é diretório: %s", pai)
	}
	return nil
}

// pedirBackup faz POST /admin/backup no servidor e devolve a resposta só quando ela é
// 200 com Content-Type application/gzip. O chamador fecha o corpo. Fora disso lê o erro
// JSON {"error": "..."} e o devolve como erro; o corpo binário nunca chega ao terminal.
// O token, quando existe, vai no cabeçalho Authorization e não aparece em mensagens.
func pedirBackup(ctx context.Context, serverURL, token string) (*http.Response, error) {
	url := strings.TrimRight(serverURL, "/") + backupRoute
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, nil)
	if err != nil {
		return nil, fmt.Errorf("cli: backup: montar requisição: %w", err)
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("cli: backup: contatar servidor %s: %w", serverURL, err)
	}
	if resp.StatusCode != http.StatusOK {
		defer func() { _ = resp.Body.Close() }()
		return nil, erroDoServidor(resp)
	}

	ct := resp.Header.Get("Content-Type")
	if mt, _, perr := mime.ParseMediaType(ct); perr != nil || mt != backupMediaType {
		_ = resp.Body.Close()
		return nil, fmt.Errorf("cli: backup: resposta do servidor não é %s (Content-Type %q)", backupMediaType, ct)
	}
	return resp, nil
}

// erroDoServidor monta o erro de uma resposta que não é 200 a partir do JSON de erro.
func erroDoServidor(resp *http.Response) error {
	data, _ := io.ReadAll(io.LimitReader(resp.Body, backupMaxErrBody))
	var body struct {
		Error string `json:"error"`
	}
	if err := json.Unmarshal(data, &body); err == nil && body.Error != "" {
		return fmt.Errorf("cli: backup: servidor respondeu %d: %s", resp.StatusCode, body.Error)
	}
	return fmt.Errorf("cli: backup: servidor respondeu %d sem mensagem de erro", resp.StatusCode)
}

// gravarBackup cria to com O_EXCL e copia o corpo. Se a cópia ou o fechamento falham,
// remove o arquivo parcial, para não deixar um tarball truncado no lugar do destino.
func gravarBackup(to string, body io.Reader) (err error) {
	f, err := os.OpenFile(to, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return fmt.Errorf("cli: backup: criar arquivo: %w", err)
	}
	defer func() {
		if cerr := f.Close(); cerr != nil && err == nil {
			err = fmt.Errorf("cli: backup: fechar arquivo: %w", cerr)
		}
		if err != nil {
			_ = os.Remove(to)
		}
	}()
	if _, cerr := io.Copy(f, body); cerr != nil {
		return fmt.Errorf("cli: backup: gravar arquivo: %w", cerr)
	}
	return nil
}
