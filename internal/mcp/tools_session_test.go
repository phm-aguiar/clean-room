package mcp

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/phm-aguiar/ai-memory-go/internal/domain"
	"github.com/phm-aguiar/ai-memory-go/internal/storage"
)

// baseTime é o instante fixo dos fixtures (UTC, microssegundos).
var baseTime = time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

// sessionFixture monta um banco real em diretório temporário, um workspace `default` com dois
// projetos (`scratch` e `outro`) e o servidor MCP com as ferramentas de sessão registradas.
type sessionFixture struct {
	t      *testing.T
	ctx    context.Context
	writer storage.Writer
	srv    ToolServer
	main   domain.Scope // default/scratch
	other  domain.Scope // default/outro
}

func newTestID(t *testing.T) domain.ID {
	t.Helper()
	id, err := domain.NewID()
	if err != nil {
		t.Fatalf("domain.NewID: %v", err)
	}
	return id
}

func newSessionFixture(t *testing.T) *sessionFixture {
	t.Helper()
	ctx := context.Background()

	// O TempDir é registrado primeiro: é removido por último, depois que os bancos fecharem.
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
	reader := storage.NewReader(readDB)

	ws := domain.WorkspaceID(newTestID(t))
	f := &sessionFixture{
		t:      t,
		ctx:    ctx,
		writer: writer,
		main:   domain.NewScope(ws, domain.ProjectID(newTestID(t))),
		other:  domain.NewScope(ws, domain.ProjectID(newTestID(t))),
	}
	if err := storage.EnsureScope(ctx, writer, f.main, "default", "scratch", baseTime); err != nil {
		t.Fatalf("EnsureScope main: %v", err)
	}
	if err := storage.EnsureScope(ctx, writer, f.other, "default", "outro", baseTime); err != nil {
		t.Fatalf("EnsureScope other: %v", err)
	}

	srv := NewToolServer()
	if err := RegisterSessionTools(srv, SessionToolDeps{
		Reader: reader,
		Now:    func() time.Time { return baseTime },
	}); err != nil {
		t.Fatalf("RegisterSessionTools: %v", err)
	}
	f.srv = srv
	return f
}

// session cria uma sessão do usuário (vazio = anônimo) no escopo. endedAt nil deixa a sessão aberta.
func (f *sessionFixture) session(user string, startedAt time.Time, endedAt *time.Time, scope domain.Scope) domain.SessionID {
	f.t.Helper()
	id := domain.SessionID(newTestID(f.t))
	err := f.writer.Do(f.ctx, func(tx *sql.Tx) error {
		if _, err := storage.OpenOrTouchSessionTx(tx, storage.SessionOpen{
			ID:        id,
			Scope:     scope,
			AgentKind: "claude-code",
			ActorUser: user,
			At:        startedAt,
		}); err != nil {
			return err
		}
		if endedAt != nil {
			_, err := storage.CloseSessionTx(tx, id, *endedAt)
			return err
		}
		return nil
	})
	if err != nil {
		f.t.Fatalf("sessão: %v", err)
	}
	return id
}

// observation grava uma observação crua na sessão, no escopo dado (SQL direto, só em teste).
func (f *sessionFixture) observation(session domain.SessionID, scope domain.Scope, title, body string, at time.Time) {
	f.t.Helper()
	id := newTestID(f.t)
	err := f.writer.Do(f.ctx, func(tx *sql.Tx) error {
		_, err := tx.Exec(
			`INSERT INTO observations (id, session_id, workspace_id, project_id, kind, title, body, importance, created_at)
			 VALUES (?, ?, ?, ?, 'user-prompt', ?, ?, 5, ?)`,
			id.Bytes(), session.Bytes(), scope.WorkspaceID.Bytes(), scope.ProjectID.Bytes(), title, body, at.UnixMicro(),
		)
		return err
	})
	if err != nil {
		f.t.Fatalf("observação: %v", err)
	}
}

// page grava a versão corrente de uma página (SQL direto, só em teste). kind vazio omite o kind.
func (f *sessionFixture) page(scope domain.Scope, path, kind string, pinned bool, updated time.Time, evidence int) {
	f.t.Helper()
	id := newTestID(f.t)
	fm := `{}`
	if kind != "" {
		fm = fmt.Sprintf(`{"kind":%q}`, kind)
	}
	pin := 0
	if pinned {
		pin = 1
	}
	err := f.writer.Do(f.ctx, func(tx *sql.Tx) error {
		if _, err := tx.Exec(
			`INSERT INTO pages (id, workspace_id, project_id, path, title, tier, body, body_sha256, frontmatter_json,
			                    is_latest, pinned, created_at, updated_at, path_search)
			 VALUES (?, ?, ?, ?, ?, 'semantic', 'corpo', ?, ?, 1, ?, ?, ?, ?)`,
			id.Bytes(), scope.WorkspaceID.Bytes(), scope.ProjectID.Bytes(), path, "título "+path,
			make([]byte, 32), fm, pin, updated.UnixMicro(), updated.UnixMicro(), path,
		); err != nil {
			return err
		}
		for i := 0; i < evidence; i++ {
			if _, err := tx.Exec(
				`INSERT INTO page_evidence (page_id, source_kind, source_id, created_at) VALUES (?, 'session', ?, ?)`,
				id.Bytes(), fmt.Sprintf("%s-%d", path, i), updated.UnixMicro(),
			); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		f.t.Fatalf("página %s: %v", path, err)
	}
}

// toolOutcome é a resposta de um tools/call: texto e marca de erro de ferramenta.
type toolOutcome struct {
	text    string
	isError bool
}

// call invoca uma ferramenta pelo protocolo (tools/call) com o ator dado no contexto.
func (f *sessionFixture) call(ctx context.Context, name string, args map[string]any) toolOutcome {
	f.t.Helper()
	resp := rpc(f.t, f.srv, ctx, 1, "tools/call", map[string]any{"name": name, "arguments": args})
	if resp == nil {
		f.t.Fatalf("%s: sem resposta", name)
	}
	if e, ok := resp["error"]; ok {
		f.t.Fatalf("%s: erro JSON-RPC inesperado: %v", name, e)
	}
	result, ok := resp["result"].(map[string]any)
	if !ok {
		f.t.Fatalf("%s: resultado ausente: %v", name, resp)
	}
	content, ok := result["content"].([]any)
	if !ok || len(content) == 0 {
		f.t.Fatalf("%s: conteúdo ausente: %v", name, result)
	}
	first, _ := content[0].(map[string]any)
	text, _ := first["text"].(string)
	isErr, _ := result["isError"].(bool)
	return toolOutcome{text: text, isError: isErr}
}

// ok decodifica uma resposta de sucesso em objeto JSON; falha o teste se for erro.
func (o toolOutcome) ok(t *testing.T) map[string]any {
	t.Helper()
	if o.isError {
		t.Fatalf("erro de ferramenta inesperado: %s", o.text)
	}
	var m map[string]any
	if err := json.Unmarshal([]byte(o.text), &m); err != nil {
		t.Fatalf("resposta não é JSON: %v: %s", err, o.text)
	}
	return m
}

// obsList devolve a lista "observations" da resposta.
func obsList(t *testing.T, m map[string]any) []map[string]any {
	t.Helper()
	raw, ok := m["observations"].([]any)
	if !ok {
		t.Fatalf("observations ausente ou inválido: %v", m["observations"])
	}
	out := make([]map[string]any, 0, len(raw))
	for _, r := range raw {
		out = append(out, r.(map[string]any))
	}
	return out
}

func numField(t *testing.T, m map[string]any, key string) int {
	t.Helper()
	v, ok := m[key].(float64)
	if !ok {
		t.Fatalf("campo %q ausente ou não numérico: %v", key, m[key])
	}
	return int(v)
}

func titles(items []map[string]any) []string {
	out := make([]string, 0, len(items))
	for _, it := range items {
		out = append(out, it["title"].(string))
	}
	return out
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// ---------------------------------------------------------------------------
// memory_read_session_observations
// ---------------------------------------------------------------------------

func TestRegisterSessionTools_ExigeLeitor(t *testing.T) {
	if err := RegisterSessionTools(NewToolServer(), SessionToolDeps{}); err == nil {
		t.Fatal("esperado erro para leitor nulo")
	}
}

func TestSessionObservations_PaginaComLimitOffsetEOrdem(t *testing.T) {
	f := newSessionFixture(t)
	sid := f.session("", baseTime, nil, f.main)
	for i := 0; i < 5; i++ {
		f.observation(sid, f.main, fmt.Sprintf("o%d", i), "corpo", baseTime.Add(time.Duration(i)*time.Second))
	}
	args := func(extra map[string]any) map[string]any {
		m := map[string]any{"session_id": sid.String()}
		for k, v := range extra {
			m[k] = v
		}
		return m
	}

	page1 := f.call(f.ctx, "memory_read_session_observations", args(map[string]any{"limit": 2})).ok(t)
	if got := titles(obsList(t, page1)); !equalStrings(got, []string{"o0", "o1"}) {
		t.Fatalf("asc limit 2: titles = %v, quer [o0 o1]", got)
	}
	if n := numField(t, page1, "total"); n != 5 {
		t.Fatalf("total = %d, quer 5", n)
	}
	if page1["order"] != "asc" || numField(t, page1, "limit") != 2 {
		t.Fatalf("order/limit na resposta: %v / %v", page1["order"], page1["limit"])
	}

	page2 := f.call(f.ctx, "memory_read_session_observations", args(map[string]any{"limit": 2, "offset": 2})).ok(t)
	if got := titles(obsList(t, page2)); !equalStrings(got, []string{"o2", "o3"}) {
		t.Fatalf("offset 2: titles = %v, quer [o2 o3]", got)
	}

	desc := f.call(f.ctx, "memory_read_session_observations", args(map[string]any{"limit": 2, "order": "desc"})).ok(t)
	if got := titles(obsList(t, desc)); !equalStrings(got, []string{"o4", "o3"}) {
		t.Fatalf("desc limit 2: titles = %v, quer [o4 o3]", got)
	}
	if desc["order"] != "desc" {
		t.Fatalf("order na resposta = %v, quer desc", desc["order"])
	}
}

func TestSessionObservations_PadroesELimitesDeLimitEBody(t *testing.T) {
	f := newSessionFixture(t)
	sid := f.session("", baseTime, nil, f.main)
	f.observation(sid, f.main, "unica", "corpo", baseTime)

	m := f.call(f.ctx, "memory_read_session_observations", map[string]any{"session_id": sid.String()}).ok(t)
	if numField(t, m, "limit") != 50 {
		t.Fatalf("limit padrão = %d, quer 50", numField(t, m, "limit"))
	}
	if numField(t, m, "body_max_chars") != 4000 {
		t.Fatalf("body_max_chars padrão = %d, quer 4000", numField(t, m, "body_max_chars"))
	}
	if m["order"] != "asc" {
		t.Fatalf("order padrão = %v, quer asc", m["order"])
	}

	cases := []struct {
		name string
		args map[string]any
		key  string
		want int
	}{
		{"limit acima do máximo", map[string]any{"limit": 999}, "limit", 200},
		{"limit zero", map[string]any{"limit": 0}, "limit", 1},
		{"body abaixo da faixa", map[string]any{"body_max_chars": 10}, "body_max_chars", 200},
		{"body acima da faixa", map[string]any{"body_max_chars": 999999}, "body_max_chars", 16384},
	}
	for _, c := range cases {
		args := map[string]any{"session_id": sid.String()}
		for k, v := range c.args {
			args[k] = v
		}
		got := f.call(f.ctx, "memory_read_session_observations", args).ok(t)
		if numField(t, got, c.key) != c.want {
			t.Fatalf("%s: %s = %d, quer %d", c.name, c.key, numField(t, got, c.key), c.want)
		}
	}
}

func TestSessionObservations_CorpoAcimaDoLimiteECortadoComMarcador(t *testing.T) {
	f := newSessionFixture(t)
	sid := f.session("", baseTime, nil, f.main)
	// Corpo longo com caracteres multibyte: o corte não pode quebrar UTF-8.
	long := strings.Repeat("é", 500)
	f.observation(sid, f.main, "longa", long, baseTime)
	f.observation(sid, f.main, "curta", "curto", baseTime.Add(time.Second))

	m := f.call(f.ctx, "memory_read_session_observations", map[string]any{
		"session_id":     sid.String(),
		"body_max_chars": 200,
	}).ok(t)
	items := obsList(t, m)
	if len(items) != 2 {
		t.Fatalf("itens = %d, quer 2", len(items))
	}
	wantLong := string([]rune(long)[:200]) + sessionObsTruncMarker
	if items[0]["body"] != wantLong {
		t.Fatalf("corpo cortado incorreto: len=%d, quer len=%d", len([]rune(items[0]["body"].(string))), len([]rune(wantLong)))
	}
	if items[0]["body_truncated"] != true {
		t.Fatalf("body_truncated = %v, quer true", items[0]["body_truncated"])
	}
	if items[1]["body"] != "curto" || items[1]["body_truncated"] != false {
		t.Fatalf("corpo curto alterado: %v / %v", items[1]["body"], items[1]["body_truncated"])
	}
}

func TestSessionObservations_OrderInvalidoEErroDeEntrada(t *testing.T) {
	f := newSessionFixture(t)
	sid := f.session("", baseTime, nil, f.main)
	f.observation(sid, f.main, "x", "corpo", baseTime)

	out := f.call(f.ctx, "memory_read_session_observations", map[string]any{
		"session_id": sid.String(),
		"order":      "aleatoria",
	})
	if !out.isError {
		t.Fatalf("order inválido deveria ser erro de ferramenta, veio: %s", out.text)
	}
	if !strings.Contains(out.text, "entrada inválida") {
		t.Fatalf("mensagem sem classe de entrada inválida: %q", out.text)
	}
}

func TestSessionObservations_SessaoInexistenteNaoEncontrado(t *testing.T) {
	f := newSessionFixture(t)
	missing := newTestID(t).String()

	out := f.call(f.ctx, "memory_read_session_observations", map[string]any{"session_id": missing})
	if !out.isError {
		t.Fatalf("sessão inexistente deveria ser erro, veio: %s", out.text)
	}
	if !strings.Contains(out.text, "não encontrado") {
		t.Fatalf("mensagem sem classe de não encontrado: %q", out.text)
	}
	if strings.Contains(out.text, missing) {
		t.Fatalf("mensagem ecoa o id informado: %q", out.text)
	}
}

func TestSessionObservations_SessaoDeOutroEscopoNaoEncontrado(t *testing.T) {
	f := newSessionFixture(t)
	sid := f.session("", baseTime, nil, f.other)
	f.observation(sid, f.other, "de outro projeto", "corpo", baseTime)

	// Lê pelo projeto `scratch`; a sessão pertence a `outro`.
	out := f.call(f.ctx, "memory_read_session_observations", map[string]any{
		"session_id": sid.String(),
		"project":    "scratch",
	})
	if !out.isError || !strings.Contains(out.text, "não encontrado") {
		t.Fatalf("sessão de outro escopo deveria ser não encontrado, veio: %+v", out)
	}
}

func TestSessionObservations_SemSessionIDUsaConcluidaMaisRecente(t *testing.T) {
	f := newSessionFixture(t)
	ended1h := baseTime.Add(time.Hour)
	ended2h := baseTime.Add(2 * time.Hour)

	older := f.session("", baseTime, &ended1h, f.main)
	f.observation(older, f.main, "antiga", "corpo", baseTime)

	newer := f.session("", baseTime.Add(time.Minute), &ended2h, f.main)
	f.observation(newer, f.main, "recente", "corpo", baseTime.Add(time.Hour))

	// Sessão aberta, iniciada depois de todas: não é concluída, não deve ser escolhida.
	open := f.session("", baseTime.Add(3*time.Hour), nil, f.main)
	f.observation(open, f.main, "aberta", "corpo", baseTime.Add(3*time.Hour))

	m := f.call(f.ctx, "memory_read_session_observations", map[string]any{}).ok(t)
	if m["session_id"] != newer.String() {
		t.Fatalf("session_id = %v, quer a concluída mais recente %s", m["session_id"], newer.String())
	}
	if got := titles(obsList(t, m)); !equalStrings(got, []string{"recente"}) {
		t.Fatalf("observações = %v, quer [recente]", got)
	}
}

func TestSessionObservations_SemConcluidaNaoEncontrado(t *testing.T) {
	f := newSessionFixture(t)
	open := f.session("", baseTime, nil, f.main)
	f.observation(open, f.main, "aberta", "corpo", baseTime)

	out := f.call(f.ctx, "memory_read_session_observations", map[string]any{})
	if !out.isError || !strings.Contains(out.text, "não encontrado") {
		t.Fatalf("sem sessão concluída deveria ser não encontrado, veio: %+v", out)
	}
}

func TestSessionObservations_FiltroDeDono(t *testing.T) {
	f := newSessionFixture(t)
	ended := baseTime.Add(time.Hour)
	shared := f.session("", baseTime, &ended, f.main)
	f.observation(shared, f.main, "compartilhada", "corpo", baseTime)

	endedAlice := baseTime.Add(2 * time.Hour)
	alice := f.session("alice", baseTime.Add(time.Minute), &endedAlice, f.main)
	f.observation(alice, f.main, "da alice", "corpo", baseTime.Add(time.Hour))

	anon := f.ctx
	aliceCtx := WithActorKey(f.ctx, ActorKey{User: "alice"})
	bobCtx := WithActorKey(f.ctx, ActorKey{User: "bob"})

	// Anônimo sem session_id: a mais recente visível é a compartilhada (a da alice não é visível).
	m := f.call(anon, "memory_read_session_observations", map[string]any{}).ok(t)
	if m["session_id"] != shared.String() {
		t.Fatalf("anônimo: session_id = %v, quer compartilhada", m["session_id"])
	}
	// Alice sem session_id: vê a sessão dela, que é a mais recente.
	m = f.call(aliceCtx, "memory_read_session_observations", map[string]any{}).ok(t)
	if m["session_id"] != alice.String() {
		t.Fatalf("alice: session_id = %v, quer a dela", m["session_id"])
	}
	// Anônimo pedindo a sessão da alice: não encontrado.
	out := f.call(anon, "memory_read_session_observations", map[string]any{"session_id": alice.String()})
	if !out.isError || !strings.Contains(out.text, "não encontrado") {
		t.Fatalf("anônimo lendo sessão da alice deveria ser não encontrado, veio: %+v", out)
	}
	// Bob pedindo a sessão da alice: não encontrado.
	out = f.call(bobCtx, "memory_read_session_observations", map[string]any{"session_id": alice.String()})
	if !out.isError || !strings.Contains(out.text, "não encontrado") {
		t.Fatalf("bob lendo sessão da alice deveria ser não encontrado, veio: %+v", out)
	}
	// Alice pedindo a própria sessão: acesso.
	m = f.call(aliceCtx, "memory_read_session_observations", map[string]any{"session_id": alice.String()}).ok(t)
	if got := titles(obsList(t, m)); !equalStrings(got, []string{"da alice"}) {
		t.Fatalf("alice lendo a própria sessão: %v", got)
	}
}

func TestSessionObservations_ElidedOtherScope(t *testing.T) {
	f := newSessionFixture(t)
	sid := f.session("", baseTime, nil, f.main)
	f.observation(sid, f.main, "a", "corpo", baseTime)
	f.observation(sid, f.main, "b", "corpo", baseTime.Add(time.Second))
	// Observação da mesma sessão gravada no outro projeto: sai do total e é contada como omitida.
	f.observation(sid, f.other, "de outro", "corpo", baseTime.Add(2*time.Second))

	m := f.call(f.ctx, "memory_read_session_observations", map[string]any{"session_id": sid.String()}).ok(t)
	if numField(t, m, "total") != 2 {
		t.Fatalf("total = %d, quer 2 (só o escopo resolvido)", numField(t, m, "total"))
	}
	if numField(t, m, "elided_other_scope") != 1 {
		t.Fatalf("elided_other_scope = %d, quer 1", numField(t, m, "elided_other_scope"))
	}
	if got := titles(obsList(t, m)); !equalStrings(got, []string{"a", "b"}) {
		t.Fatalf("observações = %v, quer [a b]", got)
	}
}

// ---------------------------------------------------------------------------
// memory_briefing
// ---------------------------------------------------------------------------

func TestBriefing_ContagensEPaginasRecentes(t *testing.T) {
	f := newSessionFixture(t)
	f.page(f.main, "notes/a.md", "", false, baseTime.Add(1*time.Hour), 0)
	f.page(f.main, "notes/b.md", "", false, baseTime.Add(3*time.Hour), 0)
	f.page(f.main, "notes/c.md", "", false, baseTime.Add(2*time.Hour), 0)

	sid := f.session("", baseTime, nil, f.main)
	f.observation(sid, f.main, "x", "corpo", baseTime)
	f.observation(sid, f.main, "y", "corpo", baseTime)

	m := f.call(f.ctx, "memory_briefing", map[string]any{}).ok(t)
	counts, _ := m["counts"].(map[string]any)
	if counts == nil {
		t.Fatalf("counts ausente: %v", m)
	}
	if numField(t, counts, "pages") != 3 {
		t.Fatalf("pages = %d, quer 3", numField(t, counts, "pages"))
	}
	if numField(t, counts, "observations") != 2 {
		t.Fatalf("observations = %d, quer 2", numField(t, counts, "observations"))
	}
	if numField(t, counts, "sessions") != 1 {
		t.Fatalf("sessions = %d, quer 1", numField(t, counts, "sessions"))
	}
	recent, _ := m["recent_pages"].([]any)
	if len(recent) != 3 {
		t.Fatalf("recent_pages = %d, quer 3", len(recent))
	}
	if first := recent[0].(map[string]any)["path"]; first != "notes/b.md" {
		t.Fatalf("página mais recente = %v, quer notes/b.md", first)
	}
	if _, has := m["pinned"]; has {
		t.Fatal("pinned presente sem páginas fixadas")
	}
	if _, has := m["settled"]; has {
		t.Fatal("settled presente sem settled_first")
	}
}

func TestBriefing_PinnedSomenteQuandoHaEAteDez(t *testing.T) {
	f := newSessionFixture(t)
	// Projeto sem páginas fixadas: a chave pinned some.
	f.page(f.other, "notes/a.md", "", false, baseTime, 0)
	empty := f.call(f.ctx, "memory_briefing", map[string]any{"project": "outro"}).ok(t)
	if _, has := empty["pinned"]; has {
		t.Fatal("pinned presente sem páginas fixadas no projeto")
	}

	// 12 fixadas e uma em _slots/ (que não entra em pinned).
	for i := 0; i < 12; i++ {
		f.page(f.main, fmt.Sprintf("notes/p%02d.md", i), "", true, baseTime.Add(time.Duration(i)*time.Minute), 0)
	}
	f.page(f.main, "_slots/estado.md", "", true, baseTime.Add(time.Hour), 0)

	m := f.call(f.ctx, "memory_briefing", map[string]any{}).ok(t)
	pinned, _ := m["pinned"].([]any)
	if len(pinned) != 10 {
		t.Fatalf("pinned = %d, quer 10", len(pinned))
	}
	for _, p := range pinned {
		path := p.(map[string]any)["path"].(string)
		if strings.HasPrefix(path, "_slots/") {
			t.Fatalf("slot apareceu em pinned: %s", path)
		}
	}
	if first := pinned[0].(map[string]any)["path"]; first != "notes/p11.md" {
		t.Fatalf("pinned mais recente = %v, quer notes/p11.md", first)
	}
}

func TestBriefing_SettledSomenteComFlagAteOito(t *testing.T) {
	f := newSessionFixture(t)

	type cand struct {
		path string
		kind string
		ev   int
		upd  time.Time
	}
	var cands []cand
	for i := 0; i < 6; i++ {
		cands = append(cands, cand{fmt.Sprintf("_rules/r%d.md", i), "", i % 3, baseTime.Add(time.Duration(i) * time.Hour)})
	}
	for i := 0; i < 4; i++ {
		cands = append(cands, cand{fmt.Sprintf("notes/d%d.md", i), "decision", (i + 1) % 3, baseTime.Add(time.Duration(10+i) * time.Hour)})
	}
	// Fora do conjunto: fato comum, mesmo com muita evidência.
	cands = append(cands, cand{"notes/fato.md", "fact", 9, baseTime.Add(20 * time.Hour)})

	for _, c := range cands {
		f.page(f.main, c.path, c.kind, false, c.upd, c.ev)
	}

	// Sem a flag: settled ausente.
	noFlag := f.call(f.ctx, "memory_briefing", map[string]any{}).ok(t)
	if _, has := noFlag["settled"]; has {
		t.Fatal("settled presente sem settled_first=true")
	}

	// Com a flag: até 8, por evidência desc e depois recência desc.
	var expected []cand
	for _, c := range cands {
		if c.kind == "fact" {
			continue
		}
		expected = append(expected, c)
	}
	sort.SliceStable(expected, func(i, j int) bool {
		if expected[i].ev != expected[j].ev {
			return expected[i].ev > expected[j].ev
		}
		return expected[i].upd.After(expected[j].upd)
	})
	want := make([]string, 0, 8)
	for _, c := range expected[:8] {
		want = append(want, c.path)
	}

	m := f.call(f.ctx, "memory_briefing", map[string]any{"settled_first": true}).ok(t)
	settled, ok := m["settled"].([]any)
	if !ok {
		t.Fatalf("settled ausente com settled_first=true: %v", m)
	}
	got := make([]string, 0, len(settled))
	for _, s := range settled {
		got = append(got, s.(map[string]any)["path"].(string))
	}
	if !equalStrings(got, want) {
		t.Fatalf("settled = %v\nquer      %v", got, want)
	}
}

func TestBriefing_SettledVazioComFlagAparecerComoLista(t *testing.T) {
	f := newSessionFixture(t)
	f.page(f.other, "notes/a.md", "", false, baseTime, 0)

	m := f.call(f.ctx, "memory_briefing", map[string]any{"project": "outro", "settled_first": true}).ok(t)
	settled, ok := m["settled"].([]any)
	if !ok || len(settled) != 0 {
		t.Fatalf("settled com flag e sem candidatas = %v, quer lista vazia", m["settled"])
	}
}
