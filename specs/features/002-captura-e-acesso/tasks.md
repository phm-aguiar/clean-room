---
graph-operators: enabled
heartbeat-threshold: 4
max-rounds: 40
---

# tasks.md: Captura e acesso (hooks, sessões, handoffs, MCP e CLI)

> Feature `002-captura-e-acesso` · Spec: `spec.md` (Aprovado: true, revisada contra os docs em 2026-10-13) · Plano: `plan.md` (Q-01 a Q-13) · Discovery: `reports/001-ai-memory-go-clean-room-discovery.md` (F002)
>
> Regras: uma ação por task; implementação e seu `_test.go` contam como um grupo de arquivos relacionado. `go.mod`/`go.sum` só são editados em T007 e T071. Cada comando novo da CLI expõe um construtor no próprio arquivo; só a T052 edita `internal/cli/root.go`. `Wiki-Keywords` apontam para termos e docs de `docs/` (o `wiki-claude/` do SDD não existe neste repo). Neste projeto o doc vence a spec: divergência encontrada durante uma task é corrigida seguindo o doc e registrada na nota da fase 0 ou na auditoria.


## Fase 0: Descoberta

- [x] **T001:** Extrair o contrato do evento de hook do Claude Code (campos por evento, `session_id`, `cwd`, `tool_name`, `tool_use_id`, `tool_input`, `tool_response`) e listar as fixtures de payload a versionar
  - **Papel:** researcher
  - **agent:** researcher1
  - **Dependências:** Nenhuma
  - **depends_on:** []
  - **Paralelizável:** true
  - **Arquivos:** `specs/features/002-captura-e-acesso/notes/hook-contract.md`
  - **Wiki-Keywords:** lifecycle-event-ingestion, harness, Claude Code, evento normalizado

- [x] **T002:** Extrair o contrato de handoff: seleção, reivindicação, nota de oferta, expiração e textos do handoff automático e da página de sessão
  - **Papel:** researcher
  - **agent:** researcher2
  - **Dependências:** Nenhuma
  - **depends_on:** []
  - **Paralelizável:** true
  - **Arquivos:** `specs/features/002-captura-e-acesso/notes/handoff-contract.md`
  - **Wiki-Keywords:** design-handoff-offer-not-claim, handoff automático, sessão viva, página de sessão

- [x] **T003:** Extrair o contrato das 12 tools MCP (argumentos, aliases, limites, respostas, erros) e da resolução de escopo das tools
  - **Papel:** researcher
  - **agent:** researcher3
  - **Dependências:** Nenhuma
  - **depends_on:** []
  - **Paralelizável:** true
  - **Arquivos:** `specs/features/002-captura-e-acesso/notes/mcp-contract.md`
  - **Wiki-Keywords:** mcp-server, ferramentas MCP, ScopeSource, chave do ator

- [x] **T004:** Avaliar o `mark3labs/mcp-go`: versão que compila com Go 1.26 e `CGO_ENABLED=0`, fixação do protocolo 2024-11-05, HTTP streamable sem estado com resposta JSON, stdio e acesso ao contexto da requisição
  - **Papel:** researcher
  - **agent:** researcher4
  - **Dependências:** Nenhuma
  - **depends_on:** []
  - **Paralelizável:** true
  - **Arquivos:** `specs/features/002-captura-e-acesso/notes/mcp-go-evaluation.md`
  - **Wiki-Keywords:** mcp-go, protocolo MCP, streamable HTTP, ADR-05

- [x] **T005:** Extrair o contrato do marcador `.ai-memory.toml` e da política de captura (caminhada, leitura por linha, `[capture]` estrito, padrões, limites, decisões e corpo substituto)
  - **Papel:** researcher
  - **agent:** researcher5
  - **Dependências:** Nenhuma
  - **depends_on:** []
  - **Paralelizável:** true
  - **Arquivos:** `specs/features/002-captura-e-acesso/notes/capture-contract.md`
  - **Wiki-Keywords:** marker-file, capture exclusions, ignore_paths, MetadataOnly

- [x] **T006:** Consolidar as 5 notas, apontar lacunas, registrar "decisão nossa" onde o doc for omisso e listar divergências spec x doc encontradas
  - **Papel:** analyst
  - **agent:** analyst1
  - **Dependências:** T001, T002, T003, T004, T005
  - **depends_on:** [T001, T002, T003, T004, T005]
  - **Paralelizável:** false
  - **Arquivos:** `specs/features/002-captura-e-acesso/notes/phase0-findings.md`
  - **Wiki-Keywords:** a confirmar, ADR, constitution, decisão nossa


## Fase 1: Fundação

- [x] **T007:** Adicionar `github.com/mark3labs/mcp-go` ao `go.mod` na versão escolhida em T004 e fixar a versão em `tech.md`
  - **Papel:** executor
  - **agent:** executor1
  - **Dependências:** T004, T006
  - **depends_on:** [T004, T006]
  - **Paralelizável:** true
  - **Arquivos:** `go.mod`, `go.sum`, `.github/memory/tech.md`
  - **Wiki-Keywords:** tech.md, mcp-go, dependências

- [x] **T008:** Criar o evento canônico de hook no domínio: vocabulário e aliases, tipo de observação, importância por evento e limite de corpo por evento, com teste
  - **Papel:** executor
  - **agent:** executor2
  - **Dependências:** T001, T006
  - **depends_on:** [T001, T006]
  - **Paralelizável:** true
  - **Arquivos:** `internal/domain/hookevent.go`, `internal/domain/hookevent_test.go`
  - **Wiki-Keywords:** vocabulário de eventos, importância, limites de corpo, domain-vocabulary

- [x] **T009:** Criar a conversão de identificador de sessão: UUID nativo mantido, string qualquer vira UUID v5 no namespace OID, com teste
  - **Papel:** executor
  - **agent:** executor3
  - **Dependências:** T006
  - **depends_on:** [T006]
  - **Paralelizável:** true
  - **Arquivos:** `internal/domain/sessionid.go`, `internal/domain/sessionid_test.go`
  - **Wiki-Keywords:** UUID v5, session_id, idempotente

- [x] **T010:** Acrescentar à configuração `bind`, `server_url`, `allowed_hosts`, `[auth] token`, `[handoff]`, `[routing] mid_session`, `[auto_scope] mode`, e o diretório pessoal e o `CLAUDE_CONFIG_DIR` capturados uma vez, com validação e teste
  - **Papel:** executor
  - **agent:** executor1
  - **Dependências:** T006, T079
  - **depends_on:** [T006, T079]
  - **Paralelizável:** true
  - **Arquivos:** `internal/config/config.go`, `internal/config/config_test.go`
  - **Wiki-Keywords:** koanf, INV-01, cli.md configuração, auto_scope

- [x] **T011:** Criar `config.LoadHook`, o carregador mínimo do caminho rápido (data_dir, server_url, arquivo do token e `AI_MEMORY_HOOK_*` em minutos até 60), com teste
  - **Papel:** executor
  - **agent:** executor1
  - **Dependências:** T010
  - **depends_on:** [T010]
  - **Paralelizável:** false
  - **Arquivos:** `internal/config/hook.go`, `internal/config/hook_test.go`
  - **Wiki-Keywords:** caminho rápido, INV-01, variáveis de tempo do hook

- [x] **T012:** Atualizar o teste de grafo de imports com `ingest` e `mcp` (`mcp` nunca importa `ingest` em produção)
  - **Papel:** executor
  - **agent:** executor2
  - **Dependências:** T006
  - **depends_on:** [T006]
  - **Paralelizável:** true
  - **Arquivos:** `internal/archtest/archtest_test.go`
  - **Wiki-Keywords:** grafo de imports, constitution, mcp, ingest

- [x] **T013:** Criar o arquivo .feature da 002 com os cenários da spec (1 principal, 5 alternativos, 7 de borda)
  - **Papel:** executor
  - **agent:** executor3
  - **Dependências:** T006
  - **depends_on:** [T006]
  - **Paralelizável:** true
  - **Arquivos:** `specs/features/002-captura-e-acesso/acceptance/captura.feature`
  - **Wiki-Keywords:** Gherkin, cenários, aceitação


## Fase 1b: Correções da 001 contra os docs

> Divergências do código da 001 com os docs, encontradas na validação de 2026-10-13 antes de executar este DAG. Cada uma afeta algo que a 002 usa. IDs a partir de T079 para não renumerar as tasks já geradas.

- [x] **T079:** Corrigir o diretório de dados da configuração: padrão no diretório de dados local do usuário com a subpasta `ai-memory`, flag global `--data-dir`, precedência `--data-dir` > `AI_MEMORY_DATA_DIR` > arquivo, e `config.toml` lido do diretório de dados quando `--config` falta, com teste
  - **Papel:** executor
  - **agent:** executor1
  - **Dependências:** T006
  - **depends_on:** [T006]
  - **Paralelizável:** true
  - **Arquivos:** `internal/config/config.go`, `internal/config/config_test.go`, `internal/cli/root.go`
  - **Wiki-Keywords:** design-decisions diretório de dados, cli.md fluxo de execução, AI_MEMORY_DATA_DIR, INV-11

- [x] **T080:** Criar as escritas rastreadas no adaptador git (registro de caminhos e commit dos registrados, restaurando `written_paths` se `git add` falhar antes do commit) e tirar o commit do `WritePage` e do `DeletePage`: quem chama faz o commit no ponto definido pelo doc (`memory_write_page`, fim de sessão, checkpoints), com teste
  - **Papel:** executor
  - **agent:** executor2
  - **Dependências:** T006
  - **depends_on:** [T006]
  - **Paralelizável:** true
  - **Arquivos:** `internal/wiki/gitrepo.go`, `internal/wiki/gitrepo_test.go`, `internal/wiki/write.go`, `internal/wiki/write_test.go`
  - **Wiki-Keywords:** wiki-files-layer histórico Git, escritas rastreadas, commits automáticos

- [x] **T081:** Criar a rota `POST /admin/backup` (valida a árvore da wiki, snapshot online do banco, tarball gzip com `wiki/`, `db/memory.sqlite` e `config.toml` se existir, `application/gzip`, 500 com erro JSON), com teste
  - **Papel:** executor
  - **agent:** executor3
  - **Dependências:** T006
  - **depends_on:** [T006]
  - **Paralelizável:** true
  - **Arquivos:** `internal/cli/admin_backup.go`, `internal/cli/admin_backup_test.go`
  - **Wiki-Keywords:** lifecycle-ops backup, tarball, snapshot online, INV-09

- [x] **T082:** Trocar o comando `backup --to` (atalho `-o`) para pedir o tarball ao servidor e gravar o corpo no arquivo, e ajustar o smoke test da 001, com teste
  - **Papel:** executor
  - **agent:** executor3
  - **Dependências:** T081, T010
  - **depends_on:** [T081, T010]
  - **Paralelizável:** true
  - **Arquivos:** `internal/cli/backup_cmd.go`, `internal/cli/backup_cmd_test.go`, `test/smoke/smoke_test.go`
  - **Wiki-Keywords:** backup, cli.md backup não é direto, cliente HTTP

- [x] **T083:** Fazer o reindex tratar `_meta.md` ausente ou inválido como erro com o caminho exato, em vez de reconstruir o nome pelo id, e contabilizar `skipped_purged_sessions` para páginas de sessões em `purged_sessions`, com teste
  - **Papel:** executor
  - **agent:** executor1
  - **Dependências:** T022
  - **depends_on:** [T022]
  - **Paralelizável:** true
  - **Arquivos:** `internal/wiki/reindex.go`, `internal/wiki/reindex_test.go`
  - **Wiki-Keywords:** lifecycle-ops reindex, manifesto, _meta.md

- [x] **T084:** Criar a checagem de banco limpo do comando `reindex` (no sistema de referência consulta as 5 tabelas `sessions`, `observations`, `handoffs`, `users` e `audit_log`; pode também reportar páginas/escopos existentes), com teste
  - **Papel:** executor
  - **agent:** executor2
  - **Dependências:** T041, T079
  - **depends_on:** [T041, T079]
  - **Paralelizável:** true
  - **Arquivos:** `internal/storage/rowcounts.go`, `internal/storage/rowcounts_test.go`, `internal/cli/reindex_cmd.go`, `internal/cli/reindex_cmd_test.go`
  - **Wiki-Keywords:** clean-target check, reindex, estado só do banco

- [x] **T085:** Criar o preenchimento idempotente de `_meta.md` para escopos sem manifesto, chamado no boot do `serve` (fazendo commit `"bootstrap missing scope metadata"` quando algum arquivo é gravado), com teste
  - **Papel:** executor
  - **agent:** executor3
  - **Dependências:** T006
  - **depends_on:** [T006]
  - **Paralelizável:** true
  - **Arquivos:** `internal/wiki/meta_backfill.go`, `internal/wiki/meta_backfill_test.go`
  - **Wiki-Keywords:** backfill de manifestos, _meta.md, ordem de boot

- [x] **T086:** Fazer o watcher registrar no adaptador git tanto o caminho removido (`Remove`) quanto o caminho criado/modificado (`Create`/`Modify`), para que o próximo commit inclua as alterações do watcher, com teste
  - **Papel:** executor
  - **agent:** executor2
  - **Dependências:** T022, T080
  - **depends_on:** [T022, T080]
  - **Paralelizável:** true
  - **Arquivos:** `internal/wiki/watcher.go`, `internal/wiki/watcher_test.go`
  - **Wiki-Keywords:** watcher, remoção, escritas rastreadas

- [x] **T087:** Fazer a exclusão por caminho remover fisicamente todas as versões do caminho, limpar entidades órfãs e gravar `audit_log` (`delete_page`) e, na variante condicional com versão diferente da esperada, não fazer nada e informar que não removeu; `wiki.DeletePage` restaura o arquivo nesse caso; com teste
  - **Papel:** executor
  - **agent:** executor1
  - **Dependências:** T006
  - **depends_on:** [T006]
  - **Paralelizável:** true
  - **Arquivos:** `internal/storage/delete.go`, `internal/storage/delete_test.go`, `internal/wiki/delete.go`, `internal/wiki/delete_test.go`
  - **Wiki-Keywords:** storage-and-retrieval exclusão de página, versão esperada, rollback

- [x] **T088:** Corrigir a ressurreição de lápide: busca a lápide mais recente (`is_latest = 0 AND superseded_at IS NOT NULL ORDER BY superseded_at DESC LIMIT 1`), a nova versão encadeia na lápide (`version = tombstone.version + 1`), o `superseded_at` da lápide é limpo (`NULL`) e o `valid_to` original é mantido, com auditoria, e com teste
  - **Papel:** executor
  - **agent:** executor2
  - **Dependências:** T006
  - **depends_on:** [T006]
  - **Paralelizável:** true
  - **Arquivos:** `internal/storage/upsert.go`, `internal/storage/upsert_test.go`
  - **Wiki-Keywords:** ressurreição de tombstone, superseded_at, caso C

- [x] **T089:** Alinhar a escrita e a exclusão de página ao fluxo de `docs/wiki-files-layer.md` §5.2: sem linha em `log.md` (o ledger é só de eventos de hook e o `okf.md` diz que o ai-memory não adota `log.md`), sem escrever `index.md` (só a migração OKF da 006 o cria), páginas sob `_slots/` marcadas como fixadas; remover o código que fica sem uso e ajustar os testes da 001 que esperavam essas linhas
  - **Papel:** executor
  - **agent:** executor3
  - **Dependências:** T080, T083, T086, T087
  - **depends_on:** [T080, T083, T086, T087]
  - **Paralelizável:** false
  - **Arquivos:** `internal/wiki/write.go`, `internal/wiki/write_test.go`, `internal/wiki/delete.go`, `internal/wiki/delete_test.go`, `internal/wiki/ledger.go`, `internal/wiki/meta.go`, `internal/wiki/meta_ledger_test.go`, `internal/wiki/reindex_test.go`, `internal/wiki/watcher_test.go`, `test/acceptance/write_page_test.go`
  - **Wiki-Keywords:** wiki-files-layer escrita de página, okf log.md, index.md, _slots

- [x] **T090:** Fazer o frontmatter representar mapeamentos aninhados e listas de mapeamentos, preservando a ordem das chaves na leitura e na emissão, para `generated: {by, at}`, `sources` e `relations: {causes|fixes|contradicts: [...]}` (`docs/okf.md`, `docs/typed-edges.md`); nulo e alias continuam recusados, exceto onde o doc aceita nulo (`entities`); com teste
  - **Papel:** executor
  - **agent:** executor1
  - **Dependências:** T006
  - **depends_on:** [T006]
  - **Paralelizável:** true
  - **Arquivos:** `internal/domain/page.go`, `internal/domain/page_test.go`, `internal/wiki/markdown.go`, `internal/wiki/markdown_test.go`
  - **Wiki-Keywords:** frontmatter, mapeamento aninhado, okf generated, sources, relations

- [x] **T091:** Alinhar a conformidade OKF da escrita nos 6 passos ordenados (`okf_version: "0.2"`, `id`, `kind`, `generated: {by, at}` migrando chaves planas `generated.by`/`generated.at`, `sources` derivado de `session_id` e `agent` sem remover os originais, e `entities: []` padrão) e ler `relations` como mapeamento por relação; com teste
  - **Papel:** executor
  - **agent:** executor1
  - **Dependências:** T089, T090
  - **depends_on:** [T089, T090]
  - **Paralelizável:** false
  - **Arquivos:** `internal/wiki/write.go`, `internal/wiki/write_test.go`, `internal/wiki/derive.go`, `internal/wiki/derive_test.go`
  - **Wiki-Keywords:** okf proveniência, generated.by, sources, relations

- [x] **T092:** Criar a semente de leitura do ponteiro de projeto ativo no boot do transporte HTTP: projeto da observação mais recente dentro de `session_ttl_secs`, num slot próprio, só para leitura, aplicada só com o slot compartilhado vazio, descartada no primeiro evento de primeiro plano e nas invalidações administrativas; falha só registrada (`docs/auto-scope.md` § "Surviving a restart"); com teste
  - **Papel:** executor
  - **agent:** executor2
  - **Dependências:** T019, T020
  - **depends_on:** [T019, T020]
  - **Paralelizável:** false
  - **Arquivos:** `internal/storage/startup_seed.go`, `internal/storage/startup_seed_test.go`
  - **Wiki-Keywords:** auto-scope, semente de startup, StartupSeed, reinício


- [x] **T093:** Alinhar o sanitizador da 001 ao contrato exato: os 27 padrões de `docs/prompts-schemas-and-sanitizer.md` §11.2 na ordem e com os limiares da tabela (inclui `auth_header` amplo com qualificador de autenticação, `env_secret` com a lista de variáveis de provedor e forma sem diferença de caixa, `telegram_token` nas duas formas, `jwt` com 16 ou mais por segmento, `aws_key` com borda de palavra); remoção de controle restrita à lista do §11.1; `[sanitize].allowlist` checada por ocorrência e `[sanitize].extra_patterns` com rótulo `custom`, expressão inválida impedindo o início (`docs/domain-vocabulary.md` §7.3, removendo o adiamento ADR-13 da 001); com teste por padrão (positivo, negativo e os casos de falso positivo do §11.4)
  - **Papel:** executor
  - **agent:** executor3
  - **Dependências:** T010, T079
  - **depends_on:** [T010, T079]
  - **Paralelizável:** false
  - **Arquivos:** `internal/domain/sanitize/patterns.go`, `internal/domain/sanitize/sanitize.go`, `internal/domain/sanitize/sanitize_test.go`, `internal/config/config.go`, `internal/config/config_test.go`
  - **Wiki-Keywords:** sanitizador, INV-06, allowlist, extra_patterns, REDACTED

## Fase 2: Implementação


### 2A Storage

- [x] **T014:** Criar sessões: abrir ou tocar, encerrar e a definição única de sessão efêmera, com teste
  - **Papel:** executor
  - **agent:** executor1
  - **Dependências:** T008
  - **depends_on:** [T008]
  - **Paralelizável:** true
  - **Arquivos:** `internal/storage/sessions.go`, `internal/storage/sessions_test.go`
  - **Wiki-Keywords:** sessão efêmera, sessions, encerramento

- [x] **T015:** Criar a chave de ingestão: reivindicar (`new`, `pending`, `completed`), concluir e reter por 30 dias, com teste
  - **Papel:** executor
  - **agent:** executor2
  - **Dependências:** T006
  - **depends_on:** [T006]
  - **Paralelizável:** true
  - **Arquivos:** `internal/storage/ingestkeys.go`, `internal/storage/ingestkeys_test.go`
  - **Wiki-Keywords:** ingest_keys, idempotência, pelo menos uma vez

- [x] **T016:** Criar a gravação de handoff: inserir com expiração prévia dos automáticos do mesmo `cwd` exato e mesmo dono, reabrir o handoff mais recente aceito por uma sessão efêmera (`ReopenLastAcceptedHandoffForSessionTx`) e cancelar por id, com teste
  - **Papel:** executor
  - **agent:** executor3
  - **Dependências:** T014, T002
  - **depends_on:** [T014, T002]
  - **Paralelizável:** true
  - **Arquivos:** `internal/storage/handoffs.go`, `internal/storage/handoffs_test.go`
  - **Wiki-Keywords:** handoffs, expiração, supersessão de handoff

- [x] **T017:** Criar a seleção de handoff (dono, aberto, manual antes de automático, `cwd` ancestral por componente sem diferenciar caixa no Windows, sessão viva de 10 min, desempate) e a listagem (`state`, `limit` 20..100, `created_at DESC`), com teste
  - **Papel:** executor
  - **agent:** executor3
  - **Dependências:** T016
  - **depends_on:** [T016]
  - **Paralelizável:** false
  - **Arquivos:** `internal/storage/handoff_select.go`, `internal/storage/handoff_select_test.go`
  - **Wiki-Keywords:** seleção de handoff, busy_since, fronteira de caminho

- [x] **T018:** Criar a reivindicação atômica de handoff com duas guardas de `open`, sessão viva reavaliada quando `busy_since` é informado (`busy_since = None` quando chamado por `handoff_id` explícito), idempotência do receptor (`consumed_by_hook`) e varredura pós-reivindicação (sempre que o handoff reivindicado for automático), com teste multi-sessão
  - **Papel:** executor
  - **agent:** executor3
  - **Dependências:** T017
  - **depends_on:** [T017]
  - **Paralelizável:** false
  - **Arquivos:** `internal/storage/handoff_claim.go`, `internal/storage/handoff_claim_test.go`
  - **Wiki-Keywords:** INV-16, reivindicação atômica, uso único, multi-sessão

- [x] **T019:** Criar o ponteiro de projeto ativo em memória (modos `per_actor`, `per_session` e `single`, TTL 1 h, teto 4096, marca `has_keyed_entry`, `Lookup` e `Read`); `session_ttl_secs = 0` vira 1 h, `max_entries = 0` vira 1, modo inválido impede a subida e o modo efetivo vai para o log `active-project isolation mode` (`docs/auto-scope.md`), com teste
  - **Papel:** executor
  - **agent:** executor1
  - **Dependências:** T006
  - **depends_on:** [T006]
  - **Paralelizável:** true
  - **Arquivos:** `internal/storage/activeproject.go`, `internal/storage/activeproject_test.go`
  - **Wiki-Keywords:** ponteiro de projeto ativo, ActiveProjectMode, Mismatch

- [x] **T020:** Ligar o ponteiro ao `ResolveScope`: leitura sem escopo degrada (ponteiro, semente, padrão) com a origem; escrita sem escopo recusa em `Mismatch` (`-32602 ambiguous unscoped write`) e recai para o padrão em `Unset`; escopo explícito inexistente falha fechado; origens serializadas no wire JSON alinhadas ao sistema de referência (`explicit`, `active_project`, `active_project_shared`, `active_project_fallback`, `default`, `default_fallback`), com a resolução por caminho de repositório saindo da leitura e indo para a resolução do hook (T027); argumentos explícitos como em `docs/auto-scope.md` (`project` em forma `workspace/project` só vira par se o nome exato não existir; `project` sozinho resolve no workspace ativo e depois no padrão; lista de escopos sem duplicata e com limite); com teste
  - **Papel:** executor
  - **agent:** executor1
  - **Dependências:** T019
  - **depends_on:** [T019]
  - **Paralelizável:** false
  - **Arquivos:** `internal/storage/scope_read.go`, `internal/storage/scope_read_test.go`
  - **Wiki-Keywords:** ScopeSource, resolução de escopo, falha fechada, escrita ambígua


### 2B Wiki

- [x] **T021:** Criar o ledger mensal de eventos de hook (`log-AAAA-MM.md`, linha `## [ts] evento | título`, título até 120 caracteres), com teste
  - **Papel:** executor
  - **agent:** executor2
  - **Dependências:** T008
  - **depends_on:** [T008]
  - **Paralelizável:** true
  - **Arquivos:** `internal/wiki/hooklog.go`, `internal/wiki/hooklog_test.go`
  - **Wiki-Keywords:** ledger, log do projeto, somente acréscimo

- [x] **T022:** Ajustar o reconhecimento de ledger para a regra de nome e conteúdo (`log.md` ou `log-AAAA-MM.md` exatos e conteúdo que abre com entrada, lendo até 8 KiB; frontmatter sem fechamento em até 64 linhas ou linha vazia no início decidem contra; `docs/complementary-contracts.md` §4), usado pelo reindex e pelo watcher, com teste
  - **Papel:** executor
  - **agent:** executor3
  - **Dependências:** T006
  - **depends_on:** [T006]
  - **Paralelizável:** true
  - **Arquivos:** `internal/wiki/ledger_detect.go`, `internal/wiki/ledger_detect_test.go`, `internal/wiki/reindex.go`, `internal/wiki/watcher.go`
  - **Wiki-Keywords:** wiki-files-layer, ledger de eventos, ignorados


### 2C Ingestão: núcleo puro

- [x] **T023:** Criar a normalização tolerante do evento (agente, sessão, `cwd` do corpo, título e corpo por evento, esquema fechado do Claude Code com `tool_use_id` de 1..128 caracteres `[A-Za-z0-9_-]`, exigência de `tool_input`/`args` em `PreToolUse`, omissão de `---` e saída em `PostToolUse` quando `tool_family == "unknown"`, `reason: <primeira_linha>` em `SessionEnd`, `occurred_at`, `ingest_key`, subagente, remoção incondicional de `last_assistant_message`), com teste por fixtures
  - **Papel:** executor
  - **agent:** executor2
  - **Dependências:** T001, T008, T009
  - **depends_on:** [T001, T008, T009]
  - **Paralelizável:** true
  - **Arquivos:** `internal/ingest/normalize.go`, `internal/ingest/normalize_test.go`, `internal/ingest/testdata/claude-code/`
  - **Wiki-Keywords:** extração tolerante, occurred_at, esquema fechado, Claude Code

- [x] **T024:** Criar a política de captura pura (estados do marcador, padrões, limites, orçamento de comparações, normalização de caminhos Windows com prefixos de provedor PowerShell, `\\?\`, `\\.\`, `\??\`, `\\localhost\C$`, candidatos duplos MSYS2/Cygwin `/c/...` e `C:/...`, remoção de NTFS ADS `:stream` e pontos/espaços finais, remoção de escapes `` ` `` do PowerShell e `^` do `cmd.exe`, rejeição de padrão `/foo` sem drive em marcador Windows, regras de `output_reader`/`inline_code`/arquivo de spool do harness, decisões `Keep`/`Drop`/`MetadataOnly` e corpo substituto), com teste
  - **Papel:** executor
  - **agent:** executor1
  - **Dependências:** T005, T006
  - **depends_on:** [T005, T006]
  - **Paralelizável:** true
  - **Arquivos:** `internal/ingest/capture/policy.go`, `internal/ingest/capture/policy_test.go`
  - **Wiki-Keywords:** política de captura, denylist, MetadataOnly, fail-safe

- [x] **T025:** Criar a síntese da página de sessão sem LLM (título, resumo de uma linha incluindo `, ended (<reason>)` quando houver razão no `SessionEnd`, corpo e frontmatter), com teste
  - **Papel:** executor
  - **agent:** executor3
  - **Dependências:** T002, T008, T090
  - **depends_on:** [T002, T008, T090]
  - **Paralelizável:** true
  - **Arquivos:** `internal/ingest/synth/session_page.go`, `internal/ingest/synth/session_page_test.go`
  - **Wiki-Keywords:** página de sessão, zero-LLM, raw observations

- [x] **T026:** Criar a síntese do handoff automático sem LLM (resumo com desduplicação quando o último prompt já está citado nas perguntas em aberto — `"Single-prompt session; the prompt is quoted under open questions."` ou omitindo `"Last: ..."` —, próximos passos, perguntas em aberto e as 27 frases de confirmação com combinação composta), com teste
  - **Papel:** executor
  - **agent:** executor1
  - **Dependências:** T002, T008
  - **depends_on:** [T002, T008]
  - **Paralelizável:** true
  - **Arquivos:** `internal/ingest/synth/handoff.go`, `internal/ingest/synth/handoff_test.go`
  - **Wiki-Keywords:** handoff automático, próximos passos, perguntas em aberto

- [x] **T027:** Criar a resolução de projeto do hook (ordem do §7.1, raiz git real, recusa de `_global`, prefixo mais longo, criação de escopo) e a sessão pegajosa (`mid_session`), com cache de 4096 e teste
  - **Papel:** executor
  - **agent:** executor2
  - **Dependências:** T020, T010
  - **depends_on:** [T020, T010]
  - **Paralelizável:** true
  - **Arquivos:** `internal/ingest/resolve.go`, `internal/ingest/resolve_test.go`
  - **Wiki-Keywords:** resolução de projeto, sessão pegajosa, repo-root, _global


### 2D Ingestão: orquestração e rotas

- [ ] **T028:** Criar o processamento do evento: sessão, gate por (projeto, chave), observação sanitizada e chave na mesma transação, desfechos de admissão, avanço do ponteiro, linha no ledger e conclusão no fim; fim de sessão que nomeia outro escopo é descartado, salvo se vier do mesmo diretório da sessão (`docs/auto-scope.md`), com teste
  - **Papel:** executor
  - **agent:** executor1
  - **Dependências:** T014, T015, T019, T021, T023, T027, T093, T028.a, T028.b
  - **depends_on:** [T014, T015, T019, T021, T023, T027, T093, T028.a, T028.b]
  - **Paralelizável:** true
  - **Arquivos:** `internal/ingest/process.go`, `internal/ingest/process_test.go`
  - **Wiki-Keywords:** admissão, INV-03, idempotência, ponteiro ativo
  - **Revisão (2026-10-18):** as pendências (a) estado da chave e (b) cwd da sessão saíram para T028.a e T028.b. O item de outro escopo já está pronto (gravado no projeto nomeado, sessão não movida, `docs/storage-and-retrieval.md:209-210`). T028 só fecha quando T028.a e T028.b fecharem e o processo usar as novas funções.

- [x] **T028.a:** Expor o estado da chave de ingestão (reivindicada com efeitos a jusante pendentes ou concluída) em `storage/ingestkeys.go`, com teste (`docs/database-schema.md:283`, `docs/complementary-contracts.md:69`)
  - **Papel:** executor
  - **agent:** executor3
  - **Dependências:** T015
  - **depends_on:** [T015]
  - **Paralelizável:** true
  - **Arquivos:** `internal/storage/ingestkeys.go`, `internal/storage/ingestkeys_test.go`
  - **Wiki-Keywords:** ingest_keys, completed_at, replay pendente

- [x] **T028.b:** Gravar e ler o `cwd` de trabalho armazenado da sessão em `storage/sessions.go`, com teste, para a exceção do fim de sessão no mesmo diretório (`docs/storage-and-retrieval.md:211`, `docs/desktop/capture-and-dedup.md:108`)
  - **Papel:** executor
  - **agent:** executor3
  - **Dependências:** T014
  - **depends_on:** [T014]
  - **Paralelizável:** true
  - **Arquivos:** `internal/storage/sessions.go`, `internal/storage/sessions_test.go`
  - **Wiki-Keywords:** sessão, cwd armazenado, fim de sessão, escopo

- [ ] **T029:** Criar o fim de sessão: efêmera sem página (reabrindo o handoff mais recente aceito por aquela sessão efêmera via `ReopenLastAcceptedHandoffForSessionTx`), substantiva com página por `WritePage` e transação que encerra a sessão e grava o handoff, e commit com o título do doc, com teste
  - **Papel:** executor
  - **agent:** executor1
  - **Dependências:** T016, T025, T026, T028, T080, T089, T091
  - **depends_on:** [T016, T025, T026, T028, T080, T089, T091]
  - **Paralelizável:** false
  - **Arquivos:** `internal/ingest/session_end.go`, `internal/ingest/session_end_test.go`
  - **Wiki-Keywords:** fim de sessão, sessão efêmera, create_on_session_end

- [ ] **T030:** Criar os checkpoints de pré e pós-compactação (página sem handoff, mantida se o agente já escreveu, commit com rótulo), com teste
  - **Papel:** executor
  - **agent:** executor1
  - **Dependências:** T029
  - **depends_on:** [T029]
  - **Paralelizável:** false
  - **Arquivos:** `internal/ingest/checkpoint.go`, `internal/ingest/checkpoint_test.go`
  - **Wiki-Keywords:** checkpoint, pré-compactação, pós-compactação

- [ ] **T031:** Criar as rotas `/hook` e `/hook/batch` (semáforo de 1024, 202, 429, 413, descartes reconhecidos, reinspeção da política usando sentinela `"C:/__ai_memory_capture_server_inspection_never_match__"` no Windows e `"/__ai_memory_capture_server_inspection_never_match__"` no Unix, fail-fast e resultados por índice), com teste
  - **Papel:** executor
  - **agent:** executor2
  - **Dependências:** T024, T028
  - **depends_on:** [T024, T028]
  - **Paralelizável:** true
  - **Arquivos:** `internal/ingest/router.go`, `internal/ingest/router_test.go`
  - **Wiki-Keywords:** INV-05, 202 ou 429, lote, reconhecimento

- [x] **T032:** Criar a rota `GET /handoff` (ordem de operações, publicação do ponteiro, seleção, renderização com cabeçalhos e aviso de segurança exatos, oferta ou reivindicação depois de renderizar, delimitadores de histórico não confiável, 403), com teste
  - **Papel:** executor
  - **agent:** executor3
  - **Dependências:** T018, T027
  - **depends_on:** [T018, T027]
  - **Paralelizável:** true
  - **Arquivos:** `internal/ingest/handoff_route.go`, `internal/ingest/handoff_route_test.go`
  - **Wiki-Keywords:** claim_on_session_start, nota de oferta, untrusted-history


### 2E MCP

- [x] **T033:** Criar a interface do servidor MCP e o adaptador `mcp-go` (nome, capacidade `tools`, protocolo 2024-11-05, identidade só do contexto, chave do ator), com teste
  - **Papel:** executor
  - **agent:** executor2
  - **Dependências:** T007, T003
  - **depends_on:** [T007, T003]
  - **Paralelizável:** true
  - **Arquivos:** `internal/mcp/server.go`, `internal/mcp/adapter_mcpgo.go`, `internal/mcp/server_test.go`, `specs/features/002-captura-e-acesso/notes/adr-protocolo-mcp.md` (ADR do pino de protocolo por wrapper, P1 da fase 0)
  - **Wiki-Keywords:** mcp-go, interface, chave do ator, protocolo

- [x] **T034:** Criar as tools de leitura `memory_query` (`global`, `answer` retornando `answer_unavailable` sem provedor, `pin_first`, `raw_hits`, `global_scope_hits`, `explain`), `memory_recent`, `memory_read_page` (`related` e `related_depth` 1..3, `served_from: "db-fallback"` quando o `.md` falta no disco) e `memory_status`, com teste
  - **Papel:** executor
  - **agent:** executor2
  - **Dependências:** T033, T020
  - **depends_on:** [T033, T020]
  - **Paralelizável:** true
  - **Arquivos:** `internal/mcp/tools_read.go`, `internal/mcp/tools_read_test.go`
  - **Wiki-Keywords:** memory_query, global_scope_hits, raw_hits, related

- [x] **T035:** Criar as tools `memory_read_session_observations` (`session_id` obrigatório, `limit` 50..200, `offset`, `body_max_chars` 200..16384 com sufixo de truncamento, `order`, `kinds`, `query`, `elided_other_scope`) e `memory_briefing` (`settled_first`, `pinned` excluindo `_slots/*`, `settled`, `counts`, `activity`, `rules`, `slots`, `recent_pages`), com teste
  - **Papel:** executor
  - **agent:** executor3
  - **Dependências:** T033, T014
  - **depends_on:** [T033, T014]
  - **Paralelizável:** true
  - **Arquivos:** `internal/mcp/tools_session.go`, `internal/mcp/tools_session_test.go`
  - **Wiki-Keywords:** observações brutas, briefing, paginação

- [x] **T036:** Criar as tools de escrita `memory_write_page` (`tier` padrão `episodic`, `scope: "global"`, `expected_page_id`, `create_only`, campos OKF, `pre-checkpoint before mcp write-page <path>` e `mcp write-page <path>`) e `memory_delete_page` (exclusão física de todas as versões, limpeza de entidades órfãs, `audit_log`, `expected_page_id` com `-32600` em divergência, idempotente retornando sempre `deleted: true` quando `expected_page_id` omitido, rejeitando `scope: "global"` com `-32602`), com teste
  - **Papel:** executor
  - **agent:** executor1
  - **Dependências:** T033, T020, T087, T089, T091
  - **depends_on:** [T033, T020, T087, T089, T091]
  - **Paralelizável:** true
  - **Arquivos:** `internal/mcp/tools_write.go`, `internal/mcp/tools_write_test.go`
  - **Wiki-Keywords:** memory_write_page, escopo global, checkpoint

- [x] **T037:** Criar as tools `memory_handoff_begin` (truncamento pós-sanitização, `from_agent` padrão `"mcp"`, `shared`), `memory_handoff_accept` (`{handoff, status: "claimed" | "consumed_by_hook" | "none_pending", scope, warnings}`, sem filtro de sessão viva quando `handoff_id` informado e com varredura pós-reivindicação quando automático), `memory_handoff_list` (`state`, `limit` 20..100, `all_owners`, ordem `created_at DESC`) e `memory_handoff_cancel` (`all_owners`, `{handoff_id, cancelled, scope, warnings}`), com teste
  - **Papel:** executor
  - **agent:** executor3
  - **Dependências:** T033, T018
  - **depends_on:** [T033, T018]
  - **Paralelizável:** true
  - **Arquivos:** `internal/mcp/tools_handoff.go`, `internal/mcp/tools_handoff_test.go`
  - **Wiki-Keywords:** memory_handoff_accept, shared, limites de handoff

- [x] **T038:** Criar os transportes MCP: stdio e HTTP streamable sem estado com resposta JSON, com teste
  - **Papel:** executor
  - **agent:** executor2
  - **Dependências:** T033
  - **depends_on:** [T033]
  - **Paralelizável:** true
  - **Arquivos:** `internal/mcp/transport.go`, `internal/mcp/transport_test.go`
  - **Wiki-Keywords:** stdio, streamable HTTP, sem estado


### 2F CLI e servidor

- [ ] **T039:** Criar o servidor HTTP: filtro de `Host` na camada mais externa (cobrindo inclusive `/healthz`: 400 sem `Host`, 403 para `Host` não permitido), limite de corpo de 10 MiB, autenticação bearer em tempo constante (comparando hashes SHA-256 dos tokens), 401 com `WWW-Authenticate: Bearer` e corpo JSON `{"error": "unauthorized: missing or invalid Authorization: Bearer <token> header"}`, regras de exposição sem autenticação humana (`docs/https-via-proxy.md` § Regras de inicialização, regras 1, 4, 5 e 6: loopback aceito; `--allow-insecure-no-auth` aceito com banner no stderr e alerta no log; contêiner por `AI_MEMORY_IN_CONTAINER` ou marcador do runtime aceito com aviso; fora disso recusa com orientação; regras 2 e 3 na 007/T028), `/healthz`, `/admin/status` e janela de encerramento de 5 s, com teste
  - **Papel:** executor
  - **agent:** executor1
  - **Dependências:** T010, T031, T032, T038, T081
  - **depends_on:** [T010, T031, T032, T038, T081]
  - **Paralelizável:** true
  - **Arquivos:** `internal/cli/httpserver.go`, `internal/cli/httpserver_test.go`
  - **Wiki-Keywords:** autenticação HTTP, allowed_hosts, bearer, rotas de máquina

- [x] **T040:** Criar a trava de instância única: trava exclusiva do sistema operacional sobre `.serve.lock` (`LockFileEx` no Windows / `flock` no Unix), segura durante a vida do processo e liberada pelo sistema ao sair (servidor que caiu nunca trava ninguém); PID do detentor em `.serve.lock.holder` (informativo, melhor esforço) citado na recusa; falha transitória ao abrir ou travar repetida com espera de 25 ms até 5 tentativas no total (trava ocupada nunca é repetida); sistema de arquivos sem suporte a trava só gera aviso; `--force` sobe mesmo assim (`docs/windows.md` § Single instance, `docs/cli.md` §3.1), com teste
  - **Papel:** executor
  - **agent:** executor3
  - **Dependências:** T006
  - **depends_on:** [T006]
  - **Paralelizável:** true
  - **Arquivos:** `internal/cli/servelock.go`, `internal/cli/servelock_test.go`
  - **Wiki-Keywords:** .serve.lock, instância única, cli.md boot

- [x] **T041:** Trocar a guarda de processo vivo pela enumeração de processos do sistema (Linux por `/proc`, Windows por `CreateToolhelp32Snapshot` comparando `ai-memory` e `ai-memory.exe` sem diferenciar maiúsculas e respeitando `AI_MEMORY_TEST_NO_PROCESS_GUARD` e `AI_MEMORY_TEST_FORCE_SIBLING_PIDS`), removendo o `ai-memory.pid`, e ajustar os testes da guarda e do `reindex`
  - **Papel:** executor
  - **agent:** executor2
  - **Dependências:** T006
  - **depends_on:** [T006]
  - **Paralelizável:** true
  - **Arquivos:** `internal/cli/liveguard.go`, `internal/cli/liveguard_other.go`, `internal/cli/liveguard_windows.go`, `internal/cli/liveguard_test.go`, `internal/cli/reindex_cmd_test.go`
  - **Wiki-Keywords:** INV-09, processo vivo, enumeração de processos

- [ ] **T042:** Criar o comando `serve` (ordem de boot, trava, abertura do armazenamento, escopo padrão, wiki, watcher, ponteiro, MCP e transporte `stdio` ou `http`), com teste
  - **Papel:** executor
  - **agent:** executor1
  - **Dependências:** T039, T040, T029, T030, T034, T035, T036, T037, T085, T092
  - **depends_on:** [T039, T040, T029, T030, T034, T035, T036, T037, T085, T092]
  - **Paralelizável:** false
  - **Arquivos:** `internal/cli/serve_cmd.go`, `internal/cli/serve_cmd_test.go`
  - **Wiki-Keywords:** serve, ordem de boot, transporte

- [x] **T043:** Criar a leitura do marcador `.ai-memory.toml` no cliente (caminhada com fronteiras, caminhada de escopo e de captura, leitura por linha, valores verdadeiros e falsos, `[capture]` estrito até 64 KiB), com teste
  - **Papel:** executor
  - **agent:** executor3
  - **Dependências:** T005, T011
  - **depends_on:** [T005, T011]
  - **Paralelizável:** true
  - **Arquivos:** `internal/cli/marker.go`, `internal/cli/marker_test.go`
  - **Wiki-Keywords:** marker-file, caminhada, leitura por linha

- [x] **T044:** Criar o spool local (itens `0600` com nome `{created_ms:013}-{pid}-{seq:016x}-{recovery_key}.json`, recuperação de `.json.tmp` órfãos com > 5 min no início da drenagem, 8 tentativas, 7 dias, limite de 10 000 arquivos sob `<data_dir>/hook-spool.capacity.lock` preservando o backlog existente e descartando o novo evento recebido, lotes de 256 itens e 8 MiB, e trava exclusiva do sistema operacional sobre `<data_dir>/hook-spool/.drain.lock` via `LockFileEx` no Windows / `flock` no Unix conferindo antes se `<data_dir>` existe), com teste
  - **Papel:** executor
  - **agent:** executor2
  - **Dependências:** T011, T041
  - **depends_on:** [T011, T041]
  - **Paralelizável:** true
  - **Arquivos:** `internal/cli/spool.go`, `internal/cli/spool_test.go`
  - **Wiki-Keywords:** spool, hook-spool, trava de arquivo

- [x] **T044.a:** Gravar `auth_mode` (`static` ou `none`) e o token em texto quando `static` em cada item do spool: campo em `spoolItem` e parâmetro em `Enqueue`, com teste de permissão `0600` e de leitura dos dois modos (`docs/hook-scripts-and-container-deploy.md:159`). Modo OIDC fica fora desta task
  - **Papel:** executor
  - **agent:** executor2
  - **Dependências:** T044
  - **depends_on:** [T044]
  - **Paralelizável:** false
  - **Arquivos:** `internal/cli/spool.go`, `internal/cli/spool_test.go`
  - **Wiki-Keywords:** spool, auth_mode, token estático, permissões

- [ ] **T045:** Criar a drenagem do spool por `POST /hook/batch` (confirmação por índice, um retry em 401, 429 sem gastar tentativa, reapontamento para o `server_url` atual), com teste
  - **Papel:** executor
  - **agent:** executor2
  - **Dependências:** T044.a
  - **depends_on:** [T044.a]
  - **Paralelizável:** false
  - **Arquivos:** `internal/cli/drain.go`, `internal/cli/drain_test.go`
  - **Wiki-Keywords:** drenagem, reconhecimento de lote, cliente HTTP
  - **Decisões registradas (2026-10-18):** o retry do 401 usa o token atual do config e não o token guardado no item, porque o doc diz "cobre token rotacionado após a captura" (`docs/cli.md:339-340`). Timeout do POST do lote: 2 s, conforme INV-05 da constituição, marcado "a confirmar" no doc (`docs/cli.md:363`). Modo OIDC fica fora desta task.

- [ ] **T046:** Criar o comando `hook` (parse com BOM até 1 MiB, remoção do campo do assistente, política, spool, drenagem incremental, início de sessão com `GET /handoff` e resposta `hookSpecificOutput.additionalContext`, fronteiras com `hook-drain` destacado usando `DETACHED_PROCESS | CREATE_NEW_PROCESS_GROUP | CREATE_NO_WINDOW | CREATE_BREAKAWAY_FROM_JOB` e fallback sem `CREATE_BREAKAWAY_FROM_JOB` em `ERROR_ACCESS_DENIED` no Windows e passando `AI_MEMORY_AUTH_TOKEN`, `--check-capture` com os 15 campos fixos e `disposition: "drop"` para `MetadataOnly`, sempre sucesso), com teste
  - **Papel:** executor
  - **agent:** executor2
  - **Dependências:** T024, T043, T044.a, T045
  - **depends_on:** [T024, T043, T044.a, T045]
  - **Paralelizável:** false
  - **Arquivos:** `internal/cli/hook_cmd.go`, `internal/cli/hook_cmd_test.go`
  - **Wiki-Keywords:** cliente de hooks, caminho rápido, nunca bloquear

- [ ] **T047:** Criar o comando oculto `hook-drain` (trava exclusiva do spool `<data_dir>/hook-spool/.drain.lock`, orçamento de 5 min), com teste
  - **Papel:** executor
  - **agent:** executor3
  - **Dependências:** T045
  - **depends_on:** [T045]
  - **Paralelizável:** true
  - **Arquivos:** `internal/cli/hookdrain_cmd.go`, `internal/cli/hookdrain_cmd_test.go`
  - **Wiki-Keywords:** hook-drain, fronteira, orçamento

- [x] **T048:** Criar a escrita idempotente de arquivo de agente (resolve links simbólicos, não toca o disco se igual, `.bak-<unix>` e escrita atômica), com teste
  - **Papel:** executor
  - **agent:** executor1
  - **Dependências:** T006
  - **depends_on:** [T006]
  - **Paralelizável:** true
  - **Arquivos:** `internal/cli/applyatomic.go`, `internal/cli/applyatomic_test.go`
  - **Wiki-Keywords:** apply_atomic, idempotente, backup do arquivo

- [x] **T049:** Criar o comando `install-hooks --agent claude-code` (imprime ou mescla em `~/.claude/settings.json` os 9 eventos `SessionStart`, `UserPromptSubmit`, `PreToolUse`, `PostToolUse`, `PreCompact`, `Stop`, `SessionEnd`, `SubagentStart` e `SubagentStop` — usando no Windows nativo o formato exec-form `{"type":"command","command":"<exe>","args":["--data-dir","<data_dir>","hook","--event","<ev>","--agent","claude-code","--server-url","<url>"]}` e `"timeout": 15` em `SessionEnd` —, preserva entradas de terceiros; `--capture-mode denylist|allowlist` gravado e reaproveitado num `--apply` posterior; `--no-capture-prompts`/`--capture-prompts`; `--project-strategy basename|repo-root` gravado no comando do hook, preservado num `--apply` posterior que o omite e removido com `basename` explícito, com o marcador vencendo o padrão da instalação e `AI_MEMORY_PROJECT_STRATEGY` valendo só com marcador (`docs/marker-file.md` § install-wide default); `docs/support-matrix.md`; com `--apply` e `--auth-token`, guarda dois arquivos só do dono no diretório de dados (`hook-auth-token` e `hook-auth.curl`), e o token nunca vai para a linha de comando; `docs/users.md` § "Hooks and agent wiring"), com teste
  - **Papel:** executor
  - **agent:** executor1
  - **Dependências:** T048, T010
  - **depends_on:** [T048, T010]
  - **Paralelizável:** true
  - **Arquivos:** `internal/cli/install_hooks.go`, `internal/cli/install_hooks_test.go`
  - **Wiki-Keywords:** install-hooks, settings.json, Claude Code

- [x] **T050:** Criar o comando `install-mcp --client claude-code` (imprime ou mescla `mcpServers.ai-memory` em `~/.claude.json` ou `$CLAUDE_CONFIG_DIR/.claude.json`, cabeçalho só com token), com teste
  - **Papel:** executor
  - **agent:** executor3
  - **Dependências:** T048, T010
  - **depends_on:** [T048, T010]
  - **Paralelizável:** true
  - **Arquivos:** `internal/cli/install_mcp.go`, `internal/cli/install_mcp_test.go`
  - **Wiki-Keywords:** install-mcp, .claude.json, mcpServers

- [ ] **T051:** Criar o comando `status` (`GET /admin/status` e saúde do spool sem conteúdo), com teste
  - **Papel:** executor
  - **agent:** executor3
  - **Dependências:** T039, T044
  - **depends_on:** [T039, T044]
  - **Paralelizável:** true
  - **Arquivos:** `internal/cli/status_cmd.go`, `internal/cli/status_cmd_test.go`
  - **Wiki-Keywords:** status, saúde do spool, admin status

- [ ] **T052:** Registrar na raiz da CLI os comandos `serve`, `hook`, `hook-drain` (oculto), `install-hooks`, `install-mcp` e `status`, com o caminho rápido do hook antes da configuração completa
  - **Papel:** executor
  - **agent:** executor1
  - **Dependências:** T042, T046, T047, T049, T050, T051
  - **depends_on:** [T042, T046, T047, T049, T050, T051]
  - **Paralelizável:** false
  - **Arquivos:** `internal/cli/root.go`
  - **Wiki-Keywords:** cobra, raiz da CLI, caminho rápido


### 2G Cenários Gherkin

- [ ] **T053:** Implementar Scenario: Fluxo ponta a ponta (hook do Claude Code → observação → `sessions/<id>.md` → handoff → `memory_query`)
  - **Papel:** executor
  - **agent:** executor1
  - **Dependências:** T052, T013
  - **depends_on:** [T052, T013]
  - **Paralelizável:** true
  - **Arquivos:** `test/acceptance/capture_flow_test.go`
  - **Wiki-Keywords:** ponta a ponta, handoff, memory_query

- [ ] **T054:** Implementar tratamento de erro: Servidor saturado (429 com o evento mantido no spool; lote com mais de 256 itens dá 413)
  - **Papel:** executor
  - **agent:** executor2
  - **Dependências:** T031, T046, T013
  - **depends_on:** [T031, T046, T013]
  - **Paralelizável:** true
  - **Arquivos:** `test/acceptance/hook_saturation_test.go`
  - **Wiki-Keywords:** 429, 413, spool

- [ ] **T055:** Implementar tratamento de erro: Leitura com escopo explícito inexistente falha fechado (e leitura sem escopo degrada com origem)
  - **Papel:** executor
  - **agent:** executor3
  - **Dependências:** T034, T013
  - **depends_on:** [T034, T013]
  - **Paralelizável:** true
  - **Arquivos:** `test/acceptance/mcp_scope_read_test.go`
  - **Wiki-Keywords:** falha fechada, ScopeSource, leitura sem escopo

- [ ] **T056:** Implementar tratamento de erro: Escrita sem escopo com ponteiro ativo divergente (`Mismatch`) é recusada como escrita ambígua (`-32602`), enquanto com ponteiro nunca publicado (`Unset`) recai para o escopo padrão
  - **Papel:** executor
  - **agent:** executor1
  - **Dependências:** T036, T013
  - **depends_on:** [T036, T013]
  - **Paralelizável:** true
  - **Arquivos:** `test/acceptance/mcp_ambiguous_write_test.go`
  - **Wiki-Keywords:** escrita ambígua, ponteiro, padrão

- [ ] **T057:** Implementar tratamento de erro: Falha no banco depois de escrever o arquivo faz `memory_write_page` reverter o arquivo
  - **Papel:** executor
  - **agent:** executor2
  - **Dependências:** T036, T013
  - **depends_on:** [T036, T013]
  - **Paralelizável:** true
  - **Arquivos:** `test/acceptance/mcp_write_rollback_test.go`
  - **Wiki-Keywords:** rollback, ErrInconsistentState, memory_write_page

- [ ] **T058:** Implementar tratamento de erro: Servidor offline (o evento fica no spool e é drenado com trava quando o servidor volta)
  - **Papel:** executor
  - **agent:** executor3
  - **Dependências:** T046, T047, T013
  - **depends_on:** [T046, T047, T013]
  - **Paralelizável:** true
  - **Arquivos:** `test/acceptance/spool_offline_test.go`
  - **Wiki-Keywords:** servidor offline, drenagem, trava

- [ ] **T059:** Implementar edge case: Replay do mesmo evento, inclusive após interrupção no meio do fim de sessão, não duplica observação e converge os efeitos
  - **Papel:** executor
  - **agent:** executor1
  - **Dependências:** T029, T031, T013
  - **depends_on:** [T029, T031, T013]
  - **Paralelizável:** true
  - **Arquivos:** `test/acceptance/replay_test.go`
  - **Wiki-Keywords:** replay, ingest_key, retomada

- [ ] **T060:** Implementar edge case: Handoff disputado por dois aceites tem um único vencedor e o perdedor recebe `handoff: null` (multi-sessão, `-race`)
  - **Papel:** executor
  - **agent:** executor2
  - **Dependências:** T032, T037, T013
  - **depends_on:** [T032, T037, T013]
  - **Paralelizável:** true
  - **Arquivos:** `test/acceptance/handoff_race_test.go`
  - **Wiki-Keywords:** INV-16, corrida, uso único

- [ ] **T061:** Implementar edge case: Escrita concorrente na mesma página pelo MCP vira cadeia de supersessão sem perder a versão perdedora
  - **Papel:** executor
  - **agent:** executor3
  - **Dependências:** T036, T013
  - **depends_on:** [T036, T013]
  - **Paralelizável:** true
  - **Arquivos:** `test/acceptance/mcp_concurrent_write_test.go`
  - **Wiki-Keywords:** supersessão, INV-16, multi-sessão

- [ ] **T062:** Implementar edge case: Segredo no payload do hook é removido antes de gravar, e o corpo respeita o limite por tipo de evento
  - **Papel:** executor
  - **agent:** executor1
  - **Dependências:** T031, T013
  - **depends_on:** [T031, T013]
  - **Paralelizável:** true
  - **Arquivos:** `test/acceptance/hook_secret_test.go`
  - **Wiki-Keywords:** sanitização, INV-06, limites de corpo

- [ ] **T063:** Implementar edge case: O campo bruto `last_assistant_message` nunca chega a log, spool, rede ou armazenamento
  - **Papel:** executor
  - **agent:** executor2
  - **Dependências:** T031, T046, T013
  - **depends_on:** [T031, T046, T013]
  - **Paralelizável:** true
  - **Arquivos:** `test/acceptance/assistant_field_test.go`
  - **Wiki-Keywords:** last_assistant_message, remoção incondicional, privacidade

- [ ] **T064:** Implementar edge case: Dois agentes no mesmo projeto têm ponteiros de projeto ativo separados, por ator
  - **Papel:** executor
  - **agent:** executor3
  - **Dependências:** T028, T034, T013
  - **depends_on:** [T028, T034, T013]
  - **Paralelizável:** true
  - **Arquivos:** `test/acceptance/active_pointer_test.go`
  - **Wiki-Keywords:** per_actor, ponteiro ativo, isolamento

- [ ] **T065:** Implementar edge cases de handoff do doc: modo oferta, sessão de origem viva nos últimos 10 min e `create_on_session_end = false`
  - **Papel:** executor
  - **agent:** executor1
  - **Dependências:** T032, T029, T013
  - **depends_on:** [T032, T029, T013]
  - **Paralelizável:** true
  - **Arquivos:** `test/acceptance/handoff_modes_test.go`
  - **Wiki-Keywords:** modo oferta, sessão viva, create_on_session_end


## Fase 3: Validação

- [ ] **T066:** Criar testes de contrato INV-05 (1025 requisições em voo dão 429 sem bloquear, lote de 257 dá 413, descarte reconhecido não gasta capacidade)
  - **Papel:** executor
  - **agent:** executor2
  - **Dependências:** T031
  - **depends_on:** [T031]
  - **Paralelizável:** true
  - **Arquivos:** `test/contract/ingest_inv_test.go`
  - **Wiki-Keywords:** INV-05, saturação, carga

- [ ] **T067:** Criar testes de contrato INV-16 da 002 (handoff exatamente uma vez com as duas guardas, ponteiro por ator)
  - **Papel:** executor
  - **agent:** executor3
  - **Dependências:** T018, T019
  - **depends_on:** [T018, T019]
  - **Paralelizável:** true
  - **Arquivos:** `test/contract/handoff_inv_test.go`
  - **Wiki-Keywords:** INV-16, duas guardas, multi-sessão

- [ ] **T068:** Atualizar o teste de contrato INV-09 para a enumeração de processos e a trava `.serve.lock`
  - **Papel:** executor
  - **agent:** executor1
  - **Dependências:** T040, T041
  - **depends_on:** [T040, T041]
  - **Paralelizável:** true
  - **Arquivos:** `test/contract/runtime_inv_test.go`
  - **Wiki-Keywords:** INV-09, processo vivo, contrato

- [ ] **T069:** Criar o teste de latência do comando `hook` sem rede (< 200 ms)
  - **Papel:** executor
  - **agent:** executor2
  - **Dependências:** T052
  - **depends_on:** [T052]
  - **Paralelizável:** true
  - **Arquivos:** `test/bench/hook_latency_test.go`
  - **Wiki-Keywords:** latência, caminho rápido, hook

- [ ] **T070:** Criar o smoke test fim a fim com o binário (`serve --transport http`, `hook` nos eventos do Claude Code, `GET /handoff`, `install-hooks` e `install-mcp` sem `--apply`)
  - **Papel:** executor
  - **agent:** executor3
  - **Dependências:** T052
  - **depends_on:** [T052]
  - **Paralelizável:** true
  - **Arquivos:** `test/smoke/capture_smoke_test.go`
  - **Wiki-Keywords:** smoke test, binário, serve

- [ ] **T071:** Rodar `go mod tidy` e conferir as dependências contra o `tech.md` (única task que mexe em `go.mod` e `go.sum` depois da T007)
  - **Papel:** executor
  - **agent:** executor1
  - **Dependências:** T066, T067, T068, T069, T070, T012, T022, T082, T083, T084, T086, T088
  - **depends_on:** [T066, T067, T068, T069, T070, T012, T022, T082, T083, T084, T086, T088]
  - **Paralelizável:** false
  - **Arquivos:** `go.mod`, `go.sum`
  - **Wiki-Keywords:** go mod tidy, tech.md, dependências

- [ ] **T072:** Rodar os portões (`scripts/check.sh` com `-race` e `golangci-lint` v2.14.0) e registrar o resultado com a saída
  - **Papel:** executor
  - **agent:** executor2
  - **Dependências:** T071, T053, T054, T055, T056, T057, T058, T059, T060, T061, T062, T063, T064, T065
  - **depends_on:** [T071, T053, T054, T055, T056, T057, T058, T059, T060, T061, T062, T063, T064, T065]
  - **Paralelizável:** false
  - **Arquivos:** `specs/features/002-captura-e-acesso/notes/gates-run.md`
  - **Wiki-Keywords:** check.sh, race, golangci-lint

- [ ] **T073:** Auditar a feature contra a constituição (checklist do `plan.md` seção 4, item a item, com evidência) e contra os docs
  - **Papel:** analyst
  - **agent:** analyst1
  - **Dependências:** T072
  - **depends_on:** [T072]
  - **Paralelizável:** false
  - **Arquivos:** `specs/features/002-captura-e-acesso/notes/constitution-audit.md`
  - **Wiki-Keywords:** constitution, auditoria, INV


## Fase 4: Documentação

- [ ] **T074:** Atualizar o README.md (servidor, hooks do Claude Code, `install-hooks`, `install-mcp`, MCP, spool, `status` e limitações da 002)
  - **Papel:** executor
  - **agent:** executor1
  - **Dependências:** T073
  - **depends_on:** [T073]
  - **Paralelizável:** true
  - **Arquivos:** `README.md`
  - **Wiki-Keywords:** README, serve, hooks

- [ ] **T075:** Atualizar o .env.example com as chaves novas da configuração (sem segredos)
  - **Papel:** executor
  - **agent:** executor2
  - **Dependências:** T010, T011
  - **depends_on:** [T010, T011]
  - **Paralelizável:** true
  - **Arquivos:** `.env.example`
  - **Wiki-Keywords:** .env.example, INV-01, configuração

- [ ] **T076:** Atualizar o tech.md com o real da 002 (versão do `mcp-go`, comandos novos, stdlib usada para processos e trava)
  - **Papel:** executor
  - **agent:** executor3
  - **Dependências:** T071
  - **depends_on:** [T071]
  - **Paralelizável:** true
  - **Arquivos:** `.github/memory/tech.md`
  - **Wiki-Keywords:** tech.md, versões, mcp-go

- [ ] **T077:** Atualizar o llms.txt (pacotes `ingest` e `mcp`, comandos novos e onde achar os contratos)
  - **Papel:** executor
  - **agent:** executor1
  - **Dependências:** T074
  - **depends_on:** [T074]
  - **Paralelizável:** true
  - **Arquivos:** `llms.txt`
  - **Wiki-Keywords:** llms.txt, onboarding

- [ ] **T078:** Criar a matriz invariante para teste da 002 (INV-05, INV-09 e INV-16 com arquivo e nome do teste)
  - **Papel:** executor
  - **agent:** executor2
  - **Dependências:** T066, T067, T068
  - **depends_on:** [T066, T067, T068]
  - **Paralelizável:** true
  - **Arquivos:** `specs/features/002-captura-e-acesso/notes/inv-test-matrix.md`
  - **Wiki-Keywords:** INV, matriz, testes de contrato




## Coordination Graph

G₀ (round 0):

- **nodes:** T001, T002, T003, T004, T005, T006, T007, T008, T009, T010, T011, T012, T013, T079, T080, T081, T082, T083, T084, T085, T086, T087, T088, T089, T090, T091, T092, T093, T014, T015, T016, T017, T018, T019, T020, T021, T022, T023, T024, T025, T026, T027, T028, T029, T030, T031, T032, T033, T034, T035, T036, T037, T038, T039, T040, T041, T042, T043, T044, T045, T046, T047, T048, T049, T050, T051, T052, T053, T054, T055, T056, T057, T058, T059, T060, T061, T062, T063, T064, T065, T066, T067, T068, T069, T070, T071, T072, T073, T074, T075, T076, T077, T078
- **edges:**
  - T001 → T006
  - T002 → T006
  - T003 → T006
  - T004 → T006
  - T005 → T006
  - T004 → T007
  - T006 → T007
  - T001 → T008
  - T006 → T008
  - T006 → T009
  - T006 → T010
  - T079 → T010
  - T010 → T011
  - T006 → T012
  - T006 → T013
  - T006 → T079
  - T006 → T080
  - T006 → T081
  - T081 → T082
  - T010 → T082
  - T022 → T083
  - T041 → T084
  - T079 → T084
  - T006 → T085
  - T022 → T086
  - T080 → T086
  - T006 → T087
  - T006 → T088
  - T080 → T089
  - T083 → T089
  - T086 → T089
  - T087 → T089
  - T006 → T090
  - T089 → T091
  - T090 → T091
  - T019 → T092
  - T020 → T092
  - T010 → T093
  - T079 → T093
  - T008 → T014
  - T006 → T015
  - T014 → T016
  - T002 → T016
  - T016 → T017
  - T017 → T018
  - T006 → T019
  - T019 → T020
  - T008 → T021
  - T006 → T022
  - T001 → T023
  - T008 → T023
  - T009 → T023
  - T005 → T024
  - T006 → T024
  - T002 → T025
  - T008 → T025
  - T090 → T025
  - T002 → T026
  - T008 → T026
  - T020 → T027
  - T010 → T027
  - T014 → T028
  - T015 → T028
  - T019 → T028
  - T021 → T028
  - T023 → T028
  - T027 → T028
  - T093 → T028
  - T016 → T029
  - T025 → T029
  - T026 → T029
  - T028 → T029
  - T080 → T029
  - T089 → T029
  - T091 → T029
  - T029 → T030
  - T024 → T031
  - T028 → T031
  - T018 → T032
  - T027 → T032
  - T007 → T033
  - T003 → T033
  - T033 → T034
  - T020 → T034
  - T033 → T035
  - T014 → T035
  - T033 → T036
  - T020 → T036
  - T087 → T036
  - T089 → T036
  - T091 → T036
  - T033 → T037
  - T018 → T037
  - T033 → T038
  - T010 → T039
  - T031 → T039
  - T032 → T039
  - T038 → T039
  - T081 → T039
  - T006 → T040
  - T006 → T041
  - T039 → T042
  - T040 → T042
  - T029 → T042
  - T030 → T042
  - T034 → T042
  - T035 → T042
  - T036 → T042
  - T037 → T042
  - T085 → T042
  - T092 → T042
  - T005 → T043
  - T011 → T043
  - T011 → T044
  - T041 → T044
  - T044 → T045
  - T024 → T046
  - T043 → T046
  - T045 → T046
  - T045 → T047
  - T006 → T048
  - T048 → T049
  - T010 → T049
  - T048 → T050
  - T010 → T050
  - T039 → T051
  - T044 → T051
  - T042 → T052
  - T046 → T052
  - T047 → T052
  - T049 → T052
  - T050 → T052
  - T051 → T052
  - T052 → T053
  - T013 → T053
  - T031 → T054
  - T046 → T054
  - T013 → T054
  - T034 → T055
  - T013 → T055
  - T036 → T056
  - T013 → T056
  - T036 → T057
  - T013 → T057
  - T046 → T058
  - T047 → T058
  - T013 → T058
  - T029 → T059
  - T031 → T059
  - T013 → T059
  - T032 → T060
  - T037 → T060
  - T013 → T060
  - T036 → T061
  - T013 → T061
  - T031 → T062
  - T013 → T062
  - T031 → T063
  - T046 → T063
  - T013 → T063
  - T028 → T064
  - T034 → T064
  - T013 → T064
  - T032 → T065
  - T029 → T065
  - T013 → T065
  - T031 → T066
  - T018 → T067
  - T019 → T067
  - T040 → T068
  - T041 → T068
  - T052 → T069
  - T052 → T070
  - T066 → T071
  - T067 → T071
  - T068 → T071
  - T069 → T071
  - T070 → T071
  - T012 → T071
  - T022 → T071
  - T082 → T071
  - T083 → T071
  - T084 → T071
  - T086 → T071
  - T088 → T071
  - T071 → T072
  - T053 → T072
  - T054 → T072
  - T055 → T072
  - T056 → T072
  - T057 → T072
  - T058 → T072
  - T059 → T072
  - T060 → T072
  - T061 → T072
  - T062 → T072
  - T063 → T072
  - T064 → T072
  - T065 → T072
  - T072 → T073
  - T073 → T074
  - T010 → T075
  - T011 → T075
  - T071 → T076
  - T074 → T077
  - T066 → T078
  - T067 → T078
  - T068 → T078
- **assignments:**
  - T001: researcher1
  - T002: researcher2
  - T003: researcher3
  - T004: researcher4
  - T005: researcher5
  - T006: analyst1
  - T007: executor1
  - T008: executor2
  - T009: executor3
  - T010: executor1
  - T011: executor1
  - T012: executor2
  - T013: executor3
  - T079: executor1
  - T080: executor2
  - T081: executor3
  - T082: executor3
  - T083: executor1
  - T084: executor2
  - T085: executor3
  - T086: executor2
  - T087: executor1
  - T088: executor2
  - T089: executor3
  - T090: executor1
  - T091: executor1
  - T092: executor2
  - T093: executor3
  - T014: executor1
  - T015: executor2
  - T016: executor3
  - T017: executor3
  - T018: executor3
  - T019: executor1
  - T020: executor1
  - T021: executor2
  - T022: executor3
  - T023: executor2
  - T024: executor1
  - T025: executor3
  - T026: executor1
  - T027: executor2
  - T028: executor1
  - T029: executor1
  - T030: executor1
  - T031: executor2
  - T032: executor3
  - T033: executor2
  - T034: executor2
  - T035: executor3
  - T036: executor1
  - T037: executor3
  - T038: executor2
  - T039: executor1
  - T040: executor3
  - T041: executor2
  - T042: executor1
  - T043: executor3
  - T044: executor2
  - T045: executor2
  - T046: executor2
  - T047: executor3
  - T048: executor1
  - T049: executor1
  - T050: executor3
  - T051: executor3
  - T052: executor1
  - T053: executor1
  - T054: executor2
  - T055: executor3
  - T056: executor1
  - T057: executor2
  - T058: executor3
  - T059: executor1
  - T060: executor2
  - T061: executor3
  - T062: executor1
  - T063: executor2
  - T064: executor3
  - T065: executor1
  - T066: executor2
  - T067: executor3
  - T068: executor1
  - T069: executor2
  - T070: executor3
  - T071: executor1
  - T072: executor2
  - T073: analyst1
  - T074: executor1
  - T075: executor2
  - T076: executor3
  - T077: executor1
  - T078: executor2
- **ready:** [T001, T002, T003, T004, T005]

> Workers: Lead, researcher1..5, analyst1, executor1..3. Validado por script: 93 tasks, sem ciclo, sem dependência quebrada, sem arquivo compartilhado entre tasks sem ordem. Mínimo de 17 rounds. Caminho crítico (calculado): T001 → T006 → T008 → T014 → T016 → T017 → T018 → T032 → T039 → T042 → T052 → T069 → T071 → T072 → T073 → T074 → T077.
