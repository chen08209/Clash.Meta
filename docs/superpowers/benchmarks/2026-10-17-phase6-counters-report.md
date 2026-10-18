# Phase 6 Traffic Counter Sharding Report

Environment:

- Windows, `windows/amd64`
- CPU: Intel Core i9-9900 CPU @ 3.10GHz
- Go: `go1.27.1`
- Samples: 6 per case
- Baseline: commit `fe482a3e`

`Manager` kept its twelve traffic counters as twelve adjacent `atomic.Int64` fields, so every connection's upload and download accounting serialised through the same cache lines. A proxied read or write performed four contended atomic adds, and the ticker's swap touched the same lines again.

Counters now live in `counterStripe`, a 128-byte unit that isolates the upload and download groups on separate line pairs. Each tracker binds one stripe for its lifetime with a round-robin cursor, so accounting lands on a different line per connection. The manager's public push API keeps its old shape and routes through one shared stripe, since nothing on a per-packet path uses it any more. Totals are summed over stripes; the one-second tick drains every stripe with an atomic swap instead of a shared swap, so no add racing with the tick is lost.

`atomic.Int64` is 8 bytes with 8-byte alignment on this target, so the padding is what actually separates the stripes; the 32-stripe array is a single 4 KiB object the Go allocator aligns to a 4096-byte boundary.

| Metric | Baseline | Sharded | Change |
| --- | ---: | ---: | ---: |
| PushDownloaded (proxied, parallel) | 44.54 ns/op | 2.185 ns/op | -95.09% |
| Allocations | 0 | 0 | unchanged |

The stripeless `Manager{}` used by existing tests binds a nil stripe and the push becomes a no-op, so zero-value managers stay safe.

Verification:

```powershell
go test ./tunnel/statistic -count=1
go test -race ./tunnel/statistic -count=1
go test -race ./tunnel -count=1
go vet ./tunnel/statistic
go test ./... -count=1
```

The full main-module run passed with `go test ./...`; the sudden `context deadline exceeded` in `transport/sudoku` on the first parallel run was a load flake and passes in isolation on both this commit and the baseline worktree.
