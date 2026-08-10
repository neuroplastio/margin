package review

import (
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
)

// modelFromSource parses src into a model and attaches the source, mirroring
// what Run does: a real document keeps its bytes around so the raw view can
// show them verbatim. Tests otherwise build models via newModel, which has no
// source (see toggleRaw's "no raw source" guard).
func modelFromSource(t *testing.T, src string) *model {
	t.Helper()
	m := newModel(parseDoc([]byte(src)), nil)
	m.src = []byte(src)
	m.w, m.h = 100, 60
	return m
}

// TestToggleRawKeyRoutesThroughTheRegistry: `\` resolves to view.raw and the
// command flips the raw flag — the same way every other verb is reachable.
func TestToggleRawKeyRoutesThroughTheRegistry(t *testing.T) {
	m := modelFromSource(t, sampleDoc)
	if m.raw {
		t.Fatal("model starts in raw mode")
	}
	m.handleKey(tea.KeyPressMsg(tea.Key{Code: '\\', Text: "\\"}))
	if !m.raw {
		t.Fatal("\\ did not switch to raw mode")
	}
	m.handleKey(tea.KeyPressMsg(tea.Key{Code: '\\', Text: "\\"}))
	if m.raw {
		t.Fatal("\\ did not switch back to rendered mode")
	}
}

// TestRawToggleRequiresSource: a seed model has no source of its own — nothing
// was ever loaded from disk or a pipe — so the toggle must refuse rather than
// render an empty raw view.
func TestRawToggleRequiresSource(t *testing.T) {
	m := newTestModel(t)
	m.handleKey(tea.KeyPressMsg(tea.Key{Code: '\\', Text: "\\"}))
	if m.raw {
		t.Fatal("seed model toggled into raw mode without a source")
	}
	if m.status != "no raw source for this document" {
		t.Fatalf("status = %q, want the no-source refusal", m.status)
	}
}

// TestRawRendersSourceVerbatim: the raw view shows every line of the source,
// in order, exactly as written — a two-line paragraph that the rendered view
// wraps stays one source line per rendered line. The gutter is still there
// (the render loop draws it), but the text after it is the raw source byte
// for byte.
func TestRawRendersSourceVerbatim(t *testing.T) {
	src := "# Title\n\nFirst paragraph.\n\nSecond paragraph.\n"
	m := modelFromSource(t, src)
	m.toggleRaw()

	lines := m.render()
	if len(lines) != 5 {
		t.Fatalf("raw view rendered %d lines, want 5 (the 5 source lines)", len(lines))
	}
	for i, srcLine := range strings.Split(strings.TrimRight(src, "\n"), "\n") {
		plain := []rune(ansiRe.ReplaceAllString(lines[i], ""))
		if len(plain) < gutterW {
			t.Fatalf("raw line %d too short to carry the gutter: %q", i, string(plain))
		}
		if got := string(plain[gutterW:]); got != srcLine {
			t.Errorf("raw line %d = %q, want %q", i, got, srcLine)
		}
	}
}

// TestRawCodeBlockKeepsSyntaxColors: the raw view stays verbatim, but the
// source lines *inside* a fenced code block borrow chroma's syntax
// highlighting — the colours are part of what the rendered view communicates
// (2026-08-10 raw-mode-colors feedback), so switching to raw should not
// throw them away. The fence lines themselves stay plain source, and a
// paragraph line is untouched.
func TestRawCodeBlockKeepsSyntaxColors(t *testing.T) {
	src := "# Title\n\nParagraph.\n\n" + "```go\n" +
		"func retry(n int) error {\n    return nil\n}\n" +
		"```\n"
	m := modelFromSource(t, src)
	m.toggleRaw()

	lines := m.render()
	var fence, code, para string
	for _, l := range lines {
		plain := string([]rune(ansiRe.ReplaceAllString(l, ""))[gutterW:])
		switch {
		case strings.HasPrefix(plain, "```"):
			fence = l
		case strings.Contains(plain, "func retry"):
			code = l
		case strings.HasPrefix(plain, "Paragraph."):
			para = l
		}
	}
	if code == "" || fence == "" || para == "" {
		t.Fatalf("render missing expected lines: code=%q fence=%q para=%q", code, fence, para)
	}
	// The code content line carries chroma's colours; the fences and a plain
	// paragraph line do not.
	if !strings.Contains(code, "\x1b[") {
		t.Error("code line lost its syntax colours in raw mode")
	}
	if strings.Contains(fence, "\x1b[") {
		t.Error("fence line picked up styling; fences are plain source")
	}
	if strings.Contains(para, "\x1b[") {
		t.Error("paragraph line picked up styling; raw mode stays verbatim")
	}
	// And the visible text is still the source, colours notwithstanding.
	plain := string([]rune(ansiRe.ReplaceAllString(code, ""))[gutterW:])
	if plain != "func retry(n int) error {" {
		t.Errorf("code line text = %q, want the verbatim source line", plain)
	}
}

// TestRawFocusAndScrollSurviveTheSwitch: toggling away and back keeps focus on
// the same block — by anchor, since the entry list shifts (threads drop out of
// the raw view) — and the scroll anchor is reset so the viewport re-anchors to
// it. Comment/line dives are cancelled: raw mode has no line dive and a
// comment cursor would point at nothing.
func TestRawFocusAndScrollSurviveTheSwitch(t *testing.T) {
	m := modelFromSource(t, sampleDoc)
	m.at = cursor{entry: 3, comment: commentNone}
	anchor := m.anchorAt()
	if anchor == "" {
		t.Fatal("focused entry has no anchor")
	}

	m.toggleRaw()
	if !m.raw {
		t.Fatal("toggle into raw failed")
	}
	if got := m.anchorAt(); got != anchor {
		t.Fatalf("raw mode focuses %q, want %q (focus survives by anchor)", got, anchor)
	}
	if m.at.comment != commentNone || m.at.line != 0 {
		t.Fatal("dive state was not cancelled entering raw mode")
	}
	if m.scrollAnchor.entry != -1 {
		t.Fatalf("scrollAnchor = %+v, want the reset sentinel so the viewport re-anchors", m.scrollAnchor)
	}

	m.toggleRaw()
	if m.raw {
		t.Fatal("toggle back to rendered failed")
	}
	if got := m.anchorAt(); got != anchor {
		t.Fatalf("back in rendered mode focuses %q, want %q", got, anchor)
	}
}

// TestRawViewExcludesThreads: threads live in .margin files, not the document,
// so the raw view shows the source alone — no thread rows, no comment text.
func TestRawViewExcludesThreads(t *testing.T) {
	m := modelFromSource(t, sampleDoc)
	m.threads["^h1"] = &thread{
		anchor: "^h1",
		posted: []comment{{author: "toly", body: "thread body must not appear", at: time.Now()}},
	}
	m.rebuild()

	m.toggleRaw()
	if !m.raw {
		t.Fatal("failed to enter raw mode")
	}
	for _, e := range m.entries {
		if e.thread != nil {
			t.Fatal("raw mode entries include a thread row; threads are not part of the source")
		}
	}
	rendered := strings.Join(m.render(), "\n")
	if strings.Contains(rendered, "thread body must not appear") {
		t.Fatal("raw view shows thread text")
	}
}

// TestRawMoveAndMarksStillWork: j/k walk blocks and space marks the focused
// block in raw mode exactly as in rendered mode — the toggle changes the
// rendering, not the review model underneath it.
func TestRawMoveAndMarksStillWork(t *testing.T) {
	m := modelFromSource(t, sampleDoc)
	m.toggleRaw()
	start := m.at

	m.handleKey(tea.KeyPressMsg(tea.Key{Code: 'j', Text: "j"}))
	if m.at.entry <= start.entry {
		t.Fatal("j did not move focus in raw mode")
	}

	m.at = cursor{entry: 1, comment: commentNone}
	m.handleKey(tea.KeyPressMsg(tea.Key{Code: ' ', Text: " "}))
	if m.marks[m.anchorAt()] != markOK {
		t.Fatal("space did not mark the focused block in raw mode")
	}
}

// TestRawDiveIsInert: l/h dive into threads or block lines — meaningless
// against a view that already shows every source line — so in raw mode they do
// nothing. H/L are the exception: they pan the raw view sideways (scrollRaw),
// see TestRawHScrollPansTheSourceView.
func TestRawDiveIsInert(t *testing.T) {
	m := modelFromSource(t, sampleDoc)
	m.toggleRaw()
	at := m.at
	m.handleKey(tea.KeyPressMsg(tea.Key{Code: 'l', Text: "l"}))
	if m.at != at {
		t.Fatalf("l in raw mode moved focus to %+v, want it inert", m.at)
	}
	m.handleKey(tea.KeyPressMsg(tea.Key{Code: 'h', Text: "h"}))
	if m.at != at {
		t.Fatalf("h in raw mode moved focus to %+v, want it inert", m.at)
	}
}

// TestRawHScrollPansTheSourceView: H/L in raw mode scroll the whole source view
// horizontally (the 2026-08-10 raw-mode-horizontal-scroll feedback) — L pans
// right, H back to the left edge, with the offset bounded by the widest source
// line minus the content width. Unlike the rendered view's per-block offsets,
// the raw offset is a single viewport-wide value (m.rawH), because raw mode is
// a flat source list. Short lines clip too, so a scrolled line reads its tail
// instead of wrapping.
func TestRawHScrollPansTheSourceView(t *testing.T) {
	// A 120-rune paragraph line against a 94-rune content width (100-col
	// terminal, contentWidth = w - 2*gutterW): max scroll is 26.
	src := "# Title\n\n" + strings.Repeat("x", 120) + "\n"
	m := modelFromSource(t, src)
	m.toggleRaw()

	// At offset 0 the long line is clipped at the content width, not wrapped.
	lines := m.render()
	foundLeft := false
	for _, l := range lines {
		plain := string([]rune(ansiRe.ReplaceAllString(l, "")))
		if len(plain) < gutterW {
			continue
		}
		body := plain[gutterW:]
		if body == string([]rune(strings.Repeat("x", 120))[:94]) {
			foundLeft = true
		}
	}
	if !foundLeft {
		t.Errorf("no rendered line showed the clipped left edge; lines:\n%q", lines)
	}

	// L scrolls right 4 columns and the render shows that slice of the line.
	m.handleKey(tea.KeyPressMsg(tea.Key{Code: 'L', Text: "L"}))
	if m.rawH != 4 {
		t.Fatalf("rawH after one L = %d, want 4", m.rawH)
	}
	lines = m.render()
	foundScrolled := false
	for _, l := range lines {
		plain := string([]rune(ansiRe.ReplaceAllString(l, "")))
		if len(plain) < gutterW {
			continue
		}
		body := plain[gutterW:]
		if body == string([]rune(strings.Repeat("x", 120))[4:98]) {
			foundScrolled = true
		}
	}
	if !foundScrolled {
		t.Errorf("no rendered line showed the scrolled slice; lines:\n%q", lines)
	}

	// H returns to the left edge.
	m.handleKey(tea.KeyPressMsg(tea.Key{Code: 'H', Text: "H"}))
	if m.rawH != 0 {
		t.Fatalf("rawH after H = %d, want 0", m.rawH)
	}
	if m.status != "source scrolled to left edge" {
		t.Errorf("status after H = %q, want the left-edge message", m.status)
	}

	// A short line scrolls too — its tail, never a wrap. "# Title" at offset 4
	// shows columns 4..6 ("tle").
	m.handleKey(tea.KeyPressMsg(tea.Key{Code: 'L', Text: "L"}))
	lines = m.render()
	if got := string([]rune(ansiRe.ReplaceAllString(lines[0], ""))[gutterW:]); got != "tle" {
		t.Errorf("short line at rawH=4 = %q, want its tail \"tle\"", got)
	}
}

// TestRawHScrollBoundsAndReset: the raw scroll offset clamps at the widest
// line minus the content width (reaching the line's tail and no further), a
// document that fits reports so instead of scrolling, and toggling out of raw
// and back resets the offset so the file is shown from its left edge again.
func TestRawHScrollBoundsAndReset(t *testing.T) {
	src := "# Title\n\nShort.\n"
	m := modelFromSource(t, src)
	m.toggleRaw()

	// Everything fits: L reports it, the offset stays 0.
	m.handleKey(tea.KeyPressMsg(tea.Key{Code: 'L', Text: "L"}))
	if m.status != "raw source fits within measure" {
		t.Errorf("status on a fitting document = %q, want the fits message", m.status)
	}
	if m.rawH != 0 {
		t.Errorf("rawH on a fitting document = %d, want 0", m.rawH)
	}

	// A long line scrolls to its tail and clamps there.
	m.src = []byte("# Title\n\n" + strings.Repeat("y", 100) + "\n")
	m.toggleRaw() // back to rendered
	m.toggleRaw() // into raw again: rawH reset to 0
	if m.rawH != 0 {
		t.Fatalf("rawH after a toggle round-trip = %d, want the reset to 0", m.rawH)
	}
	for i := 0; i < 10; i++ {
		m.handleKey(tea.KeyPressMsg(tea.Key{Code: 'L', Text: "L"}))
	}
	// w=100 → contentWidth 94, maxLen 100 → maxScroll 6; 10 L presses clamp at 6.
	if m.rawH != 6 {
		t.Errorf("rawH after 10 L presses = %d, want the clamp at 6", m.rawH)
	}
	if m.status != "source scrolled to col 6" {
		t.Errorf("status at the clamp = %q, want the col 6 message", m.status)
	}
}

// TestRawSpansMapToSourceLines: hit-testing a raw line lands on the block that
// owns that source line — the same spans the rendered view produces, so
// clicks, page keys and focus-following scroll all behave identically.
func TestRawSpansMapToSourceLines(t *testing.T) {
	m := modelFromSource(t, sampleDoc)
	m.toggleRaw()
	m.render() // spans are populated by the render pass, like everything else
	if len(m.spans) != len(m.entries) {
		t.Fatalf("raw mode has %d spans for %d entries", len(m.spans), len(m.entries))
	}
	for i, e := range m.entries {
		if e.b.line <= 0 {
			continue
		}
		s := m.spans[i]
		if s.start != e.b.line-1 || s.end != e.b.endLine-1 {
			t.Errorf("entry %d (%s) span = %+v, want source range [%d,%d]",
				i, e.b.anchor, s, e.b.line-1, e.b.endLine-1)
		}
	}
}
