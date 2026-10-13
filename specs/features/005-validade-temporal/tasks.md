---
graph-operators: enabled
heartbeat-threshold: 4
max-rounds: 40
---

# tasks.md: Validade temporal e arestas tipadas

> Feature `005-validade-temporal` · Spec: `spec.md` (draft) · Plano: `plan.md` (V-01 a V-08)
>
> **Provisório:** gerado em 2026-10-13, antes da execução das features anteriores. A T001 reconfere tudo contra o código entregue antes de qualquer execução. Regras: uma ação por task; implementação e `_test.go` contam como um grupo. `internal/mcp/tools_read.go` é editado em sequência por T010 e T012; `internal/storage/links.go` por T009 e T022.


## Fase 0: Descoberta

- [ ] **T001:** Reconferir este DAG contra o código entregue (busca, fluxo de entidades, derivação de links, tools) e ajustar as tasks antes de executar
  - **Papel:** analyst
  - **agent:** analyst1
  - **Dependências:** Nenhuma
  - **depends_on:** []
  - **Paralelizável:** false
  - **Arquivos:** `specs/features/005-validade-temporal/notes/phase0-findings.md`
  - **Wiki-Keywords:** temporal, typed-edges, código entregue


## Fase 1: Fundação

- [ ] **T002:** Criar o arquivo .feature da 005 com os cenários da spec
  - **Papel:** executor
  - **agent:** executor1
  - **Dependências:** T001
  - **depends_on:** [T001]
  - **Paralelizável:** true
  - **Arquivos:** `specs/features/005-validade-temporal/acceptance/validade-temporal.feature`
  - **Wiki-Keywords:** Gherkin, as_of, arestas tipadas

- [ ] **T003:** Criar o parse de `as_of` (data, hora e fuso obrigatórios; vazio equivale a ausência; inválido é erro), com teste
  - **Papel:** executor
  - **agent:** executor2
  - **Dependências:** T001
  - **depends_on:** [T001]
  - **Paralelizável:** true
  - **Arquivos:** `internal/domain/asof.go`, `internal/domain/asof_test.go`
  - **Wiki-Keywords:** as_of, ISO-8601, vazio, inválido

- [ ] **T004:** Criar a validação pública de `relations` (vocabulário fechado, lista, 32 alvos, 1024 caracteres, controle, qualificador, portabilidade), com teste
  - **Papel:** executor
  - **agent:** executor3
  - **Dependências:** T001
  - **depends_on:** [T001]
  - **Paralelizável:** true
  - **Arquivos:** `internal/domain/relations.go`, `internal/domain/relations_test.go`
  - **Wiki-Keywords:** relations, limites, 32 alvos, 1024 caracteres


## Fase 2: Implementação

### 2A Storage

- [ ] **T005:** Conferir e alinhar o fluxo de entidades do caminho padrão às regras de termo e de casamento do `temporal.md`, com teste
  - **Papel:** executor
  - **agent:** executor1
  - **Dependências:** T001
  - **depends_on:** [T001]
  - **Paralelizável:** true
  - **Arquivos:** `internal/storage/search_streams.go`, `internal/storage/search_streams_test.go`
  - **Wiki-Keywords:** fluxo de entidades, prefixo de palavra, compostos

- [ ] **T006:** Criar `SearchAsOf` (fluxo de entidades por janela de vínculo, fluxo de texto por janela de página e expiração em T, candidatos, RRF, autoridade da própria versão, `explain`), com teste
  - **Papel:** executor
  - **agent:** executor2
  - **Dependências:** T003, T005
  - **depends_on:** [T003, T005]
  - **Paralelizável:** false
  - **Arquivos:** `internal/storage/search_asof.go`, `internal/storage/search_asof_test.go`
  - **Wiki-Keywords:** SearchAsOf, janelas, RRF, autoridade da versão

- [ ] **T007:** Fazer o grafo da busca carregar o rótulo do link e o `explain` mostrar `edge` quando não for `references`, com teste
  - **Papel:** executor
  - **agent:** executor3
  - **Dependências:** T001
  - **depends_on:** [T001]
  - **Paralelizável:** true
  - **Arquivos:** `internal/storage/search.go`, `internal/storage/search_test.go`
  - **Wiki-Keywords:** edge, rótulo do salto, explain

- [ ] **T008:** Criar a contagem por relação dos links tipados de versões atuais e a contagem `evidence_rows` do projeto, com teste
  - **Papel:** executor
  - **agent:** executor1
  - **Dependências:** T001
  - **depends_on:** [T001]
  - **Paralelizável:** true
  - **Arquivos:** `internal/storage/status_relations.go`, `internal/storage/status_relations_test.go`
  - **Wiki-Keywords:** contagem por relação, evidence_rows, status


- [ ] **T023:** Criar a confiança derivada como função pura (`docs/storage-and-retrieval.md` §13: amplitude por sessões distintas, suporte, recência com piso 0,5, penalidade por contradição viva, teto 0,95), com testes das propriedades do doc
  - **Papel:** executor
  - **agent:** executor2
  - **Dependências:** T001
  - **depends_on:** [T001]
  - **Paralelizável:** true
  - **Arquivos:** `internal/domain/belief.go`, `internal/domain/belief_test.go`
  - **Wiki-Keywords:** confiança derivada, sessões distintas, 0.95

- [ ] **T024:** Acrescentar à configuração a seção `[retrieval]` (`query_intent`, `session_recall_bonus` 0,25, `belief_authority_weight` 0,0), com validação e teste
  - **Papel:** executor
  - **agent:** executor3
  - **Dependências:** T001
  - **depends_on:** [T001]
  - **Paralelizável:** true
  - **Arquivos:** `internal/config/config.go`, `internal/config/config_test.go`
  - **Wiki-Keywords:** retrieval, query_intent, session_recall_bonus, belief_authority_weight

- [ ] **T025:** Ligar à autoridade o peso de crença (sem consulta de evidência com peso 0 e sem `explain`; versões superseded sem bônus) e o roteamento de recall de sessão (marcadores léxicos em chinês, inglês e português, intenção `session_recall`), e expor no `explain` `evidence_count`, `confidence`, `belief_factor` e a intenção, com teste
  - **Papel:** executor
  - **agent:** executor1
  - **Dependências:** T007, T023, T024
  - **depends_on:** [T007, T023, T024]
  - **Paralelizável:** false
  - **Arquivos:** `internal/storage/rank.go`, `internal/storage/rank_test.go`, `internal/storage/search.go`, `internal/storage/search_test.go`
  - **Wiki-Keywords:** peso de crença, recall de sessão, explain, intent

- [ ] **T026:** Criar `settled_first` no `memory_briefing` (até 8 páginas atuais de regra ou decisão não expiradas, por contagem de evidência e depois recência), com teste
  - **Papel:** executor
  - **agent:** executor2
  - **Dependências:** T023
  - **depends_on:** [T023]
  - **Paralelizável:** true
  - **Arquivos:** `internal/storage/settled.go`, `internal/storage/settled_test.go`, `internal/mcp/tools_session.go`, `internal/mcp/tools_session_test.go`
  - **Wiki-Keywords:** settled_first, briefing, 8 páginas


### 2B Wiki

- [ ] **T009:** Fazer a chave de relação desconhecida e o alvo inválido no frontmatter serem ignorados com aviso, em vez de virar `references`, com teste
  - **Papel:** executor
  - **agent:** executor2
  - **Dependências:** T001
  - **depends_on:** [T001]
  - **Paralelizável:** true
  - **Arquivos:** `internal/wiki/derive.go`, `internal/wiki/derive_test.go`, `internal/storage/links.go`, `internal/storage/links_test.go`
  - **Wiki-Keywords:** relação desconhecida, ignorada com aviso, typed-edges

- [ ] **T022:** Reapontar, em toda nova versão (supersessão, ressurreição ou cadeia nova), todas as linhas de link cujo alvo casa com o caminho, inclusive as já resolvidas, como `docs/typed-edges.md` § Storage define, com teste
  - **Papel:** executor
  - **agent:** executor3
  - **Dependências:** T009
  - **depends_on:** [T009]
  - **Paralelizável:** false
  - **Arquivos:** `internal/storage/upsert.go`, `internal/storage/upsert_test.go`, `internal/storage/links.go`
  - **Wiki-Keywords:** reapontar links, alvo casado, ressurreição


### 2C MCP

- [ ] **T010:** Ligar `as_of` ao `memory_query` (erros com `global` e `scopes`, `answer_unavailable`, `streams_active` só com `explain`), com teste
  - **Papel:** executor
  - **agent:** executor3
  - **Dependências:** T006
  - **depends_on:** [T006]
  - **Paralelizável:** true
  - **Arquivos:** `internal/mcp/tools_read.go`, `internal/mcp/tools_read_test.go`
  - **Wiki-Keywords:** memory_query, as_of, global, scopes

- [ ] **T011:** Aceitar `relations` nos metadados de `memory_write_page`, rejeitando a escrita inteira em violação, com teste
  - **Papel:** executor
  - **agent:** executor1
  - **Dependências:** T004, T009
  - **depends_on:** [T004, T009]
  - **Paralelizável:** true
  - **Arquivos:** `internal/mcp/tools_write.go`, `internal/mcp/tools_write_test.go`
  - **Wiki-Keywords:** memory_write_page, relations, rejeição

- [ ] **T012:** Incluir a contagem por relação e `evidence_rows` no `memory_status`
  - **Papel:** executor
  - **agent:** executor2
  - **Dependências:** T008, T010
  - **depends_on:** [T008, T010]
  - **Paralelizável:** false
  - **Arquivos:** `internal/mcp/tools_read.go`
  - **Wiki-Keywords:** memory_status, contagem por relação, evidence_rows


### 2D Cenários Gherkin

- [ ] **T013:** Implementar Scenario: exemplo postgres e sqlite pelos fluxos de entidade e de texto
  - **Papel:** executor
  - **agent:** executor3
  - **Dependências:** T010, T002
  - **depends_on:** [T010, T002]
  - **Paralelizável:** true
  - **Arquivos:** `test/acceptance/as_of_test.go`
  - **Wiki-Keywords:** as_of, postgres, sqlite, dois fluxos

- [ ] **T014:** Implementar tratamento de erro: `as_of` inválido e combinado com `global` ou `scopes`
  - **Papel:** executor
  - **agent:** executor1
  - **Dependências:** T010, T002
  - **depends_on:** [T010, T002]
  - **Paralelizável:** true
  - **Arquivos:** `test/acceptance/as_of_errors_test.go`
  - **Wiki-Keywords:** as_of inválido, global, scopes

- [ ] **T015:** Implementar edge case: intervalo meio-aberto, ressurreição, decaimento e exclusão sob `as_of` (com supersessão concorrente)
  - **Papel:** executor
  - **agent:** executor2
  - **Dependências:** T010, T002
  - **depends_on:** [T010, T002]
  - **Paralelizável:** true
  - **Arquivos:** `test/acceptance/as_of_windows_test.go`
  - **Wiki-Keywords:** meio-aberto, ressurreição, decaimento, exclusão

- [ ] **T016:** Implementar tratamento de erro: `relations` com cada limite violado rejeita a escrita inteira
  - **Papel:** executor
  - **agent:** executor3
  - **Dependências:** T011, T002
  - **depends_on:** [T011, T002]
  - **Paralelizável:** true
  - **Arquivos:** `test/acceptance/relations_write_test.go`
  - **Wiki-Keywords:** relations, limites, rejeição

- [ ] **T017:** Implementar edge case: relação desconhecida no arquivo ignorada com aviso e `edge` no `explain`
  - **Papel:** executor
  - **agent:** executor1
  - **Dependências:** T009, T007, T002
  - **depends_on:** [T009, T007, T002]
  - **Paralelizável:** true
  - **Arquivos:** `test/acceptance/typed_edges_test.go`
  - **Wiki-Keywords:** edge, explain, relação desconhecida

- [ ] **T027:** Implementar Scenario: peso de crença e recall de sessão mudando a ordem só quando ligados, e `settled_first` no briefing
  - **Papel:** executor
  - **agent:** executor3
  - **Dependências:** T025, T026, T002
  - **depends_on:** [T025, T026, T002]
  - **Paralelizável:** true
  - **Arquivos:** `test/acceptance/belief_ranking_test.go`
  - **Wiki-Keywords:** crença, recall de sessão, settled


## Fase 3: Validação

- [ ] **T018:** Rodar os portões e registrar o resultado com a saída
  - **Papel:** executor
  - **agent:** executor2
  - **Dependências:** T012, T013, T014, T015, T016, T017, T022, T027
  - **depends_on:** [T012, T013, T014, T015, T016, T017, T022, T027]
  - **Paralelizável:** false
  - **Arquivos:** `specs/features/005-validade-temporal/notes/gates-run.md`
  - **Wiki-Keywords:** check.sh, race, golangci-lint

- [ ] **T019:** Auditar a feature contra a constituição e contra `docs/temporal.md` e `docs/typed-edges.md`
  - **Papel:** analyst
  - **agent:** analyst1
  - **Dependências:** T018
  - **depends_on:** [T018]
  - **Paralelizável:** false
  - **Arquivos:** `specs/features/005-validade-temporal/notes/constitution-audit.md`
  - **Wiki-Keywords:** constitution, auditoria, docs


## Fase 4: Documentação

- [ ] **T020:** Atualizar o README com `as_of` e `relations`
  - **Papel:** executor
  - **agent:** executor3
  - **Dependências:** T019
  - **depends_on:** [T019]
  - **Paralelizável:** true
  - **Arquivos:** `README.md`
  - **Wiki-Keywords:** README, as_of, relations

- [ ] **T021:** Atualizar o llms.txt com a 005
  - **Papel:** executor
  - **agent:** executor1
  - **Dependências:** T020
  - **depends_on:** [T020]
  - **Paralelizável:** true
  - **Arquivos:** `llms.txt`
  - **Wiki-Keywords:** llms.txt, onboarding



## Coordination Graph

G₀ (round 0):

- **nodes:** T001, T002, T003, T004, T005, T006, T007, T008, T023, T024, T025, T026, T009, T022, T010, T011, T012, T013, T014, T015, T016, T017, T027, T018, T019, T020, T021
- **edges:**
  - T001 → T002
  - T001 → T003
  - T001 → T004
  - T001 → T005
  - T003 → T006
  - T005 → T006
  - T001 → T007
  - T001 → T008
  - T001 → T023
  - T001 → T024
  - T007 → T025
  - T023 → T025
  - T024 → T025
  - T023 → T026
  - T001 → T009
  - T009 → T022
  - T006 → T010
  - T004 → T011
  - T009 → T011
  - T008 → T012
  - T010 → T012
  - T010 → T013
  - T002 → T013
  - T010 → T014
  - T002 → T014
  - T010 → T015
  - T002 → T015
  - T011 → T016
  - T002 → T016
  - T009 → T017
  - T007 → T017
  - T002 → T017
  - T025 → T027
  - T026 → T027
  - T002 → T027
  - T012 → T018
  - T013 → T018
  - T014 → T018
  - T015 → T018
  - T016 → T018
  - T017 → T018
  - T022 → T018
  - T027 → T018
  - T018 → T019
  - T019 → T020
  - T020 → T021
- **assignments:**
  - T001: analyst1
  - T002: executor1
  - T003: executor2
  - T004: executor3
  - T005: executor1
  - T006: executor2
  - T007: executor3
  - T008: executor1
  - T023: executor2
  - T024: executor3
  - T025: executor1
  - T026: executor2
  - T009: executor2
  - T022: executor3
  - T010: executor3
  - T011: executor1
  - T012: executor2
  - T013: executor3
  - T014: executor1
  - T015: executor2
  - T016: executor3
  - T017: executor1
  - T027: executor3
  - T018: executor2
  - T019: analyst1
  - T020: executor3
  - T021: executor1
- **ready:** [T001]

> Workers: Lead, analyst1, executor1..3. Validado por script: 27 tasks, sem ciclo, sem dependência quebrada, sem arquivo compartilhado entre tasks sem ordem. Mínimo de 9 rounds. Caminho crítico (calculado): T001 → T003 → T006 → T010 → T012 → T018 → T019 → T020 → T021.
