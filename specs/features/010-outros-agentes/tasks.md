---
graph-operators: enabled
heartbeat-threshold: 4
max-rounds: 40
---

# tasks.md: Integração com outros agentes

> Feature `010-outros-agentes` · Spec: `spec.md` (draft) · Plano: `plan.md` (G-01 a G-05)
>
> **Provisório:** gerado em 2026-10-13, antes da execução da 002. A fase 0 extrai os contratos por agente; a T007 reescreve as tasks da fase 2 com os descritores reais. Regras: uma ação por task; implementação e `_test.go` contam como um grupo.


## Fase 0: Descoberta

- [ ] **T001:** Reconferir este DAG contra o código entregue pela 002 (instalador, normalização, spool) e consolidar as notas da fase 0
  - **Papel:** analyst
  - **agent:** analyst1
  - **Dependências:** Nenhuma
  - **depends_on:** []
  - **Paralelizável:** false
  - **Arquivos:** `specs/features/010-outros-agentes/notes/phase0-findings.md`
  - **Wiki-Keywords:** support-matrix, código entregue, 002

- [ ] **T002:** Extrair o descritor de instalação de cada agente (arquivos, formato, aliases, eventos, entrega de handoff, fim de sessão)
  - **Papel:** researcher
  - **agent:** researcher1
  - **Dependências:** Nenhuma
  - **depends_on:** []
  - **Paralelizável:** true
  - **Arquivos:** `specs/features/010-outros-agentes/notes/install-descriptors.md`
  - **Wiki-Keywords:** mcp-install, support-matrix, arquivos por agente

- [ ] **T003:** Extrair o contrato de payload de cada agente e a lista de fixtures a versionar
  - **Papel:** researcher
  - **agent:** researcher2
  - **Dependências:** Nenhuma
  - **depends_on:** []
  - **Paralelizável:** true
  - **Arquivos:** `specs/features/010-outros-agentes/notes/payload-contracts.md`
  - **Wiki-Keywords:** lifecycle-event-ingestion 3.5, esquema fechado, fixtures

- [ ] **T004:** Extrair o contrato dos scripts de hook (sequência, respostas, marcador, envio, spool, estado local, Windows)
  - **Papel:** researcher
  - **agent:** researcher3
  - **Dependências:** Nenhuma
  - **depends_on:** []
  - **Paralelizável:** true
  - **Arquivos:** `specs/features/010-outros-agentes/notes/hook-scripts-contract.md`
  - **Wiki-Keywords:** hook-scripts, paridade, PowerShell

- [ ] **T005:** Extrair o contrato da captura por orquestrador externo
  - **Papel:** researcher
  - **agent:** researcher4
  - **Dependências:** Nenhuma
  - **depends_on:** []
  - **Paralelizável:** true
  - **Arquivos:** `specs/features/010-outros-agentes/notes/external-lifecycle-contract.md`
  - **Wiki-Keywords:** external-lifecycle, dono de captura, confirmações

- [ ] **T006:** Extrair o contrato dos adaptadores de transcrição por harness
  - **Papel:** researcher
  - **agent:** researcher5
  - **Dependências:** Nenhuma
  - **depends_on:** []
  - **Paralelizável:** true
  - **Arquivos:** `specs/features/010-outros-agentes/notes/transcript-adapters.md`
  - **Wiki-Keywords:** harness-adapters 3, JSONL, SQLite, Antigravity

- [ ] **T007:** Consolidar as notas da fase 0, apontar lacunas e reescrever as tasks da fase 2 com os descritores reais
  - **Papel:** analyst
  - **agent:** analyst1
  - **Dependências:** T001, T002, T003, T004, T005, T006
  - **depends_on:** [T001, T002, T003, T004, T005, T006]
  - **Paralelizável:** false
  - **Arquivos:** `specs/features/010-outros-agentes/notes/phase0-consolidated.md`
  - **Wiki-Keywords:** consolidação, lacunas, decisão nossa


## Fase 1: Fundação

- [ ] **T008:** Criar o arquivo .feature da 010 com os cenários da spec
  - **Papel:** executor
  - **agent:** executor1
  - **Dependências:** T007
  - **depends_on:** [T007]
  - **Paralelizável:** true
  - **Arquivos:** `specs/features/010-outros-agentes/acceptance/outros-agentes.feature`
  - **Wiki-Keywords:** Gherkin, agentes, backfill

- [ ] **T009:** Acrescentar à configuração as variáveis de diretório dos agentes, `capture_assistant`, `backfill_on_start` e `AI_MEMORY_HOOK_PLATFORM`, com teste
  - **Papel:** executor
  - **agent:** executor2
  - **Dependências:** T007
  - **depends_on:** [T007]
  - **Paralelizável:** true
  - **Arquivos:** `internal/config/config.go`, `internal/config/config_test.go`
  - **Wiki-Keywords:** CODEX_HOME, KIMI_CODE_HOME, capture_assistant, backfill_on_start

- [ ] **T010:** Criar a tabela de descritores de agente a partir da nota T002, com teste
  - **Papel:** executor
  - **agent:** executor3
  - **Dependências:** T007
  - **depends_on:** [T007]
  - **Paralelizável:** true
  - **Arquivos:** `internal/cli/agents/descriptors.go`, `internal/cli/agents/descriptors_test.go`
  - **Wiki-Keywords:** descritor de agente, tabela, aliases


## Fase 2: Implementação

### 2A Instalação

- [ ] **T011:** Criar os escritores por formato (JSON, TOML, YAML impresso, plugin e extensão gerados) sobre a escrita idempotente da 002, com teste
  - **Papel:** executor
  - **agent:** executor1
  - **Dependências:** T010, T009
  - **depends_on:** [T010, T009]
  - **Paralelizável:** false
  - **Arquivos:** `internal/cli/agents/install_formats.go`, `internal/cli/agents/install_formats_test.go`
  - **Wiki-Keywords:** JSON, TOML, YAML impresso, plugin gerado

- [ ] **T012:** Estender `install-hooks` a todos os agentes da tabela, com teste por agente
  - **Papel:** executor
  - **agent:** executor2
  - **Dependências:** T011
  - **depends_on:** [T011]
  - **Paralelizável:** false
  - **Arquivos:** `internal/cli/install_hooks.go`, `internal/cli/install_hooks_test.go`
  - **Wiki-Keywords:** install-hooks, todos os agentes

- [ ] **T013:** Estender `install-mcp` a todos os clientes da tabela (regras de Kiro e Muse), com teste por cliente
  - **Papel:** executor
  - **agent:** executor3
  - **Dependências:** T011
  - **depends_on:** [T011]
  - **Paralelizável:** false
  - **Arquivos:** `internal/cli/install_mcp.go`, `internal/cli/install_mcp_test.go`
  - **Wiki-Keywords:** install-mcp, Kiro HTTPS, Muse schema_version

- [ ] **T014:** Criar `install-skills` e `install-instructions` idempotentes, com teste
  - **Papel:** executor
  - **agent:** executor1
  - **Dependências:** T011
  - **depends_on:** [T011]
  - **Paralelizável:** true
  - **Arquivos:** `internal/cli/agents/skills_instructions.go`, `internal/cli/agents/skills_instructions_test.go`
  - **Wiki-Keywords:** install-skills, install-instructions, marcadores HTML

- [ ] **T015:** Estender o `uninstall` simétrico a todos os agentes, com teste
  - **Papel:** executor
  - **agent:** executor2
  - **Dependências:** T012, T013, T014
  - **depends_on:** [T012, T013, T014]
  - **Paralelizável:** false
  - **Arquivos:** `internal/cli/uninstall_cmd.go`, `internal/cli/uninstall_cmd_test.go`
  - **Wiki-Keywords:** uninstall simétrico, todos os agentes


### 2B Captura

- [ ] **T016:** Criar a normalização por agente (esquema fechado, Cursor, caminho genérico, extensões), com fixtures por agente
  - **Papel:** executor
  - **agent:** executor3
  - **Dependências:** T007
  - **depends_on:** [T007]
  - **Paralelizável:** true
  - **Arquivos:** `internal/ingest/agents/normalize_agents.go`, `internal/ingest/agents/normalize_agents_test.go`, `internal/ingest/testdata/agents/`
  - **Wiki-Keywords:** esquema fechado, Cursor, genérico, extensões

- [ ] **T017:** Criar o conjunto de sessões de subagente para agentes que não marcam a sessão pai, com teste
  - **Papel:** executor
  - **agent:** executor1
  - **Dependências:** T016
  - **depends_on:** [T016]
  - **Paralelizável:** false
  - **Arquivos:** `internal/ingest/subagent_sessions.go`, `internal/ingest/subagent_sessions_test.go`
  - **Wiki-Keywords:** subagente, conjunto de 4096

- [ ] **T018:** Criar a captura da resposta do assistente com opt-in duplo no servidor, com teste
  - **Papel:** executor
  - **agent:** executor2
  - **Dependências:** T016, T009
  - **depends_on:** [T016, T009]
  - **Paralelizável:** true
  - **Arquivos:** `internal/ingest/assistant_capture.go`, `internal/ingest/assistant_capture_test.go`
  - **Wiki-Keywords:** opt-in duplo, 2000 bytes, sanitização

- [ ] **T019:** Criar o checkpoint de turno do OpenCode, com teste
  - **Papel:** executor
  - **agent:** executor3
  - **Dependências:** T016
  - **depends_on:** [T016]
  - **Paralelizável:** true
  - **Arquivos:** `internal/ingest/turn_checkpoint.go`, `internal/ingest/turn_checkpoint_test.go`
  - **Wiki-Keywords:** OpenCode, checkpoint de turno

- [ ] **T020:** Criar as variantes de entrega de handoff por agente (primeiro prompt, primeiro PostToolUse, invocação 0, additionalContext), com teste
  - **Papel:** executor
  - **agent:** executor1
  - **Dependências:** T010
  - **depends_on:** [T010]
  - **Paralelizável:** true
  - **Arquivos:** `internal/cli/handoff_delivery.go`, `internal/cli/handoff_delivery_test.go`
  - **Wiki-Keywords:** entrega de handoff, Kimi, Grok, Antigravity

- [ ] **T021:** Criar `finalize-session` para agentes sem evento de fim, com teste
  - **Papel:** executor
  - **agent:** executor2
  - **Dependências:** T010
  - **depends_on:** [T010]
  - **Paralelizável:** true
  - **Arquivos:** `internal/cli/finalize_session.go`, `internal/cli/finalize_session_test.go`
  - **Wiki-Keywords:** finalize-session, agentes sem fim

- [ ] **T022:** Criar os scripts de hook POSIX e PowerShell embutidos no binário
  - **Papel:** executor
  - **agent:** executor3
  - **Dependências:** T004
  - **depends_on:** [T004]
  - **Paralelizável:** true
  - **Arquivos:** `internal/cli/hookscripts/posix.sh`, `internal/cli/hookscripts/windows.ps1`, `internal/cli/hookscripts/embed.go`
  - **Wiki-Keywords:** scripts de hook, embed, PowerShell

- [ ] **T023:** Criar os testes de paridade entre hook nativo e scripts
  - **Papel:** executor
  - **agent:** executor1
  - **Dependências:** T022
  - **depends_on:** [T022]
  - **Paralelizável:** false
  - **Arquivos:** `internal/cli/hookscripts/parity_test.go`
  - **Wiki-Keywords:** paridade, hook nativo, scripts

- [ ] **T024:** Criar a ponte `mcp-bridge` com sessão e `install-mcp --session-aware`, com teste
  - **Papel:** executor
  - **agent:** executor2
  - **Dependências:** T001
  - **depends_on:** [T001]
  - **Paralelizável:** true
  - **Arquivos:** `internal/cli/mcp_bridge.go`, `internal/cli/mcp_bridge_test.go`
  - **Wiki-Keywords:** mcp-bridge, X-Memory-Actor-Session-Id

- [ ] **T025:** Criar o suporte à captura por orquestrador externo (dono de captura, proveniência, confirmações), com teste
  - **Papel:** executor
  - **agent:** executor3
  - **Dependências:** T005
  - **depends_on:** [T005]
  - **Paralelizável:** true
  - **Arquivos:** `internal/ingest/external_capture.go`, `internal/ingest/external_capture_test.go`
  - **Wiki-Keywords:** captura externa, dono de captura, confirmações


### 2C Transcrição, backfill e diagnóstico

- [ ] **T026:** Criar os adaptadores de transcrição somente leitura por harness, com fixtures
  - **Papel:** executor
  - **agent:** executor1
  - **Dependências:** T006
  - **depends_on:** [T006]
  - **Paralelizável:** true
  - **Arquivos:** `internal/cli/transcripts/adapters.go`, `internal/cli/transcripts/adapters_test.go`, `internal/cli/transcripts/testdata/`
  - **Wiki-Keywords:** adaptadores, JSONL, SQLite, somente leitura

- [ ] **T027:** Criar o comando `backfill` (seleção, mapeamento, horário, lotes, retentativas, relatório, sentinela, portão de vazio), com teste
  - **Papel:** executor
  - **agent:** executor2
  - **Dependências:** T026, T009
  - **depends_on:** [T026, T009]
  - **Paralelizável:** false
  - **Arquivos:** `internal/cli/backfill.go`, `internal/cli/backfill_test.go`
  - **Wiki-Keywords:** backfill, sentinela, portão de vazio, relatório

- [ ] **T028:** Disparar o backfill destacado no início de sessão pelo comando `hook`, conforme a sentinela
  - **Papel:** executor
  - **agent:** executor3
  - **Dependências:** T027
  - **depends_on:** [T027]
  - **Paralelizável:** false
  - **Arquivos:** `internal/cli/hook_cmd.go`
  - **Wiki-Keywords:** gatilho do backfill, início de sessão, destacado

- [ ] **T029:** Criar o `doctor` por agente, com teste
  - **Papel:** executor
  - **agent:** executor1
  - **Dependências:** T026
  - **depends_on:** [T026]
  - **Paralelizável:** true
  - **Arquivos:** `internal/cli/doctor.go`, `internal/cli/doctor_test.go`
  - **Wiki-Keywords:** doctor, sessões locais, capturadas

- [ ] **T030:** Criar `POST /admin/repair-session-times` e `repair-backfill-timestamps`, com teste
  - **Papel:** executor
  - **agent:** executor2
  - **Dependências:** T027
  - **depends_on:** [T027]
  - **Paralelizável:** true
  - **Arquivos:** `internal/cli/admin_repair_times.go`, `internal/cli/admin_repair_times_test.go`
  - **Wiki-Keywords:** repair-session-times, 2000 itens

- [ ] **T031:** Criar `backup-agents` e `restore-agents`, com teste
  - **Papel:** executor
  - **agent:** executor3
  - **Dependências:** T010
  - **depends_on:** [T010]
  - **Paralelizável:** true
  - **Arquivos:** `internal/cli/agent_assets.go`, `internal/cli/agent_assets_test.go`
  - **Wiki-Keywords:** backup-agents, restore-agents

- [ ] **T032:** Criar `setup-agent` (extrai scripts e imprime configuração com `--host-prefix`), com teste
  - **Papel:** executor
  - **agent:** executor1
  - **Dependências:** T010, T022
  - **depends_on:** [T010, T022]
  - **Paralelizável:** true
  - **Arquivos:** `internal/cli/setup_agent.go`, `internal/cli/setup_agent_test.go`
  - **Wiki-Keywords:** setup-agent, host-prefix

- [ ] **T033:** Montar a rota da 010 no servidor HTTP
  - **Papel:** executor
  - **agent:** executor2
  - **Dependências:** T030
  - **depends_on:** [T030]
  - **Paralelizável:** false
  - **Arquivos:** `internal/cli/httpserver.go`
  - **Wiki-Keywords:** rotas, montagem

- [ ] **T034:** Registrar os comandos da 010 na raiz da CLI
  - **Papel:** executor
  - **agent:** executor3
  - **Dependências:** T015, T021, T024, T027, T029, T031, T032
  - **depends_on:** [T015, T021, T024, T027, T029, T031, T032]
  - **Paralelizável:** false
  - **Arquivos:** `internal/cli/root.go`
  - **Wiki-Keywords:** cobra, raiz da CLI


### 2D Cenários Gherkin

- [ ] **T035:** Implementar Scenario: sessão no Codex entrega handoff a uma sessão no Claude Code no mesmo diretório
  - **Papel:** executor
  - **agent:** executor1
  - **Dependências:** T034, T012, T016, T008
  - **depends_on:** [T034, T012, T016, T008]
  - **Paralelizável:** true
  - **Arquivos:** `test/acceptance/cross_agent_handoff_test.go`
  - **Wiki-Keywords:** Codex, Claude Code, handoff entre agentes

- [ ] **T036:** Implementar edge case: entrega de handoff por agente e encerramento manual de sessão
  - **Papel:** executor
  - **agent:** executor2
  - **Dependências:** T020, T021, T008
  - **depends_on:** [T020, T021, T008]
  - **Paralelizável:** true
  - **Arquivos:** `test/acceptance/agent_delivery_test.go`
  - **Wiki-Keywords:** entrega por agente, finalize-session

- [ ] **T037:** Implementar Scenario: backfill único em loja vazia, recusa em loja não vazia e canário de segredo
  - **Papel:** executor
  - **agent:** executor3
  - **Dependências:** T028, T008
  - **depends_on:** [T028, T008]
  - **Paralelizável:** true
  - **Arquivos:** `test/acceptance/backfill_test.go`
  - **Wiki-Keywords:** backfill, loja vazia, canário de segredo

- [ ] **T038:** Implementar tratamento de erro: ponte com sessão recusa subir sem id
  - **Papel:** executor
  - **agent:** executor1
  - **Dependências:** T024, T008
  - **depends_on:** [T024, T008]
  - **Paralelizável:** true
  - **Arquivos:** `test/acceptance/mcp_bridge_test.go`
  - **Wiki-Keywords:** ponte, sem id, per_session


## Fase 3: Validação

- [ ] **T039:** Rodar os portões e registrar o resultado com a saída
  - **Papel:** executor
  - **agent:** executor2
  - **Dependências:** T023, T035, T036, T037, T038, T017, T018, T019, T025, T033
  - **depends_on:** [T023, T035, T036, T037, T038, T017, T018, T019, T025, T033]
  - **Paralelizável:** false
  - **Arquivos:** `specs/features/010-outros-agentes/notes/gates-run.md`
  - **Wiki-Keywords:** check.sh, race, golangci-lint

- [ ] **T040:** Auditar a feature contra a constituição e contra a matriz de suporte
  - **Papel:** analyst
  - **agent:** analyst1
  - **Dependências:** T039
  - **depends_on:** [T039]
  - **Paralelizável:** false
  - **Arquivos:** `specs/features/010-outros-agentes/notes/constitution-audit.md`
  - **Wiki-Keywords:** constitution, auditoria, support-matrix


## Fase 4: Documentação

- [ ] **T041:** Atualizar o README com os agentes, o backfill e o `doctor`
  - **Papel:** executor
  - **agent:** executor3
  - **Dependências:** T040
  - **depends_on:** [T040]
  - **Paralelizável:** true
  - **Arquivos:** `README.md`
  - **Wiki-Keywords:** README, agentes, backfill

- [ ] **T042:** Atualizar o llms.txt com a 010
  - **Papel:** executor
  - **agent:** executor1
  - **Dependências:** T041
  - **depends_on:** [T041]
  - **Paralelizável:** true
  - **Arquivos:** `llms.txt`
  - **Wiki-Keywords:** llms.txt, onboarding

- [ ] **T043:** Atualizar o .env.example com as chaves da 010
  - **Papel:** executor
  - **agent:** executor2
  - **Dependências:** T009
  - **depends_on:** [T009]
  - **Paralelizável:** true
  - **Arquivos:** `.env.example`
  - **Wiki-Keywords:** .env.example, agentes



## Coordination Graph

G₀ (round 0):

- **nodes:** T001, T002, T003, T004, T005, T006, T007, T008, T009, T010, T011, T012, T013, T014, T015, T016, T017, T018, T019, T020, T021, T022, T023, T024, T025, T026, T027, T028, T029, T030, T031, T032, T033, T034, T035, T036, T037, T038, T039, T040, T041, T042, T043
- **edges:**
  - T001 → T007
  - T002 → T007
  - T003 → T007
  - T004 → T007
  - T005 → T007
  - T006 → T007
  - T007 → T008
  - T007 → T009
  - T007 → T010
  - T010 → T011
  - T009 → T011
  - T011 → T012
  - T011 → T013
  - T011 → T014
  - T012 → T015
  - T013 → T015
  - T014 → T015
  - T007 → T016
  - T016 → T017
  - T016 → T018
  - T009 → T018
  - T016 → T019
  - T010 → T020
  - T010 → T021
  - T004 → T022
  - T022 → T023
  - T001 → T024
  - T005 → T025
  - T006 → T026
  - T026 → T027
  - T009 → T027
  - T027 → T028
  - T026 → T029
  - T027 → T030
  - T010 → T031
  - T010 → T032
  - T022 → T032
  - T030 → T033
  - T015 → T034
  - T021 → T034
  - T024 → T034
  - T027 → T034
  - T029 → T034
  - T031 → T034
  - T032 → T034
  - T034 → T035
  - T012 → T035
  - T016 → T035
  - T008 → T035
  - T020 → T036
  - T021 → T036
  - T008 → T036
  - T028 → T037
  - T008 → T037
  - T024 → T038
  - T008 → T038
  - T023 → T039
  - T035 → T039
  - T036 → T039
  - T037 → T039
  - T038 → T039
  - T017 → T039
  - T018 → T039
  - T019 → T039
  - T025 → T039
  - T033 → T039
  - T039 → T040
  - T040 → T041
  - T041 → T042
  - T009 → T043
- **assignments:**
  - T001: analyst1
  - T002: researcher1
  - T003: researcher2
  - T004: researcher3
  - T005: researcher4
  - T006: researcher5
  - T007: analyst1
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
  - T018: executor2
  - T019: executor3
  - T020: executor1
  - T021: executor2
  - T022: executor3
  - T023: executor1
  - T024: executor2
  - T025: executor3
  - T026: executor1
  - T027: executor2
  - T028: executor3
  - T029: executor1
  - T030: executor2
  - T031: executor3
  - T032: executor1
  - T033: executor2
  - T034: executor3
  - T035: executor1
  - T036: executor2
  - T037: executor3
  - T038: executor1
  - T039: executor2
  - T040: analyst1
  - T041: executor3
  - T042: executor1
  - T043: executor2
- **ready:** [T001, T002, T003, T004, T005, T006]

> Workers: Lead, researcher1..5, analyst1, executor1..3. Validado por script: 43 tasks, sem ciclo, sem dependência quebrada, sem arquivo compartilhado entre tasks sem ordem. Mínimo de 12 rounds. Caminho crítico (calculado): T001 → T007 → T010 → T011 → T012 → T015 → T034 → T035 → T039 → T040 → T041 → T042.
