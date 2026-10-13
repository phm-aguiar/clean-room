---
graph-operators: enabled
heartbeat-threshold: 4
max-rounds: 40
---

# tasks.md: Formato OKF e migrações de estrutura da wiki

> Feature `006-okf-e-migracoes-de-wiki` · Spec: `spec.md` (draft) · Plano: `plan.md` (K-01 a K-08)
>
> **Provisório:** gerado em 2026-10-13, antes da execução das features anteriores. A T001 reconfere tudo contra o código entregue antes de qualquer execução. Regras: uma ação por task; implementação e `_test.go` contam como um grupo.


## Fase 0: Descoberta

- [ ] **T001:** Reconferir este DAG contra o código entregue (boot do serve, conformidade na escrita, frontmatter com mapeamentos) e ajustar as tasks antes de executar
  - **Papel:** analyst
  - **agent:** analyst1
  - **Dependências:** Nenhuma
  - **depends_on:** []
  - **Paralelizável:** false
  - **Arquivos:** `specs/features/006-okf-e-migracoes-de-wiki/notes/phase0-findings.md`
  - **Wiki-Keywords:** okf, wiki-migrations, código entregue


## Fase 1: Fundação

- [ ] **T002:** Criar o arquivo .feature da 006 com os cenários da spec
  - **Papel:** executor
  - **agent:** executor1
  - **Dependências:** T001
  - **depends_on:** [T001]
  - **Paralelizável:** true
  - **Arquivos:** `specs/features/006-okf-e-migracoes-de-wiki/acceptance/okf.feature`
  - **Wiki-Keywords:** Gherkin, okf, migração

- [ ] **T003:** Acrescentar `AI_MEMORY_BACKUP_DIR` à configuração, com teste
  - **Papel:** executor
  - **agent:** executor2
  - **Dependências:** T001
  - **depends_on:** [T001]
  - **Paralelizável:** true
  - **Arquivos:** `internal/config/config.go`, `internal/config/config_test.go`
  - **Wiki-Keywords:** AI_MEMORY_BACKUP_DIR, INV-01

- [ ] **T004:** Criar a verificação pura de conformidade OKF (`type` não vazio; `generated.at` exceto em `_meta.md`) e a conformação de frontmatter idempotente, com teste
  - **Papel:** executor
  - **agent:** executor3
  - **Dependências:** T001
  - **depends_on:** [T001]
  - **Paralelizável:** true
  - **Arquivos:** `internal/domain/okfconform.go`, `internal/domain/okfconform_test.go`
  - **Wiki-Keywords:** conformidade okf, type, generated.at, _meta


## Fase 2: Implementação

### 2A Executor e migração

- [ ] **T005:** Criar o executor de migrações da wiki (ordem, registro pelo escritor, recusa de nome desconhecido), com teste
  - **Papel:** executor
  - **agent:** executor1
  - **Dependências:** T001
  - **depends_on:** [T001]
  - **Paralelizável:** true
  - **Arquivos:** `internal/wiki/migrations.go`, `internal/wiki/migrations_test.go`
  - **Wiki-Keywords:** executor, wiki_migrations, formato mais novo

- [ ] **T006:** Criar a cópia de segurança prévia (destino, conteúdo, verificação, recibo por substituição atômica, reuso, detecção de contêiner), com teste
  - **Papel:** executor
  - **agent:** executor2
  - **Dependências:** T003
  - **depends_on:** [T003]
  - **Paralelizável:** true
  - **Arquivos:** `internal/cli/premigration_backup.go`, `internal/cli/premigration_backup_test.go`
  - **Wiki-Keywords:** cópia prévia, recibo, verificação, contêiner

- [ ] **T007:** Criar a migração de conformidade OKF (pendências, checkpoint, passo do banco, passo dos arquivos, `index.md`, commit único), com teste de pré, pós e reexecução
  - **Papel:** executor
  - **agent:** executor3
  - **Dependências:** T004, T005
  - **depends_on:** [T004, T005]
  - **Paralelizável:** false
  - **Arquivos:** `internal/wiki/migration_okf.go`, `internal/wiki/migration_okf_test.go`
  - **Wiki-Keywords:** migração okf, sem churn, commit único

- [ ] **T008:** Criar o reparo idempotente de `stale_after` de data pura, com teste
  - **Papel:** executor
  - **agent:** executor1
  - **Dependências:** T004
  - **depends_on:** [T004]
  - **Paralelizável:** true
  - **Arquivos:** `internal/wiki/stale_after_repair.go`, `internal/wiki/stale_after_repair_test.go`
  - **Wiki-Keywords:** stale_after, data pura, reparo

- [ ] **T009:** Ligar ao boot do `serve` a cópia prévia, o executor e o reparo, na ordem do doc
  - **Papel:** executor
  - **agent:** executor2
  - **Dependências:** T006, T007, T008
  - **depends_on:** [T006, T007, T008]
  - **Paralelizável:** false
  - **Arquivos:** `internal/cli/serve_cmd.go`
  - **Wiki-Keywords:** ordem de boot, migrações da wiki


### 2B Exportação

- [ ] **T010:** Criar a reescrita de links internos para o pacote OKF, com teste
  - **Papel:** executor
  - **agent:** executor3
  - **Dependências:** T004
  - **depends_on:** [T004]
  - **Paralelizável:** true
  - **Arquivos:** `internal/wiki/export_links.go`, `internal/wiki/export_links_test.go`
  - **Wiki-Keywords:** reescrita de links, relativo, código cercado

- [ ] **T011:** Criar a montagem do pacote OKF (exclusões, conformidade exigida, `index.md` gerado, título e descrição na cópia), com teste
  - **Papel:** executor
  - **agent:** executor1
  - **Dependências:** T004, T010
  - **depends_on:** [T004, T010]
  - **Paralelizável:** false
  - **Arquivos:** `internal/wiki/export_okf.go`, `internal/wiki/export_okf_test.go`
  - **Wiki-Keywords:** exportação, exclusões, index.md gerado

- [ ] **T012:** Criar a rota `POST /admin/export-okf`, com teste
  - **Papel:** executor
  - **agent:** executor2
  - **Dependências:** T011
  - **depends_on:** [T011]
  - **Paralelizável:** true
  - **Arquivos:** `internal/cli/admin_export_okf.go`, `internal/cli/admin_export_okf_test.go`
  - **Wiki-Keywords:** POST /admin/export-okf, gzip

- [ ] **T013:** Montar a rota da 006 no servidor HTTP
  - **Papel:** executor
  - **agent:** executor3
  - **Dependências:** T012
  - **depends_on:** [T012]
  - **Paralelizável:** false
  - **Arquivos:** `internal/cli/httpserver.go`
  - **Wiki-Keywords:** rotas, montagem

- [ ] **T014:** Criar o comando `export-okf`, com teste
  - **Papel:** executor
  - **agent:** executor1
  - **Dependências:** T013
  - **depends_on:** [T013]
  - **Paralelizável:** true
  - **Arquivos:** `internal/cli/export_okf_cmd.go`, `internal/cli/export_okf_cmd_test.go`
  - **Wiki-Keywords:** export-okf, -o, --to

- [ ] **T015:** Mostrar no `status` a cópia de segurança enquanto ela estiver em disco, com teste
  - **Papel:** executor
  - **agent:** executor2
  - **Dependências:** T006
  - **depends_on:** [T006]
  - **Paralelizável:** true
  - **Arquivos:** `internal/cli/status_cmd.go`, `internal/cli/status_cmd_test.go`
  - **Wiki-Keywords:** status, cópia de segurança

- [ ] **T016:** Registrar o comando `export-okf` na raiz da CLI
  - **Papel:** executor
  - **agent:** executor3
  - **Dependências:** T014
  - **depends_on:** [T014]
  - **Paralelizável:** false
  - **Arquivos:** `internal/cli/root.go`
  - **Wiki-Keywords:** cobra, raiz da CLI


### 2C Cenários Gherkin

- [ ] **T017:** Implementar Scenario: instalação nova registra a migração sem efeito; wiki com arquivos sem `type` é migrada com cópia e commit único
  - **Papel:** executor
  - **agent:** executor1
  - **Dependências:** T009, T002
  - **depends_on:** [T009, T002]
  - **Paralelizável:** true
  - **Arquivos:** `test/acceptance/okf_migration_test.go`
  - **Wiki-Keywords:** migração, instalação nova, arquivos à mão

- [ ] **T018:** Implementar tratamento de erro: cópia que não grava aborta sem tocar em dados; nome desconhecido recusa a abertura
  - **Papel:** executor
  - **agent:** executor2
  - **Dependências:** T009, T002
  - **depends_on:** [T009, T002]
  - **Paralelizável:** true
  - **Arquivos:** `test/acceptance/okf_migration_failures_test.go`
  - **Wiki-Keywords:** cópia falha, formato mais novo

- [ ] **T019:** Implementar Scenario: exportação sem ledger, com links reescritos, e falha com página não conforme
  - **Papel:** executor
  - **agent:** executor3
  - **Dependências:** T016, T002
  - **depends_on:** [T016, T002]
  - **Paralelizável:** true
  - **Arquivos:** `test/acceptance/okf_export_test.go`
  - **Wiki-Keywords:** exportação, ledger, links, não conforme


## Fase 3: Validação

- [ ] **T020:** Rodar os portões e registrar o resultado com a saída
  - **Papel:** executor
  - **agent:** executor1
  - **Dependências:** T017, T018, T019, T015
  - **depends_on:** [T017, T018, T019, T015]
  - **Paralelizável:** false
  - **Arquivos:** `specs/features/006-okf-e-migracoes-de-wiki/notes/gates-run.md`
  - **Wiki-Keywords:** check.sh, race, golangci-lint

- [ ] **T021:** Auditar a feature contra a constituição e contra `docs/okf.md` e `docs/wiki-migrations.md`
  - **Papel:** analyst
  - **agent:** analyst1
  - **Dependências:** T020
  - **depends_on:** [T020]
  - **Paralelizável:** false
  - **Arquivos:** `specs/features/006-okf-e-migracoes-de-wiki/notes/constitution-audit.md`
  - **Wiki-Keywords:** constitution, auditoria, okf


## Fase 4: Documentação

- [ ] **T022:** Atualizar o README com a exportação OKF, a migração e a reversão
  - **Papel:** executor
  - **agent:** executor2
  - **Dependências:** T021
  - **depends_on:** [T021]
  - **Paralelizável:** true
  - **Arquivos:** `README.md`
  - **Wiki-Keywords:** README, okf, export

- [ ] **T023:** Atualizar o llms.txt com a 006
  - **Papel:** executor
  - **agent:** executor3
  - **Dependências:** T022
  - **depends_on:** [T022]
  - **Paralelizável:** true
  - **Arquivos:** `llms.txt`
  - **Wiki-Keywords:** llms.txt, onboarding

- [ ] **T024:** Atualizar o .env.example com `AI_MEMORY_BACKUP_DIR`
  - **Papel:** executor
  - **agent:** executor1
  - **Dependências:** T003
  - **depends_on:** [T003]
  - **Paralelizável:** true
  - **Arquivos:** `.env.example`
  - **Wiki-Keywords:** .env.example, AI_MEMORY_BACKUP_DIR



## Coordination Graph

G₀ (round 0):

- **nodes:** T001, T002, T003, T004, T005, T006, T007, T008, T009, T010, T011, T012, T013, T014, T015, T016, T017, T018, T019, T020, T021, T022, T023, T024
- **edges:**
  - T001 → T002
  - T001 → T003
  - T001 → T004
  - T001 → T005
  - T003 → T006
  - T004 → T007
  - T005 → T007
  - T004 → T008
  - T006 → T009
  - T007 → T009
  - T008 → T009
  - T004 → T010
  - T004 → T011
  - T010 → T011
  - T011 → T012
  - T012 → T013
  - T013 → T014
  - T006 → T015
  - T014 → T016
  - T009 → T017
  - T002 → T017
  - T009 → T018
  - T002 → T018
  - T016 → T019
  - T002 → T019
  - T017 → T020
  - T018 → T020
  - T019 → T020
  - T015 → T020
  - T020 → T021
  - T021 → T022
  - T022 → T023
  - T003 → T024
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
  - T017: executor1
  - T018: executor2
  - T019: executor3
  - T020: executor1
  - T021: analyst1
  - T022: executor2
  - T023: executor3
  - T024: executor1
- **ready:** [T001]

> Workers: Lead, analyst1, executor1..3. Validado por script: 24 tasks, sem ciclo, sem dependência quebrada, sem arquivo compartilhado entre tasks sem ordem. Mínimo de 13 rounds. Caminho crítico (calculado): T001 → T004 → T010 → T011 → T012 → T013 → T014 → T016 → T019 → T020 → T021 → T022 → T023.
