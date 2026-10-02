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

// svgDoc is what the tests read back from a rendered SVG.
type svgDoc struct {
	width, height float64
	viewBox       [4]float64
	texts         []string
	textYs        []float64
	foreign       int
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
			switch el.Name.Local {
			case "foreignObject":
				doc.foreign++
			case "text":
				if inText == 0 {
					text.Reset()
				}
				inText++
				if y := attr("y"); y != "" {
					doc.textYs = append(doc.textYs, number(t, "text y", y))
				}
			}
		case xml.EndElement:
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
	} {
		t.Run(tc.name, func(t *testing.T) {
			doc := render(t, tc.src)
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
// older compiler a wrapped label's lines landed ±8,000,000px off the canvas.
func TestRenderKeepsTextOnCanvas(t *testing.T) {
	doc := render(t, "flowchart TD\n  A[a label long enough that it has to wrap onto a second and a third line] --> B[short]\n")
	if len(doc.textYs) < 3 {
		t.Fatalf("%d text lines, want the long label wrapped", len(doc.textYs))
	}
	top, bottom := doc.viewBox[1], doc.viewBox[1]+doc.viewBox[3]
	for _, y := range doc.textYs {
		if y < top || y > bottom {
			t.Errorf("text at y=%v, outside the viewBox's %v..%v", y, top, bottom)
		}
	}
}

// TestRenderLargeFlowchart: past about 90 nodes merman cannot convert HTML
// labels within its work budget, so the renderer draws large diagrams with
// SVG labels instead (wasm/src/main.rs).
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

func TestRenderUnsupportedKind(t *testing.T) {
	_, err := Render(context.Background(), "quadrantChart\n  title Effort vs value\n  A: [0.3, 0.6]\n")
	if err == nil || !strings.Contains(err.Error(), "quadrantChart") {
		t.Fatalf("err = %v, want one naming the unsupported kind", err)
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
	for i := range 400 {
		fmt.Fprintf(&big, "  n%d[node %d] --> n%d[node %d]\n", i, i, (i*7+1)%400, (i*7+1)%400)
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
	srcs := []string{flowchart, sequence, state, class, er}
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
