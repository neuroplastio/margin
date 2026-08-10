// The tree review (D10): `margin` with no arguments opens a tree of the
// working directory and `margin DIR/` opens a tree of that directory, while
// `margin FILE.md` stays a single-document review. A left pane lists the
// markdown files beneath the directory; `tab` moves keyboard focus between the
// pane and the document, j/k move through the files, and `enter` opens the
// focused one — loading its document, threads, store and watcher in place.
//
// What the pane looks like, where it sits and how it is toggled and focused
// was felt and unsettled (interaction.md "Not settled"); this leg picked and
// built the shape below and the maintainer judges it.
package review

import (
	"fmt"
	"io/fs"
	"path"
	"path/filepath"
	"sort"
	"strings"

	"charm.land/lipgloss/v2"
	tea "charm.land/bubbletea/v2"
)

// treeEntry is one row of the file-tree pane: either a directory header
// (dimmed, not a focus stop) or a markdown file. rel is the document's path
// relative to the review root — the same docPath threads are keyed by (D9) —
// slash-separated.
type treeEntry struct {
	rel   string
	depth int
	name  string
	isDir bool
}

// walkMarkdownTree returns the tree rows for every markdown file beneath dir,
// with paths relative to root. Hidden directories are skipped: .git and
// .margin are tooling, and .margin in particular would otherwise list every
// thread file as a reviewable document. A directory with no markdown anywhere
// beneath it never appears (D10) because rows are built only from markdown
// files' ancestor directories.
func walkMarkdownTree(dir, root string) ([]treeEntry, error) {
	var files []string
	err := filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if p != dir && strings.HasPrefix(d.Name(), ".") {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(d.Name(), ".md") && !strings.HasSuffix(d.Name(), ".markdown") {
			return nil
		}
		rel, rerr := filepath.Rel(root, p)
		if rerr != nil {
			return rerr
		}
		files = append(files, filepath.ToSlash(rel))
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Strings(files)
	return buildTree(files), nil
}

// buildTree turns a sorted list of markdown file paths into pane rows: a
// dimmed header row for each directory that has markdown beneath it, then its
// files, in tree order.
func buildTree(files []string) []treeEntry {
	var out []treeEntry
	seenDir := map[string]bool{}
	for _, f := range files {
		dir := path.Dir(f)
		var chain []string
		for d := dir; d != "." && d != "/"; d = path.Dir(d) {
			chain = append(chain, d)
		}
		for i := len(chain) - 1; i >= 0; i-- {
			d := chain[i]
			if !seenDir[d] {
				seenDir[d] = true
				out = append(out, treeEntry{rel: d, depth: strings.Count(d, "/"), name: path.Base(d), isDir: true})
			}
		}
		depth := 0
		if dir != "." {
			depth = strings.Count(dir, "/") + 1
		}
		out = append(out, treeEntry{rel: f, depth: depth, name: path.Base(f)})
	}
	return out
}

// treeWidth is the pane's column width, sized to its widest row and clamped so
// a long path cannot swallow the document column.
func treeWidth(tree []treeEntry) int {
	w := 0
	for _, e := range tree {
		if l := e.depth*2 + len(e.name) + 3; l > w {
			w = l
		}
	}
	return min(max(w, 16), 36)
}

// newTreeModel builds the model for a tree review of dir: the pane rows from
// walking it, and the first markdown file loaded as the open document.
func newTreeModel(dir string) (*model, error) {
	root, _ := resolveReviewRoot(dir)
	tree, err := walkMarkdownTree(dir, root)
	if err != nil {
		return nil, err
	}
	// A directory outside the resolved root — a marker-less dir given from
	// somewhere else, so root fell back to cwd — is its own review root:
	// re-walk with it as the base so docPaths stay inside it.
	for _, e := range tree {
		if !e.isDir && strings.HasPrefix(e.rel, "..") {
			root = dir
			if tree, err = walkMarkdownTree(dir, root); err != nil {
				return nil, err
			}
			break
		}
	}
	first := -1
	for i, e := range tree {
		if !e.isDir {
			first = i
			break
		}
	}
	if first < 0 {
		return nil, fmt.Errorf("run: no markdown files under %s", dir)
	}
	rel := tree[first].rel
	abs := filepath.Join(root, filepath.FromSlash(rel))
	doc, src, err := loadDoc(abs)
	if err != nil {
		return nil, err
	}
	threads, err := loadThreadsForDoc(root, rel)
	if err != nil {
		return nil, fmt.Errorf("run: %w", err)
	}
	m := newModelAt(abs, doc, threads)
	m.src = src
	m.tree = tree
	m.treeAt = first
	m.treeW = treeWidth(tree)
	m.store = &threadStore{root: root, docPath: rel}
	// The change-notification cursor starts at the newest event already on
	// disk — the same seed the single-document path sets, so events from
	// before the session are history, not news.
	if evs, err := readEvents(root); err == nil && len(evs) > 0 {
		m.lastEventID = evs[len(evs)-1].id
	}
	return m, nil
}

// docX is the column the document's gutter starts at: after the tree pane and
// its separator when reviewing a directory, else 0. Mouse hit-testing and the
// composer cursor offset by it.
func (m *model) docX() int {
	if m.tree == nil {
		return 0
	}
	return m.treeW + 1
}

// treeEnd moves treeAt to the first or last file row.
func (m *model) treeEnd(last bool) {
	if m.tree == nil {
		return
	}
	step, i := 1, 0
	if last {
		step, i = -1, len(m.tree)-1
	}
	for i >= 0 && i < len(m.tree) {
		if !m.tree[i].isDir {
			m.treeAt = i
			return
		}
		i += step
	}
}

// moveTree moves treeAt to the next/previous file row, skipping directory
// headers — a directory is structure, not a focus stop.
func (m *model) moveTree(d int) {
	if m.tree == nil || len(m.tree) == 0 {
		return
	}
	i := m.treeAt + d
	for i >= 0 && i < len(m.tree) && m.tree[i].isDir {
		i += d
	}
	if i < 0 || i >= len(m.tree) {
		return
	}
	m.treeAt = i
}

// openTreeFile loads the file at row i into the document pane — the "enter
// opens the focused file" half of the tree review. Threads load for the new
// document, the store and watcher re-point at it, and everything session-local
// (marks, scroll, raw, search, selection) resets: opening a file is a fresh
// review of a fresh document. Refuses while a composer is open, the same rule
// reloadDoc uses.
func (m *model) openTreeFile(i int) {
	if m.tree == nil || m.store == nil || i < 0 || i >= len(m.tree) {
		return
	}
	e := m.tree[i]
	if e.isDir {
		return
	}
	if e.rel == m.store.docPath {
		m.treeFocus = false
		return
	}
	if m.comp != nil {
		m.status = "close the editor before opening a file"
		return
	}
	abs := filepath.Join(m.store.root, filepath.FromSlash(e.rel))
	doc, src, err := loadDoc(abs)
	if err != nil {
		m.status = "open: " + err.Error()
		return
	}
	threads, err := loadThreadsForDoc(m.store.root, e.rel)
	if err != nil {
		m.status = "open: " + err.Error()
		return
	}
	if m.watcher != nil {
		m.watcher.close()
	}
	m.watcher = nil
	if watcher, err := newThreadWatcher(m.store.root, e.rel); err == nil {
		m.watcher = watcher
	}
	m.path = abs
	m.doc = doc
	m.src = src
	m.threads = threads
	m.store.docPath = e.rel
	m.marks = map[string]reviewMark{}
	m.codeScroll = map[string]int{}
	m.raw = false
	m.rawH = 0
	m.at = cursor{entry: 0, comment: commentNone}
	m.scrollAnchor = cursor{entry: -1, comment: commentNone}
	m.scroll = 0
	m.visual = false
	m.pendingKey = ""
	m.count = ""
	m.searchOpen = false
	m.searchQuery = ""
	m.searchMatches = nil
	m.searchCurrent = 0
	m.paletteOpen = false
	m.docChanged = false
	// The jumplist's cursors name the previous document's entries — stale
	// once the doc under review changes, so it restarts at the new top.
	m.jumps = []cursor{{entry: 0, comment: commentNone}}
	m.jumpIdx = 0
	reattach(m.doc, m.threads)
	m.rebuild()
	m.treeAt = i
	m.treeFocus = false
	m.status = fmt.Sprintf("opened %s — %d blocks", e.rel, len(m.doc))
}

// handlePaneKey routes keys while focus sits in the tree pane. It is a modal
// surface like the composer and search: a small set of pane keys, everything
// else ignored, and tab/esc/h return focus to the document. q keeps its quit
// meaning everywhere.
func (m *model) handlePaneKey(msg tea.KeyPressMsg) tea.Cmd {
	switch msg.String() {
	case "j", "down":
		m.moveTree(1)
	case "k", "up":
		m.moveTree(-1)
	case "g":
		m.treeEnd(false)
	case "G":
		m.treeEnd(true)
	case "enter", "l", "right":
		m.openTreeFile(m.treeAt)
	case "tab", "esc", "h", "left":
		m.treeFocus = false
		m.status = ""
	case "q", "ctrl+c":
		m.quitting = true
		return tea.Quit
	}
	return nil
}

// renderTree returns the pane's rows for one viewport, each padded to treeW so
// the document column stays aligned. The focused row carries the cursor, the
// open document carries a marker, directories are dimmed headers.
func (m *model) renderTree(viewport int) []string {
	rows := make([]string, viewport)
	if m.tree == nil || len(m.tree) == 0 {
		return rows
	}
	// Keep the focused row in view: this is the pane's own scroll, separate
	// from the document's (a long tree and a long document scroll
	// independently).
	if m.treeAt < m.treeScroll {
		m.treeScroll = m.treeAt
	}
	if m.treeAt >= m.treeScroll+viewport {
		m.treeScroll = m.treeAt - viewport + 1
	}
	if m.treeScroll < 0 {
		m.treeScroll = 0
	}
	for i := 0; i < viewport; i++ {
		idx := m.treeScroll + i
		if idx >= len(m.tree) {
			break
		}
		e := m.tree[idx]
		indent := strings.Repeat("  ", e.depth)
		switch {
		case e.isDir:
			rows[i] = dimStyle.Render(indent + e.name + "/")
		case idx == m.treeAt && m.treeFocus:
			rows[i] = focusStyle.Render("▌ ") + textStyle.Render(indent+e.name)
		case m.store != nil && e.rel == m.store.docPath:
			rows[i] = dimStyle.Render("▸ ") + textStyle.Render(indent+e.name)
		default:
			rows[i] = "  " + dimStyle.Render(indent+e.name)
		}
		rows[i] = lipgloss.NewStyle().Width(m.treeW).Render(rows[i])
	}
	return rows
}
