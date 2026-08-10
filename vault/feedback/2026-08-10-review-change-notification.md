# Visually notify the reviewer when the file changes or a new comment lands

Feature request, 2026-08-10.

Add a way to visually notify the reviewer that the document has changed on disk
or a new comment has been posted.

## Context: what happens today when the .md changes mid-review

The document itself is **not watched**. `newThreadWatcher` (watch.go) only
watches `.margin/threads/<docPath>`; the only live-reload path is
`reloadThreads` on a `threadsChangedMsg`. If the `.md` file is edited on disk
while margin is open, the reviewer keeps looking at the stale in-memory copy
with no signal at all — nothing re-reads the document until the next open.

Comments posted by an agent (via `margin comment add` or a direct thread-file
write) already reload live through the thread watcher, but the reload is
silent — the reviewer has no visual cue that a new comment landed.

## What is wanted

A visual notification in the interface for both cases:

1. **The document changed on disk** — and (implied) presumably a reload or a
   way to see the new content, since stale-forever is the current behaviour.
2. **A new comment was posted** (by an agent, while the reviewer is watching).

The exact treatment — a status line, a flash, a marker, a "file changed, press X
to reload" affordance — is the implementer's call, and needs a felt demo recipe
either way. The event log (`.margin/events.log`, D14/D15) can tell a new
`comment.add` / `comment.posted` event from a reload of existing threads, so the
"new comment" case does not have to mean "any thread write".
