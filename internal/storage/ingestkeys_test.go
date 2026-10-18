package storage

import (
	"context"
	"database/sql"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/phm-aguiar/ai-memory-go/internal/domain"
)

// ikProject devolve o ProjectID de fixture com o byte dado (proj1 = 0x11, proj2 = 0x12).
func ikProject(t *testing.T, b byte) domain.ProjectID {
	t.Helper()
	id, err := domain.ParseIDBytes(schemaID(b))
	if err != nil {
		t.Fatalf("ParseIDBytes: %v", err)
	}
	return domain.ProjectID(id)
}

// ikWriter abre o escritor real sobre o banco de teste.
func ikWriter(t *testing.T, db *sql.DB) Writer {
	t.Helper()
	w, err := NewWriter(context.Background(), db)
	if err != nil {
		t.Fatalf("NewWriter: %v", err)
	}
	t.Cleanup(func() { _ = w.Close() })
	return w
}

// ikRow é a linha de uma chave, ou exists=false quando ela não existe.
type ikRow struct {
	exists      bool
	seenAt      int64
	completedAt sql.NullInt64
}

func ikRead(t *testing.T, db *sql.DB, p domain.ProjectID, key string) ikRow {
	t.Helper()
	var r ikRow
	err := db.QueryRow(
		`SELECT seen_at, completed_at FROM ingest_keys WHERE project_id = ? AND key = ?`,
		p.Bytes(), key,
	).Scan(&r.seenAt, &r.completedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return ikRow{}
	}
	if err != nil {
		t.Fatalf("ler chave: %v", err)
	}
	r.exists = true
	return r
}

// ikClaimDo reivindica pelo escritor, como o chamador real, e devolve o resultado.
func ikClaimDo(t *testing.T, w Writer, p domain.ProjectID, key string, at time.Time) bool {
	t.Helper()
	var got bool
	err := w.Do(context.Background(), func(tx *sql.Tx) error {
		var e error
		got, e = ClaimIngestKeyTx(tx, p, key, at)
		return e
	})
	if err != nil {
		t.Fatalf("ClaimIngestKeyTx: %v", err)
	}
	return got
}

// ikCompleteDo conclui pelo escritor e devolve o resultado e o erro da transação.
func ikCompleteDo(t *testing.T, w Writer, p domain.ProjectID, key string, at time.Time) (bool, error) {
	t.Helper()
	var got bool
	err := w.Do(context.Background(), func(tx *sql.Tx) error {
		var e error
		got, e = CompleteIngestKeyTx(tx, p, key, at)
		return e
	})
	return got, err
}

// --- reivindicação: new -> pending ---

func TestIngestKeyClaim_NewBecomesPending(t *testing.T) {
	db, _ := sessSetup(t)
	w := ikWriter(t, db)
	p := ikProject(t, 0x11)
	at := time.Unix(schemaTS, 0).UTC()

	if got := ikClaimDo(t, w, p, "evt-1", at); !got {
		t.Fatalf("reivindicar chave nova: esperado true")
	}
	r := ikRead(t, db, p, "evt-1")
	if !r.exists {
		t.Fatalf("linha não criada")
	}
	if r.completedAt.Valid {
		t.Errorf("completed_at = %d, esperado NULL (pending)", r.completedAt.Int64)
	}
	if r.seenAt != at.UnixMicro() {
		t.Errorf("seen_at = %d, esperado %d", r.seenAt, at.UnixMicro())
	}
}

func TestIngestKeyClaim_PendingIsNotClaimedAgain(t *testing.T) {
	db, _ := sessSetup(t)
	w := ikWriter(t, db)
	p := ikProject(t, 0x11)
	first := time.Unix(schemaTS, 0).UTC()

	ikClaimDo(t, w, p, "evt-1", first)
	if got := ikClaimDo(t, w, p, "evt-1", first.Add(time.Hour)); got {
		t.Fatalf("chave pending reivindicada de novo: esperado false")
	}
	r := ikRead(t, db, p, "evt-1")
	if r.completedAt.Valid {
		t.Errorf("reivindicação repetida não deve concluir a chave")
	}
	if r.seenAt != first.UnixMicro() {
		t.Errorf("seen_at alterado por reivindicação repetida: %d", r.seenAt)
	}
}

func TestIngestKeyClaim_CompletedIsNotClaimedAgain(t *testing.T) {
	db, _ := sessSetup(t)
	w := ikWriter(t, db)
	p := ikProject(t, 0x11)
	at := time.Unix(schemaTS, 0).UTC()

	ikClaimDo(t, w, p, "evt-1", at)
	if _, err := ikCompleteDo(t, w, p, "evt-1", at); err != nil {
		t.Fatalf("concluir: %v", err)
	}
	before := ikRead(t, db, p, "evt-1")

	if got := ikClaimDo(t, w, p, "evt-1", at.Add(time.Hour)); got {
		t.Fatalf("chave completed reivindicada de novo: esperado false")
	}
	after := ikRead(t, db, p, "evt-1")
	if !after.completedAt.Valid || after.completedAt != before.completedAt {
		t.Errorf("completed_at alterado: antes %+v, depois %+v", before.completedAt, after.completedAt)
	}
}

func TestIngestKeyClaim_ScopedByProject(t *testing.T) {
	db, _ := sessSetup(t)
	w := ikWriter(t, db)
	at := time.Unix(schemaTS, 0).UTC()

	if !ikClaimDo(t, w, ikProject(t, 0x11), "evt-1", at) {
		t.Fatalf("proj1: esperado true")
	}
	if !ikClaimDo(t, w, ikProject(t, 0x12), "evt-1", at) {
		t.Fatalf("mesma chave em proj2 deve ser nova: esperado true")
	}
}

// Reivindicações concorrentes da mesma chave: o escritor serializa, e exatamente uma vence.
func TestIngestKeyClaim_ConcurrentOnlyOneWins(t *testing.T) {
	db, _ := sessSetup(t)
	w := ikWriter(t, db)
	p := ikProject(t, 0x11)
	at := time.Unix(schemaTS, 0).UTC()

	const n = 16
	var (
		wg   sync.WaitGroup
		mu   sync.Mutex
		wins int
		errs []error
	)
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			err := w.Do(context.Background(), func(tx *sql.Tx) error {
				got, e := ClaimIngestKeyTx(tx, p, "evt-race", at)
				if e != nil {
					return e
				}
				if got {
					mu.Lock()
					wins++
					mu.Unlock()
				}
				return nil
			})
			if err != nil {
				mu.Lock()
				errs = append(errs, err)
				mu.Unlock()
			}
		}()
	}
	wg.Wait()

	if len(errs) > 0 {
		t.Fatalf("erros em reivindicação concorrente: %v", errs[0])
	}
	if wins != 1 {
		t.Fatalf("vitórias = %d, esperado exatamente 1", wins)
	}
	if got := countSchemaRows(t, db, `SELECT COUNT(*) FROM ingest_keys WHERE project_id = ? AND key = ?`, p.Bytes(), "evt-race"); got != 1 {
		t.Errorf("linhas = %d, esperado 1", got)
	}
}

func TestIngestKeyClaim_RejectsEmptyKey(t *testing.T) {
	db, _ := sessSetup(t)
	w := ikWriter(t, db)
	err := w.Do(context.Background(), func(tx *sql.Tx) error {
		_, e := ClaimIngestKeyTx(tx, ikProject(t, 0x11), "", time.Unix(schemaTS, 0))
		return e
	})
	if err == nil {
		t.Fatalf("chave vazia: esperado erro")
	}
}

// --- conclusão: pending -> completed ---

func TestIngestKeyComplete_PendingBecomesCompleted(t *testing.T) {
	db, _ := sessSetup(t)
	w := ikWriter(t, db)
	p := ikProject(t, 0x11)
	claimAt := time.Unix(schemaTS, 0).UTC()
	doneAt := claimAt.Add(5 * time.Second)

	ikClaimDo(t, w, p, "evt-1", claimAt)
	got, err := ikCompleteDo(t, w, p, "evt-1", doneAt)
	if err != nil {
		t.Fatalf("concluir: %v", err)
	}
	if !got {
		t.Fatalf("pending -> completed: esperado true")
	}
	r := ikRead(t, db, p, "evt-1")
	if !r.completedAt.Valid || r.completedAt.Int64 != doneAt.UnixMicro() {
		t.Errorf("completed_at = %+v, esperado %d", r.completedAt, doneAt.UnixMicro())
	}
	if r.seenAt != claimAt.UnixMicro() {
		t.Errorf("seen_at alterado pela conclusão: %d", r.seenAt)
	}
}

func TestIngestKeyComplete_AlreadyCompletedIsNoop(t *testing.T) {
	db, _ := sessSetup(t)
	w := ikWriter(t, db)
	p := ikProject(t, 0x11)
	at := time.Unix(schemaTS, 0).UTC()

	ikClaimDo(t, w, p, "evt-1", at)
	if _, err := ikCompleteDo(t, w, p, "evt-1", at); err != nil {
		t.Fatalf("primeira conclusão: %v", err)
	}
	first := ikRead(t, db, p, "evt-1")

	got, err := ikCompleteDo(t, w, p, "evt-1", at.Add(time.Minute))
	if err != nil {
		t.Fatalf("segunda conclusão: %v", err)
	}
	if got {
		t.Errorf("segunda conclusão: esperado false (no-op)")
	}
	if after := ikRead(t, db, p, "evt-1"); after.completedAt != first.completedAt {
		t.Errorf("completed_at alterado na segunda conclusão")
	}
}

func TestIngestKeyComplete_NewKeyIsRejected(t *testing.T) {
	db, _ := sessSetup(t)
	w := ikWriter(t, db)
	p := ikProject(t, 0x11)

	_, err := ikCompleteDo(t, w, p, "nunca-reivindicada", time.Unix(schemaTS, 0))
	if !errors.Is(err, ErrIngestKeyNotClaimed) {
		t.Fatalf("erro = %v, esperado ErrIngestKeyNotClaimed", err)
	}
	if r := ikRead(t, db, p, "nunca-reivindicada"); r.exists {
		t.Errorf("concluir chave nova não deve criar linha")
	}
}

// --- estado: leitura new / pending / completed ---

func ikStateDo(t *testing.T, w Writer, p domain.ProjectID, key string) IngestKeyState {
	t.Helper()
	var got IngestKeyState
	err := w.Do(context.Background(), func(tx *sql.Tx) error {
		var e error
		got, e = IngestKeyStateTx(tx, p, key)
		return e
	})
	if err != nil {
		t.Fatalf("IngestKeyStateTx: %v", err)
	}
	return got
}

func TestIngestKeyState_MissingIsNew(t *testing.T) {
	db, _ := sessSetup(t)
	w := ikWriter(t, db)
	p := ikProject(t, 0x11)

	if got := ikStateDo(t, w, p, "nunca-vista"); got != IngestKeyNew {
		t.Fatalf("chave inexistente: estado = %v, esperado new", got)
	}
	if r := ikRead(t, db, p, "nunca-vista"); r.exists {
		t.Errorf("leitura de estado não deve criar linha")
	}
}

func TestIngestKeyState_ClaimedWithPendingEffectsIsPending(t *testing.T) {
	db, _ := sessSetup(t)
	w := ikWriter(t, db)
	p := ikProject(t, 0x11)
	at := time.Unix(schemaTS, 0).UTC()

	ikClaimDo(t, w, p, "evt-1", at)
	if got := ikStateDo(t, w, p, "evt-1"); got != IngestKeyPending {
		t.Fatalf("reivindicada sem conclusão: estado = %v, esperado pending", got)
	}
	// Leitura na mesma transação da reivindicação: a linha recém-criada já aparece como pending.
	err := w.Do(context.Background(), func(tx *sql.Tx) error {
		if _, e := ClaimIngestKeyTx(tx, p, "evt-2", at); e != nil {
			return e
		}
		got, e := IngestKeyStateTx(tx, p, "evt-2")
		if e != nil {
			return e
		}
		if got != IngestKeyPending {
			return errors.New("mesma transação: estado = " + got.String() + ", esperado pending")
		}
		return nil
	})
	if err != nil {
		t.Fatalf("reivindicar e ler na mesma transação: %v", err)
	}
	if r := ikRead(t, db, p, "evt-1"); r.completedAt.Valid {
		t.Errorf("leitura de estado não deve concluir a chave")
	}
}

func TestIngestKeyState_CompletedIsCompleted(t *testing.T) {
	db, _ := sessSetup(t)
	w := ikWriter(t, db)
	p := ikProject(t, 0x11)
	at := time.Unix(schemaTS, 0).UTC()

	ikClaimDo(t, w, p, "evt-1", at)
	if _, err := ikCompleteDo(t, w, p, "evt-1", at); err != nil {
		t.Fatalf("concluir: %v", err)
	}
	if got := ikStateDo(t, w, p, "evt-1"); got != IngestKeyCompleted {
		t.Fatalf("concluída: estado = %v, esperado completed", got)
	}
}

func TestIngestKeyState_ScopedByProject(t *testing.T) {
	db, _ := sessSetup(t)
	w := ikWriter(t, db)
	at := time.Unix(schemaTS, 0).UTC()

	ikClaimDo(t, w, ikProject(t, 0x11), "evt-1", at)
	if got := ikStateDo(t, w, ikProject(t, 0x12), "evt-1"); got != IngestKeyNew {
		t.Fatalf("mesma chave em outro projeto: estado = %v, esperado new", got)
	}
}

func TestIngestKeyState_RejectsNilTxAndEmptyKey(t *testing.T) {
	if _, err := IngestKeyStateTx(nil, domain.ProjectID{}, "evt-1"); err == nil {
		t.Errorf("transação nula: esperado erro")
	}
	db, _ := sessSetup(t)
	w := ikWriter(t, db)
	err := w.Do(context.Background(), func(tx *sql.Tx) error {
		_, e := IngestKeyStateTx(tx, ikProject(t, 0x11), "")
		return e
	})
	if err == nil {
		t.Fatalf("chave vazia: esperado erro")
	}
}

// --- retenção: limpeza de completed com mais de 30 dias ---

func TestIngestKeyPrune_RetentionWindow(t *testing.T) {
	db, _ := sessSetup(t)
	w := ikWriter(t, db)
	p := ikProject(t, 0x11)
	t0 := time.Unix(schemaTS, 0).UTC()

	// A: completa em t0 (fica velha). P: pending em t0 (nunca é removida).
	// B: completa uma hora depois (ainda dentro da janela na última limpeza).
	ikClaimDo(t, w, p, "old-completed", t0)
	if _, err := ikCompleteDo(t, w, p, "old-completed", t0); err != nil {
		t.Fatalf("concluir A: %v", err)
	}
	ikClaimDo(t, w, p, "old-pending", t0)
	ikClaimDo(t, w, p, "recent-completed", t0.Add(time.Hour))
	if _, err := ikCompleteDo(t, w, p, "recent-completed", t0.Add(time.Hour)); err != nil {
		t.Fatalf("concluir B: %v", err)
	}

	prune := func(now time.Time) int64 {
		t.Helper()
		var n int64
		err := w.Do(context.Background(), func(tx *sql.Tx) error {
			var e error
			n, e = PruneIngestKeysTx(tx, now)
			return e
		})
		if err != nil {
			t.Fatalf("PruneIngestKeysTx: %v", err)
		}
		return n
	}

	// Dentro da janela: nada é removido.
	if n := prune(t0.Add(IngestKeyRetention - time.Second)); n != 0 {
		t.Errorf("29d23h59m59s: removidas = %d, esperado 0", n)
	}
	// Exatamente 30 dias: ainda não é "mais de 30 dias".
	if n := prune(t0.Add(IngestKeyRetention)); n != 0 {
		t.Errorf("30d exatos: removidas = %d, esperado 0", n)
	}
	if !ikRead(t, db, p, "old-completed").exists {
		t.Fatalf("chave concluída removida antes de 30 dias")
	}

	// Fora da janela: só a concluída antiga sai.
	if n := prune(t0.Add(IngestKeyRetention + time.Second)); n != 1 {
		t.Errorf("30d + 1s: removidas = %d, esperado 1", n)
	}
	if ikRead(t, db, p, "old-completed").exists {
		t.Errorf("concluída com mais de 30 dias deveria ter sido removida")
	}
	if !ikRead(t, db, p, "recent-completed").exists {
		t.Errorf("concluída dentro da janela não deveria ser removida")
	}
	if !ikRead(t, db, p, "old-pending").exists {
		t.Errorf("chave pending nunca deve ser removida pela limpeza")
	}
}

func TestIngestKeyPrune_ClockIsParameter(t *testing.T) {
	db, _ := sessSetup(t)
	w := ikWriter(t, db)
	p := ikProject(t, 0x11)
	t0 := time.Unix(schemaTS, 0).UTC()

	ikClaimDo(t, w, p, "evt-1", t0)
	if _, err := ikCompleteDo(t, w, p, "evt-1", t0); err != nil {
		t.Fatalf("concluir: %v", err)
	}

	// O mesmo banco, relógios diferentes: a decisão depende só do parâmetro.
	var n int64
	err := w.Do(context.Background(), func(tx *sql.Tx) error {
		var e error
		n, e = PruneIngestKeysTx(tx, t0.Add(time.Hour))
		return e
	})
	if err != nil || n != 0 {
		t.Fatalf("relógio em t0+1h: removidas = %d, err = %v; esperado 0", n, err)
	}
	err = w.Do(context.Background(), func(tx *sql.Tx) error {
		var e error
		n, e = PruneIngestKeysTx(tx, t0.Add(IngestKeyRetention+time.Hour))
		return e
	})
	if err != nil || n != 1 {
		t.Fatalf("relógio em t0+30d+1h: removidas = %d, err = %v; esperado 1", n, err)
	}
}
