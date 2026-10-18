package mcp

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/phm-aguiar/ai-memory-go/internal/domain"
	"github.com/phm-aguiar/ai-memory-go/internal/storage"
)

// Ferramentas de leitura de sessão e de briefing (docs/mcp-server.md §3.2;
// specs/features/002-captura-e-acesso/notes/mcp-contract.md §3.4 e §3.6).
//
// Escopo: sempre pelo resolvedor de leitura (storage.ResolveReadScope). Sessões: só as que o
// chamador pode ver pelo filtro de dono (docs/domain-vocabulary.md §8.5). Nenhum handler lê
// cabeçalho cru: a identidade vem de ActorKeyFrom.

const (
	// Observações de sessão (contrato §3.4).
	sessionObsDefaultLimit = 50
	sessionObsMaxLimit     = 200
	sessionObsDefaultBody  = 4000
	sessionObsMinBody      = 200
	sessionObsMaxBody      = 16384
	// sessionObsTruncMarker é anexado ao corpo cortado por body_max_chars.
	sessionObsTruncMarker = " [corpo cortado]"

	// Briefing (contrato §3.6).
	briefingPinnedLimit  = 10
	briefingSettledLimit = 8
	// briefingRecentLimit é decisão nossa: o contrato não fixa o número de páginas recentes.
	briefingRecentLimit = 10

	obsOrderAsc  = "asc"
	obsOrderDesc = "desc"
)

// Classes de erro de ferramenta (phase0-findings L1). A mensagem da classe não ecoa valor
// informado nem caminho do sistema de arquivos.
var (
	errToolInput    = errors.New("entrada inválida")
	errToolNotFound = errors.New("não encontrado")

	errToolSessionNotFound = fmt.Errorf("%w: sessão não encontrada", errToolNotFound)
	errToolNoCompleted     = fmt.Errorf("%w: nenhuma sessão concluída visível no escopo", errToolNotFound)
)

// SessionToolDeps são as dependências das ferramentas de sessão e briefing.
// Reader é obrigatório. Pointer pode ser nil (sem ponteiro de projeto ativo). Now nil usa o relógio atual.
type SessionToolDeps struct {
	Reader  storage.Reader
	Pointer *storage.ActiveProjectPointer
	Now     func() time.Time
}

// RegisterSessionTools registra memory_read_session_observations e memory_briefing em srv.
func RegisterSessionTools(srv ToolServer, deps SessionToolDeps) error {
	if srv == nil {
		return errors.New("mcp: servidor nulo para ferramentas de sessão")
	}
	if deps.Reader == nil {
		return errors.New("mcp: leitor nulo para ferramentas de sessão")
	}
	if deps.Now == nil {
		deps.Now = time.Now
	}
	if err := srv.AddTool(ToolDef{
		Name: "memory_read_session_observations",
		Description: "Observações brutas de uma sessão, paginadas. Sem session_id usa a sessão concluída " +
			"mais recente visível no projeto. limit padrão 50 (máximo 200); body_max_chars padrão 4000 " +
			"(limitado a 200..16384); order asc (padrão) ou desc. Responde total e elided_other_scope.",
		InputSchema: json.RawMessage(`{"type":"object","properties":{` +
			`"session_id":{"type":"string"},` +
			`"limit":{"type":"integer"},` +
			`"offset":{"type":"integer"},` +
			`"body_max_chars":{"type":"integer"},` +
			`"order":{"type":"string","enum":["asc","desc"]},` +
			`"workspace":{"type":"string"},` +
			`"project":{"type":"string"}}}`),
	}, deps.readSessionObservations); err != nil {
		return err
	}
	return srv.AddTool(ToolDef{
		Name: "memory_briefing",
		Description: "Snapshot estruturado do projeto: contagens, páginas recentes e páginas fixadas " +
			"(até 10, omitidas quando não há). Com settled_first=true, inclui até 8 regras e decisões " +
			"de maior posição, por evidência e depois recência.",
		InputSchema: json.RawMessage(`{"type":"object","properties":{` +
			`"workspace":{"type":"string"},` +
			`"project":{"type":"string"},` +
			`"settled_first":{"type":"boolean"}}}`),
	}, deps.briefing)
}

// ---------------------------------------------------------------------------
// Entrada, escopo e identidade
// ---------------------------------------------------------------------------

// decodeToolArgs decodifica os argumentos. Sem argumentos, vale o objeto vazio.
func decodeToolArgs(args json.RawMessage, dst any) error {
	if len(args) == 0 {
		return nil
	}
	if err := json.Unmarshal(args, dst); err != nil {
		return fmt.Errorf("%w: argumentos não decodificam", errToolInput)
	}
	return nil
}

// resolveToolScope resolve o escopo de leitura pelo resolvedor do armazenamento e traduz os
// erros para as classes da ferramenta, sem eco dos nomes informados.
func (d SessionToolDeps) resolveToolScope(ctx context.Context, workspace, project string) (storage.ScopeResolution, error) {
	key, _ := ActorKeyFrom(ctx)
	res, err := storage.ResolveReadScope(ctx, d.Reader, storage.ScopeRequest{
		WorkspaceName: workspace,
		ProjectName:   project,
		Pointer:       d.Pointer,
		Actor:         storage.ActorKey{Identity: key.User, SessionID: key.SessionID},
		Now:           d.Now(),
	})
	switch {
	case err == nil:
		return res, nil
	case errors.Is(err, storage.ErrScopeInvalid):
		return storage.ScopeResolution{}, fmt.Errorf("%w: escopo inválido", errToolInput)
	case errors.Is(err, storage.ErrScopeAmbiguous):
		return storage.ScopeResolution{}, fmt.Errorf("%w: escopo ambíguo", errToolInput)
	case errors.Is(err, storage.ErrScopeNotFound):
		return storage.ScopeResolution{}, fmt.Errorf("%w: escopo não existe", errToolNotFound)
	case errors.Is(err, storage.ErrScopeMismatch):
		return storage.ScopeResolution{}, fmt.Errorf("%w: projeto ativo diverge do chamador", errToolInput)
	default:
		return storage.ScopeResolution{}, errors.New("falha ao resolver escopo")
	}
}

// actorUser devolve a chave de usuário do ator do contexto, vazia para anônimo.
func actorUser(ctx context.Context) string {
	key, _ := ActorKeyFrom(ctx)
	return key.User
}

// ownerFilter monta o filtro de dono de sessões para o usuário (docs/domain-vocabulary.md §8.5).
// Usuário: sessões dele e não atribuídas. Anônimo: só não atribuídas (nenhum registro com dono).
func ownerFilter(user string) (string, []any) {
	if user == "" {
		return "actor_user IS NULL", nil
	}
	return "(actor_user IS NULL OR actor_user = ?)", []any{user}
}

// intOr devolve *v ou def quando v é nil.
func intOr(v *int, def int) int {
	if v == nil {
		return def
	}
	return *v
}

// clampInt limita v ao intervalo [lo, hi].
func clampInt(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

// cutRunes corta s em max caracteres (runas, nunca no meio de um UTF-8). Acima do limite,
// anexa o marcador e devolve truncated true.
func cutRunes(s string, max int) (string, bool) {
	if utf8.RuneCountInString(s) <= max {
		return s, false
	}
	return string([]rune(s)[:max]) + sessionObsTruncMarker, true
}

// ---------------------------------------------------------------------------
// memory_read_session_observations
// ---------------------------------------------------------------------------

type readSessionObsArgs struct {
	SessionID    string `json:"session_id"`
	Limit        *int   `json:"limit"`
	Offset       *int   `json:"offset"`
	BodyMaxChars *int   `json:"body_max_chars"`
	Order        string `json:"order"`
	Workspace    string `json:"workspace"`
	Project      string `json:"project"`
}

type sessionObsItem struct {
	ID            string `json:"id"`
	Kind          string `json:"kind"`
	Title         string `json:"title"`
	Body          string `json:"body"`
	BodyTruncated bool   `json:"body_truncated"`
	Importance    int    `json:"importance"`
	CreatedAt     string `json:"created_at"`
	Extension     string `json:"extension,omitempty"`
	SourceEvent   string `json:"source_event,omitempty"`
}

type sessionObsResult struct {
	ResolvedBy       string           `json:"resolved_by"`
	SessionID        string           `json:"session_id"`
	Order            string           `json:"order"`
	Limit            int              `json:"limit"`
	Offset           int              `json:"offset"`
	BodyMaxChars     int              `json:"body_max_chars"`
	Total            int              `json:"total"`
	ElidedOtherScope int              `json:"elided_other_scope"`
	Observations     []sessionObsItem `json:"observations"`
}

func (d SessionToolDeps) readSessionObservations(ctx context.Context, args json.RawMessage) (ToolResult, error) {
	var in readSessionObsArgs
	if err := decodeToolArgs(args, &in); err != nil {
		return ToolResult{}, err
	}

	order := obsOrderAsc
	if raw := strings.ToLower(strings.TrimSpace(in.Order)); raw != "" {
		switch raw {
		case obsOrderAsc, obsOrderDesc:
			order = raw
		default:
			return ToolResult{}, fmt.Errorf("%w: order deve ser asc ou desc", errToolInput)
		}
	}
	offset := intOr(in.Offset, 0)
	if offset < 0 {
		return ToolResult{}, fmt.Errorf("%w: offset não pode ser negativo", errToolInput)
	}
	limit := clampInt(intOr(in.Limit, sessionObsDefaultLimit), 1, sessionObsMaxLimit)
	bodyMax := clampInt(intOr(in.BodyMaxChars, sessionObsDefaultBody), sessionObsMinBody, sessionObsMaxBody)

	res, err := d.resolveToolScope(ctx, strings.TrimSpace(in.Workspace), strings.TrimSpace(in.Project))
	if err != nil {
		return ToolResult{}, err
	}

	out := sessionObsResult{
		ResolvedBy:   string(res.Source),
		Order:        order,
		Limit:        limit,
		Offset:       offset,
		BodyMaxChars: bodyMax,
		Observations: []sessionObsItem{},
	}
	user := actorUser(ctx)
	sessionArg := strings.TrimSpace(in.SessionID)

	err = d.Reader.Read(ctx, func(tx *sql.Tx) error {
		sid, err := pickObsSession(tx, res.Scope, user, sessionArg)
		if err != nil {
			return err
		}
		sidID, err := domain.ParseIDBytes(sid)
		if err != nil {
			return fmt.Errorf("id da sessão: %w", err)
		}
		out.SessionID = sidID.String()
		return loadSessionObservations(tx, res.Scope, sid, order, limit, offset, bodyMax, &out)
	})
	if err != nil {
		return ToolResult{}, err
	}

	b, err := json.Marshal(out)
	if err != nil {
		return ToolResult{}, errors.New("falha ao serializar observações")
	}
	return ToolResult{Text: string(b)}, nil
}

// pickObsSession devolve o id (bytes) da sessão pedida ou da concluída mais recente visível no
// escopo. Sessão de outro escopo, inexistente ou não visível ao ator vira não encontrado, sem
// distinguir o motivo.
func pickObsSession(tx *sql.Tx, scope domain.Scope, user, raw string) ([]byte, error) {
	owner, ownerArgs := ownerFilter(user)
	ws, pj := scope.WorkspaceID.Bytes(), scope.ProjectID.Bytes()

	if raw != "" {
		id, err := domain.ParseID(raw)
		if err != nil {
			return nil, errToolSessionNotFound
		}
		args := append([]any{id.Bytes(), ws, pj}, ownerArgs...)
		var n int
		if err := tx.QueryRow(
			`SELECT COUNT(*) FROM sessions WHERE id = ? AND workspace_id = ? AND project_id = ? AND `+owner,
			args...,
		).Scan(&n); err != nil {
			return nil, fmt.Errorf("consultar sessão: %w", err)
		}
		if n == 0 {
			return nil, errToolSessionNotFound
		}
		return id.Bytes(), nil
	}

	args := append([]any{ws, pj}, ownerArgs...)
	var id []byte
	err := tx.QueryRow(
		`SELECT id FROM sessions
		  WHERE workspace_id = ? AND project_id = ? AND ended_at IS NOT NULL AND `+owner+`
		  ORDER BY ended_at DESC, started_at DESC, id DESC
		  LIMIT 1`,
		args...,
	).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, errToolNoCompleted
	}
	if err != nil {
		return nil, fmt.Errorf("consultar sessão concluída: %w", err)
	}
	return id, nil
}

// loadSessionObservations preenche out com a página de observações da sessão no escopo, o total
// do escopo e a contagem de itens de outro escopo omitidos. order é "asc" ou "desc" já validado.
func loadSessionObservations(tx *sql.Tx, scope domain.Scope, sid []byte, order string, limit, offset, bodyMax int, out *sessionObsResult) error {
	ws, pj := scope.WorkspaceID.Bytes(), scope.ProjectID.Bytes()

	if err := tx.QueryRow(
		`SELECT COUNT(*) FROM observations WHERE session_id = ? AND workspace_id = ? AND project_id = ?`,
		sid, ws, pj,
	).Scan(&out.Total); err != nil {
		return fmt.Errorf("contar observações: %w", err)
	}
	if err := tx.QueryRow(
		`SELECT COUNT(*) FROM observations WHERE session_id = ? AND NOT (workspace_id = ? AND project_id = ?)`,
		sid, ws, pj,
	).Scan(&out.ElidedOtherScope); err != nil {
		return fmt.Errorf("contar observações de outro escopo: %w", err)
	}

	// order vem de uma lista fechada ("asc" ou "desc"), então a interpolação não abre injeção.
	rows, err := tx.Query(
		`SELECT id, kind, title, body, importance, created_at, extension, source_event
		   FROM observations
		  WHERE session_id = ? AND workspace_id = ? AND project_id = ?
		  ORDER BY created_at `+order+`, id `+order+`
		  LIMIT ? OFFSET ?`,
		sid, ws, pj, limit, offset,
	)
	if err != nil {
		return fmt.Errorf("listar observações: %w", err)
	}
	defer func() { _ = rows.Close() }()

	for rows.Next() {
		var (
			idB                []byte
			kind, title, body  string
			importance         int
			createdAt          int64
			extension, srcEvnt sql.NullString
		)
		if err := rows.Scan(&idB, &kind, &title, &body, &importance, &createdAt, &extension, &srcEvnt); err != nil {
			return fmt.Errorf("ler observação: %w", err)
		}
		obsID, err := domain.ParseIDBytes(idB)
		if err != nil {
			return fmt.Errorf("id da observação: %w", err)
		}
		cut, truncated := cutRunes(body, bodyMax)
		out.Observations = append(out.Observations, sessionObsItem{
			ID:            domain.ObservationID(obsID).String(),
			Kind:          kind,
			Title:         title,
			Body:          cut,
			BodyTruncated: truncated,
			Importance:    importance,
			CreatedAt:     time.UnixMicro(createdAt).UTC().Format(time.RFC3339Nano),
			Extension:     extension.String,
			SourceEvent:   srcEvnt.String,
		})
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterar observações: %w", err)
	}
	return nil
}

// ---------------------------------------------------------------------------
// memory_briefing
// ---------------------------------------------------------------------------

type briefingArgs struct {
	Workspace    string `json:"workspace"`
	Project      string `json:"project"`
	SettledFirst bool   `json:"settled_first"`
}

type briefingPage struct {
	Path          string `json:"path"`
	Title         string `json:"title"`
	Tier          string `json:"tier"`
	Kind          string `json:"kind,omitempty"`
	UpdatedAt     string `json:"updated_at"`
	EvidenceCount int    `json:"evidence_count"`
}

type briefingCounts struct {
	Pages        int `json:"pages"`
	Observations int `json:"observations"`
	Sessions     int `json:"sessions"`
}

type briefingResult struct {
	ResolvedBy  string         `json:"resolved_by"`
	WorkspaceID string         `json:"workspace_id"`
	ProjectID   string         `json:"project_id"`
	Counts      briefingCounts `json:"counts"`
	RecentPages []briefingPage `json:"recent_pages"`
	// Pinned some quando há páginas fixadas (até 10), separadas dos slots.
	Pinned []briefingPage `json:"pinned,omitempty"`
	// Settled existe só com settled_first=true; ponteiro para que a lista vazia apareça como [].
	Settled *[]briefingPage `json:"settled,omitempty"`
}

// Condições SQL das listas do briefing. Caminhos são comparados por substr para que o `_` do
// LIKE não funcione como curinga.
const (
	briefingPinnedCond  = `p.pinned = 1 AND substr(p.path, 1, 7) <> '_slots/'`
	briefingSettledCond = `(substr(p.path, 1, 7) = '_rules/' OR substr(p.path, 1, 10) = 'decisions/' ` +
		`OR json_extract(p.frontmatter_json, '$.kind') IN ('rule', 'decision'))`
	briefingSettledOrder = `evidence_count DESC, p.updated_at DESC`
	briefingRecentOrder  = `p.updated_at DESC`
	briefingPinnedOrder  = `p.updated_at DESC`
)

func (d SessionToolDeps) briefing(ctx context.Context, args json.RawMessage) (ToolResult, error) {
	var in briefingArgs
	if err := decodeToolArgs(args, &in); err != nil {
		return ToolResult{}, err
	}
	res, err := d.resolveToolScope(ctx, strings.TrimSpace(in.Workspace), strings.TrimSpace(in.Project))
	if err != nil {
		return ToolResult{}, err
	}

	out := briefingResult{
		ResolvedBy:  string(res.Source),
		WorkspaceID: res.Scope.WorkspaceID.String(),
		ProjectID:   res.Scope.ProjectID.String(),
		RecentPages: []briefingPage{},
	}
	owner, ownerArgs := ownerFilter(actorUser(ctx))
	ws, pj := res.Scope.WorkspaceID.Bytes(), res.Scope.ProjectID.Bytes()

	err = d.Reader.Read(ctx, func(tx *sql.Tx) error {
		if err := tx.QueryRow(
			`SELECT COUNT(*) FROM pages WHERE workspace_id = ? AND project_id = ? AND is_latest = 1`,
			ws, pj,
		).Scan(&out.Counts.Pages); err != nil {
			return fmt.Errorf("contar páginas: %w", err)
		}
		// Observações contam só as de sessões que o ator pode ver (filtro de dono, DV §8.5).
		if err := tx.QueryRow(
			`SELECT COUNT(*) FROM observations o JOIN sessions s ON s.id = o.session_id
			  WHERE o.workspace_id = ? AND o.project_id = ? AND `+owner,
			append([]any{ws, pj}, ownerArgs...)...,
		).Scan(&out.Counts.Observations); err != nil {
			return fmt.Errorf("contar observações: %w", err)
		}
		if err := tx.QueryRow(
			`SELECT COUNT(*) FROM sessions WHERE workspace_id = ? AND project_id = ? AND `+owner,
			append([]any{ws, pj}, ownerArgs...)...,
		).Scan(&out.Counts.Sessions); err != nil {
			return fmt.Errorf("contar sessões: %w", err)
		}

		recent, err := briefingPageRows(tx, ws, pj, "1 = 1", briefingRecentOrder, briefingRecentLimit)
		if err != nil {
			return err
		}
		out.RecentPages = recent

		pinned, err := briefingPageRows(tx, ws, pj, briefingPinnedCond, briefingPinnedOrder, briefingPinnedLimit)
		if err != nil {
			return err
		}
		if len(pinned) > 0 {
			out.Pinned = pinned
		}

		if in.SettledFirst {
			settled, err := briefingPageRows(tx, ws, pj, briefingSettledCond, briefingSettledOrder, briefingSettledLimit)
			if err != nil {
				return err
			}
			out.Settled = &settled
		}
		return nil
	})
	if err != nil {
		return ToolResult{}, err
	}

	b, err := json.Marshal(out)
	if err != nil {
		return ToolResult{}, errors.New("falha ao serializar briefing")
	}
	return ToolResult{Text: string(b)}, nil
}

// briefingPageRows lista as versões correntes do projeto que casam com cond, na ordem dada.
// cond e order são constantes do pacote, nunca entrada do usuário.
func briefingPageRows(tx *sql.Tx, ws, pj []byte, cond, order string, limit int) ([]briefingPage, error) {
	rows, err := tx.Query(
		`SELECT p.path, p.title, p.tier, p.frontmatter_json, p.updated_at,
		        (SELECT COUNT(*) FROM page_evidence e WHERE e.page_id = p.id) AS evidence_count
		   FROM pages p
		  WHERE p.workspace_id = ? AND p.project_id = ? AND p.is_latest = 1 AND (`+cond+`)
		  ORDER BY `+order+`, p.path ASC
		  LIMIT ?`,
		ws, pj, limit,
	)
	if err != nil {
		return nil, fmt.Errorf("listar páginas do briefing: %w", err)
	}
	defer func() { _ = rows.Close() }()

	out := []briefingPage{}
	for rows.Next() {
		var (
			path, title, tier, fmRaw string
			updatedAt                int64
			evidence                 int
		)
		if err := rows.Scan(&path, &title, &tier, &fmRaw, &updatedAt, &evidence); err != nil {
			return nil, fmt.Errorf("ler página do briefing: %w", err)
		}
		var fm map[string]any
		_ = json.Unmarshal([]byte(fmRaw), &fm)
		kind, _ := fm["kind"].(string)
		out = append(out, briefingPage{
			Path:          path,
			Title:         title,
			Tier:          tier,
			Kind:          kind,
			UpdatedAt:     time.UnixMicro(updatedAt).UTC().Format(time.RFC3339Nano),
			EvidenceCount: evidence,
		})
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterar páginas do briefing: %w", err)
	}
	return out, nil
}
