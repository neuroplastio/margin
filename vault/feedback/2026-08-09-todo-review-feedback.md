# Feedback from maintainer review of TODO items — 2026-08-09

*(Partially drained 2026-08-09: the dive navigation and single-comment-focus
bullets landed in journal 2026-08-09.18 — block-level `j`/`k` walk blocks and
thread rows only (a thread is exactly one stop, whatever its length), `l`
dives into the focused thread's comments, `h` surfaces, and j/k inside a dive
flow back out at the ends. Diving into *multi-line blocks* landed in journal
2026-08-09.28 — `l` on a table or raw block dives into its source lines, j/k
walk them, `h` surfaces, and `c`/`gy` anchor a comment to the line under focus,
the payload the L12-18 line-reference work (journal 2026-08-09.22) supplied.
The vim-mode key-combos bullet landed in
journal 2026-08-09.19 — the host now encodes every modified-key family itself
(ctrl+backspace and friends reach nvim as CSI-u; alt combos ride the verified
legacy meta form instead of collapsing to a bare ESC), and the composer binds
`<C-BS>`/`<M-BS>` to delete-word. The incremental scroll keys landed in journal
2026-08-09.20 — `J`/`K` scroll the viewport down/up 3 lines per press without
changing focus (`m.at`). The .margin directory location landed in journal
2026-08-09.21 — `resolveReviewRoot` locates the project root via `.git`/`.margin`
anchor so `.margin` lands in the project root with `docPath` relative to it.
Palette visual separation and mouse hover indicator visibility landed in journal
2026-08-09.24 — dim horizontal rule `─` above palette, hover indicator color brightened to 248.
What remains:)*

## 1. THREAD-04 — Delete Thread & Thread UX
- **Thread UX Improvements:**
  - **Dive into multi-line blocks:** *(drained 2026-08-09 — journal 2026-08-09.28: `l` on a table or raw block dives into its source lines, j/k walk them, `h` surfaces, and `c`/`gy` anchor a comment to the line under focus. The L12-18 line-reference work is what gave a line a payload.)*

## 2. EXPORT-04 & Persistence Location
- **Locator format:** Approved (`## file.md:line (^anchor)` is good and actionable).

## 3. CMD-03 & CMD-05 — Command Palette UI & Staged Commands
- **Palette Position & Size:** Bottom position and 7-item limit are both approved.
- **Staged Commands UX (`m`/`s`):** Seeding via `m` and `s` keys is good enough for now.

## 4. SCROLL-02, SCROLL-03, SCROLL-04 — Scrolling & Mouse Interactions
- **Focus & Viewport Separation:** Separating focus and scrolling is approved ("perfect").
- **Mouse Wheel Speed:** Mouse wheel speed (3 lines per tick) should be tunable.
