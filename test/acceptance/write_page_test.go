package acceptance

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/phm-aguiar/ai-memory-go/internal/domain"
	"github.com/phm-aguiar/ai-memory-go/internal/storage"
	"github.com/phm-aguiar/ai-memory-go/internal/wiki"
)

// Títulos e textos fixos do cenário. O corpo não tem segredo.
const (
	dbPageTitle = "Banco de dados"
	dbPageBody  = "Escolhemos SQLite em modo WAL para o armazenamento local, sem servidor externo."
	dbPagePath  = "decisions/db.md"
	dbLogEvent  = "page-write | " + dbPageTitle
)

// TestWritePageAtualizaArquivoIndiceEGit espelha o cenário 1 de
// specs/features/001-nucleo/acceptance/nucleo.feature: "Escrever uma página atualiza
// arquivo, índice e git juntos". Os passos Dado/Quando/Então viram subtestes em sequência.
// Se um passo falha, os seguintes não rodam.
func TestWritePageAtualizaArquivoIndiceEGit(t *testing.T) {
	ctx := context.Background()

	// Setup comum (pré-condição de todos os passos). O TempDir é registrado primeiro, então
	// é removido por último, depois que o escritor e os bancos forem fechados.
	dataDir := t.TempDir()

	db, err := storage.Open(ctx, dataDir)
	if err != nil {
		t.Fatalf("storage.Open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	writer, err := storage.NewWriter(ctx, db)
	if err != nil {
		t.Fatalf("storage.NewWriter: %v", err)
	}
	t.Cleanup(func() { _ = writer.Close() })

	readDB, err := storage.OpenReader(ctx, dataDir)
	if err != nil {
		t.Fatalf("storage.OpenReader: %v", err)
	}
	t.Cleanup(func() { _ = readDB.Close() })
	reader := storage.NewReader(readDB)

	wikiRoot := filepath.Join(dataDir, "wiki")
	repo, err := wiki.Open(wikiRoot)
	if err != nil {
		t.Fatalf("wiki.Open: %v", err)
	}
	store := &wiki.Store{
		Root:   wikiRoot,
		Writer: writer,
		Repo:   repo,
		Locks:  wiki.NewPageLocks(),
	}

	var (
		scope domain.Scope
		rel   string // ws/proj/decisions/db.md, relativo à raiz da wiki
		res   wiki.WriteResult
	)

	if !t.Run("Dado que o projeto demo existe no workspace resolvido", func(t *testing.T) {
		wsID, err := domain.NewID()
		if err != nil {
			t.Fatalf("NewID do workspace: %v", err)
		}
		projID, err := domain.NewID()
		if err != nil {
			t.Fatalf("NewID do projeto: %v", err)
		}
		scope = domain.NewScope(domain.WorkspaceID(wsID), domain.ProjectID(projID))
		// O projeto demo é criado pelo próprio WritePage (EnsureScope). Não há resolvedor
		// central de escopo no código ainda; o escopo é montado aqui com IDs novos.
		rel = scope.WorkspaceID.String() + "/" + scope.ProjectID.String() + "/" + dbPagePath
	}) {
		return
	}

	if !t.Run("Quando o chamador pede a escrita da página decisions/db.md no projeto demo", func(t *testing.T) {
		var fm domain.Frontmatter
		if err := fm.Set("title", dbPageTitle); err != nil {
			t.Fatalf("frontmatter title: %v", err)
		}
		path, err := domain.ParsePagePath(dbPagePath)
		if err != nil {
			t.Fatalf("ParsePagePath: %v", err)
		}
		out, err := store.WritePage(ctx, wiki.PageInput{
			Scope:         scope,
			WorkspaceName: "workspace-teste",
			ProjectName:   "demo",
			RepoPath:      "repo-de-teste",
			Path:          path,
			Tier:          domain.TierSemantic,
			Body:          dbPageBody,
			Frontmatter:   fm,
			Evidence:      storage.EvidenceSource{Kind: storage.SourceSession, ID: "sessao-teste-001"},
		})
		if err != nil {
			t.Fatalf("WritePage: %v", err)
		}
		res = out
	}) {
		return
	}

	// Então o corpo e o frontmatter passam pela validação do caminho e pela sanitização.
	// Essa parte é verificada pelo conteúdo que chega ao arquivo (subteste a seguir).
	// A sanitização de segredo tem cenário próprio na feature.

	t.Run("Então o arquivo decisions/db.md é instalado com frontmatter e corpo", func(t *testing.T) {
		abs := filepath.Join(wikiRoot, scope.WorkspaceID.String(), scope.ProjectID.String(),
			"decisions", "db.md")
		data, err := os.ReadFile(abs)
		if err != nil {
			t.Fatalf("ler arquivo instalado %s: %v", abs, err)
		}
		fm, body, err := wiki.ReadMarkdown(data)
		if err != nil {
			t.Fatalf("ReadMarkdown: %v", err)
		}
		if v, _ := fm.Get("title"); v != dbPageTitle {
			t.Errorf("frontmatter title = %v, quer %q", v, dbPageTitle)
		}
		if v, _ := fm.Get("type"); v != "Decision" {
			t.Errorf("frontmatter type = %v, quer %q (derivado da família decisions)", v, "Decision")
		}
		if v, _ := fm.Get("tier"); v != string(domain.TierSemantic) {
			t.Errorf("frontmatter tier = %v, quer %q", v, domain.TierSemantic)
		}
		if !strings.Contains(body, dbPageBody) {
			t.Errorf("corpo do arquivo não contém o texto escrito; corpo = %q", body)
		}
	})

	t.Run("Então a linha corrente do banco aponta para decisions/db.md", func(t *testing.T) {
		ref := domain.NewPageRef(scope, mustPath(t, dbPagePath))
		page, err := storage.ReadPage(ctx, reader, ref)
		if err != nil {
			t.Fatalf("storage.ReadPage: %v", err)
		}
		if got := page.Ref.Path.String(); got != dbPagePath {
			t.Errorf("caminho no banco = %q, quer %q", got, dbPagePath)
		}
		if page.Version != res.Version {
			t.Errorf("versão corrente no banco = %s, quer %s", page.Version.String(), res.Version.String())
		}
		if page.Tier != domain.TierSemantic {
			t.Errorf("tier no banco = %q, quer %q", page.Tier, domain.TierSemantic)
		}
		if !strings.Contains(page.Body, dbPageBody) {
			t.Errorf("corpo no banco não contém o texto escrito")
		}
	})

	t.Run("Então um commit git existe com a mensagem page-write do caminho", func(t *testing.T) {
		subject := gitOutput(t, wikiRoot, "log", "-1", "--format=%s")
		if want := "page-write: " + rel; subject != want {
			t.Errorf("assunto do último commit = %q, quer %q", subject, want)
		}
		head := gitOutput(t, wikiRoot, "rev-parse", "HEAD")
		if head != res.Checkpoint {
			t.Errorf("HEAD = %q, checkpoint devolvido = %q", head, res.Checkpoint)
		}
	})

	t.Run("Então o log.md do projeto tem uma linha page-write para a escrita", func(t *testing.T) {
		logPath := filepath.Join(wikiRoot, scope.WorkspaceID.String(), scope.ProjectID.String(), "log.md")
		data, err := os.ReadFile(logPath)
		if err != nil {
			t.Fatalf("ler %s: %v", logPath, err)
		}
		n := 0
		for _, line := range strings.Split(string(data), "\n") {
			if strings.HasPrefix(line, "## [") && strings.Contains(line, "] "+dbLogEvent) {
				n++
			}
		}
		if n != 1 {
			t.Errorf("linhas page-write com título %q no log.md = %d, quer 1; log = %q",
				dbPageTitle, n, string(data))
		}
	})

	t.Run("Então a resposta traz identificador, caminho e checkpoint", func(t *testing.T) {
		if res.Version == (domain.PageID{}) {
			t.Errorf("Version é zero; quer identificador de página")
		}
		if res.Path != rel {
			t.Errorf("Path = %q, quer %q", res.Path, rel)
		}
		if res.Checkpoint == "" {
			t.Errorf("Checkpoint vazio; quer hash do commit")
		}
		if len(res.Avisos) != 0 {
			t.Errorf("Avisos = %q, quer nenhum", res.Avisos)
		}
	})
}

// mustPath monta um PagePath válido ou encerra o subteste.
func mustPath(t *testing.T, s string) domain.PagePath {
	t.Helper()
	p, err := domain.ParsePagePath(s)
	if err != nil {
		t.Fatalf("ParsePagePath(%q): %v", s, err)
	}
	return p
}

// gitOutput roda git com -C dir e devolve a saída aparada. Falha encerra o subteste.
func gitOutput(t *testing.T, dir string, args ...string) string {
	t.Helper()
	full := append([]string{"-C", dir}, args...)
	out, err := exec.Command("git", full...).Output()
	if err != nil {
		t.Fatalf("git %s: %v", strings.Join(args, " "), err)
	}
	return strings.TrimSpace(string(out))
}
