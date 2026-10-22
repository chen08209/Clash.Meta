# Length fields without binary.Write

Date: 2026-10-22

Environment: Windows, `windows/amd64`, Intel Core i9-9900, Go `1.27.1`.

## What changed

`binary.Write` and `binary.Read` allocate a small slice for their fixed-size
fast path, so every length field written through them cost one allocation on
the packet path. Three sites were doing that per packet or per frame:

- `transport/vless` `PacketConn.WriteTo` wrote the two-byte length with
  `binary.Write` and then wrote the payload separately,
- `transport/trojan` `writePacket` wrote its length through `binary.Write` into
  an otherwise pooled buffer,
- `transport/v2ray-plugin` `Mux.Write` wrote both frame lengths with
  `binary.Write` into its own reusable buffer.

All three now write the length bytes directly. `vless.WriteTo` additionally
builds the header and the payload into one pooled buffer, so the stream sees a
single write instead of two.

## Gate

| benchmark | before | after |
| --- | ---: | ---: |
| vless `PacketConn.WriteTo` | 21.4-44.6 ns, 2 B, 1 alloc | 65.9-83.4 ns, 0 B, 0 allocs |
| trojan `writePacket` | 58.4-72.5 ns, 2 B, 1 alloc | 46.0-80.1 ns, 0 B, 0 allocs |
| v2ray-plugin `Mux.Write` | 65.6-76.1 ns, 4 B, 2 allocs | 37.4-47.0 ns, 0 B, 0 allocs |

The vless ns/op gets worse in this harness because its `Write` stub is free,
while the new path pays a 1350-byte copy into the pooled buffer. On a real
stream each `Write` is a syscall or a TLS record, and the new path issues one
instead of two, which the harness cannot show. The allocation drop is the part
the harness does measure, and `TestPacketConnWriteToFraming` pins both the
exact frame bytes and that the header and payload share one stream write.

`TestPacketConnWriteToAllocatesNothing`, `TestWritePacketAllocatesNothing` and
`TestMuxWriteAllocatesNothing` report 1, 1 and 2 allocations on the parent
commit. Their threshold is 0.5 so a `sync.Pool` refill after a GC cannot flake
them, and they carry `//go:build !race` because the race detector instruments
every access and makes `AllocsPerRun` meaningless.

## Verification

- `go test ./... -count=1 -p 2` green (122 s).
- `go test -race ./transport/vless ./transport/trojan ./transport/v2ray-plugin` green.
- `go vet ./...` clean.
- `go build -tags with_gvisor ./...` and the tagged tests green.
- Full tree cross-builds for linux amd64/arm64/386/arm, windows amd64/arm64/386,
  darwin amd64/arm64 and freebsd amd64.
