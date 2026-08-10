package state

import (
	"strings"
	"testing"
)

func TestIsStateDiagram(t *testing.T) {
	for _, src := range []string{
		"stateDiagram-v2\n[*] --> Idle",
		"stateDiagram\nIdle --> Active",
		"\n%% a comment\nstateDiagram-v2\n[*] --> Idle",
		"stateDiagram-v2\n    Idle --> [*]\n",
	} {
		if !IsStateDiagram(src) {
			t.Errorf("IsStateDiagram(%q) = false, want true", src)
		}
	}
	for _, src := range []string{
		"flowchart TD\nA --> B",
		"stateDiagramFoo\nA --> B",
		"",
	} {
		if IsStateDiagram(src) {
			t.Errorf("IsStateDiagram(%q) = true, want false", src)
		}
	}
}

func TestParseSimple(t *testing.T) {
	d, err := Parse("stateDiagram-v2\n[*] --> Idle\nIdle --> Active\nActive --> [*]")
	if err != nil {
		t.Fatal(err)
	}
	if len(d.Transitions) != 3 {
		t.Fatalf("got %d transitions, want 3", len(d.Transitions))
	}
	// States in first-seen order; [*] appears twice but is one state.
	if len(d.States) != 3 {
		t.Fatalf("got %d states, want 3: %v", len(d.States), stateIDs(d))
	}
	want := []string{"[*]", "Idle", "Active"}
	for i, id := range want {
		if d.States[i].ID != id {
			t.Errorf("state %d = %q, want %q", i, d.States[i].ID, id)
		}
	}
}

func TestParseLegacyKeyword(t *testing.T) {
	d, err := Parse("stateDiagram\nIdle --> Active")
	if err != nil {
		t.Fatal(err)
	}
	if len(d.Transitions) != 1 {
		t.Fatalf("got %d transitions, want 1", len(d.Transitions))
	}
}

func TestParseDeclarations(t *testing.T) {
	d, err := Parse(strings.Join([]string{
		"stateDiagram-v2",
		`state "Idle all day" as Idle`,
		`state "Booting"`,
		`state Running`,
		"Idle --> Booting: power on",
		"Booting --> Running",
	}, "\n"))
	if err != nil {
		t.Fatal(err)
	}
	byID := map[string]*State{}
	for _, s := range d.States {
		byID[s.ID] = s
	}
	if s := byID["Idle"]; s == nil || s.Label != "Idle all day" {
		t.Errorf("Idle label = %v, want %q", byID["Idle"], "Idle all day")
	}
	if s := byID["Booting"]; s == nil || s.Label != "Booting" {
		t.Errorf("Booting label = %v, want %q", byID["Booting"], "Booting")
	}
	if s := byID["Running"]; s == nil || s.Label != "Running" {
		t.Errorf("Running label = %v, want %q", byID["Running"], "Running")
	}
	if len(d.Transitions) != 2 || d.Transitions[0].Label != "power on" {
		t.Errorf("transitions = %+v, want 2 with label %q", d.Transitions, "power on")
	}
}

// TestParseDescriptionAlias: a transition may reference a state by its
// description rather than its short id; both resolve to the same box.
func TestParseDescriptionAlias(t *testing.T) {
	d, err := Parse("stateDiagram-v2\nstate \"Cold and dark\" as Cold\n\"Cold and dark\" --> Hot")
	if err != nil {
		t.Fatal(err)
	}
	if len(d.States) != 2 || d.States[0].Label != "Cold and dark" {
		t.Fatalf("states = %v, want the described state first", d.States)
	}
	if d.Transitions[0].From != d.States[0] {
		t.Error("transition From did not resolve to the described state")
	}
}

func TestParseAutoCreate(t *testing.T) {
	d, err := Parse("stateDiagram-v2\nA --> B\nB --> C")
	if err != nil {
		t.Fatal(err)
	}
	if len(d.States) != 3 {
		t.Fatalf("got %d states, want 3 (auto-created)", len(d.States))
	}
}

// TestParseRejectsComposite: `state Foo { … }` is beyond the supported subset;
// it must fail the whole diagram rather than render a half-parsed one.
func TestParseRejectsComposite(t *testing.T) {
	for _, src := range []string{
		"stateDiagram-v2\nstate Working {\n    Idle --> Busy\n}",
		"stateDiagram-v2\nstate Working { Idle --> Busy }",
	} {
		if _, err := Parse(src); err == nil {
			t.Errorf("Parse(%q) succeeded, want an error", src)
		}
	}
}

func TestParseErrors(t *testing.T) {
	for _, src := range []string{
		"flowchart TD\nA --> B",
		"stateDiagram-v2\nA --> ",
		"stateDiagram-v2\n--> B",
		"stateDiagram-v2\nnot a statement",
		"stateDiagram-v2\nstate",
		"stateDiagram-v2",
		"",
	} {
		if _, err := Parse(src); err == nil {
			t.Errorf("Parse(%q) succeeded, want an error", src)
		}
	}
}

func TestParseSkipsDirectionAndComments(t *testing.T) {
	d, err := Parse("stateDiagram-v2\ndirection LR\n%% a comment\nA --> B % inline\nC --> D")
	if err != nil {
		t.Fatal(err)
	}
	if len(d.Transitions) != 2 {
		t.Fatalf("got %d transitions, want 2", len(d.Transitions))
	}
}

func stateIDs(d *StateDiagram) []string {
	var ids []string
	for _, s := range d.States {
		ids = append(ids, s.ID)
	}
	return ids
}
