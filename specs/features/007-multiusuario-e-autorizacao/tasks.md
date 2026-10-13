---
graph-operators: enabled
heartbeat-threshold: 4
max-rounds: 40
---

# tasks.md: Multiusuário, autenticação humana e autorização por projeto

> Feature `007-multiusuario-e-autorizacao` · Spec: `spec.md` (draft) · Plano: `plan.md` (U-01 a U-09)
>
> **Provisório:** gerado em 2026-10-13, antes da execução das features anteriores. A T001 reconfere tudo contra o código entregue antes de qualquer execução. Regras: uma ação por task; implementação e `_test.go` contam como um grupo. `go.mod` e `go.sum` só na T003.


## Fase 0: Descoberta

- [ ] **T001:** Reconferir este DAG contra o código entregue (middleware, resolvedor de escopo, handoffs com dono, WritePage) e ajustar as tasks antes de executar
  - **Papel:** analyst
  - **agent:** analyst1
  - **Dependências:** Nenhuma
  - **depends_on:** []
  - **Paralelizável:** false
  - **Arquivos:** `specs/features/007-multiusuario-e-autorizacao/notes/phase0-findings.md`
  - **Wiki-Keywords:** users, design-per-project-authz, código entregue

- [ ] **T002:** Extrair de `docs/security.md` e `docs/security-boundaries.md` as regras de sessão, login, pepper e recusas da inicialização que a spec resume
  - **Papel:** researcher
  - **agent:** researcher1
  - **Dependências:** Nenhuma
  - **depends_on:** []
  - **Paralelizável:** true
  - **Arquivos:** `specs/features/007-multiusuario-e-autorizacao/notes/security-contract.md`
  - **Wiki-Keywords:** security.md, sessão, login, recusas


## Fase 1: Fundação

- [ ] **T003:** Adicionar `golang.org/x/crypto` ao `go.mod` e registrar em `tech.md`
  - **Papel:** executor
  - **agent:** executor1
  - **Dependências:** T001
  - **depends_on:** [T001]
  - **Paralelizável:** true
  - **Arquivos:** `go.mod`, `go.sum`, `.github/memory/tech.md`
  - **Wiki-Keywords:** golang.org/x/crypto, argon2, tech.md

- [ ] **T004:** Acrescentar `[auth]`, `[slots] per_user` e as variáveis `AI_MEMORY_AUTH__*` à configuração, com teste
  - **Papel:** executor
  - **agent:** executor2
  - **Dependências:** T001
  - **depends_on:** [T001]
  - **Paralelizável:** true
  - **Arquivos:** `internal/config/config.go`, `internal/config/config_test.go`
  - **Wiki-Keywords:** [auth], [slots], AI_MEMORY_AUTH__

- [ ] **T005:** Criar o arquivo .feature da 007 com os cenários da spec
  - **Papel:** executor
  - **agent:** executor3
  - **Dependências:** T001, T002
  - **depends_on:** [T001, T002]
  - **Paralelizável:** true
  - **Arquivos:** `specs/features/007-multiusuario-e-autorizacao/acceptance/multiusuario.feature`
  - **Wiki-Keywords:** Gherkin, capacidades, acesso por projeto

- [ ] **T006:** Criar a chave de identidade (`user:` e `oidc:`), o nome de diretório por operador e os níveis, com teste
  - **Papel:** executor
  - **agent:** executor1
  - **Dependências:** T001
  - **depends_on:** [T001]
  - **Paralelizável:** true
  - **Arquivos:** `internal/domain/identity.go`, `internal/domain/identity_test.go`
  - **Wiki-Keywords:** chave de identidade, user:, oidc:, nível

- [ ] **T007:** Criar as regras de username, nome, e-mail e senha (mensagem genérica de política), com teste
  - **Papel:** executor
  - **agent:** executor2
  - **Dependências:** T001
  - **depends_on:** [T001]
  - **Paralelizável:** true
  - **Arquivos:** `internal/domain/accountrules.go`, `internal/domain/accountrules_test.go`
  - **Wiki-Keywords:** username, e-mail, senha, regras

- [ ] **T008:** Criar a geração e o digest das credenciais (`aim_`, `ams_`, CSRF, temporária, compatibilidade) e o hash Argon2id em PHC, com teste
  - **Papel:** executor
  - **agent:** executor3
  - **Dependências:** T003
  - **depends_on:** [T003]
  - **Paralelizável:** true
  - **Arquivos:** `internal/domain/credentials.go`, `internal/domain/credentials_test.go`
  - **Wiki-Keywords:** aim_, ams_, digest, pepper, argon2id


## Fase 2: Implementação

### 2A Storage

- [ ] **T009:** Criar contas (criar, listar com status, desabilitar, habilitar, alterar, último root recuperável), com teste
  - **Papel:** executor
  - **agent:** executor1
  - **Dependências:** T006, T007
  - **depends_on:** [T006, T007]
  - **Paralelizável:** true
  - **Arquivos:** `internal/storage/users.go`, `internal/storage/users_test.go`
  - **Wiki-Keywords:** contas, status, último root

- [ ] **T010:** Criar as chaves nativas e o token de compatibilidade (emitir, girar, revogar, listar, consultar pelo digest com validade), com teste
  - **Papel:** executor
  - **agent:** executor2
  - **Dependências:** T008, T009
  - **depends_on:** [T008, T009]
  - **Paralelizável:** false
  - **Arquivos:** `internal/storage/api_credentials.go`, `internal/storage/api_credentials_test.go`
  - **Wiki-Keywords:** chaves nativas, rotate, revoke, prévia

- [ ] **T011:** Criar as sessões web (criar, consultar pelo digest, último uso por minuto, revogar por conta), com teste
  - **Papel:** executor
  - **agent:** executor3
  - **Dependências:** T008, T009
  - **depends_on:** [T008, T009]
  - **Paralelizável:** false
  - **Arquivos:** `internal/storage/web_sessions.go`, `internal/storage/web_sessions_test.go`
  - **Wiki-Keywords:** sessão web, CSRF, 7 dias, revogação

- [ ] **T012:** Criar grants e modo de acesso de projeto (conceder, revogar, listar, auditoria, modo inicial por nome), com teste
  - **Papel:** executor
  - **agent:** executor1
  - **Dependências:** T009
  - **depends_on:** [T009]
  - **Paralelizável:** true
  - **Arquivos:** `internal/storage/grants.go`, `internal/storage/grants_test.go`
  - **Wiki-Keywords:** grants, read, write, auditoria

- [ ] **T013:** Criar a contagem de atividade de cliente MCP por dia (até 128 rótulos, `other`), com teste
  - **Papel:** executor
  - **agent:** executor2
  - **Dependências:** T001
  - **depends_on:** [T001]
  - **Paralelizável:** true
  - **Arquivos:** `internal/storage/client_activity.go`, `internal/storage/client_activity_test.go`
  - **Wiki-Keywords:** client_activity, 128 rótulos, flush


### 2B Autenticação e autorização

- [ ] **T014:** Criar a resolução de credencial no middleware (vazio, root em tempo constante, chave nativa, sessão nas rotas que aceitam) e o contexto do ator, com teste
  - **Papel:** executor
  - **agent:** executor3
  - **Dependências:** T010, T011, T004
  - **depends_on:** [T010, T011, T004]
  - **Paralelizável:** false
  - **Arquivos:** `internal/cli/authn.go`, `internal/cli/authn_test.go`
  - **Wiki-Keywords:** token portador, ordem de resolução, sessão

- [ ] **T015:** Criar `Can` (capacidades por nível) e `Decide` (regra por projeto na ordem do doc, com degradação e escopo global), com teste
  - **Papel:** executor
  - **agent:** executor1
  - **Dependências:** T006, T012
  - **depends_on:** [T006, T012]
  - **Paralelizável:** false
  - **Arquivos:** `internal/storage/authz.go`, `internal/storage/authz_test.go`
  - **Wiki-Keywords:** Can, Decide, capacidade, degradação

- [ ] **T016:** Aplicar a decisão de leitura no resolvedor de escopo e o predicado de visibilidade dentro das consultas sem escopo, com teste
  - **Papel:** executor
  - **agent:** executor2
  - **Dependências:** T015
  - **depends_on:** [T015]
  - **Paralelizável:** false
  - **Arquivos:** `internal/storage/scope_read.go`, `internal/storage/scope_read_test.go`
  - **Wiki-Keywords:** resolvedor, decisão de leitura, sem escopo

- [ ] **T017:** Aplicar a decisão de escrita na conexão do escritor, com teste multi-sessão de grant trocado durante a escrita
  - **Papel:** executor
  - **agent:** executor3
  - **Dependências:** T015
  - **depends_on:** [T015]
  - **Paralelizável:** false
  - **Arquivos:** `internal/storage/writer.go`, `internal/storage/writer_test.go`
  - **Wiki-Keywords:** decisão de escrita no escritor

- [ ] **T018:** Criar o processamento de senha fora do escritor com limite de 2 e verificação fictícia, com teste
  - **Papel:** executor
  - **agent:** executor1
  - **Dependências:** T014
  - **depends_on:** [T014]
  - **Paralelizável:** true
  - **Arquivos:** `internal/cli/password_pool.go`, `internal/cli/password_pool_test.go`
  - **Wiki-Keywords:** argon2id, 2 simultâneas, kdf saturated

- [ ] **T019:** Criar o limite de falhas de login por IP, com teste
  - **Papel:** executor
  - **agent:** executor2
  - **Dependências:** T014
  - **depends_on:** [T014]
  - **Paralelizável:** true
  - **Arquivos:** `internal/cli/login_ratelimit.go`, `internal/cli/login_ratelimit_test.go`
  - **Wiki-Keywords:** 10 falhas, 60 s, 4096 IPs


### 2C Dono, atribuição e entradas

- [ ] **T020:** Gravar o dono de sessões e handoffs só quando distingue operadores e filtrar leituras por próprias e compartilhadas, com teste multi-sessão
  - **Papel:** executor
  - **agent:** executor3
  - **Dependências:** T014
  - **depends_on:** [T014]
  - **Paralelizável:** true
  - **Arquivos:** `internal/storage/handoff_owner.go`, `internal/storage/handoff_owner_test.go`
  - **Wiki-Keywords:** dono, compartilhado, any_owner

- [ ] **T021:** Fazer o `WritePage` gravar `author_id` e o bloco `last_modified_by` conforme o ator, com teste
  - **Papel:** executor
  - **agent:** executor1
  - **Dependências:** T014
  - **depends_on:** [T014]
  - **Paralelizável:** true
  - **Arquivos:** `internal/wiki/write.go`, `internal/wiki/write_test.go`
  - **Wiki-Keywords:** author_id, last_modified_by, atribuição

- [ ] **T022:** Aplicar a decisão por projeto nas entradas por id cru (handoff no início de sessão e demais da tabela do doc), com teste
  - **Papel:** executor
  - **agent:** executor2
  - **Dependências:** T015
  - **depends_on:** [T015]
  - **Paralelizável:** true
  - **Arquivos:** `internal/cli/rawid_checks.go`, `internal/cli/rawid_checks_test.go`
  - **Wiki-Keywords:** id cru, handoff, mensagem, projeto da linha

- [ ] **T023:** Criar as fatias por operador (`[slots] per_user`): redirecionar escrita de slot compartilhado e recusar namespace alheio, com teste
  - **Papel:** executor
  - **agent:** executor3
  - **Dependências:** T004, T014
  - **depends_on:** [T004, T014]
  - **Paralelizável:** true
  - **Arquivos:** `internal/cli/slots_per_user.go`, `internal/cli/slots_per_user_test.go`
  - **Wiki-Keywords:** slots por operador, _slots, redirecionamento


### 2D Rotas e comandos

- [ ] **T024:** Criar as rotas `/auth/login`, `/auth/me`, `/auth/password`, `/auth/logout` e `/auth/recovery`, com teste
  - **Papel:** executor
  - **agent:** executor1
  - **Dependências:** T009, T011, T018, T019
  - **depends_on:** [T009, T011, T018, T019]
  - **Paralelizável:** false
  - **Arquivos:** `internal/cli/auth_routes.go`, `internal/cli/auth_routes_test.go`
  - **Wiki-Keywords:** /auth/login, /auth/me, /auth/password, recovery

- [ ] **T025:** Criar as rotas de contas, credenciais, grants, modo de acesso e `/identity`, com teste
  - **Papel:** executor
  - **agent:** executor2
  - **Dependências:** T009, T010, T012, T015
  - **depends_on:** [T009, T010, T012, T015]
  - **Paralelizável:** false
  - **Arquivos:** `internal/cli/admin_identity.go`, `internal/cli/admin_identity_test.go`
  - **Wiki-Keywords:** /admin/users, api-credentials, grants

- [ ] **T026:** Criar a rota `GET /admin/activity/by-client`, com teste
  - **Papel:** executor
  - **agent:** executor3
  - **Dependências:** T013
  - **depends_on:** [T013]
  - **Paralelizável:** true
  - **Arquivos:** `internal/cli/admin_activity.go`, `internal/cli/admin_activity_test.go`
  - **Wiki-Keywords:** /admin/activity/by-client

- [ ] **T027:** Montar as rotas da 007 e o novo middleware de autenticação no servidor HTTP
  - **Papel:** executor
  - **agent:** executor1
  - **Dependências:** T024, T025, T026
  - **depends_on:** [T024, T025, T026]
  - **Paralelizável:** false
  - **Arquivos:** `internal/cli/httpserver.go`
  - **Wiki-Keywords:** rotas, middleware, montagem

- [ ] **T028:** Criar a validação da inicialização e o bootstrap do primeiro root, incluindo as regras 2 e 3 de exposição de `docs/https-via-proxy.md` (autenticação humana fora de loopback exige `secure_cookie = true`; recusa caso contrário), com teste de cada recusa
  - **Papel:** executor
  - **agent:** executor2
  - **Dependências:** T009, T010, T008
  - **depends_on:** [T009, T010, T008]
  - **Paralelizável:** false
  - **Arquivos:** `internal/cli/startup_auth.go`, `internal/cli/startup_auth_test.go`
  - **Wiki-Keywords:** recusas, bootstrap, pepper

- [ ] **T029:** Ligar a validação e o bootstrap ao boot do `serve`
  - **Papel:** executor
  - **agent:** executor3
  - **Dependências:** T028, T027
  - **depends_on:** [T028, T027]
  - **Paralelizável:** false
  - **Arquivos:** `internal/cli/serve_cmd.go`
  - **Wiki-Keywords:** serve, validação, bootstrap

- [ ] **T030:** Criar os comandos `user`, `api-key`, `project`, `init` e `generate-auth-token`, com teste
  - **Papel:** executor
  - **agent:** executor1
  - **Dependências:** T027
  - **depends_on:** [T027]
  - **Paralelizável:** true
  - **Arquivos:** `internal/cli/identity_cmds.go`, `internal/cli/identity_cmds_test.go`
  - **Wiki-Keywords:** user, api-key, project, init, generate-auth-token

- [ ] **T031:** Acrescentar `--as-user` e `--auth-token` ao `install-hooks`, com teste
  - **Papel:** executor
  - **agent:** executor2
  - **Dependências:** T030
  - **depends_on:** [T030]
  - **Paralelizável:** false
  - **Arquivos:** `internal/cli/install_hooks.go`, `internal/cli/install_hooks_test.go`
  - **Wiki-Keywords:** install-hooks, --as-user, --auth-token

- [ ] **T032:** Registrar os comandos da 007 na raiz da CLI
  - **Papel:** executor
  - **agent:** executor3
  - **Dependências:** T031
  - **depends_on:** [T031]
  - **Paralelizável:** false
  - **Arquivos:** `internal/cli/root.go`
  - **Wiki-Keywords:** cobra, raiz da CLI


### 2E Cenários Gherkin

- [ ] **T033:** Implementar Scenario: conta, chave nativa, hooks por pessoa e atribuição das escritas
  - **Papel:** executor
  - **agent:** executor1
  - **Dependências:** T032, T029, T005
  - **depends_on:** [T032, T029, T005]
  - **Paralelizável:** true
  - **Arquivos:** `test/acceptance/accounts_test.go`
  - **Wiki-Keywords:** conta, chave, atribuição, dono

- [ ] **T034:** Implementar tratamento de erro: credenciais inválidas com tempo igual, limite por IP e troca de senha pendente
  - **Papel:** executor
  - **agent:** executor2
  - **Dependências:** T027, T005
  - **depends_on:** [T027, T005]
  - **Paralelizável:** true
  - **Arquivos:** `test/acceptance/login_test.go`
  - **Wiki-Keywords:** login, 401, 429, must-change

- [ ] **T035:** Implementar Scenario: matriz de capacidades nos dois modos, incluindo o fechamento do acesso administrativo ao criar a primeira conta
  - **Papel:** executor
  - **agent:** executor3
  - **Dependências:** T027, T005
  - **depends_on:** [T027, T005]
  - **Paralelizável:** true
  - **Arquivos:** `test/acceptance/capabilities_test.go`
  - **Wiki-Keywords:** matriz de capacidades, um operador, multiusuário

- [ ] **T036:** Implementar Scenario: acesso por projeto restrito, grants, escopo global e degradação
  - **Papel:** executor
  - **agent:** executor1
  - **Dependências:** T016, T017, T022, T005
  - **depends_on:** [T016, T017, T022, T005]
  - **Paralelizável:** true
  - **Arquivos:** `test/acceptance/project_access_test.go`
  - **Wiki-Keywords:** restrito, grant, global, degradação

- [ ] **T037:** Implementar edge case: leitura sem escopo filtra dentro da consulta, antes do limite
  - **Papel:** executor
  - **agent:** executor2
  - **Dependências:** T016, T005
  - **depends_on:** [T016, T005]
  - **Paralelizável:** true
  - **Arquivos:** `test/acceptance/unscoped_filter_test.go`
  - **Wiki-Keywords:** filtro antes do limite, global

- [ ] **T038:** Implementar tratamento de erro: cada recusa da inicialização impede o servidor de subir
  - **Papel:** executor
  - **agent:** executor3
  - **Dependências:** T029, T005
  - **depends_on:** [T029, T005]
  - **Paralelizável:** true
  - **Arquivos:** `test/acceptance/startup_refusals_test.go`
  - **Wiki-Keywords:** recusas da inicialização

- [ ] **T039:** Implementar edge case: operador X não recebe handoff do operador Y; handoffs sem dono continuam visíveis
  - **Papel:** executor
  - **agent:** executor1
  - **Dependências:** T020, T005
  - **depends_on:** [T020, T005]
  - **Paralelizável:** true
  - **Arquivos:** `test/acceptance/owner_isolation_test.go`
  - **Wiki-Keywords:** dono, operador X, operador Y


## Fase 3: Validação

- [ ] **T040:** Criar testes de contrato da autorização e adversariais do `docs/mcp-server.md` §7
  - **Papel:** executor
  - **agent:** executor2
  - **Dependências:** T027
  - **depends_on:** [T027]
  - **Paralelizável:** true
  - **Arquivos:** `test/contract/authz_inv_test.go`
  - **Wiki-Keywords:** autorização, tempo constante, INV-16

- [ ] **T041:** Rodar os portões e registrar o resultado com a saída
  - **Papel:** executor
  - **agent:** executor3
  - **Dependências:** T033, T034, T035, T036, T037, T038, T039, T040, T021, T023
  - **depends_on:** [T033, T034, T035, T036, T037, T038, T039, T040, T021, T023]
  - **Paralelizável:** false
  - **Arquivos:** `specs/features/007-multiusuario-e-autorizacao/notes/gates-run.md`
  - **Wiki-Keywords:** check.sh, race, golangci-lint

- [ ] **T042:** Auditar a feature contra a constituição e contra `docs/users.md` e `docs/design-per-project-authz.md`
  - **Papel:** analyst
  - **agent:** analyst1
  - **Dependências:** T041
  - **depends_on:** [T041]
  - **Paralelizável:** false
  - **Arquivos:** `specs/features/007-multiusuario-e-autorizacao/notes/constitution-audit.md`
  - **Wiki-Keywords:** constitution, auditoria, users


## Fase 4: Documentação

- [ ] **T043:** Atualizar o README com contas, chaves, login e acesso por projeto
  - **Papel:** executor
  - **agent:** executor1
  - **Dependências:** T042
  - **depends_on:** [T042]
  - **Paralelizável:** true
  - **Arquivos:** `README.md`
  - **Wiki-Keywords:** README, contas, chaves, acesso

- [ ] **T044:** Atualizar o llms.txt com a 007
  - **Papel:** executor
  - **agent:** executor2
  - **Dependências:** T043
  - **depends_on:** [T043]
  - **Paralelizável:** true
  - **Arquivos:** `llms.txt`
  - **Wiki-Keywords:** llms.txt, onboarding

- [ ] **T045:** Atualizar o .env.example com as chaves de `[auth]` (sem segredos)
  - **Papel:** executor
  - **agent:** executor3
  - **Dependências:** T004
  - **depends_on:** [T004]
  - **Paralelizável:** true
  - **Arquivos:** `.env.example`
  - **Wiki-Keywords:** .env.example, auth



## Coordination Graph

G₀ (round 0):

- **nodes:** T001, T002, T003, T004, T005, T006, T007, T008, T009, T010, T011, T012, T013, T014, T015, T016, T017, T018, T019, T020, T021, T022, T023, T024, T025, T026, T027, T028, T029, T030, T031, T032, T033, T034, T035, T036, T037, T038, T039, T040, T041, T042, T043, T044, T045
- **edges:**
  - T001 → T003
  - T001 → T004
  - T001 → T005
  - T002 → T005
  - T001 → T006
  - T001 → T007
  - T003 → T008
  - T006 → T009
  - T007 → T009
  - T008 → T010
  - T009 → T010
  - T008 → T011
  - T009 → T011
  - T009 → T012
  - T001 → T013
  - T010 → T014
  - T011 → T014
  - T004 → T014
  - T006 → T015
  - T012 → T015
  - T015 → T016
  - T015 → T017
  - T014 → T018
  - T014 → T019
  - T014 → T020
  - T014 → T021
  - T015 → T022
  - T004 → T023
  - T014 → T023
  - T009 → T024
  - T011 → T024
  - T018 → T024
  - T019 → T024
  - T009 → T025
  - T010 → T025
  - T012 → T025
  - T015 → T025
  - T013 → T026
  - T024 → T027
  - T025 → T027
  - T026 → T027
  - T009 → T028
  - T010 → T028
  - T008 → T028
  - T028 → T029
  - T027 → T029
  - T027 → T030
  - T030 → T031
  - T031 → T032
  - T032 → T033
  - T029 → T033
  - T005 → T033
  - T027 → T034
  - T005 → T034
  - T027 → T035
  - T005 → T035
  - T016 → T036
  - T017 → T036
  - T022 → T036
  - T005 → T036
  - T016 → T037
  - T005 → T037
  - T029 → T038
  - T005 → T038
  - T020 → T039
  - T005 → T039
  - T027 → T040
  - T033 → T041
  - T034 → T041
  - T035 → T041
  - T036 → T041
  - T037 → T041
  - T038 → T041
  - T039 → T041
  - T040 → T041
  - T021 → T041
  - T023 → T041
  - T041 → T042
  - T042 → T043
  - T043 → T044
  - T004 → T045
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
  - T027: executor1
  - T028: executor2
  - T029: executor3
  - T030: executor1
  - T031: executor2
  - T032: executor3
  - T033: executor1
  - T034: executor2
  - T035: executor3
  - T036: executor1
  - T037: executor2
  - T038: executor3
  - T039: executor1
  - T040: executor2
  - T041: executor3
  - T042: analyst1
  - T043: executor1
  - T044: executor2
  - T045: executor3
- **ready:** [T001, T002]

> Workers: Lead, researcher1, analyst1, executor1..3. Validado por script: 45 tasks, sem ciclo, sem dependência quebrada, sem arquivo compartilhado entre tasks sem ordem. Mínimo de 16 rounds. Caminho crítico (calculado): T001 → T003 → T008 → T010 → T014 → T018 → T024 → T027 → T030 → T031 → T032 → T033 → T041 → T042 → T043 → T044.
