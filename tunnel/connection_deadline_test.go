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
