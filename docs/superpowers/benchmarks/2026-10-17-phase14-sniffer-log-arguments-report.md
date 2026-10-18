# Phase 14 Sniffer Log Arguments Report

Environment:

- Windows, `windows/amd64`
- CPU: Intel Core i9-9900 CPU @ 3.10GHz
- Go: `go1.27.1`
- Samples: 6 per case

Task 10 gated the DNS, tunnel, and tun logging arguments but missed the sniffer. Two paths on the
per-connection path still built expensive arguments for debug lines:

- `replaceDomain`, on every successfully sniffed connection with `override-destination: true`, formatted
  `SourceDetail()` (a nested `Sprintf` over the source address, process, and uid), `RemoteAddress()`, and
  the metadata string before the level gate inside `Debugln` dropped the line.
- The failure paths formatted `metadata.String()` and `DstIP` per failed sniff.

All four sites are now level-gated the same way. `metadata.String()` itself turns out to be allocation
free because it returns `Host` directly, so the failure paths only save the formatting work; the success
path is the real one.

| Metric | Unconditional | Gated | Change |
| --- | ---: | ---: | ---: |
| Success-path arguments | 373.15 ns/op | 0.43 ns/op | -99.88% |
| Success-path bytes | 216 B/op | 0 B/op | -100% |
| Success-path allocations | 10 | 0 | -100% |

Verification:

```powershell
go test ./component/sniffer ./tunnel -count=1
go test -race ./component/sniffer ./tunnel -count=1
go vet ./component/sniffer
go test ./... -count=1 -p 2
```

`TestTCPSniffOutcomeIsLevelIndependent` runs the full `TCPSniff` path at debug, info, and silent levels
and asserts the sniffed metadata is identical, so the gates cannot change behaviour.
