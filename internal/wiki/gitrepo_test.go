package wiki

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"testing"
)

// gitInDir roda git de leitura em dir e devolve a saída sem espaços nas pontas.
// Não usa o adaptador, para que a verificação não dependa do código testado.
func gitInDir(t *testing.T, dir string, args ...string) string {
	t.Helper()
	full := append([]string{"-c", "core.quotePath=false"}, args...)
	cmd := exec.Command("git", full...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
	return strings.TrimSpace(string(out))
}

// writeFile grava conteúdo em root/rel, criando diretórios intermediários.
func writeFile(t *testing.T, root, rel, content string) {
	t.Helper()
	full := filepath.Join(root, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
}

var sha1Hex = regexp.MustCompile(`^[0-9a-f]{40}$`)

func TestGitOpenCreatesRepoWithMainBranchAndGitignore(t *testing.T) {
	root := t.TempDir()

	if _, err := Open(root); err != nil {
		t.Fatalf("Open: %v", err)
	}

	if _, err := os.Stat(filepath.Join(root, ".git")); err != nil {
		t.Fatalf(".git não criado: %v", err)
	}
	if got := gitInDir(t, root, "symbolic-ref", "--short", "HEAD"); got != "main" {
		t.Fatalf("branch = %q, quer main", got)
	}
	ignore, err := os.ReadFile(filepath.Join(root, ".gitignore"))
	if err != nil {
		t.Fatalf("ReadFile .gitignore: %v", err)
	}
	if !strings.Contains(string(ignore), ".ai-memory-tmp.*") {
		t.Fatalf(".gitignore sem .ai-memory-tmp.*: %q", ignore)
	}
	// sem commit inicial
	cmd := exec.Command("git", "rev-parse", "--verify", "-q", "HEAD")
	cmd.Dir = root
	if err := cmd.Run(); err == nil {
		t.Fatal("Open criou commit inicial; não deveria")
	}
}

func TestGitOpenKeepsExistingGitignoreAndRepo(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, ".gitignore", "custom\n")

	if _, err := Open(root); err != nil {
		t.Fatalf("Open: %v", err)
	}
	if _, err := Open(root); err != nil {
		t.Fatalf("segunda Open: %v", err)
	}
	got, err := os.ReadFile(filepath.Join(root, ".gitignore"))
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if string(got) != "custom\n" {
		t.Fatalf(".gitignore existente foi alterado: %q", got)
	}
}

func TestGitCommitWritesExactMessageAndAuthor(t *testing.T) {
	root := t.TempDir()
	repo, err := Open(root)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	writeFile(t, root, "ws/proj/a.md", "conteúdo\n")
	msg := "page-write: ws/proj/a.md"

	checkpoint, err := repo.Commit(context.Background(), []string{"ws/proj/a.md"}, msg)
	if err != nil {
		t.Fatalf("Commit: %v", err)
	}
	if !sha1Hex.MatchString(checkpoint) {
		t.Fatalf("checkpoint = %q, quer hash hexadecimal de 40 caracteres", checkpoint)
	}
	if head := gitInDir(t, root, "rev-parse", "HEAD"); head != checkpoint {
		t.Fatalf("checkpoint %q != HEAD %q", checkpoint, head)
	}
	if got := gitInDir(t, root, "log", "-1", "--format=%s"); got != msg {
		t.Fatalf("mensagem = %q, quer %q", got, msg)
	}
	if got := gitInDir(t, root, "log", "-1", "--format=%an <%ae>|%cn <%ce>"); got != "ai-memory <ai-memory@local>|ai-memory <ai-memory@local>" {
		t.Fatalf("autor/committer = %q", got)
	}
	if raw := gitInDir(t, root, "cat-file", "commit", "HEAD"); strings.Contains(raw, "gpgsig") {
		t.Fatal("commit assinado; commit.gpgsign deveria ser false")
	}
	if files := gitInDir(t, root, "ls-tree", "-r", "--name-only", "HEAD"); files != "ws/proj/a.md" {
		t.Fatalf("arquivos no commit = %q", files)
	}
}

func TestGitCommitWithoutChangesReturnsEmptyCheckpoint(t *testing.T) {
	root := t.TempDir()
	repo, err := Open(root)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	writeFile(t, root, "a.md", "x\n")
	if _, err := repo.Commit(context.Background(), []string{"a.md"}, "page-write: a.md"); err != nil {
		t.Fatalf("primeiro Commit: %v", err)
	}

	checkpoint, err := repo.Commit(context.Background(), []string{"a.md"}, "page-write: a.md")
	if err != nil {
		t.Fatalf("Commit sem mudança deveria dar erro nil, deu: %v", err)
	}
	if checkpoint != "" {
		t.Fatalf("checkpoint = %q, quer vazio", checkpoint)
	}
	if n := gitInDir(t, root, "rev-list", "--count", "HEAD"); n != "1" {
		t.Fatalf("commits = %s, quer 1", n)
	}
}

func TestGitCommitHandlesPathWithSpace(t *testing.T) {
	root := t.TempDir()
	repo, err := Open(root)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	rel := "minha wiki/proj x/nota com espaço.md"
	writeFile(t, root, rel, "y\n")

	checkpoint, err := repo.Commit(context.Background(), []string{rel}, "page-write: minha wiki/proj x/nota com espaço.md")
	if err != nil {
		t.Fatalf("Commit: %v", err)
	}
	if checkpoint == "" {
		t.Fatal("checkpoint vazio para mudança real")
	}
	if files := gitInDir(t, root, "ls-tree", "-r", "--name-only", "HEAD"); files != rel {
		t.Fatalf("arquivos no commit = %q, quer %q", files, rel)
	}
}

func TestGitCommitRejectsNonRelativePaths(t *testing.T) {
	root := t.TempDir()
	repo, err := Open(root)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	bad := []string{"", ".", "../fora.md", filepath.Join(root, "a.md")}
	for _, p := range bad {
		if _, err := repo.Commit(context.Background(), []string{p}, "page-write: x"); err == nil {
			t.Errorf("Commit aceitou caminho inválido %q", p)
		}
	}
	if _, err := repo.Commit(context.Background(), nil, "page-write: x"); err == nil {
		t.Error("Commit aceitou lista de caminhos vazia")
	}
}

func TestGitCommitConcurrentIsSerialized(t *testing.T) {
	root := t.TempDir()
	repo, err := Open(root)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	writeFile(t, root, "f1.md", "1\n")
	writeFile(t, root, "f2.md", "2\n")

	type result struct {
		checkpoint string
		err        error
	}
	files := []string{"f1.md", "f2.md"}
	results := make([]result, len(files))
	var wg sync.WaitGroup
	for i, f := range files {
		wg.Add(1)
		go func(i int, f string) {
			defer wg.Done()
			cp, err := repo.Commit(context.Background(), []string{f}, "page-write: "+f)
			results[i] = result{cp, err}
		}(i, f)
	}
	wg.Wait()

	for i, res := range results {
		if res.err != nil {
			t.Fatalf("commit %d falhou: %v", i, res.err)
		}
		if res.checkpoint == "" {
			t.Fatalf("commit %d sem checkpoint", i)
		}
	}
	if results[0].checkpoint == results[1].checkpoint {
		t.Fatal("dois commits com o mesmo hash")
	}
	if n := gitInDir(t, root, "rev-list", "--count", "HEAD"); n != "2" {
		t.Fatalf("commits = %s, quer 2", n)
	}
}

func TestGitOpenWithoutGitInPathReturnsErrGitNotFound(t *testing.T) {
	t.Setenv("PATH", t.TempDir())

	_, err := Open(t.TempDir())
	if !errors.Is(err, ErrGitNotFound) {
		t.Fatalf("err = %v, quer ErrGitNotFound", err)
	}
}
