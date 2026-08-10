package review

import (
	"strings"
	"testing"
)

// mermaid_test.go exercises the vendored mermaid renderer's dispatch
// (internal/review/mermaid.go → internal/mermaid-ascii): flowchart/graph,
// sequence and ER diagrams render through the vendored packages, and anything
// the vendored parser does not understand falls back to the block's plain
// source lines.

// TestRenderMermaidFlowchart: a flowchart with shapes and the common link
// forms renders as a diagram with the node labels, not the source.
func TestRenderMermaidFlowchart(t *testing.T) {
	out, ok := renderMermaid([]string{
		"flowchart TD",
		"A[Start] --> B{Decision}",
		"B -->|Yes| C[Do it]",
		"B -->|No| D[Skip it]",
		"C --- E[Quiet]",
		"D -.->|wander| F[Done]",
	})
	if !ok {
		t.Fatal("renderMermaid failed on a flowchart")
	}
	got := strings.Join(out, "\n")
	for _, want := range []string{"Start", "Decision", "Do it", "Skip it", "Quiet", "Done", "Yes", "No", "wander"} {
		if !strings.Contains(got, want) {
			t.Errorf("rendered flowchart missing %q:\n%s", want, got)
		}
	}
	// The bracket families parse into labels, not into the ids: the source
	// brackets and arrows must not leak to the screen.
	for _, gone := range []string{"A[", "B{", "-->"} {
		if strings.Contains(got, gone) {
			t.Errorf("flowchart source leaked into the render: %q\n%s", gone, got)
		}
	}
}

// TestRenderMermaidFlowchartShapes: every bracket family parses into a node
// whose id is the leading token and whose label is the bracket text, so an
// edge to a shaped node matches by id.
func TestRenderMermaidFlowchartShapes(t *testing.T) {
	out, ok := renderMermaid([]string{
		"graph TD",
		"A((Start)) --> B[[Sub]]",
		"B --> C([Rounded])",
		"C --> D[(Database)]",
	})
	if !ok {
		t.Fatal("renderMermaid failed on shaped nodes")
	}
	got := strings.Join(out, "\n")
	for _, want := range []string{"Start", "Sub", "Rounded", "Database"} {
		if !strings.Contains(got, want) {
			t.Errorf("shaped node label %q lost:\n%s", want, got)
		}
	}
	// The id's bracket text must not read as part of the label ("Start", not
	// "A((Start))").
	for _, gone := range []string{"A((Start))", "B[[Sub]]", "-->"} {
		if strings.Contains(got, gone) {
			t.Errorf("shape source leaked: %q\n%s", gone, got)
		}
	}
}

// TestRenderMermaidFlowchartLinkForms: the plain, thick and dotted links and
// the `-- text -->` between-label spelling all parse into edges.
func TestRenderMermaidFlowchartLinkForms(t *testing.T) {
	out, ok := renderMermaid([]string{
		"flowchart TD",
		"A[One] --- B[Two]",
		"B ==> C[Three]",
		"C -.-> D[Four]",
		"D -- over here --> E[Five]",
		"A == heavy ==> F[Six]",
	})
	if !ok {
		t.Fatal("renderMermaid failed on the extra link forms")
	}
	got := strings.Join(out, "\n")
	for _, want := range []string{"One", "Two", "Three", "Four", "Five", "Six", "heavy"} {
		if !strings.Contains(got, want) {
			t.Errorf("link-form render missing %q:\n%s", want, got)
		}
	}
	// The between-label "over here" sits on the edge's vertical run, where the
	// layout draws the spine glyph between the words ("over│here") — upstream's
	// edge-label placement, judged in the demo recipe. Assert the words, not
	// the contiguous phrase.
	for _, want := range []string{"over", "here"} {
		if !strings.Contains(got, want) {
			t.Errorf("link-form render missing between-label word %q:\n%s", want, got)
		}
	}
	for _, gone := range []string{"A[One] --- B[Two]", "-->"} {
		if strings.Contains(got, gone) {
			t.Errorf("link source leaked: %q\n%s", gone, got)
		}
	}
}

// TestRenderMermaidSequence: a sequence diagram renders its participants,
// message labels and notes.
func TestRenderMermaidSequence(t *testing.T) {
	out, ok := renderMermaid([]string{
		"sequenceDiagram",
		"participant A as Alice",
		"participant B as Bob",
		"A->>B: Hello Bob",
		"B-->>A: Hi Alice",
		"Note over A,B: A note",
	})
	if !ok {
		t.Fatal("renderMermaid failed on a sequence diagram")
	}
	got := strings.Join(out, "\n")
	for _, want := range []string{"Alice", "Bob", "Hello Bob", "Hi Alice", "A note"} {
		if !strings.Contains(got, want) {
			t.Errorf("rendered sequence missing %q:\n%s", want, got)
		}
	}
}

// TestRenderMermaidSequenceActivations: `activate`/`deactivate` lines (delta
// D6 in the vendored copy) are tolerated so a diagram that uses activations
// still renders; the bars themselves are not drawn.
func TestRenderMermaidSequenceActivations(t *testing.T) {
	out, ok := renderMermaid([]string{
		"sequenceDiagram",
		"A->>B: call",
		"activate B",
		"B-->>A: reply",
		"deactivate B",
	})
	if !ok {
		t.Fatal("renderMermaid failed on a sequence with activations")
	}
	got := strings.Join(out, "\n")
	for _, want := range []string{"call", "reply"} {
		if !strings.Contains(got, want) {
			t.Errorf("rendered sequence missing %q:\n%s", want, got)
		}
	}
}

// TestRenderMermaidER: an ER diagram renders its entities and relationships.
func TestRenderMermaidER(t *testing.T) {
	out, ok := renderMermaid([]string{
		"erDiagram",
		"CUSTOMER ||--o{ ORDER : places",
	})
	if !ok {
		t.Fatal("renderMermaid failed on an ER diagram")
	}
	got := strings.Join(out, "\n")
	for _, want := range []string{"CUSTOMER", "ORDER", "places"} {
		if !strings.Contains(got, want) {
			t.Errorf("rendered ER missing %q:\n%s", want, got)
		}
	}
}

// TestRenderMermaidState: a state diagram renders its states, start/end
// markers and transition labels.
func TestRenderMermaidState(t *testing.T) {
	out, ok := renderMermaid([]string{
		"stateDiagram-v2",
		"[*] --> Idle",
		"Idle --> Active: power on",
		"Active --> [*]",
	})
	if !ok {
		t.Fatal("renderMermaid failed on a state diagram")
	}
	got := strings.Join(out, "\n")
	for _, want := range []string{"Idle", "Active", "power on", "○", "▼"} {
		if !strings.Contains(got, want) {
			t.Errorf("rendered state diagram missing %q:\n%s", want, got)
		}
	}
	for _, gone := range []string{"stateDiagram-v2", "-->"} {
		if strings.Contains(got, gone) {
			t.Errorf("state source leaked into the render: %q\n%s", gone, got)
		}
	}
}

// TestRenderMermaidStateLegacyKeyword: the v1 `stateDiagram` spelling routes
// through the same package.
func TestRenderMermaidStateLegacyKeyword(t *testing.T) {
	out, ok := renderMermaid([]string{"stateDiagram", "A --> B"})
	if !ok {
		t.Fatal("renderMermaid failed on a legacy state diagram")
	}
	got := strings.Join(out, "\n")
	for _, want := range []string{"A", "B"} {
		if !strings.Contains(got, want) {
			t.Errorf("rendered state diagram missing %q:\n%s", want, got)
		}
	}
}

// TestRenderMermaidStateRejectsComposite: a composite state is beyond the
// vendored subset, so the block falls back rather than half-render.
func TestRenderMermaidStateRejectsComposite(t *testing.T) {
	if _, ok := renderMermaid([]string{
		"stateDiagram-v2",
		"state Working {",
		"    Idle --> Busy",
		"}",
	}); ok {
		t.Error("renderMermaid rendered a composite state as a diagram")
	}
}

// TestRenderMermaidRejectsUnknownKinds: kinds the vendored library does not
// support — class and gantt — fall back to plain source rather than rendering
// anything.
func TestRenderMermaidRejectsUnknownKinds(t *testing.T) {
	for _, src := range [][]string{
		{"classDiagram", "A <|-- B"},
		{"gantt", "section One"},
	} {
		if _, ok := renderMermaid(src); ok {
			t.Errorf("renderMermaid(%q) rendered an unsupported kind as a diagram", src[0])
		}
	}
}

// TestRenderMermaidFallsBackOnGarbage: a statement no vendored parser
// understands makes the whole block unsupported, so nothing wrong reaches the
// screen — the "never a half-parsed diagram" guarantee.
func TestRenderMermaidFallsBackOnGarbage(t *testing.T) {
	for _, src := range [][]string{
		{"flowchart TD", "A[Start] --> ??? B"},
		{"flowchart TD", "A -->"},          // dangling link
		{"flowchart TD", "A -- B"},         // bare head with no arrow
		{"flowchart TD", "this is not @@"}, // garbage node line
		{"sequenceDiagram", "A->>B: hi", "bogus line without an arrow"},
		{"not a header", "A --> B"},
	} {
		if _, ok := renderMermaid(src); ok {
			t.Errorf("renderMermaid(%q) rendered malformed input", strings.Join(src, "; "))
		}
	}
}

// TestMermaidDiagramStyled: box-drawing glyphs carry the muted frame colour
// and content text the bright colour, so a diagram reads against the prose
// the way the hand-rolled renderer's did.
func TestMermaidDiagramStyled(t *testing.T) {
	out, ok := renderMermaid([]string{"flowchart TD", "A[Start] --> B[Step]"})
	if !ok {
		t.Fatal("renderMermaid failed on a chain")
	}
	joined := strings.Join(out, "\n")
	if !strings.Contains(joined, mermaidBorder.Render("│")) {
		t.Errorf("no border run carries the frame style:\n%s", joined)
	}
	// The node text run is " Start " (box-padded), not "Start".
	if !strings.Contains(joined, mermaidText.Render(" Start ")) {
		t.Errorf("node text does not carry the text style:\n%s", joined)
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

// TestMermaidSequenceBlockRendersInModel: a sequence fence renders through the
// model too.
func TestMermaidSequenceBlockRendersInModel(t *testing.T) {
	src := "# Title\n\n```mermaid\nsequenceDiagram\n    participant A as Alice\n    participant B as Bob\n    A->>B: Hello Bob\n```\n"
	m := modelFromSource(t, src)
	plain := strings.Join(m.render(), "\n")
	for _, want := range []string{"Alice", "Bob", "Hello Bob"} {
		if !strings.Contains(plain, want) {
			t.Errorf("rendered document missing sequence %q", want)
		}
	}
	for _, gone := range []string{"sequenceDiagram", "-->>"} {
		if strings.Contains(plain, gone) {
			t.Errorf("sequence source leaked to the screen: %q", gone)
		}
	}
}

// TestMermaidUnsupportedBlockFallsBackToPlainSource: a mermaid block the
// renderer does not understand shows its source lines, not chroma colours and
// not a diagram.
func TestMermaidUnsupportedBlockFallsBackToPlainSource(t *testing.T) {
	src := "# Title\n\n```mermaid\nclassDiagram\n    Animal <|-- Dog\n```\n"
	m := modelFromSource(t, src)
	plain := strings.Join(m.render(), "\n")
	if !strings.Contains(plain, "classDiagram") {
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
