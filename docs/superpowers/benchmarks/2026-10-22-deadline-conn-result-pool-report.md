# Deadline Connection Read Result Pool

Date: 2026-10-22

Environment:

- Windows, `windows/amd64`
- CPU: Intel Core i9-9900 CPU @ 3.10GHz
- Go: `go1.27.1`
- Baseline: `e5c558fdf`

## Change

The `net.Conn` half of the deadline package still allocated a fresh
`connReadResult` for every piped read. The goroutine target is now a top-level
function, and the result object is reset and returned through a `sync.Pool`
once the buffer is fully consumed. The UDP packet deadline emulation that used
the same pattern was removed upstream instead of optimized.

## Benchmark

Command:

```text
go test ./common/net/deadline -run '^$' \
  -bench '^BenchmarkConnReadDeadline$' \
  -benchmem -benchtime=200ms -count=10
```

`benchstat` result:

```text
ConnReadDeadline-16   136.00 B/op   3.000 allocs/op
ConnReadDeadline-16    88.00 B/op   2.000 allocs/op
```

The allocation gate is `-35.3%` bytes and `-33.3%` allocations at `p=0.000`,
`n=10`. Two latency samples were load-contaminated and the run spanned `+7.7%`
with a `±34%` spread, so latency is recorded as unmeasured rather than gated.
The remaining two allocations are the goroutine start and the read buffer
itself.

## Verification

- `go test -race ./common/net/deadline -count=1`
- `go vet ./common/net/deadline`
