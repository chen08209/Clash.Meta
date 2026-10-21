package rules

import (
	"runtime"
	"testing"

	C "github.com/metacubex/mihomo/constant"
	"github.com/stretchr/testify/require"
)

func TestParseRuleDispatch(t *testing.T) {
	cases := []struct {
		tp         string
		payload    string
		target     string
		params     []string
		ruleType   C.RuleType
		adapter    string
		payloadOut string
	}{
		{tp: "DOMAIN", payload: "Example.com", target: "DIRECT", ruleType: C.Domain, payloadOut: "example.com"},
		{tp: "DOMAIN-SUFFIX", payload: "Example.com", target: "PROXY", ruleType: C.DomainSuffix, payloadOut: "example.com"},
		{tp: "DOMAIN-KEYWORD", payload: "Google", target: "PROXY", ruleType: C.DomainKeyword, payloadOut: "google"},
		{tp: "DOMAIN-WILDCARD", payload: "*.Example.com", target: "PROXY", ruleType: C.DomainWildcard, payloadOut: "*.example.com"},
		{tp: "IP-CIDR", payload: "10.0.0.0/8", target: "DIRECT", ruleType: C.IPCIDR},
		{tp: "IP-CIDR", payload: "10.0.0.0/8", target: "DIRECT", params: []string{"src"}, ruleType: C.SrcIPCIDR},
		{tp: "IP-CIDR6", payload: "fd00::/8", target: "DIRECT", ruleType: C.IPCIDR},
		{tp: "SRC-IP-CIDR", payload: "10.0.0.0/8", target: "DIRECT", ruleType: C.SrcIPCIDR},
		{tp: "IP-SUFFIX", payload: "10.0.0.0/8", target: "DIRECT", ruleType: C.IPSuffix},
		{tp: "SRC-IP-SUFFIX", payload: "10.0.0.0/8", target: "DIRECT", ruleType: C.SrcIPSuffix},
		{tp: "SRC-PORT", payload: "7777", target: "DIRECT", ruleType: C.SrcPort},
		{tp: "DST-PORT", payload: "443", target: "DIRECT", ruleType: C.DstPort},
		{tp: "IN-PORT", payload: "1080", target: "DIRECT", ruleType: C.InPort},
		{tp: "DSCP", payload: "46", target: "DIRECT", ruleType: C.DSCP},
		{tp: "PROCESS-NAME", payload: "chrome.exe", target: "DIRECT", ruleType: C.ProcessName},
		{tp: "PROCESS-PATH", payload: "C:\\chrome.exe", target: "DIRECT", ruleType: C.ProcessPath},
		{tp: "PROCESS-NAME-REGEX", payload: "^chrome", target: "DIRECT", ruleType: C.ProcessNameRegex},
		{tp: "PROCESS-PATH-WILDCARD", payload: "C:\\*", target: "DIRECT", ruleType: C.ProcessPathWildcard},
		{tp: "NETWORK", payload: "tcp", target: "DIRECT", ruleType: C.Network},
		{tp: "IN-TYPE", payload: "SOCKS/HTTP", target: "DIRECT", ruleType: C.InType},
		{tp: "IN-USER", payload: "mihomo", target: "DIRECT", ruleType: C.InUser},
		{tp: "IN-NAME", payload: "DEFAULT-SOCKS", target: "DIRECT", ruleType: C.InName},
		{tp: "REMATCH-NAME", payload: "sub-rule", target: "DIRECT", ruleType: C.RematchName},
	}

	for _, tc := range cases {
		parsed, err := ParseRule(tc.tp, tc.payload, tc.target, tc.params, nil)
		require.NoError(t, err, tc.tp)
		require.Equal(t, tc.ruleType, parsed.RuleType(), tc.tp)
		require.Equal(t, tc.target, parsed.Adapter(), tc.tp)
		if tc.payloadOut != "" {
			require.Equal(t, tc.payloadOut, parsed.Payload(), tc.tp)
		}
	}
}

func TestParseRuleMatchAllowsEmptyPayload(t *testing.T) {
	parsed, err := ParseRule("MATCH", "", "DIRECT", nil, nil)
	require.NoError(t, err)
	require.Equal(t, C.MATCH, parsed.RuleType())
	require.Equal(t, "DIRECT", parsed.Adapter())
}

func TestParseRuleErrors(t *testing.T) {
	_, err := ParseRule("DOMAIN", "", "DIRECT", nil, nil)
	require.ErrorContains(t, err, "missing subsequent parameters")

	_, err = ParseRule("DOMAIN-REGEX", "([", "DIRECT", nil, nil)
	require.Error(t, err, "an invalid regex must surface")

	_, err = ParseRule("IP-CIDR", "not-a-prefix", "DIRECT", nil, nil)
	require.Error(t, err)

	_, err = ParseRule("DSCP", "64", "DIRECT", nil, nil)
	require.Error(t, err)

	_, err = ParseRule("NOT-A-TYPE", "payload", "DIRECT", nil, nil)
	require.ErrorContains(t, err, "unsupported rule type")
}

func TestParseRuleUidPlatform(t *testing.T) {
	parsed, err := ParseRule("UID", "1000", "DIRECT", nil, nil)
	if runtime.GOOS == "linux" || runtime.GOOS == "android" {
		require.NoError(t, err)
		require.Equal(t, C.Uid, parsed.RuleType())
	} else {
		require.Error(t, err, "uid rules are platform gated")
		require.Nil(t, parsed)
	}
}

func TestParseRuleLogicDelegation(t *testing.T) {
	and, err := ParseRule("AND", "((DOMAIN,example.com),(DST-PORT,443))", "DIRECT", nil, nil)
	require.NoError(t, err)
	require.Equal(t, C.AND, and.RuleType())

	not, err := ParseRule("NOT", "((DOMAIN,example.com))", "DIRECT", nil, nil)
	require.NoError(t, err)
	require.Equal(t, C.NOT, not.RuleType())

	subRules := map[string][]C.Rule{
		"inner": {mustRule(t, "DOMAIN", "example.com", "DIRECT")},
	}
	sub, err := ParseRule("SUB-RULE", "(DOMAIN,example.com)", "inner", nil, subRules)
	require.NoError(t, err)
	require.Equal(t, C.SubRules, sub.RuleType())
	require.Empty(t, sub.ProviderNames(), "plain domain rules carry no providers")
}

func mustRule(t *testing.T, tp, payload, target string) C.Rule {
	t.Helper()
	parsed, err := ParseRule(tp, payload, target, nil, nil)
	require.NoError(t, err)
	return parsed
}
