package wiki

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
)

// ErrGitNotFound indica que o executável git não está no PATH.
var ErrGitNotFound = errors.New("git não encontrado no PATH")

// gitIgnoreLine é a regra do .gitignore interno, que impede que temporários
// da escrita atômica entrem em commit (ADR-25).
const gitIgnoreLine = ".ai-memory-tmp.*\n"

// gitIdentity são as opções -c aplicadas a todo comando git do adaptador.
// Autor fixo e assinatura desligada (ADR-25, P-09).
var gitIdentity = []string{
	"-c", "user.name=ai-memory",
	"-c", "user.email=ai-memory@local",
	"-c", "commit.gpgsign=false",
}

// Repo é o repositório git da wiki, operado pelo executável git do sistema
// via os/exec. Um commit por vez, serializado pelo mutex do próprio Repo.
type Repo struct {
	root string
	mu   sync.Mutex
}

// Open valida que git está no PATH e prepara o repositório em root: cria
// root se faltar, faz `git init --initial-branch=main` se não houver `.git`
// e cria `.gitignore` com `.ai-memory-tmp.*` se o arquivo faltar. Não cria
// commit inicial. Erro de PATH devolve ErrGitNotFound (com %w).
func Open(root string) (*Repo, error) {
	if _, err := exec.LookPath("git"); err != nil {
		return nil, fmt.Errorf("wiki: abrir repositório: %w; instale git e coloque-o no PATH", ErrGitNotFound)
	}
	abs, err := filepath.Abs(root)
	if err != nil {
		return nil, fmt.Errorf("wiki: resolver raiz do repositório: %w", err)
	}
	if err := os.MkdirAll(abs, 0o755); err != nil {
		return nil, fmt.Errorf("wiki: criar raiz do repositório: %w", err)
	}
	r := &Repo{root: abs}

	_, err = os.Stat(filepath.Join(abs, ".git"))
	switch {
	case err == nil:
		// já é um repositório; nada a fazer
	case errors.Is(err, fs.ErrNotExist):
		ctx := context.Background()
		if out, err := r.runCtx(ctx, "init", "--initial-branch=main"); err != nil {
			return nil, gitFailure(ctx, "init", err, out)
		}
	default:
		return nil, fmt.Errorf("wiki: verificar .git em %s: %w", abs, err)
	}

	if err := ensureGitignore(abs); err != nil {
		return nil, err
	}
	return r, nil
}

// Commit faz `git add -- <paths>` e `git commit -m <message>` sob o mutex do
// Repo, e devolve o hash completo do commit criado (`git rev-parse HEAD`).
//
// paths são relativos à raiz do repositório; caminhos absolutos, vazios, com
// `..` ou iguais a `.` são rejeitados antes de qualquer comando git.
//
// Se não há nada staged depois do add, nenhum commit é criado: devolve
// checkpoint vazio e erro nil. ctx cancela o comando git em execução.
func (r *Repo) Commit(ctx context.Context, paths []string, message string) (string, error) {
	rel, err := relPaths(paths)
	if err != nil {
		return "", err
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	addArgs := append([]string{"add", "--"}, rel...)
	if out, err := r.runCtx(ctx, addArgs...); err != nil {
		return "", gitFailure(ctx, "add", err, out)
	}

	changed, err := r.hasStagedChanges(ctx)
	if err != nil {
		return "", err
	}
	if !changed {
		return "", nil
	}

	if out, err := r.runCtx(ctx, "commit", "-m", message); err != nil {
		return "", gitFailure(ctx, "commit", err, out)
	}
	out, err := r.runCtx(ctx, "rev-parse", "HEAD")
	if err != nil {
		return "", gitFailure(ctx, "rev-parse", err, out)
	}
	return strings.TrimSpace(string(out)), nil
}

// hasStagedChanges usa `git diff --cached --quiet`: exit 0 sem mudança,
// exit 1 com mudança, qualquer outra falha é erro.
func (r *Repo) hasStagedChanges(ctx context.Context) (bool, error) {
	out, err := r.runCtx(ctx, "diff", "--cached", "--quiet")
	if err == nil {
		return false, nil
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) && exitErr.ExitCode() == 1 {
		return true, nil
	}
	return false, gitFailure(ctx, "diff --cached", err, out)
}

// runCtx monta `git <identidade> --literal-pathspecs <args>` e executa com ctx.
// --literal-pathspecs impede que caracteres de glob em nomes de arquivo sejam
// interpretados como padrão.
func (r *Repo) runCtx(ctx context.Context, args ...string) ([]byte, error) {
	full := make([]string, 0, len(gitIdentity)+1+len(args))
	full = append(full, gitIdentity...)
	full = append(full, "--literal-pathspecs")
	full = append(full, args...)
	cmd := exec.CommandContext(ctx, "git", full...)
	cmd.Dir = r.root
	return cmd.CombinedOutput()
}

// relPaths valida e normaliza os caminhos do commit para forma relativa com
// barras normais, que é o que o git recebe depois de `--`.
func relPaths(paths []string) ([]string, error) {
	if len(paths) == 0 {
		return nil, errors.New("wiki: commit sem caminhos")
	}
	out := make([]string, 0, len(paths))
	for _, p := range paths {
		if !filepath.IsLocal(p) {
			return nil, fmt.Errorf("wiki: caminho do commit deve ser relativo à raiz: %q", p)
		}
		clean := filepath.Clean(p)
		if clean == "." {
			return nil, fmt.Errorf("wiki: caminho do commit não pode ser a raiz: %q", p)
		}
		out = append(out, filepath.ToSlash(clean))
	}
	return out, nil
}

// gitFailure embrulha a falha de um comando git com a saída do próprio git.
// Se ctx foi cancelado, devolve o erro do contexto.
func gitFailure(ctx context.Context, op string, err error, out []byte) error {
	if ctxErr := ctx.Err(); ctxErr != nil {
		return fmt.Errorf("wiki: git %s: %w", op, ctxErr)
	}
	return fmt.Errorf("wiki: git %s: %w: %s", op, err, strings.TrimSpace(string(out)))
}

// ensureGitignore cria .gitignore com a regra de temporários se o arquivo
// faltar. Se já existe, não é alterado.
func ensureGitignore(root string) error {
	path := filepath.Join(root, ".gitignore")
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		if errors.Is(err, fs.ErrExist) {
			return nil
		}
		return fmt.Errorf("wiki: criar .gitignore: %w", err)
	}
	if _, err := f.WriteString(gitIgnoreLine); err != nil {
		_ = f.Close()
		return fmt.Errorf("wiki: escrever .gitignore: %w", err)
	}
	if err := f.Close(); err != nil {
		return fmt.Errorf("wiki: fechar .gitignore: %w", err)
	}
	return nil
}
