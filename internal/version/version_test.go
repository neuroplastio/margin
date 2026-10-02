package version

import "testing"

const full = "a7072af3c19e4b7d8a2f6c05e1b9d3347f8a6c21"

// stamp sets the linker's variables for one test.
func stamp(t *testing.T, v, commit, channel, dirty string) {
	t.Helper()
	ov, oc, och, om := Version, Commit, Channel, modified
	t.Cleanup(func() { Version, Commit, Channel, modified = ov, oc, och, om })
	Version, Commit, Channel, modified = v, commit, channel, dirty
}

func TestAReleaseBuildIsNamedAndOnItsChannel(t *testing.T) {
	stamp(t, "26.10.02-dev.a7072af", full, "dev", "")
	i := Current()
	if !i.Published() {
		t.Fatalf("a stamped, clean build on dev should be published: %+v", i)
	}
	if got, want := i.String(), "26.10.02-dev.a7072af ("+full+")"; got != want {
		t.Errorf("String() = %q, want %q", got, want)
	}
}

// A local build calls itself by its short commit, and a dirty one says so: its
// commit names code the binary does not contain.
func TestALocalBuildIsItsShortCommit(t *testing.T) {
	stamp(t, "", full, "", "")
	if got := Current().String(); got != "a7072af (local build)" {
		t.Errorf("clean local build: %q", got)
	}
	stamp(t, "", full, "", "true")
	i := Current()
	if got := i.String(); got != "a7072af+dirty (local build)" {
		t.Errorf("dirty local build: %q", got)
	}
	if i.Published() {
		t.Error("a local build is on no channel")
	}
}

// Stamping a channel onto a dirty tree does not make it a published build.
func TestADirtyTreeIsNeverPublished(t *testing.T) {
	stamp(t, "26.10.02-dev.a7072af", full, "dev", "true")
	if Current().Published() {
		t.Error("a dirty tree's build passed for the channel's")
	}
}
