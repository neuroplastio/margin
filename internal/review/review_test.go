package review

import (
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	uv "github.com/charmbracelet/ultraviolet"
)

const (
	draftAnchor = "^e5" // seeded thread holding unsubmitted text
	convoAnchor = "^b2" // seeded thread with two posted comments
	soloAnchor  = "^d4" // seeded thread with one posted comment
	freshAnchor = "^a1" // paragraph with no thread yet
)

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

// open opens the composer, as c or e does, and checks it holds expect.
func open(t *testing.T, m *model, anchor string, target int, expect string) {
	t.Helper()
	m.openComposer(anchor, target)
	if m.comp == nil {
		t.Fatalf("openComposer(%s, %d) opened nothing: %s", anchor, target, m.status)
	}
	if got := composed(m); !strings.Contains(got, expect) {
		t.Fatalf("the composer holds %q, want %q in it", got, expect)
	}
}

// composed is what the composer's field holds.
func composed(m *model) string { return m.comp.p.text("/text") }

// press gives the review keys, as the reviewer types them.
func press(m *model, keys ...tea.Key) {
	for _, k := range keys {
		m.handleKey(tea.KeyPressMsg(k))
	}
}

// typeText types s, a key a character.
func typeText(m *model, s string) {
	for _, r := range s {
		k := tea.Key{Code: r, Text: string(r)}
		switch r {
		case ' ':
			k = keySpace
		case '\n':
			k = keyEnter
		}
		press(m, k)
	}
}

var (
	keyEsc        = tea.Key{Code: uv.KeyEscape}
	keyEnter      = tea.Key{Code: uv.KeyEnter}
	keyCtrlS      = tea.Key{Code: 's', Mod: uv.ModCtrl}
	keyCtrlG      = tea.Key{Code: 'g', Mod: uv.ModCtrl}
	keyCtrlEnter  = tea.Key{Code: uv.KeyEnter, Mod: uv.ModCtrl}
	keyShiftEnter = tea.Key{Code: uv.KeyEnter, Mod: uv.ModShift}
	keyTab        = tea.Key{Code: uv.KeyTab}
	keySpace      = tea.Key{Code: uv.KeySpace, Text: " "}
	keyBackspace  = tea.Key{Code: uv.KeyBackspace}
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
	i := entryFor(t, m, convoAnchor)
	m.at = cursor{entry: i, comment: 0}
	before := len(m.threads[convoAnchor].posted)

	m.editFocused()
	if m.comp == nil {
		t.Fatalf("e on a posted comment did nothing: %s", m.status)
	}
	if m.comp.target != 0 {
		t.Fatalf("composer target = %d, want comment 0", m.comp.target)
	}
	// The existing text must be in the field, not an empty reply.
	if got := composed(m); !strings.Contains(got, "noisy neighbour") {
		t.Fatalf("the composer holds %q, want the comment being edited", got)
	}

	typeText(m, " Still true.")
	press(m, keyCtrlS)

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
	typeText(m, "March incident, ticket 4412.")
	press(m, keyCtrlS)

	posted := m.threads[soloAnchor].posted
	if len(posted) != before+1 {
		t.Fatalf("posted %d comments, want %d", len(posted), before+1)
	}
	if got := posted[len(posted)-1].body; got != "March incident, ticket 4412." {
		t.Fatalf("new comment = %q", got)
	}
	if m.comp != nil {
		t.Fatal("the composer is still open after submitting")
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
	typeText(m, "Persisted reply.")
	press(m, keyCtrlS)

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

// TestSubmitEmitsAnEvent: a model opened by Run (store set) not only writes
// the posted comment to the thread file but appends a comment.posted line to
// the event log, carrying the thread's anchor, the new comment's index and the
// comment's body text (D15) — the D13 wiring that a listener (the wait
// command) reads.
func TestSubmitEmitsAnEvent(t *testing.T) {
	m := newTestModel(t)
	root := t.TempDir()
	m.store = &threadStore{root: root, docPath: "document.md"}

	open(t, m, soloAnchor, newCommentSlot, "")
	typeText(m, "Notify the agent.")
	press(m, keyCtrlS)

	events, err := readEvents(root)
	if err != nil {
		t.Fatalf("readEvents: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("readEvents returned %d events, want 1", len(events))
	}
	ev := events[0]
	if ev.kind != eventCommentPosted || ev.doc != "document.md" ||
		ev.anchor != soloAnchor || ev.author != "toly" || ev.comment != len(m.threads[soloAnchor].posted)-1 {
		t.Errorf("event = %+v, want comment.posted on %s, comment %d", ev, soloAnchor, len(m.threads[soloAnchor].posted)-1)
	}
	if ev.text != "Notify the agent." {
		t.Errorf("event text = %q, want the submitted body so the agent need not re-read the thread", ev.text)
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
	typeText(m, "In memory only.")
	press(m, keyCtrlS)

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

// --- the cursor -------------------------------------------------------------

// TestEditPlacesCursorAtTheEnd is the 2026-08-09 composer feedback's
// "editing a comment should place the cursor at the end" item: `e` on a posted
// comment opens with the cursor after the comment's text, so typing appends.
func TestEditPlacesCursorAtTheEnd(t *testing.T) {
	m := newTestModel(t)
	original := m.threads[convoAnchor].posted[0].body

	open(t, m, convoAnchor, 0, "Shouldn't be global")
	typeText(m, " tail")
	press(m, keyCtrlS)

	if got := m.threads[convoAnchor].posted[0].body; got != original+" tail" {
		t.Fatalf("edited comment = %q, want %q — typing must append at the very end", got, original+" tail")
	}
}

// TestResumedDraftPlacesCursorAtTheEnd pins the settled contract
// (interaction.md) that a resumed draft puts the cursor after the existing
// text. Submitting the resumed draft posts it.
func TestResumedDraftPlacesCursorAtTheEnd(t *testing.T) {
	m := newTestModel(t)
	original := m.threads[draftAnchor].draft(newCommentSlot)

	open(t, m, draftAnchor, newCommentSlot, "contradicts")
	typeText(m, " tail")
	press(m, keyCtrlS)

	posted := m.threads[draftAnchor].posted
	if len(posted) != 1 {
		t.Fatalf("posted %d comments, want 1 — the resumed draft should submit as a new comment", len(posted))
	}
	if got := posted[0].body; got != parseComment(original)+" tail" {
		t.Fatalf("resumed draft = %q, want %q — typing must append at the very end", got, parseComment(original)+" tail")
	}
}

// TestComposerShowsTheCursor: the terminal's cursor sits in the field while a
// composer is open, after the text.
func TestComposerShowsTheCursor(t *testing.T) {
	m := newTestModel(t)
	m.View()
	open(t, m, freshAnchor, newCommentSlot, "")
	typeText(m, "abc")
	v := m.View()
	if v.Cursor == nil {
		t.Fatal("no cursor while a composer is open")
	}
	if want := m.docX() + gutterW + borderW + len("abc"); v.Cursor.X < want {
		t.Errorf("cursor at column %d, want it after the text (%d or past)", v.Cursor.X, want)
	}
}

// --- the field is only the comment ------------------------------------------

// TestFieldHoldsOnlyTheComment: no instructions, no quoted context. The block
// is on screen above the pane and the keys are in the footer.
func TestFieldHoldsOnlyTheComment(t *testing.T) {
	m := newTestModel(t)
	before := m.threads[convoAnchor].posted[0].body
	open(t, m, convoAnchor, 0, "noisy neighbour")
	if got := composed(m); got != before {
		t.Fatalf("the field holds %q, want the comment's text and nothing else (%q)", got, before)
	}
}

// --- dismissal --------------------------------------------------------------

// TestEnterIsANewline: a plain enter, and shift+enter, break the line; only the
// submit keys submit.
func TestEnterIsANewline(t *testing.T) {
	m := newTestModel(t)
	open(t, m, freshAnchor, newCommentSlot, "")
	typeText(m, "one")
	press(m, keyEnter)
	typeText(m, "two")
	press(m, keyShiftEnter)
	typeText(m, "three")
	if m.comp == nil {
		t.Fatal("enter closed the composer")
	}
	if got := composed(m); got != "one\ntwo\nthree" {
		t.Fatalf("field = %q, want three lines", got)
	}
	press(m, keyCtrlS)
	if got := m.threads[freshAnchor].posted[0].body; got != "one\ntwo\nthree" {
		t.Fatalf("posted %q, want the three lines", got)
	}
}

// TestCtrlEnterSubmits: ctrl+enter is a submit gesture beside ctrl+s.
func TestCtrlEnterSubmits(t *testing.T) {
	m := newTestModel(t)
	open(t, m, freshAnchor, newCommentSlot, "")
	typeText(m, "ship it")
	press(m, keyCtrlEnter)
	if m.comp != nil {
		t.Fatal("ctrl+enter left the composer open")
	}
	tr := m.threads[freshAnchor]
	if tr == nil || len(tr.posted) != 1 || tr.posted[0].body != "ship it" {
		t.Fatalf("ctrl+enter did not post the comment: %+v", tr)
	}
}

// TestEscapeKeepsDraft: esc closes the composer keeping what was written, so
// walking away never loses it.
func TestEscapeKeepsDraft(t *testing.T) {
	m := newTestModel(t)
	open(t, m, freshAnchor, newCommentSlot, "")
	typeText(m, "half a thought")
	press(m, keyEsc)

	if m.comp != nil {
		t.Fatal("esc left the composer open")
	}
	if got := m.threads[freshAnchor].draft(newCommentSlot); got != "half a thought" {
		t.Fatalf("draft = %q, want the unfinished text preserved", got)
	}
	if persisted, _ := loadDraft(freshAnchor, newCommentSlot); persisted != "half a thought" {
		t.Fatalf("draft was not persisted; got %q", persisted)
	}
}

// TestEscapeExitsEdit: esc on an edit keeps the change as a draft, leaving the
// posted comment untouched.
func TestEscapeExitsEdit(t *testing.T) {
	m := newTestModel(t)
	original := m.threads[convoAnchor].posted[0].body

	open(t, m, convoAnchor, 0, "noisy neighbour")
	typeText(m, " WIP")
	press(m, keyEsc)

	tr := m.threads[convoAnchor]
	if got := tr.posted[0].body; got != original {
		t.Fatalf("posted comment was mutated by an esc-exited edit: %q", got)
	}
	if got := tr.draft(0); got != original+" WIP" {
		t.Fatalf("edit draft = %q, want the unsaved edit kept", got)
	}
}

// TestCtrlGDiscards: ctrl+g throws the text away; discarding has to be asked
// for.
func TestCtrlGDiscards(t *testing.T) {
	m := newTestModel(t)
	open(t, m, draftAnchor, newCommentSlot, "contradicts")
	press(m, keyCtrlG)

	if m.comp != nil {
		t.Fatal("ctrl+g left the composer open")
	}
	if got := m.threads[draftAnchor].draft(newCommentSlot); got != "" {
		t.Fatalf("draft = %q, want it thrown away by ctrl+g", got)
	}
}

// TestComposerButtons: the pane's buttons do what the keys do — tab reaches
// them, enter presses the focused one, and a click presses the one under it.
func TestComposerButtons(t *testing.T) {
	m := newTestModel(t)
	open(t, m, freshAnchor, newCommentSlot, "")
	typeText(m, "by button")
	press(m, keyTab, keyEnter) // Submit
	tr := m.threads[freshAnchor]
	if m.comp != nil || tr == nil || len(tr.posted) != 1 {
		t.Fatalf("tab, enter did not submit: comp %v, thread %+v", m.comp, tr)
	}

	m.View()
	open(t, m, soloAnchor, newCommentSlot, "")
	typeText(m, "kept")
	m.View()
	x, y, _, _, ok := m.comp.p.box("draft")
	if !ok {
		t.Fatal("no Keep draft button in the composer")
	}
	m.handleClick(tea.Mouse{X: m.docX() + gutterW + borderW + x, Y: m.paneTop + y - m.scroll})
	if m.comp != nil {
		t.Fatal("clicking Keep draft left the composer open")
	}
	if got := m.threads[soloAnchor].draft(newCommentSlot); got != "kept" {
		t.Fatalf("draft = %q, want the text kept by the button", got)
	}
}

// TestTabStaysInTheComposer: tab walks the field and the buttons and comes
// back round, never leaving the composer without the keyboard.
func TestTabStaysInTheComposer(t *testing.T) {
	m := newTestModel(t)
	open(t, m, freshAnchor, newCommentSlot, "")
	press(m, keyTab, keyTab, keyTab, keyTab) // Submit, Keep draft, Discard, the field
	typeText(m, "back in the field")
	if got := composed(m); got != "back in the field" {
		t.Fatalf("field = %q after tabbing round, want the typing in it", got)
	}
}

// TestPasteIntoTheComposer: a paste lands in the field.
func TestPasteIntoTheComposer(t *testing.T) {
	m := newTestModel(t)
	open(t, m, freshAnchor, newCommentSlot, "")
	m.update(tea.PasteMsg{Content: "pasted\ntext"})
	if got := composed(m); got != "pasted\ntext" {
		t.Fatalf("field = %q, want the paste", got)
	}
}

// TestComposerKeepsKeysFromTheReview: while a composer is open, single-letter
// review keys type, they do not act.
func TestComposerKeepsKeysFromTheReview(t *testing.T) {
	m := newTestModel(t)
	at := m.at
	open(t, m, freshAnchor, newCommentSlot, "")
	typeText(m, "jkqrf")
	if m.quitting {
		t.Fatal("q quit while typing a comment")
	}
	if got := composed(m); got != "jkqrf" {
		t.Fatalf("field = %q, want the letters typed", got)
	}
	_ = at
}

// TestUnchangedEditCreatesNoDraft pins the "no diff, no draft" rule: editing a
// posted comment and exiting keeping a draft, without changing anything, must
// not mark the comment as an unsaved draft — nothing changed, so there is
// nothing to keep. The posted comment is untouched and neither the thread nor
// the draft cache holds text for the target.
func TestUnchangedEditCreatesNoDraft(t *testing.T) {
	m := newTestModel(t)
	original := m.threads[convoAnchor].posted[0].body

	open(t, m, convoAnchor, 0, "noisy neighbour")
	press(m, keyEsc)

	tr := m.threads[convoAnchor]
	if tr.posted[0].body != original {
		t.Fatalf("posted comment was mutated by an unchanged edit: %q", tr.posted[0].body)
	}
	if got := tr.draft(0); got != "" {
		t.Fatalf("an unchanged edit left a draft %q — no diff means no draft", got)
	}
	if got, _ := loadDraft(convoAnchor, 0); got != "" {
		t.Fatalf("an unchanged edit persisted draft %q to disk", got)
	}
	if m.status != "no changes" {
		t.Fatalf("status = %q, want %q", m.status, "no changes")
	}
}

// TestUnchangedEditClearsStaleDraft: an edit that reverts to exactly the posted
// text clears whatever draft the target held, in the thread and on disk alike.
func TestUnchangedEditClearsStaleDraft(t *testing.T) {
	m := newTestModel(t)
	original := m.threads[convoAnchor].posted[0].body
	tr := m.threads[convoAnchor]
	tr.setDraft(0, original)
	if err := saveDraft(convoAnchor, 0, original); err != nil {
		t.Fatal(err)
	}

	open(t, m, convoAnchor, 0, "noisy neighbour")
	press(m, keyEsc)

	if got := tr.draft(0); got != "" {
		t.Fatalf("a revert-to-posted edit left draft %q behind", got)
	}
	if got, _ := loadDraft(convoAnchor, 0); got != "" {
		t.Fatalf("a revert-to-posted edit left draft %q on disk", got)
	}
	if tr.posted[0].body != original {
		t.Fatalf("posted comment was mutated: %q", tr.posted[0].body)
	}
}

// --- cancel-early must not leave an empty thread ---------------------------

// TestCancelNewCommentCreatesNoThread: `c` on a block with no thread and an
// immediate esc (nothing typed, so nothing to keep) must not leave a "no
// comments yet" thread behind. Focus goes back to the block the aborted
// comment was about.
func TestCancelNewCommentCreatesNoThread(t *testing.T) {
	m := newTestModel(t)
	open(t, m, freshAnchor, newCommentSlot, "")
	press(m, keyEsc)

	if _, ok := m.threads[freshAnchor]; ok {
		t.Fatalf("cancel left an empty thread for %s behind", freshAnchor)
	}
	for i, e := range m.entries {
		if e.thread != nil && e.thread.anchor == freshAnchor {
			t.Fatalf("cancel left a thread row at entry %d", i)
		}
	}
	if got := m.entries[m.at.entry].b.anchor; got != freshAnchor {
		t.Fatalf("focus on %s, want it back on the block %s", got, freshAnchor)
	}
}

// TestSubmitEmptyNewCommentCreatesNoThread: submitting an empty field says
// "nothing to submit" and must be as thread-free as a cancel.
func TestSubmitEmptyNewCommentCreatesNoThread(t *testing.T) {
	m := newTestModel(t)
	open(t, m, freshAnchor, newCommentSlot, "")
	press(m, keyCtrlEnter)

	if _, ok := m.threads[freshAnchor]; ok {
		t.Fatalf("submitting an empty field left a thread for %s behind", freshAnchor)
	}
}

// TestDiscardNewCommentCreatesNoThread: the discard gesture must be thread-free
// too.
func TestDiscardNewCommentCreatesNoThread(t *testing.T) {
	m := newTestModel(t)
	open(t, m, freshAnchor, newCommentSlot, "")
	typeText(m, "never mind")
	press(m, keyCtrlG)

	if _, ok := m.threads[freshAnchor]; ok {
		t.Fatalf("discard left a thread for %s behind", freshAnchor)
	}
}

// TestDraftKeepsFreshThread: the drop must only fire on nothing committed —
// a kept draft is content, and its thread stays.
func TestDraftKeepsFreshThread(t *testing.T) {
	m := newTestModel(t)
	open(t, m, freshAnchor, newCommentSlot, "")
	typeText(m, "half a thought")
	press(m, keyEsc)

	tr := m.threads[freshAnchor]
	if tr == nil {
		t.Fatal("a kept draft dropped its thread")
	}
	if got := tr.draft(newCommentSlot); got != "half a thought" {
		t.Fatalf("draft = %q, want the unfinished text preserved", got)
	}
}

// TestCancelEditKeepsThread: the drop must not fire on a cancelled edit — the
// thread carries the comment being edited.
func TestCancelEditKeepsThread(t *testing.T) {
	m := newTestModel(t)
	original := m.threads[soloAnchor].posted[0].body

	open(t, m, soloAnchor, 0, "Where does thirty")
	press(m, keyEsc)

	if m.threads[soloAnchor] == nil {
		t.Fatal("a cancelled edit dropped its thread")
	}
	if got := m.threads[soloAnchor].posted[0].body; got != original {
		t.Fatalf("posted comment mutated by a cancelled edit: %q", got)
	}
}

// --- blur -------------------------------------------------------------------

// TestBlurKeepsDraft is the core bet: losing focus drops the editor and the
// half-written text survives it.
func TestBlurKeepsDraft(t *testing.T) {
	m := newTestModel(t)
	open(t, m, draftAnchor, newCommentSlot, "contradicts")
	typeText(m, " and the breaker section too")
	m.blur(nil)

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
	typeText(m, " WIP")
	m.blur(nil)

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

// TestClickAwayBlurs covers the mouse path out of a live editor: the text is
// kept and the click's spot takes focus.
func TestClickAwayBlurs(t *testing.T) {
	m := newTestModel(t)
	m.View()
	open(t, m, draftAnchor, newCommentSlot, "contradicts")
	m.View()

	m.handleClick(tea.Mouse{X: 4, Y: 0})
	if m.comp != nil {
		t.Fatal("composer still open after clicking away")
	}
	if m.threads[draftAnchor].draft(newCommentSlot) == "" {
		t.Fatal("clicking away lost the draft")
	}
}

// TestClickInsidePaneDoesNotBlur: clicks in the editor belong to the editor —
// they place its cursor.
func TestClickInsidePaneDoesNotBlur(t *testing.T) {
	m := newTestModel(t)
	m.View()
	open(t, m, draftAnchor, newCommentSlot, "contradicts")
	m.View()

	m.handleClick(tea.Mouse{X: m.docX() + gutterW + borderW + 2, Y: m.paneTop - m.scroll})
	if m.pendingOpen != nil {
		t.Fatalf("a click inside the pane queued a focus change to %+v", m.pendingOpen)
	}
	if m.comp == nil {
		t.Fatal("a click inside the pane closed the editor")
	}
	// The click put the cursor near the start of the field's first line:
	// typing lands there, not at the end.
	typeText(m, "X")
	if got := composed(m); strings.HasSuffix(got, "X") || !strings.Contains(got, "X") {
		t.Fatalf("field = %q, want the X typed where the click put the cursor", got)
	}
}

// TestViewRequestsAllMotionMouse pins the hover fix (2026-08-10.18): the view
// must ask the terminal for any-event tracking (1003), not cell-motion (1002).
// Under 1002 the terminal only reports motion while a button is held, so a bare
// pointer move — plain hover — never reached handleMotion at all.
func TestViewRequestsAllMotionMouse(t *testing.T) {
	m := newTestModel(t)
	if got := m.View().MouseMode; got != tea.MouseModeAllMotion {
		t.Fatalf("MouseMode = %v, want %v (any-event tracking, so hover works without a button)", got, tea.MouseModeAllMotion)
	}
}

// TestHoverLightsOnBareMotion is the behaviour the 2026-08-10-hover-without-lmb
// feedback asked for: a motion event carrying no button — the terminal's 1003
// hover report — lights hoveredEntry on the block under the pointer, and a
// motion over nothing clears it.
func TestHoverLightsOnBareMotion(t *testing.T) {
	m := newTestModel(t)
	m.View()

	i := blockEntryFor(t, m, draftAnchor)
	hover := tea.Mouse{X: 4, Y: m.spans[i].start - m.scroll} // Button zero: no drag
	m.handleMotion(hover)
	if m.hoveredEntry != i {
		t.Fatalf("bare motion over entry %d left hoveredEntry = %d, want %d", i, m.hoveredEntry, i)
	}

	m.handleMotion(tea.Mouse{X: 4, Y: 1 << 20})
	if m.hoveredEntry != -1 {
		t.Fatalf("bare motion over empty space left hoveredEntry = %d, want -1", m.hoveredEntry)
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

// TestPlainMarkdownStripsInlineMarkup pins plainMarkdown: an excerpt of a
// comment's body must show its text, not its markup — the collapsed thread's
// summary would otherwise read "**store**" instead of "store".
func TestPlainMarkdownStripsInlineMarkup(t *testing.T) {
	got := plainMarkdown("The **store** has `care` via [the runbook](https://x/).")
	want := "The store has care via the runbook."
	if got != want {
		t.Errorf("plainMarkdown = %q, want %q", got, want)
	}
}

// TestWrapCommentKeepsLineBreaksAndParagraphs pins the
// comments-markdown-formatting shape at the wrapper level: each source line
// stays its own rendered line, a blank source line becomes a blank rendered
// line (a paragraph break), and a long line still wraps to the measure — a
// comment must never collapse into one run-on paragraph again.
func TestWrapCommentKeepsLineBreaksAndParagraphs(t *testing.T) {
	out := wrapComment("one\ntwo\n\nthree four five six seven eight", 20, textStyle)
	if len(out) != 5 {
		t.Fatalf("wrapComment returned %d lines, want 5 (one, two, blank, wrapped paragraph): %v", len(out), out)
	}
	for i, want := range []string{"one", "two"} {
		if got := ansiRe.ReplaceAllString(out[i], ""); got != want {
			t.Errorf("line %d = %q, want %q", i, got, want)
		}
	}
	if out[2] != "" {
		t.Errorf("line 2 = %q, want a blank paragraph break", out[2])
	}
	if got := ansiRe.ReplaceAllString(strings.Join(out[3:], " "), ""); got != "three four five six seven eight" {
		t.Errorf("paragraph 2 = %q, want every word kept across the wrap", got)
	}
	for _, l := range out[3:] {
		if width := len(ansiRe.ReplaceAllString(l, "")); width > 20 {
			t.Errorf("line %q is %d columns wide, wider than the measure 20", l, width)
		}
	}
}

// TestWrapCommentRendersInlineMarkup pins that a comment body's inline markup
// reaches the screen styled, never as literal ** and ` characters — the same
// RENDER-06 treatment the document's paragraphs already have.
func TestWrapCommentRendersInlineMarkup(t *testing.T) {
	out := wrapComment("The **store** needs `care` — see the [runbook](https://x/).", 40, textStyle)
	joined := ansiRe.ReplaceAllString(strings.Join(out, " "), "")
	for _, marker := range []string{"**", "`", "](", "https://x/"} {
		if strings.Contains(joined, marker) {
			t.Errorf("raw markup %q survived into a comment body: %q", marker, joined)
		}
	}
	for _, word := range []string{"store", "care", "runbook"} {
		if !strings.Contains(joined, word) {
			t.Errorf("word %q went missing from a comment body: %q", word, joined)
		}
	}
	if !ansiRe.MatchString(strings.Join(out, " ")) {
		t.Errorf("comment body carries no ANSI styling, want bold/code/link colours")
	}
}

// TestCollapsedSummaryStripsMarkup pins that a collapsed thread's one-line
// excerpt shows a comment's text, not its raw markup — a comment that opens
// with **bold** must not read "**bold**" in the gutter while the expanded
// thread renders it styled.
func TestCollapsedSummaryStripsMarkup(t *testing.T) {
	th := &thread{
		anchor: "^x",
		posted: []comment{{author: "toly", body: "**Ship it** — no blockers.", at: time.Now()}},
	}
	got := th.summary(false)
	if strings.Contains(got, "**") {
		t.Errorf("collapsed summary %q carries raw bold markup", got)
	}
	if !strings.Contains(got, "Ship it") {
		t.Errorf("collapsed summary %q lost the comment text", got)
	}
	th.setDraft(newCommentSlot, "See `code` in the draft")
	got = th.summary(false)
	if strings.Contains(got, "`") {
		t.Errorf("collapsed draft summary %q carries raw code markup", got)
	}
	if !strings.Contains(got, "code in the draft") {
		t.Errorf("collapsed draft summary %q lost the draft text", got)
	}
}

// TestAppendCommentsRendersMarkdownAndKeepsLineBreaks pins the
// comments-markdown-formatting change at the thread-rendering level: a
// comment body with a paragraph break renders as two separated lines rather
// than one run-on paragraph, and its **bold** / `code` markup reaches the
// screen styled, not literally.
func TestAppendCommentsRendersMarkdownAndKeepsLineBreaks(t *testing.T) {
	m := seedModel()
	m.subspans = make(map[cursor]span)
	i := entryFor(t, m, convoAnchor)
	thr := m.threads[convoAnchor]
	thr.posted[0].body = "First **bold** line.\n\nSecond line with `code`."
	w := 80

	lines := m.appendComments(nil, i, thr, w, 0, 0)
	if len(lines) < 4 {
		t.Fatalf("appendComments returned %d lines, want at least 4 (head, paragraph 1, blank, paragraph 2): %q", len(lines), lines)
	}
	joined := ansiRe.ReplaceAllString(strings.Join(lines, " "), "")
	for _, marker := range []string{"**", "`"} {
		if strings.Contains(joined, marker) {
			t.Errorf("raw markup %q survived into a comment body: %q", marker, joined)
		}
	}
	if !strings.Contains(ansiRe.ReplaceAllString(lines[1], ""), "First bold line.") {
		t.Errorf("paragraph 1 = %q, want \"First bold line.\" on its own line", lines[1])
	}
	if strings.TrimSpace(ansiRe.ReplaceAllString(lines[2], "")) != "" {
		t.Errorf("line between paragraphs = %q, want a blank paragraph break", lines[2])
	}
	if !strings.Contains(ansiRe.ReplaceAllString(lines[3], ""), "Second line with") {
		t.Errorf("paragraph 2 = %q, want \"Second line with code.\"", lines[3])
	}
}

// TestHighlightCodeAppliesColourForARecognisedLanguage is the render-level
// regression test for RENDER-02: a fenced code block whose language chroma
// recognises must come back styled — some ANSI escape on at least one
// line — while every source line's own text (words, indentation) survives
// untouched underneath the colour. Tabs are expanded to spaces first
// (2026-08-12 code-block-tabs feedback), so the de-styled output is the
// source with each tab at its 8-column stop, not the raw tab byte.
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
	want := []string{"func retry(n int) error {", "        return nil", "}"}
	for i, l := range out {
		plain := ansiRe.ReplaceAllString(l, "")
		if plain != want[i] {
			t.Errorf("line %d = %q once de-styled, want %q (source with tabs expanded)", i, plain, want[i])
		}
	}
}

// TestHighlightCodeExpandsTabs pins the code-block-tabs fix: a tab inside a
// fenced block must render as spaces to the next 8-column stop, not as a
// literal tab the terminal resolves against absolute screen stops. The width
// of the expanded line is what the width maths see, so visualWidth of the
// de-styled output matches the column the terminal will draw the text at.
func TestHighlightCodeExpandsTabs(t *testing.T) {
	lines := []string{"\tx", "\t\t\ty", "a\tb"}
	out := highlightCode(lines, "go")
	want := []string{
		strings.Repeat(" ", 8) + "x",
		strings.Repeat(" ", 24) + "y",
		"a" + strings.Repeat(" ", 7) + "b",
	}
	for i, l := range out {
		plain := ansiRe.ReplaceAllString(l, "")
		if plain != want[i] {
			t.Errorf("line %d = %q once de-styled, want %q", i, plain, want[i])
		}
		if strings.ContainsRune(plain, '\t') {
			t.Errorf("line %d still carries a tab: %q", i, plain)
		}
	}
}

// TestExpandTabsLine pins the expansion rule itself: a tab advances to the
// next multiple of tabWidth from the line's own start, and the column resets
// at a newline. ANSI escapes are skipped while counting, so an already-styled
// line expands the same way a plain one does.
func TestExpandTabsLine(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"", ""},
		{"no tabs", "no tabs"},
		{"\tx", strings.Repeat(" ", 8) + "x"},
		{"x\t\ty", "x" + strings.Repeat(" ", 15) + "y"},
		{"a\tb\n\tc", "a" + strings.Repeat(" ", 7) + "b\n" + strings.Repeat(" ", 8) + "c"},
		{"\x1b[31m\tx\x1b[0m", "\x1b[31m" + strings.Repeat(" ", 8) + "x\x1b[0m"},
	}
	for _, c := range cases {
		if got := expandTabsLine(c.in, 8); got != c.want {
			t.Errorf("expandTabsLine(%q, 8) = %q, want %q", c.in, got, c.want)
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

// TestRenderCodeBlockExpandsTabs is the end-to-end half of the
// code-block-tabs fix: a document whose fenced Go block is tab-indented must
// render with spaces (a tab at its 8-column stop), never a literal tab — a
// literal tab would be resolved by the terminal against absolute screen
// stops, so the indentation would misalign past the gutter, and margin's own
// width maths would under-measure the line. The gutter carries the tab-free
// line; its de-styled width is the expanded width, matching the columns the
// terminal will actually draw.
func TestRenderCodeBlockExpandsTabs(t *testing.T) {
	src := "```go\nfunc f() {\n\tx := 1\n\t\treturn x\n}\n```\n"
	m := newModelAt("sample.md", parseDoc([]byte(src)), nil)
	m.w, m.h = 40, 60
	lines := m.render()
	content := []string{"func f() {", strings.Repeat(" ", 8) + "x := 1", strings.Repeat(" ", 16) + "return x", "}"}
	found := 0
	for _, l := range lines {
		plain := ansiRe.ReplaceAllString(l, "")
		if strings.ContainsRune(plain, '\t') {
			t.Errorf("rendered line still carries a literal tab: %q", plain)
		}
		for _, want := range content {
			if strings.Contains(plain, want) {
				found++
			}
		}
	}
	if found != len(content) {
		t.Errorf("rendered code block contains %d of %d expected lines (some tab content was lost)", found, len(content))
	}
}

// TestRenderTableAlignsColumnsAtNaturalWidth pins renderTable's basic shape:
// header above a rule above the rows, every line the same de-styled width
// (columns line up), and alignment honoured per column. Widths are the
// content's own natural widths — a table is no longer narrowed to fit a
// measure, since the 2026-08-10 viewport-width feedback replaced narrowing
// with horizontal scroll.
func TestRenderTableAlignsColumnsAtNaturalWidth(t *testing.T) {
	tb := &tableBlock{
		header: []string{"Name", "Age"},
		rows:   [][]string{{"Alice", "30"}, {"Bob", "5"}},
		aligns: []tableAlign{alignLeft, alignRight},
	}
	out := renderTable(tb, textStyle)
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

// TestRenderTableStylesInlineMarkupInCells pins the 2026-08-10
// inline-code-in-tables feedback: a cell's **bold**, `code` and [links](url)
// render styled inside the column layout — each run carries fragStyle's
// treatment the way a paragraph's runs do — while the rows still line up,
// because widths are measured on the stripped text and padding fills the
// rest with plain spaces.
func TestRenderTableStylesInlineMarkupInCells(t *testing.T) {
	tb := &tableBlock{
		header: []string{"Command", "Notes"},
		rows: [][]string{
			{"margin", "Runs `in` a **TUI**, [really](https://example.com/x)"},
			{"ls", "plain"},
		},
		aligns: []tableAlign{alignLeft, alignRight},
	}
	out := renderTable(tb, textStyle)
	if len(out) != 4 {
		t.Fatalf("renderTable returned %d line(s), want 4 (header, rule, 2 rows)", len(out))
	}
	row := out[2]
	for _, want := range []string{
		codeStyle.Render("in"),
		textStyle.Bold(true).Render("TUI"),
		linkStyle.Render("really"),
	} {
		if !strings.Contains(row, want) {
			t.Errorf("row = %q, want the cell's inline markup styled (%q)", row, want)
		}
	}
	// The styled row must still line up with the others once ANSI is
	// stripped — styling adds bytes, not columns.
	widths := map[int]int{}
	for i, l := range out {
		widths[i] = len([]rune(ansiRe.ReplaceAllString(l, "")))
	}
	for i, w := range widths {
		if w != widths[0] {
			t.Errorf("line %d is %d runes wide once de-styled, want %d like every other row: %q", i, w, widths[0], out)
		}
	}
}

// TestTableScrollsHorizontally: a table wider than the terminal keeps its
// natural column widths and scrolls horizontally with H/L — the code-block
// treatment the 2026-08-10 viewport-width feedback asked for in place of
// narrowing columns to fit. The table renders at its natural width (nothing
// shaved), and once an offset is set the render loop clips the rows through
// scrollCodeLine exactly as it clips a code block, so scrolling right pushes
// the leading column off the left edge instead of squeezing it.
func TestTableScrollsHorizontally(t *testing.T) {
	wide := &tableBlock{
		header: []string{"AlphabetColumn", "Notes"},
		rows:   [][]string{{"alpha", strings.Repeat("n", 40)}},
	}
	m := newModel([]block{{kind: blockTable, table: wide, anchor: "^t"}}, nil)
	m.w, m.h = 40, 60
	contentW := m.contentWidth()
	if m.codeScroll["^t"] != 0 {
		t.Fatalf("initial table scroll offset = %d, want 0", m.codeScroll["^t"])
	}

	// The long Notes cell overflows the content width, so there is something
	// to scroll to.
	natural := renderTable(wide, textStyle)
	if w := visualWidth(natural[2]); w <= contentW {
		t.Fatalf("table's widest line is %d cols, need an overflow of the %d-col content width", w, contentW)
	}

	// At offset 0 nothing is clipped off the left: the header's first column
	// is on screen at its natural width.
	lines := m.render()
	if plain := string([]rune(ansiRe.ReplaceAllString(lines[0], ""))[gutterW:]); !strings.Contains(plain, "AlphabetColumn") {
		t.Errorf("offset 0 header = %q, want the natural-width leading column", plain)
	}

	// L scrolls right; the rendered header is now the natural header clipped
	// at the offset — the columns did not narrow to fit.
	m.handleKey(tea.KeyPressMsg(tea.Key{Code: 'L', Text: "L"}))
	off := m.codeScroll["^t"]
	if off <= 0 {
		t.Fatalf("after 'L' table scroll offset = %d, want > 0", off)
	}
	lines = m.render()
	got := string([]rune(ansiRe.ReplaceAllString(lines[0], ""))[gutterW:])
	want := ansiRe.ReplaceAllString(scrollCodeLine(natural[0], off, contentW), "")
	if got != want {
		t.Errorf("scrolled header = %q, want the natural header clipped at offset %d: %q", got, off, want)
	}

	// H scrolls back to the left edge.
	m.handleKey(tea.KeyPressMsg(tea.Key{Code: 'H', Text: "H"}))
	if off := m.codeScroll["^t"]; off != 0 {
		t.Fatalf("after 'H' table scroll offset = %d, want 0", off)
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

// TestContentWidthAdaptsToTheTerminal pins the 2026-08-10 viewport-width
// feedback's first part: contentWidth is the terminal's own width (minus the
// gutter), not a fixed 76-column measure, so a wide terminal reads at its
// full width instead of parking the extra columns. The floor still keeps a
// very narrow terminal readable.
func TestContentWidthAdaptsToTheTerminal(t *testing.T) {
	m := newTestModel(t)

	m.w = 200
	if w := m.contentWidth(); w != 200-2*gutterW {
		t.Fatalf("contentWidth on a 200-col terminal = %d, want %d (the full width, not the old 76 measure)", w, 200-2*gutterW)
	}

	m.w = 100
	if w := m.contentWidth(); w != 100-2*gutterW {
		t.Fatalf("contentWidth on a 100-col terminal = %d, want %d", w, 100-2*gutterW)
	}

	m.w = 10
	if w := m.contentWidth(); w != minPaneCol {
		t.Fatalf("contentWidth on a 10-col terminal = %d, want the %d-col floor", w, minPaneCol)
	}
}

// TestFrontmatterScrollsHorizontally: H/L on a focused frontmatter block scroll
// a long field horizontally, the code-block treatment the frontmatter feedback
// asked for in place of the ellipsis truncation (moved from l/h to H/L by the
// 2026-08-10 viewport-width feedback).
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

	m.handleKey(tea.KeyPressMsg(tea.Key{Code: 'L', Text: "L"}))
	if off := m.codeScroll["^fm"]; off <= 0 {
		t.Errorf("after 'L' frontmatter scroll offset = %d, want > 0", off)
	}

	m.handleKey(tea.KeyPressMsg(tea.Key{Code: 'H', Text: "H"}))
	if off := m.codeScroll["^fm"]; off != 0 {
		t.Errorf("after 'H' frontmatter scroll offset = %d, want 0", off)
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

// TestCollapsedOpenThreadHasDistinctMarker: the 2026-08-10 agent-loop feedback
// — "what a gutter icon on a block actually means" (cursor vs. an active
// comment vs. a reviewed/flagged mark) was not obvious, because the collapsed
// thread's dim │ read as the mark rule's │. A collapsed open thread's marker
// is now a right-pointing triangle, never the mark rule or the focus bar.
func TestCollapsedOpenThreadHasDistinctMarker(t *testing.T) {
	m := newTestModel(t)
	m.at = cursor{entry: blockEntryFor(t, m, freshAnchor), comment: commentNone}
	lines := m.render()

	i := entryFor(t, m, convoAnchor)
	line := lines[m.spans[i].start]
	if !strings.Contains(line, dimStyle.Render("▸ ")) {
		t.Errorf("collapsed open thread line = %q, want the ▸ thread marker", line)
	}
	for _, other := range []string{okStyle.Render("│"), flagStyle.Render("│"), focusStyle.Render("▌")} {
		if strings.Contains(line, other) {
			t.Errorf("collapsed thread line = %q carries %q — the thread marker must not read as it", line, other)
		}
	}
}

// TestCollapsedResolvedThreadShowsCheckmark: THREAD-02's marker swap — a
// resolved thread's collapsed line trades the "▸" marker for a
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

// TestLineRefPrefixTypesAfterThePrefix: the 2026-08-09 composer feedback
// asked that commenting with a visual selection (or a line dive) start typing
// immediately after the auto-appended L12-18: reference. V then j selects a
// range, c prepends the reference and opens the composer with the cursor past
// it — asserted by the body: the typed text lands after the reference.
func TestLineRefPrefixTypesAfterThePrefix(t *testing.T) {
	m := newModelAt("sample.md", parseDoc([]byte(sampleDoc)), nil)
	isolateDrafts(t)
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
	if m.comp == nil {
		t.Fatalf("c with a selection opened nothing: %s", m.status)
	}
	typeText(m, "ship it")
	press(m, keyCtrlS)

	posted := m.threads[a].posted
	if len(posted) != 1 {
		t.Fatalf("posted %d comments, want 1", len(posted))
	}
	if want := strings.TrimSpace(wantPrefix + "ship it"); posted[0].body != want {
		t.Errorf("comment body = %q, want %q — the cursor must have sat past the prefix", posted[0].body, want)
	}
}

// TestShiftedKeysReachTheField is the regression for capitals vanishing: a
// shifted key, as the Kitty keyboard protocol reports it, types its character.
func TestShiftedKeysReachTheField(t *testing.T) {
	m := newTestModel(t)
	open(t, m, freshAnchor, newCommentSlot, "")
	press(m,
		tea.Key{Code: 'o', Mod: uv.ModShift, Text: "O"},
		tea.Key{Code: 'd', Mod: uv.ModShift, Text: "D"},
		tea.Key{Code: '1', Mod: uv.ModShift, ShiftedCode: '!', Text: "!"})
	if got := composed(m); got != "OD!" {
		t.Fatalf("field = %q, want the shifted characters", got)
	}
}

// TestComposerBoxDoesNotRewrap: the box around the composer is sized so its
// content area is exactly the pane's width, and the pane's rows land in it
// verbatim — lipgloss must not re-wrap them, or the cursor would sit off the
// text.
func TestComposerBoxDoesNotRewrap(t *testing.T) {
	m := newTestModel(t)
	open(t, m, freshAnchor, newCommentSlot, "")
	typeText(m, "one two three four five six seven eight nine ten eleven twelve thirteen fourteen fifteen sixteen seventeen eighteen nineteen")
	m.View()

	span := m.spans[m.at.entry]
	if got, want := span.end-span.start+1, m.paneH+3; got != want {
		t.Fatalf("composer box occupies %d rows, want %d — the box is re-wrapping the pane's lines", got, want)
	}
	lines := m.render()
	top := ansiRe.ReplaceAllString(lines[span.start], "")
	if inner := strings.Count(top, "─"); inner != m.paneW {
		t.Fatalf("box content area is %d columns, the pane is %d", inner, m.paneW)
	}
	pane := strings.Split(m.comp.p.draw(m.paneW).Plain(), "\n")
	for j := 0; j < m.paneH; j++ {
		row := ansiRe.ReplaceAllString(lines[span.start+1+j], "")
		row = strings.TrimPrefix(row, strings.Repeat(" ", gutterW)+"│")
		row = strings.TrimSuffix(row, "│")
		if got := strings.TrimRight(row, " "); got != strings.TrimRight(pane[j], " ") {
			t.Errorf("box row %d = %q, pane row = %q", j, got, pane[j])
		}
	}
}

// TestComposerShowsConversationOnNewComment pins the 2026-08-09 feedback:
// adding a comment to a thread that already has one shows the conversation
// above the editor, so the reply reads as a reply rather than a parallel
// thread. The pane's rows, the mouse routing (paneTop) and the comments'
// click targets (subspans) must all agree on where the editor starts.
func TestComposerShowsConversationOnNewComment(t *testing.T) {
	m := newTestModel(t)
	open(t, m, convoAnchor, newCommentSlot, "")
	typeText(m, "replying with the thread on screen")

	m.View()
	if m.paneLead == 0 {
		t.Fatal("new comment on an existing thread renders the composer alone — the conversation should show above it")
	}
	lines := m.render()
	plain := make([]string, len(lines))
	for i, l := range lines {
		plain[i] = ansiRe.ReplaceAllString(l, "")
	}

	span := m.spans[m.at.entry]
	above := strings.Join(plain[span.start:m.paneTop], "\n")
	for _, want := range []string{"Shouldn't be global", "Changed to per-endpoint budgets"} {
		if !strings.Contains(above, want) {
			t.Errorf("conversation above the composer is missing %q:\n%s", want, above)
		}
	}
	if rule := strings.Trim(plain[m.paneTop-1], " │"); !strings.Contains(rule, "───") {
		t.Errorf("no separator rule ahead of the composer, got %q", plain[m.paneTop-1])
	}

	// paneTop points at the pane's first row, past the conversation.
	pane := strings.Split(m.comp.p.draw(m.paneW).Plain(), "\n")
	row := strings.TrimPrefix(plain[m.paneTop], strings.Repeat(" ", gutterW)+"│")
	row = strings.TrimSuffix(row, "│")
	if got, want := strings.TrimRight(row, " "), strings.TrimRight(pane[0], " "); got != want {
		t.Errorf("paneTop row = %q, pane row 0 = %q — mouse routing would land off the text", got, want)
	}

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
// edit's text is already in the field, so the box stays composer-only and the
// conversation does not render twice.
func TestComposerBoxStaysSoloWhenEditing(t *testing.T) {
	m := newTestModel(t)
	open(t, m, convoAnchor, 0, "Shouldn't be global")

	m.View()
	if m.paneLead != 0 {
		t.Fatalf("editing renders %d lead lines, want the composer-only box", m.paneLead)
	}
	span := m.spans[m.at.entry]
	if got, want := span.end-span.start+1, m.paneH+3; got != want {
		t.Fatalf("edit box occupies %d rows, want %d (borders, the pane, the blank after)", got, want)
	}
}

// TestComposerBoxStaysSoloOnFreshThread: with no conversation to show, a new
// comment's box holds the composer alone.
func TestComposerBoxStaysSoloOnFreshThread(t *testing.T) {
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
	typeText(m, "fresh comment body")
	press(m, keyCtrlS)

	if m.at.comment != 0 {
		t.Fatalf("focus after submitting new comment = %d, want 0", m.at.comment)
	}

	m.editFocused()
	if m.comp == nil {
		t.Fatal("editFocused() after comment exit did not open composer")
	}
	if m.comp.target != 0 {
		t.Fatalf("composer target = %d, want 0", m.comp.target)
	}

	typeText(m, " updated")
	press(m, keyCtrlS)

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
	if !strings.Contains(ansiRe.ReplaceAllString(linesFocused[1], ""), "Shouldn't be global") {
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

// TestCodeBlockHorizontalScroll verifies that H and L scroll code blocks
// horizontally — the 2026-08-10 viewport-width feedback moved the binding
// from l/h (dive/surface) to H/L so the double meaning is gone.
func TestCodeBlockHorizontalScroll(t *testing.T) {
	doc := []block{
		{
			kind: blockCode,
			anchor: "^code1",
			lines: []string{"func veryLongFunctionNameThatExceedsTheMeasureWidth(ctx context.Context, req *Request, opts *Options) (*Response, error, *Result)"},
			lang: "go",
		},
	}
	m := newModel(doc, nil)
	m.w, m.h = 100, 60
	m.at = cursor{entry: 0, comment: commentNone}
	anchor := "^code1"

	// 1. Initially offset is 0
	if off := m.codeScroll[anchor]; off != 0 {
		t.Fatalf("initial code scroll offset = %d, want 0", off)
	}

	// 2. Press L to scroll right
	m.handleKey(tea.KeyPressMsg(tea.Key{Code: 'L', Text: "L"}))
	if off := m.codeScroll[anchor]; off <= 0 {
		t.Errorf("after 'L' press code scroll offset = %d, want > 0", off)
	}

	// 3. Press H to scroll left back to 0
	m.handleKey(tea.KeyPressMsg(tea.Key{Code: 'H', Text: "H"}))
	if off := m.codeScroll[anchor]; off != 0 {
		t.Errorf("after 'H' press code scroll offset = %d, want 0", off)
	}
}

// TestLCodeBlockNoLongerScrolls: the flip side of the H/L re-binding — l on a
// code block is dive now, and with no thread and no line dive to enter it is
// a quiet no-op that leaves the code block's horizontal offset untouched.
func TestLCodeBlockNoLongerScrolls(t *testing.T) {
	doc := []block{
		{
			kind: blockCode,
			anchor: "^code1",
			lines: []string{"func veryLongFunctionNameThatExceedsTheMeasureWidth(ctx context.Context, req *Request, opts *Options) (*Response, error, *Result)"},
			lang: "go",
		},
	}
	m := newModel(doc, nil)
	m.w, m.h = 100, 60
	m.at = cursor{entry: 0, comment: commentNone}
	anchor := "^code1"
	if off := m.codeScroll[anchor]; off != 0 {
		t.Fatalf("initial code scroll offset = %d, want 0", off)
	}

	m.handleKey(tea.KeyPressMsg(tea.Key{Code: 'l', Text: "l"}))
	if off := m.codeScroll[anchor]; off != 0 {
		t.Errorf("'l' scrolled the code block to offset %d — l must not scroll, H/L own that", off)
	}
	if m.status == "" {
		t.Error("'l' on a code block with no thread left no status hint")
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
