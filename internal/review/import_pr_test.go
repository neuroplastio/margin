package review

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

// intp is a *int literal helper for building fake gh comments.
func intp(n int) *int { return &n }

// ghFake is a scripted gh: `gh pr view` answers with prJSON and
// `gh api .../comments` answers with comments, JSON-encoded. Anything else
// fails loudly, so a test that drifts from the real call shape notices.
func ghFake(t *testing.T, prJSON string, comments []ghReviewComment) cmdRunner {
	t.Helper()
	return func(name string, args ...string) ([]byte, error) {
		if name != "gh" {
			t.Fatalf("runner invoked %q, want gh", name)
		}
		switch args[0] {
		case "pr":
			return []byte(prJSON), nil
		case "api":
			b, err := json.Marshal(comments)
			if err != nil {
				t.Fatalf("marshal fake comments: %v", err)
			}
			return b, nil
		default:
			t.Fatalf("gh invoked with unexpected args %q", args)
			return nil, nil
		}
	}
}

// importDoc writes a small three-block document (heading, two paragraphs) and
// returns its path and the two paragraph blocks, whose anchors and lines the
// tests use to place gh comments.
func importDoc(t *testing.T) (root, docPath string, p1, p2 *block) {
	t.Helper()
	root, docPath = writeDocUnderRoot(t, "# Title\n\nFirst paragraph.\n\nSecond paragraph.\n")
	doc, _, err := loadDoc(docPath)
	if err != nil {
		t.Fatalf("loadDoc: %v", err)
	}
	for i := range doc {
		switch {
		case p1 == nil && doc[i].kind == blockPara:
			p1 = &doc[i]
		case doc[i].kind == blockPara:
			p2 = &doc[i]
		}
	}
	if p1 == nil || p2 == nil {
		t.Fatal("doc did not yield two paragraphs")
	}
	return root, docPath, p1, p2
}

const testPR = `{"number":42,"url":"https://github.com/acme/crate/pull/42"}`

func TestImportPRWritesThreads(t *testing.T) {
	root, docPath, p1, p2 := importDoc(t)
	comments := []ghReviewComment{
		{Path: "doc.md", Line: intp(p1.line), Body: "keep the first paragraph", User: ghUser{Login: "toly"}, CreatedAt: "2026-08-13T09:00:00Z"},
		{Path: "doc.md", Line: intp(p2.line), Body: "second paragraph needs work", User: ghUser{Login: "ghost"}, CreatedAt: "2026-08-13T09:01:00Z"},
		{Path: "other.md", Line: intp(1), Body: "not this document", User: ghUser{Login: "toly"}, CreatedAt: "2026-08-13T09:02:00Z"},
		{Path: "doc.md", OriginalLine: intp(999), Body: "nowhere to land", User: ghUser{Login: "toly"}, CreatedAt: "2026-08-13T09:03:00Z"},
	}

	rep, err := ImportPR(docPath, ghFake(t, testPR, comments))
	if err != nil {
		t.Fatalf("ImportPR: %v", err)
	}
	if rep.Imported != 2 {
		t.Errorf("Imported = %d, want 2", rep.Imported)
	}
	if rep.OtherFiles != 1 {
		t.Errorf("OtherFiles = %d, want 1", rep.OtherFiles)
	}
	if len(rep.Unmapped) != 1 || !strings.Contains(rep.Unmapped[0], "999") || !strings.Contains(rep.Unmapped[0], "nowhere to land") {
		t.Errorf("Unmapped = %v, want one line naming line 999", rep.Unmapped)
	}
	if rep.PR != 42 || rep.Owner != "acme" || rep.Repo != "crate" {
		t.Errorf("PR identity = %d %s/%s, want 42 acme/crate", rep.PR, rep.Owner, rep.Repo)
	}

	threads, err := loadThreadsForDoc(root, "doc.md")
	if err != nil {
		t.Fatalf("loadThreadsForDoc: %v", err)
	}
	t1 := threads[p1.anchor]
	if t1 == nil {
		t.Fatalf("no thread on the first paragraph (anchor %s)", p1.anchor)
	}
	if t1.quote != "First paragraph." {
		t.Errorf("thread quote = %q, want the block's text", t1.quote)
	}
	if len(t1.posted) != 1 {
		t.Fatalf("first-paragraph thread has %d comments, want 1", len(t1.posted))
	}
	c := t1.posted[0]
	if c.author != "toly" || c.body != "keep the first paragraph" {
		t.Errorf("first-paragraph comment = %+v, want toly's", c)
	}
	if got := c.at.Format("2006-01-02"); got != "2026-08-13" {
		t.Errorf("comment time = %s, want 2026-08-13 (gh's timestamp preserved)", got)
	}

	t2 := threads[p2.anchor]
	if t2 == nil || len(t2.posted) != 1 || t2.posted[0].author != "ghost" {
		t.Errorf("second-paragraph thread not as expected: %+v", t2)
	}

	for anchor := range threads {
		if anchor != p1.anchor && anchor != p2.anchor {
			t.Errorf("unexpected thread on %s", anchor)
		}
	}
}

func TestImportPRGroupsCommentsOnTheSameBlock(t *testing.T) {
	root, docPath, p1, _ := importDoc(t)
	comments := []ghReviewComment{
		{Path: "doc.md", Line: intp(p1.line), Body: "first", User: ghUser{Login: "toly"}, CreatedAt: "2026-08-13T09:00:00Z"},
		{Path: "doc.md", Line: intp(p1.line), Body: "reply to first", User: ghUser{Login: "agent"}, CreatedAt: "2026-08-13T09:10:00Z"},
	}

	rep, err := ImportPR(docPath, ghFake(t, testPR, comments))
	if err != nil {
		t.Fatalf("ImportPR: %v", err)
	}
	if rep.Imported != 2 {
		t.Errorf("Imported = %d, want 2", rep.Imported)
	}
	threads, err := loadThreadsForDoc(root, "doc.md")
	if err != nil {
		t.Fatalf("loadThreadsForDoc: %v", err)
	}
	posted := threads[p1.anchor].posted
	if len(posted) != 2 {
		t.Fatalf("posted = %d comments, want 2 in one thread", len(posted))
	}
	if posted[0].body != "first" || posted[1].body != "reply to first" {
		t.Errorf("posted order wrong: %+v", posted)
	}
}

func TestImportPRIsIdempotent(t *testing.T) {
	root, docPath, p1, _ := importDoc(t)
	comments := []ghReviewComment{
		{Path: "doc.md", Line: intp(p1.line), Body: "keep the first paragraph", User: ghUser{Login: "toly"}, CreatedAt: "2026-08-13T09:00:00Z"},
	}

	fake := ghFake(t, testPR, comments)
	if _, err := ImportPR(docPath, fake); err != nil {
		t.Fatalf("ImportPR (first): %v", err)
	}
	rep, err := ImportPR(docPath, fake)
	if err != nil {
		t.Fatalf("ImportPR (second): %v", err)
	}
	if rep.Imported != 0 || rep.Skipped != 1 {
		t.Errorf("second import = Imported %d Skipped %d, want 0/1", rep.Imported, rep.Skipped)
	}
	threads, err := loadThreadsForDoc(root, "doc.md")
	if err != nil {
		t.Fatalf("loadThreadsForDoc: %v", err)
	}
	if n := len(threads[p1.anchor].posted); n != 1 {
		t.Errorf("thread holds %d comments after a re-import, want 1", n)
	}
}

func TestImportPRFallsBackToOriginalLine(t *testing.T) {
	root, docPath, _, p2 := importDoc(t)
	// A comment on the old side of the diff carries original_line, not line.
	comments := []ghReviewComment{
		{Path: "doc.md", OriginalLine: intp(p2.line), Body: "on the old side", User: ghUser{Login: "toly"}, CreatedAt: "2026-08-13T09:00:00Z"},
	}

	if _, err := ImportPR(docPath, ghFake(t, testPR, comments)); err != nil {
		t.Fatalf("ImportPR: %v", err)
	}
	threads, err := loadThreadsForDoc(root, "doc.md")
	if err != nil {
		t.Fatalf("loadThreadsForDoc: %v", err)
	}
	if threads[p2.anchor] == nil {
		t.Errorf("comment on the base-side line did not land on the second paragraph (%s)", p2.anchor)
	}
}

func TestImportPRPreservesResolvedFlag(t *testing.T) {
	root, docPath, p1, _ := importDoc(t)
	// A thread exists and is resolved before the import; the import must
	// append, not reset it.
	existing := &thread{anchor: p1.anchor, quote: p1.text, resolved: true}
	if err := writeThreadFile(root, "doc.md", existing); err != nil {
		t.Fatalf("writeThreadFile: %v", err)
	}
	comments := []ghReviewComment{
		{Path: "doc.md", Line: intp(p1.line), Body: "new from the PR", User: ghUser{Login: "toly"}, CreatedAt: "2026-08-13T09:00:00Z"},
	}

	if _, err := ImportPR(docPath, ghFake(t, testPR, comments)); err != nil {
		t.Fatalf("ImportPR: %v", err)
	}
	threads, err := loadThreadsForDoc(root, "doc.md")
	if err != nil {
		t.Fatalf("loadThreadsForDoc: %v", err)
	}
	th := threads[p1.anchor]
	if !th.resolved {
		t.Error("import cleared the thread's resolved flag")
	}
	if len(th.posted) != 1 {
		t.Errorf("posted = %d, want 1 appended to the resolved thread", len(th.posted))
	}
}

func TestImportPRNoPRForTheBranch(t *testing.T) {
	_, docPath, _, _ := importDoc(t)
	run := func(string, ...string) ([]byte, error) {
		return nil, errors.New(`exit status 1: no pull requests found for branch "main"`)
	}
	_, err := ImportPR(docPath, run)
	if err == nil {
		t.Fatal("a branch with no PR reported success")
	}
	if !strings.Contains(err.Error(), "no pull requests found") {
		t.Errorf("error does not carry gh's own message:\n%v", err)
	}
}

func TestImportPRRejectsMalformedGHOutput(t *testing.T) {
	_, docPath, _, _ := importDoc(t)
	run := func(name string, args ...string) ([]byte, error) {
		return []byte("not json"), nil
	}
	if _, err := ImportPR(docPath, run); err == nil {
		t.Fatal("malformed gh output reported success")
	}
}

func TestImportPRRejectsMalformedCommentTimestamp(t *testing.T) {
	_, docPath, p1, _ := importDoc(t)
	comments := []ghReviewComment{
		{Path: "doc.md", Line: intp(p1.line), Body: "bad time", User: ghUser{Login: "toly"}, CreatedAt: "yesterday"},
	}
	if _, err := ImportPR(docPath, ghFake(t, testPR, comments)); err == nil {
		t.Fatal("a comment with a bad timestamp reported success")
	}
}

func TestImportPRNoComments(t *testing.T) {
	_, docPath, _, _ := importDoc(t)
	rep, err := ImportPR(docPath, ghFake(t, testPR, nil))
	if err != nil {
		t.Fatalf("ImportPR: %v", err)
	}
	if rep.Imported != 0 || rep.Skipped != 0 || rep.OtherFiles != 0 || len(rep.Unmapped) != 0 {
		t.Errorf("empty PR reported %+v, want all zeros", rep)
	}
}

func TestImportPRFailsBeforeCallingGH(t *testing.T) {
	called := false
	run := func(string, ...string) ([]byte, error) {
		called = true
		return nil, nil
	}
	if _, err := ImportPR("/nonexistent/doc.md", run); err == nil {
		t.Fatal("ImportPR of a missing document reported success")
	}
	if called {
		t.Error("gh was called for a document that does not exist")
	}
}
