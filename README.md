# margin

A terminal tool for reviewing markdown — the kind agents produce by the
thousand-line.

Read the rendered document, leave comments anchored to blocks, mark what you've
reviewed and what still needs attention, and hand the whole lot back to whatever
wrote it.

> **Status: early.** `margin FILE.md` opens a real document and the review loop
> works end to end. Thread persistence is not built yet, so comments live only
> for the session, and anchors are content-derived rather than stamped into the
> source — meaning a thread does not yet survive an agent rewording its block.
> See [Roadmap](#roadmap).

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
where you left off, mid-sentence. Blur and `esc esc` are the same code path, so
there is only ever one way for work to persist.

**Comments anchor to blocks, not lines.** Line numbers do not survive an agent
rewriting a document. Blocks carry a stable id, so a paragraph can be completely
reworded and keep its thread — and when a block is deleted, its thread is
detectably orphaned rather than silently misplaced.

## Keys

| Key | Does |
| --- | --- |
| `j` / `k` | Move between blocks, and between comments in a focused thread |
| `c` | New comment on the focused block (opens in insert mode) |
| `e` | Edit the focused comment or draft (opens in normal mode) |
| `r` | Mark reviewed — on a heading, the whole section |
| `f` | Flag for later — on a heading, the whole section |
| `q` | Quit |

Inside the composer, every key belongs to nvim. Dismissal is nvim's too:

| Key | Does |
| --- | --- |
| `ctrl+s` / `ZZ` / `:wq` / `SPC c c` | Submit |
| `esc esc` / `:q` / `SPC c d` | Close, keeping a draft |
| `:q!` / `SPC c k` | Discard |

`ctrl+\` is the only key the host ever intercepts, and only so a wedged child
can't trap you.

## Install

```
go install github.com/neuroplastio/margin/cmd/margin@latest
```

## Build

```
margin FILE.md
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

`MDREVIEW_NVIM_CONFIG=user` runs the composer with your own nvim config instead
of the stripped one, for comparison.

## Roadmap

- [x] nvim-in-a-pane composer, with blur/resume drafts
- [x] Block-anchored threads, replies, in-place editing
- [x] Review marks with section roll-up
- [x] Real markdown parsing (goldmark, keeping source offsets per block)
- [x] Load a real file from the command line
- [ ] Render headings by level, and inline markup
- [ ] Stamp block ids into the source, so threads survive a rewrite
- [ ] Thread persistence as markdown under `.margin/`, readable and writable by
      an agent with no tooling
- [ ] `--stdout` export, to pipe a review straight into an agent
- [ ] Link navigation between blocks, with a jumplist
- [ ] Rendered diff between review rounds

## License

Apache 2.0
