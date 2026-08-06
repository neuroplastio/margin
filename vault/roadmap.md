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

## M1 — Real documents

*Mostly mechanical. This is where to build momentum.*

Replace the seeded document with a parsed one.

- [ ] Parse markdown with `goldmark`, keeping byte offsets per block
- [ ] Render headings, paragraphs, lists, code blocks, block quotes, tables
- [ ] Load a file from `argv`; render it at a comfortable measure
- [ ] Stamp stable block ids into the source, lazily — only blocks that acquire a
      thread get one
- [ ] Re-open a stamped file and re-attach threads by id

**Exit:** `margin some-real-doc.md` renders it, and a comment left today is still
attached tomorrow after the file has been edited around it.

**Felt within it:** how each block type renders. Do one block type per felt leg;
do not do all six and then ask.

---

## M2 — Persistence and the loop

*Mechanical.*

Threads become files an agent can read and write with no tooling.

- [ ] Threads as markdown under `.margin/threads/`, one file per thread
- [ ] Frontmatter carries anchor, quote fallback, status; replies append
- [ ] Load threads on open; watch for external changes and reload
- [ ] `--stdout` export: the whole review as a prompt an agent acts on
- [ ] Orphan detection — a thread whose block id has vanished is surfaced, not
      silently dropped
- [ ] Resolvable threads, resolvable by the reviewer *or* the agent — the loop
      does not close without this, since round two otherwise re-litigates
      everything from round one
- [ ] Deleting a comment, and deleting a thread

**Exit:** review a document, pipe the export into an agent, have it revise the
file and reply in the thread files, reopen and see the replies — and see which
threads it considers resolved.

---

## M3 — Reading at scale

*Felt-heavy. Expect this to be the slow milestone.*

- [ ] Review a directory, not just a file
- [ ] Navigation between documents, and a cross-document comment inbox
- [ ] Scroll and jump keys beyond keep-focus-visible
- [ ] Link navigation between blocks, with a jumplist (`ctrl+o` / `ctrl+i`)
- [ ] Review progress across the whole tree

**Open question that gates the design:** is the unit of review a file or a tree?
It changes the navigation model and is hard to retrofit. Raise it before building.

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
