# The mermaid renderer

`internal/mermaidsvg` embeds [Ondine](https://github.com/wevm/ondine) (MIT,
`LICENSE` here), a Mermaid-to-SVG compiler in Rust with no dependencies,
built for `wasm32-unknown-unknown` and committed gzipped as
`../mermaid.wasm.gz`. margin carries no Rust of its own: the module is
Ondine's `wasm` crate exactly as it is at the pinned commit. Nothing here is
needed to build margin, only to change the renderer.

## Rebuilding

```
make mermaid-wasm       # fetch ONDINE_REV into ./ondine, cargo build, gzip -9 -n
```

`ONDINE_REV` in the Makefile is the commit. The build runs cargo through
`mise x rust -- cargo` (override with `CARGO=...`, e.g. `CARGO=cargo` with
rustup), in Ondine's checkout, so its `rust-toolchain.toml` (Rust 1.97.0, the
`wasm32-unknown-unknown` target) and `Cargo.lock` apply. Commit
`../mermaid.wasm.gz` with the Makefile change.

## What is in it

- **Every kind Ondine draws** (`--features all`): flowchart, sequence, state,
  class, ER, gantt, pie, mindmap, gitGraph, journey, timeline, quadrant,
  xychart, sankey, block, architecture, C4, kanban, packet, radar,
  requirement, treemap, treeView, venn, wardley, railroad, ishikawa,
  cynefin, eventmodeling, swimlane, usecase, agentflow and info.
- **Ondine's `wasm` profile**: `opt-level = "z"`, LTO, one codegen unit,
  `panic = "abort"`, stripped. It is the smaller of Ondine's two builds; its
  `release` profile renders no faster that margin could notice.

## The interface

The module imports nothing. Its exports are `alloc(len) -> ptr`, which
reserves guest memory for the host to write into, and
`render(src, src_len, options, options_len, now) -> ptr`, whose result is a
status byte (0 for an SVG, 1 for an error) and then fields, each a
little-endian `u32` length and that many bytes:

- success: kind, width (f64), height (f64), a link count (u32) and each
  link's href, id and internal flag, then the SVG;
- failure: code, kind, line (u32), column (u32), snippet, message.

`options` is JSON with Ondine's render options; margin sets `idPrefix`, and
turns `responsive`, `links` and `images` off. `now` (f64 milliseconds) is a
gantt chart's today. margin runs one fresh instance per diagram, so the
module's `reset`, which empties its arena for the next render, goes unused.

## Size and speed

| Build                       | wasm  | gzipped | cold compile (wazero) | a diagram |
|-----------------------------|-------|---------|-----------------------|-----------|
| `wasm` profile (shipped)    | 2.0MB | 0.7MB   | ~0.9s                 | 1–2ms     |
| `release` profile           | 4.0MB | 1.3MB   | ~2.8s                 | 1–2ms     |
| merman, before (11 kinds)   | 7.3MB | 2.5MB   | ~2.5s                 | 15–260ms  |

A diagram's time is a fresh instance and the render, measured on margin's
`testdata/mermaid.md`.

## What margin does to its SVG

Ondine themes a diagram with CSS custom properties (`--ondine-bg`,
`--ondine-fg`, `--ondine-line`, the eight `--ondine-series-N`, …) and
`color-mix()`. usvg, which hotty-blitz paints an `<img>` with, has neither,
and draws such a diagram as black boxes with no text. `../flatten.go` puts
margin's palette in for the variables, computes each `color-mix()`, and
drops the root's custom-property declarations; Ondine's own terminal output
does the same for resvg.

Ondine measures every label as monospace with a 0.6em advance, so the SVG
names Ondine's monospace stack (`ui-monospace, SFMono-Regular, Menlo,
Consolas, 'Liberation Mono', 'DejaVu Sans Mono', monospace`): a
proportional font would not fit the boxes.
