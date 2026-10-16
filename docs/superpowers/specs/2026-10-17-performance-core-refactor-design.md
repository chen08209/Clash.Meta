# Performance Core Refactor Design

## Goal

Refactor the FlClash Clash.Meta fork for maximum forwarding performance while preserving its public Go APIs, configuration schema, protocol behaviour, and FlClash integration.

## Chosen Approach

Use an evolutionary Go refactor instead of a clean-room rewrite. The proxy's value is its mature protocol and rule compatibility; replacing that surface in one step would make performance improvements impossible to attribute and regressions difficult to isolate. The work therefore moves hot-path state to immutable snapshots, removes avoidable allocations and synchronization, and makes every optimization measurable.

## Performance Principles

1. A packet or connection hot path must not allocate for work that can be precomputed at configuration time.
2. Configuration reads use immutable snapshots. A connection never takes a configuration lock merely to choose a rule or proxy.
3. Observability is bounded and lazy. Disabled logs must not format strings, and identifiers must not be rendered unless exposed.
4. UDP processing preserves per-flow ordering while avoiding one goroutine per packet.
5. Every accepted optimization needs a benchmark covering throughput, allocation count, or p99 latency. A change that improves a microbenchmark but regresses an end-to-end relay benchmark is rejected.

## Architecture

### 1. Runtime Snapshot

Create one immutable snapshot containing rules, sub-rules, listeners, proxies, providers, rule providers, mode, and sniffer state. Updates publish a complete snapshot through an atomic pointer. Existing exported compatibility functions read the snapshot and retain their current signatures.

The snapshot is built outside the publish point. Readers acquire no lock, cannot observe a partially updated configuration, and do not cache stale pointers beyond one connection or packet.

### 2. TCP Fast Path

TCP admission remains one goroutine per connection, which is the correct model for blocking, protocol-heavy streams. The refactored path:

- reuses a per-connection match context instead of allocating three closures for every connection;
- evaluates process and DNS rule helpers lazily;
- makes tracker identities lazy and keys connection state by the binary UUID rather than its formatted string;
- checks log level before formatting metadata and rule descriptions;
- keeps sing's buffered relay implementation, which already avoids unnecessary `io.Copy` adapter allocations.

### 3. UDP Flow Path

UDP continues to hash packets to a stable worker queue so packets in one flow remain ordered. The refactored path:

- uses `netip.AddrPort` and binary flow keys instead of formatted strings;
- stores NAT mappings in a sharded, fixed-size table with generation-based cleanup;
- processes packets with batch reads and writes when the platform exposes them;
- applies a timing-wheel timeout rather than one timer per session;
- drops under backpressure and exposes the drop count instead of blocking the inbound reader.

### 4. Group And Provider Selection

Proxy groups receive an immutable member snapshot after every provider version change. URLTest, Fallback, LoadBalance, and Selector keep their public behaviour but store only the selected identity and health stamp in the group. Resolution compares versions first and then resolves from the immutable member snapshot without a mutex on the common hit path.

### 5. Benchmark Harness

Add focused Go benchmarks under `tunnel`, `tunnel/statistic`, `adapter/outboundgroup`, and `dns`. They cover:

- metadata and rule matching;
- tracker join, leave, read, and write;
- group selection under concurrent dials;
- TCP relay throughput for direct and chained connections;
- UDP packet classification and NAT lookup;
- DNS cache hit and upstream exchange allocation.

The harness records bytes per operation, allocations per operation, and B/op. Comparative runs use `benchstat`; a change is performance-positive only if its target improves without a material end-to-end regression.

## Compatibility

- Existing YAML and JSON configuration formats remain valid.
- Existing Go interfaces used by FlClash remain source-compatible.
- Rule semantics, proxy group selection, log output above the active level, and connection close behaviour remain unchanged.
- The external connection ID remains its formatted UUID string.

## Error Handling

Configuration publication is atomic and never exposes partial state. Packet queues fail closed under overload, increment a counter, and never block inbound platform threads. A provider failure preserves the previous immutable member snapshot. Benchmark and regression failures stop a phase rather than being bypassed.

## Phases

1. Benchmark and hot-path inventory: lock the baseline before changing behaviour.
2. Logging and statistics: remove hidden formatting and identifier allocation costs.
3. Runtime snapshot: eliminate configuration read locks and torn reads.
4. TCP match context: remove per-connection closures and duplicate metadata work.
5. UDP flow table: binary keys, sharding, bounded queues, and batch I/O.
6. Group snapshots: lock-free common selection paths.
7. End-to-end tuning: relay buffers, platform-specific zero-copy opportunities, and final benchmark gates.

## Non-Goals

- No protocol removal, configuration rewrite, or UI-facing feature work.
- No language rewrite in phase one.
- No platform-specific optimization without a benchmark on that platform.
