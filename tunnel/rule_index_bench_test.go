package tunnel

import (
	"fmt"
	"testing"

	C "github.com/metacubex/mihomo/constant"
	rulesCommon "github.com/metacubex/mihomo/rules/common"
	"github.com/metacubex/mihomo/rules/wrapper"
)

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
