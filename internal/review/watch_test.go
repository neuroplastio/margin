package review

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
)

func TestReloadThreadsAddsANewThreadWrittenExternally(t *testing.T) {
	m := newTestModel(t)
	root := t.TempDir()
	m.store = &threadStore{root: root, docPath: "document.md"}

	// freshAnchor has no thread yet — an agent writing one directly to disk
	// is exactly the case live reload exists for.
	external := &thread{anchor: freshAnchor, quote: "quoted text", posted: []comment{
		{author: "agent", body: "Wrote this straight to disk.", at: time.Now()},
	}}
	if err := writeThreadFile(root, "document.md", external); err != nil {
		t.Fatalf("writeThreadFile: %v", err)
	}

	if err := m.reloadThreads(); err != nil {
		t.Fatalf("reloadThreads: %v", err)
	}

	got, ok := m.threads[freshAnchor]
	if !ok {
		t.Fatalf("reloadThreads did not pick up the externally written thread")
	}
	if len(got.posted) != 1 || got.posted[0].body != "Wrote this straight to disk." {
		t.Fatalf("got.posted = %+v, want the externally written comment", got.posted)
	}
}

func TestReloadThreadsMergesAnAppendedReplyIntoAnExistingThread(t *testing.T) {
	m := newTestModel(t)
	root := t.TempDir()
	m.store = &threadStore{root: root, docPath: "document.md"}

	existing := m.threads[soloAnchor]
	before := len(existing.posted)

	// Simulate an agent appending a reply to the thread already open in
	// memory: write the extended thread straight to disk, as it would be
	// found on the filesystem, without going through m.store.save.
	extended := &thread{
		anchor: existing.anchor,
		quote:  existing.quote,
		posted: append(append([]comment{}, existing.posted...),
			comment{author: "agent", body: "Addressed.", at: time.Now()}),
	}
	if err := writeThreadFile(root, "document.md", extended); err != nil {
		t.Fatalf("writeThreadFile: %v", err)
	}

	if err := m.reloadThreads(); err != nil {
		t.Fatalf("reloadThreads: %v", err)
	}

	got := m.threads[soloAnchor]
	if len(got.posted) != before+1 {
		t.Fatalf("posted = %d comments, want %d", len(got.posted), before+1)
	}
	if last := got.posted[len(got.posted)-1]; last.body != "Addressed." {
		t.Fatalf("last comment = %q, want the agent's reply", last.body)
	}
}

// TestReloadThreadsMergesResolvedFlag pins reloadThreads' extension for
// THREAD-01: an agent that resolves a thread by writing `resolved: true`
// straight to its file, the same way it appends a reply, is picked up on the
// next reload rather than requiring margin to be restarted.
func TestReloadThreadsMergesResolvedFlag(t *testing.T) {
	m := newTestModel(t)
	root := t.TempDir()
	m.store = &threadStore{root: root, docPath: "document.md"}

	existing := m.threads[soloAnchor]
	if existing.resolved {
		t.Fatal("test fixture assumption broken: soloAnchor already resolved")
	}

	resolved := &thread{
		anchor:   existing.anchor,
		quote:    existing.quote,
		posted:   append([]comment{}, existing.posted...),
		resolved: true,
	}
	if err := writeThreadFile(root, "document.md", resolved); err != nil {
		t.Fatalf("writeThreadFile: %v", err)
	}

	if err := m.reloadThreads(); err != nil {
		t.Fatalf("reloadThreads: %v", err)
	}

	if got := m.threads[soloAnchor]; !got.resolved {
		t.Fatal("reloadThreads did not pick up the externally set resolved flag")
	}
}

func TestReloadThreadsPreservesAnUnsubmittedDraft(t *testing.T) {
	m := newTestModel(t)
	root := t.TempDir()
	m.store = &threadStore{root: root, docPath: "document.md"}

	existing := m.threads[soloAnchor]
	existing.setDraft(newCommentSlot, "half-written reply, not yet submitted")

	// Nothing has changed on disk — reloadThreads still runs (as it would
	// on any filesystem event for the directory) and must not touch drafts,
	// since writeThreadFile never represents them to begin with.
	if err := writeThreadFile(root, "document.md", &thread{
		anchor: existing.anchor, quote: existing.quote, posted: existing.posted,
	}); err != nil {
		t.Fatalf("writeThreadFile: %v", err)
	}

	if err := m.reloadThreads(); err != nil {
		t.Fatalf("reloadThreads: %v", err)
	}

	if got := m.threads[soloAnchor].draft(newCommentSlot); got != "half-written reply, not yet submitted" {
		t.Fatalf("draft = %q, want the unsubmitted draft to survive reload", got)
	}
}

func TestReloadThreadsWithoutAStoreIsANoop(t *testing.T) {
	m := newTestModel(t)
	if m.store != nil {
		t.Fatalf("newTestModel: store = %+v, want nil", m.store)
	}
	if err := m.reloadThreads(); err != nil {
		t.Fatalf("reloadThreads with no store: %v", err)
	}
}

func TestReloadThreadsReattachesOrphanState(t *testing.T) {
	m := newTestModel(t)
	root := t.TempDir()
	m.store = &threadStore{root: root, docPath: "document.md"}

	// A thread for an anchor no block carries — reloadThreads must still run
	// reattach so it comes back marked orphaned rather than silently treated
	// as live.
	orphan := &thread{anchor: "^does-not-exist", quote: "gone"}
	if err := writeThreadFile(root, "document.md", orphan); err != nil {
		t.Fatalf("writeThreadFile: %v", err)
	}

	if err := m.reloadThreads(); err != nil {
		t.Fatalf("reloadThreads: %v", err)
	}

	got, ok := m.threads["^does-not-exist"]
	if !ok {
		t.Fatalf("reloadThreads did not load the orphan thread")
	}
	if !got.orphaned {
		t.Fatalf("orphaned = false, want true for a thread with no matching block")
	}
}

// --- the watcher itself, over a real filesystem -----------------------------

func TestThreadWatcherFiresOnAFileWrittenAfterItStarts(t *testing.T) {
	root := t.TempDir()
	tw, err := newThreadWatcher(root, "document.md")
	if err != nil {
		t.Skipf("newThreadWatcher: %v (no inotify in this environment?)", err)
	}
	defer tw.close()

	done := make(chan tea.Msg, 1)
	go func() { done <- tw.wait()() }()

	// Give the watch goroutine a moment to be blocked on the channel before
	// producing the event it is waiting for.
	time.Sleep(50 * time.Millisecond)
	if err := writeThreadFile(root, "document.md", &thread{anchor: "^new", quote: "q"}); err != nil {
		t.Fatalf("writeThreadFile: %v", err)
	}

	select {
	case msg := <-done:
		if _, ok := msg.(threadsChangedMsg); !ok {
			t.Fatalf("wait() returned %#v, want threadsChangedMsg", msg)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("wait() did not observe the write within 5s")
	}
}

func TestThreadWatcherIgnoresNonMarkdownFiles(t *testing.T) {
	root := t.TempDir()
	tw, err := newThreadWatcher(root, "document.md")
	if err != nil {
		t.Skipf("newThreadWatcher: %v (no inotify in this environment?)", err)
	}
	defer tw.close()

	dir := filepath.Join(threadsDir(root), "document.md")
	path := filepath.Join(dir, "notes.txt")

	done := make(chan tea.Msg, 1)
	go func() { done <- tw.wait()() }()
	time.Sleep(50 * time.Millisecond)

	if err := os.WriteFile(path, []byte("not a thread file"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	// Follow up with a real thread file, since wait() loops past anything
	// that does not qualify — a non-.md write alone should not be what
	// unblocks it.
	if err := writeThreadFile(root, "document.md", &thread{anchor: "^new", quote: "q"}); err != nil {
		t.Fatalf("writeThreadFile: %v", err)
	}

	select {
	case msg := <-done:
		if _, ok := msg.(threadsChangedMsg); !ok {
			t.Fatalf("wait() returned %#v, want threadsChangedMsg", msg)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("wait() did not fire for the qualifying write within 5s")
	}
}

func TestNewThreadWatcherCreatesTheThreadDirUpfront(t *testing.T) {
	root := t.TempDir()
	tw, err := newThreadWatcher(root, "document.md")
	if err != nil {
		t.Skipf("newThreadWatcher: %v (no inotify in this environment?)", err)
	}
	defer tw.close()

	dir := filepath.Join(threadsDir(root), "document.md")
	if fi, err := os.Stat(dir); err != nil || !fi.IsDir() {
		t.Fatalf("thread dir %s not created upfront: %v", dir, err)
	}
}

func TestNilThreadWatcherWaitIsANoopCmd(t *testing.T) {
	var tw *threadWatcher
	if cmd := tw.wait(); cmd != nil {
		t.Fatal("nil *threadWatcher.wait() should return a nil Cmd")
	}
}

func TestNilThreadWatcherCloseIsANoop(t *testing.T) {
	var tw *threadWatcher
	if err := tw.close(); err != nil {
		t.Fatalf("nil *threadWatcher.close(): %v", err)
	}
}

// --- the document file itself is watched too --------------------------------

func TestThreadWatcherFiresOnADocumentChange(t *testing.T) {
	root := t.TempDir()
	doc := filepath.Join(root, "document.md")
	if err := os.WriteFile(doc, []byte("# hi\n"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	tw, err := newThreadWatcher(root, "document.md")
	if err != nil {
		t.Skipf("newThreadWatcher: %v (no inotify in this environment?)", err)
	}
	defer tw.close()

	done := make(chan tea.Msg, 1)
	go func() { done <- tw.wait()() }()
	time.Sleep(50 * time.Millisecond)

	if err := os.WriteFile(doc, []byte("# hi\n\nChanged.\n"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	select {
	case msg := <-done:
		if _, ok := msg.(docChangedMsg); !ok {
			t.Fatalf("wait() returned %#v, want docChangedMsg", msg)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("wait() did not observe the document write within 5s")
	}
}

// TestThreadWatcherIgnoresASiblingDocument: the doc watch is on the document's
// directory, so it can see every file in it — a sibling markdown document must
// not masquerade as a thread change (or a document change) for the review in
// front of it.
func TestThreadWatcherIgnoresASiblingDocument(t *testing.T) {
	root := t.TempDir()
	doc := filepath.Join(root, "document.md")
	sibling := filepath.Join(root, "sibling.md")
	if err := os.WriteFile(doc, []byte("# doc\n"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	if err := os.WriteFile(sibling, []byte("# sibling\n"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	tw, err := newThreadWatcher(root, "document.md")
	if err != nil {
		t.Skipf("newThreadWatcher: %v (no inotify in this environment?)", err)
	}
	defer tw.close()

	done := make(chan tea.Msg, 1)
	go func() { done <- tw.wait()() }()
	time.Sleep(50 * time.Millisecond)

	if err := os.WriteFile(sibling, []byte("# sibling\n\nmore\n"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	if err := os.WriteFile(doc, []byte("# doc\n\nchanged\n"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	select {
	case msg := <-done:
		if _, ok := msg.(docChangedMsg); !ok {
			t.Fatalf("wait() returned %#v, want docChangedMsg — the sibling write must not fire first", msg)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("wait() did not observe the document write within 5s")
	}
}

// --- reloading the document ------------------------------------------------

func TestReloadDocPicksUpAChangedDocumentAndKeepsFocus(t *testing.T) {
	root := t.TempDir()
	docPath := "doc.md"
	path := filepath.Join(root, docPath)
	// The marker stamps the block it follows (Second paragraph) with ^deadbeef,
	// so focus on it survives a reword of the paragraph above.
	src1 := "# Heading\n\nFirst paragraph.\n\nSecond paragraph.\n\n<!--margin:^deadbeef-->\n"
	if err := os.WriteFile(path, []byte(src1), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	doc, src, err := loadDoc(path)
	if err != nil {
		t.Fatalf("loadDoc: %v", err)
	}
	m := newModelAt(path, doc, nil)
	m.src = src
	m.store = &threadStore{root: root, docPath: docPath}
	m.at = cursor{entry: blockEntryFor(t, m, "^deadbeef"), comment: commentNone}
	m.docChanged = true

	src2 := "# Heading\n\nFirst paragraph — rewritten by the agent.\n\nSecond paragraph.\n\n<!--margin:^deadbeef-->\n"
	if err := os.WriteFile(path, []byte(src2), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	m.reloadDoc()

	if m.docChanged {
		t.Fatal("reloadDoc did not clear the file-changed notice")
	}
	if string(m.src) != src2 {
		t.Fatalf("m.src not refreshed by reloadDoc")
	}
	if got := m.anchorAt(); got != "^deadbeef" {
		t.Fatalf("focus anchor = %q, want ^deadbeef — focus must follow the stamped block", got)
	}
	var sawRewritten bool
	for _, b := range m.doc {
		if strings.Contains(b.text, "rewritten by the agent") {
			sawRewritten = true
		}
	}
	if !sawRewritten {
		t.Fatal("reloaded doc does not contain the rewritten paragraph")
	}
}

func TestReloadDocWithoutAStoreIsANoop(t *testing.T) {
	m := newTestModel(t)
	m.reloadDoc()
	if m.status != "nothing to reload" {
		t.Fatalf("status = %q, want %q", m.status, "nothing to reload")
	}
}

// TestReloadDocRefusesWhileComposerOpen: the prose is being written to, not
// read, so a reload must refuse rather than swap the document out from under a
// live edit (D7).
func TestReloadDocRefusesWhileComposerOpen(t *testing.T) {
	requireNvim(t)
	root := t.TempDir()
	docPath := "doc.md"
	path := filepath.Join(root, docPath)
	if err := os.WriteFile(path, []byte("# Heading\n\nParagraph one.\n"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	doc, src, err := loadDoc(path)
	if err != nil {
		t.Fatalf("loadDoc: %v", err)
	}
	m := newModelAt(path, doc, nil)
	m.src = src
	m.store = &threadStore{root: root, docPath: docPath}
	m.w, m.h = 100, 60
	m.at = cursor{entry: blockEntryFor(t, m, doc[1].anchor), comment: commentNone}

	open(t, m, doc[1].anchor, newCommentSlot, "")
	if m.comp == nil {
		t.Fatal("composer did not open")
	}
	m.reloadDoc()
	if m.comp == nil {
		t.Fatal("reloadDoc closed the composer")
	}
	if !strings.Contains(m.status, "close the editor") {
		t.Fatalf("status = %q, want the close-the-editor refusal", m.status)
	}
}

// --- the new-comment notice ------------------------------------------------

func TestNoticeNewEventsAnnouncesTheNewestComment(t *testing.T) {
	m := newTestModel(t)
	root := t.TempDir()
	m.store = &threadStore{root: root, docPath: "document.md"}

	id0, err := appendEvent(root, event{kind: eventThreadResolved, anchor: "^old", author: "toly", comment: -1})
	if err != nil {
		t.Fatalf("appendEvent: %v", err)
	}
	m.lastEventID = id0

	// An agent's reply lands, and a resolution lands after it. The reply is
	// the news; the cursor still advances past both.
	if _, err := appendEvent(root, event{kind: eventCommentPosted, anchor: "^b2", author: "agent", comment: 0, text: "Addressed."}); err != nil {
		t.Fatalf("appendEvent: %v", err)
	}
	if _, err := appendEvent(root, event{kind: eventThreadResolved, anchor: "^b2", author: "agent", comment: -1}); err != nil {
		t.Fatalf("appendEvent: %v", err)
	}

	m.noticeNewEvents()
	if m.status != "new comment from agent on ^b2" {
		t.Fatalf("status = %q, want the new-comment notice", m.status)
	}
	evs, err := readEventsAfter(root, m.lastEventID)
	if err != nil {
		t.Fatalf("readEventsAfter: %v", err)
	}
	if len(evs) != 0 {
		t.Fatalf("cursor did not advance; %d events are still new", len(evs))
	}
}

func TestNoticeNewEventsWithNothingNewIsSilent(t *testing.T) {
	m := newTestModel(t)
	root := t.TempDir()
	m.store = &threadStore{root: root, docPath: "document.md"}
	id0, err := appendEvent(root, event{kind: eventThreadResolved, anchor: "^a", author: "toly", comment: -1})
	if err != nil {
		t.Fatalf("appendEvent: %v", err)
	}
	m.lastEventID = id0

	m.status = ""
	m.noticeNewEvents()
	if m.status != "" {
		t.Fatalf("status = %q, want silence when nothing new", m.status)
	}
}

// TestReviewersOwnEmitAdvancesTheCursor pins the contract that keeps the
// reviewer's own comments from being announced back to them: emit hands back
// the id it wrote, and the model records it as seen (the same assignment
// dismiss, toggleResolved and deleteFocused make), so the watcher the write
// wakes finds nothing new to say.
func TestReviewersOwnEmitAdvancesTheCursor(t *testing.T) {
	m := newTestModel(t)
	root := t.TempDir()
	m.store = &threadStore{root: root, docPath: "document.md"}
	id0, err := appendEvent(root, event{kind: eventThreadResolved, anchor: "^a", author: "toly", comment: -1})
	if err != nil {
		t.Fatalf("appendEvent: %v", err)
	}
	m.lastEventID = id0

	id1, err := m.store.emit(event{kind: eventCommentPosted, anchor: "^b", author: "toly", comment: 0, text: "my words"})
	if err != nil {
		t.Fatalf("emit: %v", err)
	}
	if id1 == "" {
		t.Fatal("emit returned no id")
	}
	m.lastEventID = id1

	m.status = ""
	m.noticeNewEvents()
	if m.status != "" {
		t.Fatalf("status = %q, want the reviewer's own comment to be silence", m.status)
	}
}

// --- the footer notice ------------------------------------------------------

func TestDocChangedMsgSetsTheNotice(t *testing.T) {
	m := newTestModel(t)
	nm, _ := m.Update(docChangedMsg{})
	mm := nm.(*model)
	if !mm.docChanged {
		t.Fatal("docChanged not set by docChangedMsg")
	}
	if !strings.Contains(mm.status, "file changed on disk") {
		t.Fatalf("status = %q, want the file-changed notice", mm.status)
	}
}

func TestDocChangedNoticeShowsInTheFooterUntilCleared(t *testing.T) {
	m := newTestModel(t)
	m.docChanged = true
	if footer := m.View().Content; !strings.Contains(footer, "file changed") || !strings.Contains(footer, "ctrl+r") {
		t.Fatalf("footer with the notice on = %q, want the file-changed marker and its reload key", footer)
	}
	m.docChanged = false
	if footer := m.View().Content; strings.Contains(footer, "file changed") {
		t.Fatalf("footer without the notice = %q, want it gone", footer)
	}
}
