// The cross-document comment inbox: in a tree review (D10), `i` swaps the
// document column for a list of every thread across the tree's documents,
// newest activity first, and `enter` on a row opens that thread's document
// and lands on the block it is anchored to. It is the comment-inbox half of
// M3's "navigation between documents, and a cross-document comment inbox";
// the navigation half is `ctrl+]` link following (jump.go).
//
// The inbox is a reading surface, not a new persistence: it reads the same
// thread files under .margin/threads/ everything else reads and writes
// nothing. It is scoped to tree reviews — a single-document review has no
// other documents to aggregate, so `i` is unbound there (D10). Threads whose
// document is no longer in the tree still appear, reported rather than
// silently dropped — the tree-level form of the orphaned-thread rule a
// single document already follows.
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

// toggleInbox opens or closes the inbox view. Opening rebuilds the item list
// from disk, so a comment posted in another document since the last visit
// shows up, and clears the session's other surfaces — raw, search and the
// palette are different views and the inbox takes their column.
func (m *model) toggleInbox() {
	if m.inbox {
		m.inbox = false
		m.status = ""
		return
	}
	if m.tree == nil || m.store == nil {
		return
	}
	items, err := loadInbox(m.store.root, m.tree)
	if err != nil {
		m.status = "inbox: " + err.Error()
		return
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

// openInboxItem jumps to the thread at row i: it opens the thread's document
// exactly as a pane open would (openTreeFile — the same re-pointing, the same
// session reset), then lands focus on the block the thread is anchored to,
// centred, with the landing recorded in the jumplist so ctrl+o walks back.
// A thread whose document is no longer in the tree, or whose block has
// vanished from that document, is reported rather than silently dropped.
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
	// Land on the thread's block, centred like a search or link jump. A
	// thread whose block vanished from its document is surfaced, not dropped:
	// the document is open, focus sits at the top, and the status says what
	// is missing.
	for j, e := range m.entries {
		if e.b.anchor == item.anchor {
			m.pushJump(cursor{entry: j, comment: commentNone})
			m.jumpToEntry(j)
			m.status = fmt.Sprintf("inbox: %s · %s", item.doc, item.thread.summary(false))
			return
		}
	}
	m.status = fmt.Sprintf("inbox: %s — the thread's block is gone", item.doc)
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
		txt := item.doc + "  " + item.thread.summary(false)
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
		}
		txt = truncate(txt, max(0, w-gutterW-4))
		rows[i] = strings.Repeat(" ", gutterW) + marker + resolved + style.Render(txt)
	}
	return rows
}
