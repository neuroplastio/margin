package review

import _ "embed"

// skillDocument is the markdown document `margin skill` prints — the whole
// contract an agent needs to take part in an interactive review: the four CLI
// commands, the loop that binds them (launch the review in a terminal, poll the
// event log for the human's comments, reply through comment add, let the thread
// watcher carry the reply live), and the on-disk shapes the loop depends on —
// the event log, thread files, and anchors. Kept as a file (rather than a
// string literal) so the prose stays readable to whoever is judging it.
//
//go:embed skill.md
var skillDocument string

// SkillDocument returns the skill document an agent loads to learn how to use
// margin.
func SkillDocument() string { return skillDocument }
