package review

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"
)

// decodeTimePrefix extracts the unix second count a 13-char event id embeds,
// the inverse of encodeTimePrefix's first seven characters. Test-only: the
// product compares ids as strings and never needs to decode one.
func decodeTimePrefix(s string) (sec uint64, ok bool) {
	if !validEventID(s) {
		return 0, false
	}
	var v uint64
	for i := 0; i < 7; i++ {
		c := idDec(s[i])
		if c < 0 {
			return 0, false
		}
		v = v<<5 | uint64(c)
	}
	return v, true
}

// TestEncodeTimePrefixKnownValues pins the id's time prefix against values
// worked out by hand, independently of the implementation: all-zero bits; a
// 1-second count — which must come out 0000001, not a one-char 1, because the
// fixed width is what keeps lexicographic order chronological; and all-ones,
// whose letter-heavy spelling exercises the Crockford letter values the
// all-digit cases never touch.
func TestEncodeTimePrefixKnownValues(t *testing.T) {
	if got := encodeTimePrefix(0); got != "0000000" {
		t.Errorf("encodeTimePrefix(0) = %q, want all-zeroes", got)
	}
	if got := encodeTimePrefix(1); got != "0000001" {
		t.Errorf("encodeTimePrefix(1s) = %q, want the hand-derived spelling", got)
	}
	maxSec := uint64(1<<35 - 1)
	if got := encodeTimePrefix(maxSec); got != "ZZZZZZZ" {
		t.Errorf("encodeTimePrefix(all-ones) = %q, want 7 Zs", got)
	}
	if sec, ok := decodeTimePrefix("0000000ZZZZZZ"); !ok || sec != 0 {
		t.Errorf("decodeTimePrefix(0) = %d, %v; want 0, true", sec, ok)
	}
	if sec, ok := decodeTimePrefix("0000001ZZZZZZ"); !ok || sec != 1 {
		t.Errorf("decodeTimePrefix(1s) = %d, %v; want 1, true", sec, ok)
	}
	if sec, ok := decodeTimePrefix("zzzzzzzzzzzzz"); !ok || sec != maxSec {
		t.Errorf("decodeTimePrefix(lowercase) = %d, %v; want %d, true", sec, ok, maxSec)
	}
}

// TestNewEventIDIsTimeOrdered: ids are 13 chars of the Crockford alphabet,
// embed a current second, differ from each other, and sort lexicographically
// in non-decreasing time order even across the random suffixes.
func TestNewEventIDIsTimeOrdered(t *testing.T) {
	before := time.Now().Unix()
	a := newEventID()
	after := time.Now().Unix()
	if !validEventID(a) {
		t.Fatalf("newEventID() = %q, not a valid event id", a)
	}
	if sec, ok := decodeTimePrefix(a); !ok || sec < uint64(before) || sec > uint64(after) {
		t.Errorf("newEventID() embeds timestamp %d, want within [%d,%d]", sec, before, after)
	}
	if b := newEventID(); b == a {
		t.Errorf("two newEventID() calls both returned %q", a)
	}

	ids := make([]string, 200)
	for i := range ids {
		ids[i] = newEventID()
	}
	sort.Strings(ids)
	var prev uint64
	for _, id := range ids {
		sec, ok := decodeTimePrefix(id)
		if !ok {
			t.Fatalf("sorted id %q does not decode", id)
		}
		if sec < prev {
			t.Fatalf("sorted ids out of chronological order: %d then %d", prev, sec)
		}
		prev = sec
	}
}

// TestEventLineShape pins the exact on-disk JSONL line D14 specifies (as
// extended by D15: a comment-level event carries its body as `text`), and that
// it round-trips through parseEvent.
func TestEventLineShape(t *testing.T) {
	ev := event{
		id:      "1N7KFA0P7KFA0",
		at:      time.Date(2026, 8, 10, 12, 0, 0, 0, time.UTC),
		kind:    eventCommentPosted,
		doc:     "docs/spec.md",
		anchor:  "^a1b2c3",
		author:  "agent",
		comment: 2,
		text:    "rework the third paragraph",
	}
	want := `{"id":"1N7KFA0P7KFA0","at":1786363200,"type":"comment.posted","doc":"docs/spec.md","anchor":"^a1b2c3","author":"agent","comment":2,"text":"rework the third paragraph"}`
	if got := marshalEvent(ev); got != want {
		t.Errorf("marshalEvent() = %q, want %q", got, want)
	}

	round, err := parseEvent(want)
	if err != nil {
		t.Fatalf("parseEvent: %v", err)
	}
	if round.id != ev.id || round.kind != ev.kind || round.doc != ev.doc ||
		round.anchor != ev.anchor || round.author != ev.author || round.comment != ev.comment ||
		round.text != ev.text {
		t.Errorf("parseEvent round-trip = %+v, want %+v", round, ev)
	}
	if !round.at.Equal(ev.at) {
		t.Errorf("round-trip at = %v, want %v", round.at, ev.at)
	}
}

// TestEventLineThreadLevel: a thread-level event writes comment -1 and reads
// back as -1, and its line omits the text field entirely (D15) — nothing was
// said, so the line stays compact, and the reader's shape is "no text = a
// thread-level event".
func TestEventLineThreadLevel(t *testing.T) {
	ev := event{
		id:      "1N7KFA0R8KFA1",
		at:      time.Date(2026, 8, 10, 12, 0, 0, 0, time.UTC),
		kind:    eventThreadResolved,
		doc:     "doc.md",
		anchor:  "^b2",
		author:  "toly",
		comment: -1,
	}
	want := `{"id":"1N7KFA0R8KFA1","at":1786363200,"type":"thread.resolved","doc":"doc.md","anchor":"^b2","author":"toly","comment":-1}`
	if got := marshalEvent(ev); got != want {
		t.Errorf("marshalEvent() = %q, want %q", got, want)
	}
	round, err := parseEvent(want)
	if err != nil {
		t.Fatalf("parseEvent: %v", err)
	}
	if round.comment != -1 || round.kind != eventThreadResolved || round.text != "" {
		t.Errorf("round = %+v, want comment -1 thread.resolved with no text", round)
	}
}

// TestParseEventWithoutTextDefaultsEmpty: a line that predates D15's text
// field — or a hand-written one that omits it — parses with empty text, per
// the JSON contract that absent optional fields fall back to zero values. An
// old log stays readable.
func TestParseEventWithoutTextDefaultsEmpty(t *testing.T) {
	line := `{"id":"1N7KFA0P7KFA0","at":1786363200,"type":"comment.posted","doc":"doc.md","anchor":"^a","author":"x","comment":0}`
	ev, err := parseEvent(line)
	if err != nil {
		t.Fatalf("parseEvent: %v", err)
	}
	if ev.text != "" {
		t.Errorf("text = %q, want empty for a line without the field", ev.text)
	}
}

// TestMarshalEventEscapesFreeFormFields: an author or a comment body carrying
// a tab, newline or quote must not split the JSON — encoding/json escapes
// them, so the event still parses back whole and the line is one physical
// line.
func TestMarshalEventEscapesFreeFormFields(t *testing.T) {
	ev := event{
		id:      "1N7KFA0P7KFA0",
		at:      time.Date(2026, 8, 10, 12, 0, 0, 0, time.UTC),
		kind:    eventCommentPosted,
		doc:     "doc.md",
		anchor:  "^a",
		author:  "ta\tb\nc\"quote",
		comment: 0,
		text:    "first line\nsecond line\twith a \"quote\"",
	}
	line := marshalEvent(ev)
	if strings.ContainsAny(line, "\t\n") {
		t.Fatalf("marshalEvent() = %q, contains a raw tab or newline", line)
	}
	round, err := parseEvent(line)
	if err != nil {
		t.Fatalf("parseEvent on escaped line: %v", err)
	}
	if round.author != ev.author {
		t.Errorf("author = %q, want %q (escaped and restored whole)", round.author, ev.author)
	}
	if round.text != ev.text {
		t.Errorf("text = %q, want %q (multi-line comment restored whole)", round.text, ev.text)
	}
}

// TestAppendEventAndReadEventsRoundTrip: events written through appendEvent
// come back through readEvents in order with their ids and timestamps filled
// in.
func TestAppendEventAndReadEventsRoundTrip(t *testing.T) {
	root := t.TempDir()
	if _, err := appendEvent(root, event{kind: eventCommentPosted, doc: "doc.md", anchor: "^a", author: "agent", comment: 0}); err != nil {
		t.Fatalf("appendEvent: %v", err)
	}
	if _, err := appendEvent(root, event{kind: eventThreadResolved, doc: "doc.md", anchor: "^a", author: "toly", comment: -1}); err != nil {
		t.Fatalf("appendEvent: %v", err)
	}

	events, err := readEvents(root)
	if err != nil {
		t.Fatalf("readEvents: %v", err)
	}
	if len(events) != 2 {
		t.Fatalf("readEvents returned %d events, want 2", len(events))
	}
	first, second := events[0], events[1]
	if first.kind != eventCommentPosted || first.doc != "doc.md" ||
		first.anchor != "^a" || first.author != "agent" || first.comment != 0 {
		t.Errorf("first event = %+v", first)
	}
	if second.kind != eventThreadResolved || second.comment != -1 {
		t.Errorf("second event = %+v", second)
	}
	if first.id == "" || second.id == "" || first.id == second.id {
		t.Errorf("ids not filled in: %q, %q", first.id, second.id)
	}
	if !validEventID(first.id) || !validEventID(second.id) {
		t.Errorf("ids not event ids: %q, %q", first.id, second.id)
	}
	if first.at.IsZero() || second.at.IsZero() {
		t.Errorf("timestamps not filled in: %v, %v", first.at, second.at)
	}
}

// TestReadEventsSkipsTornTail: a final unterminated line is the tail of an
// append still in flight and must be skipped, not treated as a malformed event
// or silently swallowed whole.
func TestReadEventsSkipsTornTail(t *testing.T) {
	root := t.TempDir()
	complete := marshalEvent(event{
		id:      "0000000000000",
		at:      time.Unix(0, 0).UTC(),
		kind:    eventCommentPosted,
		doc:     "doc.md",
		anchor:  "^a",
		author:  "x",
		comment: 0,
	})
	if err := os.MkdirAll(filepath.Dir(eventsLogPath(root)), 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(eventsLogPath(root), []byte(complete+"\n{\"id\":\"1N7KFA0"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	events, err := readEvents(root)
	if err != nil {
		t.Fatalf("readEvents: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("readEvents returned %d events, want 1 (torn tail skipped)", len(events))
	}
}

// TestReadEventsMissingLogIsEmpty: a review root nothing has happened in has no
// log, which is not an error.
func TestReadEventsMissingLogIsEmpty(t *testing.T) {
	events, err := readEvents(t.TempDir())
	if err != nil {
		t.Fatalf("readEvents: %v", err)
	}
	if len(events) != 0 {
		t.Fatalf("readEvents returned %d events, want none", len(events))
	}
}

// TestParseEventMalformed: a completed line that does not fit the format is an
// error — the log is a contract and a listener must not silently drop events.
func TestParseEventMalformed(t *testing.T) {
	cases := []string{
		"",
		"this is not a JSON line",
		`{"id":"short"}`, // id not 13 chars
		`{"id":"1N7KFA0P7KFAI"}`, // I is not in the Crockford alphabet
		`{"id":"1N7KFA0P7KFA0","at":1786363200,"type":"comment.posted","doc":"doc.md","anchor":"^a","author":"x","comment":-2}`,
		`{"id":`,
	}
	for _, line := range cases {
		if _, err := parseEvent(line); err == nil {
			t.Errorf("parseEvent(%q): want an error, got nil", line)
		}
	}
}

// TestThreadStoreEmitWritesEvent: emit fills in the store's document path and
// lands the event in the review root's log.
func TestThreadStoreEmitWritesEvent(t *testing.T) {
	root := t.TempDir()
	s := &threadStore{root: root, docPath: "docs/spec.md"}
	if _, err := s.emit(event{kind: eventThreadResolved, anchor: "^b2", author: "toly", comment: -1}); err != nil {
		t.Fatalf("emit: %v", err)
	}

	events, err := readEvents(root)
	if err != nil {
		t.Fatalf("readEvents: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("readEvents returned %d events, want 1", len(events))
	}
	ev := events[0]
	if ev.kind != eventThreadResolved || ev.doc != "docs/spec.md" ||
		ev.anchor != "^b2" || ev.author != "toly" || ev.comment != -1 {
		t.Errorf("event = %+v", ev)
	}
	if ev.text != "" {
		t.Errorf("thread-level event text = %q, want empty (D15 omits it)", ev.text)
	}
}

// TestThreadStoreEmitNilSafe: a nil store records nothing and is not an error,
// matching save — a seeded or ephemeral model must not touch disk.
func TestThreadStoreEmitNilSafe(t *testing.T) {
	var s *threadStore
	if _, err := s.emit(event{kind: eventCommentPosted}); err != nil {
		t.Fatalf("emit on nil store: %v", err)
	}
	if _, err := os.Stat(filepath.Join(t.TempDir(), ".margin")); !os.IsNotExist(err) {
		t.Fatal("nil store created .margin on disk")
	}
}

// TestAddCommentWritesEvent: `margin comment add` records a comment.posted
// event for the reply, carrying the doc path, anchor, author and the comment's
// index in the thread.
func TestAddCommentWritesEvent(t *testing.T) {
	root, docPath := writeDocUnderRoot(t, "# Title\n\nA commentable paragraph.\n")
	anchor := firstParagraphAnchor(t, docPath)
	if _, err := AddComment(docPath, anchor, "agent", "fixed in rev 3"); err != nil {
		t.Fatalf("AddComment: %v", err)
	}
	if _, err := AddComment(docPath, anchor, "agent", "second reply"); err != nil {
		t.Fatalf("AddComment: %v", err)
	}

	events, err := readEvents(root)
	if err != nil {
		t.Fatalf("readEvents: %v", err)
	}
	if len(events) != 2 {
		t.Fatalf("readEvents returned %d events, want 2", len(events))
	}
	first, second := events[0], events[1]
	if first.kind != eventCommentPosted || first.doc != "doc.md" ||
		first.anchor != anchor || first.author != "agent" || first.comment != 0 {
		t.Errorf("first event = %+v", first)
	}
	if first.text != "fixed in rev 3" {
		t.Errorf("first event text = %q, want the reply's text so the agent need not re-read the thread", first.text)
	}
	if second.kind != eventCommentPosted || second.comment != 1 || second.text != "second reply" {
		t.Errorf("second event = %+v, want comment index 1 carrying its text", second)
	}
}

// TestDeleteFocusedEventCarriesBody: tombstoning a comment emits
// comment.deleted carrying the comment's body — D11 keeps it in the thread
// file even when deleted, so a listener sees exactly what vanished without a
// second read (D15).
func TestDeleteFocusedEventCarriesBody(t *testing.T) {
	m := newTestModel(t)
	root := t.TempDir()
	m.store = &threadStore{root: root, docPath: "document.md"}

	m.at = cursor{entry: entryFor(t, m, convoAnchor), comment: 0}
	want := m.threads[convoAnchor].posted[0].body
	m.deleteFocused()

	events, err := readEvents(root)
	if err != nil {
		t.Fatalf("readEvents: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("readEvents returned %d events, want 1", len(events))
	}
	ev := events[0]
	if ev.kind != eventCommentDeleted || ev.anchor != convoAnchor || ev.comment != 0 {
		t.Errorf("event = %+v, want comment.deleted on %s comment 0", ev, convoAnchor)
	}
	if ev.text != want {
		t.Errorf("event text = %q, want %q (the tombstoned body, D11)", ev.text, want)
	}
}

// --- readEventsAfter ---------------------------------------------------------

// writeEvents writes raw log lines into a fresh review root, returning the
// root, so a test controls the exact bytes (ids, timestamps, ordering) the
// cursor logic must resolve.
func writeEvents(t *testing.T, lines ...string) string {
	t.Helper()
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Dir(eventsLogPath(root)), 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	var b strings.Builder
	for _, l := range lines {
		b.WriteString(l)
		b.WriteString("\n")
	}
	if err := os.WriteFile(eventsLogPath(root), []byte(b.String()), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	return root
}

// logLine builds a log line for a hand-controlled event, the shape the
// reader tests need to pin cursor behaviour without the appendEvent machinery.
func logLine(id string, at time.Time, kind eventType, doc, anchor, author string, comment int) string {
	return marshalEvent(event{
		id:      id,
		at:      at,
		kind:    kind,
		doc:     doc,
		anchor:  anchor,
		author:  author,
		comment: comment,
	})
}

// TestReadEventsAfterEmptyCursorReturnsEverything: with no cursor, every event
// is new — the first call of a polling loop sees the whole history.
func TestReadEventsAfterEmptyCursorReturnsEverything(t *testing.T) {
	at := time.Date(2026, 8, 10, 12, 0, 0, 0, time.UTC)
	root := writeEvents(t,
		logLine("1N7KFA0P7KFA0", at, eventCommentPosted, "doc.md", "^a", "toly", 0),
		logLine("1N7KFA0R8KFA1", at, eventCommentPosted, "doc.md", "^a", "agent", 1),
	)
	evs, err := readEventsAfter(root, "")
	if err != nil {
		t.Fatalf("readEventsAfter: %v", err)
	}
	if len(evs) != 2 {
		t.Fatalf("readEventsAfter(\"\") returned %d events, want 2", len(evs))
	}
}

// TestReadEventsAfterCursorExcludesIt: events strictly after the cursor come
// back in file order, the cursor itself excluded.
func TestReadEventsAfterCursorExcludesIt(t *testing.T) {
	at := time.Date(2026, 8, 10, 12, 0, 0, 0, time.UTC)
	root := writeEvents(t,
		logLine("1N7KFA0P7KFA0", at, eventCommentPosted, "doc.md", "^a", "toly", 0),
		logLine("1N7KFA0R8KFA1", at, eventThreadResolved, "doc.md", "^a", "agent", -1),
		logLine("1N7KFA0S9KFA2", at, eventCommentPosted, "doc.md", "^b", "toly", 0),
	)
	evs, err := readEventsAfter(root, "1N7KFA0R8KFA1")
	if err != nil {
		t.Fatalf("readEventsAfter: %v", err)
	}
	if len(evs) != 1 || evs[0].id != "1N7KFA0S9KFA2" {
		t.Errorf("readEventsAfter(id-0002) = %+v, want only id-0003", evs)
	}
}

// TestReadEventsAfterLastCursorIsEmpty: a cursor naming the last event leaves
// nothing after it — the "nothing new yet" state a poller starts in.
func TestReadEventsAfterLastCursorIsEmpty(t *testing.T) {
	at := time.Date(2026, 8, 10, 12, 0, 0, 0, time.UTC)
	root := writeEvents(t, logLine("1N7KFA0P7KFA0", at, eventCommentPosted, "doc.md", "^a", "toly", 0))
	evs, err := readEventsAfter(root, "1N7KFA0P7KFA0")
	if err != nil {
		t.Fatalf("readEventsAfter: %v", err)
	}
	if len(evs) != 0 {
		t.Errorf("readEventsAfter(last id) = %+v, want none", evs)
	}
}

// TestReadEventsAfterUnknownCursorErrors: a cursor matching no event is an
// error — a caller pointed at the wrong log (or carrying a stale id) must be
// told, not silently handed the whole file.
func TestReadEventsAfterUnknownCursorErrors(t *testing.T) {
	at := time.Date(2026, 8, 10, 12, 0, 0, 0, time.UTC)
	root := writeEvents(t, logLine("1N7KFA0P7KFA0", at, eventCommentPosted, "doc.md", "^a", "toly", 0))
	_, err := readEventsAfter(root, "1N7KFA0T0KFA3")
	if err == nil {
		t.Fatal("readEventsAfter with an unknown cursor reported success")
	}
	if !strings.Contains(err.Error(), "1N7KFA0T0KFA3") {
		t.Errorf("error does not name the bad cursor: %v", err)
	}
}

// TestReadEventsAfterResolvesTiesByFilePosition: two events sharing a second
// are ordered by where they sit in the file, not by comparing ids — a cursor
// into an append-only log means the tie is file order (D13).
// Here the second line's id sorts *before* the first's lexically, so a string
// comparison would hand back the wrong event.
func TestReadEventsAfterResolvesTiesByFilePosition(t *testing.T) {
	at := time.Date(2026, 8, 10, 12, 0, 0, 0, time.UTC)
	root := writeEvents(t,
		logLine("ZZZZZZZZZZZZZ", at, eventCommentPosted, "doc.md", "^a", "toly", 0),
		logLine("0000000000000", at, eventCommentPosted, "doc.md", "^a", "agent", 1),
	)
	evs, err := readEventsAfter(root, "ZZZZZZZZZZZZZ")
	if err != nil {
		t.Fatalf("readEventsAfter: %v", err)
	}
	if len(evs) != 1 || evs[0].id != "0000000000000" {
		t.Errorf("readEventsAfter(ZZZZ…) = %+v, want the later line (file order, not id order)", evs)
	}
}

// TestReadEventsAfterSurfacesMalformedLine: the cursor filter runs on the
// same parsed log as everything else, so a malformed completed line is still
// an error, not silently skipped.
func TestReadEventsAfterSurfacesMalformedLine(t *testing.T) {
	at := time.Date(2026, 8, 10, 12, 0, 0, 0, time.UTC)
	root := writeEvents(t,
		logLine("1N7KFA0P7KFA0", at, eventCommentPosted, "doc.md", "^a", "toly", 0),
		"this is not an event line",
	)
	if _, err := readEventsAfter(root, "1N7KFA0P7KFA0"); err == nil {
		t.Fatal("readEventsAfter over a malformed log reported success")
	}
}

// --- WaitEvents --------------------------------------------------------------

// waitRoot creates a review root with a doc.md in it, so WaitEvents' path
// argument can resolve to a root the test controls without depending on the
// process working directory.
func waitRoot(t *testing.T) (root, docPath string) {
	t.Helper()
	return writeDocUnderRoot(t, "# Title\n\nA commentable paragraph.\n")
}

// TestWaitEventsReturnsEventsImmediately: events already past the cursor come
// back on the first poll, no waiting.
func TestWaitEventsReturnsEventsImmediately(t *testing.T) {
	root, docPath := waitRoot(t)
	if _, err := appendEvent(root, event{kind: eventCommentPosted, doc: "doc.md", anchor: "^a", author: "toly", comment: 0}); err != nil {
		t.Fatalf("appendEvent: %v", err)
	}
	lines, err := WaitEvents(docPath, "", time.Second)
	if err != nil {
		t.Fatalf("WaitEvents: %v", err)
	}
	if len(lines) != 1 {
		t.Fatalf("WaitEvents returned %d lines, want 1", len(lines))
	}
	if !strings.Contains(lines[0], "comment.posted") {
		t.Errorf("line does not carry the event:\n%s", lines[0])
	}
}

// TestWaitEventsTimesOut: with no events past the cursor and a small timeout,
// the wait gives up with ErrWaitTimeout rather than waiting forever.
func TestWaitEventsTimesOut(t *testing.T) {
	_, docPath := waitRoot(t)
	start := time.Now()
	_, err := WaitEvents(docPath, "", 30*time.Millisecond)
	if err != ErrWaitTimeout {
		t.Fatalf("WaitEvents = %v, want ErrWaitTimeout", err)
	}
	if elapsed := time.Since(start); elapsed > 2*time.Second {
		t.Errorf("timeout took %v, want it to respect the small deadline", elapsed)
	}
}

// TestWaitEventsBlocksUntilNewEvent: an empty log with no events yet blocks
// until an event lands, then returns it — the "agent waits for the reviewer's
// first comment" case.
func TestWaitEventsBlocksUntilNewEvent(t *testing.T) {
	root, docPath := waitRoot(t)
	go func() {
		time.Sleep(100 * time.Millisecond)
		_, _ = appendEvent(root, event{kind: eventThreadResolved, doc: "doc.md", anchor: "^a", author: "toly", comment: -1})
	}()
	lines, err := WaitEvents(docPath, "", 5*time.Second)
	if err != nil {
		t.Fatalf("WaitEvents: %v", err)
	}
	if len(lines) != 1 || !strings.Contains(lines[0], "thread.resolved") {
		t.Errorf("WaitEvents after a background append = %v, want the thread.resolved line", lines)
	}
}

// TestWaitEventsCursorIsFilePosition: the cursor filters by file position, so
// two events in one second come out in file order even when their ids
// sort the other way — the D13 tie rule, observed through the wait itself.
func TestWaitEventsCursorIsFilePosition(t *testing.T) {
	root, docPath := waitRoot(t)
	at := time.Date(2026, 8, 10, 12, 0, 0, 0, time.UTC)
	if err := os.MkdirAll(filepath.Dir(eventsLogPath(root)), 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	data := logLine("ZZZZZZZZZZZZZ", at, eventCommentPosted, "doc.md", "^a", "toly", 0) + "\n" +
		logLine("0000000000000", at, eventCommentPosted, "doc.md", "^a", "agent", 1) + "\n"
	if err := os.WriteFile(eventsLogPath(root), []byte(data), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	lines, err := WaitEvents(docPath, "ZZZZZZZZZZZZZ", time.Second)
	if err != nil {
		t.Fatalf("WaitEvents: %v", err)
	}
	if len(lines) != 1 || !strings.Contains(lines[0], "0000000000000") {
		t.Errorf("WaitEvents(ZZZZ…) = %v, want the later line (file order)", lines)
	}
}
