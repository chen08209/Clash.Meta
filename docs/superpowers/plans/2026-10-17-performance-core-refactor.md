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

- [x] Add a valid TCP metadata fixture with source, destination, host, inbound, and network fields populated.
- [x] Benchmark `resolveMetadata` in rule mode with a small deterministic rule list.
- [x] Benchmark tracker `Join` and `Leave` in a loop with request notification disabled.
- [x] Benchmark tracker `Read` and `Write` through `net.Pipe`, setting bytes and reporting allocations.
- [x] Run `go test -run '^$' -bench '^BenchmarkResolveMetadata|BenchmarkTracker' -benchmem ./tunnel ./tunnel/statistic`.
- [x] Save the output as the baseline for later comparison.

### Task 2: Lazy Logging

**Files:**
- Modify: `log/log.go`
- Test: `log/log_test.go`

**Interfaces:**
- Produces: an early return in `Infoln`, `Warnln`, `Errorln`, and `Debugln` when the event level is below `level`.

- [x] Write a test that sets level to `INFO`, calls `Debugln`, and asserts that no event reaches a subscriber.
- [x] Write the same gate for `INFO` events when level is `WARNING`.
- [x] Add the level check before `newLog` and the observable channel send.
- [x] Run `go test ./log`.
- [x] Compare Task 1 allocations; hidden debug formatting must disappear when debug logging is off.

### Task 3: Binary Tracker Keys

**Files:**
- Modify: `tunnel/statistic/manager.go`
- Test: `tunnel/statistic/manager_test.go`

**Interfaces:**
- Produces: `Manager.connections` keyed by `uuid.UUID`.
- Preserves: `Manager.Get(id string) Tracker` and `Tracker.ID() string`.

- [x] Write tests for store, lookup by formatted ID, range, and delete.
- [x] Change the map key from the formatted UUID string to `uuid.UUID`.
- [x] Parse the public string ID only in `Get`.
- [x] Keep `Tracker.ID()` unchanged for API compatibility.
- [x] Run the statistics benchmark and require no allocation for key formatting during `Join`.

### Task 4: Runtime Snapshot

**Files:**
- Create: `tunnel/runtime.go`
- Modify: `tunnel/tunnel.go`
- Test: `tunnel/runtime_test.go`

**Interfaces:**
- Produces: `type runtimeSnapshot struct` and `func loadRuntime() *runtimeSnapshot`.
- Consumes: existing exported update functions as publication boundaries.

- [x] Move rule, sub-rule, proxy, provider, listener, sniffer, and mode state into an immutable snapshot.
- [x] Publish snapshots through an atomic pointer after all fields are built.
- [x] Keep compatibility accessors reading from the current snapshot.
- [x] Add a concurrent stress test that updates proxies and rules while resolving connections.
- [x] Run `go test -race ./tunnel`.

### Task 5: TCP Match Context

**Files:**
- Modify: `tunnel/tunnel.go`
- Test: `tunnel/tunnel_test.go`

**Interfaces:**
- Produces: a reusable match-context type used by `resolveMetadata`.
- Preserves: `C.RuleMatchHelper` and every rule implementation.

- [x] Write a test proving process lookup and DNS resolution run at most once per connection.
- [x] Replace per-connection closures with a context whose method values are assigned to the existing helper struct.
- [x] Keep special, direct, global, and rule-mode behaviour identical.
- [x] Run `go test -race ./tunnel` and compare the metadata benchmark.

### Task 6: UDP Binary Flow Table

**Files:**
- Modify: `tunnel/tunnel.go`
- Modify: `tunnel/connection.go`
- Test: `tunnel/connection_test.go`

**Interfaces:**
- Produces: `FlowKey() netip.AddrPort` for the packet source address and port.
- Produces: a NAT map keyed by the source `netip.AddrPort`; destination NAT stays with the packet sender.
- Preserves: packet sender and write-back public interfaces.

- [x] Add a key test covering IPv4, IPv6, and legacy formatted-string adapters.
- [x] Replace string NAT keys with binary flow keys.
- [x] Keep worker selection stable per source flow.
- [x] Add an overload check that verifies queue drops never block the producer.
- [x] Run `go test -race ./tunnel`.

### Task 7: Group Snapshot Selection

**Files:**
- Modify: `adapter/outboundgroup/groupbase.go`
- Modify: `adapter/outboundgroup/urltest.go`, `fallback.go`, `loadbalance.go`, and `selector.go`
- Test: existing and new tests in `adapter/outboundgroup`

**Interfaces:**
- Produces: an immutable provider-version/member-list cache.
- Preserves all public proxy-group interfaces and FlClash failover hooks.

- [x] Add a concurrent selection benchmark plus provider version-change tests.
- [x] Publish a copied member slice only when provider versions change.
- [x] Preserve provider semantics for an empty result without caching the fallback as a generation.
- [x] Keep the current failover and recovery semantics.
- [x] Run `go test -race ./adapter/outboundgroup`.

### Task 8: End-to-End Performance Gate

**Files:**
- Modify: `test/clash_test.go`
- Create: `docs/superpowers/benchmarks/2026-10-17-baseline.txt`

**Interfaces:**
- Produces: repeatable direct and chained relay benchmarks.

- [x] Run the Direct relay benchmark against the clean baseline worktree.
- [ ] Run Shadowsocks, VMess, and Trojan relay benchmarks when Docker is available.
- [ ] Run UDP packet benchmarks for small and 1500-byte payloads.
- [ ] Compare every phase against Task 1 with `benchstat`.
- [x] Reject changes with a Direct target win but a Direct end-to-end regression.
- [x] Record the final benchmark commands and environment.

### Task 9: Large Rule-Set Index

**Files:**
- Create: `tunnel/rule_index.go`
- Modify: `tunnel/runtime.go`, `tunnel/tunnel.go`
- Test: `tunnel/rule_index_test.go`, `tunnel/rule_index_bench_test.go`

**Interfaces:**
- Produces: contiguous `DOMAIN` and `DOMAIN-SUFFIX` rule-run indexes with rule-position ordering.
- Preserves: first-match order, disabled wrappers, duplicate payloads, PASS, REMATCH, sub-rules, and non-domain rule order.

- [x] Add a red test proving a 10,000-rule exact-domain run must not be scanned linearly.
- [x] Index exact names with a position-aware hash.
- [x] Index suffixes with a reversed-label, position-aware trie.
- [x] Cover disabled wrappers, duplicate payloads, PASS, sub-rules, overlapping exact/suffix rules, and intervening non-domain rules.
- [x] Compare 6 benchmark samples against commit `bf2846b6` with `benchstat`.
- [x] Run full main-module tests and `go test -race ./tunnel`.

### Task 10: UDP Origin Map Key

**Files:**
- Modify: `tunnel/connection.go`
- Test: `tunnel/connection_perf_test.go`

**Interfaces:**
- Produces: `packetOriginKey(*C.Metadata) packetOrigin`.
- Preserves: host-first key semantics and the existing target/origin NAT mappings.

- [x] Add a red test proving the key must map without formatting metadata.
- [x] Replace the string map key with a comparable host-or-IP key.
- [x] Benchmark 6 samples against commit `92889146` with `benchstat`.
- [x] Run focused tests and `go test -race ./tunnel`.

### Task 11: UDP Mapping Snapshot

**Files:**
- Modify: `tunnel/connection.go`
- Test: `tunnel/connection_perf_test.go`

**Interfaces:**
- Produces: `packetMappings`, an immutable origin-to-target and target-to-origin snapshot.
- Preserves: first-mapping semantics and concurrent read/write safety.

- [x] Replace the per-packet `RWMutex` read with an atomic snapshot load.
- [x] Keep writes serialized and publish a new immutable snapshot.
- [x] Cover first-mapping semantics and concurrent access.
- [x] Benchmark 6 samples against commit `ec2dced6` with `benchstat`.
- [x] Run focused tests, `go test -race ./tunnel`, and the full main-module suite.

### Task 12: Sharded Traffic Counters

**Files:**
- Modify: `tunnel/statistic/manager.go`
- Modify: `tunnel/statistic/tracker.go`
- Test: `tunnel/statistic/manager_test.go`
- Benchmark: `tunnel/statistic/manager_bench_test.go`

**Interfaces:**
- Produces: `counterStripe`, a cache-line-isolated group of traffic counters, plus `Manager.bindStripe`.
- Preserves: `Manager.PushUploaded/PushDownloaded`, `Total`, `Now`, `TotalTraffic`, `NowTraffic`, `ResetStatistic`, and `Snapshot` semantics.

- [x] Write a test proving parallel pushes sum exactly across stripes.
- [x] Write a test proving the one-second swap drains every stripe without losing adds.
- [x] Bind one stripe per tracker at construction and push through it.
- [x] Aggregate totals by summing stripes; keep blips on the ticker path only.
- [x] Benchmark shared-counter versus striped pushes under `RunParallel`.
- [x] Run `go test ./tunnel/statistic`, `go test -race ./tunnel`, and `go vet`.
