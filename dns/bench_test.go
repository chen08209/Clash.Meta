package dns

import (
	"context"
	"net/netip"
	"testing"
	"time"

	D "github.com/miekg/dns"
)

func seededBenchResolver() *Resolver {
	return &Resolver{
		ipv6:    true,
		cache:   Config{}.newCache(),
		ipCache: Config{}.newIPCache(),
	}
}

func seedAnswerMessage(name string, ttl uint32) *D.Msg {
	query := &D.Msg{}
	query.SetQuestion(D.Fqdn(name), D.TypeA)
	query.Answer = []D.RR{
		&D.A{Hdr: D.RR_Header{Name: D.Fqdn(name), Rrtype: D.TypeA, Class: D.ClassINET, Ttl: ttl}, A: netip.MustParseAddr("1.2.3.4").AsSlice()},
		&D.A{Hdr: D.RR_Header{Name: D.Fqdn(name), Rrtype: D.TypeA, Class: D.ClassINET, Ttl: ttl}, A: netip.MustParseAddr("1.2.3.5").AsSlice()},
	}
	return query
}

// The message-cache hit path deep-copies the stored answer so callers can
// mutate their copy; this measures that copy on a typical two-answer reply.
func BenchmarkExchangeContextCacheHit(b *testing.B) {
	r := seededBenchResolver()
	query := seedAnswerMessage("cached.example.", 300)
	putMsgToCache(r.cache, questionKey(query.Question[0]), query)
	ctx := context.Background()

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := r.ExchangeContext(ctx, query); err != nil {
			b.Fatal(err)
		}
	}
}

// The tunnel lookups ride the parsed-ip cache; this measures the full
// lookup path including the answer slice clone on a hit.
func BenchmarkLookupIPCached(b *testing.B) {
	r := seededBenchResolver()
	ips := []netip.Addr{netip.MustParseAddr("1.2.3.4"), netip.MustParseAddr("1.2.3.5")}
	r.ipCache.SetWithExpire(ipCacheEntryKey("cached.example", D.TypeA), ips, time.Now().Add(time.Hour))
	ctx := context.Background()

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := r.lookupIPCached(ctx, "cached.example", D.TypeA); err != nil {
			b.Fatal(err)
		}
	}
}
