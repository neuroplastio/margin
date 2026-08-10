package state

import (
	"strings"
	"testing"

	"github.com/neuroplastio/margin/third_party/mermaid-ascii/pkg/diagram"
)

func render(t *testing.T, src string, useAscii bool) string {
	t.Helper()
	d, err := Parse(src)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	cfg := diagram.DefaultConfig()
	cfg.UseAscii = useAscii
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
	// Idle and Active each appear once — the chain shares boxes, it does not
	// re-draw them.
	if n := strings.Count(out, "│ Idle │"); n != 1 {
		t.Errorf("Idle box drawn %d times, want 1:\n%s", n, out)
	}
	if strings.Contains(out, "[*]") || strings.Contains(out, "-->") {
		t.Errorf("state source leaked into the render:\n%s", out)
	}
}

// TestRenderLabels: a transition label sits beside the arrow spine.
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

// TestRenderBranch: a branching source draws its box again for each branch,
// so every destination is reachable.
func TestRenderBranch(t *testing.T) {
	out := render(t, "stateDiagram-v2\nIdle --> Active\nIdle --> Off", false)
	for _, want := range []string{"Idle", "Active", "Off"} {
		if !strings.Contains(out, want) {
			t.Errorf("branch render missing %q:\n%s", want, out)
		}
	}
	if n := strings.Count(out, "│ Idle │"); n != 2 {
		t.Errorf("Idle box drawn %d times, want 2 (one per branch):\n%s", n, out)
	}
}

// TestRenderNoTransitions: declared states with no transitions stack as boxes.
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
