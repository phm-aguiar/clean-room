package mcp

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
)

// Transportes do servidor MCP (docs/mcp-server.md §1, docs/complementary-contracts.md §2).
//
// Os dois transportes só movimentam bytes: cada mensagem JSON-RPC vai para ToolServer.Handle,
// que aplica o pino de protocolo (ver adr-protocolo-mcp.md). Nenhum deles importa a biblioteca
// mark3labs/mcp-go; o ServeStdio e o NewStreamableHTTPServer da biblioteca não são usados porque
// contornariam o pino.

const (
	// maxHTTPBodyBytes limita o corpo de POST /mcp (10 MiB, como no T039).
	maxHTTPBodyBytes = 10 << 20

	// headerProtocolVersion é o cabeçalho que o cliente envia após o initialize.
	headerProtocolVersion = "MCP-Protocol-Version"

	// modernProtocolVersion é o protocolo 2026-07-28, recusado pelo pino. A checagem do
	// cabeçalho fica aqui e não no adaptador porque transport.go não importa a biblioteca;
	// o valor é o mesmo que mcp.IsModernProtocol considera moderno para 2026-07-28.
	modernProtocolVersion = "2026-07-28"
)

// ServeStdio lê mensagens JSON-RPC uma por linha de in e escreve cada resposta em out, seguida
// de quebra de linha. Notificações não geram saída. Nada além do protocolo é escrito em out; logs
// vão para stderr via slog. Retorna nil quando in chega ao fim (EOF) ou erro de leitura/escrita.
//
// A identidade do ator vem de ctx (WithActorKey): em stdio o processo tem um único ator, então o
// chamador a monta uma vez. As mensagens são processadas em sequência, na ordem em que chegam.
func ServeStdio(ctx context.Context, srv ToolServer, in io.Reader, out io.Writer) error {
	reader := bufio.NewReader(in)
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		line, readErr := reader.ReadBytes('\n')
		if msg := bytes.TrimSpace(line); len(msg) > 0 {
			if resp := srv.Handle(ctx, msg); resp != nil {
				if _, err := out.Write(append(resp, '\n')); err != nil {
					return err
				}
			}
		}
		if readErr != nil {
			if errors.Is(readErr, io.EOF) {
				return nil
			}
			return readErr
		}
	}
}

// ContextFunc preenche o contexto de uma requisição HTTP antes do tratamento. A camada de
// autenticação (T039) a injeta: ela lê o ator autenticado e chama WithActorKey. O transporte
// não lê cabeçalho de autenticação.
type ContextFunc func(ctx context.Context, r *http.Request) context.Context

// NewHTTPHandler devolve o handler de POST /mcp no modo HTTP streamable sem estado com resposta
// JSON. O chamador monta o handler na rota /mcp. Comportamento:
//
//   - POST: cada corpo é uma mensagem JSON-RPC. Resposta 200 com Content-Type application/json
//     (nunca SSE). Notificação devolve 202 sem corpo.
//   - Não emite Mcp-Session-Id e ignora o que o cliente enviar nesse cabeçalho.
//   - GET, DELETE e outros métodos: 405 com Allow: POST (WithDisableStreaming, sem sessão a encerrar).
//   - MCP-Protocol-Version: se o cliente pedir 2026-07-28 (protocolo moderno), 400 com erro JSON-RPC
//     -32602. Qualquer outro valor é aceito; a resposta do initialize já fixa 2024-11-05.
//   - Corpo acima de 10 MiB: 413.
//
// ctxFn pode ser nil; sem ele, o contexto é o da própria requisição e os handlers não têm ator.
func NewHTTPHandler(srv ToolServer, ctxFn ContextFunc) http.Handler {
	return &httpHandler{srv: srv, ctxFn: ctxFn}
}

type httpHandler struct {
	srv   ToolServer
	ctxFn ContextFunc
}

func (h *httpHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		http.Error(w, "método não permitido", http.StatusMethodNotAllowed)
		return
	}
	if v := r.Header.Get(headerProtocolVersion); v == modernProtocolVersion {
		writeRaw(w, http.StatusBadRequest, errorResponse(nil, codeInvalidParams,
			"protocolo "+v+" não suportado: o servidor fixa "+ProtocolVersion))
		return
	}

	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxHTTPBodyBytes))
	if err != nil {
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			http.Error(w, "corpo acima do limite", http.StatusRequestEntityTooLarge)
			return
		}
		http.Error(w, "falha ao ler o corpo", http.StatusBadRequest)
		return
	}

	ctx := r.Context()
	if h.ctxFn != nil {
		ctx = h.ctxFn(ctx, r)
	}
	resp := h.srv.Handle(ctx, body)
	if resp == nil {
		w.WriteHeader(http.StatusAccepted)
		return
	}
	writeRaw(w, http.StatusOK, resp)
}

// writeRaw envia um corpo JSON já serializado com o status dado.
func writeRaw(w http.ResponseWriter, status int, body []byte) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_, _ = w.Write(body)
}
