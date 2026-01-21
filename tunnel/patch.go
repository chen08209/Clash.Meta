package tunnel

import (
	"sync"

	C "github.com/metacubex/mihomo/constant"
	P "github.com/metacubex/mihomo/constant/provider"
)

var (
	allProxiesMu sync.Mutex
	// Published copy-on-write: a rebuild installs a new map rather than writing
	// to one a caller may still be reading.
	allProxies map[string]C.Proxy
	// Provider version the cache was built from, per provider name.
	allProxiesVersions map[string]uint32
)

// AllProxies returns every proxy the tunnel can route to — the ones the config
// declared plus the ones its providers carry — keyed by name. The result is
// shared and must not be modified.
//
// The answer is cached because rebuilding it costs one entry per proxy, which
// on a large subscription is megabytes of garbage per call. Only two things
// invalidate it: an apply replacing the maps, which calls invalidateAllProxies,
// and a provider swapping its list, which bumps its own Version.
func AllProxies() map[string]C.Proxy {
	// configMux before allProxiesMu, never the other way round: UpdateProxies
	// takes them in that order.
	configMux.RLock()
	defer configMux.RUnlock()

	allProxiesMu.Lock()
	defer allProxiesMu.Unlock()

	currentProviders := providers
	if allProxies != nil && allProxiesUpToDate(currentProviders) {
		return allProxies
	}

	rebuilt := make(map[string]C.Proxy)
	for name, proxy := range proxies {
		rebuilt[name] = proxy
	}
	versions := make(map[string]uint32, len(currentProviders))
	for name, p := range currentProviders {
		// Version before proxies. The other order can stamp a version newer
		// than the list that was read, and the cache never recovers.
		versions[name] = p.Version()
		for _, proxy := range p.Proxies() {
			rebuilt[proxy.Name()] = proxy
		}
	}

	allProxies = rebuilt
	allProxiesVersions = versions
	return allProxies
}

func allProxiesUpToDate(currentProviders map[string]P.ProxyProvider) bool {
	if len(currentProviders) != len(allProxiesVersions) {
		return false
	}
	for name, p := range currentProviders {
		version, tracked := allProxiesVersions[name]
		if !tracked || version != p.Version() {
			return false
		}
	}
	return true
}

// InvalidateAllProxies drops the cache for a host that would rather pay for one
// rebuild than keep the map resident. Correctness never needs it: the cache is
// the last reference to the proxies a provider already replaced, and it holds
// them until a read that may not come while the host's proxy view is closed.
func InvalidateAllProxies() {
	invalidateAllProxies()
}

// invalidateAllProxies drops the cache. UpdateProxies calls it because an apply
// replaces both maps outright, which no provider version can report.
func invalidateAllProxies() {
	allProxiesMu.Lock()
	defer allProxiesMu.Unlock()
	allProxies = nil
	allProxiesVersions = nil
}

// ProvidersSnapshot returns the proxy providers the tunnel holds, read under
// the lock an apply installs them with — Providers() takes none. The maps are
// never written in place, so the result stays valid after the lock is dropped.
func ProvidersSnapshot() map[string]P.ProxyProvider {
	configMux.RLock()
	defer configMux.RUnlock()
	return providers
}

// RuleProvidersSnapshot is ProvidersSnapshot for rule providers.
func RuleProvidersSnapshot() map[string]P.RuleProvider {
	configMux.RLock()
	defer configMux.RUnlock()
	return ruleProviders
}
