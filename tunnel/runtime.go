package tunnel

import (
	"sync"
	"sync/atomic"

	"github.com/metacubex/mihomo/component/sniffer"
	C "github.com/metacubex/mihomo/constant"
	P "github.com/metacubex/mihomo/constant/provider"
)

type runtimeSnapshot struct {
	rules             []C.Rule
	ruleIndex         *ruleIndex
	listeners         map[string]C.InboundListener
	subRules          map[string][]C.Rule
	subRuleIndexes    map[string]*ruleIndex
	proxies           map[string]C.Proxy
	providers         map[string]P.ProxyProvider
	ruleProviders     map[string]P.RuleProvider
	mode              TunnelMode
	snifferDispatcher *sniffer.Dispatcher
	sniffingEnable    bool
}

var (
	configMux      sync.Mutex
	currentRuntime atomic.Pointer[runtimeSnapshot]
)

func init() {
	currentRuntime.Store(&runtimeSnapshot{
		listeners: make(map[string]C.InboundListener),
		proxies:   make(map[string]C.Proxy),
		mode:      Rule,
	})
}

func loadRuntime() *runtimeSnapshot {
	return currentRuntime.Load()
}

func updateRuntime(mutate func(*runtimeSnapshot)) {
	configMux.Lock()
	defer configMux.Unlock()

	next := cloneRuntime(loadRuntime())
	mutate(next)
	currentRuntime.Store(next)
}

func cloneRuntime(source *runtimeSnapshot) *runtimeSnapshot {
	// Published fields are immutable. An update clones only the collection it
	// replaces and carries every other reference forward without a deep copy.
	next := *source
	return &next
}

func cloneRules(source []C.Rule) []C.Rule {
	if source == nil {
		return nil
	}
	return append([]C.Rule(nil), source...)
}

func cloneSubRules(source map[string][]C.Rule) map[string][]C.Rule {
	if source == nil {
		return nil
	}
	result := make(map[string][]C.Rule, len(source))
	for key, value := range source {
		result[key] = append([]C.Rule(nil), value...)
	}
	return result
}

func cloneMap[M ~map[K]V, K comparable, V any](source M) M {
	if source == nil {
		return nil
	}
	result := make(M, len(source))
	for key, value := range source {
		result[key] = value
	}
	return result
}
