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
- **Prompt do `answer`:** `prompts-schemas-and-sanitizer.md` não traz o prompt da resposta (só os do reranqueador e do digest). O prompt é **decisão nossa**, com as regras comuns do §0 (dado não confiável, fidelidade, só JSON) e "usar somente o conteúdo recuperado". O nome dos campos da resposta segue `docs/mcp-server.md` (`answer` com texto e `citations`), que o próprio doc marca como "a confirmar".

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
