package review

import (
	"context"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	uv "github.com/charmbracelet/ultraviolet"
	"github.com/charmbracelet/x/vt"
)

// sendKeyBytes runs a key through sendKey against a bare emulator — no child
// — and returns the bytes the child would have received. The emulator's
// output pipe is what the pty pump copies verbatim to nvim.
func sendKeyBytes(t *testing.T, k tea.Key) string {
	t.Helper()
	c := &composer{em: vt.NewSafeEmulator(80, 24)}
	// The emulator's output is a synchronous io.Pipe: sendKey's SendText
	// blocks until a reader takes the bytes, so the read has to start first.
	type result struct {
		b   []byte
		err error
	}
	ch := make(chan result, 1)
	go func() {
		buf := make([]byte, 64)
		n, err := c.em.Read(buf)
		ch <- result{buf[:n], err}
	}()
	c.sendKey(k)
	select {
	case r := <-ch:
		if r.err != nil {
			t.Fatalf("reading emulator output: %v", r.err)
		}
		return string(r.b)
	case <-time.After(2 * time.Second):
		t.Fatalf("sendKey(%+v) produced no output", k)
		return ""
	}
}

// TestSendKeyEmitsBytes pins the exact bytes every key family leaves the host
// as. This is the layer the feedback's bug lived in: the composer's job is to
// hand nvim bytes it decodes as the pressed key, and each encoding asserted
// here was fed to a real nvim and confirmed to decode as intended (journal
// 2026-08-09.19). Byte-level pinning catches a regression whether or not a
// child nvim is installed.
func TestSendKeyEmitsBytes(t *testing.T) {
	cases := []struct {
		name string
		key  tea.Key
		want string
	}{
		// Unmodified keys keep whatever vt.SendKey emits.
		{"plain x", tea.Key{Code: 'x', Text: "x"}, "x"},
		{"shifted O", tea.Key{Code: 'o', Mod: uv.ModShift, Text: "O"}, "O"},
		{"backspace", tea.Key{Code: uv.KeyBackspace}, "\x7f"},
		{"enter", tea.Key{Code: uv.KeyEnter}, "\r"},

		// The host's one intercept.
		{"ctrl+enter submits", tea.Key{Code: uv.KeyEnter, Mod: uv.ModCtrl}, "\x1b:MarginSubmit\r"},

		// ctrl-only aliases keep the legacy control bytes vt encodes, so
		// ctrl+m stays CR and ctrl+[ stays ESC.
		{"ctrl+a", tea.Key{Code: 'a', Mod: uv.ModCtrl}, "\x01"},
		{"ctrl+m", tea.Key{Code: 'm', Mod: uv.ModCtrl}, "\x0d"},
		{"ctrl+[", tea.Key{Code: '[', Mod: uv.ModCtrl}, "\x1b"},

		// The dropped class from the feedback: keys vt's fallback emitted
		// nothing for.
		{"ctrl+backspace", tea.Key{Code: uv.KeyBackspace, Mod: uv.ModCtrl}, "\x1b[127;5u"},
		{"shift+backspace", tea.Key{Code: uv.KeyBackspace, Mod: uv.ModShift}, "\x1b[127;2u"},
		{"shift+enter", tea.Key{Code: uv.KeyEnter, Mod: uv.ModShift}, "\x1b[13;2u"},
		{"ctrl+shift+a", tea.Key{Code: 'a', Mod: uv.ModCtrl | uv.ModShift}, "\x1b[97;6u"},
		{"ctrl+tab", tea.Key{Code: uv.KeyTab, Mod: uv.ModCtrl}, "\x1b[9;5u"},
		{"ctrl+1", tea.Key{Code: '1', Mod: uv.ModCtrl}, "\x1b[49;5u"},
		{"ctrl+up", tea.Key{Code: uv.KeyUp, Mod: uv.ModCtrl}, "\x1b[1;5A"},
		{"shift+ctrl+home", tea.Key{Code: uv.KeyHome, Mod: uv.ModShift | uv.ModCtrl}, "\x1b[1;6H"},
		{"ctrl+delete", tea.Key{Code: uv.KeyDelete, Mod: uv.ModCtrl}, "\x1b[3;5~"},
		{"shift+pgup", tea.Key{Code: uv.KeyPgUp, Mod: uv.ModShift}, "\x1b[5;2~"},
		{"ctrl+f1", tea.Key{Code: uv.KeyF1, Mod: uv.ModCtrl}, "\x1b[1;5P"},

		// Alt combos: legacy meta form. vt already encoded alt-only keys this
		// way; the bug was alt plus a second modifier collapsing to a bare
		// ESC — which nvim reads as Escape and leaves insert mode. None of
		// these may be a lone "\x1b".
		{"alt+x", tea.Key{Code: 'x', Mod: uv.ModAlt}, "\x1bx"},
		{"alt+backspace", tea.Key{Code: uv.KeyBackspace, Mod: uv.ModAlt}, "\x1b\x7f"},
		{"alt+shift+x", tea.Key{Code: 'x', Mod: uv.ModAlt | uv.ModShift}, "\x1bX"},
		{"alt+shift+x shifted code", tea.Key{Code: 'x', Mod: uv.ModAlt | uv.ModShift, ShiftedCode: 'X'}, "\x1bX"},
		{"alt+ctrl+x", tea.Key{Code: 'x', Mod: uv.ModAlt | uv.ModCtrl}, "\x1b\x18"},
		{"alt+delete", tea.Key{Code: uv.KeyDelete, Mod: uv.ModAlt}, "\x1b\x1b[3~"},
		{"alt+up", tea.Key{Code: uv.KeyUp, Mod: uv.ModAlt}, "\x1b\x1b[A"},
		{"alt+ctrl+backspace", tea.Key{Code: uv.KeyBackspace, Mod: uv.ModAlt | uv.ModCtrl}, "\x1b\x1b[127;5u"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := sendKeyBytes(t, tc.key); got != tc.want {
				t.Errorf("sendKey(%+v) emitted %q, want %q", tc.key, got, tc.want)
			}
		})
	}
}

// waitForScreenWithout polls until want is present and unwanted is gone,
// returning the last screen seen. Used when the assertion is an absence —
// deleted text may linger on screen for a frame or two.
func waitForScreenWithout(t *testing.T, c *composer, want, unwanted string, timeout time.Duration) string {
	t.Helper()
	deadline := time.Now().Add(timeout)
	var screen string
	for time.Now().Before(deadline) {
		screen = plainScreen(c.em)
		if strings.Contains(screen, want) && !strings.Contains(screen, unwanted) {
			return screen
		}
		time.Sleep(20 * time.Millisecond)
	}
	return screen
}

// deleteWordBeforeCursor types "hello world", sends key, and demands the
// buffer read "hello" afterwards.
func deleteWordBeforeCursor(t *testing.T, key tea.Key) {
	t.Helper()
	m := newTestModel(t)
	open(t, m, freshAnchor, newCommentSlot, "")
	typeText(m.comp, "hello world")
	if s := waitForScreen(t, m.comp.em, "world", 3*time.Second); !strings.Contains(s, "hello world") {
		t.Fatalf("seed text never landed; screen was:\n%s", s)
	}

	typeKeys(m.comp, key)
	if s := waitForScreenWithout(t, m.comp, "hello", "world", 3*time.Second); strings.Contains(s, "world") {
		t.Fatalf("%v did not delete the word; screen was:\n%s", key, s)
	}

	typeKeys(m.comp, keyCtrlS)
	if got := outcomeFromExit(waitExit(t, m.comp, 10*time.Second)); got != outcomeSubmit {
		t.Fatalf("ctrl+s gave outcome %v, want submit", got)
	}
	if body := m.comp.body(); body != "hello" {
		t.Fatalf("body = %q, want %q", body, "hello")
	}
}

// TestCtrlBackspaceDeletesWord is the end-to-end regression for the feedback's
// "vim mode doesn't receive ctrl+backspace": the key must reach nvim as
// <C-BS>, and the composer's <C-BS> → <C-W> mapping must delete the word
// before the cursor.
func TestCtrlBackspaceDeletesWord(t *testing.T) {
	deleteWordBeforeCursor(t, tea.Key{Code: uv.KeyBackspace, Mod: uv.ModCtrl})
}

// TestAltBackspaceDeletesWord covers the readline/macOS delete-word combo.
// It is also the regression for "dropping into normal mode unexpectedly":
// alt+backspace reaches nvim as ESC DEL, and only the composer's <M-BS>
// mapping keeps an unbound meta combo from collapsing into a bare Escape.
func TestAltBackspaceDeletesWord(t *testing.T) {
	deleteWordBeforeCursor(t, tea.Key{Code: uv.KeyBackspace, Mod: uv.ModAlt})
}

// TestComposerPaintsNoBackground pins the "vim mode displays a strange black
// background" fix (additional-ux-and-cli feedback): nvim's default colorscheme
// paints Normal with its own dark background (NvimDarkGrey2, RGB 20;22;27),
// so the emulator's Render() decorated every cell — typed text, empty rows,
// and erased cells alike — with a background SGR that clashed with margin's
// themed background and left artifacts on deleted character cells. The
// composer init now forces Normal/NormalFloat/EndOfBuffer backgrounds to NONE,
// so the rendered pane must carry no background escape at all.
func TestComposerPaintsNoBackground(t *testing.T) {
	requireNvim(t)
	m := newTestModel(t)
	open(t, m, freshAnchor, newCommentSlot, "")
	typeText(m.comp, "hello world")
	if s := waitForScreen(t, m.comp.em, "world", 3*time.Second); !strings.Contains(s, "hello world") {
		t.Fatalf("seed text never landed; screen was:\n%s", s)
	}

	// Erase most of the text: this was the half of the bug where erased cells
	// kept the dark background behind them.
	for i := 0; i < 5; i++ {
		typeKeys(m.comp, keyBackspace)
	}
	time.Sleep(200 * time.Millisecond)

	if got := m.comp.em.Render(); strings.Contains(got, "\x1b[48") {
		t.Fatalf("composer render carries a background SGR (the pane must be transparent):\n%q", got)
	}
}

// TestCtrlBackspaceDecodesThroughRealReader pins the whole real path, the way
// TestCtrlEnterDecodesThroughRealReader did for submit: the exact bytes a
// kitty-speaking terminal sends for ctrl+backspace (CSI 127;5u) go through
// the same uv terminal reader bubbletea v2 uses, into handleKey, and out to
// the child — where the word before the cursor must be deleted. The
// by-hand key in TestCtrlBackspaceDeletesWord proves our encoding; this
// proves the terminal's decode reaches it.
func TestCtrlBackspaceDecodesThroughRealReader(t *testing.T) {
	requireNvim(t)
	m := newTestModel(t)
	open(t, m, freshAnchor, newCommentSlot, "")
	typeText(m.comp, "hello world")
	if s := waitForScreen(t, m.comp.em, "world", 3*time.Second); !strings.Contains(s, "hello world") {
		t.Fatalf("seed text never landed; screen was:\n%s", s)
	}

	tr := uv.NewTerminalReader(strings.NewReader("\x1b[127;5u"), "xterm-256color")
	evc := make(chan uv.Event, 1)
	done := make(chan struct{})
	go func() {
		defer close(done)
		_ = tr.StreamEvents(context.Background(), evc)
	}()
	select {
	case ev := <-evc:
		kev, ok := ev.(uv.KeyPressEvent)
		if !ok {
			t.Fatalf("decoded %T, want KeyPressEvent", ev)
		}
		if kev.Code != uv.KeyBackspace || kev.Mod&uv.ModCtrl == 0 {
			t.Fatalf("decoded %#v, want ctrl+backspace", kev)
		}
		m.handleKey(tea.KeyPressMsg(tea.Key(kev)))
	case <-done:
		t.Fatal("terminal reader produced no event for CSI 127;5u")
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for the reader to decode")
	}

	if s := waitForScreenWithout(t, m.comp, "hello", "world", 3*time.Second); strings.Contains(s, "world") {
		t.Fatalf("ctrl+backspace through the real reader did not delete the word; screen was:\n%s", s)
	}
}
