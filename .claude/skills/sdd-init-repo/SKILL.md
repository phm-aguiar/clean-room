---
name: sdd-init-repo
description: Initialize or adapt a repository for Spec-Driven Development (SDD). Creates .github/memory/ (constitution.md, tech.md), specs/ (domain-events, features, infra), and updates CLAUDE.md with SDD workflow section.
when_to_use: Entry point for SDD on new or existing repos. Use when user wants to begin SDD. Language-agnostic, works on empty repos.
argument-hint: "[--with-automation]"
allowed-tools: Read Write Bash
---

# Initialize SDD Repository

Create canonical SDD directory structure and configuration.

## Quick Start

**Step 1: Check current state**

List repo root. If `.github/memory/` or `specs/` exist, ask: preserve or recreate?

**Step 2: Create global context memory**

Load `assets/templates/constitution-template.md`. Create `.github/memory/constitution.md` with template content. Create `.github/memory/tech.md` as placeholder (reference `/sdd-explore-tech` for later population).

**Step 3: Create specs tree**

```bash
mkdir -p specs/domain-events specs/features specs/infra
```

**Step 4: Update CLAUDE.md**

Load `assets/sdd-workflow-claude-md.md`. If CLAUDE.md exists and already contains "## SDD Workflow" section, skip. Otherwise, merge block at end of file. If CLAUDE.md absent, create with block.

---

## Target Structure

```
<repo>/
├── .github/memory/
│   ├── constitution.md     ← Quality gates, constraints, anti-patterns
│   └── tech.md            ← Technology stack (populated by /sdd-explore-tech)
├── specs/
│   ├── domain-events/     ← Formal contracts (AsyncAPI, OpenAPI)
│   ├── features/          ← Numbered features (001-name/spec.md, plan.md, tasks.md)
│   └── infra/             ← Infrastructure declarations
└── CLAUDE.md              ← Updated with "## SDD Workflow" section
```

---

## Reference Index

### Template Files

| Template | Purpose | Editable by user |
|---|---|---|
| `constitution-template.md` | Quality gates, constraints | Yes (after init) |
| `spec-template.md` | Feature specification | `/sdd-generate-spec` |
| `plan-template.md` | Architectural plan | `/sdd-generate-plan` |
| `tasks-template.md` | Execution matrix | `/sdd-generate-tasks` |
| `sdd-workflow-claude-md.md` | CLAUDE.md block | Auto-merged |

---

## Common Tasks

<details>
<summary><strong>.github/memory/ already exists. What now?</strong></summary>

Ask: "Preserve existing and skip, or fully recreate?" Idempotency rule: never overwrite without permission.

</details>

<details>
<summary><strong>How do I fill constitution.md after init?</strong></summary>

After init, read the template. Customize quality gates, constraints, and anti-patterns specific to your project. User approval required for each rule.

Next: `/sdd-explore-tech` to populate tech.md, then `/sdd-brainstorm` for first feature.

</details>

---

## Best Practices Checklist

- [ ] `.github/memory/constitution.md` and `tech.md` exist
- [ ] `specs/{domain-events,features,infra}/` are directories
- [ ] `CLAUDE.md` contains "## SDD Workflow" section
- [ ] If existing dirs were preserved, user approved
- [ ] No hardcoded assumptions about project language/framework

---

## Integration with SDD Pipeline

**Position:** **init-repo** → explore-tech → brainstorm → generate-spec → generate-plan → generate-tasks → execute

- **Output:** Initialized SDD directory structure + templates
- **Next:** `/sdd-explore-tech` (populate tech.md) or directly to `/sdd-brainstorm`

---

## Additional Resources

- `assets/templates/constitution-template.md` — constitution.md template
- `assets/templates/spec-template.md` — spec.md template
- `assets/templates/plan-template.md` — plan.md template
- `assets/templates/tasks-template.md` — tasks.md template
- `assets/sdd-workflow-claude-md.md` — SDD Workflow block for CLAUDE.md
- `assets/scaffold-sdd.sh` — automation script (optional)
