---
graph-operators: enabled
heartbeat-threshold: 4
max-rounds: 40
---

# tasks.md: Navegador web da wiki e API JSON

> Feature `014-navegador-web-e-api` · Spec: `spec.md` (draft) · Plano: `plan.md` (B-01 a B-05)
>
> **Provisório:** gerado em 2026-10-13, antes da execução das features anteriores. A T001 reconfere tudo contra o código entregue antes de qualquer execução. `go.mod` e `go.sum` só na T004.


## Fase 0: Descoberta

- [ ] **T001:** Reconferir este DAG contra o código entregue (autenticação da 007, Reader, busca) e consolidar as notas da fase 0
  - **Papel:** analyst
  - **agent:** analyst1
  - **Dependências:** Nenhuma
  - **depends_on:** []
  - **Paralelizável:** false
  - **Arquivos:** `specs/features/014-navegador-web-e-api/notes/phase0-findings.md`
  - **Wiki-Keywords:** wiki-browser, frontend-api, 007 entregue

- [ ] **T002:** Extrair o contrato campo a campo de cada endpoint de `/api/v1` e da hospedagem de interface própria
  - **Papel:** researcher
  - **agent:** researcher1
  - **Dependências:** Nenhuma
  - **depends_on:** []
  - **Paralelizável:** true
  - **Arquivos:** `specs/features/014-navegador-web-e-api/notes/api-contract.md`
  - **Wiki-Keywords:** frontend-api 4, campos, erros


## Fase 1: Fundação

- [ ] **T003:** Criar o arquivo .feature da 014 com os cenários da spec
  - **Papel:** executor
  - **agent:** executor1
  - **Dependências:** T001
  - **depends_on:** [T001]
  - **Paralelizável:** true
  - **Arquivos:** `specs/features/014-navegador-web-e-api/acceptance/web.feature`
  - **Wiki-Keywords:** Gherkin, navegador, API

- [ ] **T004:** Adicionar o renderizador de markdown ao `go.mod` e registrar em `tech.md`
  - **Papel:** executor
  - **agent:** executor2
  - **Dependências:** T001
  - **depends_on:** [T001]
  - **Paralelizável:** true
  - **Arquivos:** `go.mod`, `go.sum`, `.github/memory/tech.md`
  - **Wiki-Keywords:** goldmark, markdown, tech.md

- [ ] **T005:** Acrescentar à configuração a interface web, o slug, as origens CORS (validadas) e a interface customizada, com teste
  - **Papel:** executor
  - **agent:** executor3
  - **Dependências:** T001
  - **depends_on:** [T001]
  - **Paralelizável:** true
  - **Arquivos:** `internal/config/config.go`, `internal/config/config_test.go`
  - **Wiki-Keywords:** web, slug, CORS, interface customizada


## Fase 2: Implementação

- [ ] **T006:** Criar a renderização de markdown seguro (HTML bruto desligado, esquemas neutralizados, imagens relativas, wikilinks, ids de cabeçalho, trechos), com suíte de XSS
  - **Papel:** executor
  - **agent:** executor1
  - **Dependências:** T004
  - **depends_on:** [T004]
  - **Paralelizável:** true
  - **Arquivos:** `internal/cli/web/markdown.go`, `internal/cli/web/markdown_test.go`
  - **Wiki-Keywords:** markdown seguro, XSS, wikilinks

- [ ] **T007:** Criar as rotas de `/api/v1` com parâmetros, limites e erros do doc, com teste
  - **Papel:** executor
  - **agent:** executor2
  - **Dependências:** T002
  - **depends_on:** [T002]
  - **Paralelizável:** true
  - **Arquivos:** `internal/cli/web/api.go`, `internal/cli/web/api_test.go`
  - **Wiki-Keywords:** /api/v1, parâmetros, limites, erros

- [ ] **T008:** Criar o cursor opaco do `recent` incremental, com teste
  - **Papel:** executor
  - **agent:** executor3
  - **Dependências:** T007
  - **depends_on:** [T007]
  - **Paralelizável:** false
  - **Arquivos:** `internal/cli/web/cursor.go`, `internal/cli/web/cursor_test.go`
  - **Wiki-Keywords:** cursor opaco, versão 1, limites

- [ ] **T009:** Criar o middleware de cache e o ETag da página completa, com teste
  - **Papel:** executor
  - **agent:** executor1
  - **Dependências:** T007
  - **depends_on:** [T007]
  - **Paralelizável:** true
  - **Arquivos:** `internal/cli/web/cache.go`, `internal/cli/web/cache_test.go`
  - **Wiki-Keywords:** cache, ETag, 304

- [ ] **T010:** Criar o CORS restrito a `/api/v1`, com teste
  - **Papel:** executor
  - **agent:** executor2
  - **Dependências:** T007, T005
  - **depends_on:** [T007, T005]
  - **Paralelizável:** true
  - **Arquivos:** `internal/cli/web/cors.go`, `internal/cli/web/cors_test.go`
  - **Wiki-Keywords:** CORS, /api/v1, pré-voo

- [ ] **T011:** Criar as rotas HTML e os templates embutidos (início, projeto, página, busca, login, troca de senha, estáticos, favicon), com teste
  - **Papel:** executor
  - **agent:** executor3
  - **Dependências:** T006
  - **depends_on:** [T006]
  - **Paralelizável:** true
  - **Arquivos:** `internal/cli/web/html.go`, `internal/cli/web/html_test.go`, `internal/cli/web/templates/`
  - **Wiki-Keywords:** navegador HTML, templates, redirecionamento

- [ ] **T012:** Criar a montagem (prefixo, base injetada, rotas públicas e protegidas, interface customizada), com teste
  - **Papel:** executor
  - **agent:** executor1
  - **Dependências:** T011, T005
  - **depends_on:** [T011, T005]
  - **Paralelizável:** false
  - **Arquivos:** `internal/cli/web/mount.go`, `internal/cli/web/mount_test.go`
  - **Wiki-Keywords:** prefixo, base de URLs, interface customizada

- [ ] **T013:** Montar o navegador e a API no servidor HTTP
  - **Papel:** executor
  - **agent:** executor2
  - **Dependências:** T008, T009, T010, T012
  - **depends_on:** [T008, T009, T010, T012]
  - **Paralelizável:** false
  - **Arquivos:** `internal/cli/httpserver.go`
  - **Wiki-Keywords:** rotas, montagem


### Cenários Gherkin

- [ ] **T014:** Implementar Scenario: leitura pela API com ETag e 304, e escopos 403 e 404
  - **Papel:** executor
  - **agent:** executor3
  - **Dependências:** T013, T003
  - **depends_on:** [T013, T003]
  - **Paralelizável:** true
  - **Arquivos:** `test/acceptance/web_api_test.go`
  - **Wiki-Keywords:** API, ETag, 304, escopo

- [ ] **T015:** Implementar Scenario: navegação com login, cards filtrados por acesso e redirecionamento sem sessão
  - **Papel:** executor
  - **agent:** executor1
  - **Dependências:** T013, T003
  - **depends_on:** [T013, T003]
  - **Paralelizável:** true
  - **Arquivos:** `test/acceptance/web_browser_test.go`
  - **Wiki-Keywords:** navegador, login, redirecionamento

- [ ] **T016:** Implementar edge case: página com HTML e script hostis renderizada com segurança
  - **Papel:** executor
  - **agent:** executor2
  - **Dependências:** T006, T003
  - **depends_on:** [T006, T003]
  - **Paralelizável:** true
  - **Arquivos:** `test/acceptance/web_xss_test.go`
  - **Wiki-Keywords:** XSS, página hostil


## Fase 3: Validação

- [ ] **T017:** Rodar os portões e registrar o resultado com a saída
  - **Papel:** executor
  - **agent:** executor3
  - **Dependências:** T014, T015, T016
  - **depends_on:** [T014, T015, T016]
  - **Paralelizável:** false
  - **Arquivos:** `specs/features/014-navegador-web-e-api/notes/gates-run.md`
  - **Wiki-Keywords:** check.sh, race, golangci-lint

- [ ] **T018:** Auditar a feature contra a constituição e contra `docs/wiki-browser.md` e `docs/frontend-api.md`
  - **Papel:** analyst
  - **agent:** analyst1
  - **Dependências:** T017
  - **depends_on:** [T017]
  - **Paralelizável:** false
  - **Arquivos:** `specs/features/014-navegador-web-e-api/notes/constitution-audit.md`
  - **Wiki-Keywords:** constitution, auditoria, web


## Fase 4: Documentação

- [ ] **T019:** Atualizar o README com o navegador e a API
  - **Papel:** executor
  - **agent:** executor1
  - **Dependências:** T018
  - **depends_on:** [T018]
  - **Paralelizável:** true
  - **Arquivos:** `README.md`
  - **Wiki-Keywords:** README, web, API

- [ ] **T020:** Atualizar o llms.txt com a 014
  - **Papel:** executor
  - **agent:** executor2
  - **Dependências:** T019
  - **depends_on:** [T019]
  - **Paralelizável:** true
  - **Arquivos:** `llms.txt`
  - **Wiki-Keywords:** llms.txt, onboarding

- [ ] **T021:** Atualizar o .env.example com as chaves da 014
  - **Papel:** executor
  - **agent:** executor3
  - **Dependências:** T005
  - **depends_on:** [T005]
  - **Paralelizável:** true
  - **Arquivos:** `.env.example`
  - **Wiki-Keywords:** .env.example, web



## Coordination Graph

G₀ (round 0):

- **nodes:** T001, T002, T003, T004, T005, T006, T007, T008, T009, T010, T011, T012, T013, T014, T015, T016, T017, T018, T019, T020, T021
- **edges:**
  - T001 → T003
  - T001 → T004
  - T001 → T005
  - T004 → T006
  - T002 → T007
  - T007 → T008
  - T007 → T009
  - T007 → T010
  - T005 → T010
  - T006 → T011
  - T011 → T012
  - T005 → T012
  - T008 → T013
  - T009 → T013
  - T010 → T013
  - T012 → T013
  - T013 → T014
  - T003 → T014
  - T013 → T015
  - T003 → T015
  - T006 → T016
  - T003 → T016
  - T014 → T017
  - T015 → T017
  - T016 → T017
  - T017 → T018
  - T018 → T019
  - T019 → T020
  - T005 → T021
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
  - T018: analyst1
  - T019: executor1
  - T020: executor2
  - T021: executor3
- **ready:** [T001, T002]

> Workers: Lead, researcher1, analyst1, executor1..3. Validado por script: 21 tasks, sem ciclo, sem dependência quebrada, sem arquivo compartilhado entre tasks sem ordem. Mínimo de 11 rounds. Caminho crítico (calculado): T001 → T004 → T006 → T011 → T012 → T013 → T014 → T017 → T018 → T019 → T020.
