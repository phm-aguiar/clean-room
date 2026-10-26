# Backups and remote git mirror

> **Escopo:** este documento contém partes que dependem de LLM ou de embeddings. Essas partes são opcionais e não fazem parte do escopo inicial, que funciona sem LLM. Podem ser removidas ou adiadas junto com as funções correspondentes.

> This document covers backup patterns beyond the built-in snapshot command,
> `ai-memory backup --to <tarball>` (see
> [`lifecycle-ops.md#backup`](lifecycle-ops.md#backup)), and the point-in-time
> copy idiom in [`deploy.md#backups`](deploy.md#backups). If you already have a
> working tarball routine, treat this document as an additional layer.

The wiki directory is a git repository. The server commits its contents
automatically: at the end of a session, after consolidation writes pages, and
after some administrative writes. The wiki therefore already has its own commit
history, and that history can be pushed to any git remote you control. The rest
of this document covers how to set that up, what to include and exclude, and how
to schedule it.

## What the data directory contains

`ai-memory status` prints the resolved data directory. Its JSON output has a
`data_dir` field. The layout is:

```text
data/
├── wiki/    # markdown source of truth; a git repository
├── raw/     # created at initialization; immutable managed-workstream event segments
├── db/      # memory.sqlite: search index plus database-only state
├── logs/    # daily rolling log files
└── models/  # local embedding model files
```

Two more items live in the data directory. `config.toml` is the default
configuration file (the config file location defaults to the data directory).
`.serve.lock` is a lock file held by the running server.

Back up by durability:

| Item | Back up? | Why |
|---|---|---|
| `wiki/` | **Yes, always.** | Markdown source of truth. Nothing else can recreate it. It is a git repository with its own history. |
| `raw/` | **Yes, if you use managed workstreams** (otherwise empty). | Created at initialization (with `wiki/`, `db/`, `models/`). Resolved from the reference code: it holds the immutable event segments of managed workstream runs, `raw/workstreams/<workstream id>/segments/<run id>-<first 16 hex of the segment SHA-256>.jsonl` (one JSON event per line, written to a temp file, synced and renamed, never rewritten). The database row of a finished run keeps the relative `segment_path`, so the segment is the source of that history; the server runs without `raw/`, but the history it points to is lost. The `backup` command does **not** include it, and neither does the example mirror unless you copy the data directory (the example script does copy it). `reset`, `purge-project` and workspace deletion remove the segments of the workstreams they delete. |
| `db/` | **Yes, if you want anything that is not in the markdown.** | Holds the search index, which `ai-memory reindex` rebuilds from `wiki/`. It also holds database-only state: sessions, observations, handoffs, users, audit rows, embeddings, and decay counters. Rebuilding from markdown does not restore that state. The database uses write-ahead logging, so copy it only with the server stopped or through the `backup` command. See [`lifecycle-ops.md#reindex`](lifecycle-ops.md#reindex). |
| `logs/` | No. | Daily rolling log output. Nothing durable. |
| `models/` | No. | Local embedding model, about 87 MB. Files are checked against pinned checksums. They are downloaded again when missing at server startup. See [`local-embeddings.md`](local-embeddings.md). |
| `.serve.lock` | No. | Lock file of the running server. |
| `config.toml` | Yes, after checking it. | Useful to reproduce an install. Check it for inline secrets before pushing it anywhere: it **can** hold them. The `[auth]` section accepts `bearer_token`, `initial_root_password`, `recovery_token`, `token_pepper` and `actor_proxy_bearer_token` inline (prefer `bearer_token_file`, a relative path under the data directory, e.g. `auth-tokens/default`, which is also not part of a mirror you should publish). Provider API keys are read from the process environment only and are not config-file fields (profiles name the variable with `api_key_env`). |
| Credential files | **Never.** | Env files that hold provider keys, the OAuth token file `auth.json`, and certificate or private key files (`*.pem`, `*.key`, `*.crt`). |

If you need the database for quick recovery without running `reindex`, pair the
git mirror with a periodic `ai-memory backup --to <tarball>` and keep the
tarball on a second location (an external disk, a network share, or another
machine you control).

## Security: what ends up in your wiki

Read this before you configure a remote. Decide what may leave your machine,
then pick the backup shape that matches.

The wiki stores what the coding agent sends during a session that the server
classifies as observations: user prompts, tool inputs and outputs, and page
bodies written by consolidation. Before storage, a privacy strip redacts
obvious credentials: bearer tokens, vendor-prefixed API keys, PEM private key
blocks, credentials embedded in URLs, and generic `KEY=value` assignments for
names such as `TOKEN`, `SECRET`, or `PASSWORD`. The strip cannot prove that every
prompt or tool response was safe to store. If anyone ever pasted an API key, a
password, a client name, a customer record, or a sensitive path into a captured
session, that text is on disk in the wiki tree, and anything you push carries it.

For any remote target, even a private repository on a service you control, assume
that an administrator on the receiving side can read the contents. Employer-managed
hosts are often audited, and a repository that is private to you on a shared
account is still readable by whoever administers that account.

**Suggested exclusions.** The mirror repository's ignore file should carry the
rules below. This is the list in the example ignore file under
[`assets/examples/backup/`](assets/examples/backup/):

```text
# Provider credentials and OAuth material
config/*.env
config/**/*.env
config/auth.json
config/**/auth.json
config/.secrets
config/**/.secrets

# Server lock file
data/.serve.lock

# Database: search index plus database-only state
data/db/

# Local embedding model files
data/models/

# Rotated logs
data/logs/

# Scratch files
*.tmp
*.swp
```

The example ignore file does not include the certificate and key globs that the
example script excludes. Add `*.pem`, `*.key`, and `*.crt` to the ignore file,
both at the top level and under `config/` and `data/`. Add any other
install-specific secret files, such as host certificates or cloud credential files.

**Alternatives when wiki content is too sensitive to store as text on a remote.**
These are third-party tools. The project ships no example for them, so their
exact commands are a confirmar.

- **Encrypted archive to object storage.** Snapshots of the data directory made
  with a backup tool that encrypts with a passphrase-derived key, sent to a
  private object store. The storage operator sees only ciphertext.
- **Selective encryption inside the mirror.** A git encryption filter on paths
  that carry sensitive material, such as `data/wiki/**/*.md`. Keys must exist
  before the first commit; encrypting existing history is a manual step.
- **Encrypted tarball to a second location.** Pipe a tarball of the data
  directory through a file encryption tool and store the ciphertext on an external
  disk or a network share.
- **Local-only backup.** Rotate encrypted tarballs onto an external drive kept
  off-site. This gives up remote git convenience for zero third-party exposure.

If none of the wiki content is more sensitive than your project source
repository, a private mirror repository is fine. This section applies when it is not.

## Backing up the wiki to a remote git repository

The wiki is already a git repository. If your only goal is off-site storage of the
wiki, add a remote and push:

```text
cd "<data dir>/wiki"
git remote add origin <remote-url>
git push -u origin <branch>
```

Find the data directory with `ai-memory status --json` (field `data_dir`). Find
the branch with `git branch --show-current`. The code does not set the initial
branch name of the wiki repository, so it depends on the host's git defaults.

Two caveats:

- The server writes to the wiki repository. Do not run interactive or history-rewriting
  operations (rebase, merge, force push, reset) against it while the server runs.
  Read-only commands and `git push` are safe.
- The wiki carries page bodies verbatim, including content the model synthesized.
  Treat the remote like project source: private repository, restrictive access
  controls.

## Backing up the whole data directory (mirror and push)

For a full-install backup (wiki, `raw/`, and configuration), the supported pattern
is a scheduled copy into a mirror git repository, followed by a commit and a push.
Git operations run only against the mirror, never against the live wiki repository.

The copy reads live files while the server may be writing. The example does not
stop the server and does not take a database snapshot. Each wiki file is written
atomically (see invariant 10 in [`overview.md`](overview.md) section 4), so every
copied file is complete. The set of files, however, may not reflect one moment in
time.

A worked example lives in [`assets/examples/backup/`](assets/examples/backup/):

- `ai-memory-snapshot.sh`: the daily snapshot script.
- `ai-memory-backup.service`: a user-level oneshot unit that runs the script.
- `ai-memory-backup.timer`: a user-level daily timer.
- `.gitignore`: the ignore list to copy into the root of the mirror repository.

The script reads eight environment variables. Each has a default:

| Variable | Default | Meaning |
|---|---|---|
| `REPO_DIR` | `~/ai-memory-backup` | Local mirror checkout. Must be a git repository, or the run stops. |
| `DATA_SRC` | `~/.local/share/ai-memory` | Data directory to copy. Must exist, or the run stops. |
| `CFG_SRC` | `~/.config/ai-memory` | Configuration directory to copy, with secret exclusions. |
| `TARBALL_DIR` | `~/backups/ai-memory` | Destination of the daily archive. Created if missing. |
| `LOG_DIR` | `~/logs/ai-memory-snapshot` | One log file per run. All output of a run goes to that file. |
| `TARBALL_RETENTION_DAYS` | `30` | Archives older than this are deleted. |
| `LOG_RETENTION_DAYS` | `30` | Run logs older than this are deleted. |
| `SECRET_EXCLUDES` | `*.env auth.json .secrets *.pem *.key *.crt` | Glob patterns that are never copied. |

The defaults use Linux-style paths. On other systems, find the data directory with
`ai-memory status` and set `DATA_SRC` to match.

One invocation runs these steps:

1. **Copy the data directory** into the mirror, with `--delete` semantics. Excluded:
   any `.git` directory (so the wiki's own history is not copied), `models/`,
   `db/`, `logs/`, `.serve.lock`, and every `SECRET_EXCLUDES` pattern.
2. **Copy the configuration directory** into the mirror, with `--delete` semantics.
   Excluded: `.git` directories and every `SECRET_EXCLUDES` pattern. If `CFG_SRC`
   is missing, the copy step fails and the run stops (a confirmar).
3. **Stage and commit.** Stage everything. If the index has changes, commit with the
   message `auto: snapshot <date>`. If there are no changes, skip the commit and
   the push.
4. **Push** the current branch of the mirror, with a 180-second timeout. The script
   runs with strict failure handling, so a failed or timed-out push stops the run
   here: the archive and the rotation steps are then skipped for that day.
5. **Write the archive.** Create `ai-memory-<date>.tar.gz` in `TARBALL_DIR` from the
   data and configuration directories themselves (not from the mirror), with the
   same exclusions plus `.git`. A same-day run overwrites the previous archive. The
   archive excludes `db/` and `models/`, so it does not contain the database.
6. **Rotate.** Delete archives and run logs older than their retention windows.
   Only files that match the archive and log name patterns are deleted.

The script exits with an error before copying anything when `DATA_SRC` does not
exist or when `REPO_DIR` is not a git repository.

### Scheduling

The service unit is a oneshot. It runs the script, at lower CPU priority (nice 10)
and with best-effort I/O priority 7. It starts after the network is online.

The timer runs once a day. It adds a random delay of up to 5 minutes, has an
accuracy window of 15 minutes, and is persistent, so a run missed while the
machine was off fires at the next start.

Other schedulers can run the same script. Examples: a launch agent on macOS, a
scheduled task on Windows that runs the script inside a POSIX environment, or a
small cron container mounted on the data volume when the server runs in Docker.
The project ships no example for these, so their exact configuration is a confirmar.

## Installing the example (quickstart)

The commands below assume `EXAMPLES` points to the example directory
(`assets/examples/backup/` in this documentation set).

```text
EXAMPLES=/path/to/ai-memory/docs/assets/examples/backup

# 1. Create a private repository on your git host, then:
mkdir -p ~/ai-memory-backup
cd ~/ai-memory-backup
git init
git remote add origin <remote-url>
cp "$EXAMPLES/.gitignore" .
git add .gitignore
git commit -m "initial: ignore secrets, database, models, logs"
git branch -M main
# Push once so the snapshot script has a remote to push to.
git push -u origin main

# 2. Put the snapshot script somewhere on PATH.
install -Dm755 "$EXAMPLES/ai-memory-snapshot.sh" ~/.local/bin/ai-memory-snapshot.sh

# 3. Install the unit files.
install -Dm644 "$EXAMPLES/ai-memory-backup.service" ~/.config/systemd/user/ai-memory-backup.service
install -Dm644 "$EXAMPLES/ai-memory-backup.timer" ~/.config/systemd/user/ai-memory-backup.timer

# 4. Enable and start the timer.
systemctl --user daemon-reload
systemctl --user enable --now ai-memory-backup.timer

# 5. Trigger one snapshot to confirm the setup works.
systemctl --user start ai-memory-backup.service
journalctl --user -u ai-memory-backup.service --since '5 minutes ago'
```

The script refuses to run unless `REPO_DIR` contains a git repository, so the
`git init` in step 1 must come first.

## Restoring

There are two restore paths. Choose by what you need back.

### From the mirror repository

The mirror is a copy of the data and configuration directories minus derived
state and secrets. It has no `db/`, no `models/`, no `logs/`, and no wiki git
history. Recovery has two steps:

1. Clone the mirror on the target host. Copy its `data/` contents into the data
   directory and its `config/` contents into the configuration directory. Use an
   empty data directory, or move the existing one aside first.
2. With the server stopped, and with no `db/` directory in the data directory, run
   `ai-memory reindex`. It rebuilds the index from `wiki/`: it recreates workspaces
   and projects from their manifest files and reindexes every page. Embeddings are
   not computed by reindex. Run `ai-memory embed` afterwards. See
   [`lifecycle-ops.md#reindex`](lifecycle-ops.md#reindex).

What the mirror cannot restore:

- Sessions, observations, handoffs, users, audit rows, embeddings, and decay counters.
  These exist only in the database. Restore them from a `backup` tarball.
- The wiki's previous git history. The mirror does not copy the wiki's own
  repository metadata. When the server starts on a wiki without that metadata, it
  creates a new repository and starts a new history from that point. The mirror
  repository keeps its own snapshot commits.

### From a backup tarball

```text
ai-memory --data-dir <path> restore --from <tarball> --force
```

- `--force` is required when the data directory is not empty. Without it, the
  command refuses to run.
- The command refuses to run while another `ai-memory` process is alive.
- The archive is extracted into a staging directory inside the data directory. The
  restored database is opened there, which applies pending migrations and fails
  if the database is corrupt. Only then are the live `wiki/` and `db/` directories
  swapped for the staged ones, and `config.toml` is replaced when the archive has
  one. If any step fails, the previous data is moved back.
- `logs/`, `models/`, and `raw/` are not touched. The archive does not contain
  them. The local embedding model is downloaded again at startup if it is missing.

The `backup` command requires the server to be running, because it asks the server
for the archive. The archive contains the wiki tree, a snapshot of the database taken
through the database's online backup mechanism, and `config.toml` when present.

## Security posture

Remote sync runs on a channel you own. The project does not push to any remote.
This is stated in [`airgapped-install.md`](airgapped-install.md). In practice:

- **Private repository, always.** The wiki captures prompts, tool calls, and
  synthesized page bodies verbatim, which is as sensitive as project source.
- **Least-privilege push credential.** Prefer an SSH deploy key or a fine-grained
  access token scoped to the mirror repository only. Rotate it on the same cadence
  as your other long-lived push credentials.
- **Explicit secret exclusion.** The example script excludes files that match
  `SECRET_EXCLUDES`. It excludes them silently and does not abort. The example
  ignore file covers only part of that list (see above), so the mirror's ignore
  file needs the rest.
- **No encryption at rest.** The data directory is a plain filesystem tree.
  Encryption is not provided by the project. If your threat model requires it, use
  an encrypted remote, encrypt the archives before they are stored, or both.

## Related documents

- [`lifecycle-ops.md`](lifecycle-ops.md): reference for `backup`, `restore`,
  `restore-page`, `reset`, and `reindex`.
- [`deploy.md#backups`](deploy.md#backups): the on-box archive routine and the
  copy-to-laptop idiom for container deployments.
- [`airgapped-install.md`](airgapped-install.md): offline installs. It states that
  remote git sync is the operator's responsibility.
- [`local-embeddings.md`](local-embeddings.md): how `models/` is fetched and pinned,
  which is why it does not need to be in the mirror.
- [`overview.md`](overview.md): the system overview and invariants, including the
  atomic write rule that the per-file copy guarantee depends on.
