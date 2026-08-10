package review

import (
	"strings"
	"testing"
)

// TestParseMermaidFlowchart covers the node and edge grammar the renderer
// understands: shapes from their bracket family, the two label spellings, the
// chained and `&`-joined link forms, and the plain `---` link.
func TestParseMermaidFlowchart(t *testing.T) {
	g, ok := parseMermaidFlowchart([]string{
		"flowchart TD",
		"A[Start] --> B{Decision}",
		"B -->|Yes| C[Do it]",
		"B -->|No| D[Skip it]",
		"B -- try again --> C",
		"A & B --> X[Shared]",
		"X --- Y[Quiet]",
		"Z",
	})
	if !ok {
		t.Fatal("parseMermaidFlowchart rejected a valid flowchart")
	}
	if len(g.nodes) != 7 {
		t.Fatalf("parsed %d nodes, want 7", len(g.nodes))
	}
	if a := g.nodes["A"]; a.text != "Start" || a.shape != "rectangle" {
		t.Errorf("A = %+v, want text Start, rectangle", a)
	}
	if b := g.nodes["B"]; b.text != "Decision" || b.shape != "decision" {
		t.Errorf("B = %+v, want text Decision, decision shape", b)
	}
	if c := g.nodes["C"]; c.text != "Do it" {
		t.Errorf("C = %+v, want text Do it", c)
	}
	if z := g.nodes["Z"]; z.text != "Z" {
		t.Errorf("bare Z = %+v, want text Z", z)
	}

	if len(g.edges) != 7 {
		t.Fatalf("parsed %d edges, want 7: %+v", len(g.edges), g.edges)
	}
	if e := g.edges[0]; e.from != "A" || e.to != "B" || e.arrow != true || e.label != "" {
		t.Errorf("edge 0 = %+v, want A->B arrow", e)
	}
	if e := g.edges[1]; e.label != "Yes" || !e.arrow {
		t.Errorf("edge 1 = %+v, want label Yes with arrowhead", e)
	}
	if e := g.edges[3]; e.from != "B" || e.to != "C" || e.label != "try again" || !e.arrow {
		t.Errorf("edge 3 (between-label) = %+v", e)
	}
	if e := g.edges[4]; e.from != "A" || e.to != "X" {
		t.Errorf("edge 4 (& join) = %+v", e)
	}
	if e := g.edges[6]; e.from != "X" || e.to != "Y" || e.arrow {
		t.Errorf("edge 6 (plain ---) = %+v, want no arrowhead", e)
	}
}

// TestParseMermaidFlowchartChains: `A --> B --> C` is two edges, not one link
// whose label is a node.
func TestParseMermaidFlowchartChains(t *testing.T) {
	g, ok := parseMermaidFlowchart([]string{"graph LR", "A --> B --> C"})
	if !ok {
		t.Fatal("parseMermaidFlowchart rejected a chained flowchart")
	}
	if len(g.edges) != 2 {
		t.Fatalf("parsed %d edges, want 2", len(g.edges))
	}
	if g.edges[0].to != "B" || g.edges[1].from != "B" {
		t.Errorf("chained edges = %+v", g.edges)
	}
}

// TestParseMermaidFlowchartSkipsBracketedDashes: a `---` inside a node's
// label must not be read as a link token.
func TestParseMermaidFlowchartSkipsBracketedDashes(t *testing.T) {
	g, ok := parseMermaidFlowchart([]string{"flowchart TD", "A[1 --- 2] --> B"})
	if !ok {
		t.Fatal("parseMermaidFlowchart rejected a label with dashes")
	}
	if len(g.edges) != 1 || g.edges[0].from != "A" || g.edges[0].to != "B" {
		t.Fatalf("edges = %+v, want the single A->B link", g.edges)
	}
	if a := g.nodes["A"]; a.text != "1 --- 2" {
		t.Errorf("A text = %q, want the label verbatim", a.text)
	}
}

// TestParseMermaidFlowchartIgnoresComments: `%%` runs to the end of the line.
func TestParseMermaidFlowchartIgnoresComments(t *testing.T) {
	g, ok := parseMermaidFlowchart([]string{"flowchart TD %% the header", "A --> B %% why B", "%% B --> C"})
	if !ok {
		t.Fatal("parseMermaidFlowchart rejected a commented flowchart")
	}
	if len(g.edges) != 1 {
		t.Fatalf("parsed %d edges, want 1 (the commented edge is dropped)", len(g.edges))
	}
}

// TestRenderMermaidChain: a straight chain renders as one vertical flow — the
// child centred on the parent's spine, so there is no staircase of indents.
func TestRenderMermaidChain(t *testing.T) {
	out, ok := renderMermaid([]string{"flowchart TD", "A[Start] --> B[Step]"})
	if !ok {
		t.Fatal("renderMermaid failed on a chain")
	}
	got := strings.Join(out, "\n")
	for _, want := range []string{"Start", "Step", "▼"} {
		if !strings.Contains(got, want) {
			t.Errorf("rendered chain missing %q:\n%s", want, got)
		}
	}
	// The child box starts at the same column as its parent: no staircase.
	sx, _ := findLine(out, "Start")
	stx, _ := findLine(out, "Step")
	if sx != stx {
		t.Errorf("chain not centred: parent text at col %d, child at col %d\n%s", sx, stx, got)
	}
}

// TestRenderMermaidDecisionTree: branches carry their labels and land their
// arrowheads on the branch target's centre.
func TestRenderMermaidDecisionTree(t *testing.T) {
	out, ok := renderMermaid([]string{
		"flowchart TD",
		"A[Start] --> B{Decision}",
		"B -->|Yes| C[Do it]",
		"B -->|No| D[Skip it]",
	})
	if !ok {
		t.Fatal("renderMermaid failed on a decision tree")
	}
	got := strings.Join(out, "\n")
	for _, want := range []string{"Start", "Decision", "Do it", "Skip it", "◇", "Yes", "No", "▼", "├", "└"} {
		if !strings.Contains(got, want) {
			t.Errorf("rendered tree missing %q:\n%s", want, got)
		}
	}
	// The two branches are at the same indentation, so they read as siblings.
	dox, _ := findLine(out, "Do it")
	skx, _ := findLine(out, "Skip it")
	if dox != skx {
		t.Errorf("branches misaligned: Do it at col %d, Skip it at col %d\n%s", dox, skx, got)
	}
}

// TestRenderMermaidSharedNode: a node reached by two paths renders once, and
// the second path draws a ↩ reference instead of a second box.
func TestRenderMermaidSharedNode(t *testing.T) {
	out, ok := renderMermaid([]string{
		"flowchart TD",
		"A --> B --> D",
		"A --> C --> D",
	})
	if !ok {
		t.Fatal("renderMermaid failed on a shared node")
	}
	got := strings.Join(out, "\n")
	// "D" appears once as a box and once as a reference.
	boxes := strings.Count(ansiRe.ReplaceAllString(got, ""), "│ D │")
	if boxes != 1 {
		t.Errorf("D boxed %d times, want 1:\n%s", boxes, got)
	}
	if !strings.Contains(got, "↩") {
		t.Errorf("second path to D has no ↩ reference:\n%s", got)
	}
}

// TestRenderMermaidCycleTerminates: a cycle must not recurse forever; the
// back edge renders as a ↩ reference.
func TestRenderMermaidCycleTerminates(t *testing.T) {
	out, ok := renderMermaid([]string{
		"flowchart TD",
		"A --> B",
		"B --> C",
		"C --> B",
	})
	if !ok {
		t.Fatal("renderMermaid failed on a cycle")
	}
	got := strings.Join(out, "\n")
	for _, want := range []string{"A", "B", "C", "↩"} {
		if !strings.Contains(got, want) {
			t.Errorf("rendered cycle missing %q:\n%s", want, got)
		}
	}
	if strings.Count(ansiRe.ReplaceAllString(got, ""), "│ B │") != 1 {
		t.Errorf("B boxed more than once under a cycle:\n%s", got)
	}
}

// TestRenderMermaidRejectsNonFlowchart: a sequence diagram is not a flowchart
// the parser understands, so it must be reported as unsupported rather than
// half-rendered.
func TestRenderMermaidRejectsNonFlowchart(t *testing.T) {
	for _, src := range [][]string{
		{"sequenceDiagram", "A->>B: hello"},
		{"classDiagram", "A <|-- B"},
		{"gantt", "section One"},
	} {
		if _, ok := renderMermaid(src); ok {
			t.Errorf("renderMermaid(%q) rendered a non-flowchart as a diagram", src[0])
		}
	}
}

// TestRenderMermaidFallsBackOnGarbage: a malformed statement makes the whole
// block unsupported, so nothing wrong reaches the screen.
func TestRenderMermaidFallsBackOnGarbage(t *testing.T) {
	for _, src := range [][]string{
		{"flowchart TD", "A[Start] --> ??? B"},
		{"flowchart TD", "A -->"},  // dangling link
		{"flowchart TD", "A -- B"}, // bare head with no arrow
		{"not a header", "A --> B"},
	} {
		if _, ok := renderMermaid(src); ok {
			t.Errorf("renderMermaid(%q) rendered malformed input", strings.Join(src, "; "))
		}
	}
}

// TestRenderMermaidLabelsNeverTruncate: a label longer than the connector run
// gets its own row above the arrowhead, so a branch's wording is never lost.
func TestRenderMermaidLabelsNeverTruncate(t *testing.T) {
	out, ok := renderMermaid([]string{"flowchart TD", "A --> B{Choice}", "B -->|a label longer than the run| C"})
	if !ok {
		t.Fatal("renderMermaid failed on a long label")
	}
	got := strings.Join(out, "\n")
	if !strings.Contains(got, "a label longer than the run") {
		t.Errorf("long label was lost:\n%s", got)
	}
	if !strings.Contains(got, "▼") {
		t.Errorf("arrowhead lost to the label:\n%s", got)
	}
}

// TestMermaidBlockRendersInModel: a mermaid fence in a real document renders
// as a diagram through the model, and no mermaid source leaks to the screen.
func TestMermaidBlockRendersInModel(t *testing.T) {
	src := "# Title\n\n```mermaid\nflowchart TD\n    A[Start] --> B{Decision}\n    B -->|Yes| C[Do it]\n    B -->|No| D[Skip it]\n```\n"
	m := modelFromSource(t, src)
	plain := strings.Join(m.render(), "\n")
	for _, want := range []string{"Start", "Decision", "Do it", "Skip it", "Yes", "No", "▼"} {
		if !strings.Contains(plain, want) {
			t.Errorf("rendered document missing diagram %q", want)
		}
	}
	for _, gone := range []string{"flowchart", "-->"} {
		if strings.Contains(plain, gone) {
			t.Errorf("mermaid source leaked to the screen: %q", gone)
		}
	}
}

// TestMermaidUnsupportedBlockFallsBackToPlainSource: a mermaid block the
// renderer does not understand shows its source lines, not chroma colours and
// not a diagram.
func TestMermaidUnsupportedBlockFallsBackToPlainSource(t *testing.T) {
	src := "# Title\n\n```mermaid\nsequenceDiagram\n    A->>B: hello\n```\n"
	m := modelFromSource(t, src)
	plain := strings.Join(m.render(), "\n")
	if !strings.Contains(plain, "sequenceDiagram") {
		t.Error("unsupported mermaid block did not fall back to its source")
	}
}

// TestMermaidRawModeKeepsSourceVerbatim: in raw mode a mermaid fence stays
// plain source — chroma has no mermaid lexer, so it gets no syntax colours.
func TestMermaidRawModeKeepsSourceVerbatim(t *testing.T) {
	src := "# Title\n\n```mermaid\nflowchart TD\nA[Start] --> B\n```\n"
	m := modelFromSource(t, src)
	m.toggleRaw()
	lines := m.render()
	found := false
	for _, l := range lines {
		plain := string([]rune(ansiRe.ReplaceAllString(l, ""))[gutterW:])
		if strings.HasPrefix(plain, "A[Start] --> B") {
			found = true
		}
	}
	if !found {
		t.Error("raw mode lost the mermaid source line")
	}
}

// findLine returns the visual column and row of the first line containing
// needle, with styling stripped so multi-byte glyphs are counted as one column.
func findLine(lines []string, needle string) (col, row int) {
	for i, l := range lines {
		plain := ansiRe.ReplaceAllString(l, "")
		if j := strings.Index(plain, needle); j >= 0 {
			return len([]rune(plain[:j])), i
		}
	}
	return -1, -1
}
