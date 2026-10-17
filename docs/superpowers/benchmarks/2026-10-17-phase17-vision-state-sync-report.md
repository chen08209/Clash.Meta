# Phase 17 Vision State Synchronisation Report

Environment:

- Windows, `windows/amd64`
- Go: `go1.27.1`
- Evidence: `go test -race ./listener/inbound` before and after, plus a micro-benchmark of the new instructions

`go test -race ./listener/inbound` reported data races between the Vision read path and the write path:
`ReadBuffer` wrote `readLastCommand`, `readFilterUUID` and the TLS-filter state while `Upstream`,
`FrontHeadroom` and `FilterTLS` on the write path read them, with no ordering between the goroutines.
The state is shared by design, because the filter budget and the TLS handshake discovery serve both
directions of one connection.

The state that crosses goroutines is now atomic: `readProcess`, `readFilterUUID`, `readLastCommand`,
`writeDirect`, `packetsToFilter`, `isTLS`, `isTLS12orAbove`, `remainingServerHello`, `cipher` and
`enableXTLS`. The state only the read path touches stays plain, so the per-packet read work is unchanged.

Measured cost of the conversion on amd64, where an atomic load compiles to the same instruction as a
plain load:

| Operation | Cost |
| --- | ---: |
| `atomic.Int32.Load` (the steady-state per-packet case) | 0.46 ns, same as a plain read |
| `atomic.Int32.Add(-1)` (the filter budget) | ~10 ns, at most 8 times per connection |

So the worst case is roughly 80 ns once per connection, while the filter budget is consumed in the first
few packets, and zero afterwards; the steady-state cost is unchanged.

Verification:

```powershell
go test ./transport/vless/... -count=1
go test -race ./listener/inbound -count=1
go test ./... -count=1 -p 2
```

After this change no `vless.vision` frame remains in the race report; the frames that remain belong to
`transport/anytls/session`, a separate pre-existing race that reproduces on the unmodified tree.
