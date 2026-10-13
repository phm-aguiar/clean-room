# Spec: Mensagens entre projetos (caixa de entrada e fila)

## Metadados
- **ID:** 011
- **Status:** draft (revisada contra os docs em 2026-10-13; nova revisão prevista quando as features anteriores terminarem)
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
- **Regras:** remetente existente com leitura; destinatário existente com escrita (nunca criado); fila de reivindicação única (`pending` → `claimed` ou `cancelled`), corrida com um vencedor e o perdedor recebendo `null`; até 256 pendentes por caixa (409 no HTTP); `pop` sem id pega a mais antiga; `message_id` malformado é erro (400 "must be a full UUID" no HTTP), vazio é omitido; `list` só pendentes, `box` desconhecido é erro, dica de escopo explícito quando o escopo foi inferido e a caixa está vazia; `cancel` só afeta pendentes do próprio projeto e devolve 0 sem erro.
- **Limites de tamanho:** MCP corta corpo em 8000 caracteres com marcador de truncamento e assunto em 200 caracteres; HTTP corta em 8000 e 200 **bytes** sem marcador; armazenamento corta em 16 KiB; sanitização antes de guardar em todos os caminhos.
- **Segurança:** nada entra no contexto sozinho; `security_notice` ao lado do corpo no MCP; proveniência (`from_workspace_id`, `from_project_id`, `from_agent`, `from_owner_user`) fora do corpo; CLI imprime o aviso e o corpo entre delimitadores; atribuição nunca como filtro; auditoria (envio e `pop` no escopo do destinatário; `cancel` no do remetente, só se cancelou algo).
- **Admissão** (`docs/admission-webhooks.md` §2.2 e §2.4): `message_send` decidido no escopo do destinatário; `message_pop` e `message_cancel` decididos antes de rodar; observadores só quando a operação aconteceu.
- **Aviso de caixa de entrada** no `GET /handoff` (só a contagem, independente do briefing) e `pending_message_count` no `memory_briefing` de projeto.
- **Acesso por projeto** (`docs/design-per-project-authz.md`): envio exige escrita no destinatário; listar exige leitura; `pop` exige escrita.

### Fora do escopo (explicitamente)
- Mensagens entre servidores.

## Decisões nossas sobre pontos "a confirmar" do doc
- `from_agent` e `from_session_id` gravados com o agente e a sessão do ator quando conhecidos; `other` só quando desconhecido.
- Assunto continua sem regra de linha única (só higienizado e cortado).
- Admissão de `pop` e `cancel` também nas rotas administrativas, não só no MCP.
- Corpo vazio (depois de aparar) é recusado em todos os caminhos.

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

## Edge Cases
- Dois `pop` simultâneos da mesma mensagem: um vencedor, o outro `null` (multi-sessão).
- Purga do workspace ou projeto remetente ou destinatário remove as mensagens.
- Corpo com segredo: removido antes de guardar.

## Constraints
- **Segurança:** isolamento por coordenada (entrada pelo destinatário, saída pelo remetente; nenhuma consulta junta um terceiro projeto).
- **Tecnologia:** sem dependência nova; tabela `agent_messages` já na V01.

## Critérios de Sucesso
- [ ] Todas as regras de `docs/agent-messaging.md` § "Rules for sending and reading" com teste
- [ ] Corrida de `pop` com um vencedor sob `-race`
- [ ] Aviso de caixa de entrada sem texto de mensagem, testado com corpo hostil
- [ ] `scripts/check.sh` e o lint passam sem apontamentos

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
