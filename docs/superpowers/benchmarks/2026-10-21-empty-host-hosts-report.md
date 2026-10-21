# Phase 33 Empty-Host Hosts Lookup Report

Environment:

- Windows, `windows/amd64`
- Go: `go1.27.1`
- CPU: Intel Core i9-9900
- `go test -bench '^BenchmarkPreHandleMetadataEmptyHost$' -benchmem -count=6 ./tunnel`
- Comparison: `benchstat baseline.txt current.txt`

An IP-only TCP connection has no host, but metadata prehandling and rule resolution still
asked the hosts trie for an empty name. `normalizeDomain("")` then allocated an invalid-domain
error and three objects on every connection before returning a miss. The tunnel now skips the
hosts lookup when the host is empty. Hosted and mapped lookups keep the same semantics.

| Metric | Before | After | Change |
| --- | ---: | ---: | ---: |
| Time | 194.5n + 144% | 2.9n + 3% | -98.51% (p=0.002) |
| Bytes | 96 | 0 | -100% (p=0.002) |
| Allocations | 3 | 0 | -100% (p=0.002) |

Verification:

```powershell
go test -count=1 ./tunnel
go test -race -count=1 ./tunnel
go test ./... -count=1
```
