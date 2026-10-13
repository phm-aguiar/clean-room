# Phase 4: Verdict Report

Template for comparing current implementation vs. isolated design.

---

## Scored Comparison

### Correctness
Does each implementation satisfy confirmed obligations?

| Obligation | Current | Alternative | Gap evidence |
|-----------|---------|-------------|--------------|
| [obl1] | ✓ met | ✓ met | — |
| [obl2] | ✓ met | ✗ unmet | Would fail when [scenario] |
| [obl3] | ? unclear | ✓ met | Current code has ambiguous branch |

### Simplicity
Moving parts and concepts to maintain.

| Axis | Current | Alternative | Difference |
|------|---------|-------------|-----------|
| Branches | N | M | [if higher, which make sense?] |
| Dependencies | [list] | [list] | Added/removed what? |
| Concepts | [list] | [list] | Clearer to [audience]? |

### Robustness
Failure modes and state corruption risks.

| Failure mode | Current | Alternative | Risk reduction |
|--------------|---------|-------------|----------------|
| Timeout during write | partial update (risky) | atomic fail-or-succeed | eliminates corruption |
| Credential missing | crash (noisy) | graceful degradation | improves resilience |

### Testability
Deterministic seams and controllable I/O.

| Seam | Current | Alternative | Improvement |
|------|---------|-------------|------------|
| Dependency injection | hardcoded | parameter | enables mocking |
| Clock access | system time | injectable | enables time travel in tests |
| Random seed | unseeded | seedable | reproducible randomness |

### Change risk
Migration size, reversibility, dependency risk.

| Risk | Current → Alternative |
|------|----------------------|
| Migration size | [LOC changed, files affected] |
| Reversibility | [what survives backwards?] |
| Dependency risk | [new libs, versions, breakage surface] |
| Deployment | [canary window, rollback time] |

---

## Behavior Resolution

Every behavior appearing only in current implementation must be classified:

| Behavior | Classification | Evidence | Consequence |
|----------|---|----------|---|
| [behavior A] | Missed obligation | Not mentioned in brief but present in tests | Must add to brief and include in redesign |
| [behavior B] | Accidental (bug) | Defensive code, commented "workaround for Y" | Safe to drop in alternative |
| [behavior C] | Unresolved (open Q) | Unclear from tests or comments | Designer made assumption [A]. Confirm with user? |
| [behavior D] | Dead code | Unreachable branch (condition never true) | Safe to remove |

---

## Gate Application

Each proposed change must pass:

1. **Preserves obligations?** ✓ / ✗
2. **Creates observable win?** ✓ / ✗ → [correctness | robustness | testability | delegation]
3. **Benefit > migration risk?** ✓ / ✗
4. **Verifiable + rollback boundary?** ✓ / ✗

**Result**: All gates pass → change clears gate.

---

## Verdict

### Replace
✓ The alternative clears the gate for the whole unit and wins on axes that matter.

**Recommendation**:
1. Phase 1: [migration step] — verify [criteria]
2. Phase 2: [migration step] — verify [criteria]
3. [N more phased steps]
4. Rollback: [revert to previous deployment]

**Risks**: [dependency introduction, latency change, compatibility]
**Verification**: [how to validate correctness]

### Graft
✓ One or more independent parts clear the gate. Keep the rest.

**Keep current**: [reason why it wins]
**Replace part**: [isolated change]
  - Gate: passes
  - Verification: [test seam]
  - Rollback: [revert just this piece]

**Why partial?**: [whole alternative fails on [axis], but [part] can move independently]

### Keep
✗ No proposed change clears the gate.

**Current implementation wins because**:
- [it meets all obligations and alternative doesn't]
- [migration risk outweighs correctness gain]
- [unresolved questions make redesign unsafe]

**Evidence**:
- [gate failures preventing change]
- [risks that exceed benefit]

---

## Open Questions & Evidence

What would change this verdict?

| Question | Current assumption | If answer is X... | Evidence needed |
|----------|-------------------|-------------------|-----------------|
| [Q1] | [current impl does Y] | verdict shifts to [Replace/Graft/Keep] | [user confirm, decision record, new test] |
| [Q2] | [alternative assumes Z] | means [consequence] | [measurement, acceptance test result] |

---

## Migration Plan (if Replace or Graft)

### Rollback strategy
- **Window**: [N minutes with both live; or N hour canary]
- **Trigger**: [what symptom rolls back automatically?]
- **Manual rollback**: [1-step command or process]
- **Data**: [any migration reversal needed?]

### Deployment
- **Size**: [which files, estimate LOC]
- **Dependencies**: [new versions, new libs, removal]
- **Compat**: [which Go/Rust/Python versions?]
- **Performance**: [expected p50, p95, p99 before/after]

---

## Recommendation Summary

| Axis | Verdict |
|------|---------|
| **Correctness** | [Replace/Graft/Keep] — all obligations [met/unmet/unclear] |
| **Simplicity** | [Replace/Graft/Keep] — [simpler/equivalent/complex] by N concepts |
| **Robustness** | [Replace/Graft/Keep] — [new failure class eliminated / adds risk / no change] |
| **Testability** | [Replace/Graft/Keep] — [N seams added / equivalent / lost] |
| **Risk** | [Replace/Graft/Keep] — [low / medium / high] migration + [low / medium / high] dependency |

**Final verdict**: [Replace | Graft | Keep]

**Confidence**: [high | medium | low] — [why]

---

## Sign-off

- Trace completed: ✓ / ✗
- Brief sanitized: ✓ / ✗
- Designer isolation verified: ✓ / ✗
- Obligations marked: ✓ / ✗
- Dependencies vetted: ✓ / ✗
- Seams identified: ✓ / ✗

**Ready for implementation** if all ✓.
