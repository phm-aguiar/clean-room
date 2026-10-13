# Phase 1: Trace Report

Template for documenting current implementation behavior.

## Target Unit
- **Name**: [function/module/system name]
- **Location**: [path:line or package]
- **Language**: [Go/Rust/Python/etc]
- **Purpose**: [1 sentence]

---

## Entry Points & Callers

### Public interface
- [exported function signature]
- [receiver/context]

### Callers (internal)
- `path/file.go:line` — [call context]
- `path/file.go:line` — [call context]

### Callers (external)
- [package/module name] — [how used]

---

## Boundary Data

### Input parameters
| Name | Type | Nullability | Constraints |
|------|------|-------------|-------------|
| param1 | type | optional/required | [units, ranges, format] |

### Return values
| Name | Type | Nullability | Meaning |
|------|------|-------------|---------|
| result | type | optional/required | [what it represents] |

### Side effects
- Global state modified: [what, when]
- I/O: [files, network, database calls]
- Logging: [when logged, what format]
- Randomness: [where, for what]

---

## Control Flow

### Branch 1: [condition]
- Entry: [when taken]
- Operations: [what happens]
- Exit: [return value or exception]
- Evidence: [test name, code line]

### Branch 2: [condition]
- [same structure]

### Error paths
- **Panic/Exception A**: [when, what data lost]
- **Panic/Exception B**: [when, partial write risk]

---

## Dependencies & Interfaces

### Direct dependencies
- `package.Function` — [what for]
- `database.Query` — [what data]

### Dependents
- `caller1/module.go` — [how depended on]
- `caller2/module.go` — [how depended on]

### Runtime constraints
- Concurrency: [thread-safe? mutex? channel?]
- Deployment: [where runs, scaling model]
- Performance: [SLA, timeout, throughput requirement]
- Compatibility: [Go version, stdlib assumptions]

---

## Current Tests

| Test name | Coverage | Seams exercised | Hidden state |
|-----------|----------|-----------------|--------------|
| TestBranch1 | branch A | [dependency mocks] | [fixtures] |
| TestBranch2 | branch B | [dependency mocks] | [fixtures] |

---

## Behavior Classification

### Confirmed requirements
- `[file:line]` — [observed behavior] — Evidence: [test name, doc ref, issue #, user confirmation]

### Accidental behaviors
- `[file:line]` — [bug/workaround/unreachable] — Evidence: [why accidental]

### Unresolved questions
- Does [behavior] matter to callers? — Evidence: [gap, ambiguity]
- Is [edge case] reachable? — Evidence: [unclear from code/tests]

---

## Scar Hunts

Examine for hidden obligations:

### Comments
- "TODO: fix X" at [line] — Confirmed or accidental?
- "workaround for Y" at [line] — Still needed?

### Flags & config
- Feature flag `DEBUG_MODE` — when used, why
- Config `MAX_RETRIES` — defaults, overrides

### Git history
- Commit [hash] added defensive check for [bug] — still relevant?
- Blame shows [person] added fallback — why?

### Test names
- `TestBrokenX` or `TestSkipWhen_Y` — what's the story?

---

## Essential vs. Incidental

### Must cross the wall (essential)
- [outcome users depend on]
- [guarantee about consistency/atomicity]
- [observable contract]

### Can stay behind (incidental)
- Internal data structure (HashMap vs. Vec)
- Variable names
- Specific library (tokio vs. async-std)
- File layout

---

## Unresolved behaviors

List any where evidence is inconclusive:

| Behavior | Impact if assumed wrong | Evidence needed |
|----------|------------------------|-----------------|
| [behavior] | [redesign consequence] | [user confirmation / acceptance test / decision record] |

---

## Next: Phase 2 Brief

Based on this trace, phase 2 will create a sanitized brief with:
- Essential outcomes only (no impl details)
- Confirmed obligations (from this report)
- Open questions (from unresolved list)
- No file paths, symbols, or library names
