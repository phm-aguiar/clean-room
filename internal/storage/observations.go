package storage

import (
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/phm-aguiar/ai-memory-go/internal/domain"
)

// InsertObservation grava obs dentro de tx, a transação aberta por Writer.Do (INV-02).
// Não abre transação e não faz I/O fora de tx.
//
// Só aceita domain.Observation. Title e Body são sanitize.Sanitized, e o único jeito de
// criar um valor com conteúdo é sanitize.Title ou sanitize.Body (INV-06). Não há assinatura
// que receba texto cru. Como os campos da struct são exportados, a função também confere o
// que NewObservation já garante: importância de 1 a 10 e kind canônico. A confirmação fica
// aqui, antes do banco, para que um valor montado à mão não chegue ao CHECK.
//
// Gravação:
//   - title e body vêm de Sanitized.Text();
//   - kind é gravado na forma canônica;
//   - created_at recebe OccurredAt em microssegundos, ou time.Now().UnixMicro() quando OccurredAt
//     é zero. O esquema não tem coluna occurred_at: o instante resolvido vai para created_at;
//   - extension e source_event vazios são gravados como NULL.
//
// Erros voltam com %w: ErrInvalidImportance e ErrInvalidObservationKind são reconhecíveis com errors.Is.
func InsertObservation(tx *sql.Tx, obs domain.Observation) error {
	if tx == nil {
		return errors.New("insert observation: transação nula")
	}
	if obs.Importance < domain.MinImportance || obs.Importance > domain.MaxImportance {
		return fmt.Errorf("insert observation: %w: %d", domain.ErrInvalidImportance, obs.Importance)
	}
	canon, err := domain.ParseObservationKind(string(obs.Kind))
	if err != nil {
		return fmt.Errorf("insert observation: %w", err)
	}
	if canon != obs.Kind {
		return fmt.Errorf("insert observation: kind %q não é o canônico %q: %w",
			obs.Kind, canon, domain.ErrInvalidObservationKind)
	}

	createdAt := obs.OccurredAt
	if createdAt == 0 {
		createdAt = time.Now().UnixMicro()
	}

	var extension, sourceEvent any
	if obs.Extension != "" {
		extension = obs.Extension
	}
	if obs.SourceEvent != "" {
		sourceEvent = obs.SourceEvent
	}

	_, err = tx.Exec(
		`INSERT INTO observations
		   (id, session_id, workspace_id, project_id, kind, title, body, importance, created_at, extension, source_event)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		obs.ID.Bytes(),
		obs.SessionID.Bytes(),
		obs.Scope.WorkspaceID.Bytes(),
		obs.Scope.ProjectID.Bytes(),
		string(canon),
		obs.Title.Text(),
		obs.Body.Text(),
		obs.Importance,
		createdAt,
		extension,
		sourceEvent,
	)
	if err != nil {
		return fmt.Errorf("insert observation: %w", err)
	}
	return nil
}
