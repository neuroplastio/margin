package review

import (
	"context"
	"io"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/neuroplastio/hotty-go"
	"github.com/neuroplastio/hotty-go/hottytea"

	"github.com/neuroplastio/margin/internal/mermaidsvg"
)

// margin on a HOTTY host (D17): where the terminal can show HTML, the prose
// blocks are surfaces, set in the reading font, and everything that is the
// review — the gutter's focus and marks, threads, the composer, the footer,
// the tree pane — stays in cells around them. Anywhere else margin is what it
// always was: Detect finds no host, and nothing here runs.
//
// A block's height is the host's to say: its document laid out at the
// column's width. Each block is measured once on a throwaway surface placed
// with r=auto (SPEC §5.2), and until its reply comes the block shows in cells
// (D17), so a host that is slow to answer still shows the whole document.
// A multiplexer with no terminal attached answers ENOTERM: those blocks wait
// in cells for one, which margin hears as a fit on a surface kept for it.
// A block can grow after it is measured, when an image decodes or a font
// loads: placed with fit (f=1), the host says so, and the block takes the
// rows it says from then on.
//
// Surfaces are placed from Update (hottytea.Session.Layout), which is where
// Bubble Tea lets a program send anything, so on a host the frame is drawn
// in Update too, and View hands it back.

// hotState is a review's HOTTY side. nil when HOTTY is off (--no-hotty, and
// every model a test builds unless it asks).
type hotState struct {
	s *hottytea.Session

	docs hotDocs

	// Measuring. rows is a block's rows at a width, as the host laid it out
	// (0: the host refused it, so cells); last is the rows a block took at
	// any width, which stand in after a resize until the new width's come.
	rows    map[hotKey]int
	last    map[string]int
	asked   map[hotKey]bool
	pending map[int]hotKey // by request number
	nextN   int
	wave    int  // the measuring wave the timeout belongs to
	heard   int  // measures answered so far, so a wave still answering waits
	deaf    bool // a host that left a wave unanswered: cells until the next input
	want    []hotWant
	// later is what the host did not measure in time (EBUDGET, or a wave
	// left unanswered), asked again on the next input; tries counts the
	// asks, so a document the host never lays out in time ends in cells.
	later []hotKey
	tries map[hotKey]int
	// attach is a block a multiplexer with no terminal attached answered
	// ENOTERM for (SPEC §5.2), placed again on a surface of its own that is
	// kept, with fit: its first fit says a terminal laid it out, and what
	// was not measured is asked again then. attachN is that placement's
	// request number, 0 until it is placed.
	attach  *hotKey
	attachN int

	// The frame: what View returns on a host, the blocks render placed in
	// it, and where the document's column is.
	frame  *tea.View
	blocks []hotBlock
	col    hottytea.Rect
	shown  bool // the document's column is on screen (not the inbox)
	// panes are the review's panes this frame gave a rectangle of their own
	// (D22): HTML surfaces there, their cells left blank.
	panes []hotPane

	// sent is each surface's document as last sent, so a change of review
	// state sends the document again; now is this frame's.
	sent map[string]string
	now  []hotSent

	relayoutDue bool
	anchor      *hotAnchor
	closed      bool

	// hoverFrom is the surface whose hover set the model's hoveredEntry, or
	// "" when motion over the cells did: a surface takes the pointer from
	// the cells (SPEC §9.3), so the gutter's bar follows its hover, and its
	// out clears the bar only if nothing has moved it since.
	hoverFrom string

	trace *log.Logger // MARGIN_HOTTY_LOG

	// mermaid draws a diagram as SVG: mermaidsvg.Render, but for tests.
	mermaid func(ctx context.Context, src string) ([]byte, error)
}

type hotKey struct {
	html string
	w    int
}

// hotWant is a block still to be measured, and roughly where it is.
type hotWant struct {
	key  hotKey
	line int
}

// hotBlock is a block render placed as a surface: its rows in the rendered
// lines, and what it shows.
type hotBlock struct {
	entry      int
	start      int
	rows       int
	base       string
	kind       string
	reviewed   bool
	selected   bool
	searchText []string // the block's cell rendering, which the search reads
}

// hotPane is a pane placed as a surface: where, and what shows it.
type hotPane struct {
	p    *pane
	rect hottytea.Rect
	clip hottytea.Rect // what of the screen the pane may show in
}

// hotPaneName is a pane's surface's name on the host.
func hotPaneName(p *pane) string { return "margin-" + p.id }

// place puts a pane at r for this frame, shown only inside clip, if
// surfaces are in use, and reports whether it did: then its cells are the
// host's.
func (h *hotState) place(p *pane, r, clip hottytea.Rect) bool {
	if !h.native() || h.deaf {
		return false
	}
	h.panes = append(h.panes, hotPane{p, r, clip})
	return true
}

// hotSent is a surface's document in this frame.
type hotSent struct{ name, doc string }

// hotAnchor keeps the top of the screen in place while measured blocks change
// height above it.
type hotAnchor struct{ entry, off int }

// hotRelayoutMsg draws the frame again once a wave of measures has come back.
type hotRelayoutMsg struct{}

// hotDiagramMsg is a mermaid diagram drawn, or why it could not be.
type hotDiagramMsg struct {
	src  string
	svg  []byte
	err  error
	took time.Duration
}

// hotMeasureTimeoutMsg gives up on a wave of measures the host did not
// answer: none of it since heard, the answers there were when it was set.
type hotMeasureTimeoutMsg struct{ wave, heard int }

const (
	hotMeasureName = "margin-measure"
	// hotAttachName is the surface that waits for a terminal to attach.
	hotAttachName = "margin-attach"
	// hotWave is how many blocks are measured at once, nearest the screen
	// first: enough to fill it, few enough that the first frame comes soon.
	hotWave = 64
	// hotMeasureWait is how long a wave may go without an answer.
	hotMeasureWait = 3 * time.Second
	// hotTries is how often a block is measured before a host that never
	// lays it out in time (EBUDGET each time, or no answer) gets it in cells.
	hotTries = 3
	// hotRelayoutAfter gathers the replies of a wave into one frame.
	hotRelayoutAfter = 16 * time.Millisecond
)

func newHotState() *hotState {
	h := &hotState{
		s:       hottytea.New(),
		rows:    map[hotKey]int{},
		last:    map[string]int{},
		asked:   map[hotKey]bool{},
		pending: map[int]hotKey{},
		tries:   map[hotKey]int{},
		sent:    map[string]string{},
		mermaid: mermaidsvg.Render,
	}
	h.docs.res = func(id, mime string, data []byte) { h.s.Send(hotty.Res(id, mime, data)) }
	if path := os.Getenv("MARGIN_HOTTY_LOG"); path != "" {
		if f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644); err == nil {
			h.trace = log.New(f, "", log.Lmicroseconds)
		}
	}
	return h
}

// tracef notes what margin and the host said to each other, when
// MARGIN_HOTTY_LOG names a file: the way to see why a host gets cells.
func (h *hotState) tracef(format string, args ...any) {
	if h.trace != nil {
		h.trace.Printf(format, args...)
	}
}

// native reports whether surfaces are in use: the terminal is a host, and
// margin is not on its way out.
func (h *hotState) native() bool {
	return h != nil && h.s.Mode == hottytea.Native && !h.closed
}

// hotOpen points the documents at the review's current file: its images are
// beside it.
func (m *model) hotOpen() {
	if m.hot == nil {
		return
	}
	m.hot.docs.open(filepath.Dir(m.path))
}

// Update is margin's Update with HOTTY around it: the Session sees every
// message first, and on a host every message that changes something draws
// the frame and lays the surfaces out.
func (m *model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if m.hot == nil {
		return m.update(msg)
	}
	msg, hcmd := m.hot.s.Update(msg)
	cmd, redraw := m.hotUpdate(msg)
	if m.quitting && !m.hot.closed {
		// Every surface goes with margin: the host deletes the alternate
		// screen's, but only SHOULD (SPEC §5.4).
		m.hot.frame = nil
		if m.hot.attachN != 0 {
			m.hot.s.Send(hotty.Del(hotAttachName)) // sent outside Layout
		}
		closing := m.hot.s.Close()
		m.hot.closed = true
		return m, tea.Batch(hcmd, tea.Sequence(closing, cmd))
	}
	if !redraw {
		return m, tea.Batch(hcmd, cmd)
	}
	if !m.hot.native() {
		m.hot.frame = nil
		return m, tea.Batch(hcmd, cmd)
	}
	return m, tea.Batch(hcmd, cmd, m.hotFrame())
}

// hotUpdate handles what the Session hands back, and gives the rest to
// update. redraw says the frame must be drawn again.
func (m *model) hotUpdate(msg tea.Msg) (cmd tea.Cmd, redraw bool) {
	h := m.hot
	if hotInput(msg) {
		h.again()
	}
	switch msg := msg.(type) {
	case nil:
		return nil, false
	case hottytea.ReadyMsg:
		h.tracef("ready: mode %v, caps %+v", msg.Mode, msg.Caps)
		if msg.Mode == hottytea.Native {
			h.docs.net = msg.Caps.Net["img-src"]
			h.s.Send(hotty.Res(hotCSSID, "text/css", []byte(hotCSS)))
		}
		return nil, true
	case hottytea.AckMsg:
		h.tracef("ack: %+v", msg.Reply)
		if h.measured(msg.Reply) {
			return h.relayoutSoon(), false
		}
		return nil, false
	case hottytea.ErrorMsg:
		h.tracef("error: %+v", msg.Reply)
		if h.measured(msg.Reply) {
			return h.relayoutSoon(), false
		}
		m.status = "hotty: " + hotty.Err(msg.Reply).Error()
		return nil, true
	case hottytea.EventMsg:
		if msg.Event.Kind == hotty.EventFit {
			if m.hotFit(msg.Event) {
				return h.relayoutSoon(), false
			}
			return nil, false
		}
		if p := m.hotPaneOf(msg.Event.Surface); p != nil {
			return m.hotPaneEvent(p, msg.Event), true
		}
		return m.hotEvent(msg.Event), true
	case hottytea.RelayoutMsg:
		if msg.Lost && h.native() {
			// The frame's Layout sends the lost documents again, after the
			// resources they refer to.
			h.tracef("lost: the host lost documents; resources again")
			h.s.Send(hotty.Res(hotCSSID, "text/css", []byte(hotCSS)))
			h.docs.lost()
			h.attachN = 0 // the next frame places it again
		}
		return nil, true
	case hottytea.PongMsg:
		return nil, false
	case hotDiagramMsg:
		if msg.err != nil {
			h.tracef("mermaid: %v", msg.err)
			return nil, false
		}
		h.tracef("mermaid: %d bytes of SVG in %v", len(msg.svg), msg.took.Round(time.Millisecond))
		return nil, h.native() && h.docs.drew(msg.src, msg.svg)
	case hotRelayoutMsg:
		h.relayoutDue = false
		m.hotKeepTop()
		return nil, true
	case hotMeasureTimeoutMsg:
		if msg.wave != h.wave || len(h.pending) == 0 {
			return nil, false
		}
		if msg.heard != h.heard {
			// A slow host, still answering (a multiplexer that answers one
			// measure at a time): wait on it from here.
			return h.awaitWave(), false
		}
		// A host that leaves a whole wave unanswered will not answer the
		// next soon: show everything in cells rather than wait on it, until
		// the user does something (plx with no viewer answers nothing).
		h.tracef("deaf: wave %d left %d measures unanswered", msg.wave, len(h.pending))
		h.deaf = true
		for _, k := range h.pending {
			h.notYet(k)
		}
		h.pending = map[int]hotKey{}
		return nil, true
	}
	_, cmd = m.update(msg)
	return cmd, true
}

// measured takes a reply to a measuring placement, and reports whether it was
// one.
func (h *hotState) measured(r hotty.Reply) bool {
	if r.N != 0 && r.N == h.attachN {
		// ENOTERM, as expected, or rows if a terminal attached meanwhile.
		if h.attach != nil && r.OK && r.Rows > 0 {
			h.attached(r.Rows)
		}
		return true
	}
	key, ok := h.pending[r.N]
	if r.N == 0 || !ok {
		return false
	}
	delete(h.pending, r.N)
	h.heard++
	switch {
	case r.OK && r.Re == "place" && r.Rows > 0:
		h.rows[key] = r.Rows
		h.last[key.html] = r.Rows
		if h.attach != nil && *h.attach == key {
			// Asked again on input and measured before any fit came: the
			// attach surface has nothing left to wait for.
			h.unattach()
		}
	case r.Code == hotty.ENOTERM:
		// No terminal to lay it out with yet, which is no try: asked again
		// when one attaches.
		h.later = append(h.later, key)
		if h.attach == nil {
			h.attach = &key
		}
	case r.Code == hotty.EBUDGET:
		// Not laid out in time, which is not a refusal: plx before ENOTERM
		// answers so for every measure from a pane it is not showing.
		h.notYet(key)
	default:
		h.rows[key] = 0
	}
	return true
}

// notYet keeps a block the host did not measure in time for the next input,
// up to hotTries asks; after that it shows in cells.
func (h *hotState) notYet(k hotKey) {
	h.tries[k]++
	if h.tries[k] >= hotTries {
		h.rows[k] = 0
		return
	}
	h.later = append(h.later, k)
}

// again asks once more what the host did not measure in time. It is called
// on input, when the user is looking at margin: a plx pane no one sees
// answers each measure with EBUDGET, and with no viewer answers none.
func (h *hotState) again() {
	if len(h.later) == 0 || len(h.pending) > 0 {
		return
	}
	h.tracef("again: %d measures", len(h.later))
	for _, k := range h.later {
		delete(h.asked, k)
	}
	h.later = h.later[:0]
	h.deaf = false
}

// attached is a terminal laying the attach surface out in rows: they are its
// block's, the surface goes, and what was not measured is asked again.
func (h *hotState) attached(rows int) {
	k := *h.attach
	h.tracef("attach: a terminal laid %s out in %d rows", hotAttachName, rows)
	h.rows[k], h.last[k.html] = rows, rows
	h.unattach()
	h.again()
}

// unattach takes the attach surface down, if it was placed.
func (h *hotState) unattach() {
	if h.attachN != 0 {
		h.s.Send(hotty.Del(hotAttachName))
	}
	h.attach, h.attachN = nil, 0
}

// hotInput is whether msg is the user's: a key, the mouse, a paste, or what
// they did in a surface.
func hotInput(msg tea.Msg) bool {
	switch msg := msg.(type) {
	case tea.KeyPressMsg, tea.MouseMsg, tea.PasteMsg:
		return true
	case hottytea.EventMsg:
		return msg.Event.Kind != hotty.EventFit
	}
	return false
}

// hotFit takes a fit: a block's document needs other rows than it was placed
// with (an image decoded after it was measured, say). They become the block's
// rows at this width, and it reports whether they changed anything.
func (m *model) hotFit(ev hotty.Event) bool {
	h := m.hot
	rows := ev.FitRows
	if rows <= 0 {
		return false
	}
	if ev.Surface == hotAttachName {
		if h.attach == nil {
			return false
		}
		h.attached(rows)
		return true
	}
	for _, b := range h.blocks {
		if hotName(m.path, m.entries[b.entry].b.anchor, b.entry) != ev.Surface {
			continue
		}
		h.tracef("fit: %s needs %d rows, placed with %d", ev.Surface, rows, b.rows)
		k := hotKey{b.base, h.col.W}
		if h.rows[k] == rows {
			return false
		}
		h.rows[k], h.last[b.base] = rows, rows
		return true
	}
	return false
}

// relayoutSoon draws the frame again shortly, once for a wave's replies
// rather than once each.
func (h *hotState) relayoutSoon() tea.Cmd {
	if h.relayoutDue {
		return nil
	}
	h.relayoutDue = true
	return tea.Tick(hotRelayoutAfter, func(time.Time) tea.Msg { return hotRelayoutMsg{} })
}

// hotKeepTop remembers which block is at the top of the screen, so the next
// frame keeps it there when blocks above it change height.
func (m *model) hotKeepTop() {
	for i, s := range m.spans {
		if m.scroll >= s.start && m.scroll <= s.end {
			m.hot.anchor = &hotAnchor{entry: i, off: m.scroll - s.start}
			return
		}
	}
}

// hotRows says whether a block shows as a surface in this frame, and the
// rows it takes. A block not measured at this width yet is queued for it,
// and shows in cells meanwhile, unless an earlier width's rows stand in.
func (m *model) hotRows(b block, w, line int) (base string, rows int, ok bool) {
	h := m.hot
	if !h.native() || h.deaf || !hotShows(b) {
		return "", 0, false
	}
	base = h.docs.base(b, m.src)
	if base == "" {
		return "", 0, false
	}
	k := hotKey{base, w}
	if r, done := h.rows[k]; done {
		return base, r, r > 0
	}
	if !h.asked[k] {
		h.want = append(h.want, hotWant{key: k, line: line})
	}
	if r := h.last[base]; r > 0 {
		return base, r, true
	}
	return base, 0, false
}

// hotFrame draws the frame, places the surfaces in it, and measures what is
// still to be measured. It returns the commands that send it all.
func (m *model) hotFrame() tea.Cmd {
	h := m.hot
	h.want = h.want[:0]
	v := m.view()
	h.frame = &v
	h.s.Layout(append(m.hotSurfaces(), m.hotPaneSurfaces()...))
	// A pane's document went with the layout; its changes, and the keyboard
	// where the pane wants it, go after.
	for _, hp := range h.panes {
		h.s.Send(hp.p.html(hotPaneName(hp.p)).Update()...)
	}
	m.hotResend()
	measuring := m.hotMeasure()
	return tea.Batch(h.s.Flush(), measuring, h.draw())
}

// draw renders the diagrams the frame found, each off the update loop: the
// first waits for the renderer to compile, seconds on a cold cache.
func (h *hotState) draw() tea.Cmd {
	var cmds []tea.Cmd
	for _, src := range h.docs.diagramsDue {
		render := h.mermaid
		cmds = append(cmds, func() tea.Msg {
			start := time.Now()
			svg, err := render(context.Background(), src)
			return hotDiagramMsg{src: src, svg: svg, err: err, took: time.Since(start)}
		})
	}
	h.docs.diagramsDue = nil
	return tea.Batch(cmds...)
}

// hotSurfaces are the blocks on screen, as surfaces in the document's column.
func (m *model) hotSurfaces() []hottytea.Surface {
	h := m.hot
	if !h.shown {
		return nil
	}
	var out []hottytea.Surface
	h.now = h.now[:0]
	clip := h.col
	q := m.activeQuery()
	for _, b := range h.blocks {
		rect := hottytea.Rect{X: h.col.X, Y: b.start - m.scroll, W: h.col.W, H: b.rows}
		if rect.Intersect(clip).H <= 0 {
			continue
		}
		e := m.entries[b.entry]
		name := hotName(m.path, e.b.anchor, b.entry)
		doc := hotDocument(b.base, b.kind, b.reviewed, b.selected, q)
		out = append(out, hottytea.Surface{
			Name:  name,
			Rect:  rect,
			Clip:  &clip,
			Keep:  true,
			Press: true,
			Fit:   true, // every 0.3 host sends fit (SPEC §4)
			Hover: true,
			Doc: func() string {
				h.sent[name] = doc
				return doc
			},
		})
		h.now = append(h.now, hotSent{name, doc})
	}
	return out
}

// hotPaneSurfaces are the panes this frame placed.
func (m *model) hotPaneSurfaces() []hottytea.Surface {
	var out []hottytea.Surface
	for _, hp := range m.hot.panes {
		s := hp.p.surface(hotPaneName(hp.p), hp.rect)
		clip := hp.clip
		s.Clip = &clip
		out = append(out, s)
	}
	return out
}

// hotPaneOf is the pane a surface of the host's is, if any.
func (m *model) hotPaneOf(surface string) *pane {
	for _, hp := range m.hot.panes {
		if hotPaneName(hp.p) == surface {
			return hp.p
		}
	}
	return nil
}

// hotPaneEvent is what the user did in a pane on the host: its HTML
// rendition takes it, and margin hears the actions it sent.
func (m *model) hotPaneEvent(p *pane, ev hotty.Event) tea.Cmd {
	if err := p.html(hotPaneName(p)).Event(ev); err != nil {
		m.hot.tracef("pane %s: %v", p.id, err)
	}
	p.dirty = true
	if m.comp != nil && p == m.comp.p {
		if !p.c.St.Keyboard {
			// Tab past the last button, or before the field: the host gave
			// the keyboard back. The composer keeps it, as in cells.
			p.focus(p.c.FindComponent("field", rootScope))
		}
		return m.composerActs()
	}
	switch {
	case p.id == paletteID && m.paletteOpen:
		m.paletteQueried(p.text("/query"))
	case p.id == searchID && m.searchOpen:
		m.searchDrafted(p.text("/draft"))
	case p.id == inboxID && m.inbox:
		m.inboxClicked(p)
	case p.id == treeID && m.tree != nil:
		m.treeClicked(p)
	}
	return nil
}

// hotResend sends again the document of a surface whose review state or
// search marks changed since it was sent: the same rows, so the placement
// stands (SPEC §5.1).
func (m *model) hotResend() {
	h := m.hot
	for _, d := range h.now {
		if h.s.Has(d.name) && h.sent[d.name] != d.doc {
			h.s.Send(hotty.Doc(d.name, d.doc))
			h.sent[d.name] = d.doc
		}
	}
}

// hotMeasure sends the next wave of measures, nearest the screen first, once
// the last has come back. Each block goes to a measuring surface of its own,
// is placed with r=auto in the screen's last cell beneath everything (a
// window of one cell, z=-1000), and the host's reply says its rows; the
// surface is deleted straight after, so nothing of it stays on screen.
func (m *model) hotMeasure() tea.Cmd {
	h := m.hot
	if h.attach != nil && h.attachN == 0 {
		// Placed like a measure, but kept, with fit, until a terminal
		// attaches and lays it out.
		h.nextN++
		h.attachN = h.nextN
		h.s.Send(hotty.Doc(hotAttachName, hotDocument(h.attach.html, "measure", false, false, "")),
			hotty.PlaceAt(hotAttachName, max(0, m.w-1), max(0, m.h-1),
				hotty.Placement{Cols: h.attach.w, Window: hotty.Window{W: 1, H: 1}, Z: -1000, Fit: hotty.On}, hotty.N(h.attachN)))
		h.tracef("attach: no terminal yet; %s waits for one", hotAttachName)
	}
	if len(h.pending) > 0 || len(h.want) == 0 || h.deaf {
		return nil
	}
	dist := func(line int) int {
		switch {
		case line < m.scroll:
			return m.scroll - line
		case line >= m.scroll+m.h:
			return line - m.scroll - m.h + 1
		}
		return 0
	}
	sort.SliceStable(h.want, func(i, j int) bool { return dist(h.want[i].line) < dist(h.want[j].line) })
	n := 0
	for _, w := range h.want {
		if n == hotWave {
			break
		}
		if h.asked[w.key] {
			continue
		}
		h.asked[w.key] = true
		h.nextN++
		h.pending[h.nextN] = w.key
		// A surface of its own for each measure, gone as soon as it is
		// placed: a host that sizes it later than the placement (a
		// multiplexer relaying it on its next frame, as plx does) can only
		// ever see this block's document under this name.
		name := hotMeasureName + "-" + strconv.Itoa(h.nextN)
		h.s.Send(hotty.Doc(name, hotDocument(w.key.html, "measure", false, false, "")),
			hotty.PlaceAt(name, max(0, m.w-1), max(0, m.h-1),
				hotty.Placement{Cols: w.key.w, Window: hotty.Window{W: 1, H: 1}, Z: -1000}, hotty.N(h.nextN)),
			hotty.Del(name))
		n++
	}
	if n == 0 {
		return nil
	}
	h.wave++
	h.tracef("measure: wave %d, %d blocks", h.wave, n)
	return h.awaitWave()
}

// awaitWave gives up on the wave if hotMeasureWait passes without an answer
// to any of it.
func (h *hotState) awaitWave() tea.Cmd {
	wave, heard := h.wave, h.heard
	return tea.Tick(hotMeasureWait, func(time.Time) tea.Msg { return hotMeasureTimeoutMsg{wave, heard} })
}

// hotEvent is what the user did in a block: a press focuses it, as a click on
// cells does, and a click on a link within the review follows it. Links to
// the web are hyperlinks, which the terminal opens without telling margin.
// The pointer over a block marks it in the gutter, as motion over cells does.
func (m *model) hotEvent(ev hotty.Event) tea.Cmd {
	if ev.Kind == hotty.EventHover && (ev.Hover == nil || ev.Hover.Out) {
		// Out, from wherever the block went since.
		if m.hot.hoverFrom == ev.Surface {
			m.hoveredEntry = -1
			m.hot.hoverFrom = ""
		}
		return nil
	}
	entry := -1
	for _, b := range m.hot.blocks {
		if hotName(m.path, m.entries[b.entry].b.anchor, b.entry) == ev.Surface {
			entry = b.entry
			break
		}
	}
	if entry < 0 {
		return nil
	}
	switch ev.Kind {
	case hotty.EventHover:
		m.hoveredEntry = entry
		m.hot.hoverFrom = ev.Surface
	case hotty.EventPress:
		target := cursor{entry: entry, comment: commentNone}
		if m.comp != nil {
			m.blur(&target)
			return nil
		}
		m.at = target
	case hotty.EventClick:
		if ev.Link == nil {
			return nil
		}
		href := ev.Link.Href
		m.at = cursor{entry: entry, comment: commentNone}
		if !m.followHref(href) {
			m.status = href + " is outside this review"
		}
	}
	return nil
}

// hotWatch hands the terminal Bubble Tea draws on to the Session, which
// reads what goes out (hottytea.Session.Watch). A terminal's file stays one,
// so Bubble Tea still sizes it and sets it raw.
func (m *model) hotWatch(out io.Writer) io.Writer {
	if m.hot == nil {
		return out
	}
	if f, ok := out.(*os.File); ok {
		return m.hot.s.WatchFile(f)
	}
	return m.hot.s.Watch(out)
}

// hotMatches adds the search's matches in the blocks shown as surfaces, whose
// rows are blank cells: found in the block's cell rendering, as they would be
// without a host, and placed on the block's rows (on its last, for a match
// below the rows the host gave it). The matches stay in document order.
func (m *model) hotMatches(q string) {
	if m.hot == nil || len(m.hot.blocks) == 0 {
		return
	}
	added := false
	for _, b := range m.hot.blocks {
		for r, l := range b.searchText {
			_, ranges := highlightSearch(l, q)
			for _, rg := range ranges {
				m.searchMatches = append(m.searchMatches, searchMatch{
					line: b.start + min(r, b.rows-1), lo: rg.lo, hi: rg.hi, entry: b.entry,
				})
				added = true
			}
		}
	}
	if added {
		sort.SliceStable(m.searchMatches, func(i, j int) bool { return m.searchMatches[i].line < m.searchMatches[j].line })
	}
}
