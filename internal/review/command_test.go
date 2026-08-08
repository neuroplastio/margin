package review

import (
	"testing"

	tea "charm.land/bubbletea/v2"
	uv "github.com/charmbracelet/ultraviolet"
)

// TestKeymapResolvesToRegisteredCommands guards against a keymap entry that
// points at an id nothing registers — a typo that handleKey would otherwise
// silently swallow.
func TestKeymapResolvesToRegisteredCommands(t *testing.T) {
	for key, id := range keymap {
		if _, ok := commandByID(id); !ok {
			t.Errorf("keymap[%q] = %q, which is not a registered command", key, id)
		}
	}
}

// TestEveryCommandIsReachableByAKey: the registry is meant to be the one
// source of truth for both keys and (eventually) the palette. A command with
// no key bound to it yet is fine once the palette exists, but today, before
// it does, every registered command should still be reachable — otherwise it
// is dead code.
func TestEveryCommandIsReachableByAKey(t *testing.T) {
	bound := map[string]bool{}
	for _, id := range keymap {
		bound[id] = true
	}
	for _, c := range commands {
		if !bound[c.ID] {
			t.Errorf("command %q is registered but no key resolves to it", c.ID)
		}
	}
}

// TestMoveKeysRouteThroughTheRegistry: j/k/g/G resolve to the move commands
// and produce the same focus movement moveFocus itself would.
func TestMoveKeysRouteThroughTheRegistry(t *testing.T) {
	m := newTestModel(t)
	start := m.at

	m.handleKey(tea.KeyPressMsg(tea.Key{Code: 'j', Text: "j"}))
	if m.at == start {
		t.Fatal("j did not move focus")
	}
	m.handleKey(tea.KeyPressMsg(tea.Key{Code: 'k', Text: "k"}))
	if m.at != start {
		t.Fatalf("k did not return focus to %+v, got %+v", start, m.at)
	}

	m.handleKey(tea.KeyPressMsg(tea.Key{Code: 'G', Text: "G"}))
	if m.at.entry != len(m.entries)-1 {
		t.Fatalf("G left focus at entry %d, want the last entry %d", m.at.entry, len(m.entries)-1)
	}
	m.handleKey(tea.KeyPressMsg(tea.Key{Code: 'g', Text: "g"}))
	if m.at.entry != 0 {
		t.Fatalf("g left focus at entry %d, want 0", m.at.entry)
	}
}

// TestMarkKeysRouteThroughTheRegistry: r and f resolve to the same
// toggleMark calls the pre-registry switch made.
func TestMarkKeysRouteThroughTheRegistry(t *testing.T) {
	m := newTestModel(t)
	m.at = cursor{entry: blockEntryFor(t, m, freshAnchor), comment: commentNone}

	m.handleKey(tea.KeyPressMsg(tea.Key{Code: 'r', Text: "r"}))
	if m.marks[freshAnchor] != markOK {
		t.Fatalf("r gave mark %v, want markOK", m.marks[freshAnchor])
	}
	m.handleKey(tea.KeyPressMsg(tea.Key{Code: 'r', Text: "r"}))
	if m.marks[freshAnchor] != markNone {
		t.Fatalf("re-pressing r gave mark %v, want cleared", m.marks[freshAnchor])
	}
	m.handleKey(tea.KeyPressMsg(tea.Key{Code: 'f', Text: "f"}))
	if m.marks[freshAnchor] != markFlag {
		t.Fatalf("f gave mark %v, want markFlag", m.marks[freshAnchor])
	}
}

// TestResolveKeyRoutesThroughTheRegistry: R resolves to thread.resolve and
// toggles, the same way r/f toggle a mark.
func TestResolveKeyRoutesThroughTheRegistry(t *testing.T) {
	m := newTestModel(t)
	m.at = cursor{entry: entryFor(t, m, convoAnchor), comment: commentNone}

	m.handleKey(tea.KeyPressMsg(tea.Key{Code: 'R', Text: "R"}))
	if !m.threads[convoAnchor].resolved {
		t.Fatal("R did not resolve the thread")
	}
	m.handleKey(tea.KeyPressMsg(tea.Key{Code: 'R', Text: "R"}))
	if m.threads[convoAnchor].resolved {
		t.Fatal("re-pressing R did not unresolve the thread")
	}
}

// TestResolveOnBlockWithNoThreadIsANoop: thread.resolve's Applicable requires
// a thread to already exist — R on a bare paragraph must not create one just
// to resolve it.
func TestResolveOnBlockWithNoThreadIsANoop(t *testing.T) {
	m := newTestModel(t)
	m.at = cursor{entry: blockEntryFor(t, m, freshAnchor), comment: commentNone}
	m.toggleResolved()
	if m.threads[freshAnchor] != nil {
		t.Fatal("toggleResolved created a thread where none existed")
	}
	if m.status == "" {
		t.Fatal("expected a status message explaining nothing happened")
	}
}

// TestResolveTargetNamesTheAction: the palette title should read as an
// action ("resolve"/"unresolve"), not a bare state the user has to already
// know how to interpret.
func TestResolveTargetNamesTheAction(t *testing.T) {
	m := newTestModel(t)
	m.at = cursor{entry: entryFor(t, m, convoAnchor), comment: commentNone}
	if got, want := resolveTarget(m), "resolve"; got != want {
		t.Errorf("resolveTarget on an open thread = %q, want %q", got, want)
	}
	m.threads[convoAnchor].resolved = true
	if got, want := resolveTarget(m), "unresolve"; got != want {
		t.Errorf("resolveTarget on a resolved thread = %q, want %q", got, want)
	}

	m.at = cursor{entry: blockEntryFor(t, m, freshAnchor), comment: commentNone}
	if got := resolveTarget(m); got != "" {
		t.Errorf("resolveTarget on a block with no thread = %q, want \"\"", got)
	}
}

// TestQuitKeysRouteThroughTheRegistry: q and ctrl+c both resolve to
// app.quit.
func TestQuitKeysRouteThroughTheRegistry(t *testing.T) {
	for _, k := range []tea.Key{{Code: 'q', Text: "q"}, {Code: 'c', Mod: uv.ModCtrl}} {
		m := newTestModel(t)
		cmd := m.handleKey(tea.KeyPressMsg(k))
		if !m.quitting {
			t.Errorf("key %+v did not set quitting", k)
		}
		if cmd == nil {
			t.Errorf("key %+v produced no command; app.quit should return tea.Quit", k)
		}
	}
}

// TestUnknownKeyIsANoop: a key with no keymap entry does nothing, same as the
// pre-registry switch falling through with no matching case.
func TestUnknownKeyIsANoop(t *testing.T) {
	m := newTestModel(t)
	start := *m
	if cmd := m.handleKey(tea.KeyPressMsg(tea.Key{Code: 'z', Text: "z"})); cmd != nil {
		t.Fatal("unbound key produced a command")
	}
	if m.at != start.at || m.status != start.status {
		t.Fatal("unbound key changed model state")
	}
}

// --- CMD-04: focus-sensitive Target functions --------------------------------

// TestCommentTargetNamesBlockKind: comment.new's target names the kind of
// block it would attach to — the only thing distinguishing one before any
// comment exists on it.
func TestCommentTargetNamesBlockKind(t *testing.T) {
	m := newTestModel(t)

	m.at = cursor{entry: blockEntryFor(t, m, "^h1"), comment: commentNone}
	if got := commentTarget(m); got != "heading" {
		t.Errorf("commentTarget on a heading = %q, want %q", got, "heading")
	}

	m.at = cursor{entry: blockEntryFor(t, m, freshAnchor), comment: commentNone}
	if got := commentTarget(m); got != "paragraph" {
		t.Errorf("commentTarget on a paragraph = %q, want %q", got, "paragraph")
	}
}

// TestEditTargetNamesFocusedComment: focusing a specific posted comment names
// its author, so a palette row for that comment could not be confused with
// another one in the same thread.
func TestEditTargetNamesFocusedComment(t *testing.T) {
	m := newTestModel(t)
	i := entryFor(t, m, convoAnchor)

	m.at = cursor{entry: i, comment: 0}
	if got, want := editTarget(m), "comment by toly"; got != want {
		t.Errorf("editTarget at comment 0 = %q, want %q", got, want)
	}
	m.at = cursor{entry: i, comment: 1}
	if got, want := editTarget(m), "comment by agent"; got != want {
		t.Errorf("editTarget at comment 1 = %q, want %q", got, want)
	}
}

// TestEditTargetNamesDraftReply: focus on a thread holding only an
// unsubmitted reply names the draft, not a comment — there is no posted
// comment yet to attribute it to.
func TestEditTargetNamesDraftReply(t *testing.T) {
	m := newTestModel(t)
	m.at = cursor{entry: entryFor(t, m, draftAnchor), comment: commentNone}
	if got, want := editTarget(m), "draft reply"; got != want {
		t.Errorf("editTarget on a draft-only thread = %q, want %q", got, want)
	}
}

// TestEditTargetEmptyWhenNothingSpecificToEdit: comment.edit's Applicable
// only requires a thread to exist, which is true as soon as focus sits on
// the thread entry itself with no comment selected, no draft and no pending
// edit — editTarget has nothing more specific to say than editFocused's own
// "select a comment" status covers at Run time, so it returns "".
func TestEditTargetEmptyWhenNothingSpecificToEdit(t *testing.T) {
	m := newTestModel(t)
	m.at = cursor{entry: entryFor(t, m, soloAnchor), comment: commentNone}
	if got := editTarget(m); got != "" {
		t.Errorf("editTarget = %q, want \"\"", got)
	}
}

// TestMarkTargetNamesSectionSize: a single paragraph names itself "block"; a
// heading names the whole section it rolls up, matching sectionLabel's own
// wording so the palette and the status line left behind by the key never
// disagree.
func TestMarkTargetNamesSectionSize(t *testing.T) {
	m := newTestModel(t)

	m.at = cursor{entry: blockEntryFor(t, m, freshAnchor), comment: commentNone}
	if got, want := markTarget(m), "block"; got != want {
		t.Errorf("markTarget on a paragraph = %q, want %q", got, want)
	}

	m.at = cursor{entry: blockEntryFor(t, m, "^h1"), comment: commentNone}
	if got, want := markTarget(m), "section (3 blocks)"; got != want {
		t.Errorf("markTarget on a heading = %q, want %q", got, want)
	}

	m.at = cursor{entry: entryFor(t, m, convoAnchor), comment: commentNone}
	if got := markTarget(m); got != "" {
		t.Errorf("markTarget on a thread entry = %q, want \"\" (nothing markable there)", got)
	}
}
