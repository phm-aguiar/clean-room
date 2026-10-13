---
graph-operators: enabled
heartbeat-threshold: 4
max-rounds: 40
---

# tasks.md: Envelhecimento e manutenção sem LLM

> Feature `004-manutencao-sem-llm` · Spec: `spec.md` (draft) · Plano: `plan.md` (M-01 a M-10)
>
> **Provisório:** gerado em 2026-10-13, antes da execução da 002 e da 003. A T001 reconfere tudo contra o código entregue antes de qualquer execução. Regras: uma ação por task; implementação e `_test.go` contam como um grupo. `internal/cli/serve_cmd.go` é editado em sequência por T028 e T031.


## Fase 0: Descoberta

- [ ] **T001:** Reconferir este DAG contra o código entregue pela 002 e pela 003 (BumpAccess, tombstones, trava de mutação, rotas) e ajustar as tasks antes de executar
  - **Papel:** analyst
  - **agent:** analyst1
  - **Dependências:** Nenhuma
  - **depends_on:** []
  - **Paralelizável:** false
  - **Arquivos:** `specs/features/004-manutencao-sem-llm/notes/phase0-findings.md`
  - **Wiki-Keywords:** design-memory-aging, 003 entregue, nomes reais

- [ ] **T002:** Extrair o contrato de `reorg` dos docs (api-reference, temporal, wiki-files-layer) e registrar o que fica como decisão nossa
  - **Papel:** researcher
  - **agent:** researcher1
  - **Dependências:** Nenhuma
  - **depends_on:** []
  - **Paralelizável:** true
  - **Arquivos:** `specs/features/004-manutencao-sem-llm/notes/reorg-contract.md`
  - **Wiki-Keywords:** reorg, cemitério, _graveyard, temporal


## Fase 1: Fundação

- [ ] **T003:** Criar o arquivo .feature da 004 com os cenários da spec
  - **Papel:** executor
  - **agent:** executor1
  - **Dependências:** T001
  - **depends_on:** [T001]
  - **Paralelizável:** true
  - **Arquivos:** `specs/features/004-manutencao-sem-llm/acceptance/manutencao.feature`
  - **Wiki-Keywords:** Gherkin, cenários de aceitação, design-memory-aging §12

- [ ] **T004:** Acrescentar as chaves `[decay]` e `[maintenance]` com padrões e validação, com teste
  - **Papel:** executor
  - **agent:** executor2
  - **Dependências:** T001
  - **depends_on:** [T001]
  - **Paralelizável:** true
  - **Arquivos:** `internal/config/config.go`, `internal/config/config_test.go`
  - **Wiki-Keywords:** [decay], [maintenance], validação na carga

- [ ] **T005:** Criar a pontuação de retenção pura (termos de tempo, acesso e amplitude; meia-vida por tier), com teste das propriedades do doc
  - **Papel:** executor
  - **agent:** executor3
  - **Dependências:** T001
  - **depends_on:** [T001]
  - **Paralelizável:** true
  - **Arquivos:** `internal/domain/maintenance/retention.go`, `internal/domain/maintenance/retention_test.go`
  - **Wiki-Keywords:** retenção, decaimento exponencial, amplitude, salience

- [ ] **T006:** Criar a classificação pura das candidatas (expirada, fria, intocada; fixação pela coluna ou pelo frontmatter), com teste
  - **Papel:** executor
  - **agent:** executor1
  - **Dependências:** T005
  - **depends_on:** [T005]
  - **Paralelizável:** true
  - **Arquivos:** `internal/domain/maintenance/classify.go`, `internal/domain/maintenance/classify_test.go`
  - **Wiki-Keywords:** classificação, expirada, fria, fixada

- [ ] **T007:** Criar a mineração de keep-tokens (sete classes em ordem, deduplicação, até 48, 2 a 128 bytes), com teste
  - **Papel:** executor
  - **agent:** executor2
  - **Dependências:** T001
  - **depends_on:** [T001]
  - **Paralelizável:** true
  - **Arquivos:** `internal/domain/maintenance/keeptokens.go`, `internal/domain/maintenance/keeptokens_test.go`
  - **Wiki-Keywords:** keep-tokens, sete classes, 48 tokens

- [ ] **T008:** Criar a construção pura da versão compactada A2 (frontmatter com `compacted: true`, L1 de 500 bytes, fatos retidos, rodapé), com teste
  - **Papel:** executor
  - **agent:** executor3
  - **Dependências:** T007
  - **depends_on:** [T007]
  - **Paralelizável:** true
  - **Arquivos:** `internal/domain/maintenance/compact_a2.go`, `internal/domain/maintenance/compact_a2_test.go`
  - **Wiki-Keywords:** compactação A2, L1, retained facts, rodapé


## Fase 2: Implementação

### 2A Storage

- [ ] **T009:** Fazer o `UpsertPage` gravar `pages.compacted_at` quando o frontmatter traz `compacted: true`, com teste
  - **Papel:** executor
  - **agent:** executor1
  - **Dependências:** T001
  - **depends_on:** [T001]
  - **Paralelizável:** true
  - **Arquivos:** `internal/storage/upsert.go`, `internal/storage/upsert_test.go`
  - **Wiki-Keywords:** compacted_at, frontmatter compacted

- [ ] **T010:** Criar a marcação de tombstone de decaimento condicionada à versão esperada, fechando os vínculos de entidade, com teste
  - **Papel:** executor
  - **agent:** executor2
  - **Dependências:** T001
  - **depends_on:** [T001]
  - **Paralelizável:** true
  - **Arquivos:** `internal/storage/decay.go`, `internal/storage/decay_test.go`
  - **Wiki-Keywords:** tombstone de decaimento, id esperado, vínculos

- [ ] **T011:** Criar a remoção definitiva de tombstones mais antigos que o corte, com a ancestralidade e preservando página recriada, com teste
  - **Papel:** executor
  - **agent:** executor3
  - **Dependências:** T010
  - **depends_on:** [T010]
  - **Paralelizável:** false
  - **Arquivos:** `internal/storage/hard_delete.go`, `internal/storage/hard_delete_test.go`
  - **Wiki-Keywords:** remoção definitiva, ancestralidade, carência

- [ ] **T012:** Criar a poda de observações em lotes, só de sessões com página-resumo viva, com teste
  - **Papel:** executor
  - **agent:** executor1
  - **Dependências:** T001
  - **depends_on:** [T001]
  - **Paralelizável:** true
  - **Arquivos:** `internal/storage/observation_prune.go`, `internal/storage/observation_prune_test.go`
  - **Wiki-Keywords:** poda de observações, lotes, página viva

- [ ] **T013:** Criar o feedback de página (registro em `page_feedback` e ajuste de `salience` em passos de 0,25 entre 0,25 e 2,0), com teste
  - **Papel:** executor
  - **agent:** executor2
  - **Dependências:** T001
  - **depends_on:** [T001]
  - **Paralelizável:** true
  - **Arquivos:** `internal/storage/feedback.go`, `internal/storage/feedback_test.go`
  - **Wiki-Keywords:** page_feedback, salience, passos de 0,25

- [ ] **T014:** Criar a leitura e a gravação do último sucesso por job em `maintenance_scheduler_state`, com teste
  - **Papel:** executor
  - **agent:** executor3
  - **Dependências:** T001
  - **depends_on:** [T001]
  - **Paralelizável:** true
  - **Arquivos:** `internal/storage/scheduler_state.go`, `internal/storage/scheduler_state_test.go`
  - **Wiki-Keywords:** maintenance_scheduler_state, último sucesso

- [ ] **T015:** Criar a carga das candidatas da varredura numa consulta, com o mapa de amplitude só quando `breadth_weight > 0`, com teste
  - **Papel:** executor
  - **agent:** executor1
  - **Dependências:** T001
  - **depends_on:** [T001]
  - **Paralelizável:** true
  - **Arquivos:** `internal/storage/sweep_candidates.go`, `internal/storage/sweep_candidates_test.go`
  - **Wiki-Keywords:** candidatas, consulta única, amplitude

- [ ] **T016:** Alinhar o reforço de acesso ao doc (transação única, só versão atual, throttle de 60 s por página e operador em memória, anônimos compartilham a chave), com teste
  - **Papel:** executor
  - **agent:** executor2
  - **Dependências:** T001
  - **depends_on:** [T001]
  - **Paralelizável:** true
  - **Arquivos:** `internal/storage/access.go`, `internal/storage/access_test.go`
  - **Wiki-Keywords:** reforço de acesso, throttle em memória, page_access


### 2B Wiki e varredura

- [ ] **T017:** Criar a varredura de esquecimento por escopo (ordem de passes, guardas de seleção, aplicação em lote, relatório JSON, `dry_run`, recusa sem wiki), com teste
  - **Papel:** executor
  - **agent:** executor3
  - **Dependências:** T006, T008, T009, T010, T011, T012, T015
  - **depends_on:** [T006, T008, T009, T010, T011, T012, T015]
  - **Paralelizável:** false
  - **Arquivos:** `internal/wiki/sweep.go`, `internal/wiki/sweep_test.go`
  - **Wiki-Keywords:** varredura de esquecimento, ordem de passes, relatório

- [ ] **T018:** Criar o tombamento opcional de páginas sumidas do disco na reconciliação (duas passagens, disjuntor, reconferência sob a trava), com teste
  - **Papel:** executor
  - **agent:** executor1
  - **Dependências:** T001
  - **depends_on:** [T001]
  - **Paralelizável:** true
  - **Arquivos:** `internal/wiki/deleted_pages.go`, `internal/wiki/deleted_pages_test.go`, `internal/wiki/watcher.go`
  - **Wiki-Keywords:** tombamento de página sumida, duas passagens, disjuntor

- [ ] **T019:** Criar as regras de lint puras (`stale`, `duplicate`, `rule_suggestion`, `pinned_expiring`, `broken_link`, `feedback_flagged`, `contradiction` declarada), com teste
  - **Papel:** executor
  - **agent:** executor2
  - **Dependências:** T001
  - **depends_on:** [T001]
  - **Paralelizável:** true
  - **Arquivos:** `internal/domain/maintenance/lint_rules.go`, `internal/domain/maintenance/lint_rules_test.go`
  - **Wiki-Keywords:** lint por regra, stale, duplicate, broken_link

- [ ] **T020:** Criar a execução do lint com o relatório em `_lint/report.md` (removido sem achados) e a poda de relatórios datados, com teste
  - **Papel:** executor
  - **agent:** executor3
  - **Dependências:** T019
  - **depends_on:** [T019]
  - **Paralelizável:** false
  - **Arquivos:** `internal/wiki/lint_report.go`, `internal/wiki/lint_report_test.go`
  - **Wiki-Keywords:** _lint/report.md, poda de relatórios datados

- [ ] **T021:** Criar os sinais puros do curador (`cold_episodic`, `stale_slot`, `duplicate_title`, `dangling_cross_project_link`, até 25 por tipo), com teste
  - **Papel:** executor
  - **agent:** executor1
  - **Dependências:** T005
  - **depends_on:** [T005]
  - **Paralelizável:** true
  - **Arquivos:** `internal/domain/maintenance/curator.go`, `internal/domain/maintenance/curator_test.go`
  - **Wiki-Keywords:** curador, cold_episodic, stale_slot, 25 por tipo

- [ ] **T022:** Criar o `reorg` (rederiva o projeto de cada sessão pelo `cwd`, plano, resumo, retirada para o cemitério), com teste
  - **Papel:** executor
  - **agent:** executor2
  - **Dependências:** T002
  - **depends_on:** [T002]
  - **Paralelizável:** true
  - **Arquivos:** `internal/wiki/reorg.go`, `internal/wiki/reorg_test.go`
  - **Wiki-Keywords:** reorg, plano, cemitério

- [ ] **T023:** Criar o `reclaim-ledger-versions` (versões substituídas de ledger, `drop_latest`, `compact`) com o reconhecimento de ledger da 002, com teste
  - **Papel:** executor
  - **agent:** executor3
  - **Dependências:** T001
  - **depends_on:** [T001]
  - **Paralelizável:** true
  - **Arquivos:** `internal/wiki/reclaim_ledger.go`, `internal/wiki/reclaim_ledger_test.go`
  - **Wiki-Keywords:** reclaim-ledger-versions, reconhecimento de ledger


### 2C Entradas

- [ ] **T024:** Criar as tools `memory_forget_sweep`, `memory_lint` e `memory_feedback`, com teste
  - **Papel:** executor
  - **agent:** executor1
  - **Dependências:** T017, T013
  - **depends_on:** [T017, T013]
  - **Paralelizável:** true
  - **Arquivos:** `internal/mcp/tools_maintenance.go`, `internal/mcp/tools_maintenance_test.go`
  - **Wiki-Keywords:** memory_forget_sweep, memory_lint, memory_feedback

- [ ] **T025:** Criar as rotas `POST /admin/forget-sweep`, `/admin/lint`, `/admin/curator` (modo relatório; `stage` dá 422), `/admin/reorg` e `/admin/reclaim-ledger-versions`, com teste
  - **Papel:** executor
  - **agent:** executor2
  - **Dependências:** T017, T020, T021, T022, T023
  - **depends_on:** [T017, T020, T021, T022, T023]
  - **Paralelizável:** true
  - **Arquivos:** `internal/cli/admin_maintenance.go`, `internal/cli/admin_maintenance_test.go`
  - **Wiki-Keywords:** forget-sweep, lint, curator, reorg, reclaim-ledger-versions

- [ ] **T026:** Montar as rotas da 004 no servidor HTTP
  - **Papel:** executor
  - **agent:** executor3
  - **Dependências:** T025
  - **depends_on:** [T025]
  - **Paralelizável:** false
  - **Arquivos:** `internal/cli/httpserver.go`
  - **Wiki-Keywords:** rotas de manutenção, montagem

- [ ] **T027:** Criar os agendadores de varredura, projetos vazios e lint (estado persistido, atraso de início até 60 s, nova tentativa limitada), com teste
  - **Papel:** executor
  - **agent:** executor1
  - **Dependências:** T014, T017, T020, T004
  - **depends_on:** [T014, T017, T020, T004]
  - **Paralelizável:** true
  - **Arquivos:** `internal/cli/scheduler.go`, `internal/cli/scheduler_test.go`
  - **Wiki-Keywords:** agendadores, último sucesso, atraso de 60 s

- [ ] **T028:** Iniciar os agendadores no boot do `serve`
  - **Papel:** executor
  - **agent:** executor2
  - **Dependências:** T027
  - **depends_on:** [T027]
  - **Paralelizável:** false
  - **Arquivos:** `internal/cli/serve_cmd.go`
  - **Wiki-Keywords:** serve, agendadores, boot

- [ ] **T029:** Criar os comandos finos `forget-sweep`, `lint`, `curator`, `reorg` e `reclaim-ledger-versions`, com teste
  - **Papel:** executor
  - **agent:** executor3
  - **Dependências:** T026
  - **depends_on:** [T026]
  - **Paralelizável:** true
  - **Arquivos:** `internal/cli/maintenance_cmds.go`, `internal/cli/maintenance_cmds_test.go`
  - **Wiki-Keywords:** forget-sweep, lint, curator, reorg, reclaim-ledger-versions

- [ ] **T030:** Registrar na raiz da CLI os comandos da 004
  - **Papel:** executor
  - **agent:** executor1
  - **Dependências:** T029
  - **depends_on:** [T029]
  - **Paralelizável:** false
  - **Arquivos:** `internal/cli/root.go`
  - **Wiki-Keywords:** cobra, raiz da CLI

- [ ] **T031:** Registrar as tools da 004 no servidor MCP do `serve`
  - **Papel:** executor
  - **agent:** executor2
  - **Dependências:** T024, T028
  - **depends_on:** [T024, T028]
  - **Paralelizável:** false
  - **Arquivos:** `internal/cli/serve_cmd.go`
  - **Wiki-Keywords:** MCP, registro de tools


### 2D Cenários Gherkin

- [ ] **T032:** Implementar Scenario: varredura padrão despeja episódica fria, poupa semântica e fixada, e TTL apaga mesmo fixada
  - **Papel:** executor
  - **agent:** executor3
  - **Dependências:** T030, T031, T028, T003
  - **depends_on:** [T030, T031, T028, T003]
  - **Paralelizável:** true
  - **Arquivos:** `test/acceptance/forget_sweep_test.go`
  - **Wiki-Keywords:** despejo, TTL, fixada, semântica

- [ ] **T033:** Implementar Scenario: leitura reforça a retenção na medida do doc e a página sobrevive
  - **Papel:** executor
  - **agent:** executor1
  - **Dependências:** T024, T016, T003
  - **depends_on:** [T024, T016, T003]
  - **Paralelizável:** true
  - **Arquivos:** `test/acceptance/access_retention_test.go`
  - **Wiki-Keywords:** reforço de acesso, retenção, memory_read_page

- [ ] **T034:** Implementar tratamento de erro: simulação lista os mesmos candidatos sem mudar arquivos
  - **Papel:** executor
  - **agent:** executor2
  - **Dependências:** T025, T003
  - **depends_on:** [T025, T003]
  - **Paralelizável:** true
  - **Arquivos:** `test/acceptance/sweep_dry_run_test.go`
  - **Wiki-Keywords:** dry_run, observations_prunable

- [ ] **T035:** Implementar edge case: compactação A2 terminal e recuperável por `restore-page`
  - **Papel:** executor
  - **agent:** executor3
  - **Dependências:** T025, T003
  - **depends_on:** [T025, T003]
  - **Paralelizável:** true
  - **Arquivos:** `test/acceptance/compaction_a2_test.go`
  - **Wiki-Keywords:** compactação A2, terminal, restore-page

- [ ] **T036:** Implementar edge case: remoção definitiva após 180 dias e caminho reescrito antes do prazo
  - **Papel:** executor
  - **agent:** executor1
  - **Dependências:** T025, T003
  - **depends_on:** [T025, T003]
  - **Paralelizável:** true
  - **Arquivos:** `test/acceptance/hard_delete_test.go`
  - **Wiki-Keywords:** remoção definitiva, 180 dias, caminho reescrito

- [ ] **T037:** Implementar edge case: poda de observações em lotes poupando sessão despejada na mesma execução
  - **Papel:** executor
  - **agent:** executor2
  - **Dependências:** T025, T003
  - **depends_on:** [T025, T003]
  - **Paralelizável:** true
  - **Arquivos:** `test/acceptance/observation_prune_test.go`
  - **Wiki-Keywords:** poda, 5000, sessão despejada

- [ ] **T038:** Implementar edge case: tombamento de página sumida com duas passagens e disjuntor
  - **Papel:** executor
  - **agent:** executor3
  - **Dependências:** T018, T003
  - **depends_on:** [T018, T003]
  - **Paralelizável:** true
  - **Arquivos:** `test/acceptance/deleted_pages_tombstone_test.go`
  - **Wiki-Keywords:** tombamento, disjuntor, varredura parcial

- [ ] **T039:** Implementar Scenario: lint e curador produzem os achados por regra sem editar páginas
  - **Papel:** executor
  - **agent:** executor1
  - **Dependências:** T025, T003
  - **depends_on:** [T025, T003]
  - **Paralelizável:** true
  - **Arquivos:** `test/acceptance/lint_curator_test.go`
  - **Wiki-Keywords:** lint, curador, relatório

- [ ] **T040:** Implementar edge case: página alterada entre seleção e aplicação não é apagada nem despejada (multi-sessão)
  - **Papel:** executor
  - **agent:** executor2
  - **Dependências:** T025, T017, T003
  - **depends_on:** [T025, T017, T003]
  - **Paralelizável:** true
  - **Arquivos:** `test/acceptance/sweep_guards_test.go`
  - **Wiki-Keywords:** guardas de seleção, página alterada, multi-sessão


## Fase 3: Validação

- [ ] **T041:** Criar testes de contrato INV-13 e INV-16 da 004 (sem provedor; despejo e compactação supersedem)
  - **Papel:** executor
  - **agent:** executor3
  - **Dependências:** T028
  - **depends_on:** [T028]
  - **Paralelizável:** true
  - **Arquivos:** `test/contract/maintenance_inv_test.go`
  - **Wiki-Keywords:** INV-13, INV-16, zero-LLM

- [ ] **T042:** Criar o smoke test com o binário (varredura em simulação e aplicada, lint, curador)
  - **Papel:** executor
  - **agent:** executor1
  - **Dependências:** T030
  - **depends_on:** [T030]
  - **Paralelizável:** true
  - **Arquivos:** `test/smoke/maintenance_smoke_test.go`
  - **Wiki-Keywords:** smoke, forget-sweep, lint

- [ ] **T043:** Rodar os portões e registrar o resultado com a saída
  - **Papel:** executor
  - **agent:** executor2
  - **Dependências:** T032, T033, T034, T035, T036, T037, T038, T039, T040, T041, T042
  - **depends_on:** [T032, T033, T034, T035, T036, T037, T038, T039, T040, T041, T042]
  - **Paralelizável:** false
  - **Arquivos:** `specs/features/004-manutencao-sem-llm/notes/gates-run.md`
  - **Wiki-Keywords:** check.sh, race, golangci-lint

- [ ] **T044:** Auditar a feature contra a constituição e contra `docs/design-memory-aging.md` e `docs/consolidation.md`
  - **Papel:** analyst
  - **agent:** analyst1
  - **Dependências:** T043
  - **depends_on:** [T043]
  - **Paralelizável:** false
  - **Arquivos:** `specs/features/004-manutencao-sem-llm/notes/constitution-audit.md`
  - **Wiki-Keywords:** constitution, auditoria, docs


## Fase 4: Documentação

- [ ] **T045:** Atualizar o README com a manutenção, as chaves `[decay]` e `[maintenance]` e o que é irreversível
  - **Papel:** executor
  - **agent:** executor3
  - **Dependências:** T044
  - **depends_on:** [T044]
  - **Paralelizável:** true
  - **Arquivos:** `README.md`
  - **Wiki-Keywords:** README, manutenção

- [ ] **T046:** Atualizar o llms.txt com a 004
  - **Papel:** executor
  - **agent:** executor1
  - **Dependências:** T045
  - **depends_on:** [T045]
  - **Paralelizável:** true
  - **Arquivos:** `llms.txt`
  - **Wiki-Keywords:** llms.txt, onboarding

- [ ] **T047:** Atualizar o .env.example com as chaves da 004
  - **Papel:** executor
  - **agent:** executor2
  - **Dependências:** T004
  - **depends_on:** [T004]
  - **Paralelizável:** true
  - **Arquivos:** `.env.example`
  - **Wiki-Keywords:** .env.example, decay, maintenance



## Coordination Graph

G₀ (round 0):

- **nodes:** T001, T002, T003, T004, T005, T006, T007, T008, T009, T010, T011, T012, T013, T014, T015, T016, T017, T018, T019, T020, T021, T022, T023, T024, T025, T026, T027, T028, T029, T030, T031, T032, T033, T034, T035, T036, T037, T038, T039, T040, T041, T042, T043, T044, T045, T046, T047
- **edges:**
  - T001 → T003
  - T001 → T004
  - T001 → T005
  - T005 → T006
  - T001 → T007
  - T007 → T008
  - T001 → T009
  - T001 → T010
  - T010 → T011
  - T001 → T012
  - T001 → T013
  - T001 → T014
  - T001 → T015
  - T001 → T016
  - T006 → T017
  - T008 → T017
  - T009 → T017
  - T010 → T017
  - T011 → T017
  - T012 → T017
  - T015 → T017
  - T001 → T018
  - T001 → T019
  - T019 → T020
  - T005 → T021
  - T002 → T022
  - T001 → T023
  - T017 → T024
  - T013 → T024
  - T017 → T025
  - T020 → T025
  - T021 → T025
  - T022 → T025
  - T023 → T025
  - T025 → T026
  - T014 → T027
  - T017 → T027
  - T020 → T027
  - T004 → T027
  - T027 → T028
  - T026 → T029
  - T029 → T030
  - T024 → T031
  - T028 → T031
  - T030 → T032
  - T031 → T032
  - T028 → T032
  - T003 → T032
  - T024 → T033
  - T016 → T033
  - T003 → T033
  - T025 → T034
  - T003 → T034
  - T025 → T035
  - T003 → T035
  - T025 → T036
  - T003 → T036
  - T025 → T037
  - T003 → T037
  - T018 → T038
  - T003 → T038
  - T025 → T039
  - T003 → T039
  - T025 → T040
  - T017 → T040
  - T003 → T040
  - T028 → T041
  - T030 → T042
  - T032 → T043
  - T033 → T043
  - T034 → T043
  - T035 → T043
  - T036 → T043
  - T037 → T043
  - T038 → T043
  - T039 → T043
  - T040 → T043
  - T041 → T043
  - T042 → T043
  - T043 → T044
  - T044 → T045
  - T045 → T046
  - T004 → T047
- **assignments:**
  - T001: analyst1
  - T002: researcher1
  - T003: executor1
  - T004: executor2
  - T005: executor3
  - T006: executor1
  - T007: executor2
  - T008: executor3
  - T009: executor1
  - T010: executor2
  - T011: executor3
  - T012: executor1
  - T013: executor2
  - T014: executor3
  - T015: executor1
  - T016: executor2
  - T017: executor3
  - T018: executor1
  - T019: executor2
  - T020: executor3
  - T021: executor1
  - T022: executor2
  - T023: executor3
  - T024: executor1
  - T025: executor2
  - T026: executor3
  - T027: executor1
  - T028: executor2
  - T029: executor3
  - T030: executor1
  - T031: executor2
  - T032: executor3
  - T033: executor1
  - T034: executor2
  - T035: executor3
  - T036: executor1
  - T037: executor2
  - T038: executor3
  - T039: executor1
  - T040: executor2
  - T041: executor3
  - T042: executor1
  - T043: executor2
  - T044: analyst1
  - T045: executor3
  - T046: executor1
  - T047: executor2
- **ready:** [T001, T002]

> Workers: Lead, researcher1, analyst1, executor1..3. Validado por script: 47 tasks, sem ciclo, sem dependência quebrada, sem arquivo compartilhado entre tasks sem ordem. Mínimo de 13 rounds. Caminho crítico (calculado): T001 → T005 → T006 → T017 → T025 → T026 → T029 → T030 → T032 → T043 → T044 → T045 → T046.
