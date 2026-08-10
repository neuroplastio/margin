package state

import (
	"strings"
	"testing"

	"github.com/neuroplastio/margin/internal/mermaid-ascii/pkg/diagram"
)

func render(t *testing.T, src string, useAscii bool) string {
	t.Helper()
	d, err := Parse(src)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	cfg := diagram.DefaultConfig()
	cfg.UseAscii = useAscii
	cfg.BoxBorderPadding = 0
	cfg.PaddingBetweenX = 3
	cfg.PaddingBetweenY = 1
	out, err := Render(d, cfg)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	return out
}

// TestRenderChain: a simple chain renders each state once as a boxed flow with
// no source leaking through.
func TestRenderChain(t *testing.T) {
	out := render(t, "stateDiagram-v2\n[*] --> Idle\nIdle --> Active\nActive --> [*]", false)
	for _, want := range []string{"Idle", "Active", "○", "▼", "│"} {
		if !strings.Contains(out, want) {
			t.Errorf("rendered state diagram missing %q:\n%s", want, out)
		}
	}
	// Idle and Active each appear once — the graph lays one box per state.
	if n := strings.Count(out, "Idle"); n != 1 {
		t.Errorf("Idle drawn %d times, want 1:\n%s", n, out)
	}
	if n := strings.Count(out, "Active"); n != 1 {
		t.Errorf("Active drawn %d times, want 1:\n%s", n, out)
	}
	if strings.Contains(out, "[*]") || strings.Contains(out, "-->") {
		t.Errorf("state source leaked into the render:\n%s", out)
	}
}

// TestRenderLabels: a transition label sits beside the routed edge.
func TestRenderLabels(t *testing.T) {
	out := render(t, "stateDiagram-v2\nIdle --> Active: power on", false)
	if !strings.Contains(out, "power on") {
		t.Errorf("transition label missing:\n%s", out)
	}
}

// TestRenderDescription: the description, not the id, is what the box draws.
func TestRenderDescription(t *testing.T) {
	out := render(t, "stateDiagram-v2\nstate \"Idle all day\" as Idle\nIdle --> [*]", false)
	if !strings.Contains(out, "Idle all day") {
		t.Errorf("description label missing:\n%s", out)
	}
}

// TestRenderBranch: a branching source is one box with two routed arrows, not
// a duplicated box and not a ↩ reference.
func TestRenderBranch(t *testing.T) {
	out := render(t, "stateDiagram-v2\nIdle --> Active\nIdle --> Off", false)
	for _, want := range []string{"Idle", "Active", "Off"} {
		if !strings.Contains(out, want) {
			t.Errorf("branch render missing %q:\n%s", want, out)
		}
	}
	if n := strings.Count(out, "Idle"); n != 1 {
		t.Errorf("Idle drawn %d times, want 1 (branches share the box):\n%s", n, out)
	}
	if strings.Contains(out, "↩") {
		t.Errorf("branch used a ↩ reference instead of routing:\n%s", out)
	}
}

// TestRenderRevisitedState: a state re-entered as a transition target routes
// an arrow back to its box — the state appears once, no ↩ reference.
func TestRenderRevisitedState(t *testing.T) {
	out := render(t, "stateDiagram-v2\nA --> B\nB --> C\nB --> A", false)
	for _, want := range []string{"A", "B", "C"} {
		if !strings.Contains(out, want) {
			t.Errorf("revisit render missing %q:\n%s", want, out)
		}
	}
	if n := strings.Count(out, "A"); n != 1 {
		t.Errorf("A drawn %d times, want 1:\n%s", n, out)
	}
	if n := strings.Count(out, "B"); n != 1 {
		t.Errorf("B drawn %d times, want 1:\n%s", n, out)
	}
	if strings.Contains(out, "↩") {
		t.Errorf("revisit used a ↩ reference instead of routing:\n%s", out)
	}
}

// TestRenderStartEndMarker: the `[*]` start/end marker draws as a circle node.
func TestRenderStartEndMarker(t *testing.T) {
	out := render(t, "stateDiagram-v2\n[*] --> A\nA --> [*]", false)
	if !strings.Contains(out, "○") {
		t.Errorf("start/end marker missing:\n%s", out)
	}
	if strings.Contains(out, "[*]") {
		t.Errorf("marker source leaked:\n%s", out)
	}
}

// TestRenderNoTransitions: declared states with no transitions render as
// boxes.
func TestRenderNoTransitions(t *testing.T) {
	out := render(t, "stateDiagram-v2\nstate Alpha\nstate Beta", false)
	for _, want := range []string{"Alpha", "Beta"} {
		if !strings.Contains(out, want) {
			t.Errorf("stacked render missing %q:\n%s", want, out)
		}
	}
}

// TestRenderAscii: the ASCII glyph set replaces the box-drawing characters.
func TestRenderAscii(t *testing.T) {
	out := render(t, "stateDiagram-v2\nIdle --> Active", true)
	if !strings.Contains(out, "+") || strings.Contains(out, "┌") {
		t.Errorf("ASCII glyph set not applied:\n%s", out)
	}
}

// TestRenderAsciiMarker: the marker uses the ASCII circle glyph too.
func TestRenderAsciiMarker(t *testing.T) {
	out := render(t, "stateDiagram-v2\n[*] --> A", true)
	if !strings.Contains(out, "o") {
		t.Errorf("ASCII marker missing:\n%s", out)
	}
}
