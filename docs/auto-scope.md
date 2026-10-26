# Active-project isolation modes (`[auto_scope]`)

> **Escopo:** este documento contém partes que dependem de LLM ou de embeddings. Essas partes são opcionais e não fazem parte do escopo inicial, que funciona sem LLM. Podem ser removidas ou adiadas junto com as funções correspondentes.

`ai-memory serve` keeps a process-wide pointer to the "currently active
project". Read tools use it when the caller omits `workspace` and `project`,
and write tools use it when the caller omits them on a write. The pointer is
fed by lifecycle hook events, so the answer reflects the project the agent is
working in, not the server's static default project.

Two rules shape how the pointer is updated:

- Foreground events (session start, user prompt, pre-tool) advance the shared
  pointer and the caller's own keyed entry.
- Completion and shutdown events (post-tool, stop, session end, and others)
  refresh only their own keyed entry. They never move the shared pointer, so a
  delayed event from an older session cannot redirect a newer session's
  unscoped reads.

Since v1.39 the default mode keys the pointer by the caller's identity and
session, so two harnesses in one project, or two operators on one server, do
not overwrite each other's "current project". Before v1.39 the default was a
single shared slot. That mode is still available (see [Modes](#modes)).

## Why the mode matters

In the single shared slot, the last event wins. A hook firing from one
repository overwrites the slot that a concurrent unscoped read in another
repository is about to use. Because unscoped writes resolve through the same
pointer, a write can also land in the wrong project.

## Modes

The `mode` setting chooses how the pointer is keyed.

| `mode`        | Key                                         | When to use |
|---------------|---------------------------------------------|-------------|
| `single`      | none (one global slot)                      | One operator, one harness at a time. Last write wins. Opt in only if you want the pre-v1.39 behavior. |
| `per_session` | `session_id`                                | Session-aware clients or bridges that send the hook session id on every MCP request. |
| `per_actor`   | `(qualified identity, session_id)`, plus an identity-only entry | **Default.** Isolates parallel harnesses and separate operators. |

In every mode, foreground events also write the shared slot. So a caller with
no actor coordinate (an anonymous probe, or a legacy client) sees the most
recently active project instead of an empty pointer. In `per_session` and
`per_actor`, non-foreground events touch only their own keyed entry. That is
not per-session isolation for callers without a coordinate; such callers should
pass explicit `workspace` and `project` arguments when concurrent runs matter.

The default-global recall flag (set by a repository's `[recall] default_global`
marker) travels with the pointer. It is read with the same slot selection and
is `false` when no entry opted in.

## Publishing rules

Only an event that carries a working directory, a project override, or a
session scope that is already sticky publishes a pointer at all. Other events
are recorded without changing the pointer.

| Event class | Mode `single` | Mode `per_session` / `per_actor` |
|-------------|---------------|----------------------------------|
| Foreground: session start, user prompt, pre-tool | Writes the shared slot | Writes the shared slot and the caller's keyed entry. In `per_actor`, when both identity and session are present, the identity-only entry is also written. |
| Completion and other events | Nothing | Refreshes or creates only the caller's own keyed entry. The shared slot and the identity-only entry are not changed. |

An event with no actor coordinate never writes a keyed entry. It only writes
the shared slot, and only if it is foreground.

## Lookup rules

Lookup decides which project a caller gets. It runs in this order:

1. If the mode is `single`, or the caller has no usable coordinate for the
   mode, answer from the shared slot.
2. If the caller's keyed entry is live (not expired), answer with it.
3. In `per_actor`, if the caller has an identity but its keyed entry is
   missing, and the identity-only entry is live, the answer is **mismatch**.
4. If any keyed entry has ever been published in this process, the answer is
   **mismatch**.
5. Otherwise (nothing has ever been keyed, so the install has no lifecycle
   hooks feeding the pointer), answer from the shared slot.

Reads and writes treat the outcomes differently:

- **Resolved**: the pointer names a project. Reads and writes use it.
- **Mismatch**: the caller carries a coordinate, and hooks are live, but nothing
  matches it. Reads fall back to the caller's configured default project. An
  unscoped write returns an error asking for explicit `workspace` and `project`,
  so the page is not filed in a project the caller did not name.
- **Unset**: there is no pointer information at all. Reads may use the startup
  fallback described below. Writes use the configured default project.

The shared slot has no expiry. Keyed entries expire (see [Memory footprint](#memory-footprint)).

## Explicit scope arguments

Explicit arguments always win over the pointer, and they fail closed. They
never silently fall back to a different project.

- **Both `workspace` and `project`**: the pair must name an existing project.
  If it does not exist, the call returns an error. A `project` value written as
  `workspace/project` (exactly one `/`, with a non-empty name on each side) is
  tried as a pair only if the exact name does not exist. The exact name always
  wins.
- **`workspace` without `project`**: an error. The two must be given together.
- **`project` only**: resolved inside the active workspace first, then inside
  the server's default workspace. If neither contains it, the call returns an
  error. It does not fall back to the active or default project.
- **Neither**: the pointer decides (see [Lookup rules](#lookup-rules)). If the
  pointer is unset, the configured default project is used.

Creation is limited to explicit write paths:

- Read, search, maintenance, retention, embedding, and destructive paths use
  find-only lookups. They never create a workspace or a project, so a typo does
  not produce an empty scope. Full list (confirmed in audit 002 for the MCP tools):
  - **May create** (write-target resolution): `memory_write_page` (also via
    `memory_restore_page`, which resolves first without creating and then writes
    into that resolved scope) and `memory_handoff_begin`. Outside MCP, hook
    ingestion and `POST /workstream/runs` also create the workspace and project.
  - **Find-only, read access**: `memory_query`, `memory_recent`,
    `memory_read_page`, `memory_read_session_observations`, `memory_handoff_list`,
    `memory_message_list`, `memory_status`, `memory_briefing`, `memory_explore`,
    `memory_consolidate` (session lookup), and the sender side of
    `memory_message_send`.
  - **Find-only, write access** (mutate something that must already exist):
    `memory_feedback`, `memory_forget_sweep`, `memory_lint`,
    `memory_auto_improve`, `memory_delete_page`, `memory_restore_page`,
    `memory_handoff_accept`, `memory_handoff_cancel`, `memory_message_pop`,
    `memory_message_cancel`, and the recipient side of `memory_message_send`.
  - `memory_install_self_routing` resolves no scope.
  An unscoped call resolved by a fallback (not by a hook session) is logged as a
  warning.
- Explicit write paths may create a missing workspace and project. A
  `workspace/project` label never creates anything.

Tools that mutate data use the access level of the mutation, not the shape of
their arguments. A tool that deletes, records feedback, sweeps, or accepts a
handoff requires write access on the scope even if it takes read-shaped
arguments.

Multi-project search takes an explicit list of scopes. The list is
de-duplicated. A list longer than the configured maximum returns an error. An
empty workspace or project name returns an error. A scope the caller may not
read fails the whole call; it is not silently dropped.

## Actor identity sources

The actor key is built from what the request carries:

| Source | Populates |
|--------|-----------|
| Lifecycle hook request (event and agent in the query, session id in the payload) | `session_id`, `agent` |
| Root bearer token with a configured root username | `user` (the root username) |
| Trusted reverse proxy asserting a username, or an OIDC `(issuer, subject)` pair | `user` (username), or the qualified `(issuer, subject)` identity |
| Database-backed user (API credential owned by a `users` row) | `user` (the user's username) |
| MCP header `X-Memory-Actor-Session-Id` | `session_id` for tool calls |
| MCP native client metadata (for example the OpenCode session id) | `session_id` for tool calls |
| MCP header `Mcp-Session-Id` | `session_id` only as a last resort, and only when the server runs in stateful HTTP mode |
| Anonymous request, no token | empty actor, which uses the shared slot |

When a request carries both an identity and a session id, `per_actor` keys it
by both. When it carries only an identity, it uses that identity's identity-only
entry. When several sources carry a session id, the first one found wins, in
this order: the authentication layer's session id, `X-Memory-Actor-Session-Id`,
native client metadata, `Mcp-Session-Id`.

A trusted proxy must assert identity in one of two ways, and never both or
neither: a username, or an issuer and subject pair. A repeated actor header
makes the assertion ambiguous, and the request is refused.

`X-Memory-Actor-Session-Id` is the agent-run session id from the lifecycle hook
payload. It is not the login session of an identity provider. The provider's
`sid` claim identifies a browser or device session and must not be used as the
session key.

The identity key is qualified, so a username cannot collide with an OIDC
subject. When both an OIDC pair and a username are present, the OIDC pair wins.

Owner and agent identify a session; scope does not. Session ids are global for
stored observations. If a different owner reuses an id that is already owned,
the hook is dropped before it can append observations or publish a pointer for
the foreign actor. The composite key only namespaces the active-project
pointers.

A session event that names a different project or workspace from its session
is recorded in the project it names. This is the mid-session behavior described
in [`[routing] mid_session`](marker-file.md#mid-session-navigation-routing-mid_session).
One exception applies: a session-end event that names a different scope is
dropped, unless it comes from the same working directory as the session. A
drop of this kind is a rejection of that event only; it does not end someone
else's session.

## Session-aware clients

Lifecycle hooks already include the agent-run session id in their payloads. MCP
tool calls are separate HTTP requests. Static MCP configuration usually declares
only a fixed URL and fixed headers, so it cannot send the current session id on
every call. Whether a given client can add headers per request **depends on the external product**
for each client.

Claude Code can opt into a session-aware stdio bridge:

```bash
ai-memory install-mcp --client claude-code --session-aware --apply
```

The bridge reads the session id that Claude Code passes to its stdio MCP
subprocess, and forwards it as `X-Memory-Actor-Session-Id`. It keeps the
configured remote endpoint and bearer token. Existing Claude Code installs keep
the static HTTP transport unless this flag is used. The bridge refuses to start
when no session id is available, rather than silently falling back to the shared
slot.

Claude Code details that affect session keys:

- The stdio subprocess keeps the session id it received at startup across
  `/clear`, while later hooks receive the new id. Restart Claude Code after
  `/clear` when exact session continuity matters. The exact behavior **depends on the external product**
  (check the client version in use).
- On `--continue` or `--resume` without an explicit id, the subprocess may keep
  the startup id instead of the resumed one. Prefer `--resume <session-id>` for
  explicit resumes. This **depends on the external product** (check the client version in use).

Use `per_session` only when the client or bridge sends the same session id from
the hook payload on every MCP request, as `X-Memory-Actor-Session-Id` (preferred)
or `Mcp-Session-Id`. A request with a session id that matches no hook session
falls back to the server's default project. A request with no usable actor
identity still uses the shared slot.

OIDC or Keycloak authentication identifies the human user, client, and agent. It
does not identify the current coding-agent session. A gateway that validates a
token should forward the user identity (`X-Memory-Actor-User`, or the issuer and
subject pair), with optional client and agent names. It should send
`X-Memory-Actor-Session-Id` only when a session-aware bridge has forwarded a real
agent session id.

Recommended settings for static MCP configuration:

- `single` for one operator working on one project at a time.
- `per_actor` with multi-user bearer authentication when several people share a
  server. It isolates users. Concurrent sessions of the same user still need
  explicit `workspace` and `project` arguments, or a session-aware bridge.
- `per_session` plus the session-aware bridge when one operator runs concurrent
  Claude Code sessions in different projects.

## Pairing with multi-user mode

`per_actor` is most useful in multi-user mode (see [users.md](users.md)). Each
API credential resolves to its owning user, so every request is tagged with the
right user. Two authenticated users running concurrent agent sessions on the same
server no longer overwrite each other's pointer. Concurrent sessions by the same
user are isolated only if the client also forwards session ids.

A single-user install can use `per_session` alone, without a token pepper and
without a users row, but only when the client or bridge forwards the session id
on MCP calls.

## Surviving a restart

The pointer lives in process memory. Restarting the server empties it, including
for sessions that are still open. Until the next foreground hook event arrives,
every keyed read misses. Without a fallback, an unscoped read would answer for
the default project and report empty results, with no sign that the project
actually holds data.

To prevent this, the HTTP transport of `serve` seeds a **read-only fallback** at
startup. The seed is the project of the most recently created observation that
falls within the TTL window (`session_ttl_secs`). It is applied only if the
shared slot is still empty. A missing or failed lookup is logged and does not stop
the server.

Four bounds keep the seed narrow:

- **Reads only.** The seed is a reconstruction, not an observed publish. It lives
  in its own slot, so an unscoped write still resolves as if nothing were
  published. It fails closed on a real mismatch and otherwise uses the configured
  default.
- **Keyed entries are never reconstructed.** A keyed hit still wins, so per-actor
  isolation is unchanged.
- **Bounded by the TTL at startup.** A server that was down longer than the TTL
  starts with no fallback. The seed does not age after startup.
- **Superseded by the first foreground event.** The shared slot is written, and
  the seed is no longer consulted for any caller. Admin invalidations (project
  purge, project move, workspace removal) also clear the seed, so a scope that no
  longer exists cannot keep answering from it.

Reads do not fail closed on an unresolved pointer. An unresolved keyed read is
normal before the first publish (hooks are fire-and-forget, so a session's first
read can arrive before its session-start event lands), and it is normal after
TTL or cap eviction on a busy server. Failing there would turn these normal
cases into errors.

## Configuration

```toml
[auto_scope]
mode = "per_actor"        # "per_actor" (default) | "per_session" | "single"
session_ttl_secs = 3600   # TTL for keyed entries, in seconds (default 1 hour)
max_entries = 4096        # cap on keyed entries; the oldest insertions are evicted first
```

The same settings can be set through environment variables:

```bash
AI_MEMORY_AUTO_SCOPE__MODE=per_actor
AI_MEMORY_AUTO_SCOPE__SESSION_TTL_SECS=7200
AI_MEMORY_AUTO_SCOPE__MAX_ENTRIES=8192
```

Edge cases:

- `session_ttl_secs = 0` is replaced by the default of 1 hour. The server starts.
- `max_entries = 0` is raised to 1. The server starts.
- A mode value that is not one of the three listed values fails startup. This
  includes an empty value and a capitalized form such as `PerSession`. The
  server never falls back to `single` silently.

The effective mode and limits are logged once at startup with the message
`active-project isolation mode`. This is the quickest way to confirm what a
running server uses.

## Memory footprint

Each keyed entry stores a project id, a workspace id, a default-global flag, and
an insertion time. Its key stores two strings: the identity and the session id.
The size of the two strings is not limited by the code reviewed, so the total
memory depends on them. The default cap is 4096 entries. (Audit 002: confirmed
that no application-level length limit applies to the actor identity or to the
`X-Memory-Actor-Session-Id` value; they are only trimmed, and an empty value
counts as absent. The only bound is the HTTP server's own header-size limit, so
the worst case is roughly 4096 × two header-sized strings. Original behavior,
reproduce as is: no length cap on these strings. The default per-key lifetime
is 1 hour.)

Expired entries are removed on every read and write. The cap removes the oldest
insertions first when the map is full.

## Behavior after upgrading to v1.39

The default changed from `single` to `per_actor`. For most installs nothing
visible changes:

| Setup | Before v1.39 | After v1.39 |
|-------|--------------|-------------|
| One harness, hooks installed | Shared slot | Keyed entry for that session, same project |
| Static MCP client (bearer, no session id) | Shared slot | That operator's identity-only entry, same project |
| Client that forwards no identity | Shared slot | Shared slot, unchanged |
| MCP-only install, no lifecycle hooks | Shared slot | Shared slot, because nothing is ever keyed |
| Two harnesses or two operators | One slot, last write wins | One entry each |

One behavior changed on purpose. A client that forwards a session id matching no
published hook activity no longer inherits the shared slot. Answering from there
could route the request into whichever project published last, possibly another
operator's. Such a client now gets the server's configured default project.

To restore the previous behavior:

```toml
[auto_scope]
mode = "single"
```

or set `AI_MEMORY_AUTO_SCOPE__MODE=single`.

## Requirements for changes to scope resolution

Changes to scope resolution, or new MCP, admin, or web routes that resolve
scopes, should use the shared resolution helpers instead of writing their own
lookup chains. Pull requests that change scope resolution should include tests
for:

- rejection of partial scopes,
- rejection of a missing explicit scope,
- precedence of the active project,
- isolation across workspaces.
