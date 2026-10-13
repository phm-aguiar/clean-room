# Spec: LLM e embeddings (consolidação, auto-melhoria, busca vetorial, rerank, answer, explore, dream)

## Metadados
- **ID:** 016
- **Status:** draft (esqueleto gerado a partir dos docs em 2026-10-13; revisar contra os docs antes de aprovar)
- **Autor:** phm-aguiar
- **Data:** 2026-10-13
- **Feature Anterior:** 004
- **Roadmap:** `specs/features/roadmap.md`
- **Aprovado:** false

## Propósito
Recursos opcionais que usam provedor de LLM ou embeddings. O produto funciona completo sem eles (INV-13); quando ligados, melhoram a consolidação, a busca e a curadoria.

## Escopo

### Dentro do escopo
- Cliente LLM com saídas estruturadas por JSON Schema e provedores de contingência (INV-07, INV-14); configuração `llm_*` (`docs/cli.md` §2).
- Embeddings com `{provider, model, dim}` junto de cada vetor (INV-08); fluxo de vetor e de resumo na busca; backfill de embeddings.
- Deduplicação de clusters frios e banda de contradição, que não chamam LLM mas dependem de embeddings armazenados (`docs/design-memory-aging.md` §7 e §9).
- Reranqueamento (`AI_MEMORY_RERANKER=llm`), `answer` e `reasoning` em `memory_query`, `memory_explore` (`docs/mcp-server.md` §3).
- Consolidação por LLM (`memory_consolidate`, fila `session_consolidation_jobs`, `consolidate_on_session_end`) e checkpoints com consolidação (`docs/lifecycle-event-ingestion.md` §8.2 e §8.3).
- Auto-melhoria (`memory_auto_improve`, propostas e escritas pendentes), promoção para regras (`docs/design-rules-promotion.md`, ainda proposta no doc) e *dream pass* (`docs/design-memory-aging.md` §10).

### Fora do escopo (explicitamente)
- Tornar qualquer caminho padrão dependente de LLM (INV-13).

## Fontes (docs)
- `docs/design-memory-aging.md` §7, §9 e §10, `docs/consolidation.md`, `docs/mcp-server.md` §3, `docs/lifecycle-event-ingestion.md` §8, `docs/design-rules-promotion.md`, `docs/cli.md` §2 e §4.

## Bloqueio de fonte
Vários docs apontam para `../docs-adiados/` (`llm-providers.md`, `local-embeddings.md`, `prompts-schemas-and-sanitizer.md`), que **não existe** neste repositório. Sem esses docs, provedores, prompts e o modelo de embedding local não têm contrato de origem. Esta feature só pode ser especificada por inteiro quando esses docs existirem; até lá, o que não tiver doc vira "decisão nossa" explícita ou fica fora.

## Dependências
- 004 (passes de manutenção que a dedup e o dream estendem), 002 (fim de sessão e MCP).

## Comportamento Esperado
> A preencher na revisão contra os docs listados em "Fontes". Cenários Gherkin (sucesso, falha e borda) saem dos cenários de aceitação dos próprios docs, quando existirem.

## Critérios de Sucesso
- [ ] A preencher na revisão: cada regra numérica e cada cenário de aceitação dos docs com teste correspondente
- [ ] Testes de contrato das invariantes tocadas, com teste multi-sessão quando a constituição exigir
- [ ] `scripts/check.sh` e o lint passam sem apontamentos

## Checklist de Prontidão
- [ ] Propósito claro e sem ambiguidade
- [ ] Escopo delimitado (dentro/fora)
- [ ] Cenários cobrem happy path e alternativos
- [ ] Edge cases identificados com comportamento esperado
- [ ] Constraints explícitas
- [ ] Critérios de sucesso mensuráveis
- [ ] Abordagem escolhida justificada
