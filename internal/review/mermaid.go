package review

import (
	"regexp"
	"strings"

	"charm.land/lipgloss/v2"
)

// mermaid.go renders fenced blocks whose info string is `mermaid` as ASCII
// diagrams instead of chroma-highlighted source. Chroma has no mermaid lexer,
// so a diagram used to show as source lines with colours that were at best
// meaningless (the 2026-08-10 mermaid-diagrams feedback).
//
// Scope is a defensible subset, not the whole grammar: this first slice
// understands `flowchart`/`graph` and renders the flow as a boxed tree —
// spine, ├/└ junctions, ▼ arrowheads, edge labels — read top-down whatever
// direction the block asks for. Anything the parser does not understand makes
// the whole block fall back to its plain source lines (never chroma's noise,
// and never a half-parsed diagram).

var (
	mermaidBorder = rawStyle
	mermaidLabel  = dimStyle
	mermaidRef    = dimStyle
	mermaidText   = textStyle
)

// mermaidChildIndent is how far each tree level's boxes sit to the right of
// their parent's spine. Wide enough that junction ─ runs have room for a
// branch label, narrow enough that a deep flow stays on the measure.
const mermaidChildIndent = 6

// mermaidRefLen caps a duplicate-node reference (`↩ <text>`) so a long label
// cannot push the diagram off the measure by itself.
const mermaidRefLen = 24

// mermaidNode is one node of a flowchart: an id and the text to show. shape
// is the border family the node renders with; "" is a plain rectangle.
type mermaidNode struct {
	id    string
	text  string
	shape string
}

// mermaidEdge is one directed link between two nodes. arrow is false for a
// plain `---` link (no arrowhead); label is the `-->|text|` / `-- text -->`
// annotation, empty when the link carries none.
type mermaidEdge struct {
	from, to string
	label    string
	arrow    bool
}

// mermaidGraph is the parsed model of a flowchart: every node seen (a node is
// created implicitly by appearing in an edge if no definition line names it),
// every edge in source order, and the order ids first appeared in so the
// layout can pick roots deterministically.
type mermaidGraph struct {
	nodes map[string]mermaidNode
	edges []mermaidEdge
	order []string
}

// node returns the named node, or a bare `[id]` node when nothing ever
// defined it — every id that appears in an edge is addNode'd on the way in,
// so this is only a safety net.
func (g *mermaidGraph) node(id string) mermaidNode {
	if n, ok := g.nodes[id]; ok {
		return n
	}
	return mermaidNode{id: id, text: id}
}

// addNode records a node. A bare reference (`A` in an edge, with no bracket
// family) never overwrites a real definition; a real definition replaces a
// bare node seen earlier, so the layout draws the last word mermaid said
// about a node.
func (g *mermaidGraph) addNode(n mermaidNode) {
	if _, ok := g.nodes[n.id]; !ok {
		g.order = append(g.order, n.id)
		g.nodes[n.id] = n
		return
	}
	if isMermaidBare(n) {
		return
	}
	g.nodes[n.id] = n
}

// isMermaidBare reports whether n is a bare id reference — no bracket family,
// no text — rather than a definition like `A[Start]` or `A{Decision}`.
func isMermaidBare(n mermaidNode) bool {
	return n.shape == "" && n.text == n.id
}

// mermaidIDRE splits a node reference into its id and whatever follows it.
var mermaidIDRE = regexp.MustCompile(`^([A-Za-z0-9_]+)\s*(.*)$`)

// mermaidShape is one node bracket family. Longest openers first so `[[` wins
// over `[`; several shapes share a rendering (kind).
type mermaidShape struct {
	open, close, kind string
}

var mermaidShapes = []mermaidShape{
	{"[[", "]]", "subroutine"},
	{"((", "))", "circle"},
	{"{{", "}}", "hexagon"},
	{"([", "])", "stadium"},
	{"[(", ")]", "cylinder"},
	{">", "]", "asymmetric"},
	{"[/", "/]", "parallelogram"},
	{"[\\", "\\]", "parallelogram"},
	{"[/", "\\]", "parallelogram"},
	{"[\\", "/]", "parallelogram"},
	{"{", "}", "decision"},
	{"[", "]", "rectangle"},
	{"(", ")", "round"},
}

// parseMermaidNodeSpec parses one node reference: `id`, `id[text]`, `id{text}`
// or any other family in mermaidShapes. The text is everything between the
// outer delimiters (last one wins, so a `]` inside a label is not the end),
// with surrounding quotes stripped. The trailing text after the closing
// delimiter must be empty for the shape to claim the string.
func parseMermaidNodeSpec(s string) (mermaidNode, bool) {
	s = strings.TrimSpace(s)
	m := mermaidIDRE.FindStringSubmatch(s)
	if m == nil {
		return mermaidNode{}, false
	}
	id, rest := m[1], strings.TrimSpace(m[2])
	if rest == "" {
		return mermaidNode{id: id, text: id}, true
	}
	for _, sh := range mermaidShapes {
		if !strings.HasPrefix(rest, sh.open) {
			continue
		}
		inner := rest[len(sh.open):]
		idx := strings.LastIndex(inner, sh.close)
		if idx < 0 {
			continue
		}
		if strings.TrimSpace(inner[idx+len(sh.close):]) != "" {
			continue
		}
		text := strings.TrimSpace(inner[:idx])
		text = strings.Trim(text, `"'`)
		text = strings.TrimSpace(text)
		if text == "" {
			text = id
		}
		return mermaidNode{id: id, text: text, shape: sh.kind}, true
	}
	return mermaidNode{}, false
}

// parseMermaidNodeList splits a source or target list (`A & B & C`) into its
// node references. A segment that is not a valid node list reports not ok, so
// the caller can tell a node list from a stray between-label.
func parseMermaidNodeList(s string) ([]mermaidNode, bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil, false
	}
	var out []mermaidNode
	for _, part := range strings.Split(s, "&") {
		n, ok := parseMermaidNodeSpec(part)
		if !ok {
			return nil, false
		}
		out = append(out, n)
	}
	return out, true
}

// mermaidLinks is every link token the scanner recognises, longest-first so a
// `-->` wins over the `--` it starts with, `-.->` over `-.-`, and so on. The
// bare heads `--`, `==`, `-.` are the `-- text -->` spelling's start and are
// not complete links on their own; `.->` is that spelling's arrow.
var mermaidLinks = []string{
	"-->", "---",
	"-.->", "-.x", "-.X", "-.-",
	"==>", "===", "==x", "==X",
	"--x", "--X",
	"--", "==", "-.", ".->",
}

// mermaidLinkHit is a found link token's byte span.
type mermaidLinkHit struct{ start, end int }

// mermaidLinkIndexes finds every link token in s. It skips bracketed regions
// (a node's `[text]`), so a `---` inside a label is not read as a link.
func mermaidLinkIndexes(s string) []mermaidLinkHit {
	var out []mermaidLinkHit
	depth := 0
	for i := 0; i < len(s); {
		switch s[i] {
		case '[', '(', '{':
			depth++
		case ']', ')', '}':
			if depth > 0 {
				depth--
			}
		}
		if depth == 0 {
			for _, l := range mermaidLinks {
				if strings.HasPrefix(s[i:], l) {
					out = append(out, mermaidLinkHit{start: i, end: i + len(l)})
					i += len(l)
					goto next
				}
			}
		}
		i++
	next:
	}
	return out
}

// splitMermaidLabel strips a leading `|label|` from a segment — the tight
// `-->|text|` spelling — returning the label and the remaining target text.
func splitMermaidLabel(s string) (label, rest string) {
	s = strings.TrimSpace(s)
	if !strings.HasPrefix(s, "|") {
		return "", s
	}
	if i := strings.IndexByte(s[1:], '|'); i >= 0 {
		return s[1 : 1+i], strings.TrimSpace(s[1+i+1:])
	}
	return "", s
}

// mermaidEdgesFrom parses one statement (a line, or a `;`-fragment of one)
// into the nodes and edges it defines. ok is false when the statement has a
// link shape the parser does not understand, so the caller can fall back to
// plain source rather than render something wrong.
func mermaidEdgesFrom(g *mermaidGraph, s string) bool {
	s = strings.TrimSpace(s)
	if s == "" {
		return true
	}

	type mtoken struct {
		isLink bool
		link   string
		seg    string
	}
	idxs := mermaidLinkIndexes(s)
	if len(idxs) == 0 {
		// No link: a node definition statement (`A[text]`, or a bare `A`).
		if n, ok := parseMermaidNodeSpec(s); ok {
			g.addNode(n)
			return true
		}
		return false
	}
	var toks []mtoken
	pos := 0
	for _, hit := range idxs {
		toks = append(toks, mtoken{seg: s[pos:hit.start]})
		toks = append(toks, mtoken{isLink: true, link: s[hit.start:hit.end]})
		pos = hit.end
	}
	toks = append(toks, mtoken{seg: s[pos:]})

	var sources []mermaidNode
	label := ""
	arrow := false
	awaitTarget := false
	labelPending := false
	bareHead := false
	for _, t := range toks {
		if t.isLink {
			if isMermaidBareHead(t.link) {
				// `-- text -->`: the bare head only opens a between-label,
				// which the segment that follows carries; the next link token
				// is that link's arrow.
				labelPending = true
				bareHead = true
				arrow = false
				continue
			}
			arrow = strings.Contains(t.link, ">")
			awaitTarget = true
			bareHead = false
			continue
		}
		seg := t.seg
		if l, rest := splitMermaidLabel(seg); l != "" {
			label = l
			seg = rest
		}
		seg = strings.TrimSpace(seg)
		if labelPending {
			if seg != "" {
				label = seg
			}
			labelPending = false
			continue
		}
		if seg == "" {
			continue
		}
		nodes, ok := parseMermaidNodeList(seg)
		if ok {
			if awaitTarget {
				for _, src := range sources {
					for _, tgt := range nodes {
						g.addNode(src)
						g.addNode(tgt)
						g.edges = append(g.edges, mermaidEdge{from: src.id, to: tgt.id, label: label, arrow: arrow})
					}
				}
				sources = nodes
				label = ""
				awaitTarget = false
				continue
			}
			sources = nodes
			continue
		}
		if awaitTarget {
			// A non-node segment after a link is a between-label the
			// bare-head handling above left over (e.g. one containing `>`).
			label = seg
			continue
		}
		return false
	}
	if awaitTarget || labelPending || bareHead {
		return false
	}
	return true
}

// isMermaidBareHead reports whether a link token is only a head (`--`, `==`,
// `-.`) — the opening of the `-- text -->` between-label spelling, which is
// not a complete link by itself.
func isMermaidBareHead(link string) bool {
	switch link {
	case "--", "==", "-.":
		return true
	}
	return false
}

// parseMermaidFlowchart parses a mermaid `flowchart`/`graph` block into its
// model. ok is false for a block that is not a flowchart or that contains a
// statement the parser does not understand.
func parseMermaidFlowchart(lines []string) (*mermaidGraph, bool) {
	g := &mermaidGraph{nodes: map[string]mermaidNode{}}
	sawHeader := false
	for _, raw := range lines {
		// `%%` runs a mermaid comment to the end of its line.
		if i := strings.Index(raw, "%%"); i >= 0 {
			raw = raw[:i]
		}
		for _, stmt := range strings.Split(raw, ";") {
			stmt = strings.TrimSpace(stmt)
			if stmt == "" {
				continue
			}
			first := stmt
			if i := strings.IndexAny(stmt, " \t"); i >= 0 {
				first = stmt[:i]
			}
			switch strings.ToLower(first) {
			case "flowchart", "graph":
				sawHeader = true
				continue
			case "direction", "end", "subgraph", "classdef", "linkstyle", "click", "style":
				continue
			}
			if !mermaidEdgesFrom(g, stmt) {
				return nil, false
			}
		}
	}
	if !sawHeader || len(g.nodes) == 0 {
		return nil, false
	}
	return g, true
}

// mermaidLayout renders a parsed flowchart as a boxed tree: each node is a
// bordered box, its children hang off a spine beneath it with ├/└ junctions
// and ▼ arrowheads. A node reached by a second path — a shared target, or a
// cycle — draws a dim ↩ reference to the first occurrence instead of a second
// box, which is what lets a reconverging flow terminate. Nodes no root can
// reach (a pure cycle) are named on a dim line rather than silently dropped.
func mermaidLayout(g *mermaidGraph) []string {
	indeg := map[string]int{}
	adj := map[string][]mermaidEdge{}
	for _, e := range g.edges {
		indeg[e.to]++
		adj[e.from] = append(adj[e.from], e)
	}
	var roots []string
	for _, id := range g.order {
		if _, ok := g.nodes[id]; ok && indeg[id] == 0 {
			roots = append(roots, id)
		}
	}
	if len(roots) == 0 {
		return nil
	}

	seen := map[string]bool{}
	var out []mrow
	for i, r := range roots {
		seen[r] = true
		sub := mermaidRenderSub(g, adj, r, seen)
		if i > 0 {
			out = append(out, mrow{})
		}
		out = append(out, sub.rows...)
	}
	var unreachable []string
	for _, id := range g.order {
		if _, ok := g.nodes[id]; ok && !seen[id] {
			unreachable = append(unreachable, id)
		}
	}
	if len(unreachable) > 0 {
		out = append(out, mrow{}, mrow{{style: &mermaidRef, text: "(unreachable: " + strings.Join(unreachable, ", ") + ")"}})
	}
	if len(out) == 0 {
		return nil
	}

	width := 0
	for _, r := range out {
		if r.width() > width {
			width = r.width()
		}
	}
	lines := make([]string, len(out))
	for i, r := range out {
		lines[i] = r.padTo(width).render()
	}
	return lines
}

// mermaidSub is one node's rendered subtree: its rows (all the same width),
// the column its spine sits at within them, and that width.
type mermaidSub struct {
	rows  []mrow
	spine int
	width int
}

// mermaidRenderSub renders the subtree rooted at id. seen holds every node
// already rendered along the walk; a child that is already seen becomes a ↩
// reference, which also stops a cycle from recursing forever.
func mermaidRenderSub(g *mermaidGraph, adj map[string][]mermaidEdge, id string, seen map[string]bool) *mermaidSub {
	boxRows, boxSpine := mermaidBox(g.node(id))
	boxWidth := boxRows[0].width()
	edges := adj[id]
	if len(edges) == 0 {
		return &mermaidSub{rows: boxRows, spine: boxSpine, width: boxWidth}
	}

	type child struct {
		edge mermaidEdge
		sub  *mermaidSub
		ref  string
	}
	var children []child
	for _, e := range edges {
		if seen[e.to] {
			children = append(children, child{edge: e, ref: g.node(e.to).text})
			continue
		}
		seen[e.to] = true
		children = append(children, child{edge: e, sub: mermaidRenderSub(g, adj, e.to, seen)})
	}

	// A single child hangs straight below its parent, centred on the spine,
	// so a chain reads as one vertical flow rather than a staircase. Several
	// children hang at the child indent, so their branches do not collide
	// with the parent spine or with each other. A single child that is only a
	// reference has no subtree to centre on, so it uses the indent form.
	single := len(children) == 1
	childLeft := boxSpine + mermaidChildIndent
	if single && children[0].ref == "" {
		childLeft = boxSpine - children[0].sub.spine
	}
	minCol := 0
	if childLeft < 0 {
		minCol = childLeft
	}

	height := len(boxRows) + 1 // box + the lead-in spine row
	for _, c := range children {
		if c.ref != "" {
			height++
			continue
		}
		height += 1 + len(c.sub.rows)
	}
	// Shift everything so no column is negative (a single child centred on a
	// narrow box can start left of the box).
	shift := -minCol
	boxSpine += shift
	childLeft += shift

	rows := make([]mrow, 0, height)
	for _, br := range boxRows {
		r := make(mrow, 0, shift+len(br))
		r = r.padTo(shift)
		r = append(r, br...)
		rows = append(rows, r)
	}
	lead := make(mrow, 0, boxSpine+1)
	lead = lead.padTo(boxSpine)
	lead = append(lead, mspan{style: &mermaidBorder, text: "│"})
	rows = append(rows, lead)

	for i, c := range children {
		last := i == len(children)-1
		var jtRows []mrow
		if c.ref != "" {
			jtRows = []mrow{mermaidRefRow(boxSpine, childLeft, c.ref, single, last)}
		} else {
			jtRows = mermaidJunctionRows(boxSpine, childLeft, c.sub.spine, c.edge, last)
		}
		rows = append(rows, jtRows...)
		if c.ref != "" {
			continue
		}
		for _, cr := range c.sub.rows {
			var r mrow
			if !last {
				r = r.padTo(boxSpine)
				r = append(r, mspan{style: &mermaidBorder, text: "│"})
			}
			r = r.padTo(childLeft)
			r = append(r, cr...)
			rows = append(rows, r)
		}
	}

	// Normalise width (a label can stick out past the last box) and pad every
	// row to it, so the subtree is a rectangle of equal-width lines.
	width := 0
	for _, r := range rows {
		if r.width() > width {
			width = r.width()
		}
	}
	for i := range rows {
		rows[i] = rows[i].padTo(width)
	}

	return &mermaidSub{rows: rows, spine: boxSpine, width: width}
}

// mermaidJunctionRows builds the connector from the parent spine to a child's
// box: ├ (or └ for the last child) at the spine, a ─ run carrying the edge's
// label, and the arrowhead — or a bare │ for a plain `---` link — at the
// child's centre. When the label is longer than the run it gets its own row
// above the arrowhead instead of being truncated, so a branch's wording is
// never lost.
func mermaidJunctionRows(spine, childLeft, childSpine int, e mermaidEdge, last bool) []mrow {
	tieIn := childLeft + childSpine
	head := "▼"
	if !e.arrow {
		head = "│"
	}
	glyph := "├"
	if last {
		glyph = "└"
	}

	if tieIn <= spine {
		// A single child hanging straight below: just the arrowhead, with
		// the edge's label (if any) read off to its right.
		r := make(mrow, 0, spine+3)
		r = r.padTo(spine)
		r = append(r, mspan{style: &mermaidBorder, text: head})
		if e.label != "" {
			r = append(r, mspan{text: " "})
			r = append(r, mspan{style: &mermaidLabel, text: e.label})
		}
		return []mrow{r}
	}

	runLen := tieIn - spine - 1
	label := e.label
	lw := len([]rune(label))
	if label == "" || lw+2 <= runLen {
		// The run alone, or with the label centred in it, carries the
		// connector in one row.
		r := make(mrow, 0, tieIn+1)
		r = r.padTo(spine)
		r = append(r, mspan{style: &mermaidBorder, text: glyph})
		if label == "" {
			r = append(r, mspan{style: &mermaidBorder, text: strings.Repeat("─", runLen)})
		} else {
			padL := (runLen - lw) / 2
			r = append(r, mspan{style: &mermaidBorder, text: strings.Repeat("─", padL)})
			r = append(r, mspan{style: &mermaidLabel, text: label})
			r = append(r, mspan{style: &mermaidBorder, text: strings.Repeat("─", runLen-padL-lw)})
		}
		r = append(r, mspan{style: &mermaidBorder, text: head})
		return []mrow{r}
	}

	// Label longer than the run: the run row ends in a corner, and the label
	// sits right-aligned on the row above the arrowhead. Only an absurdly
	// long label is truncated, and then only to the width before the arrow.
	label = truncate(label, max(tieIn, 1))
	lw = len([]rune(label))
	r1 := make(mrow, 0, tieIn+1)
	r1 = r1.padTo(spine)
	r1 = append(r1, mspan{style: &mermaidBorder, text: glyph})
	r1 = append(r1, mspan{style: &mermaidBorder, text: strings.Repeat("─", runLen)})
	r1 = append(r1, mspan{style: &mermaidBorder, text: "┐"})
	r2 := make(mrow, 0, tieIn+1)
	r2 = r2.padTo(tieIn - lw - 1)
	r2 = append(r2, mspan{style: &mermaidLabel, text: label})
	r2 = append(r2, mspan{style: &mermaidBorder, text: head})
	return []mrow{r1, r2}
}

// mermaidRefRow builds the connector row for a node that is already rendered
// elsewhere: a ─ run then a dim `↩ <text>` instead of a second box.
func mermaidRefRow(spine, childLeft int, ref string, single, last bool) mrow {
	txt := "↩ " + truncate(ref, mermaidRefLen)
	if single {
		r := make(mrow, 0, spine+1)
		r = r.padTo(spine)
		r = append(r, mspan{style: &mermaidRef, text: txt})
		return r
	}
	glyph := "├"
	if last {
		glyph = "└"
	}
	r := make(mrow, 0, childLeft+len([]rune(txt)))
	r = r.padTo(spine)
	r = append(r, mspan{style: &mermaidBorder, text: glyph})
	r = append(r, mspan{style: &mermaidBorder, text: strings.Repeat("─", childLeft-spine-1)})
	r = append(r, mspan{style: &mermaidRef, text: txt})
	return r
}

// wrapMermaidText wraps a node's text to a comfortable width so a long label
// becomes several box lines rather than one enormous box. Words never split;
// a word longer than the cap is kept whole (the box simply grows).
func wrapMermaidText(s string) []string {
	const cap = 30
	s = strings.TrimSpace(s)
	if s == "" {
		return []string{""}
	}
	var lines []string
	var cur []rune
	width := 0
	for _, w := range strings.Fields(s) {
		wr := len([]rune(w))
		if width > 0 && width+1+wr > cap {
			lines = append(lines, string(cur))
			cur = nil
			width = 0
		}
		if width > 0 {
			cur = append(cur, ' ')
			width++
		}
		cur = append(cur, []rune(w)...)
		width += wr
	}
	if len(cur) > 0 {
		lines = append(lines, string(cur))
	}
	return lines
}

// mermaidBox builds the bordered box for one node. A decision gets a ◇ in the
// middle of its top border (the one shape that changes how a flow is read);
// everything else is a rectangle or a rounded box. Returns the box rows and
// the box's centre column — where the spine and any arrowhead hang from.
func mermaidBox(n mermaidNode) ([]mrow, int) {
	textLines := wrapMermaidText(n.text)
	inner := 0
	for _, l := range textLines {
		if w := len([]rune(l)); w > inner {
			inner = w
		}
	}
	inner = max(inner, 1)
	bw := inner + 2

	tl, tr, bl, br := "┌", "┐", "└", "┘"
	if n.shape == "round" {
		tl, tr, bl, br = "╭", "╮", "╰", "╯"
	}
	top := tl + strings.Repeat("─", bw) + tr
	if n.shape == "decision" {
		r := []rune(top)
		r[len(r)/2] = '◇'
		top = string(r)
	}

	rows := make([]mrow, 0, len(textLines)+2)
	rows = append(rows, mrow{{style: &mermaidBorder, text: top}})
	for _, t := range textLines {
		pad := inner - len([]rune(t))
		rows = append(rows, mrow{
			{style: &mermaidBorder, text: "│ "},
			{text: strings.Repeat(" ", pad/2)},
			{style: &mermaidText, text: t},
			{text: strings.Repeat(" ", pad-pad/2)},
			{style: &mermaidBorder, text: " │"},
		})
	}
	rows = append(rows, mrow{{style: &mermaidBorder, text: bl + strings.Repeat("─", bw) + br}})
	return rows, len([]rune(top)) / 2
}

// mspan is one run of a diagram line sharing one style (nil = the terminal's
// default). Layout works on plain rune counts; styling happens only at render
// time, so column arithmetic never has to reason about SGR bytes.
type mspan struct {
	style *lipgloss.Style
	text  string
}

func (s mspan) width() int { return len([]rune(s.text)) }

// mrow is one rendered line of a diagram, as styled spans.
type mrow []mspan

func (r mrow) width() int {
	w := 0
	for _, s := range r {
		w += s.width()
	}
	return w
}

// padTo appends enough spaces to reach width w.
func (r mrow) padTo(w int) mrow {
	if n := w - r.width(); n > 0 {
		return append(r, mspan{text: strings.Repeat(" ", n)})
	}
	return r
}

// render turns the row into a single styled string.
func (r mrow) render() string {
	var b strings.Builder
	for _, s := range r {
		if s.style != nil {
			b.WriteString(s.style.Render(s.text))
		} else {
			b.WriteString(s.text)
		}
	}
	return b.String()
}

// renderMermaid attempts to render a mermaid flowchart block's source lines as
// a boxed diagram. ok is false when the block is not a flowchart the parser
// understands, in which case the caller shows plain source lines instead —
// never chroma's meaningless colours on mermaid source, and never garbage
// from a half-parsed diagram.
func renderMermaid(lines []string) ([]string, bool) {
	g, ok := parseMermaidFlowchart(lines)
	if !ok {
		return nil, false
	}
	out := mermaidLayout(g)
	if len(out) == 0 {
		return nil, false
	}
	return out, true
}
