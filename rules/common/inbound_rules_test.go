package common

import (
	"net/netip"
	"runtime"
	"testing"

	C "github.com/metacubex/mihomo/constant"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type countingFindProcess struct {
	calls int
}

func (c *countingFindProcess) FindProcess() {
	c.calls++
}

func (c *countingFindProcess) ResolveIP() {}

func (c *countingFindProcess) CheckPassRule(string) bool { return false }

func (c *countingFindProcess) helper() C.RuleMatchHelper {
	return C.RuleMatchHelper{Context: c}
}

func TestInboundNameRulesMatchTheirField(test *testing.T) {
	tests := []struct {
		name     string
		build    func(payload, adapter string) (C.Rule, error)
		field    func(metadata *C.Metadata, value string)
		ruleType C.RuleType
	}{
		{"in name", func(payload, adapter string) (C.Rule, error) { return NewInName(payload, adapter) }, func(metadata *C.Metadata, value string) { metadata.InName = value }, C.InName},
		{"in user", func(payload, adapter string) (C.Rule, error) { return NewInUser(payload, adapter) }, func(metadata *C.Metadata, value string) { metadata.InUser = value }, C.InUser},
		{"rematch name", func(payload, adapter string) (C.Rule, error) { return NewRematchName(payload, adapter) }, func(metadata *C.Metadata, value string) { metadata.RematchName = value }, C.RematchName},
	}
	for _, testCase := range tests {
		test.Run(testCase.name, func(test *testing.T) {
			rule, err := testCase.build("first / second", "DIRECT")
			require.NoError(test, err)
			assert.Equal(test, testCase.ruleType, rule.RuleType())
			assert.Equal(test, "first / second", rule.Payload())
			assert.Equal(test, "DIRECT", rule.Adapter())

			for _, name := range []string{"first", "second"} {
				metadata := &C.Metadata{}
				testCase.field(metadata, name)
				matched, adapter := rule.Match(metadata, C.RuleMatchHelper{})
				assert.True(test, matched, name)
				assert.Equal(test, "DIRECT", adapter)
			}

			metadata := &C.Metadata{}
			testCase.field(metadata, "third")
			matched, _ := rule.Match(metadata, C.RuleMatchHelper{})
			assert.False(test, matched)

			_, err = testCase.build("first/", "DIRECT")
			require.Error(test, err)
		})
	}
}

func TestInTypeRuleExpandsSocks(test *testing.T) {
	rule, err := NewInType("http/socks", "DIRECT")
	require.NoError(test, err)
	assert.Equal(test, "HTTP/SOCKS", rule.Payload())

	for _, tp := range []C.Type{C.HTTP, C.SOCKS4, C.SOCKS5} {
		metadata := &C.Metadata{Type: tp}
		matched, adapter := rule.Match(metadata, C.RuleMatchHelper{})
		assert.True(test, matched, tp.String())
		assert.Equal(test, "DIRECT", adapter)
	}

	metadata := &C.Metadata{Type: C.REDIR}
	matched, _ := rule.Match(metadata, C.RuleMatchHelper{})
	assert.False(test, matched)

	_, err = NewInType("http//socks", "DIRECT")
	require.Error(test, err)
	_, err = NewInType("bogus", "DIRECT")
	require.Error(test, err)
}

func TestProcessRuleTargetsNameOrPath(test *testing.T) {
	finder := &countingFindProcess{}

	name, err := NewProcess("curl", "DIRECT", C.ProcessName)
	require.NoError(test, err)
	matched, adapter := name.Match(&C.Metadata{Process: "cUrL", ProcessPath: "/usr/bin/other"}, finder.helper())
	assert.True(test, matched)
	assert.Equal(test, "DIRECT", adapter)
	assert.Equal(test, 1, finder.calls)
	matched, _ = name.Match(&C.Metadata{Process: "wget"}, finder.helper())
	assert.False(test, matched)
	// the default branch matches the name, never the path
	matched, _ = name.Match(&C.Metadata{ProcessPath: "/usr/bin/curl"}, finder.helper())
	assert.False(test, matched)

	path, err := NewProcess("/usr/bin/.*", "DIRECT", C.ProcessPathRegex)
	require.NoError(test, err)
	matched, _ = path.Match(&C.Metadata{Process: "curl", ProcessPath: "/usr/bin/curl"}, C.RuleMatchHelper{})
	assert.True(test, matched)
	matched, _ = path.Match(&C.Metadata{ProcessPath: "/bin/curl"}, C.RuleMatchHelper{})
	assert.False(test, matched)

	wildcard, err := NewProcess("*cUrL*", "DIRECT", C.ProcessNameWildcard)
	require.NoError(test, err)
	matched, _ = wildcard.Match(&C.Metadata{Process: "myCurlApp"}, C.RuleMatchHelper{})
	assert.True(test, matched)
	matched, _ = wildcard.Match(&C.Metadata{Process: "wget"}, C.RuleMatchHelper{})
	assert.False(test, matched)

	_, err = NewProcess("(", "DIRECT", C.ProcessNameRegex)
	require.Error(test, err)
}

func TestIPSuffixMatchesTheTrailingBits(test *testing.T) {
	rule, err := NewIPSuffix("8.8.8.8/24", "DIRECT", false, false)
	require.NoError(test, err)
	assert.Equal(test, C.IPSuffix, rule.RuleType())
	assert.Equal(test, "8.8.8.8/24", rule.Payload())

	for _, ip := range []string{"8.8.8.8", "1.8.8.8"} {
		matched, _ := rule.Match(&C.Metadata{DstIP: netip.MustParseAddr(ip)}, C.RuleMatchHelper{})
		assert.True(test, matched, ip)
	}
	for _, ip := range []string{"8.8.8.7", "1.2.3.4"} {
		matched, _ := rule.Match(&C.Metadata{DstIP: netip.MustParseAddr(ip)}, C.RuleMatchHelper{})
		assert.False(test, matched, ip)
	}
	// a different address family has no trailing bits to compare
	matched, _ := rule.Match(&C.Metadata{DstIP: netip.MustParseAddr("::1")}, C.RuleMatchHelper{})
	assert.False(test, matched)

	partial, err := NewIPSuffix("8.8.0.0/20", "DIRECT", false, false)
	require.NoError(test, err)
	matched, _ = partial.Match(&C.Metadata{DstIP: netip.MustParseAddr("1.8.0.0")}, C.RuleMatchHelper{})
	assert.True(test, matched)
	matched, _ = partial.Match(&C.Metadata{DstIP: netip.MustParseAddr("1.9.0.0")}, C.RuleMatchHelper{})
	assert.False(test, matched)

	source, err := NewIPSuffix("8.8.8.8/24", "DIRECT", true, false)
	require.NoError(test, err)
	assert.Equal(test, C.SrcIPSuffix, source.RuleType())
	matched, _ = source.Match(&C.Metadata{SrcIP: netip.MustParseAddr("1.8.8.8")}, C.RuleMatchHelper{})
	assert.True(test, matched)
	matched, _ = source.Match(&C.Metadata{DstIP: netip.MustParseAddr("1.8.8.8")}, C.RuleMatchHelper{})
	assert.False(test, matched)

	_, err = NewIPSuffix("not-a-prefix", "DIRECT", false, false)
	require.Error(test, err)
}

func TestIPSuffixHonorsNoResolve(test *testing.T) {
	resolver := &countingResolve{}

	rule, err := NewIPSuffix("8.8.8.8/24", "DIRECT", false, false)
	require.NoError(test, err)
	rule.Match(&C.Metadata{}, resolver.helper())
	assert.Equal(test, 1, resolver.calls)

	noResolve, err := NewIPSuffix("8.8.8.8/24", "DIRECT", false, true)
	require.NoError(test, err)
	noResolve.Match(&C.Metadata{}, resolver.helper())
	assert.Equal(test, 1, resolver.calls)
}

func TestUidRulePlatformGate(test *testing.T) {
	rule, err := NewUid("1000-2000", "DIRECT")
	if runtime.GOOS != "linux" && runtime.GOOS != "android" {
		require.Error(test, err)
		return
	}
	require.NoError(test, err)
	assert.Equal(test, C.Uid, rule.RuleType())
	assert.Equal(test, "1000-2000", rule.Payload())

	matched, adapter := rule.Match(&C.Metadata{Uid: 1500}, C.RuleMatchHelper{})
	assert.True(test, matched)
	assert.Equal(test, "DIRECT", adapter)
	matched, _ = rule.Match(&C.Metadata{Uid: 3000}, C.RuleMatchHelper{})
	assert.False(test, matched)

	_, err = NewUid("", "DIRECT")
	require.Error(test, err)
}
