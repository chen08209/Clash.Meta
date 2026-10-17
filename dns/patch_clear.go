package dns

import (
	"reflect"
	"time"

	"github.com/metacubex/mihomo/component/resolver"
)

// ClearAll forgets what the resolvers learned from the network: every cached
// answer, default-nameserver's included, every client's connection, those only
// a nameserver-policy uses included, and the nameservers the desktop system
// resolver read. Upstream ClearCache and ResetConnection each reach only part
// of that, and mihomo calls them on its own, so they are left as they are.
func ClearAll() {
	var pending []*Resolver
	for _, r := range []resolver.Resolver{resolver.DefaultResolver, resolver.SystemResolver} {
		if rs, ok := r.(Resolvers); ok {
			pending = append(pending, rs.Resolver, rs.ProxyResolver, rs.DirectResolver)
		}
	}
	visited := map[*Resolver]struct{}{}
	reset := map[dnsClient]struct{}{}
	for len(pending) > 0 {
		r := pending[0]
		pending = pending[1:]
		if r == nil {
			continue
		}
		if _, done := visited[r]; done {
			continue
		}
		visited[r] = struct{}{}
		if r.cache != nil {
			r.cache.Clear()
		}
		for _, c := range r.clients() {
			resetClientOnce(c, reset)
		}
		pending = append(pending, r.defaultResolver)
	}
	answeredBy.Clear()
}

// A policy type added upstream needs a case here, or its clients keep their
// connections through ClearAll.
func (r *Resolver) clients() []dnsClient {
	clients := append(append([]dnsClient{}, r.main...), r.fallback...)
	for _, p := range r.policy {
		switch p := p.(type) {
		case domainTriePolicy:
			p.Foreach(func(_ string, policyClients []dnsClient) bool {
				clients = append(clients, policyClients...)
				return true
			})
		case domainMatcherPolicy:
			clients = append(clients, p.dnsClients...)
		}
	}
	return clients
}

// Policies share transports with each other and with main, and resetting one
// twice could drop a connection a query has just opened.
func resetClientOnce(c dnsClient, reset map[dnsClient]struct{}) {
	for {
		u, ok := c.(interface{ Unwrap() dnsClient })
		if !ok {
			break
		}
		c = u.Unwrap()
	}
	if reflect.TypeOf(c).Comparable() {
		if _, done := reset[c]; done {
			return
		}
		reset[c] = struct{}{}
	}
	if sc, ok := c.(*systemClient); ok {
		sc.mu.Lock()
		sc.lastFlush = time.Time{}
		sc.mu.Unlock()
	}
	c.ResetConnection()
}
