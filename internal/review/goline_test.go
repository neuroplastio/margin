package review

import (
	"fmt"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
)

// The 2026-08-09 navigation-feature-requests feedback, feature 2: `<num>gg`
// and `<num>G` jump to a markdown source line — vim's two spellings of one
// motion — landing focus on the block that contains it. Digits buffer in the
// status line and only the two line jumps consume them; any other key clears
// the buffer and behaves exactly as it would with no count typed.

// golineDoc is a small document with known source line numbers: a heading, two
// paragraphs (one multi-line), a fenced code block, a table, and a two-item
// list.
const golineDoc = `# First

Paragraph one here.
It runs on two lines.

Paragraph two.

` + "```go\ncode here\n```\n" + `
| a | b |
|---|---|
| 1 | 2 |

- item one
- item two
`

func golineModel(t *testing.T) *model {
	t.Helper()
	m := newModel(parseDoc([]byte(golineDoc)), nil)
	m.w, m.h = 100, 60
	m.render()
	return m
}

// lineOf returns the 1-based source line of the first line containing needle.
func lineOf(t *testing.T, needle string) int {
	t.Helper()
	for i, l := range strings.Split(golineDoc, "\n") {
		if strings.Contains(l, needle) {
			return i + 1
		}
	}
	t.Fatalf("%q not found in the goto test document", needle)
	return 0
}

func gotoKey(m *model, k string) {
	m.handleKey(tea.KeyPressMsg(tea.Key{Code: rune(k[0]), Text: k}))
}

func gotoType(m *model, s string) {
	for _, r := range s {
		m.handleKey(tea.KeyPressMsg(tea.Key{Code: r, Text: string(r)}))
	}
}

// TestGotoLineDigitsBufferAndEcho: typing digits accumulates a count, echoes
// it in the status line, and runs no command — focus stays where it was.
func TestGotoLineDigitsBufferAndEcho(t *testing.T) {
	m := golineModel(t)
	before := m.at
	gotoType(m, "42")
	if m.count != "42" {
		t.Errorf("count = %q, want 42", m.count)
	}
	if m.status != "line 42" {
		t.Errorf("status = %q, want the echoing count", m.status)
	}
	if m.at != before {
		t.Errorf("typing digits moved focus to %+v, want it untouched at %+v", m.at, before)
	}
}

// TestGotoLineGGJumpsToContainingBlock: `42gg` lands focus on the entry whose
// block spans the source line.
func TestGotoLineGGJumpsToContainingBlock(t *testing.T) {
	m := golineModel(t)
	n := lineOf(t, "Paragraph two")
	gotoType(m, fmt.Sprintf("%d", n))
	gotoKey(m, "g")
	gotoKey(m, "g")
	if !strings.Contains(m.entries[m.at.entry].b.text, "Paragraph two") {
		t.Errorf("42gg for line %d landed on %q, want the paragraph", n, m.entries[m.at.entry].b.text)
	}
	if m.at.comment != commentNone {
		t.Errorf("jump left focus inside a thread: %+v", m.at)
	}
	if m.count != "" {
		t.Errorf("count = %q after the jump, want it consumed", m.count)
	}
}

// TestGotoLineGJumpsSameAsGG: `42G` is the same motion as `42gg`.
func TestGotoLineGJumpsSameAsGG(t *testing.T) {
	m := golineModel(t)
	n := lineOf(t, "code here")
	gotoType(m, fmt.Sprintf("%d", n))
	gotoKey(m, "G")
	if m.entries[m.at.entry].b.kind != blockCode {
		t.Errorf("<num>G for line %d landed on kind %v, want blockCode", n, m.entries[m.at.entry].b.kind)
	}
	if m.count != "" {
		t.Errorf("count = %q after the jump, want it consumed", m.count)
	}
}

// TestGotoLineClampsPastEnd: a line beyond the document clamps to the last
// entry and says so rather than landing silently.
func TestGotoLineClampsPastEnd(t *testing.T) {
	m := golineModel(t)
	gotoType(m, "9999")
	gotoKey(m, "g")
	gotoKey(m, "g")
	if m.at.entry != len(m.entries)-1 {
		t.Errorf("9999gg landed on entry %d, want the last (%d)", m.at.entry, len(m.entries)-1)
	}
	if m.status == "" {
		t.Error("a clamped jump left no status message")
	}
}

// TestGotoLineBeforeFirstGoesFirst: a count below 1 is the first block, the
// same clamp move.first applies.
func TestGotoLineBeforeFirstGoesFirst(t *testing.T) {
	m := golineModel(t)
	m.at = cursor{entry: len(m.entries) - 1, comment: commentNone}
	gotoType(m, "0")
	gotoKey(m, "g")
	gotoKey(m, "g")
	if m.at.entry != 0 {
		t.Errorf("0gg landed on entry %d, want the first (0)", m.at.entry)
	}
}

// TestGotoLineCountClearedByOtherKey: a count is a modifier — `42j` is a plain
// j, not a repeated move, and the buffer does not leak into the next key.
func TestGotoLineCountClearedByOtherKey(t *testing.T) {
	m := golineModel(t)
	before := m.at
	gotoType(m, "42")
	gotoKey(m, "j")
	if m.count != "" {
		t.Errorf("count = %q after j, want cleared", m.count)
	}
	if m.at.entry != before.entry+1 {
		t.Errorf("42j landed on entry %d, want %d (a plain j)", m.at.entry, before.entry+1)
	}
}

// TestGotoLineThreadRowNeverTargeted: a thread row spliced beneath its block
// carries no line numbers, so a jump into the block lands on the block, not
// the row.
func TestGotoLineThreadRowNeverTargeted(t *testing.T) {
	m := golineModel(t)
	para := -1
	for i, e := range m.entries {
		if strings.Contains(e.b.text, "Paragraph one") {
			para = i
			break
		}
	}
	if para < 0 {
		t.Fatal("paragraph not found")
	}
	a := m.entries[para].b.anchor
	m.threads[a] = &thread{anchor: a, quote: m.entries[para].b.text, posted: []comment{{author: "toly", body: "hi"}}}
	m.rebuild()
	m.render()
	n := lineOf(t, "Paragraph one")
	gotoType(m, fmt.Sprintf("%d", n))
	gotoKey(m, "g")
	gotoKey(m, "g")
	if m.at.entry != para {
		t.Errorf("jump to line %d landed on entry %d, want the block (%d) not its thread row", n, m.at.entry, para)
	}
}

// TestGotoLineCentresViewport: the jump centres the block in the viewport and
// re-anchors the scroll so clampScroll does not yank the offset back.
func TestGotoLineCentresViewport(t *testing.T) {
	m := golineModel(t)
	m.h = 20
	m.render()
	n := lineOf(t, "| a | b |")
	gotoType(m, fmt.Sprintf("%d", n))
	gotoKey(m, "g")
	gotoKey(m, "g")
	want := max(0, m.spans[m.at.entry].start-(m.h-footerRows)/2)
	if m.scroll != want {
		t.Errorf("scroll = %d, want %d (centred on the block)", m.scroll, want)
	}
	if m.scrollAnchor != m.at {
		t.Errorf("scrollAnchor = %+v, want focus %+v", m.scrollAnchor, m.at)
	}
}

// TestGotoLineAfterPendingG: g then digits then g — the first g waits as a
// prefix once a count is coming, instead of running the eager first-block move
// into the jump.
func TestGotoLineAfterPendingG(t *testing.T) {
	m := golineModel(t)
	n := lineOf(t, "| 1 | 2 |")
	gotoKey(m, "g")
	gotoType(m, fmt.Sprintf("%d", n))
	gotoKey(m, "g")
	if m.entries[m.at.entry].b.kind != blockTable {
		t.Errorf("g<num>g landed on kind %v, want blockTable", m.entries[m.at.entry].b.kind)
	}
}

// TestPlainGGAndGStillFirstAndLast: without a count, gg is still the first
// block and G the last.
func TestPlainGGAndGStillFirstAndLast(t *testing.T) {
	m := golineModel(t)
	m.at = cursor{entry: len(m.entries) - 1, comment: commentNone}
	gotoKey(m, "g")
	gotoKey(m, "g")
	if m.at.entry != 0 {
		t.Errorf("plain gg landed on entry %d, want the first", m.at.entry)
	}
	gotoKey(m, "G")
	if m.at.entry != len(m.entries)-1 {
		t.Errorf("plain G landed on entry %d, want the last", m.at.entry)
	}
}

// TestGotoLineCountCapped: the digit buffer stops growing at maxCountDigits so
// a key-mash cannot build an unbounded string.
func TestGotoLineCountCapped(t *testing.T) {
	m := golineModel(t)
	gotoType(m, "1234567")
	if m.count != "123456" {
		t.Errorf("count = %q, want the first %d digits", m.count, maxCountDigits)
	}
}

// TestGotoLineDigitsIgnoredInVisualMode: visual mode has no count — digits are
// inert there and must not leave a buffer behind for the next key.
func TestGotoLineDigitsIgnoredInVisualMode(t *testing.T) {
	m := golineModel(t)
	gotoKey(m, "V")
	gotoType(m, "42")
	if m.count != "" {
		t.Errorf("count = %q in visual mode, want empty", m.count)
	}
	gotoKey(m, "esc")
	if m.visual {
		t.Error("esc did not cancel visual mode")
	}
}

// TestGotoLineCountClearedByPaletteAndSearch: opening either prompt abandons a
// half-typed count.
func TestGotoLineCountClearedByPaletteAndSearch(t *testing.T) {
	m := golineModel(t)
	gotoType(m, "42")
	gotoKey(m, ":")
	if m.count != "" || !m.paletteOpen {
		t.Errorf("count = %q, paletteOpen = %v after ':', want cleared and open", m.count, m.paletteOpen)
	}
	m.paletteOpen = false
	gotoType(m, "42")
	gotoKey(m, "/")
	if m.count != "" || !m.searchOpen {
		t.Errorf("count = %q, searchOpen = %v after '/', want cleared and open", m.count, m.searchOpen)
	}
}
