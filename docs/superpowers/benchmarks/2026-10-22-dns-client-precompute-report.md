# Phase 43 UDP DNS Client Precomputation Report

Environment:

- Windows, `windows/amd64`
- Go: `go1.27.1`
- CPU: Intel Core i9-9900
- `go test -run '^$' -bench '^BenchmarkClientExchangeUDP$' -benchmem -count=3 ./dns`, four
  interleaved baseline/current rounds on the same host
- Comparison: `benchstat baseline.txt current.txt`

`client.ExchangeContext` rebuilt the transport network, the `host:port` string, and a
`dns.Client` value on every query. The client now derives them once in `newClient` and stores
the shared `dns.Client`, which `ExchangeWithConnContext` only reads; the redundant `schema`
field is gone because it always mirrored the network.

The benchmark performs a real UDP round trip against an in-process DNS server, so its wall
clock is dominated by scheduler wakeups. This host was noisy during the run: the recorded
samples span 22.5µs–79.8µs on the baseline tree and 22.4µs–68.0µs on the current tree, so the
timing column is reported for completeness only and carries no signal. Allocations are
deterministic and are the gate.

| Metric | Before | After | Change |
| --- | ---: | ---: | ---: |
| UDP exchange | 25.27µ ± 44% | 25.73µ ± 55% | ~ (p=0.932) |
| Bytes / allocations | 4.788Ki / 23 | 4.772Ki / 22 | -0.33% / -1 alloc (p=0.000) |

Verification:

```powershell
go vet ./dns
go test -count=1 ./dns
go test -race -count=1 ./dns
go test ./... -count=1
```
