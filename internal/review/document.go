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
)

// block is one entry in the document. Commentable blocks carry an anchor, which
// in the real tool is stamped into the markdown source as `^t7f3a2` so a thread
// survives the block being reworded.
type block struct {
	kind   blockKind
	text   string
	anchor string
}

type comment struct {
	author string
	body   string
	at     time.Time
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
}

func (t *thread) draft(target int) string {
	if t.drafts == nil {
		return ""
	}
	return t.drafts[target]
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
func (t *thread) summary() string {
	if d := t.draft(newCommentSlot); d != "" {
		return "✎ draft · " + truncate(firstLine(d), 54)
	}
	if i := t.pendingEdit(); i >= 0 {
		return "✎ editing · " + truncate(firstLine(t.draft(i)), 52)
	}
	if len(t.posted) == 0 {
		return "no comments"
	}
	s := t.posted[0].author + " · " + truncate(firstLine(t.posted[0].body), 52)
	if n := len(t.posted) - 1; n > 0 {
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
		{kind: blockHeading, text: "## Retry policy"},
		{kind: blockPara, anchor: "^a1", text: "Each outbound call is retried up to three times with exponential backoff starting at 100ms. Retries are attempted on 5xx responses and on transport errors, but never on 4xx."},
		{kind: blockPara, anchor: "^b2", text: "The retry budget is shared across all endpoints, so a single misbehaving upstream can starve every other caller in the process."},
		{kind: blockPara, anchor: "^c3", text: "Backoff jitter is full-jitter, computed per attempt rather than per call, which keeps retry storms from synchronising across replicas."},
		{kind: blockHeading, text: "## Circuit breaking"},
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
