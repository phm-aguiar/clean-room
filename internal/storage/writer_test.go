package storage

import (
	"context"
	"database/sql"
	"errors"
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

var errWriterBoom = errors.New("boom")

// openWriterTestDB abre um banco temporário com a migração real e uma tabela de sonda.
func openWriterTestDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := Open(context.Background(), t.TempDir())
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if _, err := db.Exec(`CREATE TABLE writer_probe (id INTEGER PRIMARY KEY, v TEXT NOT NULL)`); err != nil {
		t.Fatalf("create probe table: %v", err)
	}
	return db
}

// insertProbe devolve uma closure que insere v na tabela de sonda.
func insertProbe(v string) func(*sql.Tx) error {
	return func(tx *sql.Tx) error {
		_, err := tx.Exec(`INSERT INTO writer_probe (v) VALUES (?)`, v)
		return err
	}
}

func countProbe(t *testing.T, db *sql.DB, v string) int {
	t.Helper()
	var n int
	if err := db.QueryRow(`SELECT COUNT(*) FROM writer_probe WHERE v = ?`, v).Scan(&n); err != nil {
		t.Fatalf("count probe: %v", err)
	}
	return n
}

// waitUntil espera cond ficar verdadeira ou falha após 5 s.
func waitUntil(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatalf("timeout esperando %s", what)
		}
		time.Sleep(time.Millisecond)
	}
}

// gateWriter ocupa o ator com um comando que só termina quando a função devolvida for chamada.
// O comando é somado a wg. Retorna só depois de o comando ter começado a executar.
func gateWriter(t *testing.T, w Writer, wg *sync.WaitGroup) (release func()) {
	t.Helper()
	started := make(chan struct{})
	gate := make(chan struct{})
	wg.Add(1)
	go func() {
		defer wg.Done()
		_ = w.Do(context.Background(), func(*sql.Tx) error {
			close(started)
			<-gate
			return nil
		})
	}()
	select {
	case <-started:
	case <-time.After(5 * time.Second):
		t.Fatal("comando de bloqueio não começou")
	}
	var once sync.Once
	return func() { once.Do(func() { close(gate) }) }
}

func TestWriterTransacaoEscreveEFicaVisivel(t *testing.T) {
	db := openWriterTestDB(t)
	w, err := NewWriter(context.Background(), db)
	if err != nil {
		t.Fatalf("NewWriter: %v", err)
	}
	t.Cleanup(func() { _ = w.Close() })

	if err := w.Do(context.Background(), insertProbe("ok")); err != nil {
		t.Fatalf("Do: %v", err)
	}
	if n := countProbe(t, db, "ok"); n != 1 {
		t.Fatalf("linhas visíveis após commit = %d, want 1", n)
	}
}

func TestWriterErroNaClosureFazRollback(t *testing.T) {
	db := openWriterTestDB(t)
	w, err := NewWriter(context.Background(), db)
	if err != nil {
		t.Fatalf("NewWriter: %v", err)
	}
	t.Cleanup(func() { _ = w.Close() })

	err = w.Do(context.Background(), func(tx *sql.Tx) error {
		if err := insertProbe("perdida")(tx); err != nil {
			return err
		}
		return errWriterBoom
	})
	if !errors.Is(err, errWriterBoom) {
		t.Fatalf("Do err = %v, want errWriterBoom", err)
	}
	if n := countProbe(t, db, "perdida"); n != 0 {
		t.Fatalf("linha de transação com erro apareceu: %d", n)
	}

	// O ator continua vivo e a próxima transação funciona.
	if err := w.Do(context.Background(), insertProbe("depois")); err != nil {
		t.Fatalf("Do após rollback: %v", err)
	}
	if n := countProbe(t, db, "depois"); n != 1 {
		t.Fatalf("linha após rollback = %d, want 1", n)
	}
}

func TestWriterQueueLenReflete(t *testing.T) {
	db := openWriterTestDB(t)
	w, err := NewWriter(context.Background(), db)
	if err != nil {
		t.Fatalf("NewWriter: %v", err)
	}
	t.Cleanup(func() { _ = w.Close() })

	if n := w.QueueLen(); n != 0 {
		t.Fatalf("QueueLen inicial = %d, want 0", n)
	}

	var wg sync.WaitGroup
	release := gateWriter(t, w, &wg)
	t.Cleanup(func() { release(); wg.Wait() })

	const k = 3
	var ran atomic.Int64
	for i := 0; i < k; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := w.Do(context.Background(), func(*sql.Tx) error {
				ran.Add(1)
				return nil
			}); err != nil {
				t.Errorf("Do enfileirado: %v", err)
			}
		}()
	}
	waitUntil(t, "k comandos na fila", func() bool { return w.QueueLen() == k })

	release()
	wg.Wait()
	if n := w.QueueLen(); n != 0 {
		t.Fatalf("QueueLen após drenar = %d, want 0", n)
	}
	if n := ran.Load(); n != k {
		t.Fatalf("comandos executados = %d, want %d", n, k)
	}
}

func TestWriterFilaCheiaBloqueiaAteCtxCancelar(t *testing.T) {
	db := openWriterTestDB(t)
	w, err := NewWriter(context.Background(), db)
	if err != nil {
		t.Fatalf("NewWriter: %v", err)
	}
	t.Cleanup(func() { _ = w.Close() })

	var wg sync.WaitGroup
	release := gateWriter(t, w, &wg)
	t.Cleanup(func() { release(); wg.Wait() })

	// Enche a fila: o ator está parado no comando de bloqueio, então nada sai da fila.
	var ran atomic.Int64
	for i := 0; i < queueCap; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := w.Do(context.Background(), func(*sql.Tx) error {
				ran.Add(1)
				return nil
			}); err != nil {
				t.Errorf("Do enfileirado: %v", err)
			}
		}()
	}
	waitUntil(t, "fila cheia", func() bool { return w.QueueLen() == queueCap })

	// Chamador extra: tem que bloquear até o ctx expirar, sem executar a closure.
	const timeout = 150 * time.Millisecond
	cctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	var extraRan bool
	start := time.Now()
	err = w.Do(cctx, func(*sql.Tx) error {
		extraRan = true
		return nil
	})
	elapsed := time.Since(start)

	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("Do com fila cheia err = %v, want DeadlineExceeded", err)
	}
	if elapsed < timeout*2/3 {
		t.Fatalf("Do retornou em %v, antes do ctx expirar: não bloqueou", elapsed)
	}
	if extraRan {
		t.Fatal("closure do chamador bloqueado foi executada")
	}
	if n := w.QueueLen(); n != queueCap {
		t.Fatalf("QueueLen = %d, want %d (chamador bloqueado não deve entrar)", n, queueCap)
	}

	release()
	wg.Wait()
	if n := ran.Load(); n != queueCap {
		t.Fatalf("comandos enfileirados executados = %d, want %d", n, queueCap)
	}
}

func TestWriterCloseExecutaFilaJaAceita(t *testing.T) {
	db := openWriterTestDB(t)
	w, err := NewWriter(context.Background(), db)
	if err != nil {
		t.Fatalf("NewWriter: %v", err)
	}

	var wg sync.WaitGroup
	release := gateWriter(t, w, &wg)
	t.Cleanup(func() { release(); wg.Wait() })

	const k = 2
	errs := make(chan error, k)
	for i := 0; i < k; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			errs <- w.Do(context.Background(), insertProbe("aceito"))
		}()
	}
	waitUntil(t, "comandos aceitos na fila", func() bool { return w.QueueLen() == k })

	closeErr := make(chan error, 1)
	go func() { closeErr <- w.Close() }()
	release()

	if err := <-closeErr; err != nil {
		t.Fatalf("Close: %v", err)
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Errorf("Do aceito antes do Close: %v", err)
		}
	}
	if n := countProbe(t, db, "aceito"); n != k {
		t.Fatalf("linhas gravadas por comandos aceitos = %d, want %d", n, k)
	}
}

func TestWriterCloseIdempotente(t *testing.T) {
	db := openWriterTestDB(t)
	w, err := NewWriter(context.Background(), db)
	if err != nil {
		t.Fatalf("NewWriter: %v", err)
	}
	if err := w.Close(); err != nil {
		t.Fatalf("primeiro Close: %v", err)
	}
	if err := w.Close(); err != nil {
		t.Fatalf("segundo Close: %v", err)
	}
}

func TestWriterComandoAposCloseDevolveErrWriterClosed(t *testing.T) {
	db := openWriterTestDB(t)
	w, err := NewWriter(context.Background(), db)
	if err != nil {
		t.Fatalf("NewWriter: %v", err)
	}
	if err := w.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	var ran bool
	err = w.Do(context.Background(), func(*sql.Tx) error {
		ran = true
		return nil
	})
	if !errors.Is(err, ErrWriterClosed) {
		t.Fatalf("Do após Close err = %v, want ErrWriterClosed", err)
	}
	if ran {
		t.Fatal("closure executada após Close")
	}
}

func TestWriterSemVazamentoDeGoroutine(t *testing.T) {
	db := openWriterTestDB(t)
	ctx := context.Background()

	// Linha de base depois de abrir o banco (o *sql.DB tem as próprias goroutines).
	const tolerance = 2
	before := runtime.NumGoroutine()

	for i := 0; i < 5; i++ {
		w, err := NewWriter(ctx, db)
		if err != nil {
			t.Fatalf("NewWriter: %v", err)
		}
		if err := w.Do(ctx, insertProbe("leak")); err != nil {
			t.Fatalf("Do: %v", err)
		}
		if err := w.Close(); err != nil {
			t.Fatalf("Close: %v", err)
		}
	}

	deadline := time.Now().Add(5 * time.Second)
	var after int
	for {
		after = runtime.NumGoroutine()
		if after <= before+tolerance {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("goroutines antes=%d depois=%d (tolerância %d): vazamento", before, after, tolerance)
		}
		time.Sleep(10 * time.Millisecond)
	}
}
