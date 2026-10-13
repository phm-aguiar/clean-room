---
name: sdd-generate-spec
description: Derive spec.md from approved Discovery Report. Condense discovery into canonical spec sections, confirm with user, write to specs/features/<id>-<slug>/spec.md.
when_to_use: After sdd-brainstorm. Use when approved discovery.md exists in reports/ and user wants to advance to plan/tasks pipeline.
argument-hint: "[reports/<id>-<slug>-discovery.md]"
allowed-tools: Read Write Bash AskUserQuestion
---

# Generate Specification

Derive spec.md from approved Discovery Report.

## Quick Start

**Step 1: Verify discovery approval**

List `reports/` if user didn't provide path. Read discovery and check `Approved: true` in metadata. If `false`, abort and request approval from `/sdd-brainstorm`.

**Step 2: Extract discovery to spec mapping**

Map discovery sections to spec fields:
- Discovery metadata → spec metadata (+ link to discovery)
- Context + problem → purpose (synthesize, don't copy)
- Objectives → scope (in/out)
- Gherkin scenarios → behavior (natural language for happy path; preserve failure titles)
- Functional requirements → behavior (complements Gherkin)
- Architectural decision → approach + alternatives
- RNFs + constraints → constraints
- DoD → success criteria

**Step 3: Fill, review, and save**

Load `assets/spec-template.md`. Substitute `{{...}}` with extracted content. Preserve `Approved: false` (gate is later in generate-plan). Confirm with user and write to `specs/features/<id>-<slug>/spec.md`.

---

## Reference Index

### Discovery → Spec Mapping

| Discovery section | Spec field | Notes |
|---|---|---|
| Metadata (ID, slug, author, date) | Metadata | + add `Discovery Report:` link |
| Context + Problem + Users | Purpose | Synthesize 1-2 paras; no verbatim copy |
| Goals + Non-Goals | Scope | In/out lists (YAGNI) |
| Gherkin scenarios | Expected Behavior | Happy path in natural language; preserve failure scenario titles |
| Functional Requirements (Must) | Expected Behavior | Complements Gherkin |
| Architectural Decision | Chosen Approach | Include alternatives considered |
| RNF + Constraints | Constraints | Copy table directly |
| DoD | Success Criteria | Use as-is (already measurable + approved) |
| Cross-reference | Dependencies | File list + related patterns |
| Technical Debts | Note in Approach | Mention if relevant |

---

## Common Tasks

<details>
<summary><strong>Discovery approval is false. What do I do?</strong></summary>

Abort. Inform user that discovery must pass the approval gate in `/sdd-brainstorm` first. Quality score must be ≥20/25 and user must explicitly approve.

</details>

<details>
<summary><strong>How much should I condense the purpose section?</strong></summary>

**Purpose** should answer: "For whom, what, why?" in 1-2 paragraphs. Synthesize the essential from discovery context—don't copy verbatim.

For Gherkin scenarios in **Expected Behavior**, keep them condensed: happy path in prose + preserve failure scenario titles so reader knows they exist.

</details>

<details>
<summary><strong>Spec already exists. Regenerate?</strong></summary>

Ask: "Overwrite or abort?" Idempotency rule applies.

</details>

---

## Best Practices Checklist

- [ ] Discovery approval verified (`Approved: true`)
- [ ] All spec-template `{{...}}` placeholders substituted
- [ ] `Discovery Report:` link present in metadata
- [ ] `Approved: false` set in spec (gate is in `/sdd-generate-plan`)
- [ ] `specs/features/<id>-<slug>/spec.md` written
- [ ] Purpose synthesized (not copy-paste)
- [ ] User confirmed before saving
- [ ] Next step (`/sdd-generate-plan`) communicated

---

## Integration with SDD Pipeline

**Position:** brainstorm → **generate-spec** → generate-plan → generate-tasks → execute

- **Input:** approved `reports/<id>-<slug>-discovery.md`
- **Output:** `specs/features/<id>-<slug>/spec.md` (Approved: false)
- **Next:** `/sdd-generate-plan`

---

## Additional Resources

- `assets/spec-template.md` — canonical spec.md template
