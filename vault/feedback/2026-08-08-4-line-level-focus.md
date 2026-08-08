# Multi-line blocks should be navigable at line level

Priority: normal
Kind: interaction (felt, and a real design change)

Lists and numbered lists should be individually addressable — a list of six
items is six things, not one. And more generally: **for any multi-line block, it
should be possible to go down to line level.**

## Why this is bigger than lists

Focus is currently one block. That was right when a block was a paragraph, but a
code fence of forty lines, a table of ten rows and a list of six items are each
one focus stop today, which means:

- A comment on a list attaches to the whole list, when what you meant was the
  third item.
- The export quotes the whole block to locate a comment, which is why it needed
  a six-line cap — the cap is a workaround for not being able to name a line.
- Reviewing a long fence means reading it with no cursor to keep your place.

## What I think it should be

A second level of focus *inside* a block, the way a focused thread already steps
through its comments. `j`/`k` moves between blocks; entering a block moves
between its lines. Anchoring, commenting and marking all become available at
whichever level you are on.

That is deliberately close to the thread model already built — `cursor{entry,
comment}` becomes something like `cursor{entry, line, comment}` — so it may be
less new machinery than it sounds.

## Open, and worth deciding before building

- **How you enter and leave a block.** `Enter` to descend and `Esc` to come back
  up? Or does `j` just walk into it automatically, so there is one flat sequence
  of stops and no mode at all? The second is fewer keys but makes long fences
  tedious to scroll past.
- **What an anchor on a line means when the block is rewritten.** Block ids
  survive rewording because the block persists. A line does not have that
  property — line 3 of a rewritten list is not the same line 3. Possibly a
  line-level comment anchors to the block and *records* a line, degrading to a
  block comment when the line no longer matches.
- **Whether marks work per line or stay per block.** Per line seems like a lot
  of state for little gain, but it is the same question.
- **Lists specifically may deserve to be real blocks rather than lines.** An
  item is a semantic unit in a way that line 4 of a code fence is not — so
  possibly lists get split into item blocks at parse time, and line-level focus
  is only for fences and tables. That would be simpler and might be enough.

That last point is worth settling first: if splitting lists into item blocks
covers most of the want, it is a much smaller change than a second focus level.
