package wiki

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/phm-aguiar/ai-memory-go/internal/domain"
	"github.com/phm-aguiar/ai-memory-go/internal/domain/sanitize"
	"github.com/phm-aguiar/ai-memory-go/internal/storage"
)

// Chaves do manifesto `_meta.md` lidas pelo reindex (ADR-23).
const (
	nameKey   = "name"
	wsIDKey   = "workspace_id"
	projIDKey = "project_id"
)

// Nomes de entradas ignoradas pelo reindex (ADR-27, WFL §8.1 e §8.2).
const (
	gitDirName       = ".git"
	pendingDirName   = "_pending"
	graveyardDirName = "_graveyard"
	bootstrapName    = "bootstrap.md"
)

// reindexEvidence é a fonte de evidência das páginas gravadas pelo reindex. O markdown não guarda a
// fonte original, e o conjunto de fontes do banco é fechado (storage.SourceKind). Usa-se a passagem
// de reconsolidação, que reconfirma a página a partir do arquivo. Como o vínculo é único por fonte, a
// repetição do reindex não cria linhas novas.
var reindexEvidence = storage.EvidenceSource{Kind: storage.SourceReconsolidation, ID: "reindex"}

// ReindexReport resume uma execução do reindex. Scopes conta os projetos que tiveram o escopo garantido.
// Pages conta os arquivos indexados sem erro, e Unchanged a parte deles que não gerou versão nova.
// Errors traz as falhas por escopo ou por arquivo, que não interrompem a execução. Avisos traz os
// avisos não fatais: manifesto ausente (nome reconstruído pelo id) e avisos de link ou de índice.
type ReindexReport struct {
	Scopes    int
	Pages     int
	Unchanged int
	Errors    []string
	Avisos    []string
}

// IndexFile lê o markdown de uma página e o grava no índice pelo mesmo UpsertPage que o WritePage usa.
// ref traz escopo e caminho relativo; absPath é o arquivo, que deve ser o caminho de pathFor. Não
// escreve no arquivo, não faz commit e não cria escopo: o escopo precisa existir (EnsureScope).
// A trava por página é a mesma de WritePage, quando s.Locks existe. Conteúdo igual à versão corrente
// não cria versão (ADR-18). Usado pelo watcher e pelo reindex.
func IndexFile(ctx context.Context, s *Store, ref domain.PageRef, absPath string) error {
	_, _, err := indexFile(ctx, s, ref, absPath)
	return err
}

// indexFile é IndexFile com o resultado do UpsertPage e os avisos, para o reindex contar.
func indexFile(ctx context.Context, s *Store, ref domain.PageRef, absPath string) (storage.UpsertOutcome, []string, error) {
	if s == nil || s.Writer == nil {
		return "", nil, errors.New("wiki: indexar página exige Writer")
	}
	expected, err := pathFor(s.Root, ref.Scope, ref.Path)
	if err != nil {
		return "", nil, err
	}
	if !samePath(filepath.Clean(absPath), expected) {
		return "", nil, fmt.Errorf("wiki: indexar %s: caminho %q não é o da página", ref.String(), absPath)
	}
	if s.Locks != nil {
		unlock, err := s.Locks.Lock(ctx, ref)
		if err != nil {
			return "", nil, fmt.Errorf("wiki: indexar %s: %w", ref.String(), err)
		}
		defer unlock()
	}

	data, err := os.ReadFile(expected)
	if err != nil {
		return "", nil, fmt.Errorf("wiki: ler %s: %w", expected, err)
	}
	fm, rawBody, err := ReadMarkdown(data)
	if err != nil {
		return "", nil, fmt.Errorf("wiki: indexar %s: %w", ref.String(), err)
	}
	// A mesma sanitização de WritePage: o arquivo pode ter sido editado à mão.
	body := sanitize.Body(rawBody, sanitize.Limits{}).Text()
	fm, err = sanitizeKeepAll(fm)
	if err != nil {
		return "", nil, fmt.Errorf("wiki: indexar %s: %w", ref.String(), err)
	}

	tier := domain.TierSemantic
	if raw := textOf(fm, tierKey); raw != "" {
		tier, err = domain.ParseTier(raw)
		if err != nil {
			return "", nil, fmt.Errorf("wiki: indexar %s: %w", ref.String(), err)
		}
	}
	// kind só entra quando válido; inválido vira ausente e não barra a indexação.
	kind, err := domain.ParsePageKind(textOf(fm, kindKey))
	if err != nil {
		kind = ""
	}

	// Mesma derivação de WritePage: expires_at validado, stale_after acrescentado se faltar.
	expires := textOf(fm, expiresKey)
	if _, err := ParseExpiresAt(expires); err != nil {
		return "", nil, fmt.Errorf("wiki: indexar %s: %w", ref.String(), err)
	}
	stale, hasStale, err := DeriveStaleAfter(expires)
	if err != nil {
		return "", nil, fmt.Errorf("wiki: indexar %s: %w", ref.String(), err)
	}
	if _, ok := fm.Get(staleKey); hasStale && !ok {
		if err := fm.Set(staleKey, stale); err != nil {
			return "", nil, fmt.Errorf("wiki: indexar %s: frontmatter: %w", ref.String(), err)
		}
	}

	title := sanitize.Title(DeriveTitle(fm, body, ref.Path.String()),
		sanitize.Limits{TitleMaxRunes: pageTitleMaxRunes}).Text()
	links, warns, err := ExtractLinks(body, fm, ref.Path)
	if err != nil {
		return "", nil, fmt.Errorf("wiki: indexar %s: %w", ref.String(), err)
	}
	avisos := append([]string(nil), warns...)

	res, err := storage.UpsertPage(ctx, s.Writer, storage.UpsertInput{
		Ref:         ref,
		Tier:        tier,
		Kind:        kind,
		Title:       title,
		Body:        body,
		Frontmatter: fm,
		Pinned:      textOf(fm, pinnedKey) == "true",
		Links:       links,
		Evidence:    reindexEvidence,
		At:          s.now(),
	})
	if err != nil {
		return "", nil, fmt.Errorf("wiki: indexar %s: %w", ref.String(), err)
	}
	avisos = append(avisos, res.Avisos...)
	return res.Outcome, avisos, nil
}

// sanitizeKeepAll copia o frontmatter sanitizando os valores de texto, sem remover chave alguma (ao
// contrário de cleanCopy, que tira generated.*: o índice guarda o frontmatter com essas chaves).
func sanitizeKeepAll(src domain.Frontmatter) (domain.Frontmatter, error) {
	var out domain.Frontmatter
	for _, k := range src.Keys() {
		v, _ := src.Get(k)
		var clean any
		switch t := v.(type) {
		case string:
			clean = sanitize.Body(t, sanitize.Limits{}).Text()
		case []string:
			items := make([]string, len(t))
			for i, it := range t {
				items[i] = sanitize.Body(it, sanitize.Limits{}).Text()
			}
			clean = items
		default:
			return domain.Frontmatter{}, fmt.Errorf("valor de frontmatter %q: tipo %T", k, v)
		}
		if err := out.Set(k, clean); err != nil {
			return domain.Frontmatter{}, fmt.Errorf("frontmatter: %w", err)
		}
	}
	return out, nil
}

// Reindex reconstrói o índice a partir da árvore markdown da wiki em root, que deve ser a raiz do
// Store. Percorre só <root>/<ws>/<proj>/ com dois níveis de UUID (diretórios que não são UUID são
// ignorados), garante cada escopo (EnsureScope, com os nomes de _meta.md) e indexa cada arquivo
// `.md` que não é ignorado (ADR-27) pelo IndexFile. Não escreve no disco e não faz commit.
//
// Sem _meta.md, ou com _meta.md inválido, o escopo é reconstruído pelos UUIDs: o id vira o nome, e
// o aviso vai para o report. Erro de um escopo ou de um arquivo vai para Errors e não para a
// execução. Só erro de contexto, de root ou de Store interrompe o reindex.
func Reindex(ctx context.Context, s *Store, root string) (ReindexReport, error) {
	var rep ReindexReport
	if s == nil || s.Writer == nil {
		return rep, errors.New("wiki: reindex exige Writer")
	}
	if !samePath(filepath.Clean(root), filepath.Clean(s.Root)) {
		return rep, fmt.Errorf("wiki: reindex: raiz %q difere da raiz do Store %q", root, s.Root)
	}
	if err := checkRoot(root); err != nil {
		return rep, err
	}
	wsEntries, err := os.ReadDir(root)
	if err != nil {
		return rep, fmt.Errorf("wiki: reindex: listar %s: %w", root, err)
	}
	for _, we := range wsEntries {
		if err := ctx.Err(); err != nil {
			return rep, fmt.Errorf("wiki: reindex: %w", err)
		}
		wsID, ok := uuidDir(filepath.Join(root, we.Name()))
		if !ok {
			continue
		}
		wsDir := filepath.Join(root, we.Name())
		projEntries, err := os.ReadDir(wsDir)
		if err != nil {
			rep.Errors = append(rep.Errors, fmt.Sprintf("wiki: reindex: listar %s: %v", wsDir, err))
			continue
		}
		for _, pe := range projEntries {
			projID, ok := uuidDir(filepath.Join(wsDir, pe.Name()))
			if !ok {
				continue
			}
			scope := domain.NewScope(domain.WorkspaceID(wsID), domain.ProjectID(projID))
			if err := reindexScope(ctx, s, root, scope, &rep); err != nil {
				return rep, err
			}
		}
	}
	return rep, nil
}

// reindexScope garante o escopo e indexa os arquivos do projeto. Só devolve erro de contexto.
func reindexScope(ctx context.Context, s *Store, root string, scope domain.Scope, rep *ReindexReport) error {
	wsID := scope.WorkspaceID.String()
	projID := scope.ProjectID.String()
	wsDir := filepath.Join(root, wsID)
	projDir := filepath.Join(wsDir, projID)

	wsName := scopeName(filepath.Join(wsDir, metaFileName), map[string]string{wsIDKey: wsID}, wsID, "workspace", rep)
	projName := scopeName(filepath.Join(projDir, metaFileName),
		map[string]string{wsIDKey: wsID, projIDKey: projID}, projID, "projeto "+projID, rep)

	if err := storage.EnsureScope(ctx, s.Writer, scope, wsName, projName, s.now()); err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return fmt.Errorf("wiki: reindex: %w", ctxErr)
		}
		rep.Errors = append(rep.Errors, fmt.Sprintf("wiki: reindex: escopo %s/%s: %v", wsID, projID, err))
		return nil
	}
	rep.Scopes++

	walkErr := filepath.WalkDir(projDir, func(p string, d fs.DirEntry, err error) error {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return ctxErr
		}
		if err != nil {
			rep.Errors = append(rep.Errors, fmt.Sprintf("wiki: reindex: caminhar %s: %v", p, err))
			return nil
		}
		if p == projDir {
			return nil
		}
		return indexEntry(ctx, s, scope, projDir, p, d, rep)
	})
	if walkErr != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return fmt.Errorf("wiki: reindex: %w", ctxErr)
		}
		rep.Errors = append(rep.Errors, fmt.Sprintf("wiki: reindex: caminhar %s: %v", projDir, walkErr))
	}
	return nil
}

// indexEntry aplica as regras de ignorados (ADR-27) a uma entrada da caminhada e indexa o `.md` que
// sobra. Devolve fs.SkipDir para não descer em diretório ignorado. Só devolve erro de contexto.
func indexEntry(ctx context.Context, s *Store, scope domain.Scope, projDir, p string, d fs.DirEntry, rep *ReindexReport) error {
	name := d.Name()
	rel, err := filepath.Rel(projDir, p)
	if err != nil {
		rep.Errors = append(rep.Errors, fmt.Sprintf("wiki: reindex: caminho %s: %v", p, err))
		return nil
	}
	slashRel := filepath.ToSlash(rel)
	topLevel := !strings.Contains(slashRel, "/")

	fi, err := os.Lstat(p)
	if err != nil {
		rep.Errors = append(rep.Errors, fmt.Sprintf("wiki: reindex: verificar %s: %v", p, err))
		return nil
	}
	// Link simbólico e ponto de reparse são ignorados: nunca se segue link na indexação.
	if fi.Mode()&os.ModeSymlink != 0 || hasReparseAttr(fi) {
		if fi.IsDir() {
			return fs.SkipDir
		}
		return nil
	}
	if strings.HasPrefix(name, tmpPrefix) {
		if fi.IsDir() {
			return fs.SkipDir
		}
		return nil
	}
	if fi.IsDir() {
		if name == gitDirName || (topLevel && (name == pendingDirName || name == graveyardDirName)) {
			return fs.SkipDir
		}
		return nil
	}
	if !fi.Mode().IsRegular() || filepath.Ext(name) != ".md" || ignoredFileName(name) {
		return nil
	}

	pagePath, err := domain.ParsePagePath(slashRel)
	if err != nil {
		rep.Errors = append(rep.Errors, fmt.Sprintf("wiki: reindex: caminho %s: %v", slashRel, err))
		return nil
	}
	outcome, avisos, err := indexFile(ctx, s, domain.NewPageRef(scope, pagePath), p)
	if err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return fmt.Errorf("wiki: reindex: %w", ctxErr)
		}
		rep.Errors = append(rep.Errors, fmt.Sprintf("wiki: reindex: %v", err))
		return nil
	}
	rep.Pages++
	if outcome == storage.UpsertUnchanged {
		rep.Unchanged++
	}
	rep.Avisos = append(rep.Avisos, avisos...)
	return nil
}

// ignoredFileName diz se o nome de arquivo é um controle da wiki que o reindex não indexa: manifesto,
// índice do bundle, bootstrap e ledgers `log*.md` (ADR-27). A comparação não distingue caixa.
func ignoredFileName(name string) bool {
	lower := strings.ToLower(name)
	switch lower {
	case metaFileName, indexFileName, bootstrapName:
		return true
	}
	ok, _ := filepath.Match("log*.md", lower)
	return ok
}

// uuidDir devolve o identificador do diretório se ele é um diretório real (sem link nem ponto de
// reparse) cujo nome é um UUID na forma canônica. Caso contrário, ok é false.
func uuidDir(p string) (domain.ID, bool) {
	fi, err := os.Lstat(p)
	if err != nil || !fi.IsDir() || fi.Mode()&os.ModeSymlink != 0 || hasReparseAttr(fi) {
		return domain.ID{}, false
	}
	id, err := domain.ParseID(filepath.Base(p))
	if err != nil || id.String() != filepath.Base(p) {
		return domain.ID{}, false
	}
	return id, true
}

// scopeName devolve o nome humano do escopo lido do _meta.md. Sem o arquivo, ou com ele inválido, usa o
// id como nome e registra o aviso no report. Arquivo inválido também vai para Errors. Não grava nada.
func scopeName(path string, identity map[string]string, id, what string, rep *ReindexReport) string {
	name, err := manifestName(path, identity)
	if err != nil {
		rep.Errors = append(rep.Errors, fmt.Sprintf("wiki: reindex: %v", err))
	}
	if name == "" {
		rep.Avisos = append(rep.Avisos, fmt.Sprintf("%s sem _meta.md válido: nome reconstruído pelo id %s", what, id))
		return id
	}
	return name
}

// manifestName lê o nome de um _meta.md. Devolve "" e nil se o arquivo não existe. Devolve erro se o
// arquivo é link, se o YAML é inválido, se falta name, ou se um identificador do manifesto não bate com
// o do escopo. Não segue links.
func manifestName(path string, identity map[string]string) (string, error) {
	fi, err := os.Lstat(path)
	if errors.Is(err, fs.ErrNotExist) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("verificar %s: %w", path, err)
	}
	if fi.Mode()&os.ModeSymlink != 0 || hasReparseAttr(fi) {
		return "", fmt.Errorf("%w: %s é link simbólico ou ponto de reparse", ErrConfinement, path)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("ler %s: %w", path, err)
	}
	fm, _, err := ReadMarkdown(data)
	if err != nil {
		return "", fmt.Errorf("ler %s: %w", path, err)
	}
	for key, want := range identity {
		if got := textOf(fm, key); got != want {
			return "", fmt.Errorf("%s: %s = %q, esperado %q", path, key, got, want)
		}
	}
	name := textOf(fm, nameKey)
	if name == "" {
		return "", fmt.Errorf("%s: name ausente", path)
	}
	return name, nil
}
