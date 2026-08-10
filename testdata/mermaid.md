---
title: Release train architecture
status: draft
owner: platform
reviewers: [toly]
tags: [ci, deploy, architecture]
description: The release-train pipeline in one document: how a push to main becomes a production deploy, the states the pipeline passes through, the services and queues it touches, and where the reviewer's eye should land when a train is blocked.
---

# Release train architecture

This document is a single map of the release-train pipeline: the deploy
workflow, the services it drives, the states a release passes through, and the
data model the release store is built on. The diagrams are mermaid, so they
render as ASCII in the review.

## Deploy workflow

The deploy pipeline starts when a push lands on `main`. Each stage gates the
next, and a failure anywhere stops the train rather than half-deploying:

```mermaid
flowchart TD
    A[Push to main] --> B[Run tests]
    B -->|green| C[Build images]
    B -->|red| Z[Notify slack]
    C --> D[Run migrations]
    C --> E[Promote canary]
    D --> F[Verify in staging]
    E --> F
    F -->|pass| G[Roll out prod]
    F -->|fail| H[Rollback]
    G --> I[Done]
    H --> Z
    Z --> J[Stop]
```

The `flowchart TD` direction is honoured by the renderer, and the `|green|`
and `|red|` edge labels name the branch each arrow takes. A train that fails
lands on `Notify slack` and `Stop`, never on a half-deployed state.

## The release lifecycle

A release passes through the states below. The start and end markers are the
`[*]` pseudo-states; the transitions carry the signal that moves the release
onward:

```mermaid
stateDiagram-v2
    [*] --> Queued
    Queued --> Building: ready
    Building --> Staging: image built
    Staging --> Canary: verified
    Canary --> Live: promoted
    Canary --> RollingBack: error rate up
    RollingBack --> Staging: reverted
    Live --> [*]
```

## What talks to what

The pipeline talks to four services. The solid arrows are synchronous calls,
the dotted ones asynchronous queue messages:

```mermaid
sequenceDiagram
    participant CI as CI runner
    participant API as Release API
    participant DB as Release store
    participant WQ as Work queue
    CI->>API: create release
    API->>DB: INSERT release
    API->>WQ: enqueue build
    WQ-->>CI: build result
    CI->>API: mark built
    API->>DB: UPDATE status
    CI-->>API: notify
```

`activate`/`deactivate` are tolerated but not drawn, so a diagram that uses
them still renders. Notes over a span of participants render on the lifeline:

```mermaid
sequenceDiagram
    participant A as API
    participant W as Worker
    Note over A,W: poll loop, 2s interval
    A->>W: claim job
    W-->>A: done
```

## The data model

The release store is a relational schema. The cardinality markers follow the
crow's-foot notation mermaid uses:

```mermaid
erDiagram
    RELEASE ||--o{ DEPLOY : triggers
    RELEASE {
        int id PK
        string sha
        string status
        timestamp created_at
    }
    DEPLOY ||--|{ STEP : runs
    DEPLOY {
        int id PK
        int release_id FK
        string environment
    }
    STEP {
        int id PK
        int deploy_id FK
        string name
        bool passed
    }
```

Attributes in the entity bodies render as a labelled block; relationships draw
between the entities. A relationship's label ("triggers", "runs") sits on its
connector.

## Unsupported kinds

Kinds the renderer does not understand fall back to their plain source lines,
never chroma and never a half-parsed diagram:

```mermaid
classDiagram
    class Deploy {
        +string sha
        +run()
    }
    Deploy <|-- CanaryDeploy
```

---

## Review guide

Open the document and walk the blocks with `j`/`k`: each diagram is one focus
stop. `H`/`L` pan a diagram wider than the terminal; `\` switches to the raw
source to confirm the mermaid behind a given box.
