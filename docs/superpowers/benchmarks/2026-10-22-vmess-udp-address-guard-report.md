# vmess UDP: compare the destination without formatting it

Date: 2026-10-22

Environment: Windows, `windows/amd64`, Intel Core i9-9900, Go `1.27.1`.

## What changed

`vmessPacketConn.WriteTo` guards every outgoing UDP packet against a
full-cone-NAT mismatch, and it did that by formatting both addresses:

```
if uc.rAddr.String() != addr.String() { return 0, ErrUDPRemoteAddrMismatch }
```

`net.UDPAddr.String` builds `host:port` through `IP.String`, `strconv.Itoa` and
`net.JoinHostPort`, so the guard cost six allocations and about half a
microsecond on every datagram, for a comparison that a single struct compare
answers.

`samePacketAddr` now compares the destinations as `netip.AddrPort` values. The
string form stays as the fallback for addresses `AddrPort` cannot express, such
as the same v4 host supplied once as 4 and once as 16 raw bytes, so the guard
keeps the exact accept/reject behaviour of the previous implementation. The
match table covers v4, v4-in-v6, v6, port, host and family mismatches.

## Gate

`BenchmarkVmessPacketWriteTo` (1350-byte payload, `windows/amd64`, n=6):

| | ns/op | B/op | allocs/op |
| --- | ---: | ---: | ---: |
| before | 463.8-759.0 | 64 | 6 |
| after | 44.2-47.9 | 0 | 0 |

`TestVmessPacketWriteToAllocatesNothing` pins it with `testing.AllocsPerRun`;
on the parent commit it fails with `Should be zero, but was 6`.

## Verification

- `go test ./... -count=1 -p 2` green (143 s).
- `go test -race ./adapter/outbound` green.
- `go vet ./...` clean.
- `go build -tags with_gvisor ./...` and `go test -tags with_gvisor ./adapter/outbound` green.
- Full tree cross-builds for linux amd64/arm64/386/arm, windows amd64/arm64/386,
  darwin amd64/arm64 and freebsd amd64.
