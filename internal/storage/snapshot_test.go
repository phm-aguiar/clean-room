package storage

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// snapshotProbeRows é o volume de dados das sondas: com ~200 bytes por linha
// ocupa dezenas de páginas, o que força vários passos de cópia.
const snapshotProbeRows = 2000

// openSnapshotSource abre um banco real (Open + migrações) com a tabela de sonda
// preenchida com snapshotProbeRows linhas.
func openSnapshotSource(t *testing.T) (dir string, db *sql.DB) {
	t.Helper()
	dir = t.TempDir()
	db = openTestStore(t, dir)
	if _, err := db.Exec(`CREATE TABLE snap_probe (id INTEGER PRIMARY KEY, v TEXT NOT NULL)`); err != nil {
		t.Fatalf("create probe table: %v", err)
	}
	insertSnapshotRows(t, db, 0, snapshotProbeRows)
	return dir, db
}

// insertSnapshotRows insere n linhas começando em from, em uma única transação.
func insertSnapshotRows(t *testing.T, db *sql.DB, from, n int) {
	t.Helper()
	tx, err := db.Begin()
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	payload := strings.Repeat("x", 200)
	for i := from; i < from+n; i++ {
		if _, err := tx.Exec(`INSERT INTO snap_probe (v) VALUES (?)`, fmt.Sprintf("%s-%d", payload, i)); err != nil {
			_ = tx.Rollback()
			t.Fatalf("insert %d: %v", i, err)
		}
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("commit inserts: %v", err)
	}
}

// countTable conta as linhas de table em db.
func countTable(t *testing.T, db *sql.DB, table string) int {
	t.Helper()
	var n int
	if err := db.QueryRow(`SELECT COUNT(*) FROM ` + table).Scan(&n); err != nil {
		t.Fatalf("count %s: %v", table, err)
	}
	return n
}

// openFileDB abre um arquivo SQLite já existente, sem migrar, e fecha ao fim do teste.
func openFileDB(t *testing.T, path string) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatalf("sql.Open snapshot: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}

func TestSnapshotCopiesSameRowCount(t *testing.T) {
	_, db := openSnapshotSource(t)
	destDir := t.TempDir()
	dest := filepath.Join(destDir, "copia.sqlite")

	if err := Snapshot(context.Background(), db, dest); err != nil {
		t.Fatalf("Snapshot: %v", err)
	}

	copyDB := openFileDB(t, dest)
	if got, want := countTable(t, copyDB, "snap_probe"), countTable(t, db, "snap_probe"); got != want {
		t.Fatalf("linhas no snapshot = %d, want %d (origem)", got, want)
	}
	if got := countTable(t, copyDB, "snap_probe"); got != snapshotProbeRows {
		t.Fatalf("linhas no snapshot = %d, want %d", got, snapshotProbeRows)
	}
}

func TestSnapshotRefusesExistingDest(t *testing.T) {
	_, db := openSnapshotSource(t)
	dest := filepath.Join(t.TempDir(), "existente.sqlite")
	const sentinela = "nao-sobrescrever"
	if err := os.WriteFile(dest, []byte(sentinela), 0o600); err != nil {
		t.Fatal(err)
	}

	err := Snapshot(context.Background(), db, dest)
	if err == nil {
		t.Fatal("Snapshot sobrescreveu destino existente; want erro")
	}
	if !strings.Contains(err.Error(), "já existe") {
		t.Fatalf("erro não indica destino existente: %v", err)
	}

	got, rerr := os.ReadFile(dest)
	if rerr != nil {
		t.Fatal(rerr)
	}
	if string(got) != sentinela {
		t.Fatalf("arquivo existente foi alterado: %q", got)
	}
}

func TestSnapshotRefusesMissingParentDir(t *testing.T) {
	_, db := openSnapshotSource(t)
	parent := filepath.Join(t.TempDir(), "nao-existe")
	dest := filepath.Join(parent, "copia.sqlite")

	if err := Snapshot(context.Background(), db, dest); err == nil {
		t.Fatal("Snapshot aceitou diretório pai inexistente; want erro")
	}
	if _, err := os.Stat(parent); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("diretório pai foi criado (stat err = %v); want não criado", err)
	}
}

func TestSnapshotOpensWithMigrations(t *testing.T) {
	_, db := openSnapshotSource(t)
	dataDir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dataDir, dbDirName), 0o700); err != nil {
		t.Fatal(err)
	}
	dest := filepath.Join(dataDir, dbDirName, dbFileName)

	if err := Snapshot(context.Background(), db, dest); err != nil {
		t.Fatalf("Snapshot: %v", err)
	}

	// Caminho de produção: o snapshot no lugar do banco deve abrir com Open
	// sem migrar nada, pois as migrações já constam no arquivo.
	reopened, err := Open(context.Background(), dataDir)
	if err != nil {
		t.Fatalf("Open do snapshot: %v", err)
	}
	t.Cleanup(func() { _ = reopened.Close() })

	if !openTableExists(t, reopened, "schema_migrations") {
		t.Fatal("snapshot sem schema_migrations")
	}
	var srcMigs, copyMigs int
	if err := db.QueryRow(`SELECT COUNT(*) FROM schema_migrations`).Scan(&srcMigs); err != nil {
		t.Fatal(err)
	}
	if err := reopened.QueryRow(`SELECT COUNT(*) FROM schema_migrations`).Scan(&copyMigs); err != nil {
		t.Fatal(err)
	}
	if copyMigs == 0 || copyMigs != srcMigs {
		t.Fatalf("schema_migrations no snapshot = %d, origem = %d; want igual e > 0", copyMigs, srcMigs)
	}

	var integrity string
	if err := reopened.QueryRow(`PRAGMA integrity_check`).Scan(&integrity); err != nil {
		t.Fatal(err)
	}
	if integrity != "ok" {
		t.Fatalf("integrity_check do snapshot = %q", integrity)
	}
}

// TestSnapshotWriterKeepsWritingDuringCopy copia pelo pool de escrita (Open).
func TestSnapshotWriterKeepsWritingDuringCopy(t *testing.T) {
	_, db := openSnapshotSource(t)
	snapshotWhileWriting(t, db, db)
}

// TestSnapshotFromReaderPoolWhileWriterActive copia pelo pool somente leitura
// (OpenReader), como o desenho pede, com o escritor ativo no pool de escrita.
func TestSnapshotFromReaderPoolWhileWriterActive(t *testing.T) {
	dir, db := openSnapshotSource(t)
	rdb, err := OpenReader(context.Background(), dir)
	if err != nil {
		t.Fatalf("OpenReader: %v", err)
	}
	t.Cleanup(func() { _ = rdb.Close() })
	snapshotWhileWriting(t, db, rdb)
}

// snapshotWhileWriting roda Snapshot a partir de src enquanto o escritor de
// write grava entre passos da cópia. dir é o diretório de dados de write.
func snapshotWhileWriting(t *testing.T, db, src *sql.DB) {
	t.Helper()
	ctx := context.Background()

	w, err := NewWriter(ctx, db)
	if err != nil {
		t.Fatalf("NewWriter: %v", err)
	}
	t.Cleanup(func() { _ = w.Close() })

	// Entre passos da cópia, o escritor grava uma linha. Se o backup travasse
	// a escrita, Do bloquearia ou devolveria erro. Como o hook roda com a cópia
	// aberta, a escrita acontece de fato durante o snapshot.
	var (
		hookCalls int
		writeErrs []error
	)
	snapshotStepHook = func() {
		hookCalls++
		if hookCalls > 3 {
			return
		}
		v := fmt.Sprintf("durante-%d", hookCalls)
		if err := w.Do(ctx, insertProbeRow(v)); err != nil {
			writeErrs = append(writeErrs, err)
		}
	}
	t.Cleanup(func() { snapshotStepHook = nil })

	dest := filepath.Join(t.TempDir(), "durante.sqlite")
	if err := Snapshot(ctx, src, dest); err != nil {
		t.Fatalf("Snapshot: %v", err)
	}
	snapshotStepHook = nil

	if hookCalls == 0 {
		t.Fatal("hook nunca rodou: a cópia não teve passos intermediários, o teste não provou nada")
	}
	if len(writeErrs) > 0 {
		t.Fatalf("escritas durante o snapshot falharam: %v", writeErrs)
	}

	// Depois do snapshot o escritor segue normal.
	if err := w.Do(ctx, insertProbeRow("depois")); err != nil {
		t.Fatalf("escrita após snapshot: %v", err)
	}
	if n := countProbeRow(t, db, "durante-1"); n != 1 {
		t.Fatalf("linha gravada durante a cópia: %d, want 1", n)
	}
	if n := countProbeRow(t, db, "depois"); n != 1 {
		t.Fatalf("linha gravada após a cópia: %d, want 1", n)
	}

	// O snapshot tem pelo menos os dados que existiam antes da cópia.
	copyDB := openFileDB(t, dest)
	if got := countTable(t, copyDB, "snap_probe"); got < snapshotProbeRows {
		t.Fatalf("linhas no snapshot = %d, want >= %d", got, snapshotProbeRows)
	}
}

func TestSnapshotCanceledCtxRemovesDest(t *testing.T) {
	_, db := openSnapshotSource(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	dest := filepath.Join(t.TempDir(), "cancelado.sqlite")
	if err := Snapshot(ctx, db, dest); err == nil {
		t.Fatal("Snapshot com ctx cancelado não devolveu erro")
	}
	if _, err := os.Stat(dest); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("destino parcial não foi removido (stat err = %v)", err)
	}
}

// insertProbeRow insere v na tabela snap_probe. Usa o escritor, então roda
// dentro de uma transação do ator.
func insertProbeRow(v string) func(*sql.Tx) error {
	return func(tx *sql.Tx) error {
		_, err := tx.Exec(`INSERT INTO snap_probe (v) VALUES (?)`, v)
		return err
	}
}

func countProbeRow(t *testing.T, db *sql.DB, v string) int {
	t.Helper()
	var n int
	if err := db.QueryRow(`SELECT COUNT(*) FROM snap_probe WHERE v = ?`, v).Scan(&n); err != nil {
		t.Fatalf("count snap_probe: %v", err)
	}
	return n
}
