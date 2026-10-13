package cli

import (
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"testing"
)

func writeLiveGuardPIDFile(t *testing.T, dir, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, PIDFileName), []byte(content), 0o600); err != nil {
		t.Fatalf("gravar marcador de PID: %v", err)
	}
}

func TestLiveGuard_SemArquivoDePID(t *testing.T) {
	if err := CheckNoLiveProcess(t.TempDir()); err != nil {
		t.Fatalf("sem marcador esperava nil, veio %v", err)
	}
}

func TestLiveGuard_PIDDeProcessoInexistente(t *testing.T) {
	const pid = 999999
	if alive, err := processAlive(pid); err == nil && alive {
		t.Skipf("pid %d existe nesta máquina; teste não se aplica", pid)
	}
	dir := t.TempDir()
	writeLiveGuardPIDFile(t, dir, strconv.Itoa(pid)+"\n")
	if err := CheckNoLiveProcess(dir); err != nil {
		t.Fatalf("PID inexistente esperava nil, veio %v", err)
	}
}

func TestLiveGuard_PIDDoProprioProcesso(t *testing.T) {
	dir := t.TempDir()
	writeLiveGuardPIDFile(t, dir, strconv.Itoa(os.Getpid()))
	err := CheckNoLiveProcess(dir)
	if !errors.Is(err, ErrProcessLive) {
		t.Fatalf("PID do próprio processo esperava ErrProcessLive, veio %v", err)
	}
}

func TestLiveGuard_PIDCorrompido(t *testing.T) {
	for _, conteudo := range []string{"abc", "", "0", "-5", "12x", "99999999999"} {
		dir := t.TempDir()
		writeLiveGuardPIDFile(t, dir, conteudo)
		err := CheckNoLiveProcess(dir)
		if !errors.Is(err, ErrPIDFileFormat) {
			t.Errorf("conteúdo %q: esperava ErrPIDFileFormat, veio %v", conteudo, err)
		}
		if errors.Is(err, ErrProcessLive) {
			t.Errorf("conteúdo %q: não deveria ser ErrProcessLive", conteudo)
		}
	}
}
