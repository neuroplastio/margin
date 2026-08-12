package review

import (
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
)

// inboxFixture writes a tree (the treeFixture docs) plus thread files for
// both documents and one for a document that is not in the tree, returning
// the review root. The threads' comments carry distinct timestamps so sort
// order is deterministic.
func inboxFixture(t *testing.T) string {
	t.Helper()
	root, _, _ := treeFixture(t)
	at := func(hour, min int) time.Time {
		return time.Date(2026, 8, 10, hour, min, 0, 0, time.UTC)
	}
	write := func(doc, anchor string, c comment) {
		t.Helper()
		if err := writeThreadFile(root, doc, &thread{
			anchor: anchor,
			quote:  "quoted block",
			posted: []comment{c},
		}); err != nil {
			t.Fatal(err)
		}
	}
	write("README.md", "^readme",
		comment{author: "you", body: "oldest", at: at(8, 0)})
	write("docs/spec.md", "^spec",
		comment{author: "agent", body: "newest", at: at(10, 0)})
	write("gone.md", "^gone",
		comment{author: "you", body: "middle", at: at(9, 0)})
	return root
}

// firstAnchor parses a markdown file and returns the first block with an
// anchor, so a test can target a block that really exists.
func firstAnchor(t *testing.T, path string) string {
	t.Helper()
	doc, _, err := loadDoc(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, b := range doc {
		if b.anchor != "" {
			return b.anchor
		}
	}
	t.Fatal("no anchored block in " + path)
	return ""
}

func TestLoadInboxAggregatesThreadsNewestFirst(t *testing.T) {
	root := inboxFixture(t)
	tree, err := walkMarkdownTree(root, root)
	if err != nil {
		t.Fatalf("walkMarkdownTree: %v", err)
	}
	items, err := loadInbox(root, tree)
	if err != nil {
		t.Fatalf("loadInbox: %v", err)
	}
	if len(items) != 3 {
		t.Fatalf("loadInbox returned %d items, want 3 (including a gone-doc thread)", len(items))
	}
	// Newest activity first: the spec thread (10:00), the gone thread (09:00),
	// the readme thread (08:00).
	want := []struct{ doc, anchor string }{
		{"docs/spec.md", "^spec"},
		{"gone.md", "^gone"},
		{"README.md", "^readme"},
	}
	for i, w := range want {
		if items[i].doc != w.doc || items[i].anchor != w.anchor {
			t.Errorf("item %d = %s %s, want %s %s", i, items[i].doc, items[i].anchor, w.doc, w.anchor)
		}
	}
	if items[0].inTree != true || items[1].inTree != false || items[2].inTree != true {
		t.Errorf("inTree flags = %v %v %v, want true false true",
			items[0].inTree, items[1].inTree, items[2].inTree)
	}
}

func TestLoadInboxAbsentThreadsDirIsEmpty(t *testing.T) {
	root, _, _ := treeFixture(t)
	tree, err := walkMarkdownTree(root, root)
	if err != nil {
		t.Fatalf("walkMarkdownTree: %v", err)
	}
	items, err := loadInbox(root, tree)
	if err != nil {
		t.Fatalf("loadInbox with no threads dir: %v", err)
	}
	if len(items) != 0 {
		t.Errorf("loadInbox returned %d items, want 0", len(items))
	}
}

func TestToggleInboxOpensAndCloses(t *testing.T) {
	root := inboxFixture(t)
	m, err := newTreeModel(root)
	if err != nil {
		t.Fatalf("newTreeModel: %v", err)
	}
	cmd, ok := commandByID("inbox.toggle")
	if !ok {
		t.Fatal("inbox.toggle is not registered")
	}
	cmd.Run(m, "")
	if !m.inbox {
		t.Fatal("inbox.toggle did not open the inbox")
	}
	if len(m.inboxItems) != 3 {
		t.Errorf("inbox has %d items, want 3", len(m.inboxItems))
	}
	cmd.Run(m, "")
	if m.inbox {
		t.Fatal("inbox.toggle did not close the inbox")
	}
}

func TestInboxKeysNavigateAndClose(t *testing.T) {
	root := inboxFixture(t)
	m, err := newTreeModel(root)
	if err != nil {
		t.Fatalf("newTreeModel: %v", err)
	}
	m.toggleInbox()
	key := func(r rune) tea.KeyPressMsg {
		return tea.KeyPressMsg(tea.Key{Code: r, Text: string(r)})
	}
	m.handleInboxKey(key('j'))
	if m.inboxAt != 1 {
		t.Errorf("after j: inboxAt = %d, want 1", m.inboxAt)
	}
	m.handleInboxKey(key('G'))
	if m.inboxAt != len(m.inboxItems)-1 {
		t.Errorf("after G: inboxAt = %d, want the last row", m.inboxAt)
	}
	m.handleInboxKey(key('g'))
	if m.inboxAt != 0 {
		t.Errorf("after g: inboxAt = %d, want 0", m.inboxAt)
	}
	m.handleInboxKey(key('k'))
	if m.inboxAt != 0 {
		t.Errorf("at the first row, k moved to %d", m.inboxAt)
	}
	m.handleInboxKey(tea.KeyPressMsg(tea.Key{Code: tea.KeyEsc}))
	if m.inbox {
		t.Error("esc did not close the inbox")
	}
}

func TestInboxKeysOnEmptyInboxStayInRange(t *testing.T) {
	root, _, _ := treeFixture(t)
	m, err := newTreeModel(root)
	if err != nil {
		t.Fatalf("newTreeModel: %v", err)
	}
	m.toggleInbox()
	if len(m.inboxItems) != 0 {
		t.Fatalf("inbox has %d items, want 0", len(m.inboxItems))
	}
	key := func(r rune) tea.KeyPressMsg {
		return tea.KeyPressMsg(tea.Key{Code: r, Text: string(r)})
	}
	for _, k := range []rune{'j', 'k', 'g', 'G'} {
		m.handleInboxKey(key(k))
		if m.inboxAt != 0 {
			t.Errorf("after %c on an empty inbox: inboxAt = %d, want 0", k, m.inboxAt)
		}
	}
}

func TestInboxOpenJumpsToTheThread(t *testing.T) {
	root, _, b := treeFixture(t)
	anchor := firstAnchor(t, filepath.Join(root, filepath.FromSlash(b)))
	if err := writeThreadFile(root, b, &thread{
		anchor: anchor,
		quote:  "Some nested prose.",
		posted: []comment{{author: "agent", body: "please fix", at: time.Now()}},
	}); err != nil {
		t.Fatal(err)
	}
	m, err := newTreeModel(root)
	if err != nil {
		t.Fatalf("newTreeModel: %v", err)
	}
	m.toggleInbox()
	if len(m.inboxItems) != 1 {
		t.Fatalf("inbox has %d items, want 1", len(m.inboxItems))
	}
	m.openInboxItem(0)
	if m.inbox {
		t.Error("opening a thread left the inbox open")
	}
	if m.store.docPath != b {
		t.Errorf("store.docPath = %q, want %q (the thread's document)", m.store.docPath, b)
	}
	if m.at.entry < 0 || m.at.entry >= len(m.entries) {
		t.Fatalf("focus entry %d out of range", m.at.entry)
	}
	if got := m.entries[m.at.entry].b.anchor; got != anchor {
		t.Errorf("landed on %q, want %q", got, anchor)
	}
	if !strings.Contains(m.status, b) {
		t.Errorf("status = %q, want it to name the document", m.status)
	}
}

func TestInboxOpenReportsDocNotInTree(t *testing.T) {
	root := inboxFixture(t)
	m, err := newTreeModel(root)
	if err != nil {
		t.Fatalf("newTreeModel: %v", err)
	}
	m.toggleInbox()
	// The gone.md thread sorts to index 1 (its 09:00 sits between the others).
	before := m.store.docPath
	m.openInboxItem(1)
	if m.store.docPath != before {
		t.Errorf("opening a gone-doc thread switched the document to %q", m.store.docPath)
	}
	if !strings.Contains(m.status, "not in this tree") {
		t.Errorf("status = %q, want the not-in-tree report", m.status)
	}
	if m.inbox {
		t.Error("opening left the inbox open")
	}
}

func TestInboxSingleDocumentNeedsAStore(t *testing.T) {
	m := seedModel()
	cmd, ok := commandByID("inbox.toggle")
	if !ok {
		t.Fatal("inbox.toggle is not registered")
	}
	if cmd.Applicable(m) {
		t.Error("inbox.toggle is applicable on a model with no store (no review root)")
	}
	cmd.Run(m, "")
	if m.inbox {
		t.Error("inbox.toggle opened the threads view with no store")
	}
}

func TestRenderInboxMarksFocusAndResolved(t *testing.T) {
	root := inboxFixture(t)
	m, err := newTreeModel(root)
	if err != nil {
		t.Fatalf("newTreeModel: %v", err)
	}
	m.toggleInbox()
	m.w, m.h = 100, 30
	m.inboxItems[1].thread.resolved = true
	rows := m.renderInbox(20)
	if len(rows) != 20 {
		t.Fatalf("renderInbox returned %d rows, want 20", len(rows))
	}
	if !strings.Contains(rows[0], "▌") {
		t.Errorf("focused row has no cursor:\n%q", rows[0])
	}
	if !strings.Contains(rows[0], "docs/spec.md") {
		t.Errorf("focused row does not name its document:\n%q", rows[0])
	}
	if !strings.Contains(rows[1], "✓") {
		t.Errorf("resolved row has no checkmark:\n%q", rows[1])
	}
	if !strings.Contains(rows[1], "doc not in tree") {
		t.Errorf("gone-doc row does not say so:\n%q", rows[1])
	}
}

func TestViewComposesPaneAndInboxColumns(t *testing.T) {
	root := inboxFixture(t)
	m, err := newTreeModel(root)
	if err != nil {
		t.Fatalf("newTreeModel: %v", err)
	}
	m.w, m.h = 100, 20
	m.toggleInbox()
	out := m.View().Content
	// The tree pane still renders beside the threads view, and the view shows a
	// thread row and the footer hint.
	for _, want := range []string{"README.md", "docs/", "threads — 3 thread(s)", "oldest"} {
		if !strings.Contains(out, want) {
			t.Errorf("inbox view does not contain %q:\n%s", want, out)
		}
	}
}

// TestSingleDocumentThreadsView: a single-document review (store set, no tree)
// now has the threads view too — it lists the current document's threads,
// newest first, orphans included. The document name is dropped from the rows
// (every row is the same document), and an orphaned thread is marked.
func TestSingleDocumentThreadsView(t *testing.T) {
	m := newTestModel(t)
	m.store = &threadStore{root: t.TempDir(), docPath: "document.md"}
	// An orphan: a thread whose block is not in the document.
	m.threads["^gone"] = &thread{
		anchor: "^gone",
		quote:  "vanished block",
		posted: []comment{{author: "you", body: "the block this was on is gone", at: time.Now()}},
	}
	reattach(m.doc, m.threads)

	cmd, ok := commandByID("inbox.toggle")
	if !ok {
		t.Fatal("inbox.toggle is not registered")
	}
	if !cmd.Applicable(m) {
		t.Fatal("inbox.toggle is not applicable in a single-document review with a store")
	}
	cmd.Run(m, "")
	if !m.inbox {
		t.Fatal("inbox.toggle did not open the threads view")
	}
	if len(m.inboxItems) != 4 {
		t.Fatalf("threads view has %d items, want 4 (3 seeded + the orphan)", len(m.inboxItems))
	}
	// The orphan is present, newest first, and its doc is the current one.
	if !m.inboxItems[0].thread.orphaned {
		t.Fatalf("first item's thread is not orphaned: %+v", m.inboxItems[0].thread)
	}
	if m.inboxItems[0].doc != "document.md" {
		t.Errorf("orphan's doc = %q, want document.md", m.inboxItems[0].doc)
	}

	m.w, m.h = 100, 30
	rows := m.renderInbox(10)
	if !strings.Contains(rows[0], "[block gone]") {
		t.Errorf("orphaned row does not say [block gone]:\n%q", rows[0])
	}
	if strings.Contains(rows[1], "document.md  ") {
		t.Errorf("single-doc row repeats the document name:\n%q", rows[1])
	}
}

// TestSingleDocumentThreadsViewOpen: entering an attached thread in a
// single-document threads view lands on the block; entering an orphaned one
// reports the block is gone without pretending to open it.
func TestSingleDocumentThreadsViewOpen(t *testing.T) {
	m := newTestModel(t)
	m.store = &threadStore{root: t.TempDir(), docPath: "document.md"}
	m.threads["^gone"] = &thread{
		anchor: "^gone",
		quote:  "vanished block",
		posted: []comment{{author: "you", body: "the block this was on is gone", at: time.Now()}},
	}
	reattach(m.doc, m.threads)

	m.toggleInbox()
	// Find the rows: the orphan sorts newest first (index 0); the convo thread
	// is in there too.
	var orphan, convo int
	for i, it := range m.inboxItems {
		if it.anchor == "^gone" {
			orphan = i
		}
		if it.anchor == convoAnchor {
			convo = i
		}
	}
	m.openInboxItem(orphan)
	if !strings.Contains(m.status, "block is gone") {
		t.Errorf("opening the orphan status = %q, want the block-gone report", m.status)
	}
	m.toggleInbox()
	m.openInboxItem(convo)
	if got := m.entries[m.at.entry].b.anchor; got != convoAnchor {
		t.Errorf("landed on %q, want %q", got, convoAnchor)
	}
}

// TestCommentHop: `]`/`[` hop focus to the next/previous thread in the
// document, recording each landing in the jumplist and reporting at the ends.
func TestCommentHop(t *testing.T) {
	m := newTestModel(t)
	m.w, m.h = 100, 60
	m.at = cursor{entry: 0, comment: commentNone}

	cmd, ok := commandByID("comment.next")
	if !ok {
		t.Fatal("comment.next is not registered")
	}
	cmd.Run(m, "")
	if m.entries[m.at.entry].thread == nil {
		t.Fatalf("] landed on a non-thread entry %d", m.at.entry)
	}
	first := m.at.entry

	cmd.Run(m, "")
	if m.at.entry <= first {
		t.Errorf("second ] did not advance: %d -> %d", first, m.at.entry)
	}

	// The hop is recorded in the jumplist: ctrl+o returns to where it started.
	if len(m.jumps) < 3 {
		t.Errorf("jumplist has %d entries, want the two hops recorded", len(m.jumps))
	}

	// Reaching the end reports instead of wrapping.
	m.at = cursor{entry: len(m.entries) - 1, comment: commentNone}
	before := m.status
	cmd.Run(m, "")
	if m.status == before || !strings.Contains(m.status, "no more comments") {
		t.Errorf("hop past the end status = %q, want the no-more report", m.status)
	}

	prev, ok := commandByID("comment.prev")
	if !ok {
		t.Fatal("comment.prev is not registered")
	}
	prev.Run(m, "")
	if m.entries[m.at.entry].thread == nil {
		t.Fatalf("[ landed on a non-thread entry %d", m.at.entry)
	}
}

// TestCommentHopKeysRouteThroughTheRegistry: ] and [ resolve to comment.next
// and comment.prev.
func TestCommentHopKeysRouteThroughTheRegistry(t *testing.T) {
	if id := keymap["]"]; id != "comment.next" {
		t.Errorf("keymap[\"]\"] = %q, want comment.next", id)
	}
	if id := keymap["["]; id != "comment.prev" {
		t.Errorf("keymap[\"[\"] = %q, want comment.prev", id)
	}
	// The palette shows the binding.
	if got := firstBinding("comment.next"); got != "]" {
		t.Errorf("firstBinding(comment.next) = %q, want ]", got)
	}
}
