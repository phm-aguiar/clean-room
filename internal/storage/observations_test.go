package storage

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/phm-aguiar/ai-memory-go/internal/domain"
	"github.com/phm-aguiar/ai-memory-go/internal/domain/sanitize"
)

// obsSetup abre o banco com o esquema real, cria escopo e uma sessão do escopo ws1/proj1.
func obsSetup(t *testing.T) (*sql.DB, domain.Scope, domain.SessionID) {
	t.Helper()
	db := openSchemaDB(t)
	s := seedSchemaScope(t, db)
	sess := schemaID(0x41)
	mustSchemaExec(t, db,
		`INSERT INTO sessions (id, workspace_id, project_id, agent_kind, started_at) VALUES (?, ?, ?, 'claude-code', ?)`,
		sess, s.ws1, s.proj1, schemaTS)
	sessID, err := domain.ParseIDBytes(sess)
	if err != nil {
		t.Fatalf("ParseIDBytes session: %v", err)
	}
	return db, entScope(t, s), domain.SessionID(sessID)
}

// obsWriter abre o escritor sobre db e o fecha ao fim do teste.
func obsWriter(t *testing.T, db *sql.DB) Writer {
	t.Helper()
	w, err := NewWriter(context.Background(), db)
	if err != nil {
		t.Fatalf("NewWriter: %v", err)
	}
	t.Cleanup(func() { _ = w.Close() })
	return w
}

// obsSecret monta em runtime um token no formato github_token (ghp_ mais 36 alfanuméricos).
// O literal completo não aparece no fonte.
func obsSecret() string {
	return strings.Join([]string{"gh", "p_", strings.Repeat("Z9", 18)}, "")
}

// obsNew sanitiza título e corpo pelo pacote sanitize e monta a observação com NewObservation.
func obsNew(t *testing.T, scope domain.Scope, sess domain.SessionID, kind domain.ObservationKind,
	rawTitle, rawBody string, importance int) domain.Observation {
	t.Helper()
	id, err := domain.NewID()
	if err != nil {
		t.Fatalf("NewID: %v", err)
	}
	title := sanitize.Title(rawTitle, sanitize.Limits{TitleMaxRunes: sanitize.ObservationTitleMaxRunes})
	body := sanitize.Body(rawBody, sanitize.Limits{BodyMaxBytes: sanitize.ObservationBodyMaxBytes})
	obs, err := domain.NewObservation(domain.ObservationID(id), sess, scope, kind, title, body, importance)
	if err != nil {
		t.Fatalf("NewObservation: %v", err)
	}
	return obs
}

// obsDo grava obs em uma transação do escritor, como faz o chamador real.
func obsDo(w Writer, obs domain.Observation) error {
	return w.Do(context.Background(), func(tx *sql.Tx) error {
		return InsertObservation(tx, obs)
	})
}

func obsCount(t *testing.T, db *sql.DB) int {
	t.Helper()
	return countSchemaRows(t, db, `SELECT COUNT(*) FROM observations`)
}

func TestObservations_GravaTituloECorpoRedigidosSemSegredoCru(t *testing.T) {
	db, scope, sess := obsSetup(t)
	w := obsWriter(t, db)

	secret := obsSecret()
	obs := obsNew(t, scope, sess, domain.ObservationPostToolUse,
		"deploy com "+secret,
		"o token "+secret+" foi usado no passo zulu",
		5)

	if err := obsDo(w, obs); err != nil {
		t.Fatalf("InsertObservation: %v", err)
	}

	var title, body, kind string
	var importance int
	err := db.QueryRow(
		`SELECT title, body, kind, importance FROM observations WHERE id = ?`, obs.ID.Bytes(),
	).Scan(&title, &body, &kind, &importance)
	if err != nil {
		t.Fatalf("consulta da observação: %v", err)
	}
	if title != obs.Title.Text() {
		t.Errorf("title = %q, esperado %q", title, obs.Title.Text())
	}
	if body != obs.Body.Text() {
		t.Errorf("body = %q, esperado %q", body, obs.Body.Text())
	}
	if !strings.Contains(title, "[REDACTED:") || !strings.Contains(body, "[REDACTED:") {
		t.Errorf("marcador de redação ausente: title=%q body=%q", title, body)
	}
	if kind != string(domain.ObservationPostToolUse) {
		t.Errorf("kind = %q, esperado %q", kind, domain.ObservationPostToolUse)
	}
	if importance != 5 {
		t.Errorf("importance = %d, esperado 5", importance)
	}

	// Confirmação pelo banco: o segredo cru não está em nenhuma linha de observations.
	if n := countSchemaRows(t, db,
		`SELECT COUNT(*) FROM observations WHERE instr(title, ?) > 0 OR instr(body, ?) > 0`,
		secret, secret); n != 0 {
		t.Fatalf("segredo cru encontrado em observations (%d linhas)", n)
	}
}

func TestObservations_TrigerFTSRecebeALinha(t *testing.T) {
	db, scope, sess := obsSetup(t)
	w := obsWriter(t, db)

	obs := obsNew(t, scope, sess, domain.ObservationStop,
		"capitulo quebec", "conteudo papaia", 3)
	if err := obsDo(w, obs); err != nil {
		t.Fatalf("InsertObservation: %v", err)
	}
	if n := ftsHits(t, db, "observations_fts", "quebec"); n != 1 {
		t.Fatalf("'quebec' em observations_fts = %d hits, quero 1", n)
	}
	if n := ftsHits(t, db, "observations_fts", "papaia"); n != 1 {
		t.Fatalf("'papaia' em observations_fts = %d hits, quero 1", n)
	}
}

func TestObservations_ImportanciaForaDeFaixaRecusadaAntesDoBanco(t *testing.T) {
	db, scope, sess := obsSetup(t)
	w := obsWriter(t, db)

	for _, imp := range []int{0, 11} {
		title := sanitize.Title("x", sanitize.Limits{TitleMaxRunes: sanitize.ObservationTitleMaxRunes})
		body := sanitize.Body("y", sanitize.Limits{BodyMaxBytes: sanitize.ObservationBodyMaxBytes})

		// O construtor já recusa.
		if _, err := domain.NewObservation(domain.ObservationID{1}, sess, scope,
			domain.ObservationStop, title, body, imp); !errors.Is(err, domain.ErrInvalidImportance) {
			t.Fatalf("NewObservation(importance=%d): erro = %v, quero ErrInvalidImportance", imp, err)
		}

		// Um valor montado à mão (campos exportados) também é recusado por InsertObservation,
		// antes de qualquer SQL.
		manual := domain.Observation{
			ID:         domain.ObservationID{1},
			SessionID:  sess,
			Scope:      scope,
			Kind:       domain.ObservationStop,
			Title:      title,
			Body:       body,
			Importance: imp,
		}
		if err := obsDo(w, manual); !errors.Is(err, domain.ErrInvalidImportance) {
			t.Fatalf("InsertObservation(importance=%d): erro = %v, quero ErrInvalidImportance", imp, err)
		}
	}
	if n := obsCount(t, db); n != 0 {
		t.Fatalf("observations tem %d linhas após importâncias recusadas, quero 0", n)
	}
}

func TestObservations_KindNaoCanonicoRecusado(t *testing.T) {
	db, scope, sess := obsSetup(t)
	w := obsWriter(t, db)

	title := sanitize.Title("a", sanitize.Limits{TitleMaxRunes: sanitize.ObservationTitleMaxRunes})
	body := sanitize.Body("b", sanitize.Limits{BodyMaxBytes: sanitize.ObservationBodyMaxBytes})
	obs := domain.Observation{
		ID:         domain.ObservationID{2},
		SessionID:  sess,
		Scope:      scope,
		Kind:       "PostToolUse", // alias, não o canônico post-tool-use
		Title:      title,
		Body:       body,
		Importance: 5,
	}
	if err := obsDo(w, obs); !errors.Is(err, domain.ErrInvalidObservationKind) {
		t.Fatalf("kind não canônico: erro = %v, quero ErrInvalidObservationKind", err)
	}
	if n := obsCount(t, db); n != 0 {
		t.Fatalf("observations tem %d linhas, quero 0", n)
	}
}

func TestObservations_TransacaoNulaRecusadaSemPanic(t *testing.T) {
	_, scope, sess := obsSetup(t)
	obs := obsNew(t, scope, sess, domain.ObservationNotification, "t", "c", 3)
	if err := InsertObservation(nil, obs); err == nil {
		t.Fatal("transação nula: erro nil, quero erro")
	}
}

func TestObservations_OccurredAtZeroUsaAgoraEExplicitoEPreservado(t *testing.T) {
	db, scope, sess := obsSetup(t)
	w := obsWriter(t, db)

	// Zero: created_at fica entre o antes e o depois da chamada, em microssegundos.
	zero := obsNew(t, scope, sess, domain.ObservationSessionStart, "zero", "agora", 5)
	before := time.Now().UnixMicro()
	if err := obsDo(w, zero); err != nil {
		t.Fatalf("InsertObservation (zero): %v", err)
	}
	after := time.Now().UnixMicro()
	var createdZero int64
	if err := db.QueryRow(`SELECT created_at FROM observations WHERE id = ?`, zero.ID.Bytes()).Scan(&createdZero); err != nil {
		t.Fatalf("consulta created_at (zero): %v", err)
	}
	if createdZero < before || createdZero > after {
		t.Errorf("created_at = %d, esperado entre %d e %d", createdZero, before, after)
	}

	// Explícito: o instante informado é gravado.
	const explicit int64 = 1700000123456789
	fixed := obsNew(t, scope, sess, domain.ObservationPreCompact, "fixo", "reprocessado", 6)
	fixed.OccurredAt = explicit
	if err := obsDo(w, fixed); err != nil {
		t.Fatalf("InsertObservation (explícito): %v", err)
	}
	var createdFixed int64
	if err := db.QueryRow(`SELECT created_at FROM observations WHERE id = ?`, fixed.ID.Bytes()).Scan(&createdFixed); err != nil {
		t.Fatalf("consulta created_at (explícito): %v", err)
	}
	if createdFixed != explicit {
		t.Errorf("created_at = %d, esperado %d", createdFixed, explicit)
	}
}

func TestObservations_EscopoEExtensaoOpcional(t *testing.T) {
	db, scope, sess := obsSetup(t)
	w := obsWriter(t, db)

	semExt := obsNew(t, scope, sess, domain.ObservationOther, "sem extensao", "campos nulos", 5)
	comExt := obsNew(t, scope, sess, domain.ObservationOther, "com extensao", "campos preenchidos", 5)
	comExt.Extension = "acme.ci"
	comExt.SourceEvent = "build-done"

	for _, o := range []domain.Observation{semExt, comExt} {
		if err := obsDo(w, o); err != nil {
			t.Fatalf("InsertObservation: %v", err)
		}
	}

	if n := countSchemaRows(t, db,
		`SELECT COUNT(*) FROM observations WHERE id = ? AND extension IS NULL AND source_event IS NULL
		   AND session_id = ? AND workspace_id = ? AND project_id = ?`,
		semExt.ID.Bytes(), sess.Bytes(), scope.WorkspaceID.Bytes(), scope.ProjectID.Bytes()); n != 1 {
		t.Errorf("linha sem extensão: escopo ou NULLs incorretos (n=%d)", n)
	}
	if n := countSchemaRows(t, db,
		`SELECT COUNT(*) FROM observations WHERE id = ? AND extension = 'acme.ci' AND source_event = 'build-done'`,
		comExt.ID.Bytes()); n != 1 {
		t.Errorf("linha com extensão: valores não gravados (n=%d)", n)
	}
}

func TestObservations_GravacaoFazParteDoWriterDo(t *testing.T) {
	db, scope, sess := obsSetup(t)
	w := obsWriter(t, db)

	obs := obsNew(t, scope, sess, domain.ObservationUserPrompt, "rollback junto", "mesma transacao", 8)
	err := w.Do(context.Background(), func(tx *sql.Tx) error {
		if err := InsertObservation(tx, obs); err != nil {
			return err
		}
		return errWriterBoom
	})
	if !errors.Is(err, errWriterBoom) {
		t.Fatalf("Do: erro = %v, quero errWriterBoom", err)
	}
	if n := obsCount(t, db); n != 0 {
		t.Fatalf("observations tem %d linhas após ROLLBACK do Do, quero 0 (a inserção não estava na transação)", n)
	}
	if n := ftsHits(t, db, "observations_fts", "mesma"); n != 0 {
		t.Fatalf("observations_fts tem %d hits após ROLLBACK, quero 0", n)
	}
}
