package storage

import (
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/phm-aguiar/ai-memory-go/internal/domain"
)

// Chave de ingestão (docs/database-schema.md §2, tabela ingest_keys; docs/lifecycle-event-ingestion.md
// §8 passo 13 e §8.1; docs/desktop/capture-and-dedup.md §4.3; docs/complementary-contracts.md §1.3).
//
// Estados de uma chave, derivados da linha em ingest_keys:
//   - new: não há linha para (projeto, chave);
//   - pending: a linha existe com completed_at NULL (reivindicada, efeitos a jusante pendentes);
//   - completed: a linha existe com completed_at preenchido.
//
// Todas as funções *Tx rodam dentro de uma transação de escrita (Writer.Do). Timestamps são
// microssegundos Unix UTC, como nas demais tabelas da captura.

// IngestKeyRetention é a validade de uma chave concluída (30 dias, docs/lifecycle-event-ingestion.md §8).
const IngestKeyRetention = 30 * 24 * time.Hour

// ErrIngestKeyNotClaimed é devolvido ao concluir uma chave que não foi reivindicada (estado new).
var ErrIngestKeyNotClaimed = errors.New("chave de ingestão não reivindicada")

// ClaimIngestKeyTx reivindica a chave para o projeto: new -> pending. Devolve true quando esta
// chamada criou a linha (a chave era new) e false quando a chave já existe em qualquer estado
// (pending ou completed), caso em que nada é alterado. A inserção é um único comando SQL com
// ON CONFLICT DO NOTHING, então a reivindicação é atômica mesmo com chamadas concorrentes:
// só uma vence.
func ClaimIngestKeyTx(tx *sql.Tx, project domain.ProjectID, key string, at time.Time) (bool, error) {
	if tx == nil {
		return false, errors.New("reivindicar chave: transação nula")
	}
	if key == "" {
		return false, errors.New("reivindicar chave: chave vazia")
	}
	if at.IsZero() {
		at = time.Now()
	}
	res, err := tx.Exec(
		`INSERT INTO ingest_keys (project_id, key, seen_at, completed_at) VALUES (?, ?, ?, NULL)
		 ON CONFLICT (project_id, key) DO NOTHING`,
		project.Bytes(), key, at.UnixMicro(),
	)
	if err != nil {
		return false, fmt.Errorf("reivindicar chave: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("reivindicar chave: linhas afetadas: %w", err)
	}
	return n == 1, nil
}

// CompleteIngestKeyTx conclui a chave: pending -> completed. Devolve true quando esta chamada fez
// a transição e false quando a chave já estava completed (no-op idempotente). Chave sem linha
// (estado new) devolve ErrIngestKeyNotClaimed: concluir exige reivindicação prévia.
func CompleteIngestKeyTx(tx *sql.Tx, project domain.ProjectID, key string, at time.Time) (bool, error) {
	if tx == nil {
		return false, errors.New("concluir chave: transação nula")
	}
	if at.IsZero() {
		at = time.Now()
	}

	res, err := tx.Exec(
		`UPDATE ingest_keys SET completed_at = ?
		 WHERE project_id = ? AND key = ? AND completed_at IS NULL`,
		at.UnixMicro(), project.Bytes(), key,
	)
	if err != nil {
		return false, fmt.Errorf("concluir chave: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("concluir chave: linhas afetadas: %w", err)
	}
	if n == 1 {
		return true, nil
	}

	var exists int
	if err := tx.QueryRow(
		`SELECT COUNT(*) FROM ingest_keys WHERE project_id = ? AND key = ?`,
		project.Bytes(), key,
	).Scan(&exists); err != nil {
		return false, fmt.Errorf("concluir chave: verificação: %w", err)
	}
	if exists == 0 {
		return false, ErrIngestKeyNotClaimed
	}
	return false, nil
}

// IngestKeyState é o estado de uma chave de ingestão, derivado da linha em ingest_keys.
type IngestKeyState int

const (
	// IngestKeyNew: não há linha para (projeto, chave).
	IngestKeyNew IngestKeyState = iota
	// IngestKeyPending: reivindicada, com efeitos a jusante pendentes (completed_at NULL).
	IngestKeyPending
	// IngestKeyCompleted: efeitos a jusante concluídos (completed_at preenchido).
	IngestKeyCompleted
)

// String devolve o nome do estado, para mensagens e logs.
func (s IngestKeyState) String() string {
	switch s {
	case IngestKeyNew:
		return "new"
	case IngestKeyPending:
		return "pending"
	case IngestKeyCompleted:
		return "completed"
	default:
		return fmt.Sprintf("IngestKeyState(%d)", int(s))
	}
}

// IngestKeyStateTx lê o estado da chave para o projeto dentro da transação, sem alterar a linha.
// Chave sem linha devolve IngestKeyNew. Deve ser chamada na mesma transação de ClaimIngestKeyTx
// quando a decisão depender do estado (gate de processamento).
func IngestKeyStateTx(tx *sql.Tx, project domain.ProjectID, key string) (IngestKeyState, error) {
	if tx == nil {
		return IngestKeyNew, errors.New("ler estado da chave: transação nula")
	}
	if key == "" {
		return IngestKeyNew, errors.New("ler estado da chave: chave vazia")
	}
	var completedAt sql.NullInt64
	err := tx.QueryRow(
		`SELECT completed_at FROM ingest_keys WHERE project_id = ? AND key = ?`,
		project.Bytes(), key,
	).Scan(&completedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return IngestKeyNew, nil
	}
	if err != nil {
		return IngestKeyNew, fmt.Errorf("ler estado da chave: %w", err)
	}
	if completedAt.Valid {
		return IngestKeyCompleted, nil
	}
	return IngestKeyPending, nil
}

// PruneIngestKeysTx remove as chaves concluídas cuja última vista (seen_at) é anterior a
// now - IngestKeyRetention, ou seja, com mais de 30 dias. O relógio é recebido por parâmetro.
// Chaves pending nunca são removidas: removê-las permitiria reexecutar efeitos ainda não
// concluídos. Devolve o número de chaves removidas.
func PruneIngestKeysTx(tx *sql.Tx, now time.Time) (int64, error) {
	if tx == nil {
		return 0, errors.New("limpar chaves de ingestão: transação nula")
	}
	cutoff := now.Add(-IngestKeyRetention).UnixMicro()
	res, err := tx.Exec(
		`DELETE FROM ingest_keys WHERE completed_at IS NOT NULL AND seen_at < ?`,
		cutoff,
	)
	if err != nil {
		return 0, fmt.Errorf("limpar chaves de ingestão: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("limpar chaves de ingestão: linhas afetadas: %w", err)
	}
	return n, nil
}
