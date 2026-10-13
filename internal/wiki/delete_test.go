package wiki

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/phm-aguiar/ai-memory-go/internal/domain"
	"github.com/phm-aguiar/ai-memory-go/internal/storage"
)

// delRel é o caminho relativo à raiz da wiki de uma página do ambiente de teste.
func delRel(env *wpEnv, page string) string {
	return env.scope.WorkspaceID.String() + "/" + env.scope.ProjectID.String() + "/" + page
}

// delPath valida um caminho de página para os testes de remoção.
func delPath(t *testing.T, page string) domain.PagePath {
	t.Helper()
	p, err := domain.ParsePagePath(page)
	if err != nil {
		t.Fatalf("ParsePagePath(%q): %v", page, err)
	}
	return p
}

// delNewVersion gera um PageID que não é a versão corrente de nenhuma página do teste.
func delNewVersion(t *testing.T) domain.PageID {
	t.Helper()
	id, err := domain.NewID()
	if err != nil {
		t.Fatalf("NewID: %v", err)
	}
	return domain.PageID(id)
}

func TestDeletePage_CaminhoFeliz(t *testing.T) {
	env := wpNewEnv(t)
	written := wpMustWrite(t, env, wpInput(t, env, "notas/apagar.md", "# Apagar\n\nCorpo a remover.\n"))
	commits := wpCommits(t, env)
	*env.clock = env.clock.Add(time.Minute)

	path := delPath(t, "notas/apagar.md")
	res, err := env.store.DeletePage(env.ctx, env.scope, path, written.Version)
	if err != nil {
		t.Fatalf("DeletePage: %v", err)
	}
	rel := delRel(env, "notas/apagar.md")
	if !res.Deleted {
		t.Fatal("Deleted = false, quer true")
	}
	if res.Path != rel {
		t.Fatalf("Path = %q, quer %q", res.Path, rel)
	}
	if res.Checkpoint == "" {
		t.Fatal("Checkpoint vazio: deveria ter commit")
	}
	if len(res.Avisos) != 0 {
		t.Fatalf("Avisos inesperados: %v", res.Avisos)
	}

	// Arquivo removido do disco.
	if _, err := os.Stat(wpAbs(env, "notas/apagar.md")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("arquivo ainda existe após a remoção: %v", err)
	}

	// Linha corrente removida do banco.
	if _, err := storage.ReadPage(env.ctx, env.reader, wpRef(t, env, "notas/apagar.md")); !errors.Is(err, storage.ErrPageNotFound) {
		t.Fatalf("ReadPage após remoção: erro = %v, quer ErrPageNotFound", err)
	}

	// Commit com a mensagem exata, a remoção do arquivo e o log.md.
	if subj := strings.TrimSpace(gitInDir(t, env.root, "log", "-1", "--format=%s")); subj != "page-delete: "+rel {
		t.Fatalf("mensagem do commit = %q, quer %q", subj, "page-delete: "+rel)
	}
	if hash := strings.TrimSpace(gitInDir(t, env.root, "rev-parse", "HEAD")); hash != res.Checkpoint {
		t.Fatalf("Checkpoint %q, HEAD %q", res.Checkpoint, hash)
	}
	if n := wpCommits(t, env); n != commits+1 {
		t.Fatalf("commits %d -> %d, quer +1", commits, n)
	}
	changed := gitInDir(t, env.root, "show", "--name-status", "--format=", "HEAD")
	if !strings.Contains(changed, "D\t"+rel) {
		t.Fatalf("commit não registra a remoção de %q:\n%s", rel, changed)
	}
	if !strings.Contains(changed, delRel(env, "log.md")) {
		t.Fatalf("commit sem log.md:\n%s", changed)
	}
	if tracked := gitInDir(t, env.root, "ls-files"); strings.Contains(tracked, rel) {
		t.Fatalf("página ainda rastreada pelo git:\n%s", tracked)
	}

	// Linha do log, depois da linha de escrita.
	want := "## [2026-03-01T10:00:00Z] page-write | Apagar\n" +
		"## [2026-03-01T10:01:00Z] page-delete | Apagar\n"
	if got := readFile(t, wpAbs(env, "log.md")); got != want {
		t.Fatalf("log.md = %q, quer %q", got, want)
	}
}

func TestDeletePage_SemVersaoCorrenteIdempotente(t *testing.T) {
	env := wpNewEnv(t)
	// Um commit de base, para que a contagem de commits funcione.
	wpMustWrite(t, env, wpInput(t, env, "notas/base.md", "# Base\n\nCorpo.\n"))
	commits := wpCommits(t, env)

	// Página que nunca existiu.
	res, err := env.store.DeletePage(env.ctx, env.scope, delPath(t, "notas/nunca.md"), domain.PageID{})
	if err != nil {
		t.Fatalf("DeletePage de página inexistente: %v", err)
	}
	if res.Deleted || res.Checkpoint != "" {
		t.Fatalf("resultado = %+v, quer Deleted=false sem checkpoint", res)
	}

	// Arquivo solto no disco, sem linha no banco: nada é tocado.
	orfao := wpAbs(env, "notas/orfao.md")
	if err := os.MkdirAll(filepath.Dir(orfao), 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(orfao, []byte("# Órfão\n"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	res, err = env.store.DeletePage(env.ctx, env.scope, delPath(t, "notas/orfao.md"), delNewVersion(t))
	if err != nil {
		t.Fatalf("DeletePage de arquivo órfão: %v", err)
	}
	if res.Deleted {
		t.Fatal("Deleted = true sem versão corrente")
	}
	if _, err := os.Stat(orfao); err != nil {
		t.Fatalf("arquivo órfão foi removido sem versão corrente: %v", err)
	}

	if n := wpCommits(t, env); n != commits {
		t.Fatalf("commits %d -> %d: remoção idempotente não deveria commitar", commits, n)
	}
}

func TestDeletePage_SegundaRemocaoEIdempotente(t *testing.T) {
	env := wpNewEnv(t)
	written := wpMustWrite(t, env, wpInput(t, env, "notas/duas.md", "# Duas\n\nCorpo.\n"))
	path := delPath(t, "notas/duas.md")
	if _, err := env.store.DeletePage(env.ctx, env.scope, path, written.Version); err != nil {
		t.Fatalf("primeira remoção: %v", err)
	}
	commits := wpCommits(t, env)

	res, err := env.store.DeletePage(env.ctx, env.scope, path, written.Version)
	if err != nil {
		t.Fatalf("segunda remoção: %v", err)
	}
	if res.Deleted || res.Checkpoint != "" {
		t.Fatalf("segunda remoção = %+v, quer Deleted=false sem checkpoint", res)
	}
	if n := wpCommits(t, env); n != commits {
		t.Fatalf("commits %d -> %d: segunda remoção não deveria commitar", commits, n)
	}
}

func TestDeletePage_FalhaNoBancoReverteArquivo(t *testing.T) {
	env := wpNewEnv(t)
	written := wpMustWrite(t, env, wpInput(t, env, "notas/reverte.md", "# Reverte\n\nconteúdo guardado\n"))
	prev := readFile(t, wpAbs(env, "notas/reverte.md"))
	commits := wpCommits(t, env)
	logBefore := readFile(t, wpAbs(env, "log.md"))

	// Chamada 1 do writer é a leitura (ReadPage); a 2 é DeletePath, que falha depois de executar.
	failing := *env.store
	failing.Writer = &wpFailingWriter{Writer: env.writer, failOn: 2}
	*env.clock = env.clock.Add(time.Minute)
	_, err := failing.DeletePage(env.ctx, env.scope, delPath(t, "notas/reverte.md"), written.Version)
	if !errors.Is(err, errBoom) {
		t.Fatalf("erro = %v, quer encadear errBoom", err)
	}
	if errors.Is(err, ErrInconsistentState) {
		t.Fatalf("restauração funcionou e não deveria devolver ErrInconsistentState: %v", err)
	}

	if got := readFile(t, wpAbs(env, "notas/reverte.md")); got != prev {
		t.Fatalf("arquivo não foi restaurado:\n%s", got)
	}
	if n := wpCommits(t, env); n != commits {
		t.Fatalf("commits %d -> %d: falha não deveria commitar", commits, n)
	}
	if readFile(t, wpAbs(env, "log.md")) != logBefore {
		t.Fatal("log.md alterado numa falha")
	}
	page, err := storage.ReadPage(env.ctx, env.reader, wpRef(t, env, "notas/reverte.md"))
	if err != nil {
		t.Fatalf("ReadPage após falha: %v", err)
	}
	if page.Version != written.Version {
		t.Fatalf("versão corrente mudou na falha: %s", page.Version.String())
	}
	entries, err := os.ReadDir(filepath.Dir(wpAbs(env, "notas/reverte.md")))
	if err != nil {
		t.Fatalf("ReadDir: %v", err)
	}
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), tmpPrefix) {
			t.Fatalf("temporário deixado para trás: %s", e.Name())
		}
	}
}

func TestDeletePage_ErrVersionMismatchPreservaArquivo(t *testing.T) {
	env := wpNewEnv(t)
	written := wpMustWrite(t, env, wpInput(t, env, "notas/versao.md", "# Versão\n\nconteúdo\n"))
	prev := readFile(t, wpAbs(env, "notas/versao.md"))
	commits := wpCommits(t, env)

	res, err := env.store.DeletePage(env.ctx, env.scope, delPath(t, "notas/versao.md"), delNewVersion(t))
	if !errors.Is(err, storage.ErrVersionMismatch) {
		t.Fatalf("erro = %v, quer ErrVersionMismatch", err)
	}
	if res.Deleted {
		t.Fatal("Deleted = true com versão divergente")
	}
	if got := readFile(t, wpAbs(env, "notas/versao.md")); got != prev {
		t.Fatalf("arquivo alterado com versão divergente:\n%s", got)
	}
	page, err := storage.ReadPage(env.ctx, env.reader, wpRef(t, env, "notas/versao.md"))
	if err != nil {
		t.Fatalf("ReadPage após divergência: %v", err)
	}
	if page.Version != written.Version {
		t.Fatalf("versão corrente mudou: %s", page.Version.String())
	}
	if n := wpCommits(t, env); n != commits {
		t.Fatalf("commits %d -> %d: divergência não deveria commitar", commits, n)
	}
}

func TestDeletePage_VersaoSupersededContinuaNoBanco(t *testing.T) {
	env := wpNewEnv(t)
	first := wpMustWrite(t, env, wpInput(t, env, "notas/historico.md", "# Histórico\n\nversão um\n"))
	*env.clock = env.clock.Add(time.Minute)
	second := wpMustWrite(t, env, wpInput(t, env, "notas/historico.md", "# Histórico\n\nversão dois\n"))
	if second.Outcome != storage.UpsertSuperseded {
		t.Fatalf("Outcome = %q, quer superseded", second.Outcome)
	}

	*env.clock = env.clock.Add(time.Minute)
	res, err := env.store.DeletePage(env.ctx, env.scope, delPath(t, "notas/historico.md"), second.Version)
	if err != nil {
		t.Fatalf("DeletePage: %v", err)
	}
	if !res.Deleted {
		t.Fatal("Deleted = false, quer true")
	}

	// A versão anterior não foi apagada nem reativada (INV-16).
	chain, err := storage.PageChain(env.ctx, env.reader, wpRef(t, env, "notas/historico.md"))
	if err != nil {
		t.Fatalf("PageChain: %v", err)
	}
	if len(chain) != 1 {
		t.Fatalf("cadeia com %d versões após remoção, quer 1 (a anterior)", len(chain))
	}
	if chain[0].ID != first.Version {
		t.Fatalf("versão restante %s, quer a anterior %s", chain[0].ID.String(), first.Version.String())
	}
	if chain[0].IsLatest {
		t.Fatal("versão anterior virou corrente")
	}
	if _, err := storage.ReadPage(env.ctx, env.reader, wpRef(t, env, "notas/historico.md")); !errors.Is(err, storage.ErrPageNotFound) {
		t.Fatalf("ReadPage: erro = %v, quer ErrPageNotFound", err)
	}
}

func TestDeletePage_CaminhoForaDaRaizRecusado(t *testing.T) {
	env := wpNewEnv(t)
	written := wpMustWrite(t, env, wpInput(t, env, "notas/fora.md", "# Fora\n"))
	path := delPath(t, "notas/fora.md")

	relativa := *env.store
	relativa.Root = "relativa/raiz"
	if _, err := relativa.DeletePage(env.ctx, env.scope, path, written.Version); !errors.Is(err, ErrConfinement) {
		t.Fatalf("raiz relativa: erro = %v, quer ErrConfinement", err)
	}

	// Componente do caminho que é link para fora da raiz. A página no banco fica intacta.
	fora := t.TempDir()
	link := filepath.Join(env.root, env.scope.WorkspaceID.String())
	if err := os.Rename(link, link+".mover"); err != nil {
		t.Fatalf("Rename: %v", err)
	}
	if err := os.Symlink(fora, link); err != nil {
		if out, jerr := exec.Command("cmd", "/c", "mklink", "/J", link, fora).CombinedOutput(); jerr != nil {
			t.Skipf("link simbólico e junção indisponíveis: %v; %v: %s", err, jerr, out)
		}
	}
	if _, err := env.store.DeletePage(env.ctx, env.scope, path, written.Version); !errors.Is(err, ErrConfinement) {
		t.Fatalf("link no caminho: erro = %v, quer ErrConfinement", err)
	}
	if entries, _ := os.ReadDir(fora); len(entries) != 0 {
		t.Fatalf("remoção escapou para fora da raiz: %d entradas", len(entries))
	}
}
