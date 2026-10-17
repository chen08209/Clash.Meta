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

### Task 13: UDP Admission Fast Path

**Files:**
- Modify: `component/nat/table.go`
- Modify: `tunnel/tunnel.go`
- Test: `tunnel/tunnel_bench_test.go`

**Interfaces:**
- Produces: `Table.LoadForFlow(netip.AddrPort)` (C.PacketSender, bool)`, a read-only flow lookup.
- Preserves: first-packet precheck semantics, flow creation, sniffing, and the drop-on-overload behaviour.

- [x] Add a test proving an existing flow admits a packet with no clone or precheck.
- [x] Move the precheck clone behind the flow-miss branch.
- [x] Benchmark an established-flow packet against commit `bfb4b8c5` with `benchstat`.
- [x] Run `go test ./tunnel -count=1` and `go test -race ./tunnel`.

### Task 15: Allocation-Free UDP Write-Back Update

**Files:**
- Modify: `component/nat/proxy.go`
- Test: `component/nat/proxy_test.go`

**Interfaces:**
- Preserves: `C.WriteBackProxy`, delegation to the most recently stored write-back, and the per-packet update cadence.

- [x] Add a test proving the proxy still delegates to the latest write-back after an update.
- [x] Add a test proving the per-packet update allocates nothing.
- [x] Replace the boxed `TypedValue` store with a direct interface store.
- [x] Benchmark 6 samples against commit `d35c609a` with `benchstat`.
- [x] Run `go test ./component/nat ./tunnel/... -count=1` and `go test -race`.

### Task 16: Rate-Limited UDP Deadline Refresh

**Files:**
- Modify: `tunnel/connection.go`
- Test: `tunnel/connection_perf_test.go`

**Interfaces:**
- Produces: `shouldRefreshDeadline(last int64, now time.Time) bool` and `refreshReadDeadline`.
- Preserves: the UDP idle timeout, packet ordering, and the read loop's deadline-before-read guarantee.

- [x] Refresh the read deadline only once the current one is more than half consumed.
- [x] Track the deadline per direction with no cross-goroutine sharing (read loop local, sender field).
- [x] Test the decision function across unset, fresh, half, and expired deadlines.
- [x] Benchmark per-packet deadline cost against the unconditional shape with `benchstat`.
- [x] Run `go test ./tunnel -count=1`, `go test -race ./tunnel`, and the full suite.

### Task 17: Lazy Log Arguments On Hot Paths

**Files:**
- Modify: `log/log.go`
- Test: `log/log_test.go`
- Modify: `dns/resolver.go`, `dns/util.go`
- Modify: `tunnel/tunnel.go`, `tunnel/match_context.go`
- Modify: `listener/sing_tun/dns.go`
- Benchmark: `dns/log_bench_test.go`

**Interfaces:**
- Produces: `log.DebugEnabled()` and `log.InfoEnabled()`.
- Preserves: every log line and its payload; only argument construction becomes level-gated.

- [x] Add level helpers with tests covering debug, info, and silent levels.
- [x] Gate the DNS cache-hit and exchange log arguments behind `DebugEnabled`.
- [x] Gate the per-connection `logMetadata` body behind `InfoEnabled`.
- [x] Gate the tun DNS hijack logs and the rule-match resolve log.
- [x] Benchmark cache-hit and exchange argument construction with `benchstat`.
- [x] Run `go test ./dns ./tunnel ./log -count=1` and the full suite.

### Task 18: Reused UDP Send Destination

**Files:**
- Modify: `tunnel/connection.go`
- Test: `tunnel/connection_perf_test.go`

**Interfaces:**
- Produces: `synchronousWriteTarget(any) bool` and the per-sender destination cache.
- Preserves: per-destination address correctness for every outbound, including queued writers.

- [x] Reuse the destination address object only when the conn writes to a kernel socket.
- [x] Rebuild the address when the destination port changes.
- [x] Walk `Upstream()` with a typed-nil guard and a depth bound.
- [x] Test reuse, freshness, and rebuild behaviour through `processPacket`.
- [x] Benchmark kernel-backed versus queued conns with `benchstat`.
- [x] Run `go test ./tunnel -count=1`, `go test -race ./tunnel`, and the full suite.

### Task 14: Reusable UDP Write-Back Address (rejected)

Investigated and dropped: `listener/inner/udp.go` enqueues the `net.Addr` into a channel and hands the
pointer to the consumer in `ReadFrom`, so a per-loop reusable `*net.UDPAddr` would observe mutated
source addresses for queued datagrams. Making that safe would mean either a new synchronous-write-back
capability interface across every listener or moving the allocation to the inner read path, neither of
which is a net win at this blast radius. The per-packet `net.UDPAddrFromAddrPort` stays.
