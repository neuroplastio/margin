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
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
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
	m.inboxScroll = 0
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
	case "j", "down":
		if m.inboxAt+1 < len(m.inboxItems) {
			m.inboxAt++
		}
	case "k", "up":
		if m.inboxAt > 0 {
			m.inboxAt--
		}
	case "g":
		m.inboxAt = 0
	case "G":
		if len(m.inboxItems) > 0 {
			m.inboxAt = len(m.inboxItems) - 1
		}
	case "enter", "l", "right":
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

// renderInbox returns the inbox's rows for one viewport, each padded so the
// document column stays aligned. The focused row carries the cursor, a
// resolved thread is dimmed with a checkmark, and a thread whose document is
// gone from the tree says so. It has its own scroll offset, like the pane —
// the document's is left alone, so closing the inbox returns the reader to
// exactly where they were.
func (m *model) renderInbox(viewport int) []string {
	rows := make([]string, viewport)
	w := m.contentWidth()
	if len(m.inboxItems) == 0 {
		rows[0] = strings.Repeat(" ", gutterW) +
			dimStyle.Render("no threads yet — comment on any block to start one")
		return rows
	}
	if m.inboxAt < m.inboxScroll {
		m.inboxScroll = m.inboxAt
	}
	if m.inboxAt >= m.inboxScroll+viewport {
		m.inboxScroll = m.inboxAt - viewport + 1
	}
	if m.inboxScroll > len(m.inboxItems)-viewport {
		m.inboxScroll = max(0, len(m.inboxItems)-viewport)
	}
	for i := 0; i < viewport; i++ {
		idx := m.inboxScroll + i
		if idx >= len(m.inboxItems) {
			break
		}
		item := m.inboxItems[idx]
		txt := item.thread.summary(false)
		// In a tree review each row names its document — the threads span the
		// tree, so a row without its home is unplaceable. In a single-document
		// review every row is the same document, so the name would be a
		// wall of identical noise; the summary alone says which thread it is.
		if m.tree != nil {
			txt = item.doc + "  " + txt
		}
		style := textStyle
		if item.thread.resolved {
			style = dimStyle
		}
		marker := "  "
		if idx == m.inboxAt {
			marker = focusStyle.Render("▌ ")
		}
		resolved := ""
		if item.thread.resolved {
			resolved = resolvedTxt.Render("✓ ")
		}
		if !item.inTree {
			txt += "  [doc not in tree]"
		} else if item.thread.orphaned {
			txt += "  [block gone]"
		}
		txt = truncate(txt, max(0, w-gutterW-4))
		rows[i] = strings.Repeat(" ", gutterW) + marker + resolved + style.Render(txt)
	}
	return rows
}
