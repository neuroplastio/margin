package review

import (
	"bytes"
	"strings"
	"testing"
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
