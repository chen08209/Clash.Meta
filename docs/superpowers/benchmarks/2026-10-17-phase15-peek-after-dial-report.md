# Phase 15 Peek After Dial Report

Environment:

- Windows, `windows/amd64`
- CPU: Intel Core i9-9900 CPU @ 3.10GHz
- Go: `go1.27.1`
- Samples: 6 per case
- Baseline: the same commit with the pre-dial peek goroutine, measured by stashing the change

Every TCP connection that had not already been peeked by the sniffer spawned a goroutine, armed a
200 ms read deadline, and peeked one byte, all before the dial, so the goroutine's deadline sat on the
shared connection while the relay started. Only a conn that needs a handshake consumes the peeked bytes;
a plain relay waits on its own first read anyway.

The buffer is now filled inside the handshake branch, after the remote conn exists: the same one-second
bounded peek, the same write of the buffered bytes to the remote conn, the same discard. The pre-dial
goroutine, its mutex, and the deadline reset after the dial are gone.

| Metric | Pre-dial goroutine | Post-dial peek | Change |
| --- | ---: | ---: | ---: |
| Allocations per connection | 214 | 212 | -0.93% |
| Bytes per connection | 49.85 KiB | 49.56 KiB | ~ |
| Setup time | 725.9 us | 731.7 us | ~ |

The time and byte totals are unchanged within noise because loopback connects and the standard library
socket machinery dominate a connection setup; the two saved allocations are the goroutine's closure and
its argument frame, and the goroutine itself is scheduler and GC work the benchmark does not price in.

Verification:

```powershell
go test ./tunnel -count=1
go test -race ./tunnel -count=1
go vet ./tunnel
go test ./... -count=1 -p 2
```

`TestDirectConnSurvivesLateFirstByte` drives a direct connection whose first byte arrives at 300 ms with
the old peek deadline at 200 ms, and `TestHandshakeConnReceivesClientHello` asserts a handshake conn is
still handed the client's first bytes through the new path.
