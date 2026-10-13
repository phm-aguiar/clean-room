---
graph-operators: enabled
heartbeat-threshold: 4
max-rounds: 40
---

# tasks.md: Provedores por assinatura

> Feature `018-provedores-por-assinatura` · Spec: `spec.md` (draft) · Plano: `plan.md` (O-01 a O-04)
>
> **Provisório:** gerado em 2026-10-13, antes da execução das features anteriores. A T001 reconfere tudo contra o código entregue antes de qualquer execução. Sem dependência nova no `go.mod`.


## Fase 0: Descoberta

- [ ] **T001:** Reconferir este DAG contra o código entregue (provedor e cadeia da 016, `auth.json` e `auth` da 009) e consolidar as notas da fase 0
  - **Papel:** analyst
  - **agent:** analyst1
  - **Dependências:** Nenhuma
  - **depends_on:** []
  - **Paralelizável:** false
  - **Arquivos:** `specs/features/018-provedores-por-assinatura/notes/phase0-findings.md`
  - **Wiki-Keywords:** 016 e 009 entregues, auth.json

- [ ] **T002:** Extrair os contratos de login, renovação e recuperação; registrar como decisão nossa o protocolo do `auth login openai-oauth` se os docs não o descreverem
  - **Papel:** researcher
  - **agent:** researcher1
  - **Dependências:** Nenhuma
  - **depends_on:** []
  - **Paralelizável:** true
  - **Arquivos:** `specs/features/018-provedores-por-assinatura/notes/oauth-contracts.md`
  - **Wiki-Keywords:** openai-oauth login, copilot device flow, codex app-server


## Fase 1: Fundação

- [ ] **T003:** Criar o arquivo .feature da 018 com os cenários da spec
  - **Papel:** executor
  - **agent:** executor1
  - **Dependências:** T001, T002
  - **depends_on:** [T001, T002]
  - **Paralelizável:** true
  - **Arquivos:** `specs/features/018-provedores-por-assinatura/acceptance/subscription-providers.feature`
  - **Wiki-Keywords:** Gherkin, login, renovação, 401

- [ ] **T004:** Acrescentar à configuração os quatro provedores, aliases, modelos padrão e a resolução de credenciais e arquivos, com teste
  - **Papel:** executor
  - **agent:** executor2
  - **Dependências:** T001
  - **depends_on:** [T001]
  - **Paralelizável:** true
  - **Arquivos:** `internal/config/config.go`, `internal/config/config_test.go`
  - **Wiki-Keywords:** aliases, credenciais de assinatura, CODEX_HOME


## Fase 2: Implementação

- [ ] **T005:** Criar o leitor do stream de Responses, com teste
  - **Papel:** executor
  - **agent:** executor3
  - **Dependências:** T001
  - **depends_on:** [T001]
  - **Paralelizável:** true
  - **Arquivos:** `internal/llm/responses_stream.go`, `internal/llm/responses_stream_test.go`
  - **Wiki-Keywords:** SSE, conclusão, 502 sintético, truncado

- [ ] **T006:** Criar a renovação OAuth com persistência no `auth.json`, com teste com relógio injetado
  - **Papel:** executor
  - **agent:** executor1
  - **Dependências:** T001
  - **depends_on:** [T001]
  - **Paralelizável:** true
  - **Arquivos:** `internal/llm/oauth_refresh.go`, `internal/llm/oauth_refresh_test.go`
  - **Wiki-Keywords:** refresh grant, 60 s, auth.json

- [ ] **T007:** Criar o provedor `anthropic-oauth`, com teste
  - **Papel:** executor
  - **agent:** executor2
  - **Dependências:** T004
  - **depends_on:** [T004]
  - **Paralelizável:** false
  - **Arquivos:** `internal/llm/anthropic_oauth.go`, `internal/llm/anthropic_oauth_test.go`
  - **Wiki-Keywords:** anthropic-oauth, bearer, beta

- [ ] **T008:** Criar o provedor `openai-oauth`, com teste
  - **Papel:** executor
  - **agent:** executor3
  - **Dependências:** T005, T006
  - **depends_on:** [T005, T006]
  - **Paralelizável:** false
  - **Arquivos:** `internal/llm/openai_oauth.go`, `internal/llm/openai_oauth_test.go`
  - **Wiki-Keywords:** openai-oauth, chatgpt-account-id

- [ ] **T009:** Criar o provedor `codex` com a recuperação de 401 serializada e limitada, com teste com executável falso
  - **Papel:** executor
  - **agent:** executor1
  - **Dependências:** T005
  - **depends_on:** [T005]
  - **Paralelizável:** false
  - **Arquivos:** `internal/llm/codex.go`, `internal/llm/codex_test.go`
  - **Wiki-Keywords:** codex, recuperação de 401, app-server

- [ ] **T010:** Criar o provedor `copilot` (troca de token, cache, escolha de endpoint), com teste
  - **Papel:** executor
  - **agent:** executor2
  - **Dependências:** T005, T006
  - **depends_on:** [T005, T006]
  - **Paralelizável:** false
  - **Arquivos:** `internal/llm/copilot.go`, `internal/llm/copilot_test.go`
  - **Wiki-Keywords:** copilot, troca de token, catálogo

- [ ] **T011:** Criar o embedder `copilot`, com teste
  - **Papel:** executor
  - **agent:** executor3
  - **Dependências:** T010
  - **depends_on:** [T010]
  - **Paralelizável:** false
  - **Arquivos:** `internal/embed/copilot.go`, `internal/embed/copilot_test.go`
  - **Wiki-Keywords:** embedder copilot, 1536

- [ ] **T012:** Ligar os quatro provedores e o embedder à construção e aos perfis de contingência, com teste
  - **Papel:** executor
  - **agent:** executor1
  - **Dependências:** T007, T008, T009, T010, T011
  - **depends_on:** [T007, T008, T009, T010, T011]
  - **Paralelizável:** false
  - **Arquivos:** `internal/llm/build.go`, `internal/llm/build_test.go`, `internal/embed/build.go`
  - **Wiki-Keywords:** construção, fallbacks sem api_key_env

- [ ] **T013:** Estender `auth login`, `auth logout` e `auth status` com `openai-oauth` e `copilot`, com teste
  - **Papel:** executor
  - **agent:** executor2
  - **Dependências:** T006, T010
  - **depends_on:** [T006, T010]
  - **Paralelizável:** false
  - **Arquivos:** `internal/cli/auth_cmd.go`, `internal/cli/auth_cmd_test.go`
  - **Wiki-Keywords:** auth login openai-oauth, copilot, logout, status

- [ ] **T014:** Acrescentar aos wrappers o repasse das variáveis desses provedores
  - **Papel:** executor
  - **agent:** executor3
  - **Dependências:** T012, T013
  - **depends_on:** [T012, T013]
  - **Paralelizável:** false
  - **Arquivos:** `contrib/wrapper/ai-memory.sh`, `contrib/wrapper/ai-memory.ps1`
  - **Wiki-Keywords:** wrapper, repasse de variáveis


### Cenários Gherkin

- [ ] **T015:** Implementar Scenario: login do Copilot e `llm-test` estruturado
  - **Papel:** executor
  - **agent:** executor1
  - **Dependências:** T014, T003
  - **depends_on:** [T014, T003]
  - **Paralelizável:** true
  - **Arquivos:** `test/acceptance/copilot_login_test.go`
  - **Wiki-Keywords:** copilot, device flow, llm-test

- [ ] **T016:** Implementar tratamento de erro: renovação recusada e refresh token substituído
  - **Papel:** executor
  - **agent:** executor2
  - **Dependências:** T014, T003
  - **depends_on:** [T014, T003]
  - **Paralelizável:** true
  - **Arquivos:** `test/acceptance/oauth_refresh_test.go`
  - **Wiki-Keywords:** renovação, refresh substituído, recusa

- [ ] **T017:** Implementar edge case: dois 401 concorrentes no `codex` com uma única recuperação
  - **Papel:** executor
  - **agent:** executor3
  - **Dependências:** T009, T003
  - **depends_on:** [T009, T003]
  - **Paralelizável:** true
  - **Arquivos:** `test/acceptance/codex_recovery_test.go`
  - **Wiki-Keywords:** 401 concorrente, recuperação única


## Fase 3: Validação

- [ ] **T018:** Rodar os portões e registrar o resultado com a saída
  - **Papel:** executor
  - **agent:** executor1
  - **Dependências:** T015, T016, T017
  - **depends_on:** [T015, T016, T017]
  - **Paralelizável:** false
  - **Arquivos:** `specs/features/018-provedores-por-assinatura/notes/gates-run.md`
  - **Wiki-Keywords:** check.sh, race, golangci-lint

- [ ] **T019:** Auditar a feature contra a constituição e contra os docs dos provedores
  - **Papel:** analyst
  - **agent:** analyst1
  - **Dependências:** T018
  - **depends_on:** [T018]
  - **Paralelizável:** false
  - **Arquivos:** `specs/features/018-provedores-por-assinatura/notes/constitution-audit.md`
  - **Wiki-Keywords:** constitution, auditoria, INV-14


## Fase 4: Documentação

- [ ] **T020:** Atualizar o README com os provedores por assinatura e o aviso do `anthropic-oauth`
  - **Papel:** executor
  - **agent:** executor2
  - **Dependências:** T019
  - **depends_on:** [T019]
  - **Paralelizável:** true
  - **Arquivos:** `README.md`
  - **Wiki-Keywords:** README, assinatura, aviso

- [ ] **T021:** Atualizar o llms.txt com a 018
  - **Papel:** executor
  - **agent:** executor3
  - **Dependências:** T020
  - **depends_on:** [T020]
  - **Paralelizável:** true
  - **Arquivos:** `llms.txt`
  - **Wiki-Keywords:** llms.txt, onboarding

- [ ] **T022:** Atualizar o .env.example com as chaves da 018
  - **Papel:** executor
  - **agent:** executor1
  - **Dependências:** T004
  - **depends_on:** [T004]
  - **Paralelizável:** true
  - **Arquivos:** `.env.example`
  - **Wiki-Keywords:** .env.example, assinatura



## Coordination Graph

G₀ (round 0):

- **nodes:** T001, T002, T003, T004, T005, T006, T007, T008, T009, T010, T011, T012, T013, T014, T015, T016, T017, T018, T019, T020, T021, T022
- **edges:**
  - T001 → T003
  - T002 → T003
  - T001 → T004
  - T001 → T005
  - T001 → T006
  - T004 → T007
  - T005 → T008
  - T006 → T008
  - T005 → T009
  - T005 → T010
  - T006 → T010
  - T010 → T011
  - T007 → T012
  - T008 → T012
  - T009 → T012
  - T010 → T012
  - T011 → T012
  - T006 → T013
  - T010 → T013
  - T012 → T014
  - T013 → T014
  - T014 → T015
  - T003 → T015
  - T014 → T016
  - T003 → T016
  - T009 → T017
  - T003 → T017
  - T015 → T018
  - T016 → T018
  - T017 → T018
  - T018 → T019
  - T019 → T020
  - T020 → T021
  - T004 → T022
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
  - T019: analyst1
  - T020: executor2
  - T021: executor3
  - T022: executor1
- **ready:** [T001, T002]

> Workers: Lead, researcher1, analyst1, executor1..3. Validado por script: 22 tasks, sem ciclo, sem dependência quebrada, sem arquivo compartilhado entre tasks sem ordem. Mínimo de 11 rounds. Caminho crítico (calculado): T001 → T005 → T010 → T011 → T012 → T014 → T015 → T018 → T019 → T020 → T021.
