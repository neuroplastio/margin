//go:build updatetest

package update

import "os"

// EnvSigners replaces the pinned release key with an allowed_signers line of
// the caller's — only in a binary built with -tags updatetest. That is how a
// test proves a real margin binary updates itself from a channel signed by a
// throwaway key. No release carries this file: the Makefile and the release
// workflow never set the tag, and a test without it proves the variable is
// ignored (TestTheKeyCannotBeMovedInARealBuild).
const EnvSigners = "MARGIN_UPDATE_SIGNERS"

func init() {
	signers = func() string {
		if v := os.Getenv(EnvSigners); v != "" {
			return v
		}
		return releaseSigners
	}
}
