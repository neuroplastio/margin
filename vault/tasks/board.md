# Board

Last updated: 2026-08-06

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
- **ID-01** `[mech]` Stamp a stable block id into the markdown source when a block
  first acquires a thread. Invisible to other renderers. Round-trip test: stamp,
  reparse, ids stable.
- **ID-02** `[mech]` Re-attach threads to blocks by id on open. Test that a block
  can be fully reworded and keep its thread, and that a deleted block orphans its
  thread detectably.

## Backlog — M2

- **STORE-01** `[mech]` Thread files under `.margin/threads/`, one per thread,
  markdown with frontmatter. Read and write, round-trip tested.
- **STORE-02** `[mech]` Load threads on open; `fsnotify` reload when an agent
  writes one while margin is running.
- **EXPORT-03** `[mech]` `--stdout` export, to pipe a review into an agent
  without going through the clipboard.
- **EXPORT-04** `[felt]` The export's wording and shape, judged by pasting it at
  an agent and seeing whether it acts correctly.

## Blocked

- Everything in M3 — blocked on **Q-0001** (file or tree as the unit of review).

## Done

- [x] **EXPORT-01/02** clipboard export with `Y`; headings commentable; `space`
      cycles marks; `ctrl+enter` submits — done 2026-08-06
- [x] **PARSE-01** goldmark into a block list with byte offsets — done 2026-08-06
- [x] **PARSE-02** real files from argv; the seeded document is now a test fixture — done 2026-08-06
- [x] **M0** Foundation — interaction model end to end — done 2026-08-06
