# Feedback from maintainer review of TODO items — 2026-08-09

## 1. THREAD-04 — Delete Thread & Thread UX
- **Thread UX Improvements:**
  - `j`/`k` navigation eats focus on threads. Need a dedicated "dive" navigation type for diving into threads and multi-line blocks.
  - When a thread has only one comment, focus shouldn't stop on it twice—it should only eat focus once.
  - Fix vim mode key combos: Vim mode doesn't receive `ctrl+backspace` or other key combos, dropping into normal mode unexpectedly.

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
