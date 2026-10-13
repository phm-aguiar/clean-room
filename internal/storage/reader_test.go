package storage

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"
)

var errReaderBoom = errors.New("boom leitura")

// openReaderTestEnv abre o banco real (Open) em dir, cria a tabela de sonda e
// devolve o pool de escrita, o pool somente leitura e o Reader sobre ele.
func openReaderTestEnv(t *testing.T) (dir string, write *sql.DB, rdb *sql.DB, r Reader) {
	t.Helper()
	dir = t.TempDir()
	write, err := Open(context.Background(), dir)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = write.Close() })
	if _, err := write.Exec(`CREATE TABLE reader_probe (id INTEGER PRIMARY KEY, v TEXT NOT NULL)`); err != nil {
		t.Fatalf("create probe table: %v", err)
	}
	rdb, err = OpenReader(context.Background(), dir)
	if err != nil {
		t.Fatalf("OpenReader: %v", err)
	}
	t.Cleanup(func() { _ = rdb.Close() })
	return dir, write, rdb, NewReader(rdb)
}

func readerCount(t *testing.T, r Reader, v string) int {
	t.Helper()
	var n int
	err := r.Read(context.Background(), func(tx *sql.Tx) error {
		return tx.QueryRow(`SELECT COUNT(*) FROM reader_probe WHERE v = ?`, v).Scan(&n)
	})
	if err != nil {
		t.Fatalf("Read count: %v", err)
	}
	return n
}

func TestReaderReadsCommittedData(t *testing.T) {
	_, write, _, r := openReaderTestEnv(t)

	w, err := NewWriter(context.Background(), write)
	if err != nil {
		t.Fatalf("NewWriter: %v", err)
	}
	defer func() { _ = w.Close() }()
	if err := w.Do(context.Background(), func(tx *sql.Tx) error {
		_, err := tx.Exec(`INSERT INTO reader_probe (v) VALUES ('visivel')`)
		return err
	}); err != nil {
		t.Fatalf("Do insert: %v", err)
	}

	if n := readerCount(t, r, "visivel"); n != 1 {
		t.Fatalf("leitura viu %d linhas commitadas, want 1", n)
	}
}

func TestReaderRejectsWrite(t *testing.T) {
	_, _, _, r := openReaderTestEnv(t)

	err := r.Read(context.Background(), func(tx *sql.Tx) error {
		_, err := tx.Exec(`INSERT INTO reader_probe (v) VALUES ('proibido')`)
		return err
	})
	if err == nil {
		t.Fatal("escrita dentro de leitura foi aceita; want erro (somente leitura)")
	}
	if !strings.Contains(strings.ToLower(err.Error()), "readonly") &&
		!strings.Contains(strings.ToLower(err.Error()), "read-only") &&
		!strings.Contains(strings.ToLower(err.Error()), "query_only") {
		t.Fatalf("erro de escrita não indica somente leitura: %v", err)
	}
	if n := readerCount(t, r, "proibido"); n != 0 {
		t.Fatalf("escrita recusada mesmo assim persistiu %d linhas", n)
	}
}

func TestReaderPropagatesFnErrorWithWrap(t *testing.T) {
	_, _, _, r := openReaderTestEnv(t)

	err := r.Read(context.Background(), func(*sql.Tx) error { return errReaderBoom })
	if !errors.Is(err, errReaderBoom) {
		t.Fatalf("erro de fn não propagado com %%w: %v", err)
	}
}

func TestReaderPanicBecomesError(t *testing.T) {
	_, _, _, r := openReaderTestEnv(t)

	err := r.Read(context.Background(), func(*sql.Tx) error {
		panic("estouro na closure")
	})
	if err == nil {
		t.Fatal("pânico na closure não virou erro")
	}
	if !strings.Contains(err.Error(), "leitura: pânico") || !strings.Contains(err.Error(), "estouro na closure") {
		t.Fatalf("erro sem marca de pânico: %v", err)
	}

	// O pool continua utilizável: a conexão da transação com pânico foi devolvida.
	if n := readerCount(t, r, "depois"); n != 0 {
		t.Fatalf("leitura após pânico devolveu %d, want 0", n)
	}
}

func TestReaderConcurrentUpToFour(t *testing.T) {
	_, _, _, r := openReaderTestEnv(t)

	const n = 4
	var arrived sync.WaitGroup
	arrived.Add(n)
	release := make(chan struct{})
	allIn := make(chan struct{})
	go func() {
		arrived.Wait()
		close(allIn)
	}()

	var wg sync.WaitGroup
	errs := make(chan error, n)
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			errs <- r.Read(context.Background(), func(tx *sql.Tx) error {
				arrived.Done()
				select {
				case <-release:
				case <-time.After(5 * time.Second):
					return errors.New("timeout segurando leitura concorrente")
				}
				var c int
				return tx.QueryRow(`SELECT COUNT(*) FROM reader_probe`).Scan(&c)
			})
		}()
	}

	select {
	case <-allIn:
	case <-time.After(5 * time.Second):
		t.Fatal("4 leituras concorrentes não entraram ao mesmo tempo")
	}
	close(release)
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatalf("leitura concorrente falhou: %v", err)
		}
	}
}

func TestReaderSoftCapBlocksFifth(t *testing.T) {
	_, _, _, r := openReaderTestEnv(t)

	const n = 4
	var arrived sync.WaitGroup
	arrived.Add(n)
	hold := make(chan struct{})
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = r.Read(context.Background(), func(*sql.Tx) error {
				arrived.Done()
				<-hold
				return nil
			})
		}()
	}
	arrived.Wait()

	// Com as 4 conexões ocupadas, a quinta espera e só falha pelo ctx.
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	err := r.Read(ctx, func(*sql.Tx) error { return nil })
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("quinta leitura com cap 4 ocupado: want DeadlineExceeded, got %v", err)
	}

	close(hold)
	wg.Wait()
}

func TestReaderNotBlockedByWriter(t *testing.T) {
	_, write, _, r := openReaderTestEnv(t)

	w, err := NewWriter(context.Background(), write)
	if err != nil {
		t.Fatalf("NewWriter: %v", err)
	}
	defer func() { _ = w.Close() }()

	// O escritor abre transação, faz INSERT (adquire o lock de escrita do WAL)
	// e segura a transação até o gate ser liberado.
	started := make(chan struct{})
	gate := make(chan struct{})
	var writerWG sync.WaitGroup
	writerWG.Add(1)
	go func() {
		defer writerWG.Done()
		_ = w.Do(context.Background(), func(tx *sql.Tx) error {
			if _, err := tx.Exec(`INSERT INTO reader_probe (v) VALUES ('pendente')`); err != nil {
				return err
			}
			close(started)
			<-gate
			return nil
		})
	}()
	select {
	case <-started:
	case <-time.After(5 * time.Second):
		t.Fatal("escritor não começou a transação")
	}

	// Com o escritor segurando a transação, a leitura tem que completar e não
	// enxergar a linha ainda não commitada (snapshot).
	type result struct {
		n   int
		err error
	}
	done := make(chan result, 1)
	go func() {
		var n int
		err := r.Read(context.Background(), func(tx *sql.Tx) error {
			return tx.QueryRow(`SELECT COUNT(*) FROM reader_probe WHERE v = 'pendente'`).Scan(&n)
		})
		done <- result{n, err}
	}()
	select {
	case res := <-done:
		if res.err != nil {
			close(gate)
			writerWG.Wait()
			t.Fatalf("leitura falhou com escritor ativo: %v", res.err)
		}
		if res.n != 0 {
			close(gate)
			writerWG.Wait()
			t.Fatalf("leitura viu linha não commitada (%d)", res.n)
		}
	case <-time.After(5 * time.Second):
		close(gate)
		writerWG.Wait()
		t.Fatal("leitura bloqueou no escritor")
	}

	close(gate)
	writerWG.Wait()
	if n := readerCount(t, r, "pendente"); n != 1 {
		t.Fatalf("após commit, leitura viu %d, want 1", n)
	}
}

// Compilação: Reader precisa ser satisfeito exatamente pelo tipo concreto.
var _ Reader = (*reader)(nil)
