---
name: sdd-validate
description: Audit SDD compliance of repository. Check .github/memory/, specs/, and each feature (spec.md, plan.md, tasks.md). Emit PASS/FAIL/WARN report with suggested actions. Read-only.
when_to_use: Before PR or to verify repository follows SDD structure. Check for missing artifacts and health. Read-only operation; no files created or modified.
context: fork
allowed-tools: Read Bash
---

# Validate SDD Structure

Audit repository SDD conformance and emit health report.

## Quick Start

**Step 1: Check global context memory**

Verify:
- `.github/memory/` exists?
- `constitution.md` exists and not empty?
- `tech.md` exists and not empty?

Report: PASS (exists + non-empty), FAIL (absent), WARN (exists but empty).

**Step 2: Check specs/ tree**

Verify `specs/` and subdirs: `domain-events/`, `features/`, `infra/`.
Empty `features/` = WARN (acceptable in new repo).

**Step 3: Validate each feature**

For each `specs/features/<id>-<name>/`:
- Does name follow pattern `<id-numeric>-<name>`? If not → WARN
- `spec.md` exists, non-empty, has canonical sections?
- `plan.md` exists, non-empty, has 4 sections?
- `tasks.md` exists, non-empty, has DAG format?

**Step 4: Emit report**

Show PASS/FAIL/WARN status for each check with suggested actions per FAIL.

---

## Reference Index

### Status Levels

| Status | Meaning | Action |
|---|---|---|
| PASS | Exists, non-empty, conforms | None needed |
| FAIL | Absent or malformed | Must fix before merge |
| WARN | Exists but empty, in progress, or non-standard naming | Suggest fix; acceptable to continue |

### Validation Checklist

| Item | Check |
|---|---|
| `.github/memory/` | Directory exists |
| `constitution.md` | File exists + non-empty |
| `tech.md` | File exists (can be empty initially) |
| `specs/` | Directory exists |
| `specs/domain-events/` | Directory exists |
| `specs/features/` | Directory exists |
| `specs/infra/` | Directory exists |
| Feature naming | Pattern: `<numeric-id>-<slug>` |
| Feature spec.md | Exists + canonical sections present |
| Feature plan.md | Exists + 4 sections (metadata, contracts, ADRs, audit) |
| Feature tasks.md | Exists + DAG format (Tnnn blocks) |
| `CLAUDE.md` | Exists + contains "## SDD Workflow" section |

---

## Common Tasks

<details>
<summary><strong>How should the report be formatted?</strong></summary>

```
SDD Validation Report
=====================
.github/memory/            PASS
  constitution.md          PASS
  tech.md                  WARN (empty — run /sdd-explore-tech)
specs/                     PASS
  domain-events/           PASS
  features/                WARN (empty — acceptable, add first feature with /sdd-brainstorm)
    001-chat-core/
      spec.md              PASS
      plan.md              FAIL (missing — run /sdd-generate-plan)
      tasks.md             WARN (in progress)
CLAUDE.md                  PASS
─────────────────────────────
Result: 7/11 checks passed
Suggested: Run /sdd-generate-plan for 001-chat-core
```

</details>

<details>
<summary><strong>Feature directory doesn't follow naming. What's the error?</strong></summary>

WARN: "Directory name should follow pattern `<numeric-id>-<slug>`. Found: `my-feature`. Rename or continue with awareness."

</details>

<details>
<summary><strong>Can I run validation during active development?</strong></summary>

Yes. WARN status (empty sections, in-progress tasks) is expected. Only FAIL blocks merge readiness.

</details>

---

## Best Practices Checklist

- [ ] Report uses PASS/FAIL/WARN format
- [ ] Each FAIL has suggested action
- [ ] Each WARN has severity note
- [ ] No files created or modified (read-only)
- [ ] Report generated in under 30 seconds

---

## Additional Resources

- `assets/content-quality.md` — freshness scoring, coverage metrics, thresholds
- `assets/check-sdd.sh` — automation script for structural validation (optional)
