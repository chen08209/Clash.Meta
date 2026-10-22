# Phase 13 Rule Helper Flag Report

Environment:

- Windows, `windows/amd64`
- CPU: Intel Core i9-9900 CPU @ 3.10GHz
- Go: `go1.27.1`
- Samples: 6 per case
- Baseline: the same commit with the per-connection walk, measured by stashing the change

`match` asked `ruleUsesMatchHelper(rule)` for every rule it visited, which unwraps rule wrappers and runs a
type switch to find out whether the rule needs the match helper. The answer never changes for a given
rule, so the walk repeated identical work on every connection.

The flag is now computed once per rule while the rule index is built, in the same loops that already
visit every position, and the match loop reads it from a parallel slice. The fallback keeps the direct
walk for positions outside an index, so a rule list published without an index behaves as before.

| Metric | Per-connection walk | Precomputed | Change |
| --- | ---: | ---: | ---: |
| Mixed 600-rule scan | 8.948 us/op | 7.667 us/op | -14.32% |
| Allocations | 4 | 4 | unchanged |

Verification:

```powershell
go test ./tunnel -count=1
go test -race ./tunnel -count=1
go vet ./tunnel
go test ./... -count=1 -p 2
```

The full main-module run passed.
