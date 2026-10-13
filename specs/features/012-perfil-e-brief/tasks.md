---
graph-operators: enabled
heartbeat-threshold: 4
max-rounds: 40
---

# tasks.md: Perfil entre projetos e brief de início de sessão

> Feature `012-perfil-e-brief` · Spec: `spec.md` (draft) · Plano: `plan.md` (P2-01 a P2-06)
>
> **Provisório:** gerado em 2026-10-13, antes da execução das features anteriores. A T001 reconfere tudo contra o código entregue antes de qualquer execução.


## Fase 0: Descoberta

- [ ] **T001:** Reconferir este DAG contra o código entregue (/handoff, marcador, agendadores, acesso por projeto) e ajustar as tasks antes de executar
  - **Papel:** analyst
  - **agent:** analyst1
  - **Dependências:** Nenhuma
  - **depends_on:** []
  - **Paralelizável:** false
  - **Arquivos:** `specs/features/012-perfil-e-brief/notes/phase0-findings.md`
  - **Wiki-Keywords:** cross-project-profile, código entregue

- [ ] **T002:** Extrair dos docs as listas fixas (marcadores, stop words, tags de linguagem e de ferramenta, palavras de categoria) e registrar o que o doc não lista como decisão nossa
  - **Papel:** researcher
  - **agent:** researcher1
  - **Dependências:** Nenhuma
  - **depends_on:** []
  - **Paralelizável:** true
  - **Arquivos:** `specs/features/012-perfil-e-brief/notes/profile-vocab.md`
  - **Wiki-Keywords:** marcadores, stop words, 13 tags, categorias


## Fase 1: Fundação

- [ ] **T003:** Criar o arquivo .feature da 012 com os cenários da spec
  - **Papel:** executor
  - **agent:** executor1
  - **Dependências:** T001
  - **depends_on:** [T001]
  - **Paralelizável:** true
  - **Arquivos:** `specs/features/012-perfil-e-brief/acceptance/perfil.feature`
  - **Wiki-Keywords:** Gherkin, perfil, brief

- [ ] **T004:** Acrescentar `[profile]` à configuração com os cortes e a validação do doc, com teste
  - **Papel:** executor
  - **agent:** executor2
  - **Dependências:** T001
  - **depends_on:** [T001]
  - **Paralelizável:** true
  - **Arquivos:** `internal/config/config.go`, `internal/config/config_test.go`
  - **Wiki-Keywords:** [profile], AI_MEMORY_PROFILE__, validação


## Fase 2: Implementação

### 2A Núcleo puro

- [ ] **T005:** Criar a detecção de candidatos em prompts e páginas curadas (regras, geral, cortes), com teste em inglês e português
  - **Papel:** executor
  - **agent:** executor3
  - **Dependências:** T002
  - **depends_on:** [T002]
  - **Paralelizável:** true
  - **Arquivos:** `internal/domain/profile/detect.go`, `internal/domain/profile/detect_test.go`
  - **Wiki-Keywords:** detecção de frases, marcadores, geral

- [ ] **T006:** Criar a chave de tópico, com teste
  - **Papel:** executor
  - **agent:** executor1
  - **Dependências:** T002
  - **depends_on:** [T002]
  - **Paralelizável:** true
  - **Arquivos:** `internal/domain/profile/topic.go`, `internal/domain/profile/topic_test.go`
  - **Wiki-Keywords:** chave de tópico, stop words

- [ ] **T007:** Criar o agrupamento e a admissão (Jaccard, projetos independentes, barras, stack, decisão mais recente), com teste
  - **Papel:** executor
  - **agent:** executor2
  - **Dependências:** T005, T006
  - **depends_on:** [T005, T006]
  - **Paralelizável:** false
  - **Arquivos:** `internal/domain/profile/converge.go`, `internal/domain/profile/converge_test.go`
  - **Wiki-Keywords:** Jaccard, admissão, independentes, barra

- [ ] **T008:** Criar a renderização da página de entrada, a confiança, o caminho e o resumo (orçamento, ordem, neutralização, estabilidade), com teste
  - **Papel:** executor
  - **agent:** executor3
  - **Dependências:** T007
  - **depends_on:** [T007]
  - **Paralelizável:** false
  - **Arquivos:** `internal/domain/profile/render.go`, `internal/domain/profile/render_test.go`
  - **Wiki-Keywords:** página de entrada, confiança, caminho, digest


### 2B Storage e wiki

- [ ] **T009:** Criar o acesso a candidatos, marcas, ledger e flags `contribute`/`consume` do projeto, com teste
  - **Papel:** executor
  - **agent:** executor1
  - **Dependências:** T001
  - **depends_on:** [T001]
  - **Paralelizável:** true
  - **Arquivos:** `internal/storage/profile_store.go`, `internal/storage/profile_store_test.go`
  - **Wiki-Keywords:** profile_candidates, marcas, ledger, flags

- [ ] **T010:** Criar a passada do perfil (colheita, convergência por escopo, escrita só de páginas gerenciadas, relatório), com teste
  - **Papel:** executor
  - **agent:** executor2
  - **Dependências:** T008, T009
  - **depends_on:** [T008, T009]
  - **Paralelizável:** false
  - **Arquivos:** `internal/wiki/profile_pass.go`, `internal/wiki/profile_pass_test.go`
  - **Wiki-Keywords:** passada, colheita, convergência, escrita

- [ ] **T011:** Criar o trabalhador do perfil (agendamento e wake-up no fim de sessão), com teste
  - **Papel:** executor
  - **agent:** executor3
  - **Dependências:** T004, T010
  - **depends_on:** [T004, T010]
  - **Paralelizável:** false
  - **Arquivos:** `internal/cli/profile_worker.go`, `internal/cli/profile_worker_test.go`
  - **Wiki-Keywords:** trabalhador, 120 s, wake-up, uma hora

- [ ] **T012:** Iniciar o trabalhador do perfil no boot do `serve`
  - **Papel:** executor
  - **agent:** executor1
  - **Dependências:** T011
  - **depends_on:** [T011]
  - **Paralelizável:** false
  - **Arquivos:** `internal/cli/serve_cmd.go`
  - **Wiki-Keywords:** serve, trabalhador do perfil


### 2C Entrega

- [ ] **T013:** Criar o resumo do perfil no início do payload de `/handoff` (seleção, casos em que é pulado, variante de equipe), com teste
  - **Papel:** executor
  - **agent:** executor2
  - **Dependências:** T008, T009
  - **depends_on:** [T008, T009]
  - **Paralelizável:** true
  - **Arquivos:** `internal/ingest/profile_digest.go`, `internal/ingest/profile_digest_test.go`
  - **Wiki-Keywords:** resumo no /handoff, seleção, pulado quando

- [ ] **T014:** Criar o brief de início de sessão (páginas centrais, títulos recentes, orçamento, histórico não confiável), com teste
  - **Papel:** executor
  - **agent:** executor3
  - **Dependências:** T001
  - **depends_on:** [T001]
  - **Paralelizável:** true
  - **Arquivos:** `internal/ingest/session_brief.go`, `internal/ingest/session_brief_test.go`
  - **Wiki-Keywords:** brief, max_chars, páginas centrais

- [ ] **T015:** Ajustar a montagem do `/handoff` para a ordem do doc (perfil, handoff, oferta, gerenciado, brief, caixa de entrada)
  - **Papel:** executor
  - **agent:** executor1
  - **Dependências:** T013, T014
  - **depends_on:** [T013, T014]
  - **Paralelizável:** false
  - **Arquivos:** `internal/ingest/handoff_route.go`, `internal/ingest/handoff_route_test.go`
  - **Wiki-Keywords:** ordem do payload

- [ ] **T016:** Acrescentar a união com o perfil `workspace`/`user` e o respeito a `consume` no `memory_query`, com teste
  - **Papel:** executor
  - **agent:** executor2
  - **Dependências:** T009
  - **depends_on:** [T009]
  - **Paralelizável:** true
  - **Arquivos:** `internal/mcp/tools_read.go`, `internal/mcp/tools_read_test.go`
  - **Wiki-Keywords:** união na consulta, consume, global_scope_hits

- [ ] **T017:** Aceitar `scope: "profile"` em `memory_write_page` e `memory_delete_page`, com teste
  - **Papel:** executor
  - **agent:** executor3
  - **Dependências:** T001
  - **depends_on:** [T001]
  - **Paralelizável:** true
  - **Arquivos:** `internal/mcp/tools_write.go`, `internal/mcp/tools_write_test.go`
  - **Wiki-Keywords:** scope profile, write_page, delete_page


### 2D Comandos

- [ ] **T018:** Criar as rotas `/admin/profile/*`, com teste
  - **Papel:** executor
  - **agent:** executor1
  - **Dependências:** T010
  - **depends_on:** [T010]
  - **Paralelizável:** true
  - **Arquivos:** `internal/cli/admin_profile.go`, `internal/cli/admin_profile_test.go`
  - **Wiki-Keywords:** /admin/profile, status, review, apply, rebuild

- [ ] **T019:** Montar as rotas da 012 no servidor HTTP
  - **Papel:** executor
  - **agent:** executor2
  - **Dependências:** T018
  - **depends_on:** [T018]
  - **Paralelizável:** false
  - **Arquivos:** `internal/cli/httpserver.go`
  - **Wiki-Keywords:** rotas, montagem

- [ ] **T020:** Criar o comando `profile` e o `apply` com bloco gerenciado, com teste
  - **Papel:** executor
  - **agent:** executor3
  - **Dependências:** T019
  - **depends_on:** [T019]
  - **Paralelizável:** true
  - **Arquivos:** `internal/cli/profile_cmd.go`, `internal/cli/profile_cmd_test.go`
  - **Wiki-Keywords:** profile status, list, show, forget, review, rebuild, apply

- [ ] **T021:** Registrar o comando `profile` na raiz da CLI
  - **Papel:** executor
  - **agent:** executor1
  - **Dependências:** T020
  - **depends_on:** [T020]
  - **Paralelizável:** false
  - **Arquivos:** `internal/cli/root.go`
  - **Wiki-Keywords:** cobra, raiz da CLI


### 2E Cenários Gherkin

- [ ] **T022:** Implementar Scenario: preferência dita em dois projetos vira entrada e aparece no resumo do terceiro
  - **Papel:** executor
  - **agent:** executor2
  - **Dependências:** T012, T015, T003
  - **depends_on:** [T012, T015, T003]
  - **Paralelizável:** true
  - **Arquivos:** `test/acceptance/profile_flow_test.go`
  - **Wiki-Keywords:** colheita, admissão, resumo

- [ ] **T023:** Implementar edge case: opt-outs, página editada à mão e tópico esquecido
  - **Papel:** executor
  - **agent:** executor3
  - **Dependências:** T015, T003
  - **depends_on:** [T015, T003]
  - **Paralelizável:** true
  - **Arquivos:** `test/acceptance/profile_optouts_test.go`
  - **Wiki-Keywords:** consume, contribute, editada à mão, forget

- [ ] **T024:** Implementar Scenario: `profile apply` com cada alvo, `--dry-run` e `--remove`
  - **Papel:** executor
  - **agent:** executor1
  - **Dependências:** T020, T003
  - **depends_on:** [T020, T003]
  - **Paralelizável:** true
  - **Arquivos:** `test/acceptance/profile_apply_test.go`
  - **Wiki-Keywords:** apply, AGENTS.md, CLAUDE.md, remove

- [ ] **T025:** Implementar edge case: fronteiras de segurança do perfil com textos hostis
  - **Papel:** executor
  - **agent:** executor2
  - **Dependências:** T015, T003
  - **depends_on:** [T015, T003]
  - **Paralelizável:** true
  - **Arquivos:** `test/acceptance/profile_security_test.go`
  - **Wiki-Keywords:** perfil user privado, neutralização


## Fase 3: Validação

- [ ] **T026:** Rodar os portões e registrar o resultado com a saída
  - **Papel:** executor
  - **agent:** executor3
  - **Dependências:** T016, T017, T021, T022, T023, T024, T025
  - **depends_on:** [T016, T017, T021, T022, T023, T024, T025]
  - **Paralelizável:** false
  - **Arquivos:** `specs/features/012-perfil-e-brief/notes/gates-run.md`
  - **Wiki-Keywords:** check.sh, race, golangci-lint

- [ ] **T027:** Auditar a feature contra a constituição e contra `docs/design-cross-project-profile.md`
  - **Papel:** analyst
  - **agent:** analyst1
  - **Dependências:** T026
  - **depends_on:** [T026]
  - **Paralelizável:** false
  - **Arquivos:** `specs/features/012-perfil-e-brief/notes/constitution-audit.md`
  - **Wiki-Keywords:** constitution, auditoria, perfil


## Fase 4: Documentação

- [ ] **T028:** Atualizar o README com o perfil e o brief
  - **Papel:** executor
  - **agent:** executor1
  - **Dependências:** T027
  - **depends_on:** [T027]
  - **Paralelizável:** true
  - **Arquivos:** `README.md`
  - **Wiki-Keywords:** README, perfil, brief

- [ ] **T029:** Atualizar o llms.txt com a 012
  - **Papel:** executor
  - **agent:** executor2
  - **Dependências:** T028
  - **depends_on:** [T028]
  - **Paralelizável:** true
  - **Arquivos:** `llms.txt`
  - **Wiki-Keywords:** llms.txt, onboarding

- [ ] **T030:** Atualizar o .env.example com as chaves de `[profile]`
  - **Papel:** executor
  - **agent:** executor3
  - **Dependências:** T004
  - **depends_on:** [T004]
  - **Paralelizável:** true
  - **Arquivos:** `.env.example`
  - **Wiki-Keywords:** .env.example, profile



## Coordination Graph

G₀ (round 0):

- **nodes:** T001, T002, T003, T004, T005, T006, T007, T008, T009, T010, T011, T012, T013, T014, T015, T016, T017, T018, T019, T020, T021, T022, T023, T024, T025, T026, T027, T028, T029, T030
- **edges:**
  - T001 → T003
  - T001 → T004
  - T002 → T005
  - T002 → T006
  - T005 → T007
  - T006 → T007
  - T007 → T008
  - T001 → T009
  - T008 → T010
  - T009 → T010
  - T004 → T011
  - T010 → T011
  - T011 → T012
  - T008 → T013
  - T009 → T013
  - T001 → T014
  - T013 → T015
  - T014 → T015
  - T009 → T016
  - T001 → T017
  - T010 → T018
  - T018 → T019
  - T019 → T020
  - T020 → T021
  - T012 → T022
  - T015 → T022
  - T003 → T022
  - T015 → T023
  - T003 → T023
  - T020 → T024
  - T003 → T024
  - T015 → T025
  - T003 → T025
  - T016 → T026
  - T017 → T026
  - T021 → T026
  - T022 → T026
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

> Workers: Lead, researcher1, analyst1, executor1..3. Validado por script: 30 tasks, sem ciclo, sem dependência quebrada, sem arquivo compartilhado entre tasks sem ordem. Mínimo de 13 rounds. Caminho crítico (calculado): T002 → T005 → T007 → T008 → T010 → T018 → T019 → T020 → T021 → T026 → T027 → T028 → T029.
