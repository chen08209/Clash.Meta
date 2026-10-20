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
| SOCKS5 outbound UDP write | 2 allocs + payload copy | 0 allocs | `TestSocksPacketConnWriteToStaysAllocationFree` |

Two defects were found and fixed on the way: listeners that wrap the client address in a
per-connection SNAT key (trojan, sudoku, snell, non-TUN sing) dropped their UDP because the
flow key could not parse the wrapper, and a DNS exchange whose context deadline fired through
the socket deadline leaked `i/o timeout` instead of the context error.

## Remaining Findings

### F1. Per-read allocations in `WaitReadFrom`: the closure contract is gone

Every UDP datagram read paid nine allocations and about 209 bytes on Windows
(`BenchmarkWaitReadFrom`), roughly four on POSIX. The closure contract is now gone: the read
hands the caller mihomo-pool memory directly and the caller recycles it with `pool.Put`, the
UDP conns recycle inside the read callback, and the sing-backed read copies into pooled
memory while releasing the sing buffer immediately. The packet benchmark now reads seven
allocations and 170 bytes per datagram, and `-race` passes for the packet, deadline and
tunnel packages. What remains per read is the syscall-layer work in F2.

### F2. Syscall-layer address allocation (POSIX and Windows)

`syscall.Recvmsg` (POSIX) and `windows.Recvfrom` (Windows) allocate their `Sockaddr` result
per call. Avoiding it means raw syscalls with a preallocated `RawSockaddrAny` per conn, once
per platform (linux, darwin, BSDs, windows). Expected gain: one to two allocations per UDP
read. Deferred: the BSD paths are not built by any CI job, so a regression there would ship
silently - this needs per-platform verification capability first.

### F3. DNS outbound per-query goroutine: investigated and dropped

`adapter/outbound/dns.go` `WriteTo` spawns a goroutine per query carrying a
`context.WithTimeout` pair and a `put` closure: about four allocations and one goroutine per
DNS query through a proxy node. A per-conn pump goroutine reading a request channel was the
considered fix, but it would serialize queries that today relay concurrently: a client's
resolver sends its A and AAAA questions back to back, and the second answer would wait behind
the first full relay for an allocation saving of one or two on a low-QPS path. The per-query
goroutine stays.

### F4. `listener/inner` UDP path: fixed

`inner.HandleTcp`/`HandleUdp` are the internal routing entry for the TLS-wrapper listeners
(jls, reality, restls, shadowtls, shadowquic, sing_hysteria2, sudoku, tlsmirror and the http
component) - an initial grep missed them behind a display artifact. Its `WriteTo` formatted
and re-parsed the target address per packet (`addr.String()` + `socks5.ParseAddr`, two to
three allocations) although every caller writes through a bind conn whose address is the
conn's own fixed target. The target is parsed once per conn and reused; the per-packet
format-and-parse round-trip is gone.

### F7. DNS cache-hit message copy (code inspection)

`getMsgFromCache` deep-copies the cached `dns.Msg` on every cache hit (`msg.Copy()` builds a
fresh message with new RR slices). The parsed-IP cache (`ip_cache.go`) already answers the hot
`LookupIP` path without touching messages, so this cost is on non-IP queries only; a pooled
or copy-on-write message representation would remove it if DNS QPS ever matters.

### F8. Resolver goroutine density per lookup (code inspection)

`Resolver.LookupIP` starts one goroutine for the AAAA race, `ipExchange` starts one per
`asyncExchange` (main plus fallback), and `refreshAsync` one per expiring entry: three to four
goroutines per resolution. At typical DNS QPS this is not a hotspot; noted because a bounded
worker pool would also bound the singleflight retry monitors.

### F9. KCP (mkcp) per-segment buffers (code inspection)

`transport/mkcp/packet.go` allocates a fresh `[]byte` per segment read and the listener reads
through a fresh 64 KiB buffer per datagram. Niche transport; only worth touching if mkcp
profiles hot.

### F5. Checked and clean

- No `fmt.Sprintf` remains on the packet paths (only cold setup and debug formatting).
- `time.Now()` on the UDP read path is gated by the half-timeout deadline check.
- NAT flow tables, packet-origin mapping and write-back are allocation-free per packet.
- Selector, URLTest and Fallback selections are allocation-free per connection.
- TCP connection setup is dominated by the dial itself; its allocations are the peek buffer
  and the extended-conn wrapper, both once per connection.

## Recommended Order

1. F2 (the last per-read item; staged behind per-platform verification capability - the BSD
   paths are built by no CI job today).
2. F7-F9 (hold until profiling shows they matter; F7 needs a message representation change,
   F8 a bounded pool, F9 is niche).
