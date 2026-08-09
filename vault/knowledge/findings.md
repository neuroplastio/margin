# Findings

Hard-won technical facts. Every entry here cost real debugging, and most were
found by a human looking at a screen, not by a test. **Read F1–F4 before touching
the composer.**

Append only. Do not delete an entry because you think it is fixed — add a note.

---

## F1 — `x/vt` silently drops keys with modifiers

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

`TestVtDropsModifiedKeys` pins the upstream behaviour: it *skips* if vt is ever
fixed, so the workaround can be revisited rather than silently kept forever.

**Why a test did not catch it originally:** the first test suite sent synthetic
keys with `Mod: 0`, which validated our plumbing and never exercised the encoding
the real terminal produces. A human pressing `shift+O` found it in seconds.

## F2 — An nvim init script runs *before* the file argument is opened

Anything in `-u <init>` that inspects the buffer sees an **empty** buffer, because
nvim has not loaded the file yet.

This silently broke "insert mode iff the buffer is empty": the check always found
it empty, so editing an existing comment always opened in insert mode.

**Fix:** do buffer-dependent setup in a `VimEnter` autocmd (`once = true`).

## F3 — `vt.Emulator` has no lock; use `vt.SafeEmulator`

The pty reader goroutine writes into the cell grid while `View()` reads it from
the Bubble Tea goroutine. The plain `Emulator` has no synchronisation, and the
resulting torn reads show up as screen flicker.

`vt.NewSafeEmulator` is the same type with an `RWMutex`. Always use it. Run
`make test-race` on any composer change — this class of bug is invisible
otherwise.

## F4 — Redraws must be damage-driven, not polled

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

The composer reports its outcome purely through the child's exit code: 0 submit,
2 keep as draft, 1 discard. No socket, no protocol, no temp-file handshake. git
has used the same channel to abort commits for decades.

The corollary is that **blur is not a separate mechanism**: the host sends
`\x1b:MarginDraft\r` and the child exits 2, which is byte-for-byte what
`esc esc` does. One persistence path, not two. Resist any change that gives blur
its own code path.

## F8 — Without nvim the test suite lies

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
