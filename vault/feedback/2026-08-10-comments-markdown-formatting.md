# Comments should support markdown formatting

Feedback, 2026-08-10.

Comments should render markdown formatting. Currently even line breaks are
getting inlined: `appendComments` (review.go) wraps comment bodies with plain
`wrap(c.body, w-6)`, which collapses newlines and reflows, so a multi-paragraph
or multi-line comment reads as one run-on paragraph.

What is wanted: markdown formatting inside comment bodies — at minimum real
line breaks / paragraph breaks preserved, and presumably inline markup
(`**bold**`, `` `code` ``, `[links](...)`) the way the document's paragraphs
render it via `wrapInline` (RENDER-06 settled that styling).

Scope decisions for the implementer: how far the formatting goes (line breaks
only vs inline markup vs fenced blocks); whether the draft and the edited
draft preview follow the same rendering; and whether the composer itself needs
a hint that markdown is understood. The exported thread file is already plain
markdown, so this is purely a rendering concern — a felt leg with a demo
recipe.
