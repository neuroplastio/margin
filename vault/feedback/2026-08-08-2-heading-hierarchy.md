# Headings must be distinguishable — identical levels is a no-go

Priority: normal
Kind: rendering (felt)
Board: RENDER-05

Every heading renders the same: bold, one colour, no indent. On a real document
with `#`, `##` and `###` you cannot tell depth while scanning, which is most of
what scanning a long document *is*.

## Ideas worth trying

Not a decision — these are the directions I would look at first, and I would
rather see two or three on screen than argue them here:

- **Font style.** Bold for `#`, plain-but-coloured for `##`, dimmer for `###`.
  A terminal has no size axis, so weight and colour are what is left.
- **Left padding.** Indent by depth so the hierarchy is visible in the left
  edge rather than in the type. Costs horizontal room and interacts with the
  gutter, which is why it needs looking at rather than assuming.
- **A level hint in the gutter.** `#`, `##`, `###` — or a depth glyph — in the
  gutter column, leaving the heading text itself alone.

They combine. A gutter hint plus a weight difference may be enough without
spending any indentation.

## Constraints

- The gutter already carries the focus bar and the review mark, and it is three
  columns wide. A level hint has to fit alongside those or widen it — and
  widening takes room from the measure.
- Whatever it is has to survive a heading being focused, marked reviewed
  (dimmed) and flagged, without the level cue becoming unreadable in any of
  those states.
- `testdata/sample.md` has all three levels, so demo against that.
