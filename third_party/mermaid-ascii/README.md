# mermaid-ascii — vendored copy with margin's deltas

This is a vendored snapshot of
[github.com/AlexanderGrooff/mermaid-ascii](https://github.com/AlexanderGrooff/mermaid-ascii)
(Go, MIT — see `LICENSE`), pinned at:

    v0.0.0-20260807155423-b1b35f67d6a5

It is **folded into the margin module proper**: this directory carries no
`go.mod` of its own, so `github.com/neuroplastio/margin/third_party/mermaid-ascii/pkg/...`
are packages of the host module — no `replace` directive is needed, and
`go install github.com/neuroplastio/margin/cmd/margin@latest` works from the
published module graph. (A nested module wired via `replace` broke that:
`go install module@version` refuses a go.mod whose replace would change how it
is interpreted as a dependency.) margin renders ` ```mermaid ` fences through
it; the in-tree caller is `internal/review/mermaid.go`.

The vendored copy is **not** a clean snapshot: it carries local deltas so that
margin can (a) import the graph renderer without the upstream CLI/web app, and
(b) keep the "never a half-parsed diagram" guarantee. Every change against
upstream is listed in `CHANGELOG.md` as a numbered delta, in a form that could
be upstreamed. When a new upstream version is adopted, the deltas should be
re-applied and re-verified against the new snapshot.

The vendored packages are self-validating as part of the host suite:

```bash
go test ./third_party/mermaid-ascii/...
```

What was vendored:

- `pkg/diagram` — config, frontmatter stripping, shared utils (upstream, unchanged).
- `pkg/sequence` — sequence diagram parser/renderer (upstream + delta D6).
- `pkg/er` — ER diagram parser/renderer (upstream + delta D2's `Keyword` export).
- `pkg/graph` — the flowchart/graph renderer, extracted from the upstream
  `cmd/` package (delta D2) and extended (deltas D3–D5).
- `cmd/testdata` — the upstream golden files the vendored packages' tests read.
- `LICENSE` — upstream's MIT license, untouched.

What was deliberately not vendored: the upstream `cmd/` CLI (cobra) and `web/`
(gin) entry points, `main.go`, `templates/`, `static/`, `docs/`, `scripts/`,
`.github/`, `.cursor/`, `.vscode/`. The graph package's own tests were not
extracted either — they exercised the whole `cmd` package, which would drag the
CLI back in. See delta D2.
