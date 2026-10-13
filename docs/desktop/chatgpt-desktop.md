# ChatGPT desktop (research notes)

> Research for RFC #878. The phrase "ChatGPT desktop" covers two surfaces: a
> desktop app that hosts chat and a coding agent, and the hosted chat
> interface. This project has no integration with the desktop chat app itself.
> Its only verifiable code concerns the coding agent of the same vendor (Codex
> CLI), which shares configuration and transcript locations with the app.
> Everything about the third-party product is marked **a confirmar** unless the
> project code shows it.

## 1. Evidence status

| Claim | Status |
|---|---|
| One desktop app hosts both Chat and Codex modes | a confirmar |
| Chat/Work toggle in the composer | a confirmar |
| The app reads the same MCP and hook configuration as the CLI | a confirmar |
| Codex CLI: hooks, MCP, and transcripts (sections 2 to 4) | verified in project code |
| Hosted chat accepts only remote MCP servers exposed as plugins | a confirmar |
| Linux availability of the official app | a confirmar |

## 2. Codex-side integration (verified in project code)

### 2.1 Hooks

- The project writes lifecycle hooks to a standalone JSON file. The location is
  `$CODEX_HOME/hooks.json` when `CODEX_HOME` is set and not blank; otherwise
  `~/.codex/hooks.json`.
- The file has a `hooks` object keyed by lifecycle event (for example
  `SessionStart`). Each event holds a list of entries with an empty `matcher`
  and `command` hook items. The project comment says this shape matches the
  CLI's upstream hook configuration; the upstream comparison is a confirmar.
- The project comment states that Codex does not run new hooks until the user
  accepts them in its interface, or until a bypass flag is set. This is
  a confirmar for the desktop app and for Chat mode.
- Hooks written to the default path are ignored by a Codex configured with
  another `CODEX_HOME`. The project resolves the same path for hooks and MCP
  config to avoid that split.

### 2.2 MCP server registration

- The project writes the MCP server entry to `$CODEX_HOME/config.toml`, or to
  `~/.codex/config.toml` when the variable is unset or blank.
- The entry uses a `url` (streamable HTTP transport). It sets
  `default_tools_approval_mode = "approve"`, so the server's tools run without
  per-call prompts.
- When an auth token is supplied, the entry sends an `Authorization` header
  with the bearer value. The alternative is to read the token from the
  environment variable `AI_MEMORY_AUTH_TOKEN`, so the literal token does not
  appear in the file.
- Whether the desktop app reads this file is a confirmar.

### 2.3 Transcripts and session identity

- Session transcripts for the CLI are searched under `$CODEX_HOME/sessions`, or
  `~/.codex/sessions` when the variable is unset. The files are JSON Lines.
- Hook payloads identify the session by the `session_id` field. The older
  `sessionId` field is also accepted. A non-empty `session_id` query parameter
  is the fallback when the body has neither.
- Whether desktop-app sessions write to the same location is a confirmar.

## 3. Chat surfaces (a confirmar)

- Hosted chat can use remote MCP servers supplied by plugins; local
  configuration files are not read. Authentication is OAuth-based. Stdio
  servers are not supported in hosted chat. All four points are a confirmar.
- Plugins and connectors are administered per workspace for team plans. a confirmar.
- Consequence for ai-memory, as a working hypothesis: a hosted chat session can
  reach ai-memory only through a remote MCP server that the user runs or
  hosts. A server bound to localhost would not be reachable. Tunnel options
  for exposing a local server are not researched here. a confirmar.

## 4. Capture and deduplication

- **Chat mode:** the project has no hook or transcript code for this surface.
  Memory writes depend on the model deciding to call the memory tools. a confirmar.
- **Codex mode:** the project already handles the Codex agent kind for hooks and
  transcripts. Whether desktop-app Codex sessions share the same identity
  (`session_id` under the Codex transcript root) is a confirmar.
- Keying captured data by agent kind and `session_id` is the intended model,
  but the project's schema does not show a composite uniqueness rule for it.
  a confirmar.
- No documented handoff between the CLI and the desktop app. a confirmar.

## 5. Linux

The Linux status of the official app is a confirmar. Community repackaging of
the Codex desktop app is covered in [codex-desktop.md](codex-desktop.md).

## 6. Sources

Vendor documentation consulted in earlier research (not re-verified in this
repository): the desktop app page, the Codex MCP extension page, the Linux app
page, the plugins page, and the secure MCP tunnel guide. Every claim marked
**a confirmar** depends on those pages. External links were removed from this
document.
