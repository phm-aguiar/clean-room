# Collapsible Sections

Create expandable/collapsible content using HTML `<details>` and `<summary>`:

## Basic Syntax

```markdown
<details>
<summary>Click to expand</summary>

This content is hidden by default.
It expands when you click the summary.

</details>
```

**Important:** Blank line after `<summary>` tag for markdown to render correctly.

## With Emoji and Bold

```markdown
<details>
<summary><strong>💬 Speaker Notes</strong></summary>

- Key talking point one
- Transition to next topic
- **Bold** emphasis works here
- [Links](https://example.com) work too

</details>
```

## Multiple Sections

```markdown
<details>
<summary><strong>📋 Section 1</strong></summary>

Content for section 1

</details>

<details>
<summary><strong>📋 Section 2</strong></summary>

Content for section 2

</details>
```

## Nesting Details

```markdown
<details>
<summary><strong>Parent</strong></summary>

Outer content

<details>
<summary>Nested details</summary>

Inner content

</details>

</details>
```

## Common Patterns

### Speaker Notes
```markdown
<details>
<summary><strong>💬 Speaker Notes</strong></summary>

- Pause here for questions
- Next talking point
- Transition to slide

</details>
```

### Implementation Details
```markdown
<details>
<summary><strong>🔧 Implementation Details</strong></summary>

Technical details about the code...

```python
code example here
```

</details>
```

### Raw Data / Logs
```markdown
<details>
<summary><strong>📊 Raw Data</strong></summary>

```
Full output or data dump here
```

</details>
```

### Background Context
```markdown
<details>
<summary><strong>💡 Background</strong></summary>

Historical context or additional information that helps
but isn't essential to understanding the main content.

</details>
```

## Styling with Horizontal Rules

Add visual separation after collapsible sections:

```markdown
<details>
<summary><strong>Details</strong></summary>

Content here

</details>

---

Next section starts here
```

## Browser Support

- ✅ Modern browsers (Chrome, Firefox, Safari, Edge)
- ✅ GitHub (full support)
- ✅ GitLab (full support)
- ✅ Most markdown renderers
- ⚠️ Some platforms may not support HTML

Always test your document on the target platform.

## Accessibility

- **Descriptive summary labels** — `Speaker Notes`, `Implementation`, not just `Details`
- **Semantic HTML** — use `<details>/<summary>` not `<div>` wrappers
- **Clear indication** — users should understand content is collapsible
- **Don't hide critical info** — only use for supplementary content
