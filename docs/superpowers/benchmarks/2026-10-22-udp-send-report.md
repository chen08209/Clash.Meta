# Phase 37 UDP Send Fast Path Report

Environment:

- Windows, `windows/amd64`
- Go: `go1.27.1`
- CPU: Intel Core i9-9900
- `go test -bench '^BenchmarkHandleUDPConnExistingFlow$' -benchmem -count=6 ./tunnel`
- Comparison: `benchstat baseline.txt current.txt`

Every existing-flow UDP packet went through a two-armed `select` containing `ctx.Done()`. The hot
path now reads an atomic closed flag, registers the in-flight send, and rechecks the flag before
using a buffered-channel-only send. `Close` publishes that flag, waits for in-flight sends to
finish, and only then drains the queue. Closed senders still drop packets, and a full queue still
drops without blocking.

| Metric | Before | After | Change |
| --- | ---: | ---: | ---: |
| Existing-flow handle | 91.79n ± 13% | 38.90n ± 12% | -57.62% (p=0.002) |
| Bytes / allocations | 0 / 0 | 0 / 0 | unchanged |

Focused send gates:

| Path | Cost |
| --- | ---: |
| ready | 47.34n + 12%, 0 allocs |
| full | 13.21n + 8%, 0 allocs |
| closed | 2.773n + 4%, 0 allocs |

Verification:

```powershell
go test -count=1 ./tunnel
go test -race -count=1 ./tunnel
go test ./... -count=1
```
