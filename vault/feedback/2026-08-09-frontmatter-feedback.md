# Frontmatter rendering feedback — 2026-08-09

Two findings from the maintainer reviewing `testdata/sample.md` (the frontmatter
coverage I added for RENDER-07).

## 1. Frontmatter is not reachable by keyboard

Root cause (maintainer's own diagnosis): the frontmatter block is not a focus
stop — `rebuild` skips `blockFrontmatter`, so `j`/`k` never land on it and the
focus-following scroll can never bring it back into view once the reviewer has
scrolled down. It is only visible by scrolling the viewport directly (`J`/`K`).

Expected: the reviewer can reach the frontmatter with the same navigation as
everything else — at minimum `g`/`j` when scrolled back up.

## 2. Truncation should be horizontal scroll, not an ellipsis

RENDER-07 currently truncates a field line wider than the measure with an
ellipsis. The maintainer prefers the code-block treatment instead: fields render
at natural width and `h`/`l` scroll them horizontally inside the block.

Everything else about the sample additions was judged OK: table alignment,
code-block horizontal scroll, level-4 headings, nested list indent, inline
markup.
