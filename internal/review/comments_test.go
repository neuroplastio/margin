package review

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

// A comment is Markdown: emphasis loses its marks, a fenced block keeps its
// lines whole, and a link keeps its text.
func TestCommentsAreMarkdown(t *testing.T) {
	m := seedModel()
	m.subspans = make(map[cursor]span)
	i := entryFor(t, m, convoAnchor)
	thr := m.threads[convoAnchor]
	thr.posted[0].body = "this is **strong** and `code`\n\n```go\nfunc f() {}\n```\n\nsee [the spec](https://example.com/spec)"
	plain := ansiRe.ReplaceAllString(strings.Join(m.appendComments(nil, i, thr, 80, 0, 0), "\n"), "")
	for _, want := range []string{"this is strong and code", "func f() {}", "the spec"} {
		if !strings.Contains(plain, want) {
			t.Errorf("thread does not show %q:\n%s", want, plain)
		}
	}
	for _, not := range []string{"**", "```", "](https"} {
		if strings.Contains(plain, not) {
			t.Errorf("thread shows Markdown marks %q:\n%s", not, plain)
		}
	}
}

// A thread's drawing is kept while what it shows holds still, and made
// again when a comment changes or the box is another width.
func TestCommentsDrawingIsKept(t *testing.T) {
	m := seedModel()
	m.subspans = make(map[cursor]span)
	i := entryFor(t, m, convoAnchor)
	thr := m.threads[convoAnchor]
	m.appendComments(nil, i, thr, 80, 0, 0)
	first := m.commentDraws[thr.anchor]
	m.appendComments(nil, i, thr, 80, 0, 0)
	if m.commentDraws[thr.anchor] != first {
		t.Error("the same thread at the same width was drawn again")
	}
	m.appendComments(nil, i, thr, 60, 0, 0)
	if m.commentDraws[thr.anchor] == first {
		t.Error("a new width reused the old drawing")
	}
	second := m.commentDraws[thr.anchor]
	thr.posted[0].body += " and more"
	m.appendComments(nil, i, thr, 60, 0, 0)
	if m.commentDraws[thr.anchor] == second {
		t.Error("an edited comment reused the old drawing")
	}
}

// A link in a document's prose is an OSC 8 hyperlink in the cells: a web URL
// as it is, a local path as a file:// URL from the document's directory, a
// leading slash from the review root, and a #fragment none (margin follows
// it).
func TestLinkURLs(t *testing.T) {
	lb := linkBase{dir: "/work/repo/docs", root: "/work/repo"}
	for href, want := range map[string]string{
		"https://example.com/a?b=1#c": "https://example.com/a?b=1#c",
		"mailto:me@example.com":       "mailto:me@example.com",
		"../README.md":                "file:///work/repo/README.md",
		"spec.md#top":                 "file:///work/repo/docs/spec.md#top",
		"/LICENSE":                    "file:///work/repo/LICENSE",
		"img/a b.png":                 "file:///work/repo/docs/img/a%20b.png",
		"#heading":                    "",
		"":                            "",
		"javascript:alert(1)":         "",
		"ftp://example.com/x":         "",
	} {
		if got := lb.url(href); got != want {
			t.Errorf("url(%q) = %q, want %q", href, got, want)
		}
	}
	if got := (linkBase{dir: "/d"}).url("/x"); got != "" {
		t.Errorf("a rooted link with no review root = %q, want none", got)
	}
}

func TestProseLinksAreHyperlinks(t *testing.T) {
	lb := linkBase{dir: "/work/repo/docs", root: "/work/repo"}
	out := strings.Join(wrapInline("see [the site](https://example.com) and [the readme](../README.md) and [up](#top)", 80, textStyle, lb), "\n")
	for _, want := range []string{
		"\x1b]8;;https://example.com\a",
		"\x1b]8;;file:///work/repo/README.md\a",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("no hyperlink %q in %q", want, out)
		}
	}
	if strings.Count(out, "\x1b]8;;") != 8 { // four link words, opened and closed
		t.Errorf("a #fragment link got a hyperlink too: %q", out)
	}
	if plain := ansi.Strip(out); plain != "see the site and the readme and up" {
		t.Errorf("the link text changed: %q", plain)
	}
	// Without a base, as in the old calls, nothing is a hyperlink.
	if strings.Contains(strings.Join(wrapInline("[x](https://example.com)", 80, textStyle), ""), "\x1b]8;") {
		t.Error("a wrap with no link base made a hyperlink")
	}
}
