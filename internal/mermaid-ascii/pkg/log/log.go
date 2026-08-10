// Package log replaces the upstream renderers' logrus dependency (delta D8).
//
// Upstream mermaid-ascii logs renderer internals at debug level through
// sirupsen/logrus. margin never enables that output — the renderers are
// called from internal/review/mermaid.go with no logger wired — so carrying
// the dependency (and its go.mod requirement) was pure weight. The calls are
// kept so the fork's debug statements survive for whoever wants them, and all
// of them no-op here. If a caller ever needs renderer tracing, this is the
// single place to grow a real logger.
package log

// Debugf is a no-op stand-in for logrus's Debugf.
func Debugf(format string, args ...interface{}) {}

// Debug is a no-op stand-in for logrus's Debug.
func Debug(args ...interface{}) {}
