# Sudoku And Inner UDP Packet Pool Report

Environment:

- Windows, `windows/amd64`
- Go: `go1.27.1`
- CPU: Intel Core i9-9900
- `go test ./listener/sudoku ./listener/inner -run '^$' -bench '^(BenchmarkUoTPacketDrop|BenchmarkInnerUDPPacketDrop)$' -benchmem -count=8`
- Comparison: `benchstat baseline.txt current.txt`

Sudoku payloads are now read into pooled buffers and the UoT wrapper is
returned from `Drop`. The inner packet wrapper uses the same single release
point.

| Metric | Baseline | Current | Change |
| --- | ---: | ---: | ---: |
| Sudoku UoT packet drop | 49.96n ± 14% | 29.90n ± 4% | -40.15% (p=0.000 n=8) |
| Sudoku bytes / allocations | 128 B / 2 allocs | 0 B / 0 allocs | -100% |
| Inner UDP packet drop | 47.15n ± 29% | 25.97n ± 9% | -44.91% (p=0.000 n=8) |
| Inner bytes / allocations | 32 B / 1 alloc | 0 B / 0 allocs | -100% |

The allocation results are the stable gates. The existing Sudoku framing,
flow-key, adapter, and inner UDP behavior tests remain in place.
