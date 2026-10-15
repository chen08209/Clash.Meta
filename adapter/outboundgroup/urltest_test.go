package outboundgroup

import (
	"context"
	"errors"
	"testing"

	"github.com/metacubex/mihomo/adapter"
	"github.com/metacubex/mihomo/adapter/outbound"
	"github.com/metacubex/mihomo/adapter/provider"
	C "github.com/metacubex/mihomo/constant"
	P "github.com/metacubex/mihomo/constant/provider"

	"github.com/stretchr/testify/require"
)

type failingProxy struct {
	*outbound.Base
}

func (f *failingProxy) DialContext(context.Context, *C.Metadata) (C.Conn, error) {
	return nil, errors.New("dial failed")
}

func TestURLTestSkipsAProxyAfterItsDialFails(t *testing.T) {
	failing := adapter.NewProxy(&failingProxy{Base: outbound.NewBase(outbound.BaseOption{
		Name: "failing",
		Type: C.Shadowsocks,
	})})
	healthy := adapter.NewProxy(outbound.NewDirectWithOption(outbound.DirectOption{Name: "healthy"}))
	proxies := []C.Proxy{failing, healthy}
	emptyFallback := adapter.NewProxy(outbound.NewDirectWithOption(outbound.DirectOption{Name: "COMPATIBLE"}))
	health := provider.NewHealthCheck(proxies, "", 0, 0, true, nil)
	pd, err := provider.NewCompatibleProvider("auto-provider", proxies, health)
	require.NoError(t, err)

	group, err := NewURLTest(
		GroupCommonOption{Name: "auto", URL: "https://example.com/generate_204", TestTimeout: 1000},
		URLTestOption{},
		emptyFallback,
		[]P.ProxyProvider{pd},
	)
	require.NoError(t, err)

	group.ForceSet(failing.Name())
	require.Equal(t, failing.Name(), group.Now())
	group.markProxyFailed(failing, errors.New("manual failure"))
	require.False(t, failing.AliveForTestUrl(group.testUrl))
	require.Equal(t, healthy.Name(), group.Now())

	failing.SetAliveForTestUrl(group.testUrl, true)
	group.ForceSet(failing.Name())
	require.Equal(t, failing.Name(), group.Now())

	_, err = group.DialContext(context.Background(), &C.Metadata{
		NetWork: C.TCP,
		Host:    "example.com",
		DstPort: 80,
	})
	require.Error(t, err)
	require.False(t, failing.AliveForTestUrl(group.testUrl))
	require.Equal(t, healthy.Name(), group.Now())
}
