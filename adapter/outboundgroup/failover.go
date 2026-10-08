package outboundgroup

import (
	"context"
	"errors"
	"time"

	C "github.com/metacubex/mihomo/constant"
)

// A blackholed first member must not spend the whole tunnel dial budget before
// another alive member gets a chance.
const fastFailoverAttemptTimeout = 2 * time.Second

type aliveStateSetter interface {
	SetAliveForTestUrl(string, bool)
}

func shouldMarkProxyFailed(proxy C.Proxy, err error) bool {
	return proxy != nil &&
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

func failoverContext(ctx context.Context, hasAlternative bool) (context.Context, context.CancelFunc) {
	if !hasAlternative {
		return ctx, func() {}
	}
	return context.WithTimeout(ctx, fastFailoverAttemptTimeout)
}
