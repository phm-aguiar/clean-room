package storage

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"embed"
	"encoding/hex"
	"errors"
	"io/fs"
	"path/filepath"
	"testing"
	"testing/fstest"

	_ "modernc.org/sqlite"
)

//go:embed migrations/*.sql
var migTestFS embed.FS

// migProductionFS devolve as migrações reais (V01__init.sql) na raiz.
func migProductionFS(t *testing.T) fs.FS {
	t.Helper()
	sub, err := fs.Sub(migTestFS, "migrations")
	if err != nil {
		t.Fatal(err)
	}
	return sub
}

// migOpenDB abre um SQLite em arquivo temporário com uma única conexão,
// para que PRAGMAs de conexão sejam observáveis pelo teste.
func migOpenDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = db.Close() })
	return db
}

func migCountRows(t *testing.T, db *sql.DB) int {
	t.Helper()
	var n int
	if err := db.QueryRow(`SELECT COUNT(*) FROM schema_migrations`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func migTableExists(t *testing.T, db *sql.DB, name string) bool {
	t.Helper()
	var n int
	if err := db.QueryRow(
		`SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name = ?`, name,
	).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n > 0
}

func TestMigraAplicaV01EmBancoVazio(t *testing.T) {
	db := migOpenDB(t)
	ctx := context.Background()

	avisos, err := Migrate(ctx, db, migProductionFS(t))
	if err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	if len(avisos) != 0 {
		t.Fatalf("avisos = %v, want none", avisos)
	}
	if !migTableExists(t, db, "schema_migrations") {
		t.Fatal("schema_migrations not created by V01")
	}
	if !migTableExists(t, db, "workspaces") {
		t.Fatal("workspaces not created by V01")
	}

	var version int64
	var name, checksum string
	var appliedOn int64
	if err := db.QueryRow(
		`SELECT version, name, applied_on, checksum FROM schema_migrations`,
	).Scan(&version, &name, &appliedOn, &checksum); err != nil {
		t.Fatal(err)
	}
	if version != 1 || name != "init" {
		t.Fatalf("row = (%d, %q), want (1, \"init\")", version, name)
	}
	if appliedOn <= 0 {
		t.Fatalf("applied_on = %d, want positive microseconds", appliedOn)
	}
	raw, err := fs.ReadFile(migProductionFS(t), "V01__init.sql")
	if err != nil {
		t.Fatal(err)
	}
	if want := migSHA256Hex(raw); checksum != want {
		t.Fatalf("checksum = %s, want %s", checksum, want)
	}
}

func TestMigraReexecucaoEhNoOp(t *testing.T) {
	db := migOpenDB(t)
	ctx := context.Background()
	fsys := migProductionFS(t)

	if _, err := Migrate(ctx, db, fsys); err != nil {
		t.Fatalf("first Migrate: %v", err)
	}
	var before int64
	if err := db.QueryRow(`SELECT applied_on FROM schema_migrations WHERE version = 1`).Scan(&before); err != nil {
		t.Fatal(err)
	}

	avisos, err := Migrate(ctx, db, fsys)
	if err != nil {
		t.Fatalf("second Migrate: %v", err)
	}
	if len(avisos) != 0 {
		t.Fatalf("avisos = %v, want none", avisos)
	}
	if n := migCountRows(t, db); n != 1 {
		t.Fatalf("rows = %d, want 1", n)
	}
	var after int64
	if err := db.QueryRow(`SELECT applied_on FROM schema_migrations WHERE version = 1`).Scan(&after); err != nil {
		t.Fatal(err)
	}
	if after != before {
		t.Fatalf("applied_on changed on re-run: %d -> %d", before, after)
	}
}

func TestMigraFKVoltamLigadas(t *testing.T) {
	db := migOpenDB(t)

	if _, err := Migrate(context.Background(), db, migProductionFS(t)); err != nil {
		t.Fatalf("Migrate: %v", err)
	}

	var fk int
	if err := db.QueryRow(`PRAGMA foreign_keys`).Scan(&fk); err != nil {
		t.Fatal(err)
	}
	if fk != 1 {
		t.Fatalf("PRAGMA foreign_keys = %d after Migrate, want 1", fk)
	}
}

func TestMigraVersaoAFrenteErrSchemaAhead(t *testing.T) {
	db := migOpenDB(t)
	ctx := context.Background()

	if _, err := Migrate(ctx, db, migProductionFS(t)); err != nil {
		t.Fatalf("setup Migrate: %v", err)
	}
	// Simula banco criado por binário mais novo: versão 3 aplicada.
	if _, err := db.Exec(
		`INSERT INTO schema_migrations (version, name, applied_on, checksum) VALUES (3, 'future', 0, 'x')`,
	); err != nil {
		t.Fatal(err)
	}

	// Binário conhece V01 e V02; V02 cria tabela que não pode aparecer se a recusa for antes de aplicar.
	v01, err := fs.ReadFile(migProductionFS(t), "V01__init.sql")
	if err != nil {
		t.Fatal(err)
	}
	fsys := fstest.MapFS{
		"V01__init.sql":  {Data: v01},
		"V02__extra.sql": {Data: []byte("CREATE TABLE extra_t (x INTEGER);")},
	}

	_, err = Migrate(ctx, db, fsys)
	if !errors.Is(err, ErrSchemaAhead) {
		t.Fatalf("err = %v, want ErrSchemaAhead", err)
	}
	if migTableExists(t, db, "extra_t") {
		t.Fatal("V02 applied despite ErrSchemaAhead")
	}
	if n := migCountRows(t, db); n != 2 {
		t.Fatalf("rows = %d, want 2 (V01 + injected row, nothing new)", n)
	}
}

func TestMigraChecksumDivergenteNaoFalha(t *testing.T) {
	db := migOpenDB(t)
	ctx := context.Background()
	fsys := migProductionFS(t)

	if _, err := Migrate(ctx, db, fsys); err != nil {
		t.Fatalf("setup Migrate: %v", err)
	}
	if _, err := db.Exec(`UPDATE schema_migrations SET checksum = 'divergente' WHERE version = 1`); err != nil {
		t.Fatal(err)
	}

	avisos, err := Migrate(ctx, db, fsys)
	if err != nil {
		t.Fatalf("Migrate with divergent checksum: %v", err)
	}
	if len(avisos) != 1 {
		t.Fatalf("avisos = %v, want exactly 1", avisos)
	}
	if n := migCountRows(t, db); n != 1 {
		t.Fatalf("rows = %d, want 1 (no reapply)", n)
	}
	var checksum string
	if err := db.QueryRow(`SELECT checksum FROM schema_migrations WHERE version = 1`).Scan(&checksum); err != nil {
		t.Fatal(err)
	}
	if checksum != "divergente" {
		t.Fatalf("stored checksum overwritten to %q", checksum)
	}
}

func TestMigraNomeForaDoPadraoFalha(t *testing.T) {
	v01, err := fs.ReadFile(migProductionFS(t), "V01__init.sql")
	if err != nil {
		t.Fatal(err)
	}

	casos := []string{
		"init.sql",
		"V01init.sql",
		"v01__init.sql",
		"V01__init.txt",
		"V01__ini t.sql",
	}
	for _, nome := range casos {
		t.Run(nome, func(t *testing.T) {
			db := migOpenDB(t)
			fsys := fstest.MapFS{
				"V01__init.sql": {Data: v01},
				nome:            {Data: []byte("SELECT 1;")},
			}
			if _, err := Migrate(context.Background(), db, fsys); err == nil {
				t.Fatalf("Migrate with file %q: err = nil, want error", nome)
			}
			if migTableExists(t, db, "schema_migrations") {
				t.Fatal("nothing should be applied when a file name is invalid")
			}
		})
	}
}

func TestMigraOrdenaPorNumero(t *testing.T) {
	db := migOpenDB(t)
	v01, err := fs.ReadFile(migProductionFS(t), "V01__init.sql")
	if err != nil {
		t.Fatal(err)
	}
	// Ordem lexicográfica colocaria V10 antes de V02 e o ALTER falharia.
	fsys := fstest.MapFS{
		"V01__init.sql":  {Data: v01},
		"V02__table.sql": {Data: []byte("CREATE TABLE ordem_t (x INTEGER);")},
		"V10__alter.sql": {Data: []byte("ALTER TABLE ordem_t ADD COLUMN y INTEGER;")},
	}

	if _, err := Migrate(context.Background(), db, fsys); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	if n := migCountRows(t, db); n != 3 {
		t.Fatalf("rows = %d, want 3", n)
	}
}

func migSHA256Hex(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}
