package tunnel

import (
	"net/netip"
	"testing"

	C "github.com/metacubex/mihomo/constant"
	rulesCommon "github.com/metacubex/mihomo/rules/common"
	"github.com/metacubex/mihomo/rules/wrapper"
	"github.com/stretchr/testify/require"
)

func TestMatchContextResolvesOnce(t *testing.T) {
	metadata := &C.Metadata{Host: "127.0.0.1"}
	context := &matchContext{metadata: metadata}

	context.ResolveIP()
	require.True(t, context.resolved)
	require.True(t, metadata.DstIP.IsValid())

	context.ResolveIP()
	require.Equal(t, metadata.DstIP, netip.MustParseAddr("127.0.0.1"))
}

func TestMatchContextChecksPassRule(t *testing.T) {
	passProxy := passTestProxy{}
	context := &matchContext{
		runtimeState: &runtimeSnapshot{
			proxies: map[string]C.Proxy{"PASS": passProxy},
		},
	}

	require.True(t, context.CheckPassRule("PASS"))
	require.False(t, context.CheckPassRule("MISSING"))
}

func TestMatchContextAttemptsProcessLookupOnce(t *testing.T) {
	context := &matchContext{
		metadata: &C.Metadata{
			NetWork: C.TCP,
			Type:    C.HTTP,
			SrcIP:   netip.MustParseAddr("127.0.0.1"),
			SrcPort: 1,
		},
		attemptProcessLookup: true,
	}

	context.FindProcess()
	require.False(t, context.attemptProcessLookup)

	context.FindProcess()
	require.False(t, context.attemptProcessLookup)
}

func TestRuleUsesMatchHelperUnwrapsPureRules(t *testing.T) {
	rule := wrapper.NewRuleWrapper(
		rulesCommon.NewDomain("benchmark.example.com", "DIRECT"),
	)

	require.False(t, ruleUsesMatchHelper(rule))
}

type passTestProxy struct {
	C.Proxy
}

func (passTestProxy) Name() string {
	return "PASS"
}

func (passTestProxy) Type() C.AdapterType {
	return C.PassRule
}

func (passTestProxy) Adapter() C.ProxyAdapter {
	return nil
}

func (passTestProxy) Unwrap(*C.Metadata, bool) C.Proxy {
	return nil
}
