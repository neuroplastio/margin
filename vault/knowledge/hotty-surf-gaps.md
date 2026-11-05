# What margin needs that hotty-surf lacks

Found while planning the port (2026-11-05), read against hotty-surf's
`origin/main` vocabulary (docs/surf.md, vocab.go). Each is told to gov, who
routes it to hotty Agent or Design Agent. Until one is filled, margin keeps
its current piece for that spot.

| # | Need | Why margin needs it | margin keeps meanwhile |
| --- | --- | --- | --- |
| G1 | **A multi-row text field** (textarea): wrapping, several rows, caret and selection, a row-count that grows to a cap, paste | the composer is where a reviewer writes comments; losing words is the worst bug (D7) | hotty-a2ui's text field |
| G2 | **A tree with folding**: nested rows, an open/closed state, keys to fold and unfold, fold-all, progress marks on a row | the tree pane of a directory of documents | hotty-a2ui's `HottyTree` |
| G3 | **Markdown in a text**: a comment's inline code, emphasis, links and fenced code, inside a `line wrap` | comments and replies are Markdown | hotty-a2ui's Markdown text |
| G4 | **A prose surface**: a block of HTML from a Markdown renderer (headings, lists, quotes, tables, inline code) in the reading font, and an image/SVG block, measured by the host | the document column (D17, D24) is the product itself | margin's own `hotdoc` surfaces |
| G5 | **A table** of columns that align across rows, with a selection | the threads view lists thread, document, time, author | hotty-a2ui's `HottyTable` |

| # | Need | Why margin needs it | margin keeps meanwhile |
| --- | --- | --- | --- |
| G6 (landed: `surfrun.Paint`, hotty-surf 17045a9; margin's copy is gone) | **A cells screen painted as ANSI lines**, exported: `cells.Screen` → the terminal's colours (surfrun's unexported `cellsSGR`, `ansiFg`) | a program that places its own cells (margin's View is lines of ANSI) paints a surf surface in the terminal | (gone) |
| G7 | **A help line that is cut from its end:** a `row` whose children keep their order from the left and the last are left out where there is no room (`fit` runs right to left, flush right: the palette's footer shape) | the footer's keys read in order, the least important last, and a narrow pane gives them up from the end | a plain `row grow`; in a very narrow pane its keys are cut by the row, untested below 897 px |

Answered: a `surfrun.Surface` is hosted beside margin's own surfaces
(leg 1, journal 2026-11-05.2).

Found in leg 4 (the palette): G5 shows as a palette with no header row and a
fixed-width id column (the longest id in view and two cells); a list of
items has no columns that align by themselves, and no scrollbar.
