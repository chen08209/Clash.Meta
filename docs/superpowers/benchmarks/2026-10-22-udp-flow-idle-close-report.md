# Phase 45 UDP Flow Idle Close Report

Environment:

- Windows, `windows/amd64`
- Go: `go1.27.1`
- CPU: Intel Core i9-9900
- `go test -run '^$' -bench '^BenchmarkUDPFlowReplyRead$' -benchmem -count=3 ./tunnel`, four
  interleaved baseline/current rounds on the same host
- Comparison: `benchstat baseline.txt current.txt`

`adapter/outbound/base.go` wrapped every outbound packet conn that is not a `syscall.Conn` -
the sing-backed ones, so hysteria, tuic and shadowquic - in `deadline.NewEnhancePacketConn`
so the read deadline the tunnel sets on them still worked. That emulation ran a goroutine per
read: `WaitReadFrom` took the `resultCh` token, spawned a pipe reader, and blocked on the
result or the pipe deadline. `handleUDPToLocal` sets the deadline before each read and only
refreshes it once it is within half of `udpTimeout` (`60s`), so for a busy flow the pipe
branch was the steady state.

The flow now owns its idle expiry. `idlePacketConn` wraps the flow's outbound conn and turns
the deadline the tunnel sets into a timer that closes that conn, which is the one interruption
every outbound packet conn honours. The read loop unblocks on the closed conn instead of on an
expired deadline; it returns through the same error path and runs the same cleanup either way.
Both directions keep the half-timeout refresh gate, so the flow still closes between
`udpTimeout/2` and `udpTimeout` after its last packet.

| Flow reply read | Before | After | Change |
| --- | ---: | ---: | ---: |
| pooled read per datagram | 9.740µ ± 24% | 6.792µ ± 35% | -30.27% (p=0.005) |
| Bytes / allocations | 97 / 2 | 0 / 0 | -100% (p=0.000) |

The emulation itself is gone: `common/net/deadline/packet.go`, `packet_enhance.go`,
`packet_sing.go` and its test are deleted, the four `common/net` packet-deadline aliases go
with them, and `NewPacketConn` no longer branches on the conn type. The `net.Conn` half of the
deadline package (the deadline conn and the pipe) is untouched - only the packet emulation was
dead once the flow stopped asking a raw conn to honour a read deadline.

Verification:

```powershell
go vet ./tunnel/... ./common/net/... ./adapter/outbound/...
go test -count=1 ./tunnel ./common/net/... ./adapter/outbound/...
go test -race -count=1 ./tunnel ./common/net/... ./adapter/outbound/... ./listener/socks ./listener/shadowsocks ./listener/trojan ./listener/mieru ./listener/tunnel ./listener/tproxy
go build ./...
go test ./... -count=1
```
