---
graph-operators: enabled
heartbeat-threshold: 4
max-rounds: 40
---

# tasks.md: Workstreams gerenciados

> Feature `013-workstreams-gerenciados` · Spec: `spec.md` (draft) · Plano: `plan.md` (W-01 a W-06)
>
> **Provisório:** gerado em 2026-10-13, antes da execução da 002 e da 010. A fase 0 extrai os contratos por harness; a T005 reescreve as tasks da fase 2 com eles.


## Fase 0: Descoberta

- [ ] **T001:** Reconferir este DAG contra o código entregue (adaptadores da 010, /handoff da 002, acesso por projeto da 007) e consolidar as notas da fase 0
  - **Papel:** analyst
  - **agent:** analyst1
  - **Dependências:** Nenhuma
  - **depends_on:** []
  - **Paralelizável:** false
  - **Arquivos:** `specs/features/013-workstreams-gerenciados/notes/phase0-findings.md`
  - **Wiki-Keywords:** managed-workstreams, 010 entregue

- [ ] **T002:** Extrair por harness o planejamento do comando, seletores nativos, flag de yolo, diretório de sessão e harnesses sem hook de fim
  - **Papel:** researcher
  - **agent:** researcher1
  - **Dependências:** Nenhuma
  - **depends_on:** []
  - **Paralelizável:** true
  - **Arquivos:** `specs/features/013-workstreams-gerenciados/notes/harness-launch-contracts.md`
  - **Wiki-Keywords:** harness-adapters 1, seletores, yolo, fim de sessão

- [ ] **T003:** Extrair o contrato do yolo e do ai-jail (oferta, reexecução, toggles, tabela de decisão, por sistema operacional)
  - **Papel:** researcher
  - **agent:** researcher2
  - **Dependências:** Nenhuma
  - **depends_on:** []
  - **Paralelizável:** true
  - **Arquivos:** `specs/features/013-workstreams-gerenciados/notes/jail-contract.md`
  - **Wiki-Keywords:** ai-jail, oferta, reexecução, toggles

- [ ] **T004:** Extrair o contrato das rotas `/workstream/*` (corpo, resposta, erros, limites)
  - **Papel:** researcher
  - **agent:** researcher3
  - **Dependências:** Nenhuma
  - **depends_on:** []
  - **Paralelizável:** true
  - **Arquivos:** `specs/features/013-workstreams-gerenciados/notes/workstream-routes.md`
  - **Wiki-Keywords:** /workstream, limites, api-reference 6

- [ ] **T005:** Consolidar as notas da fase 0 e reescrever as tasks da fase 2 com os contratos reais
  - **Papel:** analyst
  - **agent:** analyst1
  - **Dependências:** T001, T002, T003, T004
  - **depends_on:** [T001, T002, T003, T004]
  - **Paralelizável:** false
  - **Arquivos:** `specs/features/013-workstreams-gerenciados/notes/phase0-consolidated.md`
  - **Wiki-Keywords:** consolidação, lacunas, decisão nossa


## Fase 1: Fundação

- [ ] **T006:** Criar o arquivo .feature da 013 com os cenários da spec
  - **Papel:** executor
  - **agent:** executor1
  - **Dependências:** T005
  - **depends_on:** [T005]
  - **Paralelizável:** true
  - **Arquivos:** `specs/features/013-workstreams-gerenciados/acceptance/workstreams.feature`
  - **Wiki-Keywords:** Gherkin, run, lease

- [ ] **T007:** Acrescentar à configuração `run.require_server`, `run_autowire` e as variáveis do processo filho, com teste
  - **Papel:** executor
  - **agent:** executor2
  - **Dependências:** T005
  - **depends_on:** [T005]
  - **Paralelizável:** true
  - **Arquivos:** `internal/config/config.go`, `internal/config/config_test.go`
  - **Wiki-Keywords:** run.require_server, run_autowire, AI_MEMORY_RUN_ID


## Fase 2: Implementação

### 2A Storage

- [ ] **T008:** Criar workstreams (criar, selecionar, listar, renomear, validação de nome), com teste
  - **Papel:** executor
  - **agent:** executor3
  - **Dependências:** T005
  - **depends_on:** [T005]
  - **Paralelizável:** true
  - **Arquivos:** `internal/storage/workstreams.go`, `internal/storage/workstreams_test.go`
  - **Wiki-Keywords:** workstream, seleção, nome, rename

- [ ] **T009:** Criar a preparação em uma transação, heartbeat, cancelamento, recuperação forçada e regras de lease, com teste multi-sessão
  - **Papel:** executor
  - **agent:** executor1
  - **Dependências:** T008
  - **depends_on:** [T008]
  - **Paralelizável:** false
  - **Arquivos:** `internal/storage/managed_runs.go`, `internal/storage/managed_runs_test.go`
  - **Wiki-Keywords:** preparação, lease, heartbeat, cancelar, force-unlock

- [ ] **T010:** Criar o ledger (inserção idempotente por id, cursores de origem e de entrega, busca), com teste
  - **Papel:** executor
  - **agent:** executor2
  - **Dependências:** T008
  - **depends_on:** [T008]
  - **Paralelizável:** true
  - **Arquivos:** `internal/storage/workstream_ledger.go`, `internal/storage/workstream_ledger_test.go`
  - **Wiki-Keywords:** ledger, ids determinísticos, cursores, FTS

- [ ] **T011:** Criar a montagem do pacote de contexto e a confirmação com avanço do cursor, com teste
  - **Papel:** executor
  - **agent:** executor3
  - **Dependências:** T009, T010
  - **depends_on:** [T009, T010]
  - **Paralelizável:** false
  - **Arquivos:** `internal/storage/managed_context.go`, `internal/storage/managed_context_test.go`
  - **Wiki-Keywords:** pacote de contexto, 256 eventos, 30 000 caracteres


### 2B Servidor

- [ ] **T012:** Criar as rotas `/workstream/*` com limites, regra de dono e acesso por projeto, com teste
  - **Papel:** executor
  - **agent:** executor1
  - **Dependências:** T009, T010, T011
  - **depends_on:** [T009, T010, T011]
  - **Paralelizável:** false
  - **Arquivos:** `internal/cli/workstream_routes.go`, `internal/cli/workstream_routes_test.go`
  - **Wiki-Keywords:** /workstream, limites, dono, acesso

- [ ] **T013:** Estender o `/handoff` com a execução gerenciada: reivindicação do handoff e confirmação do contexto na mesma transação, descartada se o contexto falhar, com teste multi-sessão
  - **Papel:** executor
  - **agent:** executor2
  - **Dependências:** T011
  - **depends_on:** [T011]
  - **Paralelizável:** false
  - **Arquivos:** `internal/ingest/handoff_route.go`, `internal/ingest/handoff_route_test.go`, `internal/storage/handoff_claim.go`
  - **Wiki-Keywords:** handoff e contexto na mesma transação

- [ ] **T014:** Montar as rotas da 013 no servidor HTTP
  - **Papel:** executor
  - **agent:** executor3
  - **Dependências:** T012
  - **depends_on:** [T012]
  - **Paralelizável:** false
  - **Arquivos:** `internal/cli/httpserver.go`
  - **Wiki-Keywords:** rotas, montagem

- [ ] **T015:** Criar a gravação do segmento sanitizado e imutável em `raw/`, com teste
  - **Papel:** executor
  - **agent:** executor1
  - **Dependências:** T010
  - **depends_on:** [T010]
  - **Paralelizável:** true
  - **Arquivos:** `internal/cli/workstream_segments.go`, `internal/cli/workstream_segments_test.go`
  - **Wiki-Keywords:** segmento sanitizado, raw, imutável


### 2C Lançador

- [ ] **T016:** Criar os descritores de lançamento por harness a partir da nota T002 e a resolução de executável (PATHEXT no Windows, nunca o arquivo sem extensão; `cmdc` no Windows para o Command Code — `docs/windows-native-extras.md` §9), com teste
  - **Papel:** executor
  - **agent:** executor2
  - **Dependências:** T007, T005
  - **depends_on:** [T007, T005]
  - **Paralelizável:** true
  - **Arquivos:** `internal/cli/launcher/harnesses.go`, `internal/cli/launcher/harnesses_test.go`
  - **Wiki-Keywords:** descritor de lançamento, seletores, yolo

- [ ] **T017:** Criar o lançador com a ordem das 17 etapas, o lançamento degradado e os códigos de saída, com teste com harness falso
  - **Papel:** executor
  - **agent:** executor3
  - **Dependências:** T016, T014
  - **depends_on:** [T016, T014]
  - **Paralelizável:** false
  - **Arquivos:** `internal/cli/launcher/run.go`, `internal/cli/launcher/run_test.go`
  - **Wiki-Keywords:** ordem do lançamento, 17 etapas, degradado

- [ ] **T018:** Criar a escolha da sessão nativa (estabelecido, adoção interativa, seletores, `--fresh`, sessão que sumiu), com teste
  - **Papel:** executor
  - **agent:** executor1
  - **Dependências:** T017
  - **depends_on:** [T017]
  - **Paralelizável:** false
  - **Arquivos:** `internal/cli/launcher/native_session.go`, `internal/cli/launcher/native_session_test.go`
  - **Wiki-Keywords:** adoção, seletores, fresh, sessão sumida

- [ ] **T019:** Criar o heartbeat com o comportamento de queda e retomada do servidor, com teste
  - **Papel:** executor
  - **agent:** executor2
  - **Dependências:** T017
  - **depends_on:** [T017]
  - **Paralelizável:** true
  - **Arquivos:** `internal/cli/launcher/heartbeat.go`, `internal/cli/launcher/heartbeat_test.go`
  - **Wiki-Keywords:** heartbeat, 30 s, 10 s, queda do servidor

- [ ] **T020:** Criar a importação ao sair (lotes, finalização, tentativas, servidor caído depois da saída), com teste
  - **Papel:** executor
  - **agent:** executor3
  - **Dependências:** T017, T015
  - **depends_on:** [T017, T015]
  - **Paralelizável:** false
  - **Arquivos:** `internal/cli/launcher/import.go`, `internal/cli/launcher/import_test.go`
  - **Wiki-Keywords:** importação, 400 eventos, 1 MiB, finish, 3 tentativas

- [ ] **T021:** Criar a entrega de contexto pelo lançador para Crush e Grok com confirmação depois do início do processo, com teste
  - **Papel:** executor
  - **agent:** executor1
  - **Dependências:** T017
  - **depends_on:** [T017]
  - **Paralelizável:** true
  - **Arquivos:** `internal/cli/launcher/context_delivery.go`, `internal/cli/launcher/context_delivery_test.go`
  - **Wiki-Keywords:** Crush, Grok, entrega pelo lançador

- [ ] **T022:** Criar o aviso de yolo, a oferta e a reexecução no ai-jail, com teste
  - **Papel:** executor
  - **agent:** executor2
  - **Dependências:** T017, T003
  - **depends_on:** [T017, T003]
  - **Paralelizável:** true
  - **Arquivos:** `internal/cli/launcher/jail.go`, `internal/cli/launcher/jail_test.go`
  - **Wiki-Keywords:** ai-jail, yolo, true-yolo, reexecução

- [ ] **T023:** Criar o auto-wire (hooks e MCP uma vez por harness, versão e local; sem MCP no degradado), com teste
  - **Papel:** executor
  - **agent:** executor3
  - **Dependências:** T017
  - **depends_on:** [T017]
  - **Paralelizável:** true
  - **Arquivos:** `internal/cli/launcher/autowire.go`, `internal/cli/launcher/autowire_test.go`
  - **Wiki-Keywords:** auto-wire, uma vez por harness

- [ ] **T024:** Criar `workstreams`, `rename-workstream`, `workstream-search`, `show`, `continue` e `resume`, com teste
  - **Papel:** executor
  - **agent:** executor1
  - **Dependências:** T017
  - **depends_on:** [T017]
  - **Paralelizável:** true
  - **Arquivos:** `internal/cli/workstream_cmds.go`, `internal/cli/workstream_cmds_test.go`
  - **Wiki-Keywords:** workstreams, rename, search, show, continue, resume

- [ ] **T025:** Registrar `run` e os comandos da 013 na raiz da CLI
  - **Papel:** executor
  - **agent:** executor2
  - **Dependências:** T018, T019, T020, T021, T022, T023, T024
  - **depends_on:** [T018, T019, T020, T021, T022, T023, T024]
  - **Paralelizável:** false
  - **Arquivos:** `internal/cli/root.go`
  - **Wiki-Keywords:** cobra, raiz da CLI, run


### 2D Cenários Gherkin

- [ ] **T026:** Implementar Scenario: Claude, depois Codex com contexto, depois Claude retomando a sessão vinculada
  - **Papel:** executor
  - **agent:** executor3
  - **Dependências:** T025, T013, T006
  - **depends_on:** [T025, T013, T006]
  - **Paralelizável:** true
  - **Arquivos:** `test/acceptance/run_flow_test.go`
  - **Wiki-Keywords:** run, troca de harness, contexto

- [ ] **T027:** Implementar edge case: lease ocupado, recuperação forçada e processo morto sem limpeza
  - **Papel:** executor
  - **agent:** executor1
  - **Dependências:** T025, T006
  - **depends_on:** [T025, T006]
  - **Paralelizável:** true
  - **Arquivos:** `test/acceptance/run_lease_test.go`
  - **Wiki-Keywords:** lease, ocupado, force-unlock, kill

- [ ] **T028:** Implementar tratamento de erro: servidor indisponível com e sem `--require-server`
  - **Papel:** executor
  - **agent:** executor2
  - **Dependências:** T025, T006
  - **depends_on:** [T025, T006]
  - **Paralelizável:** true
  - **Arquivos:** `test/acceptance/run_degraded_test.go`
  - **Wiki-Keywords:** degradado, require-server


## Fase 3: Validação

- [ ] **T029:** Rodar os portões e registrar o resultado com a saída
  - **Papel:** executor
  - **agent:** executor3
  - **Dependências:** T026, T027, T028
  - **depends_on:** [T026, T027, T028]
  - **Paralelizável:** false
  - **Arquivos:** `specs/features/013-workstreams-gerenciados/notes/gates-run.md`
  - **Wiki-Keywords:** check.sh, race, golangci-lint

- [ ] **T030:** Auditar a feature contra a constituição e contra `docs/managed-workstreams.md`
  - **Papel:** analyst
  - **agent:** analyst1
  - **Dependências:** T029
  - **depends_on:** [T029]
  - **Paralelizável:** false
  - **Arquivos:** `specs/features/013-workstreams-gerenciados/notes/constitution-audit.md`
  - **Wiki-Keywords:** constitution, auditoria, workstreams


## Fase 4: Documentação

- [ ] **T031:** Atualizar o README com `ai-memory run` e os workstreams
  - **Papel:** executor
  - **agent:** executor1
  - **Dependências:** T030
  - **depends_on:** [T030]
  - **Paralelizável:** true
  - **Arquivos:** `README.md`
  - **Wiki-Keywords:** README, run, workstreams

- [ ] **T032:** Atualizar o llms.txt com a 013
  - **Papel:** executor
  - **agent:** executor2
  - **Dependências:** T031
  - **depends_on:** [T031]
  - **Paralelizável:** true
  - **Arquivos:** `llms.txt`
  - **Wiki-Keywords:** llms.txt, onboarding

- [ ] **T033:** Atualizar o .env.example com as chaves da 013
  - **Papel:** executor
  - **agent:** executor3
  - **Dependências:** T007
  - **depends_on:** [T007]
  - **Paralelizável:** true
  - **Arquivos:** `.env.example`
  - **Wiki-Keywords:** .env.example, run



## Coordination Graph

G₀ (round 0):

- **nodes:** T001, T002, T003, T004, T005, T006, T007, T008, T009, T010, T011, T012, T013, T014, T015, T016, T017, T018, T019, T020, T021, T022, T023, T024, T025, T026, T027, T028, T029, T030, T031, T032, T033
- **edges:**
  - T001 → T005
  - T002 → T005
  - T003 → T005
  - T004 → T005
  - T005 → T006
  - T005 → T007
  - T005 → T008
  - T008 → T009
  - T008 → T010
  - T009 → T011
  - T010 → T011
  - T009 → T012
  - T010 → T012
  - T011 → T012
  - T011 → T013
  - T012 → T014
  - T010 → T015
  - T007 → T016
  - T005 → T016
  - T016 → T017
  - T014 → T017
  - T017 → T018
  - T017 → T019
  - T017 → T020
  - T015 → T020
  - T017 → T021
  - T017 → T022
  - T003 → T022
  - T017 → T023
  - T017 → T024
  - T018 → T025
  - T019 → T025
  - T020 → T025
  - T021 → T025
  - T022 → T025
  - T023 → T025
  - T024 → T025
  - T025 → T026
  - T013 → T026
  - T006 → T026
  - T025 → T027
  - T006 → T027
  - T025 → T028
  - T006 → T028
  - T026 → T029
  - T027 → T029
  - T028 → T029
  - T029 → T030
  - T030 → T031
  - T031 → T032
  - T007 → T033
- **assignments:**
  - T001: analyst1
  - T002: researcher1
  - T003: researcher2
  - T004: researcher3
  - T005: analyst1
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
  - T030: analyst1
  - T031: executor1
  - T032: executor2
  - T033: executor3
- **ready:** [T001, T002, T003, T004]

> Workers: Lead, researcher1..3, analyst1, executor1..3. Validado por script: 33 tasks, sem ciclo, sem dependência quebrada, sem arquivo compartilhado entre tasks sem ordem. Mínimo de 15 rounds. Caminho crítico (calculado): T001 → T005 → T008 → T009 → T011 → T012 → T014 → T017 → T018 → T025 → T026 → T029 → T030 → T031 → T032.
