# Porting margin to hotty-surf — the plan

**Done 2026-11-10** (legs 5–10, hotty-surf v0.5.1; D25, journals 2026-11-10.1–.8).

Revised 2026-11-09: hotty-a2ui is archived, and the stack has moved to
`neuroplast.io/...` paths (hotty-go v0.7.1, hotty-surf v0.3.0, cel-lite
v0.2.0, myelinkey v0.2.0). The first plan (2026-11-05, gov's brief: "port
margin to latest hotty-surf") is kept below as legs 1–4, which landed.

Not changed by any leg: threads, ids, events and the other on-disk formats.

## Where margin stands

| | today (main, d5cfb3e) | target |
| --- | --- | --- |
| hotty-go, hottytea, hottytest | `github.com/neuroplastio/hotty-go` 23af564 (0.6.1 pseudo) | `neuroplast.io/hotty-go` **v0.7.1** |
| hotty-surf | `github.com/neuroplastio/hotty-surf` 3059253 (pseudo) | `neuroplast.io/hotty-surf` **v0.3.0** |
| cel-lite, myelinkey | `github.com/...` v0.1.1, v0.1.0 | `neuroplast.io/...` v0.2.0, v0.2.0 |
| hotty-a2ui | b1f6b54, archived | gone |

On hotty-surf: the seam (`surfhost.go`), the footer, the search prompt and the
palette (legs 1–4).

Still on hotty-a2ui (`kit.go`, 318 lines):

| pane | a2ui component | surf needs | gap |
| --- | --- | --- | --- |
| composer (`composer.go`) | `TextField` `longText` | a multi-row field | **G1** |
| threads view (`inbox.go`) | `HottyTable` | aligned columns, a selection | **G5** |
| tree pane (`tree.go`) | `HottyTree` | depth, open/closed, ←/→ | **G2** |
| open thread (`review.go` `threadPane`) | Markdown `Text` | inline code, emphasis, links, fences | **G3** |

At v0.3.0 hotty-surf has none of G1, G2, G3 or G5. Its board lists them
under "Kit gaps, after the kit", in the order G1, G5, G2, G3, with none
started.

## What a trial bump showed (2026-11-09, a scratch worktree, not committed)

The import paths were rewritten and the new versions taken with `go get` and
`go mod tidy`. `go build ./...` then fails in exactly two places:

1. `surfhost.go`: `surfrun.WriteFrame` now takes a menu writer as well as a
   list writer (hotty 0.7's secondary press, surf's context menus).
2. `hotty.go` `hotPaneEvent`: a host's `hotty.Event` is the new path's, and
   hotty-a2ui's `html.Rendition.Event` takes the old path's.

Everything else at the a2ui boundary is a string: `Doc()`, `Update()`, key
names, cells lines. That is why one converter is enough. Also seen: with
nothing else needing the old path, MVS drops hotty-a2ui's hotty-go to its own
floor, **v0.5.0**. The old path has to stay pinned at 23af564 for as long as
the panes run on hotty-a2ui.

## Legs, in order

### Leg 5 — the stack bump (felt; replaces the board's "hotty-surf 730ae32")

- Rewrite the import paths and pin hotty-go, hottytea and hottytest at v0.7.1,
  hotty-surf at v0.3.0, and cel-lite at v0.2.0.
- `go generate`: the `//go:generate go run …/cmd/surfgen` lines take the new
  path; regenerate `surf_gen.go` with v0.3.0's surfgen.
- `WriteFrame` takes a menu writer. Pass `nil` (margin opens no menu yet; a
  nil menu is never opened, so it can't hold the keyboard).
- **A temporary bridge, `a2uibridge.go`.** One function converts a new
  `hotty.Event` into the old one. The nested types (`Area`, `Drag`, `Link`,
  `Hover`, `View`, `Caps`, …) have the same fields, so they convert directly;
  `Button` and `At` have no old counterpart and are dropped. Keep
  `github.com/neuroplastio/hotty-go` pinned at 23af564, as an indirect
  require with a comment saying why. The file's comment says it goes in the
  last leg. Building one now is not bridging a wire (P-3): both ends are in
  this binary, and it lives only until the last a2ui pane goes.
- Bring over the branch `kit-730ae32-wip` (c202f02): 730ae32 is in v0.3.0.
  That means the lowercase footer key in `watch_test.go` and the palette's
  query row at `pad="1 0"`. Delete the branch afterwards.
- `--version` ends `· hotty 0.7`. A 0.6 host still works, because the SDK
  speaks both.
- v0.3.0 also changes the look: lowercase keycaps, `chip`/`badge`, modifiers
  beat a widget's own rules, base text enlarged, and controls scaled with
  the terminal font. Check `paletteHostHeight`, which counts Compact rows from
  `Caps.CellCSS`, against the scaled text.
- Shots at 1796x1153 and 897x1153 logical, scale 1.6, host and cells: the
  footer, search and palette, each with and without a status. Settles the
  board's 730ae32 item: does `j move  c comment` need a gap or a tone, and
  does the selected row's bar touch the first letter?
- Gate: `make check`, `make test-race`, and the `hottytest` hover regressions
  (`TestHottyHoverPlacesNothingAgain`, `…RepairsInsteadOfRelaying`).

### Leg 6 — composer on `<field rows>` (felt; waits for G1)

- `composer.surf`: a `field` with rows that grows to 8 and then scrolls,
  plus Submit/Save, Keep draft and Discard as buttons. `program="ctrl+s
  ctrl+enter ctrl+g"` on the surface replaces `composerKeys`, and escape
  keeps the draft.
- Tests that a dismiss never loses words (D7): paste, a long line, a draft
  resumed with the cursor at its end, and every outcome in both cells and
  host mode.

### Leg 7 — threads view on `<table>` (felt; waits for G5)

The columns are thread, document, time and author. A `[block gone]` row and
the selection keys stay as they are. Once the table exists, the palette's
fixed-width id column moves to it in the same leg.

### Leg 8 — tree pane on the tree list (felt; waits for G2)

margin flattens its tree into items with `depth` and `open` (G2's shape), and
the fold keys stay margin's. Progress marks go on each row.

### Leg 9 — the open thread on `<markdown>` (felt; waits for G3)

The comments are a stack of `<markdown>`. Until G3 lands, margin keeps
`commentParagraphs`.

### Leg 10 — cleanup (mech)

- Delete `kit.go`, `a2uibridge.go` and the `pane` plumbing in `hotty.go`
  (`hotPaneSurfaces`, `hotPaneEvent`).
- Drop hotty-a2ui and `github.com/neuroplastio/hotty-go` from `go.mod`.
- Write a new D that supersedes D22 ("the review's panes are hotty-surf
  surfaces"), and update the README and findings.md.

Each leg is one commit. `make check` and `make test-race` must pass under the
memory cap. A felt leg gets before and after shots (above) in a private
wrapper (HOME, XDG_* and PLX_DIR of the run's own).

## Open for the maintainer

1. **The gaps gate legs 6–9, and none is started.** They are hotty-surf's
   (hotty Agent, Design Agent), so margin asks gov to start **G1 first**.
   Until G1 lands the composer can't move, and the composer is what keeps
   hotty-a2ui in the build. The alternatives are worse: a one-line field
   loses the composer's shape (D7), and going back to nvim undoes D22.
2. **G5, G2 and G3 before their gaps land?** Two choices:
   - Wait, which is the rule in hotty-surf-gaps.md.
   - Port them now with what surf already has: a `list` with fixed-width
     texts (the palette's precedent), a `list` margin flattens with `indent`
     and a glyph for the chevron, and `line wrap` runs for comments. This
     drops the hotty-a2ui panes sooner. The cost is a home-made shape that
     gets redone when each gap lands. Plain text in comments also loses
     inline code, which is the biggest regression of the three.
   The plan waits unless told otherwise.
3. **hotty 0.7's secondary press.** surf's context menu comes out of the box
   (a block's or a thread's own menu by right press or long press). Not part
   of the port. A felt leg after it, if wanted.
4. **margin's own module path.** It is still `github.com/neuroplastio/margin`,
   as plx's is, and engram is pinned at its old path. Moving either changes
   `go install` and the launcher's packaging, so it is a leg of its own, not
   part of this port.

---

## History: the first plan (2026-11-05)

Target was hotty-surf `origin/main` 35f857b. The legs:

1. Seam: `surfhost.go` (journal 2026-11-05.2). Landed.
2. Footer (journal 2026-11-05.3, and .10 for fit-start). Landed.
3. Search prompt (2026-11-05.4). Landed.
4. Palette (2026-11-05.7, -.9). Landed.
5–8. Tree, threads view, open thread, composer: blocked on G2, G5, G3, G1.
   They are now legs 6–9 above.
9. Cleanup: now leg 10.

The document column stays margin's own `hotdoc` surfaces, placed with
hottytea's `Session` (G4: not surf's vocabulary, D17, D24).
