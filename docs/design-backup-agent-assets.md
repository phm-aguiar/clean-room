# Backup and restore of AI agent assets

> This document describes how host agent assets (MCP configuration files, skills,
> plugins, and instruction files) are backed up and restored. Everything outside
> the [Not implemented](#not-implemented) section describes current behavior.
> Items in that section are proposals with no corresponding behavior.

## 1. Context

The [server backup](backup.md) snapshots the database, the wiki, and the server
configuration. It does not cover the configuration of the AI agents that run on
a client machine: MCP server definitions, skills, plugins, and instruction files.
Without a dedicated command, moving to a new machine meant configuring every
agent again by hand.

Agent asset backup runs on the client, against local files. It does not need the
server to be running.

## 2. Coverage by agent

Global paths are relative to the user's home directory. Project paths are
relative to the directory where the command runs. The name in parentheses is the
agent name written to the archive.

| Agent (archive name) | MCP configuration | Skills | Plugins | Instructions |
| :--- | :--- | :--- | :--- | :--- |
| Claude Code (`claude-code`) | `~/.claude/settings.json`, `~/.claude.json` | `~/.claude/skills/`; project `.claude/skills/` | `~/.claude/plugins/` | Project `CLAUDE.md` |
| Claude Desktop (`claude-desktop`) | Platform config file (macOS and Windows only, see note) | — | — | — |
| Codex (`codex`) | `~/.codex/config.toml`, `~/.codex/mcp.json` | `~/.agents/skills/`, `~/.codex/skills/`; project `.agents/skills/` | `~/.codex/plugins/` | Project `AGENTS.md` |
| Antigravity CLI (`antigravity-cli`) | `~/.gemini/config/mcp_config.json`, `~/.gemini/antigravity-cli/mcp_config.json` | `~/.gemini/antigravity-cli/skills/`; project `.gemini/skills/` | `~/.gemini/antigravity-cli/plugins/` | — |
| Gemini CLI (`gemini-cli`) | `~/.gemini/settings.json` | `~/.gemini/skills/` | — | Project `GEMINI.md` |
| Cursor (`cursor`) | `~/.cursor/mcp.json`; project `.cursor/mcp.json` | — | — | Project `.cursorrules`, `.cursor/rules/` |
| OpenCode (`open-code`) | `~/.config/opencode/opencode.json`, `~/.config/opencode/opencode.jsonc`; project `opencode.json`, `opencode.jsonc` | — | `~/.config/opencode/plugins/` | — |
| Devin (`devin`) | `~/.devin/config.json` | `~/.devin/skills/`; project `.devin/skills/` | — | — |
| Grok (`grok`) | `~/.grok/config.toml`; project `.grok/config.toml` | `~/.grok/skills/`; project `.grok/skills/` | — | — |
| Kiro CLI (`kiro-cli`) | `~/.kiro/settings/mcp.json` | `~/.kiro/agents/` (every file in the tree) | — | — |
| OpenClaw (`openclaw`) | `~/.openclaw/config.json` | — | `~/.openclaw/extensions/` | — |
| Command Code (`command-code`) | `~/.commandcode/mcp.json` | — | — | — |
| Kimi Code (`kimi-code`) | `~/.kimi/mcp.json` | `~/.kimi/skills/` | — | — |
| VS Code / Copilot (`other`) | Project `.vscode/mcp.json` | — | — | — |

Notes:

- **Claude Code relocation.** When `CLAUDE_CONFIG_DIR` is set and not blank, the
  `.claude/` assets (settings, skills, plugins) are read from that directory.
  `~/.claude.json` is always read from the home directory.
- **Claude Desktop.** Backup collects the config file only on macOS and Windows.
  On Windows, a packaged (MSIX) install is detected under the local application
  data directory; otherwise the roaming application data directory is used.
  Restore resolves the same platform path on any operating system.
  Windows resolution, exactly (audited 2026-10-25): the candidates are the
  directories under `%LOCALAPPDATA%\Packages` whose name starts with `Claude_`
  (case-insensitive), sorted; each candidate's config is
  `<package>\LocalCache\Roaming\Claude\claude_desktop_config.json`. If exactly one
  candidate already has that file, it wins. If none has it and there is exactly one
  candidate, that candidate's path is used. If there are no candidates, the roaming
  path `%APPDATA%\Claude\claude_desktop_config.json` is used. Any other combination
  (several candidates and not exactly one with a config) is an error that names the
  packages and asks for an explicit config file; because Claude Desktop is resolved
  while candidates are collected, this error **aborts the whole backup**, not just
  the Claude Desktop entry. Restore uses the same resolution (on Linux, the
  `$XDG_CONFIG_HOME/Claude/` rule of the MCP installer).
- **Other agents.** Their paths are fixed under the home directory. Relocation
  variables of those agents are not consulted.
- **Home directory override.** When `AI_MEMORY_HOME` is set and not blank, it
  replaces the home directory for all path resolution in this feature.
- **Instructions** are collected only at project scope.

## 3. Commands

### 3.1 `backup-agents`

```bash
# Back up all detected agents into an archive
ai-memory backup-agents -o agent-assets.tar.gz

# Filter agents and scope
ai-memory backup-agents --agents claude,codex,antigravity --scope both -o backup.tar.gz

# Keep MCP configuration secrets (warns; archive is owner-only on Unix)
ai-memory backup-agents -o backup.tar.gz --include-secrets

# List what would be backed up, without writing an archive
ai-memory backup-agents --dry-run
```

| Option | Default | Meaning |
| :--- | :--- | :--- |
| `-o`, `--to` | Required | Destination `.tar.gz` file |
| `--agents` | All detected | Comma-separated agent names |
| `--scope` | `both` | `global`, `project`, or `both` |
| `--include-secrets` | Off | Disables MCP configuration redaction (section 5) |
| `--dry-run` | Off | Lists candidate assets and writes no archive |

Behavior:

1. Candidates come from the table in section 2, limited by `--agents` and
   `--scope`. Only regular files count. Symbolic links are never followed, neither
   at the asset root nor inside an asset folder.
2. Agent names are case-insensitive, and `_` is treated as `-`. Matching is exact,
   never a substring match. Aliases are accepted, for example `claude` (Claude
   Code), `agy` or `antigravity` (Antigravity CLI), `gemini` (Gemini CLI),
   `kiro` (Kiro CLI), `kimi` (Kimi Code), `cmd` or `commandcode` (Command Code),
   and `opencode` or `opencode-v2` (OpenCode). `claude` does not select Claude
   Desktop; use `claude-desktop`. VS Code entries match `other`, `vscode`,
   `vs-code`, `copilot`, or `vscode-copilot`.
3. Inside asset folders, subdirectories are skipped when their name starts with
   `.` or is one of `cache`, `node_modules`, `target`, `venv`, `__pycache__`, or
   `site-packages`. Files named `.DS_Store` or ending in `.pyc` or `.pyo` are
   skipped. The top-level asset folder itself is not subject to these rules, and
   the walk has no depth limit.
4. The destination archive is excluded from the candidates if it is one of them.
5. If there are no candidates, the command prints
   `No AI agent assets detected for backup.`, writes no archive, and exits
   successfully.
6. `--dry-run` prints each candidate's agent, asset kind, scope, and source path.
   It writes no archive.
7. Each asset must be 128 MiB or smaller. A larger file aborts the whole backup.
8. Entries are written sorted by archive path, with zeroed timestamps, and the
   manifest is written last. Global assets go under a group name, and project
   assets go under `project/` followed by a group name. Group names are short
   labels that may differ from the agent name. For example, project skills for
   Codex are under `project/agents/`.
9. The archive is first written to a temporary file in the destination directory
   and created with owner-only permissions (`0600`) on Unix. After the stream is
   complete and synced, the file replaces the destination. A failed run leaves a
   previous archive intact. A missing parent directory of the destination is
   created.
10. On success, the command prints the number of assets and the archive size.
    With `--include-secrets`, it also prints a warning to the error output.

### 3.2 `restore-agents`

```bash
# Inspect an archive (dry run, default)
ai-memory restore-agents -i backup.tar.gz

# Apply the reviewed plan
ai-memory restore-agents -i backup.tar.gz --apply

# Apply only Codex global assets, overwriting files that differ
ai-memory restore-agents -i backup.tar.gz --agents codex --scope global --apply --force
```

| Option | Default | Meaning |
| :--- | :--- | :--- |
| `-i`, `--from` | Required | Source `.tar.gz` archive |
| `--agents` | All in archive | Comma-separated agent names, same rules as `backup-agents` |
| `--scope` | `both` | `global`, `project`, or `both` |
| `--apply` | Off | Writes files; without it the command only previews |
| `--force` | Off | Overwrites existing files whose content differs |

Behavior:

1. The archive must be an existing file. Its contents are staged in a private
   temporary area before any target is touched. Structural and size checks are in
   section 5.
2. The manifest version must be `1`, the only supported version.
3. Every manifest entry is validated, including entries that the filters exclude.
   Validation covers the archive path and target path rules, uniqueness of archive
   paths and targets, the presence and format of the SHA-256 value, a match
   between the SHA-256 value and the stored body, permission bits no higher than
   `0777`, and the absence of symbolic links on the target path.
4. Stored bodies that the manifest does not reference are refused.
5. Filters are applied after validation. Only selected entries are planned.
6. For each selected entry, if the destination exists and is not a regular file,
   the command refuses. If the destination exists with the same size and SHA-256
   value, the entry counts as identical and is not written. Permissions are not
   compared for identical files.
7. If nothing needs writing, the command prints either
   `All N selected assets are already identical on host. Nothing to restore.` or
   `No archive assets matched the selected agent and scope filters.`, and exits
   successfully.
8. Without `--apply`, the command prints a preview with `[CREATE]` or
   `[OVERWRITE]` for each planned entry, the agent, the asset kind, the size, and
   the target path. It also prints the number of identical files skipped and a
   warning that skills, plugins, and instructions are active content. It writes
   nothing.
9. With `--apply`, each planned entry is processed in order:
   - If the target exists and `--force` is not set, the entry is skipped and
     reported as `SKIP`.
   - Otherwise, missing parent directories are created. If the target exists, a
     copy named `<target>.bak-<microseconds>` is made beside it, with `-N` appended
     if that name is taken. The new content is then written atomically. On Unix,
     the permission bits from the manifest are applied if the entry has a mode.
   - If any write fails, writes already made in this run are rolled back in
     reverse order. Overwritten files are restored from their copies, and created
     files are removed. The timestamped copies remain on disk.
10. The summary reports the counts of created, updated, identical, and skipped
    files. If the archive contains sanitized configurations, the command notes
    that some MCP values appear as `[REDACTED:<kind>]` and must be replaced with
    valid credentials.

Target resolution:

- **Project entries** resolve relative to the current working directory.
- **Global entries** resolve relative to the home directory, with these
  exceptions: `.claude/` entries go to the `CLAUDE_CONFIG_DIR` directory when it is
  set; `.config/opencode/` entries go to the OpenCode config directory; the Claude
  Desktop entry must be exactly `.claude/claude_desktop_config.json` and goes to
  the Claude Desktop platform path on the restoring machine.
- **Allowed global targets** start with one of `.claude/`, `.codex/`, `.agents/`,
  `.gemini/`, `.cursor/`, `.config/opencode/`, `.devin/`, `.grok/`, `.kiro/`,
  `.openclaw/`, `.commandcode/`, or `.kimi/`, or are exactly `.claude.json`.
- **Allowed project targets** are exactly `CLAUDE.md`, `AGENTS.md`, `GEMINI.md`,
  `.cursorrules`, `opencode.json`, `opencode.jsonc`, `.vscode/mcp.json`,
  `.cursor/mcp.json`, or `.grok/config.toml`, or start with one of
  `.claude/skills/`, `.agents/skills/`, `.cursor/rules/`, `.gemini/skills/`,
  `.devin/skills/`, or `.grok/skills/`.

## 4. Archive format

The archive is a gzip-compressed tar file. Each asset body is stored under its
archive path. A `manifest.json` file at the root of the archive describes every
body. It is the last member.

```json
{
  "version": 1,
  "created_at": "2026-09-28T14:30:00Z",
  "host": {
    "os": "macos",
    "arch": "aarch64"
  },
  "sanitized": true,
  "entries": [
    {
      "agent": "claude-code",
      "asset_kind": "mcp-config",
      "scope": "global",
      "archive_path": "claude/mcp/settings.json",
      "target_relative": ".claude/settings.json",
      "sanitized": true,
      "sha256": "3b0c4f1e9a7d2c58e61f0b9d4a7c3e2f1d0b9a8c7e6f5d4c3b2a19087f6e5d4c",
      "mode": 420
    },
    {
      "agent": "claude-code",
      "asset_kind": "skill",
      "scope": "global",
      "archive_path": "claude/skills/custom-skill/SKILL.md",
      "target_relative": ".claude/skills/custom-skill/SKILL.md",
      "sanitized": false,
      "sha256": "3b0c4f1e9a7d2c58e61f0b9d4a7c3e2f1d0b9a8c7e6f5d4c3b2a19087f6e5d4c",
      "mode": 420
    },
    {
      "agent": "codex",
      "asset_kind": "mcp-config",
      "scope": "global",
      "archive_path": "codex/config.toml",
      "target_relative": ".codex/config.toml",
      "sanitized": false,
      "sha256": "3b0c4f1e9a7d2c58e61f0b9d4a7c3e2f1d0b9a8c7e6f5d4c3b2a19087f6e5d4c",
      "mode": 384
    }
  ]
}
```

The hash values above are placeholders for illustration.

Manifest fields:

| Field | Meaning |
| :--- | :--- |
| `version` | Schema version. Only `1` is supported. |
| `created_at` | UTC creation time, in ISO 8601 form. Fractional seconds may appear. |
| `host.os`, `host.arch` | Operating system and architecture names of the source host. |
| `host.home_dir` | Part of the schema, but this version never writes it. |
| `sanitized` | `true` if any entry's text changed because of redaction. |
| `entries` | One object per asset, described below. |

Entry fields:

| Field | Meaning |
| :--- | :--- |
| `agent` | Agent name (section 2). VS Code entries use `other`. |
| `asset_kind` | `mcp-config`, `skill`, `plugin`, or `instruction`. |
| `scope` | `global` or `project`. |
| `archive_path` | Path of the body inside the archive. |
| `target_relative` | Destination relative to the home directory or the project directory. Restore uses this field, not `archive_path`. |
| `sanitized` | `true` if redaction changed this entry's text. Defaults to `false`. |
| `sha256` | SHA-256 of the stored body, as 64 lowercase hexadecimal characters. The hash is taken after redaction. |
| `mode` | Permission bits in decimal, with set-id and sticky bits removed. Written on Unix only. |

## 5. Security and invariants

1. **Secret redaction (default).** For MCP configuration assets whose content is
   UTF-8 text, the project's default sanitizer runs. It replaces known token
   formats (such as vendor API keys, bearer tokens, and private key blocks) and
   environment-style assignments whose names end in `KEY`, `TOKEN`, `SECRET`,
   `PASSWORD`, or similar with `[REDACTED:<kind>]`.
   - Non-UTF-8 MCP files are copied unchanged and marked as not sanitized, even in
     default mode.
   - Skills, plugins, and instruction files are always copied verbatim. They may
     contain credentials.
   - `--include-secrets` disables redaction for MCP files.
   - Every archive is created with owner-only permissions (`0600`) on Unix, in both
     modes, because verbatim files may contain credentials. On other platforms,
     protection depends on the native file permissions.
2. **Symbolic links.** Backup never follows symbolic links. Restore refuses to
   write when the target, or any component between the approved base directory and
   the target, is a symbolic link.
3. **Path traversal.** Restore accepts only regular-file entries with valid UTF-8
   paths. Paths must be relative and must not contain `..`, `.`, empty components,
   backslashes, NUL bytes, or drive prefixes. Destinations must match the allowlist
   in section 3.2. Destinations that are not regular files are refused.
4. **Bounded input (restore).** Restore accepts at most 20,000 entries, a manifest
   of at most 4 MiB, assets of at most 128 MiB each, and at most 512 MiB of total
   expanded content. Sizes are read from entry headers and checked before content
   is staged, and the staged bytes must match the declared size. Backup enforces
   only the 128 MiB per-asset limit, so an archive can be created that restore
   will refuse.
5. **Integrity.** Each manifest entry must reference exactly one body with a
   well-formed SHA-256 value. Duplicate archive paths or destinations, missing
   bodies, unlisted bodies, and checksum mismatches are refused before any write.
   SHA-256 detects corruption and internal inconsistency. It does not authenticate
   the author of an archive.
6. **Atomic writes and rollback.** Target files are written to a temporary file in
   the same directory, synced, and renamed over the target. The parent directory is
   synced on a best-effort basis. Overwritten files keep a timestamped copy named
   `<target>.bak-<microseconds>`. If a later write in the same run fails, earlier
   writes are rolled back.
7. **Existing files.** An existing file whose content differs is not replaced
   without `--force`. It is reported as skipped. The command makes no distinction
   between managed and custom files.
8. **Permissions.** On Unix, restored files receive the permission bits from the
   manifest. Set-id and sticky bits are never applied, and an entry with bits above
   `0777` is refused.
9. **Active content.** Restore is a dry run by default. It prints a warning that
   skills, plugins, and instructions can contain executable code or instructions
   for the model. Use `--apply` only for archives whose source and destinations
   you trust. A matching SHA-256 value is not a signature.

## 6. Tests

Automated tests cover:

- Round trip: back up, verify the manifest, extract into a temporary directory,
  and compare contents.
- Default redaction of MCP configuration secrets, and the `--include-secrets`
  bypass.
- Agent filters: exact matching, kebab-case names, and aliases.
- Rejection of path traversal and of destinations outside the allowlist.
- Byte-exact preservation of binary content.
- Rejection of SHA-256 mismatches.
- `--force` keeps a timestamped copy of the overwritten file.
- Enforcement of a one-to-one mapping between manifest entries and bodies.
- Rejection of a symlinked destination parent.
- Rejection of oversized entries detected from their headers.
- Backup does not follow symbolic links at the asset root or inside asset folders.
- A failed backup leaves the previous archive intact.

## Not implemented

The following items were proposed. None has corresponding behavior.

- Claude Code plugin folder under `Claude Extensions/`.
- Extension folders for Cursor, VS Code, and Copilot.
- Instruction files outside project scope. The original proposal listed `CLAUDE.md`,
  `AGENTS.md`, and `GEMINI.md` without a scope. Only project-scope copies are
  collected today.
- `AGENTS.md` and `rules/` instruction sources for Gemini.
- Global plugin folder `~/.gemini/plugins/`.
- OpenCode skills folder `~/.config/opencode/skills/`.
- Instruction files for Devin, Grok, and VS Code Copilot. The proposal did not
  name their locations.
- Restricting Kiro agent definitions to `*.json` files. Today every file in the
  folder is collected.
