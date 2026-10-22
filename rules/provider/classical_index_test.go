package provider

import (
	"testing"

	C "github.com/metacubex/mihomo/constant"
	"github.com/metacubex/mihomo/rules/common"
	"github.com/stretchr/testify/require"
)

type classicalStubRule struct {
	adapter string
	matched bool
	called  bool
}

func (r *classicalStubRule) RuleType() C.RuleType { return C.MATCH }
func (r *classicalStubRule) Match(*C.Metadata, C.RuleMatchHelper) (bool, string) {
	r.called = true
	return r.matched, r.adapter
}
func (r *classicalStubRule) Adapter() string       { return r.adapter }
func (r *classicalStubRule) Payload() string       { return "stub" }
func (*classicalStubRule) ProviderNames() []string { return nil }

func newClassicalTestStrategy(tb testing.TB, rules []C.Rule) *classicalStrategy {
	tb.Helper()
	strategy := &classicalStrategy{rules: rules, count: len(rules)}
	strategy.FinishInsert()
	return strategy
}

func TestClassicalIndexPreservesFirstMatch(t *testing.T) {
	matchingRule := &classicalStubRule{matched: true, adapter: "FIRST"}
	hiddenRule := &classicalStubRule{matched: true, adapter: "SECOND"}
	rules := []C.Rule{
		common.NewDomainSuffix("example.com", "unused"),
		common.NewDomain("www.example.com", "unused"),
		matchingRule,
		hiddenRule,
	}
	strategy := newClassicalTestStrategy(t, rules)

	metadata := &C.Metadata{Host: "www.example.com"}
	matched := strategy.Match(metadata, C.RuleMatchHelper{})
	require.True(t, matched)
	require.Equal(t, "FIRST", matchingRule.adapter)
	require.False(t, hiddenRule.called)
}

func TestClassicalIndexStopsAtEarlierNonDomainRule(t *testing.T) {
	matchingRule := &classicalStubRule{matched: true, adapter: "NON-DOMAIN"}
	hiddenRule := &classicalStubRule{matched: true, adapter: "DOMAIN"}
	rules := []C.Rule{
		matchingRule,
		common.NewDomainSuffix("example.com", "unused"),
		hiddenRule,
	}
	strategy := newClassicalTestStrategy(t, rules)

	metadata := &C.Metadata{Host: "www.example.com"}
	require.True(t, strategy.Match(metadata, C.RuleMatchHelper{}))
	require.True(t, matchingRule.called)
	require.False(t, hiddenRule.called)
}

func TestClassicalIndexLeavesLiteralDotSuffixLinear(t *testing.T) {
	dotRule := common.NewDomainSuffix(".target.example.com", "unused")
	suffixRule := common.NewDomainSuffix("target.example.com", "unused")
	strategy := newClassicalTestStrategy(t, []C.Rule{dotRule, suffixRule})

	metadata := &C.Metadata{Host: "www.target.example.com"}
	require.True(t, strategy.Match(metadata, C.RuleMatchHelper{}))
	require.False(t, strategy.Match(&C.Metadata{Host: "missing.example.net"}, C.RuleMatchHelper{}))
}

func TestClassicalIndexMatchesExactAndSuffix(t *testing.T) {
	rules := make([]C.Rule, 0, 1001)
	for index := 0; index < 500; index++ {
		rules = append(rules, common.NewDomain(
			"domain-"+string(rune('a'+index%26))+".example.com",
			"unused",
		))
	}
	rules = append(rules, common.NewDomain("target.example.com", "unused"))
	for index := 0; index < 500; index++ {
		rules = append(rules, common.NewDomainSuffix(
			"suffix-"+string(rune('a'+index%26))+".example.org",
			"unused",
		))
	}
	rules = append(rules, common.NewDomainSuffix("target.example.org", "unused"))
	strategy := newClassicalTestStrategy(t, rules)

	require.True(t, strategy.Match(&C.Metadata{Host: "target.example.com"}, C.RuleMatchHelper{}))
	require.True(t, strategy.Match(&C.Metadata{Host: "www.target.example.org"}, C.RuleMatchHelper{}))
	require.False(t, strategy.Match(&C.Metadata{Host: "missing.example.net"}, C.RuleMatchHelper{}))
}

func benchmarkClassicalDomains(b *testing.B, exact bool, hit bool) *classicalStrategy {
	b.Helper()
	rules := make([]C.Rule, 10_000)
	for index := range rules {
		payload := "payload-" + string(rune('a'+index%26)) + ".example.com"
		if index == 5_000 {
			payload = "target.example.com"
		}
		if exact {
			rules[index] = common.NewDomain(payload, "unused")
		} else {
			rules[index] = common.NewDomainSuffix(payload, "unused")
		}
	}
	strategy := newClassicalTestStrategy(b, rules)
	b.Cleanup(func() { strategy.Reset() })

	host := "missing.example.net"
	if hit {
		host = "target.example.com"
		if !exact {
			host = "www.target.example.com"
		}
	}
	metadata := &C.Metadata{Host: host}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if strategy.Match(metadata, C.RuleMatchHelper{}) != hit {
			b.Fatal("classical index match mismatch")
		}
	}
	return strategy
}

func BenchmarkClassicalIndexExactHit(b *testing.B) {
	benchmarkClassicalDomains(b, true, true)
}

func BenchmarkClassicalIndexExactMiss(b *testing.B) {
	benchmarkClassicalDomains(b, true, false)
}

func BenchmarkClassicalIndexSuffixHit(b *testing.B) {
	benchmarkClassicalDomains(b, false, true)
}

func BenchmarkClassicalIndexSuffixMiss(b *testing.B) {
	benchmarkClassicalDomains(b, false, false)
}
