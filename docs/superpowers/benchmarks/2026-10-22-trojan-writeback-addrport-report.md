# Trojan UDP Write-Back From AddrPort

Date: 2026-10-22

Environment:

- Windows, `windows/amd64`
- CPU: Intel Core i9-9900 CPU @ 3.10GHz
- Go: `go1.27.1`
- Baseline: `999a6f0ec`
- Change: `a18015a0c`

## Change

`listener/trojan.packet.WriteBack` materialized a `net.UDPAddr`, then
`transport/trojan.PacketConn.WriteTo` parsed it back into a socks address,
allocating twice per reply. The listener now calls `WriteToAddrPort` directly,
and the transport writes the trojan packet header from the existing
`netip.AddrPort` through a stack buffer.

## Benchmark

Command:

```text
go test ./listener/trojan -run '^$' \
  -bench '^BenchmarkTrojanPacketWriteBack$' \
  -benchmem -benchtime=1s -count=10
```

`benchstat` result:

| Benchmark | Before | After | Change |
| --- | ---: | ---: | ---: |
| `TrojanPacketWriteBack` | 52.83 ns / 52 B / 2 allocs | 2.71 ns / 0 B / 0 allocs | -94.9% time, -100% bytes, -100% allocs |

## Verification

- `go test ./... -count=1 -p 2` (the unrelated VMess/TLSMirror interop
  subtest failed once under parallel load, then passed alone and with the
  entire `listener/inbound` package)
- `go test -race ./transport/trojan ./listener/trojan -count=1`
- `go vet ./...`
