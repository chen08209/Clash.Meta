# Phase 9 UDP Deadline Refresh Report

Environment:

- Windows, `windows/amd64`
- CPU: Intel Core i9-9900 CPU @ 3.10GHz
- Go: `go1.27.1`
- Samples: 6 per case

Every UDP packet reset the session's read deadline twice: once in the sender loop before writing to the
remote and once in the return-path loop before each `WaitReadFrom`. Measured at 52.2 ns per reset on this
platform (`SetReadDeadline` 44.6 ns plus `time.Now`), a busy flow paid it per packet to push a deadline
that was already 60 seconds out.

Both directions now arm the deadline only once the current one is more than half consumed. The decision
is a single integer comparison against a tracked deadline value, so the per-packet cost collapses to the
`time.Now` call itself. The read loop tracks its deadline in a local variable and the sender loop in a
field only its own goroutine touches, so neither needs synchronisation, and a wrapped sniffer sender
still works because the state never crosses the `C.PacketSender` interface.

| Metric | Per packet | Change |
| --- | ---: | ---: |
| Unconditional reset | 52.21 ns/op | baseline |
| Gated decision | 5.80 ns/op | -88.89% |

A flow now closes between `udpTimeout/2` and `udpTimeout` after its last packet instead of always at the
full timeout: a packet arriving after the half mark re-arms the deadline, and one arriving before it
rides a deadline that still has more than half the timeout left. The minimum observed idle lifetime is
30 seconds.

Verification:

```powershell
go test ./tunnel -count=1
go test -race ./tunnel -count=1
go vet ./tunnel
go test ./... -count=1 -p 2
```

The full main-module run passed. `TestProcessPacketArmsDeadlineOncePerHalfTimeout` drives the real
`processPacket` path with a recording `PacketConn` and asserts the deadline is armed exactly once across
two back-to-back packets.
