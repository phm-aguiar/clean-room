---
graph-operators: enabled
heartbeat-threshold: 4
max-rounds: 40
---

# tasks.md: Ciclo de vida dos dados

> Feature `003-ciclo-de-vida` · Spec: `spec.md` (draft) · Plano: `plan.md` (R-01 a R-12)
>
> **Provisório:** gerado em 2026-10-13, antes da execução da 002. Nomes de arquivo e de helper da 002 vêm do `tasks.md` dela; a T001 reconfere tudo contra o código entregue antes de qualquer execução. Regras: uma ação por task; implementação e `_test.go` contam como um grupo. Só a T030 edita `internal/cli/httpserver.go` e só a T036 edita `internal/cli/root.go`. IDs T050 a T052 foram acrescentados depois (merge-workspace e compact) para não renumerar.


## Fase 0: Descoberta

- [ ] **T001:** Reconferir este DAG contra o código entregue pela 002 (nomes de arquivo, rotas, helpers de escopo, git e ponteiro) e ajustar as tasks antes de executar
  - **Papel:** analyst
  - **agent:** analyst1
  - **Dependências:** Nenhuma
  - **depends_on:** []
  - **Paralelizável:** false
  - **Arquivos:** `specs/features/003-ciclo-de-vida/notes/phase0-findings.md`
  - **Wiki-Keywords:** lifecycle-ops, 002 entregue, nomes reais


## Fase 1: Fundação

- [ ] **T002:** Criar o arquivo .feature da 003 com os cenários da spec
  - **Papel:** executor
  - **agent:** executor1
  - **Dependências:** T001
  - **depends_on:** [T001]
  - **Paralelizável:** true
  - **Arquivos:** `specs/features/003-ciclo-de-vida/acceptance/ciclo-de-vida.feature`
  - **Wiki-Keywords:** Gherkin, cenários, lifecycle-ops

- [ ] **T003:** Criar a trava exclusiva de mutação da wiki (leitura para escritas comuns, exclusiva para purga e movimentos), com teste
  - **Papel:** executor
  - **agent:** executor2
  - **Dependências:** T001
  - **depends_on:** [T001]
  - **Paralelizável:** true
  - **Arquivos:** `internal/wiki/mutationlock.go`, `internal/wiki/mutationlock_test.go`
  - **Wiki-Keywords:** trava exclusiva de mutação, leitor-escritor, ordem de aquisição

- [ ] **T004:** Fazer escrita de página, exclusão e reindex do watcher pegarem o lado de leitura da trava de mutação, antes das travas por página
  - **Papel:** executor
  - **agent:** executor2
  - **Dependências:** T003
  - **depends_on:** [T003]
  - **Paralelizável:** false
  - **Arquivos:** `internal/wiki/write.go`, `internal/wiki/delete.go`, `internal/wiki/watcher.go`
  - **Wiki-Keywords:** trava exclusiva de mutação, escrita de página, watcher

- [ ] **T005:** Criar a gravação e a consulta de tombstones de purga (sessão e escopo), com teste
  - **Papel:** executor
  - **agent:** executor3
  - **Dependências:** T001
  - **depends_on:** [T001]
  - **Paralelizável:** true
  - **Arquivos:** `internal/storage/tombstones.go`, `internal/storage/tombstones_test.go`
  - **Wiki-Keywords:** purged_sessions, purged_scopes, tombstone de purga


## Fase 2: Implementação

### 2A Storage

- [ ] **T006:** Criar a purga de projeto em transação única (suporte a `project` ou `project_id`, contagens de prévia, cascata incluindo `profile_candidates` e `ingest_keys`, colaterais, tombstone de escopo e auditoria), com teste
  - **Papel:** executor
  - **agent:** executor3
  - **Dependências:** T005
  - **depends_on:** [T005]
  - **Paralelizável:** true
  - **Arquivos:** `internal/storage/lifecycle_purge_project.go`, `internal/storage/lifecycle_purge_project_test.go`
  - **Wiki-Keywords:** purge-project, cascata, colaterais, auditoria

- [ ] **T007:** Criar a purga de sessão (resolução de escopo por `session_id`, prévia somente-leitura via `COUNT(*)`, exclusão transacional de `summary_page_id`, `pages` em `sessions/<session_id>.md` e frontmatter `session_id` no escopo, `profile_candidates`, `ingest_keys`, colaterais, tombstone e auditoria), com teste
  - **Papel:** executor
  - **agent:** executor1
  - **Dependências:** T005
  - **depends_on:** [T005]
  - **Paralelizável:** true
  - **Arquivos:** `internal/storage/lifecycle_purge_session.go`, `internal/storage/lifecycle_purge_session_test.go`
  - **Wiki-Keywords:** purge-session, por id, marca de sessão, colaterais

- [ ] **T008:** Criar a exclusão de workspace (exige `confirm: true` quando `dry_run: false`, checagem de não vazio 409 mesmo em `dry_run: true` sem `force: true`, cascata por projeto e tombstone do workspace inteiro), com teste
  - **Papel:** executor
  - **agent:** executor2
  - **Dependências:** T005
  - **depends_on:** [T005]
  - **Paralelizável:** true
  - **Arquivos:** `internal/storage/lifecycle_delete_workspace.go`, `internal/storage/lifecycle_delete_workspace_test.go`
  - **Wiki-Keywords:** delete-workspace, não vazio, tombstone de workspace

- [ ] **T009:** Criar a compactação no escritor (reconstrução dos três índices FTS e `VACUUM`), com teste
  - **Papel:** executor
  - **agent:** executor3
  - **Dependências:** T001
  - **depends_on:** [T001]
  - **Paralelizável:** true
  - **Arquivos:** `internal/storage/compact.go`, `internal/storage/compact_test.go`
  - **Wiki-Keywords:** compactação, rebuild FTS, VACUUM

- [ ] **T010:** Criar a renomeação de projeto (`from` ou `project_id`) e de workspace (200 no-op se `from == to`, 422 para nome inválido ou `_profile`/`_profile.*`, 409 Conflict se destino já existe), com teste
  - **Papel:** executor
  - **agent:** executor1
  - **Dependências:** T001
  - **depends_on:** [T001]
  - **Paralelizável:** true
  - **Arquivos:** `internal/storage/lifecycle_rename.go`, `internal/storage/lifecycle_rename_test.go`
  - **Wiki-Keywords:** rename-project, rename-workspace, validação de nome

- [ ] **T011:** Criar a troca de `workspace_id` do movimento real de projeto em transação única (com rollback do banco e remoção da entrada de `audit_log` se a renomeação do diretório em disco falhar depois), com teste
  - **Papel:** executor
  - **agent:** executor2
  - **Dependências:** T001
  - **depends_on:** [T001]
  - **Paralelizável:** true
  - **Arquivos:** `internal/storage/lifecycle_move_project.go`, `internal/storage/lifecycle_move_project_test.go`
  - **Wiki-Keywords:** move-project, re-stamp de workspace, true move

- [ ] **T012:** Criar o movimento de sessão (re-stamp de `sessions`, `observations`, `handoffs`, `profile_candidates` e `ingest_keys`, páginas `move|keep|regenerate`, `page_conflict = true` sem 409 se destino já existir, `source_scopes` e simulação), com teste
  - **Papel:** executor
  - **agent:** executor3
  - **Dependências:** T001
  - **depends_on:** [T001]
  - **Paralelizável:** true
  - **Arquivos:** `internal/storage/lifecycle_move_session.go`, `internal/storage/lifecycle_move_session_test.go`
  - **Wiki-Keywords:** move-session, source_scopes, re-home, regenerate

- [ ] **T013:** Criar a expiração em lote dos handoffs abertos de um escopo (com suporte a `older_than_days`, `dry_run`, filtro de proprietário do chamador e registro em `audit_log`), com teste
  - **Papel:** executor
  - **agent:** executor1
  - **Dependências:** T001
  - **depends_on:** [T001]
  - **Paralelizável:** true
  - **Arquivos:** `internal/storage/handoffs_expire.go`, `internal/storage/handoffs_expire_test.go`
  - **Wiki-Keywords:** handoffs expire-all, expired


### 2B Wiki

- [ ] **T014:** Criar a remoção da árvore de um escopo e de arquivos de página sob a trava exclusiva, com relato de `files_deleted` e `files_failed`, com teste
  - **Papel:** executor
  - **agent:** executor2
  - **Dependências:** T004
  - **depends_on:** [T004]
  - **Paralelizável:** true
  - **Arquivos:** `internal/wiki/scope_tree.go`, `internal/wiki/scope_tree_test.go`
  - **Wiki-Keywords:** remoção de árvore, files_failed, trava exclusiva

- [ ] **T015:** Criar o movimento do diretório de projeto para outro workspace após o commit no banco, acionando a reversão do banco em caso de falha de I/O, com teste
  - **Papel:** executor
  - **agent:** executor3
  - **Dependências:** T004
  - **depends_on:** [T004]
  - **Paralelizável:** true
  - **Arquivos:** `internal/wiki/move_tree.go`, `internal/wiki/move_tree_test.go`
  - **Wiki-Keywords:** move-project, renomear diretório, rollback

- [ ] **T016:** Criar a restauração de página a partir de um checkpoint git (`rev`: validação de `path` antes do escopo, leitura, escrita atômica, reindex), com teste
  - **Papel:** executor
  - **agent:** executor1
  - **Dependências:** T004
  - **depends_on:** [T004]
  - **Paralelizável:** true
  - **Arquivos:** `internal/wiki/restore_page.go`, `internal/wiki/restore_page_test.go`
  - **Wiki-Keywords:** restore-page, checkpoint, git show

- [ ] **T017:** Criar a listagem de checkpoints, o commit manual e o checkpoint `upgrade baseline` de wiki sem commits, com teste
  - **Papel:** executor
  - **agent:** executor2
  - **Dependências:** T001
  - **depends_on:** [T001]
  - **Paralelizável:** true
  - **Arquivos:** `internal/wiki/checkpoints.go`, `internal/wiki/checkpoints_test.go`
  - **Wiki-Keywords:** checkpoints, commit manual, upgrade baseline

- [ ] **T018:** Criar a cópia de páginas da fusão (comparação de conflito e políticas `block`, `overwrite` e `duplicate` com sufixos `-from-<slug>`, `-1`, `-2`, ... até `-9999`) pelo `WritePage`, com teste
  - **Papel:** executor
  - **agent:** executor3
  - **Dependências:** T004
  - **depends_on:** [T004]
  - **Paralelizável:** true
  - **Arquivos:** `internal/wiki/merge_copy.go`, `internal/wiki/merge_copy_test.go`
  - **Wiki-Keywords:** fusão, on_conflict, block, overwrite, duplicate

- [ ] **T019:** Criar a reescrita dos manifestos de workspace e projeto após renomear ou mover, com teste
  - **Papel:** executor
  - **agent:** executor1
  - **Dependências:** T001
  - **depends_on:** [T001]
  - **Paralelizável:** true
  - **Arquivos:** `internal/wiki/manifest_refresh.go`, `internal/wiki/manifest_refresh_test.go`
  - **Wiki-Keywords:** _meta.md, refresh de manifestos, manifest_warning


### 2C Tombstones nos caminhos de entrada

- [ ] **T020:** Fazer a ingestão descartar evento de sessão purgada e não recriar escopo purgado, com teste
  - **Papel:** executor
  - **agent:** executor2
  - **Dependências:** T005
  - **depends_on:** [T005]
  - **Paralelizável:** true
  - **Arquivos:** `internal/ingest/tombstone_check.go`, `internal/ingest/tombstone_check_test.go`, `internal/ingest/process.go`, `internal/ingest/resolve.go`
  - **Wiki-Keywords:** tombstone, hook atrasado, ressuscitar

- [ ] **T021:** Fazer reindex e reconciliação pularem escopo tombstoned e o watcher ignorar a página de sessão purgada, com teste
  - **Papel:** executor
  - **agent:** executor3
  - **Dependências:** T005, T004
  - **depends_on:** [T005, T004]
  - **Paralelizável:** false
  - **Arquivos:** `internal/wiki/reindex.go`, `internal/wiki/reindex_test.go`, `internal/wiki/watcher.go`, `internal/wiki/watcher_test.go`
  - **Wiki-Keywords:** reindex, watcher, escopo tombstoned, sessão purgada


### 2D Rotas administrativas

- [ ] **T022:** Criar os efeitos depois das operações (limpar ou retargetar o ponteiro de projeto ativo e invalidar o cache de resolução do hook), com teste
  - **Papel:** executor
  - **agent:** executor1
  - **Dependências:** T001
  - **depends_on:** [T001]
  - **Paralelizável:** true
  - **Arquivos:** `internal/cli/admin_after.go`, `internal/cli/admin_after_test.go`
  - **Wiki-Keywords:** ponteiro ativo, cache de escopo, invalidação

- [ ] **T023:** Criar a rota `POST /admin/purge-project` na ordem do doc (incluindo checagem 409 de lease vivo mesmo em `dry_run: true` sem `force: true`), com teste
  - **Papel:** executor
  - **agent:** executor2
  - **Dependências:** T006, T009, T014, T017, T022
  - **depends_on:** [T006, T009, T014, T017, T022]
  - **Paralelizável:** true
  - **Arquivos:** `internal/cli/admin_purge_project.go`, `internal/cli/admin_purge_project_test.go`
  - **Wiki-Keywords:** purge-project, lease, dry_run, compact

- [ ] **T024:** Criar a rota `POST /admin/purge-session` na ordem do doc (recebendo `{session_id, confirm, compact, dry_run}`), com teste
  - **Papel:** executor
  - **agent:** executor3
  - **Dependências:** T007, T009, T014, T017, T022
  - **depends_on:** [T007, T009, T014, T017, T022]
  - **Paralelizável:** true
  - **Arquivos:** `internal/cli/admin_purge_session.go`, `internal/cli/admin_purge_session_test.go`
  - **Wiki-Keywords:** purge-session, trava exclusiva, files_failed

- [ ] **T025:** Criar a rota `POST /admin/delete-workspace` na ordem do doc (`confirm` obrigatório quando `dry_run: false`; 409 se não vazio sem `force: true` mesmo em `dry_run: true`), com teste
  - **Papel:** executor
  - **agent:** executor1
  - **Dependências:** T008, T009, T014, T017, T022
  - **depends_on:** [T008, T009, T014, T017, T022]
  - **Paralelizável:** true
  - **Arquivos:** `internal/cli/admin_delete_workspace.go`, `internal/cli/admin_delete_workspace_test.go`
  - **Wiki-Keywords:** delete-workspace, force, dry_run

- [ ] **T026:** Criar as rotas `POST /admin/rename-project` e `POST /admin/rename-workspace` (409 quando `to` já existe; 200 no-op quando `from == to`), com teste
  - **Papel:** executor
  - **agent:** executor2
  - **Dependências:** T010, T017, T019, T022
  - **depends_on:** [T010, T017, T019, T022]
  - **Paralelizável:** true
  - **Arquivos:** `internal/cli/admin_rename.go`, `internal/cli/admin_rename_test.go`
  - **Wiki-Keywords:** rename-project, rename-workspace, manifest_warning

- [ ] **T027:** Criar a rota `POST /admin/move-project` (movimento real com banco primeiro e fusão por cópia e purga com commit `"move-project-partial ..."` quando `pages_skipped` não for vazio), com teste
  - **Papel:** executor
  - **agent:** executor3
  - **Dependências:** T006, T011, T015, T017, T018, T019, T022
  - **depends_on:** [T006, T011, T015, T017, T018, T019, T022]
  - **Paralelizável:** true
  - **Arquivos:** `internal/cli/admin_move_project.go`, `internal/cli/admin_move_project_test.go`
  - **Wiki-Keywords:** move-project, moved_via, copy-purge

- [ ] **T028:** Criar a rota `POST /admin/move-session` (`session_id` único e `session_ids[]` em lote, `to_workspace`, `to_project`, `create`, `pages: move|keep|regenerate`, `confirm`), com teste
  - **Papel:** executor
  - **agent:** executor1
  - **Dependências:** T012, T017, T022
  - **depends_on:** [T012, T017, T022]
  - **Paralelizável:** true
  - **Arquivos:** `internal/cli/admin_move_session.go`, `internal/cli/admin_move_session_test.go`
  - **Wiki-Keywords:** move-session, lote, simulação

- [ ] **T029:** Criar as rotas `POST /admin/handoffs/expire`, `GET /admin/checkpoints`, `POST /admin/restore-page` (campo JSON `rev`) e `POST /admin/commit` (campo opcional `message`), com teste
  - **Papel:** executor
  - **agent:** executor2
  - **Dependências:** T013, T016, T017
  - **depends_on:** [T013, T016, T017]
  - **Paralelizável:** true
  - **Arquivos:** `internal/cli/admin_wiki_ops.go`, `internal/cli/admin_wiki_ops_test.go`
  - **Wiki-Keywords:** handoffs expire, checkpoints, restore-page, commit

- [ ] **T050:** Criar a rota `POST /admin/merge-workspace` (422 se `from == to`, filtra `_profile`/`_profile.*`, move cada projeto e aborta com `failed_project`/`reason`/`projects_merged` em erro parcial), com teste
  - **Papel:** executor
  - **agent:** executor1
  - **Dependências:** T027
  - **depends_on:** [T027]
  - **Paralelizável:** true
  - **Arquivos:** `internal/cli/admin_merge_workspace.go`, `internal/cli/admin_merge_workspace_test.go`
  - **Wiki-Keywords:** merge-workspace, move-project, source_workspace_deleted

- [ ] **T051:** Criar a rota `POST /admin/compact` (compactação isolada aceitando `{}` via HTTP e retornando `{bytes_before, bytes_after, bytes_reclaimed}`), com teste
  - **Papel:** executor
  - **agent:** executor2
  - **Dependências:** T009
  - **depends_on:** [T009]
  - **Paralelizável:** true
  - **Arquivos:** `internal/cli/admin_compact.go`, `internal/cli/admin_compact_test.go`
  - **Wiki-Keywords:** compact, bytes_reclaimed, confirm

- [ ] **T030:** Montar as rotas da 003 no servidor HTTP
  - **Papel:** executor
  - **agent:** executor3
  - **Dependências:** T023, T024, T025, T026, T027, T028, T029, T050, T051
  - **depends_on:** [T023, T024, T025, T026, T027, T028, T029, T050, T051]
  - **Paralelizável:** false
  - **Arquivos:** `internal/cli/httpserver.go`
  - **Wiki-Keywords:** rotas de administração, montagem


### 2E CLI

- [ ] **T031:** Criar os comandos `purge-project` (com `--project` ou `--project-id`) e `purge-session` (prévia limitada a 5 s, recusa sem `--confirm`), com teste
  - **Papel:** executor
  - **agent:** executor1
  - **Dependências:** T030
  - **depends_on:** [T030]
  - **Paralelizável:** true
  - **Arquivos:** `internal/cli/purge_cmd.go`, `internal/cli/purge_cmd_test.go`
  - **Wiki-Keywords:** purge-project, purge-session, prévia de 5 s

- [ ] **T032:** Criar os comandos `rename-project` (`--from` ou `--project-id`), `move-project` (`--project` ou `--project-id`), `move-session` (resolvendo `--from-project` via `GET /admin/sessions`), `handoffs --expire-all [--older-than-days]`, `checkpoints`, `restore-page --from <rev>`, `commit` e `compact --confirm`, com teste
  - **Papel:** executor
  - **agent:** executor2
  - **Dependências:** T030
  - **depends_on:** [T030]
  - **Paralelizável:** true
  - **Arquivos:** `internal/cli/lifecycle_cmds.go`, `internal/cli/lifecycle_cmds_test.go`
  - **Wiki-Keywords:** rename-project, move-project, move-session, handoffs, checkpoints, restore-page, commit, compact

- [ ] **T033:** Criar o comando direto `restore --from --force` (allowlist de entradas incluindo GNU Sparse `'S'`, fechamento explícito do SQLite de staging antes do rename no Windows, troca reversível via `.restore-previous-*`), com teste
  - **Papel:** executor
  - **agent:** executor3
  - **Dependências:** T001
  - **depends_on:** [T001]
  - **Paralelizável:** true
  - **Arquivos:** `internal/cli/restore_cmd.go`, `internal/cli/restore_cmd_test.go`
  - **Wiki-Keywords:** restore, estágio, troca reversível, tarball

- [ ] **T034:** Criar o comando direto `reset --confirm`, com teste
  - **Papel:** executor
  - **agent:** executor1
  - **Dependências:** T001
  - **depends_on:** [T001]
  - **Paralelizável:** true
  - **Arquivos:** `internal/cli/reset_cmd.go`, `internal/cli/reset_cmd_test.go`
  - **Wiki-Keywords:** reset, dry-run, processo vivo

- [ ] **T035:** Criar o comando `uninstall` simétrico ao `install-hooks` e `install-mcp` do Claude Code, com `--purge-data` vs `--keep-data`, `--yes` e `refuse_ambient_data_dir`, com teste
  - **Papel:** executor
  - **agent:** executor2
  - **Dependências:** T001
  - **depends_on:** [T001]
  - **Paralelizável:** true
  - **Arquivos:** `internal/cli/uninstall_cmd.go`, `internal/cli/uninstall_cmd_test.go`
  - **Wiki-Keywords:** uninstall, purge-data, simetria

- [ ] **T036:** Registrar na raiz da CLI os comandos da 003
  - **Papel:** executor
  - **agent:** executor3
  - **Dependências:** T031, T032, T033, T034, T035
  - **depends_on:** [T031, T032, T033, T034, T035]
  - **Paralelizável:** false
  - **Arquivos:** `internal/cli/root.go`
  - **Wiki-Keywords:** cobra, raiz da CLI


### 2F Cenários Gherkin

- [ ] **T037:** Implementar Scenario: purga de sessão com prévia, confirmação e tombstone contra hook atrasado, reindex e watcher
  - **Papel:** executor
  - **agent:** executor1
  - **Dependências:** T036, T020, T021, T002
  - **depends_on:** [T036, T020, T021, T002]
  - **Paralelizável:** true
  - **Arquivos:** `test/acceptance/purge_session_test.go`
  - **Wiki-Keywords:** purge-session, tombstone, hook atrasado

- [ ] **T038:** Implementar tratamento de erro: sem confirmação, escopo inexistente, checkpoint prévio falho e remoção de arquivo falha
  - **Papel:** executor
  - **agent:** executor2
  - **Dependências:** T036, T002
  - **depends_on:** [T036, T002]
  - **Paralelizável:** true
  - **Arquivos:** `test/acceptance/lifecycle_failures_test.go`
  - **Wiki-Keywords:** 400, 404, 500, files_failed

- [ ] **T039:** Implementar tratamento de erro: purga de projeto com execução gerenciada de lease vivo (409 inclusive em `dry_run: true`) e com `--force`
  - **Papel:** executor
  - **agent:** executor3
  - **Dependências:** T023, T002
  - **depends_on:** [T023, T002]
  - **Paralelizável:** true
  - **Arquivos:** `test/acceptance/purge_project_test.go`
  - **Wiki-Keywords:** purge-project, lease vivo, force

- [ ] **T040:** Implementar edge case: movimento real preserva id e cadeia de supersessão (banco primeiro + rollback se disco falhar); fusão com `block`, `overwrite` e `duplicate` (`-from-<slug>`, `-1`, `-2`)
  - **Papel:** executor
  - **agent:** executor1
  - **Dependências:** T027, T002
  - **depends_on:** [T027, T002]
  - **Paralelizável:** true
  - **Arquivos:** `test/acceptance/move_project_test.go`
  - **Wiki-Keywords:** true move, INV-16, fusão

- [ ] **T041:** Implementar edge case: movimento de sessão espalhada, `regenerate`, `keep`, `page_conflict = true` (200 OK) e guardas
  - **Papel:** executor
  - **agent:** executor2
  - **Dependências:** T028, T002
  - **depends_on:** [T028, T002]
  - **Paralelizável:** true
  - **Arquivos:** `test/acceptance/move_session_test.go`
  - **Wiki-Keywords:** move-session, source_scopes, re-home

- [ ] **T042:** Implementar tratamento de erro: `restore` e `reset` com processo vivo, diretório ocupado e tarball malicioso
  - **Papel:** executor
  - **agent:** executor3
  - **Dependências:** T033, T034, T002
  - **depends_on:** [T033, T034, T002]
  - **Paralelizável:** true
  - **Arquivos:** `test/acceptance/restore_reset_test.go`
  - **Wiki-Keywords:** restore, tarball malicioso, reset

- [ ] **T043:** Implementar edge case: purga de sessão opera estritamente no escopo `(workspace_id, project_id)` da sessão e não afeta `sessions/<id>.md` de outro projeto
  - **Papel:** executor
  - **agent:** executor1
  - **Dependências:** T024, T002
  - **depends_on:** [T024, T002]
  - **Paralelizável:** true
  - **Arquivos:** `test/acceptance/purge_same_path_test.go`
  - **Wiki-Keywords:** mesmo caminho, por id, sessions

- [ ] **T052:** Implementar edge case: fusão de workspace com projetos em conflito e remoção do workspace vazio
  - **Papel:** executor
  - **agent:** executor3
  - **Dependências:** T050, T002
  - **depends_on:** [T050, T002]
  - **Paralelizável:** true
  - **Arquivos:** `test/acceptance/merge_workspace_test.go`
  - **Wiki-Keywords:** merge-workspace, on_conflict, workspace vazio


## Fase 3: Validação

- [ ] **T044:** Criar testes de contrato INV-09 e INV-16 da 003
  - **Papel:** executor
  - **agent:** executor2
  - **Dependências:** T036
  - **depends_on:** [T036]
  - **Paralelizável:** true
  - **Arquivos:** `test/contract/lifecycle_inv_test.go`
  - **Wiki-Keywords:** INV-09, INV-16, contrato

- [ ] **T045:** Criar o smoke test com o binário (purga, movimento, checkpoints, restore e reset)
  - **Papel:** executor
  - **agent:** executor3
  - **Dependências:** T036
  - **depends_on:** [T036]
  - **Paralelizável:** true
  - **Arquivos:** `test/smoke/lifecycle_smoke_test.go`
  - **Wiki-Keywords:** smoke, binário, purge, restore

- [ ] **T046:** Rodar os portões e registrar o resultado com a saída
  - **Papel:** executor
  - **agent:** executor1
  - **Dependências:** T037, T038, T039, T040, T041, T042, T043, T044, T045, T052
  - **depends_on:** [T037, T038, T039, T040, T041, T042, T043, T044, T045, T052]
  - **Paralelizável:** false
  - **Arquivos:** `specs/features/003-ciclo-de-vida/notes/gates-run.md`
  - **Wiki-Keywords:** check.sh, race, golangci-lint

- [ ] **T047:** Auditar a feature contra a constituição e contra `docs/lifecycle-ops.md`
  - **Papel:** analyst
  - **agent:** analyst1
  - **Dependências:** T046
  - **depends_on:** [T046]
  - **Paralelizável:** false
  - **Arquivos:** `specs/features/003-ciclo-de-vida/notes/constitution-audit.md`
  - **Wiki-Keywords:** constitution, auditoria, docs


## Fase 4: Documentação

- [ ] **T048:** Atualizar o README com as operações de ciclo de vida e a matriz de segurança
  - **Papel:** executor
  - **agent:** executor2
  - **Dependências:** T047
  - **depends_on:** [T047]
  - **Paralelizável:** true
  - **Arquivos:** `README.md`
  - **Wiki-Keywords:** README, ciclo de vida

- [ ] **T049:** Atualizar o llms.txt com os comandos e rotas da 003
  - **Papel:** executor
  - **agent:** executor3
  - **Dependências:** T048
  - **depends_on:** [T048]
  - **Paralelizável:** true
  - **Arquivos:** `llms.txt`
  - **Wiki-Keywords:** llms.txt, onboarding



## Coordination Graph

G₀ (round 0):

- **nodes:** T001, T002, T003, T004, T005, T006, T007, T008, T009, T010, T011, T012, T013, T014, T015, T016, T017, T018, T019, T020, T021, T022, T023, T024, T025, T026, T027, T028, T029, T050, T051, T030, T031, T032, T033, T034, T035, T036, T037, T038, T039, T040, T041, T042, T043, T052, T044, T045, T046, T047, T048, T049
- **edges:**
  - T001 → T002
  - T001 → T003
  - T003 → T004
  - T001 → T005
  - T005 → T006
  - T005 → T007
  - T005 → T008
  - T001 → T009
  - T001 → T010
  - T001 → T011
  - T001 → T012
  - T001 → T013
  - T004 → T014
  - T004 → T015
  - T004 → T016
  - T001 → T017
  - T004 → T018
  - T001 → T019
  - T005 → T020
  - T005 → T021
  - T004 → T021
  - T001 → T022
  - T006 → T023
  - T009 → T023
  - T014 → T023
  - T017 → T023
  - T022 → T023
  - T007 → T024
  - T009 → T024
  - T014 → T024
  - T017 → T024
  - T022 → T024
  - T008 → T025
  - T009 → T025
  - T014 → T025
  - T017 → T025
  - T022 → T025
  - T010 → T026
  - T017 → T026
  - T019 → T026
  - T022 → T026
  - T006 → T027
  - T011 → T027
  - T015 → T027
  - T017 → T027
  - T018 → T027
  - T019 → T027
  - T022 → T027
  - T012 → T028
  - T017 → T028
  - T022 → T028
  - T013 → T029
  - T016 → T029
  - T017 → T029
  - T027 → T050
  - T009 → T051
  - T023 → T030
  - T024 → T030
  - T025 → T030
  - T026 → T030
  - T027 → T030
  - T028 → T030
  - T029 → T030
  - T050 → T030
  - T051 → T030
  - T030 → T031
  - T030 → T032
  - T001 → T033
  - T001 → T034
  - T001 → T035
  - T031 → T036
  - T032 → T036
  - T033 → T036
  - T034 → T036
  - T035 → T036
  - T036 → T037
  - T020 → T037
  - T021 → T037
  - T002 → T037
  - T036 → T038
  - T002 → T038
  - T023 → T039
  - T002 → T039
  - T027 → T040
  - T002 → T040
  - T028 → T041
  - T002 → T041
  - T033 → T042
  - T034 → T042
  - T002 → T042
  - T024 → T043
  - T002 → T043
  - T050 → T052
  - T002 → T052
  - T036 → T044
  - T036 → T045
  - T037 → T046
  - T038 → T046
  - T039 → T046
  - T040 → T046
  - T041 → T046
  - T042 → T046
  - T043 → T046
  - T044 → T046
  - T045 → T046
  - T052 → T046
  - T046 → T047
  - T047 → T048
  - T048 → T049
- **assignments:**
  - T001: analyst1
  - T002: executor1
  - T003: executor2
  - T004: executor2
  - T005: executor3
  - T006: executor3
  - T007: executor1
  - T008: executor2
  - T009: executor3
  - T010: executor1
  - T011: executor2
  - T012: executor3
  - T013: executor1
  - T014: executor2
  - T015: executor3
  - T016: executor1
  - T017: executor2
  - T018: executor3
  - T019: executor1
  - T020: executor2
  - T021: executor3
  - T022: executor1
  - T023: executor2
  - T024: executor3
  - T025: executor1
  - T026: executor2
  - T027: executor3
  - T028: executor1
  - T029: executor2
  - T050: executor1
  - T051: executor2
  - T030: executor3
  - T031: executor1
  - T032: executor2
  - T033: executor3
  - T034: executor1
  - T035: executor2
  - T036: executor3
  - T037: executor1
  - T038: executor2
  - T039: executor3
  - T040: executor1
  - T041: executor2
  - T042: executor3
  - T043: executor1
  - T052: executor3
  - T044: executor2
  - T045: executor3
  - T046: executor1
  - T047: analyst1
  - T048: executor2
  - T049: executor3
- **ready:** [T001]

> Workers: Lead, analyst1, executor1..3. Validado por script: 52 tasks, sem ciclo, sem dependência quebrada, sem arquivo compartilhado entre tasks sem ordem. Mínimo de 14 rounds. Caminho crítico (calculado): T001 → T003 → T004 → T015 → T027 → T050 → T030 → T031 → T036 → T037 → T046 → T047 → T048 → T049.
