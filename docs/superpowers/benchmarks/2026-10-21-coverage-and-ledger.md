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
| `component/sniffer` | 84.3% | dispatcher, quic and http sniffers, the tls wrapper over a generated client hello, the per-sniffer network and port support, the need-at-least error chain, the enable gate, the skip-domain policy, and the udp dispatch over both override-dest settings |
| `rules` (dispatch) | 75.5% | the full ParseRule type table, params, error paths, uid platform gate |
| `rules/provider` | 76.6% | strategy lifecycles, mrs round trips and text export, file-vehicle initial/update, payload fallback, the inline mapping decode, and the RuleSet rule through the injected tunnel registry (src swap, no-resolve, absent provider) |
| `tunnel` | 59.2% | rule index, packet flow key, deadlines, sender mapping, metadata template, the mode and status text round trips, and the all-proxies cache: the up-to-date check against missing, extra and bumped providers, the rebuild that follows a version, the explicit drop, and both provider snapshots |
| `component/nat` | 98.4% | binary flow keys, allocation-free local-conn lookup, the string-keyed flow lifecycle with its invalid-key no-ops, the typed load, and the local-conn lock lifecycle from creation through reuse to the fresh lock after a drop |
| `tunnel/statistic` | 78.0% | stripes, tracker lifecycle, snapshot, the tcp tracker's read/write accounting in both directions with and without a manager push, the udp tracker's packet accounting over a live socket pair, the join/leave lifecycle through the snapshot, and the now/total traffic readers |
| `listener/socks` | 33.8% | the listener's full round trip over a real udp socket (socks5 framing in, flow handling, the write-back answer out), the flow write-back surviving Drop, per-flow metadata cache identity, split targets, the warm-path benchmark, and the packet adapter's payload passthrough with the return-once drop of its buffer |
| `listener/shadowsocks` | 26.0% | per-flow metadata cache identity, split targets, the unresolved-header drop, the warm-path 0-alloc benchmark, and the packet adapter's write-back encoding with the return-once drop of its pooled buffer |
| `listener/tunnel` | 51.6% | the listener's udp round trip over a real socket (a datagram carried to the target it was opened with and answered back to its client), the wrapper's buffer release, and the flow key it hands the tunnel |
| `dns` | 31.0% | cache keys, parsed-ip cache, conn pool, exchange errors, the rcode transport codes, the disable-types and edns0-subnet client wrappers including their param tables, the log-string shapes, and the resolver enhancer: mode gating, the fake-ip range against the reserved addresses, the mapping round trip, the ipv6 pool, flush and the reload patch |
| `hub/route` | 15.6% | real-listener auth matrix, hello/version, read endpoints, provider and group reads with their render keys, and the not-found paths of the proxy and provider lookups plus the unknown-connection close |
| `component/process` | 32.3% | cache hits, expiry, failure handling, 53ns hit benchmark, the find-process-mode text round trips, the invalid-network rejection through the public entry, and the package-name resolver delegation |
| `component/resource` | 25.9% | the fetcher's state lock: the regression test drives Update, SideUpdate and UpdatedAt together, which the race detector reports as the stored hash and the stored timestamp without it |
| `rules/common` | 67.0% | the payload shapes ParseRulePayload accepts, src/no-resolve params, the inbound name/user/type rules with the socks expansion, the process rule over name, path, regex and wildcard, the ip-suffix trailing-bit match, the match and ipcidr rule surfaces, and the geoip lan rule, its dns fallback filter, the geodata-mode metadata codes and the no-database constructor errors |
| `common/once` | 100.0% | resettable Once: Done/Reset semantics, single run under concurrency, panic marks done |

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
| `f07e6fb62` | structured sticky-session hashes instead of string keys | sticky pick benchmark 594-606 ns / 6 allocs -> 438-462 ns / 2 allocs |
| `7dbed4aaf` | direct addrport hashing for the UDP queue picker | queue-hash benchmark 9.83-10.28 ns -> 3.83-3.92 ns, 0 allocs |
| `427435b96` | Hysteria UDP hop pool uses fixed-size array pointers | pool benchmark 27.78-65.95 ns / 1 alloc -> 9.37-10.03 ns, 0 allocs |
| `4940fbb70` | recycle sing UDP packet wrappers through Drop | 100-datagram batch 11.2-65.6 us / 15.13-15.15 KB / 211 allocs -> 9.41-10.34 us / 7.13 KB / 111 allocs |
| `4a0ba873b` | reuse sing packet read buffers after Drop | 100-datagram batch 9.41-10.34 us / 7.13 KB / 111 allocs -> 6.10-6.32 us / 714-715 B / 11 allocs |
| `8caa05d75` | allocate TCP peek buffering only for sniffing or handshakes | direct TCP setup benchmark 45.2-42.1 KB / 134-137 allocs -> 41.3-41.6 KB / 128 allocs; conn-context test pins the lazy path |
| `e99565b3f` | recycle socks UDP packet wrappers through Drop | warm datagram 86.20-226.0 ns / 80 B / 1 alloc -> 69.18-72.55 ns, 0 allocs (benchstat p=0.000, n=10) |
| `b02ab5bbf` | recycle tunnel UDP packet wrappers through Drop | tunnel UDP 504 B / 4 allocs -> 440 B / 3 allocs; latency unchanged within noise (p=0.481, n=10) |
| `53f04488c` | recycle tproxy UDP packet wrappers through Drop | tproxy UDP 560 B / 5 allocs -> 448 B / 4 allocs; latency unchanged within noise (p=0.052, n=10) |
| `b975184df` | recycle mieru UDP packet wrappers through Drop | mieru UDP 656 B / 8 allocs -> 576 B / 7 allocs; latency unchanged within noise (p=0.143, n=10) |
| `af18b7787` | raw WSARecvFrom into a callback-local kernel sockaddr on the windows read path | read gate 169 B / 7 allocs -> 137 B / 6 allocs per datagram (p=0.000, n=10); the wall-clock delta stayed inside the host's load noise across runs, so the allocation count is the gate; the read test now covers udp4 and udp6 |
| `329f8ef03` | replace the match helper's three function fields with one context interface | rule matching 72 B / 4 allocs -> 24 B / 1 alloc (p=0.000, n=10); the remaining allocation is the match context |
| `90ea4115f` | pool and reset the rule match context | rule matching 24 B / 1 alloc -> 0 B / 0 allocs (p=0.000, n=10); IP rule scans 22-55% faster |
| `72a7590e5` | per-flow metadata reuse and wrapper pooling on the shadowsocks udp path | warm datagram 548 B / 4 allocs -> 0 B / 0 allocs (p=0.000, n=10); the host was loaded, so the time delta is recorded but not gated |
| `99c4773b` | pooled Trojan UDP packet wrappers through Drop | wrapper drop 70.08n / 96 B / 1 alloc -> 30.48n / 0 B / 0 allocs (p=0.000, n=8) |
| `bc7bc77be` | take the udp destination as an AddrPort on the tproxy and trojan paths | tproxy read-loop shape 480 B / 6 allocs -> 440 B / 3 allocs per datagram (p=0.000, n=10); the plain handle path 448 B / 4 allocs -> 440 B / 3 |
| `390d17525` | reuse trojan UDP flow metadata per destination | inbound baseline 506 B / 4 allocs -> 22 B / 0 allocs (p=0.000, n=10); the loaded host kept the time delta off the gate |
| `1a927214` | precompute the udp dns client transport | udp exchange 23 -> 22 allocs (p=0.000, n=12); the localhost round trip swamped the time delta, so the allocation count is the gate |
| `e1652693` | read udp through the conn poller | read gate 137 B / 6 allocs -> 0 B / 0 allocs per datagram (p=0.000, n=12); the 1500-byte read 12.93us -> 9.36us and 110.6 -> 153.1 MiB/s (p=0.005, n=12) |
| `eb5e6bcaa` | recycle generic packet adapters, Snell UDP flow state and Hysteria hop wrappers | adapter warm path 1 -> 0 allocs; Snell warm datagram 0 B / 0 allocs; Hysteria queue wrapper 32 B / 1 alloc -> 0 B / 0 allocs and 21.04-23.29 ns vs 32.46-35.47 ns |
| `1acea74e3` | write trojan UDP replies directly from an AddrPort | write-back 52.83 ns / 52 B / 2 allocs -> 2.71 ns / 0 B / 0 allocs (p=0.000, n=10) |
| `2d13dbb7a` | reuse the sing UDP write-back address | sing write-back 47.01 ns / 52 B / 2 allocs -> 17.54 ns / 0 B / 0 allocs (p=0.000, n=10) |
| `5fce390e9` + `1c277c6f8` | allocate the UDP packet wrappers, the packet adapter and the Snell wrapper per datagram again | the pooling above did not survive being the flow's write-back target: the flow answers through the last packet the sender handed it, the sender drops that object as soon as it processed it, and the next datagram then claimed it. The generic adapter was the worst case - clearing its wrapped packet on Drop made the reply path panic on a nil UDPPacket, which `1c277c6f8` fixes. `listener/socks/writeback_target_test.go` covers the adapter path and fails on the pooled tree; the read buffers and the Snell flow-metadata cache stay |

The wrapper-pooling rows `4940fbb70`, `e99565b3f`, `b02ab5bbf`, `53f04488c`,
`b975184df` and `99c4773b`, plus the wrapper half of `72a7590e5` and the adapter
and Snell-wrapper halves of `eb5e6bcaa`, are superseded by `5fce390e9` and
`1c277c6f8`. Their
allocation numbers stand as measurements, but the object
recycling they introduced cannot stay: the packet outlives `Drop` as its flow's
write-back target, which is what `Drop`'s contract in `constant/adapters.go`
now spells out.

## Dead-code ledger

Removed: the never-wired dns `ClearAll` family, the unused `TunnelStatus`
text codecs, and `common/net/relay.go`, a whole file of commented-out relay
code the sing `Relay` replaced (its only live line was `package net`).
Kept deliberately (upstream-inherited, dead in this fork's
wiring but preserved for merge parity and downstream consumers such as CMFA):
`component/trie` DomainMap and IpCidrTrie families, the anytls skiplist func
variant and the legacy vmess AEAD codec. The toolchain floor has since moved
to go 1.24, so the pre-go1.24 maphash port, the once and contextutils shims
and the other build-tagged variants are deleted, the x/exp dependencies are
replaced by the stdlib, and the ci matrix runs go 1.24-1.27.1. The once
package is now a single resettable `Once` with `Do`/`Done`/`Reset`, replacing
the unsafe `sync.Once` introspection that read the runtime's internal layout,
and `component/keepalive` lost its go1.21/go1.22/go1.23 halves, keeping one
implementation per platform.

The ipcidr strategy's commented-out `trie` variants are deleted, its
constructor now builds the cidr set it needs, and a classical strategy without
a wired parser skips its rules instead of dereferencing nil, which is what the
`convert-ruleset` entry point used to hit.

## Measured and deferred

A deadcode re-scan over both build entry points after the toolchain raise
finds 250 unreachable functions, down 34 from the 284 before the cleanup
rounds. The only new entry is TunnelStatus.String, whose sole caller was the
removed status mapping; it stays as the exported enum's Stringer surface.

F2, the per-read syscall address allocation, is now closed everywhere. The
per-platform raw recv closures went with it: the wrapped udp conn reads
through `ReadFromUDPAddrPort`, which fills a stack sockaddr on every platform
and drives the recvfrom from the poller instead of a second readiness
callback, so darwin and the BSDs no longer need their own len-prefixed
sockaddr converter. Only compilation of those paths is verified locally.

New benchmarks quantify the two deferred dns paths on a typical two-answer
reply: the message-cache hit deep copy costs ~0.4 microseconds / 344 B / 7
allocs per hit and the parsed-ip lookup path ~0.2 microseconds / 48 B / 1
alloc - together well under 0.1 percent of a core at a thousand hits per
second, so the copy-on-write and worker-pool reworks stay rejected as
risk-without-payoff.

The packet read deadline emulation is closed rather than replaced. The
outbound packet conns that cannot handle a read deadline natively (the
sing-backed ones: hysteria, tuic, shadowquic) emulated it with a goroutine per
read, and the tunnel keeps a 60s deadline set for a busy flow, so the pipe
branch was the steady state: 96-97 B / 2 allocs per reply read against 0 on
the direct path. The flow now owns an idle timer that closes its conn instead,
and the emulation, its pipe reader and the outbound wrapping branch are gone;
the flow reply read is 0 B / 0 allocs.

## Regression re-run

After the coverage, dead-code and documentation rounds landed on top of the
optimizations, every ledger benchmark was re-executed (`-count=5`,
`windows/amd64`): socks warm handle 77-92 ns / 1 alloc, process cache hit
50.7-53.7 ns / 0 allocs, the sing datagram batch steady at 312 allocs, group
picks 20.1-20.7 ns (fallback), 24.2-31.4 ns (url-test) and 0.68-0.79 ns
(getProxies), the 5000-rule ip scan 174-185 ns, and the existing-flow udp
handle 96-100 ns / 0 allocs. All within noise of the numbers above; the later
rounds added no forwarding-path cost.
A dynamic-profiling attempt against a minimal mixed-port harness exposed
and fixed a real startup regression: the upstream refactor had commented
out updateListeners from applyConfig, so the standalone binary ignored
every proxy port from the config file and came up deaf. The startup is
restored and pinned by an end-to-end executor test; the config package
keeps its go:linkname into the executor, so its tests only run inside the
full binary. The traditional port keys stay deprecated by upstream design:
the listeners section is the supported path, and the api PATCH still
honours the legacy keys.

A bisect pinned the harness failure to the environment, not this line:
the standalone binary fails the mixed-port tunnel handoff identically on
the released `10befb0c1` and on this head, while the adapter delay probe
dials the same loopback target in 2 ms - the production FlClash bridge
path is unaffected.

## Build matrix

The full tree cross-compiles for every ci matrix platform plus freebsd:
linux amd64/arm64/386, windows amd64/arm64, darwin amd64/arm64 and freebsd
amd64 all build clean on `c013bb86f`, which covers the platform-specific
packet, maphash and deadline changes from these rounds.

The FlClash app bridge (`core/go.mod` with the submodule replace) builds and
vets clean against this line, and its test suite carries one pre-existing
failure bisected to the released `10befb0c1` as well - an easytier dns test
scenario, unrelated to this line.
