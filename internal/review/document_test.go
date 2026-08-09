package review

import (
	"bytes"
	"strings"
	"testing"
	"time"
)

// deleteMarkedBlock removes block b, plus the id marker stampAll/stampID
// wrote immediately after it, from src. It exists only to build fixtures for
// the tests below: a document as it would look if an agent deleted a
// commented-on block outright.
func deleteMarkedBlock(t *testing.T, src []byte, b block) []byte {
	t.Helper()
	marker := []byte("<!--margin:" + b.anchor + "-->")
	idx := bytes.Index(src, marker)
	if idx < 0 {
		t.Fatalf("fixture bug: no marker for %s in source", b.anchor)
	}
	end := idx + len(marker)
	for end < len(src) && src[end] != '\n' {
		end++
	}
	if end < len(src) {
		end++ // consume the marker's own trailing newline too
	}
	out := append([]byte{}, src[:b.start]...)
	out = append(out, src[end:]...)
	return out
}

// TestReattachKeepsThreadAcrossRewording is ID-02's headline guarantee: a
// thread anchored to a stamped id must still find its block after the block's
// text has changed underneath it — that is the entire reason ids exist
// instead of a content hash (D4).
func TestReattachKeepsThreadAcrossRewording(t *testing.T) {
	stamped, blocks := stampAll([]byte(sampleDoc), parseDoc([]byte(sampleDoc)))

	var target block
	for _, b := range blocks {
		if b.kind == blockPara {
			target = b
			break
		}
	}
	if target.anchor == "" {
		t.Fatal("fixture has no paragraph to reword")
	}

	threads := map[string]*thread{
		target.anchor: {anchor: target.anchor, quote: target.text},
	}

	reworded := append([]byte{}, stamped[:target.start]...)
	reworded = append(reworded, []byte("A completely different sentence, nothing like the original.")...)
	reworded = append(reworded, stamped[target.stop:]...)

	after := parseDoc(reworded)
	reattach(after, threads)

	got := threads[target.anchor]
	if got.orphaned {
		t.Fatal("thread was orphaned after its block was merely reworded, not deleted")
	}

	var found bool
	for _, b := range after {
		if b.anchor == target.anchor {
			found = true
			if !strings.Contains(b.text, "completely different sentence") {
				t.Errorf("block kept the anchor but not the new text: %q", b.text)
			}
			if b.text == target.text {
				t.Error("test fixture did not actually reword the block")
			}
		}
	}
	if !found {
		t.Fatalf("no block in the reworded document carries anchor %s", target.anchor)
	}
}

// TestReattachOrphansWhenBlockDeleted covers the other half: a thread whose
// block is gone entirely — not just changed — must come back flagged
// orphaned rather than silently vanishing from the threads map's caller's
// view.
func TestReattachOrphansWhenBlockDeleted(t *testing.T) {
	stamped, blocks := stampAll([]byte(sampleDoc), parseDoc([]byte(sampleDoc)))

	var target block
	for _, b := range blocks {
		if b.kind == blockPara {
			target = b
			break
		}
	}
	if target.anchor == "" {
		t.Fatal("fixture has no paragraph to delete")
	}

	threads := map[string]*thread{
		target.anchor: {anchor: target.anchor, quote: target.text},
	}

	without := deleteMarkedBlock(t, stamped, target)
	after := parseDoc(without)

	for _, b := range after {
		if b.anchor == target.anchor {
			t.Fatalf("fixture bug: deleted block's anchor %s still present", target.anchor)
		}
	}

	reattach(after, threads)

	if !threads[target.anchor].orphaned {
		t.Error("thread for a deleted block was not flagged orphaned")
	}
}

// TestReattachLeavesUntouchedThreadsAttached is the sanity check: a document
// that reparses identically must not orphan anything, and reattach must be
// safe to call repeatedly.
func TestReattachLeavesUntouchedThreadsAttached(t *testing.T) {
	stamped, blocks := stampAll([]byte(sampleDoc), parseDoc([]byte(sampleDoc)))

	threads := map[string]*thread{}
	for _, b := range blocks {
		if b.commentable() {
			threads[b.anchor] = &thread{anchor: b.anchor, quote: b.text}
		}
	}

	reparsed := parseDoc(stamped)
	reattach(reparsed, threads)
	reattach(reparsed, threads) // idempotent

	for a, th := range threads {
		if th.orphaned {
			t.Errorf("thread %s was orphaned in a document that did not change", a)
		}
	}
}

// TestNewModelReattachesOnOpen is the wiring test: newModelAt is "opening" a
// document, so a threads map handed to it — the shape STORE-01 will load from
// disk — must come out reattached without the caller doing anything extra. A
// live thread shows up in the entry list; an orphaned one does not, but stays
// reachable through orphanedThreads rather than disappearing outright.
func TestNewModelReattachesOnOpen(t *testing.T) {
	stamped, blocks := stampAll([]byte(sampleDoc), parseDoc([]byte(sampleDoc)))

	var live, gone block
	for _, b := range blocks {
		if b.kind != blockPara {
			continue
		}
		if live.anchor == "" {
			live = b
			continue
		}
		gone = b
		break
	}
	if live.anchor == "" || gone.anchor == "" {
		t.Fatal("fixture needs at least two paragraphs")
	}

	without := deleteMarkedBlock(t, stamped, gone)
	doc := parseDoc(without)

	threads := map[string]*thread{
		live.anchor: {anchor: live.anchor, quote: live.text},
		gone.anchor: {anchor: gone.anchor, quote: gone.text},
	}

	m := newModelAt("doc.md", doc, threads)

	if threads[live.anchor].orphaned {
		t.Error("surviving block's thread was orphaned on open")
	}
	if !threads[gone.anchor].orphaned {
		t.Error("deleted block's thread was not orphaned on open")
	}

	for _, e := range m.entries {
		if e.thread != nil && e.thread == threads[gone.anchor] {
			t.Error("orphaned thread still made it into the rendered entry list")
		}
	}

	orphans := m.orphanedThreads()
	if len(orphans) != 1 || orphans[0] != threads[gone.anchor] {
		t.Errorf("orphanedThreads() = %v, want just the deleted block's thread", orphans)
	}
}

// TestDeleteCommentTombstones pins D11's shape: author, timestamp, and body survive.
func TestDeleteCommentTombstones(t *testing.T) {
	at := time.Date(2026, 8, 7, 9, 0, 0, 0, time.UTC)
	th := &thread{posted: []comment{
		{author: "toly", body: "why thirty seconds?", at: at},
		{author: "agent", body: "cited the incident", at: at.Add(time.Minute)},
	}}

	th.deleteComment(0)

	if !th.posted[0].deleted {
		t.Error("posted[0].deleted = false, want true")
	}
	if th.posted[0].body != "why thirty seconds?" {
		t.Errorf("posted[0].body = %q, want preserved body after deletion", th.posted[0].body)
	}
	if th.posted[0].author != "toly" {
		t.Errorf("posted[0].author = %q, want kept as toly", th.posted[0].author)
	}
	if !th.posted[0].at.Equal(at) {
		t.Errorf("posted[0].at = %v, want kept as %v", th.posted[0].at, at)
	}
	// The reply is untouched — deleting one comment must not touch its siblings.
	if th.posted[1].deleted || th.posted[1].body == "" {
		t.Error("deleteComment(0) affected posted[1]")
	}
}

// TestRestoreComment covers restoring deleted comments and toggling delete state.
func TestRestoreComment(t *testing.T) {
	th := &thread{posted: []comment{
		{author: "toly", body: "first comment"},
	}}

	th.deleteComment(0)
	if !th.posted[0].deleted {
		t.Error("want comment deleted")
	}

	th.restoreComment(0)
	if th.posted[0].deleted {
		t.Error("want comment restored")
	}

	// Toggle comment
	del := th.toggleDeleteComment(0)
	if !del || !th.posted[0].deleted {
		t.Error("toggleDeleteComment want deleted = true")
	}
	del = th.toggleDeleteComment(0)
	if del || th.posted[0].deleted {
		t.Error("toggleDeleteComment want deleted = false")
	}

	// Toggle thread
	delTh := th.toggleDeleteThread()
	if !delTh || !th.posted[0].deleted {
		t.Error("toggleDeleteThread want deleted = true")
	}
	delTh = th.toggleDeleteThread()
	if delTh || th.posted[0].deleted {
		t.Error("toggleDeleteThread want deleted = false")
	}
}

// TestDeleteCommentClearsPendingEdit covers the case an edit draft targets the
// comment being deleted: editing a deleted comment makes no sense, so the
// draft should not survive it.
func TestDeleteCommentClearsPendingEdit(t *testing.T) {
	th := &thread{posted: []comment{{author: "toly", body: "orig"}}}
	th.setDraft(0, "an in-progress edit")

	th.deleteComment(0)

	if d := th.draft(0); d != "" {
		t.Errorf("draft(0) = %q after deleting the comment it edits, want empty", d)
	}
}

// TestDeleteCommentOutOfRangeIsNoop guards a caller holding a stale index —
// deleteComment must not panic.
func TestDeleteCommentOutOfRangeIsNoop(t *testing.T) {
	th := &thread{posted: []comment{{author: "toly", body: "hi"}}}
	th.deleteComment(5)
	th.deleteComment(-1)
	if th.posted[0].deleted || th.posted[0].body != "hi" {
		t.Error("an out-of-range deleteComment mutated the existing comment")
	}
}

// TestDeleteThreadTombstonesEveryComment covers "delete a whole thread" (D11):
// deleting every posted comment while preserving bodies.
func TestDeleteThreadTombstonesEveryComment(t *testing.T) {
	th := &thread{posted: []comment{
		{author: "toly", body: "first"},
		{author: "agent", body: "second"},
	}}

	th.deleteThread()

	for i, c := range th.posted {
		if !c.deleted || c.body == "" {
			t.Errorf("posted[%d] = %+v, want deleted with body kept", i, c)
		}
	}
}
