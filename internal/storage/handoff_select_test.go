package storage

import (
	"bytes"
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/phm-aguiar/ai-memory-go/internal/domain"
)

// hsNow é o instante de referência dos testes de seleção. O corte da sessão viva é hsNow − 10 min.
var hsNow = time.Date(2026, 10, 18, 12, 0, 0, 0, time.UTC)

func hsBusy() time.Time { return hsNow.Add(-10 * time.Minute) }

// hsWriter abre o escritor sobre o banco de teste.
func hsWriter(t *testing.T, db *sql.DB) Writer {
	t.Helper()
	w, err := NewWriter(context.Background(), db)
	if err != nil {
		t.Fatalf("NewWriter: %v", err)
	}
	t.Cleanup(func() { _ = w.Close() })
	return w
}

// hsManual grava um handoff manual com dono, cwd e instante dados.
func hsManual(t *testing.T, w Writer, scope domain.Scope, owner, cwd string, at time.Time) domain.ID {
	t.Helper()
	return hoInsert(t, w, HandoffInsert{
		Scope: scope, FromAgent: "operador", Summary: "manual " + owner, Cwd: cwd, OwnerUser: owner, At: at,
	}).ID
}

// hsAuto grava um handoff automático da sessão dada, com dono, cwd e instante.
func hsAuto(t *testing.T, w Writer, scope domain.Scope, from domain.SessionID, cwd, owner string, at time.Time) domain.ID {
	t.Helper()
	h := hoAuto(scope, from, cwd, owner)
	h.At = at
	return hoInsert(t, w, h).ID
}

// hsObserve grava uma observação da sessão com created_at dado (microssegundos, como o escritor).
func hsObserve(t *testing.T, db *sql.DB, scope domain.Scope, session domain.SessionID, at time.Time) {
	t.Helper()
	obsID, err := domain.NewID()
	if err != nil {
		t.Fatalf("NewID: %v", err)
	}
	if _, err := db.Exec(
		`INSERT INTO observations (id, session_id, workspace_id, project_id, kind, title, body, created_at)
		 VALUES (?, ?, ?, ?, 'nota', 't', 'b', ?)`,
		obsID.Bytes(), session.Bytes(), scope.WorkspaceID.Bytes(), scope.ProjectID.Bytes(), at.UnixMicro(),
	); err != nil {
		t.Fatalf("inserir observação: %v", err)
	}
}

// hsSelect chama SelectHandoff sobre o pool de leitura com o corte padrão.
func hsSelect(t *testing.T, db *sql.DB, scope domain.Scope, cwd, owner string) *HandoffRow {
	t.Helper()
	row, err := SelectHandoff(db, scope, cwd, owner, hsBusy())
	if err != nil {
		t.Fatalf("SelectHandoff: %v", err)
	}
	return row
}

// hsWantID confere que a seleção devolveu o handoff esperado.
func hsWantID(t *testing.T, row *HandoffRow, want domain.ID, rotulo string) {
	t.Helper()
	if row == nil {
		t.Fatalf("%s: nenhum candidato, esperado %s", rotulo, want)
	}
	if row.ID != want {
		t.Fatalf("%s: escolhido %s, esperado %s", rotulo, row.ID, want)
	}
}

// hsWantNone confere que não há candidato.
func hsWantNone(t *testing.T, row *HandoffRow, rotulo string) {
	t.Helper()
	if row != nil {
		t.Fatalf("%s: escolhido %s, esperado nenhum candidato", rotulo, row.ID)
	}
}

func TestHandoffSelect_DonoMultiSessao(t *testing.T) {
	db, scope := sessSetup(t)
	w := hsWriter(t, db)

	// Duas sessões de operadores diferentes e uma sem dono, todas abertas (sem supersessão).
	sX := hoOpenSession(t, w, scope, 0x51, "/proj")
	sY := hoOpenSession(t, w, scope, 0x52, "/proj")
	sN := hoOpenSession(t, w, scope, 0x53, "/proj")
	hX := hsAuto(t, w, scope, sX, "/proj", "op-x", hsNow.Add(-2*time.Minute))
	hY := hsAuto(t, w, scope, sY, "/proj", "op-y", hsNow.Add(-1*time.Minute))
	hN := hsAuto(t, w, scope, sN, "/proj", "", hsNow.Add(-3*time.Minute))

	// Operador X enxerga o seu e o sem dono, e nunca o de Y (o mais recente do projeto).
	hsWantID(t, hsSelect(t, db, scope, "/proj", "op-x"), hX, "op-x com X e Y abertos")
	// Operador Y pega o seu.
	hsWantID(t, hsSelect(t, db, scope, "/proj", "op-y"), hY, "op-y")
	// Sem ator identificado só vale o que não tem dono.
	hsWantID(t, hsSelect(t, db, scope, "/proj", ""), hN, "sem ator")
	// Operador de fora enxerga só o sem dono.
	hsWantID(t, hsSelect(t, db, scope, "/proj", "op-z"), hN, "op-z")

	// Com o de X cancelado, X cai no sem dono, não no de Y.
	if err := w.Do(context.Background(), func(tx *sql.Tx) error {
		_, err := CancelHandoffTx(tx, scope, hX)
		return err
	}); err != nil {
		t.Fatalf("CancelHandoffTx: %v", err)
	}
	hsWantID(t, hsSelect(t, db, scope, "/proj", "op-x"), hN, "op-x após cancelar o seu")
}

func TestHandoffSelect_SoAberto(t *testing.T) {
	db, scope := sessSetup(t)
	w := hsWriter(t, db)

	m := hsManual(t, w, scope, "", "", hsNow.Add(-time.Minute))
	hsWantID(t, hsSelect(t, db, scope, "", ""), m, "manual aberto")

	if err := w.Do(context.Background(), func(tx *sql.Tx) error {
		_, err := CancelHandoffTx(tx, scope, m)
		return err
	}); err != nil {
		t.Fatalf("CancelHandoffTx: %v", err)
	}
	hsWantNone(t, hsSelect(t, db, scope, "", ""), "manual expirado")

	m2 := hsManual(t, w, scope, "", "", hsNow.Add(-time.Minute))
	if _, err := db.Exec(`UPDATE handoffs SET state = 'accepted' WHERE id = ?`, m2.Bytes()); err != nil {
		t.Fatalf("marcar aceito: %v", err)
	}
	hsWantNone(t, hsSelect(t, db, scope, "", ""), "manual aceito")
}

func TestHandoffSelect_ManualAntesDeAutomatico(t *testing.T) {
	db, scope := sessSetup(t)
	w := hsWriter(t, db)

	s := hoOpenSession(t, w, scope, 0x61, "/proj")
	// Automático no mesmo cwd, mais recente que o manual.
	hsAuto(t, w, scope, s, "/proj", "", hsNow.Add(-30*time.Second))
	// Manual mais antigo, em outro cwd: ainda vence, porque manual vale em qualquer cwd.
	m := hsManual(t, w, scope, "", "/outro", hsNow.Add(-5*time.Minute))

	hsWantID(t, hsSelect(t, db, scope, "/proj", ""), m, "manual antigo contra automático novo")
}

func TestHandoffSelect_CwdAncestralPorComponente(t *testing.T) {
	cases := []struct {
		name     string
		stored   string
		receptor string
		want     bool
	}{
		{"ancestral direto", "/repo", "/repo/api", true},
		{"igual", "/repo/api", "/repo/api", true},
		{"raiz é ancestral de todo caminho", "/", "/qualquer/lugar", true},
		{"descendente do receptor não vale", "/repo/api", "/repo", false},
		{"prefixo de string não é componente", "/repo", "/repository", false},
		{"irmão com prefixo comum não vale", "/repo/api", "/repo/api-v2", false},
		{"cwd armazenado vazio vale para o projeto", "", "/repo/api", true},
		{"receptor sem cwd recebe qualquer automático", "/repo/api", "", true},
		{"separador invertido é normalizado", `C:\proj`, "C:/proj/sub", true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			db, scope := sessSetup(t)
			w := hsWriter(t, db)
			s := hoOpenSession(t, w, scope, 0x62, "")
			h := hsAuto(t, w, scope, s, c.stored, "", hsNow.Add(-time.Minute))

			row := hsSelect(t, db, scope, c.receptor, "")
			if got := row != nil; got != c.want {
				t.Fatalf("armazenado %q, receptor %q: candidato = %v, esperado %v", c.stored, c.receptor, got, c.want)
			}
			if c.want {
				hsWantID(t, row, h, c.name)
			}
		})
	}
}

func TestHandoffSelect_SessaoVivaDezMinutos(t *testing.T) {
	cases := []struct {
		name   string
		hasObs bool
		obsAge time.Duration
		ended  bool
		want   bool
	}{
		{"sem observação, aberta: oferecido", false, 0, false, true},
		{"observação há 5 min, aberta: protegido", true, 5 * time.Minute, false, false},
		{"observação há 15 min, aberta: fora da janela, oferecido", true, 15 * time.Minute, false, true},
		{"observação exatamente em busy_since: não é depois, oferecido", true, 10 * time.Minute, false, true},
		{"encerrada com observação recente: oferecido", true, time.Minute, true, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			db, scope := sessSetup(t)
			w := hsWriter(t, db)
			s := hoOpenSession(t, w, scope, 0x71, "")
			h := hsAuto(t, w, scope, s, "/proj", "", hsNow.Add(-30*time.Minute))
			if c.hasObs {
				hsObserve(t, db, scope, s, hsNow.Add(-c.obsAge))
			}
			if c.ended {
				hoEndSession(t, w, s)
			}

			row := hsSelect(t, db, scope, "/proj", "")
			if got := row != nil; got != c.want {
				t.Fatalf("candidato = %v, esperado %v", got, c.want)
			}
			if c.want {
				hsWantID(t, row, h, c.name)
			}
		})
	}
}

func TestHandoffSelect_SessaoVivaNaoAfetaManual(t *testing.T) {
	db, scope := sessSetup(t)
	w := hsWriter(t, db)

	s := hoOpenSession(t, w, scope, 0x72, "/proj")
	hsAuto(t, w, scope, s, "/proj", "", hsNow.Add(-time.Minute))
	hsObserve(t, db, scope, s, hsNow.Add(-time.Minute))
	hsWantNone(t, hsSelect(t, db, scope, "/proj", ""), "só automático vivo")

	m := hsManual(t, w, scope, "", "", hsNow.Add(-2*time.Hour))
	hsWantID(t, hsSelect(t, db, scope, "/proj", ""), m, "manual com automático vivo")
}

func TestHandoffSelect_Desempate(t *testing.T) {
	t.Run("mesmo instante: cwd mais longo vence", func(t *testing.T) {
		db, scope := sessSetup(t)
		w := hsWriter(t, db)
		s1 := hoOpenSession(t, w, scope, 0x81, "")
		s2 := hoOpenSession(t, w, scope, 0x82, "")
		at := hsNow.Add(-time.Minute)
		hsAuto(t, w, scope, s1, "/repo", "", at)
		long := hsAuto(t, w, scope, s2, "/repo/api", "", at)
		hsWantID(t, hsSelect(t, db, scope, "/repo/api/x", ""), long, "cwd mais longo no empate")
	})

	t.Run("instante mais recente vence cwd mais longo", func(t *testing.T) {
		db, scope := sessSetup(t)
		w := hsWriter(t, db)
		s1 := hoOpenSession(t, w, scope, 0x83, "")
		s2 := hoOpenSession(t, w, scope, 0x84, "")
		hsAuto(t, w, scope, s1, "/repo/api", "", hsNow.Add(-2*time.Minute))
		recente := hsAuto(t, w, scope, s2, "/repo", "", hsNow.Add(-time.Minute))
		hsWantID(t, hsSelect(t, db, scope, "/repo/api/x", ""), recente, "mais recente")
	})

	t.Run("mesmo instante e mesmo cwd: id ascendente", func(t *testing.T) {
		db, scope := sessSetup(t)
		w := hsWriter(t, db)
		s1 := hoOpenSession(t, w, scope, 0x85, "")
		s2 := hoOpenSession(t, w, scope, 0x86, "")
		at := hsNow.Add(-time.Minute)
		a := hsAuto(t, w, scope, s1, "/repo", "", at)
		b := hsAuto(t, w, scope, s2, "/repo", "", at)
		want := a
		if bytes.Compare(b[:], a[:]) < 0 {
			want = b
		}
		hsWantID(t, hsSelect(t, db, scope, "/repo", ""), want, "id ascendente")
	})
}

func TestHandoffList_DonoStatusOrdemELimite(t *testing.T) {
	db, scope := sessSetup(t)
	w := hsWriter(t, db)

	s := hoOpenSession(t, w, scope, 0x91, "/proj")
	mX := hsManual(t, w, scope, "op-x", "", hsNow.Add(-5*time.Minute))
	mY := hsManual(t, w, scope, "op-y", "", hsNow.Add(-4*time.Minute))
	mN := hsManual(t, w, scope, "", "", hsNow.Add(-3*time.Minute))
	aX := hsAuto(t, w, scope, s, "/proj", "op-x", hsNow.Add(-2*time.Minute))
	eX := hsManual(t, w, scope, "op-x", "", hsNow.Add(-time.Minute))
	if err := w.Do(context.Background(), func(tx *sql.Tx) error {
		_, err := CancelHandoffTx(tx, scope, eX)
		return err
	}); err != nil {
		t.Fatalf("CancelHandoffTx: %v", err)
	}
	accX := hsManual(t, w, scope, "op-x", "", hsNow.Add(-30*time.Second))
	if _, err := db.Exec(`UPDATE handoffs SET state = 'accepted' WHERE id = ?`, accX.Bytes()); err != nil {
		t.Fatalf("marcar aceito: %v", err)
	}

	ids := func(rows []HandoffRow) []domain.ID {
		out := make([]domain.ID, len(rows))
		for i, r := range rows {
			out[i] = r.ID
		}
		return out
	}
	wantIDs := func(rotulo string, got []HandoffRow, want ...domain.ID) {
		t.Helper()
		g := ids(got)
		if len(g) != len(want) {
			t.Fatalf("%s: %d linhas, esperado %d (%v)", rotulo, len(g), len(want), g)
		}
		for i := range want {
			if g[i] != want[i] {
				t.Fatalf("%s: posição %d = %s, esperado %s", rotulo, i, g[i], want[i])
			}
		}
	}

	// Op-x vê os seus e os sem dono, do mais recente ao mais antigo; não vê Y, nem cancelado, nem aceito.
	lx, err := ListHandoffs(db, scope, "op-x", false, 0)
	if err != nil {
		t.Fatalf("ListHandoffs op-x: %v", err)
	}
	wantIDs("op-x", lx, aX, mN, mX)

	// Sem ator vê só o sem dono.
	lN, err := ListHandoffs(db, scope, "", false, 0)
	if err != nil {
		t.Fatalf("ListHandoffs sem ator: %v", err)
	}
	wantIDs("sem ator", lN, mN)

	// Privilégio administrativo (anyOwner) vê todos os abertos.
	lAll, err := ListHandoffs(db, scope, "", true, 0)
	if err != nil {
		t.Fatalf("ListHandoffs anyOwner: %v", err)
	}
	wantIDs("anyOwner", lAll, aX, mN, mY, mX)

	// Limite: 1 devolve só o mais recente.
	l1, err := ListHandoffs(db, scope, "op-x", false, 1)
	if err != nil {
		t.Fatalf("ListHandoffs limite 1: %v", err)
	}
	wantIDs("limite 1", l1, aX)

	// Listar não reivindica: o automático segue aberto.
	if st := hoState(t, db, aX); st != "open" {
		t.Fatalf("após listar, estado de aX = %q, esperado open", st)
	}
}

func TestHandoffList_LimitePadraoEMaximo(t *testing.T) {
	db, scope := sessSetup(t)
	w := hsWriter(t, db)

	if err := w.Do(context.Background(), func(tx *sql.Tx) error {
		for i := range 201 {
			if _, err := InsertHandoffTx(tx, HandoffInsert{
				Scope: scope, FromAgent: "operador", Summary: "lote",
				At: hsNow.Add(-time.Duration(i) * time.Second),
			}); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		t.Fatalf("inserir lote: %v", err)
	}

	padrao, err := ListHandoffs(db, scope, "", false, 0)
	if err != nil {
		t.Fatalf("ListHandoffs padrão: %v", err)
	}
	if len(padrao) != 50 {
		t.Errorf("limite zero: %d linhas, esperado 50", len(padrao))
	}
	maximo, err := ListHandoffs(db, scope, "", false, 500)
	if err != nil {
		t.Fatalf("ListHandoffs máximo: %v", err)
	}
	if len(maximo) != 200 {
		t.Errorf("limite 500: %d linhas, esperado 200", len(maximo))
	}
}
