package dns

import (
	"context"
	"errors"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/metacubex/mihomo/component/resolver"
	icontext "github.com/metacubex/mihomo/context"

	D "github.com/miekg/dns"
)

type answeringClient struct {
	address string
	ip      string
}

func (c *answeringClient) ExchangeContext(_ context.Context, m *D.Msg) (*D.Msg, error) {
	reply := new(D.Msg)
	reply.SetReply(m)
	reply.Answer = []D.RR{&D.A{
		Hdr: D.RR_Header{Name: m.Question[0].Name, Rrtype: D.TypeA, Class: D.ClassINET, Ttl: 60},
		A:   net.ParseIP(c.ip).To4(),
	}}
	return reply, nil
}

func (c *answeringClient) Address() string { return c.address }

func (c *answeringClient) ResetConnection() {}

func recordQueries(t *testing.T) func() []QueryRecord {
	t.Helper()
	var mu sync.Mutex
	var records []QueryRecord
	previous := DefaultQueryNotify
	DefaultQueryNotify = func(record QueryRecord) {
		mu.Lock()
		records = append(records, record)
		mu.Unlock()
	}
	t.Cleanup(func() { DefaultQueryNotify = previous })
	return func() []QueryRecord {
		mu.Lock()
		defer mu.Unlock()
		return append([]QueryRecord(nil), records...)
	}
}

type failingClient struct{}

func (failingClient) ExchangeContext(context.Context, *D.Msg) (*D.Msg, error) {
	return nil, errors.New("upstream unreachable")
}

func (failingClient) Address() string { return "udp://192.0.2.99:53" }

func (failingClient) ResetConnection() {}

func waitForRecords(t *testing.T, records func() []QueryRecord, want int) []QueryRecord {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for {
		got := records()
		if len(got) >= want || time.Now().After(deadline) {
			if len(got) != want {
				t.Fatalf("recorded %d queries, want %d: %+v", len(got), want, got)
			}
			return got
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func newAnsweringResolver(address string) *Resolver {
	return &Resolver{
		main:  []dnsClient{&answeringClient{address: address, ip: "198.51.100.7"}},
		cache: Config{}.newCache(),
	}
}

func TestLookupReportsInitiatorUpstreamAndCacheHit(t *testing.T) {
	records := recordQueries(t)
	r := newAnsweringResolver("udp://192.0.2.53:53")
	ctx := resolver.WithInitiator(context.Background(), resolver.InitiatorDirect)

	for i := 0; i < 2; i++ {
		if _, err := r.LookupIPv4(ctx, "direct.example"); err != nil {
			t.Fatal(err)
		}
	}

	got := records()
	if len(got) != 2 {
		t.Fatalf("recorded %d queries, want 2", len(got))
	}
	for i, wantCached := range []bool{false, true} {
		record := got[i]
		if record.Initiator != resolver.InitiatorDirect || record.Upstream != "udp://192.0.2.53:53" || record.Cached != wantCached {
			t.Errorf("query %d = initiator %q, upstream %q, cached %v", i, record.Initiator, record.Upstream, record.Cached)
		}
		if record.Question.Name != "direct.example." || record.Err != nil || record.Msg == nil {
			t.Errorf("query %d = %+v", i, record)
		}
	}
}

func TestUnlabeledQueryIsOther(t *testing.T) {
	records := recordQueries(t)
	r := newAnsweringResolver("https://dns.example/dns-query")
	query := new(D.Msg)
	query.SetQuestion("ech.example.", D.TypeHTTPS)

	if _, err := r.ExchangeContext(context.Background(), query); err != nil {
		t.Fatal(err)
	}

	got := records()
	if len(got) != 1 || got[0].Initiator != resolver.InitiatorOther || got[0].Question.Qtype != D.TypeHTTPS {
		t.Fatalf("records = %+v", got)
	}
}

func TestFailedExchangeDropsThePreviousUpstream(t *testing.T) {
	records := recordQueries(t)
	r := newAnsweringResolver("udp://192.0.2.53:53")
	if _, err := r.LookupIPv4(context.Background(), "flaky.example"); err != nil {
		t.Fatal(err)
	}
	r.main = []dnsClient{failingClient{}}
	r.cache = Config{}.newCache()

	if _, err := r.LookupIPv4(context.Background(), "flaky.example"); err == nil {
		t.Fatal("lookup through a failing upstream succeeded")
	}

	// A failed exchange is retried once in the background, as its own exchange.
	got := waitForRecords(t, records, 3)
	for _, record := range got[1:] {
		if record.Err == nil || record.Upstream != "" {
			t.Fatalf("records = %+v", got)
		}
	}
}

type blockingClient struct {
	answeringClient
	entered chan struct{}
	release chan struct{}
}

func (c *blockingClient) ExchangeContext(ctx context.Context, m *D.Msg) (*D.Msg, error) {
	c.entered <- struct{}{}
	<-c.release
	return c.answeringClient.ExchangeContext(ctx, m)
}

func TestCallersSharingAnExchangeRecordItOnce(t *testing.T) {
	records := recordQueries(t)
	client := &blockingClient{
		answeringClient: answeringClient{address: "udp://192.0.2.53:53", ip: "198.51.100.7"},
		entered:         make(chan struct{}, 1),
		release:         make(chan struct{}),
	}
	r := &Resolver{main: []dnsClient{client}, cache: Config{}.newCache()}

	var wg sync.WaitGroup
	lookup := func() {
		defer wg.Done()
		if _, err := r.LookupIPv4(context.Background(), "shared.example"); err != nil {
			t.Error(err)
		}
	}
	wg.Add(2)
	go lookup()
	<-client.entered
	go lookup()
	time.Sleep(50 * time.Millisecond)
	close(client.release)
	wg.Wait()

	exchanges := 0
	for _, record := range records() {
		if !record.Cached {
			exchanges++
		}
	}
	if exchanges != 1 {
		t.Fatalf("recorded %d exchanges, want 1: %+v", exchanges, records())
	}
}

func TestServeMsgReportsTheAppQueryOnce(t *testing.T) {
	records := recordQueries(t)
	r := newAnsweringResolver("tls://192.0.2.1:853")
	service := &Service{handler: withResolver(r, true)}
	query := new(D.Msg)
	query.SetQuestion("app.example.", D.TypeA)

	if _, err := service.ServeMsg(context.Background(), query); err != nil {
		t.Fatal(err)
	}

	got := records()
	if len(got) != 1 {
		t.Fatalf("recorded %d queries, want only the app query", len(got))
	}
	record := got[0]
	if record.Initiator != resolver.InitiatorApp || record.Upstream != "tls://192.0.2.1:853" || record.Cached {
		t.Fatalf("record = initiator %q, upstream %q, cached %v", record.Initiator, record.Upstream, record.Cached)
	}
}

func TestAnswersThatSkipTheResolverAreNotRecorded(t *testing.T) {
	records := recordQueries(t)
	service := &Service{handler: func(ctx *icontext.DNSContext, m *D.Msg) (*D.Msg, error) {
		ctx.SetType(icontext.DNSTypeFakeIP)
		return handleMsgWithEmptyAnswer(m), nil
	}}
	query := new(D.Msg)
	query.SetQuestion("fake.example.", D.TypeA)

	if _, err := service.ServeMsg(context.Background(), query); err != nil {
		t.Fatal(err)
	}

	if got := records(); len(got) != 0 {
		t.Fatalf("records = %+v", got)
	}
}
