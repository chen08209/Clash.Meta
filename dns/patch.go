package dns

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/metacubex/mihomo/common/lru"
	"github.com/metacubex/mihomo/component/resolver"
	icontext "github.com/metacubex/mihomo/context"

	D "github.com/miekg/dns"
)

type QueryRecord struct {
	Question  D.Question
	Msg       *D.Msg
	Initiator string
	Upstream  string
	Cached    bool
	Start     time.Time
	Err       error
}

type QueryNotify func(record QueryRecord)

var DefaultQueryNotify QueryNotify

// Queries from apps reach the resolver through withResolver, which passes the
// DNSContext that Service.ServeMsg created as the context itself.
func queryInitiator(ctx context.Context) string {
	if initiator := resolver.InitiatorFrom(ctx); initiator != "" {
		return initiator
	}
	if _, ok := ctx.(*icontext.DNSContext); ok {
		return resolver.InitiatorApp
	}
	return resolver.InitiatorOther
}

// A cached answer keeps no trace of the upstream that gave it, so that is
// remembered per resolver and question when the exchange finishes.
var answeredBy = lru.New[string, string](lru.WithSize[string, string](4096))

func answeredByKey(r *Resolver, q D.Question) string {
	return fmt.Sprintf("%p %s", r, q.String())
}

func (r *Resolver) traceCacheHit(ctx context.Context, q D.Question, msg *D.Msg) {
	notify := DefaultQueryNotify
	if notify == nil {
		return
	}
	upstream, _ := answeredBy.Get(answeredByKey(r, q))
	notify(QueryRecord{
		Question:  q,
		Msg:       msg,
		Initiator: queryInitiator(ctx),
		Upstream:  upstream,
		Cached:    true,
		Start:     time.Now(),
	})
}

type exchangeTrace struct {
	notify    QueryNotify
	resolver  *Resolver
	question  D.Question
	initiator string
	start     time.Time

	mu        sync.Mutex
	upstreams map[*D.Msg]string
}

type exchangeTraceKey struct{}

func (r *Resolver) traceExchange(ctx context.Context, q D.Question, initiator string) (context.Context, *exchangeTrace) {
	notify := DefaultQueryNotify
	if notify == nil {
		return ctx, nil
	}
	trace := &exchangeTrace{
		notify:    notify,
		resolver:  r,
		question:  q,
		initiator: initiator,
		start:     time.Now(),
		upstreams: map[*D.Msg]string{},
	}
	return context.WithValue(ctx, exchangeTraceKey{}, trace), trace
}

func traceAnswer(ctx context.Context, msg *D.Msg, upstream string) {
	trace, _ := ctx.Value(exchangeTraceKey{}).(*exchangeTrace)
	if trace == nil || msg == nil {
		return
	}
	trace.mu.Lock()
	trace.upstreams[msg] = upstream
	trace.mu.Unlock()
}

func (t *exchangeTrace) finish(msg *D.Msg, err error) {
	if t == nil {
		return
	}
	record := QueryRecord{
		Question:  t.question,
		Msg:       msg,
		Initiator: t.initiator,
		Start:     t.start,
		Err:       err,
	}
	if err == nil {
		t.mu.Lock()
		record.Upstream = t.upstreams[msg]
		t.mu.Unlock()
		if record.Upstream != "" {
			answeredBy.Set(answeredByKey(t.resolver, t.question), record.Upstream)
		}
	}
	t.notify(record)
}
