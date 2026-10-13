# Spec: Cadeia de admissão (webhooks antes da persistência)

## Metadados
- **ID:** 008
- **Status:** draft (esqueleto gerado a partir dos docs em 2026-10-13; revisar contra os docs antes de aprovar)
- **Autor:** phm-aguiar
- **Data:** 2026-10-13
- **Feature Anterior:** 007
- **Roadmap:** `specs/features/roadmap.md`
- **Aprovado:** false

## Propósito
O operador pode plugar webhooks HTTP em volta das escritas e das operações de ciclo de vida: bloqueantes, que podem alterar ou recusar antes do commit, e observadores, notificados depois.

## Escopo

### Dentro do escopo
- Configuração `[[admission_webhooks]]` e `AI_MEMORY_ADMISSION_WEBHOOKS_JSON` (`docs/admission-webhooks.md`, `docs/cli.md` §2).
- Operações de admissão: escrita de página, início e aceitação de handoff, consolidação, purgas, envio de mensagem (o que estiver implementado nas features anteriores).
- Cabeçalho `X-Memory-Skip-Admission-Chain` e a capacidade de pular a cadeia (`docs/mcp-server.md` §4.4, `docs/lifecycle-event-ingestion.md` §2).
- Prazo de 750 ms da admissão de `HandoffAccept` no início de sessão (`docs/design-handoff-offer-not-claim.md` §5).
- Nova higienização depois dos webhooks (`docs/wiki-files-layer.md`).

### Fora do escopo (explicitamente)
- Webhooks de saída genéricos que não sejam de admissão.

## Fontes (docs)
- `docs/admission-webhooks.md`, `docs/wiki-files-layer.md`, `docs/mcp-server.md` §4.4, `docs/lifecycle-event-ingestion.md`, `docs/design-handoff-offer-not-claim.md`.

## Dependências
- 007 (ator identificado e capacidade de pular a cadeia); 002 e 003 (operações admitidas).

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
