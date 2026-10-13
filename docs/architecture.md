# ai-memory - Architecture

> **Escopo:** este documento contém partes que dependem de LLM ou de embeddings. Essas partes são opcionais e não fazem parte do escopo inicial, que funciona sem LLM. Podem ser removidas ou adiadas junto com as funções correspondentes.

> The operational summary of how the system is shaped and what it guarantees.
> Long-form design rationale lives in the linked design documents. The
> reading order at the end points to the detailed references.

## Purpose

ai-memory is a single executable that gives coding agents listed in the
[support matrix](support-matrix.md), and other MCP-capable clients, long-term
memory shared across tools. A session can be stopped in one agent and
continued in another opened in the same directory. No manual note-writing
and no copying summaries between sessions.

The durable artifact is a **compiled wiki**: a git-versioned tree of markdown
pages on disk that is updated over time. Pages are versioned in place through
supersession, semantic concepts accumulate, and episodic logs decay. A
derived SQLite index provides full-text, entity, and link-neighbour
retrieval. The markdown files remain the source of
truth.

## Data flow

Solid arrows are request, read, and write paths. Dashed arrows are background
reconciliation and maintenance. The core invariant: the
markdown wiki is the source of truth, and SQLite is the derived index for
search, sessions, observations, handoffs, audit, and the optional
managed-workstream continuity ledger.

**Steady-state loop:**

1. **Capture.** The agent CLI emits a lifecycle hook (SessionStart,
   UserPromptSubmit, PostToolUse, and so on). Shell-script hooks send the
   event JSON to `POST /hook` with a 0.2 s request timeout. Native
   `ai-memory hook --event ...` commands write events to a local spool with a
   stable per-entry idempotency key. They run a short bounded cleanup at
   session start and hand session-end delivery to a detached, lock-aware
   drain helper. Operators with slow links can raise the drain, handoff, start,
   and background budgets through the minute-based environment variables
   `AI_MEMORY_HOOK_DRAIN_TIMEOUT_MINUTES`, `AI_MEMORY_HOOK_HANDOFF_TIMEOUT_MINUTES`,
   `AI_MEMORY_HOOK_START_BUDGET_MINUTES`, and
   `AI_MEMORY_HOOK_BACKGROUND_DRAIN_BUDGET_MINUTES`. The defaults are 3 s for the
   first three and 5 minutes for the background budget. Overrides are limited to
   60 minutes. Agent hot paths never block on the network. A saturated server answers HTTP 429 instead of queueing work
   without bound.
   For supported native commands and generated integrations, the
   nearest-marker capture policy runs first. A dropped file-tool event never
   reaches the spool, the queue, the transport, the logs, or the storage. See
   [Capture exclusions](marker-file.md#capture-exclusions).
2. **Admission.** The server sanitizes every payload. This is the only path
   from untrusted text into storage. It assigns a canonical observation kind
   (see [Hook event vocabulary](#hook-event-vocabulary)) and writes the
   observation. For keyed native events, the project-scoped key and the
   observation commit together. The key is marked complete only after the
   downstream processing finishes. An incomplete replay resumes the wiki and
   handoff effects without storing a second observation. A completed replay is
   acknowledged and skipped. An overlapping retry for the same key waits behind
   the original processor through a bounded gate. Downstream effects are
   at-least-once: a crash during them can repeat an already applied effect, but
   it does not silently drop the rest. An interrupted SessionEnd replay converges
   the wiki commit and the pending key, without
   appending another observation (a confirmar). Each wiki write appends a line
   of the form `## [YYYY-MM-DDTHH:MM:SSZ] <event> | <title>` to the wiki log.
3. **Session end.** On a true SessionEnd event, the server writes a rule-based
   session summary page (no LLM) under `sessions/` and opens a handoff for the
   next agent. The automatic handoff, the session's ended stamp, and the
   covered observation count are written in one database transaction (a
   confirmar). A later SessionEnd repeats this path only when the session's
   observation generation has advanced. Resumed sessions are therefore captured,
   while duplicate delivery and clock skew converge. Sessions that already ended
   when the store migrates are baselined, not replayed as historical work. The
   wiki is committed automatically.
   Clients without a reliable session-end hook need an explicit ending action.
   For Antigravity CLI this is `ai-memory finalize-session --agent antigravity-cli`.
   Codex has a native SessionEnd event from Codex CLI 0.145.0 onward.
   `finalize-session --agent codex` is the fallback for older Codex versions.
   The command selects the most recent matching open session and runs the same
   path as a native hook.
   The session page's frontmatter records `session_id` and the session's
   immutable agent kind, as `agent`. This value describes the harness that
   originated the session, not a later writer. Manual page writes receive no
   inferred agent metadata.
4. **Query.** `memory_query` combines full-text, entity-match, and
   link-neighbour results with reciprocal rank fusion (RRF). The entity index is derived from the canonical frontmatter `entities`
   list. An empty index contributes no candidates and no score.
   Before final truncation, a bounded authority multiplier adjusts relevance
   using the page's kind, tier, `pinned` flag, and explicit positive or negative
   frontmatter tags. The multiplier is clamped to `[0.55, 1.50]`. It favors
   maintained rules, decisions, procedures, and gotchas in close contests, while
   episodic, historical, lint, and test evidence stays searchable. By default,
   no query-intent routing takes part in ranking. An opt-in lexical
   session-recall routing exists under `[retrieval] query_intent` (off by
   default).
   If compiled pages return nothing in default, explicit-project, or explicit
   `scopes` mode, a bounded full-text search over raw observations returns
   fallback `raw_hits`. `global=true` searches compiled pages across all projects
   only.
   Page hits bump the page's access count and last-access time. This is the
   reinforcement term of the decay model. `memory_feedback` complements it with
   explicit per-page salience. Identified operators also add one access record
   per page and operator. An opt-in `[decay] breadth_weight` rewards pages
   reinforced by several distinct operators. The bump is throttled to at most
   once per page per minute, so a burst of overlapping searches does not flood
   the writer with redundant reinforcement writes. Other read paths that surface
   a page reinforce it through the same throttled path (`memory_read_page`,
   including its related-page walk) (a confirmar).
5. **Retention.** The forget sweep runs on demand and on the server's
   `[maintenance]` schedule. Its passes are described in
   [Memory tiers and retention](#memory-tiers-and-retention).
6. **Backups.** `ai-memory backup --to <tarball>` asks the server to snapshot
   the SQLite database with its online backup mechanism, so the writer stays
   live. `ai-memory restore` reverses the operation. Alternatively, the wiki
   directory can be synced with git and the data directory copied.

**Optional managed-workstream loop.** `ai-memory run` opens a lease for the
current repository and worktree workstream. It resolves an explicit harness, or
the newest usable local or linked harness. It creates or resumes that harness's
native session, and marks lifecycle calls with an invocation-scoped run id.
SessionStart injects an unseen, bounded event range. A harness without
SessionStart receives it through a temporary, supported global-context path
(a confirmar). When the child process exits, the host imports the native
transcript tail and a Git checkpoint (a confirmar).
Every injected packet starts with a versioned origin marker. The Claude
transcript normalizer excludes a marked packet if the harness persists it and
reads it back, so delivered history does not re-enter the ledger (a confirmar).
An explicitly pending handoff is delivered before the managed event range.
Their single-use delivery claims share one write transaction, made after the
complete startup response is assembled (a confirmar). Manual handoffs take
precedence. Otherwise the newest automatic handoff eligible for the current
directory is delivered. That same transaction expires older eligible automatic
handoffs but keeps manual and sibling-directory handoffs. Inserting a new
automatic handoff also expires prior open automatic handoffs from the exact same
directory, which bounds repeated session ends before any receiver starts.
Native harness stores are opened read-only. Raw sanitized JSONL segments are
immutable. SQLite provides monotonic sequences, full-text search, native source
and delivery cursors, and idempotent retry state. A full-ledger
`workstream-search` complements the bounded startup packet. An interactive
empty workstream may adopt a checkout-matching native session once. Eligibility
comes from the authoritative ledger and session state. After any harness
establishes the workstream, a newly joining harness starts fresh and receives
portable history, not unrelated old native history. A handled launcher failure
cancels its lease. A normal reopen retries brief finalization conflicts. An
unclean process death is bounded by the renewable lease expiry. See
[Managed cross-harness workstreams](managed-workstreams.md).

### End-to-end flow by component

Each step, attributed to the component that performs it:

```text
Agent (Claude / Codex / ...)
  | hook JSON (stdin)
  v
CLI hook -- capture policy -- local spool -- drain at boundaries
  | POST /hook/batch  (idempotent ingest key)
  v
hook router -- sanitizes (typed boundary) -- transactional admission
  |                                       (session + observation + key)
  | SessionEnd: synthesis (session page) -> wiki page write -> automatic handoff -> git commit
  v
wiki page write: scrub -> admission webhooks -> atomic file -> index
                 (supersession, full-text, links, entities, evidence) -> git commit
  ^
MCP tools (memory_query: full-text + entity + graph -> RRF -> authority)
  ^
Agent calls MCP; the web interface and /api/v1 read the same state (read-only)

Background (CLI schedules):
  forget sweep (decay / TTL / compaction / hard-delete / prune) · lint · curator

Cross-harness continuity: CLI run <-> hook workstream (/workstream/*) <-> workstream store ·
  transcript adapters feed the portable ledger.
```

## Hook event vocabulary

The observation vocabulary is a closed set of agent lifecycle events. Hook
bridges may accept client-specific aliases, but storage normalizes them to
exactly one of these kinds:

| Stored kind | Meaning |
|---|---|
| `session-start` | Agent session began. The working directory, model, and session identity are captured. |
| `user-prompt` | The user submitted prompt text to the agent. |
| `pre-tool-use` | The agent is about to call a tool. |
| `post-tool-use` | The agent finished a tool call. |
| `pre-compact` | The agent is about to compact or compress its context. |
| `post-compaction` | The agent compacted its context and supplied a post-facto summary or checkpoint. |
| `notification` | The agent emitted a notification-style event. |
| `stop` | The agent finished an interactive turn or stopped naturally. |
| `session-end` | The agent session ended. The summary and handoff path may run. |
| `other` | An unknown or unsupported hook event. |

Antigravity CLI has no native SessionStart event. Its pre-invocation hook fires
before every model call. The bridge maps only the payload with
`invocationNum = 0` to `session-start`. Later invocations are ignored before
any spool or network side effect.

Unknown events do **not** extend the set. By default they leave no source-event
metadata in storage and collapse to `other`. Third-party integrations that need
their own vocabulary can opt in by sending `extension=<namespace>` on `/hook`.
With a valid namespace, the server stores `source_event=<name>` when the client
supplies it, or the unknown event string when it does not. This pair is nullable
observation metadata, and the kind stays canonical. This is an extension seam,
not a runtime plugin system. External processors must use the HTTP and MCP
interfaces and cannot bypass the sanitizer, the hook backpressure, or the
single-writer rule.

**Content limits.** Lifecycle bodies have limits separate from the 10 MiB HTTP
request limit:

* User prompts are capped at 16 KiB, cut on a UTF-8 character boundary.
  Post-compaction summaries use the same cap (a confirmar).
* Notification and tool excerpts are capped at 2 KB.
* Native `ai-memory hook` commands apply the event-specific cap before spooling
  and transport. The server applies it again when it parses each request, so
  direct and older clients cannot bypass it.
* The sanitizer applies a 16 KiB backstop to every durable observation body
  after redaction.
* The opt-in assistant final-message excerpt (Claude Code and Stop events) is
  capped at 2 KB and passes through the same sanitizer boundary.

**Client-supplied event time.** An optional RFC 3339 `occurred_at` field on the
hook body lets a client supply the event's original time. Today only the
`ai-memory backfill` command uses it, when it replays a transcript's per-event
timestamps. Imported sessions and observations are then stamped with when they
happened, not with the import time. The field is read from the top level of the
body only, so a harness payload that happens to contain an `occurred_at` key
inside its own structure is never mistaken for it (a confirmar). It is numeric
metadata, not text, so it does not go through the sanitizer. It is still
client-controlled, so the server accepts it only if it is greater than zero and
at most five minutes ahead of server time. Anything else (missing, unparsable,
or out of bounds) resolves to "now". The store treats it as "now" and never
returns an error, which keeps hooks fire-and-forget.
A backfilled session's ended time can therefore land well in the past. A retention window measured from an
observation's own time can also make an old backfilled observation prunable
immediately instead of after it ages in place.

## Storage architecture

**Two layers, one source of truth.**

* `<data_dir>/wiki/` is the markdown source of truth. It is a git repository,
  so every session end produces a durable commit.
  It can be edited by hand in any text editor. A file watcher reconciles outside
  edits.
* `<data_dir>/db/memory.sqlite` is the derived index. It runs in WAL mode.
  One writer owns the write connection. Writes arrive through one queue and are
  applied by a single dedicated thread. Reads go through a cloneable read-only
  pool.
* `<data_dir>/raw/` holds immutable, sanitized managed-workstream JSONL segments.
  Legacy raw fallback recall searches the durable observations table through
  full-text search. Lifecycle event payloads are not a complete transcript
  archive.
* `<data_dir>/logs/` holds daily-rotated log files.
* `<data_dir>/client-projects.json` is a private, client-local list of checkout
  links for `ai-memory show`, keyed by a credential-free server identity plus
  workspace and project. It is not part of the database or wiki source of
  truth, and no server API exposes host paths (a confirmar).

**Schema (current head).**

| Table | Contents |
|---|---|
| `workspaces`, `projects` | The top of the identity coordinate. Every domain row carries the workspace, project, and path. |
| `pages` | Versioned wiki pages with an `is_latest` flag and a `supersedes` chain. Also stores last-access time, access count, a decay tombstone marker (`superseded_at`), the frontmatter TTL (`expires_at`), and explicit salience (NULL means the default, derived from feedback). |
| `pages_fts` | Full-text index over page title and body, kept in sync by triggers. |
| `sessions`, `observations` | Sanitized, bounded projections of lifecycle events. `sessions.ended_observation_count` is the stable generation watermark that decides whether a resumed session may be re-ended. Wall-clock time is not used for that decision. These tables are an operational audit trail, not a complete native transcript. |
| `observations_fts` | Full-text index over raw observation title and body. Used only as a bounded fallback. |
| `workstreams`, `managed_runs`, `workstream_native_sessions` | Optional lease state for `ai-memory run`, plus per-harness native source and delivery cursors. |
| `workstream_events`, `workstream_events_fts` | Append-only normalized transcript events and their full-text index. Immutable sanitized source batches also live under `raw/`. |
| `links` | Wikilinks and markdown cross-references. The target page id is nullable for forward links that do not resolve yet. Target workspace and project carry a cross-project scope. NULL means the source page's own project. |
| `handoffs` | Typed cross-agent handoff records, in the states open, accepted, or expired. |
| `page_feedback` | Append-only `memory_feedback` signals (`helpful`, `not_helpful`, `stale`, `wrong`), keyed by page version. It has an optional sanitized reason and the resulting salience. This is the source of truth for the derived page salience. The lint pass reads unresolved `stale` and `wrong` rows for the latest versions, so a rewrite retires the finding. |
| `page_access` | One row per latest page and qualified operator identity. Feeds the optional access-breadth term without changing the shared access counter. |
| `page_evidence` | Append-only record of what produced or reaffirmed each page version. The session summary records the session it ran on. Written in the same transaction as the page, and removed when the page is purged. It feeds a read-time, zero-LLM belief-strength `confidence`: a bounded function of distinct supporting sessions (breadth, not raw count), the recency of the newest sighting, and the number of live `contradicts` links. The result is capped at 0.95. It is exposed as `confidence` and `evidence_count` in `memory_query` explain output, as `evidence_rows` in `memory_status`, and it orders the opt-in `settled_first` briefing. It is ranking-inert by default. It folds into the page authority only when `[retrieval] belief_authority_weight` is above 0 (default 0, off). A supersession always wins regardless of evidence. A superseded version is never boosted. |
| `agent_messages` | Cross-project message inbox and queue. Directed, claim-once mail from a sender coordinate to a recipient coordinate. States: pending, claimed (popped exactly once), or cancelled by the sender. This is the only table that crosses per-project isolation, so reads are keyed by the recipient coordinate (inbox) or the sender coordinate (outbox). The user attribution fields are informational and never filter reads. The recipient inbox depth is capped. |
| `pages.compacted_at` | Marker set when the forget sweep compacts a cold episodic page in place, instead of evicting it (opt-in). It is set in the same transaction as the compacted body. The sweep and the curator skip a marked page, so it is never compacted, evicted, or reported as cold again. The full pre-compaction body stays in git and in the supersession chain. |
| `client_activity` | Server-wide MCP tool-call counters, split into reads and writes and bucketed by UTC day. Buffered calls are flushed on a one-minute background interval. Failed batches retry from bounded memory. Each day stores at most 128 sanitized client labels plus `other`, so an untrusted client name cannot create traffic-proportional rows. |
| `entities`, `entity_page_links` | Noun index derived from canonical frontmatter. Names are normalized and unique per project. Links point at immutable page versions, while retrieval filters to the latest version. Triggers prevent links across projects. This index feeds the entity stream of the fusion. |
| `audit_log` | Every mutation, addressable by time, newest first. |

**Memory tiers and retention.**

| Tier | Lifetime | Decay |
|---|---|---|
| Working | Current session (lifetime handling a confirmar) | Not eligible for decay eviction in the sweep. |
| Episodic | Retained while its score stays above the cold threshold. Then evicted, or compacted (opt-in). | Eligible for decay eviction. Score formula below. |
| Semantic | Indefinite. Default tier when a page has no tier in its frontmatter. | Not eligible for decay eviction. Changed only by supersession. |
| Procedural | Indefinite | Not eligible for decay eviction in the sweep. Included as a candidate for the contradiction check in lint. |

Only episodic, unpinned pages are eligible for decay eviction. Pinned pages are
exempt from every decay path. A page is pinned when its frontmatter has
`pinned: true`, or when it lives under `_slots/`.

For an eligible page, the retention score is:

```text
score = salience * exp(-lambda * age_days)
      + sigma * ln(1 + access_count) * exp(-mu * days_since_access) * breadth

breadth = 1 + breadth_weight * ln(1 + max(distinct_operators - 1, 0))
```

* The access term is zero when the page has no recorded access.
* `breadth_weight` defaults to 0. With the default, the breadth factor is 1.
* Salience defaults to 1.0. An explicit per-page salience replaces the default.
* `lambda` is the scalar `[decay] lambda` (default 0.02). It can be set per tier
  through `[decay.half_life_days]`, a half-life in days converted to
  `lambda = ln(2) / days`. A tier without a value uses the scalar, so the
  default reproduces the single-lambda scores exactly.

When the score falls below `cold_threshold` (default 0.20), the sweep evicts the
page through the wiki layer, removes the authoritative file, and leaves a decay
tombstone (a confirmar: tombstone behavior).

**Extractive compaction (opt-in).** With `[decay] compact_cold_episodic = true`
(default false), the sweep runs a compaction pass before the eviction pass. A
cold episodic page that is not already compacted is rewritten through the wiki
layer. The rewrite keeps the page's abstract, a first-paragraph summary, and a
set of keep-tokens (paths, URLs, code spans, error codes, identifiers), and drops
the prose (the exact set of kept elements is a confirmar). The rewrite supersedes
the prior version, so the full body stays
reachable through git and the supersession chain, and a restore command can
recover it. The compacted marker makes the page terminal for the decay pass. It
is never compacted again, never evicted, and never reported as cold. This pass
uses no LLM.

**Forget sweep passes.** On each run, the sweep does the following:

1. **TTL.** Pages past their frontmatter `expires_at` are hard-deleted through
   the wiki layer, both file and rows. This applies whether or not the page is
   pinned (a confirmar: pinned behavior).
2. **Decay eviction.** Cold episodic pages are evicted (see above).
3. **Tombstone purge.** Tombstones older than `hard_delete_after_days` (default
   180) are purged together with their full version ancestry. The purge is
   limited to the sweep's resolved workspace and project, and it removes the
   entity rows orphaned by the purge. A newer page recreated at the same path
   is preserved (a confirmar: recreated-page preservation).
4. **Observation prune (opt-in).** Disabled unless `[decay]
   observation_retention_days` is positive (default 0, never prune). It deletes
   raw observations older than that age, but only for sessions whose summary
   page is still live (not superseded). Raw capture is never removed while it
   is the last copy of a session's work. The pass runs last, so this run's own
   evictions and purges already exclude their sessions. It deletes in batches of
   `observation_prune_batch` rows (default 5000), each in its own transaction, so
   a very large prune cannot hold the write lock for long. It also lowers each
   session's observation count in the same transaction.
   The prune is irreversible. Observations are the only raw record of a session,
   so a pruned session can never be re-derived from them. Its summary
   page becomes the only surviving account of that session.

Freed database pages are reused, not returned to the operating system. The
database file does not shrink, but the backup archive does.

Scheduled sweeps and rule-based lint enumerate every existing workspace and project scope before doing per-project work. A separate
daily cleanup removes project rows at least 7 days old that contain no pages,
sessions, observations, handoffs, or managed workstreams.
Managed continuity history therefore keeps its project scope alive even when no
lifecycle-hook session has been captured (a confirmar: daily schedule).

## Cross-project links

Pages normally link within their own project, for example `[[decisions/0001.md]]`,
or a labelled link pointing at `../gotchas/x.md`. A wikilink can also name
another project, so that dependencies between projects become explicit edges in
the graph:

* `[[project:path.md]]` links to a sibling project in the same workspace.
* `[[workspace/project:path.md]]` links to a project in another workspace.

The parser yields a target with workspace, project, and path. The store
resolves it against the named project's latest page and records the scope in
the link row. A link to a page that does not exist yet stays unresolved. It is
re-pointed when that page later lands, across projects as well as within one
(a confirmar: re-pointing mechanism).

Because the target page id is global, a resolved cross-project link appears as a
backlink on its target. Related-page results carry the source's workspace and
project, so the dependency is labelled and navigable (a confirmar). This is what
turns the per-project wikis into one dependency graph. It is visible through the
graph endpoint of the web interface, and also through the dangling reference
check in lint and the dependent counts in briefings (a confirmar).

## Component responsibilities

The system is split into functional layers. Each layer has one responsibility
and exposes a typed interface. Dependencies do not form cycles, and each
boundary enforces one or more of the invariants below. The absence of
dependency cycles is a confirmar.

* **Domain model.** Types, identifiers, and errors. No input or output.
* **Storage.** Database schema and migrations, the single writer, the read-only
  reader pool, and the decay math.
* **Wiki files.** Atomic markdown writes, the file watcher, the git history, and
  the admission webhooks.
* **MCP server.** The protocol transport, the tool router, and the admin
  endpoints.
* **Hook ingress.** Payload schemas, the sanitizer, the `/hook` endpoints, and
  the capture policy.
* **Maintenance.** The ingest, lint, and forget sweep pipelines.
* **Workstream adapters.** Read-only native transcript readers and launch
  adapters.
* **CLI.** The `ai-memory` entry point and thin HTTP subcommands.
* **Web interface.** A read-only wiki browser served under `/web`, with JSON
  APIs under `/api/v1`.

## MCP tool surface (20 tools)

Hints are the server's tool annotations (a confirmar for each tool).

| Tool | Hint | Purpose |
|---|---|---|
| `memory_query` | read-only | Full-text, entity-match, and graph RRF search, followed by the bounded kind, tier, pinned, and tag authority adjustment and raw fallback. Bumps access counters for page hits. Defaults to the current project. Default-scoped calls also merge the reserved `_global` preferences scope as `global_scope_hits`. `scopes` searches named sibling projects. `global=true` searches every project at once, and each hit is labelled with its workspace and project. `explain=true` adds per-hit score details: per-stream ranks, matched entities, raw scores, RRF contributions, graph provenance including the typed edge kind (`causes`, `fixes`, `contradicts`), the evidence count, and the authority multiplier. It also adds a top-level `streams_active` list. `include_expired=true` also returns TTL-expired pages. `include_superseded=true` also returns superseded versions, each labelled `superseded: true`. The current version is never labelled. With the default `false`, the response is identical to the latest-only behavior. `pin_first=true` prepends up to 10 pinned latest pages of the project, deduplicated by page id and marked `pinned: true`, then re-truncates to the requested limit. It applies to single-project searches only. |
| `memory_recent` | read-only | The most recently updated latest pages. |
| `memory_read_page` | read-only | Fetches the full body of one page, by path or by the top full-text hit for a query. Optional workspace and project target a named sibling. `include_related=true` walks the link graph outward from the page: a bounded breadth-first walk, default 1 hop (a confirmar), hard cap 3 hops, a visited set against cycles, and at most 50 nodes in total, across projects. Each related page carries its depth and direction (`link` or `backlink`). With the default, the response has no `related` field. |
| `memory_read_session_observations` | read-only | Pages through one session's raw observations, with the full sanitized body capped per row by `body_max_chars`. Restricted to rows in the resolved scope and to sessions the caller may see. Reports `total` and `elided_other_scope`. If no session id is given, the latest completed visible session is read. |
| `memory_status` | read-only | Counts, paths, version, and the `scope` that answered: workspace, project, and `resolved_by` (`explicit`, `session`, `shared_slot`, `startup_seed`, `default`, `default_after_mismatch`; the value list is a confirmar). Unscoped reads resolved by `startup_seed` or `default_after_mismatch` also log a server warning. |
| `memory_briefing` | read-only | Structured counts, activity, rules, slots, and recent pages. Project snapshots include up to 10 pinned latest pages as standing context. This list is separate from slots. It is empty and omitted from the output when the project has no pins. Opt-in `settled_first: true` leads with up to 8 of the project's highest-standing rule and decision pages, ordered by evidence count and then recency (a confirmar: the limit of 8). |
| `memory_handoff_begin` | destructive | Opens an owner-scoped handoff for the next agent. `shared=true` publishes it to the whole project. Optional workspace and project target a named sibling. |
| `memory_handoff_list` | read-only | Lists open handoffs visible to the caller, own and shared, with inspectable body and identity fields. It does not claim or expire anything. Root-only `any_owner=true` lists across operators. |
| `memory_handoff_accept` | destructive | Fetches and acknowledges an open handoff. Passing `handoff_id` claims that exact row. Without it, the latest eligible open handoff is claimed. Automatic handoffs are matched on the working directory. Root-only `any_owner=true` recovers across operators. |
| `memory_handoff_cancel` | destructive | Marks one exact visible open handoff as expired, for a handoff created by mistake. Root-only `any_owner=true` recovers across operators. |
| `memory_message_send` | destructive | Sends a directed cross-project message into another project's inbox. Requires the target workspace and project. The recipient must already exist (fail-closed, never created). The body is secret-scrubbed and size-capped. This is the one tool that crosses project isolation on purpose. |
| `memory_message_list` | read-only | Lists pending mail for this project. `box="inbox"` (poppable, the default) or `box="outbox"` (cancellable). Bodies are untrusted cross-project input. |
| `memory_message_pop` | destructive | Claims one inbox message exactly once (the oldest, or a specific `message_id`). Returns it fenced as untrusted input with sender provenance, or `null` when the inbox is empty. |
| `memory_message_cancel` | destructive | Retracts a pending sent message by `message_id`, or clears the whole outbox when no id is given. Scoped to the sender's project. |
| `memory_feedback` | write | Records a quality signal for one page, by exact path. `helpful` and `not_helpful` step the page's salience for sweep-eligible episodic pages. `stale` and `wrong` floor the salience and surface the page as a `feedback_flagged` lint finding. It never deletes anything. The path resolves to the current version inside the transaction, so a later rewrite clears the flag. Retrieved content never authorizes feedback by itself. |
| `memory_write_page` | destructive | Writes durable wiki knowledge when the user explicitly asks to remember or annotate something. `scope: "global"` writes into the reserved `_global` preferences scope. Optional `expires_at` sets an RFC 3339 or date-only TTL. |
| `memory_delete_page` | destructive | Deletes one page by exact path. Runs the admission chain with the delete operation. Idempotent. |
| `memory_forget_sweep` | destructive | Retention pass. Evicts cold pages through the wiki layer, purges aged tombstone ancestry, and hard-deletes TTL-expired pages. `dry_run=true` previews. |
| `memory_lint` | destructive | Rule-based findings, written under `_lint/`. |
| `memory_install_self_routing` | read-only | Returns the canonical slim routing snippet, the managed Agent Skill payloads, and target hints for `CLAUDE.md` and `AGENTS.md` installs. |

The tools `memory_handoff_list` and `memory_handoff_cancel` cover the inspect-
without-claim path for clients that cannot inject SessionStart output, and the
exact-id path for cancellation. `ai-memory handoffs` lists the open handoffs of a
project, oldest first, with their ids. It is read-only and content-free: identity,
provenance, and age, never the summary body. Automatic expiry deliberately spares
manual and sibling-directory handoffs. A long-lived entry in that list is the
policy working as intended.

Some tools were added after the original narrow tool set. Briefing gives the structured half of "what is going on". `memory_write_page`
covers explicit durable annotations without misusing single-use handoffs.
`memory_install_self_routing` exists for the case where the agent must write its
own routing rules into a project's `CLAUDE.md` or `AGENTS.md`, and install the
managed Agent Skills into `.claude/skills` or `.agents/skills`. `memory_read_page`
complements `memory_query` when the agent needs a full page, not a snippet.
`memory_read_session_observations` opens the raw evidence behind a compiled page
or a raw hit, so an agent can audit what the hooks captured.
`memory_delete_page` is the exact-path destructive operation that admission-aware
mirrors need. `memory_handoff_cancel` is the safety valve for mistaken handoffs.
`memory_feedback` adds finer-grained reinforcement than access counts alone. The
access counter cannot tell "this page answered the question" from "this page
wasted a read". The narrow-surface rule still applies: every new tool must earn
its place.

The managed Agent Skills are a narrow prompt-packaging exception to the otherwise
wiki-centered design. They are static `SKILL.md` files that teach agents when to
call the MCP tools. They are not durable wiki pages, not auto-improvement output,
and not a runtime skill router.

MCP parameter aliases are deliberately sparse. `memory_query` accepts `q` or
`search` for its query. Limit fields accept `n` or `top_k` where shipped. Project
and working-directory parameters use their canonical names.

The optional session-aware Claude Code registration is a transport adapter, not a
second tool implementation. The `ai-memory mcp-bridge` command serves the upstream
tool catalogue over local stdio. It forwards tool calls to the configured HTTP
server and adds the inherited Claude Code session id as the
`X-Memory-Actor-Session-Id` header. The server therefore applies the same
authentication, scope resolution, and tool handlers as direct HTTP clients. The
adapter fails closed when no session id is present. It is installed only by the
explicit `install-mcp --client claude-code --session-aware` option.

## HTTP authentication classes

The server separates four active credential classes from one transitional
browser compatibility path:

| Class | Wire | Authorizes |
|---|---|---|
| Human password | `POST /auth/login` body | Session issuance only. |
| Web session | `ai_memory_session` cookie plus CSRF token | `/auth/me`, `/admin/*`, and `/api/v1/*`, by access level. Never `/mcp` or hooks. |
| Recovery | `POST /auth/recovery` body | Root password reset. Issues no session. |
| API key | `Authorization: Bearer` with an `aim_` native key, the root token, or an `amk_` external key | Machine APIs only. Never a web session. |
| Deprecated browser compatibility | HTTP Basic root bearer, then the HttpOnly `ai_memory_auth` cookie | GET-only browser routes, until a human password exists or bootstrap completes. Never machine routes. |

The deprecated Basic and cookie path stops as soon as human authentication
becomes active. A restart is not required. The `/web` single-page HTML is public
static content. The built-in wiki browser and the JSON APIs stay behind the
classes above.

## CLI subcommand surface

Run `ai-memory --help` for the full tree. Grouped by purpose:

```text
Setup and diagnostics   init  status  doctor  show  continue  resume  setup-agent
                        bootstrap  install-instructions  install-skills  install-hooks
                        install-mcp  generate-auth-token  completions  uninstall  upgrade
Runtime                 serve  run  hook  handoffs  message  finalize-session
Workstreams             workstreams  rename-workstream  workstream-search
Wiki                    search  read-page  write-page  delete-page  commit  checkpoints
                        restore-page  reindex  export-okf
Maintenance             forget-sweep  lint  curator  compact  reorg  backfill
                        repair-backfill-timestamps  reclaim-ledger-versions
Data and backup         backup  restore  backup-agents  restore-agents  reset
                        purge-session  purge-project  rename-project  move-project  move-session
                        audit-contamination  list-projects
Access and identity     auth  user  api-key  project  profile  server
```

The `hook-drain` and `mcp-bridge` commands are internal and hidden from the
help output. They are called by the hook and integration machinery.

A few commands carry specific behavior worth knowing:

* `reset`, `restore`, `reindex`, and `uninstall --purge-data` refuse to run
  while the server process holds the data directory (see invariant 7).
* `compact` reclaims free database pages through a vacuum and a full-text
  rebuild. It deletes no data.
* `reindex` rebuilds the database from the wiki directory. Run it with the server
  stopped and a clean data directory.
* `purge-project` is irreversible and requires `--confirm`. `uninstall` is a
  dry run without `--apply`.

## Cross-cutting invariants

These constraints apply to every change in the affected areas. Each one was
introduced to prevent a specific failure seen in comparable systems.

1. **One configuration read.** Configuration is loaded once at startup. No other
   code reads environment variables directly.
2. **Single writer.** All writes go through one queue to one dedicated writer
   thread. Reads go through a separate read-only pool.
3. **Indexes commit with the data.** The index for a write is updated in the same
   transaction as the data. No background indexing after the write returns.
4. **Typed identity in every row.** Each domain row carries the workspace id, the
   project id, and the path from the first day.
5. **Hooks are fire-and-forget.** Hook scripts time out at 200 ms or less. The
   server answers 202 immediately, or 429 when saturated. Hooks never wait for
   work to finish.
6. **Privacy strip is a typed boundary.** A sanitized observation can only be
   created by the sanitizer. The opt-in assistant excerpt enters through the same
   boundary. The client sanitizes it, and the server scrubs it again with its
   configured patterns before the write.
7. **Live-process check before direct disk operations.** `reset`, `restore`,
   `reindex`, and `uninstall --purge-data` check whether the server process is
   running, and refuse if it is. The uninstall check applies only with
   `--purge-data`. `backup` is a thin HTTP client instead. The server snapshots
   the database through the online backup mechanism while the writer stays live.
8. **Atomic file writes.** Write to a temporary file, flush it to disk, and rename
    it over the destination. The file watcher ignores the server's own writes by
    filename prefix.
9. **Absolute, canonical data directory.** The default data directory is absolute
    and logged loudly at startup.
10. **No global mutable configuration.** Every dependency is passed explicitly.
11. **Zero-LLM.** The product has no language-model dependency.
12. **Log subscribers filter their own output.** Logging does not feed back into
    itself.

## Configuration (`config.toml`)

The file lives at `<data_dir>/config.toml`. Every value can be overridden by an
environment variable with the `AI_MEMORY_` prefix. The block below shows the
shipped defaults.

```toml
bind = "127.0.0.1:49374"
log_level = "info"                 # the default filter also pins the MCP protocol logs to warn
                                   # and moves the 30 s reconcile summary to debug.
                                   # Override with log_level or RUST_LOG.
tcp_keepalive_secs = 60            # idle time before TCP keepalive probes an accepted serve
                                   # connection. Reaps sockets left half-open by a dead peer.
                                   # 0 disables keepalive. Env: AI_MEMORY_TCP_KEEPALIVE_SECS

# Capture and launch behavior. Each has an AI_MEMORY_* environment override
# (AI_MEMORY_CAPTURE_ASSISTANT, AI_MEMORY_BACKFILL_ON_START, AI_MEMORY_RUN_AUTOWIRE).
capture_assistant = false          # server-side opt-in: accept a Claude Code or Codex client's
                                   # sanitized assistant final-message marker on Stop. The client
                                   # half is enabled separately by install-hooks --capture-assistant.
backfill_on_start = true           # on the first SessionStart in an empty project, import that
                                   # project's existing local harness history once. Only bootstraps
                                   # an empty project and is hard-capped. `ai-memory backfill` runs it by hand.
run_autowire = true                # `ai-memory run <harness>` installs that harness's hooks and MCP
                                   # on first launch if missing. Idempotent, one time per harness and
                                   # version. Also available as --no-autowire.

[decay]                            # retention parameters
lambda = 0.02                      # decay rate (scalar, used when a tier has no value)
sigma = 0.6                        # weight of the access term
mu = 0.04                          # decay rate of the access term
cold_threshold = 0.20              # below this, the page is evicted and a tombstone is kept
hard_delete_after_days = 180       # tombstone age before purge
breadth_weight = 0.0               # opt-in reward for distinct operators (0 = off)
observation_retention_days = 0     # 0 = never prune raw observations
observation_prune_batch = 5000     # rows per prune transaction
compact_cold_episodic = false      # opt-in: compact a cold episodic page in place instead of
                                   # evicting it. Zero-LLM and reversible.

[decay.half_life_days]             # opt-in per-tier decay curves, all keys optional. Half-life in
                                   # days, converted to lambda = ln(2) / days. An omitted key uses
                                   # the scalar lambda, so the default is unchanged.
# working = 7
# episodic = 365
# semantic = 180
# procedural = 90

[slots]
per_user = false                   # opt-in: a bounded per-user namespace for slot writes on shared servers

[retrieval]                        # opt-in ranking signals, all off by default
query_intent = false               # (default a confirmar) lexical session-recall routing for queries phrased as
                                   # "last time" and similar. Hands session pages a reduced
                                   # default kind and tier penalty.
session_recall_bonus = 0.25        # extra authority on top of the cancelled penalty
belief_authority_weight = 0.0      # folds the belief-strength confidence into page authority as one
                                   # bounded factor inside the [0.55, 1.50] clamp. 0.0 = off.

```


## Future work

* **Richer curator actions.** The shipped curator stages one report page. Future
  work can add individual merge, supersession, and link-fix proposals, while
  deletes and semantic rewrites stay behind review.
* **Richer read surfaces for the web interface.** The multi-workspace, read-only
  wiki browser (project list, page tree, page view, search) is shipped. It stays
  read-only by design. The wiki is a machine-authored record, and a browser edit
  surface would break the invariant the store depends on. Better reading (navigation,
  diff and history views, graph exploration) is still open. See
  [Frontend API](frontend-api.md#10-known-gaps-and-deliberate-non-goals).

## Reading order

* This file: the operational summary.
* [Database schema](database-schema.md): consolidated SQLite schema.
* [API reference](api-reference.md): MCP tools, `/admin/*`, and `/auth/*` wire contracts.
* [Overview](overview.md): components, invariants, dependency graph, and the
  suggested reimplementation order.
* [Design decisions](design-decisions.md): the full v1 specification and its
  historical rationale.
