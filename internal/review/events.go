// Event log: an append-only, one-event-per-line record of everything that
// changes a review, at .margin/events.log. It exists so a listener — the
// `margin comments wait` command, an agent tailing the file, a filesystem
// watcher — can answer "what happened since the last thing I saw" without
// re-parsing or diffing thread files. Answers Q-0003: the identity the wait
// command's --since cursor names is an *event* id, not a comment id, and the
// log is stored separately from thread files (D13), which are unchanged and
// gain no id field.
package review

import (
	"crypto/rand"
	"encoding/binary"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
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
	id      string    // 26-char ULID; the --since cursor
	at      time.Time // when the event happened, UTC
	kind    eventType
	doc     string // document path relative to the review root
	anchor  string // the thread's anchor, ^-prefixed as in thread files
	author  string // who performed the action
	comment int    // 0-based index into the thread's posted comments; -1 for a thread-level event
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

// marshalEvent renders one event as a log line: seven fields, tab-separated —
// id, UTC timestamp, type, document path, anchor, author, and the comment
// index (`-` when the event is about the thread as a whole). The separator is
// a tab because a document path or anchor never contains one; the author is
// free-form, so it is sanitized rather than allowed to corrupt the line.
func marshalEvent(ev event) string {
	comment := "-"
	if ev.comment >= 0 {
		comment = strconv.Itoa(ev.comment)
	}
	return strings.Join([]string{
		ev.id,
		ev.at.UTC().Format(time.RFC3339Nano),
		string(ev.kind),
		sanitizeLogField(ev.doc),
		sanitizeLogField(ev.anchor),
		sanitizeLogField(ev.author),
		comment,
	}, "\t")
}

// sanitizeLogField strips the characters that would corrupt a tab-separated
// line out of free-form fields (the author's name, in practice). Not applied
// to id, type, timestamp or comment index, whose shapes are fixed.
func sanitizeLogField(s string) string {
	return strings.NewReplacer("\t", " ", "\r", " ", "\n", " ").Replace(s)
}

// parseEvent is marshalEvent's inverse. A malformed line is an error: a
// listener or a future version must be able to trust the log, and a silent
// skip would drop an event the same way a silent thread-file failure would
// drop a comment.
func parseEvent(line string) (event, error) {
	fields := strings.Split(line, "\t")
	if len(fields) != 7 {
		return event{}, fmt.Errorf("event log: malformed line %q", line)
	}
	if !validULID(fields[0]) {
		return event{}, fmt.Errorf("event log: bad event id %q in line %q", fields[0], line)
	}
	at, err := time.Parse(time.RFC3339Nano, fields[1])
	if err != nil {
		return event{}, fmt.Errorf("event log: bad timestamp %q: %w", fields[1], err)
	}
	comment := -1
	if fields[6] != "-" {
		n, err := strconv.Atoi(fields[6])
		if err != nil || n < 0 {
			return event{}, fmt.Errorf("event log: bad comment index %q in line %q", fields[6], line)
		}
		comment = n
	}
	return event{
		id:      fields[0],
		at:      at,
		kind:    eventType(fields[2]),
		doc:     fields[3],
		anchor:  fields[4],
		author:  fields[5],
		comment: comment,
	}, nil
}

// readEventsAfter returns the events of root's log strictly after the event
// whose id is cursor, in file order. The cursor is a position in the file, not
// a string to compare against: two events sharing a millisecond are ordered by
// where they were appended, so the filter matches the id and takes everything
// after its line — exactly how D13 says a wait command must resolve
// same-millisecond ties. An empty cursor means the whole log is new. A cursor
// that matches no event is an error: a caller pointing at the wrong log, or
// passing a stale id, should be told, not silently handed the whole file.
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

// newEventID returns a 26-character time-ordered id (a ULID): the first 10
// characters encode the current time to the millisecond, the last 16 are
// random. Lexicographic order of the string is creation order at millisecond
// granularity — which lets a --since cursor be a plain string comparison while
// the append-only file position resolves the same-millisecond ties.
func newEventID() string {
	var b [16]byte
	binary.BigEndian.PutUint64(b[:8], uint64(time.Now().UnixMilli())<<16)
	if _, err := rand.Read(b[6:]); err != nil {
		// crypto/rand failing is practically unrecoverable; fall back to a
		// time-seeded value so a valid-shaped id is still produced.
		binary.BigEndian.PutUint64(b[8:], uint64(time.Now().UnixNano()))
	}
	return encodeULID(b)
}

// ulidEnc is ULID's Crockford base32 alphabet: digits first, then the letters
// minus I, L, O and U, which are excluded to avoid confusion with 1, l, 0 and
// "u" in hex-ish text.
const ulidEnc = "0123456789ABCDEFGHJKMNPQRSTVWXYZ"

// encodeULID renders a 128-bit value — 48 bits of millisecond timestamp in the
// most significant bits, 80 bits of entropy — as its 26-character Crockford
// base32 spelling, most significant group first.
func encodeULID(b [16]byte) string {
	var out [26]byte
	bit := 0
	for i := range out {
		var v byte
		for j := 0; j < 5; j++ {
			v <<= 1
			if bit < 128 {
				v |= (b[bit>>3] >> (7 - (bit & 7))) & 1
			}
			bit++
		}
		out[i] = ulidEnc[v]
	}
	return string(out[:])
}

// validULID reports whether s is a well-formed 26-character ULID. The final
// character carries only three real bits, but any alphabet character is
// acceptable there, so a length plus alphabet check is all that is needed.
func validULID(s string) bool {
	if len(s) != 26 {
		return false
	}
	for i := 0; i < len(s); i++ {
		if ulidDec(s[i]) < 0 {
			return false
		}
	}
	return true
}

// ulidDec maps a Crockford base32 character to its value, or -1 if invalid.
// Lowercase letters are accepted for a hand-written log line; the writer
// always emits uppercase.
func ulidDec(c byte) int {
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
