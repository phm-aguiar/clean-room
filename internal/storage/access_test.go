package storage

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	"github.com/phm-aguiar/ai-memory-go/internal/domain"
)

// accPage cria um banco, um escritor e uma página corrente, e devolve o id da versão.
func accPage(t *testing.T) (*sql.DB, Writer, domain.PageID) {
	t.Helper()
	db, w, ws := upEnv(t)
	scope := upScope(t, w, ws, "ws-acc", "pj-acc")
	ref := domain.NewPageRef(scope, upPath(t, "notas/acesso.md"))
	res := upMustUpsert(t, w, upIn(t, ref, "corpo", "sess-1", upT0))
	return db, w, res.Version
}

// accCount devolve COUNT(*) de page_access para a página e o operador (actor == "" conta todos).
func accCount(t *testing.T, db *sql.DB, page domain.PageID, actor string) int {
	t.Helper()
	if actor == "" {
		return upCount(t, db, `SELECT COUNT(*) FROM page_access WHERE page_id = ?`, page.Bytes())
	}
	return upCount(t, db, `SELECT COUNT(*) FROM page_access WHERE page_id = ? AND actor = ?`, page.Bytes(), actor)
}

// accAccessCount lê pages.access_count da versão.
func accAccessCount(t *testing.T, db *sql.DB, page domain.PageID) int {
	t.Helper()
	return upCount(t, db, `SELECT access_count FROM pages WHERE id = ?`, page.Bytes())
}

// accMustBump chama BumpAccess e falha o teste em erro.
func accMustBump(t *testing.T, th *AccessThrottle, w Writer, page domain.PageID, actor string, now time.Time) bool {
	t.Helper()
	ok, err := th.BumpAccess(context.Background(), w, page, actor, now)
	if err != nil {
		t.Fatalf("BumpAccess(%q, %v): %v", actor, now, err)
	}
	return ok
}

func TestAccessPrimeiroBumpIncrementaELinha(t *testing.T) {
	db, w, page := accPage(t)
	th := NewAccessThrottle()

	if !accMustBump(t, th, w, page, "op-a", upT0) {
		t.Fatal("primeiro bump: bumped=false, esperado true")
	}
	if got := accAccessCount(t, db, page); got != 1 {
		t.Fatalf("access_count = %d, esperado 1", got)
	}
	if got := accCount(t, db, page, "op-a"); got != 1 {
		t.Fatalf("linhas de page_access para op-a = %d, esperado 1", got)
	}
	var last int64
	if err := db.QueryRow(`SELECT last_accessed_at FROM pages WHERE id = ?`, page.Bytes()).Scan(&last); err != nil {
		t.Fatalf("ler last_accessed_at: %v", err)
	}
	if last != upT0.UnixMicro() {
		t.Fatalf("last_accessed_at = %d, esperado %d", last, upT0.UnixMicro())
	}
}

func TestAccessMesmoAtorDentroDoMinutoNaoIncrementa(t *testing.T) {
	db, w, page := accPage(t)
	th := NewAccessThrottle()

	if !accMustBump(t, th, w, page, "op-a", upT0) {
		t.Fatal("primeiro bump: bumped=false, esperado true")
	}
	if accMustBump(t, th, w, page, "op-a", upT0.Add(30*time.Second)) {
		t.Fatal("bump após 30 s: bumped=true, esperado false (throttle)")
	}
	if accMustBump(t, th, w, page, "op-a", upT0.Add(time.Minute-time.Millisecond)) {
		t.Fatal("bump a 59,999 s: bumped=true, esperado false (throttle)")
	}
	if got := accAccessCount(t, db, page); got != 1 {
		t.Fatalf("access_count = %d, esperado 1 (bumps throttled não contam)", got)
	}
}

func TestAccessDepoisDeUmMinutoIncrementaDeNovo(t *testing.T) {
	db, w, page := accPage(t)
	th := NewAccessThrottle()

	accMustBump(t, th, w, page, "op-a", upT0)
	if !accMustBump(t, th, w, page, "op-a", upT0.Add(time.Minute)) {
		t.Fatal("bump a exatamente 1 min: bumped=false, esperado true")
	}
	if got := accAccessCount(t, db, page); got != 2 {
		t.Fatalf("access_count = %d, esperado 2", got)
	}
}

func TestAccessOutroAtorNaoSofreThrottleDoPrimeiro(t *testing.T) {
	db, w, page := accPage(t)
	th := NewAccessThrottle()

	accMustBump(t, th, w, page, "op-a", upT0)
	if !accMustBump(t, th, w, page, "op-b", upT0.Add(time.Second)) {
		t.Fatal("op-b na mesma página: bumped=false, esperado true")
	}
	if got := accAccessCount(t, db, page); got != 2 {
		t.Fatalf("access_count = %d, esperado 2", got)
	}
	if got := accCount(t, db, page, ""); got != 2 {
		t.Fatalf("linhas de page_access = %d, esperado 2", got)
	}
}

func TestAccessVersaoSupersededNaoIncrementa(t *testing.T) {
	db, w, ws := upEnv(t)
	scope := upScope(t, w, ws, "ws-acc-sup", "pj-acc-sup")
	ref := domain.NewPageRef(scope, upPath(t, "notas/acesso.md"))
	v1 := upMustUpsert(t, w, upIn(t, ref, "corpo v1", "sess-1", upT0)).Version
	v2 := upMustUpsert(t, w, upIn(t, ref, "corpo v2", "sess-2", upT0.Add(time.Hour))).Version
	th := NewAccessThrottle()

	if accMustBump(t, th, w, v1, "op-a", upT0.Add(2*time.Hour)) {
		t.Fatal("versão superseded: bumped=true, esperado false")
	}
	if got := accAccessCount(t, db, v1); got != 0 {
		t.Fatalf("access_count da versão superseded = %d, esperado 0", got)
	}
	if got := accCount(t, db, v1, ""); got != 0 {
		t.Fatalf("linhas de page_access da versão superseded = %d, esperado 0", got)
	}

	if !accMustBump(t, th, w, v2, "op-a", upT0.Add(2*time.Hour)) {
		t.Fatal("versão corrente: bumped=false, esperado true")
	}
	if got := accAccessCount(t, db, v2); got != 1 {
		t.Fatalf("access_count da versão corrente = %d, esperado 1", got)
	}
}

func TestAccessAtorVazioNaoGrava(t *testing.T) {
	db, w, page := accPage(t)
	th := NewAccessThrottle()

	ok, err := th.BumpAccess(context.Background(), w, page, "", upT0)
	if err != nil {
		t.Fatalf("ator vazio: erro inesperado: %v", err)
	}
	if ok {
		t.Fatal("ator vazio: bumped=true, esperado false")
	}
	if got := accAccessCount(t, db, page); got != 0 {
		t.Fatalf("access_count = %d, esperado 0", got)
	}
	if got := accCount(t, db, page, ""); got != 0 {
		t.Fatalf("linhas de page_access = %d, esperado 0", got)
	}
	if len(th.last) != 0 {
		t.Fatalf("ator vazio deixou %d entradas no throttle, esperado 0", len(th.last))
	}
}

func TestAccessPaginaInexistenteNaoGrava(t *testing.T) {
	db, w, _ := accPage(t)
	th := NewAccessThrottle()

	missingID, err := domain.NewID()
	if err != nil {
		t.Fatalf("NewID: %v", err)
	}
	missing := domain.PageID(missingID)

	ok, err := th.BumpAccess(context.Background(), w, missing, "op-a", upT0)
	if err != nil {
		t.Fatalf("página inexistente: erro inesperado: %v", err)
	}
	if ok {
		t.Fatal("página inexistente: bumped=true, esperado false")
	}
	if got := accCount(t, db, missing, ""); got != 0 {
		t.Fatalf("linhas de page_access para página inexistente = %d, esperado 0", got)
	}
}

func TestAccessFalhaDoWriterDesfazReserva(t *testing.T) {
	db, _, page := accPage(t)
	ctx := context.Background()
	w, err := NewWriter(ctx, db)
	if err != nil {
		t.Fatalf("NewWriter: %v", err)
	}
	if err := w.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	th := NewAccessThrottle()

	ok, err := th.BumpAccess(ctx, w, page, "op-a", upT0)
	if !errors.Is(err, ErrWriterClosed) {
		t.Fatalf("writer fechado: erro = %v, esperado ErrWriterClosed", err)
	}
	if ok {
		t.Fatal("writer fechado: bumped=true, esperado false")
	}
	if len(th.last) != 0 {
		t.Fatalf("reserva não foi desfeita: %d entradas no throttle", len(th.last))
	}
}

func TestAccessLimpezaPreguicosaRemoveExpiradas(t *testing.T) {
	_, w, page := accPage(t)
	th := NewAccessThrottle()

	// Enche o mapa até o limiar com entradas que já expiraram.
	for i := 0; i < accessSweepMin; i++ {
		id, err := domain.NewID()
		if err != nil {
			t.Fatalf("NewID: %v", err)
		}
		th.last[accessKey{page: domain.PageID(id), actor: "antigo"}] = upT0
	}

	later := upT0.Add(2 * time.Minute)
	if !accMustBump(t, th, w, page, "op-a", later) {
		t.Fatal("bump após expiração: bumped=false, esperado true")
	}
	if len(th.last) != 1 {
		t.Fatalf("mapa com %d entradas após limpeza, esperado 1", len(th.last))
	}
}

func TestAccessNovoThrottleComecaVazio(t *testing.T) {
	db, w, page := accPage(t)

	accMustBump(t, NewAccessThrottle(), w, page, "op-a", upT0)
	// Simula reinício: outro AccessThrottle não conhece o último bump (ADR-29).
	if !accMustBump(t, NewAccessThrottle(), w, page, "op-a", upT0.Add(time.Second)) {
		t.Fatal("throttle novo: bumped=false, esperado true (estado em memória se perde)")
	}
	if got := accAccessCount(t, db, page); got != 2 {
		t.Fatalf("access_count = %d, esperado 2", got)
	}
}
