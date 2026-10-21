package tproxy

import (
	"net"
	"net/netip"
	"testing"
	"time"

	C "github.com/metacubex/mihomo/constant"
	"github.com/stretchr/testify/require"
)

type tproxyDropPacketConn struct{}

func (tproxyDropPacketConn) ReadFrom([]byte) (int, net.Addr, error) {
	return 0, nil, net.ErrClosed
}

func (tproxyDropPacketConn) WriteTo([]byte, net.Addr) (int, error) {
	return 0, net.ErrClosed
}

func (tproxyDropPacketConn) Close() error        { return nil }
func (tproxyDropPacketConn) LocalAddr() net.Addr { return nil }
func (tproxyDropPacketConn) SetDeadline(time.Time) error {
	return nil
}

func (tproxyDropPacketConn) SetReadDeadline(time.Time) error {
	return nil
}

func (tproxyDropPacketConn) SetWriteDeadline(time.Time) error {
	return nil
}

type tproxyDropTunnel struct {
	C.Tunnel
	buffers  [][]byte
	wrappers []*packet
}

func (c *tproxyDropTunnel) HandleUDPPacket(udpPacket C.UDPPacket, _ *C.Metadata) {
	if tproxyPacket, ok := udpPacket.(*packet); ok {
		c.wrappers = append(c.wrappers, tproxyPacket)
	}
	c.buffers = append(c.buffers, append([]byte(nil), udpPacket.Data()...))
	udpPacket.Drop()
}

func TestTproxyUDPPacketWrappersReleaseTheirBuffers(t *testing.T) {
	conn := &tproxyDropPacketConn{}
	tunnel := &tproxyDropTunnel{}
	lAddr := netip.MustParseAddrPort("192.168.1.2:54321")
	rAddr := netip.MustParseAddrPort("8.8.8.8:53")

	handlePacketConn(conn, tunnel, []byte("first"), lAddr, rAddr)
	handlePacketConn(conn, tunnel, []byte("second"), lAddr, rAddr)

	require.Len(t, tunnel.buffers, 2)
	require.Equal(t, []byte("first"), tunnel.buffers[0])
	require.Equal(t, []byte("second"), tunnel.buffers[1])
	for _, wrapper := range tunnel.wrappers {
		require.Nil(t, wrapper.buf)
		require.Equal(t, lAddr, wrapper.lAddr)
		require.Same(t, conn, wrapper.pc)
	}
}

type tproxyDropOnlyTunnel struct {
	C.Tunnel
}

func (tproxyDropOnlyTunnel) HandleUDPPacket(udpPacket C.UDPPacket, _ *C.Metadata) {
	udpPacket.Drop()
}

func BenchmarkTproxyUDPHandle(b *testing.B) {
	tunnel := tproxyDropOnlyTunnel{}
	conn := &tproxyDropPacketConn{}
	lAddr := netip.MustParseAddrPort("192.168.1.2:54321")
	rAddr := netip.MustParseAddrPort("8.8.8.8:53")
	handlePacketConn(conn, tunnel, []byte("payload"), lAddr, rAddr)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		handlePacketConn(conn, tunnel, []byte("payload"), lAddr, rAddr)
	}
}
