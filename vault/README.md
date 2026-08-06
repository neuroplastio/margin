# Vault

Everything an agent needs to move margin forward, and everything the maintainer
needs to steer it. Start here, then read [`/CLAUDE.md`](../CLAUDE.md).

## Map

| Path | What it is |
| --- | --- |
| [`goals.md`](goals.md) | What margin is, what it is not, principles |
| [`roadmap.md`](roadmap.md) | Milestones in order, with exit criteria |
| [`tasks/board.md`](tasks/board.md) | The live board — where legs come from |
| [`feedback/`](feedback/) | **Human → agent.** Steer the work. Preempts the board. |
| [`questions/`](questions/) | **Agent → human.** Decisions the agent must not make. Blocks work. |
| [`journal/`](journal/) | One entry per leg. The changelog, and where demo recipes live. |
| [`knowledge/interaction.md`](knowledge/interaction.md) | The settled interaction contract. Binding. |
| [`knowledge/decisions.md`](knowledge/decisions.md) | Binding `Dn` architecture decisions. |
| [`knowledge/findings.md`](knowledge/findings.md) | Hard-won technical facts. Read before touching the composer. |

## The two loops

**The agent's loop** is one leg at a time: orient, pick, claim, implement, verify,
record, commit, stop. `/do-leg` runs exactly one. The daily scheduled `/do-run`
chains *mechanical* legs back to back — tests settle those, so they need not wait
on anyone — and stops the moment it produces something only you can judge.

**The maintainer's loop** is: run the thing, judge it, drop a file in
[`feedback/`](feedback/) or answer something in [`questions/`](questions/).

The second loop is the slow one and the whole design assumes it. Work is paced so
that the maintainer is never more than one felt change behind — see *Felt legs and
mechanical legs* in [`/CLAUDE.md`](../CLAUDE.md).

## For the maintainer, in one minute

You have three levers:

1. **Steer** — drop a markdown file in [`feedback/`](feedback/). Free-form; it just
   has to say what you want. The next leg drains it before touching the board.
2. **Unblock** — answer an open file in [`questions/`](questions/) by editing it and
   setting `Status: answered`. Work named in its `Blocks:` resumes.
3. **Release the gate** — when the board's `Awaiting review:` names a leg you have
   judged, say so in feedback (or clear the line yourself). Until then the agent
   will not start another felt change.

If you do nothing, the agent will keep doing mechanical work until it runs out,
then stop and say what it is waiting on.
