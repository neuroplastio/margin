package main

import (
	"bytes"
	"os"
	osexec "os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/neuroplastio/margin/internal/update/testchannel"
)

// install.sh, end to end (D22): a release of margin and margin-launcher on a
// test channel, installed by the script into an empty home. The launcher goes
// in the bin directory, margin is seeded in ~/.local/margin, and the installed
// margin runs with the channel's server out of reach. A second install keeps
// the home's build unless forced, and a build whose binary does not match its
// signed manifest installs nothing.
func TestInstallScriptDropsTheLauncherAndSeedsTheHome(t *testing.T) {
	gobin, err := osexec.LookPath("go")
	if err != nil {
		t.Skip("no go toolchain on PATH to build margin with")
	}
	for _, tool := range []string{"sh", "curl", "ssh-keygen"} {
		if _, err := osexec.LookPath(tool); err != nil {
			t.Skipf("no %s on PATH to run install.sh with", tool)
		}
	}
	script, err := filepath.Abs("../../install.sh")
	if err != nil {
		t.Fatal(err)
	}
	commitA, commitB, commitC := testchannel.Commit('a'), testchannel.Commit('b'), testchannel.Commit('c')
	const verA, verB, verC = "26.10.01", "26.10.02", "26.10.03"
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
	marginA, marginB := build(".", "margin-a", verA, commitA), build(".", "margin-b", verB, commitB)
	launcher := build("../margin-launcher", "margin-launcher", verA, commitA)

	ch := testchannel.New(t)
	ch.PublishAll(t, commitA, verA, map[string][]byte{"margin": marginA, "margin-launcher": launcher})

	home := t.TempDir()
	bindir := filepath.Join(home, "bin")
	mhome := filepath.Join(home, ".local", "margin")
	install := func(env ...string) (string, error) {
		t.Helper()
		cmd := osexec.Command("sh", script)
		cmd.Env = append(os.Environ(), "HOME="+home, "MARGIN_BINDIR="+bindir,
			"MARGIN_URL="+ch.URL, "MARGIN_CHANNEL=dev", "MARGIN_SIGNERS="+ch.Signers(),
			"MARGIN_COMMIT=", "MARGIN_FORCE=")
		cmd.Env = append(cmd.Env, env...)
		out, err := cmd.CombinedOutput()
		return string(out), err
	}
	same := func(path string, want []byte) bool {
		got, err := os.ReadFile(path)
		return err == nil && bytes.Equal(got, want)
	}

	out, err := install()
	if err != nil || !strings.Contains(out, "installed margin "+verA+" ("+commitA+"), verified") {
		t.Fatalf("install: %v\n%s", err, out)
	}
	if !same(filepath.Join(bindir, "margin"), launcher) {
		t.Error("the bin directory's margin is not the launcher")
	}
	if !same(filepath.Join(mhome, "bin", "margin"), marginA) {
		t.Error("the home's bin/margin is not the seed")
	}

	// The seed runs with no channel to reach: the server is a closed port.
	run := func(args ...string) (string, error) {
		t.Helper()
		cmd := osexec.Command(filepath.Join(bindir, "margin"), args...)
		cmd.Env = append(os.Environ(), "HOME="+home, "MARGIN_PKG_URL=http://127.0.0.1:1", "MARGIN_UPDATE_SIGNERS="+ch.Signers())
		out, err := cmd.CombinedOutput()
		return string(out), err
	}
	if out, err := run("--version"); err != nil || out != "margin "+verA+" ("+commitA+")\n" {
		t.Errorf("the installed margin offline: %v\n%s", err, out)
	}
	if out, err := run("--launcher-version"); err != nil || out != "margin-launcher "+verA+" ("+commitA+")\n" {
		t.Errorf("--launcher-version: %v\n%s", err, out)
	}

	// A home that holds a build keeps it; MARGIN_FORCE replaces it.
	ch.PublishAll(t, commitB, verB, map[string][]byte{"margin": marginB, "margin-launcher": launcher})
	if out, err := install(); err != nil || !strings.Contains(out, "kept the build already in") {
		t.Fatalf("a second install: %v\n%s", err, out)
	}
	if !same(filepath.Join(mhome, "bin", "margin"), marginA) {
		t.Error("a second install replaced the home's build")
	}
	if out, err := install("MARGIN_FORCE=1"); err != nil || !same(filepath.Join(mhome, "bin", "margin"), marginB) {
		t.Fatalf("a forced install: %v\n%s", err, out)
	}

	// margin altered after signing: one byte, so only the sha256 can tell.
	ch.PublishAll(t, commitC, verC, map[string][]byte{"margin": marginA, "margin-launcher": launcher})
	tampered := bytes.Clone(marginA)
	tampered[len(tampered)/2] ^= 0xff
	if err := os.WriteFile(ch.Artifact(commitC), tampered, 0o644); err != nil {
		t.Fatal(err)
	}
	if out, err := install("MARGIN_FORCE=1"); err == nil || !strings.Contains(out, "does not match the signed manifest") {
		t.Errorf("a tampered build: %v\n%s", err, out)
	}
	if !same(filepath.Join(mhome, "bin", "margin"), marginB) {
		t.Error("a tampered build was installed")
	}

	// The script's own pinned key, the release key, which never signed this
	// channel.
	if out, err := install("MARGIN_SIGNERS="); err == nil || !strings.Contains(out, "not signed by the pinned key") {
		t.Errorf("the release key: %v\n%s", err, out)
	}
}
