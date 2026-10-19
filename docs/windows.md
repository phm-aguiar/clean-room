# Windows Support

This page covers installing, running, and operating ai-memory on Windows:
where files live, how the server and hooks behave, what the service options
are, and what is still unverified. Windows-only behavior that lives in
individual files or in folders outside the main product is described in
[windows-native-extras.md](windows-native-extras.md).

Native Windows is currently at the **Experimental** tier (see
[support-matrix.md](support-matrix.md)). Items marked *(a confirmar)* are
statements that the project has not verified in its own code or CI.

## Rule of Thumb

Run `install-mcp` and `install-hooks` from the same environment that
launches your agent (Claude Code, Codex, Command Code, Devin CLI, Cursor,
Gemini CLI, Kimi Code, Kiro CLI, Antigravity CLI, or another supported
agent).

- If the agent runs inside WSL2, install ai-memory inside WSL2.
- If the agent runs as a native Windows process, install ai-memory from
  PowerShell (or `cmd.exe`) on Windows.
- Do not mix the Windows wrapper with WSL2-launched agents unless you
  deliberately override every config and hook path.
- To keep the server running across logoff and reboot on native Windows,
  use a service wrapper ([Scenario E](#scenario-e-keep-the-server-running-native-windows-service)).
  A scheduled task that starts the server through a script looks like it
  works and then stops silently at the next reboot.

The difference matters because hook configs contain executable paths. WSL2
agents need Linux paths and POSIX shell hook scripts. Native Windows agents
need Windows paths. Claude Code uses a direct executable call by default
([Native Hook Command](#native-hook-command-claude-code-on-windows)), and
other agents use native command strings that match their hook schema.
PowerShell and Git Bash script bundles are compatibility fallbacks and do
not enforce capture-policy v1.

## Choosing a Mode

| Where the agent runs | Install ai-memory in | Server | Hook form |
|---|---|---|---|
| Inside WSL2 | WSL2 (Linux release or wrapper) | Inside WSL2 or the Linux container | POSIX shell scripts (wrapper) |
| Native Windows, Docker Desktop | Windows wrapper + Linux container | Linux container | PowerShell encoded commands for every agent (the wrapper sets the platform to `windows`) |
| Native Windows, release zip | Windows release archive | Native `ai-memory.exe` | Direct executable call (Claude Code); native command strings for other agents |
| Native Windows, source build | Build from source | Native `ai-memory.exe` | Same as release zip |

## Where Things Live on Windows

| Item | Default on Windows | How to change it |
|---|---|---|
| Data directory | `ai-memory` folder inside the per-user local application data folder (`%LOCALAPPDATA%\ai-memory`) | `--data-dir` (global flag) or `AI_MEMORY_DATA_DIR` |
| Config file | `config.toml` inside the data directory | `--config` |
| Hook spool | `hook-spool` folder inside the data directory | Follows the data directory |
| Drain log | `logs\hook-drain.log` inside the data directory (one `.old` rotation) | Follows the data directory |
| Serve lock | `.serve.lock` and `.serve.lock.holder` inside the data directory | Follows the data directory |
| Staged hook scripts (native installs) | `hooks\<agent>` inside the data directory | `--hooks-dir` |
| Staged hook scripts (Docker wrapper) | `$HOME\.local\share\ai-memory\hooks` | Set by the wrapper |
| Home directory used for agent configs | The user profile (`USERPROFILE`/`HOME`) | `AI_MEMORY_HOME` (wrappers and tests) |
| Claude Code project settings (`install-hooks --scope project`) | `.claude\settings.local.json` in the directory the command is launched from | Run the command from the checkout you want to opt in |
| Devin CLI MCP config | `%USERPROFILE%\.devin\config.json` | `--config-file` |
| Devin CLI hooks | `%USERPROFILE%\.devin\hooks.v1.json` | `--config-file` (pass the main config to use its `hooks` key) |
| Kiro CLI MCP config | `%USERPROFILE%\.kiro\settings\mcp.json` | `KIRO_HOME` replaces the `.kiro` root |
| Kiro CLI v2 agent files | `%USERPROFILE%\.kiro\agents` | `KIRO_HOME`, or `--config-file` for a project-local agent |

Notes:

- The data directory is per-user. A Windows service that runs as another
  account resolves `%LOCALAPPDATA%` to that account's profile. Use an
  absolute path (see Scenario E).
- Paths written with the extended-length (`\\?\`) prefix are stripped
  before they are baked into hook commands or read back as the data
  directory. Unknown prefixed forms are kept unchanged.
- Project auto-scope treats Windows backslashes and POSIX slashes as the
  same separator when it compares a hook's working directory, a stored
  repository path, and the home-directory guard. A trailing separator on
  the home directory is ignored.
- Owner-only file and folder modes apply on Unix only. On Windows, the
  spool folder and spool files use the default permissions of the profile
  directory. Whether that is sufficient for your policy is *(a confirmar)*.

## Scenario A: Everything Inside WSL2

This is the most Linux-like Windows setup. Use it when your agent CLI is
installed and launched inside a WSL2 distribution.

```bash
# Inside WSL2.
mkdir -p ~/.local/bin
RELEASE_BASE="<release-download-base-url>"   # the project's Releases page
tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT
curl -fsSL "$RELEASE_BASE/ai-memory-wrapper" -o "$tmp/ai-memory-wrapper"
curl -fsSL "$RELEASE_BASE/ai-memory-wrapper.sha256" -o "$tmp/ai-memory-wrapper.sha256"
(cd "$tmp" && sha256sum -c ai-memory-wrapper.sha256)
install -m 0755 "$tmp/ai-memory-wrapper" ~/.local/bin/ai-memory
rm -rf "$tmp"
trap - EXIT
export PATH="$HOME/.local/bin:$PATH"

docker run -d --name ai-memory \
    --restart unless-stopped \
    -p 127.0.0.1:49374:49374 \
    -v ai-memory-data:/data \
    <official-image>:<tag>

ai-memory install-mcp --client claude-code --apply
ai-memory install-hooks --agent claude-code --apply
```

In this mode ai-memory behaves like Linux:

- Config files are written under your WSL2 home directory.
- Hook scripts are staged under `~/.local/share/ai-memory/hooks/`.
- Hook commands point at POSIX shell scripts. The wrapper sets the hook
  platform to `posix` by default.
- The agent should also be launched from WSL2 so it can execute those
  WSL paths.
- If you install the Linux binary inside WSL2 directly instead of the
  wrapper, the default hook form for Claude Code is a direct executable
  call, not the shell scripts *(a confirmar em WSL2)*.

If Docker Desktop provides the Docker engine to WSL2, enable WSL
integration for the distribution first. If you run a native Docker engine
inside WSL2, no Windows wrapper is involved.

No automated check in the project's CI exercises a WSL2 installation, so
treat WSL2 paths as *(a confirmar)* until you have run the checklist at the
end of this page.

## Scenario B: Native Windows With Docker Desktop

Use this when the agent CLI runs as a native Windows process and you want
the ai-memory server to run from the Docker image.

The Windows wrapper renders host-side agent config with
`http://127.0.0.1:49374`. Its own thin-client commands (status, search,
and so on) reach the server from inside a helper container through Docker
Desktop's `host.docker.internal` name, because Docker Desktop does not
give Linux containers host networking on Windows. Set
`AI_MEMORY_SERVER_URL` only when the server runs elsewhere (a homelab or a
remote host). The wrapper then uses that URL and skips the helper alias.

```powershell
# Install the Windows Docker wrapper.
$UserBin = "$HOME\bin"
New-Item -ItemType Directory -Force $UserBin | Out-Null
$ReleaseBase = "<release-download-base-url>"

# The Releases page publishes two wrapper variants: one for PowerShell and
# one for cmd.exe. Each has a SHA-256 file with the same name plus ".sha256".
# Download each variant, verify it, and save it as ai-memory.ps1 and
# ai-memory.cmd in $UserBin.
foreach ($Variant in @(
    @{ Asset = "<powershell-wrapper-asset>"; File = "ai-memory.ps1" },
    @{ Asset = "<cmd-wrapper-asset>";        File = "ai-memory.cmd" }
)) {
    $Target = Join-Path $UserBin $Variant.File
    Invoke-WebRequest -Uri "$ReleaseBase/$($Variant.Asset)" -OutFile $Target
    Invoke-WebRequest -Uri "$ReleaseBase/$($Variant.Asset).sha256" -OutFile "$Target.sha256"
    $Expected = ((Get-Content "$Target.sha256" -Raw) -split '\s+')[0]
    $Actual = (Get-FileHash $Target -Algorithm SHA256).Hash.ToLower()
    if ($Actual -ne $Expected.ToLower()) {
        throw "Checksum mismatch for $($Variant.Asset)"
    }
    Remove-Item "$Target.sha256"
}
Get-ChildItem "$UserBin\ai-memory.*" | Unblock-File

# Put the wrapper directory on your user PATH for future terminals.
$UserPath = [Environment]::GetEnvironmentVariable("Path", "User")
if (($UserPath -split ';') -notcontains $UserBin) {
    $NewUserPath = (($UserPath, $UserBin) | Where-Object { $_ }) -join ";"
    [Environment]::SetEnvironmentVariable("Path", $NewUserPath, "User")
    $env:Path = "$env:Path;$UserBin"
}

# Start the server with Docker Desktop. The image's default host allowlist
# includes host.docker.internal, so the wrapper's thin-client commands are
# not rejected.
docker run -d --name ai-memory `
    --restart unless-stopped `
    -p 127.0.0.1:49374:49374 `
    -v ai-memory-data:/data `
    <official-image>:<tag>

# Do not also run `ai-memory serve`. The container above is the server.

# Verify the wrapper can reach the server.
ai-memory status

# Wire MCP and lifecycle hooks for a native Windows agent.
ai-memory install-mcp --client claude-code --apply
ai-memory install-hooks --agent claude-code --apply
```

How the wrapper behaves:

- It mounts your Windows home directory into the container and the current
  directory as the project. Outside your home, it mounts the enclosing git
  repository read-only instead, or the current directory when there is none.
- Config files are written through the mounted home directory. Hook scripts
  are staged under `$HOME\.local\share\ai-memory\hooks\`.
- The wrapper sets the hook platform to `windows`, so every agent, Claude
  Code included, gets a PowerShell command. The exec form below applies to
  native installs only. PowerShell commands are encoded with
  `-EncodedCommand`, with text output and progress records suppressed, so
  the hook runner cannot expand setup variables early or write serialized
  progress to stderr.
- `ai-memory upgrade` in the wrapper pulls the newest image. It does not
  replace a local binary.
- Wrapper variables: `AI_MEMORY_DOCKER` (default `docker`), `AI_MEMORY_IMAGE`,
  and `AI_MEMORY_DATA_DIR` (an existing folder is mounted as the data
  directory; otherwise the named volume `ai-memory-data` is used, renamed by
  `AI_MEMORY_DATA_VOLUME`). Server, token, LLM, and embedding variables are
  forwarded when set.
- For `install-mcp`, `install-hooks`, and `setup-agent`, the wrapper does not
  set the helper-container URL, so the host config gets the host-reachable
  default, `http://127.0.0.1:49374`.

After upgrading the wrapper or the image, rerun `install-hooks --agent <agent> --apply`
for each native Windows agent. Agent-specific paths are listed in
[Where Things Live on Windows](#where-things-live-on-windows).

## Scenario C: Prebuilt Release Binary (No Build Tools)

Use this when the agent CLI runs as a native Windows process and you want
the native hook path without Docker or a build environment.

Each tagged release publishes a Windows archive named
`ai-memory-windows-x86_64.zip`, with a SHA-256 file in the same format as
the other release checksums.

```powershell
# Download and extract into your user data folder. Any stable path works:
# the native hook command is rendered from wherever ai-memory.exe lives.
$Dest = "$env:LOCALAPPDATA\ai-memory"
New-Item -ItemType Directory -Force $Dest | Out-Null
Invoke-WebRequest `
    -Uri "<release-download-base-url>/ai-memory-windows-x86_64.zip" `
    -OutFile "$env:TEMP\ai-memory.zip"
Expand-Archive "$env:TEMP\ai-memory.zip" -DestinationPath $Dest -Force
Get-ChildItem "$Dest\ai-memory.exe" | Unblock-File

# Put it on PATH for future terminals (optional).
$UserPath = [Environment]::GetEnvironmentVariable("Path", "User")
if (($UserPath -split ';') -notcontains $Dest) {
    [Environment]::SetEnvironmentVariable("Path", "$UserPath;$Dest", "User")
    $env:Path = "$env:Path;$Dest"
}

# Wire MCP and lifecycle hooks against your server.
& "$Dest\ai-memory.exe" install-mcp --client claude-code --apply
& "$Dest\ai-memory.exe" install-hooks --agent claude-code --apply `
    --server-url "<server-url>" --auth-token "<token>"
```

The archive contains `ai-memory.exe`, the full hooks bundle (PowerShell and
POSIX shell variants), the default configuration template, `README.md`,
`LICENSE`, and `docs/windows.md`. It omits the
Linux-only service assets.

`install-hooks` reads the path of the running executable. Keep the
extracted executable in a stable location. If you move it, rerun
`install-hooks`.

This leaves the server running only while a terminal is open. To keep it
up across logoff and reboot, continue to
[Scenario E](#scenario-e-keep-the-server-running-native-windows-service).

## Scenario D: Build From Source on Windows

Use this when you are developing ai-memory on Windows or do not want the
Docker wrapper for CLI commands.

1. Clone the repository into a folder of your choice.
2. Install the Windows-native build toolchain that the project pins, and
   Git for Windows (see [Build-time requirements](#build-time-requirements)).
3. Build the workspace and run its test suite.
4. Run the built executable (`ai-memory.exe`, in the build output folder)
   with `init`, then with `serve --transport http --bind 127.0.0.1:49374`.

```powershell
ai-memory.exe init
ai-memory.exe serve --transport http --bind 127.0.0.1:49374
```

The `serve` command holds the terminal. For a persistent server, see
[Scenario E](#scenario-e-keep-the-server-running-native-windows-service).

From another PowerShell window in the repository:

```powershell
ai-memory.exe install-mcp --client claude-code --apply
ai-memory.exe install-hooks --agent claude-code --apply
```

For release validation on native Windows, build the release profile of
the command-line package and check that `ai-memory.exe --version` matches
the version in the checkout's project metadata.

### Build-time requirements

- Git for Windows (`git.exe`) must be on `PATH` for native builds and for
  hook runs. When the server's git access layer cannot open the wiki
  repository because of a Windows path-resolution error, the server falls
  back to the Git command-line tool. This applies when opening any wiki
  repository, not only a freshly initialized one. It never applies to an
  ownership error (see [Data directory owner](#data-directory-owner)).
- The web stylesheet is vendored, so normal builds download nothing.
  Regenerating it (`TAILWIND_BUILD=1` when building the web component)
  downloads the fixed-version stylesheet compiler, verifies its SHA-256, and
  tries curl, then wget, then PowerShell. Windows x86_64 is the mapped
  platform for this step.

### If the build fails with `os error 4551`

On a machine with **Smart App Control** or **App Control for Business**
enforced, the build can fail before compiling any project code:

```text
error: failed to run custom build command for `<early-built dependency>`
  An Application Control policy has blocked this file. (os error 4551)
```

This is not a toolchain problem, and reinstalling the toolchain will not
fix it. The build compiles helper executables into a user-writable build
folder. Those helpers are unsigned, and the policy blocks unsigned
executables that run from user-writable folders. The dependencies that
fail first are the ones that build early.

Add a path exclusion for the checkout's build output folder, or build in
a location your policy already trusts. Reported by a user while running
the measurements in issue #478.

## Wiring Agents: Install, Re-run, Upgrade, Uninstall

### Install and re-run

- `install-mcp --client <client>` prints the MCP configuration when run
  without `--apply`. With `--apply`, it updates the client's config file
  in place, replacing only the ai-memory entry and keeping other servers.
  A timestamped backup is written next to the original before each
  write.
- `install-hooks --agent <agent>` behaves the same way for hook
  configuration. Running it again is idempotent. Claude Code can register
  project-scoped hooks with `--scope project`.
- Run these commands as your own user. They write per-user agent
  configuration, even when the server runs as a service.

### Per-agent notes

- **Claude Code, project scope.** On Windows, `--scope project` writes
  `.claude\settings.local.json` in the directory the command is launched
  from. On other platforms it uses the git repository root (unless that is
  the home directory). Run the command from the checkout you want to
  opt in.
- **Devin CLI.** MCP config goes to `%USERPROFILE%\.devin\config.json`. Hooks
  go to `%USERPROFILE%\.devin\hooks.v1.json`, or under the `hooks` key of the
  main config when you pass `--config-file` for it.
- **Kiro CLI.** Its MCP config is under `%USERPROFILE%\.kiro\settings`, and
  `KIRO_HOME` replaces the `.kiro` root. Hooks for the v2 engine go into
  the agent files under `.kiro\agents` (use `--config-file` for a
  project-local agent). The v3 engine is a separate choice (`kiro-v3`) that
  writes one ai-memory registration file under `hooks\`. Whether v3 capture
  is validated on live sessions is *(a confirmar)*. Run these commands from
  the native Windows environment that launches Kiro.
- **Generated script integrations.** OpenClaw, OpenCode (v1 and the v2
  beta), OMP, and Pi use generated script integrations instead of the
  hook bundle, so their Windows behavior depends on the host runtime. Pi's
  integration also bridges MCP tools *(a confirmar em Windows)*.

### Upgrade

- Native installs (release archive) can use `ai-memory upgrade`. It
  downloads the matching Windows archive and its checksum, verifies it,
  replaces the running executable, and reruns `install-hooks --apply` for
  every staged agent. Replacement renames the running executable aside
  before moving the new one in. The old copy is removed on a best-effort
  basis. Windows on ARM has no release archive.
- Docker-wrapper installs use the wrapper's `upgrade` command (see
  Scenario B). The status of the in-place upgrade work tracked in issues 801
  and 802 is *(a confirmar)*.

### Uninstall

`ai-memory uninstall` removes only ai-memory's own wiring: hook entries,
MCP entries, routing instructions, and managed skills. It never matches
third-party entries by name alone.

- Without `--apply`, it prints the removal plan and exits. With
  `--apply`, it asks for confirmation on an interactive terminal unless
  you pass `--yes`.
- `--only` limits the scope to one concern. `--mcp-url` (or `--server-url`)
  identifies the MCP entry when you used a custom URL. `--mcp-name` adds a
  name filter; an entry must match both.
- `--purge-data` also wipes the wiki, database, and raw data, through the
  reset path. It refuses while another ai-memory process is alive.
- The command does not remove the executable. For a Docker install it prints
  the teardown commands but does not run them.
- Staged hook scripts in the data directory are not listed as removed
  *(a confirmar)*.
- Hook entries in other checkouts are not reachable from the current one.
  Run `uninstall --only hooks --apply` in each of them.

## Native Hook Command (Claude Code on Windows)

By default on native Windows, Claude Code hooks use Claude's exec form:
the command is the real `ai-memory.exe` path and `args` is an argument
array. The executable is spawned directly. There is no quoted command
string for a shell and no `bash -c` wrapper around a script.

```json
{
  "type": "command",
  "command": "C:\\Users\\you\\bin\\ai-memory.exe",
  "args": ["--data-dir", "C:\\Users\\you\\AppData\\Local\\ai-memory", "hook", "--event", "pre-tool-use", "--agent", "claude-code", "--server-url", "http://host:49374"]
}
```

For `SessionEnd`, the generated handler entry also sets `"timeout": 15` (seconds). When an auth token is passed at install time, it is stored in `<data_dir>\auth-token` rather than baked into `args`.

This avoids starting a shell and several helper processes on every tool
call. Process creation is expensive on Windows. A measured speed-up of
roughly 3 to 5 times per hook (about 735 ms through a shell against about
150 to 205 ms native on one reference laptop) is *(a confirmar)*. It is
not reproduced in the repository.

- The executable path is the one of the `ai-memory` that runs
  `install-hooks` (with any `\\?\` extended-length prefix stripped). Release
  binaries and locally built binaries both work directly. If you move the
  executable, rerun `install-hooks`.
- Exec form requires a real executable. It does not run batch or script
  shims through a shell.
- The script bundles stay in place as a fallback for setup-agent and the
  Docker flow, which have no local binary.

### Platform values

Set `AI_MEMORY_HOOK_PLATFORM` before running `install-hooks`. The value is
baked into the rendered hook commands. Accepted values:

| Value | Meaning | Default for |
|---|---|---|
| `windows-native` | Claude Code exec form (direct executable call); native command string for other agents | Native Windows installs |
| `windows` | PowerShell with an encoded command and a staged PowerShell hook script | The Windows wrapper |
| `windows-bash` | `bash -c` through Git Bash and the POSIX script (the previous Windows default) | Nothing; opt in, or use it for older Claude Code builds without exec form |
| `posix` | POSIX shell scripts | The Linux and macOS wrapper |
| `posix-native` | Direct executable call on Linux and macOS, using the local spool and the OIDC token fallback | Native Linux and macOS installs |

`unix` is accepted as an alias for `posix`. Unknown values are ignored and
the per-context default applies.

### Capture-policy support

Native hook commands enforce the nearest-marker `[capture] ignore_paths`
policy before any spool or network delivery. The PowerShell and POSIX
script paths do not enforce it. After upgrading, rerun
`install-hooks --agent <agent> --apply` to refresh native hook entries.
The install output says whether enforcement is active. See
[Capture exclusions](marker-file.md#capture-exclusions).

## Hook Spool Behavior

Lifecycle hooks do not post to the server on the agent's hot path. Each
event is written to a local spool first, and the spool is drained at
session boundaries. This keeps capture reliable against a slow or remote
server, and no event is lost until the server answers with a success
status.

### Boundaries

- **Per tool call** (`pre-tool-use`, `post-tool-use`, `user-prompt-submit`):
  one local write. A busy `post-tool-use` stream runs a short catch-up
  drain once the backlog reaches the incremental threshold, with a total
  budget of 250 ms.
- **`session-start`**: a short, lock-aware cleanup drain, bounded by the
  start budget, before the handoff context is fetched.
- **`session-end`, `stop`, and `pre-compact`**: start a detached drain
  helper and return at once, so the agent is not held open by a large
  backlog. The helper runs under the same single-flight lock with the
  background budget.

### Enqueue and limits

- Each event is written to a temporary `.json.tmp` file and renamed into place
  as `{created_ms:013}-{pid}-{seq:016x}-{recovery_key}.json`, so readers never
  see a partial event. At the start of a drain pass, stranded `.json.tmp` files
  older than 5 minutes (300,000 ms) are inspected: valid spool entries up to
  10 MiB whose internal `created_ms` matches the filename prefix are recovered
  by renaming to `.json`; malformed or oversized `.json.tmp` files are deleted.
- Each event carries its own authentication (`auth_mode`): an inline static
  token (`static_token` with `token`), an OIDC mode (`oidc_at_drain`) resolved
  from the stored login at drain time, or `anonymous`, plus optional `profile`
  and `attempts`.
- An event is dropped after 7 days, or after the configured number of failed
  drain passes (default 8, set by `max_attempts` in the `[hook_spool]`
  config section or `AI_MEMORY_SPOOL_MAX_ATTEMPTS`; zero disables the
  attempt-based drop).
- The spool holds at most 10,000 files (coordinated via
  `<data_dir>\hook-spool.capacity.lock` with a 50 ms wait). When the spool is
  already at capacity (`>= 10,000` files), the existing backlog is preserved
  and the **new incoming event is dropped** (its temporary file is removed),
  writing `ai-memory: hook-spool at capacity (10000); dropping one new UNDELIVERED event`
  to stderr and exiting with status `0`.
- An unreachable server does not consume attempts. Rate-limited (`429`) and
  request-timeout (`408`) responses also keep the event queued without consuming
  an attempt. Server failures (`5xx`, `401` after token refresh retry, or a
  batch `failed_index`) increment `attempts`; permanent `4xx` responses (other
  than `401`, `408`, `429`) drop the event immediately.
- Batch uploads carry at most 256 events or 8 MiB. A single event over the
  byte limit is still sent alone.
- Dropped events are reported on stderr and in the drain log.

### Locking on Windows

- The drain takes one exclusive OS-level file lock on `<data_dir>\hook-spool\.drain.lock`
  (checking first that `<data_dir>` exists so a detached drainer surviving
  `uninstall --purge-data` does not recreate the directory). The OS releases
  the lock when the process exits.
- Windows can report a contended lock with its native lock-violation code
  (error 33) instead of the generic would-block kind. Both count as busy, so
  concurrent drains wait, skip, or expire under the same timing rules.
- Incremental `post-tool-use` drains do not wait if the lock is busy; standalone
  `hook-drain` waits up to 30 seconds, polling every 25 ms.

### Timing overrides

Built-in timings are short on agent-facing paths. High-latency or
large-backlog instances can raise them with whole-minute overrides. The
hook reads these at runtime, so they apply to the agent's environment and
do not require rerunning `install-hooks`.

| Environment variable | Built-in default | Maximum override | What it caps |
|---|---|---|---|
| `AI_MEMORY_HOOK_DRAIN_TIMEOUT_MINUTES` | 3 seconds | 60 minutes | Each event request during a drain |
| `AI_MEMORY_HOOK_HANDOFF_TIMEOUT_MINUTES` | 3 seconds | 60 minutes | The synchronous `session-start` handoff request |
| `AI_MEMORY_HOOK_START_BUDGET_MINUTES` | 3 seconds | 60 minutes | Total time `session-start` may spend waiting for the drain lock and cleaning the backlog |
| `AI_MEMORY_HOOK_BACKGROUND_DRAIN_BUDGET_MINUTES` | 5 minutes | 60 minutes | Total time the detached drain helper may run after a boundary |
| `AI_MEMORY_HOOK_INCREMENTAL_THRESHOLD` | 32 events | Positive integer | Backlog size that triggers the 250 ms `post-tool-use` catch-up drain |

- Timing values are positive whole minutes. Missing, empty, non-numeric,
  fractional, or zero values fall back to the default, and values above 60
  are clamped to 60. The incremental threshold is a positive event count;
  invalid values fall back to 32.
- The session-start budget caps how long the hook can block before the
  handoff fetch. The background budget does not keep the agent waiting.

### Malformed input

- Native hooks accept well-formed JSON, with or without one leading UTF-8
  byte-order mark. Some PowerShell pipelines add that mark when writing
  to a native process.
- Any other malformed input is neither spooled nor sent. The hook prints
  a fixed warning to stderr (never with payload contents), writes its normal
  success response, and exits successfully, so it cannot break the agent.
- The success response is `{}` for most agents. Three agents differ because
  their runners treat stdout as model context or as a decision: Kiro CLI
  gets no stdout, Kimi Code gets no stdout on prompt submission, and
  Antigravity CLI gets an allow decision on pre-tool use.

### Detached drain on Windows

- The helper starts (`<current_exe> --data-dir <data_dir> hook-drain`, passing
  the live static token in `AI_MEMORY_AUTH_TOKEN`) with no console window
  (`CREATE_NO_WINDOW`), in its own process group (`CREATE_NEW_PROCESS_GROUP`)
  and detached from the agent's console (`DETACHED_PROCESS`). It first tries
  to leave the agent's job object (`CREATE_BREAKAWAY_FROM_JOB`). If the system
  denies that (`ERROR_ACCESS_DENIED`, OS error 5), it retries spawning without
  breakaway and logs one warning line to `hook-drain.log`.
- Output goes to `logs\hook-drain.log`. Past 1 MiB the log is renamed to
  `hook-drain.log.old`, replacing any earlier `.old` file.

## Running the Server

### Commands and defaults

- `ai-memory serve` runs the MCP server on the standard input and output
  by default. Use `--transport http` for an HTTP listener.
- The default HTTP bind address is `127.0.0.1:49374`. The default MCP
  endpoint is `http://127.0.0.1:49374/mcp`.
- The web interface is off by default. `--enable-web` turns it on, and
  `--enable-api` mounts the read-only JSON API alone.
- `--data-dir` is a global flag. Pass it before the subcommand when a
  service or script needs an explicit data directory.

```powershell
ai-memory.exe --data-dir "C:\Users\you\AppData\Local\ai-memory" serve --transport http --bind 127.0.0.1:49374
```

### Single instance

Only one `serve` process may use a data directory at a time. The server
takes an exclusive lock on `.serve.lock` in the data directory and holds it
for the life of the process. The operating system releases it when the
process exits, so a crashed server never locks you out.

- A second server on the same data directory refuses to start and names
  the holder recorded in `.serve.lock.holder` (its process identifier, written
  as `pid=<pid>\n` in a separate unlocked sidecar file because Windows exclusive
  locks prevent other handles from reading `.serve.lock` directly).
  The holder file is best-effort and informational. The lock is the real
  guard.
- `--force` starts anyway. Use it only when you are certain the other
  process is gone (a hung process, or a mount with unreliable locking).
  Two live servers on one data directory corrupt wiki and index state.
- If the file system cannot take the lock at all, the server logs a
  warning and runs without the guard. It does not refuse to start.
- Transient failures when opening or locking the file (out of file
  handles, interrupted calls) are retried, with a 25 ms backoff, up to five
  attempts in total. A held lock is never retried.
- The reset, restore, reindex, and `uninstall --purge-data` commands use a
  separate check. They refuse to run while another ai-memory process is
  alive and list the process identifiers. The server does not run that
  check. On Windows, the process check must match both `ai-memory` and
  `ai-memory.exe` (case-insensitively), excluding the current process ID and
  thread entries (and honoring test overrides `AI_MEMORY_TEST_NO_PROCESS_GUARD`
  and `AI_MEMORY_TEST_FORCE_SIBLING_PIDS`).

### Exposure and authentication

- Plain HTTP on a non-loopback address is refused unless an auth token is
  configured or `--allow-insecure-no-auth` is passed. The override prints a
  warning banner at startup.
- Inside a container, the server cannot tell whether a published port reaches
  the network. It starts with a warning banner instead of refusing.
- Human (password) login over non-loopback plain HTTP is refused unless
  secure cookies are enabled behind a trusted HTTPS proxy.
- Generate a token with `ai-memory generate-auth-token` and set
  `AI_MEMORY_AUTH_TOKEN`.

### Stopping

On Windows the server stops on Ctrl+C. Elsewhere it stops on the interrupt
and terminate signals, and a stop during startup is honored too.

## Scenario E: Keep The Server Running (Native Windows Service)

A foreground server (Scenario C or D) is fine for a trial. Close the
window, log off, or reboot, and it is gone. On Linux, the packaged service
units restart the server on failure after 5 seconds. The service manager
does that work, not ai-memory. Native Windows needs a **service wrapper**
for the same behavior.

ai-memory does not register itself as a Windows service and does not
implement the service control handler. Pointing a service creation command
straight at `ai-memory.exe` is not supported. The manager would wait for a
status report that never arrives *(a confirmar em ambiente real)*.

### Scheduled tasks

A task that runs a script which starts the server and then exits is a known
failure. The task scheduler ends the task's job object when the script
exits, and that takes the child server with it. The task reports success,
and the log holds only the startup line. This was reported in issue 530 and
is *(a confirmar)* on current versions.

If you use the scheduler, point the task action at `ai-memory.exe` with
`serve` and its arguments, so the monitored process is the server. Then run
the task whether or not you are logged on, clear the default execution time
limit, and set restart-on-failure explicitly. The wrapper below is still the
recommended option.

### WinSW wrapper

WinSW is a third-party tool that wraps a console program as a Windows
service, configured by one XML file beside the executable. Pin a specific
release and record which one you installed, because it runs with your
credentials.

Rename the downloaded executable and give the XML file the same base name:

```powershell
$Dest = "$env:LOCALAPPDATA\ai-memory"
# Download the WinSW release that matches your runtime from its Releases
# page. Pin a specific tag rather than "latest".
Move-Item .\WinSW-x64.exe "$Dest\ai-memory-service.exe"
```

Create `ai-memory-service.xml` next to it. Write every path out in full. Do
not use environment variables, because a service runs under another
account by default:

```xml
<service>
  <id>ai-memory</id>
  <name>ai-memory MCP server</name>
  <description>Long-term memory server for AI coding agents.</description>
  <executable>C:\Users\you\AppData\Local\ai-memory\ai-memory.exe</executable>
  <arguments>--data-dir "C:\Users\you\AppData\Local\ai-memory" serve --transport http --bind 127.0.0.1:49374</arguments>
  <startmode>Automatic</startmode>
  <onfailure action="restart" delay="5 sec"/>
  <log mode="roll"/>
</service>
```

The restart-on-failure rule with a 5 second delay mirrors the packaged
Linux units.

Install and start it from an elevated PowerShell:

```powershell
& "$Dest\ai-memory-service.exe" install
& "$Dest\ai-memory-service.exe" start
& "$Dest\ai-memory-service.exe" status

# Verify the server answers, not only that the service reports Running.
Get-Service ai-memory
ai-memory status
```

The remaining wrapper commands are `stop`, `restart`, and `uninstall`.
Because the bind address is loopback, the service account does not affect
reachability: agents running as your user still reach `127.0.0.1:49374`.

Keep running `install-mcp` and `install-hooks` as your own user, not as the
service. They write per-user agent configuration.

### Service account and the data directory

- A service under the built-in system account resolves `%LOCALAPPDATA%` to
  that account's profile. Without an absolute `--data-dir`, it would quietly
  serve an empty directory while your real data sits in your profile.
- An absolute `--data-dir` does not make the system account safe over a data
  directory inside a user profile, because the wiki's git repository fails
  the ownership check (see the next section).
- Whenever the data directory is inside a user profile, run the service as
  that user. Add a service-account block to the XML:

```xml
  <serviceaccount>
    <username>.\you</username>
    <password>your-account-password</password>
  </serviceaccount>
```

  Use `.\you` for a local account or `DOMAIN\you` for a domain account. You
  can also set the account in the service's Log On tab after install, then
  restart the service. The system account is appropriate only for a data
  directory it owns outright, such as a dedicated folder under the program
  data location that the service account created.

- If the service fails to start with a logon failure (error 1069), the
  stored password is wrong or stale. This happens often with Microsoft
  accounts, PIN sign-in, and Windows Hello. The service password must be
  the account's password, and it must be updated whenever that password
  changes. For an unattended service, prefer a local account or a dedicated
  service account with a non-expiring password.

### Data directory owner

The wiki keeps a git repository inside the data directory. Git refuses to
commit to a repository owned by a different account (the ownership
protection described in CVE-2022-24765), and the server's git access layer
applies the same check. When the server runs as a different account, every
wiki commit fails with an ownership error. A failed wiki commit is not fatal,
so the server keeps running and capture continues, but no wiki checkpoints
are committed.

On startup, the server logs this condition at error level and names the
remedy: run the service as the owning user. Older builds logged it at warning
level *(a confirmar)*.

### Validation report

A validation report from 2026-08-31 (Windows 11, WinSW 2.12.0, ai-memory
1.38.0) describes install, start, a protocol answer on the bound port,
crash recovery within about 8 seconds, and a clean stop and uninstall. It
did not exercise boot-time startup. The repository cannot reproduce it, so
treat it as *(a confirmar)*.

## Firewall and Network Reach

- ai-memory does not create firewall rules or change network configuration.
  Create any rule for a remote server yourself.
- The default bind address is loopback. Publish the Docker port on
  `127.0.0.1` to keep it local.
- On Windows, a firewall can drop a connection to a closed loopback port
  instead of refusing it. The CLI reports both cases as "nothing answered
  (refused or timed out)" and names the resolved server URL.
- For a remote server, set `AI_MEMORY_SERVER_URL`. The HTTP server checks
  the request's Host header against an allowlist. The default allowlist
  includes `localhost`, `127.0.0.1`, `::1`, and, in the Docker image,
  `host.docker.internal`.
- A non-loopback server needs an auth token (plain HTTP without one is
  refused), and the server host's firewall must allow the port.

## Status and Limitations

### Support tier

Native Windows is at the **Experimental** tier. Promotion to Supported is a
maintainer decision. It depends on these items:

- **Pull-request CI coverage (done).** The Windows CI workflow runs nightly,
  on manual dispatch, and on pull requests that touch platform-sensitive
  areas (path handling, file locking, git plumbing, the hook bundle). A
  `windows` label forces the jobs on any pull request.
- **Hook bundle job (partly done).** The hook suite runs on the Windows
  runner through a POSIX shell. Its static checks run everywhere. Its
  PowerShell behavioral probes run only when a PowerShell 7 binary is
  present (*a confirmar no log do job*).
- **Windows-only regression tests (in progress).** They cover the drain-lock
  busy state, the git fallback, and verbatim path handling. Each new Windows
  path, lock, or git change should add its own regression test.
- **Code signing (deferred pending policy).** Release binaries and build
  helpers are unsigned, so App Control for Business and Smart App Control
  can block them (the error 4551 case). Signing needs a certificate and a
  decision on how its secret is kept in CI. Both are maintainer decisions.
- **Native upgrade (available; tracking status to confirm).** The `upgrade`
  command exists for release-archive installs (see Upgrade above).

### Known gaps

- ai-memory has no Windows service dispatcher. Persistence comes from a
  wrapper (Scenario E), the same way it comes from a service manager on
  Linux.
- Native Claude Code uses the direct executable call by default. The
  `windows-bash` platform value restores the Git Bash path.
- Other agents (Codex, Command Code, Devin CLI, OpenCode, Cursor, Gemini
  CLI, Antigravity CLI, Grok Build CLI, Zero, Kimi Code, Kiro CLI, OpenClaw,
  OMP, Pi) may use different Windows configuration locations or shell
  behavior. The defaults on this page need validation on real installs
  *(a confirmar)*.
- MCP over HTTP is less sensitive to paths than hooks, but `install-mcp
  --apply` still writes a client-specific file. Confirm the agent loads it.
- WSL2 has no automated coverage in CI. Treat WSL2 statements on this page
  as *(a confirmar)* until verified by hand.
- Windows hook support is new and needs real-world testing against native
  agent builds.

## Suggested Test Checklist

For WSL2:

1. Run all install commands inside WSL2.
2. Confirm hook commands reference POSIX shell scripts under WSL paths
   (wrapper install) or the executable path (Linux binary install).
3. Launch the agent from WSL2, call `memory_status`, then record
   `ai-memory status` before and after a prompt. The `sessions` or
   `observations` count should increase.

For native Windows:

1. Run all install commands from PowerShell or `cmd.exe`, using `ai-memory`
   or `ai-memory.ps1` as the command name.
2. Confirm hook commands match the agent. For a release or source install,
   Claude Code should use the native executable command (or `bash -c` plus the
   POSIX script when `AI_MEMORY_HOOK_PLATFORM=windows-bash`). For the Docker
   wrapper, hooks should be `powershell.exe ... -EncodedCommand` entries that
   point at the staged PowerShell hook scripts under your Windows home.
3. Launch the agent, call `memory_status`, then repeat the `ai-memory status`
   check from the WSL steps.

Report which mode you tested, which agent and version you used, and whether
the hook command ran or failed with a path or shell error. The built-in web
browser lists compiled wiki pages. Zero pages there does not mean the raw
hook observations were missed.
