---
title: Session storage migration
status: draft
owner: platform
reviewers: [toly]
tags: [storage, migration, redis]
description: Moving session state off the application servers and into a shared store so a rolling deploy stops logging everyone out and a single node failure stops taking a slice of active sessions with it.
---

# Session storage migration

Move session state off the application servers and into a shared store, so that
a deploy stops logging everyone out and so a single node failure stops taking a
slice of active sessions with it.

## Why now

Sessions currently live in each application server's memory. That was fine when
there was one server. With six behind a load balancer it means sticky routing,
and sticky routing means a rolling deploy drops every session on whichever node
is being replaced. Support sees a spike of "logged out for no reason" tickets
after every release, and the on-call runbook has a step that amounts to
apologising.

The retry budget is shared across all endpoints, so a single misbehaving
upstream can starve every other caller in the process — which is a separate
problem, but it is the reason the last incident took forty minutes to diagnose
rather than five.

## Options considered

We looked at three, in rough order of how much they would cost us:

1. **Sticky sessions with graceful drain.** Cheapest. Does not fix node
   failure, only deploys.
2. **Database-backed sessions.** No new infrastructure, but session reads are
   on the hot path for every request and the primary is already the bottleneck.
3. **Redis.** New dependency, but purpose-built for this and already used by
   the rate limiter, so the operational surface is not actually new.

Redis it is. The rate limiter has run on it for eighteen months without an
incident, and the failure modes are understood.

### What changes

| Component | Before | After |
| :--- | :---: | ---: |
| Session read | in-process map | Redis `GET`, ~0.4ms p50 |
| Session write | in-process map | Redis `SETEX`, TTL 24h |
| Deploy | drops sessions | no effect |
| Node loss | drops that node's sessions | no effect |

The session token format does not change, so existing cookies keep working and
there is no forced logout at cutover.

## Implementation

The store interface is already abstracted — `SessionStore` has exactly the three
methods we need, and the in-memory implementation becomes the test double
rather than being deleted:

```go
type SessionStore interface {
    Get(ctx context.Context, id string) (*Session, error)
    Put(ctx context.Context, s *Session, ttl time.Duration) error
    Delete(ctx context.Context, id string) error
}

func NewRedisStore(client *redis.Client, prefix string) *RedisStore {
    return &RedisStore{client: client, prefix: prefix}
}
```

Serialisation is `encoding/json` for now. It is not the fastest option, but a
session is under 2KB and the marshalling cost is noise next to the round trip.
If that stops being true, `msgpack` is a drop-in change behind the same
interface.

#### Config surface

The store lives behind a `SESSION_STORE` build tag so the in-memory and Redis
implementations can coexist in one binary during the dual-write window:

```
# session flags consumed by the cmd/sessionserver entrypoint; the empty
# SESSION_REDIS_URL below is deliberate and means "in-memory store, no
# connection", which is exactly the test-double behaviour the runbook wants
# from a cold boot before the shadow-traffic week actually starts talking to redis.
SESSION_REDIS_URL=
SESSION_REDIS_POOL_SIZE=32
```

> **Open question.** Do we need session data encrypted at rest in Redis? The
> tokens are opaque and the payload is a user id plus a permission set, but
> "permission set" is arguably sensitive. Flagging for security review rather
> than deciding here.
>
> Second paragraph of the same quote, to check the paragraph-break handling —
> a blank `>` line separates this from the first without ending the block.

## Rollout

Dual-write first, then dual-read, then cut over. Each step is independently
revertible and none of them requires a maintenance window:

- **Week 1** — write to both stores, read from memory. Redis is shadow traffic;
  if it falls over, nothing user-visible happens.
- **Week 2** — read from Redis, fall back to memory on a miss. Watch the
  fallback rate; it should trend to zero as old sessions expire.
  - What "zero" means: no fallback reads in the last full day, not just a
    quiet hour. A couple of stragglers from a long-lived session are noise.
  - If the rate climbs instead of falling, pause here and re-check the TTL:
    an expiry shorter than the ticket window looks like a bug but is a
    settings error.
- **Week 3** — remove the memory path. Delete the sticky-session config from
  the load balancer, which is the change that actually pays for this work.

Rollback at any point is a config flag, not a deploy. See the [migration
runbook](https://runbooks.internal/redis-migration) for the on-call rollback
steps.

---

## Risks

**Redis becomes a single point of failure.** Mitigated by running it in the
existing cluster with replication, but honestly this is a real trade: we are
swapping "lose one node's sessions on failure" for "lose all sessions if Redis
goes". The cluster has not had an outage in eighteen months. That is not a
guarantee, it is an observation.

**Latency budget.** Adding ~0.4ms to every authenticated request is fine in
isolation. It is not fine if it lands in the same release as something else
that adds latency, so this should ship on its own.

## Not doing

- Session sharing across regions. Out of scope and probably a bad idea.
- Changing the token format or the expiry policy — both are load-bearing
  elsewhere and neither is broken.
