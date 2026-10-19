# Typed relation edges

> **Escopo:** este documento contém partes que dependem de LLM ou de embeddings. Essas partes são opcionais e não fazem parte do escopo inicial, que funciona sem LLM. Podem ser removidas ou adiadas junto com as funções correspondentes.

A page can declare *typed* edges to other pages. A plain `[[wikilink]]` says
that two pages are related. A typed edge also says how: the page causes the
target's problem, fixes it, or contradicts it.

```yaml
---
title: Fixed the linker OOM
relations:
  fixes: ["gotchas/linker-oom.md"]
  contradicts: ["decisions/0007-static-linking.md"]
---
```

## When to use them

Plain wikilinks are the default and are enough when pages are only related.
Typed edges earn their keep in one loop: **contradictions that lint should
chase**.

1. A gotcha page exists: `gotchas/linker-oom.md` ("the linker OOMs on machines
   under 32 GB").
2. Months later a session fixes it. The consolidator (or a person) writes
   `notes/linker-fix.md` with `relations: { fixes: ["gotchas/linker-oom.md"] }`.
   The gotcha's history now shows what resolved it.
3. Later still, new evidence disagrees with a stored decision. The new page
   declares `contradicts:`, and the lint pass reports the pair as a
   `contradiction` finding until someone reconciles them. No model call is
   involved: the declaration is the signal.

If you do not use lint and do not need fix or cause chains, omit the
`relations` key. Nothing else changes.

## Vocabulary

The vocabulary is closed:

| Relation | Meaning |
|---|---|
| `causes` | this page describes a cause of the target |
| `fixes` | this page fixes the problem the target describes |
| `contradicts` | this page disagrees with the target |

The set is closed on purpose. A free-text relation column becomes an
unqueryable folksonomy. Keys are matched exactly (case-sensitive). What
happens to a key outside the set depends on the entry point; see
[Where invalid input goes](#where-invalid-input-goes).

Each relation key takes a list of targets. An empty list is accepted and
creates no edge.

## Target syntax

Targets use the same grammar as wikilinks:

| Form | Resolves to |
|---|---|
| `path` | a page in the source page's own project |
| `project:path` | a page in a sibling project of the same workspace |
| `workspace/project:path` | a page in a project of another workspace |

Rules that apply to every form:

- `path` is relative to the root of the target project.
- A target whose last segment has no extension gains `.md`.
- A target that names no page is not stored. This covers a directory form
  (trailing slash), a bare `.md` name, and any extension other than `.md`.
- The project name `_global` is the reserved preferences project. A
  `_global:path` target with no workspace resolves in the default workspace,
  named `default`. An explicit `workspace/_global:path` keeps the named
  workspace.
- In the file form, a workspace or project qualifier may contain letters,
  digits, `-`, `_`, `.` and `/`. Whitespace around the qualifier components
  is trimmed. A target whose qualifier contains other characters is read as a
  plain path, which then usually fails the rules above.

## Limits

These limits apply to the public write metadata, that is, the page write
tool and the admin write endpoint:

- At most **32 targets in total**, across all relation keys, in one write.
- Each target is at most **1024 characters**, counted before normalization and
  including its scope qualifier.
- A target may not contain control characters.
- The qualifier has at most one `/`. No component may be empty, carry
  surrounding whitespace, or be `.` or `..`.
- The normalized target must be a valid page path that is also portable as a
  file name on every supported system. Reserved characters, a trailing dot or
  space, reserved device names and a `.git` component are all rejected.
- If any target breaks a rule, the whole write is rejected with a validation
  error and nothing is saved. The error text is `invalid or oversized page
  metadata: relations`. An unknown relation key, or a value that is not a list
  of strings, is rejected earlier, when the request is parsed.

Validation order for each target on the public surface: (1) at most 1024
characters and no control character (tab and newline count as control); (2) if
it contains `:`, split at the **first** `:`: the left part is the qualifier and
must split on `/` into at most 2 components, none empty after trimming, none
different from its trimmed form, none equal to `.` or `..`; (3) the path part
is trimmed; its last segment must not be empty or exactly `.md`; if the last
segment contains a `.`, the path must end in `.md` (so `notes/v1.2` is
rejected), otherwise `.md` is appended; (4) the result must be a valid page
path and portable. Total target count is checked first, over all relation keys.
Duplicate targets are not an error on this surface (storage keeps one row per
target and label).

Page files and consolidator output have **no count or length cap**. Their
targets are checked only against the target rules above. The portability
check and the one-slash qualifier limit are not applied to them. A qualifier
with more than one `/` is read as a project name containing that slash, so it
usually resolves to nothing.

## Where invalid input goes

| Entry point | Unknown relation key | Invalid target | Too many or too long |
|---|---|---|---|
| Public write metadata | Rejected | Rejected | Rejected |
| Page file frontmatter (read on write or reindex) | Skipped with a warning | Skipped with a warning | Not limited |
| Consolidator output | Cannot occur: the output has three fixed fields, and other keys are dropped | Dropped by the target rules | Not limited |

A value under a valid key that is not a list is rejected on the public write
surface. On the file path it is ignored without a warning;
list items that are not strings are silently skipped, and a `relations` value that is not a
mapping contributes nothing. In the file path, a skipped unknown key or non-page target logs
a warning with the value cut to 200 bytes.

## Storage

- Each edge is one row in the links table, from the source page to the target
  (project, workspace and path), labelled with its relation. Plain wikilinks
  carry the label `references`. See [database-schema.md](database-schema.md).
- The same target with a different label is a separate row. A typed edge and a
  plain wikilink to the same target therefore coexist. Within one page, a
  repeated target with the same label is stored once.
- Each stored page version keeps its own edges, taken from its body and
  frontmatter. Lint and retrieval read only the latest version, so the edges
  of superseded versions are kept but ignored by default.
- Unresolved targets: a row stores the target as written. It points at a page
  only if the target's latest version exists when the row is written. When a
  page at that path is written later, every matching row is repointed to the
  new latest version, including rows that were already resolved. If the target
  is hard-deleted, its rows stay in place and become unresolved.
- The repointing runs in the same transaction on **every** page write that
  creates a version: supersession, resurrection of a tombstoned path, and a brand
  new chain. It updates all link rows (of any source version) that name the path:
  bare targets from pages of the same project; `project:path` targets (and
  `workspace/project:path`, or no workspace meaning the source's workspace) that
  name this page's project; and, when the written page is `default/_global`,
  also `_global:path` rows stored without a workspace. A content-identical
  rewrite creates no version and repoints nothing.
- No migration is needed. Existing link rows are plain references, so older
  stores need no rewrite.

## Who writes them

- **People**, in a page's frontmatter. Wiki files are plain markdown: edit the
  file and the file watcher reindexes the page. A periodic reconciliation pass
  (every 30 seconds) catches events the watcher missed. The reindex command
  rebuilds the store from the markdown, so edges come back from the files. See
  [wiki-files-layer.md](wiki-files-layer.md) and [cli.md](cli.md).
- **The public write tool and admin endpoint**, through the `relations` field
  of the write metadata. Validation is as described under Limits.
- **The consolidator**, sparingly. Both single-page consolidation and
  multi-page consolidation (`multi_page=true` in the consolidate tool) can
  record a relation when the session's evidence states it plainly, for example
  a fix that landed for a documented gotcha, or new evidence that contradicts a
  stored decision. The output has three fixed fields, one per relation. Only
  non-empty fields are written, and other keys are dropped. The write does not
  check that a target exists, so a missing target is stored unresolved. Empty
  relation fields are omitted from frontmatter, and older outputs without
  `relations` still work. See [consolidation.md](consolidation.md).

## How lint uses them

Lint treats declared edges with rules only. No model call is involved, and
this part runs whether or not the model-based passes are enabled.

- **`contradicts` edges** from latest pages of the project each produce one
  warning finding of kind `contradiction`, naming the source and target paths.
  - If the target resolves, the message asks the author to reconcile the pages
    or supersede the outdated one.
  - If the target does not resolve, the message says the declaration is stale
    because the target was deleted or renamed.
  - The finding stays until the edge is removed or the pages are reconciled.
- **Unresolved cross-project edges**, of any relation, produce a warning
  finding of kind `broken_link`. The message says whether the project is
  missing or only the page is. Unresolved edges within the same project are not
  reported by this check.
- The detector for likely contradictions (similarity band over stored
  embeddings) is separate. It never creates or persists an edge. See
  [consolidation.md](consolidation.md).

## How retrieval uses them

- **Search**. Pages already found by text, entity or vector search are expanded
  by one hop in both directions, over every label, including plain links. The
  expansion is fused with the other streams by reciprocal rank fusion, with
  constant 60. The label does **not** change ranking.
- **Search explanations** report the label of an expansion hop in the `edge`
  field when the label is not `references`. The field lives inside the hit's
  `graph_via` object (`seed_path`, `direction` = `outgoing` or `incoming`,
  `edge`); `edge` is omitted for plain links.
- **Related walk** (page read with related depth). Depth is 1 by default and
  is clamped to at most 3. At most 50 related pages are returned, each page
  appears once, and the seed page is never returned. The walk follows every
  label.
- **Page link listings** show each linked page once per direction. A page
  linked both by a typed edge and a plain link appears once.
- **Status** reports, per relation, how many typed links come from latest pages.
  This lives only in the administrative status (`GET /admin/status`, section
  `derived`, field `typed_links_from_latest_pages`, a list of `[relation, count]`
  pairs ordered by relation name) and in the CLI `status`; the MCP
  `memory_status` tool does not carry it. The count is store-wide (all
  workspaces and projects), counts every link row whose label is not
  `references` and whose source row is a latest version (expired pages
  included), and omits relations with zero rows.

There is no relation-specific ranking weight, and no traversal of fix or cause
chains beyond one hop in search. Both are deferred until an evaluation justifies
them. See [design-hindsight-borrowings.md](design-hindsight-borrowings.md) for
the live contradiction count and the deferred items.
