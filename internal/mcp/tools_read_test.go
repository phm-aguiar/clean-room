package mcp

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/phm-aguiar/ai-memory-go/internal/domain"
	"github.com/phm-aguiar/ai-memory-go/internal/storage"
)

// rtEnv é o ambiente dos testes de leitura: banco real em diretório temporário, servidor MCP com as
// quatro ferramentas registradas e um ator padrão.
type rtEnv struct {
	t     *testing.T
	dir   string
	db    *sql.DB
	w     storage.Writer
	ptr   *storage.ActiveProjectPointer
	srv   ToolServer
	ctx   context.Context
	t0    time.Time
	wsIDs map[string]domain.WorkspaceID
	actor storage.ActorKey
}

// rtOpts configura o ambiente: raiz da wiki para served_from e se há Writer para o reforço de acesso.
type rtOpts struct {
	wikiRoot string
	writer   bool
}

func newRTEnv(t *testing.T, opts rtOpts) *rtEnv {
	t.Helper()
	ctx := context.Background()
	dir := t.TempDir()
	db, err := storage.Open(ctx, dir)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	w, err := storage.NewWriter(ctx, db)
	if err != nil {
		t.Fatalf("NewWriter: %v", err)
	}
	t.Cleanup(func() { _ = w.Close() })
	rdb, err := storage.OpenReader(ctx, dir)
	if err != nil {
		t.Fatalf("OpenReader: %v", err)
	}
	t.Cleanup(func() { _ = rdb.Close() })
	// MaxEntries 0 vale 1 (docs/auto-scope.md): com um só slot, a entrada da sessão seria expulsa pelo
	// slot de identidade. O teste precisa de espaço para as duas entradas que Publish grava.
	ptr, err := storage.NewActiveProjectPointer(storage.ActiveProjectConfig{Mode: "per_actor", MaxEntries: 16}, nil)
	if err != nil {
		t.Fatalf("NewActiveProjectPointer: %v", err)
	}

	e := &rtEnv{
		t: t, dir: dir, db: db, w: w, ptr: ptr,
		ctx:   WithActorKey(ctx, ActorKey{User: "user:teste", SessionID: "sess-1"}),
		t0:    time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC),
		wsIDs: map[string]domain.WorkspaceID{},
		actor: storage.ActorKey{Identity: "user:teste", SessionID: "sess-1"},
	}
	var writer storage.Writer
	if opts.writer {
		writer = w
	}
	srv := NewToolServer()
	err = RegisterReadTools(srv, ReadDeps{
		Reader:   storage.NewReader(rdb),
		Writer:   writer,
		Pointer:  ptr,
		WikiRoot: opts.wikiRoot,
		Now:      func() time.Time { return e.t0 },
	})
	if err != nil {
		t.Fatalf("RegisterReadTools: %v", err)
	}
	e.srv = srv
	return e
}

// scope devolve o escopo de um par workspace/projeto, criando-o se faltar. Workspaces com o mesmo nome
// reusam o mesmo id.
func (e *rtEnv) scope(wsName, pjName string) domain.Scope {
	e.t.Helper()
	wsID, ok := e.wsIDs[wsName]
	if !ok {
		id, err := domain.NewID()
		if err != nil {
			e.t.Fatalf("NewID ws: %v", err)
		}
		wsID = domain.WorkspaceID(id)
		e.wsIDs[wsName] = wsID
	}
	pjID, err := domain.NewID()
	if err != nil {
		e.t.Fatalf("NewID pj: %v", err)
	}
	sc := domain.NewScope(wsID, domain.ProjectID(pjID))
	if err := storage.EnsureScope(e.ctx, e.w, sc, wsName, pjName, e.t0); err != nil {
		e.t.Fatalf("EnsureScope: %v", err)
	}
	return sc
}

// pageOpts são opções de uma página de teste.
type pageOpts struct {
	pinned bool
	links  []storage.Link
	fm     domain.Frontmatter
}

// put grava uma versão de página no instante t0 por UpsertPage.
func (e *rtEnv) put(sc domain.Scope, path, title, body string, o pageOpts) {
	e.t.Helper()
	pp, err := domain.ParsePagePath(path)
	if err != nil {
		e.t.Fatalf("ParsePagePath %q: %v", path, err)
	}
	_, err = storage.UpsertPage(e.ctx, e.w, storage.UpsertInput{
		Ref:         domain.NewPageRef(sc, pp),
		Tier:        domain.TierSemantic,
		Kind:        domain.PageKindNote,
		Title:       title,
		Body:        body,
		Frontmatter: o.fm,
		Pinned:      o.pinned,
		Links:       o.links,
		Evidence:    storage.EvidenceSource{Kind: storage.SourceSession, ID: "ev-" + path},
		At:          e.t0,
	})
	if err != nil {
		e.t.Fatalf("UpsertPage %q: %v", path, err)
	}
}

// link monta um link de saída para um caminho do mesmo escopo, ou de outro (ws e pj não vazios).
func link(t *testing.T, path string, ws, pj string) storage.Link {
	t.Helper()
	pp, err := domain.ParsePagePath(path)
	if err != nil {
		t.Fatalf("ParsePagePath %q: %v", path, err)
	}
	l := storage.Link{ToPath: pp}
	if ws != "" {
		l.ToWorkspace = &ws
	}
	if pj != "" {
		l.ToProject = &pj
	}
	return l
}

// exec roda SQL de apoio nos testes (ajuste de datas e expiração).
func (e *rtEnv) exec(query string, args ...any) {
	e.t.Helper()
	if _, err := e.db.Exec(query, args...); err != nil {
		e.t.Fatalf("exec %q: %v", query, err)
	}
}

// call chama uma ferramenta pelo protocolo e devolve o texto e a marca isError.
func (e *rtEnv) call(name string, args map[string]any) (string, bool) {
	e.t.Helper()
	return callWith(e.t, e.srv, e.ctx, name, args)
}

// callJSON chama uma ferramenta que deve ter sucesso e devolve o JSON da resposta.
func (e *rtEnv) callJSON(name string, args map[string]any) map[string]any {
	e.t.Helper()
	text, isErr := e.call(name, args)
	if isErr {
		e.t.Fatalf("%s retornou erro: %s", name, text)
	}
	return decodeText(e.t, text)
}

// callErr chama uma ferramenta que deve falhar e devolve o texto do erro.
func (e *rtEnv) callErr(name string, args map[string]any) string {
	e.t.Helper()
	text, isErr := e.call(name, args)
	if !isErr {
		e.t.Fatalf("%s deveria falhar, veio sucesso: %s", name, text)
	}
	if strings.Contains(text, e.dir) {
		e.t.Fatalf("erro de %s vaza caminho do sistema de arquivos: %s", name, text)
	}
	return text
}

// callWith chama tools/call pelo servidor e extrai o texto e isError do resultado.
func callWith(t *testing.T, srv ToolServer, ctx context.Context, name string, args map[string]any) (string, bool) {
	t.Helper()
	resp := rpc(t, srv, ctx, 7, "tools/call", map[string]any{"name": name, "arguments": args})
	if e, has := resp["error"]; has {
		t.Fatalf("erro de protocolo em %s: %v", name, e)
	}
	result, ok := resp["result"].(map[string]any)
	if !ok {
		t.Fatalf("result ausente em %s: %v", name, resp)
	}
	content, ok := result["content"].([]any)
	if !ok || len(content) != 1 {
		t.Fatalf("content inesperado em %s: %v", name, result)
	}
	item, _ := content[0].(map[string]any)
	text, _ := item["text"].(string)
	isErr, _ := result["isError"].(bool)
	return text, isErr
}

func decodeText(t *testing.T, text string) map[string]any {
	t.Helper()
	var out map[string]any
	if err := json.Unmarshal([]byte(text), &out); err != nil {
		t.Fatalf("resposta não é JSON %q: %v", text, err)
	}
	return out
}

// paths extrai os caminhos de uma lista de hits.
func paths(t *testing.T, list any) []string {
	t.Helper()
	items, ok := list.([]any)
	if !ok {
		t.Fatalf("lista esperada, veio %T: %v", list, list)
	}
	out := make([]string, 0, len(items))
	for _, it := range items {
		m, _ := it.(map[string]any)
		out = append(out, fmt.Sprint(m["path"]))
	}
	return out
}

func contains(list []string, want string) bool {
	for _, s := range list {
		if s == want {
			return true
		}
	}
	return false
}

// scopeArgsOf monta workspace e project como argumentos.
func scopeArgsOf(ws, pj string) map[string]any {
	return map[string]any{"workspace": ws, "project": pj}
}

// withArgs junta argumentos em um novo mapa.
func withArgs(base map[string]any, extra map[string]any) map[string]any {
	out := map[string]any{}
	for k, v := range base {
		out[k] = v
	}
	for k, v := range extra {
		out[k] = v
	}
	return out
}

// ---------------------------------------------------------------------------
// Registro e schemas
// ---------------------------------------------------------------------------

func TestRegistraQuatroFerramentasComSchema(t *testing.T) {
	e := newRTEnv(t, rtOpts{})
	resp := rpc(t, e.srv, e.ctx, 1, "tools/list", nil)
	result, _ := resp["result"].(map[string]any)
	tools, _ := result["tools"].([]any)

	want := map[string]bool{"memory_query": false, "memory_recent": false, "memory_read_page": false, "memory_status": false}
	for _, raw := range tools {
		tool, _ := raw.(map[string]any)
		name, _ := tool["name"].(string)
		if _, ok := want[name]; !ok {
			continue
		}
		want[name] = true
		schemaRaw, _ := json.Marshal(tool["inputSchema"])
		var schema map[string]any
		if err := json.Unmarshal(schemaRaw, &schema); err != nil {
			t.Fatalf("schema de %s inválido: %v", name, err)
		}
		if schema["type"] != "object" || schema["additionalProperties"] != false {
			t.Fatalf("schema de %s sem objeto fechado: %v", name, schema)
		}
	}
	for name, found := range want {
		if !found {
			t.Fatalf("ferramenta %s não registrada: %v", name, want)
		}
	}
}

func TestRegisterReadToolsRecusaDependenciasNulas(t *testing.T) {
	if err := RegisterReadTools(nil, ReadDeps{}); err == nil {
		t.Fatal("servidor nulo deveria ser recusado")
	}
	if err := RegisterReadTools(NewToolServer(), ReadDeps{}); err == nil {
		t.Fatal("Reader nulo deveria ser recusado")
	}
}

// ---------------------------------------------------------------------------
// memory_query
// ---------------------------------------------------------------------------

func TestMemoryQuery_BuscaFTSRetornaHitsComEscopo(t *testing.T) {
	e := newRTEnv(t, rtOpts{})
	sc := e.scope("ws-a", "proj-a")
	e.put(sc, "notas/pg.md", "Postgres", "replicação do postgres", pageOpts{})
	e.put(sc, "notas/outra.md", "Outra", "assunto diferente", pageOpts{})

	got := e.callJSON("memory_query", withArgs(scopeArgsOf("ws-a", "proj-a"), map[string]any{"query": "postgres"}))
	hits, _ := got["hits"].([]any)
	if p := paths(t, got["hits"]); len(p) != 1 || p[0] != "notas/pg.md" {
		t.Fatalf("hits = %v, quer [notas/pg.md]", p)
	}
	if got["raw_hits"] == nil {
		t.Fatal("raw_hits deve estar presente")
	}
	if len(hits) == 0 {
		t.Fatal("hits vazio")
	}
	first := hits[0].(map[string]any)
	if first["workspace"] != "ws-a" || first["project"] != "proj-a" {
		t.Fatalf("hit sem rótulo de escopo: %v", first)
	}
	scope := got["scope"].(map[string]any)
	if scope["resolved_by"] != "explicit" {
		t.Fatalf("resolved_by = %v, quer explicit", scope["resolved_by"])
	}
}

func TestMemoryQuery_SemResultadoDevolveListaVazia(t *testing.T) {
	e := newRTEnv(t, rtOpts{})
	e.scope("ws-a", "proj-a")
	got := e.callJSON("memory_query", withArgs(scopeArgsOf("ws-a", "proj-a"), map[string]any{"query": "inexistente"}))
	if _, ok := got["hits"].([]any); !ok {
		t.Fatalf("hits deve ser lista vazia, veio %v", got["hits"])
	}
}

func TestMemoryQuery_AliasesERespeitoDoLimite(t *testing.T) {
	e := newRTEnv(t, rtOpts{})
	sc := e.scope("ws-a", "proj-a")
	e.put(sc, "notas/a.md", "A", "postgres um", pageOpts{})
	e.put(sc, "notas/b.md", "B", "postgres dois", pageOpts{})
	e.put(sc, "notas/c.md", "C", "postgres tres", pageOpts{})
	base := scopeArgsOf("ws-a", "proj-a")

	// alias q para query
	got := e.callJSON("memory_query", withArgs(base, map[string]any{"q": "postgres"}))
	if n := len(paths(t, got["hits"])); n != 3 {
		t.Fatalf("alias q: %d hits, quer 3", n)
	}
	// alias search para query
	got = e.callJSON("memory_query", withArgs(base, map[string]any{"search": "postgres"}))
	if n := len(paths(t, got["hits"])); n != 3 {
		t.Fatalf("alias search: %d hits, quer 3", n)
	}
	// alias n limita
	got = e.callJSON("memory_query", withArgs(base, map[string]any{"query": "postgres", "n": 1}))
	if n := len(paths(t, got["hits"])); n != 1 {
		t.Fatalf("alias n: %d hits, quer 1", n)
	}
	// alias top_k limita
	got = e.callJSON("memory_query", withArgs(base, map[string]any{"query": "postgres", "top_k": 2}))
	if n := len(paths(t, got["hits"])); n != 2 {
		t.Fatalf("alias top_k: %d hits, quer 2", n)
	}
	// canônico vence o alias quando os valores diferem
	got = e.callJSON("memory_query", withArgs(base, map[string]any{"query": "postgres", "limit": 1, "n": 3}))
	if n := len(paths(t, got["hits"])); n != 1 {
		t.Fatalf("canônico limit deve vencer n: %d hits, quer 1", n)
	}
	// query canônico vence o alias q quando os valores diferem
	got = e.callJSON("memory_query", withArgs(base, map[string]any{"query": "postgres", "q": "inexistente"}))
	if n := len(paths(t, got["hits"])); n != 3 {
		t.Fatalf("query canônico deve vencer q: %d hits, quer 3", n)
	}
}

func TestMemoryQuery_LimiteForaDaFaixaELIMITADO(t *testing.T) {
	e := newRTEnv(t, rtOpts{})
	sc := e.scope("ws-a", "proj-a")
	e.put(sc, "notas/a.md", "A", "postgres", pageOpts{})
	base := scopeArgsOf("ws-a", "proj-a")
	for _, lim := range []int{0, -5, 500} {
		got := e.callJSON("memory_query", withArgs(base, map[string]any{"query": "postgres", "limit": lim}))
		if n := len(paths(t, got["hits"])); n != 1 {
			t.Fatalf("limit %d: %d hits, quer 1", lim, n)
		}
	}
}

func TestMemoryQuery_RecusaCombinacoesProibidas(t *testing.T) {
	e := newRTEnv(t, rtOpts{})
	e.scope("ws-a", "proj-a")
	pair := []map[string]any{{"workspace": "ws-a", "project": "proj-a"}}
	big := make([]map[string]any, 0, storage.MaxScopeList+1)
	for i := 0; i <= storage.MaxScopeList; i++ {
		big = append(big, map[string]any{"workspace": "ws-a", "project": "proj-a"})
	}
	cases := map[string]map[string]any{
		"global com workspace": {"query": "x", "global": true, "workspace": "ws-a"},
		"global com project":   {"query": "x", "global": true, "project": "proj-a"},
		"global com scopes":    {"query": "x", "global": true, "scopes": pair},
		"scopes com workspace": {"query": "x", "scopes": pair, "workspace": "ws-a"},
		"scopes com project":   {"query": "x", "scopes": pair, "project": "proj-a"},
		"scopes acima de 25":   {"query": "x", "scopes": big},
		"scopes vazio":         {"query": "x", "scopes": []map[string]any{}},
		"só workspace":         {"query": "x", "workspace": "ws-a"},
		"sem query":            {"workspace": "ws-a", "project": "proj-a"},
		"as_of não suportado":  {"query": "x", "as_of": "2026-01-01T00:00:00Z", "workspace": "ws-a", "project": "proj-a"},
	}
	for name, args := range cases {
		t.Run(name, func(t *testing.T) {
			text := e.callErr("memory_query", args)
			if !strings.Contains(text, "entrada inválida") {
				t.Fatalf("erro sem classe de entrada inválida: %s", text)
			}
		})
	}
}

func TestMemoryQuery_ScopesMesclaDoisProjetos(t *testing.T) {
	e := newRTEnv(t, rtOpts{})
	a := e.scope("ws-a", "proj-a")
	b := e.scope("ws-a", "proj-b")
	e.put(a, "notas/a.md", "A", "postgres em a", pageOpts{})
	e.put(b, "notas/b.md", "B", "postgres em b", pageOpts{})
	got := e.callJSON("memory_query", map[string]any{
		"query":  "postgres",
		"scopes": []map[string]any{scopeArgsOf("ws-a", "proj-a"), scopeArgsOf("ws-a", "proj-b")},
	})
	projects := map[string]bool{}
	for _, h := range got["hits"].([]any) {
		m := h.(map[string]any)
		projects[fmt.Sprint(m["project"])] = true
	}
	if !projects["proj-a"] || !projects["proj-b"] {
		t.Fatalf("scopes deve trazer os dois projetos, veio %v", projects)
	}
	if _, has := got["scope"]; has {
		t.Fatal("scopes não deve trazer bloco scope de projeto único")
	}
}

func TestMemoryQuery_ReasoningInvalidoERecusado(t *testing.T) {
	e := newRTEnv(t, rtOpts{})
	sc := e.scope("ws-a", "proj-a")
	e.put(sc, "notas/a.md", "A", "postgres", pageOpts{})
	base := scopeArgsOf("ws-a", "proj-a")

	text := e.callErr("memory_query", withArgs(base, map[string]any{"query": "postgres", "reasoning": "ultra"}))
	if !strings.Contains(text, "reasoning") {
		t.Fatalf("erro de reasoning deve citar o campo: %s", text)
	}
	for _, level := range []string{"minimal", "low", "medium", "high", "max"} {
		e.callJSON("memory_query", withArgs(base, map[string]any{"query": "postgres", "reasoning": level}))
	}
}

func TestMemoryQuery_AnswerDevolveNotaSemErro(t *testing.T) {
	e := newRTEnv(t, rtOpts{})
	sc := e.scope("ws-a", "proj-a")
	e.put(sc, "notas/a.md", "A", "postgres", pageOpts{})
	got := e.callJSON("memory_query", withArgs(scopeArgsOf("ws-a", "proj-a"), map[string]any{"query": "postgres", "answer": true}))
	if note, _ := got["answer_unavailable"].(string); !strings.Contains(note, "answer_unavailable") {
		t.Fatalf("answer_unavailable ausente: %v", got)
	}
	if _, has := got["answer"]; has {
		t.Fatal("sem provedor não deve haver campo answer")
	}
	if n := len(paths(t, got["hits"])); n != 1 {
		t.Fatalf("resultados devem ser mantidos com answer: %d hits", n)
	}
}

func TestMemoryQuery_GlobalDevolveGlobalHitsRotulados(t *testing.T) {
	e := newRTEnv(t, rtOpts{})
	a := e.scope("ws-a", "proj-a")
	b := e.scope("ws-b", "proj-b")
	e.put(a, "notas/a.md", "A", "postgres em a", pageOpts{})
	e.put(b, "notas/b.md", "B", "postgres em b", pageOpts{})

	got := e.callJSON("memory_query", map[string]any{"query": "postgres", "global": true})
	if _, has := got["hits"]; has {
		t.Fatal("consulta global não devolve hits de projeto único")
	}
	global, _ := got["global_hits"].([]any)
	seen := map[string]string{}
	for _, h := range global {
		m := h.(map[string]any)
		seen[fmt.Sprint(m["workspace"])+"/"+fmt.Sprint(m["project"])] = fmt.Sprint(m["path"])
	}
	if seen["ws-a/proj-a"] != "notas/a.md" || seen["ws-b/proj-b"] != "notas/b.md" {
		t.Fatalf("global_hits sem os dois rótulos: %v", seen)
	}
}

func TestMemoryQuery_DefaultGlobalDoPonteiroSemArgumentoDeEscopo(t *testing.T) {
	e := newRTEnv(t, rtOpts{})
	a := e.scope("ws-a", "proj-a")
	e.put(a, "notas/a.md", "A", "postgres", pageOpts{})
	e.ptr.Publish(e.actor, a, true, true, e.t0)

	got := e.callJSON("memory_query", map[string]any{"query": "postgres"})
	if _, has := got["global_hits"]; !has {
		t.Fatalf("default_global do ponteiro deveria ligar a consulta global: %v", got)
	}

	// Argumento explícito vence o padrão do ponteiro.
	got = e.callJSON("memory_query", map[string]any{"query": "postgres", "global": false})
	if _, has := got["hits"]; !has {
		t.Fatalf("global=false explícito deveria vencer o padrão do ponteiro: %v", got)
	}
	if _, has := got["global_hits"]; has {
		t.Fatal("global=false não deve devolver global_hits")
	}
}

func TestMemoryQuery_PinFirstAntepoePinnadas(t *testing.T) {
	e := newRTEnv(t, rtOpts{})
	sc := e.scope("ws-a", "proj-a")
	e.put(sc, "notas/fixada.md", "Fixada", "assunto sem o termo", pageOpts{pinned: true})
	e.put(sc, "notas/pg.md", "Postgres", "postgres aqui", pageOpts{})

	got := e.callJSON("memory_query", withArgs(scopeArgsOf("ws-a", "proj-a"), map[string]any{"query": "postgres", "pin_first": true}))
	p := paths(t, got["hits"])
	if len(p) < 2 || p[0] != "notas/fixada.md" {
		t.Fatalf("pin_first deve antepor a fixada: %v", p)
	}
	first := got["hits"].([]any)[0].(map[string]any)
	if first["pinned"] != true {
		t.Fatalf("página antepost a deve vir marcada pinned: %v", first)
	}
}

func TestMemoryQuery_ExplainDevolveFluxosEScoreDetails(t *testing.T) {
	e := newRTEnv(t, rtOpts{})
	sc := e.scope("ws-a", "proj-a")
	e.put(sc, "notas/pg.md", "Postgres", "postgres aqui", pageOpts{})

	got := e.callJSON("memory_query", withArgs(scopeArgsOf("ws-a", "proj-a"), map[string]any{"query": "postgres", "explain": true}))
	streams := fmt.Sprint(got["streams_active"])
	if streams != "[fts entity graph]" {
		t.Fatalf("streams_active = %s, quer [fts entity graph]", streams)
	}
	hit := got["hits"].([]any)[0].(map[string]any)
	details, _ := hit["score_details"].(map[string]any)
	if details == nil || !contains(toStrings(details["streams"]), "fts") {
		t.Fatalf("score_details deve listar fts: %v", hit["score_details"])
	}

	gotGlobal := e.callJSON("memory_query", map[string]any{"query": "postgres", "global": true, "explain": true})
	if fmt.Sprint(gotGlobal["streams_active"]) != "[fts]" {
		t.Fatalf("consulta global deve listar só fts: %v", gotGlobal["streams_active"])
	}
}

func TestMemoryQuery_IncludeExpiredTrazVencidas(t *testing.T) {
	e := newRTEnv(t, rtOpts{})
	sc := e.scope("ws-a", "proj-a")
	e.put(sc, "notas/vencida.md", "Vencida", "postgres vencido", pageOpts{})
	e.exec(`UPDATE pages SET expires_at = ? WHERE path = ? AND is_latest = 1`, e.t0.Add(-time.Hour).UnixMicro(), "notas/vencida.md")
	base := scopeArgsOf("ws-a", "proj-a")

	got := e.callJSON("memory_query", withArgs(base, map[string]any{"query": "postgres"}))
	if contains(paths(t, got["hits"]), "notas/vencida.md") {
		t.Fatal("página vencida não deve aparecer sem include_expired")
	}
	got = e.callJSON("memory_query", withArgs(base, map[string]any{"query": "postgres", "include_expired": true}))
	if !contains(paths(t, got["hits"]), "notas/vencida.md") {
		t.Fatal("include_expired deve trazer a página vencida")
	}
}

func TestMemoryQuery_EscopoInexistenteFalhaFechado(t *testing.T) {
	e := newRTEnv(t, rtOpts{})
	text := e.callErr("memory_query", withArgs(scopeArgsOf("nao-existe", "tambem-nao"), map[string]any{"query": "x"}))
	if !strings.Contains(text, "não encontrado") {
		t.Fatalf("escopo inexistente deve dizer não encontrado: %s", text)
	}
}

// ---------------------------------------------------------------------------
// memory_read_page
// ---------------------------------------------------------------------------

func TestMemoryReadPage_PorPathDevolveCorpoEFrontmatter(t *testing.T) {
	e := newRTEnv(t, rtOpts{})
	sc := e.scope("ws-a", "proj-a")
	fm := domain.Frontmatter{}
	if err := fm.Set("fonte", "teste"); err != nil {
		t.Fatalf("Set: %v", err)
	}
	e.put(sc, "notas/a.md", "Título A", "corpo completo", pageOpts{fm: fm})

	got := e.callJSON("memory_read_page", withArgs(scopeArgsOf("ws-a", "proj-a"), map[string]any{"path": "notas/a.md"}))
	if got["path"] != "notas/a.md" || got["title"] != "Título A" || got["body"] != "corpo completo" {
		t.Fatalf("leitura por path incorreta: %v", got)
	}
	front, _ := got["frontmatter"].(map[string]any)
	if front["fonte"] != "teste" {
		t.Fatalf("frontmatter não devolvido: %v", got["frontmatter"])
	}
	if _, has := got["related"]; has {
		t.Fatal("related não deve aparecer sem include_related")
	}
}

func TestMemoryReadPage_PorQueryDevolvePrimeiroResultado(t *testing.T) {
	e := newRTEnv(t, rtOpts{})
	sc := e.scope("ws-a", "proj-a")
	e.put(sc, "notas/pg.md", "Postgres", "corpo sobre postgres", pageOpts{})

	got := e.callJSON("memory_read_page", withArgs(scopeArgsOf("ws-a", "proj-a"), map[string]any{"query": "postgres"}))
	if got["path"] != "notas/pg.md" || got["body"] != "corpo sobre postgres" {
		t.Fatalf("leitura por query incorreta: %v", got)
	}
}

func TestMemoryReadPage_PathTemPrecedenciaSobreQuery(t *testing.T) {
	e := newRTEnv(t, rtOpts{})
	sc := e.scope("ws-a", "proj-a")
	e.put(sc, "notas/a.md", "A", "conteudo a", pageOpts{})
	e.put(sc, "notas/pg.md", "Postgres", "postgres aqui", pageOpts{})

	got := e.callJSON("memory_read_page", withArgs(scopeArgsOf("ws-a", "proj-a"), map[string]any{"path": "notas/a.md", "query": "postgres"}))
	if got["path"] != "notas/a.md" {
		t.Fatalf("path deve vencer query: %v", got["path"])
	}
}

func TestMemoryReadPage_ErrosDeEntradaEAusencia(t *testing.T) {
	e := newRTEnv(t, rtOpts{})
	sc := e.scope("ws-a", "proj-a")
	e.put(sc, "notas/a.md", "A", "conteudo", pageOpts{})
	base := scopeArgsOf("ws-a", "proj-a")

	if text := e.callErr("memory_read_page", base); !strings.Contains(text, "entrada inválida") {
		t.Fatalf("sem path nem query deve ser entrada inválida: %s", text)
	}
	if text := e.callErr("memory_read_page", withArgs(base, map[string]any{"path": "../fora.md"})); !strings.Contains(text, "entrada inválida") {
		t.Fatalf("caminho inválido deve ser entrada inválida: %s", text)
	}
	if text := e.callErr("memory_read_page", withArgs(base, map[string]any{"path": "notas/nao-existe.md"})); !strings.Contains(text, "não encontrado") {
		t.Fatalf("página inexistente deve ser não encontrado: %s", text)
	}
	if text := e.callErr("memory_read_page", withArgs(base, map[string]any{"query": "palavra-ausente"})); !strings.Contains(text, "não encontrado") {
		t.Fatalf("query sem casamento deve ser não encontrado: %s", text)
	}
}

func TestMemoryReadPage_IncludeRelatedEBacklinks(t *testing.T) {
	e := newRTEnv(t, rtOpts{})
	sc := e.scope("ws-a", "proj-a")
	e.put(sc, "notas/b.md", "B", "destino", pageOpts{})
	e.put(sc, "notas/a.md", "A", "origem", pageOpts{links: []storage.Link{link(t, "notas/b.md", "", "")}})
	e.put(sc, "notas/c.md", "C", "aponta para a", pageOpts{links: []storage.Link{link(t, "notas/a.md", "", "")}})
	base := withArgs(scopeArgsOf("ws-a", "proj-a"), map[string]any{"path": "notas/a.md"})

	got := e.callJSON("memory_read_page", withArgs(base, map[string]any{"include_related": true}))
	related, _ := got["related"].([]any)
	dirs := map[string]string{}
	for _, r := range related {
		m := r.(map[string]any)
		if m["depth"] != float64(1) {
			t.Fatalf("profundidade padrão deve ser 1: %v", m)
		}
		dirs[fmt.Sprint(m["path"])] = fmt.Sprint(m["direction"])
	}
	if dirs["notas/b.md"] != "link" || dirs["notas/c.md"] != "backlink" {
		t.Fatalf("related deve trazer link e backlink: %v", dirs)
	}
}

func TestMemoryReadPage_RelatedDepthAteTres(t *testing.T) {
	e := newRTEnv(t, rtOpts{})
	sc := e.scope("ws-a", "proj-a")
	// Cadeia a -> b -> c -> d -> e, criada do fim para o começo para que os links resolvam.
	e.put(sc, "notas/e.md", "E", "e", pageOpts{})
	e.put(sc, "notas/d.md", "D", "d", pageOpts{links: []storage.Link{link(t, "notas/e.md", "", "")}})
	e.put(sc, "notas/c.md", "C", "c", pageOpts{links: []storage.Link{link(t, "notas/d.md", "", "")}})
	e.put(sc, "notas/b.md", "B", "b", pageOpts{links: []storage.Link{link(t, "notas/c.md", "", "")}})
	e.put(sc, "notas/a.md", "A", "a", pageOpts{links: []storage.Link{link(t, "notas/b.md", "", "")}})
	base := withArgs(scopeArgsOf("ws-a", "proj-a"), map[string]any{"path": "notas/a.md", "include_related": true})

	depthOf := func(depth any) []string {
		got := e.callJSON("memory_read_page", withArgs(base, map[string]any{"related_depth": depth}))
		var out []string
		for _, r := range got["related"].([]any) {
			out = append(out, fmt.Sprint(r.(map[string]any)["path"]))
		}
		return out
	}
	if p := depthOf(1); len(p) != 1 || p[0] != "notas/b.md" {
		t.Fatalf("depth 1 = %v, quer [b]", p)
	}
	if p := depthOf(3); len(p) != 3 || contains(p, "notas/e.md") {
		t.Fatalf("depth 3 = %v, quer [b c d]", p)
	}
	if p := depthOf(9); len(p) != 3 || contains(p, "notas/e.md") {
		t.Fatalf("depth acima de 3 deve ser limitado a 3: %v", p)
	}
}

func TestMemoryReadPage_RelatedAtravessaProjetos(t *testing.T) {
	e := newRTEnv(t, rtOpts{})
	p1 := e.scope("ws-a", "proj-um")
	p2 := e.scope("ws-a", "proj-dois")
	e.put(p2, "notas/x.md", "X", "alvo em outro projeto", pageOpts{})
	e.put(p1, "notas/a.md", "A", "origem", pageOpts{links: []storage.Link{link(t, "notas/x.md", "ws-a", "proj-dois")}})

	got := e.callJSON("memory_read_page", withArgs(scopeArgsOf("ws-a", "proj-um"), map[string]any{"path": "notas/a.md", "include_related": true}))
	related := got["related"].([]any)
	if len(related) != 1 {
		t.Fatalf("related = %v, quer 1 item de outro projeto", related)
	}
	item := related[0].(map[string]any)
	if item["project"] != "proj-dois" || item["workspace"] != "ws-a" || item["direction"] != "link" {
		t.Fatalf("item cross-project incorreto: %v", item)
	}
}

func TestMemoryReadPage_ServedFromQuandoArquivoFalta(t *testing.T) {
	wikiRoot := t.TempDir()
	e := newRTEnv(t, rtOpts{wikiRoot: wikiRoot})
	sc := e.scope("ws-a", "proj-a")
	e.put(sc, "notas/a.md", "A", "corpo", pageOpts{})
	args := withArgs(scopeArgsOf("ws-a", "proj-a"), map[string]any{"path": "notas/a.md"})

	got := e.callJSON("memory_read_page", args)
	if got["served_from"] != "database" {
		t.Fatalf("arquivo ausente deve marcar served_from=database: %v", got["served_from"])
	}

	file := filepath.Join(wikiRoot, sc.WorkspaceID.String(), sc.ProjectID.String(), "notas", "a.md")
	if err := os.MkdirAll(filepath.Dir(file), 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(file, []byte("corpo"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	got = e.callJSON("memory_read_page", args)
	if _, has := got["served_from"]; has {
		t.Fatalf("arquivo presente não deve trazer served_from: %v", got["served_from"])
	}
}

func TestMemoryReadPage_ReforcaAcessoDoOperador(t *testing.T) {
	e := newRTEnv(t, rtOpts{writer: true})
	sc := e.scope("ws-a", "proj-a")
	e.put(sc, "notas/a.md", "A", "corpo", pageOpts{})

	e.callJSON("memory_read_page", withArgs(scopeArgsOf("ws-a", "proj-a"), map[string]any{"path": "notas/a.md"}))

	deadline := time.Now().Add(3 * time.Second)
	for {
		var n int
		if err := e.db.QueryRow(`SELECT access_count FROM pages WHERE path = ? AND is_latest = 1`, "notas/a.md").Scan(&n); err != nil {
			t.Fatalf("ler access_count: %v", err)
		}
		if n == 1 {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("access_count = %d, quer 1 após a leitura", n)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// ---------------------------------------------------------------------------
// memory_recent
// ---------------------------------------------------------------------------

func TestMemoryRecent_OrdenaPorUpdatedAtDecrescente(t *testing.T) {
	e := newRTEnv(t, rtOpts{})
	sc := e.scope("ws-a", "proj-a")
	e.put(sc, "notas/antiga.md", "Antiga", "a", pageOpts{})
	e.put(sc, "notas/nova.md", "Nova", "b", pageOpts{})
	// created_at maior em "antiga" e updated_at menor: só a ordenação por updated_at põe "nova" primeiro.
	e.exec(`UPDATE pages SET created_at = ?, updated_at = ? WHERE path = ? AND is_latest = 1`,
		e.t0.Add(2*time.Hour).UnixMicro(), e.t0.Add(-2*time.Hour).UnixMicro(), "notas/antiga.md")
	e.exec(`UPDATE pages SET created_at = ?, updated_at = ? WHERE path = ? AND is_latest = 1`,
		e.t0.UnixMicro(), e.t0.Add(time.Hour).UnixMicro(), "notas/nova.md")

	got := e.callJSON("memory_recent", scopeArgsOf("ws-a", "proj-a"))
	p := paths(t, got["pages"])
	if len(p) != 2 || p[0] != "notas/nova.md" || p[1] != "notas/antiga.md" {
		t.Fatalf("ordem = %v, quer [nova antiga] por updated_at", p)
	}
	first := got["pages"].([]any)[0].(map[string]any)
	if _, ok := first["updated_at"].(string); !ok {
		t.Fatalf("updated_at ausente: %v", first)
	}
}

func TestMemoryRecent_LimiteAliasENaoVazio(t *testing.T) {
	e := newRTEnv(t, rtOpts{})
	sc := e.scope("ws-a", "proj-a")
	e.put(sc, "notas/a.md", "A", "a", pageOpts{})
	e.put(sc, "notas/b.md", "B", "b", pageOpts{})
	base := scopeArgsOf("ws-a", "proj-a")

	if n := len(paths(t, e.callJSON("memory_recent", withArgs(base, map[string]any{"n": 1}))["pages"])); n != 1 {
		t.Fatalf("alias n: %d páginas, quer 1", n)
	}
	if n := len(paths(t, e.callJSON("memory_recent", withArgs(base, map[string]any{"limit": 0}))["pages"])); n != 1 {
		t.Fatalf("limit 0 deve ser limitado a 1: %d páginas", n)
	}
	if n := len(paths(t, e.callJSON("memory_recent", withArgs(base, map[string]any{"limit": 500}))["pages"])); n != 2 {
		t.Fatalf("limit 500 deve ser limitado a 100 e trazer as 2: %d", n)
	}
}

func TestMemoryRecent_ParParcialERecusado(t *testing.T) {
	e := newRTEnv(t, rtOpts{})
	e.scope("ws-a", "proj-a")
	if text := e.callErr("memory_recent", map[string]any{"workspace": "ws-a"}); !strings.Contains(text, "entrada inválida") {
		t.Fatalf("workspace sem project deve ser entrada inválida: %s", text)
	}
}

// ---------------------------------------------------------------------------
// memory_status
// ---------------------------------------------------------------------------

func TestMemoryStatus_ExplicitoComContagens(t *testing.T) {
	e := newRTEnv(t, rtOpts{})
	sc := e.scope("ws-a", "proj-a")
	e.put(sc, "notas/a.md", "A", "a", pageOpts{})
	e.put(sc, "notas/b.md", "B", "b", pageOpts{})

	got := e.callJSON("memory_status", scopeArgsOf("ws-a", "proj-a"))
	scope := got["scope"].(map[string]any)
	if scope["resolved_by"] != "explicit" || scope["workspace"] != "ws-a" || scope["project"] != "proj-a" {
		t.Fatalf("scope incorreto: %v", scope)
	}
	counts := got["counts"].(map[string]any)
	if counts["pages"] != float64(2) {
		t.Fatalf("counts.pages = %v, quer 2", counts["pages"])
	}
	if got["server"] != ServerName {
		t.Fatalf("server = %v", got["server"])
	}
	if _, has := got["warnings"]; has {
		t.Fatal("escopo explícito não deve ter avisos")
	}
}

func TestMemoryStatus_SemArgumentosUsaPadraoScratch(t *testing.T) {
	e := newRTEnv(t, rtOpts{})
	e.scope("default", "scratch")
	got := e.callJSON("memory_status", map[string]any{})
	scope := got["scope"].(map[string]any)
	if scope["resolved_by"] != "default" || scope["project"] != "scratch" {
		t.Fatalf("sem ponteiro, o padrão scratch deve valer: %v", scope)
	}
}

func TestMemoryStatus_SementeDeStartupGeraAviso(t *testing.T) {
	e := newRTEnv(t, rtOpts{})
	sc := e.scope("ws-a", "proj-a")
	e.ptr.SeedStartup(sc)

	got := e.callJSON("memory_status", map[string]any{})
	scope := got["scope"].(map[string]any)
	if scope["resolved_by"] != "startup_seed" {
		t.Fatalf("resolved_by = %v, quer startup_seed", scope["resolved_by"])
	}
	warnings, _ := got["warnings"].([]any)
	if len(warnings) != 1 {
		t.Fatalf("semente de startup deve gerar aviso: %v", got["warnings"])
	}
}

func TestMemoryStatus_ParParcialERecusado(t *testing.T) {
	e := newRTEnv(t, rtOpts{})
	if text := e.callErr("memory_status", map[string]any{"project": "proj-a"}); !strings.Contains(text, "entrada inválida") && !strings.Contains(text, "não encontrado") {
		t.Fatalf("project sem workspace deve falhar de forma fechada: %s", text)
	}
}

// ---------------------------------------------------------------------------
// Erros: forma e vazamento
// ---------------------------------------------------------------------------

func TestErrosDeFerramentaNaoVazamCaminhoNemSQL(t *testing.T) {
	e := newRTEnv(t, rtOpts{})
	calls := []struct {
		name string
		args map[string]any
	}{
		{"memory_query", map[string]any{"query": "x", "workspace": "nao", "project": "existe"}},
		{"memory_recent", map[string]any{"workspace": "nao", "project": "existe"}},
		{"memory_read_page", map[string]any{"path": "../x.md", "workspace": "nao", "project": "existe"}},
		{"memory_status", map[string]any{"workspace": "nao", "project": "existe"}},
	}
	for _, c := range calls {
		text, isErr := e.call(c.name, c.args)
		if !isErr {
			t.Fatalf("%s deveria falhar", c.name)
		}
		for _, leak := range []string{e.dir, "SELECT", "sqlite", "sql:"} {
			if strings.Contains(text, leak) {
				t.Fatalf("%s vaza %q: %s", c.name, leak, text)
			}
		}
	}
}

// toStrings converte um valor JSON de lista de texto.
func toStrings(v any) []string {
	items, _ := v.([]any)
	out := make([]string, 0, len(items))
	for _, it := range items {
		out = append(out, fmt.Sprint(it))
	}
	return out
}
