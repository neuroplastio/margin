package review

import (
	"context"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	uv "github.com/charmbracelet/ultraviolet"
)

// TestHeadingsAreCommentable: "this whole section is wrong" is a real comment,
// and it belongs on the heading.
func TestHeadingsAreCommentable(t *testing.T) {
	m := seedModel()
	head := -1
	for i, e := range m.entries {
		if e.b.kind == blockHeading {
			head = i
			break
		}
	}
	if head < 0 {
		t.Fatal("no heading in the seeded document")
	}
	m.at = cursor{entry: head, comment: commentNone}

	if a := m.anchorAt(); a == "" {
		t.Fatal("heading has no anchor to hang a thread on")
	}
	if th := m.ensureThread(m.anchorAt()); th == nil {
		t.Fatalf("could not start a thread on a heading: %s", m.status)
	}
}

// But a heading still has no mark of its own — it rolls up its section, so the
// two can never disagree.
func TestHeadingHasNoMarkOfItsOwn(t *testing.T) {
	m := seedModel()
	m.at = cursor{entry: 0, comment: commentNone}
	if m.entries[0].b.kind != blockHeading {
		t.Fatal("fixture no longer starts with a heading")
	}
	m.cycleMark()

	if _, ok := m.marks[m.entries[0].b.anchor]; ok {
		t.Error("marking a heading gave the heading its own mark instead of its section")
	}
	for _, a := range []string{"^a1", "^b2", "^c3"} {
		if m.marks[a] != markOK {
			t.Errorf("%s = %v, want the section mark to have reached it", a, m.marks[a])
		}
	}
}

func TestSpaceCyclesMarks(t *testing.T) {
	m := seedModel()
	m.at = cursor{entry: blockEntryFor(t, m, "^a1"), comment: commentNone}

	for _, want := range []reviewMark{markOK, markFlag, markNone, markOK} {
		m.cycleMark()
		if got := m.marks["^a1"]; got != want {
			t.Fatalf("cycle reached %v, want %v", got, want)
		}
	}
}

// A partially marked section resolves to its roll-up first, so one press makes
// it consistent rather than jumping somewhere unexpected.
func TestCycleResolvesAPartialSectionFirst(t *testing.T) {
	m := seedModel()
	m.marks["^a1"] = markOK // one of three in the first section

	m.at = cursor{entry: 0, comment: commentNone}
	m.cycleMark()
	for _, a := range []string{"^a1", "^b2", "^c3"} {
		if m.marks[a] != markOK {
			t.Fatalf("%s = %v after resolving a partial section, want markOK", a, m.marks[a])
		}
	}
	m.cycleMark()
	if m.marks["^a1"] != markFlag {
		t.Errorf("second press = %v, want the cycle to continue to markFlag", m.marks["^a1"])
	}
}

func TestSpaceIsBoundToTheCycle(t *testing.T) {
	m := seedModel()
	m.at = cursor{entry: blockEntryFor(t, m, "^a1"), comment: commentNone}
	m.handleKey(tea.KeyPressMsg(tea.Key{Code: uv.KeySpace, Text: " "}))
	if m.marks["^a1"] != markOK {
		t.Errorf("space did not cycle the mark: %v", m.marks["^a1"])
	}
}

// TestIdenticalSiblingSectionsMarkIndependently is the maintainer's marking
// bug from the 2026-08-09 todo-review feedback: marking one header section
// (their "CMD-05", anchor ^ccd3fc) lit up every sibling header. The mechanism
// is anchor collision — sections whose bodies are byte-identical (board.md's
// repeated "*(none)*" paragraphs, say) derive the same content anchor, so the
// section mark lands on the shared anchor and every twin's roll-up reads it.
func TestIdenticalSiblingSectionsMarkIndependently(t *testing.T) {
	m := newModel(parseDoc([]byte("## One\n\n*(none)*\n\n## Two\n\n*(none)*\n")), nil)

	headings := map[string]int{}
	for i, e := range m.entries {
		if e.b.kind == blockHeading {
			headings[e.b.text] = i
		}
	}
	if len(headings) != 2 {
		t.Fatalf("fixture headings = %v, want One and Two", headings)
	}

	m.at = cursor{entry: headings["One"], comment: commentNone}
	m.toggleMark(markOK)

	if got, _ := rollUp(m.marksFor(m.sectionAnchors(headings["Two"]))); got != markNone {
		t.Errorf("sibling section's roll-up = %v, want unmarked — the mark leaked across identical bodies", got)
	}
	if done, _, _ := m.reviewProgress(); done != 1 {
		t.Errorf("reviewed count = %d, want 1 — the twin must not count as reviewed", done)
	}
}

// TestMarkRendersAsGutterRule: the 2026-08-09 additional-UX feedback ("per-line
// icons for marks look weird") — a marked block carries a vertical rule in the
// mark's colour on every one of its lines, reading as one continuous line down
// the block's extent, instead of an icon repeated per line. A partial section
// roll-up keeps the settled dimmed ·.
func TestMarkRendersAsGutterRule(t *testing.T) {
	m := newTestModel(t)
	m.marks[freshAnchor] = markOK
	m.marks[convoAnchor] = markFlag
	m.at = cursor{entry: blockEntryFor(t, m, soloAnchor), comment: commentNone}
	lines := m.render()

	ruleOK := okStyle.Render("│")
	ruleFlag := flagStyle.Render("│")

	// Every text line of a reviewed block carries the rule — the repetition is
	// what reads as one continuous line. ^a1 wraps to more than one line at
	// the test width, or this pins nothing.
	i := blockEntryFor(t, m, freshAnchor)
	n := 0
	for j := m.spans[i].start; j <= m.spans[i].end; j++ {
		if lines[j] == "" {
			continue // the trailing separator carries no gutter
		}
		n++
		if !strings.Contains(lines[j], ruleOK) {
			t.Errorf("reviewed block line %d = %q, want the green rule", j, lines[j])
		}
		if strings.Contains(lines[j], "✓") {
			t.Errorf("reviewed block line %d = %q still carries the retired icon", j, lines[j])
		}
	}
	if n < 2 {
		t.Fatalf("reviewed block rendered %d text lines, want a wrap so the rule's continuity is pinned", n)
	}

	// A flagged block gets the rule in the flag colour; the seeded fixture
	// text has no "!" of its own, so finding one is the retired icon.
	i = blockEntryFor(t, m, convoAnchor)
	for j := m.spans[i].start; j <= m.spans[i].end; j++ {
		if lines[j] == "" {
			continue
		}
		if !strings.Contains(lines[j], ruleFlag) {
			t.Errorf("flagged block line %d = %q, want the orange rule", j, lines[j])
		}
		if strings.Contains(lines[j], "!") {
			t.Errorf("flagged block line %d = %q still carries the retired icon", j, lines[j])
		}
	}

	// An unmarked block's gutter stays blank.
	i = blockEntryFor(t, m, soloAnchor)
	for j := m.spans[i].start; j <= m.spans[i].end; j++ {
		if strings.Contains(lines[j], ruleOK) || strings.Contains(lines[j], ruleFlag) {
			t.Errorf("unmarked block line %d = %q carries a mark rule", j, lines[j])
		}
	}
}

// TestPartialSectionKeepsDimmedDot: interaction.md settles that partial
// coverage renders as a dimmed ·, and the rule swap does not unsettle it —
// the heading of a section with one reviewed and one flagged block shows the
// dot, not a rule.
func TestPartialSectionKeepsDimmedDot(t *testing.T) {
	m := newTestModel(t)
	m.marks[freshAnchor] = markOK
	m.marks[convoAnchor] = markFlag
	m.at = cursor{entry: blockEntryFor(t, m, soloAnchor), comment: commentNone}
	lines := m.render()

	head := lines[m.spans[0].start]
	if m.entries[0].b.kind != blockHeading {
		t.Fatal("fixture no longer starts with a heading")
	}
	if !strings.Contains(head, dimStyle.Render("·")) {
		t.Errorf("partial section heading = %q, want the dimmed ·", head)
	}
	if strings.Contains(head, okStyle.Render("│")) || strings.Contains(head, flagStyle.Render("│")) {
		t.Errorf("partial section heading = %q carries a mark rule, want the dot", head)
	}

	// Fully reviewed, the same heading trades the dot for the rule.
	m.marks["^c3"] = markOK
	m.marks[convoAnchor] = markOK
	lines = m.render()
	head = lines[m.spans[0].start]
	if !strings.Contains(head, okStyle.Render("│")) {
		t.Errorf("fully reviewed section heading = %q, want the green rule", head)
	}
}

func TestExportIsBoundToShiftY(t *testing.T) {
	m := seedModel()
	cmd := m.handleKey(tea.KeyPressMsg(tea.Key{Code: 'y', Mod: uv.ModShift, Text: "Y"}))
	if cmd == nil {
		t.Fatal("Y produced no command; the clipboard write is a tea.Cmd")
	}
	if m.status == "" {
		t.Error("Y gave no feedback about where the review went")
	}
}

// TestCtrlEnterDecodesThroughRealReader pins the whole real path for ctrl+enter
// in the composer. Ghostty reports ctrl+enter under the kitty keyboard protocol
// as CSI 13;5u; feeding those exact bytes through the same uv terminal reader
// bubbletea v2 uses must come out as a KeyPressMsg the model routes to the
// composer, which must submit. A plain terminal sends CR for both enter and
// ctrl+enter, so there ctrl+s is the submit key.
func TestCtrlEnterDecodesThroughRealReader(t *testing.T) {
	m := newTestModel(t)
	open(t, m, freshAnchor, newCommentSlot, "")
	typeText(m, "ship it")

	tr := uv.NewTerminalReader(strings.NewReader("\x1b[13;5u"), "xterm-256color")
	evc := make(chan uv.Event, 1)
	done := make(chan struct{})
	go func() {
		defer close(done)
		_ = tr.StreamEvents(context.Background(), evc)
	}()
	select {
	case ev := <-evc:
		kev, ok := ev.(uv.KeyPressEvent)
		if !ok {
			t.Fatalf("decoded %T, want KeyPressEvent", ev)
		}
		if kev.Code != uv.KeyEnter || kev.Mod&uv.ModCtrl == 0 {
			t.Fatalf("decoded %#v, want ctrl+enter", kev)
		}
		m.handleKey(tea.KeyPressMsg(tea.Key(kev)))
	case <-done:
		t.Fatal("terminal reader produced no event for CSI 13;5u")
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for the reader to decode")
	}

	if m.comp != nil {
		t.Fatal("ctrl+enter left the composer open")
	}
	if tr := m.threads[freshAnchor]; tr == nil || len(tr.posted) != 1 || tr.posted[0].body != "ship it" {
		t.Fatalf("ctrl+enter did not post the comment: %+v", tr)
	}
}
