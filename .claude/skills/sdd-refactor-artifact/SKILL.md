---
name: sdd-refactor-artifact
description: Reorganize existing SDD artifacts (spec.md, plan.md, tasks.md, CLAUDE.md, llms.txt) to canonical format. Preserves all original content—only reorganizes sections.
when_to_use: Use when existing SDD artifact does not follow canonical format and needs restructuring. Different from sdd-generate-plan/tasks which create from zero.
argument-hint: "[path/to/artifact.md]"
allowed-tools: Read Write Edit
---

# Refactor SDD Artifact

Normalize existing SDD artifact to canonical format.

## Quick Start

**Step 1: Identify artifact type**

By filename:

| File | Type | Template |
|---|---|---|
| `spec.md`, `spec-*.md` | Functional spec | spec.md |
| `plan.md`, `plan-*.md` | Architectural plan | plan.md |
| `tasks.md`, `tasks-*.md` | Execution matrix | tasks.md |
| `CLAUDE.md` | Agent guidelines | CLAUDE.md |
| `llms.txt` | LLM navigation | llms.txt |

If unclear, ask user which template to apply.

**Step 2: Read and map content**

Read target file completely. Extract semantically: headings, paragraphs, lists, tables, `{{...}}` placeholders.

Map each section to canonical template sections (see `assets/canonical-templates.md`). Content without canonical section → move to "## Additional Notes".

**Step 3: Present diff and confirm**

Show summary: renamed sections, moved sections, added sections. **Always ask before saving.** Never overwrite without permission.

---

## Reference Index

### Canonical Templates

| Artifact | Sections | Canonical Ref |
|---|---|---|
| spec.md | Metadata, Purpose, Scope, Expected Behavior, Approach, Constraints, Success Criteria, Dependencies | `assets/canonical-templates.md` |
| plan.md | Metadata, Contracts & Boundaries, ADRs, Constitution Audit | `assets/canonical-templates.md` |
| tasks.md | Metadata, Task DAG (Tnnn blocks), DAG Validation, Coordination Graph (if LATTE) | `assets/canonical-templates.md` |
| CLAUDE.md | SDD Workflow section (refactored); rest preserved as-is | `assets/canonical-templates.md` |

---

## Common Tasks

<details>
<summary><strong>Artifact is already canonical. What happens?</strong></summary>

Report idempotency: "Artifact already conforms to template. No changes needed." Do not modify.

</details>

<details>
<summary><strong>CLAUDE.md has non-SDD content. What do I refactor?</strong></summary>

Only the `## SDD Workflow` section is refactored. All other content remains untouched. Idempotency: if no SDD Workflow section exists, add it; if it exists, normalize it.

</details>

<details>
<summary><strong>I want to preserve a section that's not in the template.</strong></summary>

Move it to `## Additional Notes` at the end. No content is discarded; only reorganized.

</details>

---

## Best Practices Checklist

- [ ] Target file has same semantic content (±5% for cosmetic changes)
- [ ] All original sections appear in new version
- [ ] Placeholders `{{...}}` preserved exactly
- [ ] If CLAUDE.md, non-SDD sections identical
- [ ] User approved diff before saving

---

## Additional Resources

- `assets/canonical-templates.md` — canonical templates and section-mapping rules
