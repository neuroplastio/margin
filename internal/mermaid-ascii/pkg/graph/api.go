package graph

import (
	"github.com/neuroplastio/margin/internal/mermaid-ascii/pkg/diagram"
)

// This file is part of the margin local delta set (see CHANGELOG.md, delta
// D2): upstream kept the graph renderer in the cobra CLI's `cmd` package; the
// extraction moved it here as a library package and added this thin exported
// API so the other diagram packages' shape (Parse + Render against a
// *diagram.Config) is uniform. Entry points are mermaidFileToMap/drawMap,
// unchanged from upstream.

// The upstream CLI exposed these as cobra flags (cmd/root.go); the extracted
// library carries them as package defaults. Coords turns on coordinate
// debugging output. None are reachable from margin, which renders through
// Render with its own config.
var (
	Coords           = false
	boxBorderPadding = 1
	paddingBetweenX  = 5
	paddingBetweenY  = 5
)

// Parse parses a mermaid `graph`/`flowchart` document. The parser is strict
// (delta D3): any statement it does not understand fails the whole document,
// so a caller can fall back to plain source instead of rendering a
// half-parsed diagram.
func Parse(input string) (*graphProperties, error) {
	return mermaidFileToMap(input, "cli")
}

// NodeSpec is a programmatic graph node (delta D9). Shape is "" for a
// rectangle or "circle" for a rounded stadium node.
type NodeSpec struct {
	ID    string
	Label string
	Shape string
}

// EdgeSpec is a programmatic directed edge with an optional label.
type EdgeSpec struct {
	From  string
	To    string
	Label string
}

// NewDiagram builds a graph from programmatic nodes and edges (delta D9) —
// the bridge that lets the state renderer route a state diagram through the
// same layered layout and A* edge router as flowcharts, instead of keeping a
// second, simpler renderer. direction is "TD" or "LR". It is the caller's
// job to use ids consistently across nodes and edges.
func NewDiagram(direction string, nodes []NodeSpec, edges []EdgeSpec) *graphProperties {
	data := newTextGraphData()
	nodeSpecs := make(map[string]graphNodeSpec)
	for _, ns := range nodes {
		spec := graphNodeSpec{
			label: newGraphLabel(ns.Label),
			shape: ns.Shape,
		}
		if ns.Label != "" {
			spec.labelIsExplicit = true
		}
		nodeSpecs[ns.ID] = spec
		data.Set(ns.ID, []textEdge{})
	}
	for _, es := range edges {
		from := textNode{name: es.From}
		to := textNode{name: es.To}
		children, _ := data.Get(es.From)
		data.Set(es.From, append(children, textEdge{parent: from, child: to, label: es.Label}))
		rememberNode(from, nodeSpecs)
		rememberNode(to, nodeSpecs)
		if _, ok := data.Get(es.To); !ok {
			data.Set(es.To, []textEdge{})
		}
	}
	gd := direction
	if gd != "LR" && gd != "TD" {
		gd = "TD"
	}
	return &graphProperties{
		data:             data,
		nodeSpecs:        nodeSpecs,
		styleClasses:     &map[string]styleClass{},
		boxBorderPadding: boxBorderPadding,
		boxPaddingY:      -1,
		graphDirection:   gd,
		styleType:        "cli",
		paddingX:         paddingBetweenX,
		paddingY:         paddingBetweenY,
		subgraphs:        []*textSubgraph{},
	}
}

// Render draws a parsed graph. config may be nil to use the package defaults.
func Render(properties *graphProperties, config *diagram.Config) (string, error) {
	if properties == nil {
		return "", nil
	}
	if config != nil {
		properties.boxBorderPadding = config.BoxBorderPadding
		if config.BoxPaddingY >= 0 {
			properties.boxPaddingY = config.BoxPaddingY
		}
		properties.paddingX = config.PaddingBetweenX
		properties.paddingY = config.PaddingBetweenY
		properties.useAscii = config.UseAscii
	}
	return drawMap(properties), nil
}
