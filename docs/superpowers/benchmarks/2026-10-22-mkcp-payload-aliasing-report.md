# mKCP receive path: stop copying segment payloads twice

Date: 2026-10-22

Environment: Windows, `windows/amd64`, Intel Core i9-9900, Go `1.27.1`.

## What changed

Both receive loops already hand `packetReader.read` a fresh copy of the datagram
(`append([]byte(nil), buf[:n]...)`), because the read buffer is reused
immediately by the next `ReadFrom`/`Read`. `readSegment` then copied every data
payload a second time, so a 1350-byte datagram paid two payload-sized copies.

`readSegment` now slices the payload out of the buffer the caller already owns.
The ownership requirement is documented on `readSegment` and
`packetReader.read`, and it holds for both call sites:

- the security wrapper decrypts in place before `readSegment` runs,
- nothing writes to the datagram after parsing,
- consumers only read the payload (`Conn.deliverLocked` appends it into the
  connection read buffer), and `recvCache` keeps the segment itself alive, so
  the aliased buffer stays reachable for as long as it is used.

## Gate

`BenchmarkReadDataSegment` (1350-byte payload, `windows/amd64`, n=6):

| | ns/op | B/op | allocs/op |
| --- | ---: | ---: | ---: |
| before | 250.8-270.1 | 1472 | 3 |
| after | 57.9-129.9 | 64 | 2 |

`TestReadSegmentDoesNotCopyTheDataPayload` pins the result with
`testing.AllocsPerRun`; on the parent commit it fails with
`"3" is not less than or equal to "2"`.

The ACK path is unchanged at 80 B / 3 allocs: its `numberList` is still built
from the wire bytes, which is the next candidate in this package.

## Verification

- `go test ./... -count=1 -p 2` green (289 s under other load on this host).
- `go test -race ./transport/mkcp ./tunnel ./dns` green.
- `go vet ./...` clean.
- `go build -tags with_gvisor ./...` and `go test -tags with_gvisor ./transport/mkcp` green.
- `./transport/... ./tunnel/... ./common/...` cross-builds for linux
  amd64/arm64/386/riscv64/mips64le, windows amd64/arm64/386, darwin amd64/arm64
  and freebsd amd64/arm64.
