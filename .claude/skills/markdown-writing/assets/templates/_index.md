# Document Templates Index

Choose a template that matches your document type. Each template enforces the markdown style guide and provides pre-built structure.

## Template Selection

| Document Type | Template | Best For | Audience |
|---|---|---|---|
| **Technical Docs** | [project-documentation.md](project-documentation.md) | Software/product documentation, README, getting started | Developers, users |
| **Decisions** | [decision-record.md](decision-record.md) | Architecture Decision Record (ADR), RFC | Team, stakeholders |
| **Analysis** | [research-paper.md](research-paper.md) | Research, analysis, findings with heavy citations | Researchers, analysts |
| **Change Records** | [pull-request.md](pull-request.md) | PR documentation, change inventory, rollback plan | Reviewers, team |
| **Tracking** | [issue.md](issue.md) | Bug reports, feature requests, reproduction steps | Team, QA |
| **Status Updates** | [status-report.md](status-report.md) | Progress updates, risk summaries, decisions needed | Leadership, stakeholders |
| **Guides** | [how-to-guide.md](how-to-guide.md) | Step-by-step instructions, tutorials, procedures | End users, teams |
| **Presentations** | [presentation.md](presentation.md) | Slide deck, briefing, speaker notes | Audience, presenters |
| **Project Tracking** | [kanban.md](kanban.md) | Sprint board, work items, WIP limits | Team, product |

## Quick Start

1. Copy the template file that matches your document type
2. Fill in all sections following the structure
3. Apply [markdown-style-guide.md](../references/markdown-style-guide.md) rules
4. Add Mermaid diagrams where appropriate (see [mermaid-diagram-types.md](../references/mermaid-diagram-types.md))
5. Run through the quality checklist in the style guide

## File Conventions

For documents that accumulate over time:

| Type | Directory | Naming | Example |
|---|---|---|---|
| Pull Requests | `docs/prs/` | `pr-NNNNNNNN-short-desc.md` | `pr-00000123-fix-auth.md` |
| Issues | `docs/issues/` | `issue-NNNNNNNN-short-desc.md` | `issue-00000456-add-export.md` |
| Kanban Boards | `docs/kanban/` | `{scope}-{id}-short-desc.md` | `sprint-2026-w07-modernize.md` |
| Decision Records | `docs/decisions/` | `NNN-{slug}.md` | `001-use-postgresql.md` |
| Status Reports | `docs/status/` | `status-{date}.md` | `status-2026-02-14.md` |

## When NOT to Use a Template

- The project has its own established format (follow the project conventions)
- A different style guide is required (journal, conference, etc.)
- Custom markup or tooling is needed

In those cases, still reference [markdown-style-guide.md](../references/markdown-style-guide.md) for general principles.

## Additional Resources

- [Markdown Style Guide](../references/markdown-style-guide.md) — core formatting rules
- [Mermaid Style Guide](../references/mermaid_style_guide.md) — diagram styling
- [Mermaid Diagram Types](../references/mermaid-diagram-types.md) — which diagram to use
- Specific diagram guides: `diagrams/` directory (flowchart, sequence, etc.)
