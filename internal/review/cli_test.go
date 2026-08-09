package review

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// writeDocUnderRoot creates root (with a .git marker so resolveReviewRoot finds
// it) and writes doc.md inside it, returning the doc's absolute path.
func writeDocUnderRoot(t *testing.T, content string) (root, docPath string) {
	t.Helper()
	root = t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, ".git"), 0o755); err != nil {
		t.Fatalf("mkdir .git: %v", err)
	}
	docPath = filepath.Join(root, "doc.md")
	if err := os.WriteFile(docPath, []byte(content), 0o644); err != nil {
		t.Fatalf("write doc: %v", err)
	}
	return root, docPath
}

// firstParagraphAnchor parses doc and returns the first paragraph block's
// anchor, mirroring how an agent would learn anchors from the export.
func firstParagraphAnchor(t *testing.T, docPath string) string {
	t.Helper()
	doc, err := loadDoc(docPath)
	if err != nil {
		t.Fatalf("loadDoc: %v", err)
	}
	for _, b := range doc {
		if b.kind == blockPara && b.anchor != "" {
			return b.anchor
		}
	}
	t.Fatal("no paragraph block in doc")
	return ""
}

func TestAddCommentCreatesThreadFile(t *testing.T) {
	root, docPath := writeDocUnderRoot(t, "# Title\n\nA commentable paragraph.\n")
	anchor := firstParagraphAnchor(t, docPath)

	written, err := AddComment(docPath, anchor, "agent", "fixed in rev 3")
	if err != nil {
		t.Fatalf("AddComment: %v", err)
	}
	if !strings.HasSuffix(written, filepath.Join(".margin", "threads", "doc.md", strings.TrimPrefix(anchor, "^")+".md")) {
		t.Errorf("written path = %q, want it under .margin/threads/doc.md/<id>.md", written)
	}

	threads, err := loadThreadsForDoc(root, "doc.md")
	if err != nil {
		t.Fatalf("loadThreadsForDoc: %v", err)
	}
	th := threads[anchor]
	if th == nil {
		t.Fatalf("no thread for %s", anchor)
	}
	if th.quote != "A commentable paragraph." {
		t.Errorf("quote = %q, want the block's text", th.quote)
	}
	if len(th.posted) != 1 {
		t.Fatalf("posted = %d comments, want 1", len(th.posted))
	}
	c := th.posted[0]
	if c.author != "agent" {
		t.Errorf("author = %q, want agent", c.author)
	}
	if c.body != "fixed in rev 3" {
		t.Errorf("body = %q, want the comment text", c.body)
	}
}

func TestAddCommentAppendsToExistingThread(t *testing.T) {
	root, docPath := writeDocUnderRoot(t, "# Title\n\nA commentable paragraph.\n")
	anchor := firstParagraphAnchor(t, docPath)
	if _, err := AddComment(docPath, anchor, "toly", "first comment"); err != nil {
		t.Fatalf("AddComment (first): %v", err)
	}
	if _, err := AddComment(docPath, anchor, "agent", "agent reply"); err != nil {
		t.Fatalf("AddComment (second): %v", err)
	}

	threads, err := loadThreadsForDoc(root, "doc.md")
	if err != nil {
		t.Fatalf("loadThreadsForDoc: %v", err)
	}
	posted := threads[anchor].posted
	if len(posted) != 2 {
		t.Fatalf("posted = %d comments, want 2", len(posted))
	}
	if posted[0].body != "first comment" || posted[1].body != "agent reply" {
		t.Errorf("posted order wrong: %+v", posted)
	}
}

func TestAddCommentAcceptsAnchorWithoutCaret(t *testing.T) {
	_, docPath := writeDocUnderRoot(t, "# Title\n\nA commentable paragraph.\n")
	anchor := firstParagraphAnchor(t, docPath)
	bare := strings.TrimPrefix(anchor, "^")

	if _, err := AddComment(docPath, bare, "agent", "no caret"); err != nil {
		t.Fatalf("AddComment with bare anchor: %v", err)
	}
}

func TestAddCommentRejectsUnknownAnchor(t *testing.T) {
	_, docPath := writeDocUnderRoot(t, "# Title\n\nA commentable paragraph.\n")
	_, err := AddComment(docPath, "^deadbeef", "agent", "nowhere to land")
	if err == nil {
		t.Fatal("AddComment with a nonexistent anchor reported success")
	}
	if !strings.Contains(err.Error(), "no commentable block") {
		t.Errorf("error = %q, want it to name the missing block", err)
	}
}

func TestAddCommentRejectsEmptyText(t *testing.T) {
	_, docPath := writeDocUnderRoot(t, "# Title\n\nA commentable paragraph.\n")
	anchor := firstParagraphAnchor(t, docPath)
	if _, err := AddComment(docPath, anchor, "agent", "   "); err == nil {
		t.Fatal("AddComment with blank text reported success")
	}
}

func TestAddCommentWritesFileAnAgentCanRead(t *testing.T) {
	root, docPath := writeDocUnderRoot(t, "# Title\n\nA commentable paragraph.\n")
	anchor := firstParagraphAnchor(t, docPath)
	if _, err := AddComment(docPath, anchor, "agent", "a plain reply"); err != nil {
		t.Fatalf("AddComment: %v", err)
	}

	// The thread file must round-trip through the same parser Run uses, so a
	// reviewer reopening the document sees the comment attached.
	threads, err := loadThreadsForDoc(root, "doc.md")
	if err != nil {
		t.Fatalf("loadThreadsForDoc: %v", err)
	}
	doc, err := loadDoc(docPath)
	if err != nil {
		t.Fatalf("loadDoc: %v", err)
	}
	reattach(doc, threads)
	if threads[anchor].orphaned {
		t.Error("thread marked orphaned, want it attached to its block")
	}
}

func TestDefaultAuthorFallsBackToAgent(t *testing.T) {
	// Can't force os/user.Current to fail portably; at minimum assert the
	// function returns something non-empty so the CLI never writes an empty
	// author into a thread file header.
	if a := DefaultAuthor(); a == "" {
		t.Error("DefaultAuthor() returned empty")
	}
}

// --- export ------------------------------------------------------------------

func TestExportPrintsTheReviewFromDisk(t *testing.T) {
	_, docPath := writeDocUnderRoot(t, "# Title\n\nA commentable paragraph.\n")
	anchor := firstParagraphAnchor(t, docPath)
	if _, err := AddComment(docPath, anchor, "toly", "keep the retry budget"); err != nil {
		t.Fatalf("AddComment: %v", err)
	}

	out, err := Export(docPath, false)
	if err != nil {
		t.Fatalf("Export: %v", err)
	}
	if !strings.Contains(out, "# Review of "+docPath) {
		t.Errorf("export has no document header:\n%s", out)
	}
	if !strings.Contains(out, "doc.md:3 ("+anchor+")") {
		t.Errorf("export does not name the commented block's locator:\n%s", out)
	}
	if !strings.Contains(out, "**toly:** keep the retry budget") {
		t.Errorf("export does not carry the comment:\n%s", out)
	}
	if !strings.Contains(out, "0 of 1 blocks reviewed") {
		t.Errorf("export summary should read 0 reviewed (marks are session-only):\n%s", out)
	}
}

// A headless export reports what is on disk: threads, not session marks. A
// document nobody has commented on and nothing is flagged reads as untouched,
// the same way Y on a fresh session does.
func TestExportFromDiskOfAnUntouchedDocumentSaysSo(t *testing.T) {
	_, docPath := writeDocUnderRoot(t, "# Title\n\nA commentable paragraph.\n")
	out, err := Export(docPath, false)
	if err != nil {
		t.Fatalf("Export: %v", err)
	}
	if !strings.Contains(out, "No comments and nothing flagged") {
		t.Errorf("empty export is not self-explanatory:\n%s", out)
	}
}

func TestExportHonoursIncludeResolved(t *testing.T) {
	root, docPath := writeDocUnderRoot(t, "# Title\n\nA commentable paragraph.\n")
	anchor := firstParagraphAnchor(t, docPath)
	if _, err := AddComment(docPath, anchor, "agent", "this is handled"); err != nil {
		t.Fatalf("AddComment: %v", err)
	}
	// Mark the thread resolved the way an agent would: add `resolved: true`
	// to the frontmatter and let the next load pick it up.
	threads, err := loadThreadsForDoc(root, "doc.md")
	if err != nil {
		t.Fatalf("loadThreadsForDoc: %v", err)
	}
	th := threads[anchor]
	th.resolved = true
	if err := writeThreadFile(root, "doc.md", th); err != nil {
		t.Fatalf("writeThreadFile: %v", err)
	}

	out, err := Export(docPath, false)
	if err != nil {
		t.Fatalf("Export: %v", err)
	}
	if strings.Contains(out, "this is handled") || !strings.Contains(out, "every commented block is resolved") {
		t.Errorf("default export should hide the resolved thread:\n%s", out)
	}

	out, err = Export(docPath, true)
	if err != nil {
		t.Fatalf("Export --include-resolved: %v", err)
	}
	if !strings.Contains(out, "**agent:** this is handled") || !strings.Contains(out, "— resolved") {
		t.Errorf("--include-resolved export should show the resolved thread:\n%s", out)
	}
}

func TestExportRejectsMissingFile(t *testing.T) {
	if _, err := Export(filepath.Join(t.TempDir(), "nope.md"), false); err == nil {
		t.Fatal("Export of a nonexistent file reported success")
	}
}
