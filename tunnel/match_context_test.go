package tunnel

import (
	"fmt"
	"net/netip"
	"sync"
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

func TestMatchContextPoolReuseIsIsolated(t *testing.T) {
	first := acquireMatchContext(
		&C.Metadata{Host: "first.example.com"},
		&runtimeSnapshot{},
		true,
		true,
	)
	releaseMatchContext(first)

	metadata := &C.Metadata{Host: "second.example.com"}
	runtimeState := &runtimeSnapshot{}
	second := acquireMatchContext(metadata, runtimeState, false, false)
	require.Same(t, metadata, second.metadata)
	require.Same(t, runtimeState, second.runtimeState)
	require.False(t, second.resolved)
	require.False(t, second.attemptProcessLookup)
	releaseMatchContext(second)
}

func TestMatchContextPoolIsIsolatedAcrossGoroutines(t *testing.T) {
	const (
		workers    = 32
		iterations = 1000
	)

	var wait sync.WaitGroup
	for worker := 0; worker < workers; worker++ {
		worker := worker
		wait.Add(1)
		go func() {
			defer wait.Done()

			metadata := &C.Metadata{Host: fmt.Sprintf("worker-%d.example.com", worker)}
			runtimeState := &runtimeSnapshot{}
			for iteration := 0; iteration < iterations; iteration++ {
				resolved := iteration&1 == 0
				attemptProcessLookup := iteration&2 == 0
				context := acquireMatchContext(metadata, runtimeState, resolved, attemptProcessLookup)
				if context.metadata != metadata ||
					context.runtimeState != runtimeState ||
					context.resolved != resolved ||
					context.attemptProcessLookup != attemptProcessLookup {
					t.Errorf("worker %d got a context from another acquisition", worker)
					releaseMatchContext(context)
					return
				}
				releaseMatchContext(context)
			}
		}()
	}
	wait.Wait()
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
