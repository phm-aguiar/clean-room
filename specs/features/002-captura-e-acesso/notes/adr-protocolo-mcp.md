# ADR: pino do protocolo MCP em 2024-11-05 por wrapper no adaptador

Status: aceito (T033, 2026-10-14)
Wiki-Keywords: mcp-go, protocolo MCP, 2024-11-05, pino, wrapper, server/discover, ADR

## Contexto

- `docs/mcp-server.md` §1 fixa a versão do protocolo MCP em **2024-11-05**. O motivo é evitar negociação para
  baixo até uma versão em que o cliente descarta ferramentas. O servidor anuncia o nome `ai-memory` e a
  capacidade `tools`.
- A biblioteca `github.com/mark3labs/mcp-go` v1.2.1 **não fixa o protocolo**. Em `mcp/version.go`,
  `NegotiateLegacyVersion` ecoa a versão pedida pelo cliente quando ela é válida e não é "moderna". Um cliente
  que pede 2025-11-25 ou 2025-06-18 recebe essa versão de volta. Não existe `ServerOption` para fixar a versão.
- A biblioteca conhece o protocolo moderno 2026-07-28 (`server/discover`, protocolo no `_meta`). Um cliente que
  o usa foge do handshake `initialize`.
- O plan (Q-07) previa o risco e mandava registrar o desvio em ADR antes de seguir. A evaluation (T004) e a
  phase0 (item 3.1) recomendaram o pino por wrapper no adaptador.

## Decisão

O pino fica no adaptador `internal/mcp/adapter_mcpgo.go`. Os handlers e a interface `ToolServer` não conhecem a
biblioteca nem a versão negociada. O adaptador faz três coisas:

1. **Recusa `server/discover`** com erro JSON-RPC `-32601` (método inexistente). Notificação com esse método
   não recebe resposta.
2. **Recusa o protocolo moderno** com erro `-32602`. Vale para `initialize` com `params.protocolVersion` >=
   2026-07-28 e para qualquer método com `params._meta.protocolVersion` >= 2026-07-28. A comparação usa
   `mcp.IsModernProtocol` da própria biblioteca.
3. **Reescreve `result.protocolVersion` para `2024-11-05`** na resposta do `initialize`. Qualquer versão que o
   cliente peça (2025-11-25, 2025-06-18, 2025-03-26, 2024-11-05 ou vazia) recebe 2024-11-05. Respostas de erro
   não são alteradas.

Outras partes da identidade e da robustez, no mesmo adaptador:

- A chave do ator (`ActorKey`: usuário e id de sessão) é lida **só do contexto** (`ActorKeyFrom`), preenchido
  pelo middleware e pelos transportes via `WithActorKey`. Argumentos da chamada nunca definem identidade.
- Pânico em handler vira resultado de erro de tool (`isError: true`), sem erro JSON-RPC e sem derrubar o
  servidor. O pânico é registrado com `slog` (stderr). A mensagem ao cliente não expõe o valor do pânico.
  Isso é diferente do `server.WithRecovery()` da biblioteca, que devolveria erro JSON-RPC.
- Nomes de ferramenta repetidos, nome vazio, handler nulo e schema inválido são recusados em `AddTool`, antes de
  chegar à biblioteca (que entraria em `panic` no caso de nome repetido).

## Alternativa rejeitada

- **Reescrever a requisição `initialize`** para enviar 2024-11-05 à biblioteca (proposta do T004). Rejeitada:
  a biblioteca continuaria negociando a partir de uma versão falsa, sem produzir o contrato pedido. Além disso,
  o `initialize` reescrito deixaria de refletir o que o cliente pediu. A reescrita da resposta é direta e
  testável.
- **Usar o SDK oficial `modelcontextprotocol/go-sdk`**. Rejeitada no plan (Q-07). Não foi reavaliada aqui.
- **Fixar a versão por fork ou `replace` da biblioteca.** Rejeitada: aumenta a manutenção e não é necessária para
  o contrato.

## Consequências

- **Testes de contrato** em `internal/mcp/server_test.go`:
  `TestPinoProtocolo_ClienteRecebe2024_11_05` (cliente que pede 2025-11-25 recebe 2024-11-05),
  `TestRejeitaServerDiscover` e `TestRejeitaProtocoloModerno`.
- **Risco de atualização da biblioteca.** A lógica de negociação mudou entre as séries 0.x, 1.x e 1.2.x. Um
  upgrade pode mudar os campos do `initialize` ou o formato de erro. Antes de subir a versão, rodar os testes de
  pino.
- **Cabeçalho `MCP-Protocol-Version` fica para o T038.** A biblioteca valida esse cabeçalho nas requisições
  que não são `initialize` e aceita a lista inteira de versões. O transporte HTTP (T038) precisa recusar o valor
  moderno 2026-07-28 nesse cabeçalho. Este adaptador só trata o corpo JSON-RPC.
- **Modo HTTP sem estado.** A escolha do modo (`WithStateLess`, `WithDisableStreaming`) e a resposta JSON ficam
  para o T038. O adaptador expõe `Handle(ctx, bytes)` para que o transporte use a mesma regra nos dois
  modos (stdio e HTTP).
- **Instruções de uso** (`docs/mcp-server.md` §1) não foram definidas. Não há texto no doc nem na spec. O
  `initialize` não anuncia instruções nesta task. Pendência para a task que criar as ferramentas de leitura
  (T034) ou o transporte (T038).
- **Versão do servidor** anunciada em `serverInfo.version` é `dev`. Não há fonte de versão definida no
  projeto. Pendência para o pacote de build.
- **Dependências.** `github.com/mark3labs/mcp-go` v1.2.1 entra no `go.mod`. Entram também, como indiretas, as
  transitivas `google/jsonschema-go`, `santhosh-tekuri/jsonschema/v6`, `spf13/cast` e `yosida95/uritemplate/v3`.
  O `go.sum` ganhou as entradas delas. Registro em `.github/memory/tech.md` pendente do coordenador.
