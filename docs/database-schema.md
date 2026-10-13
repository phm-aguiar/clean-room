# Esquema de armazenamento (estado final)

> **Escopo:** este documento contém partes que dependem de LLM ou de embeddings. Essas partes são opcionais e não fazem parte do escopo inicial, que funciona sem LLM. Podem ser removidas ou adiadas junto com as funções correspondentes.

> Referência do **esquema de dados** para quem vai reimplementar o armazenamento. Apresenta o
> resultado consolidado de todas as migrações (versões 1 a 76, aplicadas em ordem), sem repetir
> reconstruções de tabela nem reparos de dados. Nomes de tabela e de coluna são contrato.
> Para a lógica que usa essas tabelas, ver [storage-and-retrieval.md](storage-and-retrieval.md).

> **Estado:** o banco é um **índice derivado** da wiki em markdown (fonte da verdade), exceto o que
> só existe no banco: sessões, observações, handoffs, mensagens, usuários e credenciais, auditoria,
> embeddings, feedback, propostas de auto-melhoria, workstreams, fila de consolidação e tombstones.
> Isso define o que a reindexação reconstrói e o que exige backup do arquivo do banco.

## 0. Convenções

| Convenção | Valor |
|---|---|
| IDs | `BLOB` de 16 bytes (UUIDv7), exceto onde indicado: `INTEGER AUTOINCREMENT` em `audit_log`, `profile_candidates` e `auto_improve_proposal_events`; `INTEGER` com `CHECK (id = 1)` em `human_auth_state` (singleton) |
| Timestamps | `INTEGER` em **microssegundos** desde a época Unix (UTC). Exceções: `client_activity.day` = dias UTC desde a época; `workstream_events.occurred_at` = `TEXT` |
| Hashes | `BLOB` de 32 bytes (SHA-256): `body_sha256`, `token_hash`, `session_hash`, `csrf_hash` |
| Booleanos | `INTEGER` com `CHECK (x IN (0,1))` |
| Listas/objetos | `TEXT` com JSON (padrões `'[]'` e `'{}'`) |
| Texto de dono | `owner_user`, `actor_user`, `accepted_by_user` e similares são `TEXT`: a chave de armazenamento da identidade do operador. **Não** são chave estrangeira para `users` |
| Pragmas na abertura | `journal_mode=WAL`, `synchronous=NORMAL`, `busy_timeout=5000` (ms), `foreign_keys=ON` em execução e **OFF durante as migrações** (que reconstroem tabelas) |
| Migrações | Aplicadas em ordem numérica, cada uma em uma transação. O histórico fica na tabela de controle `refinery_schema_history` (colunas: `version`, `name`, `applied_on`, `checksum`). Uma migração aplicada que o binário não conhece faz a abertura ser **recusada** com erro de schema mais novo que o binário. Checksum divergente de migração já aplicada é **tolerado** (a versão 62 foi reduzida a DDL; o preenchimento de janelas roda na abertura, ver [temporal.md](temporal.md)) |
| Backfills na abertura | Após as migrações, rodam backfills idempotentes e de execução única antes de aceitar tráfego: índice de entidades a partir do frontmatter, janelas de ingestão das páginas (em lotes, com checkpoint de WAL) e chaves de coordenada dos projetos. Em um banco já preenchido são no-ops |
| Escritor | **Uma** thread/conexão de escrita, alimentada por fila de comandos com capacidade de 1024 mensagens; pool de conexões somente leitura com limite flexível de 4 |

> **Requisito:** SQLite com o módulo de busca de texto completo FTS5. Decisão a validar: nomes de
> tecnologia de banco (SQLite, FTS5, WAL, tokenizer `unicode61`, `AUTOINCREMENT`, `WITHOUT ROWID`,
> e o nome da tabela de controle de migrações) são mantidos como parte do contrato.

### 0.1 Trigger de pareamento

Toda tabela com `workspace_id` **e** `project_id` tem um trigger `BEFORE INSERT` que aborta se o
workspace informado não é o workspace do projeto:

```sql
CREATE TRIGGER <tabela>_ws_proj_pairing_ai BEFORE INSERT ON <tabela>
FOR EACH ROW
WHEN NEW.workspace_id IS NOT (SELECT workspace_id FROM projects WHERE id = NEW.project_id)
BEGIN SELECT RAISE(ABORT, '<tabela>.workspace_id does not match the project''s workspace'); END;
```

Aplicado a: `pages`, `sessions`, `observations`, `handoffs`, `auto_improve_runs`,
`auto_improve_proposals`, `auto_improve_rejections`, `auto_improve_scheduler_state`,
`auto_improve_scheduler_claims` e `workstreams`.

Variantes:
- `entities` usa a variante com `NOT EXISTS`: o workspace informado precisa ser o do projeto.
- `entity_page_links` usa a mesma variante: a entidade e a página precisam ter o mesmo workspace e o
  mesmo projeto.
- Tabelas filhas de sessão (`auto_improve_scheduler_claims` e `session_consolidation_jobs`) têm um
  segundo trigger que exige que `workspace_id` e `project_id` coincidam com os da **sessão**.

---

## 1. Núcleo: escopo, páginas, links

```sql
CREATE TABLE workspaces (
  id BLOB PRIMARY KEY NOT NULL,
  name TEXT NOT NULL UNIQUE,
  created_at INTEGER NOT NULL
);

CREATE TABLE projects (
  id BLOB PRIMARY KEY NOT NULL,
  workspace_id BLOB NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
  name TEXT NOT NULL,
  repo_path TEXT,                       -- caminho absoluto, usado para casar o diretório de trabalho
  created_at INTEGER NOT NULL,
  -- autorização por projeto
  access_mode TEXT NOT NULL DEFAULT 'open' CHECK (access_mode IN ('open','restricted')),
  created_by BLOB REFERENCES users(id) ON DELETE SET NULL,
  -- identidade de repositório
  identity TEXT NOT NULL DEFAULT '',
  identity_source TEXT NOT NULL DEFAULT '',   -- explicit|manifest|git_remote|folder_name
  -- perfil entre projetos
  profile_contribute INTEGER NOT NULL DEFAULT 1 CHECK (profile_contribute IN (0,1)),
  profile_consume    INTEGER NOT NULL DEFAULT 1 CHECK (profile_consume IN (0,1)),
  -- chaves de coordenada
  canonical_name TEXT NOT NULL DEFAULT '',
  legacy_name    TEXT NOT NULL DEFAULT '',
  UNIQUE (workspace_id, name)
);
CREATE INDEX idx_projects_workspace ON projects(workspace_id);
CREATE UNIQUE INDEX idx_projects_identity ON projects(workspace_id, identity) WHERE identity <> '';
CREATE INDEX idx_projects_canonical_name ON projects(workspace_id, canonical_name) WHERE canonical_name <> '';
CREATE INDEX idx_projects_legacy_name    ON projects(workspace_id, legacy_name)    WHERE legacy_name <> '';
CREATE INDEX idx_projects_coordinate_backfill ON projects(identity_source)
  WHERE identity_source = 'git_remote' AND identity <> '' AND (canonical_name = '' OR legacy_name = '');
```

Semântica das colunas de projeto: `identity` é única por workspace quando não vazia. Uma identidade de
origem `explicit` ou `git_remote` é a que roteia a captura. `canonical_name` é a chave no estilo de
caminho (por exemplo `acme/api`) e `legacy_name` é o nome legado; as duas servem para casar clientes de
versões diferentes. `profile_contribute` e `profile_consume` espelham a seção `[profile]` do arquivo de
configuração do projeto.

```sql
CREATE TABLE pages (
  id BLOB PRIMARY KEY NOT NULL,
  workspace_id BLOB NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
  project_id   BLOB NOT NULL REFERENCES projects(id)   ON DELETE CASCADE,
  path TEXT NOT NULL,                     -- relativo à raiz da wiki, separador POSIX
  title TEXT NOT NULL,
  tier TEXT NOT NULL CHECK (tier IN ('working','episodic','semantic','procedural')),
  body TEXT NOT NULL,
  body_sha256 BLOB NOT NULL,
  frontmatter_json TEXT NOT NULL DEFAULT '{}',
  is_latest INTEGER NOT NULL DEFAULT 1 CHECK (is_latest IN (0,1)),
  supersedes BLOB REFERENCES pages(id) ON DELETE SET NULL,
  pinned INTEGER NOT NULL DEFAULT 0 CHECK (pinned IN (0,1)),
  created_at INTEGER NOT NULL,
  updated_at INTEGER NOT NULL,
  embedding_provider TEXT, embedding_model TEXT, embedding_dim INTEGER,
  -- decaimento
  last_accessed_at INTEGER, access_count INTEGER NOT NULL DEFAULT 0, superseded_at INTEGER,
  -- autoria
  author_id BLOB REFERENCES users(id) ON DELETE SET NULL,
  -- busca por caminho
  path_search TEXT NOT NULL DEFAULT '',
  -- expiração (TTL por página) e saliência (feedback)
  expires_at INTEGER, salience REAL,
  -- janela temporal de validade (as-of)
  valid_from INTEGER, valid_to INTEGER,
  -- marcador de compactação
  compacted_at INTEGER
);
CREATE UNIQUE INDEX idx_pages_latest_path ON pages(workspace_id, project_id, path) WHERE is_latest = 1;
CREATE INDEX idx_pages_updated ON pages(updated_at DESC);
CREATE INDEX idx_pages_supersedes ON pages(supersedes) WHERE supersedes IS NOT NULL;
CREATE INDEX idx_pages_decay ON pages(workspace_id, project_id, updated_at) WHERE is_latest = 1;
CREATE INDEX idx_pages_evicted ON pages(workspace_id, project_id, superseded_at)
  WHERE is_latest = 0 AND superseded_at IS NOT NULL;
CREATE INDEX idx_pages_project ON pages(project_id);
CREATE INDEX idx_pages_latest_updated ON pages(updated_at DESC) WHERE is_latest = 1;
CREATE INDEX idx_pages_project_latest_updated ON pages(workspace_id, project_id, updated_at DESC) WHERE is_latest = 1;
CREATE INDEX idx_pages_author ON pages(author_id, updated_at DESC) WHERE author_id IS NOT NULL;
CREATE INDEX idx_pages_expiry ON pages(expires_at) WHERE is_latest = 1 AND expires_at IS NOT NULL;
CREATE INDEX idx_pages_validity ON pages(workspace_id, project_id, valid_from, valid_to);
```

**Regras que o esquema e a escrita impõem:**
- **Uma versão corrente por caminho** (índice único parcial sobre `is_latest = 1`).
- **Supersessão** (escrita de uma nova versão de um caminho que já tem versão corrente, na mesma
  transação): a versão nova recebe `supersedes` apontando para a anterior; a anterior passa a
  `is_latest = 0` e recebe `valid_to` igual ao instante da nova versão; as janelas abertas das
  ligações de entidade dessa versão são fechadas (`entity_page_links.superseded_at`) no mesmo
  instante. A cadeia **nunca é destruída**: a versão antiga continua alcançável.
- `superseded_at` **não** marca supersessão. Ele é a marca de **lápide de decaimento** (página
  removida pelo varredor de esquecimento) e é a única coluna escrita por esse caminho.
- `path_search` é derivado do caminho para a busca. Calcule-o assim: a) troque `/` e `.` por espaço,
  produzindo `segmentos`; b) troque também `-` e `_` por espaço em `segmentos`, produzindo
  `palavras`; c) grave `segmentos || ' ' || palavras`. Exemplo: `notes/foo-bar.md` vira
  `notes foo-bar md notes foo bar md`. Recalcule a cada mudança de `path`.

### 1.1 Índice de texto das páginas (FTS5)

```sql
CREATE VIRTUAL TABLE pages_fts USING fts5(
  title, body, path_search,
  content='pages', content_rowid='rowid',
  tokenize="unicode61 remove_diacritics 2 tokenchars '/_-'"
);
CREATE TRIGGER pages_fts_ai AFTER INSERT ON pages BEGIN
  INSERT INTO pages_fts(rowid,title,body,path_search) VALUES (new.rowid,new.title,new.body,new.path_search);
END;
CREATE TRIGGER pages_fts_ad AFTER DELETE ON pages BEGIN
  INSERT INTO pages_fts(pages_fts,rowid,title,body,path_search)
  VALUES ('delete',old.rowid,old.title,old.body,old.path_search);
END;
CREATE TRIGGER pages_fts_au AFTER UPDATE OF title, body, path_search ON pages BEGIN
  INSERT INTO pages_fts(pages_fts,rowid,title,body,path_search)
  VALUES ('delete',old.rowid,old.title,old.body,old.path_search);
  INSERT INTO pages_fts(rowid,title,body,path_search) VALUES (new.rowid,new.title,new.body,new.path_search);
END;
```

Tokenizer: `unicode61`, **sem diacríticos** (`remove_diacritics 2`), com `/`, `_` e `-` como caracteres
de token. Assim `nome-composto` e `snake_case` viram um termo só. A tabela é de conteúdo externo
(`content='pages'`): os três triggers são obrigatórios. O trigger de atualização dispara apenas quando
muda uma das três colunas indexadas. Se o tokenizer mudar, reconstrua o índice com o comando de
`rebuild` do FTS5.

```sql
CREATE TABLE links (
  from_page_id BLOB NOT NULL REFERENCES pages(id) ON DELETE CASCADE,
  to_page_id   BLOB REFERENCES pages(id) ON DELETE SET NULL,  -- NULL = link pendente
  to_workspace TEXT,                      -- NULL = workspace da página de origem
  to_project   TEXT,                      -- NULL = projeto da página de origem
  to_path      TEXT NOT NULL,
  link_type    TEXT NOT NULL DEFAULT 'references',   -- references|causes|fixes|contradicts
  PRIMARY KEY (from_page_id, to_workspace, to_project, to_path, link_type)
);
CREATE INDEX idx_links_to ON links(to_page_id) WHERE to_page_id IS NOT NULL;
CREATE INDEX idx_links_unresolved_path ON links(to_path) WHERE to_page_id IS NULL;
CREATE INDEX idx_links_to_path ON links(to_path);
CREATE INDEX idx_links_scope ON links(to_project, to_path);
```

> Atenção: `NULL` em colunas de chave primária nunca se iguala a outro `NULL` no SQLite, então a chave
> primária **não** deduplica links com `to_workspace` ou `to_project` nulos. Os links de uma página são
> regravados (apagar e inserir) a cada nova versão, na mesma transação da página. A deduplicação é
> responsabilidade da aplicação, aplicada antes da inserção pela chave lógica
> `(to_workspace, to_project, to_path, link_type)`. Links locais ao projeto usam `to_workspace = NULL`;
> `to_project` fica `NULL` (mesmo projeto) ou recebe o nome do projeto de destino (por exemplo, o escopo
> reservado `_global`).

---

## 2. Sessões e observações

```sql
CREATE TABLE sessions (
  id BLOB PRIMARY KEY NOT NULL,
  workspace_id BLOB NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
  project_id   BLOB NOT NULL REFERENCES projects(id)   ON DELETE CASCADE,
  agent_kind TEXT NOT NULL CHECK (agent_kind IN (
    'claude-code','codex','open-code','cursor','gemini-cli','claude-desktop','openclaw',
    'antigravity-cli','omp','pi','crush','grok','zero','devin','kimi-code','kiro-cli',
    'command-code','hermes','pool','zcode','grizzybot','copilot-cli','other')),
  cwd TEXT,
  started_at INTEGER NOT NULL,
  ended_at INTEGER,
  summary_page_id BLOB REFERENCES pages(id) ON DELETE SET NULL,
  ended_observation_count INTEGER NOT NULL DEFAULT 0 CHECK (ended_observation_count >= 0),
  actor_user TEXT
);
CREATE INDEX idx_sessions_recent ON sessions(workspace_id, project_id, started_at DESC);
CREATE INDEX idx_sessions_project ON sessions(project_id);
CREATE INDEX idx_sessions_started_at ON sessions(started_at DESC);
CREATE INDEX idx_sessions_scope_ended ON sessions(workspace_id, project_id, ended_at ASC) WHERE ended_at IS NOT NULL;
CREATE INDEX idx_sessions_open_owner ON sessions(workspace_id, project_id, agent_kind, actor_user) WHERE ended_at IS NULL;
```

> **O `CHECK` de `agent_kind` é fechado.** Cada novo tipo de agente exige uma migração que reconstrói a
> tabela (versões 9, 11, 20, 25, 26, 28, 30, 32, 44, 45, 47, 50, 51, 72 e 73). A aplicação mapeia
> qualquer tipo desconhecido para `'other'` (fronteira tolerante). Em uma reimplementação, prefira
> **não** colocar o `CHECK` (use a validação na aplicação) ou aceite que adicionar um tipo de agente é
> uma migração de reconstrução.

`ended_observation_count` registra quantas observações da sessão estavam cobertas pelo último encerramento.
Na atualização para a versão 35, as sessões já encerradas receberam como linha de base a contagem atual de
observações, para não gerar um backlog de consolidação no upgrade.

```sql
CREATE TABLE observations (
  id BLOB PRIMARY KEY NOT NULL,
  session_id BLOB NOT NULL REFERENCES sessions(id) ON DELETE CASCADE,
  workspace_id BLOB NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
  project_id   BLOB NOT NULL REFERENCES projects(id)   ON DELETE CASCADE,
  kind TEXT NOT NULL,                     -- tipo da observação (texto livre no esquema)
  title TEXT NOT NULL,
  body  TEXT NOT NULL,
  importance INTEGER NOT NULL DEFAULT 5 CHECK (importance BETWEEN 1 AND 10),
  created_at INTEGER NOT NULL,
  extension TEXT,                         -- namespace do produtor externo
  source_event TEXT                       -- nome do evento de origem
);
CREATE INDEX idx_observations_session ON observations(session_id, created_at);
CREATE INDEX idx_observations_project ON observations(project_id);
CREATE INDEX idx_observations_created_at ON observations(created_at DESC);
CREATE INDEX idx_observations_project_created ON observations(workspace_id, project_id, created_at DESC);
CREATE INDEX idx_observations_extension_event
  ON observations(workspace_id, project_id, extension, source_event, created_at)
  WHERE extension IS NOT NULL AND source_event IS NOT NULL;

CREATE VIRTUAL TABLE observations_fts USING fts5(
  title, body, content='observations', content_rowid='rowid',
  tokenize="unicode61 remove_diacritics 2 tokenchars '/_-'");
-- triggers observations_fts_ai/ad/au: mesma forma dos de pages_fts, sobre título e corpo.
-- O trigger de atualização de observations_fts dispara em qualquer coluna da linha.
```

```sql
CREATE TABLE ingest_keys (                 -- idempotência de ingestão (retenção de 30 dias)
  project_id BLOB NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
  key TEXT NOT NULL,
  seen_at INTEGER NOT NULL,
  completed_at INTEGER,                    -- NULL = reivindicada, com efeitos a jusante pendentes
  PRIMARY KEY (project_id, key)
) WITHOUT ROWID;
CREATE INDEX idx_ingest_keys_seen_at ON ingest_keys(seen_at);

CREATE TABLE session_consolidation_jobs (  -- fila durável de consolidação do encerramento de sessão
  session_id BLOB NOT NULL REFERENCES sessions(id) ON DELETE CASCADE,
  workspace_id BLOB NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
  project_id   BLOB NOT NULL REFERENCES projects(id)   ON DELETE CASCADE,
  generation INTEGER NOT NULL CHECK (generation > 0),
  state TEXT NOT NULL CHECK (state IN ('pending','running','completed','failed','superseded')),
  requested_at INTEGER NOT NULL,
  next_attempt_at INTEGER NOT NULL,
  started_at INTEGER, completed_at INTEGER,
  attempts INTEGER NOT NULL DEFAULT 0 CHECK (attempts >= 0),
  claim_id BLOB, last_error TEXT,
  PRIMARY KEY (session_id, generation)
) WITHOUT ROWID;
CREATE INDEX idx_session_consolidation_jobs_due ON session_consolidation_jobs(state, next_attempt_at, requested_at);
```

**Fila de consolidação (comportamento):**
- A `generation` de um job é a **contagem de observações** da sessão no momento do enfileiramento. O
  enfileiramento só cria trabalho se a sessão estiver encerrada e tiver ao menos uma observação. Se a
  contagem não mudou desde o último job, a chave primária `(session_id, generation)` impede a duplicata:
  um novo encerramento sem observações novas não cria job.
- Um job `pending` mais novo **supersede** os `pending` mais antigos da mesma sessão (estado
  `superseded`, com `last_error` explicando o motivo).
- Número máximo de tentativas por geração: **5** (`attempts < 5` para reivindicar um job pendente).
- Um job `running` cuja reivindicação expirou (`started_at` antigo) pode ser reivindicado de novo.

---

## 3. Handoffs e mensagens entre projetos

```sql
CREATE TABLE handoffs (
  id BLOB PRIMARY KEY NOT NULL,
  workspace_id BLOB NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
  project_id   BLOB NOT NULL REFERENCES projects(id)   ON DELETE CASCADE,
  from_session_id BLOB REFERENCES sessions(id) ON DELETE SET NULL,   -- NULL = manual
  from_agent TEXT NOT NULL,
  to_agent TEXT,                          -- dica de destino opcional (ver nota abaixo)
  cwd TEXT,
  summary TEXT NOT NULL,
  open_questions TEXT NOT NULL DEFAULT '[]',   -- array JSON de strings
  next_steps     TEXT NOT NULL DEFAULT '[]',
  files_touched  TEXT NOT NULL DEFAULT '[]',
  state TEXT NOT NULL DEFAULT 'open' CHECK (state IN ('open','accepted','expired')),
  created_at INTEGER NOT NULL,
  accepted_by TEXT, accepted_at INTEGER,
  accepted_by_session BLOB REFERENCES sessions(id) ON DELETE SET NULL,
  owner_user TEXT,
  accepted_by_user TEXT
);
CREATE INDEX idx_handoffs_open_recent ON handoffs(workspace_id, project_id, created_at DESC) WHERE state = 'open';
CREATE INDEX idx_handoffs_cwd_open ON handoffs(workspace_id, project_id, cwd, created_at DESC) WHERE state = 'open';
CREATE INDEX idx_handoffs_project ON handoffs(project_id);
CREATE INDEX idx_handoffs_open_owner ON handoffs(workspace_id, project_id, owner_user, created_at DESC) WHERE state = 'open';
CREATE INDEX idx_handoffs_project_recent ON handoffs(workspace_id, project_id, created_at DESC);
CREATE INDEX idx_handoffs_project_owner_recent ON handoffs(workspace_id, project_id, owner_user, created_at DESC);
CREATE INDEX idx_handoffs_accepted_session ON handoffs(accepted_by_session, accepted_at DESC)
  WHERE state = 'accepted' AND accepted_by_session IS NOT NULL;
```

**Nota sobre `to_agent`:** é uma dica de destino opcional e tem o mesmo domínio de tipos de agente da
sessão. Os caminhos de criação atuais gravam `NULL`. Quando há valor, a leitura o devolve junto do
handoff. Nenhum filtro de listagem ou de aceite usa essa coluna.

**Aceite (reivindicação única):** o aceite é compare-and-set. A transição `open → accepted` só acontece
por um `UPDATE ... WHERE id = ? AND workspace_id = ? AND project_id = ? AND state = 'open'`, com o filtro
de dono dentro do mesmo `WHERE` quando houver. Antes disso, a aplicação confere que a sessão receptora
existe, tem o mesmo escopo e o mesmo tipo de agente, e que ela ainda não aceitou outro handoff (uma sessão
nunca consome dois batões). Quem perde a corrida recebe "não aceito". Detalhes em
[storage-and-retrieval.md](storage-and-retrieval.md) §7.3.

**Expiração:** a inserção de um handoff **automático** (com sessão de origem) expira (estado `expired`)
os handoffs automáticos abertos que estejam no **mesmo diretório exato**, sejam do **mesmo
dono** e não pertençam a sessões ainda abertas (essas cuidam do próprio handoff). A própria sessão também
é preservada. Handoffs **manuais** (sem sessão de origem) são de projeto inteiro e não expiram outros.

**Limites de conteúdo:** campos de texto são truncados em **16 KiB** (por bytes UTF-8, sem quebrar
caractere) e listas (`open_questions`, `next_steps`, `files_touched`) são limitadas a **256** itens.
As mensagens entre projetos usam o mesmo limite de campo.

```sql
CREATE TABLE agent_messages (               -- caixa de entrada entre projetos
  id BLOB PRIMARY KEY NOT NULL,
  from_workspace_id BLOB NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
  from_project_id   BLOB NOT NULL REFERENCES projects(id)   ON DELETE CASCADE,
  from_agent TEXT NOT NULL,
  from_session_id BLOB REFERENCES sessions(id) ON DELETE SET NULL,
  from_owner_user TEXT,                     -- só atribuição; nunca filtro de leitura
  to_workspace_id BLOB NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
  to_project_id   BLOB NOT NULL REFERENCES projects(id)   ON DELETE CASCADE,
  subject TEXT, body TEXT NOT NULL,
  state TEXT NOT NULL DEFAULT 'pending' CHECK (state IN ('pending','claimed','cancelled')),
  created_at INTEGER NOT NULL,
  claimed_at INTEGER,
  claimed_by_session BLOB REFERENCES sessions(id) ON DELETE SET NULL,
  claimed_by_agent TEXT, claimed_by_user TEXT
);
CREATE INDEX idx_agent_messages_inbox  ON agent_messages(to_workspace_id, to_project_id, created_at) WHERE state='pending';
CREATE INDEX idx_agent_messages_outbox ON agent_messages(from_workspace_id, from_project_id, created_at) WHERE state='pending';
```

---

## 4. Embeddings, feedback, entidades, evidência

```sql
CREATE TABLE page_embeddings (
  page_id BLOB PRIMARY KEY NOT NULL REFERENCES pages(id) ON DELETE CASCADE,
  vector BLOB NOT NULL,                    -- float32 little-endian empacotado (dim*4 bytes)
  provider TEXT NOT NULL, model TEXT NOT NULL,
  dim INTEGER NOT NULL CHECK (dim > 0),
  created_at INTEGER NOT NULL
);
CREATE INDEX idx_page_embeddings_provider ON page_embeddings(provider, model, dim);

CREATE TABLE page_abstract_embeddings (   -- embedding do resumo da página (mesma forma)
  page_id BLOB PRIMARY KEY NOT NULL REFERENCES pages(id) ON DELETE CASCADE,
  vector BLOB NOT NULL, provider TEXT NOT NULL, model TEXT NOT NULL,
  dim INTEGER NOT NULL CHECK (dim > 0), created_at INTEGER NOT NULL
);
CREATE INDEX idx_page_abstract_embeddings_provider ON page_abstract_embeddings(provider, model, dim);

CREATE TABLE page_embed_failures (        -- evita recalcular embeddings que já falharam
  page_id BLOB PRIMARY KEY NOT NULL REFERENCES pages(id) ON DELETE CASCADE,
  at INTEGER NOT NULL,
  outcome TEXT NOT NULL CHECK (outcome IN ('failed','unreadable','skipped_empty')),
  detail TEXT
);
CREATE INDEX idx_page_embed_failures_at ON page_embed_failures(at DESC);
```

O vetor é `float32` little-endian contíguo (comprimento = `dim * 4` bytes). A busca vetorial é **força
bruta**: a consulta lê todos os vetores do projeto com a mesma tupla `(provider, model, dim)` e calcula o
produto escalar. Como os vetores são de norma 1, o produto escalar equivale ao cosseno. Não há índice
vetorial dedicado. Vetores de modelos ou
dimensões diferentes nunca são comparados: a tupla fica ao lado do vetor para isso.

```sql
CREATE TABLE page_feedback (
  id BLOB PRIMARY KEY,
  page_id BLOB NOT NULL REFERENCES pages(id) ON DELETE CASCADE,
  workspace_id BLOB NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
  project_id   BLOB NOT NULL REFERENCES projects(id)   ON DELETE CASCADE,
  kind TEXT NOT NULL CHECK (kind IN ('helpful','not_helpful','stale','wrong')),
  reason TEXT,
  salience_after REAL NOT NULL CHECK (salience_after BETWEEN 0.25 AND 2.0),
  author_id BLOB REFERENCES users(id) ON DELETE SET NULL,
  created_at INTEGER NOT NULL
);
CREATE INDEX idx_page_feedback_page ON page_feedback(page_id, created_at DESC);
CREATE INDEX idx_page_feedback_scope ON page_feedback(workspace_id, project_id, created_at DESC);
```

**Saliência (regra de atualização):** a saliência fica entre `0.25` e `2.0`, com passo de `0.25`. Um
feedback `helpful` soma o passo; `not_helpful` subtrai. `stale` e `wrong` levam direto ao piso (`0.25`),
porque afirmam que o conteúdo não é mais confiável. Página sem feedback anterior parte do valor padrão de
saliência (`1.0`). O valor resultante é gravado em `salience_after`.

```sql
CREATE TABLE page_access (                 -- quem já leu a página (uma linha por página e operador)
  page_id BLOB NOT NULL REFERENCES pages(id) ON DELETE CASCADE,
  actor TEXT NOT NULL,
  PRIMARY KEY (page_id, actor)
) WITHOUT ROWID;

CREATE TABLE page_evidence (               -- de onde veio o conhecimento da página
  page_id BLOB NOT NULL REFERENCES pages(id) ON DELETE CASCADE,
  source_kind TEXT NOT NULL CHECK (source_kind IN ('session','observation','feedback','reconsolidation')),
  source_id TEXT NOT NULL,
  created_at INTEGER NOT NULL,
  PRIMARY KEY (page_id, source_kind, source_id)
) WITHOUT ROWID;

CREATE TABLE entities (
  id BLOB PRIMARY KEY,
  workspace_id BLOB NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
  project_id   BLOB NOT NULL REFERENCES projects(id)   ON DELETE CASCADE,
  name TEXT NOT NULL CHECK (length(name) BETWEEN 1 AND 64),
  created_at INTEGER NOT NULL,
  UNIQUE (workspace_id, project_id, name)
);
CREATE INDEX idx_entities_name ON entities(workspace_id, project_id, name);

CREATE TABLE entity_page_links (
  entity_id BLOB NOT NULL REFERENCES entities(id) ON DELETE CASCADE,
  page_id   BLOB NOT NULL REFERENCES pages(id)    ON DELETE CASCADE,
  valid_from INTEGER,                      -- instante de criação da versão da página
  superseded_at INTEGER,                   -- instante de criação da versão que a substituiu (NULL = janela aberta)
  PRIMARY KEY (entity_id, page_id)
);
CREATE INDEX idx_entity_page_links_page ON entity_page_links(page_id);
CREATE INDEX idx_entity_page_links_validity ON entity_page_links(entity_id, valid_from, superseded_at);
-- trigger BEFORE INSERT: entidade e página precisam ter o mesmo workspace e projeto
```

**Janelas de entidade:** na escrita, `valid_from` recebe o instante de criação da versão da página e
`superseded_at` é preenchido quando a versão é substituída, ou quando a página é aposentada sem sucessora
(decaimento, purga com regeração ou fusão). Na atualização para a versão 58, janelas abertas de versões
aposentadas sem sucessora foram fechadas com o `updated_at` da página.

---

## 5. Usuários, credenciais e sessões web

```sql
CREATE TABLE users (
  id BLOB NOT NULL PRIMARY KEY,
  username TEXT NOT NULL UNIQUE,
  name TEXT,
  email TEXT UNIQUE COLLATE NOCASE,
  token_hash BLOB UNIQUE,                  -- legado; SHA-256(token || ":" || pepper), 32 bytes
  created_at INTEGER NOT NULL,
  last_seen_at INTEGER,
  token_expired_at INTEGER,
  role TEXT NOT NULL DEFAULT 'user' CHECK (role IN ('root','user')),
  password_hash TEXT,                      -- Argon2id (formato PHC)
  must_change_password INTEGER NOT NULL DEFAULT 0 CHECK (must_change_password IN (0,1)),
  disabled_at INTEGER
);
-- triggers users_token_hash_length_ai/au: token_hash, se não nulo, tem exatamente 32 bytes
-- triggers users_token_hash_mirror_ai/au: espelham o token_hash legado em api_credentials
--   (id = id do usuário, label 'legacy-user-token'; se a linha já existe, atualiza o hash e a
--   revogação com a data de expiração do token)

CREATE TABLE human_auth_state (            -- singleton
  id INTEGER PRIMARY KEY NOT NULL CHECK (id = 1),
  bootstrap_completed INTEGER NOT NULL DEFAULT 0 CHECK (bootstrap_completed IN (0,1))
);
INSERT INTO human_auth_state(id, bootstrap_completed) VALUES (1, 0);

CREATE TABLE web_sessions (                -- login humano por cookie
  id BLOB NOT NULL PRIMARY KEY,
  session_hash BLOB NOT NULL UNIQUE CHECK (length(session_hash) = 32),
  user_id BLOB NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  csrf_hash BLOB NOT NULL CHECK (length(csrf_hash) = 32),
  created_at INTEGER NOT NULL, last_used_at INTEGER NOT NULL,
  expires_at INTEGER NOT NULL, revoked_at INTEGER
);
CREATE INDEX idx_web_sessions_user ON web_sessions(user_id);
CREATE INDEX idx_web_sessions_expires ON web_sessions(expires_at);

CREATE TABLE api_credentials (             -- tokens de API (prefixo aim_)
  id BLOB NOT NULL PRIMARY KEY,
  user_id BLOB NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  label TEXT NOT NULL,
  token_hash BLOB NOT NULL UNIQUE CHECK (length(token_hash) = 32),
  preview TEXT,                            -- trecho mascarado para listagem
  created_at INTEGER NOT NULL, last_used_at INTEGER, expires_at INTEGER, revoked_at INTEGER
);
CREATE INDEX idx_api_credentials_user ON api_credentials(user_id);

CREATE TABLE project_grants (              -- acesso por projeto restrito
  workspace_id BLOB NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
  project_id   BLOB NOT NULL REFERENCES projects(id)   ON DELETE CASCADE,
  user_id      BLOB NOT NULL REFERENCES users(id)      ON DELETE CASCADE,
  level TEXT NOT NULL CHECK (level IN ('read','write')),
  granted_by BLOB REFERENCES users(id) ON DELETE SET NULL,
  granted_at INTEGER NOT NULL,
  PRIMARY KEY (workspace_id, project_id, user_id)
) WITHOUT ROWID;
```

**Regras de credenciais:**
- Armazene **somente hashes** de token, sessão e CSRF. Nunca grave o segredo em claro.
- Token de API: prefixo `aim_` seguido de 43 caracteres URL-safe gerados por CSPRNG. Armazena-se
  `SHA-256(token || ":" || pepper)`, com o pepper do servidor. A autenticação por chave de API não
  consulta o `token_hash` legado do usuário.
- Sessão web: o segredo tem a forma de prefixo `ams_` mais valor aleatório e é guardado como SHA-256 puro
  (sem pepper); o CSRF também é guardado como hash. A validade é **absoluta**: `expires_at = created_at +
  7 dias`. Uma sessão só é aceita se não estiver revogada e `expires_at` for maior que o instante atual.
- Senha humana: Argon2id no formato PHC. A computação nunca roda na thread escritora.
- Fluxo de bootstrap, formato completo do hash, pepper e recuperação de root: ver
  [storage-and-retrieval.md](storage-and-retrieval.md) §18 e [users.md](users.md).

---

## 6. Auditoria, atividade e manutenção

```sql
CREATE TABLE audit_log (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  at INTEGER NOT NULL, op TEXT NOT NULL,
  workspace_id BLOB, project_id BLOB, page_id BLOB,
  detail TEXT NOT NULL DEFAULT '{}',
  author_id BLOB REFERENCES users(id) ON DELETE SET NULL
);
CREATE INDEX idx_audit_recent ON audit_log(at DESC);
CREATE INDEX idx_audit_log_author ON audit_log(author_id, at DESC) WHERE author_id IS NOT NULL;

CREATE TABLE client_activity (             -- contadores diários de leitura e escrita por cliente
  client TEXT NOT NULL CHECK (length(client) BETWEEN 1 AND 64),
  day INTEGER NOT NULL,                    -- dias UTC desde a época
  reads  INTEGER NOT NULL DEFAULT 0 CHECK (reads  >= 0),
  writes INTEGER NOT NULL DEFAULT 0 CHECK (writes >= 0),
  PRIMARY KEY (client, day)
) WITHOUT ROWID;

CREATE TABLE maintenance_scheduler_state ( -- último sucesso por job de manutenção
  job TEXT PRIMARY KEY, last_success_at INTEGER NOT NULL);

CREATE TABLE wiki_migrations (             -- migrações de estrutura de arquivos da wiki
  name TEXT PRIMARY KEY, applied_at INTEGER NOT NULL);

CREATE TABLE bootstrap_chunk_progress (    -- retomada do bootstrap assistido por modelo
  fingerprint TEXT NOT NULL, chunk_index INTEGER NOT NULL,
  pages_json TEXT NOT NULL, rationale TEXT NOT NULL, created_at INTEGER NOT NULL,
  PRIMARY KEY (fingerprint, chunk_index));
CREATE INDEX idx_bootstrap_chunk_progress_at ON bootstrap_chunk_progress(created_at);

-- Tombstones: impedem que um hook atrasado "ressuscite" o que foi purgado
CREATE TABLE purged_sessions (
  session_id BLOB PRIMARY KEY NOT NULL,
  workspace_id BLOB NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
  project_id   BLOB NOT NULL REFERENCES projects(id)   ON DELETE CASCADE,
  purged_at INTEGER NOT NULL);
CREATE INDEX idx_purged_sessions_at ON purged_sessions(purged_at);
CREATE TABLE purged_scopes (               -- sem chave estrangeira de propósito: o escopo já foi apagado
  workspace_id BLOB NOT NULL, project_id BLOB NOT NULL, purged_at INTEGER NOT NULL,
  PRIMARY KEY (workspace_id, project_id));
CREATE INDEX idx_purged_scopes_at ON purged_scopes(purged_at);
```

---

## 7. Auto-melhoria

```sql
CREATE TABLE auto_improve_runs (
  id BLOB PRIMARY KEY NOT NULL,
  workspace_id BLOB NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
  project_id   BLOB NOT NULL REFERENCES projects(id)   ON DELETE CASCADE,
  session_id BLOB REFERENCES sessions(id) ON DELETE SET NULL,
  provider TEXT, model TEXT, summary TEXT,
  warnings_json TEXT NOT NULL DEFAULT '[]',
  rejected_candidates_json TEXT NOT NULL DEFAULT '[]',
  config_json TEXT NOT NULL DEFAULT '{}',
  proposal_actor_json TEXT NOT NULL,
  created_at INTEGER NOT NULL);
CREATE INDEX idx_auto_improve_runs_scope_recent ON auto_improve_runs(workspace_id, project_id, created_at DESC);
CREATE INDEX idx_auto_improve_runs_scope_session ON auto_improve_runs(workspace_id, project_id, session_id) WHERE session_id IS NOT NULL;

CREATE TABLE auto_improve_proposals (
  id BLOB PRIMARY KEY NOT NULL,
  run_id BLOB NOT NULL REFERENCES auto_improve_runs(id) ON DELETE CASCADE,
  workspace_id BLOB NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
  project_id   BLOB NOT NULL REFERENCES projects(id)   ON DELETE CASCADE,
  status TEXT NOT NULL CHECK (status IN ('pending','approved','rejected','conflict','failed')),
  operation TEXT NOT NULL CHECK (operation IN ('create','update')),
  target_path TEXT NOT NULL, kind TEXT NOT NULL, title TEXT NOT NULL,
  confidence REAL NOT NULL, rationale TEXT NOT NULL,
  evidence_json TEXT NOT NULL DEFAULT '[]',
  body_markdown TEXT NOT NULL, body_sha256 BLOB NOT NULL,
  artifact_path TEXT NOT NULL, artifact_sha256 BLOB,
  target_latest_page_id_at_stage BLOB REFERENCES pages(id) ON DELETE SET NULL,
  target_body_sha256_at_stage BLOB, target_updated_at_at_stage INTEGER,
  staged_at INTEGER NOT NULL, decided_at INTEGER, decision_reason TEXT,
  decided_by_author_id BLOB REFERENCES users(id) ON DELETE SET NULL,
  decided_by_actor_json TEXT,
  applied_page_id BLOB REFERENCES pages(id) ON DELETE SET NULL,
  checkpoint TEXT,
  edit_mode TEXT NOT NULL DEFAULT 'full_page' CHECK (edit_mode IN ('full_page','patch')),
  patch_json TEXT, expected_base_body_sha256 BLOB, materialized_base_body_sha256 BLOB,
  staged_by_actor_user TEXT);
CREATE INDEX idx_auto_improve_proposals_scope_status_recent ON auto_improve_proposals(workspace_id, project_id, status, staged_at DESC);
CREATE INDEX idx_auto_improve_proposals_run ON auto_improve_proposals(run_id);
-- no máximo UMA proposta pendente por (projeto, alvo, autor)
CREATE UNIQUE INDEX idx_auto_improve_one_pending_target
  ON auto_improve_proposals(workspace_id, project_id, target_path, COALESCE(staged_by_actor_user,''))
  WHERE status = 'pending';

CREATE TABLE auto_improve_proposal_events (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  proposal_id BLOB NOT NULL REFERENCES auto_improve_proposals(id) ON DELETE CASCADE,
  event TEXT NOT NULL, actor_json TEXT NOT NULL,
  author_id BLOB REFERENCES users(id) ON DELETE SET NULL,
  detail_json TEXT NOT NULL DEFAULT '{}', at INTEGER NOT NULL);
CREATE INDEX idx_auto_improve_proposal_events_proposal_recent ON auto_improve_proposal_events(proposal_id, at ASC, id ASC);

CREATE TABLE auto_improve_rejections (     -- memória de rejeições (evita repetição)
  id BLOB PRIMARY KEY NOT NULL,
  workspace_id BLOB NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
  project_id   BLOB NOT NULL REFERENCES projects(id)   ON DELETE CASCADE,
  target_path TEXT, kind TEXT, operation TEXT, edit_mode TEXT,
  reason TEXT NOT NULL, normalized_fingerprint TEXT NOT NULL, summary TEXT NOT NULL,
  evidence_json TEXT NOT NULL DEFAULT '{}',
  source_run_id BLOB REFERENCES auto_improve_runs(id) ON DELETE SET NULL,
  source_proposal_id BLOB REFERENCES auto_improve_proposals(id) ON DELETE SET NULL,
  created_at INTEGER NOT NULL);
-- índices: (workspace, projeto, created_at DESC); (workspace, projeto, normalized_fingerprint, created_at DESC);
--          (workspace, projeto, target_path, created_at DESC) WHERE target_path IS NOT NULL

CREATE TABLE auto_improve_scheduler_state (
  workspace_id BLOB NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
  project_id   BLOB NOT NULL REFERENCES projects(id)   ON DELETE CASCADE,
  watermark_ended_at INTEGER NOT NULL,       -- só sessões encerradas depois disso entram
  initialized_at INTEGER NOT NULL, updated_at INTEGER NOT NULL,
  last_experience_run_at INTEGER,            -- instante do último passe de experiência entre sessões
  PRIMARY KEY (workspace_id, project_id));

CREATE TABLE auto_improve_scheduler_claims (
  workspace_id BLOB NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
  project_id   BLOB NOT NULL REFERENCES projects(id)   ON DELETE CASCADE,
  session_id BLOB NOT NULL REFERENCES sessions(id) ON DELETE CASCADE,
  claimed_at INTEGER NOT NULL,
  attempts INTEGER NOT NULL DEFAULT 0, last_error TEXT, last_failed_at INTEGER,
  PRIMARY KEY (session_id));
CREATE INDEX idx_auto_improve_scheduler_claims_scope_session ON auto_improve_scheduler_claims(workspace_id, project_id, session_id);
CREATE INDEX idx_auto_improve_scheduler_claims_attempts ON auto_improve_scheduler_claims(workspace_id, project_id, attempts);
```

**Regras de auto-melhoria:**
- No máximo **uma** proposta pendente por (projeto, alvo, autor). Como o índice usa `COALESCE`, em uma
  instalação de um único operador todas as propostas caem no mesmo balde (`''`), preservando a regra
  de uma pendente por alvo. Em instalações com vários operadores, cada autor tem o seu balde.
- Uma reivindicação de sessão pelo agendador tem no máximo **3** tentativas (`attempts`). Falhas
  registram `last_error` e `last_failed_at`.

---

## 8. Workstreams gerenciados (continuidade entre harnesses)

```sql
CREATE TABLE workstreams (
  id BLOB PRIMARY KEY NOT NULL,
  workspace_id BLOB NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
  project_id   BLOB NOT NULL REFERENCES projects(id)   ON DELETE CASCADE,
  repo_fingerprint TEXT NOT NULL, worktree_fingerprint TEXT NOT NULL,
  name TEXT NOT NULL, cwd TEXT NOT NULL,
  created_at INTEGER NOT NULL, selected_at INTEGER NOT NULL, updated_at INTEGER NOT NULL,
  UNIQUE (workspace_id, project_id, repo_fingerprint, worktree_fingerprint, name));
CREATE INDEX idx_workstreams_selected ON workstreams(workspace_id, project_id, repo_fingerprint, worktree_fingerprint, selected_at DESC);

CREATE TABLE workstream_native_sessions (
  workstream_id BLOB NOT NULL REFERENCES workstreams(id) ON DELETE CASCADE,
  agent_kind TEXT NOT NULL, native_session_id TEXT NOT NULL,
  is_current INTEGER NOT NULL DEFAULT 1 CHECK (is_current IN (0,1)),
  source_cursor TEXT, delivery_cursor INTEGER NOT NULL DEFAULT 0,
  created_at INTEGER NOT NULL, updated_at INTEGER NOT NULL,
  PRIMARY KEY (workstream_id, agent_kind, native_session_id));
CREATE UNIQUE INDEX idx_workstream_native_current ON workstream_native_sessions(workstream_id, agent_kind) WHERE is_current = 1;

CREATE TABLE managed_runs (
  id BLOB PRIMARY KEY NOT NULL,
  workstream_id BLOB NOT NULL REFERENCES workstreams(id) ON DELETE CASCADE,
  agent_kind TEXT NOT NULL, lease_owner TEXT NOT NULL, native_session_id TEXT,
  state TEXT NOT NULL CHECK (state IN ('active','finished','expired')),
  sync_after INTEGER NOT NULL DEFAULT 0, sync_through INTEGER NOT NULL DEFAULT 0,
  context_delivered INTEGER NOT NULL DEFAULT 0 CHECK (context_delivered IN (0,1)),
  lease_expires_at INTEGER NOT NULL, started_at INTEGER NOT NULL,
  ended_at INTEGER, exit_code INTEGER,
  native_session_linked_at INTEGER,
  owner_user TEXT);
CREATE UNIQUE INDEX idx_managed_runs_one_active ON managed_runs(workstream_id) WHERE state = 'active';
CREATE INDEX idx_managed_runs_lease ON managed_runs(state, lease_expires_at);
CREATE INDEX idx_managed_runs_adoption ON managed_runs(state, agent_kind, owner_user, context_delivered, native_session_linked_at, lease_expires_at);

CREATE TABLE workstream_events (           -- ledger portátil de eventos normalizados
  workstream_id BLOB NOT NULL REFERENCES workstreams(id) ON DELETE CASCADE,
  sequence INTEGER NOT NULL,
  event_id TEXT NOT NULL, agent_kind TEXT NOT NULL, native_session_id TEXT NOT NULL,
  source_record_id TEXT,
  kind TEXT NOT NULL CHECK (kind IN ('message','tool_call','tool_result','compaction','checkpoint','annotation')),
  role TEXT, content TEXT NOT NULL, occurred_at TEXT,
  metadata_json TEXT NOT NULL DEFAULT '{}', segment_path TEXT,
  created_at INTEGER NOT NULL,
  PRIMARY KEY (workstream_id, sequence),
  UNIQUE (workstream_id, event_id));
CREATE INDEX idx_workstream_events_recent ON workstream_events(workstream_id, sequence DESC);
CREATE VIRTUAL TABLE workstream_events_fts USING fts5(
  content, role UNINDEXED, agent_kind UNINDEXED,
  content='workstream_events', content_rowid='rowid', tokenize='unicode61 remove_diacritics 2');
-- triggers workstream_events_fts_ai/ad/au espelham o padrão de pages_fts (tokenizer sem as classes de caractere extras)
```

Só pode existir **uma** `managed_run` ativa por workstream (índice único parcial) e uma sessão nativa
"corrente" por (workstream, tipo de agente). O lease dura **90 segundos** e é renovado a cada heartbeat.
Quando `lease_expires_at` passa, uma varredura marca a run como `expired`, com `ended_at` preenchido.

---

## 9. Perfil entre projetos

```sql
CREATE TABLE profile_candidates (          -- candidatos colhidos (sem LLM ou classificados por LLM)
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  workspace_id BLOB NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
  project_id   BLOB NOT NULL REFERENCES projects(id)   ON DELETE CASCADE,
  source_kind TEXT NOT NULL CHECK (source_kind IN ('prompt','page','stack')),
  source_ref TEXT NOT NULL,                -- 'session:<id>' (prompt) | 'page:<caminho>' (page) | 'stack:<tag>' (stack)
  topic_key TEXT NOT NULL, category TEXT NOT NULL,
  statement TEXT NOT NULL, quote TEXT NOT NULL,
  applies_to TEXT NOT NULL DEFAULT '[]',   -- array JSON de stacks
  observed_at INTEGER NOT NULL, contributor TEXT,
  generality TEXT NOT NULL CHECK (generality IN ('project','general')),
  confidence REAL NOT NULL DEFAULT 0.5,
  UNIQUE (workspace_id, project_id, source_kind, source_ref, topic_key));
CREATE INDEX idx_profile_candidates_observed ON profile_candidates(observed_at DESC);

CREATE TABLE profile_entry_ledger (        -- o que o colhedor escreveu (detecta entrada apagada à mão)
  workspace_id BLOB NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
  project_id   BLOB NOT NULL REFERENCES projects(id)   ON DELETE CASCADE,
  path TEXT NOT NULL, topic_key TEXT NOT NULL, written_at INTEGER NOT NULL,
  PRIMARY KEY (workspace_id, project_id, path));

CREATE TABLE profile_harvest_marks (       -- marca-d'água de colheita por projeto
  workspace_id BLOB NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
  project_id   BLOB NOT NULL REFERENCES projects(id)   ON DELETE CASCADE,
  observations_until INTEGER NOT NULL DEFAULT 0, pages_until INTEGER NOT NULL DEFAULT 0,
  PRIMARY KEY (workspace_id, project_id));
```

---

## 10. Mapa tabela → versão → quem escreve

Os números são as versões de migração que criaram ou alteraram cada tabela. Use-os para compatibilidade
e para entender a origem de cada coluna.

| Tabela(s) | Versões | Quem escreve |
|---|---|---|
| `workspaces, projects, pages, pages_fts, sessions, observations, links, audit_log` | 1, 3, 5, 7, 8, 10, 12, 13, 15–18, 35, 36, 37, 40, 62, 65, 69, 70, 74, 76 | escritor único (escrita de página; admissão de evento de hook) |
| `handoffs` | 2, 39, 41, 48 | hooks (encerramento de sessão), camada de MCP |
| `page_embeddings, page_abstract_embeddings, page_embed_failures` | 4, 53, 61 | job de backfill e de embeddings |
| `users, api_credentials, web_sessions, human_auth_state, project_grants` | 14, 54, 55, 68 | comandos de usuário, fluxo de autenticação, administração |
| `auto_improve_*` | 21–24, 42, 57, 66 | fluxo de auto-melhoria |
| `workstreams, workstream_native_sessions, workstream_events, managed_runs` | 31, 67, 71 | comando de workstream gerenciado e rotas de workstream |
| `ingest_keys, session_consolidation_jobs` | 33, 34, 35 | roteador de hooks, consolidador |
| `entities, entity_page_links, page_feedback, page_access, page_evidence, agent_messages` | 37, 38, 43, 56, 58, 63, 64 | escritor de entidades, feedback e mensagens |
| `purged_sessions, purged_scopes, maintenance_scheduler_state, bootstrap_chunk_progress, client_activity, wiki_migrations` | 6, 29, 46, 52, 59, 60 | manutenção |
| `profile_candidates, profile_entry_ledger, profile_harvest_marks` | 75 | colhedor de perfil |

A reconstrução de `sessions` para ampliar o tipo de agente acontece nas versões 9, 11, 20, 25, 26, 28, 30,
32, 44, 45, 47, 50, 51, 72 e 73. A reconstrução de `links` está na versão 13 e a de `users` na versão 54.

**Migrações sem tabela nova (só dados ou índice):**

| Versão | O que faz |
|---|---|
| **19** (reparo de atribuição de observações órfãs) | Reparo idempotente: (1) reatribui `observations` cujo `project_id` diverge do da sessão, copiando `project_id` e `workspace_id` da **sessão** (fonte da verdade); o roteamento antigo derivava o projeto do nome do diretório a cada evento e criava projetos-fragmento quando a ferramenta rodava num subdiretório; (2) idem para `handoffs` com `from_session_id` (handoffs avulsos, com `NULL`, ficam); (3) apaga projetos órfãos (sem páginas, sessões, observações nem handoffs), exceto `scratch`. Reexecutar não altera nada. |
| **27** (reparo de atribuição de fragmentos fora de repositório) | Reexecuta o reparo da versão 19. A versão 19 e o guarda por prefixo de `projects.repo_path` não cobriam diretórios que não são repositório (nunca se grava `repo_path` para eles, senão um diretório pessoal viraria prefixo universal), então sessões nesses diretórios continuaram gerando fragmentos. A correção em tempo de execução é a atribuição estável de projeto por sessão; esta migração drena o acumulado. A limpeza de órfãos poupa `scratch` **e** `_global`, e também não apaga projetos com candidatos de auto-melhoria (propostas, runs ou rejeições). |
| **49** (índice de lápides de decaimento) | Recria `idx_pages_evicted` como índice parcial sobre `(workspace_id, project_id, superseded_at) WHERE is_latest = 0 AND superseded_at IS NOT NULL`. Uma lápide de decaimento é identificada por `superseded_at`; o filtro anterior excluía cabeças reescritas, tornando suas lápides permanentes e deixando o índice inutilizável pela consulta de limpeza. |

**Notas de estado final:**

- `users.token_hash` era obrigatório na versão 14 e passou a anulável na versão 54. Os triggers
  `users_token_hash_*` (versão 55) validam 32 bytes e espelham o hash em `api_credentials`.
- `idx_auto_improve_one_pending_target` (versão 42) inclui `COALESCE(staged_by_actor_user,'')`: uma
  pendente por (projeto, alvo, autor).
- Versão 56 preenche `entity_page_links.valid_from` com o `created_at` da página e `superseded_at` com o
  `created_at` da versão sucessora. Versão 58 fecha as janelas ainda abertas de versões aposentadas sem
  sucessora, com `pages.updated_at`.
- Versão 62 só cria `pages.valid_from`, `pages.valid_to` e `idx_pages_validity`. O preenchimento acontece
  na abertura do banco (ver [temporal.md](temporal.md)).
- Versão 35 inicializa `sessions.ended_observation_count` com a contagem de observações das sessões já
  encerradas.
- O gatilho de atualização de `observations_fts` dispara em qualquer coluna; o de `pages_fts` só em
  `title`, `body` e `path_search`.

## 11. Dicas para a reimplementação

1. **Comece pelas seções 1 e 2** (núcleo e sessões/observações). Já dá para gravar página com
   supersessão, busca textual, links e captura por hooks.
2. **Gere o `CHECK` de `agent_kind` a partir de uma lista única** no código, ou omita-o.
3. **Os triggers de FTS fazem parte do contrato.** Teste com um `rebuild` e compare os resultados.
4. **Não use `ON DELETE CASCADE` como mecanismo de purga sem tombstone:** um hook atrasado recriaria a
   sessão. Grave `purged_sessions` e `purged_scopes` na mesma transação da purga.
5. **Todo `INSERT` em tabelas com escopo duplo** (`workspace_id` e `project_id`) deve falhar se o par não
   bate. Os triggers da seção 0.1 são a rede de segurança; valide também na camada de aplicação.
6. **Timestamps em microssegundos desde a época Unix.**
7. **IDs UUIDv7** (ordenáveis por tempo), gravados como 16 bytes.
