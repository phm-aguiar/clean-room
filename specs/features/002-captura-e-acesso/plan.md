# plan.md: Plano de Implementação Técnica — Captura e acesso (002)

## 1. Metadados do Plano
- **Stack Tecnológico:** a da 001 (Go 1.26, `modernc.org/sqlite`, `git` do sistema, `koanf`, `cobra`, `slog`, `yaml.v3`, `fsnotify`, `google/uuid`) mais `github.com/mark3labs/mcp-go` (já registrado em `tech.md` como planejado; a versão é fixada na primeira task que o importa, depois de conferir que compila com Go 1.26 e `CGO_ENABLED=0`). O resto é stdlib: `net/http` (roteamento por padrões do `ServeMux`), `crypto/subtle` (comparação de token), `encoding/json`, `os/exec` (drenagem em segundo plano) e `syscall` (enumeração de processos e trava de arquivo). `CGO_ENABLED=0`, alvos `windows/amd64` e `linux/amd64`.
- **Feature Fonte:** `specs/features/002-captura-e-acesso/spec.md`
- **Escopo:** ingestão de hooks (`/hook`, `/hook/batch`, `/handoff`), cliente de hook com spool, síntese de sessão e handoff automático sem LLM, handoffs manuais, servidor MCP (stdio e HTTP) com 12 tools, CLI `serve`, `hook`, `hook-drain` (oculto), `install-hooks`, `install-mcp` e `status`, ponteiro de projeto ativo por ator, trava de instância única e guarda de processo vivo. Só Claude Code. Sem LLM, multiusuário, cadeia de admissão, workstreams, mensagens entre projetos, brief, perfil ou captura da mensagem do assistente.
- **Fontes de contrato:** `docs/lifecycle-event-ingestion.md`, `docs/design-handoff-offer-not-claim.md`, `docs/mcp-server.md`, `docs/cli.md`, `docs/marker-file.md`, `docs/mcp-install.md`, `docs/domain-vocabulary.md` §5 e §9, `docs/wiki-files-layer.md` §9, `docs/overview.md` (item 9) e `docs/security-boundaries.md` §10.

### Correções à spec encontradas ao ler os docs

A spec foi aprovada antes desta leitura. Os pontos abaixo divergiam do que os docs definem; o plano segue os docs (regra do autor: neste projeto o doc vence a spec). **As correções já foram aplicadas na spec**, que registra a lista completa (10 itens) na seção "Revisão contra os docs". Os seis que mudam a arquitetura:

| Spec diz | Doc define | Fonte |
|---|---|---|
| 9 tools, com uma `memory_handoff` de ações `begin/accept/list` | **12 tools**: o handoff são quatro tools, `memory_handoff_begin`, `memory_handoff_accept`, `memory_handoff_list` e `memory_handoff_cancel` | `docs/architecture.md` (tabela de tools), `docs/mcp-server.md` §3.2 e §3.3, `docs/design-handoff-offer-not-claim.md` §7 |
| `occurred_at` entra por migração V02 | `occurred_at` é campo de **entrada** da observação; o esquema não tem a coluna, e o instante vai para `created_at`. A 001 já faz isso. **Não há V02.** O que a 002 faz é ler `occurred_at` do topo do corpo do hook (positivo, no máximo 5 min à frente; senão "agora") | `docs/domain-vocabulary.md` §5.2, `docs/database-schema.md` (`observations`), `docs/lifecycle-event-ingestion.md` §3.3 |
| `ai-memory install` grava os hooks em `settings.json` | Dois comandos: `install-hooks --agent claude-code [--apply]` (hooks em `~/.claude/settings.json`) e `install-mcp --client claude-code [--apply]` (servidor em `~/.claude.json`, ou `$CLAUDE_CONFIG_DIR/.claude.json`; o `settings.json` não é lido para `mcpServers`) | `docs/cli.md` §8, `docs/mcp-install.md` (Claude Code) |
| `serve` grava `ai-memory.pid`; a guarda lê esse arquivo | `serve` segura a trava de instância única **`.serve.lock`** (com informação do detentor). A guarda de processo vivo **enumera os processos do sistema** e recusa se houver outro processo `ai-memory` vivo. O `ai-memory.pid` da 001 é substituído | `docs/cli.md` §2.5 e §3.1, `docs/overview.md` item 9, `docs/security-boundaries.md` §10 |
| "nenhuma rota de hook cria escopo fora do ponteiro ativo" | A resolução de projeto do hook **cria** workspace e projeto quando necessário (é caminho de escrita). Recusa só o nome reservado `_global` (captura nunca vai para o escopo global) e nunca grava como caminho de repositório um diretório que não é raiz git | `docs/lifecycle-event-ingestion.md` §7.1, passos 4, 5 e 7 |
| (não diz) | Cada evento processado acrescenta uma linha ao ledger mensal `log-AAAA-MM.md` do projeto, além do `log.md` de escrita de página da 001 | `docs/lifecycle-event-ingestion.md` §13, `docs/wiki-files-layer.md` §9 |

### Layout de pacotes e regra de imports

```text
cmd/ai-memory/main.go
internal/domain/            ← + evento canônico de hook, importância por evento, ator (sem I/O)
internal/config/            ← + bind, server_url, allowed_hosts, auth, handoff, routing, spool; LoadHook (caminho rápido)
internal/storage/           ← + sessions, ingest_keys, handoffs (seleção e reivindicação), ponteiro de projeto ativo (memória)
internal/wiki/              ← + AppendHookLog (ledger mensal); WritePage sem mudança de contrato
internal/ingest/            ← normalize (puro), capture (política pura), synth (página de sessão e handoff, puro), process, router HTTP
internal/mcp/               ← interface própria + adaptador mcp-go, 12 tools, transportes stdio e HTTP
internal/cli/               ← serve, hook, hook-drain, install-hooks, install-mcp, status; spool, marcador, .serve.lock, guarda
```

Imports: `ingest` → `domain`, `storage`, `wiki`; `mcp` → `domain`, `storage`, `wiki` (nunca `ingest` em produção); `cli` → todos. O que `ingest` e `mcp` compartilham (ponteiro de projeto ativo, seleção e reivindicação de handoff) mora em `storage`, para não criar dependência `mcp` → `ingest`. O teste de imports da 001 (`internal/archtest`) ganha as arestas novas.

## 2. Design de Contratos e Fronteiras
- **Contrato:** rotas HTTP de máquina, tools MCP, formato do spool e dos comandos de hook, chaves de configuração novas e os arquivos em disco. Nomes de rota, de parâmetro, de campo JSON e de tool são os dos docs.
- **Geração de Código:** nenhuma. Schemas das tools são gerados pelo `mcp-go` a partir das structs de argumento.
- **Esquema SQL:** **sem migração nova.** A V01 já tem `sessions`, `observations`, `ingest_keys` e `handoffs` com o estado final do doc. A 002 só passa a usá-las.

### 2.1 Rotas HTTP (`serve --transport http`)

| Rota | Método | Contrato |
|---|---|---|
| `/hook` | POST | Um evento: parâmetros de consulta (`event` obrigatório, `agent`, `cwd`, `workspace`, `project`, `session_id`, `project_strategy`, `drop_subagent`, `ingest_key`) e corpo JSON. **202** `queued`; **202** com motivo para descarte reconhecido (`capture policy dropped`, `subagent capture dropped`, `invalid marker aliases`); **429** `hook queue full`. |
| `/hook/batch` | POST | Até **256** itens `{url, body}`. Resposta `{accepted, accepted_indices, failed_index, results}`. Falha de processamento interrompe o lote (200 com `failed_index`). Saturação: **429** com os índices já aceitos. Mais de 256 itens: **413**, `accepted = 0`. |
| `/handoff` | GET | Contexto síncrono de início de sessão (`agent`, `cwd`, `workspace`, `project`, `session_id`, `project_strategy`). **200** com texto, **200** vazio se não há nada ou a leitura falha, **403** na recusa de escopo. |
| `/mcp` | POST | MCP HTTP streamable, sem estado, resposta JSON. |
| `/healthz` | GET | Saúde, sem autenticação. |
| `/admin/status` | GET | Contagens, caminhos, versão, tamanho da fila de escrita (`write_queue_depth`), contagens de purgas (`purged_sessions`, `purged_scopes`) e métricas de ingestão (`hook_metrics`: `queued`, `dropped_queue_full`, `dropped_rate_limited`, `dropped_policy`, `batches_accepted`, `batches_rejected`, `queue_depth`, `queue_capacity`). Aberta em operador único (a 002 não distingue operadores). |

Rotas de máquina (`/mcp`, `/hook`, `/hook/batch`, `/handoff`): corpo limitado a **10 MiB** (10 485 760 bytes). Middleware na ordem: filtro de `Host` (`allowed_hosts`, mais externo, cobrindo inclusive `/healthz`: 400 sem `Host`, 403 para `Host` não permitido) → limite de corpo → autenticação → handler.

### 2.2 Evento normalizado (`internal/domain`)

`HookEvent{Kind, Agent, SessionID, CWD, Title, Body, OccurredAt, IngestKey, Subagent, Extension, SourceEvent}`, sem texto cru fora da fronteira: título e corpo só viram `sanitize.Sanitized` no processamento. Vocabulário canônico e aliases (`session-start`, `SessionStart`, `user-prompt`, `UserPromptSubmit`, ...) e importância por evento conforme `lifecycle-event-ingestion.md` §3.1 e §3.2. Identificador de sessão que não é UUID vira UUID v5 no namespace OID. Limites de corpo por evento (§3.6) aplicados depois da sanitização (16 384 bytes para 16 KiB; 2 000 bytes UTF-8 seguros para 2 KB).

### 2.3 Tools MCP (12)

| Tool | Tipo | Pontos de contrato |
|---|---|---|
| `memory_query` | leitura | `query` (aliases `q`, `search`), `workspace`+`project`, `scopes` (≤ 25), `global` (bool), `limit` 1..100 (padrão 10; aliases `n`, `top_k`), `include_expired`, `include_superseded`, `explain`, `pin_first`, `answer` (bool; sem provedor retorna `answer_unavailable`), `reasoning` (`minimal\|low\|medium\|high\|max`). Sem vetor e sem rerank (INV-13). Fallback `raw_hits`; união com `_global` em `global_scope_hits`; com `global=true`, retorna `global_hits`. |
| `memory_read_page` | leitura | `path` ou `query` (`path` vence); devolve `id`, `path`, `title`, `tier`, `version`, `pinned`, `tags`, `body`, `frontmatter`, `scope`, `warnings` e `served_from: "db-fallback"` quando o `.md` falta no disco; `related` (bool, padrão `false`) e `related_depth` (1..3, padrão 1; ≤ 50 nós). |
| `memory_recent` | leitura | `limit` 1..100 (padrão 10; alias `n`); devolve `{pages, scope, warnings}`. |
| `memory_read_session_observations` | leitura | `session_id` **obrigatório** (UUID ou string convertida via UUID v5 OID), `limit` 50 (1..200), `offset` 0, `body_max_chars` 4000 (200..16384; trunca com `\n...[truncated — full body is <N> chars]`), `order` (`asc`\|`desc`), `kinds` (lista de tipos), `query` (busca FTS na sessão); devolve `session`, `observations`, `total`, `returned`, `has_more`, `elided_other_scope`, `scope`, `warnings`. |
| `memory_status` | leitura | contagens (`pages`, `sessions`, `observations`, `open_handoffs`, `purged_sessions`, `purged_scopes`), `write_queue_depth`, `data_dir`, `version`, `hook_metrics`, `scope` (`workspace`, `project`, `resolved_by`) e `warnings`. |
| `memory_briefing` | leitura | `settled_first` (bool); `pinned` (≤ 10, excluindo `_slots/*`); `settled` (≤ 8) quando `settled_first=true`; `counts`, `activity`, `rules`, `slots`, `recent_pages`, `scope`, `warnings`. |
| `memory_handoff_list` | leitura | não reivindica; `state` (`open\|accepted\|expired\|all`, padrão `open`), `limit` 20 (1..100), `all_owners` (bool, só administrador); ordem `created_at DESC`. |
| `memory_write_page` | escrita | `path` e `body` (alias `content`), `title`, `tier` (padrão `episodic`), `tags`, `pinned`, `expires_at`, `expected_page_id` (`-32600` em divergência), `create_only` (`-32600` se já existir), campos OKF (`kind`, `entities`, `abstract`, `relations`, `applies_to`, `enforced_by`), `scope: "global"` grava em `default/_global`; faz commits `pre-checkpoint before mcp write-page <path>` e `mcp write-page <path>`; resposta `{id, path, version, superseded_id, pre_checkpoint, checkpoint, scope, warnings}`. |
| `memory_delete_page` | escrita | `path`, `expected_page_id` (opcional; `-32600` se divergir da versão atual), `workspace`+`project`, `scope` (apenas `"profile"` aceito na validação; `"global"` é rejeitado com `-32602`). Remove fisicamente todas as versões do caminho, limpa entidades órfãs e grava `audit_log` (`delete_page`); sem `expected_page_id`, é idempotente e retorna sempre `deleted: true`, `{deleted: true, path, pre_checkpoint, checkpoint, scope, warnings}`. |
| `memory_handoff_begin` | escrita | `summary` (obrigatório, ≤ 3000), `open_questions`, `next_steps` (cada item ≤ 1500, total ≤ 6000, ≤ 20 itens), `files_touched` (cada caminho ≤ 512, total ≤ 4096, ≤ 20 itens), `from_agent` (padrão `"mcp"`), `shared` (bool; `true` deixa sem dono), `cwd`; resposta `{handoff_id, state: "open", created_at, scope, warnings}`. |
| `memory_handoff_accept` | escrita | `handoff_id` (opcional), `cwd` (opcional), `by_agent` (padrão `"mcp"`), `by_session` (opcional); sem `handoff_id`, seleciona o mais recente elegível para o `cwd` com filtro de sessão viva (10 min); com `handoff_id`, busca aquele id aberto **sem** filtro de sessão viva (`busy_since = None`); varredura pós-reivindicação roda sempre que o handoff reivindicado for automático; resposta `{handoff, status: "claimed" \| "consumed_by_hook" \| "none_pending", scope, warnings}` (quem perde a corrida ou não tem pendente recebe `handoff: null, status: "none_pending"`). |
| `memory_handoff_cancel` | escrita | `handoff_id` (obrigatório), `all_owners` (bool, só administrador); marca como `expired` o handoff aberto de id exato; resposta `{handoff_id, cancelled: bool, scope, warnings}`. |

Versão do protocolo fixa em **2024-11-05**; nome `ai-memory`; capacidade `tools`. Chave do ator: usuário + id de sessão, lido nesta ordem: contexto de transporte, cabeçalho `x-memory-actor-session-id`, id nativo, `mcp-session-id`. Erros das tools usam códigos JSON-RPC (`-32602` parâmetros inválidos / escopo não encontrado / escrita ambígua, `-32600` conflito de versão / permissão, `-32603` erro interno).

### 2.4 Armazenamento (acréscimos em `internal/storage`)

- `OpenOrTouchSessionTx`, `CloseSessionTx`, `SessionIsEphemeral` (definição única de sessão efêmera, usada no cálculo e na verificação), `ReopenLastAcceptedHandoffForSessionTx` (usada no encerramento de sessão efêmera).
- `ClaimIngestKeyTx(project, key)` → `new | pending | completed`; `CompleteIngestKeyTx`. Retenção de 30 dias.
- `InsertHandoffTx` (com expiração prévia dos automáticos do mesmo `cwd` exato e mesmo dono), `SelectHandoff(scope, cwd, owner, busySince)`, `ClaimHandoffTx` (troca `open → accepted` com dupla guarda), `ListHandoffs`, `CancelHandoffTx`, `SweepAfterClaimTx`.
- `ActivePointer`: mapa em memória por ator, modo `per_actor` padrão (`single` e `per_session` por configuração), TTL 1 h, ≤ 4096 entradas, marca "ponteiro ativo" (`has_keyed_entry`) que nunca volta. Consultas `Lookup` (escrita: `Resolved | Mismatch | Unset`; `Unset` recai para o escopo padrão na escrita, enquanto `Mismatch` recusa com `-32602`) e `Read` (leitura, com origem `ScopeSource`: `"explicit"`, `"active_project"`, `"active_project_shared"`, `"active_project_fallback"`, `"default"`, `"default_fallback"`). `ResolveScope` da 001 passa a receber o ponteiro como fonte na precedência do `mcp-server.md` §2.1.

### 2.5 Cliente de hook e disco

- `ai-memory hook --event <evento> --agent claude-code --server-url <url> [--check-capture]`: lê o JSON da entrada padrão (até 1 MiB, removendo BOM UTF-8) e **sempre** responde sucesso ao agente (`{}` no stdout para eventos do Claude Code sem contexto a injetar, ou `{"hookSpecificOutput":{"hookEventName":"SessionStart","additionalContext":...}}` quando há handoff no início de sessão).
- Spool em `<data_dir>/hook-spool/`, um arquivo `0600` por item com nome `{created_ms:013}-{pid}-{seq:016x}-{recovery_key}.json` e JSON `{url, body, created_ms, auth_mode, token?, attempts, profile?}` (onde `url` já contém `ingest_key` na query string). Até 8 tentativas, idade máxima 7 dias, até 10 000 arquivos (verificado sob `<data_dir>/hook-spool.capacity.lock` com espera de até 50 ms; ao atingir 10 000, preserva o backlog existente e **descarta o novo evento recebido** com aviso no stderr), lote de até 256 itens e 8 MiB. No início da drenagem, arquivos órfãos `.json.tmp` com mais de 5 minutos (≤ 10 MiB, JSON válido) são recuperados ou removidos.
- Drenagem: incremental no pós-ferramenta com ≥ 32 itens (orçamento 250 ms); no início de sessão (orçamento 3 s, seguido de `GET /handoff` com timeout de 3 s); nas fronteiras (`stop`, `pre-compact`, `session-end`), `hook-drain` em processo destacado (no Windows com `DETACHED_PROCESS | CREATE_NEW_PROCESS_GROUP | CREATE_NO_WINDOW | CREATE_BREAKAWAY_FROM_JOB` e fallback sem `CREATE_BREAKAWAY_FROM_JOB` em `ERROR_ACCESS_DENIED`), com trava exclusiva do sistema operacional (`LockFileEx` no Windows / `flock` no Unix sobre `<data_dir>/hook-spool/.drain.lock`, conferindo antes se `<data_dir>` ainda existe) e orçamento de 5 min.
- Token do hook em arquivo `0600` no `data_dir` (`hook-auth-token` e `hook-auth.curl`); nunca na linha de comando (repassado ao `hook-drain` via variável de ambiente `AI_MEMORY_AUTH_TOKEN`).
- `.serve.lock` no `data_dir`: trava exclusiva não bloqueante do sistema operacional (`LockFileEx` no Windows / `flock` no Unix) + PID informativo em `.serve.lock.holder`.

### 2.6 Configuração (chaves novas)

`bind` (`127.0.0.1:49374`), `server_url` (`http://127.0.0.1:49374`), `allowed_hosts` (`localhost`, `127.0.0.1`, `::1`), `[auth] token` (vazio = desligada), `[handoff] claim_on_session_start` (`true`) e `create_on_session_end` (`true`), `[routing] mid_session` (`follow_cwd`), `[auto_scope] mode` (`per_actor`). Entram no `.env.example` sem valores secretos.

## 3. Decisões Arquiteturais e Justificativas

- **Decisão (Q-01): ingestão em três camadas.** Normalização tolerante (`ingest/normalize`, pura), política de captura (`ingest/capture`, pura, usada pelo cliente e pela reinspeção do servidor) e orquestração (`ingest/process`). Síntese da página de sessão e do handoff automático em `ingest/synth`, pura, testada por fixtures.
  - **Justificativa:** é a separação pedida pelo doc (§14) e deixa a política testável sem servidor nem disco.
  - **Alternativa Rejeitada:** um handler único que normaliza, decide e grava (não dá para testar a política sem HTTP).

- **Decisão (Q-02): 202 ou 429, nunca bloquear (INV-05).** Semáforo de capacidade **1024** (canal com buffer). `/hook` tenta adquirir sem esperar: conseguiu, responde 202 e processa numa goroutine com o `context` do servidor; não conseguiu, 429. `/hook/batch` processa os itens em sequência dentro da requisição (fail-fast) e devolve 429 com os índices aceitos na saturação. Descarte reconhecido (política, subagente) responde 202 sem gastar capacidade.
  - **Justificativa:** o semáforo limita o fan-out (anti-padrão proibido) e dá o 429 imediato; o lote síncrono é o que permite o reconhecimento por índice que o spool usa.
  - **Alternativa Rejeitada:** fila com worker pool (adiciona uma fila ilimitada ou um segundo limite para observar) e processar `/hook` dentro da requisição (bloqueia o agente).

- **Decisão (Q-03): idempotência pela chave de ingestão, conclusão só no fim.** Gate em memória por `(projeto, chave)` (≤ 4096 entradas mortas, removidas de forma oportunista). Na mesma transação: abre ou toca a sessão, reivindica a chave e grava a observação sanitizada. Efeitos seguintes (ledger, fim de sessão) rodam depois, e a chave só é marcada como concluída ao final. Reentrega com chave pendente retoma os efeitos sem nova observação; com chave concluída, não faz nada.
  - **Justificativa:** "pelo menos uma vez" seguro e retomável (doc §8 e §14). Cobre o edge case de interrupção no meio do fim de sessão.
  - **Alternativa Rejeitada:** deduplicar por hash do corpo (eventos legítimos iguais sumiriam) e transação única cobrindo tudo (a escrita da página é outra transação, da wiki).

- **Decisão (Q-04): fim de sessão em duas etapas ordenadas.** Sessão efêmera (sem prompt nem ferramenta) encerra sem página, sem handoff, reabre o handoff mais recente aceito por aquela sessão efêmera (`state = 'open'`, limpando `accepted_at`, `accepted_by_session`, `accepted_by_agent`) e faz commit `lifecycle-only session <id8>`. Sessão substantiva: (1) sintetiza `sessions/<id>.md` (tier `episodic`, frontmatter `title`, `session_id`, `agent`, `tier`, `summary`) e grava por `wiki.WritePage`, mantendo a página se o agente já a escreveu; (2) numa transação de `storage`: expira os handoffs automáticos abertos do mesmo `cwd` exato e mesmo dono, encerra a sessão e insere o handoff automático (se `create_on_session_end`); (3) commit `session <id8>: <título ≤ 60>`.
  - **Justificativa:** a constituição exige que a wiki só mude pela camada `wiki`; encerrar a sessão e gravar o handoff juntos evita sessão encerrada sem sucessor (doc §8.3). A página antes do handoff garante que o handoff aponte para conteúdo existente.
  - **Alternativa Rejeitada:** gravar a página dentro da transação do handoff (exigiria `storage` escrever arquivo, proibido) e handoff antes da página.

- **Decisão (Q-05): handoff de uso único, com seleção e reivindicação em `storage` (INV-16).** A seleção segue `design-handoff-offer-not-claim.md` §3: filtro de dono, só `open`, manual vence automático, automático só se o `cwd` dele for ancestral ou igual ao do receptor (fronteira de componente, sem diferenciar caixa no Windows), sessão de origem viva nos últimos 10 min é protegida, desempate por `created_at`, `cwd` mais longo e id. A reivindicação é uma transação com **duas guardas independentes de `open`**: a leitura de metadados exige `state = 'open'` e o `UPDATE ... SET state = 'accepted' WHERE id = ? AND state = 'open'` precisa afetar exatamente 1 linha; a regra de sessão viva é reavaliada dentro da transação quando `busy_since` é informado (seleção sem `handoff_id`), enquanto a reivindicação por `handoff_id` explícito passa `busy_since = None` e executa a varredura pós-reivindicação sempre que o handoff reivindicado for automático; uma sessão receptora que já reivindicou não reivindica outro (`consumed_by_hook`). `GET /handoff` lê e renderiza tudo antes de reivindicar; com `claim_on_session_start = false`, só emite a nota de oferta (somente metadados). Conteúdo entregue vai entre `<!-- ai-memory:untrusted-history:start -->` e `<!-- ai-memory:untrusted-history:end -->`. Nota de oferta: preserva o sufixo literal do sistema de referência (`left <idade> ago`, resultando em `left just now ago` ou `left N minutes ago ago` conforme `design-handoff-offer-not-claim.md` §6).
  - **Justificativa:** `ingest` (rota `/handoff`) e `mcp` (`memory_handoff_accept`) usam o mesmo caminho atômico sem `mcp` importar `ingest`; as duas guardas são exigência explícita da constituição.
  - **Alternativa Rejeitada:** trava em memória (não protege dois processos nem sobrevive a reinício) e `SELECT ... FOR UPDATE` (o SQLite não tem; o escritor único já serializa, e a guarda no `UPDATE` cobre o resto).

- **Decisão (Q-06): ponteiro de projeto ativo em memória, em `storage`.** Implementa `domain-vocabulary.md` §9: modo `per_actor` padrão, chave (usuário, sessão) com slot só de usuário, slot compartilhado para chamador sem coordenada, TTL 1 h, teto 4096, marca `has_keyed_entry` que nunca volta; `Mismatch` falha fechado na escrita (`-32602`), enquanto `Unset` recai para o escopo padrão configurado. Início de sessão, prompt e pré-ferramenta avançam o ponteiro; eventos de conclusão só atualizam a entrada exata do ator. A resolução de projeto do hook segue `lifecycle-event-ingestion.md` §7 (incluindo sessão pegajosa e `mid_session`) e **cria** workspace e projeto quando necessário, recusando `_global`. No boot do transporte HTTP, uma semente só de leitura reconstrói o projeto mais recente dentro do TTL, para leituras sem escopo não responderem vazio depois de um reinício (`docs/auto-scope.md` § "Surviving a restart", T092).
  - **Justificativa:** o ponteiro é estado de processo (o doc não o persiste) e precisa ser lido pelo `mcp` e escrito pelo `ingest`; em `storage` ele fica junto do resolvedor central de escopo.
  - **Alternativa Rejeitada:** pacote novo `internal/scope` (mudaria o grafo de imports da constituição) e persistir o ponteiro em tabela (o doc define TTL de processo; persistir faria uma sessão antiga redirecionar a próxima após reinício).

- **Decisão (Q-07): MCP com `mark3labs/mcp-go` atrás de interface própria.** O pacote `mcp` define `ToolServer` e os handlers sobre `storage` e `wiki`; um adaptador fino liga ao `mcp-go` e fixa o protocolo em `2024-11-05` por wrapper. Transportes: stdio (padrão) e HTTP streamable sem estado com resposta JSON em `/mcp`. A identidade vem só do contexto da requisição, preenchido pelo middleware; nunca de cabeçalho cru dentro do handler.
  - **Justificativa:** escolha do autor (ADR-05 do discovery); a interface permite trocar o SDK.
  - **Alternativa Rejeitada:** SDK oficial `modelcontextprotocol/go-sdk` e JSON-RPC próprio (registrados na spec).

- **Decisão (Q-08): servidor HTTP com `net/http` e autenticação de operador único.** Bearer comparado em tempo constante após aplicar SHA-256 nos dois lados (evitando vazar o comprimento do token); um único token portador aceito (mais de um falha fechado); esquema `Bearer` sem diferenciar maiúsculas; token configurado vazio desliga a autenticação; nunca aceita token vazio. Rotas de máquina sem autoridade configurada passam como anônimas; com autoridade e sem token válido, 401 com `WWW-Authenticate: Bearer` e corpo JSON `{"error": "unauthorized: missing or invalid Authorization: Bearer <token> header"}`. Bind fora de loopback sem autenticação é recusado, salvo `--allow-insecure-no-auth` ou detecção de contêiner. Encerramento com janela de 5 s.
  - **Justificativa:** a stdlib cobre roteamento e limites; multiusuário, proxy de identidade e autenticação humana são das features 007 e 009.
  - **Alternativa Rejeitada:** roteador de terceiros (dependência sem ganho) e implementar já o modelo de usuários (fora do escopo).

- **Decisão (Q-09): cliente de hook no caminho rápido, com spool e drenagem desacoplada.** `hook` e `hook-drain` não carregam a configuração completa nem o log: usam `config.LoadHook`, um carregador mínimo **dentro de `internal/config`** (data_dir, server_url, arquivo do token e as variáveis de tempo `AI_MEMORY_HOOK_*`, em minutos, até 60). O item vai ao spool antes de qualquer rede, com a chave de ingestão (UUID) gerada uma vez e reaproveitada nas novas tentativas. A trava do spool (`<data_dir>/hook-spool/.drain.lock`) usa trava exclusiva de arquivo do sistema operacional (`LockFileEx` com `LOCKFILE_EXCLUSIVE_LOCK | LOCKFILE_FAIL_IMMEDIATELY` via `syscall.NewLazyDLL("kernel32.dll")` no Windows; `flock(LOCK_EX | LOCK_NB)` no Unix), liberada automaticamente pelo kernel se o processo morrer, conferindo antes se `<data_dir>` existe. `hook-drain` é iniciado com `os/exec` em processo destacado (no Windows com fallback sem `CREATE_BREAKAWAY_FROM_JOB` em caso de `ERROR_ACCESS_DENIED`), saídas descartadas. Marcador `.ai-memory.toml` lido pela regra de linha do `marker-file.md` (escopo) e por TOML estrito para `[capture]`, com os limites do doc. Política de captura conforme `lifecycle-event-ingestion.md` §4 e `marker-file.md` (denylist padrão). Descarte de subagente marcado (`drop_subagent_captures`) só para eventos marcados, que é o comportamento do Claude Code.
  - **Justificativa:** o hook roda a cada chamada de ferramenta e precisa voltar em < 200 ms sem rede; manter o carregador em `internal/config` preserva a INV-01 (um único pacote lê ambiente; cada processo carrega uma vez), e a trava do SO evita lock órfão após queda.
  - **Alternativa Rejeitada:** carregar a configuração completa no hook (lento e com log), ler `os.Getenv` no `cli` (viola INV-01) e trava manual por arquivo PID `O_EXCL` (pode deixar lock órfão ou sofrer corrida).

- **Decisão (Q-10): `.serve.lock` e guarda por enumeração de processos.** `serve` adquire trava exclusiva não bloqueante do sistema operacional sobre `<data_dir>/.serve.lock` (`LockFileEx` no Windows / `flock` no Unix, com até 5 tentativas espaçadas de 25 ms apenas para erros transitórios de abertura, e gravação informativa do PID em `.serve.lock.holder`) antes de abrir o banco; um segundo `serve` no mesmo `data_dir` recusa citando o PID detentor, salvo `--force`. A guarda de `reset`, `restore`, `reindex` e `uninstall --purge-data` **enumera os processos do sistema** e recusa se houver outro processo `ai-memory` vivo além do próprio: Linux por `/proc/*/exe` (ou `comm`), Windows por `CreateToolhelp32Snapshot`/`Process32Next` do pacote `syscall` (comparando `ai-memory` e `ai-memory.exe` sem diferenciar maiúsculas e respeitando `AI_MEMORY_TEST_NO_PROCESS_GUARD` e `AI_MEMORY_TEST_FORCE_SIBLING_PIDS`). Substitui o `ai-memory.pid` da 001 (`internal/cli/liveguard*.go`).
  - **Justificativa:** é o mecanismo dos docs (`docs/windows.md`, `docs/cli.md` §3.1); a trava do SO sobre `.serve.lock` é liberada automaticamente pelo kernel se o servidor cair, e a enumeração de processos pega `hook-drain` e outros comandos; tudo na stdlib.
  - **Alternativa Rejeitada:** manter `ai-memory.pid` ou `O_EXCL` sem trava do SO (deixa lock preso após crash) e `x/sys` (dependência fora de `tech.md`).

- **Decisão (Q-11): instalação idempotente para o Claude Code.** `install-hooks --agent claude-code` imprime, e com `--apply` mescla, os 9 hooks do Claude Code da `docs/support-matrix.md` (`SessionStart`, `UserPromptSubmit`, `PreToolUse`, `PostToolUse`, `PreCompact`, `Stop`, `SessionEnd`, `SubagentStart`, `SubagentStop`) em `~/.claude/settings.json`, com `--capture-mode` e `--no-capture-prompts`, apontando para `ai-memory hook --event ... --agent claude-code --server-url ...` (no Windows nativo, usando formato exec-form `{"type":"command","command":"<exe>","args":[...]}` com `"timeout": 15` em `SessionEnd`; o token fica nos arquivos `0600` `hook-auth-token` e `hook-auth.curl`, não no comando). `install-mcp --client claude-code` faz o mesmo para `mcpServers.ai-memory` (`type: http`, `url`, `headers.Authorization` só com token) em `~/.claude.json` ou `$CLAUDE_CONFIG_DIR/.claude.json`. Escrita: resolve links simbólicos, aplica a transformação sobre o conteúdo atual, não toca o disco se o resultado for igual, senão copia para `<arquivo>.bak-<unix>` e grava de forma atômica. A edição preserva as entradas de terceiros.
  - **Justificativa:** `cli.md` §8, `mcp-install.md` e `windows.md`; o hook dos outros não pode ser perdido.
  - **Alternativa Rejeitada:** sobrescrever o arquivo (destrói configuração do usuário) e um comando único `install` (não é o contrato).

- **Decisão (Q-12): ledger mensal de eventos de hook.** `wiki.AppendHookLog` acrescenta `## [AAAA-MM-DDTHH:MM:SSZ] <evento> | <título>` em `log-AAAA-MM.md` (mês do evento, UTC), título com quebras trocadas por espaço e cortado em 120 caracteres, evento em kebab-case. O `log.md` da 001 continua para escrita e exclusão de página. Os dois são ignorados como página pelo watcher e pelo reindex (regra de nome e conteúdo do `wiki-files-layer.md` §9).
  - **Justificativa:** `lifecycle-event-ingestion.md` §13.
  - **Alternativa Rejeitada:** reaproveitar `log.md` para tudo (o doc separa por mês para o arquivo não crescer sem limite).

- **Decisão (Q-13): contratos de teste.** `TestINV05_*`: 1025 requisições em voo produzem 429 sem bloquear, lote de 257 dá 413, descarte reconhecido não gasta capacidade. `TestINV16_*`: dois aceites concorrentes do mesmo handoff com um vencedor (`-race`), ponteiro de dois atores no mesmo projeto não se sobrescreve. Idempotência: replay com a mesma chave não cria segunda observação, inclusive após interrupção simulada no fim de sessão. Fixtures de payload do Claude Code versionadas em `internal/ingest/testdata/claude-code/`. Fluxo ponta a ponta em `test/acceptance` (hook → observação → `sessions/<id>.md` → handoff → `memory_query`). Cenários do `design-handoff-offer-not-claim.md` §9 (1, 2, 4 e 7) como testes de aceitação; os de admissão (6), run gerenciado e multiusuário ficam fora.
  - **Justificativa:** a constituição exige teste multi-sessão nomeado e as invariantes INV-05 e INV-16 com teste de contrato.
  - **Alternativa Rejeitada:** só testes unitários de handler (não exercitam saturação nem corrida).

- **Decisão (Q-14): escritas rastreadas e commit no ponto definido pelo doc (corrige a 001).** O adaptador git registra os caminhos escritos, renomeados ou removidos, e o commit inclui só os registrados. `WritePage` e `DeletePage` não fazem commit: quem chama faz, no ponto que o doc define. O fim de sessão e os checkpoints gravam a página e fazem um único commit com o rótulo do doc; `memory_write_page` e `memory_delete_page` fazem o checkpoint logo depois da operação. O watcher registra a remoção de arquivo, para o próximo commit incluí-la (T080, T086). Pelo mesmo fluxo (`docs/wiki-files-layer.md` §5.2), a escrita de página deixa de acrescentar linha em `log.md` e de escrever `index.md`, e passa a fixar as páginas de `_slots/` (T089).
  - **Justificativa:** `docs/wiki-files-layer.md` §7 e §8.1; sem isso, o fim de sessão produziria dois commits (`page-write` e `session`).
  - **Alternativa Rejeitada:** manter o commit por escrita e só acrescentar o commit de sessão (histórico diferente do doc) e varrer a árvore a cada commit (caro; o doc reserva a varredura para casos de registro não confiável).

- **Decisão (Q-15): backup pelo servidor (corrige a 001).** `POST /admin/backup` valida a árvore da wiki, tira o snapshot online do banco (API de backup do SQLite, escritor ativo) e devolve um tarball gzip com `wiki/`, `db/memory.sqlite` e `config.toml` se existir. `backup --to` (`-o`) pede ao servidor e grava o corpo; não abre o banco direto (T081, T082).
  - **Justificativa:** `docs/lifecycle-ops.md` § backup, `docs/cli.md` §9 e `docs/mcp-server.md` §6; INV-09 (backup online).
  - **Alternativa Rejeitada:** manter o snapshot SQLite local da 001 (não é o contrato e não inclui a wiki).

- **Decisão (Q-16): diretório de dados padrão do doc (corrige a 001).** Padrão no diretório de dados local do usuário com a subpasta `ai-memory`; flag global `--data-dir` > `AI_MEMORY_DATA_DIR` > arquivo; `config.toml` lido do diretório de dados quando `--config` falta. O spool, o token do hook e a `.serve.lock` ficam nesse diretório (T079).
  - **Justificativa:** `docs/design-decisions.md` e `docs/cli.md` §1, §2.1 e §2.5. Com o padrão relativo da 001 (`data`), cada repositório onde o hook roda ganharia um `./data` próprio.
  - **Alternativa Rejeitada:** exigir `--data-dir` sempre (quebra os hooks instalados).

- **Decisão (Q-17): reindex e manifestos como no doc (corrige a 001).** O comando `reindex` recusa se o banco já tem linhas nas tabelas que reconstrói e nomeia as contagens; `_meta.md` ausente ou inválido é erro com o caminho exato; o `serve` preenche no boot os `_meta.md` que faltam, de forma idempotente, para que o reindex sempre tenha manifesto (T083, T084, T085). A criação de escopo pelo reindex a partir do manifesto é a reconstrução prevista no doc (resolve a D3 da auditoria da 001).
  - **Justificativa:** `docs/lifecycle-ops.md` § reindex e `docs/cli.md` §3.1.
  - **Alternativa Rejeitada:** reconstruir o nome pelo id (estado que o doc trata como erro) e checar banco limpo dentro de `wiki.Reindex` (a reconciliação do watcher usa a mesma função e não pode recusar com banco populado).

- **Decisão (Q-18): frontmatter com mapeamentos e conformidade OKF na escrita (corrige a 001).** O tipo de frontmatter do domínio passa a representar mapeamentos aninhados e listas de mapeamentos, mantendo a ordem das chaves. A escrita emite `generated: {by, at}` (não mais as chaves planas `generated.by` e `generated.at`), com `generated.by` igual a `generated_by_model` ou `process:ai-memory/<versão do build>`, deriva `sources` de `session_id` e `agent`, e lê `relations` como mapeamento por relação (T090, T091). Revê a decisão da 001/T084, que mantinha mapeamento recusado.
  - **Justificativa:** `docs/okf.md` § Proveniência e `docs/typed-edges.md`. Sem isso, um arquivo escrito à mão no formato do doc é recusado inteiro, e as páginas gravadas ficam fora do OKF. Corrigir antes de a 002 gravar páginas de sessão evita uma migração depois.
  - **Alternativa Rejeitada:** tratar `generated`, `sources` e `relations` como exceções com tipos próprios (cada chave nova do formato pediria outra exceção) e deixar para a 006 (exigiria migrar as páginas já gravadas).

### O que não construir nesta feature

Cadeia de admissão (webhooks e `X-Memory-Skip-Admission-Chain`), limite por fonte (`hook_rate_per_sec`, desligado por padrão no doc), captura da mensagem do assistente (o campo bruto `last_assistant_message` é **sempre removido**, mas o opt-in não é implementado), brief e perfil no `/handoff`, runs gerenciados e `/workstream/*`, mensagens entre projetos, `mcp-bridge` (`--session-aware`), dialetos de schema (`?flavor=`), `doctor`, `backfill`, scripts de shell de hook, demais agentes e demais tools MCP.

## 4. Auditoria de Constituição

**Portões de Qualidade**
- [ ] `CGO_ENABLED=0 go build` e `go vet` em `windows/amd64` e `linux/amd64` — só stdlib e `mcp-go` (pure Go, conferido na task que o adiciona).
- [ ] `go test -race ./...` — Q-02, Q-05 e Q-13, via `scripts/check.sh` (container quando não há gcc).
- [ ] Teste multi-sessão para identidade de sessão, aceitação de handoff, escritor e filtros de dono — Q-05, Q-06 e Q-13 (`TestINV16_*`).
- [ ] Cada invariante com teste nomeado — `TestINV05_*` e `TestINV16_*` novos; os da 001 continuam.
- [ ] `Aprovado: true` e Gherkin — spec aprovada e já revisada contra os docs; os cenários viram o `.feature` da 002 na fase de tasks.
- [ ] Dependência nova só via `tech.md` — `mcp-go` já listado como planejado; a task que o importa fixa a versão em `tech.md`. Nenhuma outra dependência (Q-09 e Q-10 evitam `x/sys`).

**Restrições Arquiteturais**
- [ ] Binário único, layout `cmd/ai-memory` + `internal/...` — §1.
- [ ] Grafo de imports — §1; `mcp` não importa `ingest` (Q-05 e Q-06 põem o compartilhado em `storage`); `internal/archtest` atualizado.
- [ ] INV-01 — `config.Load` e `config.LoadHook`, ambos em `internal/config` (Q-09); o teste de grep da 001 continua valendo.
- [ ] INV-02 — toda gravação nova passa pelo escritor único (`Writer.Do`).
- [ ] INV-03 — observação, sessão e chave de ingestão na mesma transação (Q-03); FTS por trigger.
- [ ] INV-04 — sessões, observações e handoffs carregam `(workspace_id, project_id)`; triggers de pareamento da V01.
- [ ] INV-05 — Q-02; timeout de POST do hook padrão 2 s, configurável (ADR-04).
- [ ] INV-06 — título e corpo do evento e do handoff só viram `Sanitized` por `sanitize.Title`/`Body`; limites aplicados depois da remoção (Q-01, Q-03, Q-05).
- [ ] INV-09 — Q-10 (`.serve.lock` e enumeração de processos); `backup` online pelo servidor (Q-15).
- [ ] INV-10 — página de sessão por `wiki.WritePage` (atômica); spool e `install-*` também gravam de forma atômica; o ledger é só acréscimo (exceção já registrada na 001, D2).
- [ ] INV-11 — diretório de dados padrão do doc e `--data-dir` (Q-16); `serve` registra o `data_dir` canônico.
- [ ] INV-12 — reservada.
- [ ] INV-13 — nenhum provedor; `memory_query` sem `answer`, vetor ou rerank.
- [ ] INV-15 — logs do `serve` filtram o próprio módulo; o caminho rápido do hook não loga.
- [ ] INV-16 — páginas compartilhadas e leitura sem filtro de autoria (inalterado); handoff com dono e exatamente uma vez com duas guardas (Q-05); ponteiro por ator (Q-06).
- [ ] Resolução de escopo central — `memory_*` e `/handoff` usam `ResolveScope`; leitura nunca cria e falha fechado; escrita (`memory_write_page`, hook) cria só nomeado, conforme `mcp-server.md` §2.1 e `lifecycle-event-ingestion.md` §7.1.
- [ ] Autorização por capacidade e token em tempo constante — Q-08, num único ponto de decisão.
- [ ] Mutações da wiki só pela camada `wiki` — Q-04, Q-12 e Q-14 (commits pelos caminhos registrados).
- [ ] Markdown é a fonte da verdade — páginas de sessão são markdown reindexáveis.

**Regras de Negócio Transversais**
- [ ] Ingestão idempotente por chave, replay converge — Q-03.
- [ ] Rollback do arquivo quando o banco falha — herdado da 001 (`WritePage`), usado por `memory_write_page` e pela página de sessão.
- [ ] Observação nunca chega ao storage sem sanitizador — assinatura de `InsertObservation` (001) exige `Sanitized`.
- [ ] Tombamento desligado — inalterado.
- [ ] Escopo faseado, zero-LLM — §1 e "O que não construir".

**Preferências de Bibliotecas**
- [ ] `modernc.org/sqlite`; `mcp-go` atrás de interface do pacote `mcp` (Q-07); `cobra`/`koanf`; `google/uuid` (UUIDv7 e v5); `slog`; `regexp`; `git` por `os/exec`; stdlib onde resolve (Q-02, Q-08, Q-09, Q-10).

**Anti-Padrões Proibidos**
- [ ] Sem `os.Getenv` fora de `config` — Q-09.
- [ ] Sem escrita de arquivo da wiki fora de `wiki` — Q-04 e Q-12; spool, `.serve.lock` e `install-*` ficam fora da wiki.
- [ ] Sem "indexar depois" — Q-03.
- [ ] Sem filtro de leitura por autoria — o filtro de dono só se aplica a handoffs, que têm dono por regra (INV-16).
- [ ] Sem criar escopo em leitura, busca ou destrutivo — `/handoff` e tools de leitura usam `ResolveScope`.
- [ ] Sem concatenar caminho à mão — `PagePath` e `pathFor`; `sessions/<id>.md` montado pelo tipo de caminho.
- [ ] Sem `panic` — erros tipados; pânico em handler HTTP ou de tool vira 500 ou erro de tool, sem derrubar o servidor.
- [ ] Sem abstração preemptiva ou stub — "O que não construir" fica fora, sem esqueleto; a interface do MCP tem um adaptador real.
- [ ] Goroutine com `context` e fan-out limitado — Q-02 (semáforo), drenagem com orçamento, `hook-drain` em processo separado.
- [ ] "A confirmar" vira decisão nossa — nota de oferta (Q-05), significado de `hook_rate_per_sec = 0` (não implementado), regra de sessão viva na reivindicação por id (aplicada, Q-05).
- [ ] Clean-room — contratos só dos docs listados no §1.

> **Notas de desvio e pendências**
> 1. A spec aprovada divergia dos docs em dez pontos; todos foram corrigidos na spec (seção "Revisão contra os docs"). Os que mudam a arquitetura estão na tabela do §1.
> 2. A Q-10 substitui o mecanismo de PID da 001. A 001 tem testes da guarda (`liveguard_test.go`, `reindex_cmd_test.go`) que mudam junto.
> 3. As decisões D1 a D4 da auditoria da 001 (T089) foram incorporadas à constituição em 2026-10-13: D2 e D3 pelos docs, D1 e D4 como decisão nossa. A D3 está refletida na Q-17.
> 4. A validação da 001 contra os docs gerou as tasks T079 a T088 (fase 1b). As duas últimas corrigem o `storage` sem ADR novo, porque só alinham o código ao `docs/storage-and-retrieval.md` §5.1 e §5.2: exclusão remove todas as versões do caminho (T087) e a ressurreição limpa o `superseded_at` da lápide (T088).
