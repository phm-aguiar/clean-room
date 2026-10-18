package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// newTransportServer cria o servidor com a ferramenta eco_ator, que devolve "usuario|sessao" do
// contexto, ou "sem identidade" quando o contexto não tem ator.
func newTransportServer(t *testing.T) ToolServer {
	t.Helper()
	srv := NewToolServer()
	err := srv.AddTool(ToolDef{Name: "eco_ator", Description: "eco do ator"},
		func(ctx context.Context, _ json.RawMessage) (ToolResult, error) {
			key, ok := ActorKeyFrom(ctx)
			if !ok {
				return ToolResult{Text: "sem identidade"}, nil
			}
			return ToolResult{Text: key.User + "|" + key.SessionID}, nil
		})
	if err != nil {
		t.Fatalf("AddTool: %v", err)
	}
	return srv
}

// decodeLines separa a saída em linhas JSON, validando cada uma.
func decodeLines(t *testing.T, out []byte) []map[string]any {
	t.Helper()
	var resps []map[string]any
	for _, line := range strings.Split(strings.TrimRight(string(out), "\n"), "\n") {
		if line == "" {
			continue
		}
		var m map[string]any
		if err := json.Unmarshal([]byte(line), &m); err != nil {
			t.Fatalf("linha de saída não é JSON: %q: %v", line, err)
		}
		resps = append(resps, m)
	}
	return resps
}

const callEcoAtor = `{"jsonrpc":"2.0","id":%d,"method":"tools/call","params":{"name":"eco_ator","arguments":{}}}`

// Stdio: uma requisição gera exatamente uma linha de resposta, com o mesmo id.
func TestStdio_RequisicaoRespondeUmaLinha(t *testing.T) {
	srv := newTransportServer(t)
	in := strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"tools/list"}` + "\n")
	var out bytes.Buffer

	if err := ServeStdio(context.Background(), srv, in, &out); err != nil {
		t.Fatalf("ServeStdio: %v", err)
	}
	if !bytes.HasSuffix(out.Bytes(), []byte("\n")) {
		t.Fatalf("resposta sem quebra de linha final: %q", out.String())
	}
	resps := decodeLines(t, out.Bytes())
	if len(resps) != 1 {
		t.Fatalf("linhas de saída = %d, quer 1: %q", len(resps), out.String())
	}
	if resps[0]["id"] != float64(1) {
		t.Fatalf("id = %v, quer 1", resps[0]["id"])
	}
	if !strings.Contains(out.String(), `"eco_ator"`) {
		t.Fatalf("tools/list não lista eco_ator: %q", out.String())
	}
}

// Stdio: notificação não gera saída; a requisição final sem quebra de linha também é processada.
func TestStdio_NotificacaoSemSaidaERequisicaoSemQuebraFinal(t *testing.T) {
	srv := newTransportServer(t)
	in := strings.NewReader(
		`{"jsonrpc":"2.0","method":"notifications/initialized"}` + "\n" +
			`{"jsonrpc":"2.0","id":2,"method":"tools/list"}`)
	var out bytes.Buffer

	if err := ServeStdio(context.Background(), srv, in, &out); err != nil {
		t.Fatalf("ServeStdio: %v", err)
	}
	resps := decodeLines(t, out.Bytes())
	if len(resps) != 1 {
		t.Fatalf("linhas de saída = %d, quer 1 (só a requisição): %q", len(resps), out.String())
	}
	if resps[0]["id"] != float64(2) {
		t.Fatalf("id = %v, quer 2", resps[0]["id"])
	}
}

// Stdio: a identidade chega aos handlers pelo contexto passado ao ServeStdio.
func TestStdio_IdentidadeChegaPeloContexto(t *testing.T) {
	srv := newTransportServer(t)
	ctx := WithActorKey(context.Background(), ActorKey{User: "alice", SessionID: "sess-1"})
	in := strings.NewReader(callLine(3))
	var out bytes.Buffer

	if err := ServeStdio(ctx, srv, in, &out); err != nil {
		t.Fatalf("ServeStdio: %v", err)
	}
	resps := decodeLines(t, out.Bytes())
	if len(resps) != 1 {
		t.Fatalf("linhas de saída = %d, quer 1", len(resps))
	}
	if got := toolText(t, resps[0]); got != "alice|sess-1" {
		t.Fatalf("identidade = %q, quer %q", got, "alice|sess-1")
	}
}

// callLine monta a linha JSON-RPC de tools/call para eco_ator, com quebra de linha.
func callLine(id int) string {
	return fmt.Sprintf(callEcoAtor, id) + "\n"
}

// HTTP POST: resposta JSON simples, sem SSE e sem Mcp-Session-Id, mesmo com initialize e com o
// cabeçalho de sessão enviado pelo cliente.
func TestHTTPPost_RespondeJSONSemSessao(t *testing.T) {
	cases := map[string]string{
		"initialize": `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-11-25","capabilities":{},"clientInfo":{"name":"t","version":"0"}}}`,
		"tools/list": `{"jsonrpc":"2.0","id":2,"method":"tools/list"}`,
	}
	for name, body := range cases {
		t.Run(name, func(t *testing.T) {
			h := NewHTTPHandler(newTransportServer(t), nil)
			req := httptest.NewRequest(http.MethodPost, "/mcp", strings.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("Mcp-Session-Id", "cliente-tentou-sessao")
			rec := httptest.NewRecorder()

			h.ServeHTTP(rec, req)

			if rec.Code != http.StatusOK {
				t.Fatalf("status = %d, quer 200; corpo %q", rec.Code, rec.Body.String())
			}
			if ct := rec.Header().Get("Content-Type"); ct != "application/json" {
				t.Fatalf("Content-Type = %q, quer application/json", ct)
			}
			if sid := rec.Header().Get("Mcp-Session-Id"); sid != "" {
				t.Fatalf("Mcp-Session-Id emitido: %q (modo sem estado não emite)", sid)
			}
			resps := decodeLines(t, rec.Body.Bytes())
			if len(resps) != 1 {
				t.Fatalf("corpo com %d objetos, quer 1: %q", len(resps), rec.Body.String())
			}
		})
	}
}

// HTTP POST com initialize: a resposta traz o protocolo fixado (contrato do pino) e nenhum
// cabeçalho de sessão.
func TestHTTPPost_InitializeFixaProtocoloSemSessao(t *testing.T) {
	h := NewHTTPHandler(newTransportServer(t), nil)
	body := `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-11-25","capabilities":{},"clientInfo":{"name":"t","version":"0"}}}`
	req := httptest.NewRequest(http.MethodPost, "/mcp", strings.NewReader(body))
	rec := httptest.NewRecorder()

	h.ServeHTTP(rec, req)

	resps := decodeLines(t, rec.Body.Bytes())
	result, _ := resps[0]["result"].(map[string]any)
	if got := result["protocolVersion"]; got != ProtocolVersion {
		t.Fatalf("protocolVersion = %v, quer %q", got, ProtocolVersion)
	}
	if sid := rec.Header().Get("Mcp-Session-Id"); sid != "" {
		t.Fatalf("Mcp-Session-Id emitido no initialize: %q", sid)
	}
}

// HTTP POST com notificação: 202 sem corpo.
func TestHTTPPost_NotificacaoDevolve202SemCorpo(t *testing.T) {
	h := NewHTTPHandler(newTransportServer(t), nil)
	req := httptest.NewRequest(http.MethodPost, "/mcp",
		strings.NewReader(`{"jsonrpc":"2.0","method":"notifications/initialized"}`))
	rec := httptest.NewRecorder()

	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusAccepted {
		t.Fatalf("status = %d, quer 202", rec.Code)
	}
	if rec.Body.Len() != 0 {
		t.Fatalf("corpo em notificação: %q", rec.Body.String())
	}
}

// HTTP GET: 405 com Allow: POST e sem Mcp-Session-Id. DELETE também é 405.
func TestHTTPGet_Devolve405(t *testing.T) {
	for _, method := range []string{http.MethodGet, http.MethodDelete} {
		t.Run(method, func(t *testing.T) {
			h := NewHTTPHandler(newTransportServer(t), nil)
			req := httptest.NewRequest(method, "/mcp", nil)
			rec := httptest.NewRecorder()

			h.ServeHTTP(rec, req)

			if rec.Code != http.StatusMethodNotAllowed {
				t.Fatalf("status = %d, quer 405", rec.Code)
			}
			if allow := rec.Header().Get("Allow"); allow != http.MethodPost {
				t.Fatalf("Allow = %q, quer POST", allow)
			}
			if sid := rec.Header().Get("Mcp-Session-Id"); sid != "" {
				t.Fatalf("Mcp-Session-Id emitido em %s: %q", method, sid)
			}
		})
	}
}

// Identidade no HTTP: o ContextFunc injetado pelo chamador (camada de autenticação) define o ator
// que o handler lê; sem ContextFunc o handler não assume identidade.
func TestHTTPPost_IdentidadeViaContextFunc(t *testing.T) {
	const marker = "X-Teste-Ator"
	ctxFn := func(ctx context.Context, r *http.Request) context.Context {
		user := r.Header.Get(marker)
		if user == "" {
			return ctx
		}
		return WithActorKey(ctx, ActorKey{User: user, SessionID: "http-sess"})
	}

	t.Run("com_ctxfunc", func(t *testing.T) {
		h := NewHTTPHandler(newTransportServer(t), ctxFn)
		req := httptest.NewRequest(http.MethodPost, "/mcp", strings.NewReader(callLine(4)))
		req.Header.Set(marker, "bob")
		rec := httptest.NewRecorder()

		h.ServeHTTP(rec, req)

		resps := decodeLines(t, rec.Body.Bytes())
		if got := toolText(t, resps[0]); got != "bob|http-sess" {
			t.Fatalf("identidade = %q, quer %q", got, "bob|http-sess")
		}
	})

	t.Run("sem_ctxfunc", func(t *testing.T) {
		h := NewHTTPHandler(newTransportServer(t), nil)
		req := httptest.NewRequest(http.MethodPost, "/mcp", strings.NewReader(callLine(5)))
		req.Header.Set(marker, "bob")
		rec := httptest.NewRecorder()

		h.ServeHTTP(rec, req)

		resps := decodeLines(t, rec.Body.Bytes())
		if got := toolText(t, resps[0]); got != "sem identidade" {
			t.Fatalf("sem ContextFunc a identidade = %q, quer \"sem identidade\"", got)
		}
	})
}

// Cabeçalho MCP-Protocol-Version: 2026-07-28 é recusado com 400 e erro -32602; 2024-11-05 passa;
// ausente passa.
func TestHTTPPost_CabecalhoProtocoloModernoRecusado(t *testing.T) {
	body := `{"jsonrpc":"2.0","id":6,"method":"tools/list"}`
	cases := []struct {
		name   string
		header string
		status int
	}{
		{"moderno_recusado", "2026-07-28", http.StatusBadRequest},
		{"fixado_aceito", ProtocolVersion, http.StatusOK},
		{"ausente_aceito", "", http.StatusOK},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			h := NewHTTPHandler(newTransportServer(t), nil)
			req := httptest.NewRequest(http.MethodPost, "/mcp", strings.NewReader(body))
			if tc.header != "" {
				req.Header.Set(headerProtocolVersion, tc.header)
			}
			rec := httptest.NewRecorder()

			h.ServeHTTP(rec, req)

			if rec.Code != tc.status {
				t.Fatalf("status = %d, quer %d; corpo %q", rec.Code, tc.status, rec.Body.String())
			}
			if tc.status == http.StatusBadRequest {
				resps := decodeLines(t, rec.Body.Bytes())
				errObj, _ := resps[0]["error"].(map[string]any)
				if errObj["code"] != float64(codeInvalidParams) {
					t.Fatalf("código de erro = %v, quer %d", errObj["code"], codeInvalidParams)
				}
			}
		})
	}
}

// Corpo acima de 10 MiB é recusado com 413.
func TestHTTPPost_CorpoAcimaDoLimite413(t *testing.T) {
	h := NewHTTPHandler(newTransportServer(t), nil)
	big := strings.NewReader(strings.Repeat("a", maxHTTPBodyBytes+1))
	req := httptest.NewRequest(http.MethodPost, "/mcp", big)
	rec := httptest.NewRecorder()

	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("status = %d, quer 413", rec.Code)
	}
}
