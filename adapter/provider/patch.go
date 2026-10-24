package provider

import (
	"strings"
	"sync/atomic"
	"time"
)

const fallbackTestTimeout = 5 * time.Second

// What a health check waits when its config names no timeout: the host's own
// delay test timeout, so a scheduled check and a manual test agree.
var defaultTestTimeout atomic.Int64

func (pp *proxySetProvider) GetSubscriptionInfo() *SubscriptionInfo {
	return pp.subscriptionInfo
}

func SetDefaultTestTimeout(timeout time.Duration) {
	if timeout > 0 {
		defaultTestTimeout.Store(int64(timeout))
	}
}

func (hc *HealthCheck) testTimeout() time.Duration {
	if hc.timeout > 0 {
		return hc.timeout
	}
	if timeout := time.Duration(defaultTestTimeout.Load()); timeout > 0 {
		return timeout
	}
	return fallbackTestTimeout
}

// TestTimeout reports the timeout the config gives the health check that tests
// this provider's proxies at url, and false when it names none or never tests url.
func (bp *baseProvider) TestTimeout(url string) (time.Duration, bool) {
	hc := bp.healthCheck
	url = strings.TrimSpace(url)
	if hc.timeout <= 0 || url == "" {
		return 0, false
	}
	if url == hc.url {
		return hc.timeout, true
	}
	hc.mu.Lock()
	defer hc.mu.Unlock()
	_, tested := hc.extra[url]
	return hc.timeout, tested
}
