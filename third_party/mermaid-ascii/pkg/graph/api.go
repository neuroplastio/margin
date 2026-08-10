package graph

import (
	"github.com/AlexanderGrooff/mermaid-ascii/pkg/diagram"
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

// Render draws a parsed graph. config may be nil to use the package defaults.
func Render(properties *graphProperties, config *diagram.Config) (string, error) {
	if properties == nil {
		return "", nil
	}
	if config != nil {
		properties.boxBorderPadding = config.BoxBorderPadding
		properties.paddingX = config.PaddingBetweenX
		properties.paddingY = config.PaddingBetweenY
		properties.useAscii = config.UseAscii
	}
	return drawMap(properties), nil
}
