// Package mermaidsvg renders mermaid source to SVG inside the margin process,
// with no Node, no browser and no external binary.
//
// The renderer is merman (github.com/Latias94/merman, MIT OR Apache-2.0), a
// Rust port of Mermaid 12, compiled to a WASI command (wasm/, rebuilt by
// `make mermaid-wasm`) and embedded gzipped as mermaid.wasm.gz. It draws
// flowchart, sequence, state, ER, class, gantt, pie, mindmap, gitGraph,
// journey and timeline diagrams; any other kind is an error. wazero, a pure
// Go WebAssembly runtime, compiles it once per process and runs one fresh
// instance per diagram: source on stdin, SVG on stdout, a message on stderr
// and a non-zero exit on failure.
//
// The SVG is drawn for margin's dark palette on a transparent background,
// with every label as plain <text> (no <foreignObject>, which usvg cannot
// paint), text measured as Inter paints it, and a root that carries its
// natural size in px (width and height equal to the viewBox's).
//
// wazero must stay at or after commit 424d3ca (wazero#2535): before it, the
// amd64 optimizing compiler miscompiled an f64 select in merman's CSS
// cascade, and every multi-line label was placed millions of pixels off the
// canvas. TestRenderKeepsTextOnCanvas guards it.
package mermaidsvg

import (
	"bytes"
	"compress/gzip"
	"context"
	_ "embed"
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
	"github.com/tetratelabs/wazero/imports/wasi_snapshot_preview1"
	"github.com/tetratelabs/wazero/sys"
)

//go:embed mermaid.wasm.gz
var wasmGz []byte

const (
	// memoryLimitPages bounds one render's guest memory: 2048 pages of
	// 64KiB, 128MiB. A typical diagram peaks at 4MiB and a 400-node
	// flowchart at 7MiB.
	memoryLimitPages = 2048
	// renderTimeout bounds one render when the caller's context does not
	// bound it sooner. Typical diagrams take tens of milliseconds.
	renderTimeout = 10 * time.Second
	// maxSource is the largest source accepted, well past any diagram a
	// person writes by hand.
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

// compile decompresses and compiles the embedded module: two to three
// seconds. The compilation cache under the user's cache dir (about 35MB of
// native code) brings that to under 200ms for every later process; without
// a usable dir it compiles uncached each time.
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
	if _, err := wasi_snapshot_preview1.Instantiate(ctx, rt); err != nil {
		return fail(fmt.Errorf("mermaid renderer: wasi: %w", err))
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

// Render renders one mermaid diagram (the body of a ```mermaid fence, no
// fence lines) to a standalone SVG document.
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

	var stdout, stderr bytes.Buffer
	cfg := wazero.NewModuleConfig().
		WithName(""). // anonymous, so instances can run side by side
		WithArgs("margin-mermaid").
		WithStdin(strings.NewReader(src)).
		WithStdout(&stdout).
		WithStderr(&stderr)
	mod, runErr := engine.rt.InstantiateModule(ctx, engine.mod, cfg)
	if mod != nil {
		_ = mod.Close(context.Background())
	}

	if runErr != nil {
		var exit *sys.ExitError
		switch {
		case errors.As(runErr, &exit) && exit.ExitCode() == 0:
			// a clean proc_exit(0)
		case ctx.Err() != nil:
			return nil, fmt.Errorf("mermaid: %w", ctx.Err())
		case errors.As(runErr, &exit):
			return nil, guestError(stderr.String(), fmt.Sprintf("exit code %d", exit.ExitCode()))
		default:
			return nil, guestError(stderr.String(), runErr.Error())
		}
	}
	if stdout.Len() == 0 {
		return nil, guestError(stderr.String(), "no output")
	}
	return stdout.Bytes(), nil
}

// guestError turns what the renderer wrote to stderr into an error. A Rust
// panic (a renderer bug, not bad input) is labelled as one.
func guestError(stderr, fallback string) error {
	var lines []string
	for line := range strings.SplitSeq(strings.TrimSpace(stderr), "\n") {
		if line = strings.TrimSpace(line); line != "" && !strings.HasPrefix(line, "note: run with") {
			lines = append(lines, line)
		}
	}
	msg := strings.Join(lines, ": ")
	switch {
	case msg == "":
		return fmt.Errorf("mermaid renderer failed: %s", fallback)
	case strings.Contains(msg, "panicked at"):
		return fmt.Errorf("mermaid renderer crashed: %s", msg)
	default:
		return fmt.Errorf("mermaid: %s", msg)
	}
}
