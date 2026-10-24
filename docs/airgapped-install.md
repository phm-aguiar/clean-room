# Air-gapped / offline install

> **Escopo:** este documento contém partes que dependem de LLM ou de embeddings. Essas partes são opcionais e não fazem parte do escopo inicial, que funciona sem LLM. Podem ser removidas ou adiadas junto com as funções correspondentes.

Security reviews often ask for a documented offline install path before a
development tool is approved. ai-memory's build and runtime are already close
to offline-friendly by design. This page gathers that existing behaviour in
one place, with the exact conditions under which the program makes a network
call. It does not describe a separate offline mode.

## Build: no network required

- The build is self-contained. The database engine and the git library are
  embedded in the build, so no system libraries are needed beyond a standard
  C toolchain.
- The build requires the compiler toolchain version pinned in the repository
  (1.95). If the air-gapped host cannot download toolchains, install that
  version on a connected machine ahead of time. How to transfer the toolchain
  offline: a confirmar.
- A default build does not fetch anything over the network. The one exception
  is the maintainer mode that regenerates the web stylesheet. It is opt-in
  through a build variable and is not part of a normal build.
- Local embeddings are compiled in by default. A build with that feature
  disabled has no local embedding provider.
- Building from a vendored copy of the dependencies works the same way: a
  confirmar.

## Prebuilt binaries

Version tags publish prebuilt archives on GitHub Releases:

| Platform | Archive |
|---|---|
| Linux x86-64 | `ai-memory-linux-x86_64.tar.gz` |
| Linux ARM64 | `ai-memory-linux-aarch64.tar.gz` |
| macOS Apple silicon | `ai-memory-macos-aarch64.tar.gz` |
| macOS Intel | `ai-memory-macos-x86_64.tar.gz` |
| Windows x86-64 | `ai-memory-windows-x86_64.zip` |

Each archive has a companion file with the same name plus `.sha256`, in the
standard checksum-tool format. There are no prebuilt archives for other
platforms; build from source there.

Procedure:

1. On a connected machine, download the archive and its `.sha256` file.
2. Compute the SHA-256 of the archive and compare it with the published value.
3. Transfer the archive into the air-gapped environment and extract it.

Limit: the checksum is published in the same release as the archive. It
detects corruption and tampering in transit, but it does not protect against a
compromised release. The release process does not produce a separate
signature. If your policy requires one, plan for that gap.

## Runtime: no network calls unless configured

- The program has no telemetry, analytics, or crash reporting.
- Stored data (wiki pages and the database) lives in a single data directory
  on the local filesystem.
- The server, the command-line tool, and the lifecycle hooks connect to the
  server address in configuration. The default is the loopback address on the
  same host, so a single-host install makes no connection off the machine.
  Pointing them at a remote server sends their traffic there.
- Other outbound paths exist. Each one is either off by default or runs only
  when you explicitly request it:
  - **A cloud LLM provider** used for consolidation and other LLM features.
    Only when one is configured.
  - **A cloud embedding provider.** Only when configured. See the local
    embeddings section below for the offline alternative.
  - **LLM-based search reranking** (`AI_MEMORY_RERANKER=llm`). Off by default.
  - **Assistant final-turn capture.** Off by default. It requires enabling on
    the server and on the client. Once enabled, the captured text goes to an
    LLM provider only if one is configured.
  - **Admission webhooks.** Only to the URLs you configure.
  - **The upgrade command.** Only when you run it. See the updates section.
- One outbound call is on by default: on first start with local embeddings
  (the default provider), the server downloads the model files. See the local
  embeddings section.

The [threat model and data handling](threat-model-and-data-handling.md)
document lists what leaves the host for each path, including the caveats on
the sanitizer that runs before capture and reranking.

## Local embeddings without a network dependency

Local embeddings run in-process on the CPU. They need no API key, no
inference server, and no separate inference runtime to install.

- **Model:** all-MiniLM-L6-v2, 384 dimensions. Each input is truncated to 512
  tokens.
- **Selection:** set `embedding_provider = "local"` in configuration, or the
  `AI_MEMORY_EMBEDDING_PROVIDER` environment variable. If the setting is unset,
  `local` is used. `none`, `off`, or `disabled` opts out of vectors. Keyword,
  entity, and graph search keep working without vectors.
- **Files:** three files, about 87 MB in total, stored under
  `<data_dir>/models/all-MiniLM-L6-v2/`:
  - `model.safetensors` (model weights)
  - `tokenizer.json` (tokenizer)
  - `config.json` (model configuration)
- **Licence of the model:** a confirmar.

### Integrity checks

Each file is checked against a SHA-256 digest pinned in the program.

- **On download:** a file whose digest does not match is rejected and not
  installed. Files are written under a temporary name and renamed only after
  verification. If a download runs and finds an existing file with a wrong
  digest, it replaces that file.
- **On load:** every file is hashed before the model is loaded. A mismatch
  stops the load. An unverified file is never used.

### Download behaviour at server start

The download runs only when the provider is `local` and at least one file is
missing. Files that are already present and valid are skipped. Connection
attempts time out after 30 seconds, and a whole file transfer is capped at
600 seconds.

- **Provider unset (default):** the download runs in the background, and
  startup does not wait for it. That start runs without an embedder. Hybrid
  search is enabled on the next start. If the download or the load fails, a
  warning is logged and the server runs without vectors.
- **Provider set explicitly to `local`:** the download completes before startup
  continues. If it fails, startup fails with an error that points to the
  offline procedure. If the files are present but fail verification, startup
  also fails.

### Offline procedure

1. On a connected machine, download the three files from the public model hub,
   where the model is published under the same name.
2. Copy them to `<data_dir>/models/all-MiniLM-L6-v2/` on the air-gapped host,
   keeping the file names unchanged.
3. Set `embedding_provider = "local"` explicitly. The checksums are verified at
   load time, and no network access is needed.

On an air-gapped host, leaving the provider unset while the files are missing
causes a background download attempt on every start until the files are in
place. The attempt fails harmlessly, but to avoid it, either place the files
first or set the provider to `none`.

Cloud embedding providers cannot be used on an air-gapped host.

## Updates and patches

- **Default: manual.** Obtain the new release on a connected machine, verify
  its checksum, and transfer it in. This is the same procedure as the initial
  install.
- **The upgrade command** can download from a configurable release host, set
  with `release_base_url` in configuration or the `AI_MEMORY_RELEASE_BASE_URL`
  environment variable. This lets an internal mirror serve releases. The host
  must serve the archive at `<base>/download/<tag>/<archive>` and the checksum
  at the same path with `.sha256` appended. The command verifies the checksum
  before extracting the archive or replacing the binary. It needs network
  access to the host.
- Pass a version to pin the release tag. Without one, the command looks up the
  latest tag, using the GitHub API for the default host or a `latest/tag`
  endpoint on a custom host.
- The upgrade command supports only the platforms in the table above.

### The `upgrade` command in detail (native installs)

`ai-memory upgrade [--version <tag>] [--force]` refreshes a native
release-archive install. It is a client-only command: it never touches a
running or remote server.

**Refusals (before any download).** Each is an error with exit status non-zero:

- Inside a container: `refusing to self-upgrade inside a container; run
  `ai-memory upgrade` on the host Docker wrapper, or replace the image with
  `docker pull``.
- The canonical path of the running executable starts with a package-manager
  prefix (`/opt/homebrew/`, `/usr/local/Cellar/`,
  `/home/linuxbrew/.linuxbrew/`, `/usr/bin/`, `/bin/`, `/sbin/`, `/usr/sbin/`,
  `/nix/store/`; plain text prefix comparison, so no Windows path matches):
  `refusing to self-upgrade a package-managed install at <path>; use your
  package manager (Homebrew, AUR, apt, …) instead`.
- The executable's parent directory is not writable. The probe creates, with
  exclusive-create, a file named `.ai-memory-upgrade-write-probe` there and
  deletes it: `refusing to self-upgrade: <path> is not writable by this user;
  install under ~/.local (or another user-owned prefix) and re-run`. Only the
  directory must be writable (replacement is a rename).

**Steps.**

1. Base URL: `release_base_url` / `AI_MEMORY_RELEASE_BASE_URL`, trimmed; empty
   means the project's GitHub releases page.
2. Tag: `--version` (trimmed; a leading `v` is added when missing; empty is an
   error: `--version must be a non-empty release tag (e.g. v2.3.2)`). Without
   it: if the base URL contains the text `github.com`, `GET
   https://api.github.com/repos/<owner>/<repo>/releases/latest` and read
   `tag_name` (missing: `latest release JSON missing tag_name`); otherwise `GET
   <base>/latest/tag`, body trimmed, empty is an error (`latest tag endpoint
   returned empty body`).
3. If the tag and the running version are equal after removing a leading `v`,
   and `--force` is absent: print `already up to date (<version>)`, still run
   the plugin refresh (step 10), and exit successfully without downloading.
4. Archive name by OS/architecture: linux x86_64 and aarch64 `.tar.gz`, macOS
   aarch64 and x86_64 `.tar.gz`, windows x86_64 `.zip` (names in the table
   above); anything else (Windows on ARM) fails with `no GitHub release asset
   for this OS/arch`. Prints `→ downloading <archive> (<tag>)`.
5. HTTP: `GET <base>/download/<tag>/<archive>` and the same URL plus `.sha256`.
   Every request has a `User-Agent` of `<product>-cli/<version>`, a hard
   **120 s** total timeout, no cache and no retry; a non-2xx status fails with
   `GET <url> returned <status>`. Each body is capped at **128 MiB**: a
   `Content-Length` above the cap fails before reading, and the stream is also
   cut while reading, so a missing or lying length is covered.
6. Checksum, before extracting anything. The sidecar is UTF-8 text; blank lines
   and lines starting with `#` are skipped; the first remaining line is `<hash>`
   or `<hash> <name>` (whitespace separated; a leading `*` on the name is
   removed). The hash must be exactly 64 hex digits (`checksum line has invalid
   sha256 hash: <x>`), the name, when present, must equal the archive name
   (`checksum sidecar names <x>, expected <archive>`), and no usable line is
   `checksum sidecar contained no hash line`. Comparison ignores case; a
   mismatch is `release archive checksum mismatch (expected <e>, got <a>)`; on
   success prints `  ✓ checksum ok`. The standard checksum-tool format
   `<hex>  <name>` is accepted.
7. Extraction into a temporary directory that is discarded at the end. Only
   `.tar.gz`/`.tgz` and `.zip`. Limits: at most **4 096** entries and **512
   MiB** of declared expanded size (sum over entries). Every entry path
   loses its `.` components and may then contain only normal components (no
   `..`, root or drive prefix: `release archive contains unsafe path: <p>`);
   symbolic and hard links fail (`unsupported link entry`), as does any entry
   that is neither file nor directory (`unsupported entry type`). Allowlist:
   the files `ai-memory` / `ai-memory.exe` at the root and everything under
   `hooks/` are extracted; a fixed list of support entries is validated but not
   extracted (`LICENSE`, `README.md`, `docs/install.md`, `docs/macos.md`,
   `docs/windows.md`, `crates/ai-memory-cli/templates/config.default.toml`,
   `packaging/env/ai-memory.env`,
   `packaging/launchd/com.github.akitaonrails.ai-memory.plist`,
   `packaging/systemd/ai-memory-user.service`,
   `packaging/systemd/ai-memory.service`, `packaging/sysusers/ai-memory.conf`,
   `packaging/tmpfiles/ai-memory.conf`, and the directories leading to them).
   Anything else: `release archive contains unexpected path: <p>`. The
   archive root entry `./` is accepted only as a directory. Tar permissions
   are not preserved. After extraction the binary must exist: `release archive
   is missing the <name> binary`. (A new product's archive may ship a
   different support list; the allowlist must then be updated with it.)
8. Replace the executable. Print `→ replacing <path>`. Copy the new binary to
   the executable path with its extension replaced by `new` (`ai-memory.exe` →
   `ai-memory.new`; stale file removed first). Unix: mode 0755, then rename over
   the executable. Windows (cannot overwrite a running image, can rename it):
   remove a stale `.old` (same extension rule), rename the running executable to
   `.old`, rename `.new` into place; if that last rename fails, try to rename
   `.old` back; finally delete `.old` best-effort (often fails while the
   process still maps it, and then stays). Failure messages mention the
   leftover `.new`/`.old`.
9. Sibling `hooks` directory: only when the archive has `hooks/` **and** a
   `hooks` directory already exists beside the executable. Prints `→ refreshing
   <dir>`; copies into `.hooks.new`, moves the old one to `.hooks.old`, renames
   the new one into place (restoring the old one if that rename fails) and
   deletes `.hooks.old` best-effort.
10. Re-apply staged hooks. The staged root is `<data dir>/hooks`. Missing:
    `→ no staged hook scripts found at <dir>` and `(nothing to refresh —
    install-hooks hasn't been run with --apply yet)`. Each subdirectory is an
    agent, except `lib` and names starting with `_`; an unknown name prints
    `(skipping unknown staged agent dir `<name>`)`. For each agent in
    alphabetical order it prints `    ai-memory install-hooks --agent <name>
    --apply` and runs it with global scope and no explicit server URL or token
    (reusing what was configured); a failure of one agent prints `(skipped —
    <error>; re-run with the same --server-url / --auth-token used originally)`
    and the loop continues. Claude Code special case: if the user-level
    settings file is missing or holds no ai-memory hooks, the user-level
    re-apply is skipped (so an upgrade never creates a user-level install the
    operator never asked for) and only the staged scripts are refreshed, with a
    note about `--scope project` installs. Marketplace plugins, when installed,
    are refreshed (Claude Code) or announced (Codex).
11. If `AI_MEMORY_SERVER_URL` was set by the operator and its text contains
    `127.0.0.1`, `localhost`, `[::1]` or `0.0.0.0` it is considered local;
    otherwise stderr gets `note: AI_MEMORY_SERVER_URL=<url> looks remote — this
    upgrade refreshed only the local binary and hooks; redeploy the remote
    server separately`.
12. Final line: `✓ upgraded to <tag>`.

The running server keeps executing the old image until restarted.

## What still needs a decision from you

- **Git remote sync.** The program does not push the wiki repository to any
  remote. If you set up a push or sync, securing that channel is your
  responsibility. Examples are SSH keys and access controls on the remote.
- **Update delivery.** See the updates section above.
- **Webhooks and external tools.** Any admission webhook or external companion
  tool you add has its own network path, which you need to review.

## Related documents

- [Threat model and data handling](threat-model-and-data-handling.md): what
  leaves the host, and when.
- [Local embeddings](local-embeddings.md): provider choices, including the
  offline path in more detail.
- [Deploy](deploy.md): container and homelab deployment.
- [Security](security.md): loopback default and authentication.
