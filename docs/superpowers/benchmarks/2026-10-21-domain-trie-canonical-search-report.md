# Phase 30 Canonical Domain Search Fast Path Report

Environment:

- Windows, `windows/amd64`
- Go: `go1.27.1`
- CPU: Intel Core i9-9900
- `go test -bench '^BenchmarkDomainMapLookup/int/trie/(exact|suffix|wildcard|unicode)$' -benchmem -count=6`
- Comparison: `benchstat baseline.txt current.txt`

`DomainTrie.Search` paid for the full `normalizeDomain` scan on every query, including the common
ASCII lowercase, non-wildcard case. Search now accepts that canonical form directly and falls back
to `normalizeDomain` for uppercase, Unicode, whitespace, wildcards, empty labels, and trailing dots.
This keeps pattern rejection and wildcard semantics on the same code path as before.

| Query | Before | After | Change |
| --- | ---: | ---: | ---: |
| exact | 171.6n ± 28% | 128.7n ± 36% | -25.03% (p=0.015) |
| suffix | 146.10n ± 2% | 76.38n ± 2% | -47.72% (p=0.002) |
| wildcard | 150.15n ± 2% | 83.59n ± 3% | -44.33% (p=0.002) |
| unicode | 209.0n ± 11% | 190.6n ± 6% | not significant |

Allocations stayed at 0 for the exact, suffix, wildcard, and Unicode lookup benchmarks.

Verification:

```powershell
go test -count=1 ./component/trie ./component/resolver ./tunnel ./dns
go test ./... -count=1
go vet ./component/trie ./component/resolver ./tunnel ./dns
go test -race -count=1 ./component/trie ./tunnel ./dns
```
