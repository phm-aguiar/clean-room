package storage

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/phm-aguiar/ai-memory-go/internal/domain"
)

var eaT0 = time.Date(2026, 10, 12, 15, 30, 0, 123456000, time.UTC)

// eaFixture é um banco com a migração real, um escopo semeado, uma página existente
// (page_evidence exige a FK) e um Writer real.
type eaFixture struct {
	db    *sql.DB
	w     Writer
	scope domain.Scope
	page  domain.PageID
}

func newEAFixture(t *testing.T) eaFixture {
	t.Helper()
	db := openSchemaDB(t)
	s := seedSchemaScope(t, db)

	pageBytes := schemaID(0x41)
	if err := insertSchemaPage(db, pageBytes, s.ws1, s.proj1, "evidencia.md", "Evidencia", "corpo", 1); err != nil {
		t.Fatalf("insert page: %v", err)
	}

	w, err := NewWriter(context.Background(), db)
	if err != nil {
		t.Fatalf("NewWriter: %v", err)
	}
	t.Cleanup(func() { _ = w.Close() })

	ws, err := domain.ParseIDBytes(s.ws1)
	if err != nil {
		t.Fatalf("ParseIDBytes ws: %v", err)
	}
	pj, err := domain.ParseIDBytes(s.proj1)
	if err != nil {
		t.Fatalf("ParseIDBytes proj: %v", err)
	}
	pg, err := domain.ParseIDBytes(pageBytes)
	if err != nil {
		t.Fatalf("ParseIDBytes page: %v", err)
	}
	return eaFixture{
		db:    db,
		w:     w,
		scope: domain.NewScope(domain.WorkspaceID(ws), domain.ProjectID(pj)),
		page:  domain.PageID(pg),
	}
}

func eaDo(t *testing.T, w Writer, fn func(*sql.Tx) error) {
	t.Helper()
	if err := w.Do(context.Background(), fn); err != nil {
		t.Fatalf("Do: %v", err)
	}
}

func TestEvidenceGravaCadaSourceKindValido(t *testing.T) {
	fx := newEAFixture(t)
	kinds := []SourceKind{SourceSession, SourceObservation, SourceFeedback, SourceReconsolidation}

	for _, k := range kinds {
		src := "src-" + string(k)
		eaDo(t, fx.w, func(tx *sql.Tx) error {
			return InsertEvidence(tx, fx.page, k, src, eaT0)
		})
		n := countSchemaRows(t, fx.db,
			`SELECT COUNT(*) FROM page_evidence WHERE page_id = ? AND source_kind = ? AND source_id = ? AND created_at = ?`,
			fx.page.Bytes(), string(k), src, eaT0.UnixMicro())
		if n != 1 {
			t.Fatalf("kind %q: linhas = %d, want 1", k, n)
		}
	}

	if n := countSchemaRows(t, fx.db, `SELECT COUNT(*) FROM page_evidence`); n != len(kinds) {
		t.Fatalf("page_evidence total = %d, want %d", n, len(kinds))
	}
}

func TestEvidenceRecusaSourceKindForaDosQuatro(t *testing.T) {
	fx := newEAFixture(t)

	casos := []SourceKind{"outro", "", "Session", "message"}
	for _, k := range casos {
		err := fx.w.Do(context.Background(), func(tx *sql.Tx) error {
			return InsertEvidence(tx, fx.page, k, "x", eaT0)
		})
		if err == nil {
			t.Fatalf("source_kind %q: aceito, want recusado pelo CHECK", k)
		}
		if !strings.Contains(err.Error(), "CHECK") {
			t.Fatalf("source_kind %q: erro = %v, want falha de CHECK", k, err)
		}
	}

	if n := countSchemaRows(t, fx.db, `SELECT COUNT(*) FROM page_evidence`); n != 0 {
		t.Fatalf("page_evidence = %d após recusas, want 0", n)
	}
}

func TestEvidenceDuplicataNaoSeRepete(t *testing.T) {
	fx := newEAFixture(t)

	// Mesma fonte em duas transações e de novo na mesma transação: sem erro e sem linha extra.
	eaDo(t, fx.w, func(tx *sql.Tx) error {
		return InsertEvidence(tx, fx.page, SourceSession, "s1", eaT0)
	})
	eaDo(t, fx.w, func(tx *sql.Tx) error {
		if err := InsertEvidence(tx, fx.page, SourceSession, "s1", eaT0.Add(time.Second)); err != nil {
			return err
		}
		return InsertEvidence(tx, fx.page, SourceSession, "s1", eaT0.Add(2*time.Second))
	})

	if n := countSchemaRows(t, fx.db,
		`SELECT COUNT(*) FROM page_evidence WHERE page_id = ? AND source_kind = ? AND source_id = ?`,
		fx.page.Bytes(), string(SourceSession), "s1"); n != 1 {
		t.Fatalf("duplicata gravada: linhas = %d, want 1", n)
	}

	// O created_at da primeira gravação é o que fica.
	var created int64
	if err := fx.db.QueryRow(
		`SELECT created_at FROM page_evidence WHERE page_id = ? AND source_kind = ? AND source_id = ?`,
		fx.page.Bytes(), string(SourceSession), "s1").Scan(&created); err != nil {
		t.Fatalf("ler created_at: %v", err)
	}
	if created != eaT0.UnixMicro() {
		t.Fatalf("created_at = %d, want primeiro valor %d", created, eaT0.UnixMicro())
	}

	// Outra fonte, e a mesma fonte com outro tipo, são linhas distintas.
	eaDo(t, fx.w, func(tx *sql.Tx) error {
		if err := InsertEvidence(tx, fx.page, SourceSession, "s2", eaT0); err != nil {
			return err
		}
		return InsertEvidence(tx, fx.page, SourceObservation, "s1", eaT0)
	})
	if n := countSchemaRows(t, fx.db, `SELECT COUNT(*) FROM page_evidence`); n != 3 {
		t.Fatalf("page_evidence total = %d, want 3", n)
	}
}

func TestAuditGravaEscopoEInstante(t *testing.T) {
	fx := newEAFixture(t)
	page := fx.page

	eaDo(t, fx.w, func(tx *sql.Tx) error {
		return InsertAudit(tx, eaT0, AuditEvent{Op: "page.write", Scope: fx.scope, Page: &page})
	})

	var at int64
	var op string
	var ws, pj, pg []byte
	if err := fx.db.QueryRow(
		`SELECT at, op, workspace_id, project_id, page_id FROM audit_log WHERE op = ?`, "page.write",
	).Scan(&at, &op, &ws, &pj, &pg); err != nil {
		t.Fatalf("ler audit_log: %v", err)
	}
	if at != eaT0.UnixMicro() {
		t.Fatalf("at = %d, want %d (microssegundos)", at, eaT0.UnixMicro())
	}
	if op != "page.write" {
		t.Fatalf("op = %q, want page.write", op)
	}
	if !bytes.Equal(ws, fx.scope.WorkspaceID.Bytes()) {
		t.Fatalf("workspace_id = %x, want %x", ws, fx.scope.WorkspaceID.Bytes())
	}
	if !bytes.Equal(pj, fx.scope.ProjectID.Bytes()) {
		t.Fatalf("project_id = %x, want %x", pj, fx.scope.ProjectID.Bytes())
	}
	if !bytes.Equal(pg, page.Bytes()) {
		t.Fatalf("page_id = %x, want %x", pg, page.Bytes())
	}
}

func TestAuditSemAlvoGravaPageIDNulo(t *testing.T) {
	fx := newEAFixture(t)

	eaDo(t, fx.w, func(tx *sql.Tx) error {
		return InsertAudit(tx, eaT0, AuditEvent{Op: "scope.ensure", Scope: fx.scope})
	})

	if n := countSchemaRows(t, fx.db,
		`SELECT COUNT(*) FROM audit_log WHERE op = ? AND page_id IS NULL AND workspace_id = ? AND project_id = ?`,
		"scope.ensure", fx.scope.WorkspaceID.Bytes(), fx.scope.ProjectID.Bytes()); n != 1 {
		t.Fatalf("linha sem alvo: %d, want 1 com page_id NULL", n)
	}
}

func TestEvidenceAuditMesmaTransacaoCommitGravaAmbas(t *testing.T) {
	fx := newEAFixture(t)
	page := fx.page

	eaDo(t, fx.w, func(tx *sql.Tx) error {
		if err := InsertEvidence(tx, page, SourceFeedback, "fb-1", eaT0); err != nil {
			return err
		}
		return InsertAudit(tx, eaT0, AuditEvent{Op: "page.write", Scope: fx.scope, Page: &page})
	})

	if n := countSchemaRows(t, fx.db, `SELECT COUNT(*) FROM page_evidence WHERE source_id = ?`, "fb-1"); n != 1 {
		t.Fatalf("evidência após commit = %d, want 1", n)
	}
	if n := countSchemaRows(t, fx.db, `SELECT COUNT(*) FROM audit_log WHERE op = ?`, "page.write"); n != 1 {
		t.Fatalf("auditoria após commit = %d, want 1", n)
	}
}

func TestEvidenceAuditRollbackDesfazAsDuasGravacoes(t *testing.T) {
	fx := newEAFixture(t)
	page := fx.page
	errForcado := errors.New("falha forçada depois da escrita")

	err := fx.w.Do(context.Background(), func(tx *sql.Tx) error {
		if err := InsertEvidence(tx, page, SourceReconsolidation, "rc-1", eaT0); err != nil {
			return err
		}
		if err := InsertAudit(tx, eaT0, AuditEvent{Op: "page.write", Scope: fx.scope, Page: &page}); err != nil {
			return err
		}
		return errForcado
	})
	if !errors.Is(err, errForcado) {
		t.Fatalf("Do = %v, want %v", err, errForcado)
	}

	if n := countSchemaRows(t, fx.db, `SELECT COUNT(*) FROM page_evidence`); n != 0 {
		t.Fatalf("evidência após rollback = %d, want 0", n)
	}
	if n := countSchemaRows(t, fx.db, `SELECT COUNT(*) FROM audit_log`); n != 0 {
		t.Fatalf("auditoria após rollback = %d, want 0", n)
	}
}

func TestEvidenceFalhaDepoisDeAuditoriaRevertaAuditoria(t *testing.T) {
	fx := newEAFixture(t)
	page := fx.page

	// A auditoria entra primeiro; a evidência com source_kind inválido falha no CHECK.
	// O ROLLBACK da transação tem de levar a auditoria junto (INV-03).
	err := fx.w.Do(context.Background(), func(tx *sql.Tx) error {
		if err := InsertAudit(tx, eaT0, AuditEvent{Op: "page.write", Scope: fx.scope, Page: &page}); err != nil {
			return err
		}
		return InsertEvidence(tx, page, SourceKind("invalido"), "x", eaT0)
	})
	if err == nil {
		t.Fatal("Do aceitou source_kind inválido, want erro")
	}
	if n := countSchemaRows(t, fx.db, `SELECT COUNT(*) FROM audit_log`); n != 0 {
		t.Fatalf("auditoria sobreviveu à falha = %d linhas, want 0", n)
	}
}

func TestEvidenceAuditTxNulaRetornaErroSemPanico(t *testing.T) {
	fx := newEAFixture(t)
	page := fx.page

	if err := InsertEvidence(nil, page, SourceSession, "s1", eaT0); err == nil {
		t.Fatal("InsertEvidence(nil tx) = nil, want erro")
	}
	if err := InsertAudit(nil, eaT0, AuditEvent{Op: "page.write", Scope: fx.scope}); err == nil {
		t.Fatal("InsertAudit(nil tx) = nil, want erro")
	}
}
