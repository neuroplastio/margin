package review

import (
	"strings"

	"github.com/AlexanderGrooff/mermaid-ascii/pkg/diagram"
	"github.com/AlexanderGrooff/mermaid-ascii/pkg/er"
	"github.com/AlexanderGrooff/mermaid-ascii/pkg/graph"
	"github.com/AlexanderGrooff/mermaid-ascii/pkg/sequence"
	"github.com/AlexanderGrooff/mermaid-ascii/pkg/state"
)

// mermaid.go renders fenced blocks whose info string is `mermaid` as ASCII
// diagrams. The renderer is the vendored
// github.com/AlexanderGrooff/mermaid-ascii library (third_party/mermaid-ascii,
// MIT), which handles graph/flowchart, sequence, ER and — through the in-tree
// extension delta D7 — state diagrams; the local deltas against upstream live
// in that copy's CHANGELOG.md. This file is a thin dispatcher: pick a vendored
// package, render, and on any failure fall back to the block's plain source
// lines — never chroma's meaningless colours on mermaid source, and never a
// half-parsed diagram.
//
// (The hand-rolled flowchart renderer that shipped in 2026-08-10.17 was
// deleted in the leg that vendored this library: the vendored graph layout is
// a real layered layout, and keeping a second flowchart renderer as a
// fallback would mean two parsers to maintain and two shapes for the same
// diagram. Anything unparseable falls back to plain source, so nothing is
// lost by the deletion.)

var (
	mermaidBorder = rawStyle
	mermaidText   = textStyle
)

// mermaidConfig is the shared rendering config. Graph direction comes from the
// block's own header (flowchart LR stays LR); only the shared fields are set
// here, and otherwise upstream's defaults hold (padding 5/5, border 1).
func mermaidConfig() *diagram.Config {
	cfg := diagram.DefaultConfig()
	cfg.StyleType = "cli"
	return cfg
}

// renderMermaid attempts to render a mermaid block's source lines as an ASCII
// diagram. ok is false when no vendored package can parse the block, in which
// case the caller shows plain source lines instead. A panic inside the
// vendored layout degrades to the same fallback — an unparseable diagram and
// a crashing one must behave identically, because neither may take the
// reviewer down.
func renderMermaid(lines []string) (out []string, ok bool) {
	defer func() {
		if recover() != nil {
			out, ok = nil, false
		}
	}()
	return renderMermaidDiagram(lines)
}

func renderMermaidDiagram(lines []string) ([]string, bool) {
	src := strings.Join(lines, "\n")
	// A leading YAML frontmatter block carries the diagram's title and theme
	// config, which mean nothing in ASCII; strip it and render the rest. The
	// title is dropped rather than printed, so a diagram stays one unit.
	rest, _ := diagram.StripFrontmatter(src)

	cfg := mermaidConfig()
	switch {
	case isMermaidKind(rest, sequence.SequenceDiagramKeyword):
		sd, err := sequence.Parse(rest)
		if err != nil {
			return nil, false
		}
		s, err := sequence.Render(sd, cfg)
		if err != nil {
			return nil, false
		}
		return styleMermaidDiagram(splitMermaidOutput(s)), true
	case state.IsStateDiagram(rest):
		sd, err := state.Parse(rest)
		if err != nil {
			return nil, false
		}
		s, err := state.Render(sd, cfg)
		if err != nil {
			return nil, false
		}
		return styleMermaidDiagram(splitMermaidOutput(s)), true
	case isMermaidKind(rest, er.Keyword):
		d, err := er.Parse(rest)
		if err != nil {
			return nil, false
		}
		return styleMermaidDiagram(splitMermaidOutput(er.Render(d, false))), true
	case isMermaidKind(rest, "flowchart"), isMermaidKind(rest, "graph"):
		g, err := graph.Parse(rest)
		if err != nil {
			return nil, false
		}
		s, err := graph.Render(g, cfg)
		if err != nil {
			return nil, false
		}
		return styleMermaidDiagram(splitMermaidOutput(s)), true
	}
	return nil, false
}

// isMermaidKind reports whether the block's first meaningful line declares
// the given diagram keyword (case-insensitive, as a whole token). Only the
// keyword's own package knows how to parse its body, so this is just routing.
func isMermaidKind(src, keyword string) bool {
	for _, line := range strings.Split(src, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "%%") {
			continue
		}
		lower := strings.ToLower(trimmed)
		kw := strings.ToLower(keyword)
		if !strings.HasPrefix(lower, kw) {
			return false
		}
		rest := lower[len(kw):]
		return rest == "" || rest[0] == ' ' || rest[0] == '\t'
	}
	return false
}

// splitMermaidOutput splits a vendored render's single string into lines,
// dropping a trailing blank line (every vendored Render ends with "\n").
func splitMermaidOutput(s string) []string {
	out := strings.Split(strings.TrimRight(s, "\n"), "\n")
	for i, l := range out {
		out[i] = strings.TrimRight(l, " ")
	}
	return out
}

// styleMermaidDiagram applies margin's styling to a vendored diagram's plain
// lines: box-drawing glyphs carry the muted frame colour, everything else the
// bright text colour. The flat output cannot tell a node's label from an
// edge's, so both read as text; that is the one styling loss versus the
// hand-rolled renderer and is flagged in the demo recipe.
func styleMermaidDiagram(lines []string) []string {
	styled := make([]string, len(lines))
	for i, line := range lines {
		styled[i] = styleMermaidLine(line)
	}
	return styled
}

func styleMermaidLine(line string) string {
	var b strings.Builder
	var run []rune
	box := false
	flush := func() {
		if len(run) == 0 {
			return
		}
		if box {
			b.WriteString(mermaidBorder.Render(string(run)))
		} else {
			b.WriteString(mermaidText.Render(string(run)))
		}
		run = run[:0]
	}
	for _, r := range line {
		if b := isMermaidBoxRune(r); b != box {
			flush()
			box = b
		}
		run = append(run, r)
	}
	flush()
	return b.String()
}

// isMermaidBoxRune reports whether r is a box-drawing glyph the diagram's
// frame is made of.
func isMermaidBoxRune(r rune) bool {
	switch r {
	case '─', '│', '┌', '┐', '└', '┘', '├', '┤', '┬', '┴', '┼', '╴', '╵', '╶', '╷', '╭', '╮', '╰', '╯',
		'▲', '▼', '◄', '►', '◢', '◣', '◤', '◥', '○', '●', '◯', '◆', '◇', '·', '┈', '┊', '═', '║':
		return true
	}
	return false
}
