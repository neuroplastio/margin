package main

import (
	"bytes"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/neuroplastio/margin/internal/review"
)

// exec runs the root command with args and stub handlers, capturing its output.
func exec(t *testing.T, run func(string, review.RunOptions) error, args ...string) (string, error) {
	t.Helper()
	if run == nil {
		run = func(string, review.RunOptions) error { return nil }
	}
	var out bytes.Buffer
	root := newRootCmd(run, nil, nil, nil, nil)
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetArgs(args)
	err := root.Execute()
	return out.String(), err
}

// TestRunsWithOneFile: a single path is a single-document review, unchanged
// since before D10.
func TestRunsWithOneFile(t *testing.T) {
	var got string
	if _, err := exec(t, func(p string, _ review.RunOptions) error { got = p; return nil }, "spec.md"); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if got != "spec.md" {
		t.Errorf("runner got %q, want spec.md", got)
	}
}

// TestRunsWithNoArguments: since D10, `margin` with no path opens a tree of
// the working directory — the runner receives the empty path and Run resolves
// it to the current directory.
func TestRunsWithNoArguments(t *testing.T) {
	called := false
	if _, err := exec(t, func(string, review.RunOptions) error { called = true; return nil }); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !called {
		t.Error("no-argument margin did not reach the runner")
	}
}

// TestRejectsWrongNumberOfArguments: margin takes at most one path. Since D10
// (2026-08-07), a directory review is the zero-argument default — `margin`
// and `margin DIR/` open a tree of markdown files — so an empty argv is no
// longer a misuse. Two paths still are.
func TestRejectsWrongNumberOfArguments(t *testing.T) {
	called := false
	if _, err := exec(t, func(string, review.RunOptions) error { called = true; return nil }, "a.md", "b.md"); err == nil {
		t.Error("two files was accepted; margin takes at most one path")
	}
	if called {
		t.Error("two files reached the runner")
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
	for _, key := range []string{"space", "Y", "esc", "ctrl+enter", "ctrl+r"} {
		if !strings.Contains(out, key) {
			t.Errorf("help does not mention %q:\n%s", key, out)
		}
	}
}

// TestHelpNamesTheGutterGlyphs: the 2026-08-10 agent-loop feedback — even
// reading the docs, it was not obvious what a gutter glyph means. The help
// names each glyph, so the legend is one --help away.
func TestHelpNamesTheGutterGlyphs(t *testing.T) {
	out, err := exec(t, nil, "--help")
	if err != nil {
		t.Fatalf("--help: %v", err)
	}
	if !strings.Contains(out, "Gutter") {
		t.Errorf("help has no Gutter legend:\n%s", out)
	}
	for _, glyph := range []string{"▌", "│", "·", "▸", "✓"} {
		if !strings.Contains(out, glyph) {
			t.Errorf("help's gutter legend does not name %q:\n%s", glyph, out)
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
	root := newRootCmd(nil, addComment, nil, nil, nil)
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
	root := newRootCmd(nil, addComment, nil, nil, nil)
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

// --- comments wait ----------------------------------------------------------

// execWait runs `comments wait` with a stub WaitEvents handler, capturing the
// arguments it received and the output.
func execWait(t *testing.T, args []string) (path, since string, timeout time.Duration, out string, err error) {
	t.Helper()
	var gotPath, gotSince string
	var gotTimeout time.Duration
	waitEvents := func(path, since string, timeout time.Duration) ([]string, error) {
		gotPath, gotSince, gotTimeout = path, since, timeout
		if since == "boom" {
			return nil, errors.New("event log: no event with id boom")
		}
		if since == "timeout" {
			return nil, review.ErrWaitTimeout
		}
		return []string{"line one", "line two"}, nil
	}
	var buf bytes.Buffer
	root := newRootCmd(nil, nil, nil, nil, waitEvents)
	root.SetOut(&buf)
	root.SetErr(&buf)
	root.SetArgs(args)
	err = root.Execute()
	return gotPath, gotSince, gotTimeout, buf.String(), err
}

func TestCommentsWaitReachesTheHandler(t *testing.T) {
	path, since, timeout, out, err := execWait(t, []string{"comments", "wait", "--since", "^abc", "--timeout", "30s"})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if path != "." {
		t.Errorf("handler got path %q, want . (cwd root resolution)", path)
	}
	if since != "^abc" {
		t.Errorf("handler got since %q, want ^abc", since)
	}
	if timeout != 30*time.Second {
		t.Errorf("handler got timeout %v, want 30s", timeout)
	}
	if !strings.Contains(out, "line one") || !strings.Contains(out, "line two") {
		t.Errorf("output does not carry the event lines:\n%s", out)
	}
}

func TestCommentsWaitDefaultsSinceAndTimeout(t *testing.T) {
	_, since, timeout, _, err := execWait(t, []string{"comments", "wait"})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if since != "" {
		t.Errorf("since = %q without --since, want empty (every event is new)", since)
	}
	if timeout != 0 {
		t.Errorf("timeout = %v without --timeout, want 0 (wait forever)", timeout)
	}
}

func TestCommentsWaitTimeoutIsQuietExit(t *testing.T) {
	_, _, _, out, err := execWait(t, []string{"comments", "wait", "--since", "timeout"})
	if err == nil {
		t.Fatal("a timeout reported success")
	}
	if out != "" {
		t.Errorf("timeout printed output, want silence:\n%s", out)
	}
	if strings.Contains(out, "Usage:") {
		t.Errorf("timeout printed usage:\n%s", out)
	}
}

func TestCommentsWaitSurfacesRuntimeError(t *testing.T) {
	_, _, _, out, err := execWait(t, []string{"comments", "wait", "--since", "boom"})
	if err == nil {
		t.Fatal("a failing wait reported success")
	}
	if !strings.Contains(out, "no event with id boom") {
		t.Errorf("the actual error is missing:\n%s", out)
	}
	if strings.Contains(out, "Usage:") {
		t.Errorf("runtime error printed the usage block:\n%s", out)
	}
}

func TestCommentsWaitRejectsArgs(t *testing.T) {
	_, _, _, out, err := execWait(t, []string{"comments", "wait", "spec.md"})
	if err == nil {
		t.Fatal("comments wait with a file argument was accepted")
	}
	if !strings.Contains(out, "Usage:") {
		t.Errorf("misuse did not show usage:\n%s", out)
	}
}

func TestHelpMentionsCommentsWait(t *testing.T) {
	out, err := exec(t, nil, "--help")
	if err != nil {
		t.Fatalf("--help: %v", err)
	}
	if !strings.Contains(out, "comments") {
		t.Errorf("help does not mention the comments subcommand:\n%s", out)
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
	root := newRootCmd(nil, nil, exportReview, nil, nil)
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
	root := newRootCmd(nil, nil, exportReview, nil, nil)
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

// --- skill -------------------------------------------------------------------

func TestSkillPrintsTheDocument(t *testing.T) {
	out, err := exec(t, nil, "skill")
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	// The output is the markdown skill document, whole, on stdout.
	if !strings.HasPrefix(out, "# margin") {
		t.Errorf("skill output is not the markdown document:\n%s", out)
	}
	for _, want := range []string{"comments wait", "comment add", "export", "--author agent", "event log"} {
		if !strings.Contains(out, want) {
			t.Errorf("skill does not teach the agent about %q:\n%s", want, out)
		}
	}
}

func TestSkillRejectsArgs(t *testing.T) {
	_, err := exec(t, nil, "skill", "spec.md")
	if err == nil {
		t.Fatal("skill with a file argument was accepted")
	}
}

func TestHelpMentionsSkill(t *testing.T) {
	out, err := exec(t, nil, "--help")
	if err != nil {
		t.Fatalf("--help: %v", err)
	}
	if !strings.Contains(out, "skill") {
		t.Errorf("help does not mention the skill subcommand:\n%s", out)
	}
}
