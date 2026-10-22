# Phase 19 VLESS Response Header Report

Environment:

- Windows, `windows/amd64`
- Go: `go1.27.1`
- Evidence: `go test -race ./listener/inbound` and inspection of the write paths

`serverConn.responseWritten` decided whether the next write carries the two-byte VLESS response
header. It was a plain bool read by `Write`, `WriteBuffer`, `FrontHeadroom` and `WriterReplaceable`,
and written by the first two. The first `Write` came from the multiplex session's send loop while the
relay path used `WriteBuffer`, so the flag was a data race, and a lost update would have prepended the
header twice and corrupted the stream.

The flag is now an atomic bool and the claim is a compare-and-swap, so exactly one of the racing
writers prepends the header and the rest fall through. The load on the steady path costs the same as a
plain read on amd64.

A deeper race remains in the same report and is not fixable here: the forked `metacubex/tls` performs
an unlocked read inside `Write` while the HTTP/2 async frame writer and the multiplex send loop both
write one connection, so the racing field lives in the dependency. Fixing it means patching the
dependency or removing the concurrent-writer topology, both out of scope for this phase.

Verification:

```powershell
go test ./... -count=1 -p 2
go vet ./listener/sing_vless
go test -race ./listener/inbound -count=1
```

After the change no `sing_vless` frame appears in a read/write pair of the race report; the pairs that
remain belong to the forked TLS library's concurrent writes.
