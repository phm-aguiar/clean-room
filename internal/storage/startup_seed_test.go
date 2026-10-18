package storage

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/phm-aguiar/ai-memory-go/internal/domain"
)

// ssNow é o instante fixo dos testes de semente. Observações são gravadas com created_at relativo a ele.
var ssNow = time.Date(2026, 10, 18, 12, 0, 0, 0, time.UTC)

// ssLogger devolve um logger que escreve em buf, para afirmar o que foi registrado.
func ssLogger(buf *bytes.Buffer) *slog.Logger {
	return slog.New(slog.NewTextHandler(buf, nil))
}

// ssScope cria um escopo novo (workspace e projeto) com EnsureScope.
func ssScope(t *testing.T, w Writer, name string) domain.Scope {
	t.Helper()
	ws, err := domain.NewID()
	if err != nil {
		t.Fatalf("NewID ws: %v", err)
	}
	pj, err := domain.NewID()
	if err != nil {
		t.Fatalf("NewID pj: %v", err)
	}
	scope := domain.NewScope(domain.WorkspaceID(ws), domain.ProjectID(pj))
	rsMustEnsure(t, w, scope, "ws-"+name, "pj-"+name)
	return scope
}

// ssSession cria uma sessão do escopo (a observação exige sessão existente) e devolve o id em bytes.
func ssSession(t *testing.T, w Writer, scope domain.Scope) []byte {
	t.Helper()
	id, err := domain.NewID()
	if err != nil {
		t.Fatalf("NewID session: %v", err)
	}
	err = w.Do(context.Background(), func(tx *sql.Tx) error {
		_, err := tx.Exec(
			`INSERT INTO sessions (id, workspace_id, project_id, agent_kind, started_at) VALUES (?, ?, ?, 'claude-code', ?)`,
			id.Bytes(), scope.WorkspaceID.Bytes(), scope.ProjectID.Bytes(), schemaTS)
		return err
	})
	if err != nil {
		t.Fatalf("insert session: %v", err)
	}
	return id.Bytes()
}

// ssObs grava uma observação do escopo com created_at = at. Usa SQL direto para fixar o instante.
func ssObs(t *testing.T, w Writer, scope domain.Scope, sess []byte, at time.Time) {
	t.Helper()
	id, err := domain.NewID()
	if err != nil {
		t.Fatalf("NewID obs: %v", err)
	}
	err = w.Do(context.Background(), func(tx *sql.Tx) error {
		_, err := tx.Exec(
			`INSERT INTO observations (id, session_id, workspace_id, project_id, kind, title, body, importance, created_at)
			 VALUES (?, ?, ?, ?, 'decision', 'título', 'corpo', 5, ?)`,
			id.Bytes(), sess, scope.WorkspaceID.Bytes(), scope.ProjectID.Bytes(), at.UnixMicro())
		return err
	})
	if err != nil {
		t.Fatalf("insert observation: %v", err)
	}
}

// ssFailingReader falha em toda leitura, para simular erro de banco no boot.
type ssFailingReader struct{ err error }

func (f ssFailingReader) Read(context.Context, func(*sql.Tx) error) error { return f.err }

func TestStartupSeed_SemeiaProjetoDaObservacaoMaisRecenteDentroDoTTL(t *testing.T) {
	_, w, r := rsTestEnv(t)
	ctx := context.Background()

	fora := ssScope(t, w, "fora")
	antiga := ssScope(t, w, "antiga")
	recente := ssScope(t, w, "recente")
	ssObs(t, w, fora, ssSession(t, w, fora), ssNow.Add(-2*time.Hour))        // fora do TTL de 1 h
	ssObs(t, w, antiga, ssSession(t, w, antiga), ssNow.Add(-50*time.Minute)) // dentro, mas não a mais recente
	ssObs(t, w, recente, ssSession(t, w, recente), ssNow.Add(-30*time.Minute))

	p := apNew(t, "per_actor")
	var buf bytes.Buffer
	if !SeedStartupFromRecent(ctx, r, p, ssNow, ssLogger(&buf)) {
		t.Fatalf("semente não aplicada; log: %s", buf.String())
	}

	if res := p.Read(ActorKey{}, ssNow); res.Origin != ReadOriginStartupSeed || res.Scope != recente {
		t.Errorf("read sem coordenada = %+v, quer startup_seed no projeto mais recente", res)
	}
	// A semente é só para leitura: a escrita não a usa.
	if res := p.Lookup(ActorKey{}, ssNow); res.Outcome != LookupUnset {
		t.Errorf("lookup usou a semente: %+v, quer unset", res)
	}
	if p.shared != nil || len(p.keyed) != 0 || p.everKeyed {
		t.Errorf("semente foi gravada no slot compartilhado ou nas entradas chaveadas")
	}
	if !strings.Contains(buf.String(), "startup seed") {
		t.Errorf("aplicação da semente não foi registrada: %s", buf.String())
	}
}

func TestStartupSeed_ObservacaoForaDoTTLNaoSemeia(t *testing.T) {
	_, w, r := rsTestEnv(t)
	ctx := context.Background()

	velha := ssScope(t, w, "velha")
	ssObs(t, w, velha, ssSession(t, w, velha), ssNow.Add(-2*time.Hour))

	p := apNew(t, "per_actor")
	var buf bytes.Buffer
	if SeedStartupFromRecent(ctx, r, p, ssNow, ssLogger(&buf)) {
		t.Fatalf("semente aplicada com observação fora do TTL")
	}
	if res := p.Read(ActorKey{}, ssNow); res.Origin != ReadOriginUnset {
		t.Errorf("read = %+v, quer unset", res)
	}
	if !strings.Contains(buf.String(), "sem semente") {
		t.Errorf("ausência não foi registrada: %s", buf.String())
	}
}

func TestStartupSeed_SemObservacaoNaoSemeia(t *testing.T) {
	_, _, r := rsTestEnv(t)

	p := apNew(t, "per_actor")
	if SeedStartupFromRecent(context.Background(), r, p, ssNow, ssLogger(&bytes.Buffer{})) {
		t.Fatalf("semente aplicada sem nenhuma observação")
	}
	if res := p.Read(ActorKey{}, ssNow); res.Origin != ReadOriginUnset {
		t.Errorf("read = %+v, quer unset", res)
	}
}

func TestStartupSeed_SlotCompartilhadoPreenchidoNaoRecebeSemente(t *testing.T) {
	_, w, r := rsTestEnv(t)
	ctx := context.Background()

	recente := ssScope(t, w, "recente")
	ssObs(t, w, recente, ssSession(t, w, recente), ssNow.Add(-10*time.Minute))
	ativo := ssScope(t, w, "ativo")

	p := apNew(t, "per_actor")
	// Evento de primeiro plano já publicou: o slot compartilhado não está vazio.
	p.Publish(ActorKey{SessionID: "s1"}, ativo, false, true, ssNow)

	if SeedStartupFromRecent(ctx, r, p, ssNow, ssLogger(&bytes.Buffer{})) {
		t.Fatalf("semente aplicada com o slot compartilhado preenchido")
	}
	if p.seed != nil {
		t.Errorf("semente foi gravada mesmo com o slot preenchido")
	}
	if res := p.Read(ActorKey{}, ssNow); res.Origin != ReadOriginSharedSlot || res.Scope != ativo {
		t.Errorf("read = %+v, quer shared_slot do projeto publicado", res)
	}
}

func TestStartupSeed_PrimeiroEventoDePrimeiroPlanoDescartaASemente(t *testing.T) {
	_, w, r := rsTestEnv(t)
	ctx := context.Background()

	recente := ssScope(t, w, "recente")
	ssObs(t, w, recente, ssSession(t, w, recente), ssNow.Add(-10*time.Minute))
	ativo := ssScope(t, w, "ativo")

	p := apNew(t, "per_actor")
	if !SeedStartupFromRecent(ctx, r, p, ssNow, ssLogger(&bytes.Buffer{})) {
		t.Fatalf("semente não aplicada")
	}

	// Evento de conclusão não é de primeiro plano: não descarta a semente para quem não tem coordenada.
	p.Publish(ActorKey{SessionID: "s1"}, ativo, false, false, ssNow)
	if res := p.Read(ActorKey{}, ssNow); res.Origin != ReadOriginStartupSeed || res.Scope != recente {
		t.Errorf("após evento de conclusão: %+v, quer startup_seed ainda ativa", res)
	}

	// Primeiro evento de primeiro plano descarta a semente: o slot compartilhado passa a responder.
	p.Publish(ActorKey{SessionID: "s1"}, ativo, false, true, ssNow)
	if res := p.Read(ActorKey{}, ssNow); res.Origin != ReadOriginSharedSlot || res.Scope != ativo {
		t.Errorf("após primeiro evento de primeiro plano: %+v, quer shared_slot", res)
	}
	// Chamador de outra sessão cai em divergência, nunca na semente descartada.
	if res := p.Read(ActorKey{SessionID: "s2"}, ssNow); res.Origin != ReadOriginMismatch {
		t.Errorf("sessão s2 = %+v, quer mismatch (nunca a semente)", res)
	}
}

func TestStartupSeed_InvalidacaoAdministrativaLimpaASemente(t *testing.T) {
	_, w, r := rsTestEnv(t)
	ctx := context.Background()

	recente := ssScope(t, w, "recente")
	ssObs(t, w, recente, ssSession(t, w, recente), ssNow.Add(-10*time.Minute))

	p := apNew(t, "per_actor")
	if !SeedStartupFromRecent(ctx, r, p, ssNow, ssLogger(&bytes.Buffer{})) {
		t.Fatalf("semente não aplicada")
	}
	// Invalidação administrativa (purga, mudança ou remoção de escopo) remove a semente.
	p.ClearStartupSeed()
	if res := p.Read(ActorKey{}, ssNow); res.Origin != ReadOriginUnset {
		t.Errorf("após invalidação: %+v, quer unset", res)
	}
}

func TestStartupSeed_FalhaSoELogada(t *testing.T) {
	var buf bytes.Buffer
	p := apNew(t, "per_actor")
	failing := ssFailingReader{err: errors.New("banco indisponível")}

	if SeedStartupFromRecent(context.Background(), failing, p, ssNow, ssLogger(&buf)) {
		t.Fatalf("semente aplicada com falha de leitura")
	}
	if res := p.Read(ActorKey{}, ssNow); res.Origin != ReadOriginUnset {
		t.Errorf("read após falha = %+v, quer unset", res)
	}
	out := buf.String()
	if !strings.Contains(out, "startup seed") || !strings.Contains(out, "banco indisponível") {
		t.Errorf("falha não foi registrada: %s", out)
	}
}
