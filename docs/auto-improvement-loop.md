# Melhoria automática (auto-improvement)

## Visão rápida

Quando há um provedor de LLM configurado, o servidor revisa sessões concluídas
e propõe pequenas edições na wiki do projeto: um gotcha novo, uma regra
promovida, um ajuste em uma página de conceito desatualizada. Cada proposta
passa por validação (esquema, confiança mínima, limites de tamanho e, se
ativado, um gate de avaliação externo). As propostas válidas são registradas
como pendências, com um arquivo de revisão legível em `_pending/auto-improve/`,
e por padrão são aprovadas automaticamente pelo caminho normal de escrita da
wiki.

O que isso significa na prática:

- **Instalação sem LLM**: nada disso roda. Não há nada para decidir.
- **Com LLM, configuração padrão**: as propostas são pequenas, limitadas e
  auditadas. Cada uma entra no registro de pendências com as citações de
  evidência e a confiança. Dá para reverter pelo histórico da wiki (a confirmar). Para
  ganhar confiança, rode `ai-memory pending-writes list` por uma semana.
- **Servidor compartilhado por uma equipe**: defina `[auto_improve]
  require_approval = true`. Assim, nenhuma edição autônoma é aplicada sem
  revisão humana. As propostas ficam em `_pending/` até alguém aprovar.
- **Custo**: no máximo uma chamada limitada ao LLM por sessão que passa nos
  pré-filtros. O tamanho da entrada e da saída tem teto em `[auto_improve]`.
- **Fonte de verdade do status**: o registro de pendências no banco de dados,
  consultado por `pending-writes`. O arquivo em `_pending/auto-improve/` é um
  retrato do momento em que a proposta foi registrada. Ele não acompanha
  aprovação, rejeição ou conflito.

Um arquivo de revisão traz: o caminho de destino, a operação, a confiança, a
justificativa, as citações de evidência e o corpo ou as edições propostas. É o
suficiente para aprovar ou rejeitar sem abrir outro arquivo.

## Estado atual

| Parte | Estado |
|---|---|
| Revisão de uma sessão concluída (agendada ou manual) | Implementada |
| Registro de propostas, aprovação e rejeição | Implementada |
| Agendador em segundo plano | Implementado. Ativo por padrão quando há provedor de LLM |
| Gate de avaliação externo | Implementado. Desligado por padrão |
| Relatório de manutenção (curador) | Implementado como relatório. Propostas de fusão e substituição não existem |
| Relatório de telemetria das execuções | Implementado |
| Revisão cruzada entre sessões (experiência) | Opcional e desligada por padrão. Ver [experience.md](experience.md) |

## Origem do desenho

Um agente de código aberto com ciclo de autoaperfeiçoamento (Hermes Agent)
serviu de referência. O ai-memory adotou as lições abaixo. Não copiou o
modelo de habilidades em arquivos, porque a unidade durável aqui é a página da
wiki do projeto.

- A revisão roda depois da resposta. Nunca compete com a tarefa ativa e não
  altera o contexto que o agente já está usando.
- Limites de escrita são aplicados por código, não só por instrução no prompt.
- A revisão em segundo plano nunca espera uma confirmação interativa.
- Escrita autônoma passa por um registro de pendências com trilha de
  auditoria. Agendamento e exigência de aprovação são decisões independentes.
- Manutenção (duplicatas, páginas obsoletas) é um ciclo separado, lento e
  somente relatório. Páginas semânticas nunca são apagadas automaticamente.
- Ruído de sessões vazias, testes de fumaça e falhas transitórias não vira
  conhecimento durável.

## Gatilhos e agendamento

### Agendador em segundo plano

O agendador só é iniciado quando as três condições valem ao mesmo tempo:
`[auto_improve.scheduler] enabled` é verdadeiro, `interval_secs` é maior que
zero, `max_sessions_per_tick` é maior que zero, e há um provedor de LLM
configurado. Sem provedor, o servidor registra que o agendador não foi
iniciado e segue com a revisão manual.

- O agendador dorme o intervalo configurado depois de cada ciclo completo.
  Ciclos nunca se sobrepõem. O primeiro ciclo acontece após um intervalo.
- Em cada ciclo, os projetos são percorridos um após o outro. Cada um recebe
  no máximo `max_sessions_per_tick` sessões.
- Uma sessão é candidata quando: tem fim registrado; terminou depois da marca
  d'água do projeto; terminou há pelo menos `min_session_age_secs`; não tem
  execução registrada; e não tem reivindicação em andamento, concluída ou
  estacionada. Uma reivindicação de falha que ainda tem tentativas restantes
  não bloqueia a sessão. As candidatas são tomadas das mais antigas para as
  mais recentes.
- Antes de chamar o LLM, a sessão é reivindicada. A reivindicação impede que
  dois ciclos ou dois processos revisem a mesma sessão.

**Marca d'água.** Na primeira vez que um projeto é visto, a marca d'água recebe
o fim da sessão mais recente existente. Por isso, o histórico nunca é revisado
automaticamente na atualização. Sessões que terminam depois disso entram na
fila.

**Sucesso.** Uma revisão concluída fica registrada. A sessão não volta a ser
candidata do agendador. Assim, a revisão agendada acontece no máximo uma vez
por sessão concluída. Uma revisão manual pode rodar de novo sobre a mesma
sessão.

**Falha.** Quando a revisão falha (por exemplo, erro do provedor), a
reivindicação é liberada e a sessão volta a ser candidata no próximo ciclo. A
contagem de tentativas sobe a cada falha. Com `AUTO_IMPROVE_CLAIM_MAX_ATTEMPTS`
igual a 3 tentativas falhas, a sessão é **estacionada** e deixa de ser
candidata. O último erro fica registrado. Esse registro guarda só a classe e o
status HTTP do erro, nunca o corpo da resposta do provedor.

**Pré-filtro reprovado.** Uma sessão que não passa nos pré-filtros ainda gera
um registro de execução, sem chamada ao LLM. Por isso ela não volta a ser
candidata.

### Revisão manual

- `ai-memory auto-improve --session-id <id>` revisa uma sessão específica. Ela
  não usa reivindicação. Uma falha manual não tem reivindicação para liberar.
- `memory_auto_improve` (ferramenta MCP) revisa uma sessão indicada. Sem
  `session_id`, revisa a sessão concluída mais recente do projeto que ainda não
  tem execução registrada. Chamadas repetidas sem argumento avançam pelas
  sessões restantes, inclusive depois de uma sessão que caiu no pré-filtro.
- A rota administrativa `/admin/auto-improve` aceita o mesmo tipo de revisão.

Ambos os caminhos usam o mesmo pipeline de revisão, de validação e de
registro. A política de aprovação é a mesma.

### Sem gatilho no fim da sessão

A chave `[auto_improve] on_session_end` existe e vale `false` por padrão. Não
há consumidor dela na implementação. Hoje, o único caminho automático é o
agendador.

## Pré-filtros

Antes de qualquer chamada ao LLM, a sessão é checada:

| Condição | Resultado | Motivo registrado |
|---|---|---|
| Menos observações que `min_observations` (padrão 8) | Pula a revisão, sem chamada ao LLM | `too_few_observations` |
| Duração menor que `min_session_duration_secs` (padrão 120 s) | Pula a revisão, sem chamada ao LLM | `session_too_short` |

A duração é o intervalo, em segundos inteiros, entre a primeira e a última
observação da sessão. Quando a sessão pula, a execução é registrada com
provedor e modelo `none` e com o resumo "session skipped by preflight filters".

## O que o revisor recebe

O revisor não vê a wiki inteira. Ele recebe um recorte com teto de tamanho.

1. **Páginas recentes do projeto (para evitar duplicatas).** Até 100 páginas
   mais recentes do resumo do projeto, antes de remover as de `sessions/`. A
   remoção vale só para o revisor. A lista mostra uma linha por página: caminho,
   título, tipo e data de atualização.
2. **Corpo de páginas existentes que podem ser editadas.** Só páginas cujo
   caminho começa com um dos prefixos de `patchable_page_prefixes` (padrão
   `_rules/` e `procedures/`). São as primeiras `max_patchable_pages` (padrão
   8) dessa lista. Cada corpo é limitado a `max_patchable_body_chars` (padrão
   8000) caracteres. Páginas de outras pastas aparecem só pelo título, sem
   corpo. Por isso o revisor pode não perceber que uma proposta duplica uma
   página de `decisions/` ou `gotchas/`. A duplicata é tratada depois, na
   validação (ver abaixo).
3. **Página consolidada da sessão** (`sessions/<id>.md`), quando existe. É a
   fonte principal. Ocupa até um terço do orçamento de entrada, com teto de
   32000 caracteres.
4. **Observações selecionadas.** Quando as observações não cabem no
   orçamento, a seleção é determinística. Sempre entram a primeira e a última.
   Depois, entram amostras uniformemente espaçadas e as observações de maior
   pontuação. A pontuação considera importância, posição no tempo, tipo e
   palavras-chave de causa raiz, correção, erro, decisão e regra. Cada corpo é
   limitado a 1500 caracteres. O número de observações amostradas é no máximo
   48 quando há página de sessão e 72 quando não há. Quando há amostragem, o
   revisor recebe um aviso (`input_budget_sampled`).
5. **Resumo do buffer de rejeições** (ver abaixo).

**Orçamento de entrada.** `max_input_tokens` (padrão 24000) limita a entrada.
Os tokens são estimados em quatro caracteres por token. Mil tokens ficam
reservados ao texto fixo do prompt. Os orçamentos das partes são calculados
sobre o que sobra.

**Chamada.** Cada revisão faz uma chamada estruturada ao LLM, com teto de
16000 tokens de saída e temperatura 0,1. Cada chamada recebe um identificador
de operação novo. A chamada nunca reutiliza o identificador da sessão do
agente.

O revisor é somente leitura. Ele não grava páginas nem registros. A gravação
acontece depois, no registro de pendências.

## Formato da proposta

A saída do LLM é um objeto com três campos:

```json
{
  "summary": "resumo curto da revisão",
  "proposals": [
    {
      "operation": "create_or_update",
      "path": "gotchas/exemplo.md",
      "title": "Exemplo de gotcha",
      "kind": "gotcha",
      "confidence": 0.82,
      "rationale": "por que isto é durável",
      "evidence": [
        {"page": "sessions/abc.md", "quote": "citação curta"}
      ],
      "edit_mode": "full_page",
      "body_markdown": "# Exemplo de gotcha\n\n..."
    }
  ],
  "rejected_candidates": [
    {"reason": "smoke test de um único comando", "evidence": "sessions/xyz.md"}
  ]
}
```

Campos de cada proposta:

- **operation**: só `create_or_update`. Variações como `create`, `update`,
  `upsert` e `write` são normalizadas para esse valor. Qualquer outra coisa
  é rejeitada.
- **path**: caminho relativo na wiki. Passa por normalização de nome antes de
  ser validado.
- **kind**: um de `gotcha`, `decision`, `concept`, `procedure`, `rule`,
  `slot`, `fact` ou `note`. Sinônimos comuns são normalizados (por exemplo,
  `gotchas` vira `gotcha`). Se o tipo não combinar com o caminho, a proposta
  é rejeitada.
- **confidence**: de 0,0 a 1,0.
- **rationale**: obrigatório e não vazio.
- **evidence**: obrigatória. Cada item tem `page` e `quote` não vazios. Uma
  citação em texto puro é aceita e recebe `page` = `unspecified`.
- **edit_mode**: `full_page` (padrão) ou `patch`.
- **body_markdown**: corpo da página, sem cabeçalho de metadados. Aceita os
  nomes `body`, `markdown` e `content` como alias.
- **edits** (só em `patch`): lista de edições ancoradas. Ver a seção de
  validação.

## Validação

Cada proposta passa pelas verificações abaixo, nesta ordem. A primeira falha
rejeita a proposta e registra o motivo.

**Comuns a todas as propostas**

1. Operação canônica (`create_or_update`).
2. Confiança maior ou igual a `min_confidence` (padrão 0,75).
3. Justificativa presente.
4. Evidência presente e completa.
5. Caminho normalizado e portável para o sistema de arquivos.
6. Modo de edição reconhecido (`full_page` ou `patch`).

**Página inteira (`full_page`)**

- Corpo não vazio, com no máximo o menor entre `max_final_body_chars`
  (padrão 32000) e o teto absoluto de 32000 caracteres.
- Para `_rules/`, o corpo cabe em `max_rule_page_tokens` (padrão 2000 tokens,
  com 4 caracteres por token). Para `procedures/`, cabe em
  `max_procedure_page_tokens` (padrão 2000).
- Começa com um título de primeiro nível. Se faltar, o título da proposta é
  inserido como título de primeiro nível antes da validação. Isso gera um
  aviso.
- Caminho dentro dos destinos aceitos (ver tabela de destinos).
- Caminho não existe entre as páginas recentes listadas. A exceção é
  `_slots/current-focus.md`.
- Título não repete o de uma página recente listada. A comparação ignora
  diferenças de caixa e espaços repetidos. A exceção é `_slots/current-focus.md`.
- Tipo compatível com o caminho.

**Edição parcial (`patch`)**

- O caminho precisa estar em `_rules/` ou `procedures/`, e a página precisa
  estar entre os corpos enviados ao revisor. Caso contrário, a proposta é
  rejeitada.
- Número de edições: de 1 a `max_edits_per_proposal` (padrão 5).
- Soma dos caracteres alterados por proposta: no máximo
  `max_changed_chars_per_proposal` (padrão 12000). Cada edição tem no máximo
  `max_edit_content_chars` (padrão 4000).
- O corpo original não pode ter títulos repetidos.
- As edições são aplicadas em ordem. Cada uma usa uma âncora: um título
  markdown exato, com os marcadores (por exemplo, `## Processo de release`).
  A comparação ignora caixa e espaços repetidos.
  - `append`: insere conteúdo no fim da seção da âncora.
  - `add_section`: insere uma seção irmã logo após a âncora. O novo título
    precisa ter o mesmo nível da âncora.
  - `replace_section`: substitui a seção inteira. Exige `section_sha256` igual
    ao hash da seção atual, e o primeiro título do conteúdo precisa ser igual
    à âncora.
  - `delete_section` não é suportado.
  - Âncoras de primeiro nível não são suportadas em edições.
- O corpo resultante precisa ter exatamente um título de primeiro nível, sem
  títulos repetidos, dentro dos mesmos limites de tamanho e orçamento de
  página. Depois da validação, a proposta guarda o hash do corpo base. Esse
  hash é usado para detectar conflito na aprovação.

**Limites da execução**

- No máximo `max_proposals_per_run` propostas válidas (padrão 5). As demais
  são rejeitadas com o motivo `max_proposals_exceeded`.
- Soma das edições de patch da execução: no máximo `max_patch_edits_per_run`
  (padrão 8). Uma proposta de patch que estoura esse total é rejeitada. As
  propostas seguintes ainda podem passar.
- Resumo vazio do LLM gera um aviso, sem rejeitar nada.

Os motivos de rejeição ficam em `rejected_candidates`, com o caminho alvo
quando houver. Os principais motivos são:

| Motivo | Quando |
|---|---|
| `unsupported_operation` | Operação diferente de `create_or_update` |
| `confidence_below_threshold` | Confiança abaixo do mínimo |
| `missing_rationale` / `missing_evidence` | Justificativa ou evidência ausente |
| `invalid_path` / `unsupported_path_prefix` | Caminho inválido ou fora dos destinos |
| `unsupported_edit_mode` | Modo de edição desconhecido |
| `empty_body` / `body_missing_h1` / `body_too_large` | Corpo vazio, sem título de primeiro nível ou grande demais |
| `rule_page_budget_exceeded` / `procedure_page_budget_exceeded` | Página acima do orçamento de tokens |
| `duplicate_existing_path` / `duplicate_existing_title` | Duplica uma página recente listada |
| `kind_path_mismatch` | Tipo não combina com o caminho |
| `patch_unsupported_path` / `patch_target_not_in_context` | Patch fora de `_rules/` ou `procedures/`, ou alvo não enviado ao revisor |
| `patch_missing_edits` / `patch_too_many_edits` | Número de edições fora do limite |
| `patch_changed_chars_too_large` / `patch_edit_content_too_large` | Texto alterado acima do limite |
| `patch_run_edit_budget_exceeded` | Orçamento de edições da execução estourado |
| `duplicate_anchor` / `patch_anchor_not_found` / `patch_h1_anchor_unsupported` | Problemas com âncoras |
| `replace_section_hash_mismatch` / `replace_section_anchor_mismatch` / `replace_section_missing_hash` | Hash ou âncora de substituição não confere |
| `add_section_heading_level_mismatch` / `add_section_h1_unsupported` / `add_section_missing_heading` | Problemas em nova seção |
| `unsupported_patch_op` | Operação de edição desconhecida ou `delete_section` |
| `patch_extra_h1` | Edições criariam um segundo título de primeiro nível |
| `max_proposals_exceeded` | Passou do limite de propostas da execução |
| `too_few_observations` / `session_too_short` | Pré-filtros |
| `input_budget_sampled` | Aviso: observações foram amostradas |
| `eval_gate_failed` / `eval_gate_timeout` / `eval_gate_error` | Gate de avaliação |

## Gate de avaliação

O gate de avaliação é um comando externo, opcional e desligado por padrão.
Ele serve para alvos de alto impacto. Por padrão, esses alvos são `_rules/` e
`procedures/`. O contrato completo (formato de entrada e saída, exemplos de
avaliadores) está em [auto-improve-eval-gates.md](auto-improve-eval-gates.md).
Este resumo descreve o comportamento do pipeline.

- Roda depois da validação e antes do registro. Só afeta as propostas cujo
  caminho começa com um alvo configurado. Nas outras, o gate não é executado.
- O comando é executado diretamente, sem shell, com os argumentos separados
  por espaço. Recebe um objeto JSON pela entrada padrão, com caminho, tipo,
  operação, modo de edição, título, confiança, justificativa, corpo antes,
  corpo depois e hash base esperado.
- Deve devolver na saída padrão um objeto JSON com `passed` (booleano,
  obrigatório) e, opcionalmente, `score_before`, `score_after` e `reason`.
- A proposta passa quando `passed` é verdadeiro e, se os dois escores vierem,
  `score_after - score_before` é maior ou igual a `min_delta` (padrão 0,0).
- Reprovam só a proposta alvo: `passed` falso, delta abaixo do mínimo, JSON
  inválido ou ausência de `passed`, saída acima de 64 KiB, código de saída
  diferente de zero ou erro ao iniciar o comando. Tempo esgotado (padrão 120 s)
  também reprova, com motivo próprio.
- Se o gate está ligado sem comando configurado, as propostas alvo são
  rejeitadas com aviso.
- Propostas aprovadas pelo gate recebem uma entrada extra de evidência com a
  página `auto_improve_eval`, com os escores quando disponíveis. Os resultados
  de todas as execuções do gate ficam registrados na execução.
- Se todas as propostas são reprovadas pelo gate, a execução é registrada
  mesmo assim, com zero propostas e com as candidatas rejeitadas. Assim, o
  buffer de rejeições guarda a tentativa.
- O motivo do gate passa por limpeza de dados sensíveis e é limitado a 2048
  caracteres.
- O gate nunca roda no caminho dos hooks. Só roda na revisão.

## Registro, aprovação e estados

Cada execução registra: provedor, modelo, escopo, resumo, avisos, candidatas
rejeitadas, configuração usada, gatilho (agendador, manual ou experiência) e
resultados do gate. Para cada proposta válida, o registro guarda o tipo de
operação (criar ou atualizar), o caminho, o tipo, o título, a confiança, a
justificativa, a evidência, o corpo e, em patch, as edições e o hash base.

Quando uma proposta não pode ser registrada, o registro segue com as demais e
guarda o motivo. Motivos: a página já existe para uma criação, a página não
existe para uma atualização, ou já há uma proposta pendente para o mesmo
caminho. Esse último vale por operador, quando há mais de um. Propostas sem
operador identificado ficam em um grupo compartilhado. Quando uma proposta é
pulada, a resposta da API ou do comando mostra o caminho e o motivo.

Logo depois do registro, o arquivo de revisão em `_pending/auto-improve/<id>.md`
é escrito. Ele não entra no índice de páginas aprovadas. Antes de ser gravado,
passa por limpeza de dados sensíveis.

**Aprovação**

- Com `require_approval` falso (padrão), o agendador e a revisão manual
  aprovam cada proposta registrada pelo caminho normal de escrita da wiki. O
  ator da aprovação é o de aprovação automática, distinto do ator autor
  (`proposal_actor`, padrão `auto_improve`).
- Com `require_approval` verdadeiro, as propostas ficam `pending`. Podem ser
  aprovadas ou rejeitadas com `pending-writes`.
- Agendamento e exigência de aprovação são independentes. Desligar o
  agendador não exige aprovação manual. Exigir aprovação não impede o
  agendamento.
- No momento da aprovação, uma página fixada (pinned) nunca é reescrita. A
  proposta vira `conflict` com o motivo "target page is pinned". Existe uma
  exceção: páginas de estado em `_slots/` que não são invariantes. Slots
  invariantes continuam protegidos.
- Se o alvo mudou desde o registro (outra versão, outro hash ou outro
  identificador de página, ou página criada entretanto), a proposta vira
  `conflict` com o motivo "target changed since proposal was staged". Nenhum
  arquivo é gravado nesse caso.
- Aprovação passa pelo controle de admissão, com a operação de escrita de
  página (ver [admission-webhooks.md](admission-webhooks.md)).
- Uma rejeição manual grava o motivo. Pelo CLI, se o motivo não for dado, usa
  "rejected by reviewer".

**Estados de uma proposta**

| Estado | Significado |
|---|---|
| `pending` | Registrada, aguardando decisão |
| `approved` | Aplicada na wiki |
| `rejected` | Rejeitada por uma pessoa, com motivo |
| `conflict` | Aprovação tentada, mas recusada (página fixada ou alvo mudou) |
| `failed` | Falha ao aplicar depois da aprovação |

## Buffer de rejeições

Para cada par workspace e projeto, há um buffer persistente de rejeições. Ele
guarda:

- rejeições manuais de uma pessoa;
- conflitos e falhas de aprovação, inclusive recusas por página fixada;
- candidatas rejeitadas pelo revisor ou pela validação, quando trazem motivo
  não vazio, inclusive rejeições do gate.

Ao revisar uma nova sessão, o revisor recebe um resumo das rejeições recentes.
O resumo tem no máximo `max_rejection_context` entradas (padrão 50) e só
considera os últimos `rejection_context_days` dias (padrão 180). Ocupa no
máximo um oitavo do orçamento de entrada, com teto de 12000 caracteres. Se
qualquer um dos dois limites for zero, o resumo fica vazio.

## Filtros negativos

Estas regras estão no prompt do revisor. São instruções ao modelo. Não são
verificadas por código, exceto pela validação de tamanho, caminho e evidência.

| Filtro | Por quê |
|---|---|
| Sessões sem atividade | Aumentam o ruído da busca |
| Testes de fumaça com um único comando | Geralmente são evidência operacional, não conhecimento reutilizável |
| Marcadores de release | Podem ficar em notas, mas não viram regra nem gotcha |
| Falhas transitórias de ferramenta, credencial ou ambiente | Viram restrições falsas depois que o problema é resolvido |
| Afirmações negativas amplas sobre uma ferramenta | Viram recusas futuras depois que a ferramenta é corrigida |
| Narrativas de tarefas pontuais | A página de sessão já guarda a cronologia |
| Falhas transitórias já resolvidas | Vale registrar o padrão de correção, não a falha |
| Trechos de roteamento do ai-memory e instruções de agente (`AGENTS.md`, `CLAUDE.md`) | Não são conhecimento do projeto, a menos que a sessão tenha mudado a política do projeto |

## Destinos aceitos

| Destino | Uso | Observação |
|---|---|---|
| `gotchas/<tema>.md` | Armadilhas reproduzíveis, causa raiz, comportamento de ferramenta, abordagens que falharam e têm correção durável | Exige evidência e uma correção ou mitigação |
| `decisions/<tema>.md` | Escolhas que mudaram arquitetura, fluxo, dependências, implantação ou política | Inclui decisão, justificativa e consequências |
| `concepts/<tema>.md` | Conhecimento estável de domínio ou arquitetura | Prefira síntese a cronologia de tarefas |
| `procedures/<tema>.md` | Fluxos reutilizáveis e padrões de vários passos | Use quando o valor está na sequência |
| `_rules/<tema>.md` | Instruções explícitas de sempre ou nunca para agentes futuros | Pode sugerir atualizar `AGENTS.md` ou `CLAUDE.md` |
| `_slots/current-focus.md` | Estado de curto prazo do projeto | É estado, não verdade durável. Sobrescreve, não acumula |
| `notes/<tema>.md` | Fatos úteis que não cabem nos outros destinos | Evite usar notas como depósito |

Regras de tipo e caminho:

- Criação de página inteira só vai para os destinos da tabela. Páginas
  existentes não são recriadas por esse caminho. Atualizar uma página de
  `_rules/` ou `procedures/` exige edição parcial.
- A exceção é `_slots/current-focus.md`, que pode ser sobrescrito com uma
  proposta de página inteira.
- Outros caminhos (inclusive `sessions/` e `_pending/`) não são alvos válidos.
- O tipo `fact` vai para `notes/` ou `concepts/`. O tipo `note` vai para
  `notes/`.

## Manutenção (curador) e telemetria

### Curador

`ai-memory curator` gera um relatório por regras, somente leitura. Por padrão,
é um teste sem gravação. `--stage` cria uma única página de relatório em
`notes/curator-<data>.md`. Aprovar essa página registra só o relatório. Não
executa nenhuma ação recomendada.

Sinais detectados hoje:

- `cold_episodic`: página episódica, não fixada, com pontuação de retenção
  abaixo do limiar de frio. Páginas fixadas nunca aparecem aqui.
- `stale_slot`: slot de `_slots/` sem alteração por mais dias que o limite.
  `_slots/current-focus.md` tem limite próprio. Os demais slots têm outro
  limite.
- `duplicate_title`: páginas cujo título normalizado é igual. Não são
  comparações aproximadas.
- `dangling_cross_project_link`: link entre projetos que aponta para um alvo
  inexistente.

O curador não edita, não apaga, não funde páginas, não reescreve links e não
atualiza slots. Não existe ainda uma proposta de fusão ou de substituição.
Verificações de contradição e de links quebrados dentro do projeto ficam em
outro relatório (ver [mcp-server.md](mcp-server.md)).

Páginas semânticas nunca são apagadas automaticamente. Páginas episódicas
frias são tratadas pela rotina de retenção descrita em
[design-memory-aging.md](design-memory-aging.md).

### Relatório de telemetria

`ai-memory auto-improve-report` mostra as execuções recentes, o resultado das
propostas e as taxas de término. Padrões: janela de 30 dias e 10 linhas nas
tabelas de contagem. O relatório inclui agregados, taxas de término (aprovado,
rejeitado, conflito, falha), achados e pontos cegos. Só leitura, a menos que
`--stage` seja usado. Com `--stage`, cria uma única página de relatório
pendente de aprovação.

## Comandos, ferramentas e rotas

| Superfície | Efeito |
|---|---|
| Agendador em segundo plano | Revisa sessões concluídas novas depois da marca d'água e registra ou aplica as propostas conforme a política de aprovação |
| `ai-memory auto-improve --session-id <id>` | Revisa uma sessão e aplica ou registra as propostas válidas. Aceita sobrescrever limites por execução |
| `ai-memory auto-improve-report` | Relatório de telemetria. Só leitura, exceto com `--stage` |
| `memory_auto_improve` (MCP) | Revisão manual: sessão indicada ou a mais recente sem execução |
| `/admin/auto-improve` e `/admin/auto-improve/report` | Equivalentes administrativos das revisões e do relatório |
| `ai-memory curator` | Relatório de manutenção. Com `--stage`, cria a página de relatório |
| `ai-memory pending-writes list` | Lista propostas, com filtro de estado |
| `ai-memory pending-writes show <id>` | Mostra os detalhes de uma proposta |
| `ai-memory pending-writes diff <id>` | Mostra a diferença em markdown |
| `ai-memory pending-writes approve <id>` | Aplica pelo caminho normal de escrita |
| `ai-memory pending-writes reject <id> [--reason]` | Descarta a proposta, com trilha de auditoria |

A interface web também lista as revisões pendentes.

As instruções de roteamento (snippets de `CLAUDE.md` e `AGENTS.md`) incluem a
ferramenta `memory_auto_improve`. A atualização dessas instruções é idempotente
e acontece com `ai-memory install-instructions`.

## Configuração

Trecho de configuração com os padrões. O formato do arquivo segue a
configuração geral do ai-memory.

```toml
[auto_improve]
require_approval = false        # true deixa as propostas pendentes para revisão manual
on_session_end = false          # sem efeito observado (ver Gatilhos)
min_observations = 8
min_session_duration_secs = 120
min_confidence = 0.75
max_input_tokens = 24000
max_proposals_per_run = 5
max_patchable_pages = 8
patchable_page_prefixes = ["_rules/", "procedures/"]
max_patchable_body_chars = 8000
max_edits_per_proposal = 5
max_edit_content_chars = 4000
max_changed_chars_per_proposal = 12000
max_patch_edits_per_run = 8
max_rejection_context = 50
rejection_context_days = 180
max_final_body_chars = 32000
max_rule_page_tokens = 2000
max_procedure_page_tokens = 2000
include_raw_fallback = false    # informado ao revisor no prompt
proposal_actor = "auto_improve"
pending_path = "_pending/auto-improve"

[auto_improve.scheduler]
enabled = true                  # false desliga só a revisão em segundo plano
interval_secs = 3600            # 0 desliga só a revisão em segundo plano
max_sessions_per_tick = 1       # por projeto; 0 desliga o agendador
min_session_age_secs = 600
experience_every_sessions = 0   # 0 desliga a revisão cruzada (ver experience.md)

[auto_improve.eval]
enabled = false
command = ""                    # executado diretamente, sem shell
timeout_secs = 120
targets = ["_rules", "procedures"]
min_delta = 0.0
```

`[auto_improve.scheduler]` controla se e com que frequência a revisão em
segundo plano acontece. `[auto_improve] require_approval` controla se as
propostas válidas são aplicadas na hora ou ficam pendentes. Uma opção não
implica a outra.

O gate de avaliação é configurado em `[auto_improve.eval]`. O contrato do
comando está em [auto-improve-eval-gates.md](auto-improve-eval-gates.md).

## Limites fixos

Estes valores não aparecem na configuração.

| Limite | Valor |
|---|---|
| Teto de saída do LLM por revisão | 16000 tokens |
| Temperatura da revisão | 0,1 |
| Reserva de tokens para o texto fixo do prompt | 1000 tokens |
| Teto de caracteres de uma proposta (corpo final) | 32000 |
| Teto de caracteres da página de sessão no prompt | 32000 |
| Caracteres por observação no prompt | 1500 |
| Amostras de observações (com e sem página de sessão) | 48 e 72 |
| Teto de caracteres do resumo de rejeições | 12000 |
| Teto de saída do gate de avaliação | 64 KiB |
| Teto de caracteres do motivo do gate | 2048 |
| Tentativas de revisão antes de estacionar a sessão | 3 |

## Invariantes

Toda implementação deve manter estes pontos:

1. Revisões agendadas e manuais registram propostas. Por padrão, aplicam
   automaticamente.
2. Agendamento e aprovação são independentes.
3. Não há gatilho automático no fim da sessão para a auto-melhoria.
4. Nunca se altera o prompt de sessão ativa nem o contexto de handoff já
   anexado.
5. Nada roda dentro da latência de um hook. Hooks são disparar-e-esquecer e
   têm tempo limitado.
6. Nunca se contorna o isolamento entre workspace e projeto. Toda leitura e
   escrita passa pelo escopo correto.
7. Nunca se contorna a autorização. Rotas administrativas e de escrita exigem
   a capacidade adequada.
8. Nenhuma página é escrita diretamente por um handler ou por um trabalho em
   segundo plano. A escrita passa pelo caminho de escrita da wiki.
9. Páginas semânticas nunca são apagadas automaticamente.
10. Páginas fixadas e slots invariantes não são reescritos sem contradição
    direta citada na proposta e aprovação explícita.
11. Toda edição proposta traz evidência de origem.
12. Propostas autônomas são atribuídas a um ator próprio (`auto_improve`), e
    a aprovação tem atribuição separada.
13. Custo, tamanho de entrada, tamanho de saída e número de mutações por
    execução são limitados.
14. Cada execução gera um registro de auditoria legível por máquina. Quando há
    propostas, também há arquivos de revisão legíveis por pessoas.
15. A política de captura é uma fronteira de armazenamento do cliente, não um
    filtro do revisor. O revisor só lê o que foi armazenado. Ver
    [exclusões de captura](marker-file.md#capture-exclusions).

## Compatibilidade com instalações existentes

1. Pastas de wiki de projetos existentes não precisam de migração. Configurações
   antigas podem ter a chave `[auto_improve] mode`. Ela é ignorada. Pode ser
   removida quando for conveniente.
2. O gatilho no fim da sessão continua desligado. O agendador roda fora da
   latência dos hooks e dorme o intervalo após cada ciclo.
3. Na primeira vez que vê um projeto, o agendador grava a marca d'água. Sessões
   históricas não são revisadas na atualização. Falhas de revisão agendada não
   são repetidas para sempre. Para sessões antigas ou falhas, a revisão manual
   é o caminho de recuperação.
4. Os arquivos de registro de pendências usam migrações aditivas e idempotentes.
   Elas preservam todas as páginas da wiki e todas as linhas de sessão e de
   observação.
5. Blocos `CLAUDE.md` e `AGENTS.md` já instalados continuam válidos. Para
   receber as novas instruções, rode `ai-memory install-instructions` ou peça a
   um agente para atualizar o pacote de roteamento. A troca por marcadores é
   idempotente. Os arquivos das habilidades gerenciadas são atualizados a
   partir dos mesmos recursos do binário (a confirmar).
6. Alterações em páginas-alvo passam pelo registro de propostas antes, e a
   atribuição da aprovação fica separada do ator autor.
7. A pasta `_pending/` na raiz do projeto é reservada. Uma pasta com o mesmo
   nome aninhada (por exemplo, `notes/_pending/`) é uma página comum. Esse
   comportamento vale para a varredura da wiki. Para a migração de conformidade
   OKF, a confirmar.

## Decisões resolvidas

1. Propostas pendentes são uma estrutura de primeira classe na wiki, em
   `_pending/`. O estado e a trilha de auditoria ficam no banco de dados.
2. Lições de procedimento têm a família `procedures/`. Não são forçadas para
   `gotchas/` ou `concepts/`.
3. Propostas autônomas têm o ator `auto_improve`. A atribuição da aprovação é
   registrada separadamente.
4. A confiança mínima é configurável. Deve ser calibrada com propostas
   aplicadas em projetos reais antes de qualquer gatilho sem supervisão.

## Conclusão

O modelo do ai-memory não permite que o agente escreva memória sozinho sem
limites. O que ele oferece é um ciclo limitado, observável e revisável. Esse
ciclo transforma trabalho repetido em conhecimento durável, sem deixar a
execução da tarefa ativa. A revisão agendada e o registro de pendências em
`_pending/auto-improve/` formam o caminho correto. A política de aprovação é
separada: a aprovação automática mantém a wiki atualizada, e
`require_approval = true` dá aos administradores uma fila humana sem
desligar o ciclo de aprendizado.
