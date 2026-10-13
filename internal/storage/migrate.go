package storage

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"fmt"
	"io/fs"
	"regexp"
	"sort"
	"strconv"
	"time"
)

// migrationNameRE casa com V<nn>__<nome>.sql (plan P-01, ADR-34).
var migrationNameRE = regexp.MustCompile(`^V(\d+)__([A-Za-z0-9_]+)\.sql$`)

// migration é uma migração embutida já validada e com checksum calculado.
type migration struct {
	version  int64
	name     string
	file     string
	sql      string
	checksum string
}

// Migrate aplica em db as migrações de fsys que ainda não constam em schema_migrations.
//
// fsys tem os arquivos V<nn>__<nome>.sql na raiz. Cada migração roda em uma
// transação própria. PRAGMA foreign_keys fica OFF durante a aplicação e volta
// ON ao fim. Divergência de checksum em migração já aplicada não é erro: vira
// aviso na lista retornada. Banco com versão maior que a maior migração de
// fsys retorna ErrSchemaAhead sem aplicar nada.
func Migrate(ctx context.Context, db *sql.DB, fsys fs.FS) (avisos []string, err error) {
	migs, err := loadMigrations(fsys)
	if err != nil {
		return nil, err
	}

	conn, err := db.Conn(ctx)
	if err != nil {
		return nil, fmt.Errorf("migrate: open connection: %w", err)
	}
	defer func() { _ = conn.Close() }()

	applied, err := loadApplied(ctx, conn)
	if err != nil {
		return nil, err
	}

	var maxApplied int64
	for v := range applied {
		if v > maxApplied {
			maxApplied = v
		}
	}
	var maxEmbedded int64
	if n := len(migs); n > 0 {
		maxEmbedded = migs[n-1].version
	}
	if maxApplied > maxEmbedded {
		return nil, fmt.Errorf("migrate: database at version %d, binary up to %d: %w",
			maxApplied, maxEmbedded, ErrSchemaAhead)
	}

	for _, m := range migs {
		if stored, ok := applied[m.version]; ok && stored != m.checksum {
			avisos = append(avisos, fmt.Sprintf(
				"checksum divergente em %s: banco %s, arquivo %s", m.file, stored, m.checksum))
		}
	}

	// PRAGMA foreign_keys é por conexão e é ignorado dentro de transação:
	// por isso roda na mesma conexão, fora de qualquer tx.
	if _, err = conn.ExecContext(ctx, "PRAGMA foreign_keys = OFF"); err != nil {
		return avisos, fmt.Errorf("migrate: disable foreign keys: %w", err)
	}
	defer func() {
		if _, rerr := conn.ExecContext(context.Background(), "PRAGMA foreign_keys = ON"); rerr != nil && err == nil {
			err = fmt.Errorf("migrate: enable foreign keys: %w", rerr)
		}
	}()

	for _, m := range migs {
		if _, ok := applied[m.version]; ok {
			continue
		}
		if err = applyMigration(ctx, conn, m); err != nil {
			return avisos, err
		}
	}
	return avisos, nil
}

// loadMigrations lê fsys, valida nomes, rejeita versões duplicadas e ordena por número.
func loadMigrations(fsys fs.FS) ([]migration, error) {
	entries, err := fs.ReadDir(fsys, ".")
	if err != nil {
		return nil, fmt.Errorf("migrate: read migrations: %w", err)
	}

	var migs []migration
	seen := make(map[int64]string, len(entries))
	for _, e := range entries {
		match := migrationNameRE.FindStringSubmatch(e.Name())
		if e.IsDir() || match == nil {
			return nil, fmt.Errorf("migrate: invalid migration file name %q (expected V<nn>__<nome>.sql)", e.Name())
		}
		version, err := strconv.ParseInt(match[1], 10, 64)
		if err != nil || version < 1 {
			return nil, fmt.Errorf("migrate: invalid version in %q", e.Name())
		}
		if other, dup := seen[version]; dup {
			return nil, fmt.Errorf("migrate: duplicate version %d: %s and %s", version, other, e.Name())
		}
		seen[version] = e.Name()

		content, err := fs.ReadFile(fsys, e.Name())
		if err != nil {
			return nil, fmt.Errorf("migrate: read %s: %w", e.Name(), err)
		}
		sum := sha256.Sum256(content)
		migs = append(migs, migration{
			version:  version,
			name:     match[2],
			file:     e.Name(),
			sql:      string(content),
			checksum: hex.EncodeToString(sum[:]),
		})
	}

	sort.Slice(migs, func(i, j int) bool { return migs[i].version < migs[j].version })
	return migs, nil
}

// loadApplied devolve versão -> checksum gravado. Sem a tabela, o banco está vazio.
func loadApplied(ctx context.Context, conn *sql.Conn) (map[int64]string, error) {
	var tables int
	err := conn.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name = 'schema_migrations'`,
	).Scan(&tables)
	if err != nil {
		return nil, fmt.Errorf("migrate: check schema_migrations: %w", err)
	}

	applied := make(map[int64]string)
	if tables == 0 {
		return applied, nil
	}

	rows, err := conn.QueryContext(ctx, `SELECT version, checksum FROM schema_migrations`)
	if err != nil {
		return nil, fmt.Errorf("migrate: read schema_migrations: %w", err)
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var version int64
		var checksum string
		if err := rows.Scan(&version, &checksum); err != nil {
			return nil, fmt.Errorf("migrate: scan schema_migrations: %w", err)
		}
		applied[version] = checksum
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("migrate: iterate schema_migrations: %w", err)
	}
	return applied, nil
}

// applyMigration executa o SQL e registra a linha em schema_migrations na mesma transação.
func applyMigration(ctx context.Context, conn *sql.Conn, m migration) error {
	tx, err := conn.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("migrate %s: begin: %w", m.file, err)
	}
	if _, err := tx.ExecContext(ctx, m.sql); err != nil {
		_ = tx.Rollback()
		return fmt.Errorf("migrate %s: exec: %w", m.file, err)
	}
	if _, err := tx.ExecContext(ctx,
		`INSERT INTO schema_migrations (version, name, applied_on, checksum) VALUES (?, ?, ?, ?)`,
		m.version, m.name, time.Now().UTC().UnixMicro(), m.checksum,
	); err != nil {
		_ = tx.Rollback()
		return fmt.Errorf("migrate %s: record: %w", m.file, err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("migrate %s: commit: %w", m.file, err)
	}
	return nil
}
