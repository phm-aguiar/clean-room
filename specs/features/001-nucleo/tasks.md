# Tasks: Núcleo (domínio, armazenamento e wiki)

> Fonte: `spec.md` (Aprovado: true), `plan.md` (ADR-001 a ADR-008), `acceptance/nucleo.feature` (17 cenários).
> `docs/` vence este arquivo. Em dúvida, comportamento do original. Leia `.github/memory/constitution.md` antes de cada task.

## Fase 1: Fundação

- [ ] **T001:** Criar o módulo Go (go 1.26), a configuração do golangci-lint e o `main` mínimo
  - **Papel:** executor
  - **Dependências:** Nenhuma
  - **Paralelizável:** false
  - **Arquivos:** `go.mod`, `.golangci.yml`, `cmd/ai-memory/main.go`
  - **Wiki-Keywords:** módulo único, CGO_ENABLED=0, layout internal

- [ ] **T002:** Criar o teste do grafo de imports da constituição (`TestImportGraph`)
  - **Papel:** executor
  - **Dependências:** T001
  - **Paralelizável:** true
  - **Arquivos:** `internal/archtest/imports_test.go`
  - **Wiki-Keywords:** grafo de dependências, domain sem I/O, cli compõe

- [ ] **T003:** Implementar identificadores UUIDv7 tipados (workspace, projeto, sessão, observação, página) com BLOB de 16 bytes
  - **Papel:** executor
  - **Dependências:** T001
  - **Paralelizável:** true
  - **Arquivos:** `internal/domain/id.go`
  - **Wiki-Keywords:** UUIDv7, identidade tipada, INV-04

- [ ] **T004:** Implementar `PagePath` com validação básica e `ensure_portable` (`docs/domain-vocabulary.md` §4.2)
  - **Papel:** executor
  - **Dependências:** T001
  - **Paralelizável:** true
  - **Arquivos:** `internal/domain/pagepath.go`
  - **Wiki-Keywords:** ensure_portable, caminho de página, nomes de dispositivo

- [ ] **T005:** Implementar ADR-007: chave portável (minúsculas Unicode completas + NFC)
  - **Papel:** executor
  - **Dependências:** T004
  - **Paralelizável:** true
  - **Arquivos:** `internal/domain/portablekey.go`
  - **Wiki-Keywords:** chave portável, colisão de caixa, NFC

- [ ] **T006:** Implementar tiers e tipos de observação com grafias aceitas e "registro malformado"
  - **Papel:** executor
  - **Dependências:** T001
  - **Paralelizável:** true
  - **Arquivos:** `internal/domain/tier.go`, `internal/domain/observation.go`
  - **Wiki-Keywords:** tiers, tipos de observação, kebab-case

- [ ] **T007:** Implementar as categorias de erro com os textos literais (`docs/storage-and-retrieval.md` §19, `docs/wiki-files-layer.md` §5.2 passo 9)
  - **Papel:** executor
  - **Dependências:** T001
  - **Paralelizável:** true
  - **Arquivos:** `internal/domain/errors.go`
  - **Wiki-Keywords:** erros literais, colisão de caminho, estado inconsistente

- [ ] **T008:** Implementar as camadas de configuração com koanf (`docs/cli.md` §2.1, sem `.env`)
  - **Papel:** executor
  - **Dependências:** T001
  - **Paralelizável:** true
  - **Arquivos:** `internal/config/config.go`
  - **Wiki-Keywords:** INV-01, AI_MEMORY_, config.toml

- [ ] **T009:** Implementar a resolução e canonicalização do diretório de dados (INV-11, `docs/cli.md` §2.5)
  - **Papel:** executor
  - **Dependências:** T008
  - **Paralelizável:** true
  - **Arquivos:** `internal/config/datadir.go`
  - **Wiki-Keywords:** data dir, canonicalização, LOCALAPPDATA

- [ ] **T010:** Implementar o logger `slog` com filtro do próprio módulo (INV-15)
  - **Papel:** executor
  - **Dependências:** T001
  - **Paralelizável:** true
  - **Arquivos:** `internal/config/logger.go`
  - **Wiki-Keywords:** slog, INV-15, laço de realimentação

- [ ] **T011:** Escrever as migrações SQL `V<nn>__<nome>.sql` do esquema (`docs/database-schema.md`) embutidas por `embed.FS`
  - **Papel:** executor
  - **Dependências:** T001
  - **Paralelizável:** true
  - **Arquivos:** `internal/storage/migrations/*.sql`, `internal/storage/migrations/embed.go`
  - **Wiki-Keywords:** database schema, FTS5, migrações

- [ ] **T012:** Implementar ADR-002: runner de migrações com `refinery_schema_history` e erro de schema à frente
  - **Papel:** executor
  - **Dependências:** T011, T007
  - **Paralelizável:** false
  - **Arquivos:** `internal/storage/migrate.go`
  - **Wiki-Keywords:** refinery_schema_history, schema à frente, sem cópia antes de migrar

- [ ] **T013:** Implementar a abertura do SQLite (pragmas, WAL, `journal_size_limit` 64 MiB)
  - **Papel:** executor
  - **Dependências:** T012
  - **Paralelizável:** false
  - **Arquivos:** `internal/storage/open.go`
  - **Wiki-Keywords:** abertura do armazenamento, pragmas, WAL

## Fase 2: Implementação

- [ ] **T014:** Implementar ADR-003: sanitizador com os 27 padrões em RE2 com classes Unicode e fronteira verificada em código
  - **Papel:** executor
  - **Dependências:** T001
  - **Paralelizável:** true
  - **Arquivos:** `internal/domain/sanitize.go`, `internal/domain/sanitize_patterns.go`
  - **Wiki-Keywords:** sanitizador, padrões de segredo, allowlist

- [ ] **T015:** Implementar o tipo sanitizado de observação com tetos (título 80, corpo 16 KiB, cabeça abaixo de 64 bytes)
  - **Papel:** executor
  - **Dependências:** T014, T006
  - **Paralelizável:** true
  - **Arquivos:** `internal/domain/sanitized.go`
  - **Wiki-Keywords:** INV-06, barreira de tipo, truncated

- [ ] **T016:** Implementar ADR-001: escritor único (goroutine, fila 1024, descarte no encerramento, ocupação)
  - **Papel:** executor
  - **Dependências:** T013, T007
  - **Paralelizável:** true
  - **Arquivos:** `internal/storage/writer.go`
  - **Wiki-Keywords:** escritor único, INV-02, writer actor

- [ ] **T017:** Implementar o pool de leitura somente leitura de 4 conexões
  - **Papel:** executor
  - **Dependências:** T013, T007
  - **Paralelizável:** true
  - **Arquivos:** `internal/storage/reader.go`
  - **Wiki-Keywords:** pool de leitura, busy timeout, pânico no pool

- [ ] **T018:** Implementar o resolvedor central de escopo (leitura sem criar, origens `resolved_by`, erro de entrada)
  - **Papel:** executor
  - **Dependências:** T016, T017, T003
  - **Paralelizável:** true
  - **Arquivos:** `internal/storage/scope.go`
  - **Wiki-Keywords:** resolução de escopo, resolved_by, falha fechada

- [ ] **T019:** Implementar o upsert de página (casos A/B/C, supersessão, FTS, entidades e links na mesma transação)
  - **Papel:** executor
  - **Dependências:** T016, T005, T003
  - **Paralelizável:** true
  - **Arquivos:** `internal/storage/pages.go`
  - **Wiki-Keywords:** upsert, supersessão, INV-03

- [ ] **T020:** Implementar a remoção por caminho com versão esperada
  - **Papel:** executor
  - **Dependências:** T016
  - **Paralelizável:** true
  - **Arquivos:** `internal/storage/pages_delete.go`
  - **Wiki-Keywords:** exclusão de página, versão esperada

- [ ] **T021:** Implementar a busca híbrida RRF (k = 60) com FTS, entidades, vizinhos de link e autoridade `[0,55; 1,50]`
  - **Papel:** executor
  - **Dependências:** T017, T019
  - **Paralelizável:** true
  - **Arquivos:** `internal/storage/search.go`
  - **Wiki-Keywords:** RRF, autoridade, palavras de parada

- [ ] **T022:** Implementar o recurso a observações cruas (`raw_hits`)
  - **Papel:** executor
  - **Dependências:** T021
  - **Paralelizável:** true
  - **Arquivos:** `internal/storage/search_raw.go`
  - **Wiki-Keywords:** raw_hits, observações cruas

- [ ] **T023:** Implementar o reforço de acesso throttled (1/min por página e operador, anônimo incluso)
  - **Papel:** executor
  - **Dependências:** T016
  - **Paralelizável:** true
  - **Arquivos:** `internal/storage/reinforce.go`
  - **Wiki-Keywords:** reforço de acesso, throttle

- [ ] **T024:** Implementar o primitivo de cópia online do SQLite
  - **Papel:** executor
  - **Dependências:** T013
  - **Paralelizável:** true
  - **Arquivos:** `internal/storage/backup.go`
  - **Wiki-Keywords:** backup online, snapshot

- [ ] **T025:** Implementar ADR-004 (leitura): frontmatter via `yaml.Node` com resolução de escalares própria
  - **Papel:** executor
  - **Dependências:** T001
  - **Paralelizável:** true
  - **Arquivos:** `internal/wiki/frontmatter_read.go`
  - **Wiki-Keywords:** frontmatter, leitura do YAML, CRLF

- [ ] **T026:** Implementar ADR-004 (emissão): frontmatter canônico byte a byte
  - **Papel:** executor
  - **Dependências:** T025
  - **Paralelizável:** true
  - **Arquivos:** `internal/wiki/frontmatter_emit.go`
  - **Wiki-Keywords:** emissão, ordem de chaves, generated.at

- [ ] **T027:** Implementar a escrita atômica (temporário `.ai-memory-tmp.`, fsync, rename, repetição 32/33 no Windows)
  - **Papel:** executor
  - **Dependências:** T001
  - **Paralelizável:** true
  - **Arquivos:** `internal/wiki/atomic.go`, `internal/wiki/atomic_windows.go`, `internal/wiki/atomic_other.go`
  - **Wiki-Keywords:** escrita atômica, INV-10, sharing violation

- [ ] **T028:** Implementar ADR-006: adaptador Git (go-git, fallback `git` só no Windows, confinamento, registro de caminhos)
  - **Papel:** executor
  - **Dependências:** T001
  - **Paralelizável:** true
  - **Arquivos:** `internal/wiki/git.go`, `internal/wiki/git_cli_windows.go`, `internal/wiki/git_cli_other.go`
  - **Wiki-Keywords:** go-git, fallback git CLI, autor fixo

- [ ] **T029:** Implementar os manifestos `_meta.md` de workspace e projeto
  - **Papel:** executor
  - **Dependências:** T026, T027
  - **Paralelizável:** true
  - **Arquivos:** `internal/wiki/meta.go`
  - **Wiki-Keywords:** _meta.md, manifesto de escopo

- [ ] **T030:** Implementar ADR-005: `WritePage` (sem criar escopo, sanitização, rollback, sem commit nem ledger)
  - **Papel:** executor
  - **Dependências:** T014, T018, T019, T026, T027, T028, T029
  - **Paralelizável:** false
  - **Arquivos:** `internal/wiki/write.go`
  - **Wiki-Keywords:** escrita de página, rollback, estado inconsistente

- [ ] **T031:** Implementar `DeletePath` com quarentena `.ai-memory-delete.*` e guarda compartilhada
  - **Papel:** executor
  - **Dependências:** T020, T027, T028
  - **Paralelizável:** true
  - **Arquivos:** `internal/wiki/delete.go`
  - **Wiki-Keywords:** quarentena, remoção, guarda compartilhada

- [ ] **T032:** Implementar a reindexação de uma página a partir do disco (sem higienizar, tier padrão `semantic`)
  - **Papel:** executor
  - **Dependências:** T025, T019
  - **Paralelizável:** true
  - **Arquivos:** `internal/wiki/reindex_page.go`
  - **Wiki-Keywords:** reindexação, conteúdo idêntico, tier estrito

- [ ] **T033:** Implementar ADR-008 (watcher): fsnotify com debounce de 300 ms e filtros de §8.1
  - **Papel:** executor
  - **Dependências:** T032
  - **Paralelizável:** true
  - **Arquivos:** `internal/wiki/watcher.go`
  - **Wiki-Keywords:** watcher, debounce, eventos ignorados

- [ ] **T034:** Implementar ADR-008 (reconciliação): passada de 30 s, órfãos pulados, disjuntor de 5 falhas
  - **Papel:** executor
  - **Dependências:** T032
  - **Paralelizável:** true
  - **Arquivos:** `internal/wiki/reconcile.go`
  - **Wiki-Keywords:** reconciliação, watcher_degraded, órfão

- [ ] **T035:** Implementar o `reindex` completo com guarda de processo vivo e recriação do escopo a partir do `_meta.md`
  - **Papel:** executor
  - **Dependências:** T032, T029
  - **Paralelizável:** true
  - **Arquivos:** `internal/wiki/reindex.go`
  - **Wiki-Keywords:** reindex, INV-09, manifesto ausente

## Fase 3: Validação

- [ ] **T036:** Testar `PagePath`, `ensure_portable` e chave portável
  - **Papel:** executor
  - **Dependências:** T004, T005
  - **Paralelizável:** true
  - **Arquivos:** `internal/domain/pagepath_test.go`
  - **Wiki-Keywords:** ensure_portable, chave portável

- [ ] **T037:** Testar o sanitizador contra as fixtures (cenários "Segredo no corpo de uma página…" e "Segredo numa observação…")
  - **Papel:** executor
  - **Dependências:** T015
  - **Paralelizável:** true
  - **Arquivos:** `internal/domain/sanitize_test.go`
  - **Wiki-Keywords:** fixtures de sanitização, Bearer, Unicode

- [ ] **T038:** Testar frontmatter contra os casos dourados de `docs/wiki-files-layer.md` §3.2 e §3.3
  - **Papel:** executor
  - **Dependências:** T026
  - **Paralelizável:** true
  - **Arquivos:** `internal/wiki/frontmatter_test.go`
  - **Wiki-Keywords:** casos dourados, YAML

- [ ] **T039:** Testar a escrita de página (cenários: arquivo e índice juntos; não cria escopo; conteúdo idêntico; fixação não herdada; versão corrente após supersessão)
  - **Papel:** executor
  - **Dependências:** T030
  - **Paralelizável:** true
  - **Arquivos:** `internal/wiki/write_test.go`
  - **Wiki-Keywords:** escrita de página, supersessão, pinned

- [ ] **T040:** Testar o tratamento de erro: falha no banco após instalar o arquivo reverte a página
  - **Papel:** executor
  - **Dependências:** T030
  - **Paralelizável:** true
  - **Arquivos:** `internal/wiki/write_rollback_test.go`
  - **Wiki-Keywords:** rollback, estado inconsistente

- [ ] **T041:** Testar a resolução de escopo (cenários: escopo explícito inexistente; leitura sem escopo; workspace sem projeto)
  - **Papel:** executor
  - **Dependências:** T018
  - **Paralelizável:** true
  - **Arquivos:** `internal/storage/scope_test.go`
  - **Wiki-Keywords:** falha fechada, resolved_by

- [ ] **T042:** Testar a busca (cenários: RRF com autoridade; tipo por kind ou prefixo), `raw_hits`, reforço e desempenho com 5 mil páginas < 100 ms
  - **Papel:** executor
  - **Dependências:** T022, T023
  - **Paralelizável:** true
  - **Arquivos:** `internal/storage/search_test.go`
  - **Wiki-Keywords:** RRF, fixtures de busca, desempenho

- [ ] **T043:** Testar multi-sessão: duas sessões escrevem o mesmo caminho ao mesmo tempo (INV-16)
  - **Papel:** executor
  - **Dependências:** T030
  - **Paralelizável:** true
  - **Arquivos:** `internal/wiki/multisession_test.go`
  - **Wiki-Keywords:** INV-16, escrita concorrente

- [ ] **T044:** Testar watcher e reconciliação (cenários: edição manual; extensão em maiúsculas; diretório sem linha no banco)
  - **Papel:** executor
  - **Dependências:** T033, T034
  - **Paralelizável:** true
  - **Arquivos:** `internal/wiki/watcher_test.go`
  - **Wiki-Keywords:** watcher, reconciliação, órfão

- [ ] **T045:** Testar o `reindex` (banco ausente, tabelas com linhas, manifesto ausente, processo vivo)
  - **Papel:** executor
  - **Dependências:** T035
  - **Paralelizável:** true
  - **Arquivos:** `internal/wiki/reindex_test.go`
  - **Wiki-Keywords:** reindex, manifesto ausente

- [ ] **T046:** Testar a remoção com quarentena e versão esperada divergente
  - **Papel:** executor
  - **Dependências:** T031
  - **Paralelizável:** true
  - **Arquivos:** `internal/wiki/delete_test.go`
  - **Wiki-Keywords:** quarentena, versão esperada

- [ ] **T047:** Criar os testes de contrato `TestINVnn_...` para INV-01..06, 09..11, 13, 15 e 16
  - **Papel:** executor
  - **Dependências:** T009, T010, T015, T024, T030, T035
  - **Paralelizável:** true
  - **Arquivos:** `internal/cli/inv_contract_test.go`
  - **Wiki-Keywords:** invariantes, teste de contrato

- [ ] **T048:** Rodar o portão final: build cruzado `CGO_ENABLED=0` (windows/linux), `go vet`, `go test -race`, `golangci-lint`, `gofmt -l`
  - **Papel:** executor
  - **Dependências:** T002, T036, T037, T038, T039, T040, T041, T042, T043, T044, T045, T046, T047
  - **Paralelizável:** false
  - **Arquivos:** —
  - **Wiki-Keywords:** portões de qualidade, race, lint

## Fase 4: Documentação

- [ ] **T049:** Atualizar o Status do `spec.md` (plano e tasks existentes)
  - **Papel:** executor
  - **Dependências:** T048
  - **Paralelizável:** false
  - **Arquivos:** `specs/features/001-nucleo/spec.md`
  - **Wiki-Keywords:** status da spec
