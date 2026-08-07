# Feature request: a command palette on `:`

Priority: normal
Kind: feature

`:` should open a command palette — one place to type what you want to do, so
nothing in margin is reachable *only* by remembering a key.

## Why

The key surface is growing: `c`, `e`, `r`, `f`, `space`, `Y`, `g`, `G`, `q`, and
the backlog adds resolve, delete, paging and the wheel. Every one of those is a
thing to remember, and a new user has no way to discover them short of reading
`--help`. A palette makes the whole verb set browsable and searchable, and it
scales as more verbs land.

## Requirements

**1. `:` opens it. Type to narrow, Enter runs the highlighted command.**
Filtering happens as you type and results are ranked, not merely substring-
filtered — typing a few characters of what you half-remember should surface the
right verb.

**2. The palette is a way *in*, never a second implementation.**
This is the one requirement I would not compromise on. Selecting a command must
run exactly what its key runs — same code path, same guards, same confirmations.
If `d` on a comment asks before deleting, then `:delete` `Enter` asks too. The
moment the palette has its own behaviour, the two surfaces drift and every future
verb has to be built twice.

**3. Every command has a stable id and a description, and both are searchable.**
Rows read as two columns: the id, then what it does — `mark.reviewed  Mark
reviewed`. The id is the same string that appears in the docs and that a config
file would rebind, so `mark.rev` finds it just as typing `reviewed` does. One
name for a command everywhere.

**4. It offers what applies to what is focused, and names the target.**
With a comment focused, the palette should list the things you can do *to that
comment*. With a paragraph focused, the things you can do to a block. Commands
that do not apply should not be listed at all rather than listed and failing.
The palette's own title should name what it is about to act on, so you can see
the target before pressing Enter — the difference between "Delete" and "Delete —
comment by agent" is the difference between confidence and a guess.

**5. Keys and the palette are one surface, not two.**
A verb's key should be able to open the palette *already partway through* rather
than somewhere else entirely. If a command takes a value — a mark to apply, a
section to jump to — its key opens the palette with that command already entered
and the value list showing. The key saves the typing without taking you to a
different box.

**6. Staged commands rewind predictably.**
For a command that reached a value list: `Backspace` from an empty value rewinds
to the full command list, so a key pressed by mistake leaves you one keystroke
from everything else. `Esc` **closes** the palette and returns you exactly where
you were — it does not rewind a stage. Getting these two the wrong way round is
an easy mistake and an annoying one.

**7. Nothing it does is irreversible without the usual confirmation.**
Falls out of requirement 2, but worth stating: the palette must not become a
shortcut around a guard.

## Not settled — needs judging on a screen

- Where it appears and how much room it takes. Overlay, bottom strip, centred box?
- Whether it dims or covers the document behind it.
- Whether recently used commands float up, or the order stays stable and
  predictable. Stable ordering is easier to build muscle memory against; recency
  is faster once you have habits. I lean stable, but it should be looked at.

## Notes

**There is a prerequisite, and it is worth doing on its own.** `handleKey`
currently matches raw key strings in a `switch`. A palette needs a real registry:
every command with an id, a description, an applicability rule, and one function
that runs it — with key handling resolving a key to a command and then invoking
the same entry point the palette does. Without that, requirement 2 is unachievable
and the palette becomes a parallel implementation by construction.

That registry is worth having regardless of the palette: it is also what makes
keybindings documentable without hand-maintaining a table, and rebindable later.
Suggested split:

- a mechanical leg introducing the registry and routing existing keys through it,
  with no behaviour change and the current keys proven identical by tests
- a mechanical leg for the palette itself: open on `:`, list, filter, run
- a felt leg for how it looks and where it sits
- a mechanical leg for focus-sensitive commands and the titled target
- a later leg for staged value commands and key-opens-a-stage

The first of those has value even if the palette slips.
