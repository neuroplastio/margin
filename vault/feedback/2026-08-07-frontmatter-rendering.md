# Frontmatter should be rendered better

Priority: normal
Kind: rendering

YAML frontmatter renders badly, and it is not just cosmetic — it corrupts the
block model and leaks into the export.

## What actually happens

Given an ordinary document:

```markdown
---
name: retry-policy
description: How outbound calls are retried
status: draft
tags: [reliability, networking]
---

# Retry policy

Each outbound call is retried up to three times.
```

`parseDoc` produces:

```
[0] kind=heading level=2 line=2
    text="name: retry-policy description: How outbound calls are retried status: draft tags: [reliability, networking]"
[1] kind=heading level=1 line=8  text="Retry policy"
[2] kind=para          line=10   text="Each outbound call is retried up to three times."
```

The mechanism: goldmark is running without a frontmatter extension, so the
opening `---` is a thematic break (dropped), and the **closing** `---` turns the
YAML above it into a *setext heading*. The whole block collapses onto one line
and renders bold, in heading colour, as the most prominent thing on the page.

## Why it is worse than it looks

- **It is the first heading**, so it becomes the enclosing section for every
  block until the real `# Retry policy`. The export then says
  `Section: name: retry-policy description: How outbound calls are…` — which is
  the one field an agent uses to locate a comment.
- **It is commentable and markable.** It gets an anchor, takes a review mark,
  and counts toward `n of m blocks reviewed`. Metadata is not prose; reviewing
  it is meaningless, and it inflates the denominator.
- **`---` inside the document** has the same effect wherever it appears after a
  paragraph, so this is not only a frontmatter bug — any horizontal rule written
  that way silently turns the paragraph above it into a heading. `***` and `___`
  do not.

## What I want

Frontmatter recognised as frontmatter, and rendered as something a reader can
skim past — it is metadata about the document, not part of it.

I am not prescribing the treatment; that is the felt part and it needs judging on
a screen. Worth deciding together:

- Hidden entirely, shown only on demand?
- A compact dimmed key/value block at the top, visually distinct from prose?
- One collapsed line that expands, the way a thread does?

Two things I do think are settled regardless of the visual choice: it should not
be **commentable**, and it should not count toward **review progress**.

## Notes

The parser fix is mechanical and separable from the rendering decision — teach
`parseDoc` to recognise a leading `---…---` block and give it its own kind, with
tests covering the setext trap. That could land first and stop the export
corruption immediately, leaving how it looks for a felt leg afterwards.

goldmark has a frontmatter extension (`yuin/goldmark-meta`), which may be less
work than hand-rolling it, but check what it does to byte offsets before
adopting — ID-01's stamping depends on those being accurate.
