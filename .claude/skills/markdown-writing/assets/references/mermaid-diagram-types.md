# Mermaid Diagram Types Reference

Quick index of all 23 Mermaid diagram types. Each has a dedicated guide file in `diagrams/` with production exemplars, tips, and templates.

## Diagram Type Selection Guide

**Pick the right type for your content:**

| Use Case | Diagram Type | File | Best For |
|----------|--------------|------|----------|
| **Workflows & Logic** | | | |
| Decision logic, process steps, workflow | Flowchart | `diagrams/flowchart.md` | Any step-by-step process |
| State transitions, entity lifecycle | State Diagram | `diagrams/state.md` | State machines, workflows |
| Requirements & traceability | Requirement | `diagrams/requirement.md` | Compliance, feature tracking |
| **Relationships & Structure** | | | |
| Data model, entity relationships | ER Diagram | `diagrams/er.md` | Database schemas, data models |
| Class hierarchy, type relationships | Class Diagram | `diagrams/class.md` | OOP design, interfaces |
| Multi-level system architecture | C4 Diagram | `diagrams/c4.md` | Zoom-level architecture |
| Cloud infrastructure, services | Architecture | `diagrams/architecture.md` | AWS/cloud, microservices |
| **Sequences & Timing** | | | |
| Service interactions, messaging | Sequence | `diagrams/sequence.md` | API calls, protocol flows |
| Chronological events, milestones | Timeline | `diagrams/timeline.md` | Project history, milestones |
| Project timeline, scheduling | Gantt | `diagrams/gantt.md` | Project management, roadmaps |
| Git strategy, branching | Git Graph | `diagrams/git_graph.md` | Release flows, git workflows |
| **Hierarchies & Composition** | | | |
| Concept hierarchy, brainstorm | Mindmap | `diagrams/mindmap.md` | Knowledge organization |
| Component layout, spatial arrange | Block | `diagrams/block.md` | Hardware, system layout |
| Binary protocol, data format | Packet | `diagrams/packet.md` | Network packets, protocols |
| **Data & Metrics** | | | |
| Proportions, parts of a whole | Pie | `diagrams/pie.md` | Composition, percentages |
| Numeric trends, bar/line charts | XY Chart | `diagrams/xy_chart.md` | Time series, comparisons |
| Flow magnitude, resource distribution | Sankey | `diagrams/sankey.md` | Energy, budget, data flow |
| Multi-dimensional comparison | Radar | `diagrams/radar.md` | Skills matrix, capabilities |
| Hierarchical proportions, budget | Treemap | `diagrams/treemap.md` | Budget breakdown, hierarchy |
| **Planning & User Flows** | | | |
| Work item status, task columns | Kanban | `diagrams/kanban.md` | Sprint boards, task tracking |
| User experience, satisfaction | User Journey | `diagrams/user_journey.md` | UX flows, customer journey |
| Two-axis comparison, priority | Quadrant | `diagrams/quadrant.md` | Prioritization matrix, 2x2 |
| **Advanced & Composition** | | | |
| Code-style sequences, syntax | ZenUML | `diagrams/zenuml.md` | Programming-style sequences |
| Multiple diagrams, complex patterns | Complex Examples | `diagrams/complex-examples.md` | Multi-diagram composition |

---

## Quick Decision Tree

**Start here to pick your diagram type:**

1. **Is it about timing or sequence?**
   - Sequential steps → **Flowchart**
   - Service interactions → **Sequence**
   - Chronological events → **Timeline**
   - Project schedule → **Gantt**

2. **Is it about structure or relationships?**
   - Database model → **ER**
   - Code structure → **Class**
   - System architecture → **C4** or **Architecture**
   - Hierarchy/brainstorm → **Mindmap**

3. **Is it about data or proportions?**
   - Parts of a whole → **Pie**
   - Numeric trends → **XY Chart**
   - Resource flow → **Sankey**
   - Skills/comparison → **Radar**
   - Budget/hierarchy → **Treemap**

4. **Is it about tracking or planning?**
   - Task status → **Kanban**
   - User flow → **User Journey**
   - Priority matrix → **Quadrant**
   - Git strategy → **Git Graph**

5. **Is it about a state or protocol?**
   - State machine → **State**
   - Requirements → **Requirement**
   - Network packet → **Packet**
   - Component layout → **Block**

---

## Reading the Guides

Each guide (`diagrams/*.md`) contains:

1. **When to use this type** — what it's good for
2. **When NOT to use it** — common mistakes
3. **Production exemplar** — a real, working example
4. **Type-specific tips** — syntax, styling, accessibility
5. **Copy-paste template** — ready to adapt
6. **Common variations** — how to extend it

Always read [Mermaid Style Guide](mermaid_style_guide.md) first for:
- Emoji conventions
- Color classes
- Accessibility (accTitle/accDescr)
- Theme-neutral styling
- Complexity management

---

## Accessibility for All Types

Supported types (add `accTitle` and `accDescr`):
- ✅ Flowchart, Sequence, State, ER, C4, Class, Architecture
- ✅ Mindmap, Timeline, User Journey, Quadrant, Requirement
- ✅ Sankey, Kanban, Gantt, Git Graph, ZenUML

Not supported (add italic description above code block):
- Pie, XY Chart, Radar, Treemap, Packet, Block

See [Current Rendering & Validation](current-rendering.md) for the full matrix.

---

## Mermaid Version Notes

- Guides tested on **Mermaid 12.0.0**
- Some types use `-beta` suffix (`radar-beta`, `xychart-beta`) — check GitHub support
- Rendering varies by platform/version — always test in your destination
- Keep `.md` source + rendered `.png` export together in version control

---

## Common Pitfalls

| Mistake | Fix |
|---------|-----|
| Using Flowchart for everything | Scan the guide above; timeline/sequence/etc are better fits |
| Forgetting `accTitle`/`accDescr` | Add accessibility metadata to supported types |
| Not testing in destination | GitHub, GitLab, etc. support different Mermaid versions |
| Missing `radar-beta` keyword | Use `radar-beta` not `radar` (bare keyword doesn't exist) |
| Confusing XY Chart vs Radar | XY Chart = bars/lines; Radar = spider web |
| No text alternative | Provide italic description for types that don't support `accTitle` |

---

## Integration with Other Content

**With `artifact-diagramming`** — for inline SVG and light/dark theme legibility

**With `dataviz`** — use Mermaid for structural diagrams; use `dataviz` for measured data and statistical charts

**With `markdown-style-guide`** — place diagrams inline with related text, not in a separate "Figures" section
