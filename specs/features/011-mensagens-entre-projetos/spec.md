# Spec: Mensagens entre projetos (caixa de entrada e fila)

## Metadados
- **ID:** 011
- **Status:** spec auditada contra o original (ver "Correções da auditoria" e os relatórios em `reports/`); sem plano nem tasks; em toda dúvida, reproduzir o original
- **Autor:** phm-aguiar
- **Data:** 2026-10-13
- **Feature Anterior:** 002 (MCP, `/handoff`, resolução de escopo, `memory_briefing`), 007 (acesso por projeto), 008 (cadeia de admissão)
- **Roadmap:** `specs/features/roadmap.md`
- **Aprovado:** false

## Propósito
> Para quem, o quê e por quê.

Um agente num projeto precisa pedir algo a um agente de outro projeto **sem carregar o contexto do outro projeto**. A mensagem vai para a caixa de entrada do projeto destinatário e é reivindicada uma única vez por uma sessão de lá. É o único ponto em que a memória cruza o isolamento por projeto, de forma explícita e limitada: um projeto só vê o que é endereçado a ele (entrada) ou o que ele mandou (saída).

## Escopo

### Dentro do escopo
- **Tools MCP** (`docs/agent-messaging.md`, `docs/mcp-server.md` §3.3): `memory_message_send` (`to_workspace`, `to_project`, `body`; `subject`, `from_workspace`, `from_project`), `memory_message_list` (`box` `inbox`|`outbox`, `limit` 50 em 1..200), `memory_message_pop` (`message_id` opcional; devolve `message` e `security_notice`, ou `message: null`), `memory_message_cancel` (`message_id` ou todas as pendentes; devolve `cancelled`).
- **CLI** `message send|list|pop|cancel` com as opções do doc (corpo posicional ou pela entrada padrão; `--outbox`; `--id`; `--all`; exatamente um de `--id` e `--all`).
- **Rotas administrativas** `POST /admin/messages/send`, `/pop`, `/cancel` e `GET /admin/messages` (`docs/mcp-server.md` §6).
- **Regras:** remetente existente com leitura; destinatário existente com escrita (nunca criado); **no MCP, `send`, `pop` e `cancel` recusam escopo inferido** (`refusing to <ação> <projeto>: it was resolved by <origem>, not named, …`, texto exato no doc) antes de tocar em qualquer coisa; `pop` e `cancel` no MCP exigem escrita no projeto; fila de reivindicação única (`pending` → `claimed` ou `cancelled`), com duas guardas na mesma transação e corrida com um vencedor, o perdedor recebendo `null`; até 256 pendentes por caixa (409 no HTTP, mensagem do doc); `pop` sem id pega a mais antiga; `message_id` aparado, vazio é omitido, malformado é erro (HTTP 400 `message_id must be a full UUID`; MCP `invalid message_id: …`); `list` só pendentes, mais antigas primeiro, `box` desconhecido é erro (texto do doc), e no MCP as chaves `resolved_scope`/`scope_source`/`hint` aparecem sempre que o escopo foi inferido (não só com caixa vazia); `cancel` só afeta pendentes do próprio projeto e devolve 0 sem erro.
- **Limites de tamanho:** MCP corta corpo em 8000 caracteres com marcador de truncamento (`\n[message body truncated; N chars omitted]`) e assunto em 200 caracteres (marcador próprio); HTTP corta em 8000 e 200 **bytes** sem marcador (assunto vazio vira nenhum); armazenamento corta em 16 KiB; higienização antes do corte e antes de guardar em todos os caminhos.
- **Segurança:** nada entra no contexto sozinho; `security_notice` (texto fixo do doc) ao lado do corpo no MCP e no HTTP; proveniência (`from_workspace_id`, `from_project_id`, `from_agent`, `from_owner_user`) fora do corpo; CLI imprime o aviso e o corpo entre delimitadores; atribuição nunca como filtro (no MCP grava a identidade sempre que existe; no HTTP só quando o servidor distingue operadores); auditoria `send_message`/`pop_message` no escopo do destinatário e `cancel_message` no do remetente, só se cancelou algo, sem autor e com detalhe vazio.
- **Admissão** (`docs/admission-webhooks.md` §2.2 e §2.4): `message_send` decidido no escopo do destinatário, no MCP e no HTTP (recusa HTTP: 403 `message send refused by admission: …`); `message_pop` e `message_cancel` decididos antes de rodar **só no MCP**; observadores só depois do commit (envio completo, `pop` que reivindicou, `cancel` que cancelou algo; caixa cheia não notifica).
- **Saída da CLI** exata (`docs/agent-messaging.md` § "CLI output").
- **Aviso de caixa de entrada** no `GET /handoff` (só a contagem, independente do briefing) e `pending_message_count` no `memory_briefing` de projeto.
- **Acesso por projeto** (`docs/design-per-project-authz.md`): envio exige escrita no destinatário; listar exige leitura; `pop` exige escrita.

### Fora do escopo (explicitamente)
- Mensagens entre servidores.

## Comportamento do original em pontos antes "a confirmar" (replicar)
- `from_agent` é sempre `other` e nenhuma sessão é gravada (nem no envio nem na reivindicação).
- Assunto sem regra de linha única (só higienizado e cortado).
- Admissão de `pop` e `cancel` só no MCP; as rotas administrativas (usadas pela CLI) não a executam para essas duas operações.
- Corpo vazio **não** é recusado por nenhum caminho.
- Mensagens não expiram; purga de workspace/projeto remove as mensagens por cascata, purga de sessão só zera a referência.

## Comportamento Esperado

### Cenário Principal (Happy Path)
1. No projeto A, o agente chama `memory_message_send` para `default/project-b` com um corpo autocontido; recebe `message_id`.
2. No projeto B, o início de sessão mostra "📬 ai-memory: 1 cross-project message waiting…".
3. O agente de B chama `memory_message_pop` e recebe a mensagem com `security_notice` e a proveniência fora do corpo.

### Cenários Alternativos
- **Destinatário inexistente:** recusa; nada é guardado e nenhuma caixa é criada.
- **Caixa cheia (256 pendentes):** recusa com o limite (409 no HTTP).
- **`pop` de id que não está pendente nesta caixa:** `message: null`.
- **`cancel` de mensagem já reivindicada ou de outro remetente:** `cancelled: 0`.
- **Gancho de admissão recusa o envio:** 403 no HTTP; nada guardado.
- **Cliente MCP estático sem escopo nomeado tentando enviar, reivindicar ou cancelar:** recusado com a dica de passar `workspace`/`project`.
- **Corpo vazio:** aceito e guardado.

## Edge Cases
- Dois `pop` simultâneos da mesma mensagem: um vencedor, o outro `null` (multi-sessão).
- Purga do workspace ou projeto remetente ou destinatário remove as mensagens.
- Corpo com segredo: removido antes de guardar.

## Constraints
- **Segurança:** isolamento por coordenada (entrada pelo destinatário, saída pelo remetente; nenhuma consulta junta um terceiro projeto).
- **Tecnologia:** sem dependência nova; tabela `agent_messages` já na V01.

## Critérios de Sucesso
- [ ] Todas as regras de `docs/agent-messaging.md` § "Rules for sending and reading" com teste
- [ ] Corrida de `pop` com um vencedor, sob o detector de corrida da plataforma
- [ ] Aviso de caixa de entrada sem texto de mensagem, testado com corpo hostil
- [ ] `go vet ./...`, `go test -race ./...` e `golangci-lint run` passam sem apontamentos

## Abordagem Escolhida
Fila no `storage` com reivindicação atômica (`UPDATE ... WHERE state = 'pending'`, uma linha afetada), tools no `mcp`, rotas e CLI no `cli`, admissão pela cadeia da 008 na `wiki`.

## Dependências
- 002, 007, 008; `docs/agent-messaging.md`, `docs/admission-webhooks.md`, `docs/design-per-project-authz.md`, `docs/mcp-server.md`.

## Checklist de Prontidão
- [x] Propósito claro e sem ambiguidade
- [x] Escopo delimitado (dentro/fora)
- [x] Cenários cobrem happy path e alternativos
- [x] Edge cases identificados com comportamento esperado
- [x] Constraints explícitas
- [x] Critérios de sucesso mensuráveis
- [x] Abordagem escolhida justificada

## Correções da auditoria

Auditoria "Chinese Wall" de 2026-10-25 contra o original (relatório: `reports/013-auditoria-008-011-restantes.md`). O `docs/agent-messaging.md` foi confirmado e completado; acréscimos que valem para esta spec:

- **Ordem do texto de início de sessão:** perfil entre projetos, handoff aceito, aviso de handoff pendente, contexto de execução gerenciada, brief e, por último, o aviso de caixa de entrada, unidos por uma quebra de linha. Falha em qualquer consulta de nome usa a forma sem nomes; falha na contagem omite o aviso.
- **Nomes no aviso:** aparecem uma vez sem aspas (`<ws>/<proj> inbox`) e uma vez entre aspas duplas com escape de literal de string (`"` e `\` ganham barra; controles viram sequência de escape).
- **Higienização diferente por caminho:** o MCP usa o higienizador configurado (com padrões extras do operador); a rota administrativa (CLI) usa só as regras embutidas.
- **Formas de erro:** tabela das rotas administrativas (campos obrigatórios, 400/403/409/500) e mapeamento MCP (*invalid-parameters* para escopo inferido, `box` e id malformado; *invalid-request* para recusa de admissão; *internal* para caixa cheia e falha de armazenamento), mais a ordem de validação das tools.
- **Edge cases:** `message_id` em branco reivindica a mais antiga; empate de criação no mesmo microssegundo não tem ordem definida; recusa de admissão no envio HTTP é 403 mesmo por indisponibilidade do gancho.
- O critério de corrida foi reescrito sem citar opção de ferramenta de linguagem específica.
