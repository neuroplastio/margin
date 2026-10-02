package main

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	osexec "os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/neuroplastio/margin/internal/update/testchannel"
)

func TestUpdateTakesAnOptionalCommit(t *testing.T) {
	var got []string
	cmd := newUpdateCmd(func(_ context.Context, out io.Writer, commit string) error {
		got = append(got, commit)
		return nil
	})
	for _, args := range [][]string{{}, {"abc1234"}} {
		cmd.SetArgs(args)
		if err := cmd.Execute(); err != nil {
			t.Fatal(err)
		}
	}
	if strings.Join(got, ",") != ",abc1234" {
		t.Errorf("update got commits %q", got)
	}
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"a", "b"})
	if err := cmd.Execute(); err == nil {
		t.Error("two commits were accepted")
	}
	cmd = newUpdateCmd(func(context.Context, io.Writer, string) error { return errors.New("the dev channel holds no build") })
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	out.Reset()
	if err := cmd.Execute(); err == nil || strings.Contains(out.String(), "Usage:") {
		t.Errorf("a failed update: err %v, printed usage:\n%s", err, out.String())
	}
}

// The real thing: two margin binaries built from this tree, an older and a
// newer build of the dev channel; the newer is published to a test channel and
// the older, installed in a directory of its own, is told to update. The file
// it ran from must become the newer build — which says so when asked its
// version — and a tampered build after it must be refused.
//
// The binaries are built with -tags updatetest, the only build in which a key
// other than the release key can be pinned (MARGIN_UPDATE_SIGNERS).
func TestAStaleMarginReplacesItself(t *testing.T) {
	gobin, err := osexec.LookPath("go")
	if err != nil {
		t.Skip("no go toolchain on PATH to build margin with")
	}
	commitA, commitB, commitC := testchannel.Commit('a'), testchannel.Commit('b'), testchannel.Commit('c')
	const verA, verB, verC = "26.10.01-dev.aaaaaaa", "26.10.02-dev.bbbbbbb", "26.10.03-dev.ccccccc"
	dir := t.TempDir()
	build := func(name, v, commit string) []byte {
		t.Helper()
		out := filepath.Join(dir, name)
		pkg := "github.com/neuroplastio/margin/internal/version"
		cmd := osexec.Command(gobin, "build", "-tags", "updatetest", "-o", out,
			"-ldflags", "-X "+pkg+".Version="+v+" -X "+pkg+".Commit="+commit+" -X "+pkg+".Channel=dev", ".")
		if b, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("building margin %s: %v\n%s", v, err, b)
		}
		data, err := os.ReadFile(out)
		if err != nil {
			t.Fatal(err)
		}
		return data
	}
	older, newer := build("margin-a", verA, commitA), build("margin-b", verB, commitB)

	ch := testchannel.New(t)
	ch.Publish(t, commitA, verA, older)
	ch.Publish(t, commitB, verB, newer)

	bin := filepath.Join(t.TempDir(), "margin")
	if err := os.WriteFile(bin, older, 0o755); err != nil {
		t.Fatal(err)
	}
	margin := func(args ...string) (string, error) {
		t.Helper()
		cmd := osexec.Command(bin, args...)
		cmd.Env = append(os.Environ(), "MARGIN_PKG_URL="+ch.URL, "MARGIN_UPDATE_SIGNERS="+ch.Signers())
		out, err := cmd.CombinedOutput()
		return string(out), err
	}

	if out, _ := margin("--version"); out != "margin "+verA+" ("+commitA+")\n" {
		t.Fatalf("before: --version = %q", out)
	}
	out, err := margin("update")
	if err != nil {
		t.Fatalf("margin update: %v\n%s", err, out)
	}
	if !strings.Contains(out, "margin "+verA+" → "+verB) {
		t.Errorf("margin update said:\n%s", out)
	}
	if out, _ := margin("--version"); out != "margin "+verB+" ("+commitB+")\n" {
		t.Errorf("after: --version = %q", out)
	}
	if data, _ := os.ReadFile(bin); !bytes.Equal(data, newer) {
		t.Error("the installed binary is not the published build")
	}
	if out, err := margin("update"); err != nil || !strings.Contains(out, "already the newest") {
		t.Errorf("a second update: %v\n%s", err, out)
	}

	// A newer build whose binary was altered after signing: one byte, so the
	// size still matches and only the sha256 can tell.
	ch.Publish(t, commitC, verC, older)
	tampered := bytes.Clone(older)
	tampered[len(tampered)/2] ^= 0xff
	if err := os.WriteFile(ch.Artifact(commitC), tampered, 0o644); err != nil {
		t.Fatal(err)
	}
	if out, err := margin("update"); err == nil || !strings.Contains(out, "sha256") {
		t.Errorf("a tampered build: %v\n%s", err, out)
	}
	if out, _ := margin("--version"); out != "margin "+verB+" ("+commitB+")\n" {
		t.Errorf("after the refused update: --version = %q", out)
	}
}
