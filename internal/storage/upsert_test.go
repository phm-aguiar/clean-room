package storage

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/phm-aguiar/ai-memory-go/internal/domain"
)

// upT0 é o instante base dos testes de UpsertPage.
var upT0 = time.Date(2026, 10, 12, 15, 30, 0, 0, time.UTC)

// errUpFalhaSimulada força o rollback de uma transação que já escreveu.
var errUpFalhaSimulada = errors.New("falha simulada no meio da transação")

// upEnv abre um banco temporário com a migração real e um escritor, e cria um workspace.
func upEnv(t *testing.T) (*sql.DB, Writer, domain.WorkspaceID) {
	t.Helper()
	ctx := context.Background()
	db, err := Open(ctx, t.TempDir())
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	w, err := NewWriter(ctx, db)
	if err != nil {
		t.Fatalf("NewWriter: %v", err)
	}
	t.Cleanup(func() { _ = w.Close() })

	wsID, err := domain.NewID()
	if err != nil {
		t.Fatalf("NewID ws: %v", err)
	}
	return db, w, domain.WorkspaceID(wsID)
}

// upScope cria um projeto novo no workspace ws, com o nome dado, e devolve o escopo.
func upScope(t *testing.T, w Writer, ws domain.WorkspaceID, wsName, pjName string) domain.Scope {
	t.Helper()
	pjID, err := domain.NewID()
	if err != nil {
		t.Fatalf("NewID pj: %v", err)
	}
	scope := domain.NewScope(ws, domain.ProjectID(pjID))
	if err := EnsureScope(context.Background(), w, scope, wsName, pjName, upT0); err != nil {
		t.Fatalf("EnsureScope: %v", err)
	}
	return scope
}

// upPath cria um PagePath válido para o teste.
func upPath(t *testing.T, s string) domain.PagePath {
	t.Helper()
	p, err := domain.ParsePagePath(s)
	if err != nil {
		t.Fatalf("ParsePagePath(%q): %v", s, err)
	}
	return p
}

// upFM monta um frontmatter a partir de pares chave, valor (string ou []string).
func upFM(t *testing.T, pairs ...any) domain.Frontmatter {
	t.Helper()
	var fm domain.Frontmatter
	for i := 0; i < len(pairs); i += 2 {
		if err := fm.Set(pairs[i].(string), pairs[i+1]); err != nil {
			t.Fatalf("Set %v: %v", pairs[i], err)
		}
	}
	return fm
}

// upIn monta uma entrada padrão: semântica, kind note, entidade "Postgres" e fonte de sessão.
func upIn(t *testing.T, ref domain.PageRef, body string, sourceID string, at time.Time) UpsertInput {
	t.Helper()
	return UpsertInput{
		Ref:         ref,
		Tier:        domain.TierSemantic,
		Kind:        domain.PageKindNote,
		Title:       "Rota",
		Body:        body,
		Frontmatter: upFM(t, "entities", []string{"Postgres"}),
		Evidence:    EvidenceSource{Kind: SourceSession, ID: sourceID},
		At:          at,
	}
}

// upMustUpsert chama UpsertPage e falha o teste em caso de erro.
func upMustUpsert(t *testing.T, w Writer, in UpsertInput) UpsertResult {
	t.Helper()
	res, err := UpsertPage(context.Background(), w, in)
	if err != nil {
		t.Fatalf("UpsertPage: %v", err)
	}
	return res
}

// upCount executa uma consulta COUNT(*) com argumentos.
func upCount(t *testing.T, db *sql.DB, query string, args ...any) int {
	t.Helper()
	var n int
	if err := db.QueryRow(query, args...).Scan(&n); err != nil {
		t.Fatalf("count (%s): %v", query, err)
	}
	return n
}

// upPageFields lê as colunas de controle de uma versão pelo id.
type upPageFields struct {
	isLatest     int
	supersedes   []byte
	validFrom    sql.NullInt64
	validTo      sql.NullInt64
	supersededAt sql.NullInt64
	bodySHA      []byte
	pathSearch   string
	tier         string
	pinned       int
	fmJSON       string
	title        string
	body         string
}

func upReadPage(t *testing.T, db *sql.DB, id domain.PageID) upPageFields {
	t.Helper()
	var f upPageFields
	err := db.QueryRow(
		`SELECT is_latest, supersedes, valid_from, valid_to, superseded_at, body_sha256,
		        path_search, tier, pinned, frontmatter_json, title, body
		   FROM pages WHERE id = ?`,
		id.Bytes(),
	).Scan(&f.isLatest, &f.supersedes, &f.validFrom, &f.validTo, &f.supersededAt, &f.bodySHA,
		&f.pathSearch, &f.tier, &f.pinned, &f.fmJSON, &f.title, &f.body)
	if err != nil {
		t.Fatalf("ler página %s: %v", id.String(), err)
	}
	return f
}

// upWindow lê valid_from e superseded_at do vínculo entre a entidade name e a página.
func upWindow(t *testing.T, db *sql.DB, name string, page domain.PageID) (validFrom int64, supersededAt sql.NullInt64, found bool) {
	t.Helper()
	err := db.QueryRow(
		`SELECT l.valid_from, l.superseded_at
		   FROM entity_page_links l JOIN entities e ON e.id = l.entity_id
		  WHERE e.name = ? AND l.page_id = ?`,
		name, page.Bytes(),
	).Scan(&validFrom, &supersededAt)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, sql.NullInt64{}, false
	}
	if err != nil {
		t.Fatalf("ler janela de %q: %v", name, err)
	}
	return validFrom, supersededAt, true
}

// upSnapshot conta as linhas que uma escrita pode criar: páginas, evidência, auditoria e vínculos.
type upSnapshot struct {
	pages, evidence, audit, entityLinks, links int
}

func upTake(t *testing.T, db *sql.DB) upSnapshot {
	t.Helper()
	return upSnapshot{
		pages:       upCount(t, db, `SELECT COUNT(*) FROM pages`),
		evidence:    upCount(t, db, `SELECT COUNT(*) FROM page_evidence`),
		audit:       upCount(t, db, `SELECT COUNT(*) FROM audit_log`),
		entityLinks: upCount(t, db, `SELECT COUNT(*) FROM entity_page_links`),
		links:       upCount(t, db, `SELECT COUNT(*) FROM links`),
	}
}

func TestUpsertCasoA_ConteudoIdenticoNaoCriaVersao(t *testing.T) {
	db, w, ws := upEnv(t)
	scope := upScope(t, w, ws, "ws-a", "pj-a")
	ref := domain.NewPageRef(scope, upPath(t, "notas/rota.md"))

	at1 := upT0
	at2 := upT0.Add(time.Minute)

	r1 := upMustUpsert(t, w, upIn(t, ref, "corpo", "sess-1", at1))
	if r1.Outcome != UpsertCreated {
		t.Fatalf("primeira escrita: outcome %q, esperado %q", r1.Outcome, UpsertCreated)
	}

	// Mesmo conteúdo, outra fonte: caso A.
	r2 := upMustUpsert(t, w, upIn(t, ref, "corpo", "sess-2", at2))
	if r2.Outcome != UpsertUnchanged {
		t.Fatalf("conteúdo idêntico: outcome %q, esperado %q", r2.Outcome, UpsertUnchanged)
	}
	if r2.Version != r1.Version {
		t.Fatalf("caso A devolveu %s, esperada a versão corrente %s", r2.Version.String(), r1.Version.String())
	}

	if n := upCount(t, db, `SELECT COUNT(*) FROM pages WHERE path = ?`, "notas/rota.md"); n != 1 {
		t.Fatalf("linhas do caminho = %d, esperada 1", n)
	}
	f := upReadPage(t, db, r1.Version)
	if f.isLatest != 1 || f.validTo.Valid || f.validFrom.Int64 != at1.UnixMicro() {
		t.Errorf("versão corrente alterada pelo caso A: is_latest=%d valid_to=%v valid_from=%d",
			f.isLatest, f.validTo, f.validFrom.Int64)
	}

	// Evidência das duas fontes, na versão corrente.
	if n := upCount(t, db, `SELECT COUNT(*) FROM page_evidence WHERE page_id = ?`, r1.Version.Bytes()); n != 2 {
		t.Errorf("evidência da versão = %d, esperada 2 (sess-1 e sess-2)", n)
	}
	if n := upCount(t, db, `SELECT COUNT(*) FROM audit_log WHERE op = ? AND page_id = ?`,
		auditPageUnchanged, r1.Version.Bytes()); n != 1 {
		t.Errorf("auditoria page-unchanged = %d, esperada 1", n)
	}
}

func TestUpsertCasoA_GeneratedAtNaoContaComoMudanca(t *testing.T) {
	db, w, ws := upEnv(t)
	scope := upScope(t, w, ws, "ws-a", "pj-a")
	ref := domain.NewPageRef(scope, upPath(t, "notas/rota.md"))

	in := upIn(t, ref, "corpo", "sess-1", upT0)
	in.Frontmatter = upFM(t, "entities", []string{"Postgres"}, "generated.at", "2026-10-12T10:00:00Z")
	r1 := upMustUpsert(t, w, in)

	in2 := upIn(t, ref, "corpo", "sess-2", upT0.Add(time.Minute))
	in2.Frontmatter = upFM(t, "entities", []string{"Postgres"}, "generated.at", "2026-10-12T11:00:00Z")
	r2 := upMustUpsert(t, w, in2)

	if r2.Outcome != UpsertUnchanged || r2.Version != r1.Version {
		t.Fatalf("só generated.at mudou: outcome %q, versão %s; esperado unchanged na versão %s",
			r2.Outcome, r2.Version.String(), r1.Version.String())
	}
	if n := upCount(t, db, `SELECT COUNT(*) FROM pages`); n != 1 {
		t.Errorf("linhas de pages = %d, esperada 1", n)
	}
}

func TestUpsertCasoB_SupersedeFechaAntigaEAbreNova(t *testing.T) {
	db, w, ws := upEnv(t)
	scope := upScope(t, w, ws, "ws-b", "pj-b")
	ref := domain.NewPageRef(scope, upPath(t, "notas/rota.md"))

	at1 := upT0
	at2 := upT0.Add(time.Hour)

	r1 := upMustUpsert(t, w, upIn(t, ref, "corpo v1", "sess-1", at1))
	r2 := upMustUpsert(t, w, upIn(t, ref, "corpo v2", "sess-2", at2))

	if r2.Outcome != UpsertSuperseded {
		t.Fatalf("outcome %q, esperado %q", r2.Outcome, UpsertSuperseded)
	}
	if r2.Version == r1.Version {
		t.Fatal("caso B devolveu o mesmo id da versão antiga")
	}

	// A versão antiga continua no banco, retirada da corrente e com janela fechada.
	old := upReadPage(t, db, r1.Version)
	if old.isLatest != 0 {
		t.Errorf("versão antiga is_latest = %d, esperado 0", old.isLatest)
	}
	if !old.validTo.Valid || old.validTo.Int64 != at2.UnixMicro() {
		t.Errorf("versão antiga valid_to = %v, esperado %d", old.validTo, at2.UnixMicro())
	}
	if old.supersededAt.Valid {
		t.Errorf("versão antiga superseded_at = %d: superseded_at é só lápide de decaimento", old.supersededAt.Int64)
	}
	if old.body != "corpo v1" {
		t.Errorf("corpo da versão antiga = %q: supersede não pode alterar a versão perdedora", old.body)
	}

	// A nova versão é a corrente, aponta para a antiga e abre a janela em at2.
	nova := upReadPage(t, db, r2.Version)
	if nova.isLatest != 1 {
		t.Errorf("versão nova is_latest = %d, esperado 1", nova.isLatest)
	}
	if !bytes.Equal(nova.supersedes, r1.Version.Bytes()) {
		t.Errorf("versão nova supersedes = %x, esperado %x", nova.supersedes, r1.Version.Bytes())
	}
	if !nova.validFrom.Valid || nova.validFrom.Int64 != at2.UnixMicro() {
		t.Errorf("versão nova valid_from = %v, esperado %d", nova.validFrom, at2.UnixMicro())
	}
	if nova.validTo.Valid {
		t.Errorf("versão nova valid_to = %d, esperado NULL (aberta)", nova.validTo.Int64)
	}

	if n := upCount(t, db, `SELECT COUNT(*) FROM pages WHERE path = ?`, "notas/rota.md"); n != 2 {
		t.Errorf("linhas do caminho = %d, esperadas 2 (supersede não destrói)", n)
	}

	// Janela de entidade: fechada na antiga, aberta na nova, ambas com o instante da supersessão.
	if _, sup, found := upWindow(t, db, "postgres", r1.Version); !found || !sup.Valid || sup.Int64 != at2.UnixMicro() {
		t.Errorf("janela da entidade na versão antiga: superseded_at=%v encontrada=%v, esperado %d",
			sup, found, at2.UnixMicro())
	}
	if from, sup, found := upWindow(t, db, "postgres", r2.Version); !found || from != at2.UnixMicro() || sup.Valid {
		t.Errorf("janela da entidade na versão nova: valid_from=%d superseded_at=%v encontrada=%v", from, sup, found)
	}

	if n := upCount(t, db, `SELECT COUNT(*) FROM audit_log WHERE op = ? AND page_id = ?`,
		auditPageSupersede, r2.Version.Bytes()); n != 1 {
		t.Errorf("auditoria page-supersede = %d, esperada 1", n)
	}
	if n := upCount(t, db, `SELECT COUNT(*) FROM page_evidence WHERE page_id = ? AND source_id = ?`,
		r2.Version.Bytes(), "sess-2"); n != 1 {
		t.Errorf("evidência da versão nova = %d, esperada 1", n)
	}
}

func TestUpsertCasoC_PrimeiraVersao(t *testing.T) {
	db, w, ws := upEnv(t)
	scope := upScope(t, w, ws, "ws-c", "pj-c")
	path := upPath(t, "Notas/Rota-Nova.md")
	ref := domain.NewPageRef(scope, path)

	in := upIn(t, ref, "corpo da rota", "sess-1", upT0)
	in.Pinned = true
	res := upMustUpsert(t, w, in)

	if res.Outcome != UpsertCreated {
		t.Fatalf("outcome %q, esperado %q", res.Outcome, UpsertCreated)
	}
	f := upReadPage(t, db, res.Version)
	if f.isLatest != 1 || f.supersedes != nil {
		t.Errorf("primeira versão: is_latest=%d supersedes=%x, esperado 1 e NULL", f.isLatest, f.supersedes)
	}
	if !f.validFrom.Valid || f.validFrom.Int64 != upT0.UnixMicro() || f.validTo.Valid {
		t.Errorf("janela: valid_from=%v valid_to=%v, esperado %d e NULL", f.validFrom, f.validTo, upT0.UnixMicro())
	}
	wantSHA := sha256.Sum256([]byte("corpo da rota"))
	if !bytes.Equal(f.bodySHA, wantSHA[:]) {
		t.Errorf("body_sha256 = %x, esperado SHA-256 do corpo %x", f.bodySHA, wantSHA)
	}
	if f.pathSearch != path.SearchText() {
		t.Errorf("path_search = %q, esperado %q", f.pathSearch, path.SearchText())
	}
	if f.tier != string(domain.TierSemantic) || f.pinned != 1 || f.title != "Rota" {
		t.Errorf("tier=%q pinned=%d title=%q", f.tier, f.pinned, f.title)
	}

	fm, err := decodeFrontmatter(f.fmJSON)
	if err != nil {
		t.Fatalf("decodeFrontmatter: %v", err)
	}
	if kind, ok := fm.Get(kindKey); !ok || kind != "note" {
		t.Errorf("kind no frontmatter = %v (%v), esperado note", kind, ok)
	}

	if n := upCount(t, db, `SELECT COUNT(*) FROM audit_log WHERE op = ? AND page_id = ?`,
		auditPageCreate, res.Version.Bytes()); n != 1 {
		t.Errorf("auditoria page-create = %d, esperada 1", n)
	}
}

func TestUpsertCasoC_ColisaoDeCaixaDevolveErrPathCollision(t *testing.T) {
	db, w, ws := upEnv(t)
	scope := upScope(t, w, ws, "ws-c", "pj-c")

	upMustUpsert(t, w, upIn(t, domain.NewPageRef(scope, upPath(t, "Notas/Rota.md")), "corpo", "sess-1", upT0))
	antes := upTake(t, db)

	_, err := UpsertPage(context.Background(), w,
		upIn(t, domain.NewPageRef(scope, upPath(t, "notas/rota.md")), "outro corpo", "sess-2", upT0.Add(time.Minute)))
	if !errors.Is(err, ErrPathCollision) {
		t.Fatalf("erro = %v, esperado ErrPathCollision", err)
	}

	depois := upTake(t, db)
	if depois != antes {
		t.Errorf("colisão alterou o banco: antes %+v, depois %+v", antes, depois)
	}
	if n := upCount(t, db, `SELECT COUNT(*) FROM pages WHERE path = ?`, "notas/rota.md"); n != 0 {
		t.Errorf("caminho colidente foi gravado (%d linhas)", n)
	}
}

func TestUpsertCasoC_MesmoCaminhoOutroProjetoAceito(t *testing.T) {
	db, w, ws := upEnv(t)
	pj1 := upScope(t, w, ws, "ws-c", "pj-1")
	pj2 := upScope(t, w, ws, "ws-c", "pj-2")

	r1 := upMustUpsert(t, w, upIn(t, domain.NewPageRef(pj1, upPath(t, "Notas/Rota.md")), "corpo 1", "sess-1", upT0))
	r2 := upMustUpsert(t, w, upIn(t, domain.NewPageRef(pj2, upPath(t, "notas/rota.md")), "corpo 2", "sess-2", upT0))

	if r1.Outcome != UpsertCreated || r2.Outcome != UpsertCreated {
		t.Fatalf("outcomes %q e %q, esperado created nos dois", r1.Outcome, r2.Outcome)
	}
	if n := upCount(t, db, `SELECT COUNT(*) FROM pages WHERE is_latest = 1`); n != 2 {
		t.Errorf("versões correntes = %d, esperadas 2 (uma por projeto)", n)
	}
}

func TestUpsertCasoC_RessuscitaTombstone(t *testing.T) {
	db, w, ws := upEnv(t)
	scope := upScope(t, w, ws, "ws-c", "pj-c")
	ref := domain.NewPageRef(scope, upPath(t, "notas/rota.md"))

	at1 := upT0
	at2 := upT0.Add(time.Hour)
	at3 := upT0.Add(2 * time.Hour)

	r1 := upMustUpsert(t, w, upIn(t, ref, "corpo v1", "sess-1", at1))

	// Simula a lápide de decaimento: is_latest = 0 com superseded_at, e janelas de entidade
	// fechadas no instante da retirada, como o caminho de runtime faz.
	if err := w.Do(context.Background(), func(tx *sql.Tx) error {
		if _, err := tx.Exec(`UPDATE pages SET is_latest = 0, superseded_at = ? WHERE id = ?`,
			at2.UnixMicro(), r1.Version.Bytes()); err != nil {
			return err
		}
		return CloseEntityLinksTx(tx, r1.Version, at2.UnixMicro())
	}); err != nil {
		t.Fatalf("simular lápide: %v", err)
	}

	r2 := upMustUpsert(t, w, upIn(t, ref, "corpo v2", "sess-2", at3))

	if r2.Outcome != UpsertResurrected {
		t.Fatalf("outcome %q, esperado %q", r2.Outcome, UpsertResurrected)
	}
	if n := upCount(t, db, `SELECT COUNT(*) FROM pages WHERE path = ?`, "notas/rota.md"); n != 2 {
		t.Errorf("linhas do caminho = %d, esperadas 2: a lápide não é apagada", n)
	}
	if n := upCount(t, db, `SELECT COUNT(*) FROM pages WHERE path = ? AND is_latest = 1`, "notas/rota.md"); n != 1 {
		t.Errorf("versões correntes = %d, esperada 1", n)
	}

	tomb := upReadPage(t, db, r1.Version)
	if tomb.isLatest != 0 || !tomb.supersededAt.Valid || tomb.supersededAt.Int64 != at2.UnixMicro() {
		t.Errorf("lápide alterada: is_latest=%d superseded_at=%v", tomb.isLatest, tomb.supersededAt)
	}
	if !tomb.validTo.Valid || tomb.validTo.Int64 != at3.UnixMicro() {
		t.Errorf("lápide valid_to = %v, esperado %d (fecha a janela antes da nova versão)", tomb.validTo, at3.UnixMicro())
	}

	nova := upReadPage(t, db, r2.Version)
	if nova.isLatest != 1 || !bytes.Equal(nova.supersedes, r1.Version.Bytes()) {
		t.Errorf("nova versão: is_latest=%d supersedes=%x, esperado 1 e o id da lápide", nova.isLatest, nova.supersedes)
	}
	if from, sup, found := upWindow(t, db, "postgres", r2.Version); !found || from != at3.UnixMicro() || sup.Valid {
		t.Errorf("janela da entidade na nova versão: valid_from=%d superseded_at=%v encontrada=%v", from, sup, found)
	}
	if n := upCount(t, db, `SELECT COUNT(*) FROM audit_log WHERE op = ? AND page_id = ?`,
		auditPageResurrect, r2.Version.Bytes()); n != 1 {
		t.Errorf("auditoria page-resurrect = %d, esperada 1", n)
	}
}

func TestUpsertLinksPendentesReparadosNaCriacao(t *testing.T) {
	db, w, ws := upEnv(t)
	scope := upScope(t, w, ws, "ws-l", "pj-l")

	a := upIn(t, domain.NewPageRef(scope, upPath(t, "a.md")), "corpo a", "sess-1", upT0)
	a.Links = []Link{
		{ToPath: upPath(t, "b.md")},
		{ToPath: upPath(t, "c.md"), Relation: "inventada"},
	}
	resA := upMustUpsert(t, w, a)

	if len(resA.Avisos) != 1 || resA.Avisos[0] != "inventada" {
		t.Errorf("avisos = %v, esperado [inventada] (relação desconhecida vira references)", resA.Avisos)
	}
	if n := upCount(t, db, `SELECT COUNT(*) FROM links WHERE from_page_id = ? AND to_page_id IS NULL`,
		resA.Version.Bytes()); n != 2 {
		t.Fatalf("links pendentes antes da criação de b.md = %d, esperados 2", n)
	}

	resB := upMustUpsert(t, w, upIn(t, domain.NewPageRef(scope, upPath(t, "b.md")), "corpo b", "sess-2", upT0.Add(time.Minute)))

	var resolvido []byte
	if err := db.QueryRow(
		`SELECT to_page_id FROM links WHERE from_page_id = ? AND to_path = ?`,
		resA.Version.Bytes(), "b.md",
	).Scan(&resolvido); err != nil {
		t.Fatalf("ler link para b.md: %v", err)
	}
	if !bytes.Equal(resolvido, resB.Version.Bytes()) {
		t.Errorf("link para b.md aponta para %x, esperado a nova versão de b.md %x", resolvido, resB.Version.Bytes())
	}
	if n := upCount(t, db, `SELECT COUNT(*) FROM links WHERE from_page_id = ? AND to_page_id IS NULL`,
		resA.Version.Bytes()); n != 1 {
		t.Errorf("links ainda pendentes = %d, esperado 1 (c.md)", n)
	}
}

func TestUpsertFalhaNoMeioFazRollbackDeTudo(t *testing.T) {
	db, w, ws := upEnv(t)
	scope := upScope(t, w, ws, "ws-r", "pj-r")
	ref := domain.NewPageRef(scope, upPath(t, "notas/rota.md"))

	r1 := upMustUpsert(t, w, upIn(t, ref, "corpo v1", "sess-1", upT0))
	antes := upTake(t, db)

	// Dentro de uma só transação: supersede (escreve), cria outra página (escreve) e falha.
	supersede := upIn(t, ref, "corpo v2", "sess-2", upT0.Add(time.Minute))
	outra := upIn(t, domain.NewPageRef(scope, upPath(t, "outra.md")), "outra", "sess-3", upT0.Add(time.Minute))
	err := w.Do(context.Background(), func(tx *sql.Tx) error {
		if _, err := UpsertPageTx(tx, supersede); err != nil {
			return err
		}
		if _, err := UpsertPageTx(tx, outra); err != nil {
			return err
		}
		return errUpFalhaSimulada
	})
	if !errors.Is(err, errUpFalhaSimulada) {
		t.Fatalf("Do: erro = %v, esperado a falha simulada", err)
	}

	depois := upTake(t, db)
	if depois != antes {
		t.Errorf("rollback incompleto: antes %+v, depois %+v", antes, depois)
	}
	f := upReadPage(t, db, r1.Version)
	if f.isLatest != 1 || f.validTo.Valid {
		t.Errorf("versão corrente após rollback: is_latest=%d valid_to=%v, esperado 1 e NULL", f.isLatest, f.validTo)
	}
	if _, sup, found := upWindow(t, db, "postgres", r1.Version); !found || sup.Valid {
		t.Errorf("janela de entidade fechada após rollback: superseded_at=%v encontrada=%v", sup, found)
	}
	if n := upCount(t, db, `SELECT COUNT(*) FROM pages WHERE path = ?`, "outra.md"); n != 0 {
		t.Errorf("página outra.md sobreviveu ao rollback (%d linhas)", n)
	}
}

func TestUpsertEscritasConcorrentesSupersedemNaoDestroem(t *testing.T) {
	db, w, ws := upEnv(t)
	scope := upScope(t, w, ws, "ws-x", "pj-x")
	ref := domain.NewPageRef(scope, upPath(t, "notas/rota.md"))

	const n = 8
	entradas := make([]UpsertInput, n)
	for i := range entradas {
		entradas[i] = upIn(t, ref, fmt.Sprintf("corpo %d", i), fmt.Sprintf("sess-%d", i), upT0.Add(time.Duration(i)*time.Second))
	}

	var wg sync.WaitGroup
	errs := make(chan error, n)
	for _, in := range entradas {
		wg.Add(1)
		go func(in UpsertInput) {
			defer wg.Done()
			if _, err := UpsertPage(context.Background(), w, in); err != nil {
				errs <- err
			}
		}(in)
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Errorf("escrita concorrente falhou: %v", err)
	}

	if got := upCount(t, db, `SELECT COUNT(*) FROM pages WHERE path = ?`, "notas/rota.md"); got != n {
		t.Fatalf("versões gravadas = %d, esperadas %d: nenhuma pode ser destruída", got, n)
	}
	if got := upCount(t, db, `SELECT COUNT(*) FROM pages WHERE path = ? AND is_latest = 1`, "notas/rota.md"); got != 1 {
		t.Fatalf("versões correntes = %d, esperada 1", got)
	}

	// A cadeia de supersedes a partir da corrente alcança todas as versões.
	var cur []byte
	if err := db.QueryRow(`SELECT id FROM pages WHERE path = ? AND is_latest = 1`, "notas/rota.md").Scan(&cur); err != nil {
		t.Fatalf("ler corrente: %v", err)
	}
	alcancadas := 0
	for cur != nil && alcancadas <= n {
		alcancadas++
		var sup []byte
		if err := db.QueryRow(`SELECT supersedes FROM pages WHERE id = ?`, cur).Scan(&sup); err != nil {
			t.Fatalf("seguir cadeia: %v", err)
		}
		cur = sup
	}
	if alcancadas != n {
		t.Errorf("cadeia a partir da corrente tem %d versões, esperadas %d", alcancadas, n)
	}
}

func TestUpsertEntradaInvalidaFalhaSemPanico(t *testing.T) {
	db, w, ws := upEnv(t)
	scope := upScope(t, w, ws, "ws-v", "pj-v")
	ctx := context.Background()

	if _, err := UpsertPageTx(nil, upIn(t, domain.NewPageRef(scope, upPath(t, "a.md")), "x", "s", upT0)); err == nil {
		t.Error("transação nula: esperado erro")
	}

	semFonte := upIn(t, domain.NewPageRef(scope, upPath(t, "a.md")), "x", "", upT0)
	if _, err := UpsertPage(ctx, w, semFonte); err == nil {
		t.Error("fonte de evidência vazia: esperado erro")
	}

	semCaminho := upIn(t, domain.PageRef{Scope: scope}, "x", "s", upT0)
	if _, err := UpsertPage(ctx, w, semCaminho); err == nil {
		t.Error("caminho vazio: esperado erro")
	}

	semInstante := upIn(t, domain.NewPageRef(scope, upPath(t, "a.md")), "x", "s", time.Time{})
	if _, err := UpsertPage(ctx, w, semInstante); err == nil {
		t.Error("instante zero: esperado erro")
	}

	if n := upCount(t, db, `SELECT COUNT(*) FROM pages`); n != 0 {
		t.Errorf("entradas inválidas gravaram %d páginas", n)
	}
}

// TestUpsertSupersessaoReapontaLinksDeEntrada: uma página que aponta para outra deve continuar
// apontando para a versão corrente depois da supersessão. Os vizinhos do grafo usam to_page_id,
// então um link que aponta para a versão antiga some da busca em grafo (T081).
func TestUpsertSupersessaoReapontaLinksDeEntrada(t *testing.T) {
	db, w, ws := upEnv(t)
	scope := upScope(t, w, ws, "ws-link", "pj-link")
	alvo := domain.NewPageRef(scope, upPath(t, "notas/rota.md"))
	origem := domain.NewPageRef(scope, upPath(t, "notas/a.md"))

	upMustUpsert(t, w, upIn(t, alvo, "corpo v1", "sess-1", upT0))
	in := upIn(t, origem, "veja [[rota]]", "sess-1", upT0)
	in.Links = []Link{{ToPath: upPath(t, "notas/rota.md")}}
	rOrigem := upMustUpsert(t, w, in)

	r2 := upMustUpsert(t, w, upIn(t, alvo, "corpo v2", "sess-2", upT0.Add(time.Hour)))
	if r2.Outcome != UpsertSuperseded {
		t.Fatalf("outcome %q, esperado %q", r2.Outcome, UpsertSuperseded)
	}

	var destino []byte
	if err := db.QueryRow(
		`SELECT to_page_id FROM links WHERE from_page_id = ? AND to_path = ?`,
		rOrigem.Version.Bytes(), "notas/rota.md",
	).Scan(&destino); err != nil {
		t.Fatalf("ler link de entrada: %v", err)
	}
	if string(destino) != string(r2.Version.Bytes()) {
		t.Fatalf("link aponta para %x, esperado a versão corrente %x", destino, r2.Version.Bytes())
	}
}
