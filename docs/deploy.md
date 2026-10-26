# Deploying ai-memory to a homelab

> **Escopo:** este documento contém partes que dependem de LLM ou de embeddings. Essas partes são opcionais e não fazem parte do escopo inicial, que funciona sem LLM. Podem ser removidas ou adiadas junto com as funções correspondentes.

This guide sets up a long-lived ai-memory container on a homelab host. The end
state: the server is reachable on your LAN at `http://<host>:49374/mcp`,
configured with your LLM and embedding keys, protected by a bearer token, and
backed up with whatever tooling you already use for your Docker data
directories.

Um serviço Linux nativo, no lugar do Docker, está fora do escopo atual.
The native system service keeps its data in `/var/lib/ai-memory` and its
configuration in `/etc/ai-memory/`; the native user service uses per-user paths.
The container keeps its data at `/data` inside the container and at the host
path you mount (by default `/var/opt/docker/utils/ai-memory/data`).

Published images are multi-architecture (`linux/amd64` and `linux/arm64`), so
homelab hosts on x86_64 and ARM64 can pull the same tag.

## What is committed and what stays local

The repository ships templates and a deploy script only. Each live file is a
template with the `.example` suffix removed. Live files hold your homelab values
and API keys.

| Committed | Live file you create | What it holds |
|---|---|---|
| Deploy script | none (safe to commit) | Build, push, and remote restart logic |
| `deploy.env.example` | `deploy.env` | `SERVER`, `DEPLOY_DIR`, `IMAGE`, and optional `CONTAINER_NAME` (default `ai-memory`) |
| Production compose template | `docker-compose.prod.yml` (copied to the server as `docker-compose.yml`) | Image tag, port mapping, data volume |
| `.env.production.example` | `.env.production` | LLM and embedding settings, API keys, auth token, allowed hosts |

Version control ignores the live files. If one shows up staged, something has
drifted: unstage it before committing.

## First-time setup

Run these once, from the directory that holds the templates.

```bash
# 1. Create the local config from the templates.
cp deploy.env.example deploy.env
$EDITOR deploy.env                # SERVER, DEPLOY_DIR, IMAGE (see "Routine deploys" for the tag rule)

cp docker-compose.prod.yml.example docker-compose.prod.yml
$EDITOR docker-compose.prod.yml   # image tag, port, data volume if you deviate from the defaults

cp .env.production.example .env.production
$EDITOR .env.production           # LLM provider and key, auth token, allowed hosts
```

```bash
# 2. Create the deploy directory on the homelab. The container runs as
#    UID 1000, so the data directory must be owned by that user.
source deploy.env
ssh "$SERVER" "sudo mkdir -p $DEPLOY_DIR/data && \
               sudo chown -R 1000:1000 $DEPLOY_DIR"

# 3. Copy the compose file and the env file to the homelab.
scp docker-compose.prod.yml "$SERVER:$DEPLOY_DIR/docker-compose.yml"
scp .env.production         "$SERVER:$DEPLOY_DIR/.env.production"

# 4. Run the first deploy (see "Routine deploys").
```

The compose template mounts `/var/opt/docker/utils/ai-memory/data` into the
container. With the default `DEPLOY_DIR` (`/var/opt/docker/utils/ai-memory`),
that is the `data` directory created in step 2. If you change `DEPLOY_DIR`,
change the volume path in the compose file to match.

**Set the allowed Host names before the first deploy.** The server rejects any
request whose `Host` header is not on its allowlist, with `403 forbidden host`.
The image default is `localhost,127.0.0.1,::1,host.docker.internal`. Setting
`AI_MEMORY_ALLOWED_HOSTS` replaces that list entirely, so include every name or
IP your clients use to reach the server:

```text
AI_MEMORY_ALLOWED_HOSTS=localhost,127.0.0.1,::1,host.docker.internal,homelab,192.168.1.50
```

After the first deploy the container should be running. Verify:

```bash
curl -i http://<homelab>:49374/mcp
```

- `Connection refused`: the container is not up, or the port is published on
  `127.0.0.1` instead of `0.0.0.0`.
- `403 forbidden host`: the name in your URL is not in `AI_MEMORY_ALLOWED_HOSTS`.
  Add it, copy the env file to the server, and run `docker compose up -d`.
- `401 Unauthorized`: a token is configured and this request did not send it.
  This is expected after the security step below.
- Any other response means the server is reachable. The exact response to an
  unauthenticated request before a token is set: a confirmar.

```bash
ssh "$SERVER" "docker inspect --format='{{.State.Health.Status}}' ai-memory"
```

Expect `healthy`. The container health check runs the server's `status` command
every 30 seconds, with a 5-second timeout, 3 retries, and a 5-second start period.
`starting` is normal for the first seconds; `unhealthy` is covered in
Troubleshooting.

## Security: bearer token and encrypted transport

A server bound to your LAN with no authentication lets anyone on the network call
destructive MCP tools: delete all pages, inject fake observations, or spend your
LLM budget. Turn on the bearer token before the first deploy.

How the server behaves:

- Outside a container, the server refuses to start on a non-loopback plain-HTTP
  address without a token, unless it is started with `--allow-insecure-no-auth`.
- Inside the image, the server starts without a token and prints a warning. It
  cannot tell from inside the container whether the host publishes the port on
  loopback or on the LAN. Set the token.
- With a token set, every machine route (`/mcp`, `/hook`, `/handoff`, and
  `/workstream/*`) requires `Authorization: Bearer <token>`. A missing or wrong
  credential gets `401`. The comparison is constant-time.

```bash
# 1. Generate a token: 32 random bytes, printed as 64 hex characters.
ai-memory generate-auth-token >> .env.production
$EDITOR .env.production           # prefix the new line with AI_MEMORY_AUTH_TOKEN=

# 2. Sync to the homelab and restart.
scp .env.production "$SERVER:$DEPLOY_DIR/.env.production"
ssh "$SERVER" "cd $DEPLOY_DIR && docker compose up -d"
```

The startup log line `MCP HTTP server ready` now carries `auth=true`. Verify from
another machine (`-I` sends a HEAD request):

```bash
curl -sI http://homelab:49374/handoff                  # HTTP/1.1 401 Unauthorized
curl -sI http://homelab:49374/handoff \
     -H "Authorization: Bearer $TOKEN"                 # HTTP/1.1 200 OK
```

The `200` has an empty body when there is no handoff to deliver. A scope refusal
returns `403` with the reason.

**Then update every MCP client** to send the same token. Run
`ai-memory install-mcp --client <name> --auth-token <token>` to print the snippet
for a client, and `ai-memory install-mcp --help` for the accepted client names.
The supported clients are listed in the [support matrix](support-matrix.md).
Commands run through the CLI send `Authorization: Bearer` with the token from
`AI_MEMORY_AUTH_TOKEN`.

**Encrypted transport.** Plain HTTP on the LAN lets anyone with a packet capture
read the bearer token (and native `aim_` API keys once multi-user mode is on).
When authentication is enabled and the server is bound to a non-loopback
plain-HTTP address, it logs a startup warning that bearer credentials travel in
cleartext. Put a TLS-terminating reverse proxy in front of the server when you
bind beyond loopback or turn on multi-user mode. Options include Caddy (with
Let's Encrypt or its internal CA), Cloudflare Tunnel, nginx with certificate
files, or any proxy you already run.

See [HTTPS via a reverse proxy](https-via-proxy.md) for the full guide: when to
add TLS and when to skip it, compose examples for common proxies, trust-store
setup for an internal CA, and
[hosting under a subpath](https-via-proxy.md#hosting-under-a-subpath) with
`--base-path` or `AI_MEMORY_BASE_PATH` when the server shares a hostname with
other apps.

For a single user on loopback, the token alone is enough: the token stops the
LAN neighbour, and loopback stops packet capture. TLS earns its keep once the
deployment stops being a single user on a single machine.

## Routine deploys

After first-time setup, run the deploy script from the repository root. The script:

1. Reads `SERVER`, `DEPLOY_DIR`, and `IMAGE` from `deploy.env`, and stops with an
   error if any is missing.
2. Refuses to run if `IMAGE` already resolves to a multi-architecture manifest.
   The script builds for the architecture of the machine it runs on only.
   Pushing that over a multi-architecture tag drops every other architecture, and
   hosts on those architectures fail with `exec format error` on their next pull.
   Release tags are published by CI, so deploy a private tag instead, for example
   `<your-registry-user>/ai-memory:homelab`, and point the compose file on the
   server at that tag.
3. Builds the image locally and pushes it to your registry.
4. On the homelab, runs `docker compose pull`, `docker compose down`, and
   `docker compose up -d` in `DEPLOY_DIR`. The server does not build the image;
   the binary travels inside it.
5. Waits three seconds and prints the container state and health.

The compose file and env file on the homelab stay the same between deploys. If
you change either, copy the new file and run the script again.

To run the published release without building your own image, skip the script and
run on the homelab:

```bash
cd $DEPLOY_DIR && docker compose pull && docker compose up -d
```

### Stopping and restarts

The restart stops the running container with SIGTERM, which is the default stop
signal for containers; the image does not override it.

The server handles SIGINT and SIGTERM on both of its transports (HTTP and stdio).
Each wait on the shutdown path is bounded at five seconds. The bound is fixed and
not configurable. Because each wait is bounded on its own, a stop can take a small
multiple of five seconds. A plain `docker stop` or `docker compose down` therefore
drains and exits within seconds, instead of waiting out the grace period and
ending in SIGKILL. `docker kill` still stops the server immediately, without a
drain.

The server runs as PID 1 inside the container and installs its own signal
handlers before it boots, so it stops correctly without an init wrapper. You do
not need `--init` or a separate init process. The native service installs the
same handlers, so a stop there is a bounded drain rather than an immediate kill.

## Updating API keys

```bash
$EDITOR .env.production
scp .env.production "$SERVER:$DEPLOY_DIR/.env.production"
ssh "$SERVER" "cd $DEPLOY_DIR && docker compose up -d"
```

`docker compose up -d` reads the env file and recreates the container with the new
values. No image rebuild is needed.

## LLM provider choices

The env template defaults to a model hosted on OpenRouter, through the
OpenAI-compatible transport:

```text
AI_MEMORY_LLM_PROVIDER=openai-compat
AI_MEMORY_LLM_BASE_URL=https://openrouter.ai/api/v1
AI_MEMORY_LLM_MODEL=moonshotai/kimi-k2.6
```

Accepted `AI_MEMORY_LLM_PROVIDER` values: `anthropic`, `openai`, `openai-oauth`,
`anthropic-oauth`, `copilot`, `gemini` (alias `google`), `openai-compat`, and
`opencode`.

If `AI_MEMORY_LLM_PROVIDER` is unset, no LLM is used. Consolidation falls back to
rule-based summaries, with lower-signal handoffs.

If `AI_MEMORY_LLM_MODEL` is unset, these providers use a default model:

| Provider | Default model |
|---|---|
| `anthropic` | `claude-haiku-4-5` |
| `openai` | `gpt-5.4-mini` |
| `gemini` | `gemini-3.5-flash` |
| `openai-oauth` | `gpt-5.5` |
| `copilot` | `gpt-5.5` |

`openai-compat` has no default model. Without `AI_MEMORY_LLM_MODEL` the server
fails at startup with an error that names that variable.

Reasonable alternatives:

| Provider | Model | Approx. cost per consolidation | Notes |
|---|---|---|---|
| anthropic | `claude-haiku-4-5` | a confirmar | Recommended default in this guide: a good balance of speed, restraint, and classification quality. Not a reasoning model. |
| openai-compat (OpenRouter) | `moonshotai/kimi-k2.6` | a confirmar | Reasoning model with long latency per consolidation (a confirmar). The template default. |
| openai | `gpt-5.4-mini` | a confirmar | Cheaper, faster alternative with decent quality. |
| openai-oauth | `gpt-5.5` | ChatGPT subscription | ChatGPT/Codex backend. See the login steps below. |
| copilot | `gpt-5.5` | GitHub Copilot subscription | GitHub Copilot Chat backend. See the login steps below. |
| gemini | `gemini-3.5-flash` | a confirmar (free tier) | Google-hosted, with native structured output. |
| openai-compat (Ollama) | `qwen3:32b` (example) | none (self-hosted) | Set `AI_MEMORY_LLM_BASE_URL=http://host.docker.internal:11434/v1`. Quality depends on the model. |

Provider-specific credentials:

- **openai-oauth:** run `docker exec -it ai-memory ai-memory auth login openai-oauth`
  on the homelab host. The token is saved to `auth.json` in the data directory,
  which is the mounted volume, so it survives container recreation.
- **copilot:** run `docker exec -it ai-memory ai-memory auth login copilot` on the
  homelab host, or set `COPILOT_GITHUB_TOKEN` in the env file. `GH_TOKEN` and
  `GITHUB_TOKEN` are also accepted.
- **gemini:** set `GEMINI_API_KEY` or `GOOGLE_API_KEY`.
- **openai-compat:** set `AI_MEMORY_LLM_BASE_URL` and `LLM_API_KEY`. The key is
  optional for keyless local servers.
- **Extra headers:** for a gateway that requires a caller-identifying header, set
  `AI_MEMORY_LLM_HEADERS` to comma-separated `Name=Value` entries. Every chat
  request already sends `User-Agent: ai-memory/<version>`. Headers the server sets
  itself are refused at startup.

What we do not recommend: reasoning-mode models (for example Kimi in reasoning
mode, Claude with extended thinking, OpenAI o-series, and Gemini "thinking"
variants). They spend the token budget on internal reasoning before emitting
output, and with the strict-JSON consolidation prompt they can hang or return
empty responses (a confirmar). If you must use one, turn reasoning off with
`AI_MEMORY_LLM_REASONING_EFFORT=none`; the effect depends on the provider
(a confirmar).

Structured output: hosted OpenAI-family providers request strict JSON-schema
output. The request schema is normalized to the subset that provider accepts:
closed objects, complete `required` lists, and no unsupported combinators.
`openai-compat` uses the same schema-constrained request by default. If the
endpoint explicitly rejects the structured-output option, or returns a response
that does not match the schema, the server retries once with a tolerant prose-JSON
parser. Transport errors, 5xx responses, auth failures, and rate limits do not
trigger that retry. The retry is a second HTTP call. For an incompatible endpoint,
set `AI_MEMORY_LLM_COMPAT_STRICT=false`. If you switch to a niche local model, run
`ai-memory llm-test` before you trust it.

Timeouts: every chat request is bounded at 300 seconds by default. Slow hosted
gateways (observed with free aggregator tiers) can stream a long completion past
that limit, and then every request fails with an error prefixed `http:`. Raise
`AI_MEMORY_LLM_TIMEOUT_SECS` in the container environment to match the gateway's
worst-case generation time. Zero is rejected at startup.

## Embeddings (optional)

Embeddings enable hybrid retrieval: full-text matches combined with vector
similarity. Without them, search uses full-text matching only. To enable them, set:

```text
AI_MEMORY_EMBEDDING_PROVIDER=openai          # or voyage
AI_MEMORY_EMBEDDING_MODEL=text-embedding-3-small
AI_MEMORY_EMBEDDING_DIM=1536
```

Key resolution:

- `openai`: `EMBEDDING_API_KEY`, then `OPENAI_API_KEY`.
- `voyage`: `VOYAGE_API_KEY`.
- `openai-compat`: `EMBEDDING_API_KEY`, then `LLM_API_KEY`. The dimension must be
  set explicitly, because self-hosted model dimensions vary.

Use `EMBEDDING_API_KEY` when embeddings should authenticate against a different
provider than the chat model.

A scheduled backfill can fill missing embedding rows in the background. It is off
by default (`AI_MEMORY_MAINTENANCE__EMBEDDING_BACKFILL_INTERVAL_SECS=0`). Set a
positive interval in seconds to enable it.

## Backups

The data directory is whatever you mounted in the compose file (by default
`/var/opt/docker/utils/ai-memory/data`). It contains:

```text
data/
├── wiki/    # markdown pages: back up with rsync, or push to a git remote
├── raw/     # session log archive
├── db/      # main database, including full-text indexes and embeddings
├── logs/    # daily rolling log files
└── models/  # directory for local embedding models
```

Copying the directory with any tool that can read it works for the wiki and the
logs. For a point-in-time consistent copy of the database, use the server's
backup command. Run it inside the container, then copy the result to the homelab
host:

```bash
ssh "$SERVER" "docker exec ai-memory ai-memory backup --to /data/snapshot-$(date +%F).tar.gz"
scp "$SERVER:$DEPLOY_DIR/data/snapshot-$(date +%F).tar.gz" ./backups/
```

The server takes the snapshot from the live database while it keeps serving
writes, then builds a gzip tarball containing the database snapshot, the wiki
directory, and the config file if one is present. The CLI streams that tarball to
the path you give. See [backup](backup.md) for the backup feature in general.

### 2.0 upgrade note

The first start of a 2.0 image runs a one-time format migration. Before it
changes anything, it archives the whole data directory into a backup folder, so
the store can be restored with a 1.x binary. Inside a container the default
destination is `backups/` under the data directory, which is on the volume. Set
`AI_MEMORY_BACKUP_DIR` to choose another destination. The archive is named
`ai-memory-backup-okf-v0.2-<timestamp>.tar.gz`. The migration runs only when the
wiki contains pre-2.0 files. If a usable archive already exists from an earlier
boot of the same upgrade, the migration reuses it and does not overwrite it.

The wiki home page shows where the archive is until you delete it. See
[OKF](okf.md) for the migration itself.

## Sharing one server between people or harnesses

A deployed server is the supported way to share a project, between teammates or
between several harnesses you run yourself. Two things matter before you hand out
the URL.

**One server per data directory.** At startup the server takes an exclusive lock
on its data directory. A second server pointed at the same directory refuses to
start, and the error names the process that holds the lock. Do not bypass this
with `--force`: two live servers on one data directory corrupt the wiki and index
state. Locks are not reliable on every network filesystem. Where the filesystem
cannot lock at all, the server logs a warning and continues without the guard, so
keep the data directory on local storage. Run one server and let everyone connect
to it. That is also what makes the shared-knowledge model work.

**Check the isolation mode.** Unscoped MCP calls resolve "current project" through
a pointer that is keyed per caller. The startup line says which mode is live:

```text
active-project isolation mode mode=PerActor ...
```

`PerActor` is the default and the one you want on a shared server. `Single` is one
process-wide slot. It is fine for one harness, but concurrent sessions then share
it, and unscoped writes resolve through it too. See [auto-scope](auto-scope.md) and
[users](users.md#running-for-a-team).

### How much load one server absorbs

Every write goes through one writer, which is the right design for a single-file
database. The writer's queue holds up to 1024 pending commands. When the queue is
full, producers wait for space, so a burst larger than the queue slows its senders
down and still lands every write. Nothing is dropped.

The project has a throughput measurement that runs one to 128 concurrent writers
and reports writes per second and mean latency. It is ignored by default and
must be run explicitly. The figures are a confirmar for this revision. The shape
of the result is: throughput rises steeply with concurrency and then flattens, and
per-write latency is dominated by disk sync rather than CPU (a confirmar).

For capacity planning, an actively working agent emits roughly one lifecycle write
per tool call. At a deliberately pessimistic one tool call per second per agent,
the writer ceiling (a confirmar) supports several hundred concurrently active
agents. That is far beyond a team. The writer is unlikely to be the first limit you
hit.

Two caveats. The measurement ran on a fast local disk. Because the cost is disk
sync, a network filesystem or a slow volume gives materially lower numbers, which
is another reason to keep the data directory local. And the measurement covers the
store, not the HTTP front end. An install that saturates the writer is more likely
to be limited by the agent side.

## Reclaiming disk space

The database does not return deleted space to the filesystem. Deleted rows leave
free pages that the database reuses as it grows. For a store in steady use this is
the right behavior, and it needs no attention.

`ai-memory status` reports the figure that decides whether to act:

```text
  storage:      2.4 GiB on disk, 610.0 MiB reclaimable (25.4%)
    `ai-memory compact --confirm` would return it (blocks writes while it runs)
```

The advisory line appears only when reclaimable space is at least 20% of the
database size and at least 64 MiB. When it appears:

```bash
ssh "$SERVER" "docker exec ai-memory ai-memory compact --confirm"
```

Compaction rebuilds the full-text indexes and rewrites the whole database to
return free pages. It deletes nothing. It holds an exclusive lock for its duration,
so every write blocks until it finishes. It needs free disk space on the same
filesystem, roughly the size of the database (confirmed in the reference code comments and in the refusal text of `compact` without `--confirm`; the size measured is page count × page size, not the file system size, so a write-ahead sidecar does not move it). The command prints `Compacted: <before> → <after> (<n> reclaimed).` (binary units B/KiB/MiB/GiB/TiB, one decimal from 1 KiB up) and, when nothing was reclaimed, `Nothing to reclaim — the database had no meaningful free-page backlog. This is the normal result for a store that has not just had a large deletion.` Without `--confirm` it fails and explains the lock and the free-space need. If there is nothing
meaningful to reclaim, it says so and changes nothing.

Some destructive commands offer `--compact` inline, which runs the same reclaim
right after the delete: `purge-project`, `purge-session`, and
`reclaim-ledger-versions`. `forget-sweep` has no inline option. After a large sweep,
check `status` and run `compact` if the figure justifies it.

### Do not schedule an unconditional nightly VACUUM

The obvious move is a nightly cron entry. Resist it:

- Compaction takes an exclusive lock and rewrites the whole database. Every write
  blocks for the duration, which on a large store can be minutes. This server's
  job is answering hook traffic, so blocked writes drop captures.
- It needs free disk space roughly the database's own size, so a nightly job also
  sets a permanent floor on free space.
- Because the database reuses free pages, a store in steady use usually has almost
  nothing to reclaim. A nightly run pays the full cost on most nights for no
  benefit.

The case that genuinely leaves a large free-page backlog is a one-off deletion: a
`purge-project`, a large retention sweep, or a `forget-sweep` over months of
episodic pages. Those are events, not a schedule. Use the inline `--compact` where
it exists, or run `compact` after the event.

If you do want it automated, make it conditional on the figure above and run it in
off hours:

```bash
#!/usr/bin/env bash
# Run on the homelab host, for example from a systemd timer or cron.
set -euo pipefail

RECLAIMABLE=$(docker exec ai-memory ai-memory status --json | jq '.storage.reclaimable_bytes')
THRESHOLD=$((1024 * 1024 * 1024))   # 1 GiB; tune to your store

if [ "$RECLAIMABLE" -ge "$THRESHOLD" ]; then
    docker exec ai-memory ai-memory compact --confirm
fi
```

Drive it from a weekly systemd timer (`OnCalendar=Sun 04:00`, `Persistent=true`) or
a host cron entry. A weekly check that usually does nothing costs one cheap status
call. A nightly rewrite that usually does nothing costs a write stall every night.

### What compaction does not do

Compaction is not erasure. The wiki's git history keeps page content in its objects
and commit messages, and any backup taken earlier still holds everything. Compaction
returns bytes from the live database file. That is worth doing on its own terms, but
it is not a guarantee that content is unrecoverable. See
[lifecycle operations](lifecycle-ops.md) for the full boundary.

## Rolling back

```bash
ssh "$SERVER" "cd $DEPLOY_DIR && \
               docker tag $IMAGE $IMAGE-rollback && \
               docker pull $IMAGE@sha256:<old-digest>"
ssh "$SERVER" "cd $DEPLOY_DIR && docker compose up -d"
```

The simplest rollback is to bring back an older image by digest. There is no
automated rollback command. Record the digest of the image you are running before
each deploy, so you can pull it again. Whether your registry keeps old digests
indefinitely depends on the registry (a confirmar for Docker Hub).

## Watching logs

```bash
ssh "$SERVER" "docker logs -f --tail 100 ai-memory"
```

Or browse the daily rolling log files on the host:

```bash
ssh "$SERVER" "ls -la $DEPLOY_DIR/data/logs/"
ssh "$SERVER" "tail -100 $DEPLOY_DIR/data/logs/ai-memory.log.$(date +%F)"
```

## Troubleshooting

- **`Connection refused`** on `curl http://<host>:49374/mcp`: the container is not
  up, or the port mapping is bound to `127.0.0.1` instead of `0.0.0.0`. Check
  `docker ps` on the homelab.
- **`403 forbidden host`**: the `Host` name in the request is not in
  `AI_MEMORY_ALLOWED_HOSTS`. Add the name or IP you use, then redeploy the env file.
- **`unhealthy`** status: the container is running, but its `status` health check
  is failing. The most likely cause is that the data directory's permissions do not
  match the container's user (UID 1000). Fix with
  `sudo chown -R 1000:1000 $DEPLOY_DIR/data` on the host. A slower-onset cause,
  days rather than minutes, is file descriptor exhaustion from dead peers (for
  example a laptop that went to sleep or a VPN drop) that accumulate without TCP
  keepalive. The server enables keepalive by default, with a 60-second interval.
  Tune it with `AI_MEMORY_TCP_KEEPALIVE_SECS`, or set it to `0` to disable it.
- **`ai-memory serve` refuses to start, naming a holder**: another server already
  holds the data directory's lock. Stop that server. Use `--force` only when you are
  certain the holder is gone (for example a hung process), and never to run two
  live servers.
- **Refusing unauthenticated plain HTTP** at startup: the bind address is
  non-loopback and no token is set. Set `AI_MEMORY_AUTH_TOKEN`, or bind to loopback.
- **Embedding mismatch after a model change**: at startup the server logs a warning
  when stored `(provider, model, dimension)` triples differ from the configuration.
  Hybrid search ignores stale rows until they are re-embedded. The server keeps
  running. Rebuild with `docker exec ai-memory ai-memory embed --force`. Without
  `--project`, `--force` covers every project in the workspace. Add
  `--project <name>` to scope the rebuild. `--dry-run` reports what would be
  embedded without writing.
- **Capture looks delayed, or observations are missing**: `ai-memory status` reports
  local hook-spool health: how many events are queued on the machine that runs the
  command, the age of the oldest one, and the total failed-delivery attempts:

  ```text
    spool:
      pending:    2
      oldest:     15m 0s
      retries:    4
  ```

  A non-empty spool with an aging oldest entry means hooks reach the local queue but
  not the server. Events are not lost; they drain once the server is reachable.
  `pending: 0` means capture is keeping up.

  The spool is client-side. The section reflects the machine you run the command
  on, not the server, even when the CLI points at a homelab server. The spool is also
  printed when the server cannot be reached at all, to stderr so that `--json` output
  stays a single object on stdout. `--json` carries the same numbers under a `spool`
  object (`pending`, `oldest_age_ms`, `retries_total`).

- **Provider failures**: `ai-memory status` reports LLM and embedding health from the
  last real provider call. A fresh process reports `unknown` until the server uses
  that role. Status does not probe providers and does not spend tokens.
- **Container restart loop**: check `docker logs ai-memory`. The `ai-memory starting`
  line near the top reports the resolved configuration. A missing required setting
  fails with a clear error. For example, `openai-compat` without
  `AI_MEMORY_LLM_MODEL` stops at startup and names that variable.
