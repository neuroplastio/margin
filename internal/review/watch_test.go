package review

import (
	"os"
	"path/filepath"
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
