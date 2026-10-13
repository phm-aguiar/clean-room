# Roadmap de features

> Gerado em 2026-10-13 a partir dos docs de origem (`docs/`), que são a fonte da verdade. Substitui o "F003 backlog" único do discovery (`reports/001-ai-memory-go-clean-room-discovery.md`) por features menores, cada uma com seus docs. As specs de 003 em diante são esqueletos (`Aprovado: false`): antes de aprovar, cada uma é revisada contra os docs listados nela.

## Ordem recomendada

| # | Feature | Estado | Depende de | Por que nesta posição |
|---|---|---|---|---|
| 001 | [Núcleo](001-nucleo/spec.md) | concluída | — | Domínio, armazenamento, busca e wiki. |
| 002 | [Captura e acesso](002-captura-e-acesso/spec.md) | aprovada, `tasks.md` pronto (88 tasks) | 001 | Hooks do Claude Code, sessões, handoffs, MCP e servidor. Inclui as correções da 001 contra os docs. |
| 003 | [Ciclo de vida dos dados](003-ciclo-de-vida/spec.md) | esqueleto | 002 | Purga, renomear, mover, restaurar e reset: o operador corrige a memória sem mexer no banco. Traz os tombstones de purga. |
| 004 | [Manutenção sem LLM](004-manutencao-sem-llm/spec.md) | esqueleto | 002, 003 | Retenção, varredura de esquecimento, compactação, lint e curador. Produz a lápide de decaimento que a ressurreição consome. |
| 005 | [Validade temporal e arestas tipadas](005-validade-temporal/spec.md) | esqueleto | 002 | `as_of` e `include_superseded` no `memory_query`; a 001 já grava as janelas. |
| 006 | [OKF e migrações de wiki](006-okf-e-migracoes-de-wiki/spec.md) | esqueleto | 002 | Formato da wiki e migração de lojas antigas, antes de outros produtos lerem os arquivos. |
| 007 | [Multiusuário e autorização](007-multiusuario-e-autorizacao/spec.md) | esqueleto | 002 | Usuários, chaves de API, login humano, acesso por projeto. Base de 008, 009, 014 e 015. |
| 008 | [Cadeia de admissão](008-cadeia-de-admissao/spec.md) | esqueleto | 007 | Webhooks antes da persistência; precisa do ator identificado. |
| 009 | [SSO e OIDC](009-sso-oidc/spec.md) | esqueleto | 007 | Provedor de identidade externo sobre o modelo de usuários. |
| 010 | [Outros agentes](010-outros-agentes/spec.md) | esqueleto | 002 | Hooks e MCP nos demais agentes, scripts de hook, `mcp-bridge`, backfill e `doctor`. |
| 011 | [Mensagens entre projetos](011-mensagens-entre-projetos/spec.md) | esqueleto | 002 | Caixa de entrada entre projetos e o aviso no `/handoff`. |
| 012 | [Perfil entre projetos e brief](012-perfil-e-brief/spec.md) | esqueleto | 002, 004 | Preferências globais e resumo no início de sessão. |
| 013 | [Workstreams gerenciados](013-workstreams-gerenciados/spec.md) | esqueleto | 010 | `ai-memory run` entre harnesses. |
| 014 | [Navegador web e API](014-navegador-web-e-api/spec.md) | esqueleto | 007 | Superfícies somente leitura; exige sessão web. |
| 015 | [Implantação](015-implantacao/spec.md) | esqueleto | 007, 002 | Contêiner, offline, HTTPS e Windows nativo; exposição em rede exige autenticação. |
| 016 | [LLM e embeddings](016-llm-e-embeddings/spec.md) | esqueleto, **bloqueada por falta de doc** | 004, 002 | Opcional pela INV-13. Os docs `docs-adiados/` citados não existem neste repositório. |

Critério da ordem: primeiro o que completa o produto local sem LLM (003 a 006); depois identidade, que destrava rede e colaboração (007 a 009); depois os outros agentes e a colaboração entre projetos (010 a 013); por último as superfícies extras, a implantação e o que depende de LLM (014 a 016).

## Grafo de dependências

```text
001 → 002 → 003 → 004 → 012
            002 → 005
            002 → 006
            002 → 007 → 008
                   007 → 009
                   007 → 014
                   007 → 015
            002 → 010 → 013
            002 → 011
            004 → 016
```

## Docs sem feature própria

| Doc | Onde entra |
|---|---|
| `docs/overview.md`, `architecture.md`, `design-decisions.md`, `domain-vocabulary.md`, `database-schema.md`, `storage-and-retrieval.md`, `wiki-files-layer.md` | Base de todas as features (001 e 002, mais as partes que cada uma usa). |
| `docs/complementary-contracts.md`, `security.md`, `security-boundaries.md`, `threat-model-and-data-handling.md` | Transversais; cada feature confere a parte que toca. |
| `docs/programmatic-memory.md` | Contrato de cliente HTTP; validação das rotas da 002 e da 007. |
| `docs/usage.md`, `cookbook.md`, `test-fixtures.md` | Guias e fixtures; entram nos testes e no README. |
| `docs/companions.md`, `external-companions-and-tests.md` | **Fora do núcleo** por definição do próprio doc (produtos separados). Sem feature. |
