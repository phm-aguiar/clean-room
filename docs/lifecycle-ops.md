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
from search (the status-count part is **a confirmar**). Removal is a
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
  depends on database size (**a confirmar**: no timing was measured).

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

- A session purge records the session id. A later re-index does not restore
  that session's page file, even if the file cleanup failed.
- A project purge records the project scope. A workspace delete records the
  whole workspace. A later re-index skips any directory whose scope is
  tombstoned and leaves its files in place, untouched. Re-index never deletes
  files.

### Scope containment

The session id is never authority on its own. Every statement filters on the
workspace id and the project id as well as the session id. A session outside
the named scope is a 404 and nothing is deleted. Derived page versions are
deleted **by id**, not by path. Two projects can hold the same
`sessions/<uuid>.md` path, and deleting by path would take the other project's
page with it.

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

# Also reclaim the freed bytes (slow; rewrites the whole database file):
ai-memory purge-project --workspace default --project my-project --confirm --compact
```

Flags: `--workspace`, `--project`, `--confirm`, `--force`, `--compact`.

Without `--confirm` the CLI sends a dry run, prints the preview counts, and
exits with an error that tells you to re-run with `--confirm`. Nothing is
deleted.

What the server does, in order:

1. **Dry run.** If the request is a dry run, the server returns the counts
   and the paths a confirmed purge would remove. It runs no admission
   webhook, takes no checkpoint and deletes nothing. A dry run wins over
   `confirm`.
2. **Confirmation.** No `confirm` flag returns 400.
3. **Scope lookup.** The workspace and project are looked up by name and never
   created. Either missing returns 404.
4. **Admission.** The `purge_project` admission webhooks run before any
   deletion. A reject-policy webhook that fails aborts with 500 and nothing is
   deleted.
5. **Checkpoint.** If the wiki tree has uncommitted changes, a checkpoint is
   committed first. If that fails, the call returns 500 and nothing is
   deleted.
6. **Lease check.** A managed run blocks the purge only while it holds a
   **live** lease: its run state is active and its lease expiry is still in the
   future. A lapsed lease, typically from a crashed wrapper, does not block
   the purge. If a live lease exists and `force` is not set, the call returns
   409 and names the workstreams. With `force`, the purge proceeds.
7. **Database delete.** One transaction deletes the project row. Cascades
   remove every page version, session, observation, handoff, page embedding,
   workstream and managed run of the project. The workspace row stays. The
   scope is tombstoned and an audit row is written, attributed to the
   authenticated user when there is one. Compaction runs here if requested.
8. **File cleanup.** The project's wiki directory and each of its raw
   workstream segment directories are removed. Each path is reported in
   `files_deleted` or `files_failed`.
9. **After.** The active-project pointer is cleared if it pointed at this
   project. The scope cache is invalidated. Purge webhooks are dispatched, with
   `partial_failure` set if any file removal failed. A post-purge checkpoint is
   committed if the tree changed.

Report fields: `label`, `pages_deleted`, `sessions_deleted`,
`observations_deleted`, `handoffs_deleted`, `embeddings_deleted`,
`workstreams_deleted`, `managed_runs_deleted`, `workstream_ids`,
`collateral_observations_deleted`, `collateral_handoffs_denulled`,
`compacted`, `files_deleted`, `files_failed`, `pre_checkpoint`, `checkpoint`.

Collateral counts are about other projects. `collateral_observations_deleted`
counts observations stamped into another project by this project's sessions.
They are deleted with those sessions. `collateral_handoffs_denulled` counts
handoffs in other projects that were authored or accepted by this project's
sessions. They keep their text and lose the link.

Failure modes:

- **Workspace or project not found**: 404, nothing changed.
- **No `confirm`**: 400, nothing changed.
- **Live managed run and no `force`**: 409, nothing changed. Finish or cancel
  the session, or re-run with `force`. Forcing stops the running agent from
  saving its history.
- **Admission rejected, or the pre-purge checkpoint failed**: 500, nothing
  changed.
- **File removal partly failed** (for example a permissions problem): the
  database rows are already gone and the call returns 200 with `files_failed`
  filled in. Remove the listed paths by hand. A second call with the same
  arguments returns 404, because the project no longer exists.

The directory removal runs under the wiki's own exclusive lock, after the
database commit. The database delete and the directory removal are not one
critical section. Whether a concurrent write can slip into that gap is
**a confirmar**. The reasons the purge is safe with the server running, as the
design states them, are:

- The database delete is one transaction, and the single write queue
  serializes it against other writes.
- The directory removed belongs only to the project's own id-keyed directory.
  No other project shares files with it.
- Watcher events for files that were just removed have no database row to
  update (**a confirmar**).

### `purge-session`

```bash
ai-memory purge-session \
  --workspace default --project my-app \
  --session-id 0199f3d2-1c4e-7a10-9f3b-2b0c5d8e7a11 \
  --confirm
```

Flags: `--workspace`, `--project`, `--session-id` (a full UUID; a prefix or a
workstream id is rejected), `--confirm`, `--compact`.

Without `--confirm` the CLI sends a dry run, prints the preview, and exits with
an error. Nothing is deleted.

What the server does, in order:

1. **Dry run** (`dry_run: true` always wins over `confirm`). It returns exact
   counts from the same reads a confirmed purge uses, then rolls back. No
   admission, no checkpoint, no file removal, no audit row.
2. **Confirmation.** No `confirm` returns 400. A session id that is not a UUID
   returns 400.
3. **Scope lookup.** Missing workspace or project returns 404.
4. **Admission.** The `purge_session` admission webhooks run before any
   deletion. A reject-policy failure returns 500 with nothing deleted.
5. **Checkpoint.** A pre-purge checkpoint is committed if the tree is dirty.
   Failure returns 500 with nothing deleted.
6. **Scope check.** The session must belong to the named workspace **and**
   project. Otherwise the call returns 404 and nothing is deleted, even if the
   session id exists elsewhere.
7. **Collection.** The purge collects:
   - the session's summary page version;
   - every page version at `sessions/<session_id>.md` whose stored metadata
     names this session.

   Hand-written versions at the same path, which carry no session marker, are
   **kept**. If a live hand-written page exists at that path, the live file
   stays.
8. **Database delete.** One transaction deletes:
   - the session row, which cascades to its scheduler claims and consolidation
     jobs (they hold no content of their own);
   - the session's observations within the scope;
   - the handoffs it **authored** (`from_session_id`);
   - its auto-improve runs and their rejection records;
   - the page versions from step 7, with their embeddings.

   It also records the session id as tombstoned and writes an audit row.
   Compaction runs here if requested.
9. **File cleanup.** Only the page paths the database step returned, under the
   scope's own root, are removed. Removal is reported in `files_deleted` and
   `files_failed`. A failed removal does **not** roll back the database purge.
   The call still returns 200, and async webhooks receive `partial_failure: true`.
10. **After.** A post-purge checkpoint is committed if the tree changed.

**Kept on purpose:**

- Handoffs the session only *accepted*. Their text belongs to the session that
  wrote them. They lose only the pointer to the accepting session.
- Pages written during the session that are not its session page (decisions,
  gotchas). Pages are not tracked per session.

**Collateral damage.** Observations stamped into *another* project by this
session are deleted with the session, because the observation rows cascade
from the session. They are reported in `collateral_observations_deleted`.
Handoffs in other projects that this session authored or accepted keep their
text and lose the link. They are reported in `collateral_handoffs_denulled`.

Report fields: `session_id`, `workspace`, `project`, `observations_deleted`,
`handoffs_deleted`, `pages_deleted`, `auto_improve_runs_deleted`,
`removed_paths`, `collateral_observations_deleted`,
`collateral_handoffs_denulled`, `files_deleted`, `files_failed`, `compacted`,
`pre_checkpoint`, `checkpoint`.

Failure modes:

- **Session not in this workspace and project, or already purged**: 404,
  nothing changed.
- **No `confirm`**, or session id not a UUID: 400.
- **Admission rejected or pre-checkpoint failed**: 500, nothing changed.
- **Database failure**: 500, nothing committed.
- **File removal failed**: 200 with `files_failed`. The database rows are gone.
  Remove the listed paths by hand.

### `rename-project`

```bash
ai-memory rename-project --workspace default --from old-name --to new-name
```

Steps:

1. **Lookup.** Missing workspace or source project returns 404.
2. **Validate the new name.** The name is trimmed first. An empty or
   whitespace-only name returns 422. A name containing `/` returns 422. The
   name of the cross-project profile project is reserved and returns 422. The
   profile project itself cannot be renamed and returns 422. Leading and
   trailing whitespace are trimmed silently. They are not rejected.
3. **Update.** The name is changed. A name already used in the same workspace
   returns 422 ("name taken"). If the project was removed between the lookup
   and the update, the call returns 404.
4. **Count.** The number of latest page versions is counted for the report.
5. **Manifests and checkpoint.** The workspace and project `_meta.md` manifests
   are rewritten, and a checkpoint is committed if the tree changed.

Zero page files move, because disk paths are keyed by project id. This command
does not rename a source checkout or rewrite native agent session locators. See
[managed workstream rename behavior](managed-workstreams.md#project-and-directory-renames)
before you rename a checkout that has native sessions.

If step 5 fails after the rename committed, the call still returns 200. The
response carries `manifest_warning` and no checkpoint. The database rename
stands. Re-run the manifest refresh, or restore from the checkpoint the
response names, to repair manifest drift.

After a successful CLI rename, the client also updates its local checkout
link. If that update fails, the CLI prints a warning and the rename stands.
A direct HTTP call cannot update other machines' client registries. The next
successful managed run from a checkout refreshes its link.

Response: `workspace`, `from`, `to`, `pages`, `checkpoint`, `manifest_warning`.

### `/admin/rename-workspace`

HTTP admin route. It changes `workspaces.name` and leaves every on-disk path
unchanged, because paths are keyed by workspace id.

1. Missing source workspace returns 404.
2. A name already taken, or an invalid name, returns 422.
3. Otherwise the name is updated. Then every scope manifest is refreshed.
4. If the manifest refresh fails after the rename committed, the response is
   still 200. It has `manifests_refreshed: 0` and a `manifest_warning`. The
   database rename stands. Re-run the refresh, or restore from the checkpoint,
   to repair manifest drift.
5. The scope cache for the workspace is invalidated, and a checkpoint is
   committed if the tree changed.

### `/admin/delete-workspace`

HTTP admin route. There is no CLI subcommand for it. Request fields:
`workspace`, `force` (default `false`), `compact` (default `false`) and
`dry_run` (default `false`). It has no `confirm` field.

- **Dry run** (`dry_run: true`): returns a preview and changes nothing.

For a real run, in order:

1. **Lookup.** Missing workspace returns 404. The lookup never creates one.
2. **Non-empty check.** Without `force`, a workspace that still has projects
   returns 409 and nothing changes.
3. **Tree check.** If the workspace's wiki tree is not valid, the call returns
   500 and nothing changes.
4. **Admission.** The `purge_workspace` admission webhooks run before any
   deletion. A reject-policy failure returns 500 and nothing changes.
5. **Checkpoint.** A pre-delete checkpoint is committed if the tree is dirty.
   Failure returns 500 and nothing changes.
6. **Database delete.** One transaction deletes the workspace row. Cascades
   remove its projects, pages, sessions, observations, handoffs, embeddings,
   workstreams and managed runs. The whole workspace is tombstoned.
7. **File cleanup.** The workspace's wiki directory and each affected raw
   workstream directory are removed. Failures are listed in `files_failed`.
8. **After.** Mirror notifications are dispatched after the durable work, with
   `partial_failure` set when a file removal failed. Active-project pointers
   for the workspace are cleared, the scope cache is invalidated, and a
   post-delete checkpoint is committed if the tree changed.

Response fields: `workspace`, `projects_deleted`, `pages_deleted`,
`sessions_deleted`, `observations_deleted`, `handoffs_deleted`,
`embeddings_deleted`, `collateral_observations_deleted`,
`collateral_handoffs_denulled`, `workstreams_deleted`,
`managed_runs_deleted`, `workstream_ids`, `files_deleted`, `files_failed`,
`compacted`, `pre_checkpoint`, `checkpoint`, `dry_run`.

Failure modes:

- **Workspace not found**: 404, nothing changed.
- **Non-empty without `force`**: 409, nothing changed.
- **Reject-policy admission failed, or the pre-delete checkpoint failed**: 500,
  nothing changed.
- **File removal failed after the database commit**: 200 with `files_failed`
  filled in and `partial_failure: true` on the mirror notifications. Remove the
  reported path by hand.

### `move-project`

```bash
ai-memory move-project --from-workspace default --project my-project \
  --to-workspace other-workspace --confirm
```

Flags: `--from-workspace`, `--project`, `--to-workspace`, `--confirm`,
`--force`, `--on-conflict` (`block` (default), `overwrite` or `duplicate`).

Without `--confirm` the CLI refuses with a message. It sends no preview.

Unlike `rename-project`, which only changes a name, this crosses the workspace
boundary. The server checks, in order:

1. **Confirmation.** No `confirm` returns 400.
2. **Same workspace.** A source and destination workspace that are identical
   return 422. Use `rename-project` instead.
3. **Private profile.** A private cross-project profile project stays in the
   default workspace and cannot be moved. It returns 422.
4. **Source lookup.** Missing source returns 404.
5. **Active-session guard.** If this project is the one the hook router has
   published as the active project, the call returns 409 unless `force` is
   set. A live session's next observation would otherwise carry a stale
   workspace id.
6. **Merge detection.** The destination workspace is checked for a project with
   the same name. If there is one, the merge path runs. Otherwise the true move
   runs.
7. **Checkpoint.** A checkpoint named for the move is committed first. If that
   fails, the call returns 500 and nothing moves.

The reported `moved_via` tells you which strategy ran.

#### True move (fresh destination)

When the destination workspace has no project with the same name, the move is
lossless and cheap. The project keeps its id.

1. The destination workspace row is created if needed.
2. The destination project directory must not exist. If it does, the call
   returns 409 and nothing moves.
3. The `move_project` admission webhooks run with the source and destination
   names. A reject-policy failure returns 500 and nothing moves.
4. The project directory is renamed into the destination workspace's tree,
   under the wiki's exclusive lock.
5. In **one transaction**, the workspace id is re-stamped on: the project row,
   pages, sessions, observations, handoffs, audit rows, project grants,
   auto-improve runs, proposals, rejections, scheduler state and claims,
   consolidation jobs and managed workstreams. The project id is unchanged.
   Page embeddings and links follow page ids, so they need no change.
6. If the transaction fails, the directory is renamed back. If that rollback
   also fails, the error names the manual repair. The move is all-or-nothing
   unless the filesystem refuses the rollback.
7. The active-project pointer is retargeted to the new workspace. The scope
   cache is invalidated. Manifests are refreshed, and a post-move checkpoint is
   committed if the tree changed.

Nothing is purged. Sessions, observations, handoffs and the whole supersession
history move with the project. No page is rewritten and no embedding is
recomputed. A live managed run does not block a true move, because its lease
and ids are preserved.

Report: `moved_via: "true-move"`, `pages_copied` (the number of pages moved),
`workstreams_moved`, `source_purged: false`.

#### Copy-then-purge merge (destination has a same-named project)

Two different project ids cannot be re-stamped into one, because that would
collide on the unique name within the workspace. The merge copies the source
into the existing destination project, then purges the source.

1. The destination project is created if needed.
2. The source's **latest** page versions are listed. Older versions are not
   copied. The git history keeps them.
3. Each source page is compared with the page at the same path in the
   destination. A page conflicts when its body, its frontmatter, its title, its
   tier or its pinned flag differs. The generated-at timestamp in the
   frontmatter is ignored. Identical pages simply supersede at the same path.
4. The `on_conflict` policy decides what happens to conflicts:
   - **`block`** (default): if any conflict exists, nothing is copied. The call
     returns 409 with a `conflicts` list of the paths. The source is intact.
   - **`overwrite`**: the source version becomes the latest version at the
     same path. The destination's previous version stays in history.
   - **`duplicate`**: the source page is written to
     `<stem>-from-<source-workspace-slug>.<ext>`. If that is taken, it tries
     `<stem>-from-<source-workspace-slug>-2.<ext>`, then `-3`, and so on. The
     slug keeps ASCII letters, digits, `-` and `_`, and replaces every other
     character with `-`.

   Every conflict is listed in the response `conflicts` array as
   `path` to `moved_to`. For `overwrite`, `moved_to` equals `path`. The CLI
   sets the policy with `--on-conflict`. Direct HTTP callers set
   `"on_conflict"` in the body.
5. Each page is copied through the normal write path. Sanitization, link
   resolution, the full-text index, and the git mirror and admission hooks all
   run. The contributors enrichment webhook is skipped, because the frontmatter
   is copied verbatim. The bodies are copied without rewriting, so a link in a
   copied page that points to an old path stays as written.
6. Embeddings computed with the currently configured embedding model are
   carried over as stored vectors. Pages without a current-model vector are
   copied without one. Run `embed` afterwards to fill them in.
7. **Copy failure** (any page): the call returns 500 before the source is
   purged. The source is intact. Re-running is idempotent, because copied pages
   just supersede.
8. **Unreadable or invalid source page**: the page is skipped and listed in
   `pages_skipped`. The source purge does **not** run, because it would destroy
   data that was not copied. The call returns 200 with `source_purged: false`.
   Fix the page and re-run.
9. **Source purge.** This runs only when every page copied. The `purge_project`
   admission webhooks run on the source first, and a reject-policy failure
   returns 500 with the source intact. The purge then runs without `force` and
   without compaction. A live managed run under the source returns 409, with the
   count of pages already copied. The source stays intact until the run ends,
   and then the move can be re-run. `force` does not override this. A copy-merge
   never deletes a live lease.
10. **After the purge.** The source's wiki and raw directories are removed.
    Failures are listed in `files_failed`, and the purge webhook gets
    `partial_failure: true`. The active-project pointer for the source is
    cleared, the scope cache is invalidated, manifests are refreshed, and a
    post-move checkpoint is committed.

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

Two shapes share the same endpoint:

- **Single**: `session_id`, plus `project` (the destination) and optional
  `workspace` (the destination workspace, which defaults to the source's).
- **Batch**: `from_project` (plus optional `from_workspace`, default `default`),
  plus the destination.

Exactly one of `session_id` and `from_project` must be given. Otherwise the call
returns 400.

**Dry run by default.** Without `--confirm`, the server runs the same
transaction and rolls it back. The counts are exact (`dry_run: true`). The CLI
prints the exact command to apply. Nothing is written and no audit row is left.
With `--create` and a destination that does not exist, the dry run reports
`would_create_project: true` and the counts. It does not create the project.
Only the `--confirm` run creates it.

**Guards.** These return 409 unless `force` is set. They apply to dry runs too.

- The session is still open (no end recorded). A re-home (see below) skips this
  guard.
- A consolidation job for the session is pending or running. Its page would be
  written under the old scope.
- Batch only: the source project is the one the hook router has published as
  active.

Forcing an open session leaves the hook router's per-session pointer on the old
scope until it expires. Sticky routing then follows the session row, which now
points at the destination.

**Re-home.** If the session row already sits in the destination, the session
does not move. Only its rows still in another scope are gathered into the
destination. The response has `session_moved: false`.

**What moves, per session, in one transaction:**

- the session row;
- every observation of the session, including ones that landed in another scope;
- the handoffs it authored (`from_session_id`);
- its consolidation jobs, auto-improve runs and scheduler claims;
- its session page, according to `--pages`.

**Page handling:**

- `move` (default): every version of `sessions/<id>.md` is re-stamped into the
  destination, and the file is moved into the destination project's directory.
  The curated page and its history follow the session. If the destination
  already has a latest page or a file at that path, the call returns 409 and
  nothing moves. Retry with `regenerate`, or resolve the destination page first.
- `regenerate`: the source versions are retired (no longer latest), and any
  session summary pointer to them is cleared. The entity links of the retired
  page are closed. The file is removed, so the next consolidation of the session
  writes a fresh page in the destination. The file must go. A file left on disk
  with no latest row would be re-indexed as a new latest page by the next
  reconciliation pass.

**What does not move:**

- `sessions.cwd`. It stays as recorded. The response carries `cwd_warning` when
  its basename differs from the destination project name. New sessions started
  in that directory still resolve by basename, unless a `.ai-memory.toml`
  marker pins the project. See [marker file](marker-file.md).
- Other pages written during the session (decisions, gotchas).
- Handoffs the session *accepted*.
- Auto-improve proposals. They have no session id and target pages in the scope
  they were staged in.
- Entities and page feedback. The same gap applies to `move-project`.

**Order per session:**

1. Validate the destination. Missing destination returns 404, unless `create` is
   set.
2. Reject a batch whose source is the destination (422). A single move whose
   session already sits in the destination is a re-home, not an error.
3. Run the guards above.
4. With `--confirm` only, under the wiki's exclusive lock: run the
   `move_session` admission webhooks, with source names in the workspace and
   project context and destination names in the destination context. Then move
   the file on disk.
5. Re-stamp the database rows last. Disk goes first and the database last, as in
   `move-project`. A store failure renames the file back. The move is
   all-or-nothing unless the filesystem also refuses the rollback.

A `--confirm` run takes a checkpoint before and after. A batch takes one pair
for the whole run.

**Drains every scope, not just the one you named.** Dependent rows are matched
by session id alone. A session whose observations were scattered across projects
(for example from mid-session routing before sticky routing was on) is gathered
whole. That is the point of the command. It also means a move can empty a
project you did not mention. The move is **not cleanly reversible**. Moving the
session back returns every row to one project, and the original split is gone.

The dry run names each scope it would drain, with counts, and the response lists
them in `source_scopes`. Read that list before confirming.

**Batch.** Every session of the source scope moves one at a time, each in its own
transaction, oldest first. The batch stops at the first refusal. The error body
reports `moved` and `total`, and the sessions already moved stay moved. A dry run
of the batch shows the whole plan first.

Response (single): `session_id`, `dry_run`, `session_moved`, `would_create_project`
(dry run with `create` only), `source_scopes`, `from` and `to` (each with
`workspace` and `project`), `summary` (`observations`, `handoffs`,
`consolidation_jobs`, `auto_improve_runs`, `auto_improve_claims`,
`page_versions_moved`, `pages_regenerated`), `page` (`moved`, `regenerated`,
`already in destination` for a re-home whose page is already there, or `none`),
`cwd`, `cwd_warning`, `pre_checkpoint` (only when the tree had uncommitted
changes), `checkpoint` (only when the move changed the tree).

The batch wraps the same per-session reports in `{dry_run, would_create_project?,
from, to, total, moved, sessions: [...]}`.

Failure modes:

- **Neither or both of `session_id` and `from_project`**: 400.
- **Session or destination not found**: 404. Pass `create` to create the
  destination.
- **Batch source equals destination**: 422.
- **Open session (unless a re-home), pending consolidation job, or active source
  project (batch)**: 409 without `force`.
- **Destination already has a latest page or file at `sessions/<id>.md`**
  (`move` mode): 409, nothing moved.
- **Store failure after the file moved**: 500. The file is renamed back. The
  error names the manual repair if that rollback also fails.

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

Flags: `--path`, `--from` (a checkpoint id), `--workspace`, `--project`, `--json`.

Steps:

1. **Scope lookup.** Missing workspace or project returns 404. Nothing is created.
2. **Path check.** An invalid page path returns 422.
3. **Pre-restore checkpoint.** If the tree has uncommitted changes, a checkpoint
   named `pre-restore-page` is committed first.
4. **Read and validate.** The page's markdown is read from git at the given
   checkpoint, decoded as UTF-8 and parsed. The live file is not touched yet.
5. **Write.** The live file is replaced atomically, and the page is re-indexed.
   Its latest row is upserted, so search, links and the web view agree with
   disk.
6. **Post-restore checkpoint.** A `restore-page` checkpoint is committed if the
   live tree changed.

Failure modes:

- **Workspace or project not found**: 404, nothing changed.
- **Invalid page path**: 422, nothing changed.
- **Checkpoint or file not found** in git: 500. Any pre-restore checkpoint
  stays as an audit breadcrumb.
- **Historical markdown is malformed or not UTF-8**: 500. The live file is not
  replaced.

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
2. Takes a consistent snapshot of the live database without stopping the writer.
3. Writes a gzip tarball containing `wiki/`, the snapshot as `db/memory.sqlite`,
   and `config.toml` if it exists.
4. Returns the tarball as the response body, with content type
   `application/gzip`.

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

Flags: `--from`, `--force`, and the global `--data-dir`. There is no
`--confirm` flag.

A direct-disk operation. The server must be stopped.

Order of operations:

1. **Process check.** If any other `ai-memory` process is alive, the command
   refuses with the message "refusing to restore: N other ai-memory process(es)
   running (pids: ...). Stop them first, then re-run." Nothing changes.
2. **Source check.** The tarball must exist as a file.
3. **Occupied check.** The data directory counts as occupied if `wiki/` has any
   entries or the database file exists. If it is occupied and `--force` is not
   set, the command refuses: "refusing to restore: data dir at ... is non-empty
   (pass --force to overwrite)".
4. **Stage.** The tarball is extracted into a scratch directory inside the data
   directory. Each archive entry is checked. Only `config.toml`,
   `db/memory.sqlite`, and entries under `wiki/` are allowed. Absolute paths,
   path traversal, links and other entry types are rejected. A failure here
   leaves the live data untouched.
5. **Validate the database.** The staged database is opened. Pending migrations
   run here. A corrupt snapshot fails here, while the live data is still
   untouched.
6. **Swap.** The live `wiki/` and `db/` directories, and `config.toml` if the
   archive has one, are moved aside. The staged copies are moved into place.
   Each step is a rename within the same filesystem. If a step fails, the moves
   already made are reversed, and the previous data is back where it was.
7. **Discard.** The moved-aside copies are removed. This is the point after
   which the old data is gone, and the restore cannot be reversed.
8. **Scratch cleanup.** The staging directory is removed. If removal fails, a
   warning names the path to delete by hand.

`raw/`, `logs/` and `models/` are never touched by a restore.

Failure modes:

- **Another process alive**: refused. Same wording as the other direct-disk
  operations.
- **Tarball missing**: refused.
- **Data directory occupied without `--force`**: refused.
- **Archive invalid or corrupt, or the restored database will not open**:
  refused. The live data is unchanged.
- **Swap failed**: rolled back. If the rollback also failed, the error says the
  previous `wiki/` and `db/` are under the scratch directory and must be moved
  back by hand.

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

The use case is wipe and start over, for example when changing to a major
version with a breaking migration, or when bootstrapping a new install on an
old data directory. For a container deployment whose data is a host-path bind
mount, you can also empty the host directory after stopping the container. The
`reset` command is the portable path that works for local, bind-mounted and
named-volume data.

### `reindex`

```bash
ai-memory reindex --data-dir <path>
```

A direct-disk operation. It rebuilds the database from the wiki's markdown files.
It is a rebuild path, not a repair of a dirty index.

Order of operations:

1. **Process check.** If any other `ai-memory` process is alive, refuse.
2. **Clean-target check.** If the database already has rows in any of these
   tables, the command refuses and names the non-zero counts: workspaces,
   projects, pages, links, page embeddings, sessions, observations, handoffs,
   users, and audit rows. This stops stale database-only state from surviving
   silently.
3. **Walk the tree.** Each project directory is visited. Directories whose scope
   is tombstoned by a purge are skipped and left in place.
4. **Workspaces and projects.** Each one is recreated from its `_meta.md`
   manifest, keeping the UUIDs encoded in the directory names. A missing
   manifest is an error that gives its exact path.
5. **Pages.** The latest page rows, page links and the full-text index are
   rebuilt from the markdown. Embeddings are not computed. Run `embed` afterwards
   if you need them.
6. **Collisions.** A page whose path differs from an already indexed page only by
   letter case or Unicode normalization is skipped, because such paths cannot
   coexist on macOS or Windows. The count and each pair are reported.

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
