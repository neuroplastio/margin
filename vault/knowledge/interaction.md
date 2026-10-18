# Interaction contract

**Binding.** Everything in this file has been judged by the maintainer on a real
screen and is settled. Do not change it without feedback saying so.

**If a behaviour is not in this file, it is not settled.** Do not invent one —
raise it in [`../questions/`](../questions/) and stop. Adding to this file is the
*result* of a felt leg being reviewed, never the opening move of one.

---

## Settled

### Document navigation

| Key | Behaviour |
| --- | --- |
| `j` / `k` | Move focus between blocks, and between comments inside a focused thread |
| `g` / `G` | First / last block |
| `c` | New comment on the focused block |
| `e` | Edit the focused comment or draft |
| `space` | Cycle the mark: unmarked → reviewed → flagged |
| `r` / `f` | Set reviewed / flagged directly, toggling off on a re-press |
| `R` | Resolve or unresolve the focused thread |
| `D` | Delete the focused thread or comment (asks for confirmation) |
| `Y` | Copy the whole review to the clipboard |
| `q` | Quit |

On a heading, the mark keys apply to the whole section.

- `r` and `f` toggle: pressing the same mark again clears it. `space` is the
  key for going down a document deciding about each block in turn.
- A **partially** marked section resolves to its roll-up on the first `space`,
  so one press makes it consistent rather than jumping somewhere unexpected.
- **Headings can be commented on** — "this whole section is wrong" is a real
  comment and belongs on the heading — but still carry no mark of their own.
- `c` **always** starts a new comment. `e` **never** creates one. Conflating them
  is what made editing a seeded comment silently append a reply.
- The mouse **only moves focus**. Opening an editor is always an explicit `c` or
  `e`. Clicking inside a live composer positions the cursor in it.

### The screen

- margin runs in the **alternate screen**. It owns the terminal while open and
  hands it back untouched — scrollback and all — on exit, leaving no trace of the
  review in your shell history.
- The frame fills the terminal exactly. Rendering inline cost two rows to chrome
  nobody asked for: the shell prompt kept one above the document, and a second
  had to be left empty to stop the frame scrolling the terminal.

### Threads

- A thread renders in one of three states, all from the same block list:
  **collapsed** to a single line, **expanded** for reading, or **hosting the live
  editor**.
- Collapsed is *literally one line*, with no border. A bordered one-liner costs
  three rows, and with a thread every few paragraphs that crowds out the prose.
- Comments inside an expanded thread are individually focusable.

### The composer

The maintainer replaced the nvim composer with hotty-a2ui's text field
(2026-10-18, D22). What carried over from the nvim composer, and stays
settled:

- The field holds **the comment text and nothing else** — no instructions, no
  quoted context. The block is already on screen above the pane and the keys are
  in the footer.
- Resuming a draft, or editing a comment, puts the cursor **after** the existing
  text, not before it.

### Dismissal

| Gesture | Outcome |
| --- | --- |
| `ctrl+s` · `ctrl+enter` | Submit |
| losing focus (a click away) | Keep as a draft |

- **Keeping is the default.** Walking away from half-written text keeps it;
  discarding must be asked for. This mirrors the GitHub review model.
- Nothing ever prompts.
- `ctrl+enter` needs a terminal that tells it from enter (the Kitty keyboard
  protocol, as Ghostty speaks it); a plain terminal sends CR for both, and
  there `ctrl+s` is the submit key. Every submit gesture is one submit path.
- A plain `enter` is always a newline.

### Export

- `Y` copies the review as markdown: a header, the progress summary, then one
  section per commented or flagged block, each naming its anchor and quoting the
  block so an agent can locate it.
- Blocks that are **flagged with no comment** are included — "needs attention" is
  itself the feedback. Blocks that are reviewed and silent are omitted.
- **Drafts are excluded**, since unsubmitted means unsubmitted, but their count
  is reported so nothing goes silently missing.
- `margin --stdout FILE.md` runs the review interactively as normal, and on quit
  writes the same content `Y` produces to stdout instead of the clipboard — same
  `exportReview` call, same code path, no separate "already reviewed" mode. The
  TUI itself draws on the controlling terminal (`tea.WithOutput` → `/dev/tty`), so
  the pipe carries only the review, ANSI-free, and is safe to hand straight to an
  agent: `margin --stdout spec.md | agent -p "address this review"`.
- **Resolved threads are excluded from the export by default** (D11's
  export-safety rationale), for both `Y` and `--stdout` — the export is a list
  of what still needs doing, not a transcript of everything ever said. The
  withheld count is reported (`_N resolved thread(s) not included._`), the
  same treatment drafts already get, so nothing goes silently missing.
  `--include-resolved` brings them back, marked `— resolved` in their header,
  for an agent (or reviewer) that wants the full history. A block that is
  **flagged** is shown regardless of its thread's resolved state — the two are
  independent signals, and "needs attention" does not retract just because the
  conversation on it is marked done.

### Review marks

- Marks live **per paragraph anchor**. A heading has no mark of its own; it rolls
  up its section, so the two can never disagree.
- A flagged paragraph dominates the roll-up: a section is not clean while one
  block still needs attention.
- Partial coverage renders as a dimmed `·`.
- Reviewed prose is dimmed, so unreviewed text is what draws the eye.

---

## Not settled — do not guess

These have been raised but not judged. Each needs a felt leg and a review.

- **The A2UI composer's own keys and look** (D22, built 2026-10-18, journal
  2026-10-18.2): a single `esc` keeps a draft (there is no insert mode to
  leave first), `ctrl+g` discards (`ctrl+d` and `ctrl+k` edit text on macOS),
  `shift+enter` is a newline like `enter`, `tab` walks to the Submit / Keep
  draft / Discard buttons and round again, and the field is 3–8 rows. In
  cells the field does not soft-wrap a long line — it scrolls sideways (a kit
  gap); on a HOTTY host it is an HTML textarea, which wraps.
- **The rest of the screen as A2UI panes** (D22): the palette as a table under
  a `command · does · key` header, the query's field below it; the search
  prompt's field; the footer's help line (`j move • c comment • …`) with the
  progress and status in caption text, without the old green and orange; the
  tree pane as a folding tree with `▸` on the open file and the progress after
  the name; the threads view as a table whose selected row is reversed, a
  click selecting a row; a thread's comments drawn as Markdown, a head line
  over each.

- What a collapsed thread line should carry. Currently author, truncated first
  line, `(+N)`. Reply count? Unresolved state? Nothing at all?
- Whether the other comments in a thread should stay visible while one of them is
  being edited. Currently the editor replaces the whole thread box.
- Rendering of anything beyond headings and paragraphs: lists, tables, code
  blocks, block quotes, images.
- Scroll behaviour beyond keeping focus on screen. No explicit scroll keys, no
  wheel. Whether paging carries focus with the viewport is the live question —
  see SCROLL-02 and SCROLL-03 on the board.
- The document-tree view when reviewing a directory: built as a left pane
  (journal 2026-08-11.1) — width computed from its rows and clamped, `tab`
  toggles focus, `j`/`k` move, `enter`/`l` opens the focused file, dirs are
  dimmed headers, the open file carries a `▸` and focus a `▌`. It is **not
  settled**: those choices were the leg's best guess and need the maintainer's
  look. Cross-document link following (`ctrl+]` on a link naming another file
  in the tree opens it, journal 2026-08-11.2) is **not settled** either: the
  switch-document-on-follow semantics, the root-relative `/x` spelling, the
  jumplist restarting on a switch, and whether single-document reviews ever
  follow are all the leg's best guess. The threads view (`i`, first built as the
  cross-document inbox, journal 2026-08-11.3, generalized to single-document
  reviews 2026-08-12) is **not settled** either: it replaces the document
  column with the threads under review — every thread across the tree, newest
  first, in a tree review; the current document's threads in a single-document
  review — `enter` jumping to the thread's document and block, a thread whose
  block vanished listed `[block gone]`. Whether that is the right surface,
  whether it should refresh live, what a row should carry, and whether clicks
   should select rows are the leg's best guess. Review progress across a tree —
   built 2026-08-11 (journal 2026-08-11.4): marks are carried per-document
   across switches, each pane row shows its file's `reviewed/total` (green when
   the whole file is reviewed, orange `!` when flagged, blank for untouched
   files), and the footer rolls the whole tree up (`tree N/M reviewed`, with
   every file's blocks — opened or not — in the denominator). **Not settled:**
   whether that is the right surface, whether untouched files should read `0/n`
   instead of blank, and whether the footer should show the open document's
   numbers alongside the tree's. Whether the export covers one document or all
   of them is still open.
- Whether marks and comments belong in the same gutter column.
