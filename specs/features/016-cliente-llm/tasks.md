---
graph-operators: enabled
heartbeat-threshold: 4
max-rounds: 40
---

# tasks.md: Cliente LLM e provedores por chave

> Feature `016-cliente-llm` · Spec: `spec.md` (draft) · Plano: `plan.md` (C-01 a C-06)
>
> **Provisório:** gerado em 2026-10-13, antes da execução das features anteriores. A T001 reconfere tudo contra o código entregue antes de qualquer execução. Sem dependência nova no `go.mod`.


## Fase 0: Descoberta

- [ ] **T001:** Reconferir este DAG contra o código entregue (configuração, `/admin/status`, `status`) e consolidar as notas da fase 0
  - **Papel:** analyst
  - **agent:** analyst1
  - **Dependências:** Nenhuma
  - **depends_on:** []
  - **Paralelizável:** false
  - **Arquivos:** `specs/features/016-cliente-llm/notes/phase0-findings.md`
  - **Wiki-Keywords:** llm-client, configuração entregue, status

- [ ] **T002:** Extrair por provedor o protocolo, o formato da saída estruturada, o mapeamento de esforço e os cabeçalhos numa tabela regra → teste
  - **Papel:** researcher
  - **agent:** researcher1
  - **Dependências:** Nenhuma
  - **depends_on:** []
  - **Paralelizável:** true
  - **Arquivos:** `specs/features/016-cliente-llm/notes/provider-wire-contracts.md`
  - **Wiki-Keywords:** anthropic, openai, gemini, openai-compat, opencode


## Fase 1: Fundação

- [ ] **T003:** Criar o arquivo .feature da 016 com os cenários da spec
  - **Papel:** executor
  - **agent:** executor1
  - **Dependências:** T001, T002
  - **depends_on:** [T001, T002]
  - **Paralelizável:** true
  - **Arquivos:** `specs/features/016-cliente-llm/acceptance/llm-client.feature`
  - **Wiki-Keywords:** Gherkin, llm-test, fallback

- [ ] **T004:** Acrescentar à configuração `llm_*`, aliases, modelos padrão, credenciais por provedor, `llm_headers` e `[[llm_fallbacks]]` com as validações de carga e a pendência de credencial, com teste
  - **Papel:** executor
  - **agent:** executor2
  - **Dependências:** T001
  - **depends_on:** [T001]
  - **Paralelizável:** true
  - **Arquivos:** `internal/config/config.go`, `internal/config/config_test.go`
  - **Wiki-Keywords:** llm_*, llm_fallbacks, credenciais, llm_fallback_unresolved


## Fase 2: Implementação

### 2A Núcleo

- [ ] **T005:** Criar os tipos neutros, o id de operação UUIDv7 e as categorias de erro com a classificação transitório ou determinístico, com teste
  - **Papel:** executor
  - **agent:** executor3
  - **Dependências:** T001
  - **depends_on:** [T001]
  - **Paralelizável:** true
  - **Arquivos:** `internal/llm/types.go`, `internal/llm/types_test.go`, `internal/llm/errors.go`, `internal/llm/errors_test.go`
  - **Wiki-Keywords:** tipos neutros, id de operação, classes de erro, transitório

- [ ] **T006:** Criar a derivação de esquema por reflexão, a normalização estrita do OpenAI e a preparação do Gemini, com teste
  - **Papel:** executor
  - **agent:** executor1
  - **Dependências:** T005
  - **depends_on:** [T005]
  - **Paralelizável:** false
  - **Arquivos:** `internal/llm/schema.go`, `internal/llm/schema_test.go`
  - **Wiki-Keywords:** JSON Schema, reflexão, normalização estrita, Gemini

- [ ] **T007:** Criar o parse e a validação de `llm_headers` e o mapeamento de esforço por destino, com teste
  - **Papel:** executor
  - **agent:** executor2
  - **Dependências:** T005
  - **depends_on:** [T005]
  - **Paralelizável:** true
  - **Arquivos:** `internal/llm/headers.go`, `internal/llm/headers_test.go`, `internal/llm/effort.go`, `internal/llm/effort_test.go`
  - **Wiki-Keywords:** llm_headers, reservados, esforço, mapeamento

- [ ] **T008:** Criar a leitura limitada de respostas e de corpos de erro e o `User-Agent` padrão, com teste
  - **Papel:** executor
  - **agent:** executor3
  - **Dependências:** T005
  - **depends_on:** [T005]
  - **Paralelizável:** true
  - **Arquivos:** `internal/llm/httpbody.go`, `internal/llm/httpbody_test.go`
  - **Wiki-Keywords:** 16 MiB, 1 MiB, 1 KiB, User-Agent

- [ ] **T009:** Criar a interface `Provider` e o wrapper tipado, com teste
  - **Papel:** executor
  - **agent:** executor1
  - **Dependências:** T006, T007, T008
  - **depends_on:** [T006, T007, T008]
  - **Paralelizável:** false
  - **Arquivos:** `internal/llm/provider.go`, `internal/llm/typed.go`, `internal/llm/typed_test.go`
  - **Wiki-Keywords:** Provider, wrapper tipado


### 2B Provedores

- [ ] **T010:** Criar o provedor `anthropic`, com teste contra servidor falso
  - **Papel:** executor
  - **agent:** executor2
  - **Dependências:** T009
  - **depends_on:** [T009]
  - **Paralelizável:** true
  - **Arquivos:** `internal/llm/anthropic.go`, `internal/llm/anthropic_test.go`
  - **Wiki-Keywords:** anthropic, ferramenta result, temperatura, esforço

- [ ] **T011:** Criar o provedor `openai`, com teste contra servidor falso
  - **Papel:** executor
  - **agent:** executor3
  - **Dependências:** T009
  - **depends_on:** [T009]
  - **Paralelizável:** true
  - **Arquivos:** `internal/llm/openai.go`, `internal/llm/openai_test.go`
  - **Wiki-Keywords:** openai, json_schema estrito, gpt-5

- [ ] **T012:** Criar o provedor `gemini`, com teste contra servidor falso
  - **Papel:** executor
  - **agent:** executor1
  - **Dependências:** T009
  - **depends_on:** [T009]
  - **Paralelizável:** true
  - **Arquivos:** `internal/llm/gemini.go`, `internal/llm/gemini_test.go`
  - **Wiki-Keywords:** gemini, responseSchema, thinkingBudget

- [ ] **T013:** Criar o provedor `openai-compat` com o modo estrito, a recaída e o modo tolerante, com teste
  - **Papel:** executor
  - **agent:** executor2
  - **Dependências:** T009
  - **depends_on:** [T009]
  - **Paralelizável:** true
  - **Arquivos:** `internal/llm/openai_compat.go`, `internal/llm/openai_compat_test.go`, `internal/llm/tolerant.go`, `internal/llm/tolerant_test.go`
  - **Wiki-Keywords:** openai-compat, estrito, tolerante, objeto balanceado

- [ ] **T014:** Criar o provedor `opencode`, com teste contra servidor falso
  - **Papel:** executor
  - **agent:** executor3
  - **Dependências:** T013
  - **depends_on:** [T013]
  - **Paralelizável:** false
  - **Arquivos:** `internal/llm/opencode.go`, `internal/llm/opencode_test.go`
  - **Wiki-Keywords:** opencode, Go, Zen, escolha de protocolo

- [ ] **T015:** Criar a construção do provedor a partir da configuração (base URL permitida, `LLM_BASE_URL`, modelo padrão), com teste
  - **Papel:** executor
  - **agent:** executor1
  - **Dependências:** T010, T011, T012, T014, T004
  - **depends_on:** [T010, T011, T012, T014, T004]
  - **Paralelizável:** false
  - **Arquivos:** `internal/llm/build.go`, `internal/llm/build_test.go`
  - **Wiki-Keywords:** construção por provedor, base URL, LLM_BASE_URL


### 2C Cadeia, saúde e comandos

- [ ] **T016:** Criar a cadeia de contingência com circuito por posição e erro agregado, com teste com relógio injetado
  - **Papel:** executor
  - **agent:** executor2
  - **Dependências:** T015
  - **depends_on:** [T015]
  - **Paralelizável:** false
  - **Arquivos:** `internal/llm/chain.go`, `internal/llm/chain_test.go`
  - **Wiki-Keywords:** fallback, circuito 30 s, erro agregado

- [ ] **T017:** Criar a saúde passiva por papel e por candidato, com teste
  - **Papel:** executor
  - **agent:** executor3
  - **Dependências:** T016
  - **depends_on:** [T016]
  - **Paralelizável:** false
  - **Arquivos:** `internal/llm/health.go`, `internal/llm/health_test.go`
  - **Wiki-Keywords:** saúde passiva, llm_candidates

- [ ] **T018:** Montar o provedor ou a cadeia no `serve` (recusa com pendência), expor `llm` e `llm_candidates` no `/admin/status` e imprimir a cadeia no `status`
  - **Papel:** executor
  - **agent:** executor1
  - **Dependências:** T017
  - **depends_on:** [T017]
  - **Paralelizável:** false
  - **Arquivos:** `internal/cli/serve_cmd.go`, `internal/cli/admin_status.go`, `internal/cli/status_cmd.go`
  - **Wiki-Keywords:** serve, pendência de credencial, status

- [ ] **T019:** Criar o comando `llm-test`, com teste
  - **Papel:** executor
  - **agent:** executor2
  - **Dependências:** T015
  - **depends_on:** [T015]
  - **Paralelizável:** true
  - **Arquivos:** `internal/cli/llm_test_cmd.go`, `internal/cli/llm_test_cmd_test.go`
  - **Wiki-Keywords:** llm-test, --structured, --api-key

- [ ] **T020:** Registrar `llm-test` na raiz da CLI
  - **Papel:** executor
  - **agent:** executor3
  - **Dependências:** T018, T019
  - **depends_on:** [T018, T019]
  - **Paralelizável:** false
  - **Arquivos:** `internal/cli/root.go`
  - **Wiki-Keywords:** cobra, raiz da CLI


### Cenários Gherkin

- [ ] **T021:** Implementar Scenario: primário com 503, resposta do fallback e cadeia no `status`
  - **Papel:** executor
  - **agent:** executor1
  - **Dependências:** T020, T003
  - **depends_on:** [T020, T003]
  - **Paralelizável:** true
  - **Arquivos:** `test/acceptance/llm_fallback_test.go`
  - **Wiki-Keywords:** fallback, circuito, status

- [ ] **T022:** Implementar tratamento de erro: cabeçalho reservado, perfil inválido e credencial pendente recusando o `serve`
  - **Papel:** executor
  - **agent:** executor2
  - **Dependências:** T020, T003
  - **depends_on:** [T020, T003]
  - **Paralelizável:** true
  - **Arquivos:** `test/acceptance/llm_config_errors_test.go`
  - **Wiki-Keywords:** llm_headers, api_key_env, serve recusa

- [ ] **T023:** Implementar edge case: recaída para o modo tolerante com bloco de raciocínio e cerca de código
  - **Papel:** executor
  - **agent:** executor3
  - **Dependências:** T013, T003
  - **depends_on:** [T013, T003]
  - **Paralelizável:** true
  - **Arquivos:** `test/acceptance/llm_tolerant_test.go`
  - **Wiki-Keywords:** tolerante, think, cerca


## Fase 3: Validação

- [ ] **T024:** Rodar os portões e registrar o resultado com a saída
  - **Papel:** executor
  - **agent:** executor1
  - **Dependências:** T021, T022, T023
  - **depends_on:** [T021, T022, T023]
  - **Paralelizável:** false
  - **Arquivos:** `specs/features/016-cliente-llm/notes/gates-run.md`
  - **Wiki-Keywords:** check.sh, race, golangci-lint

- [ ] **T025:** Auditar a feature contra a constituição e contra os docs do cliente LLM
  - **Papel:** analyst
  - **agent:** analyst1
  - **Dependências:** T024
  - **depends_on:** [T024]
  - **Paralelizável:** false
  - **Arquivos:** `specs/features/016-cliente-llm/notes/constitution-audit.md`
  - **Wiki-Keywords:** constitution, auditoria, INV-07, INV-14


## Fase 4: Documentação

- [ ] **T026:** Atualizar o README com a configuração de provedor e o `llm-test`
  - **Papel:** executor
  - **agent:** executor2
  - **Dependências:** T025
  - **depends_on:** [T025]
  - **Paralelizável:** true
  - **Arquivos:** `README.md`
  - **Wiki-Keywords:** README, LLM

- [ ] **T027:** Atualizar o llms.txt com a 016
  - **Papel:** executor
  - **agent:** executor3
  - **Dependências:** T026
  - **depends_on:** [T026]
  - **Paralelizável:** true
  - **Arquivos:** `llms.txt`
  - **Wiki-Keywords:** llms.txt, onboarding

- [ ] **T028:** Atualizar o .env.example com as chaves da 016
  - **Papel:** executor
  - **agent:** executor1
  - **Dependências:** T004
  - **depends_on:** [T004]
  - **Paralelizável:** true
  - **Arquivos:** `.env.example`
  - **Wiki-Keywords:** .env.example, LLM



## Coordination Graph

G₀ (round 0):

- **nodes:** T001, T002, T003, T004, T005, T006, T007, T008, T009, T010, T011, T012, T013, T014, T015, T016, T017, T018, T019, T020, T021, T022, T023, T024, T025, T026, T027, T028
- **edges:**
  - T001 → T003
  - T002 → T003
  - T001 → T004
  - T001 → T005
  - T005 → T006
  - T005 → T007
  - T005 → T008
  - T006 → T009
  - T007 → T009
  - T008 → T009
  - T009 → T010
  - T009 → T011
  - T009 → T012
  - T009 → T013
  - T013 → T014
  - T010 → T015
  - T011 → T015
  - T012 → T015
  - T014 → T015
  - T004 → T015
  - T015 → T016
  - T016 → T017
  - T017 → T018
  - T015 → T019
  - T018 → T020
  - T019 → T020
  - T020 → T021
  - T003 → T021
  - T020 → T022
  - T003 → T022
  - T013 → T023
  - T003 → T023
  - T021 → T024
  - T022 → T024
  - T023 → T024
  - T024 → T025
  - T025 → T026
  - T026 → T027
  - T004 → T028
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
  - T025: analyst1
  - T026: executor2
  - T027: executor3
  - T028: executor1
- **ready:** [T001, T002]

> Workers: Lead, researcher1, analyst1, executor1..3. Validado por script: 28 tasks, sem ciclo, sem dependência quebrada, sem arquivo compartilhado entre tasks sem ordem. Mínimo de 16 rounds. Caminho crítico (calculado): T001 → T005 → T006 → T009 → T013 → T014 → T015 → T016 → T017 → T018 → T020 → T021 → T024 → T025 → T026 → T027.
