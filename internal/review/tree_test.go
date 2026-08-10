package review

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
)

// treeFixture writes a small markdown tree under a temp dir: a root file, a
// nested one, and a non-markdown file that must never appear. A .margin marker
// makes the temp dir its own review root, so docPaths come out clean. Returns
// the root and the relative paths of the two markdown files.
func treeFixture(t *testing.T) (root, a, b string) {
	t.Helper()
	root = t.TempDir()
	if err := os.Mkdir(filepath.Join(root, ".margin"), 0o755); err != nil {
		t.Fatal(err)
	}
	a = "README.md"
	b = "docs/spec.md"
	writeFile := func(rel, body string) {
		p := filepath.Join(root, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	writeFile(a, "# README\n\nSome root prose.\n")
	writeFile(b, "# Spec\n\nSome nested prose.\n")
	writeFile("notes.txt", "not markdown\n")
	return root, a, b
}

func TestWalkMarkdownTreeListsOnlyMarkdown(t *testing.T) {
	root, a, b := treeFixture(t)
	tree, err := walkMarkdownTree(root, root)
	if err != nil {
		t.Fatalf("walkMarkdownTree: %v", err)
	}
	var files []string
	for _, e := range tree {
		if !e.isDir {
			files = append(files, e.rel)
		}
	}
	if len(files) != 2 || files[0] != a || files[1] != b {
		t.Errorf("files = %v, want [%s %s]", files, a, b)
	}
}

func TestBuildTreeEmitsDirHeadersOnlyForDirsWithMarkdown(t *testing.T) {
	root, _, b := treeFixture(t)
	tree, err := walkMarkdownTree(root, root)
	if err != nil {
		t.Fatalf("walkMarkdownTree: %v", err)
	}
	// Rows come in sorted-file order: README.md first (root, no header), then
	// the docs/ header it needs, then its file. The only directory with
	// markdown beneath it is docs/, and only its header appears.
	var got []string
	for _, e := range tree {
		kind := "file"
		if e.isDir {
			kind = "dir"
		}
		got = append(got, kind+":"+e.name)
	}
	want := []string{"file:README.md", "dir:docs", "file:spec.md"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("rows = %v, want %v", got, want)
	}
	if tree[1].depth != 0 || tree[2].depth != 1 {
		t.Errorf("depths = %d,%d, want 0,1", tree[1].depth, tree[2].depth)
	}
	if b != tree[2].rel {
		t.Errorf("file row rel = %q, want %q", tree[2].rel, b)
	}
}

func TestBuildTreeOmitsEmptyDirs(t *testing.T) {
	root := t.TempDir()
	// empty/ has no markdown, docs/ has one — only docs/ appears.
	for _, p := range []string{"docs/spec.md", "empty/"} {
		ap := filepath.Join(root, p)
		if err := os.MkdirAll(filepath.Dir(ap), 0o755); err != nil {
			t.Fatal(err)
		}
		if strings.HasSuffix(p, ".md") {
			if err := os.WriteFile(ap, []byte("# x\n"), 0o644); err != nil {
				t.Fatal(err)
			}
		}
	}
	tree, err := walkMarkdownTree(root, root)
	if err != nil {
		t.Fatalf("walkMarkdownTree: %v", err)
	}
	for _, e := range tree {
		if e.isDir && e.name == "empty" {
			t.Error("empty/ appears in the tree; a directory with no markdown beneath must not")
		}
	}
}

func TestNewTreeModelOpensTheFirstFile(t *testing.T) {
	root, a, _ := treeFixture(t)
	m, err := newTreeModel(root)
	if err != nil {
		t.Fatalf("newTreeModel: %v", err)
	}
	if m.tree == nil || len(m.tree) != 3 {
		t.Fatalf("tree rows = %d, want 3", len(m.tree))
	}
	if m.store == nil || m.store.docPath != a {
		t.Errorf("store.docPath = %v, want %s", m.store.docPath, a)
	}
	if m.treeAt != 0 || m.tree[0].rel != a {
		t.Errorf("treeAt = %d (%s), want the root README", m.treeAt, m.tree[m.treeAt].rel)
	}
	if m.path == "" || filepath.Base(m.path) != "README.md" {
		t.Errorf("path = %q, want the README", m.path)
	}
}

func TestNewTreeModelRejectsDirWithoutMarkdown(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "notes.txt"), []byte("nope"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := newTreeModel(root); err == nil {
		t.Error("a directory with no markdown reported success")
	}
}

func TestOpenTreeFileSwitchesTheDocument(t *testing.T) {
	root, _, b := treeFixture(t)
	m, err := newTreeModel(root)
	if err != nil {
		t.Fatalf("newTreeModel: %v", err)
	}
	// Focus the docs/spec.md row (index 2) and open it.
	m.treeAt = 2
	m.openTreeFile(2)
	if m.store.docPath != b {
		t.Errorf("store.docPath = %q, want %q", m.store.docPath, b)
	}
	if filepath.Base(m.path) != "spec.md" {
		t.Errorf("path = %q, want the spec", m.path)
	}
	if m.treeFocus {
		t.Error("treeFocus stayed set after opening a file")
	}
	if !strings.Contains(m.status, b) {
		t.Errorf("status = %q, want it to name the opened file", m.status)
	}
	if len(m.doc) == 0 {
		t.Error("the switched document has no blocks")
	}
}

func TestOpenTreeFileSkipsDirRows(t *testing.T) {
	root, a, _ := treeFixture(t)
	m, err := newTreeModel(root)
	if err != nil {
		t.Fatalf("newTreeModel: %v", err)
	}
	// treeAt is already on the README; opening the docs/ dir row (1) must do
	// nothing — a directory is structure, not a document.
	m.treeAt = 1
	before := m.store.docPath
	m.openTreeFile(1)
	if m.store.docPath != before {
		t.Errorf("opening a dir row changed the document to %q", m.store.docPath)
	}
	if m.store.docPath != a {
		t.Errorf("store.docPath = %q, want %q", m.store.docPath, a)
	}
}

func TestTreeFocusTogglesWithTabCommand(t *testing.T) {
	root, _, _ := treeFixture(t)
	m, err := newTreeModel(root)
	if err != nil {
		t.Fatalf("newTreeModel: %v", err)
	}
	cmd, ok := commandByID("tree.focus")
	if !ok {
		t.Fatal("tree.focus is not registered")
	}
	cmd.Run(m, "")
	if !m.treeFocus {
		t.Error("tree.focus did not move focus into the pane")
	}
	cmd.Run(m, "")
	if m.treeFocus {
		t.Error("tree.focus did not return focus to the document")
	}
}

func TestPaneKeysNavigateAndOpen(t *testing.T) {
	root, a, b := treeFixture(t)
	m, err := newTreeModel(root)
	if err != nil {
		t.Fatalf("newTreeModel: %v", err)
	}
	key := func(r rune) tea.KeyPressMsg {
		return tea.KeyPressMsg(tea.Key{Code: r, Text: string(r)})
	}
	// tab moves focus into the pane, landing on the open document's row.
	m.handleKey(tea.KeyPressMsg(tea.Key{Code: tea.KeyTab}))
	if !m.treeFocus {
		t.Fatal("tab did not focus the pane")
	}
	// j moves past the docs/ header to the spec file; k comes back.
	m.handleKey(key('j'))
	m.handleKey(key('j'))
	if m.treeAt != 2 || m.tree[m.treeAt].rel != b {
		t.Fatalf("after j: treeAt = %d (%s), want the spec row", m.treeAt, m.tree[m.treeAt].rel)
	}
	m.handleKey(key('k'))
	m.handleKey(key('k'))
	if m.tree[m.treeAt].rel != a {
		t.Fatalf("after k: on %s, want the README", m.tree[m.treeAt].rel)
	}
	// enter opens the focused file and hands focus to the document.
	m.handleKey(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	if m.store.docPath != a {
		t.Errorf("store.docPath = %q, want %q", m.store.docPath, a)
	}
	if m.treeFocus {
		t.Error("opening left focus in the pane")
	}
}

func TestRenderTreeMarksFocusAndOpenDoc(t *testing.T) {
	root, _, _ := treeFixture(t)
	m, err := newTreeModel(root)
	if err != nil {
		t.Fatalf("newTreeModel: %v", err)
	}
	m.w, m.h = 100, 30
	m.treeFocus = true
	rows := m.renderTree(30)
	if len(rows) != 30 {
		t.Fatalf("renderTree returned %d rows, want 30", len(rows))
	}
	// Row 0 is the open README and the focused row — the cursor glyph.
	if !strings.Contains(rows[0], "▌") {
		t.Errorf("focused row has no cursor:\n%q", rows[0])
	}
	// Row 1 is the docs/ directory header, dimmed, not a focus stop.
	if !strings.Contains(rows[1], "docs/") {
		t.Errorf("dir header missing:\n%q", rows[1])
	}
}

func TestViewShrinksContentWidthForThePane(t *testing.T) {
	root, _, _ := treeFixture(t)
	m, err := newTreeModel(root)
	if err != nil {
		t.Fatalf("newTreeModel: %v", err)
	}
	m.w = 100
	if m.docX() != m.treeW+1 {
		t.Errorf("docX = %d, want treeW+1 = %d", m.docX(), m.treeW+1)
	}
	single := newModel(parseDoc([]byte("# x\n")), nil)
	single.w = 100
	if single.contentWidth() != m.contentWidth()+m.docX() {
		t.Errorf("tree contentWidth %d should be the single-doc width %d minus the pane %d",
			m.contentWidth(), single.contentWidth(), m.docX())
	}
}

func TestTreeReviewRunModelKeepsExportSingleDoc(t *testing.T) {
	root, _, _ := treeFixture(t)
	m, err := newTreeModel(root)
	if err != nil {
		t.Fatalf("newTreeModel: %v", err)
	}
	if m.ephemeral {
		t.Error("a tree review is not ephemeral")
	}
}

func TestViewComposesPaneAndDocumentColumns(t *testing.T) {
	root, _, _ := treeFixture(t)
	m, err := newTreeModel(root)
	if err != nil {
		t.Fatalf("newTreeModel: %v", err)
	}
	m.w, m.h = 100, 20
	out := m.View().Content
	// Both pane rows render: the open file (with its ▸ marker), the dimmed
	// directory header, and the nested file indented beneath it.
	for _, want := range []string{"README.md", "docs/", "spec.md", "│"} {
		if !strings.Contains(out, want) {
			t.Errorf("view does not contain %q:\n%s", want, out)
		}
	}
	// The document itself still renders to the right of the pane (ANSI runs
	// split words, so match a contiguous chunk).
	if !strings.Contains(out, "prose") {
		t.Errorf("document column missing:\n%s", out)
	}
}
