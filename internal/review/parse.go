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
	"github.com/yuin/goldmark/extension"
	eastast "github.com/yuin/goldmark/extension/ast"
	"github.com/yuin/goldmark/text"
)

// parseDoc turns markdown source into the flat block list the review model works
// against. Only top-level nodes become blocks: nesting is a rendering concern,
// and a reviewer comments on "that paragraph", not on an inline span.
//
// Headings, paragraphs, lists, block quotes, code fences and tables are all
// understood. Everything else is carried through as its own source lines and
// rendered verbatim, kind blockRaw — a felt decision about how it should look
// that has not been made yet, and guessing at several of them at once is
// exactly what the review gate exists to prevent.
//
// GFM extensions are enabled (tables, strikethrough, autolinks, task lists):
// without them a table is invisible to goldmark as a table at all — it parses
// as an *ast.Paragraph, and collapse() then does to it exactly what a
// paragraph's line breaks are supposed to get, joining rows into one wrapped
// mess (2026-08-08 rendering-bugs feedback). With the extension it is an
// *ast.Table (github.com/yuin/goldmark/extension/ast, aliased eastast below —
// not to be confused with the core ast package), which RENDER-03 gives its
// own blockTable kind and a real column layout. The other three extensions
// are inline-only — they change what a paragraph's text contains, never what
// kind of block it is — so enabling them alongside costs nothing here and
// heads off the same defect turning up again for a task list.
//
// A leading YAML frontmatter block is recognised and pulled out before goldmark
// ever sees it as prose (see frontmatterExtent) — left to goldmark it misparses
// as a thematic break plus a setext heading, which corrupts both the section
// model and the export.
func parseDoc(src []byte) []block {
	md := goldmark.New(goldmark.WithExtensions(extension.GFM))
	root := md.Parser().Parse(text.NewReader(src))

	var blocks []block
	fmStart, fmStop, hasFM := frontmatterExtent(src)
	if hasFM {
		b := block{kind: blockFrontmatter, text: string(src[fmStart:fmStop]), start: fmStart, stop: fmStop, line: 1}
		b.anchor = anchorFor(b)
		blocks = append(blocks, b)
	}
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
		if hasFM {
			if start, stop := extent(n, src); start >= fmStart && stop <= fmStop {
				// Whatever goldmark made of this range — the dropped thematic
				// break, the setext heading the closing "---" produces — is
				// already covered by the frontmatter block above.
				continue
			}
		}
		if list, ok := n.(*ast.List); ok {
			// A list is not one block but several — see blockListItem's
			// doc comment for why. blockFor stays one-node-in, one-block-out,
			// so this is handled here instead of inside it, the same way
			// markerID and the frontmatter check already are.
			blocks = append(blocks, listItemBlocks(list, src)...)
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

// frontmatterExtent reports the byte range [0, stop) of a leading YAML
// frontmatter block: the document's very first line is "---" alone, and some
// later line is "---" or "..." alone. ok is false when the document does not
// open that way, or opens that way but never closes.
//
// Scope is deliberately narrow — the opening fence must be the first byte of
// the file. A "---" used as a horizontal rule elsewhere in a document has the
// same misparse (see the frontmatter-rendering feedback) but is not what this
// recognises; that is a separate, broader problem.
func frontmatterExtent(src []byte) (start, stop int, ok bool) {
	nl := bytes.IndexByte(src, '\n')
	if nl < 0 {
		return 0, 0, false
	}
	if strings.TrimSpace(string(src[:nl])) != "---" {
		return 0, 0, false
	}
	for offset := nl + 1; offset <= len(src); {
		end := bytes.IndexByte(src[offset:], '\n')
		line := src[offset:]
		if end >= 0 {
			line = src[offset : offset+end]
		}
		if t := strings.TrimSpace(string(line)); t == "---" || t == "..." {
			return 0, offset + len(line), true
		}
		if end < 0 {
			break
		}
		offset += end + 1
	}
	return 0, 0, false
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

// stampAll gives every not-yet-stamped block in blocks a fresh id, in one pass
// over src, and returns the rewritten source together with blocks reparsed
// from it. It is what re-attachment (ID-02) actually relies on: a document
// opened after several blocks acquired threads in an earlier session needs
// all of them turned durable together, not one at a time with a reparse in
// between. blockListItem is the one exception — see the skip below.
//
// It walks from the end of the document backwards, which is the only order in
// which stampID's guarantee — bytes before the stamped block are untouched —
// stays true across the whole pass: stamping earlier blocks first would shift
// every later offset out from under blocks still waiting their turn.
func stampAll(src []byte, blocks []block) ([]byte, []block) {
	out := src
	for i := len(blocks) - 1; i >= 0; i-- {
		if blocks[i].stamped {
			continue
		}
		if blocks[i].kind == blockListItem {
			// Every item of a list shares that list's byte range (see
			// listItemBlocks) rather than a range of its own — stamping one
			// would insert a marker line mid-list, corrupting it, and would
			// collide with every sibling item's stamp landing at the same
			// spot. Sub-list stamping needs its own on-disk marker format;
			// left unstamped, with a content-derived anchor, until that is
			// designed (see blockListItem's doc comment in document.go).
			continue
		}
		out = stampID(out, blocks[i], newBlockID())
	}
	return out, parseDoc(out)
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

	case *ast.Blockquote:
		lines := quoteLinesFor(raw)
		if len(lines) == 0 {
			return block{}, false
		}
		b := block{kind: blockQuote, text: raw, lines: lines, start: start, stop: stop}
		b.anchor = anchorFor(b)
		return b, true

	case *eastast.Table:
		// b.lines keeps the raw `|` source around purely so quoteBlock can
		// reproduce it verbatim on export — the same reason blockList carries
		// lines it never renders from (see the field's doc comment). table
		// is what the interactive renderer actually reads.
		lines := strings.Split(strings.TrimRight(raw, "\n"), "\n")
		b := block{kind: blockTable, text: raw, lines: lines, table: tableFor(t, src), start: start, stop: stop}
		b.anchor = anchorFor(b)
		return b, true

	case *ast.FencedCodeBlock:
		// t.Lines() covers the fence's content only — the ``` delimiters are
		// syntax, not code, the same distinction extent() already draws for
		// stampID. Reading it directly here (rather than trimming raw, which
		// extent() has widened to include the fences for anchoring) is what
		// keeps the highlighter and quoteBlock working on code alone.
		lines := codeLinesFor(t, src)
		b := block{kind: blockCode, text: raw, lines: lines, lang: string(t.Language(src)), start: start, stop: stop}
		b.anchor = anchorFor(b)
		return b, true

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

// inlineRun is a run of a paragraph's text carrying one inline style — the
// unstyled default, bold, code, or a link's visible text. Consecutive
// unstyled characters are folded into one run so wrapInline (review.go) does
// not deal with markup one byte at a time.
type inlineRun struct {
	text             string
	bold, code, link bool
}

// parseInline recognises RENDER-06's three inline forms — **bold**,
// `code`, and [text](url) — and returns the text with their markup stripped,
// split into styled runs. It is intentionally narrow: no italics, no
// nesting, no reference-style links. Those are real gaps, not oversights —
// widening this is its own felt decision, not a side effect of this one.
//
// It scans byte-by-byte rather than with a regexp because the three forms
// need to interrupt plain text at arbitrary points and a hand-rolled scan
// falls out of that more directly. Markers are ASCII (`*`, backtick, `[`,
// `]`, `(`, `)`), so indexing by byte never lands inside a multi-byte UTF-8
// rune — continuation bytes are always >= 0x80.
func parseInline(s string) []inlineRun {
	var runs []inlineRun
	var plain strings.Builder
	flushPlain := func() {
		if plain.Len() > 0 {
			runs = append(runs, inlineRun{text: plain.String()})
			plain.Reset()
		}
	}
	for i := 0; i < len(s); {
		switch {
		case strings.HasPrefix(s[i:], "**"):
			if end := strings.Index(s[i+2:], "**"); end >= 0 {
				flushPlain()
				runs = append(runs, inlineRun{text: s[i+2 : i+2+end], bold: true})
				i += 2 + end + 2
				continue
			}
		case s[i] == '`':
			if end := strings.IndexByte(s[i+1:], '`'); end >= 0 {
				flushPlain()
				runs = append(runs, inlineRun{text: s[i+1 : i+1+end], code: true})
				i += 1 + end + 1
				continue
			}
		case s[i] == '[':
			if close := strings.IndexByte(s[i:], ']'); close >= 0 && i+close+1 < len(s) && s[i+close+1] == '(' {
				if paren := strings.IndexByte(s[i+close+2:], ')'); paren >= 0 {
					flushPlain()
					runs = append(runs, inlineRun{text: s[i+1 : i+close], link: true})
					i += close + 2 + paren + 1
					continue
				}
			}
		}
		plain.WriteByte(s[i])
		i++
	}
	flushPlain()
	return runs
}

// listItemRE recognises the start of a list item line: leading indentation,
// a marker (-, *, + or an ordered N. / N)), then its own text.
var listItemRE = regexp.MustCompile(`^(\s*)([-*+]|\d{1,9}[.)])\s+(.*)$`)

// listItemPos is a listItem plus the 0-based row, within the list's own raw
// text, that it starts on — enough for listItemBlocks to give each item a
// real line number without a second pass.
type listItemPos struct {
	listItem
	row int
}

// listItemsWithPos does the actual splitting; listItemsFor and
// listItemBlocks are its two callers, needing the text alone and the text
// plus position respectively.
//
// It works line by line off the raw source rather than walking goldmark's
// AST: every item, at any nesting depth, starts a line matching
// listItemRE, and a nested item is just such a line with more leading
// whitespace. That means nesting depth falls out of the indentation for
// free — a nested item naturally becomes its own entry with a wider prefix
// and, at render time, a deeper hanging indent — with no separate tree walk
// needed to compute it. A line that does not open a new item is a
// soft-wrapped continuation of prose and folds into the current item's text
// the same way collapse() folds a paragraph's line breaks away; a blank line
// (the separator in a loose list) is simply skipped.
func listItemsWithPos(raw string) []listItemPos {
	var items []listItemPos
	for row, line := range strings.Split(strings.TrimRight(raw, "\n"), "\n") {
		if m := listItemRE.FindStringSubmatch(line); m != nil {
			prefix := m[1] + m[2] + " "
			items = append(items, listItemPos{listItem{prefix: prefix, text: strings.TrimSpace(m[3])}, row})
			continue
		}
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || len(items) == 0 {
			continue
		}
		last := &items[len(items)-1]
		last.text += " " + trimmed
	}
	return items
}

// listItemsFor splits a list block's raw source into its items, so each can
// wrap independently at render time instead of being truncated at the
// measure (2026-08-08 rendering-bugs feedback, defect 2). See
// listItemsWithPos for how the split itself works.
func listItemsFor(raw string) []listItem {
	pos := listItemsWithPos(raw)
	items := make([]listItem, len(pos))
	for i, p := range pos {
		items[i] = p.listItem
	}
	return items
}

// listItemBlocks turns a *ast.List node into one blockListItem per item
// (2026-08-08 line-level-focus feedback) — see blockListItem's doc comment
// for why a list is several blocks rather than one carrying an items slice.
//
// Every item shares the whole list's byte range rather than getting one of
// its own: the range is only ever used for two things elsewhere, and neither
// needs a real per-item range. b.line is computed directly from row instead.
// stampID needs one, but stampAll skips blockListItem entirely (see below),
// so the shared, overlapping range is inert rather than a latent corruption
// — nothing calls stampID on one of these blocks today.
func listItemBlocks(n ast.Node, src []byte) []block {
	start, stop := extent(n, src)
	if start < 0 {
		return nil
	}
	pos := listItemsWithPos(string(src[start:stop]))
	if len(pos) == 0 {
		return nil
	}
	listLine := 1 + bytes.Count(src[:start], []byte{'\n'})
	blocks := make([]block, len(pos))
	for i, p := range pos {
		text := p.prefix + p.text
		b := block{
			kind:    blockListItem,
			text:    text,
			lines:   []string{text},
			items:   []listItem{p.listItem},
			listEnd: i == len(pos)-1,
			line:    listLine + p.row,
			start:   start,
			stop:    stop,
		}
		b.anchor = anchorFor(b)
		blocks[i] = b
	}
	return blocks
}

// codeLinesFor reads a fenced code block's content lines straight from
// goldmark's own line segments, which already exclude the ``` delimiters —
// unlike raw, which extent() has widened to include them for stampID's sake.
// An empty fence yields nil, the same convention every other line splitter
// here uses for "nothing here."
func codeLinesFor(n *ast.FencedCodeBlock, src []byte) []string {
	var buf strings.Builder
	for i := 0; i < n.Lines().Len(); i++ {
		seg := n.Lines().At(i)
		buf.Write(seg.Value(src))
	}
	content := strings.TrimRight(buf.String(), "\n")
	if content == "" {
		return nil
	}
	return strings.Split(content, "\n")
}

// tableFor walks a *eastast.Table's rows into a tableBlock: one header row
// (goldmark always parses GFM's required delimiter row into a TableHeader,
// never a plain TableRow, so the two are told apart by node type rather than
// position), then every remaining row as a body row, in source order.
func tableFor(t *eastast.Table, src []byte) *tableBlock {
	tb := &tableBlock{aligns: make([]tableAlign, len(t.Alignments))}
	for i, a := range t.Alignments {
		tb.aligns[i] = convertAlign(a)
	}
	for r := t.FirstChild(); r != nil; r = r.NextSibling() {
		cells := tableCellsFor(r, src)
		if _, ok := r.(*eastast.TableHeader); ok {
			tb.header = cells
			continue
		}
		tb.rows = append(tb.rows, cells)
	}
	return tb
}

// tableCellsFor reads one table row's cells, each stripped of its inline
// markup by cellText the same way parseInline strips a paragraph's — a table
// cell is prose too, just prose that has to fit a column instead of wrap.
func tableCellsFor(row ast.Node, src []byte) []string {
	var cells []string
	for c := row.FirstChild(); c != nil; c = c.NextSibling() {
		cells = append(cells, cellText(cellRaw(c, src)))
	}
	return cells
}

// cellRaw concatenates a table cell's own line segments — goldmark gives a
// cell's Lines() the cell's raw markdown text directly, unlike a block's
// wider extent(), so no fence- or marker-stripping is needed here.
func cellRaw(n ast.Node, src []byte) string {
	var buf strings.Builder
	lines := n.Lines()
	for i := 0; i < lines.Len(); i++ {
		seg := lines.At(i)
		buf.Write(seg.Value(src))
	}
	return buf.String()
}

// cellText strips a cell's inline markup down to plain text via parseInline,
// discarding the styling: RENDER-06's bold/code/link colours are deliberately
// not carried into a table cell yet (see tableBlock's field comment) — this
// first pass wants only the words a column-width layout has to fit.
func cellText(raw string) string {
	var buf strings.Builder
	for _, run := range parseInline(raw) {
		buf.WriteString(run.text)
	}
	return strings.TrimSpace(buf.String())
}

// convertAlign maps goldmark's own alignment enum onto tableAlign, so
// document.go need not import the extension ast package just to name a
// column's alignment.
func convertAlign(a eastast.Alignment) tableAlign {
	switch a {
	case eastast.AlignLeft:
		return alignLeft
	case eastast.AlignRight:
		return alignRight
	case eastast.AlignCenter:
		return alignCenter
	default:
		return alignNone
	}
}

// quoteLineRE strips a block quote's leading `>` marker (and the one
// optional space after it, per the CommonMark spec) from a source line.
var quoteLineRE = regexp.MustCompile(`^\s*>\s?(.*)$`)

// quoteLinesFor splits a block quote's raw source into its content lines
// with the `>` markers removed, so the renderer and the export path can both
// treat it as prose with paragraph breaks rather than as source to reproduce
// verbatim (2026-08-08 blockquote-rendering feedback: a `>` on every line is
// markdown source, not a rendered quote).
//
// It works line by line, the same way listItemsFor does, rather than
// re-walking goldmark's paragraph children: a blank line inside the quote is
// still `>` alone in the source, and stripping it by regex keeps that as a
// blank line, which is what marks a paragraph break once the markers are
// gone.
//
// A nested quote (`> > text`) only has its outer marker stripped — the inner
// `>` is left in the text. Rendering that as a second rule is the obvious
// next step if it comes up; not worth the machinery for a case that has not
// been seen in practice yet. Likewise a quote containing a list or a fence
// currently comes out as folded prose like any other paragraph text inside a
// quote, losing that inner structure — the same tradeoff RENDER-04 accepted
// for the common case (prose) rather than blocking on the rare one.
func quoteLinesFor(raw string) []string {
	var lines []string
	for _, line := range strings.Split(strings.TrimRight(raw, "\n"), "\n") {
		if m := quoteLineRE.FindStringSubmatch(line); m != nil {
			lines = append(lines, strings.TrimRight(m[1], " \t"))
			continue
		}
		// A lazy continuation line (CommonMark permits omitting the `>` on a
		// paragraph's later lines) — keep it as prose text.
		lines = append(lines, strings.TrimSpace(line))
	}
	return lines
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
