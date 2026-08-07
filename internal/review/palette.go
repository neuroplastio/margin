// Palette logic: given the command registry, a typed query, and the model's
// current focus, decide which commands to show and how to title them.
// Mechanical, no UI — not wired to a ':' key, nothing here renders. That is
// CMD-03 and the rest of the command-palette feedback (2026-08-07-command-
// palette, drained by CMD-01 — see vault/journal/2026-08-07.6.md).
//
// Two independent slices of that feedback live in this file:
//
// Requirement 1 (CMD-02): "Filtering happens as you type and results are
// ranked, not merely substring-filtered." matchCommands and fuzzyScore are a
// subsequence match against both a command's id and its description, scored
// so a tighter, earlier match outranks one that is merely present somewhere
// in a long string.
//
// Requirement 4 (CMD-04): "It offers what applies to what is focused, and
// names the target." paletteRows and paletteTitle filter the registry by
// Applicable against the current focus and title each surviving command with
// what it would act on.
package review

import (
	"sort"
	"strings"
)

// matchCommands ranks cmds against query and returns the matches, best
// first. An empty query returns every command in registry order, unranked —
// requirement 1 only calls for ranking once there is something to filter by;
// what an unranked palette shows before you type anything is explicitly left
// open in the feedback's own "Not settled" section, so this leg does not
// invent an answer for it. A query that matches nothing returns an empty
// slice: commands that do not apply should not be listed at all (the same
// principle requirement 4 states for focus-applicability), not listed and
// scored last.
func matchCommands(cmds []command, query string) []command {
	if query == "" {
		out := make([]command, len(cmds))
		copy(out, cmds)
		return out
	}

	type scored struct {
		cmd   command
		score int
		index int // original registry position, for a stable tie-break
	}
	matches := make([]scored, 0, len(cmds))
	for i, c := range cmds {
		idScore, idOK := fuzzyScore(c.ID, query)
		descScore, descOK := fuzzyScore(c.Description, query)
		if !idOK && !descOK {
			continue
		}
		best := idScore
		if descOK && (!idOK || descScore > best) {
			best = descScore
		}
		matches = append(matches, scored{cmd: c, score: best, index: i})
	}

	sort.SliceStable(matches, func(a, b int) bool {
		if matches[a].score != matches[b].score {
			return matches[a].score > matches[b].score
		}
		return matches[a].index < matches[b].index
	})

	out := make([]command, len(matches))
	for i, m := range matches {
		out[i] = m.cmd
	}
	return out
}

// fuzzyScore reports whether every rune in query appears in target, in
// order, case-insensitively — a subsequence match, the same relationship
// fzf-style fuzzy finders use. ok is false when query is not a subsequence
// of target at all, in which case score is meaningless and the caller must
// not use it.
//
// When it is a subsequence, score rewards two things a plain substring test
// would not: a match that starts right after a word boundary (the start of
// the string, or just after '.', '_', '-', or a space — the separators that
// actually appear in this project's command ids, e.g. "mark.reviewed"), and
// a run of consecutive matched characters. Both track the same intuition:
// "mark.rev" finding "mark.reviewed" by matching its whole prefix should
// clearly outrank a query that only turns up as scattered letters deep
// inside a longer word.
func fuzzyScore(target, query string) (score int, ok bool) {
	t := []rune(strings.ToLower(target))
	q := []rune(strings.ToLower(query))
	if len(q) == 0 {
		return 0, true
	}

	ti, qi := 0, 0
	lastMatch := -2
	for ti < len(t) && qi < len(q) {
		if t[ti] == q[qi] {
			points := 1
			if ti == 0 || isWordBoundary(t[ti-1]) {
				points += 3
			}
			if ti == lastMatch+1 {
				points += 2
			}
			score += points
			lastMatch = ti
			qi++
		}
		ti++
	}
	if qi < len(q) {
		return 0, false
	}
	// A tie-break, not the main signal: among equally good matches, prefer
	// the shorter target, since the same match is denser there.
	score -= len(t) / 8
	return score, true
}

// isWordBoundary reports whether r is one of the separators that appear in
// this project's command ids and descriptions, so the character after it
// counts as the start of a new word for fuzzyScore's bonus.
func isWordBoundary(r rune) bool {
	return r == '.' || r == '_' || r == '-' || r == ' '
}

// paletteRow is one row the palette would show at a given moment: a command
// together with the title that names what it is about to act on, per
// requirement 4 of the command-palette feedback — "the difference between
// 'Delete' and 'Delete — comment by agent' is the difference between
// confidence and a guess."
type paletteRow struct {
	Command command
	Title   string
}

// paletteRows produces the palette's rows for m's current focus: every
// command from cmds whose Applicable reports true against m, in registry
// order, each carrying its title. A command that does not apply is left out
// entirely, not listed and disabled — the same rule requirement 4 states for
// a query that matches nothing (matchCommands, CMD-02).
//
// This says nothing about a typed query. Focus-applicability and
// search-ranking are two different filters the palette (CMD-03) will need to
// compose, and which one narrows first — or whether composing them even
// matters, since both are pure filters over the same list — is not something
// this leg decides without a screen to judge it on.
func paletteRows(m *model, cmds []command) []paletteRow {
	out := make([]paletteRow, 0, len(cmds))
	for _, c := range cmds {
		if !c.Applicable(m) {
			continue
		}
		out = append(out, paletteRow{Command: c, Title: paletteTitle(c, m)})
	}
	return out
}

// paletteTitle names what c is about to act on, given m's focus. A command
// with no focus-specific Target — or whose Target has nothing more specific
// to say than "yes, this applies" (an empty string) — is titled by its
// description alone.
func paletteTitle(c command, m *model) string {
	if c.Target == nil {
		return c.Description
	}
	if t := c.Target(m); t != "" {
		return c.Description + " — " + t
	}
	return c.Description
}
