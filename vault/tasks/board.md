# Board

Last updated: 2026-08-09 (composer modified keys)

**Active milestone:** M2 — Persistence and the loop
**Needs a look:**
- (feedback fix) thread dive navigation: block-level `j`/`k` walk blocks and
  thread rows only (a thread is one stop whatever its length), `l` dives
  into the focused thread's comments, `h` surfaces, dived j/k pop out at the
  ends — journal 2026-08-09.18
- (feedback fix) mark visuals: per-line ✓/! icons replaced by a vertical
  rule in the mark's colour down each marked block; headings take the rule
  too, partial keeps the dimmed · — journal 2026-08-09.17
- (feedback fix) visual block selection: `V` selects blocks (bg + gutter bar +
  footer mode line), `y` yanks their markdown source; showDeleted moved to
  `T` — journal 2026-08-09.16
- (feedback fix) palette backspace cancels: `:`+backspace closes; staged-seed
  backspace closes like `esc` instead of rewinding — journal 2026-08-09.14
- (feedback fix) new-comment composer shows the thread's comments above the
  editor, dim rule between; edit stays solo — journal 2026-08-09.12
- (feedback fix) deleted comments disappear by default; `V` (thread.showDeleted)
  reveals them with the `[deleted]` marker — journal 2026-08-09.9
- SCROLL-04 (mouse hover effects) — journal 2026-08-09.7
- SCROLL-03 (mouse wheel) — journal 2026-08-09.6
- SCROLL-02 (page and half-page movement) — journal 2026-08-09.5
- CMD-05 (staged value commands and key-opens-a-stage) — journal 2026-08-09.4
- CMD-03 (command palette UI: renders at bottom, 7 items shown, typed filtering) — journal 2026-08-09.3
- EXPORT-04 (export wording and shape, agent instructions) — journal 2026-08-09.2
- THREAD-04 (delete thread/comment with confirmation and tombstone) — journal 2026-08-09.1
- RENDER-05 (heading hierarchy: weight + colour by depth) — journal
  2026-08-08.2
- RENDER-04 (block quotes: rule down the left edge) — journal 2026-08-08.3
- Lists split into per-item blocks, from the line-level-focus feedback
  (D12) — journal 2026-08-08.4
- RENDER-06 (inline markup — bold, code, links — for paragraphs) — journal
  2026-08-08.5
- RENDER-02 (fenced code blocks: chroma/monokai syntax highlighting) —
  journal 2026-08-08.6
- RENDER-03 (tables: aligned columns, dim header rule, no vertical bars) —
  journal 2026-08-08.7
- RENDER-07 (frontmatter: dimmed key/value block ahead of the document) —
  journal 2026-08-08.8
- THREAD-02 (resolved threads: checkmark marker collapsed, badge expanded,
  `R` to toggle) — journal 2026-08-08.9

> A running list of felt legs the maintainer has not judged yet — a log, not a
> gate. Nothing waits on it. Agents append a line with the leg id and its journal
> entry; the maintainer deletes lines as they are judged, and files feedback for
> anything wrong.

## Legend

- `[felt]` — needs the maintainer to look at it. Ends with a demo recipe and a stop.
- `[mech]` — tests can prove it. Proceed on your own judgement.

---

## In progress

- **(feedback fix) J/K incremental viewport scrolling** `[felt]` — add `J`/`K`
  incremental viewport scrolling (3 lines per press) so the user can scroll
  the document viewport without moving focus `m.at`. Drains the
  "Incremental Scroll Keys" bullet of
  `vault/feedback/2026-08-09-todo-review-feedback.md`. (claimed by 1d4c2720, 2026-08-09)

## Backlog — M1

*(none)*

## Backlog — M2

*(none)*

## Backlog — Commands (from feedback, 2026-08-07-command-palette)

Not part of a milestone; cross-cutting infrastructure the palette feedback
asked for. Split per the feedback's own suggested ordering — see
`vault/journal/2026-08-07.6.md` for why CMD-01 alone is this leg.

- *(none)*

## Backlog — M3 (navigation)

Independent of the tree view (D10): these are about moving around one
document, regardless of the order the tree-view work itself lands in.

*(none)*

## Blocked

*(nothing)*

Q-0001 and Q-0002 were answered 2026-08-07 and folded into
`knowledge/decisions.md` as **D10** (tree review) and **D11** (thread
resolution and deletion) in the leg that closed them; the question files are
deleted. See those entries for the settled shape, and `interaction.md`'s "Not
settled" section for what each still leaves open for a felt leg.

## Done

- [x] **(feedback fix)** composer modified-key forwarding: the composer
      silently dropped every modified key vt's SendKey has no case for
      (ctrl+backspace, shift+enter, ctrl+shift+letter, ctrl+delete, modified
      F-keys — the "vim mode doesn't receive ctrl+backspace" half of the
      feedback) and, worse, mangled any alt combo carrying a second modifier
      into a bare ESC — vt prefixes ESC for alt, strips the bit, and its
      fallback then appends nothing — which nvim reads as Escape, the
      "dropping into normal mode unexpectedly" half. `sendKey`'s modifier
      branch is now `encodeModifiedKey` (`internal/review/composer.go`),
      encoding every family in forms fed to a real nvim on a pty and
      confirmed to decode as the intended key (F19): xterm
      `CSI 1;<mod><final>` for arrows/home/end, tilde forms for
      insert/delete/pgup/pgdn/F5–F12, CSI-u for the remaining shift/ctrl
      combos, and legacy meta (ESC + the no-alt encoding) for alt. ctrl-only
      aliases (ctrl+a..z, ctrl+space, ctrl+[) keep vt's legacy control bytes
      so ctrl+m stays CR and ctrl+[ stays ESC. `composerInit` binds `<C-BS>`
      and `<M-BS>` to `<C-W>` so both delete-word combos do what hands
      expect — the `<M-BS>` binding doing double duty, since nvim resolves
      meta keys only when a mapping names them, whatever the encoding.
      Residual, recorded as F19: an *unbound* alt combo still collapses to
      Escape, as in any terminal vim; only a binding or the kitty keyboard
      protocol (unimplemented in x/vt) changes that. Tests pin the exact
      emitted bytes per key family (`TestSendKeyEmitsBytes`), both
      delete-word behaviours end to end against a real nvim, and the raw
      `CSI 127;5u` chain through the real terminal reader. Drains the "vim
      mode key combos" bullet of the todo-review feedback file; the
      multi-line-block-dive residue, .margin location, palette separation,
      J/K scroll, wheel speed and hover visibility bullets remain. `[mech]` —
      see journal 2026-08-09.19 — done 2026-08-09
- [x] **(feedback fix)** thread dive navigation: j/k no longer eat focus on
      threads — the old flat stop list (block → thread row → every comment)
      cost three presses to pass a two-comment thread and stopped twice on a
      one-comment one. Block-level j/k now walk entries only, so a thread is
      exactly one stop whatever its length (`moveFocus`, `internal/review/
      review.go`), and a new explicit dive is the only way j/k reach a
      comment: `l`/`right` (`move.dive`) steps onto the first visible
      comment from the block or its thread row (both name the same anchor,
      the rule `c`/`R`/`D` already use), j/k walk the thread's comments, and
      `h`/`left` (`move.surface`) steps back out to the thread row. Dived
      movement pops out at the ends — j past the last comment lands on the
      next entry, k past the first on the thread row — chosen over clamping,
      which would re-create the eaten press inside the dive. Visual mode's
      entry-filtering special case became dead code (entering visual already
      lifts comment focus) and went; `l` mid-selection is inert. Clicks and
      page landings still dive implicitly via the hit-test; `editFocused`'s
      stale "select a comment with j/k" hint now points at `l`. Diving into
      multi-line blocks deferred: no verb acts on a line today, and the
      L12-18 line-reference work will define what a line stop means — a
      residue bullet records this in the feedback file. Drains the "j/k eats
      focus" and "single-comment thread stops twice" bullets of the
      todo-review feedback file; the vim-mode key combos, .margin location,
      palette separation, J/K scroll, wheel speed and hover visibility
      bullets remain. `[felt]` — see journal 2026-08-09.18 — done 2026-08-09

- [x] **(feedback fix)** mark visuals in the gutter: a marked block no
      longer repeats an icon per line (a six-line reviewed paragraph showed
      six `✓`s — "weird", per the maintainer). `gutter`
      (`internal/review/review.go`) now draws a vertical rule (`│`) in the
      mark's colour on every line of the block, reading as one continuous
      line down its extent — green `114` for reviewed, orange `209` for
      flagged, the colours the retired icons already used. Headings take the
      same rule for a full section roll-up rather than keeping the icons as
      a "summary" marker (rejected: two visual languages in one column); a
      partial roll-up keeps the settled dimmed `·` (interaction.md).
      `reviewMark.glyph()` survives only in the plain-text quit-time
      summary, where `✓`/`!` is the more legible encoding. Widths untouched
      — `gutterW` stays 3, no hit-test or wrap math moves. Drains the "Mark
      Visuals in Gutter" bullet of the additional-UX feedback file; the
      vim-mode background artifacts, comment-focus highlight, CLI
      subcommands and focus-on-composer-exit bullets remain. `[felt]` — see
      journal 2026-08-09.17 — done 2026-08-09
- [x] **(feedback fix)** visual block selection and yank: `V` starts a
      blockwise selection (the maintainer's `Shift+V` ask — a terminal
      delivers it as `V`, which `thread.showDeleted` gave up, moving to
      `T`), anchored where focus sits and derived to wherever focus moves,
      so every motion extends it; `esc`, a second `V`, or any non-movement
      command cancels. Selected blocks paint a dark-slate background
      (`selLine` reasserts it after every inner SGR reset, since lipgloss
      does not reassert an outer style across chroma/inline resets) plus a
      blue gutter bar, with the pink focus bar kept on the moving endpoint
      and a `-- VISUAL -- n block(s)` footer mode line. `y` copies the
      selection's markdown *source* (`blockSource`, export.go — quoteBlock's
      dual without prefix or truncation) to the clipboard via export's
      dual path, or the focused block alone when nothing is selected.
      Thread entries in a range are conversation, not document: never
      highlighted, counted, or yanked. Drains the "Visual Line Selection"
      and "Yank Content" bullets of the visual-mode feedback file; the
      `L12-18:` comment prepending, flexible placement and `gy`/`yr`
      yank-reference bullets remain. `[felt]` — see journal 2026-08-09.16 —
      done 2026-08-09
- [x] **(feedback fix)** sibling-section marking bug: marking one header
      section (the maintainer's "CMD-05", anchor `^ccd3fc`) lit up every
      sibling header as reviewed. The mechanism was anchor collision —
      `anchorFor` derives a content anchor from `sha256(kind\x00text)` only,
      so byte-identical blocks (a TODO document's repeated `*(none)*`
      placeholders) shared one anchor, and one section's mark write was read
      back by every twin's roll-up and counted twice by `reviewProgress`.
      Fixed with a disambiguation pass at the end of `parseDoc`
      (`disambiguateAnchors`, `internal/review/parse.go`): the first
      occurrence keeps the bare anchor (so a pre-existing thread file still
      finds its block), repeats gain `#2`/`#3` ordinal suffixes — a shape
      `anchorFor` never derives, so no collision with a real anchor is
      possible. Stamped ids and the on-disk formats are untouched; the pass
      rewrites only session-local content-derived anchors, and only the
      colliding ones. Drains the "Marking Bug" bullet of the todo-review
      feedback file. `[mech]` — see journal 2026-08-09.15 — done 2026-08-09
- [x] **(feedback fix)** palette backspace cancels the palette: `:` then
      backspace now closes the palette (the `:` is "erased"), and backspace
      at a staged command's seed (`mark ` / `goto `) closes like `esc`
      instead of rewinding to the bare command list — the same want stated
      in two inbox files, the palette-backspace-cancel file and the
      todo-review file's "Backspace Rewind" bullet, both drained here.
      Typed characters still delete one by one, and inside a value stage a
      typed value character deletes before the bare-seed backspace cancels,
      so editing text never closes anything. One uniform rule in
      `handlePaletteKey` (`internal/review/review.go`): backspace cancels
      exactly where there is nothing left to edit — applied whether the
      seed came from the `m`/`s` keys or from typing `:mark ` by hand, a
      distinction the model does not record and the feedback does not ask
      for. `[felt]` — see journal 2026-08-09.14 — done 2026-08-09
- [x] **(feedback fix)** ephemeral stdin reviews: `margin -` (or `--stdin`)
      reads the document from stdin and reviews it with no persistence — no
      thread files are read or written (the store stays nil, so saves are
      no-ops, and no watcher starts) and `--stdout` is implied, the review
      printing to stdout on quit. stdin is a pipe, so the interface runs on
      the controlling terminal for *both* streams: one O_RDWR `/dev/tty`
      handle feeds `tea.WithInput` and `tea.WithOutput` alike (recorded as
      F17). `margin -` with a terminal on stdin is rejected up front with a
      "pipe something in" error rather than swallowing keystrokes until EOF.
      The export's agent instructions gain an ephemeral variant — "nothing
      was saved, there are no thread files to reply in" — since pointing an
      agent at `.margin/threads/stdin/` would send it writing files nothing
      will ever read; the file-backed wording is unchanged. `[mech]` — see
      journal 2026-08-09.13 — done 2026-08-09
- [x] **(feedback fix)** adding a comment to a thread shows the thread: the
      composer box for a *new* comment now renders the thread's visible
      comments (and the resolved badge, when set) above the emulator, with a
      dim full-width `─` rule marking where reading stops and writing starts,
      instead of swapping the whole thread for the editor. The comment loop
      of `threadLines` is extracted into `appendComments` (`review.go`),
      shared with the expanded view, so a comment reads, wraps and hit-tests
      identically in both — tombstones stay hidden unless `V` is on, through
      the same `visibleComments` filter. A new render-pass side channel
      `m.paneLead` records how far the emulator's first row moved down; `View`
      folds it into `paneTop`, so mouse routing, wheel routing and the
      hardware cursor keep pointing at the editor. Comments above the editor
      keep their subspans: clicking one while composing blurs and lands focus
      on it, the same rule clicking away already followed. Editing (`e`)
      keeps the composer-only box — the text being edited is already live in
      the emulator — and a fresh or draft-only thread renders exactly as
      before. `[felt]` — see journal 2026-08-09.12 — done 2026-08-09
- [x] **(feedback fix)** ctrl+enter in the composer: the handler was already
      correct; what was missing was a test pinning the decode half of the chain.
      `TestCtrlEnterDecodesThroughRealReader` feeds the raw kitty `CSI 13;5u`
      sequence through the same `uv.TerminalReader` bubbletea v2 uses, asserts it
      decodes to `{Code:13, Mod:ModCtrl}`, routes it through `handleKey` into the
      live composer and asserts a submit. Verified end-to-end against the real
      binary in a pty. The maintainer's report answers the 08-06.3 open question:
      on their terminal the modifier is not reported (see F16), which no margin
      change can fix. `[mech]` — see journal 2026-08-09.10 — done 2026-08-09
- [x] **(feedback fix)** deleted comments disappear by default: tombstones no
      longer render a `[deleted]` placeholder in the expanded thread or the
      collapsed summary. New `thread.showDeleted` command, bound to `V` (and
      palette `:` + "show"), reveals them still marked `[deleted]` — the
      tombstone replaced the body on disk, so provenance is all there is. The
      focus stop list and subspans filter through the same `visibleComments`
      list as the renderer, so a hidden deleted comment cannot be focused,
      clicked or counted; toggling `V` off while standing on one drops focus to
      the thread entry. A thread whose comments are all tombstones renders
      `comments deleted — V to reveal` instead of `no comments yet`. `[felt]` —
      see journal 2026-08-09.9 — done 2026-08-09
- [x] **(feedback fix)** composer wrapping misalignment: the box rendering the
      composer emulator was declared `Width(w-2*borderW)`, but lipgloss's
      `Width` includes the border, so its content area was two columns narrower
      than the emulator — lipgloss re-wrapped the emulator's already-wrapped
      rows, orphaning words and pulling the cursor off the text. Now `Width(w)`
      so the content area exactly matches the emulator. `[mech]` — see journal
      2026-08-09.8 — done 2026-08-09
- [x] **SCROLL-04** mouse hover effects: `tea.MouseMotionMsg` drives `m.hoveredEntry` via `hitTest`, rendering a dim `▌` in the gutter for the block under the pointer without moving `m.at`. `[felt]` — see journal 2026-08-09.7 — done 2026-08-09
- [x] **SCROLL-03** mouse wheel support: scrolling the wheel shifts the document viewport directly (`m.scroll`), bypassing `clampScroll`'s snapping. Focus intentionally stays left where it is. A wheel event over an open composer scrolls the comment instead. `[felt]` — see journal 2026-08-09.6 — done 2026-08-09
- [x] **SCROLL-02** page and half-page keys (`ctrl+d`/`u`, `pgup`/`pgdn`, `home`/`end`). Built to carry focus along with the viewport, matching vim's `ctrl+d` rather than a generic pager. `[felt]` — see journal 2026-08-09.5 — done 2026-08-09
- [x] **CMD-05** staged value commands and key-opens-a-stage. `mark` and `goto` staged commands added. `m` and `s` keys open the palette partway through. Backspace rewinds predictably. `[felt]` — see journal 2026-08-09.4 — done 2026-08-09
- [x] **CMD-03** the palette itself: `:` opens it, rendered at the bottom of the screen without dimming the document, showing up to 7 ranked items, supporting typed filtering and navigation. `[felt]` — see journal 2026-08-09.3 — done 2026-08-09
- [x] **EXPORT-04** agent instructions added to the export preamble, and locator includes the anchor ID `## file:line (^id)` so agents can successfully read/write thread files and resolve threads without guessing. `[felt]` — see journal 2026-08-09.2 — done 2026-08-09
- [x] **THREAD-04** what deletion looks like and what confirms it: D bound to
      thread.delete, requiring a second press to confirm. Deleted comments
      render their body as `[deleted]` with a dim style, both in expanded and
      collapsed views. `[felt]` — see journal 2026-08-09.1 — done 2026-08-09

- [x] **(feedback fix)** inline markup inside list items and quotes:
      `wrapList`/`wrapQuote` (`internal/review/review.go`) now wrap through
      `wrapInline` instead of the plain `wrap()`, so `**bold**`, `` `code` ``
      and `[links](url)` inside a list item or a block quote render the same
      way RENDER-06 already made them render inside a paragraph, instead of
      showing their raw markup characters. Not a new visual decision —
      RENDER-06 already settled the styling; this closes the gap its own
      journal entry flagged as unfinished. `[mech]` — see journal
      2026-08-08.10 — done 2026-08-08
- [x] **THREAD-02** what resolving looks like: a new `thread.resolve` command
      (`internal/review/command.go`), bound to `R`, toggles `t.resolved` (D11)
      on the thread anchored to the focused block — reachable whether that
      thread is currently collapsed or expanded, since Applicable only
      requires a thread to exist, the same test `comment.edit` already uses.
      `toggleResolved` (`internal/review/review.go`) persists the flip
      immediately through `m.store.save`, the same path a posted comment
      takes in `dismiss`, rather than waiting for some other save point.
      Visually: a resolved thread's **collapsed** line swaps the plain dim
      `│` rule for a dim green `✓` (new `resolvedTxt` style) without touching
      the draft/pending-edit colour underneath it, since unsaved text stays
      the more urgent signal; a resolved thread **expanded** gets a two-line
      `✓ resolved` badge ahead of the comments rather than the comments
      dimming, collapsing, or disappearing — resolving says "handled", not
      "no longer matters", and the conversation is still what the reviewer
      came to read. `resolveTarget` (`command.go`) names the palette action as
      "resolve" or "unresolve" rather than a bare toggle. This was the only
      remaining M1 item's neighbour still sitting in M2's backlog after
      RENDER-07 emptied M1 — the board's active milestone moves to M2 with
      this leg. `[felt]` — see journal 2026-08-08.9 for the demo recipe —
      done 2026-08-08
- [x] **RENDER-07** frontmatter's visual treatment: a new `renderFrontmatter`
      (`internal/review/review.go`) draws a `blockFrontmatter` block as a
      dimmed key/value block ahead of the document, picking the maintainer's
      middle candidate over hidden-on-demand or a collapsed line that
      expands — it needs no new key or interaction state to see, unlike
      either of the others. `frontmatterFields` (`internal/review/parse.go`)
      strips the `---` fences from the block's raw text and returns the
      inner lines with their own indentation kept, so a nested YAML value
      (a list, say) still reads correctly. It stays out of `m.entries`
      entirely — `rebuild` still filters it, unchanged from PARSE-03 — so it
      remains not focusable, not commentable, not markable; `render()` draws
      it directly against `m.doc[0]` ahead of the entry loop instead. A field
      wider than the measure is truncated with an ellipsis rather than
      wrapped, since frontmatter is key: value pairs, not prose — a wrapped
      continuation line would misread as a second field. This was the last
      item in M1's backlog, which is now empty. `[felt]` — see journal
      2026-08-08.8 for the demo recipe — done 2026-08-08
- [x] **RENDER-03** render tables: a new `blockTable` kind (`internal/
      review/document.go`, `parse.go`) replaces the old verbatim `blockRaw`
      treatment, carrying a parsed `*tableBlock` (header, rows, per-column
      alignment) built by `tableFor` from goldmark's `*eastast.Table`.
      `renderTable` (`review.go`) lays it out as a bold header row, a dim
      `─` rule, then aligned body rows — two-space column separation, no
      vertical bars, per RENDER-04's precedent that markdown's own delimiter
      (`|`, like `>`) is source syntax, not a rendering. Column widths
      narrow evenly toward a 3-rune floor (`tableColumnWidths`) when the
      natural layout would exceed the measure, rather than truncating one
      column to nothing. A cell's inline markup is stripped to plain text
      via `cellText` (RENDER-06's `parseInline`, reused unchanged) but not
      styled — styling would tangle column-width math with ANSI byte
      counts, deferred rather than solved. Colours resolve against `body`
      (`textStyle`/`reviewedTxt`) so a reviewed table dims like any other
      prose block; export's `quoteBlock` reproduces the original `|` source
      verbatim, unchanged from the old `blockRaw` path. `[felt]` — see
      journal 2026-08-08.7 for the demo recipe — done 2026-08-08
- [x] **RENDER-02** fenced code blocks: a new `blockCode` kind (`internal/
      review/document.go`, `parse.go`) replaces the old verbatim `blockRaw`
      treatment, carrying the fence's language (`t.Language(src)`) and
      content lines read straight from goldmark's own `Lines()` — which
      excludes the ``` delimiters, unlike `extent()`'s widened range used for
      anchoring/stamping — so the highlighter and export both work on the
      code alone. `highlightCode` (`review.go`) runs it through
      `chroma/v2/quick.Highlight` (`"go get github.com/alecthomas/chroma/v2"`
      — new dependency) with the `monokai` style at `terminal256` depth,
      returning pre-styled lines the shared render loop's existing
      `preStyled` path (RENDER-06) already knows how to draw without
      re-wrapping them in a uniform style. An unrecognised or empty language
      falls back to chroma's own guess-then-plaintext path rather than
      erroring, so nothing on screen breaks for a language chroma does not
      know. Export's `quoteBlock` gained its own `blockCode` case,
      reconstructing a real fenced quote (```` ```lang ```` … ` ``` `) instead
      of a flattened blockquote, so an agent reading the export still sees
      real code. Highlighting is fixed regardless of review state, same
      reasoning RENDER-06 applied to inline code and links: markup is
      markup, not prose that dims. `[felt]` — see journal 2026-08-08.6 for
      the demo recipe — done 2026-08-08
- [x] **RENDER-06** inline markup for paragraphs: a hand-rolled scanner
      (`parseInline`, `internal/review/parse.go`) recognises `**bold**`,
      `` `code` `` and `[text](url)`, stripping the markup and tagging each
      run; `glueWords`/`wrapInline` (`review.go`) wrap paragraphs the same
      way `wrap` always has but keep adjacent styled fragments glued into one
      word (so `en**hanced**` cannot pick up a space the source never had)
      and render each already-styled, so `render()`'s shared block loop skips
      re-styling a `preStyled` line rather than nesting styles. List items,
      quotes and headings are deliberately not covered yet — `wrapInline` is
      now the reusable primitive for extending them. `[felt]` — see journal
      2026-08-08.5 for the demo recipe — done 2026-08-08
- [x] **RENDER-01** render lists: shipped in two parts, neither originally
      filed under this id. Wrapping and hanging indent landed under the
      2026-08-08 rendering-bugs feedback fix; splitting each item into its
      own focusable, commentable, markable block (`blockListItem`, D12)
      landed 2026-08-08 addressing the line-level-focus feedback. "One block
      type, then stop and ask" — the task's original framing — is
      superseded by D12's per-item split; marked done rather than rewritten,
      since the actual want (lists render, and now render well) is met.
      `[felt]` — see journal 2026-08-08.4 for the demo recipe — done
      2026-08-08
- [x] **RENDER-04** block quotes: a new `blockQuote` kind (`internal/review/
      document.go`, `parse.go`) replaces the old verbatim `blockRaw`
      treatment. `quoteLinesFor` strips each line's `>` marker; `wrapQuote`
      (`review.go`) re-wraps the prose to the measure, preserving
      paragraph breaks; render prepends a dim `"│ "` rule instead of the
      source's `>`. Export's `quoteBlock` reuses the stripped lines to
      reconstruct clean markdown on the way back out. Nested quotes and a
      list/fence inside a quote deliberately left unhandled, per the
      feedback's own "not worth designing for now". `[felt]` — see journal
      2026-08-08.3 for the demo recipe — done 2026-08-08
- [x] **RENDER-05** heading hierarchy: `headingStyle(level)` (`internal/
      review/review.go`) replaces the single `headStyle` with three,
      indexed by depth — bold/colour 212 at level 1, plain colour 183 at
      level 2, dim 245 at level 3+ — clamped so a level-4+ heading falls
      back to the deepest style rather than panicking. Confirmed first that
      a real parsed heading's `block.text` already excludes the markdown
      `#` markers (goldmark's `Heading.Lines()` covers content only), so
      only the styling was missing, not syntax-stripping. Left-padding
      indentation and a gutter level hint — the feedback's other two ideas
      — deliberately not attempted: both cost room the feedback itself
      flags as needing its own look. `[felt]` — see journal 2026-08-08.2 for
      the demo recipe — done 2026-08-08
- [x] **PARSE-02** reviewed 2026-08-08 — a real document reads well at this
      measure and the loop holds up on prose the reviewer did not write. Cleared
      with defects filed rather than fixed inline: see the 2026-08-08 feedback
      for tables, list truncation, heading hierarchy, block quotes and
      line-level focus.
- [x] **EXPORT-05** resolved threads excluded from the export by default,
      `--include-resolved` to bring them back (`internal/review/export.go`):
      `exportReview` gains an `includeResolved bool` parameter; an item stays
      hidden when its thread is `resolved` and `includeResolved` is false,
      *unless* the block is also flagged — the flag is an independent signal
      and does not retract just because the thread on it is marked done. The
      withheld count is reported the same way drafts already are (`_N
      resolved thread(s) not included._`), and the all-clear message
      distinguishes "everything was resolved" from "nothing was ever said"
      (`Nothing outstanding` vs. `No comments and nothing flagged`) so the
      count above it is never contradicted by the body. A restored item is
      marked `— resolved` in its header, mirroring the existing `—
      flagged, needs attention` convention. Wired through `model.
      includeResolved` (false on every test-built model) to both `Y` and
      `--stdout`, and a new `--include-resolved` flag on `cmd/margin` sets it
      — done 2026-08-07
- [x] **THREAD-03** delete a comment, and a whole thread, as a tombstone
      (`internal/review/document.go`): `comment` gains a `deleted bool`;
      `deleteComment(i)` blanks `posted[i].body`, sets `deleted`, and drops any
      pending edit draft for it — editing a now-empty comment makes no sense;
      `deleteThread()` gives "delete a whole thread" (D11) the same treatment
      applied to every posted comment, since nothing else per-thread needs its
      own deleted flag. `marshalThread`/`parseThreadFile` (`store.go`) carry it
      through a `tombstoneMarker` (`*deleted*`) written in place of the body
      under the untouched `## author — timestamp` header, so a deleted
      comment's provenance still round-trips and reads plainly to a human or
      agent opening the file directly. `reloadThreads` needed no change — it
      already replaces `posted` wholesale on every reload, tombstones included.
      No command, no key, no rendering: nothing in the registry calls either
      method yet, and no renderer branches on `deleted` — that is THREAD-04 —
      done 2026-08-07
- [x] **THREAD-01** resolvable threads: `thread` gains a `resolved bool`
      (`internal/review/document.go`), settable and clearable by either party
      per D11 — no dedicated setter, since a plain field is all "either the
      reviewer or the agent can flip it" needs. `marshalThread` writes
      `resolved: true` only when set, so an unresolved thread's file is
      byte-for-byte what it was before this leg; `parseThreadFile` reads it
      case-insensitively and defaults every pre-existing file (no field at
      all) to unresolved. `reloadThreads` (watch.go) now merges the flag on
      an externally-edited file the same way it already merged an appended
      reply, so an agent resolving a thread by hand-editing its file is
      noticed without a restart. No command, no key, no rendering — that is
      THREAD-02, still felt and still ahead — done 2026-08-07
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
