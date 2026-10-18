package outboundgroup

import (
	"testing"

	"github.com/metacubex/mihomo/adapter"
	"github.com/metacubex/mihomo/adapter/outbound"
	C "github.com/metacubex/mihomo/constant"
	P "github.com/metacubex/mihomo/constant/provider"
	"github.com/stretchr/testify/require"
)

func TestGetProxiesCacheHitDoesNotAllocate(t *testing.T) {
	proxy := adapter.NewProxy(
		outbound.NewDirectWithOption(outbound.DirectOption{Name: "direct"}),
	)
	group := NewGroupBase(GroupBaseOption{
		Name:          "cache-hit",
		Type:          C.Selector,
		EmptyFallback: adapter.NewProxy(outbound.NewDirect()),
		Providers:     []P.ProxyProvider{failoverProvider(t, []C.Proxy{proxy})},
	})
	require.Len(t, group.GetProxies(false), 1)

	var proxies []C.Proxy
	allocations := testing.AllocsPerRun(100, func() {
		proxies = group.GetProxies(false)
	})
	require.Len(t, proxies, 1)
	require.EqualValues(t, 0, allocations)
}

func BenchmarkGetProxiesCacheHit(b *testing.B) {
	proxy := adapter.NewProxy(
		outbound.NewDirectWithOption(outbound.DirectOption{Name: "direct"}),
	)
	group := NewGroupBase(GroupBaseOption{
		Name:          "cache-hit",
		Type:          C.Selector,
		EmptyFallback: adapter.NewProxy(outbound.NewDirect()),
		Providers:     []P.ProxyProvider{failoverProvider(b, []C.Proxy{proxy})},
	})
	if len(group.GetProxies(false)) != 1 {
		b.Fatal("provider cache was not warmed")
	}

	b.ReportAllocs()
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			if proxies := group.GetProxies(false); len(proxies) != 1 {
				b.Fatalf("proxies length = %d", len(proxies))
			}
		}
	})
}
