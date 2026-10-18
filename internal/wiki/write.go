package wiki

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"runtime/debug"
	"strings"
	"time"

	"github.com/phm-aguiar/ai-memory-go/internal/domain"
	"github.com/phm-aguiar/ai-memory-go/internal/domain/sanitize"
	"github.com/phm-aguiar/ai-memory-go/internal/storage"
)

// ErrInconsistentState é a causa quando o arquivo não pode ser revertido depois que o índice
// falhou (P-03). É o mesmo sentinela de storage, para que errors.Is funcione nos dois pacotes.
var ErrInconsistentState = storage.ErrInconsistentState

const (
	// pageTitleMaxRunes é o teto do título de página no índice (ADR-10). O arquivo não é cortado.
	pageTitleMaxRunes = 80

	// generatedKey é o mapeamento de proveniência do OKF 0.2: generated: {by, at} (docs/okf.md).
	generatedKey      = "generated"
	generatedByField  = "by"
	generatedAtField  = "at"
	generatedAtLayout = "2006-01-02T15:04:05Z"
	// generatedByModel é a chave que, se presente, vira generated.by (docs/okf.md, Proveniência).
	generatedByModel = "generated_by_model"
	// generatedByPrefix seguido da versão do build é o generated.by padrão.
	generatedByPrefix = "process:ai-memory/"
	// sourcesKey, sessionKey e agentKey alimentam a derivação de sources (docs/okf.md, Proveniência).
	sourcesKey = "sources"
	sessionKey = "session_id"
	agentKey   = "agent"
	// sessionResourcePrefix é o recurso de sources de uma sessão do ai-memory.
	sessionResourcePrefix = "ai-memory://session/"
	// agentActorPrefix é o prefixo de ator de agente em sources.author.
	agentActorPrefix = "process:"

	typeKey    = "type"
	kindKey    = "kind"
	slotKey    = "slot_kind"
	tierKey    = "tier"
	pinnedKey  = "pinned"
	summaryKey = "summary"
	descKey    = "description"
	expiresKey = "expires_at"
	staleKey   = "stale_after"

	// slotsFamily é o diretório de slots: páginas sob ele são fixadas (docs/wiki-files-layer.md §5.2).
	slotsFamily = "_slots"
)

// Tabelas de derivação do tipo OKF (markdown-contract (d)). A primeira regra que se aplica vence.
var (
	typeByKind = map[string]string{
		"fact": "Fact", "note": "Note", "procedure": "Procedure", "decision": "Decision",
	}
	typeBySlot = map[string]string{
		"invariant": "Invariant", "state": "State",
	}
	typeByFamily = map[string]string{
		"sessions": "Session Summary", "_rules": "Rule", "gotchas": "Gotcha",
		"decisions": "Decision", "procedures": "Procedure", "concepts": "Concept",
		"notes": "Note", "runbooks": "Runbook", slotsFamily: "State", "_lint": "Lint Report",
		"_pending": "Pending Note",
	}
)

// Store é o fluxo de escrita de página (P-03). Root é a raiz absoluta da wiki, a mesma de Repo.
// Writer é o ator de escrita do storage. Locks serializa as escritas por página. Now é o relógio,
// injetável para teste; se nil, usa time.Now.
type Store struct {
	Root   string
	Writer storage.Writer
	Repo   *Repo
	Locks  *PageLocks
	Now    func() time.Time
}

// PageInput é o pedido de escrita de uma página. Tier é obrigatório (ADR-28). Kind é opcional.
// Pinned não é campo: a fixação vem da chave `pinned` do frontmatter (markdown-contract (a)). Sem a
// chave, a escrita herda a fixação da versão anterior; só `pinned: false` explícito desafixa.
// WorkspaceName e ProjectName são os nomes humanos gravados na criação do escopo e no manifesto.
type PageInput struct {
	Scope         domain.Scope
	WorkspaceName string
	ProjectName   string
	RepoPath      string
	Path          domain.PagePath
	Tier          domain.Tier
	Kind          domain.PageKind
	Body          string
	Frontmatter   domain.Frontmatter
	Evidence      storage.EvidenceSource
}

// WriteResult é o resultado de WritePage. Path é relativo à raiz da wiki (`<ws>/<proj>/<path>`).
// Checkpoint é sempre vazio: WritePage não faz commit (T080). Os caminhos afetados ficam
// registrados no Repo para o commit feito por quem chama (CommitTracked). Avisos traz as falhas
// tratadas como aviso (manifesto, registro para commit) e os avisos de links.
type WriteResult struct {
	Version    domain.PageID
	Outcome    storage.UpsertOutcome
	Path       string
	Checkpoint string
	Avisos     []string
}

// generated é a proveniência de uma escrita com conteúdo novo, ou herdada quando o conteúdo não mudou.
type generated struct {
	at, by string
}

// WritePage grava uma página na ordem do P-03: confinamento, sanitização, derivação, trava,
// criação do escopo, instalação do arquivo com rollback, UpsertPage, manifesto, e registro dos
// caminhos para commit (sem commit). Não escreve log.md nem index.md: o ledger é só de eventos de
// hook e o index.md da raiz é da migração OKF (docs/wiki-files-layer.md §5.2, §9). Erros de entrada
// e de índice voltam com %w. Falhas de manifesto e de registro viram aviso.
func (s *Store) WritePage(ctx context.Context, in PageInput) (WriteResult, error) {
	if s == nil || s.Writer == nil || s.Repo == nil || s.Locks == nil {
		return WriteResult{}, errors.New("wiki: escrita de página exige Writer, Repo e Locks")
	}
	now := s.now()

	// 1. Confinamento e validação de entrada. Nada é tocado antes destas checagens.
	abs, err := pathFor(s.Root, in.Scope, in.Path)
	if err != nil {
		return WriteResult{}, err
	}
	if in.WorkspaceName == "" || in.ProjectName == "" {
		return WriteResult{}, errors.New("wiki: escrita de página exige nome de workspace e de projeto")
	}
	if _, err := domain.ParseTier(string(in.Tier)); err != nil {
		return WriteResult{}, fmt.Errorf("wiki: escrita de página: %w", err)
	}
	if in.Evidence.Kind == "" || in.Evidence.ID == "" {
		return WriteResult{}, errors.New("wiki: escrita de página exige fonte de evidência")
	}

	// 2. Sanitização. O corpo e os valores de texto do frontmatter passam pelo sanitizador sem
	// teto (ADR-10): a redação roda antes de qualquer derivação.
	body := sanitize.Body(in.Body, sanitize.Limits{}).Text()
	fm, err := cleanCopy(in.Frontmatter)
	if err != nil {
		return WriteResult{}, err
	}

	// 3. Derivação. Erro de expires_at sai antes de tocar em disco.
	head, err := buildHead(in, fm)
	if err != nil {
		return WriteResult{}, err
	}
	expires := textOf(head, expiresKey)
	if _, err := ParseExpiresAt(expires); err != nil {
		return WriteResult{}, fmt.Errorf("wiki: escrita de página: %w", err)
	}
	stale, hasStale, err := DeriveStaleAfter(expires)
	if err != nil {
		return WriteResult{}, fmt.Errorf("wiki: escrita de página: %w", err)
	}
	pageTitle := sanitize.Title(DeriveTitle(head, body, in.Path.String()),
		sanitize.Limits{TitleMaxRunes: pageTitleMaxRunes}).Text()
	links, warns, err := ExtractLinks(body, head, in.Path)
	if err != nil {
		return WriteResult{}, fmt.Errorf("wiki: escrita de página: %w", err)
	}
	avisos := append([]string(nil), warns...)

	// 4. Trava por página. Todas as escritas de arquivo, manifesto e log do passo seguinte ficam
	// sob ela (T045 não tem trava própria).
	unlock, err := s.Locks.Lock(ctx, domain.NewPageRef(in.Scope, in.Path))
	if err != nil {
		return WriteResult{}, fmt.Errorf("wiki: escrita de página: %w", err)
	}
	defer unlock()

	// 5. Criação explícita do escopo, o único caminho que cria workspace ou projeto.
	if err := storage.EnsureScope(ctx, s.Writer, in.Scope, in.WorkspaceName, in.ProjectName, now); err != nil {
		return WriteResult{}, fmt.Errorf("wiki: escrita de página: %w", err)
	}

	// 6. Conteúdo novo. Se o arquivo atual tem o mesmo conteúdo (ADR-18), não é reescrito.
	prev, existed, err := readIfExists(abs)
	if err != nil {
		return WriteResult{}, err
	}
	head = inheritPinned(head, prev, existed)
	same, err := sameFileContent(prev, existed, body, head, stale, hasStale)
	if err != nil {
		return WriteResult{}, err
	}
	gen, err := generationFor(prev, same, now, resolveGeneratedBy(in.Frontmatter, head))
	if err != nil {
		return WriteResult{}, err
	}
	final, err := assemble(head, gen, stale, hasStale)
	if err != nil {
		return WriteResult{}, err
	}

	// 7. Instalação com rollback. A restauração que falha vira ErrInconsistentState.
	if !same {
		emitted, err := EmitMarkdown(final, body)
		if err != nil {
			return WriteResult{}, fmt.Errorf("wiki: emitir página: %w", err)
		}
		if err := ensureDir(filepath.Dir(abs)); err != nil {
			return WriteResult{}, err
		}
		if _, err := Write(abs, emitted); err != nil {
			if rbErr := restoreFile(abs, prev, existed); rbErr != nil {
				return WriteResult{}, fmt.Errorf("%w: restaurar %s: %w; erro original: %w",
					ErrInconsistentState, abs, rbErr, err)
			}
			return WriteResult{}, fmt.Errorf("wiki: instalar página: %w", err)
		}
	}
	res, err := storage.UpsertPage(ctx, s.Writer, storage.UpsertInput{
		Ref:         domain.NewPageRef(in.Scope, in.Path),
		Tier:        in.Tier,
		Kind:        in.Kind,
		Title:       pageTitle,
		Body:        body,
		Frontmatter: final,
		Pinned:      textOf(final, pinnedKey) == "true",
		Links:       links,
		Evidence:    in.Evidence,
		At:          now,
	})
	if err != nil {
		if !same {
			if rbErr := restoreFile(abs, prev, existed); rbErr != nil {
				return WriteResult{}, fmt.Errorf("%w: restaurar %s: %w; erro original: %w",
					ErrInconsistentState, abs, rbErr, err)
			}
		}
		return WriteResult{}, fmt.Errorf("wiki: gravar página no índice: %w", err)
	}
	avisos = append(avisos, res.Avisos...)
	changed := res.Outcome != storage.UpsertUnchanged

	rel := scopeRelPath(in.Scope, in.Path.String())
	paths := []string{rel}

	// 8. Manifesto do escopo (ADR-23). Falha vira aviso: o índice já é a verdade.
	manifest, err := EnsureScopeManifest(s.Root, in.Scope, ScopeNames{
		WorkspaceName: in.WorkspaceName,
		ProjectName:   in.ProjectName,
		RepoPath:      in.RepoPath,
	})
	if err != nil {
		avisos = append(avisos, fmt.Sprintf("manifesto do escopo não atualizado: %v", err))
		manifest = nil
	}
	paths = append(paths, manifest...)

	// 9. Registro para commit. WritePage não faz commit: quem chama commita os caminhos
	// registrados com Repo.CommitTracked, no ponto definido pelo doc (§7). Sem mudança (Unchanged)
	// e sem manifesto novo, nada é registrado. Falha de registro vira aviso.
	if changed || len(manifest) > 0 {
		if err := s.Repo.Track(paths...); err != nil {
			avisos = append(avisos, fmt.Sprintf("registro para commit não feito: %v", err))
		}
	}

	// 10. Resultado. Checkpoint fica vazio: a escrita não cria commit.
	return WriteResult{
		Version:    res.Version,
		Outcome:    res.Outcome,
		Path:       rel,
		Checkpoint: "",
		Avisos:     avisos,
	}, nil
}

// now devolve o instante da escrita: o relógio injetado ou time.Now.
func (s *Store) now() time.Time {
	if s.Now != nil {
		return s.Now()
	}
	return time.Now()
}

// scopeRelPath monta `<workspace_id>/<project_id>/<rel>`, com barra normal, relativo à raiz da wiki.
func scopeRelPath(scope domain.Scope, rel string) string {
	return scope.WorkspaceID.String() + "/" + scope.ProjectID.String() + "/" + rel
}

// cleanCopy copia o frontmatter de entrada na ordem recebida, sem a chave generated (que é da
// escrita) e com os valores de texto sanitizados sem teto, em todos os níveis.
func cleanCopy(src domain.Frontmatter) (domain.Frontmatter, error) {
	return cleanMap(src, generatedKey)
}

// cleanMap copia um mapeamento na ordem recebida, sanitizando textos sem teto, em profundidade.
// Pula a chave omit (vazia não pula nada). Aceita texto, lista de texto, mapeamento e lista de
// mapeamentos; outro tipo é erro.
func cleanMap(src domain.Frontmatter, omit string) (domain.Frontmatter, error) {
	var out domain.Frontmatter
	for _, k := range src.Keys() {
		if k == omit {
			continue
		}
		v, _ := src.Get(k)
		clean, err := cleanValue(k, v)
		if err != nil {
			return domain.Frontmatter{}, err
		}
		if err := out.Set(k, clean); err != nil {
			return domain.Frontmatter{}, fmt.Errorf("wiki: frontmatter: %w", err)
		}
	}
	return out, nil
}

// cleanValue sanitiza um valor do frontmatter, recursivamente nos mapeamentos e listas de mapeamentos.
func cleanValue(key string, v any) (any, error) {
	switch t := v.(type) {
	case string:
		return sanitize.Body(t, sanitize.Limits{}).Text(), nil
	case []string:
		items := make([]string, len(t))
		for i, it := range t {
			items[i] = sanitize.Body(it, sanitize.Limits{}).Text()
		}
		return items, nil
	case domain.Frontmatter:
		return cleanMap(t, "")
	case []domain.Frontmatter:
		items := make([]domain.Frontmatter, 0, len(t))
		for _, it := range t {
			c, err := cleanMap(it, "")
			if err != nil {
				return nil, err
			}
			items = append(items, c)
		}
		return items, nil
	}
	return nil, fmt.Errorf("wiki: valor de frontmatter %q: tipo %T", key, v)
}

// buildHead acrescenta tier, kind, type e description ao frontmatter de entrada (ADR-17). Chaves
// existentes com valor não vazio não são alteradas. type segue markdown-contract (d).
func buildHead(in PageInput, fm domain.Frontmatter) (domain.Frontmatter, error) {
	out := fm
	if err := out.Set(tierKey, string(in.Tier)); err != nil {
		return domain.Frontmatter{}, fmt.Errorf("wiki: frontmatter: %w", err)
	}
	if in.Kind != "" {
		kind, err := domain.ParsePageKind(string(in.Kind))
		if err != nil {
			return domain.Frontmatter{}, fmt.Errorf("wiki: escrita de página: %w", err)
		}
		if err := out.Set(kindKey, string(kind)); err != nil {
			return domain.Frontmatter{}, fmt.Errorf("wiki: frontmatter: %w", err)
		}
	}
	if k := textOf(out, kindKey); k != "" {
		if _, err := domain.ParsePageKind(k); err != nil {
			return domain.Frontmatter{}, fmt.Errorf("wiki: escrita de página: %w", err)
		}
	}
	if textOf(out, typeKey) == "" {
		if err := out.Set(typeKey, deriveType(out, in.Path.String())); err != nil {
			return domain.Frontmatter{}, fmt.Errorf("wiki: frontmatter: %w", err)
		}
	}
	// Caminho de slot é fixado pelo caminho (docs/wiki-files-layer.md §5.2, passo 3).
	if isSlotPath(in.Path.String()) {
		if err := out.Set(pinnedKey, "true"); err != nil {
			return domain.Frontmatter{}, fmt.Errorf("wiki: frontmatter: %w", err)
		}
	}
	if _, ok := out.Get(descKey); !ok {
		if s := textOf(out, summaryKey); s != "" {
			if err := out.Set(descKey, s); err != nil {
				return domain.Frontmatter{}, fmt.Errorf("wiki: frontmatter: %w", err)
			}
		}
	}
	if err := deriveSources(&out); err != nil {
		return domain.Frontmatter{}, fmt.Errorf("wiki: frontmatter: %w", err)
	}
	return out, nil
}

// deriveSources cria sources quando a página não tem sources e tem session_id (docs/okf.md,
// Proveniência). O item traz resource ai-memory://session/<session_id>; com agent, também traz
// author process:<agent>.
func deriveSources(fm *domain.Frontmatter) error {
	if _, ok := fm.Get(sourcesKey); ok {
		return nil
	}
	sid := textOf(*fm, sessionKey)
	if sid == "" {
		return nil
	}
	var item domain.Frontmatter
	if err := item.Set("resource", sessionResourcePrefix+sid); err != nil {
		return err
	}
	if agent := textOf(*fm, agentKey); agent != "" {
		if err := item.Set("author", agentActorPrefix+agent); err != nil {
			return err
		}
	}
	return fm.Set(sourcesKey, []domain.Frontmatter{item})
}

// resolveGeneratedBy escolhe generated.by de uma escrita com conteúdo novo (docs/okf.md): o
// generated.by já presente na entrada não é sobrescrito; senão vale generated_by_model; senão
// process:ai-memory/<versão do build>.
func resolveGeneratedBy(src, head domain.Frontmatter) string {
	if by := generatedOf(src).by; by != "" {
		return sanitize.Body(by, sanitize.Limits{}).Text()
	}
	if model := textOf(head, generatedByModel); model != "" {
		return model
	}
	return generatedByPrefix + buildVersion()
}

// buildVersion é a versão do build que entra em generated.by. Vem da informação de build do Go;
// sem versão de módulo (build de desenvolvimento), vale "dev".
func buildVersion() string {
	if info, ok := debug.ReadBuildInfo(); ok {
		if v := info.Main.Version; v != "" && v != "(devel)" {
			return v
		}
	}
	return "dev"
}

// generatedOf lê o mapeamento generated de um frontmatter. Ausente ou que não é mapeamento dá
// proveniência vazia.
func generatedOf(fm domain.Frontmatter) generated {
	var gen generated
	v, ok := fm.Get(generatedKey)
	if !ok {
		return gen
	}
	g, ok := v.(domain.Frontmatter)
	if !ok {
		return gen
	}
	gen.at = textOf(g, generatedAtField)
	gen.by = textOf(g, generatedByField)
	return gen
}

// deriveType aplica markdown-contract (d): kind, slot_kind, depois a família pelo primeiro
// segmento do caminho. Qualquer outro caso vira Note.
func deriveType(fm domain.Frontmatter, rel string) string {
	if t, ok := typeByKind[textOf(fm, kindKey)]; ok {
		return t
	}
	if t, ok := typeBySlot[textOf(fm, slotKey)]; ok {
		return t
	}
	family, _, _ := strings.Cut(rel, "/")
	if t, ok := typeByFamily[family]; ok {
		return t
	}
	return "Note"
}

// isSlotPath diz se o caminho relativo à raiz do projeto está sob `_slots/`.
func isSlotPath(rel string) bool {
	family, _, _ := strings.Cut(rel, "/")
	return family == slotsFamily
}

// textOf devolve o valor de key aparado, se for texto; senão, vazio.
func textOf(fm domain.Frontmatter, key string) string {
	v, ok := fm.Get(key)
	if !ok {
		return ""
	}
	s, ok := v.(string)
	if !ok {
		return ""
	}
	return strings.TrimSpace(s)
}

// dropGenerated copia fm sem o mapeamento generated, sem sanitizar. Usada na comparação de
// conteúdo (ADR-18).
func dropGenerated(fm domain.Frontmatter) (domain.Frontmatter, error) {
	var out domain.Frontmatter
	for _, k := range fm.Keys() {
		if k == generatedKey {
			continue
		}
		v, _ := fm.Get(k)
		if err := out.Set(k, v); err != nil {
			return domain.Frontmatter{}, fmt.Errorf("wiki: frontmatter: %w", err)
		}
	}
	return out, nil
}

// assemble monta o frontmatter final na ordem do ADR-17: o de head, depois generated (mapeamento
// {by, at}, de gen), depois stale_after se ausente.
func assemble(head domain.Frontmatter, gen generated, stale string, hasStale bool) (domain.Frontmatter, error) {
	out, err := dropGenerated(head)
	if err != nil {
		return domain.Frontmatter{}, err
	}
	if gen.by != "" || gen.at != "" {
		var g domain.Frontmatter
		if gen.by != "" {
			if err := g.Set(generatedByField, gen.by); err != nil {
				return domain.Frontmatter{}, fmt.Errorf("wiki: frontmatter: %w", err)
			}
		}
		if gen.at != "" {
			if err := g.Set(generatedAtField, gen.at); err != nil {
				return domain.Frontmatter{}, fmt.Errorf("wiki: frontmatter: %w", err)
			}
		}
		if err := out.Set(generatedKey, g); err != nil {
			return domain.Frontmatter{}, fmt.Errorf("wiki: frontmatter: %w", err)
		}
	}
	if hasStale {
		if _, ok := out.Get(staleKey); !ok {
			if err := out.Set(staleKey, stale); err != nil {
				return domain.Frontmatter{}, fmt.Errorf("wiki: frontmatter: %w", err)
			}
		}
	}
	return out, nil
}

// sameFileContent diz se o arquivo atual tem o mesmo conteúdo que a escrita produziria: corpo e
// frontmatter sem generated.* (ADR-18). Sem arquivo, ou com YAML inválido, o conteúdo é diferente.
func sameFileContent(prev []byte, existed bool, body string, head domain.Frontmatter, stale string, hasStale bool) (bool, error) {
	if !existed {
		return false, nil
	}
	curFM, curBody, err := ReadMarkdown(prev)
	if err != nil {
		return false, nil
	}
	if curBody != body {
		return false, nil
	}
	// Sem generated.at o arquivo não tem a proveniência completa: é reescrito para receber o instante.
	if generatedOf(curFM).at == "" {
		return false, nil
	}
	candidate, err := assemble(head, generated{}, stale, hasStale)
	if err != nil {
		return false, err
	}
	a, err := dropGenerated(curFM)
	if err != nil {
		return false, err
	}
	return a.Equal(candidate), nil
}

// generationFor escolhe a proveniência: herda a do arquivo quando o conteúdo é o mesmo (okf
// "Determinismo"), senão carimba o instante da escrita com o generated.by resolvido por by.
func generationFor(prev []byte, same bool, now time.Time, by string) (generated, error) {
	if !same {
		return generated{at: now.UTC().Format(generatedAtLayout), by: by}, nil
	}
	curFM, _, err := ReadMarkdown(prev)
	if err != nil {
		return generated{}, fmt.Errorf("wiki: ler proveniência: %w", err)
	}
	gen := generatedOf(curFM)
	if gen.by == "" {
		gen.by = by
	}
	return gen, nil
}

// inheritPinned mantém a fixação da versão anterior quando a escrita não envia a chave `pinned`
// (T085). Um agente que reescreve o corpo sem reenviar o frontmatter não pode perder, sem aviso, a
// proteção contra decaimento. `pinned: false` explícito desafixa. A fonte é o arquivo atual: o
// markdown é a fonte da verdade. Arquivo ausente ou ilegível não herda nada.
func inheritPinned(head domain.Frontmatter, prev []byte, existed bool) domain.Frontmatter {
	if !existed {
		return head
	}
	if _, ok := head.Get(pinnedKey); ok {
		return head
	}
	prevFM, _, err := ReadMarkdown(prev)
	if err != nil || textOf(prevFM, pinnedKey) != "true" {
		return head
	}
	out := head
	if err := out.Set(pinnedKey, "true"); err != nil {
		return head
	}
	return out
}

// readIfExists lê o arquivo se existe. Devolve o conteúdo, se existia, e erro só para falha de leitura.
func readIfExists(path string) ([]byte, bool, error) {
	data, err := os.ReadFile(path)
	switch {
	case err == nil:
		return data, true, nil
	case errors.Is(err, fs.ErrNotExist):
		return nil, false, nil
	default:
		return nil, false, fmt.Errorf("wiki: ler %s: %w", path, err)
	}
}

// restoreFile devolve o arquivo ao conteúdo anterior, ou o apaga se ele não existia antes.
func restoreFile(path string, prev []byte, existed bool) error {
	if existed {
		if _, err := Write(path, prev); err != nil {
			return err
		}
		return nil
	}
	if err := os.Remove(path); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	return nil
}
