// The threads view: `i` swaps the document column for a list of the threads
// under review — every thread across the tree's documents in a tree review
// (D10), newest activity first, and the current document's own threads in a
// single-document review — and `enter` on a row jumps to the block it is
// anchored to. It is the threads-view half of the 2026-08-12 threads-view
// feedback: a way to discover threads that are easy to lose in a long
// document, including the ones that can no longer be attached — a thread
// whose block vanished from its document is listed and reported, not dropped.
//
// The view is a reading surface, not a new persistence: it reads the same
// thread files (or, in a single-document review, the already-loaded
// m.threads) everything else reads and writes nothing. Threads whose document
// is no longer in the tree still appear, reported rather than silently
// dropped — the tree-level form of the orphaned-thread rule a single
// document already follows.
package review

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"neuroplast.io/hotty-surf/surflive"
	"neuroplast.io/hotty-surf/surftea"
)

// inboxItem is one row of the inbox: a thread, where it lives, and whether
// that document is still one of the tree's markdown files. inTree is false
// for a thread whose document was deleted — it stays in the list, but enter
// reports rather than pretending to open it.
type inboxItem struct {
	doc    string
	anchor string
	thread *thread
	inTree bool
}

// last is the thread's newest comment timestamp — the inbox's sort key and,
// eventually, its age indicator. A thread with no comments has no timestamp
// and sorts to the bottom.
func (it inboxItem) last() time.Time {
	var newest time.Time
	for _, c := range it.thread.posted {
		if c.at.After(newest) {
			newest = c.at
		}
	}
	return newest
}

// loadInbox walks every thread file beneath the review root and returns one
// item per thread, newest activity first. Thread files are keyed by document
// path (D9), so the walk of .margin/threads/ mirrors the tree of documents
// without parsing any of them — the thread files are the data. A malformed
// thread file fails the whole load, the same "surface, don't drop" rule
// loadThreadsForDoc already applies. A threads directory that does not exist
// is not an error: it just means nothing has been said about anything.
func loadInbox(root string, tree []treeEntry) ([]inboxItem, error) {
	inTree := map[string]bool{}
	for _, e := range tree {
		if !e.isDir {
			inTree[e.rel] = true
		}
	}
	items := []inboxItem{}
	err := filepath.WalkDir(threadsDir(root), func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				return nil
			}
			return err
		}
		if d.IsDir() || !strings.HasSuffix(d.Name(), ".md") {
			return nil
		}
		doc, t, err := readThreadFile(p)
		if err != nil {
			return err
		}
		items = append(items, inboxItem{doc: doc, anchor: t.anchor, thread: t, inTree: inTree[doc]})
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.SliceStable(items, func(i, j int) bool {
		return items[i].last().After(items[j].last())
	})
	return items, nil
}

// docThreadItems builds the threads-view rows for a single-document review:
// one row per thread on the current document, newest activity first, straight
// from the already-loaded m.threads (which reattach has already told apart
// into attached and orphaned). A tree review uses loadInbox instead — it has
// more than one document to aggregate, so it reads the thread files from
// disk. A single-document review has exactly one document, and m.threads *is*
// that document's threads, so re-reading the same files would be redundant
// work that misses nothing.
func (m *model) docThreadItems() []inboxItem {
	items := make([]inboxItem, 0, len(m.threads))
	for anchor, t := range m.threads {
		items = append(items, inboxItem{doc: m.store.docPath, anchor: anchor, thread: t, inTree: true})
	}
	sort.SliceStable(items, func(i, j int) bool {
		return items[i].last().After(items[j].last())
	})
	return items
}

// toggleInbox opens or closes the threads view. Opening rebuilds the item
// list — from disk in a tree review (so a comment posted in another document
// since the last visit shows up), from the already-loaded m.threads in a
// single-document review — and clears the session's other surfaces — raw,
// search and the palette are different views and the threads view takes
// their column.
func (m *model) toggleInbox() {
	if m.inbox {
		m.inbox = false
		m.status = ""
		return
	}
	if m.store == nil {
		return
	}
	var items []inboxItem
	if m.tree != nil {
		var err error
		items, err = loadInbox(m.store.root, m.tree)
		if err != nil {
			m.status = "inbox: " + err.Error()
			return
		}
	} else {
		items = m.docThreadItems()
	}
	m.inboxItems = items
	m.inboxAt = 0
	m.dropInbox() // a fresh table, scrolled to the top
	m.inbox = true
	m.raw = false
	m.visual = false
	m.searchOpen = false
	m.paletteOpen = false
	m.pendingKey = ""
	m.count = ""
	m.status = ""
}

// handleInboxKey routes keys while the inbox holds the document column. It is
// a modal surface like the tree pane: j/k/g/G move through the threads,
// enter/l opens the focused thread's document, esc/h/tab return to the
// document, q quits. Everything else is inert.
func (m *model) handleInboxKey(msg tea.KeyPressMsg) tea.Cmd {
	switch msg.String() {
	case "j", "down", "k", "up", "g", "G":
		// The table's own: its binds move the selection (threads.surf).
		h, name := m.threadsHost(m.h-footerRows), fieldKeyName(msg.Key())
		if m.hot.native() && !m.hot.deaf {
			h.Key(m.hot.s, name) // a key the host left to margin
		} else {
			h.CellsKey(name)
		}
		m.inboxActs()
	case "enter", "l", "right":
		// Enter on a host reaches margin as a key the table does not act
		// on, and the selection is margin's: it opens the focused row.
		m.openInboxItem(m.inboxAt)
	case "esc", "h", "left", "tab":
		m.inbox = false
		m.status = ""
	case "q", "ctrl+c":
		m.quitting = true
		return tea.Quit
	}
	return nil
}

// stepComment moves focus to the next (d=1) or previous (d=-1) thread in the
// document — the "next/previous comment" hop of the threads-view feedback. A
// thread is one focus stop however long it renders (journal 2026-08-09.18),
// so the walk is over m.entries' thread rows, skipping the one focus
// currently sits on. The landing is a jump: recorded in the jumplist (so
// ctrl+o returns to where the hop started) and centred like a search match.
// At the document's ends it reports rather than wraps — a hop past the last
// comment should say so, not silently restart at the first. A raw view has no
// thread rows (rebuild excludes them), so the hop reports "none".
func (m *model) stepComment(d int) {
	if m.raw {
		m.status = "no comments in raw view"
		return
	}
	start := m.at.entry
	i := start + d
	for i >= 0 && i < len(m.entries) {
		if m.entries[i].thread != nil {
			m.pushJump(cursor{entry: i, comment: commentNone})
			m.jumpToEntry(i)
			m.status = fmt.Sprintf("comment %d/%d · %s", commentOrdinal(i, m.entries), len(threadRows(m.entries)), m.entries[i].thread.summary(false))
			return
		}
		i += d
	}
	if d > 0 {
		m.status = "no more comments below"
	} else {
		m.status = "no more comments above"
	}
}

// threadRows returns the entry indices that carry a thread row.
func threadRows(entries []entry) []int {
	var out []int
	for i, e := range entries {
		if e.thread != nil {
			out = append(out, i)
		}
	}
	return out
}

// commentOrdinal reports which thread row (1-based) entry i is, in document
// order — the "3 of 7" half of the hop's status line.
func commentOrdinal(i int, entries []entry) int {
	n := 0
	for j := 0; j <= i; j++ {
		if entries[j].thread != nil {
			n++
		}
	}
	return n
}

// openInboxItem jumps to the thread at row i: in a tree review it opens the
// thread's document exactly as a pane open would (openTreeFile — the same
// re-pointing, the same session reset), then lands focus on the block the
// thread is anchored to, centred, with the landing recorded in the jumplist
// so ctrl+o walks back. In a single-document review the document is already
// open, so the landing is all there is. A thread whose document is no longer
// in the tree, or whose block has vanished from that document, is reported
// rather than silently dropped.
func (m *model) openInboxItem(i int) {
	if i < 0 || i >= len(m.inboxItems) {
		return
	}
	item := m.inboxItems[i]
	m.inbox = false
	if !item.inTree {
		m.status = fmt.Sprintf("thread %s's document %s is not in this tree", item.anchor, item.doc)
		return
	}
	// A single-document review has no tree to switch within — the document is
	// already the one under review, so there is nothing to open or re-point.
	if m.tree != nil {
		row := -1
		for j, e := range m.tree {
			if !e.isDir && e.rel == item.doc {
				row = j
				break
			}
		}
		if row < 0 {
			m.status = fmt.Sprintf("thread %s's document %s is not in this tree", item.anchor, item.doc)
			return
		}
		// A thread on the document already under review jumps without switching.
		if item.doc != m.store.docPath {
			m.openTreeFile(row)
		}
	}
	// Land on the thread's block, centred like a search or link jump. A
	// thread whose block vanished from its document is surfaced, not dropped:
	// the document is open, focus sits at the top, and the status says what
	// is missing.
	for j, e := range m.entries {
		if e.b.anchor == item.anchor {
			m.pushJump(cursor{entry: j, comment: commentNone})
			m.jumpToEntry(j)
			m.status = fmt.Sprintf("threads: %s · %s", item.doc, item.thread.summary(false))
			return
		}
	}
	m.status = fmt.Sprintf("threads: %s — the thread's block is gone", item.doc)
}

// inboxID is the threads view's surface name: a table of the threads under
// review, one row each (threads.surf).
const inboxID = "threads"

// threadsHost is the threads view as the model has it, rows tall, its state
// brought up to date: the threads, and the one focused. margin keeps the
// focus and the keys; a click on a row focuses it, and Enter or a click on
// the focused one opens it (inboxActs).
func (m *model) threadsHost(rows int) *surftea.Host {
	if m.inboxHost == nil {
		st := &Threads{}
		st.click = func(i int) { m.inboxAt = i }
		st.open = func(i int) { m.inboxAt, m.inboxOpen = i, true }
		m.inboxHost = newSurfHost("margin-"+inboxID, surflive.New(surfInterpThreads(st)), nil)
		m.inboxState = st
	}
	st := m.inboxState
	st.Tree = m.tree != nil
	st.Height = max(rows, 1)
	st.Sel = m.inboxAt
	st.Rows = st.Rows[:0]
	for i, item := range m.inboxItems {
		r := ThreadRow{K: strconv.Itoa(i), Doc: item.doc, Summary: item.thread.summary(false), Resolved: item.thread.resolved}
		if item.thread.resolved {
			r.State = "✓"
		}
		if !item.inTree {
			r.Gone = "doc not in tree"
		} else if item.thread.orphaned {
			r.Gone = "block gone"
		}
		st.Rows = append(st.Rows, r)
	}
	return m.inboxHost
}

// dropInbox forgets the threads view's surface, and the host's: the next
// one is a fresh table, scrolled to the top.
func (m *model) dropInbox() {
	if m.inboxHost != nil && m.hot != nil {
		m.hot.s.Delete(m.inboxHost.Name())
	}
	m.inboxHost, m.inboxState, m.inboxOpen = nil, nil, false
}

// inboxActs opens the row the surface asked to open (Enter on the focused
// row, or a click on it).
func (m *model) inboxActs() {
	if m.inbox && m.inboxOpen {
		m.inboxOpen = false
		m.openInboxItem(m.inboxAt)
	}
}

// renderInbox returns the threads view's rows for one viewport, in the
// document's column: its own scroll, like the pane's — the document's is
// left alone, so closing the view returns the reader to exactly where they
// were.
func (m *model) renderInbox(viewport int) []string {
	lines := m.threadsHost(viewport).Lines(m.w-m.docX(), viewport)
	rows := make([]string, viewport)
	copy(rows, lines)
	return rows
}

// Threads is the threads view's state (threads.surf lays it out): a row a
// thread, the one selected, and how tall the table is. margin keeps the
// selection and the keys; the surface shows it, and a click or Enter on a
// row asks margin to select or open it.
type Threads struct {
	Rows   []ThreadRow
	Sel    int
	Tree   bool
	Height int

	click func(int)
	open  func(int)
}

// ThreadRow is a thread in the table: its state mark, its document, what it
// says, and what is gone.
type ThreadRow struct {
	K        string
	State    string
	Doc      string
	Summary  string
	Gone     string
	Resolved bool
}

// Move, First and Last are the keys that move the selection.
func (t *Threads) Move(d int) { t.sel(t.Sel + d) }
func (t *Threads) First()     { t.sel(0) }
func (t *Threads) Last()      { t.sel(len(t.Rows) - 1) }

func (t *Threads) sel(i int) {
	if len(t.Rows) == 0 {
		return
	}
	t.Sel = max(min(i, len(t.Rows)-1), 0)
	t.Pick(t.Sel)
}

// Pick is a click on the row i.
func (t *Threads) Pick(i int) {
	if t.click != nil {
		t.click(i)
	}
}

// Open is Enter on the selected row, or a click on it.
func (t *Threads) Open(i int) {
	if t.open != nil {
		t.open(i)
	}
}
