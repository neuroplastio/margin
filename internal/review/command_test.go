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
