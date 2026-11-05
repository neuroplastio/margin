package review

import (
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/neuroplastio/hotty-go"
	"github.com/neuroplastio/hotty-go/hottytea"
	"github.com/neuroplastio/hotty-surf/pkg/cells"
	"github.com/neuroplastio/hotty-surf/surfrun"
	"github.com/neuroplastio/hotty-surf/surfui"
)

// margin's hotty-surf seam: a hotty-surf model shown as one of margin's
// surfaces. margin places its surfaces itself, with hottytea's Session, so a
// surf model is hosted the way a multiplexer hosts its panes
// (surfrun.Surface, "Hosting without surfrun.Runtime" in hotty-surf's
// guide): the Surface routes the host's events and the keys to the model and
// says what to send, and margin writes that through the Session and places
// the surface where its frame puts it.
//
// A model that posts from a goroutine of its own needs a Runner. The panes
// margin moves here post nothing, so the Surface has none. A Select's open
// list is a surface of its own (surfrun.List) that none of them uses yet,
// so a frame's List is dropped here.

// surfHost is a surf model shown as a surface of margin's. It is the
// model's surfrun.Runner.
type surfHost struct {
	name string
	m    surfrun.Model
	s    *surfrun.Surface
	// send puts a message on the program's queue (tea.Program.Send).
	send func(tea.Msg)
	// pending is the frame Show returned, which goes after the Layout that
	// placed the document (hotty-surf's guide): Layout calls the Doc func
	// exactly when Show is due.
	pending surfrun.Frame
	// r draws the model in cells, made when a terminal needs them.
	r *cells.Renderer
	// caret is where the model's focused field has the caret in cells, as of
	// the last lines.
	caret cells.Caret
}

// surfPostMsg is a function a model posted: Update runs it, then draws.
type surfPostMsg struct {
	p *surfHost
	f func()
}

// newSurfHost hosts m as the surface name. keys are margin's own keys, by
// myelin's names, which a surf text field gives back to margin. A surf
// binding to Quit never ends margin.
func newSurfHost(name string, m surfrun.Model, send func(tea.Msg), keys []string) *surfHost {
	p := &surfHost{name: name, m: m, send: send}
	p.s = surfrun.NewSurface(m, p)
	p.s.NoQuit = true
	p.s.Program = keys
	return p
}

// Post, Quit and Focus are surfrun.Runner. Post runs f on the program's
// goroutine, from Update, which then draws what changed; Quit is margin's
// own, so it does nothing; Focus is the Surface's.
func (p *surfHost) Post(f func()) {
	if p.send != nil {
		p.send(surfPostMsg{p, f})
	}
}
func (p *surfHost) Quit()            {}
func (p *surfHost) Focus(key string) { p.s.Focus(key) }

// posted runs f and sends what changed.
func (p *surfHost) posted(h *hotState, f func()) {
	f()
	h.s.Send(p.frame(p.s.Update())...)
}

// surfSheet sends surf's stylesheet, once a session: every surface's
// document links it (surfui.Document).
func (h *hotState) surfSheet() {
	if h.surfCSS {
		return
	}
	h.surfCSS = true
	h.s.Send(hotty.Res(surfui.StyleID, "text/css", []byte(surfui.CSS)))
}

// surface is the host as a hottytea surface at r, shown only inside clip.
// Its Doc is the model's document as Show renders it, which the Session
// asks for only when the host has none (the first time, or after it lost
// it).
func (p *surfHost) surface(h *hotState, r, clip hottytea.Rect) hottytea.Surface {
	h.surfSheet()
	if sz, ok := p.m.(surfrun.Sizer); ok {
		sz.Resize(r.W, r.H)
	}
	return hottytea.Surface{Name: p.name, Rect: r, Clip: &clip, Scroll: p.s.Scroll,
		Doc: func() string {
			p.pending = p.s.Show()
			return surfui.Document(p.s.Tree())
		}}
}

// sendFirst writes the frame Show returned, after the Layout that placed
// the document.
func (p *surfHost) sendFirst(h *hotState) {
	if cmds := p.frame(p.pending); len(cmds) > 0 {
		h.s.Send(cmds...)
	}
	p.pending = surfrun.Frame{}
}

// sync sends what the model's state changed since the last frame.
func (p *surfHost) sync(h *hotState) {
	f := p.s.Update()
	if cmds := p.frame(f); len(cmds) > 0 {
		h.s.Send(cmds...)
	}
}

// hide takes the surface off when margin stops wanting it: keys then reach
// only the bindings.
func (p *surfHost) hide() { p.s.Hide() }

// event hands the host's event to the model, and sends what changed.
func (p *surfHost) event(h *hotState, ev hotty.Event) bool {
	took, redraw := p.s.Event(ev)
	if redraw {
		h.s.Send(p.frame(p.s.Update())...)
	}
	return took
}

// key hands a key to the model, and sends what changed. It reports whether
// the model took it: a key it did not take is margin's.
func (p *surfHost) key(h *hotState, name string) bool {
	took, redraw := p.s.Key(name)
	if redraw {
		h.s.Send(p.frame(p.s.Update())...)
	}
	return took
}

// frame is f as hotty commands, in surfrun.Frame's order
// (surfrun.WriteFrame). A Select's list is not written: no pane uses one.
func (p *surfHost) frame(f surfrun.Frame) []string {
	var b strings.Builder
	surfrun.WriteFrame(&b, p.name, f, nil)
	if b.Len() == 0 {
		return nil
	}
	return []string{b.String()}
}

// ready tells the Surface what the host said about itself: how a binding's
// mod reads, and the system a key is labelled for (hottytea.ReadyMsg).
func (p *surfHost) ready(caps hotty.Caps) { p.s.Platform = hotty.PlatformOf(caps) }

// renderer is the cells renderer over the model's Surface, made on first
// use: where there is no host, the model is shown in cells.
func (p *surfHost) renderer() *cells.Renderer {
	if p.r == nil {
		p.r = cells.New(p.s.Cells())
		p.r.Show()
	}
	return p.r
}

// lines draws the model in cells, w by rows, as lines of the terminal's
// colours (surfrun.Paint).
func (p *surfHost) lines(w, rows int) []string {
	r := p.renderer()
	r.Update()
	scr := r.Draw(w, rows)
	p.caret = scr.Caret
	return surfrun.Paint(scr)
}

// cellsKey is a key for the model in cells (myelin's name, hottytea.KeyName),
// and cellsPaste a paste; each reports whether the model took it.
func (p *surfHost) cellsKey(name string) bool {
	took, _ := p.renderer().Key(name)
	return took
}

func (p *surfHost) cellsPaste(text string) bool {
	took, _ := p.renderer().Paste(text)
	return took
}
