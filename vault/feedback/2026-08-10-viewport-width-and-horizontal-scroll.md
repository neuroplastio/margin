# Viewport width and horizontal scroll

Feature request, 2026-08-10. One theme — width should match what the terminal
can actually show — in four parts.

1. **Adjust the viewport's max width.** On wide viewports it's possible to fit
   more text. Today the document caps at `maxMeasure` (76 columns) whatever the
   terminal's width, via `contentWidth()` (`internal/review/review.go`). On a
   wide terminal it should use the width instead of parking the extra columns.

2. **Wide blocks get a higher maxwidth cap — full terminal width, for now.**
   The blocks that support horizontal scroll — code blocks (journal
   2026-08-09.27) and, once part 3 lands, tables — should be allowed up to the
   full terminal width rather than the prose measure. "Let's try full terminal
   width for now" is the instruction.

3. **Tables should support horizontal scroll.** A table wider than its allowed
   width should keep its natural column widths and scroll horizontally, the way
   a code block does, instead of narrowing columns to fit
   (`tableColumnWidths`).

4. **Horizontal scroll is `H` and `L`, not `h` and `l`.** For consistency.
   `l`/`h` are dive/surface; on a code block (and the frontmatter) they
   currently mean horizontal scroll instead — that double meaning is what we're
   removing. `H`/`L` scroll horizontally on any block that supports it, leaving
   `l`/`h` to mean dive/surface everywhere.
