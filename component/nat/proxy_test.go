package nat

import (
	"net"
	"net/netip"
	"sync/atomic"
	"testing"

	C "github.com/metacubex/mihomo/constant"
	"github.com/stretchr/testify/require"
)

type recordingWriteBackPacket struct {
	C.UDPPacket
	localAddr net.Addr
	addrPort  netip.AddrPort
	writes    atomic.Int64
}

func (packet *recordingWriteBackPacket) LocalAddr() net.Addr { return packet.localAddr }
func (packet *recordingWriteBackPacket) Drop()               {}

func (packet *recordingWriteBackPacket) WriteBack(b []byte, addr net.Addr) (int, error) {
	packet.writes.Add(1)
	return len(b), nil
}

func (packet *recordingWriteBackPacket) WriteBackAddrPort(b []byte, addr netip.AddrPort) (int, error) {
	packet.addrPort = addr
	packet.writes.Add(1)
	return len(b), nil
}

type legacyRecordingWriteBackPacket struct {
	C.UDPPacket
	localAddr net.Addr
	addr      net.Addr
	writes    atomic.Int64
}

func (packet *legacyRecordingWriteBackPacket) LocalAddr() net.Addr { return packet.localAddr }
func (packet *legacyRecordingWriteBackPacket) Drop()               {}

func (packet *legacyRecordingWriteBackPacket) WriteBack(b []byte, addr net.Addr) (int, error) {
	packet.addr = addr
	packet.writes.Add(1)
	return len(b), nil
}

func newWriteBackTestAdapter(t testing.TB) (C.PacketAdapter, *recordingWriteBackPacket) {
	t.Helper()

	packet := &recordingWriteBackPacket{
		localAddr: net.UDPAddrFromAddrPort(netip.MustParseAddrPort("192.168.1.2:54321")),
	}
	return C.NewPacketAdapter(packet, &C.Metadata{}), packet
}

func newLegacyWriteBackTestAdapter(t testing.TB) (C.PacketAdapter, *legacyRecordingWriteBackPacket) {
	t.Helper()

	packet := &legacyRecordingWriteBackPacket{
		localAddr: net.UDPAddrFromAddrPort(netip.MustParseAddrPort("192.168.1.2:54321")),
	}
	return C.NewPacketAdapter(packet, &C.Metadata{}), packet
}

func TestWriteBackProxyDelegatesToLatestWriteBack(t *testing.T) {
	adapter, packet := newWriteBackTestAdapter(t)
	proxy := NewWriteBackProxy(adapter)

	n, err := proxy.WriteBack([]byte("hello"), net.UDPAddrFromAddrPort(netip.MustParseAddrPort("1.2.3.4:53")))
	require.NoError(t, err)
	require.Equal(t, 5, n)
	require.EqualValues(t, 1, packet.writes.Load())

	secondAdapter, secondPacket := newWriteBackTestAdapter(t)
	proxy.UpdateWriteBack(secondAdapter)

	_, err = proxy.WriteBack([]byte("hello"), net.UDPAddrFromAddrPort(netip.MustParseAddrPort("1.2.3.4:53")))
	require.NoError(t, err)
	require.EqualValues(t, 1, secondPacket.writes.Load())
	require.EqualValues(t, 1, packet.writes.Load())
}

func TestWriteBackProxyUpdateDoesNotAllocate(t *testing.T) {
	adapter, _ := newWriteBackTestAdapter(t)
	proxy := NewWriteBackProxy(adapter)

	allocations := testing.AllocsPerRun(100, func() {
		proxy.UpdateWriteBack(adapter)
	})
	require.EqualValues(t, 0, allocations)
}

func TestWriteBackProxyDelegatesAddrPort(t *testing.T) {
	adapter, packet := newWriteBackTestAdapter(t)
	proxy := NewWriteBackProxy(adapter)
	fast, ok := proxy.(C.AddrPortWriteBack)
	require.True(t, ok)
	addr := netip.MustParseAddrPort("1.2.3.4:53")

	n, err := fast.WriteBackAddrPort([]byte("hello"), addr)
	require.NoError(t, err)
	require.Equal(t, 5, n)
	require.Equal(t, addr, packet.addrPort)
	require.EqualValues(t, 1, packet.writes.Load())
}

func TestWriteBackProxyAddrPortFallsBackToNetAddr(t *testing.T) {
	adapter, packet := newLegacyWriteBackTestAdapter(t)
	proxy := NewWriteBackProxy(adapter)
	fast := proxy.(C.AddrPortWriteBack)
	addr := netip.MustParseAddrPort("1.2.3.4:53")

	n, err := fast.WriteBackAddrPort([]byte("hello"), addr)
	require.NoError(t, err)
	require.Equal(t, 5, n)
	require.Equal(t, addr, packet.addr.(*net.UDPAddr).AddrPort())
	require.EqualValues(t, 1, packet.writes.Load())
}

func BenchmarkWriteBackProxyAddrPort(b *testing.B) {
	for _, test := range []struct {
		name    string
		adapter C.PacketAdapter
	}{
		{name: "fast", adapter: mustWriteBackTestAdapter(b)},
		{name: "legacy", adapter: mustLegacyWriteBackTestAdapter(b)},
	} {
		b.Run(test.name, func(b *testing.B) {
			proxy := NewWriteBackProxy(test.adapter)
			fast := proxy.(C.AddrPortWriteBack)
			addr := netip.MustParseAddrPort("1.2.3.4:53")
			data := []byte("hello")

			b.ReportAllocs()
			b.ResetTimer()
			for index := 0; index < b.N; index++ {
				if _, err := fast.WriteBackAddrPort(data, addr); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

func mustWriteBackTestAdapter(tb testing.TB) C.PacketAdapter {
	adapter, _ := newWriteBackTestAdapter(tb)
	return adapter
}

func mustLegacyWriteBackTestAdapter(tb testing.TB) C.PacketAdapter {
	adapter, _ := newLegacyWriteBackTestAdapter(tb)
	return adapter
}

func BenchmarkWriteBackProxyUpdate(b *testing.B) {
	adapter, _ := newWriteBackTestAdapter(b)
	proxy := NewWriteBackProxy(adapter)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		proxy.UpdateWriteBack(adapter)
	}
}
