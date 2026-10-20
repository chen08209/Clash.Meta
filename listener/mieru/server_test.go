package mieru

import (
	"net"
	"net/netip"
	"testing"
	"time"

	"github.com/metacubex/mihomo/transport/socks5"
	"github.com/stretchr/testify/require"
)

type recordingPacketConn struct {
	written  []byte
	destAddr net.Addr
	writes   int
}

func (c *recordingPacketConn) ReadFrom(p []byte) (int, net.Addr, error) {
	return 0, nil, net.ErrClosed
}

func (c *recordingPacketConn) WriteTo(p []byte, addr net.Addr) (int, error) {
	c.written = append(c.written[:0], p...)
	c.destAddr = addr
	c.writes++
	return len(p), nil
}

func (c *recordingPacketConn) Close() error                     { return nil }
func (c *recordingPacketConn) LocalAddr() net.Addr              { return &net.UDPAddr{Port: 1080} }
func (c *recordingPacketConn) SetDeadline(time.Time) error      { return nil }
func (c *recordingPacketConn) SetReadDeadline(time.Time) error  { return nil }
func (c *recordingPacketConn) SetWriteDeadline(time.Time) error { return nil }

func newWriteBackTestPacket(conn net.PacketConn) *packet {
	return &packet{
		pc:      conn,
		udpAddr: net.UDPAddrFromAddrPort(netip.MustParseAddrPort("198.51.100.7:5353")),
	}
}

func wantUDPPacket(t *testing.T, addr netip.AddrPort, payload []byte) []byte {
	t.Helper()
	packet, err := socks5.EncodeUDPPacket(socks5.AddrFromStdAddrPort(addr), payload)
	require.NoError(t, err)
	return packet
}

func TestPacketWriteBackEncodesSocksHeaderAndPayload(t *testing.T) {
	conn := &recordingPacketConn{}
	pkt := newWriteBackTestPacket(conn)

	addr := netip.MustParseAddrPort("1.2.3.4:53")
	n, err := pkt.WriteBack([]byte("hello"), addr)
	require.NoError(t, err)
	require.Equal(t, len(wantUDPPacket(t, addr, []byte("hello"))), n)
	require.Equal(t, 1, conn.writes)
	require.Equal(t, wantUDPPacket(t, addr, []byte("hello")), conn.written)
	require.Equal(t, "198.51.100.7:5353", conn.destAddr.String())
}

func TestPacketWriteBackReusesThePooledBufferWithoutBleed(t *testing.T) {
	conn := &recordingPacketConn{}
	pkt := newWriteBackTestPacket(conn)

	first := netip.MustParseAddrPort("1.2.3.4:53")
	second := netip.MustParseAddrPort("5.6.7.8:99")
	longPayload := []byte("0123456789abcdef0123456789abcdef")

	for i := 0; i < 3; i++ {
		_, err := pkt.WriteBack(longPayload, first)
		require.NoError(t, err)
		require.Equal(t, wantUDPPacket(t, first, longPayload), conn.written)

		_, err = pkt.WriteBack([]byte("hi"), second)
		require.NoError(t, err)
		require.Equal(t, wantUDPPacket(t, second, []byte("hi")), conn.written)
	}
}

func TestPacketWriteBackStaysAllocationFree(t *testing.T) {
	conn := &recordingPacketConn{}
	pkt := newWriteBackTestPacket(conn)
	addr := netip.MustParseAddrPort("1.2.3.4:53")

	allocations := testing.AllocsPerRun(100, func() {
		if _, err := pkt.WriteBack([]byte("hello"), addr); err != nil {
			t.Fatal(err)
		}
	})
	require.EqualValues(t, 0, allocations)
}

func BenchmarkPacketWriteBack(b *testing.B) {
	conn := &recordingPacketConn{}
	pkt := newWriteBackTestPacket(conn)
	addr := netip.MustParseAddrPort("1.2.3.4:53")
	data := []byte("hello")

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := pkt.WriteBack(data, addr); err != nil {
			b.Fatal(err)
		}
	}
}
