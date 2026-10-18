package storage

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"fmt"
	"path"
	"slices"
	"strings"
	"time"

	"github.com/phm-aguiar/ai-memory-go/internal/domain"
)

// Seleção e listagem de handoffs (docs/design-handoff-offer-not-claim.md §3; docs/storage-and-retrieval.md
// §7.2; docs/lifecycle-event-ingestion.md §11.2; docs/mcp-server.md §3.2). Só leitura: nada aqui muda o
// estado do handoff. A reivindicação atômica fica em outra função.

// HandoffRow é um handoff lido para seleção ou listagem. FromSessionID nil é handoff manual (sem sessão de
// origem). Cwd vazio é NULL no banco e vale para o projeto inteiro. OwnerUser vazio é NULL: handoff sem dono.
type HandoffRow struct {
	ID            domain.ID
	FromSessionID *domain.SessionID
	FromAgent     string
	Cwd           string
	Summary       string
	OpenQuestions []string
	NextSteps     []string
	FilesTouched  []string
	OwnerUser     string
	CreatedAt     time.Time
}

// handoffQuerier é o que a leitura precisa. *sql.DB e *sql.Tx o satisfazem: a listagem roda sobre o
// pool de leitura; a seleção pode ser reavaliada dentro da transação de reivindicação.
type handoffQuerier interface {
	Query(query string, args ...any) (*sql.Rows, error)
}

// Limites da listagem (docs/mcp-server.md §3.2): padrão 50, máximo 200.
const (
	handoffListDefault = 50
	handoffListMax     = 200
)

// handoffCols são as colunas lidas por scanHandoffRow, na mesma ordem.
const handoffCols = `SELECT h.id, h.from_session_id, h.from_agent, h.cwd, h.summary, h.open_questions,
	h.next_steps, h.files_touched, h.owner_user, h.created_at FROM handoffs h`

// SelectHandoff devolve o handoff que o receptor pode reivindicar, ou nil quando não há candidato.
//
// Regras, na ordem de docs/design-handoff-offer-not-claim.md §3:
//  1. Dono: owner vazio só enxerga handoffs sem dono; owner informado enxerga os dele e os sem dono.
//  2. Só handoffs com estado 'open'.
//  3. Sessão viva: um automático cuja sessão de origem está aberta e teve observação com created_at
//     depois de busySince não é candidato. Sessão encerrada não bloqueia. Manual nunca é afetado.
//  4. Manual (sem sessão de origem) é candidato em qualquer cwd do projeto.
//  5. Automático é candidato se o cwd armazenado for vazio, se o cwd do receptor for vazio, ou se o
//     cwd armazenado for ancestral ou igual ao do receptor, por fronteira de componente de caminho.
//  6. Preferência: manual antes de automático; depois created_at mais recente; entre automáticos com
//     o mesmo instante, cwd mais longo (normalizado); por fim id ascendente (decisão nossa: o doc não
//     define a direção do último desempate).
//
// busySince é o instante de corte da sessão viva, calculado pelo chamador (agora − 10 min).
func SelectHandoff(q handoffQuerier, scope domain.Scope, cwd, owner string, busySince time.Time) (*HandoffRow, error) {
	ownerSQL, ownerArgs := handoffOwnerClause(owner)
	query := handoffCols + ` WHERE h.workspace_id = ? AND h.project_id = ? AND h.state = 'open'
		AND ` + ownerSQL + `
		AND (h.from_session_id IS NULL OR NOT EXISTS (
			SELECT 1 FROM sessions s
			WHERE s.id = h.from_session_id AND s.ended_at IS NULL
			  AND EXISTS (SELECT 1 FROM observations o WHERE o.session_id = s.id AND o.created_at > ?)))`
	args := []any{scope.WorkspaceID.Bytes(), scope.ProjectID.Bytes()}
	args = append(args, ownerArgs...)
	args = append(args, busySince.UnixMicro())

	rows, err := scanHandoffRows(q, query, args...)
	if err != nil {
		return nil, err
	}

	candidates := rows[:0]
	for _, r := range rows {
		if r.FromSessionID != nil && !handoffCwdReaches(r.Cwd, cwd) {
			continue
		}
		candidates = append(candidates, r)
	}
	if len(candidates) == 0 {
		return nil, nil
	}
	slices.SortFunc(candidates, compareHandoffPreference)
	best := candidates[0]
	return &best, nil
}

// ListHandoffs lista handoffs abertos do escopo sem reivindicar nenhum (docs/mcp-server.md §3.2).
// O filtro de dono é o da seleção; anyOwner true o desliga. Quem chama decide se o ator pode usar anyOwner
// (privilégio administrativo). Não aplica cwd nem sessão viva: a listagem mostra o que está aberto.
// Limite zero ou negativo vale 50; acima de 200 vale 200. Ordem: created_at mais recente, depois id.
func ListHandoffs(q handoffQuerier, scope domain.Scope, owner string, anyOwner bool, limit int) ([]HandoffRow, error) {
	switch {
	case limit <= 0:
		limit = handoffListDefault
	case limit > handoffListMax:
		limit = handoffListMax
	}
	query := handoffCols + ` WHERE h.workspace_id = ? AND h.project_id = ? AND h.state = 'open'`
	args := []any{scope.WorkspaceID.Bytes(), scope.ProjectID.Bytes()}
	if !anyOwner {
		ownerSQL, ownerArgs := handoffOwnerClause(owner)
		query += ` AND ` + ownerSQL
		args = append(args, ownerArgs...)
	}
	query += ` ORDER BY h.created_at DESC, h.id ASC LIMIT ?`
	args = append(args, limit)
	return scanHandoffRows(q, query, args...)
}

// handoffOwnerClause é o filtro de dono (regra 1). Sem ator, só handoffs sem dono; com ator, os dele e os
// sem dono.
func handoffOwnerClause(owner string) (string, []any) {
	if owner == "" {
		return `h.owner_user IS NULL`, nil
	}
	return `(h.owner_user IS NULL OR h.owner_user = ?)`, []any{owner}
}

// handoffCwdReaches diz se um automático com cwd armazenado stored vale para o receptor de cwd receptor.
// Vazio de qualquer lado vale para todos.
func handoffCwdReaches(stored, receptor string) bool {
	s, r := normalizeHandoffCwd(stored), normalizeHandoffCwd(receptor)
	if s == "" || r == "" {
		return true
	}
	return isPathAncestorOrSelf(s, r)
}

// normalizeHandoffCwd limpa o caminho para comparação: separador invertido vira '/', e path.Clean remove
// barras repetidas e '.' e '..'. Vazio continua vazio. (Decisão nossa: o doc pede normalização sem detalhar.)
func normalizeHandoffCwd(p string) string {
	if p == "" {
		return ""
	}
	return path.Clean(strings.ReplaceAll(p, `\`, "/"))
}

// isPathAncestorOrSelf diz se anc é p ou um diretório ancestor de p. Compara por componente: o prefixo
// sempre termina em '/', então "/repo" não casa com "/repository". A raiz "/" casa com todo caminho absoluto.
func isPathAncestorOrSelf(anc, p string) bool {
	if anc == p {
		return true
	}
	prefix := anc
	if !strings.HasSuffix(prefix, "/") {
		prefix += "/"
	}
	return strings.HasPrefix(p, prefix)
}

// compareHandoffPreference ordena do melhor para o pior candidato (regra 6). Retorna negativo quando a
// vem antes de b.
func compareHandoffPreference(a, b HandoffRow) int {
	aManual, bManual := a.FromSessionID == nil, b.FromSessionID == nil
	if aManual != bManual {
		if aManual {
			return -1
		}
		return 1
	}
	if c := b.CreatedAt.Compare(a.CreatedAt); c != 0 {
		return c
	}
	if !aManual {
		if la, lb := len(normalizeHandoffCwd(a.Cwd)), len(normalizeHandoffCwd(b.Cwd)); la != lb {
			return lb - la
		}
	}
	return bytes.Compare(a.ID[:], b.ID[:])
}

// scanHandoffRows executa a consulta e lê todas as linhas no formato de handoffCols.
func scanHandoffRows(q handoffQuerier, query string, args ...any) ([]HandoffRow, error) {
	rows, err := q.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("ler handoffs: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var out []HandoffRow
	for rows.Next() {
		h, err := scanHandoffRow(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, h)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("ler handoffs: %w", err)
	}
	return out, nil
}

// scanHandoffRow lê uma linha de handoffCols.
func scanHandoffRow(sc rowScanner) (HandoffRow, error) {
	var (
		idBytes, sessBytes  []byte
		cwd, owner          sql.NullString
		openQ, nextS, files string
		createdAt           int64
		h                   HandoffRow
	)
	if err := sc.Scan(&idBytes, &sessBytes, &h.FromAgent, &cwd, &h.Summary, &openQ, &nextS, &files, &owner, &createdAt); err != nil {
		return HandoffRow{}, fmt.Errorf("ler handoff: %w", err)
	}
	id, err := domain.ParseIDBytes(idBytes)
	if err != nil {
		return HandoffRow{}, fmt.Errorf("ler handoff: id: %w", err)
	}
	h.ID = id
	if sessBytes != nil {
		sid, err := domain.ParseIDBytes(sessBytes)
		if err != nil {
			return HandoffRow{}, fmt.Errorf("ler handoff: sessão de origem: %w", err)
		}
		s := domain.SessionID(sid)
		h.FromSessionID = &s
	}
	h.Cwd = cwd.String
	h.OwnerUser = owner.String
	h.CreatedAt = time.UnixMicro(createdAt)
	if h.OpenQuestions, err = decodeHandoffList(openQ); err != nil {
		return HandoffRow{}, err
	}
	if h.NextSteps, err = decodeHandoffList(nextS); err != nil {
		return HandoffRow{}, err
	}
	if h.FilesTouched, err = decodeHandoffList(files); err != nil {
		return HandoffRow{}, err
	}
	return h, nil
}

// decodeHandoffList lê um array JSON de texto gravado por encodeHandoffList.
func decodeHandoffList(s string) ([]string, error) {
	var out []string
	if err := json.Unmarshal([]byte(s), &out); err != nil {
		return nil, fmt.Errorf("ler handoff: lista: %w", err)
	}
	return out, nil
}
