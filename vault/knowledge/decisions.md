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
