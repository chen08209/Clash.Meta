# Phase 31 Windows UDP Read Syscall Report

Environment:

- Windows, `windows/amd64`
- Go: `go1.27.1`
- CPU: Intel Core i9-9900
- `go test -bench '^BenchmarkWaitReadFrom$' -benchmem -count=6 ./common/net/packet`
- Comparison: `benchstat baseline.txt current.txt`

The Windows UDP read path still used `windows.Recvfrom`, whose wrapper allocates a typed
`Sockaddr` result after the syscall. The raw path now calls `WSARecvFrom` with one `WSABuf`
and a stack `RawSockaddrAny`, then converts the kernel sockaddr directly to `netip.AddrPort`.
The syscall runs only after the readiness callback, so the would-block retry keeps its
existing shape.

| Size | Before | After | Change |
| --- | ---: | ---: | ---: |
| small | 11.546us + 4% | 6.947us + 1% | -39.83% (p=0.002) |
| 1500B | 11.686us + 10% | 7.112us + 1% | -39.14% (p=0.002) |
| bytes/read | 169 | 137 | -18.93% (p=0.002) |
| allocations/read | 7 | 6 | -14.29% (p=0.002) |

The 1500-byte throughput gate moved from about 122 MiB/s to 201 MiB/s.

Verification:

```powershell
go test -count=1 ./common/net/packet
go test -race -count=1 ./common/net/packet
go vet ./common/net/packet ./tunnel ./dns
GOOS=windows GOARCH=amd64 go build ./...
GOOS=windows GOARCH=arm64 go build ./...
GOOS=windows GOARCH=386 go build ./...
go test ./... -count=1
```
