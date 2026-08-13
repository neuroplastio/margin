# Board

Last updated: 2026-08-13 (import comments from the current GitHub PR)

**Active milestone:** M4 — Round two
**Needs a look:**
- (PR comment import) `margin import-pr FILE.md` imports the current branch's
  GitHub pull request review comments on that file as ordinary margin threads
  — gh detects the PR (`gh pr view`), `gh api --paginate` fetches its review
  comments, each comment attaches to the block its line falls in (content
  anchor, quoted text), re-imports skip on author+body, PR-level conversation
  comments are left alone, and comments on other files or no-matching-line
  comments are reported not dropped — whether line-based mapping lands
  comments on the right blocks, whether the summary readout is enough, and
  whether the TUI should gain a `:import-pr` palette action that calls it —
  journal 2026-08-13.1
- (mermaid subgraph layout) subgraph diagrams now render each root subgraph
  in its own band of the grid — the maintainer's three-subgraph reproduction
  draws as three disjoint frames with every node inside its own frame, and
  graphs without subgraphs render byte-identically (delta D10 in the vendored
  graph package) — whether the three bands read as one diagram or as three
  boxes taped together, whether the ~180-column width is tolerable with H/L
  scroll or the bands should stack vertically instead, and whether the one
  cross-band edge label (`drop list`) still straddling a frame border is
  acceptable or worse than no label — journal 2026-08-12.5
- (threads view & comment hop) `i` now opens the threads view in
  single-document reviews too: the current document's threads, newest first,
  with a thread whose block has vanished marked `[block gone]` and reported on
  enter instead of dropped; `]`/`[` hop focus to the next/previous thread in
  the document, centred and jumplist-recorded, reporting at the ends — whether
  the list is the right surface for a single document, whether `[block gone]`
  is the right marker (should the orphan's full text be reachable?), whether
  `]`/`[` are the right hop keys and not-wrapping is right, and whether
  dropping the doc name from single-doc rows is right — journal 2026-08-12.3
- (composer shift+enter) `shift+enter` inside the composer inserts a line
  break instead of exiting keeping a draft — the host intercepts
  `{Code: KeyEnter, Mod: ModShift}` and forwards the same `\r` a bare enter
  gets (the ctrl+enter-intercept shape), because forwarding the CSI-u `<S-CR>`
  was unreliable: nvim 0.12.4 dropped it silently and the maintainer's nvim
  turned it into the draft exit (the sequence's lone ESC in normal mode IS the
  mapped draft-exit) — whether shift+enter now reads as a line break, whether
  the exit the maintainer saw is gone on their nvim, and whether
  `interaction.md`'s "ctrl+\ and ctrl+enter are the only keys the host takes"
  should name shift+enter too (F26) — journal 2026-08-12.2
- (dive line marks) `space`/`r`/`f` inside a block dive now mark the focused
  source line, not the whole block — a diveable block's (table, raw) review
  state IS the roll-up of its lines, so each marked line renders its own
  gutter rule (a partly-worked table shows exactly the rows that are done),
  block-level marking sets every line, a partial table reads `·` in its
  heading's roll-up and counts neither way in progress, a flagged line flags
  the block, raw mode and the export follow, and line marks survive a tree
  switch — whether per-source-line marking is the right unit, whether a
  partly-marked table reads clearly at a glance, whether "marking the whole
  block = a mark per line" is right vs a separate block state, and whether
  the tree pane treating a partial table as not-yet-reviewed is acceptable —
  journal 2026-08-12.1
- (mouse hover lag) hover is throttled to the renderer's frame period: a
  motion report inside one 8.33ms frame window of the last one processed is
  dropped, as is any report restating the cell the pointer already occupies,
  so a pointer sweep no longer floods the event loop with a full document
  render per cell and keystrokes no longer wait for the queue to drain —
  whether the lag is gone and the hover still tracks the cursor smoothly,
  whether the ≤1-frame trailing bound the throttle costs is acceptable, and
  whether motion over an open composer and in a tree review still feels right
  — journal 2026-08-11.7
- (thread & large-comment navigation) `j`/`k` on a comment taller than the
  viewport scroll the viewport 3 lines at a time and never move focus — the
  same treatment a tall block gets, reading each comment's `m.subspans` span —
  moving on only at the comment's far edge, so a long thread is no longer
  accidentally left mid-read; landing on a tall comment opens at the entering
  edge (head walking down, bottom walking up), `dive()` opens a tall first
  comment at its head (not the tail `clampScroll` would pin), and the dive's
  flow-out at the ends is unchanged but now fires only after the edge comment
  has been read — whether 3 lines is the right step for a very long comment,
  whether the entering-edge landing is right, and whether leave-only-at-the-edge
  is the wanted reading of the jarring-exit complaint — journal 2026-08-11.6
- (draft comments deletable) `D` on a draft discards the draft instead of
  tombstoning what is underneath: an unsaved edit reverts the comment to its
  posted text, a half-written new comment is discarded (a thread that held
  nothing but the draft is dropped, focus back on the block), and a posted
  thread keeps its comments; the palette names the draft
  (`draft` / `unsaved edit to comment by …`) — whether discarding (vs
  tombstoning, vs prompting) is the wanted reading of "when a comment is a
  draft, `D` should delete it", and whether dropping a draft-only thread is
  right — journal 2026-08-11.5
- (tree review progress) each pane row carries a right-aligned `reviewed/total`
  slot — green when the whole file is reviewed, orange with a trailing `!`
  when any block is flagged, dim when partially reviewed, blank for files
  nobody has touched; marks are carried per-document across switches (they
  used to reset); and the footer reads `tree N/M reviewed · F flagged` across
  every file's blocks, opened or not (the spec's blocks count while you sit in
  the README) — whether per-file `n/m` in the pane is the right surface (vs a
  pane summary, or `0/n` on untouched files), whether the colour rules read at
  a glance, whether the footer should show the open document's numbers
  alongside the tree's, and whether carrying marks per-document is the wanted
  change (it replaces the reset-on-switch of the tree's first build) —
  journal 2026-08-11.4
- (comment inbox) `i` in a directory review replaces the document column with
  every thread across the tree, newest activity first — `enter`/`l` opens the
  thread's document (the pane-open path) and lands centred on its block,
  `ctrl+o` walks back, `esc`/`h`/`tab` return, j/k/g/G move, the wheel
  scrolls the list, and the tree pane stays rendered beside it; a thread
  whose document left the tree stays, dimmed `[doc not in tree]`, and is
  reported on enter; resolved threads render dimmed with `✓`; the list
  refreshes on open (no live tree-wide watcher yet), clicks in the inbox
  column are inert, and `i` is unbound in single-document reviews — whether a
  whole-column view is the right surface, whether `i` is the key, whether the
  tree pane should stay visible, whether resolved threads should dim, whether
  the inbox should refresh live, and whether the enter landing (open +
  centre, like a link follow) is the jump expected — journal 2026-08-11.3
- (cross-document links) `ctrl+]` on a link naming another markdown file in a
  directory review opens it — `[a](other.md)` or `[b](other.md#section)`, the
  path resolved relative to the current document (`/x` = review root);
  in-document `#fragment` links unchanged; a file outside the tree, a
  non-markdown file or an absolute URL still reports "links here point outside
  this document"; a link back at the current document resolves its fragment
  without switching; a fragment naming no heading is surfaced — whether
  following a relative link should switch documents at all vs staying, whether
  a switched-to doc's session should reset (it does, like a pane open), whether
  root-relative `/x` belongs, and whether single-document reviews should follow
  too (they don't yet) — journal 2026-08-11.2
- (directory review) `margin DIR/` / bare `margin` opens a left file-tree pane
  of the markdown files beneath the dir (dimmed dir headers for dirs with
  markdown under them, files indented); `tab` toggles focus between pane and
  document, `j`/`k`/`g`/`G` move through files, `enter`/`l` opens the focused
  file (threads/store/watcher re-point, marks+scroll reset — a fresh review),
  `esc`/`h`/`tab` returns to the doc, `q` quits; open file `▸`, focused row
  `▌`, click selects; hidden dirs skipped; export still covers one document —
  whether the pane belongs left, whether `tab` is the focus key, whether dir
  headers should be focus stops/collapsible, whether open should land you in
  the doc or stay in the pane, and whether switching should carry marks and
   scroll per-document — journal 2026-08-11.1
- (link navigation) `ctrl+]` follows the first in-document link in the
  focused block to its heading; `ctrl+o`/`ctrl+i` walk the jumplist (link,
  search, line and section jumps; the walkers j/k, page, g/G are not
  recorded) — whether `ctrl+]` is the follow key, whether search/line jumps
  belong in the list, and whether first-link-wins is the right rule —
  journal 2026-08-10.25
- (feedback feature) table cells render their own inline markup: `` `code` ``,
  **bold** and [links] inside a cell now carry RENDER-06's styles the way a
  paragraph's runs do — `tableCellsFor` keeps each cell's markup (cellText
  only trims), `tableNaturalWidths` measures the stripped text via a new
  `cellWidth` so `` `margin` `` still measures 6, and `padCell` renders each
  run through `fragStyle` then pads with plain spaces, so the columns line up
  and the padding stays colourless; the dead `truncate`-to-width path is
  dropped (a column's width is the widest cell in it) and the header row stays
  bold while a header `` `code` `` run still renders as code — journal
  2026-08-10.23
- (feedback fix) `j`/`k` on a block taller than the viewport (a long mermaid
  diagram, a big code block, a huge table) no longer leap focus to the next
  block — the plain walk hopped by the whole block height, one `j` and you were
  a screen away. While focus sits on such a block, `j`/`k` scroll the viewport
  through it 3 lines at a time (the `tallWalkStep`, matching J/K) and move
  focus on only at the block's far edge; landing on a tall block opens at the
  edge the walk is entering from (top when walking down, bottom when walking
  up) and the walked offset survives clampScroll; visual mode keeps the
  blockwise leap — journal 2026-08-10.22
- (feedback feature) `stateDiagram`/`stateDiagram-v2` fences now render as
  state diagrams through the vendored library's new `pkg/state` (third_party
  delta D7): each transition draws a `│ label` run and `▼` head between its
  boxes on one centred spine, `[*]` start/end markers draw as `○`, and a
  chain shares boxes while a branching source draws its box again per branch;
  declarations accept `state "Long description" as Short` (referencable by id
  or description), and anything outside the subset — composite states, notes —
  still falls back to plain source (strict, never half-parsed) —
  journal 2026-08-10.20
- (feedback feature) the mermaid renderer is now the vendored
  `github.com/AlexanderGrooff/mermaid-ascii` (third_party/mermaid-ascii,
  MIT, deltas in its CHANGELOG), so ` ```mermaid ` fences render flowchart/
  graph, sequence and ER through upstream's layouts — the graph renderer is a
  real layered layout with routed edges that honours `flowchart LR`, sequence
  adds participants/arrows/notes (activate/deactivate tolerated, bars not
  drawn), ER adds entities and relationships — and the hand-rolled boxed-tree
  flowchart renderer is deleted (unparseable still falls back to plain source,
  never chroma); every node draws as a rectangle now (no `◇` decision glyph),
  a long edge label on a vertical run can split across the spine
  (`over│here`), plain `---` links draw an arrowhead, and the flat output is
  re-styled run-wise (frame muted, text bright) so edge labels read bright not
  dim; state diagrams are the separate entry above — journal 2026-08-10.19
- (feedback fix) hover without LMB: the gutter `▌` now lights on a bare
  pointer move — the view switched from cell-motion (1002) to any-event (1003)
  mouse tracking, so the terminal reports motion with no button held (the
  wheel, the composer-pane carve-out and the click paths are unchanged) —
  journal 2026-08-10.18
- (feedback feature) mermaid diagrams render as ASCII flowcharts: a
  ` ```mermaid ` fence renders through an in-tree renderer as a boxed tree —
  rectangles, `╭─╮` round and `◇`-marked decision boxes, ├/└ spine junctions
  with ▼ arrowheads, a single child centred straight below its parent, edge
  labels that never truncate (a long one gets its own row above the arrow),
  and a dim `↩ text` reference where a second path reaches an already-shown
  node (reconvergence, cycles) plus a `(unreachable: …)` line for pure
  cycles; unparseable mermaid falls back to plain source, never chroma; only
  `flowchart`/`graph` are understood (`sequenceDiagram` et al. fall back),
  everything renders top-down, and shapes beyond rectangle/round/decision
  collapse to a rectangle — journal 2026-08-10.17
- (feedback feature) comments render markdown: each source line of a comment
  body stays its own rendered line (a line break the reviewer pressed is a
  break they meant — no more run-on paragraphs), a blank line is a paragraph
  break, and **bold**/`code`/links render via the document's RENDER-06
  inline styling; applied to posted bodies, unsaved edits, deleted bodies and
  the new-comment draft preview alike, and the collapsed thread's gutter
  summary strips the markup too; deliberately no fenced code blocks and no
  composer-side hint — journal 2026-08-10.16
- (feedback feature) the reviewer is told when the document changes or a new
  comment lands: the document file is watched (its directory, not the file —
  temp-rename editors survive, F23) and a change raises a persistent
  `● file changed — ctrl+r reload` footer marker plus a status line, and
  `ctrl+r` (`doc.reload`) re-reads the doc, reattaches threads, keeps focus
  by anchor and clears the notice (refuses while a composer is open); an
  agent's `comment.posted` — told apart from a plain thread reload by the
  event log, a `lastEventID` cursor seeded at open and advanced by every
  self-emit so the reviewer's own comments are never announced — reports
  `new comment from <author> on <anchor>` in the status line — journal
  2026-08-10.15
- (feedback fix) gutter glyphs are distinct and the legend is documented: a
  collapsed open thread's marker is now `▸` (the dive's direction) instead of
  the dim `│` that read as the reviewed/flagged mark rule — "a conversation
  lives here" and "reviewed or flagged" are no longer the same glyph two
  columns apart, and the cursor `▌`, mark `│`, partial `·`, thread `▸` and
  resolved `✓` are each named in a new Gutter legend in `--help`, the README,
  and a "Reading the human's screen" section in `margin skill` — journal
  2026-08-10.14
- (feedback feature) `margin skill` teaches the done-signal for a live agent:
  step 5 of the loop now points at a new "When the review is done" section —
  launch with `--stdout` chained to a sentinel
  (`margin --stdout FILE.md && echo "__MARGIN_DONE__"`), treat the launch's
  completion as done while `comments wait` polls in parallel, stop when it
  fires; `&&` over `;` so a crash prints no sentinel — journal 2026-08-10.13
- (feedback fix) `margin export`'s agent-instructions note reconciled with
  `margin skill`: the note now leads with `margin comment add` for replies
  ("keeps the thread file and the event log in step"), keeps the file edit as
  the way to resolve (`resolved: true` in the frontmatter — no resolve CLI yet,
  an M2 item), and states the tradeoff a direct edit writes no event-log line;
  the skill doc was already right and is unchanged — journal 2026-08-10.12
- (feedback feature) `margin skill`: prints the markdown document an agent
  loads to learn how to use margin — the four CLI commands, the interactive
  review loop spelled out end to end (launch in a new terminal, poll
  `comments wait` with its exit contract, reply via `comment add --author
  agent`, the human sees it live through the thread watcher), and the
  contracts (event log JSONL, thread files, anchors). I chose subcommand over
  the feedback title's `--skill` flag, markdown, the whole loop. Demo: run
  `margin skill`, then run the loop from the document alone (journal
  2026-08-10.10)
- (feedback feature) `margin comments wait [--since ID] [--timeout DUR]`:
  blocking poll over `.margin/events.log` that prints raw JSONL event lines
  after the cursor (or all, with no `--since`) and exits 0; timeout exits 1
  silently (the "nothing yet" signal), a real error exits 1 with a stderr
  message; same-second ties resolve by file position, not id order (D13/D14);
  the review root resolves from the cwd since the command takes no file —
  journal 2026-08-10.8
- (feedback fix) raw mode horizontal scroll: `H`/`L` pan the whole raw source
  view sideways through a single viewport-wide offset (`m.rawH`), clipped by
  the same `scrollCodeLine` the rendered view uses on code blocks and tables
  (every line clips at the measure once any overflows, so line length never
  jumps); the offset clamps at the widest line minus the measure and resets to
  0 on every `\` into raw, so the file always opens at column 0; short lines
  read their tail, the gutter stays pinned left — journal 2026-08-10.6
- (feedback fix) raw mode keeps code syntax colours: `\` still shows the
  source verbatim, but lines *inside* a code fence carry chroma's
  highlighting instead of plain text; the ``` fence lines and every other
  block type stay uncoloured, and an empty fence stays plain — journal
  2026-08-10.5
- (feedback fix) command palette shows key bindings: `:` now right-aligns each
  command's first registered binding on its row (`j` beside "Move focus down",
  `space` beside "Cycle", `l` for dive — first-written wins, so primary keys
  show, never the long arrow names); the binding is skipped on staged value
  rows; `keyBindings` is now the ordered slice that `keymap` is derived from —
  journal 2026-08-10.4
- (feedback feature) viewport width and horizontal scroll: `contentWidth()` is
  the terminal's width, not a fixed 76-column measure; wide blocks (code,
  tables, frontmatter) ride it too; tables keep natural column widths and
  scroll instead of narrowing (`tableColumnWidths` deleted); horizontal scroll
  is now `H`/`L` (`move.hscrollLeft/Right`), leaving `l`/`h`/`enter`/`esc`
  dive/surface everywhere — journal 2026-08-10.3
- (feedback fix) enter/esc dive bindings: `enter` dives, `esc` undives, as
  straight aliases of `l`/`h` — the whole dive flow is now reachable as
  `enter`/`j`/`esc`. esc still cancels a visual selection first, is inert
  outside a dive, and the composer owns its own esc; on a code block or the
  frontmatter the alias means enter/esc scroll horizontally rather than dive —
  journal 2026-08-10.2
- (feedback fix) save-less exit on an unchanged edit: `e` on a posted comment
  then `esc` with nothing changed no longer marks it "unsaved draft" — the
  draft path diffs the body against the posted comment, and a byte-identical
  close clears the target's draft (thread and cache) and reports `no changes`;
  a reverted stale draft is cleared too, while a changed abandoned edit still
  keeps its draft — journal 2026-08-10.1
- (feedback fix) cancel-early empty thread: `c` on a thread-less block then an
  immediate cancel no longer leaves a "no comments yet" thread behind — the
  thread `ensureThread` created for the open is dropped unless a comment was
  posted or a draft kept, and focus returns to the block; pre-existing threads
  are untouched — journal 2026-08-09.40
- (feedback fix) composer edit cursor-at-end: `e` on an existing comment opens
  the composer in normal mode with the cursor on the last character of the
  comment text, so an `a`/`A` append lands at the end; the same fix lands the
  settled draft-resume rule that was never implemented — journal 2026-08-09.39
- (feedback fix) composer insert-after-line-ref: `c` after a visual selection
  or a line dive opens the composer already in insert mode with the cursor
  past the auto-appended `L3-6: ` reference — the first keystroke is a
  character, not an `i`/`a` motion; the carve-out is shape-exact, so a prefix
  with real text behind it still opens in normal mode — journal 2026-08-09.38
- (feedback fix) esc-based composer exit: `<esc>` in normal mode closes the
  composer keeping a draft — a new comment opens in insert mode so it takes
  double `esc` (first leaves insert, second exits), an edit opens in normal
  mode so a single `esc` exits; footer hints `esc esc keep` vs `esc keep` —
  journal 2026-08-09.37
- (feedback feature) `<num>gg` / `<num>G` source-line navigation: `42gg` and
  `42G` jump to markdown source line 42, landing on the block that contains
  it and centring the viewport; `gg`/`G` alone stay first/last block. Digits
  echo as `line 42` in the status line and only the two jumps consume them —
  journal 2026-08-09.35
- (feedback feature) rich/raw toggle: `\` switches between rendered and raw
  source, focus preserved by anchor, scroll re-anchored, threads excluded,
  l/h inert — journal 2026-08-09.36
- (feedback fix) frontmatter reachable by keyboard: the frontmatter block is
  a focus stop now (g lands on it, j/k walk it, focus-follow scroll brings it
  back into view), and long fields scroll horizontally with h/l instead of an
  ellipsis truncation; it stays uncommentable/unmarkable and out of search —
  journal 2026-08-09.34
- (feedback feature) `/` search: `/` opens a bottom prompt, typing highlights
  every match live (background-only SGR 94 over rendered lines), enter commits
  and jumps to the next match after focus, `n`/`N` walk with wrap, esc cancels
  the edit — journal 2026-08-09.32
- (feedback fix) composer background artifacts: the composer pane no longer
  paints nvim's own dark background — Normal/NormalFloat/EndOfBuffer forced to
  transparent in composerInit, so the pane shows the terminal's background like
  the document; erased cells leave no marks — journal 2026-08-09.30
- (feedback fix) tunable mouse wheel speed: `--wheel-speed N` sets lines per
  tick (default 3, clamp at 1) — journal 2026-08-09.29
- (feedback fix) dive into multi-line blocks: `l` on a table/raw block dives
  into its source lines, j/k walk them, `h` surfaces, `c`/`gy` anchor a
  comment to the focused line — journal 2026-08-09.28
- (feedback fix) code block horizontal scroll: code blocks don't wrap lines, h/l scroll code blocks horizontally inside the block (offset up to max visual line length - measure width) — journal 2026-08-09.27
- (feedback fix) deleted comments keep content & restore option: deleted comments keep body text dimmed on screen and in thread files, D toggles delete/restore without confirmation — journal 2026-08-09.26
- (feedback fix) thread comment focus highlight: focusing a comment places focus bar and highlight on comment body text lines rather than author header — journal 2026-08-09.25
- (feedback fix) palette visual separation & hover visibility: dim horizontal rule ─ above command palette, hover indicator color brightened to 248 for clear visibility — journal 2026-08-09.24
- (feedback fix) focus retained on comment exit: exiting composer leaves focus on the comment so pressing e immediately re-edits it — journal 2026-08-09.23
- (feedback fix) line/range prepending in comments & yank reference: commenting
  (`c`) with visual selection prepends `L12-18: ` / `L12: ` to comment draft; `gy` / `yr` /
  `selection.yankRef` yanks line reference to clipboard — journal 2026-08-09.22
- (feedback fix) J/K incremental scrolling: J/K scroll the document viewport
  3 lines per press without moving focus — journal 2026-08-09.20
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

*(none)*

## Done

- [x] **(feedback)** import comments from the current GitHub pull request — a
      new `margin import-pr FILE.md` command runs the gh CLI (PR detected from
      the branch via `gh pr view`, review comments via `gh api --paginate
      repos/<owner>/<repo>/pulls/<n>/comments`) and writes the comments that
      name this document as ordinary margin thread files, each attached to the
      block whose source-line range contains its line (`line`, falling back to
      `original_line` for a base-side comment), with the block's content anchor
      and a quote — the same thread a reviewer's `c` would produce, so a PR
      comment becomes a local thread without any GitHub-shaped on-disk format
      (goals.md's "must not shape the local design"). Re-imports are safe:
      same-author-same-body comments are skipped. Comments on other files and
      comments whose line maps to no block are counted and listed, never
      dropped; the PR's general conversation (no line to attach to) is left to
      gh; each import appends best-effort `comment.posted` events (D13/D14).
      Chosen: CLI over a TUI action (a `:import-pr` palette command is a later
      felt leg calling the same `ImportPR`), line-mapping over a GitHub
      id-anchor scheme (a format change, forbidden here). Drains
      `vault/feedback/2026-08-13-import-comments-from-gh-pr.md`; the file is
      deleted. `[felt]` — see journal 2026-08-13.1 — done 2026-08-13



## Done

- [x] **(feedback)** mermaid subgraph layout no longer falls apart: a
      flowchart with subgraphs now places each root subgraph's nodes in its
      own contiguous band of the grid (delta D10 in the vendored
      `pkg/graph`), so the maintainer's reproduction — ~40 nodes, mixed
      shapes, dotted cross-subgraph edges — draws as three disjoint frames
      with every node inside its own frame, instead of frames crashing into
      each other, a node on a frame border and an edge label splitting across
      a frame line. `placeNodesByBand` levels nodes exactly as the flat path
      does, but each unit (a root subgraph's nodes, nested included, or all
      external nodes) keeps its own per-level slot counter; bands are laid out
      end to end with a gap cell of `paddingX`/`paddingY` between them and
      node footprints are reserved so edge routing avoids boxes. Graphs
      without subgraphs keep the unchanged flat placement, byte-identical.
      Chosen: banded placement over upstream's `ensureSubgraphSpacing`
      (redraws boxes, never moves nodes) and over falling back to plain
      source. Drains
      `vault/feedback/2026-08-12-mermaid-layout.md`; the file is deleted.
      `[felt]` — see journal 2026-08-12.5 — done 2026-08-12

- [x] **(feedback fix)** code block tabs render as spaces, not terminal tab
      stops — a fenced block's literal `\t` reached the terminal untouched
      (chroma's tty formatter has no tab option, unlike its HTML `TabWidth`),
      and the terminal resolved it against *absolute* screen tab stops, so a
      tab-indented block misaligned past the gutter and every width
      calculation counted a tab as one rune against up-to-eight rendered
      columns. New `expandTabsLine` (ANSI-aware, column resets at a newline,
      advance to the next multiple of `codeTabWidth` = 8 — the terminal and
      vim default) is fed into `highlightCode` before chroma tokenises, into
      the mermaid-fallback plain path in `render()` and `scrollBlock()`, and
      into raw mode's `hasOverflow`/`scrollRaw` measurements, so the rendered
      text and the width maths finally agree. Chosen: 8-wide expansion (the
      platform default, so the fix is deterministic, not a new preference)
      over a `--tab-width` flag. Drains
      `vault/feedback/2026-08-12-code-block-tabs-render.md`; the file is
      deleted. `[mech]` — see journal 2026-08-12.4 — done 2026-08-12

- [x] **(feedback)** a "threads" view and next/previous comment hopping — `i`
      now opens the threads view in single-document reviews too: the current
      document's threads (newest first) from the already-loaded `m.threads`
      (`docThreadItems`), with a thread whose block has vanished marked
      `[block gone]` and reported on enter instead of dropped — the "even if
      they can't be attached to the block anymore" half of the feedback. In a
      tree review `i` stays the cross-document list (`loadInbox`), unchanged.
      `inbox.toggle`'s Applicable moved from `m.tree != nil` to `m.store !=
      nil`; single-doc rows drop the redundant doc name. `]`/`[` (`comment.next`
      / `comment.prev`) hop focus to the next/previous thread row in the
      document, centred, recorded in the jumplist, reporting rather than
      wrapping at the ends; the footer reads `threads — N thread(s)`. Chosen:
      `]`/`[` over `[c`/`]c` and over `n`/`N` (taken by search). Drains
      `vault/feedback/2026-08-12-threads-view.md`; the file is deleted.
      `[felt]` — see journal 2026-08-12.3 — done 2026-08-12


- [x] **(feedback)** `shift+enter` in the composer is a line break, not an
      exit — `sendKey` intercepts `{Code: KeyEnter, Mod: ModShift}` and
      forwards the same `\r` a bare enter gets, the ctrl+enter-intercept shape
      (F16), because forwarding the CSI-u `<S-CR>` (`\x1b[13;2u`) was
      unreliable: nvim 0.12.4 dropped it silently in insert mode and the
      maintainer's nvim turned it into the draft exit (the sequence's leading
      ESC, unbound, is the mapped `<Esc>` draft-exit — F19's meta-key catch).
      A composer mapping was rejected: it only works on nvim builds that decode
      the key at all, and the unreliability *is* the bug. Chosen: host
      intercept over CSI-u forwarding. `composer.wait()` now caches the child's
      exit error so a poll and a blocking wait never call `cmd.Wait` twice
      (the second call errors and read as `discarded`); `waitForChild` and the
      test helpers share it. New `waitExitPoll` helper. Drains
      `vault/feedback/2026-08-12-shift-enter-line-break.md`; the file is
      deleted. `[felt]` — see journal 2026-08-12.2 — done 2026-08-12

- [x] **(feedback)** mark and comment individual lines when diving inside a
      block — `space`/`r`/`f` on a dived source line mark that line only
      (`lineMarks`, session-only, keyed `anchor:line`), a diveable block's
      review state is the roll-up of its lines (block-level marking sets every
      line, a flagged line flags the block, partial reads `·` in a heading),
      each marked line renders its own gutter rule in both views, progress
      (`reviewProgress`, tree pane slots, tree footer) and the export read the
      roll-up, and line marks survive a tree switch (`lineMarkCache`). The
      `c`-on-a-line comment half already existed (`L<n>` draft prefix); the
      missing marking half is what this adds. Chosen: lines-authoritative
      over a parallel block+line state (no invisible residue), `:` in the
      line-key (never in an anchor). Drains
      `vault/feedback/2026-08-11-dive-line-mark-comment.md`; the file is
      deleted. `[felt]` — see journal 2026-08-12.1 — done 2026-08-12

- [x] **(feedback)** goreleaser pipeline for pushes into `main` — a
      `.goreleaser.yml` (linux/darwin amd64/arm64, `CGO_ENABLED=0`, stamps
      `main.version` from the template) and a `.github/workflows/release.yml`
      that runs `goreleaser check` then `goreleaser release --snapshot
      --clean` on every push to `main` and uploads the archives + checksums to
      the run's Artifacts. The version is the short commit sha
      (`snapshot.version_template: "{{ .ShortCommit }}"`), not a semver tag —
      there are no tags and none are planned; snapshot mode builds into `dist/`
      and publishes nothing, which is exactly the wanted shape. Modeled on
      kubecom's setup (same v2.17.1 pin, same `check`-then-snapshot order,
      same archives + checksums upload — the bare-binary archive is kept for a
      future tag release but not uploaded, kubecom's own "double the upload for
      no new bits" judgment). Drains
      `vault/feedback/2026-08-11-goreleaser-ci.md`; the file is deleted.
      `[mech]` — see journal 2026-08-11.9 — done 2026-08-11

- [x] **(feedback fix)** mouse hover throttled — a pointer sweep across the
      document no longer floods the input queue. Bubble Tea runs `Update` and
      `View` (margin's whole document layout) for *every* message, and the
      120fps cap only bounds when a frame is flushed — so an unthrottled
      motion flood queued full renders faster than the event loop drained
      them, and keystrokes waited behind the backlog. A new `motionThrottle`
      (`internal/review/review.go`) drops a `MouseMotionMsg` that arrives
      within one frame period (8.33ms, matching `WithFPS(120)`) of the last
      one processed, or that restates the cell the pointer already occupies;
      it is wired into `runModel` via `tea.WithFilter`, which runs ahead of
      Update/View and discards the message entirely when it returns nil.
      Clicks, wheels and keys pass through untouched, so a click or scroll is
      never delayed behind the hover queue. Chosen: frame-period throttling
      at the message source over a "only when hoveredEntry changes" rule
      (which still pays a full `View()` per event — bubbletea renders
      unconditionally), a self-scaling frame-period window over a fixed
      millisecond count, position-dedup over time-only, `tea.WithFilter` over
      touching `handleMotion`. Drains
      `vault/feedback/2026-08-11-mouse-hover-lag.md`; the file is deleted.
      `[felt]` — see journal 2026-08-11.7 — done 2026-08-11

- [x] **(feedback)** thread & large-comment navigation — `j`/`k` on a comment
      taller than the viewport scroll the viewport through it `tallWalkStep`
      lines at a time instead of moving focus to the next comment, the same
      treatment a tall block gets (2026-08-10.22). The dive walk in `moveFocus`
      reads each comment's span from `m.subspans` (the render pass already
      registers it — F5's one-pass rule), so a large comment is one stop you
      scroll through: `j`/`k` move the viewport 3 lines a press and keep focus,
      only moving on at the comment's far edge — its last row at the viewport
      bottom walking down, its head at the top walking up. Landing on a tall
      comment opens at the edge the walk enters from (head down, bottom up),
      with `scrollAnchor` pinned so `clampScroll` keeps the walked offset, and
      `dive()` opens a thread whose first comment is tall at its head rather
      than the tail `clampScroll` would pin. The dive's flow-out at the ends is
      unchanged — a thread never traps — but it now fires only after the edge
      comment has been read, so leaving a thread happens by finishing it, not
      by one accidental press. Chosen: reusing `m.subspans` over a new
      per-comment layout pass, the block walk's exact 3-line step and
      entering-edge landing, keeping the flow-out at the ends over trapping,
      a `!m.visual` guard as belt-and-braces. Drains
      `vault/feedback/2026-08-11-thread-comment-navigation.md`; the file is
      deleted. `[felt]` — see journal 2026-08-11.6 — done 2026-08-11

- [x] **(feedback)** draft comments deletable with `D` — `D` on a draft
      discards the draft instead of tombstoning the comment or thread
      underneath: an unsaved edit of a posted comment reverts it to its posted
      text, a half-written new comment is discarded (a thread that held
      nothing but the draft is dropped and focus returns to the block, the
      same empty-thread rule `dismiss` uses), and a draft discard writes
      neither the thread file nor the event log — a draft is not committed
      review state. `draftAtFocus` (command.go) names the target for both the
      delete and the palette title with `editTarget`'s precedence, so `D` and
      `: thread.delete` never disagree. Chosen: discard over tombstone
      ("delete the draft" reads as the draft going away; tombstoning a comment
      to discard an edit throws away committed text), `editTarget`'s
      precedence, no prompt, no event. Drains
      `vault/feedback/2026-08-11-draft-comments-deletable.md`; the file is
      deleted. `[felt]` — see journal 2026-08-11.5 — done 2026-08-11

- [x] **Review progress across the whole tree** — a directory review now has a
      tree-wide progress readout. Marks are carried per-document across
      switches (`markCache`, keyed by rel path; `openTreeFile` saves the
      outgoing document's marks and restores the incoming document's), so
      switching no longer throws the previous document's review away. The
      denominator is the whole tree: `markTotals` holds every file's markable
      block count, computed at tree open (and refreshed on `ctrl+r` reload),
      so a file never opened still counts its blocks. Each pane row carries a
      fixed right-aligned progress slot — `n/m` dim when partially reviewed,
      green when the whole file is reviewed, orange `n/m!` when any block is
      flagged, blank for untouched files and dirs — and the pane width grows
      by the slot (still clamped at 36). The footer in a tree review reads
      `tree N/M reviewed · F flagged` across every file's blocks instead of
      the open document's readout (which now lives on its pane row);
      single-document reviews are unchanged. Chosen: session-cache marks over
      persisting them (persistence is a format decision nothing here needs),
      blank over `0/n` for untouched files, `n/m!` over separate
      reviewed/flagged columns, footer tree-roll-up over tree-plus-document.
      `[felt]` — see journal 2026-08-11.4 — done 2026-08-11

- [x] **A cross-document comment inbox** — `i` in a directory review (D10)
      replaces the document column with the comment inbox: `loadInbox` walks
      the thread files under `.margin/threads/` (D9 keys them by document
      path, so the walk mirrors the tree without parsing any document) and
      returns one row per thread, newest activity first. `enter`/`l` on a row
      opens the thread's document through the same `openTreeFile` a pane open
      performs — re-pointed store/watcher, fresh session — and lands on the
      thread's block, centred, with the landing recorded in the jumplist
      (`ctrl+o` walks back); j/k/g/G move, the wheel scrolls the inbox's own
      offset, `esc`/`h`/`tab` return to the document (scroll preserved), `q`
      quits, and the tree pane stays rendered beside it so a thread's home is
      always in view. A thread whose document left the tree stays, dimmed
      `[doc not in tree]`, and is reported on enter — nothing silently drops.
      Resolved threads render dimmed with `✓`. The list refreshes on open,
      not live; clicks in the inbox column are inert; single-document
      reviews have no inbox (D10), so `i` stays unbound there. Chosen:
      whole-column view over a tree-pane mode, `i` over `c`/`m`/`x`, keeping
      the tree pane visible, one line per thread (document + collapsed
      summary), live refresh deferred. `[felt]` — see journal 2026-08-11.3 —
      done 2026-08-11

- [x] **Cross-document link navigation (first slice of "Navigation between
      documents")** — `ctrl+]` on a link naming another markdown file in a
      directory review opens it. `jump.follow`'s non-`#` hrefs are now
      followed, not reported as outside: the path resolves markdown's way
      (relative to the current document's directory, or to the review root
      when it starts with `/`), is looked up in the tree, and switches to the
      file exactly as `openTreeFile` does — fresh review, doc/threads/store/
      watcher re-point, session state resets, tree cursor follows — landing
      on the `#fragment`'s heading (centred, like an in-document follow) when
      there is one, at the top otherwise. A link back at the current document
      resolves its fragment without switching; a fragment naming no heading is
      surfaced; a file outside the tree, a non-markdown file and an absolute
      URL still report "links here point outside this document". Single-
      document reviews stay single-document (D10) — no tree to switch into.
      Chosen: resolving against the containing file's directory (markdown's
      own rule) plus `/` = root for root-relative links; switching via
      `openTreeFile` so a follow and a pane open behave identically; landing
      centred; scoped to tree reviews only. `[felt]` — see journal
      2026-08-11.2 — done 2026-08-11

- [x] **Review a directory, not just a file** — `margin DIR/` and bare `margin`
      (D10) walk the directory for markdown files and open the first one with a
      left file-tree pane: directories that have markdown beneath them are
      dimmed headers with their files indented, `tab` toggles focus between the
      pane and the document, `j`/`k` (and `g`/`G`) move through the files
      skipping headers, `enter`/`l` opens the focused file (threads, store and
      watcher re-point, session state resets — a fresh review of a fresh
      document, refusing while a composer is open), `esc`/`h`/`tab` return to
      the document; the open file carries a `▸` and the focused row a `▌`;
      clicks on a pane row select and focus it; `contentWidth`, the composer
      box/cursor and mouse hit-testing offset by the pane width; hidden
      directories are skipped so `.margin`'s thread files never appear; arg
      validation moved to `cobra.MaximumNArgs(1)` per D10; the export still
      covers the one document under review. Chosen: left pane, `tab`, dive keys
      to open, dir headers as non-focusable rows, reset-on-switch. `[felt]` —
      see journal 2026-08-11.1 — done 2026-08-11

- [x] **Link navigation between blocks, with a jumplist** (`ctrl+o` / `ctrl+i`)
      — `ctrl+]` follows the first in-document `[text](#heading-slug)` link in
      the focused block to the heading it names (GitHub-style slugs, duplicate
      headings disambiguated with `-1`/`-2` like GitHub), centring it like a
      search jump; the jumplist records every *teleport* — link-follows, search
      jumps, source-line jumps, `goto` section jumps — and `ctrl+o`/`ctrl+i`
      walk older/newer entries with vim's truncate-forward-on-branching and
      cap-at-100 semantics. Dead ends are reported, not silent: no link, all
      links external (the tree-view milestone's job), and a fragment naming no
      heading each get a status line. Chosen: `ctrl+]` over `gf`/`gx`
      (`g` is an eager prefix that would fire move.first first) and
      `enter`/`l` (dive); recording the teleports but not the walkers
      (`j`/`k`, page keys, `g`/`G`) so the list does not flood; first-link-wins
      in a block with several. `[felt]` — see journal 2026-08-10.25 — done
      2026-08-10

- [x] **(feedback feature)** inline code (and bold/link) renders inside table
      cells: a `| `margin` runs in a TUI |` cell now shows `margin` styled as
      code instead of plain text — `tableCellsFor` keeps each cell's
      RENDER-06 inline markup (cellText trims only, never strips), column
      widths are measured on the stripped text (`cellWidth`), and `padCell`
      renders the runs through the same `fragStyle` a paragraph uses then
      pads with plain spaces, so the columns still line up and the padding
      stays colourless; the dead `truncate`-to-width path is dropped and the
      bold header now styles per-run so a header `` `code` `` cell still reads
      as code. Drains
      `vault/feedback/2026-08-10-inline-code-in-tables.md`; the file is
      deleted. `[felt]` — see journal 2026-08-10.23 — done 2026-08-10


- [x] **(feedback fix)** `j`/`k` on a block taller than the viewport scroll the
      viewport through it a few lines at a time instead of hopping focus to the
      next block — the plain walk's leap by the whole block height is what made
      a long mermaid diagram, big code block or huge table unreadable (one `j`
      and you were a screen away). `moveFocus`'s block-level walk
      (`internal/review/review.go`) now treats a tall span specially: while
      focus sits on it, `j`/`k` move the viewport `tallWalkStep` lines (3,
      matching J/K) and keep focus; only at the block's far edge — last row at
      the viewport bottom, first row at the top — does the next press move
      focus to the neighbour. Landing on a tall block opens at the edge the
      walk enters from (top walking down, bottom walking up — `clampScroll`
      would otherwise drop into the middle or pin the tail), with `scroll` and
      `scrollAnchor` set so the walked offset survives the next render. Visual
      mode keeps the plain leap: a blockwise selection wants the block as one
      stop, and mid-block scrolling would make j/k dead presses mid-selection.
      Chosen: the 3-line step (matches J/K, the default wheel tick) over a
      quarter-viewport step; the entering-edge landing over always re-opening
      at the top; applying it to every tall span including long thread rows.
      Drains
      `vault/feedback/2026-08-10-tall-block-jk-incremental-scroll.md`; the file
      is deleted. `[felt]` — see journal 2026-08-10.22 — done 2026-08-10

- [x] **(feedback fix)** `go install module@version` works again: the vendored
      `third_party/mermaid-ascii` was its own module wired through a `replace`
      directive, which the go command refuses when installing at a version
      ("The go.mod file for the module providing named packages contains one or
      more replace directives") — the copy is now **folded into the host
      module** (its `go.mod`/`go.sum` deleted, imports rewritten to
      `github.com/neuroplastio/margin/third_party/mermaid-ascii/pkg/...`, the
      root `replace` gone), so the README's `go install ...@latest` story works.
      Verified against the real install path through a local VCS redirect:
      the command failed before the fold and installs a binary after it.
      Deltas/`CHANGELOG.md`/`LICENSE` stay in place; D16 updated. Drains
      `vault/feedback/2026-08-10-go-install-replace-directive.md`; the file is
      deleted. `[mech]` — see journal 2026-08-10.21 — done 2026-08-10

- [x] **(feedback feature)** support mermaid `sequenceDiagram` and
      `stateDiagram-v2` in the review. **Slice 1 landed 2026-08-10** (journal
      2026-08-10.19): the library is vendored at `third_party/mermaid-ascii`
      (MIT preserved, deltas in its CHANGELOG, wired via a `replace`) and
      flowchart/graph, sequence and ER render through it. **Slice 2 landed
      2026-08-10** (journal 2026-08-10.20): `stateDiagram`/`stateDiagram-v2`
      render through a new `pkg/state` inside the vendored copy (delta D7) —
      states, transitions, `[*]` start/end, strict fallback for composite
      states and notes. Drains
      `vault/feedback/2026-08-10-mermaid-sequence-and-state.md`; the file is
      deleted. `[felt]` — see journal 2026-08-10.19 and 2026-08-10.20 — done
      2026-08-10

## Backlog — M1

*(none)*

## Backlog — M2

*(none)*

## Backlog — Commands (from feedback, 2026-08-07-command-palette)

Not part of a milestone; cross-cutting infrastructure the palette feedback
asked for. Split per the feedback's own suggested ordering — see
`vault/journal/2026-08-07.6.md` for why CMD-01 alone is this leg.

- *(none)*

## Backlog — M3 (tree view)

*Closed 2026-08-11 — the milestone's exit criterion was met (the two-step
navigation story works end to end), so the milestone moved to M4. The felt
parts of what landed are still on `Needs a look:` above; the unit-of-review
question stayed settled as D10.*

- *(Review a directory, not just a file — landed, see Done 2026-08-11.)*
- *(Cross-document link navigation — landed, see Done 2026-08-11.)*
- *(A cross-document comment inbox — landed, see Done 2026-08-11.)*
- *(Review progress across the whole tree — landed, see Done 2026-08-11.)*

## Backlog — M3 (navigation)

Independent of the tree view (D10): these are about moving around one
document, regardless of the order the tree-view work itself lands in.

- *(Link navigation between blocks, with a jumplist (`ctrl+o` / `ctrl+i`) —
  landed, see Done 2026-08-10.)*
- *(SCROLL-01/02/03 — scroll offset, page/half-page keys, mouse wheel — landed
  earlier; see Done.)*

## Backlog — M4 (round two)

The differentiator, and the reason block ids exist. No felt decision is baked
in here yet — each item needs its own leg, and the first one (how a previous
revision is recorded and located) touches on-disk format, so expect a question
or a Dn before building.

- Diff a document against the previous reviewed revision
- Render the diff as prose — a rewritten paragraph reads as a rewritten
  paragraph, not as a wall of `-`/`+`
- Carry marks and threads across a revision; show what changed under a mark

**Exit:** an agent hands back round four of a plan and the reviewer can see what
moved without re-reading it.

## Blocked

- **Q-0004** (how a document's previous reviewed revision is recorded and
  located) — raised 2026-08-11. Blocks M4's first item ("Diff a document
  against the previous reviewed revision") and, through it, the two M4 items
  that build on its baseline (rendering the diff as prose; carrying marks and
  threads across a revision). Whether the baseline is a snapshot taken on load,
  on session end, or on an explicit checkpoint — and where it lives on disk —
  is an on-disk format decision under `.margin/`, the expensive-to-unwind
  class; the board's own M4 note flags that this item expects a question or a
  Dn before building.

Q-0003 was answered 2026-08-10 and folded into `knowledge/decisions.md` as
**D13** (the event log at `.margin/events.log`); the question file is deleted.
The interactive-agent-review feature in
`vault/feedback/2026-08-10-interactive-agent-review.md` is complete: the event
log (2026-08-10.7) and `margin comments wait [--since <last_known_id>]`, the
CLI half that reads it (2026-08-10.8), have both landed and the feedback file
is deleted. The wait command's exact surface — its exit codes, timeout shape,
and output — is the felt part and is on `Needs a look:` above.

Q-0001 and Q-0002 were answered 2026-08-07 and folded into
`knowledge/decisions.md` as **D10** (tree review) and **D11** (thread
resolution and deletion) in the leg that closed them; the question files are
deleted. See those entries for the settled shape, and `interaction.md`'s "Not
settled" section for what each still leaves open for a felt leg.

## Done

- [x] **(feedback fix)** hover works without a button held: the gutter
      hover `▌` only lit while dragging because `View` requested cell-motion
      mouse tracking (`tea.MouseModeCellMotion`, 1002), under which the
      terminal reports motion only while a button is held — so a bare pointer
      move never became a `MouseMotionMsg` at all. The view now requests
      any-event tracking (`tea.MouseModeAllMotion`, 1003), which reports every
      move; `handleMotion` was already correct and is unchanged, as are the
      wheel, the composer-pane carve-out (hover clears over the pane) and the
      click paths. Drains
      `vault/feedback/2026-08-10-hover-without-lmb.md`; the file is deleted.
      `[felt]` — see journal 2026-08-10.18 — done 2026-08-10

- [x] **(feedback feature)** support mermaid diagrams in the review: a
      fenced block whose info string is `mermaid` renders through a new
      in-tree ASCII renderer (`internal/review/mermaid.go`) instead of
      chroma, which has no mermaid lexer. `flowchart`/`graph` parse into a
      node/edge model (both label spellings, chains, `&` joins, `%%`
      comments, `subgraph` bodies, brackets skipped when scanning links) and
      lay out as a boxed tree: bordered boxes (rectangle, `╭─╮` round, `◇`
      decision), a spine with `├`/`└` junctions and `▼`/`│` arrowheads, a
      single child centred straight below its parent so chains do not
      staircase, edge labels centred on the run or on their own row when they
      do not fit (never truncated), and a dim `↩` reference for a node a
      second path reaches (reconvergence, cycles) so the layout always
      terminates, with an `(unreachable: …)` line for pure cycles. Anything
      unparseable — other diagram types, malformed statements — falls back to
      the block's plain source lines, never chroma and never a half-parsed
      diagram. Raw mode keeps a mermaid fence plain; H/L horizontal scroll
      still applies to a diagram wider than the terminal. Export and the
      thread files are untouched (the on-disk format never changes). Drains
      `vault/feedback/2026-08-10-mermaid-diagrams.md`; the file is deleted.
      `[felt]` — see journal 2026-08-10.17 — done 2026-08-10

- [x] **(feedback feature)** comments render markdown formatting: a comment
      body no longer collapses into a run-on paragraph — `wrapComment`
      (review.go) keeps each source line its own rendered line, a blank line
      becomes a paragraph break, and **bold**/`` `code` ``/`[links]` render
      via `wrapInline`, the RENDER-06 treatment the document's paragraphs
      already have; applied to posted bodies, unsaved-edit drafts, deleted
      bodies and the new-comment draft preview, and the collapsed thread's
      `summary` strips the markup too (`plainMarkdown`), so the gutter never
      shows literal `**`. Chosen: line breaks + inline markup, no fenced
      blocks, no composer-side hint. Drains
      `vault/feedback/2026-08-10-comments-markdown-formatting.md`; the file
      is deleted. `[felt]` — see journal 2026-08-10.16 — done 2026-08-10

- [x] **(feedback feature)** visually notify the reviewer when the document
      changes on disk or a new comment lands. The document file is now
      watched — its directory, not the file, so an editor that saves via
      temp-file-then-rename still fires (F23) — and a change raises a
      persistent `● file changed — ctrl+r reload` footer marker plus a status
      line; `ctrl+r` (`doc.reload`) re-reads the document, reattaches
      threads, keeps focus on the block by anchor, refreshes the raw source
      and clears the notice, refusing while a composer is open (D7). A new
      agent comment is told apart from a plain thread reload by the event log
      — a `lastEventID` cursor seeded at open from the log tail, advanced by
      every `emit` the reviewer makes (it now returns the id it wrote) so the
      reviewer's own comments are never announced back — and reported as
      `new comment from <author> on <anchor>` in the status line. Drains
      `vault/feedback/2026-08-10-review-change-notification.md`; the file is
      deleted. `[felt]` — see journal 2026-08-10.15 — done 2026-08-10

- [x] **(feedback fix)** the gutter's glyphs are distinct and named: a
      collapsed open thread's marker is now `▸` (the point of the `l`/`enter`
      dive) instead of the dim `│` that read as the reviewed/flagged mark rule
      — cursor `▌`, mark `│` (green/orange), partial `·`, thread `▸` and
      resolved `✓` are now each its own shape, and a Gutter legend in
      `--help`, a paragraph in the README, and a "Reading the human's screen"
      section in `margin skill` name what each means. Drains
      `vault/feedback/2026-08-10-agent-loop-gutter-icon-ambiguity.md`; the file
      is deleted. `[felt]` — see journal 2026-08-10.14 — done 2026-08-10

- [x] **(feedback feature)** `margin skill` now teaches an agent participating
      live how to know when the review is done. A new "When the review is done"
      section (between the loop and the contracts) spells out the pattern the
      maintainer's loop actually runs: run the launch and the poll as two
      parallel jobs and treat the launch's completion as the done-signal —
      launch with `--stdout` chained to a sentinel
      (`margin --stdout FILE.md && echo "__MARGIN_DONE__"`), so a normal quit
      prints the final review (replies included) and echoes the sentinel, while
      `comments wait` loops in parallel and stops when the launch job
      completes. `&&` over `;`, stated and explained: a normal quit exits 0, so
      a call that completes with no sentinel is margin erroring, not the review
      finishing. Step 5 of the loop now points at the section, so the loop
      reads as one that terminates; the root/skill help texts and README carry
      the done-signal clause too. Drains
      `vault/feedback/2026-08-10-agent-loop-review-done-signal.md`; the file is
      deleted. `[felt]` — see journal 2026-08-10.13 — done 2026-08-10

- [x] **(feedback fix)** `margin export`'s "Agent instructions" note reconciled
      with `margin skill`: the note now opens by recommending `margin comment
      add` for replies (the CLI "keeps the thread file and the event log in
      step" — the skill's stance, the settled loop), keeps the thread-file edit
      as the way to resolve a thread (`resolved: true` in the frontmatter; no
      resolve CLI exists yet, an M2 item), and states the tradeoff: "a direct
      file edit reaches the human (the thread watcher reloads it) but writes no
      event-log line". The skill document already said the right thing and is
      unchanged. Drains
      `vault/feedback/2026-08-10-agent-loop-export-instructions-conflict.md`;
      the file is deleted. `[felt]` — see journal 2026-08-10.12 — done
      2026-08-10

- [x] **(feedback fix)** event log lines carry the comment text: comment-level
      events in `.margin/events.log` gain a `text` field holding the comment's
      body as it stood in the thread file at emit time (the full body, so a
      listener — an agent in the loop — sees what was said without a second
      read of the thread file or export). Thread-level events omit the field
      (`json:"text,omitempty"`), so their lines are byte-identical to the D14
      shape and absence of `text` is itself the thread-level signal. A
      tombstoned comment still carries its body (D11 keeps it). Old lines
      parse with empty text — no migration. Wired through the existing emit
      call sites: the TUI's submit (`dismiss`), delete/restore
      (`deleteFocused`) and `margin comment add`. Recorded as **D15**,
      extending D14's line shape. Drains
      `vault/feedback/2026-08-10-agent-loop-event-comment-text.md`; the file is
      deleted. `[mech]` — see journal 2026-08-10.11 — done 2026-08-10

- [x] **(feedback feature)** `margin skill` — the document an agent loads to
      learn how to use margin. A new subcommand prints `review.SkillDocument()`
      (an embedded markdown file, ~100 lines) to stdout and nothing else. The
      document teaches the **interactive review loop** end to end, built from
      the pieces that already exist and nothing new: launch `margin FILE.md` in
      a new terminal for the human (the agent never drives it); read the state
      with `margin export FILE.md`; wait with `margin comments wait --since ID
      --timeout DUR` including the three-outcome exit contract (0 = new event
      lines, 1 silent = timeout "nothing yet", 1 + stderr = real error) and the
      no-`--since` first call that seeds the cursor; reply with `margin comment
      add FILE.md --anchor ID --text "..." --author agent`, the reply reaching
      the human **live** through the thread watcher while the event log is what
      the agent tails; then loop on the last line's id. A contracts section
      pins the event log (JSONL, append-only, 13-char time-ordered ids,
      file-position ties, torn tail), thread files (frontmatter
      anchor/document/`resolved: true`; the CLI keeps thread + log in step),
      and anchors (stable across rewrites; a stale anchor fails loudly).
      Chosen: subcommand over the feedback title's `--skill` flag (every other
      agent-facing command is a subcommand), markdown over plain text, the
      whole loop spelled out over a summary, `go:embed` over a string literal.
      Drains `vault/feedback/2026-08-10-agent-skill-command.md`; the file is
      deleted. `[felt]` — see journal 2026-08-10.10 — done 2026-08-10

- [x] **(feedback fix)** event log on-disk shape: JSONL + compact ids + unix
      seconds. Each `.margin/events.log` line is now one JSON object
      (`{"id":..,"at":..,"type":..,"doc":..,"anchor":..,"author":..,"comment":..}`)
      instead of seven tab-separated fields, so the fields are self-describing
      and the free-form-field sanitizer is gone (JSON escapes tabs/quotes/
      newlines for free). Ids are 13-char Crockford base32 — 7 chars of unix
      seconds (35 bits, fixed-width so lexicographic order stays chronological)
      + 6 chars of randomness (30 bits) — down from the 26-char ULID. `at` is
      a unix timestamp at second precision, an integer, not RFC3339Nano. The
      wait command's contract, the file-position tie rule (same-millisecond →
      same-second; `readEventsAfter` matches the id and takes everything after
      its line, so the mechanism is granularity-independent) and the torn-tail
      skip all survive unchanged; recorded as **D14**. Drains
      `vault/feedback/2026-08-10-event-log-jsonl-and-compact-ids.md`; the file
      is deleted. `[mech]` — see journal 2026-08-10.9 — done 2026-08-10

- [x] **(feedback feature)** interactive agent review, slice 2: `margin
      comments wait [--since <last_known_id>]` — the CLI half that reads the
      event log. `review.WaitEvents` polls `.margin/events.log` and returns
      the raw log lines of every event after the cursor (the whole log when
      `--since` is omitted), blocking until one arrives or a timeout elapses;
      the cursor is a *file position*, so same-millisecond ties come out in
      file order exactly as D13 prescribes, and an unknown id is an error, not
      a silent reset. New `comments` subcommand tree: **exit 0** = new events
      printed; **exit 1 silent** = timeout with nothing new (the poller's
      "try again" signal); **exit 1 with a stderr message** = a real error —
      told apart by stderr being empty, not by a distinct code. Also fixed
      `resolveReviewRoot` skipping a directory's own marker (the cwd's
      `.git`/`.margin` was never a candidate for a `"."` path). Drains
      `vault/feedback/2026-08-10-interactive-agent-review.md`; the file is
      deleted. `[felt]` — see journal 2026-08-10.8 — done 2026-08-10

- [x] **(feedback feature)** interactive agent review, slice 1: the event log
      at `.margin/events.log`. Records the maintainer's answer to Q-0003 as
      **D13**: the identity the wait command's `--since` names is an *event*
      id, not a comment id — thread files are unchanged. The log is a single
      append-only file (sibling of `threads/` inside `.margin`), one
      tab-separated line per event (`id at type doc anchor author comment`),
      ids being 26-char ULIDs (48-bit ms time + 80-bit entropy) hand-rolled
      in-tree and pinned by known-value tests. Wired into every thread
      mutation: the TUI's post/edit (`dismiss`), resolve toggle and
      comment/thread delete-restore each emit after `store.save`, surfacing a
      failed append in the status line, and `margin comment add` emits
      `comment.posted` best-effort so a lost notice can never make an agent
      re-post. Reader contract: a malformed completed line is an error, an
      unterminated final line is a torn append and is skipped. Closes
      `vault/questions/Q-0003-comment-identity-for-wait.md` (deleted). The
      feedback file stays until the wait command lands. `[mech]` — see journal
      2026-08-10.7 — done 2026-08-10

- [x] **(feedback fix)** raw mode horizontal scroll: `H`/`L` pan the whole raw
      source view sideways (the raw-mode analogue of the rendered view's
      `scrollBlock`). `move.hscrollLeft/Right` are no longer inert while `\`
      is on: `Run` routes to a new `scrollRaw` when `m.raw`, which shifts a
      single viewport-wide offset `m.rawH` — one offset for the whole "the
      file" view, not `scrollBlock`'s per-anchor map, because raw mode is a
      flat source list and a per-block offset would leave half the screen
      frozen — bounded between 0 and the widest source line minus the content
      width. `renderRaw` clips every line through the same `scrollCodeLine` the
      rendered view uses on code blocks and tables: once any line overflows the
      measure, every line clips at it, so line lengths never jump when the
      offset moves off 0, and a short line reads its tail rather than wrapping.
      The gutter stays pinned left; chroma highlighting (2026-08-10.5) survives
      the clip. `toggleRaw` resets `rawH` to 0 on entering raw, so the file
      always opens from its left edge — a stale global offset would blank a
      document whose lines are all shorter than it. Drains
      `vault/feedback/2026-08-10-raw-mode-horizontal-scroll.md`; the file is
      deleted. `[felt]` — see journal 2026-08-10.6 — done 2026-08-10

- [x] **(feedback fix)** raw mode keeps code syntax colours: `\` still renders
      the source verbatim, but a source line inside a fenced code block now
      borrows chroma's syntax highlighting (`renderRaw` pre-computes
      `highlightCode` per code entry and maps content lines by
      `li - b.line`) instead of dumping plain text — the palette the rendered
      view already communicates is not thrown away on the raw switch. The
      ``` fence lines stay plain source, and an empty fence is skipped so its
      closing ``` cannot map into a bogus highlight; paragraphs, headings and
      lists remain byte-verbatim, because raw mode exists to show the exact
      bytes and chroma is the one colour that *is* the content. `selLine` and
      `applySearch` already handle ANSI lines, so selection and search work on
      the highlighted lines unchanged. Drains
      `vault/feedback/2026-08-10-raw-mode-colors.md`; the file is deleted.
      `[felt]` — see journal 2026-08-10.5 — done 2026-08-10

- [x] **(feedback fix)** command palette shows registered key binding: the
      palette (`:`) right-aligns each command's first registered key binding on
      its row, dimmed like the description — `▌ move.down — Move focus down
      …j` — so a reviewer sees the key without a second lookup. "First" is the
      binding written first in the new ordered `keyBindings` slice
      (`internal/review/command.go`), which is now the single source of truth:
      `keymap`, the O(1) lookup `handleKey` uses, is derived from it at init,
      so display order and dispatch can never disagree and "first" is not a
      map-iteration accident (`j` for move.down, not `down`; `l` for dive, not
      `right`/`enter`; `space` for mark.cycle, not the bare ` `). Staged value
      rows (`mark`'s `reviewed`/`flagged`) show no binding — they are values,
      not commands. Drains
      `vault/feedback/2026-08-10-command-palette-key-binding.md`; the file is
      deleted. `[felt]` — see journal 2026-08-10.4 — done 2026-08-10

- [x] **(feedback feature)** viewport width and horizontal scroll:
      `contentWidth()` is the terminal's width (minus gutter, 40-col floor)
      rather than a fixed 76-column measure, so a wide terminal reads at its
      full width; wide blocks (code, and tables once they scroll) are allowed
      the full terminal width; tables keep their natural column widths and
      scroll horizontally with H/L like a code block instead of narrowing
      (`tableColumnWidths` deleted with its tests); horizontal scroll moved
      from `l`/`h` to `H`/`L` (`move.hscrollLeft/Right`, bound to previously
      unbound keys), leaving `l`/`h`/`enter`/`esc` dive/surface everywhere —
      `l` on a code block no longer scrolls it. Drains
      `vault/feedback/2026-08-10-viewport-width-and-horizontal-scroll.md`; the
      file is deleted. `[felt]` — see journal 2026-08-10.3 — done 2026-08-10

- [x] **(feedback fix)** dive/undive also binds to `enter`/`esc`: `enter` dives,
      `esc` undives, in addition to the existing `l`/`h` bindings. Straight
      aliases of `move.dive`/`move.surface` (one command id, one behaviour);
      esc cancels a visual selection first, is a quiet no-op outside a dive,
      and the composer owns its own esc while open; on a code block or the
      frontmatter the alias means enter/esc scroll horizontally, exactly as
      l/h. Drains the last item of
      `vault/feedback/2026-08-09-composer-exit-and-thread-feedback.md`; the
      file is deleted. `[felt]` — see journal 2026-08-10.2 — done 2026-08-10

- [x] **(feedback fix)** save-less exit on an unchanged edit: `e` on a posted
      comment, then any draft-path exit (`esc` in normal mode, `:q`, `SPC c d`,
      blur) with the text untouched, used to store a draft identical to the
      comment and report "edit kept unsaved" — nothing changed, but a `✎
      draft` marker still appeared. `dismiss`'s `outcomeDraft` branch now diffs
      the body against the posted comment: a byte-identical close clears the
      target's draft (thread and draft cache) and reports `no changes`. A
      resumed edit that *reverts* to the posted text clears its stale draft
      too — same statement from the other direction, and it cleans up the
      identical-to-posted drafts the old behaviour could leave behind. New
      comments are untouched (no baseline to diff), as is the empty-buffer
      close. Drains the unchanged-edit-draft item of
      `vault/feedback/2026-08-09-composer-exit-and-thread-feedback.md`; the
      enter/esc dive item remains. `[felt]` — see journal 2026-08-10.1 — done
      2026-08-10

- [x] **(feedback fix)** cancel-early empty thread: `c` on a thread-less block
      opens the composer through `ensureThread`, which creates an empty thread;
      cancelling before anything was committed left a "no comments yet" row
      behind. `dismiss` now drops a thread it freshly created for the open
      (`m.freshAnchor`, set by `ensureThread`) when the composer closes with no
      posted comment, no draft and no resolved flag — every cancel path
      (double-`esc`, empty submit, `:q!`, blur) — putting focus back on the
      block the aborted comment was about. A thread that existed before the
      open is never dropped: `:q!` on a resumed draft still discards only the
      draft, not the thread (pin `TestSpacemacsDiscard`), a kept draft keeps
      its fresh thread, and a cancelled edit keeps the thread it belongs to.
      Also covers the composer-failed-to-open edge. Drains the cancel-early
      item of `vault/feedback/2026-08-09-composer-exit-and-thread-feedback.md`;
      the unchanged-edit-draft and enter/esc dive items remain.
      `[felt]` — see journal 2026-08-09.40 — done 2026-08-09

- [x] **(feedback fix)** composer edit cursor-at-end: `e` on a posted comment
      opens the composer in normal mode with the cursor on the last character
      of the comment text — `nvim_win_set_cursor(0, {last, #buf[last]})`, one
      column past the last byte, which nvim clamps onto the last character
      (F22) — so `a` appends at the end instead of after the first character.
      The same branch change lands interaction.md's settled draft-resume rule
      ("cursor after the existing text") that the code had quietly never
      implemented for non-empty buffers; empty and line-ref-prefix branches
      untouched. Drains the edit-cursor-at-end item of
      `vault/feedback/2026-08-09-composer-exit-and-thread-feedback.md`; the
      cancel-early-empty-thread, unchanged-edit-draft and enter/esc dive items
      remain. `[felt]` — see journal 2026-08-09.39 — done 2026-08-09

- [x] **(feedback fix)** composer insert-after-line-ref: `c` after a visual
      selection or a line dive opens the composer already in insert mode with
      the cursor past the auto-appended line reference (`L3-6: `), so the first
      keystroke is a character instead of an `i`/`a` motion. One new branch in
      `composerInit`'s `VimEnter` callback, ahead of the emptiness check: a
      buffer that is a single line matching exactly `L12: ` / `L12-18: `
      (trailing whitespace optional) opens with `:startinsert!` (the `A` form —
      append at end of line then insert), because nvim clamps any cursor column
      *onto* the last character and plain insert would sit the reviewer in
      front of the reference's trailing space (F22). The carve-out is
      shape-exact: a prefix with real text behind it — a resumed draft, an
      edit — still opens in normal mode, the settled insert-iff-empty rule
      unchanged. What I rejected: a host-side "I scaffolded this" flag threaded
      through `newComposer`, keeping the whole mode decision in the one
      callback. Drains the first item of
      `vault/feedback/2026-08-09-composer-exit-and-thread-feedback.md`; the
      edit cursor-at-end, cancel-early-empty-thread, unchanged-edit-draft and
      enter/esc dive items remain. `[felt]` — see journal 2026-08-09.38 — done
      2026-08-09

- [x] **(feedback fix)** esc-based composer exit: `<esc>` in normal mode closes
      the composer keeping a draft. One nvim mapping swap in `composerInit` —
      `map('n', '<Esc><Esc>', draft)` becomes `map('n', '<Esc>', draft)` — and
      the double-esc/single-esc split falls out of where each composer opens:
      a new comment opens in insert mode, so the first `<esc>` leaves insert
      mode and the second exits; an edit opens in normal mode, so a single
      `<esc>` exits. The old `<Esc><Esc>` effectively needed three presses for
      a new comment and made a lone normal-mode esc wait out `timeoutlen`; an
      exact `<Esc>` mapping fires immediately. Draft stays the default outcome
      (interaction.md's settled dismissal table); discard is still `:q!` /
      `SPC c k`, and `:q` / `SPC c d` / `ZZ` / `<C-s>` / `<C-enter>` are
      untouched. `blur` is unchanged and still one path (F7): its leading
      `\x1b` is now itself the exit in normal mode, same `cquit 2`. The footer
      hints split — `esc esc keep` under a new comment, `esc keep` under an
      edit (`escHint`, review.go) — and the help/README text drops the
      `<Esc><Esc>` spelling. Drains the first item of
      `vault/feedback/2026-08-09-composer-exit-and-thread-feedback.md`; the
      line-reference insertion, cursor-at-end, cancel-early-empty-thread,
      unchanged-edit-draft and enter/esc dive items remain.
      `[felt]` — see journal 2026-08-09.37 — done 2026-08-09

- [x] **(feedback feature)** `<num>gg` / `<num>G` source-line navigation:
      `42gg` and `42G` jump to a 1-based markdown source line (vim's two
      spellings of one motion), landing focus on the first block whose source
      range contains it — thread rows carry no line numbers, so a jump can
      never land on one — and centring the block in the viewport with
      `scrollAnchor` updated (the search jump's mechanics). Digits buffer in
      the status line (`line 42`) and only `gg`/`G` consume them; any other
      key clears the buffer and acts exactly as it would with no count typed
      (`42j` is a plain `j`). Lines no block covers clamp to the nearest block
      with a `no source line N — jumped to the nearest block` status; the
      buffer caps at six digits. Counted `g` becomes a prefix waiting for its
      second `g` instead of the eager first-block move. Drains feature 2 of
      `vault/feedback/2026-08-09-navigation-feature-requests.md`; the rich/raw
      toggle feature remains. `[felt]` — see journal 2026-08-09.35 — done
      2026-08-09
- [x] **(feedback feature)** rich/raw mode toggle: `\` (`view.raw`) switches
      between the rendered document and the raw markdown source of the same
      file. Raw mode renders `m.src` verbatim — one rendered line per source
      line, no re-wrapping, no inline styling — kept on the model because
      `loadDoc`/`loadDocFrom` now return the source alongside the blocks. Each
      block's span is its source-line range, so hit-testing, clicks, page keys
      and focus-following scroll behave identically; the gutter (focus bar +
      mark rule) rides the block's source lines. Focus survives the switch by
      anchor — rebuild drops thread rows while raw is on, since threads are
      conversation, not document — and the viewport re-anchors to it; visual
      selection and dives are cancelled, `l`/`h` are inert. A seed model has no
      source, so the toggle refuses with a status. Drains feature 3 of
      `vault/feedback/2026-08-09-navigation-feature-requests.md`; the file is
      deleted. `[felt]` — see journal 2026-08-09.36 — done 2026-08-09

- [x] **(feedback fix)** frontmatter keyboard reachability and horizontal
      scroll: the frontmatter block is a focus stop now — entry 0, so `g`/`j`
      land on it and focus-following scroll brings it back into view — and a
      field wider than the measure scrolls horizontally with `h`/`l` (the
      code-block treatment) instead of being truncated with an ellipsis.
      `rebuild` keeps it in `m.entries`, `render` draws it through the shared
      block loop (gutter focus bar included), and `scrollBlock` generalises
      the code-block scroll to cover it; the "no interaction" guarantee moves
      from the entry list to `commentable()`/`markable()`, which still exclude
      it, so it cannot be commented, marked, or counted in review progress.
      Search deliberately still excludes it (unchanged from 2026-08-09.32) —
      metadata is reachable but not a target — flagged in the demo recipe.
      Drains `vault/feedback/2026-08-09-frontmatter-feedback.md`; the file is
      deleted. `[felt]` — see journal 2026-08-09.34 — done 2026-08-09

- [x] **(feedback fix)** CLI commands for agent automation — the extract half:
      a dedicated non-interactive `margin export FILE.md [--include-resolved]`
      command (`review.Export` + a top-level `export` subcommand) prints the
      review as it stands on disk without driving the TUI — the same
      `exportReview` text `Y`/`--stdout` produce, byte-identical, so an agent
      reads the current state of a review in a pipe with no terminal. Marks are
      session-only, so a headless export reports no marks (summary reads `0 of
      N blocks reviewed`) — decided, not raised. Drains the last bullet of
      `vault/feedback/2026-08-09-additional-ux-and-cli-feedback.md`; the file
      is deleted. `[mech]` — see journal 2026-08-09.33 — done 2026-08-09

- [x] **(feedback feature)** `/` search with match highlighting: `/` opens a
      prompt in the palette's slot (dim rule, `/draft█`, live count); typing
      highlights every match as it is typed, case-insensitively, over the
      *rendered* lines (wrapped prose, aligned tables, chroma code — the query
      searches what is on screen) with a background-only SGR (`48;5;94`, warm
      brown — distinct from the selection's slate and the mark colours) so the
      line's own foreground survives, reasserting across inner resets the way
      selLine does. `enter` commits the draft and jumps to the next match
      strictly after the current focus line, centring it in the viewport with
      `scrollAnchor` updated so clampScroll does not yank the offset back;
      `n`/`N` (`search.next`/`search.prev`, new registry commands) walk the
      list with wrap; `esc` cancels the prompt's edit without committing (a
      previous query and highlight survive); backspace deletes a rune and
      cancels at an empty draft like the palette. Frontmatter excluded from
      the match list, the same call rebuild already makes. Drains feature 1 of
      `vault/feedback/2026-08-09-navigation-feature-requests.md`; the
      `<num>gg` and rich/raw-toggle features remain.
      `[felt]` — see journal 2026-08-09.32 — done 2026-08-09

- [x] **(feedback fix)** CLI comment add for agent automation: new `margin
      comment add FILE.md --anchor ^abc --text "..." [--author agent]`
      subcommand appends a comment to a thread without driving the TUI —
      `review.AddComment` resolves the root, finds the block by anchor, and
      writes through the same `writeThreadFile`/`marshalThread` the TUI uses,
      so the thread file is byte-identical to one a reviewer typed. The anchor
      must name a commentable block that exists (stale exports fail loudly,
      not silently), accepts with or without the leading `^`, and `--author`
      defaults to the OS user. Drains the add/append half of the "CLI Commands
      for Agent Automation" bullet of
      `vault/feedback/2026-08-09-additional-ux-and-cli-feedback.md`; the
      extract half stays as a residual (already served by `--stdout` and the
      plain-markdown thread files per D5). `[mech]` — see journal
      2026-08-09.31 — done 2026-08-09

- [x] **(feedback fix)** composer background artifacts: nvim's default
      colorscheme paints `Normal` with its own dark background
      (`NvimDarkGrey2`, RGB 20;22;27) on every cell, so the composer pane
      rendered as a dark slab over the document — a "strange black background
      beneath typed text" that left "persistent background artifacts on
      deleted character cells" after backspaces. `composerInit` now forces
      `Normal`, `NormalFloat` and `EndOfBuffer` to `bg = 'NONE'`, so the pane
      renders on the terminal's own background, matching the document;
      erased cells leave nothing behind. Host-side stripping of `48;` SGR
      from `em.Render()` deliberately rejected — the emulator should report
      what the child painted, and the child should simply not paint this.
      Drains the "Vim Mode Background Rendering Artifacts" bullet of
      `vault/feedback/2026-08-09-additional-ux-and-cli-feedback.md`; the CLI
      automation bullet remains. Recorded as F21.
      `[felt]` — see journal 2026-08-09.30 — done 2026-08-09

- [x] **(feedback fix)** tunable mouse wheel speed: mouse wheel scrolls a
      configurable number of lines per tick (`--wheel-speed N`, default 3,
      clamped to at least 1; 0 means the default). `handleWheel` reads
      `m.wheelSpeed` instead of a literal 3, `RunOptions.WheelSpeed` threads
      the value through from `cmd/margin`. Drains the "Mouse Wheel Speed"
      bullet of `vault/feedback/2026-08-09-todo-review-feedback.md` — the
      file's last undrained item, so the file is deleted.
      `[felt]` — see journal 2026-08-09.29 — done 2026-08-09

- [x] **(feedback fix)** dive into multi-line blocks: `l` on a table or raw
      block dives into its source lines, `j`/`k` walk them within the block's
      source range, `h` surfaces, and `c`/`gy` anchor a comment to the line
      under focus (`L<n>: ` prefix / `file:L<n>` yank) — the L12-18 payload
      that made a line worth diving to. Code blocks keep `l`/`h` for horizontal
      scroll; wrapped prose (paragraphs, quotes) has no line identity and stays
      out of the dive. Drains the "Dive into multi-line blocks" bullet of
      `vault/feedback/2026-08-09-todo-review-feedback.md`; the mouse wheel
      speed bullet remains.
      `[felt]` — see journal 2026-08-09.28 — done 2026-08-09

- [x] **(feedback fix)** code block horizontal scroll: code blocks don't wrap lines,
      `h`/`l` (`move.dive`/`move.surface`) scroll code blocks horizontally inside the block
      (4 columns per press, bounded by max visual width - content width). Drains
      `vault/feedback/2026-08-09-deleted-comments-and-code-hscroll-feedback.md`.
      `[felt]` — see journal 2026-08-09.27 — done 2026-08-09

- [x] **(feedback fix)** deleted comments keep content & restore option:
      `deleteComment` preserves comment body, `marshalThread` writes `[deleted]` in
      header line, `appendComments` renders body text dimmed, `D` toggles delete/restore
      immediately without confirmation. Drains "Deleted comments keep their content"
      bullet in `vault/feedback/2026-08-09-deleted-comments-and-code-hscroll-feedback.md`.
      `[felt]` — see journal 2026-08-09.26 — done 2026-08-09

- [x] **(feedback fix)** thread comment focus highlight: focusing a comment
      places focus bar and highlight on comment body text lines rather than author
      header. Drains the "Thread Comment Focus Highlight" bullet in
      `vault/feedback/2026-08-09-additional-ux-and-cli-feedback.md`.
      `[felt]` — see journal 2026-08-09.25 — done 2026-08-09


- [x] **(feedback fix)** palette visual separation & hover visibility: prepends
      dim horizontal rule `─` to command palette and brightens mouse hover indicator
      color to 248 (`hoverStyle`). Drains the "Visual Separation" and "Mouse Hover
      Visibility" bullets in `vault/feedback/2026-08-09-todo-review-feedback.md`.
      `[felt]` — see journal 2026-08-09.24 — done 2026-08-09

- [x] **(feedback fix)** focus retained on comment exit: exiting composer (`dismiss`)
      leaves focus (`m.at.comment`) on the newly posted or edited comment so
      pressing `e` immediately re-edits it. Drains the "Focus Retained on Comment Exit"
      bullet in `vault/feedback/2026-08-09-additional-ux-and-cli-feedback.md`.
      `[felt]` — see journal 2026-08-09.23 — done 2026-08-09

- [x] **(feedback fix)** line/range prepending in comments & yank reference:
      when commenting (`c`) with an active visual selection (`V`), automatically
      prepends line range (`L12-18: ` or `L12: `) to comment draft; new
      `selection.yankRef` command (bound to `gy` and `yr`) yanks line reference to
      clipboard. Drains `vault/feedback/2026-08-09-visual-mode-range-selection-feedback.md`.
      `[felt]` — see journal 2026-08-09.22 — done 2026-08-09ttled" section for what each still leaves open for a felt leg.

## Done

- [x] **(feedback fix)** .margin directory location: `resolveReviewRoot` walks
      up parent directories from the document path looking for a project root
      marker (`.git` or `.margin`). If found, that directory is `root` and `docPath`
      is derived relative to it; falls back to current working directory. Drains
      the ".margin directory location" bullet of `vault/feedback/2026-08-09-todo-review-feedback.md`.
      `[mech]` — see journal 2026-08-09.21 — done 2026-08-09

- [x] **(feedback fix)** J/K incremental viewport scrolling: `J`/`K` scroll
      the document viewport 3 lines per press without changing focus (`m.at`).
      Registered as `move.scrollDown` and `move.scrollUp` commands. Drains the
      "Incremental Scroll Keys" bullet of `vault/feedback/2026-08-09-todo-review-feedback.md`;
      the file's other bullets (.margin location, palette separation, wheel
      speed, hover visibility, multi-line block dive) stay for future legs.
      `[felt]` — see journal 2026-08-09.20 — done 2026-08-09

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
