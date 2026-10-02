package review

import (
	"io"
	"os"
	"path/filepath"
	"sort"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/neuroplastio/hotty-go"
	"github.com/neuroplastio/hotty-go/hottytea"
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
	deaf    bool // a host that never answered a measure: everything in cells
	want    []hotWant

	// The frame: what View returns on a host, the blocks render placed in
	// it, and where the document's column is.
	frame  *tea.View
	blocks []hotBlock
	col    hottytea.Rect
	shown  bool // the document's column is on screen (not the inbox)

	// sent is each surface's document as last sent, so a change of review
	// state sends the document again; now is this frame's.
	sent map[string]string
	now  []hotSent

	relayoutDue bool
	anchor      *hotAnchor
	closed      bool
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

// hotSent is a surface's document in this frame.
type hotSent struct{ name, doc string }

// hotAnchor keeps the top of the screen in place while measured blocks change
// height above it.
type hotAnchor struct{ entry, off int }

// hotRelayoutMsg draws the frame again once a wave of measures has come back.
type hotRelayoutMsg struct{}

// hotMeasureTimeoutMsg gives up on a wave of measures the host did not
// answer.
type hotMeasureTimeoutMsg struct{ wave int }

const (
	hotMeasureName = "margin-measure"
	// hotWave is how many blocks are measured at once, nearest the screen
	// first: enough to fill it, few enough that the first frame comes soon.
	hotWave = 64
	// hotMeasureWait is how long a wave may go unanswered.
	hotMeasureWait = 3 * time.Second
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
		sent:    map[string]string{},
	}
	h.docs.res = func(id, mime string, data []byte) { h.s.Send(hotty.Res(id, mime, data)) }
	return h
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
	switch msg := msg.(type) {
	case nil:
		return nil, false
	case hottytea.ReadyMsg:
		if msg.Mode == hottytea.Native {
			h.s.Send(hotty.Res(hotCSSID, "text/css", []byte(hotCSS)))
		}
		return nil, true
	case hottytea.AckMsg:
		if h.measured(msg.Reply) {
			return h.relayoutSoon(), false
		}
		return nil, false
	case hottytea.ErrorMsg:
		if h.measured(msg.Reply) {
			return h.relayoutSoon(), false
		}
		m.status = "hotty: " + msg.Err().Error()
		return nil, true
	case hottytea.EventMsg:
		return m.hotEvent(msg.Event), true
	case hottytea.RelayoutMsg:
		return nil, true
	case hottytea.PongMsg:
		return nil, false
	case hotRelayoutMsg:
		h.relayoutDue = false
		m.hotKeepTop()
		return nil, true
	case hotMeasureTimeoutMsg:
		if msg.wave != h.wave || len(h.pending) == 0 {
			return nil, false
		}
		// A host that leaves a whole wave unanswered will not answer the
		// next: show everything in cells rather than wait on it.
		h.deaf = true
		h.pending = map[int]hotKey{}
		return nil, true
	}
	_, cmd = m.update(msg)
	return cmd, true
}

// measured takes a reply to a measuring placement, and reports whether it was
// one.
func (h *hotState) measured(r hotty.Reply) bool {
	key, ok := h.pending[r.N]
	if r.N == 0 || !ok {
		return false
	}
	delete(h.pending, r.N)
	if r.OK && r.Re == "place" && r.Rows > 0 {
		h.rows[key] = r.Rows
		h.last[key.html] = r.Rows
	} else {
		h.rows[key] = 0
	}
	return true
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
	if !h.native() || h.deaf || !hotKind(b.kind) {
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
	h.s.Layout(m.hotSurfaces())
	m.hotResend()
	measuring := m.hotMeasure()
	return tea.Batch(h.s.Flush(), measuring)
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
			Doc: func() string {
				h.sent[name] = doc
				return doc
			},
		})
		h.now = append(h.now, hotSent{name, doc})
	}
	return out
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
// the last has come back. Each block goes to the one measuring surface, is
// placed with r=auto in the screen's last cell beneath everything (a window
// of one cell, z=-1000), and the host's reply says its rows; the surface is
// deleted at the end of the wave, so nothing of it stays on screen.
func (m *model) hotMeasure() tea.Cmd {
	h := m.hot
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
		h.s.Send(hotty.Doc(hotMeasureName, hotDocument(w.key.html, "measure", false, false, "")),
			hotty.PlaceAt(hotMeasureName, max(0, m.w-1), max(0, m.h-1),
				hotty.Placement{Cols: w.key.w, Window: hotty.Window{W: 1, H: 1}, Z: -1000}, hotty.N(h.nextN)))
		n++
	}
	if n == 0 {
		return nil
	}
	h.s.Send(hotty.Del(hotMeasureName))
	h.wave++
	wave := h.wave
	return tea.Tick(hotMeasureWait, func(time.Time) tea.Msg { return hotMeasureTimeoutMsg{wave} })
}

// hotEvent is what the user did in a block: a press focuses it, as a click on
// cells does, and a click on a link within the review follows it. Links to
// the web are hyperlinks, which the terminal opens without telling margin.
func (m *model) hotEvent(ev hotty.Event) tea.Cmd {
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
	case hotty.EventPress:
		target := cursor{entry: entry, comment: commentNone}
		if m.comp != nil {
			m.blur(&target)
			return nil
		}
		m.at = target
	case hotty.EventClick:
		href, _, ok := ev.Link()
		if !ok {
			return nil
		}
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
