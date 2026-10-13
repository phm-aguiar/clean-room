# Optional companions and separate products

> **Escopo:** este documento contém partes que dependem de LLM ou de embeddings. Essas partes são opcionais e não fazem parte do escopo inicial, que funciona sem LLM. Podem ser removidas ou adiadas junto com as funções correspondentes.

This page sets the boundary for feature ideas that are useful around ai-memory
but should not become core surface area. Two earlier proposals, a migration
importer and a browser chat/editor, were valid product ideas. Both would have
built too much import, chat, UI, and mutation behavior into the core server.
The preferred shape is optional companion software that orchestrates ai-memory
through its public interfaces.

## Core boundary

ai-memory stays a memory substrate:

- one server process owns lifecycle hook ingestion, the MCP endpoint, the
  markdown wiki, the SQLite index, authentication, admission webhooks, and the
  built-in read-only browser;
- the wiki remains markdown-in-git as the source of truth;
- SQLite remains a derived index;
- writes go through the existing wiki mutation path, the admin endpoints, or
  MCP tools;
- the built-in browser and the JSON API stay read-oriented.

Companions can be richer products. They integrate through public HTTP and MCP
surfaces instead of adding handlers, routes, or command surfaces to the core.

### Public surfaces

| Surface | Routes or tools | Purpose |
|---|---|---|
| Read API | `GET /api/v1/...` for workspaces, projects, pages, search, recent pages, graph, briefing, overview, project overview, handoffs, sessions, and session observations | Read-only data for clients and frontends |
| Admin | `POST /admin/write-page`, `POST /admin/delete-page`, `GET /admin/status` | Operator-side writes and status |
| MCP tools | `memory_query`, `memory_read_page`, `memory_write_page`, `memory_delete_page`, `memory_consolidate`, `memory_auto_improve` | Agent and client access |
| Hook ingestion | `POST /hook`, `POST /hook/batch` | Capture of lifecycle events |
| Browser | `/web` (default slug, configurable) | Built-in read-oriented UI |

Limits on the read-oriented claim:

- The JSON search route also accepts `POST` with a JSON body. It is still a
  query and does not write.
- The browser includes a root-only triage page for pending writes. Its actions
  call admin routes, not browser-side mutations of pages.
- In multi-user mode, every operational admin route is root-only.

The whole HTTP surface can be served under a configurable URL prefix. The
examples in this page assume the default root.

## Integration rules for companions

Companion projects may:

- call the read-only `/api/v1` endpoints for workspaces, projects, pages,
  search, graph, recent pages, and briefing or overview snapshots;
- call the MCP tools `memory_write_page`, `memory_delete_page`,
  `memory_read_page`, and `memory_query` when running as an agent or client;
- call `/admin/write-page` and `/admin/delete-page` when running as an
  operator-side process with an appropriate bearer token;
- set the browser UI directory option (`--web-ui-dir`, or the matching
  environment variable) so ai-memory serves an alternate static single-page
  app in place of the built-in UI. The app must use public HTTP APIs and must
  not require in-process plugins. The read-only JSON API stays mounted when the
  browser is enabled;
- run their own LLM prompts, import transforms, queues, confirmation flows, UI
  state, and project-specific policies;
- ship their own CLI, web server, container image, tests, release cadence, and
  docs.

Companion projects should not:

- become part of the core build by default;
- add core MCP tools, admin endpoints, or CLI subcommands unless a missing seam
  is independently useful to ai-memory itself;
- write wiki files or database rows directly;
- bypass the central authorization check, admission webhooks, actor
  attribution, scope resolution, or the single-writer boundary of the store;
- require ai-memory to host arbitrary plugin code in-process.

Companion features should be treated as separate products, not rejected ideas.
They can move faster than the core, have their own UX, and carry source-specific
or workflow-specific behavior without widening the default install.

If a companion exposes browser writes, it must implement its own server-side
mutation broker. Browsers talk to the companion; the companion talks to
ai-memory with an operator token. That keeps CSRF protection, confirmation,
audit, rate limits, and UI-specific policy outside the core server.

## Two kinds of companion

The importer and the browser editor are **data-seam companions**. They talk to
ai-memory's public HTTP and MCP surfaces and build on the data it stores.

The working-tree coordinator is an **independent hook consumer**. It depends on
the agent harness's hook mechanism, not on ai-memory's surfaces. See
[Working-tree coordination](#working-tree-coordination-an-independent-hook-consumer).

## Importer companion

Status: implemented. It is the first companion. It is a standalone package with
its own workspace, is not a member of the core build, and is not covered by the
core test run.

### Goal

Import or normalize existing memory corpora without making the core own every
source format and migration workflow.

### Supported sources

Initial source support is intentionally narrow:

- **OMC flat wiki directories.** The importer reads only the top-level markdown
  files of the directory. It skips the index page and session-log pages by
  default. Destination paths are deterministic and live under an `omc/` prefix,
  one file per source page, with the file name derived from a slug of the
  source name.
- **Generic external conversations.** One JSON envelope per conversation, with
  `project`, `source`, `session_id`, and `messages`. Each message has a `role`
  (`system`, `user`, or `assistant`) and `content`. Unknown fields and unknown
  roles fail the import. Product-specific export adapters stay outside the
  importer; they only need to emit the generic envelope.

Limits for conversation envelopes:

- 2 MiB per file;
- 128 messages;
- 1 MiB of total message content;
- user messages are truncated (UTF-8 safe) to 16 KiB;
- system and assistant messages are truncated to 2,000 bytes.

Truncation counts are reported in dry-run output and in the manifest.

Future sources may include:

- memory-graph exports from a coding harness;
- vector-database collections, when a user supplies the collection address and
  a schema mapping;
- future one-off importers, released on the companion's own cadence.

### Replay of conversations

A conversation is replayed through ai-memory's public hook pipeline as one
ordered batch: one session-start event, the messages in order, and one
session-end event.

- User messages use the canonical user-prompt event.
- System and assistant messages use the importer's extension vocabulary, so
  their role and body stay available to later consolidation.
- Imported sessions use a dedicated `external-import` agent identity. They never
  impersonate a live coding harness. The session-start observation names the
  source as `external:<source>`.
- The imported session ID is derived from workspace, project, source, and the
  source's session ID. Each event has a stable ingest key.
- A changed transcript gets a new terminal key. Appending messages to an export
  therefore re-runs session-end after the new messages.
- If the server accepts only a prefix of the batch, the manifest is marked
  failed with the accepted count. Rerunning the same source resumes through the
  stable keys.

### How it talks to ai-memory

Read and plan:

- inspect the destination through the read API: workspaces, projects, pages,
  search, and graph. The current code reads the destination's page list and
  individual pages;
- default to dry-run, printing planned page writes without mutating ai-memory.

Write:

- write imported or normalized pages through `/admin/write-page` or the MCP
  write tool;
- do not delete in v1;
- use the MCP query and read tools, or the read API's search and page reads, for
  duplicate detection and context checks;
- optionally call consolidation or auto-improvement after import for
  post-import refinement, instead of building that refinement into the core;
- for bulk operations, loop over the single-page operation, unless ai-memory
  later adds a generic bulk-mutation seam for its own reasons.

## Usage model

Both modes share the same rules:

- The default mode is dry-run. It prints the planned writes or events and does
  not mutate ai-memory.
- Live mode requires `--apply`.
- The OMC import requires an explicit workspace, an explicit project, and a
  manifest output path in live mode.
- The conversation import requires an explicit workspace and a manifest output
  path in live mode. Its project comes from the envelope.
- The server address defaults to a loopback address on port 49374 and can be
  overridden with an environment variable.
- The auth token comes only from an environment variable. There is no
  command-line token argument.

Options relevant to safety:

- `--create-destination` lets the write endpoint create the workspace or project
  after the read preflight fails. Without it, the destination must exist.
- `--overwrite` is required to replace existing destination pages. Without it,
  the import aborts when destination pages already exist.
- `--include-session-logs` includes session-log pages in the OMC import.
- `--show-body` prints sanitized message bodies in dry-run output. Without it,
  bodies are hidden.
- `--pinned` pins every imported page.

## Importer safety requirements

- Never open ai-memory's database or wiki directory directly.
- Require an explicit destination workspace and project.
- Preserve only the metadata supported by the public write surface: `title`,
  `kind`, `tier`, `tags`, `pinned`, and `body`. Unknown frontmatter is ignored.
  Tier values are `working`, `episodic`, `semantic`, or `procedural`; the
  default is `semantic`. Do not claim arbitrary frontmatter or author
  preservation in imports, unless a future generic core seam adds it.
- Live writes use only the admin write endpoint. The importer never deletes
  pages, in v1.
- Before each write, the importer re-checks whether the destination page
  exists. This check is best-effort: a concurrent writer can race between the
  check and the write, so do not run competing imports into the same
  destination.
- The live run stops at the first write error. The manifest records the
  completed writes and the failed checkpoint.
- Path handling fails closed. Absolute paths, parent-directory segments, unsafe
  destination paths, and reserved or internal prefixes are rejected. Duplicate
  generated destination paths abort planning.
- Conversations are fully parsed, schema-checked, bounded, and sanitized before
  any manifest or HTTP request is created. Client-side credential redaction runs
  before replay, and ai-memory's server-side sanitizer is a second boundary.
- Native source identities that look sensitive are refused before a session ID
  is derived or a manifest is written.
- Body provenance is untrusted. It cannot supply actor identity or destination
  authority.
- Surface all destructive actions in dry-run output before live mode.
- Keep LLM normalization optional. A deterministic import works with no LLM.
- Keep provider-specific performance tweaks, such as model parameter changes,
  out of importer changes. If the core needs a provider fix or optimization,
  land it as a small standalone core change.

### Implementation plan

1. Build a read-only planner for one source format, with snapshot fixtures.
2. Add dry-run output and collision detection.
3. Add live writes through the existing public write surface.
4. Add optional LLM normalization as a companion-side pass. Not yet implemented.
5. Add re-home and link rewriting as a separate subcommand, after import is
   stable. Not yet implemented. See below.
6. Only after repeated use, decide whether the core lacks a small, generic API
   seam. Do not start by patching core endpoints.

Steps 1 to 3 are in place. Steps 4 and 5 are planned.

### Re-home by kind (planned)

The planned re-home command has these rules:

- compute the move and link-rewrite plan in the companion;
- apply moves as normal writes to the new path, plus deletes of the old path;
- preserve the frontmatter that ai-memory returns through page reads;
- fail closed on collisions, missing pages, or changed source hashes.

Deletes are the first use of the delete endpoint by this companion. The command
is not implemented yet.

## Browser editor companion (undecided)

Status: undecided. Do not implement this companion without a fresh design
review. There is no implementation in the source tree.

A useful writable version is larger than it first appears. It needs a safe
core compare-and-write seam, a companion mutation broker, browser auth and CSRF
protection, confirmation state, diffing, conflict handling, audit, and later an
LLM proposal policy. The benefit may not justify that complexity for
ai-memory's main goal. The system is meant to improve its own memory through
capture, consolidation, review, pending writes, and eval gates. Manual memory
editing may be less valuable than it seems, and could distract from improving
the automatic loop.

### Goal

Offer a richer browser product for chat, editing, and curation, without turning
the built-in browser into a write-capable application.

The built-in browser stays small: project list, tree view, markdown rendering,
search, and other read-oriented inspection. A separate editor can move faster
and make stronger product decisions.

### Product shape

A separate project with a backend and a frontend. The companion can be deployed
next to ai-memory and reverse-proxied under its own path or host, for example
`/editor`, while ai-memory keeps its own paths: `/api/v1`, `/mcp`, `/admin`,
`/hook`, and `/web`.

### How it talks to ai-memory

Read:

- use `/api/v1` for project lists, pages, recent pages, search, graph,
  briefing, and overview data;
- use the companion's own LLM provider for chat orchestration if it needs more
  than raw page and search context.

Write:

- browser requests go to the companion backend, never directly to ai-memory's
  admin routes;
- the companion backend performs CSRF checks, user and session policy, rate
  limiting, and confirmation state;
- after approval, the backend calls ai-memory's existing write and delete
  endpoints with a server-side token.

Mutation flow:

1. The LLM proposes a patch, a create, or a delete as a pending action.
2. The UI shows an explicit diff and the target workspace, project, and path.
3. The user confirms or rejects the pending action.
4. The companion re-reads the current page and verifies the expected base hash.
5. The companion applies the write or delete through ai-memory's public
   mutation path, and records its own audit trail.

### Safety requirements

- No browser writes are applied automatically from an LLM response.
- Deletes always require explicit confirmation.
- Edits preserve existing metadata unless the user deliberately changes it.
- A folder or search scope limits context. It is not a mutation boundary. The
  backend must independently authorize the target page before applying a change.
- If the UI offers folder-scoped editing, the companion must enforce on the
  server that target paths stay inside the allowed folder or project.
- The companion must not rely on cookie or basic auth to perform non-GET
  ai-memory mutations from the browser. Use a server-side token and the
  companion's own CSRF and session protection.
- In multi-user mode, `/admin/*` is root-only. A companion must either run with
  an operator token or use MCP flows appropriate to the actor. It must not
  assume normal user tokens can perform admin writes.
- Propagate actor and author context wherever the public write surface supports
  it, so admission webhooks and audit stay meaningful.
- Keep `/api/v1` read-only. Do not ask the core to expose writable,
  cross-origin browser endpoints for this product.

### Implementation plan (parked)

This plan stays parked until the benefit is clearer.

1. Build a read-only editor shell against `/api/v1` first.
2. Add chat over selected page and search context, still read-only.
3. Add pending edit proposals with diff preview, but no apply action.
4. Add confirmed writes through the companion backend and ai-memory's public
   write endpoints.
5. Add confirmed deletes last.
6. Keep the built-in browser unchanged unless the core independently needs a
   small read-only API enhancement.

## macOS menu bar companion

Status: implemented as a wrapper. It is a self-contained accessory app. It is
not part of the core build and has its own build and test steps.

It is a **wrapper**, not a data dashboard. It bundles the ai-memory server
binary and the hook scripts into an app bundle, manages the server's per-user
LaunchAgent, and opens the browser UI, the status command output, the config
file, the data directory, and the logs.

### Goal

Give macOS a first-class install that does not need a prior tarball, without
reimplementing status, search, wiki browsing, or config editing in a native UI.

### How it talks to ai-memory

- `GET /admin/status` drives the menu-bar status icon and the summary lines
  (version, page and session counts, LLM role, bind address, write queue, ingest
  counts, and last write).
- The bundled CLI runs as a child process for `init` (on first run, when the
  config file is absent) and `status`. It receives the data directory as an
  argument.
- The service manager controls the LaunchAgent for the server, by a fixed label.
  The LaunchAgent plist is rendered from a template with the binary path and
  the home directory. A data-directory environment variable is added only when
  the user sets an override.
- The app opens the browser UI, the config file, the data directory, and the
  stderr log through the system workspace.

### Data and bundle

- Durable state stays in the user's Application Support folder for ai-memory,
  which is the binary's macOS default. Logs stay under the user's Library Logs
  folder for ai-memory.
- Replacing the app bundle does not rewrite that data.
- The optional data-directory override is written only into the rendered
  LaunchAgent, as `AI_MEMORY_DATA_DIR`. It is never written into the bundle.
  Changing the setting does not move existing files.

### Safety requirements

- The app does not start the LaunchAgent silently on first launch. The
  install-and-start action is an explicit user click.
- The auth token is never written into the plist. If the user saves a bearer
  token, the app keeps it in the system credential store and passes it only to
  the bundled status command, as an environment variable.
- The app does not open the database or the wiki, does not call writable admin
  routes, and does not add MCP tools.
- Do not sandbox the app in a way that blocks the service manager or
  LaunchAgents.

### Build

Run the companion's build script. It builds the server binary, builds the
native menu app, and stages the app bundle. The build needs the full Apple
developer toolchain, not only the command-line tools. Run the companion's own
test suite separately from the core checks.

For the companion's other services and tests, see
[External companions and tests](external-companions-and-tests.md).

## When to move a seam into core

A companion may reveal a missing primitive that belongs in ai-memory. Move only
small, generic seams into the core, and only after the companion proves the
need.

Good core candidates:

- a read-only API field needed by several clients;
- a narrowly scoped mutation endpoint equivalent to an existing MCP tool;
- a capability check or scope-resolution helper that prevents duplicated
  security logic.

Poor core candidates:

- source-specific import parsers;
- UI workflows;
- LLM chat prompts for editing;
- project-specific scoring, pruning, or normalization policies;
- companion-only admin commands.

This keeps ai-memory stable while still allowing richer tools to grow around it.

## Working-tree coordination: an independent hook consumer

Status: decision recorded. No coordinator exists in the source tree.

Several agent sessions sharing one checkout collide over the single git index.
One session's bulk staging can sweep up another session's staged work, and a
fix run can rewrite a tree that another session has claimed. The natural
instinct is to build the guard on ai-memory's captured tool-use events. That
does not work, for two deliberate reasons.

- **ai-memory cannot block a tool action.** The hook endpoint is capture-only
  and fire-and-forget. It answers that the event was accepted and queued, or
  that it was rate-limited or the queue is full. It never answers allow, deny,
  or ask, and it processes the event after responding. Hooks that wait on a
  round trip to the memory server can deadlock the agent engine, so there is no
  synchronous veto channel back to the harness, by design.
- **ai-memory does not retain file paths.** For closed-tool agents, the stored
  observation keeps a tool-family label and the outcome. Raw arguments, paths,
  and arbitrary tool names are extracted only transiently, for denylist
  matching, and then dropped. A consumer can see that a file operation happened
  in a session, but never which file.

Reversing either behavior, by persisting paths or adding a blocking hook, would
trade away a privacy and bounding invariant or the anti-deadlock invariant. Both
stay.

A working-tree coordinator therefore installs its own pre-tool-use hook next to
ai-memory's. It reads the raw tool input and arbitrates synchronously in its own
process, with its own ephemeral ownership state (a lock file or a small store).
It never uses the wiki or the database. It is a sibling on the same hook event,
not a seam consumer. It may still live under the companions area for
discoverability.

Scope it to the shared-index and file-ownership class. Stale-build and host
saturation problems belong to build and CI orchestration, which is a separate
problem.
