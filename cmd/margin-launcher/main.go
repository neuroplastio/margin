// Command margin-launcher is margin's thin launcher (D20), what a package
// installs as margin. It runs the margin installed in ~/.local/margin, and
// the first time, with none there, fetches the newest from the channel,
// checked against the release key. It does nothing else: margin update moves
// what it runs.
package main

import (
	"github.com/neuroplastio/engram/enlaunch"
	"github.com/neuroplastio/margin/internal/update"
	"github.com/neuroplastio/margin/internal/version"
)

func main() {
	enlaunch.Main(update.Launcher(version.Current()))
}
