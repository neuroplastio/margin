# Q-0003 — What is a comment's identity for `margin comments wait --since`?

Status: open
Blocks: interactive agent review — the `margin comments wait [--since <last_known_id>]`
feature (vault/feedback/2026-08-10-interactive-agent-review.md)
Raised: 2026-08-10

## What I need decided

For "give me the comments newer than the last one I saw", what does
`--since <last_known_id>` name — the existing `## author — timestamp` string, or
a new stable per-comment id field in the thread file?

## Why I cannot decide it

A thread file (`.margin/threads/<docPath>/<anchor>.md`, per D5/D9/D11) has no
stable comment id today: a comment is author + timestamp, in posting order. The
wait command makes an agent hand back "the last comment I saw" on every poll,
so whatever that names becomes part of what a thread file means. If the answer
is a new `id:` field, every comment section changes shape and files written
before the change need a migration story (and a hand-written file appended by
an agent — D5's whole point — needs to say whether the id is mandatory).
If the answer is the timestamp, nothing on disk changes, but identity inherits
the timestamp's edge cases: two comments sharing one timestamp (same
nanosecond, or a hand-written file) could be missed or re-reported across a
poll boundary. Either way, once agents start polling with this, the choice is
in files agents have already written — the expensive-to-unwind class.

## Options

**A. The timestamp is the identity.** `--since` takes the RFC3339Nano
timestamp a thread file already writes; new comments are those posted strictly
after it, and the agent's cursor is just "the timestamp of the last comment I
saw". No format change, no migration, agent-legible as always. Cost: identity
is only as good as timestamp uniqueness — the wait command must define the
tie edge (e.g. re-report, not skip, every comment whose timestamp equals the
boundary, so a shared timestamp can never be silently lost; the agent dedupes
by anchor+author on its side).

**B. A new stable `id:` field per comment.** Each comment section gains an id
(written by the TUI on post), and `--since` takes the id. Survives edited or
duplicate timestamps cleanly and reads unambiguously in agent logs. Cost: a
thread-file format change and a migration story for files written before it;
and what a hand-appended comment's id is (invent one? optional field with a
timestamp-derived fallback?).

## My lean

A (timestamp). The thread-file format is deliberately settled and
agent-legible; identity by timestamp is the smallest change and the tie edge is
defineable without touching the format. Choose B only if we expect timestamps
to be unreliable in practice — edited clocks, hand-written files — and note
that even B leaves the hand-appended-comment id story open, so it buys less
than it costs unless that expectation is real.
