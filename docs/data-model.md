# Modelo de dados (diagramas)

> **Papel:** visão relacional do esquema de [database-schema.md](database-schema.md), no estilo de um
> modelo físico (tabelas, chaves, cardinalidade, regras de exclusão e dependências). Gerado do estado
> final das 83 migrações da versão 3.1.1 da referência. Onde este documento e o
> `database-schema.md` divergirem, vale o `database-schema.md`, que traz as restrições `CHECK`, os índices
> e os triggers. Os diagramas usam [Mermaid](https://mermaid.js.org/syntax/entityRelationshipDiagram.html).

## Como ler

| Notação | Significado |
|---|---|
| `PK`, `FK`, `UK` | chave primária, chave estrangeira, coluna única sozinha |
| `"opcional"` | a coluna aceita nulo |
| `\|\|--o{` | o filho aponta obrigatoriamente para **um** pai; o pai tem zero ou mais filhos |
| `\|o--o{` | a referência do filho é opcional (FK que aceita nulo) |
| `\|\|--o\|` | um para zero ou um (a FK do filho também é chave única) |
| linha tracejada (`..`) | ligação **lógica**, sem chave estrangeira no banco (o rótulo diz por quê) |
| `(CASCADE)`, `(SET NULL)` | o que acontece com o filho quando o pai é apagado |

Convenções do banco que os diagramas não mostram: identificadores são `BLOB` de 16 bytes (UUIDv7), instantes
são `INTEGER` em microssegundos UTC, booleanos são `INTEGER` 0/1 com `CHECK`, JSON é `TEXT`. Toda tabela com
`workspace_id` **e** `project_id` tem trigger que recusa um par que não combina (`database-schema.md` §0.1).
As chaves estrangeiras só valem com `foreign_keys = ON`, o modo de execução (as migrações rodam com OFF).

## 1. Mapa geral

Todas as 46 tabelas e as 82 chaves estrangeiras, sem colunas.
Ligações lógicas estão nos diagramas de domínio e na seção 3.

```mermaid
erDiagram
    sessions |o--o{ agent_messages : "claimed_by_session (SET NULL)"
    sessions |o--o{ agent_messages : "from_session_id (SET NULL)"
    projects ||--o{ agent_messages : "to_project_id (CASCADE)"
    workspaces ||--o{ agent_messages : "to_workspace_id (CASCADE)"
    projects ||--o{ agent_messages : "from_project_id (CASCADE)"
    workspaces ||--o{ agent_messages : "from_workspace_id (CASCADE)"
    users ||--o{ api_credentials : "user_id (CASCADE)"
    users |o--o{ audit_log : "author_id (SET NULL)"
    users |o--o{ auto_improve_proposal_events : "author_id (SET NULL)"
    auto_improve_proposals ||--o{ auto_improve_proposal_events : "proposal_id (CASCADE)"
    pages |o--o{ auto_improve_proposals : "applied_page_id (SET NULL)"
    users |o--o{ auto_improve_proposals : "decided_by_author_id (SET NULL)"
    pages |o--o{ auto_improve_proposals : "target_latest_page_id_at_stage (SET NULL)"
    projects ||--o{ auto_improve_proposals : "project_id (CASCADE)"
    workspaces ||--o{ auto_improve_proposals : "workspace_id (CASCADE)"
    auto_improve_runs ||--o{ auto_improve_proposals : "run_id (CASCADE)"
    auto_improve_proposals |o--o{ auto_improve_rejections : "source_proposal_id (SET NULL)"
    auto_improve_runs |o--o{ auto_improve_rejections : "source_run_id (SET NULL)"
    projects ||--o{ auto_improve_rejections : "project_id (CASCADE)"
    workspaces ||--o{ auto_improve_rejections : "workspace_id (CASCADE)"
    sessions |o--o{ auto_improve_runs : "session_id (SET NULL)"
    projects ||--o{ auto_improve_runs : "project_id (CASCADE)"
    workspaces ||--o{ auto_improve_runs : "workspace_id (CASCADE)"
    sessions ||--o| auto_improve_scheduler_claims : "session_id (CASCADE)"
    projects ||--o{ auto_improve_scheduler_claims : "project_id (CASCADE)"
    workspaces ||--o{ auto_improve_scheduler_claims : "workspace_id (CASCADE)"
    projects ||--o{ auto_improve_scheduler_state : "project_id (CASCADE)"
    workspaces ||--o{ auto_improve_scheduler_state : "workspace_id (CASCADE)"
    projects ||--o{ entities : "project_id (CASCADE)"
    workspaces ||--o{ entities : "workspace_id (CASCADE)"
    pages ||--o{ entity_page_links : "page_id (CASCADE)"
    entities ||--o{ entity_page_links : "entity_id (CASCADE)"
    sessions |o--o{ handoffs : "accepted_by_session (SET NULL)"
    sessions |o--o{ handoffs : "from_session_id (SET NULL)"
    projects ||--o{ handoffs : "project_id (CASCADE)"
    workspaces ||--o{ handoffs : "workspace_id (CASCADE)"
    sessions |o--o{ ingest_keys : "session_id (CASCADE)"
    projects ||--o{ ingest_keys : "project_id (CASCADE)"
    pages |o--o{ links : "to_page_id (SET NULL)"
    pages ||--o{ links : "from_page_id (CASCADE)"
    workstreams ||--o{ managed_runs : "workstream_id (CASCADE)"
    projects ||--o{ observations : "project_id (CASCADE)"
    workspaces ||--o{ observations : "workspace_id (CASCADE)"
    sessions ||--o{ observations : "session_id (CASCADE)"
    pages ||--o| page_abstract_embeddings : "page_id (CASCADE)"
    pages ||--o{ page_access : "page_id (CASCADE)"
    pages ||--o| page_embed_failures : "page_id (CASCADE)"
    pages ||--o| page_embeddings : "page_id (CASCADE)"
    pages ||--o{ page_evidence : "page_id (CASCADE)"
    users |o--o{ page_feedback : "author_id (SET NULL)"
    projects ||--o{ page_feedback : "project_id (CASCADE)"
    workspaces ||--o{ page_feedback : "workspace_id (CASCADE)"
    pages ||--o{ page_feedback : "page_id (CASCADE)"
    users |o--o{ pages : "author_id (SET NULL)"
    pages |o--o{ pages : "supersedes (SET NULL)"
    projects ||--o{ pages : "project_id (CASCADE)"
    workspaces ||--o{ pages : "workspace_id (CASCADE)"
    projects ||--o{ profile_candidates : "project_id (CASCADE)"
    workspaces ||--o{ profile_candidates : "workspace_id (CASCADE)"
    projects ||--o{ profile_entry_ledger : "project_id (CASCADE)"
    workspaces ||--o{ profile_entry_ledger : "workspace_id (CASCADE)"
    projects ||--o{ profile_harvest_marks : "project_id (CASCADE)"
    workspaces ||--o{ profile_harvest_marks : "workspace_id (CASCADE)"
    users |o--o{ project_grants : "granted_by (SET NULL)"
    users ||--o{ project_grants : "user_id (CASCADE)"
    projects ||--o{ project_grants : "project_id (CASCADE)"
    workspaces ||--o{ project_grants : "workspace_id (CASCADE)"
    users |o--o{ projects : "created_by (SET NULL)"
    workspaces ||--o{ projects : "workspace_id (CASCADE)"
    projects ||--o{ purged_sessions : "project_id (CASCADE)"
    workspaces ||--o{ purged_sessions : "workspace_id (CASCADE)"
    projects ||--o{ session_consolidation_jobs : "project_id (CASCADE)"
    workspaces ||--o{ session_consolidation_jobs : "workspace_id (CASCADE)"
    sessions ||--o{ session_consolidation_jobs : "session_id (CASCADE)"
    pages |o--o{ sessions : "summary_page_id (SET NULL)"
    projects ||--o{ sessions : "project_id (CASCADE)"
    workspaces ||--o{ sessions : "workspace_id (CASCADE)"
    users ||--o{ web_sessions : "user_id (CASCADE)"
    workstreams ||--o{ workstream_events : "workstream_id (CASCADE)"
    workstreams ||--o{ workstream_native_sessions : "workstream_id (CASCADE)"
    projects ||--o{ workstreams : "project_id (CASCADE)"
    workspaces ||--o{ workstreams : "workspace_id (CASCADE)"
    bootstrap_chunk_progress {
        TEXT fingerprint PK
    }
    client_activity {
        TEXT client PK
    }
    human_auth_state {
        INTEGER id PK
    }
    maintenance_scheduler_state {
        TEXT job PK "opcional"
    }
    observations_fts {
        FTS title "opcional"
    }
    pages_fts {
        FTS title "opcional"
    }
    purged_scopes {
        BLOB workspace_id PK
    }
    wiki_migrations {
        TEXT name PK "opcional"
    }
    workstream_events_fts {
        FTS content "opcional"
    }
```

Tabelas sem nenhuma chave estrangeira (de ou para): `bootstrap_chunk_progress`, `client_activity`, `human_auth_state`, `maintenance_scheduler_state`, `observations_fts`, `pages_fts`, `purged_scopes`, `wiki_migrations`, `workstream_events_fts`.

## 2. Diagramas por domínio

Cada diagrama mostra as colunas das tabelas do domínio. Tabelas de outros domínios aparecem só pelo nome,
quando são alvo de uma ligação.

### 2.1 Núcleo: escopo, páginas, links

```mermaid
erDiagram
    links {
        BLOB from_page_id PK, FK
        BLOB to_page_id FK "opcional"
        TEXT to_workspace PK "opcional"
        TEXT to_project PK "opcional"
        TEXT to_path PK
        TEXT link_type PK
    }
    pages {
        BLOB id PK
        BLOB workspace_id FK
        BLOB project_id FK
        TEXT path
        TEXT title
        TEXT tier
        TEXT body
        BLOB body_sha256
        TEXT frontmatter_json
        INTEGER is_latest
        BLOB supersedes FK "opcional"
        INTEGER pinned
        INTEGER created_at
        INTEGER updated_at
        TEXT embedding_provider "opcional"
        TEXT embedding_model "opcional"
        INTEGER embedding_dim "opcional"
        INTEGER last_accessed_at "opcional"
        INTEGER access_count
        INTEGER superseded_at "opcional"
        BLOB author_id FK "opcional"
        TEXT path_search
        INTEGER expires_at "opcional"
        REAL salience "opcional"
        INTEGER valid_from "opcional"
        INTEGER valid_to "opcional"
        INTEGER compacted_at "opcional"
    }
    pages_fts {
        FTS title "opcional"
        FTS body "opcional"
        FTS path_search "opcional"
    }
    projects {
        BLOB id PK
        BLOB workspace_id FK
        TEXT name
        TEXT repo_path "opcional"
        INTEGER created_at
        TEXT access_mode
        BLOB created_by FK "opcional"
        TEXT identity
        TEXT identity_source
        INTEGER profile_contribute
        INTEGER profile_consume
        TEXT canonical_name
        TEXT legacy_name
    }
    workspaces {
        BLOB id PK
        TEXT name UK
        INTEGER created_at
    }
    pages |o--o{ links : "to_page_id (SET NULL)"
    pages ||--o{ links : "from_page_id (CASCADE)"
    users |o--o{ pages : "author_id (SET NULL)"
    pages |o--o{ pages : "supersedes (SET NULL)"
    projects ||--o{ pages : "project_id (CASCADE)"
    workspaces ||--o{ pages : "workspace_id (CASCADE)"
    users |o--o{ projects : "created_by (SET NULL)"
    workspaces ||--o{ projects : "workspace_id (CASCADE)"
    workspaces |o..o{ links : "to_workspace: por NOME (nulo = workspace da origem)"
    projects |o..o{ links : "to_project: por NOME (nulo = projeto da origem)"
    pages |o..o{ pages_fts : "rowid: FTS de conteúdo externo, por triggers"
```

### 2.2 Sessões e observações

```mermaid
erDiagram
    ingest_keys {
        BLOB project_id PK, FK
        TEXT key PK
        INTEGER seen_at
        INTEGER completed_at "opcional"
        BLOB session_id FK "opcional"
        TEXT agent_kind "opcional"
        TEXT observation_kind "opcional"
        TEXT event_identity "opcional"
        INTEGER recovery_retained
    }
    observations {
        BLOB id PK
        BLOB session_id FK
        BLOB workspace_id FK
        BLOB project_id FK
        TEXT kind
        TEXT title
        TEXT body
        INTEGER importance
        INTEGER created_at
        TEXT extension "opcional"
        TEXT source_event "opcional"
    }
    observations_fts {
        FTS title "opcional"
        FTS body "opcional"
    }
    session_consolidation_jobs {
        BLOB session_id PK, FK
        BLOB workspace_id FK
        BLOB project_id FK
        INTEGER generation PK
        TEXT state
        INTEGER requested_at
        INTEGER next_attempt_at
        INTEGER started_at "opcional"
        INTEGER completed_at "opcional"
        INTEGER attempts
        BLOB claim_id "opcional"
        TEXT last_error "opcional"
    }
    sessions {
        BLOB id PK
        BLOB workspace_id FK
        BLOB project_id FK
        TEXT agent_kind
        TEXT cwd "opcional"
        INTEGER started_at
        INTEGER ended_at "opcional"
        BLOB summary_page_id FK "opcional"
        INTEGER ended_observation_count
        TEXT actor_user "opcional"
    }
    sessions |o--o{ ingest_keys : "session_id (CASCADE)"
    projects ||--o{ ingest_keys : "project_id (CASCADE)"
    projects ||--o{ observations : "project_id (CASCADE)"
    workspaces ||--o{ observations : "workspace_id (CASCADE)"
    sessions ||--o{ observations : "session_id (CASCADE)"
    projects ||--o{ session_consolidation_jobs : "project_id (CASCADE)"
    workspaces ||--o{ session_consolidation_jobs : "workspace_id (CASCADE)"
    sessions ||--o{ session_consolidation_jobs : "session_id (CASCADE)"
    pages |o--o{ sessions : "summary_page_id (SET NULL)"
    projects ||--o{ sessions : "project_id (CASCADE)"
    workspaces ||--o{ sessions : "workspace_id (CASCADE)"
    observations |o..o{ observations_fts : "rowid: FTS de conteúdo externo, por triggers"
    users |o..o{ sessions : "actor_user: identidade do operador em texto, sem FK"
```

### 2.3 Handoffs e mensagens entre projetos

```mermaid
erDiagram
    agent_messages {
        BLOB id PK
        BLOB from_workspace_id FK
        BLOB from_project_id FK
        TEXT from_agent
        BLOB from_session_id FK "opcional"
        TEXT from_owner_user "opcional"
        BLOB to_workspace_id FK
        BLOB to_project_id FK
        TEXT subject "opcional"
        TEXT body
        TEXT state
        INTEGER created_at
        INTEGER claimed_at "opcional"
        BLOB claimed_by_session FK "opcional"
        TEXT claimed_by_agent "opcional"
        TEXT claimed_by_user "opcional"
    }
    handoffs {
        BLOB id PK
        BLOB workspace_id FK
        BLOB project_id FK
        BLOB from_session_id FK "opcional"
        TEXT from_agent
        TEXT to_agent "opcional"
        TEXT cwd "opcional"
        TEXT summary
        TEXT open_questions
        TEXT next_steps
        TEXT files_touched
        TEXT state
        INTEGER created_at
        TEXT accepted_by "opcional"
        INTEGER accepted_at "opcional"
        BLOB accepted_by_session FK "opcional"
        TEXT owner_user "opcional"
        TEXT accepted_by_user "opcional"
    }
    sessions |o--o{ agent_messages : "claimed_by_session (SET NULL)"
    sessions |o--o{ agent_messages : "from_session_id (SET NULL)"
    projects ||--o{ agent_messages : "to_project_id (CASCADE)"
    workspaces ||--o{ agent_messages : "to_workspace_id (CASCADE)"
    projects ||--o{ agent_messages : "from_project_id (CASCADE)"
    workspaces ||--o{ agent_messages : "from_workspace_id (CASCADE)"
    sessions |o--o{ handoffs : "accepted_by_session (SET NULL)"
    sessions |o--o{ handoffs : "from_session_id (SET NULL)"
    projects ||--o{ handoffs : "project_id (CASCADE)"
    workspaces ||--o{ handoffs : "workspace_id (CASCADE)"
    users |o..o{ agent_messages : "from_owner_user: identidade do operador em texto, sem FK"
    users |o..o{ agent_messages : "claimed_by_user: identidade do operador em texto, sem FK"
    users |o..o{ handoffs : "owner_user: identidade do operador em texto, sem FK"
    users |o..o{ handoffs : "accepted_by_user: identidade do operador em texto, sem FK"
```

### 2.4 Embeddings, feedback, entidades, evidência

```mermaid
erDiagram
    entities {
        BLOB id PK "opcional"
        BLOB workspace_id FK
        BLOB project_id FK
        TEXT name
        INTEGER created_at
    }
    entity_page_links {
        BLOB entity_id PK, FK
        BLOB page_id PK, FK
        INTEGER valid_from "opcional"
        INTEGER superseded_at "opcional"
    }
    page_abstract_embeddings {
        BLOB page_id PK, FK
        BLOB vector
        TEXT provider
        TEXT model
        INTEGER dim
        INTEGER created_at
    }
    page_access {
        BLOB page_id PK, FK
        TEXT actor PK
    }
    page_embed_failures {
        BLOB page_id PK, FK
        INTEGER at
        TEXT outcome
        TEXT detail "opcional"
    }
    page_embeddings {
        BLOB page_id PK, FK
        BLOB vector
        TEXT provider
        TEXT model
        INTEGER dim
        INTEGER created_at
    }
    page_evidence {
        BLOB page_id PK, FK
        TEXT source_kind PK
        TEXT source_id PK
        INTEGER created_at
    }
    page_feedback {
        BLOB id PK "opcional"
        BLOB page_id FK
        BLOB workspace_id FK
        BLOB project_id FK
        TEXT kind
        TEXT reason "opcional"
        REAL salience_after
        BLOB author_id FK "opcional"
        INTEGER created_at
    }
    projects ||--o{ entities : "project_id (CASCADE)"
    workspaces ||--o{ entities : "workspace_id (CASCADE)"
    pages ||--o{ entity_page_links : "page_id (CASCADE)"
    entities ||--o{ entity_page_links : "entity_id (CASCADE)"
    pages ||--o| page_abstract_embeddings : "page_id (CASCADE)"
    pages ||--o{ page_access : "page_id (CASCADE)"
    pages ||--o| page_embed_failures : "page_id (CASCADE)"
    pages ||--o| page_embeddings : "page_id (CASCADE)"
    pages ||--o{ page_evidence : "page_id (CASCADE)"
    users |o--o{ page_feedback : "author_id (SET NULL)"
    projects ||--o{ page_feedback : "project_id (CASCADE)"
    workspaces ||--o{ page_feedback : "workspace_id (CASCADE)"
    pages ||--o{ page_feedback : "page_id (CASCADE)"
    sessions |o..o{ page_evidence : "source_id: polimórfica: source_kind = session"
    observations |o..o{ page_evidence : "source_id: polimórfica: source_kind = observation"
```

### 2.5 Usuários, credenciais e sessões web

```mermaid
erDiagram
    api_credentials {
        BLOB id PK
        BLOB user_id FK
        TEXT label
        BLOB token_hash UK
        TEXT preview "opcional"
        INTEGER created_at
        INTEGER last_used_at "opcional"
        INTEGER expires_at "opcional"
        INTEGER revoked_at "opcional"
    }
    human_auth_state {
        INTEGER id PK
        INTEGER bootstrap_completed
    }
    project_grants {
        BLOB workspace_id PK, FK
        BLOB project_id PK, FK
        BLOB user_id PK, FK
        TEXT level
        BLOB granted_by FK "opcional"
        INTEGER granted_at
    }
    users {
        BLOB id PK
        TEXT username UK
        TEXT name "opcional"
        TEXT email UK "opcional"
        INTEGER created_at
        INTEGER last_seen_at "opcional"
        TEXT role
        TEXT password_hash "opcional"
        INTEGER must_change_password
        INTEGER disabled_at "opcional"
    }
    web_sessions {
        BLOB id PK
        BLOB session_hash UK
        BLOB user_id FK
        BLOB csrf_hash
        INTEGER created_at
        INTEGER last_used_at
        INTEGER expires_at
        INTEGER revoked_at "opcional"
    }
    users ||--o{ api_credentials : "user_id (CASCADE)"
    users |o--o{ project_grants : "granted_by (SET NULL)"
    users ||--o{ project_grants : "user_id (CASCADE)"
    projects ||--o{ project_grants : "project_id (CASCADE)"
    workspaces ||--o{ project_grants : "workspace_id (CASCADE)"
    users ||--o{ web_sessions : "user_id (CASCADE)"
```

### 2.6 Auditoria, atividade e manutenção

```mermaid
erDiagram
    audit_log {
        INTEGER id PK
        INTEGER at
        TEXT op
        BLOB workspace_id "opcional"
        BLOB project_id "opcional"
        BLOB page_id "opcional"
        TEXT detail
        BLOB author_id FK "opcional"
    }
    bootstrap_chunk_progress {
        TEXT fingerprint PK
        INTEGER chunk_index PK
        TEXT pages_json
        TEXT rationale
        INTEGER created_at
    }
    client_activity {
        TEXT client PK
        INTEGER day PK
        INTEGER reads
        INTEGER writes
    }
    maintenance_scheduler_state {
        TEXT job PK "opcional"
        INTEGER last_success_at
    }
    purged_scopes {
        BLOB workspace_id PK
        BLOB project_id PK
        INTEGER purged_at
    }
    purged_sessions {
        BLOB session_id PK
        BLOB workspace_id FK
        BLOB project_id FK
        INTEGER purged_at
    }
    wiki_migrations {
        TEXT name PK "opcional"
        INTEGER applied_at
    }
    users |o--o{ audit_log : "author_id (SET NULL)"
    projects ||--o{ purged_sessions : "project_id (CASCADE)"
    workspaces ||--o{ purged_sessions : "workspace_id (CASCADE)"
    workspaces |o..o{ audit_log : "workspace_id: sem FK: a auditoria sobrevive à purga"
    projects |o..o{ audit_log : "project_id: sem FK: a auditoria sobrevive à purga"
    pages |o..o{ audit_log : "page_id: sem FK: a auditoria sobrevive à remoção"
    workspaces |o..o{ purged_scopes : "workspace_id: lápide: o escopo já foi apagado"
    projects |o..o{ purged_scopes : "project_id: lápide: o escopo já foi apagado"
    sessions |o..o{ purged_sessions : "session_id: lápide: a sessão já foi apagada"
```

### 2.7 Auto-melhoria

```mermaid
erDiagram
    auto_improve_proposal_events {
        INTEGER id PK
        BLOB proposal_id FK
        TEXT event
        TEXT actor_json
        BLOB author_id FK "opcional"
        TEXT detail_json
        INTEGER at
    }
    auto_improve_proposals {
        BLOB id PK
        BLOB run_id FK
        BLOB workspace_id FK
        BLOB project_id FK
        TEXT status
        TEXT operation
        TEXT target_path
        TEXT kind
        TEXT title
        REAL confidence
        TEXT rationale
        TEXT evidence_json
        TEXT body_markdown
        BLOB body_sha256
        TEXT artifact_path
        BLOB artifact_sha256 "opcional"
        BLOB target_latest_page_id_at_stage FK "opcional"
        BLOB target_body_sha256_at_stage "opcional"
        INTEGER target_updated_at_at_stage "opcional"
        INTEGER staged_at
        INTEGER decided_at "opcional"
        TEXT decision_reason "opcional"
        BLOB decided_by_author_id FK "opcional"
        TEXT decided_by_actor_json "opcional"
        BLOB applied_page_id FK "opcional"
        TEXT checkpoint "opcional"
        TEXT edit_mode
        TEXT patch_json "opcional"
        BLOB expected_base_body_sha256 "opcional"
        BLOB materialized_base_body_sha256 "opcional"
        TEXT staged_by_actor_user "opcional"
    }
    auto_improve_rejections {
        BLOB id PK
        BLOB workspace_id FK
        BLOB project_id FK
        TEXT target_path "opcional"
        TEXT kind "opcional"
        TEXT operation "opcional"
        TEXT edit_mode "opcional"
        TEXT reason
        TEXT normalized_fingerprint
        TEXT summary
        TEXT evidence_json
        BLOB source_run_id FK "opcional"
        BLOB source_proposal_id FK "opcional"
        INTEGER created_at
    }
    auto_improve_runs {
        BLOB id PK
        BLOB workspace_id FK
        BLOB project_id FK
        BLOB session_id FK "opcional"
        TEXT provider "opcional"
        TEXT model "opcional"
        TEXT summary "opcional"
        TEXT warnings_json
        TEXT rejected_candidates_json
        TEXT config_json
        TEXT proposal_actor_json
        INTEGER created_at
    }
    auto_improve_scheduler_claims {
        BLOB workspace_id FK
        BLOB project_id FK
        BLOB session_id PK, FK
        INTEGER claimed_at
        INTEGER attempts
        TEXT last_error "opcional"
        INTEGER last_failed_at "opcional"
    }
    auto_improve_scheduler_state {
        BLOB workspace_id PK, FK
        BLOB project_id PK, FK
        INTEGER watermark_ended_at
        INTEGER initialized_at
        INTEGER updated_at
        INTEGER last_experience_run_at "opcional"
    }
    users |o--o{ auto_improve_proposal_events : "author_id (SET NULL)"
    auto_improve_proposals ||--o{ auto_improve_proposal_events : "proposal_id (CASCADE)"
    pages |o--o{ auto_improve_proposals : "applied_page_id (SET NULL)"
    users |o--o{ auto_improve_proposals : "decided_by_author_id (SET NULL)"
    pages |o--o{ auto_improve_proposals : "target_latest_page_id_at_stage (SET NULL)"
    projects ||--o{ auto_improve_proposals : "project_id (CASCADE)"
    workspaces ||--o{ auto_improve_proposals : "workspace_id (CASCADE)"
    auto_improve_runs ||--o{ auto_improve_proposals : "run_id (CASCADE)"
    auto_improve_proposals |o--o{ auto_improve_rejections : "source_proposal_id (SET NULL)"
    auto_improve_runs |o--o{ auto_improve_rejections : "source_run_id (SET NULL)"
    projects ||--o{ auto_improve_rejections : "project_id (CASCADE)"
    workspaces ||--o{ auto_improve_rejections : "workspace_id (CASCADE)"
    sessions |o--o{ auto_improve_runs : "session_id (SET NULL)"
    projects ||--o{ auto_improve_runs : "project_id (CASCADE)"
    workspaces ||--o{ auto_improve_runs : "workspace_id (CASCADE)"
    sessions ||--o| auto_improve_scheduler_claims : "session_id (CASCADE)"
    projects ||--o{ auto_improve_scheduler_claims : "project_id (CASCADE)"
    workspaces ||--o{ auto_improve_scheduler_claims : "workspace_id (CASCADE)"
    projects ||--o{ auto_improve_scheduler_state : "project_id (CASCADE)"
    workspaces ||--o{ auto_improve_scheduler_state : "workspace_id (CASCADE)"
    users |o..o{ auto_improve_proposals : "staged_by_actor_user: identidade do operador em texto, sem FK"
```

### 2.8 Workstreams gerenciados (continuidade entre harnesses)

```mermaid
erDiagram
    managed_runs {
        BLOB id PK
        BLOB workstream_id FK
        TEXT agent_kind
        TEXT lease_owner
        TEXT native_session_id "opcional"
        TEXT state
        INTEGER sync_after
        INTEGER sync_through
        INTEGER context_delivered
        INTEGER lease_expires_at
        INTEGER started_at
        INTEGER ended_at "opcional"
        INTEGER exit_code "opcional"
        INTEGER native_session_linked_at "opcional"
        TEXT owner_user "opcional"
        TEXT previous_agent_kind "opcional"
        TEXT previous_native_session_id "opcional"
    }
    workstream_events {
        BLOB workstream_id PK, FK
        INTEGER sequence PK
        TEXT event_id
        TEXT agent_kind
        TEXT native_session_id
        TEXT source_record_id "opcional"
        TEXT kind
        TEXT role "opcional"
        TEXT content
        TEXT occurred_at "opcional"
        TEXT metadata_json
        TEXT segment_path "opcional"
        INTEGER created_at
    }
    workstream_events_fts {
        FTS content "opcional"
        FTS role "opcional"
        FTS agent_kind "opcional"
    }
    workstream_native_sessions {
        BLOB workstream_id PK, FK
        TEXT agent_kind PK
        TEXT native_session_id PK
        INTEGER is_current
        TEXT source_cursor "opcional"
        INTEGER delivery_cursor
        INTEGER created_at
        INTEGER updated_at
    }
    workstreams {
        BLOB id PK
        BLOB workspace_id FK
        BLOB project_id FK
        TEXT repo_fingerprint
        TEXT worktree_fingerprint
        TEXT name
        TEXT cwd
        INTEGER created_at
        INTEGER selected_at
        INTEGER updated_at
    }
    workstreams ||--o{ managed_runs : "workstream_id (CASCADE)"
    workstreams ||--o{ workstream_events : "workstream_id (CASCADE)"
    workstreams ||--o{ workstream_native_sessions : "workstream_id (CASCADE)"
    projects ||--o{ workstreams : "project_id (CASCADE)"
    workspaces ||--o{ workstreams : "workspace_id (CASCADE)"
    workstream_events |o..o{ workstream_events_fts : "rowid: FTS de conteúdo externo, por triggers"
    users |o..o{ managed_runs : "owner_user: identidade do operador em texto, sem FK"
```

### 2.9 Perfil entre projetos

```mermaid
erDiagram
    profile_candidates {
        INTEGER id PK
        BLOB workspace_id FK
        BLOB project_id FK
        TEXT source_kind
        TEXT source_ref
        TEXT topic_key
        TEXT category
        TEXT statement
        TEXT quote
        TEXT applies_to
        INTEGER observed_at
        TEXT contributor "opcional"
        TEXT generality
        REAL confidence
    }
    profile_entry_ledger {
        BLOB workspace_id PK, FK
        BLOB project_id PK, FK
        TEXT path PK
        TEXT topic_key
        INTEGER written_at
    }
    profile_harvest_marks {
        BLOB workspace_id PK, FK
        BLOB project_id PK, FK
        INTEGER observations_until
        INTEGER pages_until
    }
    projects ||--o{ profile_candidates : "project_id (CASCADE)"
    workspaces ||--o{ profile_candidates : "workspace_id (CASCADE)"
    projects ||--o{ profile_entry_ledger : "project_id (CASCADE)"
    workspaces ||--o{ profile_entry_ledger : "workspace_id (CASCADE)"
    projects ||--o{ profile_harvest_marks : "project_id (CASCADE)"
    workspaces ||--o{ profile_harvest_marks : "workspace_id (CASCADE)"
```

## 3. Ligações lógicas (sem chave estrangeira)

O banco não garante estas ligações. A aplicação garante, ou elas existem de propósito sem garantia.

| Tabela.coluna | Aponta para | Por quê |
|---|---|---|
| `audit_log.workspace_id` | `workspaces` | sem FK: a auditoria sobrevive à purga |
| `audit_log.project_id` | `projects` | sem FK: a auditoria sobrevive à purga |
| `audit_log.page_id` | `pages` | sem FK: a auditoria sobrevive à remoção |
| `purged_scopes.workspace_id` | `workspaces` | lápide: o escopo já foi apagado |
| `purged_scopes.project_id` | `projects` | lápide: o escopo já foi apagado |
| `purged_sessions.session_id` | `sessions` | lápide: a sessão já foi apagada |
| `links.to_workspace` | `workspaces` | por NOME (nulo = workspace da origem) |
| `links.to_project` | `projects` | por NOME (nulo = projeto da origem) |
| `page_evidence.source_id` | `sessions` | polimórfica: source_kind = session |
| `page_evidence.source_id` | `observations` | polimórfica: source_kind = observation |
| `pages_fts.rowid` | `pages` | FTS de conteúdo externo, por triggers |
| `observations_fts.rowid` | `observations` | FTS de conteúdo externo, por triggers |
| `workstream_events_fts.rowid` | `workstream_events` | FTS de conteúdo externo, por triggers |
| `agent_messages.from_owner_user` | identidade do operador | chave de armazenamento da identidade (texto); **não** é FK para `users`, e continua válida para operadores sem linha em `users` |
| `agent_messages.claimed_by_user` | identidade do operador | chave de armazenamento da identidade (texto); **não** é FK para `users`, e continua válida para operadores sem linha em `users` |
| `auto_improve_proposals.staged_by_actor_user` | identidade do operador | chave de armazenamento da identidade (texto); **não** é FK para `users`, e continua válida para operadores sem linha em `users` |
| `handoffs.owner_user` | identidade do operador | chave de armazenamento da identidade (texto); **não** é FK para `users`, e continua válida para operadores sem linha em `users` |
| `handoffs.accepted_by_user` | identidade do operador | chave de armazenamento da identidade (texto); **não** é FK para `users`, e continua válida para operadores sem linha em `users` |
| `managed_runs.owner_user` | identidade do operador | chave de armazenamento da identidade (texto); **não** é FK para `users`, e continua válida para operadores sem linha em `users` |
| `sessions.actor_user` | identidade do operador | chave de armazenamento da identidade (texto); **não** é FK para `users`, e continua válida para operadores sem linha em `users` |

Outras colunas de identificador sem FK, que guardam ids **externos** (do agente ou do harness), não ids deste banco:
`managed_runs.native_session_id`, `managed_runs.previous_native_session_id`, `workstream_events.native_session_id`,
`workstream_events.event_id`, `workstream_events.source_record_id`, `workstream_native_sessions.native_session_id`
e `session_consolidation_jobs.claim_id` (token de reivindicação do job). `web_sessions.session_hash` é o hash do
segredo da sessão web, não uma referência.

A tabela de controle de migrações (`refinery_schema_history`: `version`, `name`, `applied_on`, `checksum`) não
aparece nos diagramas porque é criada pela ferramenta de migração, não pelas migrações. `wiki_migrations` guarda
as migrações de estrutura da wiki ([wiki-files-layer.md](wiki-files-layer.md) §10).

## 4. Efeito de apagar uma linha (regras `ON DELETE`)

Para cada tabela que é alvo de chave estrangeira: o que acontece nas filhas quando uma linha dela é apagada.
`CASCADE` apaga a filha; `SET NULL` só limpa a referência. A cascata continua nas filhas das filhas.

| Ao apagar em | Apaga em cascata | Só limpa a referência (`SET NULL`) |
|---|---|---|
| `auto_improve_proposals` | `auto_improve_proposal_events.proposal_id` | `auto_improve_rejections.source_proposal_id` |
| `auto_improve_runs` | `auto_improve_proposals.run_id` | `auto_improve_rejections.source_run_id` |
| `entities` | `entity_page_links.entity_id` | — |
| `pages` | `entity_page_links.page_id`, `links.from_page_id`, `page_abstract_embeddings.page_id`, `page_access.page_id`, `page_embed_failures.page_id`, `page_embeddings.page_id`, `page_evidence.page_id`, `page_feedback.page_id` | `auto_improve_proposals.applied_page_id`, `auto_improve_proposals.target_latest_page_id_at_stage`, `links.to_page_id`, `pages.supersedes`, `sessions.summary_page_id` |
| `projects` | `agent_messages.from_project_id`, `agent_messages.to_project_id`, `auto_improve_proposals.project_id`, `auto_improve_rejections.project_id`, `auto_improve_runs.project_id`, `auto_improve_scheduler_claims.project_id`, `auto_improve_scheduler_state.project_id`, `entities.project_id`, `handoffs.project_id`, `ingest_keys.project_id`, `observations.project_id`, `page_feedback.project_id`, `pages.project_id`, `profile_candidates.project_id`, `profile_entry_ledger.project_id`, `profile_harvest_marks.project_id`, `project_grants.project_id`, `purged_sessions.project_id`, `session_consolidation_jobs.project_id`, `sessions.project_id`, `workstreams.project_id` | — |
| `sessions` | `auto_improve_scheduler_claims.session_id`, `ingest_keys.session_id`, `observations.session_id`, `session_consolidation_jobs.session_id` | `agent_messages.claimed_by_session`, `agent_messages.from_session_id`, `auto_improve_runs.session_id`, `handoffs.accepted_by_session`, `handoffs.from_session_id` |
| `users` | `api_credentials.user_id`, `project_grants.user_id`, `web_sessions.user_id` | `audit_log.author_id`, `auto_improve_proposal_events.author_id`, `auto_improve_proposals.decided_by_author_id`, `page_feedback.author_id`, `pages.author_id`, `project_grants.granted_by`, `projects.created_by` |
| `workspaces` | `agent_messages.from_workspace_id`, `agent_messages.to_workspace_id`, `auto_improve_proposals.workspace_id`, `auto_improve_rejections.workspace_id`, `auto_improve_runs.workspace_id`, `auto_improve_scheduler_claims.workspace_id`, `auto_improve_scheduler_state.workspace_id`, `entities.workspace_id`, `handoffs.workspace_id`, `observations.workspace_id`, `page_feedback.workspace_id`, `pages.workspace_id`, `profile_candidates.workspace_id`, `profile_entry_ledger.workspace_id`, `profile_harvest_marks.workspace_id`, `project_grants.workspace_id`, `projects.workspace_id`, `purged_sessions.workspace_id`, `session_consolidation_jobs.workspace_id`, `sessions.workspace_id`, `workstreams.workspace_id` | — |
| `workstreams` | `managed_runs.workstream_id`, `workstream_events.workstream_id`, `workstream_native_sessions.workstream_id` | — |

- Apagar uma linha de `workspaces` remove, em cascata, linhas de: `agent_messages`, `auto_improve_proposal_events`, `auto_improve_proposals`, `auto_improve_rejections`, `auto_improve_runs`, `auto_improve_scheduler_claims`, `auto_improve_scheduler_state`, `entities`, `entity_page_links`, `handoffs`, `ingest_keys`, `links`, `managed_runs`, `observations`, `page_abstract_embeddings`, `page_access`, `page_embed_failures`, `page_embeddings`, `page_evidence`, `page_feedback`, `pages`, `profile_candidates`, `profile_entry_ledger`, `profile_harvest_marks`, `project_grants`, `projects`, `purged_sessions`, `session_consolidation_jobs`, `sessions`, `workstream_events`, `workstream_native_sessions`, `workstreams`.
- Apagar uma linha de `projects` remove, em cascata, linhas de: `agent_messages`, `auto_improve_proposal_events`, `auto_improve_proposals`, `auto_improve_rejections`, `auto_improve_runs`, `auto_improve_scheduler_claims`, `auto_improve_scheduler_state`, `entities`, `entity_page_links`, `handoffs`, `ingest_keys`, `links`, `managed_runs`, `observations`, `page_abstract_embeddings`, `page_access`, `page_embed_failures`, `page_embeddings`, `page_evidence`, `page_feedback`, `pages`, `profile_candidates`, `profile_entry_ledger`, `profile_harvest_marks`, `project_grants`, `purged_sessions`, `session_consolidation_jobs`, `sessions`, `workstream_events`, `workstream_native_sessions`, `workstreams`.
- Apagar uma linha de `pages` remove, em cascata, linhas de: `entity_page_links`, `links`, `page_abstract_embeddings`, `page_access`, `page_embed_failures`, `page_embeddings`, `page_evidence`, `page_feedback`.
- Apagar uma linha de `sessions` remove, em cascata, linhas de: `auto_improve_scheduler_claims`, `ingest_keys`, `observations`, `session_consolidation_jobs`.
- Apagar uma linha de `users` remove, em cascata, linhas de: `api_credentials`, `project_grants`, `web_sessions`.

Isso não substitui a purga: a purga grava a lápide (`purged_scopes`, `purged_sessions`) na mesma transação,
para que um evento atrasado não recrie o que foi apagado ([database-schema.md](database-schema.md) §11).
`audit_log` não tem chave estrangeira para escopo nem página, então sobrevive a qualquer exclusão.

## 5. Ordem de inserção (dependências)

Uma tabela só pode receber linhas depois das tabelas para as quais ela tem FK **obrigatória** (`NOT NULL`).
Níveis calculados por ordenação topológica; tabelas do mesmo nível não dependem umas das outras.
Autorreferências (`pages.supersedes`) e FKs opcionais não contam.

| Nível | Tabelas |
|---|---|
| 0 | `audit_log`, `bootstrap_chunk_progress`, `client_activity`, `human_auth_state`, `maintenance_scheduler_state`, `observations_fts`, `pages_fts`, `purged_scopes`, `users`, `wiki_migrations`, `workspaces`, `workstream_events_fts` |
| 1 | `api_credentials`, `projects`, `web_sessions` |
| 2 | `agent_messages`, `auto_improve_rejections`, `auto_improve_runs`, `auto_improve_scheduler_state`, `entities`, `handoffs`, `ingest_keys`, `pages`, `profile_candidates`, `profile_entry_ledger`, `profile_harvest_marks`, `project_grants`, `purged_sessions`, `sessions`, `workstreams` |
| 3 | `auto_improve_proposals`, `auto_improve_scheduler_claims`, `entity_page_links`, `links`, `managed_runs`, `observations`, `page_abstract_embeddings`, `page_access`, `page_embed_failures`, `page_embeddings`, `page_evidence`, `page_feedback`, `session_consolidation_jobs`, `workstream_events`, `workstream_native_sessions` |
| 4 | `auto_improve_proposal_events` |

## 6. O que é derivado da wiki e o que só existe no banco

A reindexação a partir do markdown ([wiki-files-layer.md](wiki-files-layer.md) §5.4) reconstrói **só**:
`workspaces`, `projects`, `pages` (versões correntes), `links`, `entities`, `entity_page_links` e `pages_fts`.
Todo o resto é estado só do banco (sessões, observações, handoffs, mensagens, usuários e credenciais,
auditoria, embeddings, feedback, acesso, evidências, auto-melhoria, workstreams, perfil, lápides e
agendadores) e precisa de backup do arquivo do banco. Versões antigas de página (`is_latest = 0`) também não
voltam numa reindexação: o histórico delas fica só no banco e no Git.
