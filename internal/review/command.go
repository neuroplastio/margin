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

import tea "charm.land/bubbletea/v2"

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
	// Run performs the command against m, returning whatever tea.Cmd the
	// action needs — identical to what the pre-registry switch case did.
	Run func(m *model) tea.Cmd
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
		Run:         func(m *model) tea.Cmd { m.moveFocus(1); return nil },
	},
	{
		ID:          "move.up",
		Description: "Move focus up",
		Applicable:  func(m *model) bool { return true },
		Run:         func(m *model) tea.Cmd { m.moveFocus(-1); return nil },
	},
	{
		ID:          "move.first",
		Description: "Jump to the first block",
		Applicable:  func(m *model) bool { return true },
		Run: func(m *model) tea.Cmd {
			m.at = cursor{entry: 0, comment: commentNone}
			return nil
		},
	},
	{
		ID:          "move.last",
		Description: "Jump to the last block",
		Applicable:  func(m *model) bool { return true },
		Run: func(m *model) tea.Cmd {
			m.at = cursor{entry: len(m.entries) - 1, comment: commentNone}
			return nil
		},
	},
	{
		ID:          "comment.new",
		Description: "New comment on the focused block",
		Applicable:  func(m *model) bool { return m.anchorAt() != "" },
		Target:      commentTarget,
		Run: func(m *model) tea.Cmd {
			// A new comment always starts a reply, never edits, wherever
			// focus is.
			if a := m.anchorAt(); a != "" {
				return m.openComposer(a, newCommentSlot)
			}
			m.status = "nothing to comment on here"
			return nil
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
		Run:    func(m *model) tea.Cmd { return m.editFocused() },
	},
	{
		ID:          "mark.reviewed",
		Description: "Mark reviewed",
		Applicable:  func(m *model) bool { return len(m.sectionAnchors(m.at.entry)) > 0 },
		Target:      markTarget,
		Run:         func(m *model) tea.Cmd { m.toggleMark(markOK); return nil },
	},
	{
		ID:          "mark.flagged",
		Description: "Flag for later",
		Applicable:  func(m *model) bool { return len(m.sectionAnchors(m.at.entry)) > 0 },
		Target:      markTarget,
		Run:         func(m *model) tea.Cmd { m.toggleMark(markFlag); return nil },
	},
	{
		ID:          "mark.cycle",
		Description: "Cycle the review mark",
		Applicable:  func(m *model) bool { return len(m.sectionAnchors(m.at.entry)) > 0 },
		Target:      markTarget,
		Run:         func(m *model) tea.Cmd { m.cycleMark(); return nil },
	},
	{
		ID:          "review.export",
		Description: "Copy the whole review to the clipboard",
		Applicable:  func(m *model) bool { return true },
		Run:         func(m *model) tea.Cmd { return m.exportToClipboard() },
	},
	{
		ID:          "app.quit",
		Description: "Quit",
		Applicable:  func(m *model) bool { return true },
		Run: func(m *model) tea.Cmd {
			m.quitting = true
			return tea.Quit
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
	"g":     "move.first",
	"G":     "move.last",
	"c":     "comment.new",
	"e":     "comment.edit",
	"r":     "mark.reviewed",
	"f":     "mark.flagged",
	"space": "mark.cycle", " ": "mark.cycle",
	"Y": "review.export",
	"q": "app.quit", "ctrl+c": "app.quit",
}
