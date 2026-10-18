// Package mcp expõe a memória às ferramentas de agentes pelo protocolo MCP.
//
// A biblioteca de protocolo (mark3labs/mcp-go) fica atrás da interface ToolServer,
// definida aqui. Os handlers de ferramenta conhecem apenas esta interface e os
// tipos deste pacote; a biblioteca é usada só pelo adaptador (adapter_mcpgo.go).
//
// Identidade: a chave do ator (usuário e id de sessão) vem somente do contexto da
// requisição, preenchido pelo middleware de autenticação e pelo transporte. Nenhum
// handler lê cabeçalho cru nem variável associada à tarefa de execução.
package mcp

import (
	"context"
	"encoding/json"
)

// Identidade do servidor anunciada no initialize (docs/mcp-server.md §1).
const (
	// ServerName é o nome anunciado ao cliente.
	ServerName = "ai-memory"
	// ProtocolVersion é o protocolo fixo do servidor. Qualquer versão pedida pelo
	// cliente recebe esta na resposta do initialize (ver ADR adr-protocolo-mcp.md).
	ProtocolVersion = "2024-11-05"
	// CapabilityTools é a única capacidade anunciada.
	CapabilityTools = "tools"
)

// ActorKey é a chave do ator: usuário (chave de identidade de armazenamento) e id de
// sessão (docs/mcp-server.md §2.1). Quando ausentes, valem as strings vazias.
type ActorKey struct {
	User      string
	SessionID string
}

type actorKeyCtxKey struct{}

// WithActorKey devolve um contexto que carrega a chave do ator. Só o middleware de
// autenticação e os transportes devem chamar esta função.
func WithActorKey(ctx context.Context, key ActorKey) context.Context {
	return context.WithValue(ctx, actorKeyCtxKey{}, key)
}

// ActorKeyFrom lê a chave do ator do contexto. O booleano é falso quando o contexto
// não foi preenchido pelo middleware; nesse caso o handler não deve assumir identidade.
func ActorKeyFrom(ctx context.Context) (ActorKey, bool) {
	key, ok := ctx.Value(actorKeyCtxKey{}).(ActorKey)
	return key, ok
}

// ResolveSessionID aplica a ordem de origem do id de sessão do ator (mcp-server.md §2.1):
// contexto de transporte, cabeçalho x-memory-actor-session-id, id de sessão nativo e
// cabeçalho mcp-session-id. Devolve o primeiro valor não vazio, ou "" se nenhum existir.
// O roteador de hooks deve usar a mesma regra.
func ResolveSessionID(transport, actorHeader, native, mcpSessionHeader string) string {
	for _, candidate := range []string{transport, actorHeader, native, mcpSessionHeader} {
		if candidate != "" {
			return candidate
		}
	}
	return ""
}

// ToolDef descreve uma ferramenta para tools/list. InputSchema é um JSON Schema de objeto
// já serializado; vazio vale {"type":"object"}.
type ToolDef struct {
	Name        string
	Description string
	InputSchema json.RawMessage
}

// ToolResult é o resultado de uma ferramenta. Text vai como conteúdo de texto; IsError
// marca erro de execução da ferramenta (o protocolo continua válido).
type ToolResult struct {
	Text    string
	IsError bool
}

// ToolHandler executa uma ferramenta. args são os argumentos JSON da chamada. A identidade
// do ator vem do ctx (ActorKeyFrom), nunca dos argumentos.
type ToolHandler func(ctx context.Context, args json.RawMessage) (ToolResult, error)

// ToolServer é a superfície do servidor MCP usada pelo resto do sistema. Implementações
// devem garantir que um pânico em handler vire erro de ferramenta, sem derrubar o servidor.
type ToolServer interface {
	// AddTool registra uma ferramenta. Erro para nome vazio, handler nulo ou nome repetido.
	AddTool(def ToolDef, handler ToolHandler) error
	// Handle processa uma mensagem JSON-RPC 2.0 e devolve a resposta serializada.
	// Notificações devolvem nil. O ctx chega aos handlers e deve conter a chave do ator.
	Handle(ctx context.Context, message []byte) []byte
}
