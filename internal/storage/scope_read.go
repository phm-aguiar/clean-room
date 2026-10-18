package storage

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/phm-aguiar/ai-memory-go/internal/domain"
)

// Nomes do escopo padrão de leitura (docs/domain-vocabulary.md §2): workspace
// `default` e projeto de fallback `scratch`. Só são procurados, nunca criados.
// `_global` (default/_global) é o escopo reservado de busca global.
const (
	scopeDefaultWorkspaceName = "default"
	scopeDefaultProjectName   = "scratch"
	scopeGlobalProjectName    = "_global"
)

// MaxScopeList é o máximo de pares em uma lista `scopes` (docs/mcp-server.md §2.1; contrato MCP §2.4).
const MaxScopeList = 25

// Erros de escopo além de ErrScopeNotFound (não encontrado, 404) e ErrScopeAmbiguous.
var (
	// ErrScopeInvalid é entrada inválida (equivalente a 400): workspace sem projeto, nome vazio,
	// lista acima do limite, mistura de formas.
	ErrScopeInvalid = errors.New("entrada de escopo inválida")
	// ErrScopeRequired é escrita sem escopo e sem ponteiro resolvido. Nunca grava no padrão em silêncio.
	ErrScopeRequired = errors.New("escrita sem escopo: informe workspace e projeto")
	// ErrScopeMismatch é escrita recusada porque o ponteiro de projeto ativo diverge do chamador.
	// É distinto de ErrScopeAmbiguous, para que quem chama saiba que a causa é divergência.
	ErrScopeMismatch = errors.New("escrita recusada: projeto ativo diverge do chamador, informe workspace e projeto")
)

// ScopeSource diz por qual caminho a resolução encontrou o escopo (docs/storage-and-retrieval.md §14.1).
type ScopeSource string

const (
	// ScopeSourceExplicit: workspace e projeto informados por nome.
	ScopeSourceExplicit ScopeSource = "explicit"
	// ScopeSourceSession: entrada própria do chamador no ponteiro.
	ScopeSourceSession ScopeSource = "session"
	// ScopeSourceSharedSlot: slot compartilhado do ponteiro (último projeto publicado).
	ScopeSourceSharedSlot ScopeSource = "shared_slot"
	// ScopeSourceStartupSeed: semente de startup. Só para leitura; é fallback.
	ScopeSourceStartupSeed ScopeSource = "startup_seed"
	// ScopeSourceDefault: escopo padrão do servidor (workspace default, projeto scratch).
	ScopeSourceDefault ScopeSource = "default"
	// ScopeSourceDefaultAfterMismatch: divergência do ponteiro; leitura cai no padrão. É fallback.
	ScopeSourceDefaultAfterMismatch ScopeSource = "default_after_mismatch"
)

// IsFallback diz se a origem é fallback (semente de startup ou padrão após divergência).
func (s ScopeSource) IsFallback() bool {
	return s == ScopeSourceStartupSeed || s == ScopeSourceDefaultAfterMismatch
}

// IsInferred diz se a origem foi inferida: tudo, exceto explícito e sessão (docs/storage-and-retrieval.md §14.1).
func (s ScopeSource) IsInferred() bool {
	return s != ScopeSourceExplicit && s != ScopeSourceSession
}

// ScopeRequest pede um escopo. Caminhos (docs/auto-scope.md, "Explicit scope arguments"):
//   - WorkspaceName e ProjectName juntos: nome explícito; sem o outro é entrada inválida.
//   - Só ProjectName: workspace ativo do ator, depois padrão. `workspace/project` vira par só se o nome exato não existir.
//   - Tudo vazio: o ponteiro do ator decide (Pointer, Actor, Now); sem ponteiro, usa o padrão.
//   - RepoPath: não é resolvido na leitura nem na escrita. A resolução por caminho é do hook (T027).
//
// Pointer é opcional: nil equivale a ponteiro Unset. Now zero usa o relógio atual.
// Create liga a criação de workspace e projeto nomeados, só para caminhos de escrita explícita
// (docs/storage-and-retrieval.md §14.1). Com Create falso, escopo nomeado inexistente falha fechado.
type ScopeRequest struct {
	WorkspaceName string
	ProjectName   string
	RepoPath      string

	Pointer *ActiveProjectPointer
	Actor   ActorKey
	Now     time.Time
	Create  bool
}

// ScopeResolution é o resultado da leitura: escopo, origem e a preferência default_global do ponteiro.
// DefaultGlobal só vale quando a origem vem do ponteiro (session, shared_slot ou startup_seed).
type ScopeResolution struct {
	Scope         domain.Scope
	Source        ScopeSource
	DefaultGlobal bool
}

// WriteScopeTarget é o alvo de uma escrita. Quando Exists é verdadeiro, Scope é o escopo existente.
// Quando é falso, WorkspaceName e ProjectName dizem o que criar (a criação é de quem chama, via EnsureScope).
type WriteScopeTarget struct {
	Scope         domain.Scope
	Exists        bool
	WorkspaceName string
	ProjectName   string
	Source        ScopeSource
}

// ScopeRef é um par explícito de uma lista `scopes`.
type ScopeRef struct {
	WorkspaceName string
	ProjectName   string
}

const (
	scopeByPairSQL        = `SELECT p.id, p.workspace_id FROM projects p JOIN workspaces w ON w.id = p.workspace_id WHERE w.name = ? AND p.name = ?`
	scopeByWorkspaceIDSQL = `SELECT p.id, p.workspace_id FROM projects p WHERE p.workspace_id = ? AND p.name = ?`
)

// ResolveScope é a leitura sem ponteiro de retorno completo: devolve escopo e origem. Usa a mesma
// regra de ResolveReadScope; é mantida com esta assinatura para quem não precisa de DefaultGlobal.
func ResolveScope(ctx context.Context, r Reader, req ScopeRequest) (domain.Scope, ScopeSource, error) {
	res, err := ResolveReadScope(ctx, r, req)
	if err != nil {
		return domain.Scope{}, "", err
	}
	return res.Scope, res.Source, nil
}

// ResolveReadScope resolve o escopo de leitura. Somente leitura: roda por r (transação read-only) e
// nunca cria workspace nem projeto.
//
// Leitura sem escopo nunca falha por ponteiro sem resposta: degrada (ponteiro, semente, padrão) e
// informa a origem. Nomes explícitos falham fechado: não encontrado é erro, nunca cai em outro escopo.
// Precedência com `project` sozinho: (1) workspace ativo do ator, (2) workspace padrão. Sem nome
// exato em nenhum dos dois, o par `workspace/project` é tentado. Só depois disso é não encontrado.
func ResolveReadScope(ctx context.Context, r Reader, req ScopeRequest) (ScopeResolution, error) {
	if err := rejectRepoPath(req); err != nil {
		return ScopeResolution{}, err
	}
	hasWS := req.WorkspaceName != ""
	hasPJ := req.ProjectName != ""
	switch {
	case hasWS && !hasPJ:
		return ScopeResolution{}, partialScopeError()
	case hasWS:
		label := fmt.Sprintf("workspace %q, projeto %q", req.WorkspaceName, req.ProjectName)
		scope, ok, err := findOne(ctx, r, label, scopeByPairSQL, req.WorkspaceName, req.ProjectName)
		if err != nil {
			return ScopeResolution{}, err
		}
		if !ok {
			return ScopeResolution{}, notFoundError(label)
		}
		return ScopeResolution{Scope: scope, Source: ScopeSourceExplicit}, nil
	case hasPJ:
		return readProjectOnly(ctx, r, req)
	default:
		return readWithoutScope(ctx, r, req)
	}
}

// readWithoutScope é a leitura sem nenhum nome: ponteiro, depois semente (dentro do ponteiro) e padrão.
func readWithoutScope(ctx context.Context, r Reader, req ScopeRequest) (ScopeResolution, error) {
	rr := req.pointerRead()
	if src, ok := originSource(rr.Origin); ok {
		return ScopeResolution{Scope: rr.Scope, Source: src, DefaultGlobal: rr.DefaultGlobal}, nil
	}
	src := ScopeSourceDefault
	if rr.Origin == ReadOriginMismatch {
		src = ScopeSourceDefaultAfterMismatch
	}
	return resolveDefaultScope(ctx, r, src)
}

// readProjectOnly resolve `project` sem workspace: workspace ativo, depois padrão, depois o par
// `workspace/project` se o nome exato não existir. Nunca cai no projeto do ponteiro nem no padrão.
func readProjectOnly(ctx context.Context, r Reader, req ScopeRequest) (ScopeResolution, error) {
	name := req.ProjectName
	rr := req.pointerRead()
	if _, ok := originSource(rr.Origin); ok {
		label := fmt.Sprintf("projeto %q no workspace ativo", name)
		scope, found, err := findOne(ctx, r, label, scopeByWorkspaceIDSQL, rr.Scope.WorkspaceID.Bytes(), name)
		if err != nil {
			return ScopeResolution{}, err
		}
		if found {
			return ScopeResolution{Scope: scope, Source: ScopeSourceExplicit}, nil
		}
	}

	label := fmt.Sprintf("projeto %q no workspace padrão", name)
	scope, found, err := findOne(ctx, r, label, scopeByPairSQL, scopeDefaultWorkspaceName, name)
	if err != nil {
		return ScopeResolution{}, err
	}
	if found {
		return ScopeResolution{Scope: scope, Source: ScopeSourceExplicit}, nil
	}

	if ws, pj, ok := splitScopeLabel(name); ok {
		label := fmt.Sprintf("workspace %q, projeto %q", ws, pj)
		scope, found, err := findOne(ctx, r, label, scopeByPairSQL, ws, pj)
		if err != nil {
			return ScopeResolution{}, err
		}
		if found {
			return ScopeResolution{Scope: scope, Source: ScopeSourceExplicit}, nil
		}
		return ScopeResolution{}, notFoundError(label)
	}
	return ScopeResolution{}, notFoundError(fmt.Sprintf("projeto %q", name))
}

// ResolveWriteScope resolve o alvo de uma escrita. Única parte que pode indicar criação, e só com
// req.Create. Nunca grava: quem cria é EnsureScope.
//
// Escrita sem escopo usa o ponteiro resolvido (sessão ou slot compartilhado). Sem ponteiro resolvido,
// recusa com ErrScopeRequired (Unset) ou ErrScopeMismatch (divergência). A semente de startup nunca
// vale para escrita. `project` sozinho herda o workspace do ponteiro; `workspace` sozinho é entrada
// inválida. Um rótulo `workspace/project` nunca cria escopo.
func ResolveWriteScope(ctx context.Context, r Reader, req ScopeRequest) (WriteScopeTarget, error) {
	if err := rejectRepoPath(req); err != nil {
		return WriteScopeTarget{}, err
	}
	hasWS := req.WorkspaceName != ""
	hasPJ := req.ProjectName != ""
	switch {
	case hasWS && !hasPJ:
		return WriteScopeTarget{}, partialScopeError()
	case hasWS:
		return writePair(ctx, r, req)
	case hasPJ:
		return writeProjectOnly(ctx, r, req)
	default:
		rr := req.pointerRead()
		src, err := pointerWriteSource(rr)
		if err != nil {
			return WriteScopeTarget{}, err
		}
		return WriteScopeTarget{Scope: rr.Scope, Exists: true, Source: src}, nil
	}
}

func writePair(ctx context.Context, r Reader, req ScopeRequest) (WriteScopeTarget, error) {
	label := fmt.Sprintf("workspace %q, projeto %q", req.WorkspaceName, req.ProjectName)
	scope, ok, err := findOne(ctx, r, label, scopeByPairSQL, req.WorkspaceName, req.ProjectName)
	if err != nil {
		return WriteScopeTarget{}, err
	}
	if ok {
		return WriteScopeTarget{Scope: scope, Exists: true, Source: ScopeSourceExplicit}, nil
	}
	if !req.Create {
		return WriteScopeTarget{}, notFoundError(label)
	}
	return WriteScopeTarget{WorkspaceName: req.WorkspaceName, ProjectName: req.ProjectName, Source: ScopeSourceExplicit}, nil
}

func writeProjectOnly(ctx context.Context, r Reader, req ScopeRequest) (WriteScopeTarget, error) {
	rr := req.pointerRead()
	if _, err := pointerWriteSource(rr); err != nil {
		return WriteScopeTarget{}, err
	}
	name := req.ProjectName
	wsID := rr.Scope.WorkspaceID

	label := fmt.Sprintf("projeto %q no workspace ativo", name)
	scope, ok, err := findOne(ctx, r, label, scopeByWorkspaceIDSQL, wsID.Bytes(), name)
	if err != nil {
		return WriteScopeTarget{}, err
	}
	if ok {
		return WriteScopeTarget{Scope: scope, Exists: true, Source: ScopeSourceExplicit}, nil
	}
	if ws, pj, isLabel := splitScopeLabel(name); isLabel {
		label := fmt.Sprintf("workspace %q, projeto %q", ws, pj)
		scope, ok, err := findOne(ctx, r, label, scopeByPairSQL, ws, pj)
		if err != nil {
			return WriteScopeTarget{}, err
		}
		if ok {
			return WriteScopeTarget{Scope: scope, Exists: true, Source: ScopeSourceExplicit}, nil
		}
		return WriteScopeTarget{}, notFoundError(label)
	}
	if !req.Create {
		return WriteScopeTarget{}, notFoundError(label)
	}
	wsName, err := workspaceName(ctx, r, wsID)
	if err != nil {
		return WriteScopeTarget{}, err
	}
	return WriteScopeTarget{WorkspaceName: wsName, ProjectName: name, Source: ScopeSourceExplicit}, nil
}

// ResolveScopeList resolve uma lista `scopes` de pares explícitos. Falha fechado: se um par não
// existe, a chamada inteira falha e nenhum escopo é devolvido. A lista é validada antes de qualquer
// consulta: mais de MaxScopeList entradas, ou nome vazio, é entrada inválida. Pares repetidos são
// removidos, mantendo a primeira ocorrência.
func ResolveScopeList(ctx context.Context, r Reader, refs []ScopeRef) ([]domain.Scope, error) {
	if len(refs) > MaxScopeList {
		return nil, fmt.Errorf("%w: a lista aceita no máximo %d escopos, recebeu %d", ErrScopeInvalid, MaxScopeList, len(refs))
	}
	for _, ref := range refs {
		if ref.WorkspaceName == "" || ref.ProjectName == "" {
			return nil, fmt.Errorf("%w: cada item da lista precisa de workspace e projeto não vazios", ErrScopeInvalid)
		}
	}

	seenRef := make(map[ScopeRef]struct{}, len(refs))
	seenScope := make(map[domain.Scope]struct{}, len(refs))
	out := make([]domain.Scope, 0, len(refs))
	for _, ref := range refs {
		if _, dup := seenRef[ref]; dup {
			continue
		}
		seenRef[ref] = struct{}{}

		label := fmt.Sprintf("workspace %q, projeto %q", ref.WorkspaceName, ref.ProjectName)
		scope, ok, err := findOne(ctx, r, label, scopeByPairSQL, ref.WorkspaceName, ref.ProjectName)
		if err != nil {
			return nil, err
		}
		if !ok {
			return nil, notFoundError(label)
		}
		if _, dup := seenScope[scope]; dup {
			continue
		}
		seenScope[scope] = struct{}{}
		out = append(out, scope)
	}
	return out, nil
}

// ResolveGlobalScope devolve o escopo reservado de busca global, default/_global. Só procura, nunca
// cria. Quem une o escopo global aos resultados de um projeto único é o chamador, sem duplicar.
func ResolveGlobalScope(ctx context.Context, r Reader) (domain.Scope, error) {
	label := fmt.Sprintf("global (workspace %q, projeto %q)", scopeDefaultWorkspaceName, scopeGlobalProjectName)
	scope, ok, err := findOne(ctx, r, label, scopeByPairSQL, scopeDefaultWorkspaceName, scopeGlobalProjectName)
	if err != nil {
		return domain.Scope{}, err
	}
	if !ok {
		return domain.Scope{}, notFoundError(label)
	}
	return scope, nil
}

// resolveDefaultScope devolve default/scratch com a origem dada.
func resolveDefaultScope(ctx context.Context, r Reader, src ScopeSource) (ScopeResolution, error) {
	label := fmt.Sprintf("padrão (workspace %q, projeto %q)", scopeDefaultWorkspaceName, scopeDefaultProjectName)
	scope, ok, err := findOne(ctx, r, label, scopeByPairSQL, scopeDefaultWorkspaceName, scopeDefaultProjectName)
	if err != nil {
		return ScopeResolution{}, err
	}
	if !ok {
		return ScopeResolution{}, notFoundError(label)
	}
	return ScopeResolution{Scope: scope, Source: src}, nil
}

// pointerRead consulta o ponteiro para leitura. Sem ponteiro, responde Unset.
func (req ScopeRequest) pointerRead() ReadResult {
	if req.Pointer == nil {
		return ReadResult{Origin: ReadOriginUnset}
	}
	now := req.Now
	if now.IsZero() {
		now = time.Now()
	}
	return req.Pointer.Read(req.Actor, now)
}

// originSource converte a origem do ponteiro em ScopeSource. ok é falso para Mismatch, Unset e semente
// quando não é leitura. A semente é aceita aqui porque só a leitura a usa.
func originSource(o ReadOrigin) (ScopeSource, bool) {
	switch o {
	case ReadOriginSession:
		return ScopeSourceSession, true
	case ReadOriginSharedSlot:
		return ScopeSourceSharedSlot, true
	case ReadOriginStartupSeed:
		return ScopeSourceStartupSeed, true
	}
	return "", false
}

// pointerWriteSource diz se o ponteiro serve à escrita. Só sessão e slot compartilhado servem; a semente
// de startup nunca serve, e divergência é recusada com erro não ambíguo.
func pointerWriteSource(rr ReadResult) (ScopeSource, error) {
	switch rr.Origin {
	case ReadOriginSession:
		return ScopeSourceSession, nil
	case ReadOriginSharedSlot:
		return ScopeSourceSharedSlot, nil
	case ReadOriginMismatch:
		return "", ErrScopeMismatch
	default:
		return "", ErrScopeRequired
	}
}

// rejectRepoPath recusa repo_path na leitura e na escrita. Misturar com nomes é entrada inválida.
func rejectRepoPath(req ScopeRequest) error {
	if req.RepoPath == "" {
		return nil
	}
	if req.WorkspaceName != "" || req.ProjectName != "" {
		return fmt.Errorf("%w: repo_path não combina com workspace/projeto por nome", ErrScopeInvalid)
	}
	return fmt.Errorf("%w: repo_path não é resolvido aqui (caminho de repositório é da resolução do hook, T027)", ErrScopeNotFound)
}

// partialScopeError é workspace sem projeto ou o contrário. O erro encadeia ErrScopeInvalid (entrada
// inválida) e ErrScopeNotFound: o contrato de aceitação atual pede as duas classes para este caso.
func partialScopeError() error {
	return fmt.Errorf("%w: escopo parcial: workspace e projeto devem ser informados juntos (%w)", ErrScopeInvalid, ErrScopeNotFound)
}

func notFoundError(label string) error {
	return fmt.Errorf("resolver escopo %s: %w", label, ErrScopeNotFound)
}

// splitScopeLabel reconhece `workspace/project`: exatamente uma barra e nome não vazio dos dois lados.
func splitScopeLabel(name string) (string, string, bool) {
	if strings.Count(name, "/") != 1 {
		return "", "", false
	}
	ws, pj, _ := strings.Cut(name, "/")
	if ws == "" || pj == "" {
		return "", "", false
	}
	return ws, pj, true
}

// findOne roda uma consulta de escopo e devolve o único casamento. found é falso sem casamento; mais de
// um casamento é ErrScopeAmbiguous.
func findOne(ctx context.Context, r Reader, label, query string, args ...any) (domain.Scope, bool, error) {
	found, err := findScopes(ctx, r, query, args...)
	if err != nil {
		return domain.Scope{}, false, fmt.Errorf("resolver escopo %s: %w", label, err)
	}
	switch len(found) {
	case 0:
		return domain.Scope{}, false, nil
	case 1:
		return found[0], true, nil
	default:
		return domain.Scope{}, false, fmt.Errorf("resolver escopo %s (%d casamentos): %w", label, len(found), ErrScopeAmbiguous)
	}
}

// findScopes roda uma consulta somente leitura que devolve pares (project_id, workspace_id).
func findScopes(ctx context.Context, r Reader, query string, args ...any) ([]domain.Scope, error) {
	var found []domain.Scope
	err := r.Read(ctx, func(tx *sql.Tx) error {
		rows, err := tx.QueryContext(ctx, query, args...)
		if err != nil {
			return fmt.Errorf("consulta de escopo: %w", err)
		}
		defer func() { _ = rows.Close() }()

		for rows.Next() {
			var pjB, wsB []byte
			if err := rows.Scan(&pjB, &wsB); err != nil {
				return fmt.Errorf("ler escopo: %w", err)
			}
			pjID, err := domain.ParseIDBytes(pjB)
			if err != nil {
				return fmt.Errorf("id do projeto: %w", err)
			}
			wsID, err := domain.ParseIDBytes(wsB)
			if err != nil {
				return fmt.Errorf("id do workspace: %w", err)
			}
			found = append(found, domain.NewScope(domain.WorkspaceID(wsID), domain.ProjectID(pjID)))
		}
		if err := rows.Err(); err != nil {
			return fmt.Errorf("iterar escopos: %w", err)
		}
		return nil
	})
	return found, err
}

// workspaceName lê o nome de um workspace pelo id, só para leitura.
func workspaceName(ctx context.Context, r Reader, ws domain.WorkspaceID) (string, error) {
	var name string
	found := false
	err := r.Read(ctx, func(tx *sql.Tx) error {
		err := tx.QueryRowContext(ctx, `SELECT name FROM workspaces WHERE id = ?`, ws.Bytes()).Scan(&name)
		if errors.Is(err, sql.ErrNoRows) {
			return nil
		}
		if err != nil {
			return fmt.Errorf("ler workspace: %w", err)
		}
		found = true
		return nil
	})
	if err != nil {
		return "", fmt.Errorf("resolver workspace do ponteiro: %w", err)
	}
	if !found {
		return "", fmt.Errorf("resolver workspace do ponteiro: %w", ErrScopeNotFound)
	}
	return name, nil
}
