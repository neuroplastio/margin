# UX, Rendering, and CLI Feature Feedback — 2026-08-09

*(Partially drained 2026-08-09: "Mark Visuals in Gutter" landed in journal
2026-08-09.17; "Focus Retained on Comment Exit" landed in journal 2026-08-09.23;
"Thread Comment Focus Highlight" landed in journal 2026-08-09.25 — focusing a comment
places focus bar and highlight on comment body text lines rather than author header.
The vim-mode background artifacts bullet landed in journal 2026-08-09.30 — the
composer init now forces `Normal`/`NormalFloat`/`EndOfBuffer` backgrounds to `NONE`
so nvim no longer paints its own dark slab over the pane (F21). The CLI commands
for agent automation bullet landed in journal 2026-08-09.31 — a new `margin
comment add` subcommand appends a comment to a thread without the interface,
writing the same thread file the TUI writes. What remains:)*

- **CLI Commands for Agent Automation — the extract half:** `margin comment
  add` covers writing; extraction of comments is already served by `--stdout`
  and by the thread files being plain markdown, but a dedicated non-interactive
  `margin export`-style command has not been added.

