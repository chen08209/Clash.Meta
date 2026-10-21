# Coverage and Optimization Ledger

Date: 2026-10-21

Environment: Windows, `windows/amd64`, Intel Core i9-9900, Go `1.27.1`.

This report records the state after the UDP metadata-reuse, process-cache and
lock-free group cycles: what shipped, what gate pins it, and where the test
coverage of the hot paths stands. Every optimization below is gated by a
focused test, a benchmark or both.

## Hot-path coverage

| Package | Coverage | Gate added |
| --- | ---: | --- |
| `rules/logic` | 92.7% | AND/OR/NOT semantics, PASS-RULE continuation, CheckPassRule helper, sub-rule recursion, parse errors |
| `component/sniffer` | 80.9% | dispatcher, quic and http sniffers |
| `rules` (dispatch) | 75.5% | the full ParseRule type table, params, error paths, uid platform gate |
| `tunnel` | 56.7% | rule index, packet flow key, deadlines, sender mapping, metadata template |
| `component/nat` | 54.1% | binary flow keys, allocation-free local-conn lookup |
| `tunnel/statistic` | 49.0% | stripes, tracker lifecycle, snapshot |
| `listener/socks` | 17.1% | per-flow metadata cache identity, split targets, warm-path benchmark |
| `dns` | 22.1% | cache keys, parsed-ip cache, conn pool, exchange errors |
| `hub/route` | 12.8% | real-listener auth matrix, hello/version, read endpoints |
| `component/process` | 8.5% | cache hits, expiry, failure handling, 53ns hit benchmark |
| `rules/common` | 15.6% | per-connection matcher semantics, resolve-helper contract |

The listener and transport protocol packages without test files are
integration surfaces; the remaining zero-coverage entries there are noted, not
papered over with getter-only tests.

## Optimization ledger since `50648f94b`

| Commit | Area | Gate |
| --- | --- | --- |
| `b661f3154` | per-flow metadata template on the sing listeners | pointer-identity test, batch benchmark 807 -> 312 allocs |
| `a1183832a` | socks/inner per-flow metadata reuse | pointer-identity test, warm benchmark 1 alloc |
| `1d7843ff4` | lock-free provider versions for group picks | benchstat: url-test 47->25ns, fallback 42->20ns, getProxies 35->0.7ns |
| `3b1f08b2c` | socket-to-process lookup cache | unit tests, 53ns zero-alloc hit benchmark |
| `fc5a68d26` | quic sniffer reads the conn id as a buffer view | datagram-batch benchmark 807 -> 312 allocs |
| `024860d07` | pooled plain-udp dns conns, cheaper doh url | conn-pool unit tests |
| `e7f635cb0` | preallocated api snapshots, async post-apply collect | snapshot shape via the route tests |
| `10befb0c1` | folded rule hit timestamps, first-result picker, fakeip hoist | picker pool tests |
| `4e2307dec` | restore the escape-based pre-go1.24 comparable hash | ci matrix green on 1.20-1.26 |
| `5ed65de65` | linux 386 packet fallback | `GOOS=linux GOARCH=386 go build ./...` |
| `96aee6987` | drop the never-wired dns ClearAll, unused status codecs | deadcode scan, both build entry points |

## Dead-code ledger

Removed: the never-wired dns `ClearAll` family, the unused `TunnelStatus`
text codecs. Kept deliberately (upstream-inherited, dead in this fork's
wiring but preserved for merge parity and downstream consumers such as CMFA):
`component/trie` DomainMap and IpCidrTrie families, the anytls skiplist func
variant and the legacy vmess AEAD codec. The toolchain floor has since moved
to go 1.24, so the pre-go1.24 maphash port, the once and contextutils shims
and the other build-tagged variants are deleted, the x/exp dependencies are
replaced by the stdlib, and the ci matrix runs go 1.24-1.27.1.

## Measured and deferred

A deadcode re-scan over both build entry points after the toolchain raise
finds 250 unreachable functions, down 34 from the 284 before the cleanup
rounds. The only new entry is TunnelStatus.String, whose sole caller was the
removed status mapping; it stays as the exported enum's Stringer surface.

New benchmarks quantify the two deferred dns paths on a typical two-answer
reply: the message-cache hit deep copy costs ~0.4 microseconds / 344 B / 7
allocs per hit and the parsed-ip lookup path ~0.2 microseconds / 48 B / 1
alloc - together well under 0.1 percent of a core at a thousand hits per
second, so the copy-on-write and worker-pool reworks stay rejected as
risk-without-payoff.

## Regression re-run

After the coverage, dead-code and documentation rounds landed on top of the
optimizations, every ledger benchmark was re-executed (`-count=5`,
`windows/amd64`): socks warm handle 77-92 ns / 1 alloc, process cache hit
50.7-53.7 ns / 0 allocs, the sing datagram batch steady at 312 allocs, group
picks 20.1-20.7 ns (fallback), 24.2-31.4 ns (url-test) and 0.68-0.79 ns
(getProxies), the 5000-rule ip scan 174-185 ns, and the existing-flow udp
handle 96-100 ns / 0 allocs. All within noise of the numbers above; the later
rounds added no forwarding-path cost.
', '
## Build matrix

The full tree cross-compiles for every ci matrix platform plus freebsd:
linux amd64/arm64/386, windows amd64/arm64, darwin amd64/arm64 and freebsd
amd64 all build clean on `c013bb86f`, which covers the platform-specific
packet, maphash and deadline changes from these rounds.

The FlClash app bridge (`core/go.mod` with the submodule replace) builds and
vets clean against this line, and its test suite carries one pre-existing
failure bisected to the released `10befb0c1` as well - an easytier dns test
scenario, unrelated to this line.
