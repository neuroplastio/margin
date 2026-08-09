package review

import (
	"errors"
	"io"
	"os"
	"strings"
	"testing"
)

// --- --stdin / ephemeral review --------------------------------------------
//
// The two things the feedback asked for are provable without a screen: the
// document comes off the pipe (not a file), and the session reaches for the
// controlling terminal — because stdout is implied and stdin is the document
// — rather than touching either standard stream for the interface. What a
// test cannot prove is that nothing is written to .margin/threads; that half
// is structural (the store stays nil, and save on a nil store is a pinned
// no-op), not behavioural.

func TestLoadDocFromParsesPipedMarkdown(t *testing.T) {
	blocks, err := loadDocFrom(strings.NewReader(sampleDoc), "stdin")
	if err != nil {
		t.Fatalf("loadDocFrom: %v", err)
	}
	if len(blocks) == 0 {
		t.Fatal("loadDocFrom returned no blocks for a real document")
	}
}

func TestLoadDocFromNamesTheLabelInItsError(t *testing.T) {
	_, err := loadDocFrom(strings.NewReader("\n\n"), "stdin")
	if err == nil {
		t.Fatal("loadDocFrom on empty input returned no error")
	}
	if !strings.Contains(err.Error(), "stdin") {
		t.Errorf("error does not name the label, so a piped failure would read as a file error: %v", err)
	}
}

// TestStdinRunSurfacesTTYOpenFailure pins both halves of "--stdin implies
// --stdout" at once: a stdin review reaches for the controlling terminal
// (because the interface can use neither stdin nor stdout), and a failure to
// reach it is reported rather than silently falling back to drawing over the
// pipe the review is about to be printed on.
func TestStdinRunSurfacesTTYOpenFailure(t *testing.T) {
	restoreStdin := stdinReader
	stdinReader = strings.NewReader(sampleDoc)
	defer func() { stdinReader = restoreStdin }()

	wantErr := errors.New("no controlling terminal")
	restoreTTY := openTTY
	openTTY = func() (io.ReadWriteCloser, error) { return nil, wantErr }
	defer func() { openTTY = restoreTTY }()

	err := Run("", RunOptions{Stdin: true})
	if err == nil {
		t.Fatal("Run succeeded despite openTTY failing")
	}
	if !errors.Is(err, wantErr) {
		t.Errorf("Run's error does not wrap the openTTY failure: %v", err)
	}
}

// TestStdinRunRejectsATerminalOnStdin pins the guard against `margin -` typed
// with no pipe: reading the keyboard as the document would swallow keystrokes
// until EOF and then review whatever was typed. /dev/null stands in for a
// terminal here — it is a character device, same as a tty for Stat's purposes.
func TestStdinRunRejectsATerminalOnStdin(t *testing.T) {
	f, err := os.Open("/dev/null")
	if err != nil {
		t.Skipf("/dev/null unavailable: %v", err)
	}
	defer f.Close()

	restoreStdin := stdinReader
	stdinReader = f
	defer func() { stdinReader = restoreStdin }()

	err = Run("", RunOptions{Stdin: true})
	if err == nil {
		t.Fatal("Run accepted a character device as the document")
	}
	if !strings.Contains(err.Error(), "piped") {
		t.Errorf("error does not say what --stdin expects: %v", err)
	}
}

// TestEphemeralExportPointsAtNoThreadFiles: a --stdin review writes nothing,
// so the export's agent instructions must not send the agent editing files
// under .margin/threads that nothing will ever read back.
func TestEphemeralExportPointsAtNoThreadFiles(t *testing.T) {
	path, doc, threads, marks := exportFixture()
	out := exportReview(path, doc, threads, marks, false, true)
	if strings.Contains(out, ".margin/threads") {
		t.Errorf("ephemeral export still points the agent at thread files:\n%s", out)
	}
	if !strings.Contains(out, "piped") {
		t.Errorf("ephemeral export does not say what it is:\n%s", out)
	}
	if ansiEscape.MatchString(out) {
		t.Errorf("ephemeral export carries ANSI escapes, which --stdout pipes verbatim:\n%q", out)
	}
}

// TestPersistentExportStillPointsAtThreadFiles guards the other side: the
// normal file-backed export keeps its thread-file instructions verbatim.
func TestPersistentExportStillPointsAtThreadFiles(t *testing.T) {
	path, doc, threads, marks := exportFixture()
	out := exportReview(path, doc, threads, marks, false, false)
	if !strings.Contains(out, ".margin/threads/"+path) {
		t.Errorf("file-backed export lost its thread-file instructions:\n%s", out)
	}
}
