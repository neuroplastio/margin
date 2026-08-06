# Journal

One file per leg: `YYYY-MM-DD.N.md`, `N` starting at 1 each day. This is the
changelog and the record of why things are the way they are.

## Format

```markdown
# 2026-08-06.1 — PARSE-01: parse markdown into blocks

Kind: mech            # or: felt
Task: PARSE-01
Commit: <sha>

## What changed

A short paragraph. What now works that did not before.

## Notes

Anything a future leg needs to know that does not belong in decisions.md or
findings.md. Dead ends count — knowing what was tried and failed is worth
recording.
```

## Felt legs need a demo recipe

A felt leg's entry must end with a section the maintainer can act on without
reading any code:

```markdown
## Demo

    make run
    # press j until the focus reaches the code block under "Retry policy"

Judge:
- Does the code block's background read as a block, or does it blur into the prose?
- Is the highlighting distracting at this size?
- Should the fence language be shown, and if so where?
```

Be specific. "Does it look OK?" is not a question anyone can answer usefully.
Name the keys to press and the exact thing to look at.

**Never claim a visual check you did not perform.** You cannot see the screen and
you cannot feel latency. If something needs a human eye, the demo recipe is where
it goes — not an assertion that it works.
