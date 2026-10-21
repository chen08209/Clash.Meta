# Connection context teardown

Date: 2026-10-22

Environment: Windows, `windows/amd64`, Intel Core i9-9900, Go `1.27.1`.

## What changed

`constant.PlainContext.ID()` had no caller anywhere in the tree, but three
constructors computed it eagerly:

- `context.NewConnContext` ran on every accepted TCP connection,
- `context.NewDNSContext` ran on every DNS service query,
- `context.NewPacketConnContext` was never called at all.

`utils.NewUUIDV4()` measures 114.7-117.9 ns/op at 0 allocs/op on this host, so
both live paths paid a CSPRNG read for a value nothing read back.

The deprecated `tunnel.TCPIn` and `tunnel.UDPIn` fan-in queues were also
uncalled; they existed only to hand `handleTCPConn` a `C.ConnContext`, which is
what forced the per-connection wrapper to exist. With them gone, `handleTCPConn`
takes the accepted `net.Conn` and the `*C.Metadata` directly, and the redundant
mutable `conn` copy inside it disappears.

Removed: `constant/context.go`, `context/conn.go`, `context/packetconn.go`,
`context/conn_test.go` and the two fan-in queues - a net -167 lines.

## Gate

`TestHandleTCPConnEntryAllocatesNothing` pins the accepted-entry path with
`testing.AllocsPerRun`. It reports 0 allocations after the change and 1 on the
parent commit `53f04488c`, where it fails with `Should be zero, but was 1`.

## Verification

- `go test ./... -count=1 -p 2` green (136 s).
- `go test -race ./tunnel ./dns ./context/...` green.
- `go vet ./...` clean.
- Full tree cross-builds for linux amd64/arm64/386/arm/loong64/mips64le/riscv64/s390x/ppc64le,
  windows amd64/arm64/386, darwin amd64/arm64, freebsd amd64/386/arm64 and
  openbsd amd64. netbsd and dragonfly fail the same way before and after this
  change (`github.com/metacubex/wireguard-go/ipc` build constraints) and are
  not CI platforms.

`BenchmarkTCPConnSetupDirect` is dominated by the dial and was not stable
enough on this host to gate a one-allocation removal, so the deterministic
allocation gate above carries the evidence.
