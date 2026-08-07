package review

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"testing"
)

// --- --stdout ------------------------------------------------------------
//
// Run() itself opens a real interactive session, so it is not exercised
// end to end here (nothing in this package is — see loadDoc's TestLoadDoc
// neighbours for the same boundary). What is tested is the two things the
// feedback that introduced --stdout called out explicitly: the printed
// review carries no ANSI, and a failure to reach the controlling terminal
// is reported rather than silently falling back to drawing over the pipe.

// ansiEscape matches any CSI/OSC-style escape sequence. exportReview never
// touches lipgloss — it is built with fmt/strings only — so this ought to
// never match; the test pins that rather than trusting the absence by
// inspection, since this string is exactly what --stdout pipes to whatever
// reads it.
var ansiEscape = regexp.MustCompile("\x1b\\[[0-9;]*[a-zA-Z]")

func TestExportReviewContainsNoANSI(t *testing.T) {
	path, doc, threads, marks := exportFixture()
	out := exportReview(path, doc, threads, marks, false)
	if ansiEscape.MatchString(out) {
		t.Errorf("exportReview output carries ANSI escapes, which --stdout pipes verbatim:\n%q", out)
	}
}

func TestExportReviewOfAnEmptyReviewContainsNoANSI(t *testing.T) {
	doc, _ := seedDoc()
	out := exportReview("spec.md", doc, map[string]*thread{}, map[string]reviewMark{}, false)
	if ansiEscape.MatchString(out) {
		t.Errorf("empty exportReview output carries ANSI escapes:\n%q", out)
	}
}

// TestStdoutRunSurfacesTTYOpenFailure pins that Run does not fall through to
// drawing the interface on stdout — where the review is being written —
// when the controlling terminal cannot be opened (e.g. margin run with
// stdout piped and no tty at all, such as under a CI job or another pipe
// stage). If this regresses, Run instead reaches tea.NewProgram and blocks
// on stdin, which is the failure mode this guards against.
func TestStdoutRunSurfacesTTYOpenFailure(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "spec.md")
	if err := os.WriteFile(path, []byte(sampleDoc), 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}

	wantErr := errors.New("no controlling terminal")
	restore := openTTY
	openTTY = func() (io.WriteCloser, error) { return nil, wantErr }
	defer func() { openTTY = restore }()

	err := Run(path, RunOptions{Stdout: true})
	if err == nil {
		t.Fatal("Run succeeded despite openTTY failing")
	}
	if !errors.Is(err, wantErr) {
		t.Errorf("Run's error does not wrap the openTTY failure: %v", err)
	}
}
