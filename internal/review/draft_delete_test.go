package review

import (
	"testing"
)

// TestDeleteFocusedDiscardsNewCommentDraft: focus on a thread row holding a
// half-written new comment, D discards the draft — and, when the thread held
// nothing but the draft, drops the empty thread and puts focus back on the
// block. The on-disk draft is cleared too, so nothing resurrects on reopen.
func TestDeleteFocusedDiscardsNewCommentDraft(t *testing.T) {
	m := newTestModel(t)

	i := entryFor(t, m, draftAnchor)
	m.at = cursor{entry: i, comment: commentNone}
	tr := m.threads[draftAnchor]
	if got := tr.draft(newCommentSlot); got == "" {
		t.Fatalf("test setup: %s should hold a draft", draftAnchor)
	}

	m.deleteFocused()

	if _, ok := m.threads[draftAnchor]; ok {
		t.Fatal("D on a thread holding only a draft left the empty thread behind")
	}
	if got, _ := loadDraft(draftAnchor, newCommentSlot); got != "" {
		t.Fatalf("D left draft %q on disk", got)
	}
	if m.status != "draft discarded" {
		t.Fatalf("status = %q, want %q", m.status, "draft discarded")
	}
	if e := m.entries[m.at.entry]; e.b.anchor != draftAnchor {
		t.Fatalf("focus on %s, want back on the block %s", e.b.anchor, draftAnchor)
	}
}

// TestDeleteFocusedDiscardsNewCommentDraftKeepsPostedComments: a thread with
// posted comments AND a half-written new comment — D discards the draft only;
// the thread and its comments stay.
func TestDeleteFocusedDiscardsNewCommentDraftKeepsPostedComments(t *testing.T) {
	m := newTestModel(t)

	tr := m.threads[convoAnchor]
	tr.setDraft(newCommentSlot, "half a thought")
	i := entryFor(t, m, convoAnchor)
	m.at = cursor{entry: i, comment: commentNone}

	m.deleteFocused()

	if m.threads[convoAnchor] == nil {
		t.Fatal("D on a thread with posted comments + a draft dropped the thread")
	}
	if got := tr.draft(newCommentSlot); got != "" {
		t.Fatalf("draft = %q, want discarded", got)
	}
	if len(tr.posted) != 2 {
		t.Fatalf("posted comments = %d, want the 2 untouched", len(tr.posted))
	}
	if tr.posted[0].deleted || tr.posted[1].deleted {
		t.Fatal("posted comments were tombstoned by a draft delete")
	}
	if m.status != "draft discarded" {
		t.Fatalf("status = %q, want %q", m.status, "draft discarded")
	}
}

// TestDeleteFocusedDiscardsEditDraft: focus on a comment with an unsaved edit,
// D discards the draft and keeps the comment at its posted text — it does not
// tombstone the comment.
func TestDeleteFocusedDiscardsEditDraft(t *testing.T) {
	m := newTestModel(t)

	tr := m.threads[convoAnchor]
	original := tr.posted[0].body
	tr.setDraft(0, "unsaved rewrite")
	i := entryFor(t, m, convoAnchor)
	m.at = cursor{entry: i, comment: 0}

	m.deleteFocused()

	if m.threads[convoAnchor] == nil {
		t.Fatal("D on an edit draft dropped the thread")
	}
	if got := tr.draft(0); got != "" {
		t.Fatalf("edit draft = %q, want discarded", got)
	}
	if tr.posted[0].deleted {
		t.Fatal("D on an edit draft tombstoned the comment")
	}
	if tr.posted[0].body != original {
		t.Fatalf("posted body = %q, want the original %q untouched", tr.posted[0].body, original)
	}
	if got, _ := loadDraft(convoAnchor, 0); got != "" {
		t.Fatalf("D left edit draft %q on disk", got)
	}
	if m.status != "edit discarded" {
		t.Fatalf("status = %q, want %q", m.status, "edit discarded")
	}
}

// TestDeleteFocusedDiscardsPendingEditOnThreadRow: focus on the thread row
// where the only unsubmitted text is a pending edit of a comment — D discards
// the edit, mirroring editTarget's precedence (a new-comment draft would win
// if one existed).
func TestDeleteFocusedDiscardsPendingEditOnThreadRow(t *testing.T) {
	m := newTestModel(t)

	tr := m.threads[convoAnchor]
	original := tr.posted[0].body
	tr.setDraft(0, "unsaved rewrite")
	i := entryFor(t, m, convoAnchor)
	m.at = cursor{entry: i, comment: commentNone}

	m.deleteFocused()

	if tr.posted[0].deleted {
		t.Fatal("D on a thread row with a pending edit tombstoned the comment")
	}
	if tr.posted[0].body != original {
		t.Fatalf("posted body = %q, want the original %q untouched", tr.posted[0].body, original)
	}
	if got := tr.draft(0); got != "" {
		t.Fatalf("edit draft = %q, want discarded", got)
	}
	if m.status != "edit discarded" {
		t.Fatalf("status = %q, want %q", m.status, "edit discarded")
	}
}

// TestDeleteTargetNamesDraft: the palette names what D would discard — a
// draft reply when the thread row holds a new-comment draft, an unsaved edit
// when the focused comment (or the thread row, by editTarget's precedence)
// holds one.
func TestDeleteTargetNamesDraft(t *testing.T) {
	m := newTestModel(t)

	m.at = cursor{entry: entryFor(t, m, draftAnchor), comment: commentNone}
	if got, want := deleteTarget(m), "draft"; got != want {
		t.Errorf("deleteTarget on a draft-only thread = %q, want %q", got, want)
	}

	tr := m.threads[convoAnchor]
	tr.setDraft(0, "unsaved rewrite")
	i := entryFor(t, m, convoAnchor)
	m.at = cursor{entry: i, comment: 0}
	if got, want := deleteTarget(m), "unsaved edit to comment by toly"; got != want {
		t.Errorf("deleteTarget on a comment with an edit draft = %q, want %q", got, want)
	}

	m.at = cursor{entry: i, comment: commentNone}
	if got, want := deleteTarget(m), "unsaved edit to comment by toly"; got != want {
		t.Errorf("deleteTarget on a thread row with a pending edit = %q, want %q", got, want)
	}

	tr.setDraft(0, "")
	if got, want := deleteTarget(m), "thread"; got != want {
		t.Errorf("deleteTarget on a clean thread row = %q, want %q", got, want)
	}
}
