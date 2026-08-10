# mermaid-ascii — vendored copy with margin's deltas

This is a vendored snapshot of
[github.com/AlexanderGrooff/mermaid-ascii](https://github.com/AlexanderGrooff/mermaid-ascii)
(Go, MIT — see `LICENSE`), pinned at:

    v0.0.0-20260807155423-b1b35f67d6a5

It is **folded into the margin module proper**: this directory carries no
`go.mod` of its own, so `github.com/neuroplastio/margin/internal/mermaid-ascii/pkg/...`
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
go test ./internal/mermaid-ascii/...
```

## What's here

- `pkg/diagram` — config, frontmatter stripping, shared utils (upstream, unchanged).
- `pkg/sequence` — sequence diagram parser/renderer (upstream + delta D6).
- `pkg/er` — ER diagram parser/renderer (upstream + delta D2's `Keyword` export).
- `pkg/graph` — the flowchart/graph renderer, extracted from the upstream
  `cmd/` package (delta D2) and extended (deltas D3–D5).
- `pkg/state` — state diagrams, margin's in-tree extension (delta D7).
- `pkg/log` — a no-op stand-in for the upstream `logrus` dependency (delta D8).
- `testdata/` — the golden fixtures the `pkg/er` and `pkg/sequence` tests read
  (`er`, `er-ascii`, `sequence`, `sequence-ascii`).
- `LICENSE` — upstream's MIT license, untouched.

## Footprint

The fork is deliberately bare-bones. Upstream's `go.mod` dragged in the cobra
CLI and gin web server (delta D1); the remaining external requirements
`logrus` and `orderedmap/v2` were replaced with a no-op shim and a ~30-line
internal type (delta D8). The only external dependency left is
`github.com/mattn/go-runewidth`, for Unicode display-width measurement — the
fork's tests cover CJK and East-Asian-width rendering, so it is kept rather
than reimplemented. The graph package's own tests were not extracted (they
exercised the whole `cmd` package, which would drag the CLI back in), so the
unused golden fixtures for it (`ascii`, `extended-chars`, `multibyte`) were
dropped too. See delta D8.

## Extracting to a standalone repo

The tree mirrors upstream's `pkg/` layout on purpose, so a separate repo can
be carved out mechanically:

```
git clone github.com/neuroplastio/margin mermaid-ascii
cd mermaid-ascii
git filter-repo --subdirectory-filter internal/mermaid-ascii --force
# add a go.mod: module github.com/<you>/mermaid-ascii, require
# go-runewidth; go mod tidy; run go test ./...
```

Nothing outside `internal/mermaid-ascii/` imports across its boundary except
`internal/review/mermaid.go` (a thin dispatcher) — its import paths would be
the only edits needed. The CHANGELOG deltas and this README travel with the
tree, so provenance survives the split.

What was deliberately not vendored: the upstream `cmd/` CLI (cobra) and `web/`
(gin) entry points, `main.go`, `templates/`, `static/`, `docs/`, `scripts/`,
`.github/`, `.cursor/`, `.vscode/`. See delta D2.
