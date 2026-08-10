module github.com/AlexanderGrooff/mermaid-ascii

go 1.21

// Delta D1: this go.mod is trimmed to the library packages vendored here
// (pkg/diagram, pkg/sequence, pkg/er, pkg/graph). The upstream go.mod also
// declared the cobra CLI and gin web server (gin, cobra, gookit/color,
// orderedmap, logrus, ...) and every transitive dependency of those; none of
// that is imported by the packages below, so it was dropped rather than
// vendored in. See CHANGELOG.md.

require (
	github.com/elliotchance/orderedmap/v2 v2.2.0
	github.com/mattn/go-runewidth v0.0.19
	github.com/sirupsen/logrus v1.9.0
)

require (
	github.com/clipperhouse/uax29/v2 v2.2.0 // indirect
	golang.org/x/sys v0.0.0-20220715151400-c0bba94af5f8 // indirect
)
