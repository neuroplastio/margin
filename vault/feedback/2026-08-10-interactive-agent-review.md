# Interactive review that involves the agent

Feature request, 2026-08-10. Requires architecting the skill and some
experimenting to land the final approach.

## The concept

1. Agent invokes the review process, launching margin in a new terminal
   instance (not part of margin itself, instrumented externally).
2. Human reviews and leaves comments.
3. Agent gets notified when a new comment lands, analyzes and responds using
   the margin CLI.
4. Human sees the agent's response inside the document without reloading.

## How

1. A margin CLI command that polls for new comments since the last entry:
   `margin comments wait [--since <last_known_id>]`.
2. Agent gets instructions to poll for the comments.
3. Only works in persistent mode (no `--stdout`).

## Context for the implementer

Today a thread file (`.margin/threads/...`) is a markdown file with a frontmatter
(`anchor:`, `document:`, optional `resolved:`) followed by one
`## author — timestamp` section per posted comment, in posting order. Comments
have **no stable id** — they are identified by author + timestamp. The `--since
<last_known_id>` flag therefore implies some comment identity that survives a
reload; whether that is the timestamp string or a new id field is a thread-file
format decision (the expensive-to-unwind class — raise a question rather than
guessing). A thread watcher already exists (watch.go) and reloads changed thread
files, so the "no reload" half of point 4 may already have a mechanism to hang
on.
