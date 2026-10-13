---
graph-operators: enabled
heartbeat-threshold: 4
max-rounds: 40
---

# tasks.md: Auto-melhoria

> Feature `022-auto-melhoria` · Spec: `spec.md` (draft) · Plano: `plan.md` (A-01 a A-05)
>
> **Provisório:** gerado em 2026-10-13, antes da execução das features anteriores. A T001 reconfere tudo contra o código entregue antes de qualquer execução. Sem dependência nova no `go.mod`.


## Fase 0: Descoberta

- [ ] **T001:** Reconferir este DAG contra o código entregue (projeção e sanitização da 020, entropia da 021, curador da 004, navegador da 014, admissão da 008) e consolidar as notas da fase 0
  - **Papel:** analyst
  - **agent:** analyst1
  - **Dependências:** Nenhuma
  - **depends_on:** []
  - **Paralelizável:** false
  - **Arquivos:** `specs/features/022-auto-melhoria/notes/phase0-findings.md`
  - **Wiki-Keywords:** 020 e 021 entregues, curador, navegador

- [ ] **T002:** Extrair a matriz completa de motivos de rejeição e limites numa tabela motivo → entrada → teste
  - **Papel:** researcher
  - **agent:** researcher1
  - **Dependências:** Nenhuma
  - **depends_on:** []
  - **Paralelizável:** true
  - **Arquivos:** `specs/features/022-auto-melhoria/notes/rejection-matrix.md`
  - **Wiki-Keywords:** motivos de rejeição, ordem, limites


## Fase 1: Fundação

- [ ] **T003:** Criar o arquivo .feature da 022 com os cenários da spec
  - **Papel:** executor
  - **agent:** executor1
  - **Dependências:** T001, T002
  - **depends_on:** [T001, T002]
  - **Paralelizável:** true
  - **Arquivos:** `specs/features/022-auto-melhoria/acceptance/auto-improve.feature`
  - **Wiki-Keywords:** Gherkin, agendador, aprovação, portão

- [ ] **T004:** Acrescentar à configuração `[auto_improve]`, `[auto_improve.scheduler]` e `[auto_improve.eval]`, com validação e teste
  - **Papel:** executor
  - **agent:** executor2
  - **Dependências:** T001
  - **depends_on:** [T001]
  - **Paralelizável:** true
  - **Arquivos:** `internal/config/config.go`, `internal/config/config_test.go`
  - **Wiki-Keywords:** auto_improve, scheduler, eval


## Fase 2: Implementação

### 2A Domínio

- [ ] **T005:** Criar as normalizações da proposta, com teste
  - **Papel:** executor
  - **agent:** executor3
  - **Dependências:** T002
  - **depends_on:** [T002]
  - **Paralelizável:** true
  - **Arquivos:** `internal/domain/autoimprove/normalize.go`, `internal/domain/autoimprove/normalize_test.go`
  - **Wiki-Keywords:** operation, edit_mode, kind, aliases

- [ ] **T006:** Criar a validação de página inteira e os limites da execução, com teste por motivo
  - **Papel:** executor
  - **agent:** executor1
  - **Dependências:** T005
  - **depends_on:** [T005]
  - **Paralelizável:** false
  - **Arquivos:** `internal/domain/autoimprove/validate.go`, `internal/domain/autoimprove/validate_test.go`
  - **Wiki-Keywords:** validação, motivos, destinos, orçamentos

- [ ] **T007:** Criar a materialização de patch com âncoras e hash de seção, com teste por motivo
  - **Papel:** executor
  - **agent:** executor2
  - **Dependências:** T005
  - **depends_on:** [T005]
  - **Paralelizável:** true
  - **Arquivos:** `internal/domain/autoimprove/patch.go`, `internal/domain/autoimprove/patch_test.go`
  - **Wiki-Keywords:** patch, âncoras, replace_section, hash

- [ ] **T008:** Criar a montagem do contexto do revisor com os orçamentos, com teste
  - **Papel:** executor
  - **agent:** executor3
  - **Dependências:** T001
  - **depends_on:** [T001]
  - **Paralelizável:** true
  - **Arquivos:** `internal/domain/autoimprove/context.go`, `internal/domain/autoimprove/context_test.go`
  - **Wiki-Keywords:** contexto, orçamentos, amostragem


### 2B Storage

- [ ] **T009:** Criar o registro de execuções, propostas e eventos com snapshot do alvo, com teste
  - **Papel:** executor
  - **agent:** executor1
  - **Dependências:** T001
  - **depends_on:** [T001]
  - **Paralelizável:** true
  - **Arquivos:** `internal/storage/auto_improve.go`, `internal/storage/auto_improve_test.go`
  - **Wiki-Keywords:** execuções, propostas, eventos, snapshot

- [ ] **T010:** Criar o buffer de rejeições com impressão digital, com teste
  - **Papel:** executor
  - **agent:** executor2
  - **Dependências:** T009
  - **depends_on:** [T009]
  - **Paralelizável:** false
  - **Arquivos:** `internal/storage/auto_improve_rejections.go`, `internal/storage/auto_improve_rejections_test.go`
  - **Wiki-Keywords:** buffer, impressão digital, 180 dias

- [ ] **T011:** Criar o estado do agendador e as reivindicações, com teste multi-sessão
  - **Papel:** executor
  - **agent:** executor3
  - **Dependências:** T009
  - **depends_on:** [T009]
  - **Paralelizável:** true
  - **Arquivos:** `internal/storage/auto_improve_scheduler.go`, `internal/storage/auto_improve_scheduler_test.go`
  - **Wiki-Keywords:** marca d'água, reivindicação, estacionar, âncora


### 2C Revisão, portão e aplicação

- [ ] **T012:** Criar o portão de avaliação com os 7 casos de aceitação do doc
  - **Papel:** executor
  - **agent:** executor1
  - **Dependências:** T004
  - **depends_on:** [T004]
  - **Paralelizável:** true
  - **Arquivos:** `internal/cli/eval_gate.go`, `internal/cli/eval_gate_test.go`
  - **Wiki-Keywords:** portão, sem shell, 64 KiB, timeout

- [ ] **T013:** Criar a revisão de uma sessão (pré-filtros, contexto, chamada, validação), com teste com provedor falso
  - **Papel:** executor
  - **agent:** executor2
  - **Dependências:** T006, T007, T008, T010
  - **depends_on:** [T006, T007, T008, T010]
  - **Paralelizável:** false
  - **Arquivos:** `internal/wiki/auto_improve_review.go`, `internal/wiki/auto_improve_review_test.go`
  - **Wiki-Keywords:** revisão, pré-filtros, chamada única

- [ ] **T014:** Criar o registro, o arquivo de revisão e a aprovação com conflitos, com teste
  - **Papel:** executor
  - **agent:** executor3
  - **Dependências:** T013, T009, T012
  - **depends_on:** [T013, T009, T012]
  - **Paralelizável:** false
  - **Arquivos:** `internal/wiki/auto_improve_apply.go`, `internal/wiki/auto_improve_apply_test.go`
  - **Wiki-Keywords:** registro, arquivo de revisão, aprovação, conflito

- [ ] **T015:** Criar a passada de experiência, com teste
  - **Papel:** executor
  - **agent:** executor1
  - **Dependências:** T014, T011
  - **depends_on:** [T014, T011]
  - **Paralelizável:** false
  - **Arquivos:** `internal/wiki/experience.go`, `internal/wiki/experience_test.go`
  - **Wiki-Keywords:** experiência, âncora, entropia

- [ ] **T016:** Criar o agendador da auto-melhoria, com teste com relógio injetado
  - **Papel:** executor
  - **agent:** executor2
  - **Dependências:** T014, T011, T015
  - **depends_on:** [T014, T011, T015]
  - **Paralelizável:** false
  - **Arquivos:** `internal/cli/auto_improve_scheduler.go`, `internal/cli/auto_improve_scheduler_test.go`
  - **Wiki-Keywords:** agendador, ciclos, estacionar

- [ ] **T017:** Criar o relatório de telemetria, com teste
  - **Papel:** executor
  - **agent:** executor3
  - **Dependências:** T009
  - **depends_on:** [T009]
  - **Paralelizável:** true
  - **Arquivos:** `internal/wiki/auto_improve_report.go`, `internal/wiki/auto_improve_report_test.go`
  - **Wiki-Keywords:** telemetria, taxas terminais, --stage


### 2D Superfícies

- [ ] **T018:** Criar as rotas `/admin/auto-improve`, `/admin/auto-improve/report` e `/admin/pending-writes`, com teste
  - **Papel:** executor
  - **agent:** executor1
  - **Dependências:** T014, T017
  - **depends_on:** [T014, T017]
  - **Paralelizável:** false
  - **Arquivos:** `internal/cli/admin_auto_improve.go`, `internal/cli/admin_auto_improve_test.go`
  - **Wiki-Keywords:** /admin/auto-improve, report, pending-writes, 409

- [ ] **T019:** Criar os comandos `auto-improve`, `auto-improve-report` e `pending-writes`, com teste
  - **Papel:** executor
  - **agent:** executor2
  - **Dependências:** T018
  - **depends_on:** [T018]
  - **Paralelizável:** false
  - **Arquivos:** `internal/cli/auto_improve_cmds.go`, `internal/cli/auto_improve_cmds_test.go`
  - **Wiki-Keywords:** auto-improve, auto-improve-report, pending-writes

- [ ] **T020:** Criar a ferramenta `memory_auto_improve`, com teste
  - **Papel:** executor
  - **agent:** executor3
  - **Dependências:** T014
  - **depends_on:** [T014]
  - **Paralelizável:** true
  - **Arquivos:** `internal/mcp/tools_auto_improve.go`, `internal/mcp/tools_auto_improve_test.go`
  - **Wiki-Keywords:** memory_auto_improve, argumentos removidos

- [ ] **T021:** Criar a página `/pending` do navegador (só raiz, ações nas rotas de administração), com teste
  - **Papel:** executor
  - **agent:** executor1
  - **Dependências:** T018
  - **depends_on:** [T018]
  - **Paralelizável:** true
  - **Arquivos:** `internal/cli/web/pending.go`, `internal/cli/web/pending_test.go`, `internal/cli/web/templates/`
  - **Wiki-Keywords:** /pending, raiz, aprovar, rejeitar

- [ ] **T022:** Ligar o modo `stage` do curador ao registro de pendências, com teste
  - **Papel:** executor
  - **agent:** executor2
  - **Dependências:** T014
  - **depends_on:** [T014]
  - **Paralelizável:** true
  - **Arquivos:** `internal/wiki/curator.go`, `internal/wiki/curator_test.go`
  - **Wiki-Keywords:** curador --stage, notes/curator

- [ ] **T023:** Montar o agendador, as rotas, os comandos e a ferramenta
  - **Papel:** executor
  - **agent:** executor3
  - **Dependências:** T016, T019, T020, T021, T022
  - **depends_on:** [T016, T019, T020, T021, T022]
  - **Paralelizável:** false
  - **Arquivos:** `internal/cli/serve_cmd.go`, `internal/cli/root.go`, `internal/cli/httpserver.go`, `internal/mcp/server.go`
  - **Wiki-Keywords:** montagem, agendador, rotas, ferramenta


### Cenários Gherkin

- [ ] **T024:** Implementar Scenario: sessão revisada pelo agendador com aprovação automática e com `require_approval`
  - **Papel:** executor
  - **agent:** executor1
  - **Dependências:** T023, T003
  - **depends_on:** [T023, T003]
  - **Paralelizável:** true
  - **Arquivos:** `test/acceptance/auto_improve_flow_test.go`
  - **Wiki-Keywords:** agendador, aprovação automática, pendente

- [ ] **T025:** Implementar tratamento de erro: pré-filtro, estacionamento, portão reprovando e página fixada
  - **Papel:** executor
  - **agent:** executor2
  - **Dependências:** T023, T003
  - **depends_on:** [T023, T003]
  - **Paralelizável:** true
  - **Arquivos:** `test/acceptance/auto_improve_rejections_test.go`
  - **Wiki-Keywords:** pré-filtro, estacionar, portão, fixada

- [ ] **T026:** Implementar edge case: reivindicação concorrente, âncora duplicada e experiência sem páginas suficientes
  - **Papel:** executor
  - **agent:** executor3
  - **Dependências:** T023, T003
  - **depends_on:** [T023, T003]
  - **Paralelizável:** true
  - **Arquivos:** `test/acceptance/auto_improve_concurrency_test.go`
  - **Wiki-Keywords:** reivindicação concorrente, âncora duplicada, experiência


## Fase 3: Validação

- [ ] **T027:** Rodar os portões e registrar o resultado com a saída
  - **Papel:** executor
  - **agent:** executor1
  - **Dependências:** T024, T025, T026
  - **depends_on:** [T024, T025, T026]
  - **Paralelizável:** false
  - **Arquivos:** `specs/features/022-auto-melhoria/notes/gates-run.md`
  - **Wiki-Keywords:** check.sh, race, golangci-lint

- [ ] **T028:** Auditar a feature contra a constituição e contra as 15 invariantes do doc
  - **Papel:** analyst
  - **agent:** analyst1
  - **Dependências:** T027
  - **depends_on:** [T027]
  - **Paralelizável:** false
  - **Arquivos:** `specs/features/022-auto-melhoria/notes/constitution-audit.md`
  - **Wiki-Keywords:** constitution, auditoria, invariantes da auto-melhoria


## Fase 4: Documentação

- [ ] **T029:** Atualizar o README com a auto-melhoria e a revisão de pendências
  - **Papel:** executor
  - **agent:** executor2
  - **Dependências:** T028
  - **depends_on:** [T028]
  - **Paralelizável:** true
  - **Arquivos:** `README.md`
  - **Wiki-Keywords:** README, auto-melhoria, pending-writes

- [ ] **T030:** Atualizar o llms.txt com a 022
  - **Papel:** executor
  - **agent:** executor3
  - **Dependências:** T029
  - **depends_on:** [T029]
  - **Paralelizável:** true
  - **Arquivos:** `llms.txt`
  - **Wiki-Keywords:** llms.txt, onboarding

- [ ] **T031:** Atualizar o .env.example com as chaves da 022
  - **Papel:** executor
  - **agent:** executor1
  - **Dependências:** T004
  - **depends_on:** [T004]
  - **Paralelizável:** true
  - **Arquivos:** `.env.example`
  - **Wiki-Keywords:** .env.example, auto-melhoria



## Coordination Graph

G₀ (round 0):

- **nodes:** T001, T002, T003, T004, T005, T006, T007, T008, T009, T010, T011, T012, T013, T014, T015, T016, T017, T018, T019, T020, T021, T022, T023, T024, T025, T026, T027, T028, T029, T030, T031
- **edges:**
  - T001 → T003
  - T002 → T003
  - T001 → T004
  - T002 → T005
  - T005 → T006
  - T005 → T007
  - T001 → T008
  - T001 → T009
  - T009 → T010
  - T009 → T011
  - T004 → T012
  - T006 → T013
  - T007 → T013
  - T008 → T013
  - T010 → T013
  - T013 → T014
  - T009 → T014
  - T012 → T014
  - T014 → T015
  - T011 → T015
  - T014 → T016
  - T011 → T016
  - T015 → T016
  - T009 → T017
  - T014 → T018
  - T017 → T018
  - T018 → T019
  - T014 → T020
  - T018 → T021
  - T014 → T022
  - T016 → T023
  - T019 → T023
  - T020 → T023
  - T021 → T023
  - T022 → T023
  - T023 → T024
  - T003 → T024
  - T023 → T025
  - T003 → T025
  - T023 → T026
  - T003 → T026
  - T024 → T027
  - T025 → T027
  - T026 → T027
  - T027 → T028
  - T028 → T029
  - T029 → T030
  - T004 → T031
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
  - T028: analyst1
  - T029: executor2
  - T030: executor3
  - T031: executor1
- **ready:** [T001, T002]

> Workers: Lead, researcher1, analyst1, executor1..3. Validado por script: 31 tasks, sem ciclo, sem dependência quebrada, sem arquivo compartilhado entre tasks sem ordem. Mínimo de 13 rounds. Caminho crítico (calculado): T002 → T005 → T006 → T013 → T014 → T015 → T016 → T023 → T024 → T027 → T028 → T029 → T030.
