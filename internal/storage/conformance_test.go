package storage

import (
	"context"
	"database/sql"
	"errors"
	"testing"
)

// Conformidade de compilação: as implementações concretas satisfazem as
// interfaces Writer e Reader de api.go. Se a assinatura de algum método divergir,
// o pacote não compila.
var (
	_ Writer = (*writer)(nil)
	_ Reader = (*reader)(nil)
)

var errConformanceBoom = errors.New("boom conformidade")

// conformanceEnv abre o banco real (Open) em um diretório temporário, cria a
// tabela de sonda e devolve o diretório e o pool de escrita.
func conformanceEnv(t *testing.T) (dir string, write *sql.DB) {
	t.Helper()
	dir = t.TempDir()
	write, err := Open(context.Background(), dir)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = write.Close() })
	if _, err := write.Exec(`CREATE TABLE conformance_probe (id INTEGER PRIMARY KEY, v TEXT NOT NULL)`); err != nil {
		t.Fatalf("create probe table: %v", err)
	}
	return dir, write
}

// TestConformanceWriter exercita cada método de Writer em banco real:
// Do (commit, rollback e depois de Close), QueueLen e Close (incluindo idempotência).
func TestConformanceWriter(t *testing.T) {
	ctx := context.Background()
	_, db := conformanceEnv(t)

	w, err := NewWriter(ctx, db)
	if err != nil {
		t.Fatalf("NewWriter: %v", err)
	}
	t.Cleanup(func() { _ = w.Close() })

	// QueueLen ocioso devolve 0.
	if n := w.QueueLen(); n != 0 {
		t.Fatalf("QueueLen ocioso = %d, want 0", n)
	}

	// Do com fn sem erro: a transação é commitada e o efeito fica visível.
	if err := w.Do(ctx, func(tx *sql.Tx) error {
		_, err := tx.Exec(`INSERT INTO conformance_probe (v) VALUES ('commit')`)
		return err
	}); err != nil {
		t.Fatalf("Do commit: %v", err)
	}
	var n int
	if err := db.QueryRow(`SELECT COUNT(*) FROM conformance_probe WHERE v = 'commit'`).Scan(&n); err != nil {
		t.Fatalf("count commit: %v", err)
	}
	if n != 1 {
		t.Fatalf("linhas após Do commit = %d, want 1", n)
	}

	// Do com fn devolvendo erro: ROLLBACK e o mesmo erro volta ao chamador.
	err = w.Do(ctx, func(tx *sql.Tx) error {
		if _, err := tx.Exec(`INSERT INTO conformance_probe (v) VALUES ('rollback')`); err != nil {
			return err
		}
		return errConformanceBoom
	})
	if !errors.Is(err, errConformanceBoom) {
		t.Fatalf("Do rollback err = %v, want errConformanceBoom", err)
	}
	if err := db.QueryRow(`SELECT COUNT(*) FROM conformance_probe WHERE v = 'rollback'`).Scan(&n); err != nil {
		t.Fatalf("count rollback: %v", err)
	}
	if n != 0 {
		t.Fatalf("linhas de transação com ROLLBACK = %d, want 0", n)
	}

	// Close: primeira chamada sem erro; segunda devolve o mesmo resultado (idempotente).
	if err := w.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if err := w.Close(); err != nil {
		t.Fatalf("Close idempotente: %v", err)
	}

	// Depois de Close, Do devolve ErrWriterClosed e a closure não roda.
	ran := false
	err = w.Do(ctx, func(*sql.Tx) error {
		ran = true
		return nil
	})
	if !errors.Is(err, ErrWriterClosed) {
		t.Fatalf("Do após Close err = %v, want ErrWriterClosed", err)
	}
	if ran {
		t.Fatal("closure executada após Close")
	}

	// QueueLen após Close continua 0.
	if n := w.QueueLen(); n != 0 {
		t.Fatalf("QueueLen após Close = %d, want 0", n)
	}
}

// TestConformanceReader exercita Reader.Read em banco real: leitura de dado
// commitado, propagação do erro de fn com %w e leitura sem efeito de escrita.
func TestConformanceReader(t *testing.T) {
	ctx := context.Background()
	dir, write := conformanceEnv(t)

	if _, err := write.Exec(`INSERT INTO conformance_probe (v) VALUES ('lido')`); err != nil {
		t.Fatalf("insert: %v", err)
	}

	rdb, err := OpenReader(ctx, dir)
	if err != nil {
		t.Fatalf("OpenReader: %v", err)
	}
	t.Cleanup(func() { _ = rdb.Close() })

	r := NewReader(rdb)

	// Read com fn sem erro: vê o dado commitado pelo escritor.
	var n int
	if err := r.Read(ctx, func(tx *sql.Tx) error {
		return tx.QueryRow(`SELECT COUNT(*) FROM conformance_probe WHERE v = 'lido'`).Scan(&n)
	}); err != nil {
		t.Fatalf("Read: %v", err)
	}
	if n != 1 {
		t.Fatalf("Read viu %d linhas, want 1", n)
	}

	// Erro de fn volta ao chamador encadeado (errors.Is).
	err = r.Read(ctx, func(*sql.Tx) error { return errConformanceBoom })
	if !errors.Is(err, errConformanceBoom) {
		t.Fatalf("Read err = %v, want errConformanceBoom", err)
	}

	// Escrita dentro de Read é recusada (pool somente leitura).
	err = r.Read(ctx, func(tx *sql.Tx) error {
		_, err := tx.Exec(`INSERT INTO conformance_probe (v) VALUES ('proibido')`)
		return err
	})
	if err == nil {
		t.Fatal("escrita dentro de Read foi aceita; want erro")
	}
	var m int
	if err := write.QueryRow(`SELECT COUNT(*) FROM conformance_probe WHERE v = 'proibido'`).Scan(&m); err != nil {
		t.Fatalf("count proibido: %v", err)
	}
	if m != 0 {
		t.Fatalf("escrita recusada persistiu %d linhas", m)
	}
}
