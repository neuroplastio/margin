package review

import (
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

// TestCtrlEnterSubmits: a plain terminal sends CR for both enter and
// ctrl+enter, so nvim cannot tell them apart and the host has to. This asserts
// our half — that the key resolves to the same submit path as every other
// gesture. Whether Ghostty actually reports the modifier is a human check.
func TestCtrlEnterSubmits(t *testing.T) {
	m := newTestModel(t)
	open(t, m, freshAnchor, newCommentSlot, "")
	typeText(m.comp, "ship it")
	typeKeys(m.comp, tea.Key{Code: uv.KeyEnter, Mod: uv.ModCtrl})

	if got := outcomeFromExit(waitExit(t, m.comp, 10*time.Second)); got != outcomeSubmit {
		t.Fatalf("ctrl+enter gave outcome %v, want submit", got)
	}
	if body := m.comp.body(); body != "ship it" {
		t.Fatalf("body = %q, want the text saved on submit", body)
	}
}

// A bare enter must still be a newline, not a submit.
func TestPlainEnterIsANewline(t *testing.T) {
	m := newTestModel(t)
	open(t, m, freshAnchor, newCommentSlot, "")
	typeText(m.comp, "first")
	typeKeys(m.comp, tea.Key{Code: uv.KeyEnter})
	typeText(m.comp, "second")

	if m.comp.cmd.ProcessState != nil {
		t.Fatal("plain enter exited the editor; it must insert a newline")
	}
	typeKeys(m.comp, keyCtrlS)
	waitExit(t, m.comp, 10*time.Second)
	if body := m.comp.body(); !strings.Contains(body, "first\nsecond") {
		t.Errorf("body = %q, want enter to have inserted a line break", body)
	}
}
