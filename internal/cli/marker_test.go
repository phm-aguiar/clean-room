package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// writeMarkerFile grava um .ai-memory.toml em dir, criando dir se preciso.
func writeMarkerFile(t *testing.T, dir, content string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, markerFileName), []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

// mkdirAllT cria dir e devolve o próprio caminho.
func mkdirAllT(t *testing.T, dir string) string {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	return dir
}

func TestMarkerDirsStopsAtHomeInclusive(t *testing.T) {
	base := t.TempDir()
	home := mkdirAllT(t, filepath.Join(base, "home"))
	cwd := mkdirAllT(t, filepath.Join(home, "a", "b"))

	dirs, err := markerDirs(cwd, home)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{cwd, filepath.Join(home, "a"), home}
	if len(dirs) != len(want) {
		t.Fatalf("markerDirs = %v, quer %v", dirs, want)
	}
	for i := range want {
		if dirs[i] != want[i] {
			t.Fatalf("dirs[%d] = %q, quer %q", i, dirs[i], want[i])
		}
	}
}

func TestMarkerWalkInsideHomeIgnoresMarkerAboveHome(t *testing.T) {
	base := t.TempDir()
	home := mkdirAllT(t, filepath.Join(base, "home"))
	cwd := mkdirAllT(t, filepath.Join(home, "a"))
	writeMarkerFile(t, base, "workspace = \"acima-do-home\"\n")

	if _, ok, err := readScopeMarker(cwd, home); err != nil || ok {
		t.Fatalf("marcador acima do home não deve ser lido: ok=%v err=%v", ok, err)
	}

	writeMarkerFile(t, home, "workspace = \"pessoal\"\n")
	m, ok, err := readScopeMarker(cwd, home)
	if err != nil || !ok {
		t.Fatalf("marcador no home deve ser lido: ok=%v err=%v", ok, err)
	}
	if want := filepath.Join(home, markerFileName); m.path != want {
		t.Fatalf("path = %q, quer %q", m.path, want)
	}
}

func TestMarkerWalkOutsideHomeStopsAtCheckoutRoot(t *testing.T) {
	base := t.TempDir()
	home := mkdirAllT(t, filepath.Join(base, "home"))
	work := filepath.Join(base, "work")
	repo := mkdirAllT(t, filepath.Join(work, "repo"))
	mkdirAllT(t, filepath.Join(repo, ".git"))
	cwd := mkdirAllT(t, filepath.Join(repo, "src", "pkg"))

	dirs, err := markerDirs(cwd, home)
	if err != nil {
		t.Fatal(err)
	}
	if len(dirs) != 3 || dirs[len(dirs)-1] != repo {
		t.Fatalf("a caminhada deve parar na raiz do checkout (%q): %v", repo, dirs)
	}

	writeMarkerFile(t, work, "workspace = \"acima-do-checkout\"\n")
	if _, ok, err := readScopeMarker(cwd, home); err != nil || ok {
		t.Fatalf("marcador acima da raiz do checkout não deve ser lido: ok=%v err=%v", ok, err)
	}

	writeMarkerFile(t, repo, "workspace = \"cliente\"\n")
	m, ok, err := readScopeMarker(cwd, home)
	if err != nil || !ok {
		t.Fatalf("marcador na raiz do checkout deve ser lido: ok=%v err=%v", ok, err)
	}
	if want := filepath.Join(repo, markerFileName); m.path != want {
		t.Fatalf("path = %q, quer %q", m.path, want)
	}
}

func TestMarkerWalkGitFileCountsAsCheckoutRoot(t *testing.T) {
	base := t.TempDir()
	home := mkdirAllT(t, filepath.Join(base, "home"))
	wt := mkdirAllT(t, filepath.Join(base, "work", "worktree"))
	if err := os.WriteFile(filepath.Join(wt, ".git"), []byte("gitdir: x\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	cwd := mkdirAllT(t, filepath.Join(wt, "sub"))

	dirs, err := markerDirs(cwd, home)
	if err != nil {
		t.Fatal(err)
	}
	if len(dirs) != 2 || dirs[1] != wt {
		t.Fatalf("entrada .git arquivo deve marcar a raiz: %v", dirs)
	}
}

func TestMarkerWalkOutsideHomeWithoutCheckoutStopsAtCwd(t *testing.T) {
	base := t.TempDir()
	home := mkdirAllT(t, filepath.Join(base, "home"))
	loose := filepath.Join(base, "loose")
	cwd := mkdirAllT(t, filepath.Join(loose, "inner"))
	writeMarkerFile(t, loose, "workspace = \"pai\"\n")

	dirs, err := markerDirs(cwd, home)
	if err != nil {
		t.Fatal(err)
	}
	if len(dirs) != 1 || dirs[0] != cwd {
		t.Fatalf("sem checkout fora do home, a caminhada é só o cwd: %v", dirs)
	}
	if _, ok, err := readScopeMarker(cwd, home); err != nil || ok {
		t.Fatalf("marcador do pai não deve ser lido: ok=%v err=%v", ok, err)
	}
}

func TestMarkerWalkHomeUnsetGoesToFilesystemRoot(t *testing.T) {
	base := t.TempDir()
	repo := filepath.Join(base, "repo")
	mkdirAllT(t, filepath.Join(repo, ".git"))
	cwd := mkdirAllT(t, filepath.Join(repo, "sub"))

	dirs, err := markerDirs(cwd, "")
	if err != nil {
		t.Fatal(err)
	}
	root := filepath.VolumeName(cwd) + string(filepath.Separator)
	if dirs[len(dirs)-1] != root {
		t.Fatalf("home ausente: a caminhada deve ir até %q, foi até %q", root, dirs[len(dirs)-1])
	}

	writeMarkerFile(t, base, "workspace = \"acima-do-checkout\"\n")
	m, ok, err := readScopeMarker(cwd, "")
	if err != nil || !ok {
		t.Fatalf("sem home, o marcador acima do checkout deve ser lido: ok=%v err=%v", ok, err)
	}
	if want := filepath.Join(base, markerFileName); m.path != want {
		t.Fatalf("path = %q, quer %q", m.path, want)
	}
}

func TestMarkerWalkSkipsDirectoryNamedMarker(t *testing.T) {
	base := t.TempDir()
	home := mkdirAllT(t, filepath.Join(base, "home"))
	cwd := mkdirAllT(t, filepath.Join(home, "proj"))
	mkdirAllT(t, filepath.Join(cwd, markerFileName))
	writeMarkerFile(t, home, "workspace = \"home\"\n")

	m, ok, err := readScopeMarker(cwd, home)
	if err != nil || !ok {
		t.Fatalf("diretório com nome de marcador deve ser pulado: ok=%v err=%v", ok, err)
	}
	if want := filepath.Join(home, markerFileName); m.path != want {
		t.Fatalf("path = %q, quer %q", m.path, want)
	}
}

func TestScopeWalkSkipsCaptureOnlyMarkerButCaptureWalkDoesNot(t *testing.T) {
	base := t.TempDir()
	home := mkdirAllT(t, filepath.Join(base, "home"))
	proj := mkdirAllT(t, filepath.Join(home, "proj"))
	cwd := mkdirAllT(t, filepath.Join(proj, "sub"))
	writeMarkerFile(t, home, "workspace = \"pessoal\"\n[capture]\nignore_paths = [\"home/**\"]\n")
	writeMarkerFile(t, proj, "[capture]\nignore_paths = [\"private/**\"]\n")

	m, ok, err := readScopeMarker(cwd, home)
	if err != nil || !ok {
		t.Fatalf("escopo deve vir do home: ok=%v err=%v", ok, err)
	}
	if ws, found := markerString(m.data, "workspace"); !found || ws != "pessoal" {
		t.Fatalf("workspace = %q (%v), quer \"pessoal\"", ws, found)
	}

	pol := loadCapturePolicy(cwd, home)
	if pol.state != captureActive || len(pol.ignorePaths) != 1 || pol.ignorePaths[0] != "private/**" {
		t.Fatalf("captura deve vir do marcador mais próximo (só [capture]): %+v", pol)
	}
}

func TestCaptureWalkNearestMarkerReplacesOuterRules(t *testing.T) {
	base := t.TempDir()
	home := mkdirAllT(t, filepath.Join(base, "home"))
	proj := mkdirAllT(t, filepath.Join(home, "proj"))
	cwd := mkdirAllT(t, filepath.Join(proj, "sub"))
	writeMarkerFile(t, home, "[capture]\nignore_paths = [\"home/**\"]\n")
	writeMarkerFile(t, proj, "workspace = \"p\"\n")

	pol := loadCapturePolicy(cwd, home)
	if pol.state != captureInactive {
		t.Fatalf("marcador mais próximo sem [capture] deve tornar a política inativa, não herdar: %+v", pol)
	}
}

func TestCaptureWalkNoMarkerIsInactive(t *testing.T) {
	base := t.TempDir()
	home := mkdirAllT(t, filepath.Join(base, "home"))
	cwd := mkdirAllT(t, filepath.Join(home, "x"))
	if pol := loadCapturePolicy(cwd, home); pol.state != captureInactive {
		t.Fatalf("sem marcador a política é inativa: %+v", pol)
	}
}

func TestMarkerStringLineRules(t *testing.T) {
	cases := []struct {
		name    string
		content string
		want    string
		ok      bool
	}{
		{"valor entre aspas", `workspace = "acme"`, "acme", true},
		{"sem espaços em volta do =", `workspace="acme"`, "acme", true},
		{"comentário inline depois do valor", `workspace = "acme" # cliente`, "acme", true},
		{"CRLF", "workspace = \"acme\"\r\n", "acme", true},
		{"valor nu não é lido", `workspace = acme`, "", false},
		{"aspa sem fechamento não é lida", `workspace = "acme`, "", false},
		{"linha comentada não casa", `# workspace = "acme"`, "", false},
		{"prefixo de outra chave não casa", `workspace_extra = "x"`, "", false},
		{"linha indentada não casa (regra literal de início de linha)", `  workspace = "acme"`, "", false},
		{"cabeçalho de seção é ignorado", "[capture]\nworkspace = \"acme\"", "acme", true},
		{"primeira linha que casa vence", "workspace = \"a\"\nworkspace = \"b\"", "a", true},
		{"primeira linha que casa vence mesmo sem valor legível", "workspace = a\nworkspace = \"b\"", "", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, ok := markerString([]byte(c.content), "workspace")
			if ok != c.ok || got != c.want {
				t.Fatalf("markerString(%q) = (%q, %v), quer (%q, %v)", c.content, got, ok, c.want, c.ok)
			}
		})
	}
}

func TestMarkerFlagTruthyAndFalsy(t *testing.T) {
	truthy := []string{"1", "true", "yes", "on", "TRUE", "Yes", "ON", `"true"`, `"On"`, "true # comentário", "  yes  "}
	falsy := []string{"0", "false", "no", "off", "FALSE", "No", "OFF", `"false"`, `"Off"`, "0 # comentário"}
	unknown := []string{"maybe", `"`, "", `""`, "2"}

	key := "default_global"
	for _, v := range truthy {
		content := "[recall]\n" + key + " = " + v + "\n"
		got, ok := markerFlag([]byte(content), key)
		if !ok || !got {
			t.Errorf("valor %q: quer verdadeiro, veio (%v, %v)", v, got, ok)
		}
	}
	for _, v := range falsy {
		content := key + " = " + v + "\n"
		got, ok := markerFlag([]byte(content), key)
		if !ok || got {
			t.Errorf("valor %q: quer falso, veio (%v, %v)", v, got, ok)
		}
	}
	for _, v := range unknown {
		content := key + " = " + v + "\n"
		if _, ok := markerFlag([]byte(content), key); ok {
			t.Errorf("valor %q não deve ser lido como flag", v)
		}
	}
	if _, ok := markerFlag([]byte("otra = true\n"), key); ok {
		t.Errorf("chave ausente não deve ser lida")
	}
}

func TestMarkerDeclaresScope(t *testing.T) {
	cases := []struct {
		name    string
		content string
		want    bool
	}{
		{"só [capture] não declara", "[capture]\nignore_paths = [\"a\"]\n", false},
		{"vazio não declara", "", false},
		{"chave de escopo sob seção declara", "[briefing]\nmax_chars = 100\n", true},
		{"project_strategy declara", "project_strategy = \"repo-root\"\n", true},
		{"flag contribute declara", "[profile]\ncontribute = off\n", true},
		{"chave comentada não declara", "# workspace = \"x\"\n", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := markerDeclaresScope([]byte(c.content)); got != c.want {
				t.Fatalf("markerDeclaresScope(%q) = %v, quer %v", c.content, got, c.want)
			}
		})
	}
}

func TestCaptureBlockStrictValidation(t *testing.T) {
	cases := []struct {
		name    string
		content string
		state   captureState
	}{
		{"sem marcador de [capture] é inativo", "workspace = \"x\"\n", captureInactive},
		{"arquivo vazio é inativo", "", captureInactive},
		{"[capture] vazio é inativo", "[capture]\n", captureInactive},
		{"ignore_paths vazio é inativo", "[capture]\nignore_paths = []\n", captureInactive},
		{"ignore_paths com padrões é ativo", "[capture]\nignore_paths = [\"private/**\", \"~/notes/**\"]\n", captureActive},
		{"chave desconhecida torna inválido", "[capture]\nignore_paths = [\"a\"]\nextra = true\n", captureInvalid},
		{"subtabela de capture torna inválido", "[capture]\nignore_paths = [\"a\"]\n[capture.sub]\nx = 1\n", captureInvalid},
		{"capture que não é tabela torna inválido", "capture = \"x\"\n", captureInvalid},
		{"ignore_paths que não é array torna inválido", "[capture]\nignore_paths = \"private/**\"\n", captureInvalid},
		{"entrada que não é string torna inválido", "[capture]\nignore_paths = [\"a\", 1]\n", captureInvalid},
		{"TOML quebrado torna inválido", "[capture]\nignore_paths = [\"a\"\n", captureInvalid},
		{"TOML quebrado fora de [capture] também torna inválido", "workspace = \"x\"\nisto não é toml\n", captureInvalid},
		{"não UTF-8 torna inválido", "# \xff\n[capture]\nignore_paths = [\"a\"]\n", captureInvalid},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			pol := parseCaptureBlock([]byte(c.content))
			if pol.state != c.state {
				t.Fatalf("estado = %v, quer %v (%+v)", pol.state, c.state, pol)
			}
		})
	}
}

func TestCaptureBlockSizeLimit64KiB(t *testing.T) {
	prefix := "[capture]\nignore_paths = [\"private/**\"]\n# "

	exact := prefix + strings.Repeat("x", markerCaptureMaxBytes-len(prefix))
	if len(exact) != markerCaptureMaxBytes {
		t.Fatalf("setup: tamanho %d, quer %d", len(exact), markerCaptureMaxBytes)
	}
	if pol := parseCaptureBlock([]byte(exact)); pol.state != captureActive {
		t.Fatalf("exatamente 64 KiB deve ser lido: %+v", pol.state)
	}

	over := exact + "x"
	if pol := parseCaptureBlock([]byte(over)); pol.state != captureInvalid {
		t.Fatalf("acima de 64 KiB deve tornar inválido: %v", pol.state)
	}

	dir := mkdirAllT(t, filepath.Join(t.TempDir(), "big"))
	path := filepath.Join(dir, markerFileName)
	if err := os.WriteFile(path, []byte(over), 0o600); err != nil {
		t.Fatal(err)
	}
	if pol := readCaptureBlock(path); pol.state != captureInvalid {
		t.Fatalf("arquivo em disco acima de 64 KiB deve ser inválido: %v", pol.state)
	}
}

func TestCaptureWalkReadsFileFromDisk(t *testing.T) {
	base := t.TempDir()
	home := mkdirAllT(t, filepath.Join(base, "home"))
	cwd := mkdirAllT(t, filepath.Join(home, "repo"))
	writeMarkerFile(t, cwd, "[capture]\nignore_paths = [\"docs/adr/**\"]\n")

	pol := loadCapturePolicy(cwd, home)
	if pol.state != captureActive || pol.ignorePaths[0] != "docs/adr/**" {
		t.Fatalf("política do disco: %+v", pol)
	}
}
