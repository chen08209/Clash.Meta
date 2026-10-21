# Phase 32 UDP Flow Key Reuse Report

Environment:

- Windows, `windows/amd64`
- Go: `go1.27.1`
- CPU: Intel Core i9-9900
- `go test -bench '^BenchmarkHandleUDPConnExistingFlow$' -benchmem -count=6|10 ./tunnel`
- Comparison: `benchstat baseline.txt current.txt`

The tunnel computed the packet flow key once to select a UDP worker and again inside
`handleUDPConn`. It also loaded the runtime snapshot before knowing whether an existing
flow would short-circuit. The queue now carries the already-validated key, and the
runtime snapshot is loaded only on the flow-creation branch.

| Metric | Before | After | Change |
| --- | ---: | ---: | ---: |
| Existing-flow handle | 92.69n + 2% | 89.15n + 2% | -3.81% (p=0.001) |
| Bytes/allocs per packet | 0 / 0 | 0 / 0 | unchanged |

Verification:

```powershell
go test -count=1 ./tunnel
go test -race -count=1 ./tunnel
go test ./... -count=1
```
