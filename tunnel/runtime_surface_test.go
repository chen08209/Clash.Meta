package tunnel

import (
	"testing"

	"github.com/metacubex/mihomo/common/utils"
	C "github.com/metacubex/mihomo/constant"
	P "github.com/metacubex/mihomo/constant/provider"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type surfaceProxy struct {
	C.Proxy
	name        string
	adapterType C.AdapterType
}

func (s surfaceProxy) Name() string        { return s.name }
func (s surfaceProxy) Type() C.AdapterType { return s.adapterType }

type surfaceProvider struct {
	name    string
	version uint32
	proxies []C.Proxy
}

func (s *surfaceProvider) Name() string               { return s.name }
func (s *surfaceProvider) VehicleType() P.VehicleType { return P.File }
func (s *surfaceProvider) Type() P.ProviderType       { return P.Proxy }
func (s *surfaceProvider) Initial() error             { return nil }
func (s *surfaceProvider) Update() error              { return nil }
func (s *surfaceProvider) Proxies() []C.Proxy         { return s.proxies }
func (s *surfaceProvider) Count() int                 { return len(s.proxies) }
func (s *surfaceProvider) Touch()                     {}
func (s *surfaceProvider) HealthCheck()               {}
func (s *surfaceProvider) Version() uint32            { return s.version }
func (s *surfaceProvider) HealthCheckURL() string     { return "" }
func (s *surfaceProvider) RegisterHealthCheckTask(string, utils.IntRanges[uint16], string, uint) {
}

type surfaceRuleProvider struct{}

func (surfaceRuleProvider) Name() string               { return "rule-provider-a" }
func (surfaceRuleProvider) VehicleType() P.VehicleType { return P.File }
func (surfaceRuleProvider) Type() P.ProviderType       { return P.Rule }
func (surfaceRuleProvider) Initial() error             { return nil }
func (surfaceRuleProvider) Update() error              { return nil }
func (surfaceRuleProvider) Behavior() P.RuleBehavior   { return P.Domain }
func (surfaceRuleProvider) Count() int                 { return 1 }
func (surfaceRuleProvider) Match(*C.Metadata, C.RuleMatchHelper) bool {
	return false
}
func (surfaceRuleProvider) Strategy() any { return nil }
func TestTunnelModeTextRoundTrip(test *testing.T) {
	tests := []struct {
		mode    TunnelMode
		text    string
		aliases []string
	}{
		{Global, "global", []string{"GLOBAL", "Global"}},
		{Rule, "rule", []string{"RULE"}},
		{Direct, "direct", []string{"DIRECT"}},
	}
	for _, testCase := range tests {
		text, err := testCase.mode.MarshalText()
		require.NoError(test, err)
		assert.Equal(test, testCase.text, string(text))
		assert.Equal(test, testCase.text, testCase.mode.String())

		for _, alias := range append([]string{testCase.text}, testCase.aliases...) {
			var mode TunnelMode
			require.NoError(test, mode.UnmarshalText([]byte(alias)), alias)
			assert.Equal(test, testCase.mode, mode, alias)
		}
	}

	var mode TunnelMode
	require.Error(test, mode.UnmarshalText([]byte("bogus")))
	require.Error(test, mode.UnmarshalText([]byte("")))

	assert.Len(test, ModeMapping, 3)
	for text, mapped := range ModeMapping {
		assert.Equal(test, text, mapped.String())
	}

	assert.Equal(test, "Unknown", TunnelMode(99).String())
}

func TestTunnelStatusString(test *testing.T) {
	assert.Equal(test, "suspend", Suspend.String())
	assert.Equal(test, "inner", Inner.String())
	assert.Equal(test, "running", Running.String())
	assert.Equal(test, "Unknown", TunnelStatus(99).String())
}

func TestAllProxiesCacheAndInvalidation(test *testing.T) {
	test.Cleanup(func() {
		UpdateProxies(nil, nil)
		InvalidateAllProxies()
	})

	provider := &surfaceProvider{
		name:    "provider-a",
		version: 1,
		proxies: []C.Proxy{surfaceProxy{name: "provider-a-node", adapterType: C.Direct}},
	}
	declared := surfaceProxy{name: "declared", adapterType: C.Direct}
	UpdateProxies(
		map[string]C.Proxy{"declared": declared},
		map[string]P.ProxyProvider{"provider-a": provider},
	)

	proxies := AllProxies()
	assert.Len(test, proxies, 2)
	assert.Contains(test, proxies, "declared")
	assert.Contains(test, proxies, "provider-a-node")
	require.NotNil(test, allProxies)
	require.NotNil(test, allProxiesVersions)

	// the cache survives a read while the provider version holds
	assert.True(test, allProxiesUpToDate(map[string]P.ProxyProvider{"provider-a": provider}))
	cached := AllProxies()
	assert.Equal(test, proxies, cached)

	// a missing provider, an extra one or a bumped version all invalidate it
	assert.False(test, allProxiesUpToDate(nil))
	assert.False(test, allProxiesUpToDate(map[string]P.ProxyProvider{}))
	other := &surfaceProvider{name: "provider-b"}
	assert.False(test, allProxiesUpToDate(map[string]P.ProxyProvider{"provider-a": provider, "provider-b": other}))
	provider.version = 2
	assert.False(test, allProxiesUpToDate(map[string]P.ProxyProvider{"provider-a": provider}))

	// the rebuild follows the new version and swaps the node
	provider.proxies = []C.Proxy{surfaceProxy{name: "provider-a-node-2", adapterType: C.Direct}}
	rebuilt := AllProxies()
	assert.Len(test, rebuilt, 2)
	assert.Contains(test, rebuilt, "provider-a-node-2")
	assert.NotContains(test, rebuilt, "provider-a-node")

	// an explicit drop forces the next read to rebuild
	InvalidateAllProxies()
	assert.Nil(test, allProxies)
	after := AllProxies()
	assert.Contains(test, after, "provider-a-node-2")
}

func TestProvidersSnapshotsFollowTheRuntime(test *testing.T) {
	test.Cleanup(func() {
		UpdateProxies(nil, nil)
		UpdateRules(nil, nil, nil)
	})

	provider := &surfaceProvider{name: "provider-a"}
	UpdateProxies(nil, map[string]P.ProxyProvider{"provider-a": provider})
	assert.Contains(test, ProvidersSnapshot(), "provider-a")

	UpdateRules(nil, nil, map[string]P.RuleProvider{"rule-provider-a": surfaceRuleProvider{}})
	assert.Contains(test, RuleProvidersSnapshot(), "rule-provider-a")
}
