# Sudoku UoT Address Fast Path Report

Environment:

- Windows, `windows/amd64`
- Go: `go1.27.1`
- CPU: Intel Core i9-9900
- `go test ./transport/sudoku -run '^$' -bench '^BenchmarkReadDatagram$' -benchmem -count=8`
- Comparison: `benchstat baseline.txt current.txt`

The old reader decoded the SOCKS address into a string and then the listener
parsed that string back into a `socks5.Addr`. The new path keeps the address
as SOCKS bytes, uses pooled address/payload buffers, and releases both from
the wrapper's `Drop` path.

| Metric | Baseline | Current | Change |
| --- | ---: | ---: | ---: |
| Datagram read | 511.0n ± 90% | 173.2n ± 12% | -66.12% (p=0.000 n=8) |
| Bytes / allocations | 193 B / 10 allocs | 4 B / 1 alloc | -97.93% bytes, -90% allocs |

The baseline time was noisy under host load; the allocation reduction is the
stable gate. UoT round-trip tests cover IP and domain address paths.
