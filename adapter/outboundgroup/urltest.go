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
	"github.com/metacubex/mihomo/common/singledo"
	"github.com/metacubex/mihomo/common/utils"
	C "github.com/metacubex/mihomo/constant"
	P "github.com/metacubex/mihomo/constant/provider"
)

type URLTestOption struct {
	Tolerance uint16 `group:"tolerance,omitempty"`
}

type URLTest struct {
	*GroupBase
	selectedMu         sync.RWMutex
	selected           string
	testUrl            string
	expectedStatus     string
	tolerance          uint16
	disableUDP         bool
	fastMu             sync.Mutex
	fastNode           C.Proxy
	fastSingle         *singledo.Single[C.Proxy]
	fastHasAlternative atomic.Bool
	rotateProbe        atomic.Bool
	probeCursor        atomic.Uint32
	versionsMu         sync.Mutex
	knownVersions      []uint32
}

func (u *URLTest) Now() string {
	return u.fast(false).Name()
}

func (u *URLTest) Set(name string) error {
	var p C.Proxy
	for _, proxy := range u.GetProxies(false) {
		if proxy.Name() == name {
			p = proxy
			break
		}
	}
	if p == nil {
		return errors.New("proxy not exist")
	}
	u.ForceSet(name)
	return nil
}

func (u *URLTest) selectedName() string {
	u.selectedMu.RLock()
	defer u.selectedMu.RUnlock()
	return u.selected
}

func (u *URLTest) setSelected(name string) {
	u.selectedMu.Lock()
	u.selected = name
	u.selectedMu.Unlock()
}

func (u *URLTest) ForceSet(name string) {
	u.setSelected(name)
	u.resetFastNode()
}

// markProxyFailed stops selecting a proxy after a failed dial, then lets the
// group re-check it soon so a transient failure does not park it until the
// periodic health check.
func (u *URLTest) markProxyFailed(proxy C.Proxy, err error) {
	if !shouldMarkProxyFailed(proxy, err) || !markProxyUnavailable(proxy, u.testUrl) {
		return
	}
	u.resetFastNode()
	u.rotateProbe.Store(true)
	notifyRouteChange()
	u.scheduleRecoveryProbe(proxy)
}

func (u *URLTest) scheduleRecoveryProbe(proxy C.Proxy) {
	expectedStatus, err := utils.NewUnsignedRanges[uint16](u.expectedStatus)
	if err != nil {
		return
	}
	scheduleFailedProxyProbe(
		proxy,
		u.testUrl,
		expectedStatus,
		time.Duration(u.testTimeout)*time.Millisecond,
		func() {
			u.resetFastNode()
			notifyRouteChange()
		},
	)
}

// DialContext implements C.ProxyAdapter
func (u *URLTest) DialContext(ctx context.Context, metadata *C.Metadata) (c C.Conn, err error) {
	proxy := u.fast(true)
	ctx, cancel := failoverContext(ctx, u.fastHasAlternative.Load())
	defer cancel()
	c, err = proxy.DialContext(ctx, metadata)
	if err == nil {
		c.AppendToChains(u)
	} else {
		u.markProxyFailed(proxy, err)
		u.onDialFailed(proxy.Type(), err, u.healthCheck)
	}

	if N.NeedHandshake(c) {
		c = callback.NewFirstWriteCallBackConn(c, func(err error) {
			if err == nil {
				u.onDialSuccess()
			} else {
				u.markProxyFailed(proxy, err)
				u.onDialFailed(proxy.Type(), err, u.healthCheck)
			}
		})
	}

	return c, err
}

// ListenPacketContext implements C.ProxyAdapter
func (u *URLTest) ListenPacketContext(ctx context.Context, metadata *C.Metadata) (C.PacketConn, error) {
	proxy := u.fast(true)
	ctx, cancel := failoverContext(ctx, u.fastHasAlternative.Load())
	defer cancel()
	pc, err := proxy.ListenPacketContext(ctx, metadata)
	if err == nil {
		pc.AppendToChains(u)
	} else {
		u.markProxyFailed(proxy, err)
		u.onDialFailed(proxy.Type(), err, u.healthCheck)
	}

	return pc, err
}

// Unwrap implements C.ProxyAdapter
func (u *URLTest) Unwrap(metadata *C.Metadata, touch bool) C.Proxy {
	return u.fast(touch)
}

func (u *URLTest) healthCheck() {
	u.GroupBase.healthCheck()
	u.resetFastNode()
	notifyRouteChange()
}

func (u *URLTest) shouldReplaceFastNode(fast C.Proxy, fastNotExist bool) bool {
	if u.fastNode == nil || fastNotExist || !u.fastNode.AliveForTestUrl(u.testUrl) {
		return true
	}
	return improvesOnCachedDelay(
		u.fastNode.LastDelayForTestUrl(u.testUrl),
		fast.LastDelayForTestUrl(u.testUrl),
		u.tolerance,
	)
}

func (u *URLTest) fast(touch bool) C.Proxy {
	u.dropCacheForProviderSwap()
	elm, _, shared := u.fastSingle.Do(func() (C.Proxy, error) {
		u.fastMu.Lock()
		defer u.fastMu.Unlock()
		proxies := u.GetProxies(touch)
		if len(proxies) == 0 {
			u.fastHasAlternative.Store(false)
			return u.EmptyFallback(), nil
		}

		selectedName := u.selectedName()
		var selectedProxy C.Proxy
		var fast C.Proxy
		var minDelay uint16
		fastNotExist := true
		aliveCount := int32(0)

		for _, proxy := range proxies {
			if u.fastNode != nil && proxy == u.fastNode {
				fastNotExist = false
			}
			if !proxy.AliveForTestUrl(u.testUrl) {
				continue
			}
			aliveCount++
			if proxy.Name() == selectedName {
				selectedProxy = proxy
			}
			if selectedProxy != nil && aliveCount >= 2 {
				break
			}
			delay := proxy.LastDelayForTestUrl(u.testUrl)
			if fast == nil || delay < minDelay {
				fast = proxy
				minDelay = delay
			}
		}
		u.fastHasAlternative.Store(aliveCount > 1)
		if aliveCount > 0 {
			u.rotateProbe.Store(false)
		}

		if selectedProxy != nil {
			u.fastNode = selectedProxy
			return selectedProxy, nil
		}

		if fast == nil {
			index := 0
			if u.rotateProbe.Swap(false) {
				index = int(u.probeCursor.Load()) % len(proxies)
			}
			fast = proxies[index]
			u.probeCursor.Store(uint32((index + 1) % len(proxies)))
		}
		if u.shouldReplaceFastNode(fast, fastNotExist) {
			u.fastNode = fast
		}
		return u.fastNode, nil
	})
	if shared && touch { // a shared fastSingle.Do() may cause providers untouched, so we touch them again
		u.Touch()
	}

	return elm
}

// dropCacheForProviderSwap forgets the cached fast node when a provider has
// replaced its list underneath it: the cached node may no longer be a member,
// and its delay says nothing about the members that took its place.
func (u *URLTest) dropCacheForProviderSwap() {
	u.versionsMu.Lock()
	defer u.versionsMu.Unlock()
	if u.providerVersionsEqual(u.knownVersions) {
		return
	}
	u.knownVersions = u.providerVersionsNow()
	u.resetFastNode()
}

func (u *URLTest) resetFastNode() {
	u.fastMu.Lock()
	defer u.fastMu.Unlock()
	u.fastNode = nil
	u.fastSingle.Reset()
}
// SupportUDP implements C.ProxyAdapter
func (u *URLTest) SupportUDP() bool {
	if u.disableUDP {
		return false
	}
	return u.fast(false).SupportUDP()
}

// IsL3Protocol implements C.ProxyAdapter
func (u *URLTest) IsL3Protocol(metadata *C.Metadata) bool {
	return u.fast(false).IsL3Protocol(metadata)
}

// MarshalJSON implements C.ProxyAdapter
func (u *URLTest) MarshalJSON() ([]byte, error) {
	all := []string{}
	for _, proxy := range u.GetProxies(false) {
		all = append(all, proxy.Name())
	}
	return json.Marshal(map[string]any{
		"type":           u.Type().String(),
		"now":            u.Now(),
		"all":            all,
		"testUrl":        u.testUrl,
		"expectedStatus": u.expectedStatus,
		"fixed":          u.selectedName(),
		"hidden":         u.Hidden(),
		"icon":           u.Icon(),
		"emptyFallback":  u.EmptyFallback().Name(),
	})
}

func (u *URLTest) Providers() []P.ProxyProvider {
	return u.providers
}

func (u *URLTest) Proxies() []C.Proxy {
	return u.GetProxies(false)
}

func (u *URLTest) URLTest(ctx context.Context, url string, expectedStatus utils.IntRanges[uint16]) (map[string]uint16, error) {
	return u.GroupBase.URLTest(ctx, u.testUrl, expectedStatus)
}

func NewURLTest(option GroupCommonOption, urlTestOption URLTestOption, emptyFallback C.Proxy, providers []P.ProxyProvider) (*URLTest, error) {
	if emptyFallback == nil {
		return nil, errors.New("empty fallback proxy not exist")
	}
	urlTest := &URLTest{
		GroupBase: NewGroupBase(GroupBaseOption{
			Name:           option.Name,
			Type:           C.URLTest,
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
		fastSingle:     singledo.NewSingle[C.Proxy](time.Second * 10),
		disableUDP:     option.DisableUDP,
		testUrl:        option.URL,
		expectedStatus: option.ExpectedStatus,
		tolerance:      urlTestOption.Tolerance,
	}

	return urlTest, nil
}
