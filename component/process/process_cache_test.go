package process

import (
	"errors"
	"net/netip"
	"sync/atomic"
	"testing"
	"time"

	"github.com/metacubex/mihomo/common/lru"
	"github.com/stretchr/testify/require"
)

func TestLookupProcessNameCachesHitsPerSocket(t *testing.T) {
	var calls atomic.Int32
	lookup := func(network string, srcIP netip.Addr, srcPort int) (uint32, string, error) {
		calls.Add(1)
		return 4242, `C:\app.exe`, nil
	}
	cache := lru.New(lru.WithSize[processCacheKey, processCacheEntry](8))
	ip := netip.MustParseAddr("192.168.1.2")

	uid, path, err := lookupProcessName(cache, time.Hour, lookup, "tcp", ip, 54321)
	require.NoError(t, err)
	require.EqualValues(t, 4242, uid)
	require.Equal(t, `C:\app.exe`, path)

	uid, path, err = lookupProcessName(cache, time.Hour, lookup, "tcp", ip, 54321)
	require.NoError(t, err)
	require.EqualValues(t, 4242, uid)
	require.Equal(t, `C:\app.exe`, path)
	require.EqualValues(t, 1, calls.Load(), "a repeat lookup must be served from the cache")

	_, _, err = lookupProcessName(cache, time.Hour, lookup, "tcp", ip, 54322)
	require.NoError(t, err)
	require.EqualValues(t, 2, calls.Load(), "a different socket must miss the cache")
}

func TestLookupProcessNameExpiresAndKeepsFailuresUncached(t *testing.T) {
	var calls atomic.Int32
	lookup := func(network string, srcIP netip.Addr, srcPort int) (uint32, string, error) {
		calls.Add(1)
		return 7, "/usr/bin/app", nil
	}
	cache := lru.New(lru.WithSize[processCacheKey, processCacheEntry](8))
	ip := netip.MustParseAddr("192.168.1.2")

	failing := func(network string, srcIP netip.Addr, srcPort int) (uint32, string, error) {
		return 0, "", errors.New("not found")
	}
	_, _, err := lookupProcessName(cache, time.Hour, failing, "udp", ip, 5353)
	require.Error(t, err)
	_, _, err = lookupProcessName(cache, time.Hour, lookup, "udp", ip, 5353)
	require.NoError(t, err)
	require.EqualValues(t, 1, calls.Load(), "a failed lookup must not be cached")

	uid, _, err := lookupProcessName(cache, 10*time.Millisecond, lookup, "udp", ip, 5353)
	require.NoError(t, err)
	require.EqualValues(t, 7, uid)
	require.EqualValues(t, 1, calls.Load())

	time.Sleep(30 * time.Millisecond)
	_, _, err = lookupProcessName(cache, 10*time.Millisecond, lookup, "udp", ip, 5353)
	require.NoError(t, err)
	require.EqualValues(t, 2, calls.Load(), "an expired entry must be looked up again")
}

func BenchmarkLookupProcessNameCacheHit(b *testing.B) {
	lookup := func(network string, srcIP netip.Addr, srcPort int) (uint32, string, error) {
		return 7, "C:\\app.exe", nil
	}
	cache := lru.New(lru.WithSize[processCacheKey, processCacheEntry](8))
	ip := netip.MustParseAddr("192.168.1.2")
	_, _, _ = lookupProcessName(cache, time.Hour, lookup, "tcp", ip, 54321)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, _, err := lookupProcessName(cache, time.Hour, lookup, "tcp", ip, 54321); err != nil {
			b.Fatal(err)
		}
	}
}
