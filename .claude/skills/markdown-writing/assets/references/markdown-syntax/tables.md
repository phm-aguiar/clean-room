# Tables

## Basic Table

```markdown
| Column 1 | Column 2 | Column 3 |
|----------|----------|----------|
| Data 1   | Data 2   | Data 3   |
| Data 4   | Data 5   | Data 6   |
```

## Alignment

Control column alignment with colons:

```markdown
| Left | Center | Right |
|:-----|:------:|------:|
| Left | Center | Right |
| A    | B      | C     |
```

- **Left**: `|:---|` (default)
- **Center**: `|:---:|`
- **Right**: `|---:|`

## Minimal Syntax

Pipes not required at start/end (though cleaner with them):

```markdown
Column 1 | Column 2 | Column 3
---------|----------|----------
Data 1   | Data 2   | Data 3
Data 4   | Data 5   | Data 6
```

## Inline Formatting

You can use markdown formatting inside tables:

```markdown
| Feature | Status | Notes |
|---------|--------|-------|
| Login | ✅ Complete | Uses **OAuth** |
| API | 🚧 In Progress | See [docs](./api.md) |
| Export | ❌ Not Started | `Planned for v2` |
```

## Complex Tables

With longer content and nesting:

```markdown
| Feature | Free | Pro | Enterprise |
|---------|:----:|:---:|:-----------:|
| Users | 5 | 50 | Unlimited |
| Storage | 1 GB | 100 GB | Custom |
| Support | Community | Email | 24/7 Phone |
| SLA | None | 99.5% | 99.99% |
```

## Table Best Practices

- **Header row always** — don't create headerless tables
- **Left-align text** — `|---|`
- **Right-align numbers** — `|---:|` for numeric columns
- **Concise cells** — 1–5 words per cell
- **Not for complex layouts** — max 5 columns for mobile readability
- **Avoid nesting** — don't put tables inside tables
