# Phase 40 Sing Packet Writer State Report

Environment:

- Windows, `windows/amd64`
- Go: `go1.27.1`
- CPU: Intel Core i9-9900
- `go test -bench '^BenchmarkNewPacketConnectionDatagramBatch$' -benchmem -count=6 ./listener/sing`
- Comparison: `benchstat baseline.txt current.txt`

The sing UDP connection kept two separate heap boxes for its packet writer and write mutex. They
are always used together, so they are now one `packetWriterState` object. The packet wrapper holds
one pointer, close/write-back ordering lives in one place, and the behavior is pinned by a
close-once test.

| Metric | Before | After | Change |
| --- | ---: | ---: | ---: |
| 100-datagram batch | 714.5B / 11 allocs | 715B / 10 allocs | -9.09% allocs (p=0.002) |
| Time | 6.719u + 80% | 7.919u + 25% | not significant |

Verification:

```powershell
go test -count=1 ./listener/sing
go test -race -count=1 ./listener/sing
go test ./... -count=1
```
