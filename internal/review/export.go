package review

import (
	"fmt"
	"os/exec"
	"strings"
)

// exportReview renders the whole review as markdown for an agent to act on.
//
// The shape is chosen so an agent can work from it without any tooling: every
// item names the block it refers to, quotes enough of that block to locate it,
// and then gives the exchange in order. Flagged blocks appear even when nobody
// commented, because "this needs attention" is itself feedback.
//
// Drafts are excluded — unsubmitted means unsubmitted — but they are counted in
// the summary so nothing goes silently missing.
func exportReview(path string, doc []block, threads map[string]*thread, marks map[string]reviewMark) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# Review of %s\n\n", path)

	done, flagged, total, drafts := 0, 0, 0, 0
	for _, blk := range doc {
		if blk.markable() && blk.anchor != "" {
			total++
			switch marks[blk.anchor] {
			case markOK:
				done++
			case markFlag:
				flagged++
			}
		}
		if t := threads[blk.anchor]; t != nil {
			drafts += len(t.drafts)
		}
	}

	fmt.Fprintf(&b, "%d of %d blocks reviewed", done, total)
	if flagged > 0 {
		fmt.Fprintf(&b, " · %d flagged for attention", flagged)
	}
	b.WriteString("\n")
	if drafts > 0 {
		fmt.Fprintf(&b, "\n_%d unsubmitted draft(s) were not included._\n", drafts)
	}

	items := 0
	for _, blk := range doc {
		t := threads[blk.anchor]
		mark := marks[blk.anchor]
		hasComments := t != nil && len(t.posted) > 0
		if !hasComments && mark != markFlag {
			continue
		}
		items++

		b.WriteString("\n---\n\n")
		fmt.Fprintf(&b, "## %s", blk.anchor)
		if mark == markFlag {
			b.WriteString(" — flagged, needs attention")
		}
		b.WriteString("\n\n")

		// A quote of the block, so the agent can find it even if the id has
		// been stripped. Kept short: it is a locator, not the document.
		fmt.Fprintf(&b, "> %s\n", truncate(collapse(blockText(blk)), 240))

		if hasComments {
			b.WriteString("\n")
			for _, c := range t.posted {
				fmt.Fprintf(&b, "**%s:** %s\n\n", c.author, strings.TrimSpace(c.body))
			}
		}
	}

	if items == 0 {
		b.WriteString("\nNo comments and nothing flagged.\n")
	}
	return b.String()
}

func blockText(b block) string {
	if b.kind == blockRaw {
		return strings.Join(b.lines, " ")
	}
	return b.text
}

// copyToClipboard puts s on the system clipboard by whatever means the machine
// offers, and reports which one worked.
//
// Two paths, deliberately both: an external helper is definitive when one is
// installed, and OSC 52 goes through the terminal itself, which is the only
// thing that works on a bare tty or over SSH. Emitting both is harmless — the
// content is identical — and between them something almost always lands.
//
// The OSC 52 half is not performed here; it is a tea.Cmd the caller issues,
// because only the Bubble Tea runtime may write to the terminal.
func copyToClipboard(s string) (via string, err error) {
	for _, candidate := range [][]string{
		{"wl-copy"},
		{"pbcopy"},
		{"xclip", "-selection", "clipboard"},
		{"xsel", "--clipboard", "--input"},
	} {
		bin, args := candidate[0], candidate[1:]
		if _, err := exec.LookPath(bin); err != nil {
			continue
		}
		cmd := exec.Command(bin, args...)
		cmd.Stdin = strings.NewReader(s)
		if err := cmd.Run(); err != nil {
			return "", fmt.Errorf("%s: %w", bin, err)
		}
		return bin, nil
	}
	return "", nil // nothing installed; the caller's OSC 52 command is the path
}
