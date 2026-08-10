package review

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// sameThread reports whether two threads carry the same anchor, quote and
// posted comments. time.Time is compared with Equal rather than reflect's
// field-by-field check: a round trip through RFC3339Nano strips the
// monotonic reading and normalises the location, which would fail a
// DeepEqual even though the instant is identical.
func sameThread(t *testing.T, got, want *thread) {
	t.Helper()
	if got.anchor != want.anchor {
		t.Errorf("anchor = %q, want %q", got.anchor, want.anchor)
	}
	if got.quote != want.quote {
		t.Errorf("quote = %q, want %q", got.quote, want.quote)
	}
	if got.resolved != want.resolved {
		t.Errorf("resolved = %v, want %v", got.resolved, want.resolved)
	}
	if len(got.posted) != len(want.posted) {
		t.Fatalf("posted = %d comments, want %d", len(got.posted), len(want.posted))
	}
	for i := range want.posted {
		g, w := got.posted[i], want.posted[i]
		if g.author != w.author {
			t.Errorf("comment %d: author = %q, want %q", i, g.author, w.author)
		}
		if g.body != w.body {
			t.Errorf("comment %d: body = %q, want %q", i, g.body, w.body)
		}
		if g.deleted != w.deleted {
			t.Errorf("comment %d: deleted = %v, want %v", i, g.deleted, w.deleted)
		}
		if !g.at.Equal(w.at) {
			t.Errorf("comment %d: at = %v, want %v", i, g.at, w.at)
		}
	}
}

func TestMarshalParseThreadRoundTrip(t *testing.T) {
	now := time.Now()
	want := &thread{
		anchor: "^t7f3a2",
		quote:  "The retry budget is shared across all endpoints, so a single misbehaving upstream can starve every other caller in the process.",
		posted: []comment{
			{author: "toly", body: "Shouldn't be global — a noisy neighbour here takes out everything.", at: now.Add(-40 * time.Minute)},
			{author: "agent", body: "Changed to per-endpoint budgets, kept a global cap as a ceiling.", at: now.Add(-31 * time.Minute)},
		},
	}

	data := marshalThread("docs/spec.md", want)
	docPath, got, err := parseThreadFile(data)
	if err != nil {
		t.Fatalf("parseThreadFile: %v\n--- file ---\n%s", err, data)
	}
	if docPath != "docs/spec.md" {
		t.Errorf("docPath = %q, want docs/spec.md", docPath)
	}
	sameThread(t, got, want)
}

func TestMarshalParseThreadRoundTripMultilineQuoteAndBody(t *testing.T) {
	want := &thread{
		anchor: "^ab12cd34",
		quote:  "- first item\n- second item\n\n- after a blank line",
		posted: []comment{
			{
				author: "toly",
				body:   "First paragraph of feedback.\n\nSecond paragraph, after a blank line in the reply itself.",
				at:     time.Date(2026, 8, 7, 10, 15, 0, 0, time.UTC),
			},
		},
	}

	data := marshalThread("plan.md", want)
	_, got, err := parseThreadFile(data)
	if err != nil {
		t.Fatalf("parseThreadFile: %v\n--- file ---\n%s", err, data)
	}
	sameThread(t, got, want)
}

func TestMarshalParseThreadRoundTripNoComments(t *testing.T) {
	// A thread can exist with only a quote — e.g. round-tripped before any
	// reply has been posted to it yet. Not something the app writes today
	// (ensureThread never calls writeThreadFile), but the format should not
	// choke on it.
	want := &thread{anchor: "^ff00ff00", quote: "one line"}

	data := marshalThread("notes.md", want)
	_, got, err := parseThreadFile(data)
	if err != nil {
		t.Fatalf("parseThreadFile: %v\n--- file ---\n%s", err, data)
	}
	sameThread(t, got, want)
}

func TestMarshalParseThreadRoundTripResolved(t *testing.T) {
	want := &thread{
		anchor:   "^resolved1",
		quote:    "a paragraph",
		resolved: true,
		posted:   []comment{{author: "toly", body: "fixed?", at: time.Date(2026, 8, 7, 9, 0, 0, 0, time.UTC)}},
	}

	data := marshalThread("doc.md", want)
	_, got, err := parseThreadFile(data)
	if err != nil {
		t.Fatalf("parseThreadFile: %v\n--- file ---\n%s", err, data)
	}
	sameThread(t, got, want)
}

// TestMarshalThreadOmitsResolvedWhenFalse pins the on-disk shape's backward
// compatibility claim from the header comment: an unresolved thread's file
// carries no `resolved` line at all, not `resolved: false` — so a thread file
// written before THREAD-01 existed and one written by an unresolved thread
// today are indistinguishable.
func TestMarshalThreadOmitsResolvedWhenFalse(t *testing.T) {
	data := string(marshalThread("doc.md", &thread{anchor: "^x", quote: "q"}))
	if strings.Contains(data, "resolved") {
		t.Errorf("unresolved thread file should not mention resolved at all:\n%s", data)
	}
}

// TestParseThreadFileResolvedDefaultsFalse covers every thread file already
// on disk before this leg: no `resolved` field at all must read as
// unresolved, not an error and not some other default.
func TestParseThreadFileResolvedDefaultsFalse(t *testing.T) {
	src := "---\nanchor: ^abc\ndocument: doc.md\n---\n\n> quote\n"
	_, got, err := parseThreadFile([]byte(src))
	if err != nil {
		t.Fatalf("parseThreadFile: %v", err)
	}
	if got.resolved {
		t.Error("resolved = true, want false for a file with no resolved field")
	}
}

// TestParseThreadFileResolvedExplicitFalse covers a hand-edited or agent-
// unresolved file that spells the field out rather than omitting it.
func TestParseThreadFileResolvedExplicitFalse(t *testing.T) {
	src := "---\nanchor: ^abc\ndocument: doc.md\nresolved: false\n---\n\n> quote\n"
	_, got, err := parseThreadFile([]byte(src))
	if err != nil {
		t.Fatalf("parseThreadFile: %v", err)
	}
	if got.resolved {
		t.Error("resolved = true, want false for resolved: false")
	}
}

func TestParseThreadFileResolvedCaseInsensitive(t *testing.T) {
	src := "---\nanchor: ^abc\ndocument: doc.md\nresolved: True\n---\n\n> quote\n"
	_, got, err := parseThreadFile([]byte(src))
	if err != nil {
		t.Fatalf("parseThreadFile: %v", err)
	}
	if !got.resolved {
		t.Error("resolved = false, want true for resolved: True")
	}
}

// TestMarshalParseThreadRoundTripTombstonedComment covers deleted comment round-trip:
// author, timestamp, body and deleted flag round-trip cleanly.
func TestMarshalParseThreadRoundTripTombstonedComment(t *testing.T) {
	want := &thread{
		anchor: "^deleted1",
		quote:  "a paragraph",
		posted: []comment{
			{author: "toly", body: "original deleted text", at: time.Date(2026, 8, 7, 9, 0, 0, 0, time.UTC), deleted: true},
			{author: "agent", body: "still here", at: time.Date(2026, 8, 7, 9, 5, 0, 0, time.UTC)},
		},
	}

	data := marshalThread("doc.md", want)
	_, got, err := parseThreadFile(data)
	if err != nil {
		t.Fatalf("parseThreadFile: %v\n--- file ---\n%s", err, data)
	}
	sameThread(t, got, want)
}

// TestMarshalThreadWritesDeletedHeaderMarker pins the on-disk shape for deleted comments:
// the header includes `[deleted]` and preserves the comment body.
func TestMarshalThreadWritesDeletedHeaderMarker(t *testing.T) {
	th := &thread{anchor: "^x", posted: []comment{
		{author: "toly", body: "kept body", at: time.Date(2026, 8, 7, 9, 0, 0, 0, time.UTC), deleted: true},
	}}
	data := string(marshalThread("doc.md", th))
	if !strings.Contains(data, "[deleted]") {
		t.Errorf("marshalled deleted comment does not contain [deleted] header marker:\n%s", data)
	}
	if !strings.Contains(data, "kept body") {
		t.Errorf("marshalled deleted comment does not contain body:\n%s", data)
	}
}

func TestParseThreadFileRejectsMissingFrontmatter(t *testing.T) {
	if _, _, err := parseThreadFile([]byte("no frontmatter here\n")); err == nil {
		t.Fatal("expected an error for a file with no frontmatter")
	}
}

func TestParseThreadFileRejectsUnterminatedFrontmatter(t *testing.T) {
	src := "---\nanchor: ^abc\ndocument: doc.md\n"
	if _, _, err := parseThreadFile([]byte(src)); err == nil {
		t.Fatal("expected an error for frontmatter with no closing ---")
	}
}

func TestParseThreadFileRejectsMissingAnchor(t *testing.T) {
	src := "---\ndocument: doc.md\n---\n\n> quote\n"
	if _, _, err := parseThreadFile([]byte(src)); err == nil {
		t.Fatal("expected an error for frontmatter missing an anchor")
	}
}

func TestParseThreadFileRejectsMalformedCommentHeader(t *testing.T) {
	src := "---\nanchor: ^abc\ndocument: doc.md\n---\n\n> quote\n\nnot a comment header\n"
	if _, _, err := parseThreadFile([]byte(src)); err == nil {
		t.Fatal("expected an error for a body that isn't a comment header")
	}
}

func TestThreadFilePathMirrorsDocumentTree(t *testing.T) {
	got := threadFilePath("/root", "docs/spec.md", "^t7f3a2")
	want := filepath.Join("/root", ".margin", "threads", "docs", "spec.md", "t7f3a2.md")
	if got != want {
		t.Errorf("threadFilePath = %q, want %q", got, want)
	}
}

func TestWriteReadThreadFileRoundTrip(t *testing.T) {
	root := t.TempDir()
	want := &thread{
		anchor: "^deadbeef",
		quote:  "quoted block",
		posted: []comment{
			{author: "toly", body: "a comment", at: time.Now().Add(-time.Hour)},
		},
	}

	if err := writeThreadFile(root, "sub/dir/doc.md", want); err != nil {
		t.Fatalf("writeThreadFile: %v", err)
	}

	path := threadFilePath(root, "sub/dir/doc.md", want.anchor)
	docPath, got, err := readThreadFile(path)
	if err != nil {
		t.Fatalf("readThreadFile: %v", err)
	}
	if docPath != "sub/dir/doc.md" {
		t.Errorf("docPath = %q, want sub/dir/doc.md", docPath)
	}
	sameThread(t, got, want)
}

func TestWriteThreadFileOverwrites(t *testing.T) {
	root := t.TempDir()
	th := &thread{anchor: "^aaaa", quote: "q", posted: []comment{{author: "a", body: "first", at: time.Now()}}}
	if err := writeThreadFile(root, "doc.md", th); err != nil {
		t.Fatalf("writeThreadFile: %v", err)
	}
	th.posted = []comment{{author: "a", body: "second", at: time.Now()}}
	if err := writeThreadFile(root, "doc.md", th); err != nil {
		t.Fatalf("writeThreadFile (again): %v", err)
	}

	_, got, err := readThreadFile(threadFilePath(root, "doc.md", th.anchor))
	if err != nil {
		t.Fatalf("readThreadFile: %v", err)
	}
	if len(got.posted) != 1 || got.posted[0].body != "second" {
		t.Fatalf("got.posted = %+v, want a single comment with body \"second\"", got.posted)
	}
}

func TestLoadThreadsForDocReturnsEveryThreadOnDisk(t *testing.T) {
	root := t.TempDir()
	a := &thread{anchor: "^aaaa", quote: "q1", posted: []comment{{author: "toly", body: "one", at: time.Now()}}}
	b := &thread{anchor: "^bbbb", quote: "q2", posted: []comment{{author: "agent", body: "two", at: time.Now()}}}
	if err := writeThreadFile(root, "doc.md", a); err != nil {
		t.Fatalf("writeThreadFile a: %v", err)
	}
	if err := writeThreadFile(root, "doc.md", b); err != nil {
		t.Fatalf("writeThreadFile b: %v", err)
	}
	// A thread for a different document must not leak in.
	other := &thread{anchor: "^cccc", quote: "q3"}
	if err := writeThreadFile(root, "other.md", other); err != nil {
		t.Fatalf("writeThreadFile other: %v", err)
	}

	got, err := loadThreadsForDoc(root, "doc.md")
	if err != nil {
		t.Fatalf("loadThreadsForDoc: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("loadThreadsForDoc returned %d threads, want 2: %+v", len(got), got)
	}
	sameThread(t, got["^aaaa"], a)
	sameThread(t, got["^bbbb"], b)
}

func TestLoadThreadsForDocMissingDirIsNotAnError(t *testing.T) {
	root := t.TempDir()
	got, err := loadThreadsForDoc(root, "never-reviewed.md")
	if err != nil {
		t.Fatalf("loadThreadsForDoc: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("loadThreadsForDoc = %+v, want empty", got)
	}
}

func TestLoadThreadsForDocSurfacesAMalformedFile(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(threadsDir(root), "doc.md")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "bad.md"), []byte("not a thread file"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	if _, err := loadThreadsForDoc(root, "doc.md"); err == nil {
		t.Fatal("loadThreadsForDoc: want an error for a malformed thread file, got nil")
	}
}

func TestThreadStoreSaveWritesTheFile(t *testing.T) {
	root := t.TempDir()
	s := &threadStore{root: root, docPath: "doc.md"}
	th := &thread{anchor: "^feed", quote: "q", posted: []comment{{author: "toly", body: "saved", at: time.Now()}}}

	if err := s.save(th); err != nil {
		t.Fatalf("save: %v", err)
	}

	_, got, err := readThreadFile(threadFilePath(root, "doc.md", th.anchor))
	if err != nil {
		t.Fatalf("readThreadFile: %v", err)
	}
	sameThread(t, got, th)
}

func TestNilThreadStoreSaveIsANoop(t *testing.T) {
	var s *threadStore
	if err := s.save(&thread{anchor: "^x"}); err != nil {
		t.Fatalf("save on nil store: %v", err)
	}
}

func TestMarshalThreadIsHumanReadableMarkdown(t *testing.T) {
	th := &thread{
		anchor: "^t7f3a2",
		quote:  "quoted text",
		posted: []comment{{author: "toly", body: "a reply", at: time.Date(2026, 8, 7, 9, 35, 0, 0, time.UTC)}},
	}
	data := string(marshalThread("spec.md", th))

	for _, want := range []string{
		"---\n",
		"anchor: ^t7f3a2\n",
		"document: spec.md\n",
		"> quoted text\n",
		"## toly — 2026-08-07T09:35:00Z\n",
		"a reply",
	} {
		if !strings.Contains(data, want) {
			t.Errorf("marshalled thread missing %q\n--- file ---\n%s", want, data)
		}
	}
}

func TestResolveReviewRootFindsGitRoot(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, ".git"), 0o755); err != nil {
		t.Fatalf("MkdirAll .git: %v", err)
	}
	sub := filepath.Join(root, "docs", "spec")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatalf("MkdirAll sub: %v", err)
	}
	docPath := filepath.Join(sub, "plan.md")
	if err := os.WriteFile(docPath, []byte("# Plan"), 0o644); err != nil {
		t.Fatalf("WriteFile doc: %v", err)
	}

	gotRoot, gotDocPath := resolveReviewRoot(docPath)
	if gotRoot != root {
		t.Errorf("gotRoot = %q, want %q", gotRoot, root)
	}
	wantDocPath := filepath.Join("docs", "spec", "plan.md")
	if gotDocPath != wantDocPath {
		t.Errorf("gotDocPath = %q, want %q", gotDocPath, wantDocPath)
	}
}

func TestResolveReviewRootFindsMarginRoot(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, ".margin"), 0o755); err != nil {
		t.Fatalf("MkdirAll .margin: %v", err)
	}
	docPath := filepath.Join(root, "nested", "doc.md")
	if err := os.MkdirAll(filepath.Dir(docPath), 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}

	gotRoot, gotDocPath := resolveReviewRoot(docPath)
	if gotRoot != root {
		t.Errorf("gotRoot = %q, want %q", gotRoot, root)
	}
	wantDocPath := filepath.Join("nested", "doc.md")
	if gotDocPath != wantDocPath {
		t.Errorf("gotDocPath = %q, want %q", gotDocPath, wantDocPath)
	}
}

func TestResolveReviewRootFallbackToCwd(t *testing.T) {
	dir := t.TempDir()
	docPath := filepath.Join(dir, "standalone.md")
	gotRoot, gotDoc := resolveReviewRoot(docPath)
	if gotRoot == "" {
		t.Error("expected non-empty gotRoot")
	}
	if gotDoc == "" {
		t.Error("expected non-empty gotDoc")
	}
}

// TestResolveReviewRootDirectoryUsesOwnMarker: a directory path (the wait
// command's ".", a future tree review's DIR/) must find a marker in that
// directory itself — the walk must not skip the cwd's own .git/.margin, or a
// marker in an ancestor wins and the caller lands in the wrong review.
func TestResolveReviewRootDirectoryUsesOwnMarker(t *testing.T) {
	parent := t.TempDir()
	if err := os.MkdirAll(filepath.Join(parent, ".margin"), 0o755); err != nil {
		t.Fatalf("MkdirAll parent marker: %v", err)
	}
	child := filepath.Join(parent, "child")
	if err := os.MkdirAll(filepath.Join(child, ".git"), 0o755); err != nil {
		t.Fatalf("MkdirAll child marker: %v", err)
	}

	gotRoot, _ := resolveReviewRoot(child)
	if gotRoot != child {
		t.Errorf("resolveReviewRoot(%q) = %q, want %q (its own .git, not the parent's .margin)", child, gotRoot, child)
	}
}

