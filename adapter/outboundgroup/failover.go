package outboundgroup

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/metacubex/mihomo/common/utils"
	C "github.com/metacubex/mihomo/constant"
	"github.com/metacubex/mihomo/log"
)

// A blackholed first member must not spend the whole tunnel dial budget before
// another alive member gets a chance.
const fastFailoverAttemptTimeout = 2 * time.Second

type aliveStateSetter interface {
	SetAliveForTestUrl(string, bool)
}

// dialFailureMarksNode reports whether a failed dial says anything about the
// health of the member. Direct and passthrough adapters failed the
// destination, not a node.
func dialFailureMarksNode(adapterType C.AdapterType) bool {
	switch adapterType {
	case C.Direct, C.Compatible, C.Reject, C.Pass, C.RejectDrop:
		return false
	}
	return true
}

func shouldMarkProxyFailed(proxy C.Proxy, err error) bool {
	return proxy != nil &&
		dialFailureMarksNode(proxy.Type()) &&
		!errors.Is(err, C.ErrNotSupport) &&
		!errors.Is(err, context.Canceled)
}

func markProxyUnavailable(proxy C.Proxy, testURL string) bool {
	if proxy == nil {
		return false
	}
	setter, ok := proxy.(aliveStateSetter)
	if !ok {
		return false
	}
	setter.SetAliveForTestUrl(testURL, false)
	return true
}

// A node benched by a local dial failure is only re-tested by the periodic
// health check, which defaults to five minutes; one probe after a short
// delay keeps a transient failure from parking a healthy node.
var fastFailoverRecoveryDelay = 10 * time.Second

var failoverRecoveryProbes sync.Map

// scheduleFailedProxyProbe re-tests a proxy this process marked unavailable,
// so it can return to rotation before the next periodic health check.
// Concurrent failures of one proxy and URL share a single probe; a probe
// that fails leaves the node to the periodic check.
func scheduleFailedProxyProbe(
	proxy C.Proxy,
	testURL string,
	expectedStatus utils.IntRanges[uint16],
	timeout time.Duration,
	onRecovered func(),
) {
	if proxy == nil || testURL == "" {
		return
	}
	key := proxy.Name() + "\x00" + testURL
	if _, loaded := failoverRecoveryProbes.LoadOrStore(key, struct{}{}); loaded {
		return
	}
	time.AfterFunc(fastFailoverRecoveryDelay, func() {
		defer failoverRecoveryProbes.Delete(key)
		defer func() {
			if recovered := recover(); recovered != nil {
				log.Errorln("ProxyGroup: recovery probe for %s panicked: %v", proxy.Name(), recovered)
			}
		}()
		ctx, cancel := context.WithTimeout(context.Background(), timeout)
		defer cancel()
		if _, err := proxy.URLTest(ctx, testURL, expectedStatus); err != nil {
			return
		}
		if onRecovered != nil {
			onRecovered()
		}
	})
}

func failoverContext(ctx context.Context, hasAlternative bool) (context.Context, context.CancelFunc) {
	if !hasAlternative {
		return ctx, func() {}
	}
	return context.WithTimeout(ctx, fastFailoverAttemptTimeout)
}
