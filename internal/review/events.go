// Event log: an append-only, one-event-per-line record of everything that
// changes a review, at .margin/events.log. It exists so a listener — the
// `margin comments wait` command, an agent tailing the file, a filesystem
// watcher — can answer "what happened since the last thing I saw" without
// re-parsing or diffing thread files. Answers Q-0003: the identity the wait
// command's --since cursor names is an *event* id, not a comment id, and the
// log is stored separately from thread files (D13), which are unchanged and
// gain no id field. The line shape — JSONL, compact 13-character ids, unix
// timestamps at second precision, comment text on comment-level events — is
// D14 as extended by D15.
package review

import (
	"crypto/rand"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// eventType names one kind of review event, the second field of every log
// line. The set is deliberately small: whatever changes a thread file in a way
// a listener might care about gets an event; nothing else does.
type eventType string

const (
	eventCommentPosted    eventType = "comment.posted"
	eventCommentUpdated   eventType = "comment.updated"
	eventCommentDeleted   eventType = "comment.deleted"
	eventCommentRestored  eventType = "comment.restored"
	eventThreadResolved   eventType = "thread.resolved"
	eventThreadUnresolved eventType = "thread.unresolved"
	eventThreadDeleted    eventType = "thread.deleted"
	eventThreadRestored   eventType = "thread.restored"
)

// event is one entry of the event log. id and at are filled in by appendEvent;
// the call sites only describe what happened.
type event struct {
	id      string    // 13-char time-ordered id; the --since cursor
	at      time.Time // when the event happened, UTC, whole seconds
	kind    eventType
	doc     string // document path relative to the review root
	anchor  string // the thread's anchor, ^-prefixed as in thread files
	author  string // who performed the action
	comment int    // 0-based index into the thread's posted comments; -1 for a thread-level event
	text    string // the comment's body at emit time; empty for a thread-level event
}

// eventLine is one log line's JSON shape (D14, extended by D15). Kept separate
// from event so the on-disk form — id, unix seconds, a `type` key that would
// fight the Go keyword — does not leak into the in-memory struct.
type eventLine struct {
	ID      string    `json:"id"`
	At      int64     `json:"at"`
	Type    eventType `json:"type"`
	Doc     string    `json:"doc"`
	Anchor  string    `json:"anchor"`
	Author  string    `json:"author"`
	Comment int       `json:"comment"`
	Text    string    `json:"text,omitempty"`
}

// eventsLogPath is the append-only event log for a review root: one file for
// the whole root, a sibling of threads/ inside .margin, so a listener can
// fsnotify or tail a single file instead of watching the thread tree (the
// maintainer's answer to Q-0003).
func eventsLogPath(root string) string {
	return filepath.Join(root, ".margin", "events.log")
}

// appendEvent records one event by appending a single line to the review
// root's event log. The log is append-only: margin never rewrites or truncates
// it, and each line is written as one O_APPEND write syscall, so concurrent
// writers (a running TUI and a `margin comment add`, say) cannot interleave
// within a line.
//
// The thread file is the source of truth (D5) and the event log is a
// notification on top of it: appendEvent must be called only after the thread
// write it announces has succeeded, and a caller must not let a failure here
// fail the write it records — the words are already safe, only the notice is
// lost.
func appendEvent(root string, ev event) error {
	if ev.id == "" {
		ev.id = newEventID()
	}
	if ev.at.IsZero() {
		ev.at = time.Now()
	}
	ev.at = time.Unix(ev.at.Unix(), 0) // the log's timestamps are whole seconds (D14)
	path := eventsLogPath(root)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("event log: %w", err)
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return fmt.Errorf("event log: %w", err)
	}
	defer f.Close()
	if _, err := f.WriteString(marshalEvent(ev) + "\n"); err != nil {
		return fmt.Errorf("event log: %w", err)
	}
	return nil
}

// marshalEvent renders one event as a JSONL line: a single JSON object with
// the id, the unix-second timestamp, the type, the document path, the anchor,
// the author, the comment index (-1 for a thread-level event) and, for
// comment-level events, the comment's body text. JSON is what keeps a
// free-form field safe: encoding/json escapes tabs, quotes and newlines, so a
// line is one physical line whatever the author — or the comment — is called,
// and the field-sanitizer the tab-separated format needed is gone (D14). The
// text is omitted on thread-level events (D15): nothing was said, so the line
// stays compact, and an agent parses one shape — absent text means a
// thread-level event.
func marshalEvent(ev event) string {
	// Marshal cannot fail: eventLine holds only strings, an int64 and an
	// eventType, all JSON-native. A failure here would be a bug, not a
	// recoverable error, so it panics rather than producing a half line.
	b, err := json.Marshal(eventLine{
		ID:      ev.id,
		At:      ev.at.Unix(),
		Type:    ev.kind,
		Doc:     ev.doc,
		Anchor:  ev.anchor,
		Author:  ev.author,
		Comment: ev.comment,
		Text:    ev.text,
	})
	if err != nil {
		panic("event log: marshal event: " + err.Error())
	}
	return string(b)
}

// parseEvent is marshalEvent's inverse. A malformed line is an error: a
// listener or a future version must be able to trust the log, and a silent
// skip would drop an event the same way a silent thread-file failure would
// drop a comment. The cursor depends on the id, so a well-formed id is
// required; the comment index is range-checked. Absent optional fields fall
// back to zero values, exactly as JSON semantics say.
func parseEvent(line string) (event, error) {
	var el eventLine
	if err := json.Unmarshal([]byte(line), &el); err != nil {
		return event{}, fmt.Errorf("event log: malformed line %q", line)
	}
	if !validEventID(el.ID) {
		return event{}, fmt.Errorf("event log: bad event id %q in line %q", el.ID, line)
	}
	if el.Comment < -1 {
		return event{}, fmt.Errorf("event log: bad comment index %d in line %q", el.Comment, line)
	}
	return event{
		id:      el.ID,
		at:      time.Unix(el.At, 0),
		kind:    el.Type,
		doc:     el.Doc,
		anchor:  el.Anchor,
		author:  el.Author,
		comment: el.Comment,
		text:    el.Text,
	}, nil
}

// readEventsAfter returns the events of root's log strictly after the event
// whose id is cursor, in file order. The cursor is a position in the file, not
// a string to compare against: two events sharing a second (the log's
// timestamp granularity, D14) are ordered by where they were appended, so the
// filter matches the id and takes everything after its line — exactly how D13
// says a wait command must resolve ties, at whatever granularity the id
// carries. An empty cursor means the whole log is new. A cursor that matches
// no event is an error: a caller pointing at the wrong log, or passing a stale
// id, should be told, not silently handed the whole file.
func readEventsAfter(root, cursor string) ([]event, error) {
	evs, err := readEvents(root)
	if err != nil {
		return nil, err
	}
	if cursor == "" {
		return evs, nil
	}
	for i := range evs {
		if evs[i].id == cursor {
			return evs[i+1:], nil
		}
	}
	return nil, fmt.Errorf("event log: no event with id %s", cursor)
}

// readEvents parses every completed line of the review root's event log, in
// file order. A log that does not exist yet is not an error — it just means
// nothing has happened — but a malformed *completed* line is: the log is a
// contract, and a reader must not silently drop an event. The one normal
// exception is the final line when the file does not end in a newline: every
// write ends with one, so an unterminated tail is the fragment of an append
// that was still in flight, and it is skipped, not reported.
func readEvents(root string) ([]event, error) {
	data, err := os.ReadFile(eventsLogPath(root))
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("event log: %w", err)
	}
	lines := strings.Split(string(data), "\n")
	if len(lines) > 0 && lines[len(lines)-1] != "" {
		lines = lines[:len(lines)-1]
	}
	out := make([]event, 0, len(lines))
	for _, l := range lines {
		if l == "" {
			continue
		}
		ev, err := parseEvent(l)
		if err != nil {
			return nil, err
		}
		out = append(out, ev)
	}
	return out, nil
}

// --- event ids -------------------------------------------------------------

// newEventID returns a 13-character time-ordered id: seven characters of
// Crockford base32 encoding the current unix time in seconds, then six of
// randomness. Lexicographic order of the id is creation order at second
// granularity — the fixed-width prefix makes string order chronological — so
// a --since cursor is a plain string while the append-only file position
// resolves the same-second ties (D13). D14: chosen over the 26-char ULID
// because the id mostly needs to be an id — time-ordered, unique enough,
// short enough to copy and compare — and 13 characters is half the length.
func newEventID() string {
	sec := uint64(time.Now().Unix())
	var rnd [4]byte
	if _, err := rand.Read(rnd[:]); err != nil {
		// crypto/rand failing is practically unrecoverable; fall back to a
		// time-seeded value so a valid-shaped id is still produced.
		binary.BigEndian.PutUint32(rnd[:], uint32(time.Now().UnixNano()))
	}
	return encodeTimePrefix(sec) + encodeRandom(rnd)
}

// idEnc is Crockford base32: digits first, then the letters minus I, L, O and
// U, which are excluded to avoid confusion with 1, l, 0 and "u" in hex-ish
// text. The alphabet's ASCII order is its value order, which is what makes a
// fixed-width encoded time sort chronologically.
const idEnc = "0123456789ABCDEFGHJKMNPQRSTVWXYZ"

// encodeTimePrefix renders a unix second count as its fixed-width 7-character
// Crockford base32 spelling, most significant group first. Fixed width is the
// whole point: a 1-second count encodes as 0000001, not as the one-char 1, so
// every id of a later second still sorts after every id of an earlier one.
// 35 bits of seconds covers until the year 3058.
func encodeTimePrefix(sec uint64) string {
	var out [7]byte
	for i := range out {
		out[i] = idEnc[(sec>>(5*uint(6-i)))&0x1f]
	}
	return string(out[:])
}

// encodeRandom renders 30 bits of entropy as six Crockford characters, the
// id's uniqueness within a second.
func encodeRandom(rnd [4]byte) string {
	v := binary.BigEndian.Uint32(rnd[:]) >> 2
	var out [6]byte
	for i := 0; i < 6; i++ {
		out[i] = idEnc[(v>>(5*uint(5-i)))&0x1f]
	}
	return string(out[:])
}

// validEventID reports whether s is a well-formed 13-character event id.
func validEventID(s string) bool {
	if len(s) != 13 {
		return false
	}
	for i := 0; i < len(s); i++ {
		if idDec(s[i]) < 0 {
			return false
		}
	}
	return true
}

// idDec maps a Crockford base32 character to its value, or -1 if invalid.
// Lowercase letters are accepted for a hand-written log line; the writer
// always emits uppercase.
func idDec(c byte) int {
	if c >= 'a' && c <= 'z' {
		c -= 'a' - 'A'
	}
	switch {
	case c >= '0' && c <= '9':
		return int(c - '0')
	case c >= 'A' && c <= 'Z':
		switch c {
		case 'I', 'L', 'O', 'U':
			return -1
		}
		// Crockford base32 drops I, L, O and U, so the letter values are not
		// A=10, B=11 … straight through the alphabet: each skipped letter
		// shifts everything after it down by one.
		n := int(c-'A') + 10
		if c > 'I' {
			n--
		}
		if c > 'L' {
			n--
		}
		if c > 'O' {
			n--
		}
		if c > 'U' {
			n--
		}
		return n
	default:
		return -1
	}
}
