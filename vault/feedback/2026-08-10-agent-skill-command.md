# margin --skill: AI agent skill output

Feature request, 2026-08-10. Add a new `margin --skill` command that outputs AI
agent skill information — the skill document an agent loads to learn how to use
margin. Part of the skill must instruct the agent how to do **interactive
reviews**: launch margin in a new terminal for the human, poll for their
comments, respond via the margin CLI, and let the human see the reply live in
the document.

The building blocks are all in place and should be referenced, not re-invented:

- `margin FILE.md` — launch the review in a terminal for the human.
- `margin comment add FILE.md --anchor <id> --text "..." --author agent` — the
  agent's side of the conversation.
- `margin comments wait --since <id> --timeout <dur>` — poll for new comments
  (event log, `.margin/events.log`, JSONL per D14).
- `margin export FILE.md` — read the review's current state (threads) on disk.
- The live-update half ("human sees the reply without reloading") works through
  the thread watcher; the event log is what the agent tails.

The exact output shape of the skill (flag vs subcommand, markdown vs plain
text, how much of the interactive loop to spell out) is for the implementer to
decide and record; the essential requirement is that an agent reading the
output can run the interactive review loop end to end.
