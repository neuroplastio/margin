package review

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"html"
	"image"
	_ "image/gif" // image.DecodeConfig learns the formats it sizes
	_ "image/jpeg"
	_ "image/png"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/neuroplastio/hotty-go"
	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/extension"
	"github.com/yuin/goldmark/text"
)

// The HTML a block becomes on a HOTTY host (D17). Each prose block —
// heading, paragraph, list item, quote — is a document of its own, laid out
// by the host in the reading font and placed over the rows the block takes.
// The document is the block's own markdown, rendered by goldmark: the same
// parser that found the block, so what the host shows is what the source
// says, inline markup and all. Review state (reviewed, selected) and the
// search's matches are classes and <mark>s on top, none of which changes a
// row: the host measured the block without them.

// hotCSSID is the stylesheet every block's document links to, sent once as a
// resource (SPEC §7.1) so a block costs its own markup and no more.
const hotCSSID = "margin-css"

// hotCSS sets prose in the reading font, one line to a terminal row
// (--hotty-cell-h, SPEC §8), so a block's rows are its lines and the gutter's
// marks sit beside them. Nothing that depends on review state (.ok, .sel,
// <mark>) may move a line: the host measured the block without it. The
// colours are margin's own cell palette, by their xterm-256 values, so a page
// that is half surfaces and half cells reads as one: text 252, reviewed 240,
// headings 212/183/245, links 111, inline code 216, the selection 236 and
// the search 94.
const hotCSS = `
:root {
  --text: #d0d0d0; --reviewed: #585858; --dim: #808080;
  --h1: #ff87d7; --h2: #d7afff; --h3: #8a8a8a;
  --link: #87afff; --code: #ffaf87; --sel: #303030; --hit: #875f00;
  --row: var(--hotty-cell-h, 18px); --col: var(--hotty-cell-w, 9px);
  --sans: "IBM Plex Sans", "Inter", system-ui, sans-serif;
  --mono: var(--hotty-font, ui-monospace, monospace);
}
html, body { margin: 0; height: auto; }
main {
  display: block; box-sizing: border-box; margin: 0; padding: 0;
  max-width: 80ch; color: var(--text);
  font: 1rem/var(--row) var(--sans); overflow-wrap: break-word;
}
main.ok { color: var(--reviewed); }
main.sel { background: var(--sel); }
main p, main ul, main ol, main blockquote, main pre, main h1, main h2,
main h3, main h4, main h5, main h6 { margin: 0; }
main blockquote > * + *, main li > * + * { margin-top: var(--row); }
main h1, main h2, main h3, main h4, main h5, main h6 { font-weight: 650; }
main h1 { font-size: 1.6rem; line-height: calc(2 * var(--row)); color: var(--h1); }
main h2 { font-size: 1.2rem; color: var(--h2); font-weight: 600; }
main h3 { color: var(--h3); }
main h4, main h5, main h6 { color: var(--h3); font-weight: 500; }
main strong, main b { font-weight: 650; }
main a { color: var(--link); text-decoration: underline; text-underline-offset: 2px; }
main code, main kbd {
  /* line-height 1: another font's inline box at the row's height would
     stand taller than the row, and its line would take two. */
  font-family: var(--mono); font-size: .92em; line-height: 1; color: var(--code);
  background: rgba(255, 175, 135, .08); border-radius: 3px; padding: 0 2px;
}
main.ok code { color: var(--reviewed); background: none; }
main pre { font: 1rem/var(--row) var(--mono); white-space: pre-wrap; }
main pre code { color: inherit; background: none; padding: 0; font-size: inherit; }
main del, main s { color: var(--dim); }
main mark { background: var(--hit); color: inherit; }
main blockquote { padding-left: calc(2 * var(--col)); box-shadow: inset 2px 0 0 var(--dim); }
main ul, main ol { padding-left: calc(3 * var(--col)); }
main .li { display: flex; padding-left: calc(var(--depth, 0) * 3 * var(--col)); }
main .li .mk { flex: none; min-width: calc(2 * var(--col)); padding-right: calc(var(--col) / 2); color: var(--dim); }
main .li .mk.box { font-family: var(--mono); }
main .li .lt { flex: 1; min-width: 0; }
main .li.done .lt { color: var(--dim); }
main img { display: block; max-width: 100%; max-height: calc(20 * var(--row)); width: auto; height: auto; }
main hr { border: 0; height: var(--row); background: linear-gradient(var(--dim), var(--dim)) center / 100% 1px no-repeat; }
main table { border-collapse: collapse; }
main th, main td { padding: 0 var(--col); text-align: left; }
`

// hotKind reports whether a block of this kind shows as a surface. Code,
// mermaid, tables, frontmatter and raw blocks stay cells: their layout is
// their content, the cells already draw it exactly, and a table's rows are
// what a line dive walks (D17).
func hotKind(k blockKind) bool {
	switch k {
	case blockHeading, blockPara, blockListItem, blockQuote:
		return true
	}
	return false
}

// hotMarkdown is the goldmark that renders blocks: the parser's own
// extensions (GFM), and raw HTML left out, as goldmark leaves it by default.
var hotMarkdown = goldmark.New(goldmark.WithExtensions(extension.GFM))

// hotDocs makes the documents of one review's blocks. dir is where the
// document's relative images are, and res receives each image's bytes, once,
// before a document refers to it.
type hotDocs struct {
	dir  string
	res  func(id, mime string, data []byte)
	imgs map[string]hotImage // by the image's path
	html map[string]string   // by the block's markdown

	// The current document's: each block's body, and where its lines
	// start. open forgets them.
	byBlock map[hotBlockKey]string
	starts  []int
}

// hotBlockKey is a block of the current document, by where it is.
type hotBlockKey struct {
	kind                   blockKind
	line, end, start, stop int
	anchor                 string
}

// open forgets the current document's blocks: another file, or the same one
// read again.
func (d *hotDocs) open(dir string) {
	d.dir, d.imgs, d.byBlock, d.starts = dir, nil, nil, nil
}

type hotImage struct {
	id   string // the resource; "" when the file cannot be shown
	w, h int    // its size in pixels, 0 when unknown
}

// hotImageMax is the largest image sent: past it, the image is a link to the
// file, as one on the web is.
const hotImageMax = 8 << 20

// base returns a block's document body: its markdown as HTML, before any
// review state. "" means the block has nothing to show as a surface.
func (d *hotDocs) base(b block, src []byte) string {
	bk := hotBlockKey{b.kind, b.line, b.endLine, b.start, b.stop, b.anchor}
	if s, ok := d.byBlock[bk]; ok {
		return s
	}
	if d.starts == nil && src != nil {
		d.starts = append(d.starts, 0)
		for i, c := range src {
			if c == '\n' {
				d.starts = append(d.starts, i+1)
			}
		}
	}
	md := blockMarkdown(b, src, d.starts)
	if d.byBlock == nil {
		d.byBlock = map[hotBlockKey]string{}
	}
	if strings.TrimSpace(md) == "" {
		d.byBlock[bk] = ""
		return ""
	}
	if b.kind == blockListItem {
		md = "li\x00" + md // a list item's key differs from a paragraph of the same text
	}
	if s, ok := d.html[md]; ok {
		d.byBlock[bk] = s
		return s
	}
	var s string
	if b.kind == blockListItem && len(b.items) > 0 {
		s = d.listItem(b.items[0])
	} else {
		s = d.render(md)
	}
	if d.html == nil {
		d.html = map[string]string{}
	}
	d.html[md] = s
	d.byBlock[bk] = s
	return s
}

// blockMarkdown is the block's own markdown: its whole source lines (a
// heading's byte range starts after its `#`s), found by where src's lines
// start, or for a block with no source (a seeded one) what it carries.
func blockMarkdown(b block, src []byte, starts []int) string {
	if b.kind == blockListItem && len(b.items) > 0 {
		return b.items[0].prefix + b.items[0].text
	}
	if src != nil && b.line > 0 && b.endLine >= b.line && b.endLine <= len(starts) {
		end := len(src)
		if b.endLine < len(starts) {
			end = starts[b.endLine]
		}
		return string(src[starts[b.line-1]:end])
	}
	switch b.kind {
	case blockHeading:
		return strings.Repeat("#", max(1, b.level)) + " " + b.text
	case blockQuote:
		return "> " + strings.Join(b.lines, "\n> ")
	}
	return b.text
}

// render turns markdown into HTML, with the links and images a surface can
// use (link and image, below).
func (d *hotDocs) render(md string) string {
	doc := hotMarkdown.Parser().Parse(text.NewReader([]byte(md)))
	d.adapt(doc, []byte(md))
	var b bytes.Buffer
	if err := hotMarkdown.Renderer().Render(&b, []byte(md), doc); err != nil {
		return ""
	}
	return strings.TrimSpace(b.String())
}

// inline renders one line of inline markdown, without the paragraph around it.
func (d *hotDocs) inline(md string) string {
	s := d.render(md)
	if strings.HasPrefix(s, "<p>") && strings.HasSuffix(s, "</p>") && strings.Count(s, "<p>") == 1 {
		s = s[len("<p>") : len(s)-len("</p>")]
	}
	return s
}

// adapt rewrites what a surface cannot take as the source has it. A link to
// the web is a hyperlink (target=_blank), which the terminal opens, while a
// link within the review (`#heading`, `other.md`) stays the document's, so
// a click on it reaches margin (SPEC §9), which follows it. An image is a
// resource sent in-band (SPEC §7.1), since a surface fetches nothing; one on
// the web, or one too large, becomes a hyperlink to it.
func (d *hotDocs) adapt(doc ast.Node, src []byte) {
	var images []*ast.Image
	_ = ast.Walk(doc, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		switch n := n.(type) {
		case *ast.Link:
			if external(string(n.Destination)) {
				n.SetAttributeString("target", "_blank")
			}
		case *ast.AutoLink:
			// goldmark's autolinks take no attributes; they are web links
			// by construction, and a click on one reaches margin, which
			// says so.
		case *ast.Image:
			images = append(images, n)
		}
		return ast.WalkContinue, nil
	})
	for _, img := range images {
		dest := string(img.Destination)
		if !external(dest) {
			if im := d.image(dest); im.id != "" {
				img.Destination = []byte("cid:" + im.id)
				if im.w > 0 && im.h > 0 {
					img.SetAttributeString("width", fmt.Sprint(im.w))
					img.SetAttributeString("height", fmt.Sprint(im.h))
				}
				continue
			}
		}
		// A link in the image's place, its alt text the link's.
		link := ast.NewLink()
		link.Destination = img.Destination
		link.Title = img.Title
		if external(dest) {
			link.SetAttributeString("target", "_blank")
		}
		alt := strings.TrimSpace(altText(img, src))
		if alt == "" {
			alt = dest
		}
		link.AppendChild(link, ast.NewString([]byte("[image: "+alt+"]")))
		img.Parent().ReplaceChild(img.Parent(), img, link)
	}
}

// altText is an image's alt text: the text of its children.
func altText(n ast.Node, src []byte) string {
	var b strings.Builder
	for c := n.FirstChild(); c != nil; c = c.NextSibling() {
		switch c := c.(type) {
		case *ast.Text:
			b.Write(c.Segment.Value(src))
		case *ast.String:
			b.Write(c.Value)
		default:
			b.WriteString(altText(c, src))
		}
	}
	return b.String()
}

// external reports whether a link leaves the review: a URL with a scheme.
func external(href string) bool {
	i := strings.Index(href, ":")
	if i <= 0 {
		return false
	}
	scheme := strings.ToLower(href[:i])
	for _, r := range scheme {
		if !(r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || r == '+' || r == '-' || r == '.') {
			return false
		}
	}
	return scheme != "cid"
}

// image reads a relative image beside the document, sends it as a resource
// the first time, and says how to refer to it.
func (d *hotDocs) image(dest string) hotImage {
	p := dest
	if i := strings.IndexAny(p, "?#"); i >= 0 {
		p = p[:i]
	}
	if p == "" {
		return hotImage{}
	}
	p = filepath.Join(d.dir, filepath.FromSlash(p))
	if im, ok := d.imgs[p]; ok {
		return im
	}
	im := hotImage{}
	if data, err := os.ReadFile(p); err == nil && len(data) > 0 && len(data) <= hotImageMax {
		mime := http.DetectContentType(data)
		if strings.EqualFold(filepath.Ext(p), ".svg") {
			mime = "image/svg+xml"
		}
		if strings.HasPrefix(mime, "image/") {
			sum := sha256.Sum256(data)
			im.id = "img-" + hex.EncodeToString(sum[:8])
			if c, _, err := image.DecodeConfig(bytes.NewReader(data)); err == nil {
				im.w, im.h = c.Width, c.Height
			}
			if d.res != nil {
				d.res(im.id, mime, data)
			}
		}
	}
	if d.imgs == nil {
		d.imgs = map[string]hotImage{}
	}
	d.imgs[p] = im
	return im
}

// listItem is one item of a list, as margin splits lists (D12): its marker
// in a column of its own and its text beside it, indented by its depth.
func (d *hotDocs) listItem(it listItem) string {
	indent := len(it.prefix) - len(strings.TrimLeft(it.prefix, " \t"))
	depth := indent / 2
	marker := strings.TrimSpace(it.prefix)
	switch marker {
	case "-", "*", "+":
		marker = "•"
	}
	body := it.text
	class, box := "li", ""
	switch {
	case strings.HasPrefix(body, "[ ] "):
		box, body = "☐", body[4:]
	case strings.HasPrefix(body, "[x] "), strings.HasPrefix(body, "[X] "):
		box, body, class = "☑", body[4:], "li done"
	}
	mk := `<span class="mk">` + html.EscapeString(marker) + `</span>`
	if box != "" {
		mk = `<span class="mk box">` + box + `</span>`
	}
	return fmt.Sprintf(`<div class="%s" style="--depth: %d">%s<div class="lt">%s</div></div>`,
		class, depth, mk, d.inline(body))
}

// hotDocument is a block's whole document: the shared stylesheet, and its
// body in a <main> that carries the block's kind and review state.
func hotDocument(body, kind string, reviewed, selected bool, query string) string {
	class := kind
	if reviewed {
		class += " ok"
	}
	if selected {
		class += " sel"
	}
	if query != "" {
		body = markMatches(body, query)
	}
	return `<link rel="stylesheet" href="cid:` + hotCSSID + `"><main class="` + class + `">` + body + `</main>`
}

// hotClass names a block's kind for the stylesheet.
func hotClass(b block) string {
	switch b.kind {
	case blockHeading:
		return fmt.Sprintf("h%d", max(1, min(6, b.level)))
	case blockListItem:
		return "item"
	case blockQuote:
		return "quote"
	}
	return "para"
}

// markMatches wraps what the search found in the document's text in <mark>,
// matching as the search does in cells: case-insensitive, never overlapping.
// Only text between tags is searched, so markup is never split; a match
// that crosses an element (`foo **bar**`) is found in cells but not marked
// here.
func markMatches(doc, query string) string {
	q := []rune(strings.ToLower(query))
	if len(q) == 0 {
		return doc
	}
	var b strings.Builder
	for len(doc) > 0 {
		lt := strings.IndexByte(doc, '<')
		if lt < 0 {
			lt = len(doc)
		}
		b.WriteString(markText(doc[:lt], q))
		doc = doc[lt:]
		if doc == "" {
			break
		}
		gt := strings.IndexByte(doc, '>')
		if gt < 0 {
			b.WriteString(doc)
			break
		}
		b.WriteString(doc[:gt+1])
		doc = doc[gt+1:]
	}
	return b.String()
}

// markText marks the matches in one run of escaped text.
func markText(escaped string, q []rune) string {
	if escaped == "" {
		return ""
	}
	plain := []rune(html.UnescapeString(escaped))
	lower := []rune(strings.ToLower(string(plain)))
	if len(lower) != len(plain) {
		return escaped // a case mapping that changes length: leave it unmarked
	}
	var b strings.Builder
	last := 0
	for i := 0; i+len(q) <= len(lower); i++ {
		if runesEqual(lower[i:i+len(q)], q) {
			b.WriteString(html.EscapeString(string(plain[last:i])))
			b.WriteString("<mark>" + html.EscapeString(string(plain[i:i+len(q)])) + "</mark>")
			i += len(q) - 1
			last = i + 1
		}
	}
	if last == 0 {
		return escaped
	}
	b.WriteString(html.EscapeString(string(plain[last:])))
	return b.String()
}

// hotName is a block's surface name: unique in the review, and the same from
// frame to frame, so a block scrolled away and back is shown again rather
// than sent again. The document's path is in it, so a tree review's blocks
// never take each other's surfaces.
func hotName(path, anchor string, entry int) string {
	sum := sha256.Sum256([]byte(path))
	id := anchor
	if id == "" {
		id = fmt.Sprintf("e%d", entry)
	}
	return hotty.SurfaceName("m" + hex.EncodeToString(sum[:4]) + "-" + strings.TrimPrefix(id, "^"))
}
