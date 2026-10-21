# Sing UDP Write-Back Address Reuse

Date: 2026-10-22

Environment:

- Windows, `windows/amd64`
- CPU: Intel Core i9-9900 CPU @ 3.10GHz
- Go: `go1.27.1`
- Baseline: `16784bcf4`
- Change: `2d13dbb7a`

## Change

The sing packet writer still materialized a `net.UDPAddr` for every reply
through `net.UDPAddrFromAddrPort`. The per-flow writer state now owns one
`net.UDPAddr` and a 16-byte IP buffer, copies the current `AddrPort` into it
while holding the existing write mutex, and passes that address across the
single sing `net.Addr` boundary.

## Benchmark

Command:

```text
go test ./listener/sing -run '^$' \
  -bench '^BenchmarkSingPacketWriteBack$' \
  -benchmem -benchtime=1s -count=10
```

`benchstat` result:

| Benchmark | Before | After | Change |
| --- | ---: | ---: | ---: |
| `SingPacketWriteBack` | 47.01 ns / 52 B / 2 allocs | 17.54 ns / 0 B / 0 allocs | -62.7% time, -100% bytes, -100% allocs |

## Verification

- `go test ./... -count=1 -p 2`
- `go test -race ./listener/sing ./transport/trojan ./listener/trojan -count=1`
- `go vet ./...`
