# Goals

## What margin is

A terminal tool for reviewing markdown — specifically the long documents agents
produce: plans, specs, journals, analyses, decision logs.

You read the rendered prose, leave comments anchored to blocks, mark what you
have reviewed and what still needs attention, and hand the result back to
whatever wrote it.

## The problem

Reviewing a long markdown file today means one of two bad options:

- **An editor.** No rendering, so you read wall-to-wall source and lose your
  place. To point at a passage you quote it by hand or memorise a line number.
- **A pull request.** Comments work, but the review surface is a diff, so there
  is still no rendered prose to read.

Neither closes a loop. The feedback ends up retyped into a chat window.

## Definition of done (v1)

A reviewer can:

1. Open a markdown file or directory and read it comfortably — real measure,
   real block spacing, real heading hierarchy.
2. Move through it entirely from the keyboard, vim-style.
3. Leave a comment on any block, in a real nvim, and have it survive losing
   focus.
4. Reply to and edit existing comments, including an agent's.
5. Mark blocks and whole sections reviewed or flagged, and see progress.
6. Export the whole review in a form an agent acts on directly.
7. Re-open the same document later and find their threads still attached, even
   though the agent rewrote the prose.

## Non-goals

- **Not a markdown editor.** margin reviews documents; nvim edits them.
- **Not a code review tool.** That is a solved and crowded space.
- **Not a GitHub client** in v1. Local loop first. Platform sync may come later,
  and it must not shape the local design.
- **Not a collaboration server.** One reviewer and their agents. No accounts, no
  presence, no realtime.
- **Not mouse-driven.** The mouse is a convenience for focusing. Everything must
  be reachable and fast from the keyboard, and the keyboard path is the one that
  gets designed first.

## Principles

1. **Reading comes first.** If the document is not pleasant to read, nothing else
   matters. Every feature competes for rows against the prose and usually loses.
2. **Delegate editing to nvim.** Never build a text editor. The composer is a real
   child process with the user's own muscle memory.
3. **Never lose the user's words.** Losing focus, quitting, crashing — the default
   outcome is always that the text is kept. Discarding is explicit.
4. **Anchor to meaning, not position.** Blocks carry stable ids because an agent
   will rewrite the prose around them.
5. **The agent is a participant, not an export target.** It should read and write
   threads with ordinary file tools — no CLI, no schema, no daemon.
6. **The keyboard path is the real path.** See non-goals.
7. **Prefer fewer, better keys.** Every binding is a thing to remember. Reuse vim
   meanings rather than inventing new ones.
