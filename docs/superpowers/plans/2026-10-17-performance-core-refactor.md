# Performance Core Refactor Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make forwarding hot paths allocation-light, lock-free for configuration reads, and benchmark-gated.

**Architecture:** Preserve Clash.Meta's protocol and API surface while introducing immutable runtime snapshots, lazy observability, binary UDP flow keys, and benchmark harnesses. Each phase remains independently testable and reversible.

**Tech Stack:** Go 1.20+, the existing sing networking stack, Go benchmarks, `benchstat`, and existing Clash.Meta tests.

**Spec:** `docs/superpowers/specs/2026-10-17-performance-core-refactor-design.md`

## Global Constraints

- Do not break existing YAML, JSON, Go APIs, or FlClash integration.
- Do not push or trigger GitHub Actions until explicitly requested.
- No optimization is accepted without a focused benchmark or allocation check.
- Preserve per-flow UDP ordering.
- Keep queues bounded; overload must drop and count rather than block platform readers.

---

### Task 1: Benchmark Baseline

**Files:**
- Create: `tunnel/tunnel_bench_test.go`
- Create: `tunnel/statistic/manager_bench_test.go`
- Read: `test/clash_test.go`

**Interfaces:**
- Produces: `BenchmarkResolveMetadata`, `BenchmarkTrackerJoinLeave`, `BenchmarkTrackerReadWrite`, and reusable in-package benchmark fixtures.

- [ ] Add a valid TCP metadata fixture with source, destination, host, inbound, and network fields populated.
- [ ] Benchmark `resolveMetadata` in rule mode with a small deterministic rule list.
- [ ] Benchmark tracker `Join` and `Leave` in a loop with request notification disabled.
- [ ] Benchmark tracker `Read` and `Write` through `net.Pipe`, setting bytes and reporting allocations.
- [ ] Run `go test -run '^$' -bench '^BenchmarkResolveMetadata|BenchmarkTracker' -benchmem ./tunnel ./tunnel/statistic`.
- [ ] Save the output as the baseline for later comparison.

### Task 2: Lazy Logging

**Files:**
- Modify: `log/log.go`
- Test: `log/log_test.go`

**Interfaces:**
- Produces: an early return in `Infoln`, `Warnln`, `Errorln`, and `Debugln` when the event level is below `level`.

- [ ] Write a test that sets level to `INFO`, calls `Debugln`, and asserts that no event reaches a subscriber.
- [ ] Write the same gate for `INFO` events when level is `WARNING`.
- [ ] Add the level check before `newLog` and the observable channel send.
- [ ] Run `go test ./log`.
- [ ] Compare Task 1 allocations; hidden debug formatting must disappear when debug logging is off.

### Task 3: Binary Tracker Keys

**Files:**
- Modify: `tunnel/statistic/manager.go`
- Test: `tunnel/statistic/manager_test.go`

**Interfaces:**
- Produces: `Manager.connections` keyed by `uuid.UUID`.
- Preserves: `Manager.Get(id string) Tracker` and `Tracker.ID() string`.

- [ ] Write tests for store, lookup by formatted ID, range, and delete.
- [ ] Change the map key from the formatted UUID string to `uuid.UUID`.
- [ ] Parse the public string ID only in `Get`.
- [ ] Keep `Tracker.ID()` unchanged for API compatibility.
- [ ] Run the statistics benchmark and require no allocation for key formatting during `Join`.

### Task 4: Runtime Snapshot

**Files:**
- Create: `tunnel/runtime.go`
- Modify: `tunnel/tunnel.go`
- Test: `tunnel/runtime_test.go`

**Interfaces:**
- Produces: `type runtimeSnapshot struct` and `func loadRuntime() *runtimeSnapshot`.
- Consumes: existing exported update functions as publication boundaries.

- [ ] Move rule, sub-rule, proxy, provider, listener, sniffer, and mode state into an immutable snapshot.
- [ ] Publish snapshots through an atomic pointer after all fields are built.
- [ ] Keep compatibility accessors reading from the current snapshot.
- [ ] Add a concurrent stress test that updates proxies and rules while resolving connections.
- [ ] Run `go test -race ./tunnel`.

### Task 5: TCP Match Context

**Files:**
- Modify: `tunnel/tunnel.go`
- Test: `tunnel/tunnel_test.go`

**Interfaces:**
- Produces: a reusable match-context type used by `resolveMetadata`.
- Preserves: `C.RuleMatchHelper` and every rule implementation.

- [ ] Write a test proving process lookup and DNS resolution run at most once per connection.
- [ ] Replace per-connection closures with a context whose method values are assigned to the existing helper struct.
- [ ] Keep special, direct, global, and rule-mode behaviour identical.
- [ ] Run `go test -race ./tunnel` and compare the metadata benchmark.

### Task 6: UDP Binary Flow Table

**Files:**
- Modify: `tunnel/tunnel.go`
- Modify: `tunnel/connection.go`
- Test: `tunnel/connection_test.go`

**Interfaces:**
- Produces: `type flowKey struct` using network, source address/port, and destination address/port.
- Produces: a sharded NAT map keyed by `flowKey`.
- Preserves: packet sender and write-back public interfaces.

- [ ] Add a collision test covering IPv4, IPv6, TCP, and UDP keys.
- [ ] Replace string NAT keys with binary flow keys.
- [ ] Keep worker selection stable per source flow.
- [ ] Add an overload benchmark that verifies queue drops never block the producer.
- [ ] Run `go test -race ./tunnel`.

### Task 7: Group Snapshot Selection

**Files:**
- Modify: `adapter/outboundgroup/groupbase.go`
- Modify: `adapter/outboundgroup/urltest.go`, `fallback.go`, `loadbalance.go`, and `selector.go`
- Test: existing and new tests in `adapter/outboundgroup`

**Interfaces:**
- Produces: an immutable provider-version/member-list cache.
- Preserves all public proxy-group interfaces and FlClash failover hooks.

- [ ] Add a concurrent selection benchmark with provider version changes.
- [ ] Publish a copied member slice only when provider versions change.
- [ ] Ensure a provider failure keeps the last usable snapshot.
- [ ] Keep the current failover and recovery semantics.
- [ ] Run `go test -race ./adapter/outboundgroup`.

### Task 8: End-to-End Performance Gate

**Files:**
- Modify: `test/clash_test.go`
- Create: `docs/superpowers/benchmarks/2026-10-17-baseline.txt`

**Interfaces:**
- Produces: repeatable direct and chained relay benchmarks.

- [ ] Run direct, Shadowsocks, VMess, and Trojan relay benchmarks.
- [ ] Run UDP packet benchmarks for small and 1500-byte payloads.
- [ ] Compare every phase against Task 1 with `benchstat`.
- [ ] Reject changes with a target win but an end-to-end regression.
- [ ] Record the final benchmark commands and environment.
