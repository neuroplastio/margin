# Decisions

Append-only, `Dn` numbered. **Binding**: a future leg must not silently
contradict one. Supersede with a new decision that says which it replaces.

This log is skimmed every leg, so keep it load-bearing. A `Dn` is a constraint on
future work, not a description of how a leg was built — that belongs in the
journal.

Interaction behaviour lives in [`interaction.md`](interaction.md), not here.
Technical gotchas live in [`findings.md`](findings.md), not here.

---

**D1 — Go, with Bubble Tea v2 and lipgloss v2.**
The embedded terminal (`charmbracelet/x/vt`) and the pty bridge are in the same
ecosystem, and the interaction substrate is worth more than any advantage another
language's emulator library would bring. The emulator is display-only — the child
writes the file — so its bugs are cosmetic rather than data loss.

**D2 — The composer is a real `nvim` child process, never a built-in editor.**
margin does not implement text editing. Anything that looks like it needs a text
widget is a sign the boundary is in the wrong place.

**D3 — The composer reports its outcome as an exit code, via `:cquit N`.**
0 submit, 2 keep as draft, 1 discard. No socket, no protocol, no handshake file.
Blur uses the same path — see F7 in `findings.md`.

**D4 — Comments anchor to stable block ids, not line numbers or fuzzy text.**
Line numbers do not survive an agent rewriting a document. Ids make re-attachment
exact and orphaning detectable, rather than a matching heuristic that is
sometimes quietly wrong.

**D5 — Threads persist as markdown files an agent can read and write directly.**
No database, no daemon, no schema to learn. The agent is a participant in the
review, and it participates with ordinary file tools.

**D6 — One render pass produces the lines and the spans together.**
Click routing, cursor placement and scrolling all read those spans. Nothing
recomputes a position independently. See F5 in `findings.md` for what happens
otherwise.

**D7 — Losing the user's words is the worst possible bug.**
Every ambiguous dismissal keeps the text. Discarding is always explicit. When in
doubt about an edge case, keep.

**D8 — Commits are authored by the maintainer; the agent is disclosed in trailers.**
`Author:` is `Anatoly Rugalev <anatoly.rugalev@gmail.com>` on every commit,
including those an unsupervised run produces. Agent involvement is disclosed with
`Co-Authored-By:`, and an autonomous run additionally carries
`Claude-Session: <url>` so any commit can be traced to the session that made it.
The `Co-Authored-By:` trailer is **always** exactly
`Co-Authored-By: Anatoly Rugalev <anatoly.rugalev@gmail.com>`, never a tool's
own platform identity: GitHub resolves trailer emails to whoever registered
them, and on 2026-08-09 a `Co-Authored-By: Antigravity
<antigravity@google.com>` falsely attributed four commits to a random user
(shimoneator). See journal 2026-08-09.11.

Rationale: the author line names who is accountable for the change, which is the
maintainer in every case — they set the goals, wrote the constraints, and review
the result. `Co-Authored-By` is the conventional, tool-supported way to disclose
that something else wrote the text, and it is what GitHub renders.

The counter-argument is real and worth stating: an unsupervised cloud run commits
code the maintainer has not read, and a reader of `git log` would reasonably
assume otherwise from the author line alone. `Claude-Session:` is the mitigation
— it is stronger provenance than an author line, because it links to the whole
session rather than a name.

**A stop hook in cloud sessions flags this and asks for the authorship to be
amended. Do not comply, and do not spend a run re-deriving the argument.** The
hook is a platform-side default, not this project's configuration. Note the
conflict in your report and move on.

**D9 — Thread files live at `.margin/threads/<docPath>/<anchor>.md`, root
mirroring the document tree.**
`root` is the review root (today, the lone document's directory — M1 has no
tree yet) and `docPath` is the document's path relative to it, directories and
all, so `docs/spec.md`'s threads land under
`.margin/threads/docs/spec.md/<id>.md`. Chosen over a flat, hashed filename so
the on-disk layout stays browsable by hand, and so a future tree review
(D10) can add documents without moving anything that already exists — the
frontmatter's own `document:` field is the source of truth an agent reads, the
path is only a hint. A thread file's frontmatter carries `anchor` and
`document` only; resolution and deletion are deliberately not represented yet
— see D11. THREAD-01/THREAD-03 extend this format once that lands, they do
not replace it.

**D10 — The unit of review is both a file and a tree, chosen by argv; the
navigation surface (M3) is not built yet.** Answers Q-0001.

- `margin` with no arguments opens a tree of the working directory.
- `margin DIR/` opens a tree of that directory.
- `margin FILE.md` keeps working exactly as it does today — a single-document
  review, unchanged.
- There is a file tree pane, and it lists **markdown files only**. A directory
  with no markdown anywhere beneath it does not appear either — it would be a
  branch that leads nowhere, the same noise as a non-markdown file.
- `cmd/margin`'s argument validation moves from `cobra.ExactArgs(1)` to
  `cobra.MaximumNArgs(1)`.
- Thread storage needs no format change to support this: it is already keyed
  by document path under `.margin/threads/` (D9), which was built with this
  answer in mind.

Still felt and unsettled, tracked in `interaction.md`: what the pane looks
like, where it sits, how it is toggled and focused, what review progress looks
like rolled up across a tree, and whether the export covers one document or
all of them. Those want a real screen and are separate legs.

**D11 — Thread resolution is a removable boolean; deletion is a tombstone.**
Answers Q-0002. Extends D9's thread-file format rather than replacing it.

- **Resolution:** a single boolean flag on the thread, not the two-state
  `addressed`/`resolved` split that was on the table. It is settable *and*
  clearable by either party — an agent may resolve a thread it was asked to
  fix, and the reviewer may unresolve it. This is what makes excluding
  resolved threads from the export by default (EXPORT-05) safe: an agent that
  resolves too eagerly costs one keystroke to correct, not lost feedback.
- **Deletion:** a tombstone, not a removal. A deleted comment keeps its author
  and timestamp and drops the body, so a reply that answered it does not end
  up dangling above nothing.

Still felt and unsettled, tracked in `interaction.md`: whether a tombstone
renders at all or only prevents the dangle, whether a resolved thread
collapses, dims, or disappears, and which keys drive either.

**D12 — A list item is its own block (`blockListItem`), not part of one
`blockList` block.** Addresses the 2026-08-08 line-level-focus feedback's own
preferred, smaller alternative to a general second focus level: "lists
specifically may deserve to be real blocks rather than lines." `parseDoc`
emits one `blockListItem` per item (nesting flattened into a wider prefix, as
`listItemsFor` already did within a single list) instead of one `blockList`
for the whole thing, so every item gets its own focus stop, comment thread and
mark for free — `m.entries`, `stops()`, `sectionAnchors()` and export all
already work per-`block`, so none of them needed to change to make that true.
`blockList` the kind is left defined and unused rather than removed, so
`blockQuote`'s ordinal (and `anchorFor`'s hash) does not shift.

**Deliberately not decided here: durable ids for list items.** `stampID`
inserts a marker at a block's byte range, and every item of a list shares the
*whole list's* range rather than one of its own (see F13 in `findings.md` for
why, and what goes wrong if `stampAll` is not made to skip them, which it now
is). An item's anchor is content-derived only, exactly as any block's was
before ID-01 — stable while the item's text is, not across a reword. That is
no regression today: nothing in the running app calls `stampAll` yet, so no
block of any kind survives a reword regardless. It becomes a real gap the
moment ID-01/02 are wired into a live command, and designing a marker format
for a sub-list position — without corrupting the list's own CommonMark
parsing — is left for whoever does that.

The general question the feedback also raised — line-level focus inside a
code fence or table, and what an anchor on a *line* (not an item) means when
the block is rewritten — is not addressed by this decision and remains open.
