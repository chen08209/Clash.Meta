package dns

import (
	"context"
	"net/netip"
	"time"

	"github.com/metacubex/mihomo/component/resolver"
	D "github.com/miekg/dns"
	"github.com/samber/lo"
	"golang.org/x/exp/slices"
)

func ipCacheEntryKey(host string, qtype uint16) ipCacheKey {
	return ipCacheKey{host: host, qtype: qtype}
}

// A parsed IP answer avoids copying the full DNS message on every cache hit.
// Query tracing deliberately stays on the message path because a UI record
// carries the original answer and upstream information.
func (r *Resolver) lookupIPFromCache(host string, qtype uint16) ([]netip.Addr, bool) {
	if r == nil || r.ipCache == nil || DefaultQueryNotify != nil {
		return nil, false
	}

	ips, expireTime, hit := r.ipCache.GetWithExpire(ipCacheEntryKey(host, qtype))
	if !hit || !time.Now().Before(expireTime) {
		return nil, false
	}
	return slices.Clone(ips), true
}

func (r *Resolver) lookupIPFromCachePair(host string) ([]netip.Addr, bool) {
	if r == nil || r.ipCache == nil || DefaultQueryNotify != nil {
		return nil, false
	}

	now := time.Now()
	ipv4s, ipv4Expire, ipv4Hit := r.ipCache.GetWithExpire(ipCacheEntryKey(host, D.TypeA))
	if !ipv4Hit || !now.Before(ipv4Expire) {
		return nil, false
	}
	ipv6s, ipv6Expire, ipv6Hit := r.ipCache.GetWithExpire(ipCacheEntryKey(host, D.TypeAAAA))
	if !ipv6Hit || !now.Before(ipv6Expire) {
		return nil, false
	}

	combined := make([]netip.Addr, 0, len(ipv4s)+len(ipv6s))
	combined = append(combined, ipv4s...)
	return append(combined, ipv6s...), true
}

func (r *Resolver) cacheLookupIP(host string, qtype uint16, msg *D.Msg, ips []netip.Addr) {
	if r == nil || r.ipCache == nil || DefaultQueryNotify != nil || msg == nil || len(ips) == 0 {
		return
	}

	ttl := minimalTTL(lo.Concat(msg.Answer, msg.Ns, msg.Extra))
	if ttl == 0 {
		return
	}
	r.ipCache.SetWithExpire(ipCacheEntryKey(host, qtype), slices.Clone(ips), time.Now().Add(time.Duration(ttl)*time.Second))
}

func (r *Resolver) lookupIPCached(ctx context.Context, host string, dnsType uint16) ([]netip.Addr, error) {
	if ips, hit := r.lookupIPFromCache(host, dnsType); hit {
		return ips, nil
	}

	query := &D.Msg{}
	query.SetQuestion(D.Fqdn(host), dnsType)
	msg, err := r.ExchangeContext(ctx, query)
	if err != nil {
		return nil, err
	}

	ips := msgToIP(msg)
	r.cacheLookupIP(host, dnsType, msg, ips)
	if len(ips) == 0 {
		return nil, resolver.ErrIPNotFound
	}
	return ips, nil
}
