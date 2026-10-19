# Spec: Cadeia de admissão (webhooks antes da persistência)

## Metadados
- **ID:** 008
- **Status:** draft (revisada contra os docs em 2026-10-13; **auditada contra o comportamento do original e corrigida em 2026-10-19** — `admission-webhooks.md` atualizado; `plan.md` e `tasks.md` precisam ser regenerados)
- **Autor:** phm-aguiar
- **Data:** 2026-10-13
- **Feature Anterior:** 007 (ator identificado e capacidade de pular a cadeia), 002 a 004 (operações admitidas)
- **Roadmap:** `specs/features/roadmap.md`
- **Aprovado:** false

## Propósito
> Para quem, o quê e por quê.

O operador quer estender o comportamento sem mudar o motor: acrescentar campos ao frontmatter (por exemplo `contributors`), espelhar mudanças num sistema externo (git, índice de busca, auditoria) ou recusar operações que violam uma política. A cadeia de admissão chama webhooks HTTP configurados em volta das escritas e das operações de ciclo de vida. Os bloqueantes rodam antes do commit e podem mudar a página ou recusar; os não bloqueantes só observam depois.

## Escopo

### Dentro do escopo
- **Configuração** `[[admission_webhooks]]` (`name`, `url`, `timeout_ms` 2000 com corte em 1..30000, `failure_policy` `ignore`|`reject`, `events`, `blocking` padrão `true`) e `AI_MEMORY_ADMISSION_WEBHOOKS_JSON`, que substitui a lista; mais de 16 ganchos impede a subida; lista vazia não monta a cadeia (`docs/admission-webhooks.md` §7 e §8).
- **Escrita de página** na ordem do §2.1 (corrigida): higienização, `last_modified_by`, nomes resolvidos, bloqueantes em ordem (cada um vê a página deixada pelo anterior), nova higienização do corpo e de todo texto do frontmatter, normalização (tier e pinned forçados, conformidade OKF com `generated.at`), derivação de título e links, **validação final de `expires_at` e `entities`** (erro aborta antes de gravar, inclusive vindo da saída de um gancho), instalação do arquivo com instantâneo para reversão sob trava por caminho, linha no índice (falha restaura o arquivo; falha da restauração dá `INCONSISTENT STATE: …`), embedding (falha só registrada), não bloqueantes com a página final.
- **Erros e classificação** (§4 do doc, textos exatos): `admission webhook <nome> status <código e frase>: <corpo>` e `admission webhook <nome> request failed: <erro>`; "indisponível" = erro de transporte, tempo esgotado, 5xx, 408 ou 429 (usado pela escrita de auto-melhoria, 022).
- **As 15 operações** do original (o doc dizia 14; a 15ª é `delete_event_ledger`): `write_page`, `consolidate`, `delete`, `delete_event_ledger`, `purge_project`, `purge_session`, `purge_workspace`, `move_project`, `move_session`, `handoff_begin`, `handoff_accept`, `handoff_cancel`, `message_send`, `message_pop`, `message_cancel`. Nome desconhecido em `events` faz a configuração falhar na carga.
- **Operações de notificação** (`delete`, `delete_event_ledger`, `purge_project`, `purge_session`, `purge_workspace`, `move_project`, `move_session`) com bloqueantes antes e `partial_failure` nas purgas (§2.2, §2.8, §3.1). `delete_event_ledger` é a passagem de retenção de ledgers da 004: o caminho enviado é o **nome do arquivo** do ledger, os bloqueantes rodam antes da remoção (recusa por `reject` mantém o arquivo e conta `ledgers_undeleted`), os não bloqueantes são disparados depois com frontmatter nulo e corpo vazio.
- **Handoffs: decidir, agir, notificar** (`handoff_begin`, `handoff_accept`, `handoff_cancel`; §2.4): só bloqueantes com `reject` decidem; observadores só depois de a operação acontecer; fim de sessão sem handoff em recusa; aceitação no início de sessão com prazo total de 750 ms, deixando o handoff aberto em recusa ou estouro.
- **Pré-voo de consolidação** (`consolidate`, §2.5) para as escritas que já existem com esse op: relatório de lint (004). A consolidação por LLM chega com a 016.
- **Contrato** de requisição e resposta (§3): cabeçalho `X-Memory-Op`, corpo `page` e `ctx` (`actor` com campos omitidos quando desconhecidos), resposta 200 com `page.frontmatter`/`page.body`, 204, 2xx ilegível ou acima de 1 MiB como "sem mudança", 4xx/5xx pela política.
- **Prevenção de laço** (§6): `X-Memory-Skip-Admission-Chain`, honrado só para root e anônimo.
- **Limites** (§7): 256 envios fire-and-forget em voo por processo; descarte com log ERROR para gancho bloqueante e WARN para não bloqueante.
- Operações que **não** disparam ganchos (§2.6): anexos de ledger, conteúdo de handoff, limpeza de cadeia decaída antiga, renomeações, edições externas, prévias de purga.
- Tombamento de arquivo sumido (§2.7, da 004): só os não bloqueantes de `delete`, depois do tombstone.
- Fusão por cópia (§2.8, da 003): `write_page` por página copiada, pulando o gancho chamado exatamente `contributors`, e `purge_project` terminal.

### Fora do escopo (explicitamente)
- Operações de mensagem (`message_send`, `message_pop`, `message_cancel`): a 011 as liga à cadeia quando criar as mensagens.
- Consolidação por LLM e escrita de auto-melhoria (020 e 022).
- Rota administrativa de escrita de página e aprovação de escrita pendente, enquanto não existirem.

## Comportamento Esperado

### Cenário Principal (Happy Path)
1. O operador configura um gancho `contributors` bloqueante em `write_page` e um `git-mirror` não bloqueante em `write_page`, `delete`, `purge_project` e `move_project`.
2. Uma escrita por `memory_write_page` chega ao `contributors`, que devolve o frontmatter com o autor em `contributors`; o motor higieniza de novo e grava.
3. Depois do commit, o `git-mirror` recebe a página final, sem atrasar a escrita.

### Cenários Alternativos
- **Gancho bloqueante com `reject` falha (não 2xx, inacessível, tempo esgotado):** a operação é abortada e o erro vai para quem chamou.
- **Gancho com `ignore` falha:** aviso no log; a operação segue com a página sem mudança.
- **Resposta 2xx vazia, não JSON ou acima de 1 MiB:** "sem mudança" com aviso, qualquer que seja a política.
- **Gancho tenta reintroduzir um segredo:** a nova higienização o remove.
- **Recusa de `handoff_begin` no fim de sessão:** a sessão termina com página e sem handoff.
- **Aceitação no início de sessão passa de 750 ms:** o handoff fica aberto e o resto do contexto é entregue.
- **Usuário comum manda `X-Memory-Skip-Admission-Chain`:** o cabeçalho é ignorado e os ganchos rodam.

## Edge Cases
- **Aceitação sem handoff pendente:** não é anunciada a nenhum decisor nem observador.
- **Cancelamento recusado por dono:** observadores não são avisados.
- **Purga com falha na remoção de arquivo:** ganchos recebem `partial_failure: true`.
- **Mais de 256 envios em voo:** o envio é descartado e registrado, nunca enfileirado; a operação já foi commitada.
- **Mais de 16 ganchos na configuração:** o servidor não sobe.

## Constraints
- **Performance:** a latência de uma escrita é a soma dos `timeout_ms` dos ganchos bloqueantes inscritos no op; não bloqueantes não somam.
- **Tecnologia:** `net/http` da stdlib.
- **Segurança:** a resposta do gancho passa pela higienização antes de gravar (INV-06); a lista de pulo nunca vai no fio.

## Critérios de Sucesso
- [ ] Os casos de cobertura de `docs/admission-webhooks.md` §10 passam como teste
- [ ] Tabela de combinações `blocking` × `failure_policy` do §8.1 coberta para escrita, notificação e handoff
- [ ] Prazo de 750 ms da aceitação no início de sessão testado com decisores lentos
- [ ] `scripts/check.sh` e o lint passam sem apontamentos

## Abordagem Escolhida
Um componente de cadeia no `wiki` (para a escrita de página, que é dele) chamado pelos pontos de operação, mais funções de "decidir, agir, notificar" para handoffs usadas pelo `ingest` e pelo `mcp` através do `storage`/`wiki`, sem `mcp` importar `ingest`. O cliente HTTP dos ganchos é único, com o limite de 256 envios em voo.

## Dependências
- 007: ator identificado, capacidade de pular a cadeia.
- 002 (escrita de página, handoffs), 003 (purgas e movimentos), 004 (despejo, TTL, tombamento, relatório de lint).
- `docs/admission-webhooks.md`, `docs/wiki-files-layer.md` §5.2, `docs/design-handoff-offer-not-claim.md` §5 e §7, `docs/lifecycle-ops.md`.

## Checklist de Prontidão
- [x] Propósito claro e sem ambiguidade
- [x] Escopo delimitado (dentro/fora)
- [x] Cenários cobrem happy path e alternativos
- [x] Edge cases identificados com comportamento esperado
- [x] Constraints explícitas
- [x] Critérios de sucesso mensuráveis
- [x] Abordagem escolhida justificada
