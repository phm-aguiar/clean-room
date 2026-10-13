package wiki

import (
	"context"
	"fmt"
	"sort"
	"sync"

	"github.com/phm-aguiar/ai-memory-go/internal/domain"
)

// lockKey identifica a trava de uma página. Usa a mesma chave lógica da colisão de caminhos
// (ADR-06): escopo mais PortableKey. Assim `Notas/Rota.md` e `notas/rota.md` no mesmo projeto
// disputam a mesma trava, porque no Windows e no macOS são o mesmo arquivo.
type lockKey struct {
	scope    domain.Scope
	portable string
}

// text devolve a chave composta `ws/proj/portablekey`, usada para ordenar a aquisição.
func (k lockKey) text() string {
	return k.scope.WorkspaceID.String() + "/" + k.scope.ProjectID.String() + "/" + k.portable
}

// PageLocks é um mutex por página, com espera cancelável por context.
//
// Cada trava é um canal de capacidade 1 (semáforo): enviar adquire, receber libera. Não se usa
// sync.Mutex porque ele não respeita context. O mapa cresce com as chaves já vistas e não é
// limpo: uma entrada por página travada desde a criação. Não há goroutine de fundo.
type PageLocks struct {
	mu    sync.Mutex
	slots map[lockKey]chan struct{}
}

// NewPageLocks cria um conjunto vazio de travas por página.
func NewPageLocks() *PageLocks {
	return &PageLocks{slots: make(map[lockKey]chan struct{})}
}

// slot devolve o canal de capacidade 1 da chave, criando-o sob o mutex se ainda não existe.
func (l *PageLocks) slot(k lockKey) chan struct{} {
	l.mu.Lock()
	defer l.mu.Unlock()
	ch, ok := l.slots[k]
	if !ok {
		ch = make(chan struct{}, 1)
		l.slots[k] = ch
	}
	return ch
}

// Lock adquire as travas de todas as refs e devolve a função que as libera.
//
// As travas são adquiridas em ordem determinística (chave composta `ws/proj/portablekey`),
// sem duplicatas, para que duas escritas com refs sobrepostas não entrem em deadlock.
// Se ctx cancela enquanto espera, Lock libera as travas já obtidas e devolve erro com %w de
// ctx.Err(). A função de liberação é idempotente. Sem refs, devolve liberação vazia.
func (l *PageLocks) Lock(ctx context.Context, refs ...domain.PageRef) (func(), error) {
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("trava de página: %w", err)
	}

	seen := make(map[lockKey]struct{}, len(refs))
	keys := make([]lockKey, 0, len(refs))
	for _, r := range refs {
		k := lockKey{scope: r.Scope, portable: r.Path.PortableKey()}
		if _, dup := seen[k]; dup {
			continue
		}
		seen[k] = struct{}{}
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool { return keys[i].text() < keys[j].text() })

	held := make([]chan struct{}, 0, len(keys))
	release := func() {
		for i := len(held) - 1; i >= 0; i-- {
			<-held[i]
		}
	}

	for _, k := range keys {
		ch := l.slot(k)
		select {
		case ch <- struct{}{}:
			held = append(held, ch)
		case <-ctx.Done():
			release()
			return nil, fmt.Errorf("trava de página: %w", ctx.Err())
		}
	}

	var once sync.Once
	return func() { once.Do(release) }, nil
}
