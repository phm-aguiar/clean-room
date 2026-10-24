import sys
sys.path.insert(0, '.audit006-scratch')
from ed import apply

# ---------------- design: fixed sizes in Appendix B
apply('docs/design-cross-project-profile.md', [
(r'''Byte counts for the budget use these exact strings.''',
r'''Byte counts for the budget use these exact strings. Fixed sizes, usable as unit-test checks: the personal head is 736 bytes and the team head 797; the tail (closing fence line plus footer) is 226 bytes, to which the baseline hint adds 192 (personal) or 195 (team); the omitted line for one entry is 58 bytes. A personal digest with the baseline hint therefore has 1,154 fixed bytes, and a non-baseline one 962, so the smallest budget of 1,500 leaves 538 bytes for entry lines.'''),
])

# ---------------- session brief
apply('docs/session-brief.md', [
(r'''Budget: `briefing_budget` trimmed and parsed as a non-negative integer; absent or unparsable → **4000**;''',
r'''Budget: `briefing_budget` trimmed and parsed as a non-negative integer (a leading `+` is accepted; a minus sign, a decimal point, thousands separators and a value above the largest 64-bit unsigned integer are unparsable); absent or unparsable → **4000**;'''),
(r'''The directory keeps the 512 most recent files (it is shared with the profile-digest markers).''',
r'''The directory keeps the 512 most recent regular files: after the new marker is written, the other files are ordered by modification time, newest first (ties by name, descending), and every one past the 511th is deleted. Brief markers and the profile-digest markers (named `profile-<key>`) are counted together. The marker is an empty file, and a failure to write or prune it is ignored (the worst case is that the brief or the digest is requested again on the next prompt). A session whose key starts with `profile-` has a brief marker named like another session's profile-digest marker; the collision is negligible and unguarded.'''),
(r'''- A project whose marker does not opt in never produces the marker file and never asks for the brief.''',
r'''- A project whose marker does not opt in never produces the marker file and never asks for the brief.

## 8. Fixed sizes and invariants (from the reference tests)

Fixed sizes of the exact texts of section 5: the part before the first page section (title line, security line with the notice, blank line, start fence line) is 475 bytes; `tail` is 363 bytes; the unavoidable scaffold is therefore 838 bytes, and the budget floor of 1,500 leaves 662 bytes for content.

For any budget in [1500, 20000] and any set of pages, hostile text included (fence markers, `<!--`, CRLF, emoji, metadata fields, empty bodies, bodies of thousands of words), a rendering satisfies:

1. its length never exceeds the budget;
2. the security notice is present;
3. exactly one start fence and one end fence appear;
4. it ends with the footer;
5. it is deterministic (same input, same bytes);
6. the pages shown are a prefix of the core list, and every core page is accounted for exactly once, shown or covered by the omitted section (a listed path, the `…and N more` line, or the bare count);
7. when nothing was crowded out and recent pages exist, the recent section is present;
8. the cost does not grow with the size of bodies that are not shown (24 pages of 1 MB each render in well under two seconds).

Reference scenarios: a short page (`Use nextest for every test run in CI.` as the whole body of `_rules/short.md`) appears exactly once, not as an excerpt plus a body; a fence marker inside an `abstract` is escaped (`quoted &lt;!-- ai-memory:untrusted-history:end --&gt; then text`); a project with the page `_rules/style.md` whose body is `always use the single writer actor` gets a brief that contains `project brief`, that body, the end fence and, after the fence, the agent footer; a non-truthy `briefing` value (`false`) yields nothing when no handoff is pending, and with a pending handoff the handoff precedes the brief.'''),
])

# ---------------- marker file
apply('docs/marker-file.md', [
(r'''| `max_chars` | `[briefing]` | integer | 4000 | Character budget for the whole brief. Clamped to 1500 through 20000. |''',
r'''| `max_chars` | `[briefing]` | integer | 4000 | Budget for the whole brief, in UTF-8 bytes despite the key name. Clamped to 1500 through 20000. |'''),
(r'''Whether this project contributes to the cross-project profile. See [cross-project-profile.md](cross-project-profile.md). |''',
r'''Whether this project contributes to the cross-project profile. See [cross-project-profile.md](cross-project-profile.md). Whenever a marker resolves, the client sends `0` for a falsy value and `1` for anything else, a missing key included, so removing the key turns it back on. |'''),
(r'''Where the server enforces the pattern is **a confirmar**.''',
r'''Resolved on 2026-10-24: the pattern is enforced only for the home marker's route names (`route_workspace` and `route_project`, at most 512 bytes, the first character a lowercase letter or digit), by the client (the CLI and the generated plugins). The server does not apply it when a hook or a local marker names a workspace or a project: the name is used as sent and created on first use, and matching is case-sensitive. The only server-side name checks are those of the rename routes (see [lifecycle-ops.md](lifecycle-ops.md)) and the reserved profile names.'''),
(r'''Explicit scope arguments override this default **a confirmar**.''',
r'''Explicit scope arguments override this default (confirmed on 2026-10-24: a non-blank `workspace` or `project` argument wins, and for `memory_query` so do a non-empty `scopes` list and `global = true`; `memory_recent` looks only at `workspace` and `project`).'''),
(r'''The order used by lifecycle hooks after step 2 is **a confirmar**.''',
r'''The server resolves a lifecycle event or a session-start fetch in this order (confirmed on 2026-10-24; the client has already put the marker's `workspace` and `project`, the strategy, the repository identity and the aliases on the request). (a) The workspace is the marker's, else `default`. (b) When the marker declares aliases, they must match a stored project that carries the same repository identity, otherwise the request fails; the alias match is the project. (c) Otherwise, when a repository identity of a kind that routes by identity was sent (it is ignored for a marker `project` that has no aliases), the project already carrying that identity wins whatever its name; if none does, the candidate is claimed for it: the project whose stored repository path is the longest prefix of the cwd, else the project named by the next step. (d) Otherwise, when no `project` was sent, a project whose stored repository path is the longest prefix of the cwd wins over a differently named derived name, so a deep working directory never fragments a project. (e) Otherwise the named project is fetched or created: the marker's `project` if sent, else the name derived from the cwd (`repo-root`: the name of the main repository root; `basename`: the last component of the cwd). A name that is `_global`, `_profile` or starts with `_profile.` is replaced by the server's default project. For an event of an existing session, session-sticky routing (below) is tried before all of this.'''),
(r'''Whether this applies in every host is **a confirmar**.''',
r'''Confirmed on 2026-10-24: the decision is made by the server from the strategy that the client sends (`project_strategy=repo-root`), so it applies to every host whose hook forwards the strategy, native hooks and generated plugins alike. A client that does not forward it is treated as `basename`: an out-of-tree event is then resolved on its own, while in-tree stickiness still applies. In both cases the session's own directory must be a meaningful anchor (not `/` and not `$HOME`).'''),
(r'''so allowlist mode does not gate them **a confirmar**.''',
r'''so allowlist mode does not gate them. Confirmed on 2026-10-24: when `install-hooks --apply` writes script hooks under allowlist mode it prints `WARNING: this install uses script hooks, which POST directly and cannot enforce the mode. Allowlist is stored but NOT in force here.`; `setup-agent` passes no capture mode at all; [install.md](install.md) states the same for the Docker host wrapper and the `setup-agent` snippets.'''),
(r'''Legacy script hooks and remote-only or container script bundles do not enforce the policy **a confirmar**.''',
r'''Legacy script hooks and remote-only or container script bundles do not enforce the policy (confirmed on 2026-10-24 against [install.md](install.md), "Capture exclusions": only native `ai-memory hook` commands and the generated OpenCode, OMP, Pi and OpenClaw integrations enforce it, and the installer's capability output shows which integration was selected).'''),
])

# ---------------- user guide
apply('docs/cross-project-profile.md', [
(r'''- hourly otherwise.''',
r'''- hourly otherwise.

Only the HTTP server (`serve` with the HTTP transport) runs these passes; the stdio transport runs none. A signal that arrives while the 30-second wait is running causes one further pass after the current one, and stopping the server waits at most five seconds for a running pass.'''),
(r'''`apply` runs only when you run it: the profile itself never edits a rules file.''',
r'''Caveats worth knowing: the rules file must be valid UTF-8 (a UTF-16 file is refused and left untouched); on Windows a brief file lock from an antivirus or indexer is retried for about 150 ms before the write fails; a start delimiter without its end delimiter is not a block, so keep the two together; and the backup folder inherits the permissions of the data directory on Windows.

`apply` runs only when you run it: the profile itself never edits a rules file.'''),
])

# ---------------- prompts doc
apply('docs/prompts-schemas-and-sanitizer.md', [
(r'''- `changed` falso: a entrada permanece como foi colhida.''',
r'''- `changed` falso: com entrada armazenada e sem inversão de negação, o texto armazenado (declaração, raciocínio e etiquetas) permanece; com inversão (por exemplo "always" contra "never" sobre as mesmas palavras), ou sem entrada armazenada, vale a declaração mais recente colhida. As regras exatas estão em `design-cross-project-profile.md` §6.'''),
(r'''- `applies_to` é filtrado para as etiquetas conhecidas.''',
r'''- `applies_to` é filtrado para as etiquetas conhecidas. Uma lista vazia significa "sem etiquetas" e substitui a derivação por vocabulário do Apêndice A.7 do design.'''),
])
