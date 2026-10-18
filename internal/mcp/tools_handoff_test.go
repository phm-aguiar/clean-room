package mcp

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/phm-aguiar/ai-memory-go/internal/domain"
	"github.com/phm-aguiar/ai-memory-go/internal/storage"
)

// recordingServer é um ToolServer de teste: guarda os handlers pelo nome e chama-os direto, sem protocolo.
type recordingServer struct {
	handlers map[string]ToolHandler
}

func (s *recordingServer) AddTool(def ToolDef, h ToolHandler) error {
	if _, dup := s.handlers[def.Name]; dup {
		return errors.New("ferramenta duplicada")
	}
	s.handlers[def.Name] = h
	return nil
}

func (s *recordingServer) Handle(_ context.Context, _ []byte) []byte { return nil }

// handoffFixture monta banco real em diretório temporário, o escopo default/scratch e as ferramentas de handoff.
// O relógio é f.now, avançado pelos testes.
type handoffFixture struct {
	t      *testing.T
	ctx    context.Context
	writer storage.Writer
	srv    *recordingServer
	now    time.Time
	scope  domain.Scope
}

type handoffBeginOut struct {
	HandoffID string `json:"handoff_id"`
}

type handoffListOut struct {
	Handoffs []handoffOut `json:"handoffs"`
}

type handoffCancelOut struct {
	HandoffID string `json:"handoff_id"`
	Cancelled bool   `json:"cancelled"`
	State     string `json:"state"`
}

func newHandoffTestID(t *testing.T) domain.ID {
	t.Helper()
	id, err := domain.NewID()
	if err != nil {
		t.Fatalf("domain.NewID: %v", err)
	}
	return id
}

func newHandoffFixture(t *testing.T) *handoffFixture {
	t.Helper()
	ctx := context.Background()
	dataDir := t.TempDir()

	db, err := storage.Open(ctx, dataDir)
	if err != nil {
		t.Fatalf("storage.Open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	writer, err := storage.NewWriter(ctx, db)
	if err != nil {
		t.Fatalf("storage.NewWriter: %v", err)
	}
	t.Cleanup(func() { _ = writer.Close() })

	readDB, err := storage.OpenReader(ctx, dataDir)
	if err != nil {
		t.Fatalf("storage.OpenReader: %v", err)
	}
	t.Cleanup(func() { _ = readDB.Close() })

	f := &handoffFixture{
		t:      t,
		ctx:    ctx,
		writer: writer,
		srv:    &recordingServer{handlers: map[string]ToolHandler{}},
		now:    baseTime,
		scope:  domain.NewScope(domain.WorkspaceID(newHandoffTestID(t)), domain.ProjectID(newHandoffTestID(t))),
	}
	if err := storage.EnsureScope(ctx, writer, f.scope, "default", "scratch", baseTime); err != nil {
		t.Fatalf("EnsureScope: %v", err)
	}
	if err := RegisterHandoffTools(f.srv, HandoffToolDeps{
		Writer: writer,
		Reader: storage.NewReader(readDB),
		Now:    func() time.Time { return f.now },
	}); err != nil {
		t.Fatalf("RegisterHandoffTools: %v", err)
	}
	return f
}

// actorCtx devolve o contexto de um ator: usuário e id de sessão (vazio = sem sessão).
func actorCtx(user, session string) context.Context {
	return WithActorKey(context.Background(), ActorKey{User: user, SessionID: session})
}

// openSession cria uma sessão aberta do usuário no escopo do fixture e devolve o id como string.
func (f *handoffFixture) openSession(user string) string {
	f.t.Helper()
	id := domain.SessionID(newHandoffTestID(f.t))
	err := f.writer.Do(f.ctx, func(tx *sql.Tx) error {
		_, err := storage.OpenOrTouchSessionTx(tx, storage.SessionOpen{
			ID:        id,
			Scope:     f.scope,
			AgentKind: "claude-code",
			ActorUser: user,
			At:        f.now,
		})
		return err
	})
	if err != nil {
		f.t.Fatalf("abrir sessão: %v", err)
	}
	return id.String()
}

// call chama a ferramenta pelo nome, com escopo default/scratch quando o teste não informa outro.
func (f *handoffFixture) call(ctx context.Context, name string, args map[string]any) (ToolResult, error) {
	f.t.Helper()
	if args == nil {
		args = map[string]any{}
	}
	if _, ok := args["workspace"]; !ok {
		args["workspace"] = "default"
	}
	if _, ok := args["project"]; !ok {
		args["project"] = "scratch"
	}
	raw, err := json.Marshal(args)
	if err != nil {
		f.t.Fatalf("marshal args: %v", err)
	}
	h, ok := f.srv.handlers[name]
	if !ok {
		f.t.Fatalf("ferramenta %s não registrada", name)
	}
	return h(ctx, raw)
}

// begin cria um handoff e devolve o handoff_id.
func (f *handoffFixture) begin(ctx context.Context, args map[string]any) string {
	f.t.Helper()
	res, err := f.call(ctx, "memory_handoff_begin", args)
	if err != nil {
		f.t.Fatalf("begin: %v", err)
	}
	var out handoffBeginOut
	if err := json.Unmarshal([]byte(res.Text), &out); err != nil {
		f.t.Fatalf("begin: resposta: %v", err)
	}
	if _, err := domain.ParseID(out.HandoffID); err != nil {
		f.t.Fatalf("begin: handoff_id inválido %q: %v", out.HandoffID, err)
	}
	return out.HandoffID
}

// list devolve os handoffs listados para o ator.
func (f *handoffFixture) list(ctx context.Context, args map[string]any) []handoffOut {
	f.t.Helper()
	res, err := f.call(ctx, "memory_handoff_list", args)
	if err != nil {
		f.t.Fatalf("list: %v", err)
	}
	var out handoffListOut
	if err := json.Unmarshal([]byte(res.Text), &out); err != nil {
		f.t.Fatalf("list: resposta: %v", err)
	}
	return out.Handoffs
}

// accept reivindica um handoff e devolve {handoff, status}.
func (f *handoffFixture) accept(ctx context.Context, args map[string]any) handoffAcceptOut {
	f.t.Helper()
	res, err := f.call(ctx, "memory_handoff_accept", args)
	if err != nil {
		f.t.Fatalf("accept: %v", err)
	}
	var out handoffAcceptOut
	if err := json.Unmarshal([]byte(res.Text), &out); err != nil {
		f.t.Fatalf("accept: resposta: %v", err)
	}
	return out
}

// cancel expira um handoff e devolve a resposta.
func (f *handoffFixture) cancel(ctx context.Context, args map[string]any) handoffCancelOut {
	f.t.Helper()
	res, err := f.call(ctx, "memory_handoff_cancel", args)
	if err != nil {
		f.t.Fatalf("cancel: %v", err)
	}
	var out handoffCancelOut
	if err := json.Unmarshal([]byte(res.Text), &out); err != nil {
		f.t.Fatalf("cancel: resposta: %v", err)
	}
	return out
}

func TestHandoffBeginGravaEListaPorDono(t *testing.T) {
	f := newHandoffFixture(t)
	alice := actorCtx("alice", "")
	bob := actorCtx("bob", "")

	id := f.begin(alice, map[string]any{
		"summary":        "resumo da sessão",
		"open_questions": []string{"q1"},
		"next_steps":     []string{"p1"},
		"files_touched":  []string{"a.go"},
		"cwd":            "/repo/",
	})

	got := f.list(alice, nil)
	if len(got) != 1 {
		t.Fatalf("alice: %d handoffs, quer 1", len(got))
	}
	h := got[0]
	if h.HandoffID != id || h.Summary != "resumo da sessão" || h.OwnerUser != "alice" || h.FromAgent != "mcp" {
		t.Fatalf("alice: handoff gravado errado: %+v", h)
	}
	if h.Cwd != "/repo" {
		t.Fatalf("cwd sem barra final: quer /repo, veio %q", h.Cwd)
	}
	if len(h.OpenQuestions) != 1 || h.OpenQuestions[0] != "q1" || len(h.NextSteps) != 1 || len(h.FilesTouched) != 1 {
		t.Fatalf("listas gravadas erradas: %+v", h)
	}
	if h.CreatedAt == "" {
		t.Fatal("created_at vazio")
	}

	if got := f.list(bob, nil); len(got) != 0 {
		t.Fatalf("bob viu handoff privado de alice: %+v", got)
	}
}

func TestHandoffBeginCriaProjetoAusente(t *testing.T) {
	f := newHandoffFixture(t)
	alice := actorCtx("alice", "")

	f.begin(alice, map[string]any{"summary": "novo projeto", "project": "novo"})
	if got := f.list(alice, map[string]any{"project": "novo"}); len(got) != 1 {
		t.Fatalf("projeto criado: %d handoffs, quer 1", len(got))
	}
}

func TestHandoffBeginCompartilhadoVisivelAoProjeto(t *testing.T) {
	f := newHandoffFixture(t)
	alice := actorCtx("alice", "")
	bob := actorCtx("bob", "")

	f.begin(alice, map[string]any{"summary": "privado"})
	shared := f.begin(alice, map[string]any{"summary": "compartilhado", "shared": true})

	got := f.list(bob, nil)
	if len(got) != 1 || got[0].HandoffID != shared || got[0].OwnerUser != "" {
		t.Fatalf("bob: quer só o compartilhado sem dono, veio %+v", got)
	}
	if got := f.list(alice, nil); len(got) != 2 {
		t.Fatalf("alice: %d handoffs, quer 2", len(got))
	}
}

func TestHandoffBeginExigeSummary(t *testing.T) {
	f := newHandoffFixture(t)
	_, err := f.call(actorCtx("alice", ""), "memory_handoff_begin", map[string]any{"summary": "   "})
	if !errors.Is(err, errToolInput) {
		t.Fatalf("summary vazio: erro %v, quer entrada inválida", err)
	}
}

func TestHandoffBeginRemoveSegredoAntesDoTeto(t *testing.T) {
	f := newHandoffFixture(t)
	alice := actorCtx("alice", "")
	f.begin(alice, map[string]any{"summary": "token sk-abcdefghijklmnopqrstuvwx usado"})

	got := f.list(alice, nil)
	if len(got) != 1 {
		t.Fatalf("%d handoffs, quer 1", len(got))
	}
	if strings.Contains(got[0].Summary, "sk-abcdefghijklmnopqrstuvwx") {
		t.Fatalf("segredo gravado sem redação: %q", got[0].Summary)
	}
	if !strings.Contains(got[0].Summary, "REDACTED") {
		t.Fatalf("redação ausente: %q", got[0].Summary)
	}
}

func TestHandoffBeginAplicaTetos(t *testing.T) {
	f := newHandoffFixture(t)
	alice := actorCtx("alice", "")

	f.begin(alice, map[string]any{
		"summary":        strings.Repeat("a", 3500),
		"open_questions": []string{strings.Repeat("b", 1600), strings.Repeat("b", 1600), strings.Repeat("b", 1600), strings.Repeat("b", 1600), strings.Repeat("b", 1600)},
		"next_steps":     []string{strings.Repeat("c", 1000)},
		"files_touched":  []string{strings.Repeat("d", 600), strings.Repeat("e", 600)},
	})
	h := f.list(alice, nil)[0]

	if n := utf8.RuneCountInString(h.Summary); n > handoffSummaryMax {
		t.Fatalf("resumo com %d caracteres, teto %d", n, handoffSummaryMax)
	}
	if !strings.Contains(h.Summary, "caracteres omitidos]") {
		t.Fatalf("resumo cortado sem marcador de omitidos: fim %q", h.Summary[len(h.Summary)-40:])
	}

	steps := 0
	for _, q := range h.OpenQuestions {
		if n := utf8.RuneCountInString(q); n > handoffItemMax {
			t.Fatalf("pergunta com %d caracteres, teto por item %d", n, handoffItemMax)
		}
		steps += utf8.RuneCountInString(q)
	}
	for _, p := range h.NextSteps {
		steps += utf8.RuneCountInString(p)
	}
	if steps > handoffStepsTotalMax {
		t.Fatalf("perguntas e passos somam %d caracteres, teto %d", steps, handoffStepsTotalMax)
	}
	if len(h.OpenQuestions) != 4 || len(h.NextSteps) != 0 {
		t.Fatalf("orçamento: quer 4 perguntas e 0 passos, veio %d e %d", len(h.OpenQuestions), len(h.NextSteps))
	}

	if n := utf8.RuneCountInString(h.FilesTouched[0]); n > handoffFileMax {
		t.Fatalf("arquivo com %d caracteres, teto %d", n, handoffFileMax)
	}
}

func TestHandoffBeginRecusaListaComMaisDe256Itens(t *testing.T) {
	f := newHandoffFixture(t)
	items := make([]string, handoffListItemsMax+1)
	for i := range items {
		items[i] = "x"
	}
	_, err := f.call(actorCtx("alice", ""), "memory_handoff_begin", map[string]any{"summary": "ok", "files_touched": items})
	if !errors.Is(err, errToolInput) {
		t.Fatalf("257 itens: erro %v, quer entrada inválida", err)
	}
}

func TestHandoffListLimitaResultado(t *testing.T) {
	f := newHandoffFixture(t)
	alice := actorCtx("alice", "")
	for i := 0; i < 3; i++ {
		f.begin(alice, map[string]any{"summary": "h", "shared": true})
		f.now = f.now.Add(time.Minute)
	}
	if got := f.list(alice, map[string]any{"limit": 2}); len(got) != 2 {
		t.Fatalf("limit 2: %d handoffs", len(got))
	}
}

func TestHandoffListNaoReivindica(t *testing.T) {
	f := newHandoffFixture(t)
	alice := actorCtx("alice", "")
	id := f.begin(alice, map[string]any{"summary": "h", "shared": true})
	f.list(alice, nil)
	f.list(alice, nil)

	sid := f.openSession("alice")
	out := f.accept(actorCtx("alice", sid), map[string]any{"handoff_id": id})
	if out.Status != handoffStatusClaimed || out.Handoff == nil {
		t.Fatalf("depois de listar, aceite quer claimed, veio %+v", out)
	}
}

func TestHandoffAcceptReivindicaPorId(t *testing.T) {
	f := newHandoffFixture(t)
	id := f.begin(actorCtx("alice", ""), map[string]any{"summary": "para bob", "shared": true})

	bobSid := f.openSession("bob")
	out := f.accept(actorCtx("bob", bobSid), map[string]any{"handoff_id": id})
	if out.Status != handoffStatusClaimed || out.Handoff == nil || out.Handoff.HandoffID != id || out.Handoff.Summary != "para bob" {
		t.Fatalf("aceite por id: %+v", out)
	}
	if got := f.list(actorCtx("bob", ""), nil); len(got) != 0 {
		t.Fatalf("handoff aceito ainda aberto na listagem: %+v", got)
	}
}

func TestHandoffAcceptRecusaHandoffJaReivindicado(t *testing.T) {
	f := newHandoffFixture(t)
	alice := actorCtx("alice", "")
	first := f.begin(alice, map[string]any{"summary": "primeiro", "shared": true})
	second := f.begin(alice, map[string]any{"summary": "segundo", "shared": true})

	bobSid := f.openSession("bob")
	carolSid := f.openSession("carol")
	bob := actorCtx("bob", bobSid)
	carol := actorCtx("carol", carolSid)

	if out := f.accept(bob, map[string]any{"handoff_id": first}); out.Status != handoffStatusClaimed {
		t.Fatalf("bob: quer claimed, veio %+v", out)
	}

	// Carol perde a corrida pelo mesmo id: sem handoff e sem erro.
	out := f.accept(carol, map[string]any{"handoff_id": first})
	if out.Status != handoffStatusNonePending || out.Handoff != nil {
		t.Fatalf("id já reivindicado: quer none_pending sem handoff, veio %+v", out)
	}

	// Bob já tem um aceite nesta sessão: não reivindica outro e o status é consumed_by_hook.
	out = f.accept(bob, map[string]any{"handoff_id": second})
	if out.Status != handoffStatusConsumed || out.Handoff != nil {
		t.Fatalf("receptor já com aceite: quer consumed_by_hook sem handoff, veio %+v", out)
	}
}

func TestHandoffAcceptSemIdPegaMaisRecente(t *testing.T) {
	f := newHandoffFixture(t)
	alice := actorCtx("alice", "")
	f.begin(alice, map[string]any{"summary": "antigo", "shared": true})
	f.now = f.now.Add(time.Minute)
	newer := f.begin(alice, map[string]any{"summary": "recente", "shared": true})

	bobSid := f.openSession("bob")
	out := f.accept(actorCtx("bob", bobSid), nil)
	if out.Status != handoffStatusClaimed || out.Handoff == nil || out.Handoff.HandoffID != newer {
		t.Fatalf("sem id: quer o mais recente %s, veio %+v", newer, out)
	}
}

func TestHandoffAcceptSemPendentes(t *testing.T) {
	f := newHandoffFixture(t)
	bobSid := f.openSession("bob")
	out := f.accept(actorCtx("bob", bobSid), nil)
	if out.Status != handoffStatusNonePending || out.Handoff != nil {
		t.Fatalf("sem pendentes: %+v", out)
	}
}

func TestHandoffAcceptNaoRevelaHandoffDeOutroDono(t *testing.T) {
	f := newHandoffFixture(t)
	id := f.begin(actorCtx("alice", ""), map[string]any{"summary": "privado de alice"})

	bobSid := f.openSession("bob")
	out := f.accept(actorCtx("bob", bobSid), map[string]any{"handoff_id": id})
	if out.Status != handoffStatusNonePending || out.Handoff != nil {
		t.Fatalf("dono de outro operador: quer none_pending sem corpo, veio %+v", out)
	}
	if got := f.list(actorCtx("alice", ""), nil); len(got) != 1 {
		t.Fatalf("handoff de alice mudou de estado: %d abertos", len(got))
	}
}

func TestHandoffAcceptExigeSessao(t *testing.T) {
	f := newHandoffFixture(t)
	_, err := f.call(actorCtx("alice", ""), "memory_handoff_accept", nil)
	if !errors.Is(err, errToolInput) {
		t.Fatalf("sem sessão: erro %v, quer entrada inválida", err)
	}
}

func TestHandoffCancelExpiraEImpedeAceite(t *testing.T) {
	f := newHandoffFixture(t)
	alice := actorCtx("alice", "")
	id := f.begin(alice, map[string]any{"summary": "por engano"})

	out := f.cancel(alice, map[string]any{"handoff_id": id})
	if !out.Cancelled || out.State != handoffStateExpired || out.HandoffID != id {
		t.Fatalf("cancelar: %+v", out)
	}
	again := f.cancel(alice, map[string]any{"handoff_id": id})
	if again.Cancelled {
		t.Fatalf("cancelar de novo deveria ser falso: %+v", again)
	}
	if got := f.list(alice, nil); len(got) != 0 {
		t.Fatalf("expirado ainda listado: %+v", got)
	}

	bobSid := f.openSession("bob")
	if out := f.accept(actorCtx("bob", bobSid), map[string]any{"handoff_id": id}); out.Status != handoffStatusNonePending {
		t.Fatalf("expirado ainda aceito: %+v", out)
	}
}

func TestHandoffCancelNaoAlcancaHandoffDeOutroDono(t *testing.T) {
	f := newHandoffFixture(t)
	id := f.begin(actorCtx("alice", ""), map[string]any{"summary": "de alice"})

	out := f.cancel(actorCtx("bob", ""), map[string]any{"handoff_id": id})
	if out.Cancelled {
		t.Fatalf("bob cancelou handoff de alice: %+v", out)
	}
	if got := f.list(actorCtx("alice", ""), nil); len(got) != 1 {
		t.Fatalf("handoff de alice saiu da lista: %d", len(got))
	}
}

func TestHandoffCancelExigeId(t *testing.T) {
	f := newHandoffFixture(t)
	_, err := f.call(actorCtx("alice", ""), "memory_handoff_cancel", nil)
	if !errors.Is(err, errToolInput) {
		t.Fatalf("sem handoff_id: erro %v, quer entrada inválida", err)
	}
}

func TestHandoffAnyOwnerRecusadoSemRaiz(t *testing.T) {
	f := newHandoffFixture(t)
	alice := actorCtx("alice", "")
	id := f.begin(alice, map[string]any{"summary": "h"})
	bobSid := f.openSession("bob")
	bob := actorCtx("bob", bobSid)

	calls := map[string]map[string]any{
		"memory_handoff_list":   {"any_owner": true},
		"memory_handoff_accept": {"any_owner": true, "handoff_id": id},
		"memory_handoff_cancel": {"any_owner": true, "handoff_id": id},
	}
	for name, args := range calls {
		if _, err := f.call(bob, name, args); !errors.Is(err, errToolForbidden) {
			t.Fatalf("%s com any_owner: erro %v, quer não autorizado", name, err)
		}
	}
	if got := f.list(alice, nil); len(got) != 1 {
		t.Fatalf("any_owner recusado mexeu no handoff: %d abertos", len(got))
	}
}

func TestRegisterHandoffToolsRecusaDependenciaNula(t *testing.T) {
	if err := RegisterHandoffTools(nil, HandoffToolDeps{}); err == nil {
		t.Fatal("servidor nulo aceito")
	}
	if err := RegisterHandoffTools(&recordingServer{handlers: map[string]ToolHandler{}}, HandoffToolDeps{}); err == nil {
		t.Fatal("escritor e leitor nulos aceitos")
	}
}
