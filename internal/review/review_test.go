package review

import (
	"os/exec"
	"regexp"
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

var ansiRe = regexp.MustCompile(`\x1b\[[0-9;]*[a-zA-Z]`)

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
// so multi-key mappings like <Esc><Esc> and <leader>ck actually resolve.
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
	keyEsc   = tea.Key{Code: uv.KeyEscape}
	keyEnter = tea.Key{Code: uv.KeyEnter}
	keyCtrlS = tea.Key{Code: 's', Mod: uv.ModCtrl}
	keySpace = tea.Key{Code: uv.KeySpace, Text: " "}
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

func TestDoubleEscapeKeepsDraft(t *testing.T) {
	m := newTestModel(t)
	open(t, m, freshAnchor, newCommentSlot, "")
	typeText(m.comp, "half a thought")
	// The first <Esc> leaves insert mode; the next two trigger the mapping.
	typeKeys(m.comp, keyEsc, keyEsc, keyEsc)

	if got := outcomeFromExit(waitExit(t, m.comp, 10*time.Second)); got != outcomeDraft {
		t.Fatalf("outcome = %v, want draft", got)
	}
	if body := m.comp.body(); body != "half a thought" {
		t.Fatalf("body = %q, want the unfinished text preserved", body)
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

// TestFocusVisitsEveryComment: j must step through an expanded thread rather
// than skipping over it.
func TestFocusVisitsEveryComment(t *testing.T) {
	m := newTestModel(t)
	m.at = cursor{entry: 0, comment: commentNone}

	seen := map[cursor]bool{}
	for i := 0; i < 200; i++ {
		seen[m.at] = true
		m.moveFocus(1)
	}
	want := cursor{entry: entryFor(t, m, convoAnchor), comment: 1}
	if !seen[want] {
		t.Fatalf("j never reached %+v; visited %d positions", want, len(seen))
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
