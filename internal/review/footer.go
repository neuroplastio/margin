package review

import (
	"fmt"

	"neuroplast.io/hotty-surf/surflive"
	"neuroplast.io/hotty-surf/surftea"
)

//go:generate go run neuroplast.io/hotty-surf/cmd/surfgen

// The footer (D22, amended by the hotty-surf port): the mode margin is in,
// the keys it takes there as a help line, the review's progress, and the
// last status. It is a hotty-surf component, footer.surf, which a host
// draws as a surface and a terminal as cells.

// footerID is the footer's surface name.
const footerID = "footer"

// hint is a key the footer lists, named as SPEC §10.4 names keys, with what
// it does.
type hint struct{ key, label string }

// footerState is what the footer says in the mode margin is in: the mode,
// the keys, and the progress.
func (m *model) footerState() (mode, rest string, hints []hint, progress string) {
	switch {
	case m.comp != nil:
		mode = "new comment"
		if m.comp.target != newCommentSlot {
			mode = "editing comment"
		}
		return mode, "", []hint{{keySubmit, "submit"}, {keyDraft, "keep draft"}, {keyDiscard, "discard"}}, ""
	case m.inbox:
		return "threads", fmt.Sprintf("— %d thread(s)", len(m.inboxItems)),
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
		return "-- VISUAL --", fmt.Sprintf("%d block(s)", n),
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
		mode = "● file changed"
		hints = append(hints, hint{"ctrl+r", "reload"})
	}
	if m.raw {
		hints = append(hints, hint{"\\", "rendered"}, hint{"j", "move"}, hint{"c", "comment"},
			hint{"space", "mark"}, hint{"shift+y", "copy"}, hint{"q", "quit"})
	} else {
		hints = append(hints, hint{"j", "move"}, hint{"c", "comment"}, hint{"e", "edit"},
			hint{"space", "mark"}, hint{"shift+y", "copy review"}, hint{"q", "quit"})
	}
	return mode, "", hints, progress
}

// Footer is the footer's state (footer.surf lays it out): its fields are
// what it shows. It handles nothing: the keys it lists are margin's, read
// before any surface sees them.
type Footer struct {
	Mode     string
	Rest     string
	Hints    []FooterHint
	Progress string
	Status   string
}

// FooterHint is a key the footer lists and what it does.
type FooterHint struct {
	Key   string
	Label string
}

// footerHost is the footer as a surface, and its cells.
type footerHost struct {
	f    *Footer
	host *surftea.Host
}

// footer is the footer as the model has it now, its state brought up to
// date.
func (m *model) footer() *footerHost {
	if m.foot == nil {
		f := &Footer{}
		m.foot = &footerHost{f: f, host: newSurfHost("margin-"+footerID, surflive.New(surfInterpFooter(f)), nil)}
	}
	mode, rest, hints, progress := m.footerState()
	f := m.foot.f
	f.Mode, f.Rest, f.Progress, f.Status = mode, rest, progress, m.status
	f.Hints = f.Hints[:0]
	for _, h := range hints {
		f.Hints = append(f.Hints, FooterHint{h.key, h.label})
	}
	return m.foot
}

// renderFooter is the footer's line in cells, w wide.
func (m *model) renderFooter(w int) string {
	if l := m.footer().host.Lines(w, 1); len(l) > 0 {
		return l[0]
	}
	return ""
}
