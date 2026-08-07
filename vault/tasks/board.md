# Board

Last updated: 2026-08-07 (Q-0001/Q-0002 folded into decisions.md as D10/D11)

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
  collapsed line that expands) and none is picked yet. A 2026-08-07 follow-up
  feedback item confirmed this is the same gap (not a new one — PARSE-03 named
  it as this task the moment it landed) and supplied a ready-made fixture for
  the demo recipe:

  ```markdown
  ---
  name: retry-policy
  description: How outbound calls are retried
  status: draft
  tags: [reliability, networking]
  ---

  # Retry policy
  ```

  Use this (or one like it) when this leg is picked, so the demo recipe has a
  concrete document with a realistic key/value spread to judge the treatment
  against rather than a synthetic one.

## Backlog — M2

- **THREAD-01** `[mech]` Resolvable threads: a thread carries a resolved
  boolean, settable and clearable by either the reviewer or the agent, and it
  round-trips through the thread file, per **D11**.
- **THREAD-02** `[felt]` What resolving *looks* like — which key, whether a
  resolved thread dims, collapses, or disappears, and whether resolving is
  reachable from the collapsed line or only the expanded thread.
- **THREAD-03** `[mech]` Delete a comment, and delete a whole thread, as a
  tombstone — author and timestamp kept, body dropped — per **D11**. Deletion is
  the one gesture that destroys the reviewer's own words, so it is explicit and
  confirmed — see principle 3 in goals.md.
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

- **CMD-03** `[felt]` The palette itself: `:` opens it, where it appears, how
  much room it takes, whether it dims the document, ranked vs. stable
  ordering. Needs a real screen — see the feedback's "Not settled" section for
  the open questions.
- **CMD-05** Staged value commands and key-opens-a-stage (requirement 5) —
  later; depends on CMD-03 existing first.

## Backlog — M3 (navigation)

Independent of the tree view (D10): these are about moving around one
document, regardless of the order the tree-view work itself lands in.

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

*(nothing)*

Q-0001 and Q-0002 were answered 2026-08-07 and folded into
`knowledge/decisions.md` as **D10** (tree review) and **D11** (thread
resolution and deletion) in the leg that closed them; the question files are
deleted. See those entries for the settled shape, and `interaction.md`'s "Not
settled" section for what each still leaves open for a felt leg.

## Done

- [x] **SCROLL-01** decouple the scroll offset from focus: `model` gains
      `scrollAnchor cursor`, the focus position `clampScroll` last followed;
      it now re-derives the offset from the focused span only when
      `m.at != m.scrollAnchor`, otherwise returning `m.scroll` as-is
      (clamped only to stay in range). Prerequisite for SCROLL-02/03 — no
      observable behaviour change today, since nothing yet sets `m.scroll`
      any other way — done 2026-08-07
- [x] **CMD-04** focus-sensitive listing and the titled target: `paletteRows`
      (`internal/review/palette.go`) filters the registry to what
      `Applicable` allows given the current focus; a new `command.Target`
      field (nil for focus-independent commands) names what `comment.new`,
      `comment.edit` and the three mark commands would act on —
      `commentTarget`, `editTarget`, `markTarget` — and `paletteTitle`
      appends it to the description, e.g. "Mark reviewed — section (3
      blocks)". `editTarget` mirrors `editFocused`'s own precedence
      read-only; `markTarget` shares a new `sectionLabel` helper factored out
      of `toggleMark`/`cycleMark` so the palette and the status line can
      never disagree. Still no UI, no `:` key, nothing wired into
      `handleKey` — done 2026-08-07
- [x] **CMD-02** palette matching logic: `matchCommands` (new
      `internal/review/palette.go`) ranks and filters the registry against a
      query string — a fuzzy subsequence match (`fuzzyScore`) against each
      command's id and description, scored so a match starting right after a
      word boundary (`.`, `_`, `-`, space) and running consecutively outranks
      the same letters found scattered. A query that matches nothing excludes
      the command entirely, per requirement 4's "should not be listed at all"
      applied to search too. An empty query returns the registry unranked and
      unfiltered — deliberately not deciding the "Not settled" recency-vs-
      stable question. No UI, no `:` key, not wired into `handleKey` — done
      2026-08-07
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
