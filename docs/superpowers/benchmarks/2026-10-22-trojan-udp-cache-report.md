# Trojan UDP Flow Metadata Cache

Date: 2026-10-22

Environment:

- Windows, `windows/amd64`
- CPU: Intel Core i9-9900 CPU @ 3.10GHz
- Go: `go1.27.1`
- Baseline: `da7b767f6`
- Change: `390d17525`

## Change

The wrapper pool was already covered by `99c4773b`. On top of it, the trojan
UDP loop still created a fresh `C.Metadata` for every datagram, re-parsed the
connection's fixed remote source each time, and asked `inbound.NewPacket` to
rebuild the same target metadata repeatedly.

The loop now keeps one `trojanUDPSession` per connection, reuses metadata by
destination address, and hoists the source address out of the loop. The
existing wrapper release through `Drop` is unchanged. The per-flow cache is
bounded at 256 targets and cleared when saturated.

## Benchmark

Command:

```text
go test ./listener/trojan -run '^$' \
  -bench '^BenchmarkTrojanUDPHandle$' \
  -benchmem -benchtime=1s -count=10
```

`benchstat` result:

| Benchmark | Before | After | Change |
| --- | ---: | ---: | ---: |
| `TrojanUDPHandle` | 506 B / 4 allocs | 22 B / 0 allocs | -95.7% bytes, -100% allocs |

The benchmark uses a pooled data buffer, so the remaining fractional bytes
come from pool refill/map growth rather than the per-datagram path. The host
was loaded during the baseline sample, so the wall-clock delta is not used as
the gate; allocations are exactly stable across all samples.

## Verification

- `go test ./... -count=1 -p 2`
- `go test -race ./listener/trojan -count=1`
- `go vet ./...`
