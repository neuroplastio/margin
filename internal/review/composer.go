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
vim.api.nvim_create_user_command('MdreviewSubmit', function() submit() end, { bang = true })
vim.api.nvim_create_user_command('MdreviewDiscard', function() discard() end, { bang = true })
-- The bang variant exists so ':q!' abbreviates cleanly: the abbreviation fires
-- on 'q' and the '!' rides along, which is also the right meaning.
vim.api.nvim_create_user_command('MdreviewDraft', function(o)
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
abbrev('q', 'MdreviewDraft')
abbrev('qa', 'MdreviewDraft')
abbrev('wq', 'MdreviewSubmit')
abbrev('x', 'MdreviewSubmit')
abbrev('wqa', 'MdreviewSubmit')

local map = vim.keymap.set
map({ 'n', 'i' }, '<C-s>', submit, { desc = 'submit comment' })
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
	tmpDir, err := os.MkdirTemp("", "mdreview-probe")
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
	if os.Getenv("MDREVIEW_NVIM_CONFIG") == "user" {
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
	c.em.SendText("\x1b:MdreviewDraft\r")
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
// modified special keys get encoded here.
func (c *composer) sendKey(k tea.Key) {
	// Printable characters, shifted or not, carry their literal in Text.
	// This is the path that makes O, D, A, !, ? work.
	if k.Text != "" && k.Mod&(uv.ModCtrl|uv.ModAlt|uv.ModMeta) == 0 {
		c.em.SendText(k.Text)
		return
	}

	// Special keys with modifiers: vt drops these, so emit the xterm form
	// CSI 1 ; <mod> <final>, which nvim understands. Gets you shift+arrow
	// selection and ctrl+arrow word motions.
	if k.Mod != 0 {
		if final, ok := xtermFinal[k.Code]; ok {
			mod := 1
			if k.Mod&uv.ModShift != 0 {
				mod++
			}
			if k.Mod&uv.ModAlt != 0 {
				mod += 2
			}
			if k.Mod&uv.ModCtrl != 0 {
				mod += 4
			}
			c.em.SendText(fmt.Sprintf("\x1b[1;%d%c", mod, final))
			return
		}
	}

	c.em.SendKey(uv.KeyPressEvent(uv.Key(k)))
}

var xtermFinal = map[rune]byte{
	uv.KeyUp: 'A', uv.KeyDown: 'B', uv.KeyRight: 'C', uv.KeyLeft: 'D',
	uv.KeyHome: 'H', uv.KeyEnd: 'F',
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
	dir := filepath.Join(base, "mdreview-probe", "drafts")
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
