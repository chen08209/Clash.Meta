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
	mappings := sender.mappings.Load()
	target, exists := mappings.originToTarget[packetOriginKey(origin)]
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
	target, exists := sender.mappings.Load().originToTarget[key]

	require.True(t, exists)
	require.Equal(t, "example.com", key.host)
	require.False(t, key.ip.IsValid())
	require.Equal(t, resolved.DstIP, target)
}

func TestPacketMappingConcurrentAccess(t *testing.T) {
	target := netip.MustParseAddr("203.0.113.20")
	origin := netip.MustParseAddr("192.168.1.2")
	sender := newPacketSender().(*packetSender)
	defer sender.Close()
	sender.AddMapping(
		&C.Metadata{DstIP: origin},
		&C.Metadata{DstIP: target},
	)

	done := make(chan struct{})
	go func() {
		defer close(done)
		require.Equal(t, origin, sender.RestoreReadFrom(target))
	}()
	<-done
}

func TestPacketMappingKeepsFirstDestinationAndOrigin(t *testing.T) {
	firstOrigin := &C.Metadata{DstIP: netip.MustParseAddr("192.168.1.2")}
	secondOrigin := &C.Metadata{DstIP: netip.MustParseAddr("192.168.1.3")}
	firstTarget := netip.MustParseAddr("203.0.113.20")
	secondTarget := netip.MustParseAddr("203.0.113.21")
	sender := newPacketSender().(*packetSender)
	defer sender.Close()

	sender.AddMapping(firstOrigin, &C.Metadata{DstIP: firstTarget})
	sender.AddMapping(secondOrigin, &C.Metadata{DstIP: firstTarget})
	sender.AddMapping(firstOrigin, &C.Metadata{DstIP: secondTarget})
	mappings := sender.mappings.Load()
	first, _ := mappings.originToTarget[packetOriginKey(firstOrigin)]
	firstOriginAddr, _ := mappings.targetToOrigin[firstTarget]
	secondOriginAddr, _ := mappings.targetToOrigin[secondTarget]
	require.Equal(t, firstTarget, first)
	require.Equal(t, firstOrigin.DstIP, firstOriginAddr)
	require.Equal(t, firstOrigin.DstIP, secondOriginAddr)
}

func BenchmarkPacketMappingRestore(b *testing.B) {
	target := netip.MustParseAddr("203.0.113.20")
	origin := netip.MustParseAddr("192.168.1.2")
	sender := newPacketSender().(*packetSender)
	defer sender.Close()
	sender.AddMapping(
		&C.Metadata{DstIP: origin},
		&C.Metadata{DstIP: target},
	)

	b.ReportAllocs()
	b.ResetTimer()
	for index := 0; index < b.N; index++ {
		if restored := sender.RestoreReadFrom(target); restored != origin {
			b.Fatalf("restored = %v, want %v", restored, origin)
		}
	}
}

func BenchmarkPacketMappingFirstAdd(b *testing.B) {
	origin := &C.Metadata{DstIP: netip.MustParseAddr("192.168.1.2")}
	target := &C.Metadata{DstIP: netip.MustParseAddr("203.0.113.20")}

	b.ReportAllocs()
	b.ResetTimer()
	for index := 0; index < b.N; index++ {
		sender := newPacketSender().(*packetSender)
		sender.AddMapping(origin, target)
		sender.Close()
	}
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
		if address, exists := sender.mappings.Load().originToTarget[packetOriginKey(origin)]; !exists ||
			address != target {
			b.Fatal("packet origin mapping was not found")
		}
	}
}
