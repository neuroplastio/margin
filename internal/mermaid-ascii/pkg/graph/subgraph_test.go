package graph

import (
	"fmt"
	"strings"
	"testing"

	"github.com/neuroplastio/margin/internal/mermaid-ascii/pkg/diagram"
)

// subgraph_test.go exercises the banded placement (delta D10): a graph with
// subgraphs lays each subgraph's nodes out in its own contiguous band of the
// grid, so a subgraph's frame always contains its nodes and two frames never
// overlap. The maintainer's reproduction — three subgraphs, ~40 nodes, mixed
// shapes, dotted cross-subgraph edges — used to draw its three frames
// interleaved, a node on a frame border, and frames crashing into each other.

// reproDiagram is the anonymized reproduction from
// vault/feedback/2026-08-12-mermaid-layout.md (structure, shapes, subgraph
// boundaries and label lengths kept; service names and endpoints replaced).
const reproDiagram = `flowchart TB
  subgraph a ["Node A · every 60s"]
    set[("item set<br/>added at item creation<br/>removed at item:end / item:failed")]
    set --> post["POST /api/v1/items/sync<br/>{id, ids}"]
    kill["item.finish"] --> bye["END both paths → item:end"]
    bye --> fin["/api/v1/items/end → row written, row deleted"]
  end

  subgraph b ["Node B"]
    post --> renew["live = now + 120s<br/>for every named id"]
    renew --> ans{"done?"}
    ans -->|no| reply["reply"]
    ans -->|yes| ext["Extend"]
    ext --> v{"verdict"}
    v -->|"accepted · session gone · error"| reply
    v -->|"insufficient balance"| cut["add to drop list"]
    cut --> reply
  end

  reply -.->|"drop list"| kill
  renew -.-> readers["every reader asks live > now()<br/>limit · list · detail"]

  subgraph c ["Node C · every 60s · every instance"]
    r0(["pass"]) --> r1{"a sync arrived<br/>within 120s?"}
    r1 -->|no| r2["skip this window"]
    r1 -->|yes| r3["claim live <= now<br/>stamping state"]
    r3 --> r4{"record<br/>already exists?"}
    r4 -->|yes| r5["delete row · recorded"]
    r4 -->|no| r6["write row with time"]
    r6 --> r5
  end
`

// layoutGraph mirrors drawMap's build steps so the test can inspect the graph
// after mapping instead of just the rendered string.
func layoutGraph(t *testing.T, src string, cfg *diagram.Config) *graph {
	t.Helper()
	props, err := Parse(src)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if cfg != nil {
		props.boxBorderPadding = cfg.BoxBorderPadding
		if cfg.BoxPaddingY >= 0 {
			props.boxPaddingY = cfg.BoxPaddingY
		}
		props.paddingX = cfg.PaddingBetweenX
		props.paddingY = cfg.PaddingBetweenY
		props.useAscii = cfg.UseAscii
	}
	g := mkGraph(props.data, props.nodeSpecs)
	g.setStyleClasses(props)
	g.paddingX = props.paddingX
	g.paddingY = props.paddingY
	g.useAscii = props.useAscii
	g.setSubgraphs(props.subgraphs)
	g.createMapping()
	return &g
}

func boxesOverlap(a, b *subgraph) bool {
	return a.minX <= b.maxX && b.minX <= a.maxX && a.minY <= b.maxY && b.minY <= a.maxY
}

// TestSubgraphBoxesAreDisjoint: after the banded placement every node is
// placed, and the root subgraphs' frames are pairwise disjoint — the repro
// used to draw them interleaved.
func TestSubgraphBoxesAreDisjoint(t *testing.T) {
	g := layoutGraph(t, reproDiagram, diagram.DefaultConfig())

	if len(g.subgraphs) != 3 {
		t.Fatalf("want 3 subgraphs, got %d", len(g.subgraphs))
	}
	// A subgraph with zero nodes is skipped by the banded placement; the
	// bounding box calc skips it too, and its zero rectangle would "overlap"
	// nothing but would also not be meaningful to compare.
	nonEmpty := []*subgraph{}
	for _, sg := range g.subgraphs {
		if len(sg.nodes) > 0 {
			nonEmpty = append(nonEmpty, sg)
		}
	}
	for i := 0; i < len(nonEmpty); i++ {
		for j := i + 1; j < len(nonEmpty); j++ {
			if boxesOverlap(nonEmpty[i], nonEmpty[j]) {
				t.Errorf("subgraph frames %q and %q overlap: %s vs %s",
					nonEmpty[i].name, nonEmpty[j].name,
					boxStr(nonEmpty[i]), boxStr(nonEmpty[j]))
			}
		}
	}
}

// TestSubgraphBoxesContainTheirNodes: every node of a subgraph draws inside
// that subgraph's frame — the repro used to draw nodes on top of their own
// subgraph's border.
func TestSubgraphBoxesContainTheirNodes(t *testing.T) {
	g := layoutGraph(t, reproDiagram, diagram.DefaultConfig())

	for _, sg := range g.subgraphs {
		if len(sg.nodes) == 0 {
			continue
		}
		for _, n := range sg.nodes {
			if n.drawingCoord == nil {
				t.Fatalf("node %q has no drawing coordinate", n.name)
			}
			w := g.columnWidth[n.gridCoord.x] + g.columnWidth[n.gridCoord.x+1]
			h := g.rowHeight[n.gridCoord.y] + g.rowHeight[n.gridCoord.y+1]
			if n.drawingCoord.x < sg.minX || n.drawingCoord.x+w > sg.maxX ||
				n.drawingCoord.y < sg.minY || n.drawingCoord.y+h > sg.maxY {
				t.Errorf("node %q (%s) escapes its subgraph frame %s",
					n.name, boxStr(&subgraph{minX: n.drawingCoord.x, minY: n.drawingCoord.y, maxX: n.drawingCoord.x + w, maxY: n.drawingCoord.y + h}), boxStr(sg))
			}
		}
	}
}

// TestSubgraphReproRendersAllFrames: the repro renders through the public
// API and every subgraph title survives.
func TestSubgraphReproRendersAllFrames(t *testing.T) {
	props, err := Parse(reproDiagram)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	out, err := Render(props, diagram.DefaultConfig())
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	for _, want := range []string{"Node A · every 60s", "Node B", "Node C · every 60s · every instance"} {
		if !strings.Contains(out, want) {
			t.Errorf("rendered repro missing subgraph title %q", want)
		}
	}
	// The subgraph headers themselves must not leak as node ids.
	for _, gone := range []string{"subgraph", "end\n"} {
		if strings.Contains(out, gone) {
			t.Errorf("subgraph source leaked into the render: %q", gone)
		}
	}
}

// TestFlatGraphUntouchedByBanding: a graph with no subgraphs still goes
// through the plain placement and renders the same as before the delta.
func TestFlatGraphUntouchedByBanding(t *testing.T) {
	src := "flowchart TD\n  A[Start] --> B{Decision}\n  B -->|yes| C[Do it]\n  B -->|no| D[Skip it]\n"
	props, err := Parse(src)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	cfg := diagram.DefaultConfig()
	cfg.StyleType = "cli"
	out, err := Render(props, cfg)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	for _, want := range []string{"Start", "Decision", "Do it", "Skip it", "yes", "no"} {
		if !strings.Contains(out, want) {
			t.Errorf("flat graph missing %q:\n%s", want, out)
		}
	}
	for _, gone := range []string{"A[", "B{", "-->"} {
		if strings.Contains(out, gone) {
			t.Errorf("flat graph leaked source %q:\n%s", gone, out)
		}
	}
}

// TestLRSubgraphBandsAreDisjoint: the banded placement mirrors along the flow
// axis, so LR subgraphs stack their bands vertically instead of overlapping.
func TestLRSubgraphBandsAreDisjoint(t *testing.T) {
	src := `flowchart LR
  subgraph a ["left"]
    A[Alpha] --> B[Beta]
  end
  subgraph b ["right"]
    C[Gamma] --> D[Delta]
  end
  B --> C
`
	g := layoutGraph(t, src, diagram.DefaultConfig())
	if len(g.subgraphs) != 2 {
		t.Fatalf("want 2 subgraphs, got %d", len(g.subgraphs))
	}
	if boxesOverlap(g.subgraphs[0], g.subgraphs[1]) {
		t.Errorf("LR subgraph frames overlap: %s vs %s", boxStr(g.subgraphs[0]), boxStr(g.subgraphs[1]))
	}
}

func boxStr(sg *subgraph) string {
	return fmt.Sprintf("(%d,%d,%d,%d)", sg.minX, sg.minY, sg.maxX, sg.maxY)
}
