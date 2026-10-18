package storage

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/phm-aguiar/ai-memory-go/internal/domain"
)

// Reivindicação atômica de handoff (docs/design-handoff-offer-not-claim.md §4 e §8; docs/storage-and-retrieval.md
// §7.3; docs/database-schema.md §3; docs/complementary-contracts.md §6). Tudo aqui roda dentro de uma transação de
// escrita (Writer.Do). Um handoff é reivindicado exatamente uma vez (INV-16): a troca open → accepted tem duas
// guardas independentes, a leitura do candidato com state = 'open' e o UPDATE com state = 'open'.

// ErrHandoffReceiverEnded é devolvido quando a sessão receptora já está encerrada. Decisão nossa: o doc permite
// aceite por sessão encerrada só para tipos de agente que reutilizam o id, e não diz quais são; recusar é o caminho
// seguro.
var ErrHandoffReceiverEnded = errors.New("sessão receptora encerrada")

// ErrHandoffOwner é devolvido quando o handoff pedido por id pertence a outro operador que o ator. Estado inválido
// (docs/storage-and-retrieval.md §7.3): o filtro de dono vale também para o aceite por id.
var ErrHandoffOwner = errors.New("handoff pertence a outro dono")

// handoffAcceptAuditOp é a op de audit_log gravada em toda reivindicação efetiva. Decisão nossa: o doc exige a
// auditoria (§7.3) mas não nomeia a op.
const handoffAcceptAuditOp = "handoff_accept"

// HandoffClaim são os dados de uma reivindicação. Target nil escolhe o candidato pela seleção (SelectHandoff, com
// sessão viva reavaliada nesta transação). Target não nil reivindica o id exato, só se estiver 'open' no escopo.
// Actor é o operador do ator: filtra o dono e vira accepted_by_user. Cwd é o diretório do receptor. BusySince é o
// corte da sessão viva (agora − 10 min). At zero usa o instante atual.
type HandoffClaim struct {
	Scope     domain.Scope
	Receiver  domain.SessionID
	Cwd       string
	Actor     string
	Target    *domain.ID
	BusySince time.Time
	At        time.Time
}

// HandoffClaimResult é o resultado de ClaimHandoffTx. Claimed nil significa que esta chamada não reivindicou nada
// (sem candidato, perdeu a corrida, ou o receptor já tem um handoff). Swept é a quantidade de handoffs automáticos
// expirados pela varredura pós-reivindicação.
type HandoffClaimResult struct {
	Claimed *HandoffRow
	Swept   int64
}

// ClaimHandoffTx reivindica um handoff para a sessão receptora, na transação dada. Ordem:
//  1. A sessão receptora existe, é do mesmo escopo e não está encerrada.
//  2. Idempotência do receptor: se ele já reivindicou um handoff, não reivindica outro.
//  3. Candidato: a seleção (sessão viva reavaliada aqui) ou o id exato, ambos só com state = 'open' e o filtro de dono.
//  4. Guarda 2 da troca: UPDATE ... WHERE state = 'open' (e o filtro de dono). Se não afeta uma linha, perdeu a corrida.
//  5. Auditoria da transição real.
//  6. Varredura pós-reivindicação, só quando o reivindicado é automático.
//
// A varredura expira os automáticos abertos do mesmo dono, com cwd compatível com o do receptor, de preferência
// menor que a do reivindicado, e cuja sessão de origem não está viva. Handoffs manuais nunca são varridos.
func ClaimHandoffTx(tx *sql.Tx, c HandoffClaim) (HandoffClaimResult, error) {
	if tx == nil {
		return HandoffClaimResult{}, errors.New("reivindicar handoff: transação nula")
	}
	at := c.At
	if at.IsZero() {
		at = time.Now()
	}

	agentKind, err := checkClaimReceiverTx(tx, c.Receiver, c.Scope)
	if err != nil {
		return HandoffClaimResult{}, err
	}
	already, err := receiverHasClaimTx(tx, c.Receiver)
	if err != nil {
		return HandoffClaimResult{}, err
	}
	if already {
		return HandoffClaimResult{}, nil
	}

	row, err := claimCandidateTx(tx, c)
	if err != nil {
		return HandoffClaimResult{}, err
	}
	if row == nil {
		return HandoffClaimResult{}, nil
	}

	claimed, err := markAcceptedTx(tx, c, *row, agentKind, at)
	if err != nil {
		return HandoffClaimResult{}, err
	}
	if !claimed {
		return HandoffClaimResult{}, nil
	}
	if err := InsertAudit(tx, at, AuditEvent{Op: handoffAcceptAuditOp, Scope: c.Scope}); err != nil {
		return HandoffClaimResult{}, fmt.Errorf("reivindicar handoff: %w", err)
	}

	var swept int64
	if row.FromSessionID != nil {
		swept, err = sweepAfterClaimTx(tx, c, *row)
		if err != nil {
			return HandoffClaimResult{}, err
		}
	}
	return HandoffClaimResult{Claimed: row, Swept: swept}, nil
}

// checkClaimReceiverTx confere a sessão receptora: existe, é do escopo e está aberta. Devolve o tipo de agente, que
// vai para accepted_by.
func checkClaimReceiverTx(tx *sql.Tx, id domain.SessionID, scope domain.Scope) (string, error) {
	var wsID, projID []byte
	var agentKind string
	var endedAt sql.NullInt64
	err := tx.QueryRow(
		`SELECT workspace_id, project_id, agent_kind, ended_at FROM sessions WHERE id = ?`, id.Bytes(),
	).Scan(&wsID, &projID, &agentKind, &endedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return "", ErrSessionNotFound
	}
	if err != nil {
		return "", fmt.Errorf("reivindicar handoff: sessão receptora: %w", err)
	}
	if string(wsID) != string(scope.WorkspaceID.Bytes()) || string(projID) != string(scope.ProjectID.Bytes()) {
		return "", ErrSessionScope
	}
	if endedAt.Valid {
		return "", ErrHandoffReceiverEnded
	}
	return agentKind, nil
}

// receiverHasClaimTx diz se a sessão receptora já reivindicou um handoff (uma sessão nunca consome dois batões).
func receiverHasClaimTx(tx *sql.Tx, id domain.SessionID) (bool, error) {
	var found bool
	err := tx.QueryRow(
		`SELECT EXISTS (SELECT 1 FROM handoffs WHERE accepted_by_session = ? AND state = 'accepted')`, id.Bytes(),
	).Scan(&found)
	if err != nil {
		return false, fmt.Errorf("reivindicar handoff: receptor: %w", err)
	}
	return found, nil
}

// claimCandidateTx escolhe o handoff a reivindicar. Sem Target, usa a seleção, que reavalia a sessão viva dentro
// desta transação. Com Target, lê só o id exato, com state = 'open' e no escopo: a regra de estado aberto decide, e
// uma nota obsoleta apenas perde a corrida (docs/design-handoff-offer-not-claim.md §6). Dono incompatível é erro.
func claimCandidateTx(tx *sql.Tx, c HandoffClaim) (*HandoffRow, error) {
	if c.Target == nil {
		return SelectHandoff(tx, c.Scope, c.Cwd, c.Actor, c.BusySince)
	}
	rows, err := scanHandoffRows(tx,
		handoffCols+` WHERE h.id = ? AND h.workspace_id = ? AND h.project_id = ? AND h.state = 'open'`,
		c.Target.Bytes(), c.Scope.WorkspaceID.Bytes(), c.Scope.ProjectID.Bytes(),
	)
	if err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return nil, nil
	}
	r := rows[0]
	if r.OwnerUser != "" && r.OwnerUser != c.Actor {
		return nil, ErrHandoffOwner
	}
	return &r, nil
}

// markAcceptedTx faz a guarda 2 da troca de estado: open → accepted, só se a linha ainda está 'open' e o dono bate.
// Devolve false quando nenhuma linha foi afetada.
func markAcceptedTx(tx *sql.Tx, c HandoffClaim, row HandoffRow, agentKind string, at time.Time) (bool, error) {
	ownerCond := `owner_user IS NULL`
	ownerArgs := []any{}
	if c.Actor != "" {
		ownerCond = `(owner_user IS NULL OR owner_user = ?)`
		ownerArgs = []any{c.Actor}
	}
	args := []any{agentKind, at.UnixMicro(), c.Receiver.Bytes(), nullIfEmpty(c.Actor),
		row.ID.Bytes(), c.Scope.WorkspaceID.Bytes(), c.Scope.ProjectID.Bytes()}
	args = append(args, ownerArgs...)
	res, err := tx.Exec(
		`UPDATE handoffs SET state = 'accepted', accepted_by = ?, accepted_at = ?,
		   accepted_by_session = ?, accepted_by_user = ?
		 WHERE id = ? AND workspace_id = ? AND project_id = ? AND state = 'open' AND `+ownerCond,
		args...,
	)
	if err != nil {
		return false, fmt.Errorf("reivindicar handoff: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("reivindicar handoff: linhas: %w", err)
	}
	return n == 1, nil
}

// sweepAfterClaimTx expira os automáticos abertos do mesmo dono do reivindicado, com cwd compatível com o do receptor
// (fronteira de componente), de preferência menor que a do reivindicado, e cuja sessão de origem não está viva
// (mesma regra de sessão viva da seleção, com o mesmo corte). Manuais nunca entram: o filtro exige from_session_id.
func sweepAfterClaimTx(tx *sql.Tx, c HandoffClaim, claimed HandoffRow) (int64, error) {
	rows, err := scanHandoffRows(tx,
		handoffCols+` WHERE h.workspace_id = ? AND h.project_id = ? AND h.state = 'open'
		  AND h.from_session_id IS NOT NULL AND h.owner_user IS ? AND h.id <> ?
		  AND NOT EXISTS (
			SELECT 1 FROM sessions s
			WHERE s.id = h.from_session_id AND s.ended_at IS NULL
			  AND EXISTS (SELECT 1 FROM observations o WHERE o.session_id = s.id AND o.created_at > ?))`,
		c.Scope.WorkspaceID.Bytes(), c.Scope.ProjectID.Bytes(), nullIfEmpty(claimed.OwnerUser),
		claimed.ID.Bytes(), c.BusySince.UnixMicro(),
	)
	if err != nil {
		return 0, err
	}

	var ids []any
	for _, r := range rows {
		if !handoffCwdReaches(r.Cwd, c.Cwd) {
			continue
		}
		if compareHandoffPreference(claimed, r) >= 0 {
			continue
		}
		ids = append(ids, r.ID.Bytes())
	}
	if len(ids) == 0 {
		return 0, nil
	}

	placeholders := strings.TrimSuffix(strings.Repeat("?, ", len(ids)), ", ")
	res, err := tx.Exec(
		`UPDATE handoffs SET state = 'expired' WHERE state = 'open' AND id IN (`+placeholders+`)`, ids...,
	)
	if err != nil {
		return 0, fmt.Errorf("reivindicar handoff: varredura: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("reivindicar handoff: varredura: linhas: %w", err)
	}
	return n, nil
}
