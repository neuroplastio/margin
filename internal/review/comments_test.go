package review

import (
	"strings"
	"testing"
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
