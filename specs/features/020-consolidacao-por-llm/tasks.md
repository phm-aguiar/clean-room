---
graph-operators: enabled
heartbeat-threshold: 4
max-rounds: 40
---

# tasks.md: Consolidação por LLM, bootstrap, lint e perfil por LLM

> Feature `020-consolidacao-por-llm` · Spec: `spec.md` (draft) · Plano: `plan.md` (K-01 a K-06)
>
> **Provisório:** gerado em 2026-10-13, antes da execução das features anteriores. A T001 reconfere tudo contra o código entregue antes de qualquer execução. Sem dependência nova no `go.mod`.


## Fase 0: Descoberta

- [ ] **T001:** Reconferir este DAG contra o código entregue (fim de sessão e checkpoints da 002, lint da 004, admissão da 008, perfil da 012, provedor da 016) e consolidar as notas da fase 0
  - **Papel:** analyst
  - **agent:** analyst1
  - **Dependências:** Nenhuma
  - **depends_on:** []
  - **Paralelizável:** false
  - **Arquivos:** `specs/features/020-consolidacao-por-llm/notes/phase0-findings.md`
  - **Wiki-Keywords:** fim de sessão, checkpoints, lint, perfil, admissão

- [ ] **T002:** Extrair os prompts e as validações por passe numa tabela regra → teste
  - **Papel:** researcher
  - **agent:** researcher1
  - **Dependências:** Nenhuma
  - **depends_on:** []
  - **Paralelizável:** true
  - **Arquivos:** `specs/features/020-consolidacao-por-llm/notes/prompt-contracts.md`
  - **Wiki-Keywords:** prompts, consolidação, lote, bootstrap, lint, perfil


## Fase 1: Fundação

- [ ] **T003:** Criar o arquivo .feature da 020 com os cenários da spec
  - **Papel:** executor
  - **agent:** executor1
  - **Dependências:** T001, T002
  - **depends_on:** [T001, T002]
  - **Paralelizável:** true
  - **Arquivos:** `specs/features/020-consolidacao-por-llm/acceptance/consolidation.feature`
  - **Wiki-Keywords:** Gherkin, fila, bootstrap, checkpoint

- [ ] **T004:** Acrescentar à configuração `[consolidation]`, `consolidate_on_session_end` e `[profile] llm`, com validação e teste
  - **Papel:** executor
  - **agent:** executor2
  - **Dependências:** T001
  - **depends_on:** [T001]
  - **Paralelizável:** true
  - **Arquivos:** `internal/config/config.go`, `internal/config/config_test.go`
  - **Wiki-Keywords:** consolidation, consolidate_on_session_end, profile.llm


## Fase 2: Implementação

### 2A Domínio

- [ ] **T005:** Criar os tipos de saída, a validação do §10.2 e o slug de regra, com teste
  - **Papel:** executor
  - **agent:** executor3
  - **Dependências:** T001
  - **depends_on:** [T001]
  - **Paralelizável:** true
  - **Arquivos:** `internal/domain/consolidate/types.go`, `internal/domain/consolidate/types_test.go`
  - **Wiki-Keywords:** tipos de saída, validação 10.2, slug

- [ ] **T006:** Criar a sanitização de caminho produzido pelo modelo, com teste
  - **Papel:** executor
  - **agent:** executor1
  - **Dependências:** T001
  - **depends_on:** [T001]
  - **Paralelizável:** true
  - **Arquivos:** `internal/domain/consolidate/path.go`, `internal/domain/consolidate/path_test.go`
  - **Wiki-Keywords:** sanitização de caminho, .md, idempotente

- [ ] **T007:** Criar a projeção determinística de observações, com golden
  - **Papel:** executor
  - **agent:** executor2
  - **Dependências:** T001
  - **depends_on:** [T001]
  - **Paralelizável:** true
  - **Arquivos:** `internal/domain/consolidate/projection.go`, `internal/domain/consolidate/projection_test.go`
  - **Wiki-Keywords:** projeção, pontuação, âncoras, golden

- [ ] **T008:** Criar o cálculo de orçamento de prompt, com teste
  - **Papel:** executor
  - **agent:** executor3
  - **Dependências:** T005
  - **depends_on:** [T005]
  - **Paralelizável:** false
  - **Arquivos:** `internal/domain/consolidate/budget.go`, `internal/domain/consolidate/budget_test.go`
  - **Wiki-Keywords:** orçamento, margem 0,8, reservas

- [ ] **T009:** Criar os prompts de consolidação, lote, bootstrap e lint, com teste das seções obrigatórias
  - **Papel:** executor
  - **agent:** executor1
  - **Dependências:** T002
  - **depends_on:** [T002]
  - **Paralelizável:** true
  - **Arquivos:** `internal/domain/consolidate/prompts.go`, `internal/domain/consolidate/prompts_test.go`
  - **Wiki-Keywords:** prompts, segurança, fidelidade


### 2B Storage

- [ ] **T010:** Criar a fila de consolidação, com teste multi-sessão
  - **Papel:** executor
  - **agent:** executor2
  - **Dependências:** T001
  - **depends_on:** [T001]
  - **Paralelizável:** true
  - **Arquivos:** `internal/storage/consolidation_jobs.go`, `internal/storage/consolidation_jobs_test.go`
  - **Wiki-Keywords:** fila, geração, claim_id, IMMEDIATE

- [ ] **T011:** Criar o progresso durável de bootstrap, com teste
  - **Papel:** executor
  - **agent:** executor3
  - **Dependências:** T001
  - **depends_on:** [T001]
  - **Paralelizável:** true
  - **Arquivos:** `internal/storage/bootstrap_progress.go`, `internal/storage/bootstrap_progress_test.go`
  - **Wiki-Keywords:** progresso, fingerprint, prefixo contíguo


### 2C Wiki

- [ ] **T012:** Criar a consolidação de página única (admissão, `dry_run`, preferências, retentativa, título, commit), com teste
  - **Papel:** executor
  - **agent:** executor1
  - **Dependências:** T005, T006, T007, T008, T009, T004
  - **depends_on:** [T005, T006, T007, T008, T009, T004]
  - **Paralelizável:** false
  - **Arquivos:** `internal/wiki/consolidate.go`, `internal/wiki/consolidate_test.go`
  - **Wiki-Keywords:** página única, preferências, retentativa, commit

- [ ] **T013:** Criar a consolidação em lote com as guardas por atualização, com teste
  - **Papel:** executor
  - **agent:** executor2
  - **Dependências:** T012
  - **depends_on:** [T012]
  - **Paralelizável:** false
  - **Arquivos:** `internal/wiki/consolidate_batch.go`, `internal/wiki/consolidate_batch_test.go`
  - **Wiki-Keywords:** lote, guardas, atômico

- [ ] **T014:** Criar o bootstrap no servidor (chunks, retomada, manifesto), com teste
  - **Papel:** executor
  - **agent:** executor3
  - **Dependências:** T009, T011
  - **depends_on:** [T009, T011]
  - **Paralelizável:** false
  - **Arquivos:** `internal/wiki/bootstrap.go`, `internal/wiki/bootstrap_test.go`
  - **Wiki-Keywords:** bootstrap, chunks, manifesto

- [ ] **T015:** Acrescentar ao lint a passada de contradições por LLM, com teste
  - **Papel:** executor
  - **agent:** executor1
  - **Dependências:** T009
  - **depends_on:** [T009]
  - **Paralelizável:** true
  - **Arquivos:** `internal/wiki/lint_report.go`, `internal/wiki/lint_report_test.go`
  - **Wiki-Keywords:** lint por LLM, 20 páginas, no_llm

- [ ] **T016:** Acrescentar à passada de perfil a classificação e a mesclagem por LLM com recaída, com teste
  - **Papel:** executor
  - **agent:** executor2
  - **Dependências:** T004
  - **depends_on:** [T004]
  - **Paralelizável:** true
  - **Arquivos:** `internal/domain/profile/llm.go`, `internal/domain/profile/llm_test.go`, `internal/wiki/profile_pass.go`, `internal/wiki/profile_pass_test.go`
  - **Wiki-Keywords:** perfil, classificar, mesclar, recaída


### 2D Servidor, ingestão e CLI

- [ ] **T017:** Criar o trabalhador da fila, com teste
  - **Papel:** executor
  - **agent:** executor3
  - **Dependências:** T010, T012
  - **depends_on:** [T010, T012]
  - **Paralelizável:** false
  - **Arquivos:** `internal/cli/consolidation_worker.go`, `internal/cli/consolidation_worker_test.go`
  - **Wiki-Keywords:** trabalhador, 15 s, lease 10 min, esperas

- [ ] **T018:** Ligar a consolidação aos checkpoints e o enfileiramento ao fim de sessão, com teste
  - **Papel:** executor
  - **agent:** executor1
  - **Dependências:** T012, T010
  - **depends_on:** [T012, T010]
  - **Paralelizável:** false
  - **Arquivos:** `internal/ingest/checkpoint.go`, `internal/ingest/checkpoint_test.go`, `internal/ingest/session_end.go`, `internal/ingest/session_end_test.go`
  - **Wiki-Keywords:** checkpoint, recaída, falha fechado, enfileirar

- [ ] **T019:** Criar a ferramenta `memory_consolidate`, com teste
  - **Papel:** executor
  - **agent:** executor2
  - **Dependências:** T013
  - **depends_on:** [T013]
  - **Paralelizável:** false
  - **Arquivos:** `internal/mcp/tools_consolidate.go`, `internal/mcp/tools_consolidate_test.go`
  - **Wiki-Keywords:** memory_consolidate, multi_page

- [ ] **T020:** Criar `POST /admin/bootstrap` e o `no_llm` do `/admin/lint`, com teste
  - **Papel:** executor
  - **agent:** executor3
  - **Dependências:** T014, T015
  - **depends_on:** [T014, T015]
  - **Paralelizável:** false
  - **Arquivos:** `internal/cli/admin_bootstrap.go`, `internal/cli/admin_bootstrap_test.go`, `internal/cli/admin_maintenance.go`
  - **Wiki-Keywords:** /admin/bootstrap, 422, 503, 502, no_llm

- [ ] **T021:** Criar o cliente `bootstrap` (coleta, poda, flags, saída do `--dry-run`), com teste
  - **Papel:** executor
  - **agent:** executor1
  - **Dependências:** T020
  - **depends_on:** [T020]
  - **Paralelizável:** false
  - **Arquivos:** `internal/cli/bootstrap_cmd.go`, `internal/cli/bootstrap_cmd_test.go`
  - **Wiki-Keywords:** cliente bootstrap, fontes, prioridades, dry-run

- [ ] **T022:** Montar o trabalhador, as rotas, o comando e a ferramenta
  - **Papel:** executor
  - **agent:** executor2
  - **Dependências:** T017, T018, T019, T021, T016
  - **depends_on:** [T017, T018, T019, T021, T016]
  - **Paralelizável:** false
  - **Arquivos:** `internal/cli/serve_cmd.go`, `internal/cli/root.go`, `internal/cli/httpserver.go`, `internal/mcp/server.go`
  - **Wiki-Keywords:** montagem, trabalhador, rotas, ferramenta


### Cenários Gherkin

- [ ] **T023:** Implementar Scenario: fim de sessão enfileira e o trabalhador consolida
  - **Papel:** executor
  - **agent:** executor3
  - **Dependências:** T022, T003
  - **depends_on:** [T022, T003]
  - **Paralelizável:** true
  - **Arquivos:** `test/acceptance/consolidation_queue_test.go`
  - **Wiki-Keywords:** fim de sessão, fila, consolidated

- [ ] **T024:** Implementar tratamento de erro: checkpoint com provedor fora e com recusa de admissão
  - **Papel:** executor
  - **agent:** executor1
  - **Dependências:** T022, T003
  - **depends_on:** [T022, T003]
  - **Paralelizável:** true
  - **Arquivos:** `test/acceptance/consolidation_fallback_test.go`
  - **Wiki-Keywords:** 503, admissão, falha fechado

- [ ] **T025:** Implementar edge case: bootstrap interrompido, retomada e repetição sem `--force`
  - **Papel:** executor
  - **agent:** executor2
  - **Dependências:** T022, T003
  - **depends_on:** [T022, T003]
  - **Paralelizável:** true
  - **Arquivos:** `test/acceptance/bootstrap_resume_test.go`
  - **Wiki-Keywords:** bootstrap, retomada, lacuna, 422


## Fase 3: Validação

- [ ] **T026:** Rodar os portões e registrar o resultado com a saída
  - **Papel:** executor
  - **agent:** executor3
  - **Dependências:** T023, T024, T025
  - **depends_on:** [T023, T024, T025]
  - **Paralelizável:** false
  - **Arquivos:** `specs/features/020-consolidacao-por-llm/notes/gates-run.md`
  - **Wiki-Keywords:** check.sh, race, golangci-lint

- [ ] **T027:** Auditar a feature contra a constituição e contra os docs
  - **Papel:** analyst
  - **agent:** analyst1
  - **Dependências:** T026
  - **depends_on:** [T026]
  - **Paralelizável:** false
  - **Arquivos:** `specs/features/020-consolidacao-por-llm/notes/constitution-audit.md`
  - **Wiki-Keywords:** constitution, auditoria, consolidação


## Fase 4: Documentação

- [ ] **T028:** Atualizar o README com a consolidação por LLM e o bootstrap
  - **Papel:** executor
  - **agent:** executor1
  - **Dependências:** T027
  - **depends_on:** [T027]
  - **Paralelizável:** true
  - **Arquivos:** `README.md`
  - **Wiki-Keywords:** README, consolidação, bootstrap

- [ ] **T029:** Atualizar o llms.txt com a 020
  - **Papel:** executor
  - **agent:** executor2
  - **Dependências:** T028
  - **depends_on:** [T028]
  - **Paralelizável:** true
  - **Arquivos:** `llms.txt`
  - **Wiki-Keywords:** llms.txt, onboarding

- [ ] **T030:** Atualizar o .env.example com as chaves da 020
  - **Papel:** executor
  - **agent:** executor3
  - **Dependências:** T004
  - **depends_on:** [T004]
  - **Paralelizável:** true
  - **Arquivos:** `.env.example`
  - **Wiki-Keywords:** .env.example, consolidação



## Coordination Graph

G₀ (round 0):

- **nodes:** T001, T002, T003, T004, T005, T006, T007, T008, T009, T010, T011, T012, T013, T014, T015, T016, T017, T018, T019, T020, T021, T022, T023, T024, T025, T026, T027, T028, T029, T030
- **edges:**
  - T001 → T003
  - T002 → T003
  - T001 → T004
  - T001 → T005
  - T001 → T006
  - T001 → T007
  - T005 → T008
  - T002 → T009
  - T001 → T010
  - T001 → T011
  - T005 → T012
  - T006 → T012
  - T007 → T012
  - T008 → T012
  - T009 → T012
  - T004 → T012
  - T012 → T013
  - T009 → T014
  - T011 → T014
  - T009 → T015
  - T004 → T016
  - T010 → T017
  - T012 → T017
  - T012 → T018
  - T010 → T018
  - T013 → T019
  - T014 → T020
  - T015 → T020
  - T020 → T021
  - T017 → T022
  - T018 → T022
  - T019 → T022
  - T021 → T022
  - T016 → T022
  - T022 → T023
  - T003 → T023
  - T022 → T024
  - T003 → T024
  - T022 → T025
  - T003 → T025
  - T023 → T026
  - T024 → T026
  - T025 → T026
  - T026 → T027
  - T027 → T028
  - T028 → T029
  - T004 → T030
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
  - T027: analyst1
  - T028: executor1
  - T029: executor2
  - T030: executor3
- **ready:** [T001, T002]

> Workers: Lead, researcher1, analyst1, executor1..3. Validado por script: 30 tasks, sem ciclo, sem dependência quebrada, sem arquivo compartilhado entre tasks sem ordem. Mínimo de 12 rounds. Caminho crítico (calculado): T001 → T005 → T008 → T012 → T013 → T019 → T022 → T023 → T026 → T027 → T028 → T029.
