package state

import (
	"fmt"
	"strings"

	"github.com/neuroplastio/margin/third_party/mermaid-ascii/pkg/diagram"
	"github.com/mattn/go-runewidth"
)

// box-drawing glyphs (Unicode by default, ASCII when useAscii).
type glyphs struct {
	h, v, tl, tr, bl, br rune
	arrow, circle        rune
}

var unicodeGlyphs = glyphs{'─', '│', '┌', '┐', '└', '┘', '▼', '○'}
var asciiGlyphs = glyphs{'-', '|', '+', '+', '+', '+', 'v', 'o'}

// Render draws a parsed state diagram top-down on a single arrow spine: each
// transition renders a `│ label` run and a `▼` head between its source and
// target boxes, every box centred on the same spine column. Consecutive
// transitions that share a state keep the box it drew (a chain reads as one
// flow), while a branching source draws its box again for each branch — the
// simple tradeoff for not routing edges. config may be nil for the defaults.
func Render(d *StateDiagram, config *diagram.Config) (string, error) {
	if d == nil || len(d.States) == 0 {
		return "", fmt.Errorf("no states")
	}
	if config == nil {
		config = diagram.DefaultConfig()
	}
	g := unicodeGlyphs
	if config.UseAscii {
		g = asciiGlyphs
	}

	// The spine column every box and arrow centres on; the widest box then
	// starts exactly at column 0 (see renderBox's padding).
	col := maxStateLabel(d)/2 + 2

	var lines []string
	if len(d.Transitions) == 0 {
		// States with no transitions render as a stacked list of boxes.
		for i, s := range d.States {
			if i > 0 {
				lines = append(lines, "")
			}
			lines = append(lines, renderBox(s, col, g)...)
		}
	} else {
		current := ""
		for _, t := range d.Transitions {
			if t.From.ID != current {
				lines = append(lines, renderBox(t.From, col, g)...)
				current = t.From.ID
			}
			lines = append(lines, renderArrow(t.Label, col, g)...)
			lines = append(lines, renderBox(t.To, col, g)...)
			current = t.To.ID
		}
	}
	return strings.Join(lines, "\n") + "\n", nil
}

// renderBox draws a state as a box centred on the spine column — or the
// `[*]` start/end marker as a single circle.
func renderBox(s *State, col int, g glyphs) []string {
	if s.ID == StartEndID {
		return []string{strings.Repeat(" ", col) + string(g.circle)}
	}
	inner := runewidth.StringWidth(s.Label) + 2
	line := func(b string) string {
		pad := col - runewidth.StringWidth(b)/2
		if pad < 0 {
			pad = 0
		}
		return strings.Repeat(" ", pad) + b
	}
	return []string{
		line(string(g.tl) + strings.Repeat(string(g.h), inner) + string(g.tr)),
		line(string(g.v) + " " + s.Label + " " + string(g.v)),
		line(string(g.bl) + strings.Repeat(string(g.h), inner) + string(g.br)),
	}
}

// renderArrow draws a transition's arrow down the spine: a vertical run
// carrying the label (if any) to the right of the spine, then the arrowhead.
func renderArrow(label string, col int, g glyphs) []string {
	pad := strings.Repeat(" ", col)
	if label == "" {
		return []string{pad + string(g.v), pad + string(g.arrow)}
	}
	return []string{pad + string(g.v) + " " + label, pad + string(g.arrow)}
}

// maxStateLabel returns the widest state box label (markers are width 1 and
// excluded), so the spine column can fit every box.
func maxStateLabel(d *StateDiagram) int {
	max := 0
	for _, s := range d.States {
		if s.ID == StartEndID {
			continue
		}
		if w := runewidth.StringWidth(s.Label); w > max {
			max = w
		}
	}
	return max
}
