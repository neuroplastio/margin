package review

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"strconv"
	"strings"
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

// composerKeys gives the field's keys margin answers to the program on a
// host (SPEC §10.2), so they reach margin as keys while the field has the
// host's keyboard. escape always does.
const composerKeys = keySubmit + "=program " + keySubmitAlt + "=program " + keyDiscard + "=program"

// composer is one comment being written: an A2UI longText field in its own
// pane (D22), with the comment's text and nothing else in it, and a row of
// buttons for the mouse. It is made when a comment is opened and dropped
// when it is dismissed, so a resumed draft starts with the cursor after its
// text, as a new field's does.
type composer struct {
	p      *pane
	anchor string
	target int // newCommentSlot, or the index of the posted comment being edited
}

// newComposer opens a composer on seed.
func newComposer(k *kit, anchor string, target int, seed string) *composer {
	p := k.pane(composerID, map[string]any{"text": seed})
	submit := "Submit"
	if target != newCommentSlot {
		submit = "Save"
	}
	comps := []map[string]any{
		obj("id", "root", "component", "Column", "children", []any{"field", "bar"}),
		obj("id", "field", "component", "TextField", "label", "", "value", bind("/text"),
			"variant", "longText", "placeholder", "Write a comment",
			"metadata", obj("extensions", obj("io_neuroplast_hotty", obj("keys", composerKeys)))),
		obj("id", "bar", "component", "Row", "children", []any{"submit", "draft", "discard"}),
	}
	comps = append(comps, button("submit", submit, "primary")...)
	comps = append(comps, button("draft", "Keep draft", "")...)
	comps = append(comps, button("discard", "Discard", "borderless")...)
	p.define(comps...)
	p.focus(p.c.FindComponent("field", rootScope))
	return &composer{p: p, anchor: anchor, target: target}
}

// button is a pane's button and its label, the button sending the action
// named id.
func button(id, label, variant string) []map[string]any {
	b := obj("id", id, "component", "Button", "child", id+"_t", "action", obj("event", obj("name", id)))
	if variant != "" {
		b["variant"] = variant
	}
	return []map[string]any{b, obj("id", id+"_t", "component", "Text", "text", label)}
}

// body returns what the reviewer wrote.
func (c *composer) body() string { return parseComment(c.p.text("/text")) }

// parseComment trims what the field holds: it holds only what the reviewer
// typed.
func parseComment(raw string) string { return strings.TrimSpace(raw) }

// actionOutcome is the outcome a composer button's action asks for.
func actionOutcome(name string) (outcome, bool) {
	switch name {
	case "submit":
		return outcomeSubmit, true
	case "draft":
		return outcomeDraft, true
	case "discard":
		return outcomeDiscard, true
	}
	return 0, false
}

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
