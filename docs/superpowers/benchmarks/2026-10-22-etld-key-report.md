# Phase 39 Load Balance Key Report

Environment:

- Windows, `windows/amd64`
- Go: `go1.27.1`
- CPU: Intel Core i9-9900
- `go test -bench '^BenchmarkStickySessionsProxyPick$' -benchmem -count=6 ./adapter/outboundgroup`
- Comparison: `benchstat baseline.txt current.txt`

The load-balance key parsed every domain host as a potential IP. Failed `netip.ParseAddr`
calls returned boxed errors, so every domain lookup allocated and spent time in the parser.
The key now prechecks the address alphabet before parsing, while an eTLD+1 hash cache removes
repeated public-suffix work for the same host. IP and domain hash semantics stay unchanged.

| Metric | Before | After | Change |
| --- | ---: | ---: | ---: |
| Sticky pick | 1839.0n + 14% | 258.2n + 3% | -85.96% (p=0.002) |
| Bytes | 464 | 416 | -10.34% (p=0.002) |
| Allocations | 2 | 1 | -50% (p=0.002) |

The remaining allocation is the benchmark's own metadata construction.

Verification:

```powershell
go test -count=1 ./adapter/outboundgroup
go test -race -count=1 ./adapter/outboundgroup
go test ./... -count=1
```
