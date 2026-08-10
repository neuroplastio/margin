# `go install` fails: go.mod has a replace directive

Bug, 2026-08-10. `go install github.com/neuroplastio/margin/cmd/margin@fb71eb1`
fails:

```
The go.mod file for the module providing named packages contains one or more
replace directives. It must not contain directives that would cause it to be
interpreted differently than if it were the main module.
```

## Cause

Leg 18 (journal 2026-08-10.19) vendored `mermaid-ascii` into
`third_party/mermaid-ascii` and wired it with a `replace` directive in
`go.mod`. A module with `replace` directives cannot be consumed via
`go install module@version` — it only builds as the main module. This breaks
the README's install story and any `go install`-based consumption.

## What is wanted

`go install github.com/neuroplastio/margin/cmd/margin@<commit>` (and `@latest`)
must work again. The vendored-mermaid approach and its upstreamable changelog
(per 2026-08-10-mermaid-sequence-and-state.md) should be preserved as far as
possible — the fix is about *how* the vendored code is exposed to the module
graph, not about dropping it. Options to evaluate: move the vendored code into
the module proper (e.g. an internal package) so no replace is needed; push the
vendored copy to a real module path (a fork or a versioned tag) and depend on
it normally with no replace; or a `vendor/` directory. Pick the one that keeps
`go install @version` working and the changelog upstreamable, and record the
choice. Mechanical leg.
