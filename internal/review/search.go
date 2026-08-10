// Search: `/` opens a prompt at the bottom of the screen, typing highlights
// every match in the document live, enter commits the query and jumps to the
// next match after the current focus, and n/N walk the match list afterwards.
// Drained from the 2026-08-09 navigation-feature-requests feedback, feature 1.
//
// The match list is derived from the rendered lines, not the source: the query
// searches what is on screen — the wrapped prose, the aligned table, the
// highlighted code — and the matches are where the rendered line actually
// shows the text, so a hit inside a wrapped paragraph still highlights the
// run the reader is looking at. The rendered line's plain text is the same
// rune sequence as its styled form with the ANSI stripped, so a match found in
// the plain text maps back onto the styled line one rune per column.
//
// What is searched is deliberate: document blocks and the thread boxes under
// them (a comment's words are part of the review surface). The frontmatter is
// metadata about the document — reachable by keyboard since the 2026-08-09
// frontmatter feedback (g/j land on it), but still not a search target, the
// same way it is not commentable or markable. Each match carries the entry
// whose span owns its line, so Enter lands focus on a real block.
package review

import (
	"fmt"
	"strings"
	"unicode/utf8"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

// matchRange is a query occurrence within one rendered line: lo and hi are the
// rune-column range (hi exclusive) of the match in the line's text, the same
// columns the highlight painted.
type matchRange struct{ lo, hi int }

// searchMatch is one place the query appears: the entry whose span owns the
// rendered line, and the rune range of the match on that line. Recomputed
// from the rendered lines on every render, so the list always agrees with
// what is on screen — a window resize re-wraps the prose and the matches move
// with it.
type searchMatch struct {
	line  int
	lo    int
	hi    int
	entry int
}

// searchBG paints a matched run. A background-only SGR: it sets the
// background and leaves the foreground — the paragraph's text colour, chroma's
// tokens, a bold run — untouched, so the match reads as a highlight laid over
// the existing styling rather than a box that owns the characters. 94 is a
// warm brown, deliberately distinct from the selection's slate (selBG) and
// from the mark colours, so a search highlight can never be mistaken for
// either. The end code resets only the background, restoring the default, so
// the rest of the line keeps whatever foreground styling it had.
const (
	searchBG  = "\x1b[48;5;94m"
	searchEnd = "\x1b[49m"
)

// highlightSearch paints every occurrence of query in a styled rendered line
// with searchBG, returning the re-styled line and the rune ranges painted.
// It walks the line tracking the rune column (ANSI sequences take no column),
// opening the background at each range's first rune and closing it after the
// last with a background-only reset, so nothing after the match is disturbed.
// A full reset (\x1b[m / \x1b[0m) inside a match — the boundary of an inline
// bold or code run, say — would otherwise clear the background mid-hit, so it
// is immediately reasserted, the same trick selLine uses to keep the selection
// alive across inner resets.
func highlightSearch(line, query string) (string, []matchRange) {
	if line == "" || query == "" {
		return line, nil
	}
	plain := ansiRe.ReplaceAllString(line, "")
	pl := []rune(strings.ToLower(plain))
	ql := []rune(strings.ToLower(query))
	var ranges []matchRange
	for i := 0; i+len(ql) <= len(pl); i++ {
		if runesEqual(pl[i:i+len(ql)], ql) {
			ranges = append(ranges, matchRange{lo: i, hi: i + len(ql)})
			i += len(ql) - 1
		}
	}
	if len(ranges) == 0 {
		return line, nil
	}

	var b strings.Builder
	col := 0
	mi := 0
	i := 0
	for i < len(line) {
		if line[i] == '\x1b' {
			seq := nextANSI(line[i:])
			b.WriteString(seq)
			if isFullReset(seq) {
				// An inner reset would clear the background mid-match;
				// reassert it so the highlight survives to the run's end.
				if mi < len(ranges) && col > ranges[mi].lo && col <= ranges[mi].hi {
					b.WriteString(searchBG)
				}
			}
			i += len(seq)
			continue
		}
		_, size := utf8.DecodeRuneInString(line[i:])
		if mi < len(ranges) && col == ranges[mi].lo {
			b.WriteString(searchBG)
		}
		b.WriteString(line[i : i+size])
		col++
		if mi < len(ranges) && col == ranges[mi].hi {
			b.WriteString(searchEnd)
			mi++
		}
		i += size
	}
	return b.String(), ranges
}

// nextANSI returns the full escape sequence starting at the head of s, or ""
// when s does not begin with one. Handles the C0 sequences margin's renderers
// emit — CSI SGR (\x1b[...m), plus a lone ESC.
func nextANSI(s string) string {
	if len(s) == 0 || s[0] != '\x1b' {
		return ""
	}
	if len(s) >= 2 && s[1] == '[' {
		j := 2
		for j < len(s) && (s[j] < 'a' || s[j] > 'z') && (s[j] < 'A' || s[j] > 'Z') {
			j++
		}
		if j < len(s) {
			j++
		}
		return s[:j]
	}
	return s[:1]
}

// isFullReset reports whether an ANSI sequence resets every attribute —
// \x1b[m and \x1b[0m, the two resets lipgloss and chroma emit at the end of a
// styled run.
func isFullReset(seq string) bool {
	return seq == "\x1b[m" || seq == "\x1b[0m"
}

func runesEqual(a, b []rune) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// activeQuery is the query matches are computed from right now: the prompt's
// draft while the prompt is open, so highlighting tracks the keystrokes
// before they are committed, and the committed query afterwards.
func (m *model) activeQuery() string {
	if m.searchOpen {
		return m.searchDraft
	}
	return m.searchQuery
}

// applySearch recomputes m.searchMatches over the rendered lines and paints
// the matched runs, running at the end of render so the list and the highlight
// always describe the same screen. Lines outside any entry's span — the
// inter-block blanks — take no part, and lines owned by the frontmatter entry
// are skipped by the same "metadata is not part of the document" call that
// keeps it out of comments and marks: not highlighted, not navigable. searchCurrent
// is clamped to whatever survived, so a narrower query cannot leave n/N
// pointing past the list.
func (m *model) applySearch(lines []string) []string {
	m.searchMatches = m.searchMatches[:0]
	q := m.activeQuery()
	if q == "" {
		return lines
	}
	for li, l := range lines {
		entry, ok := m.entryForLine(li)
		if !ok {
			continue
		}
		if m.entries[entry].b.kind == blockFrontmatter {
			// Frontmatter is metadata, not part of the document: it is a
			// focus stop now (g/j can land on it, the 2026-08-09 frontmatter
			// feedback), but it stays out of comments, marks and search the
			// same way it always has — searching it would make "/" jump to
			// YAML fields no query about the prose is looking for.
			continue
		}
		styled, ranges := highlightSearch(l, q)
		if len(ranges) == 0 {
			continue
		}
		lines[li] = styled
		for _, r := range ranges {
			m.searchMatches = append(m.searchMatches, searchMatch{line: li, lo: r.lo, hi: r.hi, entry: entry})
		}
	}
	if m.searchCurrent >= len(m.searchMatches) {
		m.searchCurrent = len(m.searchMatches) - 1
	}
	if m.searchCurrent < 0 {
		m.searchCurrent = 0
	}
	return lines
}

// entryForLine returns the entry whose rendered span owns line li, or ok=false
// for a line no entry spans (the trailing blank). The frontmatter is an entry
// now and its lines resolve here; applySearch filters them out explicitly.
func (m *model) entryForLine(li int) (int, bool) {
	for i, s := range m.spans {
		if li >= s.start && li <= s.end {
			return i, true
		}
	}
	return 0, false
}

// searchStep moves the current match by d and jumps to it. d == 0 means Enter:
// jump to the first match strictly after the current focus — a fresh query
// starts looking where the reader is — wrapping to the first match when there
// is none ahead. d == ±1 is n/N, stepping the list with wraparound.
func (m *model) searchStep(d int) {
	if len(m.searchMatches) == 0 {
		m.status = "no matches for " + m.searchQuery
		return
	}
	idx := m.searchCurrent
	if d != 0 {
		idx = (idx + d) % len(m.searchMatches)
		if idx < 0 {
			idx += len(m.searchMatches)
		}
	} else {
		curLine := 0
		if m.at.entry >= 0 && m.at.entry < len(m.spans) {
			curLine = m.spans[m.at.entry].start
		}
		idx = 0
		for i, mt := range m.searchMatches {
			if mt.line > curLine {
				idx = i
				break
			}
		}
	}
	m.gotoMatch(idx)
}

// gotoMatch moves focus and the viewport onto match idx: focus lands on the
// owning entry and the scroll centres the match line in the viewport, so the
// highlight that was just reached is the one under the reader's eye. scrollAnchor
// is updated to match, so clampScroll does not immediately re-derive the
// offset away from the centred line (SCROLL-01's decoupling). A search jump is
// a jump — it teleports focus — so the landing is recorded in the jumplist
// before focus moves, and ctrl+o returns to wherever the search was started.
func (m *model) gotoMatch(idx int) {
	if idx < 0 || idx >= len(m.searchMatches) {
		return
	}
	mt := m.searchMatches[idx]
	m.searchCurrent = idx
	m.pushJump(cursor{entry: mt.entry, comment: commentNone})
	m.at = cursor{entry: mt.entry, comment: commentNone}
	viewport := max(m.h-footerRows, 1)
	m.scroll = max(0, mt.line-viewport/2)
	m.scrollAnchor = m.at
	m.visual = false
}

// handleSearchKey drives the open search prompt. esc and ctrl+c cancel the
// edit without committing — the committed query and its highlight are left as
// they were, vim's behaviour when a search command line is abandoned. enter
// commits the draft as the active query and jumps forward; an empty draft
// commits nothing. Backspace deletes a rune, and at an empty draft cancels
// like the palette's backspace. Any other single character appends.
func (m *model) handleSearchKey(msg tea.KeyPressMsg) tea.Cmd {
	switch msg.String() {
	case "esc", "ctrl+c":
		m.searchOpen = false
		m.searchDraft = ""
		return nil
	case "enter":
		m.searchOpen = false
		if m.searchDraft == "" {
			m.searchDraft = ""
			return nil
		}
		m.searchQuery = m.searchDraft
		m.searchDraft = ""
		m.searchStep(0)
		return nil
	case "backspace":
		if m.searchDraft == "" {
			// Nothing left to edit — cancel, the palette's own rule.
			m.searchOpen = false
			return nil
		}
		_, size := utf8.DecodeLastRuneInString(m.searchDraft)
		m.searchDraft = m.searchDraft[:len(m.searchDraft)-size]
		m.searchCurrent = 0
		return nil
	default:
		if len(msg.String()) == 1 {
			m.searchDraft += msg.String()
			m.searchCurrent = 0
		}
		return nil
	}
}

// renderSearchPrompt draws the open search prompt where the palette sits: a
// dim rule to separate it from the document, then the `/` prompt, the typed
// draft, a position count, and a block cursor. The count is the current match
// over the total, so the reader knows how many hits there are before pressing
// enter.
func (m *model) renderSearchPrompt(w int) string {
	var b strings.Builder
	b.WriteString(dimStyle.Render(strings.Repeat("─", w)) + "\n")
	pos := "0/0"
	if len(m.searchMatches) > 0 {
		cur := m.searchCurrent + 1
		if cur < 1 {
			cur = 1
		}
		if cur > len(m.searchMatches) {
			cur = len(m.searchMatches)
		}
		pos = fmt.Sprintf("%d/%d", cur, len(m.searchMatches))
	}
	b.WriteString(focusStyle.Render("/") + m.searchDraft + dimStyle.Render("  "+pos))
	b.WriteString(focusStyle.Render("█"))
	return lipgloss.NewStyle().Width(w).Render(b.String())
}
