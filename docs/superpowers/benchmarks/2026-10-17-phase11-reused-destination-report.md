# Phase 11 Reused UDP Send Destination Report

Environment:

- Windows, `windows/amd64`
- CPU: Intel Core i9-9900 CPU @ 3.10GHz
- Go: `go1.27.1`
- Samples: 6 per case

Every packet on a mapped flow built a fresh `*net.UDPAddr` for the write destination: the struct plus
the `AsSlice` copy of the IP, two allocations and 52 bytes per packet, measured at 94 ns with the
write itself excluded.

A flow usually keeps sending to one destination, so the sender now caches the address object and
reuses it while the `netip.AddrPort` repeats. Reuse is only armed when the conn is known to consume
the address inside `WriteTo`: `synchronousWriteTarget` walks `Upstream()` to a kernel `*net.UDPConn`,
which is true exactly for the direct and socket-based proxy outbounds. Protocol outbounds queue the
address (`adapter/outbound/dns.go` hands the pointer to a channel consumed later), and they keep
receiving a fresh object. The walk runs once per sender on its first mapped packet.

| Metric | Fresh per packet | Reused | Change |
| --- | ---: | ---: | ---: |
| processPacket (hit path) | 94.07 ns/op | 39.96 ns/op | -57.52% |
| Bytes per packet | 52 B/op | 0 B/op | -100% |
| Allocations per packet | 2 | 0 | -100% |

A nil `*net.UDPConn` reaching the walk still counts as kernel-backed unless the pointer itself is
checked, because a typed nil satisfies the assertion; the guard is covered by the queued-conn test.

Verification:

```powershell
go test ./tunnel -count=1
go test -race ./tunnel -count=1
go vet ./tunnel
go test ./... -count=1 -p 2
```

The full main-module run passed.
