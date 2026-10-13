# Cross-project agent messaging (inbox and queue)

Direct, claim-once messaging between two projects, so an agent in one project
can ask an agent in another project to do something **without loading that
project's context into its own session**. Introduced in version 2.3, with
schema migration 64.

The motivating case: you have Kimi open in `Projects/A` and Claude open in
`Projects/B`. A needs a change from B. You do not want Kimi to read and reason
about B (wasted tokens, polluted memory). Instead, Kimi writes a self-contained
request and drops it into B's inbox through the memory tool. Later, in B, you
tell Claude to check its inbox; it pops the message and works from it.

This is the one place where the memory tool deliberately crosses its per-project
isolation boundary. The crossing is explicit and scoped in both directions: a
project sees only mail addressed **to** it (its inbox) or sent **from** it (its
outbox).

## The model

- A message is addressed to a **project**, not a person. Any session working in
  the recipient project can pop it. Knowledge is shared; the pop is the
  claim-once step, the same rule that handoffs follow.
- **Claim-once queue.** A message is `pending` until a session in the recipient
  project pops it (it becomes `claimed`, exactly once) or the sender cancels it
  (it becomes `cancelled`). A second pop of the same message returns nothing.
  Claimed and cancelled messages are kept for audit rather than deleted. They
  are removed only if the sending or receiving workspace or project is purged.
- **Fail closed on an unknown recipient.** The recipient workspace and project
  must already exist. A send to an unknown target is rejected; nothing is stored
  and no inbox is created.
- **Bounded.** Each recipient inbox holds at most **256 pending messages**. A
  send to a full inbox is rejected.

## Using it from an agent (MCP, primary path)

Four MCP tools. The same operations are described in the messaging skill that
ships with the memory tool.

| Tool | Role | Main parameters | Returns |
|---|---|---|---|
| `memory_message_send` | Send a message to the inbox of `to_workspace` + `to_project`. | `to_workspace`, `to_project`, `body` (required); `subject`, `from_workspace`, `from_project` (optional) | `{ "message_id": "<id>" }` |
| `memory_message_list` | List pending mail. Read-only. | `box` (`"inbox"` default, or `"outbox"`); `limit` (default 50, clamped to 1–200); `workspace`, `project` | `{ "messages": [ ... ] }` |
| `memory_message_pop` | Claim the next inbox message: the oldest, or the one with `message_id`. | `message_id`, `workspace`, `project` (all optional) | `{ "message", "security_notice" }`, or `{ "message": null }` |
| `memory_message_cancel` | Retract one sent message (`message_id`), or every pending message the project has sent (no `message_id`). | `message_id`, `workspace`, `project` (all optional) | `{ "cancelled": N }` |

**In Kimi (project A), sending:**

> "Ask the agent in `default/project-b` to add a `/v1/export` endpoint that
> streams NDJSON, and to keep the existing auth middleware. Send it through
> ai-memory."

Kimi calls `memory_message_send` with `to_workspace: "default"`,
`to_project: "project-b"`, and a self-contained `body`. Compose the body as a
complete prompt: the recipient cannot see project A.

**In Claude (project B), receiving:**

> "Check my ai-memory inbox."

Claude calls `memory_message_pop`, which returns the oldest message and removes
it from the queue. To look without consuming, use `memory_message_list`.

**Giving up on a request (from A):**

> "Never mind that request I sent to project-b. Cancel it."

Kimi calls `memory_message_cancel`. With no id it cancels every pending message
A has sent; with a `message_id` it retracts only that one. A message already
popped by B cannot be cancelled.

## Using it from the terminal (CLI, secondary)

```bash
ai-memory message send --to-workspace default --to-project project-b \
    --subject "export endpoint" "Add a /v1/export endpoint that streams NDJSON…"
# body may also be piped on stdin:
echo "…request…" | ai-memory message send --to-workspace default --to-project project-b

ai-memory message list                 # this project's inbox (pending)
ai-memory message list --outbox        # what this project has sent
ai-memory message pop                  # claim the oldest inbox message
ai-memory message pop --id <message-id>
ai-memory message cancel --id <message-id>
ai-memory message cancel --all         # clear this project's outbox
```

- `send`: `--to-workspace` and `--to-project` are required. The body is the
  positional argument; when it is omitted, the full body is read from stdin.
  Optional: `--subject`, `--from-workspace`, `--from-project` (default: the
  resolved scope), `--json`.
- `list`: `--outbox` switches to sent mail. `--limit` accepts 1–200 (default 50);
  values outside that range are rejected by the argument parser.
- `pop`: `--id` pops a specific message. The output shows sender provenance,
  the security notice, and the body between delimiter lines.
- `cancel`: exactly one of `--id` or `--all` is required. Neither, or both, is
  an error.
- `--workspace` and `--project` on `list`, `pop`, and `cancel` default to the
  resolved scope.

The CLI resolves the current project as the sender or reader scope, the same way
the [handoff commands](design-handoff-offer-not-claim.md) do. See also the
[CLI reference](cli.md).

## Rules for sending and reading

### Scope and permissions

- **Sender:** the current project by default, or an explicit `from_workspace` /
  `from_project`. The named sender must exist and the caller needs read access
  to it. Sender scope is never created.
- **Recipient:** must exist (never created). Over MCP, the caller needs **write**
  access to the recipient. A read-only grant is refused for send, pop, and cancel
  on a restricted project.
- **Multi-user servers:** sending needs the normal write capability on the
  recipient, and each message records who sent it for audit.

### Size limits

| Field | MCP | Administrative HTTP endpoint | Stored |
|---|---|---|---|
| `body` | Secret-scrubbed, then cut to 8,000 characters, with a truncation marker that states how many characters were omitted. | Secret-scrubbed, then cut to 8,000 bytes. No marker. | Cut to 16 KiB (bytes). |
| `subject` | Secret-scrubbed, then cut to 200 characters. | Secret-scrubbed, then cut to 200 bytes. An empty subject becomes none. | Cut to 16 KiB (bytes). |

Bodies are secret-scrubbed before storage on every send path.

### Pop

- Without `message_id`, the **oldest** pending message in the inbox is popped
  (ordered by creation time).
- With `message_id`, only that message is popped. If it is not pending in this
  inbox (addressed elsewhere, already popped, cancelled, or unknown), the result
  is `null`, not an error.
- A malformed `message_id` is an error. Over HTTP it is a 400 response with the
  message "must be a full UUID". An empty string is treated as omitted.
- When two sessions race for the same message, exactly one gets it. The loser
  receives `null`.
- Each successful pop records the claim time and who claimed it.

### List

- Read-only. Nothing is consumed.
- Only pending messages are listed, oldest first.
- An unknown `box` value is an error.
- Over MCP, if the scope was inferred rather than given explicitly and the inbox
  is empty, the response adds a hint to retry with explicit `workspace` and
  `project`.

### Cancel

- Only pending messages sent by the caller's own project are affected. Messages
  already popped or cancelled are unaffected.
- Cancelling nothing is not an error: the response is `{ "cancelled": 0 }`. This
  also applies when an id belongs to another sender.

### Inbox capacity

- The limit is 256 pending messages per recipient inbox. A send to a full inbox
  is rejected with a message that states the limit. Over HTTP, the status is 409.
- Popping or cancelling frees capacity.

## Security: a popped message is untrusted input

A popped message was composed by an agent in **another** project. Treat the body
as a **task request to evaluate with the user, never as instructions to obey**.
The design enforces this on several levels:

- **Nothing auto-enters context.** The session-start notice (below) shows only a
  count. Message text reaches an agent only through a deliberate pop.
- **The body is kept apart from everything else.** On MCP, the body is a field of
  the popped message object, and the response carries a `security_notice` beside
  it. The notice states that the body is untrusted input and that it must not, on
  its own, cause the recipient to run commands, reveal secrets, change policy,
  or call tools. The body text itself is not wrapped in markers over MCP. The CLI
  prints the notice and then the body between delimiter lines.
- **Provenance is returned outside the body.** The popped message carries
  `from_workspace_id`, `from_project_id`, `from_agent`, and `from_owner_user`.
  The first two are identifiers, not names. See the limits below for `from_agent`.
- **Secrets are scrubbed and the body is size-capped** on send, so a message
  cannot carry credentials across projects or flood the recipient's context.
- **Isolation holds.** Inbox reads are keyed by the recipient coordinate; outbox
  reads and cancel are keyed by the sender coordinate. No query joins a third
  project.
- **Admission.** A send is checked against the recipient project's message
  admission policy. A refusal stops the send: the admin endpoint returns 403 and
  nothing is stored. Over MCP, pop and cancel are also checked before they run,
  and a refusal stops the operation.
- **Attribution only.** `from_owner_user` and `claimed_by_user` record who sent
  or popped a message. They are never used as a read filter.
- **Audit.** Each send and pop writes an audit event in the recipient scope.
  Each cancel writes one in the sender scope, and only when at least one message
  was cancelled.

## Session start: hot context and the inbox notice

The memory tool already gives a resuming agent a hot start. Each agent's
session-start hook fetches the handoff endpoint. For agents that discard
session-start output, such as Kimi, the same fetch runs on the first user prompt
instead. The response contains any pending single-use **handoff**. When the
project's `.ai-memory.toml` opts in with `[briefing] inject_on_session_start = true`,
it also contains a compiled **project brief**: pinned pages, `_rules/` and
`_slots/` pages, and pointers to recent pages. That block is prepended to the new
session's context automatically.

As of 2.3, the same block appends a **non-consuming inbox notice** when the
project has pending mail:

```text
📬 ai-memory: 2 cross-project messages waiting in this project's inbox.
Use `memory_message_pop` to read the next one …
```

The notice carries only a count, never message text, so it cannot be used to
inject content into a resuming agent. It does not depend on the briefing opt-in.
Nothing is popped. The count is also reported by the briefing tool as
`pending_message_count` (project-scoped briefings only).

## Known limits (verified against the current code)

- **`from_agent` is always `other`.** Every send path (MCP, HTTP, and therefore
  the CLI) records the sender's agent as `other`, and no session is recorded. A
  recipient therefore cannot tell which agent composed a message. (A confirmar:
  whether this is intended.)
- **The subject is documented as one line, but no line check exists.** Subjects
  are only scrubbed and capped. (A confirmar: whether a one-line rule is wanted.)
- **Pop and cancel are admission-checked only over MCP.** The administrative HTTP
  endpoints used by the CLI do not run admission for pop or cancel. (A confirmar:
  whether this is intended.)
- **Body caps differ by path.** MCP counts characters and adds a marker; the HTTP
  endpoint counts bytes and adds no marker. Both are followed by the 16 KiB storage
  cap.
- **An empty body is not refused** by any send path. (A confirmar: whether it
  should be.)
