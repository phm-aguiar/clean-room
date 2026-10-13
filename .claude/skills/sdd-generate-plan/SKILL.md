---
name: sdd-generate-plan
description: Generate plan.md from existing spec.md with 4 canonical sections (metadata, contracts, ADRs, constitution audit).
when_to_use: After sdd-brainstorm/generate-spec. Use when spec.md exists and user wants the architectural plan.
argument-hint: "[specs/features/<id>-<slug>]"
allowed-tools: Read Write
---

# Generate Architectural Plan

Create canonical 4-section plan.md from feature spec.

## Quick Start

**Step 1: Load feature sources**

Read: `spec.md` (functionality, BDD scenarios), `tech.md` (stack), `constitution.md` (constraints).

**Step 2: Generate 4 canonical sections**

- **Metadata:** Stack, feature source path, scope summary
- **Contracts & Boundaries:** APIs, events, schemas (if spec mentions them)
- **ADRs:** ≥1 formalized decision with justification + rejected alternatives
- **Constitution Audit:** Checklist against all rules in `constitution.md`

**Step 3: Confirm and save**

Show summary (stack, # of ADRs, audit items). Confirm before writing to `specs/features/<id>-<slug>/plan.md`.

---

## Reference Index

### Plan Sections

| Section | Purpose | Min requirement |
|---|---|---|
| Metadata | Stack, feature ref, scope | 1-2 sentences |
| Contracts & Boundaries | Formal APIs/events/schemas | "None at this stage" if absent |
| ADRs | Architecture decisions | ≥1 decision with alternatives |
| Constitution Audit | Compliance checklist | All rules from constitution.md |

### ADR Format

```markdown
- **Decision:** what was chosen
  - **Rationale:** why this choice
  - **Alternative Rejected:** what was considered and why not
```

---

## Best Practices Checklist

- [ ] `plan.md` written to `specs/features/<id>-<slug>/`
- [ ] 4 canonical sections present
- [ ] ≥1 ADR formalized with alternatives
- [ ] All `constitution.md` rules listed in audit
- [ ] Placeholders `{{...}}` preserved if not fillable
- [ ] User approved before saving

---

## Additional Resources

- `assets/architecture-patterns.md` — decision tree, Go patterns by domain, anti-patterns to avoid
