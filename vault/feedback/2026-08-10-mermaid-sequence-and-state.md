# Mermaid: support sequence and state diagrams

Feedback, 2026-08-10. The mermaid renderer (journal 2026-08-10.17) only renders
`flowchart`/`graph`. Need the other two common kinds:

1. **sequence diagrams** (`sequenceDiagram`) — participants, message arrows,
   activations, notes.
2. **state diagrams** (`stateDiagram-v2`) — states, transitions, `[*]` start
   and end.

Same constraints as the flowchart slice: in-tree ASCII renderer, never a
half-parsed diagram (unparseable falls back to plain source), on-disk format
untouched. A felt leg with a demo recipe. The flowchart slice set the visual
language (boxed tree, `◇` decisions, `├`/`└` junctions, `▼` arrowheads, dim
`↩` references); keep the new kinds consistent with it where their shapes
allow, and say what you rejected.
