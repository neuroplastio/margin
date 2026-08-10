# Support mermaid diagrams

Feature request, 2026-08-10.

Support mermaid diagrams in the review. If the TUI is too limiting, consider
kitty-protocol-rendered image support or something in between.

## Context for the implementer

A mermaid diagram arrives as a fenced code block whose info string is `mermaid`
(a `blockCode` with `lang == "mermaid"`). Today every fenced block, mermaid
included, renders through chroma (`highlightCode`, RENDER-02), which has no
mermaid lexer of note — so a diagram currently shows as plain-ish source lines
with syntax colours that are at best meaningless.

There is no terminal-image machinery anywhere in the tree today: no kitty
graphics protocol (SGR/`_Gi` sequences), no iTerm/sixel path. The documented
findings file (`vault/knowledge/findings.md`) is about the composer's pty
emulator and should be read before anything touches the render pipeline.

## Possible shapes (implementer's call, record the choice)

1. **Render in the TUI** — an in-tree mermaid layout/ASCII renderer (e.g. a
   graph → box-drawing/character grid) for the common diagram kinds
   (flowchart, sequence). No external tool, always works, but a subset of the
   grammar and no fancy layout.
2. **Kitty graphics protocol** — render the diagram to an image off-screen
   (external `mmdc`/mermaid-cli, or an in-tree Go mermaid renderer emitting
   SVG→PNG) and display it as a terminal image, with a plain-source fallback
   when the terminal does not support the protocol. Depends on the terminal,
   external tooling, and the findings-file emulator's behaviour with
   out-of-band escape data.
3. **Something in between** — an ASCII renderer now, image protocol later.

This is a felt leg (or legs) with a demo recipe. The on-disk format is
untouched — the thread/export paths never need to know about mermaid. Which
shape to commit to is exactly the kind of judgement the maintainer flagged as
"if TUI is too limiting, consider … or something in between"; if the leg
cannot pick a defensible scope it may raise a question, but a buildable first
slice (e.g. flowchart + sequence ASCII) is preferred over parking.
