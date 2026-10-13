package wiki

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"testing"
	"time"
)

// listNames devolve os nomes de entradas do diretório, para checar temporários.
func listNames(t *testing.T, dir string) []string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("ReadDir(%s): %v", dir, err)
	}
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		names = append(names, e.Name())
	}
	return names
}

func TestAtomicWriteCreatesFileWithExactContent(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "pagina.md")
	want := []byte("---\ntitle: teste\n---\n\nCorpo com acentos: ção\r\n")

	if _, err := Write(path, want); err != nil {
		t.Fatalf("Write: %v", err)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("conteúdo diferente: got %q, want %q", got, want)
	}
}

func TestAtomicWriteOverwritesContent(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "pagina.md")

	if _, err := Write(path, []byte("versão longa original que será substituída")); err != nil {
		t.Fatalf("Write 1: %v", err)
	}
	want := []byte("curta")
	if _, err := Write(path, want); err != nil {
		t.Fatalf("Write 2: %v", err)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("sobrescrita não substituiu: got %q, want %q", got, want)
	}
}

func TestAtomicWriteLeavesNoTempFileAfterSuccess(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "pagina.md")

	for _, body := range []string{"um", "dois"} {
		if _, err := Write(path, []byte(body)); err != nil {
			t.Fatalf("Write(%q): %v", body, err)
		}
	}
	names := listNames(t, dir)
	if len(names) != 1 || names[0] != "pagina.md" {
		t.Fatalf("esperava só pagina.md no diretório, got %v", names)
	}
	for _, n := range names {
		if strings.HasPrefix(n, tmpPrefix) {
			t.Fatalf("temporário sobrou: %s", n)
		}
	}
}

func TestAtomicWriteFailureLeavesNoTempFile(t *testing.T) {
	t.Run("diretorio inexistente", func(t *testing.T) {
		missing := filepath.Join(t.TempDir(), "nao-existe")
		id, err := Write(filepath.Join(missing, "pagina.md"), []byte("x"))
		if err == nil {
			t.Fatal("esperava erro com diretório inexistente")
		}
		if id != 0 {
			t.Fatalf("id em falha deveria ser 0, got %d", id)
		}
		if _, statErr := os.Stat(missing); !errors.Is(statErr, os.ErrNotExist) {
			t.Fatalf("diretório não deveria ser criado: %v", statErr)
		}
	})

	t.Run("rename falha depois do temporario", func(t *testing.T) {
		// Destino é um diretório não vazio: o temporário é criado e o rename falha.
		dir := t.TempDir()
		dest := filepath.Join(dir, "ocupado.md")
		if err := os.Mkdir(dest, 0o755); err != nil {
			t.Fatalf("Mkdir: %v", err)
		}
		if err := os.WriteFile(filepath.Join(dest, "filho.txt"), []byte("f"), 0o644); err != nil {
			t.Fatalf("WriteFile: %v", err)
		}

		if _, err := Write(dest, []byte("conteudo")); err == nil {
			t.Fatal("esperava erro ao instalar sobre diretório não vazio")
		}
		for _, n := range listNames(t, dir) {
			if strings.HasPrefix(n, tmpPrefix) {
				t.Fatalf("temporário sobrou após falha: %s", n)
			}
		}
	})
}

func TestAtomicWriteReturnsIDOfInstalledFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "pagina.md")

	id1, err := Write(path, []byte("primeiro conteudo"))
	if err != nil {
		t.Fatalf("Write 1: %v", err)
	}
	if want := idOfPath(t, path); id1 != want {
		t.Fatalf("id devolvido %d difere do arquivo instalado %d", id1, want)
	}
	if runtime.GOOS != "windows" && id1 == 0 {
		t.Fatal("no Unix o inode não deveria ser 0")
	}

	// Cada escrita instala um arquivo novo, então o id deve bater com o
	// arquivo atual a cada escrita (não necessariamente com o anterior).
	id2, err := Write(path, []byte("segundo conteudo, diferente"))
	if err != nil {
		t.Fatalf("Write 2: %v", err)
	}
	if want := idOfPath(t, path); id2 != want {
		t.Fatalf("id 2 devolvido %d difere do arquivo instalado %d", id2, want)
	}
	t.Logf("id1=%d id2=%d (iguais: %v)", id1, id2, id1 == id2)
}

func idOfPath(t *testing.T, path string) uint64 {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatalf("Open(%s): %v", path, err)
	}
	defer func() { _ = f.Close() }()
	id, err := fileID(f)
	if err != nil {
		t.Fatalf("fileID: %v", err)
	}
	return id
}

func TestAtomicWriteModeIs0644(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("permissão Unix não se aplica no Windows")
	}
	path := filepath.Join(t.TempDir(), "pagina.md")
	if _, err := Write(path, []byte("x")); err != nil {
		t.Fatalf("Write: %v", err)
	}
	fi, err := os.Stat(path)
	if err != nil {
		t.Fatalf("Stat: %v", err)
	}
	if perm := fi.Mode().Perm(); perm != 0o644 {
		t.Fatalf("permissão %o, want 644", perm)
	}
}

func TestAtomicTempNameHasPrefix(t *testing.T) {
	if !strings.HasPrefix(tmpPrefix, ".ai-memory-tmp.") {
		t.Fatalf("prefixo errado: %q", tmpPrefix)
	}
	dir := t.TempDir()
	f, err := createTemp(dir)
	if err != nil {
		t.Fatalf("createTemp: %v", err)
	}
	name := f.Name()
	_ = f.Close()
	defer func() { _ = os.Remove(name) }()

	if base := filepath.Base(name); !strings.HasPrefix(base, ".ai-memory-tmp.") {
		t.Fatalf("nome do temporário sem prefixo: %s", base)
	}
	if filepath.Dir(name) != filepath.Clean(dir) {
		t.Fatalf("temporário fora do diretório do destino: %s", name)
	}
}

func TestAtomicRetryDelaysAndSharingViolation(t *testing.T) {
	want := []time.Duration{10, 20, 30, 40, 50}
	if len(retryDelays) != len(want) {
		t.Fatalf("len(retryDelays)=%d, want %d", len(retryDelays), len(want))
	}
	for i, w := range want {
		if retryDelays[i] != w*time.Millisecond {
			t.Fatalf("retryDelays[%d]=%v, want %v", i, retryDelays[i], w*time.Millisecond)
		}
	}

	cases := []struct {
		name string
		err  error
		want bool
	}{
		{"nil", nil, false},
		{"sharing 32", syscall.Errno(32), true},
		{"lock 33", syscall.Errno(33), true},
		{"access denied 5", syscall.Errno(5), false},
		{"erro comum", errors.New("x"), false},
		{"envolto em LinkError 32", &os.LinkError{Op: "rename", Old: "a", New: "b", Err: syscall.Errno(32)}, true},
	}
	for _, c := range cases {
		if got := isSharingViolation(c.err); got != c.want {
			t.Errorf("%s: isSharingViolation=%v, want %v", c.name, got, c.want)
		}
	}
}
