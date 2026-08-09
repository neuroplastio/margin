package review

import (
	"testing"

	tea "charm.land/bubbletea/v2"
	uv "github.com/charmbracelet/ultraviolet"
)

// The dive navigation model, from the 2026-08-09 todo-review feedback:
// "j/k navigation eats focus on threads. Need a dedicated 'dive' navigation
// type." Block-level j/k walk entries — blocks and thread rows — and never
// stop on a comment; l dives into the focused thread's comments, h surfaces,
// and j/k inside a dive flow back out at the ends.

// TestBlockLevelNeverStopsOnComments: however far j travels at block level,
// focus never lands on a comment — the flat stop list the feedback rejected
// cost three presses to get past a two-comment thread.
func TestBlockLevelNeverStopsOnComments(t *testing.T) {
	m := newTestModel(t)
	m.at = cursor{entry: 0, comment: commentNone}

	for i := 0; i < 200; i++ {
		if m.at.comment != commentNone {
			t.Fatalf("block-level j landed on a comment: %+v", m.at)
		}
		m.moveFocus(1)
	}
	if m.at.entry != len(m.entries)-1 {
		t.Fatalf("200 presses of j ended at entry %d, want the last entry %d", m.at.entry, len(m.entries)-1)
	}
}

// TestSingleCommentThreadEatsFocusOnce: the feedback's second bullet — a
// thread with one comment "shouldn't stop focus twice, it should only eat
// focus once". j from the block lands on the thread row (the one press the
// thread eats) and the next j is already on the following block.
func TestSingleCommentThreadEatsFocusOnce(t *testing.T) {
	m := newTestModel(t)
	m.at = cursor{entry: blockEntryFor(t, m, soloAnchor), comment: commentNone}
	threadRow := entryFor(t, m, soloAnchor)

	m.moveFocus(1)
	if m.at != (cursor{entry: threadRow, comment: commentNone}) {
		t.Fatalf("j from the block = %+v, want the thread row %d", m.at, threadRow)
	}
	m.moveFocus(1)
	if m.at.entry != threadRow+1 || m.at.comment != commentNone {
		t.Fatalf("second j = %+v, want the entry after the thread (%d)", m.at, threadRow+1)
	}
}

// TestDiveStepsIntoTheFirstComment: l on the thread row — or on the block,
// which names the same anchor — moves focus onto the first visible comment.
func TestDiveStepsIntoTheFirstComment(t *testing.T) {
	m := newTestModel(t)
	threadRow := entryFor(t, m, convoAnchor)

	for _, start := range []int{threadRow, blockEntryFor(t, m, convoAnchor)} {
		m.at = cursor{entry: start, comment: commentNone}
		m.dive()
		if m.at != (cursor{entry: threadRow, comment: 0}) {
			t.Fatalf("dive from entry %d = %+v, want the thread's first comment", start, m.at)
		}
	}
}

// TestDivedMovementVisitsEveryComment: inside a dive, j steps through the
// thread's comments in order — what j used to do uninvited at block level.
func TestDivedMovementVisitsEveryComment(t *testing.T) {
	m := newTestModel(t)
	m.at = cursor{entry: entryFor(t, m, convoAnchor), comment: commentNone}

	m.dive()
	if m.at.comment != 0 {
		t.Fatalf("dive landed on %+v, want comment 0", m.at)
	}
	m.moveFocus(1)
	if m.at.comment != 1 {
		t.Fatalf("j inside the dive = %+v, want comment 1", m.at)
	}
}

// TestDivePopsOutAtTheEnds: a dive never traps — j past the last comment
// lands on the next entry (linear reading does not stall), k past the first
// lands back on the thread row.
func TestDivePopsOutAtTheEnds(t *testing.T) {
	m := newTestModel(t)
	threadRow := entryFor(t, m, convoAnchor)

	m.at = cursor{entry: threadRow, comment: 1}
	m.moveFocus(1)
	if m.at != (cursor{entry: threadRow + 1, comment: commentNone}) {
		t.Fatalf("j past the last comment = %+v, want the next entry at block level", m.at)
	}

	m.at = cursor{entry: threadRow, comment: 0}
	m.moveFocus(-1)
	if m.at != (cursor{entry: threadRow, comment: commentNone}) {
		t.Fatalf("k past the first comment = %+v, want the thread row", m.at)
	}
}

// TestSurfaceStepsBackOut: h leaves the dive for the thread row, and does
// nothing at block level.
func TestSurfaceStepsBackOut(t *testing.T) {
	m := newTestModel(t)
	threadRow := entryFor(t, m, convoAnchor)

	m.at = cursor{entry: threadRow, comment: 1}
	m.surface()
	if m.at != (cursor{entry: threadRow, comment: commentNone}) {
		t.Fatalf("surface = %+v, want the thread row", m.at)
	}
	m.surface()
	if m.at != (cursor{entry: threadRow, comment: commentNone}) {
		t.Fatalf("surface at block level moved focus to %+v", m.at)
	}
}

// TestDiveWithoutAThreadIsANoop: l on a plain paragraph goes nowhere and
// says why, rather than wedging or jumping to some other thread.
func TestDiveWithoutAThreadIsANoop(t *testing.T) {
	m := newTestModel(t)
	m.at = cursor{entry: blockEntryFor(t, m, freshAnchor), comment: commentNone}

	m.dive()
	if m.at != (cursor{entry: blockEntryFor(t, m, freshAnchor), comment: commentNone}) {
		t.Fatalf("dive with no thread moved focus to %+v", m.at)
	}
	if m.status == "" {
		t.Fatal("dive with no thread left no status hint")
	}
}

// TestDiveWithNoVisibleCommentsIsANoop: a thread whose comments are all
// hidden tombstones has nothing to dive into — same rule the focus stop
// list already applies.
func TestDiveWithNoVisibleCommentsIsANoop(t *testing.T) {
	m := newTestModel(t)
	m.threads[soloAnchor].posted[0].deleted = true
	threadRow := entryFor(t, m, soloAnchor)
	m.at = cursor{entry: threadRow, comment: commentNone}

	m.dive()
	if m.at.comment != commentNone {
		t.Fatalf("dive reached a hidden comment: %+v", m.at)
	}
	if m.status == "" {
		t.Fatal("dive with no visible comments left no status hint")
	}
}

// TestDiveKeysRouteThroughTheRegistry: l/h, the arrow keys, and enter/esc
// resolve to the dive commands, and pressing l then h really dives and
// surfaces.
func TestDiveKeysRouteThroughTheRegistry(t *testing.T) {
	for key, want := range map[string]string{
		"l": "move.dive", "right": "move.dive", "enter": "move.dive",
		"h": "move.surface", "left": "move.surface", "esc": "move.surface",
	} {
		if got := keymap[key]; got != want {
			t.Errorf("keymap[%q] = %q, want %q", key, got, want)
		}
	}

	m := newTestModel(t)
	threadRow := entryFor(t, m, convoAnchor)
	m.at = cursor{entry: threadRow, comment: commentNone}

	m.handleKey(tea.KeyPressMsg(tea.Key{Code: 'l', Text: "l"}))
	if m.at != (cursor{entry: threadRow, comment: 0}) {
		t.Fatalf("l = %+v, want the first comment", m.at)
	}
	m.handleKey(tea.KeyPressMsg(tea.Key{Code: 'h', Text: "h"}))
	if m.at != (cursor{entry: threadRow, comment: commentNone}) {
		t.Fatalf("h = %+v, want the thread row", m.at)
	}
}

// TestEnterEscDiveAndSurface: the 2026-08-09 feedback's "enter = dive,
// esc = undive" bindings, end to end through handleKey. enter steps into the
// thread's comments like l does; esc steps back out to the thread row like h;
// esc outside a dive is a no-op (no focus change, no status).
func TestEnterEscDiveAndSurface(t *testing.T) {
	m := newTestModel(t)
	threadRow := entryFor(t, m, convoAnchor)
	m.at = cursor{entry: threadRow, comment: commentNone}

	m.handleKey(tea.KeyPressMsg(tea.Key{Code: uv.KeyEnter}))
	if m.at != (cursor{entry: threadRow, comment: 0}) {
		t.Fatalf("enter = %+v, want the first comment", m.at)
	}
	m.handleKey(tea.KeyPressMsg(tea.Key{Code: uv.KeyEscape}))
	if m.at != (cursor{entry: threadRow, comment: commentNone}) {
		t.Fatalf("esc = %+v, want the thread row", m.at)
	}

	before := m.at
	m.handleKey(tea.KeyPressMsg(tea.Key{Code: uv.KeyEscape}))
	if m.at != before {
		t.Fatalf("esc outside a dive moved focus: %+v -> %+v", before, m.at)
	}

	// esc in visual mode still cancels the selection, not surfaces — vim's
	// rule, and the precondition the enter/esc bindings must not break.
	m.visual = true
	m.handleKey(tea.KeyPressMsg(tea.Key{Code: uv.KeyEscape}))
	if m.visual {
		t.Fatal("esc in visual mode did not cancel the selection")
	}
}

// TestDiveDoesNothingInVisualMode: the selection is blockwise, so the dive
// key must not pull focus onto a comment mid-selection — entering visual
// mode already lifts comment focus, and l stays inert until it ends.
func TestDiveDoesNothingInVisualMode(t *testing.T) {
	m := newTestModel(t)
	m.at = cursor{entry: entryFor(t, m, convoAnchor), comment: commentNone}
	m.visual = true
	m.visualFrom = 0

	m.dive()
	if m.at.comment != commentNone {
		t.Fatalf("dive in visual mode pulled focus onto a comment: %+v", m.at)
	}
	if !m.visual {
		t.Fatal("an inert dive ended the selection")
	}
}
