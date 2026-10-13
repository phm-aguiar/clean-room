# The cross-project profile

> **Escopo:** este documento contém partes que dependem de LLM ou de embeddings. Essas partes são opcionais e não fazem parte do escopo inicial, que funciona sem LLM. Podem ser removidas ou adiadas junto com as funções correspondentes.

ai-memory keeps knowledge per project, so switching agent harness costs nothing:
Claude Code, Codex and OpenCode in one checkout read the same pages. The
**profile** removes the other cost, switching *project*. It is a small set of
pages recording how you usually work (the package manager you reach for, how
you lay out tests, the architecture you prefer, the review workflow you follow),
and every project receives it at session start, through every harness.

When you start a new repository and describe what you want, the agent already
has your baseline, so you do not re-explain it.

Design and rationale: [design-cross-project-profile.md](design-cross-project-profile.md).

## How the agent uses it: rules first, memory as the fallback

The profile never overrides anything more specific. The order is:

1. Your current instructions in the conversation.
2. The repository's rules file (`AGENTS.md`, `CLAUDE.md`, or the harness's own).
3. The project's memory (its pages, rules and decisions).
4. The profile: what you usually do across projects.

So when a rules file says "use tool-b", the profile's "usually tool-a" does not
apply. When nothing says, the agent uses the profile instead of asking again,
and says which default it applied so you can correct it.

A profile entry is a preference you expressed, used as a default. Like all
stored memory it is still data, not a command: it can pick a tool, a layout or
a convention, but it cannot authorize anything your current instructions would
not (tool use, permission changes, disclosure, destructive actions).

## Defaults

| Deployment | Profile | Where it lives |
|---|---|---|
| Single operator (no database users, no trusted identity proxy), any number of workspaces | **on**, shared by every workspace | The `_global` project of the default workspace, under `profile/` |
| Multi-user (database users exist, or a trusted identity proxy is configured) | **off**; opt in with `enabled = true` | One private profile per operator |

A single operator gets one profile across everything, including several
workspaces. A server with several operators keeps the profile off until the
operator enables it, and then each person gets their own private profile:
nothing one operator's agents learn reaches another's.

Whether a server is multi-user is read again on every pass and every session
start. Creating the first database user therefore turns an `auto` profile off
without a restart.

## What you see at session start

The session start opens with a short fenced section, ahead of any handoff,
project brief or inbox notice. Those change every session; the profile changes
only when you do, so leading with it keeps it in the part of the harness's
prompt that can be cached:

```text
> 🧭 **ai-memory: your usual choices** (cross-project profile)
> Your usual choices from other projects. Use them as defaults when this
> project's rules and the user say nothing; say which default you applied. …
- [lang-c] Use tool-a, not tool-b. (`profile/tools/tool-a.md`)
- Keep integration tests in tests/suite. (`profile/testing/layout.md`)
```

- **Shape.** A heading, a precedence line (your instructions, then the rules
  file, then the project's memory), a security notice, and then the entries
  inside a fence that marks them as stored, untrusted text. A footer tells the
  agent to open an entry with `memory_read_page` before relying on it for
  anything non-trivial.
- **One line per entry**, each naming its page. The line is the entry's
  statement, collapsed to one line and cut at 240 bytes, with the stack tags in
  brackets when the entry has any.
- **Filtered by stack.** An entry with language tags (for example `lang-a` or
  `lang-b`) appears only in a project whose recent activity shows one of
  those tags. Tags come from file names in the titles of the project's 2,000
  most recent observations: manifest file names and source file extensions. A
  project with no detected tags, such as one with no activity yet, gets every
  entry. An entry without tags appears everywhere.
- **Entries enforced elsewhere are left out.** An entry marked `enforced_by`
  (something else, like a hook, already enforces it) is not shown.
- **Order.** Fixed: entries directly under `profile/` first, then the known
  categories in digest order (`stack`, `architecture`, `workflow`, `testing`,
  `tools`, `style`, `habits`), then unknown categories; within a category, by
  path.
- **Bounded.** The section stays within `digest_max_bytes` (default 3,000
  bytes, measured in UTF-8 bytes). Entries that do not fit are replaced by a
  line such as "…and 4 more profile entries (`memory_query` finds them)".
  A digest reads at most 200 entries, in path order; entries past that point
  are neither shown nor counted, and `memory_query` still finds them.
- **A baseline for new projects.** A project with no current pages of its own
  gets a larger budget (`baseline_max_bytes`, default 6,000 bytes) and a note
  that the agent can offer `ai-memory profile apply` to write your usual choices
  into the new repository's rules file.
- **Stable.** The text has no timestamps and a fixed order, so it is
  byte-identical between sessions until the profile changes.
- **Team wording.** On a multi-user server with a shared profile, the heading
  reads "team defaults" and says the choices are shared, not this user's own
  (see [Team profile trust model](#team-profile-trust-model)).
- **Stored text cannot break the fence.** Any ai-memory comment delimiter in a
  statement is neutralized before it is rendered, so an entry cannot close the
  fenced region or a rules-file block early.

No digest is produced when the profile is off, when the project sets
`[profile] consume = false`, when the client already delivered it this session,
when `inject_on_session_start` is false, when no entry applies, or when the
session is started inside the profile's own scope (its pages are already the
project's memory). Any failure while reading the profile also yields no digest:
a session start never fails because of it.

The profile also travels with retrieval: `memory_query` returns profile entries
in `global_scope_hits`, next to the rest of the global scope. See
[MCP server](mcp-server.md) for the tools.

## Adding, reading and removing entries

Ask the agent to remember a standing preference ("always use tool-a in my
projects"); it writes it with `memory_write_page` and `scope: "profile"`. You
can also write one yourself:

```text
memory_write_page { "scope": "profile", "path": "tools/tool-a.md",
                    "body": "# Tool-a\n\nUse tool-a, not tool-b.\n\nWhy: workspaces and a strict lockfile." }
```

The path is stored under `profile/` (`profile/tools/tool-a.md`). Use a category
folder: `stack`, `architecture`, `workflow`, `testing`, `tools`, `style` or
`habits`; the digest lists them in that order. Optional frontmatter:

| Key | Meaning |
|---|---|
| `summary` (or `abstract`) | The digest line. Otherwise the first line of prose in the body is used. |
| `applies_to` | Language tags the entry is scoped to, e.g. `lang-a` or `[lang-b]`. Matched case-insensitively. |
| `enforced_by` | Something already enforces it (e.g. `pre-push hook`); kept out of the digest and out of rules-file blocks. |

Without a summary, the digest line is the first prose line of the body, with
headings, horizontal rules and list markers skipped, whitespace collapsed, and
the result cut at 240 bytes. A page with nothing to state is not an entry.

Writing to the profile needs write permission on the profile's scope (see
[Enabling it on a multi-user server](#enabling-it-on-a-multi-user-server)). If
the profile is off on the server, the write fails and says so.

Entries are ordinary wiki pages, markdown in git: edit them by hand, and every
change is versioned. `memory_delete_page` with `scope: "profile"` removes one
(git keeps its history). Most entries, though, are learned on their own; see
[How entries are learned](#how-entries-are-learned).

From the command line (on a multi-user server, the root token is required; see
[command-line reference](cli.md)):

```bash
ai-memory profile status                 # on/off, where it lives, entries, digest size, opt-outs
ai-memory profile list                   # every entry with its scope tags
ai-memory profile show tools/tool-a.md     # one entry (the profile/ prefix is optional)
ai-memory profile forget tools/tool-a.md   # remove one entry (stays removed until you say it again)
ai-memory profile review                 # entries with their evidence, habits still short of the bar
ai-memory profile rebuild                # re-read every project from the start (safe to repeat; removes nothing)
ai-memory profile apply                  # write the entries that fit this repo into its rules file
ai-memory profile list --user alice      # an operator's private profile (share = "user")
ai-memory profile list --workspace work  # a workspace profile (share = "workspace")
```

`profile status` prints the effective settings and the projects that opted out
of contributing.

## Turning the profile into hard rules for one repository

The digest is a default the agent applies when nothing more specific speaks.
When you want your usual choices to be **rules** in a repository, written down
where every harness and every collaborator sees them, run this from inside it:

```bash
ai-memory profile apply --dry-run   # show the block and the file it would go to
ai-memory profile apply             # write it
ai-memory profile apply --remove    # take it out again
```

- **Which file.** `--target FILE` (relative to the repository root, unless
  absolute) wins. Otherwise an existing `AGENTS.md`; else an existing
  `CLAUDE.md`, unless it contains only an import of `@AGENTS.md`; else a new
  `AGENTS.md`. The repository root is the nearest directory holding a `.git`
  entry (a directory, or the file a linked worktree has); if there is none, the
  current directory is used.
- **What it writes.** The entries that fit this repository, selected the same
  way as the digest (scoped to the language tags the project's activity shows,
  entries enforced elsewhere skipped; a project with no detected tags gets all
  of them). At most `apply_max_lines` (default 40) entries are written, each with
  the page it came from, between `<!-- ai-memory:profile:start -->` and
  `<!-- ai-memory:profile:end -->` delimiters. If more entries apply, one extra
  line counts the rest and points to `ai-memory profile list`; that line is not
  counted against the limit.
- **What it never touches.** Anything outside those delimiters, including the
  `<!-- ai-memory:start -->` routing block and your own rules. Re-running
  replaces only the block, and leaves the file as it was when the profile has
  not changed. `--remove` takes the block out and restores the file; a file
  `apply` created for the block alone is deleted. Delimiters count only when
  they stand alone on their line.
- **Backups.** The previous version of the file is copied to
  `<data dir>/backups/profile-apply/`, never next to the file, so nothing extra
  shows up in `git status`.
- **Opt-outs.** A project with `[profile] consume = false` refuses `apply`,
  both when its marker says so and when the server has it recorded. `--remove`
  is not refused.

`apply` runs only when you run it: the profile itself never edits a rules file.
Because the block then sits in the rules file, it outranks the digest for that
repository (step 2 of the order above).

## How entries are learned

### When a pass runs

A harvest-and-converge pass runs:

- about two minutes after the server starts;
- 30 seconds after a session ends (a burst of session ends in that window costs
  one pass; intermediate turn checkpoints do not trigger one);
- hourly otherwise.

Each pass reads only what is new since the previous one. For each contributing
project it reads at most 2,000 user prompts and 2,000 curated pages; a longer
history is consumed over later passes. A failure in one project or scope is
recorded in the pass report and the pass continues.

Projects are never harvested when they set `[profile] contribute = false`, when
they are reserved profile projects, or when they are the default catch-all
project of the default workspace (the one that has no working directory).

**1. Harvest.** From every project that contributes, it collects:

- **Your own prompts** that read as a preference or a correction. A sentence
  qualifies when it carries one of the preference markers below *and* either
  names an action (for example "use", "run", "keep", "commit", "test") or is a
  self-contained comparison ("prefer X over Y", "X instead of Y", "X rather
  than Y"). The markers are *always*, *never*, *prefer*, *instead of*, *rather
  than*, *don't use*, *do not use*, *from now on*, *by default*, *I usually*,
  *in all my projects*, *every project*, *across projects*, *everywhere*, and the
  Portuguese equivalents (*sempre*, *nunca*, *prefiro*, *em vez de*, *não use*,
  *a partir de agora*, *por padrão*, *eu costumo*, *em todos os meus projetos*,
  and others). Matching is on whole words, case-insensitively.

  Text is read line by line, and each line is split into sentences on `.`, `!`
  and `;` followed by whitespace or the end of the line. These never qualify: questions (anything containing `?`), text inside
  code blocks or indented as code, sentences longer than 400 bytes (a paste,
  not a preference), fragments under three words, and text that reads like
  agent output (markdown bold, or a `file:line` reference). Agents briefing
  each other through the prompt channel produce exactly that kind of text.
  The statement is the sentence collapsed to one line and cut at 200 bytes;
  the quote kept as evidence is the verbatim sentence, cut at 500 bytes.
- **Curated pages** of the project: pages under `_rules/`, `decisions/`,
  `gotchas/` and `procedures/` that were updated since the last pass. A page's
  statement is its summary or first prose line; it is general if it says so.
- **Stack signals**: the language tags the project's recent activity shows
  (manifest file names and source extensions in observation titles). Only
  language tags count; tool names are scoping hints, not a stack on their own.

Tool output (what a command printed, what a file contained) is never read, so
text an agent merely *saw* cannot become your preference.

Each finding becomes a candidate with its source, the time it was observed, the
project, the contributing operator when known, a topic key (its statement's
topic words, sorted), a category and language tags. Without an LLM, a candidate
from a prompt has confidence 0.5; a curated page 0.55; a stack signal 0.5.

**2. Converge.** Per profile scope (the shared profile, one workspace's
profile, or one operator's private profile), the candidates are considered
together with the existing entries.

- **Grouping.** Statements are grouped by topic. A candidate's topic words
  (lowercase words of at least two characters, minus function words, the
  preference markers and generic verbs; tool and language names are kept)
  are compared with each existing group. The candidate joins the group whose
  most similar member scores at least 0.5 on overlap (shared words divided by
  all words of both), breaking ties toward the older group; otherwise it
  starts a new group. Candidates are processed oldest first.
- **Admission.** A group becomes an entry when either:
  - a member was stated as general, by an explicit cross-project scope ("in all
    my projects", "every project", "across projects", "everywhere", and the
    Portuguese equivalents). A bare "always", "never", "by default" or "from
    now on" is not general; it has to meet the project threshold; or
  - its *independent* projects number at least `min_projects` (default 2).

  Independent means: the same quote seen in several projects within one hour
  of a counted occurrence counts once. That is one message fanned out to
  several checkouts (a lead agent briefing its workers), not a habit. And one
  quote backs at most one entry per pass, so a statement the classifier splits
  into two topics does not produce duplicate pages.
- **Stack entries.** A language tag seen in at least `min_projects` distinct
  projects becomes `profile/stack/<language>.md`, scoped to that language, with
  the statement "<Language> is one of the stacks this user usually works in."
  Stack entries use the plain project count, not the independent-project count.
- **Latest ruling wins.** The statement of an entry is the newest statement in
  its group. The page it replaces stays in the page history, never destroyed.
  Settled wording is kept (see below).
- **Category.** The first matching category keyword wins, checked in this
  order: `testing`, `workflow`, `tools`, `architecture`, `style`. Otherwise a
  language or tool name in the statement makes it `stack`; otherwise `habits`.
  When an LLM classified the candidate, its category is used if it is a known
  one.
- **Language tags.** Derived from the language and tool names the statement
  mentions (for example a package manager name tags its language, so "use tool-a"
  is tagged `lang-c`), or taken from the LLM.
- **Path.** `profile/<category>/<slug>.md`, where the slug is lowercase
  alphanumerics and dashes, at most 60 characters, and `entry` when empty. A
  new page whose path is taken gets `-2`, `-3`, and so on.
- **Confidence.** Starts at 0.5 for one independent project and rises by 0.1 per
  additional one; a general entry is at least 0.8; a higher candidate confidence
  (an LLM's) can raise it; the result is capped at 0.95 and rounded to two
  decimals.
- **Evidence.** The newest five pieces of evidence (project, source, date, your
  words), newest first, one per project and quote.

Each written entry is a page with a heading and the statement, the reasoning if
an LLM supplied one, and an "In your words" list of quotes with their project
and date. Its frontmatter records `kind: preference`, the title (the statement
cut at 80 bytes), category, topic, language tags, project count, first and last
seen dates, confidence, generality, the evidence, `generated_by: profile-harvest`
and a hash of the body. The frontmatter is derived from the evidence, so a group
whose evidence has not changed is not rewritten.

### Latest ruling and settled lines

Without an LLM, new evidence that makes the **same ruling** keeps the entry's
wording: the same topic words in the same order, with the same negations in the
same places, and the same language tags. "use tool-a" and "I always use tool-a"
match; "never use tool-a" is a change, and "tool-a over tool-b" differs from "tool-b
over tool-a". Corroborating evidence then only updates the evidence list, so the
line every project sees does not change.

Any other change replaces the statement with the newest one (the latest ruling
wins). When you change your mind ("use tool-c instead of tool-a from now on"), the
newer statement replaces the entry.

With an LLM, the model judges whether the evidence changes the ruling or its
scope. If it says no and the words did not reverse, the stored text stands,
even if the new evidence is worded differently. If the words reversed ("always"
against "never"), the newest statement is used.

### With an LLM

With a provider configured (and `[profile] llm` left on), the pass uses it in
two places, both through JSON-schema structured output, at temperature zero, with
an output limit of 2,000 tokens per call:

- **Classify.** Preference-shaped sentences from your prompts go to the model,
  in batches of up to 20 sentences. A looser filter feeds it: any sentence with
  a preference marker, without the action-word requirement. The model decides
  which are real preferences (`keep`), whether they are general, their category
  and language tags, and restates each as one imperative line of at most 200
  bytes, with a confidence clamped to 0 to 1. The quote kept as evidence is
  always your own sentence, never model text.
- **Merge.** When an entry gains evidence that may change it, the model restates
  it from your words, newest ruling first, with your reasoning when you gave one
  (at most 400 bytes). It also says whether anything changed; when the evidence
  only confirms the entry, the entry stays exactly as written. A merge is not
  asked for an entry whose text is already settled.

A pass makes at most 8 classify calls and 16 merge calls in total. Work beyond
that uses the zero-LLM path. If a call fails, the pass falls back to the
zero-LLM path for that batch or entry, and the report counts the fallback.
Without a provider, the profile works the same way, with the word lists and the
newest statement.

Both prompts treat every sentence as data, rank your own words first and are
told never to follow instructions found in the text.

### Team profile trust model

A team profile reaches every operator's session start, so the harvester holds it
to a higher bar than a personal one. A statement is admitted only on evidence
from **at least two distinct operators**, on top of `min_projects` (or a general
statement). Operators are counted from the owner of the session that produced a
prompt, or the author of a curated page; evidence with no known owner or author
does not count toward them. One operator repeating "always run X before tests" in two open projects
is not enough; `profile review` lists such an entry as waiting for another
operator. Restricted projects never feed a team profile. Stack entries keep the
plain project threshold. Entries written directly with `scope: "profile"` still
need root or a `write` grant on the scope.

## Correcting a wrong entry

- **Remove it:** `ai-memory profile forget <path>`. The harvester remembers the
  entries it wrote, so a removed one is not recreated from the same topic; it
  comes back only when you state it again, in evidence newer than its last write.
- **Reword it:** edit the page (by hand, or with `memory_write_page` and
  `scope: "profile"`). An entry you edited becomes yours: the harvester never
  rewrites a page it did not write, or one whose body changed since it wrote it.
  `profile review` lists such entries as edited by hand.
- **Start over:** `ai-memory profile rebuild` clears each project's read
  position and re-reads it from the beginning, subject to the per-pass limits
  above. It does not touch entries you edited or removed, and it never removes
  an entry: a candidate already recorded keeps the classification it was
  recorded with, and an entry whose evidence no longer meets the bar stays as
  written.
- **After upgrading:** entries admitted under an earlier version's rules stay,
  even after `profile rebuild`; list them with `ai-memory profile review` and
  remove the ones you do not want with `ai-memory profile forget <path>`.

## Settings

In the server configuration file (or as environment variables named
`AI_MEMORY_PROFILE__<KEY>`, for example `AI_MEMORY_PROFILE__SHARE=workspace`):

```toml
[profile]
enabled = "auto"            # auto | true | false
share = "auto"              # auto | global | workspace | user | off
min_projects = 2
inject_on_session_start = true
digest_max_bytes = 3000     # clamped 1500..12000
baseline_max_bytes = 6000   # clamped 2000..20000
apply_max_lines = 40
llm = true
```

- **`enabled`**: `auto` turns the profile on for a single-operator server and
  off for a multi-user one; `true` and `false` override that for any
  deployment. The words `on`, `yes` and `1` mean true, and `off`, `no` and `0`
  mean false. A multi-user server keeps `auto` off even if you set `share`, so
  enabling it there is always an explicit `enabled = true`.
- **`share`**: where the profile lives.
  - `global`: one profile for the whole server, in the `_global` project of the
    default workspace, for every workspace. On a multi-user server this is a
    **team** profile: readable by everyone, writable only by root or an operator
    holding a `write` grant on `_global`.
  - `workspace`: one profile per workspace, in that workspace's reserved
    `_profile` project. Use it when workspaces separate things that must not
    mix, such as work and personal, or two clients. On a multi-user server it is
    write-gated like `_global` (root or a `write` grant on that `_profile`).
  - `user`: one private profile per operator, in a restricted project of the
    default workspace named `_profile.<user id>`, created for that operator. Only
    they and root can read or write it. Root requests have no private profile,
    since no database user stands behind them. On a single-operator server it
    behaves like `global`.
  - `off`: no profile at all.
  - `auto` (default): `global` on a single-operator server, `user` on a
    multi-user one.
- **`min_projects`**: how many independent projects a choice must appear in
  before it joins the profile on its own (see Admission above). A value below 1
  is treated as 1. A choice you state as general ("in all my projects") needs
  only one.
- **`inject_on_session_start`**: `false` keeps the digest out of session start;
  the profile still reaches `memory_query`.
- **`digest_max_bytes`** / **`baseline_max_bytes`**: the digest budgets, in
  UTF-8 bytes, clamped to the ranges shown. The floors keep room for the fixed
  precedence and security lines plus a few entries.
- **`apply_max_lines`**: the most entries `ai-memory profile apply` writes into a
  rules file.
- **`llm`**: use the configured LLM provider, when there is one, to classify and
  merge entries; `false` keeps the profile fully zero-LLM.

`ai-memory profile status` prints the effective values.

### Common customizations

| You want | Set |
|---|---|
| Keep work and personal (or two clients) apart | `share = "workspace"` |
| No LLM involvement in the profile, even with a provider configured | `llm = false` |
| Only admit habits you repeat in more projects | `min_projects = 3` (or more) |
| A smaller digest at every session start | `digest_max_bytes = 1500` |
| The profile in retrieval only, no digest | `inject_on_session_start = false` |
| No profile at all on a single-user server | `enabled = false` (or `share = "off"`) |
| One project neither teaches nor receives it | `[profile] contribute = false` and `consume = false` in its marker |

### Enabling it on a multi-user server

```toml
[profile]
enabled = true          # share stays "auto": a private profile per operator
```

Each operator authenticates with their own API credential; their agents write
and read their own `_profile.<user id>`. Requests made with the root token have
no private profile. The `_profile` and `_profile.*` names are reserved: an
operator can name only their own private profile explicitly (workspace
`default`, project `_profile.<their id>`), so nobody can create another
operator's profile ahead of them. Profile projects cannot be renamed and their
access mode is fixed.

For a shared team profile instead, set `share = "global"` (or `"workspace"`)
and grant the curators `write` on `_global` (or that workspace's `_profile`):

```bash
ai-memory user grant --user alice --workspace default --project _global --level write
```

## Per-project opt-outs

In a project's marker file (`.ai-memory.toml`):

```toml
[profile]
contribute = false   # never learn from this project (client or NDA work)
consume = false      # no profile digest here, and no profile in this project's queries
```

Both default to on. Whenever a hook client finds the project's marker it sends
both values explicitly (`0` or `1`) at session start, and the server records
them on the project, so they apply to every harness. A falsy value is `0`,
`false`, `no` or `off`; any other value, or no value in the marker, is on.
Removing the key from the marker turns the setting back on at the next session
start. A session start that sends no value at all (no marker found, an older
hook bundle, a client not yet regenerated) leaves the recorded setting as it is:
an opt-out is never undone by a client that did not say anything. The recorded
value follows the marker of the most recent session start, so keep the setting
in the marker every checkout of the project shares (two checkouts whose markers
disagree flip it back and forth).

Setting `contribute = false` also removes the project's existing evidence from
the profile's inputs: candidates from an opted-out project are ignored by every
pass, including evidence harvested before you set it.

A marker that sets any setting beyond `[capture]` (for example only `[profile]`
keys, or `[briefing]` keys) is a settings boundary: an outer marker's
`workspace` and `project` do not apply through it. A marker that sets only
`[capture]` keys is transparent. `ai-memory profile
status` lists the projects that opted out of contributing.

## Privacy and isolation

- A private profile (`share = "user"`) is a restricted project: another
  operator's read, write or delete is refused, and a query never unions it for
  anyone but its owner.
- The shared scopes (`global` and `workspace` profiles) are readable by every
  operator but writable only by root or a `write` grant, because they reach
  everyone's session starts.
- Event capture is never attributed to a profile project, whether the name
  comes from a directory or a marker.
- Entries go through the same sanitizer as every page; `contribute = false`
  keeps a project out of the profile entirely, including evidence harvested
  before you set it.
- Harvesting reads only your prompts and curated pages, never tool output.
- A private profile only ever learns from its own operator's sessions and pages,
  in projects that operator can read. A shared profile on a multi-user server
  (`global` or `workspace`) never learns from a restricted project, because
  everyone can read it.
- Text sent to the LLM goes through the sanitizer again first.

## Troubleshooting

- **No digest at session start.** Run `ai-memory profile status`: the profile
  may be off (a multi-user server needs `enabled = true`), it may have no
  entries yet, the project may set `consume = false`, or every entry may be
  scoped to a language this project does not use. A session started inside the
  profile's own scope (for example in `_global` itself) gets no digest, since
  those pages are already its project memory.
- **Kimi Code.** Kimi re-fetches on every prompt; the digest is delivered on the
  first prompt of each session only.
- **The agent did not apply a default.** The rules file and your instructions
  win; check that the repository does not say otherwise. Run
  `memory_read_page` on the entry to see what it actually says.
- **A habit never became an entry.** `ai-memory profile review` shows it under
  "Not in the profile yet" with how many more projects it needs; say it once as
  a general rule ("in all my projects, …") to admit it now, or lower
  `min_projects`.
- **An entry stopped updating.** It was edited by hand, so the harvester leaves
  it alone (`profile review` says so). Delete it to hand it back.
- **An opt-out came back on.** Only an explicit value changes it: check that the
  project's marker still says `contribute = false` / `consume = false` and that
  the agent runs in a directory under that marker (`ai-memory profile status`
  lists the projects opted out of contributing).
- **A team entry is "waiting for another operator".** A team profile admits a
  statement only when at least two operators said it (see the trust model
  above). Have a second operator confirm it, or write the entry directly with
  `scope: "profile"` if you hold the grant.
- **A private profile is unreachable after `ai-memory reindex`.** Reindex
  rebuilds the database from the wiki files, and users are database-only state:
  operators created again get new ids, so an old `_profile.<old id>` project
  belongs to nobody (only root can read it). Its entries are still markdown
  files in that project's `profile/` folder inside the wiki. To carry them over,
  have the operator write one entry with `scope: "profile"` (which creates their
  new `_profile.<new id>`), then copy the old `profile/` files into the new
  project's folder in the wiki while the server runs; the wiki watcher indexes
  created and modified files into the new profile.
