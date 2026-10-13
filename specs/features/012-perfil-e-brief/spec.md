# Spec: Perfil entre projetos e brief de início de sessão

## Metadados
- **ID:** 012
- **Status:** draft (esqueleto gerado a partir dos docs em 2026-10-13; revisar contra os docs antes de aprovar)
- **Autor:** phm-aguiar
- **Data:** 2026-10-13
- **Feature Anterior:** 004
- **Roadmap:** `specs/features/roadmap.md`
- **Aprovado:** false

## Propósito
Preferências que valem para todos os projetos (o perfil) e um resumo compilado do projeto injetado no início de sessão (o brief), para o agente começar com o contexto certo.

## Escopo

### Dentro do escopo
- Perfil entre projetos: colheita sem LLM, escopos `_global`, `_profile` do workspace e de usuário, `contribute` e `consume` no marcador, união com a consulta de memória (`docs/cross-project-profile.md`, `docs/design-cross-project-profile.md`, `docs/mcp-server.md` §3.1 e §6.1).
- Comandos `profile` (`status`, `list`, `show`, `forget`, `review`, `rebuild`, `apply`) e rotas `/admin/profile/*`.
- Brief de início de sessão: `[briefing] inject_on_session_start` e `max_chars` no marcador (`docs/marker-file.md`).
- Tabelas `profile_*` (já na V01) e `memory_write_page` com `scope: "profile"`.

### Fora do escopo (explicitamente)
- Classificação e fusão do perfil por LLM (feature 016).

## Fontes (docs)
- `docs/cross-project-profile.md`, `docs/design-cross-project-profile.md`, `docs/marker-file.md`, `docs/mcp-server.md` §3 e §6.1, `docs/lifecycle-event-ingestion.md` §11.

## Dependências
- 002 (`/handoff`, MCP e marcador), 004 (evidência e retenção usadas na colheita).

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
