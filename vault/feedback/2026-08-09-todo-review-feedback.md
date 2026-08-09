# Feedback from maintainer review of TODO items — 2026-08-09

*(Partially drained 2026-08-09: the dive navigation and single-comment-focus
bullets landed in journal 2026-08-09.18 — block-level `j`/`k` walk blocks and
thread rows only (a thread is exactly one stop, whatever its length), `l`
dives into the focused thread's comments, `h` surfaces, and j/k inside a dive
flow back out at the ends. Diving into *multi-line blocks* was deferred: no
verb acts on a line today, so a per-line focus would be a stop with nothing
to do — it lands naturally with the line-reference work (L12-18 prepending)
in the visual-mode feedback file. The vim-mode key-combos bullet landed in
journal 2026-08-09.19 — the host now encodes every modified-key family itself
(ctrl+backspace and friends reach nvim as CSI-u; alt combos ride the verified
legacy meta form instead of collapsing to a bare ESC), and the composer binds
`<C-BS>`/`<M-BS>` to delete-word. One residual, recorded as F19: an
*unbound* alt combo still collapses to Escape, as in any terminal vim — only
a binding or the kitty keyboard protocol (unimplemented in x/vt) changes
that. What remains:)*

## 1. THREAD-04 — Delete Thread & Thread UX
- **Thread UX Improvements:**
  - **Dive into multi-line blocks:** the dive mechanism (`l`/`h`) currently enters threads only. Per-line focus inside a long block had no payload when the dive was built; revisit alongside the line-reference (L12-18) work.

## 2. EXPORT-04 & Persistence Location
- **.margin directory location:** `.margin` directory currently gets created inside `testdata/`, and export paths are relative to workdir. Shift `.margin` location to current working directory, with future detection of project root (e.g. `.git` or margin config file as anchor).
- **Locator format:** Approved (`## file.md:line (^anchor)` is good and actionable).

## 3. CMD-03 & CMD-05 — Command Palette UI & Staged Commands
- **Palette Position & Size:** Bottom position and 7-item limit are both approved.
- **Visual Separation:** Main document rendering is fine, but extra visual separation between palette and document wouldn't hurt.
- **Staged Commands UX (`m`/`s`):** Seeding via `m` and `s` keys is good enough for now.

## 4. SCROLL-02, SCROLL-03, SCROLL-04 — Scrolling & Mouse Interactions
- **Focus & Viewport Separation:** Separating focus and scrolling is approved ("perfect").
- **Incremental Scroll Keys:** Add incremental viewport scrolling using `J`/`K` (or similar) to allow scrolling the view without changing the focus.
- **Mouse Wheel Speed:** Mouse wheel speed (3 lines per tick) should be tunable.
- **Mouse Hover Visibility (`SCROLL-04`):** The dim hover indicator `▌` is not visible enough—feels like it's not working.
