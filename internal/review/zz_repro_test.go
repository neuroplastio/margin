package review

import (
	"strings"
	"testing"
	"time"
)

func TestReproComposerWrapping(t *testing.T) {
	requireNvim(t)
	m := newTestModel(t)
	m.w, m.h = 100, 60
	i := blockEntryFor(t, m, freshAnchor)
	m.at = cursor{entry: i, comment: commentNone}
	if cmd := m.openComposer(freshAnchor, newCommentSlot); cmd == nil {
		t.Fatalf("open failed: %s", m.status)
	}
	t.Cleanup(func() {
		if m.comp != nil {
			m.comp.close()
		}
	})
	if !waitForMode(t, m.comp, "INSERT", 10*time.Second) {
		t.Fatalf("never INSERT; screen:\n%s", plainScreen(m.comp.em))
	}
	// Long prose line, the kind the feedback describes.
	long := "This is a fairly long sentence that should wrap neatly at word boundaries within the composer pane, and the cursor should track the text as it is typed."
	typeText(m.comp, long)
	time.Sleep(300 * time.Millisecond)
	screen := plainScreen(m.comp.em)
	for i, l := range strings.Split(screen, "\n") {
		t.Logf("%02d: [%s]", i, l)
	}
	cp := m.comp.em.CursorPosition()
	t.Logf("emulator size=%dx%d cursor=%d,%d", m.comp.em.Width(), m.comp.em.Height(), cp.X, cp.Y)
	// last typed char should be near the cursor
	last := rune(long[len(long)-1])
	screenLines := strings.Split(screen, "\n")
	if cp.Y < len(screenLines) && strings.IndexRune(screenLines[cp.Y], last) < 0 {
		// cursor row doesn't contain the last char - misalignment
		t.Logf("WARN: cursor row %d does not show last typed rune %q", cp.Y, string(last))
	}
}
