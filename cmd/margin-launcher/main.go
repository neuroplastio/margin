// Command margin-launcher is margin's thin launcher (D20), what a package
// installs as margin. It runs the margin installed in ~/.local/margin, or
// the seed a package ships in /usr/lib/margin (D22), and the first time,
// with neither there, fetches the newest from the channel, checked against
// the release key. It does nothing else: margin update moves what it runs.
//
// It is stamped like the margin it is released with, so
// `margin --launcher-version` names the package's launcher, while
// `margin --version`, answered after the hand-over, names the build running.
package main

import (
	"fmt"
	"os"

	"github.com/neuroplastio/engram/enlaunch"
	"github.com/neuroplastio/margin/internal/update"
	"github.com/neuroplastio/margin/internal/version"
)

func main() {
	// A flag margin itself never takes, so it shadows nothing of margin's.
	if len(os.Args) == 2 && os.Args[1] == "--launcher-version" {
		fmt.Printf("margin-launcher %s\n", version.Current())
		return
	}
	enlaunch.Main(update.Launcher(version.Current()))
}
