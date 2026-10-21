# Rule Match Context Pool

Date: 2026-10-22

Environment:

- Windows, `windows/amd64`
- CPU: Intel Core i9-9900 CPU @ 3.10GHz
- Go: `go1.27.1`
- Baseline: `ddc0e1d90`
- Change: `90ea4115f`

## Change

The match helper context was still allocated once per rule match after the
helper-interface refactor. `matchContext` now comes from a `sync.Pool`, is
fully reinitialized on acquisition, and is zeroed and returned once
`tunnel.match` exits. The release is deferred at function entry so the
compiler can open-code it; a defer placed inside the rule loop allocated a
closure per match and would have defeated the pool.

## Benchmark

Command:

```text
go test ./tunnel -run '^$' \
  -bench '^(BenchmarkMatchMixedRulesScan|BenchmarkMatchLargeIPRules)$' \
  -benchmem -benchtime=1s -count=10
```

`benchstat` result:

| Benchmark | Before | After | Change |
| --- | ---: | ---: | ---: |
| `MatchMixedRulesScan` | 24 B / 1 alloc | 0 B / 0 allocs | allocation gate removed |
| `MatchLargeIPRules/100` | 24 B / 1 alloc | 0 B / 0 allocs | -46.3% latency (p=0.000) |
| `MatchLargeIPRules/1000` | 24 B / 1 alloc | 0 B / 0 allocs | -22.0% latency (p=0.000) |
| `MatchLargeIPRules/5000` | 24 B / 1 alloc | 0 B / 0 allocs | -55.0% latency (p=0.000) |
| `MatchMixedRulesScan` latency | 8.800 us | 8.118 us | within noise (p=0.075) |

The change also adds a concurrent pool-isolation test that reuses contexts
across 32 goroutines with distinct metadata and verifies every acquisition is
overwritten before use.

## Verification

- `go test ./... -count=1 -p 2`
- `go test -race ./tunnel ./rules/common ./rules/logic -count=1`
- `go vet ./...`
