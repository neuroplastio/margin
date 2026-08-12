package review

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
)

// Line-level review marks, from the 2026-08-11 dive-line-mark-comment
// feedback: diving into a block's lines (a table or a raw block, the line
// dive of 2026-08-09.28) makes the line the unit of review. space/r/f act on
// the focused line — stored in lineMarks, keyed anchor:line — and the block
// reads as the roll-up of its lines, exactly the way a heading rolls up its
// section. Comments on a line already worked (c prepends L<n>:, journal
// 2026-08-09.28); this is the mark half of the feedback.

// lineDiveDoc has its table on source lines 3-6: header, delimiter, two body
// rows (see line_dive_test.go).

func tableAnchor(t *testing.T, m *model) string {
	t.Helper()
	for _, e := range m.entries {
		if e.b.kind == blockTable {
			return e.b.anchor
		}
	}
	t.Fatal("no table in the document")
	return ""
}

// TestLineDiveSpaceMarksTheLine: space while dived marks the focused source
// line only — the block itself gets no mark of its own, and a sibling line
// stays unmarked.
func TestLineDiveSpaceMarksTheLine(t *testing.T) {
	m := lineDiveTable(t)
	a := tableAnchor(t, m)
	m.dive()
	for m.at.line < 5 {
		m.moveFocus(1)
	}
	if m.at.line != 5 {
		t.Fatalf("dive parked on line %d, want 5", m.at.line)
	}

	m.cycleMark()

	if m.lineMarks[lineMarkKey(a, 5)] != markOK {
		t.Errorf("line 5 = %v, want markOK", m.lineMarks[lineMarkKey(a, 5)])
	}
	if m.lineMarks[lineMarkKey(a, 6)] != markNone {
		t.Errorf("line 6 = %v, want unmarked", m.lineMarks[lineMarkKey(a, 6)])
	}
	if m.marks[a] != markNone {
		t.Errorf("the block itself = %v, want no block mark", m.marks[a])
	}
	if !strings.Contains(m.status, "line 5") {
		t.Errorf("status = %q, want it to name the line", m.status)
	}
}

// TestLineDiveCyclesLineMarks: the line's mark cycles unmarked → reviewed →
// flagged → unmarked, one key per step, like a block's.
func TestLineDiveCyclesLineMarks(t *testing.T) {
	m := lineDiveTable(t)
	a := tableAnchor(t, m)
	m.dive()
	for m.at.line < 5 {
		m.moveFocus(1)
	}

	for _, want := range []reviewMark{markOK, markFlag, markNone} {
		m.cycleMark()
		if got := m.lineMarks[lineMarkKey(a, 5)]; got != want {
			t.Fatalf("cycle reached %v, want %v", got, want)
		}
	}
}

// TestLineDiveMarksLinesIndependently: two lines of the same block can carry
// different marks, so a table the reviewer is working through row by row can
// show exactly which rows are done.
func TestLineDiveMarksLinesIndependently(t *testing.T) {
	m := lineDiveTable(t)
	a := tableAnchor(t, m)
	m.dive()
	for m.at.line < 5 {
		m.moveFocus(1)
	}
	m.toggleMark(markOK)
	for m.at.line < 6 {
		m.moveFocus(1)
	}
	m.toggleMark(markFlag)

	if m.lineMarks[lineMarkKey(a, 5)] != markOK {
		t.Errorf("line 5 = %v, want markOK", m.lineMarks[lineMarkKey(a, 5)])
	}
	if m.lineMarks[lineMarkKey(a, 6)] != markFlag {
		t.Errorf("line 6 = %v, want markFlag", m.lineMarks[lineMarkKey(a, 6)])
	}
}

// TestLineDiveRepressClearsTheLine: re-pressing the same mark on a line
// clears just that line — r on a reviewed line, f on a flagged one.
func TestLineDiveRepressClearsTheLine(t *testing.T) {
	m := lineDiveTable(t)
	a := tableAnchor(t, m)
	m.dive()
	for m.at.line < 5 {
		m.moveFocus(1)
	}

	m.toggleMark(markOK)
	if m.lineMarks[lineMarkKey(a, 5)] != markOK {
		t.Fatal("first r did not mark the line")
	}
	m.toggleMark(markOK)
	if _, ok := m.lineMarks[lineMarkKey(a, 5)]; ok {
		t.Error("re-pressing r did not clear the line's mark")
	}

	m.toggleMark(markFlag)
	m.toggleMark(markFlag)
	if _, ok := m.lineMarks[lineMarkKey(a, 5)]; ok {
		t.Error("re-pressing f did not clear the line's mark")
	}
}

// TestLineDiveKeysReachTheLineMark: space, r and f route through the key
// registry to the line-level mark, the same way they mark a block at block
// level.
func TestLineDiveKeysReachTheLineMark(t *testing.T) {
	m := lineDiveTable(t)
	a := tableAnchor(t, m)
	m.dive()
	for m.at.line < 5 {
		m.moveFocus(1)
	}

	for _, key := range []rune{'r', 'f', 'f'} {
		m.handleKey(tea.KeyPressMsg(tea.Key{Code: key, Text: string(key)}))
	}
	// r: reviewed; f: flagged; f: clears (re-press of the same mark).
	if _, ok := m.lineMarks[lineMarkKey(a, 5)]; ok {
		t.Errorf("line 5 = %v after r f f, want unmarked", m.lineMarks[lineMarkKey(a, 5)])
	}
}

// TestBlockLevelMarkSetsEveryLine: surfacing and marking a diveable block
// applies the mark to every one of its lines — the block's review state IS
// its lines' — so a whole table reviewed in one press reads reviewed and
// counts toward progress.
func TestBlockLevelMarkSetsEveryLine(t *testing.T) {
	m := lineDiveTable(t)
	a := tableAnchor(t, m)
	m.toggleMark(markOK)

	for ln := 3; ln <= 6; ln++ {
		if m.lineMarks[lineMarkKey(a, ln)] != markOK {
			t.Errorf("line %d = %v, want markOK from the block mark", ln, m.lineMarks[lineMarkKey(a, ln)])
		}
	}
	if mk, _ := m.blockMark(m.entries[m.at.entry].b); mk != markOK {
		t.Errorf("block roll-up = %v, want markOK", mk)
	}
	// The document also has "Final paragraph.", so the denominator is 2.
	if done, _, total := m.reviewProgress(); done != 1 || total != 2 {
		t.Errorf("progress = %d/%d, want 1/2", done, total)
	}
}

// TestLineMarksRollUpPartially: a block with some but not all lines marked
// reads as a partial roll-up — not done, not flagged — the same shape a
// partially marked section takes.
func TestLineMarksRollUpPartially(t *testing.T) {
	m := lineDiveTable(t)
	a := tableAnchor(t, m)
	m.lineMarks[lineMarkKey(a, 5)] = markOK

	mk, partial := m.blockMark(m.entries[m.at.entry].b)
	if mk != markOK || !partial {
		t.Errorf("one-of-two roll-up = (%v, %v), want (markOK, partial)", mk, partial)
	}
	if done, flagged, _ := m.reviewProgress(); done != 0 || flagged != 0 {
		t.Errorf("progress = %d done, %d flagged, want 0/0", done, flagged)
	}
}

// TestFlaggedLineRollsUpFlagged: any flagged line flags the whole block —
// a section is not clean while one paragraph needs attention, and neither is
// a table while one row does.
func TestFlaggedLineRollsUpFlagged(t *testing.T) {
	m := lineDiveTable(t)
	m.lineMarks[lineMarkKey(tableAnchor(t, m), 5)] = markFlag

	mk, _ := m.blockMark(m.entries[m.at.entry].b)
	if mk != markFlag {
		t.Errorf("block roll-up = %v, want markFlag", mk)
	}
	_, flagged, _ := m.reviewProgress()
	if flagged != 1 {
		t.Errorf("flagged count = %d, want 1", flagged)
	}
}

// TestLineMarkRendersOnTheLine: the mark rule rides only the marked line's
// rendered row, not the whole block — so a partly-worked table shows exactly
// the rows that are done.
func TestLineMarkRendersOnTheLine(t *testing.T) {
	m := lineDiveTable(t)
	a := tableAnchor(t, m)
	m.lineMarks[lineMarkKey(a, 5)] = markFlag

	m.render()
	span := m.spans[m.at.entry]
	lines := m.render()
	// The table's rows map 1:1 to its source lines, so line 5 is
	// span.start + (5 - 3) = row 2 (the first body row).
	rule := flagStyle.Render("│")
	for i := span.start; i <= span.end; i++ {
		has := strings.Contains(lines[i], rule)
		if i == span.start+2 && !has {
			t.Errorf("line 5's row %d lost the rule: %q", i, lines[i])
		}
		if i != span.start+2 && has {
			t.Errorf("row %d carries a rule off the marked line: %q", i, lines[i])
		}
	}
}

// TestLineMarkSurvivesSurfaceAndClears: surfacing and cycling the block mark
// acts on the whole block — the lines are the block's state, so a surface +
// space on a partially line-marked table resolves it, and another press
// clears everything (no invisible residue under the block mark).
func TestLineMarkSurvivesSurfaceAndClears(t *testing.T) {
	m := lineDiveTable(t)
	a := tableAnchor(t, m)
	m.lineMarks[lineMarkKey(a, 5)] = markOK
	m.at = cursor{entry: m.at.entry, comment: commentNone} // surface

	m.cycleMark() // partial → resolve to the roll-up (markOK)
	m.cycleMark() // reviewed → flagged
	if m.lineMarks[lineMarkKey(a, 5)] != markFlag || m.lineMarks[lineMarkKey(a, 6)] != markFlag {
		t.Errorf("lines after space space = %v / %v, want both flagged", m.lineMarks[lineMarkKey(a, 5)], m.lineMarks[lineMarkKey(a, 6)])
	}
	m.cycleMark() // flagged → clear
	for ln := 3; ln <= 6; ln++ {
		if _, ok := m.lineMarks[lineMarkKey(a, ln)]; ok {
			t.Errorf("line %d still marked after clearing the block", ln)
		}
	}
}

// TestLineMarkInRawView: raw mode's gutter rides the same line marks — the
// source line with a mark carries the rule, and an unmarked line inside a
// line-marked block stays clean.
func TestLineMarkInRawView(t *testing.T) {
	m := modelFromSource(t, lineDiveDoc)
	a := tableAnchor(t, m)
	m.lineMarks[lineMarkKey(a, 5)] = markFlag

	m.toggleRaw()
	lines := m.render()
	rule := flagStyle.Render("│")
	// Source line 5 is rendered at index 4 (0-based).
	if !strings.Contains(lines[4], rule) {
		t.Errorf("source line 5 in raw = %q, want the rule", lines[4])
	}
	if strings.Contains(lines[5], rule) {
		t.Errorf("unmarked source line 6 in raw = %q, carries the rule", lines[5])
	}
}

// TestLineMarkCountsInSectionRollUp: a heading's section roll-up reads a
// line-marked table through its lines, so a section whose table is part-done
// shows the dimmed ·, not clean.
func TestLineMarkCountsInSectionRollUp(t *testing.T) {
	m := lineDiveTable(t)
	a := tableAnchor(t, m)
	m.lineMarks[lineMarkKey(a, 5)] = markOK

	m.at = cursor{entry: 0, comment: commentNone} // the heading
	mk, partial := rollUp(m.marksFor(m.sectionAnchors(0)))
	if mk != markOK || !partial {
		t.Errorf("section roll-up = (%v, %v), want (markOK, partial)", mk, partial)
	}
}

// TestTreeSwitchCarriesLineMarks: switching documents in a tree review carries
// a diveable block's line marks per-document, the same way it carries block
// marks — the lines land in the cache with the rolled-up block mark.
func TestTreeSwitchCarriesLineMarks(t *testing.T) {
	root, _, b := treeProgressFixture(t)
	// Add a table to the README so there is a diveable block to mark.
	spec := "# Spec\n\n| A | B |\n| - | - |\n| x | y |\n"
	if err := os.WriteFile(filepath.Join(root, filepath.FromSlash(b)), []byte(spec), 0o644); err != nil {
		t.Fatal(err)
	}
	m, err := newTreeModel(root)
	if err != nil {
		t.Fatalf("newTreeModel: %v", err)
	}
	m.w, m.h = 100, 30

	// Open the spec, mark the whole table line by line.
	m.openTreeFile(2)
	a := tableAnchor(t, m)
	for ln := 3; ln <= 6; ln++ {
		m.lineMarks[lineMarkKey(a, ln)] = markOK
	}
	if m.store.docPath != b {
		t.Fatalf("store.docPath = %q, want %q", m.store.docPath, b)
	}

	// Switch away and back — the line mark must survive the round trip.
	m.openTreeFile(0)
	if m.store.docPath == b {
		t.Fatal("switched to the README but store still points at the spec")
	}
	m.openTreeFile(2)
	if m.store.docPath != b {
		t.Fatalf("store.docPath = %q, want %q", m.store.docPath, b)
	}
	if m.lineMarks[lineMarkKey(a, 5)] != markOK {
		t.Error("the line mark did not survive the switch")
	}
	if m.markCache[b][a] != markOK {
		t.Error("the rolled-up block mark did not survive into the session cache")
	}
}
