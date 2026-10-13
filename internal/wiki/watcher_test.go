package wiki

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// wcLog é um destino de log seguro para goroutines: o watcher escreve e o teste lê.
type wcLog struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (l *wcLog) Write(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.buf.Write(p)
}

func (l *wcLog) String() string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.buf.String()
}

// wcNew monta o ambiente de um teste do watcher: wpEnv real, watcher com intervalos injetados e log.
func wcNew(t *testing.T, debounce, reconcile time.Duration) (*wpEnv, *Watcher, *wcLog) {
	t.Helper()
	env := wpNewEnv(t)
	w, err := NewWatcher(env.store, debounce, reconcile)
	if err != nil {
		t.Fatalf("NewWatcher: %v", err)
	}
	logs := &wcLog{}
	w.log = slog.New(slog.NewTextHandler(logs, nil))
	return env, w, logs
}

// wcStart roda Run em uma goroutine e espera o watcher armar as observações. Ao fim do teste cancela
// o contexto e espera Run devolver, para não deixar goroutine viva.
func wcStart(t *testing.T, env *wpEnv, w *Watcher) {
	t.Helper()
	armed := make(chan struct{})
	w.onArmed = func() { close(armed) }
	ctx, cancel := context.WithCancel(env.ctx)
	done := make(chan error, 1)
	go func() { done <- w.Run(ctx) }()
	t.Cleanup(func() {
		cancel()
		select {
		case <-done:
		case <-time.After(5 * time.Second):
			t.Errorf("Run não devolveu depois do cancelamento")
		}
	})
	select {
	case <-armed:
	case err := <-done:
		t.Fatalf("Run terminou antes de armar: %v", err)
	case <-time.After(5 * time.Second):
		t.Fatalf("watcher não armou em 5 s")
	}
}

// wcWaitFor repete cond até ela valer ou o prazo acabar. Falha o teste com a descrição.
func wcWaitFor(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("esperado em 5 s: %s", what)
}

// wcCount conta linhas da consulta, que deve devolver um único inteiro.
func wcCount(t *testing.T, db *sql.DB, query string, args ...any) int {
	t.Helper()
	var n int
	if err := db.QueryRow(query, args...).Scan(&n); err != nil {
		t.Fatalf("contagem %q: %v", query, err)
	}
	return n
}

// wcLatestBody devolve o corpo da versão corrente de um caminho, ou "" se não há página.
func wcLatestBody(t *testing.T, db *sql.DB, path string) string {
	t.Helper()
	var body string
	err := db.QueryRow(`SELECT body FROM pages WHERE path = ? AND is_latest = 1`, path).Scan(&body)
	if errors.Is(err, sql.ErrNoRows) {
		return ""
	}
	if err != nil {
		t.Fatalf("corpo de %s: %v", path, err)
	}
	return body
}

func TestWatcherReindexaEdicaoManualDepoisDoDebounce(t *testing.T) {
	env, w, _ := wcNew(t, 50*time.Millisecond, time.Hour)
	wpMustWrite(t, env, wpInput(t, env, "notas/a.md", "Versão um.\n"))
	wcStart(t, env, w)

	abs := wpAbs(env, "notas/a.md")
	if err := os.WriteFile(abs, []byte("Versão dois, editada à mão.\n"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	wcWaitFor(t, "nova versão da página após edição manual", func() bool {
		return wcCount(t, env.db, `SELECT COUNT(*) FROM pages WHERE path = ?`, "notas/a.md") == 2
	})
	if body := wcLatestBody(t, env.db, "notas/a.md"); !strings.Contains(body, "editada à mão") {
		t.Fatalf("versão corrente não traz a edição: %q", body)
	}
}

func TestWatcherRajadaDeEdicoesViraUmaReindexacao(t *testing.T) {
	env, w, _ := wcNew(t, 250*time.Millisecond, time.Hour)
	wpMustWrite(t, env, wpInput(t, env, "notas/r.md", "Base.\n"))
	wcStart(t, env, w)

	abs := wpAbs(env, "notas/r.md")
	for i := 0; i < 5; i++ {
		body := "Rajada " + string(rune('A'+i)) + ".\n"
		if err := os.WriteFile(abs, []byte(body), 0o644); err != nil {
			t.Fatalf("WriteFile %d: %v", i, err)
		}
	}
	wcWaitFor(t, "versão da rajada", func() bool {
		return wcLatestBody(t, env.db, "notas/r.md") == "Rajada E.\n"
	})
	// Depois de estabilizar, a rajada inteira gerou uma única versão nova, não cinco.
	time.Sleep(400 * time.Millisecond)
	if n := wcCount(t, env.db, `SELECT COUNT(*) FROM pages WHERE path = ?`, "notas/r.md"); n != 2 {
		t.Fatalf("versões após rajada = %d, quer 2", n)
	}
}

func TestWatcherEscritaTemporariaELedgersNaoSaoIndexados(t *testing.T) {
	env, w, _ := wcNew(t, 50*time.Millisecond, time.Hour)
	// Semente antes de armar: registra workspace e projeto no banco (só WritePage faz isso) e cria
	// `notas/`, para que o watcher observe o diretório desde o início. Sem ela, a sentinela não
	// consegue ser indexada.
	wpMustWrite(t, env, wpInput(t, env, "notas/semente.md", "Semente.\n"))
	wcStart(t, env, w)

	ignorados := map[string]string{
		"notas/.ai-memory-tmp.x.md": "temporário do wiki",
		"log.md":                    "ledger na raiz do projeto",
		"_meta.md":                  "manifesto do projeto",
		"notas/log-2026-03.md":      "ledger mensal",
		"bootstrap.md":              "bootstrap",
		"_pending/p.md":             "pendência",
		".git/x.md":                 "interno do git",
		"notas/texto.txt":           "não é markdown",
	}
	for rel, body := range ignorados {
		abs := wpAbs(env, rel)
		if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
			t.Fatalf("MkdirAll %s: %v", rel, err)
		}
		if err := os.WriteFile(abs, []byte("conteúdo de "+body+"\n"), 0o644); err != nil {
			t.Fatalf("WriteFile %s: %v", rel, err)
		}
	}

	// Sentinela: um .md comum escrito depois. Quando ele aparece, os eventos anteriores já passaram.
	if err := os.WriteFile(wpAbs(env, "notas/sentinela.md"), []byte("Sentinela.\n"), 0o644); err != nil {
		t.Fatalf("WriteFile sentinela: %v", err)
	}
	wcWaitFor(t, "sentinela indexada", func() bool {
		return wcCount(t, env.db, `SELECT COUNT(*) FROM pages WHERE path = ?`, "notas/sentinela.md") == 1
	})
	time.Sleep(200 * time.Millisecond)

	for rel := range ignorados {
		if rel == "notas/texto.txt" {
			continue
		}
		key := filepath.ToSlash(rel)
		if n := wcCount(t, env.db, `SELECT COUNT(*) FROM pages WHERE path = ?`, key); n != 0 {
			t.Errorf("caminho ignorado %s virou página (%d linhas)", key, n)
		}
	}
}

func TestWatcherApagarArquivoNaoApagaAPagina(t *testing.T) {
	env, w, _ := wcNew(t, 50*time.Millisecond, time.Hour)
	wpMustWrite(t, env, wpInput(t, env, "notas/b.md", "Página que será apagada do disco.\n"))
	before := wcCount(t, env.db, `SELECT COUNT(*) FROM pages WHERE path = ? AND is_latest = 1`, "notas/b.md")
	if before != 1 {
		t.Fatalf("setup: páginas correntes = %d", before)
	}
	wcStart(t, env, w)

	if err := os.Remove(wpAbs(env, "notas/b.md")); err != nil {
		t.Fatalf("Remove: %v", err)
	}
	wcWaitFor(t, "remoção registrada", func() bool { return w.Removals() == 1 })
	time.Sleep(200 * time.Millisecond)

	if n := wcCount(t, env.db, `SELECT COUNT(*) FROM pages WHERE path = ? AND is_latest = 1`, "notas/b.md"); n != 1 {
		t.Fatalf("página sumiu do banco após apagar o arquivo: %d linhas correntes", n)
	}
	if n := wcCount(t, env.db, `SELECT COUNT(*) FROM pages WHERE path = ?`, "notas/b.md"); n != 1 {
		t.Fatalf("versões mudaram após remoção: %d", n)
	}
}

func TestWatcherCriacaoDeDiretorioReindexaOProjeto(t *testing.T) {
	env, w, _ := wcNew(t, 50*time.Millisecond, time.Hour)
	wpMustWrite(t, env, wpInput(t, env, "notas/inicial.md", "Inicial.\n"))
	wcStart(t, env, w)

	dir := wpAbs(env, "conceitos/novo")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "x.md"), []byte("Página em diretório novo.\n"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	wcWaitFor(t, "página do diretório novo indexada", func() bool {
		return wcCount(t, env.db, `SELECT COUNT(*) FROM pages WHERE path = ?`, "conceitos/novo/x.md") == 1
	})
}

func TestWatcherRunRetornaAoCancelarSemVazamento(t *testing.T) {
	env, w, _ := wcNew(t, 50*time.Millisecond, time.Hour)
	ctx, cancel := context.WithCancel(env.ctx)
	done := make(chan error, 1)
	armed := make(chan struct{})
	w.onArmed = func() { close(armed) }
	go func() { done <- w.Run(ctx) }()

	select {
	case <-armed:
	case <-time.After(5 * time.Second):
		t.Fatalf("watcher não armou")
	}
	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Run após cancelamento: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatalf("Run não retornou 2 s após o cancelamento")
	}
}

func TestWatcherRaizInvalidaFalhaNaCriacaoENoRun(t *testing.T) {
	env := wpNewEnv(t)
	if _, err := NewWatcher(env.store, 0, time.Second); err == nil {
		t.Fatalf("NewWatcher aceitou debounce zero")
	}
	if _, err := NewWatcher(nil, time.Second, time.Second); err == nil {
		t.Fatalf("NewWatcher aceitou Store nil")
	}
	bad := *env.store
	bad.Root = filepath.Join(env.root, "nao-existe")
	if _, err := NewWatcher(&bad, time.Second, time.Second); err == nil {
		t.Fatalf("NewWatcher aceitou raiz inexistente")
	}
}

func TestWatcherReconciliacaoReindexaArquivoPerdido(t *testing.T) {
	env, w, _ := wcNew(t, 50*time.Millisecond, 50*time.Millisecond)
	// Arquivo gravado antes do watcher existir: nenhum evento o cobre.
	rxWriteRaw(t, env, "notas/perdido.md", "Conteúdo que o watcher nunca viu.\n")
	wcStart(t, env, w)

	wcWaitFor(t, "página perdida indexada pela reconciliação", func() bool {
		return wcCount(t, env.db, `SELECT COUNT(*) FROM pages WHERE path = ?`, "notas/perdido.md") == 1
	})
}

func TestWatcherReconciliacaoDegradaAposCincoFalhasERecupera(t *testing.T) {
	env, w, logs := wcNew(t, 50*time.Millisecond, 20*time.Millisecond)
	real := w.reindex
	var calls atomic.Int64
	w.reindex = func(ctx context.Context) (ReindexReport, error) {
		if calls.Add(1) <= reconcileFailLimit {
			return ReindexReport{}, errors.New("falha injetada na reconciliação")
		}
		return real(ctx)
	}
	wcStart(t, env, w)

	wcWaitFor(t, "watcher_recovered após cinco falhas", func() bool {
		return strings.Contains(logs.String(), "msg=watcher_recovered")
	})
	out := logs.String()
	if n := strings.Count(out, "msg=watcher_degraded"); n != 1 {
		t.Fatalf("watcher_degraded registrado %d vezes, quer 1", n)
	}
	if calls.Load() <= reconcileFailLimit {
		t.Fatalf("reconciliação não continuou tentando após o degradado: %d chamadas", calls.Load())
	}
}
