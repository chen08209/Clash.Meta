# Regression audit of the recorded benchmarks

Date: 2026-10-22

Environment: Windows, `windows/amd64`, Intel Core i9-9900, Go `1.27.1`. The host
was running several other builds and test suites while these were taken, so the
ns/op columns are noisier than the ledger's originals; the allocation columns
are not.

Several dozen commits landed on this line after the 2026-10-21 ledger was
written, most of them from parallel work on the same core. Re-running the
ledger's benchmarks is the cheapest way to check that none of them walked an
earlier optimisation back.

| benchmark | ledger | now | verdict |
| --- | --- | --- | --- |
| `BenchmarkSocksUDPHandleCached` | 77-92 ns, 1 alloc | 74.9-96.6 ns, 0 allocs | allocation removed |
| `BenchmarkLookupProcessNameCacheHit` | 50.7-53.7 ns, 0 allocs | 56.2-67.7 ns, 0 allocs | noise, see below |
| `BenchmarkNewPacketConnectionDatagramBatch` | 312 allocs | 10 allocs, 715 B | 302 allocations removed |
| `BenchmarkFallbackSelectedProxyCached` | 20.1-20.7 ns | 22.6-24.4 ns | noise |
| `BenchmarkGetProxiesCacheHit` | 0.68-0.79 ns | 0.83-0.98 ns | noise |
| `BenchmarkURLTestSelectedProxyCached` | 24.2-31.4 ns | 26.4-33.5 ns | noise |
| `BenchmarkMatchLargeIPRules/rules=5000` | 174-185 ns | 123.5-162.4 ns | faster |
| `BenchmarkHandleUDPConnExistingFlow` | 96-100 ns, 0 allocs | 40.5-53.5 ns, 0 allocs | ~2x faster |
| `BenchmarkProcessPacketHit` | 29 ns, 0 allocs | 25.6-28.2 ns, 0 allocs | unchanged |

Allocation counts are equal or lower everywhere, which is the part of the
measurement this host cannot distort. The sub-30 ns micro-benchmarks sit 10-25%
above their recorded values; `git log -- component/process` shows no
performance commit after the one that produced the recorded number, so that
delta is the loaded host rather than a regression in the line.

## Investigated and rejected

`transport/mkcp` `packetWriter.writeSegment` still allocates a plaintext
scratch and then a datagram per segment. Sealing in place with `plaintext[:0]`
as `dst`, the pattern the `cipher.AEAD` documentation suggests, is not safe
here: `simpleAuthenticator.Seal` appends its six-byte prefix into that same
array before copying the plaintext, so the first bytes of the payload would be
overwritten with the prefix. The scratch is required by that implementation,
and the `security == nil` branch the old code carried is unreachable because
`Config.security` always returns an authenticator.

Reading a two-byte length through `io.ReadFull(conn, header[:])` still escapes
the array and allocates, because `conn` is an interface. The only ways to
remove those are to reuse a per-direction field on the conn or to park the
length in the caller's buffer before overwriting it; both change a public
method's behaviour for buffers smaller than the header, so the remaining sites
(`transport/gost` `ReadFrom`, `transport/simple-obfs` reads) were left alone.

## Coverage added

`TestRoundTripWithSeed` runs the full mKCP handshake and 64 KiB round trip with
a seed, which selects aes-128-gcm. Before it, every mKCP test used `Config{}`
and therefore only the built-in authenticator: the `cipher.NewGCM` seal/open
path was never executed. It now is, and it also exercises the receive path's
payload aliasing against a real AEAD.

## Verification

- `go test ./... -count=1 -p 2` green (129 s).
- `go test -race ./listener/sing_vless ./transport/gost` green.
- `go vet ./...` clean.
- `go build -tags with_gvisor ./...` and the tagged tests green.
