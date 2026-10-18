package storage

import (
	"bytes"
	"errors"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/phm-aguiar/ai-memory-go/internal/domain"
)

var apT0 = time.Date(2026, 10, 13, 12, 0, 0, 0, time.UTC)

func apScope(n byte) domain.Scope {
	var ws, pj [16]byte
	ws[15] = 1
	pj[15] = n
	return domain.NewScope(domain.WorkspaceID(ws), domain.ProjectID(pj))
}

func apNew(t *testing.T, mode string) *ActiveProjectPointer {
	t.Helper()
	p, err := NewActiveProjectPointer(ActiveProjectConfig{Mode: mode, SessionTTLSecs: 3600, MaxEntries: 4096}, slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil)))
	if err != nil {
		t.Fatalf("NewActiveProjectPointer(%q): %v", mode, err)
	}
	return p
}

func TestActiveProject_ModoPadraoEPerActor(t *testing.T) {
	if DefaultActiveProjectMode != ActiveProjectModePerActor {
		t.Fatalf("padrão = %q, quer per_actor", DefaultActiveProjectMode)
	}
	if got := apNew(t, string(DefaultActiveProjectMode)).Mode(); got != ActiveProjectModePerActor {
		t.Fatalf("modo efetivo = %q, quer per_actor", got)
	}
}

func TestActiveProject_TresModosExistemEModoInvalidoImpedeSubida(t *testing.T) {
	for _, m := range []string{"single", "per_session", "per_actor"} {
		if got := apNew(t, m).Mode(); string(got) != m {
			t.Errorf("modo %q: efetivo %q", m, got)
		}
	}
	for _, bad := range []string{"", "PerSession", "shared", "per-actor"} {
		p, err := NewActiveProjectPointer(ActiveProjectConfig{Mode: bad, SessionTTLSecs: 3600, MaxEntries: 4096}, nil)
		if p != nil {
			t.Errorf("modo %q: devolveu ponteiro com erro", bad)
		}
		if !errors.Is(err, ErrActiveProjectMode) {
			t.Errorf("modo %q: erro = %v, quer ErrActiveProjectMode", bad, err)
		}
	}
}

func TestActiveProject_TTLZeroViraUmaHora(t *testing.T) {
	p, err := NewActiveProjectPointer(ActiveProjectConfig{Mode: "per_actor", SessionTTLSecs: 0, MaxEntries: 10}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if p.ttl != time.Hour {
		t.Fatalf("ttl = %v, quer 1h", p.ttl)
	}
	a := ActorKey{Identity: "user:a", SessionID: "s1"}
	p.Publish(a, apScope(1), false, true, apT0)
	if r := p.Lookup(a, apT0.Add(time.Hour-time.Second)); r.Outcome != LookupResolved {
		t.Errorf("em 59m59s a entrada deveria estar viva: %v", r.Outcome)
	}
	if r := p.Lookup(a, apT0.Add(time.Hour)); r.Outcome == LookupResolved {
		t.Errorf("em 1h a entrada deveria ter vencido: %v", r.Outcome)
	}
}

func TestActiveProject_MaxEntriesZeroViraUm(t *testing.T) {
	p, err := NewActiveProjectPointer(ActiveProjectConfig{Mode: "per_session", SessionTTLSecs: 3600, MaxEntries: 0}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if p.max != 1 {
		t.Fatalf("max = %d, quer 1", p.max)
	}
}

func TestActiveProject_LogRegistraModoEfetivo(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&buf, nil))
	if _, err := NewActiveProjectPointer(ActiveProjectConfig{Mode: "per_session", SessionTTLSecs: 0, MaxEntries: 0}, logger); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	for _, want := range []string{"active-project isolation mode", "mode=per_session", "session_ttl_secs=3600", "max_entries=1"} {
		if !strings.Contains(out, want) {
			t.Errorf("log sem %q: %s", want, out)
		}
	}
}

func TestActiveProject_TetoRemoveAsMaisAntigas(t *testing.T) {
	p, err := NewActiveProjectPointer(ActiveProjectConfig{Mode: "per_session", SessionTTLSecs: 3600, MaxEntries: 3}, nil)
	if err != nil {
		t.Fatal(err)
	}
	for i := 1; i <= 4; i++ {
		p.Publish(ActorKey{SessionID: string(rune('a' + i))}, apScope(byte(i)), false, true, apT0)
	}
	if len(p.keyed) != 3 {
		t.Fatalf("entradas = %d, quer 3", len(p.keyed))
	}
	if r := p.Lookup(ActorKey{SessionID: "b"}, apT0); r.Outcome == LookupResolved {
		t.Errorf("a entrada mais antiga (b) deveria ter sido removida pelo teto")
	}
	if r := p.Lookup(ActorKey{SessionID: "e"}, apT0); r.Outcome != LookupResolved || r.Scope != apScope(4) {
		t.Errorf("a entrada mais nova (e) deveria estar presente: %+v", r)
	}
}

func TestActiveProject_TetoNaoApagaMarcaPonteiroAtivo(t *testing.T) {
	p, err := NewActiveProjectPointer(ActiveProjectConfig{Mode: "per_session", SessionTTLSecs: 60, MaxEntries: 1}, nil)
	if err != nil {
		t.Fatal(err)
	}
	p.Publish(ActorKey{SessionID: "s1"}, apScope(1), false, true, apT0)
	later := apT0.Add(10 * time.Minute)
	if r := p.Lookup(ActorKey{SessionID: "s1"}, later); r.Outcome != LookupMismatch {
		t.Errorf("entrada vencida com marca ativa: outcome = %v, quer mismatch", r.Outcome)
	}
	if !p.everKeyed {
		t.Errorf("marca ponteiro ativo foi apagada")
	}
}

func TestActiveProject_ExpiracaoRemoveEntradaNaLeitura(t *testing.T) {
	p := apNew(t, "per_session")
	p.Publish(ActorKey{SessionID: "s1"}, apScope(1), false, true, apT0)
	p.Lookup(ActorKey{SessionID: "s1"}, apT0.Add(2*time.Hour))
	if len(p.keyed) != 0 {
		t.Errorf("entrada vencida continuou no mapa: %d", len(p.keyed))
	}
}

func TestActiveProject_LookupResolvedPorSessao(t *testing.T) {
	p := apNew(t, "per_actor")
	a := ActorKey{Identity: "user:ana", SessionID: "s1"}
	p.Publish(a, apScope(7), true, true, apT0)
	r := p.Lookup(a, apT0)
	if r.Outcome != LookupResolved || r.Scope != apScope(7) || !r.DefaultGlobal {
		t.Fatalf("lookup = %+v, quer resolved com projeto 7 e default_global", r)
	}
}

func TestActiveProject_LookupMismatchNuncaUsaSlotDeOutraSessao(t *testing.T) {
	p := apNew(t, "per_session")
	p.Publish(ActorKey{SessionID: "sA"}, apScope(1), false, true, apT0)
	r := p.Lookup(ActorKey{SessionID: "sB"}, apT0)
	if r.Outcome != LookupMismatch {
		t.Fatalf("outcome = %v, quer mismatch", r.Outcome)
	}
	if r.Scope != (domain.Scope{}) {
		t.Errorf("mismatch devolveu escopo %v; não pode vazar o slot de outra sessão", r.Scope)
	}
}

func TestActiveProject_LookupUnsetSemHooks(t *testing.T) {
	p := apNew(t, "per_actor")
	if r := p.Lookup(ActorKey{Identity: "user:ana", SessionID: "s1"}, apT0); r.Outcome != LookupUnset {
		t.Errorf("outcome = %v, quer unset", r.Outcome)
	}
	if r := p.Lookup(ActorKey{}, apT0); r.Outcome != LookupUnset {
		t.Errorf("anônimo: outcome = %v, quer unset", r.Outcome)
	}
}

func TestActiveProject_LookupChamadorSemCoordenadaUsaSlotCompartilhado(t *testing.T) {
	p := apNew(t, "per_actor")
	p.Publish(ActorKey{Identity: "user:ana", SessionID: "s1"}, apScope(3), false, true, apT0)
	r := p.Lookup(ActorKey{}, apT0)
	if r.Outcome != LookupResolved || r.Scope != apScope(3) {
		t.Errorf("anônimo: %+v, quer resolved no slot compartilhado (projeto 3)", r)
	}
}

func TestActiveProject_LookupIdentidadeSemSessaoCasaComSlotDaIdentidade(t *testing.T) {
	p := apNew(t, "per_actor")
	p.Publish(ActorKey{Identity: "user:ana", SessionID: "s1"}, apScope(5), false, true, apT0)
	// Foreground com identidade e sessão grava o slot só de identidade.
	if r := p.Lookup(ActorKey{Identity: "user:ana"}, apT0); r.Outcome != LookupResolved || r.Scope != apScope(5) {
		t.Errorf("identidade sem sessão: %+v, quer resolved (projeto 5)", r)
	}
	// Outra sessão da mesma identidade: a entrada própria falta e o slot de identidade está vivo: mismatch.
	if r := p.Lookup(ActorKey{Identity: "user:ana", SessionID: "s2"}, apT0); r.Outcome != LookupMismatch {
		t.Errorf("outra sessão da mesma identidade: outcome = %v, quer mismatch", r.Outcome)
	}
}

func TestActiveProject_CompletionNaoMoveSlotCompartilhado(t *testing.T) {
	p := apNew(t, "per_session")
	p.Publish(ActorKey{SessionID: "sA"}, apScope(1), false, true, apT0)
	p.Publish(ActorKey{SessionID: "sB"}, apScope(2), false, false, apT0) // conclusão de sB
	if r := p.Lookup(ActorKey{}, apT0); r.Outcome != LookupResolved || r.Scope != apScope(1) {
		t.Errorf("slot compartilhado mudou com evento de conclusão: %+v", r)
	}
	if r := p.Lookup(ActorKey{SessionID: "sB"}, apT0); r.Outcome != LookupResolved || r.Scope != apScope(2) {
		t.Errorf("entrada própria de sB não foi atualizada: %+v", r)
	}
}

func TestActiveProject_EventoSemCoordenadaNaoGravaNada(t *testing.T) {
	p := apNew(t, "per_actor")
	p.Publish(ActorKey{}, apScope(4), false, false, apT0)
	if len(p.keyed) != 0 || p.everKeyed || p.shared != nil {
		t.Errorf("evento sem coordenada de conclusão gravou algo: keyed=%d everKeyed=%v shared=%v", len(p.keyed), p.everKeyed, p.shared)
	}
	// Com identidade (sem sessão), a conclusão grava só a própria entrada, que é o slot de identidade.
	p.Publish(ActorKey{Identity: "user:ana"}, apScope(4), false, false, apT0)
	if len(p.keyed) != 1 || p.shared != nil {
		t.Errorf("conclusão com identidade: keyed=%d shared=%v, quer 1 entrada e slot compartilhado vazio", len(p.keyed), p.shared)
	}
}

func TestActiveProject_MarcaPonteiroAtivoPersisteAposExpiracao(t *testing.T) {
	p := apNew(t, "per_session")
	p.Publish(ActorKey{SessionID: "s1"}, apScope(1), false, true, apT0)
	later := apT0.Add(2 * time.Hour)
	// Expirada, mas a marca faz a resposta ser mismatch, não unset: a instalação já teve hooks.
	if r := p.Lookup(ActorKey{SessionID: "s1"}, later); r.Outcome != LookupMismatch {
		t.Errorf("após expiração: outcome = %v, quer mismatch", r.Outcome)
	}
}

func TestActiveProject_SingleUsaSlotGlobal(t *testing.T) {
	p := apNew(t, "single")
	p.Publish(ActorKey{SessionID: "sA"}, apScope(1), false, true, apT0)
	if r := p.Lookup(ActorKey{SessionID: "sB"}, apT0); r.Outcome != LookupResolved || r.Scope != apScope(1) {
		t.Errorf("single: sessão B = %+v, quer slot global (projeto 1)", r)
	}
	if len(p.keyed) != 0 || p.everKeyed {
		t.Errorf("single gravou entrada chaveada")
	}
	p2 := apNew(t, "single")
	p2.Publish(ActorKey{SessionID: "sA"}, apScope(1), false, false, apT0)
	if r := p2.Lookup(ActorKey{}, apT0); r.Outcome != LookupUnset {
		t.Errorf("single: conclusão não deveria gravar nada: %v", r.Outcome)
	}
}

func TestActiveProject_PerSessionIgnoraIdentidade(t *testing.T) {
	p := apNew(t, "per_session")
	p.Publish(ActorKey{Identity: "user:ana", SessionID: "s1"}, apScope(6), false, true, apT0)
	if _, ok := p.keyed[pointerKey{identity: "user:ana"}]; ok {
		t.Errorf("per_session gravou slot de identidade")
	}
	if r := p.Lookup(ActorKey{Identity: "user:bia"}, apT0); r.Outcome != LookupResolved || r.Scope != apScope(6) {
		t.Errorf("identidade sem sessão em per_session: %+v, quer slot compartilhado", r)
	}
}

func TestActiveProject_ReadOrigemSession(t *testing.T) {
	p := apNew(t, "per_session")
	p.Publish(ActorKey{SessionID: "s1"}, apScope(1), true, true, apT0)
	r := p.Read(ActorKey{SessionID: "s1"}, apT0)
	if r.Origin != ReadOriginSession || r.Scope != apScope(1) || !r.DefaultGlobal {
		t.Errorf("read = %+v, quer session com projeto 1 e default_global", r)
	}
}

func TestActiveProject_ReadOrigemSharedSlot(t *testing.T) {
	p := apNew(t, "per_session")
	p.Publish(ActorKey{SessionID: "s1"}, apScope(2), false, true, apT0)
	r := p.Read(ActorKey{}, apT0)
	if r.Origin != ReadOriginSharedSlot || r.Scope != apScope(2) {
		t.Errorf("read = %+v, quer shared_slot com projeto 2", r)
	}
}

func TestActiveProject_ReadOrigemMismatch(t *testing.T) {
	p := apNew(t, "per_session")
	p.Publish(ActorKey{SessionID: "s1"}, apScope(1), false, true, apT0)
	p.SeedStartup(apScope(9))
	r := p.Read(ActorKey{SessionID: "s2"}, apT0)
	if r.Origin != ReadOriginMismatch || r.Scope != (domain.Scope{}) {
		t.Errorf("read = %+v, quer mismatch sem escopo e sem semente", r)
	}
}

func TestActiveProject_ReadOrigemUnset(t *testing.T) {
	p := apNew(t, "per_actor")
	if r := p.Read(ActorKey{Identity: "user:ana", SessionID: "s1"}, apT0); r.Origin != ReadOriginUnset {
		t.Errorf("read = %+v, quer unset", r)
	}
}

func TestActiveProject_SementeSoParaLeitura(t *testing.T) {
	p := apNew(t, "per_session")
	p.SeedStartup(apScope(8))

	if r := p.Read(ActorKey{}, apT0); r.Origin != ReadOriginStartupSeed || r.Scope != apScope(8) {
		t.Errorf("read com semente = %+v, quer startup_seed (projeto 8)", r)
	}
	// Lookup (escrita) nunca usa a semente.
	if r := p.Lookup(ActorKey{}, apT0); r.Outcome != LookupUnset {
		t.Errorf("lookup usou a semente: %+v, quer unset", r)
	}
	if p.shared != nil || len(p.keyed) != 0 || p.everKeyed {
		t.Errorf("semente foi gravada no ponteiro")
	}
	// Divergência nunca cai na semente.
	p.Publish(ActorKey{SessionID: "s1"}, apScope(1), false, true, apT0)
	if r := p.Read(ActorKey{SessionID: "s2"}, apT0); r.Origin != ReadOriginMismatch {
		t.Errorf("mismatch caiu na semente: %+v", r)
	}
	// Primeiro evento de primeiro plano supera a semente para qualquer chamador.
	if r := p.Read(ActorKey{}, apT0); r.Origin != ReadOriginSharedSlot || r.Scope != apScope(1) {
		t.Errorf("após evento de primeiro plano: %+v, quer shared_slot", r)
	}
}

func TestActiveProject_SementeLimpaPorInvalidacao(t *testing.T) {
	p := apNew(t, "per_session")
	p.SeedStartup(apScope(8))
	p.ClearStartupSeed()
	if r := p.Read(ActorKey{}, apT0); r.Origin != ReadOriginUnset {
		t.Errorf("após limpar semente: %+v, quer unset", r)
	}
}

func TestActiveProject_SementeNaoSobrepoeChaveProria(t *testing.T) {
	p := apNew(t, "per_session")
	p.SeedStartup(apScope(8))
	p.Publish(ActorKey{SessionID: "s1"}, apScope(1), false, false, apT0)
	if r := p.Read(ActorKey{SessionID: "s1"}, apT0); r.Origin != ReadOriginSession || r.Scope != apScope(1) {
		t.Errorf("entrada própria perdeu para a semente: %+v", r)
	}
}
