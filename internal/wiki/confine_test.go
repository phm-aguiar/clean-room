package wiki

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/phm-aguiar/ai-memory-go/internal/domain"
)

// confineFixture devolve uma raiz canônica, um escopo fixo e um caminho de página.
func confineFixture(t *testing.T) (string, domain.Scope, domain.PagePath) {
	t.Helper()
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatalf("EvalSymlinks(TempDir): %v", err)
	}
	ws, err := domain.ParseID("0190a1b2-c3d4-7e5f-8a6b-7c8d9e0f1a2b")
	if err != nil {
		t.Fatalf("ParseID ws: %v", err)
	}
	proj, err := domain.ParseID("0190a1b2-c3d4-7e5f-8a6b-7c8d9e0f1a2c")
	if err != nil {
		t.Fatalf("ParseID proj: %v", err)
	}
	page, err := domain.ParsePagePath("concepts/sub/a.md")
	if err != nil {
		t.Fatalf("ParsePagePath: %v", err)
	}
	scope := domain.NewScope(domain.WorkspaceID(ws), domain.ProjectID(proj))
	return root, scope, page
}

// makeLink cria um link em link apontando para target e devolve o tipo criado.
// Tenta os.Symlink. No Windows, se falhar (symlink exige privilégio), tenta junção com
// mklink /J, que não exige. Sem nenhum dos dois, pula o teste com o motivo.
func makeLink(t *testing.T, target, link string) string {
	t.Helper()
	err := os.Symlink(target, link)
	if err == nil {
		t.Logf("link criado: symlink")
		return "symlink"
	}
	if runtime.GOOS == "windows" {
		out, jerr := exec.Command("cmd", "/c", "mklink", "/J", link, target).CombinedOutput()
		if jerr == nil {
			t.Logf("link criado: junção (mklink /J); os.Symlink falhou: %v", err)
			return "junção (mklink /J)"
		}
		t.Skipf("link indisponível: symlink falhou (%v); junção falhou (%v: %s)", err, jerr, out)
	}
	t.Skipf("symlink indisponível neste ambiente: %v", err)
	return ""
}

func TestConfineNormalPathBuildsScopePath(t *testing.T) {
	root, scope, page := confineFixture(t)

	got, err := pathFor(root, scope, page)
	if err != nil {
		t.Fatalf("pathFor: %v", err)
	}
	want := filepath.Join(root, scope.WorkspaceID.String(), scope.ProjectID.String(), "concepts", "sub", "a.md")
	if got != want {
		t.Fatalf("pathFor = %q, quer %q", got, want)
	}
}

func TestConfinePathSeparatorFollowsOS(t *testing.T) {
	root, scope, page := confineFixture(t)

	got, err := pathFor(root, scope, page)
	if err != nil {
		t.Fatalf("pathFor: %v", err)
	}
	// PagePath usa "/". O caminho montado usa o separador do SO (\ no Windows).
	if runtime.GOOS == "windows" && strings.Contains(got, "/") {
		t.Fatalf("caminho do Windows com '/': %q", got)
	}
	if rel, _ := filepath.Rel(root, got); rel != filepath.Join(scope.WorkspaceID.String(), scope.ProjectID.String(), "concepts", "sub", "a.md") {
		t.Fatalf("caminho relativo inesperado: %q", rel)
	}
}

func TestConfineDoesNotCreateDirectories(t *testing.T) {
	root, scope, page := confineFixture(t)

	if _, err := pathFor(root, scope, page); err != nil {
		t.Fatalf("pathFor: %v", err)
	}
	if _, err := os.Lstat(filepath.Join(root, scope.WorkspaceID.String())); !os.IsNotExist(err) {
		t.Fatalf("pathFor criou diretório do escopo (Lstat err=%v)", err)
	}
}

func TestConfineSymlinkInScopeComponentRefused(t *testing.T) {
	outside := t.TempDir()

	cases := []struct {
		name  string
		setup func(t *testing.T, root string, scope domain.Scope)
	}{
		{
			name: "link no workspace",
			setup: func(t *testing.T, root string, scope domain.Scope) {
				makeLink(t, outside, filepath.Join(root, scope.WorkspaceID.String()))
			},
		},
		{
			name: "link no projeto",
			setup: func(t *testing.T, root string, scope domain.Scope) {
				mustMkdir(t, filepath.Join(root, scope.WorkspaceID.String()))
				makeLink(t, outside, filepath.Join(root, scope.WorkspaceID.String(), scope.ProjectID.String()))
			},
		},
		{
			name: "link em diretório intermediário do caminho",
			setup: func(t *testing.T, root string, scope domain.Scope) {
				proj := filepath.Join(root, scope.WorkspaceID.String(), scope.ProjectID.String())
				mustMkdir(t, proj)
				makeLink(t, outside, filepath.Join(proj, "concepts"))
			},
		},
		{
			name: "link no arquivo final",
			setup: func(t *testing.T, root string, scope domain.Scope) {
				dir := filepath.Join(root, scope.WorkspaceID.String(), scope.ProjectID.String(), "concepts", "sub")
				mustMkdir(t, dir)
				target := filepath.Join(outside, "a.md")
				if err := os.WriteFile(target, []byte("x"), 0o644); err != nil {
					t.Fatal(err)
				}
				makeLink(t, target, filepath.Join(dir, "a.md"))
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			root, scope, page := confineFixture(t)
			tc.setup(t, root, scope)

			_, err := pathFor(root, scope, page)
			if !errors.Is(err, ErrConfinement) {
				t.Fatalf("pathFor: erro = %v, quer ErrConfinement", err)
			}
		})
	}
}

func TestConfineRootThatIsLinkRefused(t *testing.T) {
	target := t.TempDir()
	_, scope, page := confineFixture(t)
	link := filepath.Join(t.TempDir(), "raiz-link")
	makeLink(t, target, link)

	_, err := pathFor(link, scope, page)
	if !errors.Is(err, ErrConfinement) {
		t.Fatalf("raiz link: erro = %v, quer ErrConfinement", err)
	}
}

func TestConfineRelativeRootRefused(t *testing.T) {
	_, scope, page := confineFixture(t)

	for _, root := range []string{"relativo/wiki", "", "./wiki"} {
		_, err := pathFor(root, scope, page)
		if !errors.Is(err, ErrConfinement) {
			t.Fatalf("raiz %q: erro = %v, quer ErrConfinement", root, err)
		}
	}
}

func TestConfineNonCanonicalRootRefused(t *testing.T) {
	root, scope, page := confineFixture(t)

	// Raiz com segmento ".." que volta para a própria raiz: após Clean é canônica, então passa.
	if _, err := pathFor(filepath.Join(root, "x", ".."), scope, page); err != nil {
		t.Fatalf("raiz com .. limpável deveria passar: %v", err)
	}
	// Raiz inexistente: EvalSymlinks falha e o erro não é de confinamento.
	missing := filepath.Join(root, "nao-existe")
	if _, err := pathFor(missing, scope, page); err == nil {
		t.Fatal("raiz inexistente deveria falhar")
	}
}

func TestConfineZeroPagePathRefused(t *testing.T) {
	root, scope, _ := confineFixture(t)

	_, err := pathFor(root, scope, domain.PagePath{})
	if !errors.Is(err, domain.ErrInvalidPagePath) {
		t.Fatalf("PagePath zero: erro = %v, quer ErrInvalidPagePath", err)
	}
}

func TestConfineEnsureWithinRefusesEscape(t *testing.T) {
	root, _, _ := confineFixture(t)

	cases := []string{
		filepath.Join(root, "..", "fora"),
		root,
		filepath.Join(filepath.Dir(root), "irmao", "a.md"),
	}
	for _, p := range cases {
		if err := ensureWithin(root, p); !errors.Is(err, ErrConfinement) {
			t.Fatalf("ensureWithin(%q): erro = %v, quer ErrConfinement", p, err)
		}
	}
	if err := ensureWithin(root, filepath.Join(root, "ws", "proj", "a.md")); err != nil {
		t.Fatalf("caminho dentro da raiz recusado: %v", err)
	}
}

func TestConfineHasReparseAttrOnRegularFileIsFalse(t *testing.T) {
	f := filepath.Join(t.TempDir(), "a.md")
	if err := os.WriteFile(f, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	fi, err := os.Lstat(f)
	if err != nil {
		t.Fatal(err)
	}
	if hasReparseAttr(fi) {
		t.Fatal("arquivo comum com atributo de reparse")
	}
}

func TestConfineJunctionIsReparseOnWindows(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("junção é específica do Windows")
	}
	base := t.TempDir()
	target := filepath.Join(base, "alvo")
	mustMkdir(t, target)
	junction := filepath.Join(base, "junc")
	makeLink(t, target, junction)

	fi, err := os.Lstat(junction)
	if err != nil {
		t.Fatal(err)
	}
	if !hasReparseAttr(fi) {
		t.Fatal("junção sem FILE_ATTRIBUTE_REPARSE_POINT em Lstat")
	}
}

func mustMkdir(t *testing.T, p string) {
	t.Helper()
	if err := os.MkdirAll(p, 0o755); err != nil {
		t.Fatalf("MkdirAll(%s): %v", p, err)
	}
}
