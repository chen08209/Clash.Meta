# Phase 35 BufferedConn Wrapper Removal Report

Environment:

- Windows, `windows/amd64`
- Go: `go1.27.1`
- CPU: Intel Core i9-9900
- `go test -bench '^BenchmarkNewBufferedConn$' -benchmem -count=6 ./common/net`
- Comparison: `benchstat baseline.txt current.txt`

`BufferedConn` embedded a Sing `ExtendedConn`. For a plain `*net.TCPConn`, that created an
`ExtendedConnWrapper` plus reader and writer wrappers on every connection, even though
`BufferedConn` only needs a small peek reader and generic read/write passthrough. The wrapper is
now gone: `BufferedConn` holds the underlying `net.Conn`, implements buffer reads and writes
itself, and still uses an underlying `ExtendedConn` implementation when one exists.

| Metric | Before | After | Change |
| --- | ---: | ---: | ---: |
| Construction time | 219.65n + 3% | 99.85n + 4% | -54.54% (p=0.002) |
| Construction bytes | 720 | 512 | -28.89% (p=0.002) |
| Allocations | 6 | 1 | -83.33% (p=0.002) |

Verification:

```powershell
go test -count=1 ./common/net ./component/sniffer ./tunnel
go test -race -count=1 ./common/net ./component/sniffer ./tunnel
go test ./... -count=1
```
