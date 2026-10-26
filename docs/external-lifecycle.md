# External lifecycle capture

An orchestrator that runs and observes agents can send their lifecycle events to a
shared ai-memory server instead of letting each agent's installed hooks capture them.
This document describes that path: how capture ownership is activated, how a producer
checks capture policy before queueing, how events are identified for safe retries, how
batches are acknowledged, and what limits and failure behavior apply to the native
spool and drain.

The server keeps owning storage, retrieval, consolidation and durable memory. Producers
send events; they do not write memory directly.

Related documents:

- [lifecycle-event-ingestion.md](lifecycle-event-ingestion.md): event normalization,
  field extraction, session rules and the per-item acknowledgement rules of batches.
  Those rules are not repeated here.
- [programmatic-memory.md](programmatic-memory.md): deliberate writes, queries and
  handoffs without lifecycle capture.
- [cli.md](cli.md): the hook and install commands named below.
- [companions.md](companions.md): the optional lifecycle relay that queues and
  delivers events for a producer.

## 1. Choosing a capture owner before launch

### 1.1 Activation

Set `AI_MEMORY_CAPTURE_OWNER` in the environment of the harness the orchestrator
launches, to the producer namespace (for example `example.runtime`).

- Any value that is not empty after trimming whitespace opts the process, and its
  inheriting child processes, into external capture.
- Unset, empty and whitespace-only values keep normal native capture.
- The variable is inherited process context. It is not server configuration, a
  security identity or a lease. Do not place it in the shared server's environment or
  in a global shell configuration.
- Standalone launches without the variable capture normally.
- A runtime that hosts several agents in one process applies the variable to every
  installed ai-memory integration in that process. If it needs finer ownership, it must
  manage that itself.
- Choose the owner before the execution's first event. Switching midway cannot
  deduplicate events already produced by independent paths.

### 1.2 Effect by responsibility

| Responsibility | With external capture |
|---|---|
| Native lifecycle observations | Dropped before spool, queue or network send. |
| Native drains of the spool, and the one-time boot backfill at session start | Not run. |
| Handoff, briefing and inbox-notice delivery through hooks | Preserved. |
| Native session identity used by handoff and session-aware tools | Preserved. |
| Deliberate memory tools (recall, writes) | Unchanged. |
| Repository capture policy | The producer checks it and applies it before queueing each event (section 3). |

Installed hooks that deliver context still run. The variable suppresses capture only.
Context delivery can register the receiving session and claim a pending handoff, so
suppressing capture does not make that path read-only.

Context delivery is per harness:

- Claude Code receives context through its session-start hook.
- Kimi Code receives context through its prompt-submit hook, because it discards
  session-start output.
- Grok receives the handoff on the first post-tool event, because it discards output of
  its session-start and prompt-submit hooks.
- Harnesses without automatic injection can call the `memory_handoff_accept` tool
  through the MCP server. See [mcp-server.md](mcp-server.md).

If the hook owns delivery for a harness, the orchestrator must not also claim that
handoff.

### 1.3 Integrations that honor the variable

Native hook commands, the POSIX and PowerShell hook scripts, and generated integrations
for OpenCode, OpenCode 2, OMP, Pi and OpenClaw honor the variable. Integrations generated
before this behavior existed do not. Reapply the hook installation after an upgrade to
refresh staged scripts and generated code. See
[support-matrix.md](support-matrix.md).

### 1.4 Diagnostics

`ai-memory hook --check-capture` prints one JSON object and performs no ingestion and no
handoff delivery. When the variable is active, it reports `external_capture: true` and
`admits_capture: false`. The owner value is not printed or authenticated.

`ai-memory doctor` reports when the variable is active, and shows the authenticated
machine identity of the server. Its per-agent session counts include a
`mixed_capture_sessions` figure: the number of sessions in the scope that have more than
one capture namespace, where native events and each distinct extension count separately.
A mixed count is a diagnostic signal, not proof of duplicates. A server too old to report
this leaves those fields unknown.

## 2. Reusing the public event contract

- For a live harness, send its wire agent name (for example `claude-code` or `codex`)
  and its native session ID. Use the same session ID, working directory, scope and
  authenticated operator as the harness's remaining native integration.
- The session ID in the body takes precedence over a session ID in the query string.
- Replacing only the ingestion ID with a logical agent ID from the orchestrator separates
  the events from native handoff and MCP identity. Avoid this.
- Send both `workspace` and `project` explicitly on every event. A partial scope fails
  the preflight in section 3.
- Preserve canonical lifecycle names: `session-start`, `user-prompt-submit`,
  `pre-tool-use`, `post-tool-use`, `pre-compact`, `post-compaction`, `notification`,
  `stop`, `session-end`, `subagent-start`, `subagent-stop`. The full alias list and the
  mapping to observation kinds are in [lifecycle-event-ingestion.md](lifecycle-event-ingestion.md),
  section 3.1.
- Keep the supported harness payload shape in the body. Field extraction and the
  rules for tool and prompt events are in [lifecycle-event-ingestion.md](lifecycle-event-ingestion.md),
  sections 3.3 to 3.5.
- `session-end` means the execution ended. It does not mean that a turn completed. Use
  `stop` for a turn boundary.
- A session is substantive only if it logged at least one user prompt, pre-tool or
  post-tool observation. Sessions made only of other events, including stop and
  notification events, get no session page and no automatic handoff at end. See
  [lifecycle-event-ingestion.md](lifecycle-event-ingestion.md), section 8.3.

### 2.1 Provenance fields

Set `extension=<producer-namespace>` and an explicit `source_event=<event-name>` on each
event. Both are needed to keep the namespace on recognized events. If `source_event` is
missing or invalid, the server drops `extension` as well. For an unrecognized event name
without `extension`, no source event is kept.

| Field | Limit | Allowed characters |
|---|---|---|
| `extension` | up to 64 characters | letters, digits, `.`, `_`, `-`, `:` |
| `source_event` | up to 128 characters | letters, digits, `.`, `_`, `-`, `:` |

Surrounding whitespace is trimmed before the check. These fields describe the source.
They do not add to the core event vocabulary, and they do not retain arbitrary metadata.

### 2.2 Fields the producer must not reuse

- Do not repurpose the native `agent_id` body field. Some harnesses use it as a subagent
  marker, and the server then drops the event as a subagent capture.
- Keep logical agents, parent and child relationships and workflow IDs in adapter-side
  state.
- The `managed_run` query field belongs to an ai-memory run invocation. It has its own
  lease and handoff behavior, so do not use it as a generic workflow ID.

## 3. Preflight for external producers

Run the preflight before queueing each event. It is the output of
`ai-memory hook --check-capture`. Its `policy_admits_capture` field ignores capture
ownership, because the producer is the one that queues the event.

`policy_admits_capture` is `true` only when all of these hold:

- the repository capture policy admits the event (capture mode and marker);
- the marker's routing selection is valid;
- the event is eligible for capture under the event rules;
- the server profile selected by the marker resolves. A rejected profile returns `false`,
  and the event must not be sent to the install default instead;
- the scope is neither partial nor unavailable;
- the event's disposition is not `drop`.

The preflight output also contains:

- `disposition`: `keep`, `metadata-only` or `drop`. Apply it before queueing. A
  `metadata-only` event keeps only routing fields (session ID, working directory),
  the tool family and name, the tool call ID when present, and the capture protocol
  record. Tool inputs and outputs are removed.
- `scope`: sanitized local routing hints (workspace, project, project source, project
  strategy, identity, identity source, identity style, aliases). Each hint is at most
  512 bytes. A longer hint is omitted, which makes the scope unavailable, and the
  preflight then returns `false`. Native routing values are not affected by this bound.
- `scope_resolution`: `explicit`, `partial`, `server-derived` or `unavailable`.
- `server_may_remap`: `true` when the server may choose different coordinates than the
  producer sent. It is `true` when the project did not come from a marker, or when an
  identity hint is present.

## 4. Producer payload requirements for the relay

The optional lifecycle relay in [companions.md](companions.md) applies these checks
before queueing. A producer that uses its own queue should apply the same checks.

- The body is a JSON object with a string `session_id` and a string `cwd`. Both must be
  printable, with no control characters. The session ID is limited to 256 characters
  and the working directory to 4096 characters.
- The serialized body is at most 256 KiB.
- Producer and actor namespaces are token strings of 1 to 64 characters (letters, digits,
  `.`, `_`, `-`, `:`).
- The event name and event ID are at most 128 characters each. The agent name is at most
  64 characters. Workspace and project names are printable and at most 128 characters.
- A batch carries at most 256 events.

## 5. Stable retry identity

### 5.1 The ingest key

`ingest_key` is a project-scoped key that the server stores with the observation. It is
independent of `extension`, and it is not derived from the payload.

- Construct the key once, from stable event identity, and persist it with the event
  before the first delivery attempt. Keep it the same across restarts.
- Never derive equality of events from prompt or tool content.
- Accepted keys are 1 to 64 ASCII letters, digits, `-` or `_`. Anything else is ignored,
  not rejected. The event then goes through as an unkeyed, at-least-once delivery. Validate
  keys before sending, because a malformed key silently loses its retry protection.
- Reusing a raw event ID as the key under two extensions does not namespace it, because
  the key scope is the project, not the extension. Use the derivation below instead.

### 5.2 Derivation recipe

Build the key from a tuple of seven strings, in this order:

1. the fixed string `external-capture-v1`;
2. the producer namespace;
3. the adapter's operator namespace (a stable identifier, never a bearer token);
4. the wire agent name;
5. the native session ID;
6. the source event name;
7. the event ID.

Serialize the tuple as a JSON array of strings, with no extra whitespace, encoded as
UTF-8. Compute the SHA-256 digest of those bytes and write it as 64 lowercase hexadecimal
characters. The result is 64 characters, so it passes the key rule in section 5.1.

Include the coordinates within which the event ID is unique. A producer whose event IDs
are unique across all its events may use a shorter tuple, but then it must keep the
same tuple for the life of the producer, because the key changes with the tuple.

Keep the encoding and values stable across process restarts and implementations.

### 5.3 Replay semantics and retention

- The same producer and event identity always yields the same key, so retries are safe.
- Distinct event IDs, or distinct producer namespaces, yield distinct keys even when the
  payloads are equal.
- The server claims the key and the observation in one transaction. Either both are
  committed or neither is. A replay of a completed key is acknowledged without inserting
  a second observation (outcome `replayed`). A replay of a claimed but incomplete key
  resumes the downstream effects without inserting an observation (outcome `resumed`).
- A key is removed once it is more than 30 days old, measured from its first claim. The
  removal runs during keyed writes, so an old key can remain until the next such write.
  Replays after 30 days are treated as new deliveries.
- The mechanism bounds retries. It is not permanent deduplication, and it is not an
  exactly-once guarantee for every downstream side effect.

## 6. Batches and acknowledgements

### 6.1 Request format

A batch is a JSON array. Each item has a `url` and a `body`. The `url` carries the
event's query string, and the server does not fetch it. Send the array to `POST /hook/batch`
with the normal bearer authentication.

```json
[
  {
    "url": "/hook?event=user-prompt-submit&agent=claude-code&workspace=team&project=app&extension=example.runtime&source_event=user-prompt-submit&ingest_key=example-runtime-run-17-prompt-4",
    "body": {
      "session_id": "native-session-17",
      "cwd": "/work/app",
      "prompt": "Continue the parser fix."
    }
  }
]
```

The short key in the example assumes that the runtime assigns run IDs unique across the
producer. Otherwise use the derivation in section 5.2.

### 6.2 Limits

- At most 256 items per batch. A larger batch is rejected before any item is processed,
  with status 413 and an acknowledgement that covers zero items.
- The request body is limited to 10 MiB by the server. Keep batches well below that.
- All items in one batch share one authenticated identity. Separate batches by operator
  when representing several users.

### 6.3 Processing order and independence

- Items are processed inline, in array order, within the request.
- There is no ordering across concurrent requests.
- There is no all-or-nothing transaction. Each item is committed or acknowledged on its
  own.
- A rate-limited source can be skipped while later items from other sources are
  processed. The skipped item is not acknowledged.
- A processing failure stops the batch at that item. Later items are not processed.

### 6.4 Acknowledgement fields

| Field | Meaning |
|---|---|
| `accepted_indices` | Indices acknowledged. Prefer it when present. |
| `accepted` | Length of the contiguous acknowledged prefix. Used when `accepted_indices` is absent. |
| `results` | One `{index, outcome}` entry for each acknowledged index, in index order. An empty array means nothing was acknowledged. |
| `failed_index` | The item whose processing failed, when there was a failure. |

Outcome values:

| Outcome | Meaning |
|---|---|
| `stored` | A new observation was stored. |
| `replayed` | A completed keyed delivery was repeated. |
| `resumed` | Downstream effects of a claimed, incomplete delivery were retried, or an end for an already ended session recovered a wiki commit or a new consolidation job. |
| `ignored_end` | A missing or scope-invalid end, or an already ended delivery with no recovered effect. |
| `dropped_policy` | Capture policy discarded the event. |
| `dropped_subagent` | The event is a subagent capture, and subagent capture is disabled. |
| `dropped_unauthorized` | The caller lacks write access to the project. |
| `dropped_collision` | The session identity or ownership conflicts with an existing session. |
| `dropped_invalid` | The event can never be stored: no session ID outside a session start, an invalid alias marker, or a cursor draft placeholder. |

A dropped outcome is a terminal acknowledgement. The producer must remove the item and
must not retry it. An acknowledgement can therefore mean a deliberate drop, not a new
observation.

Older servers may omit `results`. The relay records such receipts as `unknown`.

Rules for interpreting a response are in [lifecycle-event-ingestion.md](lifecycle-event-ingestion.md),
section 2.2.

### 6.5 Producer retry rules

- Inspect acknowledgements on every response, including HTTP 429 and a processing
  failure. A 429 response still acknowledges the items before the saturation point.
- Remove acknowledged items. Keep every unacknowledged item for retry.
- A processing failure is reported as success with `failed_index`. Earlier items are
  acknowledged, and the item at `failed_index` and the items after it must be retried.
- On a timeout or a lost response, retry the unchanged items with their unchanged keys.
- On saturation (429), back off. Do not open an unbounded number of requests.
- Use `POST /hook` only when the producer does not need an inline acknowledgement. That
  endpoint answers 202 before the event is persisted.

## 7. Durable queue, ordering and recovery

- Keep a durable producer-side queue when recovery after a restart matters.
- The lifecycle relay sends at most one pending event per session in each batch. Two
  events of one session are never in flight together, so a partial acknowledgement
  cannot reorder them.
- Flush earlier events of a session before sending its terminal event.
- Serialize delivery within a session wherever order matters. Different sessions may
  share the server concurrently.
- Late observations after a session has ended, and a later terminal event, follow the
  existing rules for ended sessions. See [lifecycle-event-ingestion.md](lifecycle-event-ingestion.md),
  section 8.3.
- Stopping a parent does not make the server end its child sessions. The producer must
  report each child's lifecycle transition. (Confirmed in audit 002: the server never
  infers a child ending from its parent. The only child-specific rule is that an OpenCode
  child session, marked as subagent in its own `session-end`, is closed and summarized
  **without** an automatic handoff; see lifecycle-event-ingestion.md §8.3.)

## 8. Native spool and drain limits

These limits apply to events that the native hooks spool, including events spooled before
external capture was activated. Capture ownership does not change them.

| Item | Value and behavior |
|---|---|
| Spool file cap per data directory | 10,000 files (entries plus files in the recovery quarantine, which belongs to managed-run recovery). When the cap is reached, enqueue **keeps the existing backlog and drops the new event**, printing `ai-memory: hook-spool at capacity (10000); dropping one new UNDELIVERED event` (no error; the hook still succeeds). The cap is soft: a concurrent enqueuer that holds the capacity lock past 50 ms is not waited for, so a burst can overshoot by a few entries. |
| Entry expiry by age | **7 days.** An entry older than 7 days (from its creation time) is deleted at load time and counted as dropped, whatever its attempt count, so a long-dead instance cannot grow the spool without bound. |
| Entry size | A spool file larger than 10 MiB is refused when read (it stays counted as remaining, it is not delivered). An entry whose JSON cannot be parsed is deleted and counted as dropped. |
| Directory | The spool directory is private (`0700` on Unix); entries are written to a temporary file and renamed, `0600`. File names are `<created_ms padded to 13 digits>-<pid>-<16-hex sequence>-<recovery key>.json`. |
| Attempts before an entry is dropped | 8 by default, set by the hook spool configuration. A value of 0 disables dropping by attempts. Attempts are counted only when a server was reached and the delivery failed. An endpoint-level outage does not count. |
| Batch size | 256 items per request. |
| Batch byte budget | 8 MiB soft limit. A chunk always carries at least one event, even if that event alone is larger. |
| Per-request drain timeout | 3 s by default. |
| Session-start drain budget | 3 s by default. The session-start hook drains the backlog within this budget, then fetches the handoff. |
| Handoff fetch timeout | 3 s by default. |
| Background drain budget | 5 min by default. A detached drainer starts after session end, stop and pre-compact events. |
| Mid-session catch-up | Only on `post-tool-use`, and only when the backlog is at least 32 events. Budget 250 ms. |

Environment overrides for the timeouts and budgets (`AI_MEMORY_HOOK_DRAIN_TIMEOUT_MINUTES`,
`AI_MEMORY_HOOK_HANDOFF_TIMEOUT_MINUTES`, `AI_MEMORY_HOOK_START_BUDGET_MINUTES`,
`AI_MEMORY_HOOK_BACKGROUND_DRAIN_BUDGET_MINUTES`) are whole minutes. Values above 60 are
capped at 60. Zero, negative and non-numeric values use the default. The smallest value
that can be set is therefore one minute, not the three-second default. The incremental
threshold override (`AI_MEMORY_HOOK_INCREMENTAL_THRESHOLD`) is a positive integer with no
upper bound.

Failure behavior of the native drain:

- A delivery that fails leaves the entry in the spool, and its attempt count increases.
- When the attempt count reaches the limit, the entry is removed. Any entry older than 7 days is
  removed first (see the table).
- If a spool write fails at capture time, the warning is printed and that event is
  skipped. No retry is scheduled for it.
- A session-start drain first waits for the drain lock, up to its budget. If another
  drainer holds the lock for that whole time, the session-start drain is skipped. Otherwise
  it drains until the budget is spent, and the remaining backlog waits for the next drain.

## 9. Boundaries

- The variable does not erase or cancel existing spool entries. The explicit drain
  command and the other standalone hooks still deliver that backlog.
- Generated integrations keep their existing queue disposal behavior, which can drain
  events captured earlier.
- Manual backfill, the importer, session finalization and managed transcript import
  through the run command keep their existing behavior. Do not combine them with an
  external producer for the same execution.
- External capture does not disable native harness transcript files or other vendors'
  hooks.
- Authentication, owner checks, admission rules and sanitization still apply. The
  variable grants no permissions. `extension` is self-declared provenance, not an
  identity.
- Project access modes and grants apply to database-user tokens. See [users.md](users.md).
  Pages are shared by authorized project members. (Confirmed in audit 002 for the
  ingestion path: the per-repository check only runs for a database-user viewer, and only
  when per-repository authorization is on; root and single-operator installs are not
  checked. A capture whose author may not write the project is acknowledged and dropped
  (`dropped_unauthorized` in a batch), never retried. `GET /recap` answers a project the
  viewer cannot read exactly like an unknown session.)
- This document does not introduce a framework adapter, a workflow engine or a generic
  plugin runtime. A future adapter for an observability standard can map conversation
  and tool-call identities at the boundary. Keep the ingestion producer identity separate
  from the model provider, and preserve the native session identity that the existing
  integrations need.
