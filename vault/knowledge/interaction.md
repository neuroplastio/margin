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

- It is a real `nvim` child, stripped to a textarea: no statusline, ruler, line
  numbers, sign column, `~` filler, or `cmdheight`. In a ten-row pane that chrome
  costs a third of the visible text.
- The buffer holds **the comment text and nothing else** — no instructions, no
  quoted context. The block is already on screen above the pane and the keys are
  in the footer.
- **Insert mode iff the buffer is empty.** A new comment wants typing; existing
  text — an edit, or a resumed draft — wants normal mode, so the first keystroke
  is a motion.
- The mode indicator lives in the host footer, inferred from the cursor shape the
  child reports via DECSCUSR. nvim's own `-- INSERT --` is off to save a row.
- Resuming a draft puts the cursor **after** the existing text, not before it.

### Dismissal

| Gesture | Outcome |
| --- | --- |
| `ctrl+s` · `ctrl+enter` · `ZZ` · `:wq` · `:x` · `SPC c c` | Submit |
| `esc esc` · `:q` · `SPC c d` · losing focus | Keep as a draft |
| `:q!` · `SPC c k` | Discard |

- **Keeping is the default.** Walking away from half-written text keeps it;
  discarding must be asked for. This mirrors the GitHub review model.
- Nothing ever prompts. `:q` on a modified buffer must not error or ask —
  `cnoreabbrev` maps it onto the draft path.
- Every one of these is an nvim mapping or ex command, **not** a key the host
  intercepts. The pane keeps sole ownership of input, so there is no prefix key.
- `ctrl+\` and `ctrl+enter` are the only keys the host takes. `ctrl+\` exists so
  a wedged child cannot trap the user. `ctrl+enter` is there because it cannot
  work any other way: a terminal sends CR for both enter and ctrl+enter, so nvim
  cannot distinguish them — we only can because Ghostty speaks the Kitty
  keyboard protocol. It resolves to the same ex command as every other submit
  gesture, so there is still one submit path.
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
  look. What review progress looks like rolled up across a tree, and whether
  the export covers one document or all of them, are still open.
- Whether marks and comments belong in the same gutter column.
