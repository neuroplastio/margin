# Q-0002 — How are resolution and deletion represented?

Status: open
Blocks: THREAD-01, THREAD-03, EXPORT-05
Raised: 2026-08-07

## What I need decided

Two shapes, both of which STORE-01 will bake into the thread file format:

1. Is **resolved** a flag on the thread, or an event with an author and a time?
2. Does **deleting** a comment remove it, or leave a tombstone?

## Why I cannot decide it

STORE-01 is next up and mechanical, so an agent will design the thread file
format without asking. Whatever it picks becomes the on-disk shape that both
margin and any agent read and write. Changing it later means migrating files
that a coding agent may already be writing — the expensive kind of retrofit.

Both questions also carry a rule, not just a shape: **can an agent resolve a
thread it was asked to fix?** That is the difference between the loop closing on
its own and every round re-litigating the last one.

## Options

### Resolution

**A. A boolean on the thread.** `status: resolved`. Simplest, and enough to
filter the export.

**B. An event with author and time.** `resolved-by: agent` /
`resolved-at: …`. Costs a little more, but "the agent claims it fixed this" and
"I accepted it" are different facts, and only the second should really close a
thread.

**C. Both, layered.** The agent may mark `addressed`; only the reviewer marks
`resolved`. Two states, one loop: the agent says it is done, you agree or you do
not.

### Deletion

**D. Remove it.** The comment is gone from the file. Git keeps the history, and
the thread file stays readable.

**E. Tombstone.** `deleted: true`, body dropped. Survives an agent that already
replied to it, so the reply does not dangle.

## My lean

**C** and **D**.

C because it is the only option where the loop actually closes: an agent can say
"addressed", the export stops nagging about it, but the thread does not vanish
until you have looked. That matches how you have been using flags — a marker for
"come back to this" is only useful if something else is claiming to have done
the work.

D because tombstones make the thread file worse for the agent to read, and git
already keeps the words. The dangling-reply case is real but rare, and reads
fine as a reply with nothing above it.

Worth knowing before answering: goals.md principle 3 says never lose the user's
words, so whatever deletion is, it should be explicit and confirmed — that part
is not in question.
