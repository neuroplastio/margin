# Composer exit, comment editing, and empty-thread feedback — 2026-08-09

From the maintainer, while reviewing the dive/comment flow (journal 2026-08-09.28):

## Exit to normal mode should be `esc`, not vim's `:`-based exit
- Exiting the embedded vim should be done by `esc` while in normal mode.
- Creating a new comment: **double `esc`** exits the composer.
- Editing an existing comment: **single `esc`** exits (no comment body to finish).

## Inserting after appending a line reference
- When appending line numbers to the comment, start inserting immediately after
  the appended reference (no intermediate command-line step).

## Editing a comment should place the cursor at the end
- When entering edit mode on an existing comment, the cursor should be at the
  end of the comment text.

## Cancel-early must not leave an empty thread
- Starting a new comment and immediately cancelling currently creates an empty
  thread showing "no comments yet". It should not. Cancelling before anything is
  committed must not create a thread at all.

## Save-less exit must not mark an unchanged comment as a draft
- Editing a comment and exiting without changing anything and without saving
  marks it "unsaved draft" — but nothing changed. If there is no diff, do not
  mark it as a draft.

## Dive/undive should also bind to `enter`/`esc`
- `enter` = dive, `esc` = undive, in addition to the existing `l`/`h` bindings.
