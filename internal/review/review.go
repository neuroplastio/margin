// Package review implements margin's document review model: a markdown
// document whose blocks carry anchored comment threads, where focusing a thread
// splices a real nvim in as the composer and blurring it collapses back to a
// one-line draft.
//
// The load-bearing bet is that the editor is disposable. A composer is a live
// child process; losing focus kills it, and the text survives as a draft keyed
// by anchor and target. Regaining focus respawns nvim with the draft restored.
// Blur and "esc esc" are therefore the same code path, so there is only ever one
// persistence story.
package review

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

const (
	paneRows   = 10 // composer height; nvim gets cramped below ~8
	minPaneCol = 40
	gutterW    = 3 // focus bar + review glyph + space
	borderW    = 1
	maxMeasure = 76 // the comfortable reading measure
	footerRows = 2
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
// comments.
const commentNone = -1

type cursor struct {
	entry   int
	comment int
}

type model struct {
	path    string
	doc     []block
	threads map[string]*thread
	marks   map[string]reviewMark
	entries []entry

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

	at   cursor
	comp *composer
	gen  int

	// pendingOpen is queued when a click lands on another thread while an
	// editor is open. Blur is asynchronous — nvim has to write its buffer and
	// exit — so the new focus has to wait its turn.
	pendingOpen *cursor

	w, h     int
	scroll   int
	status   string
	quitting bool

	spans    []span
	subspans map[cursor]span // comment-level spans of the expanded thread
	paneTop  int             // line index of the first emulator row; -1 when idle
	paneW    int

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
		marks:   map[string]reviewMark{},
		at:      cursor{entry: 0, comment: commentNone},
		paneTop: -1,
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
		if b.kind == blockFrontmatter {
			// Not rendered at all: it is metadata about the document, not part
			// of it, and how it should look on screen is an open felt question
			// (see the frontmatter-rendering feedback) that nothing here
			// answers by picking a treatment.
			continue
		}
		m.entries = append(m.entries, entry{b: b})
		if t, ok := m.threads[b.anchor]; ok && b.anchor != "" {
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
// one per posted comment of a thread. Building the list explicitly keeps the
// movement rules in one place instead of scattered across j and k.
func (m *model) stops() []cursor {
	var cs []cursor
	for i, e := range m.entries {
		cs = append(cs, cursor{entry: i, comment: commentNone})
		if e.thread != nil {
			for j := range e.thread.posted {
				cs = append(cs, cursor{entry: i, comment: j})
			}
		}
	}
	return cs
}

func (m *model) moveFocus(d int) {
	cs := m.stops()
	for i, c := range cs {
		if c == m.at {
			if j := i + d; j >= 0 && j < len(cs) {
				m.at = cs[j]
			}
			return
		}
	}
	if len(cs) > 0 {
		m.at = cs[0]
	}
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

// exportToClipboard renders the review and puts it on the clipboard.
func (m *model) exportToClipboard() tea.Cmd {
	out := exportReview(m.path, m.doc, m.threads, m.marks)

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
		t.setDraft(target, body)
		_ = saveDraft(t.anchor, target, body)
		if body == "" {
			m.status = "closed"
		} else if target == newCommentSlot {
			m.status = "draft kept on " + t.anchor
		} else {
			m.status = "edit kept unsaved"
		}
	default:
		t.setDraft(target, "")
		_ = clearDraft(t.anchor, target)
		m.status = "discarded"
	}

	m.comp.close()
	m.comp = nil
	m.rebuild()

	if next := m.pendingOpen; next != nil {
		m.pendingOpen = nil
		m.at = *next
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

	case tea.KeyPressMsg:
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

	// Every key margin recognises resolves to a command id via keymap and runs
	// through that command's Run — see command.go. handleKey itself carries no
	// verb-specific behaviour, so a future palette invoking the same command
	// id is guaranteed to do exactly what the key does.
	id, ok := keymap[msg.String()]
	if !ok {
		return nil
	}
	cmd, ok := commandByID(id)
	if !ok {
		return nil
	}
	return cmd.Run(m)
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
	m.status = "select a comment with j/k to edit it, or c to write a new one"
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
	headStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("212")).Bold(true)
	authorStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("114"))
	draftStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("179"))
	focusStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("212"))
	okStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("114"))
	flagStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("209"))
	reviewedTxt = lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
	rawStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("109"))
)

// gutter draws the focus bar and the review glyph in a fixed-width column, so
// marks line up down the page and can be scanned without reading the prose.
func (m *model) gutter(focused bool, mark reviewMark, partial bool) string {
	bar := " "
	if focused {
		bar = focusStyle.Render("▌")
	}
	glyph := " "
	switch mark {
	case markOK:
		glyph = okStyle.Render(mark.glyph())
	case markFlag:
		glyph = flagStyle.Render(mark.glyph())
	}
	if partial && mark != markNone {
		glyph = dimStyle.Render("·")
	}
	return bar + glyph + " "
}

// render walks the block list once, producing the lines to draw plus the span
// each entry and comment occupies. Doing it in one pass is what keeps
// hit-testing, the cursor position and scrolling from drifting apart — deriving
// any of them separately is how the cursor ended up a row off earlier.
func (m *model) render() []string {
	w := m.contentWidth()
	m.spans = make([]span, len(m.entries))
	m.subspans = map[cursor]span{}
	var lines []string

	for i, e := range m.entries {
		start := len(lines)
		focused := m.at.entry == i

		switch {
		case e.b.kind == blockHeading:
			mark, partial := rollUp(m.marksFor(m.sectionAnchors(i)))
			lines = append(lines,
				m.gutter(focused && m.at.comment == commentNone, mark, partial)+
					headStyle.Render(e.b.text), "")

		case e.b.kind == blockPara, e.b.kind == blockRaw:
			mark := m.marks[e.b.anchor]
			body := textStyle
			if mark == markOK {
				// Reviewed prose recedes so unreviewed text is what draws the eye.
				body = reviewedTxt
			}
			// A blockRaw is shown verbatim: for a code fence or a list, the
			// indentation and the line breaks are the content, so re-wrapping
			// it to the measure would destroy it.
			out := e.b.lines
			if e.b.kind == blockPara {
				out = wrap(e.b.text, w)
			} else if mark != markOK {
				body = rawStyle
			}
			for _, l := range out {
				lines = append(lines,
					m.gutter(focused && m.at.comment == commentNone, mark, false)+body.Render(l))
			}
			if m.threads[e.b.anchor] == nil {
				lines = append(lines, "")
			}

		default:
			lines = append(lines, m.threadLines(i, e.thread, w, len(lines))...)
			lines = append(lines, "")
		}

		m.spans[i] = span{start: start, end: len(lines) - 1}
	}
	return lines
}

// threadLines renders a thread in whichever of the three states it is in:
// collapsed to a line, expanded for reading, or hosting the live editor. base is
// the absolute line the thread starts at, so comment spans come out in the same
// coordinates as everything else.
func (m *model) threadLines(i int, t *thread, w, base int) []string {
	focused := m.at.entry == i
	box := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("240")).
		Width(w - 2*borderW)
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
		return indent(strings.Split(box.Render(m.comp.em.Render()), "\n"))
	}

	// Collapsed: literally one line, no box. A bordered one-liner costs three
	// rows, and with a thread every few paragraphs that crowds out the document
	// — which is what the reader came for.
	if !focused {
		style := dimStyle
		if t.draft(newCommentSlot) != "" || t.pendingEdit() >= 0 {
			style = draftStyle
		}
		return []string{strings.Repeat(" ", gutterW) + dimStyle.Render("│ ") + style.Render(t.summary())}
	}

	// Expanded: the whole exchange, ready to read. Comments are separately
	// focusable so `e` knows which one to open.
	var body []string
	mark := func(j int) string {
		if m.at.comment == j {
			return focusStyle.Render("▌ ")
		}
		return "  "
	}
	for j, c := range t.posted {
		// +1 for the box's top border, which Render adds around this content.
		commentStart := base + borderW + len(body)
		head := mark(j) + authorStyle.Render(c.author) + dimStyle.Render(" · "+humanAge(c.at))
		if d := t.draft(j); d != "" {
			head += draftStyle.Render("  ✎ edited, unsaved")
		}
		body = append(body, head)
		shown := c.body
		if d := t.draft(j); d != "" {
			shown = d
		}
		for _, l := range wrap(shown, w-6) {
			body = append(body, "  "+l)
		}
		m.subspans[cursor{entry: i, comment: j}] = span{
			start: commentStart, end: base + borderW + len(body) - 1,
		}
		if j < len(t.posted)-1 {
			body = append(body, "")
		}
	}
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
		body = []string{"  " + dimStyle.Render("no comments yet — c to write one")}
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
	// Sample only on a frame the child's output actually triggered. Bubble Tea
	// also renders immediately after the keypress itself, and counting that
	// frame would report a latency of roughly zero.
	if m.sawDamage && !m.keyAt.IsZero() {
		m.samples = append(m.samples, time.Since(m.keyAt))
		m.keyAt, m.sawDamage = time.Time{}, false
	}

	lines := m.render()
	// The emulator's first row sits one line below the focused thread's box
	// border, which render() has just measured.
	m.paneTop = -1
	if m.comp != nil && m.at.entry < len(m.spans) {
		m.paneTop = m.spans[m.at.entry].start + borderW
		m.paneW = m.contentWidth() - 2*borderW
	}

	viewport := max(m.h-footerRows, 1)
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
			"  "+what+"  ·  ctrl+s save · esc esc keep · SPC c k discard · click away to blur"))
	} else {
		done, flagged, total := m.reviewProgress()
		progress := fmt.Sprintf("%d/%d reviewed", done, total)
		if flagged > 0 {
			progress += fmt.Sprintf(" · %s", flagStyle.Render(fmt.Sprintf("%d flagged", flagged)))
		}
		b.WriteString(dimStyle.Render("j/k move · c comment · e edit · space mark · Y copy review · q quit   ") +
			dimStyle.Render(progress))
		if m.status != "" {
			b.WriteString(dimStyle.Render("   " + m.status))
		}
	}

	v := tea.NewView(b.String())
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

// clampScroll keeps the focused position — and so all of the composer, when one
// is open — inside the viewport.
func (m *model) clampScroll(total, viewport int) int {
	if total <= viewport {
		return 0
	}
	f, ok := m.subspans[m.at]
	if !ok && m.at.entry < len(m.spans) {
		f = m.spans[m.at.entry]
		ok = true
	}
	s := m.scroll
	if ok {
		if f.start < s {
			s = f.start
		}
		if f.end >= s+viewport {
			s = f.end - viewport + 1
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
	// Threads are session-only until STORE-01's persistence is read back
	// without opening the interface, so this always runs the review
	// interactively first; there is no non-interactive "just print what's on
	// disk" mode yet.
	Stdout bool
}

// openTTY opens the controlling terminal for the TUI to draw on when stdout
// is carrying the review instead. A var so a test can substitute one that
// does not need a real terminal.
var openTTY = func() (io.WriteCloser, error) {
	return os.OpenFile("/dev/tty", os.O_RDWR, 0)
}

// Run opens path for review and reports what the session produced.
func Run(path string, opts RunOptions) error {
	doc, err := loadDoc(path)
	if err != nil {
		return err
	}

	// root is the review root and docPath is the document's path relative to
	// it (D9) — today, since M1 has no tree yet, that is just the file's own
	// directory and base name. reattach (inside newModelAt) is what matches
	// these loaded threads back to the parsed blocks.
	root, docPath := filepath.Dir(path), filepath.Base(path)
	threads, err := loadThreadsForDoc(root, docPath)
	if err != nil {
		return fmt.Errorf("run: %w", err)
	}

	m := newModelAt(path, doc, threads)
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
	// The renderer's own frame quantum is the remaining floor on input latency;
	// the 60fps default means up to 16.7ms before a damaged frame reaches the wire.
	progOpts := []tea.ProgramOption{tea.WithFPS(120)}
	if opts.Stdout {
		// Bubble Tea defaults its output to stdout, which here is carrying the
		// review — the pipe would fill with escape sequences and the interface
		// would have nowhere usable to draw. Point it at the controlling
		// terminal instead and leave stdout untouched until the review itself
		// is printed below.
		tty, err := openTTY()
		if err != nil {
			return fmt.Errorf("run: --stdout needs a controlling terminal to draw the interface on: %w", err)
		}
		defer tty.Close()
		progOpts = append(progOpts, tea.WithOutput(tty))
	}
	if _, err := tea.NewProgram(m, progOpts...).Run(); err != nil {
		return fmt.Errorf("run: %w", err)
	}
	reportLatency(m.samples)

	if opts.Stdout {
		fmt.Print(exportReview(m.path, m.doc, m.threads, m.marks))
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
