package tunnel

import (
	"net"
	"net/netip"
	"testing"
	"time"

	C "github.com/metacubex/mihomo/constant"
	"github.com/metacubex/mihomo/transport/socks5"
	"github.com/stretchr/testify/require"
)

type tunnelDropPacketConn struct{}

func (tunnelDropPacketConn) ReadFrom([]byte) (int, net.Addr, error) {
	return 0, nil, net.ErrClosed
}

func (tunnelDropPacketConn) WriteTo([]byte, net.Addr) (int, error) {
	return 0, net.ErrClosed
}

func (tunnelDropPacketConn) Close() error        { return nil }
func (tunnelDropPacketConn) LocalAddr() net.Addr { return nil }
func (tunnelDropPacketConn) SetDeadline(time.Time) error {
	return nil
}

func (tunnelDropPacketConn) SetReadDeadline(time.Time) error {
	return nil
}

func (tunnelDropPacketConn) SetWriteDeadline(time.Time) error {
	return nil
}

type tunnelDropTunnel struct {
	C.Tunnel
	payloads [][]byte
	wrappers []*packet
}

func (c *tunnelDropTunnel) HandleUDPPacket(udpPacket C.UDPPacket, _ *C.Metadata) {
	if tunnelPacket, ok := udpPacket.(*packet); ok {
		c.wrappers = append(c.wrappers, tunnelPacket)
	}
	c.payloads = append(c.payloads, append([]byte(nil), udpPacket.Data()...))
	udpPacket.Drop()
}

func TestTunnelUDPPacketWrappersReleaseTheirBuffers(t *testing.T) {
	conn := &tunnelDropPacketConn{}
	tunnel := &tunnelDropTunnel{}
	target := socks5.AddrFromStdAddrPort(netip.MustParseAddrPort("8.8.8.8:53"))
	listener := &PacketConn{target: target}
	source := net.UDPAddrFromAddrPort(netip.MustParseAddrPort("192.168.1.2:54321"))

	listener.handleUDP(conn, tunnel, []byte("first"), source)
	listener.handleUDP(conn, tunnel, []byte("second"), source)

	require.Len(t, tunnel.payloads, 2)
	require.Equal(t, []byte("first"), tunnel.payloads[0])
	require.Equal(t, []byte("second"), tunnel.payloads[1])
	for _, wrapper := range tunnel.wrappers {
		require.Nil(t, wrapper.payload)
		require.Same(t, conn, wrapper.pc)
		require.Same(t, source, wrapper.rAddr)
	}
}

type tunnelDropOnlyTunnel struct {
	C.Tunnel
}

func (tunnelDropOnlyTunnel) HandleUDPPacket(udpPacket C.UDPPacket, _ *C.Metadata) {
	udpPacket.Drop()
}

func BenchmarkTunnelUDPHandle(b *testing.B) {
	tunnel := tunnelDropOnlyTunnel{}
	conn := tunnelDropPacketConn{}
	target := socks5.AddrFromStdAddrPort(netip.MustParseAddrPort("8.8.8.8:53"))
	listener := &PacketConn{target: target}
	source := net.UDPAddrFromAddrPort(netip.MustParseAddrPort("192.168.1.2:54321"))
	listener.handleUDP(conn, tunnel, []byte("payload"), source)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		listener.handleUDP(conn, tunnel, []byte("payload"), source)
	}
}
