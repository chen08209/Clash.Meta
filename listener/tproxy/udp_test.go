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
	buffers   [][]byte
	wrappers  []*packet
	metadatas []*C.Metadata
}

func (c *tproxyDropTunnel) HandleUDPPacket(udpPacket C.UDPPacket, metadata *C.Metadata) {
	c.metadatas = append(c.metadatas, metadata)
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

	handlePacketConn(conn, tunnel, []byte("first"), lAddr, rAddr, 46)
	handlePacketConn(conn, tunnel, []byte("second"), lAddr, rAddr, 0)

	require.Len(t, tunnel.buffers, 2)
	require.Equal(t, []byte("first"), tunnel.buffers[0])
	require.Equal(t, []byte("second"), tunnel.buffers[1])
	// the destination comes from the AddrPort the read loop already holds, and
	// the dscp rides beside it instead of through a per-packet addition
	require.Equal(t, "8.8.8.8", tunnel.metadatas[0].DstIP.String())
	require.EqualValues(t, 53, tunnel.metadatas[0].DstPort)
	require.EqualValues(t, 54321, tunnel.metadatas[0].SrcPort)
	require.EqualValues(t, 46, tunnel.metadatas[0].DSCP)
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
	handlePacketConn(conn, tunnel, []byte("payload"), lAddr, rAddr, 0)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		handlePacketConn(conn, tunnel, []byte("payload"), lAddr, rAddr, 0)
	}
}

// The read loop's shape, dscp included: the value rides beside the additions
// instead of being wrapped in a closure and a fresh slice per datagram.
func BenchmarkTproxyUDPHandleDSCP(b *testing.B) {
	tunnel := tproxyDropOnlyTunnel{}
	conn := &tproxyDropPacketConn{}
	lAddr := netip.MustParseAddrPort("192.168.1.2:54321")
	rAddr := netip.MustParseAddrPort("8.8.8.8:53")
	handlePacketConn(conn, tunnel, []byte("payload"), lAddr, rAddr, 46)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		handlePacketConn(conn, tunnel, []byte("payload"), lAddr, rAddr, 46)
	}
}
