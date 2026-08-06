---
name: do-leg
description: Execute exactly one small leg of margin — orient from the vault, drain feedback, pick and claim the next task, implement it, keep the tree green, record what was learned, update the journal and board, then commit to main and stop. Use when moving margin forward, whether driven by hand, by /loop, or by a scheduled agent.
---

# Do one leg of margin

Do **exactly one leg**, then stop. Never two.

Read [`/CLAUDE.md`](../../../CLAUDE.md) and [`vault/README.md`](../../../vault/README.md)
before anything else. The rule that governs this whole skill:

> **You decide how it is built. You do not decide how it feels.**

## 1. Orient

```bash
git pull --rebase
```

Read, in this order: `vault/goals.md`, the active milestone in
`vault/roadmap.md`, `vault/tasks/board.md`, the last two or three entries in
`vault/journal/`. Then list `vault/feedback/` and `vault/questions/`.

If the leg will touch the composer or the emulator, read
`vault/knowledge/findings.md` **first**. Every entry there cost real debugging and
several are invisible to tests.

## 2. Pick, in this precedence

1. **Open questions gate the board.** Any file in `vault/questions/` with
   `Status: open` removes everything in its `Blocks:` line from what you may
   pick. If a question has been answered, folding the answer into
   `interaction.md` (or `decisions.md`) and deleting the file *is* a valid leg.
2. **Feedback preempts the board.** If `vault/feedback/` holds anything but its
   README, the oldest item is this leg. Address it and delete the file in the
   same commit.
3. **Check the gate.** If the board's `Awaiting review:` names a leg, you may only
   pick a `[mech]` task. Do not start `[felt]` work while it is set.
4. Otherwise take the next unblocked board item in milestone order. If it is too
   big, split it and take the first slice — splitting is a legitimate leg.

If everything is blocked, **stop and report which question or gate is holding
you**. Do not invent busywork.

## 3. Claim

Mark the task in-progress on the board with your id and the date, then commit and
push that immediately:

```bash
git commit -m "chore(board): claim <ID>"
git push
```

This acts as a lock for any concurrent agent.

## 4. Implement

Small — roughly ≤300 lines of diff. One logical change. A compiling stub with
tests beats a large half-wired change.

**If you hit an unsettled interaction decision**, stop implementing. Write
`vault/questions/Q-NNNN-slug.md` in the documented format, commit it, and report.
A half-built leg parked behind a real question is a good outcome. A guessed
keybinding is not.

Check `vault/knowledge/interaction.md` before assuming any behaviour. If it is
not in there, it is not settled.

## 5. Verify

```bash
make check
```

Composer or emulator changes additionally require:

```bash
make test-race
```

The emulator is written from the pty goroutine and read from the render
goroutine; that class of bug appears only under `-race`.

**What you cannot verify:** anything visual, and anything about how fast it
feels. You have no screen and no sense of latency. Never write that something
looks right or feels responsive — put it in the demo recipe instead.

## 6. Record

- Durable technical facts → `vault/knowledge/findings.md` (append only).
- Binding architectural choices → `vault/knowledge/decisions.md` as a new `Dn`.
- Interaction behaviour that the maintainer has **already approved** →
  `vault/knowledge/interaction.md`. Never add something there that has not been
  judged; that file is the record of what is settled, not a place to declare it.

## 7. Journal and board

Add `vault/journal/YYYY-MM-DD.N.md` in the documented format. Move the task to
Done on the board and update `Last updated:`.

**If the leg was `[felt]`:** set the board's `Awaiting review:` to this leg's id,
and end the journal entry with a `## Demo` section — the exact command, the keys
to press, and the specific questions to answer. Concrete questions only.

## 8. Commit and stop

```bash
git add -A
git commit   # conventional prefix; end with the Co-Authored-By trailer
git push
```

Then **stop**. Report:

- what you did, in a sentence or two
- whether it was felt or mechanical
- **what the maintainer should look at**, if anything — the demo recipe, verbatim
- any question you raised, and what it blocks
- the next suggested leg

Do not begin another leg.
