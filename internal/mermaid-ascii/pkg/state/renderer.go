package state

import (
	"fmt"

	"github.com/neuroplastio/margin/internal/mermaid-ascii/pkg/diagram"
	"github.com/neuroplastio/margin/internal/mermaid-ascii/pkg/graph"
)

// Render draws a parsed state diagram as a routed graph (delta D9): each
// state becomes a node and each transition a labelled edge, and the vendored
// graph engine's layered layout and A* edge router lay them out — so a
// branch, a re-entered state or a cycle draws real arrows back to the shared
// box instead of a `↩` shortcut. The `[*]` start/end marker becomes a circle
// node. config may be nil for the defaults.
func Render(d *StateDiagram, config *diagram.Config) (string, error) {
	if d == nil || len(d.States) == 0 {
		return "", fmt.Errorf("no states")
	}
	if config == nil {
		config = diagram.DefaultConfig()
	}
	// A routed edge enters and leaves a box through its border, where the
	// junction glyph overwrites whatever cell it lands on. With no horizontal
	// padding a label that fills its box loses its edge character to the
	// `├`/`┤` (observed: `Buildin├`), so state boxes keep at least 1 column of
	// it; vertical padding is pure air, so it stays at 0 and a box is three
	// rows tall, not five.
	cfg := *config
	if cfg.BoxBorderPadding < 1 {
		cfg.BoxBorderPadding = 1
	}
	if cfg.BoxPaddingY < 0 {
		cfg.BoxPaddingY = 0
	}
	config = &cfg

	nodes := make([]graph.NodeSpec, 0, len(d.States))
	marker := "○"
	if config.UseAscii {
		marker = "o"
	}
	for _, s := range d.States {
		label := s.Label
		if s.ID == StartEndID {
			label = marker
		}
		nodes = append(nodes, graph.NodeSpec{
			ID:    nodeID(s.ID),
			Label: label,
			Shape: nodeShape(s.ID),
		})
	}
	edges := make([]graph.EdgeSpec, 0, len(d.Transitions))
	for _, t := range d.Transitions {
		edges = append(edges, graph.EdgeSpec{
			From:  nodeID(t.From.ID),
			To:    nodeID(t.To.ID),
			Label: t.Label,
		})
	}

	g := graph.NewDiagram("TD", nodes, edges)
	return graph.Render(g, config)
}

// nodeID maps a state id to a graph node id. The marker keeps its own id so
// every `[*]` in a diagram — start and end alike — refers to the same circle.
func nodeID(id string) string {
	if id == StartEndID {
		return "*"
	}
	return id
}

// nodeShape names the graph shape for a state: circles for the `[*]` marker,
// rectangles for everything else.
func nodeShape(id string) string {
	if id == StartEndID {
		return "circle"
	}
	return ""
}
