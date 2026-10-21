package provider

import (
	"bytes"
	"encoding/json"
	"net/netip"
	"testing"

	C "github.com/metacubex/mihomo/constant"
	P "github.com/metacubex/mihomo/constant/provider"
	"github.com/metacubex/mihomo/rules/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// metadataParse builds a parse function whose stub rules answer from the
// metadata itself, so the strategy under test owns every routing decision.
func metadataParse() common.ParseRuleFunc {
	return func(tp, payload, target string, params []string, subRules map[string][]C.Rule) (C.Rule, error) {
		switch tp {
		case "IP-CIDR":
			prefix, err := netip.ParsePrefix(payload)
			if err != nil {
				return nil, err
			}
			return &ipCidrStubRule{prefix: prefix}, nil
		default:
			return &domainStubRule{host: payload}, nil
		}
	}
}

type domainStubRule struct {
	host string
}

func (r *domainStubRule) RuleType() C.RuleType { return C.Domain }
func (r *domainStubRule) Match(metadata *C.Metadata, helper C.RuleMatchHelper) (bool, string) {
	return metadata.RuleHost() == r.host, r.host
}
func (r *domainStubRule) Adapter() string       { return r.host }
func (r *domainStubRule) Payload() string       { return r.host }
func (*domainStubRule) ProviderNames() []string { return nil }

type ipCidrStubRule struct {
	prefix netip.Prefix
}

func (r *ipCidrStubRule) RuleType() C.RuleType { return C.IPCIDR }
func (r *ipCidrStubRule) Match(metadata *C.Metadata, helper C.RuleMatchHelper) (bool, string) {
	return r.prefix.Contains(metadata.DstIP), r.prefix.String()
}
func (r *ipCidrStubRule) Adapter() string       { return r.prefix.String() }
func (r *ipCidrStubRule) Payload() string       { return r.prefix.String() }
func (*ipCidrStubRule) ProviderNames() []string { return nil }

func TestClassicalStrategyInsertMatchAndReset(test *testing.T) {
	strategy := NewClassicalStrategy(metadataParse())
	strategy.Insert("DOMAIN,match.com")
	strategy.Insert("DOMAIN-SUFFIX,sub.com")
	strategy.Insert("IP-CIDR,1.2.3.0/24")
	strategy.Insert("MATCH,Final")
	assert.Equal(test, 3, strategy.Count())

	strategy.FinishInsert()
	assert.True(test, strategy.Match(&C.Metadata{Host: "match.com"}, C.RuleMatchHelper{}))
	assert.True(test, strategy.Match(&C.Metadata{DstIP: netip.MustParseAddr("1.2.3.4")}, C.RuleMatchHelper{}))
	assert.False(test, strategy.Match(&C.Metadata{Host: "other.com"}, C.RuleMatchHelper{}))

	strategy.Reset()
	assert.Equal(test, 0, strategy.Count())
	assert.False(test, strategy.Match(&C.Metadata{Host: "match.com"}, C.RuleMatchHelper{}))
}

func TestClassicalStrategySkipsRulesWithoutAParser(test *testing.T) {
	strategy := NewClassicalStrategy(nil)
	strategy.Insert("DOMAIN,match.com")
	assert.Equal(test, 0, strategy.Count())

	strategy.FinishInsert()
	assert.False(test, strategy.Match(&C.Metadata{Host: "match.com"}, C.RuleMatchHelper{}))
}

func TestDomainStrategyCountResetAndInvalidInsert(test *testing.T) {
	strategy := NewDomainStrategy()
	strategy.Insert("example.com")
	strategy.Insert(".example.org")
	strategy.Insert("with/slash")
	assert.Equal(test, 2, strategy.Count())

	strategy.FinishInsert()
	assert.True(test, strategy.Match(&C.Metadata{Host: "www.example.org"}, C.RuleMatchHelper{}))

	strategy.Reset()
	assert.Equal(test, 0, strategy.Count())
	assert.False(test, strategy.Match(&C.Metadata{Host: "www.example.org"}, C.RuleMatchHelper{}))
}

func TestDomainStrategyMrsRoundTrip(test *testing.T) {
	strategy := NewDomainStrategy()
	strategy.Insert("example.com")
	strategy.Insert(".example.org")
	strategy.FinishInsert()
	assert.Equal(test, 2, strategy.Count())

	var buffer bytes.Buffer
	require.NoError(test, strategy.WriteMrs(&buffer))

	restored := NewDomainStrategy()
	require.NoError(test, restored.FromMrs(&buffer, strategy.Count()))
	assert.Equal(test, 2, restored.Count())
	for host, expected := range map[string]bool{
		"example.com":     true,
		"www.example.org": true,
		"example.org":     false,
		"example.net":     false,
	} {
		assert.Equal(test, expected, restored.Match(&C.Metadata{Host: host}, C.RuleMatchHelper{}), host)
	}

	assert.Error(test, (&domainStrategy{}).WriteMrs(&bytes.Buffer{}))
}

func TestIpcidrStrategyInsertMatchAndReset(test *testing.T) {
	strategy := NewIPCidrStrategy()
	strategy.Insert("1.2.3.0/24")
	strategy.Insert("10.0.0.0/8")
	strategy.Insert("not-a-cidr")
	assert.Equal(test, 2, strategy.Count())

	strategy.FinishInsert()
	assert.True(test, strategy.Match(&C.Metadata{DstIP: netip.MustParseAddr("1.2.3.4")}, C.RuleMatchHelper{}))
	assert.True(test, strategy.Match(&C.Metadata{DstIP: netip.MustParseAddr("10.1.2.3")}, C.RuleMatchHelper{}))
	assert.False(test, strategy.Match(&C.Metadata{DstIP: netip.MustParseAddr("192.168.1.1")}, C.RuleMatchHelper{}))

	strategy.Reset()
	assert.Equal(test, 0, strategy.Count())
	assert.False(test, strategy.Match(&C.Metadata{DstIP: netip.MustParseAddr("1.2.3.4")}, C.RuleMatchHelper{}))
}

func TestIpcidrStrategyMrsRoundTripAndDump(test *testing.T) {
	strategy := NewIPCidrStrategy()
	strategy.Insert("1.2.3.0/24")
	strategy.Insert("10.0.0.0/8")
	strategy.FinishInsert()

	var buffer bytes.Buffer
	require.NoError(test, strategy.WriteMrs(&buffer))

	restored := NewIPCidrStrategy()
	require.NoError(test, restored.FromMrs(&buffer, strategy.Count()))
	assert.Equal(test, 2, restored.Count())
	assert.True(test, restored.Match(&C.Metadata{DstIP: netip.MustParseAddr("10.9.8.7")}, C.RuleMatchHelper{}))

	var keys []string
	strategy.DumpMrs(func(key string) bool {
		keys = append(keys, key)
		return true
	})
	assert.ElementsMatch(test, []string{"1.2.3.0/24", "10.0.0.0/8"}, keys)

	ipSet := strategy.ToIpCidr()
	assert.True(test, ipSet.Contains(netip.MustParseAddr("1.2.3.200")))
	assert.False(test, ipSet.Contains(netip.MustParseAddr("11.0.0.1")))

	assert.Error(test, (&ipcidrStrategy{}).WriteMrs(&bytes.Buffer{}))
}

func TestConvertToMrsRoundTripAndTextExport(test *testing.T) {
	payload := []byte("example.com\n.example.org\nexample.net\n")

	var mrs bytes.Buffer
	require.NoError(test, ConvertToMrs(payload, P.Domain, P.TextRule, &mrs))

	strategy := NewDomainStrategy()
	restored, err := rulesMrsParse(mrs.Bytes(), strategy)
	require.NoError(test, err)
	assert.Equal(test, 3, restored.Count())
	assert.True(test, restored.Match(&C.Metadata{Host: "example.com"}, C.RuleMatchHelper{}))
	assert.True(test, restored.Match(&C.Metadata{Host: "www.example.org"}, C.RuleMatchHelper{}))
	assert.False(test, restored.Match(&C.Metadata{Host: "example.org"}, C.RuleMatchHelper{}))

	var text bytes.Buffer
	require.NoError(test, ConvertToMrs(mrs.Bytes(), P.Domain, P.MrsRule, &text))
	assert.Equal(test, ".example.org\nexample.com\nexample.net\n", text.String())

	assert.Error(test, ConvertToMrs([]byte("\n\n"), P.Domain, P.TextRule, &bytes.Buffer{}))
}

func TestInlineProviderParsesPayloadAndMatches(test *testing.T) {
	provider := NewInlineProvider("inline-domain", P.Domain, []string{"example.com", ".example.org", ""}, nil)
	assert.Equal(test, "inline-domain", provider.Name())
	assert.Equal(test, P.Inline, provider.VehicleType())
	assert.Equal(test, 2, provider.Count())

	assert.NoError(test, provider.Initial())
	assert.NoError(test, provider.Update())

	assert.True(test, provider.Match(&C.Metadata{Host: "www.example.org"}, C.RuleMatchHelper{}))
	assert.False(test, provider.Match(&C.Metadata{Host: "example.org"}, C.RuleMatchHelper{}))

	data, err := json.Marshal(provider)
	require.NoError(test, err)
	var payload struct {
		Behavior    string   `json:"behavior"`
		Name        string   `json:"name"`
		RuleCount   int      `json:"ruleCount"`
		Type        string   `json:"type"`
		VehicleType string   `json:"vehicleType"`
		Payload     []string `json:"payload"`
	}
	require.NoError(test, json.Unmarshal(data, &payload))
	assert.Equal(test, "Domain", payload.Behavior)
	assert.Equal(test, "inline-domain", payload.Name)
	assert.Equal(test, 2, payload.RuleCount)
	assert.Equal(test, "Rule", payload.Type)
	assert.Equal(test, "Inline", payload.VehicleType)
	assert.Equal(test, []string{"example.com", ".example.org", ""}, payload.Payload)
}

func TestInlineProviderIpcidrMatchesByDestination(test *testing.T) {
	provider := NewInlineProvider("inline-ipcidr", P.IPCIDR, []string{"10.0.0.0/8"}, nil)
	assert.Equal(test, 1, provider.Count())
	assert.True(test, provider.Match(&C.Metadata{DstIP: netip.MustParseAddr("10.1.2.3")}, C.RuleMatchHelper{}))
	assert.False(test, provider.Match(&C.Metadata{DstIP: netip.MustParseAddr("11.1.2.3")}, C.RuleMatchHelper{}))
}
