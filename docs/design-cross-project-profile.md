# Design: the cross-project profile

> **Escopo:** este documento contém partes que dependem de LLM ou de embeddings. Essas partes são opcionais e não fazem parte do escopo inicial, que funciona sem LLM. Podem ser removidas ou adiadas junto com as funções correspondentes.

*Status: accepted (2026-10-04). Implemented in 2.6.0; behavior corrected in 2.6.1 and 2.6.2. User guide: [cross-project-profile.md](cross-project-profile.md).*

This document describes how the profile behaves today. Anything that is only proposed is marked **Proposal**, and it is collected in section 12. A claim that could not be confirmed is marked **to confirm**.

## 1. The problem

ai-memory keeps knowledge per project, and switching harness already costs nothing: Claude Code, Codex and OpenCode in one checkout read the same pages. What still costs something is switching **project**. A new repository starts from zero, so the user re-explains the same habits in each one: the package manager they use, how they like tests laid out, which architecture they reach for, the review and commit workflow they follow.

Nothing converges that knowledge by default. The reserved `default/_global` scope only receives pages written there explicitly, reaches the agent only when a query happens to match it, and is kept out of the session-start brief. Cross-project sharing of rules is deferred in [design-rules-promotion.md](design-rules-promotion.md).

The **profile** fills that gap. It is a small, curated, cross-project record of how this user usually works, built from evidence, delivered to every project and every harness, and used as the default whenever nothing more specific says otherwise.

## 2. Doctrine: memory as the fallback, rules as the override

Before 2.6, stored memory was treated only as untrusted history, and always-on rules were expected in the repository's rules file. That protected against a stored page turning into an instruction, but the agent ignored what it already knew about the user unless someone copied it into every repository.

From 2.6 the precedence is ordered:

1. The user's current instructions in the conversation.
2. The repository's rules file (`AGENTS.md`, `CLAUDE.md`, or the harness's own).
3. The project's memory (its pages, rules and decisions).
4. The profile: what the user usually does across projects.

Lower levels fill gaps; they never override a higher one. When a rules file says "use tool-b", the profile's "usually tool-a" does not apply. When nothing says otherwise, the agent uses the profile instead of asking again, and says which default it applied so the user can correct it. The digest states this precedence itself (section 5.4).

What does not change: retrieved text is still data. A profile entry is a preference the user expressed. It can set a default (a tool, a layout, a convention), but it cannot authorize anything the current instructions would not allow: no tool use, permission change, disclosure or destructive action because a stored page says so. The digest is fenced and labelled the same way the brief is.

## 3. Ideas borrowed from an external design

An external chat design, OptChat (later rewritten as UniiChat), makes one endless conversation the memory: a tree of one-line summaries over a verbatim log, a fixed-size view, and a tool that opens a summary back into detail. The rewrite fixed a cache problem in the first version: a pair's age was measured from its first message, merges ran at every message, and the view was rebuilt at start, so old lines kept being rewritten and the cached prefix broke early. The profile has no tree and no view, and it does not use that merge formula. It applies two of the lessons: stable-first ordering, and leaving settled lines alone (sections 5.2 and 11).

Borrowed and implemented:

- **The user's own words rank first.** Candidates come mainly from the user's prompts and corrections and from curated pages, never from tool output. The merge prompt asks for the user's phrasing and reasoning, as close to verbatim as the space allows.
- **The latest ruling wins.** Within a topic group the newest statement becomes the entry. The replaced version of the page stays in the wiki's version history.
- **Fixed byte budget, stable rendering.** Budgets are UTF-8 bytes, not tokens. The digest has one line per entry (its tags, statement and page path), with no evidence counts, dates or confidence, in a fixed order.
- **Settled lines do not churn.** New evidence that agrees with an entry is added to its evidence, but the statement and therefore the digest line stay as they are. The statement changes only when the ruling changes or its scope narrows or widens.
- **Zoom.** Each digest line names its page. The digest footer tells the agent to open an entry with the page-read tool before relying on its one-line summary for anything non-trivial. Each page's evidence records the project and the source (session or page) that taught it.
- **Context for the summarizer, which never obeys.** The merge step sees the current entry when it folds in new evidence. Its prompt forbids following instructions found in the text, tells it to use the evidence only to understand the entry, and to credit quoted words to their real author.

Not borrowed: the single universal chat and harness (ai-memory is multi-harness, and each harness owns its own context), a verbatim log kept forever (the sanitizer boundary and capture exclusions take precedence), and a large token-sized view. The injected digest stays in the low kilobytes; rule adherence drops well before that size (see [design-rules-promotion.md](design-rules-promotion.md)).

## 4. Where the profile lives

The `share` setting selects the scope. `auto` (the default) picks by deployment. A deployment is **multi-user** when the server has at least one database user or a trusted identity proxy is configured. Otherwise it is single-operator.

| Deployment | Default | Profile location |
|---|---|---|
| Single operator, any number of workspaces | **on**, `share = "global"` | The `_global` project of the default workspace, under `profile/` |
| Multi-user | **off**; opt in with `enabled = true`, which gives `share = "user"` | One private profile per operator |

Explicit values:

- `global`: one profile for the whole server, in `default/_global`. Every workspace contributes and consumes it. On a multi-user server it is a **team profile**: it harvests only from projects that are not restricted, and every operator can read it, as with the rest of the global scope.
- `workspace`: one profile per workspace, in the reserved `_profile` project of that workspace. Use it when workspaces must not mix (for example work and personal, or two clients). On a multi-user server it is a team profile with the same harvest restriction.
- `user`: one private profile per operator, in a restricted project of the default workspace named `_profile.` followed by the operator's user id (hyphens removed). The operator is its creator. It harvests only that operator's own sessions and pages, and only from projects the operator can read. Other operators cannot read or write it; root can, as with every restricted project. On a single-operator server it behaves like `global`.
- `off`: no harvesting, no digest, no query union.

Profile projects are reserved names: event capture is never attributed to them.

## 5. Pipeline

A **pass** harvests every contributing project and then converges each profile scope. Section 5.5 says when passes run.

### 5.1 Harvest (per project, zero-LLM by default)

**Which projects are read.** A project is read only when its stored contribute flag is on (set by the project marker, section 7). Reserved profile projects are never read, and neither is the catch-all project named `scratch`.

**How far it is read.** Each project has two marks: the newest user-prompt time and the newest curated-page update already read. A pass reads at most 2,000 newer prompts and at most 2,000 newer page updates per project. Marks only move forward, so a long history is consumed over several passes.

**Sources.** Candidates come from three sources, after the sanitizer:

1. **User prompts.** Only observations of the user-prompt kind are read. Tool output is never a source. Sentences are checked by the detection rules below.
2. **Curated pages.** Current, unexpired pages whose path starts with `_rules/`, `decisions/`, `gotchas/` or `procedures/`. The page's statement is its summary or abstract from frontmatter, else the first line of prose in the body, else its title. It is collapsed to one line and capped at 200 bytes. The quote is the same line, and generality is read from the statement text. Confidence 0.55.
3. **Stack signals.** The project's 2,000 most recent observation titles are scanned for dependency manifest and lock file names, and for source file extensions. A token counts only when its file name matches. Only the fixed list of language tags (thirteen tags, for example `lang-a`, `lang-b` and `lang-d`) becomes a candidate: topic and tag are the language, category `stack`, confidence 0.5. Tool tags (for example a package manager or a container file) are scoping hints only and are not harvested on their own.

**Detection rules (zero-LLM).** Text is split into sentences at a period, exclamation mark or semicolon followed by whitespace or the end of a line. Fenced code blocks and indented or tab-led lines are skipped, and leading bullet and quote markers are stripped. A sentence is a candidate when all of these hold:

- It has no question mark, is at most 400 bytes, and has at least three words.
- It does not read like agent output: no bold markers (double asterisks or underscores) and no `file:line` reference token. Briefs written by one agent for another are excluded this way.
- It contains a preference marker as whole words, case-insensitive. English markers: `always`, `never`, `prefer`, `instead of`, `rather than`, `don't use`, `dont use`, `do not use`, `from now on`, `by default`, `i usually`, `in all my projects`, `every project`, `across all my projects`, `across projects`, `everywhere`. Portuguese equivalents are accepted: `sempre`, `nunca`, `prefiro`, `prefira`, `em vez de`, `ao invés de`, `ao inves de`, `não use`, `nao use`, `a partir de agora`, `daqui pra frente`, `por padrão`, `por padrao`, `eu costumo`, `em todos os meus projetos`, `em todos os projetos`, `em todo projeto`.
- It either contains a self-contained comparison marker (`prefer`, `instead of`, `rather than` and their Portuguese forms), or it names an action word (for example `use` or `run`). "It always fails" is not a preference; "always run the tests first" is.

The statement is the sentence capped at 200 bytes, and the quote is the sentence capped at 500 bytes. The candidate is **general** only when it contains a general marker: `in all my projects`, `every project`, `across all my projects`, `across projects`, `everywhere`, or `em todos os meus projetos`, `em todos os projetos`, `em todo projeto`. A bare `always`, `never` or `by default` is project-scoped. Confidence 0.5.

**Recorded candidate.** Each candidate stores its source kind (prompt, page or stack), a source reference (`session:<id>`, `page:<path>` or `stack:<tag>`), a topic key, its category, statement, quote, language tags, the time it was observed, the contributing operator (none when unknown or shared), its generality and its confidence. The identity of a candidate is the workspace, the project, the source kind, the source reference and the topic key. Recording the same identity again does nothing, so re-harvesting is idempotent.

**Topic key.** Lowercase words of at least two characters are kept (letters, digits, `+` and `#` count as word characters). Stop words are removed: English and Portuguese function words, the marker words, and generic verbs. Tool and language names are kept. The key is the sorted set of remaining words, space-separated.

### 5.2 Converge (per profile scope, zero-LLM grouping)

**Candidates per scope.** The newest 20,000 candidates of contributing projects are read. Each scope sees:

- `global`: every candidate; on a multi-user server, only from projects that are not restricted.
- `workspace`: the candidates of each workspace separately, with the same restriction on a multi-user server.
- `user`: the candidates whose operator is that operator, from projects that operator can read.

**Grouping.** Candidates are processed oldest first. A candidate joins the group whose member is most similar to it, where similarity is the Jaccard overlap of topic words. The candidate joins only when that overlap is at least 0.5, and ties go to the older group. Otherwise it starts a new group. Only groups that share at least one topic word with the candidate are compared, and each distinct word set in a group is compared once. Grouping never uses a language model.

**Admission of a statement group.**

- *Independent projects.* The same quote (compared after normalizing words and punctuation) seen in several projects within one hour of an occurrence already counted is one message fanned out, and counts once. Otherwise each project counts.
- *Bar.* A group is admitted when it is general, or when its independent projects reach `min_projects` (default 2). On a team profile it also needs at least two distinct named operators. A general statement skips the project bar, but not the operator bar. A group that does not pass is reported as **waiting**, with the projects and operators still needed.
- *One quote, one entry.* A group whose every quote already backs an entry admitted in the same pass adds nothing.
- *Stack entries.* A language tag seen in at least `min_projects` projects is admitted as a stack entry. It skips the general rule and the operator bar.

**Ruling and scope.** The entry's statement is the newest candidate's statement (the latest ruling wins). Its language tags are that candidate's tags. Its category is the candidate's category when it is one of the known categories, else it is derived from the topic. Derivation checks the keyword sets in a fixed order and takes the first hit. If none matches, a language tag name in the statement gives `stack`, and anything else gives `habits`. Language tags for a candidate come from a fixed vocabulary of language and tool words (for example a package manager name implies its ecosystem), so a statement about one stack is scoped to it rather than applied everywhere.

**Path.** `profile/<category>/<slug>.md`. The slug keeps lowercase ASCII letters and digits, joins other runs of characters with single dashes, is capped at 60 characters, and is `entry` when empty. If the path is taken by another group, a numeric suffix is added (`-2`, `-3`, and so on).

**Matching an existing page.** A group is matched to the existing non-stack page whose topic words overlap the group's words by at least 0.5. The best overlap wins, and ties go to the smaller path. Each page is claimed by at most one group per pass. A matched page keeps its path, which is how a new ruling replaces the old one. Existing pages are read with the same 200-page cap as the digest (section 5.4).

**Forgotten topics.** A ledger row (section 5.3) whose page is gone was removed by the user. The topic stays removed while the group's newest evidence is not newer than the removed entry's last write and the group overlaps that topic. A newer statement readmits it.

**Placing an admitted entry.** When the group matches an existing page:

- If the page is not managed (section 5.3), the entry is not written and the path is reported as **hand-edited**.
- If the page already holds the same evidence (same evidence list, same project count, same last-seen date), it is counted as **unchanged**.
- Otherwise the page is updated. If the new statement makes the same ruling with the same language tags as the stored text, the stored statement and reasoning are kept, so the digest line does not change. Two statements make the same ruling when their content words appear in the same order after negations are normalized. A rewording the check does not recognize counts as a change.

### 5.3 Store: the entry page

Entries are wiki pages in git, editable by hand.

```text
profile/<category>/<slug>.md
category: stack, architecture, workflow, testing, tools, style, habits
```

**Frontmatter** (written by the harvester):

- `title`: the statement capped at 80 bytes.
- `kind`: `preference`.
- `summary`: the statement.
- `category`, `topic`, `applies_to`.
- `projects`: count of distinct projects.
- `first_seen`, `last_seen`: dates, `YYYY-MM-DD`.
- `generality`: `general` or `project`.
- `confidence`: see the formula below.
- `evidence`: up to five items, newest first, deduplicated by project and quote. Each item has the project (`workspace/project`), the source (`session:<id>`, `page:<path>` or `stack:<tag>`), the date and the quote.
- `generated_by`: `profile-harvest`.
- `generated_sha`: SHA-256 of the body the harvester wrote.
- `enforced_by` (optional, set by hand): a hook or rules-file line that already enforces the entry. A non-empty value removes the entry from the digest and from `profile apply`. The harvester never writes this key.

**Body.** A `# ` heading (the title), the statement as one line, the reasoning paragraph when a merge produced one, and a `## In your words` list with each quote, its project and its date.

**Statement precedence when reading a page.** The frontmatter summary, then the frontmatter abstract, then the first prose line of the body (headings and rule lines skipped, bullets stripped), then the title. Statements read for the digest are capped at 240 bytes.

**Confidence.** The spread is `0.5 + 0.1 × (independent projects − 1)`. A general entry is at least 0.8. The value is raised to the best candidate confidence if that is higher, capped at 0.95, and rounded to two decimals.

**Managed pages.** A page is managed only when its `generated_by` stamp is present and its `generated_sha` matches its current body. The harvester rewrites only managed pages. A page written or edited by hand is never rewritten, and the group that would have updated it is reported as hand-edited.

**Writes.** Entries are written through the wiki's page-write path, so the sanitizer and the admission chain apply to them (see [wiki-files-layer.md](wiki-files-layer.md)). Rendering is deterministic: nothing depends on the clock, so an unchanged entry produces the same page.

**Ledger.** Each entry written is recorded with its path, its topic and the write time, in the same scope. A ledger row whose page no longer exists means the user removed the entry, and the topic is not recreated until a candidate newer than that write arrives (section 5.2).

### 5.4 Deliver

**Digest at session start.** A fenced section, placed first in the session-start payload, before the handoff, the managed context, the project brief and the inbox notice. It changes only when the profile changes, so it stays in the part of the harness prompt that is reused between sessions.

*Selection.* An entry is considered when it is not marked `enforced_by`, and either it has no language tags, or the project shows at least one of its tags. A project that shows no stack yet gets every entry, so a brand-new project receives the whole baseline. Reads are capped at 200 profile pages per scope, in path order.

*Order.* Entries directly under `profile/` come first. Then the categories in this order: `stack`, `architecture`, `workflow`, `testing`, `tools`, `style`, `habits`. An unknown category comes after them. Within a group, entries are ordered by path.

*Budget.* `digest_max_bytes` (default 3,000) bounds the whole section, including the header, the fences, the footer and any baseline hint. Lines are added in order and the first entry that does not fit stops the list. The rest are counted in a final line that says how many more entries exist and points to the query tool. If not even one entry fits, no digest is sent.

*Lines.* Each line has the form `- [tags] statement (page path)`. The tags part is omitted for entries without language tags. Fence delimiters and managed-block delimiters inside a statement are neutralized, so stored text cannot close a region it is rendered into.

*Stability.* The digest contains no timestamps, no counts that change between sessions and no clock-dependent text, so it is byte-identical between sessions until the profile changes.

*Header and footer.* The header gives the title, a sentence stating that these are the user's usual choices from other projects, to be used as defaults when the project's rules and the user say nothing, and that the agent should say which default it applied. It also states the precedence order and a security notice, and opens the untrusted-data fence. The footer closes the fence and tells the agent to open an entry with the page-read tool before relying on its summary for anything non-trivial.

*Team variant.* On a team profile the title is "team defaults", and the text says the choices come from several operators on the server, not necessarily this user.

*Baseline.* When the project has no current pages, the digest uses `baseline_max_bytes` (default 6,000) and adds a line saying the agent can offer `ai-memory profile apply` to write these choices into the repository's rules file.

*Skipped when.* The profile is off; `inject_on_session_start` is false; the project's consume flag is off; the request's profile-digest parameter is false (**to confirm** which client sends it and why); the operator topology cannot be read; the profile scope cannot be resolved or does not exist yet; the session is already in the profile's own scope, where its pages are read as project memory; or no entry applies. Every failure degrades to no digest.

**Query union.** For a `global` profile, the `_global` scope is already part of query results. For `workspace` and `user` profiles, the query also searches the caller's profile scope, subject to the same authorization as any read. A failure to resolve that scope drops those hits and does not fail the query. Profile hits are returned in the separate list of standing preferences (`global_scope_hits`). When the project's consume flag is off, profile hits are removed from that list.

**`profile apply`.** Writes entries into the repository's rules file (section 8).

### 5.5 When a pass runs

- The profile worker starts with the server. It does nothing while the profile is off for the deployment.
- The first pass runs 120 seconds after startup.
- Each session end wakes the worker. The wake-up waits 30 seconds, so a burst of session ends costs one pass.
- Without a wake-up, a pass runs every hour.
- The deployment's operator topology is read at each pass, so adding the first database user turns an `auto` profile off without a restart.
- A pass harvests every contributing project, then converges each target. A failure in one project or scope is recorded in the pass report, and the pass continues.

## 6. Language model use (when a provider is configured)

The zero-LLM path is complete on its own. With a provider configured and `[profile] llm` not set to false, the pass makes two kinds of structured-output calls, each with a fixed response schema, temperature 0 and an output limit of 2,000 tokens. Grouping stays zero-LLM in both cases.

**Classify (harvest).** Sentences that pass the marker pre-filter (a preference marker present, no agent-output markers) are sent in batches of up to 20. At most 8 classification calls are made per pass. The request contains the category list and the sentences, sanitized and JSON-encoded as data. For each sentence the response gives:

- `index`, `keep` (durable preference, decision or habit: yes or no);
- `generality`: `project` or `general`;
- `category`: one of the known categories, otherwise derived;
- `statement`: one imperative line, capped at 200 bytes;
- `applies_to`: only the known language tags are kept;
- `confidence`: in `[0, 1]`; a non-finite value falls back to 0.6.

The quote is always the user's own sentence, never model text. Unknown response fields are rejected. The LLM path's pre-filter is only the marker check and the agent-output check: the question, length and action-word rules belong to the zero-LLM path. Sentences beyond the call budget use the zero-LLM detection rules.

**Merge (converge).** Called for each entry about to be written whose statement differs from the stored statement, at most 16 times per pass. The request contains the current statement (if any), the latest statement and the evidence (project, date and quote), sanitized. The response gives:

- `changed`: whether the evidence changes the ruling or the scope;
- `statement`: one imperative line, capped at 200 bytes;
- `reasoning`: the user's reasoning, capped at 400 bytes, empty when none;
- `applies_to`: known language tags.

Outcome:

- `changed` is false and the newest statement does not reverse the stored one: the stored statement, reasoning and tags stand.
- `changed` is false but the newest statement reverses the stored one (same topic words, negation flipped, for example "always use X" and "never use X"): the newest harvested statement is kept, so the latest ruling wins.
- `changed` is true: the statement and reasoning are taken from the response. Tags are replaced only when the response gives at least one known tag.

**Fallbacks.** Any provider error falls back to the zero-LLM path: classification uses the zero-LLM rules for that batch, and merge keeps the newest harvested statement. Calls and fallbacks are counted in the pass report.

## 7. Configuration

Server settings (`config.toml`, or environment variables with the `AI_MEMORY_PROFILE__` prefix):

```toml
[profile]
enabled = "auto"            # auto | true | false (also on/off, yes/no, 1/0)
share = "auto"              # auto | global | workspace | user | off
min_projects = 2            # projects a choice must span (values below 1 act as 1)
inject_on_session_start = true
digest_max_bytes = 3000     # clamped to 1500..12000
baseline_max_bytes = 6000   # clamped to 2000..20000
apply_max_lines = 40        # not clamped
llm = true                  # use the configured provider when there is one
```

An unknown `enabled` value fails the configuration load. `share` accepts only the five listed values.

Per project, in the project marker file:

```toml
[profile]
contribute = false   # never harvest this project (client or NDA work)
consume = false      # no digest, no apply, no query union in this project
```

Both keys default to on. An explicit falsy value opts out. The stored flag persists: a session start that omits the key does not undo an opt-out, and only a marker that says otherwise changes it.

## 8. Commands

```text
ai-memory profile status | list | show <path> | forget <path> | review
              | rebuild | apply [--target FILE] [--dry-run | --remove] [--user NAME]
```

- **status** shows the effective share, the settings, the entry count, the digest size and the projects that opted out.
- **list** shows the current entries.
- **show** prints one entry page. **forget** deletes an entry page; the git history keeps it, and the ledger keeps the topic from being recreated until the user states it again.
- **review** writes nothing. It shows the current entries (most recent first), the waiting groups with what they still need, the pending updates, the hand-edited pages and the forgotten topics.
- **rebuild** clears every harvest mark and runs a pass at once, re-reading every contributing project. It never removes an entry. Candidates recorded by an earlier version keep the classification they were recorded with. To drop an entry admitted under older rules, run review, then forget on that entry.
- **apply** writes the entries that apply to the current repository into a managed block of its rules file. The block holds at most `apply_max_lines` (default 40) entries, selected and ordered as in the digest. If more apply, a final line says how many more exist.

  *Target.* `--target` (relative to the repository root), else an existing `AGENTS.md`, else an existing `CLAUDE.md` unless it only imports `AGENTS.md`, else a new `AGENTS.md`. The repository root is the nearest ancestor holding git metadata (a directory, or the file a linked worktree uses), or the working directory when there is none.

  *Block.* Delimited by start and end comment lines, which count only when they stand alone on their line. It opens with a heading and a sentence telling the reader to change the profile and re-run the command rather than edit the block. Text outside the block is never touched. With no block present, the block is appended after a blank line. The write is atomic, and the previous file is backed up under the data directory.

  *Options.* `--dry-run` prints the block and the target without writing. `--remove` deletes the block and the blank line before it; if the file held only the block, the file is deleted. `--user` selects an operator's private profile on a multi-user server. When no entry applies, nothing is written. The command refuses when the project's marker sets `consume = false`.
- **Server routes.** status, list, review and apply read the profile admin routes; rebuild posts to them. show and forget reuse the page read and page delete admin routes against the profile scope. On a multi-user server all of these are root-only. Operators manage their own private profile through the MCP tools.

**MCP tools.** No tool is added. `memory_write_page` with `scope: "profile"` writes a page under `profile/` in the caller's profile scope, creating it on first use. `memory_delete_page` with `scope: "profile"` removes an entry. `memory_query` returns profile hits as described in section 5.4.

## 9. Security boundaries

- A `user` profile is private. Another operator's read, write or delete is refused, because the profile is restricted to its creator (root is admitted, as with every restricted project). The harvester never admits another operator's sessions or pages into it.
- On a multi-user server, a `global` or `workspace` profile harvests only from projects that are not restricted.
- Writing a shared scope (the `global` profile or a workspace profile) on a multi-user server requires root or a write grant on that scope. Reads are open to every operator.
- `contribute = false` is honored by the harvester. `consume = false` is honored by the digest, by `profile apply` and by the query union.
- Candidates come only from user prompts and curated pages, after the sanitizer. Tool output is never a source.
- The digest and the LLM requests treat stored text as data. Statements are neutralized so they cannot close the digest fence or the managed block.
- Event capture is never attributed to a reserved profile project.
- Each boundary should have an adversarial test. Tests exist for the store and the admin routes; per-boundary coverage is **to confirm**. A row per boundary in the security boundaries document is **Proposal** (section 12).

## 10. Out of scope

Automatic edits of a rules file (only `profile apply` writes one), a universal harness, and promoting arbitrary project knowledge into the profile without evidence from more than one project or an explicit general statement.

## 11. Revisions

**2.6.1 (2026-10-08)** corrected three places where 2.6.0 diverged from the design:

1. **Digest position.** The digest is first in the session-start payload, ahead of content that changes every session.
2. **Churn on corroboration.** New evidence that agrees with an entry updates its evidence and leaves the statement and reasoning as they were. The merge reports `changed`, and its restatement is ignored when `changed` is false. 3. **Quadratic grouping.** Grouping compares a candidate only with groups that share one of its topic words, once per distinct word set in each group. This keeps a pass near-linear, with no stored state.

Also in 2.6.1: sentences that read like agent output are not candidates; the same sentence in several projects within an hour counts as one project; an explicit cross-project scope is required to waive `min_projects` (a bare "always" is not enough); and one sentence backs at most one entry.

**2.6.2** corrected the rebuild guidance: rebuild never removes an entry, and candidates keep their original classification.

## 12. Proposals (not implemented)

- **Fully incremental grouping.** Keying each candidate to a stored group would avoid regrouping every pass. It needs a storage change and was planned for 2.7. Today's grouping is the one described in section 5.2.
- **Byte ruler with retry.** Instead of truncating a statement longer than 200 bytes, re-ask the model with a byte limit. Considered and not scheduled. Truncation affects only the rare statement over 200 bytes.
- **A row per profile security boundary** in the security boundaries document. No such rows exist in the current document (to confirm).
