# Spec: Navegador web da wiki e API JSON `/api/v1`

## Metadados
- **ID:** 014
- **Status:** draft (esqueleto gerado a partir dos docs em 2026-10-13; revisar contra os docs antes de aprovar)
- **Autor:** phm-aguiar
- **Data:** 2026-10-13
- **Feature Anterior:** 007
- **Roadmap:** `specs/features/roadmap.md`
- **Aprovado:** false

## Propósito
Duas superfícies somente leitura sobre o mesmo estado da memória: um navegador web da wiki e uma API JSON para frontends de terceiros, com SPA própria opcional.

## Escopo

### Dentro do escopo
- Navegador web da wiki e API JSON somente leitura (`docs/wiki-browser.md`).
- Integração de frontends com `/api/v1` e hospedagem de SPA (`docs/frontend-api.md`).
- Autenticação dupla (token portador ou sessão web) nessas rotas (`docs/mcp-server.md` §5, `docs/cli.md` §3.3).

### Fora do escopo (explicitamente)
- Edição pela web.

## Fontes (docs)
- `docs/wiki-browser.md`, `docs/frontend-api.md`, `docs/mcp-server.md` §5, `docs/cli.md` §3.3.

## Dependências
- 007 (sessão web e CSRF).

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
