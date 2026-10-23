package review

import (
	"slices"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
)

// TestRenderKeepsBlockBodies: a frame draws a block only when what it shows
// changed. Moving focus draws nothing again (the 2026-10-23 report: j, k and
// the wheel queued behind a 43 ms frame that highlighted and wrapped the
// whole document every time), while a mark, a sideways scroll, a new width
// or a rebuild each draw the block anew.
func TestRenderKeepsBlockBodies(t *testing.T) {
	isolateDrafts(t)
	src := "A paragraph long enough that a narrower window wraps it onto a line more than a wide one does.\n\n" +
		"```go\nfunc main() { println(\"a line of code wide enough to scroll sideways in a narrow window\") }\n```\n\n" +
		"Another paragraph.\n"
	m := newModel(parseDoc([]byte(src)), nil)
	m.w, m.h = 120, 40
	first := m.render()
	kept := len(m.bodies)
	if kept != 3 {
		t.Fatalf("render kept %d bodies, want one for each of the 3 blocks", kept)
	}

	for _, k := range []string{"j", "j", "k"} {
		m.handleKey(tea.KeyPressMsg(tea.Key{Code: rune(k[0]), Text: k}))
		m.render()
	}
	if len(m.bodies) != kept {
		t.Fatalf("moving focus drew %d bodies again", len(m.bodies)-kept)
	}

	m.at = cursor{entry: 0, comment: commentNone}
	m.toggleMark(markOK)
	marked := m.render()
	if len(m.bodies) != kept+1 {
		t.Fatalf("a mark drew %d bodies, want the marked block's", len(m.bodies)-kept)
	}
	if marked[0] == first[0] {
		t.Error("the reviewed paragraph still shows in the unreviewed style")
	}

	code := m.entries[1].b
	m.w = 60
	narrow := m.render()
	if slices.Equal(stripANSI(narrow), stripANSI(marked)) {
		t.Error("a narrower window drew the blocks at the old width")
	}
	m.codeScroll[code.anchor] = 8
	scrolled := m.render()
	codeRow := m.spans[1].start
	if stripANSI(scrolled)[codeRow] == stripANSI(narrow)[codeRow] {
		t.Errorf("scrolling the code block sideways left its row as it was: %q", stripANSI(scrolled)[codeRow])
	}

	m.rebuild()
	if m.bodies != nil {
		t.Fatalf("rebuild kept %d bodies of entries it replaced", len(m.bodies))
	}
}

func stripANSI(lines []string) []string {
	out := make([]string, len(lines))
	for i, l := range lines {
		out[i] = strings.TrimRight(ansiEscape.ReplaceAllString(l, ""), " ")
	}
	return out
}
