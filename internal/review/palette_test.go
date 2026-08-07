package review

import "testing"

// TestMatchCommandsEmptyQueryReturnsRegistryOrder: nothing typed yet means no
// ranking has happened, so the result is just the registry, unreordered —
// see matchCommands' own doc comment on why this leg does not invent a
// default ordering for that case.
func TestMatchCommandsEmptyQueryReturnsRegistryOrder(t *testing.T) {
	got := matchCommands(commands, "")
	// command carries func fields, which reflect.DeepEqual never treats as
	// equal even when they are the same value — compare by id order instead,
	// which is exactly what this test cares about.
	gotIDs, wantIDs := ids(got), ids(commands)
	if len(gotIDs) != len(wantIDs) {
		t.Fatalf("matchCommands(commands, \"\") = %v, want %v", gotIDs, wantIDs)
	}
	for i := range wantIDs {
		if gotIDs[i] != wantIDs[i] {
			t.Fatalf("matchCommands(commands, \"\") = %v, want %v", gotIDs, wantIDs)
		}
	}
}

// TestMatchCommandsFiltersNonMatches: a query that is not a subsequence of
// any command's id or description excludes it entirely — requirement 4's
// "should not be listed at all rather than listed and failing" applies just
// as much to a query that finds nothing.
func TestMatchCommandsFiltersNonMatches(t *testing.T) {
	got := matchCommands(commands, "zzzzzznotacommand")
	if len(got) != 0 {
		t.Fatalf("matchCommands with an unmatchable query returned %d results, want 0", len(got))
	}
}

// TestMatchCommandsMatchesByID exercises requirement 3 directly: "mark.rev"
// is exactly the kind of half-remembered fragment the feedback names, and it
// must find mark.reviewed and nothing that only shares a prefix with it.
func TestMatchCommandsMatchesByID(t *testing.T) {
	got := matchCommands(commands, "mark.rev")
	if len(got) != 1 || got[0].ID != "mark.reviewed" {
		t.Fatalf("matchCommands(commands, %q) = %v, want exactly [mark.reviewed]", "mark.rev", ids(got))
	}
}

// TestMatchCommandsMatchesByDescription: requirement 3 also says the id and
// the description are both searchable. "clipboard" appears only in
// review.export's description, never in any command's id.
func TestMatchCommandsMatchesByDescription(t *testing.T) {
	got := matchCommands(commands, "clipboard")
	if len(got) != 1 || got[0].ID != "review.export" {
		t.Fatalf("matchCommands(commands, %q) = %v, want exactly [review.export]", "clipboard", ids(got))
	}
}

// TestMatchCommandsIsCaseInsensitive: a query typed in the wrong case must
// still find its command — nothing about requirement 1 asks the reviewer to
// match case exactly.
func TestMatchCommandsIsCaseInsensitive(t *testing.T) {
	got := matchCommands(commands, "MARK.REVIEWED")
	if len(got) != 1 || got[0].ID != "mark.reviewed" {
		t.Fatalf("matchCommands with an uppercase query = %v, want exactly [mark.reviewed]", ids(got))
	}
}

// TestMatchCommandsRanksPrefixOverScattered: two synthetic commands both
// match "mv" as a subsequence, but one has it as a leading, word-boundary
// prefix ("mv.up") and the other only as scattered letters deep inside a
// longer word ("summary.view"). Requirement 1 asks for ranking, not just
// filtering, and this is the case that distinguishes the two: a plain
// substring/subsequence filter would leave their order undefined.
func TestMatchCommandsRanksPrefixOverScattered(t *testing.T) {
	near := command{ID: "mv.up", Description: "close match"}
	far := command{ID: "summary.view", Description: "distant match"}
	cmds := []command{far, near} // registry order deliberately disfavours near

	got := matchCommands(cmds, "mv")
	if len(got) != 2 {
		t.Fatalf("matchCommands returned %d results, want 2", len(got))
	}
	if got[0].ID != "mv.up" {
		t.Fatalf("matchCommands(%q) ranked %v first, want mv.up ranked above summary.view", "mv", ids(got))
	}
}

// TestMatchCommandsStableOrderOnTie: two commands that score identically
// against the query keep their original registry order, so results do not
// jitter for no reason a reviewer could see.
func TestMatchCommandsStableOrderOnTie(t *testing.T) {
	first := command{ID: "aaa.xyz", Description: ""}
	second := command{ID: "bbb.xyz", Description: ""}
	cmds := []command{first, second}

	got := matchCommands(cmds, "xyz")
	if len(got) != 2 || got[0].ID != "aaa.xyz" || got[1].ID != "bbb.xyz" {
		t.Fatalf("matchCommands with a tied query = %v, want registry order [aaa.xyz bbb.xyz]", ids(got))
	}
}

// TestFuzzyScoreRejectsOutOfOrderQueries: a subsequence match must respect
// order — "ba" is not a match for "abc" just because both letters appear.
func TestFuzzyScoreRejectsOutOfOrderQueries(t *testing.T) {
	if _, ok := fuzzyScore("abc", "ba"); ok {
		t.Fatalf("fuzzyScore(\"abc\", \"ba\") reported a match; \"ba\" is not a subsequence of \"abc\"")
	}
}

// TestFuzzyScoreRewardsWordBoundaryAndRun: a match starting right after a
// word boundary and continuing as a consecutive run should score strictly
// higher than the same query letters found scattered mid-word. Each query
// letter appears exactly once in each target, so greedy left-to-right
// matching has only one possible alignment in either case — the comparison
// isolates the boundary/run bonus rather than an accident of which
// occurrence got picked.
func TestFuzzyScoreRewardsWordBoundaryAndRun(t *testing.T) {
	boundary, ok := fuzzyScore("abc.xyz", "xyz") // xyz starts right after '.', runs to the end
	if !ok {
		t.Fatalf("fuzzyScore(%q, %q) unexpectedly did not match", "abc.xyz", "xyz")
	}
	scattered, ok := fuzzyScore("axbyccz", "xyz") // x, y, z present but scattered mid-word
	if !ok {
		t.Fatalf("fuzzyScore(%q, %q) unexpectedly did not match", "axbyccz", "xyz")
	}
	if boundary <= scattered {
		t.Fatalf("fuzzyScore(%q, %q) = %d, fuzzyScore(%q, %q) = %d; want the word-boundary run to score higher",
			"abc.xyz", "xyz", boundary, "axbyccz", "xyz", scattered)
	}
}

// ids extracts command ids for readable test failure messages.
func ids(cmds []command) []string {
	out := make([]string, len(cmds))
	for i, c := range cmds {
		out[i] = c.ID
	}
	return out
}

// --- CMD-04: focus-sensitive listing and the titled target -------------------

// TestPaletteTitleFallsBackToDescriptionWhenTargetIsNil: a command with no
// Target (move.*, review.export, app.quit) is titled by its description
// alone, no matter where focus sits.
func TestPaletteTitleFallsBackToDescriptionWhenTargetIsNil(t *testing.T) {
	m := newTestModel(t)
	down, ok := commandByID("move.down")
	if !ok {
		t.Fatal("move.down is not registered")
	}
	if got := paletteTitle(down, m); got != down.Description {
		t.Errorf("paletteTitle(move.down) = %q, want the bare description %q", got, down.Description)
	}
}

// TestPaletteTitleAppendsTarget: a command whose Target has something to say
// appends it after an em dash, matching requirement 4's own example — "Delete
// — comment by agent" — with mark.reviewed and a heading standing in since
// there is no delete command yet (blocked on Q-0002).
func TestPaletteTitleAppendsTarget(t *testing.T) {
	m := newTestModel(t)
	m.at = cursor{entry: blockEntryFor(t, m, "^h1"), comment: commentNone}
	reviewed, ok := commandByID("mark.reviewed")
	if !ok {
		t.Fatal("mark.reviewed is not registered")
	}
	want := "Mark reviewed — section (3 blocks)"
	if got := paletteTitle(reviewed, m); got != want {
		t.Errorf("paletteTitle(mark.reviewed) = %q, want %q", got, want)
	}
}

// TestPaletteTitleFallsBackToDescriptionWhenTargetIsEmpty: Applicable and
// Target are asked separately, and Applicable being true does not guarantee
// Target has anything to add (see TestEditTargetEmptyWhenNothingSpecificToEdit)
// — the title falls back to the description rather than showing a bare
// trailing dash.
func TestPaletteTitleFallsBackToDescriptionWhenTargetIsEmpty(t *testing.T) {
	m := newTestModel(t)
	m.at = cursor{entry: entryFor(t, m, soloAnchor), comment: commentNone}
	edit, ok := commandByID("comment.edit")
	if !ok {
		t.Fatal("comment.edit is not registered")
	}
	if !edit.Applicable(m) {
		t.Fatal("test setup: comment.edit should be applicable on a thread entry")
	}
	if got := paletteTitle(edit, m); got != edit.Description {
		t.Errorf("paletteTitle(comment.edit) = %q, want the bare description %q", got, edit.Description)
	}
}

// TestPaletteRowsExcludesInapplicableCommands: focus on a plain paragraph
// with no thread yet excludes comment.edit (nothing to edit) but keeps the
// mark commands and comment.new; registry order is preserved.
func TestPaletteRowsExcludesInapplicableCommands(t *testing.T) {
	m := newTestModel(t)
	m.at = cursor{entry: blockEntryFor(t, m, freshAnchor), comment: commentNone}

	rows := paletteRows(m, commands)
	got := make([]string, len(rows))
	for i, r := range rows {
		got[i] = r.Command.ID
	}
	for _, want := range []string{"move.down", "comment.new", "mark.reviewed", "review.export", "app.quit"} {
		if !containsID(got, want) {
			t.Errorf("paletteRows on a fresh paragraph = %v, want it to include %q", got, want)
		}
	}
	if containsID(got, "comment.edit") {
		t.Errorf("paletteRows on a fresh paragraph = %v, want it to exclude comment.edit (no thread there)", got)
	}
}

// TestPaletteRowsExcludesMarkCommandsOnAThreadEntry: focus on a thread entry
// itself (nothing sectionAnchors considers markable) drops all three mark
// commands, but comment.edit is applicable — the inverse case from
// TestPaletteRowsExcludesInapplicableCommands, so the two together pin that
// applicability is judged per command, not per focus position wholesale.
func TestPaletteRowsExcludesMarkCommandsOnAThreadEntry(t *testing.T) {
	m := newTestModel(t)
	m.at = cursor{entry: entryFor(t, m, convoAnchor), comment: commentNone}

	rows := paletteRows(m, commands)
	got := make([]string, len(rows))
	for i, r := range rows {
		got[i] = r.Command.ID
	}
	for _, unwanted := range []string{"mark.reviewed", "mark.flagged", "mark.cycle"} {
		if containsID(got, unwanted) {
			t.Errorf("paletteRows on a thread entry = %v, want it to exclude %q", got, unwanted)
		}
	}
	if !containsID(got, "comment.edit") {
		t.Errorf("paletteRows on a thread entry = %v, want it to include comment.edit", got)
	}
}

func containsID(ids []string, want string) bool {
	for _, id := range ids {
		if id == want {
			return true
		}
	}
	return false
}
