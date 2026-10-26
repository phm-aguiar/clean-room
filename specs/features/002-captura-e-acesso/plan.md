# Plano: Captura e acesso (hooks, sessões, handoffs, MCP e CLI)

## Metadados
- **Feature:** 002 — `specs/features/002-captura-e-acesso/spec.md` (Aprovado: true)
- **Aceitação:** `specs/features/002-captura-e-acesso/acceptance/captura.feature`
- **Notas de contrato:** `notes/hook-contract.md`, `notes/capture-contract.md`, `notes/handoff-contract.md`, `notes/mcp-contract.md`
- **Depende de:** 001 (domínio, `storage` com escritor único e resolvedor de escopo, `wiki` com escrita atômica e Git)
- **Stack:** Go 1.26, módulo único, `CGO_ENABLED=0`; acrescenta `cobra` e `mark3labs/mcp-go` (ambos em `.github/memory/tech.md`) ao que a 001 já usa; HTTP, travas de arquivo e enumeração de processos pela stdlib
- **Alvos:** `windows/amd64` (primeiro MVP) e `linux/amd64`
- **Escopo:** captura zero-LLM de eventos do Claude Code (hook CLI com spool, `/hook`, `/hook/batch`, `GET /handoff`, `GET /recap`), ledger mensal, página de sessão por regras com commit, handoffs exatamente uma vez, servidor MCP com 12 tools (stdio e HTTP), CLI `serve`/`hook`/`hook-drain`/`install-hooks`/`install-mcp`/`status`, instância única (`.serve.lock`) e guarda de processo vivo (INV-09). Fora: LLM, multiusuário, admissão por webhook, outros agentes, workstreams, mensagens, web.
- **Precedência:** `docs/` > notas > este plano. Em dúvida, comportamento do original, inclusive defeitos conhecidos.

## Contratos e Fronteiras

### Pacotes e grafo de imports (estende `TestImportGraph` da 001)
| Pacote | Responsabilidade | Pode importar |
|---|---|---|
| `internal/domain` | + tipos de evento canônicos, chave de ingestão, estados de handoff, chave do ator, decisões de captura (Keep/Drop/MetadataOnly) | stdlib, `google/uuid`, `x/text` |
| `internal/storage` | + observações com chave de ingestão na mesma transação, sessões, handoffs (aceite condicional), ponteiro de projeto ativo por ator | `domain` |
| `internal/wiki` | + página `sessions/<id>.md`, ledger `log-AAAA-MM.md` (só acréscimo), commit automático de fim de sessão | `domain`, `storage` |
| `internal/ingest` | Roteador `/hook`, `/hook/batch`, `/handoff`, `/recap`; admissão (ordem de LEI), semáforo de 1024, lista de descarte `[ingest]`, `occurred_at`, sintetizador de sessão e handoff automático | `domain`, `storage`, `wiki` |
| `internal/mcp` | Interface própria sobre o SDK, pino de protocolo `2024-11-05`, 12 tools, chave do ator a partir do contexto | `domain`, `storage`, `wiki` (**não** `ingest`, salvo em testes) |
| `internal/hookclient` | Lado cliente do `ai-memory hook`: leitura do marcador `.ai-memory.toml`, política de captura, spool, drenagem, envio HTTP | `domain` |
| `internal/osproc` | Trava exclusiva de arquivo (`LockFileEx`/`flock`) e enumeração de processos `ai-memory` | stdlib |
| `internal/cli` | Único compositor: comandos cobra, middleware Bearer, montagem do `http.Server` | todos |

### APIs HTTP (contrato literal em `docs/api-reference.md` e `docs/lifecycle-event-ingestion.md`)
| Rota | Comportamento | Fonte |
|---|---|---|
| `POST /hook?event=&agent=` | 202 ao enfileirar (inclusive `dropped_policy` e `ingest drop list`); 429 com 1024 em voo; corpo ≤ 10 MiB | LEI §2, §4.6; INV-05 |
| `POST /hook/batch` | ≤ 256 itens, 413 acima; itens processados dentro da requisição; `outcome`, `accepted_indices`, `failed_index` com as regras de omissão do doc | LEI §2; auditoria 2026-10-25 |
| `GET /handoff` | Seleção + reivindicação (ou nota de oferta); falha de leitura = 200 vazio; recusa = 403 com motivo | `notes/handoff-contract.md` §3.1 |
| `GET /recap` | Recapitulação somente leitura da última compactação | LEI §2.5; `docs/cli.md` §5.2 passo 9a |
| `POST /mcp` | MCP HTTP streamable sem estado, resposta JSON; `--http-stateful` opcional | `docs/mcp-server.md` |
| `GET /healthz` | Única rota sem bearer quando a autenticação está ativa | `docs/users.md` |
| 401 | Texto simples `auth required` + `WWW-Authenticate: Bearer realm="ai-memory", error="invalid_token"` | auditoria 2026-10-25 |

### MCP
- 12 tools com schemas, aliases de parâmetros e erros de `notes/mcp-contract.md` §3 e `docs/mcp-server.md` §3; `initialize` responde sempre `2024-11-05`, nome `ai-memory`, capacidade `tools`, instruções de uso.
- Chave do ator = (identidade, id de sessão) na ordem de `docs/mcp-server.md` §2.1; a mesma função é usada pelo roteador de hooks.
- Escrita sem escopo: `Unset` → `default/scratch`; `Mismatch` → `-32602 ambiguous unscoped write`.

### Arquivos e formatos
- Spool: `<data_dir>/hook-spool/{created_ms:013}-{pid}-{seq:016x}-{recovery_key}.json`; trava `.drain.lock`; trava de capacidade `<data_dir>/hook-spool..capacity.lock` (nome com os dois pontos, como no original); 10 000 arquivos; 7 dias; espera de 50 ms que nunca descarta.
- Instância única: `<data_dir>/.serve.lock` + `.serve.lock.holder` (PID informativo). Sem arquivo de PID.
- Instalação: hooks em `~/.claude/settings.json` (no Windows nativo, `command` + `args` e `"timeout": 15` no `SessionEnd`); MCP em `~/.claude.json`; mesclagem que preserva entradas de terceiros e não toca o disco quando nada muda.
- Wiki: `sessions/<id>.md` (tier episodic) e `log-AAAA-MM.md` (mês UTC); commit `session <id8>: <título ≤ 60>`, `lifecycle-only session <id8>` e rótulos de checkpoint de LEI.

### Persistência
Sem migração nova: `occurred_at` é campo de entrada que vira `created_at` (`docs/database-schema.md`). Tabelas de observação, chave de ingestão, sessão e handoff são as da 001/`docs/database-schema.md`.

### Erros e textos
Textos literais de `docs/mcp-server.md`, `docs/lifecycle-event-ingestion.md`, `docs/cli.md` e das notas (ex.: "ago ago" da nota de oferta, `server accepted <n> of 1 synthetic session-end events`), testados por igualdade de string e contra `docs/assets/fixtures/` (incl. `capture-policy.json` e `home_route_cases.json`).

## ADRs

- **ADR-009 — MCP atrás de interface própria sobre `mark3labs/mcp-go`, com pino de protocolo por wrapper**
  - **Decision:** o pacote `mcp` define o registro de tools e os handlers em tipos próprios; um adaptador fino usa `mcp-go` para stdio e HTTP streamable e intercepta `initialize` para devolver sempre `2024-11-05`. A identidade chega pelo `context` preenchido no middleware de autenticação.
  - **Rationale:** contrato observável fixo (constituição, `docs/mcp-server.md` §1) sem escrever JSON-RPC; troca de SDK fica restrita ao adaptador.
  - **Alternative Rejected:** JSON-RPC próprio — mais código e manutenção de protocolo (spec, Alternativas); usar o SDK direto nos handlers — acopla o contrato à versão negociada pelo SDK.

- **ADR-010 — Ingestão com semáforo não bloqueante e idempotência na transação da observação**
  - **Decision:** `ingest` usa um semáforo de capacidade 1024 com `select`/`default` (cheio → 429 imediato); cada evento admitido vira **um** comando no escritor único que grava chave de ingestão + observação + efeitos derivados juntos; replay encontra a chave e converge sem nova linha.
  - **Rationale:** INV-05 (202/429 sem bloquear), INV-02/03 (uma transação) e regra transversal de idempotência at-least-once.
  - **Alternative Rejected:** fila em memória com worker — aceitaria 202 antes de gravar a chave e perderia eventos no encerramento; deduplicação em tabela separada fora da transação — janela de duplicação.

- **ADR-011 — Hook CLI e servidor no mesmo binário; spool em arquivo com travas do SO**
  - **Decision:** `ai-memory hook` grava o evento no spool (após política de captura e remoção de `last_assistant_message`), tenta POST com timeout de 2 s e sai com código 0; fronteiras disparam `hook-drain` (oculto) desacoplado; `session-end` drena no próprio processo com o orçamento do doc. Travas via `LockFileEx` (Windows, por `syscall` da stdlib) e `flock` (Unix).
  - **Rationale:** nunca bloquear o agente, sobreviver a servidor offline e reproduzir nomes e limites do spool; binário único conforme a constituição.
  - **Alternative Rejected:** dois binários (cold start menor, sem evidência de necessidade — spec); envio direto sem spool (perde evento com servidor offline); trava por arquivo de PID (não é a do original e falha com processo morto).

- **ADR-012 — Travas e enumeração de processos só com a stdlib, em `internal/osproc`**
  - **Decision:** `LockFileEx`/`UnlockFileEx` por `syscall.NewLazyDLL("kernel32.dll")`, `flock` por `syscall.Flock`; enumeração com `CreateToolhelp32Snapshot`/`Process32First/Next` no Windows (comparação `ai-memory`/`ai-memory.exe` sem diferenciar maiúsculas) e `/proc/*/comm`+`exe` no Linux; build tags por SO.
  - **Rationale:** `golang.org/x/sys` não está em `tech.md`; a stdlib cobre o necessário sem CGO.
  - **Alternative Rejected:** `golang.org/x/sys/windows` — dependência nova fora de `tech.md`; `gopsutil` — idem e muito maior.

- **ADR-013 — Handoff exatamente uma vez por duas guardas e `UPDATE` condicional**
  - **Decision:** seleção pela ordem de `notes/handoff-contract.md` §2; reivindicação = (1) leitura de metadados exige `state='open'`; (2) `UPDATE … SET state='accepted' … WHERE id=? AND state='open'` no escritor único, vencedor = 1 linha afetada; perdedor recebe `handoff: null`. Sessão efêmera reabre o handoff aceito por ela na mesma transação do encerramento.
  - **Rationale:** INV-16 e `docs/design-handoff-offer-not-claim.md` §7; o escritor único serializa e o `WHERE` garante a segunda guarda mesmo com leituras concorrentes.
  - **Alternative Rejected:** trava em memória por handoff — não sobrevive a reinício nem cobre o caminho do `GET /handoff` e do MCP juntos.

- **ADR-014 — Sessão sintetizada por regras numa só transação com o handoff automático**
  - **Decision:** no `session-end`, `ingest` monta `sessions/<id>.md` (regras de LEI §9–§10), chama `wiki.WritePage` e, no mesmo comando do escritor, encerra a sessão e grava o handoff; depois um único commit git. Interrupção é recuperada pelo replay da chave (sessão já encerrada → só o commit).
  - **Rationale:** zero-LLM (INV-13), convergência at-least-once e a ordem de LEI §8.
  - **Alternative Rejected:** handoff e página em transações separadas — estado parcial após queda (página sem handoff ou o contrário).

- **ADR-015 — Ponteiro de projeto ativo como mapa por ator em memória, sem teto de chaves**
  - **Decision:** mapa protegido por `sync.RWMutex` chaveado pela chave do ator, com slot compartilhado para chave vazia, estados `Unset`/`Match`/`Mismatch` e semente de startup da 001; sem limite de tamanho, como no original.
  - **Rationale:** INV-16 (um ator não sobrescreve o outro) e `docs/auto-scope.md` (auditoria: reproduzir o original sem teto).
  - **Alternative Rejected:** persistir no SQLite — o original é em memória; LRU com teto — "melhoraria" o original.

- **ADR-016 — Instaladores com mesclagem de JSON preservando ordem e idempotência**
  - **Decision:** `install-hooks`/`install-mcp` leem o JSON com `json.Decoder` preservando chaves desconhecidas, alteram só as entradas do ai-memory, comparam bytes e só gravam (de forma atômica) quando há diferença; `--apply` necessário para gravar.
  - **Rationale:** critério de sucesso (segunda aplicação não toca o disco; entradas de terceiros preservadas); stdlib suficiente.
  - **Alternative Rejected:** `map[string]any` + `json.Marshal` — reordena chaves e reescreve o arquivo sempre.

## Auditoria da Constituição

### Portões de Qualidade
- [x] Build `CGO_ENABLED=0` + `go vet` em `windows/amd64` e `linux/amd64` — travas e processos pela stdlib com build tags (ADR-012).
- [x] `go test -race ./...` — teste de carga com 1025 requisições e aceite concorrente rodam com `-race`.
- [x] `golangci-lint run` — mesma configuração da 001.
- [x] Teste multi-sessão explícito: handoff disputado, ponteiro por ator com dois agentes, escrita concorrente na mesma página (INV-16).
- [x] Testes de contrato nomeados `TestINV05_…`, `TestINV09_…`, `TestINV16_…` (critério da spec).
- [x] Valores literais contra docs e `docs/assets/fixtures/` (`capture-policy.json`, `home_route_cases.json`, fixtures de payload versionadas).
- [x] Spec com `Aprovado: true` e `acceptance/captura.feature`.
- [x] Nenhuma dependência fora de `tech.md` (ADR-012 rejeita `x/sys` e `gopsutil`).

### Restrições Arquiteturais
- [x] Binário único; novos pacotes `ingest`, `mcp`, `hookclient`, `osproc`, `cli`.
- [x] Grafo de imports: `mcp` não importa `ingest`; `cli` único compositor; `TestImportGraph` estendido.
- [x] INV-01 — configuração só em `config`; exceções de leitura do ambiente do caminho do hook (`AI_MEMORY_HOOK_*`) conforme `docs/cli.md` §5.3, capturadas pela `cli` e passadas ao `hookclient`.
- [x] INV-02 — toda gravação de ingestão, handoff e sessão passa pelo escritor único (ADR-010/013/014).
- [x] INV-03 — FTS e índices da página de sessão na mesma transação.
- [x] INV-04 — observações, sessões e handoffs com `(workspace_id, project_id, …)`.
- [x] INV-05 — ADR-010; lote ≤ 256 com 413; timeout de 2 s do hook.
- [x] INV-06 — sanitização antes do teto por tipo (16 KiB / 2 KB, título ≤ 80); `last_assistant_message` removido antes de tudo; handoff manual e páginas do MCP sanitizados pelo chamador.
- [x] INV-09 — `.serve.lock` e guarda por enumeração de processos (ADR-012); `backup` online via `POST /admin/backup` (da 001).
- [x] INV-10 — escritas atômicas; ledger mensal só acréscimo; `log.md` nunca recebe linhas.
- [x] INV-11 — diretório de dados da 001.
- [x] INV-12 — reservada.
- [x] INV-13 — página de sessão e handoff por regras; `answer: true` → `answer_unavailable`.
- [x] INV-15 — logs de `ingest`/`mcp` pelo handler filtrado da 001.
- [x] INV-16 — ADR-013 e ADR-015; páginas sem filtro de autoria; supersessão da 001.
- [x] Resolução de escopo central; criam escopo só `memory_write_page`, `memory_handoff_begin` e o ingresso de hooks (recusa `_global`, nunca grava diretório que não é raiz git como caminho de repositório).
- [x] Autorização por capacidade e Bearer em tempo constante (`crypto/subtle`); recusa de bind fora de loopback sem auth.
- [x] Mutações da wiki só por `wiki` (sessão, ledger, `memory_write_page`, `memory_delete_page`).
- [x] Markdown em git como fonte da verdade; commit automático no fim de sessão.

### Regras de Negócio Transversais
- [x] Idempotência por chave de ingestão (ADR-010, ADR-014).
- [x] Falha no banco após o arquivo reverte o arquivo (`memory_write_page` via ADR-005 da 001).
- [x] Lápide de páginas sumidas segue desligada por padrão.
- [x] Fase zero-LLM; INV-07/08/14 não se aplicam.

### Preferências de Bibliotecas
- [x] `modernc.org/sqlite` (da 001).
- [x] MCP com `mark3labs/mcp-go` atrás de interface, pino `2024-11-05` (ADR-009).
- [x] `cobra` + `koanf`, sem `viper`.
- [x] UUIDv7 e `log/slog`.
- [x] Sanitizador da 001.
- [x] Git por `go-git/v5` (commit automático), fallback só no Windows.
- [x] Stdlib preferida: `net/http`, `encoding/json`, `syscall`, `crypto/subtle`.

### Anti-Padrões Proibidos
- [x] Sem leitura de ambiente fora de `config` (exceções do doc capturadas na `cli`).
- [x] Sem escrita de wiki fora de `wiki`.
- [x] Sem "indexar depois".
- [x] Sem filtro de leitura por autoria (dono filtra só handoffs).
- [x] Leitura, busca e `memory_delete_page` sem criar escopo.
- [x] Caminhos de página via `domain.PagePath`.
- [x] Sem `panic`; erros tipados com textos literais.
- [x] Sem abstração preemptiva: interface só sobre o SDK MCP (dois lados: adaptador e testes) e travas por SO.
- [x] Goroutines (drenagem, servidor, stdio) com `context`; concorrência limitada pelo semáforo de 1024.
- [x] Sem "melhorar" o original (ex.: descarta o evento novo com spool cheio; "ago ago"; sem teto no mapa de ponteiros).
- [x] "A confirmar" resolvido pelo original (spec, Correções da auditoria).
- [x] Clean-room: nada copiado além dos docs.

Legenda: `[x]` atendido pelo plano; `[~]` não se aplica a esta feature (justificado).
