//go:build !updatetest

package update

import (
	"testing"

	"github.com/neuroplastio/margin/internal/update/testchannel"
	"github.com/neuroplastio/margin/internal/version"
)

// Only a binary built with -tags updatetest can be pointed at another key or
// server. This test is compiled the way every release is, without the tag,
// and proves the variables that would move them do nothing there — in
// margin's update and in margin-launcher alike.
func TestTheKeyAndTheServerCannotBeMovedInARealBuild(t *testing.T) {
	t.Setenv("MARGIN_UPDATE_SIGNERS", testchannel.New(t).Signers())
	t.Setenv("MARGIN_PKG_URL", "http://127.0.0.1:1")
	if got := signers(); got != releaseSigners {
		t.Errorf("signers() = %q, want the pinned release key", got)
	}
	if l := Launcher(version.Info{}); l.Base != URL || l.Signers != releaseSigners {
		t.Errorf("the launcher reads %s with %q", l.Base, l.Signers)
	}
	if c, err := Client(version.Info{}); err != nil || c.Base != URL {
		t.Errorf("update reads %v: %v", c, err)
	}
}
