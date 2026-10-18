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

// hcClaim reivindica pelo escritor, como o chamador real, e devolve o resultado.
func hcClaim(t *testing.T, w Writer, c HandoffClaim) HandoffClaimResult {
	t.Helper()
	var out HandoffClaimResult
	if err := w.Do(context.Background(), func(tx *sql.Tx) error {
		var err error
		out, err = ClaimHandoffTx(tx, c)
		return err
	}); err != nil {
		t.Fatalf("ClaimHandoffTx: %v", err)
	}
	return out
}

// hcClaimErr reivindica esperando erro e devolve o erro.
func hcClaimErr(t *testing.T, w Writer, c HandoffClaim) error {
	t.Helper()
	err := w.Do(context.Background(), func(tx *sql.Tx) error {
		_, e := ClaimHandoffTx(tx, c)
		return e
	})
	if err == nil {
		t.Fatalf("ClaimHandoffTx: esperado erro")
	}
	return err
}

// hcClaimFor monta a reivindicação de um receptor no cwd dado, com o corte padrão.
func hcClaimFor(scope domain.Scope, receiver domain.SessionID, cwd, actor string) HandoffClaim {
	return HandoffClaim{Scope: scope, Receiver: receiver, Cwd: cwd, Actor: actor, BusySince: hsBusy(), At: hsNow}
}

// hcTarget devolve o ponteiro de id exato para a reivindicação por id.
func hcTarget(id domain.ID) *domain.ID { return &id }

// hcAcceptedBy lê a sessão e o operador que aceitaram um handoff.
func hcAcceptedBy(t *testing.T, db *sql.DB, id domain.ID) ([]byte, string) {
	t.Helper()
	var sess []byte
	var user sql.NullString
	if err := db.QueryRow(
		`SELECT accepted_by_session, accepted_by_user FROM handoffs WHERE id = ?`, id.Bytes(),
	).Scan(&sess, &user); err != nil {
		t.Fatalf("ler aceite: %v", err)
	}
	return sess, user.String
}

// TestHandoffClaim_MultiSessaoSoUmaReivindica é o teste multi-sessão de INV-16: duas sessões receptoras disputam o
// mesmo handoff automático ao mesmo tempo. Só uma reivindica, e a outra recebe nulo.
func TestHandoffClaim_MultiSessaoSoUmaReivindica(t *testing.T) {
	db, scope := sessSetup(t)
	w := hsWriter(t, db)

	origem := hoOpenSession(t, w, scope, 0x71, "/proj")
	hoEndSession(t, w, origem)
	hA := hsAuto(t, w, scope, origem, "/proj", "", hsNow.Add(-time.Minute))
	rX := hoOpenSession(t, w, scope, 0x72, "/proj")
	rY := hoOpenSession(t, w, scope, 0x73, "/proj")

	type disputa struct {
		receiver domain.SessionID
		res      HandoffClaimResult
		err      error
	}
	receivers := []domain.SessionID{rX, rY}
	results := make([]disputa, len(receivers))
	start := make(chan struct{})
	var wg sync.WaitGroup
	for i, r := range receivers {
		wg.Add(1)
		go func(i int, r domain.SessionID) {
			defer wg.Done()
			<-start
			var res HandoffClaimResult
			err := w.Do(context.Background(), func(tx *sql.Tx) error {
				var e error
				res, e = ClaimHandoffTx(tx, hcClaimFor(scope, r, "/proj", ""))
				return e
			})
			results[i] = disputa{receiver: r, res: res, err: err}
		}(i, r)
	}
	close(start)
	wg.Wait()

	reivindicados := 0
	var vencedor domain.SessionID
	for _, d := range results {
		if d.err != nil {
			t.Fatalf("disputa de %s: %v", d.receiver, d.err)
		}
		if d.res.Claimed == nil {
			continue
		}
		reivindicados++
		vencedor = d.receiver
		if d.res.Claimed.ID != hA {
			t.Fatalf("reivindicado %s, esperado %s", d.res.Claimed.ID, hA)
		}
	}
	if reivindicados != 1 {
		t.Fatalf("reivindicações = %d, esperado exatamente 1", reivindicados)
	}
	if got := hoState(t, db, hA); got != "accepted" {
		t.Fatalf("estado = %q, esperado accepted", got)
	}
	sess, _ := hcAcceptedBy(t, db, hA)
	if string(sess) != string(vencedor.Bytes()) {
		t.Fatalf("accepted_by_session não é o vencedor")
	}
}

// TestHandoffClaim_SessaoVivaReavaliadaNaReivindicacao confere que a sessão viva é reavaliada no momento da
// reivindicação, e não copiada de uma leitura anterior: enquanto a origem está aberta com observação recente, o
// handoff não é reivindicado; depois que ela encerra, a mesma chamada reivindica.
func TestHandoffClaim_SessaoVivaReavaliadaNaReivindicacao(t *testing.T) {
	db, scope := sessSetup(t)
	w := hsWriter(t, db)

	origem := hoOpenSession(t, w, scope, 0x74, "/proj")
	hsObserve(t, db, scope, origem, hsNow.Add(-2*time.Minute))
	hV := hsAuto(t, w, scope, origem, "/proj", "", hsNow.Add(-3*time.Minute))
	receptor := hoOpenSession(t, w, scope, 0x75, "/proj")

	if res := hcClaim(t, w, hcClaimFor(scope, receptor, "/proj", "")); res.Claimed != nil {
		t.Fatalf("reivindicou %s com a origem viva", res.Claimed.ID)
	}
	if got := hoState(t, db, hV); got != "open" {
		t.Fatalf("estado = %q, esperado open após recusa por sessão viva", got)
	}

	hoEndSession(t, w, origem)
	res := hcClaim(t, w, hcClaimFor(scope, receptor, "/proj", ""))
	if res.Claimed == nil || res.Claimed.ID != hV {
		t.Fatalf("após encerrar a origem, reivindicado = %v, esperado %s", res.Claimed, hV)
	}
}

// TestHandoffClaim_VarreduraExpiraSoCompativeisDoMesmoDono cobre a varredura pós-reivindicação pela seleção: o
// automático de cwd ancestral do receptor, de preferência menor, é expirado; o de cwd sem relação fica aberto.
func TestHandoffClaim_VarreduraExpiraSoCompativeisDoMesmoDono(t *testing.T) {
	db, scope := sessSetup(t)
	w := hsWriter(t, db)

	sA := hoOpenSession(t, w, scope, 0x76, "/proj")
	hoEndSession(t, w, sA)
	hA := hsAuto(t, w, scope, sA, "/proj", "", hsNow.Add(-3*time.Minute))

	sB := hoOpenSession(t, w, scope, 0x77, "/proj/api")
	hoEndSession(t, w, sB)
	hB := hsAuto(t, w, scope, sB, "/proj/api", "", hsNow.Add(-1*time.Minute))

	sC := hoOpenSession(t, w, scope, 0x78, "/outro")
	hoEndSession(t, w, sC)
	hC := hsAuto(t, w, scope, sC, "/outro", "", hsNow.Add(-2*time.Minute))

	receptor := hoOpenSession(t, w, scope, 0x79, "/proj/api")
	res := hcClaim(t, w, hcClaimFor(scope, receptor, "/proj/api", ""))
	if res.Claimed == nil || res.Claimed.ID != hB {
		t.Fatalf("reivindicado = %v, esperado o mais recente %s", res.Claimed, hB)
	}
	if got := hoState(t, db, hB); got != "accepted" {
		t.Fatalf("hB = %q, esperado accepted", got)
	}
	if got := hoState(t, db, hA); got != "expired" {
		t.Fatalf("hA (cwd ancestral, preferência menor) = %q, esperado expired", got)
	}
	if got := hoState(t, db, hC); got != "open" {
		t.Fatalf("hC (cwd sem relação) = %q, esperado open", got)
	}
	if res.Swept != 1 {
		t.Fatalf("varridos = %d, esperado 1", res.Swept)
	}
}

// TestHandoffClaim_VarreduraPoupaManualEVivo confere que a varredura não toca em manual nem em automático de sessão
// viva. A reivindicação usa o id exato, o caminho em que um manual aberto pode coexistir com um automático.
func TestHandoffClaim_VarreduraPoupaManualEVivo(t *testing.T) {
	db, scope := sessSetup(t)
	w := hsWriter(t, db)

	manual := hsManual(t, w, scope, "", "", hsNow.Add(-4*time.Minute))

	sVivo := hoOpenSession(t, w, scope, 0x7a, "/proj")
	hsObserve(t, db, scope, sVivo, hsNow.Add(-time.Minute))
	hVivo := hsAuto(t, w, scope, sVivo, "/proj", "", hsNow.Add(-5*time.Minute))

	sB := hoOpenSession(t, w, scope, 0x7b, "/proj")
	hoEndSession(t, w, sB)
	hB := hsAuto(t, w, scope, sB, "/proj", "", hsNow.Add(-30*time.Second))

	// Por id exato, reivindica hB (mais recente que hVivo, então hVivo teria preferência menor). Manual aberto e
	// automático de sessão viva devem ficar abertos.
	receptor := hoOpenSession(t, w, scope, 0x7d, "/proj")
	res := hcClaim(t, w, HandoffClaim{
		Scope: scope, Receiver: receptor, Cwd: "/proj", Target: hcTarget(hB),
		BusySince: hsBusy(), At: hsNow,
	})
	if res.Claimed == nil || res.Claimed.ID != hB {
		t.Fatalf("por id, reivindicado = %v, esperado %s", res.Claimed, hB)
	}
	if got := hoState(t, db, manual); got != "open" {
		t.Fatalf("manual = %q, esperado open (varredura não toca manual)", got)
	}
	if got := hoState(t, db, hVivo); got != "open" {
		t.Fatalf("automático de sessão viva = %q, esperado open", got)
	}
	if res.Swept != 0 {
		t.Fatalf("varridos = %d, esperado 0", res.Swept)
	}
}

// TestHandoffClaim_DonoDeOutroOperadorRecusa confere o filtro de dono no aceite por id e na seleção.
func TestHandoffClaim_DonoDeOutroOperadorRecusa(t *testing.T) {
	db, scope := sessSetup(t)
	w := hsWriter(t, db)

	m := hsManual(t, w, scope, "op-x", "", hsNow.Add(-time.Minute))
	receptor := hoOpenSession(t, w, scope, 0x7e, "/proj")

	if err := hcClaimErr(t, w, HandoffClaim{
		Scope: scope, Receiver: receptor, Cwd: "/proj", Actor: "op-z", Target: hcTarget(m),
		BusySince: hsBusy(), At: hsNow,
	}); !errors.Is(err, ErrHandoffOwner) {
		t.Fatalf("erro = %v, esperado ErrHandoffOwner", err)
	}
	if got := hoState(t, db, m); got != "open" {
		t.Fatalf("estado = %q, esperado open após recusa de dono", got)
	}
	if res := hcClaim(t, w, hcClaimFor(scope, receptor, "/proj", "op-z")); res.Claimed != nil {
		t.Fatalf("seleção de op-z reivindicou handoff de op-x")
	}

	res := hcClaim(t, w, hcClaimFor(scope, receptor, "/proj", "op-x"))
	if res.Claimed == nil || res.Claimed.ID != m {
		t.Fatalf("op-x reivindicado = %v, esperado %s", res.Claimed, m)
	}
	if _, user := hcAcceptedBy(t, db, m); user != "op-x" {
		t.Fatalf("accepted_by_user = %q, esperado op-x", user)
	}
}

// TestHandoffClaim_ReceptorNaoConsomeDoisBatoes confere a idempotência do receptor: uma sessão que já reivindicou
// um handoff não reivindica outro, mesmo com outro aberto no escopo.
func TestHandoffClaim_ReceptorNaoConsomeDoisBatoes(t *testing.T) {
	db, scope := sessSetup(t)
	w := hsWriter(t, db)

	m1 := hsManual(t, w, scope, "", "", hsNow.Add(-2*time.Minute))
	m2 := hsManual(t, w, scope, "", "", hsNow.Add(-time.Minute))
	receptor := hoOpenSession(t, w, scope, 0x7f, "/proj")

	first := hcClaim(t, w, hcClaimFor(scope, receptor, "/proj", ""))
	if first.Claimed == nil || first.Claimed.ID != m2 {
		t.Fatalf("primeira reivindicação = %v, esperado %s", first.Claimed, m2)
	}
	if second := hcClaim(t, w, hcClaimFor(scope, receptor, "/proj", "")); second.Claimed != nil {
		t.Fatalf("segunda reivindicação da mesma sessão = %s, esperado nenhuma", second.Claimed.ID)
	}
	if got := hoState(t, db, m1); got != "open" {
		t.Fatalf("m1 = %q, esperado open", got)
	}
}

// TestHandoffClaim_IdJaAceitoNaoReivindicaDeNovo confere a guarda de estado no aceite por id: um handoff já aceito
// não é reivindicado de novo por outra sessão, e a resposta é nula, sem erro.
func TestHandoffClaim_IdJaAceitoNaoReivindicaDeNovo(t *testing.T) {
	db, scope := sessSetup(t)
	w := hsWriter(t, db)

	m := hsManual(t, w, scope, "", "", hsNow.Add(-time.Minute))
	r1 := hoOpenSession(t, w, scope, 0x80, "/proj")
	r2 := hoOpenSession(t, w, scope, 0x81, "/proj")

	if res := hcClaim(t, w, HandoffClaim{Scope: scope, Receiver: r1, Cwd: "/proj", Target: hcTarget(m), BusySince: hsBusy(), At: hsNow}); res.Claimed == nil {
		t.Fatalf("primeira reivindicação por id não entregou o handoff")
	}
	if res := hcClaim(t, w, HandoffClaim{Scope: scope, Receiver: r2, Cwd: "/proj", Target: hcTarget(m), BusySince: hsBusy(), At: hsNow}); res.Claimed != nil {
		t.Fatalf("segunda reivindicação por id entregou o handoff de novo")
	}
	if sess, _ := hcAcceptedBy(t, db, m); string(sess) != string(r1.Bytes()) {
		t.Fatalf("accepted_by_session não é o primeiro receptor")
	}
}

// TestHandoffClaim_ReceptorEncerradoRecusa confere que uma sessão receptora já encerrada não reivindica, e o handoff
// continua aberto.
func TestHandoffClaim_ReceptorEncerradoRecusa(t *testing.T) {
	db, scope := sessSetup(t)
	w := hsWriter(t, db)

	m := hsManual(t, w, scope, "", "", hsNow.Add(-time.Minute))
	receptor := hoOpenSession(t, w, scope, 0x82, "/proj")
	hoEndSession(t, w, receptor)

	if err := hcClaimErr(t, w, hcClaimFor(scope, receptor, "/proj", "")); !errors.Is(err, ErrHandoffReceiverEnded) {
		t.Fatalf("erro = %v, esperado ErrHandoffReceiverEnded", err)
	}
	if got := hoState(t, db, m); got != "open" {
		t.Fatalf("estado = %q, esperado open", got)
	}
}
