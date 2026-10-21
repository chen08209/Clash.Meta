package tunnel

import (
	"context"
	"io"
	"net"
	"net/netip"
	"sync"
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
	target := sender.targetFor(packetOriginKey(origin))
	require.True(t, target.IsValid())
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
	target := sender.targetFor(key)

	require.True(t, target.IsValid())
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

	var failed atomic.Bool
	var wait sync.WaitGroup
	for worker := 0; worker < 8; worker++ {
		wait.Add(1)
		go func() {
			defer wait.Done()
			for iteration := 0; iteration < 1000; iteration++ {
				if restored := sender.RestoreReadFrom(target); restored != origin {
					failed.Store(true)
					return
				}
			}
		}()
	}
	wait.Wait()
	require.False(t, failed.Load())
}

type fastReadPacketConn struct {
	C.PacketConn
	data  []byte
	from  netip.AddrPort
	calls int
}

func (c *fastReadPacketConn) WaitReadFrom() (data []byte, addr netip.AddrPort, err error) {
	if c.calls > 0 {
		return nil, netip.AddrPort{}, io.EOF
	}
	c.calls++
	return c.data, c.from, nil
}

func (c *fastReadPacketConn) SetReadDeadline(time.Time) error { return nil }
func (c *fastReadPacketConn) Close() error                    { return nil }

type recordingWriteBack struct {
	data []byte
	addr netip.AddrPort
}

func (w *recordingWriteBack) WriteBack(data []byte, addr netip.AddrPort) (int, error) {
	w.data = append(w.data[:0], data...)
	w.addr = addr
	return len(data), nil
}

func TestHandleUDPToLocalWritesBackReadAddrPort(t *testing.T) {
	sender := newPacketSender().(*packetSender)
	pc := &fastReadPacketConn{
		data: []byte("pong"),
		from: netip.MustParseAddrPort("198.51.100.7:5353"),
	}
	writeBack := &recordingWriteBack{}

	handleUDPToLocal(writeBack, pc, sender, netip.MustParseAddrPort("192.0.2.1:1234"), netip.MustParseAddrPort("203.0.113.9:53"))

	require.Equal(t, 1, pc.calls)
	require.Equal(t, pc.data, writeBack.data)
	require.Equal(t, pc.from, writeBack.addr)
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
	first := sender.targetFor(packetOriginKey(firstOrigin))
	firstOriginAddr := sender.RestoreReadFrom(firstTarget)
	secondOriginAddr := sender.RestoreReadFrom(secondTarget)
	require.Equal(t, firstTarget, first)
	require.Equal(t, firstOrigin.DstIP, firstOriginAddr)
	require.Equal(t, firstOrigin.DstIP, secondOriginAddr)
}

func TestPacketMappingFillKeepsEveryMapping(t *testing.T) {
	const mappingCount = 4096

	origins := make([]C.Metadata, mappingCount)
	targets := make([]C.Metadata, mappingCount)
	for index := range origins {
		origins[index].DstIP = netip.AddrFrom4([4]byte{10, byte(index >> 8), byte(index), 1})
		targets[index].DstIP = netip.AddrFrom4([4]byte{203, 0, byte(index >> 8), byte(index)})
	}

	sender := newPacketSender().(*packetSender)
	defer sender.Close()
	for index := range origins {
		sender.AddMapping(&origins[index], &targets[index])
	}

	allocations := testing.AllocsPerRun(1, func() {
		checkSender := newPacketSender().(*packetSender)
		for index := range origins {
			checkSender.AddMapping(&origins[index], &targets[index])
		}
		checkSender.Close()
	})
	require.Less(t, allocations, float64(2048))

	for _, index := range []int{0, mappingCount / 2, mappingCount - 1} {
		require.Equal(t, targets[index].DstIP, sender.targetFor(packetOriginKey(&origins[index])))
		require.Equal(t, origins[index].DstIP, sender.RestoreReadFrom(targets[index].DstIP))
	}
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

func BenchmarkPacketMappingFill(b *testing.B) {
	const mappingCount = 4096

	origins := make([]C.Metadata, mappingCount)
	targets := make([]C.Metadata, mappingCount)
	for index := range origins {
		origins[index].DstIP = netip.AddrFrom4([4]byte{10, byte(index >> 8), byte(index), 1})
		targets[index].DstIP = netip.AddrFrom4([4]byte{203, 0, byte(index >> 8), byte(index)})
	}

	b.ReportAllocs()
	b.ResetTimer()
	for index := 0; index < b.N; index++ {
		sender := newPacketSender().(*packetSender)
		for mapping := range origins {
			sender.AddMapping(&origins[mapping], &targets[mapping])
		}
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
		if address := sender.targetFor(packetOriginKey(origin)); address != target {
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

type destinationRecordingPacketConn struct {
	N.EnhancePacketConn
	written   int
	firstAddr net.Addr
	reused    bool
	changed   bool
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

func TestProcessPacketReusesDestinationForRepeatedWrites(t *testing.T) {
	pc := &destinationRecordingPacketConn{}
	sender, packet := newDestinationTestFlow(t, pc)

	sender.processPacket(pc, packet)
	sender.processPacket(pc, packet)

	require.Equal(t, 2, pc.written)
	require.True(t, pc.reused, "a repeated destination must reuse the address object")
	require.False(t, pc.changed)
}

func TestProcessPacketReusesDestinationForQueuedConn(t *testing.T) {
	pc := &destinationRecordingPacketConn{}
	sender, packet := newDestinationTestFlow(t, pc)

	sender.processPacket(pc, packet)
	sender.processPacket(pc, packet)

	require.Equal(t, 2, pc.written)
	require.True(t, pc.reused, "a repeated destination must reuse the address object even for a queued conn")
	require.False(t, pc.changed)
}

func TestProcessPacketRebuildsDestinationWhenPortChanges(t *testing.T) {
	pc := &destinationRecordingPacketConn{}
	sender, packet := newDestinationTestFlow(t, pc)

	sender.processPacket(pc, packet)
	packet.Metadata().DstPort = 5354
	sender.processPacket(pc, packet)

	require.Equal(t, 2, pc.written)
	require.False(t, pc.reused, "a changed destination port must rebuild the address")
	require.True(t, pc.changed)
}

func BenchmarkProcessPacketHit(b *testing.B) {
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

// The accepted conn is handed to the handler as-is; wrapping it in a
// per-connection context object used to cost an allocation on every TCP conn.
func TestHandleTCPConnEntryAllocatesNothing(t *testing.T) {
	previousStatus := status.Load()
	status.Store(Suspend)
	t.Cleanup(func() { status.Store(previousStatus) })

	conn, remote := net.Pipe()
	t.Cleanup(func() {
		_ = conn.Close()
		_ = remote.Close()
	})
	metadata := &C.Metadata{NetWork: C.TCP, Type: C.HTTP}

	allocations := testing.AllocsPerRun(1000, func() {
		Tunnel.HandleTCPConn(conn, metadata)
	})
	require.Zero(t, allocations)
}
