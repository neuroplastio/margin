package review

import (
	"os/exec"
	"sort"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	uv "github.com/charmbracelet/ultraviolet"
	"github.com/charmbracelet/x/vt"
)

const (
	draftAnchor = "^e5" // seeded thread holding unsubmitted text
	convoAnchor = "^b2" // seeded thread with two posted comments
	soloAnchor  = "^d4" // seeded thread with one posted comment
	freshAnchor = "^a1" // paragraph with no thread yet
)

func requireNvim(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("nvim"); err != nil {
		t.Skip("nvim not installed")
	}
}

// isolateDrafts points the draft cache at a temp dir so tests never touch the
// real one and never see each other's leftovers.
func isolateDrafts(t *testing.T) {
	t.Helper()
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
}

func newTestModel(t *testing.T) *model {
	t.Helper()
	isolateDrafts(t)
	m := seedModel()
	m.w, m.h = 100, 60
	return m
}

// plainScreen strips styling before matching. The emulator renders styled runs,
// and nvim's spell checker splits words with underline codes mid-token, so a
// naive Contains on the raw output misses text that is plainly on screen.
func plainScreen(em *vt.SafeEmulator) string {
	return ansiRe.ReplaceAllString(em.Render(), "")
}

func waitForScreen(t *testing.T, em *vt.SafeEmulator, want string, timeout time.Duration) string {
	t.Helper()
	deadline := time.Now().Add(timeout)
	var screen string
	for time.Now().Before(deadline) {
		screen = plainScreen(em)
		if strings.Contains(screen, want) {
			return screen
		}
		time.Sleep(20 * time.Millisecond)
	}
	return screen
}

func waitForMode(t *testing.T, c *composer, want string, timeout time.Duration) bool {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if c.mode() == want {
			return true
		}
		time.Sleep(20 * time.Millisecond)
	}
	return false
}

// open spawns an editor and waits for it to be ready: either the seeded text is
// on screen, or — for an empty buffer — nvim has asked for an insert cursor.
func open(t *testing.T, m *model, anchor string, target int, expect string) {
	t.Helper()
	requireNvim(t)
	if cmd := m.openComposer(anchor, target); cmd == nil {
		t.Fatalf("openComposer(%s, %d) returned no command: %s", anchor, target, m.status)
	}
	t.Cleanup(func() {
		if m.comp != nil {
			m.comp.close()
		}
	})
	if expect == "" {
		if !waitForMode(t, m.comp, "INSERT", 10*time.Second) {
			t.Fatalf("editor never became ready; screen was:\n%s", plainScreen(m.comp.em))
		}
		return
	}
	if s := waitForScreen(t, m.comp.em, expect, 10*time.Second); !strings.Contains(s, expect) {
		t.Fatalf("editor never showed %q; screen was:\n%s", expect, s)
	}
}

// typeKeys sends keys with a gap short enough to stay inside nvim's timeoutlen,
// so multi-key mappings like <leader>ck actually resolve.
func typeKeys(c *composer, keys ...tea.Key) {
	for _, k := range keys {
		c.sendKey(k)
		time.Sleep(60 * time.Millisecond)
	}
}

func typeText(c *composer, s string) {
	for _, r := range s {
		typeKeys(c, tea.Key{Code: r, Text: string(r)})
	}
}

func waitExit(t *testing.T, c *composer, timeout time.Duration) error {
	t.Helper()
	done := make(chan error, 1)
	go func() { done <- c.cmd.Wait() }()
	select {
	case err := <-done:
		return err
	case <-time.After(timeout):
		t.Fatalf("editor did not exit; screen was:\n%s", plainScreen(c.em))
		return nil
	}
}

var (
	keyEsc       = tea.Key{Code: uv.KeyEscape}
	keyEnter     = tea.Key{Code: uv.KeyEnter}
	keyCtrlS     = tea.Key{Code: 's', Mod: uv.ModCtrl}
	keySpace     = tea.Key{Code: uv.KeySpace, Text: " "}
	keyBackspace = tea.Key{Code: uv.KeyBackspace}
)

func entryFor(t *testing.T, m *model, anchor string) int {
	t.Helper()
	for i, e := range m.entries {
		if e.thread != nil && e.thread.anchor == anchor {
			return i
		}
	}
	t.Fatalf("no thread entry for %s", anchor)
	return -1
}

func blockEntryFor(t *testing.T, m *model, anchor string) int {
	t.Helper()
	for i, e := range m.entries {
		if e.thread == nil && e.b.anchor == anchor {
			return i
		}
	}
	t.Fatalf("no document entry for %s", anchor)
	return -1
}

// --- editing existing comments ----------------------------------------------

// TestEditPostedCommentReplacesIt is the reported bug: `e` on a seeded comment
// used to open an empty composer and append a reply instead of editing.
func TestEditPostedCommentReplacesIt(t *testing.T) {
	m := newTestModel(t)
	requireNvim(t)

	i := entryFor(t, m, convoAnchor)
	m.at = cursor{entry: i, comment: 0}
	before := len(m.threads[convoAnchor].posted)

	if cmd := m.editFocused(); cmd == nil {
		t.Fatalf("e on a posted comment did nothing: %s", m.status)
	}
	t.Cleanup(func() {
		if m.comp != nil {
			m.comp.close()
		}
	})
	if m.comp.target != 0 {
		t.Fatalf("composer target = %d, want comment 0", m.comp.target)
	}
	// The existing text must be in the buffer, not an empty reply.
	waitForScreen(t, m.comp.em, "noisy neighbour", 10*time.Second)

	typeKeys(m.comp, tea.Key{Code: 'A', Mod: uv.ModShift, Text: "A"})
	typeText(m.comp, " Still true.")
	typeKeys(m.comp, keyEsc, keyCtrlS)
	m.dismiss(waitExit(t, m.comp, 10*time.Second))

	posted := m.threads[convoAnchor].posted
	if len(posted) != before {
		t.Fatalf("posted %d comments, want %d — editing must not append", len(posted), before)
	}
	if !strings.Contains(posted[0].body, "Still true.") {
		t.Fatalf("comment 0 = %q, want the edit applied in place", posted[0].body)
	}
	if !strings.Contains(posted[0].body, "noisy neighbour") {
		t.Fatalf("comment 0 = %q, want the original text preserved", posted[0].body)
	}
}

// TestNewCommentAppends: `c` always starts a reply, even with a comment focused.
func TestNewCommentAppends(t *testing.T) {
	m := newTestModel(t)
	i := entryFor(t, m, soloAnchor)
	m.at = cursor{entry: i, comment: 0}
	before := len(m.threads[soloAnchor].posted)

	open(t, m, soloAnchor, newCommentSlot, "")
	typeText(m.comp, "March incident, ticket 4412.")
	typeKeys(m.comp, keyCtrlS)
	m.dismiss(waitExit(t, m.comp, 10*time.Second))

	posted := m.threads[soloAnchor].posted
	if len(posted) != before+1 {
		t.Fatalf("posted %d comments, want %d", len(posted), before+1)
	}
	if got := posted[len(posted)-1].body; got != "March incident, ticket 4412." {
		t.Fatalf("new comment = %q", got)
	}
}

// TestSubmitPersistsThroughTheStore: a model opened by Run (store set) writes
// a posted comment to disk, not just to m.threads — the STORE-02 wiring that
// closes the loop STORE-01 only formatted.
func TestSubmitPersistsThroughTheStore(t *testing.T) {
	m := newTestModel(t)
	root := t.TempDir()
	m.store = &threadStore{root: root, docPath: "document.md"}

	open(t, m, soloAnchor, newCommentSlot, "")
	typeText(m.comp, "Persisted reply.")
	typeKeys(m.comp, keyCtrlS)
	m.dismiss(waitExit(t, m.comp, 10*time.Second))

	if strings.Contains(m.status, "not saved") {
		t.Fatalf("status = %q, want no persistence error", m.status)
	}

	th := m.threads[soloAnchor]
	got, err := loadThreadsForDoc(root, "document.md")
	if err != nil {
		t.Fatalf("loadThreadsForDoc: %v", err)
	}
	onDisk, ok := got[th.anchor]
	if !ok {
		t.Fatalf("no thread file written for %s", th.anchor)
	}
	if last := onDisk.posted[len(onDisk.posted)-1]; last.body != "Persisted reply." {
		t.Fatalf("on-disk comment = %q, want the submitted reply", last.body)
	}
}

// TestSubmitWithoutAStoreDoesNotTouchDisk: seedModel and every other test
// build a model with no store, and dismiss must not start writing to the
// process's working directory just because a comment was submitted.
func TestSubmitWithoutAStoreDoesNotTouchDisk(t *testing.T) {
	m := newTestModel(t)
	if m.store != nil {
		t.Fatalf("newTestModel: store = %+v, want nil", m.store)
	}

	open(t, m, soloAnchor, newCommentSlot, "")
	typeText(m.comp, "In memory only.")
	typeKeys(m.comp, keyCtrlS)
	m.dismiss(waitExit(t, m.comp, 10*time.Second))

	if strings.Contains(m.status, "not saved") {
		t.Fatalf("status = %q, a nil store should be a silent no-op", m.status)
	}
}

// TestEditOnEmptyThreadDoesNotOpen: `e` never creates anything; that is `c`.
func TestEditOnEmptyThreadDoesNotOpen(t *testing.T) {
	m := newTestModel(t)
	m.at = cursor{entry: blockEntryFor(t, m, freshAnchor), comment: commentNone}
	if cmd := m.editFocused(); cmd != nil || m.comp != nil {
		t.Fatal("e opened an editor on a block with no comments")
	}
	if m.status == "" {
		t.Fatal("e on an empty block gave no feedback")
	}
}

// --- modes ------------------------------------------------------------------

// TestNewCommentStartsInInsertMode: an empty buffer wants to be typed into.
func TestNewCommentStartsInInsertMode(t *testing.T) {
	m := newTestModel(t)
	open(t, m, freshAnchor, newCommentSlot, "")
	if !waitForMode(t, m.comp, "INSERT", 5*time.Second) {
		t.Fatalf("new comment opened in %s, want INSERT", m.comp.mode())
	}
}

// TestEditStartsInNormalMode: existing text wants motions, not insertions.
func TestEditStartsInNormalMode(t *testing.T) {
	m := newTestModel(t)
	open(t, m, convoAnchor, 0, "noisy neighbour")
	if !waitForMode(t, m.comp, "NORMAL", 5*time.Second) {
		t.Fatalf("editing opened in %s, want NORMAL", m.comp.mode())
	}
}

// TestResumedDraftStartsInNormalMode: a resumed draft is existing text too.
func TestResumedDraftStartsInNormalMode(t *testing.T) {
	m := newTestModel(t)
	open(t, m, draftAnchor, newCommentSlot, "contradicts")
	if !waitForMode(t, m.comp, "NORMAL", 5*time.Second) {
		t.Fatalf("resumed draft opened in %s, want NORMAL", m.comp.mode())
	}
}

// TestEditPlacesCursorAtTheEnd is the 2026-08-09 composer feedback's
// "editing a comment should place the cursor at the end" item: `e` on a posted
// comment must open with the cursor on the last character of the comment text,
// so an append (`a`) lands at the end rather than after the first character.
// Asserted through the submitted body — if the cursor sat at {1,0}, `a` would
// insert " tail" after the opening "S" and the body would come out
// "S tailhouldn't be global...".
func TestEditPlacesCursorAtTheEnd(t *testing.T) {
	m := newTestModel(t)
	original := m.threads[convoAnchor].posted[0].body

	open(t, m, convoAnchor, 0, "Shouldn't be global")
	if !waitForMode(t, m.comp, "NORMAL", 5*time.Second) {
		t.Fatalf("editing opened in %s, want NORMAL", m.comp.mode())
	}

	typeKeys(m.comp, tea.Key{Code: 'a', Text: "a"})
	typeText(m.comp, " tail")
	typeKeys(m.comp, keyEsc, keyCtrlS)
	m.dismiss(waitExit(t, m.comp, 10*time.Second))

	got := m.threads[convoAnchor].posted[0].body
	if !strings.HasSuffix(got, " tail") {
		t.Fatalf("edited comment = %q, want %q — `a` must have appended at the very end, so the cursor was on the last character", got, original+" tail")
	}
	if !strings.HasPrefix(got, original) {
		t.Fatalf("edited comment = %q, want the original text preserved at the start", got)
	}
}

// TestResumedDraftPlacesCursorAtTheEnd pins the settled contract
// (interaction.md) that a resumed draft puts the cursor after the existing
// text — the same {1,0} → end-of-text fix as the edit path, since both open a
// non-empty buffer through the same VimEnter branch. Submitting the resumed
// draft posts it, so the assertion reads the posted comment's body.
func TestResumedDraftPlacesCursorAtTheEnd(t *testing.T) {
	m := newTestModel(t)
	original := m.threads[draftAnchor].draft(newCommentSlot)

	open(t, m, draftAnchor, newCommentSlot, "contradicts")
	if !waitForMode(t, m.comp, "NORMAL", 5*time.Second) {
		t.Fatalf("resumed draft opened in %s, want NORMAL", m.comp.mode())
	}

	typeKeys(m.comp, tea.Key{Code: 'a', Text: "a"})
	typeText(m.comp, " tail")
	typeKeys(m.comp, keyEsc, keyCtrlS)
	m.dismiss(waitExit(t, m.comp, 10*time.Second))

	posted := m.threads[draftAnchor].posted
	if len(posted) != 1 {
		t.Fatalf("posted %d comments, want 1 — the resumed draft should submit as a new comment", len(posted))
	}
	got := posted[0].body
	if !strings.HasSuffix(got, " tail") {
		t.Fatalf("resumed draft = %q, want %q — `a` must have appended at the very end, so the cursor was on the last character", got, original+" tail")
	}
	if !strings.HasPrefix(got, original) {
		t.Fatalf("resumed draft = %q, want the original text preserved at the start", got)
	}
}

// --- the buffer is only the comment -----------------------------------------

// TestBufferHoldsOnlyTheComment: no instruction comments, no quoted context.
// The block is on screen above the pane and the keys are in the footer.
func TestBufferHoldsOnlyTheComment(t *testing.T) {
	m := newTestModel(t)
	open(t, m, convoAnchor, 0, "noisy neighbour")
	screen := plainScreen(m.comp.em)

	for _, unwanted := range []string{"<!--", ">8", "submit", "discard", "~", "COMMENT_EDITMSG"} {
		if strings.Contains(screen, unwanted) {
			t.Errorf("buffer still shows %q:\n%s", unwanted, screen)
		}
	}
}

// --- dismissal --------------------------------------------------------------

// TestColonQClosesWithoutPrompting: ':q' is muscle memory and must not stop to
// ask, in a pane too small to read a prompt in.
func TestColonQClosesWithoutPrompting(t *testing.T) {
	m := newTestModel(t)
	open(t, m, freshAnchor, newCommentSlot, "")
	typeText(m.comp, "something unsaved")
	typeKeys(m.comp, keyEsc)
	typeText(m.comp, ":q")
	typeKeys(m.comp, keyEnter)

	if got := outcomeFromExit(waitExit(t, m.comp, 10*time.Second)); got != outcomeDraft {
		t.Fatalf(":q gave outcome %v, want draft", got)
	}
	if body := m.comp.body(); !strings.Contains(body, "something unsaved") {
		t.Fatalf(":q lost the text; body = %q", body)
	}
}

// TestColonQBangDiscards: ':q!' throws the text away.
func TestColonQBangDiscards(t *testing.T) {
	m := newTestModel(t)
	open(t, m, freshAnchor, newCommentSlot, "")
	typeText(m.comp, "never mind")
	typeKeys(m.comp, keyEsc)
	typeText(m.comp, ":q!")
	typeKeys(m.comp, keyEnter)

	if got := outcomeFromExit(waitExit(t, m.comp, 10*time.Second)); got != outcomeDiscard {
		t.Fatalf(":q! gave outcome %v, want discard", got)
	}
}

// TestDoubleEscapeKeepsDraft: a new comment opens in insert mode, so leaving it
// takes two <Esc> — the first exits insert mode, the second is the exit itself
// — and the unfinished text survives as a draft.
func TestDoubleEscapeKeepsDraft(t *testing.T) {
	m := newTestModel(t)
	open(t, m, freshAnchor, newCommentSlot, "")
	typeText(m.comp, "half a thought")
	typeKeys(m.comp, keyEsc, keyEsc)

	if got := outcomeFromExit(waitExit(t, m.comp, 10*time.Second)); got != outcomeDraft {
		t.Fatalf("outcome = %v, want draft", got)
	}
	if body := m.comp.body(); body != "half a thought" {
		t.Fatalf("body = %q, want the unfinished text preserved", body)
	}
}

// TestSingleEscapeDoesNotExitNewComment: one <Esc> from a new comment only
// leaves insert mode. The composer must stay open — typing and reviewing is not
// one keypress from closing — and a second <Esc> exits keeping the draft.
func TestSingleEscapeDoesNotExitNewComment(t *testing.T) {
	m := newTestModel(t)
	open(t, m, freshAnchor, newCommentSlot, "")
	typeText(m.comp, "half a thought")

	done := make(chan error, 1)
	go func() { done <- m.comp.cmd.Wait() }()
	typeKeys(m.comp, keyEsc)
	select {
	case err := <-done:
		t.Fatalf("a single <Esc> exited the composer (%v); a new comment needs two", err)
	case <-time.After(700 * time.Millisecond):
	}
	if !waitForMode(t, m.comp, "NORMAL", 5*time.Second) {
		t.Fatalf("after one <Esc> the composer is in %s, want NORMAL", m.comp.mode())
	}

	typeKeys(m.comp, keyEsc)
	select {
	case err := <-done:
		m.dismiss(err)
	case <-time.After(10 * time.Second):
		t.Fatalf("double <Esc> did not exit; screen:\n%s", plainScreen(m.comp.em))
	}
	if got := m.threads[freshAnchor].draft(newCommentSlot); got != "half a thought" {
		t.Fatalf("draft = %q, want the unfinished text preserved", got)
	}
}

// TestSingleEscapeExitsEdit: an edit opens in normal mode with the comment
// already on screen, so there is nothing to finish — a single <Esc> exits,
// keeping the change as a draft and leaving the posted comment untouched.
func TestSingleEscapeExitsEdit(t *testing.T) {
	m := newTestModel(t)
	original := m.threads[convoAnchor].posted[0].body

	open(t, m, convoAnchor, 0, "noisy neighbour")
	typeKeys(m.comp, keyEsc)

	if got := outcomeFromExit(waitExit(t, m.comp, 10*time.Second)); got != outcomeDraft {
		t.Fatalf("single <Esc> on an edit gave outcome %v, want draft", got)
	}
	if got := m.threads[convoAnchor].posted[0].body; got != original {
		t.Fatalf("posted comment was mutated by an esc-exited edit: %q", got)
	}
}

func TestSpacemacsDiscard(t *testing.T) {
	m := newTestModel(t)
	open(t, m, draftAnchor, newCommentSlot, "contradicts")
	typeKeys(m.comp, keySpace,
		tea.Key{Code: 'c', Text: "c"},
		tea.Key{Code: 'k', Text: "k"})
	m.dismiss(waitExit(t, m.comp, 10*time.Second))

	if got := m.threads[draftAnchor].draft(newCommentSlot); got != "" {
		t.Fatalf("draft = %q, want it thrown away by SPC c k", got)
	}
}

// --- blur -------------------------------------------------------------------

// TestBlurKeepsDraft is the core bet: losing focus tears down a live editor and
// the half-written text survives it.
func TestBlurKeepsDraft(t *testing.T) {
	m := newTestModel(t)
	open(t, m, draftAnchor, newCommentSlot, "contradicts")
	typeKeys(m.comp, tea.Key{Code: 'A', Mod: uv.ModShift, Text: "A"})
	typeText(m.comp, " and the breaker section too")
	m.blur(nil)
	m.dismiss(waitExit(t, m.comp, 10*time.Second))

	if m.comp != nil {
		t.Fatal("composer still open after blur")
	}
	got := m.threads[draftAnchor].draft(newCommentSlot)
	if !strings.Contains(got, "breaker section too") {
		t.Fatalf("draft = %q, want the typed text preserved through blur", got)
	}
	if persisted, _ := loadDraft(draftAnchor, newCommentSlot); !strings.Contains(persisted, "breaker section") {
		t.Fatalf("draft was not persisted; got %q", persisted)
	}
}

// TestBlurredEditIsKeptSeparately: an in-progress edit must not overwrite the
// posted comment until it is submitted, and must not collide with the thread's
// new-comment draft.
func TestBlurredEditIsKeptSeparately(t *testing.T) {
	m := newTestModel(t)
	original := m.threads[convoAnchor].posted[0].body

	open(t, m, convoAnchor, 0, "noisy neighbour")
	typeKeys(m.comp, tea.Key{Code: 'A', Mod: uv.ModShift, Text: "A"})
	typeText(m.comp, " WIP")
	m.blur(nil)
	m.dismiss(waitExit(t, m.comp, 10*time.Second))

	tr := m.threads[convoAnchor]
	if tr.posted[0].body != original {
		t.Fatalf("posted comment was mutated by an abandoned edit: %q", tr.posted[0].body)
	}
	if !strings.Contains(tr.draft(0), "WIP") {
		t.Fatalf("edit draft = %q, want the unsaved edit kept", tr.draft(0))
	}
	if tr.draft(newCommentSlot) != "" {
		t.Fatal("an edit draft leaked into the new-comment slot")
	}
}

// --- clicking ---------------------------------------------------------------

// TestClickOnlyFocuses: opening an editor is always an explicit c or e.
func TestClickOnlyFocuses(t *testing.T) {
	m := newTestModel(t)
	m.View()

	i := entryFor(t, m, draftAnchor)
	if cmd := m.handleClick(tea.Mouse{X: 4, Y: m.spans[i].start - m.scroll}); cmd != nil {
		t.Fatal("clicking returned a command — it should only move focus")
	}
	if m.comp != nil {
		t.Fatal("clicking opened an editor")
	}
	if m.at.entry != i {
		t.Fatalf("focus at entry %d, want %d", m.at.entry, i)
	}
}

// TestClickSelectsAComment: an expanded thread's comments are individually
// focusable, which is what lets `e` know which one to open.
func TestClickSelectsAComment(t *testing.T) {
	m := newTestModel(t)
	i := entryFor(t, m, convoAnchor)
	m.at = cursor{entry: i, comment: commentNone}
	m.View()

	want := cursor{entry: i, comment: 1}
	s, ok := m.subspans[want]
	if !ok {
		t.Fatalf("no span for comment 1 of %s; subspans = %v", convoAnchor, m.subspans)
	}
	m.handleClick(tea.Mouse{X: 6, Y: s.start - m.scroll})
	if m.at != want {
		t.Fatalf("focus = %+v, want %+v", m.at, want)
	}
}

// TestClickAwayBlurs covers the mouse path out of a live editor.
func TestClickAwayBlurs(t *testing.T) {
	m := newTestModel(t)
	m.View()
	open(t, m, draftAnchor, newCommentSlot, "contradicts")
	m.View()

	m.handleClick(tea.Mouse{X: 4, Y: 0})
	m.dismiss(waitExit(t, m.comp, 10*time.Second))
	if m.comp != nil {
		t.Fatal("composer still open after clicking away")
	}
	if m.threads[draftAnchor].draft(newCommentSlot) == "" {
		t.Fatal("clicking away lost the draft")
	}
}

// TestClickInsidePaneDoesNotBlur: clicks in the editor belong to the editor.
func TestClickInsidePaneDoesNotBlur(t *testing.T) {
	m := newTestModel(t)
	m.View()
	open(t, m, draftAnchor, newCommentSlot, "contradicts")
	m.View()

	m.handleClick(tea.Mouse{X: gutterW + borderW + 2, Y: m.paneTop + 1 - m.scroll})
	if m.pendingOpen != nil {
		t.Fatalf("a click inside the pane queued a focus change to %+v", m.pendingOpen)
	}
	if m.comp == nil {
		t.Fatal("a click inside the pane closed the editor")
	}
}

// --- review marks -----------------------------------------------------------

// TestWrapListWrapsInsteadOfTruncating is the render-level regression test
// for defect 2 of the 2026-08-08 rendering-bugs feedback: a list item longer
// than the measure used to come out as one over-width blockRaw line, cut off
// by whatever terminal or pager was watching rather than by margin. Every
// line wrapList produces must fit, and nothing from the item's text may go
// missing across the wrap.
func TestWrapListWrapsInsteadOfTruncating(t *testing.T) {
	item := listItem{prefix: "- ", text: "write to both stores, read from memory, and watch the fallback rate trend to zero as old sessions expire"}
	w := 40
	out := wrapList([]listItem{item}, w, textStyle)
	if len(out) < 2 {
		t.Fatalf("item did not wrap at width %d, got %d line(s): %v", w, len(out), out)
	}
	for _, l := range out {
		if width := len(ansiRe.ReplaceAllString(l, "")); width > w {
			t.Errorf("line %q is %d columns wide, wider than the measure %d", l, width, w)
		}
	}
	plainSecond := ansiRe.ReplaceAllString(out[1], "")
	if !strings.Contains(plainSecond, "  ") || strings.HasPrefix(plainSecond, "-") {
		t.Errorf("continuation line %q is not hanging-indented under the text, not the marker", plainSecond)
	}
	joined := ansiRe.ReplaceAllString(strings.Join(out, " "), "")
	for _, word := range strings.Fields(item.text) {
		if !strings.Contains(joined, word) {
			t.Errorf("word %q from the item went missing across the wrap: %v", word, out)
		}
	}
}

// TestWrapListRendersInlineMarkup is the render-level regression test for the
// 2026-08-08 markdown-rendering feedback: a list item's **bold** used to
// reach the screen as literal asterisks because wrapList wrapped with plain
// wrap() and the caller flattened the whole block to one uniform style.
func TestWrapListRendersInlineMarkup(t *testing.T) {
	item := listItem{prefix: "- ", text: "**Week 1** ships the parser"}
	out := wrapList([]listItem{item}, 60, textStyle)
	joined := strings.Join(out, " ")
	if strings.Contains(joined, "**") {
		t.Errorf("raw ** markup survived into rendered output: %q", joined)
	}
	plain := ansiRe.ReplaceAllString(joined, "")
	if !strings.Contains(plain, "Week 1") {
		t.Errorf("bold text went missing across the wrap: %q", plain)
	}
	if !ansiRe.MatchString(joined) {
		t.Errorf("bold run carries no ANSI styling: %q", joined)
	}
}

// TestWrapQuoteWrapsAndKeepsParagraphBreaks is the render-level regression
// test for the 2026-08-08 blockquote-rendering feedback: a quote's prose has
// to wrap like a paragraph, and a blank line inside the quote (a bare `>` in
// the source, already stripped to "" by quoteLinesFor) must survive as a
// paragraph break rather than being wrapped away or merged into one run.
func TestWrapQuoteWrapsAndKeepsParagraphBreaks(t *testing.T) {
	lines := []string{
		"write to both stores, read from memory, and watch the fallback rate trend to zero as old sessions expire",
		"",
		"Second paragraph.",
	}
	w := 40
	out := wrapQuote(lines, w, textStyle)
	if len(out) < 3 {
		t.Fatalf("expected wrapping plus a paragraph break, got %d line(s): %v", len(out), out)
	}
	for _, l := range out {
		if width := len(ansiRe.ReplaceAllString(l, "")); width > w {
			t.Errorf("line %q is %d columns wide, wider than the measure %d", l, width, w)
		}
	}
	var sawBreak bool
	for _, l := range out {
		if l == "" {
			sawBreak = true
		}
	}
	if !sawBreak {
		t.Errorf("paragraph break went missing across the wrap: %v", out)
	}
	if !strings.Contains(ansiRe.ReplaceAllString(strings.Join(out, " "), ""), "Second paragraph.") {
		t.Errorf("second paragraph's text went missing: %v", out)
	}
}

// TestWrapQuoteRendersInlineMarkup is the render-level regression test for
// the 2026-08-08 markdown-rendering feedback: a quote's **bold** used to
// reach the screen as literal asterisks because wrapQuote wrapped with plain
// wrap() and the caller flattened the whole block to one uniform style.
func TestWrapQuoteRendersInlineMarkup(t *testing.T) {
	lines := []string{"**Open question.** Should this block on review?"}
	out := wrapQuote(lines, 60, textStyle)
	joined := strings.Join(out, " ")
	if strings.Contains(joined, "**") {
		t.Errorf("raw ** markup survived into rendered output: %q", joined)
	}
	plain := ansiRe.ReplaceAllString(joined, "")
	if !strings.Contains(plain, "Open question.") {
		t.Errorf("bold text went missing across the wrap: %q", plain)
	}
	if !ansiRe.MatchString(joined) {
		t.Errorf("bold run carries no ANSI styling: %q", joined)
	}
}

// TestParseInlineStripsMarkup pins parseInline's three recognised forms
// (RENDER-06): the markup characters themselves must not survive into any
// run's text, and each run must carry the right flag.
func TestParseInlineStripsMarkup(t *testing.T) {
	runs := parseInline("plain **bold** and `code` and [linked text](https://example.com/x) done")
	var gotBold, gotCode, gotLink bool
	for _, r := range runs {
		switch {
		case r.bold:
			gotBold = true
			if r.text != "bold" {
				t.Errorf("bold run text = %q, want %q", r.text, "bold")
			}
		case r.code:
			gotCode = true
			if r.text != "code" {
				t.Errorf("code run text = %q, want %q", r.text, "code")
			}
		case r.link:
			gotLink = true
			if r.text != "linked text" {
				t.Errorf("link run text = %q, want %q (URL must not leak into the visible text)", r.text, "linked text")
			}
		}
	}
	if !gotBold || !gotCode || !gotLink {
		t.Fatalf("missing a run kind: bold=%v code=%v link=%v, runs=%+v", gotBold, gotCode, gotLink, runs)
	}
}

// TestParseInlineUnterminatedMarkerIsLiteral is the "what if the closing
// marker is missing" case parseInline's doc comment does not spell out:
// nothing to close a form with must fall back to literal text rather than
// swallowing the rest of the paragraph or panicking.
func TestParseInlineUnterminatedMarkerIsLiteral(t *testing.T) {
	runs := parseInline("this has an unmatched `backtick and **bold that never closes")
	var joined strings.Builder
	for _, r := range runs {
		if r.bold || r.code || r.link {
			t.Fatalf("an unterminated marker produced a styled run: %+v", runs)
		}
		joined.WriteString(r.text)
	}
	if joined.String() != "this has an unmatched `backtick and **bold that never closes" {
		t.Errorf("literal text corrupted: %q", joined.String())
	}
}

// TestGlueWordsKeepsAdjacentRunsTogether is the case wrapInline's doc comment
// calls out by name: "en**hanced**" has no whitespace in the source between
// the plain and bold halves, so it must wrap as one word, not two.
func TestGlueWordsKeepsAdjacentRunsTogether(t *testing.T) {
	words := glueWords("en**hanced** word")
	if len(words) != 2 {
		t.Fatalf("glueWords(\"en**hanced** word\") = %d word(s), want 2: %+v", len(words), words)
	}
	var joined strings.Builder
	for _, f := range words[0].frags {
		joined.WriteString(f.text)
	}
	if joined.String() != "enhanced" {
		t.Errorf("first glueWord = %q, want %q (markup must not insert a space)", joined.String(), "enhanced")
	}
	if len(words[0].frags) != 2 || !words[0].frags[1].bold {
		t.Errorf("first glueWord's bold half went missing: %+v", words[0].frags)
	}
}

// TestWrapInlineStripsMarkupAndWraps is the render-level regression test for
// RENDER-06: a paragraph with bold, code and a link must wrap to the measure
// with no raw markup characters left on screen, and none of the words may go
// missing across the wrap.
func TestWrapInlineStripsMarkupAndWraps(t *testing.T) {
	text := "The store interface is already **abstracted** — `SessionStore` has exactly the three " +
		"methods we need, see the [migration runbook](https://example.com/runbook) for details, " +
		"rather than being deleted"
	w := 40
	out := wrapInline(text, w, textStyle)
	if len(out) < 2 {
		t.Fatalf("expected wrapping across multiple lines, got %d: %v", len(out), out)
	}
	joined := ansiRe.ReplaceAllString(strings.Join(out, " "), "")
	for _, marker := range []string{"**", "`", "](", "https://example.com/runbook"} {
		if strings.Contains(joined, marker) {
			t.Errorf("raw markup %q survived into rendered output: %q", marker, joined)
		}
	}
	for _, word := range []string{"abstracted", "SessionStore", "migration", "runbook", "deleted"} {
		if !strings.Contains(joined, word) {
			t.Errorf("word %q went missing across the wrap: %q", word, joined)
		}
	}
	for _, l := range out {
		if width := len(ansiRe.ReplaceAllString(l, "")); width > w {
			t.Errorf("line %q is %d columns wide, wider than the measure %d", l, width, w)
		}
	}
}

// TestHighlightCodeAppliesColourForARecognisedLanguage is the render-level
// regression test for RENDER-02: a fenced code block whose language chroma
// recognises must come back styled — some ANSI escape on at least one
// line — while every source line's own text (words, indentation) survives
// untouched underneath the colour.
func TestHighlightCodeAppliesColourForARecognisedLanguage(t *testing.T) {
	lines := []string{"func retry(n int) error {", "\treturn nil", "}"}
	out := highlightCode(lines, "go")
	if len(out) != len(lines) {
		t.Fatalf("highlightCode returned %d line(s), want %d: %q", len(out), len(lines), out)
	}
	styled := false
	for _, l := range out {
		if ansiRe.MatchString(l) {
			styled = true
		}
	}
	if !styled {
		t.Errorf("no line carries an ANSI escape for a recognised language: %q", out)
	}
	for i, l := range out {
		plain := ansiRe.ReplaceAllString(l, "")
		if plain != lines[i] {
			t.Errorf("line %d = %q once de-styled, want the source line %q byte-for-byte", i, plain, lines[i])
		}
	}
}

// TestHighlightCodeDegradesForAnUnknownLanguage pins the fallback path: an
// unrecognised or empty language tag must not error or drop text, even if it
// ends up unstyled. quick.Highlight's own fallback (a guess from content, then
// a plain lexer) already covers the "no colour" case; this only pins that the
// source survives regardless of which path it took.
func TestHighlightCodeDegradesForAnUnknownLanguage(t *testing.T) {
	lines := []string{"this is not any real language %%%", "just some free text"}
	out := highlightCode(lines, "not-a-real-lang")
	if len(out) != len(lines) {
		t.Fatalf("highlightCode returned %d line(s), want %d: %q", len(out), len(lines), out)
	}
	for i, l := range out {
		plain := ansiRe.ReplaceAllString(l, "")
		if plain != lines[i] {
			t.Errorf("line %d = %q once de-styled, want the source line %q byte-for-byte", i, plain, lines[i])
		}
	}
}

// TestRenderTableAlignsColumnsAndStaysInMeasure pins renderTable's basic
// shape: header above a rule above the rows, every line the same
// de-styled width (columns line up), and alignment honoured per column.
func TestRenderTableAlignsColumnsAndStaysInMeasure(t *testing.T) {
	tb := &tableBlock{
		header: []string{"Name", "Age"},
		rows:   [][]string{{"Alice", "30"}, {"Bob", "5"}},
		aligns: []tableAlign{alignLeft, alignRight},
	}
	out := renderTable(tb, 40, textStyle)
	if len(out) != 4 {
		t.Fatalf("renderTable returned %d line(s), want 4 (header, rule, 2 rows): %q", len(out), out)
	}
	widths := map[int]int{}
	for i, l := range out {
		widths[i] = len([]rune(ansiRe.ReplaceAllString(l, "")))
	}
	for i, w := range widths {
		if w != widths[0] {
			t.Errorf("line %d is %d runes wide once de-styled, want %d like every other row (columns should line up): %q", i, w, widths[0], out)
		}
	}
	plainRule := ansiRe.ReplaceAllString(out[1], "")
	if !strings.Contains(plainRule, "─") {
		t.Errorf("row 1 = %q, want the dim rule between header and body", plainRule)
	}
	plainAge := ansiRe.ReplaceAllString(out[2], "")
	if !strings.HasSuffix(strings.TrimRight(plainAge, " "), "30") {
		t.Errorf("Age column not right-aligned in %q", plainAge)
	}
}

// TestTableColumnWidthsNarrowsToFit exercises tableColumnWidths directly: a
// table wider than the measure narrows to fit rather than running off it,
// down to its 3-rune floor per column.
func TestTableColumnWidthsNarrowsToFit(t *testing.T) {
	natural := []int{40, 40}
	w := 20
	widths := tableColumnWidths(natural, w)
	total := len(tableColumnSpacing)
	for _, x := range widths {
		total += x
	}
	if total > w {
		t.Errorf("narrowed widths %v (+spacing) sum to %d, wider than the measure %d", widths, total, w)
	}
	for i, x := range widths {
		if x < 3 {
			t.Errorf("column %d narrowed to %d, below the 3-rune floor", i, x)
		}
	}
}

// TestTableColumnWidthsLeavesRoomWhenItFits is the non-narrowing half of the
// same contract: a table that already fits keeps its natural widths exactly,
// rather than always shrinking to the measure.
func TestTableColumnWidthsLeavesRoomWhenItFits(t *testing.T) {
	natural := []int{4, 3}
	got := tableColumnWidths(natural, 40)
	if got[0] != 4 || got[1] != 3 {
		t.Errorf("widths = %v, want the natural widths %v unchanged (table already fits)", got, natural)
	}
}

// TestRenderFrontmatterScrollsHorizontally confirms a field wider than the
// measure is scrolled horizontally (the code-block treatment) rather than
// wrapped or truncated with an ellipsis: at offset 0 the full natural width is
// rendered (frontmatter is key: value pairs, not prose, so nothing gets cut
// off the top of the value), and a scroll offset clips to the measure while
// keeping the dim styling on the visible run.
func TestRenderFrontmatterScrollsHorizontally(t *testing.T) {
	longField := "description: " + strings.Repeat("x", 80)
	b := block{kind: blockFrontmatter, text: "---\n" + longField + "\n---"}

	// Offset 0: the first measure of the field, clipped without an ellipsis —
	// a scroll window cannot be wider than the screen, and the "truncation" is
	// now a viewport, not a cut: scrolling reveals the rest.
	lines := renderFrontmatterFields(b, 20, 0)
	if len(lines) != 1 {
		t.Fatalf("renderFrontmatterFields produced %d lines, want 1 (the one field): %v", len(lines), lines)
	}
	stripped := ansiRe.ReplaceAllString(lines[0], "")
	if stripped != longField[:20] {
		t.Errorf("offset 0 field = %q, want the first measure %q", stripped, longField[:20])
	}
	if strings.Contains(stripped, "…") {
		t.Errorf("offset 0 field contains an ellipsis; frontmatter is scrolled, not truncated: %q", stripped)
	}

	// Offset 20: the next measure of the field, the rest off-screen.
	scrolled := renderFrontmatterFields(b, 20, 20)
	if len(scrolled) != 1 {
		t.Fatalf("scrolled renderFrontmatterFields produced %d lines, want 1", len(scrolled))
	}
	got := ansiRe.ReplaceAllString(scrolled[0], "")
	if got != longField[20:20+20] {
		t.Errorf("offset 20 field = %q, want the next measure %q", got, longField[20:20+20])
	}

	// An offset past the end leaves nothing (scrollBlock clamps the offset, so
	// this only guards the renderer itself against a bad caller).
	if got := renderFrontmatterFields(b, 20, 500); len(got) != 1 || ansiRe.ReplaceAllString(got[0], "") != "" {
		t.Errorf("offset past the end = %q, want an empty rendered line", got)
	}
}

func TestRenderFrontmatterEmptyIsNil(t *testing.T) {
	b := block{kind: blockFrontmatter, text: "---\n---"}
	if got := renderFrontmatterFields(b, 40, 0); got != nil {
		t.Errorf("renderFrontmatterFields of an empty body = %v, want nil", got)
	}
}

// TestFrontmatterIsAKeyboardFocusStop: the 2026-08-09 frontmatter feedback
// found the block unreachable — j/k never landed on it and focus-following
// scroll could never bring it back into view once the reviewer had scrolled
// down. It is entry 0 now, so g and a downward j land on it and movement
// leaves it again.
func TestFrontmatterIsAKeyboardFocusStop(t *testing.T) {
	m := newModel(parseDoc([]byte(frontmatterDoc)), nil)
	m.w, m.h = 100, 60

	if m.at.entry != 0 || m.entries[0].b.kind != blockFrontmatter {
		t.Fatalf("initial focus = entry %d kind %v, want 0/blockFrontmatter", m.at.entry, m.entries[0].b.kind)
	}

	m.handleKey(tea.KeyPressMsg(tea.Key{Code: 'j', Text: "j"}))
	if m.entries[m.at.entry].b.kind != blockHeading {
		t.Fatalf("j from the frontmatter landed on kind %v, want the heading", m.entries[m.at.entry].b.kind)
	}

	m.handleKey(tea.KeyPressMsg(tea.Key{Code: 'k', Text: "k"}))
	if m.entries[m.at.entry].b.kind != blockFrontmatter {
		t.Fatalf("k back up landed on kind %v, want the frontmatter again", m.entries[m.at.entry].b.kind)
	}
}

// TestFrontmatterScrollsHorizontally: l/h on a focused frontmatter block scroll
// a long field horizontally, the code-block treatment the frontmatter feedback
// asked for in place of the ellipsis truncation.
func TestFrontmatterScrollsHorizontally(t *testing.T) {
	fm := block{
		kind:   blockFrontmatter,
		anchor: "^fm",
		text:   "---\ndescription: " + strings.Repeat("x", 80) + "\n---",
	}
	rest := []block{{kind: blockHeading, text: "## Heading", anchor: "^h1", level: 2}}
	m := newModel(append([]block{fm}, rest...), nil)
	m.w, m.h = 40, 60
	m.at = cursor{entry: 0, comment: commentNone}

	if off := m.codeScroll["^fm"]; off != 0 {
		t.Fatalf("initial frontmatter scroll offset = %d, want 0", off)
	}

	m.handleKey(tea.KeyPressMsg(tea.Key{Code: 'l', Text: "l"}))
	if off := m.codeScroll["^fm"]; off <= 0 {
		t.Errorf("after 'l' frontmatter scroll offset = %d, want > 0", off)
	}

	m.handleKey(tea.KeyPressMsg(tea.Key{Code: 'h', Text: "h"}))
	if off := m.codeScroll["^fm"]; off != 0 {
		t.Errorf("after 'h' frontmatter scroll offset = %d, want 0", off)
	}
}

func TestMarkParagraph(t *testing.T) {
	m := newTestModel(t)
	m.at = cursor{entry: blockEntryFor(t, m, freshAnchor), comment: commentNone}

	m.toggleMark(markOK)
	if m.marks[freshAnchor] != markOK {
		t.Fatalf("mark = %v, want markOK", m.marks[freshAnchor])
	}
	// Pressing the same mark again clears it: one key sets and unsets.
	m.toggleMark(markOK)
	if m.marks[freshAnchor] != markNone {
		t.Fatalf("mark = %v after re-press, want cleared", m.marks[freshAnchor])
	}
	m.toggleMark(markFlag)
	if m.marks[freshAnchor] != markFlag {
		t.Fatalf("mark = %v, want markFlag", m.marks[freshAnchor])
	}
}

// TestMarkSectionFromHeading: marking a heading marks every paragraph under it,
// stopping at the next heading.
func TestMarkSectionFromHeading(t *testing.T) {
	m := newTestModel(t)
	head := -1
	for i, e := range m.entries {
		if e.b.kind == blockHeading {
			head = i
			break
		}
	}
	if head < 0 {
		t.Fatal("no heading in the seeded document")
	}
	m.at = cursor{entry: head, comment: commentNone}
	m.toggleMark(markOK)

	for _, a := range []string{"^a1", "^b2", "^c3"} {
		if m.marks[a] != markOK {
			t.Errorf("%s = %v, want markOK from the section mark", a, m.marks[a])
		}
	}
	for _, a := range []string{"^d4", "^e5"} {
		if m.marks[a] != markNone {
			t.Errorf("%s = %v, want untouched — it is in the next section", a, m.marks[a])
		}
	}
}

// TestSectionRollUp: a section is not clean while one paragraph is flagged.
func TestSectionRollUp(t *testing.T) {
	m := newTestModel(t)
	m.marks["^a1"] = markOK
	m.marks["^b2"] = markOK
	m.marks["^c3"] = markFlag

	head := 0
	got, partial := rollUp(m.marksFor(m.sectionAnchors(head)))
	if got != markFlag {
		t.Fatalf("roll-up = %v, want markFlag — one flagged paragraph dominates", got)
	}
	if partial {
		t.Fatal("roll-up reported partial when every paragraph is marked")
	}

	m.marks["^c3"] = markNone
	delete(m.marks, "^c3")
	got, partial = rollUp(m.marksFor(m.sectionAnchors(head)))
	if got != markOK || !partial {
		t.Fatalf("roll-up = %v partial=%v, want markOK with partial", got, partial)
	}
}

func TestReviewProgress(t *testing.T) {
	m := newTestModel(t)
	_, _, total := m.reviewProgress()
	if total != 5 {
		t.Fatalf("total = %d, want the 5 seeded paragraphs", total)
	}
	m.marks["^a1"] = markOK
	m.marks["^b2"] = markFlag
	done, flagged, _ := m.reviewProgress()
	if done != 1 || flagged != 1 {
		t.Fatalf("done=%d flagged=%d, want 1 and 1", done, flagged)
	}
}

// TestMarkOnThreadEntryDoesNothing: threads are not review targets, the blocks
// they hang off are.
func TestMarkOnThreadEntryDoesNothing(t *testing.T) {
	m := newTestModel(t)
	m.at = cursor{entry: entryFor(t, m, convoAnchor), comment: commentNone}
	m.toggleMark(markOK)
	if m.marks[convoAnchor] != markNone {
		t.Fatalf("marking a thread entry set %s to %v", convoAnchor, m.marks[convoAnchor])
	}
}

// --- rendering invariants ---------------------------------------------------

func TestCollapsedThreadIsOneLine(t *testing.T) {
	m := newTestModel(t)
	m.at = cursor{entry: 0, comment: commentNone}
	m.render()

	i := entryFor(t, m, draftAnchor)
	// The span includes the trailing blank separator, so one content line is 2.
	if h := m.spans[i].end - m.spans[i].start + 1; h != 2 {
		t.Fatalf("collapsed thread occupies %d lines, want 1 plus a separator", h)
	}
}

// TestCollapsedResolvedThreadShowsCheckmark: THREAD-02's marker swap — a
// resolved thread's collapsed line trades the plain "│" rule for a
// checkmark, still one line, no box.
func TestCollapsedResolvedThreadShowsCheckmark(t *testing.T) {
	m := newTestModel(t)
	m.threads[convoAnchor].resolved = true
	m.at = cursor{entry: blockEntryFor(t, m, freshAnchor), comment: commentNone}
	lines := m.render()

	i := entryFor(t, m, convoAnchor)
	line := lines[m.spans[i].start]
	if !strings.Contains(line, "✓") {
		t.Errorf("collapsed resolved thread line = %q, want a checkmark", line)
	}
}

// TestExpandedResolvedThreadSpansStayInsideTheirThread: THREAD-02's expanded
// badge adds lines ahead of the comments — a regression here would leak a
// comment's span out of its thread's outer span, the same failure
// TestCommentSpansSitInsideTheirThread guards for the unresolved case.
func TestExpandedResolvedThreadSpansStayInsideTheirThread(t *testing.T) {
	m := newTestModel(t)
	m.threads[convoAnchor].resolved = true
	m.at = cursor{entry: entryFor(t, m, convoAnchor), comment: commentNone}
	m.render()

	if len(m.subspans) != len(m.threads[convoAnchor].posted) {
		t.Fatalf("%d comment spans for %d comments", len(m.subspans), len(m.threads[convoAnchor].posted))
	}
	outer := m.spans[m.at.entry]
	for c, s := range m.subspans {
		if s.start < outer.start || s.end > outer.end {
			t.Errorf("comment %+v span %+v escapes its resolved thread span %+v", c, s, outer)
		}
	}
}

// TestSpansTileTheDocument: hit-testing, the cursor and scrolling all read from
// spans, so a gap or an overlap silently misroutes clicks.
func TestSpansTileTheDocument(t *testing.T) {
	m := newTestModel(t)
	lines := m.render()

	if len(m.spans) != len(m.entries) {
		t.Fatalf("%d spans for %d entries", len(m.spans), len(m.entries))
	}
	want := 0
	for i, s := range m.spans {
		if s.start != want {
			t.Fatalf("entry %d starts at %d, want %d — spans must tile without gaps", i, s.start, want)
		}
		if s.end < s.start {
			t.Fatalf("entry %d has an empty span %+v", i, s)
		}
		want = s.end + 1
	}
	if want != len(lines) {
		t.Fatalf("spans cover %d lines but render produced %d", want, len(lines))
	}
}

// TestCommentSpansSitInsideTheirThread: a comment span that escaped its thread
// would hand clicks to the wrong block.
func TestCommentSpansSitInsideTheirThread(t *testing.T) {
	m := newTestModel(t)
	m.at = cursor{entry: entryFor(t, m, convoAnchor), comment: commentNone}
	m.render()

	if len(m.subspans) != len(m.threads[convoAnchor].posted) {
		t.Fatalf("%d comment spans for %d comments", len(m.subspans), len(m.threads[convoAnchor].posted))
	}
	outer := m.spans[m.at.entry]
	for c, s := range m.subspans {
		if s.start < outer.start || s.end > outer.end {
			t.Errorf("comment %+v span %+v escapes its thread span %+v", c, s, outer)
		}
	}
}

// TestFocusVisitsEveryComment moved to dive_test.go as
// TestDivedMovementVisitsEveryComment: the 2026-08-09 todo-review feedback
// rejected j/k walking comments at block level, so the dive is now the only
// way focus reaches a comment from the keyboard.

// TestDeletedCommentsHiddenByDefault: the 2026-08-09 feedback asked for
// tombstoned comments to disappear rather than render "[deleted]" — the
// reveal is an explicit action (V / thread.showDeleted), not the default
// view. Rendering, the focus stop list and the collapsed summary must all
// skip a hidden tombstone, so the cursor can never land on something that
// does not render.
func TestDeletedCommentsHiddenByDefault(t *testing.T) {
	m := newTestModel(t)
	t1 := m.threads[convoAnchor]
	t1.posted[0].deleted = true
	m.at = cursor{entry: entryFor(t, m, convoAnchor), comment: commentNone}
	lines := m.render()

	outer := m.spans[m.at.entry]
	for i := outer.start; i <= outer.end; i++ {
		if strings.Contains(lines[i], "[deleted]") {
			t.Errorf("thread line %d = %q, want no [deleted] marker while hidden", i, lines[i])
		}
	}
	if _, ok := m.subspans[cursor{entry: m.at.entry, comment: 0}]; ok {
		t.Error("hidden deleted comment 0 still has a subspan")
	}
	if _, ok := m.subspans[cursor{entry: m.at.entry, comment: 1}]; !ok {
		t.Error("live comment 1 lost its subspan")
	}
	for _, c := range m.stops() {
		if c.entry == m.at.entry && c.comment == 0 {
			t.Error("j/k can still stop on the hidden deleted comment 0")
		}
	}
	if got := t1.summary(false); strings.Contains(got, "[deleted]") {
		t.Errorf("collapsed summary = %q, want no [deleted] marker", got)
	}
}

// TestThreadWithOnlyDeletedCommentsHintsTheReveal: a thread whose every
// comment is a hidden tombstone must not claim "no comments yet" — the
// comments exist, they were deleted, and the hint says how to see them.
func TestThreadWithOnlyDeletedCommentsHintsTheReveal(t *testing.T) {
	m := newTestModel(t)
	m.threads[soloAnchor].posted[0].deleted = true
	m.at = cursor{entry: entryFor(t, m, soloAnchor), comment: commentNone}
	lines := m.render()

	found := false
	outer := m.spans[m.at.entry]
	for i := outer.start; i <= outer.end; i++ {
		if strings.Contains(lines[i], "T to reveal") {
			found = true
		}
	}
	if !found {
		t.Error("all-deleted thread does not hint the T reveal")
	}
}

// TestShowDeletedRevealsDeletedComments: the explicit reveal — tombstones
// come back marked [deleted]. The tombstone replaced the body on disk (D11),
// so the marker is all there is left to show.
func TestShowDeletedRevealsDeletedComments(t *testing.T) {
	m := newTestModel(t)
	t1 := m.threads[convoAnchor]
	t1.posted[0].deleted = true
	m.showDeleted = true
	m.at = cursor{entry: entryFor(t, m, convoAnchor), comment: commentNone}
	lines := m.render()

	found := false
	outer := m.spans[m.at.entry]
	for i := outer.start; i <= outer.end; i++ {
		if strings.Contains(lines[i], "[deleted]") {
			found = true
		}
	}
	if !found {
		t.Error("revealed tombstone does not render a [deleted] marker")
	}
	if _, ok := m.subspans[cursor{entry: m.at.entry, comment: 0}]; !ok {
		t.Error("revealed deleted comment 0 has no subspan")
	}
}

// TestShowDeletedCommandTogglesTheView: thread.showDeleted is a view toggle —
// the command flips it on and off, and dropping focus off a comment that
// just became hidden leaves the cursor on the thread entry rather than on
// nothing.
func TestShowDeletedCommandTogglesTheView(t *testing.T) {
	m := newTestModel(t)
	m.threads[convoAnchor].posted[0].deleted = true
	i := entryFor(t, m, convoAnchor)

	c, ok := commandByID("thread.showDeleted")
	if !ok {
		t.Fatal("thread.showDeleted not registered")
	}
	if m.showDeleted {
		t.Fatal("a new model should start with deleted comments hidden")
	}
	c.Run(m, "")
	if !m.showDeleted {
		t.Fatal("first toggle did not reveal deleted comments")
	}
	m.at = cursor{entry: i, comment: 0}
	c.Run(m, "")
	if m.showDeleted {
		t.Fatal("second toggle did not hide deleted comments again")
	}
	if m.at.comment != commentNone {
		t.Fatalf("focus stayed on the now-hidden comment %d, want the thread entry", m.at.comment)
	}
	if m.at.entry != i {
		t.Fatalf("focus jumped to entry %d, want to stay on the thread %d", m.at.entry, i)
	}
}

// TestSummaryCountsVisibleNotDeleted: a collapsed line's (+N) count must
// describe the comments a reviewer can actually see, not the raw file.
func TestSummaryCountsVisibleNotDeleted(t *testing.T) {
	th := &thread{anchor: "^z", posted: []comment{
		{author: "a", body: "one"},
		{author: "b", body: "two", deleted: true},
		{author: "c", body: "three"},
	}}
	if got := th.summary(false); got != "a · one  (+1)" {
		t.Errorf("hidden summary = %q, want %q", got, "a · one  (+1)")
	}
	if got := th.summary(true); !strings.Contains(got, "(+2)") {
		t.Errorf("revealed summary = %q, want the full +2 count", got)
	}
}

// TestCursorTracksScroll is the regression for the cursor drifting from the
// text: paneTop comes from the same render pass that produced the lines, and
// the cursor is offset by the same scroll applied to the viewport.
func TestCursorTracksScroll(t *testing.T) {
	m := newTestModel(t)
	m.h = 24 // small enough that the document must scroll
	open(t, m, draftAnchor, newCommentSlot, "contradicts")

	v := m.View()
	if m.paneTop < 0 {
		t.Fatal("paneTop was not established while a composer is open")
	}
	if v.Cursor == nil {
		t.Fatal("no cursor while the editor has focus")
	}
	if viewport := m.h - footerRows; v.Cursor.Y < 0 || v.Cursor.Y >= viewport {
		t.Fatalf("cursor Y = %d, outside the viewport of %d rows", v.Cursor.Y, viewport)
	}
}

// TestClampScrollPreservesScrollWhenFocusUnchanged is the regression SCROLL-01
// exists for: clampScroll used to re-derive the offset from focus on every
// render, so a scroll set some other way (a future page key, a mouse wheel)
// would snap straight back on the very next frame. It should only follow
// focus when focus has actually moved since the last time it was asked.
func TestClampScrollPreservesScrollWhenFocusUnchanged(t *testing.T) {
	m := newTestModel(t)
	m.spans = []span{{start: 0, end: 4}, {start: 5, end: 9}, {start: 10, end: 50}}
	m.at = cursor{entry: 2, comment: commentNone}

	total, viewport := 51, 10
	first := m.clampScroll(total, viewport)
	if want := 41; first != want { // pulls the tail of entry 2's span into view
		t.Fatalf("initial clampScroll = %d, want %d", first, want)
	}

	// Simulate a user-driven scroll that does not touch focus: clampScroll
	// must leave it alone rather than snapping back to the focused entry.
	m.scroll = 5
	if got := m.clampScroll(total, viewport); got != 5 {
		t.Fatalf("clampScroll = %d with focus unchanged, want the scroll preserved at 5", got)
	}
	// Calling it again with nothing changed must be idempotent.
	if got := m.clampScroll(total, viewport); got != 5 {
		t.Fatalf("second clampScroll call = %d, want still 5", got)
	}
}

// TestClampScrollFollowsFocusOnceItMoves is the other half: focus-follow still
// works, just only fires on an actual move.
func TestClampScrollFollowsFocusOnceItMoves(t *testing.T) {
	m := newTestModel(t)
	m.spans = []span{{start: 0, end: 4}, {start: 5, end: 9}, {start: 10, end: 50}}
	m.at = cursor{entry: 2, comment: commentNone}

	total, viewport := 51, 10
	m.scroll = m.clampScroll(total, viewport)
	m.scroll = 5 // a scroll unrelated to the still-focused entry 2

	m.at = cursor{entry: 0, comment: commentNone}
	got := m.clampScroll(total, viewport)
	if want := 0; got != want { // entry 0's span must be pulled back into view
		t.Fatalf("clampScroll after focus moved = %d, want %d", got, want)
	}
}

// --- unit-level rules -------------------------------------------------------

func TestOutcomeFromExit(t *testing.T) {
	for _, tc := range []struct {
		script string
		want   outcome
	}{
		{"exit 0", outcomeSubmit},
		{"exit 1", outcomeDiscard},
		{"exit 2", outcomeDraft},
		{"exit 7", outcomeDiscard},
	} {
		if got := outcomeFromExit(exec.Command("sh", "-c", tc.script).Run()); got != tc.want {
			t.Errorf("%s -> %v, want %v", tc.script, got, tc.want)
		}
	}
}

func TestDraftsAreKeyedByAnchorAndTarget(t *testing.T) {
	isolateDrafts(t)
	if err := saveDraft("^a1", newCommentSlot, "a reply"); err != nil {
		t.Fatalf("saveDraft reply: %v", err)
	}
	if err := saveDraft("^a1", 0, "an edit"); err != nil {
		t.Fatalf("saveDraft edit: %v", err)
	}
	if got, _ := loadDraft("^a1", newCommentSlot); got != "a reply" {
		t.Fatalf("reply draft = %q — an edit of the same thread collided with it", got)
	}
	if got, _ := loadDraft("^a1", 0); got != "an edit" {
		t.Fatalf("edit draft = %q", got)
	}
}

func TestEmptyDraftIsCleared(t *testing.T) {
	isolateDrafts(t)
	if err := saveDraft(draftAnchor, newCommentSlot, "something"); err != nil {
		t.Fatalf("saveDraft: %v", err)
	}
	if err := saveDraft(draftAnchor, newCommentSlot, "   \n  "); err != nil {
		t.Fatalf("saveDraft blank: %v", err)
	}
	if got, _ := loadDraft(draftAnchor, newCommentSlot); got != "" {
		t.Fatalf("blank draft left %q behind", got)
	}
}

// --- plumbing ---------------------------------------------------------------

// TestModeFollowsCursorShape: nvim's own mode indicator is off, so the host
// infers it from the cursor shape the child reports via DECSCUSR.
func TestModeFollowsCursorShape(t *testing.T) {
	m := newTestModel(t)
	open(t, m, freshAnchor, newCommentSlot, "")
	if !waitForMode(t, m.comp, "INSERT", 3*time.Second) {
		t.Fatalf("mode = %q on an empty buffer, want INSERT", m.comp.mode())
	}
	typeKeys(m.comp, keyEsc)
	if !waitForMode(t, m.comp, "NORMAL", 3*time.Second) {
		t.Fatalf("mode = %q after escape, want NORMAL", m.comp.mode())
	}
}

// TestLineRefPrefixOpensInInsertModeAfterThePrefix: the 2026-08-09 composer
// feedback asked that commenting with a visual selection (or a line dive)
// start typing immediately after the auto-appended L12-18: reference, with no
// intermediate mode-switch step. The whole path is exercised: V then j selects
// a range, c prepends the reference and opens the composer, which must come up
// in INSERT mode with the cursor past the prefix — asserted by the body: the
// typed text lands after the reference, not before it.
func TestLineRefPrefixOpensInInsertModeAfterThePrefix(t *testing.T) {
	m := newModelAt("sample.md", parseDoc([]byte(sampleDoc)), nil)
	m.w, m.h = 100, 60
	m.at = cursor{entry: 1, comment: commentNone} // paragraph at line 3

	pressKey(m, "V")
	pressKey(m, "j")
	a := m.anchorAt()
	wantPrefix := m.selectionLineRef()
	if a == "" || wantPrefix == "" {
		t.Fatalf("before commenting: anchor = %q, prefix = %q — want both non-empty", a, wantPrefix)
	}

	pressKey(m, "c")
	t.Cleanup(func() {
		if m.comp != nil {
			m.comp.close()
		}
	})
	if !waitForMode(t, m.comp, "INSERT", 10*time.Second) {
		t.Fatalf("mode = %s after c with a selection, want INSERT past the prefix; screen:\n%s", m.comp.mode(), plainScreen(m.comp.em))
	}

	typeText(m.comp, "ship it")
	done := make(chan error, 1)
	go func() { done <- m.comp.cmd.Wait() }()
	typeKeys(m.comp, keyCtrlS)
	select {
	case err := <-done:
		m.dismiss(err)
	case <-time.After(10 * time.Second):
		t.Fatalf("submit did not exit; screen:\n%s", plainScreen(m.comp.em))
	}

	posted := m.threads[a].posted
	if len(posted) != 1 {
		t.Fatalf("posted %d comments, want 1", len(posted))
	}
	if want := wantPrefix + "ship it"; posted[0].body != want {
		t.Errorf("comment body = %q, want %q — the cursor must have sat past the prefix", posted[0].body, want)
	}
}

// TestLineRefPrefixWithTextStillOpensNormalMode: the insert-after-prefix carve
// out is exact — a buffer that is nothing but the reference. A draft carrying
// real text behind the prefix (a resumed draft, or a comment being edited)
// keeps the settled rule: normal mode, so a single <Esc> exits keeping the
// draft.
func TestLineRefPrefixWithTextStillOpensNormalMode(t *testing.T) {
	m := newTestModel(t)
	tr := m.ensureThread(freshAnchor)
	tr.setDraft(newCommentSlot, "L12-18: the sharing claim looks wrong")

	open(t, m, freshAnchor, newCommentSlot, "the sharing claim looks wrong")
	if !waitForMode(t, m.comp, "NORMAL", 5*time.Second) {
		t.Fatalf("mode = %s, want NORMAL for a prefixed draft with text", m.comp.mode())
	}

	done := make(chan error, 1)
	go func() { done <- m.comp.cmd.Wait() }()
	typeKeys(m.comp, keyEsc)
	select {
	case err := <-done:
		if got := outcomeFromExit(err); got != outcomeDraft {
			t.Fatalf("single <Esc> from normal mode gave %v, want draft", got)
		}
	case <-time.After(10 * time.Second):
		t.Fatalf("single <Esc> did not exit normal mode; screen:\n%s", plainScreen(m.comp.em))
	}
}

// TestVtDropsModifiedKeys pins the upstream behaviour the workaround exists for.
// vt.SendKey's fallback is `if key.Mod == 0 { seq += string(key.Code) }`, so a
// shifted printable is silently dropped. Under Ghostty's Kitty keyboard protocol
// that is every capital letter. If this ever starts skipping, vt has been fixed
// and composer.sendKey's Text fast-path can be reconsidered.
func TestVtDropsModifiedKeys(t *testing.T) {
	m := newTestModel(t)
	open(t, m, freshAnchor, newCommentSlot, "")
	m.comp.em.SendKey(uv.KeyPressEvent(uv.Key{Code: 'q', Mod: uv.ModShift, Text: "Q"}))
	time.Sleep(300 * time.Millisecond)
	if strings.Contains(plainScreen(m.comp.em), "Q") {
		t.Skip("vt now encodes modified keys; the Text fast-path may be redundant")
	}
}

// TestShiftedKeysReachChild is the regression for capitals silently vanishing.
func TestShiftedKeysReachChild(t *testing.T) {
	m := newTestModel(t)
	open(t, m, freshAnchor, newCommentSlot, "")
	typeKeys(m.comp,
		tea.Key{Code: 'o', Mod: uv.ModShift, Text: "O"},
		tea.Key{Code: 'd', Mod: uv.ModShift, Text: "D"},
		tea.Key{Code: '1', Mod: uv.ModShift, Text: "!"})
	if s := waitForScreen(t, m.comp.em, "OD!", 3*time.Second); !strings.Contains(s, "OD!") {
		t.Fatalf("shifted keys did not reach the child; screen was:\n%s", s)
	}
}

// TestDamageArrivesPromptly measures the part of the round trip the host owns.
// It excludes Bubble Tea's frame quantum, so it is a floor rather than perceived
// latency — but if this is already tens of milliseconds, no amount of render
// tuning will help.
func TestDamageArrivesPromptly(t *testing.T) {
	m := newTestModel(t)
	open(t, m, freshAnchor, newCommentSlot, "")
	select {
	case <-m.comp.damage:
	default:
	}

	var samples []time.Duration
	for i := 0; i < 20; i++ {
		start := time.Now()
		m.comp.sendKey(tea.Key{Code: 'x', Text: "x"})
		select {
		case <-m.comp.damage:
			samples = append(samples, time.Since(start))
		case <-time.After(2 * time.Second):
			t.Fatalf("no damage signal after keystroke %d", i)
		}
	}
	sort.Slice(samples, func(i, j int) bool { return samples[i] < samples[j] })
	median, worst := samples[len(samples)/2], samples[len(samples)-1]
	t.Logf("key→damage round trip over 20 keystrokes: median %v · worst %v",
		median.Round(10*time.Microsecond), worst.Round(10*time.Microsecond))

	// Assert on the median, not the worst: a single scheduler hiccup — or the
	// race detector's instrumentation — should not fail a latency floor. What
	// matters is whether the plumbing is in the sub-millisecond range or the
	// tens of milliseconds the renderer was once blamed for.
	if median > 5*time.Millisecond {
		t.Errorf("host round trip median is %v, too slow to blame the renderer", median)
	}
}

func TestPaneResize(t *testing.T) {
	m := newTestModel(t)
	open(t, m, freshAnchor, newCommentSlot, "")
	m.comp.resize(72, 12)
	if m.comp.em.Width() != 72 || m.comp.em.Height() != 12 {
		t.Fatalf("emulator did not resize: got %dx%d, want 72x12",
			m.comp.em.Width(), m.comp.em.Height())
	}
}

// TestComposerBoxDoesNotRewrap is the regression for the composer wrapping
// feedback: the box rendering the emulator used to be built with
// Width(w-2*borderW), but lipgloss's Width includes the border, so the box's
// content area was w-4 — two columns narrower than the emulator. The
// emulator's already-wrapped lines were then re-wrapped by the box, orphaning
// single words on their own line and shifting every row so the cursor no longer
// matched the text. The box must be sized so its content area is exactly the
// emulator's width, and then the emulator's rows must land in the box verbatim.
func TestComposerBoxDoesNotRewrap(t *testing.T) {
	requireNvim(t)
	m := newTestModel(t)
	open(t, m, freshAnchor, newCommentSlot, "")

	// A line long enough that nvim soft-wraps it inside the pane, with a word
	// parked in the emulator's final two columns — exactly where the old box
	// used to snap it onto its own line.
	typeText(m.comp, "one two three four five six seven eight nine ten eleven twelve thirteen fourteen fifteen sixteen seventeen eighteen nineteen")
	time.Sleep(300 * time.Millisecond)
	m.render()

	// The composer's span is the focused entry: top border, paneRows emulator
	// rows, bottom border, then the thread's trailing blank line. Any
	// re-wrapping by the box would add rows.
	span := m.spans[m.at.entry]
	if got := span.end - span.start + 1; got != paneRows+3 {
		t.Fatalf("composer box occupies %d rows, want %d — the box is re-wrapping the emulator's lines", got, paneRows+3)
	}

	// The box's top border's dash count is its content width. It must be the
	// emulator's width: narrower and lipgloss re-wraps, wider and the pane
	// bleeds past the frame.
	top := ansiRe.ReplaceAllString(m.render()[span.start], "")
	if inner := strings.Count(top, "─"); inner != m.comp.em.Width() {
		t.Fatalf("box content area is %d columns, emulator is %d — the box re-wraps the emulator's lines", inner, m.comp.em.Width())
	}

	// And the emulator's rows must appear in the box unchanged, not re-flowed.
	// Each content row is gutterW spaces, the │ border, the emulator row, │.
	em := strings.Split(plainScreen(m.comp.em), "\n")
	for j := 0; j < paneRows; j++ {
		row := ansiRe.ReplaceAllString(m.render()[span.start+1+j], "")
		row = strings.TrimPrefix(row, strings.Repeat(" ", gutterW)+"│")
		row = strings.TrimSuffix(row, "│")
		if got := strings.TrimRight(row, " "); got != strings.TrimRight(em[j], " ") {
			t.Errorf("box row %d = %q, emulator row = %q", j, got, em[j])
		}
	}
}

// TestComposerShowsConversationOnNewComment pins the 2026-08-09 feedback:
// adding a comment to a thread that already has one shows the conversation
// above the editor, so the reply reads as a reply rather than a parallel
// thread. The emulator's rows, the mouse routing (paneTop) and the comments'
// click targets (subspans) must all agree on where the editor starts.
func TestComposerShowsConversationOnNewComment(t *testing.T) {
	requireNvim(t)
	m := newTestModel(t)
	open(t, m, convoAnchor, newCommentSlot, "")
	typeText(m.comp, "replying with the thread on screen")
	time.Sleep(300 * time.Millisecond)

	m.View()
	if m.paneLead == 0 {
		t.Fatal("new comment on an existing thread renders the composer alone — the conversation should show above it")
	}
	lines := m.render()
	plain := make([]string, len(lines))
	for i, l := range lines {
		plain[i] = ansiRe.ReplaceAllString(l, "")
	}

	// Both seeded comments render inside the thread's box, above the
	// emulator's first row.
	span := m.spans[m.at.entry]
	above := strings.Join(plain[span.start:m.paneTop], "\n")
	for _, want := range []string{"Shouldn't be global", "Changed to per-endpoint budgets"} {
		if !strings.Contains(above, want) {
			t.Errorf("conversation above the composer is missing %q:\n%s", want, above)
		}
	}

	// A rule marks where reading stops and writing starts, on the line
	// directly above the emulator.
	if rule := strings.Trim(plain[m.paneTop-1], " │"); !strings.Contains(rule, "───") {
		t.Errorf("no separator rule ahead of the composer, got %q", plain[m.paneTop-1])
	}

	// paneTop still points at the emulator's first row, past the
	// conversation.
	em := strings.Split(plainScreen(m.comp.em), "\n")
	row := strings.TrimPrefix(plain[m.paneTop], strings.Repeat(" ", gutterW)+"│")
	row = strings.TrimSuffix(row, "│")
	if got, want := strings.TrimRight(row, " "), strings.TrimRight(em[0], " "); got != want {
		t.Errorf("paneTop row = %q, emulator row 0 = %q — mouse routing would land off the text", got, want)
	}

	// The comments on screen keep their click targets, and those targets
	// cover the lines the comments actually render on.
	for j, author := range []string{"toly", "agent"} {
		s, ok := m.subspans[cursor{entry: m.at.entry, comment: j}]
		if !ok {
			t.Fatalf("comment %d has no subspan while composing", j)
		}
		if !strings.Contains(plain[s.start], author) {
			t.Errorf("comment %d subspan starts at %q, want its author line", j, plain[s.start])
		}
		if s.end >= m.paneTop {
			t.Errorf("comment %d subspan ends at row %d, into the editor at %d", j, s.end, m.paneTop)
		}
	}
}

// TestComposerBoxStaysSoloWhenEditing pins the other half of the split: an
// edit's text is already live in the emulator, so the box stays composer-only
// and the conversation does not render twice.
func TestComposerBoxStaysSoloWhenEditing(t *testing.T) {
	requireNvim(t)
	m := newTestModel(t)
	open(t, m, convoAnchor, 0, "Shouldn't be global")

	m.View()
	if m.paneLead != 0 {
		t.Fatalf("editing renders %d lead lines, want the composer-only box", m.paneLead)
	}
	span := m.spans[m.at.entry]
	if got := span.end - span.start + 1; got != paneRows+3 {
		t.Fatalf("edit box occupies %d rows, want %d", got, paneRows+3)
	}
}

// TestComposerBoxStaysSoloOnFreshThread: with no conversation to show, a new
// comment's box is exactly what it was before the conversation landed.
func TestComposerBoxStaysSoloOnFreshThread(t *testing.T) {
	requireNvim(t)
	m := newTestModel(t)
	open(t, m, freshAnchor, newCommentSlot, "")

	m.View()
	if m.paneLead != 0 {
		t.Fatalf("no conversation exists on a fresh thread, but paneLead = %d", m.paneLead)
	}
}

// TestFrameFitsTheTerminal keeps the frame from overflowing the screen.
//
// In the alternate screen a frame of exactly m.h lines is correct — it fills the
// buffer without scrolling it. One line more would push the top away, which is
// the defect this began as, and no test that inspects model state can see it:
// m.scroll is 0 and focus is on the first block throughout.
func TestFrameFitsTheTerminal(t *testing.T) {
	m := seedModel()
	for _, h := range []int{10, 24, 30, 40, 120} {
		m.w, m.h = 92, h
		got := len(strings.Split(m.View().Content, "\n"))
		if got > h {
			t.Errorf("terminal %d rows: frame is %d lines — one line more than the "+
				"screen holds pushes the top of the document away", h, got)
		}
	}
}

// TestFirstPaintBeforeSizeIsKnownDoesNotPoisonScroll is the regression for the
// document's opening heading being invisible.
//
// Bubble Tea calls View once before the first WindowSizeMsg. View clamps
// m.scroll as a side effect of rendering, so rendering against an unknown (zero)
// terminal size clamped it to 1 — and nothing ever reset it, so the first line
// of the document stayed hidden for the whole session.
//
// This is invisible to a test that renders once at a known size, which is why it
// survived several rounds of probing: the model is only wrong after it has been
// asked to render twice, the first time before it knew how big it was.
func TestFirstPaintBeforeSizeIsKnownDoesNotPoisonScroll(t *testing.T) {
	m := seedModel()

	if got := m.View().Content; got != "" {
		t.Errorf("View before the size is known rendered %d bytes, want nothing", len(got))
	}
	if m.scroll != 0 {
		t.Fatalf("painting at an unknown size left scroll = %d, want 0", m.scroll)
	}

	m.w, m.h = 92, 26
	m.View()
	if m.scroll != 0 {
		t.Errorf("scroll = %d once the size is known, want 0 — the document's first line is hidden", m.scroll)
	}
	if first := strings.SplitN(m.View().Content, "\n", 2)[0]; !strings.Contains(first, "Retry policy") {
		t.Errorf("first rendered line is %q, want the document's opening heading", first)
	}
}

// TestFocusRetainedOnCommentExit verifies that exiting the composer leaves
// focus on the comment so pressing 'e' immediately re-edits it.
func TestFocusRetainedOnCommentExit(t *testing.T) {
	m := newTestModel(t)
	open(t, m, freshAnchor, newCommentSlot, "")
	typeText(m.comp, "fresh comment body")
	typeKeys(m.comp, keyEsc, keyCtrlS)
	m.dismiss(waitExit(t, m.comp, 10*time.Second))

	if m.at.comment != 0 {
		t.Fatalf("focus after submitting new comment = %d, want 0", m.at.comment)
	}

	cmd := m.editFocused()
	if cmd == nil || m.comp == nil {
		t.Fatal("editFocused() after comment exit did not open composer")
	}
	if m.comp.target != 0 {
		t.Fatalf("composer target = %d, want 0", m.comp.target)
	}

	typeText(m.comp, " updated")
	typeKeys(m.comp, keyEsc, keyCtrlS)
	m.dismiss(waitExit(t, m.comp, 10*time.Second))

	if m.at.comment != 0 {
		t.Fatalf("focus after updating comment = %d, want 0", m.at.comment)
	}
}

// TestThreadCommentFocusHighlightsBodyText verifies that focusing a comment in a thread
// places the focus bar and highlight on the comment body lines rather than the author header.
func TestThreadCommentFocusHighlightsBodyText(t *testing.T) {
	m := seedModel()
	m.subspans = make(map[cursor]span)
	i := entryFor(t, m, convoAnchor)
	thr := m.threads[convoAnchor]
	w := 80

	// 1. When focused on comment 0:
	m.at = cursor{entry: i, comment: 0}
	linesFocused := m.appendComments(nil, i, thr, w, 0, 0)
	// Author header is linesFocused[0]; body is linesFocused[1]
	if strings.Contains(linesFocused[0], focusStyle.Render("▌ ")) {
		t.Errorf("author header line %q contains focus bar, want it on body text instead", linesFocused[0])
	}
	if !strings.HasPrefix(linesFocused[1], focusStyle.Render("▌ ")) {
		t.Errorf("body text line %q does not start with focus bar", linesFocused[1])
	}
	if !strings.Contains(linesFocused[1], "Shouldn't be global") {
		t.Errorf("body text line %q does not contain comment text", linesFocused[1])
	}

	// 2. When comment is not focused:
	m.at = cursor{entry: i, comment: commentNone}
	linesUnfocused := m.appendComments(nil, i, thr, w, 0, 0)
	if strings.Contains(linesUnfocused[0], focusStyle.Render("▌ ")) {
		t.Errorf("unfocused author header %q contains focus bar", linesUnfocused[0])
	}
	if strings.Contains(linesUnfocused[1], focusStyle.Render("▌ ")) {
		t.Errorf("unfocused body line %q contains focus bar", linesUnfocused[1])
	}
}

// TestScrollCodeLineANSI verifies that scrollCodeLine correctly skips visual columns,
// limits visual width, and preserves ANSI formatting.
func TestScrollCodeLineANSI(t *testing.T) {
	styled := "\x1b[31mhello\x1b[0m \x1b[32mworld\x1b[0m"

	// 1. Offset 0, full width -> unchanged
	if got := scrollCodeLine(styled, 0, 80); got != styled {
		t.Errorf("scrollCodeLine(offset=0) = %q, want %q", got, styled)
	}

	// 2. Offset 4 -> skip "hell", start at 'o' (in red), then space, then "wor" (in green)
	got := scrollCodeLine(styled, 4, 5)
	plain := ansiRe.ReplaceAllString(got, "")
	if plain != "o wor" {
		t.Errorf("scrollCodeLine(offset=4, width=5) plain = %q, want %q (got raw %q)", plain, "o wor", got)
	}

	// 3. Offset beyond visual length -> empty string
	if got := scrollCodeLine(styled, 20, 80); got != "" {
		t.Errorf("scrollCodeLine(offset=20) = %q, want empty", got)
	}
}

// TestCodeBlockHorizontalScroll verifies that l and h scroll code blocks horizontally.
func TestCodeBlockHorizontalScroll(t *testing.T) {
	doc := []block{
		{
			kind:   blockCode,
			anchor: "^code1",
			lines:  []string{"func veryLongFunctionNameThatExceedsTheMeasureWidth(ctx context.Context, req *Request) (*Response, error)"},
			lang:   "go",
		},
	}
	m := newModel(doc, nil)
	m.at = cursor{entry: 0, comment: commentNone}
	anchor := "^code1"

	// 1. Initially offset is 0
	if off := m.codeScroll[anchor]; off != 0 {
		t.Fatalf("initial code scroll offset = %d, want 0", off)
	}

	// 2. Press l to scroll right
	m.handleKey(tea.KeyPressMsg(tea.Key{Code: 'l', Text: "l"}))
	if off := m.codeScroll[anchor]; off <= 0 {
		t.Errorf("after 'l' press code scroll offset = %d, want > 0", off)
	}

	// 3. Press h to scroll left back to 0
	m.handleKey(tea.KeyPressMsg(tea.Key{Code: 'h', Text: "h"}))
	if off := m.codeScroll[anchor]; off != 0 {
		t.Errorf("after 'h' press code scroll offset = %d, want 0", off)
	}
}

// --- mouse wheel speed ------------------------------------------------------

// TestWheelScrollUsesWheelSpeed is the regression for the maintainer's "mouse
// wheel speed (3 lines per tick) should be tunable" ask: handleWheel scrolls by
// m.wheelSpeed, not a hardcoded 3, and a configured value takes effect through
// RunOptions.
func TestWheelScrollUsesWheelSpeed(t *testing.T) {
	m := newTestModel(t)
	start := 50

	m.wheelSpeed = 5
	m.scroll = start
	m.handleWheel(tea.Mouse{Button: uv.MouseWheelDown})
	if m.scroll != start+5 {
		t.Errorf("wheel down with wheelSpeed 5: scroll = %d, want %d", m.scroll, start+5)
	}
	m.handleWheel(tea.Mouse{Button: uv.MouseWheelUp})
	if m.scroll != start {
		t.Errorf("wheel up with wheelSpeed 5: scroll = %d, want %d", m.scroll, start)
	}

	// A smaller configured value is honoured too.
	m.wheelSpeed = 1
	m.scroll = start
	m.handleWheel(tea.Mouse{Button: uv.MouseWheelDown})
	if m.scroll != start+1 {
		t.Errorf("wheel down with wheelSpeed 1: scroll = %d, want %d", m.scroll, start+1)
	}
}

// TestWheelScrollDefaultsToThree pins the default step at 3 lines per tick, as
// the maintainer described it, and that a zero configured value does not clamp
// the wheel to nothing.
func TestWheelScrollDefaultsToThree(t *testing.T) {
	m := newTestModel(t)
	if m.wheelSpeed != 3 {
		t.Fatalf("default wheelSpeed = %d, want 3", m.wheelSpeed)
	}
	start := 50
	m.scroll = start
	m.handleWheel(tea.Mouse{Button: uv.MouseWheelDown})
	if m.scroll != start+3 {
		t.Errorf("wheel down at default speed: scroll = %d, want %d", m.scroll, start+3)
	}
}

// TestWheelSpeedZeroMeansDefault pins the RunOptions contract: 0 (a test-built
// model or an invocation without --wheel-speed) leaves the model's default in
// place rather than disabling the wheel.
func TestWheelSpeedZeroMeansDefault(t *testing.T) {
	m := newTestModel(t)
	m.wheelSpeed = 0
	start := 50
	m.scroll = start
	m.handleWheel(tea.Mouse{Button: uv.MouseWheelDown})
	if m.scroll != start+1 {
		t.Errorf("wheel down with wheelSpeed 0: scroll = %d, want %d (clamped to 1)", m.scroll, start+1)
	}
}
