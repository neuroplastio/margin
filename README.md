# margin

A terminal tool for reviewing markdown — the kind agents produce by the
thousand-line.

Read the rendered document, leave comments anchored to blocks, mark what you've
reviewed and what still needs attention, and hand the whole lot back to whatever
wrote it.

> **Status: early.** `margin FILE.md` opens a real document and the review loop
> works end to end. Comments persist as markdown files under
> `.margin/threads/`, and block ids are stamped into the source so a thread
> survives its block being reworded. An agent writing a reply to a thread file
> while margin is already open is not picked up until you reopen the document.
> `margin DIR/` (or `margin` with no argument) reviews a whole tree of markdown
> files, with a file-tree pane. See [Roadmap](#roadmap).

## Why

Reviewing a long markdown file today means one of two bad options:

- **Open it in an editor.** No rendering, so you read wall-to-wall source and
  lose your place constantly. To point at something you either quote it by hand
  or memorise a line number.
- **Open a pull request.** Comments work, but the review view is a diff. There
  is still no rendered prose to read.

Neither has a loop. You end up retyping your own feedback into a chat window.

`margin` is the third option: rendered prose, comments anchored to blocks, and
an export the agent can act on.

## What makes it different

**The composer is a real nvim.** Not a text box that imitates one. Pressing `c`
splices a live `nvim` into the document at the block you're commenting on, with
your motions, your registers, your muscle memory. It's stripped to a textarea —
no statusline, no line numbers, no `~` filler — so all ten rows are your words.

**The editor is disposable.** Losing focus kills the child process, and the text
survives as a draft keyed to the block. Focus it again and nvim respawns exactly
where you left off, mid-sentence. Blur and `esc` are the same code path, so
there is only ever one way for work to persist.

**Comments anchor to blocks, not lines.** Line numbers do not survive an agent
rewriting a document. Blocks carry a stable id, so a paragraph can be completely
reworded and keep its thread — and when a block is deleted, its thread is
detectably orphaned rather than silently misplaced.

## Keys

| Key | Does |
| --- | --- |
| `j` / `k` | Move between blocks and thread rows (a thread is one stop) |
| `\` | Toggle between the rendered document and its raw markdown source |
| `l` / `h` — or `enter` / `esc` | Dive into the focused thread's comments, or a table/raw block's lines / surface back out |
| `H` / `L` | Scroll a wide block (code, table, frontmatter) horizontally — or the whole raw source view, in raw mode |
| `/` | Search: type to highlight matches live, `enter` to jump, `esc` to cancel |
| `n` / `N` | Jump to the next / previous search match |
| `ctrl+]` | Follow the link in the focused block: in-document to the heading it names, or — in a directory review — to the file it points at (`[a](other.md)`, `[b](other.md#section)`) |
| `ctrl+o` / `ctrl+i` | Walk the jumplist back / forward (older / newer jumps) |
| `V` | Select blocks (visual mode): movement extends, `esc` cancels |
| `y` | Yank the selected blocks' markdown to the clipboard |
| `c` | New comment on the focused block (opens in insert mode) |
| `e` | Edit the focused comment or draft (opens in normal mode) |
| `space` | Cycle the mark: unmarked → reviewed → flagged |
| `r` / `f` | Set reviewed / flagged directly |
| `R` | Resolve / unresolve the focused thread |
| `D` | Delete the focused comment or thread (toggle; deleted comments keep their text and `D` restores) — or, on a draft, discard the draft text |
| `ctrl+r` | Reload the document after it changed on disk (a `● file changed` notice appears in the footer) |
| `Y` | Copy the whole review to the clipboard |
| `tab` | Toggle focus between the document and the file-tree pane (directory reviews) |
| `i` | Show the threads view: every thread under review, newest first — the whole tree's in a directory review, the current document's (orphans marked `[block gone]`) in a single-document one — `enter` jumps to the thread's document and block |
| `]` / `[` | Jump to the next / previous comment in the document |
| `q` | Quit |

On a heading, the mark keys apply to the whole section.

The gutter column beside each block says what state it is in: `▌` the cursor is
on this block (or it is part of the current selection); `│` the block is
reviewed (green) or flagged (orange); `·` a section that is only partly
reviewed; `▸` a comment thread lives here (`l`/`enter` opens it); `✓` the
thread is resolved.

`margin --stdout FILE.md` runs the same review, but writes it to stdout on
quit instead of requiring `Y` — the same content `Y` produces — so it can be
piped straight into an agent:

```
margin --stdout spec.md | agent -p "address this review"
```

### Directory reviews

`margin DIR/` — or bare `margin`, which opens the working directory — reviews
a whole tree of markdown files. A file-tree pane sits on the left, listing the
markdown files beneath the directory (hidden directories are tooling and are
skipped, and a directory with no markdown anywhere beneath it does not
appear). `tab` moves keyboard focus between the pane and the document; in the
pane, `j`/`k` move through the files and `enter` (or `l`) opens the focused
one — the document, its threads and its comments switch to the new file, while
each file's review marks are carried per-document across switches. The pane
shows a file's `reviewed/total` beside it once it has any marks (green when the
whole file is reviewed, orange with a `!` when something is flagged), and in a
directory review the footer rolls the whole tree up — `tree N/M reviewed` — so
untouched files count against the total. The pane carries a `▸` on the open
file, and a `▌` on the focused row while the pane is active.

Or pipe markdown straight in for an ephemeral review — `margin -` (or
`--stdin`) reads the document from stdin, saves nothing to `.margin/threads`,
and prints the review to stdout on quit (`--stdout` is implied):

```
agent -p "draft a plan" | margin - | agent -p "address this review"
```

The mouse wheel scrolls the document three lines per tick. `--wheel-speed N`
sets a different step size (`--wheel-speed 1` for fine-grained scrolling).

### Agent automation

An agent can also read and write a review without driving the interface — the
same thread files the reviewer would have written, and the same export the
reviewer's `Y` produces:

```
margin export spec.md            # print the review as it stands on disk
margin comment add spec.md --anchor ^abc123 --text "fixed in rev 3" --author agent
margin comments wait --since 1N7KB52S0NPCH   # block for the next event
```

`margin skill` prints the full picture: the markdown document an agent loads to
learn how to take part in an interactive review — the same four commands, the
loop that binds them (launch the review in a new terminal for the human, poll
for their comments, reply through the CLI, let the thread watcher carry the
reply live), how a live participant knows when the review is done (the launch
job's completion is the signal), and the contracts behind it (the event log,
thread files, anchors).

`margin export FILE.md` is non-interactive: it parses the document, loads its
threads from `.margin/threads/`, and prints the review straight to stdout, so a
script or CI pipeline reads the current state of a review with no terminal.
Marks are session-only, so the export carries threads but not which blocks a
reviewer had marked; `--include-resolved` brings resolved threads back the same
way it does for `Y` and `--stdout`.

`margin comments wait` is the reverse direction — the agent's half of an
interactive review. It reads `.margin/events.log` and blocks until a new event
(a comment posted, edited, deleted or restored; a thread resolved or deleted)
lands after the one `--since` names, then prints each new event's log line and
exits 0. No `--since` means every event is new. `--timeout` bounds the wait (0
= wait forever); a timeout exits 1 with nothing printed — the "nothing yet,
poll again" signal — while a real error (a broken log, an unknown `--since`)
exits 1 and writes the reason to stderr. The printed lines are the log's own
lines, so an agent takes the last line's id as the next `--since`:

```
margin comments wait --since 1N7KB52S0NPCH --timeout 60s
```

`margin import-pr FILE.md` brings a GitHub pull request into the review: it
uses the `gh` CLI to find the PR the current branch is on and imports the PR's
review comments on that file as ordinary margin threads — each attached to the
block its line falls in — so comments already made on the PR read and respond
to inside margin. Re-running is safe (comments already imported are skipped);
comments on other files, or on lines no block covers, are reported, not
dropped. Requires `gh` installed and authenticated, and the document living in
a git checkout.

The anchor is the `(^id)` shown in a block's export header. `--author` defaults
to the current user. The reply appears on the reviewer's next open (or live, via
the file watcher) — and while the review is open it is announced in the status
line (`new comment from agent on ^…`). A document edited on disk mid-review
raises a `● file changed` notice in the footer until `ctrl+r` reloads it, so an
agent's rewrite of the prose is visible without quitting and reopening.

Every thread change — a comment posted, edited, deleted or restored, a thread
resolved or deleted — is also appended to `.margin/events.log` as one JSONL
line: a JSON object carrying a time-ordered 13-character id, a unix-second
timestamp, and the event type, document, anchor, author and comment index.
JSONL makes the fields self-describing, and a listener can tail or watch that
single file to answer "what happened since the last thing I saw", instead of
parsing thread files.

The export leaves resolved threads out by default — it's a list of what still
needs doing, not a transcript of everything ever said. Pass `--include-resolved`
(with `Y` or `--stdout` alike) to get the full history back, including what
was already addressed.

Inside the composer, every key belongs to nvim. Dismissal is nvim's too:

| Key | Does |
| --- | --- |
| `ctrl+s` / `ctrl+enter` / `ZZ` / `:wq` / `SPC c c` | Submit |
| `esc` (`esc esc` for a new comment) / `:q` / `SPC c d` | Close, keeping a draft — unless nothing changed: editing a comment and closing without editing it keeps no draft |
| `:q!` / `SPC c k` | Discard |

`ctrl+\`, `ctrl+enter` and `shift+enter` are the only keys the host intercepts.
`ctrl+enter` submits; `shift+enter` is a line break (terminal-prompt muscle
memory), folded onto the same newline a bare `enter` gets.

## Install

```
go install github.com/neuroplastio/margin/cmd/margin@latest
```

Every push to `main` also builds the artifacts for Linux and macOS (amd64 +
arm64) and uploads them to the run's **Artifacts** list: `margin_<sha>_<os>_<arch>.tar.gz`
per platform plus `checksums.txt`. The archive is just the binary, the README
and the LICENSE — grab the one for your platform, verify it against
`checksums.txt` with `sha256sum -c`, and run `./margin`. This is for machines
without a Go toolchain; the version is the short commit sha (`margin --version`).

## Build

```
margin FILE.md
margin DIR/           # review every markdown file beneath the directory
margin                # same, for the working directory
margin --help
margin --version
```

```
./bin/margin testdata/sample.md    # a realistic document to try it against
./bin/margin testdata/mermaid.md   # a document exercising every mermaid diagram kind
```

```
make check              # build + test + vet
make doctor             # prove this machine can run the composer tests
make run FILE=doc.md
```

Requires Go 1.24+ and `nvim` 0.8+ on `PATH`. `./scripts/setup-env.sh` provisions
a fresh machine; it is idempotent and safe to re-run.

Note that the composer tests **skip** rather than fail when nvim is absent, so a
green suite on an unprovisioned machine is misleading — `make doctor` is what
catches that.

`MARGIN_NVIM_CONFIG=user` runs the composer with your own nvim config instead
of the stripped one, for comparison.

## Roadmap

- [x] nvim-in-a-pane composer, with blur/resume drafts
- [x] Block-anchored threads, replies, in-place editing
- [x] Review marks with section roll-up
- [x] Real markdown parsing (goldmark, keeping source offsets per block)
- [x] Load a real file from the command line
- [x] Export the review to the clipboard
- [ ] Render headings by level, and inline markup
- [x] Stamp block ids into the source, so threads survive a rewrite
- [x] Thread persistence as markdown under `.margin/`, readable and writable by
      an agent with no tooling
- [ ] Live reload — pick up a thread file an agent writes mid-session
- [x] `--stdout` export, to pipe a review straight into an agent
- [x] Ephemeral stdin reviews (`margin -`), pipe in, review, pipe out — nothing
      saved
- [x] Link navigation between blocks, with a jumplist
- [x] Directory reviews (`margin DIR/`): a file-tree pane, tab to switch focus,
      enter to open the focused document
- [ ] Rendered diff between review rounds

## License

Apache 2.0
