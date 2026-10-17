# Phase 4 UDP Origin Key Report

Environment:

- Windows, `windows/amd64`
- CPU: Intel Core i9-9900 CPU @ 3.10GHz
- Go: `go1.27.1`
- Samples: 6 per case
- Baseline: commit `92889146`

The UDP sender previously formatted every mapped packet destination with `Metadata.String()` before looking it up in `originToTarget`. For an IP destination this formatted a new string on every packet.

The map now uses a comparable key containing either the existing host string or the binary `netip.Addr`, with no formatting on lookup.

| Metric | Baseline | Binary key | Change |
| --- | ---: | ---: | ---: |
| Time | 36.71 ns/op | 17.23 ns/op | -53.06% |
| Bytes | 16 B/op | 0 B/op | -100.00% |
| Allocations | 1 alloc/op | 0 allocs/op | -100.00% |

Verification:

```powershell
go test ./tunnel -run TestPacketOriginKey -count=1
go test -race ./tunnel -count=1
go test ./...
```
