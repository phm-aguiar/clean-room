package wiki

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/phm-aguiar/ai-memory-go/internal/domain"
)

// ScopeNamesFunc devolve os nomes humanos de um escopo (workspace e projeto, e o caminho do repositório
// do projeto). O diretório da wiki só tem identificadores, então os nomes vêm de fora (o boot do serve,
// que consulta o banco).
type ScopeNamesFunc func(scope domain.Scope) (ScopeNames, error)

// BackfillReport resume uma execução do backfill de manifestos. Written traz os caminhos relativos à
// raiz, com barra normal, dos `_meta.md` criados nesta chamada, para o chamador passar a Repo.Commit.
// Invalid traz os manifestos que já existem mas não são válidos: ficam como estão, e o reindex os
// recusa com o caminho exato. Errors traz as falhas por escopo, que não interrompem a execução.
type BackfillReport struct {
	Written []string
	Invalid []string
	Errors  []string
}

// BackfillScopeManifests cria o `_meta.md` do workspace e do projeto de cada escopo que não tem o seu
// (docs/lifecycle-ops.md, reindex; ADR-23). Percorre <root>/<ws>/<proj>/ com dois níveis de UUID, como o
// reindex, e só grava arquivos que faltam: manifesto válido não é tocado (nem regravado, nem tem o mtime
// alterado), e manifesto existente e inválido é reportado em Invalid e não é sobrescrito. Chamada
// repetida sem mudança de disco não grava nada.
//
// Os nomes saem de names, chamado só quando há algum arquivo a criar no escopo. Erro de names, de
// confinamento ou de gravação vai para Errors. Não faz commit. Só erro de contexto interrompe.
func BackfillScopeManifests(ctx context.Context, root string, names ScopeNamesFunc) (BackfillReport, error) {
	var rep BackfillReport
	if names == nil {
		return rep, errors.New("wiki: backfill de manifestos exige função de nomes")
	}
	if err := checkRoot(root); err != nil {
		return rep, err
	}
	wsEntries, err := os.ReadDir(root)
	if err != nil {
		return rep, fmt.Errorf("wiki: backfill de manifestos: listar %s: %w", root, err)
	}
	for _, we := range wsEntries {
		if err := ctx.Err(); err != nil {
			return rep, fmt.Errorf("wiki: backfill de manifestos: %w", err)
		}
		wsID, ok := uuidDir(filepath.Join(root, we.Name()))
		if !ok {
			continue
		}
		projEntries, err := os.ReadDir(filepath.Join(root, wsID.String()))
		if err != nil {
			rep.Errors = append(rep.Errors, fmt.Sprintf("wiki: backfill de manifestos: listar %s: %v",
				filepath.Join(root, wsID.String()), err))
			continue
		}
		for _, pe := range projEntries {
			projID, ok := uuidDir(filepath.Join(root, wsID.String(), pe.Name()))
			if !ok {
				continue
			}
			scope := domain.NewScope(domain.WorkspaceID(wsID), domain.ProjectID(projID))
			if err := ctx.Err(); err != nil {
				return rep, fmt.Errorf("wiki: backfill de manifestos: %w", err)
			}
			backfillScope(root, scope, names, &rep)
		}
	}
	return rep, nil
}

// manifestState diz o estado de um _meta.md: ausente, válido ou inválido (com o motivo).
type manifestState struct {
	missing bool
	err     error
}

// checkManifest olha um _meta.md sem alterá-lo. Ausente é missing. Existente e que não passa em
// manifestName (link, YAML inválido, identificador diferente, sem name) é inválido.
func checkManifest(path string, identity map[string]string) manifestState {
	if _, err := os.Lstat(path); errors.Is(err, fs.ErrNotExist) {
		return manifestState{missing: true}
	}
	if _, err := manifestName(path, identity); err != nil {
		return manifestState{err: err}
	}
	return manifestState{}
}

// backfillScope cria os manifestos que faltam de um escopo. Não devolve erro: registra em rep.
func backfillScope(root string, scope domain.Scope, names ScopeNamesFunc, rep *BackfillReport) {
	wsID := scope.WorkspaceID.String()
	projID := scope.ProjectID.String()

	projPath, err := pathFor(root, scope, metaPage())
	if err != nil {
		rep.Errors = append(rep.Errors, fmt.Sprintf("wiki: backfill de manifestos: escopo %s/%s: %v", wsID, projID, err))
		return
	}
	// Mesmo cálculo de EnsureScopeManifest: <root>/<ws>/_meta.md, dois níveis acima do caminho validado.
	wsPath := filepath.Join(filepath.Dir(filepath.Dir(projPath)), metaFileName)

	wsState := checkManifest(wsPath, map[string]string{wsIDKey: wsID})
	projState := checkManifest(projPath, map[string]string{wsIDKey: wsID, projIDKey: projID})

	type target struct {
		path string
		rel  string
		fm   func(ScopeNames) (domain.Frontmatter, error)
		st   manifestState
	}
	targets := []target{
		{wsPath, wsID + "/" + metaFileName, wsManifestFM(wsID), wsState},
		{projPath, wsID + "/" + projID + "/" + metaFileName, projManifestFM(wsID, projID), projState},
	}

	var need []target
	for _, t := range targets {
		switch {
		case t.st.missing:
			need = append(need, t)
		case t.st.err != nil:
			rep.Invalid = append(rep.Invalid, fmt.Sprintf("%s: %v", t.rel, t.st.err))
		}
	}
	if len(need) == 0 {
		return
	}

	sn, err := names(scope)
	if err == nil && (sn.WorkspaceName == "" || sn.ProjectName == "") {
		err = errors.New("nome de workspace ou de projeto vazio")
	}
	if err != nil {
		rep.Errors = append(rep.Errors, fmt.Sprintf("wiki: backfill de manifestos: escopo %s/%s: nomes: %v", wsID, projID, err))
		return
	}

	for _, t := range need {
		fm, err := t.fm(sn)
		if err != nil {
			rep.Errors = append(rep.Errors, fmt.Sprintf("wiki: backfill de manifestos: %s: %v", t.rel, err))
			continue
		}
		data, err := EmitMarkdown(fm, "")
		if err != nil {
			rep.Errors = append(rep.Errors, fmt.Sprintf("wiki: backfill de manifestos: emitir %s: %v", t.rel, err))
			continue
		}
		wrote, err := writeIfChanged(t.path, data)
		if err != nil {
			rep.Errors = append(rep.Errors, fmt.Sprintf("wiki: backfill de manifestos: %v", err))
			continue
		}
		if wrote {
			rep.Written = append(rep.Written, t.rel)
		}
	}
}

// wsManifestFM monta o frontmatter do manifesto do workspace. As chaves e a ordem são as de
// EnsureScopeManifest (meta.go): type, name, workspace_id.
func wsManifestFM(wsID string) func(ScopeNames) (domain.Frontmatter, error) {
	return func(sn ScopeNames) (domain.Frontmatter, error) {
		return manifestFrontmatter(
			kv{"type", scopeMetaType},
			kv{"name", sn.WorkspaceName},
			kv{"workspace_id", wsID},
		)
	}
}

// projManifestFM monta o frontmatter do manifesto do projeto. Chaves e ordem como em EnsureScopeManifest:
// type, name, workspace_id, project_id, repo_path.
func projManifestFM(wsID, projID string) func(ScopeNames) (domain.Frontmatter, error) {
	return func(sn ScopeNames) (domain.Frontmatter, error) {
		return manifestFrontmatter(
			kv{"type", scopeMetaType},
			kv{"name", sn.ProjectName},
			kv{"workspace_id", wsID},
			kv{"project_id", projID},
			kv{"repo_path", sn.RepoPath},
		)
	}
}
