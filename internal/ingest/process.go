package ingest

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/phm-aguiar/ai-memory-go/internal/domain"
	"github.com/phm-aguiar/ai-memory-go/internal/domain/sanitize"
	"github.com/phm-aguiar/ai-memory-go/internal/storage"
	"github.com/phm-aguiar/ai-memory-go/internal/wiki"
)

// Processamento de um evento de ciclo de vida (docs/lifecycle-event-ingestion.md §8, §8.1, §8.3 e
// §13; docs/auto-scope.md "Publishing rules" e "Session-aware clients"; INV-03 e INV-16).
//
// Ordem do processamento:
//  1. identificador de sessão (novo para início sem sessão; sem sessão em outro evento recusa);
//  2. gate em memória por (projeto, chave de ingestão), que serializa entregas da mesma chave;
//  3. uma transação (Writer.Do): abre ou toca a sessão, reivindica a chave e grava a observação
//     sanitizada (INV-03: sessão, chave e observação na mesma escrita);
//  4. desfecho da admissão: replay de chave concluída não faz nada; replay de chave pendente retoma só
//     o trabalho a jusante, sem observação nova (docs/complementary-contracts.md:69); fim de sessão de
//     outro escopo é descartado, salvo se o cwd do evento for o cwd armazenado da sessão
//     (docs/storage-and-retrieval.md:210-211); evento comum de outro escopo grava no projeto nomeado
//     sem mover a sessão; sessão já encerrada não ganha observação nova;
//  5. ponteiro de projeto ativo (só eventos de primeiro plano avançam o compartilhado);
//  6. linha no ledger mensal do projeto (acréscimo);
//  7. conclusão da chave, no fim, em outra transação.
//
// Efeitos de fim de sessão (página, handoff e encerramento) não estão aqui: são da T029.

// ErrNoSession é devolvido para um evento sem identificador de sessão que não é início de sessão
// (docs/lifecycle-event-ingestion.md §8 passo 1).
var ErrNoSession = errors.New("evento sem identificador de sessão fora do início de sessão")

// Outcome é o desfecho do processamento de um evento.
type Outcome string

const (
	// OutcomeRecorded: observação nova gravada, com sessão e chave na mesma transação.
	OutcomeRecorded Outcome = "recorded"
	// OutcomeReplay: a chave já estava reivindicada. Nenhuma observação nova. Com chave concluída, nada
	// mais é feito; com chave pendente, o ponteiro, a linha do ledger e a conclusão são retomados.
	OutcomeReplay Outcome = "replay"
	// OutcomeSessionEnded: a sessão já estava encerrada. A chave é concluída sem observação nova.
	OutcomeSessionEnded Outcome = "session-ended"
	// OutcomeDropped: evento descartado sem efeito (fim de sessão de outro escopo, ou escopo
	// divergente da sessão). Reason diz o motivo.
	OutcomeDropped Outcome = "dropped"
)

// Result é o retorno de Process. LedgerPath só vem preenchido quando o ledger foi acrescido.
type Result struct {
	Outcome    Outcome
	SessionID  domain.SessionID
	Reason     string
	LedgerPath string
}

// Request são as entradas já resolvidas de um evento. A resolução de escopo (ingest/resolve) e o
// ator são feitos por quem chama (router, T031).
type Request struct {
	Event Event
	// Scope é o escopo resolvido do evento.
	Scope domain.Scope
	// Source é a regra que levou ao escopo (SourceMarker, SourceSticky, ...). Decide a publicação
	// do ponteiro junto com Event.Cwd.
	Source Source
	// DefaultGlobal é a preferência default_global do marcador, levada para o ponteiro.
	DefaultGlobal bool
	// Actor é a coordenada do chamador usada pelo ponteiro (identidade e sessão).
	Actor storage.ActorKey
	// OwnerUser é o dono do registro. Vazio quando o servidor não distingue operadores.
	OwnerUser string
}

// ErrProcessorNotReady é devolvido quando Processor não tem escritor ou raiz da wiki.
var ErrProcessorNotReady = errors.New("processador de ingestão sem escritor ou raiz da wiki")

// Processor orquestra o processamento de eventos. Um Processor é compartilhado pelas goroutines
// do servidor; o gate interno serializa só a mesma (projeto, chave).
type Processor struct {
	// Writer é o escritor único de SQLite (INV-02).
	Writer storage.Writer
	// Pointer é o ponteiro de projeto ativo. Nil desliga a publicação.
	Pointer *storage.ActiveProjectPointer
	// Sanitizer aplica a remoção de segredos antes de qualquer gravação (INV-06). Zero value usa os
	// padrões embutidos.
	Sanitizer sanitize.Sanitizer
	// WikiRoot é a raiz da wiki; o ledger do projeto fica em <raiz>/<workspace_id>/<project_id>.
	WikiRoot string
	// Now é o relógio; nil usa time.Now. Injetável para testes.
	Now func() time.Time

	gate keyGate
}

// Process processa um evento (ver a ordem no topo do arquivo).
func (p *Processor) Process(ctx context.Context, req Request) (Result, error) {
	if p.Writer == nil || p.WikiRoot == "" {
		return Result{}, ErrProcessorNotReady
	}
	ev := req.Event
	if !ev.HasSession {
		if ev.Hook != domain.HookSessionStart {
			return Result{}, ErrNoSession
		}
		id, err := domain.NewID()
		if err != nil {
			return Result{}, fmt.Errorf("processar evento: nova sessão: %w", err)
		}
		ev.Session = domain.SessionID(id)
		ev.HasSession = true
	}

	// Sem chave válida não há deduplicação: a entrega é tratada como nova (at-least-once, §3.3).
	if ev.IngestKey == "" {
		return p.run(ctx, req, ev)
	}
	release, err := p.gate.acquire(ctx, gateKey{project: req.Scope.ProjectID, key: ev.IngestKey})
	if err != nil {
		return Result{}, fmt.Errorf("processar evento: gate: %w", err)
	}
	defer release()
	return p.run(ctx, req, ev)
}

// run executa os passos 3 a 7. Chamado com o gate da chave já segurado.
func (p *Processor) run(ctx context.Context, req Request, ev Event) (Result, error) {
	at := p.now()
	res := Result{SessionID: ev.Session}
	claimed := false
	pending := false
	var recordedTitle string

	err := p.Writer.Do(ctx, func(tx *sql.Tx) error {
		touch, err := storage.OpenOrTouchSessionTx(tx, storage.SessionOpen{
			ID:        ev.Session,
			Scope:     req.Scope,
			AgentKind: ev.Agent,
			Cwd:       ev.Cwd,
			ActorUser: req.OwnerUser,
			At:        at,
		})
		ended := touch.Ended
		if errors.Is(err, storage.ErrSessionScope) {
			// Escopo divergente da sessão (docs/auto-scope.md). Fim de sessão de outro escopo só é aceito
			// se o cwd do evento for o cwd armazenado da sessão (docs/storage-and-retrieval.md:211);
			// caso contrário é descartado sem gravar nada.
			if ev.Hook == domain.HookSessionEnd {
				same, err := sameSessionCwdTx(tx, ev)
				if err != nil {
					return err
				}
				if !same {
					res.Outcome = OutcomeDropped
					res.Reason = "fim de sessão aponta outro escopo"
					return nil
				}
			}
			// Evento comum (ou fim aceito pelo mesmo diretório) de outro escopo: a observação é gravada
			// no projeto que nomeia (docs/storage-and-retrieval.md §6.1, item 2). A sessão fica no projeto
			// original (docs/marker-file.md): OpenOrTouchSessionTx parou antes de escrever qualquer coisa.
			// O estado de encerramento vem de leitura direta, porque o retorno não o traz neste caminho.
			ended, err = sessionEndedTx(tx, ev.Session)
			if err != nil {
				return err
			}
		} else if err != nil {
			return err
		}

		if ev.IngestKey != "" {
			ok, err := storage.ClaimIngestKeyTx(tx, req.Scope.ProjectID, ev.IngestKey, at)
			if err != nil {
				return err
			}
			if !ok {
				// Chave já existe: concluída ou pendente. Concluída é replay sem efeito. Pendente (a
				// observação já foi gravada, falta o trabalho a jusante) retoma só o ponteiro, o ledger e a
				// conclusão, sem inserir outra observação (docs/complementary-contracts.md:69).
				state, err := storage.IngestKeyStateTx(tx, req.Scope.ProjectID, ev.IngestKey)
				if err != nil {
					return err
				}
				res.Outcome = OutcomeReplay
				if state != storage.IngestKeyPending {
					res.Reason = "chave já concluída"
					return nil
				}
				pending = true
				res.Reason = "replay pendente: retoma o trabalho a jusante"
				recordedTitle = p.Sanitizer.Title(ev.Title, sanitize.Limits{TitleMaxRunes: sanitize.ObservationTitleMaxRunes}).Text()
				return nil
			}
			claimed = true
		}

		if ended {
			// Recuperação (§8 passo 7): sessão já encerrada não recebe observação nova.
			res.Outcome = OutcomeSessionEnded
			res.Reason = "sessão já encerrada"
			return nil
		}

		title, err := p.recordObservation(tx, req, ev, at)
		if err != nil {
			return err
		}
		recordedTitle = title
		res.Outcome = OutcomeRecorded
		return nil
	})
	if err != nil {
		return Result{}, fmt.Errorf("processar evento: admissão: %w", err)
	}
	if res.Outcome == OutcomeRecorded || pending {
		p.publish(req, ev, at)
		path, err := p.appendLedger(req.Scope, at, ev.Hook, recordedTitle)
		if err != nil {
			return res, fmt.Errorf("processar evento: ledger: %w", err)
		}
		res.LedgerPath = path
	}
	if pending || (claimed && (res.Outcome == OutcomeRecorded || res.Outcome == OutcomeSessionEnded)) {
		if err := p.complete(ctx, req.Scope, ev.IngestKey, at); err != nil {
			return res, err
		}
	}
	return res, nil
}

// sessionEndedTx diz se a sessão já está encerrada. Só é usada quando o evento aponta outro escopo:
// nesse caminho OpenOrTouchSessionTx não devolve o estado de encerramento.
func sessionEndedTx(tx *sql.Tx, id domain.SessionID) (bool, error) {
	var endedAt sql.NullInt64
	if err := tx.QueryRow(`SELECT ended_at FROM sessions WHERE id = ?`, id.Bytes()).Scan(&endedAt); err != nil {
		return false, fmt.Errorf("ler encerramento da sessão: %w", err)
	}
	return endedAt.Valid, nil
}

// sameSessionCwdTx diz se o cwd do evento é o cwd de trabalho armazenado da sessão. Comparação pelo
// mesmo normPath do resolve (separador invertido e barra final). Cwd vazio de qualquer lado não casa.
func sameSessionCwdTx(tx *sql.Tx, ev Event) (bool, error) {
	stored, err := storage.SessionCwd(tx, ev.Session)
	if err != nil {
		return false, fmt.Errorf("cwd da sessão: %w", err)
	}
	if ev.Cwd == "" || stored == "" {
		return false, nil
	}
	return normPath(ev.Cwd) == normPath(stored), nil
}

// recordObservation grava a observação sanitizada dentro da transação de admissão e devolve o
// título já sanitizado, usado no ledger.
func (p *Processor) recordObservation(tx *sql.Tx, req Request, ev Event, at time.Time) (string, error) {
	t := p.Sanitizer.Title(ev.Title, sanitize.Limits{TitleMaxRunes: sanitize.ObservationTitleMaxRunes})
	b := p.Sanitizer.Body(ev.Body, sanitize.Limits{BodyMaxBytes: bodyLimitFor(ev.Hook)})

	id, err := domain.NewID()
	if err != nil {
		return "", fmt.Errorf("observação: id: %w", err)
	}
	obs, err := domain.NewObservation(
		domain.ObservationID(id), ev.Session, req.Scope,
		kindFor(ev.Hook), t, b, importanceFor(ev.Hook),
	)
	if err != nil {
		return "", fmt.Errorf("observação: %w", err)
	}
	if !ev.OccurredAt.IsZero() {
		obs.OccurredAt = ev.OccurredAt.UnixMicro()
	}
	if err := storage.InsertObservation(tx, obs); err != nil {
		return "", err
	}
	return t.Text(), nil
}

// publish avança o ponteiro de projeto ativo quando o evento traz diretório ou sobreposição
// (docs/auto-scope.md "Publishing rules"). Eventos de primeiro plano também escrevem o slot
// compartilhado; os demais só atualizam a entrada do próprio ator.
func (p *Processor) publish(req Request, ev Event, at time.Time) {
	if p.Pointer == nil {
		return
	}
	if ev.Cwd == "" && req.Source != SourceMarker && req.Source != SourceSticky {
		return
	}
	p.Pointer.Publish(req.Actor, req.Scope, req.DefaultGlobal, foregroundEvent(ev.Hook), at)
}

// foregroundEvent diz se o evento avança o slot compartilhado: início de sessão, prompt e
// pré-ferramenta (docs/auto-scope.md).
func foregroundEvent(h domain.HookEvent) bool {
	switch h {
	case domain.HookSessionStart, domain.HookUserPrompt, domain.HookPreToolUse:
		return true
	}
	return false
}

// appendLedger acrescenta a linha do evento ao ledger mensal do projeto (§13). O título já vem
// sanitizado, então o ledger nunca recebe o texto cru.
func (p *Processor) appendLedger(scope domain.Scope, at time.Time, h domain.HookEvent, title string) (string, error) {
	dir := filepath.Join(p.WikiRoot, scope.WorkspaceID.String(), scope.ProjectID.String())
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("ledger: diretório: %w", err)
	}
	return wiki.AppendHookLog(dir, at, h, title)
}

// complete conclui a chave em outra transação (passo 7, "no final"). Chave concluída é no-op.
func (p *Processor) complete(ctx context.Context, scope domain.Scope, key string, at time.Time) error {
	err := p.Writer.Do(ctx, func(tx *sql.Tx) error {
		_, err := storage.CompleteIngestKeyTx(tx, scope.ProjectID, key, at)
		return err
	})
	if err != nil {
		return fmt.Errorf("concluir chave: %w", err)
	}
	return nil
}

func (p *Processor) now() time.Time {
	if p.Now != nil {
		return p.Now()
	}
	return time.Now()
}

// bodyLimitFor é o teto do corpo depois da sanitização (docs/lifecycle-event-ingestion.md §3.6).
func bodyLimitFor(h domain.HookEvent) int {
	switch h {
	case domain.HookNotification, domain.HookPreToolUse, domain.HookPostToolUse, domain.HookStop, domain.HookOther:
		return smallBodyMaxBytes
	}
	return sanitize.ObservationBodyMaxBytes
}

// smallBodyMaxBytes é o teto de 2 KiB dos eventos pequenos (§3.6).
const smallBodyMaxBytes = 2 * 1024

// importanceFor é a importância por evento (docs/lifecycle-event-ingestion.md §3.2).
func importanceFor(h domain.HookEvent) int {
	switch h {
	case domain.HookSessionStart, domain.HookSessionEnd:
		return 7
	case domain.HookUserPrompt:
		return 8
	case domain.HookPreToolUse, domain.HookPostToolUse:
		return 5
	case domain.HookStop, domain.HookPreCompact, domain.HookPostCompaction:
		return 6
	}
	return 3
}

// kindFor mapeia o evento para o tipo de observação (§3.1): subagentes e "outro" viram "outro";
// os demais mantêm o mesmo nome em kebab-case.
func kindFor(h domain.HookEvent) domain.ObservationKind {
	switch h {
	case domain.HookSubagentStart, domain.HookSubagentStop:
		return domain.ObservationOther
	}
	return domain.ObservationKind(h)
}

// gateKey identifica a entrega: (projeto, chave de ingestão).
type gateKey struct {
	project domain.ProjectID
	key     string
}

// gateSlot é o semáforo de uma chave em uso. refs conta quem segura ou espera; ao zerar, a entrada
// é removida, então o mapa só guarda chaves ativas.
type gateSlot struct {
	sem  chan struct{}
	refs int
}

// keyGate serializa o processamento de uma mesma (projeto, chave) e deixa as demais correr livres.
// O zero value está pronto para uso.
type keyGate struct {
	mu    sync.Mutex
	slots map[gateKey]*gateSlot
}

// acquire espera a vez da chave e devolve a função de liberação. Cancelar ctx enquanto espera
// devolve o erro de ctx sem segurar a chave.
func (g *keyGate) acquire(ctx context.Context, k gateKey) (func(), error) {
	s := g.enter(k)
	select {
	case s.sem <- struct{}{}:
	case <-ctx.Done():
		g.leave(k)
		return nil, ctx.Err()
	}
	return func() {
		<-s.sem
		g.leave(k)
	}, nil
}

func (g *keyGate) enter(k gateKey) *gateSlot {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.slots == nil {
		g.slots = make(map[gateKey]*gateSlot)
	}
	s, ok := g.slots[k]
	if !ok {
		s = &gateSlot{sem: make(chan struct{}, 1)}
		g.slots[k] = s
	}
	s.refs++
	return s
}

func (g *keyGate) leave(k gateKey) {
	g.mu.Lock()
	defer g.mu.Unlock()
	s := g.slots[k]
	s.refs--
	if s.refs == 0 {
		delete(g.slots, k)
	}
}
