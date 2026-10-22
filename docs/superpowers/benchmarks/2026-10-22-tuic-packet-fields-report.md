# TUIC v5: packet fields without binary.Write

Date: 2026-10-22

Environment: Windows, `windows/amd64`, Intel Core i9-9900, Go `1.27.1`.

## What changed

`v5.Packet` is the UDP datagram codec, so it runs for every packet a TUIC v5
session carries. Writing one packet went through `binary.Write` six times
(assoc id, packet id, fragment total, fragment id, size, address port) and
reading one through `binary.Read` six times, and each of those calls allocates
a scratch slice on its fixed-size fast path.

Both directions now use the byte methods the codec's own interfaces already
expose: `BufferedWriter` embeds `io.ByteWriter` and `BufferedReader` embeds
`io.ByteReader`, so `writeUint16`/`readUint16` write and read the two-byte
fields without producing a slice. `Dissociate` uses the same helpers.

The QUIC-relay push path also built a fresh `bufio.Writer` sized to the packet
for every datagram; it now serializes into a pooled buffer and writes that, the
same shape the other call sites in this package already use.

`transport/tuic/v4` has the same codec shape and was left alone in this change.

## Gate

| benchmark | before | after |
| --- | ---: | ---: |
| `BenchmarkPacketWriteTo` | 10 B, 6 allocs, 148.5-278.0 ns | 0 B, 0 allocs, 53.1-59.7 ns |
| `BenchmarkReadPacket` | 1424 B, 8 allocs, 428.3-887.0 ns | 1412 B, 2 allocs, 304.4-398.8 ns |

The two remaining read-side allocations are the address and the payload the
codec has to copy out of the stream.

`TestPacketWriteToAllocatesNothing` and
`TestReadPacketAllocatesPayloadAndAddressOnly` pin both; on the parent commit
they fail with `"6" is not less than or equal to "0.5"` and `"8" is not less
than or equal to "2.5"`. Both carry `//go:build !race` because the race
detector instruments every access and makes `AllocsPerRun` meaningless.

## Verification

- `go test ./... -count=1 -p 2` green (152 s).
- `go test -race ./transport/tuic/...` green.
- `go vet ./...` clean.
- `go build -tags with_gvisor ./...` and the tagged test green.
- Full tree cross-builds for linux amd64/arm64/386/arm, windows amd64/arm64/386,
  darwin amd64/arm64 and freebsd amd64.
