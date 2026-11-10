package review

import (
	"slices"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"neuroplast.io/hotty-go"
	"neuroplast.io/hotty-go/hottytest"
)

// A paste is the comment, whole, whichever way the composer is dismissed
// (D7: never lose words). In cells the paste is the composer's own; the
// outcomes are the three keys.
func TestComposerPasteSurvivesEveryOutcome(t *testing.T) {
	const pasted = "first line\nsecond line\n\nfourth, after a gap"
	for _, tc := range []struct {
		name string
		key  tea.Key
		want func(t *testing.T, m *model)
	}{
		{"submit", keyCtrlS, func(t *testing.T, m *model) {
			posted := m.threads[freshAnchor].posted
			if len(posted) != 1 || posted[0].body != pasted {
				t.Errorf("posted %+v, want the pasted text", posted)
			}
		}},
		{"submit with ctrl+enter", keyCtrlEnter, func(t *testing.T, m *model) {
			if posted := m.threads[freshAnchor].posted; len(posted) != 1 || posted[0].body != pasted {
				t.Errorf("posted %+v, want the pasted text", posted)
			}
		}},
		{"keep", keyEsc, func(t *testing.T, m *model) {
			if got := m.threads[freshAnchor].draft(newCommentSlot); got != pasted {
				t.Errorf("draft %q, want the pasted text", got)
			}
		}},
		{"discard", keyCtrlG, func(t *testing.T, m *model) {
			// A thread made for this open and left empty is dropped.
			if th := m.threads[freshAnchor]; th != nil && th.draft(newCommentSlot) != "" {
				t.Errorf("draft %q after Discard, want none", th.draft(newCommentSlot))
			}
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m := newTestModel(t)
			open(t, m, freshAnchor, newCommentSlot, "")
			m.View()
			m.update(tea.PasteMsg{Content: pasted})
			if got := composed(m); got != pasted {
				t.Fatalf("the field holds %q, want the paste whole", got)
			}
			press(m, tc.key)
			if m.comp != nil {
				t.Fatal("the composer is still open")
			}
			tc.want(t, m)
		})
	}
}

// A draft resumed starts with the caret after its text, so typing goes on
// from where the reviewer stopped.
func TestComposerResumedDraftTypesAtTheEnd(t *testing.T) {
	m := newTestModel(t)
	open(t, m, draftAnchor, newCommentSlot, "contradicts")
	m.View()
	before := composed(m)
	typeText(m, "X")
	if got := composed(m); got != before+"X" {
		t.Fatalf("the field holds %q, want %q: typing goes after the resumed text", got, before+"X")
	}
}

// The field grows with its text, three rows to eight, and the composer's
// height follows it (the box around it is the pane's rows and its border).
func TestComposerGrowsWithItsText(t *testing.T) {
	m := newTestModel(t)
	open(t, m, freshAnchor, newCommentSlot, "")
	m.View()
	if want := composerLeastField + composerBarRows; m.paneH != want {
		t.Fatalf("an empty composer is %d rows, want %d", m.paneH, want)
	}
	typeText(m, strings.Repeat("line\n", 5)+"last")
	m.View()
	if want := 6 + composerBarRows; m.paneH != want {
		t.Errorf("six lines make a composer of %d rows, want %d", m.paneH, want)
	}
	typeText(m, strings.Repeat("\nmore", 20))
	m.View()
	if want := 8 + composerBarRows; m.paneH != want {
		t.Errorf("a long comment makes a composer of %d rows, want the most, %d", m.paneH, want)
	}
	if !strings.Contains(composed(m), "line\nline") {
		t.Errorf("growing lost words: %q", composed(m))
	}
}

// Tab walks from the field to the buttons and round again, in cells, and
// Enter on a button presses it.
func TestComposerTabWalksTheButtons(t *testing.T) {
	m := newTestModel(t)
	open(t, m, freshAnchor, newCommentSlot, "")
	m.View()
	typeText(m, "walked")
	press(m, keyTab)      // Submit
	press(m, keyTab)      // Keep draft
	press(m, keyTab)      // Discard
	press(m, keyTab)      // round to the field
	typeText(m, " again") // typing goes to the field
	if got := composed(m); got != "walked again" {
		t.Fatalf("the field holds %q after a round of Tab", got)
	}
	press(m, keyTab)
	press(m, keyTab) // Keep draft
	press(m, keyEnter)
	if m.comp != nil {
		t.Fatal("Enter on Keep draft left the composer open")
	}
	if got := m.threads[freshAnchor].draft(newCommentSlot); got != "walked again" {
		t.Errorf("draft %q, want the words kept", got)
	}
}

// On a host the composer is placed with fit, and when the host measures it
// taller, the surface is placed again with the rows it said.
func TestHottyComposerFitsItsRows(t *testing.T) {
	name := "margin-" + composerID
	hotRun(t, hotDocSrc, func(h *hottytest.Host, m *model) {
		surfaceWith(t, h, "A paragraph with bold")
		h.Type("j")
		h.Type("c")
		var s *hottytest.Surface
		waitFor(t, "the composer's surface", func() bool { s = h.Surface(name); return s != nil && s.Placed() })
		waitFor(t, "its field with the keyboard", func() bool { return s.Focused() == composerFieldID })
		if !s.Fit() {
			t.Fatal("the composer is not placed with fit")
		}
		// The first placement is the estimate, and the frames after the
		// document's show may place it again: measure from a settled one.
		rows := s.Placement().Rows
		for stable := 0; stable < 5; {
			time.Sleep(60 * time.Millisecond)
			if r := s.Placement().Rows; r == rows {
				stable++
			} else {
				rows, stable = r, 0
			}
		}
		if err := h.Emit(name, hotty.EventFit, "", map[string]int{"r": rows + 3}); err != nil {
			t.Fatal(err)
		}
		waitFor(t, "the composer placed at the rows the host measured", func() bool { return s.Placement().Rows == rows+3 })
		h.Key("escape") // so that margin can quit
		waitFor(t, "the composer gone", func() bool { s := h.Surface(name); return s == nil || !s.Placed() })
	})
}

// Words typed on a host, then a key that is margin's: nothing lost.
func TestHottyComposerKeepsWordsOnEveryKey(t *testing.T) {
	name := "margin-" + composerID
	for _, tc := range []struct {
		key  string
		want string // the draft kept, or "" when nothing is
		post bool
	}{
		{"escape", "typed on a host", false},
		{"ctrl+g", "", false},
		{"ctrl+s", "typed on a host", true},
	} {
		t.Run(tc.key, func(t *testing.T) {
			m, _ := hotRun(t, hotDocSrc, func(h *hottytest.Host, m *model) {
				surfaceWith(t, h, "A paragraph with bold")
				h.Type("j")
				h.Type("c")
				var s *hottytest.Surface
				waitFor(t, "the composer's surface", func() bool { s = h.Surface(name); return s != nil && s.Placed() })
				waitFor(t, "its field with the keyboard", func() bool { return s.Focused() == composerFieldID })
				if err := h.Fill(name, composerFieldID, "typed on a host"); err != nil {
					t.Fatal(err)
				}
				h.Key(tc.key)
				waitFor(t, "the composer gone", func() bool { s := h.Surface(name); return s == nil || !s.Placed() })
			})
			var drafts, posted []string
			for _, th := range m.threads {
				if d := th.draft(newCommentSlot); d != "" {
					drafts = append(drafts, d)
				}
				for _, c := range th.posted {
					posted = append(posted, c.body)
				}
			}
			switch {
			case tc.post && !slices.Equal(posted, []string{tc.want}):
				t.Errorf("posted %q, want %q", posted, tc.want)
			case !tc.post && tc.want != "" && !slices.Equal(drafts, []string{tc.want}):
				t.Errorf("drafts %q, want %q kept", drafts, tc.want)
			case tc.want == "" && (len(drafts) > 0 || len(posted) > 0):
				t.Errorf("drafts %q, posted %q, want nothing after Discard", drafts, posted)
			}
		})
	}
}

// The palette is placed with fit too (B11): its rows are hotty-surf's
// estimate at first, from the host's cell (TestPaletteRowsAreEstimated), and
// the host's measure after.
func TestHottyPaletteFitsItsRows(t *testing.T) {
	name := "margin-" + paletteID
	hotRun(t, hotDocSrc, func(h *hottytest.Host, m *model) {
		surfaceWith(t, h, "A paragraph with bold")
		h.Type(":")
		var s *hottytest.Surface
		waitFor(t, "the palette's surface", func() bool { s = h.Surface(name); return s != nil && s.Placed() })
		waitFor(t, "its field with the keyboard", func() bool { return s.Focused() == paletteFieldID })
		if !s.Fit() {
			t.Fatal("the palette is not placed with fit")
		}
		rows := s.Placement().Rows
		for stable := 0; stable < 5; {
			time.Sleep(60 * time.Millisecond)
			if r := s.Placement().Rows; r == rows {
				stable++
			} else {
				rows, stable = r, 0
			}
		}
		if err := h.Emit(name, hotty.EventFit, "", map[string]int{"r": rows + 2}); err != nil {
			t.Fatal(err)
		}
		waitFor(t, "the palette placed at the rows the host measured", func() bool { return s.Placement().Rows == rows+2 })
		h.Key("escape")
		waitFor(t, "the palette gone", func() bool { s := h.Surface(name); return s == nil || !s.Placed() })
	})
}

// Before a host has measured the palette its rows are hotty-surf's estimate
// from the host's cell: with a 20 px cell the rule, seven commands and the
// field are the rows the hand-counted formula (a rule's 9 px, an item's cell
// and 8 px of padding, the field's cell) gave, 12. Nothing here is a host's.
func TestPaletteRowsAreEstimated(t *testing.T) {
	m := newTestModel(t)
	m.paletteOpen = true
	caps := hotty.Caps{}
	caps.CellCSS.W, caps.CellCSS.H = 10, 20
	if got := surfRows(m.paletteHost(), 100, caps); got != 12 {
		t.Errorf("the palette is estimated at %d rows with a 20 px cell, want 12", got)
	}
	if got := m.paletteHeight(); got != 9 {
		t.Errorf("the palette is %d rows in cells, want 9", got)
	}
}
