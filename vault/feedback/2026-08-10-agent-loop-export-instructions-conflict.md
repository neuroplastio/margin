# margin export's "Agent instructions" note disagrees with margin skill

Feedback, 2026-08-10, from the maintainer's agent loop.

`margin export`'s built-in "Agent instructions" note tells the agent to reply by
editing the thread markdown file directly. `margin skill` says the CLI
(`comment add`) is preferred because direct file edits don't write an event-log
line. Two different built-in sources of guidance, giving different advice — worth
reconciling, or at least having the export's note mention the tradeoff.
