---
name: do-run
description: Orchestrate one run of margin — chain mechanical legs back to back in fresh subagents until something needs the maintainer, then stop. Stops on the first felt leg, the first raised question, an empty unblocked board, or the budget. Meant for the daily scheduled routine.
---

# Do one run of margin

You are the **orchestrator**. You do **no leg work yourself** — no reading the
vault, no editing, no implementing. You prime the checkout, spawn a subagent per
leg, read each report, and decide whether to continue. That keeps your context
small; each leg gets a fresh one.

## Why this exists, and what it must not do

Mechanical work — parsers, persistence, export, refactors — has no reason to wait
on the maintainer. Tests settle it. So a run chains those back to back.

**Felt work is different.** It cannot be judged by a test, and building a second
felt change on top of an unjudged first one is how a project ends up with an
interaction model nobody chose. So the run stops the moment it produces something
only a human can evaluate.

You are the thing standing between "productive overnight run" and "eight
unreviewed UI decisions stacked on each other". Stop early rather than late.

## Budget

- **8 legs**, or **100 minutes**, whichever comes first.
- These are ceilings, not targets. Most runs should stop well before either.

## Procedure

### 1. Prime the checkout

```bash
git checkout main && git pull --rebase origin main
git config user.name "Anatoly Rugalev"
git config user.email "anatoly.rugalev@gmail.com"
```

Then confirm the environment can actually verify the composer:

```bash
make doctor
```

If `make doctor` fails, **stop the run immediately** and report it. Roughly two
thirds of the tests skip without a usable nvim, so a run in a broken environment
produces green commits that prove nothing. Do not proceed and do not "work around
it".

### 2. Check the gate before spawning anything

Read `vault/tasks/board.md`. If `Awaiting review:` already names a leg, the
maintainer has not judged it yet. You may still run, but **only mechanical legs**
— pass that constraint to every subagent.

### 3. Loop

For each leg, spawn a **fresh subagent** whose entire instruction is:

> Run the `/do-leg` skill in this repository. Do exactly one leg and stop.
> Report: the task id, whether it was felt or mechanical, whether the tree is
> green, whether you raised a question, and the demo recipe verbatim if there is
> one.
> *(Add when the gate is set:)* The board's `Awaiting review:` is set, so pick a
> `[mech]` task only.

When it returns, read the report and apply the stop rules below. If none fire,
spawn the next leg.

### 4. Stop rules — check every one after every leg

Stop the run and report when **any** of these is true:

1. **The leg was felt.** It set `Awaiting review:`. The run is over — do not spawn
   another leg, mechanical or otherwise. One unjudged felt change at a time is the
   whole point.
2. **A question was raised.** The leg parked itself behind a decision only the
   maintainer can make. Stop; more legs would guess around it.
3. **Everything unblocked is exhausted.** The subagent reports nothing available.
4. **The tree is not green**, or a leg failed, or a subagent reports it could not
   finish. Do not send another leg after a broken one.
5. **Budget reached** — 8 legs or 100 minutes.

Rule 1 is the one that will fire most often, and that is correct. A run that
completes two mechanical legs and then stops on a felt one has done its job.

### 5. Report

End with a summary the maintainer can act on without opening anything:

- Legs completed, with task ids and felt/mechanical
- **Which stop rule fired**
- **What needs their attention**, in priority order:
  - the demo recipe verbatim, if a felt leg is awaiting review
  - any question raised, and what it blocks
- The next suggested leg once they have responded

Be honest about what was not verified. You have no screen and no sense of
latency; if a leg changed something visual, say that it needs their eyes rather
than implying it works.

## Hard rules

- **Never do leg work yourself.** Always spawn a subagent.
- **Never spawn two legs in parallel.** They would race on the board and on
  `main`.
- **Never override the review gate.** If you find yourself reasoning about why
  this particular felt leg is safe to follow with another, stop — that is the
  failure this skill exists to prevent.
- **Never invent work.** If the board has nothing unblocked, that is a finished
  run, not a prompt to make something up.
