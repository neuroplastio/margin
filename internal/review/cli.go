// CLI entry points: the non-interactive half of margin, called by cmd/margin
// subcommands rather than the TUI. These are how an agent programmatically
// reads and writes the review without driving the interface (CLI-automation
// feedback, 2026-08-09).
package review

import (
	"fmt"
	"os/user"
	"strings"
	"time"
)

// AddComment appends a comment to the thread anchored at anchor on path,
// creating the thread (with a quote from the block) if none exists yet. It is
// the agent-automation counterpart to the TUI's `c`: an agent replies to a
// review without driving the interface, and the thread file it writes is the
// same shape the TUI writes, so the reviewer sees the reply on next open (or
// live, via the watcher).
//
// The anchor must name a commentable block in the document: an agent working
// from a stale export should be told its anchor is gone, not silently pointed
// at a thread nobody will see — the same "surface, don't drop" ethos reattach
// applies to a vanished anchor.
//
// Returns the thread file's path, so the caller can confirm where it landed.
func AddComment(path, anchor, author, text string) (string, error) {
	if strings.TrimSpace(text) == "" {
		return "", fmt.Errorf("comment text is empty")
	}
	doc, _, err := loadDoc(path)
	if err != nil {
		return "", err
	}
	anchor = normalizeAnchor(anchor)
	var blk *block
	for i := range doc {
		if doc[i].anchor == anchor && doc[i].commentable() {
			blk = &doc[i]
			break
		}
	}
	if blk == nil {
		return "", fmt.Errorf("no commentable block with anchor %s in %s", anchor, path)
	}

	root, docPath := resolveReviewRoot(path)
	threads, err := loadThreadsForDoc(root, docPath)
	if err != nil {
		return "", err
	}
	t := threads[anchor]
	if t == nil {
		t = &thread{anchor: anchor, quote: blk.text}
	}
	t.posted = append(t.posted, comment{author: author, body: text, at: time.Now()})
	if err := writeThreadFile(root, docPath, t); err != nil {
		return "", err
	}
	// The thread file is the record; the event log is the notice a listener
	// (the wait command, a tailer) acts on. A failed append must not fail the
	// reply that already landed on disk — an agent seeing an error would
	// re-post and duplicate it — so this is best-effort, exactly as the TUI
	// treats an event-log failure.
	_ = appendEvent(root, event{kind: eventCommentPosted, doc: docPath, anchor: t.anchor, author: author, comment: len(t.posted) - 1})
	return threadFilePath(root, docPath, anchor), nil
}

// Export renders the review of path as it stands on disk: parse the document,
// load its threads, reattach them, and return the same exportReview text Y and
// --stdout produce — without running the interface. It is the extract half of
// the CLI-automation feedback: an agent pulls the current state of a review in
// a pipe (CI, a script, a hook) with no terminal, no keystrokes, no window.
//
// Marks are session-only — nothing persists which blocks a reviewer has marked
// reviewed or flagged — so a headless export cannot report them. The summary
// line therefore reads "0 of N blocks reviewed" and no block is marked
// "flagged, needs attention": what the export carries is what is on disk, the
// threads, which is exactly what an agent acts on.
func Export(path string, includeResolved bool) (string, error) {
	doc, _, err := loadDoc(path)
	if err != nil {
		return "", err
	}
	root, docPath := resolveReviewRoot(path)
	threads, err := loadThreadsForDoc(root, docPath)
	if err != nil {
		return "", err
	}
	reattach(doc, threads)
	return exportReview(path, doc, threads, map[string]reviewMark{}, includeResolved, false), nil
}

// normalizeAnchor accepts an anchor with or without the leading `^` the export
// prints: an agent copying `(^a1b2c3)` from a block header and one copying
// `a1b2c3` from a thread filename mean the same block.
func normalizeAnchor(anchor string) string {
	if !strings.HasPrefix(anchor, "^") {
		return "^" + anchor
	}
	return anchor
}

// DefaultAuthor names who a programmatic comment came from: the OS user when
// determinable, "agent" otherwise. An agent can override via --author.
func DefaultAuthor() string {
	if u, err := user.Current(); err == nil && u.Username != "" {
		return u.Username
	}
	return "agent"
}
