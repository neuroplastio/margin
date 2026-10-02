package review

import (
	"bytes"
	"context"
	"errors"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/neuroplastio/hotty-go"
	"github.com/neuroplastio/hotty-go/hottytest"
)

// hotDocSrc exercises every block kind a surface shows, and one that stays
// cells (the code fence).
const hotDocSrc = "# Title\n\n" +
	"A paragraph with **bold**, a [section link](#second), and a [web link](https://example.com).\n\n" +
	"- one\n- [x] two done\n\n" +
	"> a quote\n\n" +
	"```go\ncode stays cells\n```\n\n" +
	"## Second\n\n" +
	"![a picture](pic.png)\n"

// hotRun runs margin over src on a HOTTY host from hottytest, until play
// returns; then it quits, and hotRun returns the model and the host.
func hotRun(t *testing.T, src string, play func(h *hottytest.Host, m *model), opts ...hottytest.Option) (*model, *hottytest.Host) {
	t.Helper()
	isolateDrafts(t)
	dir := t.TempDir()
	writePNG(t, filepath.Join(dir, "pic.png"), 40, 20)
	path := filepath.Join(dir, "doc.md")
	if err := os.WriteFile(path, []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	doc, data, err := loadDoc(path)
	if err != nil {
		t.Fatal(err)
	}
	m := newModelAt(path, doc, nil)
	m.src = data
	m.hot = newHotState()
	m.hot.mermaid = fakeMermaid
	m.hotOpen()

	h := hottytest.New(t, opts...)
	p := tea.NewProgram(m, tea.WithInput(h), tea.WithOutput(m.hotWatch(h)),
		tea.WithWindowSize(100, 40), tea.WithoutSignalHandler(),
		tea.WithEnvironment([]string{"TERM=xterm-256color"}))
	m.hot.s.Attach(p.Send)
	done := make(chan error, 1)
	go func() { _, err := p.Run(); done <- err }()
	play(h, m)
	h.Type("q")
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("margin did not quit")
	}
	return m, h
}

// fakeMermaid stands in for merman (internal/mermaidsvg has its own tests,
// and compiling it takes seconds): an SVG 300×144 for a flowchart, and the
// parser's refusal for anything else.
func fakeMermaid(_ context.Context, src string) ([]byte, error) {
	if !strings.HasPrefix(strings.TrimSpace(src), "flowchart") {
		return nil, errors.New("mermaid: Unsupported diagram type")
	}
	return []byte(`<svg xmlns="http://www.w3.org/2000/svg" width="300" height="143.5" viewBox="0 0 300 143.5"><rect width="10" height="10"/></svg>`), nil
}

func writePNG(t *testing.T, path string, w, h int) {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for x := range w {
		img.Set(x, h/2, color.RGBA{R: 255, A: 255})
	}
	var b bytes.Buffer
	if err := png.Encode(&b, img); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, b.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}
}

// surfaceWith waits for a placed surface whose text has want in it.
func surfaceWith(t *testing.T, h *hottytest.Host, want string) *hottytest.Surface {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		for _, s := range h.Surfaces() {
			if !strings.HasPrefix(s.Name(), hotMeasureName) && s.Placed() && strings.Contains(s.Text(), want) {
				return s
			}
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("no surface shows %q; screen:\n%s", want, h.Screen())
	return nil
}

// On a host the prose blocks are surfaces, measured by the host and placed
// in the document's column; code stays in cells; the stylesheet and the
// image go as resources; and quitting deletes every surface.
func TestHottyProseIsSurfaces(t *testing.T) {
	var para, h1, item, quote, pic *hottytest.Surface
	var screen string
	_, h := hotRun(t, hotDocSrc, func(h *hottytest.Host, m *model) {
		para = surfaceWith(t, h, "A paragraph with bold")
		h1 = surfaceWith(t, h, "Title")
		item = surfaceWith(t, h, "two done")
		quote = surfaceWith(t, h, "a quote")
		waitFor(t, "the picture's surface", func() bool {
			for _, s := range h.Surfaces() {
				if strings.Contains(s.HTML(), "cid:img-") && s.Placed() {
					pic = s
					return true
				}
			}
			return false
		})
		// The frame that blanks the paragraph's rows follows its placement:
		// Bubble Tea draws on its next tick.
		waitFor(t, "the frame after the surfaces", func() bool {
			screen = h.Screen()
			return !strings.Contains(screen, "A paragraph with")
		})
	}, hottytest.AutoRows(func(*hottytest.Surface, int) int { return 2 }))

	if _, _, ok := h.Resource(hotCSSID); !ok {
		t.Error("the stylesheet was not sent")
	}
	measured := false
	for _, c := range h.Commands() {
		if c.Get("a") == "place" && strings.HasPrefix(c.Get("s"), hotMeasureName+"-") && c.Get("r") == "auto" && c.Get("n") != "" {
			measured = true
		}
	}
	if !measured {
		t.Error("no block was measured with r=auto")
	}

	p := para.Placement()
	if p.Cols != 100-2*gutterW || p.Rows != 2 {
		t.Errorf("paragraph placed %dx%d, want %dx2 (the column, the measured rows)", p.Cols, p.Rows, 100-2*gutterW)
	}
	if col, _ := para.At(); col != gutterW {
		t.Errorf("paragraph at column %d, want %d (beside the gutter)", col, gutterW)
	}
	doc := para.HTML()
	for _, want := range []string{`<strong>bold</strong>`, `href="#second"`, `href="https://example.com" target="_blank"`, `class="para"`} {
		if !strings.Contains(doc, want) {
			t.Errorf("paragraph's document lacks %s:\n%s", want, doc)
		}
	}
	if !strings.Contains(h1.HTML(), "<h1>Title</h1>") || !strings.Contains(h1.HTML(), `class="h1"`) {
		t.Errorf("heading's document: %s", h1.HTML())
	}
	if !strings.Contains(item.HTML(), "☑") || !strings.Contains(item.HTML(), "li done") {
		t.Errorf("a done task's document: %s", item.HTML())
	}
	if !strings.Contains(quote.HTML(), "<blockquote>") {
		t.Errorf("quote's document: %s", quote.HTML())
	}
	if pic == nil || !strings.Contains(pic.HTML(), `width="40" height="20"`) {
		t.Errorf("no image surface sized from the file: %v", pic)
	} else {
		id := pic.HTML()[strings.Index(pic.HTML(), "cid:img-")+len("cid:"):]
		id = id[:strings.IndexByte(id, '"')]
		if mime, _, ok := h.Resource(id); !ok || mime != "image/png" {
			t.Errorf("the image resource %s: %q, %v", id, mime, ok)
		}
	}

	if !strings.Contains(screen, "code stays cells") {
		t.Errorf("the code block is not in cells:\n%s", screen)
	}
	for _, s := range h.Surfaces() {
		if strings.Contains(s.Text(), "code stays cells") {
			t.Errorf("the code block is a surface: %s", s.Name())
		}
	}

	if left := h.Surfaces(); len(left) != 0 {
		var names []string
		for _, s := range left {
			names = append(names, s.Name())
		}
		t.Errorf("surfaces left after quitting: %v", names)
	}
}

// A mermaid fence on a host is its diagram as SVG, sent as a resource and
// shown by an <img> sized from it; one merman cannot draw stays cells.
func TestHottyMermaidIsAnSVG(t *testing.T) {
	src := "Before.\n\n```mermaid\nflowchart LR\n    a --> b\n```\n\n```mermaid\npie\n    \"x\" : 1\n```\n"
	var diagram *hottytest.Surface
	var screen string
	_, h := hotRun(t, src, func(h *hottytest.Host, m *model) {
		waitFor(t, "the diagram's surface", func() bool {
			for _, s := range h.Surfaces() {
				if strings.Contains(s.HTML(), `class="diagram"`) && s.Placed() {
					diagram = s
					return true
				}
			}
			return false
		})
		waitFor(t, "the frame after it", func() bool {
			screen = h.Screen()
			return !strings.Contains(screen, "a --> b") && !strings.Contains(screen, "│ a")
		})
	})
	doc := diagram.HTML()
	if !strings.Contains(doc, `width="300" height="144"`) || !strings.Contains(doc, `src="cid:mmd-`) {
		t.Errorf("the diagram's document: %s", doc)
	}
	id := doc[strings.Index(doc, "cid:mmd-")+len("cid:"):]
	id = id[:strings.IndexByte(id, '"')]
	if mime, data, ok := h.Resource(id); !ok || mime != "image/svg+xml" || !bytes.HasPrefix(data, []byte("<svg")) {
		t.Errorf("the diagram's resource %s: %q %v", id, mime, ok)
	}
	if !strings.Contains(screen, "pie") {
		t.Errorf("the diagram merman could not draw is not in cells:\n%s", screen)
	}
}

func TestSVGSize(t *testing.T) {
	for _, c := range []struct {
		svg  string
		w, h int
	}{
		{`<svg width="300" height="143.5">`, 300, 144},
		{`<?xml version="1.0"?><svg xmlns="x" height="20px" width="10px"><rect width="5" height="5"/></svg>`, 10, 20},
		{`<svg viewBox="0 0 1 1">`, 0, 0},
		{`no svg`, 0, 0},
	} {
		if w, h := svgSize([]byte(c.svg)); w != c.w || h != c.h {
			t.Errorf("svgSize(%s) = %d×%d, want %d×%d", c.svg, w, h, c.w, c.h)
		}
	}
}

// A press on a block's surface focuses the block, as a click on cells does.
func TestHottyPressFocuses(t *testing.T) {
	m, _ := hotRun(t, hotDocSrc, func(h *hottytest.Host, m *model) {
		s := surfaceWith(t, h, "a quote")
		if err := h.Press(s.Name(), ""); err != nil {
			t.Fatal(err)
		}
	})
	if got := m.entries[m.at.entry].b; got.kind != blockQuote {
		t.Errorf("focus is on %v %q, want the quote", got.kind, got.text)
	}
}

// A click on a link to a heading in the document follows it.
func TestHottyLinkClickFollows(t *testing.T) {
	m, _ := hotRun(t, hotDocSrc, func(h *hottytest.Host, m *model) {
		s := surfaceWith(t, h, "A paragraph with bold")
		if err := h.ClickLink(s.Name(), "#second"); err != nil {
			t.Fatal(err)
		}
	})
	if got := m.entries[m.at.entry].b; got.kind != blockHeading || got.text != "Second" {
		t.Errorf("focus is on %v %q, want the heading Second", got.kind, got.text)
	}
}

// Marking a block reviewed sends its document again, dimmed; the search marks
// what it finds in a surface, and n lands on a match there.
func TestHottyReviewStateAndSearch(t *testing.T) {
	m, _ := hotRun(t, hotDocSrc, func(h *hottytest.Host, m *model) {
		s := surfaceWith(t, h, "A paragraph with bold")
		h.Type("j") // the paragraph
		h.Type("r")
		waitFor(t, "the paragraph dimmed", func() bool { return strings.Contains(s.HTML(), `class="para ok"`) })
		h.Type("/quote\r")
		q := surfaceWith(t, h, "a quote")
		waitFor(t, "the quote marked", func() bool { return strings.Contains(q.HTML(), "<mark>quote</mark>") })
	})
	if got := m.entries[m.at.entry].b; got.kind != blockQuote {
		t.Errorf("the search landed on %v %q, want the quote", got.kind, got.text)
	}
}

func waitFor(t *testing.T, what string, ok func() bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for !ok() {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s", what)
		}
		time.Sleep(5 * time.Millisecond)
	}
}

// A terminal that is not a host gets cells, and no HOTTY at all past the
// query.
func TestHottyTextTerminal(t *testing.T) {
	_, h := hotRun(t, hotDocSrc, func(h *hottytest.Host, m *model) {
		waitFor(t, "the paragraph in cells", func() bool { return strings.Contains(h.Screen(), "A paragraph with") })
	}, hottytest.Text())
	for _, c := range h.Commands() {
		if c.Get("a") != "q" {
			t.Errorf("a HOTTY command went to a terminal that is not a host: %v", c.Control)
		}
	}
}

func TestMarkMatches(t *testing.T) {
	for _, c := range []struct{ doc, q, want string }{
		{`<p>Foo bar foo</p>`, "foo", `<p><mark>Foo</mark> bar <mark>foo</mark></p>`},
		{`<a href="foo">x</a>`, "foo", `<a href="foo">x</a>`},
		{`<p>a &amp; b</p>`, "& b", `<p>a <mark>&amp; b</mark></p>`},
		{`<p>none</p>`, "zz", `<p>none</p>`},
	} {
		if got := markMatches(c.doc, c.q); got != c.want {
			t.Errorf("markMatches(%q, %q) = %q, want %q", c.doc, c.q, got, c.want)
		}
	}
}

func TestExternal(t *testing.T) {
	for href, want := range map[string]bool{
		"https://example.com": true, "mailto:a@b": true, "other.md": false,
		"#x": false, "dir/a:b.md": false, "cid:x": false, "": false,
	} {
		if got := external(href); got != want {
			t.Errorf("external(%q) = %v", href, got)
		}
	}
}

func TestHotName(t *testing.T) {
	a := hotName("/r/a.md", "^1a2b#2", 3)
	if !hotty.ValidName(a) || a == hotName("/r/b.md", "^1a2b#2", 3) {
		t.Errorf("hotName: %q is invalid, or the same in another document", a)
	}
	if hotName("/r/a.md", "", 3) == hotName("/r/a.md", "", 4) {
		t.Error("blocks without an anchor share a name")
	}
}

func TestHotListItem(t *testing.T) {
	var d hotDocs
	got := d.listItem(listItem{prefix: "    1. ", text: "**deep** item"})
	for _, want := range []string{`--depth: 2`, `<span class="mk">1.</span>`, `<strong>deep</strong> item`} {
		if !strings.Contains(got, want) {
			t.Errorf("list item lacks %s: %s", want, got)
		}
	}
	if strings.Contains(got, "<p>") {
		t.Errorf("list item text is a paragraph: %s", got)
	}
}

// A web image is a hyperlink to it, since a surface fetches nothing.
func TestHotWebImage(t *testing.T) {
	d := hotDocs{dir: t.TempDir()}
	got := d.render("![logo](https://example.com/a.png)")
	if !strings.Contains(got, `<a href="https://example.com/a.png" target="_blank">[image: logo]</a>`) {
		t.Errorf("web image: %s", got)
	}
	// A host whose img-src allows https fetches it: the image stays, and
	// the document asks for it.
	for _, net := range [][]string{{"https:"}, {"https://example.com"}} {
		d := hotDocs{dir: t.TempDir(), net: net}
		got := d.render("![logo](https://example.com/a.png)")
		if !strings.Contains(got, `<img src="https://example.com/a.png" alt="logo">`) {
			t.Errorf("net %v: web image: %s", net, got)
		}
		if doc := hotDocument(got, "para", false, false, ""); !strings.Contains(doc, `<meta name="hotty-network" content="img-src https:">`) {
			t.Errorf("net %v: the document does not ask: %s", net, doc)
		}
	}
	d.net = []string{"https://other.org"}
	if got := d.render("![logo](http://example.com/a.png)"); strings.Contains(got, "<img") {
		t.Errorf("an http image, or one from an origin the host does not allow, is a link: %s", got)
	}
	if doc := hotDocument("<p>x</p>", "para", false, false, ""); strings.Contains(doc, "hotty-network") {
		t.Errorf("a document with no web image asks for the network: %s", doc)
	}
	got = d.render("![gone](missing.png)")
	if !strings.Contains(got, `<a href="missing.png">[image: gone]</a>`) {
		t.Errorf("missing image: %s", got)
	}
}
