package mcp

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"runtime/debug"
	"sort"
	"strings"
	"time"

	"github.com/phm-aguiar/ai-memory-go/internal/domain"
	"github.com/phm-aguiar/ai-memory-go/internal/storage"
)

// Limites das ferramentas de leitura (docs/mcp-server.md §3.1 e §3.2; contrato MCP §3).
const (
	// defaultReadLimit é o limite padrão do servidor (MS §2): 10, fixo.
	defaultReadLimit = 10
	maxReadLimit     = 100
	// pinFirstMax é o máximo de páginas fixadas antepostas por pin_first (MS §3.1 passo 8).
	pinFirstMax = 10
	// maxRelatedDepth e maxRelatedNodes limitam o percurso de include_related (MS §3.2).
	maxRelatedDepth = 3
	maxRelatedNodes = 50
	// accessTimeout limita o reforço de acesso em segundo plano (MS §3.1 passo 5).
	accessTimeout = 10 * time.Second
)

// Notas devolvidas em campos próprios; não são erros (MS §3.1 passos 9 e 1).
const (
	answerUnavailableNote = "answer_unavailable: nenhum provedor de LLM configurado nesta versão; resultados sem síntese"
	supersededNote        = "include_superseded: versões substituídas não são consultadas nesta versão; resultados só com versões correntes"
	fallbackWarningFormat = "leitura sem escopo nomeado resolvida por %s; informe workspace e projeto"
)

// reasoningLevels são os valores aceitos por reasoning (MS §3.1 passo 9). Valor fora deste conjunto é recusado.
var reasoningLevels = map[string]bool{
	"minimal": true, "low": true, "medium": true, "high": true, "max": true,
}

// Schemas de entrada (JSON Schema de objeto). Não declaram minimum/maximum nem maxItems: limites são
// aplicados pelo servidor (clamp ou erro, conforme o contrato), não pelo cliente.
const (
	queryInputSchema = `{"type":"object","additionalProperties":false,"properties":{
"query":{"type":"string","description":"Consulta em texto livre. Aliases: q e search."},
"q":{"type":"string","description":"Alias de query."},
"search":{"type":"string","description":"Alias de query."},
"workspace":{"type":"string","description":"Workspace do escopo. Vai junto com project."},
"project":{"type":"string","description":"Projeto do escopo. Vai junto com workspace."},
"scopes":{"type":"array","description":"Pares {workspace, project} já existentes. No máximo 25. Não combina com workspace ou project.","items":{"type":"object","additionalProperties":false,"required":["workspace","project"],"properties":{"workspace":{"type":"string"},"project":{"type":"string"}}}},
"global":{"type":"boolean","description":"Busca FTS em todos os projetos, com rótulo de workspace e projeto em cada hit. Não combina com workspace, project ou scopes."},
"limit":{"type":"integer","description":"Quantidade de resultados. Padrão 10; valores fora de 1..100 são limitados."},
"n":{"type":"integer","description":"Alias de limit. Se limit vier também, limit vence."},
"top_k":{"type":"integer","description":"Alias de limit. Se limit vier também, limit vence."},
"include_expired":{"type":"boolean","description":"Inclui páginas com validade vencida."},
"include_superseded":{"type":"boolean","description":"Pedido de versões substituídas (sem efeito nesta versão; ver notes)."},
"explain":{"type":"boolean","description":"Acrescenta score_details por hit e streams_active."},
"pin_first":{"type":"boolean","description":"Antepõe até 10 páginas fixadas do projeto (só em projeto único)."},
"answer":{"type":"boolean","description":"Pede síntese. Sem provedor, devolve os resultados com answer_unavailable."},
"reasoning":{"type":"string","enum":["minimal","low","medium","high","max"],"description":"Orçamento da síntese. Valor desconhecido é recusado."}
}}`

	recentInputSchema = `{"type":"object","additionalProperties":false,"properties":{
"limit":{"type":"integer","description":"Quantidade de páginas. Padrão 10; valores fora de 1..100 são limitados."},
"n":{"type":"integer","description":"Alias de limit. Se limit vier também, limit vence."},
"workspace":{"type":"string","description":"Workspace do escopo. Vai junto com project."},
"project":{"type":"string","description":"Projeto do escopo. Vai junto com workspace."}
}}`

	readPageInputSchema = `{"type":"object","additionalProperties":false,"properties":{
"path":{"type":"string","description":"Caminho exato da página. Tem precedência sobre query."},
"query":{"type":"string","description":"Busca FTS; devolve o corpo completo do primeiro resultado."},
"workspace":{"type":"string","description":"Workspace do escopo. Vai junto com project."},
"project":{"type":"string","description":"Projeto do escopo. Vai junto com workspace."},
"include_related":{"type":"boolean","description":"Inclui related: páginas ligadas por links e backlinks."},
"related_depth":{"type":"integer","description":"Profundidade de related. Padrão 1; máximo 3."}
}}`

	statusInputSchema = `{"type":"object","additionalProperties":false,"properties":{
"workspace":{"type":"string","description":"Workspace do escopo. Vai junto com project."},
"project":{"type":"string","description":"Projeto do escopo. Vai junto com workspace."}
}}`
)

// ReadDeps são as dependências das ferramentas de leitura. Reader é obrigatório. Os demais são opcionais:
// Writer nil desliga o reforço de acesso; Pointer nil equivale a ponteiro Unset; WikiRoot vazio desliga
// served_from; DefaultLimit zero vale 10; Now nil usa o relógio atual.
type ReadDeps struct {
	Reader       storage.Reader
	Writer       storage.Writer
	Access       *storage.AccessThrottle
	Pointer      *storage.ActiveProjectPointer
	WikiRoot     string
	DefaultLimit int
	Now          func() time.Time
}

// RegisterReadTools registra memory_query, memory_recent, memory_read_page e memory_status em srv.
func RegisterReadTools(srv ToolServer, deps ReadDeps) error {
	if srv == nil {
		return errors.New("mcp: ferramentas de leitura exigem servidor")
	}
	if deps.Reader == nil {
		return errors.New("mcp: ferramentas de leitura exigem Reader")
	}
	t := newReadTools(deps)
	tools := []struct {
		def     ToolDef
		handler ToolHandler
	}{
		{ToolDef{Name: "memory_query", Description: "Busca páginas da memória por texto, entidades e links. Escopo por workspace e project, scopes ou global.", InputSchema: json.RawMessage(queryInputSchema)}, t.query},
		{ToolDef{Name: "memory_recent", Description: "Páginas mais recentes do escopo, por updated_at decrescente.", InputSchema: json.RawMessage(recentInputSchema)}, t.recent},
		{ToolDef{Name: "memory_read_page", Description: "Lê uma página por path ou por query. include_related devolve as páginas ligadas.", InputSchema: json.RawMessage(readPageInputSchema)}, t.readPage},
		{ToolDef{Name: "memory_status", Description: "Contagens do escopo e origem da escolha de escopo (resolved_by).", InputSchema: json.RawMessage(statusInputSchema)}, t.status},
	}
	for _, tool := range tools {
		if err := srv.AddTool(tool.def, tool.handler); err != nil {
			return err
		}
	}
	return nil
}

// readTools guarda as dependências já normalizadas das ferramentas de leitura.
type readTools struct {
	reader       storage.Reader
	writer       storage.Writer
	access       *storage.AccessThrottle
	pointer      *storage.ActiveProjectPointer
	wikiRoot     string
	defaultLimit int
	now          func() time.Time
}

func newReadTools(deps ReadDeps) *readTools {
	t := &readTools{
		reader:       deps.Reader,
		writer:       deps.Writer,
		access:       deps.Access,
		pointer:      deps.Pointer,
		wikiRoot:     deps.WikiRoot,
		defaultLimit: deps.DefaultLimit,
		now:          deps.Now,
	}
	if t.access == nil {
		t.access = storage.NewAccessThrottle()
	}
	if t.defaultLimit <= 0 {
		t.defaultLimit = defaultReadLimit
	}
	if t.now == nil {
		t.now = time.Now
	}
	return t
}

// scopeLabel são os nomes humanos de um escopo, usados nos rótulos de hit.
type scopeLabel struct {
	ws, pj string
}

// scopeTarget é um escopo resolvido com seus nomes e a origem da escolha (vazia em scopes).
type scopeTarget struct {
	scope  domain.Scope
	label  scopeLabel
	source storage.ScopeSource
}

// candidate é um resultado antes da hidratação. Os campos de pontuação só valem quando scored
// (ranking do Search) ou hasPosition (posição no fluxo FTS da consulta global).
type candidate struct {
	ref         domain.PageRef
	label       scopeLabel
	pinned      bool
	scored      bool
	rank        float64
	authority   float64
	hasPosition bool
	position    int
	streams     []string
}

// hitOut é um hit de página na resposta.
type hitOut struct {
	Path         string         `json:"path"`
	Title        string         `json:"title"`
	Kind         string         `json:"kind"`
	Tier         string         `json:"tier"`
	Body         string         `json:"body"`
	Workspace    string         `json:"workspace"`
	Project      string         `json:"project"`
	WorkspaceID  string         `json:"workspace_id"`
	ProjectID    string         `json:"project_id"`
	Pinned       bool           `json:"pinned,omitempty"`
	ScoreDetails map[string]any `json:"score_details,omitempty"`
}

// rawHitOut é uma observação casada pelo fallback de observações.
type rawHitOut struct {
	ObservationID string  `json:"observation_id"`
	SessionID     string  `json:"session_id"`
	Kind          string  `json:"kind"`
	Title         string  `json:"title"`
	Snippet       string  `json:"snippet"`
	Rank          float64 `json:"rank"`
	Workspace     string  `json:"workspace"`
	Project       string  `json:"project"`
}

// scopeArgs é um par de uma lista scopes.
type scopeArgs struct {
	Workspace string `json:"workspace"`
	Project   string `json:"project"`
}

// queryArgs são os argumentos de memory_query. Ponteiros distinguem ausente de zero nos pares com alias.
type queryArgs struct {
	Query             *string      `json:"query"`
	Q                 *string      `json:"q"`
	Search            *string      `json:"search"`
	Workspace         string       `json:"workspace"`
	Project           string       `json:"project"`
	Scopes            *[]scopeArgs `json:"scopes"`
	Global            *bool        `json:"global"`
	Limit             *int         `json:"limit"`
	N                 *int         `json:"n"`
	TopK              *int         `json:"top_k"`
	IncludeExpired    bool         `json:"include_expired"`
	IncludeSuperseded bool         `json:"include_superseded"`
	Explain           bool         `json:"explain"`
	PinFirst          bool         `json:"pin_first"`
	Answer            bool         `json:"answer"`
	Reasoning         *string      `json:"reasoning"`
}

type recentArgs struct {
	Limit     *int   `json:"limit"`
	N         *int   `json:"n"`
	Workspace string `json:"workspace"`
	Project   string `json:"project"`
}

type readPageArgs struct {
	Path           string `json:"path"`
	Query          string `json:"query"`
	Workspace      string `json:"workspace"`
	Project        string `json:"project"`
	IncludeRelated bool   `json:"include_related"`
	RelatedDepth   *int   `json:"related_depth"`
}

type statusArgs struct {
	Workspace string `json:"workspace"`
	Project   string `json:"project"`
}

// relatedNode é um item de related (MS §3.2).
type relatedNode struct {
	Path      string `json:"path"`
	Title     string `json:"title"`
	Kind      string `json:"kind"`
	Workspace string `json:"workspace"`
	Project   string `json:"project"`
	Depth     int    `json:"depth"`
	Direction string `json:"direction"`
	version   domain.PageID
}

// ---------------------------------------------------------------------------
// memory_query
// ---------------------------------------------------------------------------

func (t *readTools) query(ctx context.Context, raw json.RawMessage) (ToolResult, error) {
	var args queryArgs
	if err := decodeArgs(raw, &args); err != nil {
		return invalidArg("argumentos: " + err.Error())
	}
	q := strings.TrimSpace(firstNonEmpty(args.Query, args.Q, args.Search))
	if q == "" {
		return invalidArg("query é obrigatória (aliases q e search)")
	}
	if args.Reasoning != nil && !reasoningLevels[*args.Reasoning] {
		return invalidArg("reasoning deve ser minimal, low, medium, high ou max")
	}
	limit := clampLimit(firstInt(args.Limit, args.N, args.TopK), t.defaultLimit)

	hasPair := args.Workspace != "" || args.Project != ""
	if args.Scopes != nil {
		if hasPair {
			return invalidArg("scopes não combina com workspace ou project")
		}
		if len(*args.Scopes) == 0 {
			return invalidArg("scopes não pode ser vazio")
		}
		if len(*args.Scopes) > storage.MaxScopeList {
			return invalidArg(fmt.Sprintf("scopes aceita no máximo %d pares", storage.MaxScopeList))
		}
	}
	hasScope := hasPair || args.Scopes != nil

	actor := actorOf(ctx)
	now := t.now()
	cutoff := now
	if args.IncludeExpired {
		// Corte no início do tempo: nenhuma página é considerada vencida.
		cutoff = time.Time{}
	}

	global := false
	switch {
	case args.Global != nil:
		global = *args.Global
	case !hasScope:
		global = t.pointerDefaultGlobal(actor, now)
	}
	if global && hasScope {
		return invalidArg("global não combina com workspace, project ou scopes")
	}
	if global {
		return t.queryGlobal(ctx, q, limit, cutoff, actor, args)
	}

	single := args.Scopes == nil
	targets, source, err := t.queryTargets(ctx, args)
	if err != nil {
		return toolFailure("memory_query", err)
	}

	var cands []candidate
	raws := []rawHitOut{}
	for _, tg := range targets {
		sr, err := storage.Search(ctx, t.reader, tg.scope, q, cutoff, limit)
		if err != nil {
			return toolFailure("memory_query", err)
		}
		var membership map[domain.PageRef][]string
		if args.Explain {
			membership, err = streamMembership(ctx, t.reader, tg.scope, q, cutoff, limit)
			if err != nil {
				return toolFailure("memory_query", err)
			}
		}
		for _, p := range sr.Pages {
			cands = append(cands, candidate{
				ref: p.Ref, label: tg.label, scored: true, rank: p.Rank, authority: p.Authority,
				streams: membership[p.Ref],
			})
		}
		for _, rh := range sr.RawHits {
			raws = append(raws, rawHitOut{
				ObservationID: rh.ObservationID.String(), SessionID: rh.SessionID.String(),
				Kind: string(rh.Kind), Title: rh.Title, Snippet: rh.Snippet, Rank: rh.Rank,
				Workspace: tg.label.ws, Project: tg.label.pj,
			})
		}
	}
	if !single {
		sort.SliceStable(cands, func(i, j int) bool { return candidateLess(cands[i], cands[j]) })
		if len(cands) > limit {
			cands = cands[:limit]
		}
		sort.SliceStable(raws, func(i, j int) bool { return raws[i].Rank < raws[j].Rank })
		if len(raws) > limit {
			raws = raws[:limit]
		}
	}

	if args.PinFirst && single {
		pinned, err := t.pinnedCandidates(ctx, targets[0], cutoff, min(pinFirstMax, limit))
		if err != nil {
			return toolFailure("memory_query", err)
		}
		cands = prependPinned(pinned, cands, limit)
	}

	hits, versions, err := t.hydrate(ctx, cands, args.Explain)
	if err != nil {
		return toolFailure("memory_query", err)
	}
	out := map[string]any{"hits": hits, "raw_hits": raws}
	if single {
		out["scope"] = scopeJSON(targets[0], source)
		gHits, gVersions, err := t.globalScopeHits(ctx, targets[0].scope, q, cutoff, limit)
		if err != nil {
			return toolFailure("memory_query", err)
		}
		out["global_scope_hits"] = gHits
		versions = append(versions, gVersions...)
	}
	if args.Explain {
		out["streams_active"] = []string{"fts", "entity", "graph"}
	}
	if args.Answer {
		out["answer_unavailable"] = answerUnavailableNote
	}
	if args.IncludeSuperseded {
		out["notes"] = []string{supersededNote}
	}
	t.reinforce(actor, versions)
	return jsonResult(out)
}

// queryTargets resolve os escopos de memory_query: um só (workspace e project, ou ponteiro) ou a lista scopes.
func (t *readTools) queryTargets(ctx context.Context, args queryArgs) ([]scopeTarget, storage.ScopeSource, error) {
	if args.Scopes == nil {
		res, err := t.resolve(ctx, args.Workspace, args.Project)
		if err != nil {
			return nil, "", err
		}
		lbl, err := t.label(ctx, res.Scope)
		if err != nil {
			return nil, "", err
		}
		t.logFallback("memory_query", res)
		return []scopeTarget{{scope: res.Scope, label: lbl, source: res.Source}}, res.Source, nil
	}
	refs := make([]storage.ScopeRef, 0, len(*args.Scopes))
	for _, s := range *args.Scopes {
		refs = append(refs, storage.ScopeRef{WorkspaceName: s.Workspace, ProjectName: s.Project})
	}
	scopes, err := storage.ResolveScopeList(ctx, t.reader, refs)
	if err != nil {
		return nil, "", err
	}
	targets := make([]scopeTarget, 0, len(scopes))
	for _, sc := range scopes {
		lbl, err := t.label(ctx, sc)
		if err != nil {
			return nil, "", err
		}
		targets = append(targets, scopeTarget{scope: sc, label: lbl})
	}
	return targets, "", nil
}

// queryGlobal é a consulta global: FTS sem RRF em todos os projetos, ordenada pela posição no fluxo
// de cada projeto e depois pelo rótulo. Corta no limite.
func (t *readTools) queryGlobal(ctx context.Context, q string, limit int, cutoff time.Time, actor storage.ActorKey, args queryArgs) (ToolResult, error) {
	scopes, err := t.allScopes(ctx)
	if err != nil {
		return toolFailure("memory_query", err)
	}
	var cands []candidate
	for _, tg := range scopes {
		refs, err := storage.FTSFlow(ctx, t.reader, tg.scope, q, cutoff, storage.CandidateLimit(limit))
		if err != nil {
			return toolFailure("memory_query", err)
		}
		for i, ref := range refs {
			cands = append(cands, candidate{
				ref: ref, label: tg.label, hasPosition: true, position: i, streams: []string{"fts"},
			})
		}
	}
	sort.SliceStable(cands, func(i, j int) bool {
		a, b := cands[i], cands[j]
		if a.position != b.position {
			return a.position < b.position
		}
		if a.label.ws != b.label.ws {
			return a.label.ws < b.label.ws
		}
		if a.label.pj != b.label.pj {
			return a.label.pj < b.label.pj
		}
		return a.ref.Path.String() < b.ref.Path.String()
	})
	if len(cands) > limit {
		cands = cands[:limit]
	}
	hits, versions, err := t.hydrate(ctx, cands, args.Explain)
	if err != nil {
		return toolFailure("memory_query", err)
	}
	out := map[string]any{"global_hits": hits}
	if args.Explain {
		out["streams_active"] = []string{"fts"}
	}
	if args.Answer {
		out["answer_unavailable"] = answerUnavailableNote
	}
	t.reinforce(actor, versions)
	return jsonResult(out)
}

// globalScopeHits busca o escopo reservado _global e o une ao resultado de um projeto único.
// Sem _global, ou quando o projeto já é o próprio _global, não há hits extras.
func (t *readTools) globalScopeHits(ctx context.Context, own domain.Scope, q string, cutoff time.Time, limit int) ([]hitOut, []domain.PageID, error) {
	gs, err := storage.ResolveGlobalScope(ctx, t.reader)
	if errors.Is(err, storage.ErrScopeNotFound) {
		return []hitOut{}, nil, nil
	}
	if err != nil {
		return nil, nil, err
	}
	if gs == own {
		return []hitOut{}, nil, nil
	}
	sr, err := storage.Search(ctx, t.reader, gs, q, cutoff, limit)
	if err != nil {
		return nil, nil, err
	}
	lbl, err := t.label(ctx, gs)
	if err != nil {
		return nil, nil, err
	}
	cands := make([]candidate, 0, len(sr.Pages))
	for _, p := range sr.Pages {
		cands = append(cands, candidate{ref: p.Ref, label: lbl, scored: true, rank: p.Rank, authority: p.Authority})
	}
	return t.hydrate(ctx, cands, false)
}

// pinnedCandidates lista até n páginas fixadas e não vencidas do escopo, da mais recente para a mais antiga.
func (t *readTools) pinnedCandidates(ctx context.Context, tg scopeTarget, cutoff time.Time, n int) ([]candidate, error) {
	out := []candidate{}
	err := t.reader.Read(ctx, func(tx *sql.Tx) error {
		rows, err := tx.QueryContext(ctx,
			`SELECT path FROM pages
			  WHERE workspace_id = ? AND project_id = ? AND is_latest = 1 AND pinned = 1
			    AND (expires_at IS NULL OR expires_at > ?)
			  ORDER BY updated_at DESC, path ASC LIMIT ?`,
			tg.scope.WorkspaceID.Bytes(), tg.scope.ProjectID.Bytes(), cutoff.UnixMicro(), n,
		)
		if err != nil {
			return fmt.Errorf("listar fixadas: %w", err)
		}
		defer func() { _ = rows.Close() }()
		for rows.Next() {
			var path string
			if err := rows.Scan(&path); err != nil {
				return fmt.Errorf("ler fixada: %w", err)
			}
			pp, err := domain.ParseStoredPagePath(path)
			if err != nil {
				return fmt.Errorf("caminho fixado inválido: %w", err)
			}
			out = append(out, candidate{ref: domain.NewPageRef(tg.scope, pp), label: tg.label, pinned: true})
		}
		return rows.Err()
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

// prependPinned põe as fixadas na frente, remove duplicatas por referência e corta no limite.
func prependPinned(pinned, rest []candidate, limit int) []candidate {
	seen := make(map[domain.PageRef]bool, len(pinned)+len(rest))
	out := make([]candidate, 0, len(pinned)+len(rest))
	for _, group := range [][]candidate{pinned, rest} {
		for _, c := range group {
			if seen[c.ref] {
				continue
			}
			seen[c.ref] = true
			out = append(out, c)
		}
	}
	if len(out) > limit {
		out = out[:limit]
	}
	return out
}

// streamMembership diz, para cada página candidata do escopo, em quais fluxos ela aparece (FTS, entidade e
// grafo). Só é usada com explain.
func streamMembership(ctx context.Context, r storage.Reader, scope domain.Scope, q string, cutoff time.Time, limit int) (map[domain.PageRef][]string, error) {
	cand := storage.CandidateLimit(limit)
	fts, err := storage.FTSFlow(ctx, r, scope, q, cutoff, cand)
	if err != nil {
		return nil, err
	}
	ent, err := storage.EntityFlow(ctx, r, scope, q, cutoff, cand)
	if err != nil {
		return nil, err
	}
	graph, err := storage.GraphFlow(ctx, r, scope, storage.SeedsFromFlows(fts, ent), cutoff, cand)
	if err != nil {
		return nil, err
	}
	m := make(map[domain.PageRef][]string)
	for _, flow := range []struct {
		name string
		refs []domain.PageRef
	}{{"fts", fts}, {"entity", ent}, {"graph", graph}} {
		for _, ref := range flow.refs {
			m[ref] = append(m[ref], flow.name)
		}
	}
	return m, nil
}

// hydrate lê cada candidato pela leitura da 001 e monta os hits. Página que sumiu entre a busca e a
// leitura é descartada sem erro. Devolve também as versões lidas, para o reforço de acesso.
func (t *readTools) hydrate(ctx context.Context, cands []candidate, explain bool) ([]hitOut, []domain.PageID, error) {
	hits := make([]hitOut, 0, len(cands))
	versions := make([]domain.PageID, 0, len(cands))
	for _, c := range cands {
		page, err := storage.ReadPage(ctx, t.reader, c.ref)
		if errors.Is(err, storage.ErrPageNotFound) {
			continue
		}
		if err != nil {
			return nil, nil, err
		}
		h := hitOut{
			Path:        page.Ref.Path.String(),
			Title:       page.Title,
			Kind:        string(page.Kind),
			Tier:        string(page.Tier),
			Body:        page.Body,
			Workspace:   c.label.ws,
			Project:     c.label.pj,
			WorkspaceID: c.ref.Scope.WorkspaceID.String(),
			ProjectID:   c.ref.Scope.ProjectID.String(),
			Pinned:      c.pinned,
		}
		if explain {
			streams := c.streams
			if streams == nil {
				streams = []string{}
			}
			details := map[string]any{"streams": streams}
			if c.scored {
				details["rank"] = c.rank
				details["authority"] = c.authority
			}
			if c.hasPosition {
				details["position"] = c.position
			}
			h.ScoreDetails = details
		}
		hits = append(hits, h)
		versions = append(versions, page.Version)
	}
	return hits, versions, nil
}

// allScopes lista todos os escopos (workspace e projeto) com os nomes, para a consulta global.
func (t *readTools) allScopes(ctx context.Context) ([]scopeTarget, error) {
	out := []scopeTarget{}
	err := t.reader.Read(ctx, func(tx *sql.Tx) error {
		rows, err := tx.QueryContext(ctx,
			`SELECT w.id, p.id, w.name, p.name
			   FROM projects p JOIN workspaces w ON w.id = p.workspace_id
			  ORDER BY w.name, p.name`,
		)
		if err != nil {
			return fmt.Errorf("listar escopos: %w", err)
		}
		defer func() { _ = rows.Close() }()
		for rows.Next() {
			var wsB, pjB []byte
			var lbl scopeLabel
			if err := rows.Scan(&wsB, &pjB, &lbl.ws, &lbl.pj); err != nil {
				return fmt.Errorf("ler escopo: %w", err)
			}
			ws, err := domain.ParseIDBytes(wsB)
			if err != nil {
				return fmt.Errorf("id do workspace: %w", err)
			}
			pj, err := domain.ParseIDBytes(pjB)
			if err != nil {
				return fmt.Errorf("id do projeto: %w", err)
			}
			out = append(out, scopeTarget{scope: domain.NewScope(domain.WorkspaceID(ws), domain.ProjectID(pj)), label: lbl})
		}
		return rows.Err()
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

// ---------------------------------------------------------------------------
// memory_recent
// ---------------------------------------------------------------------------

func (t *readTools) recent(ctx context.Context, raw json.RawMessage) (ToolResult, error) {
	var args recentArgs
	if err := decodeArgs(raw, &args); err != nil {
		return invalidArg("argumentos: " + err.Error())
	}
	limit := clampLimit(firstInt(args.Limit, args.N), t.defaultLimit)
	res, err := t.resolve(ctx, args.Workspace, args.Project)
	if err != nil {
		return toolFailure("memory_recent", err)
	}
	t.logFallback("memory_recent", res)
	lbl, err := t.label(ctx, res.Scope)
	if err != nil {
		return toolFailure("memory_recent", err)
	}

	type row struct {
		path      string
		updatedAt int64
	}
	var rows []row
	err = t.reader.Read(ctx, func(tx *sql.Tx) error {
		rs, err := tx.QueryContext(ctx,
			`SELECT path, updated_at FROM pages
			  WHERE workspace_id = ? AND project_id = ? AND is_latest = 1
			    AND (expires_at IS NULL OR expires_at > ?)
			  ORDER BY updated_at DESC, path ASC LIMIT ?`,
			res.Scope.WorkspaceID.Bytes(), res.Scope.ProjectID.Bytes(), t.now().UnixMicro(), limit,
		)
		if err != nil {
			return fmt.Errorf("listar recentes: %w", err)
		}
		defer func() { _ = rs.Close() }()
		for rs.Next() {
			var r row
			if err := rs.Scan(&r.path, &r.updatedAt); err != nil {
				return fmt.Errorf("ler recente: %w", err)
			}
			rows = append(rows, r)
		}
		return rs.Err()
	})
	if err != nil {
		return toolFailure("memory_recent", err)
	}

	type recentOut struct {
		hitOut
		UpdatedAt string `json:"updated_at"`
	}
	pages := make([]recentOut, 0, len(rows))
	for _, r := range rows {
		pp, err := domain.ParseStoredPagePath(r.path)
		if err != nil {
			return toolFailure("memory_recent", err)
		}
		page, err := storage.ReadPage(ctx, t.reader, domain.NewPageRef(res.Scope, pp))
		if errors.Is(err, storage.ErrPageNotFound) {
			continue
		}
		if err != nil {
			return toolFailure("memory_recent", err)
		}
		pages = append(pages, recentOut{
			hitOut: hitOut{
				Path: page.Ref.Path.String(), Title: page.Title, Kind: string(page.Kind), Tier: string(page.Tier),
				Body: page.Body, Workspace: lbl.ws, Project: lbl.pj,
				WorkspaceID: res.Scope.WorkspaceID.String(), ProjectID: res.Scope.ProjectID.String(),
			},
			UpdatedAt: time.UnixMicro(r.updatedAt).UTC().Format(time.RFC3339Nano),
		})
	}
	return jsonResult(map[string]any{
		"pages": pages,
		"scope": scopeJSON(scopeTarget{scope: res.Scope, label: lbl}, res.Source),
	})
}

// ---------------------------------------------------------------------------
// memory_read_page
// ---------------------------------------------------------------------------

func (t *readTools) readPage(ctx context.Context, raw json.RawMessage) (ToolResult, error) {
	var args readPageArgs
	if err := decodeArgs(raw, &args); err != nil {
		return invalidArg("argumentos: " + err.Error())
	}
	path := strings.TrimSpace(args.Path)
	query := strings.TrimSpace(args.Query)
	if path == "" && query == "" {
		return invalidArg("informe path ou query")
	}
	depth := 1
	if args.RelatedDepth != nil {
		depth = min(max(*args.RelatedDepth, 1), maxRelatedDepth)
	}

	actor := actorOf(ctx)
	now := t.now()
	res, err := t.resolve(ctx, args.Workspace, args.Project)
	if err != nil {
		return toolFailure("memory_read_page", err)
	}
	t.logFallback("memory_read_page", res)

	var ref domain.PageRef
	if path != "" {
		pp, err := domain.ParsePagePath(path)
		if err != nil {
			return invalidArg("caminho de página inválido")
		}
		ref = domain.NewPageRef(res.Scope, pp)
	} else {
		sr, err := storage.Search(ctx, t.reader, res.Scope, query, now, 1)
		if err != nil {
			return toolFailure("memory_read_page", err)
		}
		if len(sr.Pages) == 0 {
			return failure("não encontrado: nenhuma página casa com a consulta")
		}
		ref = sr.Pages[0].Ref
	}

	page, err := storage.ReadPage(ctx, t.reader, ref)
	if err != nil {
		return toolFailure("memory_read_page", err)
	}
	lbl, err := t.label(ctx, res.Scope)
	if err != nil {
		return toolFailure("memory_read_page", err)
	}

	out := map[string]any{
		"path":         page.Ref.Path.String(),
		"title":        page.Title,
		"body":         page.Body,
		"frontmatter":  frontmatterMap(page.Frontmatter),
		"kind":         string(page.Kind),
		"tier":         string(page.Tier),
		"workspace":    lbl.ws,
		"project":      lbl.pj,
		"workspace_id": res.Scope.WorkspaceID.String(),
		"project_id":   res.Scope.ProjectID.String(),
		"scope":        scopeJSON(scopeTarget{scope: res.Scope, label: lbl}, res.Source),
	}
	if t.servedFromDatabase(res.Scope, page.Ref.Path.String()) {
		out["served_from"] = "database"
	}

	versions := []domain.PageID{page.Version}
	if args.IncludeRelated {
		related, relVersions, err := t.related(ctx, page.Version, depth, now)
		if err != nil {
			return toolFailure("memory_read_page", err)
		}
		out["related"] = related
		versions = append(versions, relVersions...)
	}
	t.reinforce(actor, versions)
	return jsonResult(out)
}

// servedFromDatabase diz se o arquivo Markdown da página falta na wiki, caso em que o corpo vem só do banco.
// Sem raiz da wiki configurada, não há como verificar, e a resposta não traz o campo.
func (t *readTools) servedFromDatabase(scope domain.Scope, path string) bool {
	if t.wikiRoot == "" {
		return false
	}
	full := filepath.Join(t.wikiRoot, scope.WorkspaceID.String(), scope.ProjectID.String(), filepath.FromSlash(path))
	_, err := os.Stat(full)
	return errors.Is(err, os.ErrNotExist)
}

// related percorre o grafo de links a partir da versão corrente da página, em largura, até depth saltos e
// no máximo maxRelatedNodes nós. Atravessa projetos e workspaces. Devolve os nós e as versões encontradas.
func (t *readTools) related(ctx context.Context, start domain.PageID, depth int, now time.Time) ([]relatedNode, []domain.PageID, error) {
	cutoff := now.UnixMicro()
	seen := map[string]bool{string(start.Bytes()): true}
	out := []relatedNode{}
	var versions []domain.PageID

	frontier := [][]byte{start.Bytes()}
	for level := 1; level <= depth && len(frontier) > 0; level++ {
		var next [][]byte
		for _, cur := range frontier {
			for _, outgoing := range []bool{true, false} {
				neighbors, err := t.neighbors(ctx, cur, outgoing, cutoff)
				if err != nil {
					return nil, nil, err
				}
				for _, nb := range neighbors {
					if seen[string(nb.id)] {
						continue
					}
					seen[string(nb.id)] = true
					page, err := storage.ReadPage(ctx, t.reader, nb.ref)
					if errors.Is(err, storage.ErrPageNotFound) {
						continue
					}
					if err != nil {
						return nil, nil, err
					}
					direction := "backlink"
					if outgoing {
						direction = "link"
					}
					out = append(out, relatedNode{
						Path: page.Ref.Path.String(), Title: page.Title, Kind: string(page.Kind),
						Workspace: nb.ws, Project: nb.pj, Depth: level, Direction: direction,
						version: page.Version,
					})
					versions = append(versions, page.Version)
					next = append(next, nb.id)
					if len(out) >= maxRelatedNodes {
						return out, versions, nil
					}
				}
			}
		}
		frontier = next
	}
	return out, versions, nil
}

// neighbor é uma página vizinha no grafo de links, com o id da versão corrente e o rótulo de escopo.
type neighbor struct {
	id     []byte
	ref    domain.PageRef
	ws, pj string
}

// neighbors lista os vizinhos correntes e não vencidos de uma versão: destinos dos links de saída
// (outgoing) ou origens dos links de entrada. Links pendentes não entram.
func (t *readTools) neighbors(ctx context.Context, id []byte, outgoing bool, cutoff int64) ([]neighbor, error) {
	query := relatedInSQL
	if outgoing {
		query = relatedOutSQL
	}
	var out []neighbor
	err := t.reader.Read(ctx, func(tx *sql.Tx) error {
		rows, err := tx.QueryContext(ctx, query, id, cutoff)
		if err != nil {
			return fmt.Errorf("vizinhos de links: %w", err)
		}
		defer func() { _ = rows.Close() }()
		for rows.Next() {
			var pageB, wsB, pjB []byte
			var path string
			var nb neighbor
			if err := rows.Scan(&pageB, &wsB, &pjB, &path, &nb.ws, &nb.pj); err != nil {
				return fmt.Errorf("ler vizinho: %w", err)
			}
			ws, err := domain.ParseIDBytes(wsB)
			if err != nil {
				return fmt.Errorf("id do workspace do vizinho: %w", err)
			}
			pj, err := domain.ParseIDBytes(pjB)
			if err != nil {
				return fmt.Errorf("id do projeto do vizinho: %w", err)
			}
			pp, err := domain.ParseStoredPagePath(path)
			if err != nil {
				return fmt.Errorf("caminho do vizinho: %w", err)
			}
			nb.id = pageB
			nb.ref = domain.NewPageRef(domain.NewScope(domain.WorkspaceID(ws), domain.ProjectID(pj)), pp)
			out = append(out, nb)
		}
		return rows.Err()
	})
	return out, err
}

// relatedOutSQL: destinos resolvidos dos links de saída da página. Ordem: mais recente primeiro, depois caminho.
const relatedOutSQL = `SELECT p.id, p.workspace_id, p.project_id, p.path, w.name, pj.name
	FROM links l
	JOIN pages p ON p.id = l.to_page_id
	JOIN workspaces w ON w.id = p.workspace_id
	JOIN projects pj ON pj.id = p.project_id
	WHERE l.from_page_id = ? AND l.to_page_id IS NOT NULL AND p.is_latest = 1
	  AND (p.expires_at IS NULL OR p.expires_at > ?)
	ORDER BY p.updated_at DESC, p.path ASC`

// relatedInSQL: origens dos links de entrada da página.
const relatedInSQL = `SELECT p.id, p.workspace_id, p.project_id, p.path, w.name, pj.name
	FROM links l
	JOIN pages p ON p.id = l.from_page_id
	JOIN workspaces w ON w.id = p.workspace_id
	JOIN projects pj ON pj.id = p.project_id
	WHERE l.to_page_id = ? AND p.is_latest = 1
	  AND (p.expires_at IS NULL OR p.expires_at > ?)
	ORDER BY p.updated_at DESC, p.path ASC`

// ---------------------------------------------------------------------------
// memory_status
// ---------------------------------------------------------------------------

func (t *readTools) status(ctx context.Context, raw json.RawMessage) (ToolResult, error) {
	var args statusArgs
	if err := decodeArgs(raw, &args); err != nil {
		return invalidArg("argumentos: " + err.Error())
	}
	res, err := t.resolve(ctx, args.Workspace, args.Project)
	if err != nil {
		return toolFailure("memory_status", err)
	}
	warnings := []string{}
	if res.Source.IsFallback() {
		warnings = append(warnings, fmt.Sprintf(fallbackWarningFormat, res.Source))
	}
	t.logFallback("memory_status", res)
	lbl, err := t.label(ctx, res.Scope)
	if err != nil {
		return toolFailure("memory_status", err)
	}

	counts := map[string]int{}
	err = t.reader.Read(ctx, func(tx *sql.Tx) error {
		wsB, pjB := res.Scope.WorkspaceID.Bytes(), res.Scope.ProjectID.Bytes()
		for name, q := range map[string]string{
			"pages":         `SELECT COUNT(*) FROM pages WHERE workspace_id = ? AND project_id = ? AND is_latest = 1`,
			"observations":  `SELECT COUNT(*) FROM observations WHERE workspace_id = ? AND project_id = ?`,
			"sessions":      `SELECT COUNT(*) FROM sessions WHERE workspace_id = ? AND project_id = ?`,
			"open_handoffs": `SELECT COUNT(*) FROM handoffs WHERE workspace_id = ? AND project_id = ? AND state = 'open'`,
		} {
			var n int
			if err := tx.QueryRowContext(ctx, q, wsB, pjB).Scan(&n); err != nil {
				return fmt.Errorf("contar %s: %w", name, err)
			}
			counts[name] = n
		}
		return nil
	})
	if err != nil {
		return toolFailure("memory_status", err)
	}

	out := map[string]any{
		"server":           ServerName,
		"version":          serverImplVersion,
		"protocol_version": ProtocolVersion,
		"scope":            scopeJSON(scopeTarget{scope: res.Scope, label: lbl}, res.Source),
		"counts":           counts,
	}
	if len(warnings) > 0 {
		out["warnings"] = warnings
	}
	return jsonResult(out)
}

// ---------------------------------------------------------------------------
// Auxiliares comuns
// ---------------------------------------------------------------------------

// resolve resolve o escopo de leitura pelo resolvedor da 001, usando o ator do contexto e o ponteiro.
func (t *readTools) resolve(ctx context.Context, ws, pj string) (storage.ScopeResolution, error) {
	return storage.ResolveReadScope(ctx, t.reader, storage.ScopeRequest{
		WorkspaceName: ws,
		ProjectName:   pj,
		Pointer:       t.pointer,
		Actor:         actorOf(ctx),
		Now:           t.now(),
	})
}

// label lê os nomes de workspace e projeto de um escopo já resolvido.
func (t *readTools) label(ctx context.Context, scope domain.Scope) (scopeLabel, error) {
	var lbl scopeLabel
	err := t.reader.Read(ctx, func(tx *sql.Tx) error {
		return tx.QueryRowContext(ctx,
			`SELECT w.name, p.name FROM projects p JOIN workspaces w ON w.id = p.workspace_id
			  WHERE p.id = ? AND p.workspace_id = ?`,
			scope.ProjectID.Bytes(), scope.WorkspaceID.Bytes(),
		).Scan(&lbl.ws, &lbl.pj)
	})
	if err != nil {
		return scopeLabel{}, fmt.Errorf("ler nomes do escopo: %w", err)
	}
	return lbl, nil
}

// pointerDefaultGlobal diz se o ponteiro do ator ativa a busca global por padrão. Só vale quando a
// origem vem do próprio ponteiro (sessão, slot compartilhado ou semente).
func (t *readTools) pointerDefaultGlobal(actor storage.ActorKey, now time.Time) bool {
	if t.pointer == nil {
		return false
	}
	rr := t.pointer.Read(actor, now)
	switch rr.Origin {
	case storage.ReadOriginSession, storage.ReadOriginSharedSlot, storage.ReadOriginStartupSeed:
		return rr.DefaultGlobal
	}
	return false
}

// logFallback registra o aviso de substituição de fallback (MS §2.1). Não altera a resposta.
func (t *readTools) logFallback(tool string, res storage.ScopeResolution) {
	if res.Source.IsFallback() {
		slog.Warn("escopo de leitura resolvido por fallback", "tool", tool, "resolved_by", string(res.Source))
	}
}

// reinforce incrementa o acesso das versões lidas, em segundo plano, com a janela por operador (MS §3.1
// passo 5). Sem Writer ou sem identidade do operador, não faz nada. Falhas são só registradas.
func (t *readTools) reinforce(actor storage.ActorKey, versions []domain.PageID) {
	if t.writer == nil || actor.Identity == "" || len(versions) == 0 {
		return
	}
	ids := append([]domain.PageID(nil), versions...)
	now := t.now()
	identity := actor.Identity
	go func() {
		defer func() {
			if r := recover(); r != nil {
				slog.Error("pânico no reforço de acesso", "panic", r, "stack", string(debug.Stack()))
			}
		}()
		ctx, cancel := context.WithTimeout(context.Background(), accessTimeout)
		defer cancel()
		for _, id := range ids {
			if _, err := t.access.BumpAccess(ctx, t.writer, id, identity, now); err != nil {
				slog.Warn("reforço de acesso falhou", "err", err)
			}
		}
	}()
}

// actorOf lê a chave do ator do contexto. Sem chave (contexto não preenchido pelo middleware), vale o
// chamador anônimo, que usa o slot compartilhado.
func actorOf(ctx context.Context) storage.ActorKey {
	key, ok := ActorKeyFrom(ctx)
	if !ok {
		return storage.ActorKey{}
	}
	return storage.ActorKey{Identity: key.User, SessionID: key.SessionID}
}

// scopeJSON monta o bloco scope das respostas. resolved_by é a origem da escolha (vazio em scopes).
func scopeJSON(tg scopeTarget, source storage.ScopeSource) map[string]any {
	return map[string]any{
		"workspace":    tg.label.ws,
		"project":      tg.label.pj,
		"workspace_id": tg.scope.WorkspaceID.String(),
		"project_id":   tg.scope.ProjectID.String(),
		"resolved_by":  string(source),
	}
}

// frontmatterMap converte o frontmatter em mapa JSON com os valores como estão armazenados.
func frontmatterMap(fm domain.Frontmatter) map[string]any {
	keys := fm.Keys()
	out := make(map[string]any, len(keys))
	for _, k := range keys {
		if v, ok := fm.Get(k); ok {
			out[k] = v
		}
	}
	return out
}

// candidateLess ordena por rank (menor é melhor), depois caminho e referência completa.
func candidateLess(a, b candidate) bool {
	if a.rank != b.rank {
		return a.rank < b.rank
	}
	if pa, pb := a.ref.Path.String(), b.ref.Path.String(); pa != pb {
		return pa < pb
	}
	return a.ref.String() < b.ref.String()
}

// decodeArgs decodifica os argumentos rejeitando campos desconhecidos. Argumento vazio vale {}.
func decodeArgs(raw json.RawMessage, dst any) error {
	if len(bytes.TrimSpace(raw)) == 0 {
		raw = json.RawMessage(`{}`)
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	return dec.Decode(dst)
}

// clampLimit aplica o limite: ausente vale def; fora de 1..100 é limitado (sem erro).
func clampLimit(v *int, def int) int {
	n := def
	if v != nil {
		n = *v
	}
	return min(max(n, 1), maxReadLimit)
}

// firstInt devolve o primeiro valor presente. A ordem é a de precedência: canônico, depois aliases.
func firstInt(vals ...*int) *int {
	for _, v := range vals {
		if v != nil {
			return v
		}
	}
	return nil
}

// firstNonEmpty devolve o primeiro valor não vazio, na ordem dada.
func firstNonEmpty(vals ...*string) string {
	for _, v := range vals {
		if v != nil && *v != "" {
			return *v
		}
	}
	return ""
}

// jsonResult serializa a resposta de sucesso.
func jsonResult(v any) (ToolResult, error) {
	b, err := json.Marshal(v)
	if err != nil {
		slog.Error("serializar resposta de ferramenta de leitura", "err", err)
		return failure("falha interna ao serializar a resposta")
	}
	return ToolResult{Text: string(b)}, nil
}

// failure devolve erro de ferramenta (isError), sem propagar erro de protocolo.
func failure(msg string) (ToolResult, error) {
	return ToolResult{Text: msg, IsError: true}, nil
}

// invalidArg é entrada inválida (equivale a 400). A mensagem não repete o valor informado.
func invalidArg(detail string) (ToolResult, error) {
	return failure("entrada inválida: " + detail)
}

// toolFailure traduz erros da 001 para mensagens fixas. Não repete nomes, caminhos nem SQL: a causa
// detalhada só vai para o log do servidor.
func toolFailure(tool string, err error) (ToolResult, error) {
	switch {
	case errors.Is(err, storage.ErrScopeInvalid):
		return invalidArg("escopo inválido: workspace e project vêm juntos, e scopes aceita até 25 pares")
	case errors.Is(err, storage.ErrScopeNotFound):
		return failure("não encontrado: escopo inexistente")
	case errors.Is(err, storage.ErrScopeAmbiguous):
		return failure("escopo ambíguo: informe o nome exato")
	case errors.Is(err, storage.ErrPageNotFound):
		return failure("não encontrado: página inexistente no escopo resolvido")
	}
	slog.Error("falha em ferramenta de leitura MCP", "tool", tool, "err", err)
	return failure("falha interna ao consultar a memória")
}
