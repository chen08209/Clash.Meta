package tunnel

import (
	"context"
	"net"
	"net/netip"
	"testing"
	"time"

	N "github.com/metacubex/mihomo/common/net"
	C "github.com/metacubex/mihomo/constant"
	"github.com/stretchr/testify/require"
)

type destinationRecordingPacketConn struct {
	N.EnhancePacketConn
	udp       *net.UDPConn // non-nil marks the conn as kernel-backed
	written   int
	firstAddr net.Addr
	reused    bool
	changed   bool
}

func (c *destinationRecordingPacketConn) Upstream() any {
	if c.udp == nil {
		return nil // a queued conn has no kernel socket below it
	}
	return c.udp
}

func (c *destinationRecordingPacketConn) WriteTo(b []byte, addr net.Addr) (int, error) {
	c.written++
	if c.firstAddr == nil {
		c.firstAddr = addr
	} else if addr == c.firstAddr {
		c.reused = true
	} else {
		c.changed = true
	}
	return len(b), nil
}

func (c *destinationRecordingPacketConn) SetReadDeadline(t time.Time) error { return nil }

func (c *destinationRecordingPacketConn) Chains() C.Chain               { return nil }
func (c *destinationRecordingPacketConn) ProviderChains() C.Chain       { return nil }
func (c *destinationRecordingPacketConn) AppendToChains(C.ProxyAdapter) {}
func (c *destinationRecordingPacketConn) RemoteDestination() string     { return "" }

func (c *destinationRecordingPacketConn) ResolveUDP(ctx context.Context, metadata *C.Metadata) error {
	return nil
}

func newDestinationTestFlow(t *testing.T, pc C.PacketConn) (*packetSender, C.PacketAdapter) {
	t.Helper()

	sender := newPacketSender().(*packetSender)
	t.Cleanup(sender.Close)

	origin := &C.Metadata{NetWork: C.UDP, DstIP: netip.MustParseAddr("127.0.0.2"), DstPort: 5353}
	sender.AddMapping(origin, &C.Metadata{NetWork: C.UDP, DstIP: netip.MustParseAddr("127.0.0.3"), DstPort: 5353})

	packet := C.NewPacketAdapter(
		deadlineTestPacket{localAddr: net.UDPAddrFromAddrPort(netip.MustParseAddrPort("127.0.0.1:53535")), data: []byte("ping")},
		origin,
	)
	return sender, packet
}

func TestProcessPacketReusesDestinationForKernelBackedConn(t *testing.T) {
	udp, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1)})
	require.NoError(t, err)
	t.Cleanup(func() { _ = udp.Close() })

	pc := &destinationRecordingPacketConn{udp: udp}
	sender, packet := newDestinationTestFlow(t, pc)

	sender.processPacket(pc, packet)
	sender.processPacket(pc, packet)

	require.Equal(t, 2, pc.written)
	require.True(t, pc.reused, "a repeated destination must reuse the address object")
	require.False(t, pc.changed)
}

func TestProcessPacketKeepsFreshDestinationForQueuedConn(t *testing.T) {
	pc := &destinationRecordingPacketConn{}
	sender, packet := newDestinationTestFlow(t, pc)

	sender.processPacket(pc, packet)
	sender.processPacket(pc, packet)

	require.Equal(t, 2, pc.written)
	require.False(t, pc.reused, "a conn that may queue the address must get a fresh object")
	require.True(t, pc.changed)
}

func TestProcessPacketRebuildsDestinationWhenPortChanges(t *testing.T) {
	udp, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1)})
	require.NoError(t, err)
	t.Cleanup(func() { _ = udp.Close() })

	pc := &destinationRecordingPacketConn{udp: udp}
	sender, packet := newDestinationTestFlow(t, pc)

	sender.processPacket(pc, packet)
	packet.Metadata().DstPort = 5354
	sender.processPacket(pc, packet)

	require.Equal(t, 2, pc.written)
	require.False(t, pc.reused, "a changed destination port must rebuild the address")
	require.True(t, pc.changed)
}

func BenchmarkProcessPacketHitKernelBacked(b *testing.B) {
	udp, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1)})
	if err != nil {
		b.Fatal(err)
	}
	b.Cleanup(func() { _ = udp.Close() })

	pc := &destinationRecordingPacketConn{udp: udp}
	sender := newPacketSender().(*packetSender)
	b.Cleanup(sender.Close)
	origin := &C.Metadata{NetWork: C.UDP, DstIP: netip.MustParseAddr("127.0.0.2"), DstPort: 5353}
	sender.AddMapping(origin, &C.Metadata{NetWork: C.UDP, DstIP: netip.MustParseAddr("127.0.0.3"), DstPort: 5353})
	packet := C.NewPacketAdapter(
		deadlineTestPacket{localAddr: net.UDPAddrFromAddrPort(netip.MustParseAddrPort("127.0.0.1:53535")), data: []byte("ping")},
		origin,
	)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		sender.processPacket(pc, packet)
	}
}

func BenchmarkProcessPacketHitQueued(b *testing.B) {
	pc := &destinationRecordingPacketConn{}
	sender := newPacketSender().(*packetSender)
	b.Cleanup(sender.Close)
	origin := &C.Metadata{NetWork: C.UDP, DstIP: netip.MustParseAddr("127.0.0.2"), DstPort: 5353}
	sender.AddMapping(origin, &C.Metadata{NetWork: C.UDP, DstIP: netip.MustParseAddr("127.0.0.3"), DstPort: 5353})
	packet := C.NewPacketAdapter(
		deadlineTestPacket{localAddr: net.UDPAddrFromAddrPort(netip.MustParseAddrPort("127.0.0.1:53535")), data: []byte("ping")},
		origin,
	)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		sender.processPacket(pc, packet)
	}
}
