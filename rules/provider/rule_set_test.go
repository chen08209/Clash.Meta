package provider

import (
	"net/netip"
	"testing"

	C "github.com/metacubex/mihomo/constant"
	P "github.com/metacubex/mihomo/constant/provider"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// registryTunnel answers the rule-provider lookup a RuleSet performs through
// the injected P.Tunnel, which the package keeps in its own global.
type registryTunnel struct {
	stubTunnel
	ruleProviders map[string]P.RuleProvider
}

type resolvingContext struct{}

func (resolvingContext) ResolveIP() {}

func (resolvingContext) FindProcess() {}

func (resolvingContext) CheckPassRule(string) bool { return false }

func (t *registryTunnel) RuleProviders() map[string]P.RuleProvider {
	return t.ruleProviders
}

func registerRuleProviders(test *testing.T, providers map[string]P.RuleProvider) {
	test.Helper()
	original := tunnel
	tunnel = &registryTunnel{ruleProviders: providers}
	test.Cleanup(func() { tunnel = original })
}

func domainRuleProvider(name string) P.RuleProvider {
	return NewInlineProvider(name, P.Domain, []string{"example.com", ".example.org"}, nil)
}

func ipcidrRuleProvider(name string) P.RuleProvider {
	return NewInlineProvider(name, P.IPCIDR, []string{"10.0.0.0/8"}, nil)
}

func TestRuleSetMatchesTheRegisteredProvider(test *testing.T) {
	registerRuleProviders(test, map[string]P.RuleProvider{"test-domain": domainRuleProvider("test-domain")})

	ruleSet, err := NewRuleSet("test-domain", "TestAdapter", false, false)
	require.NoError(test, err)

	assert.Equal(test, C.RuleSet, ruleSet.RuleType())
	assert.Equal(test, "TestAdapter", ruleSet.Adapter())
	assert.Equal(test, "test-domain", ruleSet.Payload())
	assert.Equal(test, []string{"test-domain"}, ruleSet.ProviderNames())

	assert.True(test, ruleSet.MatchDomain("example.com"))
	assert.True(test, ruleSet.MatchDomain("www.example.org"))
	assert.False(test, ruleSet.MatchDomain("example.net"))

	assert.False(test, ruleSet.MatchIp(netip.MustParseAddr("10.0.0.1")))
}

func TestRuleSetMatchesTheRegisteredProviderByIp(test *testing.T) {
	registerRuleProviders(test, map[string]P.RuleProvider{"test-ipcidr": ipcidrRuleProvider("test-ipcidr")})

	ruleSet, err := NewRuleSet("test-ipcidr", "TestAdapter", false, false)
	require.NoError(test, err)

	assert.True(test, ruleSet.MatchIp(netip.MustParseAddr("10.1.2.3")))
	assert.False(test, ruleSet.MatchIp(netip.MustParseAddr("11.1.2.3")))
	assert.False(test, ruleSet.MatchDomain("10.1.2.3"))
}

func TestRuleSetSrcModeSwapsTheDirection(test *testing.T) {
	registerRuleProviders(test, map[string]P.RuleProvider{"test-ipcidr": ipcidrRuleProvider("test-ipcidr")})

	metadata := &C.Metadata{
		SrcIP: netip.MustParseAddr("192.168.1.1"),
		DstIP: netip.MustParseAddr("10.0.0.1"),
	}

	dstRule, err := NewRuleSet("test-ipcidr", "TestAdapter", false, false)
	require.NoError(test, err)
	matched, adapter := dstRule.Match(metadata, C.RuleMatchHelper{})
	assert.True(test, matched)
	assert.Equal(test, "TestAdapter", adapter)

	srcRule, err := NewRuleSet("test-ipcidr", "TestAdapter", true, false)
	require.NoError(test, err)
	matched, adapter = srcRule.Match(metadata, C.RuleMatchHelper{})
	assert.False(test, matched)
	// the adapter travels with the rule regardless of the outcome
	assert.Equal(test, "TestAdapter", adapter)

	// the swap is undone after the match, so the caller keeps its metadata
	assert.Equal(test, netip.MustParseAddr("192.168.1.1"), metadata.SrcIP)
	assert.Equal(test, netip.MustParseAddr("10.0.0.1"), metadata.DstIP)
}

func TestRuleSetResolvesIpUnlessToldOtherwise(test *testing.T) {
	registerRuleProviders(test, map[string]P.RuleProvider{"test-ipcidr": ipcidrRuleProvider("test-ipcidr")})

	resolving := C.RuleMatchHelper{Context: resolvingContext{}}

	resolveRule, err := NewRuleSet("test-ipcidr", "TestAdapter", false, false)
	require.NoError(test, err)
	matched, _ := resolveRule.Match(&C.Metadata{DstIP: netip.MustParseAddr("10.0.0.1")}, resolving)
	assert.True(test, matched)

	noResolveRule, err := NewRuleSet("test-ipcidr", "TestAdapter", false, true)
	require.NoError(test, err)
	matched, _ = noResolveRule.Match(&C.Metadata{}, resolving)
	assert.False(test, matched)
}

func TestRuleSetWithoutARegisteredProviderNeverMatches(test *testing.T) {
	registerRuleProviders(test, nil)

	ruleSet, err := NewRuleSet("absent-provider", "TestAdapter", false, false)
	require.NoError(test, err)

	assert.False(test, ruleSet.MatchDomain("example.com"))
	assert.False(test, ruleSet.MatchIp(netip.MustParseAddr("10.0.0.1")))
	matched, adapter := ruleSet.Match(&C.Metadata{Host: "example.com"}, C.RuleMatchHelper{})
	assert.False(test, matched)
	assert.Empty(test, adapter)
}
