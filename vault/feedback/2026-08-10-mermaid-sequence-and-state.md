# Mermaid: support sequence and state diagrams

Feedback, 2026-08-10. The mermaid renderer (journal 2026-08-10.17) only renders
`flowchart`/`graph`. Need the other two common kinds:

1. **sequence diagrams** (`sequenceDiagram`) — participants, message arrows,
   activations, notes.
2. **state diagrams** (`stateDiagram-v2`) — states, transitions, `[*]` start
   and end.

## How the maintainer wants it done (2026-08-10, supersedes the in-tree wording)

Vendor **`github.com/AlexanderGrooff/mermaid-ascii`** (the Go project, MIT) for
what it already supports — flowchart/graph, sequence, ER — and extend it in-tree
for the rest (notably state diagrams, which upstream does not have). The
`agentic-mermaid` fork covers state but is TypeScript/Node and cannot be
vendored into a Go tree; the maintainer chose the original Go library + in-tree
extension instead.

Requirements:

- The vendored copy lives in the tree as a real vendor/third-party dir with its
  upstream MIT license preserved.
- **Keep a clean changelog inside the vendored copy** (local changes documented
  as a series of deltas) so the in-tree extensions can be upstreamed later.
- Unparseable diagrams still fall back to plain source; never a half-parsed
  diagram. On-disk format untouched. A felt leg with a demo recipe.
- The existing hand-rolled `internal/review/mermaid.go` (flowchart-only,
  journal 2026-08-10.17) is superseded — the leg decides how it is retired
  (deleted or kept as a fallback) and says so.
