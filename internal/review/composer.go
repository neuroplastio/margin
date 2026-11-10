package review

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"neuroplast.io/hotty-surf/surflive"
	"neuroplast.io/hotty-surf/surftea"
)

// outcome is how the composer was dismissed.
type outcome int

const (
	outcomeSubmit  outcome = iota // apply the text
	outcomeDraft                  // keep it unsubmitted
	outcomeDiscard                // throw it away
)

// composerID is the composer's pane: there is one composer at a time.
const composerID = "composer"

// The keys the composer answers itself (D22, interaction.md). Everything
// else is the field's: typing, the cursor, selection, undo, the clipboard.
// enter is a newline; shift+enter is one too.
const (
	keySubmit    = "ctrl+s"
	keySubmitAlt = "ctrl+enter"
	keyDraft     = "escape"
	keyDiscard   = "ctrl+g"
)

// composerFieldKeys are the keys the composer answers itself, given to the
// program by the surface's field (surfrun.Surface.Program), so they reach
// margin as keys while the field has the keyboard. escape always does.
var composerFieldKeys = []string{keySubmit, keySubmitAlt, keyDiscard}

// Compose is the composer's state (compose.surf lays it out): the text
// the field holds, the label of its first button, and the field's height in
// rows as the cells wrap it. margin keeps the rest in composer.
type Compose struct {
	Text   string
	Submit string
	// Grown is the field's rows, as @height last said.
	Grown int

	typed func(string)
	ask   func(outcome)
}

// Typed is the field's input: text is the whole of what it holds.
func (c *Compose) Typed(text string) {
	c.Text = text
	if c.typed != nil {
		c.typed(text)
	}
}

// Grew is the field's height in rows, which the program places the surface
// by.
func (c *Compose) Grew(rows int) { c.Grown = rows }

// Submitted, Kept and Discarded are the buttons.
func (c *Compose) Submitted() { c.asks(outcomeSubmit) }
func (c *Compose) Kept()      { c.asks(outcomeDraft) }
func (c *Compose) Discarded() { c.asks(outcomeDiscard) }

func (c *Compose) asks(o outcome) {
	if c.ask != nil {
		c.ask(o)
	}
}

// composer is one comment being written: a hotty-surf surface in the
// document column (D22, composer.surf), with the comment's text and nothing
// else in it, and a row of buttons for the mouse. It is made when a comment
// is opened and dropped when it is dismissed, so a resumed draft starts with
// the cursor after its text, as a new field's does.
type composer struct {
	host   *surftea.Host
	st     *Compose
	anchor string
	target int // newCommentSlot, or the index of the posted comment being edited
	// asked are the outcomes the buttons asked for since margin last looked.
	asked []outcome
}

// newComposer opens a composer on seed.
func newComposer(anchor string, target int, seed string) *composer {
	submit := "Submit"
	if target != newCommentSlot {
		submit = "Save"
	}
	c := &composer{anchor: anchor, target: target}
	c.st = &Compose{Text: seed, Submit: submit, ask: func(o outcome) { c.asked = append(c.asked, o) }}
	c.host = newSurfHost("margin-"+composerID, surflive.New(surfInterpCompose(c.st)), composerFieldKeys)
	return c
}

// body returns what the reviewer wrote.
func (c *composer) body() string { return parseComment(c.st.Text) }

// parseComment trims what the field holds: it holds only what the reviewer
// typed.
func parseComment(raw string) string { return strings.TrimSpace(raw) }

// keyOutcome is the outcome a key asks of the composer, if it is one of
// the composer's own.
func keyOutcome(name string) (outcome, bool) {
	switch name {
	case keySubmit, keySubmitAlt:
		return outcomeSubmit, true
	case keyDraft:
		return outcomeDraft, true
	case keyDiscard:
		return outcomeDiscard, true
	}
	return 0, false
}

// draftPath keys drafts by anchor and target, so a half-written reply and a
// half-finished edit of an existing comment cannot collide. In the real tool
// these become thread-file entries with `status: draft`.
func draftPath(anchor string, target int) (string, error) {
	base, err := os.UserCacheDir()
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256([]byte(anchor + "#" + strconv.Itoa(target)))
	dir := filepath.Join(base, "margin", "drafts")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	return filepath.Join(dir, hex.EncodeToString(sum[:8])+".md"), nil
}

func loadDraft(anchor string, target int) (string, error) {
	path, err := draftPath(anchor, target)
	if err != nil {
		return "", err
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

func saveDraft(anchor string, target int, body string) error {
	path, err := draftPath(anchor, target)
	if err != nil {
		return err
	}
	if strings.TrimSpace(body) == "" {
		return clearDraft(anchor, target)
	}
	return os.WriteFile(path, []byte(body), 0o644)
}

func clearDraft(anchor string, target int) error {
	path, err := draftPath(anchor, target)
	if err != nil {
		return err
	}
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

// dropComposer closes the composer, and on a host its surface: a composer
// made again is a new document there.
func (m *model) dropComposer() {
	if m.comp == nil {
		return
	}
	if m.hot != nil {
		m.hot.s.Delete(m.comp.host.Name())
	}
	m.comp.host.Close()
	m.comp = nil
}

// composerRows is the composer's height in rows, w cells wide. On a host it
// is the host's measure once it has said (a fit event), and before that
// hotty-surf's estimate from the host's cell; in cells it is the field's
// rows as the cells wrap it, and the buttons under them.
func (m *model) composerRows(w int) int {
	c := m.comp
	if m.hot.native() && !m.hot.deaf {
		if rows := surfRows(c.host, w, m.hot.caps); rows > 0 {
			return rows
		}
	}
	return composerCellRows(c, w)
}

// composerCellRows is the composer's rows in cells: one drawing of it says
// how tall the field wraps (@height), and the bar goes under it.
func composerCellRows(c *composer, w int) int {
	c.host.Lines(w, composerMostRows)
	return max(c.st.Grown, composerLeastField) + composerBarRows
}

// The field grows between three rows and eight (compose.surf), and the
// buttons are the row under it.
const (
	composerLeastField = 3
	composerBarRows    = 1
	composerMostRows   = 8 + composerBarRows
)

// composerLines is the composer drawn in cells, w wide.
func (m *model) composerLines(w int) []string {
	return m.comp.host.Lines(w, m.composerRows(w))
}
