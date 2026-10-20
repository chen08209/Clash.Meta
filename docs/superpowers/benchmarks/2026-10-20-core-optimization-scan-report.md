# Core Optimization Scan Report

Date: 2026-10-20

Environment:

- Windows, `windows/amd64`
- CPU: Intel Core i9-9900 CPU @ 3.10GHz
- Go: `go1.27.1`

This report scans the forwarding core for the remaining optimization targets after the
2026-10-17 performance phases and the 2026-10-20 netip.AddrPort refactor. Every number is
measured by a benchmark or a pprof profile in this tree; code-inspection findings are marked.

## Fixed In This Cycle

| Area | Before | After | Evidence |
| --- | ---: | ---: | --- |
| UDP write-back (socks/shadowsocks/mieru) | 2 allocs + payload copy | 0 allocs | `BenchmarkPacketWriteBack` 30 ns, 0 B |
| UDP send destination (queued outbounds) | 70 ns, 2 allocs | 26 ns, 0 allocs | `BenchmarkProcessPacketHit` |
| NAT local-conn lookup (tproxy) | 2 string allocs + parse | 0 allocs | `BenchmarkGetForLocalConnHit` 68 ns |
| Rule scan, 5000 CIDR list | ~62,494 ns | ~165 ns full match | `BenchmarkMatchLargeIPRules` |
| URLTest selection (warm) | 1 closure alloc | 0 allocs | `BenchmarkURLTestSelectedProxyCached` 50 ns |
| trojan inbound packet | 6 allocs | 4 allocs | `TestInboundNewPacketAllocationCount` |

Two defects were found and fixed on the way: listeners that wrap the client address in a
per-connection SNAT key (trojan, sudoku, snell, non-TUN sing) dropped their UDP because the
flow key could not parse the wrapper, and a DNS exchange whose context deadline fired through
the socket deadline leaked `i/o timeout` instead of the context error.

## Remaining Findings

### F1. Per-read allocations in `WaitReadFrom` (highest remaining per-packet cost)

Every UDP datagram read pays nine allocations and about 209 bytes on Windows
(`BenchmarkWaitReadFrom`), roughly four on POSIX. The pprof breakdown:

| Source | Share | Cause |
| --- | ---: | --- |
| `enhanceUDPConn.WaitReadFrom` | 55% | the `put` closure and read bookkeeping |
| `WaitReadFrom.func1` (rawConn callback) | 23% | the closure passed to `RawConn.Read` |
| `internal/syscall/windows.errnoErr` | 15% | errno boxed into `error` |
| `windows.RawSockaddrAny.Sockaddr` | 11% | the syscall-layer address object |

Two mitigations were investigated and dropped: caching the `put` closure per conn is unsafe
because `NewThreadSafePacketConn` does not serialize reads and the deadline wrapper supports
two concurrent readers by design; replacing the `put` contract with a returned buffer would
route sing-buffer memory into the mihomo pool unless every sing read path is re-plumbed.
Both are real refactors with cross-cutting risk, listed here as the next candidates in
descending order of expected gain.

### F2. Syscall-layer address allocation (POSIX and Windows)

`syscall.Recvmsg` (POSIX) and `windows.Recvfrom` (Windows) allocate their `Sockaddr` result
per call. Avoiding it means raw syscalls with a preallocated `RawSockaddrAny` per conn, once
per platform (linux, darwin, BSDs, windows). Expected gain: one to two allocations per UDP
read. Risk is high: each platform needs its own verified path.

### F3. DNS outbound: one goroutine and a context pair per query (code inspection)

`adapter/outbound/dns.go` `WriteTo` spawns a goroutine per query carrying a
`context.WithTimeout` pair and a `put` closure: about four allocations and one goroutine per
DNS query through a proxy node. Queries on one flow are serialized by the tunnel sender, so a
lazily started per-conn pump goroutine reading a request channel would amortize them. Gain is
moderate but DNS-through-proxy sits on the critical path of every new domain connection.

### F4. `listener/inner` UDP path (code inspection)

`inner.HandleTcp`/`HandleUdp` have no callers inside the repository (the hub only wires
`inner.New`). Its `WriteTo` also formats and re-parses the target address per packet
(`addr.String()` + `socks5.ParseAddr`, two to three allocations). Decide first whether the
embedder API is still needed; if not, the package is dead code to remove, if yes, the
`WriteTo` path should go netip-native like the rest of the funnel.

### F5. Checked and clean

- No `fmt.Sprintf` remains on the packet paths (only cold setup and debug formatting).
- `time.Now()` on the UDP read path is gated by the half-timeout deadline check.
- NAT flow tables, packet-origin mapping and write-back are allocation-free per packet.
- Selector, URLTest and Fallback selections are allocation-free per connection.
- TCP connection setup is dominated by the dial itself; its allocations are the peek buffer
  and the extended-conn wrapper, both once per connection.

## Recommended Order

1. F3 (contained, per-query win on the DNS critical path).
2. F1 (largest per-packet win; design the no-compatibility `WaitReadFrom` contract first).
3. F4 (decide the embedder question, then delete or netip-native).
4. F2 (per-platform raw syscalls; highest risk, do last with per-platform CI coverage).
