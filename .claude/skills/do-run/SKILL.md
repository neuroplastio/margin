---
name: do-run
description: Orchestrate one run of margin — chain legs back to back in fresh subagents, felt ones included, until something genuinely needs the maintainer. Stops only on a raised question, an empty unblocked board, a broken tree, or the budget. Meant for the daily scheduled routine.
---

# Do one run of margin

You are the **orchestrator**. You do **no leg work yourself** — no reading the
vault, no editing, no implementing. You prime the checkout, spawn a subagent per
leg, read each report, and decide whether to continue. That keeps your context
small; each leg gets a fresh one.

## Why this exists

A run chains legs back to back so the maintainer wakes up to work rather than to
a queue. Felt legs included — building a visual change and showing it is worth
more than describing it and waiting, and the demo recipes accumulate on the
board's `Needs a look:` list for them to work through.

You stop for two things only: something nobody but the maintainer can decide, and
a tree that is not green. Everything else, keep going.

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

### 2. Loop

For each leg, spawn a **fresh subagent** whose entire instruction is:

> Run the `/do-leg` skill in this repository. Do exactly one leg and stop.
> Report: the task id, whether it was felt or mechanical, whether the tree is
> green, whether you raised a question, and the demo recipe verbatim if there is
> one.

When it returns, read the report and apply the stop rules below. If none fire,
spawn the next leg.

### 3. Stop rules — check every one after every leg

Stop the run and report when **any** of these is true:

1. **A question was raised.** The leg parked itself behind something expensive to
   unwind — a format, an anchor, something later legs build on. More legs would
   guess around it.
2. **Everything unblocked is exhausted.** The subagent reports nothing available.
3. **The tree is not green**, or a leg failed, or a subagent reports it could not
   finish. Do not send another leg after a broken one.
4. **Budget reached** — 8 legs or 100 minutes.

A felt leg does **not** stop the run. It adds a line to `Needs a look:` and the
next leg starts. Multiple unjudged visual changes in one run are expected and
fine — the maintainer would rather have four things to react to than one thing
and a wait.

### 4. Report

End with a summary the maintainer can act on without opening anything:

- Legs completed, with task ids and felt/mechanical
- **Which stop rule fired**
- **What to look at**, in priority order:
  - every demo recipe from this run, verbatim — these are the whole point
  - any question raised, and what it blocks
- The next suggested leg once they have responded

Be honest about what was not verified. You have no screen and no sense of
latency; if a leg changed something visual, say that it needs their eyes rather
than implying it works.

## Hard rules

- **Never do leg work yourself.** Always spawn a subagent.
- **Never spawn two legs in parallel.** They would race on the board and on
  `main`.
- **Never invent work.** If the board has nothing unblocked, that is a finished
  run, not a prompt to make something up.
