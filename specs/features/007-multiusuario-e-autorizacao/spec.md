# Spec: Multiusuário, autenticação humana e autorização por projeto

## Metadados
- **ID:** 007
- **Status:** draft (esqueleto gerado a partir dos docs em 2026-10-13; revisar contra os docs antes de aprovar)
- **Autor:** phm-aguiar
- **Data:** 2026-10-13
- **Feature Anterior:** 002
- **Roadmap:** `specs/features/roadmap.md`
- **Aprovado:** false

## Propósito
Um servidor compartilhado precisa distinguir operadores: usuários do banco com chaves de API, login humano com sessão web e CSRF, proxy de identidade confiável, atribuição de autoria e acesso por projeto. A 002 entrega só o operador único com token de administrador.

## Escopo

### Dentro do escopo
- Usuários, identidade e atribuição; chaves de API nativas `aim_` (`docs/users.md`).
- Resolução completa do token portador: administrador, proxy de identidade confiável (`X-Memory-Actor-*`), multiusuário com hash e pepper (`docs/mcp-server.md` §4.2).
- Autenticação humana: senha (Argon2id), sessão web, CSRF, recuperação, limite por IP (`docs/mcp-server.md` §5).
- Níveis de acesso e decisão de autorização num único ponto; "distingue operadores" sem cache (§4.4).
- Autorização por projeto: grants, `project_grants` (`docs/design-per-project-authz.md`, só as partes marcadas como implementadas).
- Rotas `/admin/users*`, `/admin/api-credentials*`, `/admin/projects/grants` e `/admin/projects/access`, `/auth/*`, `/identity` (`docs/api-reference.md`).
- Filtro de dono de handoff com ator identificado (`docs/design-handoff-offer-not-claim.md` §3).

### Fora do escopo (explicitamente)
- SSO/OIDC de provedor externo (feature 009).
- Cadeia de admissão (feature 008).

## Fontes (docs)
- `docs/users.md`, `docs/design-per-project-authz.md`, `docs/mcp-server.md` §4 a §6, `docs/api-reference.md`, `docs/security.md`, `docs/security-boundaries.md`, `docs/threat-model-and-data-handling.md`.

## Dependências
- 002 (servidor HTTP e autenticação de operador único).

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
