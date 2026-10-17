# Phase 5 Packet Mapping Snapshot Report

Environment:

- Windows, `windows/amd64`
- CPU: Intel Core i9-9900 CPU @ 3.10GHz
- Go: `go1.27.1`
- Samples: 6 per case
- Baseline: commit `ec2dced6`

`packetSender` previously protected both destination maps with an `RWMutex`. Every packet paid a read-lock round trip for the destination lookup, and incoming responses paid another for the reverse mapping.

The two maps are now published as one immutable `packetMappings` snapshot behind an atomic pointer. Writers serialize with a mutex, clone the current maps, and publish a new snapshot atomically. Readers load the snapshot once and never take a lock.

| Metric | Baseline | Snapshot | Change |
| --- | ---: | ---: | ---: |
| RestoreReadFrom | 25.16 ns/op | 15.98 ns/op | -36.47% |
| Origin lookup | 36.71 ns/op | 17.72 ns/op | -51.72% |
| Allocations | 1 to 0 | 0 | already zero after the binary-key change |

The write side now clones both maps and publishes a new snapshot, so the first mapping in a fresh sender costs about `915 ns/op`, `3776 B/op`, and `13 allocs/op`. That is accepted because mappings are created once per destination while packet reads are per packet.

Verification:

```powershell
go test ./tunnel -run 'TestPacketOriginKey|TestPacketMapping' -count=1
go test ./tunnel -count=1
go test -race ./tunnel -count=1
go test ./...
```

The full main-module run passed; the slowest existing package was `listener/inbound` at 79.636 seconds.
