# A tour of margin

margin is a terminal tool for reviewing markdown. You read the **rendered**
document, leave comments anchored to *blocks*, mark what you have reviewed, and
hand the whole review back to whatever wrote the document. Press `j` and `k`
to move, `c` to comment, and `space` to mark a block. The [keys](#keys) are
listed below.

![A page under review: reviewed lines dimmed, a flagged paragraph, a thread](review.png)

## The loop

An agent writes a plan, you review it in margin, and the agent reads your
comments back, either from the export or by waiting on the event log.

```mermaid
flowchart LR
    agent([Agent]) -- writes --> doc[plan.md]
    doc --> margin{margin}
    margin -- "c: comment" --> threads[(.margin/threads)]
    margin -- "space: mark" --> marks[review marks]
    threads -- "comments wait" --> agent
    marks -- "Y: export" --> agent
```

![The review loop, going round](loop.gif)

## What a comment does

```mermaid
sequenceDiagram
    autonumber
    participant R as Reviewer
    participant M as margin
    participant L as events.log
    participant A as Agent
    R->>M: c on a paragraph
    M->>M: nvim opens in the margin
    R->>M: write, then esc
    M->>L: comment.posted
    L-->>A: margin comments wait returns
    A->>M: margin comment add --anchor ^id
    M-->>R: the reply shows in the thread
```

## A block's review state

```mermaid
stateDiagram-v2
    direction LR
    [*] --> unmarked
    unmarked --> reviewed: space or r
    reviewed --> flagged: space or f
    flagged --> unmarked: space
    reviewed --> unmarked: the block changed
```

## Keys

| Key | Does |
| --- | --- |
| `j` / `k` | Move between blocks |
| `c` | Comment on the focused block |
| `space` | Cycle the mark: unmarked, reviewed, flagged |
| `/` | Search |
| `\` | Raw source |
| `i` | Every thread under review |
| `Y` | Copy the review |

## Try it

- Mark this list item reviewed with `r`.
- Flag this one with `f`.
  - Nested items are blocks too.
- [x] Open the demo
- [ ] Leave a comment on the diagram above

> A quote is a block like any other: comment on it, or mark it. Comments survive
> the block being reworded, because they anchor to an id, not a line.

The export is what the agent reads:

```go
// margin --stdout plan.md | agent -p "address this review"
type Review struct {
	Doc     string
	Threads []Thread
}
```
