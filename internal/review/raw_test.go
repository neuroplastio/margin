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

// TestRawDiveIsInert: l/h dive into threads or block lines, and scroll code
// blocks horizontally — all meaningless against a view that already shows
// every source line. In raw mode they do nothing.
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
