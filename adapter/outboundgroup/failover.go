package outboundgroup

import (
	"context"
	"time"

	C "github.com/metacubex/mihomo/constant"
)

// A blackholed first member must not spend the whole tunnel dial budget before
// another alive member gets a chance.
const fastFailoverAttemptTimeout = 2 * time.Second

type aliveStateSetter interface {
	SetAliveForTestUrl(string, bool)
}

func markProxyUnavailable(proxy C.Proxy, testURL string) {
	if proxy == nil {
		return
	}
	if setter, ok := proxy.(aliveStateSetter); ok {
		setter.SetAliveForTestUrl(testURL, false)
	}
}

func failoverContext(ctx context.Context, hasAlternative bool) (context.Context, context.CancelFunc) {
	if !hasAlternative {
		return ctx, func() {}
	}
	return context.WithTimeout(ctx, fastFailoverAttemptTimeout)
}
