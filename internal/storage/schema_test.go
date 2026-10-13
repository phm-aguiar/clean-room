package storage

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"reflect"
	"strings"
	"testing"
)

// Testes do esquema V01 aplicado pelo caminho real (Open com migrações embutidas).
// Cobrem: tabelas da 001, triggers FTS, triggers de pareamento (DB §0.1),
// índice único parcial idx_pages_latest_path e registro em schema_migrations.

// schemaTS é um instante Unix inteiro fixo usado em todos os fixtures.
const schemaTS int64 = 1700000000

// openSchemaDB abre um banco novo em diretório temporário via Open.
func openSchemaDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := Open(context.Background(), t.TempDir())
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}

// schemaID devolve um ID BLOB de 16 bytes preenchido com b.
func schemaID(b byte) []byte {
	id := make([]byte, 16)
	for i := range id {
		id[i] = b
	}
	return id
}

// schemaScope guarda dois workspaces e três projetos:
// proj1 e proj2 pertencem a ws1; proj3 pertence a ws2.
type schemaScope struct {
	ws1, ws2, proj1, proj2, proj3 []byte
}

func seedSchemaScope(t *testing.T, db *sql.DB) schemaScope {
	t.Helper()
	s := schemaScope{
		ws1:   schemaID(0x01),
		ws2:   schemaID(0x02),
		proj1: schemaID(0x11),
		proj2: schemaID(0x12),
		proj3: schemaID(0x21),
	}
	mustSchemaExec(t, db, `INSERT INTO workspaces (id, name, created_at) VALUES (?, ?, ?)`, s.ws1, "ws-1", schemaTS)
	mustSchemaExec(t, db, `INSERT INTO workspaces (id, name, created_at) VALUES (?, ?, ?)`, s.ws2, "ws-2", schemaTS)
	mustSchemaExec(t, db, `INSERT INTO projects (id, workspace_id, name, created_at) VALUES (?, ?, ?, ?)`, s.proj1, s.ws1, "proj-1", schemaTS)
	mustSchemaExec(t, db, `INSERT INTO projects (id, workspace_id, name, created_at) VALUES (?, ?, ?, ?)`, s.proj2, s.ws1, "proj-2", schemaTS)
	mustSchemaExec(t, db, `INSERT INTO projects (id, workspace_id, name, created_at) VALUES (?, ?, ?, ?)`, s.proj3, s.ws2, "proj-3", schemaTS)
	return s
}

func mustSchemaExec(t *testing.T, db *sql.DB, query string, args ...any) {
	t.Helper()
	if _, err := db.Exec(query, args...); err != nil {
		t.Fatalf("exec %q: %v", query, err)
	}
}

func countSchemaRows(t *testing.T, db *sql.DB, query string, args ...any) int {
	t.Helper()
	var n int
	if err := db.QueryRow(query, args...).Scan(&n); err != nil {
		t.Fatalf("count %q: %v", query, err)
	}
	return n
}

func schemaObjectExists(t *testing.T, db *sql.DB, typ, name string) bool {
	t.Helper()
	n := countSchemaRows(t, db, `SELECT COUNT(*) FROM sqlite_master WHERE type = ? AND name = ?`, typ, name)
	return n == 1
}

// insertSchemaPage insere uma página com corpo de 32 bytes zerados em body_sha256.
func insertSchemaPage(db *sql.DB, id, ws, proj []byte, path, title, body string, isLatest int) error {
	_, err := db.Exec(
		`INSERT INTO pages (id, workspace_id, project_id, path, title, tier, body, body_sha256, is_latest, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, 'semantic', ?, ?, ?, ?, ?)`,
		id, ws, proj, path, title, body, make([]byte, 32), isLatest, schemaTS, schemaTS,
	)
	return err
}

func ftsHits(t *testing.T, db *sql.DB, table, term string) int {
	t.Helper()
	return countSchemaRows(t, db, "SELECT COUNT(*) FROM "+table+" WHERE "+table+" MATCH ?", term)
}

func TestSchemaTables(t *testing.T) {
	db := openSchemaDB(t)
	tables := []string{
		"workspaces", "projects", "pages", "pages_fts", "links", "entities",
		"entity_page_links", "page_access", "page_evidence", "audit_log",
		"sessions", "observations", "observations_fts", "schema_migrations",
	}
	for _, name := range tables {
		if !schemaObjectExists(t, db, "table", name) {
			t.Fatalf("tabela %q ausente do esquema V01", name)
		}
	}
}

func TestSchemaTriggersAndIndexesPresent(t *testing.T) {
	db := openSchemaDB(t)

	fts := []string{
		"pages_fts_ai", "pages_fts_ad", "pages_fts_au",
		"observations_fts_ai", "observations_fts_ad", "observations_fts_au",
	}
	pairing := []string{
		"pages_ws_proj_pairing_ai",
		"sessions_ws_proj_pairing_ai",
		"observations_ws_proj_pairing_ai",
		"session_consolidation_jobs_session_pairing_ai",
		"handoffs_ws_proj_pairing_ai",
		"entities_ws_proj_pairing_ai",
		"entity_page_links_ws_proj_pairing_ai",
		"auto_improve_runs_ws_proj_pairing_ai",
		"auto_improve_proposals_ws_proj_pairing_ai",
		"auto_improve_rejections_ws_proj_pairing_ai",
		"auto_improve_scheduler_state_ws_proj_pairing_ai",
		"auto_improve_scheduler_claims_ws_proj_pairing_ai",
		"auto_improve_scheduler_claims_session_pairing_ai",
		"workstreams_ws_proj_pairing_ai",
	}
	for _, name := range append(fts, pairing...) {
		if !schemaObjectExists(t, db, "trigger", name) {
			t.Fatalf("trigger %q ausente do esquema V01", name)
		}
	}
	if !schemaObjectExists(t, db, "index", "idx_pages_latest_path") {
		t.Fatalf("índice idx_pages_latest_path ausente do esquema V01")
	}
}

func TestSchemaMigrationsRecorded(t *testing.T) {
	db := openSchemaDB(t)

	rows, err := db.Query(`SELECT name FROM pragma_table_info('schema_migrations') ORDER BY cid`)
	if err != nil {
		t.Fatalf("pragma_table_info: %v", err)
	}
	var cols []string
	for rows.Next() {
		var c string
		if err := rows.Scan(&c); err != nil {
			_ = rows.Close()
			t.Fatalf("scan coluna: %v", err)
		}
		cols = append(cols, c)
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		t.Fatalf("iterar colunas: %v", err)
	}
	_ = rows.Close()
	wantCols := []string{"version", "name", "applied_on", "checksum"}
	if !reflect.DeepEqual(cols, wantCols) {
		t.Fatalf("colunas de schema_migrations = %v, quero %v", cols, wantCols)
	}

	raw, err := migrationsFS.ReadFile("migrations/V01__init.sql")
	if err != nil {
		t.Fatalf("ler V01 embutida: %v", err)
	}
	sum := sha256.Sum256(raw)
	wantChecksum := hex.EncodeToString(sum[:])

	var version, appliedOn int64
	var name, checksum string
	err = db.QueryRow(`SELECT version, name, applied_on, checksum FROM schema_migrations WHERE version = 1`).
		Scan(&version, &name, &appliedOn, &checksum)
	if err != nil {
		t.Fatalf("V01 não registrada em schema_migrations: %v", err)
	}
	if name != "init" {
		t.Fatalf("name = %q, quero %q", name, "init")
	}
	if appliedOn <= 0 {
		t.Fatalf("applied_on = %d, quero instante Unix positivo", appliedOn)
	}
	if checksum != wantChecksum {
		t.Fatalf("checksum = %s, quero SHA-256 do arquivo %s", checksum, wantChecksum)
	}
}

func TestSchemaFTSPages(t *testing.T) {
	db := openSchemaDB(t)
	s := seedSchemaScope(t, db)
	pid := schemaID(0x31)

	if err := insertSchemaPage(db, pid, s.ws1, s.proj1, "notas/a.md", "alfa", "bravo", 1); err != nil {
		t.Fatalf("insert página: %v", err)
	}
	if n := ftsHits(t, db, "pages_fts", "bravo"); n != 1 {
		t.Fatalf("após insert, 'bravo' em pages_fts = %d hits, quero 1", n)
	}
	if n := ftsHits(t, db, "pages_fts", "alfa"); n != 1 {
		t.Fatalf("após insert, 'alfa' (title) em pages_fts = %d hits, quero 1", n)
	}

	mustSchemaExec(t, db, `UPDATE pages SET body = ?, updated_at = ? WHERE id = ?`, "charlie", schemaTS+1, pid)
	if n := ftsHits(t, db, "pages_fts", "bravo"); n != 0 {
		t.Fatalf("após UPDATE body, 'bravo' ainda indexado (%d hits)", n)
	}
	if n := ftsHits(t, db, "pages_fts", "charlie"); n != 1 {
		t.Fatalf("após UPDATE body, 'charlie' em pages_fts = %d hits, quero 1", n)
	}

	mustSchemaExec(t, db, `UPDATE pages SET title = ?, updated_at = ? WHERE id = ?`, "delta", schemaTS+2, pid)
	if n := ftsHits(t, db, "pages_fts", "alfa"); n != 0 {
		t.Fatalf("após UPDATE title, 'alfa' ainda indexado (%d hits)", n)
	}
	if n := ftsHits(t, db, "pages_fts", "delta"); n != 1 {
		t.Fatalf("após UPDATE title, 'delta' em pages_fts = %d hits, quero 1", n)
	}

	mustSchemaExec(t, db, `DELETE FROM pages WHERE id = ?`, pid)
	if n := ftsHits(t, db, "pages_fts", "charlie"); n != 0 {
		t.Fatalf("após DELETE, 'charlie' ainda em pages_fts (%d hits)", n)
	}
	if n := ftsHits(t, db, "pages_fts", "delta"); n != 0 {
		t.Fatalf("após DELETE, 'delta' ainda em pages_fts (%d hits)", n)
	}
}

func TestSchemaFTSObservations(t *testing.T) {
	db := openSchemaDB(t)
	s := seedSchemaScope(t, db)
	sessID := schemaID(0x41)
	obsID := schemaID(0x42)

	mustSchemaExec(t, db,
		`INSERT INTO sessions (id, workspace_id, project_id, agent_kind, started_at) VALUES (?, ?, ?, 'claude-code', ?)`,
		sessID, s.ws1, s.proj1, schemaTS)
	mustSchemaExec(t, db,
		`INSERT INTO observations (id, session_id, workspace_id, project_id, kind, title, body, created_at)
		 VALUES (?, ?, ?, ?, 'nota', 'foxtrot', 'golf', ?)`,
		obsID, sessID, s.ws1, s.proj1, schemaTS)
	if n := ftsHits(t, db, "observations_fts", "golf"); n != 1 {
		t.Fatalf("após insert, 'golf' em observations_fts = %d hits, quero 1", n)
	}
	if n := ftsHits(t, db, "observations_fts", "foxtrot"); n != 1 {
		t.Fatalf("após insert, 'foxtrot' em observations_fts = %d hits, quero 1", n)
	}

	// observations_fts_au dispara em qualquer coluna: alterar só importance não pode duplicar a entrada.
	mustSchemaExec(t, db, `UPDATE observations SET importance = 7 WHERE id = ?`, obsID)
	if n := ftsHits(t, db, "observations_fts", "foxtrot"); n != 1 {
		t.Fatalf("após UPDATE de coluna não textual, 'foxtrot' = %d hits, quero 1", n)
	}

	mustSchemaExec(t, db, `UPDATE observations SET title = ? WHERE id = ?`, "hotel", obsID)
	if n := ftsHits(t, db, "observations_fts", "foxtrot"); n != 0 {
		t.Fatalf("após UPDATE title, 'foxtrot' ainda indexado (%d hits)", n)
	}
	if n := ftsHits(t, db, "observations_fts", "hotel"); n != 1 {
		t.Fatalf("após UPDATE title, 'hotel' = %d hits, quero 1", n)
	}

	mustSchemaExec(t, db, `UPDATE observations SET body = ? WHERE id = ?`, "india", obsID)
	if n := ftsHits(t, db, "observations_fts", "golf"); n != 0 {
		t.Fatalf("após UPDATE body, 'golf' ainda indexado (%d hits)", n)
	}
	if n := ftsHits(t, db, "observations_fts", "india"); n != 1 {
		t.Fatalf("após UPDATE body, 'india' = %d hits, quero 1", n)
	}

	mustSchemaExec(t, db, `DELETE FROM observations WHERE id = ?`, obsID)
	if n := ftsHits(t, db, "observations_fts", "india"); n != 0 {
		t.Fatalf("após DELETE, 'india' ainda em observations_fts (%d hits)", n)
	}
	if n := ftsHits(t, db, "observations_fts", "hotel"); n != 0 {
		t.Fatalf("após DELETE, 'hotel' ainda em observations_fts (%d hits)", n)
	}
}

func TestSchemaPairingTriggers(t *testing.T) {
	db := openSchemaDB(t)
	s := seedSchemaScope(t, db)

	// pages_ws_proj_pairing_ai: workspace da página tem de ser o do projeto.
	badPage := schemaID(0x51)
	err := insertSchemaPage(db, badPage, s.ws2, s.proj1, "ruim.md", "t", "b", 1)
	requireSchemaRejected(t, "pages com workspace de outro escopo", err)
	requireSchemaAbsent(t, db, "pages", badPage)
	okPage := schemaID(0x52)
	if err := insertSchemaPage(db, okPage, s.ws1, s.proj1, "ok.md", "t", "b", 1); err != nil {
		t.Fatalf("página no escopo correto recusada: %v", err)
	}

	// sessions_ws_proj_pairing_ai.
	badSession := schemaID(0x53)
	_, err = db.Exec(
		`INSERT INTO sessions (id, workspace_id, project_id, agent_kind, started_at) VALUES (?, ?, ?, 'claude-code', ?)`,
		badSession, s.ws2, s.proj1, schemaTS)
	requireSchemaRejected(t, "sessions com workspace de outro escopo", err)
	requireSchemaAbsent(t, db, "sessions", badSession)
	sessID := schemaID(0x54)
	mustSchemaExec(t, db,
		`INSERT INTO sessions (id, workspace_id, project_id, agent_kind, started_at) VALUES (?, ?, ?, 'claude-code', ?)`,
		sessID, s.ws1, s.proj1, schemaTS)

	// observations_ws_proj_pairing_ai.
	badObs := schemaID(0x55)
	_, err = db.Exec(
		`INSERT INTO observations (id, session_id, workspace_id, project_id, kind, title, body, created_at)
		 VALUES (?, ?, ?, ?, 'nota', 't', 'b', ?)`,
		badObs, sessID, s.ws2, s.proj1, schemaTS)
	requireSchemaRejected(t, "observations com workspace de outro escopo", err)
	requireSchemaAbsent(t, db, "observations", badObs)

	// session_consolidation_jobs_session_pairing_ai: projeto tem de ser o da sessão.
	_, err = db.Exec(
		`INSERT INTO session_consolidation_jobs
		   (session_id, workspace_id, project_id, generation, state, requested_at, next_attempt_at)
		 VALUES (?, ?, ?, 1, 'pending', ?, ?)`,
		sessID, s.ws1, s.proj2, schemaTS, schemaTS)
	requireSchemaRejected(t, "session_consolidation_jobs com projeto diferente da sessão", err)
	if n := countSchemaRows(t, db, `SELECT COUNT(*) FROM session_consolidation_jobs WHERE session_id = ?`, sessID); n != 0 {
		t.Fatalf("job recusado ficou gravado (%d linhas)", n)
	}

	// entities_ws_proj_pairing_ai.
	badEnt := schemaID(0x56)
	_, err = db.Exec(
		`INSERT INTO entities (id, workspace_id, project_id, name, created_at) VALUES (?, ?, ?, 'ent-ruim', ?)`,
		badEnt, s.ws2, s.proj1, schemaTS)
	requireSchemaRejected(t, "entities com workspace de outro escopo", err)
	requireSchemaAbsent(t, db, "entities", badEnt)
	entA := schemaID(0x57)
	mustSchemaExec(t, db,
		`INSERT INTO entities (id, workspace_id, project_id, name, created_at) VALUES (?, ?, ?, 'ent-a', ?)`,
		entA, s.ws1, s.proj1, schemaTS)

	// entity_page_links_ws_proj_pairing_ai: entidade e página no mesmo workspace e projeto.
	pageOtherProj := schemaID(0x58)
	if err := insertSchemaPage(db, pageOtherProj, s.ws1, s.proj2, "outro-projeto.md", "t", "b", 1); err != nil {
		t.Fatalf("página em proj2 recusada: %v", err)
	}
	pageOtherWs := schemaID(0x59)
	if err := insertSchemaPage(db, pageOtherWs, s.ws2, s.proj3, "outro-ws.md", "t", "b", 1); err != nil {
		t.Fatalf("página em proj3 recusada: %v", err)
	}

	_, err = db.Exec(`INSERT INTO entity_page_links (entity_id, page_id, valid_from) VALUES (?, ?, ?)`, entA, pageOtherProj, schemaTS)
	requireSchemaRejected(t, "entity_page_links com página de outro projeto", err)
	_, err = db.Exec(`INSERT INTO entity_page_links (entity_id, page_id, valid_from) VALUES (?, ?, ?)`, entA, pageOtherWs, schemaTS)
	requireSchemaRejected(t, "entity_page_links com página de outro workspace", err)
	if n := countSchemaRows(t, db, `SELECT COUNT(*) FROM entity_page_links WHERE entity_id = ?`, entA); n != 0 {
		t.Fatalf("vínculos recusados ficaram gravados (%d linhas)", n)
	}

	_, err = db.Exec(`INSERT INTO entity_page_links (entity_id, page_id, valid_from) VALUES (?, ?, ?)`, entA, okPage, schemaTS)
	if err != nil {
		t.Fatalf("entity_page_links no mesmo escopo recusado: %v", err)
	}
}

func TestSchemaLatestPathUniqueIndex(t *testing.T) {
	db := openSchemaDB(t)
	s := seedSchemaScope(t, db)

	var idxSQL string
	err := db.QueryRow(`SELECT sql FROM sqlite_master WHERE type = 'index' AND name = 'idx_pages_latest_path'`).
		Scan(&idxSQL)
	if err != nil {
		t.Fatalf("índice idx_pages_latest_path ausente: %v", err)
	}
	if !strings.Contains(idxSQL, "UNIQUE") || !strings.Contains(idxSQL, "WHERE is_latest = 1") {
		t.Fatalf("idx_pages_latest_path não é único parcial em is_latest = 1: %s", idxSQL)
	}

	if err := insertSchemaPage(db, schemaID(0x61), s.ws1, s.proj1, "docs/a.md", "a", "b", 1); err != nil {
		t.Fatalf("primeira versão latest recusada: %v", err)
	}

	// Segunda versão latest com mesmo (workspace, projeto, path) tem de ser recusada.
	err = insertSchemaPage(db, schemaID(0x62), s.ws1, s.proj1, "docs/a.md", "a2", "b2", 1)
	if err == nil {
		t.Fatalf("duas páginas latest com mesmo path foram aceitas")
	}
	if !strings.Contains(err.Error(), "UNIQUE") {
		t.Fatalf("erro esperado de UNIQUE, veio: %v", err)
	}
	requireSchemaAbsent(t, db, "pages", schemaID(0x62))

	// Versões não latest com mesmo path são aceitas, inclusive várias.
	if err := insertSchemaPage(db, schemaID(0x63), s.ws1, s.proj1, "docs/a.md", "old1", "b", 0); err != nil {
		t.Fatalf("versão não latest 1 recusada: %v", err)
	}
	if err := insertSchemaPage(db, schemaID(0x64), s.ws1, s.proj1, "docs/a.md", "old2", "b", 0); err != nil {
		t.Fatalf("versão não latest 2 recusada: %v", err)
	}

	// O índice é por (workspace, projeto, path): outro projeto pode ter a mesma latest.
	if err := insertSchemaPage(db, schemaID(0x65), s.ws1, s.proj2, "docs/a.md", "a", "b", 1); err != nil {
		t.Fatalf("latest em outro projeto com mesmo path recusada: %v", err)
	}

	if n := countSchemaRows(t, db, `SELECT COUNT(*) FROM pages WHERE path = 'docs/a.md' AND is_latest = 1`); n != 2 {
		t.Fatalf("linhas latest com path docs/a.md = %d, quero 2 (um por projeto)", n)
	}
	if n := countSchemaRows(t, db, `SELECT COUNT(*) FROM pages WHERE path = 'docs/a.md' AND is_latest = 0`); n != 2 {
		t.Fatalf("linhas não latest com path docs/a.md = %d, quero 2", n)
	}
}

func requireSchemaRejected(t *testing.T, caso string, err error) {
	t.Helper()
	if err == nil {
		t.Fatalf("%s: aceito, mas o trigger devia recusar", caso)
	}
}

func requireSchemaAbsent(t *testing.T, db *sql.DB, table string, id []byte) {
	t.Helper()
	if n := countSchemaRows(t, db, "SELECT COUNT(*) FROM "+table+" WHERE id = ?", id); n != 0 {
		t.Fatalf("linha recusada ficou gravada em %s (%d linhas)", table, n)
	}
}
