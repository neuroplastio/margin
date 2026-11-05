package review

import (
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/neuroplastio/hotty-go"
	"github.com/neuroplastio/hotty-go/hottytea"
	"github.com/neuroplastio/hotty-go/hottytest"
	"github.com/neuroplastio/hotty-surf/surfrun"
	"github.com/neuroplastio/hotty-surf/surfui"
)

// seamCount is a hotty-surf model that counts a button's clicks.
type seamCount struct{ n int }

func (c *seamCount) Render() *surfui.Node {
	return surfui.Surface(surfui.Stack(
		surfui.Text(string(rune('0'+c.n))).ID("count"),
		surfui.Button("add").ID("add").OnClick(func() { c.n++ }),
	))
}

func (c *seamCount) Do(act surfui.Action, _ surfrun.Event) {
	if f, ok := act.(func()); ok {
		f()
	}
}

// seamProgram is the smallest program in margin's shape: a Bubble Tea model
// whose Session places one hotty-surf surface, and routes its events.
type seamProgram struct {
	h    *hotState
	pane *surfHost
}

func (m *seamProgram) Init() tea.Cmd { return m.h.s.Detect() }

func (m *seamProgram) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	msg, cmd := m.h.s.Update(msg)
	switch msg := msg.(type) {
	case hottytea.EventMsg:
		if msg.Surface == m.pane.name {
			m.pane.event(m.h, msg.Event)
		}
	case hottytea.ReadyMsg:
		if msg.Mode == hottytea.Native {
			m.pane.ready(msg.Caps)
		}
	case surfPostMsg:
		msg.p.posted(m.h, msg.f)
	case tea.KeyPressMsg:
		if msg.String() == "q" {
			return m, tea.Sequence(m.h.s.Close(), tea.Quit)
		}
	}
	if m.h.s.Mode == hottytea.Native {
		m.h.s.Layout([]hottytea.Surface{m.pane.surface(m.h, hottytea.Rect{X: 0, Y: 1, W: 20, H: 3}, hottytea.Rect{W: 40, H: 10})})
		m.pane.sendFirst(m.h)
		return m, tea.Batch(cmd, m.h.s.Flush())
	}
	return m, cmd
}

func (m *seamProgram) View() tea.View {
	v := tea.NewView("margin\n")
	v.AltScreen = true
	return v
}

// TestSurfSeam: a hotty-surf model is a surface of a hottytea Session next
// to margin's own, and a click on its button reaches the model and comes
// back as a delta, not a document.
func TestSurfSeam(t *testing.T) {
	h := hottytest.New(t)
	m := &seamProgram{h: newHotState()}
	p := tea.NewProgram(m, tea.WithInput(h), tea.WithOutput(m.h.s.Watch(h)),
		tea.WithWindowSize(40, 10), tea.WithoutSignalHandler(),
		tea.WithEnvironment([]string{"TERM=xterm-256color"}))
	m.h.s.Attach(p.Send)
	m.pane = newSurfHost("margin-seam", &seamCount{}, p.Send, nil)
	done := make(chan error, 1)
	go func() { _, err := p.Run(); done <- err }()

	deadline := time.Now().Add(10 * time.Second)
	for h.Click("margin-seam", "add") != nil {
		if time.Now().After(deadline) {
			t.Fatal("the surface was never placed")
		}
		time.Sleep(time.Millisecond)
	}
	_ = h.Click("margin-seam", "add")
	time.Sleep(200 * time.Millisecond)
	h.Type("q")
	if err := <-done; err != nil {
		t.Fatal(err)
	}

	docs, deltas := 0, 0
	var last string
	for _, c := range h.Commands() {
		switch hotty.Get(c.Control, "a") {
		case "doc":
			if hotty.Get(c.Control, "s") == "margin-seam" {
				docs++
			}
		case "delta":
			ops, _ := hotty.DeltaOf(c)
			for _, op := range ops {
				deltas++
				if op.Value != nil {
					last = *op.Value
				}
			}
		}
	}
	if docs != 1 {
		t.Errorf("the surface's document was sent %d times, want once", docs)
	}
	if deltas == 0 || last != "2" {
		t.Errorf("clicks came back as %d ops ending %q, want the count 2", deltas, last)
	}
}
