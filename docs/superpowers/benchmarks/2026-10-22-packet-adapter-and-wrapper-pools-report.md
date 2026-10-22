# Packet Adapter and UDP Wrapper Pools Report

Date: 2026-10-22

Environment: Windows, `windows/amd64`, Intel Core i9-9900, Go `1.27.1`.

## Generic packet adapter

Every datagram entering the tunnel was wrapped by a fresh `constant.packetAdapter`.
The wrapper has no payload ownership of its own, so it is now returned through the
existing `Drop` release point. `Drop` still calls the wrapped packet exactly once and
clears the adapter before pooling it.

`TestNewPacketAdapterWithFlowKeyAllocatesOnlyTheAdapter` changed from requiring the
one warm allocation to requiring zero. `BenchmarkPacketAdapterLifecycle` measures the
full create/drop cycle at 27.7-46.6 ns / 0 B / 0 allocs.

## Snell UDP

The server rebuilt metadata, copied the payload with `append`, and allocated a packet
wrapper for every datagram. It now caches metadata per destination, uses the binary
flow key from the TCP peer, takes payload memory from the mihomo allocator, and returns
the wrapper through `Drop`.

`TestSnellUDPFlowCacheReusesMetadataAndWrappers` pins metadata identity and wrapper
cleanup. `TestSnellUDPHandleIsAllocationFree` shows a warm datagram costs 0 B / 0 allocs;
`BenchmarkSnellUDPHandle` measured 65.4-149.8 ns / 0 B / 0 allocs on a noisy host.
The `clientID` remains in `Metadata.InUser` and is pinned by the flow-cache test.

## Hysteria UDP hop wrapper

The port-hopping connection already pooled its 64 KiB receive buffers, but queued a
fresh `udpPacket` wrapper each time. The wrapper now shares the buffer pool lifecycle
and is returned when the reader copies the datagram or when the queue is full.

`BenchmarkUDPPacketOwnedLifecycle` models the escaping queue wrapper and measured
32.46-35.47 ns / 32 B / 1 alloc. `BenchmarkUDPPacketLifecycle` measured 21.04-23.29 ns /
0 B / 0 allocs.

## Verification

```powershell
go test ./... -count=1 -p 2
go vet ./...
go test -race -count=1 ./constant ./listener/snell ./listener/trojan ./listener/sing ./tunnel ./transport/hysteria/conns/udp
```

Full tests were green. The full-tree `staticcheck` run reports only the existing
upstream findings; none point at the changed files.
