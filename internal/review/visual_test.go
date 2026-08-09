package review

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
)

// The 2026-08-09 visual-mode feedback asked for a Shift+V blockwise selection
// and a `y` that copies the selected text. A terminal delivers Shift+V as
// "V", which thread.showDeleted gave up for it (moving to T).

func pressKey(m *model, key string) {
	code := []rune(key)[0]
	if len(key) > 1 {
		code = 0 // named keys (esc, space) carry no rune; String() is what resolves
	}
	m.handleKey(tea.KeyPressMsg(tea.Key{Code: code, Text: key}))
}

// TestVisualModeExtendsWithMovement: V anchors the selection where focus
// sits and every movement extends it to the new focus; esc cancels.
func TestVisualModeExtendsWithMovement(t *testing.T) {
	m := newTestModel(t)
	m.at = cursor{entry: 1, comment: commentNone}

	pressKey(m, "V")
	if !m.visual {
		t.Fatal("V did not enter visual mode")
	}
	lo, hi, ok := m.selectionRange()
	if !ok || lo != 1 || hi != 1 {
		t.Fatalf("selection at anchor = (%d, %d, %v), want (1, 1, true)", lo, hi, ok)
	}

	pressKey(m, "j")
	pressKey(m, "j")
	lo, hi, ok = m.selectionRange()
	if !ok || lo != 1 || hi != 3 {
		t.Fatalf("selection after jj = (%d, %d, %v), want (1, 3, true)", lo, hi, ok)
	}

	pressKey(m, "esc")
	if m.visual {
		t.Fatal("esc did not cancel the selection")
	}
	if _, _, ok := m.selectionRange(); ok {
		t.Fatal("selectionRange still reports a range after esc")
	}
}

// TestVisualModeTogglesOffOnSecondV: V in visual mode leaves it, vim's rule.
func TestVisualModeTogglesOffOnSecondV(t *testing.T) {
	m := newTestModel(t)
	pressKey(m, "V")
	if !m.visual {
		t.Fatal("V did not enter visual mode")
	}
	pressKey(m, "V")
	if m.visual {
		t.Fatal("second V did not leave visual mode")
	}
}

// TestVisualModeMovesByEntriesNotComments: the selection is blockwise, so
// j/k in visual mode never dives into a thread's comments — and entering
// visual lifts a comment-level focus onto its block rather than leaving the
// cursor where the entry-only stop list cannot see it.
func TestVisualModeMovesByEntriesNotComments(t *testing.T) {
	m := newTestModel(t)
	i := entryFor(t, m, convoAnchor) // its thread has two posted comments

	m.at = cursor{entry: i, comment: 1}
	pressKey(m, "V")
	if m.at.comment != commentNone {
		t.Fatalf("entering visual left focus on comment %d, want the block", m.at.comment)
	}

	pressKey(m, "j")
	if m.at.comment != commentNone {
		t.Fatalf("j in visual mode dove to comment %d", m.at.comment)
	}
	if m.at.entry == i {
		t.Fatal("j in visual mode did not move to the next entry")
	}
}

// TestNonMovementCommandExitsVisual: movement and the selection commands
// keep the selection alive; anything else ends it, vim's rule — so a mark
// applied with a selection up does not keep a stale highlight behind it.
func TestNonMovementCommandExitsVisual(t *testing.T) {
	m := newTestModel(t)
	m.at = cursor{entry: 1, comment: commentNone}
	pressKey(m, "V")
	pressKey(m, "j")
	if !m.visual {
		t.Fatal("movement ended the selection")
	}
	pressKey(m, "r")
	if m.visual {
		t.Fatal("mark.reviewed did not end the selection")
	}
}

// TestSelectionRendersBackgroundAndGutter: a selected block's text lines
// carry the selection background and its gutter bar the selection colour;
// blocks outside the range render plain; the thread inside the range — the
// conversation, not the document — gets neither.
func TestSelectionRendersBackgroundAndGutter(t *testing.T) {
	m := newTestModel(t)
	m.at = cursor{entry: 1, comment: commentNone} // ^a1
	pressKey(m, "V")
	for m.at.entry < 4 { // down over ^b2, its thread, and ^c3
		pressKey(m, "j")
	}
	lines := m.render()

	for i := m.spans[1].start; i <= m.spans[1].end; i++ {
		if lines[i] != "" && !strings.Contains(lines[i], selBG) {
			t.Errorf("selected block line %d = %q, want the selection background", i, lines[i])
		}
	}
	threadEntry := 3 // ^b2's thread, spliced beneath it
	if m.entries[threadEntry].thread == nil {
		t.Fatalf("entry %d is not a thread; fixture drifted", threadEntry)
	}
	for i := m.spans[threadEntry].start; i <= m.spans[threadEntry].end; i++ {
		if strings.Contains(lines[i], selBG) {
			t.Errorf("thread line %d = %q carries the selection background", i, lines[i])
		}
	}
	for i := m.spans[0].start; i <= m.spans[0].end; i++ {
		if strings.Contains(lines[i], selBG) {
			t.Errorf("unselected heading line %d = %q carries the selection background", i, lines[i])
		}
	}
	// The focused endpoint keeps the focus colour, so the selection's
	// moving end reads at a glance.
	if !strings.Contains(lines[m.spans[m.at.entry].start], focusStyle.Render("▌")) {
		t.Error("focused selection endpoint lost the focus bar")
	}
	if !strings.Contains(lines[m.spans[1].start], selStyle.Render("▌")) {
		t.Error("selected but unfocused block lacks the selection gutter bar")
	}
}

// TestSelLineReassertsBackground: a pre-styled line (chroma's own resets)
// keeps the selection background across its whole width — the wrap-in-a-
// lipgloss-style approach loses it at the first inner reset, which is why
// selLine reasserts instead.
func TestSelLineReassertsBackground(t *testing.T) {
	styled := highlightCode([]string{"func f() { return }"}, "go")[0]
	if !strings.Contains(styled, "\x1b[") {
		t.Fatal("chroma produced no styling; the test pins nothing")
	}
	got := selLine(styled)
	if !strings.HasPrefix(got, selBG) {
		t.Error("selLine does not open with the selection background")
	}
	// Every inner reset — either spelling — must be followed by the
	// background being reasserted, or the tail of the line goes unpainted.
	// The one legitimate bare reset is the terminator selLine appends.
	inner := strings.TrimSuffix(got, "\x1b[m")
	for _, reset := range []string{"\x1b[m", "\x1b[0m"} {
		if strings.Contains(strings.ReplaceAll(inner, reset+selBG, ""), reset) {
			t.Errorf("an inner %q reset is not reasserted: %q", reset, got)
		}
	}
	if selLine("") != "" {
		t.Error("selLine painted an empty line — the gaps between blocks must stay blank")
	}
}

// TestBlockSource per kind: yanking puts markdown source on the clipboard,
// not the rendered view — the paste lands in something that reads markdown.
func TestBlockSource(t *testing.T) {
	blocks := parseDoc([]byte(sampleDoc))
	byText := func(want string) block {
		t.Helper()
		for _, b := range blocks {
			if strings.Contains(b.text, want) {
				return b
			}
		}
		t.Fatalf("no block containing %q", want)
		return block{}
	}

	if got := blockSource(byText("Budgets")); got != "## Budgets" {
		t.Errorf("heading source = %q, want %q", got, "## Budgets")
	}
	if got := blockSource(byText("per-endpoint caps")); got != "- per-endpoint caps" {
		t.Errorf("list item source = %q, want %q", got, "- per-endpoint caps")
	}
	if got := blockSource(byText("Breaker state")); got != "> Breaker state is per-process." {
		t.Errorf("quote source = %q, want the > marker restored", got)
	}
	code := byText("func retry")
	want := "```go\nfunc retry(n int) error {\n    return nil\n}\n```"
	if got := blockSource(code); got != want {
		t.Errorf("code source = %q, want %q", got, want)
	}
	para := byText("Each outbound call")
	if got := blockSource(para); !strings.HasPrefix(got, "Each outbound call is retried") {
		t.Errorf("paragraph source = %q, want the source text", got)
	}
}

// TestYankWithoutSelectionYanksTheFocusedBlock: `y` with no selection is the
// degenerate range of one — the focused block — and it leaves a real
// clipboard command behind (OSC 52 at minimum).
func TestYankWithoutSelectionYanksTheFocusedBlock(t *testing.T) {
	m := newTestModel(t)
	m.at = cursor{entry: 1, comment: commentNone}
	cmd := m.yankSelection()
	if cmd == nil {
		t.Fatal("yank produced no command; the OSC 52 write is a tea.Cmd")
	}
	if strings.Contains(m.status, "nothing to yank") {
		t.Fatalf("status = %q; a focused paragraph is yankable", m.status)
	}
}

// TestYankEndsVisualAndSkipsThreads: yanking a selection copies blocks, not
// the conversation hanging off them, and always leaves visual mode.
func TestYankEndsVisualAndSkipsThreads(t *testing.T) {
	m := newTestModel(t)
	m.at = cursor{entry: 1, comment: commentNone}
	pressKey(m, "V")
	pressKey(m, "j")
	pressKey(m, "y")
	if m.visual {
		t.Fatal("yank did not leave visual mode")
	}
	if cmd := m.yankSelection(); cmd == nil {
		t.Fatal("yank produced no command")
	}
}

// TestShowDeletedMovedToT: the reveal toggle gave V up to visual mode and
// moved to T; the palette still finds it by "show".
func TestShowDeletedMovedToT(t *testing.T) {
	if got := keymap["T"]; got != "thread.showDeleted" {
		t.Errorf("keymap[T] = %q, want thread.showDeleted", got)
	}
	if got := keymap["V"]; got != "selection.start" {
		t.Errorf("keymap[V] = %q, want selection.start", got)
	}
	if got := keymap["y"]; got != "selection.yank" {
		t.Errorf("keymap[y] = %q, want selection.yank", got)
	}
	if got := keymap["yr"]; got != "selection.yankRef" {
		t.Errorf("keymap[yr] = %q, want selection.yankRef", got)
	}
	if got := keymap["gy"]; got != "selection.yankRef" {
		t.Errorf("keymap[gy] = %q, want selection.yankRef", got)
	}
}

// TestLineRangePrependingInComments: commenting (c) with an active visual selection
// prepends the line range (e.g. "L12-18: " or "L12: ") to the comment draft.
func TestLineRangePrependingInComments(t *testing.T) {
	m := newModelAt("sample.md", parseDoc([]byte(sampleDoc)), nil)
	m.w, m.h = 100, 60
	m.at = cursor{entry: 1, comment: commentNone} // paragraph at line 3

	pressKey(m, "V")
	pressKey(m, "j")
	if !m.visual {
		t.Fatal("V j did not enter visual mode")
	}

	startLine, endLine := m.selectionLineRange()
	if startLine <= 0 {
		t.Fatalf("selectionLineRange = (%d, %d), want positive line numbers", startLine, endLine)
	}

	wantPrefix := m.selectionLineRef()
	if wantPrefix == "" {
		t.Fatal("selectionLineRef returned empty string")
	}

	pressKey(m, "c")
	if m.visual {
		t.Fatal("c did not leave visual mode")
	}

	a := m.anchorAt()
	tr := m.threads[a]
	if tr == nil {
		t.Fatalf("no thread created for anchor %q", a)
	}

	draft := tr.draft(newCommentSlot)
	if !strings.HasPrefix(draft, wantPrefix) {
		t.Errorf("draft = %q, want prefix %q", draft, wantPrefix)
	}
}

// TestYankReference: yr or gy copies the line reference string (e.g. "document.md:L12-18")
// to the clipboard and clears visual mode.
func TestYankReference(t *testing.T) {
	m := newModelAt("sample.md", parseDoc([]byte(sampleDoc)), nil)
	m.w, m.h = 100, 60
	m.at = cursor{entry: 1, comment: commentNone}

	pressKey(m, "V")
	pressKey(m, "j")

	wantRef := "sample.md:L" + strings.TrimPrefix(m.selectionLineRef(), "L")
	wantRef = strings.TrimSuffix(wantRef, ": ")

	cmd := m.yankRef()
	if cmd == nil {
		t.Fatal("yankRef returned nil command")
	}
	if m.visual {
		t.Fatal("yankRef did not exit visual mode")
	}
	if !strings.Contains(m.status, wantRef) {
		t.Errorf("status = %q, want containing %q", m.status, wantRef)
	}

	// Test gy and yr keymap chords
	m.at = cursor{entry: 1, comment: commentNone}
	pressKey(m, "y")
	pressKey(m, "r")
	if !strings.Contains(m.status, "yanked reference") {
		t.Errorf("status after yr = %q, want containing 'yanked reference'", m.status)
	}

	m.at = cursor{entry: 1, comment: commentNone}
	pressKey(m, "g")
	pressKey(m, "y")
	if !strings.Contains(m.status, "yanked reference") {
		t.Errorf("status after gy = %q, want containing 'yanked reference'", m.status)
	}
}

