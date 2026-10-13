-- V01__init.sql
-- Estado final completo de docs/database-schema.md (feature 001-nucleo, plan §2.3 e P-01).
-- Desvios deliberados:
--   * Controle de migração é schema_migrations (ADR-34), não refinery_schema_history.
--   * Sem coluna nem índice para PortableKey (ADR-06): a colisão é checada em Go.
--   * sessions.agent_kind sem CHECK fechado (PLAN §2.3): validação na aplicação.
--   * observations.kind TEXT NOT NULL sem CHECK (DB §2, PLAN §2.3).
-- Sem BEGIN/COMMIT: o runner executa cada migração dentro de uma transação.

-- Controle de migrações (P-01, ADR-34)
CREATE TABLE schema_migrations (
  version    INTEGER PRIMARY KEY NOT NULL,
  name       TEXT NOT NULL,
  applied_on INTEGER NOT NULL,             -- microssegundos Unix UTC
  checksum   TEXT NOT NULL                 -- SHA-256 do arquivo, hexadecimal
);

-- =====================================================================
-- 1. Núcleo: escopo, páginas, links
-- =====================================================================

CREATE TABLE workspaces (
  id BLOB PRIMARY KEY NOT NULL,
  name TEXT NOT NULL UNIQUE,
  created_at INTEGER NOT NULL
);

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

-- Trigger de pareamento (DB §0.1)
CREATE TRIGGER pages_ws_proj_pairing_ai BEFORE INSERT ON pages
FOR EACH ROW
WHEN NEW.workspace_id IS NOT (SELECT workspace_id FROM projects WHERE id = NEW.project_id)
BEGIN SELECT RAISE(ABORT, 'pages.workspace_id does not match the project''s workspace'); END;

-- Índice de texto das páginas (DB §1.1)
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

-- =====================================================================
-- 2. Sessões e observações
-- =====================================================================

-- agent_kind sem CHECK fechado (PLAN §2.3): a validação do conjunto fica na aplicação.
CREATE TABLE sessions (
  id BLOB PRIMARY KEY NOT NULL,
  workspace_id BLOB NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
  project_id   BLOB NOT NULL REFERENCES projects(id)   ON DELETE CASCADE,
  agent_kind TEXT NOT NULL,
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

CREATE TRIGGER sessions_ws_proj_pairing_ai BEFORE INSERT ON sessions
FOR EACH ROW
WHEN NEW.workspace_id IS NOT (SELECT workspace_id FROM projects WHERE id = NEW.project_id)
BEGIN SELECT RAISE(ABORT, 'sessions.workspace_id does not match the project''s workspace'); END;

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

CREATE TRIGGER observations_ws_proj_pairing_ai BEFORE INSERT ON observations
FOR EACH ROW
WHEN NEW.workspace_id IS NOT (SELECT workspace_id FROM projects WHERE id = NEW.project_id)
BEGIN SELECT RAISE(ABORT, 'observations.workspace_id does not match the project''s workspace'); END;

CREATE VIRTUAL TABLE observations_fts USING fts5(
  title, body, content='observations', content_rowid='rowid',
  tokenize="unicode61 remove_diacritics 2 tokenchars '/_-'");
CREATE TRIGGER observations_fts_ai AFTER INSERT ON observations BEGIN
  INSERT INTO observations_fts(rowid,title,body) VALUES (new.rowid,new.title,new.body);
END;
CREATE TRIGGER observations_fts_ad AFTER DELETE ON observations BEGIN
  INSERT INTO observations_fts(observations_fts,rowid,title,body)
  VALUES ('delete',old.rowid,old.title,old.body);
END;
-- O trigger de atualização dispara em qualquer coluna da linha (DB §2).
CREATE TRIGGER observations_fts_au AFTER UPDATE ON observations BEGIN
  INSERT INTO observations_fts(observations_fts,rowid,title,body)
  VALUES ('delete',old.rowid,old.title,old.body);
  INSERT INTO observations_fts(rowid,title,body) VALUES (new.rowid,new.title,new.body);
END;

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

-- Tabela filha de sessão: ws e projeto têm de ser os da sessão (DB §0.1)
CREATE TRIGGER session_consolidation_jobs_session_pairing_ai BEFORE INSERT ON session_consolidation_jobs
FOR EACH ROW
WHEN NOT EXISTS (SELECT 1 FROM sessions s
                 WHERE s.id = NEW.session_id AND s.workspace_id = NEW.workspace_id AND s.project_id = NEW.project_id)
BEGIN SELECT RAISE(ABORT, 'session_consolidation_jobs.workspace_id/project_id do not match the session'); END;

-- =====================================================================
-- 3. Handoffs e mensagens entre projetos
-- =====================================================================

CREATE TABLE handoffs (
  id BLOB PRIMARY KEY NOT NULL,
  workspace_id BLOB NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
  project_id   BLOB NOT NULL REFERENCES projects(id)   ON DELETE CASCADE,
  from_session_id BLOB REFERENCES sessions(id) ON DELETE SET NULL,   -- NULL = manual
  from_agent TEXT NOT NULL,
  to_agent TEXT,                          -- dica de destino opcional (sem filtro de leitura)
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

CREATE TRIGGER handoffs_ws_proj_pairing_ai BEFORE INSERT ON handoffs
FOR EACH ROW
WHEN NEW.workspace_id IS NOT (SELECT workspace_id FROM projects WHERE id = NEW.project_id)
BEGIN SELECT RAISE(ABORT, 'handoffs.workspace_id does not match the project''s workspace'); END;

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

-- =====================================================================
-- 4. Embeddings, feedback, entidades, evidência
-- =====================================================================

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

-- Variante NOT EXISTS: o workspace informado precisa ser o do projeto (DB §0.1)
CREATE TRIGGER entities_ws_proj_pairing_ai BEFORE INSERT ON entities
FOR EACH ROW
WHEN NOT EXISTS (SELECT 1 FROM projects p WHERE p.id = NEW.project_id AND p.workspace_id = NEW.workspace_id)
BEGIN SELECT RAISE(ABORT, 'entities.workspace_id does not match the project''s workspace'); END;

CREATE TABLE entity_page_links (
  entity_id BLOB NOT NULL REFERENCES entities(id) ON DELETE CASCADE,
  page_id   BLOB NOT NULL REFERENCES pages(id)    ON DELETE CASCADE,
  valid_from INTEGER,                      -- instante de criação da versão da página
  superseded_at INTEGER,                   -- instante de criação da versão que a substituiu (NULL = janela aberta)
  PRIMARY KEY (entity_id, page_id)
);
CREATE INDEX idx_entity_page_links_page ON entity_page_links(page_id);
CREATE INDEX idx_entity_page_links_validity ON entity_page_links(entity_id, valid_from, superseded_at);

-- Entidade e página precisam ter o mesmo workspace e o mesmo projeto (DB §0.1)
CREATE TRIGGER entity_page_links_ws_proj_pairing_ai BEFORE INSERT ON entity_page_links
FOR EACH ROW
WHEN NOT EXISTS (SELECT 1 FROM entities e JOIN pages p ON p.id = NEW.page_id
                 WHERE e.id = NEW.entity_id
                   AND e.workspace_id = p.workspace_id AND e.project_id = p.project_id)
BEGIN SELECT RAISE(ABORT, 'entity_page_links: entity and page must share workspace and project'); END;

-- =====================================================================
-- 5. Usuários, credenciais e sessões web
-- =====================================================================

CREATE TABLE human_auth_state (            -- singleton
  id INTEGER PRIMARY KEY NOT NULL CHECK (id = 1),
  bootstrap_completed INTEGER NOT NULL DEFAULT 0 CHECK (bootstrap_completed IN (0,1))
);
INSERT OR IGNORE INTO human_auth_state(id, bootstrap_completed) VALUES (1, 0);

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

-- =====================================================================
-- 6. Auditoria, atividade e manutenção
-- =====================================================================

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

-- =====================================================================
-- 7. Auto-melhoria
-- =====================================================================

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

CREATE TRIGGER auto_improve_runs_ws_proj_pairing_ai BEFORE INSERT ON auto_improve_runs
FOR EACH ROW
WHEN NEW.workspace_id IS NOT (SELECT workspace_id FROM projects WHERE id = NEW.project_id)
BEGIN SELECT RAISE(ABORT, 'auto_improve_runs.workspace_id does not match the project''s workspace'); END;

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

CREATE TRIGGER auto_improve_proposals_ws_proj_pairing_ai BEFORE INSERT ON auto_improve_proposals
FOR EACH ROW
WHEN NEW.workspace_id IS NOT (SELECT workspace_id FROM projects WHERE id = NEW.project_id)
BEGIN SELECT RAISE(ABORT, 'auto_improve_proposals.workspace_id does not match the project''s workspace'); END;

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
CREATE INDEX idx_auto_improve_rejections_scope_recent ON auto_improve_rejections(workspace_id, project_id, created_at DESC);
CREATE INDEX idx_auto_improve_rejections_fingerprint ON auto_improve_rejections(workspace_id, project_id, normalized_fingerprint, created_at DESC);
CREATE INDEX idx_auto_improve_rejections_target ON auto_improve_rejections(workspace_id, project_id, target_path, created_at DESC) WHERE target_path IS NOT NULL;

CREATE TRIGGER auto_improve_rejections_ws_proj_pairing_ai BEFORE INSERT ON auto_improve_rejections
FOR EACH ROW
WHEN NEW.workspace_id IS NOT (SELECT workspace_id FROM projects WHERE id = NEW.project_id)
BEGIN SELECT RAISE(ABORT, 'auto_improve_rejections.workspace_id does not match the project''s workspace'); END;

CREATE TABLE auto_improve_scheduler_state (
  workspace_id BLOB NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
  project_id   BLOB NOT NULL REFERENCES projects(id)   ON DELETE CASCADE,
  watermark_ended_at INTEGER NOT NULL,       -- só sessões encerradas depois disso entram
  initialized_at INTEGER NOT NULL, updated_at INTEGER NOT NULL,
  last_experience_run_at INTEGER,            -- instante do último passe de experiência entre sessões
  PRIMARY KEY (workspace_id, project_id));

CREATE TRIGGER auto_improve_scheduler_state_ws_proj_pairing_ai BEFORE INSERT ON auto_improve_scheduler_state
FOR EACH ROW
WHEN NEW.workspace_id IS NOT (SELECT workspace_id FROM projects WHERE id = NEW.project_id)
BEGIN SELECT RAISE(ABORT, 'auto_improve_scheduler_state.workspace_id does not match the project''s workspace'); END;

CREATE TABLE auto_improve_scheduler_claims (
  workspace_id BLOB NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
  project_id   BLOB NOT NULL REFERENCES projects(id)   ON DELETE CASCADE,
  session_id BLOB NOT NULL REFERENCES sessions(id) ON DELETE CASCADE,
  claimed_at INTEGER NOT NULL,
  attempts INTEGER NOT NULL DEFAULT 0, last_error TEXT, last_failed_at INTEGER,
  PRIMARY KEY (session_id));
CREATE INDEX idx_auto_improve_scheduler_claims_scope_session ON auto_improve_scheduler_claims(workspace_id, project_id, session_id);
CREATE INDEX idx_auto_improve_scheduler_claims_attempts ON auto_improve_scheduler_claims(workspace_id, project_id, attempts);

CREATE TRIGGER auto_improve_scheduler_claims_ws_proj_pairing_ai BEFORE INSERT ON auto_improve_scheduler_claims
FOR EACH ROW
WHEN NEW.workspace_id IS NOT (SELECT workspace_id FROM projects WHERE id = NEW.project_id)
BEGIN SELECT RAISE(ABORT, 'auto_improve_scheduler_claims.workspace_id does not match the project''s workspace'); END;

-- Tabela filha de sessão: ws e projeto têm de ser os da sessão (DB §0.1)
CREATE TRIGGER auto_improve_scheduler_claims_session_pairing_ai BEFORE INSERT ON auto_improve_scheduler_claims
FOR EACH ROW
WHEN NOT EXISTS (SELECT 1 FROM sessions s
                 WHERE s.id = NEW.session_id AND s.workspace_id = NEW.workspace_id AND s.project_id = NEW.project_id)
BEGIN SELECT RAISE(ABORT, 'auto_improve_scheduler_claims.workspace_id/project_id do not match the session'); END;

-- =====================================================================
-- 8. Workstreams gerenciados
-- =====================================================================

CREATE TABLE workstreams (
  id BLOB PRIMARY KEY NOT NULL,
  workspace_id BLOB NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
  project_id   BLOB NOT NULL REFERENCES projects(id)   ON DELETE CASCADE,
  repo_fingerprint TEXT NOT NULL, worktree_fingerprint TEXT NOT NULL,
  name TEXT NOT NULL, cwd TEXT NOT NULL,
  created_at INTEGER NOT NULL, selected_at INTEGER NOT NULL, updated_at INTEGER NOT NULL,
  UNIQUE (workspace_id, project_id, repo_fingerprint, worktree_fingerprint, name));
CREATE INDEX idx_workstreams_selected ON workstreams(workspace_id, project_id, repo_fingerprint, worktree_fingerprint, selected_at DESC);

CREATE TRIGGER workstreams_ws_proj_pairing_ai BEFORE INSERT ON workstreams
FOR EACH ROW
WHEN NEW.workspace_id IS NOT (SELECT workspace_id FROM projects WHERE id = NEW.project_id)
BEGIN SELECT RAISE(ABORT, 'workstreams.workspace_id does not match the project''s workspace'); END;

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
CREATE TRIGGER workstream_events_fts_ai AFTER INSERT ON workstream_events BEGIN
  INSERT INTO workstream_events_fts(rowid,content,role,agent_kind)
  VALUES (new.rowid,new.content,new.role,new.agent_kind);
END;
CREATE TRIGGER workstream_events_fts_ad AFTER DELETE ON workstream_events BEGIN
  INSERT INTO workstream_events_fts(workstream_events_fts,rowid,content,role,agent_kind)
  VALUES ('delete',old.rowid,old.content,old.role,old.agent_kind);
END;
CREATE TRIGGER workstream_events_fts_au AFTER UPDATE ON workstream_events BEGIN
  INSERT INTO workstream_events_fts(workstream_events_fts,rowid,content,role,agent_kind)
  VALUES ('delete',old.rowid,old.content,old.role,old.agent_kind);
  INSERT INTO workstream_events_fts(rowid,content,role,agent_kind)
  VALUES (new.rowid,new.content,new.role,new.agent_kind);
END;

-- =====================================================================
-- 9. Perfil entre projetos
-- =====================================================================

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

-- Versão 49 do doc: índice de lápides de decaimento já está em idx_pages_evicted acima.
