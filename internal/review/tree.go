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

	hottycat "github.com/neuroplastio/hotty-a2ui/catalog/hotty"
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

// progW is the width of the right-aligned progress slot every pane row
// carries — a file's "reviewed/total", or blank where there is none. It is a
// fixed slot rather than appended text so the document column stays aligned
// whether or not a row has anything to show.
const progW = 7

// treeWidth is the pane's column width: the name portion sized to the widest
// row and clamped, plus the fixed progress slot, the whole thing capped so a
// long path cannot swallow the document column.
func treeWidth(tree []treeEntry) int {
	w := 0
	for _, e := range tree {
		// The tree's rule and branch glyphs take four columns a level,
		// the selection bar two, and a progress label rides after the name.
		if l := e.depth*4 + len(e.name) + 6; l > w {
			w = l
		}
	}
	return min(max(w, 16), 36-progW) + progW
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
	// The tree-progress session state: markCache holds each document's marks
	// (session-local — nothing persists marks, D5), and markTotals holds every
	// document's markable block count, computed up front so the pane's n/m and
	// the footer's tree-wide roll-up have a whole-tree denominator rather than
	// "the files opened so far". The open document's total comes from the doc
	// already loaded; the rest parse once, defensively skipping any file that
	// will not open (it contributes nothing to progress, which is honest).
	m.markCache = map[string]map[string]reviewMark{}
	m.lineMarkCache = map[string]map[string]reviewMark{}
	m.markTotals = map[string]int{}
	m.markTotals[rel] = markableTotal(doc)
	for _, e := range tree {
		if e.isDir || e.rel == rel {
			continue
		}
		if blocks, _, err := loadDoc(filepath.Join(root, filepath.FromSlash(e.rel))); err == nil {
			m.markTotals[e.rel] = markableTotal(blocks)
		}
	}
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
	m.hotOpen()
	// Tree progress is session state keyed per document: the outgoing
	// document's marks go into the cache and the incoming document's come back
	// out — a switch is a move between reviews, not a discard of them. The
	// cache holds the rolled-up effective marks (line marks folded into their
	// block), and the raw line marks ride alongside so a diveable block's
	// lines survive the round trip intact. Saved before m.doc is swapped —
	// effectiveMarks reads the outgoing document's blocks — and restored after
	// the swap, so the incoming document's own state is what lands.
	if m.markCache != nil {
		m.markCache[m.store.docPath] = m.effectiveMarks()
		m.lineMarkCache[m.store.docPath] = m.lineMarks
	}
	m.doc = doc
	m.src = src
	m.threads = threads
	if m.markCache != nil {
		m.marks = m.markCache[e.rel]
		if m.marks == nil {
			m.marks = map[string]reviewMark{}
		}
		m.lineMarks = m.lineMarkCache[e.rel]
		if m.lineMarks == nil {
			m.lineMarks = map[string]reviewMark{}
		}
	}
	m.store.docPath = e.rel
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

// treeID is the tree pane's pane (D22): the review's markdown files as a
// tree of folders.
const treeID = "tree"

// treePane is the tree pane, rows tall, as the model has it: the files, the
// one focused (the tree's selection), and the open one marked ▸. Each file
// carries its reviewed/total once it has marks. margin keeps the focus and
// the keys; a click on a file focuses it (treeClicked). Folders start open,
// and stay as the reviewer folds them.
func (m *model) treePane(rows int) *pane {
	var open []any
	for _, e := range m.tree {
		if e.isDir {
			open = append(open, e.rel+"/")
		}
	}
	p := m.panes().pane(treeID, map[string]any{"items": []any{}, "sel": "", "open": open})
	p.define(obj("id", "root", "component", "HottyTree", "catalogId", hottycat.ID,
		"items", bind("/items"), "selected", bind("/sel"), "expanded", bind("/open"),
		"height", max(rows, 1)))
	p.put("/items", m.treeNodes())
	if m.treeAt >= 0 && m.treeAt < len(m.tree) {
		p.put("/sel", m.tree[m.treeAt].rel)
	}
	return p
}

// treeNodes is the tree as HottyTree's nodes: a folder's value is its path
// and a slash, a file's its path.
func (m *model) treeNodes() []any {
	nodes, _ := m.treeLevel(0, -1)
	return nodes
}

// treeLevel is the nodes of m.tree from i on that are deeper than depth,
// and the index of the row after them.
func (m *model) treeLevel(i, depth int) ([]any, int) {
	nodes := []any{}
	for i < len(m.tree) && m.tree[i].depth > depth {
		e := m.tree[i]
		if e.isDir {
			kids, next := m.treeLevel(i+1, e.depth)
			nodes = append(nodes, obj("label", e.name, "value", e.rel+"/", "children", kids))
			i = next
			continue
		}
		label := e.name
		if m.store != nil && e.rel == m.store.docPath {
			label = "▸ " + label
		}
		if prog := m.fileProgress(e); prog != "" {
			label += "  " + prog
		}
		nodes = append(nodes, obj("label", label, "value", e.rel))
		i++
	}
	return nodes, i
}

// treeClicked takes the file a click in the tree pane selected: the pane has
// the keyboard from then on, as a click on its row always gave it.
func (m *model) treeClicked(p *pane) {
	sel := p.text("/sel")
	for i, e := range m.tree {
		if !e.isDir && e.rel == sel {
			m.treeAt = i
			m.treeFocus = true
			return
		}
	}
}

// renderTree returns the pane's rows for one viewport, each padded to treeW
// so the document column stays aligned.
func (m *model) renderTree(viewport int) []string {
	rows := make([]string, viewport)
	if len(m.tree) == 0 {
		return rows
	}
	lines := m.treePane(viewport).lines(m.treeW)
	for i := range rows {
		l := ""
		if i < len(lines) {
			l = lines[i]
		}
		rows[i] = l + strings.Repeat(" ", max(m.treeW-lipgloss.Width(l), 0))
	}
	return rows
}

// fileProgress is a file's reviewed/total once it has marks: with a
// trailing ! when any block is flagged. A file nobody has touched (or a
// directory) has none: a freshly opened tree would otherwise be
// wall-to-wall zeros, which reads as "nothing", and the tree-wide footer
// roll-up already carries the untouched denominator.
func (m *model) fileProgress(e treeEntry) string {
	if e.isDir {
		return ""
	}
	marks := m.marksForDoc(e.rel)
	if len(marks) == 0 {
		return ""
	}
	n, flagged := 0, false
	for _, mk := range marks {
		switch mk {
		case markOK:
			n++
		case markFlag:
			flagged = true
		}
	}
	total := m.markTotals[e.rel]
	switch {
	case flagged:
		return fmt.Sprintf("%d/%d!", n, total)
	case total > 0:
		return fmt.Sprintf("%d/%d", n, total)
	}
	return ""
}
