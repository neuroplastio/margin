# Feedback Inbox

**Human → agent.** Drop a markdown file here to steer margin: a UX complaint, a
bug you hit, a priority change, "that felt wrong", "ship this instead". The agent
checks this directory **before every leg** and drains it **before** touching the
board.

This is a to-do list, not an archive. An addressed item is **deleted** in the same
leg that addresses it; the permanent record lives in the journal entry, which
links back. If this directory holds only this README, the inbox is empty.

**Deleted is not gone.** Git keeps every item, and it is the only place your
*exact words* survive — a decision written from your feedback reads like the
agent's own idea once the file is gone. Before concluding "this needs the
maintainer", check whether you already said it:

```bash
git log --diff-filter=D --all -- 'vault/feedback/*'   # what was addressed, when
git show <commit> -- vault/feedback/<file>.md         # your words, verbatim
```

## How to submit

Add a file named `YYYY-MM-DD-short-slug.md`. Free-form prose is fine — nothing is
mandatory except that it says what you want. If it relates to a specific leg, name
the journal entry or board id.

Two things worth including when you have them, because the agent cannot get them
any other way:

- **What you saw**, ideally a screenshot or pasted terminal output.
- **What you pressed** to get there.

## Releasing the review gate

If the board's `Awaiting review:` names a felt leg you have now judged, say so
here — "REND-01 is fine, go on" is enough. That clears the gate and lets the next
felt leg start. You can also just clear the line on the board yourself.
