package dns

import (
	"context"
	"net"
	"testing"

	D "github.com/miekg/dns"
)

type fakeDNSClient struct{}

func (fakeDNSClient) Address() string  { return "fake" }
func (fakeDNSClient) ResetConnection() {}

func (fakeDNSClient) ExchangeContext(ctx context.Context, m *D.Msg) (*D.Msg, error) {
	resp := &D.Msg{}
	resp.SetReply(m)
	for _, q := range m.Question {
		switch q.Qtype {
		case D.TypeA:
			resp.Answer = append(resp.Answer,
				&D.A{Hdr: D.RR_Header{Name: q.Name, Rrtype: D.TypeA, Class: D.ClassINET, Ttl: 60}, A: net.ParseIP("192.0.2.10")},
				&D.A{Hdr: D.RR_Header{Name: q.Name, Rrtype: D.TypeA, Class: D.ClassINET, Ttl: 60}, A: net.ParseIP("192.0.2.11")},
			)
		case D.TypeAAAA:
			resp.Answer = append(resp.Answer,
				&D.AAAA{Hdr: D.RR_Header{Name: q.Name, Rrtype: D.TypeAAAA, Class: D.ClassINET, Ttl: 60}, AAAA: net.ParseIP("2001:db8::1")},
			)
		}
	}
	return resp, nil
}

func benchmarkWarmLookup(b *testing.B, r *Resolver, fn func(*Resolver, context.Context, string)) {
	b.Helper()
	ctx := context.Background()
	fn(r, ctx, "www.example.com")
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		fn(r, ctx, "www.example.com")
	}
}

func BenchmarkResolverLookupIPv4CacheHit(b *testing.B) {
	r := NewResolverFromClient(fakeDNSClient{})
	benchmarkWarmLookup(b, r, func(r *Resolver, ctx context.Context, host string) {
		if _, err := r.LookupIPv4(ctx, host); err != nil {
			b.Fatal(err)
		}
	})
}

func BenchmarkResolverLookupIPCacheHit(b *testing.B) {
	r := NewResolverFromClient(fakeDNSClient{})
	benchmarkWarmLookup(b, r, func(r *Resolver, ctx context.Context, host string) {
		if _, err := r.LookupIP(ctx, host); err != nil {
			b.Fatal(err)
		}
	})
}
