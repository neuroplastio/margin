# Vendored deltas vs upstream mermaid-ascii

Every change this copy carries against the pinned upstream snapshot, in the
order it was made. Each delta is written so it can be re-applied to a fresh
upstream checkout and (where sensible) submitted upstream.

## Packaging note (2026-08-10) — folded into the host module

This copy no longer carries its own `go.mod`: it is a directory of packages
inside `github.com/neuroplastio/margin`, imported as
`github.com/neuroplastio/margin/internal/mermaid-ascii/pkg/...`. D1 below
describes trimming the upstream go.mod; the requirements it listed
(`orderedmap/v2`, `go-runewidth`, `logrus`) are gone too — D8 drops logrus and
orderedmap, and `go-runewidth` is the only remaining requirement, a direct
dependency of the host module. This was done so `go install module@version`
works — a nested module wired via `replace` breaks it, because the replaced
go.mod would be interpreted differently as a dependency. D1's upstreamable
intent (split the library out of the CLI module) is unchanged.

## D10 — subgraph-band placement: a subgraph's nodes share one grid band

Upstream's layered placement is level-driven and **subgraph-agnostic**: every
node's grid coordinate comes from its graph level (roots at level 0, children
four rows down) against one shared per-level slot counter, and a subgraph's
frame is then just a bounding box drawn around whatever grid cells its nodes
happened to land in. With a few dozen nodes across several subgraphs the boxes
interleave — two frames crash into each other, a node renders on its own
subgraph's border, an edge label splits across a frame line. Reproduced by the
maintainer's `flowchart TB` with three subgraphs (see
`internal/review/mermaid_test.go`, `TestMermaidSubgraphReproBoxesDoNotOverlap`).

The fix is in `createMapping`: when the graph has subgraphs, the layout is
**banded** before the shared drawing pass runs:

- Every node is assigned to a unit — everything beneath one root-level
  subgraph (nested subgraphs included), or all nodes outside any subgraph as a
  single trailing unit.
- Nodes are still levelled by the same root-then-children walk, but each unit
  keeps its **own per-level slot counter**, so a unit's nodes cluster in a
  contiguous band of grid slots instead of scattering across the whole row.
- Bands are sized to each unit's widest level and laid out end to end with a
  gap slot between them, so two subgraph frames are always disjoint.
- Node footprints are reserved in the grid as the plain path does, so edge
  routing still avoids boxes.

The flat path (`len(g.subgraphs) == 0`) is untouched — every existing
flowchart, sequence and state diagram renders byte-identically. The banded
path also skips upstream's LR external/subgraph root separation (dead there
anyway, since each subgraph now owns its band); LR just mirrors the bands
along the other axis. Upstreamable as "place subgraph nodes as groups".

## D8 — dependencies stripped to runewidth; moved under `internal/`

The fork now sits at `internal/mermaid-ascii/` (it lived at
`third_party/mermaid-ascii/` before), mirroring the upstream `pkg/` layout so
an extraction to a standalone repo is a copy plus a `go.mod` — see
`README.md` for the recipe. Two of the three remaining external dependencies
were dropped:

- **logrus → `pkg/log` no-op shim.** The renderers' debug output was never
  enabled by margin, so every `log.Debug`/`log.Debugf` call is dead weight.
  `pkg/log` keeps the call sites (so the delta vs upstream stays small) but
  implements them as no-ops with no dependency. Upstreamable as "route debug
  output through a caller-provided sink".
- **`orderedmap/v2` → internal `textGraphData`.** The graph parser's data
  store only ever used ordered `Set`, `Get` and first-seen-order iteration;
  `pkg/graph/parse.go` now carries a ~30-line slice-plus-map type with the
  same semantics. Upstreamable as "replace orderedmap with a slice+map".

Also dropped in this delta: the unused golden-test fixtures
(`cmd/testdata/{ascii,extended-chars,multibyte}` — the graph package has no
extracted tests, so nothing read them). The remaining fixtures moved from
`cmd/testdata/` to the fork's own `testdata/`.

## D1 — go.mod trimmed to the library packages

Upstream's `go.mod` also required the cobra CLI and the gin web server
(`github.com/gin-gonic/gin`, `github.com/spf13/cobra`, `gookit/color`, and
their transitive trees). None of that is imported by the packages vendored
here, so the module requires only what `pkg/diagram`, `pkg/sequence`, `pkg/er`
and `pkg/graph` actually build against: `orderedmap/v2`, `go-runewidth`,
`logrus` (the latter two now further trimmed by D8). Upstreamable as "split the
library out of the CLI module".

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

## D7 — new `pkg/state`: state diagrams (in-tree extension)

Upstream has no state renderer, so this package is the in-tree extension the
vendoring leg promised. `stateDiagram` and `stateDiagram-v2` parse into a
state/transition model: `Parse` + `Render` + `Keyword` + `IsStateDiagram`,
following D2's package shape. Rendering itself is routed through the vendored
graph engine (see D9) — each transition draws as a routed labelled edge, `[*]`
start/end markers draw as circle nodes. Declarations accept `state Name`,
`state "Name"` and
`state "Long description" as Short` (a transition may then reference the state
by either id or description). The parser is strict, so anything outside the
subset — composite states (`state X { … }`), notes — fails the whole diagram
and the dispatcher falls back to plain source, the never-a-half-parsed
contract. `direction` lines are skipped, not rejected. Upstreamable as a new
library package.

## D9 — state diagrams route through the graph engine

The D7 state renderer drew boxes on a single centred spine and referenced a
revisited state with a dim `↩ label` shortcut. That worked but never *drew*
the edges; this delta replaces the spine renderer with a real routing: `Render`
builds the state diagram as a `graph.NewDiagram` (each state a node, each
transition a labelled edge) and renders through the vendored graph engine's
layered layout and A* edge router. A branch, a re-entered state or a cycle now
draws actual routed arrows back to the shared box. Two pieces of supporting
plumbing:

- `graph.NewDiagram(direction, nodes, edges)` — a programmatic builder (the
  graph package previously only built its model from mermaid text), so a
  caller with an already-parsed model can route through the engine without a
  source round-trip.
- Node shapes: `textNode`/`graphNodeSpec`/`node` carry a `shape` string parsed
  from the bracket family (`(( ))` → circle), and a circle node draws its
  label as a bare glyph — the state `[*]` start/end marker renders as `○`
  (`o` under ASCII), not a stadium box. A circle reserves a symmetric 2x2
  footprint (one content cell plus one cell of air each side) so its glyph
  sits at the centre of the node's grid cells, its label is drawn on the spine
  column its edges route through (not the centre of the shared drawing width),
  and its outgoing edges skip the box-start junction — so the marker reads
  `○ │ ▼`, the spine stays under the glyph, and an incoming back-edge's `◄`
  lands on the glyph's row instead of pointing at the footprint's edge.
- **State boxes force at least 1 column of horizontal padding, but no
  vertical padding.** A routed edge enters and leaves a box through its
  border, and the junction glyph overwrites the cell it lands on — so at the
  flowchart's compact `BoxBorderPadding = 0` a label that fills its box loses
  its edge character to the `├`/`┤` (observed: `Buildin├`). `state.Render`
  bumps horizontal padding to 1 so a state label never touches the border its
  edges route through; vertical padding stays 0 so a box is three rows tall,
  not five. To make that split the config gains `BoxPaddingY` (graph
  `diagram.Config`, default `-1` = inherit `BoxBorderPadding`; the
  flowchart's compact config is unchanged).

## Known gaps (candidate future deltas)

- **Graph node shapes render as rectangles** — except circles. D4 parses the
  shape families; D9 added circle rendering (`(( ))` → rounded stadium) for the
  state `[*]` markers, but the other families — decision `{}`, the `[[ ]]`
  families — still draw as rectangles. Felt: does the layered layout make shape
  glyphs necessary?
- **Edge labels on a vertical run can be split by the spine glyph**
  (`over│here`). Upstream draws the label centered on the edge, and a vertical
  connector interrupts it. Felt.
- **Plain `---` links draw an arrowhead** — upstream draws every edge with
  `▼` regardless of the link form. Felt.
- **Sequence activations are skipped** (D6) — bars are not drawn.
- **State diagrams cover the transition subset only** (D7): composite states
  (`state X { … }`) and notes (`note left of X: …`) are rejected, so a diagram
  using them falls back to plain source.
