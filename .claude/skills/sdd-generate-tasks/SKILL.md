---
name: sdd-generate-tasks
description: Generate tasks.md with atomic task DAG from spec.md and plan.md. Each task has role (researcher|analyst|executor), dependencies, parallelizability, files, and wiki-keywords. Supports experiential memory (--with-memory) and LATTE coordination.
when_to_use: After sdd-generate-plan. Use when spec.md (Approved: true) and plan.md exist and user wants the execution matrix. Hard gate applies.
argument-hint: "[specs/features/<id>-<slug>] [--with-memory]"
allowed-tools: Read Write Bash
---

# Generate Task DAG

Create canonical atomic task matrix with dependency resolution and parallelism detection.

## Quick Start

**Step 1: Verify approval gate**

Read `spec.md`. If `Approved: false`, abort. Hard gate: spec must be approved before task generation.

**Step 2: Extract task ingredients**

From spec.md + plan.md (+ discovery.md if linked):
- Extract domain names, technical patterns, components, error scenarios
- Build global keyword list for wiki searches
- Map each ingredient to one atomic task (never combine actions)
- Assign 2-5 wiki-keywords to each task based on role (researcher/analyst/executor)

**Step 3: Phase-by-phase interaction and DAG validation**

Never generate all phases at once. For each phase:
1. Propose tasks with role, dependencies, parallelism
2. Ask user: "Approve", "Adjust", "Add", "Remove"
3. Validate DAG: no cycles, no broken deps, no unintended orphans
4. Confirm and write to `specs/features/<id>-<slug>/tasks.md`

---

## Reference Index

### Task Structure

```markdown
- [ ] **Tnnn:** Task description
  - **Role:** researcher | analyst | executor
  - **Dependencies:** Txxx, Tyyy | None
  - **Parallelizable:** true | false
  - **Files:** `path/to/file.go`
  - **Wiki-Keywords:** keyword1, keyword2, keyword3
```

### Task Roles

| Role | Responsibility | Typical Keywords |
|---|---|---|
| researcher | Explore domain patterns, prior art, wiki | Domain names, concepts, scenarios |
| analyst | Audit architecture, review patterns, compliance | ADR topics, patterns, constraints |
| executor | Implement, modify, test code | Components, file paths, error handling |

### Gherkin → Tasks Mapping

Each Gherkin scenario generates 1+ tasks:
- **Success scenario** → 1 executor task (e.g., "Implement: happy path main flow")
- **Failure scenario** → 1 executor task (e.g., "Implement error handling: invalid input")
- **Edge case scenario** → 1 executor task (e.g., "Implement: concurrent requests")

Plus supporting tasks: specs, docs, configuration, testing.

---

## Common Tasks

<details>
<summary><strong>How do I ensure tasks are atomic?</strong></summary>

**Never combine actions.** "Create X and test Y" → 2 tasks.

One task = one deliverable = one file path (or file group if related). If a task touches multiple unrelated files, split it.

</details>

<details>
<summary><strong>Parallel tasks: how do I know which ones run together?</strong></summary>

**Hard rule:** Parallel tasks must have disjoint file sets. If they share a file path, they must be sequential.

Exception: `.feature` and `.go` files in the same directory can run in parallel (BDD specs don't conflict with implementation).

</details>

<details>
<summary><strong>DAG validation: what makes a valid DAG?</strong></summary>

1. **No cycles** — DFS check; if back-edge found, report and ask user to break it
2. **No broken dependencies** — every reference in `Dependencies:` must exist
3. **No unintended orphans** — task with no deps + no dependents should be intentional (ask if not)
4. **File isolation** — `Parallelizable: true` tasks must have disjoint file sets

</details>

<details>
<summary><strong>What does --with-memory do?</strong></summary>

Searches experiential memory for similar features (if available). Injects top-5 chunks as "Hints from prior features" before deriving tasks. Helps avoid reinventing patterns. If index unavailable, continues without hints.

</details>

---

## Best Practices Checklist

- [ ] Approval gate verified (`Approved: true`)
- [ ] `tasks.md` written to `specs/features/<id>-<slug>/`
- [ ] Each task has: Role, Dependencies, Parallelizable flag, Files, Wiki-Keywords
- [ ] No empty Wiki-Keywords fields (min 1 keyword per task)
- [ ] DAG validated: no cycles, no broken deps, file isolation correct
- [ ] Gherkin scenario coverage: each success + failure + edge case has corresponding task(s)
- [ ] Failure scenarios have separate error-handling tasks
- [ ] Phase-by-phase interaction completed with user approval
- [ ] Parallelizable tasks confirmed to have disjoint file sets

---

## Integration with SDD Pipeline

**Position:** brainstorm → generate-spec → generate-plan → **generate-tasks** → execute

- **Input:** `spec.md` (Approved: true) + `plan.md` (+ optional discovery.md)
- **Output:** `specs/features/<id>-<slug>/tasks.md` with atomic DAG
- **Next:** execute (individual agents pick tasks based on role)

---

## Advanced: LATTE Coordination Mode

If tasks.md frontmatter has `graph-operators: enabled`, add:
- YAML: `heartbeat-threshold`, `max-rounds`
- Section: `## Coordination Graph` with table: ID | Agent | Dependencies | Status

See `assets/latte-task-rules.md` for rules. Legacy mode (static DAG) is default.

---

## Additional Resources

- `assets/task-rules.md` — atomicity rules, canonical phases, examples
- `assets/latte-task-rules.md` — LATTE coordination graph rules
