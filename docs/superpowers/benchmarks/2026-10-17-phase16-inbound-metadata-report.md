# Phase 16 Inbound Metadata Allocations Report

Environment:

- Windows, `windows/amd64`
- Go: `go1.27.1`
- Measured with `testing.AllocsPerRun` on `adapter/inbound.NewPacket`

The inbound layer built metadata for every packet and every connection. Two of the three allocations in
`NewPacket` bought nothing:

- `metadata.RawDstAddr = metadata.UDPAddr()` formatted a `*net.UDPAddr` per packet for a field that no
  code in the repository reads and that JSON excludes (`json:"-"`). The sing listener paid two more per
  packet for the same pair of fields.
- `WithSrcAddr(...)` and `WithInAddr(...)` allocated a closure per packet, and the sing paths allocated
  three per packet, only to set a handful of fields immediately.

`SetSrcAddr`/`SetInAddr` apply the same fields without a closure, and the `With*` helpers now delegate
to them so behaviour cannot drift. The raw-address assignments are gone from every constructor.

| Metric | Before | After | Change |
| --- | ---: | ---: | ---: |
| Allocations per UDP inbound packet (NewPacket) | 3 | 1 | -66.7% |
| Closures per UDP packet (sing path) | 3 | 0 | -100% |
| Closures per TCP connection (http/https) | 2 | 0 | -100% |

The remaining allocation is the `*C.Metadata` the packet carries, which is unavoidable.

Verification:

```powershell
go test ./adapter/inbound ./listener/... ./tunnel -count=1
go vet ./adapter/inbound ./listener/sing
go test ./... -count=1 -p 2
```

`go test -race ./listener/inbound` reports a data race in `transport/vless/vision` (an `Upstream()` read
against a `ReadBuffer()` write) that reproduces on the unmodified tree, so it is pre-existing and outside
this change; CI does not run `-race`.
