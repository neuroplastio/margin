// Package version is what this margin binary knows about itself: the build
// metadata the linker stamps in (the Makefile, and the release workflow
// through it), D18.
//
// A build has a name and an identity, and they are different things. Version
// is the name, for people, never compared or sorted: `YY.MM.DD` for a release
// on the stable channel (its tag, D21), `YY.MM.DD-dev.<sha7>` for a build of
// main published to the dev channel (the date is the commit's, in UTC), and
// the short commit for a local build. Commit is the identity: the
// full commit the binary was built from. Channel is where it was published,
// and empty for a binary nobody published — every local build.
package version

import (
	"fmt"
	"runtime/debug"
)

// Stamped by -ldflags -X. Empty when unstamped.
var (
	Version = ""
	Commit  = ""
	Channel = ""
	// modified is "true" when the tree had uncommitted changes: the commit then
	// names code this binary does not contain. Unexported, so only the linker
	// sets it (-X reaches an unexported string var).
	modified = ""
)

// Info is one build's metadata.
type Info struct {
	Version  string
	Commit   string // full commit; empty when unknown
	Channel  string // empty for a build on no channel
	Modified bool
}

// Current is this binary. An unstamped binary (`go build`, `go install` from a
// checkout) falls back to the commit Go itself recorded, so it still names the
// code it holds.
func Current() Info {
	i := Info{Version: Version, Commit: Commit, Channel: Channel, Modified: modified == "true"}
	if i.Commit == "" {
		if bi, ok := debug.ReadBuildInfo(); ok {
			for _, s := range bi.Settings {
				switch s.Key {
				case "vcs.revision":
					i.Commit = s.Value
				case "vcs.modified":
					i.Modified = s.Value == "true"
				}
			}
		}
	}
	if i.Version == "" {
		i.Version = i.Short()
	}
	return i
}

// Short is the first seven characters of the commit, with +dirty for a
// modified tree; "dev" when the commit is unknown.
func (i Info) Short() string {
	if i.Commit == "" {
		return "dev"
	}
	s := ShortCommit(i.Commit)
	if i.Modified {
		s += "+dirty"
	}
	return s
}

// Published reports whether a channel holds exactly this build: stamped with a
// channel, from a clean tree, at a full commit.
func (i Info) Published() bool {
	return i.Channel != "" && !i.Modified && len(i.Commit) == 40
}

// String is what `margin --version` prints after `margin `.
func (i Info) String() string {
	if i.Published() {
		return fmt.Sprintf("%s (%s)", i.Version, i.Commit)
	}
	return i.Version + " (local build)"
}

// ShortCommit is any commit as a person reads it: its first seven characters.
func ShortCommit(commit string) string {
	if len(commit) > 7 {
		return commit[:7]
	}
	return commit
}
