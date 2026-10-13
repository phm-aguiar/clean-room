---
name: sdd-brainstorm
description: Interactive feature discovery with mandatory wiki/code cross-reference, generates Discovery Report (ADR+PRD hybrid) with Gherkin scenarios and quality gate ≥20/25.
when_to_use: Entry point of SDD pipeline. Use when discussing feature ideas, exploring requirements, or starting discovery.
argument-hint: "[feature-idea]"
allowed-tools: Read Write Bash AskUserQuestion
---

# SDD Brainstorm

Interactive discovery of SDD features with quality gates and Gherkin scenario coverage.

## Quick Start

**Step 1: Explore context**

Load `.github/memory/tech.md` and `constitution.md`. List existing features in `specs/features/` to identify next ID and related patterns.

**Step 2: Conduct cross-reference search**

Before interviewing, scan wiki index (if available) and codebase for relevant patterns. Document findings to inject into discovery and reference during interview.

**Step 3: Interview and generate Discovery Report**

Run interactive interview via `AskUserQuestion` using dimensions from `assets/interview-dimensions.md`. Collect success/failure scenarios. Fill `assets/discovery-template.md`, calculate quality score ≥20/25, present for user approval, then signal `/sdd-generate-spec`.

---

## Common Tasks

<details>
<summary><strong>How do I structure a discovery interview?</strong></summary>

Load `assets/interview-dimensions.md` for the question framework. Rules:
- One question per `AskUserQuestion` — never stack topics
- Always offer multiple choice + "Other" option
- Reference wiki/code findings during interview
- **Mandatory: ask for 2+ failure scenarios before concluding**
- Collect at least 3 success scenarios, 2 failure scenarios, 1 edge case

</details>

<details>
<summary><strong>What makes a Gherkin scenario complete?</strong></summary>

Each scenario must have:
- **Given** — initial state
- **When** — user action
- **Then** — observable outcome
- **And** — side effects (if any)

Success scenarios: happy path + variations (min 3).
Failure scenarios: input validation, external service failure, permission denial (min 2).
Edge cases: concurrency, volume, timeouts (min 1).

See `assets/discovery-template.md` for the canonical feature block format.

</details>

<details>
<summary><strong>How is quality score calculated?</strong></summary>

| Dimension | Max | Criterion |
|---|---|---|
| Completeness | 5 | All sections filled, no TODO/TBD |
| Gherkin coverage | 5 | ≥3 success + ≥2 failure + ≥1 edge case |
| Ambiguity | 5 | No vague terms without explicit definition |
| Debt surface | 5 | Technical debts listed with concrete mitigation |
| Wiki alignment | 5 | Patterns consulted and implications documented |

**Pass threshold: 20/25.** If < 20, loop: show breakdown, ask clarifying questions, update discovery.md, recalculate.

See `assets/quality-gates.md` for detailed scoring rules.

</details>

<details>
<summary><strong>What if a discovery already exists?</strong></summary>

Ask the user: "Refine existing or start fresh?" If refining, load existing discovery and loop back to interview for clarification on low-scoring dimensions.

</details>

---

## Reference Index

### Discovery Process

| File | Purpose | Use case |
|---|---|---|
| [Discovery Template](assets/discovery-template.md) | ADR+PRD hybrid format | Fill all sections from interview |
| [Quality Gates](assets/quality-gates.md) | Scoring rubric | Calculate score, identify gaps |
| [Interview Dimensions](assets/interview-dimensions.md) | Question framework | Conduct structured interview |

### Gherkin Specification

**Feature Header:**
```gherkin
Feature: <descriptive title>
  Como <user role>
  Quero <capability>
  Para <business value>

  Background:
    Dado que <shared precondition>
```

**Scenario blocks (min totals: 3 success + 2 failure + 1 edge):**
- Success: happy path, variations
- Failure: invalid input, external service, permission denied
- Edge case: concurrency, volume, timeout

See `assets/discovery-template.md` for full exemplar.

---

## Best Practices Checklist

Before declaring discovery ready for `/sdd-generate-spec`:

- [ ] `reports/<id>-<slug>-discovery.md` created
- [ ] Quality score ≥20/25 achieved
- [ ] ≥3 success scenarios, ≥2 failure scenarios, ≥1 edge case in Gherkin
- [ ] All discovery sections filled (no TODO/TBD)
- [ ] Wiki/codebase cross-reference documented
- [ ] Technical debts listed with concrete mitigation
- [ ] User gave explicit approval
- [ ] Next step (`/sdd-generate-spec`) communicated

---

## Integration with SDD Pipeline

**Position:** Start → **brainstorm** → generate-spec → generate-plan → generate-tasks → execute

- **Input:** feature idea (user prompt)
- **Output:** `reports/<id>-<slug>-discovery.md` (approved)
- **Next:** `/sdd-generate-spec` reads discovery and creates `spec.md`

---

## Additional Resources

- `assets/discovery-template.md` — template for Discovery Report
- `assets/quality-gates.md` — scoring dimensions and thresholds
- `assets/interview-dimensions.md` — structured interview questions
