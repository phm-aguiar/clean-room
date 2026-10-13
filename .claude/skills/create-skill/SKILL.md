---
name: create-skill
description: Creates a new Claude Code SKILL.md from scratch or migrates an existing skill from another harness (Cursor, Continue, Copilot, generic prompts, legacy .claude/commands/) to the Claude Code standard. Applies quality criteria: proper frontmatter, concise declarative body, dynamic context injection, and organized assets. Use when asked to "create a skill", "write a SKILL.md", "migrate this prompt to a skill", or "convert this command file".
when_to_use: Use when the user wants to author a new /slash-command skill, package a recurring workflow as a skill, or port an existing prompt file from Cursor (.cursorrules, .mdc), Continue (.continue/prompts/), Copilot (copilot-instructions.md), or any other harness format into a Claude Code SKILL.md.
argument-hint: "[skill-name | path/to/existing-file]"
allowed-tools: Read Bash Write Edit
---

# create-skill

Creates or migrates a skill to the Claude Code quality standard.

Read `${CLAUDE_SKILL_DIR}/assets/skill-template.md` before proceeding — it is the canonical output template.

## Arguments

`$ARGUMENTS` is either:
- A new skill name (e.g. `summarize-pr`) → **create mode**
- A file path to an existing prompt/command/skill → **migrate mode**

If `$ARGUMENTS` is empty, ask the user which mode they want before proceeding.

---

## Create Mode

1. Ask the user (one message, all questions together):
   - What does the skill do? (1 sentence)
   - What would a user type to trigger it? (example phrases)
   - Does it write files, run commands, commit, deploy, or send messages? (yes → side effects)
   - What CLI tools or Claude tools does it need? (for `allowed-tools`)
   - Does it take arguments? If yes, what are they?
   - Should it run in an isolated subagent? (research/analysis tasks)
   - Scope: personal (`~/.claude/skills/`) or project (`.claude/skills/`)?

2. Identify dynamic context needs: if the skill will need git state, file contents, env info, or command output, plan `!`-injection commands.

3. Fill the template from `assets/skill-template.md`.

4. Write the skill to `<scope>/<skill-name>/SKILL.md`.

5. Create `assets/` subdirectory and any supporting files referenced in the body.

---

## Migrate Mode

1. Read the source file at `$ARGUMENTS`.

2. Identify source format:
   - **Cursor** — `.cursorrules`, `.mdc` (MDC with `---` fenced frontmatter), `.cursor/rules/`
   - **Continue** — `.continue/prompts/*.md`, `config.json` slash commands
   - **Copilot** — `.github/copilot-instructions.md`, workspace `.yml`
   - **Legacy Claude** — `.claude/commands/*.md` (no frontmatter)
   - **Generic** — plain text or markdown prompt file

3. Extract from the source:
   - Core intent → `description` + `when_to_use`
   - Step-by-step instructions → body (rewrite as declarative, strip narration)
   - Variables or `{{placeholders}}` → `arguments` field + `$name` substitutions
   - Context the skill needs (files, git, env) → `!`-injection commands
   - Side effects → `disable-model-invocation: true` if present
   - Required tools → `allowed-tools`

4. Apply the Quality Rules below.

5. Fill the template from `assets/skill-template.md`.

6. Ask the user for target scope (personal vs project) and skill name if not clear from source.

7. Write to `<scope>/<skill-name>/SKILL.md`.

---

## Quality Rules

Apply to every skill, whether new or migrated:

- **`description`** uses phrases the user would literally type, not paraphrases of the skill's internal logic.
- **Body < 500 lines.** Move reference material to `assets/` files and link from `SKILL.md`.
- **Declarative body.** State WHAT to do, not WHY or how Claude works internally. Remove all sentences beginning with "This skill…", "Use this to…", "This helps you…".
- **Dynamic context via `!`-injection.** If the skill needs file content, git state, or command output at invocation time, use `` !`command` `` instead of asking Claude to run those commands inside the skill body.
- **`disable-model-invocation: true`** whenever the skill writes files, commits, deploys, sends messages, or makes API calls.
- **`allowed-tools`** lists every tool explicitly called in the body. Use `Bash(pattern *)` syntax to scope broad permissions.
- **`context: fork`** for research, review, or analysis tasks that should run without conversation history.
- **`argument-hint`** whenever the skill takes positional arguments.
- **One task per skill.** If the source does multiple unrelated things, propose splitting.

---

## Output

After writing to disk:
1. Print the created path(s)
2. Show the final frontmatter fields and a one-line rationale for each non-obvious choice
3. Call out any quality compromises made (e.g., source was too verbose to fully condense) and why

---

## Gold Standard: Progressive Disclosure Pattern

When a skill has extensive reference material (guides, templates, examples, or diagrams), organize assets using **Progressive Disclosure** — show only what's essential in SKILL.md body, with links to detailed guides on demand.

### Structure

```
skill-name/
├── SKILL.md
│   ├── Frontmatter (with metadata/version/compatibility)
│   ├── One-liner context
│   ├── Quick Start (3–4 steps max)
│   ├── Common Tasks (collapsible <details> sections)
│   ├── Reference Index (tables linking to assets)
│   └── Integration & Best Practices
│
└── assets/
    ├── references/
    │   ├── style-guide.md (main reference, can be large)
    │   ├── specialized-guides/ (topic-specific, short reads)
    │   ├── diagrams/ or examples/ (templates, exemplars)
    │   └── current-rendering.md (version/platform notes)
    │
    ├── templates/
    │   ├── _index.md (quick selector table)
    │   └── template-*.md (individual templates)
    │
    └── examples/
        └── example-*.md (real-world samples)
```

### SKILL.md Body Pattern

1. **Quick Start** (5 min max)
   - Identify the primary use case
   - Link to relevant reference or template
   - Example output

2. **Common Tasks** (collapsible `<details>` sections)
   - Real user questions (FAQ-style)
   - Each expands to show a tip or link
   - No more than 5–6 sections

3. **Reference Index** (scannable tables)
   - One table per category (guides, diagrams, templates)
   - Link text → `assets/path/file.md`
   - Brief one-liner what each file covers

4. **Integration** (if applicable)
   - Links to related skills
   - Workflow context

5. **Best Practices Checklist**
   - Verification steps before shipping

### Asset Organization Rules

- **Each guide is self-contained** — can be read independently
- **Links use relative paths** — `assets/references/diagram-type.md`
- **Index files** (e.g., `_index.md`) map users to the right asset
- **Keep SKILL.md under 500 lines** — move everything else to `assets/`
- **No duplication** — if content is in `assets/references/guide.md`, don't repeat it in SKILL.md
- **Backwards-compat link from old location** — if renaming an existing skill, keep old asset names as redirects or symbolic links

### What Goes Where

| Content | Location | Why |
|---------|----------|-----|
| Frontmatter, triggers, arguments | `SKILL.md` | User needs it to invoke |
| Quick start (essential steps only) | `SKILL.md` | User needs immediate help |
| FAQ-style answers to real questions | `SKILL.md` `<details>` | User finds common issues without diving into guides |
| Complete style guide, reference | `assets/references/` | Comprehensive; linked on demand |
| Exemplar, template, sample | `assets/templates/` or `assets/examples/` | Copy-paste starting point |
| Version notes, platform-specific | `assets/references/current-rendering.md` | Separate, searchable |
| Taxonomy of diagram/template types | `assets/references/type-index.md` | Scannable decision tree |

### Example: markdown-writing

- **SKILL.md**: Quick start (choose doc type), common tasks (FAQ), reference index (3 tables)
- **assets/references/markdown-style-guide.md**: Complete guide (700+ lines)
- **assets/references/markdown-syntax/**: 5 focused guides (text, lists, links, code, tables)
- **assets/references/mermaid-diagram-types.md**: Scannable index + decision tree
- **assets/references/diagrams/**: 23 type-specific guides
- **assets/templates/_index.md**: Quick selector + file naming conventions
- **assets/templates/**: 9 individual templates
- **assets/examples/**: Auditable example outputs

Result: Users land on SKILL.md, answer "what do I need?" in <1 min, jump to relevant asset.
