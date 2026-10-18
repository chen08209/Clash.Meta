# Phase 8 UDP Write-Back Store Report

Environment:

- Windows, `windows/amd64`
- CPU: Intel Core i9-9900 CPU @ 3.10GHz
- Go: `go1.27.1`
- Samples: 6 per case
- Baseline: commit `d35c609a`

`writeBackProxy` held its writer in `atomic.TypedValue[C.WriteBack]`, whose `Store` takes the address of the
interface value and pushes it to the heap. `packetSender.Process` calls `UpdateWriteBack` for every UDP
packet of a flow, so each packet paid a boxed store.

The proxy now stores the interface directly. Every packet of a flow carries a `*constant.PacketAdapter`,
so the concrete type never changes and `atomic.Value` is safe. Store and load measured at 10.3 ns with
zero allocations in the interface shape, and the latest write-back still wins.

| Metric | Baseline | Direct store | Change |
| --- | ---: | ---: | ---: |
| UpdateWriteBack | 21.22 ns/op | 8.06 ns/op | -62.02% |
| Bytes per packet | 16 B/op | 0 B/op | -100% |
| Allocations per packet | 1 | 0 | -100% |

## TCP data-plane survey (no change made)

A full-stack loopback benchmark (`HandleTCPConn` through the tunnel into a direct dial, 16 KiB chunks,
`Direct` mode) was profiled to look for the next hotspot: ~1.2 GB/s write and ~1.3 GB/s read at 0
allocations. The CPU profile is 87% `runtime.cgocall` inside `WSASend`/`WSARecv`, with every mihomo frame
under 2%; `sing`'s `bufio.Copy` is the only visible library frame. The TCP relay path is syscall-bound
on this platform and has no Go-level hotspot left to remove, so this round moved back to the UDP packet
path.

Verification:

```powershell
go test ./component/nat ./tunnel/... -count=1
go test -race ./component/nat ./tunnel -count=1
go vet ./component/nat ./tunnel
go test ./... -count=1 -p 2
```

The full main-module run passed.
