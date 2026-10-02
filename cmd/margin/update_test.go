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

// The launcher, end to end: margin-launcher in a directory nobody may write,
// a home with nothing in it. The first launch fetches margin a and runs it;
// margin update, in a margin the launcher started, installs b into the home
// and leaves every file it ran from alone; the next launch runs b.
func TestTheLauncherFetchesMarginAndUpdateMovesIt(t *testing.T) {
	gobin, err := osexec.LookPath("go")
	if err != nil {
		t.Skip("no go toolchain on PATH to build margin with")
	}
	commitA, commitB := testchannel.Commit('a'), testchannel.Commit('b')
	const verA, verB = "26.10.01-dev.aaaaaaa", "26.10.02-dev.bbbbbbb"
	dir := t.TempDir()
	build := func(pkg, name, v, commit string) []byte {
		t.Helper()
		out := filepath.Join(dir, name)
		vp := "github.com/neuroplastio/margin/internal/version"
		cmd := osexec.Command(gobin, "build", "-tags", "updatetest", "-o", out,
			"-ldflags", "-X "+vp+".Version="+v+" -X "+vp+".Commit="+commit+" -X "+vp+".Channel=dev", pkg)
		if b, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("building %s %s: %v\n%s", name, v, err, b)
		}
		data, err := os.ReadFile(out)
		if err != nil {
			t.Fatal(err)
		}
		return data
	}
	older, newer := build(".", "margin-a", verA, commitA), build(".", "margin-b", verB, commitB)
	launcher := build("../margin-launcher", "margin-launcher", verA, commitA)

	ch := testchannel.New(t)
	ch.Publish(t, commitA, verA, older)

	usr := t.TempDir()
	installed := filepath.Join(usr, "margin")
	if err := os.WriteFile(installed, launcher, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(usr, 0o555); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chmod(usr, 0o755) })
	home := t.TempDir()
	mhome := filepath.Join(home, ".local", "margin")
	margin := func(env []string, args ...string) (string, error) {
		t.Helper()
		cmd := osexec.Command(installed, args...)
		cmd.Env = append(os.Environ(), "HOME="+home, "MARGIN_PKG_URL="+ch.URL, "MARGIN_UPDATE_SIGNERS="+ch.Signers())
		cmd.Env = append(cmd.Env, env...)
		out, err := cmd.CombinedOutput()
		return string(out), err
	}

	// ENLAUNCH_FETCH: a seed in this machine's /usr/lib/margin, from a
	// package, must not stand in for the first fetch.
	out, err := margin([]string{"ENLAUNCH_FETCH=1"}, "--version")
	if err != nil || !strings.HasSuffix(out, "margin "+verA+" ("+commitA+")\n") || !strings.Contains(out, "margin: fetching") {
		t.Fatalf("first launch: %v\n%s", err, out)
	}

	ch.Publish(t, commitB, verB, newer)
	out, err = margin(nil, "update")
	if err != nil || !strings.Contains(out, "margin "+verA+" → "+verB) || !strings.Contains(out, "installed in "+mhome) {
		t.Fatalf("margin update under the launcher: %v\n%s", err, out)
	}
	if out, _ := margin(nil, "--version"); out != "margin "+verB+" ("+commitB+")\n" {
		t.Errorf("the next launch: --version = %q", out)
	}
	if data, _ := os.ReadFile(installed); !bytes.Equal(data, launcher) {
		t.Error("the launcher was written")
	}
	if data, _ := os.ReadFile(filepath.Join(mhome, "builds", commitA, "margin")); !bytes.Equal(data, older) {
		t.Error("margin a, which ran the update, was written")
	}
}
