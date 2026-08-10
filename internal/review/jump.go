// Jumplist and link navigation (M3, navigation):
// `jump.follow` follows the first navigable link in the focused block —
// in-document (`[text](#heading-slug)`) jumping focus to the heading it
// names, or, in a tree review (D10), cross-document (`[text](other.md)`,
// `[text](other.md#section)`) switching the review to the file the link names
// — and the jumplist records where each in-document jump landed so
// `ctrl+o` / `ctrl+i` walk back and forward through the review the way vim's
// jumplist walks a file. The roadmap names the two walk keys
// (`ctrl+o` older, `ctrl+i` newer); the follow key is `ctrl+]` — vim's "jump
// to the reference under the cursor", which is exactly the gesture a tag
// jumplist pair belongs to — chosen over `gf`/`gx` (both `g`-prefixed, and
// `g` is an eager prefix that already moves to the first block, so a two-key
// combo would fire that side effect first) and over `enter`/`l` (dive is
// settled).
//
// What counts as a jump is deliberate and mirrors vim: the verbs that
// *teleport* focus — following a link, jumping to a search match, a source
// line, a section — each record where they landed. The frequent walkers
// (`j`/`k`, page keys, `g`/`G`) deliberately do not: recording every one of
// those would flood the list the way recording `gg` every time would flood
// vim's, and the walkers already have their own verbs for returning.
package review

import (
	"path/filepath"
	"strings"
)

// maxJumps caps the jumplist. Vim's default is a hundred entries; a review
// session has no reason to need more, and an unbounded list of cursors is a
// quiet leak.
const maxJumps = 100

// headingSlug is GitHub's slug for a heading — the `#fragment` an in-document
// `[text](#slug)` link targets. Lowercased; spaces and hyphens collapse to a
// single hyphen; punctuation drops without forcing a hyphen around it (so
// `## Q & A?` is `q--a`, exactly as GitHub slugs it — the ampersand vanishes
// and the two spaces each become a hyphen); underscores are kept; a hyphen
// tail is trimmed, so `## Retry policy` is `retry-policy`.
func headingSlug(text string) string {
	var b strings.Builder
	prevDash := false
	for _, r := range strings.ToLower(text) {
		switch {
		case r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || r == '_':
			b.WriteRune(r)
			prevDash = false
		case r == ' ' || r == '-':
			if !prevDash {
				b.WriteByte('-')
				prevDash = true
			}
		default:
			prevDash = false
		}
	}
	return strings.Trim(b.String(), "-")
}

// focusedLinks returns the hrefs of every link in the focused block's prose,
// in source order, or nil for a block with no links or no prose to read
// links out of (a code block's brackets are code; the frontmatter is
// metadata; a raw block is source). Table cells are prose and count, the
// same call blockLinks makes for what is navigable prose.
func (m *model) focusedLinks() []string {
	if m.at.entry < 0 || m.at.entry >= len(m.entries) {
		return nil
	}
	b := m.entries[m.at.entry].b
	var text string
	switch b.kind {
	case blockPara, blockHeading, blockListItem, blockList:
		text = b.text
	case blockQuote:
		text = strings.Join(b.lines, " ")
	case blockTable:
		if b.table == nil {
			return nil
		}
		for _, c := range b.table.header {
			text += " " + c
		}
		for _, row := range b.table.rows {
			for _, c := range row {
				text += " " + c
			}
		}
	default:
		return nil
	}
	var hrefs []string
	for _, r := range parseInline(text) {
		if r.link && r.href != "" {
			hrefs = append(hrefs, r.href)
		}
	}
	return hrefs
}

// followLink follows the first navigable link in the focused block. A
// `#fragment` link resolves against the current document's headings — jumping
// focus to that heading and recording the landing in the jumplist (the
// in-document half, unchanged). Any other href — a path naming another
// document, optionally carrying a `#fragment` — is the cross-document half and
// only exists in a tree review (D10): it opens the file the link names,
// landing on the fragment's heading when there is one. A link to anything
// outside the review tree (an external URL, a file not beneath the review
// root) is reported as such, and a fragment that names no heading is reported
// too rather than silently dropped.
func (m *model) followLink() {
	hrefs := m.focusedLinks()
	if len(hrefs) == 0 {
		m.status = "no link here"
		return
	}
	for _, href := range hrefs {
		if strings.HasPrefix(href, "#") {
			target, ok := m.headingSlugs[strings.TrimPrefix(href, "#")]
			if !ok {
				m.status = "no heading matches " + href
				return
			}
			m.pushJump(cursor{entry: target, comment: commentNone})
			m.jumpToEntry(target)
			m.status = "followed " + href + " — " + m.entries[target].b.text
			return
		}
		// Cross-document: only a tree review has another document to switch
		// to — a single-document review (`margin FILE.md`, D10) stays one
		// document, so its outside-document report is unchanged.
		if m.tree != nil && m.store != nil && m.followDoc(href) {
			return
		}
	}
	m.status = "links here point outside this document"
}

// followDoc follows a cross-document link: an href naming a markdown file in
// the review tree. The path is resolved markdown's way — relative to the
// current document's own directory (`[next](docs/spec.md)`), with a leading
// `/` meaning relative to the review root (`[/spec](docs/spec.md)`), the two
// spellings an agent's links actually use — and then looked up in the tree, so
// only a file the review can open is ever opened. It switches to that file
// exactly as openTreeFile would (fresh review: doc, threads, store and watcher
// re-point, session state resets) and, when the href carries a `#fragment`,
// lands on the heading it names, centred like an in-document follow. A link
// back at the document under review resolves its fragment without switching.
// Returns false for anything that is not a path to a markdown file in the tree
// — an absolute URL, a `mailto:`, a file outside the review root, a
// non-markdown file — so followLink falls through to its outside-document
// report.
func (m *model) followDoc(href string) bool {
	pathPart, frag, _ := strings.Cut(href, "#")
	if pathPart == "" {
		return false
	}
	var abs string
	switch {
	case strings.HasPrefix(pathPart, "/"):
		abs = filepath.Join(m.store.root, filepath.FromSlash(pathPart[1:]))
	case strings.Contains(pathPart, "://"):
		return false
	default:
		abs = filepath.Join(filepath.Dir(m.path), filepath.FromSlash(pathPart))
	}
	rel, err := filepath.Rel(m.store.root, abs)
	if err != nil || strings.HasPrefix(rel, "..") {
		return false
	}
	rel = filepath.ToSlash(rel)
	if !strings.HasSuffix(rel, ".md") && !strings.HasSuffix(rel, ".markdown") {
		return false
	}
	i := -1
	for j, e := range m.tree {
		if !e.isDir && e.rel == rel {
			i = j
			break
		}
	}
	if i < 0 {
		return false
	}

	if rel == m.store.docPath {
		// The link points back at the document under review — no switch, just
		// resolve the fragment against it. A fragmentless self-link is a quiet
		// no-op with a status line.
		if frag == "" {
			m.status = "already on " + rel
			return true
		}
		target, ok := m.headingSlugs[frag]
		if !ok {
			m.status = "no heading matches #" + frag + " in " + rel
			return true
		}
		m.pushJump(cursor{entry: target, comment: commentNone})
		m.jumpToEntry(target)
		m.status = "followed #" + frag + " — " + m.entries[target].b.text
		return true
	}

	// Switching while an editor is open is refused the way openTreeFile
	// refuses it; keep that message. Unreachable from the key (the composer
	// owns the keyboard), but a direct call must not silently clobber it.
	if m.comp != nil {
		m.status = "close the editor before opening a file"
		return true
	}
	m.openTreeFile(i)
	if frag == "" {
		// openTreeFile's own "opened <rel> — N blocks" status says it.
		return true
	}
	target, ok := m.headingSlugs[frag]
	if !ok {
		m.status = "opened " + rel + " — no heading matches #" + frag
		return true
	}
	m.jumpToEntry(target)
	// The jumplist restarts on the document switch (openTreeFile seeds it at
	// the new top); seed it at the actual landing so ctrl+o from a later jump
	// walks back to the followed heading.
	m.jumps = []cursor{m.at}
	m.jumpIdx = 0
	m.status = "followed " + rel + "#" + frag + " — " + m.entries[target].b.text
	return true
}

// jumpToEntry moves focus to entry i and centres it in the viewport, the way
// a search jump centres its match line — the landed-on block should be under
// the reader's eye, and scrollAnchor must agree so clampScroll does not
// re-derive the offset away. Shared by followLink and the jumplist walk.
func (m *model) jumpToEntry(i int) {
	if len(m.entries) == 0 {
		return
	}
	if i < 0 {
		i = 0
	}
	if i >= len(m.entries) {
		i = len(m.entries) - 1
	}
	m.at = cursor{entry: i, comment: commentNone}
	viewport := max(m.h-footerRows, 1)
	if i < len(m.spans) {
		m.scroll = max(0, m.spans[i].start-viewport/2)
	}
	m.scrollAnchor = m.at
	m.visual = false
}

// pushJump records a jump from the reader's current position to the landing
// c: it truncates any forward history (a jump made from the middle of the
// list starts a new forward branch, the way vim drops the newer end of its
// jumplist when you jump from an older entry), appends the position the jump
// left behind and then c, and moves the current-position marker to the
// landing — so ctrl+o returns to where the jump started and ctrl+i returns to
// where it landed. The position left behind is the entry focus sits on, not a
// comment dive, which is transient. A jump that leaves and lands on the same
// entry is a no-op, so two consecutive identical landings do not stack. The
// oldest entry past maxJumps is dropped.
func (m *model) pushJump(c cursor) {
	from := cursor{entry: m.at.entry, comment: commentNone}
	if len(m.jumps) == 0 {
		m.jumps = []cursor{from}
		m.jumpIdx = 0
	}
	m.jumps = m.jumps[:m.jumpIdx+1]
	if m.jumps[m.jumpIdx] != from {
		m.jumps = append(m.jumps, from)
	}
	if m.jumps[len(m.jumps)-1] != c {
		m.jumps = append(m.jumps, c)
	}
	if len(m.jumps) > maxJumps {
		n := len(m.jumps) - maxJumps
		m.jumps = m.jumps[n:]
	}
	m.jumpIdx = len(m.jumps) - 1
}

// jumpBack walks the jumplist to the next older position, centring it; at
// the oldest entry it reports rather than wrapping. jumpForward is the mirror.
func (m *model) jumpBack() {
	if m.jumpIdx <= 0 {
		m.status = "at the oldest jump"
		return
	}
	m.jumpIdx--
	m.jumpToEntry(m.jumps[m.jumpIdx].entry)
}

func (m *model) jumpForward() {
	if m.jumpIdx >= len(m.jumps)-1 {
		m.status = "at the newest jump"
		return
	}
	m.jumpIdx++
	m.jumpToEntry(m.jumps[m.jumpIdx].entry)
}
