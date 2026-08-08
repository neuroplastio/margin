package review

import (
	"fmt"
	"os/exec"
	"strings"
)

// maxQuoteLines caps how much of a block is quoted. Enough to identify it
// unambiguously, not so much that the export becomes a copy of the document.
const maxQuoteLines = 6

// exportReview renders the whole review as markdown for an agent to act on.
//
// The shape is chosen so an agent can work from it with no tooling. Each item
// has to answer one question first — *which* block is this? — because ids are
// not yet stamped into the source (ID-01), so the agent cannot search for
// `^0afa31`. Until then the locator is the file and line, the enclosing
// section, and a faithful quote. Quote fidelity is therefore the whole game:
// a list flattened onto one line and cut mid-word is not a locator.
//
// Flagged blocks appear even when nobody commented, because "this needs
// attention" is itself feedback. Drafts are excluded — unsubmitted means
// unsubmitted — but counted, so nothing goes silently missing.
//
// Resolved threads are excluded by default too (D11's export-safety
// rationale): the export is a list of what still needs doing, and a resolved
// thread is, by definition, addressed. includeResolved overrides this so an
// agent — or the reviewer — can see the full history instead of just what is
// outstanding. A flagged block is shown regardless: "needs attention" and
// "this thread is resolved" are independent signals, and the flag alone
// justifies inclusion the same way it does for a block with no comments at
// all.
func exportReview(path string, doc []block, threads map[string]*thread, marks map[string]reviewMark, includeResolved bool) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# Review of %s\n\n", path)

	done, flagged, total, drafts, resolvedHidden := 0, 0, 0, 0, 0
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
			if t.resolved && len(t.posted) > 0 && marks[blk.anchor] != markFlag {
				resolvedHidden++
			}
		}
	}

	fmt.Fprintf(&b, "%d of %d blocks reviewed", done, total)
	if flagged > 0 {
		fmt.Fprintf(&b, " · %d flagged for attention", flagged)
	}
	b.WriteString("\n")
	if drafts > 0 {
		fmt.Fprintf(&b, "\n_%d unsubmitted draft(s) not included._\n", drafts)
	}
	if resolvedHidden > 0 && !includeResolved {
		fmt.Fprintf(&b, "\n_%d resolved thread(s) not included._\n", resolvedHidden)
	}

	items, section := 0, ""
	for _, blk := range doc {
		if blk.kind == blockHeading {
			section = blk.text
		}

		t := threads[blk.anchor]
		mark := marks[blk.anchor]
		hasComments := t != nil && len(t.posted) > 0
		resolved := hasComments && t.resolved
		show := mark == markFlag || (hasComments && (!resolved || includeResolved))
		if !show {
			continue
		}
		items++

		b.WriteString("\n---\n\n")
		b.WriteString("## " + locator(path, blk))
		if mark == markFlag {
			b.WriteString(" — flagged, needs attention")
		}
		if resolved {
			b.WriteString(" — resolved")
		}
		b.WriteString("\n\n")

		// The enclosing section, unless this block *is* that heading.
		if section != "" && blk.kind != blockHeading {
			fmt.Fprintf(&b, "Section: %s\n\n", section)
		}

		b.WriteString(quoteBlock(blk))

		if hasComments {
			b.WriteString("\n")
			for _, c := range t.posted {
				fmt.Fprintf(&b, "**%s:** %s\n\n", c.author, strings.TrimSpace(c.body))
			}
		}
	}

	if items == 0 {
		if resolvedHidden > 0 {
			b.WriteString("\nNothing outstanding — every commented block is resolved.\n")
		} else {
			b.WriteString("\nNo comments and nothing flagged.\n")
		}
	}
	// Exactly one trailing newline: this lands in a paste buffer, and a tail of
	// blank lines reads as though something went missing.
	return strings.TrimRight(b.String(), "\n") + "\n"
}

// locator names the block in the way most useful to whoever has to find it.
func locator(path string, b block) string {
	if b.line > 0 {
		return fmt.Sprintf("%s:%d", path, b.line)
	}
	return b.anchor
}

// quoteBlock renders the block as a markdown blockquote.
//
// Structure is preserved for anything whose structure is its content — a list,
// a code fence, a table. Flattening those onto one line, as an earlier version
// did, produced quotes that were neither readable nor searchable.
func quoteBlock(b block) string {
	var lines []string
	truncated := false

	switch b.kind {
	case blockHeading:
		// Reconstruct the markdown so the level is visible and the line is
		// greppable in the source.
		lines = []string{strings.Repeat("#", max(b.level, 1)) + " " + b.text}

	case blockRaw, blockList:
		lines = b.lines
		if len(lines) > maxQuoteLines {
			lines, truncated = lines[:maxQuoteLines], true
		}

	default:
		text := collapse(b.text)
		if cut, ok := truncateWords(text, 280); ok {
			text, truncated = cut, true
		}
		lines = []string{text}
	}

	var out strings.Builder
	for _, l := range lines {
		if l == "" {
			out.WriteString(">\n")
			continue
		}
		out.WriteString("> " + l + "\n")
	}
	if truncated {
		out.WriteString("> …\n")
	}
	return out.String()
}

// truncateWords cuts at a word boundary rather than mid-word, and reports
// whether it cut anything.
func truncateWords(s string, limit int) (string, bool) {
	if len(s) <= limit {
		return s, false
	}
	cut := s[:limit]
	if i := strings.LastIndexByte(cut, ' '); i > limit/2 {
		cut = cut[:i]
	}
	return strings.TrimRight(cut, " ,.;:—-"), true
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
