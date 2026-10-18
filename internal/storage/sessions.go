package storage

import (
	"bytes"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/phm-aguiar/ai-memory-go/internal/domain"
)

// Sessões da captura (docs/lifecycle-event-ingestion.md §8, §8.1 e §8.3; docs/database-schema.md §2).
//
// Tudo aqui é executado dentro de uma transação de escrita (Writer.Do), exceto as leituras
// marcadas. Timestamps de sessão são microssegundos Unix UTC, como nas demais tabelas.

var (
	// ErrSessionNotFound é devolvido quando a sessão pedida não existe.
	ErrSessionNotFound = errors.New("sessão não encontrada")
	// ErrSessionScope é devolvido quando a sessão existe em outro workspace ou projeto.
	// Uma sessão nunca muda de escopo por evento; a reatribuição é outro comando (lifecycle-ops.md).
	ErrSessionScope = errors.New("sessão pertence a outro escopo")
)

// substantiveObservationKinds é a única definição de sessão substantiva (LEI §8.3): uma sessão
// é efêmera quando nenhuma observação destes tipos existe. Tanto IsEphemeral (cálculo em Go)
// quanto SessionIsEphemeral (verificação no armazenamento) leem esta lista.
var substantiveObservationKinds = []domain.ObservationKind{
	domain.ObservationUserPrompt,
	domain.ObservationPreToolUse,
	domain.ObservationPostToolUse,
}

// IsSubstantiveObservationKind diz se um tipo de observação torna a sessão substantiva.
func IsSubstantiveObservationKind(kind domain.ObservationKind) bool {
	for _, k := range substantiveObservationKinds {
		if k == kind {
			return true
		}
	}
	return false
}

// IsEphemeral é a definição única de sessão efêmera (LEI §8.3). Função pura: recebe os tipos de
// observação da sessão e devolve true quando nenhum é prompt do usuário, pré-ferramenta nem
// pós-ferramenta. Uma sessão sem observações é efêmera.
func IsEphemeral(kinds []domain.ObservationKind) bool {
	for _, k := range kinds {
		if IsSubstantiveObservationKind(k) {
			return false
		}
	}
	return true
}

// SessionIsEphemeral aplica a mesma definição de IsEphemeral sobre as observações gravadas da
// sessão. A lista de tipos vem de substantiveObservationKinds, então o SQL e a função pura não
// divergem. Leitura: deve ser chamada dentro da transação que decide o fim da sessão.
// Sessão inexistente devolve ErrSessionNotFound.
func SessionIsEphemeral(tx *sql.Tx, id domain.SessionID) (bool, error) {
	if tx == nil {
		return false, errors.New("sessão efêmera: transação nula")
	}
	var exists int
	if err := tx.QueryRow(`SELECT COUNT(*) FROM sessions WHERE id = ?`, id.Bytes()).Scan(&exists); err != nil {
		return false, fmt.Errorf("sessão efêmera: %w", err)
	}
	if exists == 0 {
		return false, ErrSessionNotFound
	}

	placeholders := make([]string, len(substantiveObservationKinds))
	args := make([]any, 0, len(substantiveObservationKinds)+1)
	args = append(args, id.Bytes())
	for i, k := range substantiveObservationKinds {
		placeholders[i] = "?"
		args = append(args, string(k))
	}
	query := `SELECT EXISTS (SELECT 1 FROM observations WHERE session_id = ? AND kind IN (` +
		strings.Join(placeholders, ", ") + `))`

	var substantive int
	if err := tx.QueryRow(query, args...).Scan(&substantive); err != nil {
		return false, fmt.Errorf("sessão efêmera: %w", err)
	}
	return substantive == 0, nil
}

// SessionOpen são os dados para abrir ou tocar uma sessão. ActorUser vazio grava NULL.
// Cwd vazio grava NULL. At zero usa o instante atual.
type SessionOpen struct {
	ID        domain.SessionID
	Scope     domain.Scope
	AgentKind string
	Cwd       string
	ActorUser string
	At        time.Time
}

// SessionTouch é o resultado de OpenOrTouchSessionTx. Created é true quando a sessão foi criada
// agora. Ended é true quando a sessão já estava encerrada: o toque não a reabre.
type SessionTouch struct {
	Created bool
	Ended   bool
}

// OpenOrTouchSessionTx abre a sessão se ela não existe ou a toca se já existe. É idempotente:
// repetir a mesma chamada não cria outra linha, não altera started_at e não reabre sessão
// encerrada. Só preenche cwd quando a sessão ainda não tem um e a chamada traz um.
// Sessão existente em outro workspace ou projeto devolve ErrSessionScope.
func OpenOrTouchSessionTx(tx *sql.Tx, s SessionOpen) (SessionTouch, error) {
	if tx == nil {
		return SessionTouch{}, errors.New("abrir sessão: transação nula")
	}
	if strings.TrimSpace(s.AgentKind) == "" {
		return SessionTouch{}, errors.New("abrir sessão: agent_kind vazio")
	}
	at := s.At
	if at.IsZero() {
		at = time.Now()
	}

	var wsID, projID []byte
	var endedAt sql.NullInt64
	err := tx.QueryRow(
		`SELECT workspace_id, project_id, ended_at FROM sessions WHERE id = ?`,
		s.ID.Bytes(),
	).Scan(&wsID, &projID, &endedAt)

	if errors.Is(err, sql.ErrNoRows) {
		var cwd, actor any
		if s.Cwd != "" {
			cwd = s.Cwd
		}
		if s.ActorUser != "" {
			actor = s.ActorUser
		}
		_, err := tx.Exec(
			`INSERT INTO sessions (id, workspace_id, project_id, agent_kind, cwd, started_at, actor_user)
			 VALUES (?, ?, ?, ?, ?, ?, ?)`,
			s.ID.Bytes(), s.Scope.WorkspaceID.Bytes(), s.Scope.ProjectID.Bytes(),
			s.AgentKind, cwd, at.UnixMicro(), actor,
		)
		if err != nil {
			return SessionTouch{}, fmt.Errorf("abrir sessão: %w", err)
		}
		return SessionTouch{Created: true}, nil
	}
	if err != nil {
		return SessionTouch{}, fmt.Errorf("tocar sessão: %w", err)
	}

	if !bytes.Equal(wsID, s.Scope.WorkspaceID.Bytes()) || !bytes.Equal(projID, s.Scope.ProjectID.Bytes()) {
		return SessionTouch{}, ErrSessionScope
	}

	if s.Cwd != "" {
		if _, err := tx.Exec(
			`UPDATE sessions SET cwd = ? WHERE id = ? AND cwd IS NULL`,
			s.Cwd, s.ID.Bytes(),
		); err != nil {
			return SessionTouch{}, fmt.Errorf("tocar sessão: cwd: %w", err)
		}
	}
	return SessionTouch{Ended: endedAt.Valid}, nil
}

// SessionCwd devolve o cwd de trabalho gravado da sessão (gravado por OpenOrTouchSessionTx na
// abertura ou no primeiro toque com cwd). Usado pela exceção do fim de sessão no mesmo diretório
// (docs/storage-and-retrieval.md §6.1). Sessão sem cwd gravado devolve "" sem erro. Sessão
// inexistente devolve ErrSessionNotFound. Leitura: deve ser chamada dentro de uma transação.
func SessionCwd(tx *sql.Tx, id domain.SessionID) (string, error) {
	if tx == nil {
		return "", errors.New("cwd da sessão: transação nula")
	}
	var cwd sql.NullString
	err := tx.QueryRow(`SELECT cwd FROM sessions WHERE id = ?`, id.Bytes()).Scan(&cwd)
	if errors.Is(err, sql.ErrNoRows) {
		return "", ErrSessionNotFound
	}
	if err != nil {
		return "", fmt.Errorf("cwd da sessão: %w", err)
	}
	return cwd.String, nil
}

// SessionClose é o resultado de CloseSessionTx. Closed é true quando esta chamada encerrou a
// sessão; false quando ela já estava encerrada (nada muda).
type SessionClose struct {
	Closed bool
}

// CloseSessionTx marca o encerramento da sessão. Não apaga a linha. Grava ended_at e, na mesma
// escrita, ended_observation_count com a contagem atual de observações da sessão (linha de base
// do último encerramento, database-schema.md §2). É idempotente: encerrar de novo não altera
// ended_at nem a contagem. Sessão inexistente devolve ErrSessionNotFound.
func CloseSessionTx(tx *sql.Tx, id domain.SessionID, at time.Time) (SessionClose, error) {
	if tx == nil {
		return SessionClose{}, errors.New("encerrar sessão: transação nula")
	}
	if at.IsZero() {
		at = time.Now()
	}

	var endedAt sql.NullInt64
	err := tx.QueryRow(`SELECT ended_at FROM sessions WHERE id = ?`, id.Bytes()).Scan(&endedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return SessionClose{}, ErrSessionNotFound
	}
	if err != nil {
		return SessionClose{}, fmt.Errorf("encerrar sessão: %w", err)
	}
	if endedAt.Valid {
		return SessionClose{Closed: false}, nil
	}

	var count int
	if err := tx.QueryRow(`SELECT COUNT(*) FROM observations WHERE session_id = ?`, id.Bytes()).Scan(&count); err != nil {
		return SessionClose{}, fmt.Errorf("encerrar sessão: contagem: %w", err)
	}
	if _, err := tx.Exec(
		`UPDATE sessions SET ended_at = ?, ended_observation_count = ? WHERE id = ? AND ended_at IS NULL`,
		at.UnixMicro(), count, id.Bytes(),
	); err != nil {
		return SessionClose{}, fmt.Errorf("encerrar sessão: %w", err)
	}
	return SessionClose{Closed: true}, nil
}
