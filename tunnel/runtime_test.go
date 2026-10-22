package tunnel

import (
	"fmt"
	"sync"
	"testing"

	C "github.com/metacubex/mihomo/constant"
	rulesCommon "github.com/metacubex/mihomo/rules/common"
)

func TestRuntimeSnapshotUpdatesWhileResolving(t *testing.T) {
	oldRuntime := loadRuntime()
	proxies := map[string]C.Proxy{"DIRECT": directTestProxy{}}

	configMux.Lock()
	currentRuntime.Store(&runtimeSnapshot{
		proxies: proxies,
		mode:    Rule,
	})
	configMux.Unlock()
	t.Cleanup(func() {
		configMux.Lock()
		currentRuntime.Store(oldRuntime)
		invalidateAllProxies()
		configMux.Unlock()
	})

	var workers sync.WaitGroup
	resolveDone := make(chan struct{})
	workers.Add(1)
	go func() {
		defer workers.Done()
		defer close(resolveDone)
		for i := 0; i < 5000; i++ {
			metadata := &C.Metadata{
				NetWork:      C.TCP,
				Type:         C.HTTP,
				Host:         fmt.Sprintf("flow-%d.example.com", i),
				SpecialProxy: "DIRECT",
			}
			proxy, _, err := resolveMetadata(metadata)
			if err != nil {
				t.Errorf("resolveMetadata: %v", err)
				return
			}
			if proxy == nil || proxy.Name() != "DIRECT" {
				t.Errorf("proxy = %v, want DIRECT", proxy)
				return
			}
		}
	}()

	for worker := 0; worker < 2; worker++ {
		workers.Add(1)
		go func(worker int) {
			defer workers.Done()
			for i := 0; i < 1000; i++ {
				UpdateRules(
					[]C.Rule{
						rulesCommon.NewDomain(
							fmt.Sprintf("worker-%d-%d.example.com", worker, i),
							"DIRECT",
						),
					},
					nil,
					nil,
				)
				UpdateProxies(proxies, nil)
			}
		}(worker)
	}

	workers.Wait()
	<-resolveDone
}
