# UX, Rendering, and CLI Feature Feedback — 2026-08-09

*(Partially drained 2026-08-09: "Mark Visuals in Gutter" landed in journal
2026-08-09.17; "Focus Retained on Comment Exit" landed in journal 2026-08-09.23 —
exiting the composer leaves focus on the comment so pressing e immediately re-edits it.
What remains:)*

- **Vim Mode Background Rendering Artifacts:** Vim mode displays a strange black background beneath typed text; erasing text leaves behind persistent black background artifacts on deleted character cells.
- **Thread Comment Focus Highlight:** When focusing on a comment within a thread, highlight the comment body text itself rather than the author's name header.
- **CLI Commands for Agent Automation:** Provide dedicated CLI subcommands/flags so agents can programmatically extract comments and add/append their own comments without driving the interactive TUI.
