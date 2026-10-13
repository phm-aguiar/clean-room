# Day-to-day usage

> **Escopo:** este documento contém partes que dependem de LLM ou de embeddings. Essas partes são opcionais e não fazem parte do escopo inicial, que funciona sem LLM. Podem ser removidas ou adiadas junto com as funções correspondentes.

This page covers what happens after ai-memory is installed: handoffs,
compaction recovery, proactive memory queries, the web UI, the managed
routing snippet and Agent Skills package, and routine maintenance tasks.
For installation on Windows, see [windows.md](windows.md). For copy-paste recipes, see
[cookbook.md](cookbook.md).

## Capture exclusions

To keep recognized file-tool events under private paths out of ai-memory
before they are spooled or sent, configure `[capture] ignore_paths` in the
nearest `.ai-memory.toml`. The grammar, limitations, support matrix, refresh
requirements, and the local, side-effect-free `--check-capture` option of the
`hook` command are described in
[the marker-file reference](marker-file.md#capture-exclusions). This is not a
general prompt or output data-loss filter.

## Cross-agent handoff

You normally do not create handoffs by hand. With lifecycle hooks installed,
session-end capture writes the handoff and the next session-start hook fetches
it. Selection follows these rules:

- Manual handoffs are project-wide and take precedence over automatic
  session-end handoffs.
- Among automatic handoffs that match the receiving directory by path
  boundary, the newest one is delivered. When two have the same creation time,
  the one with the most specific directory wins.
- Creating a new automatic handoff expires earlier open automatic handoffs
  from the exact same directory and owner, provided their source session has
  already ended. A still-running session's earlier handoff is left alone unless
  it is that same session.
- Accepting an automatic handoff expires older matching automatic handoffs. It
  never disturbs manual handoffs or pending work from sibling directories.

```text
$ claude
> Working on the auth refactor. JWT rotation is broken; trying session cookies.
[work for an hour]
> /exit

$ codex   # in the same directory, later
[SessionStart hook fetches the handoff; Codex sees it before your prompt.]
> Picking up: you were investigating session cookies as an alternative...
```

If an agent has MCP but no lifecycle hook surface, ask it to call
`memory_handoff_begin` before quitting. The next hooked agent can still
consume that handoff automatically. Clients without a hook output channel (for
example Grok or Zero) should call `memory_handoff_list` on resume, then
`memory_handoff_accept` with the listed `handoff_id`. Listing does not claim
the row.

On a server that distinguishes operators, handoffs belong to their creator by
default: the next session sees their own handoffs plus those deliberately
shared, never a teammate's. Use `shared: true` on `memory_handoff_begin` only
when the baton is meant for anyone in the project. Root-authorized recovery can
pass `any_owner: true` to `memory_handoff_accept` or `memory_handoff_cancel`;
normal callers cannot use that switch.

Handoffs are next-session transfer, not a live message bus between agents that
are still running. Antigravity CLI exposes a `PreInvocation` event before every
model call, but ai-memory fetches a handoff only on invocation zero, which is
the startup boundary. A handoff created later in that conversation stays open
instead of being consumed by its creator's next model call.

If an agent creates a handoff by mistake, cancel it immediately with
`memory_handoff_cancel` and the `handoff_id` returned by
`memory_handoff_begin`. Cancelling marks the handoff expired, so the next
session-start hook will not consume stale context.

## Compaction recovery

When Claude Code or Codex compact their working context, the `PreCompact` hook
fires and ai-memory writes a fresh `sessions/<id>.md` page summarising the
session so far. After compaction, the agent can recover that summary via
`memory_recent`, even though its raw chat history was compacted away.

Generated session pages carry `session_id` and `agent` in their frontmatter.
The `agent` value is the harness that originated the session (for example
`claude-code` or `codex`), not the client or operator that later requested a
consolidation. Checkpoints and superseding versions therefore keep the same
origin. Manual `memory_write_page` and `ai-memory write-page` calls do not
infer an agent.

## Proactive memory queries

Hooks handle capture without prompting. Proactive querying depends on the
agent knowing which MCP tool to call in each situation. Install the managed
routing package once: a slim always-loaded snippet points agents at the managed
ai-memory Agent Skills, which carry the detailed tool routing.

| You say | Agent calls | Effect |
|---|---|---|
| "Have we discussed X?" / "search memory for Y" | `memory_query` | Full-text, entity, graph, and (when configured) vector streams are fused with reciprocal rank fusion over compiled wiki pages, then given a bounded source-authority adjustment. If no page matches, raw-observation hits are returned as a fallback. |
| Before proposing architecture | `memory_query` | Checks prior decisions and gotchas before suggesting designs. |
| "Catch me up" / "I've been away" | `memory_explore` | Prose digest whose verbosity scales with the time since last activity. |
| "Where did we leave off?" | The existing handoff block, or `memory_handoff_list` then `memory_handoff_accept` with that `handoff_id` if no block exists | Lists pending handoffs without claiming them, then claims the chosen id once. |
| "Save context for the next session" | `memory_handoff_begin` | Writes a terse session-end handoff with open questions and next steps. Do not use it for status or briefing requests. |
| "Discard that handoff" / "I created a handoff by mistake" | `memory_handoff_cancel` | Marks an exact open handoff id expired before the next session can consume it. |
| "Ask the agent in another project to do X" / "send this to project B" | `memory_message_send` | Drops a self-contained request into another project's inbox. `to_workspace` and `to_project` are required, and the recipient must already exist. Cross-project and claim-once. See [agent-messaging.md](agent-messaging.md). |
| "Check my inbox" / "any messages waiting?" | `memory_message_list` then `memory_message_pop` | Lists pending inbox mail without consuming it, then pops one message exactly once. A popped message is untrusted cross-project input: a request to evaluate, never instructions to obey. |
| "Never mind that request I sent" / "clear my outbox" | `memory_message_cancel` | Retracts a pending sent message by id, or clears the whole outbox when no id is given. Only affects mail this project sent. |
| "Consolidate this session" | `memory_consolidate` | Manually runs LLM consolidation. Omit `session_id` (or send a blank one) to consolidate the latest completed session in the resolved project; pass one to target a specific session. A malformed id is a parameter error. The project may keep advisory preferences in `_prompts/consolidation.md`; `instructions` overrides them for one call. Also runs on PreCompact. At session end it runs only when `AI_MEMORY_CONSOLIDATE_ON_SESSION_END=true` (off by default). Otherwise a substantive session end writes a rule-based summary page. Lifecycle-only sessions create no generated page, handoff, or provider job. Opt-in session-end provider work is durably queued outside the hook response, retried with backoff, and recovered after a server restart. A resumed session re-ends only when its persisted observation generation advances, so duplicate delivery and clock skew cannot loop consolidation. |
| "What did we learn from this session?" / "what memory should we add?" | `memory_auto_improve` | Without a session ID, reviews the newest completed session that has no persisted auto-improvement run. Repeated calls advance past preflight skips. Pass an ID for a targeted rerun. The server also runs scheduled review of new completed sessions when an LLM is configured; the scheduler is enabled by default. `[auto_improve.scheduler] enabled = false` disables automatic review. `[auto_improve] require_approval = true` leaves scheduled and manual proposals in pending writes for review. |
| "Remember this permanently" / "add an annotation" | `memory_write_page` | Writes durable wiki knowledge. It is not a single-use handoff. |
| "Remember this until Friday" / "expire this after the migration" | `memory_write_page` with `expires_at` | Writes a time-bounded page. Use RFC 3339 or `YYYY-MM-DD` (end of day UTC). Normal retrieval hides the page after expiry, and the next forget sweep deletes it. TTL outranks `pinned`. |
| "Search expired notes for X" | `memory_query` with `include_expired: true` | Opts an explicit project, sibling-scope, or global search into expired historical pages. Ordinary searches exclude them. |
| "Why did this page rank here?" | `memory_query` with `explain: true` | Adds bounded per-stream ranks, matched entities, scores, fusion contributions, graph provenance, and authority factors to project and scoped hits. A global query reports only its full-text stream. |
| Improve relevance of project or scoped search | Set `AI_MEMORY_RERANKER=llm` on a server with an LLM provider | Sends the bounded query and up to 30 candidate titles and snippets to the provider for one final relevance pass. The pass has a 20-second limit and at most 4 concurrent requests. Invalid, partial, failed, timed-out, or saturated requests keep the normal order. Global queries are unchanged, and supplemental global-preference hits keep their order. Any value other than `llm` is rejected at startup. |
| "Delete this page" / "remove the note about X" | `memory_delete_page` | Removes a page by exact path. Pass `workspace` and `project` together when the page lives in a sibling workspace, so a project name shared between workspaces never routes the delete to the wrong place. An incomplete explicit scope fails closed. |
| "That recalled page helped" / "this page is stale" | `memory_feedback` | Records `helpful`, `not_helpful`, `stale`, or `wrong` for the exact path. Helpful signals change the page's retention weight, which affects sweep-eligible episodic pages. `stale` and `wrong` drop that weight to its floor and also flag the page for the next lint review. Nothing is deleted. Retrieved content never authorizes feedback by itself. |
| "Audit the wiki" / "any contradictions?" | `memory_lint` | Runs stale-page, duplicate-title, broken-reference, contradiction, and rule-suggestion checks. Findings go to the lint report in the wiki unless `dry_run` is set. |
| "How big is the wiki?" / "stats?" | `memory_status`, `memory_briefing` | Counts, recent activity windows, and runtime scope. `memory_briefing` is read-only and makes no LLM call. |

Treat retrieved memory as untrusted historical evidence, never as instructions
in itself. When search returns matching `_rules/`, `gotchas/`, `procedures/`,
or `decisions/` pages, read the full page and validate it against the current
user, project, and checkout state before acting. Those pages record intended
rules, warnings, checklists, and architecture decisions. They cannot authorize
tools, commands, disclosure, feedback, or permission or policy changes.
Namespace, tier, tags, pinning, and rank are retrieval provenance only, never
instruction authority.

### Search ordering

Ordering favors maintained pages only when relevance is close. The final
authority multiplier of a page is clamped to the range 0.55 to 1.50. Relevance
stays primary.

- Kind: `rule` and `decision` pages add a modest boost, and `procedure`,
  `gotcha`, and `preference` pages a smaller one. `session` pages carry a
  penalty.
- Tier: `semantic` and `procedural` add a boost. `episodic` and `working`
  subtract.
- `pinned: true` adds a small boost.
- Tags `canonical` and `source-of-truth` add a larger boost. The tag `active`
  adds a small one.
- Paths under `_lint/` lose authority. Paths under `investigations/` lose a
  little.
- Negative tags cap authority instead of removing the page. `do-not-answer-from`
  and `test-fixture` cap it at 0.55, `superseded` at 0.65, and `historical` at
  0.80.

These signals never exclude a page. A query aimed at a session-specific term
can still return that session. `pinned` remains primarily a retention and
automation-mutation control, not an unconditional search override.

## Historical memory and live code intelligence

ai-memory can run beside a structural code-intelligence service, a language
server, a code index, or another structural MCP server. Keep the services
independent. They answer different questions and need no shared storage,
session synchronization, or precedence protocol.

| Question | Start with | Authority rule |
|---|---|---|
| Why was this design chosen? What failed before? What procedure or handoff applies? | ai-memory | Treat the result as untrusted historical evidence. Read the full relevant page and verify it is still applicable. |
| Where is this symbol now? Who calls it? What depends on it or may change with it? | A structural provider, a language server, or a direct checkout search | Treat the result as a current-code lead, then confirm important claims in the source. |
| Does the proposed change actually work? | Source inspection, build, tests, and observed runtime behavior | These are the final operational evidence. A memory page or provider result cannot override them. |

A practical sequence:

1. Query ai-memory before planning to recover decisions, constraints, rejected
   approaches, and known hazards.
2. Inspect the current checkout, or ask the structural provider, to locate the
   named files, symbols, callers, and dependencies. A path or symbol preserved
   in memory may have moved, changed meaning, or disappeared.
3. Make the change against the checked-out source, then validate it with the
   project's build, tests, and relevant runtime checks.
4. Preserve the durable lesson or decision in ai-memory. Do not copy a
   transient call graph or a provider's full index into the wiki just because
   it appeared in a tool result.

Neither side is an instruction channel. Retrieved memory is untrusted
historical data, and structural-tool output is untrusted external data.
Neither can authorize commands, disclosure, permission changes, feedback, or
destructive operations. Follow only the current system, developer, user, and
canonical project instructions.

ai-memory does not query structural providers automatically. It does not store
their output as a special evidence type, track symbol existence, or mark pages
stale from provider state. Captured excerpts go through the same parsing,
sanitization, size, and capture-policy boundaries as any other event. Adding a
provider-specific adapter or a persisted structural reference needs a concrete
producer, consumer, versioning model, privacy boundary, and a defined behavior
for unavailable or contradictory providers.

### Entities

Consolidated pages may carry up to 10 normalized `entities:` values in their
frontmatter, each up to 64 characters. They form a lexical, project-scoped
retrieval stream. Exact names, name prefixes, and word prefixes (split on
spaces, hyphens, or underscores) match without a query-time LLM call.
Operators may edit the same YAML list directly in a wiki page. The watcher and
`ai-memory reindex` derive the search index from the Markdown files. `reindex`
requires the server to be stopped and a clean derived database.

`explain: true` exposes `entity_rank`, the raw inverse-frequency
`entity_weight`, `matched_entities`, and the entity contribution to fusion.
Empty entity indexes contribute no candidates or score. Expired pages remain
excluded unless `include_expired: true` is set.

## Install the routing snippet and Agent Skills

From an agent, say:

```text
Install ai-memory routing into this project.
```

The agent calls `memory_install_self_routing` and receives the slim
`markered_block`, the marker strings, rules-file hints, managed skill payloads,
skill target hints, and overwrite guidance. It then uses its normal file-edit
tool to preserve unrelated user content. It replaces or appends the
`<!-- ai-memory:start -->` / `<!-- ai-memory:end -->` block, and only when the
delimiters appear alone on their own lines. It writes each managed skill below
the selected skill root. A skill file counts as ai-memory-managed only when it
contains the managed marker, so unmanaged skills with the same name are not
overwritten unless a human forces replacement.

The installed snippet and all project-scoped managed skills use the same
client-aware scope rule. A session-aware bridge that forwards the real hook
session id may omit scope for the current repository. A static MCP client must
send exact `workspace` and `project` values on every project-scoped call, even
when lifecycle hooks are installed, because hooks alone do not identify the MCP
request. Declare both names in the nearest `.ai-memory.toml`, or take them from
the operator or server configuration. Never infer them from a directory name or
rely on the server's last active project. Global query and global preference
modes still omit project scope arguments.

From a terminal:

```bash
ai-memory install-instructions
ai-memory install-instructions --target AGENTS.md
ai-memory install-instructions --print
ai-memory install-instructions --no-skills
```

`install-instructions` installs or updates managed skills by default. Use
`--no-skills` only when you intentionally want a snippet-only refresh. The
command replaces only the markered ai-memory block, preserves unrelated
content, and writes a backup next to an existing instruction file before
changing it. `install-instructions --print` previews the instruction snippet
only. Use `install-skills --print` to preview skill payloads. The skill flags
mirror `install-skills` with a `--skills-` prefix: `--skills-scope
project|global`, `--skills-agent
claude-code|agents|devin|grok|hermes|both`, `--skills-target-dir <dir>`, and
`--skills-force`.

Instruction target selection:

- `--target <path>` uses exactly that file.
- Without `--target`, the command extends `CLAUDE.md` if it exists, `AGENTS.md`
  if it exists, or both when both exist. If neither exists, it creates
  `CLAUDE.md` and prints a hint about `--target AGENTS.md`.

The skill agent follows the instruction target unless you override it. An
instruction target named `CLAUDE.md` selects the Claude-compatible skill root.
An `AGENTS.md` target selects the cross-client root. Both files select both.
Any other or missing target falls back to the Claude-compatible root. Use
`--target AGENTS.md` for projects that are not Claude-only. For Grok Build CLI,
select `--skills-agent grok` so skills install under its own root.

When a project keeps `AGENTS.md` as its canonical instruction file, give it a
`CLAUDE.md` whose first line is a bare `@AGENTS.md` import. Claude Code loads
`CLAUDE.md` (a confirmar: whether it also reads `AGENTS.md` on its own is
external tool behavior). A prose pointer that says "read AGENTS.md" only asks
the agent to open the file, so adherence depends on the agent doing so.

To refresh only the managed Agent Skills:

```bash
ai-memory install-skills
ai-memory install-skills --scope global --agent agents
ai-memory install-skills --scope global --agent devin
ai-memory install-skills --scope global --agent grok
ai-memory install-skills --scope global --agent hermes
ai-memory install-skills --agent both --print
ai-memory install-skills --target-dir .custom/skills --force
```

Skill roots by agent and scope:

- Project-local roots: `.claude/skills` for Claude-compatible installs,
  `.agents/skills` for cross-client installs, `.devin/skills` for Devin,
  `.grok/skills` for Grok, and `.hermes/skills` for Hermes.
- Global Claude-compatible skills go under `$CLAUDE_CONFIG_DIR/skills` when that
  variable is set. Otherwise they go under `~/.claude/skills`. Global agents
  skills go under `~/.agents/skills`.
- Global Devin skills go under `%APPDATA%\devin\skills` on Windows and
  `~/.devin/skills` on other systems.
- Global Grok skills go under `$GROK_HOME/skills`, which defaults to
  `~/.grok/skills` when `GROK_HOME` is not set.
- Global Hermes skills go under `~/.hermes/skills`.
- `--target-dir` points at an explicit skill root and bypasses scope and agent
  inference. `--print` previews target paths and skill contents. `--force`
  allows replacement of unmanaged skills with the same name. Without it,
  user-authored skills are preserved. Uninstall removes ai-memory-managed skills
  from the default project and global roots after checking the marker. Custom
  `--target-dir` roots need manual cleanup.

This is prompt packaging only. ai-memory does not run a runtime skill router,
does not store durable memory in `SKILL.md` files, and does not turn the
auto-improvement loop into a skill-authoring system. Durable knowledge still
lives in the wiki.

## Bootstrap an existing project

If you install ai-memory into a project that already has months of history,
the wiki starts empty. `ai-memory bootstrap` seeds it from the existing repo
history and documentation.

```bash
export AI_MEMORY_SERVER_URL="http://localhost:49374"
ai-memory bootstrap --dry-run
ai-memory bootstrap
```

The bootstrap collector reads the git log, the root README, the `docs/`
directory, project rule files (`CLAUDE.md`, `AGENTS.md`, and similar), and the
module-level documentation comments in the source tree. It then sends the
selected sources to the running server. The server must have an LLM provider
configured. If the server has auth enabled, set `AI_MEMORY_AUTH_TOKEN` as well.
By default the input is capped at 150,000 tokens (estimated as characters
divided by 4). Lower-priority sources are dropped first when over budget.
Larger bundles are split into sequential chunks of about 24,000 input tokens.

## Migrate from another memory tool

When replacing an existing memory system, treat the old data as untrusted
historical input until you curate it. Do not pipe raw transcripts or old memory
stores directly into ai-memory.

Migration checklist:

1. Export the old memory or history before changing hooks.
2. Keep the raw export as an archive, not as current project truth.
3. Scrub secrets, tokens, credentials, API keys, and raw logs that should not
   become durable memory.
4. Curate the useful material into reviewed Markdown pages under a temporary
   docs directory, or directly into `concepts/`, `decisions/`, `gotchas/`,
   `procedures/`, `notes/`, or `_rules/`.
5. If this checkout might be ambiguous, add `.ai-memory.toml` to pin the
   intended workspace and project before importing or installing hooks.
6. Start `ai-memory serve` locally and confirm `ai-memory status` can reach the
   server before touching existing client configs.
7. Import curated material first. Avoid importing the full legacy raw history.
8. Verify expected pages with a full `memory_query`. Use `ai-memory search` only
   when a terminal full-text lookup is enough.
9. Configure MCP and lifecycle hooks for one client at a time.
10. Only after ai-memory capture and retrieval work, disable the old memory
    hooks, plugins, or MCP servers.
11. Search each client config for stale references to the old tool. Remove stale
    `Authorization` headers or environment variables if bearer auth changed.
12. Restart each agent CLI after changing hooks, plugins, or MCP config.

Client cleanup hints:

- Claude Code: check plugins, hooks, old SessionStart injection, and MCP servers.
- Codex: check MCP config and the session, user-prompt, tool, compaction, and
  stop hooks.
- Command Code: check `~/.commandcode/mcp.json` and the four stable lifecycle
  events in `~/.commandcode/settings.json`.
- Devin CLI: check `.devin/config.json`, `.devin/hooks.v1.json`, and
  `.devin/skills` for stale MCP, hook, or routing-skill entries.
- Gemini CLI and Antigravity CLI: check `settings.json` or the equivalent hook
  and MCP config files.
- Kimi Code: check `~/.kimi-code/mcp.json` and the `[[hooks]]` entries in
  `~/.kimi-code/config.toml`. Both live under `KIMI_CODE_HOME` when that
  variable is set.
- Kiro CLI: check the `hooks` objects in the agent JSON files under
  `~/.kiro/agents/` (v2), `~/.kiro/hooks/ai-memory.json` (v3), and
  `~/.kiro/settings/mcp.json`. All live under `KIRO_HOME` when it is set.
- OpenCode, OpenClaw, and OMP: check the MCP config and plugin or extension
  directories. Move old memory plugins to a disabled or quarantine directory
  before deleting them. For the OpenCode 2 beta, the plugin file is named
  separately from the first-generation one, and the MCP entry lives under
  `mcp.servers` in the same `opencode.json` or `opencode.jsonc` file.
- VS Code Copilot, Claude Desktop, and Zed: these are MCP-only integrations, so
  confirm whether the old tool provided capture hooks elsewhere. Zed's MCP
  entries live under `context_servers` in its user `settings.json`.

If you want a visible startup reminder during the transition, keep it small. A
rules-file note such as "Active memory: ai-memory; legacy export is historical
reference only; use memory_query for retrieval" is safer than dumping large
legacy context into every session.

If you use the ChatGPT or Codex OAuth provider, sign in once before starting the
server with `AI_MEMORY_LLM_PROVIDER=openai-oauth`:

```bash
ai-memory auth login openai-oauth
ai-memory auth status
```

The login command stores only provider credentials in `auth.json` inside the
data directory. That file is separate from `AI_MEMORY_AUTH_TOKEN`, the
machine-root bearer used by MCP, hooks, handoffs, workstreams, and machine
calls to dual-auth APIs.

For GitHub Copilot, sign in with the matching provider login before starting the
server with `AI_MEMORY_LLM_PROVIDER=copilot`:

```bash
ai-memory auth login copilot
ai-memory auth status
```

Copilot login stores a GitHub user token. Before an LLM call, the provider uses
a cached short-lived Copilot API token. When that token is missing or within a
refresh margin of its expiry, the provider exchanges the GitHub token for a new
one.

## Browse the wiki in a browser

Start the server with `--enable-web` and open `http://<host>:49374/web`.

```bash
ai-memory serve --transport http --bind 127.0.0.1:49374 --enable-web
```

On macOS, the menu bar companion app starts its LaunchAgent with
`--enable-web`, and its **Open Web UI** action opens that same URL.

Docker Compose users can add the flag to the service command:

```yaml
command: ["serve", "--transport", "http", "--bind", "0.0.0.0:49374", "--enable-web"]
```

The built-in web UI is read-only. It offers a project list, a per-project page
tree, breadcrumbs, rendered Markdown, metadata, and full-text search. In rendered
pages, `[[wiki links]]` become clickable links to the target page. These forms
are supported: `[[path]]`, `[[path|label]]`, `[[project:path]]`, and
`[[workspace/project:path]]`. Unscoped targets resolve against the current
page's project. `[[...]]` stays literal inside fenced code blocks (closed only by
the same fence glyph), inline code spans, and four-space-indented code. Targets
with an external scheme (`http://`, `https://`, `mailto:`, `data:`,
`javascript:`, `vbscript:`, `tel:`, `file:`) also stay literal.

With no authority configured on loopback, the built-in wiki stays anonymous.
For human-authenticated administration, serve the compiled admin interface with
`--web-ui-dir`. It signs in through the login route and uses an HttpOnly web
session plus CSRF protection. Until human auth is active, a deprecated
GET-only compatibility path accepts the root bearer through HTTP Basic auth or
an HttpOnly legacy cookie. That path stops working as soon as a human password
or a completed bootstrap exists. Bearers stored in the browser remain
unsupported. `AI_MEMORY_AUTH_TOKEN` and `aim_` API keys stay machine-only
credentials, sent as `Authorization: Bearer <token>` by MCP, hook, handoff, and
workstream clients.

To host the web UI under a URL subpath behind a reverse proxy, use the
`--base-path` and `--web-slug` flags. The defaults are an empty base path and
`/web`. See [frontend-api.md](frontend-api.md) for the flag semantics and
[https-via-proxy.md](https-via-proxy.md) for the proxy-side setup.

## Inspect the raw wiki

The wiki is plain Markdown plus git history. Each write creates a checkpoint
commit in the wiki repository.

```bash
docker exec ai-memory ls /data/wiki/sessions/
docker exec ai-memory cat /data/wiki/sessions/<uuid>.md

# Open in Obsidian or any Markdown viewer:
docker cp ai-memory:/data/wiki ./my-ai-memory-wiki

# Time-travel:
docker exec ai-memory git -C /data/wiki log --oneline
```

## Move a session to another project

A session captured under the wrong project (a `cd` into a scratch directory, or
a subagent started elsewhere) can be reattached without reorganizing the whole
store:

```bash
ai-memory move-session <session-id> --to my-project            # dry run
ai-memory move-session <session-id> --to my-project --confirm  # apply
ai-memory move-session --from-project tmp --to my-project --confirm
```

Without `--confirm` the command only previews the move. The session, its
observations, handoffs, consolidation jobs, and its `sessions/<id>.md` page move
together. See [lifecycle-ops.md](lifecycle-ops.md#move-session) for page modes,
guards, and what stays behind.

## Project consolidation preferences

Create `_prompts/consolidation.md` in a project's wiki when its compiled pages
need stable style, terminology, emphasis, or noise-filtering preferences. For
example, its body may ask for Portuguese titles or tell the consolidator to omit
routine CI output. Automatic consolidation and both manual modes read only the
target project's page. Passing `instructions` to `memory_consolidate` replaces
that page for one call without modifying it.

The page and the per-call value are untrusted project data. ai-memory applies
the configured sanitizer, caps the value at 2,000 characters, and JSON-encodes
it in the LLM user message. Both consolidation system prompts allow only
advisory effects on style, terminology, emphasis, and noise filtering. The value
cannot add facts, authorize disclosure or tool use, or override schema, evidence,
and output rules. A preference page whose TTL has expired is ignored. When there
is no active page and no argument, no preference block is added.

## Rules vs facts

Durable project rules belong in the agent's rules file, not only in the wiki.
For Claude Code that is `CLAUDE.md`. For Codex, Devin CLI, OpenCode, OpenCode 2
beta, Cursor, Gemini CLI, Grok Build CLI, Kimi Code, Kiro CLI, and Command Code
it is usually `AGENTS.md`.

The consolidator classifies compiled observations as `decision`, `fact`,
`rule`, or `gotcha`. Rule-tagged pages are routed to `_rules/<slug>.md`, and
`memory_lint` reports a suggestion when a rule looks durable enough to copy into
`CLAUDE.md` or `AGENTS.md`.

ai-memory never edits the rules file on its own. The lint suggestion is the
whole workflow: copy the rule if it should apply every turn, or ignore it if it
was temporary context.

## Repo-native decision records

Some projects keep the reasoning behind the code in the repository itself, for
example an ADR directory (such as `docs/adr/`) maintained by hand or by a
dedicated tool, or a `context/` tree of decisions, rejected alternatives, and
constraints reviewed in pull requests. Three facts describe how such a record
and ai-memory interact:

1. **ai-memory never touches files in your repository.** Its wiki lives in the
   server's data directory. Background jobs (consolidation, curation, retention
   decay, auto-improvement) read and write wiki pages only. A decision-record
   directory in the repo is outside ai-memory's write surface. Run both side by
   side: the repo owns the canonical record, and ai-memory owns cross-session
   recall.

2. **Keep the record directory out of capture.** An agent reading the record is
   captured like any other file read. Consolidation then compiles what it saw
   into wiki pages, including a `decisions/` page that still says "active" long
   after the repo has superseded it. That page can rank first in `memory_query`
   because it matches the topic. List the directory in the marker's `[capture]`
   section so the copy is never made:

   ```toml
   [capture]
   ignore_paths = ["docs/adr/**"]   # or ["context/**"] for a context tree
   ```

   The repo owns that record, and a compiled copy goes stale the moment the repo
   moves. Details and bounds are in [marker-file.md](marker-file.md).

3. **Wiki pages marked `pinned: true` are immutable to automation.** Retention
   decay and curation skip them. The auto-improvement apply path refuses to
   rewrite them, and the proposal is recorded as a conflict with the reason.
   Unpinning is the explicit opt-out.

For a project without a repo-side record, decisions go into the wiki. The
managed durable-pages Agent Skill teaches agents the recipe: a
`decisions/<slug>.md` page with an ADR structure (Status, Context, Decision,
Consequences, including rejected alternatives), `pinned: true`, and
supersession by a new page instead of editing history. Ask an agent to "record
this as an architectural decision" and the skill does the rest. The structured
shape also retrieves noticeably better through `memory_query` than free-form
prose.
