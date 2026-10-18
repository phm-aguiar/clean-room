package ingest

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/phm-aguiar/ai-memory-go/internal/domain"
	"github.com/phm-aguiar/ai-memory-go/internal/storage"
	"github.com/phm-aguiar/ai-memory-go/internal/wiki"
)

// processEnv é o ambiente de teste: banco real com o esquema, escritor, dois projetos no mesmo
// workspace, ponteiro de projeto ativo e raiz da wiki temporária.
type processEnv struct {
	db       *sql.DB
	w        storage.Writer
	p        *Processor
	pointer  *storage.ActiveProjectPointer
	wikiRoot string
	ws       domain.WorkspaceID
	scopeA   domain.Scope
	scopeB   domain.Scope
}

var fixedNow = time.Date(2026, 10, 18, 12, 0, 0, 0, time.UTC)

func newProcessEnv(t *testing.T) *processEnv {
	t.Helper()
	ctx := context.Background()
	db, err := storage.Open(ctx, t.TempDir())
	if err != nil {
		t.Fatalf("storage.Open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	w, err := storage.NewWriter(ctx, db)
	if err != nil {
		t.Fatalf("NewWriter: %v", err)
	}
	t.Cleanup(func() { _ = w.Close() })

	ptr, err := storage.NewActiveProjectPointer(storage.ActiveProjectConfig{
		Mode:           "per_actor",
		SessionTTLSecs: int(storage.DefaultActiveProjectTTL / time.Second),
		MaxEntries:     storage.DefaultActiveProjectMaxEntries,
	}, nil)
	if err != nil {
		t.Fatalf("NewActiveProjectPointer: %v", err)
	}
	wikiRoot := t.TempDir()
	env := &processEnv{
		db:       db,
		w:        w,
		pointer:  ptr,
		wikiRoot: wikiRoot,
		ws:       domain.WorkspaceID(newTestID(t)),
	}
	env.scopeA = domain.NewScope(env.ws, domain.ProjectID(newTestID(t)))
	env.scopeB = domain.NewScope(env.ws, domain.ProjectID(newTestID(t)))
	if err := storage.EnsureScope(ctx, w, env.scopeA, "ws-1", "proj-a", fixedNow); err != nil {
		t.Fatalf("EnsureScope A: %v", err)
	}
	if err := storage.EnsureScope(ctx, w, env.scopeB, "ws-1", "proj-b", fixedNow); err != nil {
		t.Fatalf("EnsureScope B: %v", err)
	}
	env.p = &Processor{
		Writer:   w,
		Pointer:  ptr,
		WikiRoot: wikiRoot,
		Now:      func() time.Time { return fixedNow },
	}
	return env
}

func newTestID(t *testing.T) domain.ID {
	t.Helper()
	id, err := domain.NewID()
	if err != nil {
		t.Fatalf("NewID: %v", err)
	}
	return id
}

func newTestSession(t *testing.T) domain.SessionID {
	t.Helper()
	return domain.SessionID(newTestID(t))
}

func hookEvent(hook domain.HookEvent, sid domain.SessionID, key string) Event {
	return Event{
		Agent:      "claude-code",
		Hook:       hook,
		Session:    sid,
		HasSession: true,
		Cwd:        "/repo/a",
		Title:      "titulo do evento",
		Body:       "corpo do evento",
		IngestKey:  key,
	}
}

func (env *processEnv) observationCount(t *testing.T, sid domain.SessionID) int {
	t.Helper()
	var n int
	if err := env.db.QueryRow(`SELECT COUNT(*) FROM observations WHERE session_id = ?`, sid.Bytes()).Scan(&n); err != nil {
		t.Fatalf("count observations: %v", err)
	}
	return n
}

func (env *processEnv) ledgerPath(scope domain.Scope) string {
	return filepath.Join(env.wikiRoot, scope.WorkspaceID.String(), scope.ProjectID.String(), wiki.HookLogFileName(fixedNow))
}

func (env *processEnv) ledgerLines(t *testing.T, scope domain.Scope) int {
	t.Helper()
	data, err := os.ReadFile(env.ledgerPath(scope))
	if errors.Is(err, os.ErrNotExist) {
		return 0
	}
	if err != nil {
		t.Fatalf("ler ledger: %v", err)
	}
	return strings.Count(string(data), "## [")
}

func mustProcess(t *testing.T, p *Processor, req Request) Result {
	t.Helper()
	res, err := p.Process(context.Background(), req)
	if err != nil {
		t.Fatalf("Process: %v", err)
	}
	return res
}

// TestProcess_ReplayNaoDuplicaObservacaoNemLedger cobre a idempotência de replay (INV-03 e regra
// transversal de ingestão): a mesma chave não cria segunda observação nem segunda linha no ledger.
func TestProcess_ReplayNaoDuplicaObservacaoNemLedger(t *testing.T) {
	env := newProcessEnv(t)
	sid := newTestSession(t)
	req := Request{Event: hookEvent(domain.HookUserPrompt, sid, "chave-replay"), Scope: env.scopeA, Source: SourcePrefix}

	first := mustProcess(t, env.p, req)
	if first.Outcome != OutcomeRecorded {
		t.Fatalf("primeira entrega: outcome = %q, quer %q", first.Outcome, OutcomeRecorded)
	}
	if first.LedgerPath == "" {
		t.Fatal("primeira entrega: LedgerPath vazio")
	}

	second := mustProcess(t, env.p, req)
	if second.Outcome != OutcomeReplay {
		t.Fatalf("replay: outcome = %q, quer %q", second.Outcome, OutcomeReplay)
	}
	if n := env.observationCount(t, sid); n != 1 {
		t.Fatalf("observações após replay = %d, quer 1", n)
	}
	if n := env.ledgerLines(t, env.scopeA); n != 1 {
		t.Fatalf("linhas do ledger após replay = %d, quer 1", n)
	}
}

// TestProcess_GatePorProjetoEChave cobre o gate por (projeto, chave): a mesma chave em projetos
// diferentes é entrega distinta, e a mesma chave no mesmo projeto é replay.
func TestProcess_GatePorProjetoEChave(t *testing.T) {
	env := newProcessEnv(t)
	sidA := newTestSession(t)
	sidB := newTestSession(t)

	a := mustProcess(t, env.p, Request{Event: hookEvent(domain.HookUserPrompt, sidA, "mesma"), Scope: env.scopeA, Source: SourcePrefix})
	b := mustProcess(t, env.p, Request{Event: hookEvent(domain.HookUserPrompt, sidB, "mesma"), Scope: env.scopeB, Source: SourcePrefix})
	if a.Outcome != OutcomeRecorded || b.Outcome != OutcomeRecorded {
		t.Fatalf("mesma chave em projetos diferentes: outcomes %q e %q, quer ambos %q", a.Outcome, b.Outcome, OutcomeRecorded)
	}
	if env.observationCount(t, sidA) != 1 || env.observationCount(t, sidB) != 1 {
		t.Fatal("cada projeto deve gravar a sua observação")
	}

	again := mustProcess(t, env.p, Request{Event: hookEvent(domain.HookUserPrompt, sidA, "mesma"), Scope: env.scopeA, Source: SourcePrefix})
	if again.Outcome != OutcomeReplay {
		t.Fatalf("mesma chave no mesmo projeto: outcome = %q, quer %q", again.Outcome, OutcomeReplay)
	}
}

// TestProcess_EntregasConcorrentesDaMesmaChaveGravamUma roda entregas concorrentes da mesma chave
// (com -race, o gate e a transação são exercitados juntos).
func TestProcess_EntregasConcorrentesDaMesmaChaveGravamUma(t *testing.T) {
	env := newProcessEnv(t)
	sid := newTestSession(t)
	req := Request{Event: hookEvent(domain.HookUserPrompt, sid, "concorrente"), Scope: env.scopeA, Source: SourcePrefix}

	const n = 8
	var wg sync.WaitGroup
	outcomes := make(chan Outcome, n)
	errs := make(chan error, n)
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			res, err := env.p.Process(context.Background(), req)
			if err != nil {
				errs <- err
				return
			}
			outcomes <- res.Outcome
		}()
	}
	wg.Wait()
	close(outcomes)
	close(errs)
	for err := range errs {
		t.Fatalf("Process concorrente: %v", err)
	}
	recorded := 0
	for o := range outcomes {
		if o == OutcomeRecorded {
			recorded++
		}
	}
	if recorded != 1 {
		t.Fatalf("entregas concorrentes gravadas = %d, quer exatamente 1", recorded)
	}
	if got := env.observationCount(t, sid); got != 1 {
		t.Fatalf("observações = %d, quer 1", got)
	}
}

// TestProcess_FimDeSessaoDeOutroEscopoEDescartado cobre o descarte do fim de sessão que nomeia outro
// escopo, com o controle do fim de sessão no escopo da sessão.
func TestProcess_FimDeSessaoDeOutroEscopoEDescartado(t *testing.T) {
	env := newProcessEnv(t)
	sid := newTestSession(t)

	start := mustProcess(t, env.p, Request{Event: hookEvent(domain.HookSessionStart, sid, "inicio"), Scope: env.scopeA, Source: SourcePrefix})
	if start.Outcome != OutcomeRecorded {
		t.Fatalf("início: outcome = %q, quer %q", start.Outcome, OutcomeRecorded)
	}

	fimB := hookEvent(domain.HookSessionEnd, sid, "fim-b")
	fimB.Cwd = "/repo/b" // diretório diferente do armazenado na sessão ("/repo/a")
	dropped := mustProcess(t, env.p, Request{Event: fimB, Scope: env.scopeB, Source: SourceMarker})
	if dropped.Outcome != OutcomeDropped {
		t.Fatalf("fim em outro escopo: outcome = %q, quer %q", dropped.Outcome, OutcomeDropped)
	}
	if n := env.observationCount(t, sid); n != 1 {
		t.Fatalf("fim descartado gravou observação: total = %d, quer 1", n)
	}

	end := mustProcess(t, env.p, Request{Event: hookEvent(domain.HookSessionEnd, sid, "fim-a"), Scope: env.scopeA, Source: SourcePrefix})
	if end.Outcome != OutcomeRecorded {
		t.Fatalf("fim no escopo da sessão: outcome = %q, quer %q", end.Outcome, OutcomeRecorded)
	}
	if n := env.observationCount(t, sid); n != 2 {
		t.Fatalf("observações após fim no escopo da sessão = %d, quer 2", n)
	}
}

// TestProcess_FimDeSessaoDeOutroEscopoMesmoCwdEAceito cobre a exceção do fim de sessão no mesmo
// diretório (docs/storage-and-retrieval.md:210-211): o cwd do evento, com barra final, é igual ao cwd
// armazenado da sessão ("/repo/a"), então o fim é aceito e gravado no projeto nomeado.
func TestProcess_FimDeSessaoDeOutroEscopoMesmoCwdEAceito(t *testing.T) {
	env := newProcessEnv(t)
	sid := newTestSession(t)
	mustProcess(t, env.p, Request{Event: hookEvent(domain.HookSessionStart, sid, "inicio-mesmo-cwd"), Scope: env.scopeA, Source: SourcePrefix})

	fim := hookEvent(domain.HookSessionEnd, sid, "fim-mesmo-cwd")
	fim.Cwd = "/repo/a/"
	res := mustProcess(t, env.p, Request{Event: fim, Scope: env.scopeB, Source: SourceMarker})
	if res.Outcome != OutcomeRecorded {
		t.Fatalf("fim de outro escopo com mesmo cwd: outcome = %q (%s), quer %q", res.Outcome, res.Reason, OutcomeRecorded)
	}
	if n := env.observationCount(t, sid); n != 2 {
		t.Fatalf("observações após fim aceito = %d, quer 2", n)
	}
	if n := env.ledgerLines(t, env.scopeB); n != 1 {
		t.Fatalf("linhas do ledger do projeto nomeado = %d, quer 1", n)
	}
}

// TestProcess_ReplayPendenteRetomaSemNovaObservacao cobre o replay de chave pendente (docs/
// complementary-contracts.md:69): a observação já foi gravada, mas a conclusão não. A nova entrega
// retoma o ledger e a conclusão sem inserir outra observação.
func TestProcess_ReplayPendenteRetomaSemNovaObservacao(t *testing.T) {
	env := newProcessEnv(t)
	sid := newTestSession(t)
	req := Request{Event: hookEvent(domain.HookUserPrompt, sid, "chave-pendente"), Scope: env.scopeA, Source: SourcePrefix}

	first := mustProcess(t, env.p, req)
	if first.Outcome != OutcomeRecorded {
		t.Fatalf("primeira entrega: outcome = %q, quer %q", first.Outcome, OutcomeRecorded)
	}

	// Simula queda entre a admissão e a conclusão: a chave volta a pendente.
	if _, err := env.db.Exec(`UPDATE ingest_keys SET completed_at = NULL WHERE project_id = ? AND key = ?`,
		env.scopeA.ProjectID.Bytes(), "chave-pendente"); err != nil {
		t.Fatalf("zerar conclusão: %v", err)
	}
	assertKeyState(t, env, env.scopeA, "chave-pendente", storage.IngestKeyPending)

	retry := mustProcess(t, env.p, req)
	if retry.Outcome != OutcomeReplay {
		t.Fatalf("replay pendente: outcome = %q, quer %q", retry.Outcome, OutcomeReplay)
	}
	if n := env.observationCount(t, sid); n != 1 {
		t.Fatalf("replay pendente inseriu observação: total = %d, quer 1", n)
	}
	if retry.LedgerPath == "" {
		t.Fatal("replay pendente não retomou o ledger")
	}
	assertKeyState(t, env, env.scopeA, "chave-pendente", storage.IngestKeyCompleted)

	again := mustProcess(t, env.p, req)
	if again.Outcome != OutcomeReplay || again.LedgerPath != "" {
		t.Fatalf("replay após conclusão: outcome = %q, ledger = %q; quer replay sem efeito", again.Outcome, again.LedgerPath)
	}
}

// assertKeyState confere o estado da chave de ingestão dentro de uma transação de leitura.
func assertKeyState(t *testing.T, env *processEnv, scope domain.Scope, key string, want storage.IngestKeyState) {
	t.Helper()
	var got storage.IngestKeyState
	if err := env.w.Do(context.Background(), func(tx *sql.Tx) error {
		s, err := storage.IngestKeyStateTx(tx, scope.ProjectID, key)
		got = s
		return err
	}); err != nil {
		t.Fatalf("ler estado da chave: %v", err)
	}
	if got != want {
		t.Fatalf("estado da chave %q = %v, quer %v", key, got, want)
	}
}

// TestProcess_EventoDeOutroEscopoGravaNoProjetoNomeadoSemMoverSessao cobre o evento comum que aponta
// outro escopo (docs/storage-and-retrieval.md §6.1, item 2): a observação vai para o projeto que
// nomeia; a sessão continua no projeto original (docs/marker-file.md) e o ledger do projeto nomeado
// recebe a linha.
func TestProcess_EventoDeOutroEscopoGravaNoProjetoNomeadoSemMoverSessao(t *testing.T) {
	env := newProcessEnv(t)
	sid := newTestSession(t)
	mustProcess(t, env.p, Request{Event: hookEvent(domain.HookSessionStart, sid, "abre-escopo"), Scope: env.scopeA, Source: SourcePrefix})

	e := hookEvent(domain.HookUserPrompt, sid, "outro-escopo")
	e.Title = "prompt de outro escopo"
	res := mustProcess(t, env.p, Request{Event: e, Scope: env.scopeB, Source: SourceMarker})
	if res.Outcome != OutcomeRecorded {
		t.Fatalf("evento comum de outro escopo: outcome = %q, quer %q", res.Outcome, OutcomeRecorded)
	}
	if res.LedgerPath == "" {
		t.Fatal("evento de outro escopo: LedgerPath vazio")
	}

	var sessProj []byte
	if err := env.db.QueryRow(`SELECT project_id FROM sessions WHERE id = ?`, sid.Bytes()).Scan(&sessProj); err != nil {
		t.Fatalf("ler sessão: %v", err)
	}
	if !bytes.Equal(sessProj, env.scopeA.ProjectID.Bytes()) {
		t.Fatal("sessão mudou de projeto por evento de outro escopo")
	}

	var obsProj, obsWs []byte
	if err := env.db.QueryRow(`SELECT project_id, workspace_id FROM observations WHERE session_id = ? AND title = ?`,
		sid.Bytes(), "prompt de outro escopo").Scan(&obsProj, &obsWs); err != nil {
		t.Fatalf("ler observação de outro escopo: %v", err)
	}
	if !bytes.Equal(obsProj, env.scopeB.ProjectID.Bytes()) || !bytes.Equal(obsWs, env.scopeB.WorkspaceID.Bytes()) {
		t.Fatal("observação não foi gravada no projeto nomeado")
	}
	if n := env.ledgerLines(t, env.scopeB); n != 1 {
		t.Fatalf("linhas do ledger do projeto nomeado = %d, quer 1", n)
	}
	if n := env.ledgerLines(t, env.scopeA); n != 1 {
		t.Fatalf("linhas do ledger do projeto original = %d, quer 1 (só o início)", n)
	}
}

// TestProcess_EventoDeOutroEscopoEmSessaoEncerradaNaoGrava cobre o desfecho de recuperação no mesmo
// caminho: sessão já encerrada não ganha observação nova, mesmo quando o evento aponta outro escopo.
func TestProcess_EventoDeOutroEscopoEmSessaoEncerradaNaoGrava(t *testing.T) {
	env := newProcessEnv(t)
	sid := newTestSession(t)
	mustProcess(t, env.p, Request{Event: hookEvent(domain.HookSessionStart, sid, "abre-enc"), Scope: env.scopeA, Source: SourcePrefix})
	if err := env.w.Do(context.Background(), func(tx *sql.Tx) error {
		_, err := storage.CloseSessionTx(tx, sid, fixedNow)
		return err
	}); err != nil {
		t.Fatalf("CloseSessionTx: %v", err)
	}

	res := mustProcess(t, env.p, Request{Event: hookEvent(domain.HookUserPrompt, sid, "tardio-outro"), Scope: env.scopeB, Source: SourceMarker})
	if res.Outcome != OutcomeSessionEnded {
		t.Fatalf("sessão encerrada, evento de outro escopo: outcome = %q, quer %q", res.Outcome, OutcomeSessionEnded)
	}
	if n := env.observationCount(t, sid); n != 1 {
		t.Fatalf("observações após sessão encerrada = %d, quer 1", n)
	}
}

// TestProcess_AvancaPonteiroSoNoPrimeiroPlano cobre a regra de publicação do ponteiro: início de
// sessão avança o slot compartilhado; pós-ferramenta só atualiza a entrada do próprio ator.
func TestProcess_AvancaPonteiroSoNoPrimeiroPlano(t *testing.T) {
	env := newProcessEnv(t)
	actor1 := storage.ActorKey{Identity: "user-1", SessionID: "sess-1"}
	actor2 := storage.ActorKey{Identity: "user-2", SessionID: "sess-2"}

	mustProcess(t, env.p, Request{
		Event:  hookEvent(domain.HookSessionStart, newTestSession(t), "ptr-1"),
		Scope:  env.scopeA,
		Source: SourcePrefix,
		Actor:  actor1,
	})
	if got := env.pointer.Lookup(actor1, fixedNow); got.Outcome != storage.LookupResolved || got.Scope != env.scopeA {
		t.Fatalf("após início de sessão: ator 1 = %+v, quer resolvido em A", got)
	}

	mustProcess(t, env.p, Request{
		Event:  hookEvent(domain.HookPostToolUse, newTestSession(t), "ptr-2"),
		Scope:  env.scopeB,
		Source: SourcePrefix,
		Actor:  actor2,
	})
	if got := env.pointer.Lookup(actor2, fixedNow); got.Outcome != storage.LookupResolved || got.Scope != env.scopeB {
		t.Fatalf("pós-ferramenta: ator 2 = %+v, quer resolvido em B na entrada própria", got)
	}
	shared := env.pointer.Read(storage.ActorKey{}, fixedNow)
	if shared.Scope != env.scopeA {
		t.Fatalf("slot compartilhado = %v, quer inalterado em A", shared.Scope)
	}
	if got := env.pointer.Lookup(actor1, fixedNow); got.Scope != env.scopeA {
		t.Fatalf("entrada do ator 1 mudou por evento de outro ator: %+v", got)
	}
}

// TestProcess_SanitizaAntesDeGravar confirma que título e corpo passam pelo sanitizador e pelos
// tetos antes da gravação, e que o ledger recebe só o título sanitizado.
func TestProcess_SanitizaAntesDeGravar(t *testing.T) {
	env := newProcessEnv(t)
	sid := newTestSession(t)
	e := hookEvent(domain.HookUserPrompt, sid, "sanitize")
	e.Title = strings.Repeat("a", 200)
	e.Body = "use o token sk-AAAAAAAAAAAAAAAAAAAAAAAA para testar"

	mustProcess(t, env.p, Request{Event: e, Scope: env.scopeA, Source: SourcePrefix})

	var title, body string
	if err := env.db.QueryRow(`SELECT title, body FROM observations WHERE session_id = ?`, sid.Bytes()).Scan(&title, &body); err != nil {
		t.Fatalf("ler observação: %v", err)
	}
	if n := len([]rune(title)); n > 80 {
		t.Fatalf("título gravado com %d runes, quer no máximo 80", n)
	}
	if strings.Contains(body, "sk-AAAA") {
		t.Fatalf("segredo gravado no corpo: %q", body)
	}
	data, err := os.ReadFile(env.ledgerPath(env.scopeA))
	if err != nil {
		t.Fatalf("ler ledger: %v", err)
	}
	if strings.Contains(string(data), "sk-AAAA") {
		t.Fatal("segredo chegou ao ledger")
	}
}

// TestProcess_SessaoEncerradaNaoRecebeObservacao cobre o desfecho de recuperação: a sessão já
// encerrada não ganha observação nova, e a chave é concluída.
func TestProcess_SessaoEncerradaNaoRecebeObservacao(t *testing.T) {
	env := newProcessEnv(t)
	sid := newTestSession(t)
	mustProcess(t, env.p, Request{Event: hookEvent(domain.HookSessionStart, sid, "abre"), Scope: env.scopeA, Source: SourcePrefix})

	if err := env.w.Do(context.Background(), func(tx *sql.Tx) error {
		_, err := storage.CloseSessionTx(tx, sid, fixedNow)
		return err
	}); err != nil {
		t.Fatalf("CloseSessionTx: %v", err)
	}

	res := mustProcess(t, env.p, Request{Event: hookEvent(domain.HookUserPrompt, sid, "tardio"), Scope: env.scopeA, Source: SourcePrefix})
	if res.Outcome != OutcomeSessionEnded {
		t.Fatalf("sessão encerrada: outcome = %q, quer %q", res.Outcome, OutcomeSessionEnded)
	}
	if n := env.observationCount(t, sid); n != 1 {
		t.Fatalf("observações após sessão encerrada = %d, quer 1", n)
	}
}

// TestProcess_SemSessaoSoAceitaInicio cobre o identificador ausente: início de sessão ganha um
// identificador novo; outro evento é recusado.
func TestProcess_SemSessaoSoAceitaInicio(t *testing.T) {
	env := newProcessEnv(t)

	_, err := env.p.Process(context.Background(), Request{
		Event: Event{Agent: "claude-code", Hook: domain.HookUserPrompt, Title: "x", IngestKey: "sem-sessao"},
		Scope: env.scopeA,
	})
	if !errors.Is(err, ErrNoSession) {
		t.Fatalf("prompt sem sessão: erro = %v, quer ErrNoSession", err)
	}

	res := mustProcess(t, env.p, Request{
		Event: Event{Agent: "claude-code", Hook: domain.HookSessionStart, Title: "inicio", IngestKey: "inicio-novo"},
		Scope: env.scopeA,
	})
	if res.Outcome != OutcomeRecorded || res.SessionID == (domain.SessionID{}) {
		t.Fatalf("início sem sessão: outcome = %q, sessão = %v; quer gravado com id novo", res.Outcome, res.SessionID)
	}
}

// TestKeyGate_SerializaSoAMesmaChave cobre o gate isolado: chaves diferentes não esperam, a mesma
// chave espera a liberação, e cancelar a espera não deixa entrada presa.
func TestKeyGate_SerializaSoAMesmaChave(t *testing.T) {
	var g keyGate
	ctx := context.Background()
	pa := domain.ProjectID{1}
	pb := domain.ProjectID{2}
	ka := gateKey{project: pa, key: "k"}

	rel, err := g.acquire(ctx, ka)
	if err != nil {
		t.Fatalf("acquire: %v", err)
	}

	relOther, err := g.acquire(ctx, gateKey{project: pb, key: "k"})
	if err != nil {
		t.Fatalf("acquire de outro projeto: %v", err)
	}
	relOther()

	done := make(chan struct{})
	go func() {
		r, err := g.acquire(ctx, ka)
		if err == nil {
			r()
		}
		close(done)
	}()
	select {
	case <-done:
		t.Fatal("mesma chave adquirida enquanto estava segurada")
	case <-time.After(50 * time.Millisecond):
	}
	rel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("espera não foi liberada")
	}

	held, err := g.acquire(ctx, ka)
	if err != nil {
		t.Fatalf("acquire: %v", err)
	}
	cctx, cancel := context.WithCancel(ctx)
	cancel()
	if _, err := g.acquire(cctx, ka); !errors.Is(err, context.Canceled) {
		t.Fatalf("espera cancelada: erro = %v, quer context.Canceled", err)
	}
	held()
	if len(g.slots) != 0 {
		t.Fatalf("entradas presas no gate: %d", len(g.slots))
	}
}
