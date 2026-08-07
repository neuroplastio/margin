# Q-0001 — Is the unit of review a file, or a directory tree?

Status: answered
Blocks: M3, NAV-01, NAV-02
Raised: 2026-08-06

## What I need decided

Does `margin` review one document at a time, or a whole `docs/` tree as a single
review session?

## Why I cannot decide it

It changes the navigation model rather than adding to it: a tree needs a sidebar
or a document picker, a cross-document comment inbox, review progress rolled up
across files, and one export covering the batch. Retrofitting that onto a
single-file model means rewriting focus, hit-testing and the export.

Guessing wrong is expensive in both directions, and it is not something tests can
settle.

## Options

**A. Single file.** `margin spec.md`. Simplest model; everything already built
stays true. Reviewing a batch means running it repeatedly and exporting
repeatedly.

**B. Directory tree.** `margin docs/`. Matches how agents actually emit work — a
plan is rarely one file. Needs document navigation and a combined export from the
start.

**C. Single file now, tree later, but design the export and thread store for a
batch immediately.** Keeps M1 and M2 small while avoiding the expensive part of
the retrofit.

## My lean

**C.** The reviewing surface can stay single-file for M1 and M2 without cost, as
long as thread storage is keyed by file path from the beginning and the export
already speaks in terms of a set of documents rather than one. That defers the
navigation work without building the corner we would have to dig out of.

Worth knowing before answering: agents here emit `docs/` trees, and the earlier
prototype only ever handled one document.

---

## Answer (2026-08-07, maintainer)

**Both.** Not option C — do the tree properly rather than deferring it.

- `margin` with no arguments opens a tree of the working directory.
- `margin DIR/` opens a tree of that directory.
- `margin FILE.md` keeps working exactly as it does today.

There is a **file tree pane**, and it shows **markdown files only** — nothing
else in the directory appears in it.

### Reading of that, to be corrected if wrong

- A directory with no markdown anywhere beneath it should not appear either.
  Showing it would fill the tree with branches that lead nowhere, which is the
  same noise as showing non-markdown files.
- `cobra.ExactArgs(1)` in `cmd/margin` becomes `MaximumNArgs(1)`.
- Thread storage is already keyed by document path under `.margin/threads/`, so
  it needs no change — STORE-01 was built for this.

### Still felt, still unsettled

What the pane looks like, where it sits, how it is toggled and focused, what
review progress looks like across a tree, and whether the export covers one
document or all of them. Those are separate legs and want judging on a screen.

**Unblocks M3.**

