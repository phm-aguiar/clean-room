# Lifecycle operations

> **Escopo:** este documento contém partes que dependem de LLM ou de embeddings. Essas partes são opcionais e não fazem parte do escopo inicial, que funciona sem LLM. Podem ser removidas ou adiadas junto com as funções correspondentes.

Reference for the destructive and state-changing `ai-memory` operations.
Read this before running anything that mutates the wiki or the database,
especially on a homelab where mistakes are harder to undo.

Operations come in two groups:

- **Server operations** go through the server's administrative HTTP API.
  The CLI is a thin client for them. They are safe to run while the server
  is running.
- **Direct-disk operations** (`reset`, `restore`, `reindex`) read and write
  the data directory without the server. They refuse to run while another
  `ai-memory` process is alive.

## Safety matrix

| Command | Safe while server runs? | Removes data? | Reversible? | Notes |
|---|---|---|---|---|
| `purge-project --confirm` | yes | the project's rows, wiki tree and raw workstream segments | no | Logical delete unless `--compact`. Refuses with 409 while a managed workstream under the project holds a live run lease; `--force` overrides. Sibling projects are untouched. |
| `purge-session --session-id <uuid> --confirm` | yes | one session and what it derived | no | Logical delete unless `--compact`. Scoped: a session outside the named workspace and project is a 404 and nothing is deleted. Handoffs the session only *accepted* are kept. |
| `handoffs --expire-all --confirm` | yes | no (state change only) | rows kept | Marks open handoffs in the scope as `expired`. Rows and summaries stay in the audit trail. |
| `rename-project --from --to` | yes | no | yes (rename back) | Changes the name only. Wiki directories are keyed by project id, so no page file moves. The scope manifests are rewritten and a checkpoint is taken. |
| `/admin/rename-workspace` | yes | no | yes (rename back) | Changes the name only. Manifests are refreshed and a checkpoint is taken. HTTP route; no CLI subcommand. |
| `/admin/delete-workspace` | yes | the workspace and every child project | no | Logical delete unless `compact: true`. HTTP route; no CLI subcommand. Supports a dry run. |
| `move-project --confirm` | yes | the source project only in the merge case | no | Fresh destination: lossless true move, same project id. Destination already has a same-named project: copy-then-purge merge, where only latest page versions migrate. |
| `move-session <id> --to <project> --confirm` | yes | no | yes (move it back, if nothing else changed) | Re-stamps one session, or every session touching a scope, into another project. Dry run by default. Refuses with 409 an open session or a pending consolidation job unless `--force`. |
| `backup --to <file>` | yes | no | not applicable | Writes a gzip tarball of a consistent snapshot of the database plus the wiki tree. |
| `checkpoints` | yes | no | not applicable | Lists recent wiki git checkpoints. Read-only. |
| `restore-page --path --from` | yes | overwrites one markdown page | yes (restore another checkpoint) | Restores one page from wiki git history and re-indexes it. Does not restore database-only state. |
| `restore --from <tarball> --force` | **no**, stop the server first | replaces `wiki/` and the database (and `config.toml` if the archive has it) | no, unless a backup was taken first | Validates and stages the archive before touching live data. Leaves `raw/`, `logs/` and `models/` alone. |
| `reset --confirm` | **no**, stop the server first | the contents of `wiki/`, `db/` and `raw/` | no | Keeps `config.toml`, `logs/` and `models/`. Without `--confirm` it only lists what would be removed. |
| `reindex` | **no**, stop the server first | no | only with a prior database backup | Rebuilds pages, links and the full-text index from `wiki/`. Refuses if the database already has rows in the tables it rebuilds. |

Global option: direct-disk commands accept the `--data-dir` option to point
at a data directory other than the configured one.

## What "deleted" means

Session purge, project purge and workspace delete answer *"forget this"*.
After a purge, the data is gone from the API, from the status counts and
from search. Removal is a
**logical delete** by default. This distinction matters when you are
answering a regulatory erasure request rather than tidying up.

| | default | `--compact` (`compact: true`) |
|---|---|---|
| Reachable through the API and MCP tools | no | no |
| Returned by search | no | no |
| Live wiki Markdown file | removed on a best-effort basis | removed on a best-effort basis |
| Bytes still present in the database file | **yes**, in free pages | no |
| Text still in the wiki git history | **yes** | **yes** |
| Present in backups taken before the purge | **yes** | **yes** |
| Cost | one transaction | rewrites the whole database file |

### What compaction does

`--compact` runs after the delete commits. It rebuilds the three full-text
indexes (observations, pages and workstream events) and then rewrites the
database file to release free pages.

- It rebuilds **all three** indexes on every compaction, whatever was deleted.
  An ordinary delete leaves the removed tokens inside the index segments, which
  is why a rebuild is needed.
- It takes an exclusive lock on the database for the duration. Every write
  waits.
- It needs free disk space roughly equal to the database's own size.
- It is opt-in because of the lock and the disk requirement. The duration
  depends on database size.

**Compaction is not forensic erasure.** The wiki git repository keeps page
content in its objects and in its commit messages. Any backup taken before the
purge still contains everything. Removing the bytes from the live database
file is worth doing on its own terms. Do not describe it to anyone as a
guarantee that the content is unrecoverable.

### Project and workspace purges

Every row of the table above applies unchanged to `purge-project --compact`
and to `/admin/delete-workspace` with `compact: true`. The table describes any
database delete, not one command: rows go, and the bytes stay in free pages
until the file is rewritten.

A managed workstream's event rows leave through the cascade from the project
(or workspace) row, not through a direct delete on that table. The cascade does
not fire the trigger that would remove the rows from the workstream-event index.
Compaction rebuilds that index too, so a managed agent's transcript text does
not stay searchable in the file after a reclaim.

### Concurrency and tombstones

**Session purge** holds the wiki's exclusive mutation lock from before the
database delete until the file cleanup finishes. In-flight page writes and
watcher re-indexes finish first. New ones wait until cleanup is done. This
also serializes the purge with project and session moves.

- Admission webhooks run before the lock is taken.
- The two git checkpoints taken around the purge are outside the lock. They
  bracket the purge; they do not snapshot it.
- With compaction, the rebuild and reclaim run inside the lock too. A purge
  on a busy server can therefore block wiki writes for longer than the delete
  itself.
- File cleanup failures do **not** roll back the committed database purge.
  They are reported in `files_failed`. The coordination does not provide
  crash-atomic rollback.

**Project purge** deletes the database rows in one transaction. The wiki
directory and the raw workstream directories are removed afterwards. The
directory removal takes the wiki's exclusive lock on its own. The database
delete and the directory removal are **not** under one lock (see the purge
section).

**Tombstones.** Each purge records what it removed, in the same transaction as
the delete:

- A session purge records the session id (`purged_sessions`). A later re-index or hook redelivery does not restore
  that session's page file or observations, even if the file cleanup failed.
- A project purge records the project scope (`purged_scopes`). A workspace delete records the
  whole workspace (`project_id = NULL` and per-project rows). A later re-index skips any directory whose scope is
  tombstoned and leaves its files in place, untouched. Re-index never deletes
  files.

### Scope containment

In the CLI, `purge-session` checks that the session belongs to the resolved `(workspace, project)` before purging. On the server (`POST /admin/purge-session`), the session is looked up by `session_id` (returning 404 if not found) and all deletes for its summary page (`sessions/<session_id>.md`) are scoped to that session's `workspace_id` and `project_id` so another project's page at the same relative path is never touched.

## Project isolation

Every project's data lives under a directory keyed by its UUID, not by its
name:

```text
<wiki_root>/
├── .git/
├── <workspace_id>/
│   ├── _meta.md                 # workspace name, for rebuilds
│   └── <project_id>/
│       ├── concepts/
│       ├── decisions/
│       ├── gotchas/
│       ├── sessions/
│       ├── _rules/
│       ├── _meta.md             # project name and repo path, for rebuilds
│       ├── log-YYYY-MM.md       # rolling event log, one file per month
│       └── bootstrap.md
└── <other_workspace_id>/
    └── <other_project_id>/
        └── ...
```

- The mutable project name (the human-readable name you see in the web
  interface) never appears in a disk path. The stable project id does. The
  database maps name to id per workspace.
- Two projects can have the same page path (for example both have
  `decisions/0001.md`) without colliding on disk.
- The git history is rooted at the wiki root: one repository, with every
  project as a subtree. `git log -- <workspace_id>/<project_id>/` shows one
  project's history.
- Each workspace directory has a `_meta.md`, and each project directory has a
  `_meta.md`. These small frontmatter manifests store human names (plus the
  repository path for projects). A database can be rebuilt from the UUID-keyed
  tree alone, which is what `reindex` relies on.

## Command by command

### `purge-project`

```bash
ai-memory purge-project --workspace default --project my-project --confirm

# Or target by UUID directly:
ai-memory purge-project --workspace default --project-id <uuid> --confirm

# Also reclaim the freed bytes (slow; rewrites the whole database file):
ai-memory purge-project --workspace default --project my-project --confirm --compact
```

Flags: `--workspace`, `--project` (or `--project-id`, mutually exclusive), `--confirm`, `--force`, `--compact`.

Without `--confirm` the CLI sends a dry run (with a 5-second timeout), prints the preview counts, and
exits with an error that tells you to re-run with `--confirm`. Nothing is
deleted.

What the server does, in order:

1. **Confirmation / Dry run.** If `dry_run` is false and `confirm` is false, returns 400. `dry_run: true` wins over `confirm`.
2. **Scope lookup.** The workspace is looked up by name and the project by `project` name or `project_id` UUID (mutually exclusive; passing both or neither returns 400/422). Neither is created. Either missing returns 404.
3. **Lease check (applies to both dry-run and real run).** A managed run blocks the purge while it holds a
   **live** lease (`state = 'active'` and `lease_expires_at > now`). If a live lease exists and `force` is not set, the call returns
   409 **even when `dry_run: true`**. The body is just `{"error": "project has <n> active managed run(s) in workstream(s) '<nome>' (<agente>), …; finish or cancel them first, or pass force to purge anyway (the running agent will stop being able to save its history)"}`: there is **no** `active_workstreams` field (audit 008 corrected the earlier text); the names are sorted, deduplicated, and each one is `'<workstream>' (<agent>)`. Liveness is `state = 'active'` **and** `lease_expires_at > now` (a crashed wrapper leaves `active` rows behind that nothing flips until the next prepare, so `state` alone would block purges forever). With `force`, the purge or preview proceeds and the lease rows are deleted by the cascade; the still-running agent then gets 404/409 on its heartbeat and its transcript never reaches the ledger.
4. **Dry run return.** If `dry_run: true`, the server computes the exact row counts via read-only queries and returns the report with `"dry_run": true`, `"files_deleted": []`, and `"files_failed": []`. It runs no admission webhook, takes no checkpoint, and deletes nothing.
5. **Admission.** The `purge_project` admission webhooks run before any
   deletion. A reject-policy webhook that fails aborts with 500 and nothing is
   deleted.
6. **Checkpoint.** If the wiki tree has uncommitted changes, a checkpoint (`pre-purge-project <ws>/<proj>`) is
   committed first. If that fails, the call returns 500 and nothing is
   deleted.
7. **Database delete.** One transaction deletes dependent rows and the project row (`pages`, `sessions`, `observations`, `handoffs`, `page_embeddings`, `workstreams`, `managed_runs`, `consolidation_jobs`, `auto_improve_*`, `project_grants`, `profile_candidates`). The workspace row stays. The
   scope is tombstoned in `purged_scopes` and an audit row (`op = "purge_project"`) is written, attributed to the
   authenticated user when there is one. Compaction runs after the transaction if requested.
8. **File cleanup.** The project's wiki directory and each of its raw
   workstream segment directories are removed under the wiki's exclusive lock. Each path is reported in
   `files_deleted` or `files_failed`.
9. **After.** The active-project pointer is cleared if it pointed at this
   project. The scope cache is invalidated. Purge webhooks are dispatched, with
   `partial_failure` set if any file removal failed. A post-purge checkpoint (`purge-project <ws>/<proj>`) is
   committed if the tree changed.

Report fields: `label`, `pages_deleted`, `sessions_deleted`,
`observations_deleted`, `handoffs_deleted`, `embeddings_deleted`,
`workstreams_deleted`, `managed_runs_deleted`, `workstream_ids`,
`collateral_observations_deleted`, `collateral_handoffs_denulled`,
`compacted`, `files_deleted`, `files_failed`, `pre_checkpoint`, `checkpoint`, `dry_run` (when true).

Collateral counts are about other projects. `collateral_observations_deleted`
counts observations stamped into another project by this project's sessions.
They are deleted with those sessions. `collateral_handoffs_denulled` counts
handoffs in other projects that were authored or accepted by this project's
sessions. They keep their text and lose the link.

Failure modes:

- **Workspace or project not found**: 404, nothing changed.
- **No `confirm` (when `dry_run: false`)**: 400, nothing changed.
- **Live managed run and no `force` (even on `dry_run: true`)**: 409, nothing changed. Finish or cancel
  the session, or re-run with `force`. Forcing stops the running agent from
  saving its history.
- **Admission rejected, or the pre-purge checkpoint failed**: 500, nothing
  changed.
- **File removal partly failed** (for example a permissions problem): the
  database rows are already gone and the call returns 200 with `files_failed`
  filled in. Remove the listed paths by hand. A second call with the same
  arguments returns 404, because the project no longer exists.

### `purge-session`

```bash
ai-memory purge-session \
  --workspace default --project my-app \
  --session-id 0199f3d2-1c4e-7a10-9f3b-2b0c5d8e7a11 \
  --confirm
```

Flags: `--workspace`, `--project`, `--session-id` (a full UUID; a prefix or a
workstream id is rejected), `--confirm`, `--compact`.

Without `--confirm` the CLI sends a dry run (5-second timeout), prints the preview, and exits with
an error. Nothing is deleted.

What the server (`POST /admin/purge-session`) does, in order:

1. **Validation.** `session_id` must be a valid UUID (otherwise 400). If `dry_run` is false and `confirm` is false, returns 400. `dry_run: true` always wins over `confirm`.
2. **Session lookup.** The server looks up the session row by `session_id` and resolves its owning `(workspace_id, project_id)` and human `(workspace, project)` names. If the session does not exist (or was already purged), returns 404.
3. **Dry run.** When `dry_run: true`, executes read-only `COUNT(*)` queries inside the session's scope and returns the preview report (`dry_run: true`, `files_deleted: []`, `files_failed: []`). No admission, no checkpoint, no file removal, no audit row.
4. **Admission.** The `purge_session` admission webhooks run before any
   deletion. A reject-policy failure returns 500 with nothing deleted.
5. **Checkpoint.** A pre-purge checkpoint (`pre-purge-session <session_id>`) is committed if the tree is dirty.
   Failure returns 500 with nothing deleted.
6. **Exclusive lock & Database delete.** Under the wiki's exclusive mutation lock, one transaction deletes:
   - `profile_candidates` where `source_kind = 'prompt'` and `source_ref = 'session:<session_id>'`;
   - all page versions at `path = 'sessions/<session_id>.md'` in the session's `(workspace_id, project_id)` (plus any `summary_page_id` of the session), with their links, entities, evidence and embeddings;
   - the session's `auto_improve_rejections`, `auto_improve_runs`, `auto_improve_claims`, and `consolidation_jobs`;
   - handoffs the session **authored** (`from_session_id`), while clearing `accepted_by_session = NULL` on handoffs it only accepted;
   - all `observations` of the session (counting same-scope vs cross-scope `collateral_observations_deleted`);
   - the `sessions` row itself.

   It also inserts the session into `purged_sessions` (tombstone) and writes an `audit_log` row (`op = "purge_session"`). Compaction runs inside the lock if requested.
7. **File cleanup.** Only the page paths the database step returned (`sessions/<session_id>.md`), under the
   scope's own root, are removed. Removal is reported in `files_deleted` and
   `files_failed`. A failed removal does **not** roll back the database purge.
   The call still returns 200, and async webhooks receive `partial_failure: true`.
8. **After.** A post-purge checkpoint (`purge-session <session_id>`) is committed if the tree changed.

**Kept on purpose:**

- Handoffs the session only *accepted*. Their text belongs to the session that
  wrote them. They lose only the pointer to the accepting session.
- Pages written during the session that are not its session page (decisions,
  gotchas). Pages are not tracked per session.

Report fields: `session_id`, `workspace`, `project`, `observations_deleted`,
`handoffs_deleted`, `pages_deleted`, `auto_improve_runs_deleted`,
`removed_paths`, `collateral_observations_deleted`,
`collateral_handoffs_denulled`, `files_deleted`, `files_failed`, `compacted`,
`pre_checkpoint`, `checkpoint`, `dry_run` (when true).

### `rename-project`

```bash
ai-memory rename-project --workspace default --from old-name --to new-name
# Or by UUID:
ai-memory rename-project --workspace default --project-id <uuid> --to new-name
```

Steps:

1. **Lookup.** Missing workspace or source project (`from` or `project_id`, mutually exclusive) returns 404.
2. **Validate names.** Both source and target names are checked. An empty or
   whitespace-only target name, a name > 256 bytes or containing `/` or control characters, or a reserved profile project name (`_profile` or `_profile.*`, as either source or target) returns 422. Leading and
   trailing whitespace on `to` are trimmed silently.
3. **Update.** If `from == to`, the rename succeeds immediately as a no-op (200 OK). If `to` is already used by another project in the same workspace, the call returns **409 Conflict** (`project already exists`). Otherwise the name is updated in `projects`.
4. **Count.** The number of latest page versions is counted for the report.
5. **Manifests, cache and checkpoint.** The scope cache is invalidated, the workspace and project `_meta.md` manifests
   are rewritten, and a checkpoint (`rename-project <workspace_id>/<project_id>`) is committed if the tree changed.

Zero page files move, because disk paths are keyed by project id.

If step 5 fails after the rename committed, the call still returns 200. The
response carries `manifest_warning` and no checkpoint. The database rename
stands.

Response: `workspace`, `from`, `to`, `pages`, `checkpoint`, `manifest_warning`.

### `/admin/rename-workspace`

HTTP admin route. It changes `workspaces.name` and leaves every on-disk path
unchanged, because paths are keyed by workspace id.

1. Missing source workspace returns 404.
2. An empty/invalid target name returns 422. A target name already taken by another workspace returns **409 Conflict** (renaming a workspace to its own current name `from == to` is a 200 OK no-op).
3. Otherwise the name is updated. Then every scope manifest under that workspace is refreshed.
4. If the manifest refresh fails after the rename committed, the response is
   still 200. It has `manifests_refreshed: 0` and a `manifest_warning`. The
   database rename stands.
5. The scope cache for the workspace is invalidated, and a checkpoint (`rename-workspace <from> -> <to>`) is
   committed if the tree changed.

### `/admin/delete-workspace`

HTTP admin route. There is no CLI subcommand for it. Request fields:
`workspace`, `confirm` (default `false`; **required** when `dry_run: false`), `force` (default `false`), `compact` (default `false`) and
`dry_run` (default `false`).

1. **Confirmation.** When `dry_run: false` and `confirm: false`, returns 400.
2. **Lookup.** Missing workspace returns 404. The lookup never creates one.
3. **Non-empty check (applies to both dry-run and real run).** Without `force: true`, a workspace that still has projects
   returns **409 Conflict** (including `projects_count`), even when `dry_run: true`.
4. **Dry run.** When `dry_run: true` (and either workspace is empty or `force: true`), returns the preview counts (`dry_run: true`, `files_deleted: []`, `files_failed: []`) and changes nothing.
5. **Tree check.** If the workspace's wiki tree is not valid, the call returns
   500 and nothing changes.
6. **Admission.** The `purge_workspace` admission webhooks run before any
   deletion. A reject-policy failure returns 500 and nothing changes.
7. **Checkpoint.** A pre-delete checkpoint (`pre-delete-workspace <ws>`) is committed if the tree is dirty.
   Failure returns 500 and nothing changes.
8. **Database delete.** One transaction deletes all child projects and the workspace row (`workspaces`, `projects`, `pages`, `sessions`, `observations`, `handoffs`, `page_embeddings`, `workstreams`, `managed_runs`, etc.). The whole workspace is tombstoned in `purged_scopes` (`project_id = NULL` plus per-project rows). Note: `delete-workspace` does not check `managed_runs` leases and does not write to `audit_log`.
9. **File cleanup.** The workspace's wiki directory and each affected raw
   workstream directory are removed under the wiki's exclusive lock. Failures are listed in `files_failed`.
10. **After.** Mirror notifications are dispatched after the durable work, with
    `partial_failure` set when a file removal failed. Active-project pointers
    for the workspace are cleared, the scope cache is invalidated, and a
    post-delete checkpoint (`delete-workspace <ws>`) is committed if the tree changed.

Response fields: `workspace`, `projects_deleted`, `pages_deleted`,
`sessions_deleted`, `observations_deleted`, `handoffs_deleted`,
`embeddings_deleted`, `collateral_observations_deleted`,
`collateral_handoffs_denulled`, `workstreams_deleted`,
`managed_runs_deleted`, `workstream_ids`, `files_deleted`, `files_failed`,
`compacted`, `pre_checkpoint`, `checkpoint`, `dry_run`.

### `move-project`

```bash
ai-memory move-project --from-workspace default --project my-project \
  --to-workspace other-workspace --confirm
```

Flags: `--from-workspace`, `--project` (or `--project-id`, mutually exclusive), `--to-workspace`, `--confirm`,
`--force`, `--on-conflict` (`block` (default), `overwrite` or `duplicate`).

Without `--confirm` the CLI refuses with a message. It sends no preview.

Unlike `rename-project`, which only changes a name, this crosses the workspace
boundary. The server checks, in order:

1. **Confirmation.** No `confirm` returns 400.
2. **Same workspace.** A source and destination workspace that are identical
   return 422. Use `rename-project` instead.
3. **Private profile.** A private cross-project profile project (`_profile.*`) stays in the
   default workspace and cannot be moved. It returns 422.
4. **Source lookup.** Missing source returns 404.
5. **Active-session guard.** If this project is the one the hook router has
   published as the active project, the call returns 409 unless `force` is
   set. A live session's next observation would otherwise carry a stale
   workspace id.
6. **Merge detection.** The destination workspace is checked for a project with
   the same name. If there is one, the merge path runs. Otherwise the true move
   runs.
7. **Checkpoint.** A pre-move checkpoint (`pre-move-project <from_ws>/<proj> -> <to_ws>/<proj>`) is committed first. If that
   fails, the call returns 500 and nothing moves.

The reported `moved_via` tells you which strategy ran.

#### True move (fresh destination)

When the destination workspace has no project with the same name, the move is
lossless and cheap. The project keeps its id.

1. The destination workspace row is created if needed.
2. The destination project directory `<wiki>/<to_ws_id>/<proj_id>` must not exist on disk. If it does, the call
   returns 409 and nothing moves.
3. The `move_project` admission webhooks run with the source and destination
   names. A reject-policy failure returns 500 and nothing moves.
4. In **one database transaction**, the workspace id is re-stamped on: the project row,
   pages, links (`to_workspace` where matching), sessions, observations, handoffs, messages, audit rows, project grants,
   auto-improve runs, proposals, rejections, scheduler state and claims,
   consolidation jobs and managed workstreams. The project id is unchanged.
5. The project directory is then moved on disk into the destination workspace's tree,
   under the wiki's exclusive lock. If the disk move fails, the database workspace id is rolled back.
6. The active-project pointer is retargeted to the new workspace. The scope
   cache is invalidated. Manifests are refreshed, and a post-move checkpoint (`move-project <from_ws>/<proj> -> <to_ws>/<proj>`) is
   committed if the tree changed.

Report: `moved_via: "true-move"`, `merged_into_existing: false`, `pages_copied` (the number of pages moved),
`workstreams_moved`, `source_purged: false`.

#### Copy-then-purge merge (destination has a same-named project)

Two different project ids cannot be re-stamped into one, because that would
collide on the unique name within the workspace. The merge copies the source
into the existing destination project, then purges the source.

1. The destination project is resolved.
2. The source's **latest** page versions are listed. Older versions are not
   copied. The git history keeps them.
3. Each source page is compared with the page at the same path in the
   destination. A page conflicts when its body, its frontmatter (ignoring `generated.at`), its title, its
   tier or its pinned flag differs. Identical pages simply supersede at the same path.
4. The `on_conflict` policy decides what happens to conflicts:
   - **`block`** (default): if any conflict exists, nothing is copied. The call
     returns 409 with a `conflicts` list of the paths. The source is intact.
   - **`overwrite`**: the source version becomes the latest version at the
     same path. The destination's previous version stays in history.
   - **`duplicate`**: the source page is written to
     `<stem>-from-<source-workspace-slug>.<ext>`. If that is taken, it tries
     `<stem>-from-<source-workspace-slug>-1.<ext>`, then `-2`, `-3`, and so on. The
     slug keeps ASCII letters, digits, `-` and `_`, and replaces every other
     character with `-`.

   Every conflict is listed in the response `conflicts` array as
   `path` to `moved_to`. For `overwrite`, `moved_to` equals `path`.
5. Each page is copied through the normal write path.
6. Embeddings computed with the currently configured embedding model are
   carried over as stored vectors.
7. **Copy failure** (any page): the call returns 500 before the source is
   purged. The source is intact.
8. **Unreadable or invalid source page**: the page is skipped and listed in
   `pages_skipped`. The source purge does **not** run, because it would destroy
   data that was not copied. The call commits `move-project-partial <from_ws>/<proj> -> <to_ws>/<proj>` and returns 200 with `source_purged: false`.
9. **Source purge.** This runs only when `pages_skipped` is empty. The `purge_project`
   admission webhooks run on the source first, and a reject-policy failure
   returns 500 with the source intact. The purge then runs without `force` and
   without compaction. A live managed run under the source returns 409, with the
   count of pages already copied.
10. **After the purge.** The source's wiki and raw directories are removed.
    Failures are listed in `files_failed`, and the purge webhook gets
    `partial_failure: true`. The active-project pointer for the source is
    cleared, the scope cache is invalidated, manifests are refreshed, and a
    post-move checkpoint (`move-project <from_ws>/<proj> -> <to_ws>/<proj>`) is committed.

Report: `moved_via: "copy-purge"`, `merged_into_existing: true`,
`pages_copied`, `pages_skipped`, `source_purged`, the `source_*_deleted`
counts, `files_deleted`, `files_failed`, `conflicts`, `pre_checkpoint`,
`checkpoint`, `manifest_warning`.

**What does not survive a copy-merge.** The source's sessions, observations and
handoffs are removed by the source purge. That is the raw capture log for the
source project. The copied pages start a new supersession chain inside the
database. Their earlier history stays in the wiki git history. A true move loses
none of this.

The `--force` flag overrides only the active-project guard. It never overrides
a live managed run.

> **Operational caveat: moving the project your current session writes to.**
> Lifecycle hooks stamp observations into the session's project on every
> supported lifecycle event. If you move that project mid-session, the next hook
> re-creates the source under the old workspace. Before moving a live project,
> point the repository's `.ai-memory.toml` marker at the **destination**
> workspace first. New hook events then land in the destination, and the move is
> a clean operation with no contention.

Failure modes:

- **No `confirm`**: 400, nothing changed.
- **Same source and destination workspace**: 422.
- **Private profile project**: 422.
- **Source not found**: 404.
- **Active session's project without `force`**: 409, nothing changed.
- **True move: destination project directory already exists**: 409, nothing
  changed.
- **True move: admission rejected, or the database step failed**: 500, nothing
  committed. If a rollback also failed, the error includes the manual repair.
- **Merge: `block` policy with conflicts**: 409, nothing copied, with the
  conflicting paths listed.
- **Merge: a page copy failed**: 500, source intact.
- **Merge: a page was unreadable**: 200 with `source_purged: false` and the
  paths in `pages_skipped`.
- **Merge: live managed run under the source**: 409, source intact.

### `move-session`

```bash
# One session, with its page and history (dry run: no --confirm)
ai-memory move-session 0192b6a1-4c2e-7d3f-8a5b-1234567890ab --to NAS_general

# Apply
ai-memory move-session 0192b6a1-4c2e-7d3f-8a5b-1234567890ab --to NAS_general --confirm

# Every session of a stray project, into another workspace, page regenerated later
ai-memory move-session --from-project tmp --to NAS_general --to-workspace home \
  --pages regenerate --confirm
```

Flags: a positional session id, or `--from-project` (with optional
`--from-workspace`); `--to`; `--to-workspace`; `--pages` (`move`, the default,
or `regenerate`); `--confirm`; `--force`; `--create`. The CLI resolves
`--from-project` like the other scope arguments. `--to` and `--to-workspace`
are taken literally.

Use it when a session was captured under the wrong project. Examples are a
`cd` into a temporary directory before sticky routing, a subagent started in a
scratch directory, or a repository cloned under a temporary name. Its
observations then belong with the project they are about. Unlike `reorg`, which
re-derives every session's project from its working directory, this command
names the destination and leaves the rest of the store alone.

Two shapes share the same endpoint (`POST /admin/move-session`):

- **Single**: `session_id`, plus `to_project` (or `project`) and optional
  `to_workspace` (or `workspace`, default `default`).
- **Batch**: `session_ids` (array of UUIDs; the CLI resolves `--from-project` by listing sessions first and sending `session_ids`),
  plus the destination (`to_workspace`, `to_project`).

Exactly one of `session_id` (non-empty) or `session_ids` (non-empty array) must be given. Otherwise the call
returns 400.

**Dry run by default.** Without `--confirm` (`confirm: false`), the server runs a read-only preview (`Preview` mode)
and returns exact counts (`dry_run: true`). Nothing is written and no audit row is left.
With `--create` (`create: true`) and a destination that does not exist, the dry run reports
`would_create_project: true` and the counts without creating the project.
Only the `--confirm` (`confirm: true`) run creates it.

**Guards.** If the session is still open (`ended_at IS NULL`) and is not a re-home, or has a pending/running consolidation job, the server returns 409 (`open_session` or `active_consolidation`) in both `Preview` and `Execute` modes (note: `force` does not bypass this guard in the server store).

**Re-home.** If the session row already sits in the destination (`from == to`), the session
does not move (`session_moved: false`), and open sessions are allowed so scattered observations in other scopes can be gathered into the destination.

**What moves, per session, in one transaction:**

- the session row (`workspace_id`, `project_id`);
- every observation of the session, including ones that landed in another scope;
- `profile_candidates` (`source_kind = 'prompt' AND source_ref = 'session:<id>'`) and `ingest_keys` (`key LIKE '%:' || <session_id> || ':%'`), using `UPDATE OR IGNORE` followed by `DELETE` of colliding leftovers;
- the handoffs it authored (`from_session_id`);
- its consolidation jobs, auto-improve runs and scheduler claims;
- its session page, according to `pages` (`move` [default], `keep`, or `regenerate`).

**Page handling:**

- `move` (default): if the destination already has a latest page or a file on disk at `sessions/<id>.md`, the move **does not fail with 409**; instead, it records `page_conflict = true` (`pages_moved = 0`, `page: "conflict (kept in source; pass --pages regenerate to replace)"`) and moves the session and dependent rows while leaving the page in the source. Otherwise, every version of `sessions/<id>.md` (and `links.from_page_id` / `to_page_id`) is re-stamped into the destination, and the file is moved on disk into the destination project's directory (removing any tombstone in `purged_sessions`).
- `keep`: leaves the session page in the source project (`page: "kept in source"`).
- `regenerate`: the source versions are retired (`is_latest = 0`, `valid_to = now`), any
  `sessions.summary_page_id` pointer to them is cleared, entity links are closed, and the file on disk is removed so the next consolidation writes a fresh page in the destination.

**What does not move:**

- `sessions.cwd`. It stays as recorded. The response carries `cwd_warning` when
  its basename differs from the destination project name.
- Other pages written during the session (decisions, gotchas).
- Handoffs the session *accepted*.
- Auto-improve proposals.
- Entities and page feedback.

A `--confirm` run takes a checkpoint before (`pre-move-session ...`) and after (`move-session ...`), invalidates the scope cache, and refreshes `_meta.md` manifests.

**Batch.** Each session in `session_ids` moves in its own transaction. The batch stops at the first error and returns the error plus `completed: [...]` with the sessions already moved.

Response (single): `session_id`, `dry_run`, `session_moved`, `would_create_project`
(dry run with `create` only), `source_scopes`, `from` and `to` (each with
`workspace` and `project`), `summary` (`observations`, `handoffs`,
`consolidation_jobs`, `auto_improve_runs`, `auto_improve_claims`,
`page_versions_moved`, `pages_regenerated`), `page`,
`cwd`, `cwd_warning`, `pre_checkpoint`, `checkpoint`, `manifest_warning`.

The batch wraps the per-session reports in `{dry_run, count, results: [...], pre_checkpoint?, checkpoint?, manifest_warning?}`.

### `checkpoints`

```bash
ai-memory checkpoints
ai-memory checkpoints --json
```

Lists recent wiki git commits, newest first. The default is 20 entries. The
server accepts 1 to 100. The short id is enough for `restore-page`. `--json`
includes the full id.

Use it to find the checkpoint just before a bad page write, delete, purge, move
or restore, or to inspect wiki history without opening the server's git
repository.

At startup, an existing data directory whose wiki repository has no commits
gets a one-time `upgrade baseline` checkpoint. This records the existing tree
before any recovery checkpoints. A fresh, empty install has no commit until
there is content to save.

### `restore-page`

```bash
ai-memory restore-page --workspace default --project my-project \
  --path notes/foo.md --from <checkpoint>
```

Flags: `--path`, `--from` (a checkpoint id, sent as `"rev"` in the HTTP JSON body), `--workspace`, `--project`, `--json`.

Steps:

1. **Path check.** An invalid page path returns 422 (checked before scope lookup).
2. **Scope lookup.** Missing workspace or project returns 404. Nothing is created.
3. **Pre-restore checkpoint.** If the tree has uncommitted changes, a checkpoint
   named `pre-restore-page <ws>/<proj>:<path>` is committed first.
4. **Read and validate.** The page's markdown is read from git at `<rev>:<ws_id>/<proj_id>/<path>`, decoded as UTF-8 and parsed. The live file is not touched yet.
5. **Write.** The live file is replaced atomically via the standard wiki write pipeline, and the page is re-indexed.
6. **Post-restore checkpoint.** A `restore-page <ws>/<proj>:<path> from <rev>` checkpoint is committed if the
   live tree changed.

Failure modes:

- **Invalid page path**: 422, nothing changed.
- **Workspace or project not found**: 404, nothing changed.
- **Checkpoint or file not found** in git, or historical markdown malformed: 500. Any pre-restore checkpoint
  stays as an audit breadcrumb.

Not recovered: sessions, observations, handoffs, users, audit rows, access
counters and embeddings. These live only in the database. Roll them back with a
full `backup` and `restore`.

### `backup`

```bash
ai-memory backup --to /tmp/ai-memory-backup.tar.gz
```

Flag: `--to` (short form `-o`).

What the server does:

1. Validates the wiki tree. An invalid tree returns 500.
2. Takes a consistent snapshot of the live database without stopping the writer (`VACUUM INTO`).
3. Writes a gzip tarball containing `wiki/`, the snapshot as `db/memory.sqlite`,
   and `config.toml` if it exists.
4. Returns the tarball as the response body, with content type
   `application/gzip` and `Content-Disposition: attachment; filename="backup.tar.gz"`.

The CLI writes the body to `--to`. A failure returns 500 with a JSON error body.
This is the standard snapshot before a risky operation.

Restoring is the reverse. Stop the server, then restore:

```bash
# Stop the server first.
docker compose -f ~/deploy/ai-memory/docker-compose.yml down
# Restore. The restore refuses if an ai-memory process is still running.
ai-memory restore --from /tmp/ai-memory-backup.tar.gz \
  --data-dir /var/opt/docker/utils/ai-memory/data --force
# Start back up.
docker compose -f ~/deploy/ai-memory/docker-compose.yml up -d
```

With a containerized deployment, `--data-dir` points at the host-side path of
the data volume, because `restore` runs directly on the disk.

### `restore`

```bash
ai-memory restore --from <tarball> --data-dir <path> --force
```

Flags: `--from` (short `-i`), `--force`, and the global `--data-dir`. There is no
`--confirm` flag.

A direct-disk operation. The server must be stopped.

Order of operations:

1. **Process check.** If any other `ai-memory` process is alive, the command
   refuses with the message "refusing to restore: N other ai-memory process(es)
   running (pids: ...). Stop them first, then re-run." Nothing changes.
2. **Source check.** The tarball must exist as a file.
3. **Occupied check.** The data directory counts as occupied if `wiki/` has any
   entries or `db/memory.sqlite` exists. If it is occupied and `--force` is not
   set, the command refuses: "refusing to restore: data dir at ... is non-empty
   (pass --force to overwrite)".
4. **Stage.** The tarball is extracted into a scratch directory `.restore-staging-<unix_secs>-<pid>` inside the data
   directory. Each archive entry is checked:
   - Entry types allowed: regular files (`'0'`, `'\0'`), directories (`'5'`), and **GNU Sparse (`'S'`)** entries (which GNU `tar` emits for SQLite files with zero-runs). Symlinks, hardlinks, FIFOs, devices, and PAX/GNU long-name/global headers are rejected.
   - Path allowlist: only `config.toml`, `db`, `db/memory.sqlite`, `wiki`, and `wiki/...` are allowed (after stripping a leading `./`). Absolute paths, `..`, Windows drive prefixes, and any other paths are rejected.
   - The archive must contain `db/memory.sqlite`. A failure here cleans up the staging directory and leaves the live data untouched.
5. **Validate the database.** The staged database is opened (applying any pending migrations) and **explicitly closed before the swap** (crucial on Windows, where open SQLite/WAL handles prevent renaming the parent directory). A corrupt snapshot fails here, while the live data is still untouched.
6. **Swap.** Existing `wiki/` and `db/` directories, and `config.toml` if the
   archive has one, are retired into `.restore-previous-<unix_secs>-<pid>`. The staged `wiki/`, `db/`, and optional `config.toml` are renamed into place. If any step fails, the moves already made are reversed from `.restore-previous-*`.
7. **Discard & Scratch cleanup.** Both `.restore-previous-*` and `.restore-staging-*` are removed. If removing either directory fails after a successful swap, a warning is logged to stderr and the restore still succeeds.

`raw/`, `logs/` and `models/` are never touched by a restore.

### `reset`

```bash
ai-memory reset --confirm
```

A direct-disk operation. It refuses while another `ai-memory` process is alive,
and this check runs **even for the dry run**.

- Without `--confirm`, it prints each path it would remove, then
  `(dry-run; pass --confirm to wipe)`, and exits successfully. Nothing is removed.
- With `--confirm`, it removes the contents of `wiki/`, `db/` and `raw/` under
  the configured data directory. Each directory is removed and recreated empty.
  Directories that do not exist are skipped.
- `config.toml`, `logs/` and `models/` are kept.

The removal is not atomic. If a directory cannot be removed, the command stops
with an error naming that path. Directories before it may already be wiped.

### `reindex`

```bash
ai-memory reindex --data-dir <path>
```

A direct-disk operation. It rebuilds the database from the wiki's markdown files.
It is a rebuild path, not a repair of a dirty index.

Order of operations:

1. **Process check.** If any other `ai-memory` process is alive, refuse.
2. **Clean-target check.** If the database already has rows in any of these **5 tables** —
   `workspaces`, `projects`, `pages`, `sessions`, `observations` — the command refuses and names the non-zero counts (`refusing to reindex: target database at ... is not empty (...)`). This stops stale database-only state from surviving
   silently.
3. **Walk the tree.** First validates the wiki tree for symlinks/reparse points (`validate_wiki_tree`). Then each `<workspace_id>/<project_id>/` directory is visited. Directories whose scope
   is tombstoned in `purged_scopes` are skipped (`skipped_purged`) and left in place.
4. **Workspaces and projects.** Each one is recreated from its `_meta.md`
   manifest, keeping the UUIDs encoded in the directory names. A missing
   manifest is an error that gives its exact path.
5. **Pages.** The latest page rows, page links and the full-text index are
   rebuilt from the markdown (`reindex_page`). Pages under `sessions/<id>.md` whose session id is in `purged_sessions` are skipped (`skipped_purged_sessions`). Embeddings are not computed. Run `embed` afterwards
   if you need them.
6. **Collisions.** A page whose path differs from an already indexed page only by
   letter case or Unicode normalization is logged and skipped (`skipped_collisions`), because such paths cannot
   coexist on macOS or Windows.

What is rebuilt: workspaces, projects, latest pages, page links and the full-text
index.

What is not rebuilt: sessions, observations, handoffs, users and tokens, audit
rows, access counters and embeddings. These are database-only state. Keep a
backup if you need them.

Every scope directory carries a `_meta.md` manifest, which `reindex` reads for the
workspace and project names. The manifest is written with the scope's first page,
so a project that first appears while the server is running is rebuildable from
that moment on. The startup backfill runs on every boot. It repairs a tree written
by an older release, or one whose manifests were removed by hand.

If `reindex` reports a missing manifest, start the server once against that data
directory, so the backfill writes it. Then stop the server and run `reindex`
again.

Procedure to move a data directory onto a clean migration lineage:

1. Stop the server or container.
2. Back up the current data directory.
3. Move or remove `<data-dir>/db/memory.sqlite` and its write-ahead log and shared
   memory sibling files.
4. Run `ai-memory reindex --data-dir <data-dir>`.
5. After restart, run `ai-memory embed` if you need embeddings rebuilt.

## Operator workflows

### Fresh start (wipe everything)

For a container deployment with data on the host:

```bash
ssh homelab
cd ~/deploy/ai-memory
docker compose down
sudo rm -rf /var/opt/docker/utils/ai-memory/data/*
docker compose up -d
```

Or through the CLI, from any machine (slower, but portable):

```bash
docker stop ai-memory         # so the process check passes
ai-memory reset --confirm     # against the same data directory
docker start ai-memory
```

### Snapshot before a risky operation

```bash
ai-memory backup --to "/tmp/ai-memory-$(date +%Y%m%d-%H%M).tar.gz"
# ... do the risky thing ...
# ... something broke ...
docker compose down
ai-memory restore --from /tmp/ai-memory-2026-05-23-1530.tar.gz --force
docker compose up -d
```

### Drop one experimental project, keep everything else

```bash
ai-memory purge-project --project experimental --confirm
# Sibling projects (ai-memory, distrobox-gaming, ...) are untouched.
```

### Rename a project after moving its directory

```bash
ai-memory rename-project --from old --to new
```

Sessions started later in the new directory are resolved by the directory's
basename, so the rename is what lets them land in the same project. The project
id stays the same, so existing observations stay with the project. Confirm the
basename rule for your setup (**a confirmar**), or pin the project with a
`.ai-memory.toml` marker.

### Reattach a session captured under the wrong project

```bash
ai-memory move-session <session-id> --to my-project          # dry run
ai-memory move-session <session-id> --to my-project --confirm

# Or empty a stray project into the right one, then drop the empty husk:
ai-memory move-session --from-project tmp --to my-project --confirm
ai-memory purge-project --project tmp --confirm
```

The batch form drains every scope of the stray project, as described above.
Check the dry run's `source_scopes` before confirming.

## Why this matters: the flat-wiki history

Before the per-project directory layout, the wiki was flat: `wiki/<page-path>`,
with no project level. Two projects with the same page path shared one file on
disk. Purging one project then deleted files that belonged to its sibling. The
sibling's database rows survived, because the foreign keys are scoped by project
id. But every web view of the sibling's page returned 404, because its file was
gone.

Per-project directories keyed by id make that collision impossible by layout.
The disk path is derived from the project id, never from its name. Because of
that, `rename-project` only touches one column, and no file moves.
