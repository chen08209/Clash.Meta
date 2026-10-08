package outboundgroup

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/metacubex/mihomo/adapter"
	"github.com/metacubex/mihomo/adapter/outbound"
	"github.com/metacubex/mihomo/adapter/provider"
	C "github.com/metacubex/mihomo/constant"
	P "github.com/metacubex/mihomo/constant/provider"

	"github.com/stretchr/testify/require"
)

type recordingDeadlineProxy struct {
	*outbound.Base
	deadline chan time.Time
}

func (p *recordingDeadlineProxy) DialContext(ctx context.Context, _ *C.Metadata) (C.Conn, error) {
	if deadline, ok := ctx.Deadline(); ok {
		p.deadline <- deadline
	}
	return nil, errors.New("dial failed")
}

// testNodeProxy dials like DIRECT but reports a proxy type, so the failover
// tests can reach a local test server while still looking like a node.
type testNodeProxy struct {
	C.ProxyAdapter
}

func (p *testNodeProxy) Type() C.AdapterType {
	return C.Shadowsocks
}

func failoverProxy(name string) C.Proxy {
	return adapter.NewProxy(&failingProxy{Base: outbound.NewBase(outbound.BaseOption{
		Name: name,
		Type: C.Shadowsocks,
	})})
}

func failoverProvider(t *testing.T, proxies []C.Proxy) P.ProxyProvider {
	t.Helper()
	health := provider.NewHealthCheck(proxies, "", 0, 0, true, nil)
	pd, err := provider.NewCompatibleProvider("test-provider", proxies, health)
	require.NoError(t, err)
	return pd
}

func TestFallbackMarksFailedProxyDownAndSelectsHealthy(t *testing.T) {
	failing := failoverProxy("failing")
	healthy := adapter.NewProxy(outbound.NewDirectWithOption(outbound.DirectOption{Name: "healthy"}))
	proxies := []C.Proxy{failing, healthy}
	emptyFallback := adapter.NewProxy(outbound.NewDirectWithOption(outbound.DirectOption{Name: "COMPATIBLE"}))

	group, err := NewFallback(
		GroupCommonOption{Name: "fallback", URL: testUrl, TestTimeout: 1000},
		FallbackOption{},
		emptyFallback,
		[]P.ProxyProvider{failoverProvider(t, proxies)},
	)
	require.NoError(t, err)

	group.ForceSet(failing.Name())
	_, err = group.DialContext(context.Background(), &C.Metadata{
		NetWork: C.TCP,
		Host:    "example.com",
		DstPort: 80,
	})
	require.Error(t, err)
	require.False(t, failing.AliveForTestUrl(testUrl))
	require.Equal(t, healthy.Name(), group.Now())
}

func TestLoadBalanceMarksFailedProxyDown(t *testing.T) {
	failing := failoverProxy("failing")
	healthy := adapter.NewProxy(outbound.NewDirectWithOption(outbound.DirectOption{Name: "healthy"}))
	proxies := []C.Proxy{failing, healthy}
	emptyFallback := adapter.NewProxy(outbound.NewDirectWithOption(outbound.DirectOption{Name: "COMPATIBLE"}))

	group, err := NewLoadBalance(
		GroupCommonOption{Name: "balance", URL: testUrl, TestTimeout: 1000},
		LoadBalanceOption{Strategy: "round-robin"},
		emptyFallback,
		[]P.ProxyProvider{failoverProvider(t, proxies)},
	)
	require.NoError(t, err)

	_, err = group.DialContext(context.Background(), &C.Metadata{
		NetWork: C.TCP,
		Host:    "example.com",
		DstPort: 80,
	})
	require.Error(t, err)
	require.False(t, failing.AliveForTestUrl(testUrl))
	require.Equal(t, healthy.Name(), group.Unwrap(nil, false).Name())
}

func TestURLTestFailureForOneURLKeepsOtherURLsAlive(t *testing.T) {
	proxy := failoverProxy("failing")

	_, err := proxy.URLTest(context.Background(), "https://one.example/204", nil)
	require.Error(t, err)
	require.False(t, proxy.AliveForTestUrl("https://one.example/204"))
	require.True(t, proxy.AliveForTestUrl("https://two.example/204"))
}

func TestOnDialFailedReleasesFailureLockBeforeHealthCheck(t *testing.T) {
	group := NewGroupBase(GroupBaseOption{
		Name:           "group",
		Type:           C.Selector,
		TestTimeout:    1000,
		MaxFailedTimes: 1,
	})
	called := make(chan struct{}, 2)
	healthCheck := func() {
		group.failedTestMux.Lock()
		group.failedTestMux.Unlock()
		called <- struct{}{}
	}
	group.onDialFailed(C.Shadowsocks, errors.New("dial failed"), healthCheck)
	group.onDialFailed(C.Shadowsocks, errors.New("dial failed"), healthCheck)

	select {
	case <-called:
	case <-time.After(time.Second):
		t.Fatal("health-check callback deadlocked on the failure mutex")
	}
}

func TestOnDialFailedHonorsSingleFailureThreshold(t *testing.T) {
	group := NewGroupBase(GroupBaseOption{
		Name:           "group",
		Type:           C.Selector,
		TestTimeout:    1000,
		MaxFailedTimes: 1,
	})
	called := make(chan struct{})
	group.onDialFailed(C.Shadowsocks, errors.New("dial failed"), func() {
		close(called)
	})

	select {
	case <-called:
	case <-time.After(time.Second):
		t.Fatal("single failure did not trigger the configured threshold")
	}
}

func TestOnDialFailedRecoversPanic(t *testing.T) {
	group := NewGroupBase(GroupBaseOption{
		Name:           "group",
		Type:           C.Selector,
		TestTimeout:    1000,
		MaxFailedTimes: 1,
	})
	finished := make(chan struct{})
	group.onDialFailed(C.Shadowsocks, errors.New("dial failed"), func() {
		defer close(finished)
		panic("boom")
	})

	select {
	case <-finished:
		time.Sleep(10 * time.Millisecond)
	case <-time.After(time.Second):
		t.Fatal("failure handler did not run")
	}
}

func TestProxyFailureForOneTestURLKeepsOtherURLsAlive(t *testing.T) {
	proxy := adapter.NewProxy(outbound.NewDirectWithOption(outbound.DirectOption{Name: "node"}))

	proxy.SetAliveForTestUrl("https://one.example/204", false)

	require.False(t, proxy.AliveForTestUrl("https://one.example/204"))
	require.True(t, proxy.AliveForTestUrl("https://two.example/204"))
}

func TestURLTestNotifiesRouteInvalidationOnFailure(t *testing.T) {
	old := routeInvalidate
	t.Cleanup(func() { SetRouteInvalidate(old) })
	calls := 0
	SetRouteInvalidate(func() { calls++ })

	failing := failoverProxy("failing")
	healthy := adapter.NewProxy(outbound.NewDirectWithOption(outbound.DirectOption{Name: "healthy"}))
	proxies := []C.Proxy{failing, healthy}
	emptyFallback := adapter.NewProxy(outbound.NewDirectWithOption(outbound.DirectOption{Name: "COMPATIBLE"}))

	group, err := NewURLTest(
		GroupCommonOption{Name: "auto", URL: testUrl, TestTimeout: 1000},
		URLTestOption{},
		emptyFallback,
		[]P.ProxyProvider{failoverProvider(t, proxies)},
	)
	require.NoError(t, err)

	group.markProxyFailed(failing, errors.New("dial failed"))
	require.Equal(t, 1, calls)
}

func TestURLTestRotatesWhenNoProxyIsAlive(t *testing.T) {
	nodes := []*adapter.Proxy{
		failoverProxy("node-1").(*adapter.Proxy),
		failoverProxy("node-2").(*adapter.Proxy),
		failoverProxy("node-3").(*adapter.Proxy),
	}
	proxies := make([]C.Proxy, 0, len(nodes))
	for _, proxy := range nodes {
		proxy.SetAliveForTestUrl(testUrl, false)
		proxies = append(proxies, proxy)
	}
	emptyFallback := adapter.NewProxy(outbound.NewDirectWithOption(outbound.DirectOption{Name: "COMPATIBLE"}))

	group, err := NewURLTest(
		GroupCommonOption{Name: "auto", URL: testUrl, TestTimeout: 1000},
		URLTestOption{},
		emptyFallback,
		[]P.ProxyProvider{failoverProvider(t, proxies)},
	)
	require.NoError(t, err)

	first := group.fast(true)
	require.Equal(t, "node-1", first.Name())
	group.markProxyFailed(first, errors.New("dial failed"))

	second := group.fast(true)
	require.Equal(t, "node-2", second.Name())
	group.markProxyFailed(second, errors.New("dial failed"))

	third := group.fast(true)
	require.Equal(t, "node-3", third.Name())
}

func TestLoadBalanceRotatesWhenNoProxyIsAlive(t *testing.T) {
	nodes := []*adapter.Proxy{
		failoverProxy("node-1").(*adapter.Proxy),
		failoverProxy("node-2").(*adapter.Proxy),
		failoverProxy("node-3").(*adapter.Proxy),
	}
	proxies := make([]C.Proxy, 0, len(nodes))
	for _, proxy := range nodes {
		proxy.SetAliveForTestUrl(testUrl, false)
		proxies = append(proxies, proxy)
	}
	emptyFallback := adapter.NewProxy(outbound.NewDirectWithOption(outbound.DirectOption{Name: "COMPATIBLE"}))

	group, err := NewLoadBalance(
		GroupCommonOption{Name: "balance", URL: testUrl, TestTimeout: 1000},
		LoadBalanceOption{Strategy: "round-robin"},
		emptyFallback,
		[]P.ProxyProvider{failoverProvider(t, proxies)},
	)
	require.NoError(t, err)

	require.Equal(t, "node-1", group.Unwrap(nil, false).Name())
	require.Equal(t, "node-2", group.Unwrap(nil, false).Name())
	require.Equal(t, "node-3", group.Unwrap(nil, false).Name())
}

func TestFallbackRotatesWhenNoProxyIsAlive(t *testing.T) {
	nodes := []*adapter.Proxy{
		failoverProxy("node-1").(*adapter.Proxy),
		failoverProxy("node-2").(*adapter.Proxy),
		failoverProxy("node-3").(*adapter.Proxy),
	}
	proxies := make([]C.Proxy, 0, len(nodes))
	for _, proxy := range nodes {
		proxy.SetAliveForTestUrl(testUrl, false)
		proxies = append(proxies, proxy)
	}
	emptyFallback := adapter.NewProxy(outbound.NewDirectWithOption(outbound.DirectOption{Name: "COMPATIBLE"}))

	group, err := NewFallback(
		GroupCommonOption{Name: "fallback", URL: testUrl, TestTimeout: 1000},
		FallbackOption{},
		emptyFallback,
		[]P.ProxyProvider{failoverProvider(t, proxies)},
	)
	require.NoError(t, err)

	first := group.findAliveProxy(true)
	require.Equal(t, "node-1", first.Name())
	group.markProxyFailed(first, errors.New("dial failed"))

	second := group.findAliveProxy(true)
	require.Equal(t, "node-2", second.Name())
	group.markProxyFailed(second, errors.New("dial failed"))

	third := group.findAliveProxy(true)
	require.Equal(t, "node-3", third.Name())
}

func TestFallbackUsesBoundedAttemptWhenAlternativeIsAlive(t *testing.T) {
	deadline := make(chan time.Time, 1)
	failing := adapter.NewProxy(&recordingDeadlineProxy{
		Base: outbound.NewBase(outbound.BaseOption{
			Name: "failing",
			Type: C.Shadowsocks,
		}),
		deadline: deadline,
	})
	healthy := adapter.NewProxy(outbound.NewDirectWithOption(outbound.DirectOption{Name: "healthy"}))
	proxies := []C.Proxy{failing, healthy}
	emptyFallback := adapter.NewProxy(outbound.NewDirectWithOption(outbound.DirectOption{Name: "COMPATIBLE"}))

	group, err := NewFallback(
		GroupCommonOption{Name: "fallback", URL: testUrl, TestTimeout: 1000},
		FallbackOption{},
		emptyFallback,
		[]P.ProxyProvider{failoverProvider(t, proxies)},
	)
	require.NoError(t, err)

	group.ForceSet(failing.Name())
	_, err = group.DialContext(context.Background(), &C.Metadata{
		NetWork: C.TCP,
		Host:    "example.com",
		DstPort: 80,
	})
	require.Error(t, err)

	select {
	case got := <-deadline:
		remaining := time.Until(got)
		require.Greater(t, remaining, time.Duration(0))
		require.LessOrEqual(t, remaining, fastFailoverAttemptTimeout+100*time.Millisecond)
	case <-time.After(time.Second):
		t.Fatal("first fallback dial did not receive a bounded attempt deadline")
	}
}

func TestURLTestUsesBoundedAttemptWhenAlternativeIsAlive(t *testing.T) {
	deadline := make(chan time.Time, 1)
	failing := adapter.NewProxy(&recordingDeadlineProxy{
		Base: outbound.NewBase(outbound.BaseOption{
			Name: "failing",
			Type: C.Shadowsocks,
		}),
		deadline: deadline,
	})
	healthy := adapter.NewProxy(outbound.NewDirectWithOption(outbound.DirectOption{Name: "healthy"}))
	proxies := []C.Proxy{failing, healthy}
	emptyFallback := adapter.NewProxy(outbound.NewDirectWithOption(outbound.DirectOption{Name: "COMPATIBLE"}))

	group, err := NewURLTest(
		GroupCommonOption{Name: "auto", URL: testUrl, TestTimeout: 1000},
		URLTestOption{},
		emptyFallback,
		[]P.ProxyProvider{failoverProvider(t, proxies)},
	)
	require.NoError(t, err)

	_, err = group.DialContext(context.Background(), &C.Metadata{
		NetWork: C.TCP,
		Host:    "example.com",
		DstPort: 80,
	})
	require.Error(t, err)

	select {
	case got := <-deadline:
		remaining := time.Until(got)
		require.Greater(t, remaining, time.Duration(0))
		require.LessOrEqual(t, remaining, fastFailoverAttemptTimeout+100*time.Millisecond)
	case <-time.After(time.Second):
		t.Fatal("first dial did not receive a bounded attempt deadline")
	}
}

func shrinkFailoverRecoveryDelay(t *testing.T) {
	t.Helper()
	previous := fastFailoverRecoveryDelay
	fastFailoverRecoveryDelay = 10 * time.Millisecond
	t.Cleanup(func() { fastFailoverRecoveryDelay = previous })
}

func recoveryTestServer(t *testing.T) string {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	t.Cleanup(server.Close)
	return server.URL
}

func recoveringFailoverProxy(name string) C.Proxy {
	return adapter.NewProxy(&testNodeProxy{ProxyAdapter: outbound.NewDirectWithOption(outbound.DirectOption{Name: name})})
}

func TestURLTestRechecksProxyItBenched(t *testing.T) {
	shrinkFailoverRecoveryDelay(t)
	testServerURL := recoveryTestServer(t)

	benched := recoveringFailoverProxy("benched")
	healthy := adapter.NewProxy(outbound.NewDirectWithOption(outbound.DirectOption{Name: "healthy"}))
	emptyFallback := adapter.NewProxy(outbound.NewDirectWithOption(outbound.DirectOption{Name: "COMPATIBLE"}))
	group, err := NewURLTest(
		GroupCommonOption{Name: "auto", URL: testServerURL, TestTimeout: 1000, ExpectedStatus: "204"},
		URLTestOption{},
		emptyFallback,
		[]P.ProxyProvider{failoverProvider(t, []C.Proxy{benched, healthy})},
	)
	require.NoError(t, err)

	group.markProxyFailed(benched, errors.New("dial failed"))
	require.False(t, benched.AliveForTestUrl(testServerURL))

	require.Eventually(t, func() bool { return benched.AliveForTestUrl(testServerURL) }, 3*time.Second, 10*time.Millisecond)
}

func TestFallbackRechecksProxyItBenched(t *testing.T) {
	shrinkFailoverRecoveryDelay(t)
	testServerURL := recoveryTestServer(t)

	benched := recoveringFailoverProxy("benched-fallback")
	healthy := adapter.NewProxy(outbound.NewDirectWithOption(outbound.DirectOption{Name: "healthy-fallback"}))
	emptyFallback := adapter.NewProxy(outbound.NewDirectWithOption(outbound.DirectOption{Name: "COMPATIBLE"}))
	group, err := NewFallback(
		GroupCommonOption{Name: "fallback", URL: testServerURL, TestTimeout: 1000, ExpectedStatus: "204"},
		FallbackOption{},
		emptyFallback,
		[]P.ProxyProvider{failoverProvider(t, []C.Proxy{benched, healthy})},
	)
	require.NoError(t, err)

	group.markProxyFailed(benched, errors.New("dial failed"))
	require.False(t, benched.AliveForTestUrl(testServerURL))

	require.Eventually(t, func() bool { return benched.AliveForTestUrl(testServerURL) }, 3*time.Second, 10*time.Millisecond)
}

func TestLoadBalanceRechecksProxyItBenched(t *testing.T) {
	shrinkFailoverRecoveryDelay(t)
	testServerURL := recoveryTestServer(t)

	benched := recoveringFailoverProxy("benched-balance")
	healthy := adapter.NewProxy(outbound.NewDirectWithOption(outbound.DirectOption{Name: "healthy-balance"}))
	emptyFallback := adapter.NewProxy(outbound.NewDirectWithOption(outbound.DirectOption{Name: "COMPATIBLE"}))
	group, err := NewLoadBalance(
		GroupCommonOption{Name: "balance", URL: testServerURL, TestTimeout: 1000, ExpectedStatus: "204"},
		LoadBalanceOption{Strategy: "round-robin"},
		emptyFallback,
		[]P.ProxyProvider{failoverProvider(t, []C.Proxy{benched, healthy})},
	)
	require.NoError(t, err)

	group.markProxyFailed(benched, errors.New("dial failed"))
	require.False(t, benched.AliveForTestUrl(testServerURL))

	require.Eventually(t, func() bool { return benched.AliveForTestUrl(testServerURL) }, 3*time.Second, 10*time.Millisecond)
}

func TestDirectMemberFailureIsNotBenched(t *testing.T) {
	direct := adapter.NewProxy(outbound.NewDirectWithOption(outbound.DirectOption{Name: "direct"}))
	healthy := adapter.NewProxy(outbound.NewDirectWithOption(outbound.DirectOption{Name: "healthy"}))
	emptyFallback := adapter.NewProxy(outbound.NewDirectWithOption(outbound.DirectOption{Name: "COMPATIBLE"}))
	group, err := NewURLTest(
		GroupCommonOption{Name: "auto", URL: testUrl, TestTimeout: 1000},
		URLTestOption{},
		emptyFallback,
		[]P.ProxyProvider{failoverProvider(t, []C.Proxy{direct, healthy})},
	)
	require.NoError(t, err)

	group.markProxyFailed(direct, errors.New("dial failed"))
	require.True(t, direct.AliveForTestUrl(testUrl))
}

func TestFallbackSelectionChangeDropsTheCachedResolution(t *testing.T) {
	first := adapter.NewProxy(outbound.NewDirectWithOption(outbound.DirectOption{Name: "first"}))
	second := adapter.NewProxy(outbound.NewDirectWithOption(outbound.DirectOption{Name: "second"}))
	proxies := []C.Proxy{first, second}
	emptyFallback := adapter.NewProxy(outbound.NewDirectWithOption(outbound.DirectOption{Name: "COMPATIBLE"}))

	group, err := NewFallback(
		GroupCommonOption{Name: "fallback", URL: testUrl, TestTimeout: 1000},
		FallbackOption{},
		emptyFallback,
		[]P.ProxyProvider{failoverProvider(t, proxies)},
	)
	require.NoError(t, err)

	require.Equal(t, "first", group.findAliveProxy(true).Name())

	group.ForceSet("second")
	require.Equal(t, "second", group.findAliveProxy(true).Name())
}
