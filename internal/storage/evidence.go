package storage

import (
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/phm-aguiar/ai-memory-go/internal/domain"
)

// SourceKind é o tipo fechado de fonte de evidência (DV §6.4, DB §4). O banco
// recusa qualquer outro valor pelo CHECK de page_evidence.source_kind.
type SourceKind string

const (
	// SourceSession é uma sessão consolidada que produziu ou tocou a página.
	SourceSession SourceKind = "session"
	// SourceObservation é uma observação citada diretamente.
	SourceObservation SourceKind = "observation"
	// SourceFeedback é um feedback de usuário ou agente que reafirmou a página.
	SourceFeedback SourceKind = "feedback"
	// SourceReconsolidation é uma passagem de reconsolidação que reescreveu ou reconfirmou a página.
	SourceReconsolidation SourceKind = "reconsolidation"
)

// InsertEvidence grava uma fonte de evidência da página dentro de tx, a transação
// da própria escrita da página (INV-03). Não abre transação nem faz I/O fora de tx.
//
// A chave é (page_id, source_kind, source_id): citar a mesma fonte outra vez não
// gera linha nova e não é erro. A evidência é só registrada e exposta; não entra no
// ranqueamento (DV §6.4). Um source_kind fora dos valores de SourceKind falha no
// CHECK do banco, e o erro volta com %w.
func InsertEvidence(tx *sql.Tx, page domain.PageID, kind SourceKind, sourceID string, at time.Time) error {
	if tx == nil {
		return errors.New("insert page evidence: transação nula")
	}
	_, err := tx.Exec(
		`INSERT INTO page_evidence (page_id, source_kind, source_id, created_at)
		 VALUES (?, ?, ?, ?)
		 ON CONFLICT(page_id, source_kind, source_id) DO NOTHING`,
		page.Bytes(), string(kind), sourceID, at.UnixMicro(),
	)
	if err != nil {
		return fmt.Errorf("insert page evidence: %w", err)
	}
	return nil
}
