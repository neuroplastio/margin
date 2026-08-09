package review

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
)

// The 2026-08-09 navigation-feature-requests feedback, feature 1: `/` opens a
// search prompt, typing highlights every match in the document live, enter
// commits and jumps to the next match after the current focus, and n/N walk
// the match list.

func searchType(m *model, s string) {
	for _, r := range s {
		m.handleSearchKey(tea.KeyPressMsg(tea.Key{Code: r, Text: string(r)}))
	}
}

func searchEnter(m *model) {
	m.handleSearchKey(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
}

// TestSearchSlashOpensPrompt: `/` opens the prompt with an empty draft and no
// committed query.
func TestSearchSlashOpensPrompt(t *testing.T) {
	m := newTestModel(t)
	m.handleKey(tea.KeyPressMsg(tea.Key{Code: '/', Text: "/"}))
	if !m.searchOpen {
		t.Fatal("'/' did not open the search prompt")
	}
	if m.searchDraft != "" {
		t.Errorf("searchDraft = %q on open, want empty", m.searchDraft)
	}
}

// TestSearchTypingPaintsMatches: typing a query highlights every rendered line
// that contains it and fills the match list, before anything is committed.
func TestSearchTypingPaintsMatches(t *testing.T) {
	m := newTestModel(t)
	m.handleKey(tea.KeyPressMsg(tea.Key{Code: '/', Text: "/"}))
	searchType(m, "retry")

	lines := m.render()
	painted := 0
	for _, l := range lines {
		if strings.Contains(l, searchBG) {
			painted++
		}
	}
	if painted == 0 {
		t.Fatal("no rendered line carries the search highlight")
	}
	if len(m.searchMatches) == 0 {
		t.Fatal("typing a query that appears in the doc left the match list empty")
	}
	for _, mt := range m.searchMatches {
		if strings.Contains(strings.ToLower(ansiRe.ReplaceAllString(lines[mt.line], "")), "retry") {
			continue
		}
		t.Errorf("match at line %d does not contain the query", mt.line)
	}
}

// TestSearchIsCaseInsensitive: "RETRY" finds the same matches as "retry".
func TestSearchIsCaseInsensitive(t *testing.T) {
	m := newTestModel(t)
	m.handleKey(tea.KeyPressMsg(tea.Key{Code: '/', Text: "/"}))
	searchType(m, "RETRY")
	m.render()
	if len(m.searchMatches) == 0 {
		t.Fatal("uppercase query found nothing; search should be case-insensitive")
	}
}

// TestSearchEnterCommitsAndJumps: enter commits the draft as the query, closes
// the prompt, and jumps focus to the entry holding the next match after the
// current position, scrolling to bring the match line into view.
func TestSearchEnterCommitsAndJumps(t *testing.T) {
	m := newTestModel(t)
	// Focus sits on the first block (a heading). "backoff" first appears in
	// the ^a1 paragraph (entry 1), so enter should land there — the strict
	// next match after the current focus line, not the heading's own line.
	m.handleKey(tea.KeyPressMsg(tea.Key{Code: '/', Text: "/"}))
	searchType(m, "backoff")
	m.render() // the frame loop renders between a keypress and the next
	searchEnter(m)

	if m.searchOpen {
		t.Fatal("enter left the search prompt open")
	}
	if m.searchQuery != "backoff" {
		t.Errorf("searchQuery = %q after enter, want %q", m.searchQuery, "backoff")
	}
	if m.at.entry != 1 {
		t.Errorf("enter landed focus on entry %d, want the first paragraph (%d)", m.at.entry, 1)
	}
}

// TestSearchEnterEmptyDraftCommitsNothing: enter with an empty draft does not
// install a query.
func TestSearchEnterEmptyDraftCommitsNothing(t *testing.T) {
	m := newTestModel(t)
	m.handleKey(tea.KeyPressMsg(tea.Key{Code: '/', Text: "/"}))
	searchEnter(m)
	if m.searchOpen {
		t.Fatal("enter on an empty draft left the prompt open")
	}
	if m.searchQuery != "" {
		t.Errorf("searchQuery = %q, want empty (nothing committed)", m.searchQuery)
	}
}

// TestSearchEscAbandonsEdit: esc cancels the prompt's edit without committing
// — the draft is dropped and any previously committed query survives.
func TestSearchEscAbandonsEdit(t *testing.T) {
	m := newTestModel(t)
	m.handleKey(tea.KeyPressMsg(tea.Key{Code: '/', Text: "/"}))
	searchType(m, "retry")
	m.handleSearchKey(tea.KeyPressMsg(tea.Key{Code: tea.KeyEsc}))
	if m.searchOpen {
		t.Fatal("esc left the search prompt open")
	}
	if m.searchDraft != "" {
		t.Errorf("searchDraft = %q after esc, want dropped", m.searchDraft)
	}
	if m.searchQuery != "" {
		t.Errorf("searchQuery = %q, want empty (nothing was committed)", m.searchQuery)
	}
}

// TestSearchEscKeepsCommittedQuery: opening the prompt again and abandoning
// the edit leaves the previous committed search active.
func TestSearchEscKeepsCommittedQuery(t *testing.T) {
	m := newTestModel(t)
	m.handleKey(tea.KeyPressMsg(tea.Key{Code: '/', Text: "/"}))
	searchType(m, "retry")
	searchEnter(m)

	m.handleKey(tea.KeyPressMsg(tea.Key{Code: '/', Text: "/"}))
	searchType(m, "zz")
	m.handleSearchKey(tea.KeyPressMsg(tea.Key{Code: tea.KeyEsc}))
	if m.searchQuery != "retry" {
		t.Errorf("searchQuery = %q after abandoning a new search, want the committed %q", m.searchQuery, "retry")
	}
}

// TestSearchBackspaceDeletesThenCancels: backspace deletes one rune at a time,
// and at an empty draft cancels like the palette — the prompt closes.
func TestSearchBackspaceDeletesThenCancels(t *testing.T) {
	m := newTestModel(t)
	m.handleKey(tea.KeyPressMsg(tea.Key{Code: '/', Text: "/"}))
	searchType(m, "ab")
	m.handleSearchKey(tea.KeyPressMsg(tea.Key{Code: tea.KeyBackspace}))
	if m.searchDraft != "a" || !m.searchOpen {
		t.Errorf("after one backspace: draft = %q open = %v, want %q still open", m.searchDraft, m.searchOpen, "a")
	}
	m.handleSearchKey(tea.KeyPressMsg(tea.Key{Code: tea.KeyBackspace}))
	if m.searchDraft != "" || !m.searchOpen {
		t.Errorf("after two backspaces: draft = %q open = %v, want empty draft, still open", m.searchDraft, m.searchOpen)
	}
	m.handleSearchKey(tea.KeyPressMsg(tea.Key{Code: tea.KeyBackspace}))
	if m.searchOpen {
		t.Error("backspace past the last character left the prompt open, want it cancelled")
	}
}

// TestSearchNStepsMatches: after a committed search, n moves to the next match
// and N to the previous, wrapping at both ends.
func TestSearchNStepsMatches(t *testing.T) {
	m := newTestModel(t)
	m.handleKey(tea.KeyPressMsg(tea.Key{Code: '/', Text: "/"}))
	searchType(m, "retry")
	m.render()
	n := len(m.searchMatches)
	if n < 2 {
		t.Fatalf("query %q found %d matches, want at least 2 to walk", "retry", n)
	}

	// Start on the last match, then wrap past the end.
	m.gotoMatch(n - 1)
	first := m.searchCurrent
	m.searchStep(1)
	if m.searchCurrent != 0 {
		t.Errorf("n past the last match landed on %d, want wrap to %d", m.searchCurrent, 0)
	}
	m.searchStep(-1)
	if m.searchCurrent != first {
		t.Errorf("N before the first match landed on %d, want wrap to %d", m.searchCurrent, first)
	}
}

// TestSearchNextPrevKeysRouteThroughRegistry: n/N resolve to the search
// commands through the keymap.
func TestSearchNextPrevKeysRouteThroughRegistry(t *testing.T) {
	if id := keymap["n"]; id != "search.next" {
		t.Errorf("keymap[\"n\"] = %q, want search.next", id)
	}
	if id := keymap["N"]; id != "search.prev" {
		t.Errorf("keymap[\"N\"] = %q, want search.prev", id)
	}
}

// TestSearchNoMatchesStatus: stepping with nothing matching reports it.
func TestSearchNoMatchesStatus(t *testing.T) {
	m := newTestModel(t)
	m.handleKey(tea.KeyPressMsg(tea.Key{Code: '/', Text: "/"}))
	searchType(m, "zzqqzz")
	m.render()
	m.handleSearchKey(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	if m.status == "" {
		t.Fatal("a search with no matches set no status")
	}
	if !strings.Contains(m.status, "no matches") {
		t.Errorf("status = %q, want it to say there are no matches", m.status)
	}
}

// TestHighlightSearchPaintsOnlyMatchedRuns: highlightSearch paints exactly the
// matched runes with the background and leaves the rest of the styled line —
// including a foreground colour set before the match — untouched.
func TestHighlightSearchPaintsOnlyMatchedRuns(t *testing.T) {
	line := "\x1b[38;5;252mThe retry budget\x1b[0m"
	styled, ranges := highlightSearch(line, "retry")
	if len(ranges) != 1 || ranges[0].lo != 4 || ranges[0].hi != 9 {
		t.Fatalf("ranges = %+v, want one match at [4,9)", ranges)
	}
	if !strings.Contains(styled, searchBG+"retry") {
		t.Errorf("styled line %q does not open the highlight before the match", styled)
	}
	// The plain text is unchanged; only the background codes were added.
	if plain := ansiRe.ReplaceAllString(styled, ""); plain != "The retry budget" {
		t.Errorf("plain text = %q, want %q", plain, "The retry budget")
	}
}

// TestHighlightSearchReassertsAcrossInnerResets: a full reset inside the match
// — the boundary of an inline bold run split by the query — must not clear the
// background mid-hit.
func TestHighlightSearchReassertsAcrossInnerResets(t *testing.T) {
	line := "\x1b[38;5;252mfoo \x1b[1mb\x1b[0mar\x1b[0m baz\x1b[0m"
	styled, ranges := highlightSearch(line, "bar")
	if len(ranges) != 1 {
		t.Fatalf("ranges = %+v, want one", ranges)
	}
	// The reset after the bold 'b' sits strictly inside the match (between b
	// and a), so the background must be reasserted after it: once to open,
	// once to reassert, and the match closed cleanly after the last rune.
	if strings.Count(styled, searchBG) != 2 {
		t.Errorf("styled line %q: searchBG appears %d times, want 2 (open + reassert)", styled, strings.Count(styled, searchBG))
	}
	if !strings.Contains(styled, "\x1b[0m"+searchBG) {
		t.Errorf("styled line %q does not reassert the background after the inner reset", styled)
	}
}

// TestSearchPromptRenders: the open prompt renders a dim rule and the `/`
// draft with a match count.
func TestSearchPromptRenders(t *testing.T) {
	m := newTestModel(t)
	m.handleKey(tea.KeyPressMsg(tea.Key{Code: '/', Text: "/"}))
	searchType(m, "retry")
	m.render()
	box := m.renderSearchPrompt(m.w)
	plain := ansiRe.ReplaceAllString(box, "")
	if !strings.Contains(plain, "/retry") {
		t.Errorf("prompt %q does not show the draft", plain)
	}
	if !strings.Contains(plain, "/") {
		t.Errorf("prompt %q does not show the leading slash", plain)
	}
}

// TestSearchMatchesExcludeFrontmatter: lines outside any entry's span — a
// document's frontmatter — are not searched, matching how rebuild excludes the
// frontmatter from everything else.
func TestSearchMatchesExcludeFrontmatter(t *testing.T) {
	doc, threads := seedDoc()
	doc = append([]block{
		{kind: blockFrontmatter, text: "---\ntitle: The retry budget\ntags: retry\n---", anchor: ""},
	}, doc...)
	m := newModelAt("document.md", doc, threads)
	m.w, m.h = 100, 60
	m.handleKey(tea.KeyPressMsg(tea.Key{Code: '/', Text: "/"}))
	searchType(m, "retry")
	m.render()
	for _, mt := range m.searchMatches {
		if e := m.entries[mt.entry].b.kind; e == blockFrontmatter {
			t.Errorf("frontmatter block leaked into the match list")
		}
	}
}
