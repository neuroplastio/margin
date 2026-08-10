# Vendored deltas vs upstream mermaid-ascii

Every change this copy carries against the pinned upstream snapshot, in the
order it was made. Each delta is written so it can be re-applied to a fresh
upstream checkout and (where sensible) submitted upstream.

## D1 — go.mod trimmed to the library packages

Upstream's `go.mod` also required the cobra CLI and the gin web server
(`github.com/gin-gonic/gin`, `github.com/spf13/cobra`, `gookit/color`, and
their transitive trees). None of that is imported by the packages vendored
here, so the module requires only what `pkg/diagram`, `pkg/sequence`, `pkg/er`
and `pkg/graph` actually build against: `orderedmap/v2`, `go-runewidth`,
`logrus`. Upstreamable as "split the library out of the CLI module".

## D2 — graph renderer extracted from `cmd/` into `pkg/graph`

Upstream keeps the flowchart/graph renderer in the cobra CLI's `cmd` package,
so the library could not be imported without the CLI. The nine renderer files
(`graph.go`, `mapping_node.go`, `mapping_edge.go`, `draw.go`, `label.go`,
`math.go`, `arrow.go`, `direction.go`, `parse.go`) were moved to
`pkg/graph` and renamed to `package graph`; the CLI globals they referenced
(`Coords`, `boxBorderPadding`, `paddingBetweenX`, `paddingBetweenY`, once
cobra flags in `cmd/root.go`) are now package defaults in `api.go`.

`api.go` adds the exported `Parse` + `Render` entry points, mirroring the
`sequence` and `er` packages' shape, so all four diagram kinds present one
interface. For the same uniformity, `pkg/er`'s `erKeyword` constant was
exported as `Keyword` (mirroring `sequence.SequenceDiagramKeyword`).

The graph package's tests were not extracted: they exercise the whole `cmd`
package, which would re-drag in the CLI. Upstreamable as a package move plus
`api.go`.

## D3 — strict graph parse: an unparseable statement fails the diagram

Upstream's `mermaidFileToMap` treated an unparseable statement leniently: it
turned the whole line into a node (`A --- B` drew as a single box labelled
`A --- B`). margin's contract is "never a half-parsed diagram" — a block the
parser does not understand falls back to its plain source. So `parseString` now
errors on a line it cannot parse, `mermaidFileToMap` propagates the error, and
every link handler fails the edge when a segment is not a valid node. The
caller (`internal/review/mermaid.go`) falls back to the block's source lines on
any error. Upstreamable, with the tradeoff that upstream's leniency is the
deliberate opposite behaviour.

## D4 — graph grammar parity with the renderer margin replaced

The vendored graph renderer replaces the hand-rolled flowchart parser margin
shipped in 2026-08-10.17. To avoid regressing that grammar, `pkg/graph` gained
what the old parser already understood and upstream did not:

- **Node shapes.** Upstream only recognised `id[text]`; every other bracket
  family (`A((x))`, `A{x}`, `A[[x]]`, `A([x])`, `A[(x)]`, `A>x]`,
  parallelograms) made the brackets part of the node's id, so an edge to the
  plain id never matched. `parseNode`/`parseNodeLine` now recognise all the
  families: the id is always the leading token, the label the bracket text.
  Layout still draws every shape as a rectangle (upstream has no shape glyphs);
  see the gaps list.
- **Link forms.** Upstream parsed only `-->` and `<-->`. Added `---`, `==>`,
  `===`, `-.->`, `-.-`, `--x`, `==x`, `-.x` and the `-- text -->`, `== text
  ==>`, `-. text .->` between-label spellings, each with the `|label|`
  variant.
- **Skipped statements.** `direction`, `linkStyle`, `style` and `click`
  (which the old parser skipped, and which upstream silently turned into
  nodes) are now no-ops. `classDef` and `subgraph`/`end` were already handled
  upstream.

## D5 — graph CLI output is plain text, not ANSI

Upstream's `wrapTextInColor` rendered node text through `gookit/color` when the
CLI style was selected and stdout looked like a terminal. margin styles the
whole diagram itself (lipgloss, in `internal/review/mermaid.go`); embedded
gookit ANSI would fight the selection/search highlighting (see findings F18).
The `cli` branch now returns the text unchanged and the `gookit/color`
dependency is gone. The `html` branch is kept for upstream's own use.
Upstreamable as "colour choice belongs to the caller".

## D6 — sequence `activate`/`deactivate` tolerated

Upstream has no activation model, so `activate B` / `deactivate B` failed the
whole sequence diagram. The lines are now matched and skipped, so a diagram
that uses activations still renders; the activation bars themselves are **not
drawn** — that is the obvious next delta. Note in the demo recipe for
2026-08-10.19.

## Known gaps (candidate future deltas)

- **Graph node shapes render as rectangles.** D4 parses the shape families but
  the layout draws every box the same; the `◇` decision marker and rounded
  corners the previous renderer drew are gone. Felt: does the layered layout
  make shape glyphs necessary?
- **Edge labels on a vertical run can be split by the spine glyph**
  (`over│here`). Upstream draws the label centered on the edge, and a vertical
  connector interrupts it. Felt.
- **Plain `---` links draw an arrowhead** — upstream draws every edge with
  `▼` regardless of the link form. Felt.
- **Sequence activations are skipped** (D6) — bars are not drawn.
- **No state diagrams.** `stateDiagram`/`stateDiagram-v2` is the in-tree
  extension the next leg adds here as `pkg/state`, following D2's package
  shape (Parse + Render + `Keyword`).
