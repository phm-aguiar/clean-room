---
graph-operators: enabled
heartbeat-threshold: 4
max-rounds: 40
---

# tasks.md: Núcleo (domínio, armazenamento e wiki)

> Feature `001-nucleo` · Spec: `spec.md` (Aprovado: true) · Plano: `plan.md` · Discovery: `reports/001-ai-memory-go-clean-room-discovery.md`
>
> Regras: uma ação por task; implementação e seu `_test.go` contam como um grupo de arquivos relacionado. `go.mod`/`go.sum` só são editados em T008 e T070. `Wiki-Keywords` apontam para termos e docs de `docs/` (o `wiki-claude/` do SDD não existe neste repo).


## Fase 0: Descoberta

- [x] **T001:** Extrair o contrato do domínio: tipos de página, tiers, kinds de observação, regras de PagePath e chaves portáveis
  - **Papel:** researcher
  - **agent:** researcher1
  - **Dependências:** Nenhuma
  - **depends_on:** []
  - **Paralelizável:** true
  - **Arquivos:** `specs/features/001-nucleo/notes/domain-contract.md`
  - **Wiki-Keywords:** domain-vocabulary, page path, tier, portable key

- [x] **T002:** Extrair o contrato de fixtures e dos testes (multi-sessão, golden files)
  - **Papel:** researcher
  - **agent:** researcher2
  - **Dependências:** Nenhuma
  - **depends_on:** []
  - **Paralelizável:** true
  - **Arquivos:** `specs/features/001-nucleo/notes/fixtures-contract.md`
  - **Wiki-Keywords:** test-fixtures, fixtures, multi-sessão, golden

- [x] **T003:** Levantar padrões de segredo e limites de sanitização
  - **Papel:** researcher
  - **agent:** researcher3
  - **Dependências:** Nenhuma
  - **depends_on:** []
  - **Paralelizável:** true
  - **Arquivos:** `specs/features/001-nucleo/notes/sanitizer-patterns.md`
  - **Wiki-Keywords:** sanitizer, secrets, threat model, redaction

- [x] **T004:** Extrair o contrato de frontmatter, OKF, título e links
  - **Papel:** researcher
  - **agent:** researcher4
  - **Dependências:** Nenhuma
  - **depends_on:** []
  - **Paralelizável:** true
  - **Arquivos:** `specs/features/001-nucleo/notes/markdown-contract.md`
  - **Wiki-Keywords:** frontmatter, okf, typed edges, links

- [x] **T005:** Extrair o layout em disco, _meta.md, index.md, log.md e o histórico git
  - **Papel:** researcher
  - **agent:** researcher5
  - **Dependências:** Nenhuma
  - **depends_on:** []
  - **Paralelizável:** true
  - **Arquivos:** `specs/features/001-nucleo/notes/wiki-layout-contract.md`
  - **Wiki-Keywords:** wiki layout, ledger, git history, _meta

- [x] **T006:** Extrair detalhes de busca: entidades, autoridade, prepare_fts5_query e reforço de acesso
  - **Papel:** researcher
  - **agent:** researcher6
  - **Dependências:** Nenhuma
  - **depends_on:** []
  - **Paralelizável:** true
  - **Arquivos:** `specs/features/001-nucleo/notes/search-contract.md`
  - **Wiki-Keywords:** hybrid search, RRF, authority, entities

- [x] **T007:** Consolidar as 6 notas, apontar lacunas e registrar "decisão nossa" onde houver "a confirmar"
  - **Papel:** analyst
  - **agent:** analyst1
  - **Dependências:** T001, T002, T003, T004, T005, T006
  - **depends_on:** [T001, T002, T003, T004, T005, T006]
  - **Paralelizável:** false
  - **Arquivos:** `specs/features/001-nucleo/notes/phase0-findings.md`
  - **Wiki-Keywords:** a confirmar, ADR, constitution


## Fase 1: Fundação

- [x] **T008:** Criar o módulo Go (github.com/phm-aguiar/ai-memory-go) e registrar as dependências planejadas com go get, sem tidy
  - **Papel:** executor
  - **agent:** executor1
  - **Dependências:** T007
  - **depends_on:** [T007]
  - **Paralelizável:** true
  - **Arquivos:** `go.mod`, `go.sum`
  - **Wiki-Keywords:** tech.md, modernc, koanf, dependências

- [x] **T009:** Criar ID UUIDv7 de 16 bytes com parse/string e teste
  - **Papel:** executor
  - **agent:** executor2
  - **Dependências:** T008, T001
  - **depends_on:** [T008, T001]
  - **Paralelizável:** true
  - **Arquivos:** `internal/domain/id.go`, `internal/domain/id_test.go`
  - **Wiki-Keywords:** UUIDv7, BLOB, identificadores

- [x] **T010:** Criar PagePath (normalização POSIX, rejeita .. e nomes não portáveis do Windows), PortableKey e SearchText, com teste
  - **Papel:** executor
  - **agent:** executor3
  - **Dependências:** T008, T001
  - **depends_on:** [T008, T001]
  - **Paralelizável:** true
  - **Arquivos:** `internal/domain/pagepath.go`, `internal/domain/pagepath_test.go`
  - **Wiki-Keywords:** page path, portável, colisão de caixa

- [x] **T011:** Criar Tier, PageKind e ObservationKind com validação e teste
  - **Papel:** executor
  - **agent:** executor1
  - **Dependências:** T008, T001
  - **depends_on:** [T008, T001]
  - **Paralelizável:** true
  - **Arquivos:** `internal/domain/tier.go`, `internal/domain/kind.go`, `internal/domain/tier_kind_test.go`
  - **Wiki-Keywords:** tier, kind, domain-vocabulary

- [x] **T012:** Criar Scope e PageRef (identidade de 3 elementos, INV-04) com teste
  - **Papel:** executor
  - **agent:** executor2
  - **Dependências:** T009, T010
  - **depends_on:** [T009, T010]
  - **Paralelizável:** true
  - **Arquivos:** `internal/domain/scope.go`, `internal/domain/scope_test.go`
  - **Wiki-Keywords:** workspace, project, INV-04

- [x] **T013:** Criar o sanitizador: tipo Sanitized (campo privado), Text(), padrões RE2 e fixtures; remove segredo antes de limitar (título 80, corpo 16 KiB)
  - **Papel:** executor
  - **agent:** executor3
  - **Dependências:** T008, T003
  - **depends_on:** [T008, T003]
  - **Paralelizável:** true
  - **Arquivos:** `internal/domain/sanitize/sanitize.go`, `internal/domain/sanitize/patterns.go`, `internal/domain/sanitize/sanitize_test.go`, `internal/domain/sanitize/testdata/secrets.txt`
  - **Wiki-Keywords:** sanitizer, secrets, INV-06

- [x] **T014:** Criar Page, PageVersion, Frontmatter (chaves ordenadas) e Observation com campos Sanitized
  - **Papel:** executor
  - **agent:** executor1
  - **Dependências:** T011, T012, T013, T004
  - **depends_on:** [T011, T012, T013, T004]
  - **Paralelizável:** true
  - **Arquivos:** `internal/domain/page.go`, `internal/domain/page_test.go`
  - **Wiki-Keywords:** page, frontmatter, observation

- [x] **T015:** Criar internal/config: Config tipada imutável, camadas padrões < TOML < .env < env (AI_MEMORY_*, separador __), data_dir absoluto e canônico
  - **Papel:** executor
  - **agent:** executor2
  - **Dependências:** T008
  - **depends_on:** [T008]
  - **Paralelizável:** true
  - **Arquivos:** `internal/config/config.go`, `internal/config/config_test.go`
  - **Wiki-Keywords:** koanf, dotenv, INV-01, INV-11

- [x] **T016:** Criar teste de arquitetura: grafo de imports (go list -deps) e guarda de os.Getenv/LookupEnv/Environ fora de config
  - **Papel:** executor
  - **agent:** executor3
  - **Dependências:** T015
  - **depends_on:** [T015]
  - **Paralelizável:** true
  - **Arquivos:** `internal/archtest/archtest_test.go`
  - **Wiki-Keywords:** grafo de imports, constitution, INV-01

- [x] **T017:** Criar V01__init.sql com o estado final completo do database-schema.md e schema_migrations (agent_kind sem CHECK)
  - **Papel:** executor
  - **agent:** executor1
  - **Dependências:** T007
  - **depends_on:** [T007]
  - **Paralelizável:** true
  - **Arquivos:** `internal/storage/migrations/V01__init.sql`
  - **Wiki-Keywords:** database-schema, FTS5, triggers de pareamento

- [x] **T018:** Criar o runner de migrações sobre embed.FS (uma transação por migração, FK off durante, checksum tolerado, ErrSchemaAhead)
  - **Papel:** executor
  - **agent:** executor2
  - **Dependências:** T008, T017
  - **depends_on:** [T008, T017]
  - **Paralelizável:** true
  - **Arquivos:** `internal/storage/migrate.go`, `internal/storage/migrate_test.go`
  - **Wiki-Keywords:** migrações, schema à frente, checksum

- [x] **T019:** Criar os erros tipados do storage
  - **Papel:** executor
  - **agent:** executor3
  - **Dependências:** T008
  - **depends_on:** [T008]
  - **Paralelizável:** true
  - **Arquivos:** `internal/storage/errors.go`
  - **Wiki-Keywords:** erros tipados, falha fechada

- [x] **T020:** Criar storage.Open: db/ com 0700/0600 em Unix, pragmas WAL/synchronous=NORMAL/busy_timeout=5000, snapshot antes de migrar, aplica migrações
  - **Papel:** executor
  - **agent:** executor1
  - **Dependências:** T018, T019, T015
  - **depends_on:** [T018, T019, T015]
  - **Paralelizável:** true
  - **Arquivos:** `internal/storage/open.go`, `internal/storage/open_test.go`
  - **Wiki-Keywords:** abertura, WAL, snapshot antes de migrar

- [x] **T021:** Criar teste do esquema V01: tabelas, triggers FTS, triggers de pareamento e índice único parcial de is_latest
  - **Papel:** executor
  - **agent:** executor2
  - **Dependências:** T018
  - **depends_on:** [T018]
  - **Paralelizável:** true
  - **Arquivos:** `internal/storage/schema_test.go`
  - **Wiki-Keywords:** triggers, pareamento, índice único parcial


## Fase 2: Implementação


### 2A Storage

- [x] **T022:** Criar o ator de escrita (goroutine, fila 1024, closure por comando, QueueLen, Close, ErrWriterClosed) e as interfaces Writer/Reader
  - **Papel:** executor
  - **agent:** executor1
  - **Dependências:** T020
  - **depends_on:** [T020]
  - **Paralelizável:** true
  - **Arquivos:** `internal/storage/api.go`, `internal/storage/writer.go`, `internal/storage/writer_test.go`
  - **Wiki-Keywords:** escritor único, INV-02, ator, fila 1024

- [x] **T023:** Criar o pool de leitura somente leitura (soft cap 4), pânico vira erro
  - **Papel:** executor
  - **agent:** executor2
  - **Dependências:** T020
  - **depends_on:** [T020]
  - **Paralelizável:** true
  - **Arquivos:** `internal/storage/reader.go`, `internal/storage/reader_test.go`
  - **Wiki-Keywords:** pool de leitura, busy timeout

- [x] **T024:** Criar EnsureScope (único caminho que cria workspace e projeto)
  - **Papel:** executor
  - **agent:** executor3
  - **Dependências:** T022, T012
  - **depends_on:** [T022, T012]
  - **Paralelizável:** true
  - **Arquivos:** `internal/storage/scope_write.go`, `internal/storage/scope_write_test.go`
  - **Wiki-Keywords:** EnsureScope, workspace, projeto

- [x] **T025:** Criar ResolveScope (nunca cria; falha fechada com ErrScopeNotFound/ErrScopeAmbiguous; nome explícito, default e repo_path)
  - **Papel:** executor
  - **agent:** executor1
  - **Dependências:** T023, T012, T019
  - **depends_on:** [T023, T012, T019]
  - **Paralelizável:** true
  - **Arquivos:** `internal/storage/scope_read.go`, `internal/storage/scope_read_test.go`
  - **Wiki-Keywords:** resolução de escopo, falha fechada, repo_path

- [x] **T026:** Criar links: regravar por versão, dedupe pela chave lógica, reparo de links pendentes (inclui entre projetos)
  - **Papel:** executor
  - **agent:** executor2
  - **Dependências:** T022
  - **depends_on:** [T022]
  - **Paralelizável:** true
  - **Arquivos:** `internal/storage/links.go`, `internal/storage/links_test.go`
  - **Wiki-Keywords:** links, link pendente, dedupe

- [x] **T027:** Criar entidades: derivar de frontmatter e tags, janelas valid_from/superseded_at
  - **Papel:** executor
  - **agent:** executor3
  - **Dependências:** T022
  - **depends_on:** [T022]
  - **Paralelizável:** true
  - **Arquivos:** `internal/storage/entities.go`, `internal/storage/entities_test.go`
  - **Wiki-Keywords:** entidades, tags, janela de validade

- [x] **T028:** Criar evidência e auditoria dentro da transação
  - **Papel:** executor
  - **agent:** executor1
  - **Dependências:** T022
  - **depends_on:** [T022]
  - **Paralelizável:** true
  - **Arquivos:** `internal/storage/evidence.go`, `internal/storage/audit.go`, `internal/storage/evidence_audit_test.go`
  - **Wiki-Keywords:** evidência, audit_log, transação

- [x] **T029:** Criar UpsertPage com os casos A (idêntico), B (supersessão) e C (nova, colisão de caminho, ressurreição de tombstone) numa só transação
  - **Papel:** executor
  - **agent:** executor2
  - **Dependências:** T024, T026, T027, T028, T014
  - **depends_on:** [T024, T026, T027, T028, T014]
  - **Paralelizável:** true
  - **Arquivos:** `internal/storage/upsert.go`, `internal/storage/upsert_test.go`
  - **Wiki-Keywords:** upsert, supersessão, INV-16, tombstone

- [x] **T030:** Criar DeletePath (físico, idempotente, condicional à versão esperada) e limpeza de entidades órfãs
  - **Papel:** executor
  - **agent:** executor3
  - **Dependências:** T029
  - **depends_on:** [T029]
  - **Paralelizável:** true
  - **Arquivos:** `internal/storage/delete.go`, `internal/storage/delete_test.go`
  - **Wiki-Keywords:** exclusão de página, entidades órfãs

- [x] **T031:** Criar leituras ReadPage, Recent e PageChain (versão perdedora alcançável)
  - **Papel:** executor
  - **agent:** executor1
  - **Dependências:** T023, T029
  - **depends_on:** [T023, T029]
  - **Paralelizável:** true
  - **Arquivos:** `internal/storage/pages_read.go`, `internal/storage/pages_read_test.go`
  - **Wiki-Keywords:** memory_read_page, recent, cadeia de supersessão

- [x] **T032:** Criar prepareFTS5Query: sintaxe explícita, stop words, OR, dois-pontos, pontuação, sonda FTS5 e degradação
  - **Papel:** executor
  - **agent:** executor2
  - **Dependências:** T020, T006
  - **depends_on:** [T020, T006]
  - **Paralelizável:** true
  - **Arquivos:** `internal/storage/ftsquery.go`, `internal/storage/ftsquery_test.go`
  - **Wiki-Keywords:** prepare_fts5_query, stop words, FTS5

- [x] **T033:** Criar os fluxos de busca: FTS, entidades (3 a 64 chars, máx 12, peso 1/freq) e grafo bidirecional
  - **Papel:** executor
  - **agent:** executor3
  - **Dependências:** T031, T032, T006
  - **depends_on:** [T031, T032, T006]
  - **Paralelizável:** true
  - **Arquivos:** `internal/storage/search_streams.go`, `internal/storage/search_streams_test.go`
  - **Wiki-Keywords:** fluxos de busca, entidades, grafo

- [x] **T034:** Criar a fusão RRF (k=60) e o fator de autoridade [0,55; 1,50] com tetos por tag (funções puras)
  - **Papel:** executor
  - **agent:** executor1
  - **Dependências:** T014, T006
  - **depends_on:** [T014, T006]
  - **Paralelizável:** true
  - **Arquivos:** `internal/storage/rank.go`, `internal/storage/rank_test.go`
  - **Wiki-Keywords:** RRF, autoridade, tags

- [x] **T035:** Criar InsertObservation (aceita só tipos Sanitized)
  - **Papel:** executor
  - **agent:** executor1
  - **Dependências:** T022, T013, T012
  - **depends_on:** [T022, T013, T012]
  - **Paralelizável:** true
  - **Arquivos:** `internal/storage/observations.go`, `internal/storage/observations_test.go`
  - **Wiki-Keywords:** observação, sanitização, INV-06

- [x] **T036:** Criar BumpAccess throttled (1 por minuto por página e operador), page_access e access_count
  - **Papel:** executor
  - **agent:** executor3
  - **Dependências:** T029
  - **depends_on:** [T029]
  - **Paralelizável:** true
  - **Arquivos:** `internal/storage/access.go`, `internal/storage/access_test.go`
  - **Wiki-Keywords:** reforço de acesso, throttle, page_access

- [x] **T037:** Criar Search: janela de candidatos, orquestração, ordenação e fallback raw_hits por observations_fts
  - **Papel:** executor
  - **agent:** executor2
  - **Dependências:** T033, T034, T035
  - **depends_on:** [T033, T034, T035]
  - **Paralelizável:** true
  - **Arquivos:** `internal/storage/search.go`, `internal/storage/search_test.go`
  - **Wiki-Keywords:** hybrid_search, raw_hits, candidatos

- [x] **T038:** Criar Snapshot: backup online via API de backup do modernc com o escritor ativo
  - **Papel:** executor
  - **agent:** executor2
  - **Dependências:** T022
  - **depends_on:** [T022]
  - **Paralelizável:** true
  - **Arquivos:** `internal/storage/snapshot.go`, `internal/storage/snapshot_test.go`
  - **Wiki-Keywords:** backup online, snapshot, INV-09


### 2B Wiki

- [x] **T039:** Criar leitura e emissão de frontmatter YAML (bloco canônico, LF/CRLF, descarta BOM, ordem das chaves, bytes idênticos na reescrita)
  - **Papel:** executor
  - **agent:** executor1
  - **Dependências:** T014, T004
  - **depends_on:** [T014, T004]
  - **Paralelizável:** true
  - **Arquivos:** `internal/wiki/markdown.go`, `internal/wiki/markdown_test.go`
  - **Wiki-Keywords:** frontmatter, BOM, CRLF, okf

- [x] **T040:** Criar derivações: título, links, entidades (entities + tags) e expires_at (RFC 3339 ou AAAA-MM-DD; inválido é erro)
  - **Papel:** executor
  - **agent:** executor2
  - **Dependências:** T039, T004
  - **depends_on:** [T039, T004]
  - **Paralelizável:** true
  - **Arquivos:** `internal/wiki/derive.go`, `internal/wiki/derive_test.go`
  - **Wiki-Keywords:** título, links, expires_at, entidades

- [x] **T041:** Criar escrita atômica (temporário .ai-memory-tmp.*, Sync, rename, fsync do diretório, retry no Windows nos erros 32/33, devolve id do arquivo)
  - **Papel:** executor
  - **agent:** executor3
  - **Dependências:** T008, T005
  - **depends_on:** [T008, T005]
  - **Paralelizável:** true
  - **Arquivos:** `internal/wiki/atomicfile.go`, `internal/wiki/atomicfile_windows.go`, `internal/wiki/atomicfile_other.go`, `internal/wiki/atomicfile_test.go`
  - **Wiki-Keywords:** escrita atômica, INV-10, Windows, rename

- [x] **T042:** Criar confinamento e pathFor(scope, path): recusa symlinks e pontos de reparse, nunca concatena à mão
  - **Papel:** executor
  - **agent:** executor1
  - **Dependências:** T012, T005
  - **depends_on:** [T012, T005]
  - **Paralelizável:** true
  - **Arquivos:** `internal/wiki/confine.go`, `internal/wiki/confine_test.go`
  - **Wiki-Keywords:** confinamento, symlink, reparse point

- [x] **T043:** Criar o adaptador git via os/exec (valida git no PATH, init, add+commit sob mutex, autor fixo, gpgsign=false, caminhos após --)
  - **Papel:** executor
  - **agent:** executor2
  - **Dependências:** T008, T005
  - **depends_on:** [T008, T005]
  - **Paralelizável:** true
  - **Arquivos:** `internal/wiki/gitrepo.go`, `internal/wiki/gitrepo_test.go`
  - **Wiki-Keywords:** git, commit, checkpoint

- [x] **T044:** Criar a trava por página e aquisição em ordem determinística
  - **Papel:** executor
  - **agent:** executor3
  - **Dependências:** T012
  - **depends_on:** [T012]
  - **Paralelizável:** true
  - **Arquivos:** `internal/wiki/locks.go`, `internal/wiki/locks_test.go`
  - **Wiki-Keywords:** trava por página, deadlock, ordem

- [x] **T045:** Criar manifesto e ledger do escopo: _meta.md, index.md e linha no log.md
  - **Papel:** executor
  - **agent:** executor1
  - **Dependências:** T041, T042, T005
  - **depends_on:** [T041, T042, T005]
  - **Paralelizável:** true
  - **Arquivos:** `internal/wiki/meta.go`, `internal/wiki/ledger.go`, `internal/wiki/meta_ledger_test.go`
  - **Wiki-Keywords:** _meta.md, log.md, ledger

- [x] **T046:** Criar WritePage (fluxo P-03): confina, sanitiza, deriva, trava, instala o arquivo, UpsertPage, rollback do arquivo em falha (ErrInconsistentState), commit git
  - **Papel:** executor
  - **agent:** executor2
  - **Dependências:** T040, T041, T042, T043, T044, T045, T029, T013
  - **depends_on:** [T040, T041, T042, T043, T044, T045, T029, T013]
  - **Paralelizável:** true
  - **Arquivos:** `internal/wiki/write.go`, `internal/wiki/write_test.go`
  - **Wiki-Keywords:** WritePage, rollback, INV-03, P-03

- [x] **T047:** Criar DeletePage pela camada wiki (arquivo, índice e commit)
  - **Papel:** executor
  - **agent:** executor3
  - **Dependências:** T046, T030
  - **depends_on:** [T046, T030]
  - **Paralelizável:** true
  - **Arquivos:** `internal/wiki/delete.go`, `internal/wiki/delete_test.go`
  - **Wiki-Keywords:** exclusão de página, memory_delete_page

- [x] **T048:** Criar IndexFile e Reindex pelo mesmo UpsertPage, reconstruindo escopos a partir de _meta.md
  - **Papel:** executor
  - **agent:** executor1
  - **Dependências:** T046
  - **depends_on:** [T046]
  - **Paralelizável:** true
  - **Arquivos:** `internal/wiki/reindex.go`, `internal/wiki/reindex_test.go`
  - **Wiki-Keywords:** reindex, _meta.md, fonte da verdade

- [x] **T049:** Criar o watcher com fsnotify (debounce 300 ms, reconciliação 30 s, ignora temporários; apagar arquivo não apaga a página)
  - **Papel:** executor
  - **agent:** executor2
  - **Dependências:** T048, T041
  - **depends_on:** [T048, T041]
  - **Paralelizável:** true
  - **Arquivos:** `internal/wiki/watcher.go`, `internal/wiki/watcher_test.go`
  - **Wiki-Keywords:** watcher, debounce, reconciliação


### 2C CLI mínima e cenários Gherkin

- [x] **T050:** Criar logging com slog que filtra o próprio módulo (INV-15), nível via config e registro do data_dir canônico (INV-11)
  - **Papel:** executor
  - **agent:** executor1
  - **Dependências:** T015
  - **depends_on:** [T015]
  - **Paralelizável:** true
  - **Arquivos:** `internal/cli/logging.go`, `internal/cli/logging_test.go`
  - **Wiki-Keywords:** slog, INV-15, INV-11

- [x] **T051:** Criar a raiz da CLI com cobra e a composição (config uma vez, abre storage e wiki) e o main fino
  - **Papel:** executor
  - **agent:** executor2
  - **Dependências:** T050, T020, T022, T046
  - **depends_on:** [T050, T020, T022, T046]
  - **Paralelizável:** true
  - **Arquivos:** `internal/cli/root.go`, `cmd/ai-memory/main.go`
  - **Wiki-Keywords:** cobra, composição, INV-01

- [x] **T052:** Criar a guarda de processo vivo (INV-09) com build tags Windows e Linux
  - **Papel:** executor
  - **agent:** executor3
  - **Dependências:** T015
  - **depends_on:** [T015]
  - **Paralelizável:** true
  - **Arquivos:** `internal/cli/liveguard.go`, `internal/cli/liveguard_windows.go`, `internal/cli/liveguard_other.go`, `internal/cli/liveguard_test.go`
  - **Wiki-Keywords:** processo vivo, INV-09, reindex

- [x] **T053:** Criar o comando ai-memory reindex (guarda de processo vivo e wiki.Reindex)
  - **Papel:** executor
  - **agent:** executor1
  - **Dependências:** T048, T051, T052
  - **depends_on:** [T048, T051, T052]
  - **Paralelizável:** true
  - **Arquivos:** `internal/cli/reindex_cmd.go`, `internal/cli/reindex_cmd_test.go`
  - **Wiki-Keywords:** reindex, CLI, INV-09

- [x] **T054:** Criar o comando ai-memory backup --to (online, sem a guarda, usa Snapshot)
  - **Papel:** executor
  - **agent:** executor2
  - **Dependências:** T038, T051
  - **depends_on:** [T038, T051]
  - **Paralelizável:** true
  - **Arquivos:** `internal/cli/backup_cmd.go`, `internal/cli/backup_cmd_test.go`
  - **Wiki-Keywords:** backup, snapshot, CLI

- [x] **T055:** Criar o arquivo .feature com os cenários da 001 (3 de sucesso, 2 de falha, 3 de edge)
  - **Papel:** executor
  - **agent:** executor3
  - **Dependências:** T007
  - **depends_on:** [T007]
  - **Paralelizável:** true
  - **Arquivos:** `specs/features/001-nucleo/acceptance/nucleo.feature`
  - **Wiki-Keywords:** Gherkin, cenários, aceitação

- [x] **T056:** Implementar Scenario: Escrever uma página atualiza arquivo, índice e git juntos
  - **Papel:** executor
  - **agent:** executor1
  - **Dependências:** T046, T055
  - **depends_on:** [T046, T055]
  - **Paralelizável:** true
  - **Arquivos:** `test/acceptance/write_page_test.go`
  - **Wiki-Keywords:** WritePage, git, log.md

- [x] **T057:** Implementar Scenario: Busca híbrida retorna páginas ranqueadas por RRF (fixtures de docs/assets/fixtures)
  - **Papel:** executor
  - **agent:** executor2
  - **Dependências:** T037, T046, T002, T055
  - **depends_on:** [T037, T046, T002, T055]
  - **Paralelizável:** true
  - **Arquivos:** `test/acceptance/search_rrf_test.go`
  - **Wiki-Keywords:** RRF, fixtures, autoridade

- [x] **T058:** Implementar tratamento de erro: Falha no banco após instalar o arquivo reverte a página (injeção de falha no Writer)
  - **Papel:** executor
  - **agent:** executor3
  - **Dependências:** T046, T055
  - **depends_on:** [T046, T055]
  - **Paralelizável:** true
  - **Arquivos:** `test/acceptance/rollback_test.go`
  - **Wiki-Keywords:** rollback, ErrInconsistentState, falha no banco

- [x] **T059:** Implementar tratamento de erro: Leitura sem escopo resolvível falha fechada
  - **Papel:** executor
  - **agent:** executor1
  - **Dependências:** T025, T055
  - **depends_on:** [T025, T055]
  - **Paralelizável:** true
  - **Arquivos:** `test/acceptance/scope_fail_closed_test.go`
  - **Wiki-Keywords:** falha fechada, ResolveScope, escopo

- [x] **T060:** Implementar edge case: Duas sessões escrevem o mesmo caminho ao mesmo tempo (multi-sessão, -race, supersessão, perdedora alcançável)
  - **Papel:** executor
  - **agent:** executor2
  - **Dependências:** T046, T031, T055
  - **depends_on:** [T046, T031, T055]
  - **Paralelizável:** true
  - **Arquivos:** `test/acceptance/concurrent_supersession_test.go`
  - **Wiki-Keywords:** supersessão, INV-16, multi-sessão

- [x] **T061:** Implementar edge cases: Edição manual da wiki é reindexada e Segredo no texto nunca chega ao armazenamento
  - **Papel:** executor
  - **agent:** executor3
  - **Dependências:** T049, T035, T055
  - **depends_on:** [T049, T035, T055]
  - **Paralelizável:** true
  - **Arquivos:** `test/acceptance/manual_edit_test.go`, `test/acceptance/secret_sanitization_test.go`
  - **Wiki-Keywords:** watcher, sanitização, reindex


## Fase 3: Validação

- [x] **T062:** Criar a conformidade do storage: var _ Writer e var _ Reader sobre *Store
  - **Papel:** executor
  - **agent:** executor1
  - **Dependências:** T030, T037, T036, T035, T038
  - **depends_on:** [T030, T037, T036, T035, T038]
  - **Paralelizável:** true
  - **Arquivos:** `internal/storage/conformance_test.go`
  - **Wiki-Keywords:** interfaces, Writer, Reader

- [x] **T063:** Criar testes de contrato INV-02, INV-03 e INV-04
  - **Papel:** executor
  - **agent:** executor2
  - **Dependências:** T062
  - **depends_on:** [T062]
  - **Paralelizável:** true
  - **Arquivos:** `test/contract/storage_inv_test.go`
  - **Wiki-Keywords:** INV-02, INV-03, INV-04

- [x] **T064:** Criar testes de contrato INV-06, INV-10 e INV-16
  - **Papel:** executor
  - **agent:** executor3
  - **Dependências:** T046, T049
  - **depends_on:** [T046, T049]
  - **Paralelizável:** true
  - **Arquivos:** `test/contract/wiki_inv_test.go`
  - **Wiki-Keywords:** INV-06, INV-10, INV-16

- [x] **T065:** Criar testes de contrato INV-01, INV-09, INV-11, INV-13 e INV-15
  - **Papel:** executor
  - **agent:** executor1
  - **Dependências:** T016, T050, T052
  - **depends_on:** [T016, T050, T052]
  - **Paralelizável:** true
  - **Arquivos:** `test/contract/runtime_inv_test.go`
  - **Wiki-Keywords:** INV-01, INV-09, INV-13, INV-15

- [x] **T066:** Criar BenchmarkSearch5k (5 mil páginas, alvo < 100 ms, baseline registrado)
  - **Papel:** executor
  - **agent:** executor2
  - **Dependências:** T037, T046
  - **depends_on:** [T037, T046]
  - **Paralelizável:** true
  - **Arquivos:** `test/bench/search_bench_test.go`
  - **Wiki-Keywords:** benchmark, FTS5, 5 mil páginas

- [x] **T067:** Criar o script de verificação: build CGO_ENABLED=0 para windows/amd64 e linux/amd64, go vet e go test -race (falha clara sem compilador C)
  - **Papel:** executor
  - **agent:** executor3
  - **Dependências:** T051
  - **depends_on:** [T051]
  - **Paralelizável:** true
  - **Arquivos:** `scripts/check.sh`, `scripts/check.ps1`
  - **Wiki-Keywords:** CGO_ENABLED=0, race detector, cross-compile

- [x] **T068:** Criar o .golangci.yml (gofmt, govet, errcheck, staticcheck)
  - **Papel:** executor
  - **agent:** executor1
  - **Dependências:** T008
  - **depends_on:** [T008]
  - **Paralelizável:** true
  - **Arquivos:** `.golangci.yml`
  - **Wiki-Keywords:** golangci-lint, linter

- [x] **T069:** Criar o pipeline de CI no GitHub Actions (windows-latest e ubuntu-latest, roda T067 e o linter)
  - **Papel:** executor
  - **agent:** executor2
  - **Dependências:** T067, T068
  - **depends_on:** [T067, T068]
  - **Paralelizável:** true
  - **Arquivos:** `.github/workflows/ci.yml`
  - **Wiki-Keywords:** CI, GitHub Actions, matriz

- [x] **T070:** Rodar go mod tidy e conferir as dependências contra o tech.md (única task que mexe em go.mod e go.sum depois do T008)
  - **Papel:** executor
  - **agent:** executor3
  - **Dependências:** T062, T053, T054, T061, T066
  - **depends_on:** [T062, T053, T054, T061, T066]
  - **Paralelizável:** true
  - **Arquivos:** `go.mod`, `go.sum`
  - **Wiki-Keywords:** go mod tidy, tech.md, dependências

- [x] **T071:** Criar o smoke test fim a fim: compila o binário, escreve páginas num repo temporário, roda reindex e backup e confere
  - **Papel:** executor
  - **agent:** executor1
  - **Dependências:** T053, T054, T059
  - **depends_on:** [T053, T054, T059]
  - **Paralelizável:** true
  - **Arquivos:** `test/smoke/smoke_test.go`
  - **Wiki-Keywords:** smoke test, reindex, backup

- [x] **T072:** Auditar a feature contra a constituição (checklist do plan.md seção 4, item a item, com evidência)
  - **Papel:** analyst
  - **agent:** analyst1
  - **Dependências:** T056, T057, T058, T059, T060, T061, T063, T064, T065, T066, T069, T070, T071
  - **depends_on:** [T056, T057, T058, T059, T060, T061, T063, T064, T065, T066, T069, T070, T071]
  - **Paralelizável:** false
  - **Arquivos:** `specs/features/001-nucleo/notes/constitution-audit.md`
  - **Wiki-Keywords:** constitution, auditoria, INV


## Fase 4: Documentação

- [x] **T073:** Criar o README.md (o que é, build, configuração e .env, comandos reindex e backup, como rodar testes incluindo gcc para -race)
  - **Papel:** executor
  - **agent:** executor1
  - **Dependências:** T072
  - **depends_on:** [T072]
  - **Paralelizável:** true
  - **Arquivos:** `README.md`
  - **Wiki-Keywords:** README, build, configuração

- [x] **T074:** Atualizar o .env.example com as chaves finais usadas pela config (sem segredos)
  - **Papel:** executor
  - **agent:** executor2
  - **Dependências:** T015, T050
  - **depends_on:** [T015, T050]
  - **Paralelizável:** true
  - **Arquivos:** `.env.example`
  - **Wiki-Keywords:** .env.example, INV-01

- [x] **T075:** Atualizar o tech.md trocando "planejado" pelo real (versões do go.mod, linter, CI, comandos)
  - **Papel:** executor
  - **agent:** executor3
  - **Dependências:** T070, T068, T069
  - **depends_on:** [T070, T068, T069]
  - **Paralelizável:** true
  - **Arquivos:** `.github/memory/tech.md`
  - **Wiki-Keywords:** tech.md, versões, CI

- [x] **T076:** Criar o llms.txt na raiz (mapa de pacotes, invariantes e onde achar os docs)
  - **Papel:** executor
  - **agent:** executor1
  - **Dependências:** T073
  - **depends_on:** [T073]
  - **Paralelizável:** true
  - **Arquivos:** `llms.txt`
  - **Wiki-Keywords:** llms.txt, onboarding

- [x] **T077:** Criar a matriz invariante para teste (cada INV-nn com arquivo e nome do teste; INV-12 reservada)
  - **Papel:** executor
  - **agent:** executor2
  - **Dependências:** T063, T064, T065
  - **depends_on:** [T063, T064, T065]
  - **Paralelizável:** true
  - **Arquivos:** `specs/features/001-nucleo/notes/inv-test-matrix.md`
  - **Wiki-Keywords:** INV, matriz, testes de contrato


## Fase 5: Correções e rastreio pós-revisão (2026-10)

> Tasks abertas a partir da verificação de `-race`, do lint e da revisão de 2026-10. `[x]` = feita e verificada. `[ ]` com "BLOQUEADA" = depende de decisão do usuário; a pergunta está na task.
>
> **Movidas para a feature 002** (`specs/features/002-captura-e-acesso/spec.md`, seção "Herdado da 001"), em 2026-10-13: a guarda de processo vivo (antiga T086; na 002 vira `.serve.lock` e enumeração de processos, como o doc define), o `occurred_at` (antiga T087) e a D5 de `constitution-audit.md` (handoff e ponteiro por ator). Os IDs T086 e T087 não são reutilizados.
>
> **Correção (2026-10-13):** o item 7 da revisão ("falta a coluna `occurred_at`") não procede. `docs/domain-vocabulary.md` §5.2 descreve um campo de **entrada**, e `docs/database-schema.md` define `observations` sem a coluna: o instante vai para `created_at`, como `internal/storage/observations.go` já faz. Não há migração V02; a 002 só lê o campo do corpo do hook.

- [x] **T078:** Corrigir o teste da sentinela do watcher: semente com `WritePage` antes de armar (o projeto precisa existir no banco)
  - **Papel:** executor
  - **agent:** executor1
  - **Dependências:** T049
  - **depends_on:** [T049]
  - **Paralelizável:** true
  - **Arquivos:** `internal/wiki/watcher_test.go`
  - **Wiki-Keywords:** watcher, WritePage, escopo, teste
  - **Verificação:** `go test -race -count=15 -run TestWatcher ./internal/wiki/` passou 15 de 15 no container `golang:1.26`.

- [x] **T079:** Trocar o golangci-lint para v2.14.0 (v2.1.6 foi compilado com Go 1.24 e recusa `go 1.26.0`) e corrigir os apontamentos de errcheck e staticcheck
  - **Papel:** executor
  - **agent:** executor2
  - **Dependências:** T068
  - **depends_on:** [T068]
  - **Paralelizável:** true
  - **Arquivos:** `.github/workflows/ci.yml`, `.github/memory/tech.md`, `internal/cli/backup_cmd.go`, `internal/cli/reindex_cmd.go`, `internal/domain/scope_test.go`, `internal/storage/*.go`, `internal/wiki/*.go`, `test/**/*.go`
  - **Wiki-Keywords:** golangci-lint, errcheck, staticcheck, lint
  - **Verificação:** `golangci-lint run ./...` em `golangci/golangci-lint:v2.14.0` → 0 issues.

- [x] **T080:** Tornar o `-race` reproduzível sem gcc no host: `scripts/check.sh` e `scripts/check.ps1` usam o container `golang:<major.minor de go.mod>` quando não há gcc; docs e constituição atualizados
  - **Papel:** executor
  - **agent:** executor3
  - **Dependências:** T067, T073
  - **depends_on:** [T067, T073]
  - **Paralelizável:** true
  - **Arquivos:** `scripts/check.sh`, `scripts/check.ps1`, `.github/memory/constitution.md`, `README.md`, `llms.txt`, `.github/memory/tech.md`
  - **Wiki-Keywords:** race, Docker, gcc, constituição
  - **Verificação:** `sh scripts/check.sh` chega ao `-race` no container e não reporta `DATA RACE`. O `check.ps1` foi validado só por parse; não foi executado de ponta a ponta.

- [x] **T081:** Reapontar os links de entrada para a versão nova na supersessão (revisão, item 1)
  - **Papel:** executor
  - **agent:** executor2
  - **Dependências:** T029, T026
  - **depends_on:** [T029, T026]
  - **Paralelizável:** true
  - **Arquivos:** `internal/storage/upsert.go`, `internal/storage/links.go`, `internal/storage/upsert_test.go`
  - **Wiki-Keywords:** supersessão, links, to_page_id, grafo
  - **Verificação:** `TestUpsertSupersessaoReapontaLinksDeEntrada` falhava antes da correção e passa depois; `internal/storage` e `internal/wiki` passam.

- [x] **T082:** Ramo de ressurreição de lápide (T029, caso C) sem produtor de lápide (revisão, item 2): resolvido pelo doc em 2026-10-13
  - **Papel:** analyst
  - **agent:** analyst1
  - **Dependências:** T029
  - **depends_on:** [T029]
  - **Paralelizável:** false
  - **Arquivos:** `internal/storage/upsert.go`, `internal/storage/upsert_test.go`
  - **Wiki-Keywords:** lápide, decaimento, ressurreição, superseded_at
  - **Achado:** nenhum código de produção grava `pages.superseded_at`; a supersessão não o toca. O ramo só roda em teste que simula o estado com `UPDATE`. A marca não é perdida pela supersessão, mas a ressurreição é código sem uso em produção.
  - **Pergunta:** implementar o decaimento (o que produz a lápide) nesta feature ou adiar para a 002 e remover o ramo?
  - **Resolução pelo doc:** `docs/storage-and-retrieval.md` §5.1 e §5.3 definem a ressurreição e o tombstone de decaimento, então o ramo fica. O produtor da lápide é a varredura de esquecimento (`forget_sweep`, manutenção), fora da 001 e da 002. O doc também mostrou que a ressurreição da 001 está errada: deve **limpar** o `superseded_at` da lápide e manter o `valid_to` original. A correção é a 002/T088, e o item 2 da revisão estava parcialmente certo.

- [x] **T083:** `[[_global:x]]` resolve para o workspace padrão (`default/_global`), a menos que se escreva `workspace/_global` (revisão, item 3)
  - **Papel:** executor
  - **agent:** executor1
  - **Dependências:** T040
  - **depends_on:** [T040]
  - **Paralelizável:** true
  - **Arquivos:** `internal/wiki/derive.go`, `internal/wiki/derive_test.go`
  - **Wiki-Keywords:** wikilink, _global, workspace padrão
  - **Decisão (2026-10-13):** seguir o doc de origem. `docs/wiki-files-layer.md` (§ links) define `[[_global:caminho]]` como a forma reservada do projeto de preferências, que fica no workspace padrão e não no de origem, a menos que o escopo explícito `workspace/_global` seja usado. Vale também para relações tipadas, que usam a mesma gramática (`parseTarget`).
  - **Verificação:** `TestDeriveExtractLinks_FormasDoCorpo` atualizado e `TestDeriveExtractLinks_GlobalFicaNoWorkspacePadrao` novo.

- [x] **T084:** Aceitar escalares YAML não-texto no frontmatter como texto literal (revisão, item 4)
  - **Papel:** executor
  - **agent:** executor2
  - **Dependências:** T039
  - **depends_on:** [T039]
  - **Paralelizável:** true
  - **Arquivos:** `internal/wiki/markdown.go`, `internal/wiki/markdown_test.go`
  - **Wiki-Keywords:** frontmatter, YAML, booleano, data, okf_version
  - **Decisão (2026-10-13):** o doc é omisso (só mostra `okf_version: "0.2"` com aspas na saída; a nota `markdown-contract.md` registra o estilo de emissão como lacuna). Regra nossa: `!!bool`, `!!int`, `!!float` e `!!timestamp` valem pelo texto literal (booleano em minúsculas), inclusive em listas. Nulo, mapeamento e alias continuam recusados. Na reescrita o sistema emite com aspas (`pinned: "true"`), que preserva o significado; só arquivos feitos à mão mudam de bytes, uma vez, quando o sistema os reescreve.
  - **Verificação:** `TestMarkdownEscalaresNaoTextoViramTexto` novo (leitura, ida e volta, aspas na reescrita); `TestMarkdownReadYAMLInvalidoDaErroTipado` ajustado para os casos que continuam inválidos.

- [x] **T085:** A fixação (`pinned`) persiste quando a escrita não envia a chave; só `pinned: false` explícito desafixa (revisão, item 5)
  - **Papel:** executor
  - **agent:** executor3
  - **Dependências:** T046
  - **depends_on:** [T046]
  - **Paralelizável:** true
  - **Arquivos:** `internal/wiki/write.go`, `internal/wiki/write_test.go`
  - **Wiki-Keywords:** pinned, frontmatter, fixação
  - **Decisão (2026-10-13):** o doc é omisso sobre herança (`docs/mcp-server.md` só diz que o frontmatter aceita `pinned`). Regra nossa: sem a chave, a escrita herda `pinned: true` do arquivo atual (o markdown é a fonte da verdade), antes de comparar o conteúdo, para que uma reescrita idêntica continue sendo reconhecida como idêntica.
  - **Verificação:** `TestWritePage_PinnedPersisteQuandoAusenteEFalseDesafixa` falha sem a herança (pinned = 0) e passa com ela.

- [x] **T088:** Alinhar `docs/backup.md` ao comportamento real de `backup --to` (snapshot SQLite, não tarball) (revisão, item 8)
  - **Papel:** executor
  - **agent:** executor1
  - **Dependências:** T054
  - **depends_on:** [T054]
  - **Paralelizável:** true
  - **Arquivos:** `docs/backup.md`
  - **Wiki-Keywords:** backup, snapshot, tarball, INV-09
  - **Verificação:** nota de implementação no topo do documento. O comando e o teste não mudaram.
  - **Revertida (2026-10-13):** a direção estava errada. Os docs (`docs/lifecycle-ops.md` § backup, `docs/cli.md` §9, `docs/mcp-server.md` §6) definem o backup como tarball gzip gerado pelo servidor (`POST /admin/backup`), e neste projeto o doc vence o código. A nota foi removida do `docs/backup.md`, e o código passa a seguir o doc na 002 (T081 e T082).

- [x] **T089:** Decisões D1 a D4 de `constitution-audit.md` §7 (revisão, item 9), resolvidas pelos docs e incorporadas à constituição em 2026-10-13. A D5 (handoff e ponteiro por ator) saiu daqui: a 002 a implementa
  - **Papel:** analyst
  - **agent:** analyst1
  - **Dependências:** T072
  - **depends_on:** [T072]
  - **Paralelizável:** false
  - **Arquivos:** `specs/features/001-nucleo/notes/constitution-audit.md`
  - **Wiki-Keywords:** auditoria, INV-06, INV-10, INV-16, decisão
  - **Pergunta:** aceitar cada desvio (D1 a D4) como decisão nossa em ADR, ou corrigir? Cada item tem sugestão de texto no próprio audit. D1 a D4 são decisões sobre o texto da constituição e ADRs, não código de feature; a 002 não os resolve sozinha.
  - **Achado nos docs (2026-10-13):** D2 e D3 têm resposta nos docs. **D2:** os ledgers são "somente acréscimo" por definição (`docs/wiki-files-layer.md` §9, `docs/lifecycle-event-ingestion.md` §13), então a exceção do `log.md`/`log-AAAA-MM.md` à INV-10 é do próprio produto. **D3:** o `reindex` recria workspaces e projetos a partir do `_meta.md`, mantendo os UUIDs dos diretórios (`docs/lifecycle-ops.md` § reindex), então essa criação de escopo é reconstrução prevista. D1 (`a confirmar` no próprio `docs/overview.md`) e D4 (testes que leem ambiente) o doc não decide. Mudar o texto da constituição continua exigindo confirmação do autor.

- [x] **T090:** Fechar a 001 sem a primeira execução verde do CI (revisão, item 10): decidido pelo autor em 2026-10-13 ("ignora o CI do git"); o portão `-race` foi verificado localmente no container
  - **Papel:** analyst
  - **agent:** analyst1
  - **Dependências:** T072, T069
  - **depends_on:** [T072, T069]
  - **Paralelizável:** false
  - **Arquivos:** `specs/features/001-nucleo/spec.md`, `specs/features/001-nucleo/notes/constitution-audit.md`
  - **Wiki-Keywords:** fechamento, CI, -race, portão
  - **Achado:** localmente o `-race` passa no container; o CI nunca rodou. A auditoria (P2) marca isso como bloqueante.
  - **Pergunta:** esperar a primeira execução verde do CI, ou fechar com ressalva registrada?


## Coordination Graph

G₀ (round 0):

- **nodes:** T001, T002, T003, T004, T005, T006, T007, T008, T009, T010, T011, T012, T013, T014, T015, T016, T017, T018, T019, T020, T021, T022, T023, T024, T025, T026, T027, T028, T029, T030, T031, T032, T033, T034, T035, T036, T037, T038, T039, T040, T041, T042, T043, T044, T045, T046, T047, T048, T049, T050, T051, T052, T053, T054, T055, T056, T057, T058, T059, T060, T061, T062, T063, T064, T065, T066, T067, T068, T069, T070, T071, T072, T073, T074, T075, T076, T077, T078, T079, T080, T081, T082, T083, T084, T085, T088, T089, T090
- **edges:**
  - T001 → T007
  - T002 → T007
  - T003 → T007
  - T004 → T007
  - T005 → T007
  - T006 → T007
  - T007 → T008
  - T008 → T009
  - T001 → T009
  - T008 → T010
  - T001 → T010
  - T008 → T011
  - T001 → T011
  - T009 → T012
  - T010 → T012
  - T008 → T013
  - T003 → T013
  - T011 → T014
  - T012 → T014
  - T013 → T014
  - T004 → T014
  - T008 → T015
  - T015 → T016
  - T007 → T017
  - T008 → T018
  - T017 → T018
  - T008 → T019
  - T018 → T020
  - T019 → T020
  - T015 → T020
  - T018 → T021
  - T020 → T022
  - T020 → T023
  - T022 → T024
  - T012 → T024
  - T023 → T025
  - T012 → T025
  - T019 → T025
  - T022 → T026
  - T022 → T027
  - T022 → T028
  - T024 → T029
  - T026 → T029
  - T027 → T029
  - T028 → T029
  - T014 → T029
  - T029 → T030
  - T023 → T031
  - T029 → T031
  - T020 → T032
  - T006 → T032
  - T031 → T033
  - T032 → T033
  - T006 → T033
  - T014 → T034
  - T006 → T034
  - T022 → T035
  - T013 → T035
  - T012 → T035
  - T029 → T036
  - T033 → T037
  - T034 → T037
  - T035 → T037
  - T022 → T038
  - T014 → T039
  - T004 → T039
  - T039 → T040
  - T004 → T040
  - T008 → T041
  - T005 → T041
  - T012 → T042
  - T005 → T042
  - T008 → T043
  - T005 → T043
  - T012 → T044
  - T041 → T045
  - T042 → T045
  - T005 → T045
  - T040 → T046
  - T041 → T046
  - T042 → T046
  - T043 → T046
  - T044 → T046
  - T045 → T046
  - T029 → T046
  - T013 → T046
  - T046 → T047
  - T030 → T047
  - T046 → T048
  - T048 → T049
  - T041 → T049
  - T015 → T050
  - T050 → T051
  - T020 → T051
  - T022 → T051
  - T046 → T051
  - T015 → T052
  - T048 → T053
  - T051 → T053
  - T052 → T053
  - T038 → T054
  - T051 → T054
  - T007 → T055
  - T046 → T056
  - T055 → T056
  - T037 → T057
  - T046 → T057
  - T002 → T057
  - T055 → T057
  - T046 → T058
  - T055 → T058
  - T025 → T059
  - T055 → T059
  - T046 → T060
  - T031 → T060
  - T055 → T060
  - T049 → T061
  - T035 → T061
  - T055 → T061
  - T030 → T062
  - T037 → T062
  - T036 → T062
  - T035 → T062
  - T038 → T062
  - T062 → T063
  - T046 → T064
  - T049 → T064
  - T016 → T065
  - T050 → T065
  - T052 → T065
  - T037 → T066
  - T046 → T066
  - T051 → T067
  - T008 → T068
  - T067 → T069
  - T068 → T069
  - T062 → T070
  - T053 → T070
  - T054 → T070
  - T061 → T070
  - T066 → T070
  - T053 → T071
  - T054 → T071
  - T059 → T071
  - T056 → T072
  - T057 → T072
  - T058 → T072
  - T059 → T072
  - T060 → T072
  - T061 → T072
  - T063 → T072
  - T064 → T072
  - T065 → T072
  - T066 → T072
  - T069 → T072
  - T070 → T072
  - T071 → T072
  - T072 → T073
  - T015 → T074
  - T050 → T074
  - T070 → T075
  - T068 → T075
  - T069 → T075
  - T073 → T076
  - T063 → T077
  - T064 → T077
  - T065 → T077
  - T049 → T078
  - T068 → T079
  - T067 → T080
  - T073 → T080
  - T029 → T081
  - T026 → T081
  - T029 → T082
  - T040 → T083
  - T039 → T084
  - T046 → T085
  - T054 → T088
  - T072 → T089
  - T072 → T090
  - T069 → T090
- **assignments:**
  - T001: researcher1
  - T002: researcher2
  - T003: researcher3
  - T004: researcher4
  - T005: researcher5
  - T006: researcher6
  - T007: analyst1
  - T008: executor1
  - T009: executor2
  - T010: executor3
  - T011: executor1
  - T012: executor2
  - T013: executor3
  - T014: executor1
  - T015: executor2
  - T016: executor3
  - T017: executor1
  - T018: executor2
  - T019: executor3
  - T020: executor1
  - T021: executor2
  - T022: executor1
  - T023: executor2
  - T024: executor3
  - T025: executor1
  - T026: executor2
  - T027: executor3
  - T028: executor1
  - T029: executor2
  - T030: executor3
  - T031: executor1
  - T032: executor2
  - T033: executor3
  - T034: executor1
  - T035: executor1
  - T036: executor3
  - T037: executor2
  - T038: executor2
  - T039: executor1
  - T040: executor2
  - T041: executor3
  - T042: executor1
  - T043: executor2
  - T044: executor3
  - T045: executor1
  - T046: executor2
  - T047: executor3
  - T048: executor1
  - T049: executor2
  - T050: executor1
  - T051: executor2
  - T052: executor3
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
  - T072: analyst1
  - T073: executor1
  - T074: executor2
  - T075: executor3
  - T076: executor1
  - T077: executor2
  - T078: executor1
  - T079: executor2
  - T080: executor3
  - T081: executor2
  - T082: analyst1
  - T083: executor1
  - T084: executor2
  - T085: executor3
  - T088: executor1
  - T089: analyst1
  - T090: analyst1
- **ready:** [T001, T002, T003, T004, T005, T006]

> Workers: Lead, researcher1..6, analyst1, executor1..3. Caminho crítico: T001 → T007 → T008 → T018 → T020 → T022 → T024 → T029 → T031 → T033 → T037 → T062 → T063 → T072 → T073 → T076 (16 rounds mínimos).
