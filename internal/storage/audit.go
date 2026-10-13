package storage

import (
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/phm-aguiar/ai-memory-go/internal/domain"
)

// AuditEvent é um evento registrado em audit_log (DB §6).
type AuditEvent struct {
	// Op é a ação auditada (coluna op).
	Op string
	// Scope é o escopo em que a ação ocorreu (workspace_id e project_id).
	Scope domain.Scope
	// Page é o alvo da ação, ou nil para gravar page_id NULL.
	Page *domain.PageID
}

// InsertAudit grava uma linha de audit_log dentro de tx, a transação da própria
// escrita (INV-03). at é o instante do evento, gravado em microssegundos Unix UTC.
// Não abre transação nem faz I/O fora de tx. Erros voltam com %w.
func InsertAudit(tx *sql.Tx, at time.Time, ev AuditEvent) error {
	if tx == nil {
		return errors.New("insert audit: transação nula")
	}
	// any para que nil vire NULL no driver; um *domain.PageID não serializa.
	var page any
	if ev.Page != nil {
		page = ev.Page.Bytes()
	}
	_, err := tx.Exec(
		`INSERT INTO audit_log (at, op, workspace_id, project_id, page_id)
		 VALUES (?, ?, ?, ?, ?)`,
		at.UnixMicro(), ev.Op, ev.Scope.WorkspaceID.Bytes(), ev.Scope.ProjectID.Bytes(), page,
	)
	if err != nil {
		return fmt.Errorf("insert audit: %w", err)
	}
	return nil
}
