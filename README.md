# consul-state-verify

Read-only verification that a Consul server cluster still provides what it did
before a rolling server upgrade: KV, the service catalog, and the node catalog.
Two commands, sharing one snapshot format:

- **`consul-state-dump`**: takes a logical per-node snapshot to stdout (talks to
  Consul, read-only).
- **`consul-state-diff`**: judges an after snapshot against a before snapshot
  (pure computation; never talks to Consul).

They live in one repository because the snapshot shape is a contract between them.
Building both from one module (the `snapshot` package) keeps the producer and the
judge from drifting apart. There is **no in-band format version to manage**; the
release version of the pair is the version.

## Where it fits

This is check 2 of a two-check upgrade verification: check 1 is
[consul-server-tail](https://github.com/zinrai/consul-server-tail), which confirms
the cluster mechanism is healthy (one leader, voters back, node-ids unique, indexes
converged). Run the comparison **after** check 1 is OK: once the cluster has
converged, a difference between nodes is no longer replication lag but a real
divergence or loss. The goal is narrow: confirm Consul still provides what it did
before (service discovery and KV), judged from Consul's own side, not from
whatever downstream consumes it.

## consul-state-dump

```
consul-state-dump 10.0.0.1:8500 10.0.0.2:8500 10.0.0.3:8500 > before.json
```

Polls each server's HTTP API with `?stale` (every node answers from its own local
state instead of forwarding the read to the leader) and writes one normalized JSON
document. Per node: every KV key as sha256 + length + modify_index (never the
value itself, so credentials stay out of the evidence), every service name with its
healthy instance count, every catalog node as address + id + modify_index, and the
live Consul version. If any fetch from any node fails, nothing is written (no
partial dumps). `--timeout` sets the per-request HTTP timeout (default 5s).

## consul-state-diff

```
consul-state-diff before.json after.json      # full before/after comparison
consul-state-diff after.json                   # inter-node self-check on one dump
```

Pure computation over two files; a verdict can be re-run any number of times
against the same evidence. Two comparisons, of different natures:

- **Inter-node agreement** inside one dump. Once the cluster has converged,
  servers holding the same key or catalog node at the same `modify_index` must
  hold the same value; a split is state-machine divergence (the upgrade-specific
  risk) and **fails**. Partial presence across nodes is ambiguous in a single
  dump, so it is reported **to investigate** (re-dump to confirm).
- **Loss and health** against the baseline. A key, catalog node, or service gone
  everywhere after is reported **to investigate** (it may be a deliberate delete
  or a replaced server). A value that changed at an unchanged `modify_index`
  (corruption), a `modify_index` that moved backwards (regression), or a service
  that dropped to zero healthy instances (outage), **fails**. New keys and services, and advanced-index rewrites or
  re-registrations, are benign.

It fails only on what is wrong regardless of intent (divergence, corruption, an
outage); everything that needs human judgment is reported for the operator to read
and record (and, for server-membership changes, to correlate with
[consul-server-tail](https://github.com/zinrai/consul-server-tail)). Exit codes:
**0** PASS / **1** investigate / **2** FAIL / **64** usage error.

For an upgrade done as N repeated steps,
[consul-server-runbook](https://github.com/zinrai/consul-server-runbook) detects
the leader and emits the concrete per-step `consul-state-dump` / `consul-state-diff`
commands.

## License

This project is licensed under the [MIT License](./LICENSE).
