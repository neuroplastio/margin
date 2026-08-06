# margin

A terminal tool for reviewing markdown — the kind agents produce by the
thousand-line.

Read the rendered document, leave comments anchored to blocks, mark what you've
reviewed and what still needs attention, and hand the whole lot back to whatever
wrote it.

> **Status: early.** The interaction model works end to end against a seeded
> document. The markdown parser, real file loading, and thread persistence are
> not built yet. See [Roadmap](#roadmap).

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

## Build

```
make check     # build + test + vet
make run
```

Requires Go 1.24+ and `nvim` on `PATH`. `MDREVIEW_NVIM_CONFIG=user` runs the
composer with your own config instead of the stripped one, for comparison.

## Roadmap

- [x] nvim-in-a-pane composer, with blur/resume drafts
- [x] Block-anchored threads, replies, in-place editing
- [x] Review marks with section roll-up
- [ ] Real markdown parsing (goldmark, keeping source offsets per block)
- [ ] Load real files and directories; stamp block ids into the source
- [ ] Thread persistence as markdown under `.margin/`, readable and writable by
      an agent with no tooling
- [ ] `--stdout` export, to pipe a review straight into an agent
- [ ] Link navigation between blocks, with a jumplist
- [ ] Rendered diff between review rounds

## License

Apache 2.0
