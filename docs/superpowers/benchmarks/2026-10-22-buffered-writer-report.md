# Phase 36 BufferedConn Writer Retention Report

Environment:

- Windows, `windows/amd64`
- Go: `go1.27.1`
- CPU: Intel Core i9-9900
- `go test -bench '^BenchmarkBufferedConnExtendedWriter$' -benchmem -count=6 ./common/net`
- Comparison: `benchstat baseline.txt current.txt`

After removing the eager `ExtendedConnWrapper`, relay code still unwrapped an empty
`BufferedConn` back to the raw socket and then asked Sing to wrap that socket as an extended
writer again. `BufferedConn` already implements `ExtendedWriter`, so the write side is now kept
in place. The read side keeps its existing rule: it is replaceable only when no peeked bytes
remain.

| Metric | Before | After | Change |
| --- | ---: | ---: | ---: |
| Time | 23.26n + 2% | 6.13n + 37% | -73.65% (p=0.002) |
| Bytes | 16 | 0 | -100% (p=0.002) |
| Allocations | 1 | 0 | -100% (p=0.002) |

Verification:

```powershell
go test -count=1 ./common/net ./component/sniffer ./tunnel
go test -race -count=1 ./common/net ./component/sniffer ./tunnel
go test ./... -count=1
```
