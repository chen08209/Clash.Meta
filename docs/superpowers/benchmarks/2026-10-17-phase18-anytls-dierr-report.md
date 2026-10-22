# Phase 18 AnyTLS Stream Error Report

Environment:

- Windows, `windows/amd64`
- Go: `go1.27.1`
- Evidence: `go test -race ./listener/inbound` before and after

`Stream.dieErr` was written once under `dieOnce` by the session's receive loop or the user's `Close`,
and read on every `Read` and `Write` by the relay goroutines. `sync.Once` orders the writers among
themselves but says nothing to the readers, so the plain field was a data race; the same shape as the
Vision state.

The field is now `atomic.TypedValue[error]`. The store happens once per stream at close, the loads on
the per-packet paths cost the same as a plain read on amd64, and the values are unchanged.

Verification across the whole `listener/inbound` race run: of 2134 reported race blocks, the number
with an `anytls/session.Stream` frame in the read/write pair went from the baseline's races to zero.
Frames that remain in goroutine-creation stacks are informational and are not races.

```powershell
go test ./transport/anytls/... -count=1
go test -race ./listener/inbound -count=1
```

The blocks that still report races belong to other transports (`transport/snell`, `transport/vless`
XHTTP, and `listener/sing_vless`'s `responseWritten`), each a separate pre-existing race of the same
shape that reproduces on the unmodified tree; they are out of scope for this phase and documented for a
follow-up.
