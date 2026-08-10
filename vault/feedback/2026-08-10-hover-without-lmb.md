# Hover only works while the left mouse button is held

Feedback, 2026-08-10. The gray gutter icon (the `▌` hover state, SCROLL-04 /
journal 2026-08-09.7) only appears when the pointer moves while the left mouse
button is pressed and held (i.e. dragging). It does not appear when the pointer
just moves over the document with no button held — which is what "hover"
usually means.

Expected: moving the mouse over a block with no button held should light the
hover gutter marker, the same as a drag does. This is a mouse-handling bug in
how motion events are decoded/routed (see `hitTest`, `m.hoveredEntry`, and the
`MouseMotionMsg` handling — SCROLL-04 wired `tea.MouseMotionMsg` into
`hitTest`). Note the composer findings file before touching the input path.
Felt: the fix is a visual/interaction change, so a demo recipe is needed.
