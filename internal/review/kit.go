package review

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/neuroplastio/hotty-a2ui/a2ui"
	"github.com/neuroplastio/hotty-a2ui/catalog/basic"
	hottycat "github.com/neuroplastio/hotty-a2ui/catalog/hotty"
	"github.com/neuroplastio/hotty-a2ui/rendition/cells"
	"github.com/neuroplastio/hotty-a2ui/rendition/html"
	"github.com/neuroplastio/hotty-a2ui/rendition/theme"
	"github.com/neuroplastio/hotty-a2ui/view"
	"github.com/neuroplastio/hotty-go/hottytea"
)

// margin's panes (D22): every part of the review screen besides the
// document column is an A2UI surface that hotty-a2ui renders — the
// composer, the open thread, the palette, the footer, the tree pane and
// the threads view. margin is their agent: it defines each pane's
// components, writes its data model, and hears its actions. On a terminal
// that is not a HOTTY host a pane is the kit's cells rendition, drawn into
// margin's frame like any other lines; on a host, a pane margin gives a
// rectangle of its own is an HTML surface there (hotty.go).

// basicCatalog is A2UI's basic catalog, which the panes' components are
// from unless they name hotty's.
const basicCatalog = "https://a2ui.org/specification/v1_0/catalogs/basic/catalog.json"

// kitTheme is what the panes paint with: the terminal's own palette, the
// ANSI-16 roles margin's cells already use.
var kitTheme = theme.Default

// rootScope is the scope a pane's own components are found in.
var rootScope = a2ui.Scope{Path: "/"}

// kitEpoch is the panes' clock in cells. A frozen clock keeps a field's
// caret steady (the kit blinks it by the clock) and every frame
// deterministic: nothing margin shows animates.
var kitEpoch = time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

// kit is the A2UI side of a review: one processor, whose surfaces are the
// panes, by id.
type kit struct {
	p     *a2ui.Processor
	panes map[string]*pane
	// acts are the actions the panes sent, in order, until take: the
	// processor must not be called back from Send.
	acts []*a2ui.ActionMessage
}

func newKit() *kit {
	k := &kit{panes: map[string]*pane{}}
	h := hottycat.Catalog()
	hottycat.Implement(h, hottycat.Renderer{
		Focus: func(s *a2ui.Surface, scope a2ui.Scope, id string) error {
			p := k.of(s)
			if p == nil {
				return fmt.Errorf("no pane for surface %q", s.ID)
			}
			p.focus(p.c.FindComponent(id, scope))
			return nil
		},
		Blur: func(s *a2ui.Surface) error {
			if p := k.of(s); p != nil {
				p.c.Focus("")
			}
			return nil
		},
		ScrollTo: func(s *a2ui.Surface, scope a2ui.Scope, id, to string) error {
			if p := k.of(s); p != nil {
				p.c.ScrollTo(p.c.FindComponent(id, scope), to)
			}
			return nil
		},
		FoldAll:      func(*a2ui.Surface, a2ui.Scope, string, bool) error { return nil },
		Toast:        func(*a2ui.Surface, map[string]any) error { return nil },
		DismissToast: func(*a2ui.Surface, string) error { return nil },
		Timer:        func(*a2ui.Surface, a2ui.Scope, string, string) error { return nil },
	})
	k.p = a2ui.NewProcessor(basic.Catalog(), h)
	k.p.Send = func(o a2ui.Outbound) {
		if o.Action != nil {
			k.acts = append(k.acts, o.Action)
		}
	}
	k.p.Changed = func(s *a2ui.Surface, deleted bool) {
		p := k.panes[s.ID]
		switch {
		case p == nil:
		case deleted:
			delete(k.panes, s.ID)
		case p.c == nil:
			p.s, p.c = s, view.NewController(s)
			p.cr = cells.New(p.c)
			p.cr.Clock = func() time.Time { return kitEpoch }
		default:
			p.c.Rebuild()
		}
		if p != nil {
			p.dirty = true
		}
	}
	return k
}

// of is the pane a surface is.
func (k *kit) of(s *a2ui.Surface) *pane {
	if s == nil {
		return nil
	}
	return k.panes[s.ID]
}

// take returns the actions the panes sent since the last take.
func (k *kit) take() []*a2ui.ActionMessage {
	acts := k.acts
	k.acts = nil
	return acts
}

// feed hands the processor messages margin wrote as the panes' agent.
// They are margin's own, so a failure is a bug: it panics.
func (k *kit) feed(msgs ...map[string]any) {
	for _, m := range msgs {
		m["version"] = "v1.0"
		b, err := json.Marshal(m)
		if err != nil {
			panic(err)
		}
		if err := k.p.ProcessJSON(b); err != nil {
			panic(fmt.Sprintf("kit: %v: %s", err, b))
		}
	}
}

// pane is one A2UI surface of margin's: the controller the user's acts go
// through, its cells rendition, and on a host its HTML one.
type pane struct {
	k  *kit
	id string
	s  *a2ui.Surface
	c  *view.Controller
	cr *cells.Rendition
	hr *html.Rendition
	// bar: the pane is rows of the screen, as the footer and the search
	// prompt are; on a host it is a dense surface, its text and one-line
	// fields a row each, as in cells (SetDense).
	bar bool
	// components is what the pane was last defined with, as JSON: define
	// sends them again only when they changed.
	components string
	// The frame drawn last, at width w, until something changes it.
	frame *cells.Frame
	w     int
	dirty bool
}

// pane makes the pane id, with its first data model, or returns the one
// already made: drop it first for a fresh one.
func (k *kit) pane(id string, data map[string]any) *pane {
	if p := k.panes[id]; p != nil {
		return p
	}
	if data == nil {
		data = map[string]any{}
	}
	p := &pane{k: k, id: id, dirty: true}
	k.panes[id] = p
	k.feed(map[string]any{"createSurface": map[string]any{
		"surfaceId": id, "catalogId": basicCatalog, "dataModel": data}})
	return p
}

// drop deletes the pane.
func (k *kit) drop(id string) {
	if k.panes[id] == nil {
		return
	}
	k.feed(map[string]any{"deleteSurface": map[string]any{"surfaceId": id}})
	delete(k.panes, id)
}

// define gives the pane its components (the root's id is "root"), when
// they are not what it has.
func (p *pane) define(components ...map[string]any) {
	b, err := json.Marshal(components)
	if err != nil {
		panic(err)
	}
	if string(b) == p.components {
		return
	}
	p.components = string(b)
	list := make([]any, len(components))
	for i, c := range components {
		list[i] = c
	}
	p.k.feed(map[string]any{"updateComponents": map[string]any{"surfaceId": p.id, "components": list}})
}

// set writes a value to the pane's data model.
func (p *pane) set(path string, v any) {
	if err := p.s.Write(path, v); err != nil {
		panic(fmt.Sprintf("kit: %s %s: %v", p.id, path, err))
	}
	p.c.Rebuild()
	p.dirty = true
}

// put writes a value to the pane's data model when it is not what the
// model holds: a pane margin syncs every frame rebuilds only on a change.
func (p *pane) put(path string, v any) {
	have, _ := json.Marshal(p.s.Data.Value(path))
	want, _ := json.Marshal(v)
	if string(have) != string(want) {
		p.set(path, v)
	}
}

// text is a string in the pane's data model, "" when it holds none.
func (p *pane) text(path string) string {
	s, _ := p.s.Data.Value(path).(string)
	return s
}

// focus gives the pane's element id the keyboard.
func (p *pane) focus(id string) {
	p.c.Focus(id)
	p.dirty = true
}

// draw is the pane laid out w columns wide in cells.
func (p *pane) draw(w int) *cells.Frame {
	if p.frame == nil || p.dirty || p.w != w {
		p.frame, p.w, p.dirty = p.cr.Draw(w), w, false
	}
	return p.frame
}

// lines is the pane drawn w wide, a line a row, styled.
func (p *pane) lines(w int) []string {
	return strings.Split(p.draw(w).Themed(kitTheme), "\n")
}

// key gives the pane a key in cells, named as hottytea names it, and
// reports whether the pane took it.
func (p *pane) key(name string) bool {
	ok, err := p.cr.Key(name)
	if err != nil {
		panic(fmt.Sprintf("kit: %s key %q: %v", p.id, name, err))
	}
	p.dirty = p.dirty || ok
	return ok
}

// click is a press at col, row of the pane's frame.
func (p *pane) click(col, row int) {
	if err := p.cr.Click(col, row); err != nil {
		panic(fmt.Sprintf("kit: %s click: %v", p.id, err))
	}
	p.dirty = true
}

// paste gives the pane pasted text.
func (p *pane) paste(text string) {
	if _, err := p.cr.Paste(text); err != nil {
		panic(fmt.Sprintf("kit: %s paste: %v", p.id, err))
	}
	p.dirty = true
}

// cursor is where in the pane's frame the terminal's cursor goes, if any
// element of it has the keyboard.
func (p *pane) cursor(w int) (col, row int, ok bool) {
	return p.draw(w).Cursor()
}

// box is an element's rectangle in the pane's frame, by its component id.
func (p *pane) box(component string) (col, row, w, h int, ok bool) {
	id := p.c.FindComponent(component, rootScope)
	if id == "" {
		return 0, 0, 0, 0, false
	}
	return p.cr.Box(id)
}

// html is the pane's HTML rendition, for a host: made once, named name
// there.
func (p *pane) html(name string) *html.Rendition {
	if p.hr == nil {
		p.hr = html.New(p.c, name)
		p.hr.SetTheme(kitTheme)
		p.hr.SetDense(p.bar)
	}
	return p.hr
}

// surface is the pane as a host surface at r, its document the HTML
// rendition's.
func (p *pane) surface(name string, r hottytea.Rect) hottytea.Surface {
	h := p.html(name)
	return hottytea.Surface{Name: name, Rect: r, Keep: true, Doc: h.Doc}
}

// obj is a JSON object from key, value pairs.
func obj(kv ...any) map[string]any {
	m := make(map[string]any, len(kv)/2)
	for i := 0; i+1 < len(kv); i += 2 {
		m[kv[i].(string)] = kv[i+1]
	}
	return m
}

// bind is a binding to a path of the data model.
func bind(path string) map[string]any { return map[string]any{"@path": path} }
