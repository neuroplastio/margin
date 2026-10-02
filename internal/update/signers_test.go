//go:build !updatetest

package update

import (
	"testing"

	"github.com/neuroplastio/margin/internal/update/testchannel"
)

// Only a binary built with -tags updatetest can be pointed at another key.
// This test is compiled the way every release is, without the tag, and proves
// the variable that would move the key does nothing there.
func TestTheKeyCannotBeMovedInARealBuild(t *testing.T) {
	t.Setenv("MARGIN_UPDATE_SIGNERS", testchannel.New(t).Signers())
	if got := signers(); got != releaseSigners {
		t.Errorf("signers() = %q, want the pinned release key", got)
	}
}
