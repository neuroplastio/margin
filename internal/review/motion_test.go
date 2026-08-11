package review

import (
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
)

// TestMotionThrottleCoalescesReports pins the mouse-hover-lag fix: a pointer
// sweep delivers one motion report per cell, and Bubble Tea runs a full
// document render (Update then View) for every one, so an unthrottled sweep
// backs the input queue up and keystrokes wait for it to drain. The throttle
// caps motion processing at one report per frame period, and drops any report
// that restates the cell the pointer already occupies. The clock is injected
// so the coalescing is tested deterministically rather than against real
// scheduling.
func TestMotionThrottleCoalescesReports(t *testing.T) {
	now := time.Unix(0, 0)
	tk := &motionThrottle{period: time.Second / 120, now: func() time.Time { return now }}

	// The first report is let through.
	if tk.filter(tea.MouseMotionMsg{X: 1, Y: 1}) == nil {
		t.Fatal("first motion report was dropped")
	}

	// A report moments later at a new cell sits inside the frame window: the
	// renderer could never draw it, so it is dropped.
	now = now.Add(2 * time.Millisecond)
	if tk.filter(tea.MouseMotionMsg{X: 2, Y: 1}) != nil {
		t.Fatal("report within the frame period reached the model")
	}

	// A report restating the pointer's cell is dropped even after the window
	// has elapsed — nothing on screen would change.
	now = now.Add(time.Second)
	if tk.filter(tea.MouseMotionMsg{X: 1, Y: 1}) != nil {
		t.Fatal("report restating the pointer's cell reached the model")
	}

	// A report at a new cell after the window passes through.
	if tk.filter(tea.MouseMotionMsg{X: 3, Y: 1}) == nil {
		t.Fatal("report after the frame window was dropped")
	}

	// A report within the window but at the processed cell is dropped by the
	// position check, not the time check — the throttle never regresses to
	// re-rendering a cell it already painted.
	if tk.filter(tea.MouseMotionMsg{X: 3, Y: 1}) != nil {
		t.Fatal("same-cell report reached the model")
	}
}

// TestMotionThrottlePassesOtherMessages: the filter exists to throttle motion
// and nothing else — clicks, wheels and keys must pass through untouched, so
// a click, scroll or keypress is never delayed behind the hover queue.
func TestMotionThrottlePassesOtherMessages(t *testing.T) {
	tk := &motionThrottle{period: time.Second / 120, now: time.Now}

	for _, msg := range []tea.Msg{
		tea.MouseClickMsg{X: 1, Y: 1},
		tea.MouseWheelMsg{X: 1, Y: 1},
		tea.WindowSizeMsg{Width: 100, Height: 60},
		tea.KeyPressMsg{Code: 0},
	} {
		if got := tk.filter(msg); got != msg {
			t.Fatalf("filter(%T) returned %v, want the message untouched", msg, got)
		}
	}
}
