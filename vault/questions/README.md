# Questions

**Agent → human.** Interaction decisions the agent must not make on its own.

If a leg needs a behaviour that is not already settled in
[`../knowledge/interaction.md`](../knowledge/interaction.md), the agent does *not*
pick a sensible default. It writes a file here and stops.

That inversion is deliberate. A wrong interaction guess gets built on, and by the
time it is seen the cost of changing it has multiplied. An unanswered question
costs one round trip.

## Format

`Q-NNNN-short-slug.md`:

```markdown
# Q-0001 — Is the unit of review a file or a directory?

Status: open
Blocks: M3, NAV-01, NAV-02
Raised: 2026-08-06

## What I need decided

One or two sentences, stated as a choice, not an essay.

## Why I cannot decide it

What changes depending on the answer, and what it would cost to guess wrong.

## Options

**A. Single file.** …what it implies, what it costs.
**B. Directory tree.** …

## My lean

Which one and why — a recommendation, not a decision.
```

## Answering

Edit the file: set `Status: answered` and write the answer under an `## Answer`
heading. The agent folds it into `interaction.md` (or `decisions.md`) and deletes
the question file in the leg that acts on it.

An open question **blocks** everything named in its `Blocks:` line. If every
available leg is blocked, the agent stops and reports which question is holding
it — it does not invent unblocked busywork.
