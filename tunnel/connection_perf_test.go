package tunnel

import (
	"net/netip"
	"testing"

	C "github.com/metacubex/mihomo/constant"
	"github.com/stretchr/testify/require"
)

func TestPacketOriginKeyMatchesWithoutFormatting(t *testing.T) {
	origin := &C.Metadata{DstIP: netip.MustParseAddr("192.168.1.2")}
	resolved := &C.Metadata{
		DstIP: netip.MustParseAddr("203.0.113.20"),
	}
	sender := newPacketSender().(*packetSender)
	defer sender.Close()

	sender.AddMapping(origin, resolved)
	target, exists := sender.originToTarget[packetOriginKey(origin)]
	require.True(t, exists)
	require.Equal(t, resolved.DstIP, target)

	allocations := testing.AllocsPerRun(100, func() {
		key := packetOriginKey(origin)
		if key.host != "" || key.ip != origin.DstIP {
			t.Fatal("invalid packet origin key")
		}
	})
	require.EqualValues(t, 0, allocations)
}

func TestPacketOriginKeyPrefersHostOverResolvedIP(t *testing.T) {
	origin := &C.Metadata{
		Host:  "example.com",
		DstIP: netip.MustParseAddr("192.0.2.1"),
	}
	resolved := &C.Metadata{DstIP: netip.MustParseAddr("203.0.113.20")}
	sender := newPacketSender().(*packetSender)
	defer sender.Close()

	sender.AddMapping(origin, resolved)
	key := packetOriginKey(origin)
	target, exists := sender.originToTarget[key]

	require.True(t, exists)
	require.Equal(t, "example.com", key.host)
	require.False(t, key.ip.IsValid())
	require.Equal(t, resolved.DstIP, target)
}

func BenchmarkPacketOriginLookup(b *testing.B) {
	origin := &C.Metadata{DstIP: netip.MustParseAddr("192.168.1.2")}
	target := netip.MustParseAddr("203.0.113.20")
	sender := newPacketSender().(*packetSender)
	defer sender.Close()
	sender.AddMapping(origin, &C.Metadata{DstIP: target})

	b.ReportAllocs()
	b.ResetTimer()
	for index := 0; index < b.N; index++ {
		if address, exists := sender.originToTarget[packetOriginKey(origin)]; !exists ||
			address != target {
			b.Fatal("packet origin mapping was not found")
		}
	}
}
