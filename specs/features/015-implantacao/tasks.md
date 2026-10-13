---
graph-operators: enabled
heartbeat-threshold: 4
max-rounds: 40
---

# tasks.md: Implantação

> Feature `015-implantacao` · Spec: `spec.md` (draft) · Plano: `plan.md` (D-01 a D-05)
>
> **Provisório:** gerado em 2026-10-13, antes da execução das features anteriores. A T001 reconfere tudo contra o código entregue antes de qualquer execução. Sem dependência nova no `go.mod`.


## Fase 0: Descoberta

- [ ] **T001:** Reconferir este DAG contra o código entregue (normalizador da 014, limitador de login da 007, troca atômica e regras de caminho da 001) e consolidar as notas da fase 0
  - **Papel:** analyst
  - **agent:** analyst1
  - **Dependências:** Nenhuma
  - **depends_on:** []
  - **Paralelizável:** false
  - **Arquivos:** `specs/features/015-implantacao/notes/phase0-findings.md`
  - **Wiki-Keywords:** deploy, 014 entregue, atomicfile, pagepath

- [ ] **T002:** Extrair os contratos de composições, wrappers, unidades e rotinas externas de `docs/deploy.md`, `docs/backup.md` e `docs/hook-scripts-and-container-deploy.md`
  - **Papel:** researcher
  - **agent:** researcher1
  - **Dependências:** Nenhuma
  - **depends_on:** []
  - **Paralelizável:** true
  - **Arquivos:** `specs/features/015-implantacao/notes/deploy-contracts.md`
  - **Wiki-Keywords:** deploy.md, compose, wrapper, systemd


## Fase 1: Fundação

- [ ] **T003:** Criar o arquivo .feature da 015 com os cenários da spec
  - **Papel:** executor
  - **agent:** executor1
  - **Dependências:** T001
  - **depends_on:** [T001]
  - **Paralelizável:** true
  - **Arquivos:** `specs/features/015-implantacao/acceptance/deploy.feature`
  - **Wiki-Keywords:** Gherkin, upgrade, prefixo

- [ ] **T004:** Acrescentar à configuração `release_base_url`, `base_path`, `trusted_proxy_cidrs` e `tcp_keepalive_secs`, com validação e teste
  - **Papel:** executor
  - **agent:** executor2
  - **Dependências:** T001
  - **depends_on:** [T001]
  - **Paralelizável:** true
  - **Arquivos:** `internal/config/config.go`, `internal/config/config_test.go`
  - **Wiki-Keywords:** release_base_url, base_path, trusted_proxy_cidrs, tcp_keepalive_secs


## Fase 2: Implementação

### 2A Binário

- [ ] **T005:** Criar o comando `upgrade` (tag fixa ou última, download, checksum antes de extrair, troca do executável, reinstalação de hooks), com teste contra servidor de releases falso
  - **Papel:** executor
  - **agent:** executor3
  - **Dependências:** T004
  - **depends_on:** [T004]
  - **Paralelizável:** true
  - **Arquivos:** `internal/cli/upgrade.go`, `internal/cli/upgrade_test.go`
  - **Wiki-Keywords:** upgrade, sha256, release, troca do executável

- [ ] **T006:** Criar a resolução do IP do cliente com `X-Forwarded-For` e CIDRs confiáveis, com teste de tabela
  - **Papel:** executor
  - **agent:** executor1
  - **Dependências:** T004
  - **depends_on:** [T004]
  - **Paralelizável:** true
  - **Arquivos:** `internal/cli/clientip.go`, `internal/cli/clientip_test.go`
  - **Wiki-Keywords:** X-Forwarded-For, CIDR confiável

- [ ] **T007:** Ligar o IP do cliente ao limitador de login e de recuperação da 007, com teste
  - **Papel:** executor
  - **agent:** executor2
  - **Dependências:** T006
  - **depends_on:** [T006]
  - **Paralelizável:** false
  - **Arquivos:** `internal/cli/login_ratelimit.go`, `internal/cli/login_ratelimit_test.go`
  - **Wiki-Keywords:** limite de login, IP do cliente

- [ ] **T008:** Montar todas as rotas sob `base_path` com o normalizador da 014 e aplicar o keepalive TCP por conexão, com teste
  - **Papel:** executor
  - **agent:** executor3
  - **Dependências:** T007
  - **depends_on:** [T007]
  - **Paralelizável:** false
  - **Arquivos:** `internal/cli/httpserver.go`, `internal/cli/httpserver_test.go`
  - **Wiki-Keywords:** base_path, keepalive, prefixo

- [ ] **T009:** Fazer o cliente HTTP usar o caminho da URL do servidor como prefixo base, com teste
  - **Papel:** executor
  - **agent:** executor1
  - **Dependências:** T004
  - **depends_on:** [T004]
  - **Paralelizável:** true
  - **Arquivos:** `internal/cli/httpclient.go`, `internal/cli/httpclient_test.go`
  - **Wiki-Keywords:** cliente HTTP, prefixo base

- [ ] **T010:** Detectar o erro de dono do repositório da wiki e registrá-lo em nível de erro na inicialização com a orientação, com teste
  - **Papel:** executor
  - **agent:** executor2
  - **Dependências:** T001
  - **depends_on:** [T001]
  - **Paralelizável:** true
  - **Arquivos:** `internal/wiki/gitrepo.go`, `internal/wiki/gitrepo_test.go`
  - **Wiki-Keywords:** dono do repositório, safe.directory, erro

- [ ] **T011:** Criar o teste de que a configuração padrão não abre conexão de saída além do loopback
  - **Papel:** executor
  - **agent:** executor3
  - **Dependências:** T008
  - **depends_on:** [T008]
  - **Paralelizável:** false
  - **Arquivos:** `internal/cli/egress_test.go`
  - **Wiki-Keywords:** sem rede, discador, loopback


### 2B Artefatos

- [ ] **T012:** Criar o Dockerfile (build Go, runtime bookworm-slim, usuário 1000, variáveis, healthcheck, entrypoint, hooks, alvos pré-compilados)
  - **Papel:** executor
  - **agent:** executor1
  - **Dependências:** T002
  - **depends_on:** [T002]
  - **Paralelizável:** true
  - **Arquivos:** `deploy/Dockerfile`, `.dockerignore`
  - **Wiki-Keywords:** Dockerfile, bookworm-slim, uid 1000, healthcheck

- [ ] **T013:** Criar as composições de referência e os arquivos de ambiente de exemplo sem aspas
  - **Papel:** executor
  - **agent:** executor2
  - **Dependências:** T012
  - **depends_on:** [T012]
  - **Paralelizável:** false
  - **Arquivos:** `deploy/compose/`
  - **Wiki-Keywords:** compose, homelab, Caddy, túnel, multioperador

- [ ] **T014:** Criar as unidades de sistema e de usuário, sysusers, tmpfiles e as unidades de timer do snapshot
  - **Papel:** executor
  - **agent:** executor3
  - **Dependências:** T002
  - **depends_on:** [T002]
  - **Paralelizável:** true
  - **Arquivos:** `deploy/systemd/`
  - **Wiki-Keywords:** systemd, sysusers, tmpfiles, timer

- [ ] **T015:** Criar o wrapper POSIX (montagens, variáveis, cliente nativo verificado, `upgrade` em seis passos), com teste de shell
  - **Papel:** executor
  - **agent:** executor1
  - **Dependências:** T002
  - **depends_on:** [T002]
  - **Paralelizável:** true
  - **Arquivos:** `contrib/wrapper/ai-memory.sh`, `contrib/wrapper/ai-memory_test.sh`
  - **Wiki-Keywords:** wrapper POSIX, cliente nativo, upgrade

- [ ] **T016:** Criar os wrappers PowerShell e cmd do Windows, com teste
  - **Papel:** executor
  - **agent:** executor2
  - **Dependências:** T002
  - **depends_on:** [T002]
  - **Paralelizável:** true
  - **Arquivos:** `contrib/wrapper/ai-memory.ps1`, `contrib/wrapper/ai-memory.cmd`, `contrib/wrapper/wrapper.Tests.ps1`
  - **Wiki-Keywords:** wrapper PowerShell, cmd, Docker Desktop

- [ ] **T017:** Criar o script de snapshot com espelho e push e o exemplo de compactação condicional
  - **Papel:** executor
  - **agent:** executor3
  - **Dependências:** T002
  - **depends_on:** [T002]
  - **Paralelizável:** true
  - **Arquivos:** `contrib/backup/`, `contrib/maintenance/`
  - **Wiki-Keywords:** snapshot, espelho, push, compactação condicional

- [ ] **T018:** Criar o exemplo de serviço WinSW e o guia da conta de serviço
  - **Papel:** executor
  - **agent:** executor1
  - **Dependências:** T002
  - **depends_on:** [T002]
  - **Paralelizável:** true
  - **Arquivos:** `deploy/windows/`
  - **Wiki-Keywords:** WinSW, conta de serviço

- [ ] **T019:** Criar o workflow de release com os três arquivos e seus `.sha256`
  - **Papel:** executor
  - **agent:** executor2
  - **Dependências:** T012
  - **depends_on:** [T012]
  - **Paralelizável:** false
  - **Arquivos:** `.github/workflows/release.yml`
  - **Wiki-Keywords:** release, sha256, build cruzado

- [ ] **T020:** Acrescentar aos scripts de checagem o smoke da imagem e das composições quando houver Docker
  - **Papel:** executor
  - **agent:** executor3
  - **Dependências:** T012, T013
  - **depends_on:** [T012, T013]
  - **Paralelizável:** false
  - **Arquivos:** `scripts/check.sh`, `scripts/check.ps1`
  - **Wiki-Keywords:** docker build, compose config, smoke


### Cenários Gherkin

- [ ] **T021:** Implementar Scenario: `upgrade` a partir de espelho interno, com checksum errado recusado
  - **Papel:** executor
  - **agent:** executor1
  - **Dependências:** T005, T003
  - **depends_on:** [T005, T003]
  - **Paralelizável:** true
  - **Arquivos:** `test/acceptance/upgrade_test.go`
  - **Wiki-Keywords:** upgrade, checksum, espelho

- [ ] **T022:** Implementar Scenario: servidor sob `/wiki` e prefixo inválido montado na raiz
  - **Papel:** executor
  - **agent:** executor2
  - **Dependências:** T008, T003
  - **depends_on:** [T008, T003]
  - **Paralelizável:** true
  - **Arquivos:** `test/acceptance/base_path_test.go`
  - **Wiki-Keywords:** subcaminho, prefixo inválido

- [ ] **T023:** Implementar edge case: `X-Forwarded-For` não confiável, inválido e todo confiável
  - **Papel:** executor
  - **agent:** executor3
  - **Dependências:** T007, T003
  - **depends_on:** [T007, T003]
  - **Paralelizável:** true
  - **Arquivos:** `test/acceptance/client_ip_test.go`
  - **Wiki-Keywords:** X-Forwarded-For, todos confiáveis


## Fase 3: Validação

- [ ] **T024:** Rodar os portões e registrar o resultado com a saída
  - **Papel:** executor
  - **agent:** executor1
  - **Dependências:** T009, T010, T011, T014, T015, T016, T017, T018, T019, T020, T021, T022, T023
  - **depends_on:** [T009, T010, T011, T014, T015, T016, T017, T018, T019, T020, T021, T022, T023]
  - **Paralelizável:** false
  - **Arquivos:** `specs/features/015-implantacao/notes/gates-run.md`
  - **Wiki-Keywords:** check.sh, race, golangci-lint

- [ ] **T025:** Auditar a feature contra a constituição e contra os docs de implantação
  - **Papel:** analyst
  - **agent:** analyst1
  - **Dependências:** T024
  - **depends_on:** [T024]
  - **Paralelizável:** false
  - **Arquivos:** `specs/features/015-implantacao/notes/constitution-audit.md`
  - **Wiki-Keywords:** constitution, auditoria, implantação


## Fase 4: Documentação

- [ ] **T026:** Atualizar o README com contêiner, releases, `upgrade` e proxy
  - **Papel:** executor
  - **agent:** executor2
  - **Dependências:** T025
  - **depends_on:** [T025]
  - **Paralelizável:** true
  - **Arquivos:** `README.md`
  - **Wiki-Keywords:** README, deploy, upgrade

- [ ] **T027:** Atualizar o llms.txt com a 015
  - **Papel:** executor
  - **agent:** executor3
  - **Dependências:** T026
  - **depends_on:** [T026]
  - **Paralelizável:** true
  - **Arquivos:** `llms.txt`
  - **Wiki-Keywords:** llms.txt, onboarding

- [ ] **T028:** Atualizar o .env.example com as chaves da 015
  - **Papel:** executor
  - **agent:** executor1
  - **Dependências:** T004
  - **depends_on:** [T004]
  - **Paralelizável:** true
  - **Arquivos:** `.env.example`
  - **Wiki-Keywords:** .env.example, deploy



## Coordination Graph

G₀ (round 0):

- **nodes:** T001, T002, T003, T004, T005, T006, T007, T008, T009, T010, T011, T012, T013, T014, T015, T016, T017, T018, T019, T020, T021, T022, T023, T024, T025, T026, T027, T028
- **edges:**
  - T001 → T003
  - T001 → T004
  - T004 → T005
  - T004 → T006
  - T006 → T007
  - T007 → T008
  - T004 → T009
  - T001 → T010
  - T008 → T011
  - T002 → T012
  - T012 → T013
  - T002 → T014
  - T002 → T015
  - T002 → T016
  - T002 → T017
  - T002 → T018
  - T012 → T019
  - T012 → T020
  - T013 → T020
  - T005 → T021
  - T003 → T021
  - T008 → T022
  - T003 → T022
  - T007 → T023
  - T003 → T023
  - T009 → T024
  - T010 → T024
  - T011 → T024
  - T014 → T024
  - T015 → T024
  - T016 → T024
  - T017 → T024
  - T018 → T024
  - T019 → T024
  - T020 → T024
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

> Workers: Lead, researcher1, analyst1, executor1..3. Validado por script: 28 tasks, sem ciclo, sem dependência quebrada, sem arquivo compartilhado entre tasks sem ordem. Mínimo de 10 rounds. Caminho crítico (calculado): T001 → T004 → T006 → T007 → T008 → T011 → T024 → T025 → T026 → T027.
