# Spec: Integração com outros agentes (hooks, MCP, scripts, backfill, doctor)

## Metadados
- **ID:** 010
- **Status:** draft (esqueleto gerado a partir dos docs em 2026-10-13; revisar contra os docs antes de aprovar)
- **Autor:** phm-aguiar
- **Data:** 2026-10-13
- **Feature Anterior:** 002
- **Roadmap:** `specs/features/roadmap.md`
- **Aprovado:** false

## Propósito
A promessa do produto é trocar de agente sem reexplicar contexto. A 002 cobre só o Claude Code; esta feature leva a captura e o MCP aos demais agentes da matriz de suporte.

## Escopo

### Dentro do escopo
- `install-hooks` e `install-mcp` para os agentes da matriz de suporte (Codex, Cursor, OpenCode, Gemini CLI e demais), com o esquema de ferramenta de cada um (`docs/support-matrix.md`, `docs/mcp-install.md`, `docs/lifecycle-event-ingestion.md` §3.5).
- Scripts de hook POSIX e PowerShell com o mesmo contrato do hook nativo, e implantação em contêiner (`docs/hook-scripts-and-container-deploy.md`, `docs/cli.md` §5.4).
- Ponte `mcp-bridge` com identificação de sessão e modo `per_session` (`docs/mcp-install.md`, `docs/auto-scope.md`).
- Captura externa por orquestrador (`docs/external-lifecycle.md`).
- Backfill do histórico local anterior ao hook (`docs/design-boot-backfill.md`) e `doctor` (`docs/cli.md` §8).
- `install-instructions`, `install-skills`, `setup-agent`, `uninstall` simétrico (`docs/cli.md` §8).
- Captura da mensagem do assistente com opt-in duplo (`docs/lifecycle-event-ingestion.md` §5) e checkpoint de turno do OpenCode (§8.4).

### Fora do escopo (explicitamente)
- Lançamento gerenciado `ai-memory run` (feature 013).

## Fontes (docs)
- `docs/support-matrix.md`, `docs/mcp-install.md`, `docs/hook-scripts-and-container-deploy.md`, `docs/external-lifecycle.md`, `docs/design-boot-backfill.md`, `docs/auto-scope.md`, `docs/lifecycle-event-ingestion.md`, `docs/cli.md`, `docs/harness-adapters.md`.

## Dependências
- 002 (ingestão, spool, MCP e `install-*` do Claude Code).

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
