---
graph-operators: enabled
heartbeat-threshold: 4
max-rounds: 40
---

# tasks.md: SSO e identidade corporativa (OIDC)

> Feature `009-sso-oidc` · Spec: `spec.md` (draft) · Plano: `plan.md` (S-01 a S-05)
>
> **Provisório:** gerado em 2026-10-13, antes da execução das features anteriores. A T001 reconfere tudo contra o código entregue antes de qualquer execução.


## Fase 0: Descoberta

- [ ] **T001:** Reconferir este DAG contra o código entregue (ponto de autenticação da 007, cliente HTTP e drenagem da 002) e ajustar as tasks antes de executar
  - **Papel:** analyst
  - **agent:** analyst1
  - **Dependências:** Nenhuma
  - **depends_on:** []
  - **Paralelizável:** false
  - **Arquivos:** `specs/features/009-sso-oidc/notes/phase0-findings.md`
  - **Wiki-Keywords:** sso, autenticação entregue, cliente HTTP


## Fase 1: Fundação

- [ ] **T002:** Criar o arquivo .feature da 009 com os cenários da spec
  - **Papel:** executor
  - **agent:** executor1
  - **Dependências:** T001
  - **depends_on:** [T001]
  - **Paralelizável:** true
  - **Arquivos:** `specs/features/009-sso-oidc/acceptance/sso.feature`
  - **Wiki-Keywords:** Gherkin, oidc-device, proxy

- [ ] **T003:** Acrescentar `actor_proxy_bearer_token`, `root_issuer` e `root_subject` à configuração e às recusas da inicialização, com teste
  - **Papel:** executor
  - **agent:** executor2
  - **Dependências:** T001
  - **depends_on:** [T001]
  - **Paralelizável:** true
  - **Arquivos:** `internal/config/config.go`, `internal/config/config_test.go`
  - **Wiki-Keywords:** actor_proxy_bearer_token, root_issuer, root_subject


## Fase 2: Implementação

### 2A Cliente

- [ ] **T004:** Criar o fluxo de dispositivo OIDC (descoberta, autorização, sondagem com intervalo e limite, desfechos), com teste contra IdP de teste
  - **Papel:** executor
  - **agent:** executor3
  - **Dependências:** T001
  - **depends_on:** [T001]
  - **Paralelizável:** true
  - **Arquivos:** `internal/cli/oidc_device.go`, `internal/cli/oidc_device_test.go`
  - **Wiki-Keywords:** descoberta, dispositivo, sondagem, slow_down

- [ ] **T005:** Criar a leitura e a escrita atômica de `auth.json` preservando outros provedores, com teste
  - **Papel:** executor
  - **agent:** executor1
  - **Dependências:** T001
  - **depends_on:** [T001]
  - **Paralelizável:** true
  - **Arquivos:** `internal/cli/authjson.go`, `internal/cli/authjson_test.go`
  - **Wiki-Keywords:** auth.json, oidc, atômico, 0600

- [ ] **T006:** Criar a renovação do token (margem de 60 s, novo refresh token, salvar antes de usar, falhas silenciosas), com teste
  - **Papel:** executor
  - **agent:** executor2
  - **Dependências:** T004, T005
  - **depends_on:** [T004, T005]
  - **Paralelizável:** false
  - **Arquivos:** `internal/cli/oidc_refresh.go`, `internal/cli/oidc_refresh_test.go`
  - **Wiki-Keywords:** renovação, margem de 60 s, refresh_token

- [ ] **T007:** Criar a resolução de bearer com as ordens de hook e de CLI e ligá-la ao cliente HTTP e à drenagem do spool (modo `oidc` no item), com teste
  - **Papel:** executor
  - **agent:** executor3
  - **Dependências:** T006
  - **depends_on:** [T006]
  - **Paralelizável:** false
  - **Arquivos:** `internal/cli/bearer_order.go`, `internal/cli/bearer_order_test.go`
  - **Wiki-Keywords:** ordem de bearer, hook, CLI, spool

- [ ] **T008:** Criar os comandos `auth login oidc-device`, `auth status` e `auth logout oidc-device`, com teste
  - **Papel:** executor
  - **agent:** executor1
  - **Dependências:** T005, T004
  - **depends_on:** [T005, T004]
  - **Paralelizável:** false
  - **Arquivos:** `internal/cli/auth_cmd.go`, `internal/cli/auth_cmd_test.go`
  - **Wiki-Keywords:** auth login, status, logout


### 2B Servidor

- [ ] **T009:** Criar o degrau do proxy de identidade confiável no ponto de autenticação (cabeçalhos, 400, nível, chave de identidade), com teste
  - **Papel:** executor
  - **agent:** executor2
  - **Dependências:** T003
  - **depends_on:** [T003]
  - **Paralelizável:** true
  - **Arquivos:** `internal/cli/proxy_identity.go`, `internal/cli/proxy_identity_test.go`
  - **Wiki-Keywords:** X-Memory-Actor, 400, root por par

- [ ] **T010:** Inserir o degrau do proxy na ordem de resolução do bearer, entre root e credencial de banco
  - **Papel:** executor
  - **agent:** executor3
  - **Dependências:** T009
  - **depends_on:** [T009]
  - **Paralelizável:** false
  - **Arquivos:** `internal/cli/authn.go`
  - **Wiki-Keywords:** ordem de resolução, degrau do proxy

- [ ] **T011:** Registrar o comando `auth` na raiz da CLI
  - **Papel:** executor
  - **agent:** executor1
  - **Dependências:** T008
  - **depends_on:** [T008]
  - **Paralelizável:** false
  - **Arquivos:** `internal/cli/root.go`
  - **Wiki-Keywords:** cobra, raiz da CLI


### 2C Cenários Gherkin

- [ ] **T012:** Implementar Scenario: login por dispositivo, armazenamento e uso do token pelos hooks e pela CLI
  - **Papel:** executor
  - **agent:** executor2
  - **Dependências:** T008, T007, T002
  - **depends_on:** [T008, T007, T002]
  - **Paralelizável:** true
  - **Arquivos:** `test/acceptance/oidc_login_test.go`
  - **Wiki-Keywords:** login, armazenamento, bearer

- [ ] **T013:** Implementar tratamento de erro: desfechos de falha do login e da renovação
  - **Papel:** executor
  - **agent:** executor3
  - **Dependências:** T008, T002
  - **depends_on:** [T008, T002]
  - **Paralelizável:** true
  - **Arquivos:** `test/acceptance/oidc_failures_test.go`
  - **Wiki-Keywords:** access_denied, expired_token, sem refresh

- [ ] **T014:** Implementar Scenario: proxy confiável com atribuição por `oidc:`, root por par exato e recusas 400
  - **Papel:** executor
  - **agent:** executor1
  - **Dependências:** T010, T002
  - **depends_on:** [T010, T002]
  - **Paralelizável:** true
  - **Arquivos:** `test/acceptance/proxy_identity_test.go`
  - **Wiki-Keywords:** proxy, 400, root, atribuição


## Fase 3: Validação

- [ ] **T015:** Rodar os portões e registrar o resultado com a saída
  - **Papel:** executor
  - **agent:** executor2
  - **Dependências:** T011, T012, T013, T014
  - **depends_on:** [T011, T012, T013, T014]
  - **Paralelizável:** false
  - **Arquivos:** `specs/features/009-sso-oidc/notes/gates-run.md`
  - **Wiki-Keywords:** check.sh, race, golangci-lint

- [ ] **T016:** Auditar a feature contra a constituição e contra `docs/sso.md`
  - **Papel:** analyst
  - **agent:** analyst1
  - **Dependências:** T015
  - **depends_on:** [T015]
  - **Paralelizável:** false
  - **Arquivos:** `specs/features/009-sso-oidc/notes/constitution-audit.md`
  - **Wiki-Keywords:** constitution, auditoria, sso


## Fase 4: Documentação

- [ ] **T017:** Atualizar o README com o login OIDC e o formato de implantação com gateway
  - **Papel:** executor
  - **agent:** executor3
  - **Dependências:** T016
  - **depends_on:** [T016]
  - **Paralelizável:** true
  - **Arquivos:** `README.md`
  - **Wiki-Keywords:** README, SSO, gateway

- [ ] **T018:** Atualizar o llms.txt com a 009
  - **Papel:** executor
  - **agent:** executor1
  - **Dependências:** T017
  - **depends_on:** [T017]
  - **Paralelizável:** true
  - **Arquivos:** `llms.txt`
  - **Wiki-Keywords:** llms.txt, onboarding



## Coordination Graph

G₀ (round 0):

- **nodes:** T001, T002, T003, T004, T005, T006, T007, T008, T009, T010, T011, T012, T013, T014, T015, T016, T017, T018
- **edges:**
  - T001 → T002
  - T001 → T003
  - T001 → T004
  - T001 → T005
  - T004 → T006
  - T005 → T006
  - T006 → T007
  - T005 → T008
  - T004 → T008
  - T003 → T009
  - T009 → T010
  - T008 → T011
  - T008 → T012
  - T007 → T012
  - T002 → T012
  - T008 → T013
  - T002 → T013
  - T010 → T014
  - T002 → T014
  - T011 → T015
  - T012 → T015
  - T013 → T015
  - T014 → T015
  - T015 → T016
  - T016 → T017
  - T017 → T018
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
- **ready:** [T001]

> Workers: Lead, analyst1, executor1..3. Validado por script: 18 tasks, sem ciclo, sem dependência quebrada, sem arquivo compartilhado entre tasks sem ordem. Mínimo de 9 rounds. Caminho crítico (calculado): T001 → T004 → T006 → T007 → T012 → T015 → T016 → T017 → T018.
