---
graph-operators: enabled
heartbeat-threshold: 4
max-rounds: 40
---

# tasks.md: Curadoria por embeddings e dream

> Feature `021-curadoria-por-embeddings` · Spec: `spec.md` (draft) · Plano: `plan.md` (D-01 a D-04)
>
> **Provisório:** gerado em 2026-10-13, antes da execução das features anteriores. A T001 reconfere tudo contra o código entregue antes de qualquer execução. Sem dependência nova no `go.mod`.


## Fase 0: Descoberta

- [ ] **T001:** Reconferir este DAG contra o código entregue (varredura, compactação e lint da 004, vetores da 017, provedor da 016) e consolidar as notas da fase 0
  - **Papel:** analyst
  - **agent:** analyst1
  - **Dependências:** Nenhuma
  - **depends_on:** []
  - **Paralelizável:** false
  - **Arquivos:** `specs/features/021-curadoria-por-embeddings/notes/phase0-findings.md`
  - **Wiki-Keywords:** varredura, conjunto frio, compactação, vetores


## Fase 1: Fundação

- [ ] **T002:** Criar o arquivo .feature da 021 com os cenários da spec
  - **Papel:** executor
  - **agent:** executor1
  - **Dependências:** T001
  - **depends_on:** [T001]
  - **Paralelizável:** true
  - **Arquivos:** `specs/features/021-curadoria-por-embeddings/acceptance/curation.feature`
  - **Wiki-Keywords:** Gherkin, dedup, dream

- [ ] **T003:** Acrescentar à configuração `dedup_*`, `contradiction_band_*`, `[dream]` e o filtro de entropia, com as validações de carga e teste
  - **Papel:** executor
  - **agent:** executor2
  - **Dependências:** T001
  - **depends_on:** [T001]
  - **Paralelizável:** true
  - **Arquivos:** `internal/config/config.go`, `internal/config/config_test.go`
  - **Wiki-Keywords:** dedup, banda, dream, entropia


## Fase 2: Implementação

- [ ] **T004:** Criar as funções puras de agrupamento, com testes de tabela
  - **Papel:** executor
  - **agent:** executor3
  - **Dependências:** T001
  - **depends_on:** [T001]
  - **Paralelizável:** true
  - **Arquivos:** `internal/domain/maintenance/vecmath.go`, `internal/domain/maintenance/vecmath_test.go`
  - **Wiki-Keywords:** cosseno, eps adaptativo, DBSCAN, surprisal

- [ ] **T005:** Criar o filtro de entropia e boilerplate, com teste
  - **Papel:** executor
  - **agent:** executor1
  - **Dependências:** T001
  - **depends_on:** [T001]
  - **Paralelizável:** true
  - **Arquivos:** `internal/domain/maintenance/entropy.go`, `internal/domain/maintenance/entropy_test.go`
  - **Wiki-Keywords:** entropia, near_empty, repetitive

- [ ] **T006:** Criar a deduplicação de clusters frios na varredura agendada, com o cenário 5 como teste
  - **Papel:** executor
  - **agent:** executor2
  - **Dependências:** T004, T003
  - **depends_on:** [T004, T003]
  - **Paralelizável:** false
  - **Arquivos:** `internal/wiki/dedup.go`, `internal/wiki/dedup_test.go`, `internal/wiki/sweep.go`
  - **Wiki-Keywords:** dedup A3, sobrevivente, stubs, a3_merge

- [ ] **T007:** Criar a banda de contradição no lint, com teste
  - **Papel:** executor
  - **agent:** executor3
  - **Dependências:** T004, T003
  - **depends_on:** [T004, T003]
  - **Paralelizável:** true
  - **Arquivos:** `internal/wiki/contradiction_band.go`, `internal/wiki/contradiction_band_test.go`, `internal/wiki/lint_report.go`
  - **Wiki-Keywords:** banda, 60, 25 achados

- [ ] **T008:** Criar a passada do dream, com teste com provedor falso
  - **Papel:** executor
  - **agent:** executor1
  - **Dependências:** T006
  - **depends_on:** [T006]
  - **Paralelizável:** false
  - **Arquivos:** `internal/wiki/dream.go`, `internal/wiki/dream_test.go`
  - **Wiki-Keywords:** dream, surprisal, prompt literal, relatório

- [ ] **T009:** Criar o relógio de atividade carimbado em cada chamada de ferramenta, com teste
  - **Papel:** executor
  - **agent:** executor2
  - **Dependências:** T001
  - **depends_on:** [T001]
  - **Paralelizável:** true
  - **Arquivos:** `internal/mcp/activity_clock.go`, `internal/mcp/activity_clock_test.go`
  - **Wiki-Keywords:** relógio de atividade, chamada de ferramenta

- [ ] **T010:** Criar o agendador do dream, com o cenário 6 como teste com relógio injetado
  - **Papel:** executor
  - **agent:** executor3
  - **Dependências:** T008, T009
  - **depends_on:** [T008, T009]
  - **Paralelizável:** false
  - **Arquivos:** `internal/cli/dream_scheduler.go`, `internal/cli/dream_scheduler_test.go`
  - **Wiki-Keywords:** ociosidade, observador 2 s, cancelamento

- [ ] **T011:** Ligar o agendador do dream ao boot
  - **Papel:** executor
  - **agent:** executor1
  - **Dependências:** T010, T007
  - **depends_on:** [T010, T007]
  - **Paralelizável:** false
  - **Arquivos:** `internal/cli/serve_cmd.go`
  - **Wiki-Keywords:** boot, agendador


### Cenários Gherkin

- [ ] **T012:** Implementar Scenario: deduplicação de duas páginas frias e no-op sem embedder
  - **Papel:** executor
  - **agent:** executor2
  - **Dependências:** T011, T002
  - **depends_on:** [T011, T002]
  - **Paralelizável:** true
  - **Arquivos:** `test/acceptance/dedup_test.go`
  - **Wiki-Keywords:** cenário 5, sem embedder

- [ ] **T013:** Implementar Scenario: dream com cancelamento e sem provedor
  - **Papel:** executor
  - **agent:** executor3
  - **Dependências:** T011, T002
  - **depends_on:** [T011, T002]
  - **Paralelizável:** true
  - **Arquivos:** `test/acceptance/dream_test.go`
  - **Wiki-Keywords:** cenário 6, cancelamento, sem provedor

- [ ] **T014:** Implementar tratamento de erro: cluster recusado pela admissão e falha do LLM
  - **Papel:** executor
  - **agent:** executor1
  - **Dependências:** T011, T002
  - **depends_on:** [T011, T002]
  - **Paralelizável:** true
  - **Arquivos:** `test/acceptance/dream_skip_test.go`
  - **Wiki-Keywords:** admissão, falha do LLM, skipped


## Fase 3: Validação

- [ ] **T015:** Rodar os portões e registrar o resultado com a saída
  - **Papel:** executor
  - **agent:** executor2
  - **Dependências:** T012, T013, T014
  - **depends_on:** [T012, T013, T014]
  - **Paralelizável:** false
  - **Arquivos:** `specs/features/021-curadoria-por-embeddings/notes/gates-run.md`
  - **Wiki-Keywords:** check.sh, race, golangci-lint

- [ ] **T016:** Auditar a feature contra a constituição e contra `docs/design-memory-aging.md`
  - **Papel:** analyst
  - **agent:** analyst1
  - **Dependências:** T015
  - **depends_on:** [T015]
  - **Paralelizável:** false
  - **Arquivos:** `specs/features/021-curadoria-por-embeddings/notes/constitution-audit.md`
  - **Wiki-Keywords:** constitution, auditoria, memory-aging


## Fase 4: Documentação

- [ ] **T017:** Atualizar o README com deduplicação, banda e dream
  - **Papel:** executor
  - **agent:** executor3
  - **Dependências:** T016
  - **depends_on:** [T016]
  - **Paralelizável:** true
  - **Arquivos:** `README.md`
  - **Wiki-Keywords:** README, dedup, dream

- [ ] **T018:** Atualizar o llms.txt com a 021
  - **Papel:** executor
  - **agent:** executor1
  - **Dependências:** T017
  - **depends_on:** [T017]
  - **Paralelizável:** true
  - **Arquivos:** `llms.txt`
  - **Wiki-Keywords:** llms.txt, onboarding

- [ ] **T019:** Atualizar o .env.example com as chaves da 021
  - **Papel:** executor
  - **agent:** executor2
  - **Dependências:** T003
  - **depends_on:** [T003]
  - **Paralelizável:** true
  - **Arquivos:** `.env.example`
  - **Wiki-Keywords:** .env.example, dream



## Coordination Graph

G₀ (round 0):

- **nodes:** T001, T002, T003, T004, T005, T006, T007, T008, T009, T010, T011, T012, T013, T014, T015, T016, T017, T018, T019
- **edges:**
  - T001 → T002
  - T001 → T003
  - T001 → T004
  - T001 → T005
  - T004 → T006
  - T003 → T006
  - T004 → T007
  - T003 → T007
  - T006 → T008
  - T001 → T009
  - T008 → T010
  - T009 → T010
  - T010 → T011
  - T007 → T011
  - T011 → T012
  - T002 → T012
  - T011 → T013
  - T002 → T013
  - T011 → T014
  - T002 → T014
  - T012 → T015
  - T013 → T015
  - T014 → T015
  - T015 → T016
  - T016 → T017
  - T017 → T018
  - T003 → T019
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
  - T016: analyst1
  - T017: executor3
  - T018: executor1
  - T019: executor2
- **ready:** [T001]

> Workers: Lead, researcher1, analyst1, executor1..3. Validado por script: 19 tasks, sem ciclo, sem dependência quebrada, sem arquivo compartilhado entre tasks sem ordem. Mínimo de 11 rounds. Caminho crítico (calculado): T001 → T004 → T006 → T008 → T010 → T011 → T012 → T015 → T016 → T017 → T018.
