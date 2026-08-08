# Three rendering bugs found reviewing testdata/sample.md

Priority: high
Kind: bug
Take before the felt items filed alongside this.

Three separate defects. The first two are root-caused below; the third I could
not reproduce headlessly and needs looking at on a real terminal.

## 1. Tables render as collapsed prose

`| Component | Before | After |` and its rows come out as a single wrapped
paragraph:

```
| Component | Before | After | | --- | --- | --- | | Session read |
in-process map | Redis `GET`, ~0.4ms p50 | | Session write | in-process map
| Redis `SETEX`, TTL 24h | | Deploy | drops sessions | no effect | | Node
loss | drops that node's sessions | no effect |
```

**Root cause:** `goldmark.New()` is constructed with no extensions, and tables
are a GFM extension, not part of CommonMark. So the table never becomes a table
node — it parses as an `*ast.Paragraph`, and `collapse()` then does exactly what
it is supposed to do to a paragraph: joins the source lines into one and wraps
them to the measure.

Confirmed in isolation — a document containing only a three-row table parses to
one block of `kind=blockPara`.

The fix is `goldmark.New(goldmark.WithExtensions(extension.Table))`, which makes
it a `*ast.Table` and so a `blockRaw` rendered verbatim. That alone stops the
mangling. Making it *look* like a table is RENDER-03 and separate.

Worth checking what else is missing while you are in there — strikethrough,
task list items and autolinks are all GFM extensions too, and agent-written
markdown uses `- [ ]` constantly.

## 2. Long list items are cut off

Bulleted and numbered items overflow the measure and are truncated rather than
wrapped:

```
len=84  measure=76
"   - **Week 1** — write to both stores, read from memory. Redis is shadow traffic;"
```

**Root cause:** lists are `blockRaw`, and `render()` emits a `blockRaw`'s lines
verbatim — deliberately, since for a code fence the line breaks *are* the
content and re-wrapping would destroy it.

That reasoning is right for code fences and tables. It is wrong for lists: a
list item is prose, and prose has to wrap. The verbatim rule was applied to the
whole `blockRaw` bucket when it only ever justified the parts of that bucket
where layout is semantic.

So this is not a one-line wrap fix — it wants the bucket split. Something like:
lists wrap each item to the measure with a hanging indent that keeps the marker
column clear; code fences and tables stay verbatim and scroll horizontally if
they must.

## 3. The frame scrolls one line on load, hiding the top heading

Reported: opening a document scrolls by one line, so the first heading is not
visible until you scroll up.

**I could not reproduce this headlessly.** At every viewport height from 20 to
120 rows, `m.scroll` is 0 and the first rendered line is the `#` heading. So it
is not the scroll offset, and it is not `clampScroll`.

The likely candidate is a frame-height off-by-one that only manifests on a real
terminal: `View()` emits the viewport slice, then `"\n\n  "` plus the footer. If
that totals one line more than the terminal has rows, the terminal itself
scrolls and the top line goes off the top — which looks exactly like this and is
invisible to a test that inspects the string rather than a screen.

To confirm: count the lines `View().Content` actually produces against `m.h`,
for a document longer than the viewport. If it is `m.h` rather than `m.h - 1`,
that is the bug.
