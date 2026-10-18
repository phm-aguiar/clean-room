package storage

import (
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/phm-aguiar/ai-memory-go/internal/domain"
)

// Ponteiro de projeto ativo em memória (docs/domain-vocabulary.md §9; docs/auto-scope.md).
//
// Os hooks publicam o projeto em que o agente trabalha; as ferramentas MCP leem
// esse projeto quando o chamador não informa workspace e projeto. Nada aqui é
// persistido: reiniciar o processo esvazia o ponteiro (a semente de startup
// cobre só a leitura).

// ActiveProjectMode escolhe como o ponteiro é chaveado (DV §9.1).
type ActiveProjectMode string

const (
	// ActiveProjectModeSingle: slot global; a última publicação vence.
	ActiveProjectModeSingle ActiveProjectMode = "single"
	// ActiveProjectModePerSession: chave é o identificador de sessão.
	ActiveProjectModePerSession ActiveProjectMode = "per_session"
	// ActiveProjectModePerActor: chave é (identidade, sessão), com slot só de identidade ao lado.
	ActiveProjectModePerActor ActiveProjectMode = "per_actor"
)

// DefaultActiveProjectMode é o modo padrão (DV §9.1: per_actor).
const DefaultActiveProjectMode = ActiveProjectModePerActor

const (
	// DefaultActiveProjectTTL é o TTL de uma entrada chaveada (DV §9.3: 1 hora).
	DefaultActiveProjectTTL = time.Hour
	// DefaultActiveProjectMaxEntries é o teto de entradas chaveadas (DV §9.3: 4096).
	DefaultActiveProjectMaxEntries = 4096
)

// ErrActiveProjectMode é devolvido por NewActiveProjectPointer quando o modo não é um dos três válidos.
var ErrActiveProjectMode = errors.New("modo de projeto ativo inválido")

// ActiveProjectConfig são as configurações de [auto_scope] usadas pelo ponteiro.
// Valores zero seguem docs/auto-scope.md: SessionTTLSecs 0 vira 1 hora e MaxEntries 0 vira 1.
// Mode vazio é inválido (não cai em single nem em per_actor em silêncio).
type ActiveProjectConfig struct {
	Mode           string
	SessionTTLSecs int
	MaxEntries     int
}

// ActorKey é a coordenada do chamador: identidade de armazenamento e identificador de sessão.
// Os dois vazios significam chamador sem coordenada (usa o slot compartilhado).
type ActorKey struct {
	Identity  string
	SessionID string
}

// LookupOutcome é o resultado de Lookup, consulta de escrita (DV §9.4).
type LookupOutcome string

const (
	// LookupResolved: o ponteiro nomeia um projeto.
	LookupResolved LookupOutcome = "resolved"
	// LookupMismatch: o chamador tem coordenada, o ponteiro está ativo e nada casou. Falha fechada.
	LookupMismatch LookupOutcome = "mismatch"
	// LookupUnset: não há informação de ponteiro para este chamador.
	LookupUnset LookupOutcome = "unset"
)

// ReadOrigin diz de onde veio a resposta de Read (DV §9.4).
type ReadOrigin string

const (
	// ReadOriginSession: entrada própria do chamador.
	ReadOriginSession ReadOrigin = "session"
	// ReadOriginSharedSlot: slot de processo (último projeto publicado).
	ReadOriginSharedSlot ReadOrigin = "shared_slot"
	// ReadOriginStartupSeed: semente de inicialização, só para leitura.
	ReadOriginStartupSeed ReadOrigin = "startup_seed"
	// ReadOriginMismatch: divergência; o chamador usa o padrão configurado.
	ReadOriginMismatch ReadOrigin = "mismatch"
	// ReadOriginUnset: sem informação de ponteiro.
	ReadOriginUnset ReadOrigin = "unset"
)

// LookupResult é a resposta de Lookup. Scope e DefaultGlobal só valem quando Outcome é LookupResolved.
type LookupResult struct {
	Outcome       LookupOutcome
	Scope         domain.Scope
	DefaultGlobal bool
}

// ReadResult é a resposta de Read. Scope e DefaultGlobal só valem quando Origin é Session,
// SharedSlot ou StartupSeed.
type ReadResult struct {
	Origin        ReadOrigin
	Scope         domain.Scope
	DefaultGlobal bool
}

// pointerValue é o que o ponteiro guarda de um projeto publicado.
type pointerValue struct {
	scope         domain.Scope
	defaultGlobal bool
}

// pointerKey é a chave de uma entrada chaveada. Em per_actor, (identidade, "") é o slot só de identidade;
// em per_session, a chave é ("", sessão).
type pointerKey struct {
	identity  string
	sessionID string
}

// pointerEntry é uma entrada chaveada. seq ordena as inserções para a remoção por teto.
type pointerEntry struct {
	value pointerValue
	at    time.Time
	seq   uint64
}

// ActiveProjectPointer é o ponteiro de projeto ativo do processo. É seguro para uso concorrente.
// Todos os métodos recebem o instante atual como parâmetro, para que o TTL seja testável sem dormir.
type ActiveProjectPointer struct {
	mu sync.Mutex

	mode ActiveProjectMode
	ttl  time.Duration
	max  int

	keyed map[pointerKey]pointerEntry
	seq   uint64

	// shared é o slot de processo; nil até a primeira publicação de primeiro plano. Não expira.
	shared *pointerValue
	// seed é a semente de startup. Só Read a usa, e só enquanto shared é nil.
	seed *pointerValue
	// everKeyed é a marca "ponteiro ativo": alguma entrada chaveada já foi publicada. Nunca é apagada.
	everKeyed bool
}

// NewActiveProjectPointer valida a configuração, aplica os valores zero e registra o modo efetivo
// no log com a mensagem "active-project isolation mode". Modo inválido devolve erro envolvendo
// ErrActiveProjectMode, e o chamador deve impedir a subida do servidor. logger nil usa slog.Default.
func NewActiveProjectPointer(cfg ActiveProjectConfig, logger *slog.Logger) (*ActiveProjectPointer, error) {
	mode := ActiveProjectMode(cfg.Mode)
	switch mode {
	case ActiveProjectModeSingle, ActiveProjectModePerSession, ActiveProjectModePerActor:
	default:
		return nil, fmt.Errorf("%w: %q (use single, per_session ou per_actor)", ErrActiveProjectMode, cfg.Mode)
	}

	ttl := DefaultActiveProjectTTL
	if cfg.SessionTTLSecs != 0 {
		ttl = time.Duration(cfg.SessionTTLSecs) * time.Second
	}
	max := cfg.MaxEntries
	if max <= 0 {
		max = 1
	}
	if logger == nil {
		logger = slog.Default()
	}

	p := &ActiveProjectPointer{
		mode:  mode,
		ttl:   ttl,
		max:   max,
		keyed: make(map[pointerKey]pointerEntry),
	}
	logger.Info("active-project isolation mode",
		"mode", string(p.mode),
		"session_ttl_secs", int64(p.ttl/time.Second),
		"max_entries", p.max,
	)
	return p, nil
}

// Mode devolve o modo efetivo.
func (p *ActiveProjectPointer) Mode() ActiveProjectMode {
	return p.mode
}

// Publish registra um projeto publicado por um evento de hook. Eventos de primeiro plano
// (início de sessão, prompt, pré-ferramenta) gravam também o slot compartilhado; em per_actor,
// com identidade e sessão juntas, gravam também o slot só de identidade. Eventos de conclusão
// atualizam só a entrada chaveada do próprio ator. Chamador sem coordenada grava só o slot
// compartilhado, e só em primeiro plano.
func (p *ActiveProjectPointer) Publish(actor ActorKey, scope domain.Scope, defaultGlobal bool, foreground bool, now time.Time) {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.sweepLocked(now)
	v := pointerValue{scope: scope, defaultGlobal: defaultGlobal}

	if foreground {
		shared := v
		p.shared = &shared
	}
	if p.mode == ActiveProjectModeSingle {
		return
	}

	key, ok := p.keyFor(actor)
	if !ok {
		return
	}
	p.putLocked(key, v, now)

	if foreground && p.mode == ActiveProjectModePerActor && actor.Identity != "" && actor.SessionID != "" {
		p.putLocked(pointerKey{identity: actor.Identity}, v, now)
	}
}

// Lookup responde a consulta de escrita (DV §9.4). Nunca usa a semente de startup, e nunca responde
// com o slot de outra sessão quando há divergência.
func (p *ActiveProjectPointer) Lookup(actor ActorKey, now time.Time) LookupResult {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.sweepLocked(now)
	outcome, v, _ := p.lookupLocked(actor)
	if outcome != LookupResolved {
		return LookupResult{Outcome: outcome}
	}
	return LookupResult{Outcome: LookupResolved, Scope: v.scope, DefaultGlobal: v.defaultGlobal}
}

// Read responde a consulta de leitura e informa a origem (DV §9.4). Só Unset cai na semente de startup,
// e só quando o slot compartilhado está vazio. Divergência nunca usa a semente.
func (p *ActiveProjectPointer) Read(actor ActorKey, now time.Time) ReadResult {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.sweepLocked(now)
	outcome, v, src := p.lookupLocked(actor)
	switch outcome {
	case LookupResolved:
		origin := ReadOriginSharedSlot
		if src == lookupFromSession {
			origin = ReadOriginSession
		}
		return ReadResult{Origin: origin, Scope: v.scope, DefaultGlobal: v.defaultGlobal}
	case LookupMismatch:
		return ReadResult{Origin: ReadOriginMismatch}
	}
	if p.seed != nil {
		return ReadResult{Origin: ReadOriginStartupSeed, Scope: p.seed.scope, DefaultGlobal: p.seed.defaultGlobal}
	}
	return ReadResult{Origin: ReadOriginUnset}
}

// SeedStartup define a semente de startup (projeto da observação mais recente dentro do TTL).
// A semente é só para leitura e nunca é gravada no ponteiro.
func (p *ActiveProjectPointer) SeedStartup(scope domain.Scope) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.seed = &pointerValue{scope: scope}
}

// ClearStartupSeed remove a semente. Invalidações administrativas (purga, mudança ou remoção de escopo)
// chamam esta função para que um escopo removido não continue respondendo pela semente.
func (p *ActiveProjectPointer) ClearStartupSeed() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.seed = nil
}

// lookupSource diz de onde veio um Resolved, para Read informar a origem.
type lookupSource int

const (
	lookupFromSession lookupSource = iota
	lookupFromShared
)

// lookupLocked aplica as regras de Lookup (docs/auto-scope.md, "Lookup rules"). Chamador com mutex travado.
func (p *ActiveProjectPointer) lookupLocked(actor ActorKey) (LookupOutcome, pointerValue, lookupSource) {
	key, ok := p.keyFor(actor)
	if !ok {
		return p.sharedLocked()
	}
	// Regra 2: entrada própria viva.
	if e, ok := p.keyed[key]; ok {
		return LookupResolved, e.value, lookupFromSession
	}
	// Regra 3: identidade com entrada própria ausente e slot só de identidade vivo.
	if p.mode == ActiveProjectModePerActor && key.identity != "" {
		if _, ok := p.keyed[pointerKey{identity: key.identity}]; ok {
			return LookupMismatch, pointerValue{}, 0
		}
	}
	// Regra 4: alguma entrada chaveada já foi publicada nesta instalação.
	if p.everKeyed {
		return LookupMismatch, pointerValue{}, 0
	}
	// Regra 5: nada chaveado jamais; responde pelo slot compartilhado.
	return p.sharedLocked()
}

func (p *ActiveProjectPointer) sharedLocked() (LookupOutcome, pointerValue, lookupSource) {
	if p.shared == nil {
		return LookupUnset, pointerValue{}, 0
	}
	return LookupResolved, *p.shared, lookupFromShared
}

// keyFor devolve a chave do ator no modo atual. ok é false quando o ator não tem coordenada útil
// para o modo (então a consulta usa o slot compartilhado).
func (p *ActiveProjectPointer) keyFor(actor ActorKey) (pointerKey, bool) {
	switch p.mode {
	case ActiveProjectModePerSession:
		if actor.SessionID == "" {
			return pointerKey{}, false
		}
		return pointerKey{sessionID: actor.SessionID}, true
	case ActiveProjectModePerActor:
		if actor.Identity == "" && actor.SessionID == "" {
			return pointerKey{}, false
		}
		return pointerKey{identity: actor.Identity, sessionID: actor.SessionID}, true
	default:
		return pointerKey{}, false
	}
}

// putLocked grava uma entrada chaveada e aplica o teto, removendo as inserções mais antigas.
func (p *ActiveProjectPointer) putLocked(key pointerKey, v pointerValue, now time.Time) {
	p.seq++
	p.keyed[key] = pointerEntry{value: v, at: now, seq: p.seq}
	p.everKeyed = true
	for len(p.keyed) > p.max {
		var oldest pointerKey
		var oldestSeq uint64
		first := true
		for k, e := range p.keyed {
			if first || e.seq < oldestSeq {
				oldest, oldestSeq, first = k, e.seq, false
			}
		}
		delete(p.keyed, oldest)
	}
}

// sweepLocked remove entradas vencidas. Não toca em shared, seed nem everKeyed.
func (p *ActiveProjectPointer) sweepLocked(now time.Time) {
	for k, e := range p.keyed {
		if !now.Before(e.at.Add(p.ttl)) {
			delete(p.keyed, k)
		}
	}
}
