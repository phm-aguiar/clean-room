package storage

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	"github.com/phm-aguiar/ai-memory-go/internal/domain"
)

// sessSetup abre o banco com o esquema real e o escopo ws1/proj1 semeado.
func sessSetup(t *testing.T) (*sql.DB, domain.Scope) {
	t.Helper()
	db := openSchemaDB(t)
	s := seedSchemaScope(t, db)
	return db, entScope(t, s)
}

// sessID monta um SessionID a partir de um byte de fixture.
func sessID(t *testing.T, b byte) domain.SessionID {
	t.Helper()
	id, err := domain.ParseIDBytes(schemaID(b))
	if err != nil {
		t.Fatalf("ParseIDBytes: %v", err)
	}
	return domain.SessionID(id)
}

// sessInTx roda fn em uma transação do próprio db e faz commit.
func sessInTx(t *testing.T, db *sql.DB, fn func(*sql.Tx) error) {
	t.Helper()
	tx, err := db.Begin()
	if err != nil {
		t.Fatalf("Begin: %v", err)
	}
	if err := fn(tx); err != nil {
		_ = tx.Rollback()
		t.Fatalf("fn: %v", err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("Commit: %v", err)
	}
}

// sessOpenDo abre ou toca a sessão pelo escritor, como o chamador real.
func sessOpenDo(t *testing.T, w Writer, s SessionOpen) SessionTouch {
	t.Helper()
	var out SessionTouch
	err := w.Do(context.Background(), func(tx *sql.Tx) error {
		var e error
		out, e = OpenOrTouchSessionTx(tx, s)
		return e
	})
	if err != nil {
		t.Fatalf("OpenOrTouchSessionTx: %v", err)
	}
	return out
}

// sessRow lê a linha de uma sessão para conferir o que foi gravado.
type sessRow struct {
	startedAt  int64
	cwd        sql.NullString
	actor      sql.NullString
	endedAt    sql.NullInt64
	endedCount int
	agentKind  string
}

func sessReadRow(t *testing.T, db *sql.DB, id domain.SessionID) sessRow {
	t.Helper()
	var r sessRow
	err := db.QueryRow(
		`SELECT started_at, cwd, actor_user, ended_at, ended_observation_count, agent_kind FROM sessions WHERE id = ?`,
		id.Bytes(),
	).Scan(&r.startedAt, &r.cwd, &r.actor, &r.endedAt, &r.endedCount, &r.agentKind)
	if err != nil {
		t.Fatalf("ler sessão: %v", err)
	}
	return r
}

func sessCountRows(t *testing.T, db *sql.DB, id domain.SessionID) int {
	t.Helper()
	return countSchemaRows(t, db, `SELECT COUNT(*) FROM sessions WHERE id = ?`, id.Bytes())
}

// --- definição de efêmera (função pura) ---

func TestSessionIsEphemeralKinds_Positivos(t *testing.T) {
	casos := map[string][]domain.ObservationKind{
		"sem observações":         nil,
		"só início e fim":         {domain.ObservationSessionStart, domain.ObservationSessionEnd},
		"só parada e notificação": {domain.ObservationStop, domain.ObservationNotification},
		"só compactação e outros": {domain.ObservationPreCompact, domain.ObservationPostCompaction, domain.ObservationOther},
	}
	for nome, kinds := range casos {
		if !IsEphemeral(kinds) {
			t.Errorf("%s: esperado efêmera", nome)
		}
	}
}

func TestSessionIsEphemeralKinds_Negativos(t *testing.T) {
	for _, kind := range []domain.ObservationKind{
		domain.ObservationUserPrompt,
		domain.ObservationPreToolUse,
		domain.ObservationPostToolUse,
	} {
		if IsEphemeral([]domain.ObservationKind{domain.ObservationSessionStart, kind}) {
			t.Errorf("tipo %q: esperado não efêmera", kind)
		}
		if !IsSubstantiveObservationKind(kind) {
			t.Errorf("tipo %q deveria ser substantivo", kind)
		}
	}
	if IsSubstantiveObservationKind(domain.ObservationStop) {
		t.Errorf("stop não deve ser substantivo")
	}
}

// --- verificação no armazenamento: a mesma regra sobre o banco ---

func TestSessionIsEphemeral_ConcordaComAFuncaoPura(t *testing.T) {
	db, scope := sessSetup(t)
	w := obsWriter(t, db)

	casos := []struct {
		nome  string
		b     byte
		kinds []domain.ObservationKind
	}{
		{"sem observações", 0x61, nil},
		{"só início", 0x62, []domain.ObservationKind{domain.ObservationSessionStart}},
		{"parada", 0x63, []domain.ObservationKind{domain.ObservationStop}},
		{"prompt", 0x64, []domain.ObservationKind{domain.ObservationSessionStart, domain.ObservationUserPrompt}},
		{"pré-ferramenta", 0x65, []domain.ObservationKind{domain.ObservationPreToolUse}},
		{"pós-ferramenta", 0x66, []domain.ObservationKind{domain.ObservationPostToolUse}},
	}
	for _, c := range casos {
		sess := sessID(t, c.b)
		sessInTx(t, db, func(tx *sql.Tx) error {
			_, err := OpenOrTouchSessionTx(tx, SessionOpen{ID: sess, Scope: scope, AgentKind: "claude-code"})
			return err
		})
		for _, k := range c.kinds {
			obs := obsNew(t, scope, sess, k, "titulo", "corpo", 5)
			if err := obsDo(w, obs); err != nil {
				t.Fatalf("%s: InsertObservation: %v", c.nome, err)
			}
		}

		var got bool
		var err error
		sessInTx(t, db, func(tx *sql.Tx) error {
			got, err = SessionIsEphemeral(tx, sess)
			return nil
		})
		if err != nil {
			t.Fatalf("%s: SessionIsEphemeral: %v", c.nome, err)
		}
		if want := IsEphemeral(c.kinds); got != want {
			t.Errorf("%s: SQL=%v, função pura=%v", c.nome, got, want)
		}
	}
}

func TestSessionIsEphemeral_SessaoInexistente(t *testing.T) {
	db, _ := sessSetup(t)
	var err error
	sessInTx(t, db, func(tx *sql.Tx) error {
		_, err = SessionIsEphemeral(tx, sessID(t, 0x70))
		return nil
	})
	if !errors.Is(err, ErrSessionNotFound) {
		t.Fatalf("esperado ErrSessionNotFound, veio %v", err)
	}
}

// --- abrir ou tocar ---

func TestSessionOpenOrTouch_IdempotenteNaoDuplica(t *testing.T) {
	db, scope := sessSetup(t)
	w := obsWriter(t, db)
	sess := sessID(t, 0x81)

	primeira := sessOpenDo(t, w, SessionOpen{
		ID: sess, Scope: scope, AgentKind: "claude-code", Cwd: "/a", At: time.Unix(schemaTS, 0),
	})
	if !primeira.Created || primeira.Ended {
		t.Fatalf("primeira chamada: %+v, esperado Created=true", primeira)
	}
	antes := sessReadRow(t, db, sess)

	segunda := sessOpenDo(t, w, SessionOpen{
		ID: sess, Scope: scope, AgentKind: "claude-code", Cwd: "/outro", At: time.Unix(schemaTS+500, 0),
	})
	if segunda.Created {
		t.Fatalf("segunda chamada criou de novo: %+v", segunda)
	}
	if n := sessCountRows(t, db, sess); n != 1 {
		t.Fatalf("linhas da sessão = %d, esperado 1", n)
	}
	depois := sessReadRow(t, db, sess)
	if depois.startedAt != antes.startedAt {
		t.Errorf("started_at mudou: %d -> %d", antes.startedAt, depois.startedAt)
	}
	if depois.cwd.String != "/a" {
		t.Errorf("cwd já preenchido foi sobrescrito: %q", depois.cwd.String)
	}
}

func TestSessionOpenOrTouch_PreencheCwdVazio(t *testing.T) {
	db, scope := sessSetup(t)
	w := obsWriter(t, db)
	sess := sessID(t, 0x82)

	sessOpenDo(t, w, SessionOpen{ID: sess, Scope: scope, AgentKind: "claude-code"})
	if r := sessReadRow(t, db, sess); r.cwd.Valid {
		t.Fatalf("cwd deveria ser NULL, veio %q", r.cwd.String)
	}
	sessOpenDo(t, w, SessionOpen{ID: sess, Scope: scope, AgentKind: "claude-code", Cwd: "/projeto"})
	if r := sessReadRow(t, db, sess); r.cwd.String != "/projeto" {
		t.Fatalf("cwd = %q, esperado /projeto", r.cwd.String)
	}
}

func TestSessionOpenOrTouch_CriaComActorECwdNulos(t *testing.T) {
	db, scope := sessSetup(t)
	w := obsWriter(t, db)
	sess := sessID(t, 0x83)

	sessOpenDo(t, w, SessionOpen{ID: sess, Scope: scope, AgentKind: "codex", ActorUser: "ana"})
	r := sessReadRow(t, db, sess)
	if r.actor.String != "ana" || r.agentKind != "codex" {
		t.Fatalf("linha = %+v", r)
	}
	if r.cwd.Valid {
		t.Errorf("cwd deveria ser NULL")
	}
	if r.endedAt.Valid {
		t.Errorf("sessão nova não deveria estar encerrada")
	}
}

func TestSessionOpenOrTouch_EscopoDivergenteRecusa(t *testing.T) {
	db, scope := sessSetup(t)
	w := obsWriter(t, db)
	sess := sessID(t, 0x84)
	sessOpenDo(t, w, SessionOpen{ID: sess, Scope: scope, AgentKind: "claude-code"})

	outroID := sessID(t, 0x0F)
	outro := domain.NewScope(domain.WorkspaceID(outroID), domain.ProjectID(outroID))
	err := w.Do(context.Background(), func(tx *sql.Tx) error {
		_, e := OpenOrTouchSessionTx(tx, SessionOpen{ID: sess, Scope: outro, AgentKind: "claude-code"})
		return e
	})
	if !errors.Is(err, ErrSessionScope) {
		t.Fatalf("esperado ErrSessionScope, veio %v", err)
	}
}

func TestSessionOpenOrTouch_AgentKindVazioRecusa(t *testing.T) {
	db, scope := sessSetup(t)
	w := obsWriter(t, db)
	err := w.Do(context.Background(), func(tx *sql.Tx) error {
		_, e := OpenOrTouchSessionTx(tx, SessionOpen{ID: sessID(t, 0x85), Scope: scope, AgentKind: "  "})
		return e
	})
	if err == nil {
		t.Fatalf("agent_kind vazio foi aceito")
	}
	if n := sessCountRows(t, db, sessID(t, 0x85)); n != 0 {
		t.Fatalf("sessão recusada foi gravada")
	}
}

func TestSessionOpenOrTouch_ToqueNaoReabreSessaoEncerrada(t *testing.T) {
	db, scope := sessSetup(t)
	w := obsWriter(t, db)
	sess := sessID(t, 0x86)
	sessOpenDo(t, w, SessionOpen{ID: sess, Scope: scope, AgentKind: "claude-code"})
	if err := w.Do(context.Background(), func(tx *sql.Tx) error {
		_, e := CloseSessionTx(tx, sess, time.Unix(schemaTS+10, 0))
		return e
	}); err != nil {
		t.Fatalf("CloseSessionTx: %v", err)
	}

	toque := sessOpenDo(t, w, SessionOpen{ID: sess, Scope: scope, AgentKind: "claude-code"})
	if !toque.Ended {
		t.Fatalf("toque em sessão encerrada devolveu Ended=false")
	}
	if r := sessReadRow(t, db, sess); !r.endedAt.Valid {
		t.Fatalf("toque reabriu a sessão")
	}
}

// --- cwd armazenado ---

func TestSessionCwd_SessaoNovaComCwd(t *testing.T) {
	db, scope := sessSetup(t)
	w := obsWriter(t, db)
	sess := sessID(t, 0xa0)
	sessOpenDo(t, w, SessionOpen{ID: sess, Scope: scope, AgentKind: "claude-code", Cwd: "/projeto/a"})

	var got string
	var err error
	sessInTx(t, db, func(tx *sql.Tx) error {
		got, err = SessionCwd(tx, sess)
		return nil
	})
	if err != nil {
		t.Fatalf("SessionCwd: %v", err)
	}
	if got != "/projeto/a" {
		t.Fatalf("cwd = %q, esperado /projeto/a", got)
	}
}

func TestSessionCwd_ToqueMantemCwd(t *testing.T) {
	db, scope := sessSetup(t)
	w := obsWriter(t, db)
	sess := sessID(t, 0xa1)
	sessOpenDo(t, w, SessionOpen{ID: sess, Scope: scope, AgentKind: "claude-code", Cwd: "/projeto/a"})
	sessOpenDo(t, w, SessionOpen{ID: sess, Scope: scope, AgentKind: "claude-code", Cwd: "/outro"})

	var got string
	var err error
	sessInTx(t, db, func(tx *sql.Tx) error {
		got, err = SessionCwd(tx, sess)
		return nil
	})
	if err != nil {
		t.Fatalf("SessionCwd: %v", err)
	}
	if got != "/projeto/a" {
		t.Fatalf("toque alterou o cwd: %q, esperado /projeto/a", got)
	}
}

func TestSessionCwd_SessaoInexistente(t *testing.T) {
	db, _ := sessSetup(t)
	var err error
	sessInTx(t, db, func(tx *sql.Tx) error {
		_, err = SessionCwd(tx, sessID(t, 0xa2))
		return nil
	})
	if !errors.Is(err, ErrSessionNotFound) {
		t.Fatalf("esperado ErrSessionNotFound, veio %v", err)
	}
}

// --- encerrar ---

func TestSessionClose_MarcaSemApagar(t *testing.T) {
	db, scope := sessSetup(t)
	w := obsWriter(t, db)
	sess := sessID(t, 0x91)
	sessOpenDo(t, w, SessionOpen{ID: sess, Scope: scope, AgentKind: "claude-code"})
	obs := obsNew(t, scope, sess, domain.ObservationUserPrompt, "prompt", "corpo", 5)
	if err := obsDo(w, obs); err != nil {
		t.Fatalf("obsDo: %v", err)
	}

	fim := time.Unix(schemaTS+60, 0)
	var res SessionClose
	if err := w.Do(context.Background(), func(tx *sql.Tx) error {
		var e error
		res, e = CloseSessionTx(tx, sess, fim)
		return e
	}); err != nil {
		t.Fatalf("CloseSessionTx: %v", err)
	}
	if !res.Closed {
		t.Fatalf("primeiro encerramento: Closed=false")
	}
	if n := sessCountRows(t, db, sess); n != 1 {
		t.Fatalf("encerrar apagou a sessão: linhas = %d", n)
	}
	r := sessReadRow(t, db, sess)
	if !r.endedAt.Valid || r.endedAt.Int64 != fim.UnixMicro() {
		t.Fatalf("ended_at = %+v, esperado %d", r.endedAt, fim.UnixMicro())
	}
	if r.endedCount != 1 {
		t.Fatalf("ended_observation_count = %d, esperado 1", r.endedCount)
	}
}

func TestSessionClose_IdempotenteMantemPrimeiroEncerramento(t *testing.T) {
	db, scope := sessSetup(t)
	w := obsWriter(t, db)
	sess := sessID(t, 0x92)
	sessOpenDo(t, w, SessionOpen{ID: sess, Scope: scope, AgentKind: "claude-code"})

	primeiro := time.Unix(schemaTS+100, 0)
	if err := w.Do(context.Background(), func(tx *sql.Tx) error {
		_, e := CloseSessionTx(tx, sess, primeiro)
		return e
	}); err != nil {
		t.Fatalf("primeiro CloseSessionTx: %v", err)
	}

	var res SessionClose
	if err := w.Do(context.Background(), func(tx *sql.Tx) error {
		var e error
		res, e = CloseSessionTx(tx, sess, time.Unix(schemaTS+999, 0))
		return e
	}); err != nil {
		t.Fatalf("segundo CloseSessionTx: %v", err)
	}
	if res.Closed {
		t.Fatalf("segundo encerramento devolveu Closed=true")
	}
	if r := sessReadRow(t, db, sess); r.endedAt.Int64 != primeiro.UnixMicro() {
		t.Fatalf("ended_at mudou no segundo encerramento: %d", r.endedAt.Int64)
	}
}

func TestSessionClose_InexistenteDevolveErro(t *testing.T) {
	db, _ := sessSetup(t)
	w := obsWriter(t, db)
	err := w.Do(context.Background(), func(tx *sql.Tx) error {
		_, e := CloseSessionTx(tx, sessID(t, 0x93), time.Unix(schemaTS, 0))
		return e
	})
	if !errors.Is(err, ErrSessionNotFound) {
		t.Fatalf("esperado ErrSessionNotFound, veio %v", err)
	}
}
