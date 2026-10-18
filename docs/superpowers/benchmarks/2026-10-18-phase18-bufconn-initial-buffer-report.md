# Phase 18 BufferedConn Initial Buffer Report

Environment:

- Windows, `windows/amd64`
- Go: `go1.27.1`
- Evidence: `BenchmarkNewBufferedConn` before and after, `benchstat` over 6 runs each

Every connection wraps its socket in a `BufferedConn` whose only reason to exist is peeking for the
sniffer, but it allocated a 4 KiB `bufio.Reader` up front. Most connections never peek past a TLS
record header, and the sniffer grows the buffer on demand when it needs more, so the eager 4 KiB was
pure per-connection memory. The initial size is now 512 bytes behind the exported
`N.InitialPeekBufferSize`, and the tests that peek past it grow explicitly first.

| Metric | 4096 start | 512 start | Change |
| --- | ---: | ---: | ---: |
| Construction time | 781.1 ns/op | 211.6 ns/op | -72.9% |
| Construction bytes | 4304 B/op | 720 B/op | -83.3% |
| Allocations | 6/op | 6/op | unchanged |

The allocation count is unchanged because the smaller `bufio` buffer only shrinks the single eager
allocation, and the remaining five are the wrapper objects the constructor already built.

Verification:

```powershell
go test ./common/net ./component/sniffer ./tunnel -count=1
go test ./... -count=1 -p 2
```

