package review

import (
	"net/url"
	"path/filepath"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

// External links in the cells (the maintainer, 2026-11-10: "I expect them to
// be OSC8 - both to local files and web urls"): a link's text is an OSC 8
// hyperlink, which a terminal that has them makes clickable. A web link goes
// as it is; a local path goes as a file:// URL resolved from the document's
// directory (a leading "/" is the review root, as followDoc has it). A link
// to a heading in the same document (#fragment) is margin's own, followed
// with ctrl+], and has none. In-tree markdown links have one too: ctrl+]
// follows them in margin, and the terminal opens them as a file.

// linkBase is what resolves a link's target: the document's directory and
// the review root.
type linkBase struct{ dir, root string }

// linkBase is where this review's links resolve from.
func (m *model) linkBase() linkBase {
	lb := linkBase{dir: filepath.Dir(m.path)}
	if m.store != nil {
		lb.root = m.store.root
	}
	return lb
}

// url is the OSC 8 target of href, or "" for a link margin follows itself or
// one that goes nowhere safe.
func (lb linkBase) url(href string) string {
	if href == "" || strings.HasPrefix(href, "#") {
		return ""
	}
	if i := strings.Index(href, ":"); i > 0 && !strings.ContainsAny(href[:i], "/?#") {
		switch strings.ToLower(href[:i]) {
		case "http", "https", "mailto":
			return href
		}
		return ""
	}
	path, frag, _ := strings.Cut(href, "#")
	var abs string
	switch {
	case path == "":
		return ""
	case strings.HasPrefix(path, "/"):
		if lb.root == "" {
			return ""
		}
		abs = filepath.Join(lb.root, filepath.FromSlash(path[1:]))
	default:
		abs = filepath.Join(lb.dir, filepath.FromSlash(path))
	}
	if !filepath.IsAbs(abs) {
		a, err := filepath.Abs(abs)
		if err != nil {
			return ""
		}
		abs = a
	}
	u := url.URL{Scheme: "file", Path: filepath.ToSlash(abs), Fragment: frag}
	return u.String()
}

// renderFrag is a fragment of prose in its style (fragStyle), a link's text
// an OSC 8 hyperlink where it has a target.
func renderFrag(f inlineRun, body lipgloss.Style, lk []linkBase) string {
	text := fragStyle(f, body).Render(f.text)
	if f.link && len(lk) > 0 {
		if u := lk[0].url(f.href); u != "" {
			// One hyperlink around the styled word, not lipgloss's, which
			// wraps every character of an underlined run on its own.
			return ansi.SetHyperlink(u) + text + ansi.ResetHyperlink()
		}
	}
	return text
}
