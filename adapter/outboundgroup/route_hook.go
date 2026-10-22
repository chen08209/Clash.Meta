package outboundgroup

import "sync"

var (
	routeInvalidateMu sync.RWMutex
	routeInvalidate   func()
)

// SetRouteInvalidate installs a process-wide hook for a pick that may have
// moved without going through a selector write.
func SetRouteInvalidate(fn func()) {
	routeInvalidateMu.Lock()
	routeInvalidate = fn
	routeInvalidateMu.Unlock()
}

func notifyRouteChange() {
	routeInvalidateMu.RLock()
	hook := routeInvalidate
	routeInvalidateMu.RUnlock()
	if hook != nil {
		hook()
	}
}
