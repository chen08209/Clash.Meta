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
	writes    atomic.Int64
}

func (packet *recordingWriteBackPacket) LocalAddr() net.Addr { return packet.localAddr }
func (packet *recordingWriteBackPacket) Drop()               {}

func (packet *recordingWriteBackPacket) WriteBack(b []byte, addr net.Addr) (int, error) {
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

func BenchmarkWriteBackProxyUpdate(b *testing.B) {
	adapter, _ := newWriteBackTestAdapter(b)
	proxy := NewWriteBackProxy(adapter)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		proxy.UpdateWriteBack(adapter)
	}
}
