# Questions

**Agent → human.** Interaction decisions the agent must not make on its own.

**Only for decisions that are expensive to unwind.** The test is *if this turns
out wrong, is it a rename or a migration?* — on-disk formats, anchoring
semantics, anything the next few legs will build on top of.

Not for how something looks. A question about a visual treatment costs a round
trip and comes back "I do not know either, build one" — so build one, show it,
and expect to be corrected. That is cheaper for everybody, and it is what the
demo recipe in the journal entry is for.

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
