package storage

import (
	"context"
	"database/sql"
	"embed"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"

	_ "modernc.org/sqlite"
)

// migrationsFS embute as migrações de produção. A diretiva fica aqui (e não em
// migrate.go) porque Open é o ponto de entrada de produção do banco.
//
//go:embed migrations/*.sql
var migrationsFS embed.FS

const (
	dbDirName  = "db"
	dbFileName = "memory.sqlite"

	// dsnPragmas são aplicados em toda conexão do pool no momento da abertura.
	// journal_mode é persistente no arquivo; synchronous, busy_timeout e foreign_keys são por conexão.
	// foreign_keys precisa estar aqui (e não só no Migrate) para que ON DELETE CASCADE/SET NULL
	// e os FKs do esquema valham em todas as conexões do pool.
	dsnPragmas = "?_pragma=busy_timeout(5000)&_pragma=synchronous(NORMAL)&_pragma=journal_mode(WAL)&_pragma=foreign_keys(1)"
)

// Open abre (criando se preciso) o banco <dataDir>/db/memory.sqlite, valida os
// pragmas lendo-os de volta e aplica as migrações pendentes.
//
// Se o banco já existia e há migrações pendentes, copia o arquivo para
// memory.sqlite.pre-migrate-<versão>.bak antes de migrar, onde <versão> é a
// versão aplicada antes da migração (0 se não havia schema_migrations).
//
// Permissões: em Unix, db/ fica 0700 e memory.sqlite 0600. No Windows a
// permissão não é aplicada aqui: os.Chmod não modela ACL, então nada é feito.
func Open(ctx context.Context, dataDir string) (*sql.DB, error) {
	migFS, err := fs.Sub(migrationsFS, "migrations")
	if err != nil {
		return nil, fmt.Errorf("storage open: embedded migrations: %w", err)
	}

	dbDir := filepath.Join(dataDir, dbDirName)
	if err := os.MkdirAll(dbDir, 0o700); err != nil {
		return nil, fmt.Errorf("storage open: create db dir: %w", err)
	}
	if err := restrictUnix(dbDir, 0o700); err != nil {
		return nil, fmt.Errorf("storage open: chmod db dir: %w", err)
	}

	path := filepath.Join(dbDir, dbFileName)
	existed, err := fileExists(path)
	if err != nil {
		return nil, fmt.Errorf("storage open: stat database: %w", err)
	}

	db, err := sql.Open("sqlite", path+dsnPragmas)
	if err != nil {
		return nil, fmt.Errorf("storage open: open sqlite: %w", err)
	}

	if err := verifyPragmas(ctx, db); err != nil {
		_ = db.Close()
		return nil, err
	}
	if err := restrictUnix(path, 0o600); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("storage open: chmod database: %w", err)
	}
	if existed {
		if err := snapshotIfPending(ctx, db, migFS, path, dbDir); err != nil {
			_ = db.Close()
			return nil, err
		}
	}

	// Os avisos de checksum divergente não têm canal de saída nesta assinatura;
	// ficam descartados até que T022/T023 definam como reportá-los.
	if _, err := Migrate(ctx, db, migFS); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("storage open: migrate: %w", err)
	}
	return db, nil
}

// verifyPragmas lê de volta os pragmas aplicados e falha se não baterem.
func verifyPragmas(ctx context.Context, db *sql.DB) error {
	conn, err := db.Conn(ctx)
	if err != nil {
		return fmt.Errorf("storage open: connection: %w", err)
	}
	defer func() { _ = conn.Close() }()

	var journal string
	if err := conn.QueryRowContext(ctx, `PRAGMA journal_mode`).Scan(&journal); err != nil {
		return fmt.Errorf("storage open: read journal_mode: %w", err)
	}
	if journal != "wal" {
		return fmt.Errorf("storage open: journal_mode is %q, want \"wal\"", journal)
	}

	var synchronous int
	if err := conn.QueryRowContext(ctx, `PRAGMA synchronous`).Scan(&synchronous); err != nil {
		return fmt.Errorf("storage open: read synchronous: %w", err)
	}
	// NORMAL é 1 em SQLite.
	if synchronous != 1 {
		return fmt.Errorf("storage open: synchronous is %d, want 1 (NORMAL)", synchronous)
	}

	var busy int
	if err := conn.QueryRowContext(ctx, `PRAGMA busy_timeout`).Scan(&busy); err != nil {
		return fmt.Errorf("storage open: read busy_timeout: %w", err)
	}
	if busy != 5000 {
		return fmt.Errorf("storage open: busy_timeout is %d, want 5000", busy)
	}

	var fk int
	if err := conn.QueryRowContext(ctx, `PRAGMA foreign_keys`).Scan(&fk); err != nil {
		return fmt.Errorf("storage open: read foreign_keys: %w", err)
	}
	if fk != 1 {
		return fmt.Errorf("storage open: foreign_keys is %d, want 1", fk)
	}
	return nil
}

// snapshotIfPending copia o banco antes de migrar, somente se há migração não aplicada.
func snapshotIfPending(ctx context.Context, db *sql.DB, migFS fs.FS, path, dbDir string) error {
	migs, err := loadMigrations(migFS)
	if err != nil {
		return err
	}

	conn, err := db.Conn(ctx)
	if err != nil {
		return fmt.Errorf("storage open: connection: %w", err)
	}
	applied, err := loadApplied(ctx, conn)
	_ = conn.Close()
	if err != nil {
		return err
	}

	pending := false
	for _, m := range migs {
		if _, ok := applied[m.version]; !ok {
			pending = true
			break
		}
	}
	if !pending {
		return nil
	}

	var from int64
	for v := range applied {
		if v > from {
			from = v
		}
	}

	// Traz o conteúdo do WAL para o arquivo principal, para que a cópia seja completa.
	// busy != 0 indica leitor concorrente impedindo o checkpoint: a cópia seria incompleta.
	var busy, logFrames, checkpointed int
	if err := db.QueryRowContext(ctx, `PRAGMA wal_checkpoint(TRUNCATE)`).
		Scan(&busy, &logFrames, &checkpointed); err != nil {
		return fmt.Errorf("storage open: checkpoint before snapshot: %w", err)
	}
	if busy != 0 {
		return fmt.Errorf("storage open: checkpoint before snapshot blocked by another connection")
	}

	dst := filepath.Join(dbDir, fmt.Sprintf("%s.pre-migrate-%d.bak", dbFileName, from))
	if err := copyFile(path, dst); err != nil {
		return fmt.Errorf("storage open: snapshot to %s: %w", filepath.Base(dst), err)
	}
	return nil
}

// copyFile copia src para dst com permissão 0600, sobrescrevendo dst se existir.
func copyFile(src, dst string) (err error) {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer func() { _ = in.Close() }()

	out, err := os.OpenFile(dst, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	defer func() {
		if cerr := out.Close(); cerr != nil && err == nil {
			err = cerr
		}
	}()

	if _, err = io.Copy(out, in); err != nil {
		return err
	}
	return out.Sync()
}

// restrictUnix aplica mode em Unix. No Windows não faz nada: a permissão é só Unix.
func restrictUnix(path string, mode os.FileMode) error {
	if runtime.GOOS == "windows" {
		return nil
	}
	return os.Chmod(path, mode)
}

func fileExists(path string) (bool, error) {
	_, err := os.Stat(path)
	if err == nil {
		return true, nil
	}
	if errors.Is(err, fs.ErrNotExist) {
		return false, nil
	}
	return false, err
}
