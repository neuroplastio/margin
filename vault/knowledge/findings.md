# Findings

Hard-won technical facts. Every entry here cost real debugging, and most were
found by a human looking at a screen, not by a test. **Read F29–F33 before
touching the panes (D22).** F1–F4, F7, F8, F19–F22 and F26 are about the nvim
composer D22 removed, and stand as history.

Append only. Do not delete an entry because you think it is fixed — add a note.

---

## F1 — `x/vt` silently drops keys with modifiers

*Historical (2026-10-18): the nvim composer, its pty and its emulator are gone (D22).*

`vt.Emulator.SendKey`'s fallback branch is:

```go
default:
    if key.Mod == 0 { seq += string(key.Code) }
```

Any key carrying a modifier it has no explicit case for produces **nothing**.
There is a `TODO: Support Kitty, CSI u, and XTerm modifyOtherKeys` right above
the switch.

This matters because Ghostty speaks the Kitty keyboard protocol, so Bubble Tea
decodes `shift+o` as `{Code:'o', Mod:ModShift}` rather than a bare `'O'`. Under
that terminal, **every capital letter and every shifted punctuation mark
vanishes**. `O`, `A`, `D`, `!`, `?` — all silently ignored.

**The workaround** (`composer.sendKey`): printable characters carry their literal
in `Key.Text`, so send that with `SendText` and skip vt's encoder entirely.
Special keys with modifiers get encoded by hand as xterm `CSI 1;<mod><final>`.

*2026-08-09 addition:* the same fallback has a sharper failure mode for alt —
vt prefixes ESC for alt and strips the bit before matching, so an alt combo
carrying a second modifier it has no case for emits a **bare ESC** (nvim reads
it as Escape and leaves insert mode). `encodeModifiedKey` now encodes every
modified-key family itself; see F19 for what the child actually decodes.

`TestVtDropsModifiedKeys` pins the upstream behaviour: it *skips* if vt is ever
fixed, so the workaround can be revisited rather than silently kept forever.

**Why a test did not catch it originally:** the first test suite sent synthetic
keys with `Mod: 0`, which validated our plumbing and never exercised the encoding
the real terminal produces. A human pressing `shift+O` found it in seconds.

## F2 — An nvim init script runs *before* the file argument is opened

*Historical (2026-10-18): the nvim composer, its pty and its emulator are gone (D22).*

Anything in `-u <init>` that inspects the buffer sees an **empty** buffer, because
nvim has not loaded the file yet.

This silently broke "insert mode iff the buffer is empty": the check always found
it empty, so editing an existing comment always opened in insert mode.

**Fix:** do buffer-dependent setup in a `VimEnter` autocmd (`once = true`).

## F3 — `vt.Emulator` has no lock; use `vt.SafeEmulator`

*Historical (2026-10-18): the nvim composer, its pty and its emulator are gone (D22).*

The pty reader goroutine writes into the cell grid while `View()` reads it from
the Bubble Tea goroutine. The plain `Emulator` has no synchronisation, and the
resulting torn reads show up as screen flicker.

`vt.NewSafeEmulator` is the same type with an `RWMutex`. Always use it. Run
`make test-race` on any composer change — this class of bug is invisible
otherwise.

## F4 — Redraws must be damage-driven, not polled

*Historical (2026-10-18): the nvim composer, its pty and its emulator are gone (D22).*

Bubble Tea's renderer already caps at `defaultFPS = 60` (~16.7ms). Putting a
polling ticker on top of that adds a **second, unsynchronised** frame quantum:
a keystroke waits for the tick to notice the change and then for the renderer's
next frame. That measured as roughly 33ms of pure waiting, felt as sluggishness.

**The shape that works:** the pty reader is a hand-rolled pump, not `io.Copy`, so
it can signal a buffered channel of one after each write. A `waitForDamage`
command blocks on that channel, so a redraw is scheduled the instant the child
emits bytes. The buffer of one coalesces bursts into a single redraw. A slow
heartbeat (500ms) remains only as a safety net. `tea.WithFPS(120)` halves what
is left.

For reference, the host round trip — keystroke in, child's echo back, damage
signalled — measures a **median well under 1ms**. If latency is ever suspected
again, measure before blaming a layer: `TestDamageArrivesPromptly` guards this
floor, and asserts on the median rather than the worst sample so a scheduler
hiccup or `-race` overhead does not fail it.

## F5 — Derive layout, hit-testing and the cursor from one render pass

The cursor once sat exactly one row above the text because the pane's origin was
computed from a formula (`1 + len(docAbove) + 1`) while the lines were produced
separately — and the quoted block soft-wrapped to a variable height the formula
did not know about.

**The rule:** `render()` produces the lines *and* the spans in a single walk.
Everything downstream — click routing, cursor placement, scroll clamping — reads
those spans. Never recompute a position independently.

`TestSpansTileTheDocument` asserts the spans tile the output with no gaps or
overlaps, which is what makes clicks land on the right block.

## F6 — Terminal output is styled; strip ANSI before matching in tests

`em.Render()` returns styled runs, and nvim's spell checker inserts underline
codes **mid-word**. A test doing `strings.Contains(screen, "ZZZ-marker")` failed
on text that was plainly on screen, because the raw string was
`ZZZ-\x1b[4mmarkerThis\x1b[24m`.

Use the `plainScreen` helper. Anything asserting on visible text must strip ANSI
first.

## F7 — `:cquit N` is the whole IPC

*Historical (2026-10-18): the nvim composer, its pty and its emulator are gone (D22).*

The composer reports its outcome purely through the child's exit code: 0 submit,
2 keep as draft, 1 discard. No socket, no protocol, no temp-file handshake. git
has used the same channel to abort commits for decades.

The corollary is that **blur is not a separate mechanism**: the host sends
`\x1b:MarginDraft\r` and the child exits 2, which is byte-for-byte what
`esc esc` does. One persistence path, not two. Resist any change that gives blur
its own code path.

## F8 — Without nvim the test suite lies

*Historical (2026-10-18): the nvim composer, its pty and its emulator are gone (D22).*

Roughly two thirds of the tests call `requireNvim(t)`, which is `t.Skip()`. On a
machine with no nvim the suite still reports `ok`. A green run in an
unprovisioned environment — a fresh container, a cloud agent, a new laptop —
proves almost nothing about the composer.

`make doctor` (`scripts/setup-env.sh --verify`) exists for this. It runs one
composer test and demands an explicit `--- PASS`, treating `--- SKIP` as a
failure.

Provision with `scripts/setup-env.sh`. Two details that matter:

- **Install the tarball, not the AppImage.** AppImages need FUSE, which
  containers usually lack.
- **Spell files.** The composer sets `spell = true`. With no spell file nvim
  prompts to download one, and a modal prompt inside a ten-row pane would wedge
  it. The official tarball ships `runtime/spell/en.utf-8.spl`, so this is handled
  — but a hand-rolled minimal install can miss it. The `.sug` companion is
  optional and only affects `z=` suggestion quality.

## F9 — A fine-grained PAT cannot grant write outside its resource owner

**Superseded diagnosis.** This entry previously said the cloud run could not push
because the org had not authorised the GitHub App, and described a patch-handoff
workaround. That was wrong, and it sent two rounds of effort at the org settings
page. What follows is what was actually true. The run pushes normally now.

A cloud session's git credential is minted from the GitHub connection on the
Claude account — the one `/web-setup` syncs from the local `gh` CLI. So the
session can do exactly what that token can do, and nothing more.

**A fine-grained PAT is scoped to a single resource owner, fixed at creation.**
A token whose owner is the user account has no write access to org-owned
repositories at all, whatever the org permits and whatever the token's own
repository-access setting says. Its "All repositories" option means *all
repositories you own*; everything else falls under the fine print, "also includes
public repositories (read-only)". No org setting can widen that, because the
constraint is on the token.

Replacing it needs three things, and missing any one produces the identical
symptom:

1. A **new** token with the organisation as its resource owner. The existing one
   cannot be converted.
2. **Org approval** of that token, if the org requires it.
3. **Contents: Read and write.** A token defaults to Metadata only, which reads
   fine and writes nothing.

### How to tell these apart

Two readings that look like evidence and are not:

- `gh api repos/<owner>/<repo> --jq .permissions` showing `push: true` reports
  **your** access to the repository, not the token's. It says nothing.
- A successful read of a **public** repo says nothing either — any token can read
  those. Read a *private* repo in the org to prove the grant is approved.

The only honest test is a real write. This one leaves no visible artifact and
needs no cleanup, because an unreferenced blob is simply garbage-collected:

```bash
gh api -X POST repos/<owner>/<repo>/git/blobs -f content=probe -f encoding=utf-8
```

A sha back means write access. `403 Resource not accessible by personal access
token` means one of the three requirements above is missing.

**Local `git push` proves nothing about the cloud path.** It goes over SSH with
the maintainer's key; the cloud session goes through a proxy using minted,
scoped credentials. The two share no configuration, so "it works from my machine"
is not evidence — the run that reasoned that way concluded the environment was at
fault when the token was.

### If a run ever cannot push again

It commits locally and hands back `git format-patch origin/main..HEAD` as an
artifact. To land it:

```bash
git checkout main && git pull origin main
git am --3way 0001-*.patch        # preserves author, message and trailers
make check && make test-race && make doctor
git push origin main
```

`git am` is what keeps the leg's authorship intact — do not reconstruct the
change by hand from the diff. And verify before pushing: a patch that applies
cleanly has still only been tested in the environment that produced it.

The patch is reachable at `claude.ai/code/artifact/<uuid>`, which can be fetched.
The `claude.ai/api/organizations/.../files/.../contents` URL for the same content
returns 403.

## F10 — `View` mutates state, so the paint before `WindowSizeMsg` poisons it

Bubble Tea calls `View()` once **before** the first `WindowSizeMsg`, while the
terminal size is still zero. `View` clamps `m.scroll` as a side effect of
rendering, and against a zero-height viewport that clamp landed on `1`. Nothing
ever reset it, so the document's first line — its opening heading — stayed hidden
for the rest of the session.

`View` now returns nothing until `m.w` and `m.h` are known.

**Why it survived several rounds of probing.** Every check rendered *once*, at a
known size, and every one passed: `m.scroll` was 0, focus was on the first block,
the first rendered line was the heading. The model is only wrong after being
asked to render **twice, the first time before it knew how big it was** — a
sequence no single-render test reproduces.

Two lessons worth more than the fix:

- **A render function that mutates state can be poisoned by its own first call.**
  `clampScroll` has to run somewhere, but running it inside `View` means the
  earliest, least-informed render sets a value every later one inherits. Treat
  any assignment inside `View` as suspect.
- **Reproduce the real call sequence, not just the real inputs.** Setting
  `m.w, m.h` directly and calling `View` looks equivalent to what the program
  does and is not: it skips the zero-size paint that Bubble Tea always performs.

This is the third defect in this project that a passing test reported as fine
(see F1, F8). The pattern is consistent: anything about *the frame as a whole* —
its height, its offset, what actually reaches the screen — is invisible to tests
that ask the model questions. A screenshot has now been decisive twice.

## F11 — `goldmark.New()` with no extensions silently drops GFM syntax

`goldmark.New()` with no options speaks CommonMark only. Tables, strikethrough,
autolinks and task lists are all GFM extensions, not CommonMark — without
`goldmark.WithExtensions(...)`, a table never becomes an `*ast.Table`. It parses
as an `*ast.Paragraph` instead, since the pipes and dashes are just text to a
CommonMark-only parser, and `collapse()` then does to that paragraph exactly
what it is supposed to do: joins its lines into one and wraps them to the
measure. The result — a table flattened into a single wrapped run of `|` and
`---` — looked like a bug in `collapse()` or in the block model. It was neither;
the block model never saw a table at all.

`goldmark.New(goldmark.WithExtensions(extension.GFM))` (parse.go) fixes it.
Worth remembering for any future format: **a parser only produces the node
kinds it was told about.** A block that "renders wrong" is worth checking
against `md.Parser().Parse(...)`'s actual AST before assuming the bug is
downstream — an unenabled extension does not error, it just quietly reclassifies
the input as something else.

## F12 — A bucket named "undecided" accretes wrong behaviour, not just missing behaviour

`blockRaw` was documented as "any block type whose rendering has not been
decided yet" and treated its whole membership — lists, code fences, quotes,
tables — identically: shown verbatim, never re-wrapped. That was correct for a
fence or a table, where the layout *is* the content, and wrong for a list, whose
items are prose. The rule was written for the bucket's first two members and
never re-examined when a third and fourth joined it.

The tell was in the code review, not the tests: `render()`'s comment justified
verbatim rendering with "for a code fence, the line breaks are the content" —
true, and cited to defend a branch it was only half applicable to. A one-line
justification that names one member of a multi-member bucket is worth checking
against every other member before trusting it.

Fixed by splitting lists into their own kind (`blockList`, document.go) with
their own layout (`wrapList`, review.go) rather than adding a special case
inside `blockRaw`'s branch — a bucket is easier to keep honest when each member
that behaves differently gets its own name.

## F13 — `stampID`'s marker attaches to "whatever block was appended last," not to a byte position

ID-01's marker recognition (`markerID`, parse.go) does not match a `<!--
margin:^id-->` comment to the block it follows by position — it just sets the
anchor on `blocks[len(blocks)-1]`, i.e. whatever `parseDoc`'s loop most
recently appended. That is equivalent to matching by position *only* because,
until now, every block came from a distinct top-level AST sibling with its own
non-overlapping `[start,stop)` range, so "most recently appended" and
"immediately preceding in the source" were always the same block.

Splitting a list into one `blockListItem` per item (line-level-focus feedback,
2026-08-08) broke that equivalence deliberately: every item in a list shares
the *whole list's* range rather than getting one of its own (see
`listItemBlocks`, parse.go — there is no cheap way to give a list item a real
range that does not risk clipping a nested item or continuation line, and
nothing needs it except stamping). Two consequences fall out of that, and both
are worth knowing before anyone tries to design stamping for a sub-list
position:

- Stamping one item and reparsing does **not** give that item back its id —
  the marker lands after the *whole list*, and `markerID` hands it to whichever
  item was appended last, i.e. always the list's final item, regardless of
  which item was actually stamped.
- Stamping every item of a list in the same pass (what `stampAll` would do if
  it did not special-case `blockListItem`) inserts N markers back-to-back
  right after the list, and each subsequent one **overwrites** the previous
  marker's attachment on that same last item — the earlier items never get
  their own id at all, silently.

`stampAll` sidesteps this by never stamping a `blockListItem` (see the guard in
parse.go and blockListItem's doc comment in document.go) rather than trying to
fix `markerID`'s position-blindness — that would need a real per-item byte
range and a decision about what a marker inserted *inside* a list does to the
list's own CommonMark parsing (an indented HTML comment risks becoming part of
the preceding item's content; an unindented one risks closing the list
early). Left for whoever designs durable sub-block anchoring.

## F14 — a GFM table cell's `Lines()` is its own raw text, not a range needing `extent()`

Every other node type this package reads (`ast.Paragraph`, `ast.Blockquote`,
`ast.FencedCodeBlock`, the table itself) needs `extent()` to get a usable byte
range: `n.Lines()` on those either excludes delimiters that matter for
stamping (a fence's ` ``` `) or is empty for a container node whose content
lives on its children.

`*eastast.TableCell` (`github.com/yuin/goldmark/extension/ast`) is the
exception: `cell.Lines()` returns the cell's own raw markdown directly — for
`| Likes **bold** things |` the cell's `Lines()` gives back `"Likes **bold**
things"` byte-for-byte, markup and all, with no need to walk its inline
children or call `extent()`. That is what let `cellRaw` (parse.go) be a plain
segment-concatenation loop, the same shape as `codeLinesFor`, and is also what
let a table cell reuse `parseInline` (RENDER-06's paragraph-markup stripper)
unchanged in `cellText` — the input is already exactly what `parseInline`
expects a paragraph's raw text to look like.

## F15 — lipgloss `Width` includes the border; a box narrower than its content re-wraps it

`Style.Width(n)` sets the *total* width, borders included. A rounded-border box
declared `Width(74)` has a 72-column content area. That is intuitive until it
isn't: the composer pane's emulator is created at `contentWidth()-2*borderW`
columns, but `threadLines` originally rendered it through a box declared at
`Width(w-2*borderW)` too — so the box's content area came out **two columns
narrower** than the emulator feeding it. The emulator's rows are already
wrapped to its own width, and lipgloss then re-wrapped each of those rows at
the narrower width: a word in the last two columns got orphaned onto its own
line, and the box's rows stopped aligning one-for-one with the emulator's, so
the host-drawn cursor (computed from emulator coordinates) landed on the wrong
text. The 2026-08-09.8 leg fixed it by declaring the box `Width(w)`.

**The rule:** when a box's content is a grid produced elsewhere at a known
width, size the box at `contentWidth` — the content area is then
`contentWidth - 2*borderW`, exactly the grid's width. Declaring the box at the
content width instead double-counts the border and makes lipgloss re-wrap.
"Box narrower than its content re-wraps silently" is the sort of defect no
state-inspection test sees: the fix only shows in a rendered frame, and
`TestComposerBoxDoesNotRewrap` had to compare rendered rows against emulator
rows to catch it.

## F16 — the ctrl+enter modifier only exists if the terminal reports it

`ctrl+enter` submits the composer, but only because the host intercepts the
key before nvim does: a plain terminal sends CR for both enter and ctrl+enter,
so nvim cannot tell them apart. The host sees the difference only when the
terminal reports the modifier — under Ghostty, via the kitty keyboard protocol
(`CSI 13;5u`, which decodes to `{Code:13, Mod:ModCtrl}`) or xterm
modifyOtherKeys (`CSI 27;5;13~`, same decode). On any terminal that reports
neither, ctrl+enter is indistinguishable from enter and the intercept never
fires; the key just inserts a newline.

The test suite had a blind spot here that mirrors F1's: `TestCtrlEnterSubmits`
built the key by hand (`tea.Key{Code: uv.KeyEnter, Mod: uv.ModCtrl}`) and fed it
straight to the composer, which proves our *handling* but not that the real
terminal path ever produces that key. `TestCtrlEnterDecodesThroughRealReader`
(2026-08-09.10) closes it: it feeds the raw kitty sequence through the same
`uv.TerminalReader` bubbletea v2 uses, decodes it, routes it through
`handleKey`, and asserts the composer submits. The decode is now pinned — what
remains untestable is the maintainer's terminal actually reporting the
modifier, which is a property of the terminal, not of margin.

## F17 — one O_RDWR `/dev/tty` handle serves bubbletea for input *and* output

When a pipe owns a standard stream, margin runs the interface on the
controlling terminal instead: `--stdout` points `tea.WithOutput` at
`/dev/tty` (stdout carries the review), and `--stdin` additionally points
`tea.WithInput` at it (stdin carries the document). The thing worth knowing
is that a **single** `os.OpenFile("/dev/tty", os.O_RDWR, 0)` handle can be
handed to both options at once — the terminal is duplex, and bubbletea reads
and writes it independently. Two separate opens also work but buy nothing.

The companion trap: `--stdin` run with a *terminal* on stdin (someone typed
`margin -` with no pipe) must be rejected before reading, or margin swallows
keystrokes until EOF and then opens a review of whatever was typed. The check
is `os.Stdin.Stat()` + `os.ModeCharDevice` — no new dependency, and `/dev/null`
is a char device too, so the guard is unit-testable without a pty.

## F18 — lipgloss does not reassert an outer style across an inner reset

Wrapping an already-styled string in a second lipgloss style does not compose
the way nesting suggests: the outer style opens, then the *first* inner SGR
reset (`\x1b[m` / `\x1b[0m`) kills the outer attributes for the rest of the
string. Verified empirically against v2.0.5 —
`bg.Render(fg.Render("a") + " b")` emits the background opener, the inner
foreground, `a`, a bare reset, then ` b` with **no** background. Any leg that
wants a background (or any uniform attribute) painted across pre-styled
content — chroma code, RENDER-06 inline markup — must reassert it after every
inner reset instead of wrapping. `selLine` (`review.go`, visual selection,
2026-08-09.16) is the working example: open the background, replace every
inner reset with reset+opener, terminate at the end. The same trap applies to
chroma's own output, which resets per token.

## F19 — what a child nvim decodes, and the meta-key catch

*Historical (2026-10-18): the nvim composer, its pty and its emulator are gone (D22).*

Verified against nvim 0.12 on a pty, `TERM=xterm-256color`, no kitty
advertisement, by feeding bytes and watching mapped callbacks fire
(2026-08-09.19):

- nvim's input layer decodes xterm (`CSI 1;<mod><final>`), tilde
  (`CSI <n>;<mod>~`) and kitty CSI-u (`CSI <code>;<mod>u`) forms **without any
  protocol negotiation** — for shift and ctrl modifiers. `CSI 127;5u` arrives
  as `<C-BS>`, `CSI 97;6u` as `<C-S-a>`, `CSI 3;5~` as `<C-Del>`. So the host
  can forward modified keys in these forms and they just work.
- **Meta (alt) keys are different: nvim resolves them only when a mapping
  names the key, in every encoding tried** — legacy ESC-prefix (`ESC x`),
  ESC+CSI, and CSI-u with alt in the modifier all behave alike. Unbound, the
  combo collapses: nvim processes a bare `<Esc>` (insert mode ends) and the
  remaining bytes execute as normal-mode commands. This is terminal-vim
  semantics, not a margin bug, and no byte encoding fixes it from the host
  side. What fixes it for a specific key is *binding* it — the composer maps
  `<C-BS>` and `<M-BS>` to `<C-W>` — or the emulator speaking the kitty
  keyboard protocol to the child, which x/vt does not implement (its TODO
  covers the encode side too, F1).
- vt.SendKey's own alt path (ESC prefix + strip alt) is correct for alt-only
  keys but emits a **lone ESC** for alt plus a second modifier — the "drops
  into normal mode unexpectedly" report. `encodeModifiedKey` handles every
  alt family itself now.

Probe methodology that worked: spawn nvim with the real `composerInit` plus a
`vim.keymap.set` whose callback writes a mark file, wait for a `VimEnter`
ready-file, write the candidate bytes to the pty in one write, and check the
mark. A mapping firing is unambiguous — nvim decoded exactly that key.
`vim.on_key` is *not* a reliable substitute: it logs what nvim processed, and
for unbound meta combos that is a lone `<Esc>` with the rest silently
consumed, which reads as if the bytes never arrived.

## F20 — the emulator's output is a synchronous io.Pipe

*Historical (2026-10-18): the nvim composer, its pty and its emulator are gone (D22).*

`vt.Emulator`'s `SendText`/`SendKey` write into an `io.Pipe`: the call blocks
until something Reads. The composer never notices because its
`io.Copy(ptmx, c.em)` goroutine is always reading. A test that calls
`sendKey` without a concurrent reader deadlocks — start the read goroutine
*before* the send (see `sendKeyBytes` in composer_keys_test.go).

## F21 — nvim paints the pane with its own background unless told not to

*Historical (2026-10-18): the nvim composer, its pty and its emulator are gone (D22).*

Even stripped to a textarea (`-u composerInit`, no colorscheme loaded), nvim
paints the whole pane with the default colorscheme's `Normal` background —
`guibg=NvimDarkGrey2`, RGB `20;22;27` — on *every* cell, empty rows included.
The vt emulator faithfully records that as a `48;2;20;22;27` SGR on every
cell, so `em.Render()` came out as a dark slab with a different background
than the document around it, and erased characters kept the background where
the text had been — the "strange black background beneath typed text /
persistent artifacts on deleted character cells" feedback.

The fix is nvim-side, not host-side: force the background-bearing highlight
groups to transparent in `composerInit` —
`nvim_set_hl(0, 'Normal'|'NormalFloat'|'EndOfBuffer', { bg = 'NONE', ctermbg = 'NONE' })`.
(`EndOfBuffer` is the filler past the last line; `fillchars.eob` only changes
its glyph, not its background.) With that, the pane renders plain text and
the terminal's own background shows through, matching margin's document. A
host-side attempt (stripping `48;` from `em.Render()`) would be wrong: it
would break legitimate backgrounds the moment a real colorscheme or a visual
selection wants one. `set_hl`'s valid keys are `bg`/`fg`, not `guibg`/`guifg`
— the `gui` prefix names the *attribute source*, not the dictionary key, and
`nvim_set_hl` errors `E5113: invalid key: guibg`.

## F22 — you cannot put the cursor past the end of a line; `startinsert!` is how you append

*Historical (2026-10-18): the nvim composer, its pty and its emulator are gone (D22).*

`nvim_win_set_cursor` (0-based column) and Vimscript's `cursor()` (1-based)
both **clamp onto the last character** of the line — there is no column that
lands the cursor *past* it. On `"L3-6: "` (length 6), columns 6, 7 and 8 all
resolve to index 5, sitting on the trailing space. That matters because insert
mode inserts **before the character under the cursor**, so "cursor at the end
of the line" in the API sense is "typing lands in front of the trailing
space": a comment typed after a visual selection came out `L3-6:ship it`
instead of `L3-6: ship it`.

The idiom that works is the documented one: `:startinsert` works like `i`
(insert at the cursor), and **`:startinsert!` works like `A` — append at the
end of the line** (`:help :startinsert`). The VimEnter autocmd's
line-reference branch uses `vim.cmd('startinsert!')` and needs no cursor math
at all. Worth remembering whenever the composer (or any nvim embedding) wants
"open ready to type after existing text" — the 2026-08-09.37 leg used the
plain form for empty buffers, and this one (2026-08-09.38) found the `!`
variant the hard way. Probing tip: `mode()` inside headless `-c` command chains
does not report insert mode reliably (it reads `n` even after a real
`startinsert`), so the pty-backed Go tests — which see the DECSCUSR cursor
shape the child really reports — are the honest check.

## F23 — fsnotify a directory, not a file, if you want to survive save-as

`fsnotify.NewWatcher().Add(file)` watches the **inode**, not the path. An
editor that saves via temp-file-then-rename (write `x.md.tmp`, `rename` it over
`x.md`) breaks the watch: the rename event fires once, and every later save of
the *new* inode is invisible — the watcher is permanently stuck on the file
that was replaced. The robust shape is `Add(directoryOf(file))` and match
events by `filepath.Base(ev.Name) == base`. A directory watch is
**non-recursive**, so it only reports the directory's direct children — which
is exactly the scope a "this one document changed" watch wants, and is what
keeps `.margin/events.log` appends (a grandchild) and sibling documents from
being reported at all.

Attribution gotcha that follows: when one watcher covers two directories (the
document's directory *and* the `.margin/threads/<doc>` leaf), attribute each
event to the directory it came from (`filepath.Dir(ev.Name)`), not to its
suffix — a sibling `.md` in the document's directory otherwise reads as a
thread change. Discovered building the change-notification leg (2026-08-10.15);
see `newThreadWatcher` in `internal/review/watch.go`.

## F24 — upstream mermaid-ascii's graph parser is lenient: unparseable statements render as boxes

The vendored `pkg/graph` (third_party/mermaid-ascii) inherits a parser design
that, unmodified, **turns any statement it does not understand into a node
whose name is the whole line**. `mermaidFileToMap`'s per-line loop swallowed
`parseString`'s error and called `parseNode(line)`, and `parseNode` fell back
to "the whole line is the name" for anything without a trailing `[...]`. So a
valid flowchart using a form upstream did not know — `A --- B`, `A ==> B`,
`A -.-> B`, `A((Start))`, `direction LR`, `linkStyle`, `style`, `click` —
drew a labelled box containing the literal source ("A --- B", "A((Start))",
"direction LR") instead of erroring. That is the "half-parsed diagram" margin
must never render, and it is invisible to the upstream CLI's smoke tests
(a graph with one such box still "renders" and exits 0).

The deltas that fix it for margin are D3 (strict parse — an unparseable
statement fails the whole diagram) and D4 (grammar parity — shapes, the extra
link forms and the skip statements above). Both live in the vendored
`CHANGELOG.md`. The durable lesson: **a renderer whose parser cannot fail is
unusable as a fallback source, because the fallback never triggers.** Any
re-vendor must re-check that every real-world flowchart form either parses or
fails the block — the CLI's green exit proves nothing.

A second quirk, useful for anyone judging the diagrams: edge labels on a
vertical run are drawn centered on the edge, so a vertical connector glyph can
land *inside* the label ("over│here"). Upstream behaviour, flagged in the
demo recipe, not yet a delta.

## F25 — bubbletea v2 runs Update and View for every message; the FPS cap only bounds flush

`tea.WithFPS(120)` bounds how often the renderer **flushes** a frame, but the
event loop still runs `model.Update(msg)` and `model.View()` synchronously for
**every** message (`tea.go`: `model, cmd = model.Update(msg)` then
`p.render(model)`, which calls `model.View()` — unconditionally, even if
Update changed nothing). `View()` is the expensive half here: margin's builds
the whole document layout. So any message that arrives faster than `View()`
runs backs the input queue up, and everything else — keystrokes included —
waits for it to drain.

Mouse motion is the pathological source: with all-motion tracking (1003, the
hover fix) a pointer sweep across the document delivers one `MouseMotionMsg`
per cell, hundreds per second on a fast sweep, each costing a full render.
That is the "hover is laggy / focus events get replayed" report
(2026-08-11.7). The wheel, by contrast, is user-paced and clicks are discrete;
motion is the only continuous flood.

The fix lives **before** Update: `tea.WithFilter(func(Model, Msg) Msg)` runs
in the event loop ahead of Update/View (same `tea.go`, `msg = p.filter(...)`),
and returning nil drops the message entirely — no Update, no View, no render.
margin's `motionThrottle` (internal/review/review.go) drops a motion report
that arrives within one frame period (8.33ms at 120fps) of the last one
processed, or that restates the cell the pointer already occupies. The
tradeoff is structural: processing at most one motion per frame period means
the hover can trail the pointer by up to one frame — the same bound every
frame-rate capping scheme pays, and invisible in practice.

The general lesson: **the FPS option is a floor on work the screen can show,
not a throttle on work the event loop does.** High-rate input that only feeds
the display (hover, cursor blink, a progress bar) must be coalesced at the
message source; leaving it to "the renderer caps at 120fps" floods Update and
View all the same. A filter keyed on message type is the hook bubbletea
provides — there is no built-in per-message-type rate limit.

## F26 — shift+enter does not survive the trip to nvim; fold it onto a newline at the host

*Historical (2026-10-18): the nvim composer, its pty and its emulator are gone (D22).*

The feedback that shift+enter "exits the nvim editor, keeping the draft"
(2026-08-12) looked like a nvim mapping bug but is a decode trap. The host
sees the key as `{Code: KeyEnter, Mod: ModShift}` (only when the terminal
reports the modifier, mirroring F16's ctrl+enter), and forwarding it went
through `encodeModifiedKey`'s CSI-u branch — `\x1b[13;2u`. Whether that byte
stream exits or is dropped depends entirely on the child:

- nvim 0.12.4 (probed on this machine, journal 2026-08-12.2) silently drops
  an unbound `<S-CR>` in insert mode — no newline, no exit, nothing. The
  keystroke simply vanishes, which is a bug in its own right.
- The maintainer's nvim turned it into the draft exit. The sequence starts
  with a lone ESC; nvim processes CSI-u only when a mapping names the key
  (the same meta-key catch F19 documents), so unbound the ESC stands alone —
  and in normal mode ESC is *the* mapped draft-exit.

The fix is host-side and is the same shape as ctrl+enter (F16): intercept
before forwarding, fold shift+enter onto the plain `\r` a bare enter gets.
A single CR is unambiguous to every nvim. The composer's own mappings were
untouched; there is no `<S-CR>` mapping to write because the child never
sees the key. `interaction.md`'s "ctrl+\ and ctrl+enter are the only keys
the host takes" needs widening to include shift+enter, and that line is
bounding until the maintainer judges the newline behaviour.

## F27 — chroma's tty formatter passes a literal tab through; margin must expand it itself

`quick.Highlight(..., "terminal256", ...)` does not expand tabs (unlike the
HTML formatter's `TabWidth`, its tty formatter has no tab option and copies a
`\t` into the styled output verbatim). Sending that to the terminal is the
trap: the terminal resolves a tab against its *absolute* screen tab stops
(every 8 columns by default), so the indentation lands wherever the stops
happen to be relative to the gutter, and — because the gutter shifts every
code line — a "correctly" tab-indented block renders with a leading offset
that does not match the source. Worse, `visualWidth` counts a tab as one
rune while the terminal renders it as up-to-eight columns, so overflow
detection and horizontal-scroll bounds under-measure tab-heavy lines.

The fix is to expand tabs to spaces before anything measures or emits them
(2026-08-12.4): `expandTabsLine` (skip ANSI while counting, reset the column
at a newline, advance to the next multiple of 8) fed into `highlightCode`
before chroma tokenises, into the mermaid-fallback plain path, and into raw
mode's overflow/scroll measurement. Expanding is also the only faithful
option for a terminal renderer: a literal tab *cannot* be shown as itself, so
the choice is between resolved stops and expanded spaces, and only the latter
is deterministic and measurable.

## F28 — the gh CLI's shape for PR comment imports

`gh` is the intended bridge for `margin import-pr` (2026-08-13), and three of
its shapes cost a probe to get right:

- **Run `gh` in the document's directory, not the process cwd.** `gh pr view`
  resolves the repository from the current working directory, so a margin
  invoked from elsewhere in the tree would look up the wrong checkout. ImportPR
  binds the runner to `filepath.Dir(path)`.
- **`gh pr view` fails loudly and its stderr is the useful error.** No PR for
  the branch → exit 1, `no pull requests found for branch "main"`; no git
  remotes → `no git remotes found`. Both messages are what the user needs, so
  runGH wraps and passes gh's stderr through rather than inventing its own.
- **`gh api --paginate` concatenates pages into one JSON array** — a long
  review is not truncated at the first page, and the result is still a single
  array to unmarshal.
- **A review comment's `line` is its head-side blob line; `original_line` its
  base-side line, and only one is set** depending on which side of the diff the
  comment sits on. `--paginate repos/{o}/{r}/pulls/N/comments` (review
  comments) is separate from `issues/N/comments` (the PR's general
  conversation, which has no line and nothing to attach to).

## F29 — the kit's Text folds a typed line break and sets paragraphs flush

A hotty-a2ui Text is Markdown, and in cells it sets it as Markdown does: a
single newline is a soft break, folded into a space (`"one\ntwo"` draws `one
two`), and consecutive blocks — paragraphs, a list after a paragraph — draw
on consecutive rows, with no blank row between them. A backslash escape is
drawn as written (`\_` shows the backslash), so escaping text to keep it
from being Markdown makes it worse. A two-space hard break (`"three  \nfour"`)
holds. A comment, whose every typed line break is meant (journal
2026-08-10.16), goes to the kit as its paragraphs, each its own Text with a
blank row between, each line but the last ending in a hard break
(`commentParagraphs`).

## F30 — a kept surface keeps its document by name

`hottytea.Session.Layout` sends a surface's document only when the name is
new to it; a surface with `Keep: true` that drops out of the layout is hidden,
not deleted, and keeps its document. A pane dropped and made again under the
same name (the composer, each time a comment opens) is then never sent: its
fresh `html.Rendition` has sent no document, so its `Update` is nothing, and
the field never gets the host's keyboard. Dropping a pane deletes its surface
(`model.dropPane` → `Session.Delete`).

## F31 — an HTML rendition says nothing until its document is out

`html.Rendition.Update` returns nothing until `Doc` has been called — which
`Session.Layout` does for a new surface. The `a=focus` that gives a field the
host's keyboard comes from `Update`, so it must be called after `Layout`, in
the same frame (`hotFrame`), as the storybook's `LaidOut` does; called before,
it is lost until something else redraws.

## F32 — tab in cells always "handles"; the keyboard is what tells

`cells.Rendition.Key("tab")` reports the key handled whether or not focus
moved: past the last focusable (or before the first) the surface simply loses
the keyboard (`Controller.St.Keyboard` false), as a host's surface does
(SPEC §10.1). To keep the keyboard in a pane — the composer — check
`St.Keyboard` after the key, not what `Key` returned.

## F33 — `KeyName` reads a capital without shift as Caps Lock

`hottytea.KeyName` names `tea.Key{Code: 'M', Text: "M"}` (no shift) `m`: "a
capital without shift is its small letter, as Caps Lock makes it". A
terminal or a test that reports a capital that way would type a small letter
into a field. margin names a typed character by its text (`fieldKeyName`): a
capital is `shift+` its letter, and text no key names (an IME's) is pasted.

## F34 — usvg reads no CSS custom properties and no `color-mix()`

An SVG sent as an `<img>` resource is painted by usvg (0.48 in hotty-blitz),
not through the page's CSS: `var(--x, #fff)` is not resolved, fallback
included, and `color-mix()` is not a colour. A fill or stroke written with
either is dropped to its default, so an SVG themed that way paints as black
shapes with black text, on any background. Ondine's diagrams are themed
exactly so; `internal/mermaidsvg/flatten.go` writes concrete colours in
before margin sends one. An inline `<svg>` in a surface's own document is
different: Stylo resolves its `currentColor` (hotty-blitz `tests/svg.rs`).


## F35 — a kit surface a row tall draws its text below that row

hotty-a2ui's `.k-surface` has 12px of padding and fills the surface
(`min-height: 100vh`), so on a 20px row a pane's text starts about 21px down.
In a plx pane the relay clips the surface to its row exactly, so the row is
blank; straight in hottyterm it spills into the window's bottom padding,
half drawn. The footer, one row, was the case (2026-10-21). `SetDense` (kit
78a9985) takes the padding off above and below, makes a line one row high
and centres the content. A bar also must not wrap: at 60 columns the footer's
caption wrapped to a second line beside the key hints, which keep their full
width, and the bar was two rows tall again; kit d6d7db1 keeps a dense
surface's text on one line and lets the hints give way. Check a bar's document with
`hotty render doc.html --cols N --rows auto --cell 9.5x20`: it should give 1
at every width. The search prompt had the same problem: in HTML a field's
label sits above its input, and the input is 2.25rem tall with padding, so
the prompt needed 5 rows (3 dense) where margin gives it the 2 it has in
cells. Kit cf085ff (2026-10-22) lays a dense one-line field out as cells
does: the label beside the input, the input a row tall, and a Divider
without its margin. So any pane margin places over its cells rows, fields
and all, is a bar (`pane.bar`), and its document renders the cells rows.

## F36 — a surface takes the pointer from the cells under it

On a hotty host a placed surface gets the pointer (SPEC §9.3), so the
terminal reports no mouse motion over it, and a program that lights cells
beside its surfaces (margin's gutter bar) hears nothing while the pointer
is over prose. Ask for hover (`v=1`, §9.4) on those surfaces and treat
their `hover` as motion. A host sends at most one hover a frame, so an
`out` can arrive after the pointer has moved onto cells or onto another
surface: remember which surface set the mark, and clear it on an out only
from that surface (2026-10-23, journal 2026-10-23.1). The hover path costs
about a frame more than cell motion (p50 49 ms against 33 ms).

## F37 — a host's lost documents come back without their resources

A host that loses its documents (a restart that keeps nothing) has lost
their resources with them. hottytea sends a document again when a
placement finds it gone (ENOENT), but only the program can send its
resources again: on `RelayoutMsg{Lost: true}` (hotty-go 2ebf3ef), Send
every resource in Update, before the frame's Layout, so they go out ahead of
the documents in the same Flush. A loss is reported per placement: a
surface that was off screen at the first report finds its document gone
when it comes back and reports again. In hottytest, `Placement()` is only
the last `place` as sent; hottytea leaves out `f`, `p` and `v` when they
have not changed, so read the surface's `Fit()`, `Press()` and `Hover()`
(2026-10-23, journal 2026-10-23.2).
Since hotty-go 5586fd4 a reported loss takes the hidden surfaces'
documents with it, so a surface off screen at the loss no longer reports
again. One case remains: a surface placed twice before the host's first
ENOENT is read (a scroll down and back up within a frame or two) reports
twice. The first reply's frame sends its document and placement again,
and the second reply, which answers the older placement, is counted
against the newer one. Reported to the hotty agent 2026-10-23.
Fixed in hotty-go aaa1e1b (margin pins 0954c06): a reported loss takes
every document, and an ENOENT read before the host answers the first one
sent again belongs to the same loss. A loss is reported once, and margin's
test checks that each resource goes once after it.

## F38 — a plx pane no one sees answers every measure with EBUDGET

plx (1e94151) answers a pane program's `r=auto` placement itself, after
asking its viewer to lay the document out. A pane that is not shown (a
stack tab or a workspace tab in the background) gets no layer, so the
viewer never draws the surface, and after 500 ms plxd answers the measure
`EBUDGET` "no viewport measured the surface in time". With no viewer at all
it answers nothing. Showing the pane sends the program nothing: no replay,
no late `ok`, no `fit` (plx counts the late rows as heard), and plx sends
no focus events to panes. So margin started in a tab no one was looking at
took every block's EBUDGET as a refusal, and showed cells when the tab was
shown, until a resize changed the width. EBUDGET is "not now", not "never":
margin keeps such a block for the next input (a key, the pointer, a paste,
something done in a surface), up to three asks, and an unanswered wave
(deaf) waits for input the same way (2026-10-23, journal 2026-10-23.4).
Shown without any input, the blocks stay in cells until the user does
something: the rest is plx's, reported to the plx agent.
