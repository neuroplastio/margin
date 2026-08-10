# j/k on a very tall block should scroll incrementally

Feedback, 2026-08-10. On a block that is much taller than the viewport — a long
mermaid diagram, a big code block, a huge table — `j`/`k` jump focus between
blocks, so the viewport leaps by the whole block height. The reading flow is
lost: one `j` and you are a screen away, there is no way to page through the
block's content a few lines at a time.

When the focused block is taller than the viewport, `j`/`k` should scroll the
viewport down/up by a few lines within the block instead of hopping to the next
block; only once the viewport is at the block's bottom/top should the next `j`/`k`
move focus on. (There is already a `J`/`K` that scrolls the viewport 3 lines
without moving focus — the gap is that walking past a tall block with the plain
`j`/`k` is unusable.)

Context: `j`/`k` focus-walk blocks; tall blocks are single focus stops, so the
plain walk overshoots them.
