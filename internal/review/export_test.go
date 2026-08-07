package review

import (
	"fmt"
	"strings"
	"testing"
	"time"
)

func exportFixture() (string, []block, map[string]*thread, map[string]reviewMark) {
	doc, threads := seedDoc()
	marks := map[string]reviewMark{
		"^a1": markOK,
		"^c3": markFlag, // flagged, and nobody commented on it
	}
	return "spec.md", doc, threads, marks
}

func TestExportNamesTheBlockAndQuotesIt(t *testing.T) {
	out := exportReview(exportFixture())

	if !strings.Contains(out, "# Review of spec.md") {
		t.Errorf("export has no document header:\n%s", out)
	}
	// Every item must name its anchor, so an agent can find the block.
	if !strings.Contains(out, "## ^b2") {
		t.Errorf("export does not name the commented block ^b2:\n%s", out)
	}
	// And quote it, so it is findable even without the id.
	if !strings.Contains(out, "> The retry budget is shared") {
		t.Errorf("export does not quote the block it refers to:\n%s", out)
	}
}

func TestExportCarriesTheWholeExchangeInOrder(t *testing.T) {
	out := exportReview(exportFixture())

	toly := strings.Index(out, "Shouldn't be global")
	agent := strings.Index(out, "Changed to per-endpoint")
	if toly < 0 || agent < 0 {
		t.Fatalf("export dropped part of the exchange:\n%s", out)
	}
	if toly > agent {
		t.Error("export reordered the exchange; replies must follow what they answer")
	}
	if !strings.Contains(out, "**toly:**") || !strings.Contains(out, "**agent:**") {
		t.Errorf("export does not attribute comments:\n%s", out)
	}
}

// A flagged block is feedback even with no comment on it — "this needs
// attention" is the whole message.
func TestExportIncludesFlaggedBlocksWithoutComments(t *testing.T) {
	out := exportReview(exportFixture())
	if !strings.Contains(out, "## ^c3") {
		t.Errorf("flagged block ^c3 is missing from the export:\n%s", out)
	}
	if !strings.Contains(out, "flagged, needs attention") {
		t.Errorf("export does not say why ^c3 is listed:\n%s", out)
	}
}

// A reviewed block with nothing to say should not add noise.
func TestExportSkipsCleanBlocks(t *testing.T) {
	out := exportReview(exportFixture())
	if strings.Contains(out, "## ^a1") {
		t.Errorf("reviewed-and-silent block ^a1 should not appear:\n%s", out)
	}
	if !strings.Contains(out, "1 of 5 blocks reviewed") {
		t.Errorf("export summary is wrong:\n%s", out)
	}
	if !strings.Contains(out, "1 flagged") {
		t.Errorf("export summary omits the flag count:\n%s", out)
	}
}

// Unsubmitted means unsubmitted — but silently dropping work would be worse
// than including it, so the count is reported.
func TestExportExcludesDraftsButCountsThem(t *testing.T) {
	out := exportReview(exportFixture())
	if strings.Contains(out, "This contradicts the sharing claim") {
		t.Errorf("an unsubmitted draft leaked into the export:\n%s", out)
	}
	if !strings.Contains(out, "1 unsubmitted draft") {
		t.Errorf("export does not mention the withheld draft:\n%s", out)
	}
}

func TestExportOfAnUntouchedDocumentSaysSo(t *testing.T) {
	doc, _ := seedDoc()
	out := exportReview("spec.md", doc, map[string]*thread{}, map[string]reviewMark{})
	if !strings.Contains(out, "No comments and nothing flagged") {
		t.Errorf("empty export is not self-explanatory:\n%s", out)
	}
}

func TestExportQuotesRawBlocks(t *testing.T) {
	doc := parseDoc([]byte("Intro paragraph.\n\n```go\nfunc main() {}\n```\n"))
	var code block
	for _, b := range doc {
		if b.kind == blockRaw {
			code = b
		}
	}
	if code.anchor == "" {
		t.Fatal("fixture has no raw block")
	}
	threads := map[string]*thread{code.anchor: {
		anchor: code.anchor,
		posted: []comment{{author: "toly", body: "needs error handling", at: time.Now()}},
	}}
	out := exportReview("spec.md", doc, threads, map[string]reviewMark{})
	if !strings.Contains(out, "func main") {
		t.Errorf("export did not quote the code block it refers to:\n%s", out)
	}
}

// copyToClipboard must not claim success when no helper exists — OSC 52 is the
// caller's job and cannot be verified from here.
func TestCopyToClipboardReportsWhichPath(t *testing.T) {
	t.Setenv("PATH", t.TempDir()) // hide every helper
	via, err := copyToClipboard("hello")
	if err != nil {
		t.Fatalf("copyToClipboard errored with no helper installed: %v", err)
	}
	if via != "" {
		t.Errorf("reported helper %q when none is on PATH", via)
	}
}

// --- locating a block -------------------------------------------------------

// Until ids are stamped into the source (ID-01) an agent cannot search for
// `^0afa31`, so file:line is the locator that actually works.
func TestExportLocatesByFileAndLine(t *testing.T) {
	doc := parseDoc([]byte(sampleDoc))
	var target block
	for _, b := range doc {
		if b.kind == blockPara {
			target = b
			break
		}
	}
	threads := map[string]*thread{target.anchor: {
		anchor: target.anchor,
		posted: []comment{{author: "toly", body: "check this", at: time.Now()}},
	}}
	out := exportReview("spec.md", doc, threads, map[string]reviewMark{})

	want := fmt.Sprintf("## spec.md:%d", target.line)
	if !strings.Contains(out, want) {
		t.Fatalf("export does not locate the block as %q:\n%s", want, out)
	}
}

// A comment on a paragraph should say which section it is in — an agent
// otherwise has only the quote to go on.
func TestExportNamesTheEnclosingSection(t *testing.T) {
	doc := parseDoc([]byte(sampleDoc))
	var target block
	for _, b := range doc {
		if b.kind == blockPara && strings.Contains(b.text, "retry budget") {
			target = b
		}
	}
	threads := map[string]*thread{target.anchor: {
		anchor: target.anchor,
		posted: []comment{{author: "toly", body: "x", at: time.Now()}},
	}}
	out := exportReview("spec.md", doc, threads, map[string]reviewMark{})
	if !strings.Contains(out, "Section: Budgets") {
		t.Errorf("export does not name the enclosing section:\n%s", out)
	}
}

// The defect this replaces: a list was flattened onto one line and cut
// mid-sentence, which is neither readable nor greppable.
func TestExportPreservesListStructure(t *testing.T) {
	doc := parseDoc([]byte(sampleDoc))
	var list block
	for _, b := range doc {
		if b.kind == blockRaw && strings.Contains(b.text, "per-endpoint caps") {
			list = b
		}
	}
	if list.anchor == "" {
		t.Fatal("fixture has no list block")
	}
	threads := map[string]*thread{list.anchor: {
		anchor: list.anchor,
		posted: []comment{{author: "toly", body: "testing this", at: time.Now()}},
	}}
	out := exportReview("spec.md", doc, threads, map[string]reviewMark{})

	if !strings.Contains(out, "> - per-endpoint caps\n> - a global ceiling") {
		t.Errorf("list was flattened instead of quoted line by line:\n%s", out)
	}
}

func TestExportQuotesHeadingsAsMarkdown(t *testing.T) {
	doc := parseDoc([]byte(sampleDoc))
	var h block
	for _, b := range doc {
		if b.kind == blockHeading && b.level == 2 {
			h = b
		}
	}
	threads := map[string]*thread{h.anchor: {
		anchor: h.anchor,
		posted: []comment{{author: "toly", body: "rename this", at: time.Now()}},
	}}
	out := exportReview("spec.md", doc, threads, map[string]reviewMark{})
	if !strings.Contains(out, "> ## Budgets") {
		t.Errorf("heading quote does not show its level:\n%s", out)
	}
	// The heading is its own section; repeating it would be noise.
	if strings.Contains(out, "Section: Budgets") {
		t.Errorf("export repeats a heading as its own section:\n%s", out)
	}
}

func TestExportTruncatesOnWordBoundaries(t *testing.T) {
	long := strings.Repeat("alpha beta gamma delta ", 40)
	doc := parseDoc([]byte(long))
	threads := map[string]*thread{doc[0].anchor: {
		anchor: doc[0].anchor,
		posted: []comment{{author: "toly", body: "x", at: time.Now()}},
	}}
	out := exportReview("spec.md", doc, threads, map[string]reviewMark{})

	for _, line := range strings.Split(out, "\n") {
		if !strings.HasPrefix(line, "> alpha") {
			continue
		}
		if strings.HasSuffix(line, "alp") || strings.HasSuffix(line, "bet") {
			t.Errorf("quote was cut mid-word: %q", line)
		}
	}
	if !strings.Contains(out, "> …") {
		t.Errorf("truncation is not marked:\n%s", out)
	}
}

func TestExportCapsLongRawBlocks(t *testing.T) {
	var src strings.Builder
	src.WriteString("```\n")
	for i := 0; i < 40; i++ {
		fmt.Fprintf(&src, "line %d\n", i)
	}
	src.WriteString("```\n")

	doc := parseDoc([]byte(src.String()))
	threads := map[string]*thread{doc[0].anchor: {
		anchor: doc[0].anchor,
		posted: []comment{{author: "toly", body: "x", at: time.Now()}},
	}}
	out := exportReview("spec.md", doc, threads, map[string]reviewMark{})

	quoted := strings.Count(out, "\n> line ")
	if quoted > maxQuoteLines {
		t.Errorf("quoted %d lines of a long block, want at most %d", quoted, maxQuoteLines)
	}
	if !strings.Contains(out, "> …") {
		t.Errorf("long block truncation is not marked:\n%s", out)
	}
}

// TestExportExcludesFrontmatter is the export half of the frontmatter fix:
// it must not inflate the reviewed-block count, and it must not leak into
// Section: for the blocks that follow it.
func TestExportExcludesFrontmatter(t *testing.T) {
	doc := parseDoc([]byte(frontmatterDoc))

	out := exportReview("spec.md", doc, map[string]*thread{}, map[string]reviewMark{})
	if !strings.Contains(out, "0 of 1 blocks reviewed") {
		t.Errorf("frontmatter counted toward the reviewable total:\n%s", out)
	}

	// Flag the paragraph and confirm its Section: line names the real
	// heading, not the frontmatter.
	marks := map[string]reviewMark{doc[2].anchor: markFlag}
	out = exportReview("spec.md", doc, map[string]*thread{}, marks)
	if !strings.Contains(out, "Section: Retry policy") {
		t.Errorf("flagged block's section is not the real heading:\n%s", out)
	}
	if strings.Contains(out, "name: retry-policy") {
		t.Errorf("frontmatter leaked into the export:\n%s", out)
	}
}
