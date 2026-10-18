package wiki

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/phm-aguiar/ai-memory-go/internal/domain"
)

// backfillNames devolve sempre metaNames e conta as chamadas, para provar que o resolver só roda quando
// há manifesto a criar.
func backfillNames(calls *int) ScopeNamesFunc {
	return func(domain.Scope) (ScopeNames, error) {
		*calls++
		return metaNames, nil
	}
}

// backfillStat devolve o conteúdo e o mtime do arquivo, para comparar antes e depois.
func backfillStat(t *testing.T, path string) (string, time.Time) {
	t.Helper()
	fi, err := os.Stat(path)
	if err != nil {
		t.Fatalf("Stat(%s): %v", path, err)
	}
	return readFile(t, path), fi.ModTime()
}

// backfillFixedTime grava o mtime de um arquivo em um instante antigo e fixo, para detectar regravação.
func backfillFixedTime(t *testing.T, path string) {
	t.Helper()
	old := time.Date(2001, 2, 3, 4, 5, 6, 0, time.UTC)
	if err := os.Chtimes(path, old, old); err != nil {
		t.Fatalf("Chtimes(%s): %v", path, err)
	}
}

func TestMetaBackfillCreatesMissingManifestsWithNames(t *testing.T) {
	root, scope, _ := confineFixture(t)
	if err := os.MkdirAll(scopeDirOf(root, scope), 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	calls := 0

	rep, err := BackfillScopeManifests(context.Background(), root, backfillNames(&calls))
	if err != nil {
		t.Fatalf("BackfillScopeManifests: %v", err)
	}
	wantWritten := []string{
		scope.WorkspaceID.String() + "/_meta.md",
		scope.WorkspaceID.String() + "/" + scope.ProjectID.String() + "/_meta.md",
	}
	if strings.Join(rep.Written, "|") != strings.Join(wantWritten, "|") {
		t.Fatalf("Written = %v, quer %v", rep.Written, wantWritten)
	}
	if len(rep.Invalid) != 0 || len(rep.Errors) != 0 {
		t.Fatalf("Invalid = %v, Errors = %v, quer vazios", rep.Invalid, rep.Errors)
	}
	if calls != 1 {
		t.Fatalf("resolver chamado %d vezes, quer 1", calls)
	}

	wsID := scope.WorkspaceID.String()
	projID := scope.ProjectID.String()
	wsGot := readFile(t, filepath.Join(root, wsID, "_meta.md"))
	if !strings.Contains(wsGot, "name: Workspace Demo") || !strings.Contains(wsGot, "workspace_id: "+wsID) {
		t.Fatalf("manifesto do workspace sem nome ou id:\n%s", wsGot)
	}
	projGot := readFile(t, filepath.Join(root, wsID, projID, "_meta.md"))
	for _, want := range []string{
		"type: Scope Manifest",
		"name: Projeto X",
		"workspace_id: " + wsID,
		"project_id: " + projID,
		"repo_path: /repos/projeto-x",
	} {
		if !strings.Contains(projGot, want) {
			t.Fatalf("manifesto do projeto sem %q:\n%s", want, projGot)
		}
	}
	// Os manifestos criados têm que passar na leitura do reindex.
	if name, err := manifestName(filepath.Join(root, wsID, "_meta.md"), map[string]string{wsIDKey: wsID}); err != nil || name != "Workspace Demo" {
		t.Fatalf("manifestName(ws) = %q, %v", name, err)
	}
	if name, err := manifestName(filepath.Join(root, wsID, projID, "_meta.md"),
		map[string]string{wsIDKey: wsID, projIDKey: projID}); err != nil || name != "Projeto X" {
		t.Fatalf("manifestName(proj) = %q, %v", name, err)
	}
}

func TestMetaBackfillSecondRunIsIdempotent(t *testing.T) {
	root, scope, _ := confineFixture(t)
	if err := os.MkdirAll(scopeDirOf(root, scope), 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	calls := 0
	if _, err := BackfillScopeManifests(context.Background(), root, backfillNames(&calls)); err != nil {
		t.Fatalf("primeira execução: %v", err)
	}
	projPath := filepath.Join(scopeDirOf(root, scope), "_meta.md")
	wsPath := filepath.Join(root, scope.WorkspaceID.String(), "_meta.md")
	backfillFixedTime(t, projPath)
	backfillFixedTime(t, wsPath)
	projBefore, projMod := backfillStat(t, projPath)
	wsBefore, wsMod := backfillStat(t, wsPath)

	calls = 0
	rep, err := BackfillScopeManifests(context.Background(), root, backfillNames(&calls))
	if err != nil {
		t.Fatalf("segunda execução: %v", err)
	}
	if len(rep.Written) != 0 || len(rep.Invalid) != 0 || len(rep.Errors) != 0 {
		t.Fatalf("segunda execução: Written=%v Invalid=%v Errors=%v, quer tudo vazio", rep.Written, rep.Invalid, rep.Errors)
	}
	if calls != 0 {
		t.Fatalf("resolver chamado %d vezes na segunda execução, quer 0", calls)
	}
	if got, mod := backfillStat(t, projPath); got != projBefore || !mod.Equal(projMod) {
		t.Fatalf("manifesto do projeto mudou na segunda execução (mtime %v -> %v)", projMod, mod)
	}
	if got, mod := backfillStat(t, wsPath); got != wsBefore || !mod.Equal(wsMod) {
		t.Fatalf("manifesto do workspace mudou na segunda execução (mtime %v -> %v)", wsMod, mod)
	}
}

func TestMetaBackfillKeepsValidManifestIntact(t *testing.T) {
	root, scope, _ := confineFixture(t)
	// Escopo com os dois manifestos válidos, com nomes diferentes dos que o resolver devolveria.
	old := metaNames
	old.WorkspaceName = "Nome Antigo WS"
	old.ProjectName = "Nome Antigo Projeto"
	if _, err := EnsureScopeManifest(root, scope, old); err != nil {
		t.Fatalf("EnsureScopeManifest: %v", err)
	}
	projPath := filepath.Join(scopeDirOf(root, scope), "_meta.md")
	wsPath := filepath.Join(root, scope.WorkspaceID.String(), "_meta.md")
	backfillFixedTime(t, projPath)
	backfillFixedTime(t, wsPath)
	projBefore, projMod := backfillStat(t, projPath)
	wsBefore, wsMod := backfillStat(t, wsPath)

	calls := 0
	rep, err := BackfillScopeManifests(context.Background(), root, backfillNames(&calls))
	if err != nil {
		t.Fatalf("BackfillScopeManifests: %v", err)
	}
	if len(rep.Written) != 0 || len(rep.Invalid) != 0 || len(rep.Errors) != 0 {
		t.Fatalf("Written=%v Invalid=%v Errors=%v, quer tudo vazio", rep.Written, rep.Invalid, rep.Errors)
	}
	if calls != 0 {
		t.Fatalf("resolver chamado com manifestos válidos (%d vezes), quer 0", calls)
	}
	if got, mod := backfillStat(t, projPath); got != projBefore || !mod.Equal(projMod) {
		t.Fatalf("manifesto válido do projeto foi tocado")
	}
	if got, mod := backfillStat(t, wsPath); got != wsBefore || !mod.Equal(wsMod) {
		t.Fatalf("manifesto válido do workspace foi tocado")
	}
}

func TestMetaBackfillFillsOnlyMissingFileBesideValidOne(t *testing.T) {
	root, scope, _ := confineFixture(t)
	// Só o manifesto do workspace existe (válido, com nome antigo). O do projeto falta.
	old := metaNames
	old.WorkspaceName = "Nome Antigo WS"
	if _, err := EnsureScopeManifest(root, scope, old); err != nil {
		t.Fatalf("EnsureScopeManifest: %v", err)
	}
	projPath := filepath.Join(scopeDirOf(root, scope), "_meta.md")
	if err := os.Remove(projPath); err != nil {
		t.Fatalf("Remove: %v", err)
	}
	wsPath := filepath.Join(root, scope.WorkspaceID.String(), "_meta.md")
	backfillFixedTime(t, wsPath)
	wsBefore, wsMod := backfillStat(t, wsPath)

	calls := 0
	rep, err := BackfillScopeManifests(context.Background(), root, backfillNames(&calls))
	if err != nil {
		t.Fatalf("BackfillScopeManifests: %v", err)
	}
	wantWritten := scope.WorkspaceID.String() + "/" + scope.ProjectID.String() + "/_meta.md"
	if strings.Join(rep.Written, "|") != wantWritten {
		t.Fatalf("Written = %v, quer [%s]", rep.Written, wantWritten)
	}
	if got, mod := backfillStat(t, wsPath); got != wsBefore || !mod.Equal(wsMod) {
		t.Fatalf("manifesto válido do workspace foi regravado")
	}
	if !strings.Contains(readFile(t, projPath), "name: Projeto X") {
		t.Fatalf("manifesto do projeto criado sem o nome do resolver")
	}
}

func TestMetaBackfillReportsInvalidManifestWithoutOverwrite(t *testing.T) {
	root, scope, _ := confineFixture(t)
	projDir := scopeDirOf(root, scope)
	if err := os.MkdirAll(projDir, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	// Manifesto do projeto existe, mas sem frontmatter: inválido.
	projPath := filepath.Join(projDir, "_meta.md")
	const garbage = "isto não é frontmatter\n"
	if err := os.WriteFile(projPath, []byte(garbage), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	calls := 0
	rep, err := BackfillScopeManifests(context.Background(), root, backfillNames(&calls))
	if err != nil {
		t.Fatalf("BackfillScopeManifests: %v", err)
	}
	if len(rep.Invalid) != 1 || !strings.Contains(rep.Invalid[0], projPath) {
		t.Fatalf("Invalid = %v, quer uma entrada com o caminho %s", rep.Invalid, projPath)
	}
	if got := readFile(t, projPath); got != garbage {
		t.Fatalf("manifesto inválido foi sobrescrito: %q", got)
	}
	for _, rel := range rep.Written {
		if strings.HasSuffix(rel, scope.ProjectID.String()+"/_meta.md") {
			t.Fatalf("manifesto inválido do projeto aparece em Written: %v", rep.Written)
		}
	}
	// O workspace, que faltava, é criado mesmo com o projeto inválido.
	if _, err := os.Stat(filepath.Join(root, scope.WorkspaceID.String(), "_meta.md")); err != nil {
		t.Fatalf("manifesto do workspace ausente não foi criado: %v", err)
	}
}

func TestMetaBackfillReportsInvalidIdentityWithoutOverwrite(t *testing.T) {
	root, scope, _ := confineFixture(t)
	if _, err := EnsureScopeManifest(root, scope, metaNames); err != nil {
		t.Fatalf("EnsureScopeManifest: %v", err)
	}
	// O manifesto do projeto traz outro project_id: não é o desse escopo, então é inválido.
	projPath := filepath.Join(scopeDirOf(root, scope), "_meta.md")
	other := strings.Replace(readFile(t, projPath), scope.ProjectID.String(), "0190a1b2-c3d4-7e5f-8a6b-000000000000", 1)
	if err := os.WriteFile(projPath, []byte(other), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	calls := 0
	rep, err := BackfillScopeManifests(context.Background(), root, backfillNames(&calls))
	if err != nil {
		t.Fatalf("BackfillScopeManifests: %v", err)
	}
	if len(rep.Invalid) != 1 {
		t.Fatalf("Invalid = %v, quer uma entrada", rep.Invalid)
	}
	if got := readFile(t, projPath); got != other {
		t.Fatalf("manifesto com identificador divergente foi sobrescrito")
	}
}

func TestMetaBackfillResolverErrorWritesNothing(t *testing.T) {
	root, scope, _ := confineFixture(t)
	if err := os.MkdirAll(scopeDirOf(root, scope), 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	failing := func(domain.Scope) (ScopeNames, error) {
		return ScopeNames{}, errors.New("banco indisponível")
	}

	rep, err := BackfillScopeManifests(context.Background(), root, failing)
	if err != nil {
		t.Fatalf("BackfillScopeManifests: %v", err)
	}
	if len(rep.Errors) != 1 || len(rep.Written) != 0 {
		t.Fatalf("Errors = %v, Written = %v, quer 1 erro e nenhuma escrita", rep.Errors, rep.Written)
	}
	if _, err := os.Stat(filepath.Join(scopeDirOf(root, scope), "_meta.md")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("manifesto criado mesmo com erro de nomes: %v", err)
	}
}
