package storage

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/phm-aguiar/ai-memory-go/internal/domain"
)

// accessWindow é o intervalo mínimo entre dois bumps do mesmo par página e operador.
const accessWindow = time.Minute

// accessSweepMin é o tamanho do mapa abaixo do qual a limpeza preguiçosa não roda.
const accessSweepMin = 1024

// accessKey identifica o par (versão corrente da página, operador) do throttle.
type accessKey struct {
	page  domain.PageID
	actor string
}

// AccessThrottle guarda, em memória, o instante do último bump de acesso por par
// (PageID, operador). Máximo 1 bump por minuto por par (ADR-29).
//
// O estado vive só na memória do processo e se perde no reinício: depois de um
// reinício, o primeiro bump de cada par passa de novo. Isso é aceito porque o
// contador é aproximado e o schema de page_access não tem timestamp (V01 é contrato).
//
// Limpeza preguiçosa: ao entrar em BumpAccess, se o mapa atingiu o limiar
// (sweepAt), as entradas com mais de 1 minuto são removidas. Depois da limpeza o
// limiar vira max(accessSweepMin, 2*len), então o mapa fica limitado às entradas
// vivas e não cresce sem fim. Não há goroutine de limpeza.
//
// AccessThrottle é seguro para uso concorrente. Deve ser criado com NewAccessThrottle.
type AccessThrottle struct {
	mu      sync.Mutex
	last    map[accessKey]time.Time
	sweepAt int
}

// NewAccessThrottle cria um throttle vazio.
func NewAccessThrottle() *AccessThrottle {
	return &AccessThrottle{
		last:    make(map[accessKey]time.Time),
		sweepAt: accessSweepMin,
	}
}

// BumpAccess registra um acesso de actor à versão pageID (reforço de acesso, search-contract (e)).
//
// Devolve bumped=true quando incrementou pages.access_count e last_accessed_at e
// gravou a linha de page_access. Devolve bumped=false, sem erro, quando:
//   - actor é vazio (nada é gravado);
//   - o throttle ainda não expirou para o par (pageID, actor);
//   - a página não é versão corrente (is_latest = 0) ou não existe.
//
// Só quem chama o Writer grava: a escrita passa por w.Do (INV-02). O instante now
// vale para o throttle e para last_accessed_at (now.UnixMicro()); é injetado para
// que os testes controlem o relógio.
//
// A reserva do par é feita antes de w.Do, sob o mutex, para que duas chamadas
// concorrentes do mesmo par não passem as duas. Se w.Do falhar, a reserva é
// desfeita. Erros voltam com %w.
func (t *AccessThrottle) BumpAccess(ctx context.Context, w Writer, pageID domain.PageID, actor string, now time.Time) (bool, error) {
	if actor == "" {
		return false, nil
	}
	key := accessKey{page: pageID, actor: actor}

	t.mu.Lock()
	t.sweepLocked(now)
	prev, seen := t.last[key]
	if seen && now.Sub(prev) < accessWindow {
		t.mu.Unlock()
		return false, nil
	}
	t.last[key] = now
	t.mu.Unlock()

	bumped := false
	err := w.Do(ctx, func(tx *sql.Tx) error {
		ok, err := bumpAccessTx(tx, pageID, actor, now)
		if err != nil {
			return err
		}
		bumped = ok
		return nil
	})
	if err != nil {
		t.release(key, now, prev, seen)
		return false, fmt.Errorf("bump de acesso: %w", err)
	}
	return bumped, nil
}

// release desfaz a reserva feita por BumpAccess, se nenhuma outra chamada a alterou.
func (t *AccessThrottle) release(key accessKey, reserved, prev time.Time, seen bool) {
	t.mu.Lock()
	defer t.mu.Unlock()
	cur, ok := t.last[key]
	if !ok || !cur.Equal(reserved) {
		return
	}
	if seen {
		t.last[key] = prev
		return
	}
	delete(t.last, key)
}

// sweepLocked remove as entradas expiradas quando o mapa atingiu o limiar. Deve ser
// chamada com t.mu travado.
func (t *AccessThrottle) sweepLocked(now time.Time) {
	if len(t.last) < t.sweepAt {
		return
	}
	for k, at := range t.last {
		if now.Sub(at) >= accessWindow {
			delete(t.last, k)
		}
	}
	t.sweepAt = max(accessSweepMin, 2*len(t.last))
}

// bumpAccessTx faz o trabalho dentro da transação do Writer. Página ausente ou
// não corrente devolve false sem erro e sem escrita.
func bumpAccessTx(tx *sql.Tx, pageID domain.PageID, actor string, now time.Time) (bool, error) {
	var isLatest int
	err := tx.QueryRow(`SELECT is_latest FROM pages WHERE id = ?`, pageID.Bytes()).Scan(&isLatest)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("ler versão da página: %w", err)
	}
	if isLatest != 1 {
		return false, nil
	}
	if _, err := tx.Exec(
		`UPDATE pages SET access_count = access_count + 1, last_accessed_at = ? WHERE id = ?`,
		now.UnixMicro(), pageID.Bytes(),
	); err != nil {
		return false, fmt.Errorf("incrementar acesso da página: %w", err)
	}
	if _, err := tx.Exec(
		`INSERT INTO page_access (page_id, actor) VALUES (?, ?) ON CONFLICT(page_id, actor) DO NOTHING`,
		pageID.Bytes(), actor,
	); err != nil {
		return false, fmt.Errorf("registrar operador da página: %w", err)
	}
	return true, nil
}
