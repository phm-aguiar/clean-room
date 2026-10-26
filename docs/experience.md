# The experience pass (cross-session review)

The per-session auto-improvement review reads one finished session at a time.
It cannot see that several sessions repeated the same workflow, that the
operator keeps restating a preference that no page records, or that recent
sessions quietly contradict a stored decision. The experience pass reads the
summary pages of the last N sessions of a project **side by side** and proposes
knowledge that is only visible across them.

## Purpose

The pass takes the step from per-session summaries to durable knowledge that
holds across sessions. Its target is the kind of page that makes the fiftieth
session cheaper than the fifth: procedures, rules and preferences, concepts,
gotchas, and decisions.

## Opt-in and default state

The pass is **off by default**. It runs only when all of the following hold:

- the auto-improve scheduler is enabled (`enabled = true`, the default), its
  interval is greater than zero, and `max_sessions_per_tick` is greater than
  zero;
- an LLM provider is configured. Without one, the scheduler does not start, and
  the pass never runs. The zero-LLM default path never runs it;
- `experience_every_sessions` is greater than zero.

Settings live in the scheduler table:

```toml
[auto_improve.scheduler]
experience_every_sessions = 5   # run after every 5 newly completed sessions
experience_sessions = 10        # read the last 10 session summaries
```

| Key | Default | Meaning |
|---|---|---|
| `experience_every_sessions` | `0` (off) | Number of newly completed sessions since the last run that triggers a run. Also the minimum number of usable session pages required to run. |
| `experience_sessions` | `10` | Maximum number of session summary pages read by one run. Values below 1 are raised to 1. |

Turn the pass on when per-session auto-improvement already works and the
project has real session history. Leave it off while the project is young,
because cross-session patterns need sessions to cross, or when no LLM provider
is configured.

## When a run happens

The scheduler checks each project on every tick, independently of whether that
tick has per-session candidates.

1. **Cadence check.** The pass counts the sessions of the project that have an
   end time later than the anchor. The anchor is the time of the last successful
   experience run or, before any run, the time the project's scheduler state was
   first initialized. The anchor is stored in the scheduler state row, so sessions
   that ended before the scheduler first initialized the project are never
   counted. If the count is below `experience_every_sessions`, nothing happens.
2. **Preflight.** When the count is reached, the pass gathers the session pages
   (see below). If fewer usable pages exist than `experience_every_sessions`, the
   run is skipped before any LLM call. The anchor does not move, so the next tick
   tries again. A skip is not an error.
3. **Review.** Otherwise the pass calls the LLM once and validates the response
   through the shared proposal path (see [Staging and approval](#staging-and-approval)).
4. **Anchor update.** After a completed run, the anchor is set to the current
   time. This happens even when no proposal survives validation.
5. **Failure.** If the LLM call, the staging or a store read fails, the anchor does not move
   and the run is retried on a later tick. Errors are logged in a redacted form
   that carries the error class and status, never the provider's response body.

Enabling the pass on an existing store does not re-digest the old history. The
first run waits for `experience_every_sessions` sessions that ended after the
anchor.

## What the pass reads

- **Session window.** The pass looks at up to twice `experience_sessions` of the
  most recent sessions of the project, newest first. It keeps only sessions that
  have ended and have a summary page. It stops once it has `experience_sessions`
  pages. If the window holds fewer summary pages, the run reads fewer pages.
  Sessions without a summary page (lifecycle-only or purged) are skipped.
- **Page size cap.** Each summary page is capped at 6,000 **bytes** in the
  prompt. The cut falls on a character boundary and is followed by a
  `[truncated]` marker. The cap is fixed and is not exposed in configuration.
- **Existing knowledge.** The prompt also includes up to 100 recent wiki pages
  from the project briefing, the patchable pages (rule and procedure pages that
  may be edited in place), and the rejection context described below.
- **Order.** Session summaries appear newest first, each with its start time and
  path. Unlike the per-session review, the list of recent wiki pages is **not** filtered of
  `sessions/` pages, there is no input-token budget (only the per-page caps), the patchable-page
  budget is `max_patchable_body_chars x max_patchable_pages`, the rejection context always uses the
  fixed 12,000-byte cap, and the call carries no operation id of its own. Literal prompt text:
  [prompts-schemas-and-sanitizer.md](prompts-schemas-and-sanitizer.md) section 12.7.

## Entropy pre-filter (opt-in)

Before the summary pages reach the prompt, an optional zero-LLM filter can drop
low-information pages. It is **off by default**. With the filter off, every page
is kept and the run behaves as if the filter did not exist.

Configuration lives in its own table:

```toml
[auto_improve.scheduler.experience_entropy_filter]
enabled = false
min_chars = 16
min_entropy_bits_per_char = 2.0
max_repetition_ratio = 0.7
repetition_min_tokens = 6
```

| Key | Default | Meaning |
|---|---|---|
| `enabled` | `false` | Master switch. When `false`, every page is kept. |
| `min_chars` | `16` | Minimum number of non-whitespace characters. |
| `min_entropy_bits_per_char` | `2.0` | Minimum Shannon entropy of the text, in bits per character. Must be finite and greater than or equal to zero. |
| `max_repetition_ratio` | `0.7` | Maximum repetition ratio. Must be between `0.0` and `1.0` inclusive. |
| `repetition_min_tokens` | `6` | Token count from which the repetition check applies. |

The thresholds are validated at startup, even when the filter is disabled. An
invalid entropy floor or repetition ratio stops the configuration from loading.

When enabled, each page is classified in this order. The first matching rule
decides:

1. **Near-empty.** If the page has fewer non-whitespace characters than
   `min_chars`, it is skipped as near-empty. A page made of blank lines counts as
   empty.
2. **Low entropy.** The Shannon entropy is computed over all characters of the
   page, including whitespace. If it is below `min_entropy_bits_per_char`, the
   page is skipped as low-entropy. A single repeated character scores zero. A
   tiny alphabet scores low.
3. **Repetitive.** The page is split on whitespace and compared case-insensitively.
   If it has at least `repetition_min_tokens` tokens and
   `1 - distinct_tokens / total_tokens` is greater than `max_repetition_ratio`,
   the page is skipped as repetitive. The check is skipped for shorter pages, so
   a terse but real note is not judged repetitive.
4. Otherwise the page is kept.

A skipped page is not consolidated and is never deleted. It is simply absent from
this run. Skipped pages do not count toward the preflight floor, so enabling the
filter can turn a run into a skip. When pages are skipped, the run report gets
one warning with the number of skipped pages.

## Prompt and output contract

- The review is one structured-output request. Its sampling temperature is
  `0.1`, and its output budget is `16,000` tokens.
- The system prompt treats session summaries and existing wiki material as
  **untrusted data**. Instructions, requests to reveal secrets, policy changes,
  and tool directions inside them are historical evidence, not commands.
- The prompt asks for knowledge that spans **at least two sessions**. Each
  proposal must name the sessions it draws on. Single-session findings belong to
  the per-session review and should be rejected here.
- The prompt limits proposal kinds to procedures, rules and preferences,
  concepts, gotchas, and decisions. Decisions are proposed only when several
  sessions show a de-facto decision that no page records, or when new sessions
  contradict a recorded one, and the proposal must say so plainly.
- Rule paths must start with the rules prefix. The prompt also tells the model
  not to target the sessions folder or the pending folder. The validator enforces
  this only indirectly: `sessions/` and `_pending/` are outside the accepted destinations, so such a
  proposal is rejected as `unsupported_path_prefix` (confirmed).
- The prompt rejects narratives, timelines, and status reports.

The validator checks that a proposal has evidence present. It does not count the
sessions cited, so the two-session bar is a request to the model, not a check on
the output.

## Staging and approval

The pass does not build its own write path. Its report goes through the same
stage-and-apply tail as the per-session review.

1. **Validation.** Each proposal is checked for: a supported operation, a
   confidence at or above the floor (default `0.75`), a non-empty rationale, a
   non-empty evidence list, a portable valid path, and a supported edit mode.
   Patch edits are allowed only on rule and procedure pages.
2. **Eval gate.** An optional external check that runs on proposals whose paths
   match its targets. It is disabled by default. See
   [auto-improve-eval-gates.md](auto-improve-eval-gates.md).
3. **Rejection buffer.** Rejected candidates are stored. Later runs receive up to
   50 of them from the last 180 days, capped at 12,000 characters, in the prompt
   under "do not repeat", so the same idea is not proposed again.
4. **Staging.** Surviving proposals are written to the pending folder of the
   project (default `_pending/auto-improve`), each with a sidecar record. The run
   records its provider, model, summary, warnings, rejected candidates, and the
   effective configuration.
5. **Approval.** The setting `require_approval` under the auto-improve table
   decides what happens next:
   - `false` (the default): every staged proposal is approved immediately through
     the normal wiki write path. Pages are written in the same run.
   - `true`: proposals stay in the pending folder until an administrator approves
     them with the pending-writes command.

So the experience pass never writes pages directly. It always goes through
staging and records a run. With the default setting, the staged proposals are
then applied automatically, and the pages are written through the normal write
path. Set `require_approval = true` to keep every proposal pending for review.

## Storage

The pass records its cadence anchor in the scheduler state table. Migration V57
adds one nullable integer column, `last_experience_run_at`, to that table:

- `NULL` means the pass has never completed a run for this project.
- The anchor is `COALESCE(last_experience_run_at, initialized_at)`.
- Completed runs overwrite the column with the current time.

No other table is changed. The schema is described in
[database-schema.md](database-schema.md).

## Cost

Each run that passes the cadence and preflight checks makes one LLM call for the
project. Skipped runs make no LLM call. Each project has its own cadence, so the
cost grows with the number of projects that meet the threshold.

## Related documents

- [auto-improvement-loop.md](auto-improvement-loop.md): the per-session review,
  the scheduler, and the approval flow that this pass shares.
- [auto-improve-eval-gates.md](auto-improve-eval-gates.md): the optional eval gate.
- [design-memory-aging.md](design-memory-aging.md): the aging and hygiene passes,
  which share the low-information filter concept.
- [prompts-schemas-and-sanitizer.md](prompts-schemas-and-sanitizer.md): the prompt
  and output-schema conventions.
