# Q-0004 — How is a document's previous reviewed revision recorded and located?

Status: open
Blocks: M4 — "Diff a document against the previous reviewed revision", and the
two M4 items that build on it (rendering the diff as prose; carrying marks and
threads across a revision)
Raised: 2026-08-11

## What I need decided

For "what changed since you last reviewed this", what is the "previous reviewed
revision" — when does margin capture it, and where does the capture live on
disk?

## Why I cannot decide it

M4's first item is to diff a document against the revision the reviewer last
reviewed. That revision is not the current file — the current file is, by
definition, the new one — so margin must keep a copy of the document as it was
when it was last reviewed. Where that copy lives is a new on-disk shape under
`.margin/` (the expensive-to-unwind class: an agent and every later leg keep
reading it, and changing it later is a migration). When it is taken is a
semantic decision — it defines what "reviewed" means, and every round of M4
diffs against the baseline that rule produces. Get either wrong and the
differentiator reads against the wrong version, and un-doing it means touching
files the loop is already using. The board's own M4 note names this: "the first
one (how a previous revision is recorded and located) touches on-disk format,
so expect a question or a Dn before building."

## Options

**A. Snapshot on load, diff on adopt.** Every time margin loads a document —
at open, at each `ctrl+r` reload, at each switch in a tree review — it writes a
copy of the loaded bytes to `.margin/revisions/<docPath>.md`, overwriting the
previous snapshot. When a new version is loaded, margin diffs it against the
previous snapshot, shows the reviewer what moved, and the load itself adopts
the new version as the new baseline. The "previous reviewed revision" is simply
"the version you last had on screen" — honest, because it is exactly what the
reviewer last saw, regardless of whether they acted. Cost: a session that opens
and quits without reviewing still rolls the baseline forward — but the loaded
version is what the reviewer saw, so the baseline is never *wrong*, only
prematurely fresh.

**B. Snapshot at session end (quit).** The document is copied to
`.margin/revisions/<docPath>.md` when the review session ends — quit, or
leaving a document in a tree review. "Previous reviewed revision" = the version
loaded when the last session ended. Cost: a crash loses the snapshot (the
worst-case is no diff next time, not data loss — the document itself is
untouched); a quit right after a `ctrl+r` adopts a version the reviewer may not
have read yet.

**C. Explicit checkpoint.** A command or key ("mark this version reviewed")
snapshots the current document as the baseline. Reviewer-controlled: the
baseline only advances when the reviewer says it does. Cost: a new key/command
to remember and press, and the felt surface — which key, where it shows — is a
whole extra felt leg; the implicit options need none of it.

**D. Derive from thread anchors, no snapshot.** Compare each current block's
content-derived anchor (D12, M1's lazy-stamping gap) against the content quoted
in its thread files, and call differences "changed". Cost: only tells you about
blocks that have threads; M4's point is seeing what moved *everywhere*, reviewed
or not, and a paragraph nobody commented on but the agent rewrote would vanish.
Rejected.

## My lean

**A**, one snapshot per document at `.margin/revisions/<docPath>.md`,
overwritten on every load, with the diff computed when a new version is
adopted. It is the only option where the baseline is exactly "what the reviewer
last saw" with no new key and no crash window (the snapshot is written when a
version is *adopted*, not when a session happens to end). Rounds arrive via
reload in practice — the file watcher + `ctrl+r` (2026-08-10.15) is how an
agent's rewrite becomes visible — so "diff on adopt" gives the reviewer the
"here's what moved" moment at every round boundary without any interaction.
B's wording is close but opens the crash-loss and unread-adoption gaps for no
gain; C adds a whole felt surface for what A does implicitly. A single
overwritten snapshot per document keeps the format flat; keeping a *history* of
revisions is a later addition that does not migrate the single-file shape.
The one thing I genuinely cannot pick without you: whether a *bare open* should
advance the baseline (a plain `margin doc.md`, reviewed or not, becomes the new
"previous") or only an explicit reload/adopt does. A makes bare-open advance;
it is the defensible default, but it is the semantic that decides what
"reviewed" means, so it is yours to settle.
