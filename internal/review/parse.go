package review

import (
	"bytes"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"regexp"
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
		if id, ok := markerID(n, src); ok {
			// An id marker carries no content of its own — it is metadata about
			// the block immediately before it. Attaching it here, rather than
			// leaving it in the block list, is what makes it invisible to
			// margin's own renderer as well as anyone else's.
			if len(blocks) > 0 {
				blocks[len(blocks)-1].anchor = id
				blocks[len(blocks)-1].stamped = true
			}
			continue
		}
		b, ok := blockFor(n, src)
		if !ok {
			continue
		}
		// A line number is a locator an agent can act on today, independent of
		// whether this block has been stamped yet.
		b.line = 1 + bytes.Count(src[:b.start], []byte{'\n'})
		blocks = append(blocks, b)
	}
	return blocks
}

// idMarkerRE matches the invisible comment stampID writes after a block to
// carry its id. Any markdown renderer treats it as a raw HTML comment, which
// is to say it renders as nothing.
var idMarkerRE = regexp.MustCompile(`^<!--\s*margin:(\^[0-9a-f]+)\s*-->$`)

// markerID reports whether n is an id marker, and the id it carries.
func markerID(n ast.Node, src []byte) (string, bool) {
	if _, ok := n.(*ast.HTMLBlock); !ok {
		return "", false
	}
	start, stop := extent(n, src)
	if start < 0 {
		return "", false
	}
	m := idMarkerRE.FindSubmatch(bytes.TrimSpace(src[start:stop]))
	if m == nil {
		return "", false
	}
	return string(m[1]), true
}

// newBlockID generates a fresh block id, independent of the block's content.
// Content-derived ids are what stamping replaces: they change the moment the
// text they anchor does, which is exactly the case a stamped id has to
// survive.
func newBlockID() string {
	var buf [4]byte
	// crypto/rand failing here would mean the platform has no source of
	// randomness at all; there is no sane fallback, so surface it loudly
	// rather than silently handing out colliding ids.
	if _, err := rand.Read(buf[:]); err != nil {
		panic("review: could not generate a block id: " + err.Error())
	}
	return "^" + hex.EncodeToString(buf[:])
}

// stampID inserts an id marker for block b immediately after its source
// range and returns the new source. Every block's [start,stop) range that
// starts before b.stop is unaffected — callers stamping earlier-in-document
// blocks first, in order, never invalidate a range they still need.
//
// b.stop is assumed to land right after the block's last real content byte
// (never on a trailing newline) — the convention extent() normalises to —
// so a blank line, the marker, and a newline can simply be inserted there.
func stampID(src []byte, b block, id string) []byte {
	var out bytes.Buffer
	out.Write(src[:b.stop])
	out.WriteString("\n\n<!--margin:")
	out.WriteString(id)
	out.WriteString("-->\n")
	out.Write(src[b.stop:])
	return out.Bytes()
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

// extent returns the block's byte range in the source, normalised so stop
// always lands right after the last real content byte (see
// trimTrailingNewline) and, for a fenced code block, widened to include the
// ``` delimiters that goldmark's Lines() excludes (see widenFence). Both
// exist for stampID: inserting a marker at an unnormalised or fence-exclusive
// boundary corrupts the block it is meant to be invisible to.
//
// Caveat still worth knowing: for a heading, the range excludes the leading
// `#`. That is fine for anchoring, rendering and stamping — the marker always
// lands after the block, never before — so it has been left alone.
func extent(n ast.Node, src []byte) (int, int) {
	lines := n.Lines()
	if lines != nil && lines.Len() > 0 {
		start, stop := lines.At(0).Start, lines.At(lines.Len()-1).Stop
		stop = trimTrailingNewline(src, start, stop)
		if _, ok := n.(*ast.FencedCodeBlock); ok {
			// A fenced code block's Lines() covers its content only — the ```
			// delimiters are syntax, not content, so goldmark excludes them.
			// stampID needs the true range: inserting a marker right after the
			// last content line would land it inside the fence, where it prints
			// as code instead of vanishing.
			start, stop = widenFence(src, start, stop)
		}
		return start, stop
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

// trimTrailingNewline drops a trailing '\n' from stop, if any. Whether
// goldmark's own Segment.Stop includes the newline that ends a block's last
// line is inconsistent across node types (a fenced code block's does; a
// paragraph's often does not) — normalising here means every caller, in
// particular stampID, can rely on one convention: stop always lands right
// after the last real content byte.
func trimTrailingNewline(src []byte, start, stop int) int {
	if stop > start && stop <= len(src) && src[stop-1] == '\n' {
		return stop - 1
	}
	return stop
}

// widenFence extends a fenced code block's [start,stop) to include its
// opening and closing ``` (or ~~~) delimiter lines.
func widenFence(src []byte, start, stop int) (int, int) {
	if start > 0 {
		open := start - 1 // the newline ending the fence line
		for open > 0 && src[open-1] != '\n' {
			open--
		}
		start = open
	}
	i := stop
	for i < len(src) && src[i] == '\n' {
		i++
	}
	if i > stop && i < len(src) {
		// An unterminated fence at EOF has no closing line to include.
		end := i
		for end < len(src) && src[end] != '\n' {
			end++
		}
		stop = end
	}
	return start, stop
}

// collapse folds a block's source lines into one line for re-wrapping.
func collapse(s string) string {
	return strings.Join(strings.Fields(s), " ")
}

// anchorFor derives a block's id from its content. It is the fallback for a
// block that has never acquired a thread and so has never been stamped
// (stampID / newBlockID) — content-derived ids are stable while the text is,
// which is enough to attach a thread within a session, but they cannot survive
// an agent *rewriting* the block, which is the whole point of anchoring. A
// stamped id (block.stamped) replaces this the moment a block gets its first
// comment; see ID-02 for re-attaching by a stamped id on reopen.
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
