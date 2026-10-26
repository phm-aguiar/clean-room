# Installation cookbook

This page covers installation scenarios beyond the basic container setup:

- [Server on a different machine](#server-on-a-different-machine)
  (homelab, LAN host, remote server)
- [Configuring the CLI URL and auth](#configuring-the-cli-url-and-auth)
- [Arch Linux native packages (AUR)](#arch-linux-native-packages-aur)
  (systemd system service or user service)
- [macOS menu bar app](#macos-menu-bar-app)
  (self-contained `.app` + LaunchAgent)
- [Configuring other agent CLIs](#configuring-other-agent-clis)
  (Codex, Command Code, Devin CLI, OpenCode, OMP, Pi, Cursor, Claude Desktop, Gemini CLI, Antigravity CLI, Grok Build CLI, Zero, ZCode, Kimi Code, Kiro CLI, Pool, OpenClaw, VS Code Copilot, Zed)
- [Installing hooks without docker](#installing-hooks-without-docker)
  (script-based installer)
- [Running ai-memory without docker](#running-ai-memory-without-docker)
  (mise, building from source)
- [Managed cross-harness workstreams](managed-workstreams.md)
  (`ai-memory run`, transparent native resume, and argument forwarding)
- [LLM provider tiers + self-hosted Ollama](#llm-provider-tiers)
- [Common subcommands](#common-subcommands)
- [Managed routing snippets and Agent Skills](#managed-routing-snippets-and-agent-skills)
- [Human password bootstrap and recovery](#human-password-bootstrap-and-recovery)
- [Operating without auth](#operating-without-auth) (local-only)
- [Keeping ai-memory up to date](#keeping-ai-memory-up-to-date)

> **Shorthand.** Most snippets use `$TOKEN` and `homelab:49374`. If
> you're following along verbatim:
> ```bash
> export TOKEN=$(docker run --rm <imagem-ai-memory>:latest generate-auth-token)
> ```
> and replace `homelab` with `localhost` if the server runs on the
> same machine as the agent CLI.

The container image is published for `linux/amd64` and `linux/arm64`. Apple
Silicon Macs and ARM64 Linux hosts do not need `--platform linux/amd64`.

> **Podman.** The `ai-memory` wrapper uses Docker when it is on the `PATH` and
> falls back to Podman when it is not. It also works through the `podman-docker`
> `docker` shim. Set `AI_MEMORY_DOCKER=podman` to force Podman when both engines
> are installed. The default image name is fully qualified so that Podman's
> non-interactive short-name resolution does not fail. See
> [SELinux-enforcing hosts](#selinux-enforcing-hosts) for how the wrapper detects
> the engine's rootless and SELinux state.

---

## Server on a different machine

When the ai-memory server runs on a LAN host (homelab, headless server)
and you use Claude Code, Codex, or another agent CLI on a laptop:

### Server side (the homelab host)

```bash
docker run -d --name ai-memory \
    --restart unless-stopped \
    -p 0.0.0.0:49374:49374 \
    -v ai-memory-data:/data \
    -e AI_MEMORY_AUTH_TOKEN="$TOKEN" \
    -e AI_MEMORY_ALLOWED_HOSTS="<server-ip>,localhost,127.0.0.1" \
    -e AI_MEMORY_LLM_PROVIDER=anthropic \
    -e ANTHROPIC_API_KEY=sk-ant-... \
    <imagem-ai-memory>:latest
```

Exposure rules that apply to this setup:

- **Bearer token.** Outside a container, a plain HTTP listener on a non-loopback
  address refuses to start unless `AI_MEMORY_AUTH_TOKEN` is set. The only other
  way to start it is the explicit `--allow-insecure-no-auth` flag on `serve`,
  which prints a warning and is meant for intentional LAN use.
- **Containers.** Inside a container the server cannot tell whether a published
  port is reachable from the network, because the host publish spec decides
  that. In this case it starts with a warning instead of refusing. Publishing
  with `-p 127.0.0.1:49374:49374` keeps the port local; publishing on `0.0.0.0`
  or a LAN address exposes it, so set the token.
- **Host allowlist.** `AI_MEMORY_ALLOWED_HOSTS` is a comma-separated list of the
  host names and addresses that clients may put in the `Host` header. The default
  is `localhost,127.0.0.1,::1`. Requests with any other `Host` value are rejected
  before they reach the MCP, hook, admin, or web routes. The list is required
  for remote clients that connect by LAN address or name. It defends against DNS
  rebinding. It is not an access control, because a client that can reach the
  port can set any `Host` value; the bearer token is the access control.

Bearer auth does not encrypt traffic. For LAN or remote access, put a
TLS-terminating proxy in front of the server. The ready Caddy and Cloudflare
Tunnel templates are in the [HTTPS reverse-proxy guide](https-via-proxy.md).
When the proxy serves `/web` over HTTPS, also set
`AI_MEMORY_AUTH__SECURE_COOKIE=true` in the server environment and close or
redirect direct HTTP access to that hostname. Do not set it for direct HTTP:
browsers then correctly withhold the session cookie.

Human (password) authentication has its own rule. When it is enabled, the
server refuses to bind to a non-loopback plain HTTP address unless
`[auth].secure_cookie` is `true`. Non-secure human cookies are allowed only on
loopback.

### Client side (the laptop)

```bash
export AI_MEMORY_SERVER_URL="http://<server-ip>:49374"
export AI_MEMORY_AUTH_TOKEN="$TOKEN"

ai-memory install-mcp   --client claude-code --apply
ai-memory install-hooks --agent  claude-code --apply
```

Both commands print what they would write unless `--apply` is passed. With
`--apply` they modify the agent's configuration file in place.

`--session-aware` is an optional Claude Code MCP mode:

```bash
ai-memory install-mcp --client claude-code --session-aware --apply
```

It replaces the static HTTP MCP entry with a local ai-memory stdio bridge. The
bridge still connects to the configured remote server and bearer token, and it
forwards Claude's lifecycle session id. Pair it with
`[auto_scope] mode = "per_session"` when the same operator runs concurrent
Claude Code sessions in different projects. The default static HTTP
registration remains appropriate for one active project at a time. This mode
is available only for Claude Code.

If `CLAUDE_CONFIG_DIR` is set to a non-blank value, the Claude Code installers
use that directory as Claude Code's config root:

- `install-mcp` writes the MCP registration to `$CLAUDE_CONFIG_DIR/.claude.json`
  instead of `~/.claude.json`.
- `install-hooks` and `setup-agent` write to `$CLAUDE_CONFIG_DIR/settings.json`
  instead of `~/.claude/settings.json`.
- `install-skills --scope global` uses `$CLAUDE_CONFIG_DIR/skills` instead of
  `~/.claude/skills`.
- `uninstall` sweeps the active relocated paths together with the home
  defaults. It cannot discover an older arbitrary `CLAUDE_CONFIG_DIR` that is no
  longer set.

An empty or whitespace-only value counts as unset. The container wrapper
forwards the variable for config roots under its existing `$HOME` bind mount;
use the native binary when the relocated root is outside `$HOME`.

The CLI commands (`bootstrap`, `status`, `search`, `lint`, `auto-improve`,
`curator`, `pending-writes`, and others) read the two environment variables
automatically. So do `install-mcp`, `install-hooks`, and `setup-agent`: with
`AI_MEMORY_SERVER_URL` set, `install-mcp` derives the `/mcp` endpoint, and
`install-hooks` uses the bare server origin.

After upgrading ai-memory, refresh the managed routing package in existing
projects so that Claude Code, OpenCode, Codex, and Gemini pick up the new tool
guidance and proactive retrieval rules. From an agent, ask it to refresh the
ai-memory routing in the project. From the terminal, run
`ai-memory install-instructions`, or pass `--target AGENTS.md` for non-Claude
prompt files. The update is idempotent. Legacy long snippets between
`<!-- ai-memory:start -->` and `<!-- ai-memory:end -->` markers are replaced in
place with the slim snippet, and managed Agent Skills are installed or updated
alongside it.

If you install into `AGENTS.md` and the project is also used from Claude Code,
make `CLAUDE.md` import it with a bare `@AGENTS.md` first line. Claude Code
loads `CLAUDE.md` and does not read `AGENTS.md`, so without that import the
installed block is absent from context at session start.

---

## Configuring the CLI URL and auth

The `ai-memory` binary is a thin HTTP client. It never opens the wiki or the
database directly. State-touching commands go through the running server, which
is the only writer.

Configuration is two optional environment variables:

| Variable | Default | When to set it |
|---|---|---|
| `AI_MEMORY_SERVER_URL` | `http://127.0.0.1:49374` | When the server runs somewhere other than the same machine, such as `http://192.168.0.90:49374`. |
| `AI_MEMORY_AUTH_TOKEN` | unset | When the server has bearer auth enabled. |

For a single-laptop loopback server, set neither variable. For a
remote or homelab server, put both in your shell rc or direnv file:

```bash
export AI_MEMORY_SERVER_URL="http://192.168.0.90:49374"
export AI_MEMORY_AUTH_TOKEN="<token>"
```

Explicit `--server-url` and `--auth-token` flags on `install-mcp`,
`install-hooks`, and `setup-agent` override the environment. Use them when you
generate configuration for a client that talks to a different server than your
default CLI target.

The server URL for hooks is resolved in this order:

1. The `--server-url` flag.
2. The configured server URL (`AI_MEMORY_SERVER_URL`), when it is set.
3. The server URL of an existing ai-memory MCP entry for the same agent, when
   one is found.
4. The loopback default, `http://127.0.0.1:49374`.

So if you run `install-mcp --apply` first and later run `install-hooks --apply`
without env vars or flags, the hooks reuse the server from the existing
ai-memory MCP entry. Remote MCP config and lifecycle capture then point at the
same server instead of falling back to loopback.

The hook URL keeps any path that the server URL already contains. If the
server URL has no path, the configured base path is added to its origin.

All installer `--apply` modes preserve symlinked configuration files. The
atomic update is written to the symlink target, including a target that does
not exist yet, while the timestamped backup stays next to the user-facing
config path. This keeps dotfile-managed installs linked. Symlink chains are
followed up to 40 levels; a deeper chain is refused.

Every `--apply` write is a no-op when the computed content equals the existing
content. Otherwise the previous file is copied to `<path>.bak-<unix-timestamp>`
before the new content is written atomically.

`init`, `serve`, and `generate-auth-token` do not need these env vars because
they either create local files or start the server itself.

### Default project resolution (`--project-strategy`)

By default each session files memory under the basename of the current working
directory. Because an agent shell keeps its working directory between tool
calls, a single `mkdir sub && cd sub` reparents the rest of the session into a
phantom project named `sub`. To make every session for an install resolve its
project from the git repository root instead, which collapses subdirectories
and worktrees, bake the strategy into the hooks:

```bash
ai-memory install-hooks --apply --agent claude-code --project-strategy repo-root
```

`--project-strategy` accepts `basename` (the default for new installs; it bakes
nothing) or `repo-root`. Omitting it during a later `--apply` preserves the
strategy already baked into that agent's ai-memory hooks, including during the
automatic post-upgrade refresh. Pass `basename` explicitly to remove an existing
`repo-root` default. This works for every agent and delivery path. A per-repo
`.ai-memory.toml` marker's own `project_strategy` and `project` still take
precedence. See
[the marker-file reference](marker-file.md#install-wide-default-no-marker).

---
## Arch Linux native packages (AUR)

Use the native packages when you want `/usr/bin/ai-memory` plus systemd units
instead of the container wrapper. The package installs the binary and the hook
sources once. Each user still stages their agent hook scripts into their own
home directory with `install-hooks --apply`.

### Package choice

```bash
yay -S ai-memory-bin    # prebuilt Linux x86_64/aarch64 binary, fastest install
yay -S ai-memory        # builds from source, works on x86_64 and aarch64
```

The two packages install the same files and conflict with each other. Remove one
before installing the other.

Both packages install the same runtime layout:

| Path | Purpose |
|---|---|
| `/usr/bin/ai-memory` | Native CLI/server binary. |
| `/usr/share/ai-memory/hooks/` | Packaged hook source bundle used by `install-hooks`. |
| `/usr/lib/systemd/system/ai-memory.service` | System-wide service unit. |
| `/usr/lib/systemd/user/ai-memory.service` | Per-user service unit. |
| `/usr/lib/sysusers.d/ai-memory.conf` | Creates the `ai-memory` system user. |
| `/usr/lib/tmpfiles.d/ai-memory.conf` | Creates `/var/lib/ai-memory` for the system service. |
| `/etc/ai-memory/config.toml` | System-service config file, tracked as a package backup file. |
| `/etc/ai-memory/env` | System-service environment and secrets file, tracked as a package backup file. |

The binary itself does not choose between system and user mode. Each unit file
passes its own data directory and config path explicitly:

| Mode | Data dir | Config | Env/secrets | Requires sudo? |
|---|---|---|---|---|
| User service | `~/.local/share/ai-memory` | `~/.config/ai-memory/config.toml` | `~/.config/ai-memory/env` | No |
| System service | `/var/lib/ai-memory` | `/etc/ai-memory/config.toml` | `/etc/ai-memory/env` | Yes |

Both units start the server with the HTTP transport and the web interface
enabled (`serve --transport http --enable-web`). The system unit runs as the
`ai-memory` user. Its filesystem is read-only except for the state directory,
and it has no access to home directories. The user unit runs under your own
account.

Do not run both services on the same bind address. They can coexist on disk, but
only one can listen on `127.0.0.1:49374` unless you change `bind` in one config.

### User-level service

Use this on a single-user workstation. It needs no sudo after package install and
keeps all state in your home directory.

```bash
mkdir -p ~/.config/ai-memory ~/.local/share/ai-memory
ai-memory \
  --data-dir ~/.local/share/ai-memory \
  --config ~/.config/ai-memory/config.toml \
  init
```

Edit provider and auth settings if you want LLM consolidation or bearer auth:

```bash
$EDITOR ~/.config/ai-memory/config.toml
$EDITOR ~/.config/ai-memory/env
```

For a loopback-only local service, bearer auth is optional. If you want one:

```bash
TOKEN=$(ai-memory generate-auth-token)
printf 'AI_MEMORY_AUTH_TOKEN=%s\n' "$TOKEN" >> ~/.config/ai-memory/env
```

Start and inspect the service:

```bash
systemctl --user daemon-reload
systemctl --user enable --now ai-memory.service
systemctl --user status ai-memory.service
journalctl --user -u ai-memory.service -f
```

If the service should keep running after you log out:

```bash
loginctl enable-linger "$USER"
```

Verify that the HTTP listener is up. Any HTTP response from the port shows that
the server is listening. The exact response body for a plain request to the MCP
endpoint is a confirmation point, not a documented contract.

```bash
curl http://127.0.0.1:49374/mcp
```

### System-level service

Use this for a shared workstation, LAN box, or homelab-style host where the
server should run independently of any logged-in user.

Make sure the package-created user and state directory exist, then initialize
the data layout as that service user:

```bash
sudo systemd-sysusers /usr/lib/sysusers.d/ai-memory.conf
sudo systemd-tmpfiles --create /usr/lib/tmpfiles.d/ai-memory.conf
sudo -u ai-memory ai-memory \
  --data-dir /var/lib/ai-memory \
  --config /etc/ai-memory/config.toml \
  init
```

The package creates the `ai-memory` system user with a no-login shell and
`/var/lib/ai-memory` as its home and state directory, mode `0750`.

Edit system config and secrets:

```bash
sudoedit /etc/ai-memory/config.toml
sudoedit /etc/ai-memory/env
```

The package installs `/etc/ai-memory/env` with mode `0640`: owner read and write,
group read, no access for other users. The file may hold API keys. Keep it out of
backups or logs that other users can read.

For LAN exposure, set a non-loopback bind and allowed hosts in
`/etc/ai-memory/config.toml`, and set a bearer token in `/etc/ai-memory/env`:

```toml
bind = "0.0.0.0:49374"
allowed_hosts = ["homelab", "192.168.0.90", "localhost", "127.0.0.1"]
```

```bash
TOKEN=$(ai-memory generate-auth-token)
printf 'AI_MEMORY_AUTH_TOKEN=%s\n' "$TOKEN" | sudo tee -a /etc/ai-memory/env
```

Start and inspect the service:

```bash
sudo systemctl daemon-reload
sudo systemctl enable --now ai-memory.service
sudo systemctl status ai-memory.service
journalctl -u ai-memory.service -f
```

Verify from the host:

```bash
curl -sI http://127.0.0.1:49374/handoff
# 401 Unauthorized when AI_MEMORY_AUTH_TOKEN is set.
```

---
### LLM provider login with native services

> **You do not need a paid platform API key.** ai-memory's LLM features
> (consolidation, lint, auto-improve) are opt-in. When you enable them you can
> authenticate with a subscription you already pay for, instead of a metered API
> key:
>
> - a Claude Pro or Max plan through `anthropic-oauth`, using a token from
>   `claude setup-token`;
> - a ChatGPT Plus, Pro, or Codex plan through `openai-oauth`
>   (`ai-memory auth login openai-oauth`);
> - a GitHub Copilot plan through `copilot` (`ai-memory auth login copilot`).
>
> See [LLM provider catalog](llm-providers.md) for the full table. You can also
> skip an LLM entirely. Without a provider the server still captures and searches
> with full-text search (a confirmar: rule-based summaries). [Local
> embeddings](local-embeddings.md) make vector search work without a key.

API-key providers go in the relevant env file:

```bash
# User service
printf 'AI_MEMORY_LLM_PROVIDER=anthropic\nANTHROPIC_API_KEY=sk-ant-...\n' >> ~/.config/ai-memory/env
systemctl --user restart ai-memory.service

# System service
sudoedit /etc/ai-memory/env
sudo systemctl restart ai-memory.service
```

OAuth-style providers write tokens into the selected data dir. Run the login
with the same `--data-dir` and `--config` pair as the service:

```bash
# User service
ai-memory \
  --data-dir ~/.local/share/ai-memory \
  --config ~/.config/ai-memory/config.toml \
  auth login openai-oauth

# System service
sudo -u ai-memory ai-memory \
  --data-dir /var/lib/ai-memory \
  --config /etc/ai-memory/config.toml \
  auth login openai-oauth
```

The `auth login` command accepts three providers: `openai-oauth`, `copilot`, and
`oidc-device`. Use `auth login copilot` the same way for GitHub Copilot. For
per-developer native hook auth against an OIDC issuer, run `auth login
oidc-device` in the developer's selected data dir instead:

```bash
ai-memory auth login oidc-device \
  --issuer "https://issuer.example.com/realms/team" \
  --client-id "ai-memory-cli"
```

The `--client-id` option is required for `oidc-device`. The `--issuer` option
takes an OIDC issuer URL; the endpoints are discovered from its
`/.well-known/openid-configuration` document.

The stored OIDC access token is also used by thin-client HTTP commands (`status`,
`search`, `read-page`, `write-page`, `backup`, `embed`, and similar) when no
static `AI_MEMORY_AUTH_TOKEN` or `[auth].bearer_token` is configured. Static
bearer auth takes precedence over the stored token. This is for external
OIDC-aware gateways and bridges. Native server auth still uses static root
bearer tokens or database-user tokens. The `/admin/*` routes remain root-only
unless a gateway translates accepted OIDC auth into upstream auth that
ai-memory accepts (a confirmar).

OIDC and Keycloak `sid` claims describe the login provider's session, not the
coding-agent session that ai-memory uses for `[auto_scope]` isolation. Gateways
may propagate the authenticated user, client, and agent headers. The
`X-Memory-Actor-Session-Id` header should carry only a real lifecycle-hook session
id from a session-aware bridge.

Restart the service after changing provider settings:

```bash
systemctl --user restart ai-memory.service      # user mode
sudo systemctl restart ai-memory.service        # system mode
```

### Wire agent CLIs after native install

For a local loopback server with no bearer token:

```bash
ai-memory install-mcp   --client claude-code --apply
ai-memory install-hooks --agent  claude-code --apply
```

For concurrent Claude Code sessions, set `[auto_scope] mode = "per_session"` in
the server config and add `--session-aware` to the `install-mcp` command. This
works for local and LAN servers. The generated stdio bridge keeps using
`AI_MEMORY_SERVER_URL` or `--server-url`, and `AI_MEMORY_AUTH_TOKEN`.

For a bearer-protected local or LAN server, export the endpoint first. The MCP
URL includes `/mcp`. The hook URL is the bare origin.

```bash
export AI_MEMORY_SERVER_URL="http://127.0.0.1:49374"
export AI_MEMORY_AUTH_TOKEN="$TOKEN"

ai-memory install-mcp   --client claude-code --apply
ai-memory install-hooks --agent  claude-code --apply
```

`install-hooks` looks for the packaged hook sources in the system share
directories (`/usr/local/share/ai-memory/hooks`, then `/usr/share/ai-memory/hooks`
when no hooks directory is given). It stages runnable copies under
`~/.local/share/ai-memory/hooks/<agent>/` so that the agent can execute files
owned by your user. Re-run `install-hooks --apply` after package upgrades to
refresh those staged copies.

### Hook latency expectations

Hook-capable agents launch one short-lived hook process per lifecycle event. A
successful tool call normally fires both a pre-tool and a post-tool event, so
the per-invocation overhead is paid twice. As an order-of-magnitude reference,
one independent evaluation of an earlier release on native macOS aarch64
reported about 145 ms per native hook invocation (about 290 ms per completed tool
call) and about 172 ms per legacy shell-script invocation (a confirmar: the
evaluation is not reproduced in the source).

These figures are one host's measurements, not a benchmark or a performance
guarantee. Process startup, filesystem and security software, the selected data
directory, authentication, and host load can all change the result. The native
hook fast path skips full config loading and diagnostic logging, and normally spools
locally instead of waiting for the server. Keep the installer's native default
where it is supported. Latency-sensitive deployments should measure their own
agent host before they opt into additional captured events or choose a script
fallback.

### Capture-policy capability and refresh

`[capture] ignore_paths` is enforced only by native `ai-memory hook` commands and
by the generated OpenCode, OMP, Pi, and OpenClaw integrations. Local installers
select native commands where they are supported. Legacy shell-script and PowerShell hooks,
remote-only installs, and Docker script bundles do not enforce it. Re-run
`install-hooks --agent <agent> --apply`, or refresh or reinstall the generated
plugins, after upgrading. The installer's capability output shows which
integration was selected. See the canonical
[capture exclusions reference](marker-file.md#capture-exclusions).

Lifecycle observation bodies are bounded separately from the 10 MiB HTTP request
limit. The limits are:

| Field | Limit |
|---|---|
| User prompts | 16 KiB (16,384 bytes) |
| Post-compaction summaries | 16 KiB (16,384 bytes) |
| Notification and tool excerpts | 2,000 bytes |
| Assistant final-turn excerpt (opt-in, see below) | 2,000 bytes |

Native `ai-memory hook` commands truncate these fields on UTF-8 boundaries
before they enter the local spool or the wire. The server applies the same
event-specific limits to every integration, including script and generated
clients. After sanitization it truncates observation bodies to 16 KiB as a
backstop (a confirmar: that this step runs before every write to the database
and the full-text index). Native hook commands invoke the installed binary
directly, so upgrading that binary is enough to receive the client-side limits.

**Disable Claude Code prompt capture.** Regulated or mixed-trust machines can
keep the rest of Claude Code's lifecycle capture enabled and still prevent
`UserPromptSubmit` text from entering the local spool or the wire:

```bash
ai-memory install-hooks --agent claude-code --no-capture-prompts --apply
```

The installer removes only ai-memory's prompt hook and preserves third-party
hooks registered under the same event. A later bare `install-hooks --apply`,
including an upgrade refresh, keeps the disabled state. Re-enable prompt capture
explicitly with `--capture-prompts`. These options are Claude Code only. Some
other agents use their prompt hook to inject handoff context, so removing it
would break continuity. Disabling prompt capture reduces session summaries and
recall quality, because user intent is no longer part of the observation stream.
Tool and session-boundary capture continues unchanged.

**Capture only repositories that opt in.** The controls above narrow *what* is
captured. This control narrows *where*. By default a repository with no
`.ai-memory.toml` marker is still captured, so a machine that works across many
checkouts captures every new one automatically. Forgetting a marker means
capturing more, not less. Allowlist mode reverses that default:

```bash
ai-memory install-hooks --apply --capture-mode allowlist
```

In allowlist mode a repository without a marker emits no lifecycle event at all.
The event is dropped in the hook process before it can reach the local spool or
the wire, so nothing is written to disk for a repository that never opted in.
To opt a repository in, place a `.ai-memory.toml` marker in it. That is the same
file that already configures routing and `ignore_paths`.

The allowlist gate runs only in native `ai-memory hook` commands. That is the
same boundary that applies to `[capture] ignore_paths`, for the same reason: the
gate runs inside the hook binary, immediately before it spools.

A normal `install-hooks --apply` on Linux, macOS, and Windows writes native
commands, so the usual install is covered. Script installs are not covered. The
bundled shell and PowerShell hooks post to the server directly and never run the
binary, so nothing reads the mode. That includes the legacy `posix` and `windows`
platform override (`AI_MEMORY_HOOK_PLATFORM`), the Docker host wrapper, and the
`setup-agent` snippets, which emit script commands by design. `install-hooks
--apply` prints the mode in force and warns when the install it is writing cannot
enforce it.

Within that boundary the mode is not per agent. It is stored once in the data
directory, and every native hook command reads it, whichever agent invoked it. A
later bare `install-hooks --apply`, including the refresh inside `ai-memory
upgrade`, leaves it unchanged by design. Every `--apply` prints the mode in
force. To return to the default, run `--capture-mode denylist`.

Verify the policy for any repository without changing anything:

```bash
printf '{"cwd":"%s"}' "$PWD" | ai-memory hook --event user-prompt-submit \
    --agent claude-code --server-url "$AI_MEMORY_SERVER_URL" --check-capture
```

`--check-capture` inspects the policy without spooling, draining, or contacting
the server. It needs a JSON payload that names the directory to test. In the
output, `admits_capture` is the repository-level result of the capture mode, and
`marker_present` shows whether the upward walk found a marker. For the final
decision, read `policy_admits_capture`: it also requires the event, routing, and
scope checks to pass. `"policy_admits_capture": false` means that the repository
captures nothing for that event.

The trade-off is that recall is lost for every repository you do not mark. A
repository you intended to capture stays silent until you add its marker.

Some agent harnesses attach the assistant's final turn to their `Stop` event.
Claude Code sends it as a raw `last_assistant_message`. By default that text is
never persisted. The native hook binary strips the raw field before it can reach
the local spool or the wire, and the server strips it again on arrival.

**Opt-in assistant capture.** You can opt in to storing a sanitized excerpt of
the assistant's final turn, capped at 2,000 bytes, as the `Stop` body. It is a
double opt-in: enable the server first, then the client.

1. **Server:** set `capture_assistant = true` in the server configuration, or set
   `AI_MEMORY_CAPTURE_ASSISTANT=true` in its environment. Then restart
   `ai-memory serve`.
2. **Client:** re-install the hooks with the flag. Claude Code, Codex, and
   OpenCode 2 are supported:

   ```bash
   ai-memory install-hooks --agent claude-code --capture-assistant --apply
   # Codex and OpenCode 2 are supported too. Codex's Stop payload carries
   # last_assistant_message:
   ai-memory install-hooks --agent codex --capture-assistant --apply
   ai-memory install-hooks --agent opencode2 --capture-assistant --apply
   ```

The client sanitizes the excerpt with its built-in patterns and truncates it
before it touches the spool or the wire. The server scrubs it again with its
`[sanitize]` patterns before storing it. If either side is off, or the marker is
malformed, the `Stop` body stays empty.

A re-run of `install-hooks --apply` without `--capture-assistant` keeps an
existing opt-in. There is no flag that turns the option off. This is by design:
the `ai-memory run` auto-wire passes no flag, and it must not downgrade a user
who enabled assistant capture. `--capture-assistant` is available only for
Claude Code, Codex, and OpenCode 2 on a native hook platform. On any other agent,
or on the script fallback, the installer refuses the flag rather than enabling
something that cannot take effect.

Assistant text is privacy-sensitive. Before you enable it, read the
[security notes](security.md) on what the text can contain and where it flows,
including the consolidation and reviewer prompts, and any cloud LLM provider you
have configured.

Upgrading the binary is enough for native Claude Code installs, and pending spooled
events drain with the raw field stripped as well. Installs that run the shell-script or
PowerShell fallback cannot sanitize assistant text. The Docker script bundle
and an explicit `AI_MEMORY_HOOK_PLATFORM=posix` both fall in this group. For these
installs, a `Stop` payload that still carries the raw field is dropped whole by
the script rather than posted verbatim. The Docker wrapper keeps script commands
on purpose, because a binary path inside its helper container is not valid on the
host. Running `install-hooks` through that wrapper refreshes the scripts but does
not convert them. To capture assistant text safely, install a native ai-memory
client on the agent host and run `install-hooks --agent claude-code --apply` with
that native executable. Even if the script fallback is kept, the server still
strips any raw field on receipt, before persistence.

#### Spooling and delivery

Native `ai-memory hook --event ...` commands spool events locally. The POSIX
shell bundle also spools, but only on failure. It posts first. It writes the
event to the same `<data_dir>/hook-spool/` location when the server is unreachable
or answers with a 5xx status. It flushes the backlog after the next successful
delivery. A 4xx status is a permanent rejection and is not retried, except for
408, 425, and 429, which are spooled for retry. The PowerShell bundle still drops
an undelivered event.

Session start runs a short, bounded cleanup drain before it fetches a handoff.
Boundary events that can be cut short (`stop`, `pre-compact`, and `session-end`)
start a detached `hook-drain` helper, so delivery does not depend on one shutdown
hook surviving. On Unix the helper starts in a new session through a trusted
`setsid` launcher when one is available, and falls back to a separate process
group otherwise. On Windows it uses detached process flags.

The POSIX bundle assigns one idempotency key before its first POST, and keeps
that key if the event enters the spool. A server that processed an event but
lost the response does not duplicate the observation or the effects of a
completed session end. If processing stopped after the observation was committed,
the retry re-runs the downstream work. `SessionEnd` commits its end watermark
together with its automatic handoff. A retry that finds that transaction complete
finishes any interrupted wiki commit, durable provider enqueue, and ingest-key
completion, without adding a second handoff. These incomplete effects are
at-least-once until the server marks the event complete (a confirmar: details of
the retry paths).

The spool is capped. Entries older than seven days are pruned, and the file count
is bounded. A permanently undrained backlog is therefore pruned rather than
growing without limit, but old undelivered events can be lost.

The built-in timings stay short on agent-facing paths. High-latency or
large-backlog installs can raise them with whole-minute runtime environment
variables set in the agent's environment. No `install-hooks` rerun is needed:

| Env var | Built-in default | Max override | What it caps |
|---|---:|---:|---|
| `AI_MEMORY_HOOK_DRAIN_TIMEOUT_MINUTES` | 3 seconds | 60 minutes | each event POST during a drain |
| `AI_MEMORY_HOOK_HANDOFF_TIMEOUT_MINUTES` | 3 seconds | 60 minutes | the synchronous `session-start` handoff GET |
| `AI_MEMORY_HOOK_START_BUDGET_MINUTES` | 3 seconds | 60 minutes | total time `session-start` may spend waiting for the drain lock and cleanup draining |
| `AI_MEMORY_HOOK_BACKGROUND_DRAIN_BUDGET_MINUTES` | 5 minutes | 60 minutes | total time the detached `hook-drain` helper may spend after a background-drain boundary |
| `AI_MEMORY_HOOK_INCREMENTAL_THRESHOLD` | 32 events | positive integer | spool backlog size that triggers a 250 ms `post-tool-use` catch-up drain |

Timing values must be positive whole numbers of minutes. Missing, empty,
non-numeric, or zero values fall back to the built-in defaults. Values above 60
are clamped to 60. The incremental threshold is a positive event count. Invalid
values fall back to 32.

The server-side hook ingest has an optional per-source rate limiter. It helps
shared or remote installs survive one runaway agent session. Set
`AI_MEMORY_HOOK_RATE_PER_SEC` on the server to the token refill rate for each
actor or session source. `0` or an unset value disables the limiter. Set
`AI_MEMORY_HOOK_RATE_BURST` to override the burst size. The burst defaults to the
refill rate, with a minimum of one token when the limiter is enabled. The limiter
is bounded in both the number of keys and the key size. Batch drains on the
`/hook/batch` route can skip over-budget sources while still accepting later,
unrelated sources.

---
### Native service operations

```bash
# User service
systemctl --user restart ai-memory.service
systemctl --user stop ai-memory.service
journalctl --user -u ai-memory.service -n 100

# System service
sudo systemctl restart ai-memory.service
sudo systemctl stop ai-memory.service
journalctl -u ai-memory.service -n 100
```

Backups use the same CLI. Point it at the service data directory:

```bash
# User service
ai-memory --data-dir ~/.local/share/ai-memory backup --to ~/ai-memory-backup.tar.gz

# System service
sudo -u ai-memory ai-memory --data-dir /var/lib/ai-memory backup --to /var/lib/ai-memory/backup.tar.gz
```

Removing the package does not delete data, because the package carries no removal
step for the state directories. Stop the service and remove state only when you
intentionally want to erase memory:

```bash
systemctl --user disable --now ai-memory.service
sudo systemctl disable --now ai-memory.service

# Optional destructive cleanup:
rm -rf ~/.local/share/ai-memory ~/.config/ai-memory
sudo rm -rf /var/lib/ai-memory /etc/ai-memory
```

### Maintainer integration test

The regular continuous-integration run executes a host-safe native packaging
check. It uses a temporary alternate root for the systemd analysis, sysusers, and
tmpfiles tools. It verifies unit syntax, expected paths, the sysusers output, the
tmpfiles rules, the env-file mode, and the shell syntax of the AUR build files,
without writing to the host's system directories or touching real services.

The repository also includes a manual Arch integration harness. It is kept out of
routine CI on purpose, because it creates a disposable distrobox, installs
packages, starts real systemd services, and can take several minutes:

```bash
<teste-arch-systemd>
```

The harness checks the AUR metadata shape, builds the current working tree,
installs the native layout into the disposable Arch container, starts the system
service with `systemctl`, and starts the user-profile command under transient
systemd supervision. It also checks that the packaged hook sources under
`/usr/share/ai-memory/hooks` can be staged by `install-hooks`.

The destructive part of the script refuses to run unless it detects a container
or distrobox environment.

Useful settings:

```bash
AI_MEMORY_NATIVE_TEST_BOX=ai-memory-native-test <teste-arch-systemd>
AI_MEMORY_NATIVE_TEST_KEEP_BOX=1 <teste-arch-systemd>
AI_MEMORY_NATIVE_TEST_IMAGE=quay.io/toolbx/arch-toolbox:latest <teste-arch-systemd>
```

The default box name is `ai-memory-native-systemd-test`, and the default base
image is `docker.io/library/archlinux:latest`. Setting `AI_MEMORY_NATIVE_TEST_KEEP_BOX=1`
keeps the box after the run.

---

## macOS menu bar app

On a Mac, the self-contained menu bar app is the graphical install. It bundles
the native `ai-memory` binary and the `hooks/` tree. It manages the existing
LaunchAgent (`<identificador-do-launch-agent>`), and it opens `/web`,
`ai-memory status`, `config.toml`, the data directory, and the logs. It does not
replace those tools with a second dashboard.

Building the app needs a build toolchain and Xcode, the same requirements as a
source build. The Xcode version must match what the macOS companion package
requires (a confirmar: exact version):

```bash
git clone <url-do-repositorio>
cd <diretorio-do-repositorio>
# From the source tree, change into the macOS companion directory
# (the one that holds the build script and the package manifest), then run:
./build.sh
open "dist/AI Memory.app"
```

Drag **AI Memory.app** to `/Applications`, then choose **Install & Start Server**
from the menu extra. The app has no Dock icon. When the status item is green,
wire an agent with the bundled binary, so that `install-hooks` finds the sibling
`hooks/` tree:

```bash
BIN="/Applications/AI Memory.app/Contents/Resources/runtime/ai-memory"
"$BIN" install-mcp --client claude-code --apply
"$BIN" install-hooks --agent claude-code --apply
```

Durable memory stays in `~/Library/Application Support/ai-memory`. Replacing the
`.app` is an update and does not rewrite that tree (a confirmar). Prebuilt
tarball, source-build, container-wrapper, and hand-installed launchd paths are
covered in [external companions and tests](external-companions-and-tests.md),
section 5.

---
## Configuring other agent CLIs

> `install-mcp --server-url` accepts either the bare server origin or the full
> MCP endpoint. It appends a missing `/mcp` exactly once.
> `install-hooks --server-url` takes the bare server **origin**
> (for example `http://homelab:49374`). The hook scripts append `/hook`,
> `/handoff`, and the other paths themselves.

Each agent CLI needs two things:

1. **MCP registration**, so that the agent can call `memory_query`,
   `memory_recent`, and `memory_handoff_accept`.
2. **Lifecycle hooks**, so that the server captures session events
   automatically. Without them the agent can still query memory, but capture
   becomes manual.

Claude Desktop, VS Code Copilot, and Zed are MCP-only today. The hook-capable
clients listed in the [support matrix](support-matrix.md), including Pi and Zero,
have lifecycle capture paths through `install-hooks`.

> **Hook install pattern.** Local supported profiles default to host-native
> commands. Claude Code may use its supported Windows exec form, where `command`
> is the real `ai-memory.exe` and `args` are the argument tokens for
> `hook --event ...` (a confirmar). Other agents use native single command strings
> that follow their own hook schema. The PowerShell and Git Bash script bundles
> are compatibility fallbacks, and they do not enforce capture-policy v1.
> Remote-only and Docker script installs still use the two-step path: first copy
> the bundled scripts to your home directory, then run `install-hooks` in the
> container to render the configuration snippet.

### How `install-hooks --apply` merges into an agent's hook file

For the JSON-settings agents (Claude Code, Codex, Gemini CLI, Cursor, Devin, Command Code, Droid, Muse Code, CodeBuddy Code, IBM Bob, Copilot CLI, Grok, Antigravity CLI and similar) `--apply` edits the file, never replaces it:

- **Per event, ai-memory's own entries are replaced and every other tool's entries are kept.** The existing array for the event is filtered to drop ai-memory entries, then the freshly rendered entries are appended. Re-running never duplicates.
- **What counts as an ai-memory entry** (so a third-party hook is never removed): a command handler that (a) has no `args` and whose command string is one of ai-memory's own forms (the native form `<…ai-memory…> hook --event … --agent … --server-url …`, or the legacy script form carrying the exact `AI_MEMORY_HOOK_URL=` marker), or (b) is in exec form (`command` plus `args`) with an executable whose name contains `ai-memory`/`ai_memory` **and** the argument tokens `hook`, `--event`, `--agent` and `--server-url`. A helper such as `ai-memory-helper.exe --check project`, or a third-party tool that happens to run `ai-memory boot …`, is left alone. An entry may be flat or nested (inside an inner `hooks` array); either shape is recognized. Kiro CLI v2 uses its own ownership test on the command string.
- Other keys of the file (theme, schema version, other tools' settings) are preserved; a modified file keeps a timestamped backup next to it (`<file>.bak-<seconds>`); a no-op reports `already up to date`.
- Printed output: `✓ created|updated|no-op <path> (new file | backup written next to it | already up to date)`, followed by the agent's notes (below).

**Command forms.** On a native install each hook entry runs the binary itself: `<absolute ai-memory path> [--data-dir <dir>] hook --event <event> --agent <agent> --server-url <url> [--auth-token <t>] [--project-strategy <s>] [--capture-assistant]`. The executable path is the running binary's absolute path (a bare `ai-memory` when the binary sits in a Nix store, no longer exists, or the install is in a container; a `(deleted)` suffix left by an in-place upgrade is stripped). `--auth-token` appears in a **printed** snippet; `--apply` keeps the token out of the file and stores it under the data directory instead. Quoting is POSIX single quotes on Unix-like systems, double quotes on Windows; on Windows Codex and Grok (whose runners evaluate the string with PowerShell) get a leading `& ` call operator and Antigravity CLI gets the bare unquoted form (its runner wraps the string in `cmd /c "…"` and does not strip inner quotes); Claude Code on Windows uses exec form (`command` + `args`). The legacy compatibility scripts use `AI_MEMORY_HOOK_URL=<url> [AI_MEMORY_AUTH_TOKEN=<t>] [AI_MEMORY_PROJECT_STRATEGY=<s>] <script>` (POSIX), a `powershell.exe -NoLogo -NoProfile -NonInteractive -OutputFormat Text -ExecutionPolicy Bypass -EncodedCommand <UTF-16LE base64>` command that sets `$ProgressPreference='SilentlyContinue'` first (Windows), or `bash -c '<posix form with the Git-Bash path>'` (Windows with Bash).

**Entry shapes by agent.**

| Shape | Agents |
|---|---|
| Nested `{"matcher": "", "hooks": [{"type": "command", "command": …}]}` | Claude Code, Codex, Gemini CLI, Devin, Droid, Muse Code, CodeBuddy Code |
| Nested **without** `matcher` (omission means "all tools"; Bob rejects a matcher on lifecycle events) | IBM Bob, Command Code |
| Flat `{"type": "command", "command": …, "matcher": ""}` plus top-level `"version": 1` | Cursor |
| Flat without `matcher` plus top-level `"version": 1` (a matcher is rejected on `sessionStart`/`sessionEnd`/`userPromptSubmitted`/`agentStop`/`subagentStop`/`errorOccurred`) | Copilot CLI |

Muse Code additionally writes `"timeout": 30` (seconds) on every handler and the file's `"schema_version": 1` when absent.

### Factory Droid CLI, Muse Code, CodeBuddy Code and IBM Bob

All four use native commands only (no script bundle, whatever `AI_MEMORY_HOOK_PLATFORM` says), deliver the handoff on `SessionStart`, and print a note after `--apply`:

| Agent | Hook file and key | `--apply` notes | Session end |
|---|---|---|---|
| Droid (`--agent droid`) | `~/.factory/hooks.json` (the event map itself, no wrapper key) | `Droid adds SessionStart stdout to the session as context, so the prior session's handoff is delivered automatically, and its own SessionEnd closes the session.` / `start a new droid session for it to load the new hooks.` **`--apply` refuses to create `hooks.json` over hooks that still live in the `hooks` key of `~/.factory/settings.json`** (Droid only reads those while `hooks.json` is absent; the settings file is parsed as JSONC, and one that cannot be parsed is refused). The printed form tells the operator to add the entries to `settings.json` instead in that case. | `SessionEnd` |
| Muse Code (`--agent muse`) | `hooks` key of `${XDG_CONFIG_HOME:-~/.config}/muse/settings.json` (an absolute `XDG_CONFIG_HOME` only) | `Muse adds SessionStart's additionalContext to the session …` / `Muse runs hooks with a cleared environment, so the server URL and data dir are baked into each command; re-run this command after changing either. Start a new muse session to load the new hooks.` The printed snippet carries `"schema_version": 1` with the hooks. | `SessionEnd` |
| CodeBuddy Code (`--agent codebuddy`) | `hooks` key of `~/.codebuddy/settings.json` | `CodeBuddy appends SessionStart stdout (`hookSpecificOutput.additionalContext`) to the session, so the prior session's handoff is delivered automatically.` / `start a new codebuddy session for it to load the new hooks.` | `SessionEnd` |
| IBM Bob (`--agent ibm-bob`) | `hooks` key of `~/.bob/settings/settings.json` (shared by the IDE and Bob Shell) | `IBM Bob injects plain SessionStart stdout into the model context, so the prior session's handoff is delivered automatically.` / `Bob exposes Stop but no SessionEnd; close a finished session with `ai-memory finalize-session --agent ibm-bob`.` / `start a new Bob session for it to load the new hooks.` | none |

Event lists are in [support-matrix.md](support-matrix.md). Working-directory resolution for the hook: the payload's own directory first; for CodeBuddy Code (whose interactive `SessionStart` can arrive without one) the `CODEBUDDY_PROJECT_DIR` environment variable of the hook process, **never** the process's current directory (its meaning is undocumented); for Devin, a chain of fallbacks that ends at the process's current directory; every other agent has no fallback. Session id: Devin and ZCode payloads may lack one, so the hook keeps `<data>/hook-state/<agent>-session-id` (a new id on `session-start`, the stored one afterwards, one fresh id if none is stored) so one agent session shares one id; `finalize-session` clears it.

### GitHub Copilot CLI

`ai-memory install-hooks --agent copilot-cli --apply` writes `$COPILOT_HOME/hooks/ai-memory.json` (default `~/.copilot/hooks/ai-memory.json`), a file owned by ai-memory, with PascalCase event names so Copilot sends the Claude-compatible payload shape (`session_id`, `cwd`, `tool_name`, `tool_input`, `tool_result`); `PostToolUseFailure` is aliased to the post-tool event. The handoff goes through a **top-level** `additionalContext` key (not Claude Code's `hookSpecificOutput` envelope). There is no bare `copilot` alias for `--agent`, because `copilot` already means the VS Code client in `install-mcp`.

### OpenAI Codex

```bash
# MCP snippet (merge into ~/.codex/config.toml):
docker run --rm <imagem-ai-memory>:latest \
    install-mcp --client codex \
    --server-url "http://homelab:49374/mcp" \
    --auth-token "$TOKEN"

# Hooks: extract scripts, then render config:
docker cp ai-memory:/usr/local/share/ai-memory/hooks ~/.ai-memory/
docker run --rm <imagem-ai-memory>:latest \
    install-hooks --agent codex \
        --hooks-dir ~/.ai-memory/hooks \
        --server-url "http://homelab:49374" \
        --auth-token "$TOKEN"
```

The Codex hooks file is `$CODEX_HOME/hooks.json` when `CODEX_HOME` is set, and
`~/.codex/hooks.json` otherwise.

Native Codex tool hooks use the top-level `tool_name`, `tool_input`,
`tool_response`, and `tool_use_id` fields (verified against CLI 0.154.0, a
confirmar). ai-memory records the tool family and the call ID on `PreToolUse` and
`PostToolUse`. Recognized tools, such as `Bash` and `apply_patch`, also keep a
sanitized response excerpt on `PostToolUse`, capped at 2,000 bytes including
metadata. Structured JSON responses are flattened through the same bounded excerpt
path. Inputs are not copied into observations. Unknown tools, including
unrecognized MCP names, keep only metadata. `PostToolUse` alone does not prove
success, so Codex outcomes stay `unknown`.

Capture exclusions still run before native spooling. Codex's `apply_patch` passes
the patch text in `tool_input.command`. That does not give the capture policy any
direct file paths. When `ignore_paths` is active, those events keep only metadata.
ai-memory does not parse patch or shell text to infer paths. Tool events are
delivered at the normal 32-event catch-up threshold or at a lifecycle drain
boundary, so a small active turn may still have queued events.

Codex CLI 0.145.0 and later expose a native `SessionEnd` hook (a confirmar: the
version boundary). `Stop` is captured as a turn boundary and leaves the session
open. A native `SessionEnd` triggers the final summary, the handoff, and the
auto-improvement eligibility. For older clients, or when a session-end delivery
was missed, finalize the session manually:

```bash
ai-memory finalize-session
# add --all to close every matching open Codex session in this workspace/project
# or target one exact concurrent session (mutually exclusive with --all)
ai-memory finalize-session --session-id <uuid>
```

Antigravity CLI also has no true session-end event. Its `Stop` hook marks the end
of one execution loop, so ai-memory records it without closing the conversation.
Its `PreInvocation` hook runs before every model call. ai-memory treats only the
documented call with `invocationNum = 0` as a session start. Later invocations
return an empty hook result, without capturing another start and without fetching
the single-use handoff. A handoff created while the current conversation winds
down therefore stays available to the next session. After the final turn,
finalize the latest matching Antigravity session explicitly:

```bash
ai-memory finalize-session --agent antigravity-cli
# add --all only to close every matching open Antigravity session in this scope
# or add --session-id <uuid> to close one exact concurrent session
```

The `finalize-session` command accepts `--agent` (default `codex`), `--workspace`,
`--project`, `--all` or `--session-id` (the two are mutually exclusive), and
`--all-owners`. By default it only considers your own sessions and sessions with
no owner, so it cannot end a colleague's live session. `--all-owners` includes
other operators' sessions. `--reopen` works together with `--session-id` and
re-finalizes a session that has already ended. Use it when the conversation
continued and captured new observations afterward.

---
### Devin CLI

Devin keeps MCP servers in `~/.devin/config.json`. By default its lifecycle hooks
go to `~/.devin/hooks.v1.json`. If you prefer one combined Devin configuration
file, pass `--config-file ~/.devin/config.json` to `install-hooks`. ai-memory then
merges the hook entries under the `hooks` key of that file.

```bash
ai-memory install-mcp --client devin --apply \
    --server-url "http://homelab:49374/mcp" \
    --auth-token "$TOKEN"

ai-memory install-hooks --agent devin --apply \
    --server-url "http://homelab:49374" \
    --auth-token "$TOKEN"

ai-memory install-skills --agent devin
```

Devin's hook vocabulary is close to Claude Code's, with two differences:

- Devin emits `PostCompaction` after compaction and includes a `summary` field.
  ai-memory records it as `post-compaction`.
- Devin has no subagent start or stop hooks, so ai-memory cannot capture nested
  subagent boundaries for Devin.

The `SessionStart` hook injects pending handoffs through the
`hookSpecificOutput.additionalContext` field. Real Devin `SessionStart` and
`PostToolUse` payloads may omit `session_id` and `cwd`. When the payload omits
`cwd`, ai-memory infers it from the `DEVIN_PROJECT_DIR` variable or from the hook
process's working directory. When necessary it mints or reuses a per-host session
id from the hook state, so those events are still captured. A value that the
payload provides always wins.

### Kimi Code

Kimi Code keeps MCP servers in `~/.kimi-code/mcp.json` and lifecycle hooks in
`~/.kimi-code/config.toml`. Both move together when `$KIMI_CODE_HOME` is set. The
CLI also accepts `--agent kimi` as an alias. `install-mcp` writes the server URL
with a `?flavor=moonshot` query, because the Moonshot API rejects root-level
`anyOf`, `oneOf`, and `allOf` in tool parameter schemas. The ai-memory server
answers flavored requests with flat schemas. All other clients keep the upstream
shape.

```bash
ai-memory install-mcp --client kimi-code --apply \
    --server-url "http://homelab:49374/mcp" \
    --auth-token "$TOKEN"

ai-memory install-hooks --agent kimi-code --apply \
    --server-url "http://homelab:49374" \
    --auth-token "$TOKEN"
```

`install-hooks` merges `[[hooks]]` entries into `config.toml` and keeps the
provider and model settings that the same file holds. The entries cover ten
events: `SessionStart`, `SessionEnd`, `UserPromptSubmit`, `PreToolUse`,
`PostToolUse`, `PostToolUseFailure`, `Stop`, `SubagentStart`, `SubagentStop`, and
`PreCompact`. Kimi Code reports tool failures through `PostToolUseFailure`, which
reuses the post-tool-use handler. Local installs default to native
`ai-memory hook --event ... --agent kimi-code` commands. These use the local spool
with batched delivery, and they enforce capture-policy v1. The staged script
bundle under `~/.local/share/ai-memory/hooks/kimi-code/` is the compatibility
fallback. It posts to `/hook` and spools a failed delivery for a later drain. It
does not enforce capture-policy v1.

A pending handoff is injected at `UserPromptSubmit`, through the hook's standard
output. Kimi Code appends that output to the model context as a user message
before the turn. Kimi Code fires `SessionStart` but discards that hook's standard
output. Hooks installed by an older release therefore consumed handoffs without
delivering them. Existing native hook commands invoke the current `ai-memory`
binary, so they pick up the corrected delivery behavior on upgrade. Re-run
`ai-memory install-hooks --agent kimi-code --apply` only for a script-fallback
installation, so that its staged scripts are refreshed.

Kimi Code hook entries accept only the `event`, `matcher`, `command`, and
`timeout` fields. Any extra field makes the whole `config.toml` fail to load, so
prefer `install-hooks --apply` over hand edits.

### Command Code

Command Code keeps user-scope MCP configuration and hook configuration in separate
JSON files under `~/.commandcode/`. Install both integrations with:

```bash
ai-memory install-mcp --client command-code --apply \
    --server-url "http://homelab:49374/mcp" \
    --auth-token "$TOKEN"

ai-memory install-hooks --agent command-code --apply \
    --server-url "http://homelab:49374" \
    --auth-token "$TOKEN"
```

The aliases `commandcode`, `cmdc`, and `cmd` are accepted. `install-mcp` merges a
native HTTP entry into `~/.commandcode/mcp.json`. `install-hooks` merges only
Command Code's four stable events (`SessionStart`, `PreToolUse`, `PostToolUse`,
and `Stop`) into `~/.commandcode/settings.json`. It keeps the other settings and
the other hook handlers. The hook definitions deliberately omit `matcher`. Command
Code documents an omitted matcher as "all tools", and a matcher on `SessionStart`
or `Stop` prevents that lifecycle hook from firing.

Local installs use the native `ai-memory hook` command, so Command Code's native
`session_id` and `cwd` are attributed directly. The recognized `shell_command`,
`read_file`, `write_file`, and `edit_file` payloads pass through the same bounded
capture-exclusion policy as the other native integrations. A pending handoff is
injected through `hookSpecificOutput.additionalContext` at `SessionStart`.

Command Code's stable `Stop` event ends a turn, not a session. When you need
immediate consolidation and a handoff, finalize the open session after the last
turn:

```bash
ai-memory finalize-session --agent command-code
ai-memory finalize-session --agent command-code --session-id <uuid>
```

ai-memory does not install Command Code mods. Mods run arbitrary unsandboxed code,
and the stable hook and managed-session paths do not need them.

Managed sessions are opt-in:

```bash
ai-memory run command-code
ai-memory run command-code --yolo --model <model-id>
```

The aliases `commandcode`, `cmdc`, and `cmd` select the same adapter. The default
executable is `command-code` on Unix-like systems and `cmdc` on native Windows.
Fresh sessions keep Command Code's native UUID. Returning sessions use the exact
`--session <uuid>` option. The read-only transcript adapter accepts only the
observed version 3 header. It requires the UUID in the file name and the canonical
`cwd` to match the checkout. It excludes checkpoint and prompt sidecars, hidden
reasoning, images, and provider metadata. It keeps `parentId` on visible events
for branch provenance. An unknown future transcript version fails closed until its
schema is audited. Direct `cmd`, `cmdc`, or `command-code` launches are unchanged.
### Kiro CLI

Kiro CLI has one MCP surface and two incompatible lifecycle-hook formats.
ai-memory supports both through explicit installer targets. `kiro-cli` remains
the v2 target, and `kiro-cli-v3` selects the standalone v3 registration. The
global MCP file is `$KIRO_HOME/settings/mcp.json`, which defaults to
`~/.kiro/settings/mcp.json`. For a project-scoped entry, pass
`--config-file .kiro/settings/mcp.json`.

```bash
ai-memory install-mcp --client kiro-cli --apply \
    --server-url "https://memory.example/mcp" \
    --auth-token "$TOKEN"
```

The `kiro` alias is equivalent. The installed URL includes `?flavor=bedrock`, so
Kiro's Bedrock backend receives schemas without root-level `anyOf`, `oneOf`, or
`allOf`. Nested schemas and runtime validation stay intact. Kiro accepts HTTPS
for remote MCP servers and plain HTTP only on loopback. The CLI therefore rejects a
plain-HTTP remote URL before it changes the file. Put a reverse proxy in front of
the server, as described in [HTTPS via reverse proxy](https-via-proxy.md).

Install the v2 hooks with the `kiro-cli` agent value. When `--server-url` is
omitted, `install-hooks` can infer the hook origin and the bearer token from the
managed MCP entry above.

```bash
# Default v2 engine: merge hooks into every existing global agent config.
ai-memory install-hooks --agent kiro-cli --apply

# A project-local v2 agent overrides a same-named global agent. Update the
# selected local config explicitly instead of assuming the global copy runs.
ai-memory install-hooks --agent kiro-cli --apply \
    --config-file .kiro/agents/<agent-name>.json
```

The v2 engine stores camelCase hooks inside agent JSON files. ai-memory updates
only the existing `$KIRO_HOME/agents/*.json` files. It does not create an agent
that Kiro never selects. If that directory is empty, create and select an agent
first. Kiro gives project-local agents precedence over global agents, so use
`--config-file` when the active definition lives under `.kiro/agents/`. All target
files are parsed before any of them is changed. Unrelated agent fields,
third-party hooks, and the existing `--project-strategy` of each agent stay
intact.

The v2 install registers spawn, user-prompt, pre-tool, post-tool, and stop
capture. It stays fail-open when ai-memory is unavailable, and it delivers a
pending handoff through the standard output of a successful `agentSpawn` hook.
Verified v2 tool payloads enforce `[capture] ignore_paths`. An unrecognized
payload shape is stored as bounded metadata rather than as file content.

Install the v3 hooks with the explicit `kiro-cli-v3` target. The distinction is
deliberate. `kiro` and `kiro-cli` keep meaning v2, so an upgrade cannot silently
rewrite an existing installation into an incompatible format. The standalone v3
registration was acceptance-tested with an interactive Kiro CLI 2.16.2 session
started with `--v3` (a confirmar: the exact version).

```bash
# Global v3 registration under $KIRO_HOME/hooks (default ~/.kiro/hooks).
ai-memory install-hooks --agent kiro-cli-v3 --apply

# Project-local v3 registration.
ai-memory install-hooks --agent kiro-cli-v3 --apply \
    --config-file .kiro/hooks/ai-memory.json
```

The v3 installer writes the standalone schema with `version: "v1"` and
PascalCase triggers. It preserves third-party entries in a shared file. It refuses
an unsupported schema version, and it refuses a third-party entry that uses an
ai-memory-reserved hook name. Capture-only commands are bounded to one second.
`SessionStart` gets five seconds, so the bounded handoff fetch can finish. Both
engines use the same sanitized hook-ingress boundary. Documented and live
`tool_name` and `tool_input` file operations honor `[capture] ignore_paths`.
Unknown file-tool payload shapes degrade to metadata-only capture.

Kiro's v2 `stop` event ends a turn, not the session. After the final turn, close
the matching session explicitly. Use the exact session id when several Kiro
sessions are open in the same project:

```bash
ai-memory finalize-session --agent kiro-cli
ai-memory finalize-session --agent kiro-cli --session-id <uuid>
```

`ai-memory uninstall --only hooks --apply --yes` removes only the exact ai-memory
entries from the global v2 agents, from the current project's `.kiro/agents`
directory, and from the global and current-project v3 registrations. A file that
ai-memory generated entirely is deleted. Third-party entries in a shared file
remain.

`ai-memory run kiro` (alias `kiro-cli`) manages the default v2 engine and honors
`$KIRO_HOME`. To resume with the v3 engine, pass `--v3`, which is forwarded to
Kiro as a native argument (a confirmar: other options that select the engine).
Once a workstream is linked, a later plain Kiro launch recovers the stored engine
automatically. A bare `ai-memory run` considers checkout-local sessions from both
incompatible stores. See [managed workstreams](managed-workstreams.md#native-adapter-behavior).

### Pool (Poolside Agent CLI)

Pool reads lifecycle hooks from a project-scoped `.poolside/settings.yaml` at the
root of each repository it runs in. There is no user-global hook file for
ai-memory to merge into. `install-hooks --agent pool` (alias `poolside`) therefore
stages the hook scripts in a stable user-global location and prints a
ready-to-paste `hooks:` snippet. ai-memory deliberately does not write files
inside your repositories.

```bash
# Stage the scripts and print the snippet to paste into
# <repo>/.poolside/settings.yaml:
ai-memory install-hooks --agent pool --apply \
    --server-url "http://homelab:49374" \
    --auth-token "$TOKEN"
```

The snippet wires Pool's five events: `SessionStart`, `UserPromptSubmit`,
`PreToolUse`, `PostToolUse`, and `Stop`. They use Claude-style names and a
snake_case JSON payload on standard input (verified against Poolside CLI v1.0.16,
a confirmar). Local installs use the native `ai-memory hook` command. Pool's
documented `tool_name` and `tool_input` file operations honor `[capture]
ignore_paths`, and unknown file-tool payload shapes degrade to metadata-only
capture.

Pool has no true session-end event. `Stop` marks a turn boundary. After the final
turn, close the session explicitly. Use the exact session id when several Pool
sessions are open in the same project:

```bash
ai-memory finalize-session --agent pool
ai-memory finalize-session --agent pool --session-id <uuid>
```

Pool tolerates hook standard output, but injecting model-visible context from
`SessionStart` output has not been demonstrated. The session-start hook therefore
only captures, and it never fetches the single-use handoff. Recover a previous
session's handoff with the MCP `memory_handoff_accept` tool. The install does not
include a first-party `install-mcp` client, and it does not provide a managed
workstream (`ai-memory run pool`). Pool's native session-store contract has not
been demonstrated. See [managed-harness contributions](managed-harness-contributions.md).
### ZCode (z.ai)

ZCode wires lifecycle hooks in the root `hooks` block of `~/.zcode/cli/config.json`,
the same file that holds its other CLI registration. `install-hooks --agent zcode`
(alias `zai`) merges the ai-memory entries into that block, around any third-party
hooks you already have. It is idempotent. A re-run removes only ai-memory's own
entries, which are marked with `statusMessage: "ai-memory capture"`, and writes
them again in place.

```bash
ai-memory install-hooks --agent zcode --apply \
    --server-url "http://homelab:49374" \
    --auth-token "$TOKEN"

# Preview the exact hooks block without writing anything:
ai-memory install-hooks --agent zcode \
    --server-url "http://homelab:49374"
```

Entries use the exec form `{"type": "process", "command": <ai-memory>, "args":
[...]}`. ZCode spawns them with the event JSON on standard input and no shell. The
native `ai-memory hook` command reads that input directly, so the local spool,
bearer auth, and `[capture] ignore_paths` exclusions all apply. Every emitted key
comes from ZCode's documented hook schema (`type`, `command`, `args`, `enabled`,
`timeoutMs`, `statusMessage`). ZCode drops entries that carry undocumented keys,
so none are emitted. Six documented triggers are wired: `SessionStart`,
`UserPromptSubmit`, `PreToolUse`, `PostToolUse`, `PostToolUseFailure`, and `Stop`
(verified live against an embedded engine version, a confirmar).
`PostToolUseFailure` fires instead of `PostToolUse` when a tool throws. It is
forwarded to the same capture channel, with the error kept as the observation
outcome. `PermissionRequest` is deliberately not installed. Its hook chain races
the interactive permission client, and a fast client decision aborts the losing
hook. Passive capture of that event would therefore be unreliable by design.

ZCode injects `SessionStart` standard output as model context
(`hookSpecificOutput.additionalContext`). Unlike Pool, Zero, and Grok, the prior
session's handoff is therefore delivered automatically at session start. ZCode has
no true session-end event, because `Stop` fires at the end of every turn. Close
finished sessions explicitly. Use the exact session id when several ZCode sessions
are open in the same project:

```bash
ai-memory finalize-session --agent zcode
ai-memory finalize-session --agent zcode --session-id <uuid>
```

The install does not include a first-party `install-mcp` client, and it does not
provide a managed workstream (`ai-memory run zcode`).

### OpenCode

```bash
docker run --rm <imagem-ai-memory>:latest \
    install-mcp --client opencode \
    --server-url "http://homelab:49374/mcp" \
    --auth-token "$TOKEN"

# Plugin, written to ~/.config/opencode/plugins/ai-memory.ts.
# If you have the local wrapper installed, prefer `--apply`:
ai-memory install-hooks --agent opencode --apply \
    --server-url "http://homelab:49374" \
    --auth-token "$TOKEN"

# Container-only preview path. Redirect the output only if you want to write the
# file yourself:
docker run --rm <imagem-ai-memory>:latest \
    install-hooks --agent opencode \
    --server-url "http://homelab:49374" \
    --auth-token "$TOKEN"
```

Restart OpenCode after installing or changing the plugin, because plugins load at
startup.

### OpenCode 2 (beta)

The 2.0 beta installs side by side as `opencode2`. It shares the configuration
directory and the session store of version 1, but its MCP schema and plugin API
changed. Wire it with the `opencode2` client and agent names:

```bash
docker run --rm <imagem-ai-memory>:latest \
    install-mcp --client opencode2 \
    --server-url "http://homelab:49374/mcp" \
    --auth-token "$TOKEN"

# Plugin, written to ~/.config/opencode/plugins/ai-memory-opencode2.ts.
# If you have the local wrapper installed, prefer `--apply`:
ai-memory install-hooks --agent opencode2 --apply \
    --server-url "http://homelab:49374" \
    --auth-token "$TOKEN"
```

Version 2 nests servers under `mcp.servers` and has no `enabled` field. The
version 1 entries (under `mcp`) and the version 2 entries (under `mcp.servers`)
therefore coexist in the one `~/.config/opencode/opencode.json` file (or
`opencode.jsonc`). The beta supports this mixed nesting. Keep both entries, and do
not convert the file by removing the version 1 entry (a confirmar: the upstream
migration guidance). `ai-memory run opencode2` resumes the same native sessions as
`ai-memory run opencode`, through the `opencode2` binary (a confirmar). Both
plugins share the one auto-loaded directory while the beta runs side by side. The
host may warn about its sibling's file, because the two plugin APIs are
incompatible. That warning is benign. `uninstall` removes each file only on its
own ownership markers.

> **Back up `~/.local/share/opencode` before you run the beta against your real
> data.** The two binaries share one `opencode.db` file. The beta has migrated its
> schema in place before, and that left the stable version 1.x broken (a
> confirmar). ai-memory only ever opens that database read-only. The migration
> risk comes from launching `opencode2` itself, not from this integration.
>
> **The beta's background service defaults to port 49374, which is also
> ai-memory's default.** Running both on defaults makes the opencode2 service
> crash-loop with a "port already in use" error. Move one side: run
> `opencode2 service set port <free-port>`, or start ai-memory with
> `--bind 127.0.0.1:<free-port>` and install with the matching `--server-url`.

**On a Gemini or Vertex model, serve Gemini-safe schemas.** OpenCode forwards MCP
tool schemas to the configured provider verbatim. Google's schema type for
function declarations accepts only a single `type` per field. Every optional tool
argument is rendered as a nullable union (`"type": ["integer", "null"]`), and the
provider rewrites that into `any_of` with `description` still next to it. Vertex
then fails the whole session at `tools/list`, with an error that says `any_of`
must be the only field set.

Either option fixes it:

- Set `gemini_safe_schemas = true` in the server's `config.toml`, or set
  `AI_MEMORY_GEMINI_SAFE_SCHEMAS=true`. The server then collapses those unions to
  `"type": "integer"` plus `nullable: true` for every client.
- Append `?flavor=gemini` to only this client's MCP URL.

Runtime validation is unchanged either way. Gemini CLI and Antigravity CLI
normalize schemas on the client side, so they need neither option (a confirmar).

### Oh My Pi / OMP

```bash
docker run --rm <imagem-ai-memory>:latest \
    install-mcp --client omp \
    --server-url "http://homelab:49374/mcp" \
    --auth-token "$TOKEN"

# Extension, written to ~/.omp/agent/extensions/ai-memory-omp.ts.
# If you have the local wrapper installed, prefer `--apply`:
ai-memory install-hooks --agent omp --apply \
    --server-url "http://homelab:49374" \
    --auth-token "$TOKEN"
```

Restart OMP after installing or changing the extension, because extensions load
at startup. The ai-memory CLI accepts `--client omp` (or `--client oh-my-pi`) for
MCP, and `--agent omp` (or `--agent oh-my-pi`) for hooks. Both target OMP's own
`.omp` integration surface.

### Pi

Pi does not read a native `mcp.json`. ai-memory supports Pi through one generated
script extension at `~/.pi/agent/extensions/ai-memory-pi.ts`. The same file
captures lifecycle events and bridges ai-memory's HTTP MCP tools into Pi through
its tool registration API. When `PI_CODING_AGENT_DIR` is set, it relocates Pi's
whole agent home. The extension is then written to
`$PI_CODING_AGENT_DIR/extensions/ai-memory-pi.ts` instead.

The Pi and OMP extensions use distinct file names (`ai-memory-pi.ts` and
`ai-memory-omp.ts`), so installing one never overwrites the other. They are not
interchangeable. Only Pi's extension bridges MCP tools.

#### OMP profiles

`omp --profile <name>` relocates OMP's agent home to
`~/.omp/profiles/<name>/agent`. Point the installer at the same profile, so that
the extension lands where that profile loads it:

```bash
ai-memory install-hooks --agent omp --profile work --apply
# or set it once for the shell:
OMP_PROFILE=work ai-memory install-hooks --agent omp --apply
```

The profile is chosen in this order: `--profile`, then `OMP_PROFILE`, then
`PI_PROFILE`. A named profile takes precedence over `PI_CODING_AGENT_DIR`. When no
profile is named and `PI_CODING_AGENT_DIR` is set, that variable names the agent
directory outright. Otherwise the default `~/.omp/agent` is used. `PI_CONFIG_DIR`
renames the `.omp` directory under the home directory. `uninstall --profile
<name>` removes the same file.

```bash
ai-memory install-hooks --agent pi --apply \
    --server-url "http://homelab:49374" \
    --auth-token "$TOKEN"

# `install-mcp --client pi` prints this guidance instead of writing mcp.json:
ai-memory install-mcp --client pi --server-url "http://homelab:49374/mcp"
```

Restart Pi after installing or changing the extension. OMP, also called Oh My
Pi, remains separate and keeps using the `.omp` paths.

---
### Bind mounts vs docker cp

The `setup-agent` subcommand does the extract and the render in one step, using a
bind mount:

```bash
docker run --rm -v "$HOME/.ai-memory:/host" \
    <imagem-ai-memory>:latest \
    setup-agent --agent claude-code --to /host/hooks \
        --host-prefix "$HOME/.ai-memory/hooks" \
        --server-url "http://homelab:49374" --auth-token "$TOKEN"
```

This works cleanly when the container user's UID matches the host user's UID, as
on a homelab where both are 1000. It fails on rootless Docker and on hosts with
user-namespace remapping enabled (a confirmar). In those setups the container
cannot write to a host directory that belongs to a UID outside the namespace
mapping.

The `docker cp` pattern recommended above avoids this problem, because the
container engine performs the copy and the files end up owned by the user who runs
the command (a confirmar). Prefer `docker cp` as the default. Use `setup-agent`
only when your container setup is known not to remap UIDs.

### Other MCP clients

For the MCP config file path and snippet of each client, see
[MCP client installation](mcp-install.md). You can also configure each client in
one step with the container wrapper:

```bash
docker run --rm <imagem-ai-memory>:latest \
    install-mcp --client cursor          --auth-token "$TOKEN" \
    --server-url "http://homelab:49374/mcp"

docker run --rm <imagem-ai-memory>:latest \
    install-hooks --agent cursor         --auth-token "$TOKEN" \
    --server-url "http://homelab:49374"

docker run --rm <imagem-ai-memory>:latest \
    install-mcp --client claude-desktop  --auth-token "$TOKEN" \
    --server-url "http://homelab:49374/mcp"

docker run --rm <imagem-ai-memory>:latest \
    install-mcp --client gemini-cli      --auth-token "$TOKEN" \
    --server-url "http://homelab:49374/mcp"

docker run --rm <imagem-ai-memory>:latest \
    install-hooks --agent gemini-cli     --auth-token "$TOKEN" \
    --server-url "http://homelab:49374"

docker run --rm <imagem-ai-memory>:latest \
    install-mcp --client antigravity-cli --auth-token "$TOKEN" \
    --server-url "http://homelab:49374/mcp"

docker run --rm <imagem-ai-memory>:latest \
    install-hooks --agent antigravity-cli --auth-token "$TOKEN" \
    --server-url "http://homelab:49374"

docker run --rm <imagem-ai-memory>:latest \
    install-mcp --client grok            --auth-token "$TOKEN" \
    --server-url "http://homelab:49374/mcp"

docker run --rm <imagem-ai-memory>:latest \
    install-hooks --agent grok           --auth-token "$TOKEN" \
    --server-url "http://homelab:49374"

docker run --rm <imagem-ai-memory>:latest \
    install-mcp --client openclaw        --auth-token "$TOKEN" \
    --server-url "http://homelab:49374/mcp"

docker run --rm <imagem-ai-memory>:latest \
    install-hooks --agent openclaw       --auth-token "$TOKEN" \
    --server-url "http://homelab:49374"

docker run --rm <imagem-ai-memory>:latest \
    install-mcp --client kiro-cli        --auth-token "$TOKEN" \
    --server-url "https://memory.example/mcp"

docker run --rm <imagem-ai-memory>:latest \
    install-hooks --agent kiro-cli       --auth-token "$TOKEN" \
    --server-url "https://memory.example"

docker run --rm <imagem-ai-memory>:latest \
    install-mcp --client command-code    --auth-token "$TOKEN" \
    --server-url "http://homelab:49374/mcp"

docker run --rm <imagem-ai-memory>:latest \
    install-hooks --agent command-code   --auth-token "$TOKEN" \
    --server-url "http://homelab:49374"

docker run --rm <imagem-ai-memory>:latest \
    install-mcp --client vscode-copilot  --auth-token "$TOKEN" \
    --server-url "http://homelab:49374/mcp"

docker run --rm <imagem-ai-memory>:latest \
    install-mcp --client zed             --auth-token "$TOKEN" \
    --server-url "http://homelab:49374/mcp"

docker run --rm <imagem-ai-memory>:latest \
    install-mcp --client zcode           --auth-token "$TOKEN" \
    --server-url "http://homelab:49374/mcp"
```

Cursor, Gemini CLI, Antigravity CLI, Grok Build CLI, Kiro CLI, Command Code,
OpenClaw, and ZCode support both `install-mcp` and `install-hooks`. The Grok MCP
installer writes `$GROK_HOME/config.toml`, which defaults to
`~/.grok/config.toml`. Its lifecycle hooks go to `$GROK_HOME/hooks/ai-memory.json`,
which defaults to `~/.grok/hooks/ai-memory.json`. `install-hooks --agent grok`
captures lifecycle events. Grok ignores `SessionStart` standard output, so when you
resume you must accept the handoff through MCP with `memory_handoff_accept`.

Claude Desktop, VS Code Copilot, and Zed are MCP-only here. You need to prompt the
model to call `memory_query` or `memory_handoff_accept` itself. For clients with
`install-hooks` support, the capture path injects the handoff at session start, or
at the client's closest equivalent. The exceptions are Grok and Zero, whose
`SessionStart` does not show standard output to the model, and Antigravity CLI,
which uses `PreInvocation` instead (see the Antigravity notes in the Codex section).

---

## Installing hooks without docker

If you only need to use ai-memory *from* a machine, and that machine does not run
the server, download and verify the release installer first. The installer
downloads and verifies the release's hook archive before it writes any scripts:

```bash
installer_base=<url-base-do-pacote-de-hooks-da-release>
installer_tmp="$(mktemp -d)"
trap 'rm -rf "$installer_tmp"' EXIT
curl -fsSL "$installer_base" -o "$installer_tmp/ai-memory-install-hooks"
curl -fsSL "$installer_base.sha256" -o "$installer_tmp/ai-memory-install-hooks.sha256"
expected="$(awk 'NR == 1 { print $1 }' "$installer_tmp/ai-memory-install-hooks.sha256")"
if command -v sha256sum >/dev/null 2>&1; then
    actual="$(sha256sum "$installer_tmp/ai-memory-install-hooks" | awk '{ print $1 }')"
else
    actual="$(shasum -a 256 "$installer_tmp/ai-memory-install-hooks" | awk '{ print $1 }')"
fi
[ -n "$expected" ] && [ "$actual" = "$expected" ] || { echo "installer checksum mismatch" >&2; exit 1; }
chmod +x "$installer_tmp/ai-memory-install-hooks"
"$installer_tmp/ai-memory-install-hooks" --agent claude-code
rm -rf "$installer_tmp"
trap - EXIT

# Then render the JSON config (it still needs `ai-memory`, either through docker
# as a one-shot run or installed locally):
docker run --rm <imagem-ai-memory>:latest \
    install-hooks --agent claude-code \
        --hooks-dir "$HOME/.ai-memory/hooks" \
        --server-url "http://homelab:49374" \
        --auth-token "$TOKEN"
```

The curl installer accepts these values for `--agent`: `claude-code`, `codex`,
`command-code`, `cursor`, `gemini-cli`, `kimi-code`, `kiro-cli`, `antigravity-cli`,
`grok`, `opencode`, `opencode2`, `openclaw`, `omp` (or `oh-my-pi`), and `pi`. The
default is `claude-code`. Other options are:

- `--to <dir>`, the install root. The default is `$HOME/.ai-memory/hooks`.
- `--ref <release-tag>`, the release to fetch. The default is the latest release.
- `--repo <owner/repo>`, the release repository. The default is the project's
  own repository.

`--help` prints the full flag list. The installer fetches the hook archive that
belongs to the chosen release, and it checks its SHA-256 sidecar before installing
the agent's files. If the checksum does not match, it refuses to install.

OpenCode, OpenClaw, OMP (Oh My Pi), and Pi need no script extraction, because
`install-hooks` generates script plugin or extension files for them instead.
For Pi, the generated extension also provides the MCP bridge.

The generated script integrations behave like the native hooks when the server
is unreachable. A delivery that fails at the network level, or that gets a 5xx
response, is written to `<data_dir>/hook-spool/`, in the format that
`ai-memory hook-drain` reads. The plugin drains that backlog itself once the server
is reachable again. Work done for a day away from the server's network is therefore
captured instead of being dropped. Each spooled entry carries an ingest key, so the
plugin's own drain and a manual `hook-drain` can run at the same time without
ingesting an entry twice. The spool lands in the agent host's data directory
(`AI_MEMORY_DATA_DIR`, or the platform default), which is where a native
`ai-memory` install looks.

This path is the simplest choice when:

- you have curl and bash but not Docker, and
- you do not need to run a local ai-memory server. You are a client of a homelab
  or remote ai-memory.

### Hook command paths across a container boundary

`install-hooks --apply` stages the hook scripts into the data directory, and it
writes their absolute paths into the agent's configuration. When the CLI runs
inside a container but the agent runs on the host, those staged paths would be
container paths that the host cannot see. Set `AI_MEMORY_HOOKS_HOST_ROOT` to the
host directory from which the staged `hooks/` tree is mounted. The rendered
configuration then uses `<host-root>/<agent>/...` as the command paths. The bundled
container wrappers forward this variable automatically. Set it by hand only for
custom container setups.

---

## Running ai-memory without docker

Most users should use the container wrapper described in the basic setup. Arch
Linux users can use the [AUR packages](#arch-linux-native-packages-aur). On any
other host, `mise` installs a tagged release binary directly from GitHub, and no
build toolchain is needed:

```bash
mise use -g github:<dono>/<repositorio>
```

The `mise` GitHub backend downloads the release archive that matches your OS and
architecture, for example `ai-memory-linux-x86_64.tar.gz` or
`ai-memory-macos-aarch64.tar.gz`. It verifies the archive against the published
`.sha256` sidecar, then extracts the archive and puts `ai-memory` on the `PATH`.
The release workflow does not publish artifact attestations or SLSA provenance, so
only the checksum is verified. No dedicated `mise` plugin or registry entry is
needed. The backend works with any repository whose release assets follow this
naming convention. By default `mise` also holds back the very newest release for a
short safety window, so a fresh tag may resolve to the previous version for about a
day (a confirmar: the default window). To pin an exact tag and bypass that window,
use `mise use -g github:<dono>/<repositorio>@1.30.0`.

Installing from the public package registry by name is not available. The package
name is already used on the registry by an unrelated project, and so is the core
internal package name (a confirmar). Publishing would require renaming at least the
core package, and that naming decision has not been made yet.

Build from source only when you are working on ai-memory itself, or when your
platform is not covered above. On macOS, tagged releases also publish native
`ai-memory-macos-aarch64.tar.gz` and `ai-memory-macos-x86_64.tar.gz` archives, for
when you only need the client CLI.

```bash
git clone <url-do-repositorio> ~/.ai-memory
cd ~/.ai-memory
<compilar-binario-de-release>                         # build de release de todos os componentes
<binario-ai-memory> init                              # one-time
<binario-ai-memory> serve --transport http \
    --bind 127.0.0.1:49374                            # MCP + hook HTTP server
```

The default data directory is `~/.local/share/ai-memory` on Linux,
`~/Library/Application Support/ai-memory` on macOS, and the platform local-data
directory on Windows, typically `%LOCALAPPDATA%\ai-memory`. Set
`AI_MEMORY_DATA_DIR=/path` to override it. To require bearer-token auth, set
`AI_MEMORY_AUTH_TOKEN` in the server's environment.

#### Optional serve flags
#### Optional serve flags

The `serve` subcommand also accepts the options below. Each one can also be set
with its environment variable.

| Flag | Env var | What it does |
|---|---|---|
| `--enable-web` | `AI_MEMORY_ENABLE_WEB=true` | Mounts the read-only web browser and the `/api/v1` JSON API. |
| `--base-path /wiki` | `AI_MEMORY_BASE_PATH` | Hosts the whole HTTP surface (`/mcp`, `/hook`, `/admin/*`, `/api/v1`, `/web`) under a configurable subpath. Use it behind a reverse proxy that shares a hostname. Segments `.` and `..` are rejected. Unsafe characters fall back to the root path with a warning. The proxy configuration for a subpath is a confirmation point (a confirmar: no dedicated section in the reverse-proxy guide). |
| `--web-slug /web` | `AI_MEMORY_WEB_SLUG` | Where the web UI mounts inside the base path. The default is `/web`. Set `/` to mount the UI at the base path root. |
| `--web-ui-dir <path>` | `AI_MEMORY_WEB_UI_DIR` | Serves a custom single-page app from `<path>` instead of the built-in browser. ai-memory injects a `<base href>` element and an `<meta name="ai-memory-base-path">` element, so that the app can build relative URLs and API calls under the configured prefix. |
| `--cors-allow-origin <origin>` | `AI_MEMORY_CORS_ALLOW_ORIGINS` (comma-separated) | Lets the listed origins call `/api/v1`. The CORS layer is scoped to that route only. `/mcp`, `/hook`, `/admin`, and `/web` stay origin-locked. |
| _(config only)_ | `AI_MEMORY_HOOK_RATE_PER_SEC`, `AI_MEMORY_HOOK_RATE_BURST` | The optional per-actor and per-session token bucket for hook ingest. See [Spooling and delivery](#spooling-and-delivery) above. |

On macOS, see [external companions and tests](external-companions-and-tests.md),
section 5. Use the archive that matches your architecture: `aarch64` for Apple
Silicon, `x86_64` for Intel. The [menu bar app](#macos-menu-bar-app) is the
self-contained graphical path. It bundles the binary, starts the LaunchAgent, and
opens `/web` and the status view. On Windows, see [Windows support](windows.md).

The short version: run the install commands from the same environment that
launches the agent. Agents launched from WSL2 need WSL paths and POSIX
shell-script hooks. Native Windows agents can use the tagged `ai-memory-windows-x86_64.zip`
archive, the container-wrapper for Docker Desktop, or a source build. Native
Claude Code uses the exec form with a real `ai-memory.exe` by default (a confirmar). The Windows
container wrapper renders the other native Windows script-hook agents as encoded
PowerShell fallback commands.

When you run from source, `install-hooks` finds the bundled scripts in the
repository's `hooks/` directory automatically. Extracted release archives also find
the `hooks/` bundle that sits beside the `ai-memory` binary:

```bash
./target/release/ai-memory install-hooks --agent claude-code --auth-token "$TOKEN"
```

You do not need `setup-agent` in this case, because the scripts already live at
the right host path.

---

## LLM provider tiers

ai-memory works in three intensity tiers:

| Tier | What you get | Env vars | Cost |
|---|---|---|---|
| **Zero-LLM** (default) | Full-text search (FTS5), manually declared entities, graph search, rule-based session summaries, and automatic handoffs built from prompt and tool-call history (rule-based summaries: a confirmar). | (none) | $0 |
| **+ LLM consolidation** | An LLM rewrites session pages as coherent narratives, writes PreCompact checkpoints, and runs LLM-driven contradiction lint (a confirmar: checkpoints and lint). | `AI_MEMORY_LLM_PROVIDER=anthropic` and `ANTHROPIC_API_KEY` | About $0.01–0.05 per session (estimate, a confirmar) |
| **+ Anthropic via subscription** | The same LLM features, using a Claude Pro or Max subscription instead of an API key | `AI_MEMORY_LLM_PROVIDER=anthropic-oauth` and `ANTHROPIC_OAUTH_TOKEN` | Uses your Claude subscription |
| **+ ChatGPT or Codex OAuth** | The same LLM features, using a ChatGPT Pro or Plus login instead of an OpenAI Platform key | `AI_MEMORY_LLM_PROVIDER=openai-oauth` and `ai-memory auth login openai-oauth` | Uses your ChatGPT subscription |
| **+ Codex credential reuse** | The same LLM features, using the login owned by the Codex CLI, without copying or owning its refresh token | `AI_MEMORY_LLM_PROVIDER=codex` and an authenticated Codex CLI | Uses your ChatGPT subscription |
| **+ GitHub Copilot** | The same LLM features, using a GitHub Copilot subscription | `AI_MEMORY_LLM_PROVIDER=copilot` and either `ai-memory auth login copilot` or `COPILOT_GITHUB_TOKEN` | Uses your Copilot subscription |
| **+ LLM reranking** | At most one relevance pass over up to 30 bounded search candidates from projects and scopes (limits: candidates `max(limit, min(limit x 3, 30))`, 20 s per call, 4 concurrent calls). The normal order is kept when a response is invalid, fails, times out, or hits the concurrency limit. | `AI_MEMORY_RERANKER=llm` and any configured LLM provider | One LLM call per eligible query, with at most four running at once |
| **+ Hybrid retrieval** | Adds vector cosine similarity to the FTS5, entity, and graph reciprocal-rank fusion. This improves recall for paraphrased queries. | `AI_MEMORY_EMBEDDING_PROVIDER=openai` and `OPENAI_API_KEY` (or `EMBEDDING_API_KEY`) | About $0.0001 per page on backfill (estimate, a confirmar) |

### Recommended models (chosen as defaults)

If you set only the provider, ai-memory picks a default model:

| Setting | Default | Why |
|---|---|---|
| `AI_MEMORY_LLM_PROVIDER=anthropic` | `claude-haiku-4-5` | **Recommended default.** The best balance of speed, restraint, and classification quality. It is not a reasoning model. It consistently classifies durable project rules as `kind: rule`. |
| `AI_MEMORY_LLM_PROVIDER=anthropic-oauth` | `claude-sonnet-4-6` | Anthropic through a Claude subscription. Run `claude setup-token` once, then set `ANTHROPIC_OAUTH_TOKEN` (or `CLAUDE_CODE_OAUTH_TOKEN`). No `ANTHROPIC_API_KEY` is needed. It uses the same messages endpoint, with bearer-token auth. |
| `AI_MEMORY_LLM_PROVIDER=openai` | `gpt-5.4-mini` | A cheaper and faster alternative with the same parse reliability. It over-classifies thin sessions slightly more. |
| `AI_MEMORY_LLM_PROVIDER=openai-oauth` | `gpt-5.5` | The ChatGPT and Codex backend. Run `ai-memory auth login openai-oauth` once. ai-memory stores the refresh token in `<data_dir>/auth.json` and refreshes access tokens automatically. The optional `AI_MEMORY_LLM_REASONING_EFFORT` setting (`none`, `minimal`, `low`, `medium`, `high`, `xhigh`, `max`, `ultra`, or `persistent`) maps to each provider's reasoning field. Leave it unset to keep the model default. |
| `AI_MEMORY_LLM_PROVIDER=codex` | `gpt-5.6-luna` | Reuses only `access_token` and `account_id` from the Codex CLI's `auth.json`. Token renewal is delegated to `codex app-server --stdio`. |
| `AI_MEMORY_LLM_PROVIDER=copilot` | `gpt-5.5` | The GitHub Copilot Chat backend. ai-memory stores a GitHub user token in `<data_dir>/auth.json`, exchanges it for a short-lived Copilot API token, and refreshes that token before it expires. |
| `AI_MEMORY_LLM_PROVIDER=gemini` | `gemini-3.5-flash` | Google's hosted option, with a generous free tier. ai-memory disables the default dynamic thinking of Gemini 3.5 Flash, so that hidden thought tokens do not truncate strict JSON. Set `GEMINI_API_KEY` (or `GOOGLE_API_KEY`). |
| `AI_MEMORY_LLM_PROVIDER=opencode` | `claude-sonnet-4-6` | The OpenCode cloud API. It defaults to the **Go** endpoint, `opencode.ai/zen/go/v1`, which is a cost-optimized model subset. GPT-5.6 Luna uses the Go Responses endpoint. Other models use Chat Completions. For **Zen**'s full catalogue, set `AI_MEMORY_LLM_BASE_URL=https://opencode.ai/zen/v1` and an `AI_MEMORY_LLM_MODEL` from that catalogue (the default model ID belongs to Go). Requests identify ai-memory by version, and one session header is reused across related attempts. Both endpoints take `OPENCODE_API_KEY`. The alias `opencode-zen` is historical. It selects the Go endpoint like the other names, because the base URL decides the endpoint, not the alias. |
| `AI_MEMORY_EMBEDDING_PROVIDER=openai` | `text-embedding-3-small` (1536 dimensions) | About five times cheaper than `-3-large`, with a marginal loss in recall. |
| `AI_MEMORY_EMBEDDING_PROVIDER=openai` and `AI_MEMORY_EMBEDDING_BASE_URL=https://openrouter.ai/api/v1` | `openai/text-embedding-3-small` through OpenRouter | Uses `EMBEDDING_API_KEY`, else `LLM_API_KEY` or `OPENAI_API_KEY`, with the OpenAI-compatible embedding client. |
| `AI_MEMORY_EMBEDDING_PROVIDER=openai` and `AI_MEMORY_EMBEDDING_BASE_URL=https://api.orcarouter.ai/v1` | `openai/text-embedding-3-small` through OrcaRouter | Uses `EMBEDDING_API_KEY`, else `LLM_API_KEY`, with the OpenAI-compatible embedding client (a confirmar: this base URL is not in the source). |
| `AI_MEMORY_EMBEDDING_PROVIDER=voyage` | `voyage-3` (1024 dimensions) | The current general-purpose recommendation from Voyage. |
| `AI_MEMORY_EMBEDDING_PROVIDER=google` or `gemini` | `gemini-embedding-001` (768 dimensions) | Google-hosted embeddings through the embedding-content call. Set `GEMINI_API_KEY` (or `GOOGLE_API_KEY`). |
| `AI_MEMORY_EMBEDDING_PROVIDER=openai-compat` | No default. Set the model, the dimension, and the base URL explicitly. | Self-hosted engines such as Ollama, LM Studio, and vLLM. It needs no key by default. `EMBEDDING_API_KEY`, else `LLM_API_KEY`, is sent as a bearer token when it is present, for gateways. Example: `AI_MEMORY_EMBEDDING_BASE_URL=http://localhost:11434/v1`, `AI_MEMORY_EMBEDDING_MODEL=nomic-embed-text`, `AI_MEMORY_EMBEDDING_DIM=768`. Changing an existing `openai` setup with a base URL to `openai-compat` changes the stored provider, model, and dimension triple. Run `ai-memory embed --force` to re-embed. |
| `AI_MEMORY_EMBEDDING_PROVIDER=copilot` | `text-embedding-3-small` (1536 dimensions) | Reuses the OAuth login of the `copilot` LLM provider (`ai-memory auth login copilot`, `COPILOT_GITHUB_TOKEN`, or `GITHUB_COPILOT_API_TOKEN`), so no separate API key is needed. It calls the Copilot embeddings endpoint, following the OpenAI-compatible contract that Copilot documents for chat. That endpoint's exact shape has no live test against Copilot in this project, so treat it as needing a smoke test against a real Copilot account. |

> **What we don't recommend:** reasoning-mode models, such as Claude with extended
> thinking, GPT-o3, and the Gemini thinking variants. They spend the token budget on
> internal reasoning, and with the strict-JSON consolidation prompt they hang or
> return empty responses. If you must use one, turn reasoning off.

#### Embedding credentials (`EMBEDDING_API_KEY`)

`EMBEDDING_API_KEY` is optional. It provides credentials for the embedding role
alone. Set it when the embedder should authenticate against a different provider
than the LLM. Otherwise the `openai` and `openai-compat` embedders borrow the chat
model's `OPENAI_API_KEY` or `LLM_API_KEY`. That key is then sent to whatever
`AI_MEMORY_EMBEDDING_BASE_URL` points at, and the request is rejected with a 401.

For `AI_MEMORY_EMBEDDING_PROVIDER=openai`, the key is resolved in this order:

1. `EMBEDDING_API_KEY`.
2. `OPENAI_API_KEY`.
3. `LLM_API_KEY`, only when `AI_MEMORY_EMBEDDING_BASE_URL` is set.

`openai-compat` checks `EMBEDDING_API_KEY`, then `LLM_API_KEY`, and it stays
keyless when neither is set. `voyage` and `google` or `gemini` are not affected.
They read their own `VOYAGE_API_KEY`, or `GEMINI_API_KEY` or `GOOGLE_API_KEY`, and
they never borrow another role's key. When `EMBEDDING_API_KEY` is unset, key
resolution is the same as it was before that variable existed.

```bash
# Chat on api.openai.com. `openai` is the provider that sends
# `max_completion_tokens`, which gpt-5 and o-series models require, and
# `openai-compat` never sends it. Embeddings go to another endpoint.
export AI_MEMORY_LLM_PROVIDER=openai
export OPENAI_API_KEY=sk-...
export AI_MEMORY_EMBEDDING_PROVIDER=openai
export AI_MEMORY_EMBEDDING_BASE_URL=https://openrouter.ai/api/v1
export EMBEDDING_API_KEY=sk-or-v1-...
```

### Anthropic via Claude subscription (OAuth)

> **Unofficial, and against Anthropic's usage policies. Use at your own risk.**
> Anthropic provides no public OAuth API for the Claude Pro or Max subscription.
> This path reuses the `claude setup-token` credential against the messages
> endpoint. It is **not a supported or sanctioned integration**. Anthropic's terms
> reserve subscription (Claude Code) access for interactive use. Using it as an
> automated API backend may breach those terms, and **could get your account
> rate-limited, flagged, or banned**. The request-header recipe is also
> undocumented and can change without notice. For a supported path, use the
> `anthropic` provider with a real Platform API key. This option is an opt-in
> convenience only, and no guarantees are made about it.

`anthropic-oauth` is for Claude Pro or Max subscribers who want to use their
existing subscription instead of an Anthropic Platform API key. It calls the
**same** messages endpoint as the `anthropic` provider. Only the auth headers
differ: a bearer token plus the `anthropic-beta` header with the values
`oauth-2025-04-20` and `claude-code-20250219`.

```bash
# Obtain a token once, using the Claude Code CLI:
claude setup-token

# Then export it. The CLI may also write CLAUDE_CODE_OAUTH_TOKEN automatically:
export ANTHROPIC_OAUTH_TOKEN=<paste token here>
export AI_MEMORY_LLM_PROVIDER=anthropic-oauth
ai-memory serve
```

For Docker, pass the token as an environment variable:

```bash
docker run -d --name ai-memory \
    -p 127.0.0.1:49374:49374 \
    -v ai-memory-data:/data \
    -e AI_MEMORY_LLM_PROVIDER=anthropic-oauth \
    -e ANTHROPIC_OAUTH_TOKEN=<token> \
    <imagem-ai-memory>:latest
```

Both `ANTHROPIC_OAUTH_TOKEN` and `CLAUDE_CODE_OAUTH_TOKEN` are accepted.
ai-memory checks `ANTHROPIC_OAUTH_TOKEN` first. When either variable is exported
on the host, the POSIX and PowerShell container wrappers forward its name to
short-lived helper commands such as `llm-test`. The token value is inherited by
the container engine and is not placed on the wrapper's command line. The
long-lived server container still needs the provider and token variables in its
own environment, as in the example above.

For both Anthropic providers, ai-memory omits `temperature` for Claude 4.7 and
later models, and for the Claude Mythos Preview model. Those models reject
non-default sampling parameters (every recognized Claude model from 4.7 on, in any family, plus Mythos Preview). `llm-test`
starts with the same representative value of 0.2 that bootstrap and consolidation
use, and then applies the provider's compatibility normalization before it sends
the request.

> **Pick a small, fast model.** ai-memory's LLM work (session consolidation, lint,
> and explore) is summarization and extraction, not hard reasoning. A Haiku-class
> model is plenty. It is faster, cheaper, and much easier on subscription rate
> limits than Sonnet or Opus. For example, set `AI_MEMORY_LLM_MODEL=claude-haiku-4-5`.
> Keep the high-effort thinking models for your coding agent.

### OpenAI OAuth / Codex

`openai-oauth` is for ChatGPT Pro, Plus, and Codex accounts. It does **not** use
`OPENAI_API_KEY`, and it does **not** call the public OpenAI API. Requests go to
the ChatGPT and Codex responses backend, with a refreshable OAuth token.

For the container quick-start wrapper, this writes into the same named volume that
the server mounts at `/data`:

```bash
ai-memory auth login openai-oauth
docker run -d --name ai-memory \
    -p 127.0.0.1:49374:49374 \
    -v ai-memory-data:/data \
    -e AI_MEMORY_LLM_PROVIDER=openai-oauth \
    <imagem-ai-memory>:latest
```

For a remote container host, run the login on that host, against the same
container or data volume:

```bash
docker exec -it ai-memory ai-memory auth login openai-oauth
```

Use `ai-memory auth status` to check whether a token is present. Use
`ai-memory auth logout openai-oauth` to remove it.

> **Leave the model at the provider default (`gpt-5.5`).** The Codex and ChatGPT
> backend behind `openai-oauth` accepts only a small, server-defined set of model
> IDs, and it rejects others with a deterministic 400 error (for example
> `gpt-5-mini`) (behavior of the external service; ai-memory does not validate the id). Do not set `AI_MEMORY_LLM_MODEL` for this backend.
> Consolidation, lint, and explore are summarization tasks. If the default
> reasoning is too heavy, set `AI_MEMORY_LLM_REASONING_EFFORT=none` or `low`, so
> that hidden thought tokens do not consume the JSON budget. Keep high-effort
> reasoning for your coding agent.

### Codex credential reuse

The separate `codex` provider reads `$CODEX_HOME/auth.json`, and it falls back to
`.codex/auth.json` in the platform home directory. It takes only
`tokens.access_token` and `tokens.account_id`, reloads them before every call, and
never copies or writes the file. On the first 401 response, it asks
`codex app-server --stdio` to refresh the credential owned by the Codex CLI, and
then retries the responses request once.

```bash
export AI_MEMORY_LLM_PROVIDER=codex
export AI_MEMORY_LLM_MODEL=gpt-5.6-luna
export AI_MEMORY_LLM_REASONING_EFFORT=medium
ai-memory llm-test --provider codex --model gpt-5.6-luna --prompt "Reply with OK"
ai-memory llm-test --provider codex --model gpt-5.6-luna --structured --prompt "Return a short answer"
```

`AI_MEMORY_CODEX_EXECUTABLE` optionally selects another Codex binary. File storage
is supported. `auto` is supported when it resolves to the same `auth.json`.
Keyring-only and ephemeral storage are not supported. The container wrapper is
outside the automatic setup path. The executable and the credentials must both be
available inside the same container or environment.
### GitHub Copilot

The `copilot` provider takes a GitHub user token, then exchanges it for a
short-lived Copilot API token through the GitHub Copilot token endpoint on the
GitHub API host. The raw GitHub token is never sent to the Copilot API host.

For the container quick-start wrapper:

```bash
ai-memory auth login copilot
docker run -d --name ai-memory \
    -p 127.0.0.1:49374:49374 \
    -v ai-memory-data:/data \
    -e AI_MEMORY_LLM_PROVIDER=copilot \
    <imagem-ai-memory>:latest
```

For a remote container host, run the login against the same data volume:

```bash
docker exec -it ai-memory ai-memory auth login copilot
```

Non-interactive deployments can set `COPILOT_GITHUB_TOKEN` instead. When ai-memory
runs natively, it also accepts `GH_TOKEN` and `GITHUB_TOKEN`. In a container,
prefer the explicit `COPILOT_GITHUB_TOKEN`, so that you do not pass a broad token
by accident. Advanced users with a pre-minted Copilot API token can set
`GITHUB_COPILOT_API_TOKEN`, and optionally `COPILOT_API_URL`.

`auth login copilot` uses GitHub Copilot's public device-flow client ID by default.
Pass `--client-id`, or set `AI_MEMORY_COPILOT_CLIENT_ID`, if you operate your own
OAuth app.

### OpenAI-compatible providers (Ollama, vLLM, LM Studio, hosted APIs)

```bash
docker run -d --name ai-memory \
    -p 49374:49374 \
    -v ai-memory-data:/data \
    -e AI_MEMORY_AUTH_TOKEN="$TOKEN" \
    -e AI_MEMORY_LLM_PROVIDER=openai-compat \
    -e AI_MEMORY_LLM_BASE_URL=http://host.docker.internal:11434/v1 \
    -e AI_MEMORY_LLM_MODEL=qwen2.5-coder:14b \
    <imagem-ai-memory>:latest
```

There is no safe default model for `openai-compat`, so `AI_MEMORY_LLM_MODEL` is
required. For OpenRouter, which hosts models such as Kimi and DeepSeek:

```bash
-e AI_MEMORY_LLM_PROVIDER=openai-compat
-e AI_MEMORY_LLM_BASE_URL=https://openrouter.ai/api/v1
-e AI_MEMORY_LLM_MODEL=moonshotai/kimi-k2.6
-e LLM_API_KEY=sk-or-v1-...
```

`AI_MEMORY_LLM_REASONING_EFFORT` also applies on this path. The effect depends on
the endpoint:

- OpenRouter hosts receive `reasoning: { effort, exclude: true }`.
- The xAI (Grok) endpoint receives `reasoning_effort` in Chat Completions format,
  clamped to `low`, `medium`, `high`, or `xhigh`, because Grok cannot disable
  reasoning.
- Other compatible endpoints receive an OpenAI-style `reasoning_effort`.
- Anthropic and Anthropic-OAuth map the same setting to `output_config.effort`, and
  to adaptive or disabled thinking, on the models that accept those fields. The
  Haiku 4.5 model omits them, so the default model does not return a 400 error.
  Fable 5, Mythos 5, and Mythos Preview omit `thinking: disabled`, because those
  models reject it.
- `ultra` and `persistent` are clamped to `max` on OpenAI-style hosts.
- Gemini and Copilot ignore the setting.

A hosted gateway that is not listed above can be used with the same provider,
without any gateway-specific provider. Pass its API key through `LLM_API_KEY`, set
`AI_MEMORY_LLM_BASE_URL` to its OpenAI-compatible endpoint, and set a model that the
gateway offers. Two examples follow. The first is Atlas Cloud, whose endpoint
(`https://api.atlascloud.ai/v1`) is a confirmation point, because the source does
not name it. The second is OrcaRouter, whose endpoint (`https://api.orcarouter.ai/v1`)
is also a confirmation point.

```bash
-e AI_MEMORY_LLM_PROVIDER=openai-compat
-e AI_MEMORY_LLM_BASE_URL=https://api.atlascloud.ai/v1
-e AI_MEMORY_LLM_MODEL=qwen/qwen3.5-flash
-e LLM_API_KEY="$ATLASCLOUD_API_KEY"
```

```bash
-e AI_MEMORY_LLM_PROVIDER=openai-compat
-e AI_MEMORY_LLM_BASE_URL=https://api.orcarouter.ai/v1
-e AI_MEMORY_LLM_MODEL=openai/gpt-4o
-e LLM_API_KEY=sk-orca-...
```

Replace the model with a current model ID from the gateway. The OrcaRouter model
IDs use the same `provider/model` format as OpenRouter, for example
`anthropic/claude-sonnet-4.6`. ai-memory does not pick a default model for hosted
compatibility endpoints.

Structured calls to OpenAI-compatible endpoints use the operation's JSON Schema by
default. The setting is `AI_MEMORY_LLM_COMPAT_STRICT`, which defaults to `true`:

```bash
-e AI_MEMORY_LLM_COMPAT_STRICT=true
```

Current Ollama, vLLM, LM Studio, llama.cpp, and gateway endpoints accept this
OpenAI-style `response_format` with a JSON Schema. If an endpoint explicitly rejects
the structured-output field, or returns a malformed response shape, ai-memory
retries with its tolerant parser. For an endpoint that does not support the field,
turn the setting off:

```bash
-e AI_MEMORY_LLM_COMPAT_STRICT=false
```

Hosted gateways that stream long completions can run past the default per-request
ceiling of 300 seconds. Such a request fails with `http: error sending request`.
Raise the ceiling to match the gateway's worst-case generation time:

```bash
-e AI_MEMORY_LLM_TIMEOUT_SECS=900
```

#### Send a caller-identifying header to a gateway that requires one

Every chat request already carries `User-Agent: ai-memory/<version>`, so a gateway
can tell what is calling it. Some gateways also require a header of their own, for
request correlation, and they reject or throttle traffic without it. Declare those
headers once. They are sent on every chat request, whatever the provider:

```bash
-e AI_MEMORY_LLM_HEADERS=x-opencode-session=prod-01,x-opencode-client=ai-memory
```

Each entry has the form `Name=Value` or `Name: Value`, and entries are separated
by commas. A header value cannot contain a comma when it is set through the
environment variable. Use `llm_headers = [...]` in `config.toml` when a value must
contain one. These headers are refused at startup, and not duplicated onto the
request, because ai-memory sets them itself: `anthropic-beta`, `anthropic-version`,
`authorization`, `content-length`, `content-type`, `host`, `openai-beta`,
`x-api-key`, and `x-goog-api-key`. An entry for `user-agent` overrides the default.
Header values are never logged.

The `opencode` provider needs no configuration for this. OpenCode asks callers for
the `x-opencode-session` header, and that provider already sends one ID per logical
operation. The ID stays the same across retries and across the structured-output
fallback, so one consolidation pass appears as one operation in OpenCode's metrics.
Set the header through `AI_MEMORY_LLM_HEADERS` to override it, for example to tell
several ai-memory instances apart under one account.

#### Match the consolidation budget to a local model's context window

Consolidation defaults to an input target of about 100,000 tokens and an output
limit of 32,000 tokens. Those values suit a provider with a 200,000-token context.
A local model with a smaller window can reject the whole request. llama.cpp reports
this as `exceed_context_size_error`, and most gateways return HTTP 400. Lower both
limits, so that their sum fits the real context window with headroom for tokenizer
differences:

```bash
# For example, a model loaded with an 8k context window:
-e AI_MEMORY_CONSOLIDATION__MAX_INPUT_TOKENS=6500
-e AI_MEMORY_CONSOLIDATION__MAX_OUTPUT_TOKENS=1000
```

The double underscore separates the `[consolidation]` section from each key. The
input target covers the rendered observations, the current page body, the system
prompt, the page conventions, the bounded slot snapshots, the structured-output
schema, and a reserve for the provider's request envelope. Tokenizers differ, so
the budget is a conservative estimate, not an exact provider token count. When the
automatic checkpoint provider fails, the checkpoint degrades to a rule-based page
instead of being lost. Right-sized limits are still what let LLM consolidation
succeed. Startup rejects an input target below 6,000 tokens, and an output limit
below 1,000 tokens. The batch schema and a useful response do not fit reliably
below those floors.

---

## Common subcommands

This is the operational shortlist. Run `ai-memory --help` for the authoritative full
command tree.

There are two ways to invoke a subcommand against the container deployment:

```bash
# A) Against the running container. These are stateful: status, search, backup,
#    checkpoints, restore-page, audit-contamination, forget-sweep, lint, embed.
docker exec ai-memory ai-memory status --json
docker exec ai-memory ai-memory search "karpathy"
docker exec ai-memory ai-memory backup --to /data/snapshot.tar.gz

# B) One-shot, with no running container needed for pure-stdout helpers
#    (generate-auth-token, completions, install-mcp, install-hooks, setup-agent,
#    llm-test).
#    Auth login is stateful. Run it with docker exec against the running container,
#    or through the wrapper, so that it writes into the same data volume as the server.
docker run --rm <imagem-ai-memory>:latest generate-auth-token
docker run --rm <imagem-ai-memory>:latest completions zsh
docker run --rm <imagem-ai-memory>:latest install-mcp --client cursor
docker run --rm <imagem-ai-memory>:latest --help     # full subcommand tree
```

| Subcommand | Pattern | What it does |
|---|---|---|
| `serve` | `docker compose up -d` (already done) | Runs the HTTP MCP server. |
| `run [harness] [args...]` | host wrapper or native binary | Opts into one managed cross-harness workstream. Omit the harness to resume the newest usable local session, or name one explicitly: Claude Code, Codex, OpenCode, OpenCode 2, Pi, Crush, Kimi Code, Command Code, Kiro CLI (v2 or v3), OMP, Grok Build CLI, or Antigravity CLI. The `--yolo` and `--fresh` flags belong to the wrapper. Other native arguments pass through. |
| `show [--json]` | host wrapper or native binary | Chooses a client-local checkout and an installed managed harness, or returns structured discovery data without launching. Remote servers never provide checkout paths. |
| `continue [--workspace NAME]` | host wrapper or native binary | From any directory, revalidates and resumes the newest client-local managed checkout. Accepts `--yolo` and `--fresh`, but no harness-native arguments. |
| `resume [--workspace NAME] [--limit N]` | host wrapper or native binary | Interactively chooses a recent managed workstream from valid client-local checkouts. Up and Down select the workstream. Left and Right cycle through `auto` and the installed harnesses before launch. Accepts `--yolo` and `--fresh`. |
| `workstreams [--workspace NAME] [--project NAME] [--limit N] [--json]` | host wrapper or native binary | Lists recent workstreams that can be selected from the current checkout, including the current selection and the linked harnesses. Does not expose paths or native session IDs. |
| `rename-workstream (--from NAME \| --workstream-id ID) --to NAME [--workspace NAME] [--project NAME] [--json]` | host wrapper or native binary | Retitles one workstream that can be selected from the current checkout. This changes metadata only. The stable ID, the ledger, the listing order, and the current selection are unaffected. |
| `workstream-search [query]` | managed child or thin HTTP client | Searches the full visible managed-workstream ledger. A managed child receives its workstream ID automatically. |
| `status` | `docker exec` | Shows counts, paths, derived-index diagnostics, and passive LLM and embedding provider health. |
| `search "<query>"` | `docker exec` | Runs a wiki full-text search with bounded source authority. Use the MCP `memory_query` tool for entity, graph, and vector fusion. |
| `write-page` | `docker exec` | Writes a page manually. The write is atomic and indexed. |
| `backup --to` and `restore --from` | `docker exec` | Snapshots or restores the data directory. |
| `checkpoints` and `restore-page` | `docker exec` | Lists the wiki git checkpoints, or restores one markdown page and reindexes it. |
| `audit-contamination` | `docker exec` | Runs a read-only structural audit for likely cross-project contamination. |
| `forget-sweep`, `lint`, `embed` | `docker exec` | Manual maintenance. The sweep and lint also run on the server schedule by default (a confirmar). |
| `commit -m "..."` | `docker exec` | Stages and commits the wiki tree. The default message is `manual commit`. |
| `reset --confirm` | `docker exec` | Wipes the data. Refuses while sibling processes are alive. |
| `generate-auth-token` | `docker run --rm` | Prints a random hex bearer token. |
| `user add-human`, `list`, `reset-password`, `disable`, `enable`, `patch` | `docker exec` or native binary | Manages human identities and temporary passwords. Never issues an API key. |
| `user add`, `expire`, `revive`, `rotate-token` | — | Removed in 3.0. Existing 1.x tokens survive as `legacy-user-token` API keys managed with `api-key`. |
| `api-key add`, `list`, `rotate`, `revoke` | `docker exec` or native binary | Manages native `aim_` machine credentials. |
| `auth login openai-oauth` | same data volume as the server | Stores a ChatGPT or Codex OAuth refresh token for the optional `openai-oauth` LLM provider. |
| `auth login copilot` | same data volume as the server | Stores a GitHub token for the optional `copilot` LLM provider. |
| `auth login oidc-device` | same developer data directory as the native hooks and thin-client CLI commands | Stores a per-developer OIDC device token, used for native hook authentication and as HTTP CLI fallback auth. |
| `install-mcp --client` | `docker run --rm` | Prints the MCP configuration snippet for a client. |
| `install-hooks --agent` | `docker run --rm` | Prints the hook configuration snippet for an existing hooks directory. |
| `setup-agent --agent --to --host-prefix` | `docker run --rm -v` | Extracts the bundled scripts and prints the configuration, in one shot. |
| `install-instructions [--target] [--print] [--no-skills]` | the host environment that the agent's prompt files live in | Installs or updates the slim routing block in `CLAUDE.md` or `AGENTS.md`. By default it also installs or updates the managed ai-memory Agent Skills. |
| `install-skills [--scope] [--agent]` | the host environment that the agent's skill directories live in | Installs or updates only the managed ai-memory Agent Skills. |
| `uninstall --apply` | the host environment used for the install | Removes only the ai-memory-owned hooks, MCP entries, instruction blocks, managed skill files, and generated plugin files, after validating their content and markers. Use `--mcp-url` for a custom MCP endpoint. Use `--mcp-name` only to narrow the removal. |
| `llm-test --provider ...` | `docker run --rm -e ...` | Smoke-tests an LLM provider. |
| `completions <shell>` | `docker run --rm` or native binary | Prints a completion script for bash, zsh, fish, PowerShell, or elvish. See the binary's own `completions --help`. |
### Managed routing snippets and Agent Skills

The routing install is prompt packaging for agents. It does not add a runtime skill
router, and `SKILL.md` files are not durable memory pages. The wiki remains the
durable source of truth.

`ai-memory install-instructions` writes two managed prompt artifacts by default:

1. A slim instruction block in `CLAUDE.md`, in `AGENTS.md`, or in the file passed
   with `--target`. The block is bounded by `<!-- ai-memory:start -->` and
   `<!-- ai-memory:end -->` delimiters, which appear alone on their own lines.
2. Managed ai-memory Agent Skills that contain the detailed tool-routing guidance.

Re-running the command is safe. If a project still has the old, long ai-memory
block between the line-anchored markers, the refresh replaces that block in place
with the slim snippet. It leaves unrelated instructions before and after the block
alone. It also writes a timestamped `.bak-*` backup before it changes an existing
file. Managed skill files contain an ai-memory ownership marker. Same-name user
skills without that marker are preserved, unless you explicitly force the
replacement. The embedded skill payloads use LF line endings on every release
platform, so the CLI installs and the `memory_install_self_routing` tool return the
same bytes on Windows, Linux, and macOS. This does not rewrite line endings in
files that users wrote. `install-instructions --print` previews only the
instruction snippet. Run `install-skills --print` to preview the managed skill
payloads.

`install-instructions` flags for skills:

| Flag | Meaning |
|---|---|
| `--no-skills` | Refreshes only the marked instruction block. |
| `--skills-scope <scope>` | Chooses project-local or user-global skill roots. Values: `project`, `global`. Defaults to `project`. |
| `--skills-agent <agent>` | Chooses the skill root: `.claude/skills`, `.agents/skills`, `.devin/skills`, `.grok/skills`, or both the Claude and Agents roots. Values: `claude-code`, `agents`, `devin`, `grok`, `both`. By default, a `CLAUDE.md` target implies `claude-code`, an `AGENTS.md` target implies `agents`, and both files together imply `both`. |
| `--skills-target-dir <dir>` | Writes managed skill directories below an explicit root, instead of inferring the root from scope and agent. |
| `--skills-force` | Replaces unmanaged same-name skills during `install-instructions`. Without it, those skills are left untouched, and the command exits with an actionable error. |

Use `install-skills` when the instruction block is already correct and only the
Agent Skill files need a refresh:

```bash
ai-memory install-skills
ai-memory install-skills --scope global --agent agents
ai-memory install-skills --scope global --agent devin
ai-memory install-skills --scope global --agent grok
ai-memory install-skills --agent both --print
ai-memory install-skills --target-dir .custom/skills --force
```

`install-skills` flags:

| Flag | Meaning |
|---|---|
| `--scope <scope>` | Installs into this project, or into the current user's global skill roots. Values: `project`, `global`. Defaults to `project`. |
| `--agent <agent>` | Installs into Claude Code's skill root, the cross-agent skill root, Devin's skill root, Grok's skill root, or both the Claude and Agents roots. Values: `claude-code`, `agents`, `devin`, `grok`, `both`, and `hermes` (which uses `.hermes/skills`). Defaults to `claude-code`. |
| `--target-dir <dir>` | Writes managed skill directories below an explicit root. `--scope` and `--agent` are ignored. |
| `--print` | Prints the target paths and the `SKILL.md` contents, without writing files. |
| `--force` | Replaces unmanaged same-name skills. Without it, user-authored skills with the same name are preserved. |

Default skill target roots:

| Scope | `--agent claude-code` | `--agent agents` | `--agent devin` | `--agent grok` |
|---|---|---|---|---|
| `project` | `.claude/skills` | `.agents/skills` | `.devin/skills` | `.grok/skills` |
| `global` | `~/.claude/skills`, or `$CLAUDE_CONFIG_DIR/skills` when that variable is set | `~/.agents/skills` | Windows: `%APPDATA%\devin\skills`. Other systems: `~/.devin/skills`. | `$GROK_HOME/skills`, which defaults to `~/.grok/skills` |

Each managed skill is written as `<root>/<skill-name>/SKILL.md`.

`ai-memory uninstall --only skills --apply` removes managed skill files only from
the default project and global roots shown above. It validates the ai-memory
ownership marker first. If you installed with `--target-dir` or
`--skills-target-dir`, clean up that custom root by hand.

The data directory inside the container is `/data`, mounted through the compose
volume. Outside a container, override it with `AI_MEMORY_DATA_DIR=/path`.

Scheduled maintenance is configured in the `[maintenance]` section of `config.toml`.
By default, the rule-based lint and the forget sweep each run once a day, outside
hook latency, across every existing workspace and project. Each interval defaults
to 86,400 seconds. Embedding backfill is supported, but it defaults to off, because
it can call a paid provider. If you set `embedding_backfill_interval_secs` after you
configure an embedder, each scheduled tick backfills every existing workspace and
project, which may increase provider usage.

The forget sweep and the rule-based lint record their last successful completion.
On restart, a job that is not yet due waits only for the rest of its interval. A
job that has never run, or that is overdue, runs once after a bounded startup delay.
A failed run is not recorded as a success, and it retries after that bounded delay.
Embedding backfill stays opt-in, and it keeps interval-only behavior with no startup
catch-up.

---
## Bootstrap mid-project

When you adopt ai-memory in a project that has existed for a while, the wiki starts
empty. `ai-memory bootstrap` ingests the project's existing history into seed pages,
so that the first session has warm context.

```bash
cd /path/to/project
ai-memory bootstrap
```

If you installed the container wrapper and started the server on
`127.0.0.1:49374`, the wrapper reaches that loopback server from its short-lived
helper container automatically. Set `AI_MEMORY_SERVER_URL=http://<server>:49374`
only when the server is remote, or when it uses a custom host or port.

**What gets ingested by default.** The sources below are listed in the order in
which they are dropped when the input is over budget. The first row is never
dropped.

| Source | Priority (dropped first when over budget) |
|---|---|
| `CLAUDE.md` and `AGENTS.md` (project rules) | never dropped |
| `README.md` at the repository root | very late |
| Documentation files under `docs/` | late |
| Substantive git commits (body longer than 120 characters, or a conventional-commit prefix) | middle |
| Module-level documentation comments at the top of source files | first to drop |

**Flags:**

```text
--repo-path <PATH>         (default: git rev-parse --show-toplevel)
--workspace <NAME>         (default: the nearest `.ai-memory.toml` marker's
                            `workspace`, else "default")
--project <NAME>           (default: the marker's `project` when pinned,
                            else derived from cwd: the main repository root's
                            basename via `git rev-parse --show-toplevel`,
                            or basename(cwd) when no repository is found.
                            "scratch" is only a defensive fallback for
                            hook events with no usable cwd.)
--max-input-tokens N       (default: 150000; total source budget after prune)
--chunk-input-tokens N     (default: 24000; per LLM call; 0 = single call)
--since "30 days ago"      (git log filter; supports "N days/months/years ago" and YYYY-MM-DD)
--exclude-git              (skip commit history)
--exclude-readme           (skip README)
--exclude-docs             (skip the documentation files under docs/)
--exclude-code             (skip the module-level documentation comments)
--dry-run                  (collect and estimate, but do not call the LLM or write)
--force                    (re-bootstrap; overwrites the prior manifest)
--resume                   (continue an interrupted bootstrap, reusing the chunks
                            already completed for the same sources)
```

**Cost.** With the example hosted model from the original guide, a 50k-token input
cap and 1-2k generated tokens cost well under $0.20 per run (a confirmar: prices
come from the provider, not from the source).

**Idempotency.** The first run writes a manifest page named `bootstrap.md` into
the project's wiki. The manifest lists every page that was generated, with a
one-paragraph rationale. Re-running without `--force` fails with an error. To start
a clean re-bootstrap, delete the manifest and the generated pages.

**Dry-run first.** It is always worth running a dry run before the real call. It
shows which sources would actually be sent, and how many tokens they represent. The
output is JSON on standard output:

```bash
ai-memory bootstrap --dry-run
{
  "sources_collected": 117,
  "sources_sent": 22,
  "sources_dropped": 95,
  "sources_by_kind": {"git_commits": 14, "readme": 1, "doc_files": 5, "module_headers": 0, "project_rules": 2},
  "estimated_input_tokens": 48760,
  "pages_written": [],
  "rationale": "(dry-run; LLM not invoked)",
  "dry_run": true,
  "llm_chunks": 1
}
```

The command first prints a human-readable report, then a line `--- machine-readable ---`, then the JSON above. Project-rules files are `CLAUDE.md`, `AGENTS.md`, `AGENT.md` and `claude.md` (only names the directory actually lists). A `--since` value that is not `N days ago`, `N months ago`, `N year(s) ago` or `YYYY-MM-DD` is silently ignored. Full rules: `consolidation-llm.md` section 11.

Large repositories, such as those with years of git history, are pruned on the
client before they are posted. The remaining sources are then processed in
sequential LLM chunks, so that provider context limits are not exceeded. The CLI
logs `llm_chunks` in the dry-run output and in the final outcome.

**Caveat: LLM-fabricated detail.** A bootstrap run can produce pages that are
plausible but wrong. The LLM does not know your project. It infers from git history.
The wiki is under git version control, so the damage is recoverable. Review what
landed, for example with `docker exec ai-memory git -C /data/wiki diff HEAD~1`, and
revert the changes if they are wrong.

## Logs and read-only sandboxes

The CLI and the server write daily-rolling logs to `<data_dir>/logs/`, which by
default is `~/.local/share/ai-memory/logs/`. When that location is not writable,
for example in a sandbox that mounts `$HOME` read-only or as a throwaway tmpfs
(such as the `ai-jail` sandbox), ai-memory degrades instead of failing. It falls
back to the operating system's temporary directory. If that fails too, it falls
back to stderr-only logging. At each step it prints the exact path that failed.
Commands keep working either way. To keep durable file logs, and durable hook
spooling, inside a sandbox, map the data directory read-write, for example
`ai-jail --rw-map ~/.local/share/ai-memory ...`.
## Human password bootstrap and recovery

Human console login uses a username and a password. A bearer token is not pasted
into the browser. Machine APIs keep using `Authorization: Bearer`, with
`AI_MEMORY_AUTH_TOKEN` or a native `aim_` key. Before human auth is activated, the
deprecated browser compatibility path may exchange the root bearer, sent through
HTTP Basic, for an HttpOnly `ai_memory_auth` cookie. Activating human auth disables
that path immediately.

The credential classes stay isolated from one another:

- A password only issues a session.
- A session never authenticates `/mcp`, the hooks, or the workstreams.
- Recovery never issues a session.
- An API key never logs in through the login route.

### First-time root (greenfield)

Set a one-shot password, then start `serve`. On first start, the engine creates the
configured root user (the default root username is `root`, a confirmation point)
with a flag that requires a password change, and it marks the bootstrap as complete.
Later restarts ignore the value even if it stays in the environment. Unset it, so
that the plaintext is not kept in the process environment.

```bash
# At least 12 characters. It must not equal the root bearer, the actor-proxy bearer,
# or the recovery token, and it must not use an `ams_`, `aim_`, or `amk_` prefix
# (a confirmar: the prefix rule).
export AI_MEMORY_AUTH__INITIAL_ROOT_PASSWORD='choose-a-long-password'
docker compose up -d   # or: ai-memory serve
unset AI_MEMORY_AUTH__INITIAL_ROOT_PASSWORD
```

The first login must change that password, through the password route with CSRF
protection. If the password collides with an existing native API-credential hash,
startup fails closed, and bootstrap is not marked complete.

### Break-glass recovery

If the last human root is lost, set a high-entropy recovery secret of at least 32
characters. Then send a `POST` to the recovery route with a new password. Success
returns 204. It expires the legacy `ai_memory_auth` cookie, revokes that user's
sessions, and does **not** set a session cookie. Sign in again with the new password.

```bash
export AI_MEMORY_AUTH__RECOVERY_TOKEN='at-least-32-characters-of-entropy-here'
# restart serve so the process picks up the new value
curl -sS -D - -o /dev/null -X POST http://127.0.0.1:49374/auth/recovery \
  -H 'content-type: application/json' \
  -d '{"recovery_token":"'"$AI_MEMORY_AUTH__RECOVERY_TOKEN"'","new_password":"brand-new-pass!!","new_password_confirmation":"brand-new-pass!!"}'
```

To rotate, change the environment variable and restart. The previous token stops
working immediately. Public failures share one 401 response body: a wrong token,
recovery not configured, and a password that fails the policy all look the same.
The recovery token is not a bearer token, a cookie, or a login password.

When human mode is on (bootstrap completed, any password hash present, or initial
or recovery secrets configured), `serve` refuses to start unless a recoverable root
exists. A recoverable root has the root role, a password, and is not disabled.
Recovery must also be configured. Explicit machine-only remote deployments that use
`AI_MEMORY_AUTH_TOKEN` and no human secrets remain valid. A loopback deployment with
neither a bearer token nor human auth stays anonymous.

Addresses listed in `AI_MEMORY_AUTH__TRUSTED_PROXY_CIDRS` may supply an
`X-Forwarded-For` header for login rate limits. The header from untrusted peers is
ignored. When you serve behind HTTPS, set `AI_MEMORY_AUTH__SECURE_COOKIE=true`, as
described above.

## Operating without auth

For local-only, single-machine deployments, you can skip the bearer token:

```bash
docker run -d --name ai-memory \
    -p 127.0.0.1:49374:49374 \
    -v ai-memory-data:/data \
    <imagem-ai-memory>:latest
```

Notice the bind: `127.0.0.1:49374`, not `0.0.0.0:49374`. This pairing is critical.
**No bearer token and a loopback bind only** is the only safe combination. Outside a
container, the server refuses an unauthenticated LAN bind before it accepts
requests. `--allow-insecure-no-auth` overrides that refusal, but only for an
intentional, dangerous plain-HTTP deployment. Prefer `AI_MEMORY_AUTH_TOKEN` or a
loopback bind instead.

Inside a container, that refusal becomes a warning. The container must bind
`0.0.0.0` internally for `-p` to work at all, and it cannot see which host address
you published to. The `-p` option above therefore does the real work. It is what
keeps this container loopback-only. If you change it to publish on a LAN address,
set `AI_MEMORY_AUTH_TOKEN` as well.

Then wire up the agent CLI. Both commands default to no auth and to
`http://127.0.0.1:49374`, so no extra flags are needed for the local case:

```bash
ai-memory install-mcp   --client claude-code --apply
ai-memory install-hooks --agent  claude-code --apply
```

The installed container wrapper runs CLI commands inside a short-lived helper
container. For a local loopback server, it bridges that helper back to the host's
`127.0.0.1:49374` automatically. As a result, `ai-memory status`, `ai-memory search`,
and `ai-memory bootstrap` work with the same default URL as the generated agent
configuration.

### SELinux-enforcing hosts

On SELinux-enforcing Linux systems, such as Fedora, RHEL, and openSUSE, the usual
home-directory labels can stop the helper container from reaching the agent
configuration, even when its UID and GID match the host user. The wrapper checks two
things: the host's enforcement mode, and the security options that the container
engine reports. For the short-lived helper commands that touch host files
(`install-*`, `setup-agent`, `uninstall`, `backup`, `restore`, and `bootstrap`), it
adds `--security-opt label=disable`. Thin-client commands stay confined when they use
the named data volume and the implicit configuration. An explicit `--config` path, or
a valid host-backed data directory, also activates the host-file treatment. This
relaxes SELinux label confinement only for that trusted helper invocation. It does
not change the long-lived ai-memory server, which uses an engine-managed named volume.

`bootstrap` is on that list even though it only *reads* host files. An unmapped UID
and a confined label block reads just as effectively. The failure is misleading: the
command first logs `no .git found ...; bootstrapping from README/docs/rules only`,
and then fails with `Permission denied (os error 13)`.

The two engines report these facts under different keys. Docker answers
`docker info --format '{{.SecurityOptions}}'`. Podman has no such field and fails
that template, so the wrapper falls back to Podman's rootless and SELinux fields when
the Docker probe returns nothing. Rootless engines also need `-u 0:0`, because only
container UID 0 maps back to the invoking host user. On rootless Podman with SELinux
enforcing, both adjustments are required, and neither one alone lets the write
succeed.

Do not add the `:z` or `:Z` relabel suffix to the wrapper's whole `$HOME` bind mount.
Relabeling system directories such as `/home` can make the host unusable. Use
`label=disable` for the trusted helper invocation instead, as the wrapper does.
The `run`, `show`, `continue`, `resume`, `workstreams`, and `rename-workstream`
commands are the exceptions. The current wrapper intercepts them and starts a
cached, checksum-verified native client on the host. The host is where local
checkouts, harness executables, and session stores exist. The wrapper keeps an
explicit remote `AI_MEMORY_SERVER_URL`. If one of these commands logs
`data_dir=/data`, cannot find a checkout, or cannot find `codex`, `claude`, or
another host executable, refresh the stale wrapper with `ai-memory upgrade` on that
client machine.

### Docker compose alternative

If you prefer Compose, clone the repository and run:

```bash
docker compose -f docker/docker-compose.yml up -d
```

The bundled Compose file sets `restart: unless-stopped`, a healthcheck, and the
named volume. Agent setup is the same as for the regular container path.

---

## Keeping ai-memory up to date

The wrapper checks the image registry at most once every 24 hours. When a newer
image is available, it prints a one-line warning. Upgrade with:

```bash
ai-memory upgrade
```

The command downloads the wrapper and its SHA-256 checksum from the latest release.
It refuses an update that does not verify. It then pulls the latest container image,
and re-stages the hook scripts under `~/.local/share/ai-memory/hooks/<agent>/` for
the configured agents. Finally, it prints how to restart the server container, so
that the new binary is used. Re-running `install-hooks --apply` stays idempotent:
ai-memory replaces only the hook entries that it owns, and it leaves unrelated hooks
alone.

When a Compose file is found, the wrapper first verifies that its project owns the
running `ai-memory` container. A standalone container is never handed to an
unrelated Compose project just because its file sits at a conventional path. In that
case the wrapper writes the inspected standalone recreation script for review. The
script preserves the existing `/data` mount and the other runtime options.

The macOS menu bar app is not covered by `ai-memory upgrade`. Rebuild it with the
build script in the macOS companion directory of the source tree (see
[macOS menu bar app](#macos-menu-bar-app)), or replace `/Applications/AI Memory.app`
with a newer staged bundle. The wiki, the database, the configuration, and the models
stay in `~/Library/Application Support/ai-memory`.

Set `AI_MEMORY_NO_VERSION_CHECK=1` to silence the daily check. To pin wrapper
self-upgrades to a fork or a tagged release, set `AI_MEMORY_WRAPPER_URL=<url>`. The
wrapper then requires `<url>.sha256`, unless `AI_MEMORY_WRAPPER_SHA256_URL` names
the checksum URL explicitly.

When the upgraded server starts, it applies the database schema migrations and any
pending wiki-structure migrations automatically. Normal upgrades need no manual
database reset and no wiki rewrite. Migrations are forward-only. After a newer
version has applied its schema, an older binary refuses to open that data directory.
It fails closed rather than risk corruption (a confirmar). So take an
`ai-memory backup` before you upgrade, in case you need to roll back.

### Upgrading to 2.3.0

Version 2.3.0 is a normal forward upgrade. Its only new migration, `V64`, adds the
cross-project `agent_messages` table, and it does not alter or remove anything that
already exists. Two capture and usability conveniences are **on by default**. Both
are additive and non-destructive, but they are worth knowing about for your first
session after the upgrade:

- **The first `ai-memory run <harness>` installs that harness's hooks and MCP entry**
  if they are not already wired. This happens once per harness, and it is
  idempotent. It preserves your existing hook configuration, including a
  `--capture-assistant` opt-in. To disable it, use `ai-memory run --no-autowire`, or
  set `AI_MEMORY_RUN_AUTOWIRE=false`.
- **The first session in a brand-new, empty project imports that project's existing
  local harness history, once.** Installing ai-memory in the middle of a project
  therefore does not lose its context. The import runs only in an empty project, and
  it never touches a project that already has captured memory. It is also hard-capped
  (a confirmar: the cap). To disable it, set `AI_MEMORY_BACKFILL_ON_START=false`. To
  run it by hand, use `ai-memory backfill`.

If the server runs on another host, `ai-memory upgrade` refreshes only the local
wrapper, the local image, and the local hook scripts. Redeploy the remote server
separately, with the deploy script or with `docker compose pull && docker compose up -d`
in that deploy directory.

Inside a sandbox such as `ai-jail` or another bubblewrap sandbox, the wrapper can be
used from the sandbox. Run the `install-*` commands outside the sandbox, though,
because they write to `~/.local/share/ai-memory/hooks/`.

---

## See also

- [External companions and tests](external-companions-and-tests.md), section 5: macOS
  install paths, including the menu bar app, the native release archive, the source
  build, the container wrapper, and launchd.
- [Deploy guide](deploy.md): the homelab deployment walkthrough, covering the deploy
  script, TLS through a tunnel, and env-file management.
- [Usage guide](usage.md): handoffs, proactive querying, the web UI, the slim routing
  snippet with managed Agent Skills, migration from other memory tools, and raw-wiki
  inspection.
- [MCP client installation](mcp-install.md): the per-client MCP configuration
  reference for every client in the [support matrix](support-matrix.md).
- [Architecture](architecture.md): what is actually running inside ai-memory.
