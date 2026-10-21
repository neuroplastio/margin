package mermaidsvg

import (
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

const flowchart = `flowchart TD
    A[Push to main] --> B[Run tests]
    B -->|green| C[Build images]
    B -->|red| Z[Notify slack]
    C --> D[Run migrations]
    C --> E[Promote canary]
    D --> F[Verify in staging]
    E --> F
    F -->|pass| G[Roll out prod]
    F -->|fail| H[Rollback]
    G --> I[Done]
    H --> Z
    Z --> J[Stop]`

const sequence = `sequenceDiagram
    participant CI as CI runner
    participant API as Release API
    CI->>API: create release
    API-->>CI: created
    Note over CI,API: poll loop, 2s interval`

const state = `stateDiagram-v2
    [*] --> Queued
    Queued --> Building: ready
    state Building {
        [*] --> Fetching
        Fetching --> [*]
    }
    Building --> Live: promoted
    Live --> [*]
    note right of Live : retried by the supervisor`

const class = `classDiagram
    class Animal {
        <<abstract>>
        +String name
        +makeSound() void
    }
    Animal <|-- Dog`

const er = `erDiagram
    RELEASE ||--o{ DEPLOY : triggers
    RELEASE {
        int id PK
        string sha
    }`

const quadrant = `quadrantChart
    title Effort vs value
    x-axis Low effort --> High effort
    y-axis Low value --> High value
    Retries: [0.3, 0.6]`

const xychart = `xychart-beta
    title "Build minutes"
    x-axis [mon, tue, wed]
    y-axis "Minutes" 0 --> 60
    bar [12, 30, 45]`

const sankey = `sankey-beta

Budget,Builds,10
Budget,Tests,5`

// svgDoc is what the tests read back from a rendered SVG.
type svgDoc struct {
	width, height float64
	viewBox       [4]float64
	texts         []string
	textYs        []float64 // each <text> or <tspan> y, in the root's space
	foreign       int
}

// translateY is the y of a transform that only translates, and false for
// one that does anything else.
func translateY(t *testing.T, transform string) (float64, bool) {
	if transform == "" {
		return 0, true
	}
	args, ok := strings.CutPrefix(transform, "translate(")
	if !ok || !strings.HasSuffix(args, ")") {
		return 0, false
	}
	f := strings.FieldsFunc(strings.TrimSuffix(args, ")"), func(r rune) bool { return r == ',' || r == ' ' })
	if len(f) != 2 {
		return 0, len(f) == 1
	}
	return number(t, "translate y", f[1]), true
}

// parseSVG checks that out is one well-formed XML document rooted at <svg>
// and collects what the tests assert on.
func parseSVG(t *testing.T, out []byte) svgDoc {
	t.Helper()
	dec := xml.NewDecoder(strings.NewReader(string(out)))
	var doc svgDoc
	var root bool
	var inText int
	var text strings.Builder // one <text>'s characters, across its <tspan>s
	// offsets is each open element's y offset from the root, NaN under a
	// transform other than a translation.
	offsets := []float64{0}
	for {
		tok, err := dec.Token()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			t.Fatalf("SVG is not well-formed XML: %v", err)
		}
		switch el := tok.(type) {
		case xml.StartElement:
			attr := func(name string) string {
				for _, a := range el.Attr {
					if a.Name.Local == name {
						return a.Value
					}
				}
				return ""
			}
			if !root {
				root = true
				if el.Name.Local != "svg" {
					t.Fatalf("root element is <%s>, want <svg>", el.Name.Local)
				}
				doc.width = number(t, "width", attr("width"))
				doc.height = number(t, "height", attr("height"))
				f := strings.Fields(strings.ReplaceAll(attr("viewBox"), ",", " "))
				if len(f) != 4 {
					t.Fatalf("viewBox = %q, want four numbers", attr("viewBox"))
				}
				for i := range f {
					doc.viewBox[i] = number(t, "viewBox", f[i])
				}
			}
			off := offsets[len(offsets)-1]
			if dy, ok := translateY(t, attr("transform")); ok {
				off += dy
			} else {
				off = math.NaN()
			}
			offsets = append(offsets, off)
			switch el.Name.Local {
			case "foreignObject":
				doc.foreign++
			case "text", "tspan":
				if el.Name.Local == "text" {
					if inText == 0 {
						text.Reset()
					}
					inText++
				}
				if y := attr("y"); y != "" && !math.IsNaN(off) {
					doc.textYs = append(doc.textYs, off+number(t, "text y", y))
				}
			}
		case xml.EndElement:
			offsets = offsets[:len(offsets)-1]
			if el.Name.Local == "text" {
				if inText--; inText == 0 {
					if s := strings.TrimSpace(text.String()); s != "" {
						doc.texts = append(doc.texts, s)
					}
				}
			}
		case xml.CharData:
			if inText > 0 {
				text.Write(el)
			}
		}
	}
	if !root {
		t.Fatal("no root element")
	}
	return doc
}

func number(t *testing.T, what, s string) float64 {
	t.Helper()
	v, err := strconv.ParseFloat(strings.TrimSuffix(s, "px"), 64)
	if err != nil || math.IsNaN(v) || math.IsInf(v, 0) {
		t.Fatalf("%s = %q, want a number", what, s)
	}
	return v
}

func render(t *testing.T, src string) svgDoc {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	out, err := Render(ctx, src)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	return parseSVG(t, out)
}

func TestRenderKinds(t *testing.T) {
	for _, tc := range []struct {
		name, src string
		want      []string // label text that must come out as <text>
	}{
		{"flowchart", flowchart, []string{"Push to main", "green", "Stop"}},
		{"sequence", sequence, []string{"CI runner", "create release", "poll loop, 2s interval"}},
		{"state", state, []string{"Queued", "promoted", "retried by the supervisor"}},
		{"class", class, []string{"Animal", "+String name"}},
		{"er", er, []string{"RELEASE", "triggers", "sha"}},
		{"quadrant", quadrant, []string{"Effort vs value", "Retries"}},
		{"xychart", xychart, []string{"Build minutes", "tue"}},
		{"sankey", sankey, []string{"Budget", "Tests"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, err := Render(context.Background(), tc.src)
			if err != nil {
				t.Fatalf("Render: %v", err)
			}
			// usvg reads neither, and draws what uses them black.
			for _, css := range []string{"var(", "color-mix(", "--ondine", "--_"} {
				if i := strings.Index(string(out), css); i >= 0 {
					t.Errorf("%q survived flattening: %.80q", css, out[max(0, i-20):])
				}
			}
			if !strings.Contains(string(out), palette["--ondine-surface"]) && !strings.Contains(string(out), palette["--ondine-series-1"]) {
				t.Error("no colour from margin's palette")
			}
			doc := parseSVG(t, out)
			if doc.width <= 0 || doc.height <= 0 {
				t.Fatalf("size %vx%v, want a positive px size", doc.width, doc.height)
			}
			// The root carries its natural size: the viewBox's, rounded up.
			if w, h := doc.viewBox[2], doc.viewBox[3]; doc.width < w || doc.width > w+0.01 || doc.height < h || doc.height > h+0.01 {
				t.Errorf("size %vx%v, want the viewBox's %vx%v", doc.width, doc.height, w, h)
			}
			if doc.foreign != 0 {
				t.Errorf("%d <foreignObject>s; usvg cannot paint them", doc.foreign)
			}
			all := strings.Join(doc.texts, "\n")
			for _, w := range tc.want {
				if !strings.Contains(all, w) {
					t.Errorf("no <text> with %q; texts: %q", w, doc.texts)
				}
			}
		})
	}
}

// TestRenderKeepsTextOnCanvas pins the wazero fix (wazero#2535): with an
// older compiler the previous renderer's wrapped labels landed ±8,000,000px
// off the canvas. Any float miscompile in layout shows the same way.
func TestRenderKeepsTextOnCanvas(t *testing.T) {
	doc := render(t, "flowchart TD\n  A[a label long enough that it has to wrap onto a second and a third line] --> B[short]\n")
	if len(doc.textYs) < 4 {
		t.Fatalf("%d text lines, want the long label wrapped", len(doc.textYs))
	}
	top, bottom := doc.viewBox[1], doc.viewBox[1]+doc.viewBox[3]
	for _, y := range doc.textYs {
		if y < top || y > bottom {
			t.Errorf("text at y=%v, outside the viewBox's %v..%v", y, top, bottom)
		}
	}
}

// TestRenderLargeFlowchart: a diagram far past what anyone writes by hand
// still comes out whole, its labels as <text>.
func TestRenderLargeFlowchart(t *testing.T) {
	var src strings.Builder
	src.WriteString("flowchart TD\n")
	for i := range 150 {
		fmt.Fprintf(&src, "  n%d[node number %d] --> n%d[node number %d]\n", i, i, (i*7+1)%150, (i*7+1)%150)
	}
	doc := render(t, src.String())
	if doc.foreign != 0 {
		t.Errorf("%d <foreignObject>s", doc.foreign)
	}
	if !strings.Contains(strings.Join(doc.texts, "\n"), "node number 149") {
		t.Error(`no <text> with "node number 149"`)
	}
}

func TestRenderUnknownKind(t *testing.T) {
	_, err := Render(context.Background(), "zenuml\n  title Checkout\n  A.pay()\n")
	if err == nil || !strings.Contains(err.Error(), "No diagram type detected") {
		t.Fatalf("err = %v, want one saying no kind was recognised", err)
	}
}

func TestRenderParseErrorOnOneLine(t *testing.T) {
	_, err := Render(context.Background(), "sequenceDiagram\n  A->>B: hi\n  loop retry\n")
	if err == nil {
		t.Fatal("an unclosed loop rendered")
	}
	if msg := err.Error(); strings.Contains(msg, "\n") || !strings.Contains(msg, "line") || strings.Contains(msg, "^") {
		t.Errorf("err = %q, want one line naming the line, no caret", msg)
	}
}

func TestRenderRejects(t *testing.T) {
	for name, src := range map[string]string{
		"not mermaid":  "this is not mermaid at all {{{",
		"broken label": "flowchart TD\n  A[Start] --> B[Done",
		"empty":        " \n\t",
	} {
		t.Run(name, func(t *testing.T) {
			out, err := Render(context.Background(), src)
			if err == nil {
				t.Fatalf("Render(%q) = %d bytes, want an error", src, len(out))
			}
			if !strings.HasPrefix(err.Error(), "mermaid") {
				t.Errorf("error %q does not say where it came from", err)
			}
			t.Logf("%s: %v", name, err)
		})
	}
}

func TestRenderCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := Render(ctx, flowchart); !errors.Is(err, context.Canceled) {
		t.Fatalf("Render with a cancelled context: err = %v, want context.Canceled", err)
	}
}

func TestRenderDeadlineStopsTheGuest(t *testing.T) {
	render(t, "flowchart LR\n  A --> B\n") // compile first: the deadline is for the render
	var big strings.Builder
	big.WriteString("flowchart TD\n")
	for i := range 1000 { // ~200ms to draw, under Ondine's 50,000-character limit
		fmt.Fprintf(&big, "  n%d[node %d] --> n%d[node %d]\n", i, i, (i*7+1)%1000, (i*7+1)%1000)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	start := time.Now()
	_, err := Render(ctx, big.String())
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("err = %v, want context.DeadlineExceeded", err)
	}
	if took := time.Since(start); took > 2*time.Second {
		t.Errorf("Render returned %v after its deadline; the guest was not stopped", took)
	}
}

func TestRenderConcurrent(t *testing.T) {
	srcs := []string{flowchart, sequence, state, class, er, quadrant, xychart, sankey}
	var wg sync.WaitGroup
	errs := make(chan error, 2*len(srcs))
	for i := range 2 * len(srcs) {
		wg.Go(func() {
			out, err := Render(context.Background(), srcs[i%len(srcs)])
			if err == nil && !strings.Contains(string(out), "<svg") {
				err = fmt.Errorf("output is not an SVG: %.80q", out)
			}
			errs <- err
		})
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Error(err)
		}
	}
}

func BenchmarkRender(b *testing.B) {
	if _, err := Render(context.Background(), flowchart); err != nil {
		b.Fatal(err)
	}
	for b.Loop() {
		if _, err := Render(context.Background(), flowchart); err != nil {
			b.Fatal(err)
		}
	}
}

// TestMain gives the tests a compilation cache of their own: a test binary
// does not record its wazero version (see defaultCacheDir), so the shared
// one is off limits.
func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "mermaidsvg-cache-")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	cacheDir = func() string { return dir }
	code := m.Run()
	_ = os.RemoveAll(dir)
	os.Exit(code)
}

func TestDefaultCacheDirNeedsAWazeroVersion(t *testing.T) {
	// A test binary records no dependency versions, which is exactly the
	// case where sharing the user's cache is unsafe.
	if v := wazeroVersion(); v != "" {
		t.Skipf("this binary records wazero %s", v)
	}
	if dir := defaultCacheDir(); dir != "" {
		t.Errorf("defaultCacheDir() = %q without a wazero version, want no cache", dir)
	}
}
