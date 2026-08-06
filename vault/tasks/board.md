# Board

Last updated: 2026-08-06

**Active milestone:** M1 — Real documents
**Awaiting review:** *(none)*

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

- **PARSE-01** `[mech]` Parse markdown with goldmark into a block list, keeping
  each block's byte offsets. Pure function, no UI. Table-driven tests over a
  fixture document.
- **PARSE-02** `[mech]` Replace the seeded document with the parsed one, wired to
  a file path from `argv`. Headings and paragraphs only; other block types render
  as plain text for now.
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
- **EXPORT-01** `[mech]` `--stdout` export of the whole review.
- **EXPORT-02** `[felt]` The export's actual wording and shape — judged by
  pasting it at an agent and seeing whether it acts correctly.

## Blocked

- Everything in M3 — blocked on **Q-0001** (file or tree as the unit of review).

## Done

- [x] **M0** Foundation — interaction model end to end — done 2026-08-06
