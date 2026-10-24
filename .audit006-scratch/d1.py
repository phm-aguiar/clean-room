import sys
sys.path.insert(0, '.audit006-scratch')
from ed import apply
P = 'docs/design-cross-project-profile.md'
E = []

# D1 audit note
E.append((r'''(the remaining *to confirm* notes concern test coverage only).''',
r'''(the remaining *to confirm* notes concerned test coverage only). A second, line-by-line pass on 2026-10-24 compared every rule below with the harvest, convergence, delivery, `apply`, worker, project-flag, route and MCP code and with the reference tests. It closed those notes (section 9.1 is the adversarial test matrix and lists the gaps of the reference tests; section 12 confirms that the security-boundary rows do not exist), and it added: the exact filter of "user-written text" with reference vectors, the detector edge cases, the client rule for the project flags, the exact texts of `review`, `rebuild` and the `apply` refusals, the Windows file-write details, the MCP metadata validation and error codes, the LLM post-processing details, the worker signal semantics and Appendix D (golden vectors).'''))

# D2 marks ties
E.append((r'''so a long history is consumed over several passes.''',
r'''so a long history is consumed over several passes. Two consequences of "strictly after the mark, oldest first, at most 2,000 rows": rows that share the exact timestamp of the 2,000th row and fall beyond the limit are never read (the next pass starts strictly after that timestamp), and a row stored later with a timestamp older than the mark (an import or backfill that carries an earlier occurrence time) is never read either. Only `profile rebuild`, which clears every mark, recovers them.'''))

# D3a markup block
E.append((r'''(after trimming, it starts with `<`, followed by a tag of 1 to 64 characters made only of ASCII letters, digits, `_`, `-` or `/`, followed by `>` somewhere later; an opening tag with attributes such as `<pasted_content id="x">` does not match)''',
r'''(after trimming surrounding Unicode whitespace it starts with `<`, and the text between that `<` and the FIRST `>` of the whole prompt is 1 to 64 characters long and made only of ASCII letters, digits, `_`, `-` or `/`; so `<task-notification>`, `</x>` and `<b>` match whatever follows, while `<pasted_content id="x">` (a space inside the tag), `<>` and `< 5ms is fine` do not)'''))

# D3b order, vectors
E.append((r'''A delimiter quoted inside a sentence is ordinary text.''',
r'''A delimiter quoted inside a sentence is ordinary text. The two removals run in sequence: first every routing-block region is cut out of the original prompt, then every untrusted-history region is cut out of what is left; an unclosed opening delimiter in the first pass therefore discards the rest of the prompt before the second pass sees it. A cut removes only the text from the first character of the opening delimiter to the last character of the closing one: the indentation before the opening delimiter and whatever follows the closing delimiter on its line stay. The "alone on its line" test trims Unicode whitespace on both sides and counts `\r` as whitespace, so CRLF prompts behave like LF prompts. Reference vectors (input, then the text handed to the sentence detector; `\n` stands for a line break): `Always use pnpm.\n<!-- ai-memory:start -->\nNever write notes.\n<!-- ai-memory:end -->\nKeep commits small.` gives `Always use pnpm.\n\nKeep commits small.`; ``Always use pnpm. The `<!-- ai-memory:start -->` marker sits in CLAUDE.md; never run npm`` is unchanged (the delimiter is quoted mid-sentence, so `never run npm` is still detected); a prompt starting with `# AGENTS.md instructions` or `<task-notification>` gives nothing; `# AGENTS.md instructionsets are confusing` is unchanged. The ingestion path caps a prompt body at 16 KiB, so a pasted block cut before its end delimiter is common in practice. The filter is applied again when `profile rebuild` re-reads old observations, so a prompt stored before any import-side filter existed is filtered here too.'''))

# D4 detector edge cases (insert before the statement cap paragraph)
E.append((r'''The statement is the sentence capped at 200 bytes (cut on a character boundary, no ellipsis)''',
r'''**Detector edge cases (reference behavior).** (a) Lines end at `\n` or `\r\n` (the `\r` is not part of the line); a lone `\r` does not end a line. (b) The sentence split runs after the code-line skip, so a numbered-list line such as `1. Always use pnpm.` yields the fragment `1.` (dropped: fewer than three words) and then `Always use pnpm.`; an abbreviation such as `e.g. use pnpm` splits the same way; `v1.2` and `a.b` do not split (no whitespace after the dot). (c) A colon never splits (`Again: I prefer small commits over big ones.` is one sentence), and a `?` anywhere, even inside a URL, rejects the whole sentence. (d) The 400-byte limit applies to the cleaned sentence (after trimming, quote-marker removal and whitespace collapse). (e) Quote and list markers are removed per piece, so `> - Always use pnpm.` is checked as `Always use pnpm.`. (f) Lowercasing and the letter/number classes follow Appendix A.11.

The statement is the sentence capped at 200 bytes (cut on a character boundary, no ellipsis)'''))

# D5 waiting order
E.append((r'''A group that does not pass is reported as **waiting** with its newest statement, its independent project count, `projects_short` and `contributors_short`.''',
r'''A group that does not pass is reported as **waiting** with its newest statement, its independent project count, `projects_short` and `contributors_short`. Waiting groups, hand-edited paths and forgotten paths are listed in processing order: stack entries first (by tag, byte order), then statement groups in group-creation order (oldest first).'''))

# D6 page statement details
E.append((r'''`applies_to` may be a string or a list of strings; values are trimmed and lowercased and empty ones dropped.''',
r'''`applies_to` may be a string or a list of strings (any other JSON type, and non-string items, are ignored); values are trimmed, lowercased with ASCII rules only, and empty ones dropped; nothing is validated or deduplicated on read (the write paths validate, section 8). A non-string `summary` or `abstract` is skipped like a blank one. The stored reasoning of a managed page (used by the "same ruling" rule in 5.2) is the text between the first line equal to the statement and the heading `## In your words` (or the end of the body), trimmed; it is absent when no line equals the statement or the text is blank.'''))

# D7 ledger clock
E.append((r'''and the write time (current clock, microseconds), in the same scope.''',
r'''and the write time (current clock, microseconds), in the same scope. The ledger time is the wall clock of the write, not an evidence time; that is what makes "newer than the removal" mean "observed after the entry was last written". Pages are written first and the ledger afterwards, in one transaction: a crash between them leaves a page without a ledger row (a later deletion of that page is then not remembered), while candidates and marks are already stored, so the next pass rebuilds the plan from the stored candidates and finishes the work.'''))

# D8 payload order
E.append((r'''A fenced section, placed first in the session-start payload, before the handoff, the managed context, the project brief and the inbox notice.''',
r'''A fenced section, placed first in the session-start payload. The payload is the concatenation, in this order, of: the profile digest, the pending handoff, the pending-handoff notice (present when automatic claiming of the handoff is off), the managed-workstream context, the project brief and the inbox notice; each present piece is joined to the rest by a single line break (pieces end with a line break, so a blank line separates them).'''))

# D9 200 cap
E.append((r'''(pages past the 200th are neither shown nor counted)''',
r'''(pages past the 200th are neither shown nor counted; the cap applies to the pages read, before the ones with nothing to state are dropped)'''))

# D10 client flag rule
E.append((r'''The effective consume flag is what step 2 above uses.''',
r'''The effective consume flag is what step 2 above uses. **What the hook client sends.** When the client resolves a project marker (the nearest one that declares more than `[capture]`), it ALWAYS sends both parameters: `0` when the marker's value for that key is falsy and `1` for anything else (a missing key, an empty value or an unrecognised word included); when no marker resolves it sends neither. Consequently removing the key from the marker turns the setting back on at the next session start (when the caller has write access), and only a client that sends nothing leaves the stored value alone. The client reads each key line by line from that marker file wherever it sits (a `contribute = false` under any section counts, a commented-out line does not, an inline `# comment` is stripped, quotes are optional, and a line with an empty value is skipped in favour of the next matching line). The stored flags are updated by one conditional statement that writes both columns only when one of them differs, so two concurrent identical session starts are harmless: the second finds nothing to change.'''))

# D11 worker
E.append((r'''- The profile worker starts with the server (inside the `serve` process) and stops with it. It does nothing while the profile is off for the deployment.''',
r'''- The profile worker starts with the server (inside the `serve` process, HTTP transport only: the stdio transport runs no worker) and stops with it. It does nothing while the profile is off for the deployment.'''))
E.append((r'''so a burst of session ends costs one pass.''',
r'''so a burst of session ends costs one pass. A signal that arrives during the 30-second wait is not absorbed by the pass about to run: it stays pending and causes one more pass (after another 30 seconds) once that pass ends. The signal is sent for every real session end, managed and child sessions included, after the session summary page was written and committed.'''))
E.append((r'''at shutdown the server waits for it up to the shutdown grace period.''',
r'''at shutdown the server waits for it up to the shutdown grace period (5 seconds) and then exits anyway; the next start resumes from the stored marks. A pass that fails to read what decides it is logged as a warning (`profile pass failed`) and the next pass is scheduled normally. The `profile rebuild` route runs its own pass with no mutual exclusion against the worker, so the two can overlap; every step is idempotent (candidates are insert-or-ignore, pages go through the versioned page-write path), and a reimplementation may serialize them with a lock.'''))

# D12 LLM details
E.append((r'''Calls and fallbacks are counted in the pass report.''',
r'''Calls and fallbacks are counted in the pass report.

**Details to reproduce exactly (second pass).**

- *Classify batches.* The pending sentences of ALL prompts of one project are chunked together in batches of 20, in prompt order (oldest first) and sentence order; the batch, not the prompt, is the unit, and the `index` in the request is the position inside the batch. The budget of 8 is spent per attempted call (a failed call spends it too) and is shared by all projects of the pass, in project order.
- *Classify response handling.* Items whose index lies outside the batch are ignored; for a repeated index the last item wins; a sentence the model did not return is dropped (when the call itself succeeded it does NOT fall back to the zero-LLM rules); `keep = false` and a statement that is empty after whitespace collapse are dropped. The statement is whitespace-collapsed, cut at 200 bytes on a character boundary and trimmed. `category` must equal (after trimming) one of the seven categories, otherwise it is derived from the statement as in 5.2. `applies_to` is trimmed, lowercased, filtered to the 13 language tags, sorted and deduplicated; an empty list means "no tags" and replaces the vocabulary derivation (the derivation of A.7 runs only for candidates that did not go through a successful classification). `confidence` is clamped to [0, 1]; a non-finite value counts as absent. A candidate classified by the model has confidence 0.6 unless the model gave one; a sentence handled by the zero-LLM rules, in any mode, has 0.5. The topic key is computed from the model's statement.
- *Merge.* It is not called when the planned statement already equals the stored one (a settled entry), nor after the budget of 16 is spent; for a new entry the request carries `current_entry: null`. Outcomes of a successful call: `changed = false` with a stored entry and no negation flip keeps the stored statement, reasoning and language tags; `changed = false` with a flip, or with no stored entry, keeps the newest harvested statement and adds no reasoning; `changed = true` takes the model's statement when it is non-empty (collapsed, cut at 200 bytes), sets the reasoning to the model's text (collapsed, cut at 400 bytes; empty clears it) and replaces the tags only when at least one known tag came back. The flip test is the one behind the "same ruling" rule: the same set of non-negation ruling words with opposite parity of negations. `Always use pnpm` / `Never use pnpm` and `Don't use npm` / `Use npm` flip; `Always use pnpm` / `Use pnpm always`, `Never use npm` / `Never use yarn` and `never` / `always` do not.
- *Wire texts.* The system prompts and the user-message wrappers are literal in `prompts-schemas-and-sanitizer.md` section 12.8 (the section 9 there is a description; where it is shorter, this section and 12.8 rule). The request carries only sanitized strings; the evidence items sent to the merge are `project`, `date` and `quote` (the source reference is not sent).
- *Failure isolation.* A classification error falls back for that batch only (counted once in `llm_fallbacks`); a merge error keeps the newest harvested statement for that entry only. Neither stops the pass.'''))

# D13 section 7
E.append((r'''Both keys default to on. An explicit falsy value opts out. The stored flag persists: a session start that omits the key does not undo an opt-out, and only a marker that says otherwise changes it.''',
r'''Both keys default to on. A falsy value (`0`, `false`, `no`, `off`, any case, quoted or bare) opts out, and the hook client forwards the marker's reading at every session start (section 5.4, "Project flags at session start"). The stored flag persists across session starts that send nothing (no marker found, an older client); but a resolved marker that lacks the key, or carries a non-falsy value, sends "on" and re-enables it.'''))
E.append((r'''`min_projects` below 1 behaves as 1. The effective scope''',
r'''`min_projects` below 1 behaves as 1 (it is an unsigned integer). `apply_max_lines = 0` is accepted: `profile apply` then finds no line to write and prints the "No profile entries apply" message even when entries exist. The effective scope'''))

apply(P, E)
