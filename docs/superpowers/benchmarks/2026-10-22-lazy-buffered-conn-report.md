# Phase 38 Lazy BufferedConn Reader Report

Environment:

- Windows, `windows/amd64`
- Go: `go1.27.1`
- CPU: Intel Core i9-9900
- `go test -bench '^BenchmarkNewBufferedConn$' -benchmem -count=6 ./common/net`
- Comparison: `benchstat baseline.txt current.txt`

`BufferedConn` allocated a 512-byte `bufio.Reader` for every TCP connection even though most
connections never peek. The reader is now created lazily by `Peek`, `Grow`, append/prepend
helpers, and explicit reader access. Plain relay reads go straight to the underlying connection;
peeked data keeps its existing buffered-reader semantics.

| Metric | Before | After | Change |
| --- | ---: | ---: | ---: |
| Construction time | 404.45n + 101% | 0.29n + 79% | -99.93% (p=0.002) |
| Construction bytes | 512 | 0 | -100% (p=0.002) |
| Allocations | 1 | 0 | -100% (p=0.002) |

Verification:

```powershell
go test -count=1 ./common/net ./component/sniffer ./tunnel
go test -race -count=1 ./common/net ./component/sniffer ./tunnel
go test ./... -count=1
```
