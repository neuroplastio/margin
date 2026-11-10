package review

import (
	"strconv"
	"strings"

	"neuroplast.io/hotty-surf/pkg/cells"
	"neuroplast.io/hotty-surf/surfmd"
	"neuroplast.io/hotty-surf/surfrun"
	"neuroplast.io/hotty-surf/surfui"
)

// The open thread's comments (D22, amended by the hotty-surf port): a stack
// of comments, each who and when over what they said as Markdown (surfmd),
// drawn in cells at the box's width, as tall as they are (Draw(cols, 0)).
// margin keeps the two columns beside them, where the focused comment's text
// carries the focus bar and each comment has its subspan, so it asks the
// layout where each is (BoxOf). A thread's drawing is kept until what it shows
// changes.

// commentsDraw is a thread's visible comments drawn at one width.
type commentsDraw struct {
	sig   string
	lines []string
	// body and whole are the rows each comment's text, and the comment with
	// its head, takes (first row and height), by the comment's index.
	body, whole map[int][2]int
}

// commentsModel is the tree a drawing shows; it takes no input.
type commentsModel struct{ tree *surfui.Node }

func (c *commentsModel) Render() *surfui.Node            { return c.tree }
func (c *commentsModel) Do(surfui.Action, surfrun.Event) {}

// staticRunner is the runner of a surface nothing posts from.
type staticRunner struct{}

func (staticRunner) Post(func())  {}
func (staticRunner) Quit()        {}
func (staticRunner) Focus(string) {}

// commentView is one visible comment as the drawing shows it.
type commentView struct {
	j       int
	head    string
	text    string
	deleted bool
}

// commentViews are the thread's visible comments, with their heads: who and
// when, and a marker where the text is an unsaved edit or deleted.
func (m *model) commentViews(t *thread, visible []int) []commentView {
	out := make([]commentView, 0, len(visible))
	for _, j := range visible {
		c := t.posted[j]
		v := commentView{j: j, head: humanAge(c.at), text: c.body}
		switch d := t.draft(j); {
		case d != "":
			v.head += "  ✎ edited, unsaved"
			v.text = d
		case c.deleted:
			v.head += "  [deleted]"
			v.deleted = true
			if v.text == "" {
				v.text = "[deleted]"
			}
		}
		v.head = c.author + "\x00" + v.head // the author is bold, the rest quiet
		out = append(out, v)
	}
	return out
}

// drawComments draws the thread's visible comments inner cells wide, or
// gives back the drawing it kept if nothing it shows changed.
func (m *model) drawComments(t *thread, visible []int, inner int) *commentsDraw {
	views := m.commentViews(t, visible)
	var sig strings.Builder
	sig.WriteString(strconv.Itoa(inner))
	for _, v := range views {
		sig.WriteString("\x01" + strconv.Itoa(v.j) + "\x01" + v.head + "\x01" + v.text)
	}
	if d := m.commentDraws[t.anchor]; d != nil && d.sig == sig.String() {
		return d
	}
	kids := make([]*surfui.Node, 0, len(views))
	for _, v := range views {
		author, rest, _ := strings.Cut(v.head, "\x00")
		head := surfui.Row(surfui.Text(author).Bold(), surfui.Text("· "+rest).Muted().Small()).Gap(1)
		body := surfmd.Convert([]byte(v.text), surfmd.HardWraps(), surfmd.GFM(), surfmd.Compact()).Key("body")
		if v.deleted {
			body = surfui.Stack(body).Muted().Key("body")
		}
		kids = append(kids, surfui.Stack(head, body).Key("c"+strconv.Itoa(v.j)))
	}
	root := surfui.Stack(kids...).Gap(1).Key("comments")
	s := surfrun.NewSurface(&commentsModel{tree: root}, staticRunner{})
	r := cells.New(s.Cells())
	r.Show()
	r.Update()
	scr := r.Draw(inner, 0)
	d := &commentsDraw{sig: sig.String(), lines: surfrun.Paint(scr), body: map[int][2]int{}, whole: map[int][2]int{}}
	for _, v := range views {
		id := "c" + strconv.Itoa(v.j)
		if _, y, _, h, ok := r.BoxOf(id, "body"); ok {
			d.body[v.j] = [2]int{y, h}
		}
		if _, y, _, h, ok := r.BoxOf(id); ok {
			d.whole[v.j] = [2]int{y, h}
		}
	}
	if m.commentDraws == nil {
		m.commentDraws = map[string]*commentsDraw{}
	}
	m.commentDraws[t.anchor] = d
	return d
}
