# comments wait events carry no comment text

Feedback, 2026-08-10, from the maintainer's agent loop.

Every event is metadata only (id, anchor, author, type, comment index) — an
agent has to do a second read (the thread file or export) just to see what was
said. Worth considering embedding the text (or at least the new line) directly
in the event, since the round-trip is the same shape every time.

Note for the implementer: this touches the event-log line shape (`.margin/
events.log`, JSONL per D14) — the expensive-to-unwind class. A new field is
additive, but it is still a change to what a listener parses; record any shape
change as a decision superseding/extending D14, or raise a question if the
tradeoff (log size vs saved round-trip) feels like the maintainer's call rather
than the agent's.
