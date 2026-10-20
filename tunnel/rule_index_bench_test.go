package tunnel

import (
	"fmt"
	"net/netip"
	"testing"

	C "github.com/metacubex/mihomo/constant"
	rulesCommon "github.com/metacubex/mihomo/rules/common"
	"github.com/metacubex/mihomo/rules/wrapper"
	"github.com/stretchr/testify/require"
)

func TestRuleIndexPrecomputesHelperUsage(t *testing.T) {
	rules := make([]C.Rule, 0, 80)
	for index := 0; index < 20; index++ {
		domain := rulesCommon.NewDomain(fmt.Sprintf("d-%d.example.com", index), "DIRECT")
		rules = append(rules, wrapper.NewRuleWrapper(domain))

		ipCidr, err := rulesCommon.NewIPCIDR("10.0.0.0/8", "DIRECT")
		require.NoError(t, err)
		rules = append(rules, ipCidr)

		rules = append(rules, rulesCommon.NewDomainSuffix(fmt.Sprintf("s-%d.example.com", index), "DIRECT"))

		network, err := rulesCommon.NewNetworkType("udp", "DIRECT")
		require.NoError(t, err)
		rules = append(rules, network)
	}

	index := buildRuleIndex(rules)
	require.NotNil(t, index)
	require.Len(t, index.usesHelper, len(rules))

	for position, rule := range rules {
		require.Equal(t, ruleUsesMatchHelper(rule), index.usesHelperAt(position, rules),
			"precomputed helper usage mismatch at position %d", position)
	}

	// A bare rule slice without an index still answers through the direct walk.
	require.Equal(t, ruleUsesMatchHelper(rules[0]), (*ruleIndex)(nil).usesHelperAt(0, rules[:1]))
}

func benchmarkMixedRuleRuntime(tb testing.TB) {
	tb.Helper()

	oldRuntime := loadRuntime()
	configMux.Lock()
	currentRuntime.Store(&runtimeSnapshot{
		proxies: map[string]C.Proxy{"DIRECT": directTestProxy{}},
		mode:    Rule,
	})
	configMux.Unlock()
	tb.Cleanup(func() {
		configMux.Lock()
		currentRuntime.Store(oldRuntime)
		invalidateAllProxies()
		configMux.Unlock()
	})

	rules := make([]C.Rule, 0, 600)
	for index := 0; index < 300; index++ {
		domain := rulesCommon.NewDomain(fmt.Sprintf("d-%d.example.com", index), "DIRECT")
		rules = append(rules, wrapper.NewRuleWrapper(domain))

		ipCidr, err := rulesCommon.NewIPCIDR("10.0.0.0/8", "DIRECT")
		if err != nil {
			tb.Fatal(err)
		}
		rules = append(rules, ipCidr)
	}
	UpdateRules(rules, nil, nil)
}

func BenchmarkMatchMixedRulesScan(b *testing.B) {
	benchmarkMixedRuleRuntime(b)
	metadata := &C.Metadata{
		NetWork: C.TCP,
		Type:    C.HTTP,
		DstIP:   netip.MustParseAddr("192.0.2.99"),
		DstPort: 443,
	}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _, err := match(metadata, loadRuntime(), false, false)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func benchmarkLargeExactDomainRuntime(tb testing.TB) {
	tb.Helper()

	oldRuntime := loadRuntime()
	configMux.Lock()
	currentRuntime.Store(&runtimeSnapshot{
		proxies: map[string]C.Proxy{"DIRECT": directTestProxy{}},
		mode:    Rule,
	})
	configMux.Unlock()
	tb.Cleanup(func() {
		configMux.Lock()
		currentRuntime.Store(oldRuntime)
		invalidateAllProxies()
		configMux.Unlock()
	})

	rules := make([]C.Rule, 10_000)
	for index := range rules {
		domain := fmt.Sprintf("domain-%d.example.com", index)
		if index == 5_000 {
			domain = "target.example.com"
		}
		rules[index] = wrapper.NewRuleWrapper(
			rulesCommon.NewDomain(domain, "DIRECT"),
		)
	}
	UpdateRules(rules, nil, nil)
}

func BenchmarkMatchLargeExactDomainHit(b *testing.B) {
	benchmarkLargeExactDomainRuntime(b)
	metadata := &C.Metadata{
		NetWork: C.TCP,
		Type:    C.HTTP,
		Host:    "target.example.com",
	}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		proxy, _, err := match(metadata, loadRuntime(), false, false)
		if err != nil {
			b.Fatal(err)
		}
		if proxy == nil || proxy.Name() != "DIRECT" {
			b.Fatalf("proxy = %v", proxy)
		}
	}
}

func BenchmarkMatchLargeExactDomainMiss(b *testing.B) {
	benchmarkLargeExactDomainRuntime(b)
	metadata := &C.Metadata{
		NetWork: C.TCP,
		Type:    C.HTTP,
		Host:    "missing.example.com",
	}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		proxy, _, err := match(metadata, loadRuntime(), false, false)
		if err != nil {
			b.Fatal(err)
		}
		if proxy == nil || proxy.Name() != "DIRECT" {
			b.Fatalf("proxy = %v", proxy)
		}
	}
}

func BenchmarkMatchLargeDomainSuffixHit(b *testing.B) {
	benchmarkLargeExactDomainRuntime(b)
	metadata := &C.Metadata{
		NetWork: C.TCP,
		Type:    C.HTTP,
		Host:    "www.target.example.com",
	}
	rules := make([]C.Rule, 10_000)
	for index := range rules {
		suffix := fmt.Sprintf("suffix-%d.example.com", index)
		if index == 5_000 {
			suffix = "target.example.com"
		}
		rules[index] = rulesCommon.NewDomainSuffix(suffix, "DIRECT")
	}
	UpdateRules(rules, nil, nil)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		proxy, _, err := match(metadata, loadRuntime(), false, false)
		if err != nil {
			b.Fatal(err)
		}
		if proxy == nil || proxy.Name() != "DIRECT" {
			b.Fatalf("proxy = %v", proxy)
		}
	}
}

func BenchmarkMatchLargeDomainSuffixMiss(b *testing.B) {
	benchmarkLargeExactDomainRuntime(b)
	metadata := &C.Metadata{
		NetWork: C.TCP,
		Type:    C.HTTP,
		Host:    "missing.example.org",
	}
	rules := make([]C.Rule, 10_000)
	for index := range rules {
		rules[index] = rulesCommon.NewDomainSuffix(
			fmt.Sprintf("suffix-%d.example.com", index),
			"DIRECT",
		)
	}
	UpdateRules(rules, nil, nil)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		proxy, _, err := match(metadata, loadRuntime(), false, false)
		if err != nil {
			b.Fatal(err)
		}
		if proxy == nil || proxy.Name() != "DIRECT" {
			b.Fatalf("proxy = %v", proxy)
		}
	}
}

func BenchmarkMatchLargeIPRules(b *testing.B) {
	for _, ruleCount := range []int{100, 1000, 5000} {
		b.Run(fmt.Sprintf("rules=%d", ruleCount), func(b *testing.B) {
			oldRuntime := loadRuntime()
			configMux.Lock()
			currentRuntime.Store(&runtimeSnapshot{
				proxies: map[string]C.Proxy{"DIRECT": directTestProxy{}},
				mode:    Rule,
			})
			configMux.Unlock()
			b.Cleanup(func() {
				configMux.Lock()
				currentRuntime.Store(oldRuntime)
				invalidateAllProxies()
				configMux.Unlock()
			})

			rules := make([]C.Rule, 0, ruleCount)
			for index := 0; index < ruleCount; index++ {
				cidr := fmt.Sprintf("10.%d.%d.0/24", index>>8, index&0xff)
				ipCidr, err := rulesCommon.NewIPCIDR(cidr, "DIRECT")
				if err != nil {
					b.Fatal(err)
				}
				rules = append(rules, ipCidr)
			}
			UpdateRules(rules, nil, nil)
			metadata := &C.Metadata{
				NetWork: C.TCP,
				DstIP: netip.AddrFrom4([4]byte{
					10, byte((ruleCount - 1) >> 8), byte(ruleCount - 1), 5,
				}),
			}

			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				proxy, _, err := match(metadata, loadRuntime(), false, false)
				if err != nil {
					b.Fatal(err)
				}
				if proxy == nil || proxy.Name() != "DIRECT" {
					b.Fatalf("proxy = %v", proxy)
				}
			}
		})
	}
}
