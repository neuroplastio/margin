# No definitive "review is done" signal for a live-participating agent

Feedback, 2026-08-10, from the maintainer's agent loop.

No way for an agent to get a definitive "review is done" signal while also
replying live. The margin skill doc presents interactive-vs-export/--stdout as
the whole story, but doesn't describe how an agent taking part in a live review
(reply to comments via comment add as they land) is supposed to know when the
human is finished.

The pattern that actually works — launch with `--stdout`, chain a shell sentinel
after it, and treat the whole launch as one backgrounded call whose completion
is the done-signal, while running `comments wait` in a loop in parallel for live
replies — isn't documented anywhere. Worth adding as the canonical "agent
participates live + knows when to stop" recipe in `margin skill`.
