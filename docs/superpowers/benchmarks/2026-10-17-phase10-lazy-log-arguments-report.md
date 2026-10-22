# Phase 10 Lazy Log Arguments Report

Environment:

- Windows, `windows/amd64`
- CPU: Intel Core i9-9900 CPU @ 3.10GHz
- Go: `go1.27.1`
- Samples: 6 per case

Task 2 put level gates inside the log functions, which stops the channel send and formatting, but the
call site's arguments are still evaluated first. On hot paths that construction was the expensive part:

- The DNS cache-hit line built `msgToLogString(msg)` (an `Sprintf` that re-parses every answer record
  through `msgToIP`) plus a `Format`ted expiry timestamp on every cached resolution, even though the
  default `info` level means the line is discarded.
- `batchExchange` built the domain and qtype strings per upstream exchange purely for two debug lines.
- `logMetadata` built four strings per TCP connection, including an inner `Sprintf` for the matched
  rule, and now returns early when info events would be dropped anyway.
- The tun DNS hijack path built `M.Metadata.String()` per hijacked query.

The gates are the new `log.DebugEnabled`/`log.InfoEnabled` helpers. Log output is unchanged when the
level permits it; only the argument construction becomes conditional.

| Metric | Unconditional | Gated | Change |
| --- | ---: | ---: | ---: |
| DNS cache-hit arguments | 1007.50 ns/op | 0.43 ns/op | -99.96% |
| DNS cache-hit allocations | 15 allocs, 520 B | 0 | -100% |
| DNS exchange arguments | 18.53 ns/op | 0.43 ns/op | -97.69% |

Verification:

```powershell
go test ./dns ./tunnel ./log -count=1
go vet ./dns ./tunnel ./listener/sing_tun ./log
go test ./... -count=1 -p 2
```

The full main-module run passed.
