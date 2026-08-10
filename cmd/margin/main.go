// Command margin is a terminal tool for reviewing markdown: read the rendered
// document, leave anchored comments, mark blocks reviewed, and hand the result
// back to whatever wrote it.
package main

import (
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/neuroplastio/margin/internal/review"
	"github.com/spf13/cobra"
)

// version is stamped at build time; see the Makefile.
var version = "dev"

func main() {
	if err := newRootCmd(review.Run, review.AddComment, review.Export, review.DefaultAuthor, review.WaitEvents).Execute(); err != nil {
		os.Exit(1)
	}
}

// newRootCmd builds the margin command tree. It takes the runner and the
// non-interactive command handlers as arguments and is a function rather than a
// package var so tests can construct an isolated command with its own I/O,
// args, and handlers that do not open a terminal or touch disk.
func newRootCmd(run func(path string, opts review.RunOptions) error, addComment func(path, anchor, author, text string) (string, error), exportReview func(path string, includeResolved bool) (string, error), defaultAuthor func() string, waitEvents func(path, since string, timeout time.Duration) ([]string, error)) *cobra.Command {
	var stdout bool
	var includeResolved bool
	var stdin bool
	var wheelSpeed int

	root := &cobra.Command{
		Use:     "margin FILE.md",
		Short:   "Review markdown in the terminal",
		Version: version,
		Long: `margin opens a markdown file for review: read the rendered prose, leave
comments anchored to blocks, mark what you have reviewed and what still needs
attention, then copy the whole review out for whatever wrote the document.

Keys:
  j/k     move            c  comment            space  cycle the mark
  g/G     top/bottom      e  edit               r/f    reviewed / flag
  \\      rendered/raw source Y  copy the review  q  quit
  ctrl+r  reload the document after it changed on disk

Gutter (the column beside each block):
  ▌  cursor / selection here   │  reviewed (green) / flagged (orange)
  ·  partly marked section     ▸  comment thread (l/enter to open)
  ✓  resolved thread

On a heading, the mark keys apply to the whole section.

The comment composer is a real nvim, so every key inside it belongs to nvim:
  ctrl+s / ctrl+enter / :wq   submit
  esc / :q                    close, keeping a draft
  :q!                         discard

--stdout runs the review as usual, but writes it to stdout on quit instead of
requiring Y — the same content Y produces — so it can be piped straight into
an agent:

  margin --stdout FILE.md | agent -p "address this review"

margin export FILE.md is the non-interactive half: it prints the same review
straight away, without opening the interface, for a script or CI pipeline with
no terminal (the review's marks are session-only, so the export reflects the
threads on disk, not which blocks a reviewer had marked).

Or pipe markdown straight in for an ephemeral review — margin - (or
--stdin) reads the document from stdin, saves nothing to .margin/threads,
and prints the review to stdout on quit (so --stdout is implied):

  agent -p "draft a plan" | margin - | agent -p "address this review"

The export leaves out resolved threads by default — it is a list of what
still needs doing, not a transcript of everything ever said. --include-resolved
adds them back, for an agent that wants to see what it already addressed.

The mouse wheel scrolls the document three lines per tick; --wheel-speed sets
a different step size (e.g. --wheel-speed 1 for fine-grained scrolling).

margin skill prints the markdown document an agent loads to learn how to take
part in an interactive review: launch margin in a terminal for the human, poll
for their comments with margin comments wait, reply with margin comment add,
let the thread watcher carry each reply live to their open document, and treat
the launch's completion as the signal that the review is done.`,
		Args: func(cmd *cobra.Command, args []string) error {
			if stdin {
				if len(args) > 0 {
					return fmt.Errorf("--stdin reads the document from standard input; pass no file")
				}
				return nil
			}
			return cobra.ExactArgs(1)(cmd, args)
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			// Silenced here rather than on the command, so the two kinds of
			// failure read differently: getting the invocation wrong still
			// prints usage, while a file that does not exist is a runtime
			// error and should not bury its message under the help text.
			cmd.SilenceUsage = true
			path := ""
			if len(args) > 0 {
				path = args[0]
			}
			// "-" is the conventional spelling of --stdin; both mean the same
			// ephemeral review, and ephemeral implies --stdout.
			ephemeral := stdin || path == "-"
			return run(path, review.RunOptions{
				Stdout:          stdout || ephemeral,
				IncludeResolved: includeResolved,
				Stdin:           ephemeral,
				WheelSpeed:      wheelSpeed,
			})
		},
	}
	root.Flags().BoolVar(&stdout, "stdout", false, "write the review to stdout on quit, instead of requiring Y")
	root.Flags().BoolVar(&includeResolved, "include-resolved", false, "include resolved threads in the export, instead of leaving them out")
	root.Flags().BoolVar(&stdin, "stdin", false, "read the document from stdin for an ephemeral review: nothing is saved, and the review is printed on quit (implies --stdout)")
	root.Flags().IntVar(&wheelSpeed, "wheel-speed", 0, "lines one mouse wheel tick scrolls (default 3)")
	root.SetVersionTemplate("margin {{.Version}}\n")
	root.AddCommand(newCommentCmd(addComment, defaultAuthor))
	root.AddCommand(newExportCmd(exportReview))
	root.AddCommand(newCommentsCmd(waitEvents))
	root.AddCommand(newSkillCmd())
	return root
}

// newCommentCmd builds the `margin comment` subcommand tree: the
// non-interactive half of the review loop. An agent appends a reply here
// without driving the TUI — the same thread file the interface would have
// written — so the reviewer sees it on next open (or live, via the watcher).
func newCommentCmd(addComment func(path, anchor, author, text string) (string, error), defaultAuthor func() string) *cobra.Command {
	var anchor, author, text string

	add := &cobra.Command{
		Use:   "add FILE.md",
		Short: "Append a comment to the thread on a block",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			// Silenced here rather than on the command: getting the invocation
			// wrong still prints usage (cobra's MarkFlagRequired handles the
			// missing --anchor/--text cases), while a missing block or a
			// failing write is a runtime error that should not bury its
			// message under the help text.
			cmd.SilenceUsage = true
			if author == "" {
				if defaultAuthor != nil {
					author = defaultAuthor()
				} else {
					author = "agent"
				}
			}
			path, err := addComment(args[0], anchor, author, text)
			if err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "commented on %s\n", path)
			return nil
		},
	}
	add.Flags().StringVar(&anchor, "anchor", "", "block id to comment on (^abc)")
	add.Flags().StringVar(&text, "text", "", "comment body")
	add.Flags().StringVar(&author, "author", "", "who the comment is from (default: the current user)")
	if err := add.MarkFlagRequired("anchor"); err != nil {
		panic(err)
	}
	if err := add.MarkFlagRequired("text"); err != nil {
		panic(err)
	}

	cmd := &cobra.Command{
		Use:   "comment",
		Short: "Read and write comments without opening the interface",
	}
	cmd.AddCommand(add)
	return cmd
}

// newCommentsCmd builds the `margin comments` subcommand tree: the poll half
// of the interactive review loop. An agent blocks here for the reviewer's next
// comment, instead of re-exporting the review on a timer; see review.WaitEvents
// for the event-log contract this reads.
func newCommentsCmd(waitEvents func(path, since string, timeout time.Duration) ([]string, error)) *cobra.Command {
	var since string
	var timeout time.Duration

	wait := &cobra.Command{
		Use:   "wait",
		Short: "Wait for new review events since an event id",
		Long: `Wait until the review's event log (.margin/events.log) holds a new event —
a comment posted, edited, deleted or restored, a thread resolved or deleted —
after the one --since names, then print each new event's log line to stdout
and exit 0. An agent polls this instead of re-exporting the review on a timer:
the reviewer's comment lands in the log, this command notices it, and the
agent reads the lines to find what to respond to.

The printed lines are the log's own JSONL lines — one JSON object per event,
carrying the id, unix-second timestamp, event type, document, anchor, author,
comment index and, on comment-level events, the comment's body text — in file
order, so the last line's id is the --since cursor for the next call. Two
events in the same second are ordered by file position, not id comparison.

--since is optional: without it, every event in the log is new. --timeout
bounds the wait (0 = wait forever); when it elapses with nothing new the
command exits 1 and prints nothing, which a polling loop treats as "nothing
yet, poll again". A real error — a broken log, an unknown --since id — also
exits 1, but writes the reason to stderr, so the two are distinguishable by
whether stderr is empty.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			// Same runtime-error discipline as the other subcommands: a broken
			// log or a bad --since is a runtime error that should not bury its
			// message under the help text. The timeout is not an error at all:
			// nothing is wrong, so nothing is printed, and the exit code 1
			// alone tells the poller to try again.
			cmd.SilenceUsage = true
			cmd.SilenceErrors = true
			// "." resolves the review root from the cwd, the same walk a file
			// argument gets — the wait command has no document of its own.
			lines, err := waitEvents(".", since, timeout)
			if err != nil {
				if errors.Is(err, review.ErrWaitTimeout) {
					return err
				}
				fmt.Fprintln(cmd.ErrOrStderr(), err)
				return err
			}
			for _, l := range lines {
				fmt.Fprintln(cmd.OutOrStdout(), l)
			}
			return nil
		},
	}
	wait.Flags().StringVar(&since, "since", "", "last event id already seen (default: every event is new)")
	wait.Flags().DurationVar(&timeout, "timeout", 0, "wait at most this long for a new event, then exit 1 (0 = wait forever)")

	cmd := &cobra.Command{
		Use:   "comments",
		Short: "Read and wait on review events without opening the interface",
	}
	cmd.AddCommand(wait)
	return cmd
}

// newExportCmd builds the `margin export` subcommand: the extract half of CLI
// agent automation. It prints the review of a document as it stands on disk —
// the same text Y and --stdout produce — without running the interface, so an
// agent reads the current state of a review in a pipe with no terminal. Marks
// are session-only, so the export reports threads but no reviewed/flagged
// state; see review.Export.
func newExportCmd(exportReview func(path string, includeResolved bool) (string, error)) *cobra.Command {
	var includeResolved bool
	cmd := &cobra.Command{
		Use:   "export FILE.md",
		Short: "Print the review of a document as it stands on disk",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			// Same runtime-error discipline as the other subcommands: a
			// missing file or unreadable thread is a runtime error that
			// should not bury its message under the help text, while getting
			// the invocation wrong still prints usage.
			cmd.SilenceUsage = true
			out, err := exportReview(args[0], includeResolved)
			if err != nil {
				return err
			}
			fmt.Fprint(cmd.OutOrStdout(), out)
			return nil
		},
	}
	cmd.Flags().BoolVar(&includeResolved, "include-resolved", false, "include resolved threads in the export, instead of leaving them out")
	return cmd
}

// newSkillCmd builds the `margin skill` subcommand: the document an agent loads
// to learn how to use margin. It is static content — review.SkillDocument
// embeds the markdown — so there is nothing to wire in and no runtime failure
// to handle; the command exists to print it.
func newSkillCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "skill",
		Short: "Print the skill document an agent loads to use margin",
		Long: `Print the markdown document an agent loads to learn how to use margin:
the four CLI commands, the interactive review loop that binds them — launch the
review in a new terminal for the human, poll for their comments with comments
wait, reply through comment add, and let the thread watcher carry the reply
live to the open document — how a live participant knows when the review is
done, and the on-disk contracts the loop depends on: the event log, thread
files, and anchors.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			fmt.Fprint(cmd.OutOrStdout(), review.SkillDocument())
			return nil
		},
	}
}
