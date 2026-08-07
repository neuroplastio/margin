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

## F9 — The cloud run cannot push; it hands back a patch

The scheduled run has no write access to `neuroplastio/margin` — the git egress
proxy and the GitHub App integration both deny it. The org has not authorised
the app (the same org policy that blocked a fine-grained PAT for `gh api`).

**This is a known, accepted constraint, not something a run should debug.** The
routine prompt says so explicitly. A run commits locally, then produces
`git format-patch origin/main..HEAD` as an artifact and opens its report with a
line naming what did not land — `UNPUSHED: ID-02 — 1 commit, patch attached`.

The maintainer applies it:

```bash
git checkout main && git pull origin main
git am --3way 0001-<leg>.patch     # author, message and trailers preserved
make check && make test-race
git push origin main
```

`git am` is what keeps the leg's authorship and commit message intact — do not
reconstruct the change by hand from the diff.

Two things worth knowing when picking a patch up:

- **Verify, do not trust.** A patch that applies cleanly has still only been
  tested in the environment that produced it. Run `make check`, `make test-race`
  and `make doctor` before pushing.
- **The artifact page, not the API file.** A run's patch is reachable at
  `claude.ai/code/artifact/<uuid>`, which can be fetched. The
  `claude.ai/api/organizations/.../files/.../contents` URL for the same content
  returns 403.

To retire this: authorise the app for the org at
`github.com/organizations/neuroplastio/settings/installations`, then drop the
PUSH ACCESS block from the routine prompt.
