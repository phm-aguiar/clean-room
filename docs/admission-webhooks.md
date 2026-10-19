# Admission webhooks: pre-persistence HTTP hooks

> **Escopo:** este documento contém partes que dependem de LLM ou de embeddings. Essas partes são opcionais e não fazem parte do escopo inicial, que funciona sem LLM. Podem ser removidas ou adiadas junto com as funções correspondentes.

> Operator-configured HTTP hooks that the engine calls around its write and
> lifecycle operations. Blocking hooks run just before a page write commits,
> and may change the page or refuse the operation. Notification hooks run
> around delete, purge, move, handoff and message operations, and may observe,
> mirror or refuse them. Non-blocking hooks only observe, after the change has
> committed.

## 1. What this is (and isn't)

| | What you can do | What you can't do |
|---|---|---|
| Chain | Add canonical frontmatter fields (for example `contributors`); mirror a change into an external system (git, search index, audit log); refuse operations that fail a policy (for example `validate-no-secrets`). | Talk back to the engine's writer or store directly. A hook sees one operation at a time and may only change the page it is given. |
| Engine | Stays closed for modification: each new behaviour ships as an independent HTTP service, in any language. | The engine does not discover or register hooks. Operators name them in configuration. |

If an idea does not fit "change the page, or observe or refuse an operation", it
probably belongs to another extension point: the hook ingress for lifecycle
events ([lifecycle-event-ingestion.md](lifecycle-event-ingestion.md)), the
administrative HTTP surface, or an out-of-band scheduled job.

## 2. Lifecycle

### 2.1 Page writes

A page write runs the following steps, in this order:

1. The caller's body is scrubbed for secrets.
2. If the caller has an identity, a `last_modified_by` block is stamped into
   the frontmatter. The hooks therefore see it.
3. The workspace and project names are resolved (see section 5).
4. Blocking hooks run in configured order. Each one receives the page as left
   by the previous hook.
5. The hook output is scrubbed again: the body and every string value in the
   frontmatter. A hook cannot reintroduce a secret that the caller's body did
   not contain.
6. The frontmatter is normalised for the on-disk format, in this order:
   `tier` is forced to the write's tier and `pinned: true` is written when the
   page is pinned (a hook that sets `pinned: true` pins the page; slot paths are
   always pinned); then OKF conformance is applied and `generated.at` set or
   inherited (see [okf.md](okf.md)). The title and links are then derived again
   from the final content; an explicit title from the caller (scrubbed) still
   overrides the derived title. The final frontmatter is then validated: an
   `expires_at` that is neither RFC 3339 nor `YYYY-MM-DD` aborts the write with
   `invalid expires_at in frontmatter for <path> (want RFC3339 or YYYY-MM-DD): <value>`,
   and an `entities` value that is neither absent, null nor a list aborts it with
   `invalid non-array entities in frontmatter for <path>`. These errors can come
   from a hook's output, and nothing has been written at that point.
7. Under the per-path write lock, the Markdown file is replaced (keeping a
   rollback snapshot) and the store row is written. If the store write fails,
   or an optional precondition on the current latest id no longer holds, the
   file is restored to its previous content. If that restore also fails, the
   error is `INCONSISTENT STATE: wiki files changed but store write failed
   (<cause>) and rollback failed (<rollback error>)`.
8. The page is embedded when an embedder is configured. An embedding failure
   is logged, recorded against the page, and does not fail the write.
9. Non-blocking hooks are dispatched, fire-and-forget, with the final page.
   They never mutate or refuse it.

The file and the store row are therefore updated together, as one logical
step. The detailed write invariants are in [architecture.md](architecture.md).

### 2.2 Operation types

Every hook call carries an `op` value. A hook receives only the operations it
subscribes to (see section 8). There are 15 values (the table lists them in the order of the original enumeration; configuration accepts exactly these snake_case names, and an unknown name makes the configuration fail to load):

| `op` | Raised by | Page path | Gate | Notes |
|---|---|---|---|---|
| `write_page` | Direct page writes: the MCP write tool, the administrative write route, approval of a pending write, and the automatic improvement write. The default when no operation type is given. | Yes | Blocking hooks run first and may mutate the page. | Non-blocking hooks run after the commit with the final page. |
| `consolidate` | LLM consolidation writes and the lint report. A rule-based fallback write, used after the LLM provider fails, also uses this op. Lint report writes and deletes use this op too. | Yes | See "Consolidation preflight" below. | Fires up to twice per consolidation (see below). |
| `delete` | Single-page delete (MCP delete tool), decay eviction, frontmatter TTL expiry, and the optional reconcile path that tombstones a page whose file has disappeared. | Yes, no body | Blocking hooks run before the file is removed. | The delete hook is called before the file is removed, so a mirror can remove the same path. |
| `delete_event_ledger` | Deletion of a rotated hook event ledger file (`log-YYYY-MM.md`) by the opt-in ledger retention of the forget sweep. | Yes: the ledger **file name** (no directory), no body | Blocking hooks run before the file is removed (`notify` gate); a `reject` failure leaves the file and counts as `ledgers_undeleted`. | Non-blocking hooks are dispatched after the file is gone, with a null frontmatter and an empty body. No store row is touched. |
| `purge_project` | Whole-project purge, and the source teardown of a copy-purge project move. | No | Blocking hooks run before the purge. | See `partial_failure` in section 3. |
| `purge_session` | Single-session purge. | No | Blocking hooks run before any row is deleted. | The session is identified by its id, not by a path. |
| `purge_workspace` | Whole-workspace purge. | No | Blocking hooks run before any SQL or file destruction. | `ctx.project` is empty. |
| `move_project` | Project move between workspaces, without changing the project id. | No | Blocking hooks run before the directory rename and the store re-stamp. | Source names in `ctx.workspace` and `ctx.project`. Destination names in the destination fields. |
| `move_session` | Single-session move to another project: the session's rows and its session page. | No | Blocking hooks run before the page file moves and the store re-stamps. | Same name fields as `move_project`. |
| `handoff_begin` | A handoff is created: by the MCP tool, or at SessionEnd. | No | Decide, act, notify (section 2.4). | Carries scope and the acting operator. No page body. |
| `handoff_accept` | A pending handoff is consumed: by the MCP tool, or at session start. | No | Decide, act, notify (section 2.4). | Not announced if nothing was pending. |
| `handoff_cancel` | A pending handoff is discarded, by the MCP tool. | No | Decide, act, notify (section 2.4). | Not announced if the cancel is refused. |
| `message_send` | A cross-project message is sent into a recipient project's inbox. | No | Decide, act, notify (section 2.4). | Decided at the recipient scope. The message body is not sent. |
| `message_pop` | A cross-project message is taken from an inbox. | No | Decide, act, notify (section 2.4). | Observers are notified only when a message is actually returned. |
| `message_cancel` | A sender cancels its pending cross-project messages. | No | Decide, act, notify (section 2.4). | Same as the other message ops. |

Notes on the table:

- Page-path operations deliver the page path to the hook. Operations with no
  page deliver an empty path (see section 3).
- The MCP tools for handoffs and messages are described in
  [mcp-server.md](mcp-server.md). Messages are described in
  [agent-messaging.md](agent-messaging.md).

### 2.3 Sequencing for page writes and notifications

- Blocking hooks run **one after another**, in configured order. Each one sees
  the page as left by the previous one. A frontmatter hook that adds a field
  can therefore be followed by a mirror hook that copies the enriched page.
- Every blocking hook is awaited, whatever its failure policy. Each await is
  bounded by that hook's timeout.
- Notification operations (delete, purge, move) follow the same rule. A
  blocking hook on these ops adds its latency to the operation, even when its
  failure policy is `ignore`.
- A `reject` failure on a blocking hook aborts the operation. The chain stops
  at the first refusal.
- Non-blocking hooks are never awaited. They run after the operation has
  succeeded.

### 2.4 Handoff and message operations: decide, act, notify

Handoff and message operations use a fixed three-step order. Every path that
raises one of these ops follows it, so a hook sees the same event whether the
operation came from an MCP tool or from a hook path.

1. **Decide.** Only hooks that can refuse are awaited. A hook can refuse only
   if it is both `blocking = true` and `failure_policy = "reject"`. These hooks
   run one after another and stop at the first refusal.
2. **Act.** The operation runs only if no decider refused it.
3. **Notify.** Every other subscribed hook is dispatched fire-and-forget,
   after the operation, and only if the operation happened. This includes
   blocking hooks with `failure_policy = "ignore"`. Those hooks cannot refuse
   the operation, so they are treated as observers here.

The consequences are:

- No observer is told about an accept that found no pending handoff, or about
  a cancel that another operator's ownership refused.
- An accept with nothing pending is not announced to a decider. The engine
  checks for a pending handoff before it asks.

What a refusal costs the caller depends on which path raised the op:

- **MCP tools** (`memory_handoff_begin`, `memory_handoff_accept`,
  `memory_handoff_cancel`, and the message tools). A refusal aborts the tool
  call and is returned to the caller as an error. The caller asked for the
  operation, so it is told the operation did not happen.
- **SessionEnd** (automatic handoff begin). The hook asks before the session
  end is committed. A refusal skips the baton and logs the event. The session
  page, opt-in consolidation and auto-commit still run. The session ends
  without a handoff, not without a summary.
- **Session-start claim** (automatic handoff accept). The deciding chain is
  capped at 750 ms (section 7). A refusal, the 750 ms deadline, a per-hook
  timeout, or an unreachable host leaves the handoff **open** for the next
  session. The session start is never failed, and no context is consumed after
  the caller has disconnected.

Budgeting the session-start claim: deciders run one after another, and the
chain stops at the first refusal. The worst case is therefore the **sum** of
the `timeout_ms` values of every `reject`-policy blocking hook subscribed to
`handoff_accept`. The 750 ms deadline caps that sum. A chain that cannot approve
within 750 ms is treated as a refusal. The per-hook default timeout of 2000 ms
still applies to MCP tools and write paths.

If the aggregate deadline is firing before individual hooks time out, lower the
per-hook `timeout_ms` values. This lets the logs identify which decider timed
out.

### 2.5 Consolidation preflight

Consolidation runs a preflight before the LLM call:

- The preflight uses the same blocking gate as a notification. Each blocking
  hook subscribed to `consolidate` is awaited, and a `reject` failure aborts
  the consolidation.
- The preflight is sent with an **empty body**, and with the target path. For a
  multi-page run, the target path is the canonical session anchor,
  `sessions/<session-id>.md`.
- Any mutation returned during the preflight is discarded.
- Decisions should be based on `op`, `actor`, `workspace`, `project` and path.
  A hook should not reject a call only because the body is empty.
- A dry run stops after the preflight. It does not call the LLM.

The write-time check runs again with the real content, so `consolidate` fires
up to twice per consolidation. Blocking calls are **decisions, not delivery
events**. Do not count them as "one write happened". For that, use a
non-blocking observer.

### 2.6 Operations that do not fire hooks (by design)

- **Log appends** (`log.md` and monthly `log-YYYY-MM.md` files). These are
  written for every hook event. Routing each one through the chain would mean
  one HTTP request per observation. The log is a local audit artifact. Back it
  up out of band, in batches.
- **Handoff contents.** Handoff ops carry scope and the acting operator only.
  A handoff is a store row and has no file to mirror.
- **Aged decay-history cleanup.** Only the old version chain in the store is
  removed. A Markdown file at that path remains authoritative. It is reindexed
  first when needed. Initial decay eviction and TTL expiry do use the
  conditional `delete` path.
- **Project and workspace renames.** A rename updates a name in the store. The
  on-disk path is a stable identifier, so no file moves and nothing needs to be
  propagated.
- **External edits on disk.** These are reconciled by the file watcher, not by
  the chain. The chain covers the engine's own write path only.
- **Preview and dry-run purges.** The administrative preview of a purge does
  not run the chain. A preview's success is not a promise that the confirmed
  purge will be allowed by a refusing hook.

### 2.7 Missing-file tombstones

The optional reconcile path can tombstone a page whose file has disappeared. It
does not run the blocking gate, so nothing can refuse it. After the tombstone is
written, the non-blocking hooks for `delete` are dispatched. A mirror is
therefore told about the tombstone.

### 2.8 Purges and moves: failure flags

- **Copy-purge project move.** One request produces two kinds of hook call. It
  first produces one `write_page` call for each copied page, as the pages are
  written into the destination. Then it produces one terminal `purge_project`
  call, when the source is torn down.
- The copy leg skips the hook named exactly `contributors`. Copied pages keep
  their frontmatter verbatim, including any existing `contributors` list.
  Running that enrichment on every copied page would add latency without
  changing the result. Other `write_page` hooks, such as a git mirror, run
  normally for each copied page.
- The terminal `purge_project` call is not affected by the skip.

## 3. Wire contract

### 3.1 Request (engine to hook)

```http
POST <hook url>
Content-Type: application/json
X-Memory-Op: <op>
```

`<op>` is one of the 15 values in section 2.2. The header always matches
`ctx.op`.

Body:

```jsonc
{
  "page": {
    "path": "gotchas/example.md",          // relative page path; "" when the op has no page
    "frontmatter": { "title": "..." },     // JSON object; may be null on some observer dispatches
    "body": "..."                          // Markdown body, without the frontmatter block; "" when the op has no page
  },
  "ctx": {
    "workspace": "default",                // resolved name; "" when it could not be resolved
    "project": "ai-memory-ops",            // resolved name; "" when it could not be resolved
    "destination_workspace": "archive",    // move ops only; omitted otherwise
    "destination_project": "ai-memory-ops",// move ops only; omitted otherwise
    "actor": {                             // identity of the caller; each field omitted when unknown
      "agent": "claude-code",
      "user": "example-user",
      "name": "Example Name",
      "email": "user@example.test",
      "session_id": "019e6d-...",
      "issuer": "...",
      "sub": "8f3a-...",
      "client": "72836f52-..."
    },
    "op": "write_page",                    // always present; one of the 15 values
    "partial_failure": true                // purge ops only; omitted when false
  }
}
```

Field notes:

- `actor` fields are **omitted** when unknown. They are never sent as `null`.
  Known identity fields are `agent`, `user`, `name`, `email`, `session_id`,
  `issuer`, `sub` and `client`.
- `frontmatter` is an object for page writes. For operations with no page, it
  is an empty object on the notification path and may be `null` on some
  observer dispatches. Hooks should treat both as "no frontmatter".
- `partial_failure` is set only on purge calls, and only when `true`. It is
  `true` when the durable database change committed but a later filesystem step
  failed:
  - `purge_project`: the database purge committed, but removing the project
    directory failed.
  - `purge_session`: the database purge committed, but removing the session
    page file failed.
  - `purge_workspace`: the database cascade committed, but removing the
    workspace directory failed.

  A mirror that tracks filesystem state should refuse to drop its own copy
  when this flag is set.
- The skip list (section 6) is never sent on the wire.

### 3.2 Response (hook to engine)

| Status | Body | Behaviour |
|---|---|---|
| `200 OK` | JSON object `{ "page": { "frontmatter": ..., "body": ... } }`. Both inner fields are optional. | A returned `frontmatter` replaces the whole frontmatter. A returned `body` replaces the body. A missing or `null` field means "leave unchanged". Unknown keys are ignored. The engine applies the new values before the next hook, or before the final write. |
| `204 No Content` | (empty) | No change. The hook is an observer. |
| Other `2xx` with an empty, non-JSON, or unreadable body | (any) | No change, with a warning. This applies **whatever the failure policy**. |
| `4xx` / `5xx` | (optional text, logged, read up to the size cap) | See section 4. |

The engine reads at most **1 MiB** of a response body. A body over that limit is
treated as "no change", with a warning. This also applies whatever the failure
policy. A hook has no legitimate reason to return more than the page envelope.

On non-blocking and notification calls, the response body is ignored. Only the
status is checked and logged.

## 4. Failure policy

Each hook picks one policy. It applies to all the ops that hook subscribes to,
including the handoff ops.

- **`ignore` (default, recommended).** The failure is logged as a warning. The
  operation continues with the page unchanged. Use this for everything except
  safety-critical enforcers.
- **`reject`.** The operation is aborted, and the error is returned to the
  caller. A failure here means a non-2xx status, an unreachable host, or a
  request timeout. Use `reject` only when the hook is a hard precondition for
  the operation (for example, a future `validate-no-secrets` enforcer).

Exceptions, where `reject` does not abort:

- A 2xx response whose body is empty, not JSON, unreadable, or over 1 MiB. It
  is treated as "no change", with a warning.
- Any non-blocking hook, whatever its policy. These cannot refuse an operation.
- A blocking hook on a handoff op with `failure_policy = "reject"` only refuses
  if it is also blocking. The rule is in section 2.4.

On the automatic handoff paths, a refusal degrades the lifecycle event instead
of failing it (section 2.4).

Exact details of the original, for a faithful reimplementation:

- **Error texts** (the refusal reaches the caller as an I/O-class error with this message): `admission webhook <name> status <status>: <body>` for a non-2xx (`<status>` is the numeric code followed by its reason phrase, for example `403 Forbidden`; `<body>` is the response text read up to 1 MiB, invalid UTF-8 replaced, or `<response body exceeds 1048576 bytes>` / `<failed to read response body: …>`), and `admission webhook <name> request failed: <transport error>` for an unreachable host or timeout. On the page-write path (`run`) the same two texts are used; the notification path uses the same texts.
- **"Unavailable" classification** (used by the automatic improvement write, which must not record an outage as a verdict on the proposal): a refusal is *unavailable* when it came from a transport error, a timeout, any 5xx, 408 or 429; any other non-2xx (for example 403) is a real refusal.
- **Statuses on the page-write path:** 204 means no change. Any other 2xx has its body read (up to 1 MiB) and parsed as the envelope; `page` absent means no change; an unreadable, non-JSON or oversized body means no change with a warning. A JSON `null` for `frontmatter` or `body` means unchanged. 1xx and 3xx that the HTTP client does not follow count as errors under the failure policy. The default HTTP client of the original follows up to 10 redirects and honors the system proxy environment variables; a reimplementation should do the same, or document the deviation.
- **Notification and handoff paths:** any 2xx is success and the body is ignored; the body is never parsed.
- **Timeouts** are clamped when the request is made (not when the configuration is loaded): below 1 ms becomes 1, above 30 000 becomes 30 000. A zero in configuration therefore means 1 ms.
- **Chain construction:** more than 16 hooks fails with `admission chain capped at 16 webhooks, got <N>`; an empty list builds no chain. Hook names are not required to be unique (the skip list matches by name and so skips every hook with that name).
- **Order inside one chain walk:** hooks are visited in configured order; for each hook the skip list is checked first, then the op subscription, then (page-write path) whether it is non-blocking.

## 5. Workspace and project names

The engine resolves the workspace and project identifiers into the names used by
the UI and by the on-disk wiki. Hooks can therefore address pages by name,
without looking up identifiers themselves.

- Resolution happens before the chain runs. It requires a store reader, which
  the server attaches. If no reader is attached, both fields are empty strings.
- Hooks should treat names as opaque strings. The names follow the same
  validation as the workspace and project options of the command-line
  interface (confirmed: the chain does not re-check them; it copies whatever the store
  returns).
- Names are stable for the lifetime of the workspace or project. The engine
  does not rename them silently. A manual rename is a separate operation.
- Destination names are supplied by the move operation itself.

## 6. Loop prevention

A hook that writes back into the engine, for example through the administrative
write route, the MCP write tool, or the hook ingress, must send this header on
the re-entrant call:

```text
X-Memory-Skip-Admission-Chain: <name>[,<name>...]
```

- The value is a comma-separated list of hook names. Entries are trimmed, and
  empty entries are dropped.
- The engine skips the named hooks for that request only. Blocking calls,
  notifications, and non-blocking dispatch all honour the list.
- The header is honoured only for trusted callers: auth-disabled requests
  (anonymous level) and root requests. A database user who sends it has the
  header ignored. The request is not rejected, and the named hooks still run.
- Without the header on a trusted re-entrant write, the chain can recurse:
  engine, then hook, then engine, then hook, and so on.

## 7. Limits

| Limit | Value | What it caps |
|---|---|---|
| Hooks per chain | `16` | Chain length. A configuration with more than 16 hooks fails to start the server. This catches templating mistakes, such as a duplicated block, before they add 17 or more synchronous calls to every write. |
| Per-hook `timeout_ms` | Default `2000`. Values are clamped to the range `1` to `30000` ms. | One request. A value below 1 becomes 1. A value above 30000 becomes 30000. |
| Worst-case blocking latency | Sum of `timeout_ms` over all **blocking** hooks subscribed to the op | Applies to page writes, notifications, and consolidation preflight. Non-blocking hooks add no latency. |
| Session-start claim deadline | `750` ms, for the whole deciding chain | Automatic handoff acceptance at session start. The shell hook's deadline for fetching the handoff is one second. The native hook command's handoff timeout defaults to three seconds. |
| Response body size | `1 MiB` (1,048,576 bytes) | Hook response body. Above this, the response is ignored with a warning. |
| In-flight fire-and-forget requests | `256` per process | Non-blocking dispatch, and observer dispatch on handoff and message ops. Beyond this, the request is dropped and logged, not queued. The operation has already committed, so the caller is never stalled. |

Drop logging for the in-flight cap:

- A dropped request for a hook configured `blocking = true` is logged at ERROR.
  That hook was meant to run, so the drop needs to be visible.
- A dropped request for a non-blocking hook is logged at WARN.

## 8. Configuration

The hook list is set in the server configuration file, under the
`admission_webhooks` array:

```toml
[[admission_webhooks]]
name           = "contributors"                                     # stable identifier; used in the skip list and in logs
url            = "http://contributors.example.internal:8080/enrich"  # POST target
timeout_ms     = 2000                                               # per request; default 2000
failure_policy = "ignore"                                           # "ignore" (default) | "reject"
events         = ["write_page", "consolidate"]                      # required; ops this hook subscribes to
blocking       = true                                               # default true; runs before the write, may mutate or refuse

[[admission_webhooks]]
name           = "git-mirror"
url            = "http://git-mirror.example.internal:8080/sync"
timeout_ms     = 2000
failure_policy = "ignore"
events         = ["write_page", "consolidate", "delete", "purge_project", "move_project"]
blocking       = false                                              # fire-and-forget after the operation; never blocks or refuses it
```

Field rules:

- `name`: required. Used in the skip list and in log fields.
- `url`: required. Each call is a POST to this URL.
- `timeout_ms`: optional. Default 2000. Clamped to 1 to 30000 ms (section 7).
- `failure_policy`: optional. Default `ignore`.
- `events`: required. A list of op values. An empty list means the hook never
  fires.
- `blocking`: optional. Default `true`.

### 8.1 `blocking`

A hook is either blocking or non-blocking.

- **`blocking = true`** (default). The hook runs synchronously inside the
  operation. It can mutate a page write, and with `reject` it can refuse the
  operation. The operation waits for it, up to `timeout_ms`. Use this for
  enrichers and validators, for example `contributors` or `validate-no-secrets`.
- **`blocking = false`**. The hook is dispatched fire-and-forget **after** the
  operation has committed. The engine does not wait for it and ignores its
  response, so it cannot mutate or refuse anything. Use this for backups and
  mirrors, for example `git-mirror`, so a slow or unavailable sink never adds
  latency to writes.

What each combination does:

| `blocking` | `failure_policy` | Page writes and notifications | Handoff and message ops |
|---|---|---|---|
| `true` | `ignore` | Awaited. Can mutate a page write. Failures are logged and ignored. | Observer: dispatched after the operation, may be dropped under load. |
| `true` | `reject` | Awaited. Can mutate a page write. A failure aborts the operation. | Decider: awaited before the operation. A refusal aborts it. |
| `false` | any | Never awaited. Dispatched after the commit. Cannot mutate or refuse. | Observer: dispatched after the operation, may be dropped under load. |

Two consequences:

- A hook that must not be dropped on handoff or message ops needs
  `blocking = true` **and** `failure_policy = "reject"`. Both are required. With
  `blocking = false`, `reject` makes no difference.
- The latency of a page write is the sum of `timeout_ms` over its blocking
  hooks. Non-blocking hooks add none.

### 8.2 Environment override

```bash
AI_MEMORY_ADMISSION_WEBHOOKS_JSON='[{"name":"contributors","url":"http://contributors.example.internal:8080/enrich","timeout_ms":2000,"failure_policy":"ignore","events":["write_page","consolidate"],"blocking":true}]'
```

- The variable holds a JSON array of hook objects. Each object has the same
  fields as the configuration file.
- When the variable is set and not blank, it **replaces** the hook list from the
  configuration file and from any other source. A blank value is ignored.
- A parse error fails the configuration load.
- The JSON form is the recommended way to set hook lists. Structured lists are
  not reliable through indexed environment variables.

### 8.3 No hooks configured

An empty hook list means no chain is attached. The server skips chain
construction entirely, so writes have no per-write overhead beyond a single
check.

## 9. Worked examples

These examples are illustrative. The hook names and addresses are placeholders.

### 9.1 Mutating: add the writer to `frontmatter.contributors`

```jsonc
// POST /enrich
{
  "page": { "path": "gotchas/x.md", "frontmatter": { "title": "X" }, "body": "..." },
  "ctx":  { "workspace": "default", "project": "ai-memory-ops",
            "actor": { "agent": "claude-code", "user": "example-user", "client": "72836f52-..." },
            "op": "write_page" }
}

// 200 OK
{
  "page": {
    "frontmatter": {
      "title": "X",
      "contributors": [
        { "agent": "claude-code", "user": "example-user", "client": "72836f52-...",
          "first_seen": "...", "last_seen": "...", "writes": 1 }
      ]
    }
  }
}
```

The engine replaces the whole frontmatter with the returned object before it
persists. The body is unchanged, because the response does not include it. The
hook must therefore return every frontmatter field it wants to keep.

### 9.2 Side effect: mirror the write to an external repository

```jsonc
// POST /sync (same request body)
// 204 No Content
```

The hook writes the page to a local copy of an external repository, batches the
commits, and pushes asynchronously. The engine does not wait for the push. Only
the local enqueue runs in the operation, and only within the hook's timeout.

## 10. Test coverage

The behaviour in this document is covered by a suite of tests for the component.
The suite covers:

- Frontmatter and body mutations are applied.
- A `204` response changes nothing.
- `ignore` swallows failures, and `reject` aborts the operation.
- Hooks run in declared order, and each sees the previous mutation.
- The skip list short-circuits the named hooks.
- The `X-Memory-Op` header is set correctly for each op.
- Hooks receive only the ops they subscribe to.
- The hook count limit is enforced when the chain is built.
- An oversized response is treated as "no change".
- Workspace and project names appear in the payload.
- Delete and purge notifications carry the correct op, and purge notifications carry no page path.
- Non-blocking hooks are skipped on the synchronous path and dispatched after the commit.
- An end-to-end page write resolves names, runs the chain, and records the payload.

## 11. Non-goals

These are outside the current contract. They are not planned for this version.

- **Parallel fan-out.** The chain is sequential by design. Mutations compose in
  a defined order, so blocking hooks cannot run in parallel.
- **Dynamic registration.** Hooks are named by the operator in configuration. There is no endpoint to add hooks at runtime.
- **Per-hook metrics.** There is no metrics endpoint for hooks. Observability is through logs only.
