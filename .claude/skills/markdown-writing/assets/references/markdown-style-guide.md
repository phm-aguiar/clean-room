<!-- Source: https://github.com/SuperiorByteWorks-LLC/agent-project | License: Apache-2.0 | Author: Clayton Young / Superior Byte Works, LLC (Boreal Bytes) -->

# Markdown Style Guide

> **For AI agents:** Read this file for all core formatting rules. When creating any markdown document, follow these conventions for consistent, professional output. When a template exists for your document type, start from it — see [Templates](#templates).
>
> **For humans:** This guide ensures every markdown document in your project is clean, scannable, well-cited, and renders beautifully on GitHub. Reference it from your `AGENTS.md` or contributing guide.

**Target platform:** GitHub-style Markdown; verify the exact host and export pipeline.
**Reviewed:** 2026-10-11 against [CommonMark 0.31.2](https://spec.commonmark.org/0.31.2/),
[GFM](https://github.github.com/gfm/), and current GitHub formatting documentation.

Heading counts, emoji, separators, and citation placement below are optional house style.
They are not Markdown conformance rules and do not override a requested format or an
existing project/journal style. Tables and task lists are GFM extensions; footnotes,
Mermaid, HTML details, and generated heading anchors depend on the host.
**Design goal:** Clear, professional documents that communicate effectively through consistent structure, meaningful formatting, proper citations, and strategic use of diagrams.

---

## Quick Start for Agents

1. **Identify the document type** → Check if a [template](#templates) exists
2. **Structure first** → Heading hierarchy, then content
3. **Apply formatting from this guide** → Headings, text, lists, tables, images, links
4. **Add citations** → Footnote references for all claims and sources
5. **Consider diagrams** → Would a [Mermaid diagram](mermaid_style_guide.md) communicate this better than text?
6. **Add collapsible sections** → For supplementary detail, speaker notes, or lengthy context
7. **Verify** → Run through the [quality checklist](#quality-checklist)

---

## Core Principles

| #   | Principle                         | Rule                                                                                                                                                                                       |
| --- | --------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------ |
| 1   | **Answer before they ask**        | Anticipate reader questions and address them inline. A great document resolves doubts as they form — the reader finishes with no lingering "but what about...?"                            |
| 2   | **Scannable first**               | Readers skim before they read. Use headings, bold, and lists to make the structure visible at a glance.                                                                                    |
| 3   | **Cite everything**               | Every claim, statistic, or external reference gets a footnote citation with a full URL. No orphan claims.                                                                                  |
| 4   | **Diagrams over walls of text**   | If a concept involves flow, relationships, or structure, use a [Mermaid diagram](mermaid_style_guide.md) alongside the text.                                                               |
| 5   | **Generous with information**     | Don't hide the details — surface them. Use collapsible sections for depth without clutter, but never omit information because "they probably don't need it." If it's relevant, include it. |
| 6   | **Consistent structure**          | Same heading hierarchy, same formatting patterns, same emoji placement across every document.                                                                                              |
| 7   | **One idea per section**          | Each heading should cover one topic. If you're covering two ideas, split into two headings.                                                                                                |
| 8   | **Professional but approachable** | Clean formatting, no clutter, no decorative noise — but not stiff or academic. Write like a senior engineer explains to a colleague.                                                       |

---

## Document Structure

### Title and metadata

Every document starts with exactly one H1 title, followed by a brief context line and a separator:

```markdown
# Document Title Here

_Brief context — project name, date, or purpose in one line_

---
```

- **One H1 per document** — never more
- Context line in italics — what this document is, when, and for whom
- Horizontal rule separates metadata from content

### Heading hierarchy

| Level | Syntax          | Use                     | Max per document    |
| ----- | --------------- | ----------------------- | ------------------- |
| H1    | `# Title`       | Document title          | **1** (exactly one) |
| H2    | `## Section`    | Major sections          | 4–10                |
| H3    | `### Topic`     | Topics within a section | 2–5 per H2          |
| H4    | `#### Subtopic` | Subtopics when needed   | 2–4 per H3          |
| H5+   | Never use       | —                       | 0                   |

**Rules:**

- **Never skip levels** — don't jump from H2 to H4
- **Sentence case** — `## 📋 Project overview` not `## 📋 Project Overview` (exception: proper nouns)
- **Descriptive headings** — `### Authentication flow` not `### Details`

---

## Text Formatting

### Bold, italic, code

| Format     | Syntax       | When to use                                   | Example                             |
| ---------- | ------------ | --------------------------------------------- | ----------------------------------- |
| **Bold**   | `**text**`   | Key terms, important concepts, emphasis       | **Primary database** handles writes |
| _Italic_   | `*text*`     | Definitions, titles, subtle emphasis          | The process is called _sharding_    |
| `Code`     | `` `text` `` | Technical terms, commands, file names, values | Run `npm install` to install        |
| ~~Strike~~ | `~~text~~`   | Deprecated content, corrections               | ~~Old approach~~ replaced by v2     |

**Rules:**

- **Bold sparingly** — if everything is bold, nothing is. Max 2–3 bold terms per paragraph.
- **Don't combine** bold and italic (`***text***`) — pick one
- **Code for anything technical** — file names (`README.md`), commands (`git push`), config values (`true`), environment variables (`NODE_ENV`)
- **Never bold entire sentences** — bold the key word(s) within the sentence

### Blockquotes

Use blockquotes for definitions, callouts, and important notes:

```markdown
> **Definition:** A _load balancer_ distributes incoming network traffic
> across multiple servers to ensure no single server bears too much demand.
```

For warnings and callouts:

```markdown
> ⚠️ **Warning:** This operation is destructive and cannot be undone.

> 💡 **Tip:** Use `--dry-run` to preview changes before applying.

> 📌 **Note:** This requires admin permissions on the repository.
```

- Prefix with emoji + bold label for typed callouts
- Keep blockquotes to 1–3 lines
- Don't nest blockquotes (`>>`)

---

## Lists

### When to use each type

| List type | Syntax       | Use when                                  |
| --------- | ------------ | ----------------------------------------- |
| Bullet    | `- item`     | Items have no inherent order              |
| Numbered  | `1. step`    | Steps must happen in sequence             |
| Checkbox  | `- [ ] item` | Tracking completion (agendas, checklists) |

### Formatting rules

- **Indent nested items under the parent item text** — commonly 2 spaces after `- ` and 3 after `1. `. Marker width matters; a universal 2-space rule breaks ordered lists
- **Parallel structure** — every item in a list should have the same grammatical form
- **No period at end** unless items are full sentences
- **Keep items concise** — if a bullet needs a paragraph, it should be a sub-section instead
- **Max nesting depth: 2 levels** — if you need a third level, restructure

```markdown
✅ Good — parallel structure, concise:

- Configure the database connection
- Run the migration scripts
- Verify the schema changes

❌ Bad — mixed structure, verbose:

- You need to configure the database
- Migration scripts
- After that, you should verify that the schema looks correct
```

---

## Links and Citations

### Inline links

```markdown
See the [Mermaid Style Guide](mermaid_style_guide.md) for diagram conventions.
```

- **Meaningful link text** — `[Mermaid Style Guide]` not `[click here]` or `[link]`
- **Relative paths** for internal links — resolve them from the containing Markdown file, including copied templates
- **Full URLs** for external links — always `https://`

### Footnote citations

Cite external factual claims with sources that support them. Use footnotes if the
destination supports them; otherwise use inline/reference links or the required citation
style. A citation must support the claim, not merely point to a related page. Explicitly
label synthetic data and distinguish it from measurements; never invent references.

```markdown
Markdown was created by John Gruber in 2004 as a lightweight
markup language designed for readability[^1]. GitHub adopted
Mermaid diagram support in February 2022[^2].

[^1]: Gruber, J. (2004). "Markdown." _Daring Fireball_. https://daringfireball.net/projects/markdown/

[^2]: GitHub Blog. (2022). "Include diagrams in your Markdown files with Mermaid." https://github.blog/2022-02-14-include-diagrams-markdown-files-mermaid/
```

**Citation format:**

```
[^N]: Author/Org. (Year). "Title." *Publication*. https://full-url
```

**Rules:**

- **Number sequentially** — `[^1]`, `[^2]`, `[^3]` in order of appearance
- **Full URL always included** — the reader must be able to reach the source
- **Group all footnotes at the document bottom** — under a `## References` section or at the very end
- **Every external claim needs one** — statistics, quotes, methodologies, tools mentioned
- **Internal project links don't need footnotes** — use inline links instead

### Reference-style links (for repeated URLs)

When the same URL appears multiple times, use reference-style links to keep the text clean:

```markdown
The [official docs][mermaid-docs] cover all diagram types.
See [Mermaid documentation][mermaid-docs] for the full syntax.

[mermaid-docs]: https://mermaid.js.org/ 'Mermaid Documentation'
```

---

## Images and Figures

### Placement and syntax

```markdown
![Descriptive alt text for screen readers](images/architecture_overview.png)
_Figure 1: System architecture showing the three-tier deployment model_
```

**Rules:**

- **Inline with content** — place images where they're relevant, not in a separate "Images" section
- **Descriptive alt text** — `![Three-tier architecture diagram]` not `![image]` or `![screenshot]`
- **Italic caption below** — `*Figure N: What this image shows*`
- **Number figures sequentially** — Figure 1, Figure 2, etc. if multiple images
- **Relative paths** — `images/file.png` not absolute paths
- **Reasonable file sizes** — compress PNGs, use SVG where possible

### Image naming convention

```
{document-slug}_{description}.{ext}

Examples:
  auth_flow_overview.png
  deployment_architecture.svg
  api_response_example.png
```

### When NOT to use an image

If the content could be expressed as a **Mermaid diagram**, prefer that over a static image:

| Scenario                   | Use                                        |
| -------------------------- | ------------------------------------------ |
| Architecture diagram       | Mermaid `flowchart` or `architecture-beta` |
| Sequence/interaction       | Mermaid `sequenceDiagram`                  |
| Data model                 | Mermaid `erDiagram`                        |
| Timeline                   | Mermaid `timeline` or `gantt`              |
| Screenshot of UI           | Image (Mermaid can't do this)              |
| Photo / real-world image   | Image                                      |
| Complex data visualization | Scientific plotting with source data/code            |

See the [Mermaid Style Guide](mermaid_style_guide.md) for diagram type selection and styling.

---

## Tables

### When to use tables

- **Structured comparisons** — features, options, tradeoffs
- **Reference data** — configuration values, API parameters, status codes
- **Schedules and matrices** — timelines, responsibility assignments

### When NOT to use tables

- **Narrative content** — use paragraphs instead
- **Simple lists** — use bullet points
- **More than 5 columns** — becomes unreadable on mobile; restructure

### Formatting

```markdown
| Feature | Free Tier | Pro Tier | Enterprise |
| ------- | --------- | -------- | ---------- |
| Users   | 5         | 50       | Unlimited  |
| Storage | 1 GB      | 100 GB   | Custom     |
| Support | Community | Email    | Dedicated  |
```

**Rules:**

- **Header row always** — no headerless tables
- **Left-align text columns** — `|---|` (default)
- **Right-align number columns** — `|---:|` when appropriate
- **Concise cell content** — 1–5 words per cell. If you need more, it's not a table problem
- **Bold key column** — the first column or the column the reader scans first
- **Consistent formatting within columns** — don't mix sentences and fragments

---

## Code Blocks

### Inline code

Use backticks for technical terms within prose:

```markdown
Run `git status` to check for uncommitted changes.
The `NODE_ENV` variable controls the runtime environment.
```

### Fenced code blocks

Always specify the language for syntax highlighting:

````markdown
```python
def calculate_average(values: list[float]) -> float:
    """Return the arithmetic mean of a list of values."""
    return sum(values) / len(values)
```
````

**Rules:**

- **Always include language identifier** — ` ```python `, ` ```bash `, ` ```json `, etc.
- **Use ` ```text ` for plain output** — not ` ``` ` with no language
- **Keep blocks focused** — show the relevant snippet, not the entire file
- **Add a comment if context needed** — `# Configure the database connection` at the top of the block

---

## Collapsible Sections

Use HTML `<details>` for supplementary content that shouldn't clutter the main flow — speaker notes, implementation details, verbose logs, or optional deep-dives.

```markdown
<details>
<summary><strong>💬 Speaker Notes</strong></summary>

- Key talking point one
- Transition to next topic
- **Bold** emphasis works inside details
- [Links](https://example.com) work too

</details>

---
```

**Rules:**

- **Collapsed by default** — the `<details>` tag collapses automatically
- **Descriptive summary** — `<strong>💬 Speaker Notes</strong>` or `<strong>📋 Implementation Details</strong>`
- **Blank line after `<summary>` tag** — required for markdown to render inside the block
- **Optional house style:** follow with `---` if the extra separator improves clarity
- **GitHub supports Markdown inside details** with blank lines around the Markdown content. Other renderers may sanitize HTML or leave it unparsed; preview the exported result

### Common collapsible patterns

| Summary label         | Use for                                          |
| --------------------- | ------------------------------------------------ |
| 💬 **Speaker Notes**  | Presentation talking points, timing, transitions |
| 📋 **Details**        | Extended explanation, verbose context            |
| 🔧 **Implementation** | Technical details, code samples, config          |
| 📊 **Raw Data**       | Full output, logs, data tables                   |
| 💡 **Background**     | Context that helps but isn't essential           |

---

## Horizontal Rules

Use `---` (three hyphens) for visual separation:

```markdown
---
```

**When to use:**

- **After every `</details>` block** — mandatory, creates clear separation
- **After title/metadata** — separates document header from content
- **Between major sections** — when an H2 heading alone doesn't create enough visual break
- **Before footnotes/references** — separates content from citation list

**When NOT to use:**

- Between every paragraph (too busy)
- Between H3 sub-sections within the same H2 (use whitespace instead)

---

## Approved Emoji Set

Use emoji sparingly in body text for callouts and emphasis only.

### Section headings

| Emoji | Use for                                |
| ----- | -------------------------------------- |
| 📋    | Overview, summary, agenda, checklist   |
| 🎯    | Goals, objectives, outcomes, targets   |
| 📚    | Content, documentation, main body      |
| 🔗    | Resources, references, links           |
| 📍    | Agenda, navigation, current position   |
| 🏠    | Housekeeping, logistics, announcements |
| ✍️    | Tasks, assignments, action items       |

### Status and outcomes

| Emoji | Meaning                              |
| ----- | ------------------------------------ |
| ✅    | Success, complete, correct, approved |
| ❌    | Failure, incorrect, avoid, rejected  |
| ⚠️    | Warning, caution, important notice   |
| 💡    | Tip, insight, idea, best practice    |
| 📌    | Important, key point, remember       |
| 🚫    | Prohibited, do not, blocked          |

### Technical and process

| Emoji | Meaning                           |
| ----- | --------------------------------- |
| ⚙️    | Configuration, settings, process  |
| 🔧    | Tools, utilities, setup           |
| 🔍    | Analysis, investigation, review   |
| 📊    | Data, metrics, analytics          |
| 📈    | Growth, trends, improvement       |
| 🔄    | Cycle, refresh, iteration         |
| ⚡    | Performance, speed, quick action  |
| 🔐    | Security, authentication, privacy |
| 🌐    | Web, API, network, global         |
| 💾    | Storage, database, persistence    |
| 📦    | Package, artifact, deployment     |

### People and collaboration

| Emoji | Meaning                             |
| ----- | ----------------------------------- |
| 👤    | User, person, individual            |
| 👥    | Team, group, collaboration          |
| 💬    | Discussion, comments, speaker notes |
| 🎓    | Learning, education, knowledge      |
| 🤔    | Question, consideration, reflection |

### Emoji rules

1. **One per H2 heading** at the start — `## 📋 Overview`
2. **None in H3/H4** — keep sub-headings clean
3. **Sparingly in body text** — for callouts (`> ⚠️ **Warning:**`) and key markers only
4. **Never in**: titles (H1), code blocks, link text, table data cells
5. **No decorative emoji** — 🎉 💯 🔥 🎊 💥 ✨ add noise, not meaning
6. **Consistency** — same emoji = same meaning across all documents in the project

---

## Mermaid Diagram Integration

Use Mermaid for structural relationships when it improves understanding. Use data-driven plotting for quantitative figures that need exact geometry, uncertainty, or a journal-specific format.

### When to add a diagram

If a diagram helps the reader, use the table to select a type and verify host support:

1. **Read the [Mermaid Style Guide](mermaid_style_guide.md) first** — emoji, color palette, accessibility, complexity management
2. **Then open the specific type file** — exemplar, tips, template, complex example

See [Reference Index: Mermaid Diagram Types](mermaid-diagram-types.md) for the full mapping.

---

## Whitespace and Spacing

- **Blank line between paragraphs** — always
- **Blank line before and after headings** — always
- **Blank line before and after code blocks** — always
- **Blank line before and after blockquotes** — always
- **No blank line between list items** — keep lists tight
- **No trailing whitespace** — clean line endings
- **One blank line at end of file** — standard convention
- **No more than one consecutive blank line** — two blank lines = too much space

---

## Quality Checklist

### Structure

- [ ] Exactly one H1 title
- [ ] Heading hierarchy is correct (H1 → H2 → H3 → H4, no skips)
- [ ] Horizontal rules after title metadata and after every `</details>` block

### Content

- [ ] Every external claim has a footnote citation
- [ ] All footnotes have full URLs
- [ ] All links tested and working
- [ ] Meaningful link text (no "click here")
- [ ] Bold used for key terms, not entire sentences
- [ ] Code formatting for all technical terms

### Visual elements

- [ ] Images have descriptive alt text
- [ ] Images have italic figure captions
- [ ] Images placed inline with related content (not in separate section)
- [ ] Tables have header rows and consistent formatting
- [ ] Mermaid diagrams considered where applicable (with `accTitle`/`accDescr`)

### Collapsible sections

- [ ] `<details>` blocks have descriptive `<summary>` labels
- [ ] Blank line after `<summary>` tag (for markdown rendering)
- [ ] Horizontal rule `---` after every `</details>` block
- [ ] Content inside collapses renders correctly

### Polish

- [ ] No spelling or grammar errors
- [ ] Consistent whitespace (no trailing spaces, no double blanks)
- [ ] Parallel grammatical structure in lists
- [ ] Renders correctly in GitHub light and dark mode

---

## Attribution & License

This style guide is ported from the `SuperiorByteWorks-LLC/agent-project` repository under the Apache-2.0 License.

- **Source**: https://github.com/SuperiorByteWorks-LLC/agent-project
- **Author**: Clayton Young / Superior Byte Works, LLC (@borealBytes)
- **License**: Apache-2.0
- **Contributors**: K-Dense Team (integration and community feedback)
