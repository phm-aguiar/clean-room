# Marker file: `.ai-memory.toml`

The marker file declares which workspace, and optionally which project, a working directory belongs to. It sets a repository's scope without depending on the directory's name and without per-directory CLI commands.

This document covers where the file is found, how it is read, its keys, the inspection states, the home marker, the commands that read it, and the capture exclusions it can carry. Access control per project is in [design-per-project-authz.md](design-per-project-authz.md). The automatic isolation modes are in [auto-scope.md](auto-scope.md).

## Why it exists

Memory is namespaced by `(workspace, project)`. By default the workspace is `default` and the project is the basename of the working directory. That suits one developer with one repository per folder. It does not suit:

- **Consultancies** with one folder per client. Each client should get its own workspace.
- **Work, personal and open-source separation** for one developer.
- **Monorepos**, where all packages should share one project, or each package should get its own.

Static MCP clients also use the marker as the repository-owned source of explicit scope arguments. Declare both `workspace` and `project` for safe concurrent use. Static clients cannot attach the lifecycle-hook session id, so managed routing tells them to pass that pair on every project-scoped tool call. If either value is absent, the agent must take it from the operator or the server configuration and must not guess it from the checkout directory. Session-aware bridges keep automatic current-project routing.

## Discovery

### File name and location

The file is named exactly `.ai-memory.toml`, with the leading dot. It must be a regular file. A directory with that name is skipped.

It is looked up in the working directory (cwd) and in its ancestors. The walk stops at the boundary given below. The first marker found wins, so a closer marker overrides an outer one. Marker fields are never merged across files.

### Walk boundaries

| Where cwd is | The walk stops at |
|---|---|
| Inside `$HOME` | `$HOME`, inclusive |
| Outside `$HOME`, inside a git checkout | The checkout root, inclusive. The checkout root is the nearest ancestor that contains a `.git` entry (file or directory). |
| Outside `$HOME`, not inside a checkout | cwd only |
| `$HOME` is unset | The filesystem root |

Outside `$HOME`, the walk never reaches a marker placed above the checkout root, so such a checkout needs its own marker inside it. The home marker is the one exception, because it is read directly for any directory, see [Home marker](#home-marker).

### Two walks

The same file set is read by two walks that differ in one rule:

- **Scope walk.** Used for `workspace`, `project`, and the other settings. It skips markers that declare nothing except `[capture]`, so a subdirectory marker that only excludes paths does not reset the scope of its subtree. A marker counts as declaring something when it contains any of these keys anywhere in the file: `workspace`, `project`, `project_strategy`, `drop_subagent_captures`, `identity`, `identity_style`, `aliases`, `server`, `default_global`, `inject_on_session_start`, `max_chars`, `contribute`, `consume`. Key detection is line-based and ignores section headers.
- **Capture walk.** Used only for `[capture]`. It takes the nearest marker of any kind, including a capture-only marker. A capture-only marker therefore fully replaces the capture rules of its outer markers.

Lifecycle hooks always forward the working directory. When a marker is found they also forward its declared fields, see [Who reads the marker](#who-reads-the-marker).

## File format and reading

The reader is deliberately simple. The file is not parsed as a complete TOML document for scope fields.

- **Size.** `[capture]` and inspection read at most 64 KiB (65,536 bytes). A larger file is rejected there (see the inspection states). Scope fields of a local marker are read without that size limit. The home file has its own 64 KiB limit, see [Home marker](#home-marker).
- **Encoding.** The file must be UTF-8 text.
- **Scope fields and flags (line-based).** A key is read from a line that begins with the key name, then `=`. String keys must have a double-quoted value, and the value ends at the next double quote. Bare or unquoted string values are not read. Flags accept a quoted or bare token, and an inline `#` comment is stripped. Section headers are not tracked, so the first matching line wins wherever it sits in the file.
- **Strict TOML.** Used for `[capture]` and for the inspection state. Inspection also compares each string key with the strict reading. A key that appears under a section, or as a bare value, therefore shows up as a conflict. Resolution itself is tolerant: a file that is not valid TOML still provides scope fields to resolution, and only inspection reports it as `invalid`.
- **Truthy values.** `1`, `true`, `yes`, `on`, case-insensitive, quoted or bare. **Falsy values.** `0`, `false`, `no`, `off`, same rules.

## Keys

| Key | Where | Values | When absent | Notes |
|---|---|---|---|---|
| `workspace` | root | string | `default` | Optional. Pins the workspace for the marker's tree. |
| `project` | root | string | Derived name, see [Project name](#project-name) | Pins the project for the marker's tree. Wins over `project_strategy`. |
| `project_strategy` | root | `repo-root`, `repo_root`, `basename` | Install-wide default if one is set, otherwise `basename` | `repo-root` and `repo_root` are the same. Any other value is ignored for naming and reported as `conflicting` by inspection. Ignored when `project` is present. |
| `identity` | root | string | none | Explicit repository identity. Its ranking among identity sources is in [complementary-contracts.md](complementary-contracts.md). |
| `identity_style` | root | `path`, `host_path` | `path` for current static clients | Naming style for the identity. Omitted at the server boundary means `host_path`, kept for old clients. |
| `aliases` | root | array of strings | none | Up to 16 aliases, each up to 128 bytes. Sent with the identity. |
| `drop_subagent_captures` | root | truthy | off | Per-project opt-in. See [Subagent captures](#subagent-captures). |
| `server` | root | server profile name | install default | Selects a server profile. See [cli.md](cli.md). |
| `default_global` | `[recall]` | truthy | off | Makes an unscoped read default to all projects. See [Recall default](#recall-default). |
| `inject_on_session_start` | `[briefing]` | truthy | off | Injects a compiled project brief at session start. See [Session brief](#session-brief). |
| `max_chars` | `[briefing]` | integer | 4000 | Character budget for the whole brief. Clamped to 1500 through 20000. |
| `contribute` | `[profile]` | truthy or falsy | on | Whether this project contributes to the cross-project profile. See [cross-project-profile.md](cross-project-profile.md). |
| `consume` | `[profile]` | truthy or falsy | on | Whether this project reads the profile digest and union. |
| `ignore_paths` | `[capture]` | array of strings | none (inactive) | Capture exclusions. See [Capture exclusions](#capture-exclusions). |
| `[routes.*]` | home marker only | tables | none | See [Home marker](#home-marker). |

Section-style keys (`[recall]`, `[briefing]`, `[profile]`) are read by the same line-based rule, so `default_global = true` works at root or under its section.

### Name rules

`workspace` and `project` names are intended to match `^[a-z0-9][a-z0-9._-]*$`: lowercase ASCII letters and digits first, then dots, dashes and underscores. Home route names are checked against this pattern (see [Home marker](#home-marker)). The scope reader does not check local names. Where the server enforces the pattern is **a confirmar**.

### Recall default

With `[recall] default_global` on, an unscoped memory query from a session in this tree searches every project, and an unscoped recent-pages query returns the latest pages across projects. Each hit is annotated with its workspace and project. Explicit scope arguments override this default **a confirmar**. While it is active, unscoped queries return cross-project global hits instead of project hits. The reserved global preference pages still appear, annotated, among the global results. The setting is meant for meta-repositories that often need sibling-project context.

### Session brief

With `[briefing] inject_on_session_start` on, the session-start handoff fetch also returns the project's core pages (a short excerpt of each, deepened to a longer excerpt or the whole body while the budget lasts), plus the titles of recently updated pages without bodies. Core pages are pinned pages (outside the slots area), pages under the rules area, and pages under the slots area. Pinned pages come first, then ordering by path. Recent titles are capped at 10 and core pages at 24. In the session-start payload the brief comes after the profile digest, the pending handoff, the pending-handoff notice and the managed-workstream context, and before the inbox notice. It is not consumed, so it is recomposed on each opted-in session start, which is what a context clear in a host that re-runs the session-start hook needs. Only hosts whose session-start hook injects standard output as context receive it there.

The brief is scoped to the session's own `(workspace, project)`. The global reserved scope is not merged in, so a rule placed only in the global scope does not reach the brief. It stays reachable through on-demand queries. The brief is wrapped as untrusted history. Put instructions the agent must follow every turn in its own rules file, not in the brief.

The budget covers the whole brief, including headers and footers, and is measured in **UTF-8 bytes** (the key is named `max_chars` for historical reasons). It defaults to 4000, which is about four bytes per token. A missing or unparsable value gives the default; the value is clamped to 1500 through 20000. Bodies are never truncated: a page is shown as an excerpt (at most 256 bytes at the shortest depth, 500 at the next) or, when it fits whole, as its full body. Pages crowded out of the budget are listed by path, or counted when even the paths do not fit. Content is cut in this order: the recent titles and the omitted-page list are served from reserves taken before the pages are deepened; pages are first all covered by their shortest form in path order (pinned first), then privileged pages (pinned or slots) are deepened first, then the others, then whole bodies cheapest first; the pages at the end of the order are omitted first. The exact algorithm, layout and escaping are in [session-brief.md](session-brief.md).

Hosts that discard session-start output (Kimi Code at the time of writing) receive the brief on the first user prompt of the session, once per session. Their local delivery markers are created only for opted-in repositories and kept for the 512 most recent files of the delivery-marker directory. Re-briefing after a clear is not supported in that host: the marker survives a clear.

### Subagent captures

With `drop_subagent_captures` set to a truthy value, the server accepts but does not store the subagent-session captures of this project. Top-level sessions are always stored. The opt-in is per project, so one noisy project does not affect other projects on a shared server. The server interprets the value. Any other value, or absence, stores captures as usual.

### Project name

When the marker does not pin `project`, the project name is derived in this order:

1. The marker's `project`, when present.
2. For a checkout with a git remote, the identity derived from the remote (`upstream`, then `origin`). The client CLI uses the remote path name (host dropped, path segments joined with dashes) when `identity_style` is unset or `path`. Lifecycle hooks send the remote identity to the server. The full ranking of identity sources is in [complementary-contracts.md](complementary-contracts.md).
3. With `project_strategy = repo-root`, the name of the main repository root. Linked worktrees and subdirectories then share one project. The client CLI uses this only when there is no usable remote.
4. The basename of cwd.

The order used by lifecycle hooks after step 2 is **a confirmar**.

Without a git remote, the default is the basename of cwd. A marker that declares only `workspace` therefore does not force the project name to the folder name when the checkout has a remote.

### Install-wide default (no marker)

`project_strategy = "repo-root"` normally lives in a marker, which means placing a marker in (or above) every repository. To get repo-root naming for a whole install without a per-repository marker, bake it into the generated hooks at install time:

```sh
ai-memory install-hooks --apply --agent claude-code --project-strategy repo-root
```

Every session of that install then resolves its project from the main repository root. An agent that runs `mkdir sub && cd sub` and stays there does not create a project named `sub`.

This is install-time configuration written into the hook command and the generated plugins. It is not a runtime override set in the shell. The flag accepts `basename` (the default for new installs, bakes nothing) or `repo-root`. A later `install-hooks --apply` without the flag keeps the value already baked. Pass `--project-strategy basename` to remove it.

A marker's explicit `project_strategy` or `project` still wins over the install default.

The environment variable `AI_MEMORY_PROJECT_STRATEGY` provides a default for the same setting when a marker is found and does not set it. It does not apply without a marker.

## Inspection states

The inspection reports one state per directory. Checks run in this order:

1. `ignored`. `AI_MEMORY_IGNORE_MARKER` is truthy. No marker is consulted.
2. `invalid_home_routes`. No local scope marker applies, so the home file is consulted, and it cannot be used (see [Home marker](#home-marker)).
3. `absent` or `invalid`. No marker applies. A nearest file that is not valid TOML is `invalid`. A nearest file that cannot be read as text is treated as `absent`.
4. `unreadable`. The selected marker is larger than 64 KiB, or cannot be read in full.
5. `invalid`. The selected marker is not valid TOML.
6. `conflicting`. One of the checks below fails.
7. `valid`, or `home_identity_route` or `home_path_route` when a home route applied.

| State | Meaning |
|---|---|
| `ignored` | `AI_MEMORY_IGNORE_MARKER` is truthy. |
| `absent` | No marker applies. |
| `invalid` | The marker is not valid TOML. |
| `unreadable` | The marker is over 64 KiB, or cannot be read in full as UTF-8 text. |
| `invalid_home_routes` | The home file cannot be read, is over 64 KiB, is not UTF-8, has malformed route syntax, has a duplicate selector, has an ambiguous match, or has an invalid route value. Resolution fails closed for every directory that reaches the home file. |
| `conflicting` | A string key (`workspace`, `project`, `project_strategy`, `identity`, `identity_style`) reads differently by line and by strict TOML, or is not a string. Or `aliases` is invalid, `identity_style` is not `path` or `host_path`, or `project_strategy` is not one of the accepted values. |
| `valid` | The marker is consistent. A valid marker that declares nothing scope-related is valid with no scope. |
| `home_identity_route` | A home identity route applied. |
| `home_path_route` | A home path route applied. |

For the `conflicting` check, the comparison applies to local markers and to the home root. It does not apply to a home route that matched. Those checks still cover `aliases`, `identity_style`, and `project_strategy`.

**Resolution and inspection differ.** Resolution reads the scope fields line by line and does not require valid TOML. Inspection is strict. So a marker with a syntax error still sets scope during normal operation, while inspection reports it as `invalid`. Lifecycle capture uses the strict reading for `[capture]`, so a broken `[capture]` section makes the capture policy invalid.

## Home marker

A marker at `$HOME` is a special case.

- It is never a local declaration for the directories below it. Its root keys are used only as a fallback, the **home root**.
- The home root applies to any directory where no local scope marker is found. This includes directories outside `$HOME`, because the home file is read directly and not by walking.
- Its file has its own limits. It must be UTF-8 and at most 64 KiB. A file that breaks these limits, or whose routes are invalid, fails closed with `invalid_home_routes` for each directory that reaches it.
- When a local scope marker applies, the home file is not read for that directory.

### Route tables

Routes let one home file map several checkouts or paths to different scopes.

```toml
workspace = "personal"

[routes.identity."github.com/acme/api"]
route_workspace = "acme"
route_project = "api"
route_identity_style = "path"
route_aliases = ["acme-api"]

[routes.path."~/work/legacy"]
route_workspace = "legacy"
route_project = "legacy-tool"
```

Rules:

- Table headers must be exactly `[routes.identity."<selector>"]` or `[routes.path."<selector>"]`. Any other `routes` form is invalid.
- Each route requires `route_workspace` and `route_project`, each once, as non-empty quoted strings that match the name pattern and are at most 512 bytes. Optional keys: `route_identity_style` (`path` or `host_path`) and `route_aliases`. Any other `route_*` key is invalid. A `route_*` key outside a route table is invalid.
- An identity selector must be a normalized remote identity: a lowercase host, then at least one lowercase path segment. Allowed characters are letters, digits, dots, dashes, and in path segments also underscores.
- A path selector must be an absolute path: a POSIX path, a drive-letter path, or a UNC path. `~/` expands to the home directory. Backslashes are accepted. `.` and `..` are resolved without touching the file system. A selector that resolves to the root is invalid. Drive letters and UNC names compare without case.
- At most 64 routes in total. A selector is at most 512 bytes and may not contain control characters. A duplicate selector is invalid.

### Selecting a route

1. An identity route wins when the checkout's git remote identity equals its selector.
2. Otherwise the path route with the most matching components wins. A path matches by whole components, so `/a/b` matches `/a/b/c` but not `/a/bc`.
3. Two path routes at the same depth that both match make the selection ambiguous, which is invalid.
4. With no matching route, the home root applies.

A matched route replaces the scope fields `workspace`, `project`, `identity_style`, and `aliases`. It clears `project_strategy` and `identity` that the home root had set. A route without `route_identity_style` leaves the style unset.

## Who reads the marker

Two entry points read the marker:

- **Lifecycle hooks** forward the marker's fields on every event, so captures land in the declared scope. Forwarded fields are `workspace`, `project`, the project-source flag, `project_strategy`, `identity`, its source, `identity_style`, `aliases`, `drop_subagent_captures`, `default_global`, the two profile flags, and, for the handoff fetch at session start, `briefing` and `max_chars`. The profile flags are sent as `1` or `0` whenever a marker is in effect. A falsy value sends `0`, and an absent key sends `1`. With no marker, nothing is sent and the server keeps what it stored.
- **Client CLI commands** resolve `(workspace, project)` locally before calling the server. These include `run`, `bootstrap`, `search`, `read-page`, `write-page`, `lint`, `curator`, `embed`, `pending-writes`, `forget-sweep`, `auto-improve`, `purge-project`, `rename-project`, `move-project`, and `move-session` for its source side. Other commands that take a scope use the same rules.

Before this shared reading existed, only the hooks read the marker. A checkout that declared a workspace therefore had its captures stored in that workspace while every CLI command resolved into `default`.

### Resolution order for client commands

Each field resolves independently:

1. The explicit flag (`--workspace` or `--project`), when non-empty.
2. The nearest scope marker: `workspace`, and `project` or the derived project name described above.
3. The fallbacks: `default`, and the cwd-derived project name.

When the marker decides a field, the command prints one line to stderr. It names the resolved scope, which field or fields the marker decided, and the marker file:

```console
$ ai-memory search "scope resolver"
ai-memory: scope acme/api (workspace + project from /Users/dev/projects/acme/.ai-memory.toml)
```

An explicit flag that was honored is not reported as coming from the marker.

`AI_MEMORY_IGNORE_MARKER=1` skips the marker for one invocation. It restores the resolution from before the marker was read by client commands. It applies to client commands only. Lifecycle hooks still forward the marker on every event, so a command run with the variable set may resolve to a different scope than the captures around it. Use it for one-off reads, not to relocate a repository's memory.

### Migrating existing projects

Projects created under workspace `default` stay there. Move one with the CLI:

```sh
ai-memory move-project \
    --from-workspace default --project foo \
    --to-workspace movvia --confirm
```

Without `--confirm` the command is a dry run.

## Mid-session navigation: `[routing] mid_session`

The marker decides where a session starts. A long session can also move: an agent may `cd` into a scratch directory or into a sibling checkout. The server setting `[routing] mid_session` in the server configuration file decides how those mid-session events are attributed. It is runtime configuration, not a marker key.

```toml
[routing]
mid_session = "follow-cwd"   # default
# mid_session = "sticky"
```

- **`follow-cwd`** (default) re-resolves each mid-session event from its own working directory. A `cd` into a sibling checkout records those observations in that checkout's project. One session's raw record can then be split across two projects, while its session row and compiled page stay in the first.
- **`sticky`** keeps the session's project wherever the agent goes. This matches the model where a session has exactly one project. Choose it when one agent session means one project.

Two guarantees hold in both modes:

- **A marker still wins.** A marker that names a project is a deliberate rescope and is never overruled. Overrides are tagged with their source: a marker override or a derived override from the repository root. `sticky` can overrule a derived name but not a declared one. Clients that send no provenance have their overrides treated as authoritative.
- **Broad anchors never stick.** A session rooted at `/` or at `$HOME` is not a meaningful anchor. It never captures events beneath it, so one stray session started in `$HOME` cannot fold every project into one bucket.

A genuine workspace rescope, meaning a marker that names a different workspace than the session's own, disables sticking for that event.

Session-creating events are unaffected in both modes. Opening a session in a plain folder outside any repository still names the project after that folder.

Under `project_strategy = "repo-root"`, a mid-session event whose cwd is outside any git repository and outside any marker inherits the session's project. Agent scratch directories and data folders therefore do not create projects. Whether this applies in every host is **a confirmar**.

## Allowlist mode: the marker as an opt-in

By default the file is optional. A repository without one is still captured, and the marker only narrows what is taken. An install can invert that:

```bash
ai-memory install-hooks --apply --capture-mode allowlist
```

In allowlist mode, the presence of a `.ai-memory.toml` file on the capture walk is the opt-in. A repository without one emits no lifecycle event at all. Prompts, tool calls, and session boundaries are all dropped in the hook process before they reach the local spool or the network. No extra key is needed. An existing marker opts its repository in, whatever else it declares.

Forgetting the file has opposite costs in the two modes. By default a repository may be captured when you did not intend it. In allowlist mode a repository may stay silent when you wanted it captured. Choose the direction whose failure you would rather explain.

The mode is stored per install. A later bare `install-hooks --apply`, including an upgrade refresh, keeps it.

The mode is enforced by the native hook commands and by the generated plugins for the supported hosts. Raw script fallbacks, the Docker host wrapper, and `setup-agent` snippets post to the server without these checks, so allowlist mode does not gate them **a confirmar**.

## Capture exclusions

Use a `[capture]` table with `ignore_paths` to keep file-tool activity under matching paths out of capture:

```toml
[capture]
ignore_paths = ["private/**", "~/personal-notes/**"]
```

A repository that keeps decision records in its tree should exclude them too, for example `ignore_paths = ["docs/adr/**"]`. Without the exclusion, an agent's reads of those records are captured, and consolidation may compile copies that go stale when a record is superseded. See [Repo-native decision records](usage.md#repo-native-decision-records).

### Parsing and failure

- The nearest marker is authoritative. Sections do not merge. A missing `[capture]` table, or an empty `ignore_paths`, is inactive and keeps the current behavior.
- `[capture]` accepts only `ignore_paths`. An unknown key, a non-array value, an entry that is not a string, invalid TOML, text that is not UTF-8, or a file over 64 KiB makes the **whole** capture policy invalid. Nothing is applied partially.
- A pattern that fails validation makes the whole policy invalid too.

### Pattern rules

- A pattern matches an entire lexically normalized path, not a substring.
- `*` matches within one path component. `?` matches one character that is not a separator. `**` crosses directories.
- Relative patterns are rooted at the marker's directory. Absolute patterns are used as they are. `~/` expands to the home directory.
- Relative paths in file-tool events are resolved from the event's cwd.
- Rejected in patterns: the characters `! { } [ ] ( ) | ^ $ %`, the sequence `${`, three or more consecutive `*`, a `..` segment, a `~` not followed by `/`, and drive-relative paths such as `C:foo`. When the marker's base path is a Windows path (`C:/...` or `//server/share/...`), POSIX root patterns (`/foo`) are also rejected as invalid policy because they have no drive letter.
- Matching is case-sensitive on POSIX paths. Windows drive and UNC paths match ASCII case-insensitively. On a macOS host everything is compared case-insensitively after both the patterns and the candidates are folded (Unicode NFC, then full Unicode lowercase), as macOS's default filesystem does.
- **Windows path normalization before matching:**
  - Strips PowerShell provider prefixes (`Microsoft.PowerShell.Core\FileSystem::` and `FileSystem::`, case-insensitively).
  - Normalizes extended-length and device drive prefixes (`\\?\C:\...`, `\\.\C:\...`, `\??\C:\...`) and local administrative shares (`\\localhost\C$\...` in any case and `\\127.0.0.1\C$\...`; an IPv6 loopback host is **not** recognized) to `C:/...`, and `\\?\UNC\server\share\...` to `//server/share/...`. Other `\\?\`, `\\.\`, or `\??\` device paths and drive-relative paths (`C:foo`) cannot be placed and force `MetadataOnly`.
  - Strips NTFS Alternate Data Stream suffixes (`file.txt:secret:$DATA` -> `file.txt`) and trailing dots or spaces from each non-glob path segment (`secret. /` -> `secret/`).
  - Generates dual candidate spellings for MSYS2/Git Bash (`/c/Users/...`) and Cygwin (`/cygdrive/c/Users/...`) paths so both POSIX and `C:/Users/...` forms are tested against patterns.
- Matching is lexical. Symbolic links, junctions, bind mounts, and path aliases are not resolved. Add each relevant visible alias as its own pattern.

### Bounds

| Bound | Limit |
|---|---|
| Patterns per marker | 128 |
| Characters per pattern | 1,024 |
| Direct path candidates per event | 32 |
| Characters per candidate | 4,096 |
| Pattern and candidate comparisons per event | 1,000,000 |

Exhausting the comparison budget fails closed. For file tools the event becomes metadata-only. For shell tools the event is dropped.

### Decisions

With an active policy, the decision depends on the tool family:

- **File tools.** Only explicit path fields and documented direct arrays are read. Multi-edit entries and multi-read operations are included. If any candidate matches, the whole event is dropped locally before it reaches the spool, the queue, the network, transport logs, or server storage. A recognized file tool with missing, malformed, or unsupported candidates, or targeting a harness output spool file (such as `~/.claude/projects/.../tool-results/<id>.txt`), becomes **metadata-only**.
- **Search and list tools.** Dropped conservatively.
- **Shell tools.** The command is split into words (stripping PowerShell backtick `` ` `` and `cmd.exe` caret `^` escape characters) and matched. A match, or an exhausted budget, drops the event. Output-reader tools (`bashoutput`, `taskoutput`, `write_stdin`, `process`, `read_bash`, `write_bash`, `read_powershell`, `write_powershell`) and inline-code execution tools (`execute_code`) become **metadata-only** when policy is active. Free-form text is not otherwise parsed. Patch text is not parsed for paths.
- **Other tools.** Unchanged.

With an invalid policy, file tools, search/list tools and shell tools (including output-reader tools) become metadata-only. Other tools are unchanged. The full decision engine (candidate extraction, shell word splitting, nested scripts, other markers a call touches, and the server's re-inspection) is specified in [lifecycle-event-ingestion.md](lifecycle-event-ingestion.md), section 4.4a.

Metadata-only events keep only bounded routing, tool, and decision metadata. They contain no paths, patterns, arguments, output, errors, titles, or nested payload.

Prompts, assistant text, notifications, and quoted content cannot be attributed to a path, so this policy does not filter them.

This is a lexical capture boundary, not complete data-loss prevention. Excluding content before transport matters because it then cannot reach observations, session pages, handoffs, reviewer requests, proposals, or logs.

### Supported integrations and refresh

Capture policy v1 is enforced by the native hook commands and by the generated plugins for OpenCode, OMP, Pi, and OpenClaw. Local installers use native commands where the host supports them. Legacy script hooks and remote-only or container script bundles do not enforce the policy **a confirmar**.

Reinstall the hooks, or refresh the generated plugins, after upgrading. Existing hooks keep their prior behavior until then. New clients stay safe with old servers because stripping and dropping happen on the client. Old clients talking to new servers keep old behavior and cannot enforce a host-only marker policy. The policy adds no tool and no database migration.

### Checking a decision locally

`ai-memory hook --event ... --agent ... --check-capture` reads one JSON payload from standard input. It does no spool, queue, drain, network, or handoff work. It prints a JSON object with 15 fixed fields (`version`, `capture_mode`, `marker_present`, `admits_capture`, `policy_admits_capture`, `event_admits_capture`, `policy_state`, `tool_family`, `path_count`, `disposition` [which reports `"drop"` both when dropped and when reduced to metadata-only], `extraction_state`, `scope` [`workspace`, `project`, `project_strategy`, `identity`, `identity_style`, `aliases`, `drop_subagent_captures`, `default_global`], `scope_resolution`, `server_profile`, `server_resolution`) plus optional `external_capture: true`. It never prints paths, patterns, or payload content.

```bash
printf '%s\n' '{"session_id":"demo","cwd":"/example/workspace","tool_name":"Edit","tool_input":{"path":"docs/example.md"}}' \
  | ai-memory hook --event post-tool-use --agent claude-code \
      --server-url http://127.0.0.1:49374 --check-capture
```

### What is kept from tool events

Supported tool events (Claude Code, OpenCode, Pi, OMP, Antigravity) keep the canonical tool family, a validated call ID (`tool_use_id` / `callID` of 1 to 128 ASCII characters `[A-Za-z0-9_.-]`) when the host's documented schema provides one, and a post-tool outcome class.

- Pre-tool events require `tool_input` (or `args`) to classify as `"tool <family>"`; without it, the title falls back to `"pre-tool-use"` and the body is empty. They keep no command, arguments, paths, input bodies, or arbitrary tool names.
- Post-tool events add `outcome: <success|error|unknown>` and, when `tool_family` is not `"unknown"`, a `---` separator followed by the tool-response or error excerpt (or `(no output captured)`). When `tool_family` is `"unknown"`, only the header lines (`tool_family`, optional `tool_call_id`, `outcome`) are emitted without `---` or output. The full rendered body is capped at 2,000 UTF-8-safe bytes.
- For Antigravity, a successful file edit falls back to the replacement or written content in the tool arguments, because its hook payload has no output field. A failed edit keeps the error instead. The fallback is discarded with any event rejected by a capture exclusion.
- Association between a pre-tool and a post-tool event is only by matching call IDs.

Other event bodies: the user prompt is stored unless the Claude Code hooks were installed with `--no-capture-prompts`. A notification stores its message. A post-compaction event stores its summary. Other events have empty bodies unless explicitly supported.

Assistant-message capture is disabled by default and never persisted. It is available only through the explicit double opt-in: the client installs with `--capture-assistant`, and the server sets `capture_assistant`. The excerpt is sanitized on both sides and capped. The marker file cannot narrow it, because assistant text is not path-attributable. Capture exclusions are evaluated only where a path schema is proven.

## Examples

These examples assume no git remote. With a remote, the project name follows the rules in [Project name](#project-name).

### Multi-client

```text
~/projects/movvia/.ai-memory.toml     -> workspace = "movvia"
~/projects/cliente-x/.ai-memory.toml  -> workspace = "cliente-x"
~/personal/.ai-memory.toml            -> workspace = "personal"
```

- `~/projects/movvia/pe-api-core` resolves to workspace `movvia`, project `pe-api-core`.
- `~/projects/cliente-x/api` resolves to workspace `cliente-x`, project `api`.
- `~/personal/blog` resolves to workspace `personal`, project `blog`.

### Monorepo with grouped packages

```text
~/projects/movvia/.ai-memory.toml              -> workspace = "movvia"
~/projects/movvia/pe-portais/.ai-memory.toml   -> workspace = "movvia"
                                                  project   = "pe-portais"
```

- `~/projects/movvia/pe/pe-api-core` resolves to workspace `movvia`, project `pe-api-core`.
- `~/projects/movvia/pe-portais/apps/web` resolves to workspace `movvia`, project `pe-portais`. The closer marker wins.

### Worktrees and repository-root identity

```text
~/projects/.ai-memory.toml -> workspace        = "oss"
                              project_strategy = "repo-root"
```

- `~/projects/tool` resolves to workspace `oss`, project `tool`.
- `~/projects/tool/packages/cli` resolves to workspace `oss`, project `tool`.
- `~/projects/tool-feature-branch` resolves to workspace `oss`, project `tool`, when it is a linked worktree of the same repository with no remote of its own.

Worktrees placed outside the main tree have no marker ancestor of their own. The main repository is found through the shared git directory of the worktree, so the marker above is still used. Put the marker on the walk path from the worktree, commonly one `~/.ai-memory.toml`, to select the strategy. Without `project_strategy = "repo-root"`, these paths resolve by their own directory basename.

### Single workspace, no per-repository overrides

```text
~/.ai-memory.toml -> workspace = "home"
```

Every directory under `$HOME` lands in workspace `home`, with the project derived from its own basename. This is useful to leave the `default` workspace entirely.

## Troubleshooting

If a marker is not picked up:

1. The file is named exactly `.ai-memory.toml`, with the leading dot.
2. The file is in an ancestor of the working directory. It is not a sibling or a descendant.
3. No closer marker overrides it. List all markers in a tree with `find <tree> -maxdepth 5 -name '.ai-memory.toml'`.
4. The workspace and project values match the name pattern, and string values are double-quoted.
5. If `project_strategy` is used, its value is exactly `repo-root` or `repo_root`.
6. Check the state the marker is reported in, see [Inspection states](#inspection-states). The `doctor` command reports the capture policy state.

Hook scripts run fire-and-forget and do not log on success. To see what a hook sends, run it by hand with the shell trace option on (`set -x`):

```sh
printf '{"cwd":"%s"}' "$PWD" \
  | sh ~/.local/share/ai-memory/hooks/claude-code/post-tool-use.sh
```

When the marker is read, the request carries `workspace=` and `project=` parameters with the declared values.

## What the marker file does not do

- It has no glob patterns for scope. The walk follows literal ancestry only.
- It does not merge ancestor markers. The closest one wins.
- It does not migrate projects that already live in workspace `default`.
- It does not collapse worktrees or subdirectories into one project on its own. They share a project only when `project_strategy = "repo-root"` is set, per marker or install-wide, or when they share a git remote identity.
- It is not a runtime override for the server address or the auth token. Use the environment variables for those, such as `AI_MEMORY_AUTH_TOKEN`. A repo-root default can be baked into an install with `install-hooks --project-strategy repo-root`, but that is install-time configuration.
- It does not reach outside the trust boundary. The walk stops at `$HOME`. A checkout outside `$HOME` stops at its own root and needs a marker inside it. A non-repository directory outside `$HOME` needs a marker in its exact directory.
