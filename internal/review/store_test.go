package review

import (
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
