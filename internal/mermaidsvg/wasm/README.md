# margin-mermaid

The mermaid renderer behind `internal/mermaidsvg`: a WASI command (mermaid
source on stdin, one SVG on stdout, a message on stderr and a non-zero exit on
failure) that margin embeds gzipped as `../mermaid.wasm.gz` and runs with
wazero. Nothing here is needed to build margin, only to change the renderer.

## Rebuilding

```
make mermaid-wasm       # cargo build --release for wasm32-wasip1, then gzip -9 -n
make mermaid-metrics    # regenerate src/inter_metrics.rs from /usr/share/fonts/inter/Inter.ttc
```

Both run cargo through `mise x rust -- cargo` (override with `CARGO=...`).
`rust-toolchain.toml` pins Rust 1.98.1 and the `wasm32-wasip1` target. Commit
`../mermaid.wasm.gz` together with whatever changed here; `Cargo.lock` is
committed and the build uses `--locked`.

## What is in it

- **merman `=0.8.0-alpha.7`** (github.com/Latias94/merman, MIT OR Apache-2.0),
  a Rust port of Mermaid 12 used by Zed. Its third-party notices (Mermaid,
  dagre, DOMPurify, roughjs, d3-shape and others) are MIT, ISC, BSD-3-Clause
  or Apache-2.0. ELK (EPL-2.0) and the math fonts (OFL) are features this
  crate leaves off. The rest of the dependency graph is MIT/Apache-2.0 apart
  from Servo's CSS crates under lol_html (cssparser, cssparser-macros,
  selectors, dtoa-short: MPL-2.0, file-level copyleft, linked unmodified),
  foldhash (Zlib) and the Unicode-3.0 ICU data.
- **The `resvg-safe` pipeline.** usvg, which hotty-blitz paints with, cannot
  draw `<foreignObject>`, so every HTML label becomes plain `<text>`.
  Converting HTML labels gets slow fast and hits a work budget merman will
  not raise (around 90 flowchart nodes), so a diagram of more than 60
  statements, or one that hits the budget anyway, is drawn with SVG labels.
- **Inter's advance widths** (`src/inter_metrics.rs`, generated from Inter 4.1,
  SIL OFL 1.1): labels are measured the way Inter paints them, and the SVG
  names `Inter, IBM Plex Sans, system-ui, sans-serif`, so text fits its box
  wherever Inter is installed.
- **margin's palette.** Dark appearance, the xterm-256 colours of hotdoc.go's
  stylesheet, transparent root. The root `<svg>` gets `width`/`height` in px
  equal to its viewBox.

## Size

The shipped module is 7.3MB (2.5MB gzipped). Diagram kinds are Cargo features.
The build carries the eleven that markdown documents commonly use: flowchart,
sequence, state, ER, class, gantt, pie, mindmap, gitGraph, journey and
timeline. A kind left out is an error from `Render`, and margin falls back to
text.

| Build                                  | wasm   | gzipped | cold compile |
|----------------------------------------|--------|---------|--------------|
| flowchart, sequence, state, ER, class  | 6.6MB  | 2.3MB   | ~2.0s        |
| those plus the six above (shipped)     | 7.3MB  | 2.5MB   | ~2.5s        |
| all 37 kinds                           | 9.9MB  | —       | —            |
| all 37 kinds plus `layout-cytoscape`   | 10.3MB | 3.6MB   | ~3.7s        |

Where the shipped module's bytes go: merman's shared parsing, layout and SVG
code is about 1.4MB, static data 1.3MB, Rust's std, alloc and hash maps with
serde_json about 2MB, and the HTML, CSS and markdown parsers the label
pipeline needs about 0.4MB. Per kind, roughly: flowchart 650KB, state 320KB,
class 290KB, sequence 250KB, ER 160KB, gantt 140KB, mindmap 115KB, gitGraph
100KB, timeline 45KB, journey 35KB. Among the kinds left out: architecture
200KB plus 430KB for the cytoscape layout it needs, C4 and block 125KB each,
requirement 110KB, xychart 80KB, quadrant, sankey, kanban and radar 30–40KB
each. To add one, add its `diagram-*` feature in `Cargo.toml` and run
`make mermaid-wasm`.

Release profile: `opt-level = "s"`, LTO, one codegen unit, `panic = "abort"`,
stripped. On the all-kinds build, `"z"` saved 2MB and doubled render time,
and `3` added 3MB for no measurable gain.
