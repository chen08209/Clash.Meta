# Trojan UDP Packet Wrapper Pool

Environment:

- Windows, `windows/amd64`
- Go: `go1.27.1`
- CPU: Intel Core i9-9900
- `go test ./listener/trojan -run '^$' -bench '^BenchmarkTrojanPacketDrop$' -benchmem -count=8`

The owned path models the previous behavior: every datagram allocated a new
`packet` wrapper and `Drop` only returned the payload buffer. The pooled path
returns the wrapper through a `sync.Pool` at the same `Drop` release point.

| Metric | Owned | Pooled | Change |
| --- | ---: | ---: | ---: |
| Wrapper drop | 70.08n ± 14% | 30.48n ± 3% | -56.51% (p=0.000 n=8) |
| Bytes / allocations | 96 B / 1 alloc | 0 B / 0 allocs | -100% |

The existing wrapper semantics are covered by the trojan packet and inbound
packet tests.
