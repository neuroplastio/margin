# Mermaid layout failure

Mermaid diagram layout failure on this example — a `flowchart TB` with three
subgraphs, mixed node shapes (database `[("…")]`, `{"…"}` decision, `["…"]`),
multi-line `<br/>` labels, and dotted edges with labels.

Roughly 40 nodes across three subgraphs; the rendering breaks apart / overlaps,
so it is unreadable.

Anonymized reproduction (real service names, endpoints and domain terms replaced
with neutral placeholders; structure, shapes, subgraph boundaries and label
lengths kept identical so the layout failure reproduces):

```mermaid
flowchart TB
  subgraph a ["Node A · every 60s"]
    set[("item set<br/>added at item creation<br/>removed at item:end / item:failed")]
    set --> post["POST /api/v1/items/sync<br/>{id, ids}"]
    kill["item.finish"] --> bye["END both paths → item:end"]
    bye --> fin["/api/v1/items/end → row written, row deleted"]
  end

  subgraph b ["Node B"]
    post --> renew["live = now + 120s<br/>for every named id"]
    renew --> ans{"done?"}
    ans -->|no| reply["reply"]
    ans -->|yes| ext["Extend"]
    ext --> v{"verdict"}
    v -->|"accepted · session gone · error"| reply
    v -->|"insufficient balance"| cut["add to drop list"]
    cut --> reply
  end

  reply -.->|"drop list"| kill
  renew -.-> readers["every reader asks live > now()<br/>limit · list · detail"]

  subgraph c ["Node C · every 60s · every instance"]
    r0(["pass"]) --> r1{"a sync arrived<br/>within 120s?"}
    r1 -->|no| r2["skip this window"]
    r1 -->|yes| r3["claim live <= now<br/>stamping state"]
    r3 --> r4{"record<br/>already exists?"}
    r4 -->|yes| r5["delete row · recorded"]
    r4 -->|no| r6["write row with time"]
    r6 --> r5
  end
```

The domain terms above are placeholders — the real diagram used proprietary
service/endpoint names, deliberately not recorded here.
