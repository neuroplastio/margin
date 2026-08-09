# Feature requests: search, line navigation, rich/raw toggle — 2026-08-09

Three navigation features, requested together from the interactive session.
All are felt legs. Order within this file is not priority — pick any unblocked
one when draining, or split as the maintainer prefers.

## 1. `/` search with match highlighting

Use the forward slash key to start a search (vim-style), with matched text
highlighted in the document.

Open questions the leg should settle, not raise: what happens on enter (jump
to next match? close the prompt?), whether `n`/`N` repeat the search, and how
the prompt renders relative to the palette.

## 2. Line-number navigation with `<num>gg`

Jump to a source line number using vim's count-plus-`gg` form, e.g. `42gg`.

Open question: whether the count should also work with `G` (`42G`), and what
"line number" means here — the markdown source line, or the rendered line.

## 3. Rich/raw mode toggle

A toggle action that switches between the rendered document view and the raw
markdown source of the same document.

Open questions the leg should settle: which key toggles it, whether raw mode
reuses the existing blockRaw/verbatim rendering path, and whether focus and
scroll position survive the switch.
