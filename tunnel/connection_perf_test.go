package tunnel

import (
	"context"
	"net"
	"net/netip"
	"sync/atomic"
	"testing"
	"time"

	N "github.com/metacubex/mihomo/common/net"
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

func TestShouldRefreshDeadlineAcrossDeadlineAges(t *testing.T) {
	now := time.Now()

	require.True(t, shouldRefreshDeadline(0, now), "an unset deadline must be armed on the first packet")
	require.False(t, shouldRefreshDeadline(now.Add(udpTimeout).UnixNano(), now), "a fresh deadline must not be re-armed")
	require.False(t, shouldRefreshDeadline(now.Add(udpTimeout/2+time.Second).UnixNano(), now))
	require.True(t, shouldRefreshDeadline(now.Add(udpTimeout/2-time.Second).UnixNano(), now))
	require.True(t, shouldRefreshDeadline(now.Add(-time.Second).UnixNano(), now), "an expired deadline must be re-armed")
}

type deadlineRecordingPacketConn struct {
	N.EnhancePacketConn
	sets atomic.Int64
	last atomic.Int64
}

func (c *deadlineRecordingPacketConn) SetReadDeadline(t time.Time) error {
	c.sets.Add(1)
	c.last.Store(t.UnixNano())
	return c.EnhancePacketConn.SetReadDeadline(t)
}

func (c *deadlineRecordingPacketConn) Chains() C.Chain               { return nil }
func (c *deadlineRecordingPacketConn) ProviderChains() C.Chain       { return nil }
func (c *deadlineRecordingPacketConn) AppendToChains(C.ProxyAdapter) {}
func (c *deadlineRecordingPacketConn) RemoteDestination() string     { return "" }

func (c *deadlineRecordingPacketConn) ResolveUDP(ctx context.Context, metadata *C.Metadata) error {
	return nil
}

type deadlineTestPacket struct {
	C.UDPPacket
	localAddr net.Addr
	data      []byte
}

func (packet deadlineTestPacket) LocalAddr() net.Addr { return packet.localAddr }
func (packet deadlineTestPacket) Data() []byte        { return packet.data }
func (deadlineTestPacket) Drop()                      {}

func TestProcessPacketArmsDeadlineOncePerHalfTimeout(t *testing.T) {
	udp, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1)})
	require.NoError(t, err)
	t.Cleanup(func() { _ = udp.Close() })
	pc := &deadlineRecordingPacketConn{EnhancePacketConn: N.NewEnhancePacketConn(udp)}

	sender := newPacketSender().(*packetSender)
	t.Cleanup(sender.Close)

	origin := &C.Metadata{NetWork: C.UDP, DstIP: netip.MustParseAddr("127.0.0.2"), DstPort: 5353}
	sender.AddMapping(origin, &C.Metadata{NetWork: C.UDP, DstIP: netip.MustParseAddr("127.0.0.3"), DstPort: 5353})

	packet := C.NewPacketAdapter(
		deadlineTestPacket{localAddr: net.UDPAddrFromAddrPort(netip.MustParseAddrPort("127.0.0.1:53535")), data: []byte("ping")},
		origin,
	)

	sender.processPacket(pc, packet)
	require.EqualValues(t, 1, pc.sets.Load(), "the first packet must arm the deadline")
	firstDeadline := pc.last.Load()

	sender.processPacket(pc, packet)
	require.EqualValues(t, 1, pc.sets.Load(), "a packet inside the half-timeout window must not re-arm")
	require.Equal(t, firstDeadline, pc.last.Load())
}

func BenchmarkUDPDeadlineUnconditional(b *testing.B) {
	udp, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1)})
	if err != nil {
		b.Fatal(err)
	}
	b.Cleanup(func() { _ = udp.Close() })
	pc := N.NewEnhancePacketConn(udp)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = pc.SetReadDeadline(time.Now().Add(udpTimeout))
	}
}

func BenchmarkUDPDeadlineGated(b *testing.B) {
	last := time.Now().Add(udpTimeout).UnixNano()

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if shouldRefreshDeadline(last, time.Now()) {
			b.Fatal("a fresh deadline must not trigger a refresh")
		}
	}
}
