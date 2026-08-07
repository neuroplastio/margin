# Board

Last updated: 2026-08-07 (STORE-03 done)

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
- **RENDER-07** `[felt]` Frontmatter's visual treatment. PARSE-03 stopped it
  from corrupting the block model and made it invisible in the meantime, but
  invisible is a default, not a decision — the maintainer's feedback offered
  three real candidates (hidden-on-demand, a dimmed key/value block, a
  collapsed line that expands) and none is picked yet.

## Backlog — M2

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
- **EXPORT-05** `[mech]` Resolved threads are excluded from the export by
  default — it is a list of what still needs doing — with a flag to include them
  so an agent can see what it already addressed. Depends on THREAD-01.
- **EXPORT-04** `[felt]` The export's wording and shape, judged by pasting it at
  an agent and seeing whether it acts correctly. *First round addressed
  2026-08-07 (line locators, structure-preserving quotes); still wants a real
  agent to act on one.*

## Backlog — Commands (from feedback, 2026-08-07-command-palette)

Not part of a milestone; cross-cutting infrastructure the palette feedback
asked for. Split per the feedback's own suggested ordering — see
`vault/journal/2026-08-07.6.md` for why CMD-01 alone is this leg.

- **CMD-02** `[mech]` Palette matching logic: given the registry and a query
  string, rank and filter commands — no UI, just the function and its tests.
  Testable in isolation from where or how it is ever displayed.
- **CMD-03** `[felt]` The palette itself: `:` opens it, where it appears, how
  much room it takes, whether it dims the document, ranked vs. stable
  ordering. Needs a real screen — see the feedback's "Not settled" section for
  the open questions.
- **CMD-04** `[mech]` Focus-sensitive listing and a titled target: given the
  registry's `Applicable` and the current focus, produce the filtered,
  targeted list the palette would show (e.g. "Delete — comment by agent").
  Still no UI.
- **CMD-05** Staged value commands and key-opens-a-stage (requirement 5) —
  later; depends on CMD-03 existing first.

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
  deletion are represented in the thread file). STORE-01 shipped without
  either field on purpose, so answering Q-0002 only has to extend the format
  now, not migrate it.

## Done

- [x] **STORE-03** live reload: `threadWatcher` (`internal/review/watch.go`)
      wraps `fsnotify`, watching `.margin/threads/<docPath>/` for the open
      document and turning a create/write/rename of a `.md` file into a
      `threadsChangedMsg` the Bubble Tea loop reloads on. `reloadThreads`
      re-reads every thread file for the document and merges into the
      in-memory map — new anchor added, existing anchor's quote and posted
      comments replaced — without touching unsubmitted drafts, since those
      are never written to disk to begin with. Watcher setup is non-fatal:
      a review still opens if it fails. Completes the STORE-02 journal's
      three-part split — done 2026-08-07
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
- [x] **STORE-01** thread files under `.margin/threads/<docPath>/<anchor>.md`
      (D9), markdown with frontmatter (`anchor`, `document`), quote fallback
      plus posted comments; `marshalThread`/`parseThreadFile` round-trip
      tested. Resolved/deletion fields deliberately left out pending
      **Q-0002**. Not yet wired into the running app — that's STORE-02 —
      done 2026-08-07
- [x] **PARSE-03** frontmatter parsing (from feedback): a leading
      `---…---` block is recognised before goldmark can misparse it as a
      setext heading (F10), given its own `blockFrontmatter` kind, and
      excluded from comments, marks and review progress. The visual
      treatment is deliberately left undecided — RENDER-07 — done 2026-08-07
- [x] **CMD-01** command registry (from feedback, the prerequisite it asked
      for): every verb margin can perform is now one entry in
      `internal/review/command.go` — id, description, applicability guard,
      `Run`. `handleKey` resolves a key through `keymap` to a command id and
      calls its `Run`; it carries no verb-specific behaviour of its own. No
      behaviour change — existing tests pass unmodified, plus new registry
      tests. The palette itself (CMD-02 onward) is not built — done 2026-08-07
- [x] **STORE-02** (partial — see STORE-03 above) `loadThreadsForDoc` loads
      every thread file on disk for the document `Run` opens, and `dismiss`
      persists a posted comment through a new `threadStore`, nil on every
      model a test builds so submitting in a test never touches disk. The
      fsnotify live-reload half of the original task is split out as
      STORE-03 — done 2026-08-07
- [x] **EXPORT-03** (from feedback, promoted ahead of the rest of M2)
      `margin --stdout FILE.md` runs the review interactively and, on quit,
      writes the same content `Y` produces to stdout instead of the
      clipboard — same `exportReview` call, no second non-interactive mode,
      per the feedback's own note that STORE-01 landing does not change this
      leg's semantics. `tea.WithOutput` points the interface at `/dev/tty`
      (see F11 in findings.md) so the pipe stays ANSI-free — pinned by
      `TestExportReviewContainsNoANSI` and a test that a missing controlling
      terminal is a reported error, not a silent fall-through to drawing over
      the pipe — done 2026-08-07
