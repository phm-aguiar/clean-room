# Codex desktop (the ChatGPT desktop app)

> **Escopo:** este documento contém partes que dependem de LLM ou de embeddings. Essas partes são opcionais e não fazem parte do escopo inicial, que funciona sem LLM. Podem ser removidas ou adiadas junto com as funções correspondentes.

> Research notes on how the Codex desktop app relates to ai-memory. Each claim
> carries one of three evidence labels:
>
> - **Project behavior**: checked against how ai-memory itself implements Codex
>   support (install commands, hook events, MCP setup, transcript reading).
> - **Observed**: seen on a research machine on 2026-10-08 and recorded in
>   [verification-2026-10.md](verification-2026-10.md). Not checked against the
>   Codex source.
> - **a confirmar**: no evidence yet. Do not treat as fact.

## Agent identity

- **Project behavior**: the agent name is `codex`. It is accepted by hook
  installation, MCP client setup, the managed-workstream harness and the
  finalize command. Finalize defaults to Codex.
- **Project behavior**: Codex is one of the hook-capable agents, together with
  Claude Code and OpenCode.
- **Project behavior**: the LLM provider option `codex` reuses the Codex CLI
  login, including its delegated token refresh.

## Relationship to the desktop app

- **Observed**: the desktop app starts the `codex` program found on the system
  path in server mode, and sets `CODEX_HOME` to the user's Codex directory
  unless it is already set. The desktop app and the CLI therefore share the
  config file, the hooks file, the hook trust state and the session store.
  Observed on one Linux machine only.
- **Observed**: no desktop-app session had been written to the session store
  yet, so the desktop session path is not confirmed.
- **a confirmar**: which platforms the desktop app supports and how it is
  distributed.
- **a confirmar**: whether a conversation list in the CLI and one in the
  desktop app can be bridged. No bridge is known to exist.
- **a confirmar**: whether the web and mobile ChatGPT surfaces read local Codex
  configuration.

## Hooks installed for Codex

### Where the hooks file is written

- **Project behavior**: `$CODEX_HOME/hooks.json` when `CODEX_HOME` is set and
  not blank. Otherwise `~/.codex/hooks.json`. `install-hooks --config-file`
  overrides both.
- **Project behavior**: `install-hooks` needs `--agent codex`. Its default agent
  is Claude Code.
- **Project behavior**: the write is atomic. Entries that other tools placed on
  the same event are kept. Entries that ai-memory placed are replaced. An update
  writes a backup next to the file. If nothing changed, the installer reports
  "already up to date".
- **Project behavior**: the file uses the nested shape, the same as Claude Code.
  Each event holds entries with a matcher and a nested list of command handlers.
- **Project behavior**: each hook has a variant for Unix-like systems and one
  for Windows.

### Events installed (seven)

| Codex event | ai-memory event | Behavior |
|---|---|---|
| `SessionStart` | `session-start` | Sends the payload to the server. Then fetches any pending handoff for the project and prints it to standard output. Failures do not block the session. |
| `UserPromptSubmit` | `user-prompt` | Sends the prompt. |
| `PreToolUse` | `pre-tool-use` | Sends the tool call. Format in the tool section below. |
| `PostToolUse` | `post-tool-use` | Sends the tool result. Format in the tool section below. |
| `PreCompact` | `pre-compact` | Refreshes the session checkpoint page. Does not end the session and does not create a handoff. |
| `Stop` | `stop` | Sends the stop event. The assistant message is included only when the opt-in is on. |
| `SessionEnd` | `session-end` | Sessions with no content are closed with no artifacts. Substantive sessions get a summary page and an automatic handoff. **a confirmar**: the comment in the implementation says this event needs Codex CLI 0.145.0 or later. |

- **Project behavior**: no subagent events are installed for Codex.
- **Project behavior**: the seven events are the complete installed set. The
  upstream product documents more events (permission request, post-compaction,
  subagent start and stop, interrupt). Those are **a confirmar** for Codex and
  are not installed by ai-memory.

### Server address and install options

- **Project behavior**: the default server address is `http://127.0.0.1:49374`.
  The hook scripts read the `AI_MEMORY_HOOK_URL` variable, and
  `install-hooks --server-url` bakes a server address into the hooks.
- **Project behavior**: `--auth-token` adds a bearer token to the hook requests.
- **Project behavior**: `--project-strategy` and `--capture-mode` bake defaults
  into the hooks. Under the `allowlist` capture mode, a repository without a
  project marker emits no event at all.

### Trust

- **Project behavior**: the installer prints a review notice when it creates or
  updates the hooks file. It prints nothing when the file is already current.
  The notice also names a flag that skips the review for automated installs
  (`--dangerously-bypass-hook-trust`). **a confirmar** against the Codex
  documentation.
- **Observed**: untrusted hooks do not run, and no message is shown.
- **Observed**: trust is recorded against the content of each hook (event,
  matcher and handler). Changing a command (for example the binary path, the
  data directory or the server address) makes the hook untrusted again until it
  is reviewed.
- **Observed**: with trusted hooks, a server session runs `SessionStart`,
  `UserPromptSubmit`, `PreToolUse`, `PostToolUse` and `Stop`.
- **Observed**: `SessionEnd` fires when a thread is archived or when the server
  shuts down cleanly. It does not fire when a client unsubscribes.

### Payload fields

- **Project behavior**: the hook path reads `session_id`, `cwd`, `prompt`,
  `tool_name`, `tool_use_id`, `tool_input`, `tool_response` and, on `Stop`,
  `last_assistant_message`.
- **Project behavior**: the hook path does not read the transcript path field.
  Transcripts are found in the session store instead.
- **Observed**: the payload also carries `hook_event_name`, `model` and
  `permission_mode`. `SessionStart` carries `source`. `UserPromptSubmit` carries
  `turn_id`.
- **Observed**: the payload does not say which client started the session. The
  client name appears only in the session header of the transcript.

### Tool events

- **Project behavior**: Codex uses the closed-tool format. The title is
  `tool <family>`, where the family is `file`, `search-list`, `non-file` or
  `unknown`.
- **Project behavior**: the body has a `tool_family:` line and, when present, a
  tool call identifier. `PostToolUse` adds an `outcome:` line. The output
  follows a `---` separator.
- **Project behavior**: the output comes only from the `tool_response` field.
  When no output exists, the body says `(no output captured)`.
- **Project behavior**: the outcome for Codex is always `unknown`. The
  implementation comment says `PostToolUse` also fires for failed commands, so
  success or error cannot be read from the event.

### Assistant message capture (opt-in)

- **Project behavior**: the `Stop` payload carries `last_assistant_message`. The
  implementation comment says this was verified on Codex CLI 0.154.0.
- **Project behavior**: capture is off by default. It needs two switches: the
  server setting `capture_assistant = true`, and `install-hooks
  --capture-assistant` on a platform with native hooks. The supported agents are
  Claude Code, Codex and OpenCode 2.
- **Project behavior**: a raw message above 64 KiB (65,536 bytes) or an empty
  message is treated as absent. The excerpt is capped at 2,000 bytes, cut on a
  UTF-8 boundary and sanitized. The raw field never reaches the local spool, the
  network or storage.

### Known gaps

- **Observed**: the desktop app ran the hooks only in scratch setups. A prompt
  typed in the desktop app with the installed hooks has not been run yet.
- **a confirmar**: whether the desktop app passes anything at thread start that
  disables hooks.

## MCP server setup

- **Project behavior**: `install-mcp --client codex` writes the server into
  `$CODEX_HOME/config.toml`, or `~/.codex/config.toml` when the variable is not
  set. The entry is a `[mcp_servers.<name>]` table.
- **Project behavior**: the default server address is
  `http://127.0.0.1:49374/mcp`. The table sets
  `default_tools_approval_mode = "approve"`. The implementation comment gives
  the reason: without it Codex asks before every call, and automatic capture
  becomes unusable. The write-capable tools are marked as destructive by the
  server itself, so they can be gated per tool.
- **Project behavior**: with a token, the table has an `http_headers` entry with
  `Authorization: Bearer <token>`. The implementation comments say that a
  literal bearer key and a plain `headers` table were not accepted for this
  transport. **a confirmar** against the current Codex schema.
- **Project behavior**: other servers in the same file are kept. The table is
  rewritten in block form.
- **a confirmar**: the desktop app's own steps for adding a server, and whether
  the desktop app lists servers with a slash command.
- **a confirmar**: the length limit that Codex applies to a server's
  instructions text.

## Transcript store

- **Project behavior**: the session root is `$CODEX_HOME/sessions` when the
  variable is set and not blank. Otherwise it is `~/.codex/sessions`.
- **Project behavior**: the reader accepts every `.jsonl` file under the root. It
  does not require a file name pattern. **a confirmar**: the file naming pattern
  Codex uses.
- **Project behavior**: the session header record carries the session identifier
  and the working directory. A session matches a project when the recorded
  working directory matches.
- **Project behavior**: reasoning content is not imported. The importer records
  that as a known loss. Synthetic context inserted by the harness is filtered.
- **Project behavior**: when the reader scans bucket directories, it stops after
  50,000 entries.
- **Project behavior**: the agent-assets backup includes the Codex config, the
  MCP file, skills and plugins under the `.codex` folder of the home directory.
- **a confirmar**: the role of the prompt history file that Codex keeps next to
  the sessions. The project does not read it.

## Pending checks

- **Observed, pending**: one prompt in the desktop app with trusted ai-memory
  hooks installed, then check whether a session with a desktop originator reaches
  the server.
- **a confirmar**: the full list of hook handler types the upstream product
  supports. The project installs command handlers only.

## Sources

- **Observed**: [verification-2026-10.md](verification-2026-10.md), the live
  verification of 2026-10-08.
- **Project context** (plain references, no links): the install guide, the MCP
  install guide, the lifecycle event ingestion note, the support matrix and the
  harness adapters note.
