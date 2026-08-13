# margin: how an agent takes part in a review

margin is a terminal tool for reviewing markdown — the long documents agents
produce: plans, specs, journals, decision logs. A human opens a document in
margin, reads the rendered prose, leaves comments anchored to blocks, and marks
what is reviewed and what still needs attention. You, the agent, are a
participant in that review: you read what the human says and you reply. This
document is the whole contract for taking part. Where it and `margin --help`
disagree, trust this — it is the loop that has been settled and tested.

## The five commands

| Command | What it is for |
| --- | --- |
| `margin FILE.md` | Open a document for review. An interactive terminal UI — the human's screen. You launch it; you never drive it. |
| `margin export FILE.md` | Print the review as it stands on disk: every block that is commented or flagged, with its anchor id, a quote, and the conversation. The non-interactive way to read the current state. |
| `margin comments wait [--since ID] [--timeout DUR]` | Block until a new event lands in the review's event log, then print its lines. The way you notice the human has spoken. |
| `margin comment add FILE.md --anchor ID --text "..." [--author agent]` | Append a reply to a block's thread. The way you speak. |
| `margin import-pr FILE.md` | Import the current branch's GitHub pull request comments into the review of FILE.md. The bridge from GitHub: gh detects the PR and fetches its review comments, and each comment naming this file lands as an ordinary margin thread on the block its line falls in. |

All five operate on the same `.margin/` directory the TUI reads and writes, so
they work in a pipe, a CI job, or a bare shell — no terminal, no running margin.

`import-pr` is the one command you are not expected to reach for in the loop:
it is how a review gets seeded from GitHub in the first place, usually before
you or the human start talking. Run it when the document under review is part
of a pull request and the PR's review comments should appear as threads —
gh must be installed, authenticated, and able to find the PR from the branch
the document's checkout is on. Re-running is safe: a comment already imported
(same author, same text) is skipped. Comments on other files, and comments
whose line matches no block, are reported rather than dropped, and the PR's
general conversation comments (which have no line) are left to gh.

## Reading the human's screen

You never drive the interface, but you may be asked to describe what is on it.
The gutter column beside each block says what state the block is in — the same
legend is in `margin --help`:

- `▌` — the cursor is on this block (pink), or it is part of the current
  selection (blue)
- `│` — reviewed (green) or flagged (orange)
- `·` — a section that is only partly reviewed
- `▸` — a comment thread lives on this block; `l`/`enter` opens it
- `✓` — the thread is resolved

## The interactive review loop

Reviewing with margin is a loop between you and the human. Run it end to end:

1. **Launch the review.** Open a new terminal and run `margin FILE.md`. The
   human reads and comments in that terminal. Do not watch it — the event log
   and the thread watcher are your connection to it.

2. **Read the starting state.** Run `margin export FILE.md`. Every item in the
   export names its block's anchor in its header — the `(^id)` in parentheses.
   That id is the block's address; you will need it to reply.

3. **Wait for the human.** Instead of re-exporting on a timer, block on the
   event log:

   `margin comments wait --since ID --timeout 60s`

   The id is the last event you have already seen — the cursor. Each call
   prints every event strictly after it, in order. Three outcomes, told apart
   by what lands on stdout and stderr:

   - **Exit 0 with lines on stdout** — new events. Each line is one JSONL event
     object:
     `{"id":"...","at":...,"type":"comment.posted","doc":"FILE.md","anchor":"^...","author":"...","comment":2,"text":"..."}`
     `anchor` names the block, `comment` is the index of the message within its
     thread (or `-1` for a thread-level event), `text` is the comment's body
     itself (absent on thread-level events), and `type` is what happened:
     `comment.posted`, `comment.updated`, `comment.deleted`, `comment.restored`,
     `thread.resolved`, `thread.unresolved`, `thread.deleted`, `thread.restored`.
     Any of them can be work.
   - **Exit 1 with nothing on stdout** — `--timeout` elapsed with nothing new.
     The normal "nothing yet" of a poll: pause briefly, then call again with the
     same `--since`. Pick a timeout to match how quickly a human answers; the
     default is to wait forever, so you usually want one.
   - **Exit 1 with a message on stderr** — a real error (a broken log, a
     `--since` id no event carries). Stop and look; this is not the "nothing
     yet" case.

   The very first call — before you have seen any event — takes no `--since`
   and returns the whole log, which gives you the initial cursor for the loop.

4. **Reply.** Answer each event with a comment of your own:

   `margin comment add FILE.md --anchor ID --text "..." --author agent`

   The id is from the event's `anchor` field (the leading `^` is optional). The
   reply is written to the thread file the TUI reads, so the human sees it
   **live**: the thread watcher reloads on disk changes, no reopen needed. The
   loop stays in step — the event log is append-only, so a poll that has already
   passed your reply's id never reports it again; and if you are unsure a reply
   landed, `margin export` shows the thread, which is the record.

5. **Loop.** Take the last line's `id` as the next `--since` and go back to
   step 3. Run `margin export` again whenever you want the whole picture rather
   than the delta. The loop has no end of its own — how to know when the human
   has finished is the next section.

Resolve what you fixed: a thread whose work is done carries `resolved: true` in
its thread file's frontmatter (below). The reviewer can set and unset it too, so
resolving is a suggestion, not a verdict.

## When the review is done

The loop above repeats forever: a "nothing yet" poll looks exactly the same
whether the human is thinking or has quit. When you are participating live —
replying as comments land — run the launch and the poll as two parallel jobs,
and treat the launch's completion as the done-signal.

Launch with `--stdout`, chained to a sentinel, as one backgrounded call:

    margin --stdout FILE.md && echo "__MARGIN_DONE__"

The human quits (`q`) when they are finished. margin prints the whole review to
stdout — the same content `Y` and `margin export` produce, your replies
included — the sentinel echoes, and the backgrounded call completes. That
completion is "the review is done", and the stdout it carried is the final
state, worth keeping as the record. The `&&` is deliberate: the sentinel fires
only on a normal quit, so a call that completes with no sentinel (and a non-zero
exit) is margin erroring, not the review finishing.

While the launch job runs, poll in parallel — step 3 above, looping, replying
via `margin comment add`:

    margin comments wait --since ID --timeout 60s

Two jobs, one review: the wait loop is your live connection to the human, the
launch job is your done-signal. When the launch job completes, stop polling —
there is nothing left to answer, and the review it printed is the whole story.

## The contracts behind the loop

- **Event log.** `.margin/events.log` in the review root's `.margin` directory:
  one JSONL line per event, append-only, never rewritten. This is the
  notification channel; thread files are the record. Ids are 13 characters and
  time-ordered (a later event sorts after an earlier one), and two events in the
  same second are ordered by file position, not id. A final unterminated line is
  an append still in flight; ignore it.
- **Threads.** Plain markdown under `.margin/threads/<docPath>/<anchor>.md`, one
  file per block, browsable and editable with ordinary file tools. The
  frontmatter carries the document and anchor, plus `resolved: true` once the
  thread is done; the body is the conversation — one `**author:** text` entry
  per comment, oldest first. Prefer the CLI for replies: `comment add` keeps the
  thread file and the event log in step for free. A direct file edit reaches the
  human just the same (the watcher reloads it), but writes no event-log line —
  the record is the thread file, as always.
- **Anchors.** Block ids. A comment stays attached to its block even when an
  agent rewrites the prose around it. An anchor that no longer exists is a real
  error: `comment add` fails loudly rather than writing a thread nothing will
  show, so a stale export is a signal to re-read the document, not a bug to
  work around.
