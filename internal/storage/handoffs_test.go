package storage

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	"github.com/phm-aguiar/ai-memory-go/internal/domain"
)

// hoOpenSession abre uma sessão no escopo com o cwd dado, pelo escritor.
func hoOpenSession(t *testing.T, w Writer, scope domain.Scope, b byte, cwd string) domain.SessionID {
	t.Helper()
	id := sessID(t, b)
	sessOpenDo(t, w, SessionOpen{ID: id, Scope: scope, AgentKind: "claude-code", Cwd: cwd})
	return id
}

// hoEndSession encerra a sessão pelo escritor.
func hoEndSession(t *testing.T, w Writer, id domain.SessionID) {
	t.Helper()
	if err := w.Do(context.Background(), func(tx *sql.Tx) error {
		_, err := CloseSessionTx(tx, id, time.Time{})
		return err
	}); err != nil {
		t.Fatalf("CloseSessionTx: %v", err)
	}
}

// hoInsert grava um handoff pelo escritor e devolve o resultado.
func hoInsert(t *testing.T, w Writer, h HandoffInsert) HandoffInsertResult {
	t.Helper()
	var out HandoffInsertResult
	if err := w.Do(context.Background(), func(tx *sql.Tx) error {
		var err error
		out, err = InsertHandoffTx(tx, h)
		return err
	}); err != nil {
		t.Fatalf("InsertHandoffTx: %v", err)
	}
	return out
}

// hoInsertErr grava um handoff esperando erro e devolve o erro.
func hoInsertErr(t *testing.T, w Writer, h HandoffInsert) error {
	t.Helper()
	err := w.Do(context.Background(), func(tx *sql.Tx) error {
		_, e := InsertHandoffTx(tx, h)
		return e
	})
	if err == nil {
		t.Fatalf("InsertHandoffTx: esperado erro")
	}
	return err
}

// hoAuto monta um handoff automático com a sessão de origem dada.
func hoAuto(scope domain.Scope, from domain.SessionID, cwd, owner string) HandoffInsert {
	f := from
	return HandoffInsert{
		Scope:       scope,
		FromSession: &f,
		FromAgent:   "claude-code",
		Cwd:         cwd,
		Summary:     "resumo",
		OwnerUser:   owner,
	}
}

// hoState lê o estado de um handoff.
func hoState(t *testing.T, db *sql.DB, id domain.ID) string {
	t.Helper()
	var st string
	if err := db.QueryRow(`SELECT state FROM handoffs WHERE id = ?`, id.Bytes()).Scan(&st); err != nil {
		t.Fatalf("ler estado do handoff: %v", err)
	}
	return st
}

func TestHandoffInsert_ManualSemSessaoGravaAberto(t *testing.T) {
	db, scope := sessSetup(t)
	w, err := NewWriter(context.Background(), db)
	if err != nil {
		t.Fatalf("NewWriter: %v", err)
	}
	defer w.Close()

	res := hoInsert(t, w, HandoffInsert{Scope: scope, FromAgent: "operador", Summary: "manual", Cwd: "/x"})
	if res.Superseded != 0 {
		t.Errorf("manual: Superseded = %d, esperado 0", res.Superseded)
	}
	if st := hoState(t, db, res.ID); st != "open" {
		t.Errorf("manual: state = %q, esperado open", st)
	}
}

func TestHandoffInsert_AutomaticoExpiraEncerradoMesmoCwdEDono(t *testing.T) {
	db, scope := sessSetup(t)
	w, err := NewWriter(context.Background(), db)
	if err != nil {
		t.Fatalf("NewWriter: %v", err)
	}
	defer w.Close()

	// Anterior encerrada, mesmo cwd exato e mesmo dono: deve expirar.
	antiga := hoOpenSession(t, w, scope, 0x41, "/proj")
	hAntiga := hoInsert(t, w, hoAuto(scope, antiga, "/proj", "op"))
	hoEndSession(t, w, antiga)

	// Nova sessão (ainda aberta), que grava o próprio handoff.
	nova := hoOpenSession(t, w, scope, 0x42, "/proj")
	res := hoInsert(t, w, hoAuto(scope, nova, "/proj", "op"))

	if res.Superseded != 1 {
		t.Errorf("Superseded = %d, esperado 1", res.Superseded)
	}
	if st := hoState(t, db, hAntiga.ID); st != "expired" {
		t.Errorf("encerrada: state = %q, esperado expired", st)
	}
	if st := hoState(t, db, res.ID); st != "open" {
		t.Errorf("novo: state = %q, esperado open", st)
	}
}

func TestHandoffInsert_AutomaticoExpiraAnteriorDaPropriaSessao(t *testing.T) {
	db, scope := sessSetup(t)
	w, err := NewWriter(context.Background(), db)
	if err != nil {
		t.Fatalf("NewWriter: %v", err)
	}
	defer w.Close()

	propria := hoOpenSession(t, w, scope, 0x51, "/proj")
	anterior := hoInsert(t, w, hoAuto(scope, propria, "/proj", "op"))
	res := hoInsert(t, w, hoAuto(scope, propria, "/proj", "op"))

	if res.Superseded != 1 {
		t.Errorf("Superseded = %d, esperado 1", res.Superseded)
	}
	if st := hoState(t, db, anterior.ID); st != "expired" {
		t.Errorf("própria sessão: state = %q, esperado expired", st)
	}
	if st := hoState(t, db, res.ID); st != "open" {
		t.Errorf("novo: state = %q, esperado open", st)
	}
}

func TestHandoffInsert_PoupaSessaoAbertaAlheia(t *testing.T) {
	db, scope := sessSetup(t)
	w, err := NewWriter(context.Background(), db)
	if err != nil {
		t.Fatalf("NewWriter: %v", err)
	}
	defer w.Close()

	alheia := hoOpenSession(t, w, scope, 0x61, "/proj")
	hAlheia := hoInsert(t, w, hoAuto(scope, alheia, "/proj", "op"))

	outra := hoOpenSession(t, w, scope, 0x62, "/proj")
	res := hoInsert(t, w, hoAuto(scope, outra, "/proj", "op"))

	if res.Superseded != 0 {
		t.Errorf("Superseded = %d, esperado 0", res.Superseded)
	}
	if st := hoState(t, db, hAlheia.ID); st != "open" {
		t.Errorf("sessão aberta alheia: state = %q, esperado open", st)
	}
}

func TestHandoffInsert_PoupaCwdDiferente(t *testing.T) {
	db, scope := sessSetup(t)
	w, err := NewWriter(context.Background(), db)
	if err != nil {
		t.Fatalf("NewWriter: %v", err)
	}
	defer w.Close()

	// Cwd é ancestral/descendente, mas não exatamente igual: não expira (só igualdade).
	origem := hoOpenSession(t, w, scope, 0x71, "/proj/sub")
	hOutro := hoInsert(t, w, hoAuto(scope, origem, "/proj/sub", "op"))
	hoEndSession(t, w, origem)

	nova := hoOpenSession(t, w, scope, 0x72, "/proj")
	res := hoInsert(t, w, hoAuto(scope, nova, "/proj", "op"))

	if res.Superseded != 0 {
		t.Errorf("Superseded = %d, esperado 0", res.Superseded)
	}
	if st := hoState(t, db, hOutro.ID); st != "open" {
		t.Errorf("cwd diferente: state = %q, esperado open", st)
	}
}

func TestHandoffInsert_PoupaManual(t *testing.T) {
	db, scope := sessSetup(t)
	w, err := NewWriter(context.Background(), db)
	if err != nil {
		t.Fatalf("NewWriter: %v", err)
	}
	defer w.Close()

	manual := hoInsert(t, w, HandoffInsert{Scope: scope, FromAgent: "operador", Summary: "m", Cwd: "/proj", OwnerUser: "op"})

	nova := hoOpenSession(t, w, scope, 0x81, "/proj")
	res := hoInsert(t, w, hoAuto(scope, nova, "/proj", "op"))

	if res.Superseded != 0 {
		t.Errorf("Superseded = %d, esperado 0", res.Superseded)
	}
	if st := hoState(t, db, manual.ID); st != "open" {
		t.Errorf("manual: state = %q, esperado open", st)
	}
}

func TestHandoffInsert_PoupaDonoDiferente(t *testing.T) {
	db, scope := sessSetup(t)
	w, err := NewWriter(context.Background(), db)
	if err != nil {
		t.Fatalf("NewWriter: %v", err)
	}
	defer w.Close()

	alheio := hoOpenSession(t, w, scope, 0x91, "/proj")
	hOutroDono := hoInsert(t, w, hoAuto(scope, alheio, "/proj", "outro"))
	hoEndSession(t, w, alheio)

	nova := hoOpenSession(t, w, scope, 0x92, "/proj")
	res := hoInsert(t, w, hoAuto(scope, nova, "/proj", "op"))

	if res.Superseded != 0 {
		t.Errorf("Superseded = %d, esperado 0", res.Superseded)
	}
	if st := hoState(t, db, hOutroDono.ID); st != "open" {
		t.Errorf("dono diferente: state = %q, esperado open", st)
	}
}

func TestHandoffInsert_SessaoInexistenteEForaDoEscopo(t *testing.T) {
	db, scope := sessSetup(t)
	w, err := NewWriter(context.Background(), db)
	if err != nil {
		t.Fatalf("NewWriter: %v", err)
	}
	defer w.Close()

	err = hoInsertErr(t, w, hoAuto(scope, sessID(t, 0xA1), "/proj", "op"))
	if !errors.Is(err, ErrSessionNotFound) {
		t.Errorf("sessão inexistente: erro = %v, esperado ErrSessionNotFound", err)
	}
}

func TestHandoffCancel_OpenVirarExpired(t *testing.T) {
	db, scope := sessSetup(t)
	w, err := NewWriter(context.Background(), db)
	if err != nil {
		t.Fatalf("NewWriter: %v", err)
	}
	defer w.Close()

	res := hoInsert(t, w, HandoffInsert{Scope: scope, FromAgent: "operador", Summary: "m"})

	var out HandoffCancel
	if err := w.Do(context.Background(), func(tx *sql.Tx) error {
		var e error
		out, e = CancelHandoffTx(tx, scope, res.ID)
		return e
	}); err != nil {
		t.Fatalf("CancelHandoffTx: %v", err)
	}
	if !out.Cancelled {
		t.Errorf("open: Cancelled = false, esperado true")
	}
	if st := hoState(t, db, res.ID); st != "expired" {
		t.Errorf("open: state = %q, esperado expired", st)
	}
}

func TestHandoffCancel_JaExpiradoOuInexistenteNaoErra(t *testing.T) {
	db, scope := sessSetup(t)
	w, err := NewWriter(context.Background(), db)
	if err != nil {
		t.Fatalf("NewWriter: %v", err)
	}
	defer w.Close()

	res := hoInsert(t, w, HandoffInsert{Scope: scope, FromAgent: "operador", Summary: "m"})
	cancel := func(id domain.ID) HandoffCancel {
		t.Helper()
		var out HandoffCancel
		if err := w.Do(context.Background(), func(tx *sql.Tx) error {
			var e error
			out, e = CancelHandoffTx(tx, scope, id)
			return e
		}); err != nil {
			t.Fatalf("CancelHandoffTx: %v", err)
		}
		return out
	}

	if out := cancel(res.ID); !out.Cancelled {
		t.Fatalf("primeiro cancelamento: Cancelled = false, esperado true")
	}
	if out := cancel(res.ID); out.Cancelled {
		t.Errorf("já expired: Cancelled = true, esperado false")
	}
	if st := hoState(t, db, res.ID); st != "expired" {
		t.Errorf("já expired: state = %q, esperado expired", st)
	}

	inexistente, err := domain.NewID()
	if err != nil {
		t.Fatalf("NewID: %v", err)
	}
	if out := cancel(inexistente); out.Cancelled {
		t.Errorf("inexistente: Cancelled = true, esperado false")
	}
}
