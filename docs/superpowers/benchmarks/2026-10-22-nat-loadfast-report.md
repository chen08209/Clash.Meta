# Phase 41 NAT Flow Lookup Fast Path Report

Environment:

- Windows, `windows/amd64`
- Go: `go1.27.1`
- CPU: Intel Core i9-9900
- `go test -bench '^BenchmarkTableGetOrCreateForFlowHit$' -benchmem -count=6 ./component/nat`
- Comparison: `benchstat baseline.txt current.txt`

`GetOrCreateForFlow` used the combined load-or-store path even on a hit. It now checks the
read-only map first and only falls back to load-or-store on a miss. Existing flows keep their
first-mapping semantics; new flows still use the same maker and race-safe publication.

| Metric | Before | After | Change |
| --- | ---: | ---: | ---: |
| Existing-flow get-or-create | 24.44n + 3% | 21.42n + 3% | -12.38% (p=0.002) |
| Bytes / allocations | 0 / 0 | 0 / 0 | unchanged |

The existing-flow handle benchmark stayed at about 38.5 ns / 0 allocs, so the map fast path is
the measured gate.

Verification:

```powershell
go test -count=1 ./component/nat ./tunnel
go test -race -count=1 ./component/nat ./tunnel
go test ./... -count=1
```
