package review

import (
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
