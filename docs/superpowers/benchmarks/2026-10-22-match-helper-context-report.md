# Match Helper Context Refactor

Date: 2026-10-22

Environment:

- Windows, `windows/amd64`
- CPU: Intel Core i9-9900 CPU @ 3.10GHz
- Go: `go1.27.1`
- Baseline: `efd78d90f`
- Change: `329f8ef03`

## Change

`C.RuleMatchHelper` previously carried three function fields. Building it for
one connection created three closures and an escaping `matchContext`; rules
that needed IP resolution paid four allocations before doing any work.

The helper now carries one `RuleMatchContext` interface. The tunnel
`matchContext` implements `ResolveIP`, `FindProcess`, and `CheckPassRule`, and
the helper methods keep the zero value safe. Source and `no-resolve` rule sets
set `NoResolve` on their local helper copy instead of replacing a function
field.

## Benchmark

Command:

```text
go test ./tunnel -run '^$' \
  -bench '^(BenchmarkMatchMixedRulesScan|BenchmarkMatchLargeIPRules|BenchmarkResolveMetadata)$' \
  -benchmem -benchtime=1s -count=10
```

`benchstat` result:

| Benchmark | Before | After | Change |
| --- | ---: | ---: | ---: |
| `MatchMixedRulesScan` | 72 B / 4 allocs | 24 B / 1 alloc | -66.7% bytes, -75% allocs |
| `MatchLargeIPRules/100` | 72 B / 4 allocs | 24 B / 1 alloc | -66.7% bytes, -75% allocs |
| `MatchLargeIPRules/1000` | 72 B / 4 allocs | 24 B / 1 alloc | -66.7% bytes, -75% allocs |
| `MatchLargeIPRules/5000` | 72 B / 4 allocs | 24 B / 1 alloc | -66.7% bytes, -75% allocs |
| `ResolveMetadata` | 416 B / 1 alloc | 416 B / 1 alloc | unchanged |

The wall-clock samples were noisy on this host, but the allocation reduction is
stable and statistically exact: every rule-match sample moved from four
allocations to one. The remaining allocation is the short-lived match context
itself.

## Verification

- `go test ./... -count=1 -p 2`
- `go test -race ./tunnel ./rules/common ./rules/logic -count=1`
- `go vet ./...`

`rules/provider` has a pre-existing `component/resource.Fetcher` race under
`-race`; it reproduces on the unmodified `efd78d90f` baseline and is unrelated
to the helper contract.
