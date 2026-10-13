---
graph-operators: enabled
heartbeat-threshold: 4
max-rounds: 40
---

# tasks.md: Mensagens entre projetos

> Feature `011-mensagens-entre-projetos` · Spec: `spec.md` (draft) · Plano: `plan.md` (M2-01 a M2-05)
>
> **Provisório:** gerado em 2026-10-13, antes da execução das features anteriores. A T001 reconfere tudo contra o código entregue antes de qualquer execução.


## Fase 0: Descoberta

- [ ] **T001:** Reconferir este DAG contra o código entregue (MCP, admissão, /handoff, briefing) e ajustar as tasks antes de executar
  - **Papel:** analyst
  - **agent:** analyst1
  - **Dependências:** Nenhuma
  - **depends_on:** []
  - **Paralelizável:** false
  - **Arquivos:** `specs/features/011-mensagens-entre-projetos/notes/phase0-findings.md`
  - **Wiki-Keywords:** agent-messaging, código entregue


## Fase 1: Fundação

- [ ] **T002:** Criar o arquivo .feature da 011 com os cenários da spec
  - **Papel:** executor
  - **agent:** executor1
  - **Dependências:** T001
  - **depends_on:** [T001]
  - **Paralelizável:** true
  - **Arquivos:** `specs/features/011-mensagens-entre-projetos/acceptance/mensagens.feature`
  - **Wiki-Keywords:** Gherkin, mensagens


## Fase 2: Implementação

- [ ] **T003:** Criar a fila de mensagens (envio com capacidade, listagem, pop atômico, cancelamento, auditoria), com teste multi-sessão
  - **Papel:** executor
  - **agent:** executor2
  - **Dependências:** T001
  - **depends_on:** [T001]
  - **Paralelizável:** true
  - **Arquivos:** `internal/storage/messages.go`, `internal/storage/messages_test.go`
  - **Wiki-Keywords:** fila, reivindicação atômica, 256 pendentes

- [ ] **T004:** Criar os cortes de corpo e assunto por caminho (caracteres com marcador, bytes sem marcador, 16 KiB), com teste
  - **Papel:** executor
  - **agent:** executor3
  - **Dependências:** T001
  - **depends_on:** [T001]
  - **Paralelizável:** true
  - **Arquivos:** `internal/domain/messagecaps.go`, `internal/domain/messagecaps_test.go`
  - **Wiki-Keywords:** cortes, caracteres, bytes, marcador

- [ ] **T005:** Ligar `message_send`, `message_pop` e `message_cancel` à cadeia de admissão (decidir, agir, notificar), com teste
  - **Papel:** executor
  - **agent:** executor1
  - **Dependências:** T003
  - **depends_on:** [T003]
  - **Paralelizável:** true
  - **Arquivos:** `internal/wiki/message_admission.go`, `internal/wiki/message_admission_test.go`
  - **Wiki-Keywords:** admissão, message_send, pop, cancel

- [ ] **T006:** Criar as quatro tools de mensagem, com teste
  - **Papel:** executor
  - **agent:** executor2
  - **Dependências:** T004, T005
  - **depends_on:** [T004, T005]
  - **Paralelizável:** false
  - **Arquivos:** `internal/mcp/tools_messages.go`, `internal/mcp/tools_messages_test.go`
  - **Wiki-Keywords:** memory_message_send, list, pop, cancel, security_notice

- [ ] **T007:** Criar as rotas administrativas de mensagem, com teste
  - **Papel:** executor
  - **agent:** executor3
  - **Dependências:** T004, T005
  - **depends_on:** [T004, T005]
  - **Paralelizável:** false
  - **Arquivos:** `internal/cli/admin_messages.go`, `internal/cli/admin_messages_test.go`
  - **Wiki-Keywords:** /admin/messages, 409, 400

- [ ] **T008:** Montar as rotas da 011 no servidor HTTP
  - **Papel:** executor
  - **agent:** executor1
  - **Dependências:** T007
  - **depends_on:** [T007]
  - **Paralelizável:** false
  - **Arquivos:** `internal/cli/httpserver.go`
  - **Wiki-Keywords:** rotas, montagem

- [ ] **T009:** Criar o comando `message`, com teste
  - **Papel:** executor
  - **agent:** executor2
  - **Dependências:** T008
  - **depends_on:** [T008]
  - **Paralelizável:** true
  - **Arquivos:** `internal/cli/message_cmd.go`, `internal/cli/message_cmd_test.go`
  - **Wiki-Keywords:** message send, list, pop, cancel, stdin

- [ ] **T010:** Registrar o comando `message` na raiz da CLI
  - **Papel:** executor
  - **agent:** executor3
  - **Dependências:** T009
  - **depends_on:** [T009]
  - **Paralelizável:** false
  - **Arquivos:** `internal/cli/root.go`
  - **Wiki-Keywords:** cobra, raiz da CLI

- [ ] **T011:** Acrescentar o aviso de caixa de entrada (só contagem) ao `GET /handoff`, com teste de corpo hostil
  - **Papel:** executor
  - **agent:** executor1
  - **Dependências:** T003
  - **depends_on:** [T003]
  - **Paralelizável:** true
  - **Arquivos:** `internal/ingest/handoff_route.go`, `internal/ingest/handoff_route_test.go`
  - **Wiki-Keywords:** aviso de caixa de entrada, contagem

- [ ] **T012:** Acrescentar `pending_message_count` ao `memory_briefing` de projeto, com teste
  - **Papel:** executor
  - **agent:** executor2
  - **Dependências:** T006
  - **depends_on:** [T006]
  - **Paralelizável:** false
  - **Arquivos:** `internal/mcp/tools_session.go`, `internal/mcp/tools_session_test.go`
  - **Wiki-Keywords:** pending_message_count, briefing

- [ ] **T013:** Implementar Scenario: envio, aviso no início de sessão e pop com proveniência e aviso de segurança
  - **Papel:** executor
  - **agent:** executor3
  - **Dependências:** T006, T002
  - **depends_on:** [T006, T002]
  - **Paralelizável:** true
  - **Arquivos:** `test/acceptance/messages_test.go`
  - **Wiki-Keywords:** envio, pop, aviso, isolamento

- [ ] **T014:** Implementar tratamento de erro: destinatário inexistente, caixa cheia, corpo vazio e recusa de admissão
  - **Papel:** executor
  - **agent:** executor1
  - **Dependências:** T006, T002
  - **depends_on:** [T006, T002]
  - **Paralelizável:** true
  - **Arquivos:** `test/acceptance/messages_failures_test.go`
  - **Wiki-Keywords:** destinatário inexistente, caixa cheia, corpo vazio

- [ ] **T015:** Implementar edge case: dois pops simultâneos com um vencedor
  - **Papel:** executor
  - **agent:** executor2
  - **Dependências:** T006, T002
  - **depends_on:** [T006, T002]
  - **Paralelizável:** true
  - **Arquivos:** `test/acceptance/messages_race_test.go`
  - **Wiki-Keywords:** corrida de pop, -race


## Fase 3: Validação

- [ ] **T016:** Rodar os portões e registrar o resultado com a saída
  - **Papel:** executor
  - **agent:** executor3
  - **Dependências:** T010, T011, T012, T013, T014, T015
  - **depends_on:** [T010, T011, T012, T013, T014, T015]
  - **Paralelizável:** false
  - **Arquivos:** `specs/features/011-mensagens-entre-projetos/notes/gates-run.md`
  - **Wiki-Keywords:** check.sh, race, golangci-lint

- [ ] **T017:** Auditar a feature contra a constituição e contra `docs/agent-messaging.md`
  - **Papel:** analyst
  - **agent:** analyst1
  - **Dependências:** T016
  - **depends_on:** [T016]
  - **Paralelizável:** false
  - **Arquivos:** `specs/features/011-mensagens-entre-projetos/notes/constitution-audit.md`
  - **Wiki-Keywords:** constitution, auditoria, agent-messaging


## Fase 4: Documentação

- [ ] **T018:** Atualizar o README com as mensagens entre projetos
  - **Papel:** executor
  - **agent:** executor1
  - **Dependências:** T017
  - **depends_on:** [T017]
  - **Paralelizável:** true
  - **Arquivos:** `README.md`
  - **Wiki-Keywords:** README, mensagens

- [ ] **T019:** Atualizar o llms.txt com a 011
  - **Papel:** executor
  - **agent:** executor2
  - **Dependências:** T018
  - **depends_on:** [T018]
  - **Paralelizável:** true
  - **Arquivos:** `llms.txt`
  - **Wiki-Keywords:** llms.txt, onboarding



## Coordination Graph

G₀ (round 0):

- **nodes:** T001, T002, T003, T004, T005, T006, T007, T008, T009, T010, T011, T012, T013, T014, T015, T016, T017, T018, T019
- **edges:**
  - T001 → T002
  - T001 → T003
  - T001 → T004
  - T003 → T005
  - T004 → T006
  - T005 → T006
  - T004 → T007
  - T005 → T007
  - T007 → T008
  - T008 → T009
  - T009 → T010
  - T003 → T011
  - T006 → T012
  - T006 → T013
  - T002 → T013
  - T006 → T014
  - T002 → T014
  - T006 → T015
  - T002 → T015
  - T010 → T016
  - T011 → T016
  - T012 → T016
  - T013 → T016
  - T014 → T016
  - T015 → T016
  - T016 → T017
  - T017 → T018
  - T018 → T019
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
  - T017: analyst1
  - T018: executor1
  - T019: executor2
- **ready:** [T001]

> Workers: Lead, analyst1, executor1..3. Validado por script: 19 tasks, sem ciclo, sem dependência quebrada, sem arquivo compartilhado entre tasks sem ordem. Mínimo de 11 rounds. Caminho crítico (calculado): T001 → T003 → T005 → T007 → T008 → T009 → T010 → T016 → T017 → T018 → T019.
