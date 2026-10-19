package outboundgroup

import (
	"fmt"
	"testing"

	"github.com/metacubex/mihomo/adapter"
	"github.com/metacubex/mihomo/adapter/outbound"
	C "github.com/metacubex/mihomo/constant"
	P "github.com/metacubex/mihomo/constant/provider"
	"github.com/stretchr/testify/require"
)

func fallbackBenchmarkGroup(tb testing.TB, size int) (*Fallback, C.Proxy) {
	tb.Helper()

	proxies := make([]C.Proxy, size)
	for index := range proxies {
		proxies[index] = adapter.NewProxy(outbound.NewDirectWithOption(outbound.DirectOption{
			Name: fmt.Sprintf("node-%04d", index),
		}))
	}

	group, err := NewFallback(
		GroupCommonOption{Name: "fallback-index", URL: testUrl},
		FallbackOption{},
		adapter.NewProxy(outbound.NewDirectWithOption(outbound.DirectOption{Name: "COMPATIBLE"})),
		[]P.ProxyProvider{failoverProvider(tb, proxies)},
	)
	require.NoError(tb, err)
	selected := proxies[len(proxies)-1]
	group.ForceSet(selected.Name())
	return group, selected
}

func TestFallbackFindsIndexedSelectedProxy(t *testing.T) {
	group, selected := fallbackBenchmarkGroup(t, 64)
	group.clearSelection()

	proxy, hasAlternative := group.findAliveProxyState(false)
	require.Equal(t, selected.Name(), proxy.Name())
	require.True(t, hasAlternative)
}

func TestFallbackSkipsAnIndexedDeadSelectedProxy(t *testing.T) {
	group, selected := fallbackBenchmarkGroup(t, 64)
	selected.(aliveStateSetter).SetAliveForTestUrl(group.testUrl, false)
	group.clearSelection()

	proxy, hasAlternative := group.findAliveProxyState(false)
	require.Equal(t, "node-0000", proxy.Name())
	require.True(t, hasAlternative)
}

func BenchmarkFallbackSelectedProxyLargeGroup(b *testing.B) {
	group, selected := fallbackBenchmarkGroup(b, 4096)
	want := selected.Name()

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		group.clearSelection()
		if group.findAliveProxy(false).Name() != want {
			b.Fatal("selected proxy mismatch")
		}
	}
}
