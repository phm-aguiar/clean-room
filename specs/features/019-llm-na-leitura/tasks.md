---
graph-operators: enabled
heartbeat-threshold: 4
max-rounds: 40
---

# tasks.md: LLM na leitura

> Feature `019-llm-na-leitura` · Spec: `spec.md` (draft) · Plano: `plan.md` (L-01 a L-04)
>
> **Provisório:** gerado em 2026-10-13, antes da execução das features anteriores. A T001 reconfere tudo contra o código entregue antes de qualquer execução. Sem dependência nova no `go.mod`.


## Fase 0: Descoberta

- [ ] **T001:** Reconferir este DAG contra o código entregue (`memory_query`, `memory_briefing`, provedor da 016, fluxo vetorial da 017) e consolidar as notas da fase 0
  - **Papel:** analyst
  - **agent:** analyst1
  - **Dependências:** Nenhuma
  - **depends_on:** []
  - **Paralelizável:** false
  - **Arquivos:** `specs/features/019-llm-na-leitura/notes/phase0-findings.md`
  - **Wiki-Keywords:** memory_query, briefing, 016 e 017 entregues


## Fase 1: Fundação

- [ ] **T002:** Criar o arquivo .feature da 019 com os cenários da spec
  - **Papel:** executor
  - **agent:** executor1
  - **Dependências:** T001
  - **depends_on:** [T001]
  - **Paralelizável:** true
  - **Arquivos:** `specs/features/019-llm-na-leitura/acceptance/llm-read.feature`
  - **Wiki-Keywords:** Gherkin, rerank, answer, explore

- [ ] **T003:** Acrescentar à configuração `reranker` (só `llm`, exige provedor), com teste
  - **Papel:** executor
  - **agent:** executor2
  - **Dependências:** T001
  - **depends_on:** [T001]
  - **Paralelizável:** true
  - **Arquivos:** `internal/config/config.go`, `internal/config/config_test.go`
  - **Wiki-Keywords:** reranker, llm obrigatório

- [ ] **T004:** Criar os prompts do reranqueador, do digest e do `answer` (este como decisão nossa), com teste da primeira frase
  - **Papel:** executor
  - **agent:** executor3
  - **Dependências:** T001
  - **depends_on:** [T001]
  - **Paralelizável:** true
  - **Arquivos:** `internal/llm/prompts/rerank.go`, `internal/llm/prompts/explore.go`, `internal/llm/prompts/answer.go`, `internal/llm/prompts/prompts_test.go`
  - **Wiki-Keywords:** prompts, primeira frase fixa, digest, answer


## Fase 2: Implementação

- [ ] **T005:** Criar o reranqueador por LLM (orçamentos em bytes, esquema, mapeamento de volta), com teste
  - **Papel:** executor
  - **agent:** executor1
  - **Dependências:** T004
  - **depends_on:** [T004]
  - **Paralelizável:** false
  - **Arquivos:** `internal/llm/rerank.go`, `internal/llm/rerank_test.go`
  - **Wiki-Keywords:** reranqueador, orçamentos, esquema, mapeamento

- [ ] **T006:** Criar a etapa de reranqueamento do `memory_query` (candidatos, semáforo, tempo, completude, ordem, `rerank_score`), com teste
  - **Papel:** executor
  - **agent:** executor2
  - **Dependências:** T005, T003
  - **depends_on:** [T005, T003]
  - **Paralelizável:** false
  - **Arquivos:** `internal/mcp/rerank_stage.go`, `internal/mcp/rerank_stage_test.go`
  - **Wiki-Keywords:** semáforo 4, 20 s, completude, rerank_score

- [ ] **T007:** Ligar o reranqueamento e o `answer` com `reasoning` ao `memory_query`, com teste
  - **Papel:** executor
  - **agent:** executor3
  - **Dependências:** T006
  - **depends_on:** [T006]
  - **Paralelizável:** false
  - **Arquivos:** `internal/mcp/tools_read.go`, `internal/mcp/tools_read_test.go`
  - **Wiki-Keywords:** answer, reasoning, answer_unavailable

- [ ] **T008:** Criar o cálculo do balde de intervalo, com teste de tabela
  - **Papel:** executor
  - **agent:** executor1
  - **Dependências:** T004
  - **depends_on:** [T004]
  - **Paralelizável:** true
  - **Arquivos:** `internal/mcp/explore_bucket.go`, `internal/mcp/explore_bucket_test.go`
  - **Wiki-Keywords:** balde de intervalo, verbosidade

- [ ] **T009:** Criar o `memory_explore` com degradação e incremento de acesso em lote, com teste
  - **Papel:** executor
  - **agent:** executor2
  - **Dependências:** T008
  - **depends_on:** [T008]
  - **Paralelizável:** false
  - **Arquivos:** `internal/mcp/tools_explore.go`, `internal/mcp/tools_explore_test.go`
  - **Wiki-Keywords:** memory_explore, prose null, acesso em lote

- [ ] **T010:** Registrar `memory_explore` e os argumentos novos no servidor MCP
  - **Papel:** executor
  - **agent:** executor3
  - **Dependências:** T007, T009
  - **depends_on:** [T007, T009]
  - **Paralelizável:** false
  - **Arquivos:** `internal/mcp/server.go`
  - **Wiki-Keywords:** registro de ferramentas


### Cenários Gherkin

- [ ] **T011:** Implementar Scenario: reranqueamento com `rerank_score` e ordem alterada
  - **Papel:** executor
  - **agent:** executor1
  - **Dependências:** T010, T002
  - **depends_on:** [T010, T002]
  - **Paralelizável:** true
  - **Arquivos:** `test/acceptance/rerank_test.go`
  - **Wiki-Keywords:** rerank, explain, ordem

- [ ] **T012:** Implementar tratamento de erro: sem provedor, resposta parcial e semáforo cheio
  - **Papel:** executor
  - **agent:** executor2
  - **Dependências:** T010, T002
  - **depends_on:** [T010, T002]
  - **Paralelizável:** true
  - **Arquivos:** `test/acceptance/llm_read_degrade_test.go`
  - **Wiki-Keywords:** sem provedor, parcial, saturado

- [ ] **T013:** Implementar edge case: `memory_explore` nos baldes `none` e `stale`
  - **Papel:** executor
  - **agent:** executor3
  - **Dependências:** T010, T002
  - **depends_on:** [T010, T002]
  - **Paralelizável:** true
  - **Arquivos:** `test/acceptance/explore_buckets_test.go`
  - **Wiki-Keywords:** baldes, none, stale


## Fase 3: Validação

- [ ] **T014:** Rodar os portões e registrar o resultado com a saída
  - **Papel:** executor
  - **agent:** executor1
  - **Dependências:** T011, T012, T013
  - **depends_on:** [T011, T012, T013]
  - **Paralelizável:** false
  - **Arquivos:** `specs/features/019-llm-na-leitura/notes/gates-run.md`
  - **Wiki-Keywords:** check.sh, race, golangci-lint

- [ ] **T015:** Auditar a feature contra a constituição e contra os docs
  - **Papel:** analyst
  - **agent:** analyst1
  - **Dependências:** T014
  - **depends_on:** [T014]
  - **Paralelizável:** false
  - **Arquivos:** `specs/features/019-llm-na-leitura/notes/constitution-audit.md`
  - **Wiki-Keywords:** constitution, auditoria, INV-13


## Fase 4: Documentação

- [ ] **T016:** Atualizar o README com rerank, `answer` e `memory_explore`
  - **Papel:** executor
  - **agent:** executor2
  - **Dependências:** T015
  - **depends_on:** [T015]
  - **Paralelizável:** true
  - **Arquivos:** `README.md`
  - **Wiki-Keywords:** README, rerank, explore

- [ ] **T017:** Atualizar o llms.txt com a 019
  - **Papel:** executor
  - **agent:** executor3
  - **Dependências:** T016
  - **depends_on:** [T016]
  - **Paralelizável:** true
  - **Arquivos:** `llms.txt`
  - **Wiki-Keywords:** llms.txt, onboarding

- [ ] **T018:** Atualizar o .env.example com `AI_MEMORY_RERANKER`
  - **Papel:** executor
  - **agent:** executor1
  - **Dependências:** T003
  - **depends_on:** [T003]
  - **Paralelizável:** true
  - **Arquivos:** `.env.example`
  - **Wiki-Keywords:** .env.example, reranker



## Coordination Graph

G₀ (round 0):

- **nodes:** T001, T002, T003, T004, T005, T006, T007, T008, T009, T010, T011, T012, T013, T014, T015, T016, T017, T018
- **edges:**
  - T001 → T002
  - T001 → T003
  - T001 → T004
  - T004 → T005
  - T005 → T006
  - T003 → T006
  - T006 → T007
  - T004 → T008
  - T008 → T009
  - T007 → T010
  - T009 → T010
  - T010 → T011
  - T002 → T011
  - T010 → T012
  - T002 → T012
  - T010 → T013
  - T002 → T013
  - T011 → T014
  - T012 → T014
  - T013 → T014
  - T014 → T015
  - T015 → T016
  - T016 → T017
  - T003 → T018
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
  - T015: analyst1
  - T016: executor2
  - T017: executor3
  - T018: executor1
- **ready:** [T001]

> Workers: Lead, researcher1, analyst1, executor1..3. Validado por script: 18 tasks, sem ciclo, sem dependência quebrada, sem arquivo compartilhado entre tasks sem ordem. Mínimo de 11 rounds. Caminho crítico (calculado): T001 → T004 → T005 → T006 → T007 → T010 → T011 → T014 → T015 → T016 → T017.
