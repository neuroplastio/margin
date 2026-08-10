package graph

import (
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/elliotchance/orderedmap/v2"
	log "github.com/sirupsen/logrus"
)

type graphProperties struct {
	data             *orderedmap.OrderedMap[string, []textEdge]
	nodeSpecs        map[string]graphNodeSpec
	styleClasses     *map[string]styleClass
	boxBorderPadding int
	graphDirection   string
	styleType        string
	paddingX         int
	paddingY         int
	subgraphs        []*textSubgraph
	useAscii         bool
}

type textNode struct {
	name       string
	label      graphLabel
	hasLabel   bool
	styleClass string
}

type graphNodeSpec struct {
	label           graphLabel
	labelIsExplicit bool
	styleClass      string
}

type textEdge struct {
	parent          textNode
	child           textNode
	label           string
	isBidirectional bool
}

type textSubgraph struct {
	id       string
	name     string
	label    graphLabel
	nodes    []string
	parent   *textSubgraph
	children []*textSubgraph
}

func parseSubgraphHeader(header string) textSubgraph {
	trimmed := strings.TrimSpace(header)
	labelText := trimmed
	id := ""

	if match := regexp.MustCompile(`^(\S+)\s*\[(.+)\]$`).FindStringSubmatch(trimmed); match != nil {
		id = strings.TrimSpace(match[1])
		labelText = strings.TrimSpace(match[2])
		labelText = strings.Trim(labelText, `"`)
	}

	return textSubgraph{
		id:    id,
		name:  labelText,
		label: newGraphLabel(labelText),
		nodes: []string{},
	}
}

func splitGraphLines(mermaid string) []string {
	lines := []string{}
	var current strings.Builder
	bracketDepth := 0
	inQuotes := false

	for i := 0; i < len(mermaid); i++ {
		switch mermaid[i] {
		case '"':
			inQuotes = !inQuotes
		case '[':
			if !inQuotes {
				bracketDepth++
			}
		case ']':
			if !inQuotes && bracketDepth > 0 {
				bracketDepth--
			}
		case '\n':
			if bracketDepth == 0 {
				lines = append(lines, current.String())
				current.Reset()
				continue
			}
		case '\\':
			if i+1 < len(mermaid) && mermaid[i+1] == 'n' && bracketDepth == 0 {
				lines = append(lines, current.String())
				current.Reset()
				i++
				continue
			}
		}

		current.WriteByte(mermaid[i])
	}

	return append(lines, current.String())
}

func parseNode(line string) textNode {
	// Trim any whitespace from the line that might be left after comment removal
	trimmedLine := strings.TrimSpace(line)
	styleClass := ""
	if idx := strings.LastIndex(trimmedLine, ":::"); idx != -1 {
		styleClass = strings.TrimSpace(trimmedLine[idx+3:])
		trimmedLine = strings.TrimSpace(trimmedLine[:idx])
	}

	// Delta D4: node shapes. Upstream only recognised `id[text]`; every other
	// bracket family made the brackets part of the node's id, so `A((Start))`
	// and a later edge to plain `A` never matched. The shape families are
	// recognised here so the id is always the leading token and the label the
	// text inside the brackets.
	if node, ok := parseNodeLine(trimmedLine); ok {
		node.styleClass = styleClass
		return node
	}

	return textNode{name: trimmedLine, label: newGraphLabel(trimmedLine), styleClass: styleClass}
}

// nodeIDRE splits a node definition into its id and whatever follows it.
var nodeIDRE = regexp.MustCompile(`^([A-Za-z0-9_]+)\s*(.*)$`)

// nodeShapes are mermaid's node bracket families, longest openers first so
// `[[` wins over `[` and `((` over `(`.
var nodeShapes = []struct{ open, close_ string }{
	{"[[", "]]"},
	{"((", "))"},
	{"{{", "}}"},
	{"([", "])"},
	{"[(", ")]"},
	{">", "]"},
	{"[/", "/]"},
	{"[\\", "\\]"},
	{"[/", "\\]"},
	{"[\\", "/]"},
	{"{", "}"},
	{"[", "]"},
	{"(", ")"},
}

// parseNodeLine parses a node definition line: a bare id (`A`) or a shaped
// definition (`A[text]`, `A{text}`, `A((text))`, ...). ok is false for a line
// that is neither — the strict-parse delta (D3) rejects it so the caller can
// fall back to plain source rather than render a half-parsed diagram.
func parseNodeLine(s string) (textNode, bool) {
	s = strings.TrimSpace(s)
	m := nodeIDRE.FindStringSubmatch(s)
	if m == nil {
		return textNode{}, false
	}
	id, rest := m[1], strings.TrimSpace(m[2])
	if rest == "" {
		return textNode{name: id, label: newGraphLabel(id)}, true
	}
	for _, sh := range nodeShapes {
		if !strings.HasPrefix(rest, sh.open) {
			continue
		}
		inner := rest[len(sh.open):]
		idx := strings.LastIndex(inner, sh.close_)
		if idx < 0 {
			continue
		}
		if strings.TrimSpace(inner[idx+len(sh.close_):]) != "" {
			continue
		}
		label := strings.Trim(strings.TrimSpace(inner[:idx]), `"'`)
		if label == "" {
			label = id
		}
		return textNode{name: id, label: newGraphLabel(label), hasLabel: true}, true
	}
	return textNode{}, false
}

func parseStyleClass(matchedLine []string) styleClass {
	className := matchedLine[0]
	styles := matchedLine[1]
	// Styles are comma separated and key-values are separated by colon
	// Example: fill:#f9f,stroke:#333,stroke-width:4px
	styleMap := make(map[string]string)
	for _, style := range strings.Split(styles, ",") {
		kv := strings.Split(style, ":")
		styleMap[kv[0]] = kv[1]
	}
	return styleClass{className, styleMap}
}

func setArrowWithLabel(lhs, rhs []textNode, label string, isBidirectional bool, gp *graphProperties) []textNode {
	log.Debug("Setting arrow from ", lhs, " to ", rhs, " with label ", label)
	for _, l := range lhs {
		for _, r := range rhs {
			setData(l, textEdge{l, r, label, isBidirectional}, gp.data, gp.nodeSpecs)
		}
	}
	return rhs
}

func setArrow(lhs, rhs []textNode, gp *graphProperties) []textNode {
	return setArrowWithLabel(lhs, rhs, "", false, gp)
}

func setBidirectionalArrow(lhs, rhs []textNode, gp *graphProperties) []textNode {
	return setArrowWithLabel(lhs, rhs, "", true, gp)
}

func rememberNode(node textNode, nodeSpecs map[string]graphNodeSpec) {
	spec := nodeSpecs[node.name]
	if node.hasLabel || len(spec.label.lines) == 0 {
		spec.label = node.label
		spec.labelIsExplicit = node.hasLabel
	}
	if node.styleClass != "" {
		spec.styleClass = node.styleClass
	}
	nodeSpecs[node.name] = spec
}

func addNode(node textNode, data *orderedmap.OrderedMap[string, []textEdge], nodeSpecs map[string]graphNodeSpec) {
	rememberNode(node, nodeSpecs)
	if _, ok := data.Get(node.name); !ok {
		data.Set(node.name, []textEdge{})
	}
}

func setData(parent textNode, edge textEdge, data *orderedmap.OrderedMap[string, []textEdge], nodeSpecs map[string]graphNodeSpec) {
	rememberNode(parent, nodeSpecs)
	rememberNode(edge.child, nodeSpecs)
	// Check if the parent is in the map
	if children, ok := data.Get(parent.name); ok {
		// If it is, append the child to the list of children
		data.Set(parent.name, append(children, edge))
	} else {
		// If it isn't, add it to the map
		data.Set(parent.name, []textEdge{edge})
	}
	// Check if the child is in the map
	if _, ok := data.Get(edge.child.name); ok {
		// If it is, do nothing
	} else {
		// If it isn't, add it to the map
		data.Set(edge.child.name, []textEdge{})
	}
}

func (gp *graphProperties) parseString(line string) ([]textNode, error) {
	log.Debugf("Parsing line: %v", line)
	// Patterns are matched in order
	patterns := []struct {
		regex   *regexp.Regexp
		handler func([]string) ([]textNode, error)
	}{
		{
			regex: regexp.MustCompile(`^\s*$`),
			handler: func(match []string) ([]textNode, error) {
				// Ignore empty lines
				return []textNode{}, nil
			},
		},
		{
			// Delta D4: skip statements that carry no layout information
			// instead of letting them fall into the lenient node fallback.
			regex: regexp.MustCompile(`(?i)^\s*(?:direction|linkstyle|style|click)\b`),
			handler: func(match []string) ([]textNode, error) {
				return []textNode{}, nil
			},
		},
		{
			// Delta D4: the `-- text -->` between-label spelling, so `A -- text
			// --> B` reads as one labeled edge instead of a node named
			// "A -- text". Tried before the plain `-->` patterns below.
			regex: regexp.MustCompile(`(?s)^(.+?)\s*--\s*(.+?)\s*-->\s*(.+)$`),
			handler: func(match []string) ([]textNode, error) {
				return setArrowWithLabelOrFallback(gp, match, 1, false)
			},
		},
		{
			// Delta D4: the `== text ==>` between-label spelling.
			regex: regexp.MustCompile(`(?s)^(.+?)\s*==\s*(.+?)\s*==>\s*(.+)$`),
			handler: func(match []string) ([]textNode, error) {
				return setArrowWithLabelOrFallback(gp, match, 1, false)
			},
		},
		{
			// Delta D4: the `-. text .->` between-label spelling.
			regex: regexp.MustCompile(`(?s)^(.+?)\s*-\.\s*(.+?)\s*\.->\s*(.+)$`),
			handler: func(match []string) ([]textNode, error) {
				return setArrowWithLabelOrFallback(gp, match, 1, false)
			},
		},
		{
			regex: regexp.MustCompile(`(?s)^(.+)\s*<-->\s*\|(.+)\|\s*(.+)$`),
			handler: func(match []string) ([]textNode, error) {
				return setArrowWithLabelOrFallback(gp, match, 1, true)
			},
		},
		{
			regex: regexp.MustCompile(`(?s)^(.+)\s*<-->\s*(.+)$`),
			handler: func(match []string) ([]textNode, error) {
				lhs, err := gp.parseSegment(match[0])
				if err != nil {
					return nil, err
				}
				rhs, err := gp.parseSegment(match[1])
				if err != nil {
					return nil, err
				}
				return setBidirectionalArrow(lhs, rhs, gp), nil
			},
		},
		{
			regex: regexp.MustCompile(`(?s)^(.+)\s*-->\s*\|(.+)\|\s*(.+)$`),
			handler: func(match []string) ([]textNode, error) {
				return setArrowWithLabelOrFallback(gp, match, 1, false)
			},
		},
		{
			regex: regexp.MustCompile(`(?s)^(.+)\s*-->\s*(.+)$`),
			handler: func(match []string) ([]textNode, error) {
				return setArrowOrFallback(gp, match, 1)
			},
		},
		{
			// Delta D4: the remaining link forms — plain `---`, thick `==>`,
			// dotted `-.->`, their cross-head variants and unlabelled plain
			// forms. Upstream only recognised `-->` and `<-->`, so `A --- B`
			// used to collapse into one node named "A --- B".
			regex: regexp.MustCompile(`(?s)^(.+?)\s*---\s*\|(.+)\|\s*(.+)$`),
			handler: func(match []string) ([]textNode, error) {
				return setArrowWithLabelOrFallback(gp, match, 1, false)
			},
		},
		{
			regex: regexp.MustCompile(`(?s)^(.+?)\s*---\s*(.+)$`),
			handler: func(match []string) ([]textNode, error) {
				return setArrowOrFallback(gp, match, 1)
			},
		},
		{
			regex: regexp.MustCompile(`(?s)^(.+?)\s*==>\s*\|(.+)\|\s*(.+)$`),
			handler: func(match []string) ([]textNode, error) {
				return setArrowWithLabelOrFallback(gp, match, 1, false)
			},
		},
		{
			regex: regexp.MustCompile(`(?s)^(.+?)\s*==>\s*(.+)$`),
			handler: func(match []string) ([]textNode, error) {
				return setArrowOrFallback(gp, match, 1)
			},
		},
		{
			regex: regexp.MustCompile(`(?s)^(.+?)\s*===\s*(.+)$`),
			handler: func(match []string) ([]textNode, error) {
				return setArrowOrFallback(gp, match, 1)
			},
		},
		{
			regex: regexp.MustCompile(`(?s)^(.+?)\s*-\.->\s*\|(.+)\|\s*(.+)$`),
			handler: func(match []string) ([]textNode, error) {
				return setArrowWithLabelOrFallback(gp, match, 1, false)
			},
		},
		{
			regex: regexp.MustCompile(`(?s)^(.+?)\s*-\.->\s*(.+)$`),
			handler: func(match []string) ([]textNode, error) {
				return setArrowOrFallback(gp, match, 1)
			},
		},
		{
			regex: regexp.MustCompile(`(?s)^(.+?)\s*-\.-\s*(.+)$`),
			handler: func(match []string) ([]textNode, error) {
				return setArrowOrFallback(gp, match, 1)
			},
		},
		{
			regex: regexp.MustCompile(`(?s)^(.+?)\s*--x\s*(.+)$`),
			handler: func(match []string) ([]textNode, error) {
				return setArrowOrFallback(gp, match, 1)
			},
		},
		{
			regex: regexp.MustCompile(`(?s)^(.+?)\s*==x\s*(.+)$`),
			handler: func(match []string) ([]textNode, error) {
				return setArrowOrFallback(gp, match, 1)
			},
		},
		{
			regex: regexp.MustCompile(`(?s)^(.+?)\s*-\.x\s*(.+)$`),
			handler: func(match []string) ([]textNode, error) {
				return setArrowOrFallback(gp, match, 1)
			},
		},
		{
			regex: regexp.MustCompile(`^classDef\s+(.+)\s+(.+)$`),
			handler: func(match []string) ([]textNode, error) {
				s := parseStyleClass(match)
				(*gp.styleClasses)[s.name] = s
				return []textNode{}, nil
			},
		},
		{
			regex: regexp.MustCompile(`(?s)^(.+) & (.+)$`),
			handler: func(match []string) ([]textNode, error) {
				lhs, err := gp.parseSegment(match[0])
				if err != nil {
					return nil, err
				}
				rhs, err := gp.parseSegment(match[1])
				if err != nil {
					return nil, err
				}
				return append(lhs, rhs...), nil
			},
		},
		{
			// Delta D3: a bare or shaped node definition (`A`, `A[text]`,
			// `A{text}`) with no link on the line. Anything else here is
			// unparseable and errors, so the whole diagram falls back to
			// plain source rather than rendering the line as a bogus node.
			regex: regexp.MustCompile(`(?s)^(.+)$`),
			handler: func(match []string) ([]textNode, error) {
				if node, ok := parseNodeLine(match[0]); ok {
					return []textNode{node}, nil
				}
				return nil, fmt.Errorf("could not parse line: %s", match[0])
			},
		},
	}
	for _, pattern := range patterns {
		if match := pattern.regex.FindStringSubmatch(line); match != nil {
			nodes, err := pattern.handler(match[1:])
			if err == nil {
				return nodes, nil
			}
		}
	}
	return []textNode{}, errors.New("Could not parse line: " + line)
}

// parseSegment parses one side of a link (`A[Start]`, a bare `A`, or a
// chained remainder like `B --> C`). When the side is a single node it is
// accepted as a definition; anything else errors, so the whole diagram fails
// instead of drawing a bogus node (delta D3).
func (gp *graphProperties) parseSegment(s string) ([]textNode, error) {
	nodes, err := gp.parseString(s)
	if err == nil {
		return nodes, nil
	}
	if n, ok := parseNodeLine(s); ok {
		return []textNode{n}, nil
	}
	return nil, err
}

// setArrowWithLabelOrFallback and setArrowOrFallback (delta D4) build an edge
// from a regex match, recursing into the lhs and rhs segments. An unparseable
// segment fails the edge (and so the whole diagram) rather than drawing a
// bogus node.
func setArrowWithLabelOrFallback(gp *graphProperties, match []string, labelIdx int, bidir bool) ([]textNode, error) {
	lhs, err := gp.parseSegment(match[0])
	if err != nil {
		return nil, err
	}
	rhs, err := gp.parseSegment(match[2])
	if err != nil {
		return nil, err
	}
	return setArrowWithLabel(lhs, rhs, match[labelIdx], bidir, gp), nil
}

func setArrowOrFallback(gp *graphProperties, match []string, rhsIdx int) ([]textNode, error) {
	lhs, err := gp.parseSegment(match[0])
	if err != nil {
		return nil, err
	}
	rhs, err := gp.parseSegment(match[rhsIdx])
	if err != nil {
		return nil, err
	}
	return setArrow(lhs, rhs, gp), nil
}

func mermaidFileToMap(mermaid, styleType string) (*graphProperties, error) {
	rawLines := splitGraphLines(mermaid)

	// Process lines to remove comments
	lines := []string{}
	for _, line := range rawLines {
		// Stop processing at "---" separator (used in test files)
		if line == "---" {
			break
		}

		// Skip lines that start with %% (comment lines)
		if strings.HasPrefix(strings.TrimSpace(line), "%%") {
			continue
		}

		// Remove inline comments (anything after %%) and trim resulting whitespace
		if idx := strings.Index(line, "%%"); idx != -1 {
			line = strings.TrimSpace(line[:idx])
		}

		// Skip empty lines after comment removal
		if len(strings.TrimSpace(line)) > 0 {
			lines = append(lines, line)
		}
	}

	data := orderedmap.NewOrderedMap[string, []textEdge]()
	styleClasses := make(map[string]styleClass)
	properties := graphProperties{
		data:             data,
		nodeSpecs:        make(map[string]graphNodeSpec),
		styleClasses:     &styleClasses,
		boxBorderPadding: boxBorderPadding,
		graphDirection:   "",
		styleType:        styleType,
		paddingX:         paddingBetweenX,
		paddingY:         paddingBetweenY,
		subgraphs:        []*textSubgraph{},
	}

	// Pick up optional padding directives before the graph definition
	paddingRegex := regexp.MustCompile(`^(?i)padding([xy])\s*=\s*(\d+)$`)
	for len(lines) > 0 {
		trimmed := strings.TrimSpace(lines[0])
		if trimmed == "" {
			lines = lines[1:]
			continue
		}
		if match := paddingRegex.FindStringSubmatch(trimmed); match != nil {
			paddingValue, err := strconv.Atoi(match[2])
			if err != nil {
				return &properties, err
			}
			if strings.EqualFold(match[1], "x") {
				properties.paddingX = paddingValue
			} else {
				properties.paddingY = paddingValue
			}
			lines = lines[1:]
			continue
		}
		break
	}

	if len(lines) == 0 {
		return &properties, errors.New("missing graph definition")
	}

	// The first line declares the diagram: "graph" or "flowchart" followed by an
	// optional direction (e.g. "flowchart LR", "graph TD", or a bare "graph").
	// strings.Fields collapses any surrounding or repeated whitespace, so
	// indented or trailing-padded declarations parse correctly; TrimRight drops a
	// trailing separator (mermaid allows "graph TD;").
	fields := strings.Fields(strings.TrimRight(lines[0], "; \t\r"))
	if len(fields) == 0 || (fields[0] != "graph" && fields[0] != "flowchart") {
		return &properties, fmt.Errorf("unsupported graph type '%s'. Supported types: 'graph' or 'flowchart' with an optional direction (TD, TB, BT, LR, RL)", strings.TrimSpace(lines[0]))
	}
	if len(fields) > 2 {
		return &properties, fmt.Errorf("unexpected tokens after graph direction: %q", strings.Join(fields[2:], " "))
	}

	// Mermaid defaults to top-down when no direction is given. The renderer only
	// lays out along the horizontal (LR) or vertical (TD) axis; the reverse
	// directions RL and BT are accepted but drawn on their axis without the
	// reversal (RL renders left-to-right, BT top-down).
	properties.graphDirection = "TD"
	if len(fields) == 2 {
		switch fields[1] {
		case "LR", "RL":
			properties.graphDirection = "LR"
		case "TD", "TB", "BT":
			properties.graphDirection = "TD"
		default:
			return &properties, fmt.Errorf("unsupported graph direction '%s'. Supported directions: TD, TB, BT, LR, RL", fields[1])
		}
	}
	lines = lines[1:]

	// Track subgraph context using a stack
	subgraphStack := []*textSubgraph{}
	subgraphRegex := regexp.MustCompile(`^\s*subgraph\s+(.+)$`)
	endRegex := regexp.MustCompile(`^\s*end\s*$`)

	// Iterate over the lines
	for _, line := range lines {
		trimmedLine := strings.TrimSpace(line)

		// Check for subgraph start
		if match := subgraphRegex.FindStringSubmatch(trimmedLine); match != nil {
			header := parseSubgraphHeader(match[1])
			newSubgraph := &textSubgraph{
				id:       header.id,
				name:     header.name,
				label:    header.label,
				nodes:    []string{},
				children: []*textSubgraph{},
			}

			// Set parent relationship if we're nested
			if len(subgraphStack) > 0 {
				parent := subgraphStack[len(subgraphStack)-1]
				newSubgraph.parent = parent
				parent.children = append(parent.children, newSubgraph)
			}

			subgraphStack = append(subgraphStack, newSubgraph)
			properties.subgraphs = append(properties.subgraphs, newSubgraph)
			log.Debugf("Started subgraph %s", newSubgraph.name)
			continue
		}

		// Check for subgraph end
		if endRegex.MatchString(trimmedLine) {
			if len(subgraphStack) > 0 {
				closedSubgraph := subgraphStack[len(subgraphStack)-1]
				subgraphStack = subgraphStack[:len(subgraphStack)-1]
				log.Debugf("Ended subgraph %s", closedSubgraph.name)
			}
			continue
		}

		// Remember nodes before parsing this line
		existingNodes := make(map[string]bool)
		for el := data.Front(); el != nil; el = el.Next() {
			existingNodes[el.Key] = true
		}

		// Parse nodes and edges normally. Delta D3: upstream swallowed an
		// unparseable line by turning the whole line into a node, which drew
		// "A --- B" or "foo bar" as a labelled box — a half-parsed diagram.
		// A statement the parser does not understand now fails the whole
		// document, so margin can fall back to the block's plain source.
		nodes, err := properties.parseString(line)
		if err != nil {
			return &properties, fmt.Errorf("%s (line %q)", err, strings.TrimSpace(line))
		}
		// Ensure all returned nodes are in the map
		for _, node := range nodes {
			addNode(node, properties.data, properties.nodeSpecs)
		}

		// Add all new nodes to current subgraph(s)
		if len(subgraphStack) > 0 {
			for el := data.Front(); el != nil; el = el.Next() {
				nodeName := el.Key
				// If this is a new node (wasn't in existingNodes), add it to subgraph
				if !existingNodes[nodeName] {
					for _, sg := range subgraphStack {
						// Check if node is not already in the subgraph
						found := false
						for _, n := range sg.nodes {
							if n == nodeName {
								found = true
								break
							}
						}
						if !found {
							sg.nodes = append(sg.nodes, nodeName)
							log.Debugf("Added node %s to subgraph %s", nodeName, sg.name)
						}
					}
				}
			}
		}
	}
	return &properties, nil
}
