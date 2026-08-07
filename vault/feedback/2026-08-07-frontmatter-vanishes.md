# Frontmatter vanishes instead of being rendered

Priority: normal
Kind: rendering
Follows: PARSE-03 (2026-08-07)

PARSE-03 fixed the parsing and then dropped the block on the floor. Opening a
document with frontmatter now shows nothing where it used to show a mangled
heading — which is better, but it is not what was asked for.

## What it does now

The parser is correct. Given:

```markdown
---
name: retry-policy
description: How outbound calls are retried
status: draft
tags: [reliability, networking]
---

# Retry policy
```

`parseDoc` produces exactly the right thing — a `blockFrontmatter` at line 1 with
its text intact, no setext-heading corruption, and it is neither `commentable()`
nor `markable()`, so it cannot take a comment or a review mark and does not
inflate review progress. All of that is right and none of it needs redoing.

But `render()` has no case for `blockFrontmatter`, so it emits nothing. The
rendered document begins at `Retry policy`. The metadata is parsed, held, and
then invisible.

## What I want

It rendered. Frontmatter is often the most useful part of an agent-generated
document — `status: draft`, a description, tags — and a reviewer opening the file
should see it without going back to the source.

The treatment is the part I want to look at rather than have decided, but to be
concrete about the shape: something visually distinct from prose, compact, and
clearly *about* the document rather than part of it. A dimmed key/value block at
the top is the obvious first attempt. A single collapsed line that expands is the
other. Either is worth trying; I would rather see one on screen than argue it in
the abstract.

## Process note, not a complaint

"Hidden entirely" was one of three options the original feedback listed, and it
said explicitly that the treatment was the felt part and needed judging on a
screen. PARSE-03 ran as a mechanical leg and picked one by omission — the
renderer simply has no branch for the new kind, so the choice was made by not
making it.

Worth noticing because it is the failure mode the felt/mechanical split exists to
prevent, and it did not look like a decision at the time. A useful check when a
mechanical leg introduces a new block kind, message, or state: *does something
now render differently, or not render at all?* If yes, the leg has a felt half,
and it should stop and show it rather than let the default stand.

Nothing to revert here — the parse work is good. This is the felt half arriving
late.
