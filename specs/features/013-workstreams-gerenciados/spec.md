# Spec: Workstreams gerenciados (`ai-memory run`)

## Metadados
- **ID:** 013
- **Status:** draft (esqueleto gerado a partir dos docs em 2026-10-13; revisar contra os docs antes de aprovar)
- **Autor:** phm-aguiar
- **Data:** 2026-10-13
- **Feature Anterior:** 010
- **Roadmap:** `specs/features/roadmap.md`
- **Aprovado:** false

## Propósito
Uma mesma sessão de trabalho passa entre harnesses (Claude Code, Codex, OpenCode e outros) por um lançador opcional que prepara a execução, extrai a transcrição e mantém um ledger portátil de eventos.

## Escopo

### Dentro do escopo
- `ai-memory run`, `show`, `continue`, `resume`, `workstreams`, `rename-workstream`, `workstream-search` (`docs/cli.md` §7 e §9, `docs/managed-workstreams.md`).
- Rotas `/workstream/*` e seus limites (`docs/lifecycle-event-ingestion.md` §12, `docs/api-reference.md` §6).
- Adaptadores de harness: planejamento do comando, diretório de sessão, extração incremental da transcrição (`docs/harness-adapters.md`).
- Modo `--yolo` mais seguro e integração com ai-jail (`docs/design-yolo-safety-ai-jail.md`).
- Contribuições de harness gerenciado (`docs/managed-harness-contributions.md`).
- Tabelas `workstreams`, `managed_runs`, `workstream_*` (já na V01).

### Fora do escopo (explicitamente)
- Consolidação por LLM das sessões gerenciadas (feature 016).

## Fontes (docs)
- `docs/managed-workstreams.md`, `docs/harness-adapters.md`, `docs/managed-harness-contributions.md`, `docs/design-yolo-safety-ai-jail.md`, `docs/lifecycle-event-ingestion.md` §12, `docs/cli.md`.

## Dependências
- 010 (adaptadores e instalação nos demais agentes).

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
