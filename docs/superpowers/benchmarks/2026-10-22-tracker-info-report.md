# Phase 34 Tracker Info Embedding Report

Environment:

- Windows, `windows/amd64`
- Go: `go1.27.1`
- CPU: Intel Core i9-9900
- `go test -bench '^BenchmarkNewTCPTracker$' -benchmem -count=6 ./tunnel/statistic`
- Comparison: `benchstat baseline.txt current.txt`

`tcpTracker` and `udpTracker` allocated their tracker object and then allocated a separate
`TrackerInfo` object. Both now embed `TrackerInfo` by value and return the address of that
embedded field, so connection setup performs one fewer heap allocation while keeping the
same `Info()` pointer, UUID identity, counters, and JSON shape.

| Metric | Before | After | Change |
| --- | ---: | ---: | ---: |
| Allocations per TCP tracker | 8 | 7 | -12.50% (p=0.002) |
| Bytes per TCP tracker | 736 | 736 | unchanged |

Verification:

```powershell
go test -count=1 ./tunnel/statistic ./tunnel
go test -race -count=1 ./tunnel/statistic ./tunnel
go test ./... -count=1
```
