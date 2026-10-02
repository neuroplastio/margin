//go:build updatetest

package update

import "os"

// EnvSigners and EnvURL replace the pinned release key and server with the
// caller's — only in a binary built with -tags updatetest. That is how a test
// proves a real margin binary, and margin-launcher, update from a channel
// served by a test server and signed by a throwaway key. No release carries
// this file: the Makefile and the release workflow never set the tag, and a
// test without it proves the variables are ignored
// (TestTheKeyAndTheServerCannotBeMovedInARealBuild).
const (
	EnvSigners = "MARGIN_UPDATE_SIGNERS"
	EnvURL     = "MARGIN_PKG_URL"
)

func init() {
	signers = func() string {
		if v := os.Getenv(EnvSigners); v != "" {
			return v
		}
		return releaseSigners
	}
	pkgURL = func() string {
		if v := os.Getenv(EnvURL); v != "" {
			return v
		}
		return URL
	}
}
