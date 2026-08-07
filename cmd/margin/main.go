// Command margin is a terminal tool for reviewing markdown: read the rendered
// document, leave anchored comments, mark blocks reviewed, and hand the result
// back to whatever wrote it.
package main

import (
	"os"

	"github.com/neuroplastio/margin/internal/review"
	"github.com/spf13/cobra"
)

// version is stamped at build time; see the Makefile.
var version = "dev"

func main() {
	if err := newRootCmd(review.Run).Execute(); err != nil {
		os.Exit(1)
	}
}

// newRootCmd builds the margin command tree. It takes the runner as an argument
// and is a function rather than a package var so tests can construct an isolated
// command with its own I/O, args, and a runner that does not open a terminal.
func newRootCmd(run func(path string, opts review.RunOptions) error) *cobra.Command {
	var stdout bool

	root := &cobra.Command{
		Use:     "margin FILE.md",
		Short:   "Review markdown in the terminal",
		Version: version,
		Long: `margin opens a markdown file for review: read the rendered prose, leave
comments anchored to blocks, mark what you have reviewed and what still needs
attention, then copy the whole review out for whatever wrote the document.

Keys:
  j/k     move                c  comment            space  cycle the mark
  g/G     top/bottom          e  edit               r/f    reviewed / flag
  Y       copy the review     q  quit

On a heading, the mark keys apply to the whole section.

The comment composer is a real nvim, so every key inside it belongs to nvim:
  ctrl+s / ctrl+enter / :wq   submit
  esc esc / :q                close, keeping a draft
  :q!                         discard

--stdout runs the review as usual, but writes it to stdout on quit instead of
requiring Y — the same content Y produces — so it can be piped straight into
an agent:

  margin --stdout FILE.md | agent -p "address this review"`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			// Silenced here rather than on the command, so the two kinds of
			// failure read differently: getting the invocation wrong still
			// prints usage, while a file that does not exist is a runtime
			// error and should not bury its message under the help text.
			cmd.SilenceUsage = true
			return run(args[0], review.RunOptions{Stdout: stdout})
		},
	}
	root.Flags().BoolVar(&stdout, "stdout", false, "write the review to stdout on quit, instead of requiring Y")
	root.SetVersionTemplate("margin {{.Version}}\n")
	return root
}
