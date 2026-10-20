package tunnel

import (
	"fmt"
	"net/netip"
	"sync/atomic"
	"testing"

	C "github.com/metacubex/mihomo/constant"
	rulesCommon "github.com/metacubex/mihomo/rules/common"
	"github.com/metacubex/mihomo/rules/wrapper"
	"github.com/stretchr/testify/require"
)

func setupRuleRuntime(
	t *testing.T,
	proxies map[string]C.Proxy,
	rules []C.Rule,
	subRules map[string][]C.Rule,
) {
	t.Helper()

	oldRuntime := loadRuntime()
	configMux.Lock()
	currentRuntime.Store(&runtimeSnapshot{
		proxies: proxies,
		mode:    Rule,
	})
	configMux.Unlock()
	t.Cleanup(func() {
		configMux.Lock()
		currentRuntime.Store(oldRuntime)
		invalidateAllProxies()
		configMux.Unlock()
	})
	UpdateRules(rules, subRules, nil)
}

func TestMatchDoesNotScanAnExactDomainRun(t *testing.T) {
	const ruleCount = 10_000
	const target = 5_000

	var calls atomic.Int32
	rules := make([]C.Rule, 0, ruleCount)
	for i := 0; i < ruleCount; i++ {
		domain := fmt.Sprintf("domain-%d.example.com", i)
		if i == target {
			domain = "target.example.com"
		}
		rules = append(rules, &countingDomainRule{
			Rule:  rulesCommon.NewDomain(domain, "DIRECT"),
			calls: &calls,
		})
	}

	setupRuleRuntime(
		t,
		map[string]C.Proxy{"DIRECT": directTestProxy{}},
		rules,
		nil,
	)
	metadata := &C.Metadata{
		NetWork: C.TCP,
		Type:    C.HTTP,
		Host:    "target.example.com",
	}

	proxy, rule, err := match(metadata, loadRuntime(), false, false)
	require.NoError(t, err)
	require.Equal(t, "DIRECT", proxy.Name())
	require.Equal(t, rules[target], rule)
	require.LessOrEqual(t, calls.Load(), int32(2), "exact-domain match scanned the rule run")
}

func TestMatchDoesNotScanAnIPRun(t *testing.T) {
	const ruleCount = 10_000
	const target = 5_000
	var calls atomic.Int32
	rules := make([]C.Rule, 0, ruleCount)
	for index := 0; index < ruleCount; index++ {
		cidr := fmt.Sprintf("10.%d.%d.0/24", index>>8, index&0xff)
		if index == target {
			cidr = "203.0.113.0/24"
		}
		ipCidr, err := rulesCommon.NewIPCIDR(cidr, "DIRECT")
		require.NoError(t, err)
		rules = append(rules, &countingIPRule{Rule: ipCidr, calls: &calls})
	}

	setupRuleRuntime(
		t,
		map[string]C.Proxy{"DIRECT": directTestProxy{}},
		rules,
		nil,
	)
	metadata := &C.Metadata{
		NetWork: C.TCP,
		DstIP:   netip.MustParseAddr("203.0.113.5"),
	}

	proxy, rule, err := match(metadata, loadRuntime(), false, false)
	require.NoError(t, err)
	require.Equal(t, "DIRECT", proxy.Name())
	require.Equal(t, rules[target], rule)
	require.LessOrEqual(t, calls.Load(), int32(2), "ip-cidr match scanned the rule run")
}

func TestMatchPreservesOrderAcrossOverlappingIPRules(t *testing.T) {
	wide, err := rulesCommon.NewIPCIDR("203.0.0.0/8", "DIRECT")
	require.NoError(t, err)
	narrow, err := rulesCommon.NewIPCIDR("203.0.113.0/24", "DIRECT")
	require.NoError(t, err)
	middle, err := rulesCommon.NewIPCIDR("203.0.112.0/20", "DIRECT")
	require.NoError(t, err)
	rules := []C.Rule{wide, narrow, middle}

	setupRuleRuntime(
		t,
		map[string]C.Proxy{"DIRECT": directTestProxy{}},
		rules,
		nil,
	)
	metadata := &C.Metadata{
		NetWork: C.TCP,
		DstIP:   netip.MustParseAddr("203.0.113.5"),
	}

	proxy, rule, err := match(metadata, loadRuntime(), false, false)
	require.NoError(t, err)
	require.Equal(t, "DIRECT", proxy.Name())
	require.Equal(t, C.Rule(wide), rule, "the first configured rule must win")
}

func TestMatchIPRunSkipsSourceAndDestinationIndependently(t *testing.T) {
	sourceRule, err := rulesCommon.NewIPCIDR(
		"192.168.0.0/16",
		"DIRECT",
		rulesCommon.WithIPCIDRSourceIP(true),
	)
	require.NoError(t, err)
	destinationRule, err := rulesCommon.NewIPCIDR("203.0.113.0/24", "DIRECT")
	require.NoError(t, err)
	rules := []C.Rule{destinationRule, sourceRule}

	setupRuleRuntime(
		t,
		map[string]C.Proxy{"DIRECT": directTestProxy{}},
		rules,
		nil,
	)
	metadata := &C.Metadata{
		NetWork: C.TCP,
		DstIP:   netip.MustParseAddr("203.0.113.5"),
		SrcIP:   netip.MustParseAddr("192.168.1.2"),
	}

	proxy, rule, err := match(metadata, loadRuntime(), false, false)
	require.NoError(t, err)
	require.Equal(t, "DIRECT", proxy.Name())
	require.Equal(t, C.Rule(destinationRule), rule)
}

func TestRuleIndexNextIPRunWithUnresolvedAddressesScansLinearly(t *testing.T) {
	first, err := rulesCommon.NewIPCIDR("10.0.0.0/8", "DIRECT")
	require.NoError(t, err)
	second, err := rulesCommon.NewIPCIDR("203.0.113.0/24", "DIRECT")
	require.NoError(t, err)
	rules := []C.Rule{first, second}
	index := buildRuleIndex(rules)
	metadata := &C.Metadata{NetWork: C.UDP}

	require.Equal(t, 0, index.next(0, metadata),
		"an unresolved address must scan the run from its start")
	require.Equal(t, 1, index.next(1, metadata))
}

func TestRuleIndexIPRunSkipsToItsEndWhenNothingMatches(t *testing.T) {
	first, err := rulesCommon.NewIPCIDR("10.0.0.0/8", "DIRECT")
	require.NoError(t, err)
	second, err := rulesCommon.NewIPCIDR("203.0.113.0/24", "DIRECT")
	require.NoError(t, err)
	after, err := rulesCommon.NewNetworkType("udp", "DIRECT")
	require.NoError(t, err)
	rules := []C.Rule{first, second, after}
	index := buildRuleIndex(rules)
	metadata := &C.Metadata{
		NetWork: C.TCP,
		DstIP:   netip.MustParseAddr("8.8.8.8"),
	}

	require.Equal(t, 2, index.next(0, metadata),
		"a run with no matching rule must be skipped whole")
}

func TestMatchFallsThroughADisabledIndexedRule(t *testing.T) {
	disabled := wrapper.NewRuleWrapper(
		rulesCommon.NewDomain("target.example.com", "DIRECT"),
	)
	disabled.SetDisabled(true)
	enabled := wrapper.NewRuleWrapper(
		rulesCommon.NewDomain("target.example.com", "DIRECT"),
	)
	rules := []C.Rule{disabled, enabled}

	setupRuleRuntime(
		t,
		map[string]C.Proxy{"DIRECT": directTestProxy{}},
		rules,
		nil,
	)

	proxy, rule, err := match(
		&C.Metadata{NetWork: C.TCP, Host: "target.example.com"},
		loadRuntime(),
		false,
		false,
	)
	require.NoError(t, err)
	require.Equal(t, "DIRECT", proxy.Name())
	require.Equal(t, C.Rule(enabled), rule)
}

func TestMatchUsesSubRuleExactDomainPrefix(t *testing.T) {
	const ruleCount = 10_000
	var calls atomic.Int32
	rules := make([]C.Rule, 0, ruleCount)
	for index := 0; index < ruleCount; index++ {
		domain := fmt.Sprintf("sub-%d.example.com", index)
		if index == 5_000 {
			domain = "sub-target.example.com"
		}
		rules = append(rules, &countingDomainRule{
			Rule:  rulesCommon.NewDomain(domain, "DIRECT"),
			calls: &calls,
		})
	}

	setupRuleRuntime(
		t,
		map[string]C.Proxy{"DIRECT": directTestProxy{}},
		nil,
		map[string][]C.Rule{"sub": rules},
	)

	proxy, _, err := match(
		&C.Metadata{
			NetWork:      C.TCP,
			Host:         "sub-target.example.com",
			SpecialRules: "sub",
		},
		loadRuntime(),
		false,
		false,
	)
	require.NoError(t, err)
	require.Equal(t, "DIRECT", proxy.Name())
	require.LessOrEqual(t, calls.Load(), int32(2))
}

func TestMatchIndexesExactDomainSegmentsAroundOtherRules(t *testing.T) {
	const segmentSize = 1_000
	var calls atomic.Int32
	rules := make([]C.Rule, 0, 2*segmentSize+1)
	for index := 0; index < segmentSize; index++ {
		rules = append(rules, &countingDomainRule{
			Rule: rulesCommon.NewDomain(
				fmt.Sprintf("first-%d.example.com", index),
				"DIRECT",
			),
			calls: &calls,
		})
	}
	network, err := rulesCommon.NewNetworkType("udp", "DIRECT")
	require.NoError(t, err)
	rules = append(rules, &countingDomainRule{Rule: network, calls: &calls})
	for index := 0; index < segmentSize; index++ {
		domain := fmt.Sprintf("second-%d.example.com", index)
		if index == segmentSize/2 {
			domain = "second-target.example.com"
		}
		rules = append(rules, &countingDomainRule{
			Rule:  rulesCommon.NewDomain(domain, "DIRECT"),
			calls: &calls,
		})
	}

	setupRuleRuntime(
		t,
		map[string]C.Proxy{"DIRECT": directTestProxy{}},
		rules,
		nil,
	)

	proxy, _, err := match(
		&C.Metadata{NetWork: C.TCP, Host: "second-target.example.com"},
		loadRuntime(),
		false,
		false,
	)
	require.NoError(t, err)
	require.Equal(t, "DIRECT", proxy.Name())
	require.LessOrEqual(
		t,
		calls.Load(),
		int32(2),
		"indexed segments scanned or the intervening rule was skipped",
	)
}

func TestMatchFallsThroughRepeatedDisabledIndexedRules(t *testing.T) {
	var rules []C.Rule
	for index := 0; index < 3; index++ {
		rule := wrapper.NewRuleWrapper(
			rulesCommon.NewDomain("target.example.com", "DIRECT"),
		)
		if index < 2 {
			rule.SetDisabled(true)
		}
		rules = append(rules, rule)
	}
	setupRuleRuntime(
		t,
		map[string]C.Proxy{"DIRECT": directTestProxy{}},
		rules,
		nil,
	)

	proxy, rule, err := match(
		&C.Metadata{NetWork: C.TCP, Host: "target.example.com"},
		loadRuntime(),
		false,
		false,
	)
	require.NoError(t, err)
	require.Equal(t, "DIRECT", proxy.Name())
	require.Equal(t, C.Rule(rules[2]), rule)
}

func TestMatchContinuesAfterAnIndexedPassRule(t *testing.T) {
	network, err := rulesCommon.NewNetworkType("tcp", "DIRECT")
	require.NoError(t, err)
	rules := []C.Rule{
		rulesCommon.NewDomain("target.example.com", "PASS"),
		network,
	}
	setupRuleRuntime(
		t,
		map[string]C.Proxy{
			"DIRECT": directTestProxy{},
			"PASS":   passThroughTestProxy{},
		},
		rules,
		nil,
	)

	proxy, rule, err := match(
		&C.Metadata{NetWork: C.TCP, Host: "target.example.com"},
		loadRuntime(),
		false,
		false,
	)
	require.NoError(t, err)
	require.Equal(t, "DIRECT", proxy.Name())
	require.Equal(t, C.Rule(network), rule)
}

func TestMatchDoesNotScanADomainSuffixRun(t *testing.T) {
	const ruleCount = 10_000
	const target = 5_000
	var calls atomic.Int32
	rules := make([]C.Rule, 0, ruleCount)
	for index := 0; index < ruleCount; index++ {
		suffix := fmt.Sprintf("suffix-%d.example.com", index)
		if index == target {
			suffix = "target.example.com"
		}
		rules = append(rules, &countingDomainRule{
			Rule:  rulesCommon.NewDomainSuffix(suffix, "DIRECT"),
			calls: &calls,
		})
	}
	setupRuleRuntime(
		t,
		map[string]C.Proxy{"DIRECT": directTestProxy{}},
		rules,
		nil,
	)

	proxy, _, err := match(
		&C.Metadata{
			NetWork: C.TCP,
			Host:    "www.target.example.com",
		},
		loadRuntime(),
		false,
		false,
	)
	require.NoError(t, err)
	require.Equal(t, "DIRECT", proxy.Name())
	require.LessOrEqual(
		t,
		calls.Load(),
		int32(2),
		"domain-suffix match scanned the rule run",
	)
}

func TestMatchPreservesOrderAcrossOverlappingDomainRules(t *testing.T) {
	rules := []C.Rule{
		rulesCommon.NewDomainSuffix("target.example.com", "DIRECT"),
		rulesCommon.NewDomain("target.example.com", "DIRECT"),
		rulesCommon.NewDomainSuffix("example.com", "DIRECT"),
	}
	setupRuleRuntime(
		t,
		map[string]C.Proxy{"DIRECT": directTestProxy{}},
		rules,
		nil,
	)

	proxy, rule, err := match(
		&C.Metadata{NetWork: C.TCP, Host: "target.example.com"},
		loadRuntime(),
		false,
		false,
	)
	require.NoError(t, err)
	require.Equal(t, "DIRECT", proxy.Name())
	require.Equal(t, C.Rule(rules[0]), rule)
}

type countingDomainRule struct {
	C.Rule
	calls *atomic.Int32
}

func (rule *countingDomainRule) Match(
	metadata *C.Metadata,
	helper C.RuleMatchHelper,
) (bool, string) {
	rule.calls.Add(1)
	return rule.Rule.Match(metadata, helper)
}

func (rule *countingDomainRule) Unwrap() C.Rule {
	return rule.Rule
}

type countingIPRule struct {
	C.Rule
	calls *atomic.Int32
}

func (rule *countingIPRule) Match(
	metadata *C.Metadata,
	helper C.RuleMatchHelper,
) (bool, string) {
	rule.calls.Add(1)
	return rule.Rule.Match(metadata, helper)
}

func (rule *countingIPRule) Unwrap() C.Rule {
	return rule.Rule
}

type passThroughTestProxy struct {
	C.Proxy
}

func (passThroughTestProxy) Name() string {
	return "PASS"
}

func (passThroughTestProxy) Type() C.AdapterType {
	return C.Pass
}

func (passThroughTestProxy) Adapter() C.ProxyAdapter {
	return nil
}

func (passThroughTestProxy) Unwrap(*C.Metadata, bool) C.Proxy {
	return nil
}
