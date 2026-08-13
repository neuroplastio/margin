package review

import (
	"strings"
	"testing"
)

// The essential requirement the feedback names: an agent reading the skill
// document can run the interactive review loop end to end. These tests pin the
// loop's steps and the contracts behind them — prose changes freely, but a step
// that stops being taught is a regression.
func TestSkillSpellsTheInteractiveLoopEndToEnd(t *testing.T) {
	doc := SkillDocument()

	// Launch, read, wait, reply — in that order, so the loop reads as a loop.
	steps := []string{
		"Launch the review",
		"Read the starting state",
		"Wait for the human",
		"Reply",
		"Loop",
	}
	last := 0
	for _, step := range steps {
		i := strings.Index(doc, step)
		if i < 0 {
			t.Errorf("skill does not teach the %q step:\n%s", step, doc)
			continue
		}
		if i < last {
			t.Errorf("skill presents the %q step before an earlier one — the loop must read in order", step)
		}
		last = i
	}

	// Every building block command, spelled as the feedback names it.
	for _, cmd := range []string{
		"margin FILE.md",
		"margin export FILE.md",
		"margin comments wait --since ID --timeout 60s",
		`margin comment add FILE.md --anchor ID --text "..." --author agent`,
		"margin import-pr FILE.md",
	} {
		if !strings.Contains(doc, cmd) {
			t.Errorf("skill does not teach the command %q:\n%s", cmd, doc)
		}
	}
}

func TestSkillTeachesTheWaitExitContract(t *testing.T) {
	doc := SkillDocument()
	// The three outcomes of `comments wait` — new events, a quiet timeout, a
	// loud error — are the poll discipline; an agent that cannot tell timeout
	// from error will treat a broken log as "nothing yet" forever.
	if !strings.Contains(doc, "Exit 0") || !strings.Contains(doc, "Exit 1") {
		t.Errorf("skill does not spell out the wait exit contract:\n%s", doc)
	}
	if !strings.Contains(doc, "stderr") {
		t.Errorf("skill does not teach how to tell timeout from error (stderr):\n%s", doc)
	}
}

// The feedback's essential requirement: an agent participating live (replying
// as comments land) also has a definitive "the review is done" signal. The
// taught recipe is to run the launch and the poll as two parallel jobs — launch
// with --stdout chained to a sentinel, take the launch's completion as the
// done-signal, poll with comments wait in parallel — and stop when it fires.
func TestSkillTeachesTheDoneSignal(t *testing.T) {
	doc := SkillDocument()
	if !strings.Contains(doc, "When the review is done") {
		t.Errorf("skill has no done-signal section:\n%s", doc)
	}
	for _, want := range []string{
		"--stdout",
		"sentinel",
		"backgrounded",
		"in parallel",
		"done-signal",
	} {
		if !strings.Contains(doc, want) {
			t.Errorf("skill does not teach the %q done-signal element:\n%s", want, doc)
		}
	}
}

func TestSkillTeachesTheLiveUpdateHalf(t *testing.T) {
	doc := SkillDocument()
	// "The human sees the reply live" is the point of the loop: the thread
	// watcher reloads on disk changes, and the event log is what the agent
	// tails. Both halves must be in the document.
	if !strings.Contains(doc, "watcher") || !strings.Contains(doc, "live") {
		t.Errorf("skill does not teach that replies reach the human live via the watcher:\n%s", doc)
	}
	if !strings.Contains(doc, "events.log") || !strings.Contains(doc, "append-only") {
		t.Errorf("skill does not teach the event log contract:\n%s", doc)
	}
}

func TestSkillIsWellFormedMarkdown(t *testing.T) {
	doc := SkillDocument()
	if !strings.HasPrefix(doc, "# ") {
		t.Errorf("skill document does not open with a level-1 heading:\n%s", doc)
	}
	if !strings.HasSuffix(doc, "\n") {
		t.Error("skill document does not end in a newline")
	}
	if !strings.Contains(doc, "## The interactive review loop") {
		t.Errorf("skill document has no interactive-loop section:\n%s", doc)
	}
}

// TestSkillNamesTheGutterGlyphs: the 2026-08-10 agent-loop feedback named
// `margin skill` as one of the places where what a gutter icon means is not
// obvious — an agent asked to describe the screen got it wrong on the first
// try. The skill now has a "Reading the human's screen" section naming each
// glyph and pointing at --help as the same legend.
func TestSkillNamesTheGutterGlyphs(t *testing.T) {
	doc := SkillDocument()
	if !strings.Contains(doc, "Reading the human's screen") {
		t.Errorf("skill has no screen-reading section:\n%s", doc)
	}
	for _, glyph := range []string{"▌", "│", "·", "▸", "✓"} {
		if !strings.Contains(doc, glyph) {
			t.Errorf("skill's screen legend does not name %q:\n%s", glyph, doc)
		}
	}
}
