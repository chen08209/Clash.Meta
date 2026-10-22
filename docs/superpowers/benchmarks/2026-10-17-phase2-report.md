# Phase 2 Benchmark Report

Environment:

- Windows, `windows/amd64`
- CPU: Intel Core i9-9900 CPU @ 3.10GHz
- Go: `go1.27.1`
- Toolchains: `D:\Codex\tools\go-1.27.1`, MinGW 16.2.0 for race checks
- Module proxy: `https://goproxy.cn,direct`

Commands:

```powershell
$env:GOENV='off'
$env:GOPROXY='https://goproxy.cn,direct'
& 'D:\Codex\tools\go-1.27.1\bin\go.exe' test -run '^$' -bench '^(BenchmarkResolveMetadata|BenchmarkTracker)' -benchmem ./tunnel ./tunnel/statistic
& 'D:\Codex\tools\go-1.27.1\bin\go.exe' test -run '^$' -bench '^BenchmarkPacketFlowKey$' -benchmem ./tunnel
& 'D:\Codex\tools\go-1.27.1\bin\go.exe' test -run '^$' -bench '^BenchmarkTableGetOrCreateForFlowHit$' -benchmem ./component/nat
& 'D:\Codex\tools\go-1.27.1\bin\go.exe' test -run '^$' -bench '^BenchmarkGetProxiesCacheHit$' -benchmem ./adapter/outboundgroup
```

The temporary end-to-end module used the same command as `Benchmark_Direct` with `SKIP_E2E_IMAGES=1`; it replaced `github.com/metacubex/mihomo` with either this checkout or the clean baseline worktree.

Phase 2 results:

- `ResolveMetadata`: 250.0 ns/op, 416 B/op, 1 alloc/op
- `ResolveMetadataFallback`: 373.7 ns/op, 448 B/op, 3 allocs/op
- `TrackerJoinLeave`: 109.6 ns/op, 32 B/op, 1 alloc/op
- `TrackerWrite`: 2476 ns/op, 6618.37 MB/s, 0 allocs/op
- `TrackerRead`: 1618 ns/op, 10127.38 MB/s, 0 allocs/op
- `PacketFlowKey`: 3.101 ns/op, 0 B/op, 0 allocs/op
- `TableGetOrCreateForFlowHit`: 24.76 ns/op, 0 B/op, 0 allocs/op
- `GetProxiesCacheHit`: 41.18 ns/op, 0 B/op, 0 allocs/op

Direct end-to-end, three runs:

- Current Write: 4514.60, 3994.25, and 4645.05 MB/s; 0 allocs/op
- Baseline Write: 2308.83, 2317.23, and 2298.30 MB/s; 0 allocs/op
- Current Read: 2863.44, 1786.32, and 3222.41 MB/s; 0 allocs/op
- Baseline Read: 2501.41, 2477.89, and 2519.74 MB/s; 0 allocs/op

Shadowsocks, VMess, and Trojan interop benchmarks were not run because Docker Desktop is not running on this host. Those remain a release-gate gap rather than an inferred result.
