package wiki

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/phm-aguiar/ai-memory-go/internal/domain"
)

func newTestScope(t *testing.T) domain.Scope {
	t.Helper()
	ws, err := domain.NewID()
	if err != nil {
		t.Fatalf("NewID: %v", err)
	}
	pr, err := domain.NewID()
	if err != nil {
		t.Fatalf("NewID: %v", err)
	}
	return domain.NewScope(domain.WorkspaceID(ws), domain.ProjectID(pr))
}

func newTestRef(t *testing.T, s domain.Scope, path string) domain.PageRef {
	t.Helper()
	p, err := domain.ParsePagePath(path)
	if err != nil {
		t.Fatalf("ParsePagePath(%q): %v", path, err)
	}
	return domain.NewPageRef(s, p)
}

// lockWithin chama Lock com um prazo curto, para que um bloqueio indevido vire falha e não travamento.
func lockWithin(t *testing.T, d time.Duration, l *PageLocks, refs ...domain.PageRef) (func(), error) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), d)
	defer cancel()
	return l.Lock(ctx, refs...)
}

func TestLock_MesmaChaveBloqueiaAteUnlock(t *testing.T) {
	l := NewPageLocks()
	s := newTestScope(t)
	ref := newTestRef(t, s, "notes/a.md")

	unlock1, err := lockWithin(t, time.Second, l, ref)
	if err != nil {
		t.Fatalf("primeira trava: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	acquired := make(chan func(), 1)
	errc := make(chan error, 1)
	go func() {
		u, err := l.Lock(ctx, ref)
		if err != nil {
			errc <- err
			return
		}
		acquired <- u
	}()

	select {
	case <-acquired:
		t.Fatal("segunda trava adquirida antes do unlock")
	case err := <-errc:
		t.Fatalf("segunda trava falhou antes do unlock: %v", err)
	case <-time.After(50 * time.Millisecond):
	}

	unlock1()

	select {
	case u := <-acquired:
		u()
	case err := <-errc:
		t.Fatalf("segunda trava falhou após unlock: %v", err)
	case <-time.After(time.Second):
		t.Fatal("segunda trava não foi adquirida após unlock")
	}
}

func TestLock_ChavesDiferentesNaoBloqueiam(t *testing.T) {
	l := NewPageLocks()
	s := newTestScope(t)
	a := newTestRef(t, s, "notes/a.md")
	b := newTestRef(t, s, "notes/b.md")

	ua, err := lockWithin(t, time.Second, l, a)
	if err != nil {
		t.Fatalf("trava A: %v", err)
	}
	defer ua()

	ub, err := lockWithin(t, time.Second, l, b)
	if err != nil {
		t.Fatalf("trava B bloqueou com chave diferente: %v", err)
	}
	ub()
}

func TestLock_EscopoDiferenteNaoBloqueia(t *testing.T) {
	l := NewPageLocks()
	s1 := newTestScope(t)
	s2 := newTestScope(t)
	ua, err := lockWithin(t, time.Second, l, newTestRef(t, s1, "notes/a.md"))
	if err != nil {
		t.Fatalf("trava s1: %v", err)
	}
	defer ua()

	ub, err := lockWithin(t, time.Second, l, newTestRef(t, s2, "notes/a.md"))
	if err != nil {
		t.Fatalf("trava s2 bloqueou com escopo diferente: %v", err)
	}
	ub()
}

func TestLock_CaixaDiferenteDisputaMesmaTrava(t *testing.T) {
	l := NewPageLocks()
	s := newTestScope(t)
	upper := newTestRef(t, s, "Notas/Rota.md")
	lower := newTestRef(t, s, "notas/rota.md")

	u1, err := lockWithin(t, time.Second, l, upper)
	if err != nil {
		t.Fatalf("trava Notas/Rota.md: %v", err)
	}
	defer u1()

	_, err = lockWithin(t, 100*time.Millisecond, l, lower)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("notas/rota.md deveria disputar a trava de Notas/Rota.md; err = %v", err)
	}
}

func TestLock_ReferenciasEmOrdemInversaNaoTravam(t *testing.T) {
	l := NewPageLocks()
	s := newTestScope(t)
	a := newTestRef(t, s, "notes/a.md")
	b := newTestRef(t, s, "notes/b.md")

	const rodadas = 200
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	var wg sync.WaitGroup
	errc := make(chan error, 2*rodadas)
	adota := func(refs ...domain.PageRef) {
		defer wg.Done()
		for i := 0; i < rodadas; i++ {
			u, err := l.Lock(ctx, refs...)
			if err != nil {
				errc <- err
				return
			}
			u()
		}
	}

	wg.Add(2)
	go adota(a, b)
	go adota(b, a)

	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(15 * time.Second):
		t.Fatal("deadlock: adoções com refs em ordem inversa não terminaram")
	}
	close(errc)
	for err := range errc {
		t.Fatalf("adoção falhou: %v", err)
	}
}

func TestLock_ContextoCanceladoLiberaParcialENaoPrendeTrava(t *testing.T) {
	l := NewPageLocks()
	s := newTestScope(t)
	a := newTestRef(t, s, "notes/a.md")
	b := newTestRef(t, s, "notes/b.md")

	// Ordem textual: a antes de b. Segura B; a adoção (A, B) pega A primeiro e espera em B.
	ub, err := lockWithin(t, time.Second, l, b)
	if err != nil {
		t.Fatalf("trava B: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	_, err = l.Lock(ctx, a, b)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("espera cancelada deveria devolver DeadlineExceeded; err = %v", err)
	}

	ub()

	// A não pode ter ficado presa pela adoção cancelada.
	ua, err := lockWithin(t, time.Second, l, a)
	if err != nil {
		t.Fatalf("trava A presa após adoção cancelada: %v", err)
	}
	ua()
}

func TestLock_ContextoJaCanceladoDevolveErro(t *testing.T) {
	l := NewPageLocks()
	s := newTestScope(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := l.Lock(ctx, newTestRef(t, s, "notes/a.md"))
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("ctx já cancelado deveria devolver Canceled; err = %v", err)
	}
}

func TestLock_RefsDuplicadasNaoTravam(t *testing.T) {
	l := NewPageLocks()
	s := newTestScope(t)
	a := newTestRef(t, s, "notes/a.md")
	aCaixa := newTestRef(t, s, "Notes/A.md")

	u, err := lockWithin(t, time.Second, l, a, a, aCaixa)
	if err != nil {
		t.Fatalf("refs duplicadas travaram sozinhas: %v", err)
	}
	u()
}

func TestLock_UnlockIdempotente(t *testing.T) {
	l := NewPageLocks()
	s := newTestScope(t)
	ref := newTestRef(t, s, "notes/a.md")

	u, err := lockWithin(t, time.Second, l, ref)
	if err != nil {
		t.Fatalf("trava: %v", err)
	}
	u()
	u()

	u2, err := lockWithin(t, time.Second, l, ref)
	if err != nil {
		t.Fatalf("trava após unlock duplo ficou presa: %v", err)
	}
	u2()
}

func TestLock_SemRefsDevolveLiberacaoVazia(t *testing.T) {
	l := NewPageLocks()
	u, err := lockWithin(t, time.Second, l)
	if err != nil {
		t.Fatalf("Lock sem refs: %v", err)
	}
	u()
}
