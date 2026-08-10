# Roadmap

Milestones in order. Work them in sequence; do not start the next one while the
current has open exit criteria.

Each milestone names roughly how much of it is **felt** (needs the maintainer to
look at it) versus **mechanical** (tests can prove it). Felt-heavy milestones will
be slow by design — that is not a problem to solve.

---

## M0 — Foundation ✅

*Status: done, 2026-08-06.*

The interaction model, working end to end against a document seeded in code.

- [x] nvim spliced into the document as a composer, stripped to a textarea
- [x] Blur kills the child, drafts survive, refocus resumes in place
- [x] Block-anchored threads with replies and in-place editing
- [x] Review marks with section roll-up
- [x] Damage-driven rendering; host round trip measured, not guessed

---

## M1 — Real documents ✅

*Status: done, 2026-08-08.*

*Mostly mechanical. This is where to build momentum.*

Replace the seeded document with a parsed one.

- [x] Parse markdown with `goldmark`, keeping byte offsets per block
- [x] Render headings, paragraphs, lists, code blocks, block quotes, tables
- [x] Load a file from `argv`; render it at a comfortable measure
- [x] Stamp stable block ids into the source, lazily — only blocks that acquire a
      thread get one
- [x] Re-open a stamped file and re-attach threads by id

The lazy-stamping item's *wiring* is recorded as a known gap in D12: `stampAll`
exists and is tested (ID-01/02) but nothing in the running app calls it yet, so
a block's anchor is content-derived and survives a reword only while its text is
unchanged. Board record: closed when RENDER-07 emptied `Backlog — M1`.

**Exit:** `margin some-real-doc.md` renders it, and a comment left today is still
attached tomorrow after the file has been edited around it.

**Felt within it:** how each block type renders. Do one block type per felt leg;
do not do all six and then ask.

---

## M2 — Persistence and the loop ✅

*Status: done, 2026-08-10.*

*Mechanical.*

Threads become files an agent can read and write with no tooling.

- [x] Threads as markdown under `.margin/threads/`, one file per thread
- [x] Frontmatter carries anchor, quote fallback, status; replies append
- [x] Load threads on open; watch for external changes and reload
- [x] `--stdout` export: the whole review as a prompt an agent acts on
- [x] Orphan detection — a thread whose block id has vanished is surfaced, not
      silently dropped
- [x] Resolvable threads, resolvable by the reviewer *or* the agent — the loop
      does not close without this, since round two otherwise re-litigates
      everything from round one
- [x] Deleting a comment, and deleting a thread

**Exit:** review a document, pipe the export into an agent, have it revise the
file and reply in the thread files, reopen and see the replies — and see which
threads it considers resolved.

---

## M3 — Reading at scale

*Felt-heavy. Expect this to be the slow milestone.*

- [x] Review a directory, not just a file — `margin DIR/` / bare `margin`
      opens a tree of markdown files in a left pane (`tab` toggles focus,
      `j`/`k` move, `enter`/`l` opens the focused file, switching threads and
      marks to it); the pane's look/position/toggle/focus is felt, journal
      2026-08-11.1
- [ ] Navigation between documents, and a cross-document comment inbox
      *(both halves landed 2026-08-11: cross-document link following —
      `ctrl+]` on a link to another file in the tree opens it — journal
      2026-08-11.2, and the comment inbox — `i` lists every thread across the
      tree, newest first, `enter` jumping to its document and block — journal
      2026-08-11.3)*
- [x] A scroll offset the user owns, decoupled from focus-follow (SCROLL-01)
- [x] Page and half-page keys (SCROLL-02)
- [x] Mouse wheel (SCROLL-03)
- [x] Link navigation between blocks, with a jumplist (`ctrl+o` / `ctrl+i`)
- [ ] Review progress across the whole tree

**Open question that gates the design:** is the unit of review a file or a tree?
It changes the navigation model and is hard to retrofit. Raise it before building.
*(Answered 2026-08-07: Q-0001 closed as **D10** — the unit of review is both a
file and a tree, chosen by argv; `margin FILE.md` stays a single-document review.
The tree pane's first build is journal 2026-08-11.1 — the appearance, position,
toggle and focus it chose are felt and need the maintainer's look (on the
board's `Needs a look:` list); interaction.md's "Not settled" tracks what is
still open.)*

---

## M4 — Round two

*The differentiator, and the reason block ids exist.*

- [ ] Diff a document against the previous reviewed revision
- [ ] Render the diff as prose — a rewritten paragraph reads as a rewritten
      paragraph, not as a wall of `-`/`+`
- [ ] Carry marks and threads across a revision; show what changed under a mark

**Exit:** an agent hands back round four of a plan and the reviewer can see what
moved without re-reading it.

---

## Later, explicitly not now

- GitHub / GitLab review sync
- An MCP server so an agent can ask an anchored question and block on the answer
  (the idea is good and worth stealing; it is not v1)
- Images, mermaid, LaTeX in the rendered view
- Any form of multi-user collaboration
