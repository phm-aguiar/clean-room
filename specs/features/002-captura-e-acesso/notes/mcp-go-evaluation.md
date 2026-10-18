# Avaliação do `mark3labs/mcp-go` (T004)

Wiki-Keywords: mcp-go, protocolo MCP, streamable HTTP, ADR-05

## Método e limite da verificação
- Toda a verificação foi por **leitura de documentação e código-fonte público** (GitHub raw, tag `v1.2.1`). **Nenhum build foi executado** com `mcp-go`; a compatibilidade com Go 1.26 e `CGO_ENABLED=0` é inferida, não provada. T007 deve rodar o build de prova.
- Ambiente local: `go version go1.26.5 windows/amd64`; `go.mod` do projeto: `go 1.26.0` (comando `go version` e `go.mod`).
- Prioridade das fontes: docs/ > spec > plan. Onde há conflito, vence o doc.

## Requisitos vindos dos docs e da spec
- `docs/mcp-server.md:22` — versão do protocolo MCP **fixa em 2024-11-05**, para evitar negociar para baixo até uma versão em que o cliente descarta ferramentas. Servidor anuncia nome `ai-memory`, capacidade `tools` e instruções.
- `docs/mcp-server.md:24-27` — identidade lida do contexto da requisição; a biblioteca executa cada ferramenta em tarefa separada, que não herda variáveis da tarefa de execução.
- `docs/mcp-server.md:59-60` — id de sessão do ator: contexto de transporte, cabeçalho `x-memory-actor-session-id`, id nativo, `mcp-session-id`.
- `docs/api-reference.md:20`, `docs/complementary-contracts.md:107` — `/mcp` Streamable HTTP, **sem estado por padrão**, sem `Mcp-Session-Id`; com `--http-stateful` há sessão.
- `docs/https-via-proxy.md:254` — modo padrão responde JSON simples (sem streaming); modo stateful usa SSE.
- `specs/features/002-captura-e-acesso/plan.md:49,76,123-125,178` — `/mcp` POST sem estado, resposta JSON; protocolo 2024-11-05; `mcp-go` atrás de interface própria; **risco declarado: se o `mcp-go` não permitir fixar 2024-11-05, a primeira task de MCP registra o desvio em ADR**; build `CGO_ENABLED=0` em windows/amd64 e linux/amd64.
- `.github/memory/tech.md:19` — `mcp-go` listado como planejado; a versão é fixada na task que o importa (T007).

## Versão
- Última release estável: **v1.2.1** (publicada em 09 de outubro; mudança única: correção de validação OAuth do recurso em metadados well-known) — https://github.com/mark3labs/mcp-go/releases. O ano não aparece na página; considerando a data de hoje (2026-10-13), é o mês corrente.
- `go.mod` da v1.2.1 declara `go 1.25.5` (https://raw.githubusercontent.com/mark3labs/mcp-go/v1.2.1/go.mod). Como 1.25.5 <= 1.26.x, o requisito de toolchain é satisfeito pelo Go 1.26. **Verificação: documentação/código, não build.**
- Dependências diretas da v1.2.1 (mesmo go.mod): `google/jsonschema-go v0.4.2`, `google/uuid v1.6.0`, `santhosh-tekuri/jsonschema/v6 v6.0.2`, `spf13/cast v1.7.1`, `yosida95/uritemplate/v3 v3.0.2` (testify e go-internal são de teste). Todas são Go puro conhecidas; nenhuma exige cgo. `google/uuid` já é usada pelo projeto. Dependências novas relevantes ao `tech.md`: jsonschema-go, santhosh-tekuri/jsonschema, cast, uritemplate.

## Ponto 1 — Compila com Go 1.26 e `CGO_ENABLED=0`
- Go mínimo 1.25.5 (acima). Em `server/stdio.go` não há `import "C"` nem build tags; importa só stdlib (`os/signal`, `syscall` com SIGTERM/SIGINT, que existem em Windows e Linux) e o subpacote `mcp`. Fonte: https://raw.githubusercontent.com/mark3labs/mcp-go/v1.2.1/server/stdio.go.
- Não foi lido o restante do repositório; a ausência de cgo nas dependências é inferida do conhecimento delas (Go puro). **Status: provável OK, não provado.** T007 deve rodar `CGO_ENABLED=0 go build ./...` e `go vet` em windows/amd64 e linux/amd64 (plan.md:178).
- Nota: a release v0.58.0 elevou o pré-requisito do CONTRIBUTING de Go 1.23 para 1.25, confirmando que a linha 1.x exige Go recente (releases page).

## Ponto 2 — Fixar o protocolo em 2024-11-05: **NÃO é suportado nativamente (risco confirmado)**
- `mcp/version.go` (v1.2.1): `LATEST_PROTOCOL_VERSION = ProtocolVersion20260728`; `ValidProtocolVersions` = 2026-07-28, 2025-11-25, 2025-06-18, 2025-03-26, 2024-11-05; `LATEST_LEGACY_PROTOCOL_VERSION = ProtocolVersion20251125`.
- `server/server.go`: `handleInitialize` usa `s.protocolVersion(clientVersion)` = `mcp.NegotiateLegacyVersion`. Lógica (mcp/version.go:90-101): cliente vazio vira 2025-03-26; se a versão do cliente é válida e não é "moderna", **ecoa a do cliente**; caso contrário devolve 2025-11-25. Não há `ServerOption` para fixar a versão (nenhuma encontrada em server.go).
- Consequência: um cliente que pede 2025-11-25 ou 2025-06-18 recebe essa versão de volta, contrariando `docs/mcp-server.md:22`. Só quem pede 2024-11-05 recebe 2024-11-05.
- Além disso, o `StreamableHTTPServer` valida o cabeçalho `MCP-Protocol-Version` em requisições não-initialize e rejeita versões não suportadas (server/streamable_http.go; v1.2.0 "rejects unsupported MCP-Protocol-Version headers"). Se o servidor anunciar 2024-11-05 mas o cliente enviar cabeçalho de outra versão válida, a requisição ainda passa (a lista válida é ampla).
- **Mitigação recomendada (decisão de T007/ADR, a validar):** no adaptador, um wrapper `http.Handler` que (a) reescreve `params.protocolVersion` do `initialize` para `2024-11-05` antes de chegar ao `mcp-go` e (b) força `MCP-Protocol-Version: 2024-11-05` na requisição; para stdio, um wrapper de `io.Reader` equivalente ou resposta pós-processada. Alternativa: pós-processar a resposta do initialize. Em ambos os casos o desvio deve ser registrado em ADR conforme plan.md:125. Não verificado por execução se a reescrita é suficiente em todas as rotas.
- Risco adicional: v1.0.0 adicionou o protocolo 2026-07-28 ("modern"), alcançado via `server/discover`; clientes que o usem fogem do handshake `initialize`. Precisa ser bloqueado/rejeitado pelo wrapper para honrar o pino.

## Ponto 3 — HTTP streamable sem estado com resposta JSON: **suportado**
- `NewStreamableHTTPServer(server *MCPServer, opts ...StreamableHTTPOption) *StreamableHTTPServer`; `ServeHTTP` implementa `http.Handler` (roteia POST, GET, DELETE; HEAD devolve 200; outros métodos 404). Fonte: server/streamable_http.go.
- `WithStateLess(true)`: "the server will manage no session information. Every request will be treated as a new session"; equivale a `WithSessionIdManager` com `StatelessSessionIdManager`. Em modo stateless nenhum `Mcp-Session-Id` é devolvido no initialize — casa com `docs/complementary-contracts.md:107`.
- Resposta: JSON por padrão; só vira SSE se a sessão for promovida a SSE (por exemplo, notificação durante o tratamento) e o writer suportar streaming. Como as 12 tools não emitem notificações, o resultado é JSON, mas **o modo stateless sozinho não garante JSON**; os handlers não devem enviar notificações/progresso.
- `WithDisableStreaming()`: GET responde 405 em vez de stream — recomendável no modo padrão (ajusta ao `plan.md:49`, que lista só POST).
- Proteção embutida de DNS-rebinding para localhost e tratamento de CORS aplicados antes do roteamento (a menos que desabilitados) — verificar interação com o proxy reverso descrito em `docs/https-via-proxy.md` e com Bearer.
- Modo `--http-stateful`: usa sessões do `mcp-go` (padrão sem `WithStateLess`) e SSE, coerente com `docs/https-via-proxy.md:254`.

## Ponto 4 — stdio: **suportado**
- `func ServeStdio(server *MCPServer, opts ...StdioOption) error`; `WithStdioContextFunc(fn func(ctx context.Context) context.Context)`. Fonte: server/stdio.go.
- Chamadas `tools/call` vão para fila e um pool de goroutines (padrão 5, máximo 100); se a fila encher, rodam de forma síncrona no loop de leitura. Outras requisições com id rodam em goroutine própria; notificações, de forma síncrona.
- `ServeStdio` instala tratamento de SIGTERM/SIGINT (relevante para o shutdown de `serve`). Em stdio nada pode escrever em stdout além do protocolo (logs via `slog` em stderr).

## Ponto 5 — Acesso ao contexto da requisição: **suportado, com ressalvas**
- HTTP: `WithHTTPContextFunc(func(ctx context.Context, r *http.Request) context.Context)` "customise the context to the server using the incoming request" (server/streamable_http.go). É o ponto para injetar o ator do Bearer no contexto; alternativa mais robusta: o middleware de autenticação Chi já colocar o ator em `r.Context()` e o `ContextFunc` apenas preservar/derivar.
- stdio: `WithStdioContextFunc` aplica o **mesmo contexto a todas as requisições** do processo (server/stdio.go) — o ator em stdio é único por processo; a chave "usuário + id de sessão" (plan.md:76) usará o cabeçalho/argumento ou id nativo, não o contexto de transporte.
- Handler de tool: `func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error)`; o `ctx` vem do despachante, derivado do contexto da conexão/requisição, então valores injetados pelo ContextFunc chegam aos handlers mesmo com execução em goroutines (coerente com `docs/mcp-server.md:24-27`: ler do contexto, nunca de variáveis da tarefa). Tarefas assíncronas recebem filho cancelável do mesmo ctx.
- Não verificado por execução se, no HTTP stateless, o `ctx` do handler é cancelado quando a resposta termina antes do trabalho; confirmar em teste.

## Recomendação para T007
- **Fixar `github.com/mark3labs/mcp-go v1.2.1`** (última estável, `go 1.25.5` <= 1.26). Registrar em `.github/memory/tech.md` a versão e as dependências transitivas novas.
- Prever na interface `ToolServer` do pacote `mcp` (plan.md:123) que o pino de protocolo e a reescrita ficam no adaptador, não nos handlers.
- Opções do adaptador: `server.NewMCPServer("ai-memory", ..., server.WithToolCapabilities(false), server.WithInstructions(...))`, `NewStreamableHTTPServer(..., WithStateLess(true), WithDisableStreaming(), WithHTTPContextFunc(...))` para o modo padrão; sem `WithStateLess` para `--http-stateful`; `ServeStdio` com `WithStdioContextFunc` para stdio.
- Registrar ADR de desvio (plan.md:125) descrevendo o pino por wrapper, pois a lib não o suporta.

## Riscos
1. **Alto:** a biblioteca não fixa 2024-11-05; ecoa a versão do cliente (até 2025-11-25) e conhece o protocolo moderno 2026-07-28. Exige wrapper e ADR; risco de regressão a cada upgrade (a lógica de negociação mudou entre séries).
2. **Médio:** compilação com `CGO_ENABLED=0`/Go 1.26 apenas inferida; provar em T007 nas duas plataformas.
3. **Médio:** resposta JSON no stateless depende de os handlers não emitirem notificações; configurar `WithDisableStreaming`.
4. **Médio:** proteção DNS-rebinding/CORS embutida pode bloquear acessos via proxy ou Host não localhost; validar com `docs/https-via-proxy.md`.
5. **Baixo:** a API é jovem e mudou muito (v0.x para v1.x: 1.0 em 2026-07-28 spec); fixar versão exata e manter atrás da interface própria (constituição, `.github/memory/constitution.md:71`).
6. **Baixo:** acúmulo de dependências transitivas (jsonschema ×2, cast, uritemplate) a registrar em `tech.md`.

## Fontes
- https://github.com/mark3labs/mcp-go/releases
- https://raw.githubusercontent.com/mark3labs/mcp-go/v1.2.1/{go.mod, mcp/version.go, server/server.go, server/streamable_http.go, server/stdio.go}
- docs/mcp-server.md, docs/api-reference.md, docs/complementary-contracts.md, docs/https-via-proxy.md, specs/features/002-captura-e-acesso/{spec.md,plan.md}, .github/memory/{constitution.md,tech.md}
