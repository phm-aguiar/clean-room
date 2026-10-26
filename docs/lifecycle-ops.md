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
- After the rewrite it tries to checkpoint and truncate the write-ahead log;
  a reader holding it back (or a failed checkpoint) only logs a warning, never
  fails the committed operation.

On-demand form, deleting nothing: `ai-memory compact --confirm` (thin client of
`POST /admin/compact`, which itself requires `confirm: true`). Without
`--confirm` the CLI refuses locally (exit 1) with:
`compact rewrites the whole database under an exclusive lock: every write blocks until it finishes, and it needs free disk space of roughly the database's own size.`
followed by a blank line, `Check \`ai-memory status\` for the reclaimable figure first, then:`, a blank
line and `  ai-memory compact --confirm`. On success it prints
`Compacted: <before> → <after> (<reclaimed> reclaimed).` with binary units
(`<n> B` below 1024, otherwise one decimal and `KiB`/`MiB`/`GiB`/`TiB`), and,
when nothing was reclaimed, also
`Nothing to reclaim — the database had no meaningful free-page backlog. This is the normal result for a store that has not just had a large deletion.`
Sizes are the database's own page count × page size, not the file size on
disk (the write-ahead log is ignored).

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
  whole workspace (one row with an all-zero project id; no per-project rows). A later re-index skips any directory whose scope is
  tombstoned and leaves its files in place, untouched. Re-index never deletes
  files.

### Scope containment

In the CLI, `purge-session` checks that the session belongs to the resolved `(workspace, project)` before purging. On the server (`POST /admin/purge-session`), the request names `workspace` and `project` explicitly. The session must belong to that scope (checked inside the delete transaction), otherwise 404: the session id alone never reaches another scope. Every delete (pages, observations, handoffs, auto-improvement rows) is filtered by that `workspace_id` and `project_id`, so another project's page at the same relative path is never touched. The only cross-scope effects are the cascades of the session row (collateral observations, handoff links) and the deletion of the session's prompt profile candidates.

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
2. **Scope lookup.** The workspace is looked up by name and the project by `project` name or `project_id` UUID (mutually exclusive; passing both returns **400** `project and project_id are mutually exclusive`, neither returns **400** `either project or project_id is required`, an unparsable id returns **400** `invalid project id: <detail>`). Neither is created. Either missing returns 404 (`project not found in workspace` for the project). The same resolution, with the same codes, serves `rename-project` (`from` plays the role of `project`) and `move-project`.
3. **Lease check (applies to both dry-run and real run).** A managed run blocks the purge while it holds a
   **live** lease (`state = 'active'` and `lease_expires_at > now`). If a live lease exists and `force` is not set, the call returns
   409 **even when `dry_run: true`**. The body is just `{"error": "project has <n> active managed run(s) in workstream(s) '<nome>' (<agente>), …; finish or cancel them first, or pass force to purge anyway (the running agent will stop being able to save its history)"}`: there is **no** `active_workstreams` field; the names are sorted, deduplicated, and each one is `'<workstream>' (<agent>)`. Liveness is `state = 'active'` **and** `lease_expires_at > now` (a crashed wrapper leaves `active` rows behind that nothing flips until the next prepare, so `state` alone would block purges forever). With `force`, the purge or preview proceeds and the lease rows are deleted by the cascade; the still-running agent then gets 404/409 on its heartbeat and its transcript never reaches the ledger.
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
8. **File cleanup.** The project's wiki directory is removed under the wiki's
   exclusive lock, after a check that the tree holds no symbolic link or reparse
   point (a failed check puts the path in `files_failed`). An already-missing
   wiki directory counts as removed and **is** listed in `files_deleted`. Then each
   raw workstream segment directory (`raw/workstreams/<id>/`) is removed **outside**
   the wiki lock; a missing one is silently skipped (listed nowhere); any other
   error goes to `files_failed`.

Audit notes on ordering (verified against the original):

- The real run checks `confirm` **before** resolving the scope; the dry run
  resolves the scope and goes straight to the preview (so with `dry_run: true`
  a missing scope is still 404).
- On the real run, the live-lease check (step 3) runs **inside the database
  transaction of step 7**, that is, *after* admission (step 5) and the pre-purge
  checkpoint (step 6). A 409 on the real run can therefore come after the
  admission webhooks were called and a pre-purge commit was made; nothing is
  deleted. On the dry run, the same check runs in the preview transaction.
- The preview runs the same counting queries inside a transaction that is rolled
  back; it never issues the delete.
- The delete is a single deletion of the project row inside the workspace; every
  dependent table leaves by cascade. Counts are taken just before, in the same
  transaction: `pages_deleted` counts **all versions**; `embeddings_deleted` counts
  embedding rows of those pages; `managed_runs_deleted` counts runs of the
  project's workstreams. The tombstone is an insert-or-replace (a repeated purge
  refreshes `purged_at`). The audit row has no page and no detail.
- Compaction (`compact: true`) runs right after the commit, inside the database
  writer, **not** under the wiki lock. If it fails, the call returns 500 although
  the delete is already committed, and the file cleanup, webhooks and pointer
  clearing of steps 8 and 9 do **not** run; a later reindex still skips the
  directory because of the tombstone.
- The label in the report and checkpoints is `<workspace as sent>/<resolved project name>`.
- Without `confirm`: 400 `destructive operation requires confirm=true`.
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

Request body: `{workspace, project, session_id, confirm, compact, dry_run}`
(`workspace` and `project` are **required** names; the last three default to
`false`). Verified against the original:

1. **Dry run first.** With `dry_run: true` the server goes to the preview
   (step 3) whatever `confirm` says.
2. **Validation and lookup (real run).** Without `confirm`: 400
   `destructive operation requires confirm=true` (checked **before** the id).
   `session_id` is trimmed and must be a full UUID, otherwise 400
   `session_id must be a full UUID`. The workspace and project are looked up by
   name without creating them (missing: 404). The session id alone never grants
   access to another scope: the session must belong to the named
   workspace/project, checked inside the delete transaction (step 6). Otherwise
   404 `session <id> not found in this workspace/project` (also for a session
   that was already purged).
3. **Dry run.** Same id validation and scope lookup, then the store runs the same
   selection queries as the real purge inside a transaction that is rolled back,
   and returns the report with `dry_run: true`, `files_deleted: []`,
   `files_failed: []`, `compacted: false` and no checkpoints. No admission, no
   wiki lock, no file removal, no tombstone, no audit row.
4. **Admission.** The `purge_session` admission webhooks run before any
   deletion (with the authenticated actor). A failure returns 500 with nothing deleted.
   Admission runs **before** the session/scope check, so a 404 for a session of
   another scope can come after the webhooks were called.
5. **Checkpoint.** A pre-purge checkpoint `pre-purge-session <workspace>/<project>: <session_id>`
   is committed if the tree is dirty. Failure returns 500 with nothing deleted.
6. **Exclusive lock and database delete.** The project tree is checked for
   symbolic links/reparse points before and after taking the wiki's exclusive
   mutation lock. Then one transaction:
   - checks that the session belongs to the scope (404 otherwise, see step 2);
   - selects the pages to delete: the session's recorded summary page **plus**
     every version at `sessions/<session_id>.md` whose frontmatter `session_id`
     equals this session. A manual edit at that path without that key is **not**
     deleted;
   - selects the files to remove: each distinct path of those pages, **unless**
     a current version at that path is not one of the deleted pages (a later
     manual rewrite survives, and so does its file);
   - counts the collateral rows: observations of this session stamped into
     another project, and handoffs of other projects authored or accepted by it;
   - deletes, innermost first: the rejections of each of the session's
     auto-improvement runs; the session's auto-improvement runs
     (`auto_improve_runs_deleted`); the handoffs it **authored** in this scope;
     the selected pages (their embeddings, links, entity links and evidence go
     by cascade); the session's observations in this scope; the profile
     candidates with source `prompt` and reference `session:<id>` (in **any**
     project); then the tombstone in `purged_sessions` (insert, or refresh
     `purged_at` if it already exists); then the session row, whose cascade
     removes its consolidation jobs, scheduler claims and the ingest keys tagged
     with this session (keys without a session tag stay; schema-verified) and, through it, the
     collateral observations of other projects; handoffs that only reference it
     elsewhere lose the link (set to null);
   - writes the audit row `purge_session` (no page, no detail) and commits.
   Compaction, when requested, runs right after the commit inside the writer,
   while the wiki lock is still held. A compaction failure returns 500 although
   the purge is committed, and no file is removed.
7. **File cleanup.** Still under the lock, each selected path (relative to the
   scope's own root) is removed. A missing file is silently skipped (listed
   nowhere). Any other error goes to `files_failed`. A failed removal does **not**
   roll back the database purge: the call still returns 200, and the async
   webhooks receive `partial_failure: true`.
8. **After.** The lock is released, the purge webhooks are dispatched, and a
   post-purge checkpoint `purge-session <workspace>/<project>: <session_id>` is
   committed if the tree changed (a failure there only logs a warning,
   `checkpoint: null`). Unlike the project purge, the active-project pointer
   and the scope cache are not touched.

`pages_deleted` counts page **rows** (versions), and `removed_paths` lists the
paths selected for file removal. `workspace` and `project` in the report echo
the request.

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

1. **Lookup.** Missing workspace or source project (`from` or `project_id`, mutually exclusive: both or neither is 400, see `purge-project`) returns 404.
2. **Validate names** (inside the same database transaction as the update, in
   this order; each failure is **422** with `{"error": "invalid project name: <reason>"}`):
   target empty or whitespace-only (`project name must not be empty or all whitespace`);
   target contains `/` (`project name must not contain '/' (it appears in URL paths)`);
   target is a reserved profile name, `_profile` or `_profile.*`
   (`<name> is reserved for the cross-project profile`); source is a profile project
   (`a cross-project profile project cannot be renamed`). Leading and trailing
   whitespace on `to` are trimmed silently before storing. Verified against the
   original: the rename path applies **no** length limit and **no** control-character
   check of its own.
3. **Update.** The name column is updated and an audit row (`rename_project`,
   detail `{from, to}`) is written in the same transaction. If `to` is already used
   by another project in the same workspace (uniqueness constraint), the call returns
   **422** (not 409) with `{"error": "project name '<to>' is already taken in this workspace"}`.
   Renaming to the current name is not special-cased: it succeeds (200), still writes
   the audit row and still refreshes the manifests. If the row vanished between lookup
   and update (concurrent purge), the call returns 404
   (`project id <id> no longer exists in workspace <id> (race with concurrent purge or delete)`).
   Any other database error is 500.
4. **Count.** The number of latest page versions is counted for the report.
5. **Manifests, cache and checkpoint.** The scope cache is invalidated, the workspace and project `_meta.md` manifests
   are rewritten, and a checkpoint (`rename-project <workspace_id>/<project_id>`) is committed if the tree changed.

Zero page files move, because disk paths are keyed by project id.

If step 5 fails after the rename committed, the call still returns 200. The
response carries `manifest_warning` and no checkpoint. The database rename
stands.

Response: `workspace`, `from`, `to`, `pages`, `checkpoint`, `manifest_warning`.
`from` is the name the server resolved (useful when the call used `project_id`);
`to` echoes the request value **as sent** (untrimmed). `pages` counts the latest
versions of the project, expired ones included.

CLI (`rename-project`, thin client): `--from` and `--project-id` are mutually
exclusive; without either, the source is the project resolved for the current
directory (marker or project strategy). After the call it prints any
`manifest_warning` to stderr as `ai-memory: warning: <text>`, re-keys the
client-local checkout link from the old name to the new one (a failure there only
prints `ai-memory: project renamed, but the client-local checkout link could not be updated (<error>)`
and does not fail the command), and finally prints
`Renamed <ws>/<old> → <ws>/<new> (<n> pages now under the new name).`
A non-2xx answer fails the command (exit 1).

### `/admin/rename-workspace`

HTTP admin route. It changes `workspaces.name` and leaves every on-disk path
unchanged, because paths are keyed by workspace id.

1. Missing source workspace returns 404.
2. An empty/whitespace-only target (`invalid workspace name: workspace name must not be empty or all whitespace`)
   or one containing `/` (`invalid workspace name: workspace name must not contain '/'`) returns 422.
   A target name already taken by another workspace also returns **422** (not 409),
   `workspace name '<to>' is already taken`. The target is trimmed before storing.
   Renaming to the current name is not special-cased (200). A row that vanished
   concurrently is 404; other database errors are 500.
3. Otherwise the name is updated. Then the scope manifests are refreshed (the
   refresh pass, verified in the code, holds the wiki's exclusive mutation lock
   and rewrites the manifest of **every** workspace and **every** project of the
   store, not only this workspace. `manifests_refreshed` counts only the manifests
   whose content actually changed. Without a store attached to the wiki, it is 0).
4. If the manifest refresh fails after the rename committed, the response is
   still 200. It has `manifests_refreshed: 0` and a `manifest_warning`. The
   database rename stands.
5. The scope cache for the workspace is invalidated, and a checkpoint (`rename-workspace <from> -> <to>`) is
   committed if the tree changed.

### `/admin/delete-workspace`

HTTP admin route. There is no CLI subcommand for it. Request fields (verified
against the original): `workspace`, `force` (default `false`), `compact`
(default `false`) and `dry_run` (default `false`). **There is no `confirm`
field**: a request without `dry_run` deletes. An extra `confirm` field is ignored.
The protection is the non-empty guard (step 3) plus the route being root-only.

1. **Dry run first.** With `dry_run: true` the preview runs, whatever `force` says
   (step 4).
2. **Lookup.** Missing workspace returns 404. The lookup never creates one.
3. **Non-empty check (dry run and real run).** Without `force: true`, a
   workspace that still has projects returns **409**
   `workspace still has <n> project(s); pass force to delete anyway`. There is no
   project list or count field. The dry-run message adds
   ` (pass "force": true alongside "dry_run": true to preview a non-empty workspace)`.
   On the real run the count comes from the operator's project listing, and the
   store re-checks inside the transaction (also 409).
4. **Dry run.** The same counting queries run inside a transaction that is rolled
   back. The response has `dry_run: true`, `files_deleted: []`, `files_failed: []`
   and `compacted: false`, with no checkpoints. No admission and no tree check.
5. **Tree check.** If the workspace's wiki tree holds a symbolic link or reparse
   point, the call returns 500 and nothing changes.
6. **Admission.** The `purge_workspace` admission webhooks run (with an empty
   project name). A failure returns 500 and nothing changes.
7. **Checkpoint.** A pre-delete checkpoint `pre-delete-workspace <ws>` is
   committed if the tree is dirty. Failure returns 500 and nothing changes.
8. **Database delete.** One transaction counts the rows (the counts include
   collateral observations of other workspaces deleted through the sessions'
   cascade, and handoffs of other workspaces that lose a session link), then
   deletes the workspace row. Everything under it leaves by cascade. The whole
   workspace is tombstoned in `purged_scopes` by **one** row whose project id is
   all zeros (refreshed if it already exists). There are **no** per-project
   tombstone rows. There is **no** managed-run lease check (a live agent loses its
   lease) and **no** audit row. With `compact`, compaction runs after the commit;
   if it fails, the call returns 500 with the delete already committed and no file
   cleanup.
9. **File cleanup.** The workspace's wiki directory is removed under the wiki's
   exclusive lock. A missing directory counts as removed and is listed. Then each
   raw workstream directory is removed outside the lock (a missing one is skipped,
   other errors go to `files_failed`).
10. **After.** Purge webhooks are dispatched, with `partial_failure` when a removal
    failed. Active-project pointers for the workspace are cleared, the scope cache is
    invalidated, and a post-delete checkpoint `delete-workspace <ws>` is committed if
    the tree changed (failure: warning only).

Response fields: `workspace`, `projects_deleted`, `pages_deleted` (all versions),
`sessions_deleted`, `observations_deleted`, `handoffs_deleted`,
`embeddings_deleted`, `collateral_observations_deleted`,
`collateral_handoffs_denulled`, `workstreams_deleted`,
`managed_runs_deleted`, `workstream_ids`, `files_deleted`, `files_failed`,
`compacted`, `pre_checkpoint` and `checkpoint` (omitted when absent), `dry_run`
(present only when true).

### `/admin/merge-workspace`

HTTP admin route, verified against the original. Body:
`{from, to, confirm, force (false), on_conflict (block)}`.

1. Without `confirm`: 400 `destructive operation requires confirm=true`.
2. `from == to` (names compared as sent): 422 `from and to are identical; nothing to merge`.
3. Source workspace missing: 404 (never created). The destination is created by
   the per-project moves when needed.
4. The source's projects are listed (most recently updated first) and each one
   goes through the same path as `move-project` (`confirm: true`, the request's
   `force` and `on_conflict`, selected by name). Private profile projects are
   **not** filtered out: one of them fails its move with 422 and stops the merge.
5. At the first failing project, the merge stops with that project's status and
   `{error: "merge stopped at project '<p>' in workspace '<from>'", failed_project, reason, projects_merged, source_workspace_deleted: false}`.
   Moves already done stand (each is idempotent on re-run).
6. When all moves succeed, the emptied source is deleted with the
   `delete-workspace` path, `force: false` and no compaction. If that fails (for
   example, a project was recreated meanwhile), the call still returns 200 with
   `source_workspace_deleted: false`.

Response: `{from, to, projects_merged: [<move-project report>...], source_workspace_deleted}`.

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
2. Both project trees (source, and the would-be destination directory) are
   checked for symbolic links/reparse points (failure: 500). The destination project directory
   `<wiki>/<to_ws_id>/<proj_id>` must not exist on disk. If it does, the call returns 409
   `destination dir already exists: <path>; refusing true-move` and nothing moves.
3. Under the wiki's exclusive mutation lock (trees re-checked, destination re-checked:
   409 if it appeared), the `move_project` admission webhooks run with the source and
   destination names. A rejection returns 500 `move aborted (nothing changed): <detail>`.
4. **Disk first:** the project directory is renamed into the destination workspace's
   tree (skipped when the project has no directory yet).
5. **Then one database transaction** re-stamps the workspace id, matching by project id,
   on: pages (all versions), sessions, observations, handoffs, project grants; and,
   where the row still carries the source workspace: audit rows, auto-improvement runs,
   proposals, rejections, scheduler state and claims, consolidation jobs, managed
   workstreams, entities, page feedback, purged-session tombstones, profile candidates,
   profile entry ledger and harvest marks; messages addressed to or sent by the project
   (each end separately); finally the project row (if it is no longer in the source
   workspace: 404). Link rows are **not** re-stamped (their targets are by name and
   page id). The project id is unchanged. If the transaction fails, the directory is
   renamed back; if that also fails: 500
   `INCONSISTENT STATE: files moved but DB re-stamp failed (...) and dir rename-back also failed (...); manually move <dst> -> <src> or finish the re-stamp`.
   On success the move webhooks are dispatched.
6. The active-project pointer is retargeted to the new workspace. The scope
   cache is invalidated. Manifests are refreshed, and a post-move checkpoint (`move-project <from_ws>/<proj> -> <to_ws>/<proj>`) is
   committed if the tree changed.

Report: `moved_via: "true-move"`, `merged_into_existing: false`, `pages_copied` (page rows re-stamped, **all versions**), `from`/`to` (`<ws>/<proj>` labels),
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
7. **Copy failure** (any page): the call returns 500
   `copy of <path> failed: <detail>` before the source is purged. The source is intact,
   but pages copied **before** the failure stay in the destination (no rollback; a re-run
   supersedes them idempotently). Pages are copied one by one through the normal write
   path, without re-embedding and skipping only the contributors enrichment webhook.
   The source's `pinned` flag, tier and stored title are kept.
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

Audit notes (verified against the original):

- Server check order: `confirm` (400 `destructive operation requires confirm=true`);
  identical workspace **names**, compared as sent (422
  `from_workspace and to_workspace are identical; use rename-project instead`);
  requested project name starting with `_profile.` (422
  `a private cross-project profile stays in the default workspace and cannot be moved`,
  even if it does not exist); source lookup (400/404, as in `purge-project`); the same
  profile check on the resolved name (so `project_id` is refused too); a workspace
  profile `_profile` **may** move; active-project guard (409
  `<ws>/<proj> is the active session's project; a live move risks stale-cache writes. Re-run with force=true to proceed.`);
  merge detection; pre-checkpoint.
- Merge: the destination is resolved with get-or-create. The `block` check only covers
  pages whose source file could be read. Under `duplicate`, the suffix search
  (`-from-<slug>`, then `-from-<slug>-1`, `-2`, ...) has **no upper bound**. It skips paths
  already used in this run and paths with a page in the destination. A source page whose
  natural path was already taken by an earlier de-duplicated page in the same run is
  also de-duplicated and listed in `conflicts`.
- Merge source purge: no `force`, no compaction, audit author empty. A live managed
  run returns 409 `<n> page(s) were copied to <ws>/<proj>, but the source <label> could not be purged: ...`.

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

Verified against the original (audit 011). Two shapes share one endpoint
(`POST /admin/move-session`). Request body:
`{session_id?, from_workspace?, from_project?, workspace?, project, pages, confirm, force, create}`.

- **Single:** `session_id` (UUID). The source is the scope where the session row
  sits now. A missing session: 404 `session <id> not found`.
- **Batch:** `from_project` plus `from_workspace` (default `default`). The source
  scope must exist (404 otherwise; never created). The batch takes **every session
  touching the source**: a session row there, or at least one observation stamped
  into it. Oldest first (row `started_at` or first observation in the source).
  The CLI resolves `--from-project` like any other scope (marker, else literal)
  and sends `from_project`, not a list of ids.
- Exactly one of `session_id` and `from_project` must be given, otherwise 400
  `provide exactly one of session_id (single move) or from_project (batch)`.
- Destination: `project` (required) in `workspace` (default: the source
  workspace). Missing destination: 404, unless `create: true`. With
  `create` and `confirm` the destination is created first. With `create` and no
  `confirm`, nothing is created and the report carries
  `would_create_project: true` (counts computed read-only).
- `pages`: `move` (default) or `regenerate`. There is no `keep` mode.

**Dry run by default.** Without `confirm`, the store runs the **whole move
inside a transaction that is rolled back** (including the audit row), and the
report has `dry_run: true`. No admission, no file is touched, no checkpoint.

**Guards, per session, in this order (all bypassed by `force: true`):**

1. Session still open (no end recorded) and not a re-home: 409
   `session <id> is still open (no session end recorded); finish it or force the move (--force / force: true)`.
2. Consolidation job `pending` or `running`: 409
   `session <id> has a pending or running consolidation job; wait for it or force the move (--force / force: true)`.

Batch-only guards (before any session): source equal to the destination: 422
`source and destination scopes are identical`; source project is the hook
router's active project and no `force`: 409
`<ws>/<proj> is the active session's project; force the move (--force / force: true) to move its sessions anyway`.

Then both project trees are checked for symbolic links and reparse points.

**Re-home.** When the session row already sits in the destination, the row
does not move (`session_moved: false`). Open sessions are allowed, and only stray
rows in other scopes are gathered into the destination.

**What moves, per session, in one transaction:**

1. The session row (`workspace_id`, `project_id`), unless it is a re-home.
2. `source_scopes`: before the re-stamp, every scope (other than the
   destination) holding observations of the session, with names and counts,
   largest first.
3. Re-stamped into the destination, wherever they are, when not already there:
   observations, handoffs it **authored**, consolidation jobs, auto-improvement
   runs and scheduler claims (each counted).
4. Prompt profile candidates (`session:<id>`) and the session's ingest keys:
   re-stamped where possible, and colliding leftovers outside the destination
   deleted.
5. Session page `sessions/<id>.md`. The rows considered are those of the source
   scope (in a re-home: of every scope other than the destination).
   - `move`: if the destination already has a current version at that path, or
     a re-home finds current versions in more than one outside scope: 409
     `<path taken message>; re-run with pages=regenerate or resolve the destination page first`.
     Otherwise every considered version is re-stamped (`page_versions_moved`).
     Links and evidence follow the page ids.
   - `regenerate`: the open entity links of the current version are closed and
     the current version is retired (`is_latest = 0`, `valid_to` = one instant,
     no tombstone mark; `pages_regenerated`). The session's summary pointer to
     those rows is cleared, so the next consolidation writes a fresh page in
     the destination.
6. Audit row `move_session`, attributed to the destination scope.

The `purged_sessions` tombstone is **not** touched by a move.

**Files (confirmed run only), under the wiki's exclusive lock:**

- Admission (`move_session`, with source and destination names) runs **inside**
  the lock, before any file or row moves. A rejection aborts with nothing changed.
- If source and destination differ and the source file exists:
  - `move`: if a file already exists at the destination path: 409 (checked also
    before the dry run, so the dry run predicts it). Otherwise the file is
    renamed into the destination's `sessions/`.
  - `regenerate`: the file is renamed to a temporary name in the same directory,
    ignored by the watcher (`.ai-memory-tmp.move-session.<id>.md`).
- Then the database transaction runs. If it fails, the file is renamed back. If
  that rename also fails: 500
  `INCONSISTENT STATE: session page file moved but DB re-stamp failed (...) and moving it back also failed (...); manually move <a> -> <b>`.
- On success, `regenerate` deletes the parked file (a failure only logs a
  warning), and the move webhooks are dispatched.

**After (confirmed).** Single form: pre-checkpoint
`pre-move-session <id> <from_ws>/<from_proj> -> <to_ws>/<to_proj>` before (failure:
500, nothing moved). Then, after the move, the manifests of every scope are rewritten
(failure: warning only) and the post checkpoint `move-session <id> ...` is taken.
Batch form: one pair for the whole batch (`pre-move-session <from> -> <to>` /
`move-session <from> -> <to>`), taken only when there is at least one session
(the post checkpoint only when at least one session moved). The scope cache and
the active-project pointer are not touched.

**What does not move:** `sessions.cwd`; other pages written during the session;
handoffs the session only *accepted*; entities and page feedback. When the cwd's
last component differs from the destination project name, the report carries
`cwd_warning`:
`session cwd '<cwd>' resolves to project '<base>' by basename, not '<to>'; the cwd was left as recorded. New sessions started there still land in '<base>' unless a .ai-memory.toml marker pins project = "<to>"; with [routing] mid_session = "sticky" the remaining events of this session follow it.`

**Batch failure.** Each session moves in its own transaction. The batch stops at
the first error. The response keeps that error's status and `error` and adds
`session_id`, `dry_run`, `total`, `moved`, `sessions` (reports of those already
moved, which stay moved), and the post `checkpoint` and `manifest_warning` if any.

Response (single): `session_id`, `dry_run`, `session_moved`,
`would_create_project` (only when true), `source_scopes` (omitted when empty),
`from` and `to` (`{workspace, project}`; `to` echoes the names as requested),
`summary` (`observations`, `handoffs`, `consolidation_jobs`, `auto_improve_runs`,
`auto_improve_claims`, `page_versions_moved`, `pages_regenerated`), `page`
(`moved`, `regenerated`, `already in destination` or `none`), `cwd`,
`cwd_warning`, `pre_checkpoint`, `checkpoint`, `manifest_warning` (optional fields
omitted when absent).

Response (batch): `{dry_run, would_create_project?, from, to, total, moved, sessions: [...], pre_checkpoint?, checkpoint?, manifest_warning?}`.

CLI output: the per-session counts block (`observations:`, `handoffs:`,
`consolidation jobs:`, `auto-improve runs:`, `auto-improve claims:`,
`page: <label> (<n> version(s) moved, <m> retired)`), the session row line
(`moved`, `would move` or `already in destination (re-home: only stray rows gathered)`),
the scopes being drained, a batch table, `Pre-move checkpoint: <oid>` /
`Checkpoint: <oid>`, and, for a dry run,
`(dry run; nothing was written)` / `Re-run with --confirm to apply:` followed by
the exact command. A `manifest_warning` goes to stderr as
`ai-memory: warning: <text>`. A non-2xx answer fails the command (exit 1).

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

Request body: `{workspace, project, path, rev}`. Verified against the original:

1. **Path check.** An invalid page path returns 422 `invalid path: <detail>`. It is
   checked before the scope lookup.
2. **Scope lookup.** Missing workspace or project returns 404. Nothing is created.
3. **Pre-restore checkpoint.** If the tree has uncommitted changes, a checkpoint
   `pre-restore-page <ws>/<proj>: <path>` is committed first (failure: 500, nothing
   changed).
4. **Read and validate.** The page path is checked against the project root
   (no escaping). The file is read from git at `<rev>:<ws_id>/<proj_id>/<path>`,
   decoded as UTF-8 (otherwise `<path> at <rev> is not valid UTF-8 markdown: ...`)
   and parsed. Title, links, tier, pin, TTL and entities are derived from that
   historical file. The live file is not touched yet.
5. **Write.** This is **not** the standard page-write pipeline: no admission
   webhooks, no secret sanitizer, no author. Under the wiki's **shared** (not
   exclusive) mutation lock, the scope rows and manifests are ensured, the
   historical bytes are written **verbatim** to the live file (atomic replace), and
   the page is upserted in the index with the normal supersession rule (identical
   content creates no version). The restored version is then embedded, like a
   normal write, outside the lock.
6. **Post-restore checkpoint.** `restore-page <ws>/<proj>: <path> from <rev>` is
   committed if the tree changed (failure: warning only, `checkpoint: null`).

Response: `{page_id, path, restored_from, pre_checkpoint, checkpoint}`.

CLI: the scope comes from `--workspace`/`--project` or the marker. It prints
`restored <path> under <ws>/<proj> from <rev> (page_id=<first 8 chars>)`, then
`pre-restore checkpoint: <oid>` and `restore checkpoint: <oid>` when present.
`--json` prints the response instead. An error is prefixed with
`restoring page via server` (exit 1).

Failure modes:

- **Invalid page path**: 422, nothing changed.
- **Workspace or project not found**: 404, nothing changed.
- **Checkpoint or file not found** in git, non-UTF-8 content, malformed
  historical markdown, or a write/index/embedding error: 500. Any pre-restore
  checkpoint stays as an audit breadcrumb. An embedding failure comes **after**
  the file and index were already written.
Not recovered: sessions, observations, handoffs, users, audit rows, access
counters and embeddings. These live only in the database. Roll them back with a
full `backup` and `restore`.

### `backup`

```bash
ai-memory backup --to /tmp/ai-memory-backup.tar.gz
```

Flag: `--to` (short form `-o`).

What the server does:

1. Validates the wiki tree, refusing symbolic links and reparse points. An
   invalid tree returns 500 (`validating wiki tree: ...`).
2. Takes a consistent snapshot of the live database into a temporary file,
   using the database engine's **online backup** facility on a read connection
   (not `VACUUM INTO`). The writer is not stopped. Failure: `sqlite snapshot: ...`.
3. Validates the wiki tree **again** right before archiving
   (`validating wiki tree before archive: ...`). Then it writes, into a temporary
   file, a gzip tarball with deterministic headers that does not follow symbolic
   links. Contents, in order: the whole `wiki/` directory, including its git
   repository (skipped if `wiki/` does not exist); the snapshot as
   `db/memory.sqlite` (it may be stored as a sparse entry, which `restore`
   accepts); and `config.toml` if it is a file. Errors: `archiving wiki/: ...`,
   `archiving db snapshot: ...`, `archiving config.toml: ...`.
4. Streams the file as the response body (never buffered in memory) with
   content type `application/gzip` and
   `Content-Disposition: attachment; filename="backup.tar.gz"`. There is no
   request body.

A failure returns 500 with a JSON `{error}` body. The CLI creates the parent
directory of `--to` if needed, streams the body to that path and prints
`✓ wrote backup to <path> (<size>)`, with binary units and two decimals above
1 KiB. An error is prefixed with `requesting backup from server` (exit 1).
This is the standard snapshot before a risky operation. The backup holds no
`raw/`, `logs/` or `models/`.

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
2. **Source check.** The tarball must exist as a regular file, otherwise
   `source tarball <path> not found`.
3. **Occupied check.** The data directory counts as occupied if `wiki/` is a
   directory with at least one entry, or `db/memory.sqlite` is a file. If it is
   occupied and `--force` is not set, the command refuses:
   `refusing to restore: data dir at <path> is non-empty (pass --force to overwrite)`.
   The data directory is created if it does not exist.
4. **Stage.** A scratch directory `.restore-staging-<YYYYMMDD-HHMMSS>-<pid>`
   (UTC timestamp) is created inside the data directory, and the gzip tarball is
   extracted into it. Every entry is validated before it is written (permissions
   are not preserved):
   - every path component must be a plain name. An absolute path, `..`, a leading
     `./`, or a drive/root prefix is rejected with `backup contains unsafe path: <p>`;
   - symbolic and hard links: `backup contains unsupported link entry: <p>`;
   - anything that is not a regular file (including GNU sparse entries, which hold
     hole-encoded regular files) or a directory: `backup contains unsupported entry type: <p>`;
   - allowed directories: `wiki`, `wiki/...`, `db`. Allowed files: `config.toml`,
     `db/memory.sqlite`, `wiki/...`. Anything else:
     `backup contains unexpected path: <p>`.
   A stream that is not gzip, or is truncated, fails as
   `extracting <tarball> into <staging>: ...`. Verified against the original: the
   archive is **not** required to contain `db/memory.sqlite`. Without it, step 5
   creates a new, empty, migrated database and the restore proceeds.
5. **Validate the database.** The staged store is opened (applying pending
   migrations) and then fully closed. The writer thread is joined and every
   connection is released before any rename, which Windows needs (an open
   database or log handle blocks the directory rename). A failure stops here with
   `opening restored store: ...`, and the live data is untouched.
6. **Swap.** A directory `.restore-previous-<same stamp>` is created. The live
   `wiki/` and `db/` (those that exist) are renamed into it. The live
   `config.toml` is moved aside **only** if the archive carries one. Then the
   staged `wiki/`, `db/` and `config.toml` (those that exist) are renamed into
   place. All moves are same-filesystem renames. If any step fails, the placed
   entries are removed and the parked ones renamed back, and the command fails with
   `restore swap failed; the previous data was moved back into place: ...`. If the
   rollback also fails:
   `INCONSISTENT STATE: restore swap failed (...) and moving the previous data back also failed (...); the pre-restore wiki/ and db/ are under <previous> — move them back into <data dir> by hand`.
7. **Cleanup.** After a successful swap, `.restore-previous-*` is removed (failure:
   stderr
   `warning: restore succeeded but the pre-restore data under <p> could not be removed (...); delete it by hand`,
   exit 0). The staging directory is removed in **every** outcome (failure: stderr
   `warning: could not remove staging dir <p> (...); delete it by hand`). On success the
   command prints `restored <tarball> -> <data dir>`.

Windows notes: every rename fails if a process holds a file open in `wiki/`
or `db/` (an editor, an antivirus scan, OneDrive sync, or a running server whose
process guard was not detected, see `reset`). The rollback then restores the
previous state. Because the scratch directories live inside the data directory,
no move crosses volumes.
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

Audit details (verified against the original):

- Order is fixed: `wiki/`, then `db/`, then `raw/`. Each existing directory is
  removed recursively and recreated empty; the failure messages are
  `removing <path>` or `recreating <path>` (with the underlying OS error). The
  dry run lists only the directories that exist, one `would remove <path>` line
  each. A missing data directory is a no-op (nothing is created).
- The same wipe (same three directories, same order) backs
  `uninstall --purge-data`.
- The process guard refusal message is
  `refusing to <verb>: <n> other ai-memory process(es) running (pids: [<pid>, ...]). Stop them first, then re-run.`
  (`<verb>` is `reset`, `reindex`, `restore`...). Exit is the generic failure (1).
- The guard looks for processes whose name is **exactly** `ai-memory`, excluding
  itself (and, on Linux, excluding thread entries). On Windows the image name is
  `ai-memory.exe`, so the exact match on `ai-memory` misses live siblings and
  the guard does not refuse. Original behavior, reproduce as is: the comparison
  is the exact name `ai-memory` (no `.exe` variant, no case folding). With a live server holding `db/` open, Windows also
  refuses the recursive removal ("file in use"), which surfaces as the
  `removing <path>` error after earlier directories may already be gone.

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
3. **Walk the tree.** First validates the wiki tree, refusing symbolic links and reparse points. Then each `<workspace_id>/<project_id>/` directory is visited. Directories whose scope
   is tombstoned in `purged_scopes` are skipped (`skipped_purged`) and left in place.
4. **Workspaces and projects.** Each one is recreated from its `_meta.md`
   manifest, keeping the UUIDs encoded in the directory names. A missing
   manifest is an error that gives its exact path.
5. **Pages.** The latest page rows, page links and the full-text index are
   rebuilt from the markdown, one page at a time. Pages under `sessions/<id>.md` whose session id is in `purged_sessions` are skipped (`skipped_purged_sessions`). Embeddings are not computed. Run `embed` afterwards
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
id stays the same, so existing observations stay with the project. This holds
for the default `basename` project strategy (project = last component of the
working directory); with the `repo-root` strategy the name is the basename of
the repository root instead (see [install.md](install.md) and
[lifecycle-event-ingestion.md](lifecycle-event-ingestion.md)). A
`.ai-memory.toml` marker overrides both and pins the project explicitly.

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
