# Lists

## Unordered Lists

Using hyphens, asterisks, or plus signs:

```markdown
- Item 1
- Item 2
- Item 3

* Item 1
* Item 2

+ Item 1
+ Item 2
```

## Ordered Lists

```markdown
1. First item
2. Second item
3. Third item
```

Numbers don't need to be sequential:

```markdown
1. First item
1. Second item
1. Third item
```

## Nested Lists

Indent nested items (2 spaces for bullets, 3 spaces for numbered):

```markdown
- Item 1
  - Nested item 1.1
  - Nested item 1.2
- Item 2
  - Nested item 2.1

1. First item
   1. Nested item 1.1
   2. Nested item 1.2
2. Second item
```

## Task Lists (GitHub Flavored Markdown)

Checkboxes for tracking:

```markdown
- [x] Completed task
- [ ] Incomplete task
- [ ] Another unchecked task
```

## Mixed Lists

Combine ordered and unordered:

```markdown
1. First step
   - Sub-step A
   - Sub-step B
2. Second step
   - Sub-step A
```

## List Formatting Rules

- **Parallel structure** — keep items grammatically consistent
- **No period at end** unless items are complete sentences
- **Max nesting: 2 levels** — restructure if you need deeper nesting
- **No blank lines** between list items (unless items are multi-paragraph)
- **Consistency** — pick one marker type per list (all `-` or all `*`, not mixed)
