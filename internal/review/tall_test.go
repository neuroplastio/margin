package review

import (
	"testing"
)

// tallModel returns a model with three block entries — small, tall, small —
// and the spans a render would give them. Entry 1 spans 36 rows at viewport 22
// (m.h 24), so it is the "much taller than the viewport" case the
// tall-block-jk-incremental-scroll feedback names.
func tallModel() *model {
	m := seedModel()
	m.w, m.h = 100, 24 // viewport = m.h - footerRows = 22
	m.entries = []entry{{}, {}, {}}
	m.spans = []span{{start: 0, end: 4}, {start: 5, end: 40}, {start: 41, end: 45}}
	m.at = cursor{entry: 0, comment: commentNone}
	return m
}

// TestMoveFocusTallBlockLandsAtTop: walking down onto a block taller than the
// viewport opens it at its top, not its bottom — the walk should read the
// whole block in order, so the first screen shows where it begins.
func TestMoveFocusTallBlockLandsAtTop(t *testing.T) {
	m := tallModel()
	m.moveFocus(1)
	if m.at.entry != 1 {
		t.Fatalf("focus = entry %d, want 1", m.at.entry)
	}
	if want := 5; m.scroll != want {
		t.Fatalf("scroll = %d after landing on the tall block, want %d (its top row)", m.scroll, want)
	}
	if m.scrollAnchor != m.at {
		t.Fatalf("scrollAnchor = %+v, want the landed focus %+v so clampScroll keeps the top", m.scrollAnchor, m.at)
	}
}

// TestMoveFocusTallBlockScrollsInside: while focus sits on a tall block, j/k
// scroll the viewport tallWalkStep lines at a time and never move focus on —
// the block is one stop, but walking must not leap over it.
func TestMoveFocusTallBlockScrollsInside(t *testing.T) {
	m := tallModel()
	m.moveFocus(1) // land at the top, scroll 5
	for press := 0; press < 3; press++ {
		before := m.scroll
		m.moveFocus(1)
		if m.at.entry != 1 {
			t.Fatalf("press %d: focus left the tall block (entry %d), want it to stay", press, m.at.entry)
		}
		if want := min(40-22+1, before+tallWalkStep); m.scroll != want {
			t.Fatalf("press %d: scroll = %d, want %d", press, m.scroll, want)
		}
	}
	if want := 5 + 3*tallWalkStep; m.scroll != want {
		t.Fatalf("scroll = %d after three presses, want %d", m.scroll, want)
	}

	// The scroll must survive a render's clampScroll — the walk owns the
	// offset now, and clampScroll must not re-derive it from the block.
	if got := m.clampScroll(46, 22); got != m.scroll {
		t.Fatalf("clampScroll = %d, want the walked scroll %d preserved", got, m.scroll)
	}
}

// TestMoveFocusTallBlockMovesOnAtTheBottom: once the viewport reaches the
// tall block's last row, the next j moves focus to the neighbour.
func TestMoveFocusTallBlockMovesOnAtTheBottom(t *testing.T) {
	m := tallModel()
	m.moveFocus(1)         // land at the top
	m.scroll = 40 - 22 + 1 // bottom-aligned: rows 19..40 fill the viewport
	m.moveFocus(1)
	if m.at.entry != 2 {
		t.Fatalf("focus = entry %d after walking past the tall block, want 2", m.at.entry)
	}
}

// TestMoveFocusTallBlockWalksUp: walking up onto a tall block opens it at its
// bottom, k scrolls up through it, and at its top the next k moves focus on.
func TestMoveFocusTallBlockWalksUp(t *testing.T) {
	m := tallModel()
	m.at = cursor{entry: 2, comment: commentNone}
	m.moveFocus(-1) // land at the bottom: rows 19..40 fill the viewport
	if m.at.entry != 1 {
		t.Fatalf("focus = entry %d after walking up, want 1", m.at.entry)
	}
	if want := 40 - 22 + 1; m.scroll != want {
		t.Fatalf("scroll = %d, want %d (bottom-aligned on the tall block)", m.scroll, want)
	}
	if m.scrollAnchor != m.at {
		t.Fatalf("scrollAnchor = %+v, want %+v", m.scrollAnchor, m.at)
	}

	for m.scroll > 5 {
		before := m.scroll
		m.moveFocus(-1)
		if m.at.entry != 1 {
			t.Fatalf("k left the tall block early (entry %d at scroll %d)", m.at.entry, before)
		}
		if want := max(5, before-tallWalkStep); m.scroll != want {
			t.Fatalf("scroll = %d, want %d", m.scroll, want)
		}
	}

	m.moveFocus(-1) // at the top now
	if m.at.entry != 0 {
		t.Fatalf("focus = entry %d after walking past the tall block's top, want 0", m.at.entry)
	}
}

// TestMoveFocusTallBlockClampsAtEdges: at the first/last entry a tall block
// never walks off the document — j at the bottom of the last block and k at
// the top of the first are quiet no-ops that keep focus where it is.
func TestMoveFocusTallBlockClampsAtEdges(t *testing.T) {
	m := tallModel()
	m.at = cursor{entry: 2, comment: commentNone}
	m.moveFocus(1)
	if m.at.entry != 2 {
		t.Fatalf("focus = entry %d, want 2 (already last)", m.at.entry)
	}

	m = tallModel()
	m.scroll = 5 // already at the tall block's top row
	m.moveFocus(-1)
	if m.at.entry != 0 {
		t.Fatalf("k from the first entry moved focus to entry %d, want 0", m.at.entry)
	}
}

// TestMoveFocusTallBlockVisualKeepsTheLeap: visual mode does not scroll inside
// a tall block — j/k keep their blockwise leap so a selection extends across
// the whole block in one press.
func TestMoveFocusTallBlockVisualKeepsTheLeap(t *testing.T) {
	m := tallModel()
	m.visual = true
	m.moveFocus(1)
	if m.at.entry != 1 {
		t.Fatalf("focus = entry %d, want 1", m.at.entry)
	}
	if m.scroll != 0 {
		t.Fatalf("scroll = %d in visual mode, want the pre-move 0", m.scroll)
	}
	m.moveFocus(1)
	if m.at.entry != 2 {
		t.Fatalf("focus = entry %d after a second j, want 2 — the block was one leap", m.at.entry)
	}
}

// TestMoveFocusTallBlockRendered is the render-path check: walking a document
// whose code block is taller than the viewport scrolls through the block and
// only moves on once its tail is reached, with the walked offset surviving the
// render's clampScroll.
func TestMoveFocusTallBlockRendered(t *testing.T) {
	long := make([]string, 40)
	for i := range long {
		long[i] = "line of the long block"
	}
	m := newModelAt("tall.md", []block{
		{kind: blockPara, anchor: "^a", text: "before"},
		{kind: blockCode, anchor: "^b", lines: long},
		{kind: blockPara, anchor: "^c", text: "after"},
	}, nil)
	m.w, m.h = 100, 24
	m.render() // populate spans
	if len(m.spans) != 3 {
		t.Fatalf("render produced %d spans, want 3", len(m.spans))
	}
	tall := m.spans[1]
	if tall.end-tall.start+1 <= m.h-footerRows {
		t.Fatalf("test block is not taller than the viewport: span %+v at viewport %d", tall, m.h-footerRows)
	}

	m.at = cursor{entry: 0, comment: commentNone}
	m.moveFocus(1)
	if m.at.entry != 1 || m.scroll != tall.start {
		t.Fatalf("after j: focus entry %d scroll %d, want entry 1 scroll %d (top of the block)", m.at.entry, m.scroll, tall.start)
	}

	last := m.scroll
	for m.at.entry == 1 && m.scroll != tall.end-m.h+footerRows+1 {
		before := m.scroll
		m.moveFocus(1)
		if m.at.entry != 1 {
			break
		}
		if m.scroll <= before {
			t.Fatalf("j at scroll %d did not advance within the block (now %d)", before, m.scroll)
		}
		last = m.scroll
	}
	if m.at.entry != 1 {
		t.Fatalf("walk left the tall block before its tail: scroll %d, tail at %d", last, tall.end-m.h+footerRows+1)
	}
	if last != tall.end-m.h+footerRows+1 {
		t.Fatalf("walk stopped at scroll %d, want the bottom-aligned %d", last, tall.end-m.h+footerRows+1)
	}

	m.moveFocus(1)
	if m.at.entry != 2 {
		t.Fatalf("focus = entry %d after the block's tail, want 2", m.at.entry)
	}
}
