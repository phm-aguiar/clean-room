package storage

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/phm-aguiar/ai-memory-go/internal/domain"
)

// Gravação de handoff (docs/design-handoff-offer-not-claim.md §4; docs/database-schema.md §3;
// docs/usage.md: cancelar marca o handoff como expirado). Tudo aqui roda dentro de uma
// transação de escrita (Writer.Do), no mesmo padrão de sessions.go.
//
// Estados da linha: 'open', 'accepted' e 'expired'. Só 'open' pode ser expirado por esta
// gravação ou cancelado por id.

// HandoffInsert são os dados de um handoff novo. FromSession nil cria handoff manual (sem
// sessão de origem); não nil cria handoff automático. Cwd e OwnerUser vazios gravam NULL.
// At zero usa o instante atual.
type HandoffInsert struct {
	Scope         domain.Scope
	FromSession   *domain.SessionID
	FromAgent     string
	Cwd           string
	Summary       string
	OpenQuestions []string
	NextSteps     []string
	FilesTouched  []string
	OwnerUser     string
	At            time.Time
}

// HandoffInsertResult é o resultado de InsertHandoffTx. ID é o identificador novo (UUIDv7).
// Superseded é a quantidade de handoffs automáticos abertos que foram expirados antes da
// inserção. Sempre zero para handoff manual.
type HandoffInsertResult struct {
	ID         domain.ID
	Superseded int64
}

// HandoffCancel é o resultado de CancelHandoffTx. Cancelled é true quando esta chamada
// marcou o handoff como expirado; false quando ele não estava aberto ou não existe no escopo.
type HandoffCancel struct {
	Cancelled bool
}

// InsertHandoffTx grava um handoff. Para handoff automático (FromSession não nil), antes de
// inserir expira (state = 'expired') os handoffs automáticos abertos do mesmo escopo cujo cwd
// é exatamente igual ao informado (NULL equivale a NULL), cujo dono é o mesmo (NULL equivale
// a NULL), e cuja sessão de origem ou já encerrou ou é a própria sessão que grava. Sessões
// abertas são poupadas: cada uma atualiza o próprio bastão. Não há critério de idade nem
// ancestralidade de cwd. Handoff manual não expira nenhum outro handoff.
//
// Sessão de origem inexistente devolve ErrSessionNotFound; de outro escopo, ErrSessionScope.
func InsertHandoffTx(tx *sql.Tx, h HandoffInsert) (HandoffInsertResult, error) {
	if tx == nil {
		return HandoffInsertResult{}, errors.New("gravar handoff: transação nula")
	}
	if strings.TrimSpace(h.FromAgent) == "" {
		return HandoffInsertResult{}, errors.New("gravar handoff: from_agent vazio")
	}
	at := h.At
	if at.IsZero() {
		at = time.Now()
	}
	openQuestions, err := encodeHandoffList(h.OpenQuestions)
	if err != nil {
		return HandoffInsertResult{}, err
	}
	nextSteps, err := encodeHandoffList(h.NextSteps)
	if err != nil {
		return HandoffInsertResult{}, err
	}
	filesTouched, err := encodeHandoffList(h.FilesTouched)
	if err != nil {
		return HandoffInsertResult{}, err
	}

	var superseded int64
	var fromSession any
	if h.FromSession != nil {
		if err := checkHandoffSessionScope(tx, *h.FromSession, h.Scope); err != nil {
			return HandoffInsertResult{}, err
		}
		superseded, err = expireSupersededHandoffsTx(tx, h, *h.FromSession)
		if err != nil {
			return HandoffInsertResult{}, err
		}
		fromSession = h.FromSession.Bytes()
	}

	id, err := domain.NewID()
	if err != nil {
		return HandoffInsertResult{}, fmt.Errorf("gravar handoff: id: %w", err)
	}

	if _, err := tx.Exec(
		`INSERT INTO handoffs (id, workspace_id, project_id, from_session_id, from_agent, cwd, summary,
		   open_questions, next_steps, files_touched, state, created_at, owner_user)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 'open', ?, ?)`,
		id.Bytes(), h.Scope.WorkspaceID.Bytes(), h.Scope.ProjectID.Bytes(), fromSession,
		h.FromAgent, nullIfEmpty(h.Cwd), h.Summary,
		openQuestions, nextSteps, filesTouched, at.UnixMicro(), nullIfEmpty(h.OwnerUser),
	); err != nil {
		return HandoffInsertResult{}, fmt.Errorf("gravar handoff: %w", err)
	}
	return HandoffInsertResult{ID: id, Superseded: superseded}, nil
}

// CancelHandoffTx marca como expirado o handoff de id exato, somente se ele estiver 'open' no
// escopo. Não apaga a linha. Se o handoff já não estava aberto, ou não existe no escopo, devolve
// Cancelled false sem erro.
func CancelHandoffTx(tx *sql.Tx, scope domain.Scope, id domain.ID) (HandoffCancel, error) {
	if tx == nil {
		return HandoffCancel{}, errors.New("cancelar handoff: transação nula")
	}
	res, err := tx.Exec(
		`UPDATE handoffs SET state = 'expired'
		 WHERE id = ? AND workspace_id = ? AND project_id = ? AND state = 'open'`,
		id.Bytes(), scope.WorkspaceID.Bytes(), scope.ProjectID.Bytes(),
	)
	if err != nil {
		return HandoffCancel{}, fmt.Errorf("cancelar handoff: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return HandoffCancel{}, fmt.Errorf("cancelar handoff: linhas: %w", err)
	}
	return HandoffCancel{Cancelled: n == 1}, nil
}

// checkHandoffSessionScope confere que a sessão de origem existe e pertence ao escopo.
func checkHandoffSessionScope(tx *sql.Tx, id domain.SessionID, scope domain.Scope) error {
	var wsID, projID []byte
	err := tx.QueryRow(`SELECT workspace_id, project_id FROM sessions WHERE id = ?`, id.Bytes()).Scan(&wsID, &projID)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrSessionNotFound
	}
	if err != nil {
		return fmt.Errorf("gravar handoff: sessão de origem: %w", err)
	}
	if string(wsID) != string(scope.WorkspaceID.Bytes()) || string(projID) != string(scope.ProjectID.Bytes()) {
		return ErrSessionScope
	}
	return nil
}

// expireSupersededHandoffsTx expira os automáticos abertos do mesmo escopo, com cwd e dono
// iguais aos da inserção, cuja sessão de origem encerrou ou é a sessão que grava.
func expireSupersededHandoffsTx(tx *sql.Tx, h HandoffInsert, writer domain.SessionID) (int64, error) {
	res, err := tx.Exec(
		`UPDATE handoffs SET state = 'expired'
		 WHERE workspace_id = ? AND project_id = ? AND state = 'open'
		   AND from_session_id IS NOT NULL
		   AND cwd IS ?
		   AND owner_user IS ?
		   AND (from_session_id = ?
		        OR from_session_id IN (SELECT id FROM sessions WHERE ended_at IS NOT NULL))`,
		h.Scope.WorkspaceID.Bytes(), h.Scope.ProjectID.Bytes(),
		nullIfEmpty(h.Cwd), nullIfEmpty(h.OwnerUser), writer.Bytes(),
	)
	if err != nil {
		return 0, fmt.Errorf("gravar handoff: expirar anteriores: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("gravar handoff: expirar anteriores: linhas: %w", err)
	}
	return n, nil
}

// encodeHandoffList serializa uma lista como array JSON. Lista nula vira '[]'.
func encodeHandoffList(items []string) (string, error) {
	if items == nil {
		items = []string{}
	}
	b, err := json.Marshal(items)
	if err != nil {
		return "", fmt.Errorf("gravar handoff: lista: %w", err)
	}
	return string(b), nil
}

// nullIfEmpty devolve nil (NULL no banco) para texto vazio.
func nullIfEmpty(s string) any {
	if s == "" {
		return nil
	}
	return s
}
