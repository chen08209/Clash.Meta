package common

import (
	"net/netip"
	"testing"

	C "github.com/metacubex/mihomo/constant"
	"github.com/stretchr/testify/require"
)

type countingResolve struct {
	calls int
}

func (c *countingResolve) ResolveIP() {
	c.calls++
}

func (c *countingResolve) FindProcess() {}

func (c *countingResolve) CheckPassRule(string) bool { return false }

func (c *countingResolve) helper() C.RuleMatchHelper {
	return C.RuleMatchHelper{Context: c}
}

func TestDomainMatcherSemantics(t *testing.T) {
	rule := NewDomain("Example.com", "DIRECT")
	// The rule payload is lowercased, and matching compares the raw rule host
	// byte for byte: an uppercase connection host does not match.
	matched, adapter := rule.Match(&C.Metadata{Host: "example.com"}, C.RuleMatchHelper{})
	require.True(t, matched)
	require.Equal(t, "DIRECT", adapter)
	matched, _ = rule.Match(&C.Metadata{Host: "WWW.example.com"}, C.RuleMatchHelper{})
	require.False(t, matched)
	// A sniffed host wins over the original one.
	matched, _ = rule.Match(&C.Metadata{Host: "other.com", SniffHost: "example.com"}, C.RuleMatchHelper{})
	require.True(t, matched)
}

func TestDomainKeywordAndWildcard(t *testing.T) {
	keyword := NewDomainKeyword("google", "DIRECT")
	matched, _ := keyword.Match(&C.Metadata{Host: "www.google.com"}, C.RuleMatchHelper{})
	require.True(t, matched)
	matched, _ = keyword.Match(&C.Metadata{Host: "google.cn"}, C.RuleMatchHelper{})
	require.True(t, matched)
	matched, _ = keyword.Match(&C.Metadata{Host: "example.com"}, C.RuleMatchHelper{})
	require.False(t, matched)

	wildcard, err := NewDomainWildcard("*.example.com", "DIRECT")
	require.NoError(t, err)
	matched, _ = wildcard.Match(&C.Metadata{Host: "a.example.com"}, C.RuleMatchHelper{})
	require.True(t, matched)
	matched, _ = wildcard.Match(&C.Metadata{Host: "example.com"}, C.RuleMatchHelper{})
	require.False(t, matched)
}

func TestDomainRegexMatch(t *testing.T) {
	rule, err := NewDomainRegex(`^cdn[0-9]+\.example\.com$`, "DIRECT")
	require.NoError(t, err)
	matched, _ := rule.Match(&C.Metadata{Host: "cdn12.example.com"}, C.RuleMatchHelper{})
	require.True(t, matched)
	matched, _ = rule.Match(&C.Metadata{Host: "cdn.example.com"}, C.RuleMatchHelper{})
	require.False(t, matched)
}

func TestPortMatchTargetsEachDirection(t *testing.T) {
	port, err := NewPort("80,443,8000-9000", "DIRECT", C.DstPort)
	require.NoError(t, err)
	matched, _ := port.Match(&C.Metadata{DstPort: 443}, C.RuleMatchHelper{})
	require.True(t, matched)
	matched, _ = port.Match(&C.Metadata{DstPort: 8443}, C.RuleMatchHelper{})
	require.True(t, matched)
	matched, _ = port.Match(&C.Metadata{DstPort: 9001}, C.RuleMatchHelper{})
	require.False(t, matched)

	srcPort, err := NewPort("7777", "DIRECT", C.SrcPort)
	require.NoError(t, err)
	matched, _ = srcPort.Match(&C.Metadata{DstPort: 80, SrcPort: 7777}, C.RuleMatchHelper{})
	require.True(t, matched)
	matched, _ = srcPort.Match(&C.Metadata{DstPort: 80}, C.RuleMatchHelper{})
	require.False(t, matched)

	inPort, err := NewPort("1080", "DIRECT", C.InPort)
	require.NoError(t, err)
	matched, _ = inPort.Match(&C.Metadata{DstPort: 80, InPort: 1080}, C.RuleMatchHelper{})
	require.True(t, matched)
	matched, _ = inPort.Match(&C.Metadata{DstPort: 80}, C.RuleMatchHelper{})
	require.False(t, matched)
}

func TestNetworkTypeMatch(t *testing.T) {
	tcp, err := NewNetworkType("tcp", "DIRECT")
	require.NoError(t, err)
	matched, _ := tcp.Match(&C.Metadata{NetWork: C.TCP}, C.RuleMatchHelper{})
	require.True(t, matched)
	matched, _ = tcp.Match(&C.Metadata{NetWork: C.UDP}, C.RuleMatchHelper{})
	require.False(t, matched)

	_, err = NewNetworkType("icmp", "DIRECT")
	require.Error(t, err)
}

func TestMatchRuleAlwaysMatches(t *testing.T) {
	final := &Match{adapter: "DIRECT"}
	matched, adapter := final.Match(&C.Metadata{}, C.RuleMatchHelper{})
	require.True(t, matched)
	require.Equal(t, "DIRECT", adapter)
	require.Empty(t, final.Payload())
}

func TestIPCIDRMatchAndResolveHelper(t *testing.T) {
	resolver := &countingResolve{}
	dstRule, err := NewIPCIDR("192.168.0.0/16", "DIRECT")
	require.NoError(t, err)
	matched, _ := dstRule.Match(&C.Metadata{DstIP: netip.MustParseAddr("192.168.1.2")}, resolver.helper())
	require.True(t, matched)
	require.Equal(t, 1, resolver.calls, "a dst cidr without no-resolve resolves first")

	matched, _ = dstRule.Match(&C.Metadata{DstIP: netip.MustParseAddr("10.0.0.1")}, resolver.helper())
	require.False(t, matched)

	srcRule, err := NewIPCIDR("192.168.0.0/16", "DIRECT", WithIPCIDRSourceIP(true), WithIPCIDRNoResolve(true))
	require.NoError(t, err)
	require.Equal(t, C.SrcIPCIDR, srcRule.RuleType())
	matched, _ = srcRule.Match(&C.Metadata{DstIP: netip.MustParseAddr("192.168.1.2"), SrcIP: netip.MustParseAddr("10.0.0.1")}, resolver.helper())
	require.False(t, matched, "a source rule must not look at the destination")
	before := resolver.calls
	matched, _ = srcRule.Match(&C.Metadata{SrcIP: netip.MustParseAddr("192.168.1.2")}, resolver.helper())
	require.True(t, matched)
	require.Equal(t, before, resolver.calls, "a source rule never triggers the resolve helper")
}

func TestDSCPMatch(t *testing.T) {
	dscp, err := NewDSCP("46,10-20", "DIRECT")
	require.NoError(t, err)
	matched, _ := dscp.Match(&C.Metadata{DSCP: 46}, C.RuleMatchHelper{})
	require.True(t, matched)
	matched, _ = dscp.Match(&C.Metadata{DSCP: 15}, C.RuleMatchHelper{})
	require.True(t, matched)
	matched, _ = dscp.Match(&C.Metadata{DSCP: 63}, C.RuleMatchHelper{})
	require.False(t, matched)

	_, err = NewDSCP("64", "DIRECT")
	require.Error(t, err, "dscp values above 63 are invalid")
}
