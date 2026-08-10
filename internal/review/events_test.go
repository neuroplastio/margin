package review

import (
	"encoding/binary"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"
)

// decodeULID extracts the millisecond timestamp a ULID embeds, the inverse of
// encodeULID's timestamp half. Test-only: the product compares ids as strings
// and never needs to decode one.
func decodeULID(s string) (ms uint64, ok bool) {
	if !validULID(s) {
		return 0, false
	}
	var b [16]byte
	bit := 0
	for i := 0; i < 26; i++ {
		v := ulidDec(s[i])
		bits := 5
		if i == 25 {
			bits = 3 // the final character carries only the last three bits
		}
		for j := bits - 1; j >= 0; j-- {
			if bit < 128 {
				b[bit>>3] |= byte((v>>j)&1) << (7 - (bit & 7))
			}
			bit++
		}
	}
	return binary.BigEndian.Uint64(b[:8]) >> 16, true
}

// TestEncodeULIDKnownValues pins the encoder and decoder against values
// worked out by hand, independently of the implementation: all-zero bits; a
// 1ms timestamp with no entropy (the timestamp's value 1 lives in the last
// five-bit group of the timestamp field, which encodes as 4 — 0b00100 — in
// character 9); and all-ones bits, whose letter-heavy spelling exercises the
// Crockford letter values the all-digit cases never touch.
func TestEncodeULIDKnownValues(t *testing.T) {
	var zero [16]byte
	if got := encodeULID(zero); got != "00000000000000000000000000" {
		t.Errorf("encodeULID(0) = %q, want all-zeroes", got)
	}
	one := [16]byte{0, 0, 0, 0, 0, 1}
	if got := encodeULID(one); got != "00000000040000000000000000" {
		t.Errorf("encodeULID(1ms) = %q, want the hand-derived spelling", got)
	}
	maxMS := uint64(1<<48 - 1)
	var all [16]byte
	for i := range all {
		all[i] = 0xFF
	}
	// The final character carries only three real bits; the encoder pads the
	// remaining two with zeros at the low end, so all-ones reads 0b11100 = W.
	if got := encodeULID(all); got != "ZZZZZZZZZZZZZZZZZZZZZZZZZW" {
		t.Errorf("encodeULID(all-ones) = %q, want 25 Zs + W", got)
	}
	if ms, ok := decodeULID("00000000000000000000000000"); !ok || ms != 0 {
		t.Errorf("decodeULID(0) = %d, %v; want 0, true", ms, ok)
	}
	if ms, ok := decodeULID("00000000040000000000000000"); !ok || ms != 1 {
		t.Errorf("decodeULID(1ms) = %d, %v; want 1, true", ms, ok)
	}
	if ms, ok := decodeULID("ZZZZZZZZZZZZZZZZZZZZZZZZZW"); !ok || ms != maxMS {
		t.Errorf("decodeULID(all-ones) = %d, %v; want %d, true", ms, ok, maxMS)
	}
	if ms, ok := decodeULID("zzzzzzzzzzzzzzzzzzzzzzzzzw"); !ok || ms != maxMS {
		t.Errorf("decodeULID(lowercase) = %d, %v; want %d, true", ms, ok, maxMS)
	}
}

// TestNewEventIDIsATimeOrderedULID: ids are 26 chars of the Crockford
// alphabet, embed a current timestamp, differ from each other, and sort
// lexicographically in non-decreasing time order even across the random
// suffixes.
func TestNewEventIDIsATimeOrderedULID(t *testing.T) {
	before := time.Now().UnixMilli()
	a := newEventID()
	after := time.Now().UnixMilli()
	if !validULID(a) {
		t.Fatalf("newEventID() = %q, not a valid ULID", a)
	}
	if ms, ok := decodeULID(a); !ok || ms < uint64(before) || ms > uint64(after) {
		t.Errorf("newEventID() embeds timestamp %d, want within [%d,%d]", ms, before, after)
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
		ms, ok := decodeULID(id)
		if !ok {
			t.Fatalf("sorted id %q does not decode", id)
		}
		if ms < prev {
			t.Fatalf("sorted ids out of chronological order: %d then %d", prev, ms)
		}
		prev = ms
	}
}

// TestEventLineShape pins the exact on-disk line D13 specifies, and that it
// round-trips through parseEvent.
func TestEventLineShape(t *testing.T) {
	ev := event{
		id:      "01ARZ3NDEKTSV4RRFFQ69G5FAV",
		at:      time.Date(2026, 8, 10, 12, 0, 0, 123000000, time.UTC),
		kind:    eventCommentPosted,
		doc:     "docs/spec.md",
		anchor:  "^a1b2c3",
		author:  "agent",
		comment: 2,
	}
	want := "01ARZ3NDEKTSV4RRFFQ69G5FAV\t2026-08-10T12:00:00.123Z\tcomment.posted\tdocs/spec.md\t^a1b2c3\tagent\t2"
	if got := marshalEvent(ev); got != want {
		t.Errorf("marshalEvent() = %q, want %q", got, want)
	}

	round, err := parseEvent(want)
	if err != nil {
		t.Fatalf("parseEvent: %v", err)
	}
	if round.id != ev.id || round.kind != ev.kind || round.doc != ev.doc ||
		round.anchor != ev.anchor || round.author != ev.author || round.comment != ev.comment {
		t.Errorf("parseEvent round-trip = %+v, want %+v", round, ev)
	}
	if !round.at.Equal(ev.at) {
		t.Errorf("round-trip at = %v, want %v", round.at, ev.at)
	}
}

// TestEventLineThreadLevel: a thread-level event writes `-` for the comment
// index and reads back as -1.
func TestEventLineThreadLevel(t *testing.T) {
	ev := event{
		id:      "01ARZ3NDEKTSV4RRFFQ69G5FAV",
		at:      time.Date(2026, 8, 10, 12, 0, 0, 0, time.UTC),
		kind:    eventThreadResolved,
		doc:     "doc.md",
		anchor:  "^b2",
		author:  "toly",
		comment: -1,
	}
	want := "01ARZ3NDEKTSV4RRFFQ69G5FAV\t2026-08-10T12:00:00Z\tthread.resolved\tdoc.md\t^b2\ttoly\t-"
	if got := marshalEvent(ev); got != want {
		t.Errorf("marshalEvent() = %q, want %q", got, want)
	}
	round, err := parseEvent(want)
	if err != nil {
		t.Fatalf("parseEvent: %v", err)
	}
	if round.comment != -1 || round.kind != eventThreadResolved {
		t.Errorf("round = %+v, want comment -1 thread.resolved", round)
	}
}

// TestMarshalEventSanitizesFreeFormFields: an author carrying a tab or newline
// must not split the line into more fields or a second line.
func TestMarshalEventSanitizesFreeFormFields(t *testing.T) {
	ev := event{
		id:      "01ARZ3NDEKTSV4RRFFQ69G5FAV",
		at:      time.Date(2026, 8, 10, 12, 0, 0, 0, time.UTC),
		kind:    eventCommentPosted,
		doc:     "doc.md",
		anchor:  "^a",
		author:  "tab\there\nnewline",
		comment: 0,
	}
	line := marshalEvent(ev)
	fields := strings.Split(line, "\t")
	if len(fields) != 7 {
		t.Fatalf("marshalEvent() = %q, split into %d fields", line, len(fields))
	}
	if fields[5] != "tab here newline" {
		t.Errorf("author field = %q, want tabs/newlines replaced", fields[5])
	}
	if _, err := parseEvent(line); err != nil {
		t.Fatalf("parseEvent on sanitized line: %v", err)
	}
}

// TestAppendEventAndReadEventsRoundTrip: events written through appendEvent
// come back through readEvents in order with their ids and timestamps filled
// in.
func TestAppendEventAndReadEventsRoundTrip(t *testing.T) {
	root := t.TempDir()
	if err := appendEvent(root, event{kind: eventCommentPosted, doc: "doc.md", anchor: "^a", author: "agent", comment: 0}); err != nil {
		t.Fatalf("appendEvent: %v", err)
	}
	if err := appendEvent(root, event{kind: eventThreadResolved, doc: "doc.md", anchor: "^a", author: "toly", comment: -1}); err != nil {
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
	if !validULID(first.id) || !validULID(second.id) {
		t.Errorf("ids not ULIDs: %q, %q", first.id, second.id)
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
		id:      "00000000000000000000000000",
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
	if err := os.WriteFile(eventsLogPath(root), []byte(complete+"\n01ARZ3NDEKTS"), 0o644); err != nil {
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
	id := "01ARZ3NDEKTSV4RRFFQ69G5FAV"
	cases := []string{
		"",
		id,
		id + "\t2026-08-10T12:00:00Z\tcomment.posted\tdoc.md\t^a",
		"not-a-ulid\t2026-08-10T12:00:00Z\tcomment.posted\tdoc.md\t^a\tx\t0",
		id + "\tbadtimestamp\tcomment.posted\tdoc.md\t^a\tx\t0",
		id + "\t2026-08-10T12:00:00Z\tcomment.posted\tdoc.md\t^a\tx\tzz",
		id + "\t2026-08-10T12:00:00Z\tcomment.posted\tdoc.md\t^a\tx\t-1",
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
	if err := s.emit(event{kind: eventThreadResolved, anchor: "^b2", author: "toly", comment: -1}); err != nil {
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
}

// TestThreadStoreEmitNilSafe: a nil store records nothing and is not an error,
// matching save — a seeded or ephemeral model must not touch disk.
func TestThreadStoreEmitNilSafe(t *testing.T) {
	var s *threadStore
	if err := s.emit(event{kind: eventCommentPosted}); err != nil {
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
	if second.kind != eventCommentPosted || second.comment != 1 {
		t.Errorf("second event = %+v, want comment index 1", second)
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

// eventLine builds a log line for a hand-controlled event, the shape the
// reader tests need to pin cursor behaviour without the appendEvent machinery.
func eventLine(id string, at time.Time, kind eventType, doc, anchor, author string, comment int) string {
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
		eventLine("01ARZ3NDEKTSV4RRFFQ69G5FAV", at, eventCommentPosted, "doc.md", "^a", "toly", 0),
		eventLine("01ARZ3NDEKTSV4RRFFQ69G5FAW", at, eventCommentPosted, "doc.md", "^a", "agent", 1),
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
		eventLine("01ARZ3NDEKTSV4RRFFQ69G5FAV", at, eventCommentPosted, "doc.md", "^a", "toly", 0),
		eventLine("01ARZ3NDEKTSV4RRFFQ69G5FAW", at, eventThreadResolved, "doc.md", "^a", "agent", -1),
		eventLine("01ARZ3NDEKTSV4RRFFQ69G5FAX", at, eventCommentPosted, "doc.md", "^b", "toly", 0),
	)
	evs, err := readEventsAfter(root, "01ARZ3NDEKTSV4RRFFQ69G5FAW")
	if err != nil {
		t.Fatalf("readEventsAfter: %v", err)
	}
	if len(evs) != 1 || evs[0].id != "01ARZ3NDEKTSV4RRFFQ69G5FAX" {
		t.Errorf("readEventsAfter(id-0002) = %+v, want only id-0003", evs)
	}
}

// TestReadEventsAfterLastCursorIsEmpty: a cursor naming the last event leaves
// nothing after it — the "nothing new yet" state a poller starts in.
func TestReadEventsAfterLastCursorIsEmpty(t *testing.T) {
	at := time.Date(2026, 8, 10, 12, 0, 0, 0, time.UTC)
	root := writeEvents(t, eventLine("01ARZ3NDEKTSV4RRFFQ69G5FAV", at, eventCommentPosted, "doc.md", "^a", "toly", 0))
	evs, err := readEventsAfter(root, "01ARZ3NDEKTSV4RRFFQ69G5FAV")
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
	root := writeEvents(t, eventLine("01ARZ3NDEKTSV4RRFFQ69G5FAV", at, eventCommentPosted, "doc.md", "^a", "toly", 0))
	_, err := readEventsAfter(root, "01ARZ3NDEKTSV4RRFFQ69G5FAZ")
	if err == nil {
		t.Fatal("readEventsAfter with an unknown cursor reported success")
	}
	if !strings.Contains(err.Error(), "01ARZ3NDEKTSV4RRFFQ69G5FAZ") {
		t.Errorf("error does not name the bad cursor: %v", err)
	}
}

// TestReadEventsAfterResolvesTiesByFilePosition: two events sharing a
// millisecond are ordered by where they sit in the file, not by comparing
// ids — a cursor into an append-only log means the tie is file order (D13).
// Here the second line's id sorts *before* the first's lexically, so a string
// comparison would hand back the wrong event.
func TestReadEventsAfterResolvesTiesByFilePosition(t *testing.T) {
	at := time.Date(2026, 8, 10, 12, 0, 0, 0, time.UTC)
	root := writeEvents(t,
		eventLine("ZZZZZZZZZZZZZZZZZZZZZZZZZW", at, eventCommentPosted, "doc.md", "^a", "toly", 0),
		eventLine("00000000000000000000000000", at, eventCommentPosted, "doc.md", "^a", "agent", 1),
	)
	evs, err := readEventsAfter(root, "ZZZZZZZZZZZZZZZZZZZZZZZZZW")
	if err != nil {
		t.Fatalf("readEventsAfter: %v", err)
	}
	if len(evs) != 1 || evs[0].id != "00000000000000000000000000" {
		t.Errorf("readEventsAfter(ZZZZ…) = %+v, want the later line (file order, not id order)", evs)
	}
}

// TestReadEventsAfterSurfacesMalformedLine: the cursor filter runs on the
// same parsed log as everything else, so a malformed completed line is still
// an error, not silently skipped.
func TestReadEventsAfterSurfacesMalformedLine(t *testing.T) {
	at := time.Date(2026, 8, 10, 12, 0, 0, 0, time.UTC)
	root := writeEvents(t,
		eventLine("01ARZ3NDEKTSV4RRFFQ69G5FAV", at, eventCommentPosted, "doc.md", "^a", "toly", 0),
		"this is not an event line",
	)
	if _, err := readEventsAfter(root, "01ARZ3NDEKTSV4RRFFQ69G5FAV"); err == nil {
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
	if err := appendEvent(root, event{kind: eventCommentPosted, doc: "doc.md", anchor: "^a", author: "toly", comment: 0}); err != nil {
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
		_ = appendEvent(root, event{kind: eventThreadResolved, doc: "doc.md", anchor: "^a", author: "toly", comment: -1})
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
// two events in one millisecond come out in file order even when their ids
// sort the other way — the D13 tie rule, observed through the wait itself.
func TestWaitEventsCursorIsFilePosition(t *testing.T) {
	root, docPath := waitRoot(t)
	at := time.Date(2026, 8, 10, 12, 0, 0, 0, time.UTC)
	if err := os.MkdirAll(filepath.Dir(eventsLogPath(root)), 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	data := eventLine("ZZZZZZZZZZZZZZZZZZZZZZZZZW", at, eventCommentPosted, "doc.md", "^a", "toly", 0) + "\n" +
		eventLine("00000000000000000000000000", at, eventCommentPosted, "doc.md", "^a", "agent", 1) + "\n"
	if err := os.WriteFile(eventsLogPath(root), []byte(data), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	lines, err := WaitEvents(docPath, "ZZZZZZZZZZZZZZZZZZZZZZZZZW", time.Second)
	if err != nil {
		t.Fatalf("WaitEvents: %v", err)
	}
	if len(lines) != 1 || !strings.Contains(lines[0], "00000000000000000000000000") {
		t.Errorf("WaitEvents(ZZZZ…) = %v, want the later line (file order)", lines)
	}
}
