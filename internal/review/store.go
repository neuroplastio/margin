// Thread persistence: one markdown file per thread under `.margin/threads/`,
// keyed by document path and anchor (D9). This is the "no schema, ordinary
// file tools" half of D5 — an agent reads and writes these with nothing
// beyond a text editor.
//
// A thread file's frontmatter also carries `resolved` (THREAD-01, per D11) —
// a bare boolean, omitted from the frontmatter entirely when false so an
// unresolved thread's file is byte-for-byte what it was before this field
// existed. Deletion's tombstone shape (also D11) is THREAD-03 and not part
// of this file yet.
//
// STORE-02 wires this into the running app: Run loads every thread file on
// disk for the document it opens (loadThreadsForDoc), and dismiss persists a
// posted comment back through threadStore.save. Reloading while margin is
// already running — noticing a file an agent wrote mid-session — is not part
// of that; it needs a filesystem watcher and is split out on the board.
package review

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

// frontmatterDelim is the line that opens and closes a thread file's
// frontmatter block, same convention as Jekyll/Hugo front matter — familiar
// to anything that has ever looked at a markdown file with metadata.
const frontmatterDelim = "---"

// threadsDir is where every thread file for a review root lives. root is the
// directory margin was pointed at — today always the lone document's
// directory, since M1 reviews one file at a time, but the layout does not
// assume that: everything is already keyed by docPath underneath it, which is
// what lets a future tree review (D10) add documents without moving
// anything that already exists on disk.
func threadsDir(root string) string {
	return filepath.Join(root, ".margin", "threads")
}

// threadFilePath is where the thread anchored to anchor on docPath is stored,
// rooted at root. docPath's directories are preserved under threads/, so
// threads for docs/spec.md land at .margin/threads/docs/spec.md/<id>.md — a
// tree of thread files that mirrors the tree of documents.
func threadFilePath(root, docPath, anchor string) string {
	id := strings.TrimPrefix(anchor, "^")
	return filepath.Join(threadsDir(root), filepath.FromSlash(docPath), id+".md")
}

// writeThreadFile marshals t and writes it to its path under root, creating
// any missing directories. Overwrites whatever was there — a thread file has
// exactly one writer's worth of truth: the in-memory thread.
func writeThreadFile(root, docPath string, t *thread) error {
	path := threadFilePath(root, docPath, t.anchor)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("thread file: %w", err)
	}
	if err := os.WriteFile(path, marshalThread(docPath, t), 0o644); err != nil {
		return fmt.Errorf("thread file: %w", err)
	}
	return nil
}

// readThreadFile reads and parses the thread file at path, returning the
// document path recorded in its frontmatter alongside the thread.
func readThreadFile(path string) (docPath string, t *thread, err error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", nil, fmt.Errorf("thread file: %w", err)
	}
	docPath, t, err = parseThreadFile(data)
	if err != nil {
		return "", nil, fmt.Errorf("thread file %s: %w", path, err)
	}
	return docPath, t, nil
}

// loadThreadsForDoc reads every thread file already on disk for docPath under
// root, keyed by anchor, so a document reopened for review finds its threads
// still attached (goals.md item 7). A threads directory that does not exist
// yet is not an error — it just means nothing has been said about this
// document — but a thread file that exists and fails to parse is: the same
// "surface, don't drop" reasoning readThreadFile already documents applies
// here, one level up.
func loadThreadsForDoc(root, docPath string) (map[string]*thread, error) {
	dir := filepath.Join(threadsDir(root), filepath.FromSlash(docPath))
	entries, err := os.ReadDir(dir)
	if errors.Is(err, os.ErrNotExist) {
		return map[string]*thread{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("loading threads for %s: %w", docPath, err)
	}

	threads := map[string]*thread{}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".md") {
			continue
		}
		_, t, err := readThreadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			return nil, err
		}
		threads[t.anchor] = t
	}
	return threads, nil
}

// threadStore is where a model persists posted comments once it is opened
// against a real path (Run). It is nil on a model built for a test or the
// seeded document, which is what keeps dismiss() from writing to disk during
// every test that exercises a submit.
type threadStore struct {
	root    string
	docPath string
}

// save writes t to its thread file. Nil-safe so callers do not need an
// "if m.store != nil" guard at every call site — an unconfigured store is
// simply a no-op, so the call site reads as "persistence is optional" rather
// than a special case to remember.
func (s *threadStore) save(t *thread) error {
	if s == nil {
		return nil
	}
	return writeThreadFile(s.root, s.docPath, t)
}

// marshalThread renders a thread as the markdown-with-frontmatter file an
// agent reads and writes directly: anchor and document identify what the
// thread is attached to, the quoted block is the fallback for a human (or an
// agent) reading the file without margin open, and each posted comment
// appears as its own `## author — timestamp` section, in posting order.
func marshalThread(docPath string, t *thread) []byte {
	var b strings.Builder
	b.WriteString(frontmatterDelim + "\n")
	fmt.Fprintf(&b, "anchor: %s\n", t.anchor)
	fmt.Fprintf(&b, "document: %s\n", docPath)
	if t.resolved {
		b.WriteString("resolved: true\n")
	}
	b.WriteString(frontmatterDelim + "\n\n")

	for _, l := range quoteAsLines(t.quote) {
		b.WriteString(l)
		b.WriteString("\n")
	}

	for _, c := range t.posted {
		b.WriteString("\n## ")
		b.WriteString(c.author)
		b.WriteString(" — ")
		b.WriteString(c.at.UTC().Format(time.RFC3339Nano))
		b.WriteString("\n\n")
		b.WriteString(strings.Trim(c.body, "\n"))
		b.WriteString("\n")
	}
	return []byte(b.String())
}

// commentHeaderRE matches a comment section's header line: `## author —
// timestamp`. The em dash is part of the fixed shape we write, not something
// a hand-written file needs to reproduce exactly — a parse failure here is
// reported with the offending line so a malformed file fails loudly rather
// than losing a comment silently.
var commentHeaderRE = regexp.MustCompile(`^## (.+?) — (.+)$`)

// parseThreadFile is marshalThread's inverse. It returns an error for
// anything that does not round-trip: a caller with a thread file that fails
// to parse should surface that, not silently drop the thread (the same
// principle reattach applies to a vanished anchor — see document.go).
func parseThreadFile(data []byte) (docPath string, t *thread, err error) {
	lines := strings.Split(string(data), "\n")
	if len(lines) == 0 || strings.TrimSpace(lines[0]) != frontmatterDelim {
		return "", nil, fmt.Errorf("expected frontmatter starting with %q", frontmatterDelim)
	}

	fields := map[string]string{}
	i := 1
	for ; i < len(lines) && strings.TrimSpace(lines[i]) != frontmatterDelim; i++ {
		k, v, ok := strings.Cut(lines[i], ":")
		if !ok {
			return "", nil, fmt.Errorf("malformed frontmatter line %q", lines[i])
		}
		fields[strings.TrimSpace(k)] = strings.TrimSpace(v)
	}
	if i >= len(lines) {
		return "", nil, fmt.Errorf("unterminated frontmatter")
	}
	body := lines[i+1:]

	anchor := fields["anchor"]
	if anchor == "" {
		return "", nil, fmt.Errorf("frontmatter missing anchor")
	}
	docPath = fields["document"]
	// Anything other than a literal "true" reads as unresolved, including a
	// missing field (every thread file written before THREAD-01) and a
	// hand-edited "false" — there is no third state to mismatch on.
	resolved := strings.EqualFold(fields["resolved"], "true")

	body = skipBlank(body)
	var quote []string
	for len(body) > 0 && strings.HasPrefix(body[0], ">") {
		quote = append(quote, body[0])
		body = body[1:]
	}

	posted, err := parseComments(skipBlank(body))
	if err != nil {
		return "", nil, err
	}

	return docPath, &thread{
		anchor:   anchor,
		quote:    linesFromQuote(quote),
		posted:   posted,
		resolved: resolved,
	}, nil
}

// parseComments walks the comment sections of a thread file body, in order.
func parseComments(lines []string) ([]comment, error) {
	var out []comment
	for i := 0; i < len(lines); {
		m := commentHeaderRE.FindStringSubmatch(lines[i])
		if m == nil {
			return nil, fmt.Errorf("expected a comment header (## author — time), got %q", lines[i])
		}
		author, rawTime := m[1], m[2]
		at, err := time.Parse(time.RFC3339Nano, rawTime)
		if err != nil {
			return nil, fmt.Errorf("comment %q: bad timestamp %q: %w", author, rawTime, err)
		}
		i++
		start := i
		for i < len(lines) && !strings.HasPrefix(lines[i], "## ") {
			i++
		}
		out = append(out, comment{
			author: author,
			body:   strings.Trim(strings.Join(lines[start:i], "\n"), "\n"),
			at:     at,
		})
		i = skipBlankFrom(lines, i)
	}
	return out, nil
}

// quoteAsLines renders a (possibly multi-line) quote as markdown blockquote
// lines, the same "> " convention export.go's quoteBlock uses.
func quoteAsLines(quote string) []string {
	src := strings.Split(quote, "\n")
	out := make([]string, len(src))
	for i, l := range src {
		if l == "" {
			out[i] = ">"
		} else {
			out[i] = "> " + l
		}
	}
	return out
}

// linesFromQuote is quoteAsLines' inverse.
func linesFromQuote(lines []string) string {
	out := make([]string, len(lines))
	for i, l := range lines {
		out[i] = strings.TrimPrefix(strings.TrimPrefix(l, ">"), " ")
	}
	return strings.Join(out, "\n")
}

func skipBlank(lines []string) []string {
	for len(lines) > 0 && strings.TrimSpace(lines[0]) == "" {
		lines = lines[1:]
	}
	return lines
}

func skipBlankFrom(lines []string, i int) int {
	for i < len(lines) && strings.TrimSpace(lines[i]) == "" {
		i++
	}
	return i
}
