// Palette matching: given the command registry and a query string, rank and
// filter commands by how well they match. This is CMD-02 — mechanical, no
// UI. It is not wired to a ':' key, no palette renders, nothing here decides
// where results would appear or how many rows they get; that is CMD-03 and
// the rest of the command-palette feedback (2026-08-07-command-palette,
// drained by CMD-01 — see vault/journal/2026-08-07.6.md).
//
// Requirement 1 of that feedback: "Filtering happens as you type and results
// are ranked, not merely substring-filtered — typing a few characters of
// what you half-remember should surface the right verb." That is the whole
// spec this leg implements: a fuzzy subsequence match against both a
// command's id and its description, scored so a tighter, earlier match
// outranks one that is merely present somewhere in a long string.
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
