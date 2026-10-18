package review

import (
	"fmt"

	hottycat "github.com/neuroplastio/hotty-a2ui/catalog/hotty"
)

// footerID is the footer's pane (D22): the mode margin is in, the keys it
// takes there as a help line (HottyKeyHints, from a HottyShortcut each),
// the review's progress, and the last status.
const footerID = "footer"

// hint is a key the footer lists, named as SPEC §10.4 names keys, with what
// it does.
type hint struct{ key, label string }

// footerState is what the footer says in the mode margin is in: the mode,
// the keys, and the progress.
func (m *model) footerState() (mode string, hints []hint, progress string) {
	switch {
	case m.comp != nil:
		mode = "**new comment**"
		if m.comp.target != newCommentSlot {
			mode = "**editing comment**"
		}
		return mode, []hint{{keySubmit, "submit"}, {keyDraft, "keep draft"}, {keyDiscard, "discard"}}, ""
	case m.inbox:
		return fmt.Sprintf("**threads** — %d thread(s)", len(m.inboxItems)),
			[]hint{{"j", "move"}, {"enter", "open"}, {"escape", "close"}}, ""
	case m.visual:
		n := 0
		if lo, hi, ok := m.selectionRange(); ok {
			for i := lo; i <= hi; i++ {
				if m.entries[i].thread == nil {
					n++
				}
			}
		}
		return fmt.Sprintf("**-- VISUAL --**  %d block(s)", n),
			[]hint{{"j", "extend"}, {"y", "yank"}, {"shift+v", "cancel"}}, ""
	}
	done, flagged, total := m.reviewProgress()
	progress = fmt.Sprintf("%d/%d reviewed", done, total)
	// A tree review's footer rolls the whole tree up, not just the open
	// document — the open document's own numbers sit on its pane row, so the
	// footer is where the tree-wide answer lives.
	if m.tree != nil {
		done, flagged, total = m.treeProgress()
		progress = fmt.Sprintf("tree %d/%d reviewed", done, total)
	}
	if flagged > 0 {
		progress += fmt.Sprintf(" · %d flagged", flagged)
	}
	if m.docChanged {
		// The persistent half of the change notice: a status line alone is
		// overwritten by the next verb, so the footer keeps "the document
		// moved, ctrl+r reloads" in view until the reviewer acts on it.
		mode = "**●** file changed"
		hints = append(hints, hint{"ctrl+r", "reload"})
	}
	if m.raw {
		hints = append(hints, hint{"\\", "rendered"}, hint{"j", "move"}, hint{"c", "comment"},
			hint{"space", "mark"}, hint{"shift+y", "copy"}, hint{"q", "quit"})
	} else {
		hints = append(hints, hint{"j", "move"}, hint{"c", "comment"}, hint{"e", "edit"},
			hint{"space", "mark"}, hint{"shift+y", "copy review"}, hint{"q", "quit"})
	}
	return mode, hints, progress
}

// footerPane is the footer's pane as the model has it.
func (m *model) footerPane() *pane {
	mode, hints, progress := m.footerState()
	p := m.panes().pane(footerID, map[string]any{"mode": "", "progress": "", "status": ""})
	children := []any{"mode", "hints", "progress", "status"}
	comps := []map[string]any{
		nil,
		obj("id", "mode", "component", "Text", "text", bind("/mode")),
		obj("id", "hints", "component", "HottyKeyHints", "catalogId", hottycat.ID, "toggle", false, "weight", 1),
		obj("id", "progress", "component", "Text", "text", bind("/progress"), "variant", "caption"),
		obj("id", "status", "component", "Text", "text", bind("/status"), "variant", "caption"),
	}
	for i, h := range hints {
		// The footer only lists the keys: they are margin's, read before
		// any pane sees them, so the action is never sent.
		id := fmt.Sprintf("key%d", i)
		children = append(children, id)
		comps = append(comps, obj("id", id, "component", "HottyShortcut", "catalogId", hottycat.ID,
			"key", h.key, "label", h.label, "action", obj("event", obj("name", "hint"))))
	}
	comps[0] = obj("id", "root", "component", "Row", "children", children, "align", "center")
	p.define(comps...)
	p.put("/mode", mode)
	// A pane's Text is Markdown, and the kit shows a backslash escape as
	// it is: a status quoting a comment's Markdown shows it rendered.
	p.put("/progress", progress)
	p.put("/status", m.status)
	return p
}

// renderFooter is the footer's line, w wide.
func (m *model) renderFooter(w int) string {
	return m.footerPane().lines(w)[0]
}
