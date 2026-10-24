# Spec: LLM na leitura (reranqueamento, `answer` e `memory_explore`)

## Metadados
- **ID:** 019
- **Status:** draft (revisada contra os docs em 2026-10-13; nova revisão prevista quando a 018 terminar)
- **Autor:** phm-aguiar
- **Data:** 2026-10-13
- **Feature Anterior:** 016 (provedor), 017 (fluxo vetorial na mesma consulta), 005 (`explain`), 002 (`memory_query`, `memory_briefing`)
- **Roadmap:** `specs/features/roadmap.md`
- **Aprovado:** false

## Propósito
> Para quem, o quê e por quê.

Com um provedor configurado, a leitura pode melhorar em três pontos opcionais: reordenar os candidatos por relevância julgada pelo modelo, responder a pergunta com citações sobre o que foi recuperado, e resumir em prosa o estado do projeto. Os três degradam sem erro: sem provedor, falha ou estouro de tempo, a resposta é a mesma da busca sem LLM.

## Escopo

### Dentro do escopo
- **Reranqueador** (`docs/llm-client.md` §9, `prompts-schemas-and-sanitizer.md` §8.1, `docs/mcp-server.md` §3.1 passo 4): `AI_MEMORY_RERANKER=llm` (só esse valor; exige provedor, senão a inicialização falha); candidatos `max(limit, min(limit × 3, 30))`, só com 2 ou mais; entrada JSON com consulta (1000 bytes), candidatos numerados a partir de 1 com título (200) e texto (600), reticências no corte; prompt de juiz com a escala 1,0/0,7/0,3/0,0 e a **primeira frase fixa**; temperatura 0, até 4000 tokens; esquema `{scores: [{candidate, relevance}]}` com campos extras rejeitados; mapeamento de volta (índice fora, repetido, relevância inválida descartados) e completude conferida por quem chama; no máximo 4 chamadas simultâneas sem espera; 20 s; qualquer falha mantém a ordem; `rerank_score` no `explain`; consultas `global=true` e os resultados globais suplementares nunca passam pelo reranqueamento; log só com classe e status.
- **`answer` e `reasoning`** (`docs/mcp-server.md` §3.1 passo 9, `docs/architecture-llm.md` § Ferramentas MCP): só com `answer=true`; resposta estruturada com texto e citações, usando **somente** o conteúdo recuperado do projeto e de `_global`; sem provedor, `answer_unavailable` (nunca erro); falha com classe do erro e status (ou `none`); orçamento base 2000 tokens escalado por `reasoning` (`minimal` 1×, `low` 1,5×, `medium` 2×, `high` 3×, `max` 4×); valor desconhecido rejeitado pelo esquema; consulta global e `as_of` respondem `answer_unavailable`.
- **`memory_explore`** (`prompts-schemas-and-sanitizer.md` §8.2, `docs/mcp-server.md` §3.2): digest em prosa sobre o snapshot do briefing (JSON em bloco de código), balde de intervalo calculado da última observação (`none`, `fresh`, `today`, `recent`, `dormant`, `stale`), verbosidade por balde, foco opcional, fidelidade ao snapshot, `##` só em `dormant` e `stale`, nunca a resposta inteira em cerca; temperatura 0,2; orçamento 16 000 escalado por `reasoning`; sem provedor, `prose: null`, motivo e o snapshot em JSON; incremento de acesso em lote nas páginas expostas.

### Fora do escopo (explicitamente)
- Adaptador de reranqueamento por endpoint juiz (`docs/jev-reranker-adapter.md` §2): proposta não implementada no doc.

## Comportamento Esperado

### Cenário Principal (Happy Path)
1. Com reranqueador ligado e provedor falso, `memory_query {"query": "deploy", "limit": 5, "explain": true}` busca 15 candidatos, reordena pela pontuação e mostra `rerank_score`.
2. `memory_query` com `answer: true` devolve os resultados e a resposta com citações.
3. `memory_explore` num projeto com última observação de 3 dias atrás devolve de 1 a 2 parágrafos (balde `recent`).

### Cenários Alternativos
- **Resposta do reranqueador sem pontuação para um candidato:** ordem anterior mantida.
- **Quinta chamada simultânea de rerank:** ordem local na hora, sem espera.
- **`answer=true` sem provedor:** resultados com `answer_unavailable`.
- **`memory_explore` sem provedor:** `prose: null`, motivo e o snapshot.

## Edge Cases
- `reasoning` desconhecido: rejeitado pela validação de argumentos.
- Projeto sem atividade: balde `none`, um parágrafo dizendo que não há atividade, mais as regras se houver.
- Rerank que estoura 20 s: resposta com a ordem anterior dentro do prazo.

## Constraints
- **INV-13**; **INV-07** (rerank estruturado; o digest é prosa por definição do doc).
- **Segurança:** todo texto de candidato, consulta e snapshot é dado não confiável no prompt.

## Critérios de Sucesso
- [ ] Todas as regras de rerank do §3.1 passo 4 cobertas por teste com provedor falso
- [ ] Tabela de baldes e de multiplicadores coberta por teste
- [ ] `scripts/check.sh` e o lint passam sem apontamentos

## Abordagem Escolhida
`internal/llm/rerank.go` (reranqueador por LLM sobre o `Provider` da 016) e a orquestração no `mcp` (`memory_query` e `memory_explore`), com semáforo de 4 vagas e `context.WithTimeout` de 20 s.

## Decisões tomadas
- **Prompt do `answer`:** (corrigido na auditoria) o prompt **existe no original** e agora está literal em `prompts-schemas-and-sanitizer.md` §8.3; não é decisão nossa. Os nomes dos campos (`answer: {text, citations}`) foram confirmados.

## Dependências
- 016, 017, 005, 002; `docs/llm-client.md` §9, `prompts-schemas-and-sanitizer.md` §0, §8 e §10; `docs/mcp-server.md` §3.1 e §3.2.

## Checklist de Prontidão
- [x] Propósito claro e sem ambiguidade
- [x] Escopo delimitado (dentro/fora)
- [x] Cenários cobrem happy path e alternativos
- [x] Edge cases identificados com comportamento esperado
- [x] Constraints explícitas
- [x] Critérios de sucesso mensuráveis
- [x] Abordagem escolhida justificada

## Correções da auditoria (2026-10-20, contra o original)

Docs corrigidos: `prompts-schemas-and-sanitizer.md` (§8.1 a §8.3 e §12), `mcp-server.md` §3.1 e §3.2, `architecture-llm.md`, `install.md`, `llm-providers.md` §6.4. Mudanças que afetam a implementação:

- **Reranqueador.** O prompt de sistema é literal (a primeira frase fixa é `You are a retrieval reranker for a software project's memory wiki.`). O campo `text` de cada candidato é o **trecho de FTS** do resultado (com `<mark>` e `…`, sem remoção), não o corpo; corte em bytes UTF-8 com `…` incluído no orçamento (consulta 1000, título 200, texto 600). Completude é **tudo ou nada**: qualquer candidato sem nota válida, nota repetida ou id desconhecido mantém a ordem original inteira. Empate de nota segue a posição original; candidatos além do 30º vão ao fim; `rerank_score` só existe com `explain`. A busca da interface web usa o mesmo estágio e **o mesmo limite de 4 chamadas**. Chamada sem id de operação próprio.
- **`answer`.** Roda por último (depois de reordenar, `pin_first` e união `_global`/perfil), sobre os `hits` finais + `global_scope_hits` (nunca `raw_hits`), com o trecho sem `<mark>`. Temperatura **0,1**; esquema `{answer, citations}`; o servidor **não** confere as citações. Quatro mensagens de `answer_unavailable` literais (global, `as_of`, sem provedor, falha com `class=… status=…`). Orçamento 2000/3000/4000/6000/8000 para `minimal/low/medium/high/max` (`low` = base + base/2).
- **`memory_explore`.** Chamada de texto livre (sem esquema), temperatura 0,2, 16000 × nível (16000/24000/32000/48000/64000). Baldes por horas fracionárias (`<1`, `<24`, `<168`, `<720`, senão `stale`), com a descrição literal enviada ao modelo; resposta `{prose, gap:{hours_since_last, bucket, description}, briefing}`; sem provedor ou falha: `prose: null`, `reason` literal e `briefing`. O incremento de acesso das páginas expostas ocorre antes e independe do LLM. Prompt de sistema e mensagem literais no §8.2.
- Config: `AI_MEMORY_RERANKER` aceita `llm` sem diferença de caixa; outro valor ou ausência de provedor derruba a inicialização.

**Status: Fiel** (após as correções; a única dependência aberta é o comportamento da saída estruturada do cliente de LLM, da 016).
