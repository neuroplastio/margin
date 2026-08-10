// Live reload: notices a thread file an agent writes on disk while margin is
// already running, and pulls it into the in-memory review without the
// reviewer having to reopen the document. STORE-02 covered load-on-open and
// write-on-submit; this is the third piece the STORE-01 journal named, split
// off onto the board as STORE-03 because it needed its own message type
// wired into the Bubble Tea loop.
package review

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/fsnotify/fsnotify"

	tea "charm.land/bubbletea/v2"
)

// threadsChangedMsg says a thread file for the open document changed on disk
// — created, written, or renamed into place — and the in-memory threads
// should be reloaded. It carries no payload: reloadThreads re-reads every
// file for the document rather than trying to interpret which one changed,
// since a single logical save can still produce more than one filesystem
// event (a temp-file-then-rename, for instance).
type threadsChangedMsg struct{}

// docChangedMsg says the document file itself changed on disk — an agent (or
// an editor) rewriting the prose mid-review. It is a different thing from a
// thread file changing, so it is a different message: the conversation moved
// vs. the document moved. The reviewer is told and offered a reload; the
// in-memory document is left untouched until they ask (reloadDoc), so a
// change landing mid-read never yanks the prose out from under them.
type docChangedMsg struct{}

// threadWatcher watches the on-disk directories for one document's threads
// and its document file, turning filesystem events into messages. It follows
// the same shape waitForDamage uses for the composer's pty output: a channel
// wrapped in a tea.Cmd that blocks until there is something worth a redraw
// over.
type threadWatcher struct {
	w *fsnotify.Watcher

	// dir is the threads leaf directory this watcher watches,
	// root/.margin/threads/<docPath>. Events in it are thread changes.
	dir string

	// docDir and docBase watch the document file itself: the directory that
	// contains it (watched, not the file, so an editor that saves via
	// temp-file-then-rename still fires — fsnotify's file watch would be on
	// the old inode) and the file's base name, matched against events that
	// land in that directory. Empty when there is no document to watch.
	docDir  string
	docBase string
}

// newThreadWatcher starts watching root/.margin/threads/docPath and the
// document file itself. fsnotify can only watch a directory that already
// exists, so the threads directory is created upfront if the document has no
// threads yet — one mkdir either way, since the first locally posted comment
// would create the same directory through threadStore.save.
func newThreadWatcher(root, docPath string) (*threadWatcher, error) {
	dir := filepath.Join(threadsDir(root), filepath.FromSlash(docPath))
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("thread watcher: %w", err)
	}
	w, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, fmt.Errorf("thread watcher: %w", err)
	}
	if err := w.Add(dir); err != nil {
		w.Close()
		return nil, fmt.Errorf("thread watcher: %w", err)
	}
	tw := &threadWatcher{w: w, dir: dir}

	// The document file gets its own watch, on its containing directory: the
	// change-notification feedback ("the .md file is not watched; nothing
	// re-reads the document until the next open"). The directory rather than
	// the file, so a save that renames a temp file into place is still seen.
	absDoc := filepath.Join(root, filepath.FromSlash(docPath))
	tw.docDir, tw.docBase = filepath.Dir(absDoc), filepath.Base(absDoc)
	if tw.docDir != dir {
		if err := w.Add(tw.docDir); err != nil {
			w.Close()
			return nil, fmt.Errorf("thread watcher: %w", err)
		}
	}
	return tw, nil
}

// close releases the underlying OS watch. Nil-safe, matching threadStore.save
// — Run defers this unconditionally, whether or not a watcher was ever set up.
func (tw *threadWatcher) close() error {
	if tw == nil {
		return nil
	}
	return tw.w.Close()
}

// wait blocks for the next event worth waking the model over. A change to the
// document file is a docChangedMsg — the prose moved; a create, write or
// rename of a .md thread file is a threadsChangedMsg — the conversation moved.
// Anything else — a directory mtime bump, a stray non-markdown file, an
// unrelated sibling document — is filtered out here so the model only ever
// wakes up for something it can act on. A closed watcher (Run's deferred
// close, once the program has quit) or an fsnotify-level error both end the
// loop by returning a nil message: live reload stopping is not worth
// interrupting the reviewer over, since the document is still fully readable
// without it.
func (tw *threadWatcher) wait() tea.Cmd {
	if tw == nil {
		return nil
	}
	return func() tea.Msg {
		for {
			select {
			case ev, ok := <-tw.w.Events:
				if !ok {
					return nil
				}
				if ev.Op&(fsnotify.Write|fsnotify.Create|fsnotify.Rename) == 0 {
					continue
				}
				// A thread file's name is <anchor-id>.md, always inside the
				// threads leaf dir; the document is a specific base name in
				// its own directory. Each event is attributed to the
				// directory it came from, so a sibling document edited next
				// to the one being reviewed never wakes the reviewer.
				if tw.docDir != "" && filepath.Dir(ev.Name) == tw.docDir && filepath.Base(ev.Name) == tw.docBase {
					return docChangedMsg{}
				}
				if filepath.Dir(ev.Name) == tw.dir && strings.HasSuffix(ev.Name, ".md") {
					return threadsChangedMsg{}
				}
				continue
			case _, ok := <-tw.w.Errors:
				if !ok {
					return nil
				}
				return nil
			}
		}
	}
}

// reloadThreads re-reads every thread file on disk for the open document and
// merges what it finds into the in-memory threads: a new anchor is added, an
// existing one has its quote, posted comments and resolved flag replaced with
// what is on disk — an agent resolving a thread by editing its file directly
// should be noticed the same way a reply is. Local-only state — an
// unsubmitted draft — is never touched here, since writeThreadFile never
// writes drafts to begin with, so there is nothing on disk that could clobber
// one.
func (m *model) reloadThreads() error {
	if m.store == nil {
		return nil
	}
	onDisk, err := loadThreadsForDoc(m.store.root, m.store.docPath)
	if err != nil {
		return err
	}
	for anchor, t := range onDisk {
		if existing, ok := m.threads[anchor]; ok {
			existing.quote = t.quote
			existing.posted = t.posted
			existing.resolved = t.resolved
		} else {
			m.threads[anchor] = t
		}
	}
	reattach(m.doc, m.threads)
	m.rebuild()
	return nil
}

// noticeNewEvents reports a comment that landed while the reviewer was
// watching — the "a new comment was posted" half of the change-notification
// feedback. The event log (D13/D14) is what distinguishes a new
// comment.posted from a reload of threads that already existed, so this reads
// the events strictly after the last one this session has seen rather than
// diffing thread files. The reviewer's own posts never reach here: every emit
// advances lastEventID as it writes, so this session's own lines are already
// "seen" by the time their thread-file write wakes the watcher.
func (m *model) noticeNewEvents() {
	evs, err := readEventsAfter(m.store.root, m.lastEventID)
	if err != nil || len(evs) == 0 {
		return
	}
	m.lastEventID = evs[len(evs)-1].id
	for i := len(evs) - 1; i >= 0; i-- {
		if evs[i].kind == eventCommentPosted {
			m.status = fmt.Sprintf("new comment from %s on %s", evs[i].author, evs[i].anchor)
			return
		}
	}
}

// reloadDoc re-reads the document from disk and swaps it in — the reload half
// of the "file changed on disk" notice, so an agent's rewrite of the prose
// becomes visible without quitting and reopening. Threads reattach the same
// way they do on open (reattach), marks survive because they are keyed by
// anchor, and focus follows the block it was on, by anchor — an unchanged
// block keeps its stamped id and its focus even if everything around it moved.
//
// It refuses while a composer is open: the prose is being written to, not
// read, and swapping the document out from under a live edit would be exactly
// the "lose the user's words" failure D7 forbids.
func (m *model) reloadDoc() {
	if m.store == nil {
		m.status = "nothing to reload"
		return
	}
	if m.comp != nil {
		m.status = "close the editor before reloading the document"
		return
	}
	path := filepath.Join(m.store.root, filepath.FromSlash(m.store.docPath))
	doc, src, err := loadDoc(path)
	if err != nil {
		m.status = "reload: " + err.Error()
		return
	}
	prevAnchor := m.anchorAt()
	prevComment := m.at.comment
	prevEntry := m.at.entry
	m.doc = doc
	m.src = src
	m.rawH = 0
	// The tree's per-document totals follow the reloaded blocks, so a
	// document an agent rewrote re-counts its markable blocks (a reload is
	// exactly the "prose moved under me" case a stale total would misreport).
	if m.markTotals != nil {
		m.markTotals[m.store.docPath] = markableTotal(doc)
	}
	reattach(m.doc, m.threads)
	m.rebuild()
	// Re-focus: by anchor when the focused block still exists (the common
	// case — an edit elsewhere in the file), otherwise clamped to the
	// nearest entry. A comment dive keeps its comment when the thread row
	// survived; a line dive is cancelled, since the block's lines may have
	// moved under it.
	m.at = cursor{entry: 0, comment: commentNone}
	if prevAnchor != "" {
		for i, e := range m.entries {
			if e.b.anchor == prevAnchor {
				m.at = cursor{entry: i, comment: commentNone}
				if prevComment >= 0 {
					if t := e.thread; t != nil && prevComment < len(t.posted) {
						m.at.comment = prevComment
					}
				}
				break
			}
		}
	} else if len(m.entries) > 0 {
		if prevEntry >= len(m.entries) {
			prevEntry = len(m.entries) - 1
		}
		m.at = cursor{entry: prevEntry, comment: commentNone}
	}
	m.visual = false
	m.pendingKey = ""
	m.count = ""
	m.docChanged = false
	m.scrollAnchor = cursor{entry: -1, comment: commentNone}
	m.status = fmt.Sprintf("reloaded — %d blocks", len(m.doc))
}
