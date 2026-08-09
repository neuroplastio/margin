package review

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
)

// The line dive, from the 2026-08-09 todo-review feedback's "Dive into
// multi-line blocks" bullet: l on a block whose source lines each render as
// a row (a table or a raw block) dives into its lines, j/k walk them, h
// surfaces, and c/gy anchor a comment to the line under focus (the L12-18
// payload). Paragraphs and quotes wrap, so their source lines have no visual
// identity and they stay out of the dive; code blocks keep l/h for horizontal
// scroll.

const lineDiveDoc = `# Header

| A | B |
| - | - |
| x | y |
| z | w |

Final paragraph.
`

// lineDiveTable returns a model over lineDiveDoc with focus on the table.
func lineDiveTable(t *testing.T) *model {
	t.Helper()
	m := newModelAt("doc.md", parseDoc([]byte(lineDiveDoc)), nil)
	m.w, m.h = 100, 60
	for i, e := range m.entries {
		if e.b.kind == blockTable {
			m.at = cursor{entry: i, comment: commentNone}
			return m
		}
	}
	t.Fatal("no table in lineDiveDoc")
	return nil
}

// TestLineDiveEntersTableLines: l on a multi-line table moves focus onto its
// first source line — the same dedicated dive that steps into a thread, now
// stepping into a block.
func TestLineDiveEntersTableLines(t *testing.T) {
	m := lineDiveTable(t)
	lo, hi, ok := m.lineDiveRange()
	if !ok || lo != 3 || hi != 6 {
		t.Fatalf("lineDiveRange = (%d, %d, %v), want (3, 6, true)", lo, hi, ok)
	}

	m.dive()
	if m.at.comment != commentNone {
		t.Fatalf("dive left a comment cursor: %+v", m.at)
	}
	if m.at.line != lo {
		t.Fatalf("dive landed on line %d, want %d", m.at.line, lo)
	}
}

// TestLineDiveWalksSourceLines: inside the dive, j/k step through the block's
// source lines in order.
func TestLineDiveWalksSourceLines(t *testing.T) {
	m := lineDiveTable(t)
	m.dive()

	m.moveFocus(1)
	if m.at.line != 4 {
		t.Fatalf("j from line 3 = line %d, want 4", m.at.line)
	}
	m.moveFocus(1)
	if m.at.line != 5 {
		t.Fatalf("j from line 4 = line %d, want 5", m.at.line)
	}
	m.moveFocus(-1)
	if m.at.line != 4 {
		t.Fatalf("k from line 5 = line %d, want 4", m.at.line)
	}
}

// TestLineDivePopsOutAtTheEnds: like the thread dive, a line dive never
// traps — j past the last line lands on the next entry, k past the first
// surfaces back onto the block.
func TestLineDivePopsOutAtTheEnds(t *testing.T) {
	m := lineDiveTable(t)
	tableEntry := m.at.entry

	m.dive()
	for m.at.line < 6 {
		m.moveFocus(1)
	}
	m.moveFocus(1)
	if m.at != (cursor{entry: tableEntry + 1, comment: commentNone}) {
		t.Fatalf("j past the last line = %+v, want the next entry (%d)", m.at, tableEntry+1)
	}

	m.at = cursor{entry: tableEntry, comment: commentNone}
	m.dive()
	m.moveFocus(-1)
	if m.at != (cursor{entry: tableEntry, comment: commentNone}) {
		t.Fatalf("k past the first line = %+v, want the block", m.at)
	}
}

// TestLineDiveSurfaces: h leaves the dive for the block, and does nothing at
// block level.
func TestLineDiveSurfaces(t *testing.T) {
	m := lineDiveTable(t)
	tableEntry := m.at.entry
	m.dive()
	if m.at.line == 0 {
		t.Fatal("dive did not enter a line")
	}

	m.surface()
	if m.at != (cursor{entry: tableEntry, comment: commentNone}) {
		t.Fatalf("surface = %+v, want the block", m.at)
	}
	m.surface()
	if m.at != (cursor{entry: tableEntry, comment: commentNone}) {
		t.Fatalf("surface at block level moved focus to %+v", m.at)
	}
}

// TestLineDiveKeysRouteThroughTheRegistry: l on a diveable block dives, h
// surfaces, via the same commands threads use.
func TestLineDiveKeysRouteThroughTheRegistry(t *testing.T) {
	m := lineDiveTable(t)
	tableEntry := m.at.entry

	m.handleKey(tea.KeyPressMsg(tea.Key{Code: 'l', Text: "l"}))
	if m.at.line == 0 {
		t.Fatalf("l did not dive into lines: %+v", m.at)
	}
	m.handleKey(tea.KeyPressMsg(tea.Key{Code: 'h', Text: "h"}))
	if m.at != (cursor{entry: tableEntry, comment: commentNone}) {
		t.Fatalf("h did not surface: %+v", m.at)
	}
}

// TestLineDiveCommentPrefix: c while dived prepends the single-line reference
// to the comment draft — the L12-18 payload this dive exists for.
func TestLineDiveCommentPrefix(t *testing.T) {
	m := lineDiveTable(t)
	m.dive()
	for m.at.line < 5 {
		m.moveFocus(1)
	}
	if m.at.line != 5 {
		t.Fatalf("dive parked on line %d, want 5", m.at.line)
	}

	m.handleKey(tea.KeyPressMsg(tea.Key{Code: 'c', Text: "c"}))
	a := m.anchorAt()
	tr := m.threads[a]
	if tr == nil {
		t.Fatalf("no thread created for anchor %q", a)
	}
	draft := tr.draft(newCommentSlot)
	if !strings.HasPrefix(draft, "L5: ") {
		t.Errorf("draft = %q, want the L5: prefix", draft)
	}
}

// TestLineDiveYankRef: gy while dived yanks the focused line's reference, not
// the whole block's range.
func TestLineDiveYankRef(t *testing.T) {
	m := lineDiveTable(t)
	m.dive()
	for m.at.line < 4 {
		m.moveFocus(1)
	}

	cmd := m.yankRef()
	if cmd == nil {
		t.Fatal("yankRef returned nil command")
	}
	if !strings.Contains(m.status, "doc.md:L4") {
		t.Errorf("status = %q, want the focused line reference doc.md:L4", m.status)
	}
	if m.at.line != 4 {
		t.Fatalf("yankRef moved the dive off line 4: %+v", m.at)
	}
}

// TestLineDiveRendersFocusBarOnTheLine: the focus bar rides the dived line's
// rendered row only, not every row of the block as block-level focus does.
func TestLineDiveRendersFocusBarOnTheLine(t *testing.T) {
	m := lineDiveTable(t)
	m.render() // populate m.spans before reading it
	span := m.spans[m.at.entry]
	lines := m.render()
	// A table renders 2 + len(rows) content rows (header, rule, body), then
	// the block's trailing blank — the blank is the gap after a block, never
	// part of it, so it carries no bar even at block level.
	contentEnd := span.start + 2 + len(m.entries[m.at.entry].b.table.rows)
	for i := span.start; i < contentEnd; i++ {
		if !strings.Contains(lines[i], focusStyle.Render("▌")) {
			t.Fatalf("block-level focus should carry the bar on row %d", i)
		}
	}

	m.dive()
	for m.at.line < 5 {
		m.moveFocus(1)
	}
	lines = m.render()
	// A table's rendered rows map 1:1 to its source lines, so the focused
	// source line's bar sits on span.start + (line - block start).
	focusedRow := span.start + (m.at.line - 3)
	for i := span.start; i < contentEnd; i++ {
		hasBar := strings.Contains(lines[i], focusStyle.Render("▌"))
		if i == focusedRow && !hasBar {
			t.Errorf("row %d (line %d) lost the focus bar: %q", i, m.at.line, lines[i])
		}
		if i != focusedRow && hasBar {
			t.Errorf("row %d carries the focus bar off the dived line: %q", i, lines[i])
		}
	}
}

// TestLineDiveNotOnWrappedProse: a paragraph re-wraps its source lines, so a
// source line has no row of its own and l must not offer a line dive.
func TestLineDiveNotOnWrappedProse(t *testing.T) {
	m := lineDiveTable(t)
	m.at = cursor{entry: 0, comment: commentNone} // the heading
	for m.entries[m.at.entry].b.kind != blockPara {
		m.moveFocus(1)
	}
	if m.entries[m.at.entry].b.kind != blockPara {
		t.Fatalf("expected a paragraph at %d", m.at.entry)
	}
	if _, _, ok := m.lineDiveRange(); ok {
		t.Fatal("paragraph is line-diveable; wrapped prose has no line identity")
	}
	m.dive()
	if m.at.line != 0 {
		t.Fatalf("paragraph dive entered a line: %+v", m.at)
	}
}

// TestLineDiveNotOnCodeBlock: code blocks keep l/h for horizontal scroll, so
// they are not line-diveable either.
func TestLineDiveNotOnCodeBlock(t *testing.T) {
	m := newModelAt("doc.md", parseDoc([]byte("```go\nfunc f() {\n    return\n}\n```\n")), nil)
	m.w, m.h = 100, 60
	if m.entries[m.at.entry].b.kind != blockCode {
		t.Fatalf("expected a code block at 0, got %d", m.entries[m.at.entry].b.kind)
	}
	if _, _, ok := m.lineDiveRange(); ok {
		t.Fatal("code block is line-diveable; l/h already scroll it")
	}
}
