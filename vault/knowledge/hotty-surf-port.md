# Porting margin to hotty-surf — the plan

Brief: gov, 2026-11-05 (the maintainer: "port margin to latest hotty-surf,
and I will also spot some gaps like a missing textarea"). Target: hotty-surf
`origin/main` (35f857b at the time of writing), by a Go pseudo-version. The
kit redesign on its branch `design/2026-11-05-kit` is not built on; when it
lands, margin moves to it in a leg of its own.

Not changed by any leg: threads, ids, events and the other on-disk formats.

## What margin draws today

- The document column: each prose block a HOTTY surface from goldmark HTML
  (`hotdoc.go`, D17), mermaid as SVG (D24), cells elsewhere. Placed by
  margin itself with hottytea's `Session`, measured per block.
- Seven A2UI panes (D22, `kit.go`): footer, search prompt, palette, tree
  pane, threads view (inbox), an open thread, composer. Cells rendition
  elsewhere, HTML surface on a host.

## What maps to what

| margin | hotty-surf |
| --- | --- |
| footer (`footer.go`) | `row`/`line` of `key` and `text`, `fit` for the narrow pane |
| search prompt (`search.go`) | `row search` with a `field`, the count a `text` |
| palette (`palette.go`) | `dialog` with a `field` and a `list` of `item`s; the gallery's palette story is the shape |
| tree pane (`tree.go`) | `list nav` of `item`s with `indent`; folding is the gap below |
| threads view (`inbox.go`) | `list entries` of `item`s; a table is rows of `text`s |
| an open thread's comments (`review.go` `threadPane`) | `stack` of `line wrap`s; Markdown comments are a gap |
| composer (`composer.go`) | needs a multi-row text field: **gap**; stays hotty-a2ui's |
| the document column's prose/SVG surfaces | not surf's vocabulary: **gap**; stays margin's `hotdoc` |

## What goes, and what stays

- Goes, pane by pane, as each leg lands: that pane's use of hotty-a2ui
  (`kit.go` shrinks to what the composer and thread view still need).
- hotty-a2ui leaves `go.mod` only after the composer and thread view move.
- `hottytea` stays as long as margin places the document's surfaces itself.
  Whether surf's `surfrun.Surface` can be hosted inside margin's Bubble Tea
  loop (it is built for a host that relays documents) is leg 1's question.
- cells fallback: surf's own (`pkg/cells`) replaces `rendition/cells` per pane.

## Legs, in order

1. **Seam.** `go.mod` adds hotty-surf; one trivial surf component hosted in
   margin's loop through `surfrun.Surface`, hottytea placing it, cells and
   host both. Mechanical; no visible change. Fails fast if hosting does not
   fit, and then the gap list says why.
2. **Footer** on surf. Felt. Before/after shots.
3. **Search prompt.** Felt.
4. **Palette.** Felt.
5. **Tree pane**, with whatever folding surf has; the fold keys stay as is
   until the gap is filled. Felt.
6. **Threads view.** Felt.
7. **Open thread.** Felt; comments stay plain text in surf until Markdown
   exists, so this leg may keep hotty-a2ui for the thread pane (said in its
   journal entry).
8. **Composer** — held until surf has a textarea.
9. **Cleanup:** drop what no pane uses; update README, D22 amended by a new D.

Each leg: one commit, `make check` (and `make test-race` for panes/HOTTY)
green, under the memory cap; a felt leg gets before and after shots at
1796x1153 and 897x1153 logical, scale 1.6, on a host and in cells, in a
private wrapper (HOME, XDG_*, PLX_DIR of the run's own).

## Gaps

In [hotty-surf-gaps.md](hotty-surf-gaps.md). Never worked around inside
margin with a home-made component.
