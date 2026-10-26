# Auditoria 013: features 008 e 011 e docs restantes

- **Data:** 2026-10-25
- **Tipo:** auditoria "Chinese Wall" (docs do clean room comparados ao código original, que é somente leitura)
- **Escopo:** 008 (cadeia de admissão), 011 (mensagens entre projetos e aviso no `/handoff`), `admission-webhooks.md`, `agent-messaging.md`, `design-backup-agent-assets.md`, `support-matrix.md` (o que não é de harness), `desktop/*`, `design-rules-promotion.md`, `jev-reranker-adapter.md`, `companions.md`, `external-companions-and-tests.md`

## 1. Arquivos corrigidos

| Arquivo | Alteração |
|---|---|
| `docs/admission-webhooks.md` | §4: sem assinatura/autenticação/retentativa; classificação "indisponível" só na escrita de página; como a recusa de decisão chega ao chamador (MCP e HTTP); erros de configuração. §6: cabeçalho de pulo honrado também sem contexto de autenticação. |
| `docs/agent-messaging.md` | Ordem completa do texto de início de sessão; detalhes de renderização do aviso (aspas com escape); higienizador diferente por caminho; tabela de formas de requisição e erro das rotas administrativas; mapeamento de classes de erro MCP e ordem de validação; edge cases de `pop`. |
| `docs/jev-reranker-adapter.md` | §1.2: o reranqueador também atua na busca web por projeto, com o limite de 4 chamadas compartilhado. |
| `docs/design-rules-promotion.md` | §2: "exposição ao usuário" da confiança de crença resolvida (só nos detalhes de explicação); chave de configuração e corte de valor negativo. |
| `docs/design-backup-agent-assets.md` | §2: regra exata de resolução do Claude Desktop no Windows (pacotes MSIX) e o caso ambíguo, que aborta o backup inteiro. |
| `docs/desktop/installer-and-coexistence.md` | §6.5: nome e regras da cópia de segurança de configurações globais. §7.1: trava de instância única existe. §7.6: caso de vários pacotes MSIX; lista completa de clientes de instalação de MCP (o Zed está incluído). |
| `specs/features/008-cadeia-de-admissao/spec.md` | Seção "Correções da auditoria"; item "Tecnologia" reescrito sem citar biblioteca de linguagem. |
| `specs/features/011-mensagens-entre-projetos/spec.md` | Seção "Correções da auditoria"; critério de corrida reescrito sem citar opção de ferramenta de linguagem. |

## 2. Conceito errado e correção

| Onde | O doc dizia (ou deixava implícito) | O original faz |
|---|---|---|
| admission-webhooks §4 | Nada sobre assinatura nem retentativa (o pedido da auditoria supunha HMAC) | Não há assinatura HMAC, nem cabeçalho de autenticação, nem retentativa. Uma tentativa por gancho; o descarte acima de 256 envios em voo é definitivo. |
| admission-webhooks §4 | A classificação "indisponível" parecia valer para toda recusa | Só existe no caminho de escrita de página. Nos caminhos de notificação e decisão, toda falha de um gancho `reject` é recusa. |
| admission-webhooks §6 | Cabeçalho de pulo honrado "só para anônimo e root" | Também em chamadas sem contexto de autenticação (stdio local), tratadas como anônimas. |
| agent-messaging, aviso | "O aviso é o último bloco, depois de handoff, aviso de handoff, contexto gerenciado e brief" | Há um bloco antes de todos, o resumo do perfil entre projetos. Os blocos são unidos por uma quebra de linha. |
| agent-messaging, aviso | Exemplo com `workspace "default"` sem regra de citação | Os nomes citados recebem escape de literal de string; a cópia sem aspas não recebe. |
| agent-messaging, higienização | "Secret-scrubbed" igual nos dois caminhos | No MCP vale o higienizador configurado, com os padrões extras do operador. Na rota administrativa (a CLI) valem só as regras embutidas. |
| agent-messaging, erros | 403 na recusa HTTP sem dizer o motivo | É 403 para qualquer recusa, inclusive por tempo esgotado ou 5xx do decisor. No MCP a caixa cheia é erro *internal*, e a recusa de admissão é *invalid-request*. |
| jev-reranker-adapter §1.2 | "Atua apenas sobre `memory_query`" | Também na busca web por projeto, com o mesmo limite de 4 chamadas simultâneas, compartilhado entre as duas superfícies. |
| design-rules-promotion §2 | Exposição da confiança de crença "a confirmar" | Só nos detalhes de explicação (`confidence`, `evidence_count`, e `belief_factor` quando o peso é positivo). |
| installer-and-coexistence §7.1 | Bloqueio entre dois servidores "a confirmar" | Existe uma trava exclusiva em `.serve.lock`, com o detentor em `.serve.lock.holder`, e a opção `--force`. |
| installer-and-coexistence §6.5 | Local da cópia de configuração global "a confirmar" | A cópia fica em `<caminho>.bak-<segundos Unix>`, ao lado do arquivo. Duas escritas no mesmo segundo sobrescrevem a mesma cópia. |
| installer-and-coexistence §7.6 | Vários pacotes MSIX "a confirmar"; lista de clientes e Zed "a confirmar" | Com vários pacotes, a instalação falha listando os pacotes. A lista completa de 27 clientes está no doc, e o Zed está nela. |

## 3. Principais equívocos

1. **Supor que o webhook tem contrato de segurança.** Ele não tem assinatura nem autenticação. Uma reimplementação que acrescente HMAC muda o fio, e os ganchos de terceiros escritos para o original continuam funcionando só se o cabeçalho novo for opcional.
2. **Tratar o caminho HTTP de mensagens como espelho do MCP.** Os dois diferem no conjunto de regras do higienizador, na contagem (bytes contra caracteres), no marcador de truncamento, na admissão de `pop` e `cancel` e nas classes de erro.
3. **Achar que o reranqueador é exclusivo do MCP.** A web divide o mesmo limite de concorrência. Uma carga de busca na interface pode, portanto, degradar o reranqueamento do `memory_query`.

## 4. Confirmações sem mudança

- **Cadeia de admissão:** as 15 operações, os valores de `X-Memory-Op`, o limite de 16 ganchos, o corte do tempo limite em 1 a 30 000 ms, a resposta de até 1 MiB, os 256 envios em voo com log ERROR ou WARN, a ordem pular, inscrição, bloqueante, e os textos de erro.
- **Mensagens:** a máquina de estados, as duas guardas e o compare-and-set, os 256 pendentes com o texto exato, os limites 8000 e 200, o corte de armazenamento em 16 KiB, a auditoria `send_message`, `pop_message` e `cancel_message`, a cascata na purga, `from_agent` sempre `other`, o texto do `security_notice` e a saída da CLI.
- **`jev-reranker-adapter.md` §2:** é só proposta. Não há adaptador, configuração nem referência no código.
- **`design-rules-promotion.md`:** é só proposta. Não há comandos `rules …`, chave `[rules_promotion]` nem bloco de promoção. Os blocos de base listados existem.
- **`companions.md` e `external-companions-and-tests.md`:** estão fora do núcleo. A cópia do original não tem pasta de companions, só o núcleo e um harness de avaliação separado (`evals`), coerente com os docs.
- **`design-backup-agent-assets.md`:** a tabela de caminhos, os limites de 128 MiB e as mensagens conferem.
- **`desktop/*` e `support-matrix.md`:** quase todos os "a confirmar" restantes tratam de produtos de terceiros (comportamento do app, caminhos internos, versões). O código do original não responde a eles, e eles foram mantidos.

## 5. Status

| Doc / spec | Status |
|---|---|
| `admission-webhooks.md` / spec 008 | **Fiel** (após as correções) |
| `agent-messaging.md` / spec 011 | **Fiel** (após as correções) |
| `jev-reranker-adapter.md` | **Fiel** |
| `design-rules-promotion.md` | **Ainda incompleta** (efeito do decaimento no ranking segue "a confirmar"; a motivação externa não é verificável no código) |
| `design-backup-agent-assets.md` | **Fiel** |
| `companions.md`, `external-companions-and-tests.md` | **Fiel** (fora do núcleo confirmado) |
| `desktop/installer-and-coexistence.md` | **Ainda incompleta** (restam itens de terceiros e de empacotamento) |
| demais `desktop/*`, `support-matrix.md` | **Ainda incompleta** (pendências são de produtos de terceiros, não respondíveis pelo código) |

## 6. Pendências

1. `design-handoff-offer-not-claim.md` §6 e `complementary-contracts.md`: a duplicação "ago ago" no aviso de handoff pendente foi confirmada no original (o rótulo de idade já termina em "ago" e o modelo acrescenta outro). Normalizar ou não é decisão do usuário. Os docs não são deste escopo e não foram mudados.
2. Decidir se a reimplementação iguala o higienizador do caminho HTTP de mensagens ao do MCP (melhor segurança) ou replica a diferença (fidelidade).
3. Decidir se a reimplementação acrescenta assinatura opcional aos webhooks. Hoje não existe, e a documentação diz isso.
4. `design-rules-promotion.md`: o efeito do decaimento no ranking continua "a confirmar". Ele pertence ao doc de recuperação, não a esta proposta.
5. `plan.md` e `tasks.md` das features 008 e 011 precisam ser regenerados para incorporar as correções (não foram tocados).
