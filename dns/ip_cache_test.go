package dns

import (
	"context"
	"net/netip"
	"testing"

	D "github.com/miekg/dns"
	"github.com/stretchr/testify/require"
)

func TestLookupIPUsesParsedCache(t *testing.T) {
	client := &countingDNSClient{}
	r := NewResolverFromClient(client)
	ctx := context.Background()

	ips, err := r.LookupIP(ctx, "www.example.com")
	require.NoError(t, err)
	require.NotEmpty(t, ips)
	require.EqualValues(t, 2, client.queries.Load(), "the first A and AAAA queries must both be exchanged")

	repeated, err := r.LookupIP(ctx, "www.example.com")
	require.NoError(t, err)
	require.Equal(t, ips, repeated)
	require.EqualValues(t, 2, client.queries.Load(), "the parsed A/AAAA result must not dial upstream again")
	_, _, hit := r.ipCache.GetWithExpire(ipCacheEntryKey("www.example.com", D.TypeA))
	require.True(t, hit)
	_, _, hit = r.ipCache.GetWithExpire(ipCacheEntryKey("www.example.com", D.TypeAAAA))
	require.True(t, hit)

	r.ClearCache()
	_, _, hit = r.ipCache.GetWithExpire(ipCacheEntryKey("www.example.com", D.TypeA))
	require.False(t, hit)
	_, _, hit = r.ipCache.GetWithExpire(ipCacheEntryKey("www.example.com", D.TypeAAAA))
	require.False(t, hit)
}

func TestLookupIPCachePairKeepsAddressOrder(t *testing.T) {
	client := &countingDNSClient{}
	r := NewResolverFromClient(client)
	ctx := context.Background()

	_, err := r.LookupIP(ctx, "www.example.com")
	require.NoError(t, err)

	ips, hit := r.lookupIPFromCachePair("www.example.com")
	require.True(t, hit)
	require.Equal(t, []netip.Addr{
		netip.MustParseAddr("192.0.2.10"),
		netip.MustParseAddr("192.0.2.11"),
		netip.MustParseAddr("2001:db8::1"),
	}, ips)
}
