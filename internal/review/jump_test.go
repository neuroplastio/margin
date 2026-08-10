package review

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	uv "github.com/charmbracelet/ultraviolet"
)

// jumpDoc is a small document with headings and an in-document link, so
// jump.follow has something to resolve. Headings: "Retry policy" (slug
// retry-policy) and a duplicate "Retry policy" (slug retry-policy-1), plus a
// paragraph linking to the first.
const jumpDoc = `# Top

See [the policy](#retry-policy) and the [revision](#retry-policy-1).

## Retry policy

Backoff is exponential.

## Retry policy

Jitter is full.

## External

See [the docs](https://example.com) and [other](/elsewhere.md).
`

func jumpModel(t *testing.T) *model {
	t.Helper()
	m := newModel(parseDoc([]byte(jumpDoc)), nil)
	m.w, m.h = 100, 60
	m.render() // compute spans so jumpToEntry can centre
	return m
}

// TestHeadingSlug: GitHub-style slugs — lowercase, spaces collapse to a
// hyphen, punctuation drops (possibly leaving a double hyphen, as GitHub
// does), underscores are kept, and a hyphen tail is trimmed.
func TestHeadingSlug(t *testing.T) {
	cases := []struct{ in, want string }{
		{"Retry policy", "retry-policy"},
		{"Q & A?", "q--a"},
		{"  Spaces   Everywhere  ", "spaces-everywhere"},
		{"Under_score", "under_score"},
		{"hyphen-tail-", "hyphen-tail"},
		{"UPPER Case", "upper-case"},
	}
	for _, c := range cases {
		if got := headingSlug(c.in); got != c.want {
			t.Errorf("headingSlug(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

// TestRebuildSlugsMapsHeadings: rebuildSlugs maps each heading's slug to its
// entry index, and a duplicate heading keeps GitHub's -1 disambiguation.
func TestRebuildSlugsMapsHeadings(t *testing.T) {
	m := jumpModel(t)
	if got := m.headingSlugs["retry-policy"]; got != 2 {
		t.Errorf("slug retry-policy -> entry %d, want 2", got)
	}
	if got := m.headingSlugs["retry-policy-1"]; got != 4 {
		t.Errorf("slug retry-policy-1 -> entry %d, want 4", got)
	}
}

// TestFollowLinkJumpsToHeading: ctrl+] on the linking paragraph resolves the
// first in-document link and lands focus on the heading it names, centred.
func TestFollowLinkJumpsToHeading(t *testing.T) {
	m := jumpModel(t)
	m.at = cursor{entry: 1, comment: commentNone}
	m.followLink()

	if m.at.entry != 2 {
		t.Fatalf("followLink left focus on entry %d, want the heading entry 2", m.at.entry)
	}
	if !strings.Contains(m.status, "followed #retry-policy") {
		t.Errorf("status = %q, want it to report the followed link", m.status)
	}
	if !strings.Contains(m.status, "Retry policy") {
		t.Errorf("status = %q, want it to name the heading text", m.status)
	}
	// The jump centres the heading, and scrollAnchor agrees so clampScroll
	// does not yank the viewport back.
	if m.scrollAnchor != m.at {
		t.Errorf("scrollAnchor = %+v, want the landing %+v", m.scrollAnchor, m.at)
	}
	if m.spans[2].start == 0 || m.scroll != max(0, m.spans[2].start-29) {
		t.Errorf("scroll = %d, want the heading centred (span start %d)", m.scroll, m.spans[2].start)
	}
}

// TestFollowLinkNoLink: a block with no link reports it rather than moving.
func TestFollowLinkNoLink(t *testing.T) {
	m := jumpModel(t)
	m.at = cursor{entry: 3, comment: commentNone} // the "Backoff is exponential." paragraph
	m.followLink()
	if m.status != "no link here" {
		t.Errorf("status = %q, want %q", m.status, "no link here")
	}
	if m.at.entry != 3 {
		t.Errorf("followLink moved focus to entry %d, want it to stay", m.at.entry)
	}
}

// TestFollowLinkExternalOnly: a block whose links all point outside the
// document reports that; following a relative/absolute link is the tree-view
// milestone's job, not this one's.
func TestFollowLinkExternalOnly(t *testing.T) {
	m := jumpModel(t)
	m.at = cursor{entry: 7, comment: commentNone} // the External paragraph
	m.followLink()
	if m.status != "links here point outside this document" {
		t.Errorf("status = %q, want the outside-document message", m.status)
	}
}

// TestFollowLinkNoMatchingHeading: a fragment that names no heading is
// surfaced, not silently dropped.
func TestFollowLinkNoMatchingHeading(t *testing.T) {
	m := jumpModel(t)
	m.at = cursor{entry: 1, comment: commentNone}
	// Point the link at a slug that does not exist.
	m.entries[1].b.text = "See [the policy](#missing)."
	m.followLink()
	if m.status != "no heading matches #missing" {
		t.Errorf("status = %q, want %q", m.status, "no heading matches #missing")
	}
	if m.at.entry != 1 {
		t.Errorf("followLink moved focus to entry %d, want it to stay", m.at.entry)
	}
}

// TestPushJumpTruncatesForwardHistory: a jump from the middle of the
// jumplist drops the newer entries — vim's rule, so ctrl+i does not reach a
// branch the user has already walked off. The jump also records the position
// it left behind (the reader's current entry), so ctrl+o returns to it.
func TestPushJumpTruncatesForwardHistory(t *testing.T) {
	m := jumpModel(t)
	m.jumps = []cursor{{entry: 0, comment: commentNone}, {entry: 2, comment: commentNone}, {entry: 4, comment: commentNone}}
	m.jumpIdx = 1 // current position is entry 2
	m.at = cursor{entry: 2, comment: commentNone}

	m.pushJump(cursor{entry: 6, comment: commentNone})
	want := []cursor{
		{entry: 0, comment: commentNone},
		{entry: 2, comment: commentNone},
		{entry: 6, comment: commentNone},
	}
	if !sameJumps(m.jumps, want) {
		t.Errorf("jumps = %+v, want %+v", m.jumps, want)
	}
	if m.jumpIdx != 2 {
		t.Errorf("jumpIdx = %d, want 2", m.jumpIdx)
	}
}

// TestPushJumpDedupes: a jump that leaves and lands on the same entry is a
// no-op, so a search that lands where the last one did does not stack.
func TestPushJumpDedupes(t *testing.T) {
	m := jumpModel(t)
	m.jumps = []cursor{{entry: 0, comment: commentNone}, {entry: 2, comment: commentNone}}
	m.jumpIdx = 1
	m.at = cursor{entry: 2, comment: commentNone}
	m.pushJump(cursor{entry: 2, comment: commentNone})
	want := []cursor{{entry: 0, comment: commentNone}, {entry: 2, comment: commentNone}}
	if !sameJumps(m.jumps, want) {
		t.Errorf("jumps = %+v, want %+v (no duplicate top)", m.jumps, want)
	}
}

// TestPushJumpCaps: the jumplist never grows past maxJumps.
func TestPushJumpCaps(t *testing.T) {
	m := jumpModel(t)
	m.jumps = make([]cursor, maxJumps)
	m.jumpIdx = maxJumps - 1
	m.pushJump(cursor{entry: 99, comment: commentNone})
	if len(m.jumps) != maxJumps {
		t.Fatalf("len(jumps) = %d, want %d", len(m.jumps), maxJumps)
	}
	if m.jumps[maxJumps-1].entry != 99 {
		t.Errorf("newest entry = %d, want 99", m.jumps[maxJumps-1].entry)
	}
}

func sameJumps(a, b []cursor) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// TestJumpBackForward: ctrl+o walks to an older position and ctrl+i back to a
// newer one; at either end the walk reports instead of wrapping.
func TestJumpBackForward(t *testing.T) {
	m := jumpModel(t)
	m.jumps = []cursor{{entry: 0}, {entry: 2}, {entry: 4}}
	m.jumpIdx = 2
	m.at = cursor{entry: 4, comment: commentNone}

	m.jumpBack()
	if m.at.entry != 2 {
		t.Errorf("jumpBack left focus on entry %d, want 2", m.at.entry)
	}
	m.jumpBack()
	if m.at.entry != 0 {
		t.Errorf("jumpBack left focus on entry %d, want 0", m.at.entry)
	}
	if m.status != "" {
		t.Errorf("jumpBack at the oldest entry set status %q, want empty", m.status)
	}
	m.jumpBack()
	if m.at.entry != 0 {
		t.Errorf("jumpBack past the oldest entry moved focus to %d, want it to stay", m.at.entry)
	}
	if m.status != "at the oldest jump" {
		t.Errorf("status = %q, want %q", m.status, "at the oldest jump")
	}

	m.jumpForward()
	if m.at.entry != 2 {
		t.Errorf("jumpForward left focus on entry %d, want 2", m.at.entry)
	}
	m.jumpForward()
	if m.at.entry != 4 {
		t.Errorf("jumpForward left focus on entry %d, want 4", m.at.entry)
	}
	m.jumpForward()
	if m.at.entry != 4 {
		t.Errorf("jumpForward past the newest entry moved focus to %d, want it to stay", m.at.entry)
	}
	if m.status != "at the newest jump" {
		t.Errorf("status = %q, want %q", m.status, "at the newest jump")
	}
}

// TestFollowLinkRecordsJump: following a link records the landing, so ctrl+o
// returns to the paragraph the link came from.
func TestFollowLinkRecordsJump(t *testing.T) {
	m := jumpModel(t)
	m.at = cursor{entry: 1, comment: commentNone}
	m.jumps = []cursor{m.at}
	m.followLink()

	m.jumpBack()
	if m.at.entry != 1 {
		t.Errorf("jumpBack after followLink landed on entry %d, want the source paragraph 1", m.at.entry)
	}
}

// TestSearchJumpRecordsJumplist: a search jump is a teleport, so it lands in
// the jumplist and ctrl+o returns to wherever the search started.
func TestSearchJumpRecordsJumplist(t *testing.T) {
	m := jumpModel(t)
	m.at = cursor{entry: 0, comment: commentNone}
	m.searchMatches = []searchMatch{{line: 5, entry: 2}}
	m.gotoMatch(0)
	if m.at.entry != 2 {
		t.Fatalf("gotoMatch landed on entry %d, want 2", m.at.entry)
	}
	m.jumpBack()
	if m.at.entry != 0 {
		t.Errorf("jumpBack after a search jump landed on entry %d, want 0", m.at.entry)
	}
}

// TestLineJumpRecordsJumplist: the source-line jump records too.
func TestLineJumpRecordsJumplist(t *testing.T) {
	m := jumpModel(t)
	m.at = cursor{entry: 0, comment: commentNone}
	m.jumpToLine(4) // line 4 is the "## Retry policy" heading
	m.jumpBack()
	if m.at.entry != 0 {
		t.Errorf("jumpBack after a line jump landed on entry %d, want 0", m.at.entry)
	}
}

// TestJumpKeysRouteThroughTheRegistry: ctrl+], ctrl+o and ctrl+i resolve to
// the jump commands and behave as those commands do.
func TestJumpKeysRouteThroughTheRegistry(t *testing.T) {
	m := jumpModel(t)
	m.at = cursor{entry: 1, comment: commentNone}

	m.handleKey(teaKey("ctrl+]"))
	if m.at.entry != 2 {
		t.Errorf("ctrl+] left focus on entry %d, want the followed heading 2", m.at.entry)
	}

	m.handleKey(teaKey("ctrl+o"))
	if m.at.entry != 1 {
		t.Errorf("ctrl+o left focus on entry %d, want back to the paragraph 1", m.at.entry)
	}

	m.handleKey(teaKey("ctrl+i"))
	if m.at.entry != 2 {
		t.Errorf("ctrl+i left focus on entry %d, want forward to the heading 2", m.at.entry)
	}
}

func teaKey(s string) tea.KeyPressMsg {
	var k tea.Key
	switch s {
	case "ctrl+]":
		k = tea.Key{Code: ']', Mod: uv.ModCtrl}
	case "ctrl+o":
		k = tea.Key{Code: 'o', Mod: uv.ModCtrl}
	case "ctrl+i":
		k = tea.Key{Code: 'i', Mod: uv.ModCtrl}
	}
	return tea.KeyPressMsg(k)
}
