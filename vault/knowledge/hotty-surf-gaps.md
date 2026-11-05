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

Open question for leg 1, not a gap yet: whether a `surfrun.Surface` can be
hosted inside a program that places its own surfaces with hottytea's
`Session` (margin's shape), rather than owning the terminal.
