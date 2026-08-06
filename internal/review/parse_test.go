package review

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const sampleDoc = `# Retry policy

Each outbound call is retried up to three times with exponential
backoff starting at 100ms.

## Budgets

The retry budget is shared across all endpoints.

- per-endpoint caps
- a global ceiling

` + "```go" + `
func retry(n int) error {
    return nil
}
` + "```" + `

> Breaker state is per-process.

---

Final paragraph.
`

func parseSample(t *testing.T) []block {
	t.Helper()
	return parseDoc([]byte(sampleDoc))
}

func TestParseFindsTopLevelBlocks(t *testing.T) {
	blocks := parseSample(t)

	var got []blockKind
	for _, b := range blocks {
		got = append(got, b.kind)
	}
	want := []blockKind{
		blockHeading, // # Retry policy
		blockPara,    // Each outbound call…
		blockHeading, // ## Budgets
		blockPara,    // The retry budget…
		blockRaw,     // list
		blockRaw,     // code fence
		blockRaw,     // block quote
		blockPara,    // Final paragraph.
	}
	if len(got) != len(want) {
		for i, b := range blocks {
			t.Logf("  [%d] kind=%d %q", i, b.kind, truncate(firstLine(b.text), 40))
		}
		t.Fatalf("parsed %d blocks, want %d", len(got), len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("block %d: kind %d, want %d", i, got[i], want[i])
		}
	}
}

func TestParseCollapsesParagraphsButNotRawBlocks(t *testing.T) {
	blocks := parseSample(t)

	// A paragraph is re-wrapped to the reading measure, so its source line
	// breaks must be gone.
	p := blocks[1]
	if strings.Contains(p.text, "\n") {
		t.Errorf("paragraph kept a source line break: %q", p.text)
	}
	if !strings.Contains(p.text, "exponential backoff starting") {
		t.Errorf("paragraph lost text across its line break: %q", p.text)
	}

	// A code fence must keep its lines and indentation verbatim — there, the
	// line breaks are the content.
	var code block
	for _, b := range blocks {
		if b.kind == blockRaw && strings.Contains(b.text, "func retry") {
			code = b
		}
	}
	if len(code.lines) < 3 {
		t.Fatalf("code block has %d lines, want its structure preserved: %q", len(code.lines), code.text)
	}
	if !strings.Contains(strings.Join(code.lines, "\n"), "    return nil") {
		t.Errorf("code block lost its indentation: %q", code.lines)
	}
}

func TestParseCapturesHeadingLevels(t *testing.T) {
	blocks := parseSample(t)
	if blocks[0].level != 1 {
		t.Errorf("`# Retry policy` has level %d, want 1", blocks[0].level)
	}
	if blocks[2].level != 2 {
		t.Errorf("`## Budgets` has level %d, want 2", blocks[2].level)
	}
	if blocks[0].text != "Retry policy" {
		t.Errorf("heading text = %q, want the `#` stripped", blocks[0].text)
	}
}

// Offsets are what ID-01 will use to stamp ids back into the source, so they
// have to actually point at the block.
func TestParseOffsetsPointAtTheSource(t *testing.T) {
	src := []byte(sampleDoc)
	for i, b := range parseDoc(src) {
		if b.start < 0 || b.stop > len(src) || b.start >= b.stop {
			t.Fatalf("block %d has a bad range [%d,%d) over %d bytes", i, b.start, b.stop, len(src))
		}
		slice := string(src[b.start:b.stop])
		// The first word of the block must appear in the slice its offsets name.
		first := strings.Fields(b.text)
		if len(first) == 0 {
			continue
		}
		if !strings.Contains(slice, first[0]) {
			t.Errorf("block %d offsets [%d,%d) select %q, which does not contain %q",
				i, b.start, b.stop, truncate(slice, 40), first[0])
		}
	}
}

func TestParseSkipsEmptyAndThematicBreaks(t *testing.T) {
	for _, b := range parseSample(t) {
		if strings.TrimSpace(b.text) == "" {
			t.Errorf("parsed an empty block: %+v", b)
		}
		if strings.TrimSpace(b.text) == "---" {
			t.Error("thematic break became a reviewable block")
		}
	}
}

func TestAnchorsAreStableAndDistinct(t *testing.T) {
	first := parseSample(t)
	second := parseSample(t)

	seen := map[string]int{}
	for i := range first {
		if first[i].anchor != second[i].anchor {
			t.Errorf("block %d anchor is not stable across parses: %s vs %s",
				i, first[i].anchor, second[i].anchor)
		}
		if first[i].kind == blockHeading {
			continue // headings carry an anchor but are not commentable
		}
		if prev, dup := seen[first[i].anchor]; dup {
			t.Errorf("blocks %d and %d share anchor %s", prev, i, first[i].anchor)
		}
		seen[first[i].anchor] = i
	}
}

func TestLoadDoc(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "spec.md")
	if err := os.WriteFile(path, []byte(sampleDoc), 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}

	blocks, err := loadDoc(path)
	if err != nil {
		t.Fatalf("loadDoc: %v", err)
	}
	if len(blocks) == 0 {
		t.Fatal("loadDoc returned no blocks")
	}

	if _, err := loadDoc(filepath.Join(dir, "nope.md")); err == nil {
		t.Error("loadDoc on a missing file returned no error")
	}

	empty := filepath.Join(dir, "empty.md")
	if err := os.WriteFile(empty, []byte("\n\n"), 0o644); err != nil {
		t.Fatalf("write empty: %v", err)
	}
	if _, err := loadDoc(empty); err == nil {
		t.Error("loadDoc on a document with no content returned no error")
	}
}

// A parsed document must work with the whole review model, not just parse — the
// point of the baseline is that a real file behaves like the seeded one.
func TestParsedDocumentDrivesTheModel(t *testing.T) {
	m := newModel(parseDoc([]byte(sampleDoc)), nil)
	m.w, m.h = 100, 60

	lines := m.render()
	if len(lines) == 0 {
		t.Fatal("parsed document rendered nothing")
	}
	if !strings.Contains(strings.Join(lines, "\n"), "Retry policy") {
		t.Error("rendered output does not contain the document's first heading")
	}

	// Focus must reach a commentable block, and a comment must be able to
	// attach to it.
	var target int = -1
	for i, e := range m.entries {
		if e.b.commentable() {
			target = i
			break
		}
	}
	if target < 0 {
		t.Fatal("no commentable block in a parsed document")
	}
	m.at = cursor{entry: target, comment: commentNone}
	if th := m.ensureThread(m.anchorAt()); th == nil {
		t.Fatalf("could not start a thread on a parsed block %q", m.anchorAt())
	}

	// And a mark must apply to it.
	m.toggleMark(markOK)
	if m.marks[m.anchorAt()] != markOK {
		t.Errorf("marking a parsed block did not take: %v", m.marks[m.anchorAt()])
	}
	if done, _, total := m.reviewProgress(); total == 0 || done != 1 {
		t.Errorf("progress = %d/%d, want 1 of a non-zero total", done, total)
	}
}
