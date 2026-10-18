package mcp

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

// rpc monta uma requisição JSON-RPC, envia ao servidor e devolve a resposta decodificada.
// Devolve nil quando o servidor não responde (notificação).
func rpc(t *testing.T, srv ToolServer, ctx context.Context, id int, method string, params any) map[string]any {
	t.Helper()
	req := map[string]any{"jsonrpc": "2.0", "id": id, "method": method}
	if params != nil {
		req["params"] = params
	}
	raw, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	out := srv.Handle(ctx, raw)
	if out == nil {
		return nil
	}
	var resp map[string]any
	if err := json.Unmarshal(out, &resp); err != nil {
		t.Fatalf("resposta inválida %q: %v", out, err)
	}
	return resp
}

func initParams(version string) map[string]any {
	return map[string]any{
		"protocolVersion": version,
		"capabilities":    map[string]any{},
		"clientInfo":      map[string]any{"name": "teste", "version": "0"},
	}
}

// Contrato: cliente que pede 2025-11-25 (ou qualquer outra versão legada) recebe 2024-11-05,
// com nome ai-memory e capacidade tools anunciadas.
func TestPinoProtocolo_ClienteRecebe2024_11_05(t *testing.T) {
	cases := []string{"2025-11-25", "2025-06-18", "2025-03-26", "2024-11-05", ""}
	for _, requested := range cases {
		t.Run("pede_"+requested, func(t *testing.T) {
			srv := NewToolServer()
			resp := rpc(t, srv, context.Background(), 1, "initialize", initParams(requested))
			if _, isErr := resp["error"]; isErr {
				t.Fatalf("initialize retornou erro: %v", resp["error"])
			}
			result, ok := resp["result"].(map[string]any)
			if !ok {
				t.Fatalf("result ausente: %v", resp)
			}
			if got := result["protocolVersion"]; got != ProtocolVersion {
				t.Fatalf("protocolVersion = %v, quer %q", got, ProtocolVersion)
			}
			info, _ := result["serverInfo"].(map[string]any)
			if info["name"] != "ai-memory" {
				t.Fatalf("serverInfo.name = %v, quer ai-memory", info["name"])
			}
			caps, _ := result["capabilities"].(map[string]any)
			if _, has := caps[CapabilityTools]; !has {
				t.Fatalf("capacidade tools ausente: %v", caps)
			}
		})
	}
}

// Recusa: server/discover (protocolo moderno) responde erro de método inexistente.
func TestRejeitaServerDiscover(t *testing.T) {
	srv := NewToolServer()
	resp := rpc(t, srv, context.Background(), 7, "server/discover", map[string]any{})
	errObj, _ := resp["error"].(map[string]any)
	if errObj == nil {
		t.Fatalf("server/discover deveria retornar erro, veio %v", resp)
	}
	if code := errObj["code"]; code != float64(codeMethodNotFound) {
		t.Fatalf("code = %v, quer %d", code, codeMethodNotFound)
	}
	if _, has := resp["result"]; has {
		t.Fatalf("server/discover não pode devolver result: %v", resp)
	}
}

// Recusa: requisição que pede o protocolo moderno (2026-07-28), seja por initialize,
// seja por _meta em outro método.
func TestRejeitaProtocoloModerno(t *testing.T) {
	srv := NewToolServer()

	resp := rpc(t, srv, context.Background(), 1, "initialize", initParams("2026-07-28"))
	if errObj, _ := resp["error"].(map[string]any); errObj == nil || errObj["code"] != float64(codeInvalidParams) {
		t.Fatalf("initialize 2026-07-28 deveria falhar com -32602, veio %v", resp)
	}

	meta := map[string]any{"_meta": map[string]any{"protocolVersion": "2026-07-28"}}
	resp = rpc(t, srv, context.Background(), 2, "tools/list", meta)
	if errObj, _ := resp["error"].(map[string]any); errObj == nil || errObj["code"] != float64(codeInvalidParams) {
		t.Fatalf("tools/list com _meta moderno deveria falhar com -32602, veio %v", resp)
	}
	if !strings.Contains(strings.ToLower(resp["error"].(map[string]any)["message"].(string)), ProtocolVersion) {
		t.Fatalf("mensagem de erro deveria citar o protocolo fixo %s: %v", ProtocolVersion, resp)
	}
}

// Identidade: a chave do ator vem só do contexto. Argumentos forjados são ignorados.
func TestIdentidadeSoDoContexto(t *testing.T) {
	srv := NewToolServer()
	err := srv.AddTool(ToolDef{Name: "quem"}, func(ctx context.Context, args json.RawMessage) (ToolResult, error) {
		key, ok := ActorKeyFrom(ctx)
		if !ok {
			return ToolResult{Text: "anonimo"}, nil
		}
		return ToolResult{Text: key.User + "|" + key.SessionID}, nil
	})
	if err != nil {
		t.Fatalf("AddTool: %v", err)
	}
	call := map[string]any{
		"name":      "quem",
		"arguments": map[string]any{"user": "forjado", "session_id": "forjada"},
	}

	withActor := WithActorKey(context.Background(), ActorKey{User: "user:alice", SessionID: "sess-1"})
	resp := rpc(t, srv, withActor, 1, "tools/call", call)
	if got := toolText(t, resp); got != "user:alice|sess-1" {
		t.Fatalf("identidade do contexto: got %q, quer %q", got, "user:alice|sess-1")
	}

	resp = rpc(t, srv, context.Background(), 2, "tools/call", call)
	if got := toolText(t, resp); got != "anonimo" {
		t.Fatalf("sem contexto de ator: got %q, quer anonimo (argumentos não podem definir identidade)", got)
	}
}

// Ordem da chave de sessão: contexto de transporte, cabeçalho, nativo e mcp-session-id.
func TestResolveSessionIDOrdem(t *testing.T) {
	cases := []struct {
		name                                  string
		transport, header, native, mcpSession string
		want                                  string
	}{
		{"transporte vence", "t", "h", "n", "m", "t"},
		{"cabecalho sem transporte", "", "h", "n", "m", "h"},
		{"nativo sem cabecalho", "", "", "n", "m", "n"},
		{"mcp-session-id por ultimo", "", "", "", "m", "m"},
		{"nenhum", "", "", "", "", ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := ResolveSessionID(c.transport, c.header, c.native, c.mcpSession); got != c.want {
				t.Fatalf("got %q, quer %q", got, c.want)
			}
		})
	}
}

// Pânico em handler vira erro de ferramenta; o servidor segue respondendo.
func TestPanicViraErroDeFerramenta(t *testing.T) {
	srv := NewToolServer()
	if err := srv.AddTool(ToolDef{Name: "explode"}, func(context.Context, json.RawMessage) (ToolResult, error) {
		panic("boom")
	}); err != nil {
		t.Fatalf("AddTool: %v", err)
	}
	if err := srv.AddTool(ToolDef{Name: "ok"}, func(context.Context, json.RawMessage) (ToolResult, error) {
		return ToolResult{Text: "vivo"}, nil
	}); err != nil {
		t.Fatalf("AddTool: %v", err)
	}

	resp := rpc(t, srv, context.Background(), 1, "tools/call", map[string]any{"name": "explode"})
	if _, hasErr := resp["error"]; hasErr {
		t.Fatalf("pânico não pode virar erro JSON-RPC: %v", resp)
	}
	result, _ := resp["result"].(map[string]any)
	if result["isError"] != true {
		t.Fatalf("pânico deveria virar isError=true: %v", resp)
	}

	resp = rpc(t, srv, context.Background(), 2, "tools/call", map[string]any{"name": "ok"})
	if got := toolText(t, resp); got != "vivo" {
		t.Fatalf("servidor não sobreviveu ao pânico: got %q", got)
	}
}

// Erro devolvido pelo handler vira isError, sem erro JSON-RPC.
func TestErroDoHandlerViraIsError(t *testing.T) {
	srv := NewToolServer()
	if err := srv.AddTool(ToolDef{Name: "falha"}, func(context.Context, json.RawMessage) (ToolResult, error) {
		return ToolResult{}, context.DeadlineExceeded
	}); err != nil {
		t.Fatalf("AddTool: %v", err)
	}
	resp := rpc(t, srv, context.Background(), 1, "tools/call", map[string]any{"name": "falha"})
	result, _ := resp["result"].(map[string]any)
	if result["isError"] != true {
		t.Fatalf("erro do handler deveria virar isError=true: %v", resp)
	}
}

// Notificação não recebe resposta.
func TestNotificacaoSemResposta(t *testing.T) {
	srv := NewToolServer()
	raw := []byte(`{"jsonrpc":"2.0","method":"notifications/initialized"}`)
	if out := srv.Handle(context.Background(), raw); out != nil {
		t.Fatalf("notificação não deveria responder, veio %s", out)
	}
}

// Registro: nome vazio, handler nulo, nome repetido e schema inválido são recusados.
func TestAddToolValidacao(t *testing.T) {
	srv := NewToolServer()
	noop := func(context.Context, json.RawMessage) (ToolResult, error) { return ToolResult{}, nil }

	if err := srv.AddTool(ToolDef{Name: ""}, noop); err == nil {
		t.Fatal("nome vazio deveria falhar")
	}
	if err := srv.AddTool(ToolDef{Name: "nulo"}, nil); err == nil {
		t.Fatal("handler nulo deveria falhar")
	}
	if err := srv.AddTool(ToolDef{Name: "schema", InputSchema: json.RawMessage(`{nao-json`)}, noop); err == nil {
		t.Fatal("schema inválido deveria falhar")
	}
	if err := srv.AddTool(ToolDef{Name: "dup"}, noop); err != nil {
		t.Fatalf("primeiro registro falhou: %v", err)
	}
	if err := srv.AddTool(ToolDef{Name: "dup"}, noop); err == nil {
		t.Fatal("nome repetido deveria falhar")
	}
}

// toolText extrai o primeiro texto de um resultado de tools/call.
func toolText(t *testing.T, resp map[string]any) string {
	t.Helper()
	result, ok := resp["result"].(map[string]any)
	if !ok {
		t.Fatalf("sem result: %v", resp)
	}
	content, _ := result["content"].([]any)
	if len(content) == 0 {
		t.Fatalf("sem content: %v", resp)
	}
	item, _ := content[0].(map[string]any)
	text, _ := item["text"].(string)
	return text
}
