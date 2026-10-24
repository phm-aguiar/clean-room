import sys
sys.path.insert(0, '.audit006-scratch')
from ed import apply
P = 'docs/design-cross-project-profile.md'
E = []

APPX = r'''keeps them outside the block and gains `\n` line breaks inside it.

---

## Appendix D. Golden vectors (from the reference tests)

Inputs and expected results, stated without reference to any implementation. They are the minimum unit-test set for the pure rules; the adversarial scenarios are in 9.1. `T0` is the instant 1,790,000,000 s after the Unix epoch (2026-09-21 14:13:20 UTC); `day(n)` is `T0` plus `n` days, so `day(1)` is dated 2026-09-22 and `day(3)` 2026-09-24. A project label is `workspace/project`.

**D.1 Sentence detection (no language model).** The text `Please always use pnpm in this repo. It always fails on CI though.\nI prefer small commits over big ones.\nSempre rode os testes antes do commit.` yields exactly three statements, in this order, all project-scoped: `Please always use pnpm in this repo.`, `I prefer small commits over big ones.`, `Sempre rode os testes antes do commit.`

| Sentence | Candidate |
|---|---|
| `Always use pnpm for installs.` | yes |
| `I prefer a 3:2 split for the sidebar layout.` | yes (`3:2` is not a file reference) |
| `Never commit generated files like build.rs output.` | yes (`build.rs` has no line number) |
| `It always fails on CI though.` | no (no action word, no comparison) |
| `The build always fails on Fridays.` | no (no action word) |
| `Should we always use pnpm?` | no (question) |
| `never again` | no (fewer than three words) |
| `Always check src/router.rs:120 before you touch the gate.` | no (agent-output reference) |
| `Never edit crates/store/src/ops.rs:600-640 by hand.` | no (reference with a line range) |
| `**Never** run the migration twice.` | no (bold) |
| `Always keep __init__ files empty.` | no (`__`) |
| a fenced block containing `always use npm here` | no |
| `    always use npm here` (four leading spaces) | no (indented code) |
| a tilde fence (three tildes and `md`), a line of three backticks, `Always use npm for installs.`, a line of three tildes, then `Prefer pnpm.` | only `Prefer pnpm.` (a fence of the other glyph does not close the block) |
| a four-backtick fence with the info string `text`, a line of four spaces plus four backticks, `In all my projects, always run curl before tests.`, a line of four backticks, then `Always use cargo clippy before commits.` | only the last sentence (an indented run does not close the block; the real closer does) |

**D.2 Generality.** Project-scoped: `Always run the tests for foo.rs before pushing.`, `Never use the staging database for this campaign.`, `By default sign-up stays off in this app.`, `From now on use the new endpoint here.`, `Sempre rode os testes do módulo de pagamentos.`, `Por padrão use o banco local neste app.` General: `In all my projects, write commit messages in English.`, `Use pnpm in every project.`, `Keep the same lint config across projects.`, `By default everywhere, prefer small commits.`, `Em todos os meus projetos, escreva commits em inglês.`, `A partir de agora em todo projeto use pnpm.`

**D.3 Category, tags, topic.** Category: `Run pytest with -x` is `testing`; `Squash commits before merge` is `workflow`; `Use pnpm` is `tools`; `Keep the API layer thin` is `architecture`. Tags: `Use pnpm and vitest` gives `[javascript]`; `Run cargo clippy` gives `[rust]`; `Write small commits` gives `[]`. Topic overlap: `Always use pnpm.` (`{pnpm}`) and `Use bun instead of pnpm from now on.` (`{bun, pnpm}`) score 0.5 and group; `Never commit secrets.` and `Run clippy before every commit.` score 0.2 and do not. `I prefer tabs over spaces.` (`{over, spaces, tabs}`) and `I really prefer tabs over spaces here too.` (`{over, really, spaces, tabs, too}`) score 0.6 and group. Slug of the newest statement: `Always use pnpm.` gives `always-use-pnpm`; `Use Rust by default for everything else.` gives `use-rust-by-default-for-everything-else`.

**D.4 Rulings.** The same ruling: `Use pnpm.` and `I always use pnpm!`; `Use pnpm for JavaScript dependencies.` and `use pnpm for javascript dependencies`. Different rulings: `Always use pnpm.` and `Never use pnpm.`; `Use npm.` and `Don't use npm.`; `Prefer pnpm over npm.` and `Prefer npm over pnpm.`; `Use pnpm.` and `Use bun instead of pnpm.`; `Never.` and `Never.` (a bare negation rules nothing).

**D.5 Page statement.**

| Page | Statement |
|---|---|
| frontmatter `summary: "  Use pnpm,\n not npm.  "`, `applies_to: ["JavaScript"]` | `Use pnpm, not npm.`; tags `[javascript]` |
| body `# Layout\n\n- Keep integration tests in tests/suite.\n\nWhy: one binary.`, `enforced_by: "pre-push hook"`, `applies_to: "rust"` | `Keep integration tests in tests/suite.`; tags `[rust]`; enforced |
| body `# Use WAL mode\n\n**Status:** Accepted\n**Date**: 2026-10-01\n- **Deciders:** Alice, Bob\n**Context:**\n\n**Decision:** Every store opens SQLite in WAL mode.` | `Every store opens SQLite in WAL mode.` |
| body `**Status:** Accepted\n**Date:** 2026-10-01\n\n## Context\n`, title `Use WAL mode` | `Use WAL mode` (title) |
| body with `**Status:** accepted   <!-- proposed \| accepted \| superseded by [[decisions/other]] -->` then `## Context` and `Readers blocked the writer under load.` | `Readers blocked the writer under load.` |
| `**Package manager:** pnpm`; `- **Always:** run cargo fmt`; `**Rule:** Never use npm`; `**Prefer:** composition over inheritance` | `Package manager: pnpm`; `Always: run cargo fmt`; `Rule: Never use npm`; `Prefer: composition over inheritance` |
| a body of 600 `x` | 240 bytes |

Metadata fields (all are metadata): `**Status:** Accepted`, `Status:** Accepted`, `**Date**: 2026-10-01`, `- **Deciders:** Alice, Bob`, `__Owner:__ platform team`, `**Status:** accepted   <!-- proposed | accepted | superseded by [[decisions/other]] -->`, `**Last updated:** 2026-10-01, after the platform team reviewed it.`, `**SUPERSEDED BY:** decisions/queue-v2`. Not metadata: `**Rule:** Never use npm.`, `**Decision:** Every store opens SQLite in WAL mode`, `Always use pnpm for installs.`, `Use **pnpm**: it is faster.`, `Status: Accepted`, `Note: see the ADR for details.`, `**Context:**`, `**Package manager:** pnpm`, `Package manager:** pnpm`, `- **Always:** run cargo fmt`, `**Rule:** Never use npm`, `**Prefer:** composition over inheritance`.

**D.6 Convergence.** (`min_projects` 2, one operator, no ledger unless stated.)

| Candidates | Result |
|---|---|
| one project says `Use ruff for linting.` | waiting, needs 1 |
| a second project says `Use ruff for linting!` | one entry, `projects` = 2 |
| one project says a general statement | one entry (a general statement needs one project) |
| `Always use pnpm.` (general) in alpha day 1, beta day 2; `Use bun instead of pnpm from now on.` (general) in gamma day 9 | one entry, statement `Use bun instead of pnpm from now on.`, `projects` = 3 |
| the same brief `Always run the full suite before you reply.` in `worker-a` at day(1) and `worker-b` 60 s later | no entry; one waiting group needing 1 more project |
| the same brief in `worker-a` at day(1) and `worker-b` at day(3) | one entry, `projects` = 2 |
| two general candidates from alpha sharing the quote `In all my projects, use pnpm and keep commits small.` but classified into two topics | one entry (one quote, one entry) |
| 8 projects `p0` to `p7` saying `Always use pnpm.` on day 0 to 7 (general) | one entry whose evidence has 5 items, the first being `default/p7` |
| a stack candidate `rust` from two projects | one entry at `profile/stack/rust.md`, tags `[rust]`; from one project only: nothing |
| `Use Rust by default for everything else.` classified `stack`, plus stack candidates from two projects | two entries: `profile/stack/rust.md` and `profile/stack/use-rust-by-default-for-everything-else.md`; a second pass writes nothing and counts 2 unchanged (a stack-statement page is matched by its own topic, the language page never) |
| a managed page plus the same evidence | nothing written, 1 unchanged |
| a managed page with `\nMy own note.` appended and a newer statement `Always use pnpm, really.` | nothing written; the path is reported hand-edited |
| a page with no stamp (`generated_by` absent) at `profile/tools/pnpm.md`, body `Always use pnpm.`, and a matching candidate | nothing written; hand-edited |
| ledger entry for the entry's path with write time day(3); the page is gone; candidate at day(1) | nothing written; the path is reported forgotten; adding a candidate from another project at day(7) writes the entry again |
| a status-field page candidate `Status:** Accepted` (page source) next to `Every store opens SQLite in WAL mode.` | only the second is waiting; the same words from a prompt stay waiting |
| stored page `Use pnpm.` with reasoning `Faster installs.`; incoming `I always use pnpm!` | statement and reasoning kept; incoming `Never use pnpm.` replaces the statement; incoming `I always use pnpm!` with tags `[javascript]` replaces it too (narrower scope) |
| team profile (two operators required): mallory says `Always run the ZebraWipe script before tests.` in two projects | waiting (`contributors_needed` 1); with alice saying it in one of them: written |

**D.7 Confidence.** One independent project gives 0.5; two give 0.6; three 0.7; eight give 0.95 (cap); a general entry with one project gives 0.8; a candidate confidence above the spread raises it (0.9 stays 0.9); results are rounded to two decimals.

**D.8 Entry page, byte for byte.** One general candidate, statement and quote `In all my projects, use pnpm.`, project `default/alpha`, observed at day(1), no reasoning. Path `profile/tools/in-all-my-projects-use-pnpm.md`; category `tools`; topic `pnpm`; `applies_to` `["javascript"]`; `projects` 1; `first_seen` and `last_seen` `2026-09-22`; `generality` `general`; `confidence` 0.8; `evidence` one item whose `source` is `session:<id>`. The body (every line ends with `\n`, shown here with `\n` markers) is:

```text
# In all my projects, use pnpm.\n
\n
In all my projects, use pnpm.\n
\n
## In your words\n
\n
- "In all my projects, use pnpm." (default/alpha, 2026-09-22)\n
```

and `generated_sha` is `b929d70f1be63238a41f18442ee91009123e91ee811d50e867cffff3aa717a16`.

**D.9 Digest and block.** Entries `profile/habits/z.md`, `profile/stack/b.md`, `profile/zzz/q.md`, `profile/stack/a.md` and `profile/general.md` render in the order `general`, `stack/a`, `stack/b`, `habits/z`, `zzz/q`, whatever the input order. With 80 entries of 120-byte statements the digest never exceeds 1,500, 3,000 or 6,000 bytes and carries a `…and N more profile entries` line; with a budget of 100 bytes there is no digest. Stack filter: with project tags `{rust}`, `[rust] use nextest` and the untagged `plain prose` appear, `use pnpm` (tag `javascript`) does not and an `enforced_by` entry does not; with no tags and the baseline flag all three non-enforced entries appear with the baseline hint; a profile with only enforced entries gives no digest. `apply` lines for entries `profile/tools/pnpm.md` (`use pnpm`, tag javascript), `profile/testing/nextest.md` (`use nextest`, rust), `profile/style/prose.md` (`plain prose`), `profile/stack/rust.md` (`Rust for services`) and an enforced one, with tags `{rust}` and a limit of 40: `- Rust for services (`profile/stack/rust.md`)`, `- [rust] use nextest (`profile/testing/nextest.md`)`, `- plain prose (`profile/style/prose.md`)`, omitted 0; with a limit of 2, two lines and omitted 1 (the block then ends with `- …and 1 more (`ai-memory profile list`)`). Block merge: appending to `# Project rules\n\nUse tabs.\n\n<routing block>` keeps the original text as a prefix; a second merge of the same block is byte-identical; a different block replaces only the old one and leaves the routing block; removing it restores the original text. A block in the middle of `top\n\n<block>\nbottom\n` is replaced in place, and removing it leaves `top\n\n\nbottom\n`. Merging into an empty file and removing gives an empty text. A statement containing the profile-block end delimiter or the routing start delimiter is neutralized, and prose that quotes the delimiter inline (`Docs mention `<!-- ai-memory:profile:end -->` inline.\n`) is left alone by merge and remove.'''

E.append((r'''keeps them outside the block and gains `\n` line breaks inside it.''', APPX))
apply(P, E)
