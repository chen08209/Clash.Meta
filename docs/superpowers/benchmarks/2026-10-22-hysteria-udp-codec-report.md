# Hysteria UDP: direct codec and structured peers

Date: 2026-10-22

Environment: Windows, `windows/amd64`, Intel Core i9-9900, Go `1.27.1`.

## What changed

The hysteria UDP path built and parsed an 18-byte header per datagram, and both
steps went the long way round.

`udpMessage.Pack` allocated a `bytes.Buffer` and called `binary.Write` five
times; `Unpack` did the mirror image with `bytes.Reader` and `binary.Read`. A
single datagram therefore cost 9 allocations to build and 10 to parse. Both now
write and read their fields with `binary.BigEndian` straight into the buffer.

`core.UDPConn` then took and returned the peer as a formatted `host:port`
string, so the same address was rendered on one side and parsed back on the
other: `SocksaddrFromNet(addr).String()` + `SplitHostPort` on the way out,
`JoinHostPort` + `ParseSocksaddr` on the way in. The interface now carries
`M.Socksaddr`, which both sides already had in hand.

## Gate

`BenchmarkUDPMessagePack` / `BenchmarkUDPMessageUnpack` (1350-byte payload,
`windows/amd64`, n=6):

| | ns/op | B/op | allocs/op |
| --- | ---: | ---: | ---: |
| Pack before | 371.4-383.0 | 1472 | 9 |
| Pack after | 231.6-298.2 | 1408 | 1 |
| Unpack before | 275.2-460.2 | 96 | 10 |
| Unpack after | 21.9-23.9 | 16 | 1 |

`BenchmarkHyPacketConnWriteTo` / `BenchmarkHyPacketConnReadFrom` /
`BenchmarkQuicPktConnReadFrom`:

| | ns/op before | ns/op after | allocs before | allocs after |
| --- | ---: | ---: | ---: | ---: |
| `hyPacketConn.WriteTo` | 95.4-243.0 | 18.3-19.1 | 3 | 0 |
| `hyPacketConn.ReadFrom` | 123.6-281.8 | 60.9-108.3 | 2 | 2 |
| `quicPktConn.ReadFrom` | 92.0-243.3 | 64.8-69.7 | 2 | 0 |

The two remaining `hyPacketConn.ReadFrom` allocations are the `*net.UDPAddr`
the `net.PacketConn` interface requires.

`TestUDPMessageWireFormat` pins the exact bytes the hysteria server reads, so
the codec rewrite cannot drift from the wire format;
`TestUDPMessageCodecAllocatesOnce` and
`TestHyPacketConnWriteToAllocatesNothing` pin the allocation counts, and
`TestHyPacketConnReturnsStructuredAddresses` pins the v4/v6 round trip.

## Verification

- `go test ./... -count=1 -p 2` green (127 s).
- `go test -race ./transport/hysteria/core ./adapter/outbound` green.
- `go vet ./...` clean.
- `go build -tags with_gvisor ./...` and the tagged tests green.
- Full tree cross-builds for linux amd64/arm64/386/arm, windows amd64/arm64/386,
  darwin amd64/arm64 and freebsd amd64.
