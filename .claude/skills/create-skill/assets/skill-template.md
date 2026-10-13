# Skill Output Template

Canonical structure for a Claude Code `SKILL.md`. Fill every section; remove optional fields that don't apply. **Do not leave placeholder text in the final file.**

---

## Frontmatter

```yaml
---
# REQUIRED — what the skill does and when to trigger it (max 1536 chars combined)
description: <one clear sentence of what it does, plus example phrases the user would say>

# OPTIONAL — extra trigger phrases; appended to description for Claude's invocation decision
when_to_use: <additional phrases and example requests that should activate this skill>

# OPTIONAL — shown in / autocomplete
argument-hint: <[arg-name] or [arg1] [arg2]>

# OPTIONAL — named positional args for $name substitution in the body
arguments: <space-separated names or YAML list>

# REQUIRED when skill writes files, commits, deploys, sends messages, or calls external APIs
disable-model-invocation: true

# OPTIONAL — hide from / menu; skill stays in Claude's context as background knowledge
user-invocable: false

# OPTIONAL — pre-approve tools so Claude doesn't prompt the user per-use
allowed-tools: <Read Write Edit Bash(git *) Bash(gh *)>

# OPTIONAL — execute in isolated subagent (no conversation history)
context: fork
agent: <Explore | Plan | general-purpose | custom-agent-name>

# OPTIONAL — override model when skill is active
model: <inherit | claude-sonnet-4-6 | claude-haiku-4-5-20251001>

# OPTIONAL — override effort level when skill is active
effort: <low | medium | high | xhigh | max>
---
```

---

## Body

```markdown
# <Skill Title>

<One sentence of non-obvious context ONLY if there is a hidden constraint or invariant not captured in the description.>

## Arguments (include only if skill takes arguments)

- `$ARGUMENTS` — <what the full argument string means>
- `$0` — <first positional arg>
- `$1` — <second positional arg>

## Context (include only if skill needs runtime data)

- Current branch: !`git rev-parse --abbrev-ref HEAD`
- Staged diff: !`git diff --cached`
- <Add other !`command` injections as needed>

## Instructions

<Numbered, declarative steps. WHAT to do — no narration, no WHY, no "this helps you".>

1. <Action>
2. <Action>
3. <Action>

## Additional Resources (include only if assets directory exists)

- See `${CLAUDE_SKILL_DIR}/assets/<file>.md` for <what it contains and when to load it>
```

---

## Directory Structure

```
<skill-name>/
├── SKILL.md                        # Entry point (required)
└── assets/
    ├── <reference>.md              # Reference material loaded on demand
    └── <template>.md               # Output template for Claude to fill
```

---

## Quality Checklist

Verify before writing to disk:

- [ ] `description` uses phrases the user would literally type, not paraphrases
- [ ] `disable-model-invocation: true` present when any side effect exists
- [ ] `allowed-tools` lists every tool mentioned in the body
- [ ] Body is under 500 lines; excess material moved to `assets/`
- [ ] No sentences beginning with "This skill…", "Use this to…", "This is used when…"
- [ ] Dynamic context uses `!`-injection, not "run this command and tell me the output"
- [ ] `context: fork` set if the task is research, review, or analysis
- [ ] `argument-hint` set if the skill accepts positional arguments
- [ ] One task per skill; split if the source does multiple unrelated things
