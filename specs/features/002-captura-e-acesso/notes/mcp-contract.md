# Contrato das 12 tools MCP e resolução de escopo (T003)

Fontes, em ordem de prioridade: `docs/` (vence), depois `specs/features/002-captura-e-acesso/spec.md` e `plan.md`. Onde o doc é omisso, o texto marca **decisão nossa**. Siglas: MS = `docs/mcp-server.md`; AR = `docs/architecture.md` (seção "MCP tool surface"); SR = `docs/storage-and-retrieval.md`; DV = `docs/domain-vocabulary.md`; HO = `docs/design-handoff-offer-not-claim.md`; DD = `docs/design-decisions.md`.

## 0. Divergências entre fontes (atenção do Analyst)

- MS §1 e cabeçalho dizem "23 ferramentas". AR diz "20 tools" no título, mas a tabela lista 22 linhas. A contagem não é consistente nos docs. A 002 implementa um **subconjunto de 12** (spec.md "Dentro do escopo", linha das tools; plan.md §2.3). Ficam **fora** da 002: `memory_feedback`, `memory_forget_sweep`, `memory_lint`, `memory_install_self_routing`, `memory_explore`, `memory_consolidate`, `memory_auto_improve`, `memory_message_*` (4) (spec.md "Fora do escopo"; plan.md §6 "O que não construir").
- O doc usa `memory_handoff` como 4 tools separadas (AR; MS §3.3; HO §7). O plano já alinhou (plan.md linha 15; spec.md tabela de divergências, item 1).
- Algumas respostas só aparecem no doc com "a confirmar" (marcadas abaixo).

## 1. Princípios do servidor (MS §1, §2; plan.md §2.3, Q-07)

- Protocolo MCP fixo em **2024-11-05**; nome do servidor `ai-memory`; capacidade `tools`; o servidor anuncia instruções de uso (MS §1). Se o `mcp-go` não permitir fixar a versão, a primeira task de MCP registra ADR (plan.md Q-07).
- Transportes: stdio (padrão) e HTTP streamable sem estado, resposta JSON, em `POST /mcp` (plan.md §2.2 e Q-07). Autenticação Bearer; sessão web nunca autentica `/mcp` (MS §4.3, §5).
- Identidade vem só do contexto da requisição, preenchido pelo middleware de autenticação, nunca de cabeçalho cru no handler. Não usar variáveis associadas à tarefa, porque a lib MCP roda cada tool em tarefa separada (MS §1).
- Escopo sempre pelo resolvedor de escopo; leitura não cria e falha fechado; escrita pode criar workspace e projeto nomeados (MS §1).
- **Ponto único de chamada de tool** registra atividade do cliente e alimenta o relógio de atividade do ciclo de sonho (MS §1). Na 002 não há dream pass; **decisão nossa**: manter o ponto único (wrapper) e registrar a atividade só se a 001 já tiver o armazenamento (`client_activity`); caso contrário, deixar fora, sem stub (plan.md §6).
- Schemas das tools gerados pelo `mcp-go` a partir das structs de argumento (plan.md §1, "Geração de Código"). Dialetos `?flavor=` ficam **fora** (plan.md §6).
- Toda escrita de página passa pelo caminho único da wiki (sanitização, admissão, atribuição, rollback, índice) (MS §1). Na 002 não há cadeia de admissão (plan.md linha 6).
- O pacote `mcp` não importa `ingest` em produção; ponteiro de projeto ativo, seleção e reivindicação de handoff moram em `storage` (plan.md §1, Q-05, Q-06).
- Sem `panic`: pânico em handler vira erro de tool, sem derrubar o servidor (plan.md checklist, linha 223).

## 2. Resolução de escopo e chave do ator

### 2.1 Chave do ator (MS §2.1; DV §9.3, §8.4; plan.md §2.3)

- Chave do ator = (usuário, id de sessão). O "usuário" é a **chave de identidade** de armazenamento (DV §8.4): `user:<nome>` ou `oidc:<len emissor>:<emissor><sujeito>`. Na 002 só existe o ator anônimo ou o do token único, então o usuário é vazio ou `user:<nome>` (**decisão nossa**: multiusuário/OIDC fora do escopo, plan.md linha 6).
- Id de sessão, nesta ordem (MS §2.1; plan.md §2.3): (1) contexto de transporte; (2) cabeçalho `x-memory-actor-session-id`; (3) id de sessão nativo; (4) cabeçalho `mcp-session-id`.
- O roteador de hooks precisa usar a **mesma regra**; senão leituras e escritas do mesmo operador caem em slots diferentes (MS §2.1).
- Chave vazia (sem usuário nem sessão) usa o slot compartilhado (DV §9.1, §9.3).

### 2.2 Ponteiro de projeto ativo (DV §9; plan.md Q-06, §2.4)

- Modos: `single`, `per_session`, `per_actor` (**padrão**) (DV §9.1). Na 002: `per_actor` padrão; `single` e `per_session` por configuração (plan.md Q-06).
- Cada entrada guarda projeto resolvido, preferência `default_global` e instante de inserção. TTL **1 h**; teto **4096** entradas, removendo as mais antigas; expiradas removidas na leitura e na escrita (DV §9.3).
- Marca "ponteiro ativo" (alguma entrada já publicada) nunca é apagada (DV §9.4).
- Consulta de escrita `Lookup` devolve `Resolved | Mismatch | Unset`. `Mismatch` = chamador tem coordenada, ponteiro ativo e nada casou; **nunca** responde com o slot de outra sessão, falha fechada (DV §9.4).
- Consulta de leitura `Read` devolve origem: `Session`, `SharedSlot`, `StartupSeed`, mais `Mismatch` e `Unset` (DV §9.4).
- A semente de startup (`--project`) é **só para leitura**; nunca grava (DV §9.4).
- Hooks (início de sessão, prompt, pré-ferramenta) avançam o ponteiro; eventos de conclusão só atualizam a entrada exata do ator (plan.md Q-06).

### 2.3 `ScopeSource` (SR §14.1; DD §11; MS §2.1; AR `memory_status`)

- Valores internos / conceituais: `Explicit`, `ActiveProject` (`Session`), `ActiveProjectShared` (`SharedSlot`), `ActiveProjectFallback` (`StartupSeed`), `Default`, `DefaultFallback` (`DefaultAfterMismatch`).
- Valores serializados no wire JSON (`scope.resolved_by`): `"explicit"`, `"active_project"`, `"active_project_shared"`, `"active_project_fallback"`, `"default"`, `"default_fallback"` (MS §2.1).
- "Fallback" = `active_project_fallback` e `default_fallback`; "inferidas" = tudo exceto `explicit` e `active_project` (SR §14.1). Leitura sem escopo resolvida por fallback adiciona aviso em `warnings` (MS §2.1).
- Precedência de **leitura** (MS §2.1): (1) `workspace`+`project` explícitos; (2) `project` explícito (aceita `ws/proj` se o nome literal não existir no workspace ativo/padrão; senão busca no workspace do ponteiro ativo e depois no workspace padrão); (3) ponteiro de projeto ativo do ator (`active_project`, `active_project_shared`, `active_project_fallback`); (4) projeto padrão de startup (`default` ou `default_fallback`).

### 2.4 Regras de escopo para leitura e escrita

- **Leitura, busca e operações destrutivas (`memory_delete_page`, `memory_handoff_*` exceto `begin`):** busca sem criação; escopo explícito inexistente falha fechado (`-32602`) (MS §2.1; SR §14.1).
- **Escrita (`memory_write_page` e `memory_handoff_begin`, únicas que criam workspace e projeto nomeados)** (SR §14.1; MS §2.1):
  - com `workspace`+`project` explícitos: cria escopo se não existir (recusando `_global` e `_profile*` salvo via `scope`);
  - com `project` e sem `workspace`: se contiver `/` e o nome literal não existir, divide em `ws/proj`; caso contrário usa o workspace do ponteiro ativo (ou o workspace padrão se o ponteiro estiver `Unset`); se o ponteiro estiver em `Mismatch`, falha com `-32602 ambiguous write`;
  - sem `workspace` e sem `project`: consulta `ActiveProject.Lookup`; se `Resolved`, usa o escopo ativo; se `Mismatch` (alguma entrada chaveada já foi publicada neste processo ou o usuário tem entrada só de identidade, e nada casou com a coordenada do chamador), recusa com `-32602 ambiguous unscoped write`; se `Unset` (nenhuma entrada chaveada jamais publicada e slot compartilhado vazio, ex.: modo `stdio`), recai para o escopo padrão configurado (`default/scratch`).

### 2.5 Aliases de parâmetros (MS §2.1; AR)

- `query` aceita também `q` e `search` (`memory_query`).
- Limite: `n` ou `top_k` em `memory_query`; `n` em `memory_recent`.
- Em `memory_write_page`, `body` aceita alias `content`. Se canônico e alias vierem juntos, o canônico vence.

## 3. Contrato por tool (12)

### 3.1 `memory_query` (leitura) — MS §3.1
- **Argumentos:** `query` (aliases `q`, `search`, obrigatório, não vazio), `workspace`, `project`, `scopes` (≤ 25), `global` (bool), `limit` (padrão 10, clamp 1..100; aliases `n`, `top_k`), `include_expired`, `include_superseded`, `explain`, `pin_first`, `answer` (bool), `reasoning` (`minimal|low|medium|high|max`).
- **Modo zero-LLM:** quando `answer=true` e não há provedor LLM configurado, retorna os resultados normalmente com `answer: null` e `answer_unavailable: "no LLM provider configured (set [llm].provider in config.toml)"`.
- **Resposta:** `{query, scope, scopes, hits, raw_hits, global_hits, global_scope_hits, profile_hits: [], answer, answer_unavailable, warnings}` (e `streams_active` / `score_details` quando `explain=true`).

### 3.2 `memory_read_page` (leitura) — MS §3.2
- **Argumentos:** `path` ou `query` (`path` vence; erro `-32602` se ambos ausentes), `workspace`, `project`, `related` (bool, padrão `false`), `related_depth` (padrão 1, clamp 1..3).
- **Resposta:** `{id, path, title, tier, version, pinned, tags, body, frontmatter, served_from?, related?, scope, warnings}` (`served_from: "db-fallback"` quando o `.md` falta no disco).

### 3.3 `memory_recent` (leitura) — MS §3.2
- **Argumentos:** `limit` (padrão 10, clamp 1..100; alias `n`), `workspace`, `project`.
- **Resposta:** `{pages: [{id, path, title, summary, tier, updated_at, tags, importance}], scope, warnings}`.

### 3.4 `memory_read_session_observations` (leitura) — MS §3.2
- **Argumentos:** `session_id` (**obrigatório**, UUID ou string convertida via UUID v5 OID), `workspace`, `project`, `kinds` (array de tipos), `query` (busca FTS na sessão), `limit` (padrão 50, clamp 1..200), `offset` (padrão 0), `order` (`asc` padrão ou `desc`), `body_max_chars` (padrão 4000, clamp 200..16384; trunca com `\n...[truncated — full body is <N> chars]`).
- **Resposta:** `{session, observations, total, returned, offset, limit, has_more, body_max_chars, elided_other_scope, scope, warnings}`.

### 3.5 `memory_status` (leitura) — MS §3.2
- **Argumentos:** `workspace`, `project` (opcionais).
- **Resposta:** `{workspace, project, pages, sessions, observations, open_handoffs, purged_sessions, purged_scopes, write_queue_depth, data_dir, version, hook_metrics, scope, warnings}`.

### 3.6 `memory_briefing` (leitura) — MS §3.2
- **Argumentos:** `workspace`, `project`, `settled_first` (bool, padrão `false`).
- **Resposta:** `{workspace, project, counts, activity, rules, pinned?, settled?, slots, recent_pages, scope, warnings}` (`pinned` ≤ 10 excluindo `_slots/*`; `settled` ≤ 8 quando `settled_first=true`).

### 3.7 `memory_handoff_list` (leitura) — MS §3.3
- **Argumentos:** `workspace`, `project`, `state` (`open|accepted|expired|all`, padrão `open`), `limit` (padrão 20, clamp 1..100), `all_owners` (bool, só administrador: `-32600` se não admin).
- **Resposta:** `{handoffs, scope, warnings}` ordenado por `created_at DESC`.

### 3.8 `memory_write_page` (escrita) — MS §3.3
- **Argumentos:** `path`, `body` (alias `content`), `title`, `tier` (padrão `"episodic"`), `tags`, `pinned`, `expires_at`, `session_id`, `expected_page_id` (`-32600` em divergência), `create_only` (`-32600` se já existir), campos OKF (`kind`, `entities`, `abstract`, `relations`, `applies_to`, `enforced_by`), `workspace`, `project`, `scope` (`"global"` ou `"profile"`).
- **Commits:** tenta `pre-checkpoint before mcp write-page <path>` antes e `mcp write-page <path>` depois (falhas viram `warnings`).
- **Resposta:** `{id, path, version, superseded_id, pre_checkpoint, checkpoint, scope, warnings}`.

### 3.9 `memory_delete_page` (escrita) — MS §3.3
- **Argumentos:** `path`, `expected_page_id` (opcional; `-32600` em divergência), `workspace`, `project`, `scope` (apenas `"profile"` é aceito na validação; `"global"` retorna `-32602`).
- **Comportamento:** remove fisicamente todas as versões do caminho, limpa entidades órfãs e grava `audit_log` (`delete_page`). Sem `expected_page_id`, é idempotente e retorna sempre `deleted: true`.
- **Resposta:** `{deleted: true, path, pre_checkpoint, checkpoint, scope, warnings}`.

### 3.10 `memory_handoff_begin` (escrita) — MS §3.3
- **Argumentos:** `summary` (obrigatório, ≤ 3000), `open_questions`, `next_steps` (item ≤ 1500, total ≤ 6000, ≤ 20 itens), `files_touched` (item ≤ 512, total ≤ 4096, ≤ 20 itens), `from_agent` (padrão `"mcp"`), `cwd`, `shared` (bool), `workspace`, `project`.
- **Resposta:** `{handoff_id, state: "open", created_at, scope, warnings}`.

### 3.11 `memory_handoff_accept` (escrita) — MS §3.3
- **Argumentos:** `handoff_id` (opcional), `cwd` (opcional), `by_agent` (padrão `"mcp"`), `by_session` (opcional), `workspace`, `project`.
- **Comportamento:** sem `handoff_id`, seleciona com filtro de sessão viva (10 min); com `handoff_id`, busca aquele id aberto **sem** filtro de sessão viva (`busy_since = None`). Executa varredura pós-reivindicação sempre que o handoff reivindicado for automático.
- **Resposta:** `{handoff, status: "claimed" | "consumed_by_hook" | "none_pending", scope, warnings}`.

### 3.12 `memory_handoff_cancel` (escrita) — MS §3.3
- **Argumentos:** `handoff_id` (obrigatório), `all_owners` (bool, só admin), `workspace`, `project`.
- **Resposta:** `{handoff_id, cancelled: bool, scope, warnings}`.

## 4. Formato de erro e Autenticação HTTP (`/mcp`)
- Erros das tools usam códigos JSON-RPC (`-32602`, `-32600`, `-32603`).
- Autenticação Bearer em tempo constante (comparando hashes SHA-256 dos tokens); em falha, `401 Unauthorized` com `WWW-Authenticate: Bearer` e corpo JSON `{"error": "unauthorized: missing or invalid Authorization: Bearer <token> header"}`. Filtro de `Host` na camada mais externa (cobrindo inclusive `/healthz`: 400 sem `Host`, 403 para `Host` não permitido).
