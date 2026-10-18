package ingest

import (
	"container/list"
	"errors"
	"fmt"
	"strings"
	"sync"

	"github.com/phm-aguiar/ai-memory-go/internal/domain"
	"github.com/phm-aguiar/ai-memory-go/internal/storage"
)

// Resolução de projeto do hook e sessão pegajosa.
//
// Fontes: docs/lifecycle-event-ingestion.md (LEI) §6 e §7; docs/marker-file.md (MF) "Scope walk",
// "Keys", "Project name" e "Mid-session navigation"; docs/domain-vocabulary.md §9.2.
//
// Função pura: não lê disco nem variável de ambiente. O conteúdo do marcador, a raiz do repositório
// git, o diretório home e os projetos conhecidos entram por parâmetro. A única memória com estado é o
// cache da sessão pegajosa (StickyCache), só em memória. Quem cria escopo é EnsureScope de quem chama:
// Resolution.ScopeRequest devolve o pedido para storage.ResolveWriteScope com Create.

// Valores de [routing] mid_session (config.Config.MidSession).
const (
	MidSessionFollowCwd = "follow-cwd"
	MidSessionSticky    = "sticky"
)

// Estratégias de nome de projeto (MF "Keys", project_strategy).
const (
	StrategyBasename = "basename"
	StrategyRepoRoot = "repo-root"
)

// Nomes padrão do servidor, usados quando o chamador não informa o padrão (LEI §7.1 passo 1).
const (
	defaultWorkspaceName = "default"
	defaultProjectName   = "scratch"
	// reservedGlobalProject é o nome reservado de busca global (LEI §7.1 passo 5). Nunca é projeto do hook.
	reservedGlobalProject = "_global"
)

// StickyCacheLimit é o teto de entradas do cache da sessão pegajosa (LEI §6: 4096).
const StickyCacheLimit = 4096

// ErrUnknownMidSession é um valor de mid_session fora de follow-cwd e sticky.
var ErrUnknownMidSession = errors.New("mid_session desconhecido: use follow-cwd ou sticky")

// Source diz por qual regra a resolução chegou ao projeto.
type Source string

const (
	// SourceMarker: o marcador declara `project` (sobreposição explícita, vence tudo).
	SourceMarker Source = "marker"
	// SourcePrefix: projeto conhecido cujo caminho registrado é o prefixo mais longo do cwd.
	SourcePrefix Source = "prefix"
	// SourceDerived: nome derivado do diretório pela estratégia.
	SourceDerived Source = "derived"
	// SourceDefault: projeto padrão do servidor (sem cwd, ou nome recusado).
	SourceDefault Source = "default"
	// SourceSticky: sessão pegajosa, mantém o projeto da sessão (LEI §7.2).
	SourceSticky Source = "sticky"
)

// MarkerScope são os campos de escopo lidos de um marcador, pela regra de linha (MF "File format and
// reading"). Campo vazio significa ausente.
type MarkerScope struct {
	Workspace string
	Project   string
	// Strategy é normalizada: "repo-root", "basename" ou vazio. Valor inválido vira vazio (MF "Keys").
	Strategy string
}

// KnownProject é um projeto já existente com o caminho do repositório registrado.
type KnownProject struct {
	Workspace string
	Project   string
	// Path é a raiz do repositório git gravada para o projeto. Vazio quando o projeto não tem caminho.
	Path string
}

// Anchor é a âncora de uma sessão: o diretório em que ela abriu e o escopo que recebeu.
type Anchor struct {
	Dir       string
	Workspace string
	Project   string
}

// ResolveInput são as entradas da resolução de um evento.
type ResolveInput struct {
	// Cwd é o diretório de trabalho do evento (normalize.go, Event.Cwd). Vazio quando ausente.
	Cwd string
	// RepoRoot é a raiz do repositório git principal, achada pelo chamador. Vazio quando o cwd não está
	// em um repositório git real (LEI §7.1 passo 4).
	RepoRoot string
	// MarkerContent é o conteúdo já lido do marcador de escopo mais próximo. Nil quando não há marcador.
	// Conteúdo que não declara nenhuma chave de escopo é ignorado (MF "Scope walk").
	MarkerContent []byte
	// Known são os projetos já existentes, para a escolha do prefixo mais longo (LEI §7.1 passo 6).
	Known []KnownProject
	// Session e HasSession identificam a sessão do evento.
	Session    domain.SessionID
	HasSession bool
	// Opening é verdadeiro para o evento que abre a sessão. Esse evento sempre resolve pelo próprio
	// diretório (MF "Mid-session": a abertura nunca é afetada pelo modo).
	Opening bool
	// MidSession é o valor de [routing] mid_session. Vazio equivale a follow-cwd.
	MidSession string
	// DefaultStrategy é a estratégia instalada para o ambiente, usada sem marcador. Vazio = basename.
	DefaultStrategy string
	// HomeDir é o diretório home. Âncora em home nunca gruda (LEI §7.2). Vazio desliga a regra.
	HomeDir string
	// DefaultWorkspace e DefaultProject são o escopo padrão do servidor. Vazios: default e scratch.
	DefaultWorkspace string
	DefaultProject   string
}

// Resolution é o resultado da resolução.
type Resolution struct {
	Workspace string
	Project   string
	Source    Source
	// RepoPath é a raiz git a gravar com o projeto. Vazio quando não há raiz git real (LEI §7.1 passo 4).
	RepoPath string
}

// ScopeRequest monta o pedido de escrita com criação ligada. O resolvedor não grava: quem chama passa
// o pedido a storage.ResolveWriteScope e, se o alvo não existir, cria por EnsureScope.
func (r Resolution) ScopeRequest() storage.ScopeRequest {
	return storage.ScopeRequest{WorkspaceName: r.Workspace, ProjectName: r.Project, Create: true}
}

// ReadMarkerScope lê os campos de escopo de um marcador pela regra de linha (MF "File format and
// reading"): a chave vale quando a linha começa com o nome, seguido de `=`; string exige aspas duplas;
// a primeira linha que casa decide. Não exige TOML válido.
func ReadMarkerScope(content []byte) MarkerScope {
	var ms MarkerScope
	ms.Workspace = lineString(content, "workspace")
	ms.Project = lineString(content, "project")
	ms.Strategy = normalizeStrategy(lineString(content, "project_strategy"))
	return ms
}

// MarkerDeclaresScope diz se o marcador conta para a caminhada de escopo (MF "Scope walk"): contém,
// em qualquer lugar, uma das chaves que declaram algo. Marcador só de [capture] não conta.
func MarkerDeclaresScope(content []byte) bool {
	declaring := map[string]bool{
		"workspace": true, "project": true, "project_strategy": true, "drop_subagent_captures": true,
		"identity": true, "identity_style": true, "aliases": true, "server": true, "default_global": true,
		"inject_on_session_start": true, "max_chars": true, "contribute": true, "consume": true,
	}
	for _, line := range splitLines(content) {
		if key, _, ok := splitKeyLine(line); ok && declaring[key] {
			return true
		}
	}
	return false
}

// lineString devolve o valor string da primeira linha da chave. Vazio quando ausente, sem aspas ou vazio.
func lineString(content []byte, key string) string {
	for _, line := range splitLines(content) {
		k, rest, ok := splitKeyLine(line)
		if !ok || k != key {
			continue
		}
		rest = strings.TrimLeft(rest, " \t")
		if !strings.HasPrefix(rest, `"`) {
			return ""
		}
		end := strings.IndexByte(rest[1:], '"')
		if end < 0 {
			return ""
		}
		return rest[1 : 1+end]
	}
	return ""
}

// splitKeyLine separa `chave = resto` de uma linha. A chave começa na coluna 0 (regra de linha de MF).
func splitKeyLine(line string) (key, rest string, ok bool) {
	eq := strings.IndexByte(line, '=')
	if eq <= 0 {
		return "", "", false
	}
	key = strings.TrimRight(line[:eq], " \t")
	if key == "" || strings.ContainsAny(key, " \t#[\"") {
		return "", "", false
	}
	return key, line[eq+1:], true
}

func splitLines(content []byte) []string {
	return strings.Split(strings.ReplaceAll(string(content), "\r\n", "\n"), "\n")
}

// normalizeStrategy aceita repo-root, repo_root e basename. Qualquer outro valor vira vazio (MF "Keys").
func normalizeStrategy(s string) string {
	switch s {
	case StrategyRepoRoot, "repo_root":
		return StrategyRepoRoot
	case StrategyBasename:
		return StrategyBasename
	}
	return ""
}

// Resolve aplica a ordem de resolução do hook (LEI §7.1) e a sessão pegajosa (LEI §7.2) a um evento.
// anchor é a âncora da sessão, quando existe. Função pura.
func Resolve(in ResolveInput, anchor *Anchor) (Resolution, error) {
	mode, err := midSessionMode(in.MidSession)
	if err != nil {
		return Resolution{}, err
	}
	ms := MarkerScope{}
	hasMarker := MarkerDeclaresScope(in.MarkerContent)
	if hasMarker {
		ms = ReadMarkerScope(in.MarkerContent)
	}

	if !in.Opening && anchor != nil && stickyApplies(in, ms, hasMarker, mode, *anchor) {
		return Resolution{Workspace: anchor.Workspace, Project: anchor.Project, Source: SourceSticky}, nil
	}
	return resolveFull(in, ms)
}

// resolveFull é a ordem de §7.1 sem sessão pegajosa.
func resolveFull(in ResolveInput, ms MarkerScope) (Resolution, error) {
	// Passo 3, primeira parte: sobreposição do marcador vence qualquer outra regra.
	if ms.Project != "" {
		ws := orDefault(ms.Workspace, defaultWorkspace(in))
		return finishProject(in, Resolution{Workspace: ws, Project: ms.Project, Source: SourceMarker, RepoPath: in.RepoRoot}), nil
	}
	// Passo 1: sem diretório e sem sobreposição, projeto padrão.
	if in.Cwd == "" {
		return defaultResolution(in), nil
	}
	// Passo 6: prefixo mais longo entre projetos com caminho registrado. Subdiretório cai no pai.
	if k, ok := longestPrefix(in.Known, in.Cwd, ms.Workspace); ok {
		return finishProject(in, Resolution{Workspace: k.Workspace, Project: k.Project, Source: SourcePrefix, RepoPath: in.RepoRoot}), nil
	}
	// Passo 3, segunda parte: nome derivado pela estratégia.
	strategy := ms.Strategy
	if strategy == "" {
		strategy = normalizeStrategy(in.DefaultStrategy)
	}
	if strategy == "" {
		strategy = StrategyBasename
	}
	name := baseName(in.Cwd)
	if strategy == StrategyRepoRoot && in.RepoRoot != "" {
		name = baseName(in.RepoRoot)
	}
	ws := orDefault(ms.Workspace, defaultWorkspace(in))
	return finishProject(in, Resolution{Workspace: ws, Project: name, Source: SourceDerived, RepoPath: in.RepoRoot}), nil
}

// finishProject aplica os passos 2 a 5 ao candidato: nome vazio ou `_global` recusa e usa o padrão;
// caminho do repositório só fica se for raiz git real (já vem vazio quando não é).
func finishProject(in ResolveInput, r Resolution) Resolution {
	if r.Project == "" || r.Project == reservedGlobalProject {
		return defaultResolution(in)
	}
	return r
}

func defaultResolution(in ResolveInput) Resolution {
	return Resolution{Workspace: defaultWorkspace(in), Project: defaultProject(in), Source: SourceDefault}
}

func defaultWorkspace(in ResolveInput) string {
	return orDefault(in.DefaultWorkspace, defaultWorkspaceName)
}

func defaultProject(in ResolveInput) string {
	return orDefault(in.DefaultProject, defaultProjectName)
}

func orDefault(v, def string) string {
	if v == "" {
		return def
	}
	return v
}

// stickyApplies diz se a sessão mantém a âncora para este evento (LEI §7.2 e MF "Mid-session").
func stickyApplies(in ResolveInput, ms MarkerScope, hasMarker bool, mode string, a Anchor) bool {
	if !in.HasSession || a.Dir == "" || isBroadAnchor(a.Dir, in.HomeDir) {
		return false
	}
	// Marcador explícito sempre vence: sobreposição de projeto ou reescopo de workspace.
	if ms.Project != "" {
		return false
	}
	if ms.Workspace != "" && ms.Workspace != a.Workspace {
		return false
	}
	if mode == MidSessionSticky {
		return true
	}
	// follow-cwd: gruda dentro da subárvore da sessão.
	if in.Cwd != "" && isWithin(in.Cwd, a.Dir) {
		return true
	}
	// follow-cwd fora da subárvore: só com repo-root, cwd fora de qualquer repositório git e sem marcador.
	strategy := ms.Strategy
	if strategy == "" {
		strategy = normalizeStrategy(in.DefaultStrategy)
	}
	return strategy == StrategyRepoRoot && in.RepoRoot == "" && !hasMarker && in.Cwd != ""
}

func midSessionMode(v string) (string, error) {
	switch v {
	case "", MidSessionFollowCwd:
		return MidSessionFollowCwd, nil
	case MidSessionSticky:
		return MidSessionSticky, nil
	}
	return "", fmt.Errorf("%w: recebeu %q", ErrUnknownMidSession, v)
}

// longestPrefix devolve o projeto cujo caminho registrado é o prefixo mais longo do dir (por componentes).
// Com workspace do marcador, só considera projetos desse workspace.
func longestPrefix(known []KnownProject, dir, workspace string) (KnownProject, bool) {
	var best KnownProject
	bestLen := -1
	for _, k := range known {
		if k.Path == "" || k.Project == "" {
			continue
		}
		if workspace != "" && k.Workspace != workspace {
			continue
		}
		if !isWithin(dir, k.Path) {
			continue
		}
		if l := len(normPath(k.Path)); l > bestLen {
			best, bestLen = k, l
		}
	}
	return best, bestLen >= 0
}

// normPath troca separadores por `/` e tira a barra final, sem mexer em caixa (LEI §7.1 passo 3).
// A raiz `/` vira vazio; a raiz de unidade `C:/` vira `C:`.
func normPath(p string) string {
	p = strings.ReplaceAll(p, `\`, "/")
	return strings.TrimRight(p, "/")
}

// isWithin diz se dir é root ou está dentro de root, por componentes completos.
func isWithin(dir, root string) bool {
	if root == "" || dir == "" {
		return false
	}
	if normPath(root) == "" {
		// Raiz do sistema de arquivos: tudo que é absoluto está dentro.
		return strings.HasPrefix(strings.ReplaceAll(dir, `\`, "/"), "/")
	}
	d := normPath(dir)
	r := normPath(root)
	return d == r || strings.HasPrefix(d, r+"/")
}

// isBroadAnchor diz se a âncora é uma raiz ampla que nunca gruda: raiz do sistema de arquivos,
// raiz de unidade ou o home (LEI §7.2; MF "Broad anchors never stick").
func isBroadAnchor(dir, home string) bool {
	d := normPath(dir)
	if d == "" || isDriveRoot(d) {
		return true
	}
	return home != "" && d == normPath(home)
}

// isDriveRoot diz se o caminho normalizado é só uma letra de unidade, como `C:`.
func isDriveRoot(d string) bool {
	return len(d) == 2 && d[1] == ':' && ((d[0] >= 'a' && d[0] <= 'z') || (d[0] >= 'A' && d[0] <= 'Z'))
}

// baseName devolve o último componente do caminho cru, sem mexer em caixa. Vazio para raízes.
func baseName(p string) string {
	c := strings.TrimRight(p, `/\`)
	if c == "" || strings.HasSuffix(c, ":") {
		return ""
	}
	if i := strings.LastIndexAny(c, `/\`); i >= 0 {
		return c[i+1:]
	}
	return c
}

// StickyCache guarda a âncora de cada sessão, em memória, com teto de StickyCacheLimit entradas.
// Ao estourar, sai a entrada usada há mais tempo (LRU). Nunca grava fora de memória.
type StickyCache struct {
	mu    sync.Mutex
	max   int
	order *list.List
	items map[domain.SessionID]*list.Element
}

type stickyEntry struct {
	id     domain.SessionID
	anchor Anchor
}

// NewStickyCache cria o cache com o teto padrão de StickyCacheLimit.
func NewStickyCache() *StickyCache {
	return &StickyCache{max: StickyCacheLimit, order: list.New(), items: make(map[domain.SessionID]*list.Element)}
}

// Get devolve a âncora da sessão e a marca como usada.
func (c *StickyCache) Get(id domain.SessionID) (Anchor, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	el, ok := c.items[id]
	if !ok {
		return Anchor{}, false
	}
	c.order.MoveToBack(el)
	return el.Value.(*stickyEntry).anchor, true
}

// Put grava ou troca a âncora da sessão e a marca como usada. Passa do teto, sai a mais antiga.
func (c *StickyCache) Put(id domain.SessionID, a Anchor) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if el, ok := c.items[id]; ok {
		el.Value.(*stickyEntry).anchor = a
		c.order.MoveToBack(el)
		return
	}
	c.items[id] = c.order.PushBack(&stickyEntry{id: id, anchor: a})
	for c.order.Len() > c.max {
		front := c.order.Front()
		c.order.Remove(front)
		delete(c.items, front.Value.(*stickyEntry).id)
	}
}

// Len devolve o número de entradas.
func (c *StickyCache) Len() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.order.Len()
}

// Resolver liga a resolução pura ao cache da sessão pegajosa.
type Resolver struct {
	cache *StickyCache
}

// NewResolver cria um resolvedor com cache próprio de StickyCacheLimit entradas.
func NewResolver() *Resolver {
	return &Resolver{cache: NewStickyCache()}
}

// Resolve resolve o evento, consulta a âncora da sessão e atualiza a âncora. A âncora é gravada no
// evento que abre a sessão e, quando a sessão ainda não tem âncora, no primeiro evento visto.
func (r *Resolver) Resolve(in ResolveInput) (Resolution, error) {
	var anchor *Anchor
	if in.HasSession {
		if a, ok := r.cache.Get(in.Session); ok {
			anchor = &a
		}
	}
	res, err := Resolve(in, anchor)
	if err != nil {
		return Resolution{}, err
	}
	if in.HasSession && res.Source != SourceSticky && (in.Opening || anchor == nil) {
		r.cache.Put(in.Session, Anchor{Dir: in.Cwd, Workspace: res.Workspace, Project: res.Project})
	}
	return res, nil
}
