package review

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"strings"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/text"
)

// parseDoc turns markdown source into the flat block list the review model works
// against. Only top-level nodes become blocks: nesting is a rendering concern,
// and a reviewer comments on "that paragraph", not on an inline span.
//
// Headings and paragraphs are understood. Everything else — lists, code fences,
// quotes, tables — is carried through as its own source lines and rendered
// verbatim. That is deliberate for now: each of those is a felt decision about
// how it should look, and guessing at six of them at once is exactly what the
// review gate exists to prevent.
func parseDoc(src []byte) []block {
	md := goldmark.New()
	root := md.Parser().Parse(text.NewReader(src))

	var blocks []block
	for n := root.FirstChild(); n != nil; n = n.NextSibling() {
		b, ok := blockFor(n, src)
		if !ok {
			continue
		}
		blocks = append(blocks, b)
	}
	return blocks
}

func blockFor(n ast.Node, src []byte) (block, bool) {
	start, stop := extent(n, src)
	if start < 0 {
		return block{}, false
	}
	raw := string(src[start:stop])

	switch t := n.(type) {
	case *ast.Heading:
		// A heading's text is short and never re-wrapped, so it is stored as
		// one line with its level for the renderer to style.
		txt := strings.TrimSpace(collapse(raw))
		if txt == "" {
			return block{}, false
		}
		b := block{kind: blockHeading, text: txt, level: t.Level, start: start, stop: stop}
		b.anchor = anchorFor(b)
		return b, true

	case *ast.Paragraph:
		// Paragraphs are re-wrapped to the reading measure, so their source
		// line breaks are collapsed away here.
		txt := strings.TrimSpace(collapse(raw))
		if txt == "" {
			return block{}, false
		}
		b := block{kind: blockPara, text: txt, start: start, stop: stop}
		b.anchor = anchorFor(b)
		return b, true

	case *ast.ThematicBreak:
		return block{}, false

	default:
		lines := strings.Split(strings.TrimRight(raw, "\n"), "\n")
		if len(lines) == 1 && strings.TrimSpace(lines[0]) == "" {
			return block{}, false
		}
		b := block{kind: blockRaw, text: raw, lines: lines, start: start, stop: stop}
		b.anchor = anchorFor(b)
		return b, true
	}
}

// extent returns the block's byte range in the source.
//
// Caveat worth knowing: goldmark's Lines() covers a node's *content*, so for a
// fenced code block the range excludes the ``` fences themselves, and for a
// heading it excludes the leading #. That is fine for anchoring and rendering,
// but ID-01 — which stamps ids back into the source — will need fence-inclusive
// extents and should widen this to line boundaries.
func extent(n ast.Node, src []byte) (int, int) {
	lines := n.Lines()
	if lines != nil && lines.Len() > 0 {
		return lines.At(0).Start, lines.At(lines.Len() - 1).Stop
	}
	// Container nodes (lists, quotes) carry no lines of their own; take the
	// span of their descendants.
	start, stop := -1, -1
	_ = ast.Walk(n, func(c ast.Node, entering bool) (ast.WalkStatus, error) {
		// Lines() panics on inline nodes, and only blocks carry source
		// segments anyway.
		if !entering || c.Type() != ast.TypeBlock {
			return ast.WalkContinue, nil
		}
		cl := c.Lines()
		if cl == nil || cl.Len() == 0 {
			return ast.WalkContinue, nil
		}
		s, e := cl.At(0).Start, cl.At(cl.Len()-1).Stop
		if start < 0 || s < start {
			start = s
		}
		if e > stop {
			stop = e
		}
		return ast.WalkContinue, nil
	})
	if start < 0 {
		return -1, -1
	}
	// Widen to whole lines so list markers and quote carets are not sliced off.
	for start > 0 && src[start-1] != '\n' {
		start--
	}
	for stop < len(src) && src[stop] != '\n' {
		stop++
	}
	return start, stop
}

// collapse folds a block's source lines into one line for re-wrapping.
func collapse(s string) string {
	return strings.Join(strings.Fields(s), " ")
}

// anchorFor derives a block's id from its content.
//
// This is an interim scheme. Content-derived ids are stable while the text is,
// which is enough to attach threads within a session — but the whole point of
// anchoring is surviving an agent *rewriting* the text, and a content hash by
// definition cannot. ID-01 replaces this by stamping a generated id into the
// markdown source, at which point rewording a block keeps its thread.
func anchorFor(b block) string {
	sum := sha256.Sum256([]byte(fmt.Sprintf("%d\x00%s", b.kind, b.text)))
	return "^" + hex.EncodeToString(sum[:3])
}

// loadDoc reads and parses a markdown file.
func loadDoc(path string) ([]block, error) {
	src, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	blocks := parseDoc(src)
	if len(blocks) == 0 {
		return nil, fmt.Errorf("%s has no reviewable content", path)
	}
	return blocks, nil
}
