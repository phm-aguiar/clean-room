# Tasks: Captura e acesso (hooks, sessões, handoffs, MCP e CLI)

> Fonte: `spec.md` (Aprovado: true), `plan.md` (ADR-009 a ADR-016), `acceptance/captura.feature` (12 cenários), notas em `notes/`.
> Pré-requisito: todas as tasks da 001 concluídas ("Nenhuma" abaixo significa sem dependência dentro da 002).
> `docs/` vence este arquivo. Em dúvida, comportamento do original. Leia `.github/memory/constitution.md` antes de cada task.

## Fase 1: Fundação

- [ ] **T001:** Estender `TestImportGraph` com `ingest`, `mcp`, `hookclient` e `osproc` (`mcp` não importa `ingest` em produção)
  - **Papel:** executor
  - **Dependências:** Nenhuma
  - **Paralelizável:** true
  - **Arquivos:** `internal/archtest/imports_test.go`
  - **Wiki-Keywords:** grafo de dependências, mcp sem ingest, cli compõe

- [ ] **T002:** Implementar os tipos de evento canônicos com grafias aceitas (kebab, snake, Pascal), importância e teto de corpo por tipo (16 KiB / 2 KB, `docs/lifecycle-event-ingestion.md` §3.1, §3.2, §3.6)
  - **Papel:** executor
  - **Dependências:** Nenhuma
  - **Paralelizável:** true
  - **Arquivos:** `internal/domain/event.go`
  - **Wiki-Keywords:** tipos de evento, importância, teto de corpo

- [ ] **T003:** Implementar a chave de ingestão e a chave do ator (identidade + id de sessão na ordem de `docs/mcp-server.md` §2.1)
  - **Papel:** executor
  - **Dependências:** Nenhuma
  - **Paralelizável:** true
  - **Arquivos:** `internal/domain/ingestkey.go`, `internal/domain/actor.go`
  - **Wiki-Keywords:** chave de ingestão, chave do ator, slot compartilhado

- [ ] **T004:** Implementar estados e tipos de handoff (`open`/`accepted`/`expired`; manual/automático; dono opcional)
  - **Papel:** executor
  - **Dependências:** Nenhuma
  - **Paralelizável:** true
  - **Arquivos:** `internal/domain/handoff.go`
  - **Wiki-Keywords:** handoff, estados, dono

- [ ] **T005:** Implementar as decisões de captura (Keep/Drop/MetadataOnly) e o corpo substituto de MetadataOnly (`notes/capture-contract.md` §6–§7)
  - **Papel:** executor
  - **Dependências:** Nenhuma
  - **Paralelizável:** true
  - **Arquivos:** `internal/domain/capture.go`
  - **Wiki-Keywords:** política de captura, MetadataOnly, corpo substituto

- [ ] **T006:** Implementar ADR-012: trava exclusiva de arquivo (`LockFileEx` via `syscall` no Windows, `flock` no Unix)
  - **Papel:** executor
  - **Dependências:** Nenhuma
  - **Paralelizável:** true
  - **Arquivos:** `internal/osproc/lock.go`, `internal/osproc/lock_windows.go`, `internal/osproc/lock_unix.go`
  - **Wiki-Keywords:** LockFileEx, flock, trava do SO

- [ ] **T007:** Implementar ADR-012: enumeração de processos `ai-memory` vivos (`ai-memory`/`ai-memory.exe` sem diferenciar maiúsculas no Windows; `/proc` no Linux)
  - **Papel:** executor
  - **Dependências:** Nenhuma
  - **Paralelizável:** true
  - **Arquivos:** `internal/osproc/procs.go`, `internal/osproc/procs_windows.go`, `internal/osproc/procs_linux.go`
  - **Wiki-Keywords:** INV-09, processo vivo, CreateToolhelp32Snapshot

- [ ] **T008:** Implementar no storage a gravação da observação com a chave de ingestão na mesma transação (replay converge, `occurred_at` vira `created_at`)
  - **Papel:** executor
  - **Dependências:** T002, T003
  - **Paralelizável:** true
  - **Arquivos:** `internal/storage/observations.go`
  - **Wiki-Keywords:** idempotência, chave de ingestão, created_at

- [ ] **T009:** Implementar no storage as sessões (abrir, registrar atividade, encerrar, detectar sessão efêmera)
  - **Papel:** executor
  - **Dependências:** T002
  - **Paralelizável:** true
  - **Arquivos:** `internal/storage/sessions.go`
  - **Wiki-Keywords:** sessão, sessão efêmera, busy_since

- [ ] **T010:** Implementar ADR-013 no storage: handoffs com seleção por prioridade (`notes/handoff-contract.md` §2) e aceite por `UPDATE` condicional
  - **Papel:** executor
  - **Dependências:** T004
  - **Paralelizável:** true
  - **Arquivos:** `internal/storage/handoffs.go`
  - **Wiki-Keywords:** handoff exatamente uma vez, duas guardas, INV-16

- [ ] **T011:** Implementar ADR-015: ponteiro de projeto ativo por ator (Unset/Match/Mismatch, slot compartilhado, sem teto de chaves)
  - **Papel:** executor
  - **Dependências:** T003
  - **Paralelizável:** true
  - **Arquivos:** `internal/storage/activeproject.go`
  - **Wiki-Keywords:** ponteiro de projeto ativo, Mismatch, auto-scope

- [ ] **T012:** Acrescentar à configuração as chaves de `[ingest]`, `[handoff]`, `[auth]` e do cliente de hook (timeout 2 s, URL do servidor)
  - **Papel:** executor
  - **Dependências:** Nenhuma
  - **Paralelizável:** true
  - **Arquivos:** `internal/config/ingest.go`
  - **Wiki-Keywords:** drop_tool_names, claim_on_session_start, bearer_token_file

## Fase 2: Implementação

- [ ] **T013:** Implementar na wiki o ledger mensal `log-AAAA-MM.md` (só acréscimo, mês UTC; `log.md` nunca recebe linhas)
  - **Papel:** executor
  - **Dependências:** T002
  - **Paralelizável:** true
  - **Arquivos:** `internal/wiki/ledger.go`
  - **Wiki-Keywords:** ledger, log mensal, INV-10

- [ ] **T014:** Implementar na wiki o commit automático com os rótulos de LEI (`session <id8>: <título ≤ 60>`, `lifecycle-only session <id8>`, checkpoint)
  - **Papel:** executor
  - **Dependências:** Nenhuma
  - **Paralelizável:** true
  - **Arquivos:** `internal/wiki/autocommit.go`
  - **Wiki-Keywords:** commit automático, go-git, rótulos

- [ ] **T015:** Implementar o parsing tolerante do payload, `occurred_at` (positivo e ≤ 5 min à frente) e a remoção de `last_assistant_message`
  - **Papel:** executor
  - **Dependências:** T002
  - **Paralelizável:** true
  - **Arquivos:** `internal/ingest/payload.go`
  - **Wiki-Keywords:** payload do hook, occurred_at, last_assistant_message

- [ ] **T016:** Implementar a ordem de admissão do `/hook` (aliases, lista de descarte `[ingest]` → `dropped_policy`, subagente, sanitização antes do teto)
  - **Papel:** executor
  - **Dependências:** T005, T015
  - **Paralelizável:** true
  - **Arquivos:** `internal/ingest/admit.go`
  - **Wiki-Keywords:** admissão, ingest drop list, INV-06

- [ ] **T017:** Implementar a resolução de projeto do hook (cria workspace e projeto, recusa `_global`, nunca grava diretório não-raiz git como caminho de repositório)
  - **Papel:** executor
  - **Dependências:** T011
  - **Paralelizável:** true
  - **Arquivos:** `internal/ingest/scope.go`
  - **Wiki-Keywords:** resolução de projeto, _global, auto-scope

- [ ] **T018:** Implementar ADR-010: `POST /hook` com semáforo não bloqueante de 1024 (202/429) e corpo ≤ 10 MiB
  - **Papel:** executor
  - **Dependências:** T008, T016, T017
  - **Paralelizável:** true
  - **Arquivos:** `internal/ingest/hook.go`
  - **Wiki-Keywords:** INV-05, 202, 429

- [ ] **T019:** Implementar `POST /hook/batch` (≤ 256 itens, 413 acima; `outcome`, `accepted_indices`, `failed_index` com regras de omissão)
  - **Papel:** executor
  - **Dependências:** T018
  - **Paralelizável:** true
  - **Arquivos:** `internal/ingest/batch.go`
  - **Wiki-Keywords:** lote, 413, accepted_indices

- [ ] **T020:** Implementar ADR-014: sintetizador por regras de `sessions/<id>.md` e do handoff automático (LEI §9–§10)
  - **Papel:** executor
  - **Dependências:** T009, T010
  - **Paralelizável:** true
  - **Arquivos:** `internal/ingest/synth.go`
  - **Wiki-Keywords:** página de sessão, resumo, próximos passos

- [ ] **T021:** Implementar o processamento do `session-end` (encerra sessão, página e handoff numa transação, commit; efêmera reabre o handoff aceito)
  - **Papel:** executor
  - **Dependências:** T013, T014, T020
  - **Paralelizável:** true
  - **Arquivos:** `internal/ingest/sessionend.go`
  - **Wiki-Keywords:** session-end, sessão efêmera, reabrir handoff

- [ ] **T022:** Implementar `GET /handoff` (seleção, reivindicação ou nota de oferta; falha = 200 vazio; recusa = 403)
  - **Papel:** executor
  - **Dependências:** T010, T011
  - **Paralelizável:** true
  - **Arquivos:** `internal/ingest/handoff.go`
  - **Wiki-Keywords:** GET /handoff, nota de oferta, claim_on_session_start

- [ ] **T023:** Implementar `GET /recap` (recapitulação somente leitura da última compactação)
  - **Papel:** executor
  - **Dependências:** T009
  - **Paralelizável:** true
  - **Arquivos:** `internal/ingest/recap.go`
  - **Wiki-Keywords:** recap, reload context, compactação

- [ ] **T024:** Implementar ADR-009: adaptador MCP sobre `mcp-go` (stdio, HTTP streamable, pino `2024-11-05`, ator pelo contexto, atividade do cliente)
  - **Papel:** executor
  - **Dependências:** T003
  - **Paralelizável:** true
  - **Arquivos:** `internal/mcp/server.go`, `internal/mcp/adapter.go`
  - **Wiki-Keywords:** mcp-go, protocolo 2024-11-05, interface MCP

- [ ] **T025:** Implementar as tools de leitura `memory_query`, `memory_read_page`, `memory_recent` e `memory_read_session_observations`
  - **Papel:** executor
  - **Dependências:** T024
  - **Paralelizável:** true
  - **Arquivos:** `internal/mcp/tools_read.go`
  - **Wiki-Keywords:** memory_query, falha fechado, resolved_by

- [ ] **T026:** Implementar as tools `memory_status` e `memory_briefing`
  - **Papel:** executor
  - **Dependências:** T024
  - **Paralelizável:** true
  - **Arquivos:** `internal/mcp/tools_status.go`
  - **Wiki-Keywords:** memory_status, memory_briefing, ScopeSource

- [ ] **T027:** Implementar as tools de escrita `memory_write_page` (Unset → `default/scratch`, Mismatch → `-32602 ambiguous unscoped write`) e `memory_delete_page`
  - **Papel:** executor
  - **Dependências:** T024, T011
  - **Paralelizável:** true
  - **Arquivos:** `internal/mcp/tools_write.go`
  - **Wiki-Keywords:** memory_write_page, escrita ambígua, frontmatter

- [ ] **T028:** Implementar as tools `memory_handoff_begin`, `_accept`, `_list` e `_cancel` (perdedor recebe `handoff: null`)
  - **Papel:** executor
  - **Dependências:** T024, T010
  - **Paralelizável:** true
  - **Arquivos:** `internal/mcp/tools_handoff.go`
  - **Wiki-Keywords:** memory_handoff_accept, handoff null, cancel

- [ ] **T029:** Implementar o leitor do marcador `.ai-memory.toml` (scope walk e capture walk, fronteiras, 64 KiB, leitura por linha e TOML estrito)
  - **Papel:** executor
  - **Dependências:** T005
  - **Paralelizável:** true
  - **Arquivos:** `internal/hookclient/marker.go`
  - **Wiki-Keywords:** marker-file, scope walk, capture walk

- [ ] **T030:** Implementar a política de captura (padrões, normalização no Windows, allowlist/denylist, limites)
  - **Papel:** executor
  - **Dependências:** T029
  - **Paralelizável:** true
  - **Arquivos:** `internal/hookclient/policy.go`
  - **Wiki-Keywords:** capture exclusions, ignore_paths, allowlist

- [ ] **T031:** Implementar ADR-011: spool (nome do arquivo, 10 000 arquivos descartando o evento novo, trava de capacidade com espera de 50 ms, 7 dias)
  - **Papel:** executor
  - **Dependências:** T006
  - **Paralelizável:** true
  - **Arquivos:** `internal/hookclient/spool.go`
  - **Wiki-Keywords:** spool, capacity.lock, descarte do evento novo

- [ ] **T032:** Implementar o envio HTTP do hook (timeout 2 s, 429 mantém no spool) e a drenagem com `.drain.lock`
  - **Papel:** executor
  - **Dependências:** T031
  - **Paralelizável:** true
  - **Arquivos:** `internal/hookclient/send.go`, `internal/hookclient/drain.go`
  - **Wiki-Keywords:** hook-drain, drain.lock, timeout

- [ ] **T033:** Implementar o middleware Bearer (`crypto/subtle`, 401 texto simples com `WWW-Authenticate`, `/healthz` livre, recusa de bind fora de loopback sem auth)
  - **Papel:** executor
  - **Dependências:** T012
  - **Paralelizável:** true
  - **Arquivos:** `internal/cli/auth.go`
  - **Wiki-Keywords:** Bearer, tempo constante, auth required

- [ ] **T034:** Implementar `ai-memory serve` (`.serve.lock` + `.serve.lock.holder`, rotas, MCP stdio e HTTP)
  - **Papel:** executor
  - **Dependências:** T006, T018, T019, T021, T022, T023, T025, T026, T027, T028, T033
  - **Paralelizável:** false
  - **Arquivos:** `internal/cli/serve.go`
  - **Wiki-Keywords:** serve, instância única, INV-09

- [ ] **T035:** Implementar `ai-memory hook` e `hook-drain` oculto (sempre código 0; `session-end` drenado no próprio processo)
  - **Papel:** executor
  - **Dependências:** T030, T032, T034
  - **Paralelizável:** false
  - **Arquivos:** `internal/cli/hook.go`
  - **Wiki-Keywords:** ai-memory hook, nunca bloqueia, session-end

- [ ] **T036:** Implementar ADR-016: `install-hooks --agent claude-code` (Windows nativo com `command` + `args`, `"timeout": 15` no SessionEnd) e a mesclagem de JSON
  - **Papel:** executor
  - **Dependências:** Nenhuma
  - **Paralelizável:** true
  - **Arquivos:** `internal/cli/installhooks.go`, `internal/cli/jsonmerge.go`
  - **Wiki-Keywords:** install-hooks, settings.json, idempotência

- [ ] **T037:** Implementar `install-mcp --client claude-code` (`~/.claude.json`)
  - **Papel:** executor
  - **Dependências:** T036
  - **Paralelizável:** true
  - **Arquivos:** `internal/cli/installmcp.go`
  - **Wiki-Keywords:** install-mcp, claude.json, mesclagem

- [ ] **T038:** Implementar `ai-memory status`
  - **Papel:** executor
  - **Dependências:** T035
  - **Paralelizável:** false
  - **Arquivos:** `internal/cli/status.go`
  - **Wiki-Keywords:** status, healthz

- [ ] **T039:** Implementar a guarda INV-09 por enumeração de processos em `reset`, `restore`, `reindex` e `uninstall --purge-data`
  - **Papel:** executor
  - **Dependências:** T007
  - **Paralelizável:** true
  - **Arquivos:** `internal/cli/guard.go`
  - **Wiki-Keywords:** INV-09, guarda de processo vivo, reindex

## Fase 3: Validação

- [ ] **T040:** Versionar as fixtures de payload do Claude Code (`notes/hook-contract.md` §11)
  - **Papel:** executor
  - **Dependências:** T015
  - **Paralelizável:** true
  - **Arquivos:** `internal/ingest/testdata/claude-code/*.json`
  - **Wiki-Keywords:** fixtures de payload, claude-code

- [ ] **T041:** Testar o cenário "Captura de uma sessão e entrega ao agente seguinte" ponta a ponta
  - **Papel:** executor
  - **Dependências:** T034, T035, T040
  - **Paralelizável:** true
  - **Arquivos:** `internal/cli/e2e_test.go`
  - **Wiki-Keywords:** happy path, handoff, memory_query

- [ ] **T042:** Testar o tratamento de erro "Servidor saturado responde 429" com 1025 requisições em voo e lote de 257 itens → 413
  - **Papel:** executor
  - **Dependências:** T019
  - **Paralelizável:** true
  - **Arquivos:** `internal/ingest/saturation_test.go`
  - **Wiki-Keywords:** 429, carga, 413

- [ ] **T043:** Testar "Leitura com escopo explícito inexistente falha fechada, e leitura sem escopo degrada"
  - **Papel:** executor
  - **Dependências:** T025
  - **Paralelizável:** true
  - **Arquivos:** `internal/mcp/scope_test.go`
  - **Wiki-Keywords:** falha fechado, degradação, resolved_by

- [ ] **T044:** Testar "Escrita sem escopo com ponteiro ativo divergente é recusada como ambígua" (e Unset → `default/scratch`)
  - **Papel:** executor
  - **Dependências:** T027
  - **Paralelizável:** true
  - **Arquivos:** `internal/mcp/write_test.go`
  - **Wiki-Keywords:** Mismatch, -32602, default/scratch

- [ ] **T045:** Testar "Falha no banco após escrever o arquivo reverte o arquivo"
  - **Papel:** executor
  - **Dependências:** T027
  - **Paralelizável:** true
  - **Arquivos:** `internal/mcp/write_rollback_test.go`
  - **Wiki-Keywords:** rollback, memory_write_page

- [ ] **T046:** Testar "Servidor offline mantém o evento no spool até a drenagem" (7 dias, spool cheio descarta o novo)
  - **Papel:** executor
  - **Dependências:** T032
  - **Paralelizável:** true
  - **Arquivos:** `internal/hookclient/spool_test.go`
  - **Wiki-Keywords:** spool, drenagem, servidor offline

- [ ] **T047:** Testar "Replay do mesmo evento não duplica a observação", inclusive interrompido no `session-end`
  - **Papel:** executor
  - **Dependências:** T021
  - **Paralelizável:** true
  - **Arquivos:** `internal/ingest/replay_test.go`
  - **Wiki-Keywords:** replay, idempotência, convergência

- [ ] **T048:** Testar multi-sessão "Handoff disputado tem exatamente um vencedor" com `-race` (GET /handoff e MCP)
  - **Papel:** executor
  - **Dependências:** T022, T028
  - **Paralelizável:** true
  - **Arquivos:** `internal/ingest/handoff_race_test.go`
  - **Wiki-Keywords:** INV-16, handoff disputado, race

- [ ] **T049:** Testar multi-sessão "Escritas concorrentes na mesma página formam cadeia de supersessão" via MCP
  - **Papel:** executor
  - **Dependências:** T027
  - **Paralelizável:** true
  - **Arquivos:** `internal/mcp/multisession_test.go`
  - **Wiki-Keywords:** supersessão, INV-16, multi-sessão

- [ ] **T050:** Testar "Segredo no payload do hook é removido antes de gravar e os campos são limitados" e as regras de `occurred_at`
  - **Papel:** executor
  - **Dependências:** T016
  - **Paralelizável:** true
  - **Arquivos:** `internal/ingest/admit_test.go`
  - **Wiki-Keywords:** sanitizador, teto por tipo, occurred_at

- [ ] **T051:** Testar "Resposta bruta do assistente nunca é persistida nem transmitida" (log, spool, rede e banco)
  - **Papel:** executor
  - **Dependências:** T035
  - **Paralelizável:** true
  - **Arquivos:** `internal/cli/assistant_strip_test.go`
  - **Wiki-Keywords:** last_assistant_message, privacidade

- [ ] **T052:** Testar multi-sessão "Dois agentes no mesmo projeto não sobrescrevem o ponteiro um do outro"
  - **Papel:** executor
  - **Dependências:** T011, T018
  - **Paralelizável:** true
  - **Arquivos:** `internal/ingest/actor_pointer_test.go`
  - **Wiki-Keywords:** ponteiro por ator, INV-16

- [ ] **T053:** Testar a política de captura contra `docs/assets/fixtures/capture-policy.json` e `home_route_cases.json`
  - **Papel:** executor
  - **Dependências:** T030
  - **Paralelizável:** true
  - **Arquivos:** `internal/hookclient/policy_test.go`
  - **Wiki-Keywords:** capture-policy.json, vetores, home routes

- [ ] **T054:** Testar a instância única (segundo `serve` recusa) e a guarda de `reindex` com e sem outro processo vivo
  - **Papel:** executor
  - **Dependências:** T034, T039
  - **Paralelizável:** true
  - **Arquivos:** `internal/cli/singleinstance_test.go`
  - **Wiki-Keywords:** serve.lock, INV-09, reindex

- [ ] **T055:** Testar a idempotência de `install-hooks` e `install-mcp` (segunda aplicação não toca o disco; entradas de terceiros preservadas)
  - **Papel:** executor
  - **Dependências:** T037
  - **Paralelizável:** true
  - **Arquivos:** `internal/cli/install_test.go`
  - **Wiki-Keywords:** install-hooks, idempotência, terceiros

- [ ] **T056:** Testar as 12 tools por stdio e HTTP com Bearer e o `initialize` respondendo `2024-11-05`
  - **Papel:** executor
  - **Dependências:** T034
  - **Paralelizável:** true
  - **Arquivos:** `internal/cli/mcp_transport_test.go`
  - **Wiki-Keywords:** 12 tools, stdio, HTTP streamable

- [ ] **T057:** Criar os testes de contrato `TestINV05_…`, `TestINV09_…` e `TestINV16_…`
  - **Papel:** executor
  - **Dependências:** T042, T048, T052, T054
  - **Paralelizável:** true
  - **Arquivos:** `internal/cli/inv002_contract_test.go`
  - **Wiki-Keywords:** invariantes, teste de contrato

- [ ] **T058:** Rodar o portão final: build cruzado `CGO_ENABLED=0` (windows/linux), `go vet`, `go test -race`, `golangci-lint`, `gofmt -l`
  - **Papel:** executor
  - **Dependências:** T001, T041, T042, T043, T044, T045, T046, T047, T048, T049, T050, T051, T052, T053, T054, T055, T056, T057
  - **Paralelizável:** false
  - **Arquivos:** —
  - **Wiki-Keywords:** portões de qualidade, race, lint

## Fase 4: Documentação

- [ ] **T059:** Atualizar o Status do `spec.md` (plano e tasks existentes)
  - **Papel:** executor
  - **Dependências:** T058
  - **Paralelizável:** false
  - **Arquivos:** `specs/features/002-captura-e-acesso/spec.md`
  - **Wiki-Keywords:** status da spec
