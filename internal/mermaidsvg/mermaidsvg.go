// Package mermaidsvg renders mermaid source to SVG inside the margin process,
// with no Node, no browser and no external binary.
//
// The renderer is Ondine (github.com/wevm/ondine, MIT), a Mermaid compiler in
// Rust with no dependencies, built for wasm32-unknown-unknown at a pinned
// commit (wasm/README.md, `make mermaid-wasm`) and embedded gzipped as
// mermaid.wasm.gz. It draws all 33 kinds it knows, flowchart to wardley; a
// source it cannot place is an error. The module imports nothing: the host
// copies the source and options in through its alloc export, calls render,
// and reads one length-prefixed result buffer out of its memory. wazero, a
// pure Go WebAssembly runtime, compiles it once per process and runs one
// fresh instance per diagram.
//
// Ondine themes a diagram with CSS custom properties and color-mix(), which
// usvg (what hotty-blitz paints an SVG <img> with) has neither of: as it
// comes, a diagram paints as black boxes with no text. flatten writes
// margin's palette in their place (flatten.go). Labels are plain <text>,
// measured as a monospace font with a 0.6em advance, on a transparent root
// that carries its natural size in px (width and height equal to the
// viewBox's).
//
// wazero stays at or after commit 424d3ca (wazero#2535): before it, the amd64
// optimizing compiler miscompiled an f64 select, which put the previous
// renderer's wrapped labels millions of pixels off the canvas.
// TestRenderKeepsTextOnCanvas guards it.
package mermaidsvg

import (
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	_ "embed"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime/debug"
	"strings"
	"sync"
	"time"

	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/api"
)

//go:embed mermaid.wasm.gz
var wasmGz []byte

const (
	// memoryLimitPages bounds one render's guest memory: 2048 pages of
	// 64KiB, 128MiB. A typical diagram needs a few hundred KiB.
	memoryLimitPages = 2048
	// renderTimeout bounds one render when the caller's context does not
	// bound it sooner. Typical diagrams take a millisecond or two.
	renderTimeout = 10 * time.Second
	// maxSource is the largest source copied into the renderer. Ondine
	// itself refuses one past 50,000 characters, mermaid's maxTextSize.
	maxSource = 1 << 20
)

// compiled is the process-wide runtime and compiled module, made once.
type compiled struct {
	rt  wazero.Runtime
	mod wazero.CompiledModule
	err error
}

var (
	startOnce sync.Once
	ready     = make(chan struct{})
	engine    compiled
)

// Warm starts compiling the renderer in the background and returns at once,
// so the first Render does not pay for it. Calling it more than once, or not
// at all, is fine: Render compiles on first use either way.
func Warm() { start() }

func start() {
	startOnce.Do(func() {
		go func() {
			defer close(ready)
			defer func() {
				if r := recover(); r != nil {
					engine = compiled{err: fmt.Errorf("mermaid renderer: compile: panic: %v", r)}
				}
			}()
			engine = compile()
		}()
	})
}

// compile decompresses and compiles the embedded module: about a second.
// The compilation cache under the user's cache dir brings that to a few tens
// of milliseconds for every later process; without a usable dir it compiles
// uncached each time.
func compile() compiled {
	ctx := context.Background()
	cfg := wazero.NewRuntimeConfig().
		WithCloseOnContextDone(true).
		WithMemoryLimitPages(memoryLimitPages)
	if cache := compilationCache(); cache != nil {
		cfg = cfg.WithCompilationCache(cache)
	}
	rt := wazero.NewRuntimeWithConfig(ctx, cfg)
	fail := func(err error) compiled {
		_ = rt.Close(ctx)
		return compiled{err: err}
	}
	zr, err := gzip.NewReader(bytes.NewReader(wasmGz))
	if err != nil {
		return fail(fmt.Errorf("mermaid renderer: embedded module: %w", err))
	}
	wasm, err := io.ReadAll(zr)
	if err != nil {
		return fail(fmt.Errorf("mermaid renderer: embedded module: %w", err))
	}
	mod, err := rt.CompileModule(ctx, wasm)
	if err != nil {
		return fail(fmt.Errorf("mermaid renderer: compile: %w", err))
	}
	for _, name := range []string{"alloc", "render"} {
		if _, ok := mod.ExportedFunctions()[name]; !ok {
			return fail(fmt.Errorf("mermaid renderer: the module has no %s export", name))
		}
	}
	return compiled{rt: rt, mod: mod}
}

// cacheDir is where compiled code is kept between runs, "" for no cache.
// Tests point it at a fresh temporary directory.
var cacheDir = defaultCacheDir

// defaultCacheDir is under the user's cache dir, and only when this binary
// records which wazero it was built with. wazero keys its cache by its own
// version, read from the same build info; without it (test binaries, a
// replaced module) every version shares one "dev" key, and loading code that
// another wazero compiled crashes the process outright.
func defaultCacheDir() string {
	if wazeroVersion() == "" {
		return ""
	}
	dir, err := os.UserCacheDir()
	if err != nil {
		return ""
	}
	return filepath.Join(dir, "margin", "wazero")
}

// wazeroVersion is the wazero module version this binary was built with, or
// "" when the build info does not say.
func wazeroVersion() string {
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return ""
	}
	for _, dep := range info.Deps {
		if dep.Path == "github.com/tetratelabs/wazero" && dep.Replace == nil {
			if v := dep.Version; v != "" && v != "(devel)" {
				return v
			}
		}
	}
	return ""
}

func compilationCache() wazero.CompilationCache {
	dir := cacheDir()
	if dir == "" {
		return nil
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil
	}
	cache, err := wazero.NewCompilationCacheWithDir(dir)
	if err != nil {
		return nil
	}
	return cache
}

// options are Ondine's render options, as the JSON object its render export
// reads. Font size, padding and the rest stay Ondine's defaults.
type options struct {
	// IDPrefix starts every id in the SVG, so no two diagrams share one.
	IDPrefix string `json:"idPrefix"`
	// Responsive off keeps the root at its natural size, with no
	// max-width:100% of its own: margin's stylesheet sizes the <img>.
	Responsive bool `json:"responsive"`
	// Links and Images off: an <img> follows no link, and fetches nothing.
	Links  bool `json:"links"`
	Images bool `json:"images"`
}

// Render renders one mermaid diagram (the body of a ```mermaid fence, no
// fence lines) to a standalone SVG document, in margin's palette.
//
// It is safe to call from any number of goroutines. The first call waits for
// the renderer to compile (see Warm); every call runs in its own instance,
// bounded to 128MiB of guest memory and to ctx or ten seconds, whichever ends
// first. Source mermaid cannot parse is an error carrying the parser's
// message; so is anything that goes wrong inside the renderer, which never
// takes the process down with it.
func Render(ctx context.Context, src string) (svg []byte, err error) {
	if strings.TrimSpace(src) == "" {
		return nil, errors.New("mermaid: empty diagram")
	}
	if len(src) > maxSource {
		return nil, fmt.Errorf("mermaid: diagram source is %d bytes, over the %d limit", len(src), maxSource)
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	start()
	select {
	case <-ready:
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	if engine.err != nil {
		return nil, engine.err
	}

	ctx, cancel := context.WithTimeout(ctx, renderTimeout)
	defer cancel()
	// wazero reports guest traps as errors; this only guards against a bug
	// on the host side of the boundary turning into a crash of margin.
	defer func() {
		if r := recover(); r != nil {
			svg, err = nil, fmt.Errorf("mermaid renderer: panic: %v", r)
		}
	}()

	sum := sha256.Sum256([]byte(src))
	opts, err := json.Marshal(options{IDPrefix: "m" + hex.EncodeToString(sum[:4]) + "-"})
	if err != nil {
		return nil, fmt.Errorf("mermaid renderer: options: %w", err)
	}
	mod, err := engine.rt.InstantiateModule(ctx, engine.mod, wazero.NewModuleConfig().WithName("")) // anonymous, so instances can run side by side
	if err != nil {
		return nil, guestError(ctx, err)
	}
	defer mod.Close(context.Background())
	out, err := run(ctx, mod, src, string(opts))
	if err != nil {
		return nil, err
	}
	return flatten(out), nil
}

// run hands src and opts to an instance's render export and decodes what it
// returns: the SVG, or the renderer's account of why there is none.
func run(ctx context.Context, mod api.Module, src, opts string) ([]byte, error) {
	put := func(s string) (uint64, error) {
		res, err := mod.ExportedFunction("alloc").Call(ctx, uint64(len(s)))
		if err != nil {
			return 0, guestError(ctx, err)
		}
		if res[0] == 0 || !mod.Memory().WriteString(uint32(res[0]), s) {
			return 0, errors.New("mermaid renderer: out of memory")
		}
		return res[0], nil
	}
	srcAt, err := put(src)
	if err != nil {
		return nil, err
	}
	optsAt, err := put(opts)
	if err != nil {
		return nil, err
	}
	// A gantt chart's "today" is the host's: the module reads no clock.
	now := api.EncodeF64(float64(time.Now().UnixMilli()))
	res, err := mod.ExportedFunction("render").Call(ctx, srcAt, uint64(len(src)), optsAt, uint64(len(opts)), now)
	if err != nil {
		return nil, guestError(ctx, err)
	}
	return decode(mod.Memory(), uint32(res[0]))
}

// decode reads render's result at at: a status byte, then fields of a
// little-endian u32 length and that many bytes. Success is the kind, width,
// height, links (a count, then href, id and internal for each) and the SVG;
// failure is the code, kind, line, column, snippet and message.
func decode(mem api.Memory, at uint32) ([]byte, error) {
	broken := errors.New("mermaid renderer: unreadable result")
	status, ok := mem.ReadByte(at)
	if !ok {
		return nil, broken
	}
	at++
	field := func() []byte {
		n, ok := mem.ReadUint32Le(at)
		if !ok {
			return nil
		}
		b, ok := mem.Read(at+4, n)
		if !ok {
			return nil
		}
		at += 4 + n
		return b
	}
	u32 := func(b []byte) uint32 {
		if len(b) != 4 {
			return 0
		}
		return binary.LittleEndian.Uint32(b)
	}

	if status != 0 {
		_, _, line := field(), field(), u32(field())
		_, snippet, msg := field(), field(), field()
		text := oneLine(string(msg), string(snippet))
		switch {
		case text == "":
			return nil, errors.New("mermaid: the renderer failed")
		case line > 0 && !strings.Contains(text, "line"):
			return nil, fmt.Errorf("mermaid: line %d: %s", line, text)
		default:
			return nil, fmt.Errorf("mermaid: %s", text)
		}
	}

	field() // kind
	field() // width, an f64: the root's width attribute says the same
	field() // height
	for range u32(field()) {
		field()
		field()
		field()
	}
	svg := field()
	if !bytes.HasPrefix(svg, []byte("<svg")) {
		return nil, broken
	}
	return bytes.Clone(svg), nil
}

// oneLine is a parser's message on one line: mermaid's spans several, with
// the source line it stopped in and a caret under the place, which say no
// more than "on line N" does.
func oneLine(msg, snippet string) string {
	var kept []string
	for line := range strings.SplitSeq(msg, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || line == strings.TrimSpace(snippet) || strings.Trim(line, "-^ ") == "" {
			continue
		}
		kept = append(kept, line)
	}
	return strings.Join(kept, " ")
}

// guestError says why a call into the renderer stopped: the caller's context
// ending, or a trap (a Rust panic is one: the module aborts on panic).
func guestError(ctx context.Context, err error) error {
	if ctx.Err() != nil {
		return fmt.Errorf("mermaid: %w", ctx.Err())
	}
	return fmt.Errorf("mermaid renderer crashed: %v", err)
}
