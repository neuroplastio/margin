# Q-0001 — Is the unit of review a file, or a directory tree?

Status: open
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
