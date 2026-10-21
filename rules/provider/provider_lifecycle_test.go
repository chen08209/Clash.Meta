package provider

import (
	"bytes"
	"encoding/json"
	"net/netip"
	"os"
	"path/filepath"
	"testing"

	"github.com/metacubex/mihomo/component/resource"
	C "github.com/metacubex/mihomo/constant"
	P "github.com/metacubex/mihomo/constant/provider"
	"github.com/metacubex/mihomo/common/utils"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type stubTunnel struct{}

func (*stubTunnel) Providers() map[string]P.ProxyProvider    { return nil }
func (*stubTunnel) RuleProviders() map[string]P.RuleProvider { return nil }
func (*stubTunnel) RuleUpdateCallback() *utils.Callback[P.RuleProvider] {
	return utils.NewCallback[P.RuleProvider]()
}

// withStubTunnel points the provider update callback at a no-op tunnel for the
// duration of the test and restores the package global afterwards.
func withStubTunnel(test *testing.T) {
	test.Helper()
	original := tunnel
	tunnel = &stubTunnel{}
	test.Cleanup(func() { tunnel = original })
}

func writeRuleFile(test *testing.T, name string, content string) string {
	test.Helper()
	path := filepath.Join(test.TempDir(), name)
	require.NoError(test, os.WriteFile(path, []byte(content), 0o644))
	return path
}

// closeProvider stops the file watcher through the concrete wrapper, which the
// RuleProvider interface does not expose.
func closeProvider(test *testing.T, ruleProvider P.RuleProvider) {
	test.Helper()
	if closer, ok := ruleProvider.(interface{ Close() error }); ok {
		test.Cleanup(func() { _ = closer.Close() })
	}
}

func TestRuleSetProviderInitialAndUpdateFromFile(test *testing.T) {
	withStubTunnel(test)
	path := writeRuleFile(test, "domain.txt", "example.com\n.example.org\n")

	ruleProvider := NewRuleSetProvider(
		"file-domain",
		P.Domain,
		P.TextRule,
		0,
		resource.NewFileVehicle(path),
		nil,
		nil,
		nil,
	)
	closeProvider(test, ruleProvider)

	require.NoError(test, ruleProvider.Initial())
	assert.Equal(test, "file-domain", ruleProvider.Name())
	assert.Equal(test, P.File, ruleProvider.VehicleType())
	assert.Equal(test, P.Rule, ruleProvider.Type())
	assert.Equal(test, P.Domain, ruleProvider.Behavior())
	assert.Equal(test, 2, ruleProvider.Count())
	assert.True(test, ruleProvider.Match(&C.Metadata{Host: "www.example.org"}, C.RuleMatchHelper{}))
	assert.False(test, ruleProvider.Match(&C.Metadata{Host: "example.net"}, C.RuleMatchHelper{}))

	require.NoError(test, os.WriteFile(path, []byte("example.net\n.example.com\n"), 0o644))
	require.NoError(test, ruleProvider.Update())
	assert.Equal(test, 2, ruleProvider.Count())
	assert.True(test, ruleProvider.Match(&C.Metadata{Host: "www.example.com"}, C.RuleMatchHelper{}))
	assert.False(test, ruleProvider.Match(&C.Metadata{Host: "www.example.org"}, C.RuleMatchHelper{}))

	data, err := json.Marshal(ruleProvider)
	require.NoError(test, err)
	var payload struct {
		Behavior    string `json:"behavior"`
		Format      string `json:"format"`
		Name        string `json:"name"`
		RuleCount   int    `json:"ruleCount"`
		Type        string `json:"type"`
		VehicleType string `json:"vehicleType"`
	}
	require.NoError(test, json.Unmarshal(data, &payload))
	assert.Equal(test, "Domain", payload.Behavior)
	assert.Equal(test, "TextRule", payload.Format)
	assert.Equal(test, "Rule", payload.Type)
	assert.Equal(test, "File", payload.VehicleType)
	assert.Equal(test, 2, payload.RuleCount)
}

func TestRuleSetProviderInitialFromMrsFile(test *testing.T) {
	withStubTunnel(test)
	var mrs bytes.Buffer
	require.NoError(test, ConvertToMrs([]byte("10.0.0.0/8\n192.168.0.0/16\n"), P.IPCIDR, P.TextRule, &mrs))
	path := writeRuleFile(test, "ipcidr.mrs", mrs.String())

	ruleProvider := NewRuleSetProvider(
		"file-ipcidr",
		P.IPCIDR,
		P.MrsRule,
		0,
		resource.NewFileVehicle(path),
		nil,
		nil,
		nil,
	)
	closeProvider(test, ruleProvider)

	require.NoError(test, ruleProvider.Initial())
	assert.Equal(test, 2, ruleProvider.Count())
	assert.True(test, ruleProvider.Match(&C.Metadata{DstIP: netip.MustParseAddr("10.1.2.3")}, C.RuleMatchHelper{}))
	assert.True(test, ruleProvider.Match(&C.Metadata{DstIP: netip.MustParseAddr("192.168.1.1")}, C.RuleMatchHelper{}))
	assert.False(test, ruleProvider.Match(&C.Metadata{DstIP: netip.MustParseAddr("172.16.0.1")}, C.RuleMatchHelper{}))
}

func TestRuleSetProviderKeepsPayloadWhenTheFileIsMissing(test *testing.T) {
	withStubTunnel(test)
	missing := filepath.Join(test.TempDir(), "absent.txt")

	ruleProvider := NewRuleSetProvider(
		"fallback-domain",
		P.Domain,
		P.TextRule,
		0,
		resource.NewFileVehicle(missing),
		[]string{"example.com", ".example.org"},
		nil,
		nil,
	)
	closeProvider(test, ruleProvider)

	assert.Error(test, ruleProvider.Initial())
	assert.Equal(test, 2, ruleProvider.Count())
	assert.True(test, ruleProvider.Match(&C.Metadata{Host: "www.example.org"}, C.RuleMatchHelper{}))
	assert.False(test, ruleProvider.Match(&C.Metadata{Host: "example.net"}, C.RuleMatchHelper{}))
}

func TestParseRuleProviderInlineMapping(test *testing.T) {
	ruleProvider, err := ParseRuleProvider(
		"inline-domain",
		map[string]any{
			"type":     "inline",
			"behavior": "domain",
			"payload":  []string{"example.com", ".example.org"},
		},
		nil,
		func(string) resource.BundleFile { return nil },
	)
	require.NoError(test, err)
	closeProvider(test, ruleProvider)

	assert.Equal(test, "inline-domain", ruleProvider.Name())
	assert.Equal(test, P.Inline, ruleProvider.VehicleType())
	assert.Equal(test, 2, ruleProvider.Count())
	assert.True(test, ruleProvider.Match(&C.Metadata{Host: "www.example.org"}, C.RuleMatchHelper{}))
	assert.False(test, ruleProvider.Match(&C.Metadata{Host: "example.net"}, C.RuleMatchHelper{}))
}
