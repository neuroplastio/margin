# UX, Rendering, and CLI Feature Feedback — 2026-08-09

- **Vim Mode Background Rendering Artifacts:** Vim mode displays a strange black background beneath typed text; erasing text leaves behind persistent black background artifacts on deleted character cells.
- **Thread Comment Focus Highlight:** When focusing on a comment within a thread, highlight the comment body text itself rather than the author's name header.
- **CLI Commands for Agent Automation:** Provide dedicated CLI subcommands/flags so agents can programmatically extract comments and add/append their own comments without driving the interactive TUI.
- **Mark Visuals in Gutter:** Per-line icons for marks look weird. Replace per-line mark icons with sleek vertical lines in the gutter instead.
- **Focus Retained on Comment Exit:** When exiting the comment editor/composer, keep focus on the comment so pressing `e` immediately re-edits it.
