// The command registry. Every verb margin can perform — moving focus,
// commenting, marking, exporting, quitting — is one entry here: a stable id,
// a description, an applicability guard, and the function that runs it.
//
// This exists so a key is never the only way to reach a verb. handleKey does
// nothing but resolve a keypress to a command id via keymap and call its Run;
// it holds no behaviour of its own. A future command palette (see the
// 2026-08-07 command-palette feedback) is meant to resolve the same way, so
// selecting "mark.reviewed" from the palette and pressing `r` run through the
// identical Run — one implementation, two ways in. That property is the
// point; nothing about the palette itself is decided or built here.
package review

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
)

// command is one registered verb.
type command struct {
	// ID is stable, dotted, and doubles as what a palette would search and a
	// config file would rebind — e.g. "mark.reviewed".
	ID string
	// Description is the human-readable explanation shown next to the id.
	Description string
	// Applicable reports whether the command makes sense given the model's
	// current focus. It is metadata, not a gate: handleKey does not consult
	// it, because every Run below already carries the guard that produces the
	// original status message when it does not apply, and re-checking here
	// would just suppress that message. It exists for a future palette to
	// decide what to list, per requirement 4 of the command-palette feedback.
	Applicable func(m *model) bool
	// Target names what the command is about to act on, given the current
	// focus — e.g. "comment by agent" — so the palette can show "Delete —
	// comment by agent" rather than a bare "Delete" (requirement 4: "the
	// difference between confidence and a guess"). nil for a command with no
	// focus-specific target (move.*, review.export, app.quit): those act the
	// same regardless of where focus sits, so a title is just the
	// description. When non-nil, an empty return also falls back to the
	// description alone — Applicable can be true (a thread exists) while
	// there is nothing more specific to name (no comment focused, no draft).
	Target func(m *model) string
	// Values returns a list of values this command accepts, if it is a staged command.
	// If nil, the command takes no value.
	Values func(m *model) []string
	// Run performs the command against m, returning whatever tea.Cmd the
	// action needs. If the command is staged, the selected value is passed as val.
	Run func(m *model, val string) tea.Cmd
}

// commands is the registry, in a fixed display order. Order does not reorder
// on use — the command-palette feedback leans toward stable ordering, but
// that is explicitly undecided; this order is simply the one the old switch
// statement declared its cases in.
var commands = []command{
	{
		ID:          "move.down",
		Description: "Move focus down",
		Applicable:  func(m *model) bool { return true },
		Run:         func(m *model, val string) tea.Cmd { m.moveFocus(1); return nil },
	},
	{
		ID:          "move.up",
		Description: "Move focus up",
		Applicable:  func(m *model) bool { return true },
		Run:         func(m *model, val string) tea.Cmd { m.moveFocus(-1); return nil },
	},
	{
		ID:          "move.pageDown",
		Description: "Page down",
		Applicable:  func(m *model) bool { return true },
		Run:         func(m *model, val string) tea.Cmd { m.pageScroll(false, true); return nil },
	},
	{
		ID:          "move.pageUp",
		Description: "Page up",
		Applicable:  func(m *model) bool { return true },
		Run:         func(m *model, val string) tea.Cmd { m.pageScroll(false, false); return nil },
	},
	{
		ID:          "move.halfPageDown",
		Description: "Half page down",
		Applicable:  func(m *model) bool { return true },
		Run:         func(m *model, val string) tea.Cmd { m.pageScroll(true, true); return nil },
	},
	{
		ID:          "move.halfPageUp",
		Description: "Half page up",
		Applicable:  func(m *model) bool { return true },
		Run:         func(m *model, val string) tea.Cmd { m.pageScroll(true, false); return nil },
	},
	{
		ID:          "move.scrollDown",
		Description: "Scroll viewport down incrementally",
		Applicable:  func(m *model) bool { return true },
		Run:         func(m *model, val string) tea.Cmd { m.scrollViewport(3); return nil },
	},
	{
		ID:          "move.scrollUp",
		Description: "Scroll viewport up incrementally",
		Applicable:  func(m *model) bool { return true },
		Run:         func(m *model, val string) tea.Cmd { m.scrollViewport(-3); return nil },
	},
	{
		ID:          "move.first",
		Description: "Jump to the first block",
		Applicable:  func(m *model) bool { return true },
		Run: func(m *model, val string) tea.Cmd {
			m.at = cursor{entry: 0, comment: commentNone}
			return nil
		},
	},
	{
		ID:          "move.last",
		Description: "Jump to the last block",
		Applicable:  func(m *model) bool { return true },
		Run: func(m *model, val string) tea.Cmd {
			m.at = cursor{entry: len(m.entries) - 1, comment: commentNone}
			return nil
		},
	},
	{
		ID:          "search.next",
		Description: "Jump to the next search match",
		Applicable:  func(m *model) bool { return m.searchQuery != "" },
		Run:         func(m *model, val string) tea.Cmd { m.searchStep(1); return nil },
	},
	{
		ID:          "search.prev",
		Description: "Jump to the previous search match",
		Applicable:  func(m *model) bool { return m.searchQuery != "" },
		Run:         func(m *model, val string) tea.Cmd { m.searchStep(-1); return nil },
	},
	{
		// move.dive is the 2026-08-09 todo-review feedback's "dedicated dive
		// navigation type": the only way focus reaches a comment from the
		// keyboard. j/k at block level walk blocks and thread rows only, so
		// a thread is one stop however long its conversation; l steps into
		// it, and h (move.surface) steps back out. It also steps into a
		// multi-line table or raw block line by line (the same feedback's
		// "dive into multi-line blocks" bullet) — a line dive walks the
		// block's source lines and c/gy anchor a comment to the one under
		// focus.
		ID:          "move.dive",
		Description: "Dive into thread or block lines / scroll code block right",
		Applicable: func(m *model) bool {
			if m.visual {
				return false
			}
			if m.at.comment == commentNone && m.focusedKind() == blockCode {
				return true
			}
			if m.at.comment != commentNone || m.at.line != 0 {
				return false
			}
			if _, _, ok := m.lineDiveRange(); ok {
				return true
			}
			t := m.threads[m.anchorAt()]
			return t != nil && len(m.visibleComments(t)) > 0
		},
		Run: func(m *model, val string) tea.Cmd {
			if m.at.comment == commentNone && m.focusedKind() == blockCode {
				m.scrollCode(4)
				return nil
			}
			m.dive()
			return nil
		},
	},
	{
		ID:          "move.surface",
		Description: "Surface back to block / thread / scroll code block left",
		Applicable: func(m *model) bool {
			if m.at.comment == commentNone && m.focusedKind() == blockCode && m.codeScroll[m.anchorAt()] > 0 {
				return true
			}
			return m.at.comment != commentNone || m.at.line != 0
		},
		Run: func(m *model, val string) tea.Cmd {
			if m.at.comment == commentNone && m.focusedKind() == blockCode {
				m.scrollCode(-4)
				return nil
			}
			m.surface()
			return nil
		},
	},
	{
		ID:          "comment.new",
		Description: "New comment on the focused block",
		Applicable:  func(m *model) bool { return m.anchorAt() != "" },
		Target:      commentTarget,
		Run: func(m *model, val string) tea.Cmd {
			// A new comment always starts a reply, never edits, wherever
			// focus is.
			a := m.anchorAt()
			if a == "" {
				m.status = "nothing to comment on here"
				return nil
			}
			ref := ""
			if m.visual {
				ref = m.selectionLineRef()
				m.visual = false
			} else if m.at.line != 0 {
				// Dived into a block's lines: the comment is about the line
				// under focus, so it carries the single-line reference (the
				// L12-18 payload that made a line dive worth building).
				ref = fmt.Sprintf("L%d: ", m.at.line)
			}
			if ref != "" {
				t := m.ensureThread(a)
				if t != nil {
					currDraft := t.draft(newCommentSlot)
					if !strings.HasPrefix(currDraft, ref) {
						t.setDraft(newCommentSlot, ref+currDraft)
					}
				}
			}
			return m.openComposer(a, newCommentSlot)
		},
	},
	{
		ID:          "comment.edit",
		Description: "Edit the focused comment or draft",
		Applicable: func(m *model) bool {
			a := m.anchorAt()
			return a != "" && m.threads[a] != nil
		},
		Target: editTarget,
		Run:    func(m *model, val string) tea.Cmd { return m.editFocused() },
	},
	{
		ID:          "mark.reviewed",
		Description: "Mark reviewed",
		Applicable:  func(m *model) bool { return len(m.sectionAnchors(m.at.entry)) > 0 },
		Target:      markTarget,
		Run:         func(m *model, val string) tea.Cmd { m.toggleMark(markOK); return nil },
	},
	{
		ID:          "mark.flagged",
		Description: "Flag for later",
		Applicable:  func(m *model) bool { return len(m.sectionAnchors(m.at.entry)) > 0 },
		Target:      markTarget,
		Run:         func(m *model, val string) tea.Cmd { m.toggleMark(markFlag); return nil },
	},
	{
		ID:          "mark.cycle",
		Description: "Cycle the review mark",
		Applicable:  func(m *model) bool { return len(m.sectionAnchors(m.at.entry)) > 0 },
		Target:      markTarget,
		Run:         func(m *model, val string) tea.Cmd { m.cycleMark(); return nil },
	},
	{
		ID:          "thread.resolve",
		Description: "Resolve or unresolve the focused thread",
		Applicable: func(m *model) bool {
			a := m.anchorAt()
			return a != "" && m.threads[a] != nil
		},
		Target: resolveTarget,
		Run:    func(m *model, val string) tea.Cmd { m.toggleResolved(); return nil },
	},
	{
		ID:          "thread.delete",
		Description: "Delete the focused comment or thread",
		Applicable: func(m *model) bool {
			a := m.anchorAt()
			return a != "" && m.threads[a] != nil
		},
		Target: deleteTarget,
		Run:    func(m *model, val string) tea.Cmd { return m.deleteFocused() },
	},
	{
		// thread.showDeleted toggles whether tombstoned comments render at
		// all — deleted by default per the 2026-08-09 feedback, revealed by
		// this command (`T` — `V` went to visual mode, 2026-08-09 — or
		// palette: `:` + "show"). A view toggle, so it
		// applies to the whole document regardless of focus, the same way
		// review.export does.
		ID:          "thread.showDeleted",
		Description: "Show or hide deleted comments",
		Applicable:  func(m *model) bool { return true },
		Run: func(m *model, val string) tea.Cmd {
			m.showDeleted = !m.showDeleted
			if m.showDeleted {
				m.status = "showing deleted comments"
				return nil
			}
			m.status = "hiding deleted comments"
			// A comment that just became hidden can no longer hold focus:
			// drop back to the thread entry so the cursor never lands where
			// nothing renders.
			if m.at.comment >= 0 {
				t := m.threads[m.anchorAt()]
				if t != nil && m.at.comment < len(t.posted) && t.posted[m.at.comment].deleted {
					m.at.comment = commentNone
				}
			}
			return nil
		},
	},
	{
		// selection.start enters (or leaves) visual mode: a blockwise
		// selection anchored where focus sits, extended by every movement,
		// cancelled by esc, a second V, or any command that is not movement
		// or selection itself. The 2026-08-09 visual-mode feedback asked
		// for Shift+V; a terminal delivers that as "V", which
		// thread.showDeleted gave up for it — the selection mode earns the
		// modifier key, the reveal toggle keeps the palette and moves to T.
		ID:          "selection.start",
		Description: "Select blocks (visual mode)",
		Applicable:  func(m *model) bool { return true },
		Run: func(m *model, val string) tea.Cmd {
			if m.visual {
				m.visual = false
				return nil
			}
			m.visual = true
			m.visualFrom = m.at.entry
			// Visual mode operates blockwise — lift any comment or line
			// dive onto its block.
			m.at.comment = commentNone
			m.at.line = 0
			m.status = ""
			return nil
		},
	},
	{
		ID:          "selection.yank",
		Description: "Yank selected blocks (or focused block)",
		Applicable:  func(m *model) bool { return true },
		Run:         func(m *model, val string) tea.Cmd { return m.yankSelection() },
	},
	{
		ID:          "selection.yankRef",
		Description: "Yank line reference",
		Applicable:  func(m *model) bool { return true },
		Run:         func(m *model, val string) tea.Cmd { return m.yankRef() },
	},
	{
		ID:          "review.export",
		Description: "Copy the whole review to the clipboard",
		Applicable:  func(m *model) bool { return true },
		Run:         func(m *model, val string) tea.Cmd { return m.exportToClipboard() },
	},
	{
		ID:          "app.quit",
		Description: "Quit",
		Applicable:  func(m *model) bool { return true },
		Run: func(m *model, val string) tea.Cmd {
			m.quitting = true
			return tea.Quit
		},
	},
	{
		ID:          "mark",
		Description: "Apply a review mark",
		Applicable:  func(m *model) bool { return len(m.sectionAnchors(m.at.entry)) > 0 },
		Target:      markTarget,
		Values: func(m *model) []string {
			return []string{"reviewed", "flagged"}
		},
		Run: func(m *model, val string) tea.Cmd {
			switch val {
			case "reviewed":
				m.toggleMark(markOK)
			case "flagged":
				m.toggleMark(markFlag)
			}
			return nil
		},
	},
	{
		ID:          "goto",
		Description: "Jump to a section",
		Applicable:  func(m *model) bool { return true },
		Values: func(m *model) []string {
			var sections []string
			for _, e := range m.entries {
				if e.b.kind == blockHeading {
					sections = append(sections, string(e.b.text))
				}
			}
			return sections
		},
		Run: func(m *model, val string) tea.Cmd {
			for i, e := range m.entries {
				if e.b.kind == blockHeading && string(e.b.text) == val {
					m.at = cursor{entry: i, comment: commentNone}
					break
				}
			}
			return nil
		},
	},
}

// commandByID looks a command up by its stable id.
func commandByID(id string) (command, bool) {
	for _, c := range commands {
		if c.ID == id {
			return c, true
		}
	}
	return command{}, false
}

// commentTarget names the block comment.new would attach to, by kind — the
// only thing distinguishing one focused block from another before any
// comment exists on it.
func commentTarget(m *model) string {
	if m.at.entry < 0 || m.at.entry >= len(m.entries) {
		return ""
	}
	switch m.entries[m.at.entry].b.kind {
	case blockHeading:
		return "heading"
	case blockPara:
		return "paragraph"
	case blockQuote:
		return "quote"
	case blockListItem:
		return "list item"
	case blockCode:
		return "code block"
	default:
		return "block"
	}
}

// editTarget names what comment.edit would open, mirroring editFocused's own
// precedence (review.go) without any of its side effects: the focused posted
// comment by author, else an unsubmitted reply draft, else a posted comment
// with an edit in progress. Applicable already guarantees a thread exists;
// none of the three matching is possible when focus is on the block itself
// with nothing yet drafted, in which case an empty target falls back to the
// bare description, same as editFocused's own status message covers that
// case at Run time.
func editTarget(m *model) string {
	t := m.threads[m.anchorAt()]
	if t == nil {
		return ""
	}
	if m.at.comment >= 0 && m.at.comment < len(t.posted) {
		return "comment by " + t.posted[m.at.comment].author
	}
	if t.draft(newCommentSlot) != "" {
		return "draft reply"
	}
	if i := t.pendingEdit(); i >= 0 {
		return "unsaved edit to comment by " + t.posted[i].author
	}
	return ""
}

// resolveTarget names what thread.resolve is about to do — "Resolve" or
// "Unresolve" — so the palette (once it exists) reads as an action, not a
// toggle the user has to already know the state to interpret. Applicable
// already guarantees a thread exists at focus.
func resolveTarget(m *model) string {
	t := m.threads[m.anchorAt()]
	if t == nil {
		return ""
	}
	if t.resolved {
		return "unresolve"
	}
	return "resolve"
}

// deleteTarget names what thread.delete is about to do.
func deleteTarget(m *model) string {
	t := m.threads[m.anchorAt()]
	if t == nil {
		return ""
	}
	if m.at.comment >= 0 && m.at.comment < len(t.posted) {
		return "comment by " + t.posted[m.at.comment].author
	}
	return "thread"
}

// markTarget names what a mark command would act on: a single block, or the
// whole section when focus sits on a heading. Shares sectionLabel (review.go)
// with toggleMark and cycleMark's own status messages, so the palette's title
// and the status line left behind by pressing the key never say two
// different things about the same action.
func markTarget(m *model) string {
	anchors := m.sectionAnchors(m.at.entry)
	if len(anchors) == 0 {
		return ""
	}
	return sectionLabel(anchors)
}

// keymap is the one place a raw key string binds to a command id. Every key
// margin recognises outside a live composer is here; handleKey has no
// per-key logic beyond this lookup.
var keymap = map[string]string{
	"j": "move.down", "down": "move.down",
	"k": "move.up", "up": "move.up",
	"J": "move.scrollDown",
	"K": "move.scrollUp",
	"l": "move.dive", "right": "move.dive",
	"h": "move.surface", "left": "move.surface",
	"ctrl+d": "move.halfPageDown",
	"ctrl+u": "move.halfPageUp",
	"ctrl+f": "move.pageDown", "pgdown": "move.pageDown",
	"ctrl+b": "move.pageUp", "pgup": "move.pageUp",
	"g":     "move.first", "home": "move.first",
	"G":     "move.last", "end": "move.last",
	"n":     "search.next",
	"N":     "search.prev",
	"c":     "comment.new",
	"e":     "comment.edit",
	"r":     "mark.reviewed",
	"f":     "mark.flagged",
	"space": "mark.cycle", " ": "mark.cycle",
	"R": "thread.resolve",
	"D": "thread.delete",
	"T": "thread.showDeleted",
	"V":  "selection.start",
	"y":  "selection.yank",
	"yr": "selection.yankRef",
	"gy": "selection.yankRef",
	"Y":  "review.export",
	"m": "mark",
	"s": "goto",
	"q": "app.quit", "ctrl+c": "app.quit",
}
