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

func urlTestBenchmarkGroup(tb testing.TB, size int) (*URLTest, C.Proxy) {
	tb.Helper()

	proxies := make([]C.Proxy, size)
	for index := range proxies {
		proxies[index] = adapter.NewProxy(outbound.NewDirectWithOption(outbound.DirectOption{
			Name: fmt.Sprintf("node-%04d", index),
		}))
	}

	group, err := NewURLTest(
		GroupCommonOption{Name: "url-test-index", URL: testUrl},
		URLTestOption{},
		adapter.NewProxy(outbound.NewDirectWithOption(outbound.DirectOption{Name: "COMPATIBLE"})),
		[]P.ProxyProvider{failoverProvider(tb, proxies)},
	)
	require.NoError(tb, err)
	selected := proxies[len(proxies)-1]
	group.ForceSet(selected.Name())
	return group, selected
}

func TestURLTestFindsIndexedSelectedProxy(t *testing.T) {
	group, selected := urlTestBenchmarkGroup(t, 64)
	require.Equal(t, selected.Name(), group.fast(false).Name())
	require.True(t, group.fastHasAlternative.Load())
}

func TestURLTestSkipsAnIndexedDeadSelectedProxy(t *testing.T) {
	group, selected := urlTestBenchmarkGroup(t, 64)
	selected.(aliveStateSetter).SetAliveForTestUrl(group.testUrl, false)

	require.Equal(t, "node-0000", group.fast(false).Name())
	require.True(t, group.fastHasAlternative.Load())
}

func BenchmarkURLTestSelectedProxyLargeGroup(b *testing.B) {
	group, selected := urlTestBenchmarkGroup(b, 4096)
	want := selected.Name()

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		group.resetFastNode()
		if group.fast(false).Name() != want {
			b.Fatal("selected proxy mismatch")
		}
	}
}
