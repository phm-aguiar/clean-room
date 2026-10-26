# Roadmap de features

> Gerado em 2026-10-13 a partir dos docs de origem (`docs/`), que são a fonte da verdade, e revisado em
> 2026-10-25. A reimplementação é em Go 1.26 e o Windows é o primeiro MVP.
>
> **Docs antes adiados:** os 14 docs de LLM, embeddings e auto-melhoria (antes em `docs-adiados/`, fora do repositório) foram incorporados a `docs/` em 2026-10-13; só os links relativos entre as duas pastas foram ajustados.
>
> **Estado em 2026-10-25:** nenhuma feature tem `plan.md` nem `tasks.md`, e não há código de reimplementação no workspace. Todas as `spec.md` foram auditadas contra o original (relatórios em `reports/`, de `009` a `014`) e seguem a regra: em toda dúvida, reproduzir o comportamento do original, inclusive nos defeitos conhecidos descritos nos docs. O plano e as tasks serão gerados depois, a partir dos docs corrigidos.

## Ordem recomendada

| # | Feature | Estado | Depende de | Por que nesta posição |
|---|---|---|---|---|
| 001 | [Núcleo](001-nucleo/spec.md) | spec auditada (`reports/009-auditoria-001.md`); sem plan/tasks | — | Domínio, armazenamento, busca e wiki. |
| 002 | [Captura e acesso](002-captura-e-acesso/spec.md) | spec auditada (`reports/010-auditoria-002.md`); sem plan/tasks | 001 | Hooks do Claude Code, sessões, handoffs, MCP e servidor. |
| 003 | [Ciclo de vida dos dados](003-ciclo-de-vida/spec.md) | spec auditada (`reports/011-auditoria-003-004-005.md`); sem plan/tasks | 002 | Purga, renomear, mover, mesclar workspace, restaurar, reset e compactação do banco. |
| 004 | [Manutenção sem LLM](004-manutencao-sem-llm/spec.md) | spec auditada (`reports/011-auditoria-003-004-005.md`); sem plan/tasks | 002, 003 | Retenção, varredura, compactação, lint e curador. Produz a lápide de decaimento. |
| 005 | [Validade temporal, arestas tipadas e sinais de ranking](005-validade-temporal/spec.md) | spec auditada (`reports/011-auditoria-003-004-005.md`); sem plan/tasks | 002, 004 | `as_of`, `include_superseded`, arestas tipadas, reapontamento de links, confiança derivada, peso de crença, recall de sessão e `settled_first`. |
| 006 | [OKF e migrações de wiki](006-okf-e-migracoes-de-wiki/spec.md) | spec auditada (`reports/012-auditoria-006-007-009.md`); sem plan/tasks | 002 | Leitura e exportação OKF e migração de lojas antigas. |
| 007 | [Multiusuário e autorização](007-multiusuario-e-autorizacao/spec.md) | spec auditada (`reports/012-auditoria-006-007-009.md`); sem plan/tasks | 002 | Usuários, chaves, login humano, acesso por projeto, `init` e `generate-auth-token`. Base de 008 a 015. |
| 008 | [Cadeia de admissão](008-cadeia-de-admissao/spec.md) | spec auditada (`reports/013-auditoria-008-011-restantes.md`); sem plan/tasks | 007, 002 a 004 | Webhooks antes da persistência; precisa do ator identificado. |
| 009 | [SSO e OIDC](009-sso-oidc/spec.md) | spec auditada (`reports/012-auditoria-006-007-009.md`); sem plan/tasks | 007 | Provedor de identidade externo sobre o modelo de usuários. |
| 010 | [Outros agentes](010-outros-agentes/spec.md) | spec auditada; sem plan/tasks | 002, 007, 009 | Hooks e MCP nos demais agentes, scripts de hook, adaptadores de transcrição, `mcp-bridge`, backfill, `doctor`, `backup-agents`. |
| 011 | [Mensagens entre projetos](011-mensagens-entre-projetos/spec.md) | spec auditada (`reports/013-auditoria-008-011-restantes.md`); sem plan/tasks | 002, 007, 008 | Caixa de entrada entre projetos e o aviso no `/handoff`. |
| 012 | [Perfil entre projetos e brief](012-perfil-e-brief/spec.md) | spec auditada; sem plan/tasks | 002, 004, 007 | Preferências globais e resumo no início de sessão (caminho sem LLM). |
| 013 | [Workstreams gerenciados](013-workstreams-gerenciados/spec.md) | spec auditada; sem plan/tasks | 010, 002, 007 | `ai-memory run` entre harnesses. |
| 014 | [Navegador web e API](014-navegador-web-e-api/spec.md) | spec auditada; sem plan/tasks | 007, 002 | Navegador HTML e `/api/v1`, somente leitura. |
| 015 | [Implantação](015-implantacao/spec.md) | spec auditada (wrappers/`setup` não verificáveis no material de referência); sem plan/tasks | 014, 007, 002 | Contêiner, releases, `upgrade`, wrappers, prefixo do servidor, proxy, Windows nativo. |
| 016 | [Cliente LLM e provedores por chave](016-cliente-llm/spec.md) | spec auditada; sem plan/tasks | 002, 015 | Fronteira única de rede para modelos: saída estruturada, erros, contingência, saúde, `llm-test`. |
| 017 | [Embeddings e busca vetorial](017-embeddings-e-busca-vetorial/spec.md) | spec auditada; sem plan/tasks | 016, 004, 002 | Embedder local padrão (MiniLM em Go puro), remotos, fluxos vetoriais, backfill, `embed`. |
| 018 | [Provedores por assinatura](018-provedores-por-assinatura/spec.md) | spec auditada; sem plan/tasks | 016, 017, 009, 015 | `anthropic-oauth`, `openai-oauth`, `codex`, `copilot` e os logins `auth`. |
| 019 | [LLM na leitura](019-llm-na-leitura/spec.md) | spec auditada; sem plan/tasks | 016, 017, 005, 002 | Reranqueamento, `answer`/`reasoning` e `memory_explore`. |
| 020 | [Consolidação por LLM](020-consolidacao-por-llm/spec.md) | spec auditada; sem plan/tasks | 016, 008, 007, 004, 012, 002 | Página única e lote, fila do fim de sessão, checkpoints, bootstrap, lint e perfil por LLM. |
| 021 | [Curadoria por embeddings](021-curadoria-por-embeddings/spec.md) | spec auditada; sem plan/tasks | 017, 016, 004, 008 | Deduplicação A3, banda de contradição, filtro de entropia e *dream*. |
| 022 | [Auto-melhoria](022-auto-melhoria/spec.md) | spec auditada; sem plan/tasks | 020, 021, 016, 014, 008, 007, 004 | Revisão, validação, portão de avaliação, pendências, aprovação, experiência e `/pending`. |

Todas as specs também passaram pela varredura de vazamentos (`reports/014-varredura-vazamentos.md`) e pela revisão de fidelidade (`reports/016-specs-fidelidade.md`). Relatórios de rodadas anteriores (citados em algumas specs) não estão mais em `reports/`.

Critério da ordem: primeiro o que completa o produto local sem LLM (003 a 006); depois identidade, que destrava rede e colaboração (007 a 009); depois os outros agentes e a colaboração entre projetos (010 a 013); depois as superfícies extras e a implantação (014 e 015); por último o bloco opcional de LLM e embeddings (016 a 022), na ordem fronteira → vetores → provedores por login → leitura → escrita → curadoria → auto-melhoria.

## Grafo de dependências

```text
001 → 002 → 003 → 004 → 005 → 019
                   004 → 012 → 020 → 022
                   004 → 017 → 021 → 022
            002 → 006
            002 → 007 → 008 → 011
                         008 → 020
                   007 → 009 → 010 → 013
                   007 → 014 → 015 → 016 → 017 → 018
                                      016 → 019
                                      016 → 020
                         014 → 022
```

(Arestas de 002 para todas as features e de 007 para 010 a 015 omitidas para legibilidade; a coluna "Depende de" é a lista completa.)

## Ajustes feitos em features anteriores durante a revisão de 014 a 022

| Onde | O quê | Doc |
|---|---|---|
| 002 | Regras de exposição 1, 4, 5 e 6 (contêiner, `--allow-insecure-no-auth`, banner) | `docs/https-via-proxy.md` § Regras de inicialização |
| 002 | Trava do sistema operacional sobre `.serve.lock`, detentor em `.serve.lock.holder`, repetição de 25 ms até 5 tentativas, aviso quando o sistema de arquivos não trava | `docs/windows.md` § Single instance |
| 002 | `--project-strategy basename\|repo-root` no `install-hooks` | `docs/marker-file.md` § install-wide default |
| 001/002 | Sanitizador com os 27 padrões exatos, `allowlist` e `extra_patterns` | `docs/prompts-schemas-and-sanitizer.md` §11, `docs/domain-vocabulary.md` §7.3 |
| 005 | Confiança derivada, peso de crença, recall de sessão, `settled_first`, `evidence_rows` | `docs/storage-and-retrieval.md` §11.5 e §13, `docs/mcp-server.md` §3.2, `docs/design-hindsight-borrowings.md` |
| 007 | Regras de exposição 2 e 3 (`secure_cookie` obrigatório com autenticação humana fora de loopback) | `docs/https-via-proxy.md` |
| 013 | Resolução de executável com PATHEXT e `cmdc` no Windows | `docs/windows-native-extras.md` §9 |
| Constituição | Fases (001 a 015 zero-LLM; 016 a 022 com LLM) e política dos docs | — |
| 001, 002, 004, 007, 008, 012, 013, 014, 015 | Referências a "F003" e à antiga 016 única trocadas pelas features deste roadmap | — |

## Docs sem feature própria

| Doc | Onde entra |
|---|---|
| `docs/overview.md`, `architecture.md`, `design-decisions.md`, `domain-vocabulary.md`, `database-schema.md`, `storage-and-retrieval.md`, `wiki-files-layer.md` | Base de todas as features (001 e 002, mais as partes que cada uma usa). |
| `docs/complementary-contracts.md`, `security.md`, `security-boundaries.md`, `threat-model-and-data-handling.md` | Transversais; cada feature confere a parte que toca. |
| `docs/programmatic-memory.md` | Contrato de cliente HTTP; validação das rotas da 002 e da 007. |
| `docs/usage.md`, `cookbook.md`, `test-fixtures.md`, `install.md` | Guias e fixtures; entram nos testes e no README. |
| `docs/companions.md`, `external-companions-and-tests.md` | **Fora do núcleo** por definição do próprio doc (produtos separados). Sem feature. |
| `docs/design-rules-promotion.md` | Proposta no próprio doc; fora. |
| `docs/jev-reranker-adapter.md` §2 | Proposta não implementada (adaptador externo); fora. |
| `docs/vector-backend-policy.md` § Propostas | `sqlite-vec` adiado pelo próprio doc; a 017 segue as decisões D1 a D4. |
