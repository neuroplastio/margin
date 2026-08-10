// Package state parses and renders mermaid state diagrams (`stateDiagram` and
// `stateDiagram-v2`) as ASCII: state boxes connected by labelled transition
// arrows, with `[*]` start/end markers.
//
// This package is a margin local delta (CHANGELOG.md, delta D7): upstream
// mermaid-ascii has no state renderer, so this is the in-tree extension the
// vendoring leg promised. It follows the other packages' shape — a `Keyword`
// constant, `IsStateDiagram`, `Parse` and `Render` against a *diagram.Config —
// so the extension is upstreamable.
package state

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/neuroplastio/margin/third_party/mermaid-ascii/pkg/diagram"
)

// Keyword is the stateDiagram-v2 declaration keyword. The legacy spelling
// `stateDiagram` (v1) is also accepted; IsStateDiagram routes on both.
const Keyword = "stateDiagram-v2"

// StartEndID is the pseudo-state id mermaid uses for the start and end
// markers; it draws as a circle rather than a box.
const StartEndID = "[*]"

// State is a named state box. ID is what transitions reference; Label is what
// the box draws (a description if one was given, otherwise the id).
type State struct {
	ID    string
	Label string
}

// Transition connects two states with an optional label.
type Transition struct {
	From  *State
	To    *State
	Label string
}

// StateDiagram is a parsed state diagram. States are kept in first-seen order;
// a transition referencing an undeclared state auto-creates it.
type StateDiagram struct {
	States      []*State
	Transitions []*Transition
	byID        map[string]*State
}

var (
	// stateDiagramHeaders are the two header spellings, longest first so
	// `stateDiagram-v2` is never read as `stateDiagram`.
	stateDiagramHeaders = []string{"statediagram-v2", "statediagram"}

	// directionRegex matches a layout directive (`direction TB|LR|...`), which
	// carries no ASCII meaning; it's skipped so a stray one doesn't fail the
	// diagram.
	directionRegex = regexp.MustCompile(`(?i)^\s*direction\s+\S+\s*$`)

	// asRegex splits `state "Long description" as Short` at the LAST " as "
	// (the id is a single non-space token), so a description containing " as "
	// survives: the greedy `.+` reaches the final " as ".
	asRegex = regexp.MustCompile(`(?i)^\s*(.+)\s+as\s+(\S+)\s*$`)
)

// IsStateDiagram reports whether the input's first meaningful line declares a
// state diagram (either header spelling, case-insensitive, whole token).
func IsStateDiagram(input string) bool {
	for _, line := range strings.Split(input, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "%%") {
			continue
		}
		return isStateDiagramHeader(trimmed)
	}
	return false
}

func isStateDiagramHeader(line string) bool {
	lower := strings.ToLower(line)
	for _, kw := range stateDiagramHeaders {
		if lower == kw || strings.HasPrefix(lower, kw+" ") || strings.HasPrefix(lower, kw+"\t") {
			return true
		}
	}
	return false
}

// Parse parses a stateDiagram / stateDiagram-v2 document. The parser is
// strict: any statement it does not understand — composite states, notes,
// anything outside the transition/declaration subset — fails the whole
// document, so the caller can fall back to plain source instead of rendering a
// half-parsed diagram.
func Parse(input string) (*StateDiagram, error) {
	input = strings.TrimSpace(input)
	if input == "" {
		return nil, fmt.Errorf("empty input")
	}
	lines := diagram.RemoveComments(diagram.SplitLines(input))
	if len(lines) == 0 {
		return nil, fmt.Errorf("no content found")
	}
	if !isStateDiagramHeader(strings.TrimSpace(lines[0])) {
		return nil, fmt.Errorf("expected %q keyword", Keyword)
	}

	d := &StateDiagram{byID: map[string]*State{}}
	for i, line := range lines[1:] {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || directionRegex.MatchString(trimmed) {
			continue
		}

		// A state declaration: `state Idle`, `state "Idle"`, or
		// `state "Idle all day" as Idle`. Checked before transitions, which a
		// declaration line never contains.
		if strings.EqualFold(trimmed, "state") || strings.HasPrefix(strings.ToLower(trimmed), "state ") {
			id, label, err := parseStateDecl(trimmed)
			if err != nil {
				return nil, fmt.Errorf("line %d: %w", i+2, err)
			}
			d.declare(id, label)
			continue
		}

		if ok, err := d.parseTransition(trimmed); ok {
			if err != nil {
				return nil, fmt.Errorf("line %d: %w", i+2, err)
			}
			continue
		}

		return nil, fmt.Errorf("line %d: invalid syntax: %q", i+2, trimmed)
	}

	if len(d.States) == 0 && len(d.Transitions) == 0 {
		return nil, fmt.Errorf("no states found")
	}
	return d, nil
}

// parseStateDecl splits the body of a `state ...` declaration into its id and
// display label. Composite states (`state X { … }`) are rejected: drawing a
// boxed sub-diagram is beyond the current subset, and a half-ignored one is
// worse than falling back to plain source.
func parseStateDecl(line string) (id, label string, err error) {
	rest := strings.TrimSpace(strings.TrimSpace(line[len("state"):]))
	if rest == "" {
		return "", "", fmt.Errorf("state declaration needs a name: %q", line)
	}
	if strings.Contains(rest, "{") || strings.Contains(rest, "}") {
		return "", "", fmt.Errorf("composite states are not supported yet: %q", line)
	}
	if m := asRegex.FindStringSubmatch(rest); m != nil {
		label = strings.Trim(strings.TrimSpace(m[1]), `"`)
		id = strings.Trim(m[2], `"`)
		if id == "" {
			return "", "", fmt.Errorf("state alias is empty: %q", line)
		}
		return id, label, nil
	}
	label = strings.Trim(rest, `"`)
	if label == "" {
		return "", "", fmt.Errorf("invalid state declaration %q", line)
	}
	return label, label, nil
}

// parseTransition parses `From --> To` and `From --> To: label`. ok is false
// when the line carries no `-->` arrow, so parsing can fall through to the
// other statement forms.
func (d *StateDiagram) parseTransition(line string) (bool, error) {
	idx := strings.Index(line, "-->")
	if idx < 0 {
		return false, nil
	}
	from := strings.TrimSpace(line[:idx])
	rest := strings.TrimSpace(line[idx+3:])
	to := rest
	label := ""
	if i := strings.Index(rest, ":"); i >= 0 {
		to = strings.TrimSpace(rest[:i])
		label = strings.TrimSpace(rest[i+1:])
	}
	if from == "" || to == "" {
		return true, fmt.Errorf("transition needs a source and target: %q", line)
	}
	d.Transitions = append(d.Transitions, &Transition{
		From:  d.state(from),
		To:    d.state(to),
		Label: label,
	})
	return true, nil
}

// state returns the state with the given id, creating it (labelled with the
// id) if it does not exist. Quotes are stripped so a transition endpoint
// `"Cold"` resolves to the same state a declaration registered.
func (d *StateDiagram) state(id string) *State {
	id = strings.Trim(id, `"`)
	if s, ok := d.byID[id]; ok {
		return s
	}
	s := &State{ID: id, Label: id}
	d.byID[id] = s
	d.States = append(d.States, s)
	return s
}

// declare registers a state from a `state ...` declaration. The description is
// registered as an alias for the id, so a transition that references a state
// by its description (`"Long description" --> X`) resolves to the same box.
func (d *StateDiagram) declare(id, label string) *State {
	s := d.state(id)
	s.Label = label
	if label != id {
		if _, taken := d.byID[label]; !taken {
			d.byID[label] = s
		}
	}
	return s
}
