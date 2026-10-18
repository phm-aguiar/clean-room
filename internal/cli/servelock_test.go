package cli

import (
	"bytes"
	"errors"
	"log/slog"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

// fakeServeOps troca as operações do SO por fakes até o fim do teste. lock nil mantém a
// trava real. A trava falsa não guarda nada, então o unlock também vira no-op.
func fakeServeOps(t *testing.T, open func(string) (*os.File, error), lock func(*os.File) error) {
	t.Helper()
	prevOpen, prevLock, prevUnlock := sysOpen, sysLock, sysUnlock
	if open != nil {
		sysOpen = open
	}
	if lock != nil {
		sysLock = lock
		sysUnlock = func(*os.File) error { return nil }
	}
	t.Cleanup(func() {
		sysOpen, sysLock, sysUnlock = prevOpen, prevLock, prevUnlock
	})
}

// osOpenServe abre o arquivo de trava como o código de produção.
func osOpenServe(path string) (*os.File, error) {
	return os.OpenFile(path, os.O_RDWR|os.O_CREATE, 0o600)
}

func newServeTestLogger(buf *bytes.Buffer) *slog.Logger {
	return slog.New(slog.NewTextHandler(buf, nil))
}

func TestServeLock_TravaRealExclusivaELiberada(t *testing.T) {
	dir := t.TempDir()
	log := newServeTestLogger(&bytes.Buffer{})

	a, err := AcquireServeLock(dir, false, log)
	if err != nil {
		t.Fatalf("primeira trava: %v", err)
	}
	if !a.Held() {
		t.Fatal("primeira trava deveria estar mantida")
	}

	_, err = AcquireServeLock(dir, false, log)
	if !errors.Is(err, ErrInstanceLocked) {
		t.Fatalf("segunda trava esperava ErrInstanceLocked, veio %v", err)
	}
	if !strings.Contains(err.Error(), strconv.Itoa(os.Getpid())) {
		t.Fatalf("recusa deve citar o PID do detentor %d: %v", os.Getpid(), err)
	}

	if err := a.Release(); err != nil {
		t.Fatalf("release: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, serveHolderFile)); !os.IsNotExist(err) {
		t.Fatalf("holder deveria ser removido na liberação, stat: %v", err)
	}

	c, err := AcquireServeLock(dir, false, log)
	if err != nil {
		t.Fatalf("após liberar, nova trava deveria passar: %v", err)
	}
	if err := c.Release(); err != nil {
		t.Fatalf("release da terceira trava: %v", err)
	}
}

func TestServeLock_ReleaseRemoveHolderSoSeForProprio(t *testing.T) {
	dir := t.TempDir()
	log := newServeTestLogger(&bytes.Buffer{})
	l, err := AcquireServeLock(dir, false, log)
	if err != nil {
		t.Fatalf("trava: %v", err)
	}
	holder := filepath.Join(dir, serveHolderFile)
	if err := os.WriteFile(holder, []byte("999999\n"), 0o600); err != nil {
		t.Fatalf("simular holder alheio: %v", err)
	}
	if err := l.Release(); err != nil {
		t.Fatalf("release: %v", err)
	}
	raw, err := os.ReadFile(holder)
	if err != nil {
		t.Fatalf("holder alheio deveria permanecer: %v", err)
	}
	if strings.TrimSpace(string(raw)) != "999999" {
		t.Fatalf("holder alheio alterado: %q", raw)
	}
}

func TestServeLock_ReleaseSemTravaNaoFaz(t *testing.T) {
	var nilLock *ServeLock
	if err := nilLock.Release(); err != nil {
		t.Fatalf("release de nil: %v", err)
	}
	if err := (&ServeLock{}).Release(); err != nil {
		t.Fatalf("release sem trava: %v", err)
	}
}

func TestServeLock_TravaOcupadaNaoERepetida(t *testing.T) {
	var calls int
	fakeServeOps(t, osOpenServe, func(*os.File) error {
		calls++
		return lockHeldErrnos[0]
	})
	_, err := AcquireServeLock(t.TempDir(), false, newServeTestLogger(&bytes.Buffer{}))
	if !errors.Is(err, ErrInstanceLocked) {
		t.Fatalf("esperava ErrInstanceLocked, veio %v", err)
	}
	if calls != 1 {
		t.Fatalf("trava ocupada não pode ser repetida: %d chamadas de trava", calls)
	}
}

func TestServeLock_RecusaCitaPIDDoHolder(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, serveHolderFile), []byte("4242\n"), 0o600); err != nil {
		t.Fatalf("holder: %v", err)
	}
	fakeServeOps(t, osOpenServe, func(*os.File) error { return lockHeldErrnos[0] })

	_, err := AcquireServeLock(dir, false, newServeTestLogger(&bytes.Buffer{}))
	if !errors.Is(err, ErrInstanceLocked) {
		t.Fatalf("esperava ErrInstanceLocked, veio %v", err)
	}
	if !strings.Contains(err.Error(), "4242") {
		t.Fatalf("recusa deve citar o pid 4242 do holder: %v", err)
	}
}

func TestServeLock_ForceSobeComAviso(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, serveHolderFile), []byte("4242\n"), 0o600); err != nil {
		t.Fatalf("holder: %v", err)
	}
	fakeServeOps(t, osOpenServe, func(*os.File) error { return lockHeldErrnos[0] })

	var buf bytes.Buffer
	l, err := AcquireServeLock(dir, true, newServeTestLogger(&buf))
	if err != nil {
		t.Fatalf("--force deveria subir, veio %v", err)
	}
	if l.Held() {
		t.Fatal("--force sobe sem trava")
	}
	if !strings.Contains(buf.String(), "--force") || !strings.Contains(buf.String(), "4242") {
		t.Fatalf("--force deve registrar aviso com o holder: %q", buf.String())
	}
	if err := l.Release(); err != nil {
		t.Fatalf("release após --force: %v", err)
	}
}

func TestServeLock_TransitoriaRepetidaAteCincoTentativas(t *testing.T) {
	var calls int
	fakeServeOps(t, osOpenServe, func(*os.File) error {
		calls++
		if calls < serveLockAttempts {
			return lockTransientErrnos[0]
		}
		return nil
	})

	start := time.Now()
	l, err := AcquireServeLock(t.TempDir(), false, newServeTestLogger(&bytes.Buffer{}))
	elapsed := time.Since(start)
	if err != nil {
		t.Fatalf("quinta tentativa deveria passar: %v", err)
	}
	if !l.Held() {
		t.Fatal("deveria ter a trava após retry")
	}
	if calls != serveLockAttempts {
		t.Fatalf("esperava %d chamadas de trava, veio %d", serveLockAttempts, calls)
	}
	if min := time.Duration(serveLockAttempts-1) * serveLockBackoff; elapsed < min {
		t.Fatalf("espera entre tentativas: %v < %v", elapsed, min)
	}
	_ = l.Release()
}

func TestServeLock_TransitoriaEsgotadaRecusa(t *testing.T) {
	var calls int
	fakeServeOps(t, osOpenServe, func(*os.File) error {
		calls++
		return lockTransientErrnos[0]
	})

	_, err := AcquireServeLock(t.TempDir(), false, newServeTestLogger(&bytes.Buffer{}))
	if err == nil {
		t.Fatal("transitória esgotada deveria recusar o boot")
	}
	if errors.Is(err, ErrInstanceLocked) {
		t.Fatalf("transitória esgotada não é trava ocupada: %v", err)
	}
	if calls != serveLockAttempts {
		t.Fatalf("esperava %d tentativas, veio %d", serveLockAttempts, calls)
	}
	if !errors.Is(err, lockTransientErrnos[0]) {
		t.Fatalf("erro deve encadear a causa transitória: %v", err)
	}
}

func TestServeLock_TransitoriaNaAberturaEhRepetida(t *testing.T) {
	var opens int
	fakeServeOps(t, func(path string) (*os.File, error) {
		opens++
		if opens <= 2 {
			return nil, lockTransientErrnos[0]
		}
		return osOpenServe(path)
	}, func(*os.File) error { return nil })

	l, err := AcquireServeLock(t.TempDir(), false, newServeTestLogger(&bytes.Buffer{}))
	if err != nil {
		t.Fatalf("abertura após transitórias deveria passar: %v", err)
	}
	if opens != 3 {
		t.Fatalf("esperava 3 aberturas, veio %d", opens)
	}
	_ = l.Release()
}

func TestServeLock_SemSuporteSobeSemTravaComAviso(t *testing.T) {
	var calls int
	fakeServeOps(t, osOpenServe, func(*os.File) error {
		calls++
		return lockUnsupportedErrnos[0]
	})

	var buf bytes.Buffer
	l, err := AcquireServeLock(t.TempDir(), false, newServeTestLogger(&buf))
	if err != nil {
		t.Fatalf("sem suporte deveria subir, veio %v", err)
	}
	if l.Held() {
		t.Fatal("sem suporte não tem trava")
	}
	if calls != 1 {
		t.Fatalf("sem suporte não é transitória, esperava 1 chamada, veio %d", calls)
	}
	if !strings.Contains(buf.String(), "sem suporte a trava") {
		t.Fatalf("esperava aviso de sem suporte: %q", buf.String())
	}
	if err := l.Release(); err != nil {
		t.Fatalf("release sem trava: %v", err)
	}
}

func TestServeLock_ErroNaoClassificadoAoTravarRecusa(t *testing.T) {
	cause := errors.New("falha não prevista")
	for _, force := range []bool{false, true} {
		var calls int
		fakeServeOps(t, osOpenServe, func(*os.File) error {
			calls++
			return cause
		})

		_, err := AcquireServeLock(t.TempDir(), force, newServeTestLogger(&bytes.Buffer{}))
		if !errors.Is(err, cause) {
			t.Fatalf("force=%v: erro não classificado deveria recusar com a causa, veio %v", force, err)
		}
		if errors.Is(err, ErrInstanceLocked) {
			t.Fatalf("force=%v: erro não classificado não é trava ocupada", force)
		}
		if calls != 1 {
			t.Fatalf("force=%v: erro não classificado não é repetido, veio %d chamadas", force, calls)
		}
	}
}

func TestServeLock_ErroNaoClassificadoAoAbrirRecusa(t *testing.T) {
	cause := errors.New("permissão negada")
	fakeServeOps(t, func(string) (*os.File, error) { return nil, cause }, func(*os.File) error {
		t.Fatal("trava não deveria ser tentada quando a abertura falha")
		return nil
	})

	_, err := AcquireServeLock(t.TempDir(), true, newServeTestLogger(&bytes.Buffer{}))
	if !errors.Is(err, cause) {
		t.Fatalf("erro de abertura não classificado deveria recusar, veio %v", err)
	}
}
