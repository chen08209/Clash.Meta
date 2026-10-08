package outboundgroup

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"sync/atomic"
	"time"

	"github.com/metacubex/mihomo/common/callback"
	N "github.com/metacubex/mihomo/common/net"
	"github.com/metacubex/mihomo/common/utils"
	C "github.com/metacubex/mihomo/constant"
	P "github.com/metacubex/mihomo/constant/provider"
)

type FallbackOption struct{}

type fallbackSelection struct {
	proxy          C.Proxy
	hasAlternative bool
	at             time.Time
}

// A resolved pick is reused briefly like URLTest fast node, so a large
// fallback group does not walk its members on every connection.
const fallbackSelectionTTL = 10 * time.Second

type Fallback struct {
	*GroupBase
	disableUDP     bool
	testUrl        string
	selectedMu     sync.RWMutex
	selected       string
	expectedStatus string
	rotateProbe    atomic.Bool
	probeCursor    atomic.Uint32
	selectionMu    sync.Mutex
	selection      *fallbackSelection
}

func (f *Fallback) Now() string {
	proxy := f.findAliveProxy(false)
	return proxy.Name()
}

func (f *Fallback) healthCheck() {
	f.clearSelection()
	f.GroupBase.healthCheck()
	f.clearSelection()
	notifyRouteChange()
}

func (f *Fallback) selectedName() string {
	f.selectedMu.RLock()
	defer f.selectedMu.RUnlock()
	return f.selected
}

func (f *Fallback) setSelected(name string) {
	f.selectedMu.Lock()
	f.selected = name
	f.selectedMu.Unlock()
	f.clearSelection()
}

func (f *Fallback) clearSelected(name string) {
	f.selectedMu.Lock()
	if f.selected == name {
		f.selected = ""
	}
	f.selectedMu.Unlock()
	f.clearSelection()
}

func (f *Fallback) markProxyFailed(proxy C.Proxy, err error) {
	if !shouldMarkProxyFailed(proxy, err) || !markProxyUnavailable(proxy, f.testUrl) {
		return
	}
	f.clearSelected(proxy.Name())
	f.rotateProbe.Store(true)
	notifyRouteChange()
	f.scheduleRecoveryProbe(proxy)
}

func (f *Fallback) scheduleRecoveryProbe(proxy C.Proxy) {
	expectedStatus, err := utils.NewUnsignedRanges[uint16](f.expectedStatus)
	if err != nil {
		return
	}
	scheduleFailedProxyProbe(
		proxy,
		f.testUrl,
		expectedStatus,
		time.Duration(f.testTimeout)*time.Millisecond,
		notifyRouteChange,
	)
}

// DialContext implements C.ProxyAdapter
func (f *Fallback) DialContext(ctx context.Context, metadata *C.Metadata) (C.Conn, error) {
	proxy, hasAlternative := f.findAliveProxyState(true)
	ctx, cancel := failoverContext(ctx, hasAlternative)
	defer cancel()
	c, err := proxy.DialContext(ctx, metadata)
	if err == nil {
		c.AppendToChains(f)
	} else {
		f.markProxyFailed(proxy, err)
		f.onDialFailed(proxy.Type(), err, f.healthCheck)
	}

	if N.NeedHandshake(c) {
		c = callback.NewFirstWriteCallBackConn(c, func(err error) {
			if err == nil {
				f.onDialSuccess()
			} else {
				f.markProxyFailed(proxy, err)
				f.onDialFailed(proxy.Type(), err, f.healthCheck)
			}
		})
	}

	return c, err
}

// ListenPacketContext implements C.ProxyAdapter
func (f *Fallback) ListenPacketContext(ctx context.Context, metadata *C.Metadata) (C.PacketConn, error) {
	proxy, hasAlternative := f.findAliveProxyState(true)
	ctx, cancel := failoverContext(ctx, hasAlternative)
	defer cancel()
	pc, err := proxy.ListenPacketContext(ctx, metadata)
	if err == nil {
		pc.AppendToChains(f)
	} else {
		f.markProxyFailed(proxy, err)
		f.onDialFailed(proxy.Type(), err, f.healthCheck)
	}

	return pc, err
}

// SupportUDP implements C.ProxyAdapter
func (f *Fallback) SupportUDP() bool {
	if f.disableUDP {
		return false
	}

	proxy := f.findAliveProxy(false)
	return proxy.SupportUDP()
}

// IsL3Protocol implements C.ProxyAdapter
func (f *Fallback) IsL3Protocol(metadata *C.Metadata) bool {
	proxy := f.findAliveProxy(false)
	return proxy.IsL3Protocol(metadata)
}

// MarshalJSON implements C.ProxyAdapter
func (f *Fallback) MarshalJSON() ([]byte, error) {
	all := []string{}
	for _, proxy := range f.GetProxies(false) {
		all = append(all, proxy.Name())
	}
	return json.Marshal(map[string]any{
		"type":           f.Type().String(),
		"now":            f.Now(),
		"all":            all,
		"testUrl":        f.testUrl,
		"expectedStatus": f.expectedStatus,
		"fixed":          f.selectedName(),
		"hidden":         f.Hidden(),
		"icon":           f.Icon(),
		"emptyFallback":  f.EmptyFallback().Name(),
	})
}

// Unwrap implements C.ProxyAdapter
func (f *Fallback) Unwrap(metadata *C.Metadata, touch bool) C.Proxy {
	return f.findAliveProxy(touch)
}

func (f *Fallback) clearSelection() {
	f.selectionMu.Lock()
	f.selection = nil
	f.selectionMu.Unlock()
}

func (f *Fallback) cachedSelection() (fallbackSelection, bool) {
	f.selectionMu.Lock()
	defer f.selectionMu.Unlock()
	selection := f.selection
	if selection == nil || time.Since(selection.at) >= fallbackSelectionTTL {
		return fallbackSelection{}, false
	}
	return *selection, true
}

func (f *Fallback) storeSelection(proxy C.Proxy, hasAlternative bool) {
	f.selectionMu.Lock()
	f.selection = &fallbackSelection{
		proxy:          proxy,
		hasAlternative: hasAlternative,
		at:             time.Now(),
	}
	f.selectionMu.Unlock()
}

func (f *Fallback) findAliveProxy(touch bool) C.Proxy {
	proxy, _ := f.findAliveProxyState(touch)
	return proxy
}

func (f *Fallback) findAliveProxyState(touch bool) (C.Proxy, bool) {
	if selection, ok := f.cachedSelection(); ok {
		if touch {
			f.Touch()
		}
		return selection.proxy, selection.hasAlternative
	}
	proxy, hasAlternative := f.resolveAliveProxyState(touch)
	if proxy != f.EmptyFallback() && proxy.AliveForTestUrl(f.testUrl) {
		f.storeSelection(proxy, hasAlternative)
	}
	return proxy, hasAlternative
}

func (f *Fallback) resolveAliveProxyState(touch bool) (C.Proxy, bool) {
	proxies := f.GetProxies(touch)
	if len(proxies) == 0 {
		return f.EmptyFallback(), false
	}

	selectedName := f.selectedName()
	var selectedProxy C.Proxy
	var firstAlive C.Proxy
	aliveCount := int32(0)
	for _, proxy := range proxies {
		if !proxy.AliveForTestUrl(f.testUrl) {
			continue
		}
		aliveCount++
		if firstAlive == nil {
			firstAlive = proxy
		}
		if selectedName != "" && proxy.Name() == selectedName {
			selectedProxy = proxy
		}
		if aliveCount >= 2 && (selectedName == "" || selectedProxy != nil) {
			break
		}
	}
	hasAlternative := aliveCount > 1
	if aliveCount > 0 {
		f.rotateProbe.Store(false)
	}
	if selectedProxy != nil {
		return selectedProxy, hasAlternative
	}
	if selectedName != "" {
		f.clearSelected(selectedName)
	}
	if firstAlive != nil {
		return firstAlive, hasAlternative
	}

	index := 0
	if f.rotateProbe.Swap(false) {
		index = int(f.probeCursor.Load()) % len(proxies)
	}
	f.probeCursor.Store(uint32((index + 1) % len(proxies)))
	return proxies[index], false
}

func (f *Fallback) Set(name string) error {
	var p C.Proxy
	for _, proxy := range f.GetProxies(false) {
		if proxy.Name() == name {
			p = proxy
			break
		}
	}

	if p == nil {
		return errors.New("proxy not exist")
	}

	f.setSelected(name)
	if !p.AliveForTestUrl(f.testUrl) {
		timeout := time.Duration(f.testTimeout) * time.Millisecond
		if timeout <= 0 {
			timeout = 5 * time.Second
		}
		ctx, cancel := context.WithTimeout(context.Background(), timeout)
		defer cancel()
		expectedStatus, _ := utils.NewUnsignedRanges[uint16](f.expectedStatus)
		_, _ = p.URLTest(ctx, f.testUrl, expectedStatus)
	}

	return nil
}

func (f *Fallback) ForceSet(name string) {
	f.setSelected(name)
}

func (f *Fallback) Providers() []P.ProxyProvider {
	return f.providers
}

func (f *Fallback) Proxies() []C.Proxy {
	return f.GetProxies(false)
}

func NewFallback(option GroupCommonOption, fallbackOption FallbackOption, emptyFallback C.Proxy, providers []P.ProxyProvider) (*Fallback, error) {
	return &Fallback{
		GroupBase: NewGroupBase(GroupBaseOption{
			Name:           option.Name,
			Type:           C.Fallback,
			Hidden:         option.Hidden,
			Icon:           option.Icon,
			Filter:         option.Filter,
			ExcludeFilter:  option.ExcludeFilter,
			ExcludeType:    option.ExcludeType,
			TestTimeout:    option.TestTimeout,
			MaxFailedTimes: option.MaxFailedTimes,
			EmptyFallback:  emptyFallback,
			Providers:      providers,
			UsePosition:    option.UsePosition,
		}),
		disableUDP:     option.DisableUDP,
		testUrl:        option.URL,
		expectedStatus: option.ExpectedStatus,
	}, nil
}
