# Pull `--stdout` forward

Priority: high
Kind: feature
Board: EXPORT-03 (already exists — this promotes it, do not file a duplicate)

`margin --stdout FILE.md` should write the review to stdout, so a review can be
piped straight at an agent:

```bash
margin --stdout spec.md | claude -p "address this review"
```

The clipboard works, but it needs a human in the middle of every loop. This is
the thing that makes the loop scriptable.

## Two things to get right

**1. The TUI must not render to stdout.**

Bubble Tea defaults its output to `os.Stdout` (`tea.go:618`). If stdout is
carrying the review, the pipe fills with escape sequences and the interface has
nowhere usable to draw. `tea.WithOutput` exists for exactly this: when `--stdout`
is set, point the program at `/dev/tty` and keep stdout clean for the review
alone.

Worth a test that asserts the piped output contains no ANSI — this is the kind of
thing that looks fine on a terminal and only breaks once someone actually pipes it.

**2. What it emits, given there is no persistence yet.**

Threads live only for the session until STORE-01 lands, so `--stdout` cannot mean
"print the review already on disk" — there isn't one. It means: run the review
normally, and **on quit, write the review to stdout** instead of requiring `Y`.
Same content `Y` produces, same code path.

That is useful today and stays correct after STORE-01, when a `--stdout` with no
interactive step could additionally print a stored review without opening the
interface at all. Do not build that second mode now.

An empty review should still print its body — the "No comments and nothing
flagged" text, exactly as `Y` gives it — rather than printing nothing. A silent
empty pipe is indistinguishable from a crash.
