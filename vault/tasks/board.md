# Board

Last updated: 2026-08-07 (ID-02)

**Active milestone:** M1 — Real documents
**Awaiting review:** PARSE-02 — the first real-file baseline

> `Awaiting review:` names the one felt leg the maintainer has not judged yet.
> While it is set, only mechanical legs may be picked. Clear it when feedback
> lands.

## Legend

- `[felt]` — needs the maintainer to look at it. Ends with a demo recipe and a stop.
- `[mech]` — tests can prove it. Proceed on your own judgement.

---

## In progress

*(none)*

## Backlog — M1

- **RENDER-05** `[felt]` Heading hierarchy. Levels are parsed and carried on the
  block but unused by the renderer, so `#` and `###` look identical. A terminal
  has no font sizes, so this needs a real decision: indentation, rules, colour
  weight, or a visible prefix.
- **RENDER-06** `[felt]` Inline markup — `**bold**`, `` `code` ``, links —
  currently shows as raw source, which is a large share of the reading-comfort
  gap on a real document.
- **RENDER-01** `[felt]` Render lists. One block type, then stop and ask.
- **RENDER-02** `[felt]` Render fenced code blocks with chroma highlighting.
- **RENDER-03** `[felt]` Render tables.
- **RENDER-04** `[felt]` Render block quotes.

## Backlog — M2

- **STORE-01** `[mech]` Thread files under `.margin/threads/`, one per thread,
  markdown with frontmatter. Read and write, round-trip tested.
- **STORE-02** `[mech]` Load threads on open; `fsnotify` reload when an agent
  writes one while margin is running.
- **THREAD-01** `[mech]` Resolvable threads: a thread carries a resolved state,
  set by either the reviewer or the agent, and it round-trips through the thread
  file. Blocked on **Q-0002** for how it is represented.
- **THREAD-02** `[felt]` What resolving *looks* like — which key, whether a
  resolved thread dims, collapses, or disappears, and whether resolving is
  reachable from the collapsed line or only the expanded thread.
- **THREAD-03** `[mech]` Delete a comment, and delete a whole thread. Deletion is
  the one gesture that destroys the reviewer's own words, so it is explicit and
  confirmed — see principle 3 in goals.md. Blocked on **Q-0002**.
- **THREAD-04** `[felt]` What deletion looks like and what confirms it. An agent
  reply already in the thread makes "delete the thread" less obviously the
  reviewer's alone to do.
- **EXPORT-03** `[mech]` `--stdout` export, to pipe a review into an agent
  without going through the clipboard. *Promoted by feedback 2026-08-07 — take
  it ahead of the rest of M2.*
- **EXPORT-05** `[mech]` Resolved threads are excluded from the export by
  default — it is a list of what still needs doing — with a flag to include them
  so an agent can see what it already addressed. Depends on THREAD-01.
- **EXPORT-04** `[felt]` The export's wording and shape, judged by pasting it at
  an agent and seeing whether it acts correctly. *First round addressed
  2026-08-07 (line locators, structure-preserving quotes); still wants a real
  agent to act on one.*

## Backlog — M3 (navigation)

Not blocked by Q-0001: these are about moving around one document, and are
independent of whether the unit of review turns out to be a file or a tree.

- **SCROLL-01** `[mech]` Decouple the scroll offset from focus. Today `clampScroll`
  recomputes it from the focused block on every render, so any user-driven scroll
  would snap straight back. Needs a real offset the user owns, with focus-follow
  applying only when focus actually moved. Prerequisite for the other two.
- **SCROLL-02** `[felt]` Page and half-page keys — `ctrl+d`/`ctrl+u`,
  `ctrl+f`/`ctrl+b`, `pgup`/`pgdn`, `home`/`end`. The open decision is whether
  they carry focus along with the viewport, the way vim's `ctrl+d` carries the
  cursor, or scroll underneath a focus that stays put.
- **SCROLL-03** `[felt]` Mouse wheel. Almost certainly scroll-only with focus
  left where it is, since that is how a wheel behaves everywhere — but that makes
  it deliberately inconsistent with SCROLL-02, which is worth deciding on purpose
  rather than by accident. Also: a wheel over an open composer should probably
  scroll the comment rather than the document.

## Blocked

- The **multi-document** parts of M3 — blocked on **Q-0001** (file or tree as
  the unit of review). The single-document navigation items above are not.
- **THREAD-01**, **THREAD-03** — blocked on **Q-0002** (how resolution and
  deletion are represented in the thread file). Worth answering before STORE-01
  fixes that format.

## Done

- [x] **EXPORT-01/02** clipboard export with `Y`; headings commentable; `space`
      cycles marks; `ctrl+enter` submits — done 2026-08-06
- [x] **PARSE-01** goldmark into a block list with byte offsets — done 2026-08-06
- [x] **PARSE-02** real files from argv; the seeded document is now a test fixture — done 2026-08-06
- [x] **M0** Foundation — interaction model end to end — done 2026-08-06
- [x] **ID-01** stamp a stable block id into the source, invisible to other
      renderers, as an HTML comment after the block; content-derived anchors
      remain the fallback until a block is stamped — done 2026-08-07
- [x] **ID-02** re-attach threads to blocks by id on open (`reattach`), plus
      `stampAll` to give every commented-on block a durable id in one pass;
      a thread whose block is gone is flagged orphaned rather than silently
      dropped — done 2026-08-07
