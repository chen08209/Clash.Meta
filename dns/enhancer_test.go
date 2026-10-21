package dns

import (
	"net/netip"
	"testing"

	"github.com/metacubex/mihomo/component/fakeip"
	C "github.com/metacubex/mihomo/constant"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestFakeIPPool(test *testing.T, prefix string) *fakeip.Pool {
	test.Helper()
	pool, err := fakeip.New(fakeip.Options{
		IPNet: netip.MustParsePrefix(prefix),
		Size:  64,
	})
	require.NoError(test, err)
	return pool
}

func TestEnhancerModeGating(test *testing.T) {
	pool := newTestFakeIPPool(test, "198.18.0.0/16")
	ip := pool.Lookup("example.com")

	normal := NewEnhancer(EnhancerConfig{EnhancedMode: C.DNSNormal, FakeIPPool: pool})
	assert.False(test, normal.FakeIPEnabled())
	assert.False(test, normal.MappingEnabled())
	assert.False(test, normal.IsExistFakeIP(ip))
	assert.False(test, normal.IsFakeIP(ip))
	assert.False(test, normal.IsFakeBroadcastIP(pool.Broadcast()))

	for _, mode := range []C.DNSMode{C.DNSFakeIP, C.DNSMapping} {
		enhancer := NewEnhancer(EnhancerConfig{EnhancedMode: mode, FakeIPPool: pool})
		assert.True(test, enhancer.MappingEnabled(), mode.String())
		assert.Equal(test, mode == C.DNSFakeIP, enhancer.FakeIPEnabled(), mode.String())
	}
}

func TestEnhancerFakeIPRange(test *testing.T) {
	pool := newTestFakeIPPool(test, "198.18.0.0/16")
	enhancer := NewEnhancer(EnhancerConfig{EnhancedMode: C.DNSFakeIP, FakeIPPool: pool})

	ip := pool.Lookup("example.com")
	assert.True(test, enhancer.IsFakeIP(ip))
	assert.True(test, enhancer.IsExistFakeIP(ip))
	host, existed := enhancer.FindHostByIP(ip)
	assert.True(test, existed)
	assert.Equal(test, "example.com", host)

	// the gateway and the broadcast are reserved, and an address outside the
	// range is never a fake ip
	assert.False(test, enhancer.IsFakeIP(pool.Gateway()))
	assert.False(test, enhancer.IsFakeIP(pool.Broadcast()))
	assert.False(test, enhancer.IsExistFakeIP(pool.Gateway()))
	assert.False(test, enhancer.IsFakeIP(netip.MustParseAddr("1.2.3.4")))

	// an unallocated address inside the range reads as a fake ip - the range
	// check is what routing needs - but not as an existing entry
	assert.True(test, enhancer.IsFakeIP(netip.MustParseAddr("198.18.99.99")))
	assert.False(test, enhancer.IsExistFakeIP(netip.MustParseAddr("198.18.99.99")))

	assert.True(test, enhancer.IsFakeBroadcastIP(pool.Broadcast()))
	assert.False(test, enhancer.IsFakeBroadcastIP(ip))
}

func TestEnhancerMappingRoundTrip(test *testing.T) {
	pool := newTestFakeIPPool(test, "198.18.0.0/16")
	enhancer := NewEnhancer(EnhancerConfig{EnhancedMode: C.DNSMapping, FakeIPPool: pool})

	// mapping mode does not hand out fake ips, but it records hosts by ip
	ip := netip.MustParseAddr("10.0.0.1")
	assert.False(test, enhancer.FakeIPEnabled())
	assert.False(test, enhancer.IsFakeIP(ip))

	enhancer.InsertHostByIP(ip, "example.com")
	host, existed := enhancer.FindHostByIP(ip)
	assert.True(test, existed)
	assert.Equal(test, "example.com", host)

	// a pool lookup wins over the mapping, and an unknown ip finds nothing
	fakeIP := pool.Lookup("example.org")
	enhancer.InsertHostByIP(fakeIP, "wrong.example")
	host, existed = enhancer.FindHostByIP(fakeIP)
	assert.True(test, existed)
	assert.Equal(test, "example.org", host)

	_, existed = enhancer.FindHostByIP(netip.MustParseAddr("10.0.0.2"))
	assert.False(test, existed)

	// normal mode keeps no mapping at all
	normal := NewEnhancer(EnhancerConfig{EnhancedMode: C.DNSNormal})
	normal.InsertHostByIP(ip, "example.com")
	_, existed = normal.FindHostByIP(ip)
	assert.False(test, existed)
}

func TestEnhancerIPv6Pool(test *testing.T) {
	pool4 := newTestFakeIPPool(test, "198.18.0.0/16")
	pool6 := newTestFakeIPPool(test, "fc00::/18")
	enhancer := NewEnhancer(EnhancerConfig{
		EnhancedMode: C.DNSFakeIP,
		FakeIPPool:   pool4,
		FakeIPPool6:  pool6,
		IPv6:         true,
	})

	ip6 := pool6.Lookup("example.com")
	assert.True(test, enhancer.IsFakeIP(ip6))
	assert.True(test, enhancer.IsExistFakeIP(ip6))
	assert.True(test, enhancer.IsFakeIP(pool4.Lookup("example.net")))
	assert.False(test, enhancer.IsFakeIP(netip.MustParseAddr("2001:db8::1")))
}

func TestEnhancerFlushPatchAndStore(test *testing.T) {
	pool := newTestFakeIPPool(test, "198.18.0.0/16")
	source := NewEnhancer(EnhancerConfig{EnhancedMode: C.DNSFakeIP, FakeIPPool: pool})
	ip := pool.Lookup("example.com")
	require.True(test, source.IsExistFakeIP(ip))

	require.NoError(test, source.FlushFakeIP())
	assert.False(test, source.IsExistFakeIP(ip))

	// patch copies the previous enhancer's pool into this one, as a config
	// reload does: the two hold separate pools over the same range
	previous := newTestFakeIPPool(test, "198.18.0.1/16")
	target := NewEnhancer(EnhancerConfig{EnhancedMode: C.DNSFakeIP, FakeIPPool: previous})
	previous.Lookup("patched.example")
	target.InsertHostByIP(netip.MustParseAddr("10.0.0.1"), "target.example")
	source.InsertHostByIP(netip.MustParseAddr("10.0.0.2"), "source.example")
	source.PatchFrom(target)

	host, existed := source.FindHostByIP(netip.MustParseAddr("10.0.0.1"))
	assert.True(test, existed)
	assert.Equal(test, "target.example", host)
	patchedIP := previous.Lookup("patched.example")
	host, existed = source.FindHostByIP(patchedIP)
	assert.True(test, existed)
	assert.Equal(test, "patched.example", host)

	source.StoreFakePoolState()
}

func TestEnhancerTTLFloor(test *testing.T) {
	enhancer := NewEnhancer(EnhancerConfig{
		EnhancedMode: C.DNSFakeIP,
		FakeIPPool:   newTestFakeIPPool(test, "198.18.0.0/16"),
		FakeIPTTL:    0,
	})
	assert.Equal(test, 1, enhancer.fakeIPTTL)
}
