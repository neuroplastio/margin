package main

import (
	"bytes"
	"errors"
	"strings"
	"testing"

	"github.com/neuroplastio/margin/internal/review"
)

// exec runs the root command with args and stub handlers, capturing its output.
func exec(t *testing.T, run func(string, review.RunOptions) error, args ...string) (string, error) {
	t.Helper()
	if run == nil {
		run = func(string, review.RunOptions) error { return nil }
	}
	var out bytes.Buffer
	root := newRootCmd(run, nil, nil, nil)
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetArgs(args)
	err := root.Execute()
	return out.String(), err
}

func TestRunsWithOneFile(t *testing.T) {
	var got string
	if _, err := exec(t, func(p string, _ review.RunOptions) error { got = p; return nil }, "spec.md"); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if got != "spec.md" {
		t.Errorf("runner got %q, want spec.md", got)
	}
}

func TestRejectsWrongNumberOfArguments(t *testing.T) {
	for _, args := range [][]string{{}, {"a.md", "b.md"}} {
		called := false
		out, err := exec(t, func(string, review.RunOptions) error { called = true; return nil }, args...)
		if err == nil {
			t.Errorf("%v was accepted; margin takes exactly one file", args)
		}
		if called {
			t.Errorf("%v reached the runner", args)
		}
		if !strings.Contains(out, "Usage:") {
			t.Errorf("%v did not show usage:\n%s", args, out)
		}
	}
}

// A missing file is a runtime failure, not a misuse of the command. Dumping the
// full help text on top of it buries the actual message.
func TestRuntimeErrorDoesNotPrintUsage(t *testing.T) {
	out, err := exec(t, func(string, review.RunOptions) error {
		return errors.New("open nope.md: no such file or directory")
	}, "nope.md")

	if err == nil {
		t.Fatal("a failing run reported success")
	}
	if !strings.Contains(out, "no such file or directory") {
		t.Errorf("the actual error is missing:\n%s", out)
	}
	if strings.Contains(out, "Usage:") {
		t.Errorf("runtime error printed the usage block:\n%s", out)
	}
}

func TestVersionFlag(t *testing.T) {
	out, err := exec(t, nil, "--version")
	if err != nil {
		t.Fatalf("--version: %v", err)
	}
	if !strings.HasPrefix(out, "margin ") {
		t.Errorf("version output = %q, want it to name the tool", out)
	}
}

func TestHelpListsTheKeys(t *testing.T) {
	out, err := exec(t, nil, "--help")
	if err != nil {
		t.Fatalf("--help: %v", err)
	}
	// The help is the only place a new user learns the keys, so the ones that
	// are not guessable have to be in it.
	for _, key := range []string{"space", "Y", "esc esc", "ctrl+enter"} {
		if !strings.Contains(out, key) {
			t.Errorf("help does not mention %q:\n%s", key, out)
		}
	}
}

func TestUnknownFlagIsRejected(t *testing.T) {
	called := false
	if _, err := exec(t, func(string, review.RunOptions) error { called = true; return nil }, "--nope", "spec.md"); err == nil {
		t.Error("an unknown flag was accepted")
	}
	if called {
		t.Error("an unknown flag reached the runner")
	}
}

// --- --stdout ----------------------------------------------------------------

func TestStdoutFlagReachesTheRunner(t *testing.T) {
	var got review.RunOptions
	if _, err := exec(t, func(_ string, opts review.RunOptions) error { got = opts; return nil }, "--stdout", "spec.md"); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !got.Stdout {
		t.Error("--stdout did not set RunOptions.Stdout")
	}
}

func TestWithoutStdoutFlagDefaultsFalse(t *testing.T) {
	var got review.RunOptions
	got.Stdout = true // prove the zero value, not a leftover
	if _, err := exec(t, func(_ string, opts review.RunOptions) error { got = opts; return nil }, "spec.md"); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if got.Stdout {
		t.Error("RunOptions.Stdout is set without --stdout")
	}
}

func TestHelpMentionsStdout(t *testing.T) {
	out, err := exec(t, nil, "--help")
	if err != nil {
		t.Fatalf("--help: %v", err)
	}
	if !strings.Contains(out, "--stdout") {
		t.Errorf("help does not mention --stdout:\n%s", out)
	}
}

// --- --stdin ------------------------------------------------------------------

func TestStdinFlagReachesTheRunner(t *testing.T) {
	var got review.RunOptions
	if _, err := exec(t, func(_ string, opts review.RunOptions) error { got = opts; return nil }, "--stdin"); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !got.Stdin {
		t.Error("--stdin did not set RunOptions.Stdin")
	}
	if !got.Stdout {
		t.Error("--stdin did not imply RunOptions.Stdout")
	}
}

func TestDashMeansStdin(t *testing.T) {
	var gotPath string
	var got review.RunOptions
	if _, err := exec(t, func(p string, opts review.RunOptions) error { gotPath, got = p, opts; return nil }, "-"); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if gotPath != "-" {
		t.Errorf("runner got path %q, want -", gotPath)
	}
	if !got.Stdin || !got.Stdout {
		t.Errorf(`"margin -" gave RunOptions{Stdin: %v, Stdout: %v}, want both set`, got.Stdin, got.Stdout)
	}
}

func TestStdinRejectsAFileArgument(t *testing.T) {
	called := false
	out, err := exec(t, func(string, review.RunOptions) error { called = true; return nil }, "--stdin", "spec.md")
	if err == nil {
		t.Error("--stdin with a file argument was accepted")
	}
	if called {
		t.Error("--stdin with a file argument reached the runner")
	}
	if !strings.Contains(out, "Usage:") {
		t.Errorf("misuse did not show usage:\n%s", out)
	}
}

func TestWithoutStdinFlagDefaultsFalse(t *testing.T) {
	var got review.RunOptions
	got.Stdin = true // prove the zero value, not a leftover
	if _, err := exec(t, func(_ string, opts review.RunOptions) error { got = opts; return nil }, "spec.md"); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if got.Stdin {
		t.Error("RunOptions.Stdin is set without --stdin or -")
	}
}

func TestHelpMentionsStdin(t *testing.T) {
	out, err := exec(t, nil, "--help")
	if err != nil {
		t.Fatalf("--help: %v", err)
	}
	if !strings.Contains(out, "--stdin") {
		t.Errorf("help does not mention --stdin:\n%s", out)
	}
}

// --- --include-resolved -------------------------------------------------------

func TestIncludeResolvedFlagReachesTheRunner(t *testing.T) {
	var got review.RunOptions
	if _, err := exec(t, func(_ string, opts review.RunOptions) error { got = opts; return nil }, "--include-resolved", "spec.md"); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !got.IncludeResolved {
		t.Error("--include-resolved did not set RunOptions.IncludeResolved")
	}
}

func TestWithoutIncludeResolvedFlagDefaultsFalse(t *testing.T) {
	var got review.RunOptions
	got.IncludeResolved = true // prove the zero value, not a leftover
	if _, err := exec(t, func(_ string, opts review.RunOptions) error { got = opts; return nil }, "spec.md"); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if got.IncludeResolved {
		t.Error("RunOptions.IncludeResolved is set without --include-resolved")
	}
}

func TestHelpMentionsIncludeResolved(t *testing.T) {
	out, err := exec(t, nil, "--help")
	if err != nil {
		t.Fatalf("--help: %v", err)
	}
	if !strings.Contains(out, "--include-resolved") {
		t.Errorf("help does not mention --include-resolved:\n%s", out)
	}
}

// --- --wheel-speed -----------------------------------------------------------

func TestWheelSpeedFlagReachesTheRunner(t *testing.T) {
	var got review.RunOptions
	if _, err := exec(t, func(_ string, opts review.RunOptions) error { got = opts; return nil }, "--wheel-speed", "5", "spec.md"); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if got.WheelSpeed != 5 {
		t.Errorf("--wheel-speed 5 gave RunOptions.WheelSpeed = %d, want 5", got.WheelSpeed)
	}
}

func TestWithoutWheelSpeedFlagDefaultsZero(t *testing.T) {
	var got review.RunOptions
	got.WheelSpeed = 7 // prove the zero value, not a leftover
	if _, err := exec(t, func(_ string, opts review.RunOptions) error { got = opts; return nil }, "spec.md"); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if got.WheelSpeed != 0 {
		t.Errorf("RunOptions.WheelSpeed = %d without --wheel-speed, want 0 (the default is the model's job)", got.WheelSpeed)
	}
}

func TestHelpMentionsWheelSpeed(t *testing.T) {
	out, err := exec(t, nil, "--help")
	if err != nil {
		t.Fatalf("--help: %v", err)
	}
	if !strings.Contains(out, "--wheel-speed") {
		t.Errorf("help does not mention --wheel-speed:\n%s", out)
	}
}

// --- comment add -------------------------------------------------------------

// execComment runs `comment add` with a stub AddComment handler, capturing the
// arguments it received and the output.
func execComment(t *testing.T, args []string) (path, anchor, author, text string, out string, err error) {
	t.Helper()
	var gotP, gotA, gotAuthor, gotT string
	addComment := func(p, a, au, tx string) (string, error) {
		gotP, gotA, gotAuthor, gotT = p, a, au, tx
		return "/root/.margin/threads/doc.md/x.md", nil
	}
	var buf bytes.Buffer
	root := newRootCmd(nil, addComment, nil, nil)
	root.SetOut(&buf)
	root.SetErr(&buf)
	root.SetArgs(args)
	err = root.Execute()
	return gotP, gotA, gotAuthor, gotT, buf.String(), err
}

func TestCommentAddReachesTheHandler(t *testing.T) {
	path, anchor, author, text, _, err := execComment(t,
		[]string{"comment", "add", "spec.md", "--anchor", "^abc123", "--text", "reply text", "--author", "agent"})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if path != "spec.md" || anchor != "^abc123" || author != "agent" || text != "reply text" {
		t.Errorf("handler got (%q, %q, %q, %q), want (spec.md, ^abc123, agent, reply text)", path, anchor, author, text)
	}
}

func TestCommentAddDefaultsAuthorToCurrentUser(t *testing.T) {
	_, _, author, _, _, err := execComment(t,
		[]string{"comment", "add", "spec.md", "--anchor", "^abc", "--text", "no author given"})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if author == "" {
		t.Error("author is empty without --author, want a default")
	}
}

func TestCommentAddRequiresAnchor(t *testing.T) {
	_, _, _, _, out, err := execComment(t, []string{"comment", "add", "spec.md", "--text", "no anchor"})
	if err == nil {
		t.Fatal("comment add without --anchor reported success")
	}
	if !strings.Contains(out, "Usage:") {
		t.Errorf("misuse did not show usage:\n%s", out)
	}
}

func TestCommentAddRequiresText(t *testing.T) {
	_, _, _, _, out, err := execComment(t, []string{"comment", "add", "spec.md", "--anchor", "^abc"})
	if err == nil {
		t.Fatal("comment add without --text reported success")
	}
	if !strings.Contains(out, "Usage:") {
		t.Errorf("misuse did not show usage:\n%s", out)
	}
}

func TestCommentAddRequiresAFile(t *testing.T) {
	_, _, _, _, out, err := execComment(t, []string{"comment", "add", "--anchor", "^abc", "--text", "hi"})
	if err == nil {
		t.Fatal("comment add without a file reported success")
	}
	if !strings.Contains(out, "Usage:") {
		t.Errorf("misuse did not show usage:\n%s", out)
	}
}

func TestCommentAddPrintsWhereItLanded(t *testing.T) {
	_, _, _, _, out, err := execComment(t,
		[]string{"comment", "add", "spec.md", "--anchor", "^abc", "--text", "hi"})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !strings.Contains(out, "/root/.margin/threads/doc.md/x.md") {
		t.Errorf("output does not name the thread file:\n%s", out)
	}
}

func TestCommentAddSurfacesRuntimeError(t *testing.T) {
	addComment := func(string, string, string, string) (string, error) {
		return "", errors.New("no commentable block with anchor ^abc in spec.md")
	}
	var buf bytes.Buffer
	root := newRootCmd(nil, addComment, nil, nil)
	root.SetOut(&buf)
	root.SetErr(&buf)
	root.SetArgs([]string{"comment", "add", "spec.md", "--anchor", "^abc", "--text", "hi"})
	err := root.Execute()
	if err == nil {
		t.Fatal("a failing add reported success")
	}
	if !strings.Contains(buf.String(), "no commentable block") {
		t.Errorf("the actual error is missing:\n%s", buf.String())
	}
	if strings.Contains(buf.String(), "Usage:") {
		t.Errorf("runtime error printed the usage block:\n%s", buf.String())
	}
}

func TestHelpMentionsCommentAdd(t *testing.T) {
	out, err := exec(t, nil, "--help")
	if err != nil {
		t.Fatalf("--help: %v", err)
	}
	if !strings.Contains(out, "comment") {
		t.Errorf("help does not mention the comment subcommand:\n%s", out)
	}
}

// --- export ------------------------------------------------------------------

// execExport runs `export` with a stub Export handler, capturing the arguments
// it received and the output.
func execExport(t *testing.T, args []string) (path string, includeResolved bool, out string, err error) {
	t.Helper()
	var gotP string
	var gotInc bool
	exportReview := func(p string, inc bool) (string, error) {
		gotP, gotInc = p, inc
		return "# Review of " + p + "\n", nil
	}
	var buf bytes.Buffer
	root := newRootCmd(nil, nil, exportReview, nil)
	root.SetOut(&buf)
	root.SetErr(&buf)
	root.SetArgs(args)
	err = root.Execute()
	return gotP, gotInc, buf.String(), err
}

func TestExportReachesTheHandler(t *testing.T) {
	path, inc, out, err := execExport(t, []string{"export", "spec.md"})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if path != "spec.md" || inc {
		t.Errorf("handler got (path=%q, includeResolved=%v), want (spec.md, false)", path, inc)
	}
	if !strings.Contains(out, "# Review of spec.md") {
		t.Errorf("output does not carry the export:\n%s", out)
	}
}

func TestExportIncludeResolvedFlagReachesTheHandler(t *testing.T) {
	_, inc, _, err := execExport(t, []string{"export", "spec.md", "--include-resolved"})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !inc {
		t.Error("--include-resolved did not reach the handler")
	}
}

func TestExportRequiresAFile(t *testing.T) {
	_, _, out, err := execExport(t, []string{"export"})
	if err == nil {
		t.Fatal("export without a file reported success")
	}
	if !strings.Contains(out, "Usage:") {
		t.Errorf("misuse did not show usage:\n%s", out)
	}
}

func TestExportSurfacesRuntimeError(t *testing.T) {
	exportReview := func(string, bool) (string, error) {
		return "", errors.New("open nope.md: no such file or directory")
	}
	var buf bytes.Buffer
	root := newRootCmd(nil, nil, exportReview, nil)
	root.SetOut(&buf)
	root.SetErr(&buf)
	root.SetArgs([]string{"export", "nope.md"})
	err := root.Execute()
	if err == nil {
		t.Fatal("a failing export reported success")
	}
	if !strings.Contains(buf.String(), "no such file or directory") {
		t.Errorf("the actual error is missing:\n%s", buf.String())
	}
	if strings.Contains(buf.String(), "Usage:") {
		t.Errorf("runtime error printed the usage block:\n%s", buf.String())
	}
}

func TestHelpMentionsExport(t *testing.T) {
	out, err := exec(t, nil, "--help")
	if err != nil {
		t.Fatalf("--help: %v", err)
	}
	if !strings.Contains(out, "export") {
		t.Errorf("help does not mention the export subcommand:\n%s", out)
	}
}
