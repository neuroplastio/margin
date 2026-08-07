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

// threadWatcher watches the on-disk directory for one document's threads and
// turns filesystem events into threadsChangedMsg. It follows the same shape
// waitForDamage uses for the composer's pty output: a channel wrapped in a
// tea.Cmd that blocks until there is something worth a redraw over.
type threadWatcher struct {
	w *fsnotify.Watcher
}

// newThreadWatcher starts watching root/.margin/threads/docPath for changes.
// fsnotify can only watch a directory that already exists, so this creates
// it upfront if the document has no threads yet — one mkdir either way,
// since the first locally posted comment would create the same directory
// through threadStore.save.
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
	return &threadWatcher{w: w}, nil
}

// close releases the underlying OS watch. Nil-safe, matching threadStore.save
// — Run defers this unconditionally, whether or not a watcher was ever set up.
func (tw *threadWatcher) close() error {
	if tw == nil {
		return nil
	}
	return tw.w.Close()
}

// wait blocks for the next event worth reloading over: a create, write or
// rename of a .md file. Anything else — a directory mtime bump, a stray
// non-markdown file — is filtered out here so the model only ever wakes up
// for something it can act on. A closed watcher (Run's deferred close, once
// the program has quit) or an fsnotify-level error both end the loop by
// returning a nil message: live reload stopping is not worth interrupting
// the reviewer over, since the document is still fully readable without it.
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
				if !strings.HasSuffix(ev.Name, ".md") {
					continue
				}
				if ev.Op&(fsnotify.Write|fsnotify.Create|fsnotify.Rename) == 0 {
					continue
				}
				return threadsChangedMsg{}
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
