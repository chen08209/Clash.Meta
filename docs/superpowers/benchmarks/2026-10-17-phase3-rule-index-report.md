# Phase 3 Rule Index Benchmark Report

Environment:

- Windows, `windows/amd64`
- CPU: Intel Core i9-9900 CPU @ 3.10GHz
- Go: `go1.27.1`
- Benchmarks: 6 samples per case
- Comparison tool: `D:\Codex\tools\go-bin\benchstat.exe`
- Baseline: commit `bf2846b6`

The index groups every contiguous run of `DOMAIN` and `DOMAIN-SUFFIX` rules. Exact names use a hash; suffixes use a reversed-label trie. Both store rule positions so first-match order, disabled wrappers, duplicate payloads, PASS, REMATCH, and intervening non-domain rules keep their original semantics.

Results:

| Case | Baseline | Indexed | Change |
| --- | ---: | ---: | ---: |
| 10k exact-domain hit | 114817.00 ns/op | 66.18 ns/op | -99.94% |
| 10k exact-domain miss | 228697.50 ns/op | 37.48 ns/op | -99.98% |
| 10k domain-suffix hit | 90512.00 ns/op | 102.00 ns/op | -99.89% |
| 10k domain-suffix miss | 179639.00 ns/op | 34.03 ns/op | -99.98% |

All four indexed cases remain `0 B/op, 0 allocs/op`.

Verification:

```powershell
go test ./tunnel -run 'TestMatch' -count=1
go test ./tunnel -count=1
go test -race ./tunnel -count=1
go test ./...
```

The full main-module run completed successfully. Its slowest existing package was `listener/inbound` at 86.263 seconds.
