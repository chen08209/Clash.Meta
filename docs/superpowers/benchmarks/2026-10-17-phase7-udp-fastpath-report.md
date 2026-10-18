# Phase 7 UDP Admission Fast Path Report

Environment:

- Windows, `windows/amd64`
- CPU: Intel Core i9-9900 CPU @ 3.10GHz
- Go: `go1.27.1`
- Samples: 6 per case
- Baseline: commit `bfb4b8c5`

`handleUDPConn` ran the full `preHandleMetadata(metadata.Clone())` precheck for every packet of an
established flow, cloning the metadata and re-querying the resolvers each time. The clone result was
discarded, so for packets belonging to an existing flow the work bought nothing.

Admission now reads the flow table first. An established flow sends the packet straight to its sender,
and only a new flow pays the precheck clone. The flow-miss branch still runs the full creation path, so
first-packet semantics, sniffing, and drop-on-overload behaviour are unchanged.

Two additional per-packet allocations surfaced once the clone was gone and were removed with it:

- `fixMetadata` parsed the host unconditionally, and `netip.ParseAddr` allocates an error object when the
  host is not an IP literal, which includes the empty host every raw-IP flow carries.
- The dial closure captured the per-packet `metadata` variable, so escape analysis moved that variable to
  the heap on every packet. The slow path is now `startUDPSender`/`dialUDPSender` taking parameters.

| Metric | Baseline | Fast path | Change |
| --- | ---: | ---: | ---: |
| Established-flow packet | 395.20 ns/op | 96.92 ns/op | -75.47% |
| Bytes per packet | 152 B/op | 0 B/op | -100% |
| Allocations per packet | 5 | 0 | -100% |

Verification:

```powershell
go test ./tunnel/... ./component/nat/ -count=1
go test -race ./tunnel/ -count=1
go vet ./tunnel/ ./component/nat/
go test ./... -count=1 -p 2
```

The full main-module run passed.
