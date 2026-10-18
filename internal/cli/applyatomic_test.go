package cli

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"
)

// constTransform devolve sempre next, ignorando o conteúdo atual.
func constTransform(next string) func([]byte) ([]byte, error) {
	return func([]byte) ([]byte, error) { return []byte(next), nil }
}

// bakFiles lista os arquivos `<base>.bak-*` no diretório dir.
func bakFiles(t *testing.T, dir, base string) []string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("ReadDir(%s): %v", dir, err)
	}
	re := regexp.MustCompile(`^` + regexp.QuoteMeta(base) + `\.bak-\d+$`)
	var out []string
	for _, e := range entries {
		if re.MatchString(e.Name()) {
			out = append(out, e.Name())
		}
	}
	return out
}

// onlyNames devolve os nomes do diretório, para checar que não sobrou temporário.
func onlyNames(t *testing.T, dir string) []string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("ReadDir(%s): %v", dir, err)
	}
	var names []string
	for _, e := range entries {
		names = append(names, e.Name())
	}
	return names
}

// makeSymlinkOrSkip cria link -> target ou faz skip com mensagem clara quando
// a plataforma não permite (no Windows, exige modo de desenvolvedor ou privilégio).
func makeSymlinkOrSkip(t *testing.T, target, link string) {
	t.Helper()
	if err := os.Symlink(target, link); err != nil {
		t.Skipf("criação de link simbólico indisponível nesta plataforma (no Windows exige modo de desenvolvedor ou privilégio de administrador): %v", err)
	}
}

func TestApplyAtomicNoOpDoesNotTouchDisk(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "AGENT.md")
	if err := os.WriteFile(path, []byte("same"), 0o644); err != nil {
		t.Fatal(err)
	}
	past := time.Date(2000, 1, 2, 3, 4, 5, 0, time.UTC)
	if err := os.Chtimes(path, past, past); err != nil {
		t.Fatal(err)
	}
	before := onlyNames(t, dir)

	status, err := ApplyAtomic(path, constTransform("same"))
	if err != nil {
		t.Fatalf("ApplyAtomic: %v", err)
	}
	if status != ApplyNoOp {
		t.Fatalf("status = %v, quer NoOp", status)
	}
	fi, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if !fi.ModTime().Equal(past) {
		t.Errorf("mtime mudou: %v, quer %v", fi.ModTime(), past)
	}
	if got := onlyNames(t, dir); strings.Join(got, ",") != strings.Join(before, ",") {
		t.Errorf("entradas do diretório mudaram: antes %v, depois %v", before, got)
	}
	if n := len(bakFiles(t, dir, "AGENT.md")); n != 0 {
		t.Errorf("NoOp criou %d backup(s)", n)
	}
}

func TestApplyAtomicDifferentContentBacksUpAndReplaces(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "AGENT.md")
	if err := os.WriteFile(path, []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}

	fixed := int64(1700000000)
	orig := nowUnix
	nowUnix = func() int64 { return fixed }
	t.Cleanup(func() { nowUnix = orig })

	status, err := ApplyAtomic(path, constTransform("new"))
	if err != nil {
		t.Fatalf("ApplyAtomic: %v", err)
	}
	if status != ApplyWritten {
		t.Fatalf("status = %v, quer Written", status)
	}

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "new" {
		t.Errorf("conteúdo = %q, quer %q", got, "new")
	}

	bakName := "AGENT.md.bak-" + strconv.FormatInt(fixed, 10)
	bak, err := os.ReadFile(filepath.Join(dir, bakName))
	if err != nil {
		t.Fatalf("backup %s ausente: %v", bakName, err)
	}
	if string(bak) != "old" {
		t.Errorf("conteúdo do backup = %q, quer %q", bak, "old")
	}

	// Nenhum temporário de escrita pode sobrar no diretório.
	for _, name := range onlyNames(t, dir) {
		if name != "AGENT.md" && name != bakName {
			t.Errorf("arquivo inesperado no diretório: %s", name)
		}
	}
}

func TestApplyAtomicBackupUsesUnixSecondsNow(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "AGENT.md")
	if err := os.WriteFile(path, []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}
	lo := time.Now().Unix()
	if _, err := ApplyAtomic(path, constTransform("new")); err != nil {
		t.Fatalf("ApplyAtomic: %v", err)
	}
	hi := time.Now().Unix()

	baks := bakFiles(t, dir, "AGENT.md")
	if len(baks) != 1 {
		t.Fatalf("backups = %v, quer exatamente 1", baks)
	}
	suffix := strings.TrimPrefix(baks[0], "AGENT.md.bak-")
	ts, err := strconv.ParseInt(suffix, 10, 64)
	if err != nil {
		t.Fatalf("sufixo %q não é inteiro Unix: %v", suffix, err)
	}
	if ts < lo || ts > hi {
		t.Errorf("timestamp do backup %d fora do intervalo [%d, %d] em segundos", ts, lo, hi)
	}
}

func TestApplyAtomicMissingTargetIsCreatedWithoutBackup(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "AGENT.md")

	status, err := ApplyAtomic(path, constTransform("fresh"))
	if err != nil {
		t.Fatalf("ApplyAtomic: %v", err)
	}
	if status != ApplyWritten {
		t.Fatalf("status = %v, quer Written", status)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "fresh" {
		t.Errorf("conteúdo = %q, quer %q", got, "fresh")
	}
	if n := len(bakFiles(t, dir, "AGENT.md")); n != 0 {
		t.Errorf("arquivo novo gerou %d backup(s), quer 0", n)
	}
}

func TestApplyAtomicTransformSeesCurrentContent(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "AGENT.md")
	if err := os.WriteFile(path, []byte("abc"), 0o644); err != nil {
		t.Fatal(err)
	}
	var seen string
	status, err := ApplyAtomic(path, func(cur []byte) ([]byte, error) {
		seen = string(cur)
		return append(cur, "d"...), nil
	})
	if err != nil {
		t.Fatalf("ApplyAtomic: %v", err)
	}
	if seen != "abc" {
		t.Errorf("transform viu %q, quer %q", seen, "abc")
	}
	if status != ApplyWritten {
		t.Fatalf("status = %v, quer Written", status)
	}
}

func TestApplyAtomicTransformErrorChangesNothing(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "AGENT.md")
	if err := os.WriteFile(path, []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}
	sentinel := errors.New("falha de transformação")
	_, err := ApplyAtomic(path, func([]byte) ([]byte, error) { return nil, sentinel })
	if !errors.Is(err, sentinel) {
		t.Fatalf("err = %v, quer encapsular %v", err, sentinel)
	}
	got, _ := os.ReadFile(path)
	if string(got) != "old" {
		t.Errorf("conteúdo mudou para %q após erro de transformação", got)
	}
	if n := len(bakFiles(t, dir, "AGENT.md")); n != 0 {
		t.Errorf("erro de transformação criou %d backup(s)", n)
	}
}

func TestApplyAtomicSymlinkWritesTargetAndKeepsLink(t *testing.T) {
	dir := t.TempDir()
	realDir := filepath.Join(dir, "dotfiles")
	if err := os.Mkdir(realDir, 0o755); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(realDir, "settings.json")
	if err := os.WriteFile(target, []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}
	homeDir := filepath.Join(dir, "home")
	if err := os.Mkdir(homeDir, 0o755); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(homeDir, "settings.json")
	makeSymlinkOrSkip(t, target, link)

	status, err := ApplyAtomic(link, constTransform("new"))
	if err != nil {
		t.Fatalf("ApplyAtomic: %v", err)
	}
	if status != ApplyWritten {
		t.Fatalf("status = %v, quer Written", status)
	}

	li, err := os.Lstat(link)
	if err != nil {
		t.Fatal(err)
	}
	if li.Mode()&os.ModeSymlink == 0 {
		t.Errorf("o link foi substituído por arquivo comum")
	}
	got, err := os.ReadFile(target)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "new" {
		t.Errorf("alvo = %q, quer %q", got, "new")
	}
	// O backup fica ao lado do caminho visível (o link), não ao lado do alvo.
	if n := len(bakFiles(t, homeDir, "settings.json")); n != 1 {
		t.Errorf("backups ao lado do link = %d, quer 1", n)
	}
	if n := len(bakFiles(t, realDir, "settings.json")); n != 0 {
		t.Errorf("backup apareceu ao lado do alvo = %d, quer 0", n)
	}
}

func TestApplyAtomicSymlinkChainIsFollowed(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "real.md")
	if err := os.WriteFile(target, []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}
	mid := filepath.Join(dir, "mid.md")
	makeSymlinkOrSkip(t, target, mid)
	link := filepath.Join(dir, "AGENT.md")
	// Link relativo ao diretório do próprio link.
	if err := os.Symlink("mid.md", link); err != nil {
		t.Skipf("link relativo indisponível: %v", err)
	}

	if _, err := ApplyAtomic(link, constTransform("new")); err != nil {
		t.Fatalf("ApplyAtomic: %v", err)
	}
	got, _ := os.ReadFile(target)
	if string(got) != "new" {
		t.Errorf("alvo final = %q, quer %q", got, "new")
	}
	if fi, _ := os.Lstat(link); fi.Mode()&os.ModeSymlink == 0 {
		t.Errorf("link da cadeia foi substituído")
	}
}

func TestApplyAtomicSymlinkToMissingTargetCreatesIt(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "dotfiles", "AGENT.md")
	if err := os.Mkdir(filepath.Dir(target), 0o755); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(dir, "AGENT.md.link")
	makeSymlinkOrSkip(t, target, link)

	status, err := ApplyAtomic(link, constTransform("created"))
	if err != nil {
		t.Fatalf("ApplyAtomic: %v", err)
	}
	if status != ApplyWritten {
		t.Fatalf("status = %v, quer Written", status)
	}
	got, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("alvo inexistente não foi criado: %v", err)
	}
	if string(got) != "created" {
		t.Errorf("alvo = %q, quer %q", got, "created")
	}
	if fi, _ := os.Lstat(link); fi.Mode()&os.ModeSymlink == 0 {
		t.Errorf("link foi substituído")
	}
	if n := len(bakFiles(t, dir, "AGENT.md.link")); n != 0 {
		t.Errorf("alvo inexistente gerou %d backup(s), quer 0", n)
	}
}

func TestApplyAtomicSymlinkChainTooDeepIsRefused(t *testing.T) {
	dir := t.TempDir()
	// Cadeia de 41 links: estoura o limite de 40 saltos.
	prev := filepath.Join(dir, "end.md")
	if err := os.WriteFile(prev, []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 41; i++ {
		cur := filepath.Join(dir, fmt.Sprintf("hop%02d.md", i))
		makeSymlinkOrSkip(t, prev, cur)
		prev = cur
	}
	_, err := ApplyAtomic(prev, constTransform("new"))
	if !errors.Is(err, ErrSymlinkChainTooDeep) {
		t.Fatalf("err = %v, quer ErrSymlinkChainTooDeep", err)
	}
	got, _ := os.ReadFile(filepath.Join(dir, "end.md"))
	if string(got) != "old" {
		t.Errorf("alvo mudou para %q apesar da recusa", got)
	}
}

func TestApplyAtomicMissingDirectoryReturnsError(t *testing.T) {
	dir := t.TempDir()
	missing := filepath.Join(dir, "no-such-dir", "AGENT.md")

	status, err := ApplyAtomic(missing, constTransform("x"))
	if err == nil {
		t.Fatalf("status = %v, sem erro; quer erro para diretório pai inexistente", status)
	}
	if _, statErr := os.Stat(filepath.Join(dir, "no-such-dir")); !errors.Is(statErr, os.ErrNotExist) {
		t.Errorf("o diretório pai foi criado: stat err = %v", statErr)
	}
}

func TestApplyAtomicBackupCollisionIsRefused(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "AGENT.md")
	if err := os.WriteFile(path, []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}
	fixed := int64(1700000001)
	orig := nowUnix
	nowUnix = func() int64 { return fixed }
	t.Cleanup(func() { nowUnix = orig })

	existing := filepath.Join(dir, "AGENT.md.bak-"+strconv.FormatInt(fixed, 10))
	if err := os.WriteFile(existing, []byte("earlier backup"), 0o644); err != nil {
		t.Fatal(err)
	}

	_, err := ApplyAtomic(path, constTransform("new"))
	if !errors.Is(err, ErrBackupExists) {
		t.Fatalf("err = %v, quer ErrBackupExists", err)
	}
	got, _ := os.ReadFile(existing)
	if string(got) != "earlier backup" {
		t.Errorf("backup existente foi sobrescrito: %q", got)
	}
	got, _ = os.ReadFile(path)
	if string(got) != "old" {
		t.Errorf("alvo mudou para %q apesar do erro de backup", got)
	}
}
