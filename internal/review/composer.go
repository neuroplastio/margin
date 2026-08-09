package review

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	tea "charm.land/bubbletea/v2"
	uv "github.com/charmbracelet/ultraviolet"
	"github.com/charmbracelet/x/vt"
	"github.com/creack/pty"
)

// outcome is how the composer was dismissed. It travels from nvim to the host as
// an exit code, which needs no IPC and no protocol: `:cquit N` is a first-class
// vim feature, and git has used the same channel to abort commits for decades.
type outcome int

const (
	outcomeSubmit  outcome = iota // exit 0  — apply the text
	outcomeDraft                  // exit 2  — keep it unsubmitted
	outcomeDiscard                // exit 1  — throw it away
)

func (o outcome) String() string {
	switch o {
	case outcomeSubmit:
		return "submitted"
	case outcomeDraft:
		return "kept as draft"
	default:
		return "discarded"
	}
}

// outcomeFromExit maps the child's exit status. Draft is the outcome for a plain
// close, matching the GitHub review model where walking away from a half-written
// comment keeps it rather than losing it; discarding has to be asked for.
func outcomeFromExit(err error) outcome {
	if err == nil {
		return outcomeSubmit
	}
	var ee *exec.ExitError
	if errors.As(err, &ee) {
		switch ee.ExitCode() {
		case 2:
			return outcomeDraft
		case 1:
			return outcomeDiscard
		}
	}
	return outcomeDiscard
}

// composerInit is the whole "strip nvim down to a textarea" brief.
//
// Chrome first: no statusline, no ruler, no line numbers, no sign column, no
// '~' filler past the end of the buffer, and cmdheight=0 so the command line
// does not reserve a row. In a 10-row pane those would cost a third of the
// visible text. The mode indicator is dropped too — the host draws it in the
// footer instead, inferred from the cursor shape the child already reports.
//
// The buffer holds the comment text and nothing else: no instructions, no
// quoted context. The block being commented on is already on screen directly
// above the pane and the keys are in the host footer, so repeating either
// inside the editor is just clutter to scroll past.
//
// Every exit is an nvim mapping or command rather than a key the host
// intercepts, so the pane keeps sole ownership of input. The ex-command
// abbreviations matter as much as the mappings: `:q` and `:wq` are muscle
// memory and should do the obvious thing rather than prompt.
const composerInit = `
vim.g.mapleader = ' '
vim.g.maplocalleader = ' '

vim.opt.number = false
vim.opt.relativenumber = false
vim.opt.signcolumn = 'no'
vim.opt.laststatus = 0
vim.opt.ruler = false
vim.opt.showcmd = false
vim.opt.showmode = false
vim.opt.cmdheight = 0
vim.opt.fillchars = { eob = ' ' }

-- nvim's default colorscheme paints Normal with its own dark background
-- (NvimDarkGrey2), which clashes with margin's themed background and leaves
-- background SGR behind on erased cells. The pane is a window on the host's
-- document: paint nothing, let margin's own background show through.
vim.api.nvim_set_hl(0, 'Normal', { bg = 'NONE', ctermbg = 'NONE' })
vim.api.nvim_set_hl(0, 'NormalFloat', { bg = 'NONE', ctermbg = 'NONE' })
vim.api.nvim_set_hl(0, 'EndOfBuffer', { bg = 'NONE', ctermbg = 'NONE' })

vim.opt.wrap = true
vim.opt.linebreak = true
vim.opt.breakindent = true
vim.opt.spell = true
vim.opt.spelllang = 'en_us'
vim.opt.swapfile = false
vim.opt.shadafile = 'NONE'

-- <Esc><Esc> is a pending-mapping prefix, so a lone <Esc> in normal mode waits
-- this long before resolving. Keep it short or leaving insert mode feels laggy.
vim.opt.timeoutlen = 250

local function submit()
  vim.cmd('silent write')
  vim.cmd('quitall')
end

local function draft()
  vim.cmd('silent write')
  vim.cmd('cquit 2')
end

local function discard()
  vim.cmd('cquit 1')
end

-- User commands give the host a deterministic way to dismiss the pane. Sending
-- <Esc><Esc> would work too, but only from normal mode; a command survives
-- whatever mode or pending state the editor happens to be in.
vim.api.nvim_create_user_command('MarginSubmit', function() submit() end, { bang = true })
vim.api.nvim_create_user_command('MarginDiscard', function() discard() end, { bang = true })
-- The bang variant exists so ':q!' abbreviates cleanly: the abbreviation fires
-- on 'q' and the '!' rides along, which is also the right meaning.
vim.api.nvim_create_user_command('MarginDraft', function(o)
  if o.bang then discard() else draft() end
end, { bang = true })

-- ':q' closes keeping a draft, ':q!' throws it away, ':wq' and ':x' submit.
-- Without these, ':q' on a modified buffer either errors or prompts, and a
-- prompt inside a ten-row pane is exactly the friction this is meant to avoid.
local function abbrev(from, to)
  vim.cmd(string.format(
    "cnoreabbrev <expr> %s (getcmdtype() == ':' && getcmdline() ==# '%s') ? '%s' : '%s'",
    from, from, to, from))
end
abbrev('q', 'MarginDraft')
abbrev('qa', 'MarginDraft')
abbrev('wq', 'MarginSubmit')
abbrev('x', 'MarginSubmit')
abbrev('wqa', 'MarginSubmit')

local map = vim.keymap.set
map({ 'n', 'i' }, '<C-s>', submit, { desc = 'submit comment' })
-- Terminals that distinguish ctrl+backspace from plain backspace deliver it
-- as <C-BS>, and alt+backspace arrives as <M-BS>; nvim leaves both unbound.
-- The host now forwards them (see encodeModifiedKey), so give them the
-- meaning every other editor gives them — and, for <M-BS>, a mapping is also
-- what stops nvim collapsing the unbound meta combo into a bare <Esc>.
map('i', '<C-BS>', '<C-W>', { desc = 'delete word before cursor' })
map('i', '<M-BS>', '<C-W>', { desc = 'delete word before cursor' })
map('n', 'ZZ', submit, { desc = 'submit comment' })
map('n', '<Esc><Esc>', draft, { desc = 'close, keeping a draft' })
map('n', '<leader>cc', submit, { desc = 'submit comment' })
map('n', '<leader>cd', draft, { desc = 'close, keeping a draft' })
map('n', '<leader>ck', discard, { desc = 'discard comment' })

vim.cmd('autocmd BufEnter * setlocal filetype=markdown')

-- Insert mode iff there is nothing to read. A new comment wants to be typed
-- into; existing text — a resumed draft, or a comment being edited — wants
-- normal mode, so the first keystroke is a motion rather than an insertion.
--
-- This has to wait for VimEnter: an init script runs before nvim opens the file
-- argument, so checking the buffer here would always find it empty and always
-- start in insert mode.
vim.api.nvim_create_autocmd('VimEnter', {
  once = true,
  callback = function()
    local empty = true
    for _, l in ipairs(vim.api.nvim_buf_get_lines(0, 0, -1, false)) do
      if l:match('%S') then
        empty = false
        break
      end
    end
    if empty then
      vim.cmd('startinsert')
    else
      vim.api.nvim_win_set_cursor(0, { 1, 0 })
    end
  end,
})
`

// composer is one live editor pane: a child nvim, the emulator it paints into,
// and the pty between them. The host creates and destroys these as comments are
// opened and dismissed, so everything here is per-session rather than global.
type composer struct {
	gen    int
	em     *vt.SafeEmulator
	cur    *cursorState
	damage chan struct{}

	ptmx *os.File
	cmd  *exec.Cmd

	anchor string
	target int // newCommentSlot, or the index of the posted comment being edited
	path   string
	tmpDir string
}

// newComposer spawns nvim on a scratch file holding seed and nothing else.
func newComposer(gen int, anchor string, target int, seed string, w, h int) (*composer, error) {
	tmpDir, err := os.MkdirTemp("", "margin")
	if err != nil {
		return nil, err
	}

	initPath := filepath.Join(tmpDir, "composer.lua")
	if err := os.WriteFile(initPath, []byte(composerInit), 0o644); err != nil {
		return nil, err
	}
	path := filepath.Join(tmpDir, "COMMENT_EDITMSG.md")
	if err := os.WriteFile(path, []byte(seed), 0o644); err != nil {
		return nil, err
	}

	editor := os.Getenv("EDITOR")
	if editor == "" {
		editor = "nvim"
	}
	args := []string{"--noplugin", "-i", "NONE", "-u", initPath, path}
	if os.Getenv("MARGIN_NVIM_CONFIG") == "user" {
		// Escape hatch for comparing the stripped pane against the real config.
		args = []string{path}
	}

	cmd := exec.Command(editor, args...)
	cmd.Env = append(os.Environ(), "TERM=xterm-256color", "COLORTERM=truecolor")
	ptmx, err := pty.StartWithSize(cmd, &pty.Winsize{Rows: uint16(h), Cols: uint16(w)})
	if err != nil {
		return nil, fmt.Errorf("start %s: %w", editor, err)
	}

	c := &composer{
		gen: gen, ptmx: ptmx, cmd: cmd, path: path, tmpDir: tmpDir,
		anchor: anchor, target: target,
		em:     vt.NewSafeEmulator(w, h),
		cur:    &cursorState{shape: tea.CursorBlock, blink: true, visible: true},
		damage: make(chan struct{}, 1),
	}
	c.em.SetCallbacks(vt.Callbacks{
		CursorStyle: func(style vt.CursorStyle, blink bool) {
			// vt and Bubble Tea declare Block/Underline/Bar in the same order.
			c.cur.set(tea.CursorShape(style), blink)
		},
		CursorVisibility: func(visible bool) { c.cur.setVisible(visible) },
	})

	// Output side is a hand-rolled pump rather than io.Copy so it can signal
	// damage; the signal is coalesced through a buffered channel of one, so a
	// burst of output schedules a single redraw instead of flooding.
	go func() {
		buf := make([]byte, 32*1024)
		for {
			n, err := ptmx.Read(buf)
			if n > 0 {
				_, _ = c.em.Write(buf[:n])
				select {
				case c.damage <- struct{}{}:
				default:
				}
			}
			if err != nil {
				return
			}
		}
	}()
	go func() { _, _ = io.Copy(ptmx, c.em) }()

	return c, nil
}

// body returns what the reviewer wrote.
func (c *composer) body() string {
	b, err := os.ReadFile(c.path)
	if err != nil {
		return ""
	}
	return parseComment(string(b))
}

// mode infers NORMAL vs INSERT from the cursor shape the child requested via
// DECSCUSR. nvim's own '-- INSERT --' is switched off to save a row, so the host
// renders the indicator in the footer instead.
func (c *composer) mode() string {
	shape, _, _ := c.cur.get()
	if shape == tea.CursorBar || shape == tea.CursorUnderline {
		return "INSERT"
	}
	return "NORMAL"
}

// requestDraft asks the child to save and exit with the draft code. This is how
// blur works: losing focus is the same outcome as pressing esc esc, so the host
// does not need a second persistence path. The leading <Esc> leaves insert or
// visual mode first; the ex command then runs regardless of pending state.
func (c *composer) requestDraft() {
	c.em.SendText("\x1b:MarginDraft\r")
}

// sendMouse forwards a click to the child in pane-relative coordinates, so
// clicking inside the editor positions the cursor rather than moving focus.
func (c *composer) sendMouse(relX, relY int, button uv.MouseButton, release bool) {
	m := uv.Mouse{X: relX, Y: relY, Button: button}
	if release {
		c.em.SendMouse(uv.MouseReleaseEvent(m))
		return
	}
	c.em.SendMouse(uv.MouseClickEvent(m))
}

// resize keeps the emulator grid and the child's winsize in step with the pane.
func (c *composer) resize(w, h int) {
	c.em.Resize(w, h)
	_ = pty.Setsize(c.ptmx, &pty.Winsize{Rows: uint16(h), Cols: uint16(w)})
}

func (c *composer) close() {
	if c.cmd.Process != nil {
		_ = c.cmd.Process.Kill()
	}
	_ = c.ptmx.Close()
	_ = os.RemoveAll(c.tmpDir)
}

// sendKey forwards a host keypress to the child.
//
// It cannot just hand everything to vt.Emulator.SendKey: that function's
// fallback branch is `if key.Mod == 0 { seq += string(key.Code) }`, so any key
// carrying a modifier it doesn't have an explicit case for is silently dropped.
// Under Ghostty, which speaks the Kitty keyboard protocol, Bubble Tea decodes
// shift+o as {Code:'o', Mod:ModShift} rather than a bare 'O' — so capitals and
// every shifted punctuation vanish. Hence: printable text goes as text, and
// every key carrying a modifier goes through encodeModifiedKey.
func (c *composer) sendKey(k tea.Key) {
	// ctrl+enter submits.
	//
	// This is the one place besides ctrl+\ where the host reads a key instead
	// of forwarding it, and it is here only because it cannot work any other
	// way: a plain terminal sends CR for both enter and ctrl+enter, so nvim
	// cannot tell them apart. We only see the difference because Ghostty speaks
	// the Kitty keyboard protocol — and the embedded emulator does not
	// advertise that protocol to the child, so nvim could not receive the
	// distinction even if we forwarded it.
	//
	// It resolves to the same ex command as every other submit gesture, so
	// there is still exactly one submit path.
	if k.Code == uv.KeyEnter && k.Mod&uv.ModCtrl != 0 {
		c.em.SendText("\x1b:MarginSubmit\r")
		return
	}

	// Printable characters, shifted or not, carry their literal in Text.
	// This is the path that makes O, D, A, !, ? work.
	if k.Text != "" && k.Mod&(uv.ModCtrl|uv.ModAlt|uv.ModMeta) == 0 {
		c.em.SendText(k.Text)
		return
	}

	// Keys carrying modifiers are encoded here rather than handed to
	// vt.SendKey. vt's fallback drops any modified key it has no explicit
	// case for (F1) — ctrl+backspace, shift+enter, ctrl+shift+letter — and,
	// worse, an alt combo carrying a second modifier comes out as a bare
	// ESC, which throws nvim into normal mode mid-typing.
	if k.Mod != 0 {
		if seq, ok := encodeModifiedKey(k); ok {
			c.em.SendText(seq)
			return
		}
	}

	c.em.SendKey(uv.KeyPressEvent(uv.Key(k)))
}

// encodeModifiedKey renders a key with modifiers into bytes a child nvim
// decodes as that key. Every form used below was fed to a real nvim on a pty
// (TERM=xterm-256color, no kitty advertisement) and confirmed to decode as
// the intended key — see journal 2026-08-09.19.
//
//   - alt combos: ESC + the legacy encoding of the key without alt (the meta
//     form terminal vim has always used: ESC x is <M-x>, ESC X is <M-S-x>,
//     ESC ^X is <C-M-x>, ESC DEL is <M-BS>). nvim resolves meta keys only
//     when a mapping names them, whatever the encoding — CSI-u with alt in
//     the modifier included — so the modern form buys nothing here.
//   - arrows, home, end:              CSI 1 ; <mod> <final>   (xterm)
//   - insert, delete, pgup, pgdn, F5-F12:  CSI <n> ; <mod> ~
//   - F1-F4:                          CSI 1 ; <mod> <P/Q/R/S>
//   - everything else — ctrl+backspace, shift+enter, ctrl+shift+letter:
//     CSI <code> ; <mod> u  (kitty's CSI-u form)
//
// The one family left to vt.SendKey (ok=false) is ctrl-only combos vt has a
// control-byte case for (ctrl+a..z, ctrl+space, ctrl+[ et al.). Those bytes
// double as other keys in every terminal — ctrl+m is CR, ctrl+[ is ESC — so
// they keep the legacy encoding rather than becoming distinct CSI-u keys.
func encodeModifiedKey(k tea.Key) (string, bool) {
	if k.Mod&uv.ModAlt != 0 {
		return encodeMetaKey(k)
	}
	if final, ok := xtermFinal[k.Code]; ok {
		return fmt.Sprintf("\x1b[1;%d%c", xtermModParam(k.Mod), final), true
	}
	if n, ok := xtermTilde[k.Code]; ok {
		return fmt.Sprintf("\x1b[%d;%d~", n, xtermModParam(k.Mod)), true
	}
	if final, ok := xtermF1toF4[k.Code]; ok {
		return fmt.Sprintf("\x1b[1;%d%c", xtermModParam(k.Mod), final), true
	}
	if k.Mod == uv.ModCtrl && vtCtrlAlias(k.Code) {
		return "", false // vt's legacy control byte
	}
	return fmt.Sprintf("\x1b[%d;%du", k.Code, xtermModParam(k.Mod)), true
}

// encodeMetaKey renders an alt combo as ESC + the encoding of the key without
// alt. vt.SendKey already does this for alt-only keys, but an alt combo
// carrying a second modifier falls through vt's fallback and comes out as a
// bare ESC — nvim reads that as Escape and drops out of insert mode, the
// "dropping into normal mode unexpectedly" half of the feedback.
func encodeMetaKey(k tea.Key) (string, bool) {
	inner := tea.Key{Code: k.Code, Mod: k.Mod &^ uv.ModAlt, ShiftedCode: k.ShiftedCode}
	switch inner.Mod {
	case 0:
		// alt+key: ESC + the key's plain legacy bytes.
		if s, ok := legacyPlain(k.Code); ok {
			return "\x1b" + s, true
		}
	case uv.ModShift:
		// alt+shift+printable: ESC + the shifted literal (ESC X is <M-S-x>).
		if r, ok := shiftedRune(inner); ok {
			return "\x1b" + string(r), true
		}
	case uv.ModCtrl:
		// alt+ctrl+letter et al.: ESC + the control byte (ESC ^X is <C-M-x>).
		if b, ok := ctrlByte(k.Code); ok {
			return "\x1b" + string([]byte{b}), true
		}
	}
	// Everything else: ESC + the no-alt encoding (ESC ESC[3~ is <M-Del>,
	// ESC ESC[97;5u is <C-M-a> — both verified).
	if seq, ok := encodeModifiedKey(inner); ok {
		return "\x1b" + seq, true
	}
	return "", false
}

// legacyPlain is the unmodified encoding of a special key, as terminals have
// sent it since xterm: control bytes for the ascii specials, CSI/SS3 forms
// for navigation and function keys.
func legacyPlain(code rune) (string, bool) {
	switch code {
	case ' ':
		return " ", true
	case uv.KeyEnter:
		return "\r", true
	case uv.KeyTab:
		return "\t", true
	case uv.KeyBackspace:
		return "\x7f", true
	case uv.KeyEscape:
		return "\x1b", true
	}
	if final, ok := xtermFinal[code]; ok {
		return "\x1b[" + string(final), true
	}
	if n, ok := xtermTilde[code]; ok {
		return fmt.Sprintf("\x1b[%d~", n), true
	}
	if final, ok := xtermF1toF4[code]; ok {
		return "\x1bO" + string(final), true
	}
	if code > ' ' && code < 0x7f {
		return string(code), true
	}
	return "", false
}

// shiftedRune returns the shifted literal of a key: the shifted codepoint the
// terminal reported if there is one, else the uppercase letter. Digits and
// punctuation without a reported shifted form give up rather than guess a
// keyboard layout.
func shiftedRune(k tea.Key) (rune, bool) {
	if k.ShiftedCode > ' ' && k.ShiftedCode < 0x7f {
		return k.ShiftedCode, true
	}
	if k.Code >= 'a' && k.Code <= 'z' {
		return k.Code - 'a' + 'A', true
	}
	return 0, false
}

// ctrlByte is the legacy control byte for a ctrl combo: ctrl+space is NUL,
// ctrl+a..z are 0x01-0x1a, and ctrl+[ \ ] ^ _ are 0x1b-0x1f — the same table
// vt.SendKey's explicit cases encode.
func ctrlByte(code rune) (byte, bool) {
	switch {
	case code == ' ':
		return 0x00, true
	case code >= 'a' && code <= 'z':
		return byte(code - 'a' + 1), true
	case code >= '[' && code <= '_':
		return byte(code - '[' + 0x1b), true
	}
	return 0, false
}

// xtermModParam is the CSI modifier parameter: 1, plus 1 for shift, 2 for
// alt, 4 for ctrl, 8 for meta.
func xtermModParam(mod uv.KeyMod) int {
	n := 1
	if mod&uv.ModShift != 0 {
		n++
	}
	if mod&uv.ModAlt != 0 {
		n += 2
	}
	if mod&uv.ModCtrl != 0 {
		n += 4
	}
	if mod&uv.ModMeta != 0 {
		n += 8
	}
	return n
}

// vtCtrlAlias reports whether code is one of the keys vt.SendKey encodes as a
// legacy control byte when combined with ctrl alone (its explicit ctrl table:
// space, a-z, and [ \ ] ^ _). Those bytes double as other keys in every
// terminal — ctrl+m is CR, ctrl+[ is ESC — so they keep the legacy encoding
// rather than becoming distinct CSI-u keys.
func vtCtrlAlias(code rune) bool {
	_, ok := ctrlByte(code)
	return ok
}

var xtermFinal = map[rune]byte{
	uv.KeyUp: 'A', uv.KeyDown: 'B', uv.KeyRight: 'C', uv.KeyLeft: 'D',
	uv.KeyHome: 'H', uv.KeyEnd: 'F',
}

var xtermTilde = map[rune]int{
	uv.KeyInsert: 2, uv.KeyDelete: 3, uv.KeyPgUp: 5, uv.KeyPgDown: 6,
	uv.KeyF5: 15, uv.KeyF6: 17, uv.KeyF7: 18, uv.KeyF8: 19,
	uv.KeyF9: 20, uv.KeyF10: 21, uv.KeyF11: 23, uv.KeyF12: 24,
}

var xtermF1toF4 = map[rune]byte{
	uv.KeyF1: 'P', uv.KeyF2: 'Q', uv.KeyF3: 'R', uv.KeyF4: 'S',
}

// parseComment trims the buffer. There is no fold to strip any more — the buffer
// holds only what the reviewer typed.
func parseComment(raw string) string { return strings.TrimSpace(raw) }

// draftPath keys drafts by anchor and target, so a half-written reply and a
// half-finished edit of an existing comment cannot collide. In the real tool
// these become thread-file entries with `status: draft`.
func draftPath(anchor string, target int) (string, error) {
	base, err := os.UserCacheDir()
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256([]byte(anchor + "#" + strconv.Itoa(target)))
	dir := filepath.Join(base, "margin", "drafts")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	return filepath.Join(dir, hex.EncodeToString(sum[:8])+".md"), nil
}

func loadDraft(anchor string, target int) (string, error) {
	path, err := draftPath(anchor, target)
	if err != nil {
		return "", err
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

func saveDraft(anchor string, target int, body string) error {
	path, err := draftPath(anchor, target)
	if err != nil {
		return err
	}
	if strings.TrimSpace(body) == "" {
		return clearDraft(anchor, target)
	}
	return os.WriteFile(path, []byte(body), 0o644)
}

func clearDraft(anchor string, target int) error {
	path, err := draftPath(anchor, target)
	if err != nil {
		return err
	}
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}
