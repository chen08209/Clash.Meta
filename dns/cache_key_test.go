package dns

import (
	"context"
	"sync/atomic"
	"testing"

	D "github.com/miekg/dns"
	"github.com/stretchr/testify/require"
)

type countingDNSClient struct {
	stubDNSClient
	queries atomic.Int64
}

func (c *countingDNSClient) ExchangeContext(ctx context.Context, m *D.Msg) (*D.Msg, error) {
	c.queries.Add(1)
	return c.stubDNSClient.ExchangeContext(ctx, m)
}

func TestResolverCacheKeySeparatesQueryTypes(t *testing.T) {
	client := &countingDNSClient{}
	r := NewResolverFromClient(client)
	ctx := context.Background()

	ips, err := r.LookupIPv4(ctx, "www.example.com")
	require.NoError(t, err)
	require.NotEmpty(t, ips)
	require.EqualValues(t, 1, client.queries.Load())

	ips, err = r.LookupIPv4(ctx, "www.example.com")
	require.NoError(t, err)
	require.NotEmpty(t, ips)
	require.EqualValues(t, 1, client.queries.Load(), "a repeated A query must hit the cache")

	ips, err = r.LookupIPv6(ctx, "www.example.com")
	require.NoError(t, err)
	require.NotEmpty(t, ips)
	require.EqualValues(t, 2, client.queries.Load(), "the AAAA answer must not reuse the A entry")

	ips, err = r.LookupIPv6(ctx, "www.example.com")
	require.NoError(t, err)
	require.NotEmpty(t, ips)
	require.EqualValues(t, 2, client.queries.Load(), "a repeated AAAA query must hit the cache")

	ips, err = r.LookupIPv4(ctx, "other.example.com")
	require.NoError(t, err)
	require.NotEmpty(t, ips)
	require.EqualValues(t, 3, client.queries.Load(), "a different name must miss the cache")
}
