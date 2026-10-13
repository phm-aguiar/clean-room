package wiki

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"time"

	"github.com/fsnotify/fsnotify"

	"github.com/phm-aguiar/ai-memory-go/internal/domain"
)

// reconcileFailLimit é o número de falhas seguidas da reconciliação que marca o watcher como degradado.
const reconcileFailLimit = 5

// Watcher observa a árvore da wiki e reindexa as páginas editadas à mão (P-08, WFL §8). Cada evento de
// `.md` entra num mapa de caminhos pendentes com debounce por caminho; ao vencer a janela, o caminho é
// indexado pelo IndexFile, o mesmo código do reindex. Remoção só registra o caminho: nunca mexe no banco.
// A reconciliação roda a cada intervalo e cobre eventos perdidos. O watcher não escreve no disco.
//
// Run é a única goroutine do watcher. A goroutine interna do fsnotify é encerrada por Close, que Run chama ao sair.
type Watcher struct {
	store     *Store
	debounce  time.Duration
	reconcile time.Duration
	reindex   func(ctx context.Context) (ReindexReport, error)
	log       *slog.Logger
	removals  atomic.Int64
	// onArmed, se não nil, é chamado uma vez depois que as observações iniciais estão registradas.
	// Existe para os testes saberem quando o watcher já está ouvindo.
	onArmed func()
}

// NewWatcher cria o watcher da wiki de s. debounce e reconcileEvery vêm de config.WatchDebounce e
// config.ReconcileInterval; são injetáveis para o teste não esperar 30 s. Usa slog.Default para os logs.
func NewWatcher(s *Store, debounce, reconcileEvery time.Duration) (*Watcher, error) {
	if s == nil || s.Writer == nil {
		return nil, errors.New("wiki: watcher exige Writer")
	}
	if err := checkRoot(s.Root); err != nil {
		return nil, fmt.Errorf("wiki: watcher: %w", err)
	}
	if debounce <= 0 || reconcileEvery <= 0 {
		return nil, errors.New("wiki: watcher exige debounce e intervalo de reconciliação positivos")
	}
	w := &Watcher{
		store:     s,
		debounce:  debounce,
		reconcile: reconcileEvery,
		log:       slog.Default(),
	}
	w.reindex = func(ctx context.Context) (ReindexReport, error) {
		return Reindex(ctx, s, s.Root)
	}
	return w, nil
}

// Removals devolve quantos caminhos removidos foram registrados desde a criação. Não altera o banco.
func (w *Watcher) Removals() int64 {
	return w.removals.Load()
}

// Run observa <root> recursivamente e processa eventos e reconciliações até ctx ser cancelado. Ao
// cancelamento devolve nil. Devolve erro só se a raiz for inválida ou o fsnotify não puder ser criado
// ou observar a raiz. A primeira reconciliação acontece depois do primeiro intervalo, não no boot.
func (w *Watcher) Run(ctx context.Context) error {
	if err := checkRoot(w.store.Root); err != nil {
		return fmt.Errorf("wiki: watcher: %w", err)
	}
	fw, err := fsnotify.NewWatcher()
	if err != nil {
		return fmt.Errorf("wiki: watcher: criar fsnotify: %w", err)
	}
	defer func() { _ = fw.Close() }()
	if err := addTree(fw, w.store.Root, w.log); err != nil {
		return fmt.Errorf("wiki: watcher: observar raiz: %w", err)
	}
	if w.onArmed != nil {
		w.onArmed()
	}

	pending := make(map[string]time.Time)
	var st reconcileState
	ticker := time.NewTicker(w.reconcile)
	defer ticker.Stop()

	for {
		var timer *time.Timer
		var fire <-chan time.Time
		if next, ok := earliest(pending); ok {
			timer = time.NewTimer(time.Until(next))
			fire = timer.C
		}
		select {
		case <-ctx.Done():
			stopTimer(timer)
			return nil
		case ev, ok := <-fw.Events:
			stopTimer(timer)
			if !ok {
				return errors.New("wiki: watcher: canal de eventos do fsnotify fechado")
			}
			w.handleEvent(fw, ev, pending, time.Now())
		case err, ok := <-fw.Errors:
			stopTimer(timer)
			if !ok {
				return errors.New("wiki: watcher: canal de erros do fsnotify fechado")
			}
			w.log.Warn("watcher_fsnotify_erro", "err", err)
		case <-fire:
			stopTimer(timer)
			w.flush(ctx, pending, time.Now())
		case <-ticker.C:
			stopTimer(timer)
			w.reconcileOnce(ctx, &st)
		}
		if ctx.Err() != nil {
			return nil
		}
	}
}

// reconcileState guarda as falhas seguidas da reconciliação e se o watcher está degradado.
type reconcileState struct {
	fails    int
	degraded bool
}

// reconcileOnce roda uma reconciliação (Reindex sobre a raiz) e atualiza o estado de falhas. Uma falha é
// erro devolvido pelo Reindex; erros por arquivo ou escopo vão para o log e não contam como falha.
func (w *Watcher) reconcileOnce(ctx context.Context, st *reconcileState) {
	rep, err := w.reindex(ctx)
	if err != nil {
		if ctx.Err() != nil {
			return
		}
		st.fails++
		if st.fails == reconcileFailLimit {
			st.degraded = true
			w.log.Error("watcher_degraded", "falhas", st.fails, "err", err)
		} else {
			w.log.Warn("watcher_reconciliacao_falhou", "falhas", st.fails, "err", err)
		}
		return
	}
	if st.degraded {
		w.log.Info("watcher_recovered", "falhas_anteriores", st.fails)
	}
	st.fails = 0
	st.degraded = false
	if n := len(rep.Errors); n > 0 {
		w.log.Warn("watcher_reconciliacao_com_erros", "erros", n, "primeiro", rep.Errors[0])
	}
}

// handleEvent aplica os filtros de ignorados (ADR-27) a um evento do fsnotify e, quando ele importa,
// põe o caminho na fila de pendentes com debounce. Diretório criado entra na observação na hora.
func (w *Watcher) handleEvent(fw *fsnotify.Watcher, ev fsnotify.Event, pending map[string]time.Time, now time.Time) {
	abs := filepath.Clean(ev.Name)
	if ev.Has(fsnotify.Create) {
		w.watchNewDir(fw, abs)
	}
	_, parts, ok := w.locate(abs)
	if !ok {
		return
	}
	if ev.Has(fsnotify.Remove) || ev.Has(fsnotify.Rename) {
		if pageFile(parts) {
			pending[abs] = now.Add(w.debounce)
		}
		return
	}
	// Criação, escrita e evento genérico: o flush confere o estado atual no disco.
	fi, err := os.Lstat(abs)
	if err != nil {
		return
	}
	if fi.IsDir() || (fi.Mode().IsRegular() && pageFile(parts)) {
		pending[abs] = now.Add(w.debounce)
	}
}

// watchNewDir acrescenta à observação um diretório recém-criado e tudo que há dentro dele.
func (w *Watcher) watchNewDir(fw *fsnotify.Watcher, abs string) {
	fi, err := os.Lstat(abs)
	if err != nil || !fi.IsDir() || fi.Mode()&os.ModeSymlink != 0 || hasReparseAttr(fi) {
		return
	}
	if err := addTree(fw, abs, w.log); err != nil {
		w.log.Warn("watcher_observar_falhou", "path", abs, "err", err)
	}
}

// flush processa os caminhos cujo prazo de debounce venceu.
func (w *Watcher) flush(ctx context.Context, pending map[string]time.Time, now time.Time) {
	for p, due := range pending {
		if due.After(now) {
			continue
		}
		delete(pending, p)
		if ctx.Err() != nil {
			return
		}
		w.process(ctx, p)
	}
}

// process decide o que fazer com um caminho pendente, olhando o disco no momento do flush:
//   - sumiu: só registra a remoção (não apaga página nem mexe no banco);
//   - diretório de projeto: reindexa o projeto inteiro (criação de diretório);
//   - `.md` regular: IndexFile, o mesmo caminho do reindex.
func (w *Watcher) process(ctx context.Context, abs string) {
	fi, err := os.Lstat(abs)
	if errors.Is(err, fs.ErrNotExist) {
		w.removals.Add(1)
		w.log.Info("watcher_remocao_registrada", "path", w.relPath(abs))
		return
	}
	if err != nil {
		w.log.Warn("watcher_verificar_falhou", "path", abs, "err", err)
		return
	}
	if fi.Mode()&os.ModeSymlink != 0 || hasReparseAttr(fi) {
		return
	}
	scope, parts, ok := w.locate(abs)
	if !ok {
		return
	}
	if fi.IsDir() {
		var rep ReindexReport
		if err := reindexScope(ctx, w.store, w.store.Root, scope, &rep); err != nil {
			return
		}
		if n := len(rep.Errors); n > 0 {
			w.log.Warn("watcher_reindex_projeto_com_erros", "erros", n, "primeiro", rep.Errors[0])
		}
		return
	}
	if !fi.Mode().IsRegular() || !pageFile(parts) {
		return
	}
	pagePath, err := domain.ParsePagePath(strings.Join(parts[2:], "/"))
	if err != nil {
		w.log.Warn("watcher_caminho_invalido", "path", w.relPath(abs), "err", err)
		return
	}
	ref := domain.NewPageRef(scope, pagePath)
	if err := IndexFile(ctx, w.store, ref, abs); err != nil {
		if ctx.Err() != nil {
			return
		}
		w.log.Warn("watcher_indexar_falhou", "page", ref.String(), "err", err)
	}
}

// locate devolve o escopo e os componentes relativos à raiz de um caminho, se ele fica em
// `<ws>/<proj>/...` com dois UUIDs canônicos e não cai em nenhum ignorado de caminho (ADR-27): `.git`
// em qualquer nível, prefixo temporário, `_pending/` e `_graveyard/` no topo do projeto.
func (w *Watcher) locate(abs string) (domain.Scope, []string, bool) {
	root := w.store.Root
	rel, err := filepath.Rel(root, abs)
	if err != nil || rel == "." || filepath.IsAbs(rel) || rel == ".." ||
		strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return domain.Scope{}, nil, false
	}
	parts := strings.Split(filepath.ToSlash(rel), "/")
	if len(parts) < 2 {
		return domain.Scope{}, nil, false
	}
	wsID, err := domain.ParseID(parts[0])
	if err != nil || wsID.String() != parts[0] {
		return domain.Scope{}, nil, false
	}
	projID, err := domain.ParseID(parts[1])
	if err != nil || projID.String() != parts[1] {
		return domain.Scope{}, nil, false
	}
	for _, p := range parts {
		if p == gitDirName || strings.HasPrefix(p, tmpPrefix) {
			return domain.Scope{}, nil, false
		}
	}
	if len(parts) >= 3 && (parts[2] == pendingDirName || parts[2] == graveyardDirName) {
		return domain.Scope{}, nil, false
	}
	scope := domain.NewScope(domain.WorkspaceID(wsID), domain.ProjectID(projID))
	return scope, parts, true
}

// relPath devolve o caminho relativo à raiz para os logs. Fora da raiz devolve o próprio caminho.
func (w *Watcher) relPath(abs string) string {
	rel, err := filepath.Rel(w.store.Root, abs)
	if err != nil {
		return abs
	}
	return filepath.ToSlash(rel)
}

// pageFile diz se os componentes (a partir da raiz) são uma página: dentro de um projeto, `.md` e fora
// dos controles da wiki (ADR-27).
func pageFile(parts []string) bool {
	if len(parts) < 3 {
		return false
	}
	name := parts[len(parts)-1]
	return filepath.Ext(name) == ".md" && !ignoredFileName(name)
}

// addTree registra a observação de top e de todos os diretórios dentro dele, pulando `.git` e pontos
// de reparse. Erro em top devolve erro; erro em subdiretório vai para o log.
func addTree(fw *fsnotify.Watcher, top string, log *slog.Logger) error {
	return filepath.WalkDir(top, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			if p == top {
				return err
			}
			log.Warn("watcher_caminhar_falhou", "path", p, "err", err)
			return nil
		}
		if !d.IsDir() {
			return nil
		}
		if d.Name() == gitDirName {
			return fs.SkipDir
		}
		if p != top {
			if fi, err := os.Lstat(p); err == nil && hasReparseAttr(fi) {
				return fs.SkipDir
			}
		}
		if err := fw.Add(p); err != nil {
			if p == top {
				return err
			}
			log.Warn("watcher_observar_falhou", "path", p, "err", err)
		}
		return nil
	})
}

// earliest devolve o menor prazo entre os pendentes, se houver algum.
func earliest(pending map[string]time.Time) (time.Time, bool) {
	var best time.Time
	found := false
	for _, due := range pending {
		if !found || due.Before(best) {
			best = due
			found = true
		}
	}
	return best, found
}

// stopTimer para o timer se ele existe. Timer já disparado não causa problema.
func stopTimer(t *time.Timer) {
	if t != nil {
		t.Stop()
	}
}
