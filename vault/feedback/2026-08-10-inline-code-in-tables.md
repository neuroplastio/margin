# Inline code (backticks) doesn't render inside table cells

Feedback, 2026-08-10. In a table cell, `` `inline code` `` loses its code
styling and reads as plain text — `| `margin` runs in a TUI |` shows "margin"
as ordinary prose instead of in the monospace/code style it gets everywhere
else in the review.

What I saw: a cell containing backtick-wrapped text renders the words with no
code treatment (and the backticks themselves are stripped), so code inside a
table is indistinguishable from surrounding text.

Context: this looks like the known first-pass limitation in `cellText`
(parse.go) — it runs `parseInline` but discards the styling, keeping only the
plain text, because RENDER-06's bold/code/link colours were deliberately not
carried into table cells yet. This item asks for the code (and likely bold/link
too) runs to be carried through the column-width layout so inline markup
renders inside cells the way it does in paragraphs.
