package wiki

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/phm-aguiar/ai-memory-go/internal/domain"
)

// Nomes dos arquivos de controle do escopo na árvore da wiki (wiki-layout-contract (a) e (c)).
const (
	metaFileName  = "_meta.md"
	indexFileName = "index.md"
	scopeMetaType = "Scope Manifest"
	okfVersion    = "0.2"
	indexTitle    = "# Índice\n"
)

// ScopeNames são os nomes humanos gravados no manifesto do escopo (ADR-23). RepoPath entra só no
// manifesto do projeto.
type ScopeNames struct {
	WorkspaceName string
	ProjectName   string
	RepoPath      string
}

// EnsureScopeManifest garante os manifestos `_meta.md` do workspace e do projeto (ADR-23).
//
// Chaves do projeto, nesta ordem: type, name, workspace_id, project_id, repo_path. Chaves do
// workspace: type, name, workspace_id. Sem corpo. Os caminhos saem de pathFor (confinamento,
// WFL §12). Se o arquivo já existe com o mesmo conteúdo, não é regravado. Se o conteúdo é outro
// (por exemplo, rename de projeto), é regravado com os nomes recebidos.
//
// Não faz commit. Devolve os caminhos relativos à raiz, com barra normal, que foram gravados nesta
// chamada, para o chamador passar a Repo.Commit. Lista vazia significa nada alterado.
func EnsureScopeManifest(root string, scope domain.Scope, names ScopeNames) ([]string, error) {
	if names.WorkspaceName == "" || names.ProjectName == "" {
		return nil, errors.New("wiki: manifesto do escopo exige nome de workspace e de projeto")
	}
	projPath, err := pathFor(root, scope, metaPage())
	if err != nil {
		return nil, err
	}
	// O caminho do workspace sai do caminho já validado: <root>/<ws>/<proj>/_meta.md. Sobe dois
	// níveis, e <root>/<ws>/_meta.md fica dentro do que pathFor confinou.
	wsPath := filepath.Join(filepath.Dir(filepath.Dir(projPath)), metaFileName)

	wsID := scope.WorkspaceID.String()
	projID := scope.ProjectID.String()

	wsFM, err := manifestFrontmatter(
		kv{"type", scopeMetaType},
		kv{"name", names.WorkspaceName},
		kv{"workspace_id", wsID},
	)
	if err != nil {
		return nil, err
	}
	projFM, err := manifestFrontmatter(
		kv{"type", scopeMetaType},
		kv{"name", names.ProjectName},
		kv{"workspace_id", wsID},
		kv{"project_id", projID},
		kv{"repo_path", names.RepoPath},
	)
	if err != nil {
		return nil, err
	}

	var changed []string
	for _, m := range []struct {
		path string
		fm   domain.Frontmatter
		rel  string
	}{
		{wsPath, wsFM, wsID + "/" + metaFileName},
		{projPath, projFM, wsID + "/" + projID + "/" + metaFileName},
	} {
		data, err := EmitMarkdown(m.fm, "")
		if err != nil {
			return nil, fmt.Errorf("wiki: emitir %s: %w", m.rel, err)
		}
		wrote, err := writeIfChanged(m.path, data)
		if err != nil {
			return nil, err
		}
		if wrote {
			changed = append(changed, m.rel)
		}
	}
	return changed, nil
}

// EnsureProjectIndex cria `index.md` na raiz do projeto se ele não existe (ADR-24). Conteúdo:
// frontmatter `okf_version: "0.2"` e corpo `# Índice`, sem lista. Se o arquivo existe, não é tocado.
// A operação normal não o regrava.
//
// Devolve o caminho relativo à raiz, com barra normal, se criou o arquivo; lista vazia se já existia.
func EnsureProjectIndex(root string, scope domain.Scope) ([]string, error) {
	path, err := pathFor(root, scope, indexPage())
	if err != nil {
		return nil, err
	}
	if _, err := os.Lstat(path); err == nil {
		return nil, nil
	} else if !errors.Is(err, fs.ErrNotExist) {
		return nil, fmt.Errorf("wiki: verificar %s: %w", path, err)
	}
	fm, err := manifestFrontmatter(kv{"okf_version", okfVersion})
	if err != nil {
		return nil, err
	}
	data, err := EmitMarkdown(fm, indexTitle)
	if err != nil {
		return nil, fmt.Errorf("wiki: emitir index.md: %w", err)
	}
	if err := ensureDir(filepath.Dir(path)); err != nil {
		return nil, err
	}
	if _, err := Write(path, data); err != nil {
		return nil, err
	}
	return []string{scope.WorkspaceID.String() + "/" + scope.ProjectID.String() + "/" + indexFileName}, nil
}

// kv é um par chave/valor do frontmatter, para montar o bloco com a ordem da chamada.
type kv struct {
	key, value string
}

// manifestFrontmatter monta o frontmatter na ordem dos pares recebidos.
func manifestFrontmatter(pairs ...kv) (domain.Frontmatter, error) {
	var fm domain.Frontmatter
	for _, p := range pairs {
		if err := fm.Set(p.key, p.value); err != nil {
			return domain.Frontmatter{}, fmt.Errorf("wiki: chave de frontmatter %q: %w", p.key, err)
		}
	}
	return fm, nil
}

// writeIfChanged grava data em path, a menos que o arquivo já tenha exatamente esse conteúdo.
// Devolve true quando gravou. Cria o diretório pai se faltar.
func writeIfChanged(path string, data []byte) (bool, error) {
	cur, err := os.ReadFile(path)
	switch {
	case err == nil:
		if bytes.Equal(cur, data) {
			return false, nil
		}
	case !errors.Is(err, fs.ErrNotExist):
		return false, fmt.Errorf("wiki: ler %s: %w", path, err)
	}
	if err := ensureDir(filepath.Dir(path)); err != nil {
		return false, err
	}
	if _, err := Write(path, data); err != nil {
		return false, err
	}
	return true, nil
}

// ensureDir cria o diretório com permissão 0755 (Unix; no Windows não se aplica). Os links já foram
// recusados por pathFor antes desta chamada.
func ensureDir(dir string) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("wiki: criar diretório %s: %w", dir, err)
	}
	return nil
}

// metaPage e indexPage são os caminhos de sistema do escopo. Usam ParseStoredPagePath porque o
// construtor estrito recusa os nomes base reservados (`_meta.md`, `index.md`, `log.md`), que são
// justamente os que o próprio sistema grava. Os nomes são constantes válidas, então o erro não
// ocorre; se ocorresse, pathFor recusaria o PagePath zero com ErrInvalidPagePath.
func metaPage() domain.PagePath {
	p, _ := domain.ParseStoredPagePath(metaFileName)
	return p
}

func indexPage() domain.PagePath {
	p, _ := domain.ParseStoredPagePath(indexFileName)
	return p
}

func logPage() domain.PagePath {
	p, _ := domain.ParseStoredPagePath(logFileName)
	return p
}
