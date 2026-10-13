---
graph-operators: enabled
heartbeat-threshold: 4
max-rounds: 40
---

# tasks.md: Cadeia de admissão

> Feature `008-cadeia-de-admissao` · Spec: `spec.md` (draft) · Plano: `plan.md` (A-01 a A-06)
>
> **Provisório:** gerado em 2026-10-13, antes da execução das features anteriores. A T001 reconfere tudo contra o código entregue e lista os pontos de operação reais antes de qualquer execução. O decidir-agir-notificar dos handoffs fica na `wiki`, que envolve a reivindicação do `storage` (o `storage` não importa a `wiki`).


## Fase 0: Descoberta

- [ ] **T001:** Reconferir este DAG contra o código entregue (escrita de página, purgas, movimentos, despejo, handoffs, relatório de lint) e listar cada ponto de operação a ligar
  - **Papel:** analyst
  - **agent:** analyst1
  - **Dependências:** Nenhuma
  - **depends_on:** []
  - **Paralelizável:** false
  - **Arquivos:** `specs/features/008-cadeia-de-admissao/notes/phase0-findings.md`
  - **Wiki-Keywords:** admission-webhooks, pontos de operação, código entregue


## Fase 1: Fundação

- [ ] **T002:** Criar o arquivo .feature da 008 com os cenários da spec
  - **Papel:** executor
  - **agent:** executor1
  - **Dependências:** T001
  - **depends_on:** [T001]
  - **Paralelizável:** true
  - **Arquivos:** `specs/features/008-cadeia-de-admissao/acceptance/admissao.feature`
  - **Wiki-Keywords:** Gherkin, admissão, ganchos

- [ ] **T003:** Acrescentar `[[admission_webhooks]]` e `AI_MEMORY_ADMISSION_WEBHOOKS_JSON` à configuração (substituição, corte de tempo, limite de 16), com teste
  - **Papel:** executor
  - **agent:** executor2
  - **Dependências:** T001
  - **depends_on:** [T001]
  - **Paralelizável:** true
  - **Arquivos:** `internal/config/config.go`, `internal/config/config_test.go`
  - **Wiki-Keywords:** admission_webhooks, JSON, 16 ganchos


## Fase 2: Implementação

### 2A Cadeia

- [ ] **T004:** Criar a cadeia (ganchos em ordem, contrato de fio, política de falha, 1 MiB, semáforo de 256 com log por tipo, lista de pulo), com teste
  - **Papel:** executor
  - **agent:** executor3
  - **Dependências:** T003
  - **depends_on:** [T003]
  - **Paralelizável:** false
  - **Arquivos:** `internal/wiki/admission.go`, `internal/wiki/admission_test.go`
  - **Wiki-Keywords:** cadeia, Mutate, Gate, Decide, Notify, 1 MiB, 256

- [ ] **T005:** Ligar a cadeia à escrita de página na ordem do doc (bloqueantes, segunda higienização, normalização, derivação, não bloqueantes com a página final), com teste
  - **Papel:** executor
  - **agent:** executor1
  - **Dependências:** T004
  - **depends_on:** [T004]
  - **Paralelizável:** false
  - **Arquivos:** `internal/wiki/write.go`, `internal/wiki/write_test.go`
  - **Wiki-Keywords:** escrita de página, segunda higienização, redrivação

- [ ] **T006:** Ligar `Gate` e `Notify` à exclusão, despejo, TTL, tombamento, purgas e movimentos, com `partial_failure` nas purgas, com teste
  - **Papel:** executor
  - **agent:** executor2
  - **Dependências:** T004
  - **depends_on:** [T004]
  - **Paralelizável:** true
  - **Arquivos:** `internal/wiki/admission_ops.go`, `internal/wiki/admission_ops_test.go`
  - **Wiki-Keywords:** delete, purge, move, partial_failure

- [ ] **T007:** Ligar decidir, agir e notificar aos handoffs (fim de sessão sem handoff em recusa, início de sessão com prazo de 750 ms, tools com erro), com teste multi-sessão
  - **Papel:** executor
  - **agent:** executor3
  - **Dependências:** T004
  - **depends_on:** [T004]
  - **Paralelizável:** true
  - **Arquivos:** `internal/wiki/handoff_admission.go`, `internal/wiki/handoff_admission_test.go`, `internal/ingest/session_end.go`, `internal/ingest/handoff_route.go`, `internal/mcp/tools_handoff.go`
  - **Wiki-Keywords:** decidir, agir, notificar, 750 ms

- [ ] **T008:** Ligar o op `consolidate` à escrita e à remoção do relatório de lint
  - **Papel:** executor
  - **agent:** executor1
  - **Dependências:** T005
  - **depends_on:** [T005]
  - **Paralelizável:** false
  - **Arquivos:** `internal/wiki/lint_report.go`
  - **Wiki-Keywords:** consolidate, pré-voo, relatório de lint

- [ ] **T009:** Fazer a cópia da fusão pular o gancho chamado exatamente `contributors` e disparar `purge_project` terminal
  - **Papel:** executor
  - **agent:** executor2
  - **Dependências:** T004
  - **depends_on:** [T004]
  - **Paralelizável:** true
  - **Arquivos:** `internal/wiki/merge_copy.go`
  - **Wiki-Keywords:** fusão, contributors, write_page


### 2B Servidor

- [ ] **T010:** Ler `X-Memory-Skip-Admission-Chain` no middleware e honrar só para root e anônimo, com teste
  - **Papel:** executor
  - **agent:** executor3
  - **Dependências:** T004
  - **depends_on:** [T004]
  - **Paralelizável:** true
  - **Arquivos:** `internal/cli/admission_skip.go`, `internal/cli/admission_skip_test.go`
  - **Wiki-Keywords:** X-Memory-Skip-Admission-Chain, root, anônimo

- [ ] **T011:** Montar a cadeia no boot do `serve` (nula sem ganchos)
  - **Papel:** executor
  - **agent:** executor1
  - **Dependências:** T004, T010
  - **depends_on:** [T004, T010]
  - **Paralelizável:** false
  - **Arquivos:** `internal/cli/serve_cmd.go`
  - **Wiki-Keywords:** serve, cadeia, montagem


### 2C Cenários Gherkin

- [ ] **T012:** Implementar Scenario: gancho bloqueante muta a escrita e o não bloqueante recebe a página final depois do commit
  - **Papel:** executor
  - **agent:** executor2
  - **Dependências:** T005, T011, T002
  - **depends_on:** [T005, T011, T002]
  - **Paralelizável:** true
  - **Arquivos:** `test/acceptance/admission_write_test.go`
  - **Wiki-Keywords:** contributors, git-mirror, mutação

- [ ] **T013:** Implementar tratamento de erro: `reject` aborta, `ignore` segue, resposta ilegível ou grande não muda, segredo reintroduzido é removido
  - **Papel:** executor
  - **agent:** executor3
  - **Dependências:** T005, T002
  - **depends_on:** [T005, T002]
  - **Paralelizável:** true
  - **Arquivos:** `test/acceptance/admission_failures_test.go`
  - **Wiki-Keywords:** reject, ignore, 1 MiB, segredo

- [ ] **T014:** Implementar edge case: handoffs com decisores lentos, recusa no fim de sessão e aceitação sem pendente não anunciada
  - **Papel:** executor
  - **agent:** executor1
  - **Dependências:** T007, T002
  - **depends_on:** [T007, T002]
  - **Paralelizável:** true
  - **Arquivos:** `test/acceptance/admission_handoff_test.go`
  - **Wiki-Keywords:** handoff, 750 ms, observador

- [ ] **T015:** Implementar edge case: notificações de exclusão, purga com `partial_failure` e fusão pulando `contributors`
  - **Papel:** executor
  - **agent:** executor2
  - **Dependências:** T006, T009, T002
  - **depends_on:** [T006, T009, T002]
  - **Paralelizável:** true
  - **Arquivos:** `test/acceptance/admission_ops_test.go`
  - **Wiki-Keywords:** purga, partial_failure, fusão

- [ ] **T016:** Implementar edge case: lista de pulo honrada para root e ignorada para usuário comum
  - **Papel:** executor
  - **agent:** executor3
  - **Dependências:** T010, T002
  - **depends_on:** [T010, T002]
  - **Paralelizável:** true
  - **Arquivos:** `test/acceptance/admission_skip_test.go`
  - **Wiki-Keywords:** lista de pulo, usuário comum


## Fase 3: Validação

- [ ] **T017:** Rodar os portões e registrar o resultado com a saída
  - **Papel:** executor
  - **agent:** executor1
  - **Dependências:** T008, T012, T013, T014, T015, T016
  - **depends_on:** [T008, T012, T013, T014, T015, T016]
  - **Paralelizável:** false
  - **Arquivos:** `specs/features/008-cadeia-de-admissao/notes/gates-run.md`
  - **Wiki-Keywords:** check.sh, race, golangci-lint

- [ ] **T018:** Auditar a feature contra a constituição e contra `docs/admission-webhooks.md`
  - **Papel:** analyst
  - **agent:** analyst1
  - **Dependências:** T017
  - **depends_on:** [T017]
  - **Paralelizável:** false
  - **Arquivos:** `specs/features/008-cadeia-de-admissao/notes/constitution-audit.md`
  - **Wiki-Keywords:** constitution, auditoria, admission


## Fase 4: Documentação

- [ ] **T019:** Atualizar o README com a configuração de ganchos e os exemplos do doc
  - **Papel:** executor
  - **agent:** executor2
  - **Dependências:** T018
  - **depends_on:** [T018]
  - **Paralelizável:** true
  - **Arquivos:** `README.md`
  - **Wiki-Keywords:** README, admissão

- [ ] **T020:** Atualizar o llms.txt com a 008
  - **Papel:** executor
  - **agent:** executor3
  - **Dependências:** T019
  - **depends_on:** [T019]
  - **Paralelizável:** true
  - **Arquivos:** `llms.txt`
  - **Wiki-Keywords:** llms.txt, onboarding

- [ ] **T021:** Atualizar o .env.example com `AI_MEMORY_ADMISSION_WEBHOOKS_JSON` (exemplo sem URL real)
  - **Papel:** executor
  - **agent:** executor1
  - **Dependências:** T003
  - **depends_on:** [T003]
  - **Paralelizável:** true
  - **Arquivos:** `.env.example`
  - **Wiki-Keywords:** .env.example, admission



## Coordination Graph

G₀ (round 0):

- **nodes:** T001, T002, T003, T004, T005, T006, T007, T008, T009, T010, T011, T012, T013, T014, T015, T016, T017, T018, T019, T020, T021
- **edges:**
  - T001 → T002
  - T001 → T003
  - T003 → T004
  - T004 → T005
  - T004 → T006
  - T004 → T007
  - T005 → T008
  - T004 → T009
  - T004 → T010
  - T004 → T011
  - T010 → T011
  - T005 → T012
  - T011 → T012
  - T002 → T012
  - T005 → T013
  - T002 → T013
  - T007 → T014
  - T002 → T014
  - T006 → T015
  - T009 → T015
  - T002 → T015
  - T010 → T016
  - T002 → T016
  - T008 → T017
  - T012 → T017
  - T013 → T017
  - T014 → T017
  - T015 → T017
  - T016 → T017
  - T017 → T018
  - T018 → T019
  - T019 → T020
  - T003 → T021
- **assignments:**
  - T001: analyst1
  - T002: executor1
  - T003: executor2
  - T004: executor3
  - T005: executor1
  - T006: executor2
  - T007: executor3
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
  - T018: analyst1
  - T019: executor2
  - T020: executor3
  - T021: executor1
- **ready:** [T001]

> Workers: Lead, analyst1, executor1..3. Validado por script: 21 tasks, sem ciclo, sem dependência quebrada, sem arquivo compartilhado entre tasks sem ordem. Mínimo de 10 rounds. Caminho crítico (calculado): T001 → T003 → T004 → T010 → T011 → T012 → T017 → T018 → T019 → T020.
