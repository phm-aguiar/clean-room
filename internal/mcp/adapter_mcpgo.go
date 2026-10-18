package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"runtime/debug"
	"sync"

	mcpgo "github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// serverImplVersion é a versão anunciada em serverInfo.version.
const serverImplVersion = "dev"

// methodServerDiscover é o RPC do protocolo moderno (2026-07-28), recusado pelo pino.
const methodServerDiscover = "server/discover"

// Códigos de erro JSON-RPC usados pelo pino.
const (
	codeMethodNotFound = -32601
	codeInvalidParams  = -32602
	codeInternalError  = -32603
)

// mcpgoServer implementa ToolServer sobre mark3labs/mcp-go v1.2.1.
//
// A biblioteca negocia o protocolo e ecoa a versão do cliente; este adaptador fixa
// 2024-11-05 (ver ADR adr-protocolo-mcp.md): recusa o protocolo moderno e reescreve
// protocolVersion na resposta do initialize.
type mcpgoServer struct {
	srv   *server.MCPServer
	mu    sync.Mutex
	names map[string]struct{}
}

// NewToolServer cria o servidor MCP com nome ai-memory e capacidade tools.
func NewToolServer() ToolServer {
	return &mcpgoServer{
		srv: server.NewMCPServer(
			ServerName,
			serverImplVersion,
			server.WithToolCapabilities(false),
		),
		names: make(map[string]struct{}),
	}
}

// AddTool registra a ferramenta na biblioteca, envolvendo o handler para que pânico
// vire erro de ferramenta.
func (a *mcpgoServer) AddTool(def ToolDef, handler ToolHandler) error {
	if def.Name == "" {
		return errors.New("mcp: nome de ferramenta vazio")
	}
	if handler == nil {
		return fmt.Errorf("mcp: handler nulo para a ferramenta %q", def.Name)
	}
	schema := def.InputSchema
	if len(schema) == 0 {
		schema = json.RawMessage(`{"type":"object"}`)
	}
	if !json.Valid(schema) {
		return fmt.Errorf("mcp: schema inválido para a ferramenta %q", def.Name)
	}

	a.mu.Lock()
	if _, dup := a.names[def.Name]; dup {
		a.mu.Unlock()
		return fmt.Errorf("mcp: ferramenta %q já registrada", def.Name)
	}
	a.names[def.Name] = struct{}{}
	a.mu.Unlock()

	tool := mcpgo.NewToolWithRawSchema(def.Name, def.Description, schema)
	a.srv.AddTool(tool, toolFunc(def.Name, handler))
	return nil
}

// toolFunc adapta um ToolHandler ao formato da biblioteca. Erro do handler e pânico
// viram resultado com IsError, para que o protocolo continue íntegro.
func toolFunc(name string, handler ToolHandler) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcpgo.CallToolRequest) (res *mcpgo.CallToolResult, err error) {
		defer func() {
			if r := recover(); r != nil {
				slog.Error("pânico em handler de ferramenta MCP", "tool", name, "panic", r, "stack", string(debug.Stack()))
				res = mcpgo.NewToolResultError(fmt.Sprintf("ferramenta %s falhou internamente", name))
				err = nil
			}
		}()
		args, argErr := rawArguments(req)
		if argErr != nil {
			return mcpgo.NewToolResultError(argErr.Error()), nil
		}
		out, herr := handler(ctx, args)
		if herr != nil {
			return mcpgo.NewToolResultError(herr.Error()), nil
		}
		if out.IsError {
			return mcpgo.NewToolResultError(out.Text), nil
		}
		return mcpgo.NewToolResultText(out.Text), nil
	}
}

// rawArguments devolve os argumentos como JSON. Sem argumentos, vale {}.
func rawArguments(req mcpgo.CallToolRequest) (json.RawMessage, error) {
	switch v := req.GetRawArguments().(type) {
	case json.RawMessage:
		if len(v) > 0 {
			return v, nil
		}
	case nil:
	default:
		b, err := json.Marshal(v)
		if err != nil {
			return nil, fmt.Errorf("argumentos inválidos: %w", err)
		}
		return b, nil
	}
	return json.RawMessage(`{}`), nil
}

// Handle processa uma mensagem. Primeiro aplica o pino de protocolo; depois delega à
// biblioteca e, para initialize, reescreve protocolVersion na resposta.
func (a *mcpgoServer) Handle(ctx context.Context, message []byte) []byte {
	var head struct {
		Method string          `json:"method"`
		ID     json.RawMessage `json:"id"`
		Params json.RawMessage `json:"params"`
	}
	if err := json.Unmarshal(message, &head); err != nil {
		// Mensagem malformada: a biblioteca responde com erro de parse.
		return a.delegate(ctx, message, "")
	}
	notification := len(head.ID) == 0 || string(head.ID) == "null"

	if v := pinViolation(head.Method, head.Params); v != nil {
		if notification {
			return nil
		}
		return errorResponse(head.ID, v.code, v.message)
	}
	return a.delegate(ctx, message, head.Method)
}

// violation descreve uma recusa do pino de protocolo.
type violation struct {
	code    int
	message string
}

// pinViolation recusa server/discover e qualquer requisição que peça o protocolo moderno,
// seja por _meta.protocolVersion, seja por params.protocolVersion no initialize.
func pinViolation(method string, params json.RawMessage) *violation {
	if method == methodServerDiscover {
		return &violation{
			code:    codeMethodNotFound,
			message: "server/discover não é suportado: o servidor fixa o protocolo " + ProtocolVersion,
		}
	}
	var p struct {
		ProtocolVersion string `json:"protocolVersion"`
		Meta            *struct {
			ProtocolVersion string `json:"protocolVersion"`
		} `json:"_meta"`
	}
	if len(params) == 0 || json.Unmarshal(params, &p) != nil {
		return nil
	}
	requested := p.ProtocolVersion
	if method != "initialize" {
		requested = ""
	}
	if p.Meta != nil && p.Meta.ProtocolVersion != "" {
		requested = p.Meta.ProtocolVersion
	}
	if requested != "" && mcpgo.IsModernProtocol(requested) {
		return &violation{
			code:    codeInvalidParams,
			message: "protocolo " + requested + " não suportado: o servidor fixa " + ProtocolVersion,
		}
	}
	return nil
}

// delegate chama a biblioteca. Em initialize, reescreve a versão da resposta.
func (a *mcpgoServer) delegate(ctx context.Context, message []byte, method string) []byte {
	resp := a.srv.HandleMessage(ctx, message)
	if resp == nil {
		return nil
	}
	out, err := json.Marshal(resp)
	if err != nil {
		return errorResponse(nil, codeInternalError, "falha ao serializar a resposta")
	}
	if method == "initialize" {
		pinned, perr := pinInitializeVersion(out)
		if perr != nil {
			return errorResponse(nil, codeInternalError, "falha ao fixar a versão do protocolo")
		}
		return pinned
	}
	return out
}

// pinInitializeVersion troca result.protocolVersion por ProtocolVersion. Respostas de erro
// (sem result) são devolvidas como estão.
func pinInitializeVersion(out []byte) ([]byte, error) {
	var envelope map[string]json.RawMessage
	if err := json.Unmarshal(out, &envelope); err != nil {
		return nil, err
	}
	rawResult, ok := envelope["result"]
	if !ok {
		return out, nil
	}
	var result map[string]json.RawMessage
	if err := json.Unmarshal(rawResult, &result); err != nil {
		return nil, err
	}
	version, err := json.Marshal(ProtocolVersion)
	if err != nil {
		return nil, err
	}
	result["protocolVersion"] = version
	newResult, err := json.Marshal(result)
	if err != nil {
		return nil, err
	}
	envelope["result"] = newResult
	return json.Marshal(envelope)
}

// errorResponse monta uma resposta de erro JSON-RPC 2.0. Sem id, usa null.
func errorResponse(id json.RawMessage, code int, message string) []byte {
	if len(id) == 0 {
		id = json.RawMessage("null")
	}
	b, err := json.Marshal(struct {
		JSONRPC string          `json:"jsonrpc"`
		ID      json.RawMessage `json:"id"`
		Error   struct {
			Code    int    `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}{
		JSONRPC: "2.0",
		ID:      id,
		Error: struct {
			Code    int    `json:"code"`
			Message string `json:"message"`
		}{Code: code, Message: message},
	})
	if err != nil {
		return []byte(`{"jsonrpc":"2.0","id":null,"error":{"code":-32603,"message":"erro interno"}}`)
	}
	return b
}
