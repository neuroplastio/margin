# Block quotes should be a vertical rule, not a `>` on every line

Priority: normal
Kind: rendering (felt)
Board: RENDER-04

A block quote currently renders its source verbatim, so every line starts with
`>`:

```
> **Open question.** Do we need session data encrypted at rest in Redis? The
> tokens are opaque and the payload is a user id plus a permission set, but
> "permission set" is arguably sensitive.
```

That is markdown source, not a rendered quote. It should be a **vertical rule
down the left edge** with the text padded off it — the shape a quote has
everywhere else — so it reads as a set-apart passage rather than as prose with a
punctuation mark stuck to it.

## Notes

- The text inside a quote is prose and should wrap to the measure, minus
  whatever the rule and its padding cost. Same underlying issue as the list
  truncation filed alongside this: `blockRaw` renders verbatim, which is right
  for code and wrong for anything made of sentences.
- Nested quotes exist but are rare in agent output; a second rule is the obvious
  answer if it comes up, and not worth designing for now.
- A quote can contain a list or a fence. Worth knowing what that does before
  committing to a treatment, even if the answer is "not yet".
