package storage

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"modernc.org/sqlite"
)

// snapshotPagesPerStep é quantas páginas cada passo da API de backup copia.
// Passos pequenos deixam o escritor intercalar commits durante a cópia; o
// SQLite reinicia a cópia quando o banco de origem muda entre passos, então a
// cópia final sempre reflete um estado consistente.
const snapshotPagesPerStep = 16

// snapshotStepHook é chamado depois de cada passo que ainda tem páginas a copiar,
// com a cópia aberta. É nil em produção; os testes o usam para provar que o
// escritor continua funcionando enquanto o backup está em andamento.
var snapshotStepHook func()

// backupSource é o método de backup exposto pelo driver modernc.org/sqlite em
// sua conexão interna (*conn.NewBackup). A conexão não é exportada, então o
// acesso é feito por type assertion sobre o driver.Conn obtido em sql.Conn.Raw.
type backupSource interface {
	NewBackup(dstURI string) (*sqlite.Backup, error)
}

// Snapshot copia o banco vivo de db para destPath usando a API de backup online
// do SQLite (sqlite3_backup_*). A cópia roda sem interromper o escritor: leitores
// e escritores seguem ativos durante a operação e o arquivo resultante é um banco
// válido, com as migrações aplicadas.
//
// Regras:
//   - se destPath já existe, devolve erro e não toca no arquivo existente;
//   - se o diretório pai de destPath não existe, devolve erro (não é criado);
//   - em qualquer falha, o arquivo de destino criado por esta chamada é removido.
//
// O chamador escolhe o pool: qualquer *sql.DB aberto pelo pacote serve.
func Snapshot(ctx context.Context, db *sql.DB, destPath string) (err error) {
	if strings.ContainsRune(destPath, '?') {
		// NewBackup recebe o caminho como DSN; '?' iniciaria a lista de parâmetros.
		return fmt.Errorf("storage snapshot: caminho de destino não pode conter '?': %s", destPath)
	}

	dir := filepath.Dir(destPath)
	info, err := os.Stat(dir)
	if err != nil {
		return fmt.Errorf("storage snapshot: diretório de destino: %w", err)
	}
	if !info.IsDir() {
		return fmt.Errorf("storage snapshot: diretório de destino não é diretório: %s", dir)
	}

	// O_EXCL torna a recusa atômica: se outro processo criar o arquivo entre a
	// checagem e a escrita, esta chamada falha em vez de sobrescrever.
	f, err := os.OpenFile(destPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		if errors.Is(err, fs.ErrExist) {
			return fmt.Errorf("storage snapshot: destino já existe: %s", destPath)
		}
		return fmt.Errorf("storage snapshot: criar destino: %w", err)
	}
	if err := f.Close(); err != nil {
		_ = os.Remove(destPath)
		return fmt.Errorf("storage snapshot: fechar destino: %w", err)
	}

	defer func() {
		if err != nil {
			removeSnapshotFiles(destPath)
		}
	}()

	return backupInto(ctx, db, destPath)
}

// backupInto roda a cópia online de db para destPath, que já existe e está vazio.
func backupInto(ctx context.Context, db *sql.DB, destPath string) error {
	conn, err := db.Conn(ctx)
	if err != nil {
		return fmt.Errorf("storage snapshot: connection: %w", err)
	}
	defer func() { _ = conn.Close() }()

	return conn.Raw(func(driverConn any) error {
		src, ok := driverConn.(backupSource)
		if !ok {
			return fmt.Errorf("storage snapshot: driver %T sem API de backup", driverConn)
		}

		bck, err := src.NewBackup(destPath)
		if err != nil {
			return fmt.Errorf("storage snapshot: iniciar backup: %w", err)
		}

		stepErr := copyPages(ctx, bck)
		// Finish libera o backup e fecha a conexão de destino mesmo após erro.
		finErr := bck.Finish()
		if stepErr != nil {
			return stepErr
		}
		if finErr != nil {
			return fmt.Errorf("storage snapshot: finalizar backup: %w", finErr)
		}
		return nil
	})
}

// copyPages avança o backup até copiar todas as páginas, respeitando ctx entre passos.
func copyPages(ctx context.Context, bck *sqlite.Backup) error {
	for {
		if err := ctx.Err(); err != nil {
			return fmt.Errorf("storage snapshot: %w", err)
		}
		more, err := bck.Step(snapshotPagesPerStep)
		if err != nil {
			return fmt.Errorf("storage snapshot: copiar páginas: %w", err)
		}
		if !more {
			return nil
		}
		if snapshotStepHook != nil {
			snapshotStepHook()
		}
	}
}

// removeSnapshotFiles remove o destino e os arquivos auxiliares do WAL, se existirem.
// Erros são ignorados: é limpeza após falha que já será devolvida ao chamador.
func removeSnapshotFiles(destPath string) {
	for _, suffix := range []string{"", "-wal", "-shm", "-journal"} {
		_ = os.Remove(destPath + suffix)
	}
}
