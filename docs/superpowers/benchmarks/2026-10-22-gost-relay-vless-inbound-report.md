# gost relay guard and the vless inbound frame

Date: 2026-10-22

Environment: Windows, `windows/amd64`, Intel Core i9-9900, Go `1.27.1`.

## What changed

Two more per-packet sites carried the same shapes the earlier rounds removed.

`transport/gost` `relayPacketConn.WriteTo` checks that the packet still belongs
to the bound association and did that with `addr.String() != c.raddr.String()`,
so both addresses were rendered through `IP.String`, `strconv.Itoa` and
`net.JoinHostPort` on every datagram. It then wrote the two-byte length from a
stack array through the `net.Conn` interface, which escapes, and wrote the
payload separately. The guard now compares `netip.AddrPort` values, with the
string form kept as the fallback for addresses `AddrPort` cannot express, and
the length and payload share one pooled write.

`listener/sing_vless` `serverPacketConn.WriteTo` had the same length-field
shape as the outbound `transport/vless` path that was fixed earlier:
`binary.Write` for the two-byte header plus a second write for the payload. It
now uses the same single pooled write.

## Gate

| benchmark | before | after |
| --- | ---: | ---: |
| gost `relayPacketConn.WriteTo` | 193.9-354.7 ns, 64 B, 7 allocs | 54.9-58.4 ns, 0 B, 0 allocs |
| vless inbound `serverPacketConn.WriteTo` | 20.5-42.5 ns, 2 B, 1 alloc | 33.6-35.7 ns, 0 B, 0 allocs |

The vless inbound ns/op rises in this harness for the same reason as the
outbound one: its `Write` stub is free, so the harness sees the added copy and
not the stream write the change saves. `TestServerPacketConnWriteToFraming`
pins the frame bytes and that the length and payload share one write.

`TestRelayPacketConnWriteToAllocatesNothing` and
`TestServerPacketConnWriteToAllocatesNothing` report 7 and 1 allocations on the
parent commit `1a927214e`; both threshold at 0.5 so a `sync.Pool` refill after
a GC cannot flake them, and both carry `//go:build !race` because the race
detector instruments every access and makes `AllocsPerRun` meaningless.
`TestRelayPacketConnWriteToAssociationGuard` pins the accept/reject table,
including the same destination given as 4-byte and 16-byte v4.

## Verification

- `go test ./... -count=1 -p 2` green (129 s).
- `go test -race ./listener/sing_vless ./transport/gost` green.
- `go vet ./...` clean.
- `go build -tags with_gvisor ./...` and the tagged tests green.
- Full tree cross-builds for linux amd64/arm64/386/arm, windows amd64/arm64/386,
  darwin amd64/arm64 and freebsd amd64.
