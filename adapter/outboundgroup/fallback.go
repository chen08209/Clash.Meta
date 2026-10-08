package outboundgroup

import (
	"context"
	"encoding/json"
	"errors"
	"sync/atomic"
	"time"

	"github.com/metacubex/mihomo/common/callback"
	N "github.com/metacubex/mihomo/common/net"
	"github.com/metacubex/mihomo/common/utils"
	C "github.com/metacubex/mihomo/constant"
	P "github.com/metacubex/mihomo/constant/provider"
)

type FallbackOption struct{}

type Fallback struct {
	*GroupBase
	disableUDP     bool
	testUrl        string
	selected       string
	expectedStatus string
	aliveCount     atomic.Int32
}

func (f *Fallback) Now() string {
	proxy := f.findAliveProxy(false)
	return proxy.Name()
}

func (f *Fallback) healthCheck() {
	f.GroupBase.healthCheck()
	notifyRouteChange()
}

func (f *Fallback) markProxyFailed(proxy C.Proxy, err error) {
	if proxy == nil || errors.Is(err, C.ErrNotSupport) || errors.Is(err, context.Canceled) {
		return
	}
	markProxyUnavailable(proxy, f.testUrl)
	if f.selected == proxy.Name() {
		f.selected = ""
	}
	notifyRouteChange()
}

// DialContext implements C.ProxyAdapter
func (f *Fallback) DialContext(ctx context.Context, metadata *C.Metadata) (C.Conn, error) {
	proxy := f.findAliveProxy(true)
	ctx, cancel := failoverContext(ctx, f.aliveCount.Load())
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
	proxy := f.findAliveProxy(true)
	ctx, cancel := failoverContext(ctx, f.aliveCount.Load())
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
	return f.findAliveProxy(false).IsL3Protocol(metadata)
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
		"fixed":          f.selected,
		"hidden":         f.Hidden(),
		"icon":           f.Icon(),
		"emptyFallback":  f.EmptyFallback().Name(),
	})
}

// Unwrap implements C.ProxyAdapter
func (f *Fallback) Unwrap(metadata *C.Metadata, touch bool) C.Proxy {
	proxy := f.findAliveProxy(touch)
	return proxy
}

func (f *Fallback) findAliveProxy(touch bool) C.Proxy {
	proxies := f.GetProxies(touch)
	if len(proxies) == 0 {
		return nil
	}

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
		if f.selected != "" && proxy.Name() == f.selected {
			selectedProxy = proxy
		}
		if aliveCount >= 2 && (f.selected == "" || selectedProxy != nil) {
			break
		}
	}
	f.aliveCount.Store(aliveCount)

	if selectedProxy != nil {
		return selectedProxy
	}
	if f.selected != "" {
		f.selected = ""
	}
	if firstAlive != nil {
		return firstAlive
	}

	return proxies[0]
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

	f.selected = name
	if !p.AliveForTestUrl(f.testUrl) {
		ctx, cancel := context.WithTimeout(context.Background(), time.Millisecond*time.Duration(5000))
		defer cancel()
		expectedStatus, _ := utils.NewUnsignedRanges[uint16](f.expectedStatus)
		_, _ = p.URLTest(ctx, f.testUrl, expectedStatus)
	}

	return nil
}

func (f *Fallback) ForceSet(name string) {
	f.selected = name
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
