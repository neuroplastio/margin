# Event log: JSONL format and more compact ids

Feedback, 2026-08-10. From a test run of the event log (journal 2026-08-10.7,
`.margin/events.log`, D13). Two changes to the on-disk shape:

1. **Use JSONL format.** Each event line should be a JSON object instead of the
   current seven tab-separated fields. JSONL makes the fields self-describing
   and sidesteps the free-form-field sanitising entirely.
2. **Make the ids more compact.** The 26-character ULID is heavy for an id that
   mostly just needs to be *an id* — time-ordered, unique enough, short enough
   to copy, paste and compare. Something shorter is wanted.
3. **Use a unix timestamp for the time field, at second precision.** No
   RFC3339Nano string.

This is the on-disk format the `--since` cursor and any listener already depend
on, so the new shapes must be recorded in decisions.md (supersede/extend D13)
before or with the change. The log is new enough that there is no real history
to migrate — treat this as a correction to a format that has not shipped to any
agent yet. The wait command's contract, the same-millisecond tie rule and the
torn-tail handling should survive the change unchanged.
