package storage

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	_ "modernc.org/sqlite"
)

// openTestStore abre o banco em dir e fecha ao fim do teste.
func openTestStore(t *testing.T, dir string) *sql.DB {
	t.Helper()
	db, err := Open(context.Background(), dir)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}

func openTableExists(t *testing.T, db *sql.DB, name string) bool {
	t.Helper()
	var n int
	if err := db.QueryRow(
		`SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name = ?`, name,
	).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n == 1
}

func openGlobBak(t *testing.T, dbDir string) []string {
	t.Helper()
	matches, err := filepath.Glob(filepath.Join(dbDir, "*.bak"))
	if err != nil {
		t.Fatal(err)
	}
	return matches
}

func TestOpenCreatesDBAndAppliesV01(t *testing.T) {
	dir := t.TempDir()
	db := openTestStore(t, dir)

	path := filepath.Join(dir, "db", "memory.sqlite")
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("database file not created at %s: %v", path, err)
	}
	if !openTableExists(t, db, "schema_migrations") {
		t.Fatal("schema_migrations table missing: V01 not applied")
	}
	var n int
	if err := db.QueryRow(`SELECT COUNT(*) FROM schema_migrations WHERE version = 1`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("schema_migrations rows for version 1 = %d, want 1", n)
	}
}

func TestOpenPragmas(t *testing.T) {
	db := openTestStore(t, t.TempDir())

	var journal string
	if err := db.QueryRow(`PRAGMA journal_mode`).Scan(&journal); err != nil {
		t.Fatal(err)
	}
	if journal != "wal" {
		t.Fatalf("journal_mode = %q, want wal", journal)
	}

	// busy_timeout e synchronous são por conexão: confere em duas conexões simultâneas.
	c1, err := db.Conn(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = c1.Close() }()
	c2, err := db.Conn(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = c2.Close() }()

	for i, c := range []*sql.Conn{c1, c2} {
		var busy int
		if err := c.QueryRowContext(context.Background(), `PRAGMA busy_timeout`).Scan(&busy); err != nil {
			t.Fatal(err)
		}
		if busy != 5000 {
			t.Fatalf("conn %d busy_timeout = %d, want 5000", i, busy)
		}
		var sync int
		if err := c.QueryRowContext(context.Background(), `PRAGMA synchronous`).Scan(&sync); err != nil {
			t.Fatal(err)
		}
		if sync != 1 {
			t.Fatalf("conn %d synchronous = %d, want 1 (NORMAL)", i, sync)
		}
	}
}

// TestOpenForeignKeysOnEveryPoolConn garante que foreign_keys vem do DSN e não só
// do Migrate: duas conexões simultâneas do pool precisam ter FK ligado.
func TestOpenForeignKeysOnEveryPoolConn(t *testing.T) {
	db := openTestStore(t, t.TempDir())
	ctx := context.Background()

	c1, err := db.Conn(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = c1.Close() }()
	c2, err := db.Conn(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = c2.Close() }()

	for i, c := range []*sql.Conn{c1, c2} {
		var fk int
		if err := c.QueryRowContext(ctx, `PRAGMA foreign_keys`).Scan(&fk); err != nil {
			t.Fatal(err)
		}
		if fk != 1 {
			t.Fatalf("conn %d foreign_keys = %d, want 1", i, fk)
		}
	}
}

// TestOpenForeignKeyEnforcedOnPoolConn insere projects com workspace_id inexistente
// em duas conexões simultâneas do pool; ambas devem falhar com erro de FOREIGN KEY.
func TestOpenForeignKeyEnforcedOnPoolConn(t *testing.T) {
	db := openTestStore(t, t.TempDir())
	ctx := context.Background()

	c1, err := db.Conn(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = c1.Close() }()
	c2, err := db.Conn(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = c2.Close() }()

	projectID := make([]byte, 16)
	projectID[15] = 1
	missingWorkspaceID := make([]byte, 16)

	for i, c := range []*sql.Conn{c1, c2} {
		_, err := c.ExecContext(ctx,
			`INSERT INTO projects (id, workspace_id, name, created_at) VALUES (?, ?, 'fk-probe', 0)`,
			projectID, missingWorkspaceID,
		)
		if err == nil {
			t.Fatalf("conn %d: insert with missing workspace_id succeeded: FK not enforced", i)
		}
		if !strings.Contains(err.Error(), "FOREIGN KEY") {
			t.Fatalf("conn %d: insert error = %v, want FOREIGN KEY constraint", i, err)
		}
	}
}

func TestOpenReopenDoesNotReapply(t *testing.T) {
	dir := t.TempDir()

	db1, err := Open(context.Background(), dir)
	if err != nil {
		t.Fatalf("first Open: %v", err)
	}
	var appliedOn1 int64
	if err := db1.QueryRow(`SELECT applied_on FROM schema_migrations WHERE version = 1`).Scan(&appliedOn1); err != nil {
		t.Fatal(err)
	}
	if err := db1.Close(); err != nil {
		t.Fatal(err)
	}

	db2 := openTestStore(t, dir)
	var rows int
	var appliedOn2 int64
	if err := db2.QueryRow(`SELECT COUNT(*), MIN(applied_on) FROM schema_migrations`).Scan(&rows, &appliedOn2); err != nil {
		t.Fatal(err)
	}
	if rows != 1 {
		t.Fatalf("schema_migrations rows after reopen = %d, want 1", rows)
	}
	if appliedOn2 != appliedOn1 {
		t.Fatalf("applied_on changed on reopen: %d -> %d (migration reapplied)", appliedOn1, appliedOn2)
	}
	if baks := openGlobBak(t, filepath.Join(dir, "db")); len(baks) != 0 {
		t.Fatalf("snapshot created on reopen without pending migration: %v", baks)
	}
}

func TestOpenNoSnapshotOnNewDB(t *testing.T) {
	dir := t.TempDir()
	openTestStore(t, dir)

	if baks := openGlobBak(t, filepath.Join(dir, "db")); len(baks) != 0 {
		t.Fatalf("snapshot created for new database: %v", baks)
	}
}

func TestOpenSnapshotOnlyWhenPending(t *testing.T) {
	dir := t.TempDir()
	dbDir := filepath.Join(dir, "db")
	if err := os.MkdirAll(dbDir, 0o700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dbDir, "memory.sqlite")

	// Banco existente sem schema_migrations: há migração pendente.
	raw, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := raw.Exec(`CREATE TABLE legacy (x INTEGER); INSERT INTO legacy (x) VALUES (42)`); err != nil {
		t.Fatal(err)
	}
	if err := raw.Close(); err != nil {
		t.Fatal(err)
	}

	db := openTestStore(t, dir)
	if !openTableExists(t, db, "schema_migrations") {
		t.Fatal("V01 not applied to existing database")
	}

	snap := filepath.Join(dbDir, "memory.sqlite.pre-migrate-0.bak")
	if _, err := os.Stat(snap); err != nil {
		t.Fatalf("snapshot not created at %s: %v", snap, err)
	}
	baks := openGlobBak(t, dbDir)
	if len(baks) != 1 {
		t.Fatalf("snapshots = %v, want exactly one", baks)
	}

	// O snapshot é o estado anterior à migração: tem a tabela legacy e não tem schema_migrations.
	snapDB, err := sql.Open("sqlite", snap)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = snapDB.Close() }()
	var x int
	if err := snapDB.QueryRow(`SELECT x FROM legacy`).Scan(&x); err != nil {
		t.Fatalf("snapshot missing legacy data: %v", err)
	}
	if x != 42 {
		t.Fatalf("snapshot legacy.x = %d, want 42", x)
	}
	if openTableExists(t, snapDB, "schema_migrations") {
		t.Fatal("snapshot already contains schema_migrations: taken after migration")
	}
}

func TestOpenPermissionsUnix(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("permissões Unix não se aplicam ao Windows")
	}
	dir := t.TempDir()
	openTestStore(t, dir)

	dinfo, err := os.Stat(filepath.Join(dir, "db"))
	if err != nil {
		t.Fatal(err)
	}
	if perm := dinfo.Mode().Perm(); perm != 0o700 {
		t.Fatalf("db dir perm = %o, want 700", perm)
	}
	finfo, err := os.Stat(filepath.Join(dir, "db", "memory.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	if perm := finfo.Mode().Perm(); perm != 0o600 {
		t.Fatalf("database perm = %o, want 600", perm)
	}
}
