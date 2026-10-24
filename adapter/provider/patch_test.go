package provider

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/metacubex/mihomo/adapter"
	"github.com/metacubex/mihomo/adapter/outbound"
	C "github.com/metacubex/mihomo/constant"
)

func TestHealthCheckWithoutATimeoutWaitsTheDefault(t *testing.T) {
	t.Cleanup(func() { defaultTestTimeout.Store(0) })

	if got := NewHealthCheck(nil, "http://own", 0, 0, true, nil).testTimeout(); got != fallbackTestTimeout {
		t.Errorf("unset timeout before any default = %v, want %v", got, fallbackTestTimeout)
	}
	SetDefaultTestTimeout(8 * time.Second)
	SetDefaultTestTimeout(0)
	if got := NewHealthCheck(nil, "http://own", 0, 0, true, nil).testTimeout(); got != 8*time.Second {
		t.Errorf("unset timeout = %v, want the default 8s", got)
	}
	if got := NewHealthCheck(nil, "http://own", 3000, 0, true, nil).testTimeout(); got != 3*time.Second {
		t.Errorf("configured timeout = %v, want 3s over the default", got)
	}
}

func TestTestTimeoutCoversOnlyTheURLsTheCheckTests(t *testing.T) {
	hc := NewHealthCheck(nil, "http://own", 3000, 0, true, nil)
	hc.registerHealthCheckTask("http://group", nil, "", 300)
	pd := &baseProvider{healthCheck: hc}

	for url, tested := range map[string]bool{"http://own": true, " http://group ": true, "http://other": false, "": false} {
		timeout, ok := pd.TestTimeout(url)
		if ok != tested || ok && timeout != 3*time.Second {
			t.Errorf("TestTimeout(%q) = %v, %t; want 3s, %t", url, timeout, ok, tested)
		}
	}

	unset := &baseProvider{healthCheck: NewHealthCheck(nil, "http://own", 0, 0, true, nil)}
	if _, ok := unset.TestTimeout("http://own"); ok {
		t.Error("TestTimeout reported a timeout for a check whose config names none")
	}
}

func directProxies(names ...string) []C.Proxy {
	proxies := make([]C.Proxy, 0, len(names))
	for _, name := range names {
		proxies = append(proxies, adapter.NewProxy(outbound.NewDirectWithOption(outbound.DirectOption{Name: name})))
	}
	return proxies
}

func slowNoContentServer(t *testing.T) (string, chan struct{}) {
	t.Helper()
	requests := make(chan struct{}, 64)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests <- struct{}{}
		time.Sleep(100 * time.Millisecond)
		w.WriteHeader(http.StatusNoContent)
	}))
	t.Cleanup(server.Close)
	return server.URL, requests
}

func assertTested(t *testing.T, proxies []C.Proxy, url string) {
	t.Helper()
	for _, proxy := range proxies {
		if len(proxy.ExtraDelayHistories()[url].History) == 0 {
			t.Errorf("%s was never tested at %s", proxy.Name(), url)
		}
	}
}

func TestCheckTestsProxiesReplacedMidRound(t *testing.T) {
	url, requests := slowNoContentServer(t)
	hc := NewHealthCheck(directProxies("a", "b"), url, 2000, 0, true, nil)
	t.Cleanup(hc.close)

	first := make(chan struct{})
	go func() {
		defer close(first)
		hc.check()
	}()
	<-requests
	replaced := directProxies("a", "b")
	hc.setProxies(replaced)
	hc.check()
	<-first

	assertTested(t, replaced, url)
	if got := len(requests) + 1; got != 4 {
		t.Errorf("%d probes, want 4: the replaced proxies tested once, not once per waiting caller", got)
	}
}

func TestCheckTestsProxiesReplacedRightAfterARound(t *testing.T) {
	url, _ := slowNoContentServer(t)
	hc := NewHealthCheck(directProxies("a", "b"), url, 2000, 0, true, nil)
	t.Cleanup(hc.close)

	hc.check()
	replaced := directProxies("a", "b")
	hc.setProxies(replaced)
	hc.check()

	assertTested(t, replaced, url)
}
