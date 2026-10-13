# Spec: SSO e identidade corporativa (OIDC)

## Metadados
- **ID:** 009
- **Status:** draft (esqueleto gerado a partir dos docs em 2026-10-13; revisar contra os docs antes de aprovar)
- **Autor:** phm-aguiar
- **Data:** 2026-10-13
- **Feature Anterior:** 007
- **Roadmap:** `specs/features/roadmap.md`
- **Aprovado:** false

## Propósito
Integrar o servidor a um provedor de identidade que emite tokens OpenID Connect: login do desenvolvedor, credenciais de cliente e mapeamento de identidade para o modelo de usuários.

## Escopo

### Dentro do escopo
- Fluxo de login do desenvolvedor e credenciais OIDC do cliente (`auth.json` no diretório de dados, `ai-memory auth login|logout|status`) (`docs/sso.md`, `docs/cli.md` §2.5 e §9).
- Mapeamento `issuer` + `sub` para usuário; administrador por par exato (`docs/mcp-server.md` §4.2).
- Renovação de credencial OIDC na drenagem do spool (`docs/cli.md` §5.2).

### Fora do escopo (explicitamente)
- Provedores que não sejam OIDC.

## Fontes (docs)
- `docs/sso.md`, `docs/users.md`, `docs/mcp-server.md` §4, `docs/cli.md`.

## Dependências
- 007 (modelo de usuários e autorização).

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
