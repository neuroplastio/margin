package main

import (
	"bytes"
	"errors"
	"strings"
	"testing"
)

// exec runs the root command with args and a stub runner, capturing its output.
func exec(t *testing.T, run func(string) error, args ...string) (string, error) {
	t.Helper()
	if run == nil {
		run = func(string) error { return nil }
	}
	var out bytes.Buffer
	root := newRootCmd(run)
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetArgs(args)
	err := root.Execute()
	return out.String(), err
}

func TestRunsWithOneFile(t *testing.T) {
	var got string
	if _, err := exec(t, func(p string) error { got = p; return nil }, "spec.md"); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if got != "spec.md" {
		t.Errorf("runner got %q, want spec.md", got)
	}
}

func TestRejectsWrongNumberOfArguments(t *testing.T) {
	for _, args := range [][]string{{}, {"a.md", "b.md"}} {
		called := false
		out, err := exec(t, func(string) error { called = true; return nil }, args...)
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
	out, err := exec(t, func(string) error {
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
	if _, err := exec(t, func(string) error { called = true; return nil }, "--nope", "spec.md"); err == nil {
		t.Error("an unknown flag was accepted")
	}
	if called {
		t.Error("an unknown flag reached the runner")
	}
}
