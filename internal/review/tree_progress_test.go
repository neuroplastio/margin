package review

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// treeProgressFixture writes a small markdown tree with known markable counts:
// the root file has two paragraphs (total 2), the nested spec one (total 1).
func treeProgressFixture(t *testing.T) (root, a, b string) {
	t.Helper()
	root = t.TempDir()
	if err := os.Mkdir(filepath.Join(root, ".margin"), 0o755); err != nil {
		t.Fatal(err)
	}
	a, b = "README.md", "docs/spec.md"
	writeFile := func(rel, body string) {
		p := filepath.Join(root, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	writeFile(a, "# README\n\nFirst prose.\n\nSecond prose.\n")
	writeFile(b, "# Spec\n\nNested prose.\n")
	return root, a, b
}

func firstMarkableEntry(m *model) int {
	for i, e := range m.entries {
		if e.b.markable() {
			return i
		}
	}
	return -1
}

// markFocused marks the first markable block with want.
func markFocused(t *testing.T, m *model, want reviewMark) {
	t.Helper()
	i := firstMarkableEntry(m)
	if i < 0 {
		t.Fatal("no markable block in the document")
	}
	markFocusedOn(t, m, i, want)
}

// markFocusedOn marks the block at entry i with want.
func markFocusedOn(t *testing.T, m *model, i int, want reviewMark) {
	t.Helper()
	m.at = cursor{entry: i, comment: commentNone}
	m.toggleMark(want)
}

// markAllReviewed marks every markable block reviewed, writing the state
// directly — toggleMark's clear-on-repress is exactly what a test that wants
// "the whole file done" must not trip over.
func markAllReviewed(m *model) {
	for _, e := range m.entries {
		if e.b.markable() && e.b.anchor != "" {
			m.marks[e.b.anchor] = markOK
		}
	}
}

// TestPaneShowsPerFileProgress: a file's pane row carries its reviewed/total —
// green when the whole file is reviewed, orange with a `!` when any block is
// flagged, dim for a partially reviewed file, and blank for a file nobody has
// touched.
func TestPaneShowsPerFileProgress(t *testing.T) {
	root, _, _ := treeProgressFixture(t)
	m, err := newTreeModel(root)
	if err != nil {
		t.Fatalf("newTreeModel: %v", err)
	}
	m.w, m.h = 100, 30

	// Untouched: the README row shows nothing in its progress slot.
	rows := m.renderTree(30)
	if strings.Contains(rows[0], "/") {
		t.Errorf("untouched README row carries progress:\n%q", rows[0])
	}

	// One of two README blocks reviewed: dim "1/2".
	markFocused(t, m, markOK)
	rows = m.renderTree(30)
	if !strings.Contains(rows[0], dimStyle.Render("1/2")) {
		t.Errorf("partially reviewed README row = %q, want dim 1/2", rows[0])
	}

	// The whole README reviewed: green "2/2".
	markAllReviewed(m)
	rows = m.renderTree(30)
	if !strings.Contains(rows[0], okStyle.Render("2/2")) {
		t.Errorf("fully reviewed README row = %q, want green 2/2", rows[0])
	}

	// A flagged block: orange "1/2!" — the `!` signals attention remains.
	markFocusedOn(t, m, firstMarkableEntry(m), markFlag)
	rows = m.renderTree(30)
	if !strings.Contains(rows[0], flagStyle.Render("1/2!")) {
		t.Errorf("flagged README row = %q, want orange 1/2!", rows[0])
	}

	// The spec is still untouched, but every non-empty row keeps the same
	// total width.
	for _, r := range rows {
		if r == "" {
			continue
		}
		if w := visualWidth(r); w != m.treeW {
			t.Errorf("pane row width = %d, want %d:\n%q", w, m.treeW, r)
		}
	}
}

// TestTreeMarkTotalsComputedForEveryFileAtOpen: the tree's progress denominator
// is the whole tree, opened or not — newTreeModel computes a markable count
// for every file, not just the one it opens.
func TestTreeMarkTotalsComputedForEveryFileAtOpen(t *testing.T) {
	root, a, b := treeProgressFixture(t)
	m, err := newTreeModel(root)
	if err != nil {
		t.Fatalf("newTreeModel: %v", err)
	}
	if m.markCache == nil || m.markTotals == nil {
		t.Fatal("tree review has no tree-progress session state")
	}
	if m.markTotals[a] != 2 {
		t.Errorf("markTotals[%s] = %d, want 2", a, m.markTotals[a])
	}
	if m.markTotals[b] != 1 {
		t.Errorf("markTotals[%s] = %d, want 1 (computed before the file was opened)", b, m.markTotals[b])
	}
}

// TestTreeMarksCarriedAcrossSwitches: a switch between documents is a move
// between reviews, not a discard — the outgoing document's marks land in the
// session cache and the incoming document's come back out.
func TestTreeMarksCarriedAcrossSwitches(t *testing.T) {
	root, a, b := treeProgressFixture(t)
	m, err := newTreeModel(root)
	if err != nil {
		t.Fatalf("newTreeModel: %v", err)
	}

	markFocused(t, m, markOK)
	readmeAnchor := m.anchorAt()
	if readmeAnchor == "" {
		t.Fatal("no anchor for the marked README block")
	}

	// Switch to the spec — the README's mark must leave m.marks and enter the
	// cache, so the spec's own (anchor-derived) marks can never collide.
	m.openTreeFile(2)
	if m.store.docPath != b {
		t.Fatalf("store.docPath = %q, want %q", m.store.docPath, b)
	}
	if _, ok := m.marks[readmeAnchor]; ok {
		t.Error("the README's mark leaked into the spec's marks")
	}
	if m.markCache[a][readmeAnchor] != markOK {
		t.Error("the README's mark did not survive into the session cache")
	}

	markFocused(t, m, markFlag)
	specAnchor := m.anchorAt()

	// Back to the README — its mark returns, and the spec's is in the cache.
	m.openTreeFile(0)
	if m.marks[readmeAnchor] != markOK {
		t.Errorf("README mark = %v after switching back, want markOK", m.marks[readmeAnchor])
	}
	if m.markCache[b][specAnchor] != markFlag {
		t.Errorf("spec mark = %v in cache, want markFlag", m.markCache[b][specAnchor])
	}

	// The tree-wide roll-up sees both documents' marks through marksForDoc.
	done, flagged, total := m.treeProgress()
	if done != 1 || flagged != 1 || total != 3 {
		t.Errorf("treeProgress = %d/%d + %d flagged, want 1/3 + 1 flagged", done, total, flagged)
	}
}

// TestFooterRollsUpTheWholeTree: in a tree review the footer shows the tree-wide
// roll-up — reviewed/total across every file's markable blocks (opened or not),
// with the flagged count — instead of the single-document readout.
func TestFooterRollsUpTheWholeTree(t *testing.T) {
	root, _, _ := treeProgressFixture(t)
	m, err := newTreeModel(root)
	if err != nil {
		t.Fatalf("newTreeModel: %v", err)
	}
	m.w, m.h = 100, 20

	// One README block reviewed; the spec untouched but counted in the total.
	markFocused(t, m, markOK)
	out := m.View().Content
	if !strings.Contains(out, "tree 1/3 reviewed") {
		t.Errorf("footer does not roll the whole tree up:\n%s", out)
	}

	// Flag a spec block: the flagged count joins the footer.
	m.openTreeFile(2)
	markFocused(t, m, markFlag)
	out = m.View().Content
	if !strings.Contains(out, "tree 1/3 reviewed") || !strings.Contains(out, "1 flagged") {
		t.Errorf("footer after flagging the spec = %q, want tree 1/3 reviewed · 1 flagged", out)
	}
}
