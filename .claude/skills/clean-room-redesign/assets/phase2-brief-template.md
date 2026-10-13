# Phase 2: Sanitized Brief

Template for communicating unit to isolated designer.
**Zero implementation details. No file paths, symbols, org names, secrets.**

---

## Problem

What goes wrong if this capability doesn't exist?

[Describe user-facing impact or system failure, not mechanism.]

Example:
- Users cannot [action] and lose [data]
- System becomes inconsistent when [scenario]
- Performance degrades to unusable under [condition]

---

## Goal

Observable outcomes and guarantees. Not implementation strategy.

### Primary outcome
[What should happen when the capability works correctly?]

### Guarantees
- Atomicity: [does it succeed entirely or not at all?]
- Ordering: [must X always happen before Y?]
- Idempotency: [safe to retry? what if retried N times?]
- Durability: [data persists after what event?]
- Consistency: [invariants maintained?]

---

## Context

### Callers
[Generic description of who calls this.]
- A web handler receiving user requests
- A background job processing a queue
- An initialization routine that runs once at startup

### Callees
[Generic description of what this calls.]
- A configuration loader (key-value store, filesystem, database)
- A credential provider (environment, vault, agent)
- A database connector (with reconnect logic)

### Boundary data
[Generic types and constraints. Use language-neutral notation per guides.]

```text
INPUT: Request {
  user_id: string (UUIDs, lowercase)
  action: enum [create | read | update | delete]
  resource_id: string (optional, required if action != create)
  metadata: key-value pairs (max 10 entries, max 1KB total)
}

OUTPUT: Response {
  status: enum [success | conflict | not_found | permission_denied | timeout]
  result: object (structure depends on action)
  error: string (optional, human-readable message)
}

SIDE EFFECT: Audit log entry written
  - timestamp (UTC)
  - user_id
  - action
  - resource_id
  - outcome (success/failure)
```

---

## Hard Constraints

| Constraint | Immovable | Preference | Why |
|-----------|-----------|-----------|-----|
| Must complete within 200ms | ✓ immovable | — | SLA for user experience |
| No more than 5 retries | — | preferred | Resource exhaustion risk |
| Must not modify DB on read | ✓ immovable | — | Consistency guarantee |
| Use only stdlib, no new deps | — | preferred | Deployment simplicity |

---

## Confirmed Obligations

[From phase 1 trace, obligations with evidence.]

- **Obligation 1**: [outcome required by all callers]
  - Evidence: tests X, Y; user confirmation; decision record [ref]
  - Scope: always applies

- **Obligation 2**: [error handling requirement]
  - Evidence: issue #123; acceptance criteria doc
  - Scope: when [specific condition]

- **Obligation 3**: [performance requirement]
  - Evidence: SLA documented in [ref]; monitored by [metric name]
  - Scope: p95 latency must be < 200ms

---

## Open Questions

[From phase 1 unresolved list. Designer must decide but cannot ask you.]

- **Q1**: When [edge case], should the system [option A] or [option B]?
  - Evidence gap: no test; original code unclear
  - Consequence: affects robustness if wrong

- **Q2**: Is [behavior] a confirmed requirement or accidental?
  - Evidence gap: appears in test but not doc
  - Consequence: might redesign away if accidental

---

## Out of Scope

What this unit must NOT do:

- Must NOT modify [entity type]
- Must NOT call external API [service name]
- Must NOT change data retention policy
- Must NOT affect other modules' performance

---

## Done Looks Like

Externally observable acceptance criteria:

- [ ] When [scenario], [outcome] occurs within [time]
- [ ] When [error condition], system responds with [status], no [data] lost
- [ ] Audit trail records [what] for every [action]
- [ ] Concurrent calls don't [negative outcome]
- [ ] Under load [number of requests], latency stays < [time]

---

## Reference Material

- Language-neutral examples: See guide for pseudocode, JSON schema, state machines
- No specific library names, file paths, or code symbols in this brief
- Designer has access to [offline design | public web search]
- Designer will have these dependencies available: [list, if any from confirmation]

---

## Validation Checklist

Before sending to designer:

- [ ] No file paths (no `src/`, `internal/`, etc.)
- [ ] No symbol names (no `function_name`, `ClassName`, etc.)
- [ ] No library names (no `tokio`, `serde`, brand-specific APIs)
- [ ] No organization, product, or project names
- [ ] No secrets, credentials, or customer data
- [ ] Each obligation appears exactly once
- [ ] No accidental/unresolved behavior presented as requirement
- [ ] Every confirmed obligation testable/observable from outside
- [ ] At least two visibly different implementations could satisfy this
- [ ] Safe to web-search each obligation using just the brief
- [ ] Open questions don't reveal implementation details

**Pass all checks? → Ready for isolation.**

---

## Phase 3 Handoff

Designer receives:
1. This brief (no prior context)
2. Language-neutral guides (for pseudocode, JSON, etc.)
3. Optional: public documentation if already published

Designer will not receive:
- Current implementation code
- File paths or symbols
- Repository credentials
- Internal decision records
