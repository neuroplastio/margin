package review

import (
	"fmt"
	"strings"
	"time"
)

// blockKind distinguishes what a row in the block list is. Threads are blocks
// too — that is the whole layout model: collapsed, expanded and composing are
// three renderings of one list, not three components.
type blockKind int

const (
	blockHeading blockKind = iota
	blockPara
	blockThread
	// blockRaw is any block type whose rendering has not been decided yet —
	// code fences, quotes, tables. Its source lines are shown verbatim: for a
	// fence or a table, layout is content, and re-wrapping it would destroy it.
	blockRaw
	// blockFrontmatter is a leading YAML frontmatter block (see
	// frontmatterExtent in parse.go). It is metadata about the document, not
	// part of it: never commentable, never markable, excluded from review
	// progress. Added after the other kinds so a document with no frontmatter
	// never sees its blocks' kind values shift — those feed anchorFor's hash
	// for any block that has not yet been stamped.
	blockFrontmatter
	// blockList is a bulleted or numbered list. Split out from blockRaw
	// (2026-08-08 rendering-bugs feedback): a list item is prose and has to
	// wrap, unlike a fence or a table where the layout itself is the content.
	// Added last for the same reason blockFrontmatter was: existing kind
	// ordinals must not shift under anchorFor's hash.
	blockList
	// blockQuote is a block quote. Split out from blockRaw (2026-08-08
	// blockquote-rendering feedback): the `>` markers are markdown source, not
	// a rendering, and the quoted text is prose that has to wrap like a
	// paragraph does — the same reasoning that split blockList out earlier.
	// Added last for the same reason: existing kind ordinals must not shift
	// under anchorFor's hash.
	blockQuote
	// blockListItem is one item of a list, replacing blockList as of the
	// 2026-08-08 line-level-focus feedback: "a list of six items is six
	// things, not one." parseDoc now emits one blockListItem per item
	// (nesting flattened, same as listItemsFor already did within a single
	// blockList) instead of one blockList for the whole thing, so each item
	// gets its own focus stop, comment thread and mark for free — the
	// existing per-block machinery (m.entries, stops(), sectionAnchors())
	// needs no change to make that true. blockList itself is left defined,
	// not reused, so its ordinal (and blockQuote's after it) does not shift
	// under anchorFor's hash. Added last for the same reason as the others.
	//
	// Not solved here: a stamped, durable id for an item. stampID inserts a
	// marker at a block's byte range, and that mechanism has only ever
	// operated on top-level siblings of the document root — a list item's
	// range sits inside the list's own range, so naively stamping it would
	// insert a marker mid-list and corrupt it. stampAll skips blockListItem
	// for exactly this reason (see parse.go); an item's anchor is always
	// content-derived, same as any block was before ID-01, and does not yet
	// survive the item being reworded. That is no regression today — nothing
	// in the running app calls stampAll yet, so no block's anchor survives a
	// reword regardless of kind — but it is a real gap once ID-01/02 do get
	// wired in, and designing the on-disk marker format for a sub-list
	// position is deliberately left for whoever does that.
	blockListItem
	// blockCode is a fenced code block. Split out from blockRaw (RENDER-02):
	// unlike a table, a fence names its own language in the info string,
	// which is enough to run it through a real highlighter instead of
	// showing it as plain source. Added last for the same reason as the
	// others: existing kind ordinals must not shift under anchorFor's hash.
	blockCode
	// blockTable is a GFM table. Split out from blockRaw (RENDER-03): a
	// table's rows and columns are structured data, not lines to show
	// verbatim, so it carries a parsed table (see the table field) the
	// renderer can lay out with real column widths and alignment instead of
	// printing the source's `|` pipes as-is. Added last for the same reason
	// as the others: existing kind ordinals must not shift under
	// anchorFor's hash.
	blockTable
)

// block is one entry in the document. Commentable blocks carry an anchor, which
// in the real tool is stamped into the markdown source as `^t7f3a2` so a thread
// survives the block being reworded.
type block struct {
	kind   blockKind
	text   string
	anchor string

	// stamped reports whether anchor came from an id marker in the source
	// (ID-01) rather than being derived from content. A stamped anchor
	// survives the block being reworded; a derived one does not.
	stamped bool

	// lines holds the verbatim source of a blockRaw, which must not be
	// re-wrapped — indentation and line breaks are the content. A blockList
	// also carries it, purely so export's quoteBlock can keep reusing the
	// verbatim path; the interactive renderer uses items instead. A
	// blockQuote carries its lines with the leading `>` markers already
	// stripped, so both the renderer and quoteBlock can treat it as prose
	// with paragraph breaks rather than as source to reproduce verbatim. A
	// blockCode carries its lines with the opening/closing ``` fence already
	// stripped — the highlighter and quoteBlock both work on the code
	// itself, and lang carries what the fence said back for either to use.
	// A blockTable carries its raw source lines too, for exactly the
	// blockList reason above: quoteBlock reproduces the original `|` markup
	// verbatim on export, while the interactive renderer uses table instead.
	lines []string

	// lang is a blockCode's fence info string (e.g. "go"), used to pick a
	// chroma lexer. Empty for a fence with no language, or any other kind.
	lang string

	// table holds a blockTable's parsed rows and columns, nil for every
	// other kind. Cell text has already had RENDER-06's inline markup
	// stripped (see cellText in parse.go) — a table cell is prose too, but
	// styling it in place would tangle column-width math with ANSI byte
	// counts, so this first pass renders plain text and defers colour
	// inside cells the same way RENDER-06 deferred lists and quotes.
	table *tableBlock

	// items holds a blockListItem's own item, wrapped in a one-element slice
	// so wrapList (built for a whole blockList's items) can render it
	// unchanged. Nil for every other kind.
	items []listItem

	// listEnd is true for the last blockListItem split out of a given list.
	// Render uses it to add the usual blank line after the list as a whole
	// rather than after every item, so a six-item list still reads as one
	// list, not six blocks with gaps between them. Meaningless for any other
	// kind.
	listEnd bool

	level       int // heading level
	line        int // 1-based line the block starts on; 0 when unknown
	endLine     int // 1-based line the block ends on; 0 when unknown
	start, stop int // byte range in the source document
}

// lineRange returns the 1-based start and end line numbers of block b in the source document.
func (b block) lineRange() (startLine, endLine int) {
	if b.line <= 0 {
		return 0, 0
	}
	if b.endLine < b.line {
		return b.line, b.line
	}
	return b.line, b.endLine
}

type comment struct {
	author string
	body   string
	at     time.Time

	// deleted marks this comment as tombstoned rather than removed, per D11:
	// author and at are kept so a reply that answered it does not end up
	// dangling above nothing, and body is dropped. Set only by deleteComment
	// (or deleteThread, which tombstones every comment in a thread) — never
	// construct one with deleted set and body non-empty by hand.
	deleted bool
}

// reviewMark records how far a block has been reviewed. Marks live per
// paragraph anchor; a heading has no mark of its own, it rolls up its section —
// so marking a section and marking each of its paragraphs are the same state,
// and the two can never disagree.
type reviewMark int

const (
	markNone reviewMark = iota
	markOK              // read and accepted
	markFlag            // needs attention later
)

// commentable reports whether a block can carry a thread. Headings can: "this
// whole section is wrong" is a real comment, and it belongs on the heading.
func (b block) commentable() bool {
	return b.kind == blockHeading || b.kind == blockPara || b.kind == blockRaw || b.kind == blockList || b.kind == blockQuote || b.kind == blockListItem || b.kind == blockCode || b.kind == blockTable
}

// markable reports whether a block holds a review mark of its own. Headings do
// not — they roll up their section, so the two can never disagree.
func (b block) markable() bool {
	return b.kind == blockPara || b.kind == blockRaw || b.kind == blockList || b.kind == blockQuote || b.kind == blockListItem || b.kind == blockCode || b.kind == blockTable
}

// listItem is one item of a blockList, split out of the list's raw source so
// it can wrap independently of its siblings. prefix is the marker exactly as
// it reads on screen — indentation folded in, so a nested item gets a wider
// prefix and, at render time, a deeper hanging indent — with no separate
// nesting-depth field needed.
type listItem struct {
	prefix string // e.g. "- ", "  - ", "1. ", printed before the item's first line
	text   string // the item's own prose, collapsed, ready to re-wrap
}

// tableAlign is a column's alignment, carried over from GFM's delimiter row
// (`:---`, `---:`, `:---:`) so the renderer can pad each column the way the
// source asked for rather than always flush-left.
type tableAlign int

const (
	alignNone tableAlign = iota
	alignLeft
	alignRight
	alignCenter
)

// tableBlock is a blockTable's parsed structure: one row of header cells,
// zero or more rows of body cells, and one alignment per column. header and
// each row can be shorter than aligns — a ragged source table, or one with
// no explicit alignment row — so every reader goes through cellAt (review.go)
// rather than indexing directly.
type tableBlock struct {
	header []string
	rows   [][]string
	aligns []tableAlign
}

// next returns the mark one step around the cycle: unmarked, reviewed, flagged.
func (r reviewMark) next() reviewMark {
	switch r {
	case markNone:
		return markOK
	case markOK:
		return markFlag
	default:
		return markNone
	}
}

func (r reviewMark) glyph() string {
	switch r {
	case markOK:
		return "✓"
	case markFlag:
		return "!"
	default:
		return " "
	}
}

// rollUp summarises a section from its paragraphs' marks. Anything flagged
// dominates: a section is not clean while one paragraph still needs attention.
func rollUp(marks []reviewMark) (mark reviewMark, partial bool) {
	if len(marks) == 0 {
		return markNone, false
	}
	ok, flagged := 0, 0
	for _, m := range marks {
		switch m {
		case markOK:
			ok++
		case markFlag:
			flagged++
		}
	}
	switch {
	case flagged > 0:
		return markFlag, ok+flagged < len(marks)
	case ok == len(marks):
		return markOK, false
	case ok > 0:
		return markOK, true
	default:
		return markNone, false
	}
}

// newCommentSlot is the drafts key for text that is not yet a comment. Using one
// map for both cases means resuming a half-written reply and resuming a
// half-finished edit are the same code path.
const newCommentSlot = -1

// thread hangs off an anchor. drafts holds unsubmitted text per target: the
// new-comment slot, or the index of the posted comment being edited.
type thread struct {
	anchor string
	quote  string
	posted []comment
	drafts map[int]string

	// orphaned reports that no block in the current document carries this
	// thread's anchor any more — the block it was attached to is gone, not
	// just reworded (a reworded block keeps its stamped id, so it still
	// matches). Set by reattach on open; nothing here decides what an
	// orphaned thread looks like on screen, only that it can be told apart
	// from an attached one.
	orphaned bool

	// resolved is a single boolean, settable and clearable by either party —
	// the reviewer or an agent replying in the thread file — per D11. There
	// is no separate "addressed" state and no record of who last flipped it;
	// the whole point (see D11's export-safety rationale) is that undoing an
	// over-eager resolution costs one flip, not a negotiation. Nothing here
	// decides what a resolved thread looks like on screen — that is THREAD-02.
	resolved bool
}

// reattach matches every thread in threads to the block that now carries its
// anchor, after doc has been (re)parsed — the "on open" half of ID-01's id
// stamping: a thread persisted against a stamped id must still find its block
// even though the block's content, and so its position, has changed. A thread
// whose anchor matches no block in doc is orphaned rather than silently
// dropped, so a caller can surface it instead of losing it from view.
//
// Reattachment itself needs no bespoke matching logic — threads are already
// keyed by anchor, so any lookup by anchor (map index, this function) finds
// the same block a stamped id was written against, no matter how the text
// around it changed. What reattach adds is the part nothing else does:
// noticing when that lookup comes up empty.
func reattach(doc []block, threads map[string]*thread) {
	live := make(map[string]bool, len(doc))
	for _, b := range doc {
		if b.anchor != "" {
			live[b.anchor] = true
		}
	}
	for anchor, t := range threads {
		// Idempotent: a thread whose block has reappeared (unusual, but not
		// impossible if the same anchor is stamped back in by hand) is not
		// left stuck orphaned from an earlier reattach.
		t.orphaned = !live[anchor]
	}
}

// deleteComment marks the posted comment at index i as deleted (D11): its
// author, timestamp, and body are preserved so deleted comments can be viewed
// and restored. An out-of-range index is a no-op. Any in-progress edit of the
// comment is discarded along with it.
func (t *thread) deleteComment(i int) {
	if i < 0 || i >= len(t.posted) {
		return
	}
	t.posted[i].deleted = true
	delete(t.drafts, i)
}

// restoreComment restores a deleted comment at index i.
func (t *thread) restoreComment(i int) {
	if i < 0 || i >= len(t.posted) {
		return
	}
	t.posted[i].deleted = false
}

// toggleDeleteComment toggles the deleted status of the comment at index i.
// Returns true if the comment is now deleted, false if restored.
func (t *thread) toggleDeleteComment(i int) bool {
	if i < 0 || i >= len(t.posted) {
		return false
	}
	if t.posted[i].deleted {
		t.restoreComment(i)
		return false
	}
	t.deleteComment(i)
	return true
}

// deleteThread marks every posted comment in t as deleted.
func (t *thread) deleteThread() {
	for i := range t.posted {
		t.deleteComment(i)
	}
}

// restoreThread restores every posted comment in t.
func (t *thread) restoreThread() {
	for i := range t.posted {
		t.restoreComment(i)
	}
}

// toggleDeleteThread toggles the deleted status of every comment in t.
// If any non-deleted comment exists, all are deleted; if all are deleted, all are restored.
// Returns true if thread is now deleted, false if restored.
func (t *thread) toggleDeleteThread() bool {
	hasActive := false
	for _, c := range t.posted {
		if !c.deleted {
			hasActive = true
			break
		}
	}
	if hasActive {
		t.deleteThread()
		return true
	}
	t.restoreThread()
	return false
}

func (t *thread) draft(target int) string {
	if t.drafts == nil {
		return ""
	}
	return t.drafts[target]
}

// hasDeleted reports whether any posted comment is a tombstone — the hint an
// otherwise-empty expanded thread shows instead of "no comments yet", since
// the comments are not missing, they were deleted.
func (t *thread) hasDeleted() bool {
	for _, c := range t.posted {
		if c.deleted {
			return true
		}
	}
	return false
}

func (t *thread) setDraft(target int, body string) {
	if strings.TrimSpace(body) == "" {
		delete(t.drafts, target)
		return
	}
	if t.drafts == nil {
		t.drafts = map[int]string{}
	}
	t.drafts[target] = body
}

// pendingEdit reports the lowest-numbered comment with an unsaved edit, so the
// collapsed line can say so.
func (t *thread) pendingEdit() int {
	for i := range t.posted {
		if t.draft(i) != "" {
			return i
		}
	}
	return -1
}

// summary is the one-line form shown when the thread is not focused.
// Tombstoned comments are skipped unless showDeleted is set, so a collapsed
// line never reads "[deleted]" when the expanded thread hides it — the two
// views always describe the same set of comments. A thread whose every
// comment is tombstoned falls back to "no comments", accurate enough for a
// line the reviewer can still expand to inspect with showDeleted on.
func (t *thread) summary(showDeleted bool) string {
	if d := t.draft(newCommentSlot); d != "" {
		return "✎ draft · " + truncate(firstLine(d), 54)
	}
	if i := t.pendingEdit(); i >= 0 {
		return "✎ editing · " + truncate(firstLine(t.draft(i)), 52)
	}
	first := -1
	visible := 0
	for j := range t.posted {
		if t.posted[j].deleted && !showDeleted {
			continue
		}
		if first < 0 {
			first = j
		}
		visible++
	}
	if visible == 0 {
		return "no comments"
	}
	c := t.posted[first]
	body := c.body
	if c.deleted {
		body = "[deleted]"
	}
	s := c.author + " · " + truncate(firstLine(body), 52)
	if n := visible - 1; n > 0 {
		s += fmt.Sprintf("  (+%d)", n)
	}
	return s
}

func firstLine(s string) string { return strings.SplitN(s, "\n", 2)[0] }

func truncate(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n-1]) + "…"
}

// seedDoc builds the prototype document: a few paragraphs with three anchored
// threads in different states — an exchange with an agent, a single open
// question, and one holding an unsubmitted draft.
func seedDoc() ([]block, map[string]*thread) {
	blocks := []block{
		{kind: blockHeading, text: "## Retry policy", anchor: "^h1", level: 2},
		{kind: blockPara, anchor: "^a1", text: "Each outbound call is retried up to three times with exponential backoff starting at 100ms. Retries are attempted on 5xx responses and on transport errors, but never on 4xx."},
		{kind: blockPara, anchor: "^b2", text: "The retry budget is shared across all endpoints, so a single misbehaving upstream can starve every other caller in the process."},
		{kind: blockPara, anchor: "^c3", text: "Backoff jitter is full-jitter, computed per attempt rather than per call, which keeps retry storms from synchronising across replicas."},
		{kind: blockHeading, text: "## Circuit breaking", anchor: "^h2", level: 2},
		{kind: blockPara, anchor: "^d4", text: "A breaker opens after five consecutive failures and half-opens after thirty seconds. Half-open admits a single probe request."},
		{kind: blockPara, anchor: "^e5", text: "Breaker state is per-process and is not shared across replicas, so a failing upstream is discovered independently by each instance."},
	}

	now := time.Now()
	threads := map[string]*thread{
		"^b2": {
			anchor: "^b2",
			quote:  blocks[2].text,
			posted: []comment{
				{author: "toly", body: "Shouldn't be global — a noisy neighbour here takes out everything.", at: now.Add(-40 * time.Minute)},
				{author: "agent", body: "Changed to per-endpoint budgets, kept a global cap as a ceiling.", at: now.Add(-31 * time.Minute)},
			},
		},
		"^d4": {
			anchor: "^d4",
			quote:  blocks[5].text,
			posted: []comment{
				{author: "toly", body: "Where does thirty seconds come from? Cite the incident or drop the number.", at: now.Add(-12 * time.Minute)},
			},
		},
		"^e5": {
			anchor: "^e5",
			quote:  blocks[6].text,
			drafts: map[int]string{newCommentSlot: "This contradicts the sharing claim in ^b2 —"},
		},
	}
	return blocks, threads
}
