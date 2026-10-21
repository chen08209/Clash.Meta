# Phase 44 UDP Read Through The Conn Poller Report

Environment:

- Windows, `windows/amd64`
- Go: `go1.27.1`
- CPU: Intel Core i9-9900
- `go test -run '^$' -bench '^BenchmarkWaitReadFrom$' -benchmem -count=3 ./common/net/packet`, four
  interleaved baseline/current rounds on the same host
- Comparison: `benchstat baseline.txt current.txt`

Four platform files (`packet_windows.go`, `packet_linux.go`, `packet_linux_386.go`,
`packet_posix.go`) each carried their own `syscall.RawConn.Read` closure that called a raw
recv into pooled memory. The closure contract forces its frame onto the heap, so every
datagram read allocated: `pprof -sample_index=alloc_objects` put all six objects per read
inside `WaitReadFrom` (the function prologue, `readErr`, `hasData` and the closure itself),
and the readiness-then-recv shape spent one poll cycle on a callback that only reported
readiness.

The wrapped `*net.UDPConn` now reads through `ReadFromUDPAddrPort` into the same pooled
buffer. The `AddrPort` variant fills a stack sockaddr, so the read path allocates nothing on
any platform, and the poller drives the recvfrom itself instead of a second callback. The
files collapse into one unsuffixed implementation; the per-platform sockaddr converters and
their wire-order test go with them.

| Size | Before | After | Change |
| --- | ---: | ---: | ---: |
| small | 9.591µ ± 29% | 8.921µ ± 28% | ~ (p=0.143) |
| 1500B | 12.931µ ± 37% | 9.357µ ± 30% | -27.64% (p=0.005) |
| 1500B throughput | 110.6Mi ± 59% | 153.1Mi ± 43% | +38.37% (p=0.005) |
| Bytes / allocations | 137 / 6 | 0 / 0 | -100% (p=0.000) |

This also closes the deferred darwin/BSD half of F2: those platforms read through the syscall
wrappers that allocate a typed sockaddr per call, and they now ride the same stdlib path as
the rest of the tree. Only the compilation of that path is verified locally; no darwin or BSD
test binary runs on this host.

Verification:

```powershell
go vet ./common/net/packet
go test -count=1 ./common/net/packet
go test -race -count=1 ./common/net/packet ./common/net/deadline ./tunnel ./listener/socks ./listener/shadowsocks ./listener/trojan ./listener/mieru ./listener/tunnel ./listener/tproxy
go build ./common/net/packet ./common/net/deadline ./tunnel ./listener/...   # linux amd64/386/arm64, darwin amd64/arm64, windows amd64/arm64/386, freebsd/amd64, android/arm64
go test ./... -count=1
```
