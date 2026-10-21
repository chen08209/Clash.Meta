# Phase 45 Deadline Pipe Read Cost Report (target recorded, not landed)

Environment:

- Windows, `windows/amd64`
- Go: `go1.27.1`
- CPU: Intel Core i9-9900
- `go test -run '^$' -bench 'DeadlineWrapper' -benchmem -count=5 .` against a scratch module
  that wraps `packet.NewEnhancePacketConn(*net.UDPConn)` in `deadline.NewEnhancePacketConn`
  exactly as `adapter/outbound/base.go` does

`adapter/outbound/base.go` wraps every outbound packet conn that is not a `syscall.Conn` - the
sing-backed ones, so hysteria, tuic and shadowquic - in `deadline.NewEnhancePacketConn` so the
read deadline the tunnel sets on them still works. The wrapper emulates that deadline with a
goroutine per read: `WaitReadFrom` takes the `resultCh` token, spawns `pipeWaitReadFrom`, and
blocks on the result or the pipe deadline.

`handleUDPToLocal` sets that deadline before the read and only refreshes it once it is within
half of `udpTimeout` (`60s`), so after the first datagram the deadline is set for 30-60s at a
time. For a busy flow the pipe branch is therefore the steady state, not the fallback.

| Read | Time | Allocations |
| --- | ---: | ---: |
| deadline unset (direct path) | 4.98-9.41us | 0 B / 0 allocs |
| deadline set (pipe path) | 12.39-17.65us | 96-97 B / 2 allocs |

The two objects are the `enhanceReadResult` and the goroutine that carries the read; the extra
time is the channel handoff on top of the read itself.

The fix is not mechanical. `resultCh` is a one-token handshake shared by four read shapes
(`ReadFrom`, `WaitReadFrom`, `ReadPacket`, `WaitReadPacket`), and each shape owns its own
result type plus the `nil` token the consumer returns. Making the pipe reader persistent -
start it once per conn instead of once per read - means replacing that handshake, and a wrong
handshake hangs the tunnel UDP read loop rather than slowing it. It is left as the next
target with the numbers above; the direct path already costs 0 allocations once the deadline
is unset.
