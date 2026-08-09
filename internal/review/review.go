// Package review implements margin's document review model: a markdown
// document whose blocks carry anchored comment threads, where focusing a thread
// splices a real nvim in as the composer and blurring it collapses back to a
// one-line draft.
//
// The load-bearing bet is that the editor is disposable. A composer is a live
// child process; losing focus kills it, and the text survives as a draft keyed
// by anchor and target. Regaining focus respawns nvim with the draft restored.
// Blur and pressing "esc" are therefore the same code path, so there is only
// ever one persistence story.
package review

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/alecthomas/chroma/v2/quick"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	uv "github.com/charmbracelet/ultraviolet"
)

const (
	paneRows   = 10 // composer height; nvim gets cramped below ~8
	minPaneCol = 40
	gutterW    = 3 // focus bar + mark rule + space
	borderW    = 1
	maxMeasure = 76 // the comfortable reading measure
	footerRows = 2
	// maxCountDigits caps the `<num>gg` digit buffer. Six digits reach line
	// 999,999 — beyond any document margin will open — and stops a key-mash
	// from growing an unbounded string.
	maxCountDigits = 6
)

// damageMsg says the child wrote something, so the grid may have changed. It is
// the only thing that triggers a redraw: polling on a ticker adds a full tick of
// latency to every keystroke and burns frames when nothing moved.
type damageMsg struct{ gen int }

type heartbeatMsg time.Time

type childExitMsg struct {
	gen int
	err error
}

// cursorState mirrors the child's cursor. The emulator reports style and
// visibility through callbacks, which fire on the pty reader goroutine, so it
// needs its own lock — View() reads it from the Bubble Tea goroutine.
type cursorState struct {
	mu      sync.RWMutex
	shape   tea.CursorShape
	blink   bool
	visible bool
}

func (c *cursorState) set(shape tea.CursorShape, blink bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.shape, c.blink = shape, blink
}

func (c *cursorState) setVisible(v bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.visible = v
}

func (c *cursorState) get() (tea.CursorShape, bool, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.shape, c.blink, c.visible
}

// entry is one row of the flattened block list: either a document block or the
// thread spliced in beneath it.
type entry struct {
	b      block
	thread *thread
}

// span records where something landed in the rendered lines, for hit-testing
// and scrolling.
type span struct{ start, end int }

// cursor is where focus sits: an entry, and within a focused thread, which
// posted comment. commentNone means the entry itself rather than one of its
// comments. line is the source line a dive into a multi-line block sits on
// (the 2026-08-09 todo-review "multi-line blocks" bullet): 0 at block or
// comment level, a 1-based source line number once l has dived into a table
// or raw block. The comment and line dives are mutually exclusive — line is
// only ever set alongside commentNone.
const commentNone = -1

type cursor struct {
	entry   int
	comment int
	line    int
}

type model struct {
	path    string
	doc     []block
	src     []byte
	threads map[string]*thread
	marks   map[string]reviewMark
	entries []entry

	// raw switches the view to the document's verbatim markdown source — the
	// 2026-08-09 navigation-feature-requests "rich/raw mode toggle". Rendered
	// view re-reads m.doc's blocks; raw view re-reads m.src, the exact bytes
	// loadDoc/loadDocFrom produced (nil on a seed model, which has no source
	// of its own — the toggle then reports "no raw source" rather than
	// pretending). Focus and scroll survive the switch: the same entry stays
	// focused and the viewport re-anchors to it, so toggling back and forth
	// never loses the reader's place.
	raw bool

	// codeScroll stores horizontal scroll offsets (in visual columns) per code block anchor.
	codeScroll map[string]int

	// includeResolved controls whether exportReview (Y and --stdout alike)
	// includes resolved threads or leaves them out per D11's export-safety
	// rationale. False on every model a test builds; only Run threads through
	// the --include-resolved flag.
	includeResolved bool

	// ephemeral marks a --stdin review: the document came from a pipe and
	// nothing is persisted, so the export swaps its "edit the thread file"
	// agent instructions for wording that does not point at files that will
	// never exist. False on every model a test builds; only Run sets it.
	ephemeral bool

	// store is where a posted comment is persisted, in addition to the
	// in-memory threads map above. nil on every model a test or seedModel
	// builds — only Run sets one, since only Run is opening a real path with
	// somewhere on disk to write to.
	store *threadStore

	// watcher notices a thread file changing on disk out from under the
	// running session — an agent replying, or writing a new thread — and is
	// what makes reloadThreads fire without the reviewer reopening the
	// document. nil wherever store is nil, for the same reason.
	watcher *threadWatcher

	at           cursor
	hoveredEntry int
	comp         *composer
	gen          int

	// pendingOpen is queued when a click lands on another thread while an
	// editor is open. Blur is asynchronous — nvim has to write its buffer and
	// exit — so the new focus has to wait its turn.
	pendingOpen *cursor

	w, h   int
	scroll int
	// wheelSpeed is how many lines one mouse wheel tick scrolls the document
	// viewport. Defaults to 3 (a common terminal default); tunable per run via
	// RunOptions.WheelSpeed.
	wheelSpeed int
	// scrollAnchor is the last focus position clampScroll pulled the viewport
	// to. Re-anchoring only fires when m.at has moved since — otherwise a
	// scroll offset the user set directly (SCROLL-02/03) would snap straight
	// back to focus on the very next render. Starts at an entry index no
	// cursor legitimately has, so the first render always anchors once.
	scrollAnchor cursor
	status       string
	quitting     bool
	confirmDelete bool // true when the user must press the delete key again

	// showDeleted controls whether tombstoned comments render at all. Default
	// off: deleted comments disappear (maintainer feedback, 2026-08-09) and
	// thread.showDeleted turns them back on — still marked [deleted], since
	// the tombstone replaced the body on disk and there is no text left to
	// recover.
	showDeleted bool

	// visual reports that a blockwise selection is active (the 2026-08-09
	// visual-mode feedback's Shift+V): anchored at visualFrom, extended to
	// wherever focus moves, rendered as a background tint plus a gutter bar,
	// and cancelled by esc, a second V, or any command that is not movement
	// or selection itself. Thread entries inside the range are the
	// conversation about the document, not the document — they take no part
	// in the highlight, the block count, or the yank.
	visual     bool
	visualFrom int
	pendingKey string

	// count is the digit buffer for a `<num>gg` / `<num>G` source-line jump
	// (the 2026-08-09 navigation-feature-requests feedback, feature 2):
	// digits accumulate here and only the two line jumps consume it — vim's
	// count is a modifier, it never acts alone, so any other key clears it and
	// behaves exactly as it would with no count typed. "" means no count
	// pending.
	count string

	// freshAnchor is the anchor of a thread ensureThread created for a
	// composer open. It is the marker that lets dismiss drop a thread whose
	// first comment was never committed (the 2026-08-09 cancel-early
	// feedback) without ever touching a thread that existed before the
	// composer opened — a `:q!` on a resumed draft still discards only the
	// draft, not the thread. Cleared when dismiss runs; the thread is
	// re-created on the next open if nothing else populated it.
	freshAnchor string

	paletteOpen     bool
	paletteQuery    string
	paletteSelected int

	// searchOpen is the `/` prompt being shown at the bottom of the screen.
	// While it is open, every key routes to handleSearchKey (search.go):
	// printable characters append to searchDraft, and the matches — recomputed
	// from the draft every render — highlight the document live. enter commits
	// searchDraft as searchQuery and jumps to the next match; esc abandons the
	// edit, leaving the previous searchQuery and its highlight untouched.
	searchOpen    bool
	searchDraft   string
	searchQuery   string
	searchMatches []searchMatch
	searchCurrent int

	spans    []span
	subspans map[cursor]span // comment-level spans of the expanded thread
	paneTop  int             // line index of the first emulator row; -1 when idle
	paneW    int
	// paneLead counts the box content lines rendered ahead of the composer
	// emulator — the thread's existing conversation, shown while a new
	// comment is being written into it. render() resets it and threadLines
	// sets it, the same render-pass side channel spans and subspans use, and
	// View folds it into paneTop so mouse routing and cursor placement keep
	// pointing at the emulator's first row.
	paneLead int

	// Latency instrumentation: keyAt is stamped when a keypress is forwarded
	// and sampled on the first frame the child's response reaches, so samples
	// measure the whole round trip — host key, pty, nvim, emulator, our frame.
	keyAt     time.Time
	sawDamage bool
	samples   []time.Duration
}

// seedModel builds a model over the document seeded in code. Tests use it so
// they have known anchors to assert against.
func seedModel() *model {
	doc, threads := seedDoc()
	return newModel(doc, threads)
}

func newModel(doc []block, threads map[string]*thread) *model {
	return newModelAt("document.md", doc, threads)
}

func newModelAt(path string, doc []block, threads map[string]*thread) *model {
	if threads == nil {
		threads = map[string]*thread{}
	}
	// Opening a document is exactly the moment a thread carried over from an
	// earlier session needs to find its block again — see reattach in
	// document.go. A fresh in-memory threads map reattaches as a no-op; this
	// is where it starts mattering once threads are loaded from disk (M2).
	reattach(doc, threads)
	m := &model{
		path: path, doc: doc, threads: threads,
		marks:        map[string]reviewMark{},
		codeScroll:   map[string]int{},
		at:           cursor{entry: 0, comment: commentNone},
		hoveredEntry: -1,
		scrollAnchor: cursor{entry: -1, comment: commentNone},
		paneTop:      -1,
		wheelSpeed:   3,
	}
	m.rebuild()
	return m
}

// orphanedThreads returns every thread whose block no longer exists in the
// current document, in no particular order. Nothing renders this yet — how an
// orphan should be surfaced is a felt decision — but the data has to exist
// and be queryable before it can be shown.
func (m *model) orphanedThreads() []*thread {
	var out []*thread
	for _, t := range m.threads {
		if t.orphaned {
			out = append(out, t)
		}
	}
	return out
}

// rebuild flattens the document and its threads into the single list that
// everything else — focus, hit-testing, rendering — works against.
func (m *model) rebuild() {
	m.entries = m.entries[:0]
	for _, b := range m.doc {
		// Frontmatter is metadata, not prose — commentable() and markable()
		// exclude it, so no comment, mark or progress accounting can ever
		// land on it. But it is still something the reviewer reads, and the
		// 2026-08-09 frontmatter feedback asked for it to be reachable by
		// keyboard, so it IS a focus stop now: entry 0, landed on by g/j,
		// kept on screen by focus-following scroll, and scrolled horizontally
		// by h/l when a field overflows the measure. What changed from
		// RENDER-07 is where the "no interaction" guarantee lives — in
		// commentable/markable, not in the entry list.
		m.entries = append(m.entries, entry{b: b})
		// Raw view is the document's source, not the review of it: threads are
		// conversation that lives in .margin files, so their rows are left out
		// while raw is on — a thread row has no source lines for the focus bar
		// to ride anyway.
		if t, ok := m.threads[b.anchor]; ok && b.anchor != "" && !m.raw {
			m.entries = append(m.entries, entry{
				b:      block{kind: blockThread, anchor: b.anchor},
				thread: t,
			})
		}
	}
	if m.at.entry >= len(m.entries) {
		m.at = cursor{entry: len(m.entries) - 1, comment: commentNone}
	}
}

func (m *model) Init() tea.Cmd { return tea.Batch(m.heartbeat(), m.watcher.wait()) }

func (m *model) heartbeat() tea.Cmd {
	return tea.Tick(500*time.Millisecond, func(t time.Time) tea.Msg { return heartbeatMsg(t) })
}

func (m *model) waitForDamage() tea.Cmd {
	if m.comp == nil {
		return nil
	}
	c := m.comp
	return func() tea.Msg {
		<-c.damage
		return damageMsg{gen: c.gen}
	}
}

func (m *model) waitForChild() tea.Cmd {
	if m.comp == nil {
		return nil
	}
	c := m.comp
	return func() tea.Msg { return childExitMsg{gen: c.gen, err: c.cmd.Wait()} }
}

func (m *model) contentWidth() int {
	return max(min(m.w-2*gutterW, maxMeasure), minPaneCol)
}

// --- focus ------------------------------------------------------------------

// stops is every position focus can occupy, in document order: each entry, plus
// one per posted comment of a thread. j/k no longer walk this list — block
// level moves entry to entry and comments are reached by diving (see
// moveFocus) — but every cursor in it remains focusable by a click, a page
// landing, or a dive, so pageScroll still uses it for its end-of-document
// fallbacks and tests pin that a hidden deleted comment never appears in it.
func (m *model) stops() []cursor {
	var cs []cursor
	for i, e := range m.entries {
		cs = append(cs, cursor{entry: i, comment: commentNone})
		if e.thread != nil {
			for _, j := range m.visibleComments(e.thread) {
				cs = append(cs, cursor{entry: i, comment: j})
			}
		}
	}
	return cs
}

// visibleComments returns the posted comment indices that should render right
// now: every comment, unless it is a tombstone and showDeleted is off.
// Rendering, focus stops, hit-testing and the collapsed summary all agree
// through this one list, so a hidden deleted comment can never be focused,
// clicked or counted.
func (m *model) visibleComments(t *thread) []int {
	var out []int
	for j := range t.posted {
		if t.posted[j].deleted && !m.showDeleted {
			continue
		}
		out = append(out, j)
	}
	return out
}

// selectionRange returns the entry-index range the selection covers, lo to hi
// inclusive, or false when visual mode is off. The range is derived from the
// anchor and wherever focus is now — never stored — so any movement (j/k,
// g/G, a click, a page key) extends it, and nothing has to be kept in sync.
func (m *model) selectionRange() (lo, hi int, ok bool) {
	if !m.visual || len(m.entries) == 0 {
		return 0, 0, false
	}
	lo, hi = m.visualFrom, m.at.entry
	if lo > hi {
		lo, hi = hi, lo
	}
	return max(lo, 0), min(hi, len(m.entries)-1), true
}

func (m *model) moveFocus(d int) {
	if len(m.entries) == 0 {
		return
	}
	// Dived into a multi-line block's lines: j/k walk its source lines, and
	// flow back out at the ends like the thread dive — down past the last
	// line lands on the next entry, up past the first surfaces back onto the
	// block. The alternative, clamping inside the block until h is pressed,
	// re-creates the thread-dive feedback's complaint inside the line dive: a
	// j that goes nowhere.
	if m.at.line != 0 {
		lo, hi, ok := m.lineDiveRange()
		if !ok {
			// The block the dive is standing on shrank or vanished; surface
			// rather than strand focus on a line that no longer renders.
			m.at.line = 0
			return
		}
		np := m.at.line + d
		if np >= lo && np <= hi {
			m.at.line = np
			return
		}
		if d > 0 {
			if m.at.entry+1 < len(m.entries) {
				m.at = cursor{entry: m.at.entry + 1, comment: commentNone}
			}
		} else {
			m.at.line = 0
		}
		return
	}
	// Dived into a thread: j/k walk its visible comments, and flow back out
	// at the ends rather than trapping — down past the last comment lands on
	// the next entry, up past the first lands back on the thread row. The
	// alternative, clamping inside the thread until h is pressed, re-creates
	// the feedback's complaint inside the dive: a j that goes nowhere.
	if m.at.comment != commentNone && m.at.entry >= 0 && m.at.entry < len(m.entries) {
		if t := m.entries[m.at.entry].thread; t != nil {
			vis := m.visibleComments(t)
			p := slices.Index(vis, m.at.comment)
			if p < 0 {
				// The focused comment fell out of the visible list (a
				// tombstone hidden mid-session, say) — recover onto the
				// first visible one rather than wedging the walk.
				p = 0
			}
			if np := p + d; np >= 0 && np < len(vis) {
				m.at.comment = vis[np]
				return
			}
			if d > 0 {
				if m.at.entry+1 < len(m.entries) {
					m.at = cursor{entry: m.at.entry + 1, comment: commentNone}
				}
			} else {
				m.at.comment = commentNone
			}
			return
		}
		// A dive on a non-thread entry should never happen; if it does,
		// recover to block level rather than stranding focus.
		m.at.comment = commentNone
	}
	// Block level: j/k walk entries — blocks and thread rows — and never
	// stop on a comment. A thread is exactly one stop however long its
	// conversation, reached by diving (l) rather than by walking: the
	// 2026-08-09 todo-review feedback's fix for "j/k eats focus on threads"
	// (three presses to get past a two-comment thread) and for a
	// single-comment thread stopping focus twice. Visual mode needs no
	// special case here: entering it lifts focus off comments, so the dive
	// branch above can never fire mid-selection, and entry-only movement is
	// what a blockwise selection already wanted.
	j := m.at.entry + d
	if j < 0 {
		j = 0
	}
	if j >= len(m.entries) {
		j = len(m.entries) - 1
	}
	m.at = cursor{entry: j, comment: commentNone}
}

// gotoCount parses the `<num>gg` / `<num>G` digit buffer. The buffer is
// always non-empty, digit-only and capped when called, so the error branch is
// unreachable — it exists only because strconv.Atoi returns one.
func gotoCount(s string) int {
	n, err := strconv.Atoi(s)
	if err != nil {
		return 0
	}
	return n
}

// jumpToLine moves focus to the first entry whose block contains source line
// n (1-based), centring the block in the viewport like a search jump. "Line"
// here is a line of the markdown source — the locator reviewers and agents
// already use (L12-18 references, file:L<n> yanks) — not a rendered row,
// which shifts with the terminal width. Thread rows carry no line numbers, so
// a jump can never land on one. A line no block covers (a blank separator, or
// one past the document) clamps to the nearest block, with a status line
// saying so rather than silently landing somewhere unexpected.
func (m *model) jumpToLine(n int) {
	if len(m.entries) == 0 {
		return
	}
	target, found := m.entryForSourceLine(n)
	if found {
		m.status = ""
	} else {
		m.status = fmt.Sprintf("no source line %d — jumped to the nearest block", n)
	}
	m.at = cursor{entry: target, comment: commentNone}
	viewport := max(m.h-footerRows, 1)
	if target >= 0 && target < len(m.spans) {
		m.scroll = max(0, m.spans[target].start-viewport/2)
	}
	m.scrollAnchor = m.at
}

// entryForSourceLine returns the index of the first entry whose block spans
// source line n, and whether one does. Line-less entries (thread rows) are
// skipped — the jump targets document blocks only. When no block covers n — a
// blank separator line, or a line before the first block (n < 1) — it returns
// the first block strictly after n, or the last block when n is past the
// document.
func (m *model) entryForSourceLine(n int) (int, bool) {
	after := -1
	for i, e := range m.entries {
		lo, hi := e.b.lineRange()
		if lo <= 0 {
			continue
		}
		if n >= lo && n <= hi {
			return i, true
		}
		if after < 0 && lo > n {
			after = i
		}
	}
	if after >= 0 {
		return after, false
	}
	return len(m.entries) - 1, false
}

// lineDiveRange returns the 1-based source-line range a line-dive would walk
// for the focused entry, or ok=false when the block cannot be dived into line
// by line. Only block kinds where a source line is a rendered row can: a
// wrapped paragraph or quote has no line identity once re-wrapped, so diving
// them would be stops with nothing to see. Code blocks are deliberately left
// out too — l/h already scroll a focused code block horizontally (the code
// block horizontal scroll feedback), and layering a line dive on top of that
// would make one key do two jobs.
func (m *model) lineDiveRange() (lo, hi int, ok bool) {
	if m.at.entry < 0 || m.at.entry >= len(m.entries) {
		return 0, 0, false
	}
	b := m.entries[m.at.entry].b
	if b.kind != blockTable && b.kind != blockRaw {
		return 0, 0, false
	}
	return b.line, b.endLine, b.line > 0 && b.endLine > b.line
}

// dive steps into the thread at focus — the only way j/k ever reach a
// comment (the feedback's "dedicated dive navigation type") — or, on a
// block whose source lines each render as a row, into its lines (the
// "dive into multi-line blocks" bullet). It resolves the thread by anchor,
// so it works from the block or from its thread row, the same way c, R and D
// already find the thread at focus. Anywhere else it is a quiet no-op with a
// hint why.
func (m *model) dive() {
	if m.visual || m.at.comment != commentNone || m.at.line != 0 {
		return
	}
	// A multi-line block whose lines are individually visible dives into
	// them first: j/k walk the source lines and c/gy anchor a comment to the
	// one under focus (L12-18). A block with a thread still reaches it — the
	// thread row spliced beneath the block is its own entry, and l there
	// dives into the conversation, one j away.
	if lo, _, ok := m.lineDiveRange(); ok {
		m.at.line = lo
		return
	}
	a := m.anchorAt()
	if a == "" {
		m.status = "no thread or lines here to dive into"
		return
	}
	t := m.threads[a]
	if t == nil {
		m.status = "no thread or lines here to dive into"
		return
	}
	vis := m.visibleComments(t)
	if len(vis) == 0 {
		m.status = "no visible comments to dive into"
		return
	}
	for i, e := range m.entries {
		if e.thread == t {
			m.at = cursor{entry: i, comment: vis[0]}
			return
		}
	}
}

// surface steps back out of a dive: focus leaves a focused line for its
// block, or a comment for its thread row, putting j/k back at block level.
// Outside a dive there is no level above the document, so it does nothing.
func (m *model) surface() {
	m.at.line = 0
	m.at.comment = commentNone
}

func (m *model) pageScroll(half, down bool) {
	if len(m.spans) == 0 {
		return
	}
	viewport := max(m.h-footerRows, 1)
	dist := viewport
	if half {
		dist /= 2
	}
	if dist < 1 {
		dist = 1
	}
	if !down {
		dist = -dist
	}

	m.scroll += dist

	var docLine int
	f, ok := m.subspans[m.at]
	if !ok && m.at.entry < len(m.spans) {
		f = m.spans[m.at.entry]
		ok = true
	}
	if ok {
		docLine = f.start + dist
	} else {
		docLine = m.scroll
	}

	target, hit := m.hitTest(docLine)
	if !hit {
		if down {
			stops := m.stops()
			target = stops[len(stops)-1]
		} else {
			target = m.stops()[0]
		}
	}
	m.at = target
	m.scrollAnchor = m.at
}

// scrollViewport shifts the viewport offset directly without changing focus (m.at).
func (m *model) scrollViewport(delta int) {
	m.scroll += delta
}

// anchorAt is the document anchor the focus currently sits on, "" for headings.
func (m *model) anchorAt() string {
	if m.at.entry < 0 || m.at.entry >= len(m.entries) {
		return ""
	}
	return m.entries[m.at.entry].b.anchor
}

// ensureThread returns the thread for an anchor, creating an empty one if this
// is the first comment on that block.
func (m *model) ensureThread(anchor string) *thread {
	if t, ok := m.threads[anchor]; ok {
		return t
	}
	for _, b := range m.doc {
		if b.anchor == anchor && b.commentable() {
			t := &thread{anchor: anchor, quote: b.text}
			m.threads[anchor] = t
			m.freshAnchor = anchor
			m.rebuild()
			return t
		}
	}
	return nil
}

// --- review marks -----------------------------------------------------------

// sectionAnchors returns the paragraph anchors governed by the entry at i: just
// its own, or every paragraph under it when i is a heading.
func (m *model) sectionAnchors(i int) []string {
	if i < 0 || i >= len(m.entries) {
		return nil
	}
	e := m.entries[i]
	// A thread is not a review target; the block it hangs off is.
	if e.b.markable() {
		if e.b.anchor == "" {
			return nil
		}
		return []string{e.b.anchor}
	}
	if e.b.kind != blockHeading {
		return nil
	}
	var out []string
	for j := i + 1; j < len(m.entries); j++ {
		nb := m.entries[j].b
		if nb.kind == blockHeading {
			break
		}
		if nb.markable() && nb.anchor != "" {
			out = append(out, nb.anchor)
		}
	}
	return out
}

func (m *model) marksFor(anchors []string) []reviewMark {
	out := make([]reviewMark, 0, len(anchors))
	for _, a := range anchors {
		out = append(out, m.marks[a])
	}
	return out
}

// toggleMark applies want to the focused block, or to the whole section when
// the focus is a heading. Re-pressing the same mark clears it, so one key both
// sets and unsets.
func (m *model) toggleMark(want reviewMark) {
	anchors := m.sectionAnchors(m.at.entry)
	if len(anchors) == 0 {
		m.status = "nothing to mark here"
		return
	}
	current, partial := rollUp(m.marksFor(anchors))
	clearing := current == want && !partial

	for _, a := range anchors {
		if clearing {
			delete(m.marks, a)
		} else {
			m.marks[a] = want
		}
	}

	what := sectionLabel(anchors)
	switch {
	case clearing:
		m.status = "cleared " + what
	case want == markOK:
		m.status = "marked " + what + " reviewed"
	default:
		m.status = "flagged " + what + " for later"
	}
}

// toggleResolved flips the resolved flag on the thread anchored to the
// focused block (D11: a plain boolean, settable and clearable by either
// party) and persists the change immediately, the same way a posted comment
// is saved in dismiss — resolving is a small enough action that making it
// wait for some other save point would be surprising. Reachable whether the
// thread is collapsed or expanded: resolving is about the thread, not about
// whatever state it happens to be rendered in right now.
func (m *model) toggleResolved() {
	a := m.anchorAt()
	if a == "" || m.threads[a] == nil {
		m.status = "no thread here to resolve"
		return
	}
	t := m.threads[a]
	t.resolved = !t.resolved
	m.status = "resolved"
	if !t.resolved {
		m.status = "unresolved"
	}
	if m.store != nil {
		_ = m.store.save(t)
	}
}

// deleteFocused tombstones the focused comment (if expanded) or the whole
// thread, asking for a second press to confirm since an agent reply already
// in the thread makes deletion less obviously the reviewer's alone to do.
func (m *model) deleteFocused() tea.Cmd {
	anchor := m.anchorAt()
	if anchor == "" || m.threads[anchor] == nil {
		return nil
	}
	t := m.threads[anchor]
	
	if m.at.comment >= 0 && m.at.comment < len(t.posted) {
		if t.toggleDeleteComment(m.at.comment) {
			m.status = "comment deleted"
		} else {
			m.status = "comment restored"
		}
	} else {
		if t.toggleDeleteThread() {
			m.status = "thread deleted"
		} else {
			m.status = "thread restored"
		}
	}
	
	if m.store != nil {
		_ = m.store.save(t)
	}
	return nil
}

// sectionLabel names what a mark command is about to act on: a single block,
// or the whole section when focus sits on a heading — the same distinction
// sectionAnchors already draws. Shared by toggleMark, cycleMark and the mark
// commands' palette Target (command.go) so the wording never drifts between
// the status line and what the palette would show for the same action.
func sectionLabel(anchors []string) string {
	if len(anchors) > 1 {
		return fmt.Sprintf("section (%d blocks)", len(anchors))
	}
	return "block"
}

// cycleMark steps the focused block — or the whole section, on a heading —
// around unmarked → reviewed → flagged. One key for the common case of moving
// down a document deciding about each block in turn.
func (m *model) cycleMark() {
	anchors := m.sectionAnchors(m.at.entry)
	if len(anchors) == 0 {
		m.status = "nothing to mark here"
		return
	}
	current, partial := rollUp(m.marksFor(anchors))
	// A partially marked section resolves to its roll-up first, so one press
	// makes it consistent rather than jumping somewhere unexpected.
	want := current.next()
	if partial {
		want = current
	}

	for _, a := range anchors {
		if want == markNone {
			delete(m.marks, a)
		} else {
			m.marks[a] = want
		}
	}

	what := sectionLabel(anchors)
	switch want {
	case markOK:
		m.status = "reviewed · " + what
	case markFlag:
		m.status = "flagged · " + what
	default:
		m.status = "cleared · " + what
	}
}

// yankSelection copies the markdown source of every block in the selection —
// or of the focused block alone when no selection is active — and leaves
// visual mode. The clipboard gets source, not the rendered view: a yank is
// pasting into something that reads markdown, and the rendered gutter rules
// and highlights would be garbage there. Thread entries inside the range
// contribute nothing, since they are the conversation about the document.
func (m *model) yankSelection() tea.Cmd {
	lo, hi, ok := m.selectionRange()
	if !ok {
		lo, hi = m.at.entry, m.at.entry
	}
	var blocks []string
	for i := lo; i <= hi && i < len(m.entries); i++ {
		if e := m.entries[i]; e.thread == nil {
			if s := blockSource(e.b); s != "" {
				blocks = append(blocks, s)
			}
		}
	}
	m.visual = false
	if len(blocks) == 0 {
		m.status = "nothing to yank here"
		return nil
	}
	out := strings.Join(blocks, "\n\n") + "\n"

	// Same dual-path delivery as exportToClipboard: a helper when one is
	// installed, OSC 52 always, since it is what works over SSH.
	via, err := copyToClipboard(out)
	switch {
	case err != nil:
		m.status = "clipboard: " + err.Error()
	case via != "":
		m.status = fmt.Sprintf("yanked %d block(s) (%s)", len(blocks), via)
	default:
		m.status = "yank sent to clipboard via OSC 52 — check your terminal allows it"
	}
	return tea.SetClipboard(out)
}

// blockLineRange returns the 1-based start and end line numbers for the block at entry i.
func (m *model) blockLineRange(i int) (startLine, endLine int) {
	if i < 0 || i >= len(m.entries) {
		return 0, 0
	}
	e := m.entries[i]
	b := e.b
	if e.thread != nil {
		if a := e.b.anchor; a != "" {
			for _, docB := range m.doc {
				if docB.anchor == a {
					b = docB
					break
				}
			}
		}
	}
	return b.lineRange()
}

// selectionLineRange returns the 1-based start and end line numbers for the active selection
// or for the focused entry when no selection is active.
func (m *model) selectionLineRange() (startLine, endLine int) {
	if m.visual {
		lo, hi, ok := m.selectionRange()
		if ok {
			sLo, _ := m.blockLineRange(lo)
			_, eHi := m.blockLineRange(hi)
			if sLo > 0 && eHi > 0 {
				return sLo, eHi
			}
		}
	}
	return m.blockLineRange(m.at.entry)
}

// selectionLineRef returns the formatted line reference prefix for commenting, e.g. "L12-18: " or "L12: ".
func (m *model) selectionLineRef() string {
	startLine, endLine := m.selectionLineRange()
	if startLine <= 0 {
		return ""
	}
	if startLine == endLine {
		return fmt.Sprintf("L%d: ", startLine)
	}
	return fmt.Sprintf("L%d-%d: ", startLine, endLine)
}

// yankRef copies the line number / range reference to the clipboard (selection.yankRef / gy / yr).
func (m *model) yankRef() tea.Cmd {
	startLine, endLine := m.selectionLineRange()
	if m.at.line != 0 {
		// Dived into a block's lines: the reference is the line under focus,
		// not the whole block (see comment.new — same payload).
		startLine, endLine = m.at.line, m.at.line
	}
	if m.visual {
		m.visual = false
	}
	if startLine <= 0 {
		m.status = "nothing to reference here"
		return nil
	}

	var lineStr string
	if startLine == endLine {
		lineStr = fmt.Sprintf("L%d", startLine)
	} else {
		lineStr = fmt.Sprintf("L%d-%d", startLine, endLine)
	}

	docName := ""
	if m.path != "" && m.path != "-" {
		docName = filepath.Base(m.path)
	}

	var ref string
	if docName != "" {
		ref = fmt.Sprintf("%s:%s", docName, lineStr)
	} else {
		ref = lineStr
	}

	via, err := copyToClipboard(ref)
	if err != nil {
		m.status = "clipboard: " + err.Error()
	} else if via != "" {
		m.status = fmt.Sprintf("yanked reference %s (%s)", ref, via)
	} else {
		m.status = "yanked reference " + ref
	}
	return tea.SetClipboard(ref)
}

// exportToClipboard renders the review and puts it on the clipboard.
func (m *model) exportToClipboard() tea.Cmd {
	out := exportReview(m.path, m.doc, m.threads, m.marks, m.includeResolved, m.ephemeral)

	via, err := copyToClipboard(out)
	switch {
	case err != nil:
		m.status = "clipboard: " + err.Error()
	case via != "":
		m.status = fmt.Sprintf("review copied (%s), %d lines", via, strings.Count(out, "\n")+1)
	default:
		// No helper installed, so OSC 52 is the only path — and whether it
		// lands depends on the terminal allowing it. Say so rather than
		// claiming success we cannot verify.
		m.status = "review sent to clipboard via OSC 52 — check your terminal allows it"
	}
	// Issued regardless: OSC 52 is what works over SSH and on a bare tty.
	return tea.SetClipboard(out)
}

// reviewProgress counts paragraphs that have been looked at.
func (m *model) reviewProgress() (done, flagged, total int) {
	for _, b := range m.doc {
		if !b.markable() || b.anchor == "" {
			continue
		}
		total++
		switch m.marks[b.anchor] {
		case markOK:
			done++
		case markFlag:
			flagged++
		}
	}
	return
}

// --- composing --------------------------------------------------------------

// escHint names the exit gesture in the composer footer. A new comment opens in
// insert mode, so it takes double <Esc> — the first leaves insert mode, the
// second exits; an edit opens in normal mode, so a single <Esc> exits.
func escHint(target int) string {
	if target == newCommentSlot {
		return "esc esc keep"
	}
	return "esc keep"
}

// openComposer splices a live editor into a thread. target selects what is being
// written: newCommentSlot for a reply, or the index of a posted comment to edit.
func (m *model) openComposer(anchor string, target int) tea.Cmd {
	t := m.ensureThread(anchor)
	if t == nil {
		m.status = "nothing to comment on here"
		return nil
	}

	// A resumed draft wins over the saved text: it is the newer of the two.
	seed := t.draft(target)
	if seed == "" && target != newCommentSlot && target < len(t.posted) {
		seed = t.posted[target].body
	}

	m.gen++
	c, err := newComposer(m.gen, anchor, target, seed, m.contentWidth()-2*borderW, paneRows)
	if err != nil {
		m.status = "could not open editor: " + err.Error()
		// A thread ensureThread just created for this open would linger as an
		// empty "no comments yet" row with nothing ever committed to it — the
		// same situation dismiss's cancel-early drop handles, minus the
		// composer to dismiss.
		if m.freshAnchor == anchor && len(t.posted) == 0 && len(t.drafts) == 0 && !t.resolved {
			delete(m.threads, anchor)
			m.rebuild()
		}
		m.freshAnchor = ""
		return nil
	}
	m.comp = c
	m.status = ""
	// Focus follows the composer so scrolling keeps the pane on screen.
	for i, e := range m.entries {
		if e.thread == t {
			m.at = cursor{entry: i, comment: commentNone}
		}
	}
	return tea.Batch(m.waitForDamage(), m.waitForChild())
}

// blur asks the composer to shut down, optionally queueing a position to focus
// once it has. Nothing is torn down here: the child has to write its buffer
// first, and childExitMsg is where the outcome gets applied.
func (m *model) blur(then *cursor) {
	if m.comp == nil {
		return
	}
	m.pendingOpen = then
	m.comp.requestDraft()
}

// dismiss applies the outcome the child signalled through its exit code.
func (m *model) dismiss(err error) tea.Cmd {
	out := outcomeFromExit(err)
	body := m.comp.body()
	t := m.threads[m.comp.anchor]
	target := m.comp.target

	switch {
	case out == outcomeSubmit && body != "":
		if target == newCommentSlot {
			t.posted = append(t.posted, comment{author: "toly", body: body, at: time.Now()})
			m.status = "comment posted"
		} else if target < len(t.posted) {
			t.posted[target].body = body
			t.posted[target].at = time.Now()
			m.status = "comment updated"
		}
		t.setDraft(target, "")
		_ = clearDraft(t.anchor, target)
		if err := m.store.save(t); err != nil {
			// The comment is still in m.threads, so nothing is lost this
			// session — but it will not survive reopening the document, and
			// that gap is worth surfacing rather than quietly succeeding.
			m.status += " (not saved to disk: " + err.Error() + ")"
		}
	case out == outcomeSubmit:
		m.status = "nothing to submit"
		t.setDraft(target, "")
		_ = clearDraft(t.anchor, target)
	case out == outcomeDraft:
		// Editing a comment and exiting without changing anything must not
		// mark it as an unsaved draft — there is no diff to keep (feedback
		// 2026-08-09). The baseline is the posted comment itself: only an
		// edit of a posted comment has one, so a new comment keeps a draft
		// whenever it has text, and an edit that reverts to the posted text
		// clears a pre-existing edit draft rather than leaving stale text.
		if target != newCommentSlot && target < len(t.posted) && body == t.posted[target].body {
			t.setDraft(target, "")
			_ = clearDraft(t.anchor, target)
			m.status = "no changes"
		} else {
			t.setDraft(target, body)
			_ = saveDraft(t.anchor, target, body)
			if body == "" {
				m.status = "closed"
			} else if target == newCommentSlot {
				m.status = "draft kept on " + t.anchor
			} else {
				m.status = "edit kept unsaved"
			}
		}
	default:
		t.setDraft(target, "")
		_ = clearDraft(t.anchor, target)
		m.status = "discarded"
	}

	m.comp.close()
	m.comp = nil

	// Cancelling before committing must not leave an empty thread behind
	// (feedback 2026-08-09): a thread ensureThread created for this open, and
	// that now holds no posted comment, no draft and no resolved flag, has
	// nothing worth a row. Drop it, putting focus back on the block it hung
	// off (the thread row entry, if focus sat on one, will vanish with it). A
	// thread that existed before the composer opened is never dropped here.
	if m.freshAnchor == t.anchor && len(t.posted) == 0 && len(t.drafts) == 0 && !t.resolved {
		for i, e := range m.entries {
			if e.b.anchor == t.anchor && e.thread == nil {
				m.at = cursor{entry: i, comment: commentNone}
				break
			}
		}
		delete(m.threads, t.anchor)
	}
	m.freshAnchor = ""
	m.rebuild()

	if next := m.pendingOpen; next != nil {
		m.pendingOpen = nil
		m.at = *next
	} else if target == newCommentSlot {
		if out == outcomeSubmit && body != "" && len(t.posted) > 0 {
			m.at.comment = len(t.posted) - 1
		}
	} else if target >= 0 && target < len(t.posted) {
		m.at.comment = target
	}
	return nil
}

// --- update -----------------------------------------------------------------

func (m *model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.w, m.h = msg.Width, msg.Height
		if m.comp != nil {
			m.comp.resize(m.contentWidth()-2*borderW, paneRows)
		}
		return m, nil

	case damageMsg:
		if m.quitting || m.comp == nil || msg.gen != m.comp.gen {
			return m, nil
		}
		m.sawDamage = true
		return m, m.waitForDamage()

	case heartbeatMsg:
		if m.quitting {
			return m, nil
		}
		return m, m.heartbeat()

	case childExitMsg:
		if m.comp == nil || msg.gen != m.comp.gen {
			return m, nil
		}
		return m, m.dismiss(msg.err)

	case threadsChangedMsg:
		if m.quitting {
			return m, nil
		}
		if err := m.reloadThreads(); err != nil {
			// Surface, don't drop — same reasoning loadThreadsForDoc already
			// documents for a malformed file found on open. Live reload
			// failing once should not stop watching for the next change.
			m.status = "reload: " + err.Error()
		}
		return m, m.watcher.wait()

	case tea.MouseClickMsg:
		return m, m.handleClick(msg.Mouse())

	case tea.MouseWheelMsg:
		return m, m.handleWheel(tea.Mouse(msg))

	case tea.MouseMotionMsg:
		return m, m.handleMotion(tea.Mouse(msg))

	case tea.KeyPressMsg:
		if m.searchOpen {
			return m, m.handleSearchKey(msg)
		}
		if m.paletteOpen {
			return m, m.handlePaletteKey(msg)
		}
		return m, m.handleKey(msg)
	}
	return m, nil
}

func (m *model) handleKey(msg tea.KeyPressMsg) tea.Cmd {
	// ctrl+\ is the only key the host ever intercepts while a composer has
	// focus, and it exists purely so a wedged child cannot trap you. Submit,
	// draft and discard are all nvim mappings — see composerInit.
	if msg.String() == "ctrl+\\" {
		m.quitting = true
		if m.comp != nil {
			m.comp.close()
		}
		return tea.Quit
	}
	if m.comp != nil {
		if m.keyAt.IsZero() {
			m.keyAt = time.Now()
		}
		m.comp.sendKey(msg.Key())
		return nil
	}

	// esc cancels an active selection and does nothing otherwise — outside
	// visual mode esc has no binding at all (the composer owns its own esc).
	if m.visual && msg.String() == "esc" {
		m.visual = false
		m.pendingKey = ""
		m.count = ""
		return nil
	}

	// A digit starts or extends the pending `<num>gg` / `<num>G` count. It
	// never runs a command by itself — vim's count is a modifier for the two
	// line jumps and nothing else. The status line echoes the buffer so
	// typing `42` is not dead: `line 42`.
	if !m.visual {
		if d := msg.String(); len(d) == 1 && d[0] >= '0' && d[0] <= '9' {
			if len(m.count) < maxCountDigits {
				m.count += d
				m.status = "line " + m.count
			}
			// A digit is always consumed — one past the cap must not leak
			// into the next key as an ordinary count-clearing press.
			return nil
		}
	}

	if m.pendingKey != "" {
		pk := m.pendingKey
		combo := pk + msg.String()
		m.pendingKey = ""
		// `42gg`: a count completes the g prefix into a source-line jump
		// rather than the plain first-block move — the digit buffer was
		// accumulating while the first g waited. A plain `gg` carries no
		// count, so it falls through to the keymap lookup, where the bare
		// first `g` has already run move.first.
		if pk == "g" && msg.String() == "g" && m.count != "" {
			m.jumpToLine(gotoCount(m.count))
			m.count = ""
			return nil
		}
		if id, ok := keymap[combo]; ok {
			if cmd, ok := commandByID(id); ok {
				m.count = ""
				return cmd.Run(m, "")
			}
		}
		// Any other key after g ends the pending count too — only gg/G
		// consume it.
		m.count = ""
	}

	// Every key margin recognises resolves to a command id via keymap and runs
	// through that command's Run — see command.go. handleKey itself carries no
	// verb-specific behaviour, so a future palette invoking the same command
	// id is guaranteed to do exactly what the key does.
	if msg.String() == ":" && m.comp == nil {
		m.visual = false
		m.pendingKey = ""
		m.count = ""
		m.paletteOpen = true
		m.paletteQuery = ""
		m.paletteSelected = 0
		return nil
	}
	if msg.String() == "/" && m.comp == nil {
		m.visual = false
		m.pendingKey = ""
		m.count = ""
		m.searchOpen = true
		m.searchDraft = ""
		m.searchCurrent = 0
		return nil
	}

	id, ok := keymap[msg.String()]
	if !ok {
		// A count with no line jump after it clears — vim's count never acts
		// alone, and the unbound key is still unbound.
		m.count = ""
		return nil
	}
	cmd, ok := commandByID(id)
	if !ok {
		return nil
	}
	// The count completes only on `gg` and `G` — `<num>gg` and `<num>G` are
	// vim's two spellings of one motion, the source-line jump. A counted `g`
	// becomes a prefix waiting for its second `g` instead of running the
	// eager first-block move; a counted `G` jumps straight away.
	if msg.String() == "g" && m.count != "" {
		m.pendingKey = "g"
		return nil
	}
	if msg.String() == "G" && m.count != "" {
		m.jumpToLine(gotoCount(m.count))
		m.count = ""
		m.pendingKey = ""
		return nil
	}
	if cmd.Values != nil {
		m.visual = false
		m.pendingKey = ""
		m.paletteOpen = true
		m.paletteQuery = cmd.ID + " "
		m.paletteSelected = 0
		return nil
	}
	out := cmd.Run(m, "")
	if msg.String() == "y" || msg.String() == "g" {
		m.pendingKey = msg.String()
	} else {
		m.pendingKey = ""
	}
	// Only gg/G consume the count; any other command clears it.
	m.count = ""
	// Any command that is not movement or selection itself ends the
	// selection — vim's rule — so there is no bookkeeping about which verbs
	// keep a selection alive.
	if m.visual && !strings.HasPrefix(id, "move.") && !strings.HasPrefix(id, "selection.") {
		m.visual = false
	}
	return out
}

func (m *model) flushPendingKey() tea.Cmd {
	if m.pendingKey == "" {
		return nil
	}
	pk := m.pendingKey
	m.pendingKey = ""
	if id, ok := keymap[pk]; ok {
		if cmd, ok := commandByID(id); ok {
			return cmd.Run(m, "")
		}
	}
	return nil
}

func (m *model) handlePaletteKey(msg tea.KeyPressMsg) tea.Cmd {
	switch msg.String() {
	case "esc", "ctrl+c":
		m.paletteOpen = false
		m.paletteQuery = ""
		return nil
	case "enter":
		rows := paletteRows(m, commands, m.paletteQuery)
		if len(rows) > 0 && m.paletteSelected >= 0 && m.paletteSelected < len(rows) {
			cmd := rows[m.paletteSelected].Command
			if rows[m.paletteSelected].IsValue {
				val := rows[m.paletteSelected].Value
				m.paletteOpen = false
				m.paletteQuery = ""
				return cmd.Run(m, val)
			}
			if cmd.Values != nil {
				m.paletteQuery = cmd.ID + " "
				m.paletteSelected = 0
				return nil
			}
			m.paletteOpen = false
			m.paletteQuery = ""
			return cmd.Run(m, "")
		}
		return nil
	case "up", "ctrl+k":
		m.paletteSelected--
		if m.paletteSelected < 0 {
			m.paletteSelected = 0
		}
		return nil
	case "down", "ctrl+j", "tab":
		rows := paletteRows(m, commands, m.paletteQuery)
		m.paletteSelected++
		if m.paletteSelected >= len(rows) {
			m.paletteSelected = len(rows) - 1
			if m.paletteSelected < 0 {
				m.paletteSelected = 0
			}
		}
		return nil
	case "backspace":
		// Backspace cancels where there is nothing left to edit, and only
		// there — the maintainer's call (2026-08-09 palette-backspace
		// feedback): `:` then backspace erases the `:`, closing the
		// palette; and backing out of a staged command's seed closes too,
		// the same as esc, rather than rewinding to the command list.
		// Characters actually typed still delete one by one.
		for _, c := range commands {
			if c.Values != nil && m.paletteQuery == c.ID+" " {
				m.paletteOpen = false
				m.paletteQuery = ""
				return nil
			}
		}
		if m.paletteQuery == "" {
			m.paletteOpen = false
			return nil
		}
		m.paletteQuery = m.paletteQuery[:len(m.paletteQuery)-1]
		m.paletteSelected = 0
		return nil
	default:
		if len(msg.String()) == 1 {
			m.paletteQuery += msg.String()
			m.paletteSelected = 0
		}
		return nil
	}
}

// editFocused opens whatever the focus is sitting on for editing: a posted
// comment, or a half-written reply. It never creates a new comment — that is
// what `c` is for, and conflating the two is why editing a seeded comment used
// to silently append a reply instead.
func (m *model) editFocused() tea.Cmd {
	anchor := m.anchorAt()
	if anchor == "" {
		m.status = "nothing to edit here"
		return nil
	}
	t := m.threads[anchor]
	if t == nil {
		m.status = "no comments here yet — c to write one"
		return nil
	}
	if m.at.comment >= 0 && m.at.comment < len(t.posted) {
		return m.openComposer(anchor, m.at.comment)
	}
	if t.draft(newCommentSlot) != "" {
		return m.openComposer(anchor, newCommentSlot)
	}
	if i := t.pendingEdit(); i >= 0 {
		return m.openComposer(anchor, i)
	}
	m.status = "l to dive into the thread and pick a comment, or c to write a new one"
	return nil
}

// handleClick routes a click. Inside a live composer it positions the cursor;
// everywhere else it only moves focus — opening an editor is always an explicit
// c or e, never a side effect of clicking.
func (m *model) handleClick(mo tea.Mouse) tea.Cmd {
	line := mo.Y + m.scroll

	if m.comp != nil && m.paneTop >= 0 {
		relY, relX := line-m.paneTop, mo.X-(gutterW+borderW)
		if relY >= 0 && relY < paneRows && relX >= 0 && relX < m.paneW {
			m.comp.sendMouse(relX, relY, mo.Button, false)
			return nil
		}
	}

	target, ok := m.hitTest(line)
	if m.comp != nil {
		// Clicking away is a blur, and the click's landing spot becomes the
		// focus once the editor has finished writing.
		if ok {
			m.blur(&target)
		} else {
			m.blur(nil)
		}
		return nil
	}
	if ok {
		m.at = target
	}
	return nil
}

// handleMotion sets hoveredEntry for the block under the pointer.
func (m *model) handleMotion(mo tea.Mouse) tea.Cmd {
	line := mo.Y + m.scroll

	if m.comp != nil && m.paneTop >= 0 {
		relY, relX := line-m.paneTop, mo.X-(gutterW+borderW)
		if relY >= 0 && relY < paneRows && relX >= 0 && relX < m.paneW {
			m.hoveredEntry = -1
			return nil
		}
	}

	target, ok := m.hitTest(line)
	if ok {
		m.hoveredEntry = target.entry
	} else {
		m.hoveredEntry = -1
	}
	return nil
}

// handleWheel scrolls the document, or passes the wheel event to an open
// composer if the pointer is over it. Focus is explicitly left alone.
func (m *model) handleWheel(mo tea.Mouse) tea.Cmd {
	line := mo.Y + m.scroll

	if m.comp != nil && m.paneTop >= 0 {
		relY, relX := line-m.paneTop, mo.X-(gutterW+borderW)
		if relY >= 0 && relY < paneRows && relX >= 0 && relX < m.paneW {
			m.comp.sendMouse(relX, relY, mo.Button, false)
			return nil
		}
	}

	delta := m.wheelSpeed
	if delta < 1 {
		delta = 1
	}
	if mo.Button == uv.MouseWheelUp {
		m.scroll -= delta
	} else if mo.Button == uv.MouseWheelDown {
		m.scroll += delta
	}
	return nil
}

// hitTest maps a rendered line back to a focus position, preferring the
// finer-grained comment span when one covers the line.
func (m *model) hitTest(line int) (cursor, bool) {
	for c, s := range m.subspans {
		if line >= s.start && line <= s.end {
			return c, true
		}
	}
	for i, s := range m.spans {
		if line >= s.start && line <= s.end {
			return cursor{entry: i, comment: commentNone}, true
		}
	}
	return cursor{}, false
}

// --- rendering --------------------------------------------------------------

var (
	dimStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("244"))
	textStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("252"))
	authorStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("114"))
	draftStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("179"))
	focusStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("212"))
	hoverStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("248"))
	okStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("114"))
	flagStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("209"))
	reviewedTxt = lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
	resolvedTxt = lipgloss.NewStyle().Foreground(lipgloss.Color("108"))
	selStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("111"))
	rawStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("109"))
	codeStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("216"))
	linkStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("111")).Underline(true)
)

// headingStyles carries the visual weight for each heading depth: bold and
// brightest at level 1, plain-but-coloured at level 2, dim at level 3 and
// deeper. A terminal has no font-size axis, so weight and colour are what is
// left to show depth while scanning (RENDER-05, feedback/2026-08-08-2-
// heading-hierarchy.md) — chosen over left-padding indentation (costs
// measure width) or a gutter glyph (the gutter is already full with the
// focus bar and the section's review mark).
var headingStyles = []lipgloss.Style{
	lipgloss.NewStyle().Foreground(lipgloss.Color("212")).Bold(true),  // level 1
	lipgloss.NewStyle().Foreground(lipgloss.Color("183")).Bold(false), // level 2
	lipgloss.NewStyle().Foreground(lipgloss.Color("245")).Bold(false), // level 3+
}

// headingStyle picks the style for a heading's depth, clamping anything
// deeper than the styles defined above to the deepest one — margin's own
// documents rarely go past level 3, and a document that does should still
// render rather than panic.
func headingStyle(level int) lipgloss.Style {
	i := level - 1
	if i < 0 {
		i = 0
	}
	if i >= len(headingStyles) {
		i = len(headingStyles) - 1
	}
	return headingStyles[i]
}

// selBG is the selection background's SGR opener. It is applied by selLine
// rather than by wrapping a line in a lipgloss background style because
// lipgloss does not reassert an outer style after an inner one resets: a
// pre-styled line (chroma's code, RENDER-06's inline markup) ends its own
// spans with a full reset, which would cut the background mid-line.
const selBG = "\x1b[48;5;236m"

var selReassert = strings.NewReplacer(
	"\x1b[m", "\x1b[m"+selBG,
	"\x1b[0m", "\x1b[0m"+selBG,
)

// selLine paints the selection background across an already-rendered line by
// reasserting it after every inner reset, so one uniform treatment covers
// chroma code, inline markup and plain text alike. Empty lines stay
// untouched: the blank after a block is the gap between blocks, and painting
// it would merge every block of the selection into one slab.
func selLine(s string) string {
	if s == "" {
		return s
	}
	return selBG + selReassert.Replace(s) + "\x1b[m"
}

// gutter draws the focus bar and the review mark in a fixed-width column, so
// marks line up down the page and can be scanned without reading the prose.
// A mark is a vertical rule in the mark's colour, drawn on every line of the
// block, so a marked block reads as one continuous line down its extent
// rather than an icon repeated per line (2026-08-09 additional-UX feedback:
// "per-line icons for marks look weird"). A partial roll-up keeps the settled
// dimmed · (interaction.md). A focused block that is also selected keeps the
// focus colour, so the selection's moving end reads at a glance — vim's
// cursor in visual mode.
func (m *model) gutter(focused bool, hovered bool, selected bool, mark reviewMark, partial bool) string {
	bar := " "
	if focused {
		bar = focusStyle.Render("▌")
	} else if selected {
		bar = selStyle.Render("▌")
	} else if hovered {
		bar = hoverStyle.Render("▌")
	}
	rule := " "
	switch mark {
	case markOK:
		rule = okStyle.Render("│")
	case markFlag:
		rule = flagStyle.Render("│")
	}
	if partial && mark != markNone {
		rule = dimStyle.Render("·")
	}
	return bar + rule + " "
}

// renderRaw produces the raw source view: every line of m.src verbatim, one
// rendered line per source line — no re-wrapping, no inline styling, no block
// reconstruction. It is the "rich/raw mode toggle" of the 2026-08-09
// navigation-feature-requests: the exact bytes loadDoc/loadDocFrom read, shown
// as-is, so a reviewer can check the source of truth behind the render. The
// gutter stays — the focus bar rides the focused block's source-line range and
// the mark rule rides a marked block's — so focus, marks and selection all
// keep their meaning in raw mode, and j/k still walk blocks. Threads are
// conversation, not document, so rebuild() already leaves them out of entries
// while raw is on. Search runs over these rendered lines like any others, so
// "/" finds a string in the source too.
func (m *model) renderRaw() []string {
	m.spans = make([]span, len(m.entries))
	m.subspans = map[cursor]span{}
	m.paneLead = 0
	for i := range m.spans {
		m.spans[i] = span{start: -1, end: -1}
	}

	src := strings.TrimRight(string(m.src), "\n")
	var srcLines []string
	if src != "" {
		srcLines = strings.Split(src, "\n")
	}
	lines := make([]string, len(srcLines))

	// owner maps a source line to the entry whose block covers it, or -1 for
	// a line no block claims (a blank separator, say). Each entry's span is
	// its source-line range, so hit-testing and focus-following scroll work
	// on the raw view exactly as they do on the rendered one.
	owner := make([]int, len(srcLines))
	for i := range owner {
		owner[i] = -1
	}
	for i, e := range m.entries {
		b := e.b
		if b.line <= 0 || b.endLine < b.line {
			continue
		}
		lo, hi := b.line-1, b.endLine-1
		for n := lo; n <= hi && n < len(srcLines); n++ {
			owner[n] = i
		}
		m.spans[i] = span{start: lo, end: hi}
	}

	selLo, selHi, sel := m.selectionRange()

	for li, raw := range srcLines {
		ei := owner[li]
		focused := ei == m.at.entry
		hovered := ei >= 0 && ei == m.hoveredEntry
		selected := sel && ei >= selLo && ei <= selHi
		var mark reviewMark
		var partial bool
		if ei >= 0 {
			if m.entries[ei].b.kind == blockHeading {
				mark, partial = rollUp(m.marksFor(m.sectionAnchors(ei)))
			} else {
				mark = m.marks[m.entries[ei].b.anchor]
			}
		}
		text := raw
		if selected {
			text = selLine(text)
		}
		lines[li] = m.gutter(focused && m.at.comment == commentNone, hovered, selected, mark, partial) + text
	}
	return m.applySearch(lines)
}

// toggleRaw flips between the rendered document and its raw markdown source
// (view.raw / `\`). Focus survives the switch by anchor, not index — rebuild
// drops thread rows while raw is on, shifting entry indices, so the block
// under focus is re-found by its anchor after the rebuild — and the viewport
// re-anchors to it on the next render, so the reader stays where they were.
// Any dive or visual selection is cancelled: raw mode has no line dive (the
// lines are all visible already) and a selection measured in rendered entries
// would shift under the rebuild.
func (m *model) toggleRaw() {
	if len(m.src) == 0 {
		m.status = "no raw source for this document"
		return
	}
	a := m.anchorAt()
	m.raw = !m.raw
	m.at.line = 0
	m.at.comment = commentNone
	m.visual = false
	m.pendingKey = ""
	m.rebuild()
	if a != "" {
		for i, e := range m.entries {
			if e.b.anchor == a {
				m.at = cursor{entry: i, comment: commentNone}
				break
			}
		}
	}
	// Force clampScroll to re-anchor to the newly rebuilt focus on the next
	// render rather than holding the old offset.
	m.scrollAnchor = cursor{entry: -1, comment: commentNone}
	if m.raw {
		m.status = "raw source view — \\ toggles back"
	} else {
		m.status = "rendered view"
	}
}

// render walks the block list once, producing the lines to draw plus the span
// each entry and comment occupies. Doing it in one pass is what keeps
// hit-testing, the cursor position and scrolling from drifting apart — deriving
// any of them separately is how the cursor ended up a row off earlier.
func (m *model) render() []string {
	if m.raw {
		return m.renderRaw()
	}
	w := m.contentWidth()
	m.spans = make([]span, len(m.entries))
	m.subspans = map[cursor]span{}
	m.paneLead = 0
	var lines []string

	selLo, selHi, sel := m.selectionRange()

	for i, e := range m.entries {
		start := len(lines)
		focused := m.at.entry == i
		hovered := m.hoveredEntry == i
		selected := sel && i >= selLo && i <= selHi

		switch {
		case e.b.kind == blockHeading:
			mark, partial := rollUp(m.marksFor(m.sectionAnchors(i)))
			text := headingStyle(e.b.level).Render(e.b.text)
			if selected {
				text = selLine(text)
			}
			lines = append(lines,
				m.gutter(focused && m.at.comment == commentNone, hovered, selected, mark, partial)+
					text, "")

		case e.b.kind == blockPara, e.b.kind == blockRaw, e.b.kind == blockList, e.b.kind == blockQuote, e.b.kind == blockListItem, e.b.kind == blockCode, e.b.kind == blockTable, e.b.kind == blockFrontmatter:
			mark := m.marks[e.b.anchor]
			body := textStyle
			if mark == markOK {
				// Reviewed prose recedes so unreviewed text is what draws the eye.
				body = reviewedTxt
			}
			// A blockRaw is shown verbatim: for a code fence or a table, the
			// indentation and the line breaks are the content, so re-wrapping
			// it to the measure would destroy it. A blockList's items and a
			// blockQuote's text are prose, though, so they wrap like a
			// paragraph does — a list one item at a time with a hanging
			// indent that keeps the marker column clear (2026-08-08
			// rendering-bugs feedback, defect 2), a quote against a rule down
			// the left edge instead of a `>` on every line (2026-08-08
			// blockquote-rendering feedback). Both also carry RENDER-06's
			// inline markup now — **bold** inside a list item or a quote
			// used to reach the screen as literal asterisks, since
			// wrapList/wrapQuote wrapped with plain wrap() and left the
			// caller to flatten the block to one uniform body.Render
			// (2026-08-08 markdown-rendering feedback).
			var out []string
			rule := ""
			// preStyled marks output already carrying its own ANSI styling
			// (RENDER-06's inline markup, resolved per-fragment against body
			// above) so the loop below does not flatten it back to one
			// uniform body.Render.
			preStyled := false
			switch e.b.kind {
			case blockPara:
				out = wrapInline(e.b.text, w, body)
				preStyled = true
			case blockList, blockListItem:
				if mark != markOK {
					body = rawStyle
				}
				out = wrapList(e.b.items, w, body)
				preStyled = true
			case blockQuote:
				rule = dimStyle.Render("│ ")
				out = wrapQuote(e.b.lines, w-2, body)
				preStyled = true
			case blockCode:
				// Chroma's own colours are the highlighting, regardless of
				// review state — the same call RENDER-06 made for inline
				// code and links: markup renders as markup, not as prose
				// that happens to dim once reviewed.
				out = highlightCode(e.b.lines, e.b.lang)
				off := m.codeScroll[e.b.anchor]
				if off > 0 || hasOverflow(out, w) {
					scrolled := make([]string, len(out))
					for idx, l := range out {
						scrolled[idx] = scrollCodeLine(l, off, w)
					}
					out = scrolled
				}
				preStyled = true
			case blockFrontmatter:
				// Dimmed key/value lines, never truncated — a field wider
				// than the measure scrolls horizontally with h/l, the code
				// block treatment (the 2026-08-09 frontmatter feedback).
				out = renderFrontmatterFields(e.b, w, m.codeScroll[e.b.anchor])
				preStyled = true
			case blockTable:
				// A table is data, not markup, so it dims on review the same
				// way a paragraph or a quote's text does — body already
				// carries that (reviewedTxt above), and renderTable's header
				// row builds on it with .Bold(true), the same "build on top
				// of body" call RENDER-06 made for a paragraph's bold runs.
				out = renderTable(e.b.table, w, body)
				preStyled = true
			default:
				out = e.b.lines
				if mark != markOK {
					body = rawStyle
				}
			}
			// A line dive (a table or raw block whose lines are walked
			// individually) moves the focus bar onto the dived source line's
			// rendered row — at block level the whole block carries it. Row
			// and source line coincide 1:1 for the diveable kinds, so the
			// offset is the source line minus the block's first line.
			focusedRow := -1
			if focused && m.at.line != 0 {
				if r := m.at.line - e.b.line; r >= 0 && r < len(out) {
					focusedRow = r
				}
			}
			for row, l := range out {
				text := l
				if !preStyled {
					text = body.Render(l)
				}
				text = rule + text
				if selected {
					text = selLine(text)
				}
				rowFocused := focused && m.at.comment == commentNone && (m.at.line == 0 || row == focusedRow)
				lines = append(lines,
					m.gutter(rowFocused, hovered, selected, mark, false)+text)
			}
			// A blockListItem gets its trailing blank line only once, after
			// the list's last item, so a six-item list still reads as one
			// list with one gap after it rather than six blocks each with
			// their own — the split into per-item blocks (2026-08-08
			// line-level-focus feedback) is for focus and comments, not for
			// spacing.
			if m.threads[e.b.anchor] == nil && (e.b.kind != blockListItem || e.b.listEnd) {
				lines = append(lines, "")
			}

		default:
			lines = append(lines, m.threadLines(i, e.thread, w, len(lines))...)
			lines = append(lines, "")
		}

		m.spans[i] = span{start: start, end: len(lines) - 1}
	}
	// A search query paints its matches across the rendered lines and
	// recomputes the match list, so the highlight and n/N's walk always
	// describe the same screen (search.go). Runs after the entry loop because
	// it needs the spans just computed to map each line back to its entry.
	lines = m.applySearch(lines)
	return lines
}

// appendComments renders the thread's visible posted comments onto body — a
// head line (focus mark, author, age, unsaved-edit/deleted marker) over the
// wrapped text, a blank line between comments — registering each comment's
// subspan as it goes. off is how many content lines will sit ahead of the
// comments in the final box (the resolved badge's two lines when it shows);
// base is the absolute line the box starts at, so the subspans come out in
// the same coordinates as everything else. Both views that show the
// conversation — the expanded thread, and the composer box of a new reply —
// render it through here, so a comment reads and hit-tests the same whether
// you are reading the thread or writing into it.
func (m *model) appendComments(body []string, i int, t *thread, w, base, off int) []string {
	visible := m.visibleComments(t)
	for vi, j := range visible {
		c := t.posted[j]
		focused := m.at.comment == j
		// +borderW for the box's top border, which Render adds around this
		// content.
		commentStart := base + borderW + off + len(body)
		head := "  " + authorStyle.Render(c.author) + dimStyle.Render(" · "+humanAge(c.at))
		if d := t.draft(j); d != "" {
			head += draftStyle.Render("  ✎ edited, unsaved")
		} else if c.deleted {
			head += dimStyle.Render("  [deleted]")
		}
		body = append(body, head)

		bodyPrefix := "  "
		if focused {
			bodyPrefix = focusStyle.Render("▌ ")
		}

		if d := t.draft(j); d != "" {
			for _, l := range wrap(d, w-6) {
				if focused {
					body = append(body, bodyPrefix+focusStyle.Render(l))
				} else {
					body = append(body, bodyPrefix+l)
				}
			}
		} else if c.deleted {
			bodyText := c.body
			if bodyText == "" {
				bodyText = "[deleted]"
			}
			for _, l := range wrap(bodyText, w-6) {
				body = append(body, bodyPrefix+dimStyle.Render(l))
			}
		} else {
			for _, l := range wrap(c.body, w-6) {
				if focused {
					body = append(body, bodyPrefix+focusStyle.Render(l))
				} else {
					body = append(body, bodyPrefix+l)
				}
			}
		}

		if m.subspans != nil {
			m.subspans[cursor{entry: i, comment: j}] = span{
				start: commentStart, end: base + borderW + off + len(body) - 1,
			}
		}
		if vi < len(visible)-1 {
			body = append(body, "")
		}
	}
	return body
}

// threadLines renders a thread in whichever of the three states it is in:
// collapsed to a line, expanded for reading, or hosting the live editor. base is
// the absolute line the thread starts at, so comment spans come out in the same
// coordinates as everything else.
func (m *model) threadLines(i int, t *thread, w, base int) []string {
	focused := m.at.entry == i
	// Width includes the border, so the content area is w-2*borderW — which is
	// exactly the width the composer emulator was created at. Making it any
	// narrower would let lipgloss re-wrap the emulator's already-wrapped lines,
	// orphaning words and pulling the cursor off the text.
	box := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("240")).
		Width(w)
	if focused {
		box = box.BorderForeground(lipgloss.Color("212"))
	}
	indent := func(ls []string) []string {
		for j := range ls {
			ls[j] = strings.Repeat(" ", gutterW) + ls[j]
		}
		return ls
	}

	if m.comp != nil && m.comp.anchor == t.anchor {
		// Writing a new comment into a thread shows the thread: a reply
		// written against the conversation it joins reads as a reply, where
		// the same box holding only the editor read as starting a parallel
		// thread (2026-08-09 todo-review feedback). The conversation renders
		// through the same appendComments the expanded view uses, a dim rule
		// marks where reading stops and writing starts, and paneLead tells
		// View how far down the emulator's first row moved. An edit keeps
		// the composer-only box: the text being edited is already live in
		// the emulator, so the want the feedback names — seeing what you are
		// replying to — does not apply to it.
		var lead []string
		if m.comp.target == newCommentSlot {
			if t.resolved {
				lead = append(lead, resolvedTxt.Render("✓ resolved"), "")
			}
			// off is 0: whatever lead already holds (the badge) is counted
			// by appendComments itself through len(body).
			if lead = m.appendComments(lead, i, t, w, base, 0); len(lead) > 0 {
				lead = append(lead, "", dimStyle.Render(strings.Repeat("─", w-2*borderW)))
			}
		}
		m.paneLead = len(lead)
		body := append(lead, strings.Split(m.comp.em.Render(), "\n")...)
		return indent(strings.Split(box.Render(strings.Join(body, "\n")), "\n"))
	}

	// Collapsed: literally one line, no box. A bordered one-liner costs three
	// rows, and with a thread every few paragraphs that crowds out the document
	// — which is what the reader came for. A resolved thread swaps the plain
	// rule for a dim green checkmark: still one line, still no box, but a
	// glance down the gutter now tells collapsed-and-done apart from
	// collapsed-and-open without expanding either. It does not also mute the
	// draft/pending-edit colour below — unsaved text is the more urgent signal
	// of the two and should not go quiet just because the thread was resolved
	// around it.
	if !focused {
		marker := dimStyle.Render("│ ")
		if t.resolved {
			marker = resolvedTxt.Render("✓ ")
		}
		style := dimStyle
		if t.draft(newCommentSlot) != "" || t.pendingEdit() >= 0 {
			style = draftStyle
		}
		return []string{strings.Repeat(" ", gutterW) + marker + style.Render(t.summary(m.showDeleted))}
	}

	// Expanded: the whole exchange, ready to read. Comments are separately
	// focusable so `e` knows which one to open. A resolved thread gets a
	// two-line badge ahead of the comments rather than hiding or dimming
	// them — resolving says "this is handled", not "this no longer matters";
	// the conversation that led there is still what the reviewer came to
	// read.
	resolvedOffset := 0
	if t.resolved {
		resolvedOffset = 2
	}
	body := m.appendComments(nil, i, t, w, base, resolvedOffset)
	if d := t.draft(newCommentSlot); d != "" {
		if len(body) > 0 {
			body = append(body, "")
		}
		body = append(body, "  "+draftStyle.Render("draft · unsubmitted"))
		for _, l := range wrap(d, w-6) {
			body = append(body, "  "+l)
		}
	}
	if len(body) == 0 {
		if t.hasDeleted() {
			body = []string{"  " + dimStyle.Render("comments deleted — T to reveal")}
		} else {
			body = []string{"  " + dimStyle.Render("no comments yet — c to write one")}
		}
	}
	if t.resolved {
		body = append([]string{resolvedTxt.Render("✓ resolved"), ""}, body...)
	}
	return indent(strings.Split(box.Render(strings.Join(body, "\n")), "\n"))
}

func humanAge(t time.Time) string {
	d := time.Since(t)
	switch {
	case d < time.Minute:
		return "just now"
	case d < time.Hour:
		return fmt.Sprintf("%dm ago", int(d.Minutes()))
	default:
		return fmt.Sprintf("%dh ago", int(d.Hours()))
	}
}

func (m *model) View() tea.View {
	if m.quitting {
		return tea.NewView("")
	}
	// Bubble Tea calls View once before the first WindowSizeMsg, when the
	// terminal size is still unknown. Rendering then is not merely wasted: View
	// clamps m.scroll as a side effect, and against a zero-height viewport that
	// clamp lands on 1 — an offset every later render inherits, which silently
	// hides the document's first line for the rest of the session.
	if m.w == 0 || m.h == 0 {
		v := tea.NewView("")
		v.AltScreen = true
		return v
	}
	// Sample only on a frame the child's output actually triggered. Bubble Tea
	// also renders immediately after the keypress itself, and counting that
	// frame would report a latency of roughly zero.
	if m.sawDamage && !m.keyAt.IsZero() {
		m.samples = append(m.samples, time.Since(m.keyAt))
		m.keyAt, m.sawDamage = time.Time{}, false
	}

	lines := m.render()
	// The emulator's first row sits one line below the focused thread's box
	// border, past whatever conversation render() drew ahead of it — both
	// measured by the render pass that just ran.
	m.paneTop = -1
	if m.comp != nil && m.at.entry < len(m.spans) {
		m.paneTop = m.spans[m.at.entry].start + borderW + m.paneLead
		m.paneW = m.contentWidth() - 2*borderW
	}

	var paletteBox string
	paletteHeight := 0
	if m.paletteOpen {
		paletteBox = m.renderPalette(m.w)
		paletteHeight = strings.Count(paletteBox, "\n") + 1
	}
	var searchBox string
	searchHeight := 0
	if m.searchOpen {
		searchBox = m.renderSearchPrompt(m.w)
		searchHeight = strings.Count(searchBox, "\n") + 1
	}

	viewport := max(m.h-footerRows-paletteHeight-searchHeight, 1)
	m.scroll = m.clampScroll(len(lines), viewport)
	visible := lines[min(m.scroll, len(lines)):min(m.scroll+viewport, len(lines))]

	var b strings.Builder
	b.WriteString(strings.Join(visible, "\n"))
	b.WriteString("\n\n ")
	if m.comp != nil {
		what := "new comment"
		if m.comp.target != newCommentSlot {
			what = "editing comment"
		}
		b.WriteString(authorStyle.Render(m.comp.mode()) + dimStyle.Render(
			"  "+what+"  ·  ctrl+s save · "+escHint(m.comp.target)+" · SPC c k discard · click away to blur"))
	} else if m.paletteOpen {
		b.WriteString(paletteBox)
	} else if m.searchOpen {
		b.WriteString(searchBox)
	} else if m.visual {
		n := 0
		if lo, hi, ok := m.selectionRange(); ok {
			for i := lo; i <= hi; i++ {
				if m.entries[i].thread == nil {
					n++
				}
			}
		}
		b.WriteString(selStyle.Render("-- VISUAL --") + dimStyle.Render(fmt.Sprintf(
			"  %d block(s)  ·  j/k extend · y yank · V/esc cancel", n)))
		if m.status != "" {
			b.WriteString(dimStyle.Render("   " + m.status))
		}
	} else {
		done, flagged, total := m.reviewProgress()
		progress := fmt.Sprintf("%d/%d reviewed", done, total)
		if flagged > 0 {
			progress += fmt.Sprintf(" · %s", flagStyle.Render(fmt.Sprintf("%d flagged", flagged)))
		}
		hint := "j/k move · c comment · e edit · space mark · Y copy review · q quit   "
		if m.raw {
			hint = "\\ rendered · j/k move · c comment · space mark · Y copy · q quit   "
		}
		b.WriteString(dimStyle.Render(hint) +
			dimStyle.Render(progress))
		if m.status != "" {
			b.WriteString(dimStyle.Render("   " + m.status))
		}
	}

	v := tea.NewView(b.String())
	// margin owns the terminal while it runs and hands it back untouched —
	// scrollback and all — on exit. Rendering inline left the shell prompt
	// occupying a row above the document and cost a second row to keep the
	// frame from scrolling the terminal, so two of the reader's rows went to
	// chrome nobody asked for.
	v.AltScreen = true
	v.MouseMode = tea.MouseModeCellMotion
	if m.comp != nil && m.paneTop >= 0 {
		if shape, blink, shown := m.comp.cur.get(); shown {
			cp := m.comp.em.CursorPosition()
			if y := m.paneTop + cp.Y - m.scroll; y >= 0 && y < viewport {
				c := tea.NewCursor(gutterW+borderW+cp.X, y)
				c.Shape, c.Blink = shape, blink
				v.Cursor = c
			}
		}
	}
	return v
}

// clampScroll owns the scroll offset. It only pulls the viewport to follow
// focus when focus has actually moved since the last render — the rest of the
// time m.scroll is whatever it already was, merely clamped to stay in range.
// That is what leaves room for a user-driven scroll (SCROLL-02/03) to hold:
// today nothing sets m.scroll directly, so this only changes behaviour once
// something does, but re-deriving the offset from focus unconditionally, the
// way this used to work, would undo that scroll on the very next render.
func (m *model) clampScroll(total, viewport int) int {
	if total <= viewport {
		return 0
	}
	s := m.scroll
	if m.at != m.scrollAnchor {
		m.scrollAnchor = m.at
		f, ok := m.subspans[m.at]
		if !ok && m.at.entry < len(m.spans) {
			f = m.spans[m.at.entry]
			ok = true
		}
		if ok {
			if f.start < s {
				s = f.start
			}
			if f.end >= s+viewport {
				s = f.end - viewport + 1
			}
		}
	}
	return max(0, min(s, total-viewport))
}

func wrap(s string, w int) []string {
	if w <= 0 {
		return []string{s}
	}
	var out, line []string
	n := 0
	for _, word := range strings.Fields(s) {
		if n > 0 && n+1+len(word) > w {
			out = append(out, strings.Join(line, " "))
			line, n = nil, 0
		}
		line = append(line, word)
		n += len(word) + 1
	}
	if len(line) > 0 {
		out = append(out, strings.Join(line, " "))
	}
	if len(out) == 0 {
		return []string{""}
	}
	return out
}

// glueWord is one unbreakable unit for wrapping: a run of text with no
// whitespace in it, possibly stitched together from more than one inlineRun
// (e.g. "en**hanced**" is one word made of a plain fragment and a bold one).
// Splitting on whitespace this way, rather than treating each inlineRun as
// its own word, is what keeps "en**hanced**" from picking up a space it
// never had in the source.
type glueWord struct {
	frags []inlineRun
	width int
}

// glueWords turns a paragraph's inline runs (see parseInline) into the
// unbreakable units wrapInline wraps. A run's own text is split on
// whitespace; adjacent fragments with no whitespace between them — whether
// from the same run or a neighbouring one — join into a single glueWord, so
// wrapping never inserts a space markup removed.
func glueWords(s string) []glueWord {
	var words []glueWord
	var cur glueWord
	flush := func() {
		if len(cur.frags) > 0 {
			words = append(words, cur)
			cur = glueWord{}
		}
	}
	for _, run := range parseInline(s) {
		text := run.text
		for len(text) > 0 {
			if r := text[0]; r == ' ' || r == '\t' || r == '\n' {
				flush()
				text = strings.TrimLeft(text, " \t\n")
				continue
			}
			piece := text
			if idx := strings.IndexAny(text, " \t\n"); idx >= 0 {
				piece, text = text[:idx], text[idx:]
			} else {
				text = ""
			}
			cur.frags = append(cur.frags, inlineRun{text: piece, bold: run.bold, code: run.code, link: run.link})
			cur.width += lipgloss.Width(piece)
		}
	}
	flush()
	return words
}

// fragStyle picks a glueWord fragment's style: code and links always render
// the same regardless of review state — they are markup, not prose, the same
// reasoning RENDER-04 applied to a block quote's rule — while bold builds on
// top of body so reviewed prose still dims even where it is emphasised.
func fragStyle(f inlineRun, body lipgloss.Style) lipgloss.Style {
	switch {
	case f.code:
		return codeStyle
	case f.link:
		return linkStyle
	case f.bold:
		return body.Bold(true)
	default:
		return body
	}
}

// wrapInline is wrap's inline-markup-aware sibling: it wraps a paragraph's
// raw markdown text to w the same way wrap does, but returns lines that are
// already ANSI-styled per RENDER-06 — **bold**, `code`, [links](url) — rather
// than plain text for the caller to style uniformly. body is the base style
// (plain or dimmed-for-reviewed) plain runs and bold both build on.
func wrapInline(s string, w int, body lipgloss.Style) []string {
	words := glueWords(s)
	var out []string
	var line []glueWord
	n := 0
	flushLine := func() {
		if len(line) == 0 {
			return
		}
		var sb strings.Builder
		for i, wd := range line {
			if i > 0 {
				sb.WriteByte(' ')
			}
			for _, f := range wd.frags {
				sb.WriteString(fragStyle(f, body).Render(f.text))
			}
		}
		out = append(out, sb.String())
		line, n = nil, 0
	}
	for _, wd := range words {
		if w > 0 && n > 0 && n+1+wd.width > w {
			flushLine()
		}
		line = append(line, wd)
		n += wd.width + 1
	}
	flushLine()
	if len(out) == 0 {
		return []string{""}
	}
	return out
}

// wrapList lays out a blockList's items one at a time: each item's own prose
// wraps to the measure the way wrapInline wraps a paragraph, minus its
// prefix's width, and every line after the first is indented to that same
// width so continuation text lines up under the item's own text rather than
// under its marker. It shares wrapInline rather than wrap so an item's own
// **bold**, `code` and [links](url) render instead of showing their raw
// markup (2026-08-08 markdown-rendering feedback) — the prefix and hang
// themselves are plain, unstyled indentation, never part of the source text.
func wrapList(items []listItem, w int, body lipgloss.Style) []string {
	var out []string
	for _, item := range items {
		hang := strings.Repeat(" ", len(item.prefix))
		for j, l := range wrapInline(item.text, w-len(item.prefix), body) {
			if j == 0 {
				out = append(out, item.prefix+l)
			} else {
				out = append(out, hang+l)
			}
		}
	}
	if len(out) == 0 {
		return []string{""}
	}
	return out
}

// wrapQuote lays out a blockQuote's stripped lines (see quoteLinesFor) as
// prose: consecutive non-blank lines are folded into one paragraph and
// re-wrapped to w via wrapInline — the way collapse()+wrapInline() treats a
// regular paragraph, so a quote's own **bold**, `code` and [links](url)
// render instead of showing their raw markup (2026-08-08
// markdown-rendering feedback) — while a blank line — the marker
// quoteLinesFor leaves behind for a `>` alone in the source — becomes a
// paragraph break in the output. w is already the measure minus the rule's
// own width; the caller (render) prepends the rule to every line this
// returns, blank ones included, so the rule reads as one continuous line
// down the quote rather than breaking for its own paragraph gaps.
func wrapQuote(lines []string, w int, body lipgloss.Style) []string {
	var out []string
	var para []string
	flush := func() {
		if len(para) == 0 {
			return
		}
		out = append(out, wrapInline(strings.Join(para, " "), w, body)...)
		para = nil
	}
	for _, l := range lines {
		if strings.TrimSpace(l) == "" {
			flush()
			out = append(out, "")
			continue
		}
		para = append(para, l)
	}
	flush()
	// A trailing blank line inside the quote's source (rare, but a quote can
	// end on a bare `>`) would otherwise show as a rule with nothing after
	// it.
	for len(out) > 0 && out[len(out)-1] == "" {
		out = out[:len(out)-1]
	}
	if len(out) == 0 {
		return []string{""}
	}
	return out
}

// codeStyleName is the chroma style highlightCode renders with — a dark,
// muted palette chosen to sit near the rest of margin's own colours (see
// RENDER-06's codeStyle/linkStyle) rather than a bright, high-contrast theme
// meant for a dedicated editor pane.
const codeStyleName = "monokai"

// highlightCode runs a fenced code block's content through chroma, returning
// one already-ANSI-styled line per input line — render()'s preStyled path,
// the same one wrapInline uses for RENDER-06's inline markup. lang is the
// fence's info string; chroma falls back to a lexer it guesses from the
// content, and finally to an unhighlighted passthrough, so an unrecognised
// or absent language degrades to plain text rather than an error on screen.
func highlightCode(lines []string, lang string) []string {
	if len(lines) == 0 {
		return []string{""}
	}
	src := strings.Join(lines, "\n")
	var buf bytes.Buffer
	if err := quick.Highlight(&buf, src, lang, "terminal256", codeStyleName); err != nil {
		return lines
	}
	return strings.Split(strings.TrimRight(buf.String(), "\n"), "\n")
}

// focusedKind returns the blockKind of the currently focused entry,
// or blockRaw if index is invalid.
func (m *model) focusedKind() blockKind {
	if m.at.entry < 0 || m.at.entry >= len(m.entries) {
		return blockRaw
	}
	return m.entries[m.at.entry].b.kind
}

// scrollBlock shifts the horizontal scroll offset of the focused block by
// delta, for the two kinds that do not wrap to the measure: code blocks and
// the frontmatter block. l scrolls right, h scrolls left, 4 columns per press
// (the same step size the code block horizontal scroll feedback settled for
// blockCode and the frontmatter feedback asks to extend to blockFrontmatter).
func (m *model) scrollBlock(delta int) {
	if m.at.entry < 0 || m.at.entry >= len(m.entries) {
		return
	}
	e := m.entries[m.at.entry]
	var lines []string
	switch e.b.kind {
	case blockCode:
		lines = highlightCode(e.b.lines, e.b.lang)
	case blockFrontmatter:
		lines = frontmatterFields(e.b.text)
	default:
		return
	}
	anchor := e.b.anchor
	if anchor == "" {
		return
	}

	maxLen := 0
	for _, l := range lines {
		if w := visualWidth(l); w > maxLen {
			maxLen = w
		}
	}

	w := m.contentWidth()
	maxScroll := maxLen - w
	if maxScroll < 0 {
		maxScroll = 0
	}

	if m.codeScroll == nil {
		m.codeScroll = make(map[string]int)
	}

	curr := m.codeScroll[anchor]
	next := curr + delta
	if next < 0 {
		next = 0
	}
	if next > maxScroll {
		next = maxScroll
	}

	if curr == next && delta > 0 && maxScroll == 0 {
		m.status = "block fits within measure"
		return
	}

	m.codeScroll[anchor] = next
	if next == 0 {
		m.status = "block scrolled to left edge"
	} else {
		m.status = fmt.Sprintf("block scrolled to col %d", next)
	}
}

func hasOverflow(lines []string, w int) bool {
	for _, l := range lines {
		if visualWidth(l) > w {
			return true
		}
	}
	return false
}

var ansiRe = regexp.MustCompile(`\x1b\[[0-9;]*[a-zA-Z]`)

// visualWidth returns the visual column width of an ANSI-formatted string.
func visualWidth(s string) int {
	return len([]rune(ansiRe.ReplaceAllString(s, "")))
}

// scrollCodeLine takes an ANSI-styled line of text (e.g. output from chroma),
// skips offset visual columns, and returns at most width visual columns of text,
// preserving ANSI graphics styling (color/bold) from before offset.
func scrollCodeLine(line string, offset int, width int) string {
	if offset <= 0 && width <= 0 {
		return line
	}
	plainLen := visualWidth(line)
	if offset <= 0 && plainLen <= width {
		return line
	}
	if offset >= plainLen {
		return ""
	}

	type chunk struct {
		isANSI bool
		text   string
	}
	var chunks []chunk
	i := 0
	for i < len(line) {
		if line[i] == '\x1b' {
			j := i + 1
			if j < len(line) && line[j] == '[' {
				j++
				for j < len(line) && (line[j] < 'a' || line[j] > 'z') && (line[j] < 'A' || line[j] > 'Z') {
					j++
				}
				if j < len(line) {
					j++
				}
			}
			chunks = append(chunks, chunk{isANSI: true, text: line[i:j]})
			i = j
		} else {
			r, size := utf8.DecodeRuneInString(line[i:])
			chunks = append(chunks, chunk{isANSI: false, text: string(r)})
			i += size
		}
	}

	var activeANSI strings.Builder
	var result strings.Builder
	visCol := 0
	hasOutput := false
	sawANSIInOutput := false

	for _, c := range chunks {
		if c.isANSI {
			if visCol < offset {
				activeANSI.WriteString(c.text)
			} else if visCol < offset+width {
				if !hasOutput {
					result.WriteString(activeANSI.String())
					hasOutput = true
				}
				result.WriteString(c.text)
				sawANSIInOutput = true
			}
		} else {
			if visCol >= offset && visCol < offset+width {
				if !hasOutput {
					result.WriteString(activeANSI.String())
					hasOutput = true
				}
				result.WriteString(c.text)
			}
			visCol++
		}
	}

	if hasOutput && (activeANSI.Len() > 0 || sawANSIInOutput) {
		result.WriteString("\x1b[0m")
	}

	return result.String()
}

// renderFrontmatterFields draws a document's leading frontmatter
// (blockFrontmatter) as dimmed key/value lines, one per field. RENDER-07 chose
// the dimmed block over hidden-on-demand and a collapsed line that expands
// because it needs no new key and no new focus state to see at all; the
// 2026-08-09 frontmatter feedback then asked for the two changes this function
// is the output of. A field line wider than the measure is scrolled
// horizontally with h/l (the code-block treatment) rather than wrapped or
// truncated with an ellipsis — frontmatter is short key: value pairs, not
// prose, so a wrapped continuation line would read as a second field, and a
// truncated one hides the value it was there to show. off is the horizontal
// scroll offset in visual columns, clamped to the field width minus the
// measure; scrollCodeLine does the clipping and keeps the dim styling on the
// visible run.
func renderFrontmatterFields(b block, w, off int) []string {
	fields := frontmatterFields(b.text)
	if len(fields) == 0 {
		return nil
	}
	lines := make([]string, 0, len(fields))
	for _, f := range fields {
		lines = append(lines, dimStyle.Render(f))
	}
	if off > 0 || hasOverflow(lines, w) {
		scrolled := make([]string, len(lines))
		for idx, l := range lines {
			scrolled[idx] = scrollCodeLine(l, off, w)
		}
		lines = scrolled
	}
	return lines
}

// tableColumnSpacing is the gap between two adjacent columns. No vertical
// bar: RENDER-04 already chose a rule over a literal `>` for block quotes on
// the grounds that markdown's own delimiter is source syntax, not a
// rendering, and a `|` on every row is the same call for a table. Two spaces
// reads as a column break without spending a whole glyph column on it.
const tableColumnSpacing = "  "

// renderTable lays out a blockTable as aligned columns: a bold header row, a
// dim rule under it, then one row per line, colours all resolved against
// body so a reviewed table dims exactly like the paragraph and quote text
// beside it. Column widths come from the content itself (tableColumnWidths)
// so a narrow table (most of them) is not stretched to the full measure.
func renderTable(tb *tableBlock, w int, body lipgloss.Style) []string {
	if tb == nil {
		return nil
	}
	cols := tableColumns(tb)
	if cols == 0 {
		return nil
	}
	widths := tableColumnWidths(tableNaturalWidths(tb, cols), w)

	lines := make([]string, 0, 2+len(tb.rows))
	lines = append(lines, tableRowLine(tb.header, widths, tb.aligns, body.Bold(true)))
	lines = append(lines, dimStyle.Render(tableRuleLine(widths)))
	for _, row := range tb.rows {
		lines = append(lines, tableRowLine(row, widths, tb.aligns, body))
	}
	return lines
}

// tableColumns is the widest of the header, the alignment row, and every body
// row — a source table can be ragged (a short row, or no explicit alignment),
// and every reader (cellAt, tableAlignAt) treats a short slice as trailing
// empty/unaligned cells rather than a mismatch to fail on.
func tableColumns(tb *tableBlock) int {
	n := len(tb.header)
	if len(tb.aligns) > n {
		n = len(tb.aligns)
	}
	for _, row := range tb.rows {
		if len(row) > n {
			n = len(row)
		}
	}
	return n
}

// tableNaturalWidths is each column's content width before any narrowing —
// the widest cell (header included) in that column, in runes rather than
// bytes so a multi-byte cell does not read as artificially wide.
func tableNaturalWidths(tb *tableBlock, cols int) []int {
	widths := make([]int, cols)
	for i := range widths {
		widths[i] = len([]rune(cellAt(tb.header, i)))
	}
	for _, row := range tb.rows {
		for i := range widths {
			if l := len([]rune(cellAt(row, i))); l > widths[i] {
				widths[i] = l
			}
		}
	}
	return widths
}

// tableColumnWidths narrows natural column widths to fit w, when they do not
// already. It repeatedly shaves one rune off the currently-widest column
// above a 3-rune floor (enough room for truncate's own ellipsis) until the
// row fits or every column has hit the floor — an even, proportional
// narrowing rather than truncating one column down to nothing while its
// neighbours stay full width. A row so packed with columns that even the
// floor does not fit is left over-width rather than unreadable; that is a
// terminal too narrow for this table, not a bug in the algorithm.
func tableColumnWidths(natural []int, w int) []int {
	const floor = 3
	widths := append([]int(nil), natural...)
	spacing := len(tableColumnSpacing) * (len(widths) - 1)
	total := func() int {
		sum := spacing
		for _, x := range widths {
			sum += x
		}
		return sum
	}
	for total() > w {
		widest := -1
		for i, x := range widths {
			if x > floor && (widest < 0 || x > widths[widest]) {
				widest = i
			}
		}
		if widest < 0 {
			break
		}
		widths[widest]--
	}
	return widths
}

// cellAt returns row's i'th cell, or "" past its end — the ragged-row
// tolerance tableColumns' doc comment describes.
func cellAt(row []string, i int) string {
	if i < 0 || i >= len(row) {
		return ""
	}
	return row[i]
}

// tableAlignAt returns column i's alignment, or alignNone (flush-left, no
// padding preference) past the end of aligns.
func tableAlignAt(aligns []tableAlign, i int) tableAlign {
	if i < 0 || i >= len(aligns) {
		return alignNone
	}
	return aligns[i]
}

// padCell truncates s to width (via truncate, the same rune-aware ellipsis
// thread summaries already use) and pads it out to width according to align.
func padCell(s string, width int, align tableAlign) string {
	if width > 0 {
		s = truncate(s, width)
	}
	pad := width - len([]rune(s))
	if pad < 0 {
		pad = 0
	}
	switch align {
	case alignRight:
		return strings.Repeat(" ", pad) + s
	case alignCenter:
		left := pad / 2
		return strings.Repeat(" ", left) + s + strings.Repeat(" ", pad-left)
	default:
		return s + strings.Repeat(" ", pad)
	}
}

// tableRowLine renders one row (header or body) as a single already-styled
// line: every cell padded to its column's width and alignment, then style
// applied per cell before joining — so the padding spaces themselves carry no
// colour, which matters for alignRight and alignCenter's leading spaces.
func tableRowLine(row []string, widths []int, aligns []tableAlign, style lipgloss.Style) string {
	cells := make([]string, len(widths))
	for i, width := range widths {
		cells[i] = style.Render(padCell(cellAt(row, i), width, tableAlignAt(aligns, i)))
	}
	return strings.Join(cells, tableColumnSpacing)
}

// tableRuleLine draws the dim rule between the header and the body, one dash
// run per column, the same widths the rows above and below it use, so the
// rule lines up under the header exactly rather than approximating it.
func tableRuleLine(widths []int) string {
	cells := make([]string, len(widths))
	for i, width := range widths {
		cells[i] = strings.Repeat("─", width)
	}
	return strings.Join(cells, tableColumnSpacing)
}

// reportLatency prints the keypress-to-frame round trip, so "feels sluggish"
// becomes a number that can be compared across changes.
func reportLatency(samples []time.Duration) {
	if len(samples) == 0 {
		return
	}
	sorted := append([]time.Duration(nil), samples...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i] < sorted[j] })
	at := func(q float64) time.Duration {
		return sorted[int(q*float64(len(sorted)-1))].Round(100 * time.Microsecond)
	}
	fmt.Fprintf(os.Stderr, "\nkey → frame latency over %d keystrokes: p50 %v · p90 %v · p99 %v · max %v\n",
		len(sorted), at(0.5), at(0.9), at(0.99), at(1))
}

// RunOptions controls how Run behaves, beyond which file it opens.
type RunOptions struct {
	// Stdout, when set, writes the review to stdout on quit instead of
	// requiring Y — the same content Y produces, via the same exportReview
	// call — so a review can be piped straight into an agent:
	//
	//	margin --stdout FILE.md | agent -p "address this review"
	//
	// This always runs the review interactively first — the whole point is
	// that the printed review reflects the session that just happened, marks
	// included. For a non-interactive dump of what is on disk (threads, not
	// session marks), use `margin export FILE.md`, which is Export.
	Stdout bool

	// IncludeResolved, when set, includes resolved threads in the export (Y
	// and --stdout alike) instead of the default of leaving them out —
	// EXPORT-05, per D11's export-safety rationale: excluding them by default
	// is safe because undoing an over-eager resolution costs one flip, not
	// lost feedback, and this flag is that undo for the one review that needs
	// the full history.
	IncludeResolved bool

	// Stdin, when set, reads the document from stdin instead of path and
	// reviews it ephemerally: no thread files are read or written (the store
	// stays nil, so saves are no-ops, and no watcher starts) and Stdout is
	// implied — the review is printed on quit, since the pipe is the whole
	// point. The export names the document "stdin".
	Stdin bool

	// WheelSpeed is how many lines one mouse wheel tick scrolls the document
	// viewport. 0 means the default of 3 lines per tick — the maintainer's
	// "mouse wheel speed should be tunable" ask (2026-08-09), satisfied with
	// a per-invocation --wheel-speed flag rather than a session command, since
	// the wheel's step size is a hardware/perception preference about how the
	// document moves, not an act on it.
	WheelSpeed int
}

// openTTY opens the controlling terminal for the TUI when a pipe has taken
// over one of the standard streams: output, when stdout is carrying the
// review, and input as well in --stdin mode, where stdin is carrying the
// document. A var so a test can substitute one that does not need a real
// terminal.
var openTTY = func() (io.ReadWriteCloser, error) {
	return os.OpenFile("/dev/tty", os.O_RDWR, 0)
}

// stdinReader is where Run reads the document in --stdin mode. A var so a
// test can substitute one without standing on the process's real stdin.
var stdinReader io.Reader = os.Stdin

// Run opens path for review and reports what the session produced.
func Run(path string, opts RunOptions) error {
	if opts.Stdin {
		// --stdin implies --stdout: nothing is persisted, so the printed
		// review on quit is the only way the session's work leaves it.
		opts.Stdout = true
	}

	var doc []block
	var err error
	var src []byte
	if opts.Stdin {
		// A terminal on stdin means the reviewer typed `margin -` with no
		// pipe — reading it would swallow keystrokes until EOF, then open a
		// review of whatever they happened to type. Say so instead.
		if f, ok := stdinReader.(*os.File); ok {
			if info, serr := f.Stat(); serr == nil && info.Mode()&os.ModeCharDevice != 0 {
				return fmt.Errorf("run: --stdin expects markdown piped in; pass a file or pipe one (e.g. agent -p ... | margin -)")
			}
		}
		doc, src, err = loadDocFrom(stdinReader, "stdin")
		path = "stdin"
	} else {
		doc, src, err = loadDoc(path)
	}
	if err != nil {
		return err
	}

	// root is the review root and docPath is the document's path relative to
	// it (D9) — today, since M1 has no tree yet, that is just the file's own
	// directory and base name. reattach (inside newModelAt) is what matches
	// these loaded threads back to the parsed blocks. An ephemeral stdin
	// review skips all of it: there is nothing on disk to read threads from
	// or write them back to.
	var threads map[string]*thread
	var root, docPath string
	if !opts.Stdin {
		root, docPath = resolveReviewRoot(path)
		threads, err = loadThreadsForDoc(root, docPath)
		if err != nil {
			return fmt.Errorf("run: %w", err)
		}
	}

	m := newModelAt(path, doc, threads)
	m.src = src
	m.includeResolved = opts.IncludeResolved
	m.ephemeral = opts.Stdin
	if opts.WheelSpeed > 0 {
		m.wheelSpeed = opts.WheelSpeed
	}
	if !opts.Stdin {
		m.store = &threadStore{root: root, docPath: docPath}
		// Live reload is a nice-to-have on top of a working review, not a
		// precondition for one — a read-only filesystem or an exhausted inotify
		// watch budget should not stop the reviewer from opening the document,
		// so a failure here is silently not fatal. watcher stays nil and
		// m.watcher.wait() (nil-safe) simply never fires.
		if watcher, err := newThreadWatcher(root, docPath); err == nil {
			m.watcher = watcher
			defer watcher.close()
		}
	}
	// The renderer's own frame quantum is the remaining floor on input latency;
	// the 60fps default means up to 16.7ms before a damaged frame reaches the wire.
	progOpts := []tea.ProgramOption{tea.WithFPS(120)}
	if opts.Stdout {
		// Bubble Tea defaults its output to stdout, which here is carrying the
		// review — the pipe would fill with escape sequences and the interface
		// would have nowhere usable to draw. Point it at the controlling
		// terminal instead and leave stdout untouched until the review itself
		// is printed below. In --stdin mode the keys have to come from the
		// terminal too, since stdin is the document; one /dev/tty handle,
		// opened read/write, serves both.
		tty, err := openTTY()
		if err != nil {
			if opts.Stdin {
				return fmt.Errorf("run: --stdin needs a controlling terminal for the interface (stdin is the document): %w", err)
			}
			return fmt.Errorf("run: --stdout needs a controlling terminal to draw the interface on: %w", err)
		}
		defer tty.Close()
		progOpts = append(progOpts, tea.WithOutput(tty))
		if opts.Stdin {
			progOpts = append(progOpts, tea.WithInput(tty))
		}
	}
	if _, err := tea.NewProgram(m, progOpts...).Run(); err != nil {
		return fmt.Errorf("run: %w", err)
	}
	reportLatency(m.samples)

	if opts.Stdout {
		fmt.Print(exportReview(m.path, m.doc, m.threads, m.marks, m.includeResolved, m.ephemeral))
		return nil
	}

	for _, b := range m.doc {
		if !b.commentable() || b.anchor == "" {
			continue
		}
		if mk := m.marks[b.anchor]; mk != markNone {
			fmt.Printf("%s  [%s]\n", b.anchor, mk.glyph())
		}
		t := m.threads[b.anchor]
		if t == nil {
			continue
		}
		for _, c := range t.posted {
			fmt.Printf("%s  %s: %s\n", t.anchor, c.author, firstLine(c.body))
		}
		if d := t.draft(newCommentSlot); d != "" {
			fmt.Printf("%s  draft: %s\n", t.anchor, firstLine(d))
		}
	}
	return nil
}

func (m *model) renderPalette(w int) string {
	rows := paletteRows(m, commands, m.paletteQuery)
	
	if m.paletteSelected >= len(rows) {
		m.paletteSelected = max(0, len(rows)-1)
	}
	if m.paletteSelected < 0 {
		m.paletteSelected = 0
	}

	var b strings.Builder
	b.WriteString(dimStyle.Render(strings.Repeat("─", w)) + "\n")
	
	showCount := 7
	start := 0
	if m.paletteSelected >= showCount {
		start = m.paletteSelected - showCount + 1
	}
	end := start + showCount
	if end > len(rows) {
		end = len(rows)
	}

	for i := start; i < end; i++ {
		row := rows[i]
		cursor := "  "
		style := dimStyle
		if i == m.paletteSelected {
			cursor = focusStyle.Render("▌ ")
			style = textStyle
		}
		b.WriteString(cursor + style.Render(row.Command.ID) + dimStyle.Render(" — "+row.Title) + "\n")
	}
	
	b.WriteString(focusStyle.Render(":") + m.paletteQuery + focusStyle.Render("█"))
	
	return lipgloss.NewStyle().Width(w).Render(b.String())
}
