# Phase 12 DNS Cache Key Report

Environment:

- Windows, `windows/amd64`
- CPU: Intel Core i9-9900 CPU @ 3.10GHz
- Go: `go1.27.1`
- Samples: 6 per case
- Baseline: the same commit with the string key, measured by stashing the change

Every DNS lookup formatted its question into a string to use as the cache key, on both the get and the
put path. `Question.String` builds an escaped representation with several allocations, which showed up
as the largest single allocation site in a cached lookup profile after the answer copy.

The `dnsCache` key is now a comparable `cacheKey{name, qtype, qclass}` struct built directly from the
question, so the key costs nothing to construct and hashes natively in the lru/arc maps. The string
key sprinted the name with escaping but never lowercased it, so name and type separation behave exactly
as before.

| Metric | String key | Struct key | Change |
| --- | ---: | ---: | ---: |
| LookupIPv4 cached: allocs | 16 | 13 | -18.75% |
| LookupIPv4 cached: bytes | 714 B | 642 B | -10.08% |
| LookupIP cached: time | 5.616 us | 4.732 us | -15.75% |
| LookupIP cached: allocs | 37 | 31 | -16.22% |

The cached lookup still copies the stored message before adjusting TTLs, which keeps downstream
caches honest; that copy is now the dominant remaining cost and needs the message, so it stays.

Verification:

```powershell
go test ./dns -count=1
go test -race ./dns -count=1
go vet ./dns
go test ./... -count=1 -p 2
```

The full main-module run passed.
