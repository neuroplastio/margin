package mermaidsvg

import (
	"bytes"
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// palette is margin's colours as Ondine's public theme variables: the
// xterm-256 values of internal/review's stylesheet, so a diagram reads as
// part of the page around it. bg is what a label masks the line behind it
// with; surface fills nodes. The series are the charts' colours, in order.
var palette = map[string]string{
	"--ondine-bg":          "#1c1c1c", // 234
	"--ondine-fg":          "#d0d0d0", // 252, text
	"--ondine-muted":       "#808080", // 244, secondary text
	"--ondine-line":        "#87afff", // 111, as links: edges and arrowheads
	"--ondine-surface":     "#262626", // 235, node fill
	"--ondine-surface-alt": "#303030", // 236, subgraphs, grids, the selection's
	"--ondine-border":      "#808080", // 244, node outlines
	"--ondine-accent":      "#87afff", // 111
	"--ondine-critical":    "#ff8787", // 210
	"--ondine-stroke":      "1.5px",
	"--ondine-series-1":    "#87afff", // 111
	"--ondine-series-2":    "#ff87d7", // 212
	"--ondine-series-3":    "#ffaf87", // 216
	"--ondine-series-4":    "#d7afff", // 183
	"--ondine-series-5":    "#87d7af", // 115
	"--ondine-series-6":    "#d7d787", // 186
	"--ondine-series-7":    "#87d7d7", // 116
	"--ondine-series-8":    "#ff8787", // 210
}

var rootStyle = regexp.MustCompile(`^<svg\b[^>]*?\sstyle="([^"]*)"`)

// flatten writes margin's palette into an SVG Ondine drew, for a painter with
// no custom properties and no color-mix(), as usvg has neither. Every var()
// becomes the value the root declares for it, palette first, else its
// fallback; every color-mix() then becomes the colour it computes; and the
// root's custom-property declarations go, leaving only what usvg reads.
// Ondine's own terminal output does the same for resvg.
func flatten(svg []byte) []byte {
	m := rootStyle.FindSubmatchIndex(svg)
	if m == nil {
		return svg
	}
	declared := map[string]string{}
	var kept []string
	for _, d := range splitTop(string(svg[m[2]:m[3]]), ';') {
		name, value, ok := strings.Cut(d, ":")
		switch name = strings.TrimSpace(name); {
		case !ok || name == "":
		case strings.HasPrefix(name, "--"):
			declared[name] = strings.TrimSpace(value)
		default:
			kept = append(kept, name+":"+strings.TrimSpace(value))
		}
	}
	for name, value := range palette {
		declared[name] = value
	}

	var out bytes.Buffer
	out.Grow(len(svg))
	out.Write(svg[:m[2]])
	out.WriteString(mix(resolve(strings.Join(kept, ";"), declared, 0)))
	out.WriteString(mix(resolve(string(svg[m[3]:]), declared, 0)))
	return out.Bytes()
}

// resolve replaces each var(--name, fallback) in s with the value declared
// for name, or the fallback, itself resolved. Ondine's declarations refer to
// one another a few levels deep and never in a cycle; depth stops one anyway.
func resolve(s string, declared map[string]string, depth int) string {
	if depth > 8 || !strings.Contains(s, "var(") {
		return s
	}
	var out strings.Builder
	for {
		start := strings.Index(s, "var(")
		if start < 0 {
			out.WriteString(s)
			return out.String()
		}
		end := closing(s, start+3)
		inner := s[start+4 : end]
		name, fallback, _ := strings.Cut(inner, ",")
		value, ok := declared[strings.TrimSpace(name)]
		if !ok {
			value = strings.TrimSpace(fallback)
		}
		out.WriteString(s[:start])
		out.WriteString(resolve(value, declared, depth+1))
		if end == len(s) {
			return out.String()
		}
		s = s[end+1:]
	}
}

// mix replaces each color-mix(in srgb, A p%, B q%) in s with the colour it
// computes, innermost first. One it cannot compute becomes its first colour.
func mix(s string) string {
	if !strings.Contains(s, "color-mix(") {
		return s
	}
	var out strings.Builder
	for {
		start := strings.Index(s, "color-mix(")
		if start < 0 {
			out.WriteString(s)
			return out.String()
		}
		end := closing(s, start+9)
		inner := mix(s[start+10 : end])
		out.WriteString(s[:start])
		out.WriteString(mixed(inner))
		if end == len(s) {
			return out.String()
		}
		s = s[end+1:]
	}
}

// mixed computes the arguments of one color-mix() in srgb: each colour's
// share is its percentage, or what the other's leaves, or half.
func mixed(args string) string {
	parts := splitTop(args, ',')
	if len(parts) != 3 {
		return strings.TrimSpace(args)
	}
	stop := func(s string) (string, float64, bool) {
		f := strings.Fields(s)
		if len(f) == 2 && strings.HasSuffix(f[1], "%") {
			p, err := strconv.ParseFloat(strings.TrimSuffix(f[1], "%"), 64)
			return f[0], p / 100, err == nil
		}
		return strings.TrimSpace(s), 0, false
	}
	c1, p1, has1 := stop(parts[1])
	c2, p2, has2 := stop(parts[2])
	switch {
	case has1 && has2 && p1+p2 > 0:
		p1 = p1 / (p1 + p2)
	case has1:
	case has2:
		p1 = 1 - p2
	default:
		p1 = 0.5
	}
	a, ok1 := rgb(c1)
	b, ok2 := rgb(c2)
	if strings.TrimSpace(parts[0]) != "in srgb" || !ok1 || !ok2 {
		return c1
	}
	var c [3]int
	for i := range c {
		c[i] = int(float64(a[i])*p1 + float64(b[i])*(1-p1) + 0.5)
	}
	return fmt.Sprintf("#%02x%02x%02x", c[0], c[1], c[2])
}

// rgb reads a #rgb or #rrggbb colour.
func rgb(s string) ([3]int, bool) {
	var c [3]int
	h, ok := strings.CutPrefix(strings.TrimSpace(s), "#")
	if !ok {
		return c, false
	}
	if len(h) == 3 {
		h = string([]byte{h[0], h[0], h[1], h[1], h[2], h[2]})
	}
	if len(h) != 6 {
		return c, false
	}
	for i := range c {
		v, err := strconv.ParseUint(h[2*i:2*i+2], 16, 8)
		if err != nil {
			return c, false
		}
		c[i] = int(v)
	}
	return c, true
}

// closing is the index of the parenthesis that closes the one at open, or
// len(s) when none does.
func closing(s string, open int) int {
	depth := 0
	for i := open; i < len(s); i++ {
		switch s[i] {
		case '(':
			depth++
		case ')':
			if depth--; depth == 0 {
				return i
			}
		}
	}
	return len(s)
}

// splitTop splits s at each sep outside parentheses.
func splitTop(s string, sep byte) []string {
	var parts []string
	depth, last := 0, 0
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case '(':
			depth++
		case ')':
			depth--
		case sep:
			if depth == 0 {
				parts = append(parts, s[last:i])
				last = i + 1
			}
		}
	}
	return append(parts, s[last:])
}
