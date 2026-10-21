package common

import (
	"net/netip"
	"testing"

	"github.com/metacubex/mihomo/component/geodata"
	C "github.com/metacubex/mihomo/constant"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The lan country is computed in code, so its rule needs no geo database.
func TestGeoIPLanRule(test *testing.T) {
	rule, err := NewGEOIP("LAN", "DIRECT", false, false)
	require.NoError(test, err)
	assert.Equal(test, C.GEOIP, rule.RuleType())
	assert.Equal(test, "lan", rule.Payload())
	assert.Equal(test, "DIRECT", rule.Adapter())
	assert.Equal(test, "lan", rule.GetCountry())
	assert.Equal(test, 0, rule.GetRecodeSize())
	_, err = rule.GetIPMatcher()
	require.Error(test, err) // not geodata mode

	for _, ip := range []string{"127.0.0.1", "10.0.0.1", "192.168.1.1", "fe80::1", "ff02::1"} {
		matched, adapter := rule.Match(&C.Metadata{DstIP: netip.MustParseAddr(ip)}, C.RuleMatchHelper{})
		assert.True(test, matched, ip)
		assert.Equal(test, "DIRECT", adapter)
		assert.True(test, rule.MatchIp(netip.MustParseAddr(ip)), ip)
	}
	for _, ip := range []string{"8.8.8.8", "1.2.3.4", "2001:db8::1"} {
		matched, _ := rule.Match(&C.Metadata{DstIP: netip.MustParseAddr(ip)}, C.RuleMatchHelper{})
		assert.False(test, matched, ip)
		assert.False(test, rule.MatchIp(netip.MustParseAddr(ip)), ip)
	}

	// an unresolved address never matches, and the resolve helper is asked first
	resolver := &countingResolve{}
	matched, _ := rule.Match(&C.Metadata{}, resolver.helper())
	assert.False(test, matched)
	assert.Equal(test, 1, resolver.calls)

	// no-resolve leaves the helper alone but still resolves nothing
	noResolve, err := NewGEOIP("lan", "DIRECT", false, true)
	require.NoError(test, err)
	noResolve.Match(&C.Metadata{}, resolver.helper())
	assert.Equal(test, 1, resolver.calls)
}

func TestGeoIPLanRuleMatchesTheSourceAddress(test *testing.T) {
	rule, err := NewGEOIP("lan", "DIRECT", true, false)
	require.NoError(test, err)
	assert.Equal(test, C.SrcGEOIP, rule.RuleType())

	matched, _ := rule.Match(&C.Metadata{SrcIP: netip.MustParseAddr("10.0.0.1")}, C.RuleMatchHelper{})
	assert.True(test, matched)
	matched, _ = rule.Match(&C.Metadata{DstIP: netip.MustParseAddr("10.0.0.1")}, C.RuleMatchHelper{})
	assert.False(test, matched)
}

// dns.fallback-filter.geoip inverts the lan rule: everything but the lan ranges
// passes the filter.
func TestGeoIPDnsFallbackFilterInvertsLan(test *testing.T) {
	rule, err := NewGEOIP("lan", "DIRECT", false, false)
	require.NoError(test, err)

	filter := rule.DnsFallbackFilter()
	assert.True(test, filter.MatchIp(netip.MustParseAddr("8.8.8.8")))
	assert.False(test, filter.MatchIp(netip.MustParseAddr("10.0.0.1")))
	assert.False(test, filter.MatchIp(netip.Addr{}))
}

func TestGeoRulesNeedTheirDatabase(test *testing.T) {
	_, err := NewGEOSITE("cn", "DIRECT")
	require.Error(test, err)
	_, err = NewIPASN("13335", "DIRECT", false, false)
	require.Error(test, err)
}

// In geodata mode the metadata carries the resolved codes, so a hit never
// reaches the matcher and needs no database.
func TestGeoIPGeodataModeUsesTheMetadataCodes(test *testing.T) {
	originalMode := geodata.GeodataMode()
	geodata.SetGeodataMode(true)
	test.Cleanup(func() { geodata.SetGeodataMode(originalMode) })

	rule := &GEOIP{Base: Base{}, country: "cn", adapter: "DIRECT"}

	metadata := &C.Metadata{DstIP: netip.MustParseAddr("1.2.3.4"), DstGeoIP: []string{"cn"}}
	matched, adapter := rule.Match(metadata, C.RuleMatchHelper{})
	assert.True(test, matched)
	assert.Equal(test, "DIRECT", adapter)
	assert.Equal(test, []string{"cn"}, metadata.DstGeoIP)

	metadata = &C.Metadata{DstIP: netip.MustParseAddr("1.2.3.4")}
	matched, _ = rule.Match(metadata, C.RuleMatchHelper{})
	assert.False(test, matched)

	sourceRule := &GEOIP{Base: Base{}, country: "cn", adapter: "DIRECT", isSourceIP: true}
	matched, _ = sourceRule.Match(&C.Metadata{SrcIP: netip.MustParseAddr("1.2.3.4"), SrcGeoIP: []string{"cn"}}, C.RuleMatchHelper{})
	assert.True(test, matched)
	matched, _ = sourceRule.Match(&C.Metadata{SrcIP: netip.MustParseAddr("1.2.3.4")}, C.RuleMatchHelper{})
	assert.False(test, matched)

	// without codes the matcher cannot load, so both direct paths stay false
	assert.False(test, rule.MatchIp(netip.MustParseAddr("1.2.3.4")))
	assert.False(test, rule.DnsFallbackFilter().MatchIp(netip.MustParseAddr("1.2.3.4")))
}
