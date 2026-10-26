# Design: safer `--yolo` and ai-jail integration

Status: implemented. Every behavior described here was checked against the
current source. Statements marked "a confirmar" describe the behavior of an
external tool (ai-jail, or the Claude Code harness) that this repository cannot
verify; they are listed in the last section. This document has no pending
proposal: every section describes behavior that exists.

The design covers the `run` command and has five parts:

1. A confirmation warning before `--yolo` disables an agent's permission prompts.
2. An offer to re-run the session inside ai-jail when ai-jail is usable on the host.
3. Detection of "already inside ai-jail", to avoid double wrapping and repeated prompts.
4. An opt-in "true yolo" for Claude that also forces its bypass permission mode.
5. Flags and an interactive checklist that choose what the jail exposes.

## Motivation

`--yolo` maps each harness to its dangerous-mode flag (see
[harness-adapters.md](harness-adapters.md)). For Claude that is
`--dangerously-skip-permissions`. The agent then runs every tool call without
confirmation. Three gaps follow:

1. **No warning.** A user types `--yolo` and the agent is immediately
   unsupervised, with no reminder of what that means or a way to back out.
2. **No sandbox nudge.** ai-jail exists to contain an unsupervised agent, but
   nothing connects the two. The user must remember to run ai-jail themselves.
3. **Claude still pauses.** Even with the skip flag, Claude Code still prompts on
   explicit `ask` permission rules and on its own command-safety checks, so an
   unattended run can stall. The external documentation states that no
   permission mode skips these (a confirmar, see the last section).

## Non-goals

- Changing the behavior of runs that use neither `--yolo` nor `--jail`. The one
  side effect outside those flags is the ai-jail availability probe described in
  §2.1, which runs on every launch outside a jail but shows nothing unless
  `--yolo` or `--jail` was requested.
- Modifying ai-jail. Detection uses ai-jail's existing observable surface. No
  change on the ai-jail side is required.
- Prompting in any non-interactive path. Warnings and offers appear only when
  both standard input and standard error are terminals. Hook, CI, detached, and
  piped runs are not affected.

## Terms

- **Interactive**: standard input and standard error are both terminals. This is
  the same gate the native-session picker already uses.
- **Inside ai-jail** (or **jailed**): the process is already running inside an
  ai-jail sandbox (§3).
- **Usable ai-jail**: ai-jail is found on this host, its sandbox backend is
  available for this operating system, and its dry-run preflight succeeds (§2.1).
- **Toggle**: an ai-jail `--X` / `--no-X` switch that mounts a credential or
  exposes a host capability (§5).
- **Managed run**: the run record that a launch prepares, with a lease, before it
  starts the agent. See [managed-workstreams.md](managed-workstreams.md).

## 1. Yolo warning

The warning is shown when all three conditions hold:

- `--yolo` or `--true-yolo` was requested, either before or after the harness name;
- the session is interactive;
- the process is not already inside ai-jail.

It prints:

```text
⚠  --yolo runs every tool call with no confirmation. An agent can delete
   files, run any command, and reach the network unsupervised.
   Proceed? [Y/n]
```

Answer rules:

- Enter, `y`, `yes`, end of input, or any other text proceeds. This is a
  default-yes prompt, not a strict allowlist.
- Only `n` or `no` (case-insensitive, surrounding spaces ignored) declines. The
  launch then stops with the message "aborted: --yolo not confirmed": nothing is
  started and the exit status is non-zero.

Timing: the prompt runs after the managed run is prepared and before native
session selection and before the harness's permission flag is applied. Any stop
at this point (decline, error, or interruption) cancels the prepared managed run.

When the session is not interactive, no prompt is shown and the harness flag is
applied silently, so scripts, hooks, and CI keep working unchanged.

Harness without a verified flag: for the Kiro harness on its v2 engine the flag is
the trust-all-tools option. On its v3 engine, and on any engine selected by a
non-default argument, no equivalent is documented. In that case the launch
continues without a flag and prints (stderr) `ai-memory: --yolo maps to no verified flag on the selected Kiro engine (v3 replaced --trust-all-tools with permissions.yaml); launching without it`.

## 2. ai-jail detection and offer

### 2.1 When the offer can appear

The offer appears only when accepting it can actually work. ai-jail is usable when
all of these hold:

- **Operating system**: Linux or macOS. On Windows ai-jail is unsupported, so
  there is never an offer, even when a file with that name is on the search path.
- **Sandbox backend**: on Linux, the backend tool (`bwrap`) must be on the search path, or
  its override variable (`BWRAP_BIN`, a path) must be set (the override is used without an existence
  check). On macOS, the system sandbox tool (`sandbox-exec`) must be on the search path.
- **ai-jail binary**: `ai-jail` on the search path. When it is not on the search
  path, `~/.local/bin/ai-jail` is used as a fallback, if it is an executable file.
- **Preflight**: ai-jail is run once in dry-run mode against the current
  executable. Dry-run does not execute the agent and does not write
  configuration, but it applies ai-jail's own backend trust and local
  configuration checks. The launch is usable only if that run exits successfully.
  The exact invocation is `<ai-jail> --dry-run -- <current executable>` with
  stdin, stdout and stderr discarded; failing to start it, or a non-zero
  status, means "not usable". The `~/.local/bin/ai-jail` fallback is computed from
  the `HOME` variable and only when it is set. The search-path lookup requires
  a regular file; on Unix it must also have an execute bit.

The exact path that passes these checks is the path that is later executed. It is
never looked up again through the search path, so the re-exec cannot pick a
different binary.

When ai-jail is not usable, there is no ai-jail question at all. The launch
continues directly after the §1 warning, if one was shown.

On every launch outside a jail where ai-jail and its backend are found, the
preflight runs. Its result changes nothing unless `--yolo` or `--jail` was requested.

### 2.2 The offer

When ai-jail is usable, the process is not jailed, `--yolo` is interactive, and
`--no-jail` was not given, the warning from §1 is followed by a second question:

```text
ai-jail is installed. Re-run this session inside it? [Y/n]
```

Enter, `y`, `yes`, end of input, or any other text accepts. Only `n` or `no`
declines.

- **Declined**: the run proceeds unjailed. The user already confirmed the
  warning, so this is their choice.
- **Accepted**: the prepared managed run is cancelled, then the process re-execs
  under ai-jail (§2.3). A user who accepts the offer then sees the toggle
  checklist (§5.6), unless a project ai-jail configuration file takes over the
  toggles (§5.5).

### 2.3 The re-exec

The re-exec is a replacement of the current process, not a nested spawn (Unix).
On a platform without process replacement the launcher instead spawns ai-jail,
waits for it and exits with its status (1 when the status carries no code); that
branch is unreachable in the Windows MVP because ai-jail is never usable there.
It runs ai-jail with this argument list:

```text
ai-jail --network --agent-state [--no-save-config] \
        [--env NAME ...] [toggle flags ...] \
        -- <current executable> <original arguments, unchanged>
```

- `--network` is always present, `--agent-state` is always present on this path,
  and `--no-save-config` is present only when the installed ai-jail advertises
  it (§5.9).
- One `--env NAME` pair follows for each forwarded variable that is set in the
  current environment (see below). Names only, never values.
- Toggle flags come after the environment pairs and before the separator.
- The original arguments are forwarded exactly as received, including `--yolo`.
  Inside the jail the run is treated as jailed (§3), so it does not warn or offer
  again.
- Before the re-exec a one-line summary of what the jail receives is printed to
  standard error (§5.7).

Why each part exists:

- **`--` separator.** ai-jail refuses its own flags when they appear after the
  command, because it cannot tell whether a flag belongs to the sandbox or to the
  child. ai-memory's run command shares flag names with ai-jail, so without the
  separator a forwarded flag such as an environment flag made the launch fail.
  After the separator ai-jail passes everything to the wrapped command verbatim.
  The project changelog records that forwarded flags colliding with ai-jail's own
  flags need ai-jail 2.4.2 or later. The separator is emitted regardless.
- **`--network`, not a filtered host mode.** ai-jail shares the host network
  namespace only when network access is enabled. That keeps the host-side
  ai-memory server (default address `http://127.0.0.1:49374`) and the agent's
  hooks reachable. A filtered host mode runs in a private network namespace with
  a proxy for external hosts only, which would cut off the loopback server, so it
  is not used. This is soft protection: the agent has full outbound network. That
  is accepted, because the agent needs outbound access to its own model API
  anyway (a confirmar, see the last section).
- **Environment passthrough.** ai-jail clears the environment by default and
  re-adds only what is named. ai-memory therefore forwards the names below, and
  only those that are set:

  `AI_MEMORY_SERVER_URL`, `AI_MEMORY_BASE_PATH`, `AI_MEMORY_HOOK_URL`,
  `AI_MEMORY_DATA_DIR`, `AI_MEMORY_AUTH_TOKEN`, `CLAUDE_CONFIG_DIR`,
  `ANTHROPIC_API_KEY`, `ANTHROPIC_OAUTH_TOKEN`, `CLAUDE_CODE_OAUTH_TOKEN`,
  `COPILOT_GITHUB_TOKEN`, `GEMINI_API_KEY`, `GOOGLE_API_KEY`,
  `OPENAI_API_KEY`, `OPENROUTER_API_KEY` (14 names; the earlier text of this
  document listed 13 and omitted `AI_MEMORY_BASE_PATH`).

  The list is fixed. A name is forwarded when it is set in the environment that
  the configuration loader captured at startup (never re-read from the live
  process environment), or when the launcher itself sets it on the ai-jail
  process (next paragraph). A name that is not set is not forwarded, and no
  value is logged or written by the re-exec.

  **Values resolved from configuration.** ai-jail's private home hides the
  data directory, so the jailed run cannot read `config.toml`. The launcher
  therefore sets, on the ai-jail process, `AI_MEMORY_SERVER_URL` (with the
  resolved server URL) when the URL came from explicit configuration or was
  derived from a custom `bind`, and `AI_MEMORY_BASE_PATH` (with the resolved
  base path) when it is not empty. Both are then forwarded by name.

  **A bearer loaded from configuration is never exported.** If the server
  bearer token came from `config.toml` (or the file it references) and
  `AI_MEMORY_AUTH_TOKEN` is not set in the operator's environment, the re-exec
  is refused before ai-jail starts, for an explicit `--jail` and for an
  accepted offer alike, with the single-line message (exact text, no wrapping)
  "--jail cannot forward the server bearer loaded from config into the managed harness; use a least-privilege AI_MEMORY_AUTH_TOKEN for this launch, or run without --jail". Reason: a
  config-file token is server authority (possibly root) that the harness did
  not hold before, and forwarding it would let an unsupervised agent exfiltrate
  it. A token the operator put in `AI_MEMORY_AUTH_TOKEN` already belongs to the
  child's boundary and is forwarded unchanged. (This rule was missing from the
  earlier text of this document.)
- **`--no-save-config`.** On ordinary runs ai-jail merges its command-line flags
  into the project's `.ai-jail` file and writes it back. Without this flag the
  first jailed run would persist ai-memory's transient flags (network, agent
  state, chosen credentials) into the user's repository. Later runs would then
  skip the checklist and lose credential mounts that a project file cannot enable.
- **`--agent-state`.** It keeps the harness's own login and credential state
  across ai-jail's otherwise ephemeral private home. ai-jail derives the per-harness
  location from the wrapped run command it parses.

## 3. Already inside ai-jail

ai-jail sets no dedicated marker variable for this. Detection uses the observable
surface of the sandbox, per operating system:

- **Linux**: the hostname is `ai-sandbox`. The check reads the kernel hostname
  and compares it. A missing or unreadable value counts as "not jailed".
- **macOS**: the sandbox has no separate hostname, so the check uses the shell
  prompt variable. ai-jail forces it to begin with `(jail) `. This signal is
  weaker, because a shell can overwrite the variable. A false negative only shows
  the warning again inside the jail, which is safe. A false positive is considered
  unlikely because ai-memory is started directly by ai-jail, not through an
  interactive shell that would reset the variable (a confirmar, see the last section).
- **Windows**: always "not jailed".

Detection fails open to the safe side: if the signal is missing or unrecognized,
the process is treated as not jailed, so the §1 warning is shown rather than
silently skipped.

When the process is detected as jailed:

- the warning and the ai-jail offer are both skipped, and the run proceeds
  directly, so a user who types the sandbox command themselves sees no extra prompt;
- `--jail` and `--no-jail` are ignored: there is no nesting and no error. The
  one exception is the conflict check below, which still applies.
- no ai-jail binary is looked up, so a missing or unusable ai-jail is not an error.

Giving `--jail` and `--no-jail` together is always an error ("--jail and --no-jail
cannot be used together"), even inside a jail.

## 4. Claude "true yolo" (opt-in)

True yolo is off by default. It is enabled in one of two ways:

- per launch, with the `--true-yolo` flag;
- for every launch, with the top-level configuration key `claude_true_yolo`
  (environment variable `AI_MEMORY_CLAUDE_TRUE_YOLO`).

Effects:

- **Implies `--yolo`.** It requests the same warning (§1), the same offer (§2),
  and each harness's dangerous-mode mapping. Passing both flags is redundant but
  harmless. For every harness other than Claude, true yolo is the same as `--yolo`.
- **Claude only.** For the Claude harness, the launch adds a settings argument
  after the harness arguments. The argument is a JSON value that sets the
  permission default mode to `bypassPermissions`. Exact text appended as the last two
  arguments of the harness invocation, after the harness's own arguments and after
  the dangerous-mode flag: `--settings` and
  `{"permissions":{"defaultMode":"bypassPermissions"}}` (no `ask` key). Command-line settings take
  precedence over the user's and project's settings, so a default mode there
  (for example `auto` or `acceptEdits`) cannot narrow the run (a confirmar, see
  the last section).
- **Config key only upgrades.** `claude_true_yolo` never turns a launch without
  `--yolo` into a permission-bypassing launch. It applies only when `--yolo` or
  `--true-yolo` is already present, and in that case the warning is still shown.
- **Flag placement.** `--true-yolo` is recognized anywhere after the `run`
  command, including after native harness arguments. It is removed from the
  harness arguments and is never forwarded to the harness.

What true yolo deliberately does not do:

- It cannot silence an explicit `ask` rule. Claude Code enforces `ask` and `deny`
  rules in every permission mode. Permission arrays passed in the settings
  argument are merged with the user, project, and local scopes, not used instead of
  them. For that reason no empty `ask` array is injected. To run without those
  pauses, the user removes the `ask` rules from their own settings. `deny` rules
  never pause; they block. Keeping them costs no interruptions (a confirmar).
- It cannot skip Claude Code's built-in command-safety checks.
- It sets no environment variables. Earlier releases set three environment
  variables that Claude Code does not read, and an empty `ask` array. Both were
  removed, because they implied a protection that did not exist (see the project
  changelog).

True yolo is documented as "best paired with a clean sandbox", that is, with ai-jail.

## 5. Choosing what the jail exposes

ai-jail keeps credentials and host devices out of the sandbox by default. Each
one is exposed through a boolean toggle (`--X` / `--no-X`). A jailed agent that
cannot reach the GitHub CLI, the AWS CLI, or an SSH key often cannot finish its
task. The user therefore chooses what the jail mounts, either with flags or with
an interactive checklist. Pressing Enter does the sensible thing.

### 5.1 Command-line flags

Two wrapper flags are accepted on `ai-memory run`. They are recognized after the
harness name too, like `--yolo`, and never reach the harness.

| Form | Meaning |
|---|---|
| `--jail` | Re-run inside ai-jail now, without the "Re-run inside it?" question and without the checklist, using the smart defaults (§5.3). |
| `--jail=github,aws,no-mise` | Same, with exactly the listed toggles. `no-X` forces one toggle off. Every checklist row shown on this host that the list does not name is forced off too. |
| `--jail=all` / `--jail=none` | Every checklist row shown on this host on, or every one of them off. Entries are applied left to right, so `all,no-docker` works. |
| `--no-jail` | Never re-run inside ai-jail. The §1 warning still shows. Conflicts with `--jail`. |

Notes:

- `--jail` requires `=` before its list. `ai-memory run --jail claude` therefore
  keeps `claude` as the harness name.
- `--jail` works with or without `--yolo`, and in non-interactive runs too. That
  is how a script gets a jailed run.
- When ai-jail is not usable on the host, an explicit `--jail` fails with an error
  instead of silently running unjailed. Exact message (single line): "--jail: ai-jail is not usable on this host (it needs `ai-jail` plus its sandbox backend, bwrap on Linux or sandbox-exec on macOS, and does not run on Windows); refusing to run unjailed — drop --jail to run without it". The check runs after the `--jail`/`--no-jail` conflict check and after the harness executable is verified, and before any server contact.
- An explicit `--jail` re-execs before the managed run is prepared, so no lease is
  opened and then cancelled outside the jail.
- Inside ai-jail both flags are ignored (§3).
- When several `--jail` or `--no-jail` arguments appear, the last `--jail` wins.

There are deliberately no bare toggle flags on `ai-memory run`, such as
`--github` or `--aws`. They would collide with harness-native flags of the same
name, and a wrapper flag after the harness name is removed from the harness
arguments, so a native flag would be silently taken away. The single namespaced
`--jail=` list has no such collision.

### 5.2 Toggles

| Toggle | Kind | Shown in checklist when | Smart default (Enter, or bare `--jail`) |
|---|---|---|---|
| `github` (`~/.config/gh`) | credential | that path exists | on when present |
| `aws` (`~/.aws`) | credential | that path exists | on when present |
| `kube` (`~/.kube`) | credential | that path exists | on when present |
| `gcloud` (`~/.config/gcloud`) | credential | that path exists | on when present |
| `docker-config` (`~/.docker/config.json`) | credential | that file exists | on when present |
| `ssh` (`~/.ssh` read-only, plus the SSH agent socket) | credential | `~/.ssh` exists, or an SSH agent socket is set | on only when `git push` to `origin` uses SSH |
| `worktree` | capability | the checkout is a linked git worktree | on there |
| `docker` (socket; grants host root) | capability | always | off |
| `gpu`, `display` | capability | Linux only | off |
| `pictures`, `tailscale` | capability | always | off |
| `audio`, `x11`, `host-shm`, `terminal-passthrough`, `update-check`, `mise`, `toolchains` | capability | never; `--jail=` only | not passed |

#### 5.2.1 Exact labels, notes and order (checklist rows)

Rows appear in this fixed order (numbers are assigned after hiding the invisible
ones). The label and the note are printed verbatim; the label column is padded to
the longest visible label and followed by three spaces and the note. Row layout:
`  [x] <n>) <label padded>   <note>` (`[ ]` when unchecked).

| # | Stem | Label | Note | Kind | Needs ai-jail |
|---|---|---|---|---|---|
| 1 | `github` | GitHub CLI credentials | `~/.config/gh, read-only` | credential | 2.5.0 |
| 2 | `aws` | AWS credentials | `~/.aws, read-only` | credential | 2.5.0 |
| 3 | `kube` | Kubernetes credentials | `~/.kube, read-only` | credential | 2.5.0 |
| 4 | `gcloud` | Google Cloud credentials | `~/.config/gcloud, read-only` | credential | 2.5.0 |
| 5 | `docker-config` | Docker registry credentials | `~/.docker/config.json, read-only` | credential | 2.5.0 |
| 6 | `ssh` | SSH keys + agent | `~/.ssh read-only + SSH_AUTH_SOCK; needed for git over SSH` | credential | 2.4.1 |
| 7 | `worktree` | Linked worktree metadata | `writable, so the agent can commit here` | capability | 2.4.1 |
| 8 | `docker` | Docker socket | `⚠ grants host root` | capability | 2.4.1 |
| 9 | `gpu` | GPU devices | `/dev/dri, /dev/nvidia*` | capability | 2.4.1 |
| 10 | `display` | X11/Wayland display | `GUI apps and the clipboard` | capability | 2.4.1 |
| 11 | `pictures` | Pictures folder | `~/Pictures, read-only` | capability | 2.4.1 |
| 12 | `tailscale` | Tailscale socket | `reach your tailnet` | capability | 2.4.1 |
| – | `audio`, `x11`, `host-shm`, `terminal-passthrough`, `update-check`, `mise` | Host audio, X11 socket, Host shared memory, Terminal passthrough, Update check, mise integration | (empty) | capability | 2.4.1 |
| – | `toolchains` | Toolchain caches | (empty) | capability | 2.5.0 |

The last two lines are accepted only through `--jail=`; they never appear in the
checklist. "Visible" additionally requires the installed ai-jail to advertise the
flag (§5.9). The credential rows test the existence of the path under the `HOME`
variable (the environment variables that relocate a tool's configuration, such as
`GH_CONFIG_DIR` or `XDG_CONFIG_HOME`, are never consulted by ai-memory). A
credential row is pre-checked exactly when it is visible. `ssh` is visible when
`~/.ssh` exists or `SSH_AUTH_SOCK` is set and non-empty; `worktree` is visible
(and pre-checked) only in a linked worktree; `gpu` and `display` are visible only
on Linux. A project `.ai-jail` makes every pre-check false.

The `ssh` default reads the URL that `git push` to `origin` uses, not the fetch
URL. This includes an HTTPS fetch URL whose push URL is rewritten to SSH. An SSH
push URL is any of: a `ssh://` URL (or the `git+ssh` and `ssh+git` spellings),
or the scp-style `[user@]host:path` form, where `host` may be an alias from the
SSH configuration. A one-letter host before a colon is a Windows drive, not SSH.
An HTTPS URL is never SSH.

The credential rows check the exact path that ai-jail mounts. The global
environment variable that points the GitHub CLI elsewhere is not consulted, because
the mount is fixed to `~/.config/gh` (a confirmar, see the last section).

### 5.3 Why these defaults

- Every credential that is present is pre-checked. A user who keeps GitHub or AWS
  credentials on the machine almost always wants the agent to use them. A row is
  shown only when its path exists, so a checked box always mounts something.
- SSH is shown but left unchecked for an HTTPS remote, because only an SSH
  `origin` makes `git push` need it.
- The worktree row is pre-checked in a linked worktree. Without its metadata
  mounted writable, the agent cannot commit there.
- Host capabilities stay off. They widen the sandbox and are not needed for the
  usual task. The Docker socket is never on by default: it grants effective root on
  the host, and its checklist note says so.

### 5.4 An explicit selection is exact

ai-jail's own configuration can also enable toggles. The trusted global
configuration file may turn on, for example, `docker`. Whenever the user makes a
selection, ai-memory therefore states every visible row explicitly, and ai-jail's
command-line flags override its configuration:

- **Interactive checklist.** Every row the user saw is passed as marked: checked
  rows as `--X`, unchecked rows as `--no-X`. An unchecked Docker row stays off even
  when the global configuration enables it. The checklist is what you see is what
  you get.
- **`--jail=LIST`, including `none`.** The named entries are applied in order, so
  later entries win. Then `--no-X` is added for every visible checklist row the list
  did not name. `none` therefore turns off every checklist row. Rows that are not
  visible, such as absent credentials and capability-only toggles, are never forced:
  an absent credential mounts nothing either way, and those toggles stay out of the
  invocation unless named.
- **Bare `--jail`.** The user saw no selection, so only the smart-default rows are
  enabled. Unchecked rows emit nothing, and the user's own ai-jail configuration
  still decides them.

### 5.5 A project `.ai-jail` file replaces the checklist

ai-jail reads its project configuration only from the directory the process was
started in. The re-exec inherits that directory. If a regular file named `.ai-jail`
exists there:

| Case | No project `.ai-jail` | Project `.ai-jail` present |
|---|---|---|
| Offer accepted, no flag | Checklist; every row passed as marked | No checklist and no toggles; the file is loaded as-is |
| Bare `--jail` | Smart-default rows on; the rest left to ai-jail configuration | No toggles; the file is loaded as-is |
| `--jail=LIST` | Exactly the list; unnamed visible rows `--no-X` | The same exact list, applied on top of the file (command-line flags override configuration) |

A project file is untrusted, because it lives in the repository the agent works
on. ai-jail lets it disable capabilities but never enable credentials, the Docker
socket, or agent state (a confirmar). For that reason ai-memory never pre-checks
anything on the file's behalf, and a repository cannot steer the checklist through
it. Only the presence of the file is checked. Its contents are never read or parsed.

The global ai-jail configuration file is not consulted. ai-jail writes it for its
own status-bar preferences, so most users have one. To get credentials mounted
automatically next to a project `.ai-jail`, the user enables them in the trusted
global file, or passes `--jail=github,...`. The project file cannot enable them.

### 5.6 Checklist

Without either flag, and without a project `.ai-jail` file, the checklist appears
after the user accepts the §2.2 offer. It lists the rows visible on this host with
their smart defaults:

```text
Enable in the jail (Enter = as marked; numbers flip, e.g. "2 4"; "all" / "none"):
  [x] 1) GitHub CLI credentials   ~/.config/gh, read-only
  [x] 2) AWS credentials          ~/.aws, read-only
  [ ] 3) SSH keys + agent         ~/.ssh read-only + SSH_AUTH_SOCK; needed for git over SSH
  [ ] 4) Docker socket            ⚠ grants host root
>
```

Input rules:

- Enter, or end of input, accepts the marks as shown.
- Row numbers, separated by spaces, commas, or tabs, flip those rows, and the list
  is shown again. A line with any unknown number changes nothing.
- `all` or `none` set every row.
- An unrecognized answer changes nothing and asks again. The third unrecognized
  answer aborts the launch, rather than guessing what to mount. Each unrecognized answer prints (stderr) "`<token>` is not a row number between 1 and <N>; press Enter to accept, or type numbers, `all`, or `none`", where `<token>` is the first offending token; the abort error is "too many unrecognized answers to the ai-jail checklist" and the already prepared managed run is then cancelled like any failure. Input is read line by line from standard input; the prompt is `> ` and the whole list is redrawn after every change. Case is ignored (`ALL`, `None`). A repeated number in one line flips twice (no net change). End of input prints an empty line and accepts the marks as shown. Row numbers follow the display order of §5.2 (only visible rows are numbered, from 1).
- When no row is visible, the checklist is skipped.

After the checklist, one summary line is printed (§5.7).

### 5.7 Summary line

Before the re-exec, one line on standard error says what the jail receives. It
groups the enabled toggles into credentials and capabilities, then lists the
user's explicit `no-X` entries as "forced off", then says "everything else in the
checklist off" when an explicit selection left rows out, and "plus the project
`.ai-jail`" when that file takes over. With nothing enabled it says the re-run
has no extra mounts. Exact shapes (stderr, one line): `ai-memory: re-running inside ai-jail (<part>; <part>; …)` with the parts, in this order and only the non-empty ones, `credentials: <label>, <label>`, `capabilities: <label>, …` (labels from the table in §5.2.1; "credentials" are the toggles of kind credential that are on, including SSH), `forced off: --no-<X> --no-<Y>` (only the `no-X` entries the user typed, with the flag spelling, separated by spaces), `everything else in the checklist off`, `plus the project .ai-jail`; and, with no parts, `ai-memory: re-running inside ai-jail with no extra mounts`.

### 5.8 Decision table

| | Already inside ai-jail | `--no-jail` | `--jail[=…]` | Neither flag |
|---|---|---|---|---|
| `--yolo` warning (interactive only) | No | Yes | Yes | Yes |
| ai-jail | No (no nesting) | Never | Re-exec now; error if not usable | Offer and checklist when interactive and usable; no checklist with a project `.ai-jail` |

Where the warning is shown with an explicit `--jail`, the user confirms once, and
no ai-jail question is asked, because the re-exec is already decided.

### 5.9 Version requirements

- ai-jail rejects unknown flags, so ai-memory passes only flags the installed
  ai-jail supports. It reads the installed ai-jail's help output, once per launch
  that needs it.
- A toggle counts as supported only when the exact `--X` token appears in that
  help text. Tokens are split on whitespace and on `/`, so `--docker` and
  `--docker-config` stay distinct, and a substring match never counts.
- Unsupported toggles are hidden from the checklist. A toggle named explicitly in
  `--jail=` that is unsupported is an error. The error names the ai-jail release
  that has it.
- The credential mounts (`github`, `aws`, `kube`, `gcloud`, `docker-config`) and
  `toolchains` need ai-jail 2.5.0. The other toggles are available from 2.4.1.
- `--no-save-config` is passed only when the installed ai-jail supports it. Both
  2.4.1 and 2.5.0 are documented to support it (a confirmar).

### 5.10 Switches that are never offered

`--jail=` refuses the following with the single-line message "--jail: `<name as typed>` is not available through ai-memory; run ai-jail directly" (the check lowercases the entry, strips a `no-` prefix, and runs **before** the unknown-name check, so `no-network` is also refused):

- switches that weaken or reconfigure the sandbox itself: `private-home`, `lockdown`,
  `landlock`, `seccomp`, `rlimits`, `systemd-user`, `inherit-env`,
  `macos-host-ipc`, `browser`, and the audit flags (`audit-log`, `audit-show`, `audit-verify`);
- switches that ai-memory sets itself: `network`, `agent-state`, and `save-config`.

Anyone who needs one of these runs ai-jail directly. An unknown name is refused
with the list of valid toggles and the note that `all`, `none`, and a `no-` prefix
are also accepted: "--jail: unknown toggle `<name as typed>`; valid toggles: github, aws, kube, gcloud, docker-config, ssh, worktree, docker, gpu, display, pictures, tailscale, audio, x11, host-shm, terminal-passthrough, update-check, mise, toolchains, plus `all`, `none`, and a `no-` prefix to force one off". A known toggle the installed ai-jail does not advertise: "--jail: the installed ai-jail does not support --<toggle>; it needs ai-jail <release> or newer" (`<release>` = 2.5.0 for the credential toggles and `toolchains`, 2.4.1 for the others). Entries are comma-separated, trimmed, empty entries ignored; an empty list is a bare `--jail`.

### 5.11 Security notes

- A credential mounted into an unsupervised agent is usable by that agent.
  Read-only mounts stop the agent from changing the files, not from using the
  tokens inside them. Pre-checking present credentials trades that exposure for a
  jailed agent that can do its job. The checklist shows each credential, and
  `--jail=none` or unchecking a row opts out.
- The Docker socket grants effective root on the host. It is never on by default.
- The re-exec forwards only environment names that are already set (§2.3). It adds
  no new secret surface. The toggle flags carry no values or secrets.

## 6. Behavior per operating system

| Concern | Linux | macOS | Windows |
|---|---|---|---|
| ai-jail usable | Yes, with the Linux sandbox backend | Yes, with the macOS sandbox backend | Never; no offer |
| Already-inside detection | Hostname `ai-sandbox` | Prompt variable starts with `(jail) ` | Always "not jailed" |
| `--network` reaches the host loopback server | Yes (host network shared) | Depends on the sandbox profile (a confirmar) | Not applicable |
| Warning prompt (interactive only) | Yes | Yes | Yes |
| Claude true-yolo settings argument | Yes | Yes | Yes |
| `--jail` explicit | Re-exec or error | Re-exec or error | Error (ai-jail not usable) |

## 7. Security considerations

- The warning and the offer require an interactive terminal. No new prompt reaches
  hook, CI, detached, or piped paths.
- The re-exec forwards only environment names that are already set, adds no new
  secret surface, and does not log token values.
- True yolo is off by default, applies to Claude only, and is documented as
  sandbox-first.
- Detection fails open to the safe side (§3).
- Before any re-exec, the prepared managed run is cancelled, so the jailed run opens
  its own run cleanly instead of waiting for an orphaned lease to time out.

## 8. Verification

The behavior above is covered by tests of these properties, which can be run
without a real sandbox:

- ai-jail usability per operating system, with an injected search path, including
  the fallback location and the rule that Windows never offers.
- Already-inside detection per operating system, with injected environment and hostname.
- Re-exec argument assembly: the baseline flags, environment names only when set,
  the current executable and argument forwarding, and the placement of toggles
  before the separator.
- Toggle parsing: lists, negations, `all`, `none`, unknown and reserved names, and
  error messages.
- Support detection from help text for each supported ai-jail release.
- Smart defaults with an injected home directory, including the project-file rules.
- The decision table, flag stripping, and the checklist and confirmation input rules.
- Claude true yolo: the settings argument is added for Claude and nothing is added
  for other harnesses.
- End-to-end runs with a fake ai-jail, a fake harness, and a mock server. These
  cover the offer and checklist on a terminal, `--yolo --no-jail`, and the failure
  when ai-jail is not usable. An optional check runs a real ai-jail dry run, when one
  is installed, to confirm that every supported toggle is accepted.

## 9. External assumptions

These statements describe external tools. This repository does not contain their
source, so they are not verified here (a confirmar):

- ai-jail: the hostname `ai-sandbox` is set on the Linux backend; the macOS
  backend forces the prompt variable to start with `(jail) `; dry-run does not
  execute the child or write configuration, but applies backend trust checks.
- ai-jail: it clears the environment by default; the network flag only disables
  network isolation when set; a filtered host mode uses a private network namespace
  with a proxy for external hosts.
- ai-jail: it exports no dedicated marker variable.
- ai-jail: a project configuration file may disable capabilities but never enable
  credentials, the Docker socket, or agent state; the global configuration file is
  written for status-bar preferences.
- ai-jail: the GitHub CLI credentials are mounted from the fixed path regardless of
  the environment variable that moves them.
- ai-jail: the separator behavior described in §2.3, and the release in which each
  toggle appears (§5.9).
- Claude Code: the settings argument has higher precedence than user and project
  settings; `ask` and `deny` rules and built-in command-safety checks are enforced in
  every permission mode; permission arrays from different scopes are merged.
- Claude Code: the external documentation lists the actions no permission mode
  auto-approves.

### 9.1 Resolution of the "a confirmar" items against the reference implementation

Each item below was checked against the reference implementation's own source and
tests (the original is read-only evidence). "External" means the answer lives in a
tool that the reference repository does not contain; the original only relies on it,
so the item stays a documented assumption and is **not** resolved here.

| Item (section) | Status | Evidence |
|---|---|---|
| `--no-save-config` exists in ai-jail 2.4.1 and 2.5.0 (§5.9) | **Resolved** | The original's end-to-end test with a real ai-jail asserts that "every ai-jail since 2.4.1 advertises `--ssh`, `--gpu` and `--no-save-config`". The help text of 2.4.1 kept as a fixture lists `--save-config / --no-save-config`. |
| `--network` only disables isolation when set; default off (§2.3, §9) | **Resolved textually** | The 2.4.1 help fixture reads `--network / --no-network  Enable/disable unrestricted network access (default: off)`. |
| Docker socket, SSH, Tailscale and pictures are off by default (§5.2, §5.3) | **Resolved textually** | Same fixture: each line says "(default: off)"; the Docker line adds "grants host root". |
| The GitHub CLI credentials are mounted from the fixed path (§5.2, §9) | **Resolved for ai-memory's side** | The 2.5.0 fixture reads `--github / --no-github  Enable/disable read-only ~/.config/gh mount`. ai-memory never reads `GH_CONFIG_DIR` or `XDG_CONFIG_HOME` (its end-to-end tests clear them from the environment precisely so they cannot influence the checklist). What ai-jail does if those variables are set stays external. |
| The `--` separator and the release in which each toggle appears (§2.3, §5.9) | **Resolved for the toggle table** | The table in §5.2.1 (2.5.0 for credentials and `toolchains`, 2.4.1 for the rest) is the one the original compares against. The changelog statement about 2.4.2 and flag collisions is not in the original's source (external). |
| Claude Code `--settings` precedence; `ask`/`deny` rules and command-safety checks enforced in every permission mode; permission arrays merged (§1, §4) | **External** | The original asserts these in comments citing Claude Code's documentation ("Actions no mode auto-approves"); the only code consequence is the exact settings value of §4 and the absence of an `ask` key, both now documented. |
| The Linux hostname is `ai-sandbox`; the macOS prompt starts with `(jail) `; no marker variable (§3) | **External** | The original's detection reads exactly those two signals (§3) and treats anything else as "not jailed". |
| A project `.ai-jail` can disable but never enable credentials, the Docker socket or agent state (§5.5) | **External** | The original relies on it (comment in the toggle-defaults code) and never reads the file's contents. |
| `--network` reaches the host loopback server on macOS (§6) | **External** | Nothing in the original tests the macOS network profile. |
| Dry-run neither executes the child nor writes configuration but applies backend trust checks (§2.1) | **External** | The original only invokes `<ai-jail> --dry-run -- <current executable>` and checks the exit status. |

For the related support and platform matrix, see
[support-matrix.md](support-matrix.md). For the sandbox boundaries of the whole
system, see [security-boundaries.md](security-boundaries.md). For Windows
specifics, see [windows.md](windows.md). For the run command's flags, see
[cli.md](cli.md).
