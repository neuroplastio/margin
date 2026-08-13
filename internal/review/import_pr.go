// Importing comments from the GitHub pull request the current branch is on.
// The local loop stays the design (goals.md: "Not a GitHub client" — platform
// sync must not shape the local design), so an import never invents a
// GitHub-shaped on-disk format: it shells out to the gh CLI to detect the PR
// and fetch comments, and writes ordinary margin thread files, byte-identical
// to what the TUI writes, using each comment's line to pick the block and that
// block's own content-derived anchor. What lands is a local review,
// indistinguishable from one typed by hand — which is exactly what lets the
// maintainer treat a GitHub comment as a first-class thread from then on.
package review

import (
	"encoding/json"
	"fmt"
	"net/url"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
)

// cmdRunner runs an external command and returns its combined output. ImportPR
// takes it as a parameter so tests can substitute a scripted responder for gh;
// nil means the real runner (runGH).
type cmdRunner func(name string, args ...string) ([]byte, error)

// runGH runs name with args in dir, returning the combined output. The command
// runs in the document's directory, so `gh` detects the PR from the same
// checkout the document belongs to even when margin is invoked from elsewhere.
// A non-zero exit surfaces gh's own message — gh's stderr carries the useful
// half of "no pull request for this branch".
func runGH(dir, name string, args ...string) ([]byte, error) {
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		if msg := strings.TrimSpace(string(out)); msg != "" {
			return out, fmt.Errorf("%s: %w: %s", name, err, msg)
		}
		return out, fmt.Errorf("%s: %w", name, err)
	}
	return out, nil
}

// ghPR is the subset of `gh pr view --json number,url` ImportPR reads.
// Owner and Repo are filled in from the URL by ghCurrentPR.
type ghPR struct {
	Number int    `json:"number"`
	URL    string `json:"url"`
	Owner  string
	Repo   string
}

// ghUser is a GitHub user as nested in a comment object.
type ghUser struct {
	Login string `json:"login"`
}

// ghReviewComment is the subset of one review comment from
// `gh api .../pulls/N/comments` that matters here. line is the comment's line
// in the head (new-side) blob and original_line its line in the base; GitHub
// sets only the one for the side the comment is on, so ImportPR falls back.
type ghReviewComment struct {
	Path         string `json:"path"`
	Line         *int   `json:"line"`
	OriginalLine *int   `json:"original_line"`
	Body         string `json:"body"`
	User         ghUser `json:"user"`
	CreatedAt    string `json:"created_at"`
}

// ImportReport is what an import learned, for the command to summarise.
type ImportReport struct {
	PR         int
	Owner      string
	Repo       string
	Imported   int      // review comments written into thread files
	Skipped    int      // already in a thread (same author + body) — a re-import
	OtherFiles int      // review comments on files other than this document
	Unmapped   []string // one line each: a comment whose line maps to no block
}

// ImportPR imports the current pull request's review comments into the review
// of path. gh detects the PR (from the branch the document's checkout is on)
// and fetches the comments; each comment that names this document is attached
// to the block its line falls in, as an ordinary margin thread — the same
// thread file a reviewer typing `c` would write. A comment already present in
// a thread (same author, same body — a re-import) is skipped; a comment on
// another file, or on a line no block covers, is reported rather than dropped.
//
// The PR's general conversation comments have no line and therefore nothing to
// attach to; they are deliberately left to gh. Marking a thread resolved on
// disk survives an import: an existing thread keeps its resolved flag, only its
// posted list grows.
func ImportPR(path string, run cmdRunner) (ImportReport, error) {
	var rep ImportReport
	doc, _, err := loadDoc(path)
	if err != nil {
		return rep, err
	}
	root, docPath := resolveReviewRoot(path)
	ghDoc := filepath.ToSlash(docPath)
	if run == nil {
		run = func(name string, args ...string) ([]byte, error) {
			return runGH(filepath.Dir(path), name, args...)
		}
	}

	pr, err := ghCurrentPR(run)
	if err != nil {
		return rep, err
	}
	rep.PR, rep.Owner, rep.Repo = pr.Number, pr.Owner, pr.Repo

	comments, err := ghPRComments(run, pr.Owner, pr.Repo, pr.Number)
	if err != nil {
		return rep, err
	}

	threads, err := loadThreadsForDoc(root, docPath)
	if err != nil {
		return rep, err
	}
	changed := map[string]*thread{}
	var events []event

	for _, c := range comments {
		if c.Path != ghDoc {
			rep.OtherFiles++
			continue
		}
		line := 0
		if c.Line != nil {
			line = *c.Line
		} else if c.OriginalLine != nil {
			line = *c.OriginalLine
		}
		blk := blockForLine(doc, line)
		if blk == nil {
			rep.Unmapped = append(rep.Unmapped, describeUnmapped(c))
			continue
		}
		anchor := blk.anchor
		t := threads[anchor]
		if t == nil {
			t = &thread{anchor: anchor, quote: blk.text}
			threads[anchor] = t
		}
		if threadHasComment(t, c.User.Login, c.Body) {
			rep.Skipped++
			continue
		}
		at, err := time.Parse(time.RFC3339, c.CreatedAt)
		if err != nil {
			return rep, fmt.Errorf("import-pr: comment by %s: bad timestamp %q", c.User.Login, c.CreatedAt)
		}
		t.posted = append(t.posted, comment{author: c.User.Login, body: c.Body, at: at})
		changed[anchor] = t
		events = append(events, event{
			kind:    eventCommentPosted,
			doc:     docPath,
			anchor:  anchor,
			author:  c.User.Login,
			comment: len(t.posted) - 1,
			text:    c.Body,
		})
	}

	for _, t := range changed {
		if err := writeThreadFile(root, docPath, t); err != nil {
			return rep, err
		}
	}
	// Best-effort notices, exactly as AddComment treats a failed append: the
	// thread file is the record, the event log the notice (D13/D14), and a
	// lost notice must not fail the import that already landed on disk.
	for _, ev := range events {
		_, _ = appendEvent(root, ev)
	}
	rep.Imported = len(events)
	return rep, nil
}

// ghCurrentPR asks gh which PR the current branch is on and pulls owner and
// repo out of its URL. `gh pr view` fails with gh's own message when there is
// no PR, or no repository — that message is the useful error and is passed
// through.
func ghCurrentPR(run cmdRunner) (ghPR, error) {
	var pr ghPR
	out, err := run("gh", "pr", "view", "--json", "number,url")
	if err != nil {
		return pr, fmt.Errorf("import-pr: detecting the pull request: %w", err)
	}
	if err := json.Unmarshal(out, &pr); err != nil {
		return pr, fmt.Errorf("import-pr: gh pr view gave malformed output: %w", err)
	}
	u, err := url.Parse(strings.TrimSpace(pr.URL))
	if err != nil {
		return pr, fmt.Errorf("import-pr: gh pr view gave an unparsable URL %q: %w", pr.URL, err)
	}
	segs := strings.Split(strings.Trim(u.Path, "/"), "/")
	if len(segs) != 4 || segs[2] != "pull" {
		return pr, fmt.Errorf("import-pr: gh pr view gave an unexpected URL %q", pr.URL)
	}
	pr.Owner, pr.Repo = segs[0], segs[1]
	return pr, nil
}

// ghPRComments fetches every review comment on the PR, paginated so a long
// review is not truncated at the first page. gh merges the pages into one JSON
// array.
func ghPRComments(run cmdRunner, owner, repo string, number int) ([]ghReviewComment, error) {
	arg := fmt.Sprintf("repos/%s/%s/pulls/%d/comments", owner, repo, number)
	out, err := run("gh", "api", "--paginate", arg)
	if err != nil {
		return nil, fmt.Errorf("import-pr: fetching pull request comments: %w", err)
	}
	var comments []ghReviewComment
	if err := json.Unmarshal(out, &comments); err != nil {
		return nil, fmt.Errorf("import-pr: gh api gave malformed output: %w", err)
	}
	return comments, nil
}

// blockForLine returns the first commentable block whose source-line range
// contains line. Ordinary blocks' ranges are disjoint; a list item covers its
// own single line, so a PR comment on an item lands on exactly that item.
func blockForLine(doc []block, line int) *block {
	if line <= 0 {
		return nil
	}
	for i := range doc {
		if doc[i].commentable() && doc[i].line > 0 && line >= doc[i].line && line <= doc[i].endLine {
			return &doc[i]
		}
	}
	return nil
}

// threadHasComment reports whether t already holds a live comment by author
// with body — the "already imported" signal that makes re-running an import
// safe. Tombstones are ignored, so a deleted comment can be re-imported.
func threadHasComment(t *thread, author, body string) bool {
	for _, c := range t.posted {
		if !c.deleted && c.author == author && c.body == body {
			return true
		}
	}
	return false
}

// describeUnmapped is the one-line report for a comment no block could be
// found for: its GitHub locator and first line, so the reviewer can decide
// what to do with it rather than hunting for it.
func describeUnmapped(c ghReviewComment) string {
	line := 0
	if c.Line != nil {
		line = *c.Line
	} else if c.OriginalLine != nil {
		line = *c.OriginalLine
	}
	return fmt.Sprintf("%s:%d — %s: %s", c.Path, line, c.User.Login, firstLine(c.Body))
}

// ImportSummary renders an ImportReport as the one-paragraph human readout —
// shared by the `margin import-pr` command and the TUI's `:import-pr` palette
// action, so the two never disagree about what an import did.
func ImportSummary(rep ImportReport) string {
	noun := "comment"
	if rep.Imported != 1 {
		noun = "comments"
	}
	var b strings.Builder
	fmt.Fprintf(&b, "imported %d %s from PR #%d (%s/%s)", rep.Imported, noun, rep.PR, rep.Owner, rep.Repo)
	if rep.Skipped > 0 {
		fmt.Fprintf(&b, "; %d already imported", rep.Skipped)
	}
	if rep.OtherFiles > 0 {
		fmt.Fprintf(&b, "; %d on other files left alone", rep.OtherFiles)
	}
	if n := len(rep.Unmapped); n > 0 {
		fmt.Fprintf(&b, "; %d on no matching line", n)
	}
	b.WriteString(".")
	return b.String()
}

// importPR runs the gh CLI import against the document under review, in the
// background — gh is a blocking external call and must not run on the render
// goroutine — and reports the outcome through importPRMsg. It refuses where
// there is no review root to write threads into (an ephemeral stdin review,
// or a document that never resolved a store). The report's summary lands in
// the status line; any error is surfaced the same way. It never modifies the
// model before the message returns: the import's effect is on disk, and
// importPRMsg's handler is what reconciles the on-screen threads with it.
func (m *model) importPR() tea.Cmd {
	if m.store == nil {
		m.status = "nothing to import into"
		return nil
	}
	m.status = "importing comments from the current pull request…"
	path := m.path
	return func() tea.Msg {
		rep, err := ImportPR(path, nil)
		return importPRMsg{rep: rep, err: err}
	}
}
