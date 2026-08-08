package review

import (
	"bytes"
	"os"
	"path/filepath"
	"reflect"
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
		blockHeading,  // # Retry policy
		blockPara,     // Each outbound call…
		blockHeading,  // ## Budgets
		blockPara,     // The retry budget…
		blockListItem, // - per-endpoint caps
		blockListItem, // - a global ceiling
		blockCode,     // code fence
		blockQuote,    // block quote
		blockPara,     // Final paragraph.
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
		if b.kind == blockCode && strings.Contains(b.text, "func retry") {
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

// TestParseStripsQuoteMarkers is the parse-level regression test for the
// 2026-08-08 blockquote-rendering feedback: quoteLinesFor must remove the
// `>` (and its one optional space) from every line of a block quote, keeping
// a blank source line as an empty line so a multi-paragraph quote's internal
// breaks survive for wrapQuote to render as paragraph breaks.
func TestParseStripsQuoteMarkers(t *testing.T) {
	src := "> First paragraph,\n> still going.\n>\n> Second paragraph.\n"
	blocks := parseDoc([]byte(src))
	if len(blocks) != 1 || blocks[0].kind != blockQuote {
		t.Fatalf("parsed %d block(s), want 1 blockQuote: %+v", len(blocks), blocks)
	}
	b := blocks[0]
	want := []string{"First paragraph,", "still going.", "", "Second paragraph."}
	if len(b.lines) != len(want) {
		t.Fatalf("quote has %d lines, want %d: %q", len(b.lines), len(want), b.lines)
	}
	for i := range want {
		if b.lines[i] != want[i] {
			t.Errorf("line %d = %q, want %q", i, b.lines[i], want[i])
		}
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

// TestTableExtensionKeepsRowsIntact is the regression test for defect 1 of
// the 2026-08-08 rendering-bugs feedback: with no goldmark extensions
// enabled, a table has no *ast.Table to become — it parses as an
// *ast.Paragraph, and collapse() then joins its rows into one wrapped mess.
// Enabling extension.GFM (see parseDoc) makes it an *ast.Table instead. It
// was blockRaw, shown verbatim, until RENDER-03 gave it its own blockTable
// kind with a real parsed structure — updated here rather than left to rot,
// the same way RENDER-02 retired this test's fenced-code sibling.
func TestTableExtensionKeepsRowsIntact(t *testing.T) {
	src := "| Component | Before |\n| --- | --- |\n| Session read | in-process map |\n"
	blocks := parseDoc([]byte(src))
	if len(blocks) != 1 {
		t.Fatalf("table parsed into %d blocks, want 1: %+v", len(blocks), blocks)
	}
	b := blocks[0]
	if b.kind != blockTable {
		t.Fatalf("table parsed as kind=%d, want blockTable: %q", b.kind, b.text)
	}
	if len(b.lines) != 3 {
		t.Errorf("table has %d verbatim source lines, want its 3 rows kept apart: %q", len(b.lines), b.lines)
	}
	if b.table == nil {
		t.Fatal("blockTable has no parsed table")
	}
	if want := []string{"Component", "Before"}; !reflect.DeepEqual(b.table.header, want) {
		t.Errorf("header = %v, want %v", b.table.header, want)
	}
	if len(b.table.rows) != 1 || !reflect.DeepEqual(b.table.rows[0], []string{"Session read", "in-process map"}) {
		t.Errorf("rows = %v, want [[Session read in-process map]]", b.table.rows)
	}
}

// TestTableCellsStripInlineMarkupAndRespectAlignment pins tableFor's two other
// jobs: a cell's own inline markup is stripped down to plain text (cellText),
// and each column's alignment carries over from the delimiter row.
func TestTableCellsStripInlineMarkupAndRespectAlignment(t *testing.T) {
	src := "| Name | Age | Notes |\n| :--- | ---: | :---: |\n| Alice | 30 | Likes **bold** things |\n"
	blocks := parseDoc([]byte(src))
	if len(blocks) != 1 || blocks[0].kind != blockTable {
		t.Fatalf("parsed %d block(s), want 1 blockTable: %+v", len(blocks), blocks)
	}
	tb := blocks[0].table
	wantAligns := []tableAlign{alignLeft, alignRight, alignCenter}
	if !reflect.DeepEqual(tb.aligns, wantAligns) {
		t.Errorf("aligns = %v, want %v", tb.aligns, wantAligns)
	}
	if len(tb.rows) != 1 || tb.rows[0][2] != "Likes bold things" {
		t.Errorf("row = %v, want the Notes cell markup-stripped to %q", tb.rows, "Likes bold things")
	}
}

// TestParseFencedCodeBlockCarriesLanguageAndStripsFence is RENDER-02's parse-
// level test: a fenced code block must come out as blockCode, keep its
// language for the highlighter, and hand back content lines with the ```
// delimiters gone — the highlighter and quoteBlock both work on the code
// alone, not the fence syntax around it.
func TestParseFencedCodeBlockCarriesLanguageAndStripsFence(t *testing.T) {
	src := "```go\nfunc f() int {\n\treturn 1\n}\n```\n"
	blocks := parseDoc([]byte(src))
	if len(blocks) != 1 || blocks[0].kind != blockCode {
		t.Fatalf("parsed %d block(s), want 1 blockCode: %+v", len(blocks), blocks)
	}
	b := blocks[0]
	if b.lang != "go" {
		t.Errorf("lang = %q, want %q", b.lang, "go")
	}
	want := []string{"func f() int {", "\treturn 1", "}"}
	if len(b.lines) != len(want) {
		t.Fatalf("lines = %q, want %q", b.lines, want)
	}
	for i := range want {
		if b.lines[i] != want[i] {
			t.Errorf("line %d = %q, want %q", i, b.lines[i], want[i])
		}
	}
	for _, l := range b.lines {
		if strings.Contains(l, "```") {
			t.Errorf("fence delimiter leaked into a content line: %q", l)
		}
	}
	// text (used for stampID/anchoring) still carries the full raw markdown,
	// fences included — only lines is fence-stripped.
	if !strings.HasPrefix(b.text, "```go") {
		t.Errorf("text = %q, want the fence kept for anchoring/stamping", b.text)
	}
}

// TestParseFencedCodeBlockWithNoLanguage checks the language tag is simply
// empty, not some placeholder, when the fence names none — highlightCode
// treats "" as "guess from content," not as an error.
func TestParseFencedCodeBlockWithNoLanguage(t *testing.T) {
	blocks := parseDoc([]byte("```\nplain text\n```\n"))
	if len(blocks) != 1 || blocks[0].kind != blockCode {
		t.Fatalf("parsed %d block(s), want 1 blockCode: %+v", len(blocks), blocks)
	}
	if blocks[0].lang != "" {
		t.Errorf("lang = %q, want empty for a fence with no info string", blocks[0].lang)
	}
}

// TestListItemsForSplitsByMarkerAndFoldsContinuations exercises listItemsFor
// directly against the shape defect 2 named: a long item wrapped in the
// source across two lines, followed by a short sibling item.
func TestListItemsForSplitsByMarkerAndFoldsContinuations(t *testing.T) {
	raw := "- **Week 1** — write to both stores, read from memory. Redis is shadow traffic;\n" +
		"  if it falls over, nothing user-visible happens.\n" +
		"- **Week 2** — read from Redis.\n"
	items := listItemsFor(raw)
	if len(items) != 2 {
		t.Fatalf("got %d items, want 2: %+v", len(items), items)
	}
	if items[0].prefix != "- " {
		t.Errorf("item 0 prefix = %q, want %q", items[0].prefix, "- ")
	}
	if !strings.Contains(items[0].text, "shadow traffic") || !strings.Contains(items[0].text, "user-visible happens") {
		t.Errorf("item 0 did not fold its continuation line in: %q", items[0].text)
	}
	if strings.Contains(items[0].text, "\n") {
		t.Errorf("item 0 text kept a line break, want it collapsed like a paragraph: %q", items[0].text)
	}
	if items[1].text != "**Week 2** — read from Redis." {
		t.Errorf("item 1 text = %q", items[1].text)
	}
}

// TestListItemsForKeepsNestedItemsIndentedApart checks that a nested item —
// indented deeper in the source — becomes its own entry with a wider prefix,
// which is what gives it a deeper hanging indent at render time without any
// separate nesting-depth bookkeeping.
func TestListItemsForKeepsNestedItemsIndentedApart(t *testing.T) {
	raw := "- top\n  - nested\n"
	items := listItemsFor(raw)
	if len(items) != 2 {
		t.Fatalf("got %d items, want 2: %+v", len(items), items)
	}
	if items[1].prefix != "  - " {
		t.Errorf("nested item prefix = %q, want %q", items[1].prefix, "  - ")
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
		if target.kind == blockListItem {
			// Every item of a list shares the whole list's byte range (see
			// listItemBlocks), so a marker stamped for one item lands after
			// the last item instead — the single-block round trip this test
			// checks does not hold for these yet. stampAll knows the same
			// thing and never stamps one (parse.go); this is that limit,
			// not a bug in stampID.
			continue
		}
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
		if b.kind == blockCode && strings.Contains(b.text, "func retry") {
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
		if b.kind == blockCode && strings.Contains(b.text, "func retry") {
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

// frontmatterDoc reproduces the 2026-08-07 frontmatter-rendering feedback
// verbatim: left to goldmark alone, the opening "---" is a thematic break
// (dropped) and the closing one turns the YAML above it into a setext
// heading, which then corrupts both the section model and the export.
const frontmatterDoc = `---
name: retry-policy
description: How outbound calls are retried
status: draft
tags: [reliability, networking]
---

# Retry policy

Each outbound call is retried up to three times.
`

func TestParseRecognisesLeadingFrontmatter(t *testing.T) {
	blocks := parseDoc([]byte(frontmatterDoc))

	if len(blocks) == 0 || blocks[0].kind != blockFrontmatter {
		t.Fatalf("first block kind = %v, want blockFrontmatter", blocks[0].kind)
	}
	if !strings.Contains(blocks[0].text, "name: retry-policy") {
		t.Errorf("frontmatter block text = %q, want the YAML body", blocks[0].text)
	}
	if strings.Contains(blocks[0].text, "# Retry policy") {
		t.Errorf("frontmatter block swallowed prose after it: %q", blocks[0].text)
	}

	var got []blockKind
	for _, b := range blocks {
		got = append(got, b.kind)
	}
	want := []blockKind{blockFrontmatter, blockHeading, blockPara}
	if len(got) != len(want) {
		for i, b := range blocks {
			t.Logf("  [%d] kind=%d %q", i, b.kind, truncate(firstLine(b.text), 60))
		}
		t.Fatalf("parsed %d blocks, want %d", len(got), len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("block %d: kind %d, want %d", i, got[i], want[i])
		}
	}

	// The setext trap: the real heading must read as the heading, not as a
	// one-line collapse of the whole frontmatter body.
	if blocks[1].text != "Retry policy" {
		t.Errorf("heading text = %q, want %q", blocks[1].text, "Retry policy")
	}
	if blocks[1].level != 1 {
		t.Errorf("heading level = %d, want 1", blocks[1].level)
	}
}

// TestParseFrontmatterIsNotCommentableOrMarkable is the other half of the
// feedback: frontmatter must not be reviewable, or it inflates the progress
// denominator and pollutes the export's Section: line.
func TestParseFrontmatterIsNotCommentableOrMarkable(t *testing.T) {
	blocks := parseDoc([]byte(frontmatterDoc))
	fm := blocks[0]
	if fm.kind != blockFrontmatter {
		t.Fatalf("fixture bug: block 0 is not frontmatter (kind=%v)", fm.kind)
	}
	if fm.commentable() {
		t.Error("frontmatter block is commentable, want it excluded")
	}
	if fm.markable() {
		t.Error("frontmatter block is markable, want it excluded")
	}
}

// TestParseFrontmatterRequiresLeadingPosition confirms the fix is scoped to a
// document that opens with "---": a "---" used as a horizontal rule elsewhere
// keeps its existing (separately tracked) behaviour rather than being
// swallowed as frontmatter.
func TestParseFrontmatterRequiresLeadingPosition(t *testing.T) {
	src := "# Heading\n\n---\n\nAfter the rule.\n"
	_, _, ok := frontmatterExtent([]byte(src))
	if ok {
		t.Error("frontmatterExtent matched a mid-document horizontal rule")
	}
}

// TestParseUnterminatedFrontmatterIsNotSwallowed checks the case where a
// document opens with "---" but never closes it — parseDoc must not eat the
// rest of the document looking for a fence that is not there.
func TestParseUnterminatedFrontmatterIsNotSwallowed(t *testing.T) {
	src := "---\nnot actually frontmatter\n"
	_, _, ok := frontmatterExtent([]byte(src))
	if ok {
		t.Error("frontmatterExtent matched an unterminated opening fence")
	}
}

// TestParsedFrontmatterDoesNotRenderOrCrash is the render half of the fix:
// how frontmatter should look is an open felt question, so for now it must
// not appear in the rendered document at all, and it must not hit the
// default branch of the render switch — which is built for a thread entry
// and would misbehave given a bare block.
func TestParsedFrontmatterDoesNotRenderOrCrash(t *testing.T) {
	m := newModel(parseDoc([]byte(frontmatterDoc)), nil)
	m.w, m.h = 100, 60

	lines := m.render()
	joined := strings.Join(lines, "\n")
	if strings.Contains(joined, "name: retry-policy") {
		t.Errorf("frontmatter rendered into the document view:\n%s", joined)
	}
	if !strings.Contains(joined, "Retry policy") {
		t.Error("real heading did not render")
	}
	for _, e := range m.entries {
		if e.b.kind == blockFrontmatter {
			t.Error("a frontmatter block reached m.entries; it should be filtered in rebuild")
		}
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
	stamped := 0

	for i := len(orig) - 1; i >= 0; i-- {
		if orig[i].kind == blockListItem {
			// Every item of a list shares the whole list's byte range (see
			// listItemBlocks), so stamping them one at a time in the same
			// pass as everything else does not hold — stampAll excludes
			// them for the same reason (parse.go). Covered on its own by
			// TestStampIDRoundTrip's matching skip.
			continue
		}
		blocks := parseDoc(cur)
		if len(blocks) != len(orig) {
			t.Fatalf("stamping block %d: block count drifted to %d, want %d", i, len(blocks), len(orig))
		}
		id := newBlockID()
		want[i] = id
		cur = stampID(cur, blocks[i], id)
		stamped++
	}

	final := parseDoc(cur)
	if len(final) != len(orig) {
		t.Fatalf("after stamping every block: %d blocks, want %d", len(final), len(orig))
	}
	for i := range orig {
		if orig[i].kind == blockListItem {
			continue
		}
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
	if n := strings.Count(string(cur), "<!--margin:"); n != stamped {
		t.Errorf("%d markers in the source, stamped %d blocks", n, stamped)
	}
}

// TestStampAllStampsOnlyUnstampedBlocks checks the "lazily" half of the
// contract: stampAll must not touch a block that already carries an id, and
// must give every other one a distinct id.
func TestStampAllStampsOnlyUnstampedBlocks(t *testing.T) {
	src := []byte(sampleDoc)
	orig := parseDoc(src)

	// Pre-stamp one block by hand, as if an earlier session had already
	// commented on it.
	pre := orig[1]
	preID := newBlockID()
	src = stampID(src, pre, preID)
	afterPre := parseDoc(src)

	out, blocks := stampAll(src, afterPre)
	if len(blocks) != len(orig) {
		t.Fatalf("stampAll changed the block count: %d, want %d", len(blocks), len(orig))
	}

	seen := map[string]bool{}
	wantMarkers := 0
	for i, b := range blocks {
		if b.kind == blockListItem {
			// stampAll deliberately never stamps these (parse.go) — see
			// TestStampAllNeverStampsListItems.
			continue
		}
		wantMarkers++
		if !b.stamped {
			t.Errorf("block %d was not stamped", i)
		}
		if seen[b.anchor] {
			t.Errorf("block %d reuses id %s", i, b.anchor)
		}
		seen[b.anchor] = true
		if b.text != orig[i].text {
			t.Errorf("block %d text changed: %q vs %q", i, b.text, orig[i].text)
		}
	}
	if blocks[1].anchor != preID {
		t.Errorf("stampAll gave the already-stamped block a new id: %s, want %s (untouched)", blocks[1].anchor, preID)
	}
	if n := strings.Count(string(out), "<!--margin:"); n != wantMarkers {
		t.Errorf("%d markers in the source, want one per stampable block (%d)", n, wantMarkers)
	}
}

// TestStampAllNeverStampsListItems pins the exclusion directly: a list
// item's anchor stays content-derived (unstamped) through stampAll, rather
// than silently corrupting the list the way stamping it would (see
// listItemBlocks' doc comment).
func TestStampAllNeverStampsListItems(t *testing.T) {
	src := []byte(sampleDoc)
	out, blocks := stampAll(src, parseDoc(src))

	var found bool
	for _, b := range blocks {
		if b.kind != blockListItem {
			continue
		}
		found = true
		if b.stamped {
			t.Errorf("list item %q was stamped", b.text)
		}
	}
	if !found {
		t.Fatal("fixture has no list item to check")
	}
	// The list's own raw text must survive completely intact and unsplit —
	// if a marker had landed between its two items (the bug this guards
	// against), this exact substring would no longer appear.
	if !strings.Contains(string(out), "- per-endpoint caps\n- a global ceiling") {
		t.Errorf("stampAll wrote a marker inside the list:\n%s", out)
	}
}

// TestStampAllIsNoopOnceEverythingIsStamped confirms stampAll can be called
// again on an already-fully-stamped document without adding duplicate
// markers or changing any id.
func TestStampAllIsNoopOnceEverythingIsStamped(t *testing.T) {
	src := []byte(sampleDoc)
	stamped, blocks := stampAll(src, parseDoc(src))

	again, blocks2 := stampAll(stamped, blocks)
	if !bytes.Equal(again, stamped) {
		t.Error("stampAll on an already-stamped document changed the source")
	}
	for i := range blocks {
		if blocks2[i].anchor != blocks[i].anchor {
			t.Errorf("block %d anchor changed on a no-op stampAll: %s vs %s", i, blocks2[i].anchor, blocks[i].anchor)
		}
	}
}
