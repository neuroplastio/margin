package review

import (
	"bytes"
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

// TestStampIDRoundTrip is ID-01's core guarantee: stamping a block, then
// reparsing, gives that block back the same id — and no other block. This is
// the property everything else (re-attachment, persistence) will build on.
func TestStampIDRoundTrip(t *testing.T) {
	before := parseSample(t)

	for i, target := range before {
		src := []byte(sampleDoc)
		id := newBlockID()
		stamped := stampID(src, target, id)

		after := parseDoc(stamped)
		if len(after) != len(before) {
			t.Fatalf("block %d: stamping changed the block count: %d vs %d", i, len(after), len(before))
		}
		if after[i].anchor != id {
			t.Errorf("block %d: anchor = %q after stamping, want %q", i, after[i].anchor, id)
		}
		if !after[i].stamped {
			t.Errorf("block %d: stamped id was not flagged as stamped", i)
		}
		if after[i].text != before[i].text {
			t.Errorf("block %d: text changed by stamping: %q vs %q", i, after[i].text, before[i].text)
		}
		if after[i].kind != before[i].kind {
			t.Errorf("block %d: kind changed by stamping: %v vs %v", i, after[i].kind, before[i].kind)
		}
		for j, ob := range after {
			if j == i {
				continue
			}
			if ob.stamped {
				t.Errorf("block %d picked up a stamp meant for block %d", j, i)
			}
		}

		// Reparsing again — as a fresh margin session opening the file a second
		// time would — must find the same id still attached, not a new one.
		again := parseDoc(stamped)
		if again[i].anchor != id {
			t.Errorf("block %d: anchor drifted on a second reparse: %q vs %q", i, again[i].anchor, id)
		}
	}
}

// TestStampIDInvisibleToOtherRenderers checks the marker reads as an ordinary
// markdown comment: it must not become a block of its own, and it must not
// leak into the text or lines of the block it is attached to, including a
// fenced code block where a byte in the wrong place corrupts the fence.
func TestStampIDInvisibleToOtherRenderers(t *testing.T) {
	src := []byte(sampleDoc)
	before := parseDoc(src)

	var code block
	for _, b := range before {
		if b.kind == blockRaw && strings.Contains(b.text, "func retry") {
			code = b
		}
	}
	if code.text == "" {
		t.Fatal("fixture has no code block to stamp")
	}

	id := newBlockID()
	stamped := stampID(src, code, id)
	if !strings.Contains(string(stamped), "<!--margin:"+id+"-->") {
		t.Fatalf("stamped source does not contain the marker verbatim")
	}

	after := parseDoc(stamped)
	if len(after) != len(before) {
		t.Fatalf("marker became its own block: %d blocks, want %d", len(after), len(before))
	}
	for i, b := range after {
		if strings.Contains(b.text, "margin:") {
			t.Errorf("block %d text leaked the marker: %q", i, b.text)
		}
		for _, l := range b.lines {
			if strings.Contains(l, "margin:") {
				t.Errorf("block %d lines leaked the marker: %q", i, l)
			}
		}
	}

	// The fence itself must still be intact: three lines of code, not the
	// marker spliced in as a fourth line of "code".
	var gotCode block
	for _, b := range after {
		if b.kind == blockRaw && strings.Contains(b.text, "func retry") {
			gotCode = b
		}
	}
	if len(gotCode.lines) != len(code.lines) {
		t.Errorf("code block gained/lost lines after stamping: %v vs %v", gotCode.lines, code.lines)
	}
}

// TestStampIDDoesNotDisturbEarlierBlocks confirms the documented guarantee on
// stampID: bytes before the stamped block's own range are untouched, so a
// caller stamping blocks one at a time, earliest first, never has to
// re-locate a block it already has offsets for.
func TestStampIDDoesNotDisturbEarlierBlocks(t *testing.T) {
	src := []byte(sampleDoc)
	blocks := parseDoc(src)

	// Stamp the last block; everything before its start must be byte-for-byte
	// unchanged.
	last := blocks[len(blocks)-1]
	stamped := stampID(src, last, newBlockID())
	if !bytes.Equal(stamped[:last.start], src[:last.start]) {
		t.Error("stamping the last block changed bytes before it")
	}
}

func TestNewBlockIDIsDistinctAndWellFormed(t *testing.T) {
	seen := map[string]bool{}
	for i := 0; i < 100; i++ {
		id := newBlockID()
		if !idMarkerRE.MatchString("<!--margin:" + id + "-->") {
			t.Fatalf("id %q does not round-trip through the marker pattern", id)
		}
		if seen[id] {
			t.Fatalf("newBlockID produced a duplicate: %s", id)
		}
		seen[id] = true
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

	// A thread must attach to any commentable block, headings included.
	find := func(pred func(block) bool, what string) int {
		for i, e := range m.entries {
			if e.thread == nil && pred(e.b) {
				return i
			}
		}
		t.Fatalf("no %s block in a parsed document", what)
		return -1
	}

	m.at = cursor{entry: find(block.commentable, "commentable"), comment: commentNone}
	if th := m.ensureThread(m.anchorAt()); th == nil {
		t.Fatalf("could not start a thread on a parsed block %q", m.anchorAt())
	}

	// A mark applies to a markable block — headings roll up their section
	// instead of carrying one, so they are deliberately not a target here.
	m.at = cursor{entry: find(block.markable, "markable"), comment: commentNone}
	m.toggleMark(markOK)
	if m.marks[m.anchorAt()] != markOK {
		t.Errorf("marking a parsed block did not take: %v", m.marks[m.anchorAt()])
	}
	if done, _, total := m.reviewProgress(); total == 0 || done != 1 {
		t.Errorf("progress = %d/%d, want 1 of a non-zero total", done, total)
	}
}

// TestStampEveryBlockSequentially covers the property ID-02 will actually rely
// on, which the single-stamp tests do not: stamping *every* block of a document
// one after another, rather than one block of a pristine source.
//
// stampID only guarantees that bytes before the stamped block are undisturbed,
// so a caller must work backwards through the document — otherwise each
// insertion invalidates the offsets of every block after it.
func TestStampEveryBlockSequentially(t *testing.T) {
	orig := parseSample(t)
	cur := []byte(sampleDoc)
	want := map[int]string{}

	for i := len(orig) - 1; i >= 0; i-- {
		blocks := parseDoc(cur)
		if len(blocks) != len(orig) {
			t.Fatalf("stamping block %d: block count drifted to %d, want %d", i, len(blocks), len(orig))
		}
		id := newBlockID()
		want[i] = id
		cur = stampID(cur, blocks[i], id)
	}

	final := parseDoc(cur)
	if len(final) != len(orig) {
		t.Fatalf("after stamping every block: %d blocks, want %d", len(final), len(orig))
	}
	for i := range orig {
		if final[i].text != orig[i].text {
			t.Errorf("block %d text changed:\n  was %q\n  now %q", i, orig[i].text, final[i].text)
		}
		if final[i].kind != orig[i].kind {
			t.Errorf("block %d kind changed: %v -> %v", i, orig[i].kind, final[i].kind)
		}
		if final[i].anchor != want[i] {
			t.Errorf("block %d anchor = %q, want %q", i, final[i].anchor, want[i])
		}
		if !final[i].stamped {
			t.Errorf("block %d is not flagged as stamped", i)
		}
	}
	if n := strings.Count(string(cur), "<!--margin:"); n != len(orig) {
		t.Errorf("%d markers in the source, stamped %d blocks", n, len(orig))
	}
}
